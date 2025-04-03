package paymentactions

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"

	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
	"github.com/abishekmuthian/open-payment-host/src/payment"
	"github.com/plutov/paypal/v4"
)

// HandlePaypalWebhook receives the webhook POST request from the Square
func HandlePaypalWebhook(w http.ResponseWriter, r *http.Request) error {
	if r.Method != "POST" {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return nil
	}

	// Create a client instance
	c, err := paypal.NewClient(config.Get("paypal_client_id"), config.Get("paypal_client_secret"), paypal.APIBaseSandBox)

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

	var eventSubscription payment.PaypalEventSubscriptionModel

	b, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		log.Error(log.V{"ioutil.ReadAll: %v": err})
		return err
	}

	err = json.Unmarshal(b, &eventSubscription)

	if err != nil {
		log.Error(log.V{"Paypal Payment JSON Unmarshall": err})
	}

	log.Info(log.V{"Paypal Payment parsed": eventSubscription})

	return err
}
