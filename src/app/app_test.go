package app

import (
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/abishekmuthian/open-payment-host/src/internal/testenv"
	"github.com/abishekmuthian/open-payment-host/src/lib/auth"
	"github.com/abishekmuthian/open-payment-host/src/lib/auth/can"
	apphelpers "github.com/abishekmuthian/open-payment-host/src/lib/helpers"
	"github.com/abishekmuthian/open-payment-host/src/lib/mux"
	"github.com/abishekmuthian/open-payment-host/src/lib/view"
	"github.com/abishekmuthian/open-payment-host/src/users"
)

const adminPassword = "admin-test-password"

var (
	routerOnce sync.Once
	appRouter  *mux.Mux
)

// setupApp runs the real setup functions (assets, views, auth, routes and the
// default admin) against a temporary config, database and sandboxed root.
func setupApp(t *testing.T) *mux.Mux {
	t.Helper()
	testenv.Config(t, map[string]string{
		"admin_email":            "admin@merchant.test",
		"admin_default_password": adminPassword,
		"square_access_token":    "sq_token", "square_app_id": "sq_app", "square_location_id": "sq_loc",
		"palm_key": "palm-test-key",
	})
	testenv.DB(t)
	testenv.Sandbox(t)
	routerOnce.Do(func() {
		SetupAssets()
		SetupView()
		SetupAuth()
		appRouter = SetupRoutes()
	})
	// Rendering must not rescan templates from a sandbox that no longer exists.
	view.Production = true
	testenv.Auth(t)
	SetupDefaultUser()
	return appRouter
}

func serve(t *testing.T, r *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	return testenv.ServeWith(t, appRouter, r)
}

// TestAuth tests our authentication is functioning after setup.
func TestAuth(t *testing.T) {

	SetupAuth()

	user := &users.User{}

	// Test anon cannot access /users
	err := can.List(user, users.MockAnon())
	if err == nil {
		t.Fatalf("app: authentication block failed for anon")
	}

	// Test anon cannot edit admin user
	err = can.Update(users.MockAdmin(), users.MockAnon())
	if err == nil {
		t.Fatalf("app: authentication block failed for anon")
	}

	// Test admin can access /users
	err = can.List(user, users.MockAdmin())
	if err != nil {
		t.Fatalf("app: authentication failed for admin")
	}

	// Test admin can admin user
	err = can.Manage(user, users.MockAdmin())
	if err != nil {
		t.Fatalf("app: authentication failed for admin")
	}

}

func TestSetupDefaultUser(t *testing.T) {
	setupApp(t)
	admin, err := users.Find(1)
	if err != nil || !admin.Admin() || admin.Email != "admin@merchant.test" {
		t.Fatalf("default admin: %+v %v", admin, err)
	}
	SetupDefaultUser() // idempotent
	if n := testenv.Scalar(t, "SELECT COUNT(*) FROM users"); n != 1 {
		t.Fatalf("users=%d", n)
	}
}

