package mux

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

var routes = []string{
	`/`,
	`/pages`,
	`/pages/create`,
	`/pages/{id:\d+}/update`,
	`/pages/{id:\d+}/destroy`,
	`/pages/{id:\d+}`,
	`/users`,
	`/users/foobar`,
	`/users/create`,
	`/users/{id:\d+}/update`,
	`/users/{id:\d+}/destroy`,
	`/users/{id:\d+}`,
	`/elephants`,
	`/elephants/create`,
	`/elephants/{id:\d+}/update`,
	`/elephants/{id:\d+}/destroy`,
	`/elephants/{id:\d+}`,
	`/foo`,
	`/foo/create`,
	`/foo/{id:\d+}/update`,
	`/foo/{id:\d+}/destroy`,
	`/foo/{id:\d+}`,
	`/dl`,
	`/dl/create`,
	`/dl/{id:\d+}/update`,
	`/dl/{id:\d+}/destroy`,
	`/dl/{id:\d+}`,
	`/pages`,
	`/pages/create`,
	`/pages/{id:\d+}/update`,
	`/pages/{id:\d+}/destroy`,
	`/pages/{id:\d+}`,
	`/users`,
	`/users/foobar`,
	`/users/create`,
	`/users/{id:\d+}/update`,
	`/users/{id:\d+}/destroy`,
	`/users/{id:\d+}`,
	`/elephants`,
	`/elephants/create`,
	`/elephants/{id:\d+}/update`,
	`/elephants/{id:\d+}/destroy`,
	`/elephants/{id:\d+}`,
	`/foo`,
	`/foo/create`,
	`/foo/{id:\d+}/update`,
	`/foo/{id:\d+}/destroy`,
	`/foo/{id:\d+}`,
	`/dl`,
	`/dl/create`,
	`/dl/{id:\d+}/update`,
	`/dl/{id:\d+}/destroy`,
	`/dl/{id:\d+}`,
	`/pages`,
	`/pages/create`,
	`/pages/{id:\d+}/update`,
	`/pages/{id:\d+}/destroy`,
	`/pages/{id:\d+}`,
	`/users`,
	`/users/foobar`,
	`/users/create`,
	`/users/{id:\d+}/update`,
	`/users/{id:\d+}/destroy`,
	`/users/{id:\d+}`,
	`/elephants`,
	`/elephants/create`,
	`/elephants/{id:\d+}/update`,
	`/elephants/{id:\d+}/destroy`,
	`/elephants/{id:\d+}`,
	`/foo`,
	`/foo/create`,
	`/foo/{id:\d+}/update`,
	`/foo/{id:\d+}/destroy`,
	`/foo/{id:\d+}`,
	`/dl`,
	`/dl/create`,
	`/dl/{id:\d+}/update`,
	`/dl/{id:\d+}/destroy`,
	`/dl/{id:\d+}`,
	`/pages`,
	`/pages/create`,
	`/pages/{id:\d+}/update`,
	`/pages/{id:\d+}/destroy`,
	`/pages/{id:\d+}`,
	`/users`,
	`/users/foobar`,
	`/users/create`,
	`/users/{id:\d+}/update`,
	`/users/{id:\d+}/destroy`,
	`/users/{id:\d+}`,
	`/elephants`,
	`/elephants/create`,
	`/elephants/{id:\d+}/update`,
	`/elephants/{id:\d+}/destroy`,
	`/elephants/{id:\d+}`,
	`/foo`,
	`/foo/create`,
	`/foo/{id:\d+}/update`,
	`/foo/{id:\d+}/destroy`,
	`/foo/{id:\d+}`,
	`/dl`,
	`/dl/create`,
	`/dl/{id:\d+}/update`,
	`/dl/{id:\d+}/destroy`,
	`/dl/{id:\d+}`,
}

// logMiddleware adds logging to requests (before and after)
func logMiddleware(h http.HandlerFunc) http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {
		println("before:%s", r.URL.Path)
		start := time.Now()
		h(w, r)
		println("after:%v", time.Now().Sub(start))
	}

}

