package subscriptions

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/abishekmuthian/open-payment-host/src/lib/mailchimp"
	"github.com/abishekmuthian/open-payment-host/src/lib/query"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
	"github.com/abishekmuthian/open-payment-host/src/products"
	"github.com/plutov/paypal/v4"
)

// HandlePaypalWebhook receives the webhook POST request from the Paypal
func HandlePaypalWebhook(w http.ResponseWriter, r *http.Request) error {
	if r.Method != "POST" {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return nil
	}

	// Determine API base URL based on environment
	apiBase := paypal.APIBaseLive
	if !config.Production() {
		apiBase = paypal.APIBaseSandBox
	}

	// Create a client instance
	c, err := paypal.NewClient(config.Get("paypal_client_id"), config.Get("paypal_client_secret"), apiBase)

	if err != nil {
		log.Error(log.V{"Paypal Client Initialization": err})
	}
	c.SetLog(os.Stdout) // Set log to terminal stdout

	// Check if the event is from Paypal
	ctx := context.Background()
	verifyWebhookResponse, err := c.VerifyWebhookSignature(ctx, r, config.Get("paypal_webhook_id"))
	if err != nil {
		log.Error(log.V{"Paypal Webhook Verification": err})
	}

	log.Info(log.V{"Paypal Webhook Response": verifyWebhookResponse.VerificationStatus})

	if verifyWebhookResponse.VerificationStatus == "SUCCESS" {
		// Signature is valid. Return 200 OK.
		w.WriteHeader(200)
	} else {
		// Signature is invalid
		w.WriteHeader(403)
		log.Error(log.V{"Paypal Webhook": "Invalid signature"})
		return nil
	}

	var paypalWebhookEvent PaypalWebhookEvent

	b, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		log.Error(log.V{"Paypal webhoook, ioutil.ReadAll: %v": err})
		return err
	}

	err = json.Unmarshal(b, &paypalWebhookEvent)

	if err != nil {
		log.Error(log.V{"Paypal Webhook JSON Unmarshall": err})
	}

	log.Info(log.V{"Paypal Webhook Event Parsed": paypalWebhookEvent})

	switch paypalWebhookEvent.EventType {
	case "CHECKOUT.ORDER.APPROVED":
		// Handle order approved event
		log.Info(log.V{"Paypal Checkout Order Approved": paypalWebhookEvent})
		var paypalEventCheckout PaypalEventCheckout

		err = json.Unmarshal(b, &paypalEventCheckout)

		if err != nil {
			log.Error(log.V{"Paypal Webhook Checkout JSON Unmarshall": err})
			return err
		}

		log.Info(log.V{"Paypal Webhook Event Parsed": paypalEventCheckout})

		var subscription *Subscription

		if len(paypalEventCheckout.Resource.PurchaseUnits[0].Payments.Captures) < 1 {
			log.Info(log.V{"msg": "Paypal Webhook,  Payment capture is not present searching using payer email"})
			subscription, err = FindPayment((paypalEventCheckout.Resource.ID))
			if err != nil {
				log.Info(log.V{"Paypal Webhook": "Cannot find the subscription using resource ID"})
			}
		} else if subscription == nil {
			// Only try to find by Capture ID if Captures array has elements
			subscription, err = FindPayment(paypalEventCheckout.Resource.PurchaseUnits[0].Payments.Captures[0].ID)
			if err != nil {
				log.Info(log.V{"Webhook, error finding paypal order in db using Capture Id": err})
			}
		}

		if subscription == nil {
			subscription := New()
			err := recordPaypalCheckoutOrder(paypalEventCheckout, subscription)

			if err != nil {
				log.Error(log.V{"Webhook, error recording paypal order in db": err})
				return err
			}

			// Try to find the created subscription
			if len(paypalEventCheckout.Resource.PurchaseUnits[0].Payments.Captures) > 0 {
				subscription, err = FindPayment(paypalEventCheckout.Resource.PurchaseUnits[0].Payments.Captures[0].ID)
				if err != nil {
					log.Info(log.V{"Webhook, error finding paypal order in db using Capture Id for updating it": err})
				}
			}

			if subscription == nil || err != nil {
				subscription, err = FindPayment(paypalEventCheckout.Resource.ID)

				if err != nil {
					log.Info(log.V{"Webhook, error finding paypal order in db using Resource Id for updating it": err})
					return err
				}
			}
			productId := subscription.ProductId

			product, err := products.Find(productId)
			if err != nil {
				log.Error(log.V{"Webhook, error finding product in db": err})
				return err
			} else {
				// Update counters based on product schedule for CHECKOUT.ORDER.APPROVED events
				if product != nil {
					productParams := make(map[string]string)
					if product.Schedule == "onetime" {
						product.TotalOnetimePayments += 1
						productParams["total_onetime_payments"] = strconv.FormatInt(product.TotalOnetimePayments, 10)
					} else {
						// Monthly or yearly subscription
						product.TotalSubscribers += 1
						productParams["total_subscribers"] = strconv.FormatInt(product.TotalSubscribers, 10)
					}
					err = product.Update(productParams)
					if err != nil {
						log.Error(log.V{"Paypal webhook, Error updating product counters for CHECKOUT.ORDER.APPROVED": err})
					}
				}

				// If mailchimp list id and mailchimp token is available add to the mailchimp list
				if product.MailchimpAudienceID != "" && config.Get("mailchimp_token") != "" {
					// Add to the mailchimp list
					audience := mailchimp.Audience{
						MergeFields: mailchimp.Merge{FirstName: subscription.FirstName},
						Email:       subscription.CustomerEmail,
						Status:      "subscribed",
					}
					go mailchimp.AddToAudience(audience, product.MailchimpAudienceID, mailchimp.GetMD5Hash(subscription.CustomerEmail), config.Get("mailchimp_token"))
				}
			}

			if product.WebhookURL != "" && product.WebhookSecret != "" {
				params := map[string]interface{}{
					"subscription_id": subscription.PaymentId,
					"custom_id":       subscription.UserId,
					"status":          "active",
					"email":           subscription.CustomerEmail,
				}

				go func() {
					err := SendWebhook(product.WebhookURL, product.WebhookSecret, params)
					if err != nil {
						log.Error(log.V{"Paypal webhook, Error sending webhook to product's URL": err})
					} else {
						log.Info(log.V{"msg": "Successfully sent webhook to product's URL"})
					}
				}()
			}

		} else {
			log.Info(log.V{"Webhook, paypal order already exists in db, Order ID": subscription.ID})
		}
	case "PAYMENT.CAPTURE.REFUNDED":
		log.Info(log.V{"Paypal Checkout Order Refunded": paypalWebhookEvent})
		var paypalEventCaptureRefund PaypalEventCaptureRefund

		err = json.Unmarshal(b, &paypalEventCaptureRefund)

		if err != nil {
			log.Error(log.V{"Paypal Webhook Checkout Refund JSON Unmarshall": err})
			return err
		}

		log.Info(log.V{"Paypal Webhook Event Parsed": paypalEventCaptureRefund})

		// Parse paypalEventCaptureRefund.Links[] and find the up link to get the captureid e.g. https://api.sandbox.paypal.com/v2/payments/captures/2K3372465B542845P
		captureId := strings.Split(paypalEventCaptureRefund.Resource.Links[1].Href, "/")[len(strings.Split(paypalEventCaptureRefund.Resource.Links[1].Href, "/"))-1]

		var subscription *Subscription

		subscription, err = FindPayment(captureId)
		if err != nil {
			log.Info(log.V{"Webhook, error finding paypal order in db using Capture Id for updating it": err})
			subscription, err = FindPayerEmail(paypalEventCaptureRefund.Resource.Payer.EmailAddress)
			if err != nil {
				log.Error(log.V{"Webhook, error finding paypal order in db using Payer Email for updating it": err})
				return err
			}
		}

		transactionParams := make(map[string]string)

		transactionParams["payment_status"] = "REFUNDED"

		err := subscription.Update(transactionParams)

		if err != nil {
			log.Error(log.V{"Error updating subscription status in db": err})
		}

	case "BILLING.SUBSCRIPTION.ACTIVATED":
		// Handle subscription activated event
		log.Info(log.V{"Paypal Subscription Created": paypalWebhookEvent})

		var paypalEventSubscription PaypalEventSubscription

		err = json.Unmarshal(b, &paypalEventSubscription)

		if err != nil {
			log.Error(log.V{"Paypal Webhook Checkout JSON Unmarshall": err})
			return err
		}

		log.Info(log.V{"Paypal Webhook Event Parsed": paypalEventSubscription})
		transactionId := paypalEventSubscription.Resource.ID

		var subscription *Subscription

		subscription, err = FindSubscription(transactionId)
		if err != nil {
			log.Error(log.V{"Webhook, error finding paypal subscription in db using Transaction Id for updating it": err})
		}

		if subscription == nil {
			subscription := New()
			err := recordPaypalSubscription(paypalEventSubscription, *subscription)

			if err != nil {
				log.Error(log.V{"Webhook, error recording paypal order in db": err})
				return err
			}

			subscription, err = FindSubscription(transactionId)

			if err == nil {
				// Call the webhook from the product
				productId := subscription.ProductId

				product, err := products.Find(productId)
				if err != nil {
					log.Error(log.V{"Webhook, error finding product in db": err})
					return err
				} else {
					// If mailchimp list id and mailchimp token is available add to the mailchimp list
					if product.MailchimpAudienceID != "" && config.Get("mailchimp_token") != "" {
						// Add to the mailchimp list
						audience := mailchimp.Audience{
							MergeFields: mailchimp.Merge{FirstName: subscription.FirstName},
							Email:       subscription.CustomerEmail,
							Status:      "subscribed",
						}
						go mailchimp.AddToAudience(audience, product.MailchimpAudienceID, mailchimp.GetMD5Hash(subscription.CustomerEmail), config.Get("mailchimp_token"))
					}
					if product.WebhookURL != "" && product.WebhookSecret != "" {
						params := map[string]interface{}{
							"subscription_id": subscription.SubscriptionId,
							"custom_id":       subscription.UserId,
							"status":          "active",
							"email":           subscription.CustomerEmail,
						}

						go func() {
							err := SendWebhook(product.WebhookURL, product.WebhookSecret, params)
							if err != nil {
								log.Error(log.V{"Paypal webhook, Error sending webhook to product's URL": err})
							} else {
								log.Info(log.V{"msg": "Successfully sent webhook to product's URL"})
							}
						}()
					}
				}
			} else {
				log.Error(log.V{"Paypal Webhook, error finding subscription to send webhook": err})
			}

		} else {
			log.Info(log.V{"Webhook, paypal order already exists in db, Order ID": subscription.ID})
		}

	case "BILLING.SUBSCRIPTION.CREATED":
		// Handle subscription created event
		log.Info(log.V{"Paypal Subscription Created": paypalWebhookEvent})

		var paypalEventSubscription PaypalEventSubscription

		err = json.Unmarshal(b, &paypalEventSubscription)

		if err != nil {
			log.Error(log.V{"Paypal Webhook Checkout JSON Unmarshall": err})
			return err
		}

		log.Info(log.V{"Paypal Webhook Event Parsed": paypalEventSubscription})

		transactionId := paypalEventSubscription.Resource.ID

		var subscription *Subscription

		subscription, err = FindSubscription(transactionId)
		if err != nil {
			log.Error(log.V{"Webhook, error finding paypal subscription in db using Transaction Id for updating it": err})
			return nil
		}

		err = updatePaypalSubscription(paypalEventSubscription, subscription)

	case "BILLING.SUBSCRIPTION.UPDATED":
		// Handle subscription updated event
		log.Info(log.V{"Paypal Subscription Updated": paypalWebhookEvent})
		var paypalEventSubscription PaypalEventSubscription

		err = json.Unmarshal(b, &paypalEventSubscription)

		if err != nil {
			log.Error(log.V{"Paypal Webhook Checkout JSON Unmarshall": err})
			return err
		}

		log.Info(log.V{"Paypal Webhook Event Parsed": paypalEventSubscription})

		transactionId := paypalEventSubscription.Resource.ID

		var subscription *Subscription

		subscription, err = FindSubscription(transactionId)
		if err != nil {
			log.Error(log.V{"Webhook, error finding paypal subscription in db using Transaction Id for updating it": err})
			return nil
		}

		err = updatePaypalSubscription(paypalEventSubscription, subscription)
	case "BILLING.SUBSCRIPTION.EXPIRED":
		// Handle subscription expired event
		log.Info(log.V{"Paypal Subscription Expired": paypalWebhookEvent})
		var paypalEventSubscription PaypalEventSubscription

		err = json.Unmarshal(b, &paypalEventSubscription)

		if err != nil {
			log.Error(log.V{"Paypal Webhook Checkout JSON Unmarshall": err})
			return err
		}

		log.Info(log.V{"Paypal Webhook Event Parsed": paypalEventSubscription})

		transactionId := paypalEventSubscription.Resource.ID

		var subscription *Subscription

		subscription, err = FindSubscription(transactionId)
		if err != nil {
			log.Error(log.V{"Webhook, error finding paypal subscription in db using Transaction Id for updating it": err})
			return nil
		}

		err = updatePaypalSubscription(paypalEventSubscription, subscription)
	case "BILLING.SUBSCRIPTION.CANCELLED":
		// Handle subscription cancelled event
		log.Info(log.V{"Paypal Subscription Cancelled": paypalWebhookEvent})
		var paypalEventSubscription PaypalEventSubscription

		err = json.Unmarshal(b, &paypalEventSubscription)

		if err != nil {
			log.Error(log.V{"Paypal Webhook Checkout JSON Unmarshall": err})
			return err
		}

		log.Info(log.V{"Paypal Webhook Event Parsed": paypalEventSubscription})

		transactionId := paypalEventSubscription.Resource.ID

		var subscription *Subscription

		subscription, err = FindSubscription(transactionId)
		if err != nil {
			log.Error(log.V{"Webhook, error finding paypal subscription in db using Transaction Id for updating it": err})
			return nil
		}

		err = updatePaypalSubscription(paypalEventSubscription, subscription)

		if err == nil {
			// Call the webhook from the product
			productId := subscription.ProductId

			product, err := products.Find(productId)
			if err != nil {
				log.Error(log.V{"Webhook, error finding product in db": err})
				return err
			} else {
				if product.WebhookURL != "" && product.WebhookSecret != "" {
					params := map[string]interface{}{
						"subscription_id": subscription.SubscriptionId,
						"custom_id":       subscription.UserId,
						"status":          "cancelled",
						"email":           subscription.CustomerEmail,
					}

					go func() {
						err := SendWebhook(product.WebhookURL, product.WebhookSecret, params)
						if err != nil {
							log.Error(log.V{"Paypal webhook, Error sending webhook to product's URL": err})
						} else {
							log.Info(log.V{"msg": "Successfully sent webhook to product's URL"})
						}
					}()
				}
			}
		}
	case "BILLING.SUBSCRIPTION.SUSPENDED":
		// Handle subscription suspended event
		log.Info(log.V{"Paypal Subscription Suspended": paypalWebhookEvent})
		var paypalEventSubscription PaypalEventSubscription

		err = json.Unmarshal(b, &paypalEventSubscription)

		if err != nil {
			log.Error(log.V{"Paypal Webhook Checkout JSON Unmarshall": err})
			return err
		}

		log.Info(log.V{"Paypal Webhook Event Parsed": paypalEventSubscription})

		transactionId := paypalEventSubscription.Resource.ID

		var subscription *Subscription

		subscription, err = FindSubscription(transactionId)
		if err != nil {
			log.Error(log.V{"Webhook, error finding paypal subscription in db using Transaction Id for updating it": err})
			return nil
		}

		err = updatePaypalSubscription(paypalEventSubscription, subscription)
	case "BILLING.SUBSCRIPTION.PAYMENT.FAILED":
		// Handle payment failed event
		log.Error(log.V{"Paypal Payment Failed": paypalWebhookEvent})
		var paypalEventSubscription PaypalEventSubscription

		err = json.Unmarshal(b, &paypalEventSubscription)

		if err != nil {
			log.Error(log.V{"Paypal Webhook Checkout JSON Unmarshall": err})
			return err
		}

		log.Info(log.V{"Paypal Webhook Event Parsed": paypalEventSubscription})

		transactionId := paypalEventSubscription.Resource.ID

		var subscription *Subscription

		subscription, err = FindSubscription(transactionId)
		if err != nil {
			log.Error(log.V{"Webhook, error finding paypal subscription in db using Transaction Id for updating it": err})
			return nil
		}

		err = updatePaypalSubscription(paypalEventSubscription, subscription)
	}

	// var eventSubscription payment.PaypalEventSubscriptionModel

	return err
}

