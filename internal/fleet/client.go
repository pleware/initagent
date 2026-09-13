// Package fleet is a small REST client for the hub API, used by the
// initagent fleet CLI and the MCP server.
package fleet

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pleware/initagent/internal/brand"
)

// Client talks to a hub with an API token.
type Client struct {
	HubURL string
	Token  string
	http   *http.Client
}

// ClientConfig is stored at ~/.initagent/fleet.json by `initagent fleet login`.
type ClientConfig struct {
	HubURL string `json:"hubUrl"`
	Token  string `json:"token"`
}

func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, brand.ConfigDir, brand.FleetConfigFile), nil
}

// SaveConfig persists hub URL + API token for later fleet/mcp commands.
func SaveConfig(cfg ClientConfig) error {
	p, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(cfg, "", "  ")
	return os.WriteFile(p, b, 0o600)
}

// NewFromEnv builds a client from INITAGENT_HUB/INITAGENT_TOKEN env vars or
// the fleet config file under ConfigDir.
func NewFromEnv() (*Client, error) {
	hub := os.Getenv(brand.EnvHub)
	token := os.Getenv(brand.EnvToken)
	if hub == "" || token == "" {
		p, err := configPath()
		if err == nil {
			if b, err := os.ReadFile(p); err == nil {
				var cfg ClientConfig
				if json.Unmarshal(b, &cfg) == nil {
					if hub == "" {
						hub = cfg.HubURL
					}
					if token == "" {
						token = cfg.Token
					}
				}
			}
		}
	}
	if hub == "" || token == "" {
		return nil, fmt.Errorf("no hub configured — run `%s fleet login --hub URL --token TOKEN` (create a token in the UI under Settings), or set %s and %s", brand.Binary, brand.EnvHub, brand.EnvToken)
	}
	return New(hub, token), nil
}

func New(hubURL, token string) *Client {
	return &Client{
		HubURL: strings.TrimRight(hubURL, "/"),
		Token:  token,
		http:   &http.Client{Timeout: 120 * time.Second},
	}
}

// StatusError is a refusal the hub explained, with the code kept alongside
// the message.
//
// Both halves matter once tokens are scoped: 401 says the credential is wrong
// or revoked, 403 says it is real but does not carry this verb. A caller that
// only sees the text cannot tell "sign in again" from "re-mint with one more
// scope", and those are different instructions to give a person.
type StatusError struct {
	Code    int
	Message string
}

func (e *StatusError) Error() string { return e.Message }

func (c *Client) do(method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.HubURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode >= 400 {
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(data, &e) == nil && e.Error != "" {
			return &StatusError{Code: resp.StatusCode, Message: e.Error}
		}
		return &StatusError{Code: resp.StatusCode, Message: fmt.Sprintf("%s %s: %s", method, path, resp.Status)}
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}

// --- typed API surface ---

type Connector struct {
	Id       string `json:"id"`
	Name     string `json:"name"`
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	IsHub    bool   `json:"isHub"`
	Online   bool   `json:"online"`
	Tmux     bool   `json:"tmux"`
	LastSeen int64  `json:"lastSeen"`
}

type Session struct {
	Name         string `json:"name"`
	Kind         string `json:"kind"`
	Status       string `json:"status"`
	CreatedAt    int64  `json:"createdAt"`
	LastActivity int64  `json:"lastActivity"`
	Attached     bool   `json:"attached"`
	Ephemeral    bool   `json:"ephemeral"`
	ConnectorId     string `json:"connectorId,omitempty"`
	ConnectorName   string `json:"connectorName,omitempty"`
}

type ExecResult struct {
	ExitCode  int    `json:"exitCode"`
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr"`
	Truncated bool   `json:"truncated"`
}

func (c *Client) Connectors() ([]Connector, error) {
	var out []Connector
	err := c.do("GET", "/api/connectors", nil, &out)
	return out, err
}

// ResolveConnector accepts a connector id or (case-insensitive) name.
func (c *Client) ResolveConnector(ref string) (*Connector, error) {
	connectors, err := c.Connectors()
	if err != nil {
		return nil, err
	}
	for i := range connectors {
		if connectors[i].Id == ref || strings.EqualFold(connectors[i].Name, ref) {
			return &connectors[i], nil
		}
	}
	return nil, fmt.Errorf("no connector named %q — run `%s fleet connectors` to list", ref, brand.Binary)
}

func (c *Client) FleetSessions() ([]Session, error) {
	var out []Session
	err := c.do("GET", "/api/agents", nil, &out)
	return out, err
}

func (c *Client) Sessions(connectorId string) ([]Session, error) {
	var out []Session
	err := c.do("GET", "/api/connectors/"+connectorId+"/sessions", nil, &out)
	return out, err
}

func (c *Client) CreateSession(connectorId, name, cwd, command, kind string) error {
	body := map[string]string{"name": name, "cwd": cwd, "command": command, "kind": kind}
	return c.do("POST", "/api/connectors/"+connectorId+"/sessions", body, nil)
}

func (c *Client) KillSession(connectorId, name string) error {
	return c.do("DELETE", "/api/connectors/"+connectorId+"/sessions/"+name, nil, nil)
}

func (c *Client) SendInput(connectorId, session, text string, enter bool) error {
	body := map[string]any{"text": text, "enter": enter}
	return c.do("POST", "/api/connectors/"+connectorId+"/sessions/"+session+"/input", body, nil)
}

func (c *Client) ReadOutput(connectorId, session string, lines int) (string, error) {
	var out struct {
		Output string `json:"output"`
	}
	path := fmt.Sprintf("/api/connectors/%s/sessions/%s/output?lines=%d", connectorId, session, lines)
	err := c.do("GET", path, nil, &out)
	return out.Output, err
}

func (c *Client) Run(connectorId, command, cwd string, timeoutSec int) (ExecResult, error) {
	var out ExecResult
	body := map[string]any{"command": command, "cwd": cwd, "timeoutSec": timeoutSec}
	err := c.do("POST", "/api/connectors/"+connectorId+"/exec", body, &out)
	return out, err
}
