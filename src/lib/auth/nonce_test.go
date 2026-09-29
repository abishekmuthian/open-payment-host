package auth

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNonceTokenUniqueAndWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		nonce, err := NonceToken(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
		if err != nil {
			t.Fatal(err)
		}
		raw, err := base64.RawURLEncoding.DecodeString(nonce)
		if err != nil || len(raw) != 16 {
			t.Fatalf("nonce %q decodes to %d bytes: %v", nonce, len(raw), err)
		}
		if seen[nonce] {
			t.Fatalf("nonce repeated after %d calls", i)
		}
		seen[nonce] = true
	}
}

func withKeys(t *testing.T) {
	t.Helper()
	oldH, oldS, oldN := HMACKey, SecretKey, SessionName
	HMACKey = HexToBytes("abcdef0123456789abcdef0123456789")
	SecretKey = HexToBytes("0123456789abcdef0123456789abcdef")
	SessionName = "auth_test_session"
	t.Cleanup(func() { HMACKey, SecretKey, SessionName = oldH, oldS, oldN })
}

func TestCheckAuthenticityTokenRejections(t *testing.T) {
	withKeys(t)
	// No session cookie at all.
	if err := CheckAuthenticityToken("anything", httptest.NewRequest(http.MethodPost, "/", nil)); err == nil {
		t.Fatal("token accepted without session")
	}
	// Valid session with a token of the wrong length or wrong secret.
	w := httptest.NewRecorder()
	token, err := AuthenticityToken(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	for _, c := range w.Result().Cookies() {
		r.AddCookie(c)
	}
	if err := CheckAuthenticityToken(token, r); err != nil {
		t.Fatalf("valid token rejected: %v", err)
	}
	for _, bad := range []string{"", "short", token[:len(token)-4], BytesToBase64(RandomToken(TokenLength * 2))} {
		if err := CheckAuthenticityToken(bad, r); err == nil {
			t.Fatalf("bad token %q accepted", bad)
		}
	}
	// Keys missing: sessions cannot be read.
	HMACKey = nil
	if err := CheckAuthenticityToken(token, r); err == nil {
		t.Fatal("token accepted without keys")
	}
}

func TestSessionCookieTamperingAndExpiry(t *testing.T) {
	withKeys(t)
	store := &CookieSessionStore{values: map[string]string{SessionUserKey: "7"}}
	w := httptest.NewRecorder()
	if err := store.Save(w); err != nil {
		t.Fatal(err)
	}
	cookie := w.Result().Cookies()[0]
	if !cookie.HttpOnly || cookie.Path != "/" {
		t.Fatalf("cookie flags: %+v", cookie)
	}
	load := func(value string) (SessionStore, error) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.AddCookie(&http.Cookie{Name: SessionName, Value: value})
		return SessionGet(r)
	}
	if s, err := load(cookie.Value); err != nil || s.Get(SessionUserKey) != "7" {
		t.Fatalf("round trip failed: %v", err)
	}
	raw, _ := base64.URLEncoding.DecodeString(cookie.Value)
	tampered := []byte(string(raw))
	tampered[len(tampered)/2] ^= 1
	if s, err := load(base64.URLEncoding.EncodeToString(tampered)); err == nil || s.Get(SessionUserKey) != "" {
		t.Fatal("tampered cookie accepted")
	}
	if _, err := load(strings.Repeat("A", MaxCookieSize+1)); err == nil {
		t.Fatal("oversized cookie accepted")
	}
	oldMax := MaxAge
	MaxAge = -1 // every timestamp is now older than the maximum age
	t.Cleanup(func() { MaxAge = oldMax })
	if _, err := load(cookie.Value); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expired cookie accepted: %v", err)
	}
}
