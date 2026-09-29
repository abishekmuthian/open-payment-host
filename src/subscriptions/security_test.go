package subscriptions

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/abishekmuthian/open-payment-host/src/internal/testenv"
	"github.com/abishekmuthian/open-payment-host/src/lib/query"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/products"
)

func setupPayments(t *testing.T) {
	t.Helper()
	testenv.Config(t, map[string]string{
		"square_signature_key":        "test-signature-key",
		"square_notification_url":     "https://merchant.test/subscriptions/square-webhook",
		"subscription_client_country": "",
	})
	// Real baseline tables plus every release migration, never the user's DB.
	testenv.DB(t)
	testenv.Exec(t, `INSERT INTO products(id,name,schedule,total_onetime_payments,total_subscribers) VALUES(1,'Paid download','onetime',0,0)`)
}
func fixtureAttempt(t *testing.T, gateway, schedule string) (*PaymentAttempt, ProviderFacts) {
	t.Helper()
	return fixtureProductAttempt(t, 1, gateway, schedule)
}
func fixtureProductAttempt(t *testing.T, productID int64, gateway, schedule string) (*PaymentAttempt, ProviderFacts) {
	t.Helper()
	a, err := newAttempt(productID, gateway, schedule, "DF", 1050, "USD", "", "customer&1", "https://client.test/return")
	if err != nil {
		t.Fatal(err)
	}
	f := ProviderFacts{Gateway: gateway, Amount: 1050, Currency: "USD", Paid: true, PaymentId: "pay_" + a.Id}
	if schedule == "onetime" {
		f.OrderId = "order_" + a.Id
	} else {
		f.SubscriptionId = "sub_" + a.Id
	}
	if err := a.SetProviderIds(f.OrderId, "", f.SubscriptionId); err != nil {
		t.Fatal(err)
	}
	return a, f
}
func TestVerifiedPaymentsAllGatewaysAndSchedules(t *testing.T) {
	setupPayments(t)
	for _, gateway := range []string{"stripe", "square", "paypal", "razorpay"} {
		for _, schedule := range []string{"onetime", "monthly", "yearly"} {
			t.Run(gateway+"/"+schedule, func(t *testing.T) {
				a, f := fixtureAttempt(t, gateway, schedule)
				if changed, err := verifyAndFulfill(a, f); err != nil || !changed {
					t.Fatalf("fulfill: %v, %v", changed, err)
				}
				if changed, err := verifyAndFulfill(a, f); err != nil || changed {
					t.Fatalf("replay: %v, %v", changed, err)
				}
			})
		}
	}
	if n := testenv.Scalar(t, "SELECT COUNT(*) FROM subscriptions"); n != 12 {
		t.Fatalf("transactions=%d", n)
	}
	if n := testenv.Scalar(t, "SELECT total_onetime_payments FROM products WHERE id=1"); n != 4 {
		t.Fatalf("one-time count=%d", n)
	}
	if n := testenv.Scalar(t, "SELECT total_subscribers FROM products WHERE id=1"); n != 8 {
		t.Fatalf("subscriber count=%d", n)
	}
	if n := testenv.Scalar(t, "SELECT COUNT(*) FROM payment_outbox"); n != 12 {
		t.Fatalf("outbox=%d", n)
	}
}
func TestPaymentMismatchNeverFulfills(t *testing.T) {
	setupPayments(t)
	for _, gateway := range []string{"stripe", "square", "paypal", "razorpay"} {
		for _, mutation := range []string{"unpaid", "amount", "currency", "gateway", "order", "price", "subscription", "expired"} {
			t.Run(gateway+"/"+mutation, func(t *testing.T) {
				schedule := "onetime"
				if mutation == "subscription" {
					schedule = "monthly"
				}
				a, f := fixtureAttempt(t, gateway, schedule)
				switch mutation {
				case "unpaid":
					f.Paid = false
				case "amount":
					f.Amount = 1
				case "currency":
					f.Currency = "INR"
				case "gateway":
					f.Gateway = "other"
				case "order":
					f.OrderId = "victim_order"
				case "price":
					a.PriceId = "price_expensive"
					f.PriceId = "price_cheap"
				case "subscription":
					f.SubscriptionId = "victim_sub"
				case "expired":
					a.ExpiresAt = time.Now().Add(-time.Hour)
				}
				if _, err := verifyAndFulfill(a, f); err == nil {
					t.Fatal("invalid payment fulfilled")
				}
			})
		}
	}
	if n := testenv.Scalar(t, "SELECT COUNT(*) FROM subscriptions"); n != 0 {
		t.Fatalf("unexpected transactions %d", n)
	}
}
func TestFulfillmentRollbackAndConcurrentRetry(t *testing.T) {
	setupPayments(t)
	a, f := fixtureAttempt(t, "paypal", "onetime")
	if _, err := query.ExecSQL(`CREATE TRIGGER reject_transaction BEFORE INSERT ON subscriptions BEGIN SELECT RAISE(ABORT,'test failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := verifyAndFulfill(a, f); err == nil {
		t.Fatal("expected DB failure")
	}
	stored, err := FindAttempt(a.Id)
	if err != nil || stored.Status != "pending" {
		t.Fatalf("attempt committed despite failure: %v %v", stored, err)
	}
	if testenv.Scalar(t, "SELECT COUNT(*) FROM payment_transactions") != 0 {
		t.Fatal("partial transaction committed")
	}
	testenv.Exec(t, "DROP TRIGGER reject_transaction")
	var wg sync.WaitGroup
	errs := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); copy := *a; _, err := verifyAndFulfill(&copy, f); errs <- err }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if testenv.Scalar(t, "SELECT COUNT(*) FROM subscriptions") != 1 || testenv.Scalar(t, "SELECT total_onetime_payments FROM products WHERE id=1") != 1 {
		t.Fatal("concurrent duplicate side effects")
	}
}
func TestCookieRequiresSecretAndExpiresOnServer(t *testing.T) {
	setupPayments(t)
	a, _ := fixtureAttempt(t, "paypal", "onetime")
	for _, value := range []string{a.Id, a.Id + ".wrong"} {
		r := httptest.NewRequest("GET", "/subscriptions/payment-status?attempt_id="+a.Id, nil)
		r.AddCookie(&http.Cookie{Name: AttemptCookie, Value: value})
		if bindAttemptToRequest(r, a) {
			t.Fatal("public attempt id authorized browser")
		}
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	setAttemptCookie(w, r, a)
	r.AddCookie(w.Result().Cookies()[0])
	if !bindAttemptToRequest(r, a) {
		t.Fatal("valid browser rejected")
	}
	w = httptest.NewRecorder()
	if err := issueCompletionCookie(w, r, a); err != nil {
		t.Fatal(err)
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == CompletionCookie {
			if !c.HttpOnly || c.SameSite != http.SameSiteLaxMode {
				t.Fatal("unsafe cookie")
			}
			r.AddCookie(c)
		}
	}
	if !completionCookieValid(r, a) {
		t.Fatal("valid completion rejected")
	}
	a.CompletionExpiresAt = time.Now().Add(-time.Second).Unix()
	if completionCookieValid(r, a) {
		t.Fatal("expired completion accepted")
	}
	if a.CreatedAt.IsZero() {
		t.Fatal("creation timestamp missing")
	}
}
func TestEventFailureRemainsRetryable(t *testing.T) {
	setupPayments(t)
	calls := 0
	process := func() error {
		calls++
		if calls == 1 {
			return errors.New("provider unavailable")
		}
		return nil
	}
	if processPaymentEvent("square", "event_1", process) == nil {
		t.Fatal("expected retryable failure")
	}
	if err := processPaymentEvent("square", "event_1", process); err != nil {
		t.Fatal(err)
	}
	if err := processPaymentEvent("square", "event_1", process); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("calls=%d", calls)
	}
}
func TestSquareSignatureUsesExactRawBody(t *testing.T) {
	setupPayments(t)
	body := []byte("{\n  \"event_id\": \"event\"\n}")
	h := hmac.New(sha256.New, []byte(config.Get("square_signature_key")))
	h.Write([]byte(config.Get("square_notification_url")))
	h.Write(body)
	sig := base64.StdEncoding.EncodeToString(h.Sum(nil))
	if !isFromSquare(sig, body) {
		t.Fatal("raw signature rejected")
	}
	if isFromSquare(sig, []byte(`{"event_id":"event"}`)) {
		t.Fatal("altered body accepted")
	}
	config.Configuration(0)["square_notification_url"] = "https://wrong.test/webhook"
	if isFromSquare(sig, body) {
		t.Fatal("wrong notification URL accepted")
	}
}
func TestRedirectAndMoney(t *testing.T) {
	setupPayments(t)
	p := &products.Story{WebhookURL: "https://client.test/hook", AllowedRedirectOrigins: "https://other.test"}
	for _, raw := range []string{"https://client.test/done?foo=1#fragment", "https://other.test/done"} {
		if _, err := ValidateRedirectURI(p, raw); err != nil {
			t.Fatal(err)
		}
	}
	for _, raw := range []string{"https://client.test.attacker.test/done", "https://user:pw@client.test/done", "//client.test/path", "javascript:alert(1)", "https://unlisted.test/"} {
		if _, err := ValidateRedirectURI(p, raw); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	local := &products.Story{WebhookURL: "http://localhost:3000/api/webhooks/oph"}
	if _, err := ValidateRedirectURI(local, "http://localhost:3000/payment/success"); err != nil {
		t.Fatalf("development webhook origin rejected: %v", err)
	}
	oldMode := config.Current.Mode
	t.Cleanup(func() { config.Current.Mode = oldMode })
	config.Current.Mode = config.ModeProduction
	if _, err := ValidateRedirectURI(local, "http://localhost:3000/payment/success"); err == nil {
		t.Fatal("production accepted HTTP webhook origin")
	}
	config.Current.Mode = oldMode
	u := BuildRedirectURL("https://client.test/done?custom_id=old#fragment", map[string]string{"custom_id": "a&b?c#d"})
	if u != "https://client.test/done?custom_id=a%26b%3Fc%23d#fragment" {
		t.Fatal(u)
	}
	for _, test := range []struct {
		value, currency string
		want            int64
	}{{"10.50", "USD", 1050}, {"12", "JPY", 12}, {"1.234", "KWD", 1234}} {
		n, err := majorValueToMinor(test.value, test.currency)
		if err != nil || n != test.want {
			t.Fatalf("%+v: %d %v", test, n, err)
		}
	}
	for _, raw := range []string{"-1", "1,25", "1.001", "+1", "1e2", "NaN", "9223372036854775807"} {
		if _, err := majorValueToMinor(raw, "USD"); err == nil {
			t.Fatalf("accepted amount %s", raw)
		}
	}
}

type paymentRoundTripper func(*http.Request) (*http.Response, error)

func (f paymentRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func mockPayments(t *testing.T, f func(*http.Request) string) {
	t.Helper()
	old := paymentHTTPClient
	paymentHTTPClient = &http.Client{Transport: paymentRoundTripper(func(r *http.Request) (*http.Response, error) {
		body := f(r)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	t.Cleanup(func() { paymentHTTPClient = old })
}
func signedRazorpayWebhook(t *testing.T, body, eventID string) *http.Request {
	t.Helper()
	h := hmac.New(sha256.New, []byte(config.Get("razorpay_webhook_secret")))
	_, _ = h.Write([]byte(body))
	r := httptest.NewRequest(http.MethodPost, "/subscriptions/razorpay-webhook", strings.NewReader(body))
	r.Header.Set("X-Razorpay-Signature", hex.EncodeToString(h.Sum(nil)))
	r.Header.Set("X-Razorpay-Event-Id", eventID)
	return r
}

func TestRazorpayOrderPaidSeparatesSubscriptionInvoicesFromOneTimeOrders(t *testing.T) {
	setupPayments(t)
	config.Configuration(0)["razorpay_webhook_secret"] = "webhook-secret"

	monthly, _ := fixtureAttempt(t, "razorpay", "monthly")
	oneTime, _ := fixtureAttempt(t, "razorpay", "onetime")
	providerCalls := 0
	mockPayments(t, func(r *http.Request) string {
		providerCalls++
		switch {
		case strings.Contains(r.URL.Path, "/payments/payment-one"):
			return fmt.Sprintf(`{"id":"payment-one","order_id":%q,"status":"captured","captured":true,"amount":1050,"currency":"USD","amount_refunded":0}`, oneTime.ProviderOrderId)
		case strings.Contains(r.URL.Path, "/orders/"+oneTime.ProviderOrderId):
			return fmt.Sprintf(`{"id":%q,"receipt":%q,"status":"paid","amount":1050,"amount_paid":1050,"currency":"USD"}`, oneTime.ProviderOrderId, oneTime.Id)
		}
		t.Fatalf("unexpected provider request %s", r.URL)
		return ""
	})

	recurringBody := `{"event":"order.paid","payload":{"order":{"entity":{"id":"invoice-order"}},"payment":{"entity":{"id":"invoice-payment","order_id":"invoice-order","invoice_id":"invoice"}}}}`
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		if err := HandleRazorpayWebhook(w, signedRazorpayWebhook(t, recurringBody, "recurring-event")); err != nil {
			t.Fatal(err)
		}
		if w.Code != http.StatusOK {
			t.Fatalf("recurring order.paid status=%d", w.Code)
		}
	}
	if providerCalls != 0 {
		t.Fatalf("invoice-backed order made %d provider calls", providerCalls)
	}
	storedMonthly, err := FindAttempt(monthly.Id)
	if err != nil {
		t.Fatal(err)
	}
	if storedMonthly.Status != AttemptStatusPending || testenv.Scalar(t, "SELECT COUNT(*) FROM payment_events") != 1 || testenv.Scalar(t, "SELECT COUNT(*) FROM payment_transactions") != 0 || testenv.Scalar(t, "SELECT COUNT(*) FROM subscriptions") != 0 || testenv.Scalar(t, "SELECT COUNT(*) FROM payment_outbox") != 0 {
		t.Fatal("invoice-backed order event changed payment state")
	}

	malformed := []string{
		`{"event":"order.paid","payload":{"order":{"entity":{"id":"invoice-order"}},"payment":{"entity":{"order_id":"invoice-order","invoice_id":"invoice"}}}}`,
		`{"event":"order.paid","payload":{"order":{"entity":{"id":"invoice-order"}},"payment":{"entity":{"id":"invoice-payment","order_id":"different-order","invoice_id":"invoice"}}}}`,
	}
	for i, body := range malformed {
		w := httptest.NewRecorder()
		if err := HandleRazorpayWebhook(w, signedRazorpayWebhook(t, body, fmt.Sprintf("malformed-%d", i))); err != nil {
			t.Fatal(err)
		}
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("malformed invoice-backed order status=%d", w.Code)
		}
	}
	if testenv.Scalar(t, "SELECT COUNT(*) FROM payment_events") != 1 || providerCalls != 0 {
		t.Fatal("malformed invoice-backed event was accepted")
	}

	oneTimeBody := fmt.Sprintf(`{"event":"order.paid","payload":{"order":{"entity":{"id":%q}},"payment":{"entity":{"id":"payment-one","order_id":%q}}}}`, oneTime.ProviderOrderId, oneTime.ProviderOrderId)
	w := httptest.NewRecorder()
	if err := HandleRazorpayWebhook(w, signedRazorpayWebhook(t, oneTimeBody, "one-time-event")); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("one-time order.paid status=%d", w.Code)
	}
	completed, err := FindAttempt(oneTime.Id)
	if err != nil {
		t.Fatal(err)
	}
	if providerCalls != 2 || completed.Status != AttemptStatusCompleted || testenv.Scalar(t, "SELECT COUNT(*) FROM payment_transactions") != 1 || testenv.Scalar(t, "SELECT COUNT(*) FROM subscriptions") != 1 || testenv.Scalar(t, "SELECT COUNT(*) FROM payment_outbox") != 1 || testenv.Scalar(t, "SELECT total_onetime_payments FROM products WHERE id=1") != 1 {
		t.Fatal("one-time order event was not fulfilled normally")
	}
}

func TestCancellationCapabilityConsumedBeforeProviderCall(t *testing.T) {
	setupPayments(t)
	a, _ := fixtureAttempt(t, "paypal", "monthly")
	token, err := openCapability(a.CancellationTokenCiphertext)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	mockPayments(t, func(r *http.Request) string {
		if strings.Contains(r.URL.Path, "oauth2") {
			return `{"access_token":"test"}`
		}
		calls++
		return `{}`
	})
	if err := CancelAttempt(a, "wrong"); err == nil {
		t.Fatal("invalid token accepted")
	}
	if calls != 0 {
		t.Fatal("unauthorized provider call")
	}
	if err := CancelAttempt(a, token); err != nil {
		t.Fatal(err)
	}
	if err := CancelAttempt(a, token); err == nil {
		t.Fatal("reused token accepted")
	}
	if calls != 1 {
		t.Fatalf("provider calls=%d", calls)
	}
}
func TestWebhookBodyAndStableDeliveryID(t *testing.T) {
	setupPayments(t)
	a, f := fixtureAttempt(t, "paypal", "monthly")
	cancellationURL, err := url.Parse(a.CancellationURL())
	if err != nil {
		t.Fatal(err)
	}
	if !a.CancellationTokenValid(cancellationURL.Query().Get("cancellation_token")) || cancellationURL.Query().Get("token") != "" {
		t.Fatal("generated cancellation URL uses an invalid parameter")
	}
	testenv.Exec(t, `UPDATE products SET webhook_url='https://client.test/hook',webhook_secret='test-secret' WHERE id=1`)
	if _, err := verifyAndFulfill(a, f); err != nil {
		t.Fatal(err)
	}
	var firstID, firstBody string
	calls := 0
	old := paymentHTTPClient
	t.Cleanup(func() { paymentHTTPClient = old })
	paymentHTTPClient = &http.Client{Transport: paymentRoundTripper(func(r *http.Request) (*http.Response, error) {
		calls++
		b, _ := io.ReadAll(r.Body)
		id := r.Header.Get("X-OPH-Event-ID")
		if id == "" || r.Header.Get("Content-Type") != "application/json" || r.Header.Get("X-OPH-Signature") != GenerateSignature(b, "test-secret") {
			t.Error("invalid webhook headers")
		}
		var payload map[string]string
		if err := json.Unmarshal(b, &payload); err != nil {
			t.Error(err)
		}
		if payload["subscription_id"] != a.ProviderSubscriptionId || payload["custom_id"] != "customer&1" || !a.CancellationTokenValid(payload["cancellation_token"]) || payload["cancellation_url"] != "" {
			t.Error("incompatible webhook payload")
		}
		status := 200
		if calls == 1 {
			firstID = id
			firstBody = string(b)
			status = 503
		} else if id != firstID || string(b) != firstBody {
			t.Error("delivery identity changed across retry")
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})}
	if deliverPaymentEffects() == nil {
		t.Fatal("non-2xx was acknowledged")
	}
	elapseOutboxBackoff(t)
	if err := deliverPaymentEffects(); err != nil {
		t.Fatal(err)
	}
	if err := deliverPaymentEffects(); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("deliveries=%d", calls)
	}
}
func TestMailingFailureDoesNotBlockWebhooksAndPreservesOrder(t *testing.T) {
	setupPayments(t)
	config.Configuration(config.ModeDevelopment)["mailchimp_token"] = "test-us1"
	testenv.Exec(t, `UPDATE products SET webhook_url='https://client.test/hook',webhook_secret='test-secret',mailchimp_audience_id='missing-list' WHERE id=1`)
	older, olderFacts := fixtureAttempt(t, "paypal", "monthly")
	olderFacts.Email = "older@example.com"
	if _, err := verifyAndFulfill(older, olderFacts); err != nil {
		t.Fatal(err)
	}
	testenv.Exec(t, fmt.Sprintf("UPDATE payment_outbox SET webhook_done=1 WHERE attempt_id='%s'", older.Id))
	newer, newerFacts := fixtureAttempt(t, "paypal", "monthly")
	newerFacts.Email = "newer@example.com"
	if _, err := verifyAndFulfill(newer, newerFacts); err != nil {
		t.Fatal(err)
	}
	webhookCalls := 0
	mailingCalls := 0
	old := paymentHTTPClient
	t.Cleanup(func() { paymentHTTPClient = old })
	paymentHTTPClient = &http.Client{Transport: paymentRoundTripper(func(r *http.Request) (*http.Response, error) {
		status := http.StatusOK
		switch r.URL.Host {
		case "client.test":
			webhookCalls++
		case "us1.api.mailchimp.com":
			mailingCalls++
			status = http.StatusNotFound
		default:
			t.Fatalf("unexpected delivery request %s", r.URL)
		}
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}, nil
	})}
	err := deliverPaymentEffects()
	if err == nil || !strings.Contains(err.Error(), "mailing delivery") {
		t.Fatalf("expected labelled mailing failure, got %v", err)
	}
	if webhookCalls != 1 || mailingCalls != 1 {
		t.Fatalf("webhook calls=%d mailing calls=%d", webhookCalls, mailingCalls)
	}
	if n := testenv.Scalar(t, "SELECT COUNT(*) FROM payment_outbox WHERE webhook_done=1"); n != 2 {
		t.Fatalf("delivered webhooks=%d", n)
	}
	if n := testenv.Scalar(t, "SELECT COUNT(*) FROM payment_outbox WHERE mailing_done=1"); n != 0 {
		t.Fatalf("mailing deliveries passed failed predecessor=%d", n)
	}
}
func TestWebhookFailureDoesNotBlockMailingAndPreservesOrder(t *testing.T) {
	setupPayments(t)
	testenv.Exec(t, `UPDATE products SET webhook_url='https://client.test/hook',webhook_secret='test-secret' WHERE id=1`)
	for _, email := range []string{"older@example.com", "newer@example.com"} {
		a, facts := fixtureAttempt(t, "paypal", "monthly")
		facts.Email = email
		if _, err := verifyAndFulfill(a, facts); err != nil {
			t.Fatal(err)
		}
	}
	webhookCalls := 0
	old := paymentHTTPClient
	t.Cleanup(func() { paymentHTTPClient = old })
	paymentHTTPClient = &http.Client{Transport: paymentRoundTripper(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "client.test" {
			t.Fatalf("unexpected delivery request %s", r.URL)
		}
		webhookCalls++
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}, nil
	})}
	err := deliverPaymentEffects()
	if err == nil || !strings.Contains(err.Error(), "webhook delivery") {
		t.Fatalf("expected labelled webhook failure, got %v", err)
	}
	if webhookCalls != 1 {
		t.Fatalf("later webhook passed failed predecessor: calls=%d", webhookCalls)
	}
	if n := testenv.Scalar(t, "SELECT COUNT(*) FROM payment_outbox WHERE webhook_done=1"); n != 0 {
		t.Fatalf("failed webhooks acknowledged=%d", n)
	}
	if n := testenv.Scalar(t, "SELECT COUNT(*) FROM payment_outbox WHERE mailing_done=1"); n != 2 {
		t.Fatalf("mailing deliveries blocked by webhook=%d", n)
	}
}
func TestRenewalRefundAndCancellationIdempotency(t *testing.T) {
	setupPayments(t)
	for _, gateway := range []string{"stripe", "square", "paypal", "razorpay"} {
		a, f := fixtureAttempt(t, gateway, "monthly")
		if _, err := verifyAndFulfill(a, f); err != nil {
			t.Fatal(err)
		}
		f.PaymentId = "renewal_" + a.Id
		for i := 0; i < 2; i++ {
			if _, err := verifyAndFulfill(a, f); err != nil {
				t.Fatal(err)
			}
		}
		for i := 0; i < 2; i++ {
			if err := applyRefund(gateway, f.PaymentId, "refund_"+a.Id, 1050, "USD"); err != nil {
				t.Fatal(err)
			}
		}
		if err := applySubscriptionStatus(gateway, a.ProviderSubscriptionId, "cancelled"); err != nil {
			t.Fatal(err)
		}
		cancelled, err := FindAttempt(a.Id)
		if err != nil {
			t.Fatal(err)
		}
		if cancelled.CancelledAt.IsZero() || cancelled.CancelledAt.Location() != time.UTC {
			t.Fatalf("%s cancellation time=%v", gateway, cancelled.CancelledAt)
		}
		time.Sleep(2 * time.Millisecond)
		if err := applySubscriptionStatus(gateway, a.ProviderSubscriptionId, "cancelled"); err != nil {
			t.Fatal(err)
		}
		duplicate, err := FindAttempt(a.Id)
		if err != nil {
			t.Fatal(err)
		}
		if !duplicate.CancelledAt.Equal(cancelled.CancelledAt) {
			t.Fatalf("%s duplicate changed cancellation time: %v != %v", gateway, duplicate.CancelledAt, cancelled.CancelledAt)
		}
	}
	if n := testenv.Scalar(t, "SELECT COUNT(*) FROM subscriptions"); n != 8 {
		t.Fatalf("transactions=%d", n)
	}
	if n := testenv.Scalar(t, "SELECT total_subscribers FROM products WHERE id=1"); n != 0 {
		t.Fatalf("subscribers=%d", n)
	}
}
func TestPayPalOrderRejectsWrongReferenceAndSKU(t *testing.T) {
	setupPayments(t)
	a, _ := fixtureAttempt(t, "paypal", "onetime")
	body := fmt.Sprintf(`{"id":%q,"status":"COMPLETED","purchase_units":[{"reference_id":%q,"amount":{"value":"10.50","currency_code":"USD"},"items":[{"sku":"1","quantity":"1"}],"payments":{"captures":[{"id":"capture","status":"COMPLETED","final_capture":true,"amount":{"value":"10.50","currency_code":"USD"}}]}}]}`, a.ProviderOrderId, a.Id)
	var o PayPalOrderDetailsResult
	if err := json.Unmarshal([]byte(body), &o); err != nil {
		t.Fatal(err)
	}
	if _, err := paypalOrderFacts(&o, a); err != nil {
		t.Fatal(err)
	}
	o.PurchaseUnits[0].Items[0].Sku = "2"
	if _, err := paypalOrderFacts(&o, a); err == nil {
		t.Fatal("product substitution accepted")
	}
	o.PurchaseUnits = nil
	if _, err := paypalOrderFacts(&o, a); err == nil {
		t.Fatal("empty purchase units accepted")
	}
}
