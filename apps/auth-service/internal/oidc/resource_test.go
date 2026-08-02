package oidc

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

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

func okAuthenticator() fakeAuthenticator {
	return fakeAuthenticator{
		exchange: func(context.Context, string) (string, error) { return "raw-id-token", nil },
		verify: func(context.Context, string) (user.GoogleProfile, string, error) {
			return user.GoogleProfile{Sub: "g-1", Email: "a@b.com", Name: "Ann"}, "nonce-1", nil
		},
	}
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
		Resolve:        func(context.Context, string) (string, string, error) { return "fleet-1", "owner", nil },
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
	rec := httptest.NewRecorder()
	setStateCookie(rec, testSecret, state, nonce, false)
	for _, c := range rec.Result().Cookies() {
		if c.Name == stateCookieName {
			return c
		}
	}
	t.Fatalf("setStateCookie set no %s cookie", stateCookieName)
	return nil
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
	d.Resolve = func(context.Context, string) (string, string, error) { return "", "", nil }

	rec, _ := callback(t, d, "/auth/callback?code=abc&state=s-1", stateCookie(t, "s-1", "nonce-1"))

	if got, want := rec.Header().Get("Location"), "http://app.test/onboarding#access_token=access-jwt"; got != want {
		t.Fatalf("Location = %q, want %q", got, want)
	}
}

// clearsStateCookie reports whether the response deletes the state cookie.
// Late exits clear it twice — once by the existing call at the top of the
// handler, once by failLogin — so this looks for *any* deleting Set-Cookie
// rather than counting them (design §3.1).
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
// callbackHandler. Each case asserts the 302, the exact Location, that the
// abandoned attempt's state cookie is cleared (FR-ERR-8), and that the
// operator-facing log line still fires unchanged (FR-ERR-7).
func TestCallback_failureRedirectsToLogin(t *testing.T) {
	boom := errors.New("boom")

	cases := []struct {
		name       string
		target     string
		withCookie bool
		mutate     func(d *Dependencies)
		code       string
		logMsg     string
	}{
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
		{
			name:       "code exchange fails",
			target:     "/auth/callback?code=abc&state=s-1",
			withCookie: true,
			mutate: func(d *Dependencies) {
				a := okAuthenticator()
				a.exchange = func(context.Context, string) (string, error) { return "", boom }
				d.OIDC = a
			},
			code:   "auth_failed",
			logMsg: "oidc code exchange",
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
			code:   "auth_failed",
			logMsg: "oidc id_token verification",
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
			code:   "auth_failed",
			logMsg: "oidc nonce mismatch",
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
			code:   "server_error",
			logMsg: "provision user from google",
		},
		{
			name:       "membership resolution fails",
			target:     "/auth/callback?code=abc&state=s-1",
			withCookie: true,
			mutate: func(d *Dependencies) {
				d.Resolve = func(context.Context, string) (string, string, error) { return "", "", boom }
			},
			code:   "server_error",
			logMsg: "resolve membership on callback",
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
			code:   "server_error",
			logMsg: "mint access on callback",
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
			code:   "server_error",
			logMsg: "issue refresh on callback",
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
			if !clearsStateCookie(rec) {
				t.Errorf("expected a Set-Cookie deleting %s", stateCookieName)
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
