package subscriptions

import (
	"encoding/json"
	"fmt"
	"github.com/abishekmuthian/open-payment-host/src/internal/testenv"
	"github.com/abishekmuthian/open-payment-host/src/lib/auth"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stripe/stripe-go/v72"
)

func TestRazorpayCapturedPaymentMustBelongToSubscription(t *testing.T) {
	setupPayments(t)
	a, _ := fixtureAttempt(t, "razorpay", "monthly")
	invoiceSub := a.ProviderSubscriptionId
	captured := true
	mockPayments(t, func(r *http.Request) string {
		switch {
		case strings.Contains(r.URL.Path, "/subscriptions/"):
			return fmt.Sprintf(`{"id":%q,"plan_id":"","status":"active","notes":{"attempt_id":%q}}`, a.ProviderSubscriptionId, a.Id)
		case strings.Contains(r.URL.Path, "/payments/"):
			return fmt.Sprintf(`{"id":"payment","order_id":"order","invoice_id":"invoice","status":"captured","captured":%t,"amount":1050,"currency":"USD"}`, captured)
		case strings.Contains(r.URL.Path, "/invoices/"):
			return fmt.Sprintf(`{"id":"invoice","subscription_id":%q,"payment_id":"payment","order_id":"order","status":"paid"}`, invoiceSub)
		}
		t.Fatalf("unexpected request %s", r.URL)
		return ""
	})
	if _, err := fetchRazorpaySubscriptionFacts(a.ProviderSubscriptionId, "payment"); err != nil {
		t.Fatal(err)
	}
	invoiceSub = "another_subscription"
	if _, err := fetchRazorpaySubscriptionFacts(a.ProviderSubscriptionId, "payment"); err == nil {
		t.Fatal("unrelated captured payment accepted")
	}
	invoiceSub = a.ProviderSubscriptionId
	captured = false
	if _, err := fetchRazorpaySubscriptionFacts(a.ProviderSubscriptionId, "payment"); err == nil {
		t.Fatal("uncaptured payment accepted")
	}
}

func TestSquareActiveSubscriptionRequiresPaidInvoice(t *testing.T) {
	setupPayments(t)
	a, _ := fixtureAttempt(t, "square", "yearly")
	status := "UNPAID"
	amount := int64(1050)
	mockPayments(t, func(r *http.Request) string {
		switch {
		case strings.Contains(r.URL.Path, "/invoices/"):
			return fmt.Sprintf(`{"invoice":{"id":"invoice","status":%q,"subscription_id":%q,"order_id":"order"}}`, status, a.ProviderSubscriptionId)
		case strings.Contains(r.URL.Path, "/subscriptions/"):
			return fmt.Sprintf(`{"subscription":{"id":%q,"plan_id":"","status":"ACTIVE"}}`, a.ProviderSubscriptionId)
		case strings.Contains(r.URL.Path, "/orders/"):
			return `{"order":{"id":"order","state":"COMPLETED","tenders":[{"payment_id":"payment"}]}}`
		case strings.Contains(r.URL.Path, "/payments/"):
			return fmt.Sprintf(`{"payment":{"id":"payment","order_id":"order","status":"COMPLETED","amount_money":{"amount":%d,"currency":"USD"}}}`, amount)
		}
		t.Fatalf("unexpected request %s", r.URL)
		return ""
	})
	if _, err := fetchSquareInvoiceFacts(a, "invoice"); err == nil {
		t.Fatal("unpaid invoice accepted")
	}
	status = "PAID"
	f, err := fetchSquareInvoiceFacts(a, "invoice")
	if err != nil {
		t.Fatal(err)
	}
	if err := validatePayment(a, f); err != nil {
		t.Fatal(err)
	}
	amount = 1
	f, err = fetchSquareInvoiceFacts(a, "invoice")
	if err != nil {
		t.Fatal(err)
	}
	if err := validatePayment(a, f); err == nil {
		t.Fatal("underpayment accepted")
	}
}

