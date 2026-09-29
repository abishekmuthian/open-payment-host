package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
)

func TestClientIP(t *testing.T) {
	old := config.Current
	t.Cleanup(func() { config.Current = old })
	config.Current = &config.Config{Mode: config.ModeDevelopment}

	r := httptest.NewRequest(http.MethodPost, "/users/login", nil)
	r.RemoteAddr = "203.0.113.7:4321"
	r.Header.Set("X-Forwarded-For", "198.51.100.1")
	r.Header.Set("CF-Connecting-IP", "198.51.100.2")
	if got := ClientIP(r); got != "203.0.113.7" {
		t.Fatalf("development ip = %q", got)
	}
	r.RemoteAddr = "[2001:db8::1]:443"
	if got := ClientIP(r); got != "2001:db8::1" {
		t.Fatalf("ipv6 ip = %q", got)
	}
	r.RemoteAddr = "no-port"
	if got := ClientIP(r); got != "no-port" {
		t.Fatalf("portless ip = %q", got)
	}

	config.Current.Mode = config.ModeProduction
	r.RemoteAddr = "203.0.113.7:4321"
	if got := ClientIP(r); got != "198.51.100.2" {
		t.Fatalf("production ip = %q", got)
	}
	r.Header.Del("CF-Connecting-IP")
	if got := ClientIP(r); got != "203.0.113.7" {
		t.Fatalf("production fallback ip = %q", got)
	}
}
