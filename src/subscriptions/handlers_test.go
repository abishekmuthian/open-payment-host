package subscriptions

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/abishekmuthian/open-payment-host/src/internal/testenv"
)

// subscriptionRoutes registers this package's routes on the shared test mux,
// mapped as in src/app/routes.go, so mux.Params can resolve them.
func subscriptionRoutes() {
	for _, r := range []struct {
		method, pattern string
		h               func(http.ResponseWriter, *http.Request) error
	}{
		{"POST", "/subscriptions/create-checkout-session", HandleCreateCheckoutSession},
		{"GET", "/subscriptions/create-checkout-session", HandleCreateCheckoutSession},
		{"GET", "/subscriptions/billing", HandleBillingShow},
		{"POST", "/subscriptions/billing", HandleBilling},
		{"GET", "/subscriptions/square", HandleSquareShow},
		{"POST", "/subscriptions/square", HandleSquare},
		{"GET", "/subscriptions/paypal", HandlePaypalShow},
		{"POST", "/subscriptions/paypal/orders", HandlePaypalCreateOrder},
		{"POST", "/subscriptions/paypal/orders/{id:[a-zA-Z0-9]+}/capture", HandlePaypalCaptureOrder},
		{"POST", "/subscriptions/paypal/subscriptions", HandlePaypalCreateSubscription},
		{"GET", "/subscriptions/razorpay", HandleRazorpayShow},
		{"POST", "/subscriptions/subscribe", HandleCreateSubscription},
		{"GET", "/subscriptions/success", HandlePaymentSuccess},
		{"GET", "/subscriptions/payment-status", HandlePaymentStatus},
		{"GET", "/subscriptions/stripe-success", HandleStripeSuccess},
		{"POST", "/subscriptions/cancellation-link", HandleCancellationLink},
		{"GET", "/subscriptions/failure", HandlePaymentFailure},
	} {
		testenv.Route(r.method, r.pattern, r.h)
	}
}

// setupHandlers prepares payments plus auth, templates and routes.
func setupHandlers(t *testing.T) {
	t.Helper()
	setupWebhooks(t)
	testenv.Auth(t)
	testenv.Templates(t)
	subscriptionRoutes()
}

func browserCookie(a *PaymentAttempt) *http.Cookie {
	return &http.Cookie{Name: AttemptCookie, Value: a.Id + "." + a.browserToken}
}

func get(t *testing.T, path string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, path, nil)
	for _, c := range cookies {
		r.AddCookie(c)
	}
	return testenv.Serve(t, r)
}

func post(t *testing.T, path string, form url.Values, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	r := testenv.AuthedPOST(t, path, form, 0)
	for _, c := range cookies {
		r.AddCookie(c)
	}
	return testenv.Serve(t, r)
}

