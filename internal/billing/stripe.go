package billing

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/pleware/initagent/internal/orgplan"
	"github.com/stripe/stripe-go/v82"
	checkoutsession "github.com/stripe/stripe-go/v82/checkout/session"
	"github.com/stripe/stripe-go/v82/subscription"
	"github.com/stripe/stripe-go/v82/webhook"
)

func stripeStart(secret string) func(context.Context, CheckoutRequest, string) (string, error) {
	return func(_ context.Context, req CheckoutRequest, price string) (string, error) {
		stripe.Key = secret
		params := &stripe.CheckoutSessionParams{
			Mode:       stripe.String(string(stripe.CheckoutSessionModeSubscription)),
			SuccessURL: stripe.String(req.SuccessURL),
			CancelURL:  stripe.String(req.CancelURL),
			LineItems: []*stripe.CheckoutSessionLineItemParams{{
				Price:    stripe.String(price),
				Quantity: stripe.Int64(int64(req.Quantity)),
			}},
			ClientReferenceID: stripe.String(req.OrgID),
			Metadata: map[string]string{
				"org_id":  req.OrgID,
				"plan_id": string(req.Plan),
			},
			SubscriptionData: &stripe.CheckoutSessionSubscriptionDataParams{
				Metadata: map[string]string{
					"org_id":  req.OrgID,
					"plan_id": string(req.Plan),
				},
			},
		}
		if req.CustomerID != "" {
			params.Customer = stripe.String(req.CustomerID)
		} else {
			params.CustomerEmail = stripe.String(req.Buyer.Email)
		}
		sess, err := checkoutsession.New(params)
		if err != nil {
			return "", fmt.Errorf("stripe checkout: %w", err)
		}
		if sess == nil || sess.URL == "" {
			return "", fmt.Errorf("stripe checkout: empty session URL")
		}
		return sess.URL, nil
	}
}

func stripeSetQuantity(secret string) func(context.Context, string, int) error {
	return func(_ context.Context, subscriptionID string, n int) error {
		stripe.Key = secret
		sub, err := subscription.Get(subscriptionID, nil)
		if err != nil {
			return fmt.Errorf("stripe subscription: %w", err)
		}
		if sub == nil || sub.Items == nil || len(sub.Items.Data) == 0 {
			return fmt.Errorf("stripe subscription %q has no items", subscriptionID)
		}
		_, err = subscription.Update(subscriptionID, &stripe.SubscriptionParams{
			Items: []*stripe.SubscriptionItemsParams{{
				ID:       stripe.String(sub.Items.Data[0].ID),
				Quantity: stripe.Int64(int64(n)),
			}},
		})
		if err != nil {
			return fmt.Errorf("stripe quantity: %w", err)
		}
		return nil
	}
}

func stripeParse(payload []byte, sig, secret string) (Event, error) {
	raw, err := webhook.ConstructEventWithOptions(payload, sig, secret, webhook.ConstructEventOptions{
		IgnoreAPIVersionMismatch: true,
	})
	if err != nil {
		return Event{}, fmt.Errorf("stripe webhook: %w", err)
	}
	return reduceStripeEvent(string(raw.Type), raw.ID, raw.Data.Raw)
}

func reduceStripeEvent(kind, id string, data json.RawMessage) (Event, error) {
	ev := Event{ID: id, Kind: kind}
	switch kind {
	case KindCheckout:
		var sess struct {
			ClientReferenceID string       `json:"client_reference_id"`
			Customer          expandableID `json:"customer"`
			Subscription      expandableID `json:"subscription"`
			Metadata          map[string]string
		}
		if err := json.Unmarshal(data, &sess); err != nil {
			return Event{}, err
		}
		ev.OrgID = cmp.Or(sess.Metadata["org_id"], sess.ClientReferenceID)
		ev.Plan = orgplan.ID(sess.Metadata["plan_id"])
		ev.CustomerID = string(sess.Customer)
		ev.SubscriptionID = string(sess.Subscription)
	case KindInvoicePaid:
		var inv struct {
			ID           string       `json:"id"`
			Customer     expandableID `json:"customer"`
			AmountPaid   int64        `json:"amount_paid"`
			Currency     string       `json:"currency"`
			Subscription expandableID `json:"subscription"`
			Parent       *struct {
				SubscriptionDetails *struct {
					Metadata     map[string]string `json:"metadata"`
					Subscription string            `json:"subscription"`
				} `json:"subscription_details"`
			} `json:"parent"`
			SubscriptionDetails *struct {
				Metadata map[string]string `json:"metadata"`
			} `json:"subscription_details"`
			Lines *struct {
				Data []struct {
					Metadata map[string]string `json:"metadata"`
				} `json:"data"`
			} `json:"lines"`
		}
		if err := json.Unmarshal(data, &inv); err != nil {
			return Event{}, err
		}
		meta := map[string]string{}
		if inv.SubscriptionDetails != nil {
			meta = inv.SubscriptionDetails.Metadata
		}
		if inv.Parent != nil && inv.Parent.SubscriptionDetails != nil {
			if meta == nil {
				meta = map[string]string{}
			}
			for k, v := range inv.Parent.SubscriptionDetails.Metadata {
				meta[k] = v
			}
			if ev.SubscriptionID == "" {
				ev.SubscriptionID = inv.Parent.SubscriptionDetails.Subscription
			}
		}
		if inv.Lines != nil {
			for _, line := range inv.Lines.Data {
				for k, v := range line.Metadata {
					if meta[k] == "" {
						meta[k] = v
					}
				}
			}
		}
		ev.OrgID = meta["org_id"]
		ev.Plan = orgplan.ID(meta["plan_id"])
		ev.CustomerID = string(inv.Customer)
		if ev.SubscriptionID == "" {
			ev.SubscriptionID = string(inv.Subscription)
		}
		ev.InvoiceID = inv.ID
		ev.AmountCents = inv.AmountPaid
		ev.Currency = strings.ToUpper(inv.Currency)
	default:
		return Event{}, fmt.Errorf("%w: %s", ErrIgnored, kind)
	}
	if ev.OrgID == "" {
		return Event{}, fmt.Errorf("stripe webhook: missing org_id on %s", kind)
	}
	if ev.Plan == "" {
		return Event{}, fmt.Errorf("stripe webhook: missing plan_id on %s", kind)
	}
	if _, err := orgplan.Parse(string(ev.Plan)); err != nil {
		return Event{}, fmt.Errorf("stripe webhook: %w", err)
	}
	return ev, nil
}

// expandableID accepts a Stripe id as a string or as {"id":"…"}.
type expandableID string

func (id *expandableID) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" {
		*id = ""
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*id = expandableID(s)
		return nil
	}
	var obj struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(b, &obj); err != nil {
		return err
	}
	*id = expandableID(obj.ID)
	return nil
}

// amountLabel is used in tests so a zero Stripe invoice is visible.
func amountLabel(cents int64, currency string) string {
	return strconv.FormatInt(cents, 10) + " " + strings.ToUpper(currency)
}
