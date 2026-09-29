package subscriptions

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/abishekmuthian/open-payment-host/src/internal/testenv"
	"github.com/abishekmuthian/open-payment-host/src/products"
)

func TestCurrencyMinorDecimals(t *testing.T) {
	for currency, want := range map[string]int{"USD": 2, "usd": 2, "INR": 2, "JPY": 0, "jpy": 0, "KRW": 0, "KWD": 3, "bhd": 3, "": 2, "XYZ": 2} {
		if got := currencyMinorDecimals(currency); got != want {
			t.Errorf("%q: %d want %d", currency, got, want)
		}
	}
}

func TestMajorValueToMinor(t *testing.T) {
	ok := []struct {
		value, currency string
		want            int64
	}{
		{"10.50", "USD", 1050}, {"10.5", "USD", 1050}, {"10", "USD", 1000}, {"0", "USD", 0}, {"0.01", "usd", 1},
		{"12", "JPY", 12}, {"1.234", "KWD", 1234}, {"1.2", "KWD", 1200}, {"007", "USD", 700},
		{"92233720368547758.07", "USD", 9223372036854775807},
	}
	for _, c := range ok {
		got, err := majorValueToMinor(c.value, c.currency)
		if err != nil || got != c.want {
			t.Errorf("%s %s: %d %v want %d", c.value, c.currency, got, err, c.want)
		}
	}
	bad := []struct{ value, currency string }{
		{"", "USD"}, {"1", ""}, {"1", "US"}, {"1", "USDX"}, {"10.", "USD"}, {".5", "USD"}, {"1.5", "JPY"},
		{"1.2345", "KWD"}, {"1.001", "USD"}, {"-1", "USD"}, {"+1", "USD"}, {"1,25", "USD"}, {"1e2", "USD"},
		{"NaN", "USD"}, {"1.2.3", "USD"}, {" 1", "USD"}, {"92233720368547758.08", "USD"},
	}
	for _, c := range bad {
		if got, err := majorValueToMinor(c.value, c.currency); err == nil {
			t.Errorf("%q %q accepted as %d", c.value, c.currency, got)
		}
	}
}

func TestConfiguredMinorAndFormatting(t *testing.T) {
	cases := []struct {
		value        interface{}
		currency     string
		alreadyMinor bool
		want         int64
		display      string
	}{
		{10.5, "USD", false, 1050, "10.50 USD"},
		{43.21, "USD", false, 4321, "43.21 USD"},
		{123456.0, "INR", false, 12345600, "123456.00 INR"},
		{1050.0, "USD", true, 1050, "10.50 USD"},
		{json.Number("1050"), "USD", true, 1050, "10.50 USD"},
		{int64(5), "USD", true, 5, "0.05 USD"},
		{"12.34", "USD", false, 1234, "12.34 USD"},
		{1000.0, "JPY", false, 1000, "1000 JPY"},
		{1.234, "KWD", false, 1234, "1.234 KWD"},
		{49900, "INR", true, 49900, "499.00 INR"},
	}
	for _, c := range cases {
		got, err := configuredMinor(c.value, c.currency, c.alreadyMinor)
		if err != nil || got != c.want {
			t.Errorf("configuredMinor(%v,%s,%v)=%d %v want %d", c.value, c.currency, c.alreadyMinor, got, err, c.want)
		}
		display, err := FormatConfiguredPrice(c.value, c.currency, c.alreadyMinor)
		if err != nil || display != c.display {
			t.Errorf("FormatConfiguredPrice(%v)=%q %v want %q", c.value, display, err, c.display)
		}
	}
	for _, bad := range []struct {
		value        interface{}
		alreadyMinor bool
	}{{10.5, true}, {-1.0, true}, {"1e3", true}, {nil, false}, {nil, true}, {1e21, true}, {10.555, false}, {"ten", false}, {-3.0, false}} {
		if got, err := configuredMinor(bad.value, "USD", bad.alreadyMinor); err == nil {
			t.Errorf("configuredMinor(%v,%v) accepted as %d", bad.value, bad.alreadyMinor, got)
		}
		if _, err := FormatConfiguredPrice(bad.value, "USD", bad.alreadyMinor); err == nil {
			t.Errorf("FormatConfiguredPrice(%v) accepted", bad.value)
		}
	}
	for _, c := range []struct {
		amount   int64
		currency string
		want     string
	}{{0, "USD", "0.00"}, {7, "USD", "0.07"}, {100, "USD", "1.00"}, {5, "KWD", "0.005"}, {5, "JPY", "5"}} {
		if got := minorDecimal(c.amount, c.currency); got != c.want {
			t.Errorf("minorDecimal(%d,%s)=%q want %q", c.amount, c.currency, got, c.want)
		}
	}
}

