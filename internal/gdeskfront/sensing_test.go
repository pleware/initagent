package gdeskfront

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/pleware/initagent/internal/gdesksensor"
	"github.com/pleware/initagent/internal/gdeskseam"
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
