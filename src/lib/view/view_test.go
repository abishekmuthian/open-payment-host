package view

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {

	// Load will fail as there is no src
	err := LoadTemplates()
	if err == nil {
		t.Errorf("failed to warn on missing src")
	}

	// Load from test_data instead
	LoadTemplatesAtPaths([]string{"test_data"}, DefaultHelpers())

	// Print test templates just for information
	PrintTemplates()

	// Test reload
	ReloadTemplates()

	// Setup request and recorder
	r := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	// Try setting context for use later
	token := "1231324"
	ctx := r.Context()
	ctx = context.WithValue(ctx, AuthenticityContext, token)
	r = r.WithContext(ctx)

	v := NewRenderer(w, r)
	v.AddKey("url", "https://example.com")
	v.AddKey("class", "my class")
	v.AddKey("text", "hello world content")
	v.CacheKey("mykey")
	v.Template("template.html.got")
	v.Layout("")
	err = v.Render()
	if err != nil {
		t.Errorf("error rendering template:%s", err)
	}

	if !strings.Contains(w.Body.String(), "hello world content") {
		t.Errorf("error rendering template missing content")
	}

	s, err := v.RenderToString()
	if err != nil || !strings.Contains(s, "hello world content") {
		t.Errorf("error rendering template missing content")
	}

	if !strings.Contains(s, token) {
		t.Errorf("error rendering template missing content")
	}

}
