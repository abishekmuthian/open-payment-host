package subscriptions

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/abishekmuthian/open-payment-host/src/internal/testenv"
	"github.com/abishekmuthian/open-payment-host/src/products"
)

func TestGenerateSignatureKnownAnswer(t *testing.T) {
	// RFC 4231 style vector: HMAC-SHA256(key="key", "The quick brown fox jumps over the lazy dog").
	got := GenerateSignature([]byte("The quick brown fox jumps over the lazy dog"), "key")
	if got != "f7bc83f430538424b13298e6aa6fb143ef4d59a14946175997479dbc2d1a3cd8" {
		t.Fatalf("signature=%s", got)
	}
}

func TestSendProductWebhook(t *testing.T) {
	var got *http.Request
	var body []byte
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		body, _ = io.ReadAll(r.Body)
		if status == http.StatusFound {
			http.Redirect(w, r, "/elsewhere", http.StatusFound)
			return
		}
		w.WriteHeader(status)
	}))
	defer srv.Close()
	payload := []byte(`{"custom_id":"c","status":"active"}`)
	if err := sendProductWebhook(srv.URL+"/hook", "shh", "evt_1", "payment.active", "2026-01-01T00:00:00Z", payload); err != nil {
		t.Fatal(err)
	}
	for header, want := range map[string]string{
		"Content-Type":     "application/json",
		"X-OPH-Signature":  GenerateSignature(payload, "shh"),
		"X-OPH-Event-ID":   "evt_1",
		"X-OPH-Event-Type": "payment.active",
		"X-OPH-Timestamp":  "2026-01-01T00:00:00Z",
	} {
		if got.Header.Get(header) != want {
			t.Errorf("%s=%q want %q", header, got.Header.Get(header), want)
		}
	}
	if string(body) != string(payload) || got.Method != http.MethodPost {
		t.Fatalf("body=%s method=%s", body, got.Method)
	}
	for _, s := range []int{http.StatusFound, http.StatusInternalServerError, http.StatusNotFound} {
		status = s
		if err := sendProductWebhook(srv.URL+"/hook", "shh", "evt_1", "payment.active", "", payload); err == nil {
			t.Errorf("HTTP %d treated as delivered", s)
		}
	}
	if err := sendProductWebhook("://bad", "shh", "evt", "t", "", payload); err == nil {
		t.Error("bad URL accepted")
	}
}

func TestOutboxSkipsProductWebhooksForStripeAndSquare(t *testing.T) {
	setupPayments(t)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	defer srv.Close()
	testenv.Exec(t, "UPDATE products SET webhook_url=?, webhook_secret='secret-1234'", srv.URL)
	for _, gateway := range gateways {
		fulfilled(t, gateway, "onetime")
	}
	if err := deliverPaymentEffects(); err != nil {
		t.Fatal(err)
	}
	// The experimental product API only covers PayPal and Razorpay.
	if calls != 2 {
		t.Fatalf("product webhooks sent=%d, want 2", calls)
	}
	if n := testenv.Scalar(t, "SELECT COUNT(*) FROM payment_outbox WHERE webhook_done=1 AND mailing_done=1"); n != 4 {
		t.Fatalf("done rows=%d", n)
	}
	// Nothing left to deliver.
	if err := deliverPaymentEffects(); err != nil || calls != 2 {
		t.Fatalf("redelivered: calls=%d err=%v", calls, err)
	}
}

func TestOutboxWebhookFailureIsRetried(t *testing.T) {
	setupPayments(t)
	fail := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			http.Redirect(w, r, "https://elsewhere.test/", http.StatusFound)
		}
	}))
	defer srv.Close()
	testenv.Exec(t, "UPDATE products SET webhook_url=?, webhook_secret='secret-1234'", srv.URL)
	fulfilled(t, "paypal", "onetime")
	if err := deliverPaymentEffects(); err == nil {
		t.Fatal("302 reported as delivered")
	}
	if n := testenv.Scalar(t, "SELECT COUNT(*) FROM payment_outbox WHERE webhook_done=0 AND lease_until=0"); n != 1 {
		t.Fatalf("failed row not released for retry: %d", n)
	}
	fail = false
	elapseOutboxBackoff(t)
	if err := deliverPaymentEffects(); err != nil {
		t.Fatal(err)
	}
	if n := testenv.Scalar(t, "SELECT COUNT(*) FROM payment_outbox WHERE webhook_done=1"); n != 1 {
		t.Fatal("retry not delivered")
	}
}

