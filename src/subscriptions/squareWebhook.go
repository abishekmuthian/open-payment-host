package subscriptions

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
	"github.com/abishekmuthian/open-payment-host/src/products"
)

// HandleSquareWebhook receives the webhook POST request from the Square
func HandleSquareWebhook(w http.ResponseWriter, r *http.Request) error {
	if r.Method != "POST" {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return nil
	}

	// Check if the event is from Square
	signature := r.Header.Get("x-square-hmacsha256-signature")

	b, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		log.Error(log.V{"ioutil.ReadAll: %v": err})
		return err
	}

	if isFromSquare(signature, b) {
		// Signature is valid. Return 200 OK.
		w.WriteHeader(200)
		log.Info(log.V{"Request body: ": string(b)})
	} else {
		// Signature is invalid. Return 403 Forbidden.
		w.WriteHeader(403)
		log.Error(log.V{"Square Webhook": "Invalid signature"})
		return nil
	}

	// First, detect event type by checking for "payment" or "subscription" in the type field
	var baseEvent struct {
		Type string `json:"type"`
	}
	err = json.Unmarshal(b, &baseEvent)
	if err != nil {
		log.Error(log.V{"Square Event Type JSON Unmarshall": err})
		return err
	}

	log.Info(log.V{"Square Event Type": baseEvent.Type})

	// Handle payment events (payment.created, payment.updated)
	if baseEvent.Type == "payment.created" || baseEvent.Type == "payment.updated" {
		var eventPayment EventPaymentModel
		err = json.Unmarshal(b, &eventPayment)
		if err != nil {
			log.Error(log.V{"Square Payment Event JSON Unmarshall": err})
			return err
		}

		log.Info(log.V{"Square Payment Event parsed": eventPayment})

		// Only process COMPLETED payments
		if eventPayment.Data.Object.Payment.Status == "COMPLETED" {
			var payment *Subscription
			payment, err = FindPayment(eventPayment.Data.Object.Payment.ID)
			if err != nil {
				log.Info(log.V{"Webhook, error finding payment using Payment Id": err})
			}

			if payment == nil {
				payment := New()
				err = recordSquarePaymentTransaction(eventPayment, payment)
				if err != nil {
					log.Error(log.V{"Webhook, error recording payment transaction": err})
					return err
				}
			} else {
				log.Info(log.V{"Webhook payment already present in the DB": payment.ID})
			}
		}
		return nil
	} else {
		// Handle subscription events (subscription.created, subscription.updated)
		var eventSubscription EventSubscriptionModel
		err = json.Unmarshal(b, &eventSubscription)
		if err != nil {
			log.Error(log.V{"Square Subscription Event JSON Unmarshall": err})
			return err
		}

		log.Info(log.V{"Square Subscription Event parsed": eventSubscription})

		var subscription *Subscription
		subscription, err = FindSubscription(eventSubscription.Data.Object.Subscription.ID)
		if err != nil {
			log.Info(log.V{"Webhook, error finding subscription using Subscription Id": err})
		}

		if subscription == nil {
			subscription := New()
			err = recordSquareSubscriptionPaymentTransaction(eventSubscription, subscription)
			if err != nil {
				log.Error(log.V{"Webhook, error recording subscription transaction": err})
				return err
			}
		} else if eventSubscription.Data.Object.Subscription.Status != "ACTIVE" {

			// Update the subscription in the database
			transactionParams := make(map[string]string)
			transactionParams["payment_status"] = eventSubscription.Data.Object.Subscription.Status
			err = subscription.Update(transactionParams)

			if err == nil {
				log.Info(log.V{"Webhook transaction updated to db, Subscription ID": subscription.ID})
			}

			// Decrement subscriber count only for recurring subscriptions (not one-time payments)
			if err == nil && subscription.ProductId > 0 {
				product, err := products.Find(subscription.ProductId)
				if err != nil {
					log.Error(log.V{"Square webhook, Error finding product": err})
				} else if product != nil && product.Schedule != "onetime" {
					product.TotalSubscribers -= 1
					productParams := make(map[string]string)
					productParams["total_subscribers"] = strconv.FormatInt(product.TotalSubscribers, 10)
					err = product.Update(productParams)
					if err != nil {
						log.Error(log.V{"Square webhook, Error updating total subscribers for product": err})
					}
				}
			}
		}
		return nil
	}
}

