package storyactions

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/abishekmuthian/open-payment-host/src/internal/testenv"
	"github.com/abishekmuthian/open-payment-host/src/products"
)

// gatewaysOn enables every gateway with dummy credentials so all price
// fields are handled; no provider is ever contacted (transports are mocked).
var gatewaysOn = map[string]string{
	"stripe": "yes", "stripe_key": "pk_test", "stripe_secret": "sk_test",
	"square": "yes", "square_access_token": "sq_token", "square_app_id": "sq_app",
	"paypal": "yes", "paypal_client_id": "pp_id", "paypal_client_secret": "pp_secret",
	"razorpay": "yes", "razorpay_key_id": "rzp_key", "razorpay_key_secret": "rzp_secret",
}

var schedules = []string{"onetime", "monthly", "yearly"}
var toggleGateways = []string{"stripe", "square", "paypal", "razorpay", "api"}

const webhookSecret = "whsec-SECRET-123"

type fixture struct {
	admin, reader int64
}

// setupHandlers installs config, DB, templates, auth and the product routes
// exactly as src/app/routes.go maps them.
func setupHandlers(t *testing.T, overrides map[string]string) fixture {
	t.Helper()
	cfg := map[string]string{}
	for k, v := range gatewaysOn {
		cfg[k] = v
	}
	for k, v := range overrides {
		cfg[k] = v
	}
	testenv.Setup(t, cfg)
	testenv.Route(http.MethodGet, "/index{format:(.xml)?}", HandleIndex)
	testenv.Route(http.MethodGet, "/products/create", HandleCreateShow)
	testenv.Route(http.MethodPost, "/products/create", HandleCreate)
	testenv.Route(http.MethodGet, "/products/create/price/{fieldIndex:[0-9]+}/{pg:[a-zA-Z]+}/{schedule:[a-zA-Z ]+}", HandlePrice)
	testenv.Route(http.MethodGet, "/products/create/schedule", HandleSchedule)
	testenv.Route(http.MethodPost, "/products/toggle/stripe", HandleToggleStripe)
	testenv.Route(http.MethodPost, "/products/toggle/square", HandleToggleSquare)
	testenv.Route(http.MethodPost, "/products/toggle/paypal", HandleTogglePaypal)
	testenv.Route(http.MethodPost, "/products/toggle/razorpay", HandleToggleRazorpay)
	testenv.Route(http.MethodPost, "/products/toggle/api", HandleToggleAPI)
	testenv.Route(http.MethodPost, "/products/{id:[0-9]+}/toggle/stripe", HandleToggleStripeUpdate)
	testenv.Route(http.MethodPost, "/products/{id:[0-9]+}/toggle/square", HandleToggleSquareUpdate)
	testenv.Route(http.MethodPost, "/products/{id:[0-9]+}/toggle/paypal", HandleTogglePaypalUpdate)
	testenv.Route(http.MethodPost, "/products/{id:[0-9]+}/toggle/razorpay", HandleToggleRazorpayUpdate)
	testenv.Route(http.MethodPost, "/products/{id:[0-9]+}/toggle/api", HandleToggleAPIUpdate)
	testenv.Route(http.MethodGet, "/products/{id:[0-9]+}/update", HandleUpdateShow)
	testenv.Route(http.MethodPost, "/products/{id:[0-9]+}/update", HandleUpdate)
	testenv.Route(http.MethodPost, "/products/{id:[0-9]+}/destroy", HandleDestroy)
	testenv.Route(http.MethodGet, "/products/{id:[0-9]+}", HandleShow)
	testenv.Route(http.MethodGet, "/products{format:(.xml)?}", HandleIndex)
	testenv.Route(http.MethodGet, "/sitemap.xml", HandleSiteMap)
	return fixture{
		admin:  testenv.SeedUser(t, testenv.RoleAdmin, "admin@merchant.test", "admin-password"),
		reader: testenv.SeedUser(t, testenv.RoleReader, "reader@merchant.test", "reader-password"),
	}
}

// seedPricedProduct stores recognisable prices for every gateway.
func seedPricedProduct(t *testing.T, schedule string, owner int64) int64 {
	t.Helper()
	return testenv.SeedProduct(t, map[string]interface{}{
		"schedule":       schedule,
		"user_id":        owner,
		"stripe_price":   map[string]string{"DF": "price_TESTDF"},
		"square_price":   map[string]interface{}{"DF": map[string]interface{}{"amount": 4321, "currency": "USD"}},
		"paypal_price":   map[string]interface{}{"DF": map[string]interface{}{"amount": 43.21, "currency": "USD", "plan_id": "P-PLANDF"}},
		"razorpay_price": map[string]interface{}{"DF": map[string]interface{}{"amount": 43.21, "currency": "INR", "plan_id": "plan_TESTDF"}},
		"webhook_url":    "https://client.test/hook",
		"webhook_secret": webhookSecret,
	})
}

