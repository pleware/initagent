package billing

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/pleware/initagent/internal/offering"
	"github.com/pleware/initagent/internal/orgplan"
	"github.com/stripe/stripe-go/v82/webhook"
)

func TestNewSelfHostNeverReady(t *testing.T) {
	t.Parallel()
	s := New(Config{Offering: offering.Selfhost, StripeSecret: "sk_test"})
	if s.Ready() || s.Hosted() || s.InvoicesReady() {
		t.Fatal("self-host must not expose billing")
	}
	_, err := s.StartCheckout(t.Context(), CheckoutRequest{Plan: orgplan.Starter})
	if !errors.Is(err, ErrNotHosted) {
		t.Fatalf("checkout = %v", err)
	}
	if _, err := s.ParseWebhook(nil, ""); !errors.Is(err, ErrNotHosted) {
		t.Fatalf("webhook = %v", err)
	}
}

func TestNewHostedWithoutKeys(t *testing.T) {
	t.Parallel()
	s := New(Config{Offering: offering.Hosted})
	if !s.Hosted() || s.Ready() {
		t.Fatal("hosted without Stripe is listed, not ready")
	}
	_, err := s.StartCheckout(t.Context(), CheckoutRequest{Plan: orgplan.Starter})
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("checkout = %v", err)
	}
}

func TestStartCheckoutUsesOverridePrice(t *testing.T) {
	t.Parallel()
	var sawPrice string
	s := New(Config{Offering: offering.Hosted, PriceStarter: "price_override"})
	s.start = func(_ context.Context, req CheckoutRequest, price string) (string, error) {
		sawPrice = price
		if req.Quantity != 1 {
			t.Fatalf("quantity = %d", req.Quantity)
		}
		return "https://checkout.test/s", nil
	}
	got, err := s.StartCheckout(t.Context(), CheckoutRequest{
		OrgID: "org-1", Plan: orgplan.Starter,
		Buyer: Buyer{
			Name: "ACME", TaxNo: "5252445767", Street: "Prosta 1",
			City: "Warszawa", PostCode: "00-001", Email: "a@b.c",
		},
	})
	if err != nil || got.URL != "https://checkout.test/s" {
		t.Fatalf("got %+v %v", got, err)
	}
	if sawPrice != "price_override" {
		t.Fatalf("price = %q", sawPrice)
	}
}

func TestStartCheckoutRefusesFreeAndIncompleteBuyer(t *testing.T) {
	t.Parallel()
	s := New(Config{Offering: offering.Hosted, PriceStarter: "price_x"})
	s.start = func(context.Context, CheckoutRequest, string) (string, error) {
		t.Fatal("must not start")
		return "", nil
	}
	_, err := s.StartCheckout(t.Context(), CheckoutRequest{
		Plan: orgplan.Free,
		Buyer: Buyer{
			Name: "ACME", TaxNo: "5252445767", Street: "Prosta 1",
			City: "Warszawa", PostCode: "00-001", Email: "a@b.c",
		},
	})
	if !errors.Is(err, ErrUnknownPlan) {
		t.Fatalf("free = %v", err)
	}
	_, err = s.StartCheckout(t.Context(), CheckoutRequest{Plan: orgplan.Starter})
	if !errors.Is(err, ErrBuyer) {
		t.Fatalf("buyer = %v", err)
	}
}

func TestIssueInvoiceRequiresReadyBuyer(t *testing.T) {
	t.Parallel()
	s := New(Config{Offering: offering.Hosted})
	s.issue = func(context.Context, Invoice) (string, error) { return "1", nil }
	_, err := s.IssueInvoice(t.Context(), Invoice{})
	if !errors.Is(err, ErrBuyer) {
		t.Fatalf("empty = %v", err)
	}
	id, err := s.IssueInvoice(t.Context(), Invoice{
		Idempotency: "in_1",
		Buyer: Buyer{
			Name: "ACME", TaxNo: "5252445767", Street: "Prosta 1",
			City: "Warszawa", PostCode: "00-001", Email: "a@b.c",
		},
	})
	if err != nil || id != "1" {
		t.Fatalf("issue = %q %v", id, err)
	}
}

func TestParseWebhookNeedsSecret(t *testing.T) {
	t.Parallel()
	s := New(Config{Offering: offering.Hosted, StripeSecret: "sk_test"})
	if _, err := s.ParseWebhook([]byte(`{}`), "sig"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("parse = %v", err)
	}
}

