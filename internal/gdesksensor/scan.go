package gdesksensor

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os/exec"
)

// MaxLine bounds one sensor fact.
//
// Generous, because a crowded room is a long `faces` array, and small enough
// that a producer looping without a newline cannot take the desk's memory with
// it. A line over the bound ends the read rather than being skipped: at that
// point the stream is not this contract, and pretending to resynchronise
// mid-object is how a reader starts inventing facts.
const MaxLine = 1 << 20

// Scan reads sensor facts until the stream ends, handing each classified line
// to observe.
//
// A blank line is skipped rather than counted as malformed — a producer that
// ends its output with a newline is behaving, and a counter that ticks for good
// behaviour is a counter nobody trusts.
func Scan(r io.Reader, observe func(Reading)) error {
	lines := bufio.NewScanner(r)
	lines.Buffer(make([]byte, 0, bufio.MaxScanTokenSize), MaxLine)
	for lines.Scan() {
		line := bytes.TrimSpace(lines.Bytes())
		if len(line) == 0 {
			continue
		}
		observe(Parse(line))
	}
	if err := lines.Err(); err != nil {
		return fmt.Errorf("%w: reading facts: %w", ErrSensor, err)
	}
	return nil
}

// Run reads one sensor's facts until it exits, then reports how it went.
//
// The caller builds the command, because which binary, which flags, and where
// diagnostics land are its business — stdout is this function's. Build it with
// exec.CommandContext: a sensor dies with its parent, and a cancelled desk that
// left a camera held open is the failure this contract avoided by not having a
// port. Send cmd.Stderr somewhere: a sensor's diagnostics are the other half of
// the story, and stdout will not carry them.
func Run(cmd *exec.Cmd, observe func(Reading)) error {
	if cmd.Stdout != nil {
		return fmt.Errorf("%w: stdout is the fact channel and something else has taken it", ErrSensor)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("%w: no pipe from %s: %w", ErrSensor, cmd.Path, err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%w: %s did not start: %w", ErrSensor, cmd.Path, err)
	}
	// Read to the end before waiting: Wait closes this pipe, so the other order
	// loses whatever the sensor said last.
	readErr := Scan(out, observe)
	var exited error
	if err := cmd.Wait(); err != nil {
		exited = fmt.Errorf("%w: %s exited: %w", ErrSensor, cmd.Path, err)
	}
	return errors.Join(readErr, exited)
}
