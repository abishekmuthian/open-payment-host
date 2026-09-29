package subscriptions

import (
	"encoding/json"
	"errors"
	"github.com/abishekmuthian/open-payment-host/src/lib/server"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/stripe/stripe-go/v72"
	stripesession "github.com/stripe/stripe-go/v72/checkout/session"
	"github.com/stripe/stripe-go/v72/invoice"
	"net/http"
	"strconv"
)

func stripeSessions() stripesession.Client {
	return stripesession.Client{B: stripe.GetBackend(stripe.APIBackend), Key: config.Get("stripe_secret")}
}
func fetchStripeSessionFacts(a *PaymentAttempt) (ProviderFacts, error) {
	client := stripeSessions()
	s, err := client.Get(a.ProviderOrderId, nil)
	if err != nil {
		return ProviderFacts{}, err
	}
	if s.ID != a.ProviderOrderId || s.Metadata["attempt_id"] != a.Id || s.Metadata["product_id"] != strconv.FormatInt(a.ProductId, 10) {
		return ProviderFacts{}, errors.New("session metadata mismatch")
	}
	if s.Status != "complete" || s.PaymentStatus != "paid" {
		return ProviderFacts{}, errPaymentPending
	}
	mode := stripe.CheckoutSessionModePayment
	if a.Schedule != "onetime" {
		mode = stripe.CheckoutSessionModeSubscription
	}
	if s.Mode != mode {
		return ProviderFacts{}, errors.New("session mode mismatch")
	}
	items := client.ListLineItems(s.ID, nil)
	count := 0
	for items.Next() {
		count++
		item := items.LineItem()
		if item.Price == nil || item.Price.ID != a.PriceId || item.Quantity != 1 || item.AmountTotal != a.Amount || !currencyEqual(string(item.Currency), a.Currency) {
			return ProviderFacts{}, errors.New("checkout line item mismatch")
		}
	}
	if err := items.Err(); err != nil {
		return ProviderFacts{}, err
	}
	if count != 1 {
		return ProviderFacts{}, errors.New("expected one line item")
	}
	f := ProviderFacts{Gateway: "stripe", OrderId: s.ID, PriceId: a.PriceId, Amount: s.AmountTotal, Currency: string(s.Currency), Paid: true}
	if s.PaymentIntent != nil {
		f.PaymentId = s.PaymentIntent.ID
	}
	if s.Subscription != nil {
		f.SubscriptionId = s.Subscription.ID
		// A recurring Checkout Session is bound to its first invoice. Renewals
		// also use invoice IDs, preventing double insertion of that first charge.
		var raw struct {
			Invoice *stripe.Invoice `json:"invoice"`
		}
		if s.LastResponse != nil {
			if err := json.Unmarshal(s.LastResponse.RawJSON, &raw); err != nil {
				return ProviderFacts{}, err
			}
		}
		if raw.Invoice != nil {
			f.PaymentId = raw.Invoice.ID
		} else {
			// Older Stripe API versions omit Session.invoice. Select only the
			// subscription creation invoice, never its latest renewal.
			invoices := invoice.Client{B: stripe.GetBackend(stripe.APIBackend), Key: config.Get("stripe_secret")}
			it := invoices.List(&stripe.InvoiceListParams{Subscription: stripe.String(s.Subscription.ID)})
			for it.Next() {
				inv := it.Invoice()
				if inv.BillingReason != stripe.InvoiceBillingReasonSubscriptionCreate {
					continue
				}
				if !inv.Paid || inv.Status != "paid" || inv.AmountPaid != a.Amount || !currencyEqual(string(inv.Currency), a.Currency) || inv.Subscription == nil || inv.Subscription.ID != s.Subscription.ID {
					return ProviderFacts{}, errors.New("initial subscription invoice mismatch")
				}
				if f.PaymentId != "" {
					return ProviderFacts{}, errors.New("ambiguous initial invoice")
				}
				f.PaymentId = inv.ID
			}
			if err := it.Err(); err != nil {
				return ProviderFacts{}, err
			}
			if f.PaymentId == "" {
				return ProviderFacts{}, errPaymentPending
			}
		}
		if err := a.SetProviderIds("", "", s.Subscription.ID); err != nil {
			return ProviderFacts{}, err
		}
	}
	if s.CustomerDetails != nil {
		f.Email = s.CustomerDetails.Email
		f.Name = s.CustomerDetails.Name
	}
	return f, nil
}
func HandleStripeSuccess(w http.ResponseWriter, r *http.Request) error {
	paymentResponseHeaders(w)
	if r.URL.Query().Has("product_id") {
		return server.BadRequestError(errors.New("product_id is not accepted on success"))
	}
	a, err := FindAttemptByProviderOrder("stripe", r.URL.Query().Get("session_id"))
	if err != nil || !bindAttemptToRequest(r, a) {
		return server.NotAuthorizedError(errors.New("payment could not be verified"))
	}
	f, err := fetchStripeSessionFacts(a)
	if err != nil {
		return server.BadRequestError(err)
	}
	if _, err = verifyAndFulfill(a, f); err != nil {
		return server.BadRequestError(err)
	}
	return completeAndRedirect(w, r, a, f)
}