// TestRoutes requests every route in routes.go as an anonymous visitor and as
// the admin (POSTs carry a valid CSRF token) and checks the status.
func TestRoutes(t *testing.T) {
	setupApp(t)
	testenv.SeedProduct(t, map[string]interface{}{"name": "Guide", "user_id": 1})
	testenv.SeedProduct(t, map[string]interface{}{"name": "Deletable", "user_id": 1})
	testenv.MockTransport(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "generativelanguage.googleapis.com" {
			return testenv.Response(200, `{"candidates":[{"output":"text"}]}`), nil
		}
		return nil, fmt.Errorf("blocked request to %s", r.URL.Host)
	})
	const anon, admin = 0, 1
	cases := []struct {
		method, path string
		form         url.Values
		want         [2]int // anonymous, admin
	}{
		{"GET", "/", nil, [2]int{200, 200}},
		{"GET", "/favicon.ico", nil, [2]int{200, 200}},
		{"GET", "/files/missing.txt", nil, [2]int{404, 404}},
		{"GET", "/assets/scripts/app.js", nil, [2]int{200, 200}},
		{"GET", "/assets/icons/favicon-16x16.png", nil, [2]int{200, 200}},
		{"GET", "/assets/scripts/missing.js", nil, [2]int{404, 404}},
		{"GET", "/index", nil, [2]int{200, 200}},
		{"GET", "/index.xml", nil, [2]int{200, 200}},
		{"GET", "/products/create", nil, [2]int{401, 200}},
		{"GET", "/products/create/price/0/stripe/any", nil, [2]int{200, 200}},
		{"GET", "/products/create/price/0/paypal/onetime", nil, [2]int{200, 200}},
		{"GET", "/products/create/schedule?schedule=monthly", nil, [2]int{200, 200}},
		{"GET", "/products/1/update", nil, [2]int{401, 200}},
		{"GET", "/products/1/subscription", nil, [2]int{302, 200}},
		{"GET", "/products/1", nil, [2]int{200, 200}},
		{"GET", "/products/999", nil, [2]int{404, 404}},
		{"GET", "/products", nil, [2]int{200, 200}},
		{"GET", "/products.xml", nil, [2]int{200, 200}},
		{"GET", "/sitemap.xml", nil, [2]int{200, 200}},
		{"GET", "/subscriptions/billing?productId=1&type=onetime", nil, [2]int{200, 200}},
		{"GET", "/subscriptions/square?productId=1&type=onetime", nil, [2]int{200, 200}},
		{"GET", "/subscriptions/paypal?product_id=1", nil, [2]int{400, 400}},
		{"GET", "/subscriptions/razorpay?product_id=1", nil, [2]int{400, 400}},
		{"GET", "/subscriptions/success?attempt_id=none", nil, [2]int{401, 401}},
		{"GET", "/subscriptions/payment-status?attempt_id=none", nil, [2]int{404, 404}},
		{"GET", "/subscriptions/stripe-success?session_id=none", nil, [2]int{401, 401}},
		{"GET", "/subscriptions/cancel?subscription_id=none&cancellation_token=x", nil, [2]int{401, 401}},
		{"GET", "/subscriptions/failure?errorDetail=Declined", nil, [2]int{200, 200}},
		{"GET", "/users/1/password/change", nil, [2]int{401, 200}},
		{"GET", "/users/login", nil, [2]int{200, 302}},
		{"POST", "/products/create", url.Values{"name": {"Created"}}, [2]int{401, 302}},
		{"POST", "/products/toggle/stripe", url.Values{"stripe-toggle": {"on"}, "schedule": {"onetime"}}, [2]int{401, 200}},
		{"POST", "/products/toggle/square", url.Values{"square-toggle": {"on"}, "schedule": {"onetime"}}, [2]int{401, 200}},
		{"POST", "/products/toggle/paypal", url.Values{"paypal-toggle": {"on"}, "schedule": {"monthly"}}, [2]int{401, 200}},
		{"POST", "/products/toggle/razorpay", url.Values{"razorpay-toggle": {"on"}, "schedule": {"yearly"}}, [2]int{401, 200}},
		{"POST", "/products/toggle/api", url.Values{"api-toggle": {"on"}}, [2]int{401, 200}},
		{"POST", "/products/1/toggle/stripe", url.Values{"stripe-toggle": {"on"}, "schedule": {"onetime"}}, [2]int{401, 200}},
		{"POST", "/products/1/toggle/square", url.Values{"square-toggle": {"on"}, "schedule": {"onetime"}}, [2]int{401, 200}},
		{"POST", "/products/1/toggle/paypal", url.Values{"paypal-toggle": {"on"}, "schedule": {"onetime"}}, [2]int{401, 200}},
		{"POST", "/products/1/toggle/razorpay", url.Values{"razorpay-toggle": {"on"}, "schedule": {"onetime"}}, [2]int{401, 200}},
		{"POST", "/products/1/toggle/api", url.Values{"api-toggle": {"on"}}, [2]int{401, 200}},
		{"POST", "/product/editor/suggestion", url.Values{"text": {"x"}}, [2]int{401, 200}},
		{"POST", "/product/editor/upload", nil, [2]int{401, 200}},
		{"POST", "/products/1/update", url.Values{"name": {"Guide"}, "schedule": {"onetime"}}, [2]int{401, 302}},
		{"POST", "/products/1/subscription/subscribe", nil, [2]int{401, 302}},
		{"POST", "/products/1/subscription/unsubscribe", nil, [2]int{401, 302}},
		{"POST", "/subscriptions/create-checkout-session", url.Values{"productId": {"1"}}, [2]int{302, 302}},
		{"POST", "/subscriptions/billing", url.Values{"productId": {"1"}}, [2]int{302, 302}},
		{"POST", "/subscriptions/square", url.Values{"productId": {"1"}}, [2]int{302, 302}},
		{"POST", "/subscriptions/paypal/orders", nil, [2]int{401, 401}},
		{"POST", "/subscriptions/paypal/orders/ABC/capture", nil, [2]int{401, 401}},
		{"POST", "/subscriptions/paypal/subscriptions", nil, [2]int{401, 401}},
		{"POST", "/subscriptions/subscribe", url.Values{"productId": {"1"}}, [2]int{400, 400}},
		{"POST", "/subscriptions/cancel", url.Values{"subscription_id": {"none"}}, [2]int{401, 401}},
		{"POST", "/subscriptions/cancellation-link", url.Values{"subscription_id": {"none"}}, [2]int{401, 404}},
		{"POST", "/subscriptions/stripe-webhook", nil, [2]int{503, 503}},
		{"POST", "/subscriptions/square-webhook", nil, [2]int{403, 403}},
		{"POST", "/subscriptions/paypal-webhook", nil, [2]int{503, 503}},
		{"POST", "/subscriptions/razorpay-webhook", nil, [2]int{403, 403}},
		{"POST", "/users/1/password/change", url.Values{"password": {"x"}, "password-confirm": {"x"}}, [2]int{401, 302}},
		{"POST", "/users/login", nil, [2]int{302, 302}},
		{"POST", "/users/logout", nil, [2]int{302, 302}},
		{"POST", "/products/2/destroy", nil, [2]int{401, 302}},
	}
	for _, c := range cases {
		for i, user := range []int64{anon, admin} {
			var r *http.Request
			if c.method == "GET" {
				r = httptest.NewRequest(http.MethodGet, c.path, nil)
				if user > 0 {
					testenv.LoggedIn(t, r, user)
				}
			} else {
				form := url.Values{}
				for k, v := range c.form {
					form[k] = v
				}
				r = testenv.AuthedPOST(t, c.path, form, user)
			}
			w := serve(t, r)
			if w.Code != c.want[i] || w.Code == http.StatusInternalServerError {
				t.Errorf("%s %s as %s: status=%d want %d", c.method, c.path, []string{"anon", "admin"}[i], w.Code, c.want[i])
			}
		}
	}
}

