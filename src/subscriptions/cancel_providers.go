package subscriptions

import (
	"errors"
	"github.com/abishekmuthian/open-payment-host/src/lib/mux"
	"github.com/abishekmuthian/open-payment-host/src/lib/query"
	"github.com/abishekmuthian/open-payment-host/src/lib/server"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/session"
	"github.com/abishekmuthian/open-payment-host/src/products"
	"github.com/stripe/stripe-go/v72"
	stripesub "github.com/stripe/stripe-go/v72/sub"
	"net/http"
	"net/url"
)

func FindCancellationAttempt(id string) (*PaymentAttempt, error) {
	if id == "" {
		return nil, errors.New("missing subscription")
	}
	rows, err := attemptQuery().Where("provider_subscription_id=?", id).Results()
	if err != nil {
		return nil, err
	}
	if len(rows) != 1 {
		return nil, errors.New("subscription is absent or ambiguous")
	}
	return attemptWithColumns(rows[0]), nil
}
func CancelAttempt(a *PaymentAttempt, token string) error {
	if !a.CancellationTokenValid(token) {
		return errors.New("invalid cancellation token")
	}
	// Consume before contacting a provider. Concurrent requests and uncertain
	// network failures must never reuse the capability for another API call.
	if err := query.Transaction(func(tx *query.Tx) error {
		result, err := tx.Exec("UPDATE payment_attempts SET cancellation_used=1 WHERE id=? AND cancellation_used=0 AND cancellation_token_hash=? AND status NOT IN ('cancelled','expired')", a.Id, hashToken(token))
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return errors.New("cancellation already requested")
		}
		return nil
	}); err != nil {
		return err
	}
	switch a.Gateway {
	case "stripe":
		return CancelStripeSubscription(a.ProviderSubscriptionId)
	case "square":
		return CancelSquareSubscription(a.ProviderSubscriptionId)
	case "paypal":
		return CancelPaypalSubscription(a.ProviderSubscriptionId)
	case "razorpay":
		return CancelRazorpaySubscription(a.ProviderSubscriptionId)
	}
	return errors.New("unknown payment gateway")
}
func CancelSquareSubscription(id string) error {
	return providerJSON("square", http.MethodPost, "/subscriptions/"+url.PathEscape(id)+"/cancel", map[string]interface{}{}, nil)
}
func CancelStripeSubscription(id string) error {
	c := stripesub.Client{B: stripe.GetBackend(stripe.APIBackend), Key: config.Get("stripe_secret")}
	_, err := c.Update(id, &stripe.SubscriptionParams{CancelAtPeriodEnd: stripe.Bool(true)})
	return err
}

// HandleCancellationLink lets administrators issue or replace a capability for
// a stored subscription, including legacy subscriptions. It never fulfills it.
func HandleCancellationLink(w http.ResponseWriter, r *http.Request) error {
	paymentResponseHeaders(w)
	if !session.CurrentUser(w, r).Admin() {
		return server.NotAuthorizedError(nil)
	}
	if err := session.CheckAuthenticity(w, r); err != nil {
		return err
	}
	params, err := mux.Params(r)
	if err != nil {
		return server.BadRequestError(err)
	}
	id := params.Get("subscription_id")
	a, err := FindCancellationAttempt(id)
	if err != nil {
		stored, err := FindSubscription(id)
		if err != nil {
			return server.NotFoundError(err)
		}
		p, err := products.Find(stored.ProductId)
		if err != nil {
			return server.NotFoundError(err)
		}
		if p.Schedule == "onetime" {
			return server.BadRequestError(errors.New("not a subscription"))
		}
		a, err = newAttempt(p.ID, stored.PaymentGateway, p.Schedule, "DF", 1, "USD", "", stored.UserId, "")
		if err != nil {
			return server.InternalError(err)
		}
		if err = a.SetProviderIds("", "", id); err != nil {
			return server.InternalError(err)
		}
		if err = attemptQuery().Where("id=?", a.Id).Update(map[string]string{"status": "legacy"}); err != nil {
			return server.InternalError(err)
		}
	}
	token, err := generateToken(32)
	if err != nil {
		return server.InternalError(err)
	}
	encrypted, err := sealCapability(token)
	if err != nil {
		return server.InternalError(err)
	}
	if err = attemptQuery().Where("id=?", a.Id).Update(map[string]string{"cancellation_token_hash": hashToken(token), "cancellation_token_ciphertext": encrypted, "cancellation_used": "0"}); err != nil {
		return server.InternalError(err)
	}
	a.CancellationTokenCiphertext = encrypted
	a.CancellationUsed = false
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, err = w.Write([]byte(a.CancellationURL()))
	return err
}