func TestCurrencyEqual(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{{"usd", "USD", true}, {"INR", "INR", true}, {"USD", "EUR", false}, {"", "", false}, {"USD", "", false}} {
		if currencyEqual(c.a, c.b) != c.want {
			t.Errorf("currencyEqual(%q,%q) != %v", c.a, c.b, c.want)
		}
	}
}

func TestAddPercentage(t *testing.T) {
	for _, c := range []struct {
		amount int64
		pct    string
		want   int64
	}{{1000, "18", 1180}, {1000, "0", 1000}, {999, "7.5", 1074}, {1, "50", 2}, {3, "50", 5}, {1050, "12.25", 1179}} {
		got, err := addPercentage(c.amount, c.pct)
		if err != nil || got != c.want {
			t.Errorf("addPercentage(%d,%s)=%d %v want %d", c.amount, c.pct, got, err, c.want)
		}
	}
	for _, pct := range []string{"-1", "abc", ""} {
		if _, err := addPercentage(1000, pct); err == nil {
			t.Errorf("addPercentage accepted %q", pct)
		}
	}
	if _, err := addPercentage(9223372036854775807, "100"); err == nil {
		t.Error("overflow accepted")
	}
}

func TestStripeExpectedAmount(t *testing.T) {
	testenv.Config(t, map[string]string{"stripe_secret": "sk_test"})
	rates := map[string]string{
		"txr_excl":     `{"id":"txr_excl","active":true,"inclusive":false,"percentage":18}`,
		"txr_frac":     `{"id":"txr_frac","active":true,"inclusive":false,"percentage":7.25}`,
		"txr_incl":     `{"id":"txr_incl","active":true,"inclusive":true,"percentage":20}`,
		"txr_inactive": `{"id":"txr_inactive","active":false,"inclusive":false,"percentage":5}`,
	}
	testenv.StripeBackend(t, func(r *http.Request) (*http.Response, error) {
		id := strings.TrimPrefix(r.URL.Path, "/v1/tax_rates/")
		if body, ok := rates[id]; ok {
			return testenv.Response(http.StatusOK, body), nil
		}
		return testenv.Response(http.StatusNotFound, `{"error":{"type":"invalid_request_error","message":"No such tax rate"}}`), nil
	})
	for _, c := range []struct {
		base int64
		id   string
		want int64
	}{{1000, "", 1000}, {1000, "txr_excl", 1180}, {999, "txr_frac", 1071}, {1000, "txr_incl", 1000}} {
		got, err := stripeExpectedAmount(c.base, c.id)
		if err != nil || got != c.want {
			t.Errorf("%d %s: %d %v want %d", c.base, c.id, got, err, c.want)
		}
	}
	for _, id := range []string{"txr_inactive", "txr_missing"} {
		if _, err := stripeExpectedAmount(1000, id); err == nil {
			t.Errorf("%s accepted", id)
		}
	}
}