func cookieNamed(w *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range w.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func TestHandlePaymentStatus(t *testing.T) {
	setupHandlers(t)
	a, f := fixtureAttempt(t, "razorpay", "monthly")
	if w := get(t, "/subscriptions/payment-status?attempt_id=missing"); w.Code != http.StatusNotFound {
		t.Fatalf("unknown attempt status=%d", w.Code)
	}
	if w := get(t, "/subscriptions/payment-status?attempt_id="+a.Id); w.Code != http.StatusUnauthorized {
		t.Fatalf("no browser binding status=%d", w.Code)
	}
	other, _ := fixtureAttempt(t, "razorpay", "monthly")
	if w := get(t, "/subscriptions/payment-status?attempt_id="+a.Id, browserCookie(other)); w.Code != http.StatusUnauthorized {
		t.Fatalf("another browser's cookie status=%d", w.Code)
	}
	w := get(t, "/subscriptions/payment-status?attempt_id="+a.Id, browserCookie(a))
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || w.Code != 200 || body["status"] != "pending" || body["success_url"] != "" {
		t.Fatalf("pending: %d %v %v", w.Code, body, err)
	}
	if w.Header().Get("Cache-Control") != "no-store" || cookieNamed(w, CompletionCookie) != nil {
		t.Fatal("pending response headers or cookie")
	}
	if _, err := verifyAndFulfill(a, f); err != nil {
		t.Fatal(err)
	}
	w = get(t, "/subscriptions/payment-status?attempt_id="+a.Id, browserCookie(a))
	body = nil
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["status"] != "completed" || body["success_url"] != "/subscriptions/success?attempt_id="+a.Id {
		t.Fatalf("completed: %v", body)
	}
	completion := cookieNamed(w, CompletionCookie)
	if completion == nil || !completion.HttpOnly || completion.Path != "/subscriptions" {
		t.Fatalf("completion cookie %+v", completion)
	}
	// The completion cookie unlocks the clean success URL, which returns the
	// buyer to the validated merchant redirect with frozen parameters.
	success := get(t, body["success_url"], completion)
	want := "https://client.test/return?custom_id=customer%261&subscription_id=" + a.ProviderSubscriptionId
	if success.Code != http.StatusFound || success.Header().Get("Location") != want {
		t.Fatalf("success page: %d %s", success.Code, success.Header().Get("Location"))
	}
	if w := get(t, body["success_url"]); w.Code != http.StatusUnauthorized {
		t.Fatalf("success without cookie status=%d", w.Code)
	}
}

func TestHandleStripeSuccess(t *testing.T) {
	setupHandlers(t)
	a := stripeAttempt(t, "onetime")
	stripeAPI(t, map[string]string{
		"GET /v1/checkout/sessions/" + a.ProviderOrderId:                 stripeSession(a, `,"payment_intent":"pi_ok"`),
		"GET /v1/checkout/sessions/" + a.ProviderOrderId + "/line_items": stripeLineItems,
	})
	path := "/subscriptions/stripe-success?session_id=" + a.ProviderOrderId
	if w := get(t, path+"&product_id=2", browserCookie(a)); w.Code != http.StatusBadRequest {
		t.Fatalf("product_id accepted: %d", w.Code)
	}
	if w := get(t, path); w.Code != http.StatusUnauthorized {
		t.Fatalf("unbound browser status=%d", w.Code)
	}
	if w := get(t, "/subscriptions/stripe-success?session_id=cs_unknown", browserCookie(a)); w.Code != http.StatusUnauthorized {
		t.Fatalf("unknown session status=%d", w.Code)
	}
	w := get(t, path, browserCookie(a))
	if w.Code != http.StatusFound || w.Header().Get("Location") != "/subscriptions/success?attempt_id="+a.Id || cookieNamed(w, CompletionCookie) == nil {
		t.Fatalf("success: %d %s", w.Code, w.Header().Get("Location"))
	}
	if attemptStatus(t, a.Id) != "completed" {
		t.Fatal("not fulfilled")
	}
	// Replaying the callback is harmless.
	if w := get(t, path, browserCookie(a)); w.Code != http.StatusFound || testenv.Scalar(t, "SELECT COUNT(*) FROM subscriptions") != 1 {
		t.Fatalf("replay: %d", w.Code)
	}
	unpaid := stripeAttempt(t, "onetime")
	stripeAPI(t, map[string]string{"GET /v1/checkout/sessions/" + unpaid.ProviderOrderId: strings.Replace(stripeSession(unpaid, ""), `"paid"`, `"unpaid"`, 1)})
	if w := get(t, "/subscriptions/stripe-success?session_id="+unpaid.ProviderOrderId, browserCookie(unpaid)); w.Code != http.StatusBadRequest {
		t.Fatalf("unpaid session status=%d", w.Code)
	}
}

func paypalCheckout(t *testing.T, schedule string) (*PaymentAttempt, *http.Cookie) {
	t.Helper()
	price := ""
	if schedule != "onetime" {
		price = "P-PLAN"
	}
	a, err := newAttempt(1, "paypal", schedule, "DF", 1050, "USD", price, "cust", "")
	if err != nil {
		t.Fatal(err)
	}
	return a, browserCookie(a)
}

func TestPaypalCheckoutHandlers(t *testing.T) {
	setupHandlers(t)
	var created map[string]interface{}
	api := mockProviderAPI(t, nil)
	api.routes = map[string]string{
		"POST /v2/checkout/orders":                  `{"id":"ORDERNEW","status":"CREATED"}`,
		"POST /v2/checkout/orders/ORDERNEW/capture": `{"id":"ORDERNEW","status":"COMPLETED"}`,
		"POST /v1/billing/subscriptions":            `{"id":"I-NEW","status":"APPROVAL_PENDING"}`,
	}
	mockPayments(t, func(r *http.Request) string {
		if strings.HasSuffix(r.URL.Path, "/oauth2/token") {
			return `{"access_token":"token"}`
		}
		if r.Method == http.MethodPost && r.URL.Path == "/v2/checkout/orders" {
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &created)
			if r.Header.Get("PayPal-Request-Id") == "" {
				t.Error("missing idempotency key")
			}
		}
		return api.routes[r.Method+" "+r.URL.Path]
	})
	one, cookie := paypalCheckout(t, "onetime")
	if w := post(t, "/subscriptions/paypal/orders", nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("order without checkout cookie status=%d", w.Code)
	}
	if w := testenv.Serve(t, testenv.POST("/subscriptions/paypal/orders", url.Values{})); w.Code != http.StatusUnauthorized {
		t.Fatalf("order without CSRF token status=%d", w.Code)
	}
	w := post(t, "/subscriptions/paypal/orders", nil, cookie)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "ORDERNEW") {
		t.Fatalf("create order: %d %s", w.Code, w.Body.String())
	}
	unit := created["purchase_units"].([]interface{})[0].(map[string]interface{})
	if unit["reference_id"] != one.Id || unit["amount"].(map[string]interface{})["value"] != "10.50" {
		t.Fatalf("order payload: %v", unit)
	}
	// Repeating returns the bound order without another provider call.
	created = nil
	if w := post(t, "/subscriptions/paypal/orders", nil, cookie); !strings.Contains(w.Body.String(), "ORDERNEW") || created != nil {
		t.Fatal("second create order contacted PayPal")
	}
	if w := post(t, "/subscriptions/paypal/orders/OTHER/capture", nil, cookie); w.Code != http.StatusUnauthorized {
		t.Fatalf("capture of another order status=%d", w.Code)
	}
	if w := post(t, "/subscriptions/paypal/orders/ORDERNEW/capture", nil, cookie); w.Code != 200 || !strings.Contains(w.Body.String(), "COMPLETED") {
		t.Fatalf("capture: %d %s", w.Code, w.Body.String())
	}
	// A one-time capability cannot create a subscription and vice versa.
	if w := post(t, "/subscriptions/paypal/subscriptions", nil, cookie); w.Code != http.StatusUnauthorized {
		t.Fatalf("subscription with one-time capability status=%d", w.Code)
	}
	sub, subCookie := paypalCheckout(t, "monthly")
	if w := post(t, "/subscriptions/paypal/orders", nil, subCookie); w.Code != http.StatusUnauthorized {
		t.Fatalf("order with subscription capability status=%d", w.Code)
	}
	if w := post(t, "/subscriptions/paypal/subscriptions", nil, subCookie); w.Code != 200 || !strings.Contains(w.Body.String(), "I-NEW") {
		t.Fatalf("create subscription: %d %s", w.Code, w.Body.String())
	}
	if a, _ := FindAttempt(sub.Id); a.ProviderSubscriptionId != "I-NEW" {
		t.Fatal("subscription not bound")
	}
}

