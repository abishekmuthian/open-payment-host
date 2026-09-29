package subscriptions

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/abishekmuthian/open-payment-host/src/lib/query"
	"github.com/abishekmuthian/open-payment-host/src/lib/resource"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
	"github.com/abishekmuthian/open-payment-host/src/products"
	"net/url"
	"strings"
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

// Each product is an ordered delivery partition: webhook URLs and mailing
// lists are configured per product, so one merchant's failing endpoint must
// not delay other products.
const (
	outboxWorkers    = 8
	outboxBatch      = 100
	outboxRetryBase  = 30 * time.Second
	outboxRetryMax   = time.Hour
	outboxGiveUpAge  = 72 * time.Hour
	outboxErrorLimit = 500
	// outboxFailed marks an abandoned row ({channel}_done=1 is delivered). To
	// replay it, reset {channel}_done, _attempts and _next_attempt_at to 0.
	outboxFailed = 2
)

func deliverPaymentEffects() error {
	outboxMu.Lock()
	defer outboxMu.Unlock()
	// Channels share the row lease, so they run one after the other; a
	// failure in one channel does not prevent the other from advancing.
	webhookErr := deliverPaymentEffectChannel("webhook", deliverPaymentWebhook)
	mailingErr := deliverPaymentEffectChannel("mailing", deliverPaymentMailing)
	if webhookErr != nil {
		webhookErr = fmt.Errorf("webhook delivery: %w", webhookErr)
	}
	if mailingErr != nil {
		mailingErr = fmt.Errorf("mailing delivery: %w", mailingErr)
	}
	return errors.Join(webhookErr, mailingErr)
}

func deliverPaymentEffectChannel(channel string, deliver func(string) error) error {
	rows, err := query.New("payment_outbox", "id").
		Select("SELECT DISTINCT payment_attempts.product_id FROM payment_outbox INNER JOIN payment_attempts ON payment_attempts.id=payment_outbox.attempt_id").
		Where("payment_outbox." + channel + "_done=0").Order("payment_attempts.product_id").Results()
	if err != nil {
		return err
	}
	// HTTP runs outside transactions, so concurrent partitions only serialize
	// on SQLite's single connection for their short lease and status updates.
	errs := make([]error, len(rows))
	slots := make(chan struct{}, outboxWorkers)
	var wg sync.WaitGroup
	for i, row := range rows {
		productID := resource.ValidateInt(row["product_id"])
		slots <- struct{}{}
		wg.Add(1)
		go func(i int, productID int64) {
			defer wg.Done()
			defer func() { <-slots }()
			errs[i] = deliverProductEffects(channel, productID, deliver)
		}(i, productID)
	}
	wg.Wait()
	return errors.Join(errs...)
}

// deliverProductEffects delivers one product's pending effects in order and
// stops at the first failure, unclaimed row or row waiting for its backoff.
func deliverProductEffects(channel string, productID int64, deliver func(string) error) error {
	rows, err := query.New("payment_outbox", "id").
		Select("SELECT payment_outbox.id, payment_outbox.event_type, payment_outbox.created_at, payment_outbox."+channel+"_attempts AS attempts, payment_outbox."+channel+"_next_attempt_at AS next_attempt_at FROM payment_outbox INNER JOIN payment_attempts ON payment_attempts.id=payment_outbox.attempt_id").
		Where("payment_outbox."+channel+"_done=0 AND payment_attempts.product_id=?", productID).
		Order("payment_outbox.created_at,payment_outbox.id").Limit(outboxBatch).Results()
	if err != nil {
		return fmt.Errorf("product %d: %w", productID, err)
	}
	for _, row := range rows {
		id := resource.ValidateString(row["id"])
		eventType := resource.ValidateString(row["event_type"])
		now := time.Now()
		if resource.ValidateInt(row["next_attempt_at"]) > now.Unix() {
			return nil
		}
		claimed := false
		err := query.Transaction(func(tx *query.Tx) error {
			result, err := tx.Exec("UPDATE payment_outbox SET lease_until=? WHERE id=? AND lease_until<? AND "+channel+"_done=0 AND "+channel+"_next_attempt_at<=?", now.Add(5*time.Minute).Unix(), id, now.Unix(), now.Unix())
			if err != nil {
				return err
			}
			n, err := result.RowsAffected()
			claimed = n == 1
			return err
		})
		if err != nil {
			return fmt.Errorf("product %d event %s (%s): %w", productID, id, eventType, err)
		}
		// Keep delivery ordering when another process owns the oldest event.
		if !claimed {
			return nil
		}
		attempts := resource.ValidateInt(row["attempts"])
		abandoned, err := deliverClaimedPaymentEffect(channel, id, resource.ValidateString(row["created_at"]), attempts, deliver)
		if abandoned {
			log.Error(log.V{"payment delivery abandoned": channel, "product_id": productID, "event_id": id, "event_type": eventType, "attempts": attempts + 1, "last_error": err})
			continue
		}
		if err != nil {
			return fmt.Errorf("product %d event %s (%s): %w", productID, id, eventType, err)
		}
	}
	return nil
}

// deliverClaimedPaymentEffect delivers a leased row and releases the lease.
// A failure schedules an exponential backoff, or abandons a row older than
// outboxGiveUpAge; abandoned is true only once that state is stored.
func deliverClaimedPaymentEffect(channel, id, createdAt string, attempts int64, deliver func(string) error) (abandoned bool, err error) {
	err = redactOutboxError(deliver(id))
	now := time.Now()
	statement := "UPDATE payment_outbox SET lease_until=0 WHERE id=?"
	args := []interface{}{id}
	giveUp := false
	if err != nil {
		lastError := err.Error()
		if len(lastError) > outboxErrorLimit {
			lastError = strings.ToValidUTF8(lastError[:outboxErrorLimit], "")
		}
		done, next := 0, now.Add(outboxRetryDelay(attempts+1)).Unix()
		if created, parseErr := time.Parse(time.RFC3339Nano, createdAt); parseErr == nil && now.Sub(created) > outboxGiveUpAge {
			giveUp = true
			done, next = outboxFailed, 0
		}
		statement = "UPDATE payment_outbox SET lease_until=0, " + channel + "_attempts=?, " + channel + "_next_attempt_at=?, " + channel + "_last_error=?, " + channel + "_done=? WHERE id=?"
		args = []interface{}{attempts + 1, next, lastError, done, id}
	}
	releaseErr := query.Transaction(func(tx *query.Tx) error {
		_, err := tx.Exec(statement, args...)
		return err
	})
	if releaseErr != nil {
		return false, errors.Join(err, fmt.Errorf("release outbox lease: %w", releaseErr))
	}
	return giveUp, err
}

// outboxRetryDelay is 30s doubled for each further failure, capped at 1h.
func outboxRetryDelay(attempts int64) time.Duration {
	delay := outboxRetryBase
	for i := int64(1); i < attempts && delay < outboxRetryMax; i++ {
		delay *= 2
	}
	return min(delay, outboxRetryMax)
}

// redactOutboxError drops the path and query from failed request URLs:
// webhook URLs can carry secrets and Mailchimp paths identify the buyer.
func redactOutboxError(err error) error {
	var urlErr *url.Error
	if !errors.As(err, &urlErr) {
		return err
	}
	destination := "destination"
	if u, parseErr := url.Parse(urlErr.URL); parseErr == nil && u.Host != "" {
		destination = u.Scheme + "://" + u.Host
	}
	return fmt.Errorf("%s %s: %w", urlErr.Op, destination, urlErr.Err)
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
