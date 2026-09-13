package billing

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pleware/initagent/internal/orgplan"
	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/webhook"
)

func withStripeAPI(t *testing.T, h http.Handler) func() {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	prev := stripe.GetBackend(stripe.APIBackend)
	stripe.SetBackend(stripe.APIBackend, stripe.GetBackendWithConfig(stripe.APIBackend, &stripe.BackendConfig{
		HTTPClient: srv.Client(),
		URL:        stripe.String(srv.URL),
	}))
	return func() { stripe.SetBackend(stripe.APIBackend, prev) }
}

func validBuyer() Buyer {
	return Buyer{
		Name: "ACME", TaxNo: "5252445767", Street: "Prosta 1",
		City: "Warszawa", PostCode: "00-001", Email: "a@b.c",
	}
}

func TestReduceCheckoutEvent(t *testing.T) {
	t.Parallel()
	raw, _ := json.Marshal(map[string]any{
		"client_reference_id": "org-1",
		"customer":            "cus_1",
		"subscription":        "sub_1",
		"metadata":            map[string]string{"org_id": "org-1", "plan_id": "starter"},
	})
	ev, err := reduceStripeEvent(KindCheckout, "evt_1", raw)
	if err != nil {
		t.Fatal(err)
	}
	if ev.OrgID != "org-1" || ev.Plan != orgplan.Starter || ev.CustomerID != "cus_1" || ev.SubscriptionID != "sub_1" {
		t.Fatalf("event = %+v", ev)
	}
}

func TestReduceCheckoutEventExpandedCustomer(t *testing.T) {
	t.Parallel()
	raw, _ := json.Marshal(map[string]any{
		"client_reference_id": "org-1",
		"customer":            map[string]string{"id": "cus_obj"},
		"subscription":        map[string]string{"id": "sub_obj"},
		"metadata":            map[string]string{"org_id": "org-1", "plan_id": "starter"},
	})
	ev, err := reduceStripeEvent(KindCheckout, "evt_obj", raw)
	if err != nil {
		t.Fatal(err)
	}
	if ev.CustomerID != "cus_obj" || ev.SubscriptionID != "sub_obj" {
		t.Fatalf("event = %+v", ev)
	}
}

func TestReduceInvoicePaidFromParent(t *testing.T) {
	t.Parallel()
	raw, _ := json.Marshal(map[string]any{
		"id":          "in_parent",
		"customer":    "cus_9",
		"amount_paid": 1000,
		"currency":    "usd",
		"parent": map[string]any{
			"subscription_details": map[string]any{
				"subscription": "sub_9",
				"metadata":     map[string]string{"org_id": "org-9", "plan_id": "starter"},
			},
		},
	})
	ev, err := reduceStripeEvent(KindInvoicePaid, "evt_p", raw)
	if err != nil {
		t.Fatal(err)
	}
	if ev.OrgID != "org-9" || ev.Plan != orgplan.Starter || ev.SubscriptionID != "sub_9" || ev.InvoiceID != "in_parent" {
		t.Fatalf("event = %+v", ev)
	}
}

func TestReduceInvoicePaidEvent(t *testing.T) {
	t.Parallel()
	raw, _ := json.Marshal(map[string]any{
		"id":          "in_1",
		"customer":    "cus_1",
		"amount_paid": 500,
		"currency":    "usd",
		"subscription_details": map[string]any{
			"metadata": map[string]string{"org_id": "org-9", "plan_id": "team"},
		},
	})
	ev, err := reduceStripeEvent(KindInvoicePaid, "evt_2", raw)
	if err != nil {
		t.Fatal(err)
	}
	if ev.OrgID != "org-9" || ev.Plan != orgplan.Team || ev.InvoiceID != "in_1" || ev.AmountCents != 500 {
		t.Fatalf("event = %+v", ev)
	}
}

func TestReduceEventNeedsOrgAndPlan(t *testing.T) {
	t.Parallel()
	raw, _ := json.Marshal(map[string]any{
		"metadata": map[string]string{"plan_id": "starter"},
	})
	if _, err := reduceStripeEvent(KindCheckout, "evt", raw); err == nil {
		t.Fatal("missing org_id must fail")
	}
}

func TestReduceEventRejectsUnknownPlan(t *testing.T) {
	t.Parallel()
	raw, _ := json.Marshal(map[string]any{
		"metadata": map[string]string{"org_id": "org-1", "plan_id": "hobby"},
	})
	if _, err := reduceStripeEvent(KindCheckout, "evt", raw); err == nil {
		t.Fatal("unknown plan must fail")
	}
	if _, err := reduceStripeEvent("customer.created", "evt", raw); err == nil {
		t.Fatal("other kinds must fail")
	}
}

