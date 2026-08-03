# Login Page Redesign & Recoverable OAuth Failures Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rewrite `/login` as a typographic, card-less page with ready/redirecting/failed states, and make every `callbackHandler` failure redirect back to `/login#error=<code>` instead of dead-ending on a plaintext error page.

**Architecture:** auth-service gains a single `failLogin` helper that clears the state cookie and 302s to `{AppBaseURL}{LoginPath}#error=<code>`, replacing nine `http.Error` exits; three `Dependencies` fields become consumer-side interfaces so the handler is testable without Google or a database. The SPA gains `lib/auth/loginError.ts`, which reads-and-strips the `#error` fragment exactly once per page load (module-scope memoisation), and `LoginPage.tsx` is rewritten around it. `ThemeToggle`'s presentational half is extracted to `ThemeToggleButton` so the login page can offer a theme control with no authenticated `PATCH` behind it.

**Tech Stack:** Go 1.25 (chi, logrus, `net/http/httptest`, `logrus/hooks/test`), React 18 + TypeScript, Vite, Vitest + Testing Library, Tailwind with the project's semantic token layer, shadcn `Button`, `lucide-react`.

## Global Constraints

> **SUPERSEDED — FR-ERR-8 ("the state cookie is cleared on every failure
> path").** Review found this makes an unauthenticated denial-of-login
> possible: `GET /auth/callback` is public and carries no CSRF token, so the
> provider-`?error=` pre-check and the missing-`code`/`state` exit run before
> `verifyStateCookie` and are reachable by any third party who can make a
> victim's browser issue the request — clearing there destroys an in-flight
> `oidc_state` they never touched. The pre-diff `http.Error` exits left it
> intact, so clearing was a regression. **Ruled by the user: the finding
> governs, not the plan.** The cookie is now cleared exactly once, immediately
> after `verifyStateCookie` succeeds; exits before that point leave it alone.
> Recorded in `failLogin`'s doc comment and in the test table's
> `wantCookieCleared` field. Commit `e3b9a04`.

- **Do not add dependencies.** `lucide-react`, `Button`, `logrus`, and `logrus/hooks/test` (ships inside the existing `github.com/sirupsen/logrus v1.9.4` module) are all already available.
- **Only existing CSS tokens.** No new custom properties in `apps/web/src/index.css`. The danger callout uses exactly `border-danger-border bg-danger-subtle text-danger-subtle-foreground`.
- **No hardcoded palette classes in `.tsx`.** `apps/web/src/test/conventions.test.ts:113-154` fails any `.tsx` matching `(bg|text|border|ring|divide)-(gray|slate|zinc|neutral|white|black|red|green|blue|amber|yellow|emerald|orange)`. The Google mark's white backing is an in-SVG `fill="#FFFFFF"`, which is outside that regex by construction.
- **Exactly four error codes**, spelled: `cancelled`, `invalid_state`, `auth_failed`, `server_error`. No others, ever, on the wire.
- **Error code travels in the URL fragment**, never the query string.
- **Server-side logging is unchanged.** Every existing `log.WithError(...).Error("<msg>")` line in `callbackHandler` keeps its exact message and fields. `failLogin` replaces only the `http.Error` call that followed it.
- **`ThemeToggle.test.tsx` is not edited.** It must keep passing byte-for-byte (PRD acceptance criterion).
- **Failure copy, verbatim:**
  - generic (danger): `Sign-in didn't complete. Nothing was saved — try again, or use a different Google account.`
  - cancelled (neutral): `Sign-in cancelled.`
- **Headline, verbatim:** `Your cars.` on line one, `One place.` on line two.
- **Do not touch** `/onboarding`, `/invites/:token/accept`, `deploy/`, token minting, refresh rotation, cookie flags, or the HMAC state scheme.
- **Design deviation carried forward (design §9):** Google's `?error=access_denied` maps to `cancelled`; **any other** `?error=` value maps to `auth_failed`. This refines PRD FR-ERR-6, which mapped all of them to `cancelled`.

---

## File Structure

| File | Responsibility |
| --- | --- |
| `apps/auth-service/internal/oidc/resource.go` (modify) | Consumer interfaces, `LoginPath`, `loginErrorCode` constants, `failLogin`, the rewritten failure exits, the provider-`error` pre-check |
| `apps/auth-service/internal/oidc/resource_test.go` (create) | Fakes + success path + the full failure table + provider-error cases |
| `apps/auth-service/cmd/main.go` (modify) | Wire `APP_LOGIN_PATH` |
| `apps/web/src/lib/auth/loginError.ts` (create) | Read/normalise/strip `#error=`; map code → tone + copy |
| `apps/web/src/lib/auth/loginError.test.ts` (create) | Parsing, normalisation, stripping, memoisation |
| `apps/web/src/components/ThemeToggleButton.tsx` (create) | Presentational theme cycle control — icon, cycle map, aria contract |
| `apps/web/src/components/ThemeToggleButton.test.tsx` (create) | The cycle, the labels, preference-not-resolved-theme icon rule |
| `apps/web/src/components/ThemeToggle.tsx` (modify) | Reduced to the authenticated mutation wrapper |
| `apps/web/src/components/GoogleMark.tsx` (create) | The four-colour Google "G" with its own white backing |
| `apps/web/src/pages/LoginPage.tsx` (modify) | Direction D composition + the three page states |
| `apps/web/src/pages/LoginPage.test.tsx` (create) | Composition, three states, no-network theme control, auth bounce |

---

### Task 1: Make `callbackHandler` testable — consumer-side interfaces

Six of the nine failure exits sit behind concrete processor types, which is why `resource.go` has no test file today. Narrowing three `Dependencies` fields to interfaces declared in `oidc` makes every exit reachable from a test. `*oidc.Processor`, `*user.Processor` and `*session.Processor` satisfy them implicitly, so `cmd/main.go` compiles unchanged.

**Files:**
- Modify: `apps/auth-service/internal/oidc/resource.go:1-52` (imports, `Dependencies`)
- Test: `apps/auth-service/internal/oidc/resource_test.go` (create)

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: `oidc.Authenticator`, `oidc.UserProvisioner`, `oidc.TokenIssuer` (exported interfaces on `Dependencies`); test helpers `okDeps() Dependencies`, `okAuthenticator() fakeAuthenticator`, `stateCookie(t *testing.T, state, nonce string) *http.Cookie`, `callback(t *testing.T, d Dependencies, target string, cookies ...*http.Cookie) (*httptest.ResponseRecorder, *test.Hook)` — Tasks 2 and 3 build directly on all four.

- [x] **Step 1: Write the failing test**

Create `apps/auth-service/internal/oidc/resource_test.go`:

```go
package oidc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

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
```

- [x] **Step 2: Run the test to verify it fails**

Run: `go test github.com/jtumidanski/myfleet/apps/auth-service/internal/oidc -run TestCallback -v`
Expected: FAIL to compile — `cannot use fakeAuthenticator{} (value of type fakeAuthenticator) as *Processor value in struct literal` (and the same for `Users` and `Sessions`).

- [x] **Step 3: Narrow the three `Dependencies` fields to interfaces**

In `apps/auth-service/internal/oidc/resource.go`, add `"context"` to the import block (first entry, before `"crypto/hmac"`), then insert the interfaces immediately above `Dependencies` (currently line 26) and change the three field types:

```go
// Authenticator is the OIDC surface the callback needs. Declared here, at the
// consumer, so the handler can be exercised without a live Google endpoint
// (design §3.2). *Processor satisfies it implicitly.
type Authenticator interface {
	AuthCodeURL(state, nonce string) string
	Exchange(ctx context.Context, code string) (string, error)
	Verify(ctx context.Context, rawIDToken string) (user.GoogleProfile, string, error)
}

// UserProvisioner is the single user-store operation the callback performs.
type UserProvisioner interface {
	ProvisionFromGoogle(gp user.GoogleProfile) (user.Model, error)
}

// TokenIssuer mints the pair the browser leaves with.
type TokenIssuer interface {
	MintAccess(pr session.Principal) (string, error)
	IssueRefresh(userID string) (string, error)
}

// Dependencies bundles everything the callback orchestration needs. The
// membership resolver is injected (Decision 1) so this package never imports
// the concrete membership client.
type Dependencies struct {
	OIDC        Authenticator
	Users       UserProvisioner
	Sessions    TokenIssuer
	Resolve     session.MembershipResolver
	StateSecret []byte
	// AppBaseURL is the SPA origin the browser is redirected back to.
	AppBaseURL string
	// HomePath / OnboardingPath are relative paths under AppBaseURL.
	HomePath       string
	OnboardingPath string
	// CookieSecure controls the Secure flag on cookies this package sets. It is
	// false for local plaintext HTTP (Traefik :80) and true in production.
	CookieSecure bool
}
```

Nothing else in `resource.go` changes: every call site (`d.OIDC.AuthCodeURL`, `d.OIDC.Exchange`, `d.OIDC.Verify`, `d.Users.ProvisionFromGoogle`, `d.Sessions.MintAccess`, `d.Sessions.IssueRefresh`) is already method-only.

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test github.com/jtumidanski/myfleet/apps/auth-service/internal/oidc -run TestCallback -v`
Expected: PASS (2 tests).

- [x] **Step 5: Verify `cmd/main.go` still compiles against the new types**

Run: `make build && make vet`
Expected: both succeed with no output. This is the check that the three concrete processors satisfy the interfaces — if a signature were transcribed wrong, `oidcDeps` in `apps/auth-service/cmd/main.go:72-82` would fail to compile.

- [x] **Step 6: Commit**

```bash
git add apps/auth-service/internal/oidc/resource.go apps/auth-service/internal/oidc/resource_test.go
git commit -m "refactor(auth): narrow oidc Dependencies to consumer-side interfaces"
```

---

### Task 2: Redirect every failure exit to `/login#error=<code>`

Nine `http.Error` calls collapse into one `failLogin` helper. `Dependencies` gains `LoginPath`, wired from `APP_LOGIN_PATH`.

**Files:**
- Modify: `apps/auth-service/internal/oidc/resource.go:26-43` (`Dependencies`), `:65-131` (`callbackHandler` failure exits), and a new helper block after `callbackHandler`
- Modify: `apps/auth-service/cmd/main.go:72-82`
- Test: `apps/auth-service/internal/oidc/resource_test.go` (append)

**Interfaces:**
- Consumes: `okDeps()`, `okAuthenticator()`, `stateCookie()`, `callback()` from Task 1.
- Produces: `loginErrorCode` (unexported string type) with constants `errCancelled`, `errInvalidState`, `errAuthFailed`, `errServerError`; `failLogin(w http.ResponseWriter, req *http.Request, d Dependencies, code loginErrorCode)`; `Dependencies.LoginPath string`. Task 3 calls `failLogin` with `errCancelled` and `errAuthFailed`.

- [x] **Step 1: Write the failing tests**

First add `LoginPath: "/login",` to `okDeps()` in `resource_test.go`, immediately after the `OnboardingPath` line:

```go
		OnboardingPath: "/onboarding",
		LoginPath:      "/login",
		CookieSecure:   false,
```

Then extend the import block with `"errors"` (after `"context"`) and `"github.com/sirupsen/logrus"` (as the first entry of the third group, above `logrus/hooks/test`), and append:

```go
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
```

- [x] **Step 2: Run the tests to verify they fail**

Run: `go test github.com/jtumidanski/myfleet/apps/auth-service/internal/oidc -run TestCallback_failure -v`
Expected: FAIL to compile — `unknown field LoginPath in struct literal of type Dependencies`.

- [x] **Step 3: Add `LoginPath`, the code constants, and `failLogin`**

In `apps/auth-service/internal/oidc/resource.go`, add the field to `Dependencies` beside its siblings:

```go
	// HomePath / OnboardingPath / LoginPath are relative paths under AppBaseURL.
	HomePath       string
	OnboardingPath string
	LoginPath      string
```

Then add this block immediately after `InitializeRoutes` (i.e. above `loginHandler`):

```go
// loginErrorCode is the coarse, browser-visible outcome of a failed callback.
// Deliberately not derived from the underlying error: nothing about the
// failure's internals reaches the SPA (FR-ERR-4). A typed constant set means
// the compiler, not review, enforces the closed vocabulary.
type loginErrorCode string

const (
	errCancelled    loginErrorCode = "cancelled"
	errInvalidState loginErrorCode = "invalid_state"
	errAuthFailed   loginErrorCode = "auth_failed"
	errServerError  loginErrorCode = "server_error"
)

// failLogin returns the browser to the SPA's login page carrying a coarse
// reason, instead of dead-ending on a plaintext error body.
//
// The state cookie is cleared on every path (FR-ERR-8): each of these exits
// abandons the attempt, and the page the user lands on offers "Try again", so a
// stale signed state must not survive to collide with the next attempt's. The
// clear must precede http.Redirect, which calls WriteHeader — headers set after
// that are dropped silently.
//
// The location is composed entirely from server configuration plus a constant,
// so there is no open-redirect surface (FR-ERR-9). It is deliberately NOT
// query-escaped: escaping would turn the "#" into "%23" and put the code in the
// path rather than the fragment.
func failLogin(w http.ResponseWriter, req *http.Request, d Dependencies, code loginErrorCode) {
	clearStateCookie(w, d.CookieSecure)
	http.Redirect(w, req, d.AppBaseURL+d.LoginPath+"#error="+string(code), http.StatusFound)
}
```

- [x] **Step 4: Replace the nine `http.Error` exits**

In `callbackHandler`, replace each `http.Error(...)` line — and only that line — with the matching `failLogin` call. The `log.WithError(...)` lines above them are untouched.

```go
		if code == "" || state == "" {
			failLogin(w, req, d, errInvalidState)
			return
		}
		wantNonce, ok := verifyStateCookie(req, d.StateSecret, state)
		if !ok {
			failLogin(w, req, d, errInvalidState)
			return
		}
		clearStateCookie(w, d.CookieSecure)

		ctx := req.Context()
		rawIDToken, err := d.OIDC.Exchange(ctx, code)
		if err != nil {
			log.WithError(err).Error("oidc code exchange")
			failLogin(w, req, d, errAuthFailed)
			return
		}
		profile, gotNonce, err := d.OIDC.Verify(ctx, rawIDToken)
		if err != nil {
			log.WithError(err).Error("oidc id_token verification")
			failLogin(w, req, d, errAuthFailed)
			return
		}
		// idtoken.Validate does not check the nonce; bind the id_token to this
		// login attempt by comparing its nonce to the one in the state cookie.
		if gotNonce == "" || !hmac.Equal([]byte(gotNonce), []byte(wantNonce)) {
			log.Error("oidc nonce mismatch")
			failLogin(w, req, d, errAuthFailed)
			return
		}

		u, err := d.Users.ProvisionFromGoogle(profile)
		if err != nil {
			log.WithError(err).Error("provision user from google")
			failLogin(w, req, d, errServerError)
			return
		}

		fleetID, role, err := d.Resolve(ctx, u.ID())
		if err != nil {
			log.WithError(err).Error("resolve membership on callback")
			failLogin(w, req, d, errServerError)
			return
		}

		access, err := d.Sessions.MintAccess(session.Principal{
			UserID:        u.ID(),
			Email:         u.Email(),
			ActiveFleetID: fleetID,
			Role:          role,
		})
		if err != nil {
			log.WithError(err).Error("mint access on callback")
			failLogin(w, req, d, errServerError)
			return
		}
		refresh, err := d.Sessions.IssueRefresh(u.ID())
		if err != nil {
			log.WithError(err).Error("issue refresh on callback")
			failLogin(w, req, d, errServerError)
			return
		}
```

- [x] **Step 5: Confirm no `http.Error` remains**

Run: `grep -c "http.Error" apps/auth-service/internal/oidc/resource.go`
Expected: `0`

- [x] **Step 6: Run the tests to verify they pass**

Run: `go test github.com/jtumidanski/myfleet/apps/auth-service/internal/oidc -v`
Expected: PASS — `TestCallback_successRedirectsHomeWithAccessToken`, `TestCallback_successWithoutFleetGoesToOnboarding`, all ten subtests of `TestCallback_failureRedirectsToLogin`, `TestCallback_failureHonoursLoginPath`, and the pre-existing `TestProfileFromClaims_extractsFields`.

- [x] **Step 7: Wire `APP_LOGIN_PATH` in `cmd/main.go`**

In `apps/auth-service/cmd/main.go`, add one line to the `oidc.Dependencies` literal, after `OnboardingPath`:

```go
	oidcDeps := oidc.Dependencies{
		OIDC:           oidcProc,
		Users:          users,
		Sessions:       sess,
		Resolve:        resolve,
		StateSecret:    []byte(config.MustGet("OIDC_STATE_SECRET")),
		AppBaseURL:     config.Get("APP_BASE_URL", "http://localhost"),
		HomePath:       config.Get("APP_HOME_PATH", "/"),
		OnboardingPath: config.Get("APP_ONBOARDING_PATH", "/onboarding"),
		LoginPath:      config.Get("APP_LOGIN_PATH", "/login"),
		CookieSecure:   cookieSecure,
	}
```

No `deploy/` change: `APP_HOME_PATH` and `APP_ONBOARDING_PATH` are likewise absent from the manifests and rely on their Go defaults.

- [x] **Step 8: Verify the service builds**

Run: `make build && make vet && go test -race github.com/jtumidanski/myfleet/apps/auth-service/...`
Expected: all pass.

- [x] **Step 9: Commit**

```bash
git add apps/auth-service/internal/oidc/resource.go apps/auth-service/internal/oidc/resource_test.go apps/auth-service/cmd/main.go
git commit -m "feat(auth): redirect callback failures to /login#error instead of plaintext errors"
```

---

### Task 3: Handle Google's `?error=` before anything else

Today a user who clicks **Cancel** at Google's consent screen returns with no `code` and falls into the generic missing-code exit. A dedicated pre-check separates a deliberate cancel from a real fault.

**Files:**
- Modify: `apps/auth-service/internal/oidc/resource.go` (top of `callbackHandler`)
- Test: `apps/auth-service/internal/oidc/resource_test.go` (append)

**Interfaces:**
- Consumes: `failLogin`, `errCancelled`, `errAuthFailed` (Task 2); `okDeps()`, `callback()`, `clearsStateCookie()`, `loggedAt()` (Tasks 1-2).
- Produces: nothing further tasks depend on.

- [x] **Step 1: Write the failing tests**

Append to `apps/auth-service/internal/oidc/resource_test.go`:

```go
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
	if !clearsStateCookie(rec) {
		t.Errorf("expected a Set-Cookie deleting %s", stateCookieName)
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
	if !clearsStateCookie(rec) {
		t.Errorf("expected a Set-Cookie deleting %s", stateCookieName)
	}
	entries := hook.AllEntries()
	if len(entries) != 1 || entries[0].Data["oauth_error"] != "invalid_scope" {
		t.Errorf("expected one entry carrying oauth_error=invalid_scope, got %+v", entries)
	}
}
```

- [x] **Step 2: Run the tests to verify they fail**

Run: `go test github.com/jtumidanski/myfleet/apps/auth-service/internal/oidc -run TestCallback_providerAccessDeniedIsCancelled -v`
Expected: FAIL — `Location = "http://app.test/login#error=invalid_state", want "http://app.test/login#error=cancelled"` (the request has no `code`, so it currently falls into the missing-code exit).

- [x] **Step 3: Add the pre-check**

In `apps/auth-service/internal/oidc/resource.go`, insert at the very top of the `callbackHandler` closure, above the existing `code := req.URL.Query().Get("code")`:

```go
		// Google reports a declined consent screen — and its own
		// misconfigurations — on `error`, with no `code`. Checked first so a
		// cancel is not misreported as a missing-code fault.
		if oauthErr := req.URL.Query().Get("error"); oauthErr != "" {
			// Info, not Error: declining consent is a normal outcome and must
			// not inflate the error rate. Logged all the same, because a spike
			// in access_denied is a UX signal.
			log.WithField("oauth_error", oauthErr).Info("oidc callback returned provider error")
			if oauthErr == "access_denied" {
				failLogin(w, req, d, errCancelled)
				return
			}
			// invalid_scope / invalid_request / server_error are our
			// misconfigurations, not the user's choice (design §3.3).
			failLogin(w, req, d, errAuthFailed)
			return
		}
```

- [x] **Step 4: Run the tests to verify they pass**

Run: `go test -race github.com/jtumidanski/myfleet/apps/auth-service/... -v`
Expected: PASS, including both new tests and every test from Tasks 1-2.

- [x] **Step 5: Commit**

```bash
git add apps/auth-service/internal/oidc/resource.go apps/auth-service/internal/oidc/resource_test.go
git commit -m "feat(auth): distinguish a cancelled consent screen from a failed callback"
```

---

### Task 4: `lib/auth/loginError.ts` — read, normalise, strip

The SPA's half of the fragment contract. The module owns the closed code set, the read-and-strip, and the code → presentation mapping — nothing else.

**Files:**
- Create: `apps/web/src/lib/auth/loginError.ts`
- Test: `apps/web/src/lib/auth/loginError.test.ts`

**Interfaces:**
- Consumes: nothing from earlier tasks (the contract is the four code strings, produced by Task 2).
- Produces: `type LoginErrorCode = 'cancelled' | 'invalid_state' | 'auth_failed' | 'server_error'`; `interface LoginErrorNotice { tone: 'neutral' | 'danger'; message: string }`; `consumeLoginError(): LoginErrorCode | null`; `noticeFor(code: LoginErrorCode): LoginErrorNotice`. Task 6 imports both functions and the `LoginErrorCode` type.

- [x] **Step 1: Write the failing tests**

Create `apps/web/src/lib/auth/loginError.test.ts`:

```ts
import { describe, it, expect, beforeEach, vi } from 'vitest';
import type { LoginErrorCode } from './loginError';

/**
 * The module memoises its read at module scope (design §4.1), so every case
 * needs a fresh module instance. `vi.resetModules()` clears the source-module
 * registry; node_modules stay externalised, so this is cheap.
 */
async function freshModule(url: string) {
  window.history.replaceState(null, '', url);
  vi.resetModules();
  return import('./loginError');
}

describe('consumeLoginError', () => {
  beforeEach(() => {
    window.history.replaceState(null, '', '/login');
  });

  it.each<LoginErrorCode>(['cancelled', 'invalid_state', 'auth_failed', 'server_error'])(
    'parses the %s code',
    async (code) => {
      const { consumeLoginError } = await freshModule(`/login#error=${code}`);
      expect(consumeLoginError()).toBe(code);
    },
  );

  // FR-STATE-6: anything outside the closed set is a generic failure, and the
  // supplied string is discarded at the parser so nothing downstream can render
  // it.
  it.each([
    ['an unknown code', '/login#error=totally_made_up'],
    ['an empty value', '/login#error='],
    ['a malformed percent-encoding', '/login#error=%zz'],
    ['an injected string', '/login#error=%3Cscript%3Ealert(1)%3C%2Fscript%3E'],
  ])('normalises %s to server_error', async (_name, url) => {
    const { consumeLoginError } = await freshModule(url);
    expect(consumeLoginError()).toBe('server_error');
  });

  it('returns null when the hash carries no error key', async () => {
    const { consumeLoginError } = await freshModule('/login');
    expect(consumeLoginError()).toBeNull();
  });

  // FR-STATE-8: the two fragment consumers are mutually exclusive. Reading the
  // error must not eat the token AuthProvider is about to capture.
  it('ignores and preserves an access_token fragment', async () => {
    const { consumeLoginError } = await freshModule('/login#access_token=jwt-value');
    expect(consumeLoginError()).toBeNull();
    expect(window.location.hash).toBe('#access_token=jwt-value');
  });

  // FR-STATE-7: a reload or a shared URL must not resurrect a stale message.
  it('strips the fragment while preserving path and query', async () => {
    const { consumeLoginError } = await freshModule('/login?next=%2Fvehicles#error=auth_failed');
    consumeLoginError();
    expect(window.location.hash).toBe('');
    expect(window.location.pathname).toBe('/login');
    expect(window.location.search).toBe('?next=%2Fvehicles');
  });

  it('memoises so a second call survives the strip', async () => {
    const { consumeLoginError } = await freshModule('/login#error=cancelled');
    expect(consumeLoginError()).toBe('cancelled');
    expect(consumeLoginError()).toBe('cancelled');
  });
});

