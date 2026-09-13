package gdeskfront

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/pleware/initagent/internal/gdesk"
	"github.com/pleware/initagent/internal/gdeskseam"
	"github.com/pleware/initagent/internal/gdesksensor"
)

// operatorLog is what the console would show: the ring, read over the same hop
// the console polls, because a line kept only in the process log is a line
// nobody standing at the box can reach.
func operatorLog(t *testing.T, d *Desk) []gdeskseam.TraceLine {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+d.Addr()+gdeskseam.LogsPath, nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+testToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	var dump gdeskseam.TraceDump
	if err := json.NewDecoder(resp.Body).Decode(&dump); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return dump.Lines
}

// awaitLine polls the ring until one line matches, and names what it wanted when
// it gives up. The sensor runs in its own goroutine, so a single read would be a
// race with it.
func awaitLine(t *testing.T, d *Desk, want func(gdeskseam.TraceLine) bool, describe string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		lines := operatorLog(t, d)
		for _, line := range lines {
			if want(line) {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no line %s; log = %+v", describe, operatorLog(t, d))
}

func TestSensingRunsWhenVisionIsConfigured(t *testing.T) {
	d := serving(t, Options{
		Config: configured(t, map[string]string{
			"INITAGENT_GDESK_VISION_CAMERA": "0",
		}),
		VisionRun: func(ctx context.Context, _ *exec.Cmd, observe func(gdesksensor.Reading)) error {
			observe(gdesksensor.Parse([]byte(gdesksensor.FixtureCrowd)))
			<-ctx.Done()
			return ctx.Err()
		},
	})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		sensing := servicesOf(t, d)["local sensing"]
		if sensing.State == gdeskseam.ServiceListening && strings.Contains(sensing.Note, "2 people") {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("services = %+v", servicesOf(t, d))
}

func TestAttendanceReachesTheSeamWhenVisionReads(t *testing.T) {
	d := serving(t, Options{
		Config: configured(t, map[string]string{
			"INITAGENT_GDESK_VISION_CAMERA": "0",
		}),
		VisionRun: func(ctx context.Context, _ *exec.Cmd, observe func(gdesksensor.Reading)) error {
			observe(gdesksensor.Parse([]byte(gdesksensor.FixtureCrowd)))
			observe(gdesksensor.Parse([]byte(gdesksensor.FixtureCrowd)))
			<-ctx.Done()
			return ctx.Err()
		},
	})

	view, err := d.Views().Bind("gdesk:test", gdesk.DefaultConversation)
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	reader := view.Feed().Subscribe()
	defer reader.Close()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, event := range reader.Drain() {
			if event.Kind != gdeskseam.EventAttendanceChanged {
				continue
			}
			if event.Seq != 1 {
				t.Fatalf("seq = %d, want one attendance event after dedupe", event.Seq)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("no attendance event on the seam")
}

// TestASensorThatWillNotStartSaysSo is the defect this file was written around.
// Twice on 2026-09-13 a desk ran with a camera configured and none running, and
// the whole chain — desk log, operator console, the glass — said exactly what an
// empty room says. The launcher's error used to go to `_`.
func TestASensorThatWillNotStartSaysSo(t *testing.T) {
	d := serving(t, Options{
		Config: configured(t, map[string]string{
			"INITAGENT_GDESK_VISION_CAMERA": "0",
		}),
		VisionRun: func(context.Context, *exec.Cmd, func(gdesksensor.Reading)) error {
			return errors.New("exec: \"pware-vision\": executable file not found in %PATH%")
		},
	})

	awaitLine(t, d, func(line gdeskseam.TraceLine) bool {
		return line.Level == "warn" &&
			strings.Contains(line.Text, "camera-0") &&
			strings.Contains(line.Text, "executable file not found")
	}, "warning that names the sensor and the reason")
}

// TestTheSensorsOwnDiagnosticsReachTheOperator is the second silence: nothing
// set cmd.Stderr, so a child that explained itself on the way down explained
// itself to a closed pipe. The Python traceback from the tracker crash was never
// seen by anybody reading a log.
func TestTheSensorsOwnDiagnosticsReachTheOperator(t *testing.T) {
	d := serving(t, Options{
		Config: configured(t, map[string]string{
			"INITAGENT_GDESK_VISION_CAMERA": "0",
			"INITAGENT_GDESK_VISION_SENSOR": "camera-front",
		}),
		VisionRun: func(ctx context.Context, cmd *exec.Cmd, _ func(gdesksensor.Reading)) error {
			fmt.Fprint(cmd.Stderr, "[vision] camera 0 at 1280x720\nValueError: shapes (3,3) and (1,3)\n")
			<-ctx.Done()
			return ctx.Err()
		},
	})

	awaitLine(t, d, func(line gdeskseam.TraceLine) bool {
		return line.Level == "info" &&
			strings.Contains(line.Text, "camera-front") &&
			strings.Contains(line.Text, "ValueError")
	}, "the child's own line, named with the sensor it came from")
}

// TestAStoppedDeskDoesNotBlameItsSensor keeps the warning worth reading. Closing
// the desk kills the child, so the error the launcher returns is the signal we
// sent — and a warning on every clean stop is a warning an operator learns to
// scroll past.
func TestAStoppedDeskDoesNotBlameItsSensor(t *testing.T) {
	d, err := Open(Options{
		Config: configured(t, map[string]string{
			"INITAGENT_GDESK_VISION_CAMERA": "0",
		}),
		VisionRun: func(ctx context.Context, _ *exec.Cmd, _ func(gdesksensor.Reading)) error {
			<-ctx.Done()
			return errors.New("signal: killed")
		},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan error, 1)
	go func() { stopped <- d.Serve(ctx) }()

	// Read the ring while the desk is still up, because Close takes the port
	// with it and the hop the console polls goes with the port.
	awaitLine(t, d, func(line gdeskseam.TraceLine) bool {
		return strings.Contains(line.Text, "listening")
	}, "the listening line, so the desk was up before it was stopped")

	cancel()
	if err := <-stopped; err != nil {
		t.Fatalf("Serve: %v", err)
	}
	for _, line := range d.trace.Dump().Lines {
		if line.Level == "warn" {
			t.Fatalf("clean stop warned: %q", line.Text)
		}
	}
}

// A pipe hands over whatever arrived, not whatever was printed, so one line can
// come in three writes and three lines in one.
func TestSensorStderrSplitsOnNewlinesAcrossWrites(t *testing.T) {
	t.Parallel()
	var got []string
	w := &sensorStderr{note: func(line string) { got = append(got, line) }}

	fmt.Fprint(w, "[vision] cam")
	fmt.Fprint(w, "era 0 ready\nTraceback")
	fmt.Fprint(w, " (most recent call last):\n")

	want := []string{"[vision] camera 0 ready", "Traceback (most recent call last):"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("lines = %q, want %q", got, want)
	}
}

// Windows children end lines with CRLF, and a carriage return left on the text
// lands in the JSON the console reads.
func TestSensorStderrDropsCarriageReturnsAndBlankLines(t *testing.T) {
	t.Parallel()
	var got []string
	w := &sensorStderr{note: func(line string) { got = append(got, line) }}

	fmt.Fprint(w, "first\r\n\r\n\nsecond\r\n")

	want := []string{"first", "second"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("lines = %q, want %q", got, want)
	}
}

// A process killed mid-sentence never sends the newline, and that sentence is
// the one worth having.
func TestSensorStderrFlushesAnUnterminatedTail(t *testing.T) {
	t.Parallel()
	var got []string
	w := &sensorStderr{note: func(line string) { got = append(got, line) }}

	fmt.Fprint(w, "killed while saying")
	if len(got) != 0 {
		t.Fatalf("lines = %q, want nothing before the flush", got)
	}
	w.flush()
	w.flush()

	if fmt.Sprint(got) != fmt.Sprint([]string{"killed while saying"}) {
		t.Fatalf("lines = %q, want the tail exactly once", got)
	}
}

// A camera library writing a frame to stderr is not writing prose. Waiting for a
// newline that is not coming would hold all of it.
func TestSensorStderrDoesNotWaitForeverForANewline(t *testing.T) {
	t.Parallel()
	var got []string
	w := &sensorStderr{note: func(line string) { got = append(got, line) }}

	fmt.Fprint(w, strings.Repeat("x", maxSensorLine+10))

	if len(got) != 1 || len(got[0]) < maxSensorLine {
		t.Fatalf("lines = %d, first %d bytes; want one line of at least %d", len(got), len(got[0]), maxSensorLine)
	}
	if len(w.buf) != 0 {
		t.Fatalf("buffer kept %d bytes after emitting", len(w.buf))
	}
}
