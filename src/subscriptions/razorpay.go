package subscriptions

import (
	"errors"
	"net/http"
	"time"

	"github.com/abishekmuthian/open-payment-host/src/lib/mux"
	"github.com/abishekmuthian/open-payment-host/src/lib/server"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
	"github.com/abishekmuthian/open-payment-host/src/lib/session"
	"github.com/abishekmuthian/open-payment-host/src/lib/view"
	"github.com/abishekmuthian/open-payment-host/src/products"
	razorpay "github.com/razorpay/razorpay-go"
)

func HandleRazorpayShow(w http.ResponseWriter, r *http.Request) error {
	// Fetch the  params
	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

	// Get current user
	currentUser := session.CurrentUser(w, r)

	productId := params.GetInt("product_id")
	customerName := params.Get("customer_name")
	customerEmail := params.Get("customer_email")
	subscriptionId := params.Get("subscription_id")

	product, err := products.Find(productId)
	if err != nil {
		// Handle the error appropriately
		log.Error(log.V{"Error finding product with ID": productId, "error": err})
		return server.InternalError(err)
	}

	// Get the country from IP
	clientCountry := r.Header.Get("CF-IPCountry")
	if !config.Production() {
		// There will be no CF request header in the development/test
		clientCountry = config.Get("subscription_client_country")
	}

	log.Info(log.V{"Subscription, Client Country": clientCountry})

	// Render the template
	view := view.NewRenderer(w, r)

	view.AddKey("currentUser", currentUser)

	switch product.Schedule {
	case "onetime":
		var amount interface{}
		var currency interface{}

		// Check if price exists for client country
		if priceMap, exists := product.RazorpayPrice[clientCountry]; exists && priceMap != nil {
			amount = priceMap["amount"]
			currency = priceMap["currency"]
		}

		// If there is no amount for the client country then get the amount for default country
		if amount == nil || currency == nil {
			clientCountry = "DF"
			if priceMap, exists := product.RazorpayPrice[clientCountry]; exists && priceMap != nil {
				amount = priceMap["amount"]
				currency = priceMap["currency"]
			}
		}

		// Check if amount and currency are still nil after fallback
		if amount == nil || currency == nil {
			log.Error(log.V{"Razorpay price not configured for product": productId})
			return server.InternalError(errors.New("razorpay price not configured for this product"))
		}

		// Convert the amount string to integer and multiply by 100 to get the amount in paisa
		amountInt := int(amount.(float64))

		amountInt = amountInt * 100

		// Create Order ID
		client := razorpay.NewClient(config.Get("razorpay_key_id"), config.Get("razorpay_key_secret"))

		data := map[string]interface{}{
			"amount":   amountInt, // Amount is in currency subunits. Default currency is INR. Hence, 50000 refers to 50000 paise
			"currency": currency,
		}
		order, err := client.Order.Create(data, nil)

		if err != nil {
			log.Error(log.V{"Error creating Razorpay order": err})
			return server.InternalError(err)
		}

		if order == nil || order["id"] == nil {
			log.Error(log.V{"Razorpay Order ID is nil": err})
			return server.InternalError(err)
		}
		view.AddKey("meta_product_amount", amountInt)
		view.AddKey("meta_product_currency", currency)
		view.AddKey("meta_product_order_id", order["id"])
		view.AddKey("meta_payment_script_type", "checkout")

	case "monthly", "yearly":
		// Subscription ID retrieved from product
		view.AddKey("meta_product_subscription_ID", subscriptionId)
		view.AddKey("meta_payment_script_type", "subscription")
	}

	if customerName != "" {
		view.AddKey("customerName", customerName)
	}
	if customerEmail != "" {
		view.AddKey("customerEmail", customerEmail)
	}
	view.AddKey("loadRazorpayScript", true)

	view.AddKey("loadHypermedia", true)
	view.AddKey("story", product)
	view.AddKey("loadSweetAlert", true)
	view.AddKey("meta_product_id", productId)

	view.AddKey("meta_product_title", product.Name)
	view.AddKey("meta_razorpay_key_id", config.Get("razorpay_key_id"))
	view.AddKey("clientCountry", clientCountry)

	// Set the name and year
	view.AddKey("name", config.Get("name"))
	view.AddKey("year", time.Now().Year())
	return view.Render()
}

func CancelRazorpaySubscription(subscriptionId string) error {
	client := razorpay.NewClient(config.Get("razorpay_key_id"), config.Get("razorpay_key_secret"))

	data := map[string]interface{}{

		"cancel_at_cycle_end": true,
	}

	body, err := client.Subscription.Cancel(subscriptionId, data, nil)

	if err != nil {
		log.Error(log.V{"Error cancelling Razorpay subscription": err})
		return err
	}

	if body["status"] == "active" {
		log.Info(log.V{"Razorpay subscription will be canceled at cycle end": body})
		return nil
	} else {
		log.Error(log.V{"Razorpay subscription not canceled": body})
		return errors.New("razorpay subscription not canceled")
	}

}