describe('noticeFor', () => {
  it('presents a cancellation neutrally', async () => {
    const { noticeFor } = await freshModule('/login');
    expect(noticeFor('cancelled')).toEqual({ tone: 'neutral', message: 'Sign-in cancelled.' });
  });

  it.each<LoginErrorCode>(['invalid_state', 'auth_failed', 'server_error'])(
    'presents %s as a danger with the shared copy',
    async (code) => {
      const { noticeFor } = await freshModule('/login');
      expect(noticeFor(code)).toEqual({
        tone: 'danger',
        message:
          "Sign-in didn't complete. Nothing was saved — try again, or use a different Google account.",
      });
    },
  );
});
```

- [x] **Step 2: Run the tests to verify they fail**

Run: `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22 && npm run -w apps/web test -- src/lib/auth/loginError.test.ts`
Expected: FAIL — `Failed to resolve import "./loginError"`.

- [x] **Step 3: Write the module**

Create `apps/web/src/lib/auth/loginError.ts`:

```ts
/** The closed set auth-service redirects with (FR-ERR-4). Nothing else is valid. */
export type LoginErrorCode = 'cancelled' | 'invalid_state' | 'auth_failed' | 'server_error';

export interface LoginErrorNotice {
  tone: 'neutral' | 'danger';
  message: string;
}

