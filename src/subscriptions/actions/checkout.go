package actions

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/abishekmuthian/open-payment-host/src/lib/mux"
	s3 "github.com/abishekmuthian/open-payment-host/src/lib/s3"
	"github.com/abishekmuthian/open-payment-host/src/lib/server"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
	"github.com/abishekmuthian/open-payment-host/src/lib/session"
	"github.com/abishekmuthian/open-payment-host/src/products"
	"github.com/stripe/stripe-go/v72"
	stripesession "github.com/stripe/stripe-go/v72/checkout/session"
	"github.com/stripe/stripe-go/v72/price"
)

func HandleCreateCheckoutSession(w http.ResponseWriter, r *http.Request) error {

	// Set your secret key. Remember to switch to your live secret key in production.
	// See your keys here: https://dashboard.stripe.com/account/apikeys
	stripe.Key = config.Get("stripe_secret")

	if r.Method != "POST" {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return nil
	}

	var req struct {
		Price             string `json:"priceId"`
		AuthenticityToken string `json:"authenticityToken"`
		Product           string `json:"productId"`
	}

	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

	req.Price = params.Get("priceId")
	req.Product = params.Get("productId")

	var successURL *string

	productID, err := strconv.ParseInt(req.Product, 10, 64)

	if err == nil {
		product, err := products.Find(productID)

		if err == nil {

			if product.S3Bucket != "" && product.S3Key != "" {
				downloadUrl, err := s3.GeneratePresignedUrl(product.S3Bucket, product.S3Key)

				if err == nil {
					return server.RedirectExternal(w, r, downloadUrl)
				}
				successURL = stripe.String(downloadUrl)
			}

		}
	}

	if *successURL == "" || successURL == nil {
		successURL = stripe.String(config.Get("stripe_callback_domain") + "/payment/success?session_id={CHECKOUT_SESSION_ID}")
	}

	// Needed when using stripe JS
	/*	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Error(log.V{"Checkout json.NewDecoder.Decode: %v": err})
		if e, ok := err.(*json.SyntaxError); ok {
			log.Error(log.V{"syntax error at byte offset %d": e.Offset})
		}
		log.Error(log.V{"Response %q": r.Body})
		return nil
	}*/

	// Check token authenticity
	err = session.CheckAuthenticity(w, r)
	if err != nil {
		return err
	}

	// Anon users are allowed to subscribe
	/* 	// Authorise
	   	currentUser := session.CurrentUser(w, r)

	   	subscription := subscriptions.New()

	   	err = can.Create(subscription, currentUser)
	   	if err != nil {
	   		// FIXME: Redirection to to error page not working
	   		return server.NotAuthorizedError(err)
	   	} */

	// See https://stripe.com/docs/api/checkout/sessions/create
	// for additional parameters to pass.
	// {CHECKOUT_SESSION_ID} is a string literal; do not change it!
	// the actual Session ID is returned in the query parameter when your customer
	// is redirected to the success page.

	// Subscription or One Time Payment
	var mode *string
	var taxRate []*string
	var subscriptionData *stripe.CheckoutSessionSubscriptionDataParams

	// Check if the price is recurring or one time
	p, err := price.Get(req.Price, nil)

	if err == nil {
		log.Info(log.V{"Currency:": p.Currency})

		if p.Type == "recurring" {
			mode = stripe.String(string(stripe.CheckoutSessionModeSubscription))
			subscriptionData = &stripe.CheckoutSessionSubscriptionDataParams{
				DefaultTaxRates: stripe.StringSlice([]string{
					config.Get("stripe_tax_rate_IN"),
				}),
			}
		} else if p.Type == "one_time" {
			mode = stripe.String(string(stripe.CheckoutSessionModePayment))
			taxRate = stripe.StringSlice([]string{
				config.Get("stripe_tax_rate_IN"),
			})
		}
	}

	clientCountry := r.Header.Get("CF-IPCountry")
	log.Info(log.V{"Subscription, Client Country": clientCountry})
	if !config.Production() {
		// There will be no CF request header in the development/test
		clientCountry = config.Get("subscription_client_country")
	}

	// Redirect to the new story
	productId, err := strconv.ParseInt(req.Product, 10, 64)
	if err != nil {
		return server.InternalError(err)
	}
	story, err := products.Find(productId)
	if err != nil {
		return server.InternalError(err)
	}

	if clientCountry == "IN" {
		// If India, add tax ID
		params := &stripe.CheckoutSessionParams{
			BillingAddressCollection: stripe.String("required"),
			CancelURL:                stripe.String(config.Get("stripe_callback_domain") + "/payment/cancel"),
			LineItems: []*stripe.CheckoutSessionLineItemParams{
				{
					Price: stripe.String(req.Price),
					// For metered billing, do not pass quantity
					Quantity: stripe.Int64(1),
					TaxRates: taxRate,
				},
			},
			Mode: mode,
			PaymentMethodTypes: stripe.StringSlice([]string{
				"card",
			}),
			SubscriptionData: subscriptionData,

			SuccessURL: successURL,
		}

		params.AddMetadata("plan", story.NameDisplay())

		if req.Product != "" {
			params.AddMetadata("product_id", req.Product)
		}

		s, err := stripesession.New(params)
		if err != nil {
			// Needed when using stripe JS
			/*			w.WriteHeader(http.StatusBadRequest)
						writeJSON(w, nil, err)
						return nil*/
			return server.InternalError(err)
		}
		// Needed when using stripe JS
		/*		writeJSON(w, struct {
					SessionID string `json:"sessionId"`
				}{
					SessionID: s.ID,
				}, nil)*/
		// Then redirect to the URL on the Checkout Session
		http.Redirect(w, r, s.URL, http.StatusSeeOther)

	} else {
		// No Tax ID for rest of the world
		params := &stripe.CheckoutSessionParams{
			BillingAddressCollection: stripe.String("required"),
			SuccessURL:               successURL,
			CancelURL:                stripe.String(config.Get("stripe_callback_domain") + "/payment/cancel"),
			PaymentMethodTypes: stripe.StringSlice([]string{
				"card",
			}),
			Mode: mode,
			LineItems: []*stripe.CheckoutSessionLineItemParams{
				{
					Price: stripe.String(req.Price),
					// For metered billing, do not pass quantity
					Quantity: stripe.Int64(1),
				},
			},
		}

		params.AddMetadata("plan", story.NameDisplay())

		if req.Product != "" {
			params.AddMetadata("product_id", req.Product)
		}

		s, err := stripesession.New(params)
		if err != nil {
			// Needed when using stripe JS
			/*			w.WriteHeader(http.StatusBadRequest)
						writeJSON(w, nil, err)
						return nil*/
			return server.InternalError(err)
		}
		// Needed when using stripe JS
		/*		writeJSON(w, struct {
					SessionID string `json:"sessionId"`
				}{
					SessionID: s.ID,
				}, nil)*/
		// Then redirect to the URL on the Checkout Session
		http.Redirect(w, r, s.URL, http.StatusSeeOther)

	}

	return err
}

type errResp struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

func writeJSON(w http.ResponseWriter, v interface{}, err error) {
	var respVal interface{}
	if err != nil {
		msg := err.Error()
		var serr *stripe.Error
		if errors.As(err, &serr) {
			msg = serr.Msg
		}
		w.WriteHeader(http.StatusBadRequest)
		var e errResp
		e.Error.Message = msg
		respVal = e
	} else {
		respVal = v
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(respVal); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Error(log.V{"json.NewEncoder.Encode: %v": err})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err := io.Copy(w, &buf); err != nil {
		log.Error(log.V{"io.Copy: %v": err})
		return
	}
}
