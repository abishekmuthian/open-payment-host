package subscriptions

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	EventPaymentActive         = "payment.active"
	EventSubscriptionActive    = "subscription.active"
	EventSubscriptionCancelled = "subscription.cancelled"
)

var paymentHTTPClient = &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}

func sendProductWebhook(destination, secret, id, eventType, created string, body []byte) error {
	req, err := http.NewRequest(http.MethodPost, destination, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-OPH-Signature", GenerateSignature(body, secret))
	req.Header.Set("X-OPH-Event-ID", id)
	req.Header.Set("X-OPH-Event-Type", eventType)
	req.Header.Set("X-OPH-Timestamp", created)
	resp, err := paymentHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("product webhook returned HTTP %d", resp.StatusCode)
	}
	return nil
}
func GenerateSignature(body []byte, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}
