package gdeskfront

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"

	"github.com/pleware/initagent/internal/gdesk"
	"github.com/pleware/initagent/internal/gdesksensor"
)

// VisionRun replaces the default sensor launcher in tests. Production leaves it nil.
type VisionRun func(ctx context.Context, cmd *exec.Cmd, observe func(gdesksensor.Reading)) error

func visionCommand(ctx context.Context, cfg gdesk.Vision) *exec.Cmd {
	exe := cfg.Command
	if exe == "" {
		exe = "pware-vision"
	}
	sensor := cfg.Sensor
	if sensor == "" {
		sensor = fmt.Sprintf("camera-%d", cfg.Camera)
	}
	return exec.CommandContext(ctx, exe,
		"--camera", strconv.Itoa(cfg.Camera),
		"--sensor", sensor,
	)
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
	go func() {
		defer close(done)
		_ = run(ctx, visionCommand(ctx, d.visionCfg), d.observeVision)
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
