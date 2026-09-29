// Package testenv provides a hermetic test harness: a temporary config,
// a migrated SQLite database, templates, auth keys, seeded records and
// mocked HTTP transports. It must never touch secrets/ or a real database.
//
// It deliberately imports none of the application packages (app, products,
// users, subscriptions) so that their internal tests can use it without
// import cycles. Tests using it mutate process globals (config, database,
// cwd, transports) and must not call t.Parallel().
package testenv

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stripe/stripe-go/v72"

	"github.com/abishekmuthian/open-payment-host/src/lib/auth"
	"github.com/abishekmuthian/open-payment-host/src/lib/auth/can"
	"github.com/abishekmuthian/open-payment-host/src/lib/helpers"
	"github.com/abishekmuthian/open-payment-host/src/lib/mux"
	"github.com/abishekmuthian/open-payment-host/src/lib/query"
	"github.com/abishekmuthian/open-payment-host/src/lib/server"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/view"
)

// Test keys; never valid anywhere else.
const (
	SecretKey = "0123456789abcdef0123456789abcdef"
	HMACKey   = "abcdef0123456789abcdef0123456789"
	RootURL   = "https://merchant.test"
)

// Role ids mirror users.Anon/Reader/Admin (not imported to avoid cycles).
const (
	RoleAnon   = 0
	RoleReader = 20
	RoleAdmin  = 100
)

var rootOnce sync.Once
var rootDir string

// Root changes the working directory to the repository root (the directory
// holding go.mod) for the duration of the test and returns it.
func Root(t testing.TB) string {
	t.Helper()
	rootOnce.Do(func() {
		dir, err := os.Getwd()
		if err != nil {
			return
		}
		for {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				rootDir = dir
				return
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				return
			}
			dir = parent
		}
	})
	if rootDir == "" {
		t.Fatal("testenv: go.mod not found")
	}
	target := rootDir
	if sandboxDir != "" {
		target = sandboxDir
	}
	chdir(t, target)
	return target
}

func chdir(t testing.TB, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
}

var sandboxDir string

// Sandbox makes a temporary project root for code that writes relative to the
// working directory (asset compilation writes public/assets/{scripts,styles}
// and secrets/assets.json). src, db and read-only public files are symlinked
// to the repository; everything written lands in t.TempDir(). Root(t) returns
// the sandbox while it is active.
func Sandbox(t testing.TB) string {
	t.Helper()
	repo := Root(t)
	dir := t.TempDir()
	for _, d := range []string{"secrets", "public/assets/scripts", "public/assets/styles"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0755); err != nil {
			t.Fatal(err)
		}
	}
	for _, link := range []string{"src", "db", "public/favicon.ico", "public/assets/icons", "public/assets/images"} {
		if _, err := os.Stat(filepath.Join(repo, link)); err != nil {
			continue
		}
		if err := os.Symlink(filepath.Join(repo, link), filepath.Join(dir, link)); err != nil {
			t.Fatal(err)
		}
	}
	sandboxDir = dir
	t.Cleanup(func() { sandboxDir = "" })
	chdir(t, dir)
	return dir
}

// Config installs a temporary config as config.Current in development mode.
// Overrides apply to both development and production sections.
func Config(t testing.TB, overrides map[string]string) *config.Config {
	t.Helper()
	values := map[string]string{
		"secret_key":                  SecretKey,
		"hmac_key":                    HMACKey,
		"session_name":                "oph_test_session",
		"root_url":                    RootURL,
		"name":                        "Open Payment Host Test",
		"paypal_api_domain":           "https://api.paypal.test",
		"square_domain":               "https://square.test/v2",
		"subscription_client_country": "US",
	}
	for k, v := range overrides {
		values[k] = v
	}
	data := map[string]map[string]string{"development": values, "production": copyMap(values)}
	b, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	c := config.New()
	if err := c.Load(path); err != nil {
		t.Fatal(err)
	}
	old := config.Current
	config.Current = c
	t.Cleanup(func() { config.Current = old })
	Hermetic(t)
	return c
}

// Set changes a config value in both modes for the rest of the test.
func Set(key, value string) {
	for _, mode := range []int{config.ModeDevelopment, config.ModeProduction} {
		saved := config.Current.Mode
		config.Current.Mode = mode
		config.Configuration(mode)[key] = value
		config.Current.Mode = saved
	}
}

