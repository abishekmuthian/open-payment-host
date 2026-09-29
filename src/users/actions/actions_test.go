package useractions

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/abishekmuthian/open-payment-host/src/internal/testenv"
	"github.com/abishekmuthian/open-payment-host/src/lib/auth"
	"github.com/abishekmuthian/open-payment-host/src/lib/ratelimit"
	"github.com/abishekmuthian/open-payment-host/src/users"
)

const defaultPassword = "default-admin-password"

func setupUsers(t *testing.T) {
	t.Helper()
	testenv.Setup(t, map[string]string{
		"admin_email":            "admin@merchant.test",
		"admin_default_password": defaultPassword,
		"turnstile_site_key":     "site-key",
		"turnstile_secret_key":   "turnstile-secret",
	})
	testenv.Route(http.MethodGet, "/users/{id:[0-9]+}/password/change", HandlePasswordChangeShow)
	testenv.Route(http.MethodPost, "/users/{id:[0-9]+}/password/change", HandlePasswordChange)
	testenv.Route(http.MethodGet, "/users/login", HandleLoginShow)
	testenv.Route(http.MethodPost, "/users/login", HandleLogin)
	testenv.Route(http.MethodPost, "/users/logout", HandleLogout)
	// Limiters are process globals; give every test fresh counters.
	oldIP, oldAccount := loginIPLimiter, loginAccountLimiter
	loginIPLimiter = ratelimit.New(oldIP.Max, oldIP.Window)
	loginAccountLimiter = ratelimit.New(oldAccount.Max, oldAccount.Window)
	t.Cleanup(func() { loginIPLimiter, loginAccountLimiter = oldIP, oldAccount })
}

// turnstile answers siteverify with the given JSON and records the posted form.
func turnstile(t *testing.T, response string) *url.Values {
	posted := &url.Values{}
	testenv.MockTransport(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://challenges.cloudflare.com/turnstile/v0/siteverify" {
			return nil, fmt.Errorf("unexpected request %s", r.URL)
		}
		body, _ := io.ReadAll(r.Body)
		*posted, _ = url.ParseQuery(string(body))
		return testenv.Response(http.StatusOK, response), nil
	})
	return posted
}

func login(t *testing.T, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	return testenv.Serve(t, testenv.AuthedPOST(t, "/users/login", form, 0))
}

// sessionUser returns the user id stored in the session cookie set by w.
func sessionUser(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	return sessionValue(t, w, auth.SessionUserKey)
}

// sessionValue returns key from the session cookie set by w.
func sessionValue(t *testing.T, w *httptest.ResponseRecorder, key string) string {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range w.Result().Cookies() {
		if c.Name == auth.SessionName && c.MaxAge >= 0 {
			r.AddCookie(c)
		}
	}
	s, err := auth.SessionGet(r)
	if err != nil {
		return ""
	}
	return s.Get(key)
}

func TestCreateAndUpdateAdminUser(t *testing.T) {
	testenv.Config(t, nil)
	testenv.DB(t)
	if err := HandleCreateAdminUser("admin@merchant.test", "first-password"); err != nil {
		t.Fatal(err)
	}
	u, err := users.Find(1)
	if err != nil || !u.Admin() || u.Email != "admin@merchant.test" || auth.CheckPassword("first-password", u.PasswordHash) != nil {
		t.Fatalf("admin not created: %+v %v", u, err)
	}
	if err := HandleUpdate(1, "new@merchant.test", "second-password"); err != nil {
		t.Fatal(err)
	}
	u, _ = users.Find(1)
	if u.Email != "new@merchant.test" || auth.CheckPassword("second-password", u.PasswordHash) != nil || !u.Admin() {
		t.Fatalf("admin not updated: %+v", u)
	}
	if err := HandleUpdate(99, "x@y.z", "password"); err == nil {
		t.Fatal("updating a missing user succeeded")
	}
}

func TestLoginShow(t *testing.T) {
	setupUsers(t)
	id := testenv.SeedUser(t, testenv.RoleAdmin, "admin@merchant.test", "strong-password")
	w := testenv.Serve(t, httptest.NewRequest(http.MethodGet, "/users/login?error=not_a_valid_login&redirecturl=/products", nil))
	body := w.Body.String()
	if w.Code != http.StatusOK || !strings.Contains(body, `data-sitekey="site-key"`) || !strings.Contains(body, "Invalid login credentials") || !strings.Contains(body, `value="/products"`) {
		t.Fatalf("status=%d body=%s", w.Code, body)
	}
	r := httptest.NewRequest(http.MethodGet, "/users/login", nil)
	testenv.LoggedIn(t, r, id)
	if w := testenv.Serve(t, r); w.Code != http.StatusFound || w.Header().Get("Location") != "/?warn=already_logged_in" {
		t.Fatalf("logged in user: %d %s", w.Code, w.Header().Get("Location"))
	}
}

