package subscriptions

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/abishekmuthian/open-payment-host/src/lib/mailchimp"
	"github.com/abishekmuthian/open-payment-host/src/lib/query"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
	"github.com/abishekmuthian/open-payment-host/src/products"
	"github.com/razorpay/razorpay-go/utils"
)

// HandleRazorpayWebhook receives the webhook POST request from the Razorpay
func HandleRazorpayWebhook(w http.ResponseWriter, r *http.Request) error {
	if r.Method != "POST" {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return nil
	}

	b, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		log.Error(log.V{"Razorpay webhook, ioutil.ReadAll: %v": err})
		return err
	}

	// Verify Razorpay Webhook
	webhookVerificationStatus := utils.VerifyWebhookSignature(string(b), r.Header.Get("X-Razorpay-Signature"), config.Get("razorpay_webhook_secret"))

	if webhookVerificationStatus {
		log.Info(log.V{"msg": "Razorpay webhook verified"})
		// Signature is valid. Return 200 OK.
		w.WriteHeader(200)
	} else {
		// Signature is invalid
		w.WriteHeader(403)
		log.Error(log.V{"Razorpay Webhook": "Invalid Razorpay Webhook Signature"})
		return nil
	}

	var razorpayWebhookEvent RazorpayWebhookEvent

	err = json.Unmarshal(b, &razorpayWebhookEvent)

	if err != nil {
		log.Error(log.V{"Razorpay Webhook JSON Unmarshall": err})
	}

	log.Info(log.V{"Razorpay Webhook Event Parsed": razorpayWebhookEvent})

	switch razorpayWebhookEvent.Event {
	case "order.paid":
		// Handle order paid event
		log.Info(log.V{"Razorpay webhook event": "Order Paid"})
		var razorpayEventOrderPaid RazorpayEventOrderPaid

		err = json.Unmarshal(b, &razorpayEventOrderPaid)

		if err != nil {
			log.Error(log.V{"Razorpay  Webhook Checkout JSON Unmarshall": err})
			return err
		}

		log.Info(log.V{"Razorpay  Webhook Event Parsed": razorpayEventOrderPaid})
		var subscription *Subscription

		subscription, err = FindPayment(razorpayEventOrderPaid.Payload.Order.Entity.ID)
		if err != nil {
			log.Info(log.V{"Webhook, error finding razorpay order in db using Capture Id": err})
		}

		if subscription == nil {
			subscription := New()
			err := recordRazorpayCheckoutOrder(razorpayEventOrderPaid, subscription)

			if err != nil {
				log.Error(log.V{"Webhook, error recording razorpay order in db": err})
				return err
			}

			// Add the email id to mailchimp

			subscription, err = FindPayment(razorpayEventOrderPaid.Payload.Order.Entity.ID)
			if err != nil {
				log.Error(log.V{"Razorpay Webhook, error finding razorpay transaction id in db using entity id for updating it": err})

				return err
			}
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
			}

			if product.WebhookURL != "" && product.WebhookSecret != "" {
				params := map[string]interface{}{
					"subscription_id": subscription.PaymentId,
					"custom_id":       strconv.FormatInt(subscription.UserId, 10),
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
			log.Info(log.V{"Webhook, razorpay order already exists in db, Order ID": subscription.ID})
		}
	case "subscription.authenticated":
		log.Info(log.V{"Razorpay webhook event": "Subscription Authenticated"})
	case "subscription.activated":
		log.Info(log.V{"Razorpay webhook event": "Subscription Activated"})
		var razorpayEventSubscriptionCompleted RazorpayEventSubscriptionCompleted

		err = json.Unmarshal(b, &razorpayEventSubscriptionCompleted)

		if err != nil {
			log.Error(log.V{"Razorpay  Webhook Subscription JSON Unmarshall": err})
			return err
		}

		log.Info(log.V{"Razorpay  Webhook Event Parsed": razorpayEventSubscriptionCompleted})

		var subscription *Subscription

		subscription, err = FindSubscription(razorpayEventSubscriptionCompleted.Payload.Subscription.Entity.ID)
		if err != nil {
			log.Info(log.V{"Webhook, error finding razorpay subscription in db using Capture Id": err})
		}

		if subscription == nil {
			subscription := New()
			err := recordRazorpaySubscription(razorpayEventSubscriptionCompleted, subscription)

			if err != nil {
				log.Error(log.V{"Webhook, error recording razorpay order in db": err})
				return err
			}

			subscription, err = FindSubscription(razorpayEventSubscriptionCompleted.Payload.Subscription.Entity.ID)

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
							"custom_id":       strconv.FormatInt(subscription.UserId, 10),
							"status":          "active",
							"email":           subscription.CustomerEmail,
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
			} else {
				log.Error(log.V{"Razorpay Webhook, error finding subscription to send webhook": err})
			}

		} else {
			log.Info(log.V{"Webhook, razorpay subscription already exists in db, Order ID": subscription.ID})
		}
	case "subscription.charged":
		log.Info(log.V{"Razorpay webhook event": "Subscription Charged"})
		var razorpayEventSubscriptionCompleted RazorpayEventSubscriptionCompleted

		err = json.Unmarshal(b, &razorpayEventSubscriptionCompleted)

		if err != nil {
			log.Error(log.V{"Razorpay  Webhook Subscription JSON Unmarshall": err})
			return err
		}

		log.Info(log.V{"Razorpay  Webhook Event Parsed": razorpayEventSubscriptionCompleted})

		var subscription *Subscription

		subscription, err = FindSubscription(razorpayEventSubscriptionCompleted.Payload.Subscription.Entity.ID)
		if err != nil {
			log.Info(log.V{"Webhook, error finding razorpay subscription in db using Capture Id": err})
			return nil

		}
		err = updateRazorpaySubscription(razorpayEventSubscriptionCompleted, subscription)
	case "subscription.completed":
		log.Info(log.V{"Razorpay webhook event": "Subscription Completed"})
		var razorpayEventSubscriptionCompleted RazorpayEventSubscriptionCompleted

		err = json.Unmarshal(b, &razorpayEventSubscriptionCompleted)

		if err != nil {
			log.Error(log.V{"Razorpay  Webhook Subscription JSON Unmarshall": err})
			return err
		}

		log.Info(log.V{"Razorpay  Webhook Event Parsed": razorpayEventSubscriptionCompleted})

		var subscription *Subscription

		subscription, err = FindSubscription(razorpayEventSubscriptionCompleted.Payload.Subscription.Entity.ID)
		if err != nil {
			log.Info(log.V{"Webhook, error finding razorpay subscription in db using Capture Id": err})
			return nil

		}
		err = updateRazorpaySubscription(razorpayEventSubscriptionCompleted, subscription)
	case "subscription.updated":
		log.Info(log.V{"Razorpay webhook event": "Subscription Updated"})
		var razorpayEventSubscriptionCompleted RazorpayEventSubscriptionCompleted

		err = json.Unmarshal(b, &razorpayEventSubscriptionCompleted)

		if err != nil {
			log.Error(log.V{"Razorpay  Webhook Subscription JSON Unmarshall": err})
			return err
		}

		log.Info(log.V{"Razorpay  Webhook Event Parsed": razorpayEventSubscriptionCompleted})

		var subscription *Subscription

		subscription, err = FindSubscription(razorpayEventSubscriptionCompleted.Payload.Subscription.Entity.ID)
		if err != nil {
			log.Info(log.V{"Webhook, error finding razorpay subscription in db using Capture Id": err})
			return nil

		}
		err = updateRazorpaySubscription(razorpayEventSubscriptionCompleted, subscription)
	case "subscription.pending":
		log.Info(log.V{"Razorpay webhook event": "Subscription Pending"})
		var razorpayEventSubscriptionCompleted RazorpayEventSubscriptionCompleted

		err = json.Unmarshal(b, &razorpayEventSubscriptionCompleted)

		if err != nil {
			log.Error(log.V{"Razorpay  Webhook Subscription JSON Unmarshall": err})
			return err
		}

		log.Info(log.V{"Razorpay  Webhook Event Parsed": razorpayEventSubscriptionCompleted})

		var subscription *Subscription

		subscription, err = FindSubscription(razorpayEventSubscriptionCompleted.Payload.Subscription.Entity.ID)
		if err != nil {
			log.Info(log.V{"Webhook, error finding razorpay subscription in db using Capture Id": err})
			return nil

		}
		err = updateRazorpaySubscription(razorpayEventSubscriptionCompleted, subscription)
	case "subscription.halted":
		log.Info(log.V{"Razorpay webhook event": "Subscription Halted"})
		var razorpayEventSubscriptionCompleted RazorpayEventSubscriptionCompleted

		err = json.Unmarshal(b, &razorpayEventSubscriptionCompleted)

		if err != nil {
			log.Error(log.V{"Razorpay  Webhook Subscription JSON Unmarshall": err})
			return err
		}

		log.Info(log.V{"Razorpay  Webhook Event Parsed": razorpayEventSubscriptionCompleted})

		var subscription *Subscription

		subscription, err = FindSubscription(razorpayEventSubscriptionCompleted.Payload.Subscription.Entity.ID)
		if err != nil {
			log.Info(log.V{"Webhook, error finding razorpay subscription in db using Capture Id": err})
			return nil

		}
		err = updateRazorpaySubscription(razorpayEventSubscriptionCompleted, subscription)
	case "subscription.cancelled":
		log.Info(log.V{"Razorpay webhook event": "Subscription Cancelled"})
		var razorpayEventSubscriptionCompleted RazorpayEventSubscriptionCompleted

		err = json.Unmarshal(b, &razorpayEventSubscriptionCompleted)

		if err != nil {
			log.Error(log.V{"Razorpay  Webhook Subscription JSON Unmarshall": err})
			return err
		}

		log.Info(log.V{"Razorpay  Webhook Event Parsed": razorpayEventSubscriptionCompleted})

		var subscription *Subscription

		subscription, err = FindSubscription(razorpayEventSubscriptionCompleted.Payload.Subscription.Entity.ID)
		if err != nil {
			log.Info(log.V{"Webhook, error finding razorpay subscription in db using Capture Id": err})
			return nil

		}
		err = updateRazorpaySubscription(razorpayEventSubscriptionCompleted, subscription)

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
						"custom_id":       strconv.FormatInt(subscription.UserId, 10),
						"status":          "cancelled",
						"email":           subscription.CustomerEmail,
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
		}
	case "subscription.paused":
		log.Info(log.V{"Razorpay webhook event": "Subscription Paused"})
		var razorpayEventSubscriptionCompleted RazorpayEventSubscriptionCompleted

		err = json.Unmarshal(b, &razorpayEventSubscriptionCompleted)

		if err != nil {
			log.Error(log.V{"Razorpay  Webhook Subscription JSON Unmarshall": err})
			return err
		}

		log.Info(log.V{"Razorpay  Webhook Event Parsed": razorpayEventSubscriptionCompleted})

		var subscription *Subscription

		subscription, err = FindSubscription(razorpayEventSubscriptionCompleted.Payload.Subscription.Entity.ID)
		if err != nil {
			log.Info(log.V{"Webhook, error finding razorpay subscription in db using Capture Id": err})
			return nil
		}
		err = updateRazorpaySubscription(razorpayEventSubscriptionCompleted, subscription)
	case "subscription.resumed":
		log.Info(log.V{"Razorpay webhook event": "Subscription Resumed"})
		var razorpayEventSubscriptionCompleted RazorpayEventSubscriptionCompleted

		err = json.Unmarshal(b, &razorpayEventSubscriptionCompleted)

		if err != nil {
			log.Error(log.V{"Razorpay  Webhook Subscription JSON Unmarshall": err})
			return err
		}

		log.Info(log.V{"Razorpay  Webhook Event Parsed": razorpayEventSubscriptionCompleted})

		var subscription *Subscription

		subscription, err = FindSubscription(razorpayEventSubscriptionCompleted.Payload.Subscription.Entity.ID)
		if err != nil {
			log.Info(log.V{"Webhook, error finding razorpay subscription in db using Capture Id": err})
			return nil
		}
		err = updateRazorpaySubscription(razorpayEventSubscriptionCompleted, subscription)

	}

	return err
}