func razorpaySign(payload string) string {
	h := hmac.New(sha256.New, []byte("rzp_secret"))
	h.Write([]byte(payload))
	return hex.EncodeToString(h.Sum(nil))
}

func TestRazorpaySuccessCallbacks(t *testing.T) {
	setupHandlers(t)
	one, err := newAttempt(1, "razorpay", "onetime", "DF", 1050, "USD", "", "cust", "https://client.test/done")
	if err != nil {
		t.Fatal(err)
	}
	if err := one.SetProviderIds("order_1", "", ""); err != nil {
		t.Fatal(err)
	}
	sub, err := newAttempt(1, "razorpay", "monthly", "DF", 1050, "USD", "plan_1", "cust", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := sub.SetProviderIds("", "", "sub_1"); err != nil {
		t.Fatal(err)
	}
	testenv.Exec(t, "UPDATE products SET webhook_url='https://client.test/hook'")
	subStatus := "created"
	mockPayments(t, func(r *http.Request) string {
		switch r.URL.Path {
		case "/v1/payments/pay_1":
			return `{"id":"pay_1","order_id":"order_1","status":"captured","captured":true,"amount":1050,"currency":"USD","email":"b@example.test"}`
		case "/v1/orders/order_1":
			return fmt.Sprintf(`{"id":"order_1","receipt":%q,"status":"paid","amount":1050,"amount_paid":1050,"currency":"USD"}`, one.Id)
		case "/v1/subscriptions/sub_1":
			return fmt.Sprintf(`{"id":"sub_1","plan_id":"plan_1","status":%q,"notes":{"attempt_id":%q}}`, subStatus, sub.Id)
		case "/v1/payments/pay_2":
			return `{"id":"pay_2","order_id":"order_2","invoice_id":"inv_2","status":"captured","captured":true,"amount":1050,"currency":"USD"}`
		case "/v1/invoices/inv_2":
			return `{"id":"inv_2","subscription_id":"sub_1","payment_id":"pay_2","order_id":"order_2","status":"paid"}`
		}
		t.Errorf("unexpected request %s", r.URL)
		return "{}"
	})
	orderQuery := func(sig string) string {
		return "/subscriptions/success?" + url.Values{"razorpay_order_id": {"order_1"}, "razorpay_payment_id": {"pay_1"}, "razorpay_signature": {sig}}.Encode()
	}
	failure := "/subscriptions/failure?errorDetail=Payment+verification+failed"
	if w := get(t, orderQuery(razorpaySign("order_1|pay_1"))+"&product_id=1", browserCookie(one)); w.Code != http.StatusBadRequest {
		t.Fatalf("product_id accepted: %d", w.Code)
	}
	for name, w := range map[string]*httptest.ResponseRecorder{
		"bad signature": get(t, orderQuery("bad"), browserCookie(one)),
		"other browser": get(t, orderQuery(razorpaySign("order_1|pay_1")), browserCookie(sub)),
		"no cookie":     get(t, orderQuery(razorpaySign("order_1|pay_1"))),
		"unknown order": get(t, "/subscriptions/success?razorpay_order_id=nope&razorpay_payment_id=pay_1&razorpay_signature=x", browserCookie(one)),
		"null callback": get(t, "/subscriptions/success?razorpay_order_id=null&razorpay_signature=x", browserCookie(one)),
	} {
		if w.Code != http.StatusFound || w.Header().Get("Location") != failure {
			t.Fatalf("%s: %d %s", name, w.Code, w.Header().Get("Location"))
		}
	}
	if attemptStatus(t, one.Id) != "pending" {
		t.Fatal("rejected callback fulfilled")
	}
	w := get(t, orderQuery(razorpaySign("order_1|pay_1")), browserCookie(one))
	if w.Code != http.StatusFound || w.Header().Get("Location") != "/subscriptions/success?attempt_id="+one.Id {
		t.Fatalf("order success: %d %s", w.Code, w.Header().Get("Location"))
	}
	// The clean URL redirects to the validated merchant redirect with frozen params.
	done := get(t, w.Header().Get("Location"), cookieNamed(w, CompletionCookie))
	if done.Code != http.StatusFound || done.Header().Get("Location") != "https://client.test/done?custom_id=cust&order_id=order_1" {
		t.Fatalf("merchant redirect: %d %s", done.Code, done.Header().Get("Location"))
	}

	subQuery := "/subscriptions/success?" + url.Values{"razorpay_subscription_id": {"sub_1"}, "razorpay_payment_id": {"pay_2"}, "razorpay_signature": {razorpaySign("pay_2|sub_1")}}.Encode()
	if w := get(t, subQuery, browserCookie(sub)); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "verification in progress") {
		t.Fatalf("pending subscription: %d", w.Code)
	}
	subStatus = "active"
	if w := get(t, subQuery, browserCookie(sub)); w.Code != http.StatusFound || attemptStatus(t, sub.Id) != "completed" {
		t.Fatalf("subscription success: %d", w.Code)
	}
}

