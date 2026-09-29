package session

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/abishekmuthian/open-payment-host/src/internal/testenv"
	"github.com/abishekmuthian/open-payment-host/src/lib/auth"
	"github.com/abishekmuthian/open-payment-host/src/lib/view"
)

func TestShouldSetToken(t *testing.T) {
	cases := []struct {
		method, path string
		want         bool
	}{
		{http.MethodGet, "/", true},
		{http.MethodGet, "/products/1", true},
		{http.MethodGet, "/assets/scripts/app.js", false},
		{http.MethodGet, "/files/x.png", false},
		{http.MethodPost, "/users/login", false},
		{http.MethodHead, "/", false},
	}
	for _, c := range cases {
		if got := shouldSetToken(httptest.NewRequest(c.method, c.path, nil)); got != c.want {
			t.Errorf("%s %s = %v", c.method, c.path, got)
		}
	}
}

func TestMiddlewareAddsTokenToGetRequests(t *testing.T) {
	testenv.Config(t, nil)
	testenv.Auth(t)
	var token interface{}
	h := Middleware(func(w http.ResponseWriter, r *http.Request) { token = r.Context().Value(view.AuthenticityContext) })
	w := httptest.NewRecorder()
	h(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if s, _ := token.(string); s == "" || !strings.Contains(w.Header().Get("Set-Cookie"), auth.SessionName) {
		t.Fatalf("token=%v cookie=%q", token, w.Header().Get("Set-Cookie"))
	}
	token = nil
	h(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", nil))
	if token != nil {
		t.Fatal("token minted for POST")
	}
}

func TestCheckAuthenticity(t *testing.T) {
	testenv.Config(t, nil)
	testenv.Auth(t)
	testenv.Route(http.MethodPost, "/csrf-test", func(http.ResponseWriter, *http.Request) error { return nil })
	testenv.Route(http.MethodGet, "/csrf-test", func(http.ResponseWriter, *http.Request) error { return nil })

	if err := CheckAuthenticity(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/csrf-test", nil)); err != nil {
		t.Fatalf("GET checked: %v", err)
	}
	if err := CheckAuthenticity(httptest.NewRecorder(), testenv.AuthedPOST(t, "/csrf-test", nil, 0)); err != nil {
		t.Fatalf("valid token rejected: %v", err)
	}
	for name, token := range map[string]string{"missing": "", "garbage": "garbage", "foreign": ""} {
		r := testenv.AuthedPOST(t, "/csrf-test", nil, 7)
		if name == "foreign" {
			_, token = testenv.Session(t, 7) // valid token for another session
		}
		r = withToken(r, token)
		w := httptest.NewRecorder()
		if err := CheckAuthenticity(w, r); err == nil {
			t.Fatalf("%s token accepted", name)
		}
		if !strings.Contains(w.Header().Get("Set-Cookie"), "Max-Age=0") {
			t.Fatalf("%s: session not cleared: %q", name, w.Header().Get("Set-Cookie"))
		}
	}
	// No session at all.
	r := testenv.POST("/csrf-test", url.Values{"authenticity_token": {"x"}})
	if err := CheckAuthenticity(httptest.NewRecorder(), r); err == nil {
		t.Fatal("sessionless POST accepted")
	}
	// Explicit token variant.
	cookie, token := testenv.Session(t, 0)
	r = httptest.NewRequest(http.MethodPost, "/csrf-test", nil)
	r.AddCookie(cookie)
	if err := CheckAuthenticityToken(httptest.NewRecorder(), r, token); err != nil {
		t.Fatalf("explicit token rejected: %v", err)
	}
	if err := CheckAuthenticityToken(httptest.NewRecorder(), r, "bad"); err == nil {
		t.Fatal("explicit bad token accepted")
	}
}

// withToken replaces the authenticity_token form value of a POST.
func withToken(r *http.Request, token string) *http.Request {
	form := url.Values{"authenticity_token": {token}}
	n := httptest.NewRequest(http.MethodPost, r.URL.Path, strings.NewReader(form.Encode())).WithContext(context.Background())
	n.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range r.Cookies() {
		n.AddCookie(c)
	}
	return n
}

func TestCurrentUserRejectsTamperedCookie(t *testing.T) {
	testenv.Config(t, nil)
	testenv.Auth(t)
	testenv.DB(t)
	id := testenv.SeedUser(t, testenv.RoleAdmin, "admin@merchant.test", "password-1")
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	testenv.LoggedIn(t, r, id)
	if u := CurrentUser(httptest.NewRecorder(), r); u.ID != id || !u.Admin() {
		t.Fatalf("current user = %+v", u)
	}
	cookie, _ := r.Cookie(auth.SessionName)
	tampered := httptest.NewRequest(http.MethodGet, "/", nil)
	tampered.AddCookie(&http.Cookie{Name: auth.SessionName, Value: cookie.Value[:len(cookie.Value)-6] + "AAAAAA"})
	if u := CurrentUser(httptest.NewRecorder(), tampered); !u.Anon() {
		t.Fatalf("tampered cookie authenticated as %d", u.ID)
	}
	// A session for a deleted user is anonymous.
	testenv.Exec(t, "DELETE FROM users WHERE id=?", id)
	if u := CurrentUser(httptest.NewRecorder(), r); u == nil || u.ID != 0 || u.Admin() {
		t.Fatalf("deleted user authenticated: %+v", u)
	}
}