func TestRedirectOriginAndValidation(t *testing.T) {
	testenv.Config(t, nil)
	for raw, want := range map[string]string{
		"https://Client.Test/done":       "https://client.test",
		"HTTPS://CLIENT.TEST:8443/x?y#z": "https://client.test:8443",
		"http://localhost:3000/ok":       "http://localhost:3000",
	} {
		if got, err := redirectOrigin(raw); err != nil || got != want {
			t.Errorf("redirectOrigin(%q)=%q %v want %q", raw, got, err, want)
		}
	}
	for _, raw := range []string{"", "/relative", "//client.test/x", "ftp://client.test", "javascript:alert(1)", "https://user@client.test", "https:///nohost", "mailto:a@b.c", "https://client.test\\@evil.test"} {
		if got, err := redirectOrigin(raw); err == nil {
			t.Errorf("redirectOrigin(%q) accepted as %q", raw, got)
		}
	}
	p := &products.Story{WebhookURL: "https://hook.test/api", AllowedRedirectOrigins: "https://a.test, https://b.test:8443\nhttps://c.test\r\n"}
	for _, raw := range []string{"https://hook.test/thanks", "https://A.TEST/x", "https://b.test:8443/y", "https://c.test/"} {
		if got, err := ValidateRedirectURI(p, raw); err != nil || got != raw {
			t.Errorf("ValidateRedirectURI(%q)=%q %v", raw, got, err)
		}
	}
	for _, raw := range []string{"https://b.test/y", "https://a.test:444/x", "http://a.test/x", "https://d.test/", "https://a.test.evil.test/"} {
		if _, err := ValidateRedirectURI(p, raw); err == nil {
			t.Errorf("ValidateRedirectURI(%q) accepted", raw)
		}
	}
	if got, err := ValidateRedirectURI(p, ""); got != "" || err != nil {
		t.Errorf("empty redirect: %q %v", got, err)
	}
	if _, err := ValidateRedirectURI(nil, "https://a.test/"); err == nil {
		t.Error("nil product accepted")
	}
	testenv.Production(t)
	if _, err := redirectOrigin("http://client.test/"); err == nil {
		t.Error("production accepted plain HTTP redirect")
	}
	if _, err := ValidateRedirectURI(&products.Story{AllowedRedirectOrigins: "http://client.test"}, "http://client.test/x"); err == nil {
		t.Error("production accepted HTTP allowed origin")
	}
}

func TestBuildRedirectURL(t *testing.T) {
	for _, c := range []struct {
		base   string
		params map[string]string
		want   string
	}{
		{"https://c.test/done", map[string]string{"custom_id": "a b"}, "https://c.test/done?custom_id=a+b"},
		{"https://c.test/done?keep=1&custom_id=old", map[string]string{"custom_id": "new"}, "https://c.test/done?custom_id=new&keep=1"},
		{"https://c.test/done#frag", map[string]string{"x": "&"}, "https://c.test/done?x=%26#frag"},
		{"://bad", map[string]string{"x": "1"}, ""},
	} {
		if got := BuildRedirectURL(c.base, c.params); got != c.want {
			t.Errorf("BuildRedirectURL(%q)=%q want %q", c.base, got, c.want)
		}
	}
}

func TestCapabilityRoundTripAndTampering(t *testing.T) {
	testenv.Config(t, nil)
	sealed, err := sealCapability("secret token value")
	if err != nil {
		t.Fatal(err)
	}
	again, _ := sealCapability("secret token value")
	if sealed == again {
		t.Fatal("ciphertext is deterministic")
	}
	if strings.Contains(sealed, "secret") {
		t.Fatal("plaintext visible")
	}
	if got, err := openCapability(sealed); err != nil || got != "secret token value" {
		t.Fatalf("round trip: %q %v", got, err)
	}
	tampered := []byte(sealed)
	tampered[len(tampered)-3] ^= 1
	for name, value := range map[string]string{"tampered": string(tampered), "truncated": sealed[:10], "empty": "", "not base64": "!!!", "short": "AAAA"} {
		if got, err := openCapability(value); err == nil {
			t.Errorf("%s ciphertext opened as %q", name, got)
		}
	}
	testenv.Set("secret_key", strings.Repeat("x", 32))
	if _, err := openCapability(sealed); err == nil {
		t.Error("opened with a different key")
	}
	testenv.Set("secret_key", "short-key")
	if _, err := sealCapability("x"); err == nil {
		t.Error("sealed with a short key")
	}
	if _, err := openCapability(sealed); err == nil {
		t.Error("opened with a short key")
	}
}

func TestTokenHelpers(t *testing.T) {
	a, _ := generateToken(16)
	b, _ := generateToken(16)
	if len(a) != 32 || a == b {
		t.Fatalf("tokens %q %q", a, b)
	}
	h := hashToken(a)
	if !tokensMatch(a, h) || tokensMatch(b, h) || tokensMatch("", h) || tokensMatch(a, "") {
		t.Fatal("tokensMatch")
	}
	if formatInt(42) != "42" || fmt.Sprint(len(hashToken("x"))) != "64" {
		t.Fatal("helpers")
	}
}
