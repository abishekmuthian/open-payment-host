package products

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/abishekmuthian/open-payment-host/src/internal/testenv"
	"github.com/abishekmuthian/open-payment-host/src/lib/status"
)

func TestStoryCRUDAndPriceColumns(t *testing.T) {
	testenv.Config(t, nil)
	testenv.DB(t)

	id, err := New().Create(map[string]string{
		"name":                        "Handbook #go #pay",
		"schedule":                    "monthly",
		"status":                      "100",
		"user_id":                     "3",
		"stripe_price":                `{"DF":"price_DF","IN":"price_IN"}`,
		"square_price":                `{"DF":{"amount":1050,"currency":"USD"}}`,
		"paypal_price":                `{"US":{"amount":10.5,"currency":"USD","plan_id":"P-1","tax":1}}`,
		"razorpay_price":              `{"IN":{"plan_id":"plan_1"}}`,
		"square_subscription_plan_Id": `{"DF":"CAT_1"}`,
		"webhook_url":                 "https://client.test/hook",
		"webhook_secret":              "secret-value",
		"allowed_redirect_origins":    "https://client.test",
		"listmonk_list_id":            "7",
		"total_subscribers":           "2",
		"total_onetime_payments":      "3",
	})
	if err != nil {
		t.Fatal(err)
	}
	p, err := Find(id)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "Handbook #go #pay" || p.Schedule != "monthly" || p.Status != status.Published || p.UserID != 3 {
		t.Fatalf("scalar columns: %+v", p)
	}
	if !reflect.DeepEqual(p.StripePrice, map[string]string{"DF": "price_DF", "IN": "price_IN"}) {
		t.Fatalf("stripe=%v", p.StripePrice)
	}
	// JSON numbers come back as float64.
	if p.SquarePrice["DF"]["amount"] != 1050.0 || p.SquarePrice["DF"]["currency"] != "USD" {
		t.Fatalf("square=%v", p.SquarePrice)
	}
	if p.PaypalPrice["US"]["amount"] != 10.5 || p.PaypalPrice["US"]["tax"] != 1.0 || p.PaypalPrice["US"]["plan_id"] != "P-1" {
		t.Fatalf("paypal=%v", p.PaypalPrice)
	}
	if p.RazorpayPrice["IN"]["plan_id"] != "plan_1" || p.SquareSubscriptionPlanId["DF"] != "CAT_1" {
		t.Fatalf("razorpay=%v plans=%v", p.RazorpayPrice, p.SquareSubscriptionPlanId)
	}
	if p.WebhookURL != "https://client.test/hook" || p.WebhookSecret != "secret-value" || p.AllowedRedirectOrigins != "https://client.test" {
		t.Fatal("webhook columns")
	}
	if p.ListmonkListID != 7 || p.TotalSubscribers != 2 || p.TotalOnetimePayments != 3 {
		t.Fatalf("counters: %+v", p)
	}

	oldKey := p.CacheKey()
	time.Sleep(1100 * time.Millisecond) // updated_at has second precision
	if err := p.Update(map[string]string{"name": "Renamed"}); err != nil {
		t.Fatal(err)
	}
	p2, _ := FindFirst("name=?", "Renamed")
	if p2 == nil || p2.ID != id || p2.CacheKey() == oldKey {
		t.Fatalf("update/cache key: %+v", p2)
	}
	if err := p2.Destroy(); err != nil {
		t.Fatal(err)
	}
	if _, err := Find(id); err == nil {
		t.Fatal("destroyed product still found")
	}
	if all, err := FindAll(Published()); err != nil || len(all) != 0 {
		t.Fatalf("published after destroy: %d %v", len(all), err)
	}
}

func TestNewWithColumnsToleratesBadJSON(t *testing.T) {
	p := NewWithColumns(map[string]interface{}{"id": int64(4), "stripe_price": "not json", "square_price": []byte(`{"DF":{"amount":1}}`), "paypal_price": nil})
	if p.ID != 4 || len(p.StripePrice) != 0 || p.SquarePrice["DF"]["amount"] != 1.0 || len(p.PaypalPrice) != 0 {
		t.Fatalf("%+v", p)
	}
}

func TestStoryHelpers(t *testing.T) {
	testenv.Config(t, map[string]string{"domain": "merchant.test"})
	s := New()
	s.Name, s.URL, s.Points, s.UserID, s.ID = "Great tool #go #payments", "https://www.example.com/x", 1, 5, 12
	if s.NameDisplay() != "Great tool " || !reflect.DeepEqual(s.Tags(), []string{"go", "payments"}) {
		t.Fatalf("name=%q tags=%v", s.NameDisplay(), s.Tags())
	}
	if !reflect.DeepEqual(s.GetHashTag(), []string{"#go", "#payments"}) {
		t.Fatalf("hashtags=%v", s.GetHashTag())
	}
	if s.DestinationURL() != "https://www.example.com/x" || s.Domain() != "example.com" {
		t.Fatalf("destination=%q domain=%q", s.DestinationURL(), s.Domain())
	}
	if !strings.HasPrefix(s.CanonicalURL(), "/products/12-") || s.PermaURL() != testenv.RootURL+"/products/12" {
		t.Fatalf("canonical=%q perma=%q", s.CanonicalURL(), s.PermaURL())
	}
	s.URL = ""
	if s.DestinationURL() != s.CanonicalURL() || s.Domain() != "merchant.test" {
		t.Fatal("empty URL fallbacks")
	}
	s.Points = -1
	if s.DestinationURL() != "" || s.NegativePoints() != 1 {
		t.Fatal("downvoted story")
	}
	plain := &Story{Name: "Plain"}
	if plain.NameDisplay() != "Plain" || plain.Tags() != nil {
		t.Fatal("no hashtags")
	}
	if !s.OwnedBy(5) || s.OwnedBy(6) {
		t.Fatal("ownership")
	}
	s.CreatedAt = time.Now()
	if !s.Editable() {
		t.Fatal("new story not editable")
	}
	s.CreatedAt = time.Now().Add(-2 * time.Hour)
	if s.Editable() {
		t.Fatal("old story editable")
	}
	if New().Status != status.Draft || New().TableName != TableName {
		t.Fatal("New defaults")
	}
}

func TestAllowedParams(t *testing.T) {
	admin := map[string]bool{}
	for _, p := range AllowedParamsAdmin() {
		admin[p] = true
	}
	for _, p := range []string{"stripe_price", "square_price", "paypal_price", "razorpay_price", "schedule", "webhook_url", "webhook_secret", "allowed_redirect_origins", "listmonk_list_id", "status"} {
		if !admin[p] {
			t.Errorf("admins cannot set %s", p)
		}
	}
	for _, p := range AllowedParams() {
		if strings.Contains(p, "price") || strings.HasPrefix(p, "webhook") || p == "status" || p == "user_id" {
			t.Errorf("non-admins may set %s", p)
		}
	}
}
