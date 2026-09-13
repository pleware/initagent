// Package billing is the hosted payment and fiscal-invoice seam (drafts 25, 48).
//
// Stripe takes the card. Fakturownia.pl issues the company invoice and
// queues it for KSeF. Self-host never constructs a live client: there is
// no path against our Stripe or our Fakturownia account.
package billing

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/pleware/initagent/internal/offering"
	"github.com/pleware/initagent/internal/orgplan"
)

var (
	// ErrNotHosted means this process is self-host. Plans stay a cloud surface.
	ErrNotHosted = errors.New("billing: plans are only on the hosted hub")
	// ErrNotConfigured means hosted, but Stripe is not wired yet.
	ErrNotConfigured = errors.New("billing: checkout is not configured")
	// ErrUnknownPlan means the slug is not a self-serve paid catalogue row.
	ErrUnknownPlan = errors.New("billing: plan is not self-serve paid")
	// ErrNoPrice means the slug is paid but no Stripe Price id is set.
	ErrNoPrice = errors.New("billing: Stripe Price id is missing")
	// ErrBuyer means the org has not given enough invoice details.
	ErrBuyer = errors.New("billing: buyer is incomplete")
	// ErrIgnored means Stripe delivered an event this seam does not apply.
	ErrIgnored = errors.New("billing: ignored event")
)

// Config is what ops supplies via env. Secret values never live in this tree.
type Config struct {
	Offering          offering.Kind
	StripeSecret      string
	WebhookSecret     string
	PriceStarter      string
	PriceTeam         string
	FakturowniaToken  string
	FakturowniaDomain string
}

// CheckoutRequest starts a Stripe Checkout Session for one org.
type CheckoutRequest struct {
	OrgID      string
	Plan       orgplan.ID
	Quantity   int
	SuccessURL string
	CancelURL  string
	CustomerID string
	Buyer      Buyer
}

// CheckoutSession is the URL the browser must open.
type CheckoutSession struct {
	URL string `json:"url"`
}

const (
	// KindCheckout is Stripe checkout.session.completed.
	KindCheckout = "checkout.session.completed"
	// KindInvoicePaid is Stripe invoice.paid (first charge and renewals).
	KindInvoicePaid = "invoice.paid"
)

// Event is one verified Stripe event, reduced to what the hub must apply.
type Event struct {
	ID             string
	Kind           string
	OrgID          string
	Plan           orgplan.ID
	CustomerID     string
	SubscriptionID string
	InvoiceID      string
	AmountCents    int64
	Currency       string
	Buyer          Buyer
}

// Invoice is one fiscal document to create after Stripe marks a payment paid.
type Invoice struct {
	OrgID       string
	Plan        orgplan.ID
	Quantity    int
	AmountCents int64
	Currency    string
	Description string
	Idempotency string
	Buyer       Buyer
}

// Service is the hosted billing seam. A nil-ready service is how self-host
// and an unwired hosted hub look from the outside: handlers refuse.
type Service struct {
	hosted        bool
	prices        map[orgplan.ID]string
	webhookSecret string
	start         func(context.Context, CheckoutRequest, string) (string, error)
	parse         func(payload []byte, sig, secret string) (Event, error)
	setQty        func(context.Context, string, int) error
	issue         func(context.Context, Invoice) (string, error)
}

// New builds the seam. Missing Stripe keys are not a start error: the
// cockpit still lists plans, and Checkout says it is not configured.
func New(cfg Config) *Service {
	if cfg.Offering != offering.Hosted {
		return &Service{hosted: false}
	}
	s := &Service{
		hosted:        true,
		prices:        hostedPrices(cfg),
		webhookSecret: strings.TrimSpace(cfg.WebhookSecret),
	}
	secret := strings.TrimSpace(cfg.StripeSecret)
	if secret != "" {
		s.start = stripeStart(secret)
		s.parse = stripeParse
		s.setQty = stripeSetQuantity(secret)
	}
	token := strings.TrimSpace(cfg.FakturowniaToken)
	domain := strings.TrimSpace(cfg.FakturowniaDomain)
	if token != "" && domain != "" {
		s.issue = fakturowniaIssue(domain, token, nil)
	}
	return s
}

