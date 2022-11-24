package translation

import (
	"context"
	"net/http"
	"strings"

	//	"github.com/fragmenta/mux/log"
	"github.com/abishekmuthian/open-payment-host/src/lib/view"
)

// Middleware sets a token on every GET request so that it can be
// inserted into the view. It ignores get requests for /files and /assets.
func Middleware(h http.HandlerFunc) http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {

		// If a get method, we need to set the token for use in views
		if shouldSetToken(r) {
			// Decide on a language
			// First try the browser Accept-Language header
			lang := requestLang(r)

			// The cookie overrides the header
			cookieLang := cookieLang(r)
			if cookieLang != "" {
				lang = cookieLang
			}

			// Debug only - print the language chosen
			//log.Printf("translation: setting lang %s", lang)

			// Choose a default language if none set in either location
			if lang == "" {
				lang = DefaultLanguage
			}

			// Save the language to the request context for use in views
			ctx := r.Context()
			ctx = context.WithValue(ctx, view.LanguageContext, lang)
			r = r.WithContext(ctx)

		}

		h(w, r)
	}

}

// requestLang returns the language in the request Accept-Language header
// only the preferred first language is returned
// headers of form en-US,en;q=0.8,ro;q=0.6
// are turned into the en
func requestLang(r *http.Request) string {
	lang := r.Header.Get("Accept-Language")
	// en-US,en;q=0.8,ro;q=0.6 -> en-US
	if strings.Contains(lang, ",") {
		lang = strings.Split(lang, ",")[0]
	}
	// en-US -> en
	if strings.Contains(lang, "-") {
		lang = strings.Split(lang, "-")[0]
	}

	return lang
}

// cookieLang returns the language in the request cookie (if any)
func cookieLang(r *http.Request) string {
	c, err := r.Cookie("lang")
	if err != nil {
		return ""
	}
	return c.Value
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