func TestHomePagination(t *testing.T) {
	setupApp(t)
	for i := 0; i < 12; i++ {
		testenv.SeedProduct(t, map[string]interface{}{"name": fmt.Sprintf("Product %02d", i), "points": 12 - i})
	}
	testenv.SeedProduct(t, map[string]interface{}{"name": "Secret draft", "status": 1, "points": 99})
	testenv.SeedProduct(t, map[string]interface{}{"name": "Suspended one", "status": 50, "points": 98})

	full := serve(t, httptest.NewRequest(http.MethodGet, "/", nil))
	body := full.Body.String()
	if full.Code != 200 || !strings.Contains(body, "<html") || !strings.Contains(body, `id="products-container"`) || !strings.Contains(body, "Product 00") {
		t.Fatalf("full page status=%d", full.Code)
	}
	if !strings.Contains(body, `hx-get="/?page=1"`) {
		t.Fatal("first page lacks Load More")
	}
	// Regression: drafts and suspended products were listed on the home page.
	if strings.Contains(body, "Secret draft") || strings.Contains(body, "Suspended one") {
		t.Fatal("hidden products listed for visitors")
	}
	admin := httptest.NewRequest(http.MethodGet, "/", nil)
	testenv.LoggedIn(t, admin, 1)
	if !strings.Contains(serve(t, admin).Body.String(), "Secret draft") {
		t.Fatal("admin cannot see drafts on the home page")
	}

	partial := httptest.NewRequest(http.MethodGet, "/?page=1", nil)
	partial.Header.Set("HX-Request", "true")
	w := serve(t, partial)
	body = w.Body.String()
	if w.Code != 200 || strings.Contains(body, "<html") || !strings.Contains(body, "Product 11") || strings.Contains(body, "Product 00") {
		t.Fatalf("partial page status=%d body=%s", w.Code, body)
	}
	if !strings.Contains(body, `id="load-more-container" hx-swap-oob="true"></div>`) {
		t.Fatal("last page did not remove Load More with an out-of-band swap")
	}
	first := httptest.NewRequest(http.MethodGet, "/?page=0", nil)
	first.Header.Set("HX-Request", "true")
	if body := serve(t, first).Body.String(); !strings.Contains(body, `hx-swap-oob="true"`) || !strings.Contains(body, `hx-get="/?page=1"`) {
		t.Fatal("partial first page lacks the next Load More button")
	}
}

