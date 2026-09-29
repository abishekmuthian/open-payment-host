package stats

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/abishekmuthian/open-payment-host/src/internal/testenv"
)

// TestStats tests hits are counted per visitor and survive a purge.
func TestStats(t *testing.T) {
	testenv.Config(t, nil)
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("User-Agent", "stats-test-browser")
	c := UserCount()
	RegisterHit(r)
	newc := UserCount()
	if newc <= c {
		t.Errorf("Stats count incorrect")
	}
	RegisterHit(r) // same visitor
	if UserCount() != newc {
		t.Errorf("repeat visitor counted twice")
	}

	purgeUsers()

	if newc != UserCount() {
		t.Errorf("Stats count incorrect after purge")
	}

	for _, skip := range []*http.Request{httptest.NewRequest("GET", "/products.xml", nil), httptest.NewRequest("GET", "/", nil)} {
		skip.Header.Set("User-Agent", "Googlebot")
		RegisterHit(skip)
	}
	if UserCount() != newc {
		t.Errorf("bots or feeds counted")
	}

	w := httptest.NewRecorder()
	if err := HandleUserCount(w, r); err != nil || w.Code != http.StatusOK {
		t.Fatalf("user count handler: %v %d", err, w.Code)
	}
}

func TestStatsDoesNotPostWithoutAnalyticsURL(t *testing.T) {
	testenv.Config(t, map[string]string{"analytics_URL": ""})
	var calls int32
	testenv.MockTransport(t, func(r *http.Request) (*http.Response, error) {
		atomic.AddInt32(&calls, 1)
		return testenv.Response(http.StatusOK, "{}"), nil
	})
	r := httptest.NewRequest("GET", "/products/1", nil)
	r.Header.Set("User-Agent", "no-analytics-browser")
	RegisterHit(r)
	time.Sleep(50 * time.Millisecond)
	if n := atomic.LoadInt32(&calls); n != 0 {
		t.Fatalf("analytics posted %d times with no collector configured", n)
	}
}

func TestStatsPostsToConfiguredCollector(t *testing.T) {
	got := make(chan url.Values, 1)
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		v, _ := url.ParseQuery(string(body))
		got <- v
		_, _ = io.WriteString(w, "{}")
	}))
	defer collector.Close()
	testenv.Config(t, map[string]string{"analytics_URL": collector.URL, "analytics_property_id": "UA-TEST"})
	r := httptest.NewRequest("GET", "/products/1", nil)
	r.Header.Set("User-Agent", "analytics-browser")
	RegisterHit(r)
	select {
	case v := <-got:
		if v.Get("tid") != "UA-TEST" || v.Get("dp") != "/products/1" || v.Get("t") != "pageview" || v.Get("cid") == "" {
			t.Fatalf("payload=%v", v)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("collector not called")
	}
	// A malformed collector URL must not panic the background sender.
	testenv.Set("analytics_URL", "http://bad host/%zz")
	sendToGA("ua", "", "cid", url.Values{})
}
