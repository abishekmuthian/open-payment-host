package subscriptions

import (
	"encoding/json"
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
	rows, err := query.New("payment_outbox", "id").Where("webhook_done=0 OR mailing_done=0").Order("created_at,id").Limit(100).Results()
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
		if err := deliverClaimedPaymentEffect(id); err != nil {
			return err
		}
	}
	return nil
}

func deliverClaimedPaymentEffect(id string) error {
	defer query.New("payment_outbox", "id").Where("id=?", id).Update(map[string]string{"lease_until": "0"})
	// Re-read after claiming: another worker may have delivered a stale row.
	row, err := query.New("payment_outbox", "id").Where("id=?", id).FirstResult()
	if err != nil {
		return err
	}
	a, err := FindAttempt(resource.ValidateString(row["attempt_id"]))
	if err != nil {
		return err
	}
	p, err := products.Find(a.ProductId)
	if err != nil {
		return err
	}
	payload, err := openCapability(resource.ValidateString(row["payload"]))
	if err != nil {
		return err
	}
	eventType := resource.ValidateString(row["event_type"])
	if resource.ValidateInt(row["webhook_done"]) == 0 {
		// The public experimental API remains limited to PayPal and Razorpay.
		if (a.Gateway == "paypal" || a.Gateway == "razorpay") && p.WebhookURL != "" && p.WebhookSecret != "" {
			if err := sendProductWebhook(p.WebhookURL, p.WebhookSecret, id, eventType, resource.ValidateString(row["created_at"]), []byte(payload)); err != nil {
				return err
			}
		}
		if err := query.New("payment_outbox", "id").Where("id=?", id).Update(map[string]string{"webhook_done": "1"}); err != nil {
			return err
		}
	}
	if resource.ValidateInt(row["mailing_done"]) == 0 {
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
		if err := query.New("payment_outbox", "id").Where("id=?", id).Update(map[string]string{"mailing_done": "1"}); err != nil {
			return err
		}
	}
	return nil
}
