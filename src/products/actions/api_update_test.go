package storyactions

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/abishekmuthian/open-payment-host/src/internal/testenv"
	"github.com/abishekmuthian/open-payment-host/src/products"
)

func TestUpdateAPIDisableAndReenable(t *testing.T) {
	f := setupHandlers(t, nil)
	id := seedPricedProduct(t, "onetime", f.admin)
	path := fmt.Sprintf("/products/%d/update", id)
	form := url.Values{"name": {"Handbook"}, "schedule": {"onetime"}}

	assertToggle := func(enabled bool) {
		t.Helper()
		r := httptest.NewRequest(http.MethodGet, path, nil)
		testenv.LoggedIn(t, r, f.admin)
		w := testenv.Serve(t, r)
		if w.Code != http.StatusOK {
			t.Fatalf("update form status=%d", w.Code)
		}
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(w.Body.String()))
		if err != nil {
			t.Fatal(err)
		}
		toggle := doc.Find("#api-toggle-update")
		if toggle.Length() != 1 {
			t.Fatal("API toggle missing")
		}
		script, _ := toggle.Attr("_")
		if !strings.Contains(script, fmt.Sprintf("set my.checked to %t", enabled)) {
			t.Fatalf("API toggle did not initialize enabled=%t: %s", enabled, script)
		}
	}

	assertToggle(true)
	// An unchecked API toggle removes both webhook fields from the form.
	w := testenv.Serve(t, testenv.AuthedPOST(t, path, form, f.admin))
	if w.Code != http.StatusFound {
		t.Fatalf("disable status=%d body=%s", w.Code, w.Body.String())
	}
	p, err := products.Find(id)
	if err != nil {
		t.Fatal(err)
	}
	if p.WebhookURL != "" || p.WebhookSecret != "" {
		t.Fatal("disabling API retained saved webhook settings")
	}
	assertToggle(false)

	form.Set("api-toggle", "on")
	form.Set("webhook_url", "https://client.test/new-hook")
	form.Set("webhook_secret", "replacement-secret")
	w = testenv.Serve(t, testenv.AuthedPOST(t, path, form, f.admin))
	if w.Code != http.StatusFound {
		t.Fatalf("enable status=%d body=%s", w.Code, w.Body.String())
	}
	p, err = products.Find(id)
	if err != nil {
		t.Fatal(err)
	}
	if p.WebhookURL != form.Get("webhook_url") || p.WebhookSecret != form.Get("webhook_secret") {
		t.Fatal("re-enabling API did not persist webhook settings")
	}
	assertToggle(true)
}

func TestUpdateAPIDisableRequiresAuthorisation(t *testing.T) {
	f := setupHandlers(t, nil)
	id := seedPricedProduct(t, "onetime", f.admin)
	path := fmt.Sprintf("/products/%d/update", id)
	form := url.Values{"name": {"Handbook"}, "schedule": {"onetime"}}

	for _, user := range []int64{0, f.reader} {
		w := testenv.Serve(t, testenv.AuthedPOST(t, path, form, user))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("user=%d status=%d", user, w.Code)
		}
	}
	if w := testenv.Serve(t, testenv.POST(path, form)); w.Code != http.StatusUnauthorized {
		t.Fatalf("tokenless status=%d", w.Code)
	}
	p, err := products.Find(id)
	if err != nil {
		t.Fatal(err)
	}
	if p.WebhookURL != "https://client.test/hook" || p.WebhookSecret != webhookSecret {
		t.Fatal("unauthorized update changed webhook settings")
	}

	// Readers can update their own products, but webhook fields are admin-only.
	id = seedPricedProduct(t, "onetime", f.reader)
	path = fmt.Sprintf("/products/%d/update", id)
	w := testenv.Serve(t, testenv.AuthedPOST(t, path, form, f.reader))
	if w.Code != http.StatusFound {
		t.Fatalf("reader-owner status=%d body=%s", w.Code, w.Body.String())
	}
	p, err = products.Find(id)
	if err != nil {
		t.Fatal(err)
	}
	if p.WebhookURL != "https://client.test/hook" || p.WebhookSecret != webhookSecret {
		t.Fatal("reader-owner update cleared admin-only webhook settings")
	}
}
