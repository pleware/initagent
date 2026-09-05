package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/pleware/initagent/internal/protocol"
	"github.com/pleware/initagent/internal/scheduler"
)

func connectAgentWS(t *testing.T, g *Gateway) (deviceID string, conn *websocket.Conn, ts *httptest.Server) {
	t.Helper()
	deviceID, token, err := g.Store().CreateDevice(context.Background(), g.Project().ID, "box", "box", "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	ts = httptest.NewServer(g.Handler())
	t.Cleanup(ts.Close)
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/ws/agent"
	conn, _, err = websocket.DefaultDialer.Dial(wsURL, http.Header{
		"Authorization": {"Bearer " + token},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	hello, _ := protocol.NewMsg(protocol.TypeHello, 0, 0, protocol.Hello{Hostname: "box", OS: "linux"})
	if err := conn.WriteJSON(hello); err != nil {
		t.Fatal(err)
	}
	var welcome protocol.Msg
	if err := conn.ReadJSON(&welcome); err != nil {
		t.Fatal(err)
	}
	if welcome.Type != protocol.TypeWelcome {
		t.Fatalf("welcome = %+v", welcome)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if g.connFor(deviceID) != nil {
			return deviceID, conn, ts
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("agent never attached")
	return "", nil, nil
}

func replyExec(t *testing.T, conn *websocket.Conn, exit int) {
	t.Helper()
	go func() {
		for {
			var m protocol.Msg
			if err := conn.ReadJSON(&m); err != nil {
				return
			}
			if m.Type != protocol.TypeExec {
				continue
			}
			var e protocol.Exec
			_ = json.Unmarshal(m.Data, &e)
			res, _ := protocol.NewMsg(protocol.TypeResult, m.Id, 0, protocol.ExecResult{
				ExitCode: exit,
				Stdout:   e.Command,
				Stderr:   "err",
			})
			_ = conn.WriteJSON(res)
		}
	}()
}

func postTask(t *testing.T, ts *httptest.Server, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/tasks", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	ts.Config.Handler.ServeHTTP(rec, req)
	return rec
}

func TestCreateTaskExecDone(t *testing.T) {
	g := openTest(t, "")
	deviceID, conn, ts := connectAgentWS(t, g)
	replyExec(t, conn, 0)

	rec := postTask(t, ts, map[string]string{"command": "echo hi", "deviceId": deviceID})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var view TaskView
	if err := json.NewDecoder(rec.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	if view.State != string(scheduler.TaskDone) || view.ExitCode != 0 || view.Stdout != "echo hi" {
		t.Fatalf("view = %+v", view)
	}
	if view.Reason != "exec" {
		t.Fatalf("reason = %q, want the exec resolver's reason", view.Reason)
	}
	if view.AssignedWorkerID != deviceID {
		t.Fatalf("worker = %q", view.AssignedWorkerID)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/tasks/"+view.ID, nil)
	get := httptest.NewRecorder()
	g.Handler().ServeHTTP(get, req)
	if get.Code != http.StatusOK {
		t.Fatalf("get = %d", get.Code)
	}
}

func TestCreateTaskExecFailed(t *testing.T) {
	g := openTest(t, "")
	deviceID, conn, ts := connectAgentWS(t, g)
	replyExec(t, conn, 7)

	rec := postTask(t, ts, map[string]string{"command": "false", "deviceId": deviceID})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var view TaskView
	if err := json.NewDecoder(rec.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	if view.State != string(scheduler.TaskFailed) || view.ExitCode != 7 {
		t.Fatalf("view = %+v", view)
	}
}

func TestCreateTaskPicksOnlineWorker(t *testing.T) {
	g := openTest(t, "")
	_, conn, ts := connectAgentWS(t, g)
	replyExec(t, conn, 0)

	rec := postTask(t, ts, map[string]string{"command": "true"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
}

func TestCreateTaskNoWorker(t *testing.T) {
	g := openTest(t, "")
	req := httptest.NewRequest(http.MethodPost, "/api/tasks", strings.NewReader(`{"command":"true"}`))
	rec := httptest.NewRecorder()
	g.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCreateTaskEmptyCommand(t *testing.T) {
	g := openTest(t, "")
	req := httptest.NewRequest(http.MethodPost, "/api/tasks", strings.NewReader(`{"command":"  "}`))
	rec := httptest.NewRecorder()
	g.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCreateTaskBadJSON(t *testing.T) {
	g := openTest(t, "")
	req := httptest.NewRequest(http.MethodPost, "/api/tasks", strings.NewReader(`{`))
	rec := httptest.NewRecorder()
	g.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCreateTaskOfflineDevice(t *testing.T) {
	g := openTest(t, "")
	dev, _, err := g.Store().CreateDevice(context.Background(), g.Project().ID, "box", "box", "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/tasks", strings.NewReader(`{"command":"true","deviceId":"`+dev+`"}`))
	rec := httptest.NewRecorder()
	g.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCreateTaskBadDeviceID(t *testing.T) {
	g := openTest(t, "")
	req := httptest.NewRequest(http.MethodPost, "/api/tasks", strings.NewReader(`{"command":"true","deviceId":"tsk-nope"}`))
	rec := httptest.NewRecorder()
	g.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestGetTaskNotFound(t *testing.T) {
	g := openTest(t, "")
	req := httptest.NewRequest(http.MethodGet, "/api/tasks/tsk-01900000-0000-7000-8000-000000000000", nil)
	rec := httptest.NewRecorder()
	g.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/tasks/not-an-id", nil)
	rec = httptest.NewRecorder()
	g.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("bad id status = %d", rec.Code)
	}
}

func TestRunQueuedEmptyCommandFails(t *testing.T) {
	g := openTest(t, "")
	deviceID, conn, _ := connectAgentWS(t, g)
	replyExec(t, conn, 0)
	if _, err := g.Store().Enqueue(context.Background(), scheduler.Task{ProjectID: g.Project().ID}); err != nil {
		t.Fatal(err)
	}
	view, err := g.RunQueued(context.Background(), g.Project().ID, deviceID)
	if err != nil {
		t.Fatal(err)
	}
	if view.State != string(scheduler.TaskFailed) || view.Reason != "empty command" {
		t.Fatalf("view = %+v", view)
	}
}

func TestRunQueuedTimeoutFails(t *testing.T) {
	g := openTest(t, "")
	deviceID, _, _ := connectAgentWS(t, g)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := g.Store().Enqueue(context.Background(), scheduler.Task{
		ProjectID: g.Project().ID,
		Command:   "sleep",
	}); err != nil {
		t.Fatal(err)
	}
	view, err := g.RunQueued(ctx, g.Project().ID, deviceID)
	if err != nil {
		t.Fatal(err)
	}
	if view.State != string(scheduler.TaskFailed) {
		t.Fatalf("view = %+v", view)
	}
}

func TestRunQueuedDisconnectFails(t *testing.T) {
	g := openTest(t, "")
	deviceID, conn, _ := connectAgentWS(t, g)
	if _, err := g.Store().Enqueue(context.Background(), scheduler.Task{
		ProjectID: g.Project().ID,
		Command:   "hang",
	}); err != nil {
		t.Fatal(err)
	}
	errc := make(chan error, 1)
	var view TaskView
	go func() {
		var err error
		view, err = g.RunQueued(context.Background(), g.Project().ID, deviceID)
		errc <- err
	}()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		_ = conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
		var m protocol.Msg
		if err := conn.ReadJSON(&m); err == nil && m.Type == protocol.TypeExec {
			_ = conn.Close()
			break
		}
	}
	select {
	case err := <-errc:
		if err != nil {
			t.Fatal(err)
		}
		if view.State != string(scheduler.TaskFailed) {
			t.Fatalf("view = %+v", view)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("RunQueued did not return after disconnect")
	}
}

func TestRunQueuedOffline(t *testing.T) {
	g := openTest(t, "")
	_, err := g.RunQueued(context.Background(), g.Project().ID, mustDevice(t))
	if err != ErrDeviceOffline {
		t.Fatalf("err = %v", err)
	}
}

func TestCreateTaskNoSlot(t *testing.T) {
	g := openTest(t, "")
	deviceID, conn, ts := connectAgentWS(t, g)
	replyExec(t, conn, 0)
	if _, err := g.Store().Enqueue(context.Background(), scheduler.Task{
		ProjectID: g.Project().ID,
		Command:   "hold",
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := g.Claim(context.Background(), g.Project().ID, deviceID); err != nil {
		t.Fatal(err)
	}
	rec := postTask(t, ts, map[string]string{"command": "next", "deviceId": deviceID})
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
}

func TestRunQueuedBadExecJSON(t *testing.T) {
	g := openTest(t, "")
	deviceID, conn, _ := connectAgentWS(t, g)
	go func() {
		var m protocol.Msg
		if err := conn.ReadJSON(&m); err != nil {
			return
		}
		_ = conn.WriteJSON(protocol.Msg{Type: protocol.TypeResult, Id: m.Id, Data: json.RawMessage(`"nope"`)})
	}()
	if _, err := g.Store().Enqueue(context.Background(), scheduler.Task{
		ProjectID: g.Project().ID,
		Command:   "true",
	}); err != nil {
		t.Fatal(err)
	}
	view, err := g.RunQueued(context.Background(), g.Project().ID, deviceID)
	if err != nil {
		t.Fatal(err)
	}
	if view.State != string(scheduler.TaskFailed) {
		t.Fatalf("view = %+v", view)
	}
}

func TestGetTaskAfterClose(t *testing.T) {
	g := openTest(t, "")
	task, err := g.Store().Enqueue(context.Background(), scheduler.Task{ProjectID: g.Project().ID})
	if err != nil {
		t.Fatal(err)
	}
	g.Close()
	req := httptest.NewRequest(http.MethodGet, "/api/tasks/"+task.ID, nil)
	rec := httptest.NewRecorder()
	g.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
}