func TestSecurityHeadersAndCSRF(t *testing.T) {
	setupApp(t)
	w := serve(t, httptest.NewRequest(http.MethodGet, "/", nil))
	for _, h := range []string{"Content-Security-Policy", "Strict-Transport-Security", "X-Content-Type-Options", "Referrer-Policy"} {
		if w.Header().Get(h) == "" {
			t.Errorf("missing %s", h)
		}
	}
	// The page's csp-nonce meta tag (used by the PayPal SDK) is the CSP nonce.
	csp := w.Header().Get("Content-Security-Policy")
	m := regexp.MustCompile(`'nonce-([^']+)'`).FindStringSubmatch(csp)
	if m == nil || !strings.Contains(w.Body.String(), `<meta content="`+m[1]+`" name="csp-nonce">`) {
		t.Fatal("rendered nonce does not match CSP")
	}
	// The layout exposes the CSRF token for HTMX and forms.
	if !regexp.MustCompile(`<meta content="[^"]+" name="authenticity_token">`).MatchString(w.Body.String()) {
		t.Fatal("authenticity token meta tag missing")
	}
	// A POST without a token is refused and the session cleared.
	r := testenv.POST("/users/logout", url.Values{})
	testenv.LoggedIn(t, r, 1)
	w = serve(t, r)
	if w.Code != http.StatusUnauthorized || !strings.Contains(w.Header().Get("Set-Cookie"), "Max-Age=0") {
		t.Fatalf("tokenless POST: %d %q", w.Code, w.Header().Get("Set-Cookie"))
	}
}

