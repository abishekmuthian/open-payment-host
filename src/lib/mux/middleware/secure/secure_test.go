package secure

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abishekmuthian/open-payment-host/src/lib/view"
)

func serve(t *testing.T) (*httptest.ResponseRecorder, string) {
	t.Helper()
	var nonce string
	h := Middleware(func(w http.ResponseWriter, r *http.Request) {
		nonce, _ = r.Context().Value(view.NonceContext).(string)
	})
	w := httptest.NewRecorder()
	h(w, httptest.NewRequest(http.MethodGet, "/", nil))
	return w, nonce
}

func TestSecurityHeaders(t *testing.T) {
	w, nonce := serve(t)
	csp := w.Header().Get("Content-Security-Policy")
	if nonce == "" || !strings.Contains(csp, "'nonce-"+nonce+"'") {
		t.Fatalf("nonce %q not in CSP %q", nonce, csp)
	}
	for header, want := range map[string]string{
		"Strict-Transport-Security": "max-age=63072000; includeSubDomains; preload",
		"X-Content-Type-Options":    "nosniff",
		"Referrer-Policy":           "strict-origin",
	} {
		if got := w.Header().Get(header); got != want {
			t.Errorf("%s = %q want %q", header, got, want)
		}
	}
	directives := map[string]string{}
	for _, d := range strings.Split(csp, ";") {
		fields := strings.Fields(d)
		if len(fields) > 0 {
			directives[fields[0]] = strings.Join(fields[1:], " ")
		}
	}
	if directives["frame-ancestors"] != "'self'" {
		t.Errorf("frame-ancestors = %q", directives["frame-ancestors"])
	}
	if strings.Contains(directives["script-src"], "'unsafe-inline'") || strings.Contains(directives["script-src"], "'unsafe-eval'") {
		t.Errorf("script-src allows unsafe code: %q", directives["script-src"])
	}
	// Gateway SDKs and Turnstile must be allowed where they load from.
	for directive, origins := range map[string][]string{
		"script-src":  {"challenges.cloudflare.com", "https://*.squarecdn.com", "https://*.paypal.com", "https://*.paypalobjects.com", "https://*.razorpay.com"},
		"frame-src":   {"challenges.cloudflare.com", "https://*.squarecdn.com", "https://*.paypal.com", "https://*.razorpay.com"},
		"connect-src": {"https://pci-connect.squareup.com", "https://*.paypal.com", "https://*.razorpay.com"},
	} {
		for _, origin := range origins {
			if !strings.Contains(" "+directives[directive]+" ", " "+origin+" ") {
				t.Errorf("%s missing %s", directive, origin)
			}
		}
	}
}

func TestNonceDiffersPerRequest(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		_, nonce := serve(t)
		if seen[nonce] {
			t.Fatalf("nonce %q reused", nonce)
		}
		seen[nonce] = true
	}
}

func TestHSTSMiddleware(t *testing.T) {
	w := httptest.NewRecorder()
	HSTSMiddleware(func(http.ResponseWriter, *http.Request) {})(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Header().Get("Strict-Transport-Security") == "" {
		t.Fatal("HSTS header missing")
	}
}
