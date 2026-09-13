package gdesk

import (
	"strings"
	"testing"
)

func TestLoadConfigVisionFromEnv(t *testing.T) {
	t.Parallel()
	env := openaiEntry()
	env["INITAGENT_GDESK_CHAT"] = "openai/gpt-4o-mini"
	env["INITAGENT_GDESK_VISION_CAMERA"] = "1"
	env["INITAGENT_GDESK_VISION_SENSOR"] = "lobby-cam"

	cfg, err := LoadConfig(env)
	if err != nil {
		t.Fatal(err)
	}
	vision, ok := cfg.Vision()
	if !ok || vision.Camera != 1 || vision.Sensor != "lobby-cam" {
		t.Fatalf("vision = %+v ok=%v", vision, ok)
	}
}

func TestLoadConfigRejectsBadVisionCamera(t *testing.T) {
	t.Parallel()
	env := openaiEntry()
	env["INITAGENT_GDESK_CHAT"] = "openai/gpt-4o-mini"
	env["INITAGENT_GDESK_VISION_CAMERA"] = "not-a-number"

	if _, err := LoadConfig(env); err == nil || !strings.Contains(err.Error(), "GDESK_VISION_CAMERA") {
		t.Fatalf("err = %v", err)
	}
}

func TestDeskFileVisionSection(t *testing.T) {
	t.Parallel()
	raw := []byte(`
seam:
  token: tok
roles:
  chat: openai/gpt-4o-mini
providers:
  - id: openai
    shape: openai
    secret_kind: openai
secrets:
  openai: sk-test
vision:
  camera: 0
  sensor: reception
`)
	env, err := deskFileToEnv(raw)
	if err != nil {
		t.Fatal(err)
	}
	if env["INITAGENT_GDESK_VISION_CAMERA"] != "0" {
		t.Fatalf("camera = %q", env["INITAGENT_GDESK_VISION_CAMERA"])
	}
	if env["INITAGENT_GDESK_VISION_SENSOR"] != "reception" {
		t.Fatalf("sensor = %q", env["INITAGENT_GDESK_VISION_SENSOR"])
	}
}