// TestDefaultPasswordChangeEnforced checks that a session which logged in with
// the default admin password can only reach the password change page.
// Regression: the change was only a redirect after login.
func TestDefaultPasswordChangeEnforced(t *testing.T) {
	setupApp(t)
	w := httptest.NewRecorder()
	s, err := auth.Session(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if err != nil {
		t.Fatal(err)
	}
	s.Set(auth.SessionUserKey, "1")
	s.Set(auth.SessionPasswordChangeKey, "1")
	if err := s.Save(w); err != nil {
		t.Fatal(err)
	}
	cookie := w.Result().Cookies()[0]
	get := func(path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		r.AddCookie(cookie)
		return serve(t, r)
	}
	const change = "/users/1/password/change"
	for _, path := range []string{"/", "/products/create", "/products/1/update"} {
		if w := get(path); w.Code != http.StatusFound || w.Header().Get("Location") != change {
			t.Errorf("%s: %d %q", path, w.Code, w.Header().Get("Location"))
		}
	}
	for _, path := range []string{change, "/assets/scripts/app.js"} {
		if w := get(path); w.Code != http.StatusOK {
			t.Errorf("%s: %d", path, w.Code)
		}
	}
}

func TestStaticFilesAndCacheHeaders(t *testing.T) {
	setupApp(t)
	for _, path := range []string{"/assets/scripts/app.js", "/favicon.ico"} {
		w := serve(t, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != 200 || w.Header().Get("Cache-Control") != "" {
			t.Fatalf("%s dev: %d %q", path, w.Code, w.Header().Get("Cache-Control"))
		}
	}
	testenv.Production(t)
	for _, path := range []string{"/assets/scripts/app.js", "/assets/icons/favicon-16x16.png"} {
		w := serve(t, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != 200 || w.Header().Get("Cache-Control") != "max-age=2592000" || w.Header().Get("ETag") == "" || w.Header().Get("Expires") == "" {
			t.Fatalf("%s production: %d %v", path, w.Code, w.Header())
		}
	}
	for _, path := range []string{"/assets/../secrets/fragmenta.json", "/files/../../go.mod", "/assets/nope.css"} {
		if w := serve(t, httptest.NewRequest(http.MethodGet, path, nil)); w.Code != http.StatusNotFound {
			t.Errorf("%s status=%d", path, w.Code)
		}
	}
	w := httptest.NewRecorder()
	if err := serveAsset(w, httptest.NewRequest(http.MethodGet, "/files/x.js", nil)); err == nil {
		t.Fatal("serveAsset served outside /assets")
	}
}

func TestTemplatesParse(t *testing.T) {
	testenv.Root(t)
	// The same helpers as SetupView and SetupAssets register.
	helpers := view.DefaultHelpers()
	helpers["markup"] = apphelpers.Markup
	helpers["timeago"] = apphelpers.TimeAgo
	helpers["root_url"] = apphelpers.RootURL
	if err := view.LoadTemplatesAtPaths([]string{"src"}, helpers); err != nil {
		t.Fatalf("templates do not parse: %v", err)
	}
}

// TestTemplatesAreCSPCompatible guards the nonce-based CSP: inline scripts must
// carry the nonce, external scripts must come from allowed origins, and inline
// event handler attributes are forbidden (use hyperscript instead).
func TestTemplatesAreCSPCompatible(t *testing.T) {
	root := testenv.Root(t)
	scriptTag := regexp.MustCompile(`(?is)<script\b([^>]*)>`)
	handler := regexp.MustCompile(`(?i)<[a-z][^>]*\son[a-z]+\s*=`)
	allowedSrc := regexp.MustCompile(`^(\{\{|/|https://(challenges\.cloudflare\.com|[a-z.]*squarecdn\.com|[a-z.]*paypal\.com|[a-z.]*paypalobjects\.com|[a-z.]*razorpay\.com)/)`)
	srcAttr := regexp.MustCompile(`(?i)\bsrc\s*=\s*"([^"]*)"`)
	count := 0
	err := filepath.WalkDir(filepath.Join(root, "src"), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".got") {
			return err
		}
		count++
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		for _, m := range scriptTag.FindAllStringSubmatch(string(b), -1) {
			attrs := m[1]
			if src := srcAttr.FindStringSubmatch(attrs); src != nil {
				if !allowedSrc.MatchString(src[1]) {
					t.Errorf("%s: script from origin not allowed by CSP: %s", rel, src[1])
				}
			} else if !strings.Contains(attrs, "nonce") && !strings.Contains(attrs, "application/ld+json") {
				t.Errorf("%s: inline <script%s> without nonce", rel, attrs)
			}
		}
		if loc := handler.FindStringIndex(string(b)); loc != nil {
			t.Errorf("%s: inline event handler %q", rel, string(b)[loc[0]:loc[1]])
		}
		return nil
	})
	if err != nil || count < 50 {
		t.Fatalf("walked %d templates: %v", count, err)
	}
}
