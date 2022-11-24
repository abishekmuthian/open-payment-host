package actions

import (
	"encoding/json"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
	"github.com/abishekmuthian/open-payment-host/src/lib/session"
	"github.com/abishekmuthian/open-payment-host/src/subscriptions"
	"github.com/stripe/stripe-go/v72"
	portalsession "github.com/stripe/stripe-go/v72/billingportal/session"
	stripesession "github.com/stripe/stripe-go/v72/checkout/session"
	"net/http"
	"strconv"
)

func HandleCheckoutSession(w http.ResponseWriter, r *http.Request) error {
	// Set your secret key. Remember to switch to your live secret key in production.
	// See your keys here: https://dashboard.stripe.com/account/apikeys
	stripe.Key = config.Get("stripe_secret")

	if r.Method != "GET" {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return nil
	}
	sessionID := r.URL.Query().Get("sessionId")
	s, err := stripesession.Get(sessionID, nil)
	writeJSON(w, s, err)
	return err
}

func HandleCustomerPortal(w http.ResponseWriter, r *http.Request) error {
	// Set your secret key. Remember to switch to your live secret key in production.
	// See your keys here: https://dashboard.stripe.com/account/apikeys
	stripe.Key = config.Get("stripe_secret")

	if r.Method != "POST" {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return nil
	}

	var req struct {
		AuthenticityToken string `json:"authenticityToken"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Error(log.V{"Portal json.NewDecoder.Decode: %v": err})
		return nil
	}

	// Check token authenticity
	err := session.CheckAuthenticityToken(w, r, req.AuthenticityToken)
	if err != nil {
		return err
	}

	// Authorise update user
	currentUser := session.CurrentUser(w, r)

	// The URL to which the user is redirected when they are done managing
	// billing in the portal.
	returnURL := config.Get("stripe_callback_domain") + "/" + "users" + "/" + strconv.FormatInt(currentUser.ID, 10) + "/update"

	subscription, err := subscriptions.FindCustomerId(currentUser.ID)

	if err == nil {
		params := &stripe.BillingPortalSessionParams{
			Customer:  stripe.String(subscription.CustomerId),
			ReturnURL: stripe.String(returnURL),
		}
		ps, _ := portalsession.New(params)

		writeJSON(w, struct {
			URL string `json:"url"`
		}{
			URL: ps.URL,
		}, nil)
	} else {
		log.Error(log.V{"Portal, Error: ": err})
	}

	return err
}
