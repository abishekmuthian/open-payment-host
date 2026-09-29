package subscriptions

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abishekmuthian/open-payment-host/src/internal/testenv"
	"github.com/abishekmuthian/open-payment-host/src/lib/query"
)

var gateways = []string{"stripe", "square", "paypal", "razorpay"}

// fulfilled creates and fulfils an attempt, returning it with its facts.
func fulfilled(t *testing.T, gateway, schedule string) (*PaymentAttempt, ProviderFacts) {
	t.Helper()
	a, f := fixtureAttempt(t, gateway, schedule)
	if _, err := verifyAndFulfill(a, f); err != nil {
		t.Fatal(err)
	}
	return a, f
}

func outboxEvents(t *testing.T, attemptID string) []string {
	t.Helper()
	rows, err := attemptQuery().Select("SELECT event_type FROM payment_outbox").Where("attempt_id=?", attemptID).Order("created_at,id").Results()
	if err != nil {
		t.Fatal(err)
	}
	var events []string
	for _, r := range rows {
		events = append(events, r["event_type"].(string))
	}
	return events
}

func attemptState(t *testing.T, id string) (status string, counted int64) {
	t.Helper()
	a, err := FindAttempt(id)
	if err != nil {
		t.Fatal(err)
	}
	return a.Status, testenv.Scalar(t, "SELECT counted FROM payment_attempts WHERE id=?", id)
}

func subscribers(t *testing.T) int64 {
	return testenv.Scalar(t, "SELECT total_subscribers FROM products WHERE id=1")
}

func TestApplySubscriptionStatusAllGateways(t *testing.T) {
	for _, gateway := range gateways {
		for _, status := range []string{"cancelled", "expired", "suspended", "paused", "past_due", "halted", "pending"} {
			t.Run(gateway+"/"+status, func(t *testing.T) {
				setupPayments(t)
				a, _ := fulfilled(t, gateway, "monthly")
				if subscribers(t) != 1 {
					t.Fatal("not counted")
				}
				// ACTIVE is never proof of payment and changes nothing.
				if err := applySubscriptionStatus(gateway, a.ProviderSubscriptionId, "active"); err != nil {
					t.Fatal(err)
				}
				if s, c := attemptState(t, a.Id); s != "completed" || c != 1 {
					t.Fatalf("active changed state: %s %d", s, c)
				}
				for i := 0; i < 2; i++ { // the second delivery is a no-op
					if err := applySubscriptionStatus(gateway, a.ProviderSubscriptionId, status); err != nil {
						t.Fatal(err)
					}
				}
				if s, c := attemptState(t, a.Id); s != status || c != 0 {
					t.Fatalf("state=%s counted=%d", s, c)
				}
				if subscribers(t) != 0 {
					t.Fatalf("subscribers=%d, want one decrement", subscribers(t))
				}
				if got := testenv.Text(t, "SELECT payment_status FROM subscriptions WHERE subscr_id=?", a.ProviderSubscriptionId); got != strings.ToUpper(status) {
					t.Fatalf("stored status=%s", got)
				}
				events := outboxEvents(t, a.Id)
				if len(events) != 2 || events[0] != EventSubscriptionActive || events[1] != "subscription."+status {
					t.Fatalf("events=%v", events)
				}
				cancelledAt := testenv.Text(t, "SELECT cancelled_at FROM payment_attempts WHERE id=?", a.Id)
				if (cancelledAt != "") != (status == "cancelled") {
					t.Fatalf("cancelled_at=%q for %s", cancelledAt, status)
				}
			})
		}
	}
}