func toggleForm(gateway, schedule string, on bool) url.Values {
	form := url.Values{"schedule": {schedule}}
	if on {
		form.Set(gateway+"-toggle", "on")
	}
	return form
}

func TestCreateToggles(t *testing.T) {
	f := setupHandlers(t, nil)
	for _, gateway := range toggleGateways {
		for _, schedule := range schedules {
			path := "/products/toggle/" + gateway
			t.Run(gateway+"/"+schedule, func(t *testing.T) {
				off := testenv.Serve(t, testenv.AuthedPOST(t, path, toggleForm(gateway, schedule, false), f.admin))
				if off.Code != http.StatusOK || !testenv.Blank(off.Body.String()) {
					t.Fatalf("off: status=%d body=%q", off.Code, off.Body.String())
				}
				on := testenv.Serve(t, testenv.AuthedPOST(t, path, toggleForm(gateway, schedule, true), f.admin))
				body := on.Body.String()
				if on.Code != http.StatusOK || testenv.Blank(body) {
					t.Fatalf("on: status=%d body=%q", on.Code, body)
				}
				if strings.Contains(body, "<html") {
					t.Fatal("toggle partial rendered with layout")
				}
				switch gateway {
				case "api":
					if !strings.Contains(body, `name="webhook_secret"`) {
						t.Fatal("webhook fields missing")
					}
				case "paypal", "razorpay":
					// Add Country must carry the requested schedule, not a DB value.
					if !strings.Contains(body, "/"+gateway+"/"+schedule+`"`) {
						t.Fatalf("add-country URL lacks schedule %s", schedule)
					}
				default:
					if !strings.Contains(body, gateway+"_country_0") {
						t.Fatal("price row missing")
					}
				}
			})
		}
	}
}

func TestCreateTogglesRequireAuthorisation(t *testing.T) {
	setupHandlers(t, nil)
	for _, gateway := range toggleGateways {
		path := "/products/toggle/" + gateway
		anon := testenv.Serve(t, testenv.AuthedPOST(t, path, toggleForm(gateway, "onetime", true), 0))
		if anon.Code != http.StatusUnauthorized {
			t.Errorf("%s: anonymous toggle status=%d", gateway, anon.Code)
		}
		noToken := testenv.Serve(t, testenv.POST(path, toggleForm(gateway, "onetime", true)))
		if noToken.Code != http.StatusUnauthorized {
			t.Errorf("%s: tokenless toggle status=%d", gateway, noToken.Code)
		}
	}
}

func TestUpdateTogglesPrefillOnlyForMatchingSchedule(t *testing.T) {
	f := setupHandlers(t, nil)
	markers := map[string][]string{
		"stripe":   {"price_TESTDF"},
		"square":   {"4321"},
		"paypal":   {"43.21"},
		"razorpay": {}, // amount only on one-time rows, checked below
	}
	for _, dbSchedule := range schedules {
		id := seedPricedProduct(t, dbSchedule, f.admin)
		for gateway, want := range markers {
			for _, schedule := range schedules {
				t.Run(fmt.Sprintf("%s/db=%s/req=%s", gateway, dbSchedule, schedule), func(t *testing.T) {
					path := fmt.Sprintf("/products/%d/toggle/%s", id, gateway)
					off := testenv.Serve(t, testenv.AuthedPOST(t, path, toggleForm(gateway, schedule, false), f.admin))
					if off.Code != http.StatusOK || !testenv.Blank(off.Body.String()) {
						t.Fatalf("off: status=%d body=%q", off.Code, off.Body.String())
					}
					on := testenv.Serve(t, testenv.AuthedPOST(t, path, toggleForm(gateway, schedule, true), f.admin))
					body := on.Body.String()
					if on.Code != http.StatusOK {
						t.Fatalf("on: status=%d body=%s", on.Code, body)
					}
					if !strings.Contains(body, "/"+gateway+"/"+schedule+`"`) {
						t.Fatalf("add-country URL lacks request schedule %s", schedule)
					}
					for _, marker := range want {
						if got := strings.Contains(body, marker); got != (schedule == dbSchedule) {
							t.Fatalf("marker %q present=%v with db=%s req=%s", marker, got, dbSchedule, schedule)
						}
					}
					if gateway == "razorpay" {
						if got := strings.Contains(body, "43.21"); got != (schedule == dbSchedule && schedule == "onetime") {
							t.Fatalf("razorpay amount present=%v with db=%s req=%s", got, dbSchedule, schedule)
						}
					}
					if gateway == "paypal" || gateway == "razorpay" {
						plan := map[string]string{"paypal": "P-PLANDF", "razorpay": "plan_TESTDF"}[gateway]
						if got := strings.Contains(body, plan); got != (schedule == dbSchedule && schedule != "onetime") {
							t.Fatalf("plan id present=%v with db=%s req=%s", got, dbSchedule, schedule)
						}
					}
				})
			}
		}
	}
}

