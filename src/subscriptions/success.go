package subscriptions

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/abishekmuthian/open-payment-host/src/lib/mux"
	"github.com/abishekmuthian/open-payment-host/src/lib/s3"
	"github.com/abishekmuthian/open-payment-host/src/lib/server"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
	"github.com/abishekmuthian/open-payment-host/src/lib/session"
	"github.com/abishekmuthian/open-payment-host/src/lib/view"
	"github.com/abishekmuthian/open-payment-host/src/products"
	"github.com/razorpay/razorpay-go/utils"
)

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

// buildRedirectURL constructs a properly formatted redirect URL by appending parameters
// It handles existing query strings and sanitizes multiple ? into proper & separators
func buildRedirectURL(baseURL string, params map[string]string) string {
	// Replace any occurrence of multiple ? with & (except the first one)
	url := baseURL
	parts := strings.Split(url, "?")
	if len(parts) > 1 {
		// Keep first part and first ?, join rest with &
		url = parts[0] + "?" + strings.Join(parts[1:], "&")
	}

	// Determine separator for new parameters
	separator := "&"
	if !strings.Contains(url, "?") {
		separator = "?"
	}

	// Append new parameters
	for key, value := range params {
		url += separator + key + "=" + value
		separator = "&"
	}

	return url
}

