// Package connectorops runs connector-side operations — exec, filesystem
// list/download/upload, and setup probing — over a live connector
// connection. Both the hub (single-plane fallback) and the gateway (project
// plane) perform these operations, so the logic lives here and each plane
// adapts its agentConn to Conn.
package connectorops

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/pleware/initagent/internal/protocol"
)

// Channel receives one stream's traffic from the connector.
type Channel struct {
	OnBinary  func([]byte)
	OnControl func(protocol.Msg)
}

// Conn is the server-side view of a live connector connection.
type Conn interface {
	// Call sends an id-carrying request and unmarshals the reply into out.
	Call(ctx context.Context, typ string, payload, out any) error
	// OpenChannel registers a stream handler and returns its channel id.
	OpenChannel(h *Channel) uint32
	CloseChannel(id uint32)
	SendJSON(m protocol.Msg) error
	SendBinary(channel uint32, payload []byte) error
}

// ShellQuote makes a string safe as a single sh word.
func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// Exec runs a command to completion on the device. The device gets its own
// timeout plus slack for the round trip.
func Exec(parent context.Context, c Conn, command, cwd string, timeoutSec int) (protocol.ExecResult, error) {
	d := 75 * time.Second
	if timeoutSec > 0 {
		d = time.Duration(timeoutSec+15) * time.Second
	}
	ctx, cancel := context.WithTimeout(parent, d)
	defer cancel()
	var res protocol.ExecResult
	err := c.Call(ctx, protocol.TypeExec, protocol.Exec{Command: command, Cwd: cwd, TimeoutSec: timeoutSec}, &res)
	return res, err
}

// ListDir lists a directory on the device.
func ListDir(ctx context.Context, c Conn, path string) (protocol.FsListResult, error) {
	var res protocol.FsListResult
	err := c.Call(ctx, protocol.TypeFsList, protocol.FsList{Path: path}, &res)
	return res, err
}

// Download streams path from the device to w. It reads through an internal
// pipe so a slow or dropped reader fails the connector-side write fast
// instead of blocking the device's single read loop.
func Download(c Conn, path string, w io.Writer) error {
	done := make(chan string, 1)
	pr, pw := io.Pipe()
	defer pr.Close()

	ch := c.OpenChannel(&Channel{
		OnBinary: func(p []byte) {
			buf := make([]byte, len(p))
			copy(buf, p)
			_, _ = pw.Write(buf)
		},
		OnControl: func(m protocol.Msg) {
			switch m.Type {
			case protocol.TypeFsEOF:
				done <- ""
			case protocol.TypeFsErr, protocol.TypeTermExit:
				done <- m.Error
			}
		},
	})
	defer c.CloseChannel(ch)

	req, err := protocol.NewMsg(protocol.TypeFsRead, 0, ch, protocol.FsTransfer{Path: path})
	if err != nil {
		return err
	}
	if err := c.SendJSON(req); err != nil {
		return err
	}
	go func() {
		errMsg := <-done
		if errMsg != "" {
			_ = pw.CloseWithError(fmt.Errorf("%s", errMsg))
		} else {
			_ = pw.Close()
		}
	}()

	_, err = io.Copy(w, pr)
	return err
}

// Upload streams r into target on the device. The reader is exhausted before
// this returns; a send failure tells the device to discard its partial temp
// file.
func Upload(c Conn, target string, r io.Reader) error {
	done := make(chan string, 1)
	ch := c.OpenChannel(&Channel{
		OnControl: func(m protocol.Msg) {
			switch m.Type {
			case protocol.TypeFsEOF:
				done <- ""
			case protocol.TypeFsErr, protocol.TypeTermExit:
				done <- m.Error
			}
		},
	})
	defer c.CloseChannel(ch)

	start, err := protocol.NewMsg(protocol.TypeFsWrite, 0, ch, protocol.FsTransfer{Path: target})
	if err != nil {
		return err
	}
	if err := c.SendJSON(start); err != nil {
		return err
	}
	buf := make([]byte, 64*1024)
	for {
		n, rerr := r.Read(buf)
		if n > 0 {
			if err := c.SendBinary(ch, buf[:n]); err != nil {
				_ = c.SendJSON(protocol.Msg{Type: protocol.TypeFsErr, Channel: ch, Error: err.Error()})
				return err
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			_ = c.SendJSON(protocol.Msg{Type: protocol.TypeFsErr, Channel: ch, Error: rerr.Error()})
			return rerr
		}
	}
	_ = c.SendJSON(protocol.Msg{Type: protocol.TypeFsEOF, Channel: ch})

	timer := time.NewTimer(60 * time.Second)
	defer timer.Stop()
	select {
	case errMsg := <-done:
		if errMsg != "" {
			return fmt.Errorf("%s", errMsg)
		}
		return nil
	case <-timer.C:
		return fmt.Errorf("device did not confirm upload")
	}
}
