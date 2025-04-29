// Package secure adds headers to protect against xss and reflection attacks and force use of https
package secure

import (
	"context"
	"fmt"
	"net/http"

	"github.com/abishekmuthian/open-payment-host/src/lib/auth"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
	"github.com/abishekmuthian/open-payment-host/src/lib/view"
)

// These package level variables should be called if required to set policies before the middleware is added

// ContentSecurityPolicy defaults to a strict policy disallowing iframes and scripts from any other origin save self (and Google Analytics for scripts)
// var ContentSecurityPolicy = "frame-ancestors 'self'; connect-src 'self'; frame-src 'self' challenges.cloudflare.com; style-src 'self' 'unsafe-inline' esm.sh; script-src 'self' challenges.cloudflare.com esm.sh; img-src 'self'"

// Middleware adds some headers suitable for secure sites
func Middleware(h http.HandlerFunc) http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {
		// Nonce Implementation Test

		// This sets the nonce on the encrypted session cookie
		nonce, err := auth.NonceToken(w, r)
		if err != nil {
			log.Error(log.Values{"msg": "session: project setting nonce", "error": err})
		} else {
			// Save the token to the request context for use in views
			ctx := r.Context()
			ctx = context.WithValue(ctx, view.NonceContext, nonce)
			r = r.WithContext(ctx)
		}

		// Before Square
		// var ContentSecurityPolicy = fmt.Sprintf("frame-ancestors 'self'; connect-src 'self' https://pci-connect.squareupsandbox.com https://pci-connect.squareup.com; frame-src 'self' challenges.cloudflare.com https://sandbox.web.squarecdn.com; style-src 'self' 'unsafe-inline' 'unsafe-eval' https://unpkg.com/trix@2.0.0/dist/trix.css 'nonce-%s'; script-src 'self' challenges.cloudflare.com https://unpkg.com/trix@2.0.0/dist/trix.umd.min.js https://*.squarecdn.com https://js.squareupsandbox.com https://*.squarecdn.com https://js.squareup.com ; img-src 'self' data:", nonce)

		// After Square integration
		var ContentSecurityPolicy = fmt.Sprintf("frame-ancestors 'self'; connect-src 'self' https://*.s3.amazonaws.com https://pci-connect.squareupsandbox.com https://pci-connect.squareup.com https://api.squareupsandbox.com https://api.squareup.com https://*.paypal.com; frame-src 'self' challenges.cloudflare.com https://*.squarecdn.com https://*.squareupsandbox.com https://*.squareup.com https://*.ndsprod.nds-sandbox-issuer.com https://*.ndsprod.nds-issuer.com https://*.paypal.com; style-src 'self' 'unsafe-inline' https://*.squarecdn.com https://*.squareupsandbox.com https://*.squareup.com https://unpkg.com/trix@2.0.0/dist/trix.css https://*.paypal.com; script-src 'self' challenges.cloudflare.com https://unpkg.com/trix@2.0.0/dist/trix.umd.min.js https://*.squarecdn.com https://*.squareupsandbox.com https://*.squareup.com https://*.paypal.com https://*.paypalobjects.com 'nonce-%s'; img-src 'self' https://*.squarecdn.com https://*.squareupsandbox.com https://*.squareup.com https://*.paypal.com https://*.paypalobjects.com data:", nonce)

		// Add some headers for security

		// Allow no iframing - could also restrict scripts to this domain only (+GA?)
		w.Header().Set("Content-Security-Policy", ContentSecurityPolicy)

		// Allow only https connections for the next 2 years, requesting to be preloaded
		w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")

		// Set ReferrerPolicy explicitly to send only the domain, not the path
		w.Header().Set("Referrer-Policy", "strict-origin")

		// Ask browsers to block xss by default
		w.Header().Set("X-XSS-Protection", "1; mode=block")

		// Don't allow browser sniffing for content types
		w.Header().Set("X-Content-Type-Options", "nosniff")

		// Call the handler
		h(w, r)

	}
}

// HSTSMiddleware adds only the Strict-Transport-Security with a duration of 2 years
func HSTSMiddleware(h http.HandlerFunc) http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {

		// Allow only https connections for the next 2 years, requesting to be preloaded
		w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")

		// Call the handler
		h(w, r)

	}
}
