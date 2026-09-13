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
		_ = run(ctx, visionCommand(ctx, d.visionCfg), d.sensors.Observe)
	}()
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
