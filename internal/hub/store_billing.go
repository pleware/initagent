package hub

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/pleware/initagent/internal/billing"
	"github.com/pleware/initagent/internal/orgplan"
	"github.com/pleware/initagent/internal/store"
)

// OrgBilling is the hosted invoice profile and Stripe ids for one org.
type OrgBilling struct {
	OrgID              string `json:"orgId"`
	Plan               string `json:"plan"`
	People             int    `json:"people"`
	CheckoutReady      bool   `json:"checkoutReady"`
	InvoicesReady      bool   `json:"invoicesReady"`
	Name               string `json:"name"`
	TaxNo              string `json:"taxNo"`
	Street             string `json:"street"`
	City               string `json:"city"`
	PostCode           string `json:"postCode"`
	Country            string `json:"country"`
	Email              string `json:"email"`
	StripeCustomer     string `json:"-"`
	StripeSubscription string `json:"-"`
}

func (s *Store) ensureOrgBilling() error {
	for _, col := range []string{
		"billing_name", "billing_tax_no", "billing_street", "billing_city",
		"billing_postcode", "billing_country", "billing_email",
		"stripe_customer", "stripe_subscription",
	} {
		if err := s.ensureColumn("orgs", col, "TEXT NOT NULL DEFAULT ''"); err != nil {
			return err
		}
	}
	createdAt := "INTEGER NOT NULL"
	if s.db.Dialect() == store.Postgres {
		createdAt = "BIGINT NOT NULL"
	}
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS billing_invoices (
		stripe_invoice TEXT PRIMARY KEY,
		org_id         TEXT NOT NULL,
		fakturownia_id TEXT NOT NULL DEFAULT '',
		created_at     ` + createdAt + `
	)`)
	return err
}

func (s *Store) GetOrgBilling(orgID string) (*OrgBilling, error) {
	var b OrgBilling
	err := s.db.QueryRow(`SELECT o.id, o.plan, COUNT(m.account_id),
		o.billing_name, o.billing_tax_no, o.billing_street, o.billing_city,
		o.billing_postcode, o.billing_country, o.billing_email,
		o.stripe_customer, o.stripe_subscription
		FROM orgs o LEFT JOIN org_members m ON m.org_id = o.id
		WHERE o.id = ?
		GROUP BY o.id, o.plan, o.billing_name, o.billing_tax_no, o.billing_street,
			o.billing_city, o.billing_postcode, o.billing_country, o.billing_email,
			o.stripe_customer, o.stripe_subscription`, orgID).
		Scan(&b.OrgID, &b.Plan, &b.People, &b.Name, &b.TaxNo, &b.Street, &b.City,
			&b.PostCode, &b.Country, &b.Email, &b.StripeCustomer, &b.StripeSubscription)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (s *Store) SaveOrgBilling(orgID string, buyer billing.Buyer) error {
	if err := buyer.Validate(); err != nil {
		return err
	}
	_, err := s.db.Exec(`UPDATE orgs SET
		billing_name = ?, billing_tax_no = ?, billing_street = ?, billing_city = ?,
		billing_postcode = ?, billing_country = ?, billing_email = ?
		WHERE id = ?`,
		strings.TrimSpace(buyer.Name), billing.CleanNIP(buyer.TaxNo),
		strings.TrimSpace(buyer.Street), strings.TrimSpace(buyer.City),
		strings.TrimSpace(buyer.PostCode), buyer.NormCountry(), strings.TrimSpace(buyer.Email),
		orgID)
	return err
}

func (s *Store) ApplyPaidPlan(ev billing.Event) error {
	if _, err := orgplan.Parse(string(ev.Plan)); err != nil {
		return err
	}
	_, err := s.db.Exec(`UPDATE orgs SET plan = ?,
		stripe_customer = CASE WHEN ? = '' THEN stripe_customer ELSE ? END,
		stripe_subscription = CASE WHEN ? = '' THEN stripe_subscription ELSE ? END
		WHERE id = ?`,
		string(ev.Plan), ev.CustomerID, ev.CustomerID, ev.SubscriptionID, ev.SubscriptionID, ev.OrgID)
	return err
}

func (s *Store) RecordBillingInvoice(stripeInvoice, orgID, fakturowniaID string) (bool, error) {
	res, err := s.db.Exec(`INSERT INTO billing_invoices (stripe_invoice, org_id, fakturownia_id, created_at)
		VALUES (?, ?, ?, ?)`, stripeInvoice, orgID, fakturowniaID, time.Now().Unix())
	if err != nil {
		if uniqueConstraint(err) {
			return false, nil
		}
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (s *Store) HasBillingInvoice(stripeInvoice string) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT 1 FROM billing_invoices WHERE stripe_invoice = ?`, stripeInvoice).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}