// recordPaypalCheckoutOrder records the checkout order in the db
func recordPaypalCheckoutOrder(checkoutOrder PaypalEventCheckout, subscription *Subscription) error {

	// Params not validated using ValidateParams as user did not create these?
	transactionParams := make(map[string]string)
	transactionParams["pg"] = "paypal"
	if len(checkoutOrder.Resource.PurchaseUnits[0].Payments.Captures) > 0 {
		transactionParams["txn_id"] = checkoutOrder.Resource.PurchaseUnits[0].Payments.Captures[0].ID
	} else {
		transactionParams["txn_id"] = checkoutOrder.Resource.ID
	}
	transactionParams["payment_date"] = query.TimeString(checkoutOrder.Resource.CreateTime.UTC())
	transactionParams["payment_gross"] = checkoutOrder.Resource.PurchaseUnits[0].Amount.Value
	transactionParams["mc_currency"] = checkoutOrder.Resource.PurchaseUnits[0].Amount.CurrencyCode
	transactionParams["payer_id"] = checkoutOrder.Resource.Payer.PayerID
	transactionParams["payer_email"] = checkoutOrder.Resource.Payer.EmailAddress
	transactionParams["payment_status"] = checkoutOrder.Resource.Status
	if len(checkoutOrder.Resource.PurchaseUnits[0].Amount.Breakdown.TaxTotal.Value) > 0 {
		transactionParams["tax"] = checkoutOrder.Resource.PurchaseUnits[0].Amount.Breakdown.TaxTotal.Value
	}
	transactionParams["item_name"] = checkoutOrder.Resource.PurchaseUnits[0].Items[0].Name
	transactionParams["item_number"] = checkoutOrder.Resource.PurchaseUnits[0].Items[0].Sku
	transactionParams["first_name"] = checkoutOrder.Resource.Payer.Name.GivenName
	if len(checkoutOrder.Resource.PurchaseUnits[0].CustomID) > 0 {
		transactionParams["user_id"] = checkoutOrder.Resource.PurchaseUnits[0].CustomID
	}

	dbId, err := subscription.Create(transactionParams)

	if err == nil {
		log.Info(log.V{"Webhook, Paypal order added to db, ID: ": dbId})
	}

	return err
}