func TestStripeParseSignedCheckout(t *testing.T) {
	t.Parallel()
	payload, err := json.Marshal(map[string]any{
		"id":   "evt_signed",
		"type": KindCheckout,
		"data": map[string]any{
			"object": map[string]any{
				"client_reference_id": "org-1",
				"customer":            "cus_1",
				"subscription":        "sub_1",
				"metadata":            map[string]string{"org_id": "org-1", "plan_id": "starter"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	signed := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{
		Payload: payload,
		Secret:  "whsec_test",
	})
	ev, err := stripeParse(signed.Payload, signed.Header, "whsec_test")
	if err != nil {
		t.Fatal(err)
	}
	if ev.OrgID != "org-1" || ev.Plan != orgplan.Starter {
		t.Fatalf("event = %+v", ev)
	}
	if _, err := stripeParse(payload, "bad", "whsec_test"); err == nil {
		t.Fatal("bad signature must fail")
	}
}

func TestAmountLabel(t *testing.T) {
	t.Parallel()
	if got := amountLabel(500, "usd"); got != "500 USD" {
		t.Fatalf("label = %q", got)
	}
}

func TestStripeStartCreatesSession(t *testing.T) {
	reset := withStripeAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/v1/checkout/sessions") {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"cs_test","url":"https://checkout.stripe.test/s"}`))
	}))
	defer reset()

	start := stripeStart("sk_test")
	url, err := start(context.Background(), CheckoutRequest{
		OrgID: "org-1", Plan: orgplan.Starter, Quantity: 2,
		SuccessURL: "https://hub/s", CancelURL: "https://hub/c",
		CustomerID: "cus_existing",
		Buyer:      validBuyer(),
	}, "price_1")
	if err != nil || url != "https://checkout.stripe.test/s" {
		t.Fatalf("url = %q err = %v", url, err)
	}
}

func TestStripeStartUsesCustomerEmail(t *testing.T) {
	reset := withStripeAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"cs_test","url":"https://checkout.stripe.test/s"}`))
	}))
	defer reset()

	start := stripeStart("sk_test")
	if _, err := start(context.Background(), CheckoutRequest{
		OrgID: "org-1", Plan: orgplan.Starter,
		SuccessURL: "https://hub/s", CancelURL: "https://hub/c",
		Buyer: validBuyer(),
	}, "price_1"); err != nil {
		t.Fatal(err)
	}
}

func TestStripeStartEmptyURLFails(t *testing.T) {
	reset := withStripeAPI(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"cs_test","url":""}`))
	}))
	defer reset()

	start := stripeStart("sk_test")
	if _, err := start(context.Background(), CheckoutRequest{
		OrgID: "org-1", Plan: orgplan.Starter,
		SuccessURL: "https://hub/s", CancelURL: "https://hub/c",
		Buyer: validBuyer(),
	}, "price_1"); err == nil {
		t.Fatal("empty session URL must fail")
	}
}

func TestStripeSetQuantityUpdates(t *testing.T) {
	reset := withStripeAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/subscriptions/sub_1"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"sub_1","items":{"data":[{"id":"si_1"}]}}`))
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/subscriptions/sub_1"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"sub_1"}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer reset()

	setQty := stripeSetQuantity("sk_test")
	if err := setQty(context.Background(), "sub_1", 4); err != nil {
		t.Fatal(err)
	}
}

func TestStripeSetQuantityNoItemsFails(t *testing.T) {
	reset := withStripeAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"sub_1","items":{"data":[]}}`))
	}))
	defer reset()

	setQty := stripeSetQuantity("sk_test")
	if err := setQty(context.Background(), "sub_1", 2); err == nil {
		t.Fatal("subscription without items must fail")
	}
}

func TestExpandableIDUnmarshal(t *testing.T) {
	t.Parallel()
	var id expandableID
	if err := id.UnmarshalJSON([]byte("null")); err != nil || id != "" {
		t.Fatalf("null = %q %v", id, err)
	}
	if err := id.UnmarshalJSON([]byte(`"cus_plain"`)); err != nil || id != "cus_plain" {
		t.Fatalf("string = %q %v", id, err)
	}
	if err := id.UnmarshalJSON([]byte(`{"id":"cus_obj"}`)); err != nil || id != "cus_obj" {
		t.Fatalf("object = %q %v", id, err)
	}
	if err := id.UnmarshalJSON([]byte(`{`)); err == nil {
		t.Fatal("bad JSON must fail")
	}
}

func TestReduceInvoicePaidMergesLineMetadata(t *testing.T) {
	t.Parallel()
	raw, _ := json.Marshal(map[string]any{
		"id":          "in_lines",
		"customer":    "cus_1",
		"amount_paid": 900,
		"currency":    "pln",
		"subscription_details": map[string]any{
			"metadata": map[string]string{"plan_id": "starter"},
		},
		"lines": map[string]any{
			"data": []map[string]any{{
				"metadata": map[string]string{"org_id": "org-lines"},
			}},
		},
	})
	ev, err := reduceStripeEvent(KindInvoicePaid, "evt_lines", raw)
	if err != nil {
		t.Fatal(err)
	}
	if ev.OrgID != "org-lines" || ev.Plan != orgplan.Starter || ev.Currency != "PLN" {
		t.Fatalf("event = %+v", ev)
	}
}
