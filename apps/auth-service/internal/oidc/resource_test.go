package oidc

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"

	"github.com/jtumidanski/myfleet/apps/auth-service/internal/session"
	"github.com/jtumidanski/myfleet/apps/auth-service/internal/user"
)

// --- fakes satisfying the consumer-side interfaces (design §3.2) ---
//
// Function fields rather than canned values: a failure case overwrites exactly
// the one collaborator whose error it exercises and leaves the rest succeeding.

type fakeAuthenticator struct {
	exchange func(ctx context.Context, code string) (string, error)
	verify   func(ctx context.Context, rawIDToken string) (user.GoogleProfile, string, error)
}

func (f fakeAuthenticator) AuthCodeURL(state, nonce string) string {
	return "https://accounts.example/o/oauth2/auth?state=" + state + "&nonce=" + nonce
}

func (f fakeAuthenticator) Exchange(ctx context.Context, code string) (string, error) {
	return f.exchange(ctx, code)
}

func (f fakeAuthenticator) Verify(ctx context.Context, rawIDToken string) (user.GoogleProfile, string, error) {
	return f.verify(ctx, rawIDToken)
}

type fakeProvisioner struct {
	provision func(gp user.GoogleProfile) (user.Model, error)
}

func (f fakeProvisioner) ProvisionFromGoogle(gp user.GoogleProfile) (user.Model, error) {
	return f.provision(gp)
}

type fakeIssuer struct {
	mint  func(pr session.Principal) (string, error)
	issue func(userID string) (string, error)
}

func (f fakeIssuer) MintAccess(pr session.Principal) (string, error) { return f.mint(pr) }

func (f fakeIssuer) IssueRefresh(userID string) (string, error) { return f.issue(userID) }

// --- fixtures ---

var testSecret = []byte("test-state-secret")

func testLogger() logrus.FieldLogger {
	l := logrus.New()
	l.SetOutput(nil)
	return l
}

func okAuthenticator() fakeAuthenticator {
	return fakeAuthenticator{
		exchange: func(context.Context, string) (string, error) { return "raw-id-token", nil },
		verify: func(context.Context, string) (user.GoogleProfile, string, error) {
			return user.GoogleProfile{Sub: "g-1", Email: "a@b.com", Name: "Ann"}, "nonce-1", nil
		},
	}
}

// principal is the resolver's happy-path answer. Built in the test rather than
// in oidc's production code, which the arch test forbids from constructing a
// Principal of its own.
func principal(fleetID string) session.Principal {
	return session.Principal{UserID: "u-1", Email: "a@b.com", ActiveFleetID: fleetID, Role: "owner"}
}

// okDeps returns Dependencies whose every collaborator succeeds.
func okDeps() Dependencies {
	return Dependencies{
		OIDC: okAuthenticator(),
		Users: fakeProvisioner{
			provision: func(user.GoogleProfile) (user.Model, error) {
				return user.NewBuilder().SetGoogleSub("g-1").SetEmail("a@b.com").Build(), nil
			},
		},
		Sessions: fakeIssuer{
			mint:  func(session.Principal) (string, error) { return "access-jwt", nil },
			issue: func(string) (string, error) { return "refresh-raw", nil },
		},
		Resolve: func(context.Context, string) (session.Principal, error) {
			return principal("fleet-1"), nil
		},
		StateSecret:    testSecret,
		AppBaseURL:     "http://app.test",
		HomePath:       "/",
		OnboardingPath: "/onboarding",
		LoginPath:      "/login",
		CookieSecure:   false,
	}
}

// stateCookie mints a real signed state cookie by calling the production setter
// against a throwaway recorder — no hand-rolled HMAC in the test.
func stateCookie(t *testing.T, state, nonce string) *http.Cookie {
	t.Helper()
	return stateCookieFor(t, state, nonce, "")
}

// stateCookieFor is stateCookie with an explicit post-login return path.
func stateCookieFor(t *testing.T, state, nonce, returnPath string) *http.Cookie {
	t.Helper()
	rec := httptest.NewRecorder()
	setStateCookie(rec, testSecret, state, nonce, returnPath, false)
	for _, c := range rec.Result().Cookies() {
		if c.Name == stateCookieName {
			return c
		}
	}
	t.Fatalf("setStateCookie set no %s cookie", stateCookieName)
	return nil
}

