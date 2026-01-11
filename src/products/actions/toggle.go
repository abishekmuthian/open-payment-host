package storyactions

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/abishekmuthian/open-payment-host/src/lib/mux"
	"github.com/abishekmuthian/open-payment-host/src/lib/server"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
	"github.com/abishekmuthian/open-payment-host/src/lib/session"
	"github.com/abishekmuthian/open-payment-host/src/lib/view"
	"github.com/abishekmuthian/open-payment-host/src/products"
)

// HandleTogglePaypal handles toggle on/off for PayPal payment gateway
// Responds to post /products/toggle/paypal
func HandleTogglePaypal(w http.ResponseWriter, r *http.Request) error {
	// Check the authenticity token
	err := session.CheckAuthenticity(w, r)
	if err != nil {
		return server.NotAuthorizedError(err)
	}

	// Get the params
	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

	// When checkbox is checked, it sends "on", when unchecked it's not included
	checked := params.Get("paypal-toggle")
	schedule := params.Get("schedule")

	log.Info(log.V{
		"Handler":  "PayPal Toggle",
		"checked":  checked,
		"schedule": schedule,
		"isEmpty":  checked == "",
	})

	// If unchecked (empty or missing), return empty content
	if checked == "" {
		log.Info(log.V{"PayPal": "RETURNING EMPTY"})
		w.WriteHeader(http.StatusOK)
		_, err = w.Write([]byte("<!-- EMPTY -->"))
		return err
	}

	// If checked, return the pricing fields based on schedule
	log.Info(log.V{"PayPal": "RETURNING FIELDS", "schedule": schedule})
	view := view.NewRenderer(w, r)
	view.AddKey("schedule", schedule)
	view.AddKey("paypal", config.GetBool("paypal"))

	view.Template("products/views/paypal_toggle.html.got")
	view.Layout("")

	return view.Render()
}

// HandleToggleRazorpay handles toggle on/off for Razorpay payment gateway
// Responds to post /products/toggle/razorpay
func HandleToggleRazorpay(w http.ResponseWriter, r *http.Request) error {
	log.Info(log.V{"Handler": "Razorpay Toggle - START"})

	// Check the authenticity token
	err := session.CheckAuthenticity(w, r)
	if err != nil {
		log.Error(log.V{"Razorpay": "Auth error", "error": err})
		return server.NotAuthorizedError(err)
	}

	// Get the params
	params, err := mux.Params(r)
	if err != nil {
		log.Error(log.V{"Razorpay": "Params error", "error": err})
		return server.InternalError(err)
	}

	// When checkbox is checked, it sends "on", when unchecked it's not included
	checked := params.Get("razorpay-toggle")
	schedule := params.Get("schedule")

	log.Info(log.V{
		"Handler":   "Razorpay Toggle",
		"checked":   checked,
		"schedule":  schedule,
		"isEmpty":   checked == "",
		"allParams": params,
	})

	// If unchecked (empty or missing), return empty content
	if checked == "" {
		log.Info(log.V{"Razorpay": "RETURNING EMPTY"})
		w.WriteHeader(http.StatusOK)
		_, err = w.Write([]byte(""))
		return err
	}

	// If checked, return the pricing fields based on schedule
	log.Info(log.V{"Razorpay": "RETURNING FIELDS", "schedule": schedule})
	view := view.NewRenderer(w, r)
	view.AddKey("schedule", schedule)
	view.AddKey("razorpay", config.GetBool("razorpay"))

	view.Template("products/views/razorpay_toggle.html.got")
	view.Layout("")

	return view.Render()
}

