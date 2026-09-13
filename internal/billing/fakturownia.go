package billing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/pleware/initagent/internal/orgplan"
)

type fakturowniaCreate struct {
	APIToken       string             `json:"api_token"`
	GovSaveAndSend bool               `json:"gov_save_and_send,omitempty"`
	Invoice        fakturowniaInvoice `json:"invoice"`
}

type fakturowniaInvoice struct {
	Kind          string                `json:"kind"`
	Status        string                `json:"status"`
	SellDate      string                `json:"sell_date"`
	IssueDate     string                `json:"issue_date"`
	PaymentType   string                `json:"payment_type"`
	Currency      string                `json:"currency,omitempty"`
	Oid           string                `json:"oid,omitempty"`
	BuyerName     string                `json:"buyer_name"`
	BuyerTaxNo    string                `json:"buyer_tax_no,omitempty"`
	BuyerStreet   string                `json:"buyer_street"`
	BuyerPostCode string                `json:"buyer_post_code"`
	BuyerCity     string                `json:"buyer_city"`
	BuyerCountry  string                `json:"buyer_country"`
	BuyerEmail    string                `json:"buyer_email"`
	BuyerCompany  bool                  `json:"buyer_company"`
	Positions     []fakturowniaPosition `json:"positions"`
}

type fakturowniaPosition struct {
	Name            string  `json:"name"`
	Tax             int     `json:"tax"`
	Quantity        int     `json:"quantity"`
	TotalPriceGross float64 `json:"total_price_gross"`
}

type fakturowniaCreated struct {
	ID    int64  `json:"id"`
	Error string `json:"error"`
	Code  string `json:"code"`
}

func fakturowniaIssue(domain, token string, client *http.Client) func(context.Context, Invoice) (string, error) {
	domain = strings.TrimSuffix(strings.TrimSpace(domain), "/")
	if !strings.Contains(domain, "://") {
		domain = "https://" + domain
	}
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	return func(ctx context.Context, inv Invoice) (string, error) {
		body, err := json.Marshal(fakturowniaPayload(token, inv))
		if err != nil {
			return "", err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, domain+"/invoices.json", bytes.NewReader(body))
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		var created fakturowniaCreated
		_ = json.Unmarshal(raw, &created)
		if resp.StatusCode >= 300 || created.ID == 0 {
			msg := strings.TrimSpace(created.Error)
			if msg == "" {
				msg = strings.TrimSpace(string(raw))
			}
			if msg == "" {
				msg = resp.Status
			}
			return "", fmt.Errorf("fakturownia: %s", msg)
		}
		return fmt.Sprintf("%d", created.ID), nil
	}
}

func fakturowniaPayload(token string, inv Invoice) fakturowniaCreate {
	qty := inv.Quantity
	if qty < 1 {
		qty = 1
	}
	gross := float64(inv.AmountCents) / 100
	if inv.AmountCents <= 0 {
		gross = 0
	}
	day := time.Now().UTC().Format("2006-01-02")
	tax := 0
	if inv.Buyer.NormCountry() == "PL" {
		tax = 23
	}
	currency := strings.ToUpper(strings.TrimSpace(inv.Currency))
	if currency == "" {
		currency = "USD"
	}
	plan := string(inv.Plan)
	if plan == "" {
		plan = string(orgplan.Starter)
	}
	name := inv.Description
	if strings.TrimSpace(name) == "" {
		name = fmt.Sprintf("initAgent %s — %d person / month", plan, qty)
		if qty != 1 {
			name = fmt.Sprintf("initAgent %s — %d people / month", plan, qty)
		}
	}
	return fakturowniaCreate{
		APIToken:       token,
		GovSaveAndSend: inv.Buyer.Company(),
		Invoice: fakturowniaInvoice{
			Kind:          "vat",
			Status:        "paid",
			SellDate:      day,
			IssueDate:     day,
			PaymentType:   "card",
			Currency:      currency,
			Oid:           inv.Idempotency,
			BuyerName:     strings.TrimSpace(inv.Buyer.Name),
			BuyerTaxNo:    CleanNIP(inv.Buyer.TaxNo),
			BuyerStreet:   strings.TrimSpace(inv.Buyer.Street),
			BuyerPostCode: strings.TrimSpace(inv.Buyer.PostCode),
			BuyerCity:     strings.TrimSpace(inv.Buyer.City),
			BuyerCountry:  inv.Buyer.NormCountry(),
			BuyerEmail:    strings.TrimSpace(inv.Buyer.Email),
			BuyerCompany:  inv.Buyer.Company(),
			Positions: []fakturowniaPosition{{
				Name:            name,
				Tax:             tax,
				Quantity:        1,
				TotalPriceGross: gross,
			}},
		},
	}
}