// Ready reports whether Checkout can start.
func (s *Service) Ready() bool {
	return s != nil && s.hosted && s.start != nil && (s.prices[orgplan.Starter] != "" || s.prices[orgplan.Team] != "")
}

// Hosted reports whether this process is the cloud offering.
func (s *Service) Hosted() bool {
	return s != nil && s.hosted
}

// InvoicesReady reports whether a paid Stripe event can mint a Fakturownia row.
func (s *Service) InvoicesReady() bool {
	return s != nil && s.issue != nil
}

// StartCheckout opens Stripe Checkout for a self-serve paid plan.
func (s *Service) StartCheckout(ctx context.Context, req CheckoutRequest) (CheckoutSession, error) {
	if s == nil || !s.hosted {
		return CheckoutSession{}, ErrNotHosted
	}
	if s.start == nil {
		return CheckoutSession{}, ErrNotConfigured
	}
	if err := req.Buyer.Validate(); err != nil {
		return CheckoutSession{}, err
	}
	price, err := s.priceOf(req.Plan)
	if err != nil {
		return CheckoutSession{}, err
	}
	if req.Quantity < 1 {
		req.Quantity = 1
	}
	url, err := s.start(ctx, req, price)
	if err != nil {
		return CheckoutSession{}, err
	}
	return CheckoutSession{URL: url}, nil
}

// ParseWebhook verifies a Stripe signature and reduces the event.
func (s *Service) ParseWebhook(payload []byte, sig string) (Event, error) {
	if s == nil || !s.hosted {
		return Event{}, ErrNotHosted
	}
	if s.parse == nil || s.webhookSecret == "" {
		return Event{}, ErrNotConfigured
	}
	return s.parse(payload, sig, s.webhookSecret)
}

// IssueInvoice creates the Fakturownia VAT invoice and asks KSeF to take it
// when the buyer is a company with a NIP.
func (s *Service) IssueInvoice(ctx context.Context, inv Invoice) (string, error) {
	if s == nil || !s.hosted {
		return "", ErrNotHosted
	}
	if s.issue == nil {
		return "", fmt.Errorf("billing: Fakturownia is not configured")
	}
	if err := inv.Buyer.Validate(); err != nil {
		return "", err
	}
	if strings.TrimSpace(inv.Idempotency) == "" {
		return "", fmt.Errorf("billing: invoice idempotency is required")
	}
	return s.issue(ctx, inv)
}

// SetQuantity updates the Stripe subscription to the current org roster.
func (s *Service) SetQuantity(ctx context.Context, subscriptionID string, n int) error {
	if s == nil || !s.hosted || s.setQty == nil {
		return nil
	}
	subscriptionID = strings.TrimSpace(subscriptionID)
	if subscriptionID == "" {
		return nil
	}
	if n < 1 {
		n = 1
	}
	return s.setQty(ctx, subscriptionID, n)
}

func (s *Service) priceOf(id orgplan.ID) (string, error) {
	if !orgplan.SelfServe(string(id)) {
		return "", ErrUnknownPlan
	}
	p, ok := orgplan.Lookup(string(id))
	if !ok || p.Charge.Kind != orgplan.ChargeUSD {
		return "", ErrUnknownPlan
	}
	price := s.prices[id]
	if price == "" {
		return "", ErrNoPrice
	}
	return price, nil
}

func hostedPrices(cfg Config) map[orgplan.ID]string {
	out := map[orgplan.ID]string{
		orgplan.Starter: strings.TrimSpace(orgplan.CheckoutPrice(orgplan.Starter)),
		orgplan.Team:    strings.TrimSpace(orgplan.CheckoutPrice(orgplan.Team)),
	}
	if v := strings.TrimSpace(cfg.PriceStarter); v != "" {
		out[orgplan.Starter] = v
	}
	if v := strings.TrimSpace(cfg.PriceTeam); v != "" {
		out[orgplan.Team] = v
	}
	return out
}
