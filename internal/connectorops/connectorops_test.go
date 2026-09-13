package connectorops

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/pleware/initagent/internal/protocol"
)

// fakeConn is a thread-safe Conn stub. callFn answers Call; channel and
// message traffic is recorded so tests can drive and inspect a stream.
type fakeConn struct {
	mu         sync.Mutex
	callFn     func(ctx context.Context, typ string, payload, out any) error
	nextCh     uint32
	channels   map[uint32]*Channel
	opened     chan uint32
	jsonMsgs   []protocol.Msg
	binaryMsgs map[uint32][][]byte
}

func newFakeConn() *fakeConn {
	return &fakeConn{
		channels:   map[uint32]*Channel{},
		opened:     make(chan uint32, 8),
		binaryMsgs: map[uint32][][]byte{},
	}
}

func (f *fakeConn) Call(ctx context.Context, typ string, payload, out any) error {
	if f.callFn != nil {
		return f.callFn(ctx, typ, payload, out)
	}
	return nil
}

func (f *fakeConn) OpenChannel(h *Channel) uint32 {
	f.mu.Lock()
	f.nextCh++
	id := f.nextCh
	f.channels[id] = h
	f.mu.Unlock()
	f.opened <- id
	return id
}

func (f *fakeConn) CloseChannel(id uint32) {
	f.mu.Lock()
	delete(f.channels, id)
	f.mu.Unlock()
}

func (f *fakeConn) SendJSON(m protocol.Msg) error {
	f.mu.Lock()
	f.jsonMsgs = append(f.jsonMsgs, m)
	f.mu.Unlock()
	return nil
}

func (f *fakeConn) SendBinary(ch uint32, p []byte) error {
	f.mu.Lock()
	f.binaryMsgs[ch] = append(f.binaryMsgs[ch], append([]byte(nil), p...))
	f.mu.Unlock()
	return nil
}

func (f *fakeConn) channel(id uint32) *Channel {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.channels[id]
}

func (f *fakeConn) jsonTypes() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.jsonMsgs))
	for _, m := range f.jsonMsgs {
		out = append(out, m.Type)
	}
	return out
}

func (f *fakeConn) binaries(id uint32) []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []byte
	for _, p := range f.binaryMsgs[id] {
		out = append(out, p...)
	}
	return out
}

func TestShellQuote(t *testing.T) {
	cases := map[string]string{
		"plain":     "'plain'",
		"has space": "'has space'",
		"quote's":   `'quote'\''s'`,
	}
	for in, want := range cases {
		if got := ShellQuote(in); got != want {
			t.Errorf("ShellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExecForwardsCommand(t *testing.T) {
	fc := newFakeConn()
	var got protocol.Exec
	fc.callFn = func(_ context.Context, typ string, payload, out any) error {
		if typ != protocol.TypeExec {
			t.Errorf("typ = %q", typ)
		}
		got = payload.(protocol.Exec)
		*(out.(*protocol.ExecResult)) = protocol.ExecResult{ExitCode: 0, Stdout: "ok"}
		return nil
	}
	res, err := Exec(context.Background(), fc, "echo hi", "/srv", 42)
	if err != nil {
		t.Fatal(err)
	}
	if got.Command != "echo hi" || got.Cwd != "/srv" || got.TimeoutSec != 42 {
		t.Fatalf("exec = %+v", got)
	}
	if res.Stdout != "ok" {
		t.Fatalf("result = %+v", res)
	}
}

func TestExecPropagatesError(t *testing.T) {
	fc := newFakeConn()
	fc.callFn = func(context.Context, string, any, any) error { return errors.New("nope") }
	if _, err := Exec(context.Background(), fc, "true", "", 0); err == nil {
		t.Fatal("expected error")
	}
}

func TestListDirForwardsPath(t *testing.T) {
	fc := newFakeConn()
	var got protocol.FsList
	fc.callFn = func(_ context.Context, typ string, payload, out any) error {
		got = payload.(protocol.FsList)
		*(out.(*protocol.FsListResult)) = protocol.FsListResult{
			Path:    "/srv",
			Entries: []protocol.FsEntry{{Name: "a.txt"}},
		}
		return nil
	}
	res, err := ListDir(context.Background(), fc, "/srv")
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "/srv" || len(res.Entries) != 1 || res.Entries[0].Name != "a.txt" {
		t.Fatalf("list = %+v", res)
	}
}

func TestDownloadStreamsToWriter(t *testing.T) {
	fc := newFakeConn()
	var out bytes.Buffer
	done := make(chan error, 1)
	go func() { done <- Download(fc, "/a/b.txt", &out) }()

	id := <-fc.opened
	ch := fc.channel(id)
	ch.OnBinary([]byte("hello "))
	ch.OnBinary([]byte("world"))
	ch.OnControl(protocol.Msg{Type: protocol.TypeFsEOF})

	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if out.String() != "hello world" {
		t.Fatalf("out = %q", out.String())
	}
	if !contains(fc.jsonTypes(), protocol.TypeFsRead) {
		t.Fatalf("expected fs.read, got %v", fc.jsonTypes())
	}
}

func TestDownloadReportsConnectorError(t *testing.T) {
	fc := newFakeConn()
	var out bytes.Buffer
	done := make(chan error, 1)
	go func() { done <- Download(fc, "/a/b.txt", &out) }()

	id := <-fc.opened
	ch := fc.channel(id)
	ch.OnControl(protocol.Msg{Type: protocol.TypeFsErr, Error: "no such file"})

	err := <-done
	if err == nil || !strings.Contains(err.Error(), "no such file") {
		t.Fatalf("err = %v", err)
	}
}

func TestUploadStreamsFromReader(t *testing.T) {
	fc := newFakeConn()
	r := strings.NewReader("file contents")
	done := make(chan error, 1)
	go func() { done <- Upload(fc, "/a/b.txt", r) }()

	id := <-fc.opened
	fc.channel(id).OnControl(protocol.Msg{Type: protocol.TypeFsEOF})

	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got := string(fc.binaries(id)); got != "file contents" {
		t.Fatalf("binary = %q", got)
	}
	types := fc.jsonTypes()
	if !contains(types, protocol.TypeFsWrite) || !contains(types, protocol.TypeFsEOF) {
		t.Fatalf("json types = %v", types)
	}
}

func TestUploadReportsConnectorError(t *testing.T) {
	fc := newFakeConn()
	r := strings.NewReader("x")
	done := make(chan error, 1)
	go func() { done <- Upload(fc, "/a/b.txt", r) }()

	id := <-fc.opened
	fc.channel(id).OnControl(protocol.Msg{Type: protocol.TypeFsErr, Error: "disk full"})

	err := <-done
	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("err = %v", err)
	}
}

func TestUploadReaderError(t *testing.T) {
	fc := newFakeConn()
	r := io.MultiReader(strings.NewReader("abc"), errReader{})
	done := make(chan error, 1)
	go func() { done <- Upload(fc, "/a/b.txt", r) }()

	<-fc.opened
	err := <-done
	if err == nil {
		t.Fatal("expected reader error")
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

func contains(xs []string, x string) bool {
	for _, s := range xs {
		if s == x {
			return true
		}
	}
	return false
}