// Production switches the current config to production mode for the test.
func Production(t testing.TB) {
	t.Helper()
	old := config.Current.Mode
	config.Current.Mode = config.ModeProduction
	t.Cleanup(func() { config.Current.Mode = old })
}

func copyMap(m map[string]string) map[string]string {
	c := make(map[string]string, len(m))
	for k, v := range m {
		c[k] = v
	}
	return c
}

// LatestMigration returns the highest migration version in db/migrate.
func LatestMigration(t testing.TB) int64 {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(Root(t), "db", "migrate", "*.sql"))
	if err != nil {
		t.Fatal(err)
	}
	var latest int64
	for _, f := range files {
		n, err := strconv.ParseInt(strings.SplitN(filepath.Base(f), "_", 2)[0], 10, 64)
		if err == nil && n > latest {
			latest = n
		}
	}
	return latest
}

// DB opens a fresh SQLite database in t.TempDir() through the production
// code path (Create-Tables.sql plus every migration) and closes it on cleanup.
func DB(t testing.TB) string {
	t.Helper()
	Root(t)
	path := filepath.Join(t.TempDir(), "oph-test.db")
	if err := query.OpenDatabase(map[string]string{"adapter": "sqlite3", "db": path}, &sync.RWMutex{}); err != nil {
		query.CloseDatabase()
		t.Fatalf("testenv: open database: %v", err)
	}
	t.Cleanup(func() { query.CloseDatabase() })
	version, dirty := MigrationState(t)
	if dirty != 0 || version != LatestMigration(t) {
		t.Fatalf("testenv: schema_migrations version=%d dirty=%d, want version=%d clean", version, dirty, LatestMigration(t))
	}
	return path
}

// MigrationState returns the recorded migration version and dirty flag.
func MigrationState(t testing.TB) (version, dirty int64) {
	t.Helper()
	rows, err := query.QuerySQL("SELECT version, dirty FROM schema_migrations")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatal("testenv: schema_migrations is empty")
	}
	var d bool
	if err := rows.Scan(&version, &d); err != nil {
		t.Fatal(err)
	}
	if d {
		dirty = 1
	}
	return version, dirty
}

// Columns returns the column names of a table.
func Columns(t testing.TB, table string) map[string]bool {
	t.Helper()
	rows, err := query.QuerySQL("SELECT name FROM pragma_table_info(?)", table)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	cols := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		cols[strings.ToLower(name)] = true
	}
	return cols
}

var templatesOnce sync.Once
var templatesErr error

// Templates loads every .got template under src once, with the same helpers
// as app.SetupView, and disables per-render rescans.
func Templates(t testing.TB) {
	t.Helper()
	Root(t)
	templatesOnce.Do(func() {
		view.Helpers["markup"] = helpers.Markup
		view.Helpers["timeago"] = helpers.TimeAgo
		view.Helpers["root_url"] = helpers.RootURL
		templatesErr = view.LoadTemplatesAtPaths([]string{"src"}, view.Helpers)
	})
	if templatesErr != nil {
		t.Fatalf("testenv: templates: %v", templatesErr)
	}
	view.Production = true
}

var canOnce sync.Once

// Auth installs session keys and the same authorisation rules as
// app.SetupAuth (table names inlined to avoid import cycles).
func Auth(t testing.TB) {
	t.Helper()
	oldH, oldS, oldN := auth.HMACKey, auth.SecretKey, auth.SessionName
	auth.HMACKey = auth.HexToBytes(HMACKey)
	auth.SecretKey = auth.HexToBytes(SecretKey)
	auth.SessionName = "oph_test_session"
	t.Cleanup(func() { auth.HMACKey, auth.SecretKey, auth.SessionName = oldH, oldS, oldN })
	canOnce.Do(func() {
		can.Authorise(RoleAdmin, can.ManageResource, can.Anything)
		can.AuthoriseOwner(RoleReader, can.UpdateResource, "users")
		can.Authorise(RoleReader, can.CreateResource, "products")
		can.AuthoriseOwner(RoleReader, can.UpdateResource, "products")
		can.Authorise(RoleReader, can.CreateResource, "subscriptions")
		can.AuthoriseOwner(RoleReader, can.UpdateResource, "subscriptions")
		can.AuthoriseOwner(RoleAnon, can.CreateResource, "users")
	})
}