func TestPayPalSubscriptionRejectsRecentUnrelatedAndOldTransactions(t *testing.T) {
	setupPayments(t)
	a, _ := fixtureAttempt(t, "paypal", "monthly")
	custom := a.Id
	paidAt := time.Now().UTC()
	hasPayment := true
	mockPayments(t, func(r *http.Request) string {
		if strings.Contains(r.URL.Path, "oauth2") {
			return `{"access_token":"test"}`
		}
		if strings.HasSuffix(r.URL.Path, "/transactions") {
			if !hasPayment {
				return `{"transactions":[]}`
			}
			return fmt.Sprintf(`{"transactions":[{"id":"payment","status":"COMPLETED","time":%q,"amount_with_breakdown":{"gross_amount":{"value":"10.50","currency_code":"USD"}}}]}`, paidAt.Format(time.RFC3339))
		}
		return fmt.Sprintf(`{"id":%q,"plan_id":"","custom_id":%q,"status":"ACTIVE"}`, a.ProviderSubscriptionId, custom)
	})
	if _, err := fetchPaypalSubscriptionFacts(a); err != nil {
		t.Fatal(err)
	}
	custom = "another_attempt"
	if _, err := fetchPaypalSubscriptionFacts(a); err == nil {
		t.Fatal("unrelated subscription accepted")
	}
	custom = a.Id
	paidAt = a.CreatedAt.Add(-time.Hour)
	if _, err := fetchPaypalSubscriptionFacts(a); err == nil {
		t.Fatal("old transaction accepted")
	}
	hasPayment = false
	if _, err := fetchPaypalSubscriptionFacts(a); err == nil {
		t.Fatal("ACTIVE without payment accepted")
	}
}