const CODES: readonly string[] = ['cancelled', 'invalid_state', 'auth_failed', 'server_error'];

// One sentence for all three failure codes. The invalid_state / auth_failed /
// server_error split exists for log correlation in auth-service, not for the
// reader — a person cannot act differently on it, and "invalid state" is jargon
// (design §4.2, open question 3). The table is keyed on all four codes anyway,
// so diverging one later is a one-line change.
const GENERIC_FAILURE =
  "Sign-in didn't complete. Nothing was saved — try again, or use a different Google account.";

const NOTICES: Record<LoginErrorCode, LoginErrorNotice> = {
  // Cancelling is a choice, not a fault: neutral tone, no alarm (FR-STATE-5).
  cancelled: { tone: 'neutral', message: 'Sign-in cancelled.' },
  invalid_state: { tone: 'danger', message: GENERIC_FAILURE },
  auth_failed: { tone: 'danger', message: GENERIC_FAILURE },
  server_error: { tone: 'danger', message: GENERIC_FAILURE },
};

/** Maps a code to how the login page should present it. */
export function noticeFor(code: LoginErrorCode): LoginErrorNotice {
  return NOTICES[code];
}

function isLoginErrorCode(value: string): value is LoginErrorCode {
  return CODES.includes(value);
}

function readAndStrip(): LoginErrorCode | null {
  const hash = window.location.hash;
  const params = new URLSearchParams(hash.startsWith('#') ? hash.slice(1) : hash);
  // No `error` key at all — including a `#access_token=` fragment, which
  // belongs to captureTokenFromHash and must survive untouched (FR-STATE-8).
  if (!params.has('error')) return null;

  // Strip before returning so a reload or a shared URL cannot resurrect a stale
  // message (FR-STATE-7). Mirrors captureTokenFromHash's replaceState.
  window.history.replaceState(null, '', window.location.pathname + window.location.search);

  const raw = params.get('error') ?? '';
  // FR-STATE-6: the raw value dies here. An unknown, empty, malformed or
  // injected string becomes the generic failure, so nothing downstream can
  // render attacker-supplied text.
  return isLoginErrorCode(raw) ? raw : 'server_error';
}