func TestLoginFlows(t *testing.T) {
	setupUsers(t)
	admin := testenv.SeedUser(t, testenv.RoleAdmin, "admin@merchant.test", "strong-password")
	fresh := testenv.SeedUser(t, testenv.RoleAdmin, "fresh@merchant.test", defaultPassword)
	ok := `{"success":true}`
	creds := func(email, password string, extra ...string) url.Values {
		v := url.Values{"email": {email}, "password": {password}, "cf-turnstile-response": {"token"}}
		for i := 0; i+1 < len(extra); i += 2 {
			v.Set(extra[i], extra[i+1])
		}
		return v
	}
	cases := []struct {
		name      string
		turnstile string
		form      url.Values
		location  string
		userID    string
	}{
		{"missing turnstile", ok, url.Values{"email": {"admin@merchant.test"}, "password": {"strong-password"}}, "/users/login?error=security_challenge_not_completed_login#login", ""},
		{"turnstile failure", `{"success":false,"error-codes":["invalid-input-response"]}`, creds("admin@merchant.test", "strong-password"), "/users/login?error=security_challenge_failed_login#login", ""},
		// Regression: an empty error-codes list panicked on ErrorCodes[0].
		{"turnstile failure without codes", `{"success":false}`, creds("admin@merchant.test", "strong-password"), "/users/login?error=security_challenge_failed_login#login", ""},
		{"turnstile garbage", `not json`, creds("admin@merchant.test", "strong-password"), "/users/login?error=security_challenge_failed_login#login", ""},
		{"unknown email", ok, creds("nobody@merchant.test", "strong-password"), "/users/login?error=not_a_valid_login", ""},
		{"wrong password", ok, creds("admin@merchant.test", "wrong-password"), "/users/login?error=not_a_valid_login", ""},
		{"success", ok, creds("admin@merchant.test", "strong-password"), "/", fmt.Sprint(admin)},
		{"success with redirect", ok, creds("admin@merchant.test", "strong-password", "redirectURL", "/products/1"), "/products/1", fmt.Sprint(admin)},
		// Regression: protocol-relative redirect URLs were followed off-site.
		{"open redirect ignored", ok, creds("admin@merchant.test", "strong-password", "redirectURL", "//evil.test"), "/", fmt.Sprint(admin)},
		{"backslash redirect ignored", ok, creds("admin@merchant.test", "strong-password", "redirectURL", "/\\evil.test"), "/", fmt.Sprint(admin)},
		{"default password must be changed", ok, creds("fresh@merchant.test", defaultPassword), fmt.Sprintf("/users/%d/password/change", fresh), fmt.Sprint(fresh)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			posted := turnstile(t, c.turnstile)
			w := login(t, c.form)
			if w.Code != http.StatusFound || w.Header().Get("Location") != c.location {
				t.Fatalf("status=%d location=%q body=%s", w.Code, w.Header().Get("Location"), w.Body.String())
			}
			if got := sessionUser(t, w); got != c.userID {
				t.Fatalf("session user=%q want %q", got, c.userID)
			}
			// Only a default-password login restricts the session to the change page.
			wantFlag := ""
			if c.userID == fmt.Sprint(fresh) {
				wantFlag = c.userID
			}
			if got := sessionValue(t, w, auth.SessionPasswordChangeKey); got != wantFlag {
				t.Fatalf("password change flag=%q want %q", got, wantFlag)
			}
			if c.form.Get("cf-turnstile-response") != "" && (posted.Get("secret") != "turnstile-secret" || posted.Get("response") != "token") {
				t.Fatalf("siteverify form = %v", *posted)
			}
		})
	}
	// Without a CSRF token the login is refused and no session is created.
	turnstile(t, ok)
	w := testenv.Serve(t, testenv.POST("/users/login", creds("admin@merchant.test", "strong-password")))
	if w.Code == http.StatusFound || sessionUser(t, w) != "" {
		t.Fatalf("tokenless login status=%d", w.Code)
	}
}