// Test the mux matches
func TestMux(t *testing.T) {
	m := New()

	m.AddMiddleware(logMiddleware)

	// Add routes - sending everything to the same test handler
	for _, p := range routes {
		m.Add(p, handler)
	}

	// Match all the routes with a GET request
	for _, p := range routes {
		path := strings.Replace(p, `{id:\d+}`, "99", -1)
		req := httptest.NewRequest("GET", path, nil)
		r := m.Match(req)
		if r == nil {
			t.Errorf("error parsing route:%s", p)
		}
	}

	// TODO: test RouteRequest separately

}

// a structure for testing only
type route struct {
	m string
	p string
}

// Taken from httprouter tests for comparison
var staticRoutes = []route{
	{"GET", "/"},
	{"GET", "/cmd.html"},
	{"GET", "/code.html"},
	{"GET", "/contrib.html"},
	{"GET", "/contribute.html"},
	{"GET", "/debugging_with_gdb.html"},
	{"GET", "/docs.html"},
	{"GET", "/effective_go.html"},
	{"GET", "/files.log"},
	{"GET", "/gccgo_contribute.html"},
	{"GET", "/gccgo_install.html"},
	{"GET", "/go-logo-black.png"},
	{"GET", "/go-logo-blue.png"},
	{"GET", "/go-logo-white.png"},
	{"GET", "/go1.1.html"},
	{"GET", "/go1.2.html"},
	{"GET", "/go1.html"},
	{"GET", "/go1compat.html"},
	{"GET", "/go_faq.html"},
	{"GET", "/go_mem.html"},
	{"GET", "/go_spec.html"},
	{"GET", "/help.html"},
	{"GET", "/ie.css"},
	{"GET", "/install-source.html"},
	{"GET", "/install.html"},
	{"GET", "/logo-153x55.png"},
	{"GET", "/Makefile"},
	{"GET", "/root.html"},
	{"GET", "/share.png"},
	{"GET", "/sieve.gif"},
	{"GET", "/tos.html"},
	{"GET", "/articles/"},
	{"GET", "/articles/go_command.html"},
	{"GET", "/articles/index.html"},
	{"GET", "/articles/wiki/"},
	{"GET", "/articles/wiki/edit.html"},
	{"GET", "/articles/wiki/final-noclosure.go"},
	{"GET", "/articles/wiki/final-noerror.go"},
	{"GET", "/articles/wiki/final-parsetemplate.go"},
	{"GET", "/articles/wiki/final-template.go"},
	{"GET", "/articles/wiki/final.go"},
	{"GET", "/articles/wiki/get.go"},
	{"GET", "/articles/wiki/http-sample.go"},
	{"GET", "/articles/wiki/index.html"},
	{"GET", "/articles/wiki/Makefile"},
	{"GET", "/articles/wiki/notemplate.go"},
	{"GET", "/articles/wiki/part1-noerror.go"},
	{"GET", "/articles/wiki/part1.go"},
	{"GET", "/articles/wiki/part2.go"},
	{"GET", "/articles/wiki/part3-errorhandling.go"},
	{"GET", "/articles/wiki/part3.go"},
	{"GET", "/articles/wiki/test.bash"},
	{"GET", "/articles/wiki/test_edit.good"},
	{"GET", "/articles/wiki/test_Test.txt.good"},
	{"GET", "/articles/wiki/test_view.good"},
	{"GET", "/articles/wiki/view.html"},
	{"GET", "/codewalk/"},
	{"GET", "/codewalk/codewalk.css"},
	{"GET", "/codewalk/codewalk.js"},
	{"GET", "/codewalk/codewalk.xml"},
	{"GET", "/codewalk/functions.xml"},
	{"GET", "/codewalk/markov.go"},
	{"GET", "/codewalk/markov.xml"},
	{"GET", "/codewalk/pig.go"},
	{"GET", "/codewalk/popout.png"},
	{"GET", "/codewalk/run"},
	{"GET", "/codewalk/sharemem.xml"},
	{"GET", "/codewalk/urlpoll.go"},
	{"GET", "/devel/"},
	{"GET", "/devel/release.html"},
	{"GET", "/devel/weekly.html"},
	{"GET", "/gopher/"},
	{"GET", "/gopher/appenginegopher.jpg"},
	{"GET", "/gopher/appenginegophercolor.jpg"},
	{"GET", "/gopher/appenginelogo.gif"},
	{"GET", "/gopher/bumper.png"},
	{"GET", "/gopher/bumper192x108.png"},
	{"GET", "/gopher/bumper320x180.png"},
	{"GET", "/gopher/bumper480x270.png"},
	{"GET", "/gopher/bumper640x360.png"},
	{"GET", "/gopher/doc.png"},
	{"GET", "/gopher/frontpage.png"},
	{"GET", "/gopher/gopherbw.png"},
	{"GET", "/gopher/gophercolor.png"},
	{"GET", "/gopher/gophercolor16x16.png"},
	{"GET", "/gopher/help.png"},
	{"GET", "/gopher/pkg.png"},
	{"GET", "/gopher/project.png"},
	{"GET", "/gopher/ref.png"},
	{"GET", "/gopher/run.png"},
	{"GET", "/gopher/talks.png"},
	{"GET", "/gopher/pencil/"},
	{"GET", "/gopher/pencil/gopherhat.jpg"},
	{"GET", "/gopher/pencil/gopherhelmet.jpg"},
	{"GET", "/gopher/pencil/gophermega.jpg"},
	{"GET", "/gopher/pencil/gopherrunning.jpg"},
	{"GET", "/gopher/pencil/gopherswim.jpg"},
	{"GET", "/gopher/pencil/gopherswrench.jpg"},
	{"GET", "/play/"},
	{"GET", "/play/fib.go"},
	{"GET", "/play/hello.go"},
	{"GET", "/play/life.go"},
	{"GET", "/play/peano.go"},
	{"GET", "/play/pi.go"},
	{"GET", "/play/sieve.go"},
	{"GET", "/play/solitaire.go"},
	{"GET", "/play/tree.go"},
	{"GET", "/progs/"},
	{"GET", "/progs/cgo1.go"},
	{"GET", "/progs/cgo2.go"},
	{"GET", "/progs/cgo3.go"},
	{"GET", "/progs/cgo4.go"},
	{"GET", "/progs/defer.go"},
	{"GET", "/progs/defer.out"},
	{"GET", "/progs/defer2.go"},
	{"GET", "/progs/defer2.out"},
	{"GET", "/progs/eff_bytesize.go"},
	{"GET", "/progs/eff_bytesize.out"},
	{"GET", "/progs/eff_qr.go"},
	{"GET", "/progs/eff_sequence.go"},
	{"GET", "/progs/eff_sequence.out"},
	{"GET", "/progs/eff_unused1.go"},
	{"GET", "/progs/eff_unused2.go"},
	{"GET", "/progs/error.go"},
	{"GET", "/progs/error2.go"},
	{"GET", "/progs/error3.go"},
	{"GET", "/progs/error4.go"},
	{"GET", "/progs/go1.go"},
	{"GET", "/progs/gobs1.go"},
	{"GET", "/progs/gobs2.go"},
	{"GET", "/progs/image_draw.go"},
	{"GET", "/progs/image_package1.go"},
	{"GET", "/progs/image_package1.out"},
	{"GET", "/progs/image_package2.go"},
	{"GET", "/progs/image_package2.out"},
	{"GET", "/progs/image_package3.go"},
	{"GET", "/progs/image_package3.out"},
	{"GET", "/progs/image_package4.go"},
	{"GET", "/progs/image_package4.out"},
	{"GET", "/progs/image_package5.go"},
	{"GET", "/progs/image_package5.out"},
	{"GET", "/progs/image_package6.go"},
	{"GET", "/progs/image_package6.out"},
	{"GET", "/progs/interface.go"},
	{"GET", "/progs/interface2.go"},
	{"GET", "/progs/interface2.out"},
	{"GET", "/progs/json1.go"},
	{"GET", "/progs/json2.go"},
	{"GET", "/progs/json2.out"},
	{"GET", "/progs/json3.go"},
	{"GET", "/progs/json4.go"},
	{"GET", "/progs/json5.go"},
	{"GET", "/progs/run"},
	{"GET", "/progs/slices.go"},
	{"GET", "/progs/timeout1.go"},
	{"GET", "/progs/timeout2.go"},
	{"GET", "/progs/update.bash"},
}