let captured: LoginErrorCode | null | undefined;

/**
 * Read the `#error=<code>` fragment auth-service redirects with, exactly once
 * per page load, and strip it.
 *
 * Memoised at module scope rather than read inside a hook: React StrictMode
 * mounts, unmounts and remounts in development, and a per-mount read would find
 * the fragment already stripped on the second mount — the callout would vanish
 * in dev but not in prod. Fresh page load = fresh module instance = fresh read,
 * which is exactly the lifetime FR-STATE-7 describes.
 */
export function consumeLoginError(): LoginErrorCode | null {
  if (captured !== undefined) return captured;
  captured = readAndStrip();
  return captured;
}
```

- [x] **Step 4: Run the tests to verify they pass**

Run: `npm run -w apps/web test -- src/lib/auth/loginError.test.ts`
Expected: PASS (14 tests).

- [x] **Step 5: Commit**

```bash
git add apps/web/src/lib/auth/loginError.ts apps/web/src/lib/auth/loginError.test.ts
git commit -m "feat(web): parse and strip the login error fragment"
```

---

### Task 5: Extract `ThemeToggleButton` from `ThemeToggle`

The login page needs the theme control without the authenticated `PATCH` behind it. Splitting presentation from mutation gives it one, with no auth concept anywhere inside the theme components (FR-PRETOGGLE-5). The rendered DOM is unchanged, so `ThemeToggle.test.tsx` must keep passing untouched.

**Files:**
- Create: `apps/web/src/components/ThemeToggleButton.tsx`
- Create: `apps/web/src/components/ThemeToggleButton.test.tsx`
- Modify: `apps/web/src/components/ThemeToggle.tsx` (whole file)
- Do **not** edit: `apps/web/src/components/ThemeToggle.test.tsx`

**Interfaces:**
- Consumes: `ThemePreference` from `../types/models/user`; `useTheme` from `../context/ThemeContext`.
- Produces: `ThemeToggleButton({ preference, onSelect }: { preference: ThemePreference; onSelect: (next: ThemePreference) => void })` — Task 6 renders it with `preference`/`setPreference` straight from `useTheme()`.

- [x] **Step 1: Write the failing test**

Create `apps/web/src/components/ThemeToggleButton.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, act } from '@testing-library/react';
import { ThemeToggleButton } from './ThemeToggleButton';
import type { ThemePreference } from '../types/models/user';

