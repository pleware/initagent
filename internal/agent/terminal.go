package agent

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"sync"

	"github.com/pleware/initagent/internal/protocol"
)

// termStream is one live PTY bridged to a hub channel.
type termStream struct {
	channel uint32
	backend terminalBackend
	once    sync.Once
}

type terminalBackend interface {
	io.ReadWriteCloser
	Resize(cols, rows uint16) error
	// KillWait reaps the child and returns its OS exit code, or -1 if unknown.
	KillWait() int
}

func (t *termStream) write(p []byte) {
	if _, err := t.backend.Write(p); err != nil {
		log.Printf("term %d: pty write: %v", t.channel, err)
	}
}

// close closes the PTY, which makes pumpTerm's Read return so the process
// tears down. It deliberately does NOT Kill or Wait the process: those happen
// only in pumpTerm, so Kill and Wait never race across goroutines.
func (t *termStream) close() {
	t.once.Do(func() {
		t.backend.Close()
	})
}

// clampDim keeps hub-supplied terminal dimensions within a sane uint16 range.
func clampDim(v int) uint16 {
	if v < 1 {
		return 1
	}
	if v > 1000 {
		return 1000
	}
	return uint16(v)
}

// termOpen attaches a PTY to the named session and streams it on channel.
//
// With tmux available the PTY runs `tmux attach` (or new-session -A for
// ephemeral-less flows); without tmux this attaches to an in-memory ephemeral
// session created by createSession's fallback.
func (a *Agent) termOpen(channel uint32, req protocol.TermOpen) {
	fail := func(err error) {
		m, _ := protocol.NewMsg(protocol.TypeTermExit, 0, channel, nil)
		m.Error = err.Error()
		a.send(m)
	}

	var cmd *exec.Cmd
	if eph := a.ephemeral(req.Session); eph != nil {
		a.attachEphemeral(channel, eph, req)
		return
	}
	if !tmuxAvailable() {
		// Same as tmux new-session -A: + Terminal names the session in the
		// browser and the first attach creates it. Windows has no tmux.
		if err := a.createEphemeral(protocol.SessionCreate{Name: req.Session, Kind: "shell"}); err != nil {
			fail(err)
			return
		}
		if eph := a.ephemeral(req.Session); eph != nil {
			a.attachEphemeral(channel, eph, req)
			return
		}
		fail(fmt.Errorf("session %q could not be created", req.Session))
		return
	}
	// Reject a duplicate open on a channel already in use, so a buggy/hostile
	// hub can't orphan the first PTY and its goroutine.
	a.mu.Lock()
	_, busy := a.terms[channel]
	a.mu.Unlock()
	if busy {
		fail(fmt.Errorf("channel %d already open", channel))
		return
	}

	// -A creates the session if it doesn't exist, so opening a terminal for a
	// brand-new name "just works".
	cmd = exec.Command("tmux", "new-session", "-A", "-s", req.Session)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	backend, err := startTerminal(cmd, clampDim(req.Cols), clampDim(req.Rows))
	if err != nil {
		fail(fmt.Errorf("starting pty: %w", err))
		return
	}
	t := &termStream{channel: channel, backend: backend}

	a.mu.Lock()
	a.terms[channel] = t
	a.mu.Unlock()

	opened, _ := protocol.NewMsg(protocol.TypeTermOpened, 0, channel, nil)
	a.send(opened)

	go a.pumpTerm(t)
}

// pumpTerm copies PTY output to the hub until the PTY dies.
func (a *Agent) pumpTerm(t *termStream) {
	buf := make([]byte, 32*1024)
	for {
		n, err := t.backend.Read(buf)
		if n > 0 {
			if werr := a.sendBinary(t.channel, buf[:n]); werr != nil {
				break
			}
		}
		if err != nil {
			break
		}
	}
	// Process cleanup happens in the platform backend. On Windows the process
	// was spawned through ConPTY rather than os/exec, so cmd.Wait is invalid;
	// the handle still answers GetExitCodeProcess after `exit`.
	code := t.backend.KillWait()
	t.close() // idempotent; ensures the PTY is closed if we exited via read error
	a.mu.Lock()
	delete(a.terms, t.channel)
	a.mu.Unlock()
	exit, _ := protocol.NewMsg(protocol.TypeTermExit, 0, t.channel, nil)
	if code >= 0 {
		exit.Error = fmt.Sprintf("exit %d", code)
	}
	a.send(exit)
}

func (a *Agent) termResize(channel uint32, req protocol.TermResize) {
	a.mu.Lock()
	t := a.terms[channel]
	a.mu.Unlock()
	if t == nil {
		return
	}
	_ = t.backend.Resize(clampDim(req.Cols), clampDim(req.Rows))
}

func (a *Agent) termClose(channel uint32) {
	a.mu.Lock()
	t := a.terms[channel]
	delete(a.terms, channel)
	a.mu.Unlock()
	if t != nil {
		// Detach politely: closing the PTY ends `tmux attach` without killing
		// the underlying session.
		t.close()
	}
}
