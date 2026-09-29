package session

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/abishekmuthian/open-payment-host/src/internal/testenv"
	"github.com/abishekmuthian/open-payment-host/src/lib/auth"
)

// sessionCookie returns a session cookie holding the given values.
func sessionCookie(t *testing.T, values map[string]string) *http.Cookie {
	t.Helper()
	w := httptest.NewRecorder()
	s, err := auth.Session(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range values {
		s.Set(k, v)
	}
	if err := s.Save(w); err != nil {
		t.Fatal(err)
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == auth.SessionName {
			return c
		}
	}
	t.Fatal("no session cookie")
	return nil
}

func TestPasswordChangeMiddleware(t *testing.T) {
	testenv.Config(t, nil)
	testenv.Auth(t)
	restricted := sessionCookie(t, map[string]string{auth.SessionUserKey: "7", auth.SessionPasswordChangeKey: "7"})
	cleared := sessionCookie(t, map[string]string{auth.SessionUserKey: "7", auth.SessionPasswordChangeKey: ""})
	// A flag for another user (e.g. left over before logging in as someone else) is ignored.
	stale := sessionCookie(t, map[string]string{auth.SessionUserKey: "8", auth.SessionPasswordChangeKey: "7"})

	const change = "/users/7/password/change"
	cases := []struct {
		name         string
		cookie       *http.Cookie
		method, path string
		htmx         bool
		reached      bool
	}{
		{"change page", restricted, http.MethodGet, change, false, true},
		{"change page error redirect", restricted, http.MethodGet, change + "/", false, true},
		{"change submit", restricted, http.MethodPost, change, false, true},
		{"logout", restricted, http.MethodPost, "/users/logout", false, true},
		{"login", restricted, http.MethodPost, "/users/login", false, true},
		{"asset", restricted, http.MethodGet, "/assets/scripts/app.js", false, true},
		{"favicon", restricted, http.MethodGet, "/favicon.ico", false, true},
		{"home", restricted, http.MethodGet, "/", false, false},
		{"product create", restricted, http.MethodGet, "/products/create", false, false},
		{"other user change page", restricted, http.MethodGet, "/users/8/password/change", false, false},
		{"cancellation link", restricted, http.MethodPost, "/subscriptions/cancellation-link", false, false},
		{"htmx toggle", restricted, http.MethodPost, "/products/1/toggle/api", true, false},
		{"changed password", cleared, http.MethodGet, "/products/create", false, true},
		{"stale flag", stale, http.MethodGet, "/products/create", false, true},
		{"anonymous", nil, http.MethodPost, "/subscriptions/stripe-webhook", false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			reached := false
			h := PasswordChangeMiddleware(func(w http.ResponseWriter, r *http.Request) { reached = true })
			r := httptest.NewRequest(c.method, c.path, nil)
			if c.cookie != nil {
				r.AddCookie(c.cookie)
			}
			if c.htmx {
				r.Header.Set("HX-Request", "true")
			}
			w := httptest.NewRecorder()
			h(w, r)
			if reached != c.reached {
				t.Fatalf("reached=%v want %v (status %d)", reached, c.reached, w.Code)
			}
			if c.reached {
				return
			}
			if c.htmx {
				if w.Code != http.StatusOK || w.Header().Get("HX-Redirect") != change {
					t.Fatalf("htmx status=%d HX-Redirect=%q", w.Code, w.Header().Get("HX-Redirect"))
				}
				return
			}
			if w.Code != http.StatusFound || w.Header().Get("Location") != change {
				t.Fatalf("status=%d location=%q", w.Code, w.Header().Get("Location"))
			}
		})
	}
}
