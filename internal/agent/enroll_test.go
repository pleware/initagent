package agent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestEnrollToWritesConfig(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/enroll" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"deviceId": "dev-test", "deviceToken": "tok-test",
		})
	}))
	t.Cleanup(ts.Close)

	path := filepath.Join(t.TempDir(), "connector.json")
	cfg, err := EnrollTo(ts.URL, "enroll-token", path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DeviceId != "dev-test" || cfg.Token != "tok-test" || cfg.HubURL != ts.URL {
		t.Fatalf("cfg = %+v", cfg)
	}
	loaded, err := LoadConfigFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded != cfg {
		t.Fatalf("loaded %+v, want %+v", loaded, cfg)
	}
}

func TestEnrollToRejectsEmptyCredentials(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"deviceId": "", "deviceToken": ""})
	}))
	t.Cleanup(ts.Close)
	path := filepath.Join(t.TempDir(), "connector.json")
	if _, err := EnrollTo(ts.URL, "enroll-token", path); err == nil {
		t.Fatal("expected an error")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("should not write a config: %v", err)
	}
}

func TestLoadConfigFromMissing(t *testing.T) {
	if _, err := LoadConfigFrom(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("expected an error")
	}
}