// cookieRequest returns a callback request carrying the cookies set on rec.
func cookieRequest(rec *httptest.ResponseRecorder) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/auth/callback", nil)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}
	return req
}

// callback drives callbackHandler and hands back the response plus a logrus
// hook holding everything the handler logged.
func callback(t *testing.T, d Dependencies, target string, cookies ...*http.Cookie) (*httptest.ResponseRecorder, *test.Hook) {
	t.Helper()
	log, hook := test.NewNullLogger()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	callbackHandler(log, d).ServeHTTP(rec, req)
	return rec, hook
}

// --- success path ---
//
// Pinned first so the failure-exit rewrite in the following tasks cannot break
// the one path that must keep working.

func TestCallback_successRedirectsHomeWithAccessToken(t *testing.T) {
	rec, _ := callback(t, okDeps(), "/auth/callback?code=abc&state=s-1", stateCookie(t, "s-1", "nonce-1"))

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
	}
	if got, want := rec.Header().Get("Location"), "http://app.test/#access_token=access-jwt"; got != want {
		t.Fatalf("Location = %q, want %q", got, want)
	}
}

func TestCallback_successWithoutFleetGoesToOnboarding(t *testing.T) {
	d := okDeps()
	d.Resolve = func(context.Context, string) (session.Principal, error) { return principal(""), nil }

	rec, _ := callback(t, d, "/auth/callback?code=abc&state=s-1", stateCookie(t, "s-1", "nonce-1"))

	if got, want := rec.Header().Get("Location"), "http://app.test/onboarding#access_token=access-jwt"; got != want {
		t.Fatalf("Location = %q, want %q", got, want)
	}
}

// End-to-end counterpart to TestCallbackDestination_prefersReturnPath: the path
// signed into the state cookie at login must survive the whole dance and beat
// both the home and the onboarding default. A fleetless invitee is the case
// that matters — onboarding would swallow the invite.
func TestCallback_successHonoursReturnPathFromStateCookie(t *testing.T) {
	d := okDeps()
	d.Resolve = func(context.Context, string) (session.Principal, error) { return principal(""), nil }

	rec, _ := callback(t, d, "/auth/callback?code=abc&state=s-1",
		stateCookieFor(t, "s-1", "nonce-1", "/invites/abc123/accept"))

	want := "http://app.test/invites/abc123/accept#access_token=access-jwt"
	if got := rec.Header().Get("Location"); got != want {
		t.Fatalf("Location = %q, want %q", got, want)
	}
}

// clearsStateCookie reports whether the response deletes the state cookie.
// It looks for *any* deleting Set-Cookie rather than counting them, so it
// cannot be fooled by the number of clearing sites changing.
func clearsStateCookie(rec *httptest.ResponseRecorder) bool {
	for _, c := range rec.Result().Cookies() {
		if c.Name == stateCookieName && c.MaxAge < 0 {
			return true
		}
	}
	return false
}

func loggedAt(hook *test.Hook, level logrus.Level, msg string) bool {
	for _, e := range hook.AllEntries() {
		if e.Level == level && e.Message == msg {
			return true
		}
	}
	return false
}