func TestUpdateAPIToggleShowsWebhookToOwnerOnly(t *testing.T) {
	f := setupHandlers(t, nil)
	id := seedPricedProduct(t, "onetime", f.admin)
	path := fmt.Sprintf("/products/%d/toggle/api", id)
	on := testenv.Serve(t, testenv.AuthedPOST(t, path, toggleForm("api", "onetime", true), f.admin))
	if on.Code != http.StatusOK || !strings.Contains(on.Body.String(), webhookSecret) {
		t.Fatalf("admin: status=%d, secret shown=%v", on.Code, strings.Contains(on.Body.String(), webhookSecret))
	}
	if !strings.Contains(on.Body.String(), "https://client.test/hook") {
		t.Fatal("webhook URL not prefilled")
	}
}

// Regression: update toggles only checked CSRF, so any visitor with a session
// token could read a product's webhook secret and prices.
func TestUpdateTogglesRejectAnonAndNonOwner(t *testing.T) {
	f := setupHandlers(t, nil)
	id := seedPricedProduct(t, "monthly", f.admin)
	for _, gateway := range toggleGateways {
		path := fmt.Sprintf("/products/%d/toggle/%s", id, gateway)
		for name, user := range map[string]int64{"anon": 0, "non-owner": f.reader} {
			w := testenv.Serve(t, testenv.AuthedPOST(t, path, toggleForm(gateway, "monthly", true), user))
			if w.Code != http.StatusUnauthorized {
				t.Errorf("%s %s: status=%d", gateway, name, w.Code)
			}
			for _, secret := range []string{webhookSecret, "price_TESTDF", "P-PLANDF", "plan_TESTDF"} {
				if strings.Contains(w.Body.String(), secret) {
					t.Errorf("%s %s: leaked %s", gateway, name, secret)
				}
			}
		}
	}
	// An owning reader may use the toggles on their own product.
	own := seedPricedProduct(t, "monthly", f.reader)
	w := testenv.Serve(t, testenv.AuthedPOST(t, fmt.Sprintf("/products/%d/toggle/api", own), toggleForm("api", "monthly", true), f.reader))
	if w.Code != http.StatusOK {
		t.Fatalf("owner status=%d", w.Code)
	}
	missing := testenv.Serve(t, testenv.AuthedPOST(t, "/products/9999/toggle/stripe", toggleForm("stripe", "monthly", true), f.admin))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing product status=%d", missing.Code)
	}
}

func TestHandlePrice(t *testing.T) {
	setupHandlers(t, nil)
	for _, pg := range []string{"stripe", "square", "paypal", "razorpay"} {
		for _, schedule := range schedules {
			t.Run(pg+"/"+schedule, func(t *testing.T) {
				w := testenv.Serve(t, httptest.NewRequest(http.MethodGet, "/products/create/price/2/"+pg+"/"+schedule, nil))
				body := w.Body.String()
				if w.Code != http.StatusOK || !strings.Contains(body, pg+"_country_3") {
					t.Fatalf("status=%d body=%q", w.Code, body)
				}
				wantNext := "/products/create/price/3/" + pg + "/"
				if pg == "paypal" || pg == "razorpay" {
					wantNext += schedule
				}
				if !strings.Contains(body, wantNext) {
					t.Fatalf("next add-country URL %q missing", wantNext)
				}
			})
		}
	}
	for _, path := range []string{"/products/create/price/0/bitcoin/onetime", "/products/create/price/0/paypal/weekly", "/products/create/price/0/razorpay/any"} {
		if w := testenv.Serve(t, httptest.NewRequest(http.MethodGet, path, nil)); w.Code != http.StatusNotFound {
			t.Errorf("%s status=%d", path, w.Code)
		}
	}
}