// HandleTogglePaypalUpdate handles toggle on/off for PayPal in update page
// Responds to post /products/{id:[0-9]+}/toggle/paypal
func HandleTogglePaypalUpdate(w http.ResponseWriter, r *http.Request) error {
	// Check the authenticity token
	err := session.CheckAuthenticity(w, r)
	if err != nil {
		return server.NotAuthorizedError(err)
	}

	// Get the params
	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

	// When checkbox is checked, it sends "on", when unchecked it's not included
	checked := params.Get("paypal-toggle")

	// If unchecked, return empty content
	if checked == "" {
		w.WriteHeader(http.StatusOK)
		_, err = w.Write([]byte(""))
		return err
	}

	// If checked, we need to render the fields with existing product data
	// For now, just trigger a page reload or return the static template
	// The data is already rendered server-side, so we just return the div content

	// Get the product ID
	id := params.GetInt("id")

	// Find the product to get pricing data
	product, err := products.Find(id)
	if err != nil {
		return server.NotFoundError(err)
	}

	schedule := params.Get("schedule")

	log.Info(log.V{
		"Handler":         "PayPalUpdate Toggle",
		"productID":       id,
		"requestSchedule": schedule,
		"dbSchedule":      product.Schedule,
		"schedulesMatch":  schedule == product.Schedule,
	})

	view := view.NewRenderer(w, r)
	view.AddKey("schedule", schedule)
	view.AddKey("story", product)
	view.AddKey("paypal", config.GetBool("paypal"))
	view.AddKey("fieldIndex", 0)

	// Only load existing pricing data if the schedule hasn't changed
	if schedule == product.Schedule {
		// Parse existing PayPal pricing data
		paypalPriceJSON, err := json.Marshal(product.PaypalPrice)
		if err == nil {
			var paypalPrices map[string]map[string]interface{}
			err := json.Unmarshal([]byte(paypalPriceJSON), &paypalPrices)
			if err != nil {
				log.Error(log.V{"Error unmarshalling PayPal JSON:": err})
				return err
			}
			log.Info(log.V{"PayPal": "Loading existing prices", "count": len(paypalPrices)})
			view.AddKey("paypalPrices", paypalPrices)
		} else {
			log.Info(log.V{"PayPal": "Marshal error, empty prices"})
			view.AddKey("paypalPrices", make(map[string]map[string]interface{}))
		}
	} else {
		// Schedule changed, show empty fields
		log.Info(log.V{"PayPal": "Schedule mismatch, returning empty fields"})
		view.AddKey("paypalPrices", make(map[string]map[string]interface{}))
	}

	// Add sorted countries
	countryMap := CreateCountryMap()
	var countries []Country
	for code, name := range countryMap {
		countries = append(countries, Country{Code: code, Name: name})
	}
	sort.Sort(ByName(countries))
	view.AddKey("sortedCountries", countries)

	view.Template("products/views/paypal_toggle_update.html.got")
	view.Layout("")

	return view.Render()
}

// HandleToggleStripe handles toggle on/off for Stripe payment gateway
// Responds to post /products/toggle/stripe
func HandleToggleStripe(w http.ResponseWriter, r *http.Request) error {
	// Check the authenticity token
	err := session.CheckAuthenticity(w, r)
	if err != nil {
		return server.NotAuthorizedError(err)
	}

	// Get the params
	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

	// When checkbox is checked, it sends "on", when unchecked it's not included
	checked := params.Get("stripe-toggle")
	schedule := params.Get("schedule")

	log.Info(log.V{
		"Handler":  "Stripe Toggle",
		"checked":  checked,
		"schedule": schedule,
		"isEmpty":  checked == "",
	})

	// If unchecked (empty or missing), return empty content
	if checked == "" {
		log.Info(log.V{"Stripe": "RETURNING EMPTY"})
		w.WriteHeader(http.StatusOK)
		_, err = w.Write([]byte(""))
		return err
	}

	// If checked, return the pricing fields based on schedule
	log.Info(log.V{"Stripe": "RETURNING FIELDS", "schedule": schedule})
	view := view.NewRenderer(w, r)
	view.AddKey("schedule", schedule)
	view.AddKey("stripe", config.GetBool("stripe"))

	view.Template("products/views/stripe_toggle.html.got")
	view.Layout("")

	return view.Render()
}

// HandleToggleSquare handles toggle on/off for Square payment gateway
// Responds to post /products/toggle/square
func HandleToggleSquare(w http.ResponseWriter, r *http.Request) error {
	// Check the authenticity token
	err := session.CheckAuthenticity(w, r)
	if err != nil {
		return server.NotAuthorizedError(err)
	}

	// Get the params
	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

	// When checkbox is checked, it sends "on", when unchecked it's not included
	checked := params.Get("square-toggle")
	schedule := params.Get("schedule")

	log.Info(log.V{
		"Handler":  "Square Toggle",
		"checked":  checked,
		"schedule": schedule,
		"isEmpty":  checked == "",
	})

	// If unchecked (empty or missing), return empty content
	if checked == "" {
		log.Info(log.V{"Square": "RETURNING EMPTY"})
		w.WriteHeader(http.StatusOK)
		_, err = w.Write([]byte(""))
		return err
	}

	// If checked, return the pricing fields based on schedule
	log.Info(log.V{"Square": "RETURNING FIELDS", "schedule": schedule})
	view := view.NewRenderer(w, r)
	view.AddKey("schedule", schedule)
	view.AddKey("square", config.GetBool("square"))

	view.Template("products/views/square_toggle.html.got")
	view.Layout("")

	return view.Render()
}