describe('ThemeToggleButton', () => {
  const onSelect = vi.fn<(next: ThemePreference) => void>();

  beforeEach(() => {
    onSelect.mockReset();
  });

  // FR-TOGGLE-2 / FR-TOGGLE-4: the label names the current state AND the next
  // action, so a screen-reader user can operate the cycle without sighted
  // feedback. Now this component's contract rather than ThemeToggle's.
  it.each<[ThemePreference, ThemePreference, string]>([
    ['light', 'dark', 'Theme: light. Switch to dark.'],
    ['dark', 'system', 'Theme: dark. Switch to system.'],
    ['system', 'light', 'Theme: system. Switch to light.'],
  ])('from %s, selects %s and labels itself %s', (preference, next, label) => {
    render(<ThemeToggleButton preference={preference} onSelect={onSelect} />);

    const button = screen.getByRole('button');
    expect(button).toHaveAttribute('aria-label', label);
    expect(button).toHaveAttribute('title', `Theme: ${preference}`);

    act(() => button.click());
    expect(onSelect).toHaveBeenCalledTimes(1);
    expect(onSelect).toHaveBeenCalledWith(next);
  });

  // FR-TOGGLE-3: the icon tracks the PREFERENCE, not the resolved theme, or
  // `system` would be indistinguishable from whichever theme it resolved to.
  // lucide-react stamps a `lucide-<kebab-name>` class onto each <svg>, which is
  // what lets the assertion tell the icons apart.
  it('shows the icon for the given preference', () => {
    render(<ThemeToggleButton preference="system" onSelect={onSelect} />);

    const svg = screen.getByRole('button').querySelector('svg');
    expect(svg).toHaveClass('lucide-monitor');
    expect(svg).not.toHaveClass('lucide-moon');
  });

  // It reads no context and holds no state, so it can be rendered bare — which
  // is the point: it knows nothing about auth (FR-PRETOGGLE-5).
  it('renders without a ThemeProvider', () => {
    expect(() =>
      render(<ThemeToggleButton preference="light" onSelect={onSelect} />),
    ).not.toThrow();
  });
});
```

- [x] **Step 2: Run the test to verify it fails**

Run: `npm run -w apps/web test -- src/components/ThemeToggleButton.test.tsx`
Expected: FAIL — `Failed to resolve import "./ThemeToggleButton"`.

- [x] **Step 3: Create the presentational component**

Create `apps/web/src/components/ThemeToggleButton.tsx` — the cycle map, icon map, aria strings and markup move here verbatim from `ThemeToggle.tsx`:

```tsx
import { Monitor, Moon, Sun, type LucideIcon } from 'lucide-react';
import type { ThemePreference } from '../types/models/user';
import { Button } from './ui/button';

// FR-TOGGLE-2: a fixed cycle, so the control is predictable without a menu.
const NEXT: Record<ThemePreference, ThemePreference> = {
  light: 'dark',
  dark: 'system',
  system: 'light',
};

// FR-TOGGLE-3: keyed on the PREFERENCE, not the resolved theme — otherwise
// `system` would be indistinguishable from whichever theme it resolved to.
const META: Record<ThemePreference, { Icon: LucideIcon; label: string }> = {
  light: { Icon: Sun, label: 'light' },
  dark: { Icon: Moon, label: 'dark' },
  system: { Icon: Monitor, label: 'system' },
};

export interface ThemeToggleButtonProps {
  preference: ThemePreference;
  onSelect: (next: ThemePreference) => void;
}

/**
 * The theme cycle control, with no opinion about what happens next.
 *
 * It holds no state and reads no context, so it knows nothing about auth
 * (FR-PRETOGGLE-5) — which is what lets the signed-out login page use the same
 * control as the authenticated header without a request firing. The header's
 * `PATCH` lives one level up, in ThemeToggle.
 */
export function ThemeToggleButton({ preference, onSelect }: ThemeToggleButtonProps) {
  const next = NEXT[preference];
  const { Icon, label } = META[preference];

  return (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      onClick={() => onSelect(next)}
      title={`Theme: ${label}`}
      aria-label={`Theme: ${label}. Switch to ${META[next].label}.`}
    >
      <Icon className="h-4 w-4" aria-hidden="true" />
    </Button>
  );
}
```

- [x] **Step 4: Reduce `ThemeToggle` to the mutation wrapper**

Replace the whole of `apps/web/src/components/ThemeToggle.tsx`:

```tsx
import { toast } from 'sonner';
import { useTheme } from '../context/ThemeContext';
import { useUpdateTheme } from '../lib/hooks/api/auth';
import { ThemeToggleButton } from './ThemeToggleButton';

/**
 * The header theme control — the only place in the app where a theme change and
 * a network mutation fire together, so there is exactly one file to read to
 * understand when theming touches the network.
 *
 * The visual change is applied optimistically and is never rolled back on a
 * failed save (FR-PERSIST-4, FR-PERSIST-5): reverting the theme under the
 * user's cursor is more jarring than a preference that fails to stick, so the
 * toast explains the outcome instead.
 *
 * The presentation lives in ThemeToggleButton, which the signed-out login page
 * uses directly — there is no session there, so the mutation must not come
 * along (FR-PRETOGGLE-3).
 */