func TestHandleSchedule(t *testing.T) {
	setupHandlers(t, nil)
	for _, schedule := range schedules {
		w := testenv.Serve(t, httptest.NewRequest(http.MethodGet, "/products/create/schedule?schedule="+schedule, nil))
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "/paypal/"+schedule+`"`) {
			t.Errorf("%s: status=%d", schedule, w.Code)
		}
	}
}

// squareCatalog answers Square catalog calls made when Square prices are saved.
func squareCatalog(t *testing.T, calls *int) {
	testenv.MockTransport(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "square.test" || !strings.HasSuffix(r.URL.Path, "/catalog/object") {
			return nil, fmt.Errorf("unexpected request %s", r.URL)
		}
		*calls++
		return testenv.Response(http.StatusOK, `{"catalog_object":{"id":"CAT_PLAN","type":"SUBSCRIPTION_PLAN"}}`), nil
	})
}

func productForm(schedule string) url.Values {
	return url.Values{
		"name": {"Handbook #go #payments"}, "summary": {"Summary"}, "description": {"<p>Body</p>"},
		"schedule": {schedule}, "status": {"100"},
		"webhook_url": {"https://client.test/hook"}, "webhook_secret": {webhookSecret},
		"allowed_redirect_origins": {"https://client.test"},
		"stripe_country_0":         {"DF"}, "stripe_plan_id_0": {"price_DF"},
		"stripe_country_1": {"IN"}, "stripe_plan_id_1": {"price_IN"},
		"square_country_0": {"DF"}, "square_amount_0": {"1050"}, "square_currency_0": {"USD"},
		"paypal_country_0": {"DF"}, "paypal_amount_0": {"10.50"}, "paypal_currency_0": {"USD"}, "paypal_plan_id_0": {"P-1"}, "paypal_tax_0": {"1"},
		"razorpay_country_0": {"IN"}, "razorpay_amount_0": {"499"}, "razorpay_currency_0": {"INR"}, "razorpay_plan_id_0": {"plan_1"},
	}
}

func TestCreatePersistsPricesScheduleAndWebhook(t *testing.T) {
	f := setupHandlers(t, nil)
	calls := 0
	squareCatalog(t, &calls)
	w := testenv.Serve(t, testenv.AuthedPOST(t, "/products/create", productForm("yearly"), f.admin))
	if w.Code != http.StatusFound {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	p, err := products.FindFirst("name=?", "Handbook #go #payments")
	if err != nil {
		t.Fatal(err)
	}
	if p.Schedule != "yearly" || p.WebhookURL != "https://client.test/hook" || p.WebhookSecret != webhookSecret || p.AllowedRedirectOrigins != "https://client.test" {
		t.Fatalf("fields not persisted: %+v", p)
	}
	if p.StripePrice["DF"] != "price_DF" || p.StripePrice["IN"] != "price_IN" {
		t.Fatalf("stripe=%v", p.StripePrice)
	}
	if p.SquarePrice["DF"]["amount"] != 1050.0 || p.SquarePrice["DF"]["currency"] != "USD" {
		t.Fatalf("square=%v", p.SquarePrice)
	}
	if p.PaypalPrice["DF"]["amount"] != 10.5 || p.PaypalPrice["DF"]["plan_id"] != "P-1" || p.PaypalPrice["DF"]["tax"] != 1.0 {
		t.Fatalf("paypal=%v", p.PaypalPrice)
	}
	if p.RazorpayPrice["IN"]["plan_id"] != "plan_1" || p.RazorpayPrice["IN"]["currency"] != "INR" {
		t.Fatalf("razorpay=%v", p.RazorpayPrice)
	}
	if calls != 1 || p.SquareSubscriptionPlanId["DF"] != "CAT_PLAN" {
		t.Fatalf("square plan calls=%d ids=%v", calls, p.SquareSubscriptionPlanId)
	}
	if p.UserID != f.admin {
		t.Fatalf("owner=%d", p.UserID)
	}
}

func TestCreateRejectsHashtagsAnonAndBadAmounts(t *testing.T) {
	f := setupHandlers(t, nil)
	calls := 0
	squareCatalog(t, &calls)
	form := productForm("onetime")
	form.Set("name", "Spam #a #b #c")
	if w := testenv.Serve(t, testenv.AuthedPOST(t, "/products/create", form, f.admin)); w.Code != http.StatusUnauthorized {
		t.Fatalf("3 hashtags status=%d", w.Code)
	}
	if w := testenv.Serve(t, testenv.AuthedPOST(t, "/products/create", productForm("onetime"), 0)); w.Code != http.StatusUnauthorized {
		t.Fatalf("anon status=%d", w.Code)
	}
	if n := testenv.Scalar(t, "SELECT COUNT(*) FROM products"); n != 0 {
		t.Fatalf("rejected creates stored %d products", n)
	}
	// Regression: an unparsable Square amount panicked in amount.(float64).
	form = productForm("onetime")
	form.Set("square_amount_0", "ten dollars")
	w := testenv.Serve(t, testenv.AuthedPOST(t, "/products/create", form, f.admin))
	if w.Code != http.StatusFound {
		t.Fatalf("bad amount status=%d", w.Code)
	}
	p, err := products.FindFirst("name=?", "Handbook #go #payments")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.SquarePrice) != 0 || calls != 0 {
		t.Fatalf("invalid square row stored: %v (calls=%d)", p.SquarePrice, calls)
	}
}

