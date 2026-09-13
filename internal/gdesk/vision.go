package gdesk

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/pleware/initagent/internal/brand"
)

// Vision is the walk-up camera sensor this desk may start. Empty means the
// connector reads no facts and the service list keeps local sensing declared.
type Vision struct {
	Camera  int
	Sensor  string
	Command string
}

func parseVision(env map[string]string) (Vision, bool, error) {
	cmd := strings.TrimSpace(env[brand.EnvGdeskVisionCommand])
	camRaw, camSet := env[brand.EnvGdeskVisionCamera]
	if !camSet && cmd == "" {
		return Vision{}, false, nil
	}
	camera := 0
	if camSet {
		raw := strings.TrimSpace(camRaw)
		if raw == "" {
			return Vision{}, false, fmt.Errorf("%w: %s is set but empty", ErrConfig, brand.EnvGdeskVisionCamera)
		}
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			return Vision{}, false, fmt.Errorf("%w: %s must be a non-negative camera index, got %q",
				ErrConfig, brand.EnvGdeskVisionCamera, camRaw)
		}
		camera = n
	}
	sensor := strings.TrimSpace(env[brand.EnvGdeskVisionSensor])
	return Vision{Camera: camera, Sensor: sensor, Command: cmd}, true, nil
}