// BenchmarkRoutes tests routes against a set of benchmark static routes
// taken from the httprouter router comparison.
/*
go test -bench=. -timeout=180m -benchtime 10s  -benchmem
BenchmarkRoutes-4        	  200000	     69200 ns/op	       0 B/op	       0 allocs/op
BenchmarkRoutesParse-4   	 2000000	      8593 ns/op	       0 B/op	       0 allocs/op
*/
func BenchmarkRoutes(b *testing.B) {
	m := New()

	// Set up a parallel set of paths for these routes
	var requests []*http.Request
	// Add routes - sending everything to the same test handler
	for _, r := range staticRoutes {
		req := httptest.NewRequest(r.m, r.p, nil)
		requests = append(requests, req)

		m.Add(r.p, handler)
	}

	// Now benchmark matching GET requests
	// Match all the routes with a GET request
	// this does not include parsing
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for _, r := range requests {
			r := m.Match(r)
			if r == nil {
				b.Errorf("error parsing route:%v", r)
			}
		}
	}

}

// Parse
// https://parse.com/docs/rest#summary
var parseAPI = []route{
	// Objects
	{"POST", "/1/classes/{className:\\d+}"},
	{"GET", "/1/classes/{className:\\d+}/{objectId:\\d+}"},
	{"PUT", "/1/classes/{className:\\d+}/{objectId:\\d+}"},
	{"GET", "/1/classes/{className:\\d+}"},
	{"DELETE", "/1/classes/{className:\\d+}/{objectId:\\d+}"},

	// Users
	{"POST", "/1/users"},
	{"GET", "/1/login"},
	{"GET", "/1/users/{objectId:\\d+}"},
	{"PUT", "/1/users/{objectId:\\d+}"},
	{"GET", "/1/users"},
	{"DELETE", "/1/users/{objectId:\\d+}"},
	{"POST", "/1/requestPasswordReset"},

	// Roles
	{"POST", "/1/roles"},
	{"GET", "/1/roles/{objectId:\\d+}"},
	{"PUT", "/1/roles/{objectId:\\d+}"},
	{"GET", "/1/roles"},
	{"DELETE", "/1/roles/{objectId:\\d+}"},

	// Files
	{"POST", "/1/files/{objectId:\\d+}"},

	// Analytics
	{"POST", "/1/events/{objectId:\\d+}"},

	// Push Notifications
	{"POST", "/1/push"},

	// Installations
	{"POST", "/1/installations"},
	{"GET", "/1/installations/{objectId:\\d+}"},
	{"PUT", "/1/installations/{objectId:\\d+}"},
	{"GET", "/1/installations"},
	{"DELETE", "/1/installations/{objectId:\\d+}"},

	// Cloud Functions
	{"POST", "/1/functions"},
}