func TestUpdatePersistsAndAuthorises(t *testing.T) {
	f := setupHandlers(t, nil)
	calls := 0
	squareCatalog(t, &calls)
	id := seedPricedProduct(t, "onetime", f.admin)
	path := fmt.Sprintf("/products/%d/update", id)

	for name, user := range map[string]int64{"anon": 0, "non-owner": f.reader} {
		if w := testenv.Serve(t, testenv.AuthedPOST(t, path, productForm("monthly"), user)); w.Code != http.StatusUnauthorized {
			t.Fatalf("%s status=%d", name, w.Code)
		}
	}
	if p, _ := products.Find(id); p.Schedule != "onetime" {
		t.Fatal("unauthorised update applied")
	}

	form := productForm("monthly")
	form.Set("square_amount_0", "12abc") // must not panic; row is skipped
	w := testenv.Serve(t, testenv.AuthedPOST(t, path, form, f.admin))
	if w.Code != http.StatusFound {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	p, err := products.Find(id)
	if err != nil {
		t.Fatal(err)
	}
	if p.Schedule != "monthly" || p.StripePrice["IN"] != "price_IN" || p.PaypalPrice["DF"]["plan_id"] != "P-1" || p.RazorpayPrice["IN"]["plan_id"] != "plan_1" {
		t.Fatalf("update not persisted: %+v", p)
	}
	if len(p.SquarePrice) != 0 || calls != 0 {
		t.Fatalf("invalid square row stored: %v calls=%d", p.SquarePrice, calls)
	}
	form.Set("name", "x #a #b #c")
	if w := testenv.Serve(t, testenv.AuthedPOST(t, path, form, f.admin)); w.Code != http.StatusUnauthorized {
		t.Fatalf("hashtags status=%d", w.Code)
	}
	show := httptest.NewRequest(http.MethodGet, path, nil)
	testenv.LoggedIn(t, show, f.admin)
	if w := testenv.Serve(t, show); w.Code != http.StatusOK {
		t.Fatalf("update form status=%d", w.Code)
	}
	anonShow := testenv.Serve(t, httptest.NewRequest(http.MethodGet, path, nil))
	if anonShow.Code != http.StatusUnauthorized {
		t.Fatalf("anon update form status=%d", anonShow.Code)
	}
}

func TestCreateShowRequiresAuthorisation(t *testing.T) {
	f := setupHandlers(t, nil)
	if w := testenv.Serve(t, httptest.NewRequest(http.MethodGet, "/products/create", nil)); w.Code != http.StatusUnauthorized {
		t.Fatalf("anon status=%d", w.Code)
	}
	r := httptest.NewRequest(http.MethodGet, "/products/create", nil)
	testenv.LoggedIn(t, r, f.admin)
	if w := testenv.Serve(t, r); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "stripe-toggle") {
		t.Fatalf("admin status=%d", w.Code)
	}
}

