package hub

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"

	"github.com/pleware/initagent/internal/auth"
	"github.com/pleware/initagent/internal/authz"
	"github.com/pleware/initagent/internal/offering"
)

func TestRegisterCustomerOnHostedClaimedHub(t *testing.T) {
	f := claimedHub(t, offering.Hosted)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}

	resp := postJSON(t, f.ts, client, "/api/register", map[string]string{
		"email":    "  Ada@Example.COM ",
		"password": "correct-horse-battery",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("register: %d, want 200", resp.StatusCode)
	}

	account, err := f.srv.store.AccountByEmail("ada@example.com")
	if err != nil || account == nil {
		t.Fatalf("AccountByEmail = (%v, %v), want the customer", account, err)
	}
	if account.IsAdmin {
		t.Error("registered account is the platform admin")
	}

	got := getMe(t, f.ts, client)
	if !got.Authenticated || got.PlatformAdmin || got.Email != "ada@example.com" {
		t.Errorf("me after register = %+v, want a signed-in customer", got)
	}
	if len(got.Orgs) != 1 || got.Orgs[0].Name != auth.DefaultOrgName || got.Orgs[0].Role != string(authz.RoleOwner) {
		t.Errorf("orgs after register = %+v, want owner of %q", got.Orgs, auth.DefaultOrgName)
	}
}

func TestRegisterRefusesSelfHost(t *testing.T) {
	f := claimedHub(t, offering.Selfhost)
	resp := postJSON(t, f.ts, &http.Client{}, "/api/register", map[string]string{
		"email":    "ada@example.com",
		"password": "correct-horse-battery",
	})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("self-host register: %d, want 404", resp.StatusCode)
	}
	n, err := f.srv.store.CountAccounts()
	if err != nil || n != 1 {
		t.Fatalf("CountAccounts after refused register = (%d, %v), want 1", n, err)
	}
	got := getMe(t, f.ts, &http.Client{})
	if got.Signup {
		t.Error("self-host /api/me offered signup")
	}
}

func TestRegisterRefusesUnclaimedHosted(t *testing.T) {
	srv := newHub(t, t.TempDir(), offering.Hosted)
	ts := httptest.NewServer(srv.mux)
	t.Cleanup(ts.Close)
	resp := postJSON(t, ts, &http.Client{}, "/api/register", map[string]string{
		"email":    "ada@example.com",
		"password": "correct-horse-battery",
	})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("unclaimed register: %d, want 409", resp.StatusCode)
	}
}

func TestRegisterDuplicateEmail(t *testing.T) {
	f := claimedHub(t, offering.Hosted)
	resp := postJSON(t, f.ts, &http.Client{}, "/api/register", map[string]string{
		"email":    "ops@example.com",
		"password": "correct-horse-battery",
	})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate register: %d, want 409", resp.StatusCode)
	}
}

func TestRegisterPasswordFloor(t *testing.T) {
	f := claimedHub(t, offering.Hosted)
	resp := postJSON(t, f.ts, &http.Client{}, "/api/register", map[string]string{
		"email":    "ada@example.com",
		"password": "hunter22",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("short password register: %d, want 400", resp.StatusCode)
	}
}

type meView struct {
	Claimed       bool         `json:"claimed"`
	Offering      string       `json:"offering"`
	Signup        bool         `json:"signup"`
	Authenticated bool         `json:"authenticated"`
	PlatformAdmin bool         `json:"platformAdmin"`
	Email         string       `json:"email"`
	Orgs          []Membership `json:"orgs"`
}

func getMe(t *testing.T, ts *httptest.Server, client *http.Client) meView {
	t.Helper()
	resp, err := client.Get(ts.URL + "/api/me")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var got meView
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	return got
}
