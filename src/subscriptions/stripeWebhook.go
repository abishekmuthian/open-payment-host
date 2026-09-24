package subscriptions

import (
	"encoding/json"
	"errors"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/stripe/stripe-go/v72"
	"github.com/stripe/stripe-go/v72/charge"
	"github.com/stripe/stripe-go/v72/invoice"
	"github.com/stripe/stripe-go/v72/refund"
	"github.com/stripe/stripe-go/v72/sub"
	"github.com/stripe/stripe-go/v72/webhook"
	"net/http"
)

func HandleWebhook(w http.ResponseWriter, r *http.Request) error {
	body, err := readWebhookBody(w, r)
	if err != nil {
		return nil
	}
	secret := config.Get("stripe_webhook_secret")
	if secret == "" {
		http.Error(w, "Webhook is not configured", 503)
		return nil
	}
	e, err := webhook.ConstructEvent(body, r.Header.Get("Stripe-Signature"), secret)
	if err != nil {
		http.Error(w, "Invalid signature", 403)
		return nil
	}
	var resource struct {
		ID           string `json:"id"`
		Subscription string `json:"subscription"`
	}
	if err = json.Unmarshal(e.Data.Raw, &resource); err != nil {
		http.Error(w, "Invalid event", 400)
		return nil
	}
	err = processPaymentEvent("stripe", e.ID, func() error {
		switch e.Type {
		case "checkout.session.completed", "checkout.session.async_payment_succeeded":
			a, err := FindAttemptByProviderOrder("stripe", resource.ID)
			if err != nil {
				return err
			}
			f, err := fetchStripeSessionFacts(a)
			if err != nil {
				return err
			}
			_, err = verifyAndFulfill(a, f)
			return err
		case "invoice.paid", "invoice.payment_succeeded":
			return fulfillStripeInvoice(resource.ID)
		case "customer.subscription.deleted":
			return applySubscriptionStatus("stripe", resource.ID, "cancelled")
		case "invoice.payment_failed":
			return applySubscriptionStatus("stripe", resource.Subscription, "past_due")
		case "charge.refunded":
			client := charge.Client{B: stripe.GetBackend(stripe.APIBackend), Key: config.Get("stripe_secret")}
			c, err := client.Get(resource.ID, nil)
			if err != nil {
				return err
			}
			txn := ""
			if c.PaymentIntent != nil {
				txn = c.PaymentIntent.ID
			}
			if c.Invoice != nil {
				txn = c.Invoice.ID
			}
			refunds := refund.Client{B: stripe.GetBackend(stripe.APIBackend), Key: config.Get("stripe_secret")}
			it := refunds.List(&stripe.RefundListParams{Charge: stripe.String(c.ID)})
			for it.Next() {
				r := it.Refund()
				if r.Status == "succeeded" {
					if err := applyRefund("stripe", txn, r.ID, r.Amount, string(r.Currency)); err != nil {
						return err
					}
				}
			}
			return it.Err()
		}
		return nil
	})
	return webhookResult(w, err)
}
func fulfillStripeInvoice(id string) error {
	client := invoice.Client{B: stripe.GetBackend(stripe.APIBackend), Key: config.Get("stripe_secret")}
	inv, err := client.Get(id, nil)
	if err != nil {
		return err
	}
	if inv.ID != id || !inv.Paid || inv.Status != "paid" || inv.Subscription == nil {
		return errPaymentPending
	}
	a, err := FindAttemptByProviderSubscription("stripe", inv.Subscription.ID)
	if err != nil {
		client := sub.Client{B: stripe.GetBackend(stripe.APIBackend), Key: config.Get("stripe_secret")}
		subscription, err := client.Get(inv.Subscription.ID, nil)
		if err != nil {
			return err
		}
		a, err = FindAttempt(subscription.Metadata["attempt_id"])
		if err != nil {
			return err
		}
		facts, err := fetchStripeSessionFacts(a)
		if err != nil {
			return err
		}
		if facts.SubscriptionId != inv.Subscription.ID {
			return errors.New("invoice subscription mismatch")
		}
		if _, err := verifyAndFulfill(a, facts); err != nil {
			return err
		}
	}
	if inv.Lines == nil || inv.Lines.HasMore || len(inv.Lines.Data) != 1 {
		return errors.New("unexpected invoice line items")
	}
	item := inv.Lines.Data[0]
	if item.Price == nil || item.Price.ID != a.PriceId || item.Quantity != 1 {
		return errors.New("invoice price or quantity mismatch")
	}
	_, err = verifyAndFulfill(a, ProviderFacts{Gateway: "stripe", SubscriptionId: inv.Subscription.ID, PaymentId: inv.ID, PriceId: item.Price.ID, Amount: inv.AmountPaid, Currency: string(inv.Currency), Paid: true, Email: inv.CustomerEmail})
	return err
}
