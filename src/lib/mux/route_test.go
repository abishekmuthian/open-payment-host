package mux

import (
	"io"
	"net/http"
	"testing"
)

// a test handler
func handler(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	io.WriteString(w, "<h1>test</h1>")
	return nil
}

type testMatch struct {
	method  string
	path    string
	result  bool
	pattern string
	id      string
}

// Note matches allow arbitrary endings unless excluded by regexp
// this lets us have slugs for seo easily.
var getTests = []testMatch{
	{`GET`, `/`, true, `/`, ``},
	{`GET`, `/elephants`, true, `/elephants`, ``},
	{`GET`, `/elephants/foo`, false, `/elephants`, ``}, // fail due to different path
	{`GET`, `/dod/1`, false, `/dod/{\d+}`, ``},         // fail due to malformed regexp
	{`GET`, `/do`, true, `/do`, ``},
	{`GET`, `/dod/1`, true, `/dod/{id:\d+}`, ``},
	{`GET`, `/dod/1`, true, `/dod/{:\d+}`, ``},                  // nameless param allowed, should it be?
	{`GET`, `/dod/1-my-foo-pattern`, true, `/dod/{id:\d+}`, ``}, // note this matches even though we have extra at end of url
	{`GET`, `/pattern`, false, `/pattern/foo`, ``},
	{`GET`, `/users`, true, `/users`, ``},
	{`GET`, `/users/create`, true, `/users/create`, ``},
	{`GET`, `/users/1/show`, true, `/users/{id:\d+}/show`, ``},
	{`GET`, `/users/1/update`, true, `/users/{id:\d+}/update`, ``},
	{`GET`, `/users/1/destroy`, true, `/users/{id:\d+}/destroy`, ``},
	{`GET`, `/users/1/destro`, false, `/users/{id:\d+}/destroy`, ``},
	{`GET`, `/users/1`, true, `/users/{id:\d+}`, ``},
	{`GET`, `/users/1-slug-for-seo`, true, `/users/{id:\d+}`, ``},
	{`GET`, `/users/2342345`, true, `/users/{id:\d+}`, ``},
	{`GET`, `/test-wildcard`, true, `/{path:.*}`, ``},
}

// These tests should be run for EVERY type of route, work out how best to do this.
// Perhaps list of creation functions to run?

// TestNewRoute tests creating a route and testing matches (maybe and real)
func TestGetRoutes(t *testing.T) {

	for _, match := range getTests {
		r, err := NewRoute(match.pattern, handler)
		// Test pattern set but exclude intentionally bad
		if (err != nil) && match.pattern != `/dod/{\d+}` {
			t.Errorf("route: error creating route with pattern:%s", match.pattern)
		}

		// Test methods - allow empty method for get?
		if !r.MatchMethod("GET") || !r.MatchMethod("") || !r.MatchMethod("HEAD") {
			t.Errorf("route: doesn't match GET/HEAD")
		}

		if r.MatchMethod("POST") {
			t.Errorf("route: matches POST or empty method")
		}

		// Test maybe match - this may or may not match result but must be positive if match is positive
		if !r.MatchMaybe(match.path) && match.result {
			t.Errorf("route: %s does not match maybe expected match:%s", match.pattern, match.path)
		}

		// Test exact match
		if r.Match(match.path) != match.result {
			t.Errorf("route: %s does not match expected match:%s", match.pattern, match.path)
		}

		// Test matching model - for a good match all 3 should be true,
		// for a bad match at least last should be false
		result := r.MatchMethod(match.method) && r.MatchMaybe(match.path) && r.Match(match.path)
		if result != match.result {
			t.Errorf("route: %s does not match expected match expected:%v got:%v ", r, match.result, result)
		}

	}

}

// TestNaiveRoute tests creating a basic route
func TestNaiveRoute(t *testing.T) {

	r, err := NewNaiveRoute("/", handler)
	if err != nil {
		t.Error("route: error creating route")
	}

	// Test defaults
	if !r.MatchMethod(http.MethodHead) {
		t.Errorf("route: does not match method " + http.MethodHead)
	}
	if !r.MatchMethod(http.MethodGet) {
		t.Errorf("route: does not match method " + http.MethodHead)
	}

	if !r.MatchMaybe("/") {
		t.Errorf("route: does not match method " + http.MethodGet)
	}

	if r.(*NaiveRoute).Pattern() != "/" {
		t.Errorf("route: pattern incorrect")
	}

}

