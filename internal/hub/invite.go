package hub

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/pleware/initagent/internal/auth"
	"github.com/pleware/initagent/internal/authz"
	"github.com/pleware/initagent/internal/funnel"
	"github.com/pleware/initagent/internal/mailer"
)

// handleInvitePeek returns the public facts a live secret may see.
// Missing, used, or expired tokens share one 400 so the form cannot
// tell those states apart.
func (s *Server) handleInvitePeek(w http.ResponseWriter, r *http.Request) {
	if !s.loginRL.allow(s.clientAddr(r)) {
		httpError(w, http.StatusTooManyRequests, "too many attempts, try again in a minute")
		return
	}
	secret := strings.TrimSpace(r.URL.Query().Get("token"))
	if secret == "" {
		httpError(w, http.StatusBadRequest, auth.ErrInviteToken.Error())
		return
	}
	preview, err := s.store.PeekInvite(hashToken(secret), time.Now())
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if preview == nil {
		httpError(w, http.StatusBadRequest, auth.ErrInviteToken.Error())
		return
	}
	writeJSON(w, preview)
}

// handleInviteRedeem joins the holder of a live secret to the org.
//
// A new address mints an `acc-` without a second org (`08`). An existing
// address attaches membership; the password must match that account and
// is not re-floored. The email on the invite is a hint, not a credential:
// redeem body's email is who joins.
func (s *Server) handleInviteRedeem(w http.ResponseWriter, r *http.Request) {
	if !s.loginRL.allow(s.clientAddr(r)) {
		httpError(w, http.StatusTooManyRequests, "too many attempts, try again in a minute")
		return
	}
	var req struct {
		Token    string `json:"token"`
		Password string `json:"password"`
		Email    string `json:"email"`
		Locale   string `json:"locale"`
	}
	if err := readJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, "bad request")
		return
	}
	secret := strings.TrimSpace(req.Token)
	if secret == "" {
		httpError(w, http.StatusBadRequest, auth.ErrInviteToken.Error())
		return
	}
	email, err := auth.NormalizeEmail(req.Email)
	if err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	now := time.Now()
	tokenHash := hashToken(secret)
	preview, err := s.store.PeekInvite(tokenHash, now)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if preview == nil {
		httpError(w, http.StatusBadRequest, auth.ErrInviteToken.Error())
		return
	}

	existing, err := s.store.AccountByEmail(email)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	existingID := ""
	passwordHash := ""
	if existing != nil {
		if !existing.VerifyPassword(req.Password) {
			httpError(w, http.StatusUnauthorized, "wrong email or password")
			return
		}
		existingID = existing.Id
	} else {
		if err := auth.CheckPassword(s.opts.Offering, email, req.Password); err != nil {
			httpError(w, http.StatusBadRequest, err.Error())
			return
		}
		passwordHash, err = auth.HashPassword(req.Password)
		if err != nil {
			httpError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	account, err := s.store.AcceptOrgInvite(tokenHash, email, passwordHash, req.Locale, existingID, now)
	if err != nil {
		if s.reportPlanLimit(w, err, preview.OrgId, existingID, "") {
			return
		}
		if errors.Is(err, auth.ErrInviteToken) {
			httpError(w, http.StatusBadRequest, err.Error())
			return
		}
		if errors.Is(err, auth.ErrEmailTaken) || errors.Is(err, auth.ErrLocale) {
			httpError(w, http.StatusBadRequest, err.Error())
			return
		}
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if existingID == "" {
		s.recordEvent(funnel.Event{
			Kind:      funnel.KindInviteRedeem,
			OrgID:     preview.OrgId,
			AccountID: account.Id,
		})
	}
	s.issueSession(w, r, account.Id)
	writeJSON(w, map[string]bool{"ok": true})
}

// handleCreateOrgInvite mints a one-time secret. The response always
// includes the link so OSS can copy it; hosted also queues mail.
func (s *Server) handleCreateOrgInvite(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	orgID := r.PathValue("id")
	if cred.Scoped() && !cred.Can(authz.AdminOrg, orgID, "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	if !cred.Can(authz.AdminOrg, orgID, "") {
		hideOrRefuse(w, cred, authz.AdminOrg, "no such organization", bound{org: orgID})
		return
	}
	var req struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := readJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, "bad request")
		return
	}
	email, err := auth.NormalizeEmail(req.Email)
	if err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	roleName := strings.TrimSpace(req.Role)
	if roleName == "" {
		roleName = string(authz.RoleMember)
	}
	role, err := authz.ParseRole(roleName)
	if err != nil {
		forbid(w, err)
		return
	}
	roster, err := s.store.OrgRoster(orgID)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := authz.AuthorizeInvite(cred.Actor, roster, role); err != nil {
		forbid(w, err)
		return
	}
	secret, err := auth.NewInviteToken()
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	now := time.Now()
	inv, err := s.store.CreateOrgInvite(orgID, email, hashToken(secret), role, now, now.Add(auth.InviteTTL))
	if err != nil {
		if s.reportPlanLimit(w, err, orgID, cred.Actor.Account, "") {
			return
		}
		if errors.Is(err, auth.ErrAlreadyMember) {
			httpError(w, http.StatusConflict, err.Error())
			return
		}
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	origin, err := publicOrigin(r, s.opts.TLSDomain)
	if err != nil {
		httpError(w, http.StatusBadRequest, "cannot build invite link")
		return
	}
	link := origin + "/invite?token=" + url.QueryEscape(secret)
	org, err := s.store.OrgById(orgID)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	orgName := orgID
	if org != nil {
		orgName = org.Name
	}
	locale := auth.LocaleEN
	if cred.Actor.Account != "" {
		if inviter, err := s.store.AccountById(cred.Actor.Account); err == nil && inviter != nil {
			locale = inviter.Locale
		}
	}
	subject, text, htmlBody := mailer.Invite(orgName, link, locale)
	if _, err := s.EnqueueMail(mailer.KindInvite, email, subject, text, htmlBody); err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]any{
		"id":        inv.Id,
		"email":     inv.Email,
		"role":      inv.Role,
		"expiresAt": inv.ExpiresAt,
		"link":      link,
	})
}

func (s *Server) handleListOrgInvites(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	orgID := r.PathValue("id")
	if !cred.Can(authz.AdminOrg, orgID, "") {
		hideOrRefuse(w, cred, authz.AdminOrg, "no such organization", bound{org: orgID})
		return
	}
	invites, err := s.store.ListOrgInvites(orgID, time.Now())
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, invites)
}

func (s *Server) handleRevokeOrgInvite(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	orgID := r.PathValue("id")
	if !cred.Can(authz.AdminOrg, orgID, "") {
		hideOrRefuse(w, cred, authz.AdminOrg, "no such organization", bound{org: orgID})
		return
	}
	if _, err := s.store.RevokeOrgInvite(orgID, r.PathValue("inviteId"), time.Now()); err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}