func TestLoginRateLimit(t *testing.T) {
	setupUsers(t)
	admin := testenv.SeedUser(t, testenv.RoleAdmin, "admin@merchant.test", "strong-password")
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	loginIPLimiter.SetClock(clock)
	loginAccountLimiter.SetClock(clock)

	calls := 0
	turnstileResult := `{"success":true}`
	testenv.MockTransport(t, func(r *http.Request) (*http.Response, error) {
		calls++
		return testenv.Response(http.StatusOK, turnstileResult), nil
	})
	attempt := func(email, password string) (string, string) {
		t.Helper()
		w := login(t, url.Values{"email": {email}, "password": {password}, "cf-turnstile-response": {"token"}})
		if w.Code != http.StatusFound {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
		return w.Header().Get("Location"), sessionUser(t, w)
	}
	const limited = "/users/login?error=too_many_login_attempts#login"
	resetIP := func() { loginIPLimiter = ratelimit.New(10, 15*time.Minute); loginIPLimiter.SetClock(clock) }

	// Five wrong passwords lock the account; even the right password is then
	// refused without a siteverify call or a session.
	for i := 0; i < 5; i++ {
		if loc, _ := attempt("admin@merchant.test", "wrong-password"); loc != "/users/login?error=not_a_valid_login" {
			t.Fatalf("attempt %d: %s", i, loc)
		}
	}
	calls = 0
	if loc, user := attempt(" Admin@Merchant.test ", "strong-password"); loc != limited || user != "" {
		t.Fatalf("blocked account: %s user=%q", loc, user)
	}
	if calls != 0 {
		t.Fatalf("siteverify calls for a blocked attempt = %d", calls)
	}

	// Other accounts from the same IP work until the IP limit (10) is reached.
	for i := 0; i < 4; i++ {
		if loc, _ := attempt("nobody@merchant.test", "x"); loc != "/users/login?error=not_a_valid_login" {
			t.Fatalf("other account attempt %d: %s", i, loc)
		}
	}
	if loc, _ := attempt("other@merchant.test", "x"); loc != "/users/login?error=not_a_valid_login" {
		t.Fatalf("10th IP failure: %s", loc)
	}
	if loc, _ := attempt("another@merchant.test", "x"); loc != limited {
		t.Fatalf("IP not blocked: %s", loc)
	}

	// Both windows expire.
	now = now.Add(15 * time.Minute)
	if loc, user := attempt("admin@merchant.test", "strong-password"); loc != "/" || user != fmt.Sprint(admin) {
		t.Fatalf("after window: %s user=%q", loc, user)
	}

	// Success resets the account counter.
	resetIP()
	for i := 0; i < 4; i++ {
		attempt("admin@merchant.test", "wrong-password")
	}
	if loc, _ := attempt("admin@merchant.test", "strong-password"); loc != "/" {
		t.Fatalf("success before limit: %s", loc)
	}
	for i := 0; i < 4; i++ {
		if loc, _ := attempt("admin@merchant.test", "wrong-password"); loc != "/users/login?error=not_a_valid_login" {
			t.Fatalf("counter not reset, attempt %d: %s", i, loc)
		}
	}

	// Turnstile failures count toward the IP but never lock the account.
	resetIP()
	loginAccountLimiter = ratelimit.New(5, 15*time.Minute)
	turnstileResult = `{"success":false}`
	for i := 0; i < 9; i++ {
		if loc, _ := attempt("admin@merchant.test", "strong-password"); loc != "/users/login?error=security_challenge_failed_login#login" {
			t.Fatalf("turnstile failure %d: %s", i, loc)
		}
	}
	w := login(t, url.Values{"email": {"admin@merchant.test"}, "password": {"strong-password"}})
	if loc := w.Header().Get("Location"); loc != "/users/login?error=security_challenge_not_completed_login#login" {
		t.Fatalf("missing turnstile: %s", loc)
	}
	if blocked, _ := loginAccountLimiter.Blocked("admin@merchant.test"); blocked {
		t.Fatal("turnstile failures locked the account")
	}
	if loc, _ := attempt("admin@merchant.test", "strong-password"); loc != limited {
		t.Fatalf("IP not blocked by turnstile failures: %s", loc)
	}

	// The login page explains the block.
	w = testenv.Serve(t, httptest.NewRequest(http.MethodGet, "/users/login?error=too_many_login_attempts", nil))
	if !strings.Contains(w.Body.String(), "Too many failed login attempts") {
		t.Fatalf("message missing: %s", w.Body.String())
	}
}

func TestPasswordChange(t *testing.T) {
	setupUsers(t)
	admin := testenv.SeedUser(t, testenv.RoleAdmin, "admin@merchant.test", defaultPassword)
	reader := testenv.SeedUser(t, testenv.RoleReader, "reader@merchant.test", "reader-password")
	path := fmt.Sprintf("/users/%d/password/change", admin)
	change := func(user int64, target string, pass, confirm string) *httptest.ResponseRecorder {
		return testenv.Serve(t, testenv.AuthedPOST(t, target, url.Values{"password": {pass}, "password-confirm": {confirm}}, user))
	}
	for _, c := range []struct {
		pass, confirm, errCode string
	}{
		{"new-password-1", "new-password-2", "passwords_dont_match"},
		{"short", "short", "low_passwords_characters"},
		{defaultPassword, defaultPassword, "no_default_password"},
	} {
		w := change(admin, path, c.pass, c.confirm)
		if w.Code != http.StatusFound || !strings.HasSuffix(w.Header().Get("Location"), "error="+c.errCode) {
			t.Errorf("%s: %d %s", c.errCode, w.Code, w.Header().Get("Location"))
		}
	}
	// Regression: only the current password was checked, so a user whose
	// password had changed could set it back to the default.
	other := testenv.SeedUser(t, testenv.RoleAdmin, "other@merchant.test", "other-password")
	if w := change(other, fmt.Sprintf("/users/%d/password/change", other), defaultPassword, defaultPassword); !strings.HasSuffix(w.Header().Get("Location"), "error=no_default_password") {
		t.Fatalf("reset to default password: %d %s", w.Code, w.Header().Get("Location"))
	}
	// Another user's id is refused.
	if w := change(reader, path, "stolen-password", "stolen-password"); w.Code != http.StatusUnauthorized {
		t.Fatalf("reader changing admin password status=%d", w.Code)
	}
	if w := change(0, path, "stolen-password", "stolen-password"); w.Code != http.StatusUnauthorized {
		t.Fatalf("anon changing admin password status=%d", w.Code)
	}
	if w := testenv.Serve(t, testenv.POST(path, url.Values{"password": {"no-token-pass"}, "password-confirm": {"no-token-pass"}})); w.Code == http.StatusFound {
		t.Fatal("tokenless change accepted")
	}
	u, _ := users.Find(admin)
	if auth.CheckPassword(defaultPassword, u.PasswordHash) != nil {
		t.Fatal("password changed by a rejected request")
	}
	// Success stores the new hash and logs the user out.
	w := change(admin, path, "brand-new-password", "brand-new-password")
	if w.Code != http.StatusFound || w.Header().Get("Location") != "/" {
		t.Fatalf("success: %d %s", w.Code, w.Header().Get("Location"))
	}
	cleared := false
	for _, c := range w.Result().Cookies() {
		cleared = cleared || (c.Name == auth.SessionName && c.MaxAge < 0)
	}
	if !cleared {
		t.Fatal("session not cleared after password change")
	}
	u, _ = users.Find(admin)
	if auth.CheckPassword("brand-new-password", u.PasswordHash) != nil {
		t.Fatal("new password not stored")
	}
	// The change form is shown only to the owner.
	r := httptest.NewRequest(http.MethodGet, path, nil)
	testenv.LoggedIn(t, r, admin)
	if w := testenv.Serve(t, r); w.Code != http.StatusOK {
		t.Fatalf("owner form status=%d", w.Code)
	}
	r = httptest.NewRequest(http.MethodGet, path, nil)
	testenv.LoggedIn(t, r, reader)
	if w := testenv.Serve(t, r); w.Code != http.StatusUnauthorized {
		t.Fatalf("other user form status=%d", w.Code)
	}
}

func TestLogoutRequiresToken(t *testing.T) {
	setupUsers(t)
	id := testenv.SeedUser(t, testenv.RoleAdmin, "admin@merchant.test", "strong-password")
	r := testenv.POST("/users/logout", url.Values{})
	testenv.LoggedIn(t, r, id)
	if w := testenv.Serve(t, r); w.Code == http.StatusFound {
		t.Fatal("logout without CSRF token accepted")
	}
	w := testenv.Serve(t, testenv.AuthedPOST(t, "/users/logout", nil, id))
	if w.Code != http.StatusFound || w.Header().Get("Location") != "/" {
		t.Fatalf("logout: %d %s", w.Code, w.Header().Get("Location"))
	}
	if !strings.Contains(w.Header().Get("Set-Cookie"), "Max-Age=0") {
		t.Fatalf("session cookie not cleared: %q", w.Header().Get("Set-Cookie"))
	}
}