func TestIssueInvoiceNeedsHostedReadyIdempotency(t *testing.T) {
	t.Parallel()
	self := New(Config{Offering: offering.Selfhost})
	if _, err := self.IssueInvoice(t.Context(), Invoice{Idempotency: "in"}); !errors.Is(err, ErrNotHosted) {
		t.Fatalf("self-host = %v", err)
	}
	hosted := New(Config{Offering: offering.Hosted})
	if _, err := hosted.IssueInvoice(t.Context(), Invoice{
		Idempotency: "in",
		Buyer: Buyer{
			Name: "ACME", TaxNo: "5252445767", Street: "Prosta 1",
			City: "Warszawa", PostCode: "00-001", Email: "a@b.c",
		},
	}); err == nil {
		t.Fatal("unwired Fakturownia must fail")
	}
	hosted.issue = func(context.Context, Invoice) (string, error) { return "1", nil }
	if _, err := hosted.IssueInvoice(t.Context(), Invoice{
		Buyer: Buyer{
			Name: "ACME", TaxNo: "5252445767", Street: "Prosta 1",
			City: "Warszawa", PostCode: "00-001", Email: "a@b.c",
		},
	}); err == nil {
		t.Fatal("missing idempotency must fail")
	}
}

func TestReadyNeedsAPrice(t *testing.T) {
	t.Parallel()
	s := New(Config{
		Offering: offering.Hosted, StripeSecret: "sk_test",
		WebhookSecret: "whsec", PriceStarter: "price_starter",
	})
	if !s.Ready() || !s.Hosted() || s.InvoicesReady() {
		t.Fatal("secret + price is checkout-ready, invoices still unwired")
	}
}

func TestSetQuantityHosted(t *testing.T) {
	t.Parallel()
	var saw string
	var n int
	s := New(Config{Offering: offering.Hosted, StripeSecret: "sk_test", PriceStarter: "price_x"})
	s.setQty = func(_ context.Context, id string, qty int) error {
		saw, n = id, qty
		return nil
	}
	if err := s.SetQuantity(t.Context(), "  sub_1  ", 0); err != nil {
		t.Fatal(err)
	}
	if saw != "sub_1" || n != 1 {
		t.Fatalf("qty = %q %d", saw, n)
	}
	if err := s.SetQuantity(t.Context(), "", 3); err != nil {
		t.Fatal(err)
	}
}

func TestSetQuantityNoopsWhenUnwired(t *testing.T) {
	t.Parallel()
	s := New(Config{Offering: offering.Selfhost})
	if err := s.SetQuantity(t.Context(), "sub_1", 2); err != nil {
		t.Fatal(err)
	}
}

func TestHostedPricesPrefersEnv(t *testing.T) {
	t.Parallel()
	got := hostedPrices(Config{
		PriceStarter: " price_env ",
		PriceTeam:    "price_team_env",
	})
	if got[orgplan.Starter] != "price_env" || got[orgplan.Team] != "price_team_env" {
		t.Fatalf("prices = %+v", got)
	}
}

func TestNewHostedWiresInvoices(t *testing.T) {
	t.Parallel()
	s := New(Config{
		Offering:          offering.Hosted,
		FakturowniaToken:  "tok",
		FakturowniaDomain: "acme.fakturownia.pl",
	})
	if !s.Hosted() || s.Ready() || !s.InvoicesReady() {
		t.Fatalf("hosted invoices = ready %v checkout %v", s.InvoicesReady(), s.Ready())
	}
}

func TestParseWebhookWithSecret(t *testing.T) {
	t.Parallel()
	payload, err := json.Marshal(map[string]any{
		"id":   "evt_wh",
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
		Secret:  "whsec_live",
	})
	s := New(Config{
		Offering: offering.Hosted, StripeSecret: "sk_test",
		WebhookSecret: "whsec_live", PriceStarter: "price_x",
	})
	ev, err := s.ParseWebhook(signed.Payload, signed.Header)
	if err != nil || ev.OrgID != "org-1" || ev.Plan != orgplan.Starter {
		t.Fatalf("event = %+v err = %v", ev, err)
	}
}

func TestStartCheckoutPropagatesStartError(t *testing.T) {
	t.Parallel()
	s := New(Config{Offering: offering.Hosted, PriceStarter: "price_x"})
	s.start = func(context.Context, CheckoutRequest, string) (string, error) {
		return "", errors.New("stripe down")
	}
	_, err := s.StartCheckout(t.Context(), CheckoutRequest{
		Plan: orgplan.Starter,
		Buyer: Buyer{
			Name: "ACME", TaxNo: "5252445767", Street: "Prosta 1",
			City: "Warszawa", PostCode: "00-001", Email: "a@b.c",
		},
	})
	if err == nil || err.Error() != "stripe down" {
		t.Fatalf("err = %v", err)
	}
}

func TestStartCheckoutNeedsAPrice(t *testing.T) {
	t.Parallel()
	if orgplan.CheckoutPrice(orgplan.Starter) != "" {
		t.Skip("catalogue already has a Price id")
	}
	s := New(Config{Offering: offering.Hosted})
	s.start = func(context.Context, CheckoutRequest, string) (string, error) {
		t.Fatal("must not start without a Price")
		return "", nil
	}
	_, err := s.StartCheckout(t.Context(), CheckoutRequest{
		Plan: orgplan.Starter,
		Buyer: Buyer{
			Name: "ACME", TaxNo: "5252445767", Street: "Prosta 1",
			City: "Warszawa", PostCode: "00-001", Email: "a@b.c",
		},
	})
	if !errors.Is(err, ErrNoPrice) {
		t.Fatalf("empty price: %v", err)
	}
}