export function ThemeToggle() {
  const { preference, setPreference } = useTheme();
  const updateTheme = useUpdateTheme();

  return (
    <ThemeToggleButton
      preference={preference}
      onSelect={(next) => {
        setPreference(next);
        updateTheme.mutate(next, {
          onError: () => {
            toast.error("Couldn't save your theme preference. It'll reset next time you reload.");
          },
        });
      }}
    />
  );
}
```

- [x] **Step 5: Run both toggle test files to verify they pass**

Run: `npm run -w apps/web test -- src/components/ThemeToggleButton.test.tsx src/components/ThemeToggle.test.tsx`
Expected: PASS. `ThemeToggle.test.tsx` passes **unmodified** — the rendered DOM is identical, and its assertions (`getByRole('button')`, `aria-label`, `title`, the lucide icon class) all still hold.

- [x] **Step 6: Run the full frontend suite**

Run: `npm run -w apps/web test`
Expected: PASS, including `AppLayout.test.tsx`, which renders `ThemeToggle` in place.

- [x] **Step 7: Commit**

```bash
git add apps/web/src/components/ThemeToggleButton.tsx apps/web/src/components/ThemeToggleButton.test.tsx apps/web/src/components/ThemeToggle.tsx
git commit -m "refactor(web): split ThemeToggleButton out of ThemeToggle"
```

---

### Task 6: Rewrite `/login` — Direction D with three states

The page composition, the Google mark, and the three states. `GoogleMark` is created here rather than in its own task: it has one consumer, no state, and no test of its own beyond the page's.

**Files:**
- Create: `apps/web/src/components/GoogleMark.tsx`
- Modify: `apps/web/src/pages/LoginPage.tsx` (whole file)
- Test: `apps/web/src/pages/LoginPage.test.tsx` (create)

**Interfaces:**
- Consumes: `consumeLoginError`, `noticeFor` (Task 4); `ThemeToggleButton` (Task 5); `useAuth` (`login`, `isAuthenticated`), `useTheme` (`preference`, `setPreference`), `BrandMark`, `Button`.
- Produces: `GoogleMark({ className }: { className?: string })`; the rewritten `LoginPage()`. Nothing downstream consumes them.

- [x] **Step 1: Write the failing test**

Create `apps/web/src/pages/LoginPage.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, act } from '@testing-library/react';
import { MemoryRouter, Routes, Route } from 'react-router-dom';
import { ThemeProvider } from '../context/ThemeContext';
import type { AuthContextValue } from '../context/AuthContext';
import { THEME_STORAGE_KEY } from '../lib/theme';
import { resetMatchMedia } from '../test/setup';

// Mock the auth context so the page can be exercised without the provider/query
// stack — the pattern AppLayout.test.tsx already uses.
const mockAuth = vi.fn<() => AuthContextValue>();
vi.mock('../context/AuthContext', () => ({
  useAuth: () => mockAuth(),
}));

const login = vi.fn();

function baseAuth(overrides: Partial<AuthContextValue> = {}): AuthContextValue {
  return {
    user: null,
    activeFleetId: null,
    role: null,
    isAuthenticated: false,
    isLoading: false,
    login,
    logout: vi.fn(),
    ...overrides,
  };
}

/**
 * consumeLoginError memoises at module scope (design §4.1), so each case needs
 * a fresh module registry — hence resetModules plus a dynamic import of the
 * page rather than a top-level one.
 */
async function renderLogin(hash = '', auth: Partial<AuthContextValue> = {}) {
  window.history.replaceState(null, '', `/login${hash}`);
  mockAuth.mockReturnValue(baseAuth(auth));
  vi.resetModules();
  const { LoginPage } = await import('./LoginPage');
  return render(
    <MemoryRouter initialEntries={['/login']}>
      <ThemeProvider>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route path="/" element={<div>dashboard</div>} />
        </Routes>
      </ThemeProvider>
    </MemoryRouter>,
  );
}

const GENERIC_FAILURE =
  "Sign-in didn't complete. Nothing was saved — try again, or use a different Google account.";

function signInButton() {
  return screen.getByRole('button', { name: /Google|Try again/ });
}

describe('LoginPage', () => {
  beforeEach(() => {
    localStorage.clear();
    resetMatchMedia();
    document.documentElement.classList.remove('dark');
    login.mockReset();
  });

  // FR-PAGE-1 / FR-PAGE-2 / FR-PAGE-4 / FR-PAGE-6: a typographic composition,
  // not a card, that says what MyFleet is before asking for an account.
  it('renders the headline, the scopes line and no card', async () => {
    const { container } = await renderLogin();

    expect(screen.getByText('Your cars.')).toBeInTheDocument();
    expect(screen.getByText('One place.')).toBeInTheDocument();
    expect(screen.getByText(/name, email address, and profile photo/i)).toBeInTheDocument();
    expect(container.querySelector('.bg-card')).toBeNull();
  });

  // FR-STATE-1: the default state.
  it('offers an enabled Continue with Google button by default', async () => {
    await renderLogin();

    expect(signInButton()).toHaveTextContent('Continue with Google');
    expect(signInButton()).toBeEnabled();
    expect(screen.queryByRole('alert')).toBeNull();
  });

  // FR-STATE-2 / FR-STATE-3 / FR-A11Y-2: the press is acknowledged, a second
  // activation is impossible, and the status is announced non-visually.
  it('disables itself and announces the handoff when pressed', async () => {
    await renderLogin();

    act(() => signInButton().click());

    expect(signInButton()).toBeDisabled();
    expect(screen.getByRole('status')).toHaveTextContent('Redirecting to Google');
    expect(login).toHaveBeenCalledTimes(1);

    // A double-click cannot start two OAuth flows.
    act(() => signInButton().click());
    expect(login).toHaveBeenCalledTimes(1);
  });

  // FR-STATE-4 / FR-A11Y-1.
  it('shows an announced danger callout and relabels the button on failure', async () => {
    await renderLogin('#error=auth_failed');

    expect(screen.getByRole('alert')).toHaveTextContent(GENERIC_FAILURE);
    expect(signInButton()).toHaveTextContent('Try again');
  });

  // FR-STATE-5: cancelling is a choice, not a fault — no red, no alert, no
  // relabelled button.
  it('shows a neutral line and keeps the label when the user cancelled', async () => {
    await renderLogin('#error=cancelled');

    expect(screen.queryByRole('alert')).toBeNull();
    expect(screen.getByText('Sign-in cancelled.')).toBeInTheDocument();
    expect(signInButton()).toHaveTextContent('Continue with Google');
  });

  // FR-STATE-6: an attacker-supplied fragment never reaches the DOM.
  it('renders generic copy for a garbage code without echoing it', async () => {
    await renderLogin('#error=%3Cscript%3Ealert(1)%3C%2Fscript%3E');

    expect(screen.getByRole('alert')).toHaveTextContent(GENERIC_FAILURE);
    expect(document.body.textContent).not.toContain('alert(1)');
  });

  // FR-STATE-7.
  it('strips the error fragment from the URL', async () => {
    await renderLogin('#error=auth_failed');

    expect(window.location.hash).toBe('');
  });

  // FR-PRETOGGLE-1 / FR-PRETOGGLE-2 / FR-PRETOGGLE-3: a working theme control
  // with no session behind it. Spying on `fetch` rather than on a mocked
  // useUpdateTheme is the stronger claim — it fails if ANY request appears.
  it('cycles the theme without issuing a request', async () => {
    const fetchSpy = vi
      .spyOn(globalThis, 'fetch')
      .mockImplementation(() => Promise.reject(new Error('no network in tests')));
    localStorage.setItem(THEME_STORAGE_KEY, 'light');
    await renderLogin();

    const toggle = () => screen.getByRole('button', { name: /^Theme:/ });
    expect(toggle()).toHaveAttribute('aria-label', 'Theme: light. Switch to dark.');

    act(() => toggle().click());
    expect(toggle()).toHaveAttribute('aria-label', 'Theme: dark. Switch to system.');

    act(() => toggle().click());
    expect(toggle()).toHaveAttribute('aria-label', 'Theme: system. Switch to light.');

    act(() => toggle().click());
    expect(toggle()).toHaveAttribute('aria-label', 'Theme: light. Switch to dark.');

    expect(fetchSpy).not.toHaveBeenCalled();
    fetchSpy.mockRestore();
  });

  // FR-STATE-9: the pre-existing bounce is preserved.
  it('sends an already-authenticated visitor into the app', async () => {
    await renderLogin('', { isAuthenticated: true });

    expect(screen.getByText('dashboard')).toBeInTheDocument();
  });
});
```

- [x] **Step 2: Run the test to verify it fails**

Run: `npm run -w apps/web test -- src/pages/LoginPage.test.tsx`
Expected: FAIL — `Unable to find an element with the text: Your cars.` (the current page still renders the card).

- [x] **Step 3: Create the Google mark**

Create `apps/web/src/components/GoogleMark.tsx`:

```tsx
/**
 * Google's four-colour "G", inline.
 *
 * Two project conventions point the other way here, and both are deliberately
 * departed from:
 *
 * 1. Everything else in the app inherits `currentColor` (see BrandMark).
 *    Recolouring this mark is precisely what Google's branding terms forbid, so
 *    the fills are fixed hex values.
 * 2. The mark carries its own white backing disc. FR-PAGE-8 puts it on a solid
 *    `bg-primary` button, and `--primary` is near-black in light mode and
 *    near-white in dark; the terms require a light surface under the mark. A
 *    Tailwind `bg-white` tile is unavailable — src/test/conventions.test.ts
 *    fails any .tsx containing a hardcoded palette class — and rightly so: that
 *    rule exists to stop palette colours bypassing the token system, and a fixed
 *    third-party mark is not part of the palette. The disc lives inside the SVG.
 *
 * aria-hidden: the button's own text supplies the accessible name (FR-A11Y-4).
 */
