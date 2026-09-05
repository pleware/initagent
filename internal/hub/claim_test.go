package hub

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/pleware/initagent/internal/auth"
	"github.com/pleware/initagent/internal/brand"
	"github.com/pleware/initagent/internal/offering"
)

// newHub builds a server on dataDir without listening. Reusing one dataDir
// across two calls is how these tests restart a hub.
func newHub(t *testing.T, dataDir string, kind offering.Kind) *Server {
	t.Helper()
	srv, err := NewServer(Options{Addr: "127.0.0.1:0", DataDir: dataDir, Version: "test", Offering: kind})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.store.Close() })
	return srv
}

func postJSON(t *testing.T, ts *httptest.Server, client *http.Client, path string, body any) *http.Response {
	t.Helper()
	return requestJSON(t, ts, client, http.MethodPost, path, body)
}

// requestJSON sends any method with an optional JSON body. A nil body sends
// no body at all, which is what DELETE wants.
func requestJSON(t *testing.T, ts *httptest.Server, client *http.Client, method, path string, body any) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, ts.URL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

// A hub that already has an owner must not mint a token, and must not leave
// one on disk from an earlier unclaimed run. A live token file on a claimed
// hub is a credential nobody is watching.
func TestClaimedHubRetiresTheToken(t *testing.T) {
	dataDir := t.TempDir()
	first := newHub(t, dataDir, offering.Selfhost)
	if first.claim.expected() == "" {
		t.Fatal("a fresh hub minted no token")
	}
	tokenPath := filepath.Join(dataDir, brand.ClaimTokenFile)
	if _, err := os.Stat(tokenPath); err != nil {
		t.Fatalf("token file missing on an unclaimed hub: %v", err)
	}

	hash, err := auth.HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := first.store.ClaimHub("ops@example.com", hash, "Ops"); err != nil {
		t.Fatal(err)
	}
	if err := first.store.Close(); err != nil {
		t.Fatal(err)
	}

	second := newHub(t, dataDir, offering.Selfhost)
	if got := second.claim.expected(); got != "" {
		t.Errorf("claimed hub minted a token: %q", got)
	}
	if _, err := os.Stat(tokenPath); !os.IsNotExist(err) {
		t.Errorf("token file survived on a claimed hub: %v", err)
	}
}

// A self-host hub set up before accounts existed carries upstream's
// anonymous password setting. It counts as claimed, so a stranger cannot
// take it over after an upgrade, and its password keeps working.
func TestLegacyPasswordInstall(t *testing.T) {
	dataDir := t.TempDir()
	first := newHub(t, dataDir, offering.Selfhost)
	hash, err := auth.HashPassword("upstream-secret")
	if err != nil {
		t.Fatal(err)
	}
	if err := first.store.SetSetting(legacyPasswordSetting, hash); err != nil {
		t.Fatal(err)
	}
	if err := first.store.Close(); err != nil {
		t.Fatal(err)
	}

	srv := newHub(t, dataDir, offering.Selfhost)
	if claimed, err := srv.claimed(); err != nil || !claimed {
		t.Fatalf("claimed() = (%v, %v) for a hub with the upstream secret, want true", claimed, err)
	}
	if got := srv.claim.expected(); got != "" {
		t.Errorf("hub with the upstream secret minted a token: %q", got)
	}

	ts := httptest.NewServer(srv.mux)
	t.Cleanup(ts.Close)
	client := &http.Client{}
	if resp := postJSON(t, ts, client, "/api/login", map[string]string{"password": "upstream-secret"}); resp.StatusCode != 200 {
		t.Errorf("legacy login: %d, want 200", resp.StatusCode)
	}
	if resp := postJSON(t, ts, client, "/api/login", map[string]string{"password": "wrong"}); resp.StatusCode != 401 {
		t.Errorf("legacy login with a wrong password: %d, want 401", resp.StatusCode)
	}
}

// The login screen has to be able to say which kind of hub this is, and
// whether it still needs claiming.
func TestMeReportsOfferingAndClaimState(t *testing.T) {
	dataDir := t.TempDir()
	srv := newHub(t, dataDir, offering.Hosted)
	ts := httptest.NewServer(srv.mux)
	t.Cleanup(ts.Close)

	var me struct {
		Claimed        bool   `json:"claimed"`
		Offering       string `json:"offering"`
		Signup         bool   `json:"signup"`
		DefaultOrgName string `json:"defaultOrgName"`
		Authenticated  bool   `json:"authenticated"`
	}
	get := func() {
		t.Helper()
		resp, err := http.Get(ts.URL + "/api/me")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if err := json.NewDecoder(resp.Body).Decode(&me); err != nil {
			t.Fatal(err)
		}
	}

	get()
	if me.Offering != string(offering.Hosted) {
		t.Errorf("offering = %q, want %q", me.Offering, offering.Hosted)
	}
	if me.DefaultOrgName != auth.DefaultOrgName {
		t.Errorf("defaultOrgName = %q, want %q", me.DefaultOrgName, auth.DefaultOrgName)
	}
	if me.Claimed || me.Authenticated || me.Signup {
		t.Errorf("fresh hub reports claimed=%v authenticated=%v signup=%v, want all false", me.Claimed, me.Authenticated, me.Signup)
	}

	hash, err := auth.HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := srv.store.ClaimHub("ops@example.com", hash, "Ops"); err != nil {
		t.Fatal(err)
	}
	get()
	if !me.Claimed {
		t.Error("hub with an admin account still reports itself unclaimed")
	}
	if !me.Signup {
		t.Error("claimed hosted hub did not offer signup")
	}
}

// The hosted floor is higher than the self-host one because the same
// credential now guards a public control plane.
func TestClaimPasswordFloorFollowsOffering(t *testing.T) {
	tests := []struct {
		name     string
		kind     offering.Kind
		password string
		want     int
	}{
		{"self-host accepts eight", offering.Selfhost, "hunter22", 200},
		{"hosted refuses eight", offering.Hosted, "hunter22", 400},
		{"hosted accepts thirteen", offering.Hosted, "correct-horse", 200},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := newHub(t, t.TempDir(), tc.kind)
			ts := httptest.NewServer(srv.mux)
			t.Cleanup(ts.Close)
			resp := postJSON(t, ts, &http.Client{}, "/api/setup", map[string]string{
				"email":    "ops@example.com",
				"password": tc.password,
				"token":    srv.claim.expected(),
			})
			if resp.StatusCode != tc.want {
				t.Errorf("claim with %q on %s: %d, want %d", tc.password, tc.kind, resp.StatusCode, tc.want)
			}
		})
	}
}