func TestPaypalSuccessCallbacks(t *testing.T) {
	setupHandlers(t)
	one, cookie := paypalCheckout(t, "onetime")
	if err := one.SetProviderIds("ORDER_S", "", ""); err != nil {
		t.Fatal(err)
	}
	sub, subCookie := paypalCheckout(t, "yearly")
	if err := sub.SetProviderIds("", "", "I-S"); err != nil {
		t.Fatal(err)
	}
	status := "APPROVAL_PENDING"
	mockProviderAPI(t, map[string]string{})
	mockPayments(t, func(r *http.Request) string {
		switch {
		case strings.HasSuffix(r.URL.Path, "/oauth2/token"):
			return `{"access_token":"token"}`
		case r.URL.Path == "/v2/checkout/orders/ORDER_S":
			return paypalOrder(one, "CAP_S")
		case r.URL.Path == "/v2/checkout/orders/ORDER_X":
			return `{"id":"ORDER_X","purchase_units":[]}`
		case r.URL.Path == "/v1/billing/subscriptions/I-S":
			return fmt.Sprintf(`{"id":"I-S","plan_id":"P-PLAN","custom_id":%q,"status":%q}`, sub.Id, status)
		case r.URL.Path == "/v1/billing/subscriptions/I-S/transactions":
			return fmt.Sprintf(`{"transactions":[{"id":"T-S","status":"COMPLETED","time":%q,"amount_with_breakdown":{"gross_amount":{"currency_code":"USD","value":"10.50"}}}]}`, time.Now().UTC().Format(time.RFC3339))
		}
		t.Errorf("unexpected request %s", r.URL)
		return "{}"
	})
	failure := "/subscriptions/failure?errorDetail=Payment+verification+failed"
	for name, w := range map[string]*httptest.ResponseRecorder{
		"other browser": get(t, "/subscriptions/success?paypal_orderid=ORDER_S", subCookie),
		"empty order":   get(t, "/subscriptions/success?paypal_orderid=ORDER_X", cookie),
		"unknown sub":   get(t, "/subscriptions/success?paypal_subscriptionid=I-NOPE", subCookie),
	} {
		if w.Header().Get("Location") != failure {
			t.Fatalf("%s: %d %s", name, w.Code, w.Header().Get("Location"))
		}
	}
	if w := get(t, "/subscriptions/success?paypal_orderid=ORDER_S", cookie); w.Code != http.StatusFound || attemptStatus(t, one.Id) != "completed" {
		t.Fatalf("order success: %d", w.Code)
	}
	if w := get(t, "/subscriptions/success?paypal_subscriptionid=I-S", subCookie); w.Code != http.StatusOK || attemptStatus(t, sub.Id) != "pending" {
		t.Fatalf("pending subscription: %d", w.Code)
	}
	status = "ACTIVE"
	if w := get(t, "/subscriptions/success?paypal_subscriptionid=I-S", subCookie); w.Code != http.StatusFound || attemptStatus(t, sub.Id) != "completed" {
		t.Fatalf("active subscription: %d", w.Code)
	}
	// Clean URL: pending attempts show the verification page only to their browser.
	pending, pendingCookie := paypalCheckout(t, "onetime")
	if w := get(t, "/subscriptions/success?attempt_id="+pending.Id, pendingCookie); w.Code != http.StatusOK {
		t.Fatalf("pending page status=%d", w.Code)
	}
	if w := get(t, "/subscriptions/success?attempt_id="+pending.Id); w.Code != http.StatusUnauthorized {
		t.Fatalf("pending page without cookie status=%d", w.Code)
	}
}

