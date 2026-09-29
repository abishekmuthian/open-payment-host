package subscriptions

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/plutov/paypal/v4"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func HandlePaypalWebhook(w http.ResponseWriter, r *http.Request) error {
	body, err := readWebhookBody(w, r)
	if err != nil {
		return nil
	}
	if config.Get("paypal_webhook_id") == "" {
		http.Error(w, "Webhook is not configured", 503)
		return nil
	}
	c, err := paypal.NewClient(config.Get("paypal_client_id"), config.Get("paypal_client_secret"), config.Get("paypal_api_domain"))
	if err != nil {
		return webhookResult(w, err)
	}
	c.SetHTTPClient(paymentHTTPClient)
	r.Body = io.NopCloser(bytes.NewReader(body))
	verification, err := c.VerifyWebhookSignature(r.Context(), r, config.Get("paypal_webhook_id"))
	if err != nil || verification == nil || verification.VerificationStatus != "SUCCESS" {
		http.Error(w, "Invalid signature", 403)
		return nil
	}
	var e struct {
		ID       string `json:"id"`
		Type     string `json:"event_type"`
		Resource struct {
			ID                 string `json:"id"`
			Status             string `json:"status"`
			BillingAgreementID string `json:"billing_agreement_id"`
			Amount             struct {
				Value        string `json:"value"`
				Currency     string `json:"currency"`
				CurrencyCode string `json:"currency_code"`
				Total        string `json:"total"`
			} `json:"amount"`
			SupplementaryData struct {
				RelatedIDs struct {
					OrderID   string `json:"order_id"`
					CaptureID string `json:"capture_id"`
				} `json:"related_ids"`
			} `json:"supplementary_data"`
			Links []struct {
				Rel  string `json:"rel"`
				Href string `json:"href"`
			} `json:"links"`
		} `json:"resource"`
	}
	if err = json.Unmarshal(body, &e); err != nil {
		http.Error(w, "Invalid event", 400)
		return nil
	}
	err = processPaymentEvent("paypal", e.ID, func() error {
		resource := e.Resource
		switch e.Type {
		case "PAYMENT.CAPTURE.COMPLETED", "CHECKOUT.ORDER.COMPLETED":
			id := resource.ID
			if e.Type == "PAYMENT.CAPTURE.COMPLETED" {
				id = resource.SupplementaryData.RelatedIDs.OrderID
			}
			a, err := FindAttemptByProviderOrder("paypal", id)
			if err != nil {
				return err
			}
			o, err := fetchPaypalOrderDetails(id)
			if err != nil {
				return err
			}
			f, err := paypalOrderFacts(o, a)
			if err != nil {
				return err
			}
			_, err = verifyAndFulfill(a, f)
			return err
		case "BILLING.SUBSCRIPTION.ACTIVATED", "PAYMENT.SALE.COMPLETED":
			id := resource.ID
			txn := ""
			if e.Type == "PAYMENT.SALE.COMPLETED" {
				id = resource.BillingAgreementID
				txn = resource.ID
			}
			a, err := FindAttemptByProviderSubscription("paypal", id)
			if err != nil {
				return err
			}
			f, err := fetchPaypalSubscriptionTransactionFacts(a, txn)
			if err != nil {
				return err
			}
			_, err = verifyAndFulfill(a, f)
			return err
		case "BILLING.SUBSCRIPTION.CANCELLED":
			return applySubscriptionStatus("paypal", resource.ID, "cancelled")
		case "BILLING.SUBSCRIPTION.EXPIRED":
			return applySubscriptionStatus("paypal", resource.ID, "expired")
		case "BILLING.SUBSCRIPTION.SUSPENDED":
			return applySubscriptionStatus("paypal", resource.ID, "suspended")
		case "PAYMENT.CAPTURE.REFUNDED", "PAYMENT.SALE.REFUNDED":
			capture := resource.SupplementaryData.RelatedIDs.CaptureID
			if capture == "" {
				for _, l := range resource.Links {
					if l.Rel == "up" {
						u, err := url.Parse(l.Href)
						if err == nil {
							parts := strings.Split(strings.TrimRight(u.Path, "/"), "/")
							capture = parts[len(parts)-1]
						}
					}
				}
			}
			currency := resource.Amount.CurrencyCode
			if currency == "" {
				currency = resource.Amount.Currency
			}
			value := resource.Amount.Value
			if value == "" {
				value = resource.Amount.Total
			}
			amount, err := majorValueToMinor(value, currency)
			if err != nil {
				return err
			}
			if resource.Status != "COMPLETED" && resource.Status != "completed" && resource.Status != "" {
				return errors.New("refund not completed")
			}
			return applyRefund("paypal", capture, resource.ID, amount, currency)
		}
		return nil
	})
	return webhookResult(w, err)
}