func TestTerminalStatusesAreSticky(t *testing.T) {
	setupPayments(t)
	for _, terminal := range []string{"cancelled", "expired"} {
		a, _ := fulfilled(t, "paypal", "monthly")
		if err := applySubscriptionStatus("paypal", a.ProviderSubscriptionId, terminal); err != nil {
			t.Fatal(err)
		}
		for _, later := range []string{"suspended", "paused", "cancelled", "expired"} {
			if err := applySubscriptionStatus("paypal", a.ProviderSubscriptionId, later); err != nil {
				t.Fatal(err)
			}
		}
		if s, _ := attemptState(t, a.Id); s != terminal {
			t.Fatalf("%s overwritten by %s", terminal, s)
		}
		if n := len(outboxEvents(t, a.Id)); n != 2 {
			t.Fatalf("%s: %d events", terminal, n)
		}
		// A cancelled or expired attempt never accepts a new payment.
		f := ProviderFacts{Gateway: "paypal", SubscriptionId: a.ProviderSubscriptionId, PaymentId: "late_" + a.Id, Amount: a.Amount, Currency: a.Currency, Paid: true}
		if _, err := verifyAndFulfill(a, f); err == nil {
			t.Fatalf("%s attempt accepted a payment", terminal)
		}
	}
	if subscribers(t) != 0 {
		t.Fatalf("subscribers=%d", subscribers(t))
	}
}

func TestSuspendedSubscriptionReactivatesOnRenewal(t *testing.T) {
	setupPayments(t)
	a, f := fulfilled(t, "stripe", "monthly")
	if err := applySubscriptionStatus("stripe", a.ProviderSubscriptionId, "past_due"); err != nil {
		t.Fatal(err)
	}
	f.PaymentId = "renewal_" + a.Id
	if _, err := verifyAndFulfill(a, f); err != nil {
		t.Fatal(err)
	}
	if s, c := attemptState(t, a.Id); s != "completed" || c != 1 || subscribers(t) != 1 {
		t.Fatalf("state=%s counted=%d subscribers=%d", s, c, subscribers(t))
	}
	events := outboxEvents(t, a.Id)
	if len(events) != 3 || events[2] != EventSubscriptionActive {
		t.Fatalf("events=%v", events)
	}
}

// suspensions maps each gateway to a provider state that stops counting a
// subscriber without ending it.
var suspensions = map[string]string{"stripe": "past_due", "square": "paused", "paypal": "suspended", "razorpay": "halted"}

func TestTerminalStatusAfterSuspensionIsReported(t *testing.T) {
	for _, gateway := range gateways {
		for _, terminal := range []string{"cancelled", "expired"} {
			t.Run(gateway+"/"+terminal, func(t *testing.T) {
				setupPayments(t)
				a, _ := fulfilled(t, gateway, "monthly")
				suspension := suspensions[gateway]
				for _, status := range []string{suspension, suspension, terminal, terminal} {
					if err := applySubscriptionStatus(gateway, a.ProviderSubscriptionId, status); err != nil {
						t.Fatal(err)
					}
				}
				events := outboxEvents(t, a.Id)
				if len(events) != 3 || events[0] != EventSubscriptionActive || events[1] != "subscription."+suspension || events[2] != "subscription."+terminal {
					t.Fatalf("events=%v", events)
				}
				if s, c := attemptState(t, a.Id); s != terminal || c != 0 {
					t.Fatalf("state=%s counted=%d", s, c)
				}
				if subscribers(t) != 0 {
					t.Fatalf("subscribers=%d, want one decrement", subscribers(t))
				}
				cancelledAt := testenv.Text(t, "SELECT cancelled_at FROM payment_attempts WHERE id=?", a.Id)
				if (cancelledAt != "") != (terminal == "cancelled") {
					t.Fatalf("cancelled_at=%q for %s", cancelledAt, terminal)
				}
			})
		}
	}
}

func TestUnpaidSubscriptionStatusSendsNoEvent(t *testing.T) {
	setupPayments(t)
	a, _ := fixtureAttempt(t, "paypal", "monthly")
	for _, status := range []string{"suspended", "cancelled"} {
		if err := applySubscriptionStatus("paypal", a.ProviderSubscriptionId, status); err != nil {
			t.Fatal(err)
		}
	}
	if s, _ := attemptState(t, a.Id); s != "cancelled" {
		t.Fatalf("state=%s", s)
	}
	if events := outboxEvents(t, a.Id); len(events) != 0 {
		t.Fatalf("unpaid attempt produced events=%v", events)
	}
	if subscribers(t) != 0 {
		t.Fatalf("subscribers=%d", subscribers(t))
	}
}

