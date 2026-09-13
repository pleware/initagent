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

	"github.com/pleware/initagent/internal/completion"
	"github.com/pleware/initagent/internal/protocol"
	"github.com/pleware/initagent/internal/scheduler"
)

func connectAgentWS(t *testing.T, g *Gateway) (connectorID string, conn *websocket.Conn, ts *httptest.Server) {
	t.Helper()
	return connectAgent(t, g, protocol.Hello{Hostname: "box", OS: "linux"})
}

func connectAgent(t *testing.T, g *Gateway, hello protocol.Hello) (connectorID string, conn *websocket.Conn, ts *httptest.Server) {
	t.Helper()
	connectorID, token, err := g.Store().CreateConnector(context.Background(), g.Project().ID, "box", "box", "linux", "amd64")
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
	msg, _ := protocol.NewMsg(protocol.TypeHello, 0, 0, hello)
	if err := conn.WriteJSON(msg); err != nil {
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
		if g.connFor(connectorID) != nil {
			return connectorID, conn, ts
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
	connectorID, conn, ts := connectAgentWS(t, g)
	replyExec(t, conn, 0)

	rec := postTask(t, ts, map[string]string{"command": "echo hi", "connectorId": connectorID})
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
	if view.AssignedWorkerID != connectorID {
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
	connectorID, conn, ts := connectAgentWS(t, g)
	replyExec(t, conn, 7)

	rec := postTask(t, ts, map[string]string{"command": "false", "connectorId": connectorID})
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

func replyProcess(t *testing.T, conn *websocket.Conn, exit int) {
	t.Helper()
	go func() {
		for {
			var m protocol.Msg
			if err := conn.ReadJSON(&m); err != nil {
				return
			}
			if m.Type != protocol.TypeProcessStart {
				continue
			}
			res, _ := protocol.NewMsg(protocol.TypeResult, m.Id, 0, protocol.ProcessResult{
				Pid:      4242,
				ExitCode: exit,
			})
			_ = conn.WriteJSON(res)
		}
	}()
}

func replySendKeys(t *testing.T, conn *websocket.Conn, exit int) {
	t.Helper()
	go func() {
		for {
			var m protocol.Msg
			if err := conn.ReadJSON(&m); err != nil {
				return
			}
			if m.Type != protocol.TypeRunSendKeys {
				continue
			}
			var req protocol.RunSendKeys
			_ = json.Unmarshal(m.Data, &req)
			res, _ := protocol.NewMsg(protocol.TypeResult, m.Id, 0, protocol.RunSendKeysResult{
				ExitCode: exit,
				Output:   completion.Marker(req.Nonce, exit),
			})
			_ = conn.WriteJSON(res)
		}
	}()
}

func TestCreateTaskProcessDone(t *testing.T) {
	g := openTest(t, "")
	connectorID, conn, ts := connectAgentWS(t, g)
	replyProcess(t, conn, 0)

	rec := postTask(t, ts, map[string]string{"command": "coder", "connectorId": connectorID, "launch": "process"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var view TaskView
	if err := json.NewDecoder(rec.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	if view.State != string(scheduler.TaskDone) || view.ExitCode != 0 || view.Reason != "process" {
		t.Fatalf("view = %+v", view)
	}
	if view.Launch != scheduler.LaunchProcess {
		t.Fatalf("launch = %q", view.Launch)
	}
}

func TestCreateTaskProcessFailed(t *testing.T) {
	g := openTest(t, "")
	connectorID, conn, ts := connectAgentWS(t, g)
	replyProcess(t, conn, 3)

	rec := postTask(t, ts, map[string]string{"command": "coder", "connectorId": connectorID, "launch": "process"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var view TaskView
	if err := json.NewDecoder(rec.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	if view.State != string(scheduler.TaskFailed) || view.ExitCode != 3 || view.Reason != "process" {
		t.Fatalf("view = %+v", view)
	}
}

func TestCreateTaskSendKeysDone(t *testing.T) {
	g := openTest(t, "")
	connectorID, conn, ts := connectAgentWS(t, g)
	replySendKeys(t, conn, 0)

	rec := postTask(t, ts, map[string]string{"command": "coder", "connectorId": connectorID, "launch": "send_keys"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var view TaskView
	if err := json.NewDecoder(rec.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	if view.State != string(scheduler.TaskDone) || view.ExitCode != 0 || view.Reason != "sentinel" {
		t.Fatalf("view = %+v", view)
	}
	if view.Launch != scheduler.LaunchSendKeys {
		t.Fatalf("launch = %q", view.Launch)
	}
}

func TestCreateTaskSendKeysFailed(t *testing.T) {
	g := openTest(t, "")
	connectorID, conn, ts := connectAgentWS(t, g)
	replySendKeys(t, conn, 2)

	rec := postTask(t, ts, map[string]string{"command": "coder", "connectorId": connectorID, "launch": "send_keys"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var view TaskView
	if err := json.NewDecoder(rec.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	if view.State != string(scheduler.TaskFailed) || view.ExitCode != 2 || view.Reason != "sentinel" {
		t.Fatalf("view = %+v", view)
	}
}

func TestCreateTaskSendKeysDoneFile(t *testing.T) {
	g := openTest(t, "")
	connectorID, conn, ts := connectAgentWS(t, g)
	go func() {
		for {
			var m protocol.Msg
			if err := conn.ReadJSON(&m); err != nil {
				return
			}
			if m.Type != protocol.TypeRunSendKeys {
				continue
			}
			res, _ := protocol.NewMsg(protocol.TypeResult, m.Id, 0, protocol.RunSendKeysResult{
				ExitCode: 0,
				DoneFile: "0\n",
			})
			_ = conn.WriteJSON(res)
		}
	}()

	rec := postTask(t, ts, map[string]string{"command": "coder", "connectorId": connectorID, "launch": "send_keys"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var view TaskView
	if err := json.NewDecoder(rec.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	if view.State != string(scheduler.TaskDone) || view.ExitCode != 0 || view.Reason != "file" {
		t.Fatalf("view = %+v", view)
	}
}

func TestCreateTaskUnknownLaunch(t *testing.T) {
	g := openTest(t, "")
	connectorID, conn, ts := connectAgentWS(t, g)
	replyExec(t, conn, 0)
	rec := postTask(t, ts, map[string]string{"command": "true", "connectorId": connectorID, "launch": "tmux"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
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
	dev, _, err := g.Store().CreateConnector(context.Background(), g.Project().ID, "box", "box", "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/tasks", strings.NewReader(`{"command":"true","connectorId":"`+dev+`"}`))
	rec := httptest.NewRecorder()
	g.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCreateTaskBadConnectorID(t *testing.T) {
	g := openTest(t, "")
	req := httptest.NewRequest(http.MethodPost, "/api/tasks", strings.NewReader(`{"command":"true","connectorId":"task-nope"}`))
	rec := httptest.NewRecorder()
	g.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestGetTaskNotFound(t *testing.T) {
	g := openTest(t, "")
	req := httptest.NewRequest(http.MethodGet, "/api/tasks/task-01900000-0000-7000-8000-000000000000", nil)
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
	connectorID, conn, _ := connectAgentWS(t, g)
	replyExec(t, conn, 0)
	if _, err := g.Store().Enqueue(context.Background(), scheduler.Task{ProjectID: g.Project().ID}); err != nil {
		t.Fatal(err)
	}
	view, err := g.RunQueued(context.Background(), g.Project().ID, connectorID)
	if err != nil {
		t.Fatal(err)
	}
	if view.State != string(scheduler.TaskFailed) || view.Reason != "empty command" {
		t.Fatalf("view = %+v", view)
	}
}

func TestRunQueuedTimeoutFails(t *testing.T) {
	g := openTest(t, "")
	connectorID, _, _ := connectAgentWS(t, g)
	// The budget also covers claiming the task, which is a database write. Too
	// short and a loaded runner spends it before the device is ever asked, so
	// RunQueued returns the deadline instead of a task it never started.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := g.Store().Enqueue(context.Background(), scheduler.Task{
		ProjectID: g.Project().ID,
		Command:   "sleep",
	}); err != nil {
		t.Fatal(err)
	}
	view, err := g.RunQueued(ctx, g.Project().ID, connectorID)
	if err != nil {
		t.Fatal(err)
	}
	if view.State != string(scheduler.TaskFailed) {
		t.Fatalf("view = %+v", view)
	}
}

func TestRunQueuedDisconnectFails(t *testing.T) {
	g := openTest(t, "")
	connectorID, conn, _ := connectAgentWS(t, g)
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
		view, err = g.RunQueued(context.Background(), g.Project().ID, connectorID)
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
	if err != ErrConnectorOffline {
		t.Fatalf("err = %v", err)
	}
}

func TestCreateTaskNoSlot(t *testing.T) {
	g := openTest(t, "")
	connectorID, conn, ts := connectAgentWS(t, g)
	replyExec(t, conn, 0)
	if _, err := g.Store().Enqueue(context.Background(), scheduler.Task{
		ProjectID: g.Project().ID,
		Command:   "hold",
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := g.Claim(context.Background(), g.Project().ID, connectorID); err != nil {
		t.Fatal(err)
	}
	rec := postTask(t, ts, map[string]string{"command": "next", "connectorId": connectorID})
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
}

func TestRunQueuedBadExecJSON(t *testing.T) {
	g := openTest(t, "")
	connectorID, conn, _ := connectAgentWS(t, g)
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
	view, err := g.RunQueued(context.Background(), g.Project().ID, connectorID)
	if err != nil {
		t.Fatal(err)
	}
	if view.State != string(scheduler.TaskFailed) {
		t.Fatalf("view = %+v", view)
	}
}

func TestRunQueuedBadProcessJSON(t *testing.T) {
	g := openTest(t, "")
	connectorID, conn, _ := connectAgentWS(t, g)
	go func() {
		var m protocol.Msg
		if err := conn.ReadJSON(&m); err != nil {
			return
		}
		_ = conn.WriteJSON(protocol.Msg{Type: protocol.TypeResult, Id: m.Id, Data: json.RawMessage(`"nope"`)})
	}()
	if _, err := g.Store().Enqueue(context.Background(), scheduler.Task{
		ProjectID:  g.Project().ID,
		Command:    "coder",
		LaunchMode: scheduler.LaunchProcess,
	}); err != nil {
		t.Fatal(err)
	}
	view, err := g.RunQueued(context.Background(), g.Project().ID, connectorID)
	if err != nil {
		t.Fatal(err)
	}
	if view.State != string(scheduler.TaskFailed) {
		t.Fatalf("view = %+v", view)
	}
}

func TestRunQueuedSendKeysNoMarker(t *testing.T) {
	g := openTest(t, "")
	connectorID, conn, _ := connectAgentWS(t, g)
	go func() {
		var m protocol.Msg
		if err := conn.ReadJSON(&m); err != nil {
			return
		}
		res, _ := protocol.NewMsg(protocol.TypeResult, m.Id, 0, protocol.RunSendKeysResult{
			ExitCode: 0,
			Output:   "no marker here",
		})
		_ = conn.WriteJSON(res)
	}()
	if _, err := g.Store().Enqueue(context.Background(), scheduler.Task{
		ProjectID:  g.Project().ID,
		Command:    "coder",
		LaunchMode: scheduler.LaunchSendKeys,
	}); err != nil {
		t.Fatal(err)
	}
	view, err := g.RunQueued(context.Background(), g.Project().ID, connectorID)
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

func TestProcessOnOffline(t *testing.T) {
	g := openTest(t, "")
	_, err := g.processOn(t.Context(), &scheduler.Task{AssignedWorkerID: mustDevice(t)})
	if err != ErrConnectorOffline {
		t.Fatalf("err = %v", err)
	}
}

func TestSendKeysOnOffline(t *testing.T) {
	g := openTest(t, "")
	_, err := g.sendKeysOn(t.Context(), &scheduler.Task{AssignedWorkerID: mustDevice(t)}, "0123456789abcdef0123456789abcdef")
	if err != ErrConnectorOffline {
		t.Fatalf("err = %v", err)
	}
}