func TestStripeSessionRejectsCheapPriceAndWrongMetadata(t *testing.T) {
	setupPayments(t)
	a, _ := fixtureAttempt(t, "stripe", "onetime")
	a.PriceId = "expensive"
	price := "expensive"
	product := "1"
	old := stripe.GetBackend(stripe.APIBackend)
	defer stripe.SetBackend(stripe.APIBackend, old)
	client := &http.Client{Transport: paymentRoundTripper(func(r *http.Request) (*http.Response, error) {
		var body string
		if strings.HasSuffix(r.URL.Path, "/line_items") {
			body = fmt.Sprintf(`{"object":"list","data":[{"id":"line","price":{"id":%q},"quantity":1,"amount_total":1050,"currency":"usd"}],"has_more":false}`, price)
		} else {
			body = fmt.Sprintf(`{"id":%q,"status":"complete","payment_status":"paid","mode":"payment","amount_total":1050,"currency":"usd","payment_intent":"payment","metadata":{"attempt_id":%q,"product_id":%q}}`, a.ProviderOrderId, a.Id, product)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	stripe.SetBackend(stripe.APIBackend, stripe.GetBackendWithConfig(stripe.APIBackend, &stripe.BackendConfig{HTTPClient: client}))
	if _, err := fetchStripeSessionFacts(a); err != nil {
		t.Fatal(err)
	}
	price = "cheap"
	if _, err := fetchStripeSessionFacts(a); err == nil {
		t.Fatal("cheap price accepted")
	}
	price = "expensive"
	product = "2"
	if _, err := fetchStripeSessionFacts(a); err == nil {
		t.Fatal("wrong product accepted")
	}
}

func TestOutboxLeasePreventsConcurrentDelivery(t *testing.T) {
	setupPayments(t)
	a, f := fixtureAttempt(t, "paypal", "onetime")
	if _, err := verifyAndFulfill(a, f); err != nil {
		t.Fatal(err)
	}
	testenv.Exec(t, fmt.Sprintf("UPDATE payment_outbox SET lease_until=%d", time.Now().Add(time.Minute).Unix()))
	if err := deliverPaymentEffects(); err != nil {
		t.Fatal(err)
	}
	if n := testenv.Scalar(t, "SELECT COUNT(*) FROM payment_outbox WHERE webhook_done=1"); n != 0 {
		t.Fatal("processed another worker's lease")
	}
	testenv.Exec(t, "UPDATE payment_outbox SET lease_until=0")
	if err := deliverPaymentEffects(); err != nil {
		t.Fatal(err)
	}
	if n := testenv.Scalar(t, "SELECT COUNT(*) FROM payment_outbox WHERE webhook_done=1 AND mailing_done=1"); n != 1 {
		t.Fatal("released work not delivered")
	}
}

func TestSquareAnnualPlanValidation(t *testing.T) {
	setupPayments(t)
	cadence := "ANNUAL"
	mockPayments(t, func(r *http.Request) string {
		body, _ := json.Marshal(map[string]interface{}{"object": map[string]interface{}{"id": "plan", "subscription_plan_data": map[string]interface{}{"phases": []interface{}{map[string]interface{}{"cadence": cadence, "recurring_price_money": map[string]interface{}{"amount": 1050, "currency": "USD"}}}}}})
		return string(body)
	})
	if err := validateSquarePlan("plan", "yearly", 1050, "USD"); err != nil {
		t.Fatal(err)
	}
	cadence = "MONTHLY"
	if err := validateSquarePlan("plan", "yearly", 1050, "USD"); err == nil {
		t.Fatal("monthly plan accepted for yearly product")
	}
}

func TestSquarePreboundPaymentStillActivatesAndExpires(t *testing.T) {
	setupPayments(t)
	for _, expired := range []bool{false, true} {
		a, err := newAttempt(1, "square", "onetime", "DF", 1050, "USD", "", "", "")
		if err != nil {
			t.Fatal(err)
		}
		paymentID := "payment_" + a.Id
		if err := a.SetProviderIds("", paymentID, ""); err != nil {
			t.Fatal(err)
		}
		if expired {
			a.ExpiresAt = time.Now().Add(-time.Hour)
		}
		_, err = verifyAndFulfill(a, ProviderFacts{Gateway: "square", PaymentId: paymentID, Amount: 1050, Currency: "USD", Paid: true})
		if expired && err == nil {
			t.Fatal("expired prebound payment accepted")
		}
		if !expired && err != nil {
			t.Fatal(err)
		}
	}
	if n := testenv.Scalar(t, "SELECT COUNT(*) FROM payment_outbox"); n != 1 {
		t.Fatalf("activation events=%d", n)
	}
}

func authenticatedPaymentPOST(t *testing.T, path string, values url.Values) *http.Request {
	t.Helper()
	oldH, oldS := auth.HMACKey, auth.SecretKey
	auth.HMACKey = []byte("0123456789abcdef0123456789abcdef")
	auth.SecretKey = []byte("abcdef0123456789abcdef0123456789")
	t.Cleanup(func() { auth.HMACKey = oldH; auth.SecretKey = oldS })
	w := httptest.NewRecorder()
	token, err := auth.AuthenticityToken(w, httptest.NewRequest("GET", "/", nil))
	if err != nil {
		t.Fatal(err)
	}
	values.Set("authenticity_token", token)
	r := httptest.NewRequest("POST", path, strings.NewReader(values.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range w.Result().Cookies() {
		r.AddCookie(c)
	}
	return r
}

func TestSquareCheckoutIgnoresPostedAmountAndCurrency(t *testing.T) {
	setupPayments(t)
	testenv.Exec(t, `UPDATE products SET square_price='{"DF":{"amount":1050,"currency":"USD"}}' WHERE id=1`)
	config.Configuration(0)["square_domain"] = "https://square.test/v2"
	subscriptionRoutes()
	calls := 0
	mockPayments(t, func(r *http.Request) string {
		calls++
		var payload struct {
			Reference string `json:"reference_id"`
			Money     struct {
				Amount   int64  `json:"amount"`
				Currency string `json:"currency"`
			} `json:"amount_money"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.Money.Amount != 1050 || payload.Money.Currency != "USD" {
			t.Fatalf("browser controlled charge: %+v", payload)
		}
		return fmt.Sprintf(`{"payment":{"id":"payment","status":"COMPLETED","reference_id":%q,"amount_money":{"amount":1050,"currency":"USD"}}}`, payload.Reference)
	})
	r := authenticatedPaymentPOST(t, "/subscriptions/square", url.Values{"productId": {"1"}, "amount": {"1"}, "currency": {"JPY"}, "paymentToken": {"token"}})
	w := httptest.NewRecorder()
	if err := HandleSquare(w, r); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || w.Code != 302 {
		t.Fatalf("calls=%d status=%d", calls, w.Code)
	}
	if n := testenv.Scalar(t, "SELECT COUNT(*) FROM payment_outbox"); n != 1 {
		t.Fatal("missing activation")
	}
}

func TestCancellationRejectsMissingCrossAndReusedTokensAllGateways(t *testing.T) {
	setupPayments(t)
	old := stripe.GetBackend(stripe.APIBackend)
	defer stripe.SetBackend(stripe.APIBackend, old)
	calls := 0
	transport := paymentRoundTripper(func(r *http.Request) (*http.Response, error) {
		calls++
		id := strings.TrimSuffix(r.URL.Path, "/cancel")
		id = id[strings.LastIndex(id, "/")+1:]
		body := fmt.Sprintf(`{"id":%q,"status":"active"}`, id)
		if strings.Contains(r.URL.Path, "oauth2") {
			calls--
			body = `{"access_token":"test"}`
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	client := &http.Client{Transport: transport}
	oldHTTP := paymentHTTPClient
	paymentHTTPClient = client
	t.Cleanup(func() { paymentHTTPClient = oldHTTP })
	stripe.SetBackend(stripe.APIBackend, stripe.GetBackendWithConfig(stripe.APIBackend, &stripe.BackendConfig{HTTPClient: client}))
	for _, gateway := range []string{"stripe", "square", "paypal", "razorpay"} {
		a, _ := fixtureAttempt(t, gateway, "monthly")
		other, _ := fixtureAttempt(t, gateway, "monthly")
		token, _ := openCapability(a.CancellationTokenCiphertext)
		cross, _ := openCapability(other.CancellationTokenCiphertext)
		before := calls
		for _, bad := range []string{"", cross, "wrong"} {
			if err := CancelAttempt(a, bad); err == nil {
				t.Fatalf("%s accepted bad token", gateway)
			}
		}
		if calls != before {
			t.Fatal("unauthorized provider call")
		}
		if err := CancelAttempt(a, token); err != nil {
			t.Fatal(err)
		}
		if err := CancelAttempt(a, token); err == nil {
			t.Fatal("reused token accepted")
		}
		if calls != before+1 {
			t.Fatalf("%s provider calls=%d", gateway, calls-before)
		}
	}
}

func TestInvalidWebhookSignaturesNeverRecordEvents(t *testing.T) {
	setupPayments(t)
	cfg := config.Configuration(0)
	cfg["stripe_webhook_secret"] = "secret"
	cfg["razorpay_webhook_secret"] = "secret"
	cfg["paypal_webhook_id"] = "webhook"
	cfg["paypal_client_id"] = "client"
	cfg["paypal_client_secret"] = "secret"
	mockPayments(t, func(r *http.Request) string {
		if strings.Contains(r.URL.Path, "oauth2") {
			return `{"access_token":"test","expires_in":3600}`
		}
		return `{"verification_status":"FAILURE"}`
	})
	for _, handler := range []func(http.ResponseWriter, *http.Request) error{HandleWebhook, HandleSquareWebhook, HandlePaypalWebhook, HandleRazorpayWebhook} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/webhook", strings.NewReader(`{"id":"event","event_id":"event"}`))
		if err := handler(w, r); err != nil {
			t.Fatal(err)
		}
		if w.Code != 403 {
			t.Fatalf("signature failure status=%d", w.Code)
		}
	}
	if n := testenv.Scalar(t, "SELECT COUNT(*) FROM payment_events"); n != 0 {
		t.Fatal("unauthenticated event recorded")
	}
}

func TestLegacyCancellationStatusIsIdempotentAndCannotFulfill(t *testing.T) {
	setupPayments(t)
	testenv.Exec(t, `INSERT INTO subscriptions(pg,subscr_id,item_number,payment_status,user_id,payer_email) VALUES('paypal','legacy',1,'ACTIVE','external','reader@example.test'); UPDATE products SET total_subscribers=1 WHERE id=1`)
	if err := applySubscriptionStatus("paypal", "legacy", "cancelled"); err != nil {
		t.Fatal(err)
	}
	a, err := FindAttemptByProviderSubscription("paypal", "legacy")
	if err != nil {
		t.Fatal(err)
	}
	if a.CancelledAt.IsZero() || a.CancelledAt.Location() != time.UTC {
		t.Fatalf("legacy cancellation time=%v", a.CancelledAt)
	}
	time.Sleep(2 * time.Millisecond)
	if err := applySubscriptionStatus("paypal", "legacy", "cancelled"); err != nil {
		t.Fatal(err)
	}
	duplicate, err := FindAttempt(a.Id)
	if err != nil {
		t.Fatal(err)
	}
	if !duplicate.CancelledAt.Equal(a.CancelledAt) {
		t.Fatalf("duplicate changed legacy cancellation time: %v != %v", duplicate.CancelledAt, a.CancelledAt)
	}
	if n := testenv.Scalar(t, "SELECT total_subscribers FROM products WHERE id=1"); n != 0 {
		t.Fatal("legacy counter wrong")
	}
	if n := testenv.Scalar(t, "SELECT COUNT(*) FROM payment_outbox"); n != 1 {
		t.Fatal("legacy webhook not deduplicated")
	}
	if _, err := verifyAndFulfill(a, ProviderFacts{Gateway: "paypal", SubscriptionId: "legacy", PaymentId: "legacy_payment", Amount: 1, Currency: "USD", Paid: true}); err == nil {
		t.Fatal("legacy record authorized fulfillment")
	}
}

func TestStripeCheckoutDerivesPriceAndKeepsCountryTaxWithFallback(t *testing.T) {
	setupPayments(t)
	testenv.Exec(t, `UPDATE products SET stripe_price='{"DF":"configured_price"}' WHERE id=1`)
	cfg := config.Configuration(0)
	cfg["subscription_client_country"] = "IN"
	cfg["stripe_tax_rate_IN"] = "tax_IN"
	subscriptionRoutes()
	old := stripe.GetBackend(stripe.APIBackend)
	defer stripe.SetBackend(stripe.APIBackend, old)
	var attemptID string
	client := &http.Client{Transport: paymentRoundTripper(func(r *http.Request) (*http.Response, error) {
		body := ""
		switch r.URL.Path {
		case "/v1/prices/configured_price":
			body = `{"id":"configured_price","active":true,"type":"one_time","unit_amount":1000,"currency":"usd"}`
		case "/v1/tax_rates/tax_IN":
			body = `{"id":"tax_IN","active":true,"inclusive":false,"percentage":18}`
		case "/v1/checkout/sessions":
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if r.Form.Get("line_items[0][price]") != "configured_price" || r.Form.Get("line_items[0][tax_rates][0]") != "tax_IN" {
				t.Fatalf("wrong configured checkout: %v", r.Form)
			}
			attemptID = r.Form.Get("metadata[attempt_id]")
			body = `{"id":"session","url":"https://checkout.stripe.test/pay"}`
		default:
			t.Fatalf("unexpected provider request %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	stripe.SetBackend(stripe.APIBackend, stripe.GetBackendWithConfig(stripe.APIBackend, &stripe.BackendConfig{HTTPClient: client}))
	r := authenticatedPaymentPOST(t, "/subscriptions/create-checkout-session", url.Values{"productId": {"1"}, "priceId": {"cheap_price"}})
	w := httptest.NewRecorder()
	if err := HandleCreateCheckoutSession(w, r); err != nil {
		t.Fatal(err)
	}
	a, err := FindAttempt(attemptID)
	if err != nil {
		t.Fatal(err)
	}
	if a.Amount != 1180 || a.PriceId != "configured_price" || a.Country != "DF" {
		t.Fatalf("incorrect frozen amount/price/country: %d %s %s", a.Amount, a.PriceId, a.Country)
	}
}
