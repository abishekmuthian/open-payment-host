package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRedirectOnlyAllowsLocalPaths(t *testing.T) {
	cases := []struct {
		path string
		ok   bool
	}{
		{"/ok", true},
		{"/users/1/password/change", true},
		{"/?warn=already_logged_in#top", true},
		{"//evil.test", false},
		{"//evil.test/path", false},
		{"/\\evil.test", false},
		{"/\\\\evil.test", false},
		{"/search?q=a\\b", true},
		{"/\t/evil.test", false},
		{"/\n/evil.test", false},
		{"http://x", false},
		{"https://evil.test/", false},
		{"javascript:alert(1)", false},
		{"relative/path", false},
		{"", false},
	}
	for _, c := range cases {
		w := httptest.NewRecorder()
		err := Redirect(w, httptest.NewRequest(http.MethodGet, "/", nil), c.path)
		if c.ok {
			if err != nil || w.Code != http.StatusFound || w.Header().Get("Location") == "" {
				t.Errorf("%q: err=%v code=%d", c.path, err, w.Code)
			}
			continue
		}
		if err == nil || w.Header().Get("Location") != "" {
			t.Errorf("%q: redirect allowed (location %q)", c.path, w.Header().Get("Location"))
		}
		if IsLocalPath(c.path) {
			t.Errorf("IsLocalPath(%q) = true", c.path)
		}
	}
	w := httptest.NewRecorder()
	if err := RedirectStatus(w, httptest.NewRequest(http.MethodGet, "/", nil), "/moved", http.StatusMovedPermanently); err != nil || w.Code != http.StatusMovedPermanently {
		t.Fatalf("status redirect: %v %d", err, w.Code)
	}
}
