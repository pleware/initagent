package gdeskfront

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/pleware/initagent/internal/gdesk"
	"github.com/pleware/initagent/internal/gdeskseam"
	"github.com/pleware/initagent/internal/gdesksensor"
)

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
