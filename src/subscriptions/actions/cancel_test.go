package actions

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/abishekmuthian/open-payment-host/src/internal/testenv"
	"github.com/abishekmuthian/open-payment-host/src/subscriptions"
)

// issueLink stores a legacy Stripe subscription and has an admin issue a
// cancellation link for it, returning the link's query.
func issueLink(t *testing.T) url.Values {
	t.Helper()
	testenv.Setup(t, map[string]string{"stripe_secret": "sk_test"})
	testenv.Route(http.MethodGet, "/subscriptions/cancel", HandlePaymentCancel)
	testenv.Route(http.MethodPost, "/subscriptions/cancel", HandlePaymentCancel)
	testenv.Route(http.MethodPost, "/subscriptions/cancellation-link", subscriptions.HandleCancellationLink)
	admin := testenv.SeedUser(t, testenv.RoleAdmin, "admin@merchant.test", "admin-password")
	testenv.SeedProduct(t, map[string]interface{}{"schedule": "monthly", "allowed_redirect_origins": "https://client.test"})
	testenv.Exec(t, `INSERT INTO subscriptions(pg,subscr_id,item_number,payment_status,user_id) VALUES('stripe','sub_old',1,'ACTIVE','cust&1')`)
	w := testenv.Serve(t, testenv.AuthedPOST(t, "/subscriptions/cancellation-link", url.Values{"subscription_id": {"sub_old"}}, admin))
	u, err := url.Parse(w.Body.String())
	if w.Code != http.StatusOK || err != nil {
		t.Fatalf("link: %d %s", w.Code, w.Body.String())
	}
	return u.Query()
}

func TestHandlePaymentCancel(t *testing.T) {
	q := issueLink(t)
	calls := 0
	testenv.StripeBackend(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/subscriptions/sub_old" {
			t.Errorf("unexpected Stripe request %s %s", r.Method, r.URL)
		}
		_ = r.ParseForm()
		if r.Form.Get("cancel_at_period_end") != "true" {
			t.Errorf("cancellation params %v", r.Form)
		}
		calls++
		return testenv.Response(http.StatusOK, `{"id":"sub_old","object":"subscription","cancel_at_period_end":true}`), nil
	})
	page := func(v url.Values) *httptest.ResponseRecorder {
		return testenv.Serve(t, httptest.NewRequest(http.MethodGet, "/subscriptions/cancel?"+v.Encode(), nil))
	}
	with := func(key, value string) url.Values {
		v := url.Values{}
		for k := range q {
			v.Set(k, q.Get(k))
		}
		v.Set(key, value)
		return v
	}
	if w := page(with("cancellation_token", "")); w.Code != http.StatusUnauthorized {
		t.Fatalf("missing token status=%d", w.Code)
	}
	if w := page(with("cancellation_token", strings.Repeat("0", 64))); w.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token status=%d", w.Code)
	}
	if w := page(with("subscription_id", "sub_other")); w.Code != http.StatusUnauthorized {
		t.Fatalf("other subscription status=%d", w.Code)
	}
	if w := page(with("redirect_uri", "https://evil.test/")); w.Code != http.StatusBadRequest {
		t.Fatalf("unlisted redirect status=%d", w.Code)
	}
	w := page(q)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "sub_old") || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("confirm page status=%d", w.Code)
	}
	if calls != 0 {
		t.Fatal("GET cancelled the subscription")
	}
	// POST without a CSRF token does not consume the capability.
	if w := testenv.Serve(t, testenv.POST("/subscriptions/cancel", q)); w.Code != http.StatusUnauthorized || calls != 0 {
		t.Fatalf("tokenless POST: %d calls=%d", w.Code, calls)
	}
	form := with("redirect_uri", "https://client.test/after")
	w = testenv.Serve(t, testenv.AuthedPOST(t, "/subscriptions/cancel", form, 0))
	if w.Code != http.StatusFound || w.Header().Get("Location") != "https://client.test/after?custom_id=cust%261&subscription_id=sub_old" {
		t.Fatalf("cancel: %d %s", w.Code, w.Header().Get("Location"))
	}
	if calls != 1 {
		t.Fatalf("provider calls=%d", calls)
	}
	// The capability is single use.
	if w := testenv.Serve(t, testenv.AuthedPOST(t, "/subscriptions/cancel", with("redirect_uri", ""), 0)); w.Code != http.StatusUnauthorized || calls != 1 {
		t.Fatalf("reused link: %d calls=%d", w.Code, calls)
	}
}

func TestHandlePaymentCancelWithoutRedirectRendersPage(t *testing.T) {
	q := issueLink(t)
	testenv.StripeBackend(t, func(r *http.Request) (*http.Response, error) {
		return testenv.Response(http.StatusOK, `{"id":"sub_old","object":"subscription"}`), nil
	})
	w := testenv.Serve(t, testenv.AuthedPOST(t, "/subscriptions/cancel", q, 0))
	if w.Code != http.StatusOK || w.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("status=%d", w.Code)
	}
}
