package subscriptions

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/abishekmuthian/open-payment-host/src/internal/testenv"
)

// jsonWith returns base with top-level (or dotted nested) fields replaced.
func jsonWith(t *testing.T, base string, changes map[string]interface{}) string {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(base), &m); err != nil {
		t.Fatal(err)
	}
	for path, v := range changes {
		parts := strings.Split(path, ".")
		target := m
		for _, p := range parts[:len(parts)-1] {
			target = target[p].(map[string]interface{})
		}
		target[parts[len(parts)-1]] = v
	}
	b, _ := json.Marshal(m)
	return string(b)
}

// routedProvider answers paymentHTTPClient by URL path from a mutable map.
func routedProvider(t *testing.T, routes map[string]string) {
	t.Helper()
	mockPayments(t, func(r *http.Request) string {
		if strings.HasSuffix(r.URL.Path, "/oauth2/token") {
			return `{"access_token":"token"}`
		}
		if body, ok := routes[r.URL.Path]; ok {
			return body
		}
		t.Errorf("unexpected request %s", r.URL.Path)
		return "{}"
	})
}

func TestPaypalOrderFactsGuards(t *testing.T) {
	setupPayments(t)
	a, err := newAttempt(1, "paypal", "onetime", "DF", 1050, "USD", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.SetProviderIds("ORDER", "", ""); err != nil {
		t.Fatal(err)
	}
	base := paypalOrder(a, "CAP")
	parse := func(s string) *PayPalOrderDetailsResult {
		var o PayPalOrderDetailsResult
		if err := json.Unmarshal([]byte(s), &o); err != nil {
			t.Fatal(err)
		}
		return &o
	}
	f, err := paypalOrderFacts(parse(base), a)
	if err != nil || f.Amount != 1050 || f.Currency != "USD" || f.PaymentId != "CAP" || f.OrderId != "ORDER" || f.Email != "buyer@example.test" || f.Name != "Buyer" {
		t.Fatalf("valid order: %+v %v", f, err)
	}
	unit := func(field string, v interface{}) string {
		var m map[string]interface{}
		_ = json.Unmarshal([]byte(base), &m)
		u := m["purchase_units"].([]interface{})[0].(map[string]interface{})
		parts := strings.Split(field, ".")
		target := u
		for _, p := range parts[:len(parts)-1] {
			switch x := target[p].(type) {
			case []interface{}:
				target = x[0].(map[string]interface{})
			default:
				target = x.(map[string]interface{})
			}
		}
		target[parts[len(parts)-1]] = v
		b, _ := json.Marshal(m)
		return string(b)
	}
	cases := map[string]struct {
		body    string
		pending bool
	}{
		"order id":        {jsonWith(t, base, map[string]interface{}{"id": "OTHER"}), false},
		"order status":    {jsonWith(t, base, map[string]interface{}{"status": "APPROVED"}), false},
		"two units":       {strings.Replace(base, `"purchase_units":[{`, `"purchase_units":[{"reference_id":"x"},{`, 1), false},
		"reference":       {unit("reference_id", "another-attempt"), false},
		"sku":             {unit("items.sku", "2"), false},
		"quantity":        {unit("items.quantity", "2"), false},
		"capture pending": {unit("payments.captures.status", "PENDING"), true},
		"not final":       {unit("payments.captures.final_capture", false), true},
		"capture amount":  {unit("payments.captures.amount.value", "1.00"), false},
		"bad amount":      {unit("payments.captures.amount.value", "10.505"), false},
		"total mismatch":  {unit("amount.value", "20.00"), false},
		"currency":        {unit("payments.captures.amount.currency_code", "EUR"), false},
	}
	for name, c := range cases {
		f, err := paypalOrderFacts(parse(c.body), a)
		if err == nil {
			// A mismatched amount is caught later by validatePayment.
			if verr := validatePayment(a, f); verr == nil {
				t.Errorf("%s: accepted %+v", name, f)
			}
			continue
		}
		if errors.Is(err, errPaymentPending) != c.pending {
			t.Errorf("%s: pending=%v err=%v", name, errors.Is(err, errPaymentPending), err)
		}
	}
	if _, err := paypalOrderFacts(nil, a); err == nil {
		t.Error("nil order accepted")
	}
	sub, _ := newAttempt(1, "paypal", "monthly", "DF", 1050, "USD", "P", "", "")
	if _, err := paypalOrderFacts(parse(base), sub); err == nil {
		t.Error("subscription attempt accepted an order")
	}
}

func TestRazorpayPaymentFactsGuards(t *testing.T) {
	setupPayments(t)
	a, err := newAttempt(1, "razorpay", "onetime", "DF", 1050, "USD", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.SetProviderIds("order_1", "", ""); err != nil {
		t.Fatal(err)
	}
	payment := `{"id":"pay_1","order_id":"order_1","status":"captured","captured":true,"amount":1050,"currency":"USD","amount_refunded":0,"email":"b@example.test"}`
	order := fmt.Sprintf(`{"id":"order_1","receipt":%q,"status":"paid","amount":1050,"amount_paid":1050,"currency":"USD"}`, a.Id)
	routes := map[string]string{"/v1/payments/pay_1": payment, "/v1/orders/order_1": order}
	routedProvider(t, routes)
	f, err := fetchRazorpayPaymentFacts("order_1", "pay_1")
	if err != nil || f.Amount != 1050 || f.OrderId != "order_1" || f.PaymentId != "pay_1" || f.Email != "b@example.test" {
		t.Fatalf("valid: %+v %v", f, err)
	}
	for name, c := range map[string]struct {
		path    string
		changes map[string]interface{}
	}{
		"payment id":      {"/v1/payments/pay_1", map[string]interface{}{"id": "pay_2"}},
		"payment order":   {"/v1/payments/pay_1", map[string]interface{}{"order_id": "order_2"}},
		"authorized only": {"/v1/payments/pay_1", map[string]interface{}{"status": "authorized"}},
		"not captured":    {"/v1/payments/pay_1", map[string]interface{}{"captured": false}},
		"refunded":        {"/v1/payments/pay_1", map[string]interface{}{"amount_refunded": 1}},
		"receipt":         {"/v1/orders/order_1", map[string]interface{}{"receipt": "other"}},
		"order unpaid":    {"/v1/orders/order_1", map[string]interface{}{"status": "attempted"}},
		"order amount":    {"/v1/orders/order_1", map[string]interface{}{"amount": 1}},
		"partly paid":     {"/v1/orders/order_1", map[string]interface{}{"amount_paid": 1}},
		"order currency":  {"/v1/orders/order_1", map[string]interface{}{"currency": "INR"}},
		"order id":        {"/v1/orders/order_1", map[string]interface{}{"id": "order_x"}},
	} {
		routes["/v1/payments/pay_1"], routes["/v1/orders/order_1"] = payment, order
		routes[c.path] = jsonWith(t, routes[c.path], c.changes)
		if f, err := fetchRazorpayPaymentFacts("order_1", "pay_1"); err == nil {
			t.Errorf("%s: accepted %+v", name, f)
		}
	}
}

func TestRazorpaySubscriptionFactsGuards(t *testing.T) {
	setupPayments(t)
	a, err := newAttempt(1, "razorpay", "monthly", "DF", 1050, "USD", "plan_1", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.SetProviderIds("", "", "sub_1"); err != nil {
		t.Fatal(err)
	}
	sub := fmt.Sprintf(`{"id":"sub_1","plan_id":"plan_1","status":"active","notes":{"attempt_id":%q}}`, a.Id)
	payment := `{"id":"pay_1","order_id":"order_1","invoice_id":"inv_1","status":"captured","captured":true,"amount":1050,"currency":"USD"}`
	invoice := `{"id":"inv_1","subscription_id":"sub_1","payment_id":"pay_1","order_id":"order_1","status":"paid"}`
	routes := map[string]string{}
	reset := func() {
		routes["/v1/subscriptions/sub_1"], routes["/v1/payments/pay_1"], routes["/v1/invoices/inv_1"] = sub, payment, invoice
	}
	reset()
	routedProvider(t, routes)
	if f, err := fetchRazorpaySubscriptionFacts("sub_1", "pay_1"); err != nil || f.PriceId != "plan_1" || f.SubscriptionId != "sub_1" {
		t.Fatalf("valid: %+v %v", f, err)
	}
	for name, c := range map[string]struct {
		path    string
		changes map[string]interface{}
		pending bool
	}{
		"plan":             {"/v1/subscriptions/sub_1", map[string]interface{}{"plan_id": "plan_cheap"}, false},
		"notes":            {"/v1/subscriptions/sub_1", map[string]interface{}{"notes": map[string]string{"attempt_id": "x"}}, false},
		"created":          {"/v1/subscriptions/sub_1", map[string]interface{}{"status": "created"}, true},
		"payment captured": {"/v1/payments/pay_1", map[string]interface{}{"captured": false}, true},
		"payment refunded": {"/v1/payments/pay_1", map[string]interface{}{"amount_refunded": 5}, true},
		"no invoice":       {"/v1/payments/pay_1", map[string]interface{}{"invoice_id": ""}, false},
		"invoice sub":      {"/v1/invoices/inv_1", map[string]interface{}{"subscription_id": "sub_2"}, false},
		"invoice payment":  {"/v1/invoices/inv_1", map[string]interface{}{"payment_id": "pay_2"}, false},
		"invoice order":    {"/v1/invoices/inv_1", map[string]interface{}{"order_id": "order_2"}, false},
		"invoice unpaid":   {"/v1/invoices/inv_1", map[string]interface{}{"status": "issued"}, false},
	} {
		reset()
		routes[c.path] = jsonWith(t, routes[c.path], c.changes)
		_, err := fetchRazorpaySubscriptionFacts("sub_1", "pay_1")
		if err == nil || errors.Is(err, errPaymentPending) != c.pending {
			t.Errorf("%s: err=%v pending=%v", name, err, c.pending)
		}
	}
	reset()
	routes["/v1/subscriptions/sub_unknown"] = `{"id":"sub_unknown","plan_id":"plan_1","status":"active"}`
	if _, err := fetchRazorpaySubscriptionFacts("sub_unknown", "pay_1"); err == nil {
		t.Error("unknown subscription accepted")
	}
}

func TestSquareFactsGuards(t *testing.T) {
	setupPayments(t)
	testenv.Set("square_domain", "https://square.test/v2")
	one, _ := newAttempt(1, "square", "onetime", "DF", 1050, "USD", "", "", "")
	sub, _ := newAttempt(1, "square", "yearly", "DF", 1050, "USD", "PLAN", "", "")
	if err := sub.SetProviderIds("", "", "SUB"); err != nil {
		t.Fatal(err)
	}
	payment := fmt.Sprintf(`{"payment":{"id":"PAY","reference_id":%q,"status":"COMPLETED","order_id":"ORD","amount_money":{"amount":1050,"currency":"USD"}}}`, one.Id)
	routes := map[string]string{
		"/v2/payments/PAY":      payment,
		"/v2/invoices/INV":      `{"invoice":{"id":"INV","status":"PAID","subscription_id":"SUB","order_id":"ORD"}}`,
		"/v2/subscriptions/SUB": `{"subscription":{"id":"SUB","plan_id":"PLAN"}}`,
		"/v2/orders/ORD":        `{"order":{"id":"ORD","state":"COMPLETED","tenders":[{"payment_id":"PAY"}]}}`,
	}
	routedProvider(t, routes)
	if f, err := fetchSquarePaymentFacts(one, "PAY"); err != nil || f.Amount != 1050 || f.PaymentId != "PAY" {
		t.Fatalf("valid payment: %+v %v", f, err)
	}
	if f, err := fetchSquareInvoiceFacts(sub, "INV"); err != nil || f.SubscriptionId != "SUB" || f.PriceId != "PLAN" {
		t.Fatalf("valid invoice: %+v %v", f, err)
	}
	if _, err := fetchSquarePaymentFacts(sub, "PAY"); err == nil {
		t.Error("subscription attempt accepted a one-time payment")
	}
	other, _ := newAttempt(1, "square", "onetime", "DF", 1050, "USD", "", "", "")
	if _, err := fetchSquarePaymentFacts(other, "PAY"); err == nil {
		t.Error("payment for another attempt accepted")
	}
	original := map[string]string{}
	for k, v := range routes {
		original[k] = v
	}
	for name, c := range map[string]struct {
		path, body string
		pending    bool
	}{
		"invoice unpaid":    {"/v2/invoices/INV", `{"invoice":{"id":"INV","status":"UNPAID","subscription_id":"SUB","order_id":"ORD"}}`, true},
		"invoice other sub": {"/v2/invoices/INV", `{"invoice":{"id":"INV","status":"PAID","subscription_id":"SUB2","order_id":"ORD"}}`, true},
		"plan mismatch":     {"/v2/subscriptions/SUB", `{"subscription":{"id":"SUB","plan_id":"CHEAP"}}`, false},
		"order open":        {"/v2/orders/ORD", `{"order":{"id":"ORD","state":"OPEN","tenders":[{"payment_id":"PAY"}]}}`, false},
		"split tender":      {"/v2/orders/ORD", `{"order":{"id":"ORD","state":"COMPLETED","tenders":[{"payment_id":"PAY"},{"payment_id":"PAY2"}]}}`, false},
		"no payment":        {"/v2/orders/ORD", `{"order":{"id":"ORD","state":"COMPLETED","tenders":[{}]}}`, false},
		"payment order":     {"/v2/payments/PAY", strings.Replace(payment, `"ORD"`, `"ORD2"`, 1), false},
		"payment status":    {"/v2/payments/PAY", strings.Replace(payment, "COMPLETED", "APPROVED", 1), false},
	} {
		for k, v := range original {
			routes[k] = v
		}
		routes[c.path] = c.body
		_, err := fetchSquareInvoiceFacts(sub, "INV")
		if err == nil || errors.Is(err, errPaymentPending) != c.pending {
			t.Errorf("%s: err=%v", name, err)
		}
	}
}

func TestValidateSquarePlanGuards(t *testing.T) {
	setupPayments(t)
	body := ""
	mockPayments(t, func(r *http.Request) string { return body })
	plan := func(id string, phases string) string {
		return fmt.Sprintf(`{"object":{"id":%q,"subscription_plan_data":{"phases":[%s]}}}`, id, phases)
	}
	phase := func(cadence string, amount int, currency string) string {
		return fmt.Sprintf(`{"cadence":%q,"recurring_price_money":{"amount":%d,"currency":%q}}`, cadence, amount, currency)
	}
	body = plan("P", phase("MONTHLY", 1050, "usd"))
	if err := validateSquarePlan("P", "monthly", 1050, "USD"); err != nil {
		t.Fatal(err)
	}
	for name, b := range map[string]string{
		"other plan": plan("Q", phase("MONTHLY", 1050, "USD")),
		"two phases": plan("P", phase("MONTHLY", 1, "USD")+","+phase("MONTHLY", 1050, "USD")),
		"no phases":  plan("P", ""),
		"amount":     plan("P", phase("MONTHLY", 100, "USD")),
		"currency":   plan("P", phase("MONTHLY", 1050, "EUR")),
		"cadence":    plan("P", phase("WEEKLY", 1050, "USD")),
	} {
		body = b
		if err := validateSquarePlan("P", "monthly", 1050, "USD"); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestProviderJSONErrors(t *testing.T) {
	setupPayments(t)
	if err := providerJSON("bitcoin", http.MethodGet, "/x", nil, nil); err == nil {
		t.Fatal("unknown gateway accepted")
	}
	old := paymentHTTPClient
	t.Cleanup(func() { paymentHTTPClient = old })
	status := http.StatusInternalServerError
	var seen *http.Request
	paymentHTTPClient = &http.Client{Transport: paymentRoundTripper(func(r *http.Request) (*http.Response, error) {
		seen = r
		return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"id":"x"}`))}, nil
	})}
	var out struct{ ID string }
	if err := providerJSON("razorpay", http.MethodGet, "/payments/x", nil, &out); err == nil || strings.Contains(err.Error(), "rzp") {
		t.Fatalf("HTTP 500 accepted or leaked credentials: %v", err)
	}
	testenv.Set("razorpay_key_id", "rzp_key")
	testenv.Set("razorpay_key_secret", "rzp_secret")
	status = http.StatusOK
	if err := providerJSON("razorpay", http.MethodGet, "/payments/x", nil, &out); err != nil || out.ID != "x" {
		t.Fatalf("success: %v %+v", err, out)
	}
	if user, pass, ok := seen.BasicAuth(); !ok || user != "rzp_key" || pass != "rzp_secret" || seen.URL.String() != "https://api.razorpay.com/v1/payments/x" {
		t.Fatalf("razorpay request %s auth=%v", seen.URL, ok)
	}
	if err := providerJSON("square", http.MethodPost, "/x", map[string]string{"a": "b"}, nil); err != nil || seen.Header.Get("Square-Version") == "" || !strings.HasPrefix(seen.Header.Get("Authorization"), "Bearer ") {
		t.Fatalf("square headers: %v %v", err, seen.Header)
	}
	status = http.StatusUnauthorized
	if _, err := GetPaypalAuthorizationToken(); err == nil {
		t.Fatal("PayPal token accepted on 401")
	}
}

func TestStripeSubscriptionSessionFacts(t *testing.T) {
	setupPayments(t)
	testenv.Set("stripe_secret", "sk_test")
	a := stripeAttempt(t, "monthly")
	session := stripeSession(a, `,"subscription":"sub_S"`) // older API: no invoice on the session
	routes := map[string]string{
		"GET /v1/checkout/sessions/" + a.ProviderOrderId:                 session,
		"GET /v1/checkout/sessions/" + a.ProviderOrderId + "/line_items": stripeLineItems,
		"GET /v1/invoices": `{"object":"list","data":[
			{"id":"in_renew","object":"invoice","billing_reason":"subscription_cycle","paid":true,"status":"paid","amount_paid":1050,"currency":"usd","subscription":"sub_S"},
			{"id":"in_first","object":"invoice","billing_reason":"subscription_create","paid":true,"status":"paid","amount_paid":1050,"currency":"usd","subscription":"sub_S"}],"has_more":false,"url":"/v1/invoices"}`,
	}
	stripeAPI(t, routes)
	f, err := fetchStripeSessionFacts(a)
	if err != nil || f.PaymentId != "in_first" || f.SubscriptionId != "sub_S" || f.Email != "buyer@example.test" {
		t.Fatalf("fallback invoice: %+v %v", f, err)
	}
	for name, c := range map[string]struct {
		key, body string
		pending   bool
	}{
		"mode":           {"GET /v1/checkout/sessions/" + a.ProviderOrderId, strings.Replace(session, `"mode":"subscription"`, `"mode":"payment"`, 1), false},
		"unpaid":         {"GET /v1/checkout/sessions/" + a.ProviderOrderId, strings.Replace(session, `"payment_status":"paid"`, `"payment_status":"unpaid"`, 1), true},
		"open":           {"GET /v1/checkout/sessions/" + a.ProviderOrderId, strings.Replace(session, `"status":"complete"`, `"status":"open"`, 1), true},
		"attempt":        {"GET /v1/checkout/sessions/" + a.ProviderOrderId, strings.Replace(session, a.Id, "other", 1), false},
		"no line items":  {"GET /v1/checkout/sessions/" + a.ProviderOrderId + "/line_items", `{"object":"list","data":[],"has_more":false}`, false},
		"two line items": {"GET /v1/checkout/sessions/" + a.ProviderOrderId + "/line_items", strings.Replace(stripeLineItems, `[{`, `[{"id":"li_0","object":"item","price":{"id":"price_1"},"quantity":1,"amount_total":1050,"currency":"usd"},{`, 1), false},
		"quantity":       {"GET /v1/checkout/sessions/" + a.ProviderOrderId + "/line_items", strings.Replace(stripeLineItems, `"quantity":1`, `"quantity":2`, 1), false},
		"no initial":     {"GET /v1/invoices", `{"object":"list","data":[],"has_more":false,"url":"/v1/invoices"}`, true},
		"initial unpaid": {"GET /v1/invoices", `{"object":"list","data":[{"id":"in_first","object":"invoice","billing_reason":"subscription_create","paid":false,"status":"open","amount_paid":0,"currency":"usd","subscription":"sub_S"}],"has_more":false,"url":"/v1/invoices"}`, false},
		"ambiguous": {"GET /v1/invoices", `{"object":"list","data":[
			{"id":"in_a","object":"invoice","billing_reason":"subscription_create","paid":true,"status":"paid","amount_paid":1050,"currency":"usd","subscription":"sub_S"},
			{"id":"in_b","object":"invoice","billing_reason":"subscription_create","paid":true,"status":"paid","amount_paid":1050,"currency":"usd","subscription":"sub_S"}],"has_more":false,"url":"/v1/invoices"}`, false},
	} {
		saved := routes[c.key]
		routes[c.key] = c.body
		_, err := fetchStripeSessionFacts(a)
		routes[c.key] = saved
		if err == nil || errors.Is(err, errPaymentPending) != c.pending {
			t.Errorf("%s: err=%v", name, err)
		}
	}
}

func TestPaypalSubscriptionTransactionSelection(t *testing.T) {
	setupPayments(t)
	a, err := newAttempt(1, "paypal", "monthly", "DF", 1050, "USD", "P-1", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.SetProviderIds("", "", "I-1"); err != nil {
		t.Fatal(err)
	}
	a, _ = FindAttempt(a.Id)
	tx := func(id string, at time.Time, value string) string {
		return fmt.Sprintf(`{"id":%q,"status":"COMPLETED","time":%q,"amount_with_breakdown":{"gross_amount":{"currency_code":"USD","value":%q}}}`, id, at.UTC().Format(time.RFC3339), value)
	}
	now := time.Now()
	routes := map[string]string{
		"/v1/billing/subscriptions/I-1": fmt.Sprintf(`{"id":"I-1","plan_id":"P-1","custom_id":%q,"status":"ACTIVE"}`, a.Id),
		"/v1/billing/subscriptions/I-1/transactions": fmt.Sprintf(`{"transactions":[%s,%s,%s]}`,
			tx("OLD", now.Add(-48*time.Hour), "1.00"), tx("SECOND", now.Add(2*time.Minute), "10.50"), tx("FIRST", now.Add(time.Minute), "10.50")),
	}
	routedProvider(t, routes)
	f, err := fetchPaypalSubscriptionFacts(a)
	if err != nil || f.PaymentId != "FIRST" || f.Amount != 1050 {
		t.Fatalf("initial selection: %+v %v", f, err)
	}
	if f, err := fetchPaypalSubscriptionTransactionFacts(a, "SECOND"); err != nil || f.PaymentId != "SECOND" {
		t.Fatalf("renewal selection: %+v %v", f, err)
	}
	if _, err := fetchPaypalSubscriptionTransactionFacts(a, "OLD"); !errors.Is(err, errPaymentPending) {
		t.Fatalf("transaction before the attempt accepted: %v", err)
	}
	routes["/v1/billing/subscriptions/I-1"] = fmt.Sprintf(`{"id":"I-1","plan_id":"P-CHEAP","custom_id":%q,"status":"ACTIVE"}`, a.Id)
	if _, err := fetchPaypalSubscriptionFacts(a); err == nil || errors.Is(err, errPaymentPending) {
		t.Fatalf("plan mismatch: %v", err)
	}
}
