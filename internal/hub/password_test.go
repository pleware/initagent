package hub

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/pleware/initagent/internal/auth"
	"github.com/pleware/initagent/internal/mailer"
	"github.com/pleware/initagent/internal/offering"
)

func TestPasswordForgotUnknownEmailIsOk(t *testing.T) {
	f := claimedHub(t, offering.Hosted)
	before, err := f.srv.store.countMail()
	if err != nil {
		t.Fatal(err)
	}
	resp := postJSON(t, f.ts, &http.Client{}, "/api/password/forgot", map[string]string{
		"email": "nobody@example.com",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("forgot unknown: %d, want 200", resp.StatusCode)
	}
	after, err := f.srv.store.countMail()
	if err != nil || after != before {
		t.Fatalf("mail count %d → %d, %v", before, after, err)
	}
}

func TestPasswordForgotInvalidEmail(t *testing.T) {
	f := claimedHub(t, offering.Hosted)
	resp := postJSON(t, f.ts, &http.Client{}, "/api/password/forgot", map[string]string{
		"email": "not-an-email",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("forgot invalid: %d, want 400", resp.StatusCode)
	}
}

func TestPasswordResetRoundTrip(t *testing.T) {
	f := claimedHub(t, offering.Hosted)
	const next = "new-horse-battery-staple"
	secret := requestReset(t, f, "ops@example.com")

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	resp := postJSON(t, f.ts, client, "/api/password/reset", map[string]string{
		"token":    secret,
		"password": next,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reset: %d, want 200", resp.StatusCode)
	}
	got := getMe(t, f.ts, client)
	if !got.Authenticated || got.Email != "ops@example.com" {
		t.Fatalf("me after reset = %+v", got)
	}

	old := postJSON(t, f.ts, &http.Client{}, "/api/login", map[string]string{
		"email": "ops@example.com", "password": "correct-horse-battery-staple",
	})
	if old.StatusCode != http.StatusUnauthorized {
		t.Fatalf("old password still works: %d", old.StatusCode)
	}
	fresh := f.signIn(t, "ops@example.com", next)
	me := getMe(t, f.ts, fresh)
	if !me.Authenticated {
		t.Fatal("login with new password failed")
	}

	again := postJSON(t, f.ts, &http.Client{}, "/api/password/reset", map[string]string{
		"token":    secret,
		"password": "another-horse-battery",
	})
	if again.StatusCode != http.StatusBadRequest {
		t.Fatalf("reused token: %d, want 400", again.StatusCode)
	}
}

func TestPasswordResetMailUsesAccountLocale(t *testing.T) {
	f := claimedHub(t, offering.Selfhost)
	requestReset(t, f, "ops@example.com")
	row, err := f.srv.store.lastMail()
	if err != nil || row == nil {
		t.Fatalf("lastMail: %v %v", row, err)
	}
	if row.Kind != mailer.KindPasswordReset {
		t.Fatalf("kind %s", row.Kind)
	}
	if !strings.Contains(row.Subject, "Reset your password") || strings.Contains(row.Subject, "Zresetuj") {
		t.Fatalf("default-locale mail = %q", row.Subject)
	}

	if err := f.srv.store.SetAccountLocale(f.ownerId, auth.LocalePL); err != nil {
		t.Fatal(err)
	}
	requestReset(t, f, "ops@example.com")
	pl, err := f.srv.store.lastMail()
	if err != nil || pl == nil {
		t.Fatalf("lastMail after locale change: %v %v", pl, err)
	}
	if !strings.Contains(pl.Subject, "Zresetuj") || strings.Contains(pl.Subject, "Reset your password") {
		t.Fatalf("Polish mail = %q", pl.Subject)
	}
}

func TestPasswordResetRejectsWeakAndExpired(t *testing.T) {
	f := claimedHub(t, offering.Hosted)
	secret := requestReset(t, f, "ops@example.com")
	resp := postJSON(t, f.ts, &http.Client{}, "/api/password/reset", map[string]string{
		"token":    secret,
		"password": "short",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("weak password: %d, want 400", resp.StatusCode)
	}
	// Token must still work after a refused password.
	if resp := postJSON(t, f.ts, &http.Client{}, "/api/password/reset", map[string]string{
		"token":    secret,
		"password": "new-horse-battery-staple",
	}); resp.StatusCode != http.StatusOK {
		t.Fatalf("reset after weak refusal: %d", resp.StatusCode)
	}

	other, err := auth.NewResetToken()
	if err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-2 * time.Hour)
	if err := f.srv.store.CreatePasswordReset(f.ownerId, hashToken(other), past, past.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	expired := postJSON(t, f.ts, &http.Client{}, "/api/password/reset", map[string]string{
		"token":    other,
		"password": "new-horse-battery-staple",
	})
	if expired.StatusCode != http.StatusBadRequest {
		t.Fatalf("expired token: %d, want 400", expired.StatusCode)
	}
}

func TestPasswordResetUnknownToken(t *testing.T) {
	f := claimedHub(t, offering.Hosted)
	resp := postJSON(t, f.ts, &http.Client{}, "/api/password/reset", map[string]string{
		"token":    strings.Repeat("ab", 32),
		"password": "new-horse-battery-staple",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown token: %d, want 400", resp.StatusCode)
	}
}

func TestOpenStoreCreatesPasswordResets(t *testing.T) {
	s := testStore(t)
	ok, err := s.hasTable("password_resets")
	if err != nil || !ok {
		t.Fatalf("password_resets missing: ok=%v err=%v", ok, err)
	}
}

func TestPublicOrigin(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://evil.example/api/password/forgot", nil)
	req.Host = "evil.example"
	got, err := publicOrigin(req, "app.initagent.dev")
	if err != nil || got != "https://app.initagent.dev" {
		t.Fatalf("tls domain: %q %v", got, err)
	}
	req.Header.Set("X-Forwarded-Host", "hub.example, ignored")
	req.Header.Set("X-Forwarded-Proto", "https")
	got, err = publicOrigin(req, "")
	if err != nil || got != "https://hub.example" {
		t.Fatalf("forwarded: %q %v", got, err)
	}
	bad := httptest.NewRequest(http.MethodPost, "http://x/", nil)
	req2 := bad.Clone(t.Context())
	req2.Host = "evil.example/phish"
	if _, err := publicOrigin(req2, ""); err == nil {
		t.Fatal("slash in host should fail")
	}
}

func requestReset(t *testing.T, f *adminFixture, email string) string {
	t.Helper()
	resp := postJSON(t, f.ts, &http.Client{}, "/api/password/forgot", map[string]string{"email": email})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("forgot: %d", resp.StatusCode)
	}
	row, err := f.srv.store.lastMail()
	if err != nil || row == nil {
		t.Fatalf("lastMail: %v %v", row, err)
	}
	return resetTokenFromBody(t, row.Text)
}

func resetTokenFromBody(t *testing.T, body string) string {
	t.Helper()
	const marker = "/reset?token="
	i := strings.Index(body, marker)
	if i < 0 {
		t.Fatalf("no reset link in %q", body)
	}
	rest := strings.Fields(body[i+len(marker):])[0]
	tok, err := url.QueryUnescape(strings.Trim(rest, `"`))
	if err != nil || tok == "" {
		t.Fatalf("token %q: %v", rest, err)
	}
	return tok
}

func TestPasswordForgotJSONOk(t *testing.T) {
	f := claimedHub(t, offering.Hosted)
	resp := postJSON(t, f.ts, &http.Client{}, "/api/password/forgot", map[string]string{
		"email": "ops@example.com",
	})
	defer resp.Body.Close()
	var got map[string]bool
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil || !got["ok"] {
		t.Fatalf("body = %v %v", got, err)
	}
}