export function GoogleMark({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 48 48" className={className} aria-hidden="true" focusable="false">
      <circle cx="24" cy="24" r="24" fill="#FFFFFF" />
      <path
        fill="#EA4335"
        d="M24 9.5c3.54 0 6.71 1.22 9.21 3.6l6.85-6.85C35.9 2.38 30.47 0 24 0 14.62 0 6.51 5.38 2.56 13.22l7.98 6.19C12.43 13.72 17.74 9.5 24 9.5z"
      />
      <path
        fill="#4285F4"
        d="M46.98 24.55c0-1.57-.15-3.09-.38-4.55H24v9.02h12.94c-.58 2.96-2.26 5.48-4.78 7.18l7.73 6c4.51-4.18 7.09-10.36 7.09-17.65z"
      />
      <path
        fill="#FBBC05"
        d="M10.53 28.59c-.48-1.45-.76-2.99-.76-4.59s.27-3.14.76-4.59l-7.98-6.19C.92 16.46 0 20.12 0 24c0 3.88.92 7.54 2.56 10.78l7.97-6.19z"
      />
      <path
        fill="#34A853"
        d="M24 48c6.48 0 11.93-2.13 15.89-5.81l-7.73-6c-2.15 1.45-4.92 2.3-8.16 2.3-6.26 0-11.57-4.22-13.47-9.91l-7.98 6.19C6.51 42.62 14.62 48 24 48z"
      />
    </svg>
  );
}
```

- [x] **Step 4: Rewrite the page**

Replace the whole of `apps/web/src/pages/LoginPage.tsx`:

```tsx
import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Loader2 } from 'lucide-react';
import { useAuth } from '../context/AuthContext';
import { useTheme } from '../context/ThemeContext';
import { BrandMark } from '../components/BrandMark';
import { GoogleMark } from '../components/GoogleMark';
import { ThemeToggleButton } from '../components/ThemeToggleButton';
import { Button } from '../components/ui/button';
import { consumeLoginError, noticeFor } from '../lib/auth/loginError';

/**
 * The only screen an unauthenticated visitor sees. Three states: ready,
 * redirecting, and failed.
 *
 * The button performs a full navigation to auth-service's
 * `GET /api/auth/login/google`, which runs the OAuth dance and redirects back
 * either with `#access_token=<jwt>` (AuthProvider captures it) or, on any
 * failure, back here with `#error=<code>` (consumeLoginError reads it).
 * Already-authenticated visitors are bounced to the app.
 */
export function LoginPage() {
  const { login, isAuthenticated } = useAuth();
  // No mutation behind the control: there is no session on /login, so the
  // authenticated PATCH would 401 and toast on every click (FR-PRETOGGLE-3).
  const { preference, setPreference } = useTheme();
  const navigate = useNavigate();
  // Read once per page load; the module memoises, so StrictMode's remount does
  // not swallow the notice (design §4.1).
  const [errorCode] = useState(consumeLoginError);
  const [redirecting, setRedirecting] = useState(false);

  useEffect(() => {
    if (isAuthenticated) navigate('/', { replace: true });
  }, [isAuthenticated, navigate]);

  const notice = errorCode ? noticeFor(errorCode) : null;
  const failed = notice?.tone === 'danger';

  return (
    <div className="relative flex min-h-screen flex-col justify-center bg-background px-6 sm:px-12 lg:px-24">
      <div className="absolute right-4 top-4">
        <ThemeToggleButton preference={preference} onSelect={setPreference} />
      </div>

      <div className="w-full max-w-xl space-y-8">
        <div className="flex items-center gap-2 text-xs font-medium uppercase tracking-[0.2em] text-muted-foreground">
          <BrandMark className="h-4 w-4" />
          MyFleet
        </div>

        {/* Capped near 11ch so it wraps to two lines predictably rather than
            overflowing a 320px viewport (FR-PAGE-9). */}
        <h1 className="max-w-[11ch] text-4xl font-semibold leading-[1.05] tracking-tight sm:text-5xl lg:text-6xl">
          <span className="block text-foreground">Your cars.</span>
          <span className="block text-muted-foreground">One place.</span>
        </h1>

        <div className="h-px bg-border" />

        {/* role="alert" only on the danger branch: a cancellation is not an
            alert, and announcing it as one is the same category error as
            painting it red (FR-STATE-4, FR-STATE-5, FR-A11Y-1). */}
        {notice &&
          (failed ? (
            <div
              role="alert"
              className="rounded-md border border-danger-border bg-danger-subtle p-3 text-sm text-danger-subtle-foreground"
            >
              {notice.message}
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">{notice.message}</p>
          ))}

        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:gap-6">
          <Button
            type="button"
            className="w-full sm:w-auto"
            disabled={redirecting}
            onClick={() => {
              // One-way: login() is a full navigation, so this state has no
              // exit path in-page (FR-STATE-2) and `disabled` makes a second
              // activation impossible (FR-STATE-3).
              setRedirecting(true);
              login();
            }}
          >
            {redirecting ? (
              <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />
            ) : (
              <GoogleMark className="h-4 w-4" />
            )}
            {redirecting ? 'Redirecting to Google…' : failed ? 'Try again' : 'Continue with Google'}
          </Button>
          <p className="text-sm text-muted-foreground">
            MyFleet receives your name, email address, and profile photo.
          </p>
        </div>

        {/* A live region, because relabelling an element that has just become
            disabled is not reliably announced (FR-A11Y-2). */}
        {redirecting && (
          <span className="sr-only" role="status">
            Redirecting to Google…
          </span>
        )}

        <p className="text-sm text-muted-foreground">
          Maintenance, mileage, and receipts for every car in your household. Sign in with Google.
        </p>
      </div>
    </div>
  );
}
```

- [x] **Step 5: Run the test to verify it passes**

Run: `npm run -w apps/web test -- src/pages/LoginPage.test.tsx`
Expected: PASS (10 tests).

If the `#error=...` cases fail with the notice absent while the module's own tests pass, the cause is the memoisation surviving between cases — check that `vi.resetModules()` runs **before** the dynamic `import('./LoginPage')` in `renderLogin`, not after.