// TestCallback_failureRedirectsToLogin walks every failure exit in
// callbackHandler. Each case asserts the 302, the exact Location, whether the
// abandoned attempt's state cookie is cleared, and that the operator-facing log
// line still fires unchanged (FR-ERR-7).
//
// wantCookieCleared splits the table in two, and the split is the point.
// /auth/callback is public and unauthenticated, so the exits BEFORE
// verifyStateCookie are reachable by any third party who can make a victim's
// browser issue the request; clearing there would let them kill an in-flight
// login they never touched. Those cases assert the cookie is left ALONE.
// Everything at or after successful state verification has proven ownership of
// the attempt and still clears. This deliberately departs from plan Global
// Constraint FR-ERR-8 — see failLogin's doc comment for the adjudication.
func TestCallback_failureRedirectsToLogin(t *testing.T) {
	boom := errors.New("boom")

	cases := []struct {
		name              string
		target            string
		withCookie        bool
		mutate            func(d *Dependencies)
		code              string
		logMsg            string
		wantCookieCleared bool
	}{
		// --- pre-verification: the cookie must survive ---
		{
			name:   "missing code",
			target: "/auth/callback?state=s-1",
			code:   "invalid_state",
		},
		{
			name:   "missing state",
			target: "/auth/callback?code=abc",
			code:   "invalid_state",
		},
		{
			name:   "no state cookie",
			target: "/auth/callback?code=abc&state=s-1",
			code:   "invalid_state",
		},
		// A state cookie IS present, but it does not match the callback's state
		// parameter, so verification fails. Still a pre-verification exit: an
		// attacker who guesses nothing can replay /auth/callback?code=x&state=x
		// against a victim mid-login, and must not be able to void the cookie.
		{
			name:       "state cookie does not match the state parameter",
			target:     "/auth/callback?code=abc&state=s-other",
			withCookie: true,
			code:       "invalid_state",
		},
		// --- at or after verification: the cookie is cleared ---
		{
			name:       "code exchange fails",
			target:     "/auth/callback?code=abc&state=s-1",
			withCookie: true,
			mutate: func(d *Dependencies) {
				a := okAuthenticator()
				a.exchange = func(context.Context, string) (string, error) { return "", boom }
				d.OIDC = a
			},
			code:              "auth_failed",
			logMsg:            "oidc code exchange",
			wantCookieCleared: true,
		},
		{
			name:       "id_token verification fails",
			target:     "/auth/callback?code=abc&state=s-1",
			withCookie: true,
			mutate: func(d *Dependencies) {
				a := okAuthenticator()
				a.verify = func(context.Context, string) (user.GoogleProfile, string, error) {
					return user.GoogleProfile{}, "", boom
				}
				d.OIDC = a
			},
			code:              "auth_failed",
			logMsg:            "oidc id_token verification",
			wantCookieCleared: true,
		},
		{
			name:       "nonce mismatch",
			target:     "/auth/callback?code=abc&state=s-1",
			withCookie: true,
			mutate: func(d *Dependencies) {
				a := okAuthenticator()
				a.verify = func(context.Context, string) (user.GoogleProfile, string, error) {
					return user.GoogleProfile{Sub: "g-1"}, "a-different-nonce", nil
				}
				d.OIDC = a
			},
			code:              "auth_failed",
			logMsg:            "oidc nonce mismatch",
			wantCookieCleared: true,
		},
		{
			name:       "provisioning fails",
			target:     "/auth/callback?code=abc&state=s-1",
			withCookie: true,
			mutate: func(d *Dependencies) {
				d.Users = fakeProvisioner{
					provision: func(user.GoogleProfile) (user.Model, error) { return user.Model{}, boom },
				}
			},
			code:              "server_error",
			logMsg:            "provision user from google",
			wantCookieCleared: true,
		},
		{
			name:       "principal resolution fails",
			target:     "/auth/callback?code=abc&state=s-1",
			withCookie: true,
			mutate: func(d *Dependencies) {
				d.Resolve = func(context.Context, string) (session.Principal, error) {
					return session.Principal{}, boom
				}
			},
			code:              "server_error",
			logMsg:            "resolve principal on callback",
			wantCookieCleared: true,
		},
		{
			name:       "access mint fails",
			target:     "/auth/callback?code=abc&state=s-1",
			withCookie: true,
			mutate: func(d *Dependencies) {
				d.Sessions = fakeIssuer{
					mint:  func(session.Principal) (string, error) { return "", boom },
					issue: func(string) (string, error) { return "refresh-raw", nil },
				}
			},
			code:              "server_error",
			logMsg:            "mint access on callback",
			wantCookieCleared: true,
		},
		{
			name:       "refresh issue fails",
			target:     "/auth/callback?code=abc&state=s-1",
			withCookie: true,
			mutate: func(d *Dependencies) {
				d.Sessions = fakeIssuer{
					mint:  func(session.Principal) (string, error) { return "access-jwt", nil },
					issue: func(string) (string, error) { return "", boom },
				}
			},
			code:              "server_error",
			logMsg:            "issue refresh on callback",
			wantCookieCleared: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := okDeps()
			if c.mutate != nil {
				c.mutate(&d)
			}
			var cookies []*http.Cookie
			if c.withCookie {
				cookies = append(cookies, stateCookie(t, "s-1", "nonce-1"))
			}

			rec, hook := callback(t, d, c.target, cookies...)

			if rec.Code != http.StatusFound {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
			}
			want := "http://app.test/login#error=" + c.code
			if got := rec.Header().Get("Location"); got != want {
				t.Errorf("Location = %q, want %q", got, want)
			}
			if got := clearsStateCookie(rec); got != c.wantCookieCleared {
				if c.wantCookieCleared {
					t.Errorf("expected a Set-Cookie deleting %s", stateCookieName)
				} else {
					t.Errorf("expected NO Set-Cookie deleting %s: this exit runs before "+
						"state verification, and any third party can reach it", stateCookieName)
				}
			}
			if c.logMsg != "" && !loggedAt(hook, logrus.ErrorLevel, c.logMsg) {
				t.Errorf("expected an error log %q, got %+v", c.logMsg, hook.AllEntries())
			}
		})
	}
}