func TestHandleCancellationLink(t *testing.T) {
	setupHandlers(t)
	admin := testenv.SeedUser(t, testenv.RoleAdmin, "admin@merchant.test", "admin-password")
	reader := testenv.SeedUser(t, testenv.RoleReader, "reader@merchant.test", "reader-password")
	a, _ := fulfilled(t, "razorpay", "monthly")
	link := func(user int64, id string) *httptest.ResponseRecorder {
		return testenv.Serve(t, testenv.AuthedPOST(t, "/subscriptions/cancellation-link", url.Values{"subscription_id": {id}}, user))
	}
	for name, user := range map[string]int64{"anon": 0, "reader": reader} {
		if w := link(user, a.ProviderSubscriptionId); w.Code != http.StatusUnauthorized {
			t.Fatalf("%s status=%d", name, w.Code)
		}
	}
	w := link(admin, a.ProviderSubscriptionId)
	u, err := url.Parse(w.Body.String())
	if w.Code != 200 || err != nil || u.Query().Get("subscription_id") != a.ProviderSubscriptionId || !strings.HasPrefix(w.Body.String(), testenv.RootURL+"/subscriptions/cancel?") {
		t.Fatalf("link: %d %q", w.Code, w.Body.String())
	}
	token := u.Query().Get("cancellation_token")
	fresh, _ := FindAttempt(a.Id)
	if !fresh.CancellationTokenValid(token) {
		t.Fatal("issued token invalid")
	}
	// Reissuing replaces the previous capability.
	w = link(admin, a.ProviderSubscriptionId)
	fresh, _ = FindAttempt(a.Id)
	if fresh.CancellationTokenValid(token) {
		t.Fatal("old token still valid after reissue")
	}
	// Legacy subscription: a non-fulfillable correlation record is created.
	testenv.Exec(t, `INSERT INTO subscriptions(pg,subscr_id,item_number,payment_status,user_id) VALUES('stripe','sub_old',1,'ACTIVE','cust')`)
	testenv.Exec(t, "UPDATE products SET schedule='monthly'")
	if w := link(admin, "sub_old"); w.Code != 200 || !strings.Contains(w.Body.String(), "subscription_id=sub_old") {
		t.Fatalf("legacy link: %d %s", w.Code, w.Body.String())
	}
	legacy, err := FindAttemptByProviderSubscription("stripe", "sub_old")
	if err != nil || legacy.Status != "legacy" {
		t.Fatalf("legacy attempt: %+v %v", legacy, err)
	}
	testenv.Exec(t, `INSERT INTO subscriptions(pg,subscr_id,item_number,payment_status,user_id) VALUES('stripe','one_old',1,'ACTIVE','cust')`)
	testenv.Exec(t, "UPDATE products SET schedule='onetime'")
	if w := link(admin, "one_old"); w.Code != http.StatusBadRequest {
		t.Fatalf("one-time legacy status=%d", w.Code)
	}
	if w := link(admin, "sub_missing"); w.Code != http.StatusNotFound {
		t.Fatalf("missing subscription status=%d", w.Code)
	}
}