func recordRazorpayCheckoutOrder(razorpayEventOrderPaid RazorpayEventOrderPaid, subscription *Subscription) error {
	// Params not validated using ValidateParams as user did not create these?
	transactionParams := make(map[string]string)
	transactionParams["pg"] = "razorpay"
	transactionParams["txn_id"] = razorpayEventOrderPaid.Payload.Order.Entity.ID
	createdAtTime := time.Unix(razorpayEventOrderPaid.Payload.Order.Entity.CreatedAt, 0) // Convert to time.Time

	transactionParams["payment_date"] = query.TimeString(createdAtTime)
	transactionParams["payment_gross"] = strconv.Itoa(razorpayEventOrderPaid.Payload.Payment.Entity.Amount / 100)
	transactionParams["payment_fee"] = strconv.Itoa(razorpayEventOrderPaid.Payload.Payment.Entity.Fee / 100) // Fee is in INR
	transactionParams["mc_currency"] = razorpayEventOrderPaid.Payload.Payment.Entity.Currency
	transactionParams["payment_status"] = razorpayEventOrderPaid.Payload.Payment.Entity.Status

	// FIXME: Razorpay doesn't seem to be supporting tax collection, The tax is got from the product
	// transactionParams["tax"] =

	if len(razorpayEventOrderPaid.Payload.Payment.Entity.Notes) > 0 {
		if customID, exists := razorpayEventOrderPaid.Payload.Payment.Entity.Notes["custom_id"]; exists {
			transactionParams["user_id"] = customID.(string)
		}

		// Phone number storage is disabled for privacy - phone is sent to Razorpay but not stored locally
		// if phone, exists := razorpayEventOrderPaid.Payload.Payment.Entity.Notes["phone"]; exists {
		// 	transactionParams["payer_phone"] = phone.(string)
		// }

		if address, exists := razorpayEventOrderPaid.Payload.Payment.Entity.Notes["address"]; exists {
			transactionParams["address_street"] = address.(string)
		}

		if addressCity, exists := razorpayEventOrderPaid.Payload.Payment.Entity.Notes["address_city"]; exists {
			transactionParams["address_city"] = addressCity.(string)
		}

		if addressState, exists := razorpayEventOrderPaid.Payload.Payment.Entity.Notes["address_state"]; exists {
			transactionParams["address_state"] = addressState.(string)
		}

		if addressPincode, exists := razorpayEventOrderPaid.Payload.Payment.Entity.Notes["address_pincode"]; exists {
			transactionParams["address_zip"] = addressPincode.(string)
		}

		if email, exists := razorpayEventOrderPaid.Payload.Payment.Entity.Notes["email"]; exists {
			transactionParams["payer_email"] = email.(string)
		}

		if name, exists := razorpayEventOrderPaid.Payload.Payment.Entity.Notes["name"]; exists {
			transactionParams["first_name"] = name.(string)
		}

		if productIdString, exists := razorpayEventOrderPaid.Payload.Payment.Entity.Notes["product_id"]; exists {

			transactionParams["item_number"] = productIdString.(string)

			// Convert receipt to int64
			productId, err := strconv.ParseInt(productIdString.(string), 10, 64)
			if err == nil {

				product, err := products.Find(productId)
				if err == nil {
					transactionParams["item_name"] = product.Name
				} else {
					log.Error(log.V{"Razorpay webhook, Error finding product": err})
				}
			} else {
				log.Error(log.V{"Razorpay webhook, Error converting receipt to int64": err})
			}

		}

	}

	dbId, err := subscription.Create(transactionParams)

	if err == nil {
		log.Info(log.V{"Webhook, razorpay order added to db, ID: ": dbId})
	}

	return err
}

