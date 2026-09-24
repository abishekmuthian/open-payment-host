package subscriptions

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/abishekmuthian/open-payment-host/src/lib/query"
	"github.com/abishekmuthian/open-payment-host/src/lib/s3"
	"github.com/abishekmuthian/open-payment-host/src/products"
)

type ProviderFacts struct {
	Gateway        string
	OrderId        string
	PaymentId      string
	SubscriptionId string
	Amount         int64
	Currency       string
	PriceId        string
	Paid           bool
	Email          string
	Name           string
}

func validatePayment(a *PaymentAttempt, f ProviderFacts) error {
	if a == nil || f.Gateway != a.Gateway {
		return errors.New("payment gateway mismatch")
	}
	if !f.Paid || f.PaymentId == "" || f.Amount != a.Amount || !currencyEqual(f.Currency, a.Currency) {
		return errors.New("payment is unpaid or has incorrect amount or currency")
	}
	if a.PriceId != "" && f.PriceId != a.PriceId {
		return errors.New("payment price or plan mismatch")
	}
	return verifyProviderIds(a, f)
}
func verifyProviderIds(a *PaymentAttempt, f ProviderFacts) error {
	if a.Schedule != "onetime" && (f.SubscriptionId == "" || a.ProviderSubscriptionId != f.SubscriptionId) {
		return errors.New("subscription mismatch")
	}
	if f.OrderId != "" && f.OrderId != a.ProviderOrderId {
		return errors.New("order mismatch")
	}
	if a.Schedule == "onetime" {
		if a.ProviderOrderId != "" && f.OrderId != a.ProviderOrderId {
			return errors.New("missing or mismatched order")
		}
		if a.ProviderOrderId == "" && (a.ProviderPaymentId == "" || a.ProviderPaymentId != f.PaymentId) {
			return errors.New("payment mismatch")
		}
	}
	return nil
}
func currencyEqual(a, b string) bool { return a != "" && b != "" && strings.EqualFold(a, b) }

