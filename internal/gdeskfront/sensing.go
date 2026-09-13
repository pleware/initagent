package gdeskfront

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/pleware/initagent/internal/gdesk"
	"github.com/pleware/initagent/internal/gdesksensor"
)

// VisionRun replaces the default sensor launcher in tests. Production leaves it nil.
type VisionRun func(ctx context.Context, cmd *exec.Cmd, observe func(gdesksensor.Reading)) error

func visionSensor(cfg gdesk.Vision) string {
	if cfg.Sensor != "" {
		return cfg.Sensor
	}
	return fmt.Sprintf("camera-%d", cfg.Camera)
}

func visionCommand(ctx context.Context, cfg gdesk.Vision) *exec.Cmd {
	exe := cfg.Command
	if exe == "" {
		exe = "pware-vision"
	}
	return exec.CommandContext(ctx, exe,
		"--camera", strconv.Itoa(cfg.Camera),
		"--sensor", visionSensor(cfg),
	)
}

// maxSensorLine bounds one diagnostic line. A child that writes a great deal
// with no newline is not writing prose — a camera library dumping a frame to
// stderr is the realistic case — and a reader that waited for the newline would
// hold all of it in memory.
const maxSensorLine = 4 << 10

// sensorStderr turns a child's diagnostics into desk lines, one per newline.
//
// The child's own words are kept at info rather than promoted to a warning: this
// is its log, not its verdict, and our sensor announces a healthy camera on this
// stream. What went wrong is the launcher's line, written once by startVision
// with the exit status in hand.
type sensorStderr struct {
	note func(string)
	buf  []byte
}

func (w *sensorStderr) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		w.emit(w.buf[:i])
		w.buf = w.buf[i+1:]
	}
	if len(w.buf) >= maxSensorLine {
		w.emit(w.buf)
		w.buf = nil
	}
	return len(p), nil
}

// flush reports a last line the child never terminated. A process killed
// mid-sentence is exactly when that sentence matters.
func (w *sensorStderr) flush() {
	if len(w.buf) > 0 {
		w.emit(w.buf)
		w.buf = nil
	}
}

func (w *sensorStderr) emit(line []byte) {
	text := strings.TrimRight(string(line), "\r")
	if text != "" {
		w.note(text)
	}
}

func (d *Desk) startVision(ctx context.Context) {
	if !d.visionSet || d.sensors != nil {
		return
	}
	d.sensors = gdesksensor.New()
	done := make(chan struct{})
	d.visionDone = done
	run := d.visionRun
	if run == nil {
		run = func(ctx context.Context, cmd *exec.Cmd, observe func(gdesksensor.Reading)) error {
			return gdesksensor.Run(cmd, observe)
		}
	}
	sensor := visionSensor(d.visionCfg)
	cmd := visionCommand(ctx, d.visionCfg)
	// The other half of the story. gdesksensor.Run reads facts from stdout and
	// says in its own documentation that the caller owes the child's stderr
	// somewhere; unset, it goes to a closed pipe, so a sensor that explains
	// itself on the way down explains itself to nobody.
	child := &sensorStderr{note: func(line string) { d.trace.Note("info", "gdesk %s: %s", sensor, line) }}
	cmd.Stderr = child
	go func() {
		defer close(done)
		err := run(ctx, cmd, d.observeVision)
		child.flush()
		// A desk being shut down kills its sensor, so the error here is the
		// signal we sent. Reporting that as a failure would put a warning in
		// every clean stop, and a warning nobody believes is worse than none.
		if err == nil || ctx.Err() != nil {
			return
		}
		// Warn rather than error: the desk is still a desk without a camera. It
		// answers, it just cannot see, and the row this line explains is the one
		// that would otherwise read "waiting for the first reading" forever.
		d.trace.Note("warn", "gdesk %s stopped: %v", sensor, err)
	}()
}

func (d *Desk) observeVision(r gdesksensor.Reading) {
	d.sensors.Observe(r)
	if r.Outcome != gdesksensor.OutcomeFact || d.views == nil {
		return
	}
	d.publishAttendance(attendanceFact(r.Fact))
}

func attendanceFact(f gdesksensor.Attendance) gdesk.AttendanceChanged {
	faces := make([]gdesk.AttendanceFace, len(f.Faces))
	for i, face := range f.Faces {
		faces[i] = gdesk.AttendanceFace{
			Rank:  face.Rank,
			Range: string(face.Range),
			Gaze:  string(face.Gaze),
		}
	}
	return gdesk.AttendanceChanged{
		At:     f.At,
		Sensor: f.Sensor,
		Source: f.Source,
		Total:  f.Total,
		Near:   f.Near,
		Far:    f.Far,
		Faces:  faces,
	}
}

func (d *Desk) publishAttendance(fact gdesk.AttendanceChanged) {
	d.attendanceMu.Lock()
	defer d.attendanceMu.Unlock()
	if d.lastAttendanceSet && attendanceSame(d.lastAttendance, fact) {
		return
	}
	d.lastAttendance = fact
	d.lastAttendanceSet = true
	d.views.Record(gdesk.DefaultConversation, fact)
}

func attendanceSame(a, b gdesk.AttendanceChanged) bool {
	if a.Sensor != b.Sensor || a.Source != b.Source ||
		a.Total != b.Total || a.Near != b.Near || a.Far != b.Far ||
		len(a.Faces) != len(b.Faces) {
		return false
	}
	for i := range a.Faces {
		if a.Faces[i] != b.Faces[i] {
			return false
		}
	}
	return true
}

func (d *Desk) waitVision() {
	if d.visionDone == nil {
		return
	}
	<-d.visionDone
	d.visionDone = nil
}

func sensingNote(st gdesksensor.State, producer string) string {
	if len(st.Seen) == 0 {
		if st.Malformed+st.Invalid+st.Unrecognised > 0 {
			return fmt.Sprintf("%s running; no good reading yet", producer)
		}
		return producer + " running; waiting for the first reading"
	}
	s := st.Seen[0].Attendance
	return fmt.Sprintf("%s; %d people (%d near, %d far)", producer, s.Total, s.Near, s.Far)
}
