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
			view.AddKey("paypalPrices", paypalPrices)
		} else {
			view.AddKey("paypalPrices", make(map[string]map[string]interface{}))
		}
	} else {
		// Schedule changed, show empty fields
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
			view.AddKey("razorpayPrices", razorpayPrices)
		} else {
			view.AddKey("razorpayPrices", make(map[string]map[string]interface{}))
		}
	} else {
		// Schedule changed, show empty fields
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
