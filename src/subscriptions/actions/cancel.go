package actions

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/abishekmuthian/open-payment-host/src/lib/mux"
	"github.com/abishekmuthian/open-payment-host/src/lib/server"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
	"github.com/abishekmuthian/open-payment-host/src/lib/session"
	"github.com/abishekmuthian/open-payment-host/src/lib/view"
	"github.com/abishekmuthian/open-payment-host/src/products"
	"github.com/abishekmuthian/open-payment-host/src/subscriptions"
)

// HandlePaymentCancel handles the success routine of the payment
func HandlePaymentCancel(w http.ResponseWriter, r *http.Request) error {

	// Authorise
	currentUser := session.CurrentUser(w, r)

	// Fetch the  params
	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

	// Get the subscription ID from the request
	subscriptionId := params.Get("subscription_id")
	log.Info(log.V{"Subscription ID: ": subscriptionId})

	redirectURI := params.Get("redirect_uri")
	customId := params.Get("custom_id")

	// Find the subscription in the database
	subscription, err := subscriptions.FindSubscription(subscriptionId)

	if err != nil || subscription == nil {
		log.Error(log.V{"Error finding subscription": err})
		return server.InternalError(err)
	}

	customIdInt, err := strconv.ParseInt(customId, 10, 64)
	if err != nil {
		log.Error(log.V{"Error parsing custom_id to int64": err})
		return server.InternalError(err)
	}

	if customIdInt != subscription.UserId {
		log.Error(log.V{"Invalid custom_id for the subscription": err})
		return server.InternalError(errors.New("Invalid custom_id for the subscription"))
	}

	pg := subscription.PaymentGateway
	log.Info(log.V{"Payment Gateway: ": pg})
	switch pg {
	case "paypal":
		// Handle PayPal subscription cancellation
		err := subscriptions.CancelPaypalSubscription(subscriptionId)
		if err != nil {
			log.Error(log.V{"Error cancelling PayPal subscription": err})
			return server.InternalError(err)
		}
	case "razorpay":
		// Handle Razorpay subscription cancellation
		err := subscriptions.CancelRazorpaySubscription(subscriptionId)
		if err != nil {
			log.Error(log.V{"Error cancelling Razorpay subscription": err})
			return server.InternalError(err)
		}

	default:
		log.Error(log.V{"Error unknown payment gateway": err})
	}

	product, err := products.Find(subscription.ProductId)

	if err != nil {
		log.Error(log.V{"Error finding product": err})
	} else {
		if product.WebhookURL != "" && product.WebhookSecret != "" {
			params := map[string]interface{}{
				"subscription_id": subscriptionId,
				"custom_id":       strconv.FormatInt(subscription.UserId, 10),
				"status":          "cancelled",
			}

			go func() {
				err := subscriptions.SendWebhook(product.WebhookURL, product.WebhookSecret, params)
				if err != nil {
					log.Error(log.V{"Cancel, Error sending webhook to product's URL": err})
				} else {
					log.Info(log.V{"msg": "Successfully sent webhook to product's URL"})
				}
			}()
		}
		return server.RedirectExternal(w, r, redirectURI)
	}

	// Render the template
	view := view.NewRenderer(w, r)
	view.AddKey("currentUser", currentUser)
	// Set the name and year
	view.AddKey("name", config.Get("name"))
	view.AddKey("year", time.Now().Year())

	view.Template("subscriptions/views/payment_cancel.html.got")

	return view.Render()
}