func recordRazorpaySubscription(razorpayEventSubscriptionCompleted RazorpayEventSubscriptionCompleted, subscription *Subscription) error {
	var product *products.Story

	// Params not validated using ValidateParams as user did not create these?
	transactionParams := make(map[string]string)
	transactionParams["pg"] = "razorpay"
	transactionParams["subscr_id"] = razorpayEventSubscriptionCompleted.Payload.Subscription.Entity.ID
	createdAtTime := time.Unix(razorpayEventSubscriptionCompleted.Payload.Subscription.Entity.CreatedAt, 0) // Convert to time.Time

	transactionParams["payment_date"] = query.TimeString(createdAtTime)
	transactionParams["payment_gross"] = strconv.Itoa(razorpayEventSubscriptionCompleted.Payload.Payment.Entity.Amount / 100)
	transactionParams["payment_fee"] = strconv.Itoa(razorpayEventSubscriptionCompleted.Payload.Payment.Entity.Fee / 100) // Fee is in INR
	transactionParams["mc_currency"] = razorpayEventSubscriptionCompleted.Payload.Payment.Entity.Currency
	transactionParams["payment_status"] = razorpayEventSubscriptionCompleted.Payload.Subscription.Entity.Status
	transactionParams["payer_id"] = razorpayEventSubscriptionCompleted.Payload.Subscription.Entity.CustomerID

	// FIXME: Razorpay doesn't seem to be supporting tax collection, The tax is got from the product
	// transactionParams["tax"] =

	if len(razorpayEventSubscriptionCompleted.Payload.Payment.Entity.Notes) > 0 {
		if customID, exists := razorpayEventSubscriptionCompleted.Payload.Payment.Entity.Notes["custom_id"]; exists {
			transactionParams["user_id"] = customID.(string)
		}

		// Phone number storage is disabled for privacy - phone is sent to Razorpay but not stored locally
		// if phone, exists := razorpayEventSubscriptionCompleted.Payload.Payment.Entity.Notes["phone"]; exists {
		// 	transactionParams["payer_phone"] = phone.(string)
		// }

		if address, exists := razorpayEventSubscriptionCompleted.Payload.Payment.Entity.Notes["address"]; exists {
			transactionParams["address_street"] = address.(string)
		}

		if addressCity, exists := razorpayEventSubscriptionCompleted.Payload.Payment.Entity.Notes["address_city"]; exists {
			transactionParams["address_city"] = addressCity.(string)
		}

		if addressState, exists := razorpayEventSubscriptionCompleted.Payload.Payment.Entity.Notes["address_state"]; exists {
			transactionParams["address_state"] = addressState.(string)
		}

		if addressPincode, exists := razorpayEventSubscriptionCompleted.Payload.Payment.Entity.Notes["address_pincode"]; exists {
			transactionParams["address_zip"] = addressPincode.(string)
		}

		if email, exists := razorpayEventSubscriptionCompleted.Payload.Payment.Entity.Notes["email"]; exists {
			transactionParams["payer_email"] = email.(string)
		}

		if name, exists := razorpayEventSubscriptionCompleted.Payload.Payment.Entity.Notes["name"]; exists {
			transactionParams["first_name"] = name.(string)
		}

		if productIdString, exists := razorpayEventSubscriptionCompleted.Payload.Payment.Entity.Notes["product_id"]; exists {

			transactionParams["item_number"] = productIdString.(string)

			// Convert receipt to int64
			productId, err := strconv.ParseInt(productIdString.(string), 10, 64)
			if err == nil {

				product, err = products.Find(productId)
				if err == nil {
					transactionParams["item_name"] = product.Name
				} else {
					log.Error(log.V{"Razorpay webhook, Error finding product": err})
				}
			} else {
				log.Error(log.V{"Razorpay webhook, Error converting receipt to int64": err})
			}

		}

	}

	dbId, err := subscription.Create(transactionParams)

	if err == nil {
		log.Info(log.V{"Webhook, razorpay subscription added to db, ID: ": dbId})

		// Update total subscribers for the product
		if product != nil {
			product.TotalSubscribers += 1
			transactionParams := make(map[string]string)
			transactionParams["total_subscribers"] = strconv.FormatInt(product.TotalSubscribers, 10) // Use FormatInt instead of Itoa
			err = product.Update(transactionParams)
			if err != nil {
				log.Error(log.V{"Razorpay webhook, Error updating total subscribers for product": err})
				return err
			}
		}
	}

	return err
}

func updateRazorpaySubscription(razorpayEventSubscriptionCompleted RazorpayEventSubscriptionCompleted, subscription *Subscription) error {
	transactionParams := make(map[string]string)

	transactionParams["payment_status"] = razorpayEventSubscriptionCompleted.Payload.Subscription.Entity.Status

	err := subscription.Update(transactionParams)

	if err == nil {
		log.Info(log.V{"msg": "Webhook, razorpay subscription updated to db"})
		// Update the total subscribers count for the product associated with this subscription
		product, err := products.Find(subscription.ProductId)
		if err != nil {
			log.Error(log.V{"Razorpay webhook, Error finding product": err})
			return err
		} else if product != nil {
			// Check if product status is not active and then decrement the count
			if subscription.PaymentStaus != "active" {

				// Decrement the total subscribers in the product
				product.TotalSubscribers -= 1
				transactionParams := make(map[string]string)
				transactionParams["total_subscribers"] = strconv.FormatInt(product.TotalSubscribers, 10) // Use FormatInt instead of Itoa
				err = product.Update(transactionParams)
				if err != nil {
					log.Error(log.V{"Razorpay webhook, Error updating total subscribers for product": err})
					return err
				}
			}
		}
	}
	return err
}
