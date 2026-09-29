package subscriptions

import (
	"encoding/json"
	"errors"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/razorpay/razorpay-go/utils"
	"net/http"
)

func HandleRazorpayWebhook(w http.ResponseWriter, r *http.Request) error {
	body, err := readWebhookBody(w, r)
	if err != nil {
		return nil
	}
	secret := config.Get("razorpay_webhook_secret")
	if secret == "" || !utils.VerifyWebhookSignature(string(body), r.Header.Get("X-Razorpay-Signature"), secret) {
		http.Error(w, "Invalid signature", 403)
		return nil
	}
	var e struct {
		Event   string `json:"event"`
		Payload struct {
			Order struct {
				Entity struct {
					ID string `json:"id"`
				} `json:"entity"`
			} `json:"order"`
			Payment struct {
				Entity razorpayPayment `json:"entity"`
			} `json:"payment"`
			Subscription struct {
				Entity struct {
					ID     string `json:"id"`
					Status string `json:"status"`
				} `json:"entity"`
			} `json:"subscription"`
			Refund struct {
				Entity struct {
					ID        string `json:"id"`
					PaymentID string `json:"payment_id"`
					Amount    int64  `json:"amount"`
					Currency  string `json:"currency"`
					Status    string `json:"status"`
				} `json:"entity"`
			} `json:"refund"`
		} `json:"payload"`
	}
	if err = json.Unmarshal(body, &e); err != nil {
		http.Error(w, "Invalid event", 400)
		return nil
	}
	err = processPaymentEvent("razorpay", r.Header.Get("X-Razorpay-Event-Id"), func() error {
		switch e.Event {
		case "order.paid":
			orderID := e.Payload.Order.Entity.ID
			payment := e.Payload.Payment.Entity
			if payment.InvoiceID != "" {
				if orderID == "" || payment.ID == "" || payment.OrderID == "" || payment.OrderID != orderID {
					return errors.New("invalid invoice-backed Razorpay order event")
				}
				// Razorpay also emits order.paid for subscription invoices. The
				// subscription.charged event performs the recurring payment proof.
				return nil
			}
			a, err := FindAttemptByProviderOrder("razorpay", orderID)
			if err != nil {
				return err
			}
			f, err := fetchRazorpayPaymentFacts(a.ProviderOrderId, payment.ID)
			if err != nil {
				return err
			}
			_, err = verifyAndFulfill(a, f)
			return err
		case "subscription.charged":
			a, err := FindAttemptByProviderSubscription("razorpay", e.Payload.Subscription.Entity.ID)
			if err != nil {
				return err
			}
			f, err := fetchRazorpaySubscriptionFacts(a.ProviderSubscriptionId, e.Payload.Payment.Entity.ID)
			if err != nil {
				return err
			}
			_, err = verifyAndFulfill(a, f)
			return err
		case "subscription.cancelled", "subscription.completed", "subscription.halted", "subscription.paused", "subscription.pending":
			status := e.Payload.Subscription.Entity.Status
			if status == "completed" {
				status = "expired"
			}
			switch status {
			case "cancelled", "expired", "halted", "paused", "pending":
				return applySubscriptionStatus("razorpay", e.Payload.Subscription.Entity.ID, status)
			}
			return errors.New("unexpected subscription status")
		case "refund.processed":
			f := e.Payload.Refund.Entity
			if f.Status != "processed" {
				return errors.New("refund is not processed")
			}
			return applyRefund("razorpay", f.PaymentID, f.ID, f.Amount, f.Currency)
		}
		return nil
	})
	return webhookResult(w, err)
}