func TestDestroyChecksTokenAndOwner(t *testing.T) {
	f := setupHandlers(t, nil)
	id := seedPricedProduct(t, "onetime", f.admin)
	path := fmt.Sprintf("/products/%d/destroy", id)
	if w := testenv.Serve(t, testenv.POST(path, url.Values{})); w.Code != http.StatusUnauthorized {
		t.Fatalf("tokenless destroy status=%d", w.Code)
	}
	if w := testenv.Serve(t, testenv.AuthedPOST(t, path, nil, f.reader)); w.Code != http.StatusUnauthorized {
		t.Fatalf("non-owner destroy status=%d", w.Code)
	}
	if n := testenv.Scalar(t, "SELECT COUNT(*) FROM products WHERE id=?", id); n != 1 {
		t.Fatal("product deleted without authorisation")
	}
	if w := testenv.Serve(t, testenv.AuthedPOST(t, path, nil, f.admin)); w.Code != http.StatusFound {
		t.Fatalf("admin destroy status=%d", w.Code)
	}
	if n := testenv.Scalar(t, "SELECT COUNT(*) FROM products WHERE id=?", id); n != 0 {
		t.Fatal("product not deleted")
	}
}

func showAs(t *testing.T, id, user int64, header map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/products/%d", id), nil)
	for k, v := range header {
		r.Header.Set(k, v)
	}
	if user > 0 {
		testenv.LoggedIn(t, r, user)
	}
	return testenv.Serve(t, r)
}

func TestShowHidesDraftAndSuspendedFromVisitors(t *testing.T) {
	f := setupHandlers(t, nil)
	for status, name := range map[int]string{1: "draft", 50: "suspended"} {
		id := testenv.SeedProduct(t, map[string]interface{}{"status": status, "name": "Hidden " + name})
		if w := showAs(t, id, 0, nil); w.Code != http.StatusNotFound {
			t.Errorf("%s anon status=%d", name, w.Code)
		}
		if w := showAs(t, id, f.admin, nil); w.Code != http.StatusOK {
			t.Errorf("%s admin status=%d", name, w.Code)
		}
	}
	if w := showAs(t, 999, 0, nil); w.Code != http.StatusNotFound {
		t.Errorf("missing product status=%d", w.Code)
	}
	id := testenv.SeedProduct(t, map[string]interface{}{"name": "Free thing"})
	w := showAs(t, id, 0, nil)
	if w.Code != http.StatusOK || strings.Contains(w.Body.String(), "_checkout") || strings.Contains(w.Body.String(), "checkout_button") {
		t.Fatalf("unpriced product status=%d shows checkout", w.Code)
	}
}

// stripePrices answers GET /v1/prices/{id} from the mocked Stripe backend.
func stripePrices(t *testing.T, interval string) {
	testenv.StripeBackend(t, func(r *http.Request) (*http.Response, error) {
		if !strings.HasPrefix(r.URL.Path, "/v1/prices/") {
			return nil, fmt.Errorf("unexpected stripe request %s", r.URL)
		}
		id := strings.TrimPrefix(r.URL.Path, "/v1/prices/")
		body := fmt.Sprintf(`{"id":%q,"object":"price","active":true,"currency":"usd","unit_amount":1050,"type":"one_time"}`, id)
		if interval != "" {
			body = fmt.Sprintf(`{"id":%q,"object":"price","active":true,"currency":"usd","unit_amount":1050,"type":"recurring","recurring":{"interval":%q,"interval_count":1}}`, id, interval)
		}
		return testenv.Response(http.StatusOK, body), nil
	})
}

// razorpayPlans answers Razorpay plan fetches through http.DefaultTransport.
func razorpayPlans(t *testing.T) {
	testenv.MockTransport(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.razorpay.com" || !strings.Contains(r.URL.Path, "/plans/") {
			return nil, fmt.Errorf("unexpected request %s", r.URL)
		}
		return testenv.Response(http.StatusOK, `{"id":"plan_R","period":"monthly","interval":1,"item":{"amount":49900,"currency":"INR"}}`), nil
	})
}

