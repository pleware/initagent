package billing

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pleware/initagent/internal/orgplan"
)

func TestFakturowniaPayloadSendsKSeFForPLCompany(t *testing.T) {
	t.Parallel()
	got := fakturowniaPayload("tok", Invoice{
		Plan:        orgplan.Starter,
		Quantity:    2,
		AmountCents: 1230,
		Currency:    "usd",
		Idempotency: "in_1",
		Buyer: Buyer{
			Name: "ACME", TaxNo: "525-244-57-67", Street: "Prosta 1",
			City: "Warszawa", PostCode: "00-001", Country: "PL", Email: "a@b.c",
		},
	})
	if !got.GovSaveAndSend || !got.Invoice.BuyerCompany || got.Invoice.BuyerTaxNo != "5252445767" {
		t.Fatalf("KSeF flags = %+v", got)
	}
	if got.Invoice.Positions[0].Tax != 23 || got.Invoice.Positions[0].TotalPriceGross != 12.3 {
		t.Fatalf("position = %+v", got.Invoice.Positions[0])
	}
	if got.Invoice.Oid != "in_1" || got.Invoice.Status != "paid" {
		t.Fatalf("invoice = %+v", got.Invoice)
	}
}

func TestFakturowniaPayloadForeignSkipsKSeF(t *testing.T) {
	t.Parallel()
	got := fakturowniaPayload("tok", Invoice{
		Description: "custom",
		AmountCents: 0,
		Buyer: Buyer{
			Name: "ACME", Street: "1 Road", City: "Berlin",
			PostCode: "10115", Country: "DE", Email: "a@b.c",
		},
	})
	if got.GovSaveAndSend || got.Invoice.BuyerCompany || got.Invoice.Positions[0].Tax != 0 {
		t.Fatalf("foreign = %+v", got)
	}
	if got.Invoice.Positions[0].Name != "custom" || got.Invoice.Currency != "USD" {
		t.Fatalf("defaults = %+v", got.Invoice)
	}
}

func TestFakturowniaIssue(t *testing.T) {
	t.Parallel()
	var sawToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/invoices.json" {
			t.Errorf("path = %s", r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		sawToken = string(raw)
		w.Write([]byte(`{"id": 99}`))
	}))
	t.Cleanup(srv.Close)
	issue := fakturowniaIssue(srv.URL, "secret", srv.Client())
	id, err := issue(t.Context(), Invoice{
		Plan: orgplan.Team, AmountCents: 500, Idempotency: "in_2",
		Buyer: Buyer{
			Name: "ACME", TaxNo: "5252445767", Street: "Prosta 1",
			City: "Warszawa", PostCode: "00-001", Email: "a@b.c",
		},
	})
	if err != nil || id != "99" {
		t.Fatalf("issue = %q %v", id, err)
	}
	if !strings.Contains(sawToken, "secret") || !strings.Contains(sawToken, "gov_save_and_send") {
		t.Fatalf("body = %s", sawToken)
	}
}

func TestFakturowniaIssueBareDomain(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"id": 7}`))
	}))
	t.Cleanup(srv.Close)
	host := strings.TrimPrefix(srv.URL, "https://")
	issue := fakturowniaIssue(host+"/", "tok", srv.Client())
	id, err := issue(t.Context(), Invoice{
		Plan: orgplan.Starter, AmountCents: 100, Idempotency: "in_bare",
		Buyer: Buyer{
			Name: "ACME", TaxNo: "5252445767", Street: "Prosta 1",
			City: "Warszawa", PostCode: "00-001", Email: "a@b.c",
		},
	})
	if err != nil || id != "7" {
		t.Fatalf("issue = %q %v", id, err)
	}
}

func TestFakturowniaIssueError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`{"error":"buyer_tax_no is invalid"}`))
	}))
	t.Cleanup(srv.Close)
	_, err := fakturowniaIssue(srv.URL, "t", srv.Client())(t.Context(), Invoice{
		Idempotency: "x",
		Buyer: Buyer{
			Name: "ACME", TaxNo: "5252445767", Street: "Prosta 1",
			City: "Warszawa", PostCode: "00-001", Email: "a@b.c",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "buyer_tax_no") {
		t.Fatalf("err = %v", err)
	}
}
