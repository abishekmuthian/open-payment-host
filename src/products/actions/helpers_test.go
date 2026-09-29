package storyactions

import (
	"encoding/json"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/abishekmuthian/open-payment-host/src/internal/testenv"
	"github.com/abishekmuthian/open-payment-host/src/products"
)

func TestHashTags(t *testing.T) {
	cases := []struct {
		in      string
		count   int
		removed string
	}{
		{"Plain name", 0, "Plain name"},
		{"Tool #go", 1, "Tool "},
		{"Tool #go #payments", 2, "Tool  "},
		{"#a#b #c", 3, " "},
		{"price #1 and # alone", 2, "price  and  alone"},
	}
	for _, c := range cases {
		if got := CountHashTag(c.in); got != c.count {
			t.Errorf("CountHashTag(%q)=%d want %d", c.in, got, c.count)
		}
		if got := RemoveHashTag(c.in); got != c.removed {
			t.Errorf("RemoveHashTag(%q)=%q want %q", c.in, got, c.removed)
		}
	}
	if got := MetaHashTag([]string{"#go", "#pay"}); got != "go,pay," {
		t.Errorf("MetaHashTag=%q", got)
	}
	if MetaHashTag(nil) != "" {
		t.Error("MetaHashTag(nil) not empty")
	}
}

func TestSquarePlanCadence(t *testing.T) {
	for schedule, want := range map[string]string{"onetime": "MONTHLY", "monthly": "MONTHLY", "yearly": "ANNUAL", "": "MONTHLY"} {
		if got := squarePlanCadence(schedule); got != want {
			t.Errorf("%q: %s want %s", schedule, got, want)
		}
	}
}

func TestTruncateString(t *testing.T) {
	cases := []struct {
		in    string
		limit int
		want  string
	}{
		{"", 10, ""}, // regression: short strings gained "..."
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"},
		{"this is longer than ten", 10, "this is..."},
		{"héllo wörld ünïcode", 8, "héllo..."},
		{"abcdef", 3, "abc"},
	}
	for _, c := range cases {
		got := truncateString(c.in, c.limit)
		if got != c.want || len([]rune(got)) > c.limit {
			t.Errorf("truncateString(%q,%d)=%q want %q", c.in, c.limit, got, c.want)
		}
	}
}

func TestPaymentEntryURLEscapes(t *testing.T) {
	got := paymentEntryURL("paypal", 7, "a&b=c #d", "https://client.test/done?x=1&y=2")
	u, err := url.Parse(got)
	if err != nil || u.Path != "/subscriptions/paypal" {
		t.Fatalf("%q: %v", got, err)
	}
	q := u.Query()
	if q.Get("product_id") != "7" || q.Get("custom_id") != "a&b=c #d" || q.Get("redirect_uri") != "https://client.test/done?x=1&y=2" || len(q) != 3 {
		t.Fatalf("query=%v", q)
	}
}

func TestCreateCountryMap(t *testing.T) {
	testenv.Root(t)
	m := CreateCountryMap()
	if len(m) < 200 || m["DF"] == "" || m["IN"] != "India" || m["US"] == "" {
		t.Fatalf("country map has %d entries (DF=%q IN=%q)", len(m), m["DF"], m["IN"])
	}
	var countries []Country
	for code, name := range m {
		countries = append(countries, Country{Code: code, Name: name})
	}
	sort.Sort(ByName(countries))
	if !sort.SliceIsSorted(countries, func(i, j int) bool { return countries[i].Name < countries[j].Name }) {
		t.Fatal("ByName does not sort")
	}
}

func TestSelectGateway(t *testing.T) {
	amount := func(a float64) map[string]interface{} { return map[string]interface{}{"amount": a, "currency": "USD"} }
	plan := map[string]interface{}{"plan_id": "P-1"}
	full := func() *products.Story {
		return &products.Story{
			StripePrice:   map[string]string{"US": "price_US", "DF": "price_DF"},
			SquarePrice:   map[string]map[string]interface{}{"US": amount(1), "DF": amount(1)},
			PaypalPrice:   map[string]map[string]interface{}{"US": amount(1), "DF": amount(1)},
			RazorpayPrice: map[string]map[string]interface{}{"US": amount(1), "DF": amount(1)},
		}
	}
	cases := []struct {
		name            string
		story           *products.Story
		country         string
		gateway, priced string
	}{
		{"priority stripe", full(), "US", "stripe", "US"},
		{"country beats default", &products.Story{StripePrice: map[string]string{"DF": "p"}, RazorpayPrice: map[string]map[string]interface{}{"IN": amount(1)}}, "IN", "razorpay", "IN"},
		{"square before paypal", &products.Story{SquarePrice: map[string]map[string]interface{}{"US": amount(1)}, PaypalPrice: map[string]map[string]interface{}{"US": amount(1)}}, "US", "square", "US"},
		{"paypal before razorpay", &products.Story{PaypalPrice: map[string]map[string]interface{}{"US": plan}, RazorpayPrice: map[string]map[string]interface{}{"US": plan}}, "US", "paypal", "US"},
		{"razorpay plan only", &products.Story{RazorpayPrice: map[string]map[string]interface{}{"IN": plan}}, "IN", "razorpay", "IN"},
		{"square without amount ignored", &products.Story{SquarePrice: map[string]map[string]interface{}{"US": {"currency": "USD"}}, PaypalPrice: map[string]map[string]interface{}{"DF": amount(1)}}, "US", "paypal", "DF"},
		{"default fallback", full(), "FR", "stripe", "DF"},
		{"default fallback razorpay", &products.Story{RazorpayPrice: map[string]map[string]interface{}{"DF": plan}}, "FR", "razorpay", "DF"},
		{"empty country uses default", &products.Story{PaypalPrice: map[string]map[string]interface{}{"DF": amount(1)}}, "", "paypal", "DF"},
		{"empty entries ignored", &products.Story{StripePrice: map[string]string{"US": ""}, PaypalPrice: map[string]map[string]interface{}{"US": {}}}, "US", "", "US"},
		{"no prices", &products.Story{}, "US", "", "US"},
	}
	for _, c := range cases {
		gateway, country := selectGateway(c.story, c.country)
		if gateway != c.gateway || country != c.priced {
			t.Errorf("%s: got %s/%s want %s/%s", c.name, gateway, country, c.gateway, c.priced)
		}
	}
}

