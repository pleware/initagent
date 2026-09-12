package hub

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/pleware/initagent/internal/auth"
	"github.com/pleware/initagent/internal/authz"
	"github.com/pleware/initagent/internal/mailer"
	"github.com/pleware/initagent/internal/offering"
	"github.com/pleware/initagent/internal/orgplan"
)

func TestOpenStoreCreatesOrgInvites(t *testing.T) {
	s := testStore(t)
	ok, err := s.hasTable("org_invites")
	if err != nil || !ok {
		t.Fatalf("org_invites missing: ok=%v err=%v", ok, err)
	}
}

func TestInviteSelfHostCreateRedeemNewPerson(t *testing.T) {
	f := claimedHub(t, offering.Selfhost)
	created := createInvite(t, f, f.client, f.orgId, "dev@example.com", "")
	if created.Role != string(authz.RoleMember) {
		t.Fatalf("default role = %q, want member", created.Role)
	}
	if !strings.Contains(created.Link, "/invite?token=") {
		t.Fatalf("link = %q", created.Link)
	}
	if !strings.HasPrefix(created.Id, "org_invite-") {
		t.Fatalf("id = %q", created.Id)
	}

	listed := listInvites(t, f, f.client, f.orgId)
	if len(listed) != 1 || listed[0].Email != "dev@example.com" {
		t.Fatalf("list = %+v", listed)
	}
	raw, _ := json.Marshal(listed)
	if strings.Contains(string(raw), "token") || strings.Contains(string(raw), created.Link) {
		t.Fatalf("list leaked a secret: %s", raw)
	}

	secret := inviteSecret(created.Link)
	preview := peekInvite(t, f, secret)
	if preview.Email != "dev@example.com" || preview.OrgName != "Example Ops" || preview.Role != "member" {
		t.Fatalf("peek = %+v", preview)
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	resp := postJSON(t, f.ts, client, "/api/invite/redeem", map[string]string{
		"token":    secret,
		"email":    "dev@example.com",
		"password": "correct-horse-battery",
		"locale":   "pl",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("redeem: %d", resp.StatusCode)
	}
	me := getMe(t, f.ts, client)
	if !me.Authenticated || me.Email != "dev@example.com" || me.Locale != auth.LocalePL {
		t.Fatalf("me after redeem = %+v", me)
	}
	if len(me.Orgs) != 1 || me.Orgs[0].OrgId != f.orgId || me.Orgs[0].Role != "member" {
		t.Fatalf("orgs after redeem = %+v", me.Orgs)
	}

	again := postJSON(t, f.ts, &http.Client{}, "/api/invite/redeem", map[string]string{
		"token":    secret,
		"email":    "dev@example.com",
		"password": "correct-horse-battery",
	})
	if again.StatusCode != http.StatusBadRequest {
		t.Fatalf("reused token: %d, want 400", again.StatusCode)
	}
}

func TestInviteHostedFreeRefusesCreate(t *testing.T) {
	f := hostedCustomer(t)
	resp := f.do(t, http.MethodPost, "/api/orgs/"+f.orgId+"/invites", map[string]string{
		"email": "dev@example.com",
	})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("free invite: %d, want 409", resp.StatusCode)
	}
	var body struct {
		Code  string `json:"code"`
		Wall  string `json:"wall"`
		Limit int    `json:"limit"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "plan_limit" || body.Wall != "people" || body.Limit != 1 {
		t.Fatalf("wall = %+v", body)
	}
}

func TestInviteHostedEnterpriseRedeem(t *testing.T) {
	f := hostedCustomer(t)
	if err := f.srv.store.SetOrgPlan(f.orgId, orgplan.Enterprise); err != nil {
		t.Fatal(err)
	}
	created := createInvite(t, f, f.client, f.orgId, "dev@example.com", "admin")
	if created.Role != "admin" {
		t.Fatalf("role = %q", created.Role)
	}
	resp := postJSON(t, f.ts, &http.Client{}, "/api/invite/redeem", map[string]string{
		"token":    inviteSecret(created.Link),
		"email":    "dev@example.com",
		"password": "correct-horse-battery",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("redeem: %d", resp.StatusCode)
	}
	members, err := f.srv.store.ListOrgMembers(f.orgId)
	if err != nil || len(members) != 2 {
		t.Fatalf("members = %d, %v", len(members), err)
	}
}

func TestInviteExistingAccountAttachesNoSecondOrg(t *testing.T) {
	f := hostedCustomer(t)
	if err := f.srv.store.SetOrgPlan(f.orgId, orgplan.Enterprise); err != nil {
		t.Fatal(err)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	other := &http.Client{Jar: jar}
	resp := postJSON(t, f.ts, other, "/api/register", map[string]string{
		"email":    "bob@example.com",
		"password": "correct-horse-battery",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("register bob: %d", resp.StatusCode)
	}
	bob, err := f.srv.store.AccountByEmail("bob@example.com")
	if err != nil || bob == nil {
		t.Fatal(err)
	}
	before, err := f.srv.store.ListAccountOrgs(bob.Id)
	if err != nil || len(before) != 1 {
		t.Fatalf("bob orgs before = %d, %v", len(before), err)
	}

	created := createInvite(t, f, f.client, f.orgId, "bob@example.com", "")
	resp = postJSON(t, f.ts, &http.Client{}, "/api/invite/redeem", map[string]string{
		"token":    inviteSecret(created.Link),
		"email":    "bob@example.com",
		"password": "correct-horse-battery",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("redeem existing: %d", resp.StatusCode)
	}
	after, err := f.srv.store.ListAccountOrgs(bob.Id)
	if err != nil || len(after) != 2 {
		t.Fatalf("bob orgs after = %d, %v want 2 (own + invited)", len(after), err)
	}
}

func TestInviteRedeemEmailNeedNotMatchHint(t *testing.T) {
	f := claimedHub(t, offering.Selfhost)
	created := createInvite(t, f, f.client, f.orgId, "hint@example.com", "")
	resp := postJSON(t, f.ts, &http.Client{}, "/api/invite/redeem", map[string]string{
		"token":    inviteSecret(created.Link),
		"email":    "other@example.com",
		"password": "correct-horse-battery",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("mismatch redeem: %d", resp.StatusCode)
	}
	account, err := f.srv.store.AccountByEmail("other@example.com")
	if err != nil || account == nil {
		t.Fatal("redeemer account missing")
	}
}

func TestInviteExpiredToken(t *testing.T) {
	f := claimedHub(t, offering.Selfhost)
	secret, err := auth.NewInviteToken()
	if err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-8 * 24 * time.Hour)
	if _, err := f.srv.store.CreateOrgInvite(f.orgId, "late@example.com", hashToken(secret),
		authz.RoleMember, past, past.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	resp := postJSON(t, f.ts, &http.Client{}, "/api/invite/redeem", map[string]string{
		"token":    secret,
		"email":    "late@example.com",
		"password": "correct-horse-battery",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expired: %d, want 400", resp.StatusCode)
	}
	peek := requestJSON(t, f.ts, &http.Client{}, http.MethodGet, "/api/invite?token="+url.QueryEscape(secret), nil)
	if peek.StatusCode != http.StatusBadRequest {
		t.Fatalf("peek expired: %d, want 400", peek.StatusCode)
	}
}

func TestInviteMemberCannotInvite(t *testing.T) {
	f := claimedHub(t, offering.Selfhost)
	f.addMember(t, "mem@example.com", "correct-horse-battery", authz.RoleMember)
	client := f.signIn(t, "mem@example.com", "correct-horse-battery")
	resp := requestJSON(t, f.ts, client, http.MethodPost, "/api/orgs/"+f.orgId+"/invites", map[string]string{
		"email": "new@example.com",
	})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("member invite: %d, want 404", resp.StatusCode)
	}
}

func TestInviteAdminCannotInviteOwner(t *testing.T) {
	f := claimedHub(t, offering.Selfhost)
	f.addMember(t, "adm@example.com", "correct-horse-battery", authz.RoleAdmin)
	client := f.signIn(t, "adm@example.com", "correct-horse-battery")
	resp := requestJSON(t, f.ts, client, http.MethodPost, "/api/orgs/"+f.orgId+"/invites", map[string]string{
		"email": "boss@example.com",
		"role":  "owner",
	})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("admin invites owner: %d, want 409", resp.StatusCode)
	}
}

func TestInviteRevokeAndResend(t *testing.T) {
	f := claimedHub(t, offering.Selfhost)
	first := createInvite(t, f, f.client, f.orgId, "dev@example.com", "")
	second := createInvite(t, f, f.client, f.orgId, "dev@example.com", "")
	old := postJSON(t, f.ts, &http.Client{}, "/api/invite/redeem", map[string]string{
		"token":    inviteSecret(first.Link),
		"email":    "dev@example.com",
		"password": "correct-horse-battery",
	})
	if old.StatusCode != http.StatusBadRequest {
		t.Fatalf("retired first token: %d, want 400", old.StatusCode)
	}
	if resp := postJSON(t, f.ts, &http.Client{}, "/api/invite/redeem", map[string]string{
		"token":    inviteSecret(second.Link),
		"email":    "dev@example.com",
		"password": "correct-horse-battery",
	}); resp.StatusCode != http.StatusOK {
		t.Fatalf("second token: %d", resp.StatusCode)
	}

	third := createInvite(t, f, f.client, f.orgId, "other@example.com", "")
	rev := f.do(t, http.MethodDelete, "/api/orgs/"+f.orgId+"/invites/"+third.Id, nil)
	if rev.StatusCode != http.StatusOK {
		t.Fatalf("revoke: %d", rev.StatusCode)
	}
	if resp := postJSON(t, f.ts, &http.Client{}, "/api/invite/redeem", map[string]string{
		"token":    inviteSecret(third.Link),
		"email":    "other@example.com",
		"password": "correct-horse-battery",
	}); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("revoked token: %d, want 400", resp.StatusCode)
	}
	again := f.do(t, http.MethodDelete, "/api/orgs/"+f.orgId+"/invites/"+third.Id, nil)
	if again.StatusCode != http.StatusOK {
		t.Fatalf("idempotent revoke: %d", again.StatusCode)
	}
}

func TestInviteAlreadyMember(t *testing.T) {
	f := claimedHub(t, offering.Selfhost)
	resp := f.do(t, http.MethodPost, "/api/orgs/"+f.orgId+"/invites", map[string]string{
		"email": "ops@example.com",
	})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("invite existing member: %d, want 409", resp.StatusCode)
	}

	created := createInvite(t, f, f.client, f.orgId, "soon@example.com", "")
	hash, err := auth.HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatal(err)
	}
	account, err := f.srv.store.CreateAccount("soon@example.com", hash)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.srv.store.AddOrgMember(f.orgId, account.Id, authz.RoleMember); err != nil {
		t.Fatal(err)
	}
	if resp := postJSON(t, f.ts, &http.Client{}, "/api/invite/redeem", map[string]string{
		"token":    inviteSecret(created.Link),
		"email":    "soon@example.com",
		"password": "correct-horse-battery",
	}); resp.StatusCode != http.StatusOK {
		t.Fatalf("already-member redeem: %d, want 200", resp.StatusCode)
	}
}

func TestInviteWrongPasswordAndWeakNew(t *testing.T) {
	f := claimedHub(t, offering.Selfhost)
	hash, err := auth.HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.store.CreateAccount("exists@example.com", hash); err != nil {
		t.Fatal(err)
	}
	created := createInvite(t, f, f.client, f.orgId, "exists@example.com", "")
	wrong := postJSON(t, f.ts, &http.Client{}, "/api/invite/redeem", map[string]string{
		"token":    inviteSecret(created.Link),
		"email":    "exists@example.com",
		"password": "definitely-not-that",
	})
	if wrong.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong password: %d, want 401", wrong.StatusCode)
	}

	fresh := createInvite(t, f, f.client, f.orgId, "new@example.com", "")
	weak := postJSON(t, f.ts, &http.Client{}, "/api/invite/redeem", map[string]string{
		"token":    inviteSecret(fresh.Link),
		"email":    "new@example.com",
		"password": "short",
	})
	if weak.StatusCode != http.StatusBadRequest {
		t.Fatalf("weak new password: %d, want 400", weak.StatusCode)
	}
	if resp := postJSON(t, f.ts, &http.Client{}, "/api/invite/redeem", map[string]string{
		"token":    inviteSecret(fresh.Link),
		"email":    "new@example.com",
		"password": "correct-horse-battery",
	}); resp.StatusCode != http.StatusOK {
		t.Fatalf("token after weak refusal: %d", resp.StatusCode)
	}
}

func TestInviteMailUsesInviterLocale(t *testing.T) {
	f := claimedHub(t, offering.Selfhost)
	createInvite(t, f, f.client, f.orgId, "one@example.com", "")
	en, err := f.srv.store.lastMail()
	if err != nil || en == nil || en.Kind != mailer.KindInvite {
		t.Fatalf("english mail: %v %v", en, err)
	}
	if !strings.Contains(en.Subject, "Invitation to") || strings.Contains(en.Subject, "Zaproszenie") {
		t.Fatalf("default-locale mail = %q", en.Subject)
	}
	if err := f.srv.store.SetAccountLocale(f.ownerId, auth.LocalePL); err != nil {
		t.Fatal(err)
	}
	createInvite(t, f, f.client, f.orgId, "two@example.com", "")
	pl, err := f.srv.store.lastMail()
	if err != nil || pl == nil {
		t.Fatal(err)
	}
	if !strings.Contains(pl.Subject, "Zaproszenie") {
		t.Fatalf("Polish mail = %q", pl.Subject)
	}
}

func TestInviteAnonymousCreateIsUnauthorized(t *testing.T) {
	f := claimedHub(t, offering.Selfhost)
	resp := requestJSON(t, f.ts, &http.Client{}, http.MethodPost, "/api/orgs/"+f.orgId+"/invites", map[string]string{
		"email": "x@example.com",
	})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anon create: %d, want 401", resp.StatusCode)
	}
}

type createdInvite struct {
	Id        string `json:"id"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	ExpiresAt int64  `json:"expiresAt"`
	Link      string `json:"link"`
}

func createInvite(t *testing.T, f *adminFixture, client *http.Client, orgID, email, role string) createdInvite {
	t.Helper()
	body := map[string]string{"email": email}
	if role != "" {
		body["role"] = role
	}
	resp := requestJSON(t, f.ts, client, http.MethodPost, "/api/orgs/"+orgID+"/invites", body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create invite: %d", resp.StatusCode)
	}
	var out createdInvite
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func listInvites(t *testing.T, f *adminFixture, client *http.Client, orgID string) []OrgInvite {
	t.Helper()
	resp := requestJSON(t, f.ts, client, http.MethodGet, "/api/orgs/"+orgID+"/invites", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list invites: %d", resp.StatusCode)
	}
	var out []OrgInvite
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func peekInvite(t *testing.T, f *adminFixture, secret string) InvitePreview {
	t.Helper()
	resp := requestJSON(t, f.ts, &http.Client{}, http.MethodGet, "/api/invite?token="+url.QueryEscape(secret), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("peek: %d", resp.StatusCode)
	}
	var out InvitePreview
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func inviteSecret(link string) string {
	u, err := url.Parse(link)
	if err != nil {
		return ""
	}
	return u.Query().Get("token")
}