// HandleToggleAPI handles toggle on/off for API webhook
// Responds to post /products/toggle/api
func HandleToggleAPI(w http.ResponseWriter, r *http.Request) error {
	// Check the authenticity token
	err := session.CheckAuthenticity(w, r)
	if err != nil {
		return server.NotAuthorizedError(err)
	}

	// Get the params
	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

	// When checkbox is checked, it sends "on", when unchecked it's not included
	checked := params.Get("api-toggle")

	log.Info(log.V{
		"Handler": "API Toggle",
		"checked": checked,
		"isEmpty": checked == "",
	})

	// If unchecked (empty or missing), return empty content
	if checked == "" {
		log.Info(log.V{"API": "RETURNING EMPTY"})
		w.WriteHeader(http.StatusOK)
		_, err = w.Write([]byte(""))
		return err
	}

	// If checked, return the webhook fields
	log.Info(log.V{"API": "RETURNING FIELDS"})
	view := view.NewRenderer(w, r)

	view.Template("products/views/api_toggle.html.got")
	view.Layout("")

	return view.Render()
}

// HandleToggleRazorpayUpdate handles toggle on/off for Razorpay in update page
// Responds to post /products/{id:[0-9]+}/toggle/razorpay
func HandleToggleRazorpayUpdate(w http.ResponseWriter, r *http.Request) error {
	// Check the authenticity token
	err := session.CheckAuthenticity(w, r)
	if err != nil {
		return server.NotAuthorizedError(err)
	}

	// Get the params
	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

	// When checkbox is checked, it sends "on", when unchecked it's not included
	checked := params.Get("razorpay-toggle")

	// If unchecked, return empty content
	if checked == "" {
		w.WriteHeader(http.StatusOK)
		_, err = w.Write([]byte(""))
		return err
	}

	// Get the product ID
	id := params.GetInt("id")

	// Find the product to get pricing data
	product, err := products.Find(id)
	if err != nil {
		return server.NotFoundError(err)
	}

	schedule := params.Get("schedule")

	log.Info(log.V{
		"Handler":         "RazorpayUpdate Toggle",
		"productID":       id,
		"requestSchedule": schedule,
		"dbSchedule":      product.Schedule,
		"schedulesMatch":  schedule == product.Schedule,
	})

	view := view.NewRenderer(w, r)
	view.AddKey("schedule", schedule)
	view.AddKey("story", product)
	view.AddKey("razorpay", config.GetBool("razorpay"))
	view.AddKey("fieldIndex", 0)

	// Only load existing pricing data if the schedule hasn't changed
	if schedule == product.Schedule {
		// Parse existing Razorpay pricing data
		razorpayPriceJSON, err := json.Marshal(product.RazorpayPrice)
		if err == nil {
			var razorpayPrices map[string]map[string]interface{}
			err := json.Unmarshal([]byte(razorpayPriceJSON), &razorpayPrices)
			if err != nil {
				log.Error(log.V{"Error unmarshalling Razorpay JSON:": err})
				return err
			}
			log.Info(log.V{"Razorpay": "Loading existing prices", "count": len(razorpayPrices)})
			view.AddKey("razorpayPrices", razorpayPrices)
		} else {
			log.Info(log.V{"Razorpay": "Marshal error, empty prices"})
			view.AddKey("razorpayPrices", make(map[string]map[string]interface{}))
		}
	} else {
		// Schedule changed, show empty fields
		log.Info(log.V{"Razorpay": "Schedule mismatch, returning empty fields"})
		view.AddKey("razorpayPrices", make(map[string]map[string]interface{}))
	}

	// Add sorted countries
	countryMap := CreateCountryMap()
	var countries []Country
	for code, name := range countryMap {
		countries = append(countries, Country{Code: code, Name: name})
	}
	sort.Sort(ByName(countries))
	view.AddKey("sortedCountries", countries)

	view.Template("products/views/razorpay_toggle_update.html.got")
	view.Layout("")

	return view.Render()
}