func TestBuildPriceMaps(t *testing.T) {
	form := url.Values{
		// Stripe: row 2 has no price ID; row 3 repeats DF; placeholder country skipped.
		"stripe_country_0": {"DF"}, "stripe_plan_id_0": {"price_DF"},
		"stripe_country_2": {"IN"}, "stripe_plan_id_2": {""},
		"stripe_country_3": {"DF"}, "stripe_plan_id_3": {"price_dup"},
		"stripe_country_4": {"Select Country"}, "stripe_plan_id_4": {"price_x"},
		"stripe_country_10": {"US"}, "stripe_plan_id_10": {"price_US"},
		// Square: bad amount and missing currency rows are skipped.
		"square_country_0": {"DF"}, "square_amount_0": {"1050"}, "square_currency_0": {"USD"},
		"square_country_1": {"IN"}, "square_amount_1": {"ten"}, "square_currency_1": {"INR"},
		"square_country_2": {"US"}, "square_amount_2": {"99"},
		// PayPal: tax parsed; bad tax skips the row.
		"paypal_country_0": {"DF"}, "paypal_amount_0": {"10.50"}, "paypal_currency_0": {"USD"}, "paypal_tax_0": {"1"},
		"paypal_country_1": {"IN"}, "paypal_amount_1": {"5"}, "paypal_currency_1": {"INR"}, "paypal_tax_1": {"x"},
		// Razorpay: amount-less row lacks what one-time needs.
		"razorpay_country_0": {"IN"}, "razorpay_amount_0": {"499"}, "razorpay_currency_0": {"INR"},
		"razorpay_country_1": {"US"}, "razorpay_plan_id_1": {"plan_1"},
	}
	got := buildPriceMaps(form, "onetime")
	if !reflect.DeepEqual(got.Stripe, map[string]string{"DF": "price_DF", "US": "price_US"}) {
		t.Errorf("stripe=%v", got.Stripe)
	}
	if !reflect.DeepEqual(got.Square, map[string]map[string]interface{}{"DF": {"amount": 1050.0, "currency": "USD"}}) {
		t.Errorf("square=%v", got.Square)
	}
	if !reflect.DeepEqual(got.Paypal, map[string]map[string]interface{}{"DF": {"amount": 10.5, "currency": "USD", "tax": 1.0}}) {
		t.Errorf("paypal=%v", got.Paypal)
	}
	if !reflect.DeepEqual(got.Razorpay, map[string]map[string]interface{}{"IN": {"amount": 499.0, "currency": "INR"}}) {
		t.Errorf("razorpay=%v", got.Razorpay)
	}

	// Recurring schedules need plan IDs; amounts are optional.
	for _, schedule := range []string{"monthly", "yearly"} {
		got = buildPriceMaps(form, schedule)
		if !reflect.DeepEqual(got.Razorpay, map[string]map[string]interface{}{"US": {"plan_id": "plan_1"}}) {
			t.Errorf("%s razorpay=%v", schedule, got.Razorpay)
		}
		if len(got.Paypal) != 0 {
			t.Errorf("%s paypal rows without plan kept: %v", schedule, got.Paypal)
		}
	}

	// Stored JSON round-trips through the model exactly as the handlers save it.
	got = buildPriceMaps(form, "onetime")
	cols := map[string]interface{}{}
	for col, v := range map[string]interface{}{"stripe_price": got.Stripe, "square_price": got.Square, "paypal_price": got.Paypal, "razorpay_price": got.Razorpay} {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		cols[col] = string(b)
	}
	p := products.NewWithColumns(cols)
	if !reflect.DeepEqual(p.StripePrice, got.Stripe) || !reflect.DeepEqual(p.SquarePrice, got.Square) || !reflect.DeepEqual(p.PaypalPrice, got.Paypal) || !reflect.DeepEqual(p.RazorpayPrice, got.Razorpay) {
		t.Fatalf("round trip mismatch: %+v", p)
	}
	if empty := buildPriceMaps(url.Values{}, "onetime"); len(empty.Stripe)+len(empty.Square)+len(empty.Paypal)+len(empty.Razorpay) != 0 || empty.Stripe == nil {
		t.Fatal("empty form must give empty, non-nil maps (stored as {})")
	}
	if strings.Contains(mustJSON(t, buildPriceMaps(url.Values{}, "onetime").Stripe), "null") {
		t.Fatal("empty stripe map serialised as null")
	}
}

func mustJSON(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