// FR-ERR-2: the login path is configuration, not a constant.
func TestCallback_failureHonoursLoginPath(t *testing.T) {
	d := okDeps()
	d.LoginPath = "/signin"

	rec, _ := callback(t, d, "/auth/callback?code=abc")

	if got, want := rec.Header().Get("Location"), "http://app.test/signin#error=invalid_state"; got != want {
		t.Fatalf("Location = %q, want %q", got, want)
	}
}

// FR-ERR-6 as refined by design §3.3: declining consent is a normal outcome and
// gets its own code, so the SPA can present it neutrally.
func TestCallback_providerAccessDeniedIsCancelled(t *testing.T) {
	rec, hook := callback(t, okDeps(), "/auth/callback?error=access_denied&state=s-1")

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
	}
	if got, want := rec.Header().Get("Location"), "http://app.test/login#error=cancelled"; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
	// This is THE exit an attacker reaches for: /auth/callback?error=access_denied
	// needs no code, no state and no secret, so if it cleared the cookie, any
	// page that can navigate a victim's browser could void an in-flight login.
	if clearsStateCookie(rec) {
		t.Errorf("provider-error exit must NOT delete %s — it runs before state "+
			"verification and is reachable by anyone", stateCookieName)
	}
	// Info, not Error: a cancel must not inflate the error rate, but a spike in
	// access_denied is still a signal worth having.
	if !loggedAt(hook, logrus.InfoLevel, "oidc callback returned provider error") {
		t.Errorf("expected an info log, got %+v", hook.AllEntries())
	}
}

// Design §3.3 / §9: every other provider error is a misconfiguration, not a
// user choice. Reporting invalid_scope as "cancelled" would hide a real fault.
func TestCallback_otherProviderErrorIsAuthFailed(t *testing.T) {
	rec, hook := callback(t, okDeps(), "/auth/callback?error=invalid_scope&state=s-1")

	if got, want := rec.Header().Get("Location"), "http://app.test/login#error=auth_failed"; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
	if clearsStateCookie(rec) {
		t.Errorf("provider-error exit must NOT delete %s — it runs before state "+
			"verification and is reachable by anyone", stateCookieName)
	}
	entries := hook.AllEntries()
	if len(entries) != 1 || entries[0].Data["oauth_error"] != "invalid_scope" {
		t.Errorf("expected one entry carrying oauth_error=invalid_scope, got %+v", entries)
	}
}

// --- post-login return path ---

// storedReturnPath decodes the return path a handler actually WROTE into the
// state cookie. Reading it back through verifyStateCookie would not do: that
// re-sanitizes on read, so it reports "" for a hostile value whether or not the
// handler sanitized on write. Only the raw payload pins the write side.
func storedReturnPath(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected exactly one cookie, got %d", len(cookies))
	}
	raw, err := base64.RawURLEncoding.DecodeString(cookies[0].Value)
	if err != nil {
		t.Fatalf("decode cookie: %v", err)
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 5 {
		t.Fatalf("expected 5 cookie fields, got %d", len(parts))
	}
	decoded, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil {
		t.Fatalf("decode return path field %q: %v", parts[3], err)
	}
	return string(decoded)
}