func TestShowPriceLabelPerGatewayAndSchedule(t *testing.T) {
	setupHandlers(t, nil)
	labels := map[string]map[string]string{
		"stripe":   {"onetime": "10.50 usd/One Time", "monthly": "10.50 usd/month", "yearly": "10.50 usd/year"},
		"square":   {"onetime": "10.50 USD/One Time", "monthly": "10.50 USD/Monthly", "yearly": "10.50 USD/Year"},
		"paypal":   {"onetime": "10.50 USD/One Time", "monthly": "10.50 USD/Monthly", "yearly": "10.50 USD/Year"},
		"razorpay": {"onetime": "123456.00 INR/One Time", "monthly": "499.00 INR/Monthly", "yearly": "499.00 INR/Yearly"},
	}
	prices := map[string]interface{}{
		"stripe":   map[string]string{"DF": "price_DF"},
		"square":   map[string]interface{}{"DF": map[string]interface{}{"amount": 1050, "currency": "USD"}},
		"paypal":   map[string]interface{}{"DF": map[string]interface{}{"amount": 10.5, "currency": "USD", "plan_id": "P-1"}},
		"razorpay": map[string]interface{}{"DF": map[string]interface{}{"amount": 123456, "currency": "INR", "plan_id": "plan_R"}},
	}
	for gateway, bySchedule := range labels {
		for _, schedule := range schedules {
			t.Run(gateway+"/"+schedule, func(t *testing.T) {
				interval := map[string]string{"monthly": "month", "yearly": "year"}[schedule]
				stripePrices(t, interval)
				razorpayPlans(t)
				id := testenv.SeedProduct(t, map[string]interface{}{"schedule": schedule, gateway + "_price": prices[gateway]})
				w := showAs(t, id, 0, nil)
				body := w.Body.String()
				if w.Code != http.StatusOK {
					t.Fatalf("status=%d body=%s", w.Code, body)
				}
				if !strings.Contains(body, bySchedule[schedule]) {
					t.Fatalf("label %q missing", bySchedule[schedule])
				}
				if gateway == "paypal" || gateway == "razorpay" {
					link := paymentEntryURL(gateway, id, "", "")
					if !strings.Contains(body, strings.ReplaceAll(link, "&", "&amp;")) {
						t.Fatalf("payment link %s missing", link)
					}
				}
			})
		}
	}
}

// Regression: plan-only and malformed PayPal/Razorpay entries panicked in
// amount.(float64) / currency.(string).
func TestShowMalformedPricesDoNotPanic(t *testing.T) {
	setupHandlers(t, nil)
	razorpayPlans(t)
	cases := []struct {
		name, schedule string
		cols           map[string]interface{}
		want           int
	}{
		{"razorpay onetime plan only", "onetime", map[string]interface{}{"razorpay_price": map[string]interface{}{"DF": map[string]interface{}{"plan_id": "plan_R"}}}, http.StatusInternalServerError},
		{"paypal onetime plan only", "onetime", map[string]interface{}{"paypal_price": map[string]interface{}{"DF": map[string]interface{}{"plan_id": "P-1"}}}, http.StatusInternalServerError},
		{"paypal amount string", "onetime", map[string]interface{}{"paypal_price": map[string]interface{}{"DF": map[string]interface{}{"amount": "ten", "currency": "USD"}}}, http.StatusInternalServerError},
		{"paypal currency number", "onetime", map[string]interface{}{"paypal_price": map[string]interface{}{"DF": map[string]interface{}{"amount": 10, "currency": 5}}}, http.StatusInternalServerError},
		{"square currency number", "onetime", map[string]interface{}{"square_price": map[string]interface{}{"DF": map[string]interface{}{"amount": 10, "currency": 5}}}, http.StatusInternalServerError},
		{"paypal monthly plan only", "monthly", map[string]interface{}{"paypal_price": map[string]interface{}{"DF": map[string]interface{}{"plan_id": "P-1"}}}, http.StatusOK},
		{"razorpay monthly plan only", "monthly", map[string]interface{}{"razorpay_price": map[string]interface{}{"DF": map[string]interface{}{"plan_id": "plan_R"}}}, http.StatusOK},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.cols["schedule"] = c.schedule
			id := testenv.SeedProduct(t, c.cols)
			if w := showAs(t, id, 0, nil); w.Code != c.want {
				t.Fatalf("status=%d want %d body=%s", w.Code, c.want, w.Body.String())
			}
		})
	}
}

func TestShowSelectsGatewayByCountry(t *testing.T) {
	setupHandlers(t, map[string]string{"subscription_client_country": "IN"})
	stripePrices(t, "")
	id := testenv.SeedProduct(t, map[string]interface{}{
		"stripe_price":   map[string]string{"DF": "price_DF"},
		"razorpay_price": map[string]interface{}{"IN": map[string]interface{}{"amount": 499, "currency": "INR"}},
	})
	if body := showAs(t, id, 0, nil).Body.String(); !strings.Contains(body, "razorpay_checkout") || strings.Contains(body, "checkout_button") {
		t.Fatal("country price did not win over stripe default")
	}
	testenv.Set("subscription_client_country", "US")
	if body := showAs(t, id, 0, nil).Body.String(); !strings.Contains(body, "checkout_button") {
		t.Fatal("stripe default not used for unlisted country")
	}
	// In production the Cloudflare country header decides.
	testenv.Production(t)
	if body := showAs(t, id, 0, map[string]string{"CF-IPCountry": "IN"}).Body.String(); !strings.Contains(body, "razorpay_checkout") {
		t.Fatal("CF-IPCountry ignored in production")
	}
	if body := showAs(t, id, 0, map[string]string{"CF-IPCountry": "FR"}).Body.String(); !strings.Contains(body, "checkout_button") {
		t.Fatal("production fallback to DF failed")
	}
}