// recordPaypalSubscription function to record a PayPal subscription event
func recordPaypalSubscription(paypalEventSubscription PaypalEventSubscription, subscription Subscription) error {
	var product *products.Story

	// Params not validated using ValidateParams as user did not create these?
	transactionParams := make(map[string]string)
	transactionParams["pg"] = "paypal"
	transactionParams["subscr_id"] = paypalEventSubscription.Resource.ID
	transactionParams["payment_date"] = query.TimeString(paypalEventSubscription.Resource.CreateTime.UTC())
	transactionParams["payment_gross"] = paypalEventSubscription.Resource.BillingInfo.LastPayment.Amount.Value
	transactionParams["mc_currency"] = paypalEventSubscription.Resource.BillingInfo.LastPayment.Amount.CurrencyCode
	transactionParams["payer_id"] = paypalEventSubscription.Resource.Subscriber.PayerID
	transactionParams["payer_email"] = paypalEventSubscription.Resource.Subscriber.EmailAddress
	transactionParams["payment_status"] = paypalEventSubscription.Resource.Status
	transactionParams["first_name"] = paypalEventSubscription.Resource.Subscriber.Name.GivenName
	if len(paypalEventSubscription.Resource.CustomID) > 0 {
		transactionParams["user_id"] = paypalEventSubscription.Resource.CustomID
	}

	product, err := products.FindPaypalPlanId(paypalEventSubscription.Resource.PlanID)
	if err == nil {
		transactionParams["item_name"] = product.Name
		transactionParams["item_number"] = strconv.FormatInt(product.ID, 10)
	} else {
		log.Error(log.V{"Error finding product by plan id for recording paypal subscription in db": err})
	}

	paypalAuthorizationToken, err := GetPaypalAuthorizationToken()

	if err == nil {
		transaction, err := GetPaypalSubscriptionTransaction(paypalEventSubscription.Resource.ID, paypalAuthorizationToken)

		if err == nil {
			if len(transaction.Transactions) > 0 {
				transactionParams["tax"] = transaction.Transactions[0].AmountWithBreakdown.TaxAmount.Value
			}
		} else {
			log.Error(log.V{"Error getting paypal subscription transaction for recording paypal subscription in db": err})
		}
	} else {
		log.Error(log.V{"Error getting paypal authorization token for recording paypal subscription in db": err})
	}

	dbId, err := subscription.Create(transactionParams)

	if err == nil {
		log.Info(log.V{"Webhook, Paypal order added to db, ID: ": dbId})

		// Update counters based on product schedule
		if product != nil {
			transactionParams := make(map[string]string)
			if product.Schedule == "onetime" {
				product.TotalOnetimePayments += 1
				transactionParams["total_onetime_payments"] = strconv.FormatInt(product.TotalOnetimePayments, 10)
			} else {
				// Monthly or yearly subscription
				product.TotalSubscribers += 1
				transactionParams["total_subscribers"] = strconv.FormatInt(product.TotalSubscribers, 10)
			}
			err = product.Update(transactionParams)
			if err != nil {
				log.Error(log.V{"Paypal webhook, Error updating product counters": err})
				return err
			}
		}

	}

	return err
}

func updatePaypalSubscription(paypalEventSubscription PaypalEventSubscription, subscription *Subscription) error {
	transactionParams := make(map[string]string)
	transactionParams["payment_status"] = paypalEventSubscription.Resource.Status

	err := subscription.Update(transactionParams)

	if err == nil {
		log.Info(log.V{"msg": "Webhook, Paypal order added to db"})
		// Update the total subscribers count for the product associated with this subscription
		product, err := products.Find(subscription.ProductId)
		if err != nil {
			log.Error(log.V{"Paypal webhook, Error finding product": err})
			return err
		} else if product != nil {
			// Check if product status is not ACTIVE and then decrement the count
			// Only decrement for recurring subscriptions (not one-time payments)
			if subscription.PaymentStaus != "ACTIVE" && product.Schedule != "onetime" {

				// Decrement the total subscribers in the product
				product.TotalSubscribers -= 1
				transactionParams := make(map[string]string)
				transactionParams["total_subscribers"] = strconv.FormatInt(product.TotalSubscribers, 10) // Use FormatInt instead of Itoa
				err = product.Update(transactionParams)
				if err != nil {
					log.Error(log.V{"Paypal webhook, Error updating total subscribers for product": err})
					return err
				}
			}
		}
	}

	return err

}
