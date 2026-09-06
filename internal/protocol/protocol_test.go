package protocol

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestFrameRoundTrip(t *testing.T) {
	payload := []byte("hello, overseer")
	frame := EncodeFrame(42, payload)
	ch, got, err := DecodeFrame(frame)
	if err != nil {
		t.Fatal(err)
	}
	if ch != 42 {
		t.Errorf("channel = %d, want 42", ch)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("payload = %q, want %q", got, payload)
	}
}

func TestFrameEmptyPayload(t *testing.T) {
	frame := EncodeFrame(7, nil)
	ch, got, err := DecodeFrame(frame)
	if err != nil {
		t.Fatal(err)
	}
	if ch != 7 || len(got) != 0 {
		t.Errorf("got channel %d payload %q", ch, got)
	}
}

func TestFrameTooShort(t *testing.T) {
	if _, _, err := DecodeFrame([]byte{1, 2}); err == nil {
		t.Error("expected error for short frame")
	}
}

func TestMsgRoundTrip(t *testing.T) {
	m, err := NewMsg(TypeExec, 9, 0, Exec{Command: "uptime", TimeoutSec: 5})
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var back Msg
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.Type != TypeExec || back.Id != 9 {
		t.Errorf("got %+v", back)
	}
	var e Exec
	if err := json.Unmarshal(back.Data, &e); err != nil {
		t.Fatal(err)
	}
	if e.Command != "uptime" || e.TimeoutSec != 5 {
		t.Errorf("got %+v", e)
	}
}

func TestProcessStartRoundTrip(t *testing.T) {
	m, err := NewMsg(TypeProcessStart, 3, 0, ProcessStart{Command: "coder", TimeoutSec: 30, RunID: "tsk-1"})
	if err != nil {
		t.Fatal(err)
	}
	var back Msg
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.Type != TypeProcessStart || back.Id != 3 {
		t.Fatalf("got %+v", back)
	}
	var p ProcessStart
	if err := json.Unmarshal(back.Data, &p); err != nil {
		t.Fatal(err)
	}
	if p.Command != "coder" || p.TimeoutSec != 30 || p.RunID != "tsk-1" {
		t.Fatalf("got %+v", p)
	}
}

func TestRunSendKeysRoundTrip(t *testing.T) {
	m, err := NewMsg(TypeRunSendKeys, 4, 0, RunSendKeys{
		Command: "coder", Nonce: "0123456789abcdef", RunID: "tsk-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	var back Msg
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.Type != TypeRunSendKeys {
		t.Fatalf("got %+v", back)
	}
	var r RunSendKeys
	if err := json.Unmarshal(back.Data, &r); err != nil {
		t.Fatal(err)
	}
	if r.Command != "coder" || r.Nonce != "0123456789abcdef" {
		t.Fatalf("got %+v", r)
	}
}