// HandleToggleStripeUpdate handles toggle on/off for Stripe in update page
// Responds to post /products/{id:[0-9]+}/toggle/stripe
func HandleToggleStripeUpdate(w http.ResponseWriter, r *http.Request) error {
	// Check the authenticity token
	err := session.CheckAuthenticity(w, r)
	if err != nil {
		return server.NotAuthorizedError(err)
	}

	// Get the params
	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

	// When checkbox is checked, it sends "on", when unchecked it's not included
	checked := params.Get("stripe-toggle")

	// If unchecked, return empty content
	if checked == "" {
		w.WriteHeader(http.StatusOK)
		_, err = w.Write([]byte(""))
		return err
	}

	// Get the product ID
	id := params.GetInt("id")

	// Find the product to get pricing data
	product, err := products.Find(id)
	if err != nil {
		return server.NotFoundError(err)
	}

	schedule := params.Get("schedule")

	log.Info(log.V{
		"Handler":         "StripeUpdate Toggle",
		"productID":       id,
		"requestSchedule": schedule,
		"dbSchedule":      product.Schedule,
		"schedulesMatch":  schedule == product.Schedule,
	})

	view := view.NewRenderer(w, r)
	view.AddKey("schedule", schedule)
	view.AddKey("story", product)
	view.AddKey("stripe", config.GetBool("stripe"))
	view.AddKey("fieldIndex", 0)

	// Only load existing pricing data if the schedule hasn't changed
	if schedule == product.Schedule {
		// product.StripePrice is already a Go map, use it directly
		if product.StripePrice != nil && len(product.StripePrice) > 0 {
			log.Info(log.V{"Stripe": "Loading existing prices", "count": len(product.StripePrice)})
			view.AddKey("stripePrices", product.StripePrice)
		} else {
			log.Info(log.V{"Stripe": "No existing prices"})
			view.AddKey("stripePrices", make(map[string]string))
		}
	} else {
		// Schedule changed, show empty fields
		log.Info(log.V{"Stripe": "Schedule mismatch, returning empty fields"})
		view.AddKey("stripePrices", make(map[string]string))
	}

	// Add sorted countries
	countryMap := CreateCountryMap()
	var countries []Country
	for code, name := range countryMap {
		countries = append(countries, Country{Code: code, Name: name})
	}
	sort.Sort(ByName(countries))
	view.AddKey("sortedCountries", countries)

	view.Template("products/views/stripe_toggle_update.html.got")
	view.Layout("")

	return view.Render()
}

// HandleToggleSquareUpdate handles toggle on/off for Square in update page
// Responds to post /products/{id:[0-9]+}/toggle/square
func HandleToggleSquareUpdate(w http.ResponseWriter, r *http.Request) error {
	// Check the authenticity token
	err := session.CheckAuthenticity(w, r)
	if err != nil {
		return server.NotAuthorizedError(err)
	}

	// Get the params
	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

	// When checkbox is checked, it sends "on", when unchecked it's not included
	checked := params.Get("square-toggle")

	// If unchecked, return empty content
	if checked == "" {
		w.WriteHeader(http.StatusOK)
		_, err = w.Write([]byte(""))
		return err
	}

	// Get the product ID
	id := params.GetInt("id")

	// Find the product to get pricing data
	product, err := products.Find(id)
	if err != nil {
		return server.NotFoundError(err)
	}

	schedule := params.Get("schedule")

	log.Info(log.V{
		"Handler":         "SquareUpdate Toggle",
		"productID":       id,
		"requestSchedule": schedule,
		"dbSchedule":      product.Schedule,
		"schedulesMatch":  schedule == product.Schedule,
	})

	view := view.NewRenderer(w, r)
	view.AddKey("schedule", schedule)
	view.AddKey("story", product)
	view.AddKey("square", config.GetBool("square"))
	view.AddKey("fieldIndex", 0)

	// Only load existing pricing data if the schedule hasn't changed
	if schedule == product.Schedule {
		// product.SquarePrice is already a Go map, use it directly
		if product.SquarePrice != nil && len(product.SquarePrice) > 0 {
			log.Info(log.V{"Square": "Loading existing prices", "count": len(product.SquarePrice)})
			view.AddKey("squarePrices", product.SquarePrice)
		} else {
			log.Info(log.V{"Square": "No existing prices"})
			view.AddKey("squarePrices", make(map[string]map[string]interface{}))
		}
	} else {
		// Schedule changed, show empty fields
		log.Info(log.V{"Square": "Schedule mismatch, returning empty fields"})
		view.AddKey("squarePrices", make(map[string]map[string]interface{}))
	}

	// Add sorted countries
	countryMap := CreateCountryMap()
	var countries []Country
	for code, name := range countryMap {
		countries = append(countries, Country{Code: code, Name: name})
	}
	sort.Sort(ByName(countries))
	view.AddKey("sortedCountries", countries)

	view.Template("products/views/square_toggle_update.html.got")
	view.Layout("")

	return view.Render()
}

// HandleToggleAPIUpdate handles toggle on/off for API webhook in update page
// Responds to post /products/{id:[0-9]+}/toggle/api
func HandleToggleAPIUpdate(w http.ResponseWriter, r *http.Request) error {
	// Check the authenticity token
	err := session.CheckAuthenticity(w, r)
	if err != nil {
		return server.NotAuthorizedError(err)
	}

	// Get the params
	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

	// When checkbox is checked, it sends "on", when unchecked it's not included
	checked := params.Get("api-toggle")

	// If unchecked, return empty content
	if checked == "" {
		w.WriteHeader(http.StatusOK)
		_, err = w.Write([]byte(""))
		return err
	}

	// Get the product ID
	id := params.GetInt("id")

	// Find the product to get webhook data
	product, err := products.Find(id)
	if err != nil {
		return server.NotFoundError(err)
	}

	view := view.NewRenderer(w, r)
	view.AddKey("story", product)

	view.Template("products/views/api_toggle_update.html.got")
	view.Layout("")

	return view.Render()
}
