package subscriptions

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/abishekmuthian/open-payment-host/src/internal/testenv"
	"github.com/stripe/stripe-go/v72/webhook"
)

const (
	stripeWebhookSecret = "whsec_test"
	razorpaySecret      = "webhook-secret"
)

type webhookHandler func(http.ResponseWriter, *http.Request) error

// setupWebhooks configures every gateway's webhook credentials.
func setupWebhooks(t *testing.T) {
	t.Helper()
	setupPayments(t)
	for k, v := range map[string]string{
		"stripe_webhook_secret": stripeWebhookSecret, "stripe_secret": "sk_test",
		"paypal_webhook_id": "WH-1", "paypal_client_id": "client", "paypal_client_secret": "secret",
		"razorpay_webhook_secret": razorpaySecret, "razorpay_key_id": "rzp_key", "razorpay_key_secret": "rzp_secret",
		"square_access_token": "sq_token",
	} {
		testenv.Set(k, v)
	}
}

func deliver(t *testing.T, h webhookHandler, r *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	if err := h(w, r); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	return w
}

func stripeSigned(body string) *http.Request {
	now := time.Now()
	sig := hex.EncodeToString(webhook.ComputeSignature(now, []byte(body), stripeWebhookSecret))
	r := httptest.NewRequest(http.MethodPost, "/subscriptions/stripe-webhook", strings.NewReader(body))
	r.Header.Set("Stripe-Signature", fmt.Sprintf("t=%d,v1=%s", now.Unix(), sig))
	return r
}

func squareSigned(body string) *http.Request {
	h := hmac.New(sha256.New, []byte("test-signature-key"))
	h.Write([]byte("https://merchant.test/subscriptions/square-webhook" + body))
	r := httptest.NewRequest(http.MethodPost, "/subscriptions/square-webhook", strings.NewReader(body))
	r.Header.Set("x-square-hmacsha256-signature", base64.StdEncoding.EncodeToString(h.Sum(nil)))
	return r
}

func paypalEvent(body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/subscriptions/paypal-webhook", strings.NewReader(body))
	r.Header.Set("PAYPAL-TRANSMISSION-ID", "tx")
	return r
}

// providerAPI answers paymentHTTPClient calls by "METHOD path" and counts them.
// PayPal OAuth and webhook verification are answered automatically.
type providerAPI struct {
	mu     sync.Mutex
	routes map[string]string
	calls  map[string]int
}

func mockProviderAPI(t *testing.T, routes map[string]string) *providerAPI {
	t.Helper()
	api := &providerAPI{routes: routes, calls: map[string]int{}}
	mockPayments(t, func(r *http.Request) string {
		api.mu.Lock()
		defer api.mu.Unlock()
		key := r.Method + " " + r.URL.Path
		api.calls[key]++
		switch {
		case strings.HasSuffix(r.URL.Path, "/v1/oauth2/token"):
			return `{"access_token":"token","expires_in":3600}`
		case strings.HasSuffix(r.URL.Path, "/v1/notifications/verify-webhook-signature"):
			return `{"verification_status":"SUCCESS"}`
		}
		if body, ok := api.routes[key]; ok {
			return body
		}
		t.Errorf("unexpected provider request %s", key)
		return `{}`
	})
	return api
}

func (p *providerAPI) count(key string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls[key]
}

// stripeAPI answers the mocked Stripe backend by path.
func stripeAPI(t *testing.T, routes map[string]string) {
	t.Helper()
	testenv.StripeBackend(t, func(r *http.Request) (*http.Response, error) {
		if body, ok := routes[r.Method+" "+r.URL.Path]; ok {
			return testenv.Response(http.StatusOK, body), nil
		}
		t.Errorf("unexpected Stripe request %s %s", r.Method, r.URL)
		return testenv.Response(http.StatusNotFound, `{"error":{"message":"not found"}}`), nil
	})
}

