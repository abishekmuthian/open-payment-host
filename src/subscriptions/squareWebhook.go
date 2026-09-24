package subscriptions

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"net/http"
	"net/url"
)

func isFromSquare(signature string, body []byte) bool {
	key := config.Get("square_signature_key")
	notification := config.Get("square_notification_url")
	if key == "" || notification == "" || signature == "" {
		return false
	}
	h := hmac.New(sha256.New, []byte(key))
	h.Write([]byte(notification))
	h.Write(body)
	actual, err := base64.StdEncoding.DecodeString(signature)
	return err == nil && hmac.Equal(actual, h.Sum(nil))
}
func HandleSquareWebhook(w http.ResponseWriter, r *http.Request) error {
	body, err := readWebhookBody(w, r)
	if err != nil {
		return nil
	}
	if !isFromSquare(r.Header.Get("x-square-hmacsha256-signature"), body) {
		http.Error(w, "Invalid signature", 403)
		return nil
	}
	var e struct {
		ID   string `json:"event_id"`
		Type string `json:"type"`
		Data struct {
			Object struct {
				Payment struct {
					ID          string `json:"id"`
					ReferenceID string `json:"reference_id"`
					Status      string `json:"status"`
				} `json:"payment"`
				Invoice struct {
					ID             string `json:"id"`
					SubscriptionID string `json:"subscription_id"`
				} `json:"invoice"`
				Subscription struct {
					ID     string `json:"id"`
					Status string `json:"status"`
				} `json:"subscription"`
				Refund struct {
					ID          string `json:"id"`
					PaymentID   string `json:"payment_id"`
					Status      string `json:"status"`
					AmountMoney struct {
						Amount   int64  `json:"amount"`
						Currency string `json:"currency"`
					} `json:"amount_money"`
				} `json:"refund"`
			} `json:"object"`
		} `json:"data"`
	}
	if err = json.Unmarshal(body, &e); err != nil {
		http.Error(w, "Invalid event", 400)
		return nil
	}
	err = processPaymentEvent("square", e.ID, func() error {
		o := e.Data.Object
		switch e.Type {
		case "payment.created", "payment.updated":
			if o.Payment.Status != "COMPLETED" || o.Payment.ReferenceID == "" {
				return nil
			}
			a, err := FindAttempt(o.Payment.ReferenceID)
			if err != nil {
				return err
			}
			f, err := fetchSquarePaymentFacts(a, o.Payment.ID)
			if err != nil {
				return err
			}
			_, err = verifyAndFulfill(a, f)
			return err
		case "invoice.payment_made":
			if o.Invoice.SubscriptionID == "" {
				return nil
			}
			a, err := FindAttemptByProviderSubscription("square", o.Invoice.SubscriptionID)
			if err != nil {
				return err
			}
			f, err := fetchSquareInvoiceFacts(a, o.Invoice.ID)
			if err != nil {
				return err
			}
			_, err = verifyAndFulfill(a, f)
			return err
		case "subscription.updated":
			switch o.Subscription.Status {
			case "CANCELED":
				return applySubscriptionStatus("square", o.Subscription.ID, "cancelled")
			case "DEACTIVATED":
				return applySubscriptionStatus("square", o.Subscription.ID, "expired")
			case "PAUSED":
				return applySubscriptionStatus("square", o.Subscription.ID, "paused")
			}
		case "refund.created", "refund.updated":
			f := o.Refund
			if f.Status != "COMPLETED" {
				return nil
			}
			return applyRefund("square", f.PaymentID, f.ID, f.AmountMoney.Amount, f.AmountMoney.Currency)
		}
		return nil
	})
	return webhookResult(w, err)
}
func fetchSquarePaymentFacts(a *PaymentAttempt, id string) (ProviderFacts, error) {
	var result Charge
	if err := providerJSON("square", http.MethodGet, "/payments/"+url.PathEscape(id), nil, &result); err != nil {
		return ProviderFacts{}, err
	}
	p := result.Payment
	if a.Gateway != "square" || a.Schedule != "onetime" || p.ID != id || p.ReferenceID != a.Id || p.Status != "COMPLETED" {
		return ProviderFacts{}, errors.New("invalid Square payment")
	}
	if err := a.SetProviderIds("", p.ID, ""); err != nil {
		return ProviderFacts{}, err
	}
	return ProviderFacts{Gateway: "square", PaymentId: p.ID, Amount: int64(p.AmountMoney.Amount), Currency: p.AmountMoney.Currency, Paid: true, Email: p.BuyerEmailAddress}, nil
}
func fetchSquareInvoiceFacts(a *PaymentAttempt, id string) (ProviderFacts, error) {
	var result struct {
		Invoice struct {
			ID               string `json:"id"`
			Status           string `json:"status"`
			SubscriptionID   string `json:"subscription_id"`
			OrderID          string `json:"order_id"`
			PrimaryRecipient struct {
				Email string `json:"email_address"`
			} `json:"primary_recipient"`
		} `json:"invoice"`
	}
	if err := providerJSON("square", http.MethodGet, "/invoices/"+url.PathEscape(id), nil, &result); err != nil {
		return ProviderFacts{}, err
	}
	invoice := result.Invoice
	if invoice.ID != id || invoice.SubscriptionID != a.ProviderSubscriptionId || invoice.Status != "PAID" || invoice.OrderID == "" {
		return ProviderFacts{}, errPaymentPending
	}
	var sub SubscriptionModel
	if err := providerJSON("square", http.MethodGet, "/subscriptions/"+url.PathEscape(a.ProviderSubscriptionId), nil, &sub); err != nil {
		return ProviderFacts{}, err
	}
	if sub.Subscription.ID != a.ProviderSubscriptionId || sub.Subscription.PlanID != a.PriceId {
		return ProviderFacts{}, errors.New("subscription plan mismatch")
	}
	var order struct {
		Order struct {
			ID      string `json:"id"`
			State   string `json:"state"`
			Tenders []struct {
				PaymentID string `json:"payment_id"`
			} `json:"tenders"`
		} `json:"order"`
	}
	if err := providerJSON("square", http.MethodGet, "/orders/"+url.PathEscape(invoice.OrderID), nil, &order); err != nil {
		return ProviderFacts{}, err
	}
	if order.Order.ID != invoice.OrderID || order.Order.State != "COMPLETED" || len(order.Order.Tenders) != 1 {
		return ProviderFacts{}, errors.New("unsupported or unpaid subscription invoice")
	}
	paymentID := order.Order.Tenders[0].PaymentID
	if paymentID == "" {
		return ProviderFacts{}, errors.New("invoice has no payment")
	}
	var payment Charge
	if err := providerJSON("square", http.MethodGet, "/payments/"+url.PathEscape(paymentID), nil, &payment); err != nil {
		return ProviderFacts{}, err
	}
	p := payment.Payment
	if p.ID != paymentID || p.OrderID != invoice.OrderID || p.Status != "COMPLETED" {
		return ProviderFacts{}, errors.New("invoice payment mismatch")
	}
	return ProviderFacts{Gateway: "square", PaymentId: p.ID, SubscriptionId: a.ProviderSubscriptionId, PriceId: a.PriceId, Amount: int64(p.AmountMoney.Amount), Currency: p.AmountMoney.Currency, Paid: true, Email: invoice.PrimaryRecipient.Email}, nil
}

func validateSquarePlan(id, schedule string, amount int64, currency string) error {
	var result struct {
		Object struct {
			ID   string `json:"id"`
			Plan struct {
				Phases []struct {
					Cadence string `json:"cadence"`
					Price   struct {
						Amount   int64  `json:"amount"`
						Currency string `json:"currency"`
					} `json:"recurring_price_money"`
				} `json:"phases"`
			} `json:"subscription_plan_data"`
		} `json:"object"`
	}
	if err := providerJSON("square", http.MethodGet, "/catalog/object/"+url.PathEscape(id), nil, &result); err != nil {
		return err
	}
	cadence := "MONTHLY"
	if schedule == "yearly" {
		cadence = "ANNUAL"
	}
	if result.Object.ID != id || len(result.Object.Plan.Phases) != 1 {
		return errors.New("unsupported Square plan")
	}
	p := result.Object.Plan.Phases[0]
	if p.Cadence != cadence || p.Price.Amount != amount || !currencyEqual(p.Price.Currency, currency) {
		return errors.New("Square plan price or cadence mismatch")
	}
	return nil
}
