package hub

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/pleware/initagent/internal/authz"
	"github.com/pleware/initagent/internal/billing"
	"github.com/pleware/initagent/internal/offering"
	"github.com/pleware/initagent/internal/orgplan"
)

func (s *Server) hostedPlansOnly(w http.ResponseWriter) bool {
	if s.opts.Offering == offering.Hosted && s.billing != nil && s.billing.Hosted() {
		return false
	}
	httpError(w, http.StatusNotFound, billing.ErrNotHosted.Error())
	return true
}

func (s *Server) handleGetBilling(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if s.hostedPlansOnly(w) {
		return
	}
	orgID := r.PathValue("id")
	if !cred.Can(authz.ReadOrg, orgID, "") {
		hideOrRefuse(w, cred, authz.ReadOrg, "no such organization", bound{org: orgID})
		return
	}
	row, err := s.store.GetOrgBilling(orgID)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if row == nil {
		httpError(w, http.StatusNotFound, "no such organization")
		return
	}
	row.CheckoutReady = s.billing.Ready()
	row.InvoicesReady = s.billing.InvoicesReady()
	writeJSON(w, row)
}

func (s *Server) handlePatchBilling(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if s.hostedPlansOnly(w) {
		return
	}
	orgID := r.PathValue("id")
	if !cred.Can(authz.AdminOrg, orgID, "") {
		hideOrRefuse(w, cred, authz.AdminOrg, "no such organization", bound{org: orgID})
		return
	}
	var req billing.Buyer
	if err := readJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, "bad request")
		return
	}
	if err := s.store.SaveOrgBilling(orgID, req); err != nil {
		if errors.Is(err, billing.ErrBuyer) {
			httpError(w, http.StatusBadRequest, err.Error())
			return
		}
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.handleGetBilling(w, r, cred)
}

func (s *Server) handleCheckout(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if s.hostedPlansOnly(w) {
		return
	}
	orgID := r.PathValue("id")
	if !cred.Can(authz.AdminOrg, orgID, "") {
		hideOrRefuse(w, cred, authz.AdminOrg, "no such organization", bound{org: orgID})
		return
	}
	var req struct {
		Plan string `json:"plan"`
	}
	if err := readJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, "bad request")
		return
	}
	plan, err := orgplan.Parse(req.Plan)
	if err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	row, err := s.store.GetOrgBilling(orgID)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if row == nil {
		httpError(w, http.StatusNotFound, "no such organization")
		return
	}
	buyer := billing.Buyer{
		Name: row.Name, TaxNo: row.TaxNo, Street: row.Street,
		City: row.City, PostCode: row.PostCode, Country: row.Country, Email: row.Email,
	}
	if buyer.Email == "" && cred.Actor.Account != "" {
		if acc, err := s.store.AccountById(cred.Actor.Account); err == nil && acc != nil {
			buyer.Email = acc.Email
		}
	}
	origin, err := publicOrigin(r, s.opts.TLSDomain)
	if err != nil {
		httpError(w, http.StatusBadRequest, "cannot build checkout return URL")
		return
	}
	qty := row.People
	if qty < 1 {
		qty = 1
	}
	sess, err := s.billing.StartCheckout(r.Context(), billing.CheckoutRequest{
		OrgID:      orgID,
		Plan:       plan,
		Quantity:   qty,
		SuccessURL: origin + "/plans?paid=1",
		CancelURL:  origin + "/plans?canceled=1",
		CustomerID: row.StripeCustomer,
		Buyer:      buyer,
	})
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, billing.ErrNotConfigured) || errors.Is(err, billing.ErrNoPrice) {
			status = http.StatusServiceUnavailable
		}
		httpError(w, status, err.Error())
		return
	}
	writeJSON(w, sess)
}

func (s *Server) handleBillingWebhook(w http.ResponseWriter, r *http.Request) {
	if s.hostedPlansOnly(w) {
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		httpError(w, http.StatusBadRequest, "bad request")
		return
	}
	ev, err := s.billing.ParseWebhook(body, r.Header.Get("Stripe-Signature"))
	if err != nil {
		if errors.Is(err, billing.ErrIgnored) {
			w.WriteHeader(http.StatusOK)
			return
		}
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.store.ApplyPaidPlan(ev); err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if ev.Kind == billing.KindInvoicePaid && ev.InvoiceID != "" && ev.AmountCents > 0 {
		if err := s.issueFiscalInvoice(r, ev); err != nil {
			httpError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func (s *Server) issueFiscalInvoice(r *http.Request, ev billing.Event) error {
	if !s.billing.InvoicesReady() {
		return nil
	}
	seen, err := s.store.HasBillingInvoice(ev.InvoiceID)
	if err != nil || seen {
		return err
	}
	row, err := s.store.GetOrgBilling(ev.OrgID)
	if err != nil || row == nil {
		return err
	}
	buyer := billing.Buyer{
		Name: row.Name, TaxNo: row.TaxNo, Street: row.Street,
		City: row.City, PostCode: row.PostCode, Country: row.Country, Email: row.Email,
	}
	if err := buyer.Validate(); err != nil {
		return nil
	}
	id, err := s.billing.IssueInvoice(r.Context(), billing.Invoice{
		OrgID:       ev.OrgID,
		Plan:        ev.Plan,
		Quantity:    row.People,
		AmountCents: ev.AmountCents,
		Currency:    ev.Currency,
		Idempotency: ev.InvoiceID,
		Buyer:       buyer,
	})
	if err != nil {
		return err
	}
	_, err = s.store.RecordBillingInvoice(ev.InvoiceID, ev.OrgID, id)
	return err
}

func (s *Server) syncBillingQuantity(orgID string) {
	if s.billing == nil || !s.billing.Ready() {
		return
	}
	row, err := s.store.GetOrgBilling(orgID)
	if err != nil || row == nil || row.StripeSubscription == "" {
		return
	}
	n := row.People
	if n < 1 {
		n = 1
	}
	_ = s.billing.SetQuantity(context.Background(), row.StripeSubscription, n)
}