// TestWrongMethods tests creating a route
func TestMethods(t *testing.T) {

	r, err := NewRoute("/", handler)
	if err != nil {
		t.Error("route: error creating route")
	}

	// Test defaults
	if !r.MatchMethod(http.MethodHead) {
		t.Errorf("route: does not match method " + http.MethodHead)
	}
	if !r.MatchMethod(http.MethodGet) {
		t.Errorf("route: does not match method " + http.MethodHead)
	}

	// Test changing methods
	r = r.Get()
	if !r.MatchMethod(http.MethodGet) {
		t.Errorf("route: does not match method " + http.MethodGet)
	}
	r = r.Put()
	if !r.MatchMethod(http.MethodPut) {
		t.Errorf("route: does not match method " + http.MethodPut)
	}
	r = r.Delete()
	if !r.MatchMethod(http.MethodDelete) {
		t.Errorf("route: does not match method " + http.MethodDelete)
	}

	r.Methods(http.MethodGet)
	if !r.MatchMethod(http.MethodGet) {
		t.Errorf("route: does not match method " + http.MethodGet)
	}

	r = r.Methods(http.MethodDelete, http.MethodHead, http.MethodGet, http.MethodPost)
	if !r.MatchMethod(http.MethodHead) {
		t.Errorf("route: does not match method " + http.MethodHead)
	}

	if r.(*NaiveRoute).methods[0] != http.MethodDelete {
		t.Errorf("route: does not match methods %v", r)
	}

}

// TestWrongMethods tests creating a route
func TestWrongMethods(t *testing.T) {
	// Test other methods on a post route
	pattern := "/users"
	path := "/users"
	r, err := NewRoute(pattern, handler)
	if err != nil {
		t.Error("route: error creating route")
	}
	t.Logf("ROUTE:%s", r)

	// Set to post
	r.Post()
	if r.Handler() == nil {
		t.Errorf("route: handler not set")
	}

	// Check GET or HEAD doesn't work
	if r.MatchMethod("GET") || r.MatchMethod("") || r.MatchMethod("HEAD") || r.MatchMethod("FOOBAR") {
		t.Errorf("route: POST matches GET/HEAD")
	}

	if !r.MatchMethod("POST") {
		t.Errorf("route: POST does not match POST ")
	}

	// Test maybe match works for exact match string
	if !r.MatchMaybe(path) {
		t.Errorf("route: %s does not match maybe expected match for users.", r)
	}

	// Test maybe match maybe rejects very different path
	if r.MatchMaybe("&@#$%@#") || r.MatchMaybe("/FOOBAR/1/23BOO") || r.MatchMaybe("/asdfasdfasdfasdf/1/23") {
		t.Error("route: match maybe too vague", r)
	}

	// Test exact match path
	if !r.Match(path) {
		t.Errorf("route: %s does not match expected match for %s", r, path)
	}

}

var paramTests = []testMatch{
	{`GET`, `/`, true, `/`, ``},
	{`GET`, `/pa/ges/4/edit`, false, ``, ``},
	{`GET`, `/pages/3/edit`, true, ``, `3`},
	{`GET`, `/pages/3434234/edit`, true, ``, `3434234`},
	{`GET`, `/pages//edit`, false, ``, ``},
	{`GET`, `/pages/asdf/edit`, false, ``, ``},
	{`GET`, `/pages/0-234/edit`, false, ``, ``},
	{`GET`, `/pages/../0-234/edit`, false, ``, ``},
	{`GET`, `/pages/9999999999999/edit`, true, ``, `9999999999999`},
}

func TestRouteRegexpParsing(t *testing.T) {

	// Test parsing on incomplete route (nil)
	rr, _ := NewRoute("/", handler)
	urlParams := rr.Parse("")
	if len(urlParams) > 0 {
		t.Errorf("route: returned params for nil:%v", urlParams)
	}

	// Test bad patterns
	_, err := NewRoute("/{test:.*", handler)
	if err == nil {
		t.Errorf("route: bad route should not have been created")
	}

	// Test bad patterns
	_, err = NewRoute("/{{test:.*}{}}}}", handler)
	if err == nil {
		t.Errorf("route: bad route should not have been created")
	}

	// Test short patterns
	_, err = NewRoute("/{a:b|c}", handler)
	if err != nil {
		t.Errorf("route: short route %s", err)
	}
	// Test short patterns
	_, err = NewRoute("/a", handler)
	if err != nil {
		t.Errorf("route: short route %s", err)
	}

	// Test some routes against known pattern
	pattern := "/pages/{id:[0-9]+}/edit"
	r, err := NewRoute(pattern, handler)
	if err != nil {
		t.Errorf("route: error creating route")
	}

	// Parse params with route above
	for _, match := range paramTests {

		// Note we do NOT check match here
		urlParams := r.Parse(match.path)
		if match.result {
			// If we got a match check id
			if urlParams["id"] != match.id {
				t.Errorf("route: failed to match params got:%v", urlParams)
			}
		} else {
			// else we should have no good match, check no params
			if len(urlParams) > 0 {
				t.Errorf("route: %v failed to reject invalid params :%v", r, urlParams)
			}
		}

	}

}