// verifyAndFulfill validates authenticated provider facts, then atomically
// records the transaction, counters, attempt state and durable outbound work.
// Both event retries and different events for the same transaction are safe.
func verifyAndFulfill(a *PaymentAttempt, f ProviderFacts) (bool, error) {
	if err := validatePayment(a, f); err != nil {
		return false, err
	}
	product, err := products.Find(a.ProductId)
	if err != nil {
		return false, err
	}
	changed := false
	err = query.Transaction(func(tx *query.Tx) error {
		// Acquire the write lock before reading state (also serializes SQLite writers).
		if _, err := tx.Exec("UPDATE payment_attempts SET id=id WHERE id=?", a.Id); err != nil {
			return err
		}
		var status, initial, completed string
		var counted int
		if err := tx.QueryRow("SELECT status,provider_payment_id,counted,COALESCE(completed_at,'') FROM payment_attempts WHERE id=?", a.Id).Scan(&status, &initial, &counted, &completed); err != nil {
			return err
		}
		var existing string
		err := tx.QueryRow("SELECT attempt_id FROM payment_transactions WHERE gateway=? AND transaction_id=?", a.Gateway, f.PaymentId).Scan(&existing)
		if err == nil {
			if existing != a.Id {
				return errors.New("transaction already belongs to another attempt")
			}
			a.Status = status
			return nil
		}
		if err != sql.ErrNoRows {
			return err
		}
		if a.Schedule == "onetime" && status != "pending" {
			return errors.New("one-time attempt already settled")
		}
		if status == "legacy" || status == "cancelled" || status == "refunded" || status == "expired" {
			return errors.New("attempt no longer accepts payments")
		}
		if completed == "" && a.Expired() {
			return errors.New("payment attempt expired")
		}
		now := query.TimeString(time.Now().UTC())
		_, err = tx.Exec("INSERT INTO payment_transactions(gateway,transaction_id,attempt_id,amount,currency) VALUES(?,?,?,?,?)", a.Gateway, f.PaymentId, a.Id, f.Amount, f.Currency)
		if err != nil {
			return err
		}
		_, err = tx.Exec(`INSERT INTO subscriptions(created_at,updated_at,pg,txn_id,payment_date,payment_gross,mc_currency,payment_status,txn_type,item_name,item_number,first_name,payer_email,user_id,subscr_id) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, now, now, a.Gateway, f.PaymentId, now, minorDecimal(f.Amount, f.Currency), f.Currency, "ACTIVE", a.Schedule, product.Name, a.ProductId, f.Name, f.Email, a.CustomId, f.SubscriptionId)
		if err != nil {
			return err
		}
		if counted == 0 {
			column := "total_subscribers"
			if a.Schedule == "onetime" {
				column = "total_onetime_payments"
			}
			result, err := tx.Exec("UPDATE products SET "+column+"=COALESCE("+column+",0)+1 WHERE id=?", a.ProductId)
			if err != nil {
				return err
			}
			n, err := result.RowsAffected()
			if err != nil {
				return err
			}
			if n != 1 {
				return errors.New("product no longer exists")
			}
		}
		first := completed == ""
		if first {
			initial = f.PaymentId
		}
		_, err = tx.Exec("UPDATE payment_attempts SET status='completed',provider_payment_id=?,counted=1,completed_at=?,updated_at=? WHERE id=?", initial, now, now, a.Id)
		if err != nil {
			return err
		}
		if first || counted == 0 {
			eventType := EventPaymentActive
			if a.Schedule != "onetime" {
				eventType = EventSubscriptionActive
			}
			if err := enqueuePaymentEffect(tx, a, f, eventType, "active"); err != nil {
				return err
			}
		}
		a.ProviderPaymentId = initial
		a.Status = "completed"
		changed = true
		return nil
	})
	return changed, err
}

func enqueuePaymentEffect(tx *query.Tx, a *PaymentAttempt, f ProviderFacts, eventType, status string) error {
	payload := map[string]interface{}{"custom_id": a.CustomId, "status": status, "email": f.Email}
	if a.Schedule == "onetime" {
		payload["order_id"] = a.ProviderOrderId
	} else {
		payload["subscription_id"] = a.ProviderSubscriptionId
		if status == "active" {
			if token := a.cancellationToken(); token != "" {
				payload["cancellation_token"] = token
			}
		}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	encrypted, err := sealCapability(string(body))
	if err != nil {
		return err
	}
	id, err := generateToken(24)
	if err != nil {
		return err
	}
	_, err = tx.Exec("INSERT INTO payment_outbox(id,attempt_id,event_type,created_at,payload) VALUES(?,?,?,?,?)", id, a.Id, eventType, time.Now().UTC().Format("2006-01-02T15:04:05.000000000Z"), encrypted)
	return err
}

func generateDownloadURL(product *products.Story) (string, error) {
	if product.S3Bucket == "" || product.S3Key == "" {
		return "", nil
	}
	return s3.GeneratePresignedUrl(product.S3Bucket, product.S3Key)
}

// applySubscriptionStatus handles verified provider states, including legacy
// records found only by their stored gateway and subscription ID.
func applySubscriptionStatus(gateway, subscriptionID, status string) error {
	if subscriptionID == "" {
		return errors.New("missing subscription id")
	}
	a, err := FindAttemptByProviderSubscription(gateway, subscriptionID)
	if err != nil {
		return applyLegacyStatus(gateway, subscriptionID, status)
	}
	if a.Status == "legacy" {
		return applyLegacyStatus(gateway, subscriptionID, status)
	}
	// ACTIVE is not proof of a paid first invoice, nor of a paid renewal.
	if status == "active" {
		return nil
	}
	return query.Transaction(func(tx *query.Tx) error {
		if _, err := tx.Exec("UPDATE payment_attempts SET id=id WHERE id=?", a.Id); err != nil {
			return err
		}
		var previous string
		var counted int
		if err := tx.QueryRow("SELECT status,counted FROM payment_attempts WHERE id=?", a.Id).Scan(&previous, &counted); err != nil {
			return err
		}
		if previous == status || previous == "cancelled" || previous == "expired" {
			return nil
		}
		if counted != 0 {
			if _, err := tx.Exec("UPDATE products SET total_subscribers=CASE WHEN total_subscribers>0 THEN total_subscribers-1 ELSE 0 END WHERE id=?", a.ProductId); err != nil {
				return err
			}
		}
		if _, err := tx.Exec("UPDATE payment_attempts SET status=?,counted=0 WHERE id=?", status, a.Id); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE subscriptions SET payment_status=? WHERE pg=? AND subscr_id=?", strings.ToUpper(status), gateway, subscriptionID); err != nil {
			return err
		}
		if counted != 0 {
			return enqueuePaymentEffect(tx, a, ProviderFacts{}, "subscription."+status, status)
		}
		return nil
	})
}
func applyLegacyStatus(gateway, id, status string) error {
	if status == "active" {
		return nil
	}
	stored, err := FindFirst("pg=? AND subscr_id=?", gateway, id)
	if err != nil {
		return err
	}
	a, err := FindAttemptByProviderSubscription(gateway, id)
	if err != nil {
		// Legacy status processing gets a non-fulfillable correlation record.
		// Only the stored subscription supplies product and customer identity.
		a, err = newAttempt(stored.ProductId, gateway, "monthly", "DF", 1, "USD", "", stored.UserId, "")
		if err != nil {
			return err
		}
		if err := attemptQuery().Where("id=?", a.Id).Update(map[string]string{"status": "legacy"}); err != nil {
			return err
		}
		if err := a.SetProviderIds("", "", id); err != nil {
			return err
		}
	}
	return query.Transaction(func(tx *query.Tx) error {
		if _, err := tx.Exec("UPDATE payment_attempts SET id=id WHERE id=?", a.Id); err != nil {
			return err
		}
		var productID int64
		var previous string
		err := tx.QueryRow("SELECT item_number,payment_status FROM subscriptions WHERE pg=? AND subscr_id=? ORDER BY id DESC LIMIT 1", gateway, id).Scan(&productID, &previous)
		if err == sql.ErrNoRows {
			return nil
		}
		if err != nil {
			return err
		}
		if strings.EqualFold(previous, "active") {
			if _, err := tx.Exec("UPDATE products SET total_subscribers=CASE WHEN total_subscribers>0 THEN total_subscribers-1 ELSE 0 END WHERE id=?", productID); err != nil {
				return err
			}
		}
		if strings.EqualFold(previous, status) {
			return nil
		}
		if _, err = tx.Exec("UPDATE subscriptions SET payment_status=? WHERE pg=? AND subscr_id=?", strings.ToUpper(status), gateway, id); err != nil {
			return err
		}
		return enqueuePaymentEffect(tx, a, ProviderFacts{Email: stored.CustomerEmail}, "subscription."+status, status)
	})
}

func applyRefund(gateway, transactionID, refundID string, amount int64, currency string) error {
	if transactionID == "" || refundID == "" || amount <= 0 {
		return errors.New("invalid refund")
	}
	var a *PaymentAttempt
	cols, err := query.New("payment_transactions", "transaction_id").Where("gateway=? AND transaction_id=?", gateway, transactionID).FirstResult()
	if err != nil {
		return err
	}
	a, err = FindAttempt(fmt.Sprint(cols["attempt_id"]))
	if err != nil {
		return err
	}
	return query.Transaction(func(tx *query.Tx) error {
		if _, err := tx.Exec("UPDATE payment_attempts SET id=id WHERE id=?", a.Id); err != nil {
			return err
		}
		duplicate, err := eventExists(tx, gateway, "refund:"+refundID)
		if err != nil || duplicate {
			return err
		}
		var total, refunded int64
		var c string
		if err := tx.QueryRow("SELECT amount,refunded,currency FROM payment_transactions WHERE gateway=? AND transaction_id=?", gateway, transactionID).Scan(&total, &refunded, &c); err != nil {
			return err
		}
		if !currencyEqual(currency, c) || amount > total-refunded {
			return errors.New("refund amount or currency mismatch")
		}
		if _, err := tx.Exec("UPDATE payment_transactions SET refunded=refunded+? WHERE gateway=? AND transaction_id=?", amount, gateway, transactionID); err != nil {
			return err
		}
		status := "partially_refunded"
		if refunded+amount == total {
			status = "refunded"
			if a.Schedule == "onetime" || a.ProviderPaymentId == transactionID {
				var counted int
				if err := tx.QueryRow("SELECT counted FROM payment_attempts WHERE id=?", a.Id).Scan(&counted); err != nil {
					return err
				}
				if counted != 0 {
					column := "total_subscribers"
					if a.Schedule == "onetime" {
						column = "total_onetime_payments"
					}
					if _, err := tx.Exec("UPDATE products SET "+column+"=CASE WHEN "+column+">0 THEN "+column+"-1 ELSE 0 END WHERE id=?", a.ProductId); err != nil {
						return err
					}
				}
				if _, err := tx.Exec("UPDATE payment_attempts SET status='refunded',counted=0 WHERE id=?", a.Id); err != nil {
					return err
				}
			}
		}
		if _, err := tx.Exec("UPDATE subscriptions SET payment_status=? WHERE pg=? AND txn_id=?", strings.ToUpper(status), gateway, transactionID); err != nil {
			return err
		}
		kind := "payment."
		if a.Schedule != "onetime" {
			kind = "subscription."
		}
		if err := enqueuePaymentEffect(tx, a, ProviderFacts{}, kind+status, status); err != nil {
			return err
		}
		return recordEvent(tx, gateway, "refund:"+refundID)
	})
}
