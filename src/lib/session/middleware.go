package session

import (
	"context"
	"net/http"
	"strings"

	"github.com/abishekmuthian/open-payment-host/src/lib/auth"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
	"github.com/abishekmuthian/open-payment-host/src/lib/view"
)

// Middleware sets a token on every GET request so that it can be
// inserted into the view. It currently ignores requests for files and assets.
func Middleware(h http.HandlerFunc) http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {

		// If a get method, we need to set the token for use in views
		if shouldSetToken(r) {

			// This sets the token on the encrypted session cookie
			token, err := auth.AuthenticityToken(w, r)
			if err != nil {
				log.Error(log.Values{"msg": "session: project setting token", "error": err})
			} else {
				// Save the token to the request context for use in views
				ctx := r.Context()
				ctx = context.WithValue(ctx, view.AuthenticityContext, token)
				r = r.WithContext(ctx)
			}
		}

		h(w, r)
	}

}

// shouldSetToken returns true if this request requires a token set.
func shouldSetToken(r *http.Request) bool {

	// No tokens on anything but GET requests
	if r.Method != http.MethodGet {
		return false
	}

	// No tokens on non-html resources
	if strings.HasPrefix(r.URL.Path, "/files") ||
		strings.HasPrefix(r.URL.Path, "/assets") {
		return false
	}

	return true
}

// PasswordChangeMiddleware restricts a session that logged in with the default
// admin password to the password change page, logout, login and static assets.
// The login handler sets auth.SessionPasswordChangeKey; changing the password
// logs the user out, which clears it.
func PasswordChangeMiddleware(h http.HandlerFunc) http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {

		changePath, restricted := passwordChangePath(r)
		if !restricted || passwordChangeAllowed(r, changePath) {
			h(w, r)
			return
		}

		// htmx would swap a followed redirect into the page, so ask it to navigate
		if r.Header.Get("HX-Request") == "true" {
			w.Header().Set("HX-Redirect", changePath)
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Redirect(w, r, changePath, http.StatusFound)
	}

}

// passwordChangePath returns the password change page for the logged-in user
// when the session must change the default password.
func passwordChangePath(r *http.Request) (string, bool) {
	s, err := auth.SessionGet(r)
	if err != nil {
		return "", false
	}
	id := s.Get(auth.SessionPasswordChangeKey)
	if id == "" || id != s.Get(auth.SessionUserKey) {
		return "", false
	}
	return "/users/" + id + "/password/change", true
}

// passwordChangeAllowed reports whether a restricted session may make request r.
func passwordChangeAllowed(r *http.Request, changePath string) bool {
	p := r.URL.Path
	switch {
	case p == changePath || p == changePath+"/":
		return r.Method == http.MethodGet || r.Method == http.MethodPost
	case p == "/users/logout":
		return r.Method == http.MethodPost
	case p == "/users/login":
		return r.Method == http.MethodGet || r.Method == http.MethodPost
	case p == "/favicon.ico" || strings.HasPrefix(p, "/assets/"):
		return r.Method == http.MethodGet || r.Method == http.MethodHead
	}
	return false
}
