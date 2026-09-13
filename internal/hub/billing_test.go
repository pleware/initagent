package hub

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/pleware/initagent/internal/billing"
	"github.com/pleware/initagent/internal/offering"
	"github.com/pleware/initagent/internal/orgplan"
	"github.com/stripe/stripe-go/v82/webhook"
)

func TestSelfHostHasNoBillingRoutes(t *testing.T) {
	f := claimedHub(t, offering.Selfhost)
	resp := f.do(t, http.MethodGet, "/api/orgs/"+f.orgId+"/billing", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET billing on self-host: %d, want 404", resp.StatusCode)
	}
	resp = f.do(t, http.MethodPost, "/api/orgs/"+f.orgId+"/checkout", map[string]string{"plan": "starter"})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("checkout on self-host: %d, want 404", resp.StatusCode)
	}
}

func TestHostedBillingProfileAndUnwiredCheckout(t *testing.T) {
	f := hostedCustomer(t)
	path := "/api/orgs/" + f.orgId + "/billing"
	resp := f.do(t, http.MethodGet, path, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET billing: %d", resp.StatusCode)
	}
	var got OrgBilling
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Plan != string(orgplan.Free) || got.CheckoutReady {
		t.Fatalf("billing = %+v, want free and not ready", got)
	}

	resp = f.do(t, http.MethodPatch, path, billing.Buyer{
		Name: "ACME", TaxNo: "5252445767", Street: "Prosta 1",
		City: "Warszawa", PostCode: "00-001", Country: "PL", Email: "fin@acme.test",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH billing: %d", resp.StatusCode)
	}

	resp = f.do(t, http.MethodPost, "/api/orgs/"+f.orgId+"/checkout", map[string]string{"plan": "starter"})
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("unwired checkout: %d, want 503", resp.StatusCode)
	}
}

func TestApplyPaidPlanWritesCatalogueId(t *testing.T) {
	f := hostedCustomer(t)
	if err := f.srv.store.ApplyPaidPlan(billing.Event{
		OrgID: f.orgId, Plan: orgplan.Starter, CustomerID: "cus_1", SubscriptionID: "sub_1",
	}); err != nil {
		t.Fatal(err)
	}
	org, err := f.srv.store.OrgById(f.orgId)
	if err != nil || org == nil || org.Plan != string(orgplan.Starter) {
		t.Fatalf("org = %+v err=%v", org, err)
	}
	row, err := f.srv.store.GetOrgBilling(f.orgId)
	if err != nil || row.StripeCustomer != "cus_1" || row.StripeSubscription != "sub_1" {
		t.Fatalf("stripe ids = %+v %v", row, err)
	}
}

func TestBillingWebhookUpgradesPlan(t *testing.T) {
	f := hostedCustomer(t)
	whsec := "whsec_test"
	f.srv.billing = billing.New(billing.Config{
		Offering:      offering.Hosted,
		StripeSecret:  "sk_test",
		WebhookSecret: whsec,
		PriceStarter:  "price_x",
	})
	payload, err := json.Marshal(map[string]any{
		"id":   "evt_checkout",
		"type": billing.KindCheckout,
		"data": map[string]any{
			"object": map[string]any{
				"client_reference_id": f.orgId,
				"customer":            "cus_webhook",
				"subscription":        "sub_webhook",
				"metadata":            map[string]string{"org_id": f.orgId, "plan_id": "starter"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	signed := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{
		Payload: payload,
		Secret:  whsec,
	})
	req, err := http.NewRequest(http.MethodPost, f.ts.URL+"/api/billing/webhook", bytes.NewReader(signed.Payload))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Stripe-Signature", signed.Header)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("webhook: %d body=%s", resp.StatusCode, bodyOf(t, resp))
	}
	org, err := f.srv.store.OrgById(f.orgId)
	if err != nil || org == nil || org.Plan != string(orgplan.Starter) {
		t.Fatalf("org plan = %+v err=%v", org, err)
	}
}

func TestBillingWebhookIgnoresUnknownEvents(t *testing.T) {
	f := hostedCustomer(t)
	whsec := "whsec_ignore"
	f.srv.billing = billing.New(billing.Config{
		Offering:      offering.Hosted,
		StripeSecret:  "sk_test",
		WebhookSecret: whsec,
		PriceStarter:  "price_x",
	})
	payload, err := json.Marshal(map[string]any{
		"id":   "evt_other",
		"type": "customer.created",
		"data": map[string]any{"object": map[string]any{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	signed := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{
		Payload: payload,
		Secret:  whsec,
	})
	req, err := http.NewRequest(http.MethodPost, f.ts.URL+"/api/billing/webhook", bytes.NewReader(signed.Payload))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Stripe-Signature", signed.Header)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("ignored event: %d", resp.StatusCode)
	}
}

func TestRecordBillingInvoiceIsIdempotent(t *testing.T) {
	f := hostedCustomer(t)
	ok, err := f.srv.store.RecordBillingInvoice("in_1", f.orgId, "88")
	if err != nil || !ok {
		t.Fatalf("first = %v %v", ok, err)
	}
	ok, err = f.srv.store.RecordBillingInvoice("in_1", f.orgId, "88")
	if err != nil || ok {
		t.Fatalf("second = %v %v", ok, err)
	}
	seen, err := f.srv.store.HasBillingInvoice("in_1")
	if err != nil || !seen {
		t.Fatalf("seen = %v %v", seen, err)
	}
}