// fakeListmonk is an in-memory Listmonk subscribers API.
type fakeListmonk struct {
	mu          sync.Mutex
	subscribers map[string]*struct {
		ID    int
		Lists map[int]bool
	}
	preconfirm []bool
	auth       []string
}

func newFakeListmonk(t *testing.T) (*fakeListmonk, *httptest.Server) {
	f := &fakeListmonk{subscribers: map[string]*struct {
		ID    int
		Lists map[int]bool
	}{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.auth = append(f.auth, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		render := func(email string) map[string]interface{} {
			s := f.subscribers[email]
			var lists []map[string]int
			for id := range s.Lists {
				lists = append(lists, map[string]int{"id": id})
			}
			return map[string]interface{}{"id": s.ID, "email": email, "lists": lists}
		}
		var body struct {
			Email      string `json:"email"`
			Lists      []int  `json:"lists"`
			Preconfirm bool   `json:"preconfirm_subscriptions"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/subscribers":
			q := r.URL.Query().Get("query")
			results := []interface{}{}
			for email := range f.subscribers {
				if q == "subscribers.email = '"+strings.ReplaceAll(email, "'", "''")+"'" {
					results = append(results, render(email))
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": map[string]interface{}{"results": results}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/subscribers":
			s := &struct {
				ID    int
				Lists map[int]bool
			}{ID: len(f.subscribers) + 1, Lists: map[int]bool{}}
			for _, l := range body.Lists {
				s.Lists[l] = true
			}
			f.subscribers[body.Email] = s
			f.preconfirm = append(f.preconfirm, body.Preconfirm)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": render(body.Email)})
		case r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, "/api/subscribers/"):
			for email, s := range f.subscribers {
				if r.URL.Path == fmt.Sprintf("/api/subscribers/%d", s.ID) {
					s.Lists = map[int]bool{}
					for _, l := range body.Lists {
						s.Lists[l] = true
					}
					_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": render(email)})
					return
				}
			}
			w.WriteHeader(http.StatusNotFound)
		default:
			t.Errorf("unexpected listmonk request %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return f, srv
}

func (f *fakeListmonk) onList(email string, list int) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	s := f.subscribers[email]
	return s != nil && s.Lists[list]
}

func TestMailingSyncFollowsEntitlement(t *testing.T) {
	setupPayments(t)
	lm, srv := newFakeListmonk(t)
	testenv.Set("listmonk_URL", srv.URL)
	testenv.Set("listmonk_API_token", "api:secret")
	testenv.Exec(t, "UPDATE products SET listmonk_list_id=7")

	first, f1 := fixtureAttempt(t, "razorpay", "onetime")
	f1.Email = "buyer@example.test"
	if _, err := verifyAndFulfill(first, f1); err != nil {
		t.Fatal(err)
	}
	if err := deliverPaymentEffects(); err != nil {
		t.Fatal(err)
	}
	if !lm.onList("buyer@example.test", 7) {
		t.Fatal("buyer not subscribed")
	}
	// A second purchase by the same buyer, then a refund of the first.
	second, f2 := fixtureAttempt(t, "razorpay", "onetime")
	f2.Email = "buyer@example.test"
	if _, err := verifyAndFulfill(second, f2); err != nil {
		t.Fatal(err)
	}
	if err := applyRefund("razorpay", f1.PaymentId, "r1", 1050, "USD"); err != nil {
		t.Fatal(err)
	}
	if err := deliverPaymentEffects(); err != nil {
		t.Fatal(err)
	}
	if !lm.onList("buyer@example.test", 7) {
		t.Fatal("refund of one purchase unsubscribed a buyer who still owns another")
	}
	// Refunding the remaining purchase removes the buyer from the list.
	if err := applyRefund("razorpay", f2.PaymentId, "r2", 1050, "USD"); err != nil {
		t.Fatal(err)
	}
	if err := deliverPaymentEffects(); err != nil {
		t.Fatal(err)
	}
	if lm.onList("buyer@example.test", 7) {
		t.Fatal("fully refunded buyer still subscribed")
	}
	for _, a := range lm.auth {
		if a != "token api:secret" {
			t.Fatalf("authorization header %q", a)
		}
	}
	if len(lm.preconfirm) != 1 || !lm.preconfirm[0] {
		t.Fatalf("preconfirm flags %v", lm.preconfirm)
	}
}

func TestMailchimpRequestShape(t *testing.T) {
	setupPayments(t)
	var req *http.Request
	var body map[string]string
	mockPayments(t, func(r *http.Request) string {
		req = r
		_ = json.NewDecoder(r.Body).Decode(&body)
		return "{}"
	})
	testenv.Set("mailchimp_token", "abc123-us6")
	p := &products.Story{MailchimpAudienceID: "aud 1"}
	if err := syncPaymentMailingLists(p, "Buyer@Example.test", "active"); err != nil {
		t.Fatal(err)
	}
	sum := md5.Sum([]byte("buyer@example.test"))
	wantPath := "/3.0/lists/aud%201/members/" + hex.EncodeToString(sum[:])
	if req.Method != http.MethodPut || req.URL.Host != "us6.api.mailchimp.com" || req.URL.EscapedPath() != wantPath {
		t.Fatalf("request %s %s%s want path %s", req.Method, req.URL.Host, req.URL.EscapedPath(), wantPath)
	}
	if user, pass, ok := req.BasicAuth(); !ok || user != "oph" || pass != "abc123-us6" {
		t.Fatal("basic auth")
	}
	if body["email_address"] != "Buyer@Example.test" || body["status"] != "subscribed" || body["status_if_new"] != "subscribed" {
		t.Fatalf("body=%v", body)
	}
	if err := syncPaymentMailingLists(p, "buyer@example.test", "refunded"); err != nil || body["status"] != "unsubscribed" {
		t.Fatalf("unsubscribe: %v %v", body, err)
	}
	req = nil
	if err := syncPaymentMailingLists(p, "", "active"); err != nil || req != nil {
		t.Fatal("empty email contacted Mailchimp")
	}
	if err := syncPaymentMailingLists(p, "b@example.test", "partially_refunded"); err != nil || req != nil {
		t.Fatal("partial refund changed the mailing list")
	}
	for _, token := range []string{"abc123-eu1", "abc123", "abc-us6x", "abc-evil.com/us6"} {
		testenv.Set("mailchimp_token", token)
		if err := syncPaymentMailingLists(p, "b@example.test", "active"); err == nil {
			t.Errorf("token %q accepted", token)
		}
	}
}

func TestListmonkPreconfirmSetting(t *testing.T) {
	testenv.Config(t, nil)
	for value, want := range map[string]bool{"": true, "true": true, "YES": true, " 1 ": true, "on": true, "false": false, "no": false, "0": false, "maybe": false} {
		testenv.Set("listmonk_preconfirm_subscriptions", value)
		if got := listmonkPreconfirmSubscriptions(); got != want {
			t.Errorf("%q: %v want %v", value, got, want)
		}
	}
}

// elapseOutboxBackoff makes failed rows due again, as if the retry delay passed.
func elapseOutboxBackoff(t *testing.T) {
	t.Helper()
	testenv.Exec(t, "UPDATE payment_outbox SET webhook_next_attempt_at=0, mailing_next_attempt_at=0")
}

// deliveryRecorder is a paymentHTTPClient transport that records requests
// from concurrent delivery workers and answers each with respond.
type deliveryRecorder struct {
	mu       sync.Mutex
	requests []*http.Request
	respond  func(*http.Request) (int, error)
}

func recordDeliveries(t *testing.T, respond func(*http.Request) (int, error)) *deliveryRecorder {
	t.Helper()
	d := &deliveryRecorder{respond: respond}
	old := paymentHTTPClient
	t.Cleanup(func() { paymentHTTPClient = old })
	paymentHTTPClient = &http.Client{Transport: paymentRoundTripper(func(r *http.Request) (*http.Response, error) {
		d.mu.Lock()
		d.requests = append(d.requests, r)
		respond := d.respond
		d.mu.Unlock()
		status, err := respond(r)
		if err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}, nil
	})}
	return d
}

func (d *deliveryRecorder) setRespond(respond func(*http.Request) (int, error)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.respond = respond
}

// events returns the X-OPH-Event-ID of each request sent to host, in order.
func (d *deliveryRecorder) events(host string) []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	var ids []string
	for _, r := range d.requests {
		if r.URL.Host == host {
			ids = append(ids, r.Header.Get("X-OPH-Event-ID"))
		}
	}
	return ids
}

func seedSecondProduct(t *testing.T) {
	t.Helper()
	testenv.Exec(t, `INSERT INTO products(id,name,schedule,total_onetime_payments,total_subscribers) VALUES(2,'Other merchant','onetime',0,0)`)
}

func outboxRowIDs(t *testing.T, attemptID string) []string {
	t.Helper()
	rows, err := attemptQuery().Select("SELECT id FROM payment_outbox").Where("attempt_id=?", attemptID).Order("created_at,id").Results()
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, r := range rows {
		ids = append(ids, r["id"].(string))
	}
	return ids
}

func TestOutboxDeadEndpointDoesNotBlockOtherProducts(t *testing.T) {
	setupPayments(t)
	seedSecondProduct(t)
	testenv.Exec(t, `UPDATE products SET webhook_url='https://dead.test/hook?token=url-secret',webhook_secret='test-secret' WHERE id=1`)
	testenv.Exec(t, `UPDATE products SET webhook_url='https://live.test/hook',webhook_secret='test-secret' WHERE id=2`)
	older, olderFacts := fixtureProductAttempt(t, 1, "paypal", "onetime")
	if _, err := verifyAndFulfill(older, olderFacts); err != nil {
		t.Fatal(err)
	}
	newer, newerFacts := fixtureProductAttempt(t, 2, "razorpay", "onetime")
	if _, err := verifyAndFulfill(newer, newerFacts); err != nil {
		t.Fatal(err)
	}
	deliveries := recordDeliveries(t, func(r *http.Request) (int, error) {
		if r.URL.Host == "dead.test" {
			return 0, errors.New("connection refused")
		}
		return http.StatusOK, nil
	})
	olderEvent := outboxRowIDs(t, older.Id)[0]
	err := deliverPaymentEffects()
	if err == nil {
		t.Fatal("dead endpoint reported as delivered")
	}
	for _, want := range []string{"webhook delivery", "product 1", olderEvent, EventPaymentActive, "dead.test"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "url-secret") {
		t.Errorf("error leaks the webhook URL query: %v", err)
	}
	if got := deliveries.events("live.test"); len(got) != 1 || got[0] != outboxRowIDs(t, newer.Id)[0] {
		t.Fatalf("other product's webhook blocked: %v", got)
	}
	if n := testenv.Scalar(t, "SELECT webhook_done FROM payment_outbox WHERE attempt_id=?", newer.Id); n != 1 {
		t.Fatalf("other product webhook_done=%d", n)
	}
	if n := testenv.Scalar(t, "SELECT COUNT(*) FROM payment_outbox WHERE attempt_id=? AND webhook_done=0 AND webhook_attempts=1 AND webhook_next_attempt_at>?", older.Id, time.Now().Unix()); n != 1 {
		t.Fatal("failed row not kept pending with a backoff")
	}
	lastError := testenv.Text(t, "SELECT webhook_last_error FROM payment_outbox WHERE attempt_id=?", older.Id)
	if !strings.Contains(lastError, "dead.test") || strings.Contains(lastError, "url-secret") || strings.Contains(lastError, "/hook") {
		t.Fatalf("last error %q", lastError)
	}
}

func TestOutboxKeepsOrderWithinFailingProduct(t *testing.T) {
	setupPayments(t)
	testenv.Exec(t, `UPDATE products SET webhook_url='https://client.test/hook',webhook_secret='test-secret' WHERE id=1`)
	var want []string
	for i := 0; i < 2; i++ {
		a, _ := fulfilled(t, "paypal", "onetime")
		want = append(want, outboxRowIDs(t, a.Id)...)
	}
	deliveries := recordDeliveries(t, func(*http.Request) (int, error) { return http.StatusServiceUnavailable, nil })
	if err := deliverPaymentEffects(); err == nil {
		t.Fatal("503 reported as delivered")
	}
	if got := deliveries.events("client.test"); len(got) != 1 || got[0] != want[0] {
		t.Fatalf("later event passed its failed predecessor: %v", got)
	}
	// While backing off, the head row holds its product's partition.
	if err := deliverPaymentEffects(); err != nil {
		t.Fatal(err)
	}
	if got := deliveries.events("client.test"); len(got) != 1 {
		t.Fatalf("retried before the backoff elapsed: %v", got)
	}
	deliveries.setRespond(func(*http.Request) (int, error) { return http.StatusOK, nil })
	elapseOutboxBackoff(t)
	if err := deliverPaymentEffects(); err != nil {
		t.Fatal(err)
	}
	if got := deliveries.events("client.test"); strings.Join(got, ",") != strings.Join([]string{want[0], want[0], want[1]}, ",") {
		t.Fatalf("delivery order %v, want %v then %v", got, want[0], want)
	}
}

func TestOutboxMailingFailureDoesNotBlockOtherProducts(t *testing.T) {
	setupPayments(t)
	seedSecondProduct(t)
	testenv.Set("mailchimp_token", "test-us1")
	testenv.Exec(t, `UPDATE products SET mailchimp_audience_id='missing-list' WHERE id=1`)
	testenv.Exec(t, `UPDATE products SET mailchimp_audience_id='live-list' WHERE id=2`)
	older, olderFacts := fixtureProductAttempt(t, 1, "stripe", "onetime")
	olderFacts.Email = "older@example.com"
	if _, err := verifyAndFulfill(older, olderFacts); err != nil {
		t.Fatal(err)
	}
	newer, newerFacts := fixtureProductAttempt(t, 2, "square", "onetime")
	newerFacts.Email = "newer@example.com"
	if _, err := verifyAndFulfill(newer, newerFacts); err != nil {
		t.Fatal(err)
	}
	deliveries := recordDeliveries(t, func(r *http.Request) (int, error) {
		if strings.Contains(r.URL.Path, "/lists/missing-list/") {
			return http.StatusNotFound, nil
		}
		return http.StatusOK, nil
	})
	err := deliverPaymentEffects()
	if err == nil || !strings.Contains(err.Error(), "mailing delivery") || !strings.Contains(err.Error(), "product 1") {
		t.Fatalf("expected labelled product 1 mailing failure, got %v", err)
	}
	if n := len(deliveries.events("us1.api.mailchimp.com")); n != 2 {
		t.Fatalf("mailchimp requests=%d", n)
	}
	if n := testenv.Scalar(t, "SELECT mailing_done FROM payment_outbox WHERE attempt_id=?", newer.Id); n != 1 {
		t.Fatalf("other product mailing_done=%d", n)
	}
	if n := testenv.Scalar(t, "SELECT COUNT(*) FROM payment_outbox WHERE attempt_id=? AND mailing_done=0 AND mailing_attempts=1", older.Id); n != 1 {
		t.Fatal("failed mailing not kept pending")
	}
}

func TestOutboxBacklogDoesNotStarveOtherProducts(t *testing.T) {
	setupPayments(t)
	seedSecondProduct(t)
	testenv.Exec(t, `UPDATE products SET webhook_url='https://dead.test/hook',webhook_secret='test-secret' WHERE id=1`)
	testenv.Exec(t, `UPDATE products SET webhook_url='https://live.test/hook',webhook_secret='test-secret' WHERE id=2`)
	stuck, _ := fulfilled(t, "paypal", "onetime")
	for i := 0; i < outboxBatch+20; i++ {
		testenv.Exec(t, `INSERT INTO payment_outbox(id,attempt_id,event_type,created_at,payload)
			SELECT ?,attempt_id,event_type,created_at,payload FROM payment_outbox WHERE attempt_id=? LIMIT 1`, fmt.Sprintf("backlog-%03d", i), stuck.Id)
	}
	other, otherFacts := fixtureProductAttempt(t, 2, "razorpay", "onetime")
	if _, err := verifyAndFulfill(other, otherFacts); err != nil {
		t.Fatal(err)
	}
	deliveries := recordDeliveries(t, func(r *http.Request) (int, error) {
		if r.URL.Host == "dead.test" {
			return http.StatusServiceUnavailable, nil
		}
		return http.StatusOK, nil
	})
	if err := deliverPaymentEffects(); err == nil {
		t.Fatal("failing product reported as delivered")
	}
	if n := len(deliveries.events("dead.test")); n != 1 {
		t.Fatalf("failing product attempts=%d", n)
	}
	if n := testenv.Scalar(t, "SELECT webhook_done FROM payment_outbox WHERE attempt_id=?", other.Id); n != 1 {
		t.Fatal("backlog of another product starved this product")
	}
}

func TestOutboxGivesUpAfterThreeDaysAndCanReplay(t *testing.T) {
	setupPayments(t)
	testenv.Exec(t, `UPDATE products SET webhook_url='https://client.test/hook',webhook_secret='test-secret' WHERE id=1`)
	old, _ := fulfilled(t, "paypal", "onetime")
	recent, _ := fulfilled(t, "paypal", "onetime")
	oldEvent := outboxRowIDs(t, old.Id)[0]
	recentEvent := outboxRowIDs(t, recent.Id)[0]
	testenv.Exec(t, "UPDATE payment_outbox SET created_at=? WHERE id=?", time.Now().Add(-outboxGiveUpAge-time.Hour).UTC().Format(time.RFC3339Nano), oldEvent)
	deliveries := recordDeliveries(t, func(r *http.Request) (int, error) {
		if r.Header.Get("X-OPH-Event-ID") == oldEvent {
			return http.StatusServiceUnavailable, nil
		}
		return http.StatusOK, nil
	})
	// An old row is still attempted once; its failure abandons it and the
	// partition moves on.
	if err := deliverPaymentEffects(); err != nil {
		t.Fatal(err)
	}
	if got := deliveries.events("client.test"); strings.Join(got, ",") != oldEvent+","+recentEvent {
		t.Fatalf("deliveries %v", got)
	}
	if n := testenv.Scalar(t, "SELECT COUNT(*) FROM payment_outbox WHERE id=? AND webhook_done=? AND webhook_attempts=1 AND webhook_last_error<>''", oldEvent, outboxFailed); n != 1 {
		t.Fatal("expired row not marked failed")
	}
	if n := testenv.Scalar(t, "SELECT webhook_done FROM payment_outbox WHERE id=?", recentEvent); n != 1 {
		t.Fatal("partition stalled behind an abandoned row")
	}
	// Abandoned rows are not retried until an operator replays them.
	if err := deliverPaymentEffects(); err != nil || len(deliveries.events("client.test")) != 2 {
		t.Fatalf("abandoned row retried: %v", err)
	}
	deliveries.setRespond(func(*http.Request) (int, error) { return http.StatusOK, nil })
	testenv.Exec(t, "UPDATE payment_outbox SET webhook_done=0, webhook_attempts=0, webhook_next_attempt_at=0 WHERE id=?", oldEvent)
	if err := deliverPaymentEffects(); err != nil {
		t.Fatal(err)
	}
	if n := testenv.Scalar(t, "SELECT webhook_done FROM payment_outbox WHERE id=?", oldEvent); n != 1 {
		t.Fatal("replayed row not delivered")
	}
}

func TestOutboxRetryDelay(t *testing.T) {
	for attempts, want := range map[int64]time.Duration{1: 30 * time.Second, 2: time.Minute, 3: 2 * time.Minute, 7: 32 * time.Minute, 8: time.Hour, 1000: time.Hour} {
		if got := outboxRetryDelay(attempts); got != want {
			t.Errorf("attempts %d: %v want %v", attempts, got, want)
		}
	}
}