// recordSquarePaymentTransaction adds one-time payment transaction to database from Square Webhook
func recordSquarePaymentTransaction(eventPayment EventPaymentModel, payment *Subscription) error {
	// Params not validated using ValidateParams as user did not create these?
	transactionParams := make(map[string]string)
	transactionParams["pg"] = "square"
	transactionParams["txn_id"] = eventPayment.Data.Object.Payment.ID
	transactionParams["payment_date"] = eventPayment.Data.Object.Payment.CreatedAt
	transactionParams["receipt_id"] = eventPayment.Data.Object.Payment.ReceiptNumber
	transactionParams["mc_gross"] = strconv.FormatInt(eventPayment.Data.Object.Payment.AmountMoney.Amount, 10)
	transactionParams["payment_gross"] = strconv.FormatInt(eventPayment.Data.Object.Payment.TotalMoney.Amount, 10)
	transactionParams["mc_currency"] = eventPayment.Data.Object.Payment.AmountMoney.Currency
	transactionParams["payer_id"] = eventPayment.Data.Object.Payment.CustomerID
	transactionParams["txn_type"] = eventPayment.Data.Type
	transactionParams["payment_status"] = eventPayment.Data.Object.Payment.Status

	// Extract product ID from ReferenceID (format: "Product Id: 123")
	var productId int64
	if eventPayment.Data.Object.Payment.ReferenceID != "" {
		// Parse the reference ID to extract product ID
		_, err := fmt.Sscanf(eventPayment.Data.Object.Payment.ReferenceID, "Product Id: %d", &productId)
		if err == nil && productId > 0 {
			transactionParams["item_number"] = strconv.FormatInt(productId, 10)
		}
	}

	dbId, err := payment.Create(transactionParams)

	if err == nil {
		log.Info(log.V{"Webhook payment transaction added to db, ID: ": dbId})

		// Update counters based on product schedule
		if productId > 0 {
			product, err := products.Find(productId)
			if err != nil {
				log.Error(log.V{"Square webhook, Error finding product by ID": err})
			} else if product != nil {
				productParams := make(map[string]string)
				// One-time payments always increment TotalOnetimePayments
				product.TotalOnetimePayments += 1
				productParams["total_onetime_payments"] = strconv.FormatInt(product.TotalOnetimePayments, 10)
				err = product.Update(productParams)
				if err != nil {
					log.Error(log.V{"Square webhook, Error updating product counters": err})
				}
			}
		}
	}

	return err
}

// isFromSquare generates a signature from the url and body and compares it to the Square signature header.
func isFromSquare(signature string, body []byte) bool {
	payload := new(bytes.Buffer)
	_ = json.Compact(payload, body)

	appended := append([]byte(config.Get("square_notification_url")), payload.Bytes()...)
	key := []byte(config.Get("square_signature_key"))
	hash := hmac.New(sha256.New, key)
	hash.Write(appended)

	return signature == base64.StdEncoding.EncodeToString(hash.Sum(nil))
}

// recordSquareSubscriptionPaymentTransaction adds the transaction to database from Square Webhook
func recordSquareSubscriptionPaymentTransaction(eventSubscription EventSubscriptionModel, subscription *Subscription) error {
	// Params not validated using ValidateParams as user did not create these?
	transactionParams := make(map[string]string)
	transactionParams["pg"] = "square"
	transactionParams["txn_id"] = eventSubscription.Data.Object.Subscription.PlanID
	transactionParams["payment_date"] = eventSubscription.Data.Object.Subscription.CreatedDate
	transactionParams["payer_id"] = eventSubscription.Data.Object.Subscription.CustomerID
	transactionParams["txn_type"] = eventSubscription.Data.Type
	transactionParams["payment_status"] = eventSubscription.Data.Object.Subscription.Status
	transactionParams["subscr_id"] = eventSubscription.Data.Object.Subscription.ID

	dbId, err := subscription.Create(transactionParams)

	if err == nil {
		log.Info(log.V{"Webhook transaction added to db, ID: ": dbId})

		// Update counters based on product schedule
		// Find product by Square plan ID
		product, err := products.FindSquarePlanId(eventSubscription.Data.Object.Subscription.PlanID)
		if err != nil {
			log.Error(log.V{"Square webhook, Error finding product by plan ID": err})
		} else if product != nil {
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
				log.Error(log.V{"Square webhook, Error updating product counters": err})
			}
		}
	}

	return err
}