// go test -test.bench BenchmarkRoutes -benchmem
func BenchmarkRoutesParse(b *testing.B) {
	m := New()

	// disable cache for routes parse so that we don't just test cache
	MaxCacheEntries = 0

	// Set up a parallel set of paths for these routes
	var requests []*http.Request
	// Add routes - sending everything to the same test handler
	for _, r := range parseAPI {
		path := strings.Replace(r.p, `{objectId:\d+}`, "99", -1)
		path = strings.Replace(path, `{className:\d+}`, "2", -1)
		req := httptest.NewRequest(r.m, path, nil)
		requests = append(requests, req)

		m.Add(r.p, handler).Methods(r.m)
	}

	// Now benchmark matching requests
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for _, r := range requests {
			r := m.Match(r)
			if r == nil {
				b.Errorf("error parsing route:%v", r)
			}
		}
	}

}

type testCache struct {
	method  string
	pattern string
	handler HandlerFunc
}

func listHandler(w http.ResponseWriter, r *http.Request) error       { return errors.New("list") }
func updateShowHandler(w http.ResponseWriter, r *http.Request) error { return errors.New("updateshow") }
func updateHandler(w http.ResponseWriter, r *http.Request) error     { return errors.New("updatepost") }
func destroyHandler(w http.ResponseWriter, r *http.Request) error    { return errors.New("destroypost") }
func showHandler(w http.ResponseWriter, r *http.Request) error       { return errors.New("show") }