// stripeAttempt creates a pending Stripe attempt bound to a checkout session.
func stripeAttempt(t *testing.T, schedule string) *PaymentAttempt {
	t.Helper()
	a, err := newAttempt(1, "stripe", schedule, "DF", 1050, "USD", "price_1", "cust", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.SetProviderIds("cs_"+a.Id[:8], "", ""); err != nil {
		t.Fatal(err)
	}
	return a
}

func stripeSession(a *PaymentAttempt, extra string) string {
	mode := "payment"
	if a.Schedule != "onetime" {
		mode = "subscription"
	}
	return fmt.Sprintf(`{"id":%q,"object":"checkout.session","status":"complete","payment_status":"paid","mode":%q,"amount_total":1050,"currency":"usd","metadata":{"attempt_id":%q,"product_id":"1"},"customer_details":{"email":"buyer@example.test","name":"Buyer"}%s}`, a.ProviderOrderId, mode, a.Id, extra)
}

const stripeLineItems = `{"object":"list","data":[{"id":"li_1","object":"item","price":{"id":"price_1"},"quantity":1,"amount_total":1050,"currency":"usd"}],"has_more":false}`

func stripeEventBody(id, typ, object string) string {
	return fmt.Sprintf(`{"id":%q,"object":"event","type":%q,"data":{"object":%s}}`, id, typ, object)
}

func attemptStatus(t *testing.T, id string) string {
	t.Helper()
	a, err := FindAttempt(id)
	if err != nil {
		t.Fatal(err)
	}
	return a.Status
}

func TestStripeWebhookEvents(t *testing.T) {
	setupWebhooks(t)
	one := stripeAttempt(t, "onetime")
	sub := stripeAttempt(t, "monthly")
	late := stripeAttempt(t, "yearly") // its session was never processed
	lateSub := "sub_late_" + late.Id[:6]
	subID := "sub_" + sub.Id[:6]
	stripeAPI(t, map[string]string{
		"GET /v1/checkout/sessions/" + one.ProviderOrderId:                  stripeSession(one, `,"payment_intent":"pi_one"`),
		"GET /v1/checkout/sessions/" + one.ProviderOrderId + "/line_items":  stripeLineItems,
		"GET /v1/checkout/sessions/" + sub.ProviderOrderId:                  stripeSession(sub, fmt.Sprintf(`,"subscription":%q,"invoice":"in_first"`, subID)),
		"GET /v1/checkout/sessions/" + sub.ProviderOrderId + "/line_items":  stripeLineItems,
		"GET /v1/checkout/sessions/" + late.ProviderOrderId:                 stripeSession(late, fmt.Sprintf(`,"subscription":%q,"invoice":"in_late"`, lateSub)),
		"GET /v1/checkout/sessions/" + late.ProviderOrderId + "/line_items": stripeLineItems,
		"GET /v1/invoices/in_renew":                                         fmt.Sprintf(`{"id":"in_renew","object":"invoice","paid":true,"status":"paid","subscription":%q,"amount_paid":1050,"currency":"usd","customer_email":"buyer@example.test","lines":{"object":"list","data":[{"id":"il_1","object":"line_item","price":{"id":"price_1"},"quantity":1}],"has_more":false}}`, subID),
		"GET /v1/invoices/in_unpaid":                                        fmt.Sprintf(`{"id":"in_unpaid","object":"invoice","paid":false,"status":"open","subscription":%q}`, subID),
		"GET /v1/invoices/in_late":                                          fmt.Sprintf(`{"id":"in_late","object":"invoice","paid":true,"status":"paid","subscription":%q,"amount_paid":1050,"currency":"usd","lines":{"object":"list","data":[{"id":"il_2","object":"line_item","price":{"id":"price_1"},"quantity":1}],"has_more":false}}`, lateSub),
		"GET /v1/subscriptions/" + lateSub:                                  fmt.Sprintf(`{"id":%q,"object":"subscription","metadata":{"attempt_id":%q}}`, lateSub, late.Id),
		"GET /v1/charges/ch_1":                                              `{"id":"ch_1","object":"charge","payment_intent":"pi_one"}`,
		"GET /v1/refunds":                                                   `{"object":"list","data":[{"id":"re_1","object":"refund","amount":1050,"currency":"usd","status":"succeeded"},{"id":"re_pending","object":"refund","amount":1,"currency":"usd","status":"pending"}],"has_more":false,"url":"/v1/refunds"}`,
	})
	steps := []struct {
		name, event string
		code        int
		check       func() error
	}{
		{"payment session", stripeEventBody("evt_1", "checkout.session.completed", fmt.Sprintf(`{"id":%q}`, one.ProviderOrderId)), 200, func() error {
			if attemptStatus(t, one.Id) != "completed" || testenv.Text(t, "SELECT payer_email FROM subscriptions WHERE txn_id='pi_one'") != "buyer@example.test" {
				return fmt.Errorf("one-time not fulfilled")
			}
			return nil
		}},
		{"subscription session", stripeEventBody("evt_2", "checkout.session.async_payment_succeeded", fmt.Sprintf(`{"id":%q}`, sub.ProviderOrderId)), 200, func() error {
			a, _ := FindAttempt(sub.Id)
			if a.Status != "completed" || a.ProviderSubscriptionId != subID || a.ProviderPaymentId != "in_first" {
				return fmt.Errorf("subscription not fulfilled: %+v", a)
			}
			return nil
		}},
		{"renewal invoice", stripeEventBody("evt_3", "invoice.paid", `{"id":"in_renew"}`), 200, func() error {
			if n := testenv.Scalar(t, "SELECT COUNT(*) FROM subscriptions WHERE subscr_id=?", subID); n != 2 {
				return fmt.Errorf("renewal rows=%d", n)
			}
			return nil
		}},
		{"unpaid invoice retried", stripeEventBody("evt_4", "invoice.payment_succeeded", `{"id":"in_unpaid"}`), 503, nil},
		{"invoice before session (fallback)", stripeEventBody("evt_5", "invoice.paid", `{"id":"in_late"}`), 200, func() error {
			a, _ := FindAttempt(late.Id)
			if a.Status != "completed" || a.ProviderSubscriptionId != lateSub || testenv.Scalar(t, "SELECT COUNT(*) FROM subscriptions WHERE subscr_id=?", lateSub) != 1 {
				return fmt.Errorf("fallback not fulfilled once: %+v", a)
			}
			return nil
		}},
		{"payment failed", stripeEventBody("evt_6", "invoice.payment_failed", fmt.Sprintf(`{"id":"in_x","subscription":%q}`, subID)), 200, func() error {
			if s := attemptStatus(t, sub.Id); s != "past_due" {
				return fmt.Errorf("status=%s", s)
			}
			return nil
		}},
		{"subscription deleted", stripeEventBody("evt_7", "customer.subscription.deleted", fmt.Sprintf(`{"id":%q}`, subID)), 200, func() error {
			if s := attemptStatus(t, sub.Id); s != "cancelled" {
				return fmt.Errorf("status=%s", s)
			}
			return nil
		}},
		{"charge refunded", stripeEventBody("evt_8", "charge.refunded", `{"id":"ch_1"}`), 200, func() error {
			if s := attemptStatus(t, one.Id); s != "refunded" {
				return fmt.Errorf("status=%s", s)
			}
			return nil
		}},
		{"ignored type", stripeEventBody("evt_9", "customer.created", `{"id":"cus_1"}`), 200, nil},
		{"invalid object", `{"id":"evt_10","object":"event","type":"invoice.paid","data":{"object":"nope"}}`, 403, nil}, // the SDK parses while verifying
	}
	for _, s := range steps {
		w := deliver(t, HandleWebhook, stripeSigned(s.event))
		if w.Code != s.code {
			t.Fatalf("%s: status=%d body=%s", s.name, w.Code, w.Body.String())
		}
		if s.check != nil {
			if err := s.check(); err != nil {
				t.Fatalf("%s: %v", s.name, err)
			}
		}
	}
	if subscribers(t) != 1 { // yearly late subscription remains; monthly cancelled
		t.Fatalf("subscribers=%d", subscribers(t))
	}
	if n := testenv.Scalar(t, "SELECT COUNT(*) FROM payment_events WHERE gateway='stripe' AND event_id NOT LIKE 'refund:%'"); n != 8 {
		t.Fatalf("recorded stripe events=%d", n)
	}
	testenv.Set("stripe_webhook_secret", "")
	if w := deliver(t, HandleWebhook, stripeSigned(stripeEventBody("evt_11", "customer.created", `{}`))); w.Code != 503 {
		t.Fatalf("unconfigured secret status=%d", w.Code)
	}
}

func squareBody(id, typ, object string) string {
	return fmt.Sprintf(`{"event_id":%q,"type":%q,"data":{"object":%s}}`, id, typ, object)
}

func TestSquareWebhookEvents(t *testing.T) {
	setupWebhooks(t)
	one, err := newAttempt(1, "square", "onetime", "DF", 1050, "USD", "", "cust", "")
	if err != nil {
		t.Fatal(err)
	}
	sub, err := newAttempt(1, "square", "monthly", "DF", 1050, "USD", "PLAN_1", "cust", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := sub.SetProviderIds("", "", "SQSUB_1"); err != nil {
		t.Fatal(err)
	}
	api := mockProviderAPI(t, map[string]string{
		"GET /v2/payments/PAY_1":        fmt.Sprintf(`{"payment":{"id":"PAY_1","reference_id":%q,"status":"COMPLETED","amount_money":{"amount":1050,"currency":"USD"},"buyer_email_address":"buyer@example.test"}}`, one.Id),
		"GET /v2/invoices/INV_1":        `{"invoice":{"id":"INV_1","status":"PAID","subscription_id":"SQSUB_1","order_id":"ORD_1","primary_recipient":{"email_address":"sub@example.test"}}}`,
		"GET /v2/subscriptions/SQSUB_1": `{"subscription":{"id":"SQSUB_1","plan_id":"PLAN_1"}}`,
		"GET /v2/orders/ORD_1":          `{"order":{"id":"ORD_1","state":"COMPLETED","tenders":[{"payment_id":"PAY_2"}]}}`,
		"GET /v2/payments/PAY_2":        `{"payment":{"id":"PAY_2","order_id":"ORD_1","status":"COMPLETED","amount_money":{"amount":1050,"currency":"USD"}}}`,
	})
	payment := func(status string) string {
		return fmt.Sprintf(`{"payment":{"id":"PAY_1","reference_id":%q,"status":%q}}`, one.Id, status)
	}
	steps := []struct {
		name, body string
		code       int
		wantStatus map[string]string
	}{
		{"approved payment ignored", squareBody("e1", "payment.created", payment("APPROVED")), 200, map[string]string{one.Id: "pending"}},
		{"completed payment", squareBody("e2", "payment.updated", payment("COMPLETED")), 200, map[string]string{one.Id: "completed"}},
		{"invoice paid", squareBody("e3", "invoice.payment_made", `{"invoice":{"id":"INV_1","subscription_id":"SQSUB_1"}}`), 200, map[string]string{sub.Id: "completed"}},
		{"invoice without subscription ignored", squareBody("e4", "invoice.payment_made", `{"invoice":{"id":"INV_X"}}`), 200, nil},
		{"active update ignored", squareBody("e5", "subscription.updated", `{"subscription":{"id":"SQSUB_1","status":"ACTIVE"}}`), 200, map[string]string{sub.Id: "completed"}},
		{"paused", squareBody("e6", "subscription.updated", `{"subscription":{"id":"SQSUB_1","status":"PAUSED"}}`), 200, map[string]string{sub.Id: "paused"}},
		{"deactivated", squareBody("e7", "subscription.updated", `{"subscription":{"id":"SQSUB_1","status":"DEACTIVATED"}}`), 200, map[string]string{sub.Id: "expired"}},
		{"cancel after expiry is sticky", squareBody("e8", "subscription.updated", `{"subscription":{"id":"SQSUB_1","status":"CANCELED"}}`), 200, map[string]string{sub.Id: "expired"}},
		{"pending refund ignored", squareBody("e9", "refund.created", `{"refund":{"id":"R1","payment_id":"PAY_1","status":"PENDING","amount_money":{"amount":1050,"currency":"USD"}}}`), 200, map[string]string{one.Id: "completed"}},
		{"completed refund", squareBody("e10", "refund.updated", `{"refund":{"id":"R1","payment_id":"PAY_1","status":"COMPLETED","amount_money":{"amount":1050,"currency":"USD"}}}`), 200, map[string]string{one.Id: "refunded"}},
		{"unknown payment retried", squareBody("e11", "payment.updated", `{"payment":{"id":"PAY_9","reference_id":"missing","status":"COMPLETED"}}`), 503, nil},
	}
	for _, s := range steps {
		w := deliver(t, HandleSquareWebhook, squareSigned(s.body))
		if w.Code != s.code {
			t.Fatalf("%s: status=%d", s.name, w.Code)
		}
		for id, want := range s.wantStatus {
			if got := attemptStatus(t, id); got != want {
				t.Fatalf("%s: status=%s want %s", s.name, got, want)
			}
		}
	}
	if api.count("GET /v2/payments/PAY_1") != 1 || api.count("GET /v2/payments/PAY_2") != 1 {
		t.Fatalf("provider calls: %v", api.calls)
	}
	if got := testenv.Text(t, "SELECT payer_email FROM subscriptions WHERE txn_id='PAY_2'"); got != "sub@example.test" {
		t.Fatalf("invoice email=%q", got)
	}
}

func paypalBody(id, typ, resource string) string {
	return fmt.Sprintf(`{"id":%q,"event_type":%q,"resource":%s}`, id, typ, resource)
}

func paypalOrder(a *PaymentAttempt, captureID string) string {
	return fmt.Sprintf(`{"id":%q,"status":"COMPLETED","purchase_units":[{"reference_id":%q,"items":[{"sku":"1","quantity":"1"}],"amount":{"currency_code":"USD","value":"10.50"},"payments":{"captures":[{"id":%q,"status":"COMPLETED","final_capture":true,"amount":{"currency_code":"USD","value":"10.50"}}]}}],"payer":{"email_address":"buyer@example.test","name":{"given_name":"Buyer"}}}`, a.ProviderOrderId, a.Id, captureID)
}

func TestPaypalWebhookEvents(t *testing.T) {
	setupWebhooks(t)
	capture, err := newAttempt(1, "paypal", "onetime", "DF", 1050, "USD", "", "cust", "")
	if err != nil {
		t.Fatal(err)
	}
	order, err := newAttempt(1, "paypal", "onetime", "DF", 1050, "USD", "", "cust", "")
	if err != nil {
		t.Fatal(err)
	}
	sub, err := newAttempt(1, "paypal", "monthly", "DF", 1050, "USD", "P-PLAN", "cust", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range []struct {
		a              *PaymentAttempt
		order, subscID string
	}{{capture, "ORDER_C", ""}, {order, "ORDER_O", ""}, {sub, "", "I-SUB"}} {
		if err := x.a.SetProviderIds(x.order, "", x.subscID); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC().Add(time.Minute).Format(time.RFC3339)
	transactions := fmt.Sprintf(`{"transactions":[{"id":"TXN_1","status":"COMPLETED","time":%q,"amount_with_breakdown":{"gross_amount":{"currency_code":"USD","value":"10.50"}}},{"id":"TXN_2","status":"COMPLETED","time":%q,"amount_with_breakdown":{"gross_amount":{"currency_code":"USD","value":"10.50"}}}]}`, now, time.Now().UTC().Add(2*time.Minute).Format(time.RFC3339))
	api := mockProviderAPI(t, map[string]string{
		"GET /v2/checkout/orders/ORDER_C":                  paypalOrder(capture, "CAP_C"),
		"GET /v2/checkout/orders/ORDER_O":                  paypalOrder(order, "CAP_O"),
		"GET /v1/billing/subscriptions/I-SUB":              fmt.Sprintf(`{"id":"I-SUB","plan_id":"P-PLAN","custom_id":%q,"status":"ACTIVE","subscriber":{"email_address":"sub@example.test"}}`, sub.Id),
		"GET /v1/billing/subscriptions/I-SUB/transactions": transactions,
	})
	steps := []struct {
		name, body string
		code       int
		want       map[string]string
	}{
		{"capture completed", paypalBody("WH-1", "PAYMENT.CAPTURE.COMPLETED", `{"id":"CAP_C","supplementary_data":{"related_ids":{"order_id":"ORDER_C"}}}`), 200, map[string]string{capture.Id: "completed"}},
		{"order completed", paypalBody("WH-2", "CHECKOUT.ORDER.COMPLETED", `{"id":"ORDER_O"}`), 200, map[string]string{order.Id: "completed"}},
		{"subscription activated", paypalBody("WH-3", "BILLING.SUBSCRIPTION.ACTIVATED", `{"id":"I-SUB"}`), 200, map[string]string{sub.Id: "completed"}},
		{"renewal sale", paypalBody("WH-4", "PAYMENT.SALE.COMPLETED", `{"id":"TXN_2","billing_agreement_id":"I-SUB"}`), 200, nil},
		{"refund by capture id", paypalBody("WH-5", "PAYMENT.CAPTURE.REFUNDED", `{"id":"REF_1","status":"COMPLETED","amount":{"value":"10.50","currency_code":"USD"},"supplementary_data":{"related_ids":{"capture_id":"CAP_C"}}}`), 200, map[string]string{capture.Id: "refunded"}},
		{"refund by up link", paypalBody("WH-6", "PAYMENT.CAPTURE.REFUNDED", `{"id":"REF_2","status":"COMPLETED","amount":{"value":"5.00","currency_code":"USD"},"links":[{"rel":"self","href":"https://api.paypal.test/v2/payments/refunds/REF_2"},{"rel":"up","href":"https://api.paypal.test/v2/payments/captures/CAP_O"}]}`), 200, map[string]string{order.Id: "completed"}},
		{"legacy sale refund", paypalBody("WH-7", "PAYMENT.SALE.REFUNDED", `{"id":"REF_3","state":"completed","amount":{"total":"10.50","currency":"USD"},"links":[{"rel":"up","href":"https://api.paypal.test/v1/payments/sale/TXN_2"}]}`), 200, nil},
		{"pending refund retried", paypalBody("WH-8", "PAYMENT.CAPTURE.REFUNDED", `{"id":"REF_4","status":"PENDING","amount":{"value":"1.00","currency_code":"USD"},"supplementary_data":{"related_ids":{"capture_id":"CAP_O"}}}`), 503, nil},
		{"suspended", paypalBody("WH-9", "BILLING.SUBSCRIPTION.SUSPENDED", `{"id":"I-SUB"}`), 200, map[string]string{sub.Id: "suspended"}},
		{"cancelled", paypalBody("WH-10", "BILLING.SUBSCRIPTION.CANCELLED", `{"id":"I-SUB"}`), 200, map[string]string{sub.Id: "cancelled"}},
		{"expired after cancel is sticky", paypalBody("WH-11", "BILLING.SUBSCRIPTION.EXPIRED", `{"id":"I-SUB"}`), 200, map[string]string{sub.Id: "cancelled"}},
	}
	for _, s := range steps {
		w := deliver(t, HandlePaypalWebhook, paypalEvent(s.body))
		if w.Code != s.code {
			t.Fatalf("%s: status=%d", s.name, w.Code)
		}
		for id, want := range s.want {
			if got := attemptStatus(t, id); got != want {
				t.Fatalf("%s: status=%s want %s", s.name, got, want)
			}
		}
	}
	if n := testenv.Scalar(t, "SELECT refunded FROM payment_transactions WHERE transaction_id='CAP_O'"); n != 500 {
		t.Fatalf("up-link refund=%d", n)
	}
	if got := testenv.Text(t, "SELECT payment_status FROM subscriptions WHERE txn_id='TXN_2'"); got != "CANCELLED" {
		t.Fatalf("renewal row status=%s", got) // refunded, then cancellation updates every row of the subscription
	}
	if api.count("POST /v1/notifications/verify-webhook-signature") != len(steps) {
		t.Fatalf("signature verified %d times", api.count("POST /v1/notifications/verify-webhook-signature"))
	}
	testenv.Set("paypal_webhook_id", "")
	if w := deliver(t, HandlePaypalWebhook, paypalEvent(paypalBody("WH-12", "X", `{}`))); w.Code != 503 {
		t.Fatalf("unconfigured webhook id status=%d", w.Code)
	}
}

func razorpayBody(event, payload string) string {
	return fmt.Sprintf(`{"event":%q,"payload":%s}`, event, payload)
}

func TestRazorpayWebhookEvents(t *testing.T) {
	setupWebhooks(t)
	sub, err := newAttempt(1, "razorpay", "monthly", "DF", 1050, "USD", "plan_1", "cust", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := sub.SetProviderIds("", "", "sub_R"); err != nil {
		t.Fatal(err)
	}
	mockProviderAPI(t, map[string]string{
		"GET /v1/subscriptions/sub_R": fmt.Sprintf(`{"id":"sub_R","plan_id":"plan_1","status":"active","notes":{"attempt_id":%q}}`, sub.Id),
		"GET /v1/payments/pay_R":      `{"id":"pay_R","order_id":"order_R","invoice_id":"inv_R","status":"captured","captured":true,"amount":1050,"currency":"USD","email":"buyer@example.test"}`,
		"GET /v1/invoices/inv_R":      `{"id":"inv_R","subscription_id":"sub_R","payment_id":"pay_R","order_id":"order_R","status":"paid"}`,
	})
	status := func(event, status string) string {
		return razorpayBody(event, fmt.Sprintf(`{"subscription":{"entity":{"id":"sub_R","status":%q}}}`, status))
	}
	steps := []struct {
		name, id, body string
		code           int
		want           string
	}{
		{"charged", "ev1", razorpayBody("subscription.charged", `{"subscription":{"entity":{"id":"sub_R"}},"payment":{"entity":{"id":"pay_R"}}}`), 200, "completed"},
		{"status mismatch retried", "ev2", status("subscription.halted", "active"), 503, "completed"},
		{"paused", "ev3", status("subscription.paused", "paused"), 200, "paused"},
		{"pending", "ev4", status("subscription.pending", "pending"), 200, "pending"},
		{"halted", "ev5", status("subscription.halted", "halted"), 200, "halted"},
		{"completed means expired", "ev6", status("subscription.completed", "completed"), 200, "expired"},
		{"cancel after expiry is sticky", "ev7", status("subscription.cancelled", "cancelled"), 200, "expired"},
		{"unprocessed refund retried", "ev8", razorpayBody("refund.processed", `{"refund":{"entity":{"id":"rfnd_1","payment_id":"pay_R","amount":1050,"currency":"USD","status":"pending"}}}`), 503, "expired"},
		{"refund", "ev9", razorpayBody("refund.processed", `{"refund":{"entity":{"id":"rfnd_1","payment_id":"pay_R","amount":1050,"currency":"USD","status":"processed"}}}`), 200, "refunded"},
		{"missing event id", "", razorpayBody("payment.captured", `{}`), 503, "refunded"},
		{"ignored event", "ev10", razorpayBody("payment.authorized", `{}`), 200, "refunded"},
	}
	for _, s := range steps {
		w := deliver(t, HandleRazorpayWebhook, signedRazorpayWebhook(t, s.body, s.id))
		if w.Code != s.code {
			t.Fatalf("%s: status=%d", s.name, w.Code)
		}
		if got := attemptStatus(t, sub.Id); got != s.want {
			t.Fatalf("%s: status=%s want %s", s.name, got, s.want)
		}
	}
	testenv.Set("razorpay_webhook_secret", "")
	if w := deliver(t, HandleRazorpayWebhook, signedRazorpayWebhook(t, razorpayBody("x", `{}`), "ev11")); w.Code != 403 {
		t.Fatalf("unconfigured secret status=%d", w.Code)
	}
}

// signedFor returns a validly signed delivery of body to each gateway.
func signedFor(t *testing.T, gateway, body, eventID string) (webhookHandler, *http.Request) {
	switch gateway {
	case "stripe":
		return HandleWebhook, stripeSigned(body)
	case "square":
		return HandleSquareWebhook, squareSigned(body)
	case "paypal":
		return HandlePaypalWebhook, paypalEvent(body)
	default:
		return HandleRazorpayWebhook, signedRazorpayWebhook(t, body, eventID)
	}
}

func TestWebhookCommonBehaviour(t *testing.T) {
	setupWebhooks(t)
	mockProviderAPI(t, nil)
	ignored := map[string]string{
		"stripe":   stripeEventBody("dup_1", "customer.created", `{"id":"cus_1"}`),
		"square":   squareBody("dup_1", "customer.created", `{}`),
		"paypal":   paypalBody("dup_1", "CUSTOMER.CREATED", `{"id":"x"}`),
		"razorpay": razorpayBody("payment.authorized", `{}`),
	}
	for _, gateway := range gateways {
		t.Run(gateway, func(t *testing.T) {
			// Signed but not JSON.
			h, r := signedFor(t, gateway, "not json", "bad_json")
			w := deliver(t, h, r)
			if gateway == "paypal" || gateway == "stripe" {
				// These SDKs parse the event while verifying it, so it is refused as unsigned.
				if w.Code != http.StatusForbidden {
					t.Fatalf("invalid JSON status=%d", w.Code)
				}
			} else if w.Code != http.StatusBadRequest {
				t.Fatalf("invalid JSON status=%d", w.Code)
			}
			// A repeated event ID is processed once.
			for i := 0; i < 2; i++ {
				h, r := signedFor(t, gateway, ignored[gateway], "dup_1")
				if w := deliver(t, h, r); w.Code != http.StatusOK {
					t.Fatalf("delivery %d status=%d", i, w.Code)
				}
			}
			if n := testenv.Scalar(t, "SELECT COUNT(*) FROM payment_events WHERE gateway=? AND event_id='dup_1'", gateway); n != 1 {
				t.Fatalf("events recorded=%d", n)
			}
			// Only POST is accepted.
			w = deliver(t, h, httptest.NewRequest(http.MethodGet, "/webhook", nil))
			if w.Code != http.StatusMethodNotAllowed {
				t.Fatalf("GET status=%d", w.Code)
			}
			// Bodies over 2 MB are refused before any verification.
			h, r = signedFor(t, gateway, `{"pad":"`+strings.Repeat("x", 2<<20)+`"}`, "big")
			if w := deliver(t, h, r); w.Code != http.StatusBadRequest {
				t.Fatalf("oversized status=%d", w.Code)
			}
		})
	}
}
