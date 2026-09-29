package helpers_test

import (
	"strings"
	"testing"
	"time"

	"github.com/abishekmuthian/open-payment-host/src/internal/testenv"
	"github.com/abishekmuthian/open-payment-host/src/lib/helpers"
)

// TestRootURL tests our root url loaded
func TestRootURL(t *testing.T) {
	testenv.Config(t, map[string]string{"root_url": "https://shop.test"})
	if got := helpers.RootURL(); got != "https://shop.test" {
		t.Errorf("helpers: root url = %q", got)
	}
}

func TestMarkup(t *testing.T) {
	cases := []struct{ in, want, not string }{
		{" @kenny ", "<a href=", ""},
		{"visit https://example.com now", `href="https://example.com"`, ""},
		{"line one\nline two", "<p>", ""},
		{`<script>alert(1)</script>hello`, "hello", "<script"},
		{`<a href="javascript:alert(1)">x</a>`, "x", "javascript:"},
	}
	for _, c := range cases {
		got := string(helpers.Markup(c.in))
		if !strings.Contains(got, c.want) || (c.not != "" && strings.Contains(got, c.not)) {
			t.Errorf("Markup(%q) = %q", c.in, got)
		}
	}
}

func TestTimeAgo(t *testing.T) {
	now := time.Now()
	cases := map[time.Duration]string{
		10 * time.Second: "10 seconds ago",
		5 * time.Minute:  "5 minutes ago",
		time.Hour:        "1 hour ago",
		3 * time.Hour:    "3 hours ago",
		30 * time.Hour:   "1 day ago",
		72 * time.Hour:   "3 days ago",
	}
	for d, want := range cases {
		if got := helpers.TimeAgo(now.Add(-d)); got != want {
			t.Errorf("TimeAgo(-%v) = %q want %q", d, got, want)
		}
	}
}