// Setup is the common combination: config, root, auth, templates and a database.
func Setup(t testing.TB, overrides map[string]string) {
	t.Helper()
	Config(t, overrides)
	Auth(t)
	Templates(t)
	DB(t)
}

// Exec runs SQL. Without args the string may hold several ';'-separated statements.
func Exec(t testing.TB, sql string, args ...interface{}) {
	t.Helper()
	statements := []string{sql}
	if len(args) == 0 {
		statements = strings.Split(sql, ";")
	}
	for _, s := range statements {
		if strings.TrimSpace(s) == "" {
			continue
		}
		if _, err := query.ExecSQL(s, args...); err != nil {
			t.Fatalf("testenv: exec %q: %v", s, err)
		}
	}
}

// Scalar returns the single integer produced by a query.
func Scalar(t testing.TB, sql string, args ...interface{}) int64 {
	t.Helper()
	rows, err := query.QuerySQL(sql, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatalf("testenv: no row for %q", sql)
	}
	var n int64
	if err := rows.Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// Text returns the single string produced by a query.
func Text(t testing.TB, sql string, args ...interface{}) string {
	t.Helper()
	rows, err := query.QuerySQL(sql, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatalf("testenv: no row for %q", sql)
	}
	var s *string
	if err := rows.Scan(&s); err != nil {
		t.Fatal(err)
	}
	if s == nil {
		return ""
	}
	return *s
}

// SeedUser inserts a user with a bcrypt password hash and returns its id.
func SeedUser(t testing.TB, role int64, email, password string) int64 {
	t.Helper()
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	now := query.TimeString(time.Now().UTC())
	id, err := query.New("users", "id").Insert(map[string]string{
		"created_at": now, "updated_at": now, "status": "100",
		"role": strconv.FormatInt(role, 10), "email": email, "password_hash": hash,
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// SeedProduct inserts a published one-time product; cols override defaults.
// Map and slice values are JSON encoded, as the application stores them.
func SeedProduct(t testing.TB, cols map[string]interface{}) int64 {
	t.Helper()
	now := query.TimeString(time.Now().UTC())
	params := map[string]string{
		"created_at": now, "updated_at": now, "status": "100", "name": "Test product",
		"summary": "A product", "description": "<p>A product</p>", "schedule": "onetime",
		"user_id": "1", "points": "1", "total_subscribers": "0", "total_onetime_payments": "0",
	}
	for k, v := range cols {
		switch value := v.(type) {
		case string:
			params[k] = value
		case nil:
			delete(params, k)
		case map[string]string, map[string]interface{}, map[string]map[string]interface{}, []interface{}, []string:
			b, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			params[k] = string(b)
		default:
			params[k] = fmt.Sprint(value)
		}
	}
	id, err := query.New("products", "id").Insert(params)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// Session returns a session cookie holding a fresh CSRF secret and, when
// userID > 0, the logged-in user, plus a valid authenticity token for it.
func Session(t testing.TB, userID int64) (*http.Cookie, string) {
	t.Helper()
	w := httptest.NewRecorder()
	session, err := auth.Session(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if err != nil {
		t.Fatal(err)
	}
	secret := auth.BytesToBase64(auth.RandomToken(auth.TokenLength))
	session.Set(auth.SessionTokenKey, secret)
	if userID > 0 {
		session.Set(auth.SessionUserKey, strconv.FormatInt(userID, 10))
	}
	if err := session.Save(w); err != nil {
		t.Fatal(err)
	}
	token := auth.BytesToBase64(auth.AuthenticityTokenWithSecret(auth.Base64ToBytes(secret)))
	for _, c := range w.Result().Cookies() {
		if c.Name == auth.SessionName {
			return c, token
		}
	}
	t.Fatal("testenv: no session cookie")
	return nil, ""
}

// LoggedIn adds a session cookie for userID to r and returns its CSRF token.
func LoggedIn(t testing.TB, r *http.Request, userID int64) string {
	t.Helper()
	cookie, token := Session(t, userID)
	r.AddCookie(cookie)
	return token
}

// AuthedPOST builds a form POST carrying a valid CSRF token and session
// cookie; userID 0 means an anonymous visitor.
func AuthedPOST(t testing.TB, path string, form url.Values, userID int64) *http.Request {
	t.Helper()
	cookie, token := Session(t, userID)
	if form == nil {
		form = url.Values{}
	}
	form.Set(auth.SessionTokenKey, token)
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(cookie)
	return r
}

// POST builds a form POST without any session or token.
func POST(path string, form url.Values) *http.Request {
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return r
}

// RoundTripFunc adapts a function to http.RoundTripper.
type RoundTripFunc func(*http.Request) (*http.Response, error)

// RoundTrip implements http.RoundTripper.
func (f RoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Response builds a canned HTTP response.
func Response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Status: http.StatusText(status), Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
}

var realTransport = http.DefaultTransport

// Hermetic replaces http.DefaultTransport with one that refuses every
// non-loopback request, so no test can reach a real provider by accident.
func Hermetic(t testing.TB) {
	t.Helper()
	swapTransport(t, RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if isLoopback(r.URL.Hostname()) {
			return realTransport.RoundTrip(r)
		}
		return nil, fmt.Errorf("testenv: blocked external request to %s", r.URL.Host)
	}))
}

// MockTransport routes every http.DefaultTransport request to fn.
func MockTransport(t testing.TB, fn func(*http.Request) (*http.Response, error)) {
	t.Helper()
	swapTransport(t, RoundTripFunc(fn))
}

func swapTransport(t testing.TB, rt http.RoundTripper) {
	old := http.DefaultTransport
	http.DefaultTransport = rt
	t.Cleanup(func() { http.DefaultTransport = old })
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// StripeBackend points the Stripe API backend at fn, with retries disabled.
func StripeBackend(t testing.TB, fn func(*http.Request) (*http.Response, error)) {
	t.Helper()
	old := stripe.GetBackend(stripe.APIBackend)
	retries := int64(0)
	stripe.SetBackend(stripe.APIBackend, stripe.GetBackendWithConfig(stripe.APIBackend, &stripe.BackendConfig{
		HTTPClient:        &http.Client{Transport: RoundTripFunc(fn)},
		MaxNetworkRetries: &retries,
		LeveledLogger:     &stripe.LeveledLogger{Level: stripe.LevelNull},
	}))
	t.Cleanup(func() { stripe.SetBackend(stripe.APIBackend, old) })
}

var (
	routerMu sync.Mutex
	router   *mux.Mux
	routes   = map[string]bool{}
)

// Router returns the process-wide default mux used by mux.Params. mux.SetDefault
// only honours its first call, so all tests in a binary must share one mux.
func Router() *mux.Mux {
	routerMu.Lock()
	defer routerMu.Unlock()
	if router == nil {
		router = mux.New()
		router.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			status := server.ToStatusError(err)
			w.WriteHeader(status.Status)
			_, _ = io.WriteString(w, status.Error())
		}
		mux.SetDefault(router)
	}
	return router
}

// Route registers a route on the shared mux once; repeated calls are no-ops.
func Route(method, pattern string, h mux.HandlerFunc) *mux.Mux {
	m := Router()
	routerMu.Lock()
	defer routerMu.Unlock()
	key := method + " " + pattern
	if !routes[key] {
		routes[key] = true
		if method == http.MethodPost {
			m.Post(pattern, h)
		} else {
			m.Get(pattern, h)
		}
	}
	return m
}

// Serve runs r through the shared mux and returns the recorded response.
// A handler panic fails the test (status 999) instead of crashing the binary.
func Serve(t testing.TB, r *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	return ServeWith(t, Router(), r)
}

// ServeWith runs r through h, turning a panic into a test failure.
func ServeWith(t testing.TB, h http.Handler, r *http.Request) (w *httptest.ResponseRecorder) {
	t.Helper()
	w = httptest.NewRecorder()
	defer func() {
		if p := recover(); p != nil {
			t.Errorf("handler panicked on %s %s: %v", r.Method, r.URL, p)
			w.Code = 999
		}
	}()
	h.ServeHTTP(w, r)
	return w
}

var commentRE = regexp.MustCompile(`(?s)<!--.*?-->`)

// Blank reports whether an HTML body has no content besides whitespace and comments.
func Blank(body string) bool {
	return strings.TrimSpace(commentRE.ReplaceAllString(body, "")) == ""
}

// SortedKeys returns map keys in order, for stable assertions.
func SortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
