package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/pleware/initagent/internal/brand"
)

// ConfigPath returns where the agent config lives.
func ConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, brand.ConfigDir, brand.ConnectorConfigFile), nil
}

// LoadConfig reads the enrolled-agent config from the default path.
func LoadConfig() (Config, error) {
	p, err := ConfigPath()
	if err != nil {
		return Config{}, err
	}
	return LoadConfigFrom(p)
}

// LoadConfigFrom reads a connector config from path.
func LoadConfigFrom(path string) (Config, error) {
	var cfg Config
	b, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("reading %s (is this connector enrolled? run `%s agent enroll`): %w", path, brand.Binary, err)
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return cfg, fmt.Errorf("parsing %s: %w", path, err)
	}
	return cfg, nil
}

type enrollRequest struct {
	Token    string `json:"token"`
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
}

type enrollResponse struct {
	ConnectorId    string `json:"connectorId"`
	ConnectorToken string `json:"connectorToken"`
}

// Enroll exchanges a single-use enrollment token for a permanent connector
// credential and writes the default agent config.
func Enroll(hubURL, token string) (Config, error) {
	p, err := ConfigPath()
	if err != nil {
		return Config{}, err
	}
	return EnrollTo(hubURL, token, p)
}

// EnrollTo is Enroll with an explicit config path. The self-host hub uses
// this so the first-box worker does not share ~/.initagent/connector.json
// with a separately installed connector.
func EnrollTo(hubURL, token, configPath string) (Config, error) {
	hubURL = strings.TrimRight(hubURL, "/")
	hostname, _ := os.Hostname()
	body, _ := json.Marshal(enrollRequest{
		Token: token, Hostname: hostname, OS: runtime.GOOS, Arch: runtime.GOARCH,
	})
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Post(hubURL+"/api/enroll", "application/json", bytes.NewReader(body))
	if err != nil {
		return Config{}, fmt.Errorf("reaching hub at %s: %w", hubURL, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode != http.StatusOK {
		return Config{}, fmt.Errorf("hub refused enrollment (%s): %s", resp.Status, strings.TrimSpace(string(respBody)))
	}
	var er enrollResponse
	if err := json.Unmarshal(respBody, &er); err != nil {
		return Config{}, fmt.Errorf("parsing enrollment response: %w", err)
	}
	if er.ConnectorId == "" || er.ConnectorToken == "" {
		return Config{}, fmt.Errorf("enrollment response missing connector credentials")
	}

	cfg := Config{HubURL: hubURL, ConnectorId: er.ConnectorId, Token: er.ConnectorToken}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		return Config{}, err
	}
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return Config{}, err
	}
	if err := os.WriteFile(configPath, out, 0o600); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
