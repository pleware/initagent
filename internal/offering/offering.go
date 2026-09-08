// Package offering resolves how a hub process is run: selfhost or hosted.
//
// The token lives in <data-dir>/offering (draft 18). Missing file is
// selfhost. A flag or INITAGENT_OFFERING overrides the file for one start.
// hosted refuses to start without a Postgres URL. Do not infer offering
// from "Postgres is set".
package offering

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pleware/initagent/internal/brand"
)

// Kind is the hub offering of this installation.
type Kind string

const (
	Selfhost Kind = "selfhost"
	Hosted   Kind = "hosted"
)

// Parse accepts one token, trimmed, case-insensitive.
func Parse(s string) (Kind, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case string(Selfhost):
		return Selfhost, nil
	case string(Hosted):
		return Hosted, nil
	case "":
		return "", fmt.Errorf("offering: empty")
	default:
		return "", fmt.Errorf("offering %q: want %s or %s", s, Selfhost, Hosted)
	}
}

// Resolve picks flag, then env, then a present file, else selfhost.
// A present file that is empty or invalid is an error, not a silent default.
func Resolve(flagVal, envVal, fileBody string, filePresent bool) (Kind, error) {
	if raw := cmp.Or(strings.TrimSpace(flagVal), strings.TrimSpace(envVal)); raw != "" {
		return Parse(raw)
	}
	if filePresent {
		return Parse(fileBody)
	}
	return Selfhost, nil
}

// RequireStart enforces hosted boot rules. OAuth secrets are not required
// until cloud login ships (26).
func RequireStart(kind Kind, databaseURL string) error {
	if kind != Hosted {
		return nil
	}
	if strings.TrimSpace(databaseURL) == "" {
		return fmt.Errorf("offering %s requires Postgres (--database-url or %s)", Hosted, brand.EnvDatabaseURL)
	}
	return nil
}

// CompanionListen is the loopback address self-host `serve` binds when
// `--gateway-url` is empty. Enroll, tasks, and the first-box worker need a
// gateway; hosted never starts one here (ops runs `initagent gateway`).
const CompanionListen = "127.0.0.1:4201"

// CompanionGateway is how self-host `serve` gets a placement URL without a
// flag. A set flag always wins. Hosted with an empty flag starts nothing:
// that hub has no local gateway. The companion still talks HTTP, so hub
// and gateway stay separate packages (`02`); this is only the single-box
// default so `initagent serve` can mint an enroll command.
func CompanionGateway(kind Kind, flagURL string) (listen, url string, start bool) {
	if raw := strings.TrimSpace(flagURL); raw != "" {
		return "", raw, false
	}
	if kind != Selfhost {
		return "", "", false
	}
	return CompanionListen, "http://" + CompanionListen, true
}

// ReadFile loads <dataDir>/offering. Missing is ( "", false, nil ).
func ReadFile(dataDir string) (body string, present bool, err error) {
	path := filepath.Join(dataDir, brand.OfferingFile)
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read offering: %w", err)
	}
	return string(b), true, nil
}

// Dir is --data-dir, or ~/ConfigDir when empty.
func Dir(explicit string) (string, error) {
	if strings.TrimSpace(explicit) != "" {
		return explicit, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("offering data dir: %w", err)
	}
	return filepath.Join(home, brand.ConfigDir), nil
}