func TestCancelledAfterSuspensionDeliversProductWebhook(t *testing.T) {
	setupPayments(t)
	var received []map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if r.Header.Get("X-OPH-Signature") != GenerateSignature(b, "test-secret") {
			t.Error("invalid webhook signature")
		}
		var payload map[string]string
		if err := json.Unmarshal(b, &payload); err != nil {
			t.Error(err)
		}
		payload["event_type"] = r.Header.Get("X-OPH-Event-Type")
		received = append(received, payload)
	}))
	defer srv.Close()
	testenv.Exec(t, `UPDATE products SET webhook_url=?,webhook_secret='test-secret' WHERE id=1`, srv.URL)
	a, _ := fulfilled(t, "paypal", "monthly")
	for _, status := range []string{"suspended", "cancelled"} {
		if err := applySubscriptionStatus("paypal", a.ProviderSubscriptionId, status); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := query.New("payment_outbox", "id").Where("attempt_id=?", a.Id).Order("created_at,id").Results()
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if err := deliverPaymentWebhook(row["id"].(string)); err != nil {
			t.Fatal(err)
		}
	}
	if len(received) != 3 {
		t.Fatalf("deliveries=%v", received)
	}
	last := received[2]
	if last["event_type"] != "subscription.cancelled" || last["status"] != "cancelled" || last["subscription_id"] != a.ProviderSubscriptionId || last["custom_id"] != "customer&1" {
		t.Fatalf("cancelled webhook=%v", last)
	}
}

func TestApplySubscriptionStatusInputs(t *testing.T) {
	setupPayments(t)
	if err := applySubscriptionStatus("stripe", "", "cancelled"); err == nil {
		t.Fatal("empty subscription id accepted")
	}
	// Unknown subscription with no stored legacy row.
	if err := applySubscriptionStatus("stripe", "sub_unknown", "cancelled"); err == nil {
		t.Fatal("unknown subscription accepted")
	}
	if err := applyLegacyStatus("stripe", "sub_unknown", "active"); err != nil {
		t.Fatalf("legacy active: %v", err)
	}
	if n := testenv.Scalar(t, "SELECT COUNT(*) FROM payment_attempts"); n != 0 {
		t.Fatalf("attempts created for unknown subscription: %d", n)
	}
}

func TestLegacyStatusTransitions(t *testing.T) {
	setupPayments(t)
	testenv.Exec(t, `INSERT INTO subscriptions(pg,subscr_id,item_number,payment_status,user_id,payer_email) VALUES('stripe','sub_legacy',1,'ACTIVE','cust','buyer@example.test'); UPDATE products SET total_subscribers=1 WHERE id=1`)
	for _, status := range []string{"suspended", "suspended", "cancelled"} {
		if err := applySubscriptionStatus("stripe", "sub_legacy", status); err != nil {
			t.Fatal(err)
		}
	}
	if subscribers(t) != 0 {
		t.Fatalf("subscribers=%d", subscribers(t))
	}
	if got := testenv.Text(t, "SELECT payment_status FROM subscriptions WHERE subscr_id='sub_legacy'"); got != "CANCELLED" {
		t.Fatalf("stored=%s", got)
	}
	a, err := FindAttemptByProviderSubscription("stripe", "sub_legacy")
	if err != nil || a.Status != "legacy" || a.CustomId != "cust" {
		t.Fatalf("legacy attempt: %+v %v", a, err)
	}
	if events := outboxEvents(t, a.Id); len(events) != 2 || events[0] != "subscription.suspended" || events[1] != "subscription.cancelled" {
		t.Fatalf("events=%v", events)
	}
}