var testStaticCacheHandlers = []testCache{
	{"GET", "/", listHandler},
	{"GET", "/my/longer/static/path", showHandler},
	{"GET", "/my/longer", updateHandler},
}

// go test -test.bench BenchmarkRoutes -benchmem
// Benchmark hitting / repeatedly with cache on
// should get very fast response times
func BenchmarkStaticCached(b *testing.B) {
	m := New()
	MaxCacheEntries = 500

	var requests []*http.Request

	// Set up the handlers
	for _, tch := range testStaticCacheHandlers {
		m.Get(tch.pattern, tch.handler)
		req := httptest.NewRequest(tch.method, tch.pattern, nil)
		requests = append(requests, req)
	}

	// Now benchmark matching requests
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for _, r := range requests {
			route := m.Match(r)
			if route == nil {
				b.Errorf("error matching request:%v %v", r, m)
			}
		}
	}

}

var testCacheHandlers = []testCache{
	{"GET", "/users", listHandler},
	{"GET", "/users/{id:\\d+}/update", updateShowHandler},
	//	{"POST", "/users/{id:\\d+}/update", updateHandler},
	{"POST", "/users/{id:\\d+}/destroy", destroyHandler},
	{"GET", "/users/{id:\\d+}", showHandler},
}

// TestCache tests the cache does not distort results.
func TestCache(t *testing.T) {
	m := New()
	MaxCacheEntries = 500

	// Set up the handlers
	for _, tch := range testCacheHandlers {
		switch tch.method {
		case "GET":
			m.Get(tch.pattern, tch.handler)
		case "POST":
			m.Post(tch.pattern, tch.handler)
		}
	}

	// Check the response is correct at first (no cache)
	var responses []Route
	for _, tch := range testCacheHandlers {
		path := strings.Replace(tch.pattern, "{id:\\d+}", "33", -1)
		req := httptest.NewRequest(tch.method, path, nil)
		w := httptest.NewRecorder()
		route := m.Match(req)
		responses = append(responses, route)
		// Check errors returned by handlers registered on t
		if route.Handler()(w, req).Error() != tch.handler(w, req).Error() {
			t.Errorf("Failed to match route handler:%s->%s", route.Handler()(w, req).Error(), tch.handler(w, req).Error())
		}
	}

	// Check subsequent 100 runs (with cache) give the same result
	for i := 1; i < 100; i++ {
		for _, tch := range testCacheHandlers {
			path := strings.Replace(tch.pattern, "{id:\\d+}", fmt.Sprintf("%d", i), -1)
			req := httptest.NewRequest(tch.method, path, nil)
			w := httptest.NewRecorder()
			route := m.Match(req)
			responses = append(responses, route)
			// Check errors returned by handlers registered on t
			if route.Handler()(w, req).Error() != tch.handler(w, req).Error() {
				t.Errorf("Failed to match route handler:%s->%s", route.Handler()(w, req).Error(), tch.handler(w, req).Error())
			}
		}
	}

}
