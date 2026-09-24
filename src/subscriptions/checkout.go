package subscriptions

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/abishekmuthian/open-payment-host/src/lib/mux"
	"github.com/abishekmuthian/open-payment-host/src/lib/server"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
	"github.com/abishekmuthian/open-payment-host/src/lib/session"
	"github.com/abishekmuthian/open-payment-host/src/products"
	"github.com/stripe/stripe-go/v72"
	"github.com/stripe/stripe-go/v72/price"
)

// HandleCreateCheckoutSession creates a Stripe Checkout Session for the product.
// The configured Price ID is derived from the product, country and schedule on
// the server - the browser cannot pair an independent priceId with an
// unrelated productId. The attempt id and product id are stored in Checkout
// metadata and verified again at success and webhook time.
func HandleCreateCheckoutSession(w http.ResponseWriter, r *http.Request) error {

	// Set your secret key. Remember to switch to your live secret key in production.
	// See your keys here: https://dashboard.stripe.com/account/apikeys

	if r.Method != "POST" {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return nil
	}

	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

	// Check token authenticity
	err = session.CheckAuthenticity(w, r)
	if err != nil {
		return err
	}

	// Anon users are allowed to subscribe

	// Resolve the product from the posted product id
	productId, err := strconv.ParseInt(params.Get("productId"), 10, 64)
	if err != nil {
		return server.InternalError(err)
	}
	story, err := products.Find(productId)
	if err != nil {
		return server.InternalError(err)
	}

	// Get the client country
	clientCountry := r.Header.Get("CF-IPCountry")
	log.Info(log.V{"Subscription, Client Country": clientCountry})
	if !config.Production() {
		// There will be no CF request header in the development/test
		clientCountry = config.Get("subscription_client_country")
	}

	taxCountry := clientCountry

	// Derive the configured Price ID from the product, never from the browser.
	// A posted priceId has no authority over the charged price.
	priceId := story.StripePrice[clientCountry]
	if priceId == "" {
		clientCountry = "DF"
		priceId = story.StripePrice[clientCountry]
	}

	if priceId == "" {
		log.Error(log.V{"Checkout, no stripe price configured for product": story.ID, "country": clientCountry})
		return server.Redirect(w, r, "/subscriptions/failure?errorDetail=No+price+configured+for+your+region")
	}

	// Fetch the price to determine recurring vs one time and freeze the
	// expected amount and currency in the payment attempt
	priceClient := price.Client{B: stripe.GetBackend(stripe.APIBackend), Key: config.Get("stripe_secret")}
	p, err := priceClient.Get(priceId, nil)
	if err != nil {
		log.Error(log.V{"Checkout, error fetching stripe price": err})
		return server.InternalError(err)
	}

	if !p.Active || p.UnitAmount <= 0 || (story.Schedule == "onetime" && p.Type != "one_time") || (story.Schedule != "onetime" && p.Type != "recurring") {
		return server.BadRequestError(errors.New("price schedule mismatch"))
	}
	if p.Recurring != nil {
		interval := "month"
		if story.Schedule == "yearly" {
			interval = "year"
		}
		if string(p.Recurring.Interval) != interval || p.Recurring.IntervalCount != 1 {
			return server.BadRequestError(errors.New("price interval mismatch"))
		}
	}
	expectedAmount, err := stripeExpectedAmount(p.UnitAmount, config.Get(fmt.Sprintf("stripe_tax_rate_%s", taxCountry)))
	if err != nil {
		return server.InternalError(err)
	}
	// Subscription or One Time Payment
	var mode *string
	var taxRate []*string
	var subscriptionData *stripe.CheckoutSessionSubscriptionDataParams
	schedule := "onetime"

	if p.Type == "recurring" {
		mode = stripe.String(string(stripe.CheckoutSessionModeSubscription))
		schedule = story.Schedule
		if config.Get(fmt.Sprintf("stripe_tax_rate_%s", taxCountry)) != "" {
			subscriptionData = &stripe.CheckoutSessionSubscriptionDataParams{
				DefaultTaxRates: stripe.StringSlice([]string{
					config.Get(fmt.Sprintf("stripe_tax_rate_%s", taxCountry)),
				}),
			}
		}
	} else if p.Type == "one_time" {
		mode = stripe.String(string(stripe.CheckoutSessionModePayment))
		if config.Get(fmt.Sprintf("stripe_tax_rate_%s", taxCountry)) != "" {
			taxRate = stripe.StringSlice([]string{
				config.Get(fmt.Sprintf("stripe_tax_rate_%s", taxCountry)),
			})
		}
	} else {
		return server.Redirect(w, r, "/subscriptions/failure?errorDetail=Unsupported+price+type")
	}

	// Create the immutable payment attempt with the server-side price
	attempt, err := newAttempt(story.ID, "stripe", schedule, clientCountry, expectedAmount, string(p.Currency), priceId, "", "")
	if err != nil {
		log.Error(log.V{"Checkout, error creating payment attempt": err})
		return server.InternalError(err)
	}

	successURL := stripe.String(config.Get("stripe_callback_domain") + "/subscriptions/stripe-success?session_id={CHECKOUT_SESSION_ID}")

	sessionParams := &stripe.CheckoutSessionParams{
		BillingAddressCollection: stripe.String("required"),
		CancelURL:                stripe.String(config.Get("stripe_callback_domain") + "/subscriptions/failure"),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Price: stripe.String(priceId),
				// For metered billing, do not pass quantity
				Quantity: stripe.Int64(1),
				TaxRates: taxRate,
			},
		},
		Mode:               mode,
		PaymentMethodTypes: stripe.StringSlice([]string{"card"}),
		SubscriptionData:   subscriptionData,
		SuccessURL:         successURL,
	}

	if schedule != "onetime" {
		if sessionParams.SubscriptionData == nil {
			sessionParams.SubscriptionData = &stripe.CheckoutSessionSubscriptionDataParams{}
		}
		sessionParams.SubscriptionData.AddMetadata("attempt_id", attempt.Id)
	}
	sessionParams.AddMetadata("plan", story.NameDisplay())
	sessionParams.AddMetadata("product_id", strconv.FormatInt(story.ID, 10))
	sessionParams.AddMetadata("attempt_id", attempt.Id)

	client := stripeSessions()
	s, err := client.New(sessionParams)
	if err != nil {
		return server.InternalError(err)
	}

	// Store the checkout session id in the attempt
	if err := attempt.SetProviderIds(s.ID, "", ""); err != nil {
		log.Error(log.V{"Checkout, error storing session id in attempt": err})
		return server.InternalError(err)
	}

	// Bind the attempt to this browser
	setAttemptCookie(w, r, attempt)

	// Then redirect to the URL on the Checkout Session
	http.Redirect(w, r, s.URL, http.StatusSeeOther)

	return nil
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