func TestApplyRefundOneTime(t *testing.T) {
	setupPayments(t)
	a, f := fulfilled(t, "razorpay", "onetime")
	onetime := func() int64 { return testenv.Scalar(t, "SELECT total_onetime_payments FROM products WHERE id=1") }
	if onetime() != 1 {
		t.Fatal("not counted")
	}
	for _, bad := range []struct {
		txn, refund string
		amount      int64
		currency    string
	}{
		{"", "r", 1, "USD"}, {f.PaymentId, "", 1, "USD"}, {f.PaymentId, "r", 0, "USD"}, {f.PaymentId, "r", -5, "USD"},
		{"unknown_txn", "r", 1, "USD"}, {f.PaymentId, "r_currency", 100, "EUR"}, {f.PaymentId, "r_over", 1051, "USD"},
	} {
		if err := applyRefund("razorpay", bad.txn, bad.refund, bad.amount, bad.currency); err == nil {
			t.Errorf("refund %+v accepted", bad)
		}
	}
	if err := applyRefund("stripe", f.PaymentId, "r_gateway", 100, "USD"); err == nil {
		t.Error("refund on another gateway's transaction accepted")
	}
	// Partial, then duplicate delivery, then the remainder.
	for i := 0; i < 2; i++ {
		if err := applyRefund("razorpay", f.PaymentId, "r1", 500, "usd"); err != nil {
			t.Fatal(err)
		}
	}
	if s, c := attemptState(t, a.Id); s != "completed" || c != 1 || onetime() != 1 {
		t.Fatalf("partial refund: %s %d %d", s, c, onetime())
	}
	if got := testenv.Text(t, "SELECT payment_status FROM subscriptions WHERE txn_id=?", f.PaymentId); got != "PARTIALLY_REFUNDED" {
		t.Fatalf("stored=%s", got)
	}
	if err := applyRefund("razorpay", f.PaymentId, "r2", 551, "USD"); err == nil {
		t.Fatal("over-refund of remainder accepted")
	}
	if err := applyRefund("razorpay", f.PaymentId, "r2", 550, "USD"); err != nil {
		t.Fatal(err)
	}
	if s, c := attemptState(t, a.Id); s != "refunded" || c != 0 || onetime() != 0 {
		t.Fatalf("full refund: %s %d %d", s, c, onetime())
	}
	if n := testenv.Scalar(t, "SELECT refunded FROM payment_transactions WHERE transaction_id=?", f.PaymentId); n != 1050 {
		t.Fatalf("refunded=%d", n)
	}
	events := outboxEvents(t, a.Id)
	if len(events) != 3 || events[1] != "payment.partially_refunded" || events[2] != "payment.refunded" {
		t.Fatalf("events=%v", events)
	}
	if err := applyRefund("razorpay", f.PaymentId, "r3", 1, "USD"); err == nil {
		t.Fatal("refund beyond total accepted")
	}
}

func TestApplyRefundRenewalVersusInitial(t *testing.T) {
	setupPayments(t)
	a, f := fulfilled(t, "paypal", "yearly")
	initial := f.PaymentId
	renewal := f
	renewal.PaymentId = "renewal_" + a.Id
	if _, err := verifyAndFulfill(a, renewal); err != nil {
		t.Fatal(err)
	}
	// Refunding a renewal marks only that transaction; the subscription stays counted.
	if err := applyRefund("paypal", renewal.PaymentId, "rr", 1050, "USD"); err != nil {
		t.Fatal(err)
	}
	if s, c := attemptState(t, a.Id); s != "completed" || c != 1 || subscribers(t) != 1 {
		t.Fatalf("renewal refund changed attempt: %s %d %d", s, c, subscribers(t))
	}
	if got := testenv.Text(t, "SELECT payment_status FROM subscriptions WHERE txn_id=?", initial); got != "ACTIVE" {
		t.Fatalf("initial transaction status=%s", got)
	}
	// Refunding the initial payment revokes the subscription.
	if err := applyRefund("paypal", initial, "ri", 1050, "USD"); err != nil {
		t.Fatal(err)
	}
	if s, c := attemptState(t, a.Id); s != "refunded" || c != 0 || subscribers(t) != 0 {
		t.Fatalf("initial refund: %s %d %d", s, c, subscribers(t))
	}
	if events := outboxEvents(t, a.Id); len(events) != 3 || events[1] != "subscription.refunded" || events[2] != "subscription.refunded" {
		t.Fatalf("events=%v", events)
	}
}