func TestIndexAndSitemapRenderOnSQLite(t *testing.T) {
	f := setupHandlers(t, nil)
	for i := 0; i < 6; i++ {
		testenv.SeedProduct(t, map[string]interface{}{"name": fmt.Sprintf("Guide %d", i), "summary": "Learn 100% of it"})
	}
	testenv.SeedProduct(t, map[string]interface{}{"name": "Hidden draft", "status": 1})
	cases := []struct {
		path      string
		user      int64
		want, not []string
	}{
		{"/products", 0, []string{"Guide 5"}, []string{"Hidden draft"}},
		{"/products?q=GUIDE", 0, []string{"Guide 5"}, nil},
		{"/products?q=nomatch", 0, nil, []string{"Guide 5"}},
		{"/products?q=100%25", 0, []string{"Guide 5"}, nil},
		{"/products?q=_", 0, nil, []string{"Guide 5"}},
		{"/products", f.admin, []string{"Hidden draft"}, nil},
		{"/products.xml", 0, []string{"Guide 5"}, nil},
		{"/sitemap.xml", 0, []string{"/products/"}, nil},
	}
	for _, c := range cases {
		r := httptest.NewRequest(http.MethodGet, c.path, nil)
		if c.user > 0 {
			testenv.LoggedIn(t, r, c.user)
		}
		w := testenv.Serve(t, r)
		if w.Code != http.StatusOK {
			t.Errorf("%s status=%d body=%s", c.path, w.Code, w.Body.String())
			continue
		}
		for _, s := range c.want {
			if !strings.Contains(w.Body.String(), s) {
				t.Errorf("%s missing %q", c.path, s)
			}
		}
		for _, s := range c.not {
			if strings.Contains(w.Body.String(), s) {
				t.Errorf("%s unexpectedly contains %q", c.path, s)
			}
		}
	}
}

// Regression: the editor endpoints only checked CSRF, so anonymous visitors
// could spend the merchant's AI API key and upload files.
func TestEditorEndpointsRequireAuthorisation(t *testing.T) {
	f := setupHandlers(t, map[string]string{"palm_key": "palm-test-key"})
	testenv.Route(http.MethodPost, "/product/editor/suggestion", HandleGetSuggestion)
	testenv.Route(http.MethodPost, "/product/editor/upload", HandleFileAttachment)
	calls := 0
	testenv.MockTransport(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host != "generativelanguage.googleapis.com" || r.URL.Query().Get("key") != "palm-test-key" {
			return nil, fmt.Errorf("unexpected request %s", r.URL)
		}
		return testenv.Response(http.StatusOK, `{"candidates":[{"output":"**Great** product"}]}`), nil
	})
	for _, path := range []string{"/product/editor/suggestion", "/product/editor/upload"} {
		for name, user := range map[string]int64{"anon": 0} {
			if w := testenv.Serve(t, testenv.AuthedPOST(t, path, url.Values{"text": {"x"}}, user)); w.Code != http.StatusUnauthorized {
				t.Errorf("%s %s status=%d", name, path, w.Code)
			}
		}
	}
	if calls != 0 {
		t.Fatal("anonymous request reached the AI API")
	}
	w := testenv.Serve(t, testenv.AuthedPOST(t, "/product/editor/suggestion", url.Values{"text": {"Describe"}}, f.admin))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Great") || calls != 1 {
		t.Fatalf("admin suggestion: %d %s", w.Code, w.Body.String())
	}
	// A provider outage is an error, not a panic.
	testenv.MockTransport(t, func(r *http.Request) (*http.Response, error) { return nil, fmt.Errorf("offline") })
	if w := testenv.Serve(t, testenv.AuthedPOST(t, "/product/editor/suggestion", url.Values{"text": {"x"}}, f.admin)); w.Code != http.StatusInternalServerError {
		t.Fatalf("outage status=%d", w.Code)
	}
}