// HandlePaymentSuccess handles the success routine of the payment
func HandlePaymentSuccess(w http.ResponseWriter, r *http.Request) error {

	// Authorise
	currentUser := session.CurrentUser(w, r)
	log.Info(log.V{"Payment Success, User ID: ": currentUser.UserID()})

	// Get the params
	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

	productId := params.GetInt("product_id")

	paypalOrderId := params.Get("paypal_orderid")
	paypalSubscriptionId := params.Get("paypal_subscriptionid")
	redirectURI := params.Get("redirect_uri")
	customId := params.Get("custom_id")

	razorpayPaymentId := params.Get("razorpay_payment_id")
	razorpayOrderId := params.Get("razorpay_order_id")
	razorpaySubscriptionId := params.Get("razorpay_subscription_id")
	razorpaySignature := params.Get("razorpay_signature")

	if razorpayOrderId != "" && razorpayOrderId != "null" && razorpayPaymentId != "" && razorpayPaymentId != "null" && razorpaySignature != "" && razorpaySignature != "null" {
		razorpayParams := map[string]interface{}{
			"razorpay_order_id":   razorpayOrderId,
			"razorpay_payment_id": razorpayPaymentId,
		}

		razorpayOrderCompleted := utils.VerifyPaymentSignature(razorpayParams, razorpaySignature, config.Get("razorpay_key_secret"))

		if razorpayOrderCompleted {
			log.Info(log.V{"Razorpay order completed": razorpayOrderId})
			if redirectURI != "null" && customId != "null" { // Because the request is from JavaScript
				params := map[string]string{
					"custom_id": customId,
					"order_id":  razorpayOrderId,
				}
				return server.RedirectExternal(w, r, buildRedirectURL(redirectURI, params))
			}
		} else {
			log.Error(log.V{"Razorpay order verification failed": razorpayOrderId})
			return server.InternalError(errors.New("razorpay Order verification failed"))
		}
	} else if razorpaySubscriptionId != "" && razorpaySubscriptionId != "null" && razorpayPaymentId != "" && razorpayPaymentId != "null" && razorpaySignature != "" && razorpaySignature != "null" {
		razorpayParams := map[string]interface{}{
			"razorpay_subscription_id": razorpaySubscriptionId,
			"razorpay_payment_id":      razorpayPaymentId,
		}

		razorpayOrderCompleted := utils.VerifySubscriptionSignature(razorpayParams, razorpaySignature, config.Get("razorpay_key_secret"))

		if razorpayOrderCompleted {
			log.Info(log.V{"Razorpay subscription completed": razorpayOrderId})

			// Send webhook if available
			product, err := products.Find(productId)
			if err != nil {
				log.Error(log.V{"Success, Error finding product": err})
				return server.InternalError(err)
			}

			if product.WebhookURL != "" && product.WebhookSecret != "" {
				params := map[string]interface{}{
					"subscription_id": razorpaySubscriptionId,
					"custom_id":       customId,
					"status":          "active",
					"email":           "",
				}

				go func() {
					err := SendWebhook(product.WebhookURL, product.WebhookSecret, params)
					if err != nil {
						log.Error(log.V{"Razorpay webhook, Error sending webhook to product's URL": err})
					} else {
						log.Info(log.V{"msg": "Successfully sent webhook to product's URL"})
					}
				}()
			}

			if (redirectURI != "" && redirectURI != "null") && (customId != "" && customId != "null") {
				params := map[string]string{
					"custom_id":       customId,
					"subscription_id": razorpaySubscriptionId,
				}
				return server.RedirectExternal(w, r, buildRedirectURL(redirectURI, params))
			}
		} else {
			log.Error(log.V{"Razorpay subscription verification failed": razorpayOrderId})
			return server.InternalError(errors.New("razorpay subscription verification failed"))
		}
	}

	var paypalOrderCompleted, paypalSubscriptionCompleted bool

	if paypalOrderId != "" && paypalOrderId != "null" {
		paypalOrderCompleted, err = IsPayPalOrderValid(paypalOrderId)

		if err != nil {
			log.Error(log.V{"Error checking paypal order details": err})
		}
	}

	if paypalSubscriptionId != "" && paypalSubscriptionId != "null" {
		paypalSubscriptionCompleted, err = IsPaypalSubscriptionValid(paypalSubscriptionId)
		if err != nil {
			log.Error(log.V{"Error checking paypal subscription transaction details": err})
		}
	}

	// FIXME: Right now only transaction completion status is checked, check against customId if
	if paypalOrderCompleted || paypalSubscriptionCompleted {
		log.Info(log.V{"Paypal Order/Subscription Completed: ": paypalOrderId})

		// Handle the download file
		// FIXME: Implement a better way to check if download file is enabled

		product, err := products.Find(productId)

		if err != nil {
			return server.InternalError(err)
		}
		if product.S3Bucket != "" && product.S3Key != "" {

			downloadUrl, err := s3.GeneratePresignedUrl(product.S3Bucket, product.S3Key)

			if err == nil {
				return server.RedirectExternal(w, r, downloadUrl)
			}
		}

		if paypalSubscriptionCompleted {
			// Send webhook if available
			product, err := products.Find(productId)
			if err != nil {
				log.Error(log.V{"Success, Error finding product": err})
				return server.InternalError(err)
			}

			if product.WebhookURL != "" && product.WebhookSecret != "" {
				params := map[string]interface{}{
					"subscription_id": paypalSubscriptionId,
					"custom_id":       customId,
					"status":          "active",
					"email":           "",
				}

				go func() {
					err := SendWebhook(product.WebhookURL, product.WebhookSecret, params)
					if err != nil {
						log.Error(log.V{"Razorpay webhook, Error sending webhook to product's URL": err})
					} else {
						log.Info(log.V{"msg": "Successfully sent webhook to product's URL"})
					}
				}()
			}
		}

		if (redirectURI != "" && redirectURI != "null") && (customId != "" && customId != "null") {
			if paypalOrderCompleted {
				params := map[string]string{
					"custom_id": customId,
					"order_id":  paypalOrderId,
				}
				return server.RedirectExternal(w, r, buildRedirectURL(redirectURI, params))
			} else if paypalSubscriptionCompleted {
				params := map[string]string{
					"custom_id":       customId,
					"subscription_id": paypalSubscriptionId,
				}
				return server.RedirectExternal(w, r, buildRedirectURL(redirectURI, params))
			}
		}
	}

	// Render the template
	view := view.NewRenderer(w, r)
	view.AddKey("currentUser", currentUser)
	// Set the name and year
	view.AddKey("name", config.Get("name"))
	view.AddKey("year", time.Now().Year())

	view.Template("subscriptions/views/payment_success.html.got")

	return view.Render()
}