func TestSafeReturnPath_acceptsSiteRelativePaths(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"/invites/abc123/accept", "/invites/abc123/accept"},
		{"/vehicles?sort=name", "/vehicles?sort=name"},
		{"/", "/"},
		// A fragment would collide with the "#access_token=" the callback
		// appends, so it is dropped rather than carried through.
		{"/settings#members", "/settings"},
		// 512 bytes is the documented ceiling and must still be accepted.
		{"/" + strings.Repeat("a", maxReturnPathLen-1), "/" + strings.Repeat("a", maxReturnPathLen-1)},
		// Dot segments and duplicate slashes are collapsed rather than rejected.
		// What matters is that the result can no longer resolve to an authority:
		// both of these read as "//evil.example" to a browser if left alone.
		{"/.//evil.example", "/evil.example"},
		{"/x/../..//evil.example", "/evil.example"},
		{"/invites/abc123/accept/", "/invites/abc123/accept"},
	} {
		t.Run(tc.in, func(t *testing.T) {
			if got := safeReturnPath(tc.in); got != tc.want {
				t.Errorf("safeReturnPath(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestSafeReturnPath_rejectsOffSiteAndMalformed(t *testing.T) {
	for _, in := range []string{
		"",
		"https://evil.example/steal",
		"//evil.example/steal",       // protocol-relative
		"/\\evil.example/steal",      // browsers treat /\ as protocol-relative too
		"javascript:alert(1)",        // scheme
		"vehicles",                   // not site-relative
		"/vehicles\r\nSet-Cookie: x", // header injection attempt
		// Same-origin but routed to the services, not the SPA: the access-token
		// fragment would be stranded on a JSON 401.
		"/api/fleet/vehicles",
		"/" + strings.Repeat("a", maxReturnPathLen),
	} {
		t.Run(in, func(t *testing.T) {
			if got := safeReturnPath(in); got != "" {
				t.Errorf("safeReturnPath(%q) = %q, want rejected", in, got)
			}
		})
	}
}

func TestStateCookie_roundTripsReturnPath(t *testing.T) {
	rec := httptest.NewRecorder()
	setStateCookie(rec, testSecret, "st", "nn", "/invites/abc123/accept", false)

	nonce, returnPath, ok := verifyStateCookie(cookieRequest(rec), testSecret, "st")
	if !ok {
		t.Fatal("state cookie should verify")
	}
	if nonce != "nn" {
		t.Fatalf("nonce = %q, want %q", nonce, "nn")
	}
	if returnPath != "/invites/abc123/accept" {
		t.Fatalf("returnPath = %q, want /invites/abc123/accept", returnPath)
	}
}

func TestStateCookie_roundTripsEmptyReturnPath(t *testing.T) {
	rec := httptest.NewRecorder()
	setStateCookie(rec, testSecret, "st", "nn", "", false)

	nonce, returnPath, ok := verifyStateCookie(cookieRequest(rec), testSecret, "st")
	if !ok || nonce != "nn" || returnPath != "" {
		t.Fatalf("empty-path cookie: nonce=%q path=%q ok=%v", nonce, returnPath, ok)
	}
}

func TestStateCookie_rejectsTamperedReturnPath(t *testing.T) {
	rec := httptest.NewRecorder()
	setStateCookie(rec, testSecret, "st", "nn", "/vehicles", false)

	raw, err := base64.RawURLEncoding.DecodeString(rec.Result().Cookies()[0].Value)
	if err != nil {
		t.Fatalf("decode cookie: %v", err)
	}
	parts := strings.Split(string(raw), "|")
	parts[3] = base64.RawURLEncoding.EncodeToString([]byte("https://evil.example"))
	tampered := base64.RawURLEncoding.EncodeToString([]byte(strings.Join(parts, "|")))

	req := httptest.NewRequest(http.MethodGet, "/auth/callback", nil)
	req.AddCookie(&http.Cookie{Name: stateCookieName, Value: tampered})

	if _, _, ok := verifyStateCookie(req, testSecret, "st"); ok {
		t.Fatal("a tampered return path must fail the signature check")
	}
}

// A login started on the previous build must still complete after this one
// deploys, so the pre-return-path cookie format stays acceptable for a release.
func TestStateCookie_acceptsLegacyFourFieldFormat(t *testing.T) {
	exp := itoa(time.Now().Add(stateTTL).Unix())
	payload := "st|nn|" + exp
	value := base64.RawURLEncoding.EncodeToString([]byte(payload + "|" + sign(testSecret, payload)))

	req := httptest.NewRequest(http.MethodGet, "/auth/callback", nil)
	req.AddCookie(&http.Cookie{Name: stateCookieName, Value: value})

	nonce, returnPath, ok := verifyStateCookie(req, testSecret, "st")
	if !ok {
		t.Fatal("legacy 4-field cookie should still verify during rollout")
	}
	if nonce != "nn" || returnPath != "" {
		t.Fatalf("legacy cookie: nonce=%q returnPath=%q", nonce, returnPath)
	}
}

func TestStateCookie_rejectsForgedLegacyFormat(t *testing.T) {
	exp := itoa(time.Now().Add(stateTTL).Unix())
	payload := "st|nn|" + exp
	value := base64.RawURLEncoding.EncodeToString([]byte(payload + "|" + sign([]byte("wrong-secret"), payload)))

	req := httptest.NewRequest(http.MethodGet, "/auth/callback", nil)
	req.AddCookie(&http.Cookie{Name: stateCookieName, Value: value})

	if _, _, ok := verifyStateCookie(req, testSecret, "st"); ok {
		t.Fatal("legacy tolerance must not skip the signature check")
	}
}

// Drives the real handler: it must sanitize return_to and persist the result.
// NewProcessor is a pure struct constructor and AuthCodeURL is string building,
// so no network is involved.
func TestLoginHandler_storesSanitizedReturnPath(t *testing.T) {
	d := Dependencies{
		OIDC:        NewProcessor("client-id", "client-secret", "http://auth.local/auth/callback"),
		StateSecret: testSecret,
	}

	for _, tc := range []struct{ name, query, want string }{
		{"site-relative path is kept", "?return_to=%2Finvites%2Fabc123%2Faccept", "/invites/abc123/accept"},
		{"off-site url is dropped", "?return_to=https%3A%2F%2Fevil.example", ""},
		{"protocol-relative url is dropped", "?return_to=%2F%2Fevil.example", ""},
		{"absent param yields no return path", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/auth/login/google"+tc.query, nil)
			loginHandler(testLogger(), d)(rec, req)

			if rec.Code != http.StatusFound {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
			}

			// The state the handler signed into the cookie is the one it sent to
			// Google, so read it back off the redirect to verify the cookie.
			loc, err := rec.Result().Location()
			if err != nil {
				t.Fatalf("redirect location: %v", err)
			}
			state := loc.Query().Get("state")
			if state == "" {
				t.Fatal("redirect carries no state")
			}
			if _, _, ok := verifyStateCookie(cookieRequest(rec), testSecret, state); !ok {
				t.Fatal("handler's state cookie should verify")
			}

			if got := storedReturnPath(t, rec); got != tc.want {
				t.Errorf("stored returnPath = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCallbackDestination_prefersReturnPath(t *testing.T) {
	// Distinct values so transposed arguments cannot coincidentally pass.
	d := Dependencies{AppBaseURL: "http://app.local", HomePath: "/home", OnboardingPath: "/onboarding"}

	for _, tc := range []struct{ name, fleetID, returnPath, want string }{
		// A fleetless invitee must land on the accept route, not onboarding.
		{"return path wins for fleetless user", "", "/invites/abc123/accept", "http://app.local/invites/abc123/accept"},
		{"return path wins for fleeted user", "f1", "/vehicles", "http://app.local/vehicles"},
		{"fleetless default is onboarding", "", "", "http://app.local/onboarding"},
		{"fleeted default is home", "f1", "", "http://app.local/home"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pr := session.Principal{UserID: "u1", ActiveFleetID: tc.fleetID}
			if got := destination(d, pr, tc.returnPath); got != tc.want {
				t.Errorf("destination(%q, %q) = %q, want %q", tc.fleetID, tc.returnPath, got, tc.want)
			}
		})
	}
}
