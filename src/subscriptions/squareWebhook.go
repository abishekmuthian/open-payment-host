package subscriptions

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"

	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
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

	var eventSubscription EventSubscriptionModel

	err = json.Unmarshal(b, &eventSubscription)

	if err != nil {
		log.Error(log.V{"Square Payment JSON Unmarshall": err})
	}

	log.Info(log.V{"Square Payment parsed": eventSubscription})

	var subscription *Subscription

	subscription, err = FindSubscription(eventSubscription.Data.Object.Subscription.ID)
	if err != nil {
		log.Info(log.V{"Webhook, error finding subscription using Subscription Id": err})
	}

	if subscription == nil {
		subscription := New()
		err := recordSquareSubscriptionPaymentTransaction(eventSubscription, subscription)
		if err != nil {
			log.Error(log.V{"Webhook, error recording subscription transaction": err})
		}
	} else if eventSubscription.Data.Object.Subscription.Status != "ACTIVE" {

		// Update the subscription in the database
		transactionParams := make(map[string]string)
		transactionParams["payment_status"] = eventSubscription.Data.Object.Subscription.Status
		err := subscription.Update(transactionParams)

		if err == nil {
			log.Info(log.V{"Webhook transaction updated to db, Subscription ID": subscription.ID})
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
	}

	return err
}