- [x] **Step 6: Run the whole frontend suite**

Run: `npm run -w apps/web test`
Expected: PASS — in particular `src/test/conventions.test.ts` ("no hardcoded palette classes"), which now walks `GoogleMark.tsx` and `LoginPage.tsx`.

- [x] **Step 7: Commit**

```bash
git add apps/web/src/components/GoogleMark.tsx apps/web/src/pages/LoginPage.tsx apps/web/src/pages/LoginPage.test.tsx
git commit -m "feat(web): redesign the login page with ready, redirecting and failed states"
```

---

### Task 7: Full verification

Everything the PRD's acceptance criteria demand that is not already asserted by a unit test.

**Files:** none modified. This task produces evidence, not code.

**Interfaces:**
- Consumes: everything from Tasks 1-6.
- Produces: nothing.

- [x] **Step 1: Run the full CI target**

```sh
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
make ci
```

Expected: `lint-check`, `vet`, `test`, `build`, `fe-test`, `fe-build`, `manifests` and `carfax-template` all pass. If `lint-check` reports formatting, run `make lint` and re-run `make ci`.

- [x] **Step 2: Confirm no plaintext error exit survives**

```sh
grep -c "http.Error" apps/auth-service/internal/oidc/resource.go
```

Expected: `0` (PRD acceptance criterion).

- [x] **Step 3: Render and dry-run both kustomize overlays**

This task changes no manifest, but CLAUDE.md requires both overlays re-verified before a PR, and the local overlay is not exempt.

```sh
kustomize build deploy/k8s/overlays/local
kustomize build deploy/k8s/overlays/main
kustomize build deploy/k8s/overlays/main  | kubectl apply --dry-run=server -f -
kustomize build deploy/k8s/overlays/local | kubectl apply --dry-run=server -f -
```

Expected: both render; both dry-runs report `configured`/`created (server dry run)` with no errors. The `main` overlay must show no PersistentVolumeClaims, no Secrets, no ClusterRole and no placeholder values. If no cluster is reachable, record that the two dry-runs were skipped and why — do not silently drop them.

- [ ] **Step 4: Manual visual check**

Run `npm run -w apps/web dev` and visit each of these, in **both** light and dark, at a 320px-wide viewport and at desktop width (FR-PAGE-9, FR-PAGE-10, FR-A11Y-5):

| URL | Expected |
| --- | --- |
| `/login` | Headline wraps to two lines and does not overflow; button and scopes line stack under ~640px; the Google mark is legible on the solid button in both themes |
| `/login#error=cancelled` | Muted "Sign-in cancelled." line; button still reads "Continue with Google"; URL hash gone after load |
| `/login#error=auth_failed` | Danger callout; button reads "Try again" |
| `/login#error=nonsense` | Same generic callout; the string `nonsense` appears nowhere on the page |

Also press the theme control in the top-right and confirm it cycles light → dark → system → light with no toast and no request in the network panel.

- [x] **Step 5: Rebase onto `main` if the `return_to` work has landed**

PRD §7.4: a separate branch implements post-login return paths and touches `LoginPage.tsx`, `LoginPage.test.tsx`, `resource.go` and `resource_test.go`. If it has merged to `main`:

```sh
git fetch origin && git rebase origin/main
```

Reconcile rather than overwrite:
- `LoginPage.tsx` — the `onClick` wrapper in Task 6 is the designated seam; `login()` becomes `login(from)` inside it, and nothing else in the composition moves.
- `LoginPage.test.tsx` / `resource_test.go` — created by both branches; merge the case lists.
- `Dependencies` — the two branches add different fields; take both.

Then re-run Step 1.

**Done as a MERGE, not a rebase** (user's call — it keeps the six reviewed
commits intact): commit `c445237`. The reconciliation was wider than predicted:
`session.MembershipResolver` had also been renamed to
`session.PrincipalResolver` and now returns a whole `Principal`, `setStateCookie`
gained a `returnPath` parameter, `verifyStateCookie` returns three values, and a
new arch test forbids `oidc/resource.go` from constructing a `Principal` at all.
`navigate('/')` needed the same `from` treatment as `login()`. See the merge
commit message for the file-by-file resolution.

- [ ] **Step 6: Code review**

Per CLAUDE.md, before opening the PR:

Invoke `superpowers:requesting-code-review`. Go and TypeScript both changed, so it dispatches `plan-adherence-reviewer`, `backend-guidelines-reviewer` and `frontend-guidelines-reviewer`. Findings land in `docs/tasks/task-010-login-redesign-error-recovery/audit.md`.

- [ ] **Step 7: Commit any review fixes**

```bash
git add -A
git commit -m "fix(task-010): address code review findings"
```

---

## Acceptance Criteria Coverage

| PRD criterion | Where satisfied |
| --- | --- |
| No `Card`, left-aligned, `bg-background` | Task 6, Step 4 + test "renders the headline, the scopes line and no card" |
| Headline `Your cars.` / `One place.`, second line muted | Task 6, Step 4 + same test |
| Solid button, Google mark, three scopes named | Task 6, Steps 3-4 + same test |
| Press disables and shows a redirecting state | Task 6 test "disables itself and announces the handoff when pressed" |
| `#error=auth_failed` → `role="alert"` + "Try again" | Task 6 test "shows an announced danger callout…" |
| `#error=cancelled` → neutral line, label unchanged | Task 6 test "shows a neutral line and keeps the label…" |
| `#error=<garbage>` → generic copy, string not echoed | Task 4 normalisation tests + Task 6 test "renders generic copy for a garbage code…" |
| Fragment stripped after reading | Task 4 test "strips the fragment while preserving path and query" + Task 6 test "strips the error fragment from the URL" |
| Theme control on `/login`, cycles three, no `PATCH` | Task 5 + Task 6 test "cycles the theme without issuing a request" (fetch spy) |
| `ThemeToggle` unchanged behaviour, its tests unmodified | Task 5, Steps 4-6 |
| All nine exits 302 with the right fragment; `grep -c http.Error` → 0 | Task 2, Steps 4-5 + `TestCallback_failureRedirectsToLogin` |
| `?error=access_denied` → `cancelled` | Task 3 `TestCallback_providerAccessDeniedIsCancelled` |
| Failure log lines still fire | Task 2 `loggedAt(hook, logrus.ErrorLevel, …)` per table row |
| Early exits clear the state cookie | Task 2 `clearsStateCookie` per table row + Task 3 both tests |
| `APP_LOGIN_PATH` defaults and overrides | Task 2, Step 7 + `TestCallback_failureHonoursLoginPath` |
| Legible at 320px, light and dark | Task 7, Step 4 |
| `make ci` passes | Task 7, Step 1 |
| Both overlays render and dry-run | Task 7, Step 3 |
| Code review before PR, findings in `audit.md` | Task 7, Step 6 |