func checkoutPrices(t *testing.T, prices map[string]string, sessions *url.Values) {
	t.Helper()
	testenv.StripeBackend(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/v1/checkout/sessions" {
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			*sessions = r.Form
			return testenv.Response(200, `{"id":"cs_new","url":"https://checkout.stripe.test/pay"}`), nil
		}
		if body, ok := prices[r.URL.Path]; ok {
			return testenv.Response(200, body), nil
		}
		return testenv.Response(404, `{"error":{"message":"missing"}}`), nil
	})
}

func TestCheckoutBranches(t *testing.T) {
	setupHandlers(t)
	testenv.Set("subscription_client_country", "IN")
	testenv.Set("stripe_tax_rate_IN", "txr_IN")
	testenv.Set("stripe_callback_domain", "https://merchant.test")
	var sessions url.Values
	checkoutPrices(t, map[string]string{
		"/v1/prices/price_once":     `{"id":"price_once","active":true,"type":"one_time","unit_amount":1000,"currency":"usd"}`,
		"/v1/prices/price_month":    `{"id":"price_month","active":true,"type":"recurring","unit_amount":1000,"currency":"usd","recurring":{"interval":"month","interval_count":1}}`,
		"/v1/prices/price_quarter":  `{"id":"price_quarter","active":true,"type":"recurring","unit_amount":1000,"currency":"usd","recurring":{"interval":"month","interval_count":3}}`,
		"/v1/prices/price_inactive": `{"id":"price_inactive","active":false,"type":"one_time","unit_amount":1000,"currency":"usd"}`,
		"/v1/tax_rates/txr_IN":      `{"id":"txr_IN","active":true,"inclusive":false,"percentage":18}`,
	}, &sessions)
	checkout := func(schedule string, prices map[string]string) *httptest.ResponseRecorder {
		b, _ := json.Marshal(prices)
		testenv.Exec(t, "UPDATE products SET schedule=?, stripe_price=? WHERE id=1", schedule, string(b))
		sessions = nil
		return post(t, "/subscriptions/create-checkout-session", url.Values{"productId": {"1"}, "priceId": {"price_cheap"}})
	}
	if w := get(t, "/subscriptions/create-checkout-session"); w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET status=%d", w.Code)
	}
	if w := checkout("onetime", nil); w.Code != http.StatusFound || !strings.Contains(w.Header().Get("Location"), "No+price+configured") {
		t.Fatalf("no price: %d %s", w.Code, w.Header().Get("Location"))
	}
	for name, c := range map[string]struct {
		schedule string
		price    string
	}{
		"inactive":           {"onetime", "price_inactive"},
		"recurring for once": {"onetime", "price_month"},
		"once for recurring": {"monthly", "price_once"},
		"wrong interval":     {"yearly", "price_month"},
		"interval count":     {"monthly", "price_quarter"},
	} {
		if w := checkout(c.schedule, map[string]string{"DF": c.price}); w.Code != http.StatusBadRequest || sessions != nil {
			t.Errorf("%s: status=%d session created=%v", name, w.Code, sessions != nil)
		}
	}
	if w := checkout("onetime", map[string]string{"DF": "price_missing"}); w.Code != http.StatusInternalServerError {
		t.Fatalf("unknown price status=%d", w.Code)
	}
	// Country price wins and its tax rate is applied to the line item.
	w := checkout("monthly", map[string]string{"IN": "price_month", "DF": "price_once"})
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "https://checkout.stripe.test/pay" {
		t.Fatalf("subscription checkout: %d %s", w.Code, w.Header().Get("Location"))
	}
	attemptID := sessions.Get("metadata[attempt_id]")
	if sessions.Get("mode") != "subscription" || sessions.Get("line_items[0][price]") != "price_month" || sessions.Get("subscription_data[default_tax_rates][0]") != "txr_IN" || sessions.Get("line_items[0][tax_rates][0]") != "" {
		t.Fatalf("session params: %v", sessions)
	}
	if sessions.Get("subscription_data[metadata][attempt_id]") != attemptID || sessions.Get("metadata[product_id]") != "1" || !strings.HasPrefix(sessions.Get("success_url"), "https://merchant.test/subscriptions/stripe-success") {
		t.Fatalf("session metadata: %v", sessions)
	}
	a, err := FindAttempt(attemptID)
	if err != nil || a.Amount != 1180 || a.Schedule != "monthly" || a.Country != "IN" || a.ProviderOrderId != "cs_new" || a.PriceId != "price_month" {
		t.Fatalf("attempt: %+v %v", a, err)
	}
	bound := cookieNamed(w, AttemptCookie)
	if bound == nil || !strings.HasPrefix(bound.Value, a.Id+".") || !bound.HttpOnly {
		t.Fatalf("attempt cookie %+v", bound)
	}
	if w := post(t, "/subscriptions/create-checkout-session", url.Values{"productId": {"abc"}}); w.Code != http.StatusBadRequest {
		t.Fatalf("bad product id status=%d", w.Code)
	}
	if w := post(t, "/subscriptions/create-checkout-session", url.Values{"productId": {"999"}}); w.Code != http.StatusNotFound {
		t.Fatalf("unknown product status=%d", w.Code)
	}
	if w := testenv.Serve(t, testenv.POST("/subscriptions/create-checkout-session", url.Values{"productId": {"1"}})); w.Code != http.StatusUnauthorized {
		t.Fatalf("missing CSRF token status=%d", w.Code)
	}
}
