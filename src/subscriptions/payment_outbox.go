package subscriptions

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/abishekmuthian/open-payment-host/src/lib/query"
	"github.com/abishekmuthian/open-payment-host/src/lib/resource"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
	"github.com/abishekmuthian/open-payment-host/src/products"
	"sync"
	"time"
)

var outboxMu sync.Mutex
var outboxOnce sync.Once

// StartPaymentDeliveries retries persisted effects after restart. A recipient
// must deduplicate X-OPH-Event-ID when its reply is lost after accepting a POST.
func StartPaymentDeliveries() {
	outboxOnce.Do(func() {
		go func() {
			for {
				if err := deliverPaymentEffects(); err != nil {
					log.Error(log.V{"payment delivery": err})
				}
				time.Sleep(30 * time.Second)
			}
		}()
	})
}
func deliverPaymentEffects() error {
	outboxMu.Lock()
	defer outboxMu.Unlock()
	// Preserve ordering within each channel without letting a failure in one
	// channel prevent the other from advancing.
	webhookErr := deliverPaymentEffectChannel("webhook_done", deliverPaymentWebhook)
	mailingErr := deliverPaymentEffectChannel("mailing_done", deliverPaymentMailing)
	if webhookErr != nil {
		webhookErr = fmt.Errorf("webhook delivery: %w", webhookErr)
	}
	if mailingErr != nil {
		mailingErr = fmt.Errorf("mailing delivery: %w", mailingErr)
	}
	return errors.Join(webhookErr, mailingErr)
}

func deliverPaymentEffectChannel(doneColumn string, deliver func(string) error) error {
	rows, err := query.New("payment_outbox", "id").Where(doneColumn + "=0").Order("created_at,id").Limit(100).Results()
	if err != nil {
		return err
	}
	for _, row := range rows {
		id := resource.ValidateString(row["id"])
		claimed := false
		err := query.Transaction(func(tx *query.Tx) error {
			result, err := tx.Exec("UPDATE payment_outbox SET lease_until=? WHERE id=? AND lease_until<?", time.Now().Add(5*time.Minute).Unix(), id, time.Now().Unix())
			if err != nil {
				return err
			}
			n, err := result.RowsAffected()
			claimed = n == 1
			return err
		})
		if err != nil {
			return err
		}
		// Keep delivery ordering when another process owns the oldest event.
		if !claimed {
			return nil
		}
		if err := deliverClaimedPaymentEffect(id, deliver); err != nil {
			return err
		}
	}
	return nil
}

func deliverClaimedPaymentEffect(id string, deliver func(string) error) (err error) {
	defer func() {
		releaseErr := query.New("payment_outbox", "id").Where("id=?", id).Update(map[string]string{"lease_until": "0"})
		if releaseErr != nil {
			err = errors.Join(err, fmt.Errorf("release outbox lease: %w", releaseErr))
		}
	}()
	return deliver(id)
}

type paymentEffect struct {
	row     query.Result
	attempt *PaymentAttempt
	product *products.Story
	payload string
}

func loadPaymentEffect(id string) (*paymentEffect, error) {
	// Re-read after claiming: another worker may have delivered a stale row.
	row, err := query.New("payment_outbox", "id").Where("id=?", id).FirstResult()
	if err != nil {
		return nil, err
	}
	a, err := FindAttempt(resource.ValidateString(row["attempt_id"]))
	if err != nil {
		return nil, err
	}
	p, err := products.Find(a.ProductId)
	if err != nil {
		return nil, err
	}
	payload, err := openCapability(resource.ValidateString(row["payload"]))
	if err != nil {
		return nil, err
	}
	return &paymentEffect{row: row, attempt: a, product: p, payload: payload}, nil
}

func deliverPaymentWebhook(id string) error {
	effect, err := loadPaymentEffect(id)
	if err != nil {
		return err
	}
	if resource.ValidateInt(effect.row["webhook_done"]) != 0 {
		return nil
	}
	a := effect.attempt
	p := effect.product
	eventType := resource.ValidateString(effect.row["event_type"])
	// The public experimental API remains limited to PayPal and Razorpay.
	if (a.Gateway == "paypal" || a.Gateway == "razorpay") && p.WebhookURL != "" && p.WebhookSecret != "" {
		if err := sendProductWebhook(p.WebhookURL, p.WebhookSecret, id, eventType, resource.ValidateString(effect.row["created_at"]), []byte(effect.payload)); err != nil {
			return err
		}
	}
	return query.New("payment_outbox", "id").Where("id=?", id).Update(map[string]string{"webhook_done": "1"})
}

func deliverPaymentMailing(id string) error {
	effect, err := loadPaymentEffect(id)
	if err != nil {
		return err
	}
	if resource.ValidateInt(effect.row["mailing_done"]) != 0 {
		return nil
	}
	a := effect.attempt
	p := effect.product
	payload := effect.payload
	var data struct {
		Email  string `json:"email"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(payload), &data); err != nil {
		return err
	}
	if data.Email == "" {
		s, err := FindFirst("pg=? AND txn_id=?", a.Gateway, a.ProviderPaymentId)
		if err == nil {
			data.Email = s.CustomerEmail
		}
	}
	// Synchronize current entitlement, including other paid purchases by
	// this email. A historical refund must not unsubscribe a renewed buyer.
	entitled, err := query.New("payment_attempts", "id").Where(`product_id=? AND counted=1 AND EXISTS
            (SELECT 1 FROM subscriptions s WHERE s.pg=payment_attempts.gateway AND s.payer_email=? AND
            (s.txn_id=payment_attempts.provider_payment_id OR
            (payment_attempts.provider_subscription_id<>'' AND s.subscr_id=payment_attempts.provider_subscription_id)))`, a.ProductId, data.Email).Limit(1).Results()
	if err != nil {
		return err
	}
	mailingStatus := "cancelled"
	if len(entitled) > 0 {
		mailingStatus = "active"
	}
	if err := syncPaymentMailingLists(p, data.Email, mailingStatus); err != nil {
		return err
	}
	return query.New("payment_outbox", "id").Where("id=?", id).Update(map[string]string{"mailing_done": "1"})
}
