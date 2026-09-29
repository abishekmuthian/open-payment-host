package server

import (
	"net"
	"net/http"
	"strings"

	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
)

// ClientIP returns the client address used for rate limiting.
//
// In production the origin is assumed to be reachable only through
// Cloudflare, so CF-Connecting-IP is trusted. X-Forwarded-For is never used:
// clients can set it to rotate their identity.
func ClientIP(r *http.Request) string {
	if config.Production() {
		if ip := strings.TrimSpace(r.Header.Get("CF-Connecting-IP")); ip != "" {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
