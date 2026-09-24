package actions

import (
	"errors"
	"github.com/abishekmuthian/open-payment-host/src/lib/mux"
	"github.com/abishekmuthian/open-payment-host/src/lib/server"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/session"
	"github.com/abishekmuthian/open-payment-host/src/lib/view"
	"github.com/abishekmuthian/open-payment-host/src/subscriptions"
	"net/http"
	"time"
)

func HandlePaymentCancel(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	params, err := mux.Params(r)
	if err != nil {
		return server.BadRequestError(err)
	}
	a, err := subscriptions.FindCancellationAttempt(params.Get("subscription_id"))
	cancellationToken := params.Get("cancellation_token")
	if err != nil || !a.CancellationTokenValid(cancellationToken) {
		return server.NotAuthorizedError(errors.New("a valid unused cancellation link is required"))
	}
	if r.Method == http.MethodPost {
		if err := session.CheckAuthenticity(w, r); err != nil {
			return err
		}
		if err := subscriptions.CancelAttempt(a, cancellationToken); err != nil {
			return server.InternalError(err)
		}
		if a.RedirectURI != "" {
			return server.RedirectExternal(w, r, subscriptions.BuildRedirectURL(a.RedirectURI, map[string]string{"custom_id": a.CustomId, "subscription_id": a.ProviderSubscriptionId}))
		}
	} else if r.Method != http.MethodGet {
		return server.BadRequestError(errors.New("method not allowed"))
	}
	v := view.NewRenderer(w, r)
	v.AddKey("currentUser", session.CurrentUser(w, r))
	v.AddKey("name", config.Get("name"))
	v.AddKey("year", time.Now().Year())
	if r.Method == http.MethodGet {
		v.AddKey("subscription_id", a.ProviderSubscriptionId)
		v.AddKey("cancellation_token", cancellationToken)
		v.Template("subscriptions/views/payment_cancel_confirm.html.got")
	} else {
		v.Template("subscriptions/views/payment_cancel.html.got")
	}
	return v.Render()
}
