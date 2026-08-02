# Login Page Redesign & Recoverable OAuth Failures — Design

Task: `task-010-login-redesign-error-recovery`
PRD: [`prd.md`](./prd.md) (approved)
Date: 2026-08-02
Status: Proposed

---

## 1. Scope of this document

The PRD settles *what* is built. This document settles *how*: the module
boundaries, the shape of the contract between auth-service and the SPA, the
Go interface change that makes nine failure exits testable, and the four open
questions the PRD deferred to design.

It does not restate requirements. Every decision below cites the FR it serves.

---

## 2. Architecture at a glance

One contract crosses the service boundary — a URL fragment — and it already
has a sibling:

```
                    ┌──────────────────────────────────────────┐
  Google  ─────────▶│  auth-service  GET /api/auth/callback    │
   (302 back)       │  callbackHandler                         │
                    │    success ──▶ 302 {base}{home}#access_token=<jwt>
                    │    failure ──▶ 302 {base}{login}#error=<code>
                    └──────────────────────────────────────────┘
                                        │ 302
                                        ▼
                    ┌──────────────────────────────────────────┐
                    │  SPA                                     │
                    │   AuthProvider ─▶ lib/api/token.ts       │
                    │                    captureTokenFromHash()│  reads #access_token
                    │   LoginPage    ─▶ lib/auth/loginError.ts │
                    │                    consumeLoginError()   │  reads #error
                    └──────────────────────────────────────────┘
```

The two fragment consumers never collide: `captureTokenFromHash` returns early
unless the hash contains `access_token=`, and `consumeLoginError` returns null
unless it contains `error=`. A callback response carries exactly one of the two
(FR-STATE-8). Neither reads the other's key, so the mutual exclusion is
structural rather than ordering-dependent.

New/changed units, each with one job:

| Unit | Job | Depends on |
| --- | --- | --- |
| `oidc.failLogin` (Go, unexported) | Clear state cookie, 302 to `{base}{login}#error=<code>` | `Dependencies` |
| `oidc.Authenticator` / `UserProvisioner` / `TokenIssuer` (Go) | Narrow consumer-side interfaces that make the handler fakeable | — |
| `lib/auth/loginError.ts` | Read + normalise + strip the `#error` fragment; map code → presentation | `window.location`, `history` |
| `components/GoogleMark.tsx` | The Google "G", self-contained brand asset | — |
| `components/ThemeToggleButton.tsx` | Presentational theme cycle control (icon, cycle, aria contract) | `ThemePreference` type only |
| `components/ThemeToggle.tsx` | The above + the authenticated `PATCH` and its toast | `useTheme`, `useUpdateTheme` |
| `pages/LoginPage.tsx` | Composition + the three page states | all of the above |

---

## 3. Backend design

### 3.1 The failure exit

Nine `http.Error` calls collapse to one helper. Codes are typed constants, not
string literals at nine call sites, so the closed set in FR-ERR-4 is enforced by
the compiler rather than by review.

```go
// loginErrorCode is the coarse, browser-visible outcome of a failed callback.
// Deliberately not derived from the underlying error: nothing about the
// failure's internals reaches the SPA (FR-ERR-4, §8 Security).
type loginErrorCode string

const (
	errCancelled    loginErrorCode = "cancelled"
	errInvalidState loginErrorCode = "invalid_state"
	errAuthFailed   loginErrorCode = "auth_failed"
	errServerError  loginErrorCode = "server_error"
)

// failLogin returns the browser to the SPA's login page carrying a coarse
// reason. It clears the state cookie on every path (FR-ERR-8): each of these
// exits abandons the attempt, and the page the user lands on offers "Try
// again", so a stale signed state must not survive to collide with the next
// attempt's.
func failLogin(w http.ResponseWriter, req *http.Request, d Dependencies, code loginErrorCode) {
	clearStateCookie(w, d.CookieSecure)
	http.Redirect(w, req, d.AppBaseURL+d.LoginPath+"#error="+string(code), http.StatusFound)
}
```

Notes that matter at implementation time:

- `clearStateCookie` must precede `http.Redirect`, which calls `WriteHeader`.
  Headers set afterwards are dropped silently.
- On the late exits (post-`verifyStateCookie`) the cookie is cleared twice — once
  by the existing call at `resource.go:78`, once by `failLogin`. Two identical
  `MaxAge:-1` `Set-Cookie` headers are harmless and the alternative (tracking
  whether the clear already happened) buys nothing. The existing line stays where
  it is because it is the *success* path's clear.
- `http.Redirect` writes the location verbatim apart from non-ASCII escaping; the
  `#` and the code survive intact. No `url.QueryEscape` is applied — and must not
  be, or `#` would become `%23`. The code comes from a closed constant set, so
  there is nothing to escape (FR-ERR-9).

`Dependencies` gains `LoginPath string`, wired in `cmd/main.go` beside its two
siblings as `config.Get("APP_LOGIN_PATH", "/login")` (FR-ERR-2).

### 3.2 Making nine exits testable — Decision B

Six of the nine exits sit behind `*oidc.Processor`, `*user.Processor` and
`*session.Processor`, all concrete types on `Dependencies`. Today they are
unreachable from a test without a live Google endpoint and a database, which is
why `resource.go` has no test file at all.

Three options were considered:

| | Approach | Cost | Coverage reached |
| --- | --- | --- | --- |
| A | Test only the three network-free exits; assert the rest by inspection | ~0 | 3 of 9 |
| B | **Make the three `Dependencies` fields interface-typed** | one struct edit, no call-site change | 9 of 9 |
| C | Extract the orchestration into a pure `decide()` returning a destination or a code | large rewrite; conflicts hard with the in-flight `return_to` work | 9 of 9 |

**B is chosen.** The interfaces are declared in the consuming package, which is
the Go idiom, and `*oidc.Processor`, `*user.Processor` and `*session.Processor`
satisfy them implicitly — `cmd/main.go`'s composite literal compiles unchanged
(verified against `apps/auth-service/cmd/main.go:72-82`, which assigns those
three concrete values by field name).

```go
// Authenticator is the OIDC surface the callback needs. Declared here, at the
// consumer, so the handler can be exercised without a live Google endpoint.
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
```

`Dependencies.OIDC`, `.Users` and `.Sessions` change type from `*Processor`,
`*user.Processor`, `*session.Processor` to `Authenticator`, `UserProvisioner`,
`TokenIssuer`. `Resolve` is already an injected func type
(`session.MembershipResolver`), so the fourth dependency is fakeable already —
this change simply brings the other three up to the same standard.

Signatures above are transcribed from source, not memory:
`processor.go:42-69` (oidc), `user/processor.go:33` (`ProvisionFromGoogle` returns
a value `Model`, not a pointer), `session/processor.go:59,77`.

### 3.3 The Google-`error` pre-check, and a refinement to FR-ERR-6

A new check runs first, before `code`/`state` are examined:

```go
if oauthErr := req.URL.Query().Get("error"); oauthErr != "" {
	// Not a fault: declining consent is a normal outcome. Logged at Info so
	// it does not pollute the error rate, but logged, because a spike in
	// access_denied is a UX signal.
	log.WithField("oauth_error", oauthErr).Info("oidc callback returned provider error")
	if oauthErr == "access_denied" {
		failLogin(w, req, d, errCancelled)
		return
	}
	failLogin(w, req, d, errAuthFailed)
	return
}
```

**This refines FR-ERR-6**, which maps *any* `error` parameter to `cancelled`.
Google also returns `invalid_scope`, `invalid_request` and `server_error` on
that parameter; telling a user "Sign-in cancelled" when the app's OAuth client
is misconfigured is a lie that costs a support round-trip. Splitting
`access_denied` from everything else costs two lines and one test case. Recorded
as a deviation in §9.

### 3.4 Exit map

Nine exits, one new pre-check, unchanged logging (FR-ERR-7 — every existing
`log.WithError(...).Error(...)` line stays exactly as written; `failLogin`
replaces only the `http.Error` call that followed it).

| Line (current) | Condition | Code |
| --- | --- | --- |
| *new, first* | `?error=access_denied` | `cancelled` |
| *new, first* | `?error=<anything else>` | `auth_failed` |
| `resource.go:70` | missing `code` or `state` | `invalid_state` |
| `resource.go:75` | `verifyStateCookie` failed | `invalid_state` |
| `resource.go:84` | code exchange failed | `auth_failed` |
| `resource.go:90` | id_token verification failed | `auth_failed` |
| `resource.go:97` | nonce mismatch | `auth_failed` |
| `resource.go:104` | `ProvisionFromGoogle` failed | `server_error` |
| `resource.go:111` | membership `Resolve` failed | `server_error` |
| `resource.go:123` | `MintAccess` failed | `server_error` |
| `resource.go:129` | `IssueRefresh` failed | `server_error` |

The success path is untouched.

### 3.5 Backend test strategy

`internal/oidc/resource_test.go`, package `oidc` (internal, matching
`processor_test.go`) so the state-cookie helpers are reachable.

Fixture shape:

- A valid state cookie is minted by calling `setStateCookie` against a throwaway
  `httptest.ResponseRecorder` and replaying the resulting cookie onto the
  request. No hand-rolled HMAC in the test.
- Fakes are three tiny structs with function fields, satisfying the §3.2
  interfaces. A `user.Model` for the success path comes from
  `user.NewBuilder().SetGoogleSub(...).SetEmail(...).Build()` (`user/builder.go:12`).
- Logging is asserted with `logrus/hooks/test.NewNullLogger()`, which ships with
  the existing logrus dependency — no new module. This is what turns FR-ERR-7
  from a promise into a test.

One table-driven test covers all eleven failure rows: each case asserts
status 302, the exact `Location` value, a `Set-Cookie` deleting `oidc_state`
(FR-ERR-8), and the expected log message where one exists. A separate test
pins the success path so the redirect refactor cannot silently break it, and one
more asserts `APP_LOGIN_PATH`'s effect by setting `LoginPath: "/signin"`.

The `grep -c "http.Error" internal/oidc/resource.go` → `0` criterion is a manual
check at review time. It is not worth a Go test that reads its own source.

---

## 4. Frontend design

### 4.1 `lib/auth/loginError.ts` — reading the fragment

The module owns three things and nothing else: the closed code set, the
read-and-strip, and the code → presentation mapping.

```ts
export type LoginErrorCode = 'cancelled' | 'invalid_state' | 'auth_failed' | 'server_error';

export interface LoginErrorNotice {
  tone: 'neutral' | 'danger';
  message: string;
}

export function consumeLoginError(): LoginErrorCode | null;
export function noticeFor(code: LoginErrorCode): LoginErrorNotice;
```

**Decision: `consumeLoginError` memoises its result at module scope.**

The obvious implementation — parse and strip inside `useState`'s initializer —
is wrong under React 18 StrictMode, which mounts, unmounts and remounts in
development. The second mount's initializer runs *after* the first mount's strip,
finds a bare URL, and the callout vanishes in dev but not in prod. Splitting it
(pure parse in the initializer, strip in an effect) fixes the double-*invoke* but
not the re*mount*.

Memoising at module scope makes the read genuinely once-per-page-load, which is
the semantics the requirement actually wants:

```ts
let captured: LoginErrorCode | null | undefined;

/**
 * Read the `#error=<code>` fragment auth-service redirects with, exactly once
 * per page load, and strip it so a reload or a shared URL cannot resurrect a
 * stale message (FR-STATE-7).
 *
 * Memoised at module scope rather than read inside a hook: React StrictMode
 * mounts, unmounts and remounts in development, and a per-mount read would
 * find the fragment already stripped on the second mount. Fresh page load =
 * fresh module instance = fresh read, which is exactly the lifetime FR-STATE-7
 * describes.
 */
export function consumeLoginError(): LoginErrorCode | null {
  if (captured !== undefined) return captured;
  captured = readAndStrip();
  return captured;
}
```

Alternatives rejected:

- **sessionStorage.** Survives reloads, which violates FR-STATE-7 outright.
- **Reading it in `AuthProvider`** beside `captureTokenFromHash`. Would make the
  auth context carry login-page presentation state — the same boundary violation
  task-003 avoided by routing theme through `ThemeSync` rather than `AuthContext`.

Consequence worth naming: within one page load, navigating away from `/login`
and back re-shows the notice. For an unauthenticated visitor the only way back
to `/login` *is* a `RequireAuth` bounce, i.e. a remount of the same failed
attempt, where re-showing is correct. A successful sign-in is a full navigation,
which resets the module.

Normalisation (FR-STATE-6): `readAndStrip` parses the hash with
`URLSearchParams`, and any value outside the four-member set — absent, empty,
malformed percent-encoding, or an injected string — becomes `server_error`. The
raw value is discarded at the parser, so nothing downstream can render it. A
missing `error` key entirely returns `null` (the ready state), which is the one
case that must not be coerced.

### 4.2 Copy, and open questions 1 & 3

`noticeFor` is a lookup table keyed on all four codes even though three share
copy today — the table is the extension point, so diverging one later touches
one line rather than reshaping the mapping.

| Code | Tone | Copy |
| --- | --- | --- |
| `cancelled` | `neutral` | "Sign-in cancelled." |
| `invalid_state` | `danger` | "Sign-in didn't complete. Nothing was saved — try again, or use a different Google account." |
| `auth_failed` | `danger` | *same as above* |
| `server_error` | `danger` | *same as above* |

**Open question 1 — resolved** with the copy above. It states the outcome, states
that retrying is safe, and offers the one alternative action that ever helps
(wrong Google account).

**Open question 3 — resolved: no.** `invalid_state` gets no distinct copy. The
split exists for log correlation; a reader cannot act differently on it, and a
message that says "invalid state" would be jargon leaking into the UI. The
distinction survives where it is useful — in auth-service's logs.

### 4.3 `ThemeToggleButton` — the extraction

```ts
interface ThemeToggleButtonProps {
  preference: ThemePreference;
  onSelect: (next: ThemePreference) => void;
}
```

The `NEXT` cycle map, the `META` icon/label map, the `aria-label`/`title` strings
and the `<Button variant="ghost" size="icon">` markup all move into
`ThemeToggleButton.tsx` verbatim. It computes `next` and hands it to `onSelect`;
it holds no state and reads no context, so it knows nothing about auth
(FR-PRETOGGLE-5).

`ThemeToggle.tsx` reduces to the mutation wrapper:

```tsx
export function ThemeToggle() {
  const { preference, setPreference } = useTheme();
  const updateTheme = useUpdateTheme();
  return (
    <ThemeToggleButton
      preference={preference}
      onSelect={(next) => {
        setPreference(next);
        updateTheme.mutate(next, { onError: () => toast.error(/* unchanged */) });
      }}
    />
  );
}
```

`LoginPage` wires the same component straight to context:
`<ThemeToggleButton preference={preference} onSelect={setPreference} />`. No
network path exists to fire, so FR-PRETOGGLE-3 holds by construction rather than
by a conditional inside `ThemeToggle` — which was the alternative, and is worse:
an `authenticated?: boolean` prop would put an auth concept inside the theme
control, exactly what FR-PRETOGGLE-5 forbids.

The rendered DOM is byte-identical to today's, so `ThemeToggle.test.tsx` passes
unmodified — it queries `screen.getByRole('button')` and asserts `aria-label`,
`title`, and the lucide icon class, none of which move.

**Open question 4 — resolved: keep it identical.** No quieter variant. FR-A11Y-3
requires the same aria contract, the ghost icon button is already the quietest
control in the system, and a second visual treatment for one call site is a
divergence to maintain for no gain. Differentiation comes from placement alone:
absolutely positioned at the viewport's top-right, against a `relative` page root.

### 4.4 `GoogleMark` — a brand asset with its own background

FR-PAGE-8 puts the Google "G" on a solid `bg-primary` button. `--primary` is
near-black in light mode and near-white in dark, and Google's branding terms
require the four-colour mark to sit on a light surface. The mark therefore
carries its own: a white disc drawn *inside* the SVG, beneath the G.

This is deliberate and needs the comment, because two project rules point the
other way:

- `src/test/conventions.test.ts` fails any `.tsx` containing `bg-white` and
  friends. A Tailwind-applied white tile is not an option; an in-SVG `<circle
  fill="#FFFFFF">` is outside the scanner's regex and outside its intent — the
  rule exists to stop *palette* colours bypassing the token system, and a fixed
  third-party mark is not part of the palette.
- Everything else in the app inherits `currentColor` (`BrandMark.tsx:17`). The
  Google mark must not: recolouring it is precisely what the brand terms forbid.

`aria-hidden="true"`, no accessible name — the button's own text supplies it
(FR-A11Y-4), matching `BrandMark`'s established treatment.

### 4.5 `LoginPage` — composition and states

One file, ~120 lines, no extracted sub-components. The failure callout is eight
lines of JSX with one call site and no state of its own; a `LoginNotice.tsx`
would be indirection without isolation. The two things that *are* extracted
(`GoogleMark`, `ThemeToggleButton`) are extracted because they have a second
consumer or an independent test.

State:

```tsx
const [errorCode] = useState(consumeLoginError);   // read once, module-memoised
const [redirecting, setRedirecting] = useState(false);
```

`redirecting` is one-way. `login()` performs a full navigation, so the state has
no exit path in-page (FR-STATE-2) and `disabled` makes a second activation
impossible (FR-STATE-3). The click handler is a wrapper, not `onClick={login}`
directly:

```tsx
onClick={() => {
  setRedirecting(true);
  login();
}}
```

which is also the seam that absorbs the `return_to` work's `login(from)`
signature change (PRD §7.4) as a one-argument edit.

Structure, top to bottom:

```
<div className="relative flex min-h-screen flex-col justify-center bg-background px-6 sm:px-12 lg:px-24">
  <div className="absolute right-4 top-4">        ← ThemeToggleButton   (FR-PRETOGGLE-1)
  <div className="w-full max-w-xl space-y-8">
    eyebrow:   <BrandMark/> MYFLEET               (FR-PAGE-3)
    headline:  Your cars. / One place.            (FR-PAGE-4)
    rule:      <div className="h-px bg-border"/>  (FR-PAGE-5)
    notice:    danger callout | muted line        (FR-STATE-4 / FR-STATE-5)
    cta row:   [G Continue with Google]  scopes   (FR-PAGE-6, FR-PAGE-8)
    footer:    descriptor + "Sign in with Google" (FR-PAGE-7)
</div>
```

Responsive behaviour (FR-PAGE-9): the headline uses
`text-4xl sm:text-5xl lg:text-6xl` with `max-w-[11ch]`, so it steps down rather
than overflowing; the CTA row is `flex-col sm:flex-row` so the scopes line drops
beneath the button under ~640px. Left inset scales with the same breakpoints.
Only existing tokens are used (FR-PAGE-10) — the danger callout reuses the
`border-danger-border bg-danger-subtle text-danger-subtle-foreground` trio
already established at
`components/features/vehicles/maintenance/MaintenanceQueueView.tsx:47-60`.

The notice slot renders one of three things:

| `errorCode` | Rendering |
| --- | --- |
| `null` | nothing; button reads "Continue with Google" |
| `cancelled` | `<p className="text-sm text-muted-foreground">` — no `role`, no callout, button label unchanged (FR-STATE-5) |
| anything else | `<div role="alert">` danger callout; button relabels to "Try again" (FR-STATE-4, FR-A11Y-1) |

`role="alert"` only on the danger branch: a cancellation is not an alert, and
announcing it as one is the same category error as painting it red.

The redirecting state adds `<span className="sr-only" role="status">Redirecting
to Google…</span>` (FR-A11Y-2). A `role="status"` live region is needed because
relabelling an element that has just become `disabled` is not reliably announced
— the spinner and the changed button text are visual-only signals.

**Open question 2 — resolved: keep the footer.** It carries the only sentence on
the page that says what MyFleet actually does, which is user story #1's whole
point; the headline is deliberately too terse to do that job. Keeping it also
lets the scopes line stay to a single short clause. It stays one line and muted.

### 4.6 Frontend test strategy

`loginError.test.ts` — the module memoises, so each test gets a fresh instance
via `vi.resetModules()` + `await import('./loginError')`. No test-only reset
export; the module's public surface stays two functions.

Cases: each of the four codes parses; an unknown code, an empty value and a
malformed percent-encoding all normalise to `server_error`; no `error` key
returns `null`; a hash of `#access_token=...` returns `null` and is **not**
stripped (FR-STATE-8); the hash is stripped via `replaceState` after a read while
`pathname` and `search` survive; a second call returns the memoised value.

`LoginPage.test.tsx` — rendered inside `ThemeProvider` with `../context/AuthContext`
mocked (the pattern `ThemeToggle.test.tsx:9-16` already uses for `useUpdateTheme`).
Cases: composition assertions for the headline lines, scopes line and absence of
a card; click → button `disabled` + status text present + `login` called once;
`#error=auth_failed` → `role="alert"` present and button reads "Try again";
`#error=cancelled` → no `role="alert"`, muted line present, label unchanged;
`#error=<script-ish garbage>` → generic copy rendered and
`document.body.textContent` does not contain the supplied string; authenticated
visitor navigates away (FR-STATE-9).

The no-network assertion for FR-PRETOGGLE-3 is `vi.spyOn(globalThis, 'fetch')`
asserted zero-called after cycling the control through all three preferences.
Asserting on `fetch` rather than on a mocked `useUpdateTheme` is the stronger
claim: it fails if *any* request appears, including one added later by a
different route.

`ThemeToggleButton.test.tsx` — the cycle, the labels and the preference-not-
resolved-theme icon rule, moved down from `ThemeToggle.test.tsx`.
`ThemeToggle.test.tsx` itself is **not** edited (PRD acceptance criterion); the
new file duplicates two of its assertions at the lower level, which is correct —
they are now that component's contract.

---

## 5. Data flow — a cancelled sign-in, end to end

1. User clicks "Continue with Google" → `redirecting` state → full navigation to
   `GET /api/auth/login/google` → state cookie set → 302 to Google.
2. User clicks **Cancel** → Google 302s to
   `/api/auth/callback?error=access_denied&state=…`.
3. `callbackHandler`'s new pre-check matches, logs at Info, `failLogin` clears
   `oidc_state` and 302s to `http://localhost/login#error=cancelled`.
4. SPA boots. `AuthProvider` runs `captureTokenFromHash()`, which sees no
   `access_token=` and returns null without touching the hash.
5. `LoginPage` mounts, `consumeLoginError()` reads `cancelled`, strips the hash
   via `replaceState`, memoises.
6. Page renders the muted "Sign-in cancelled." line; the button still reads
   "Continue with Google". No red, no alert, no dead end.

---

## 6. What is deliberately not built

- No new error-taxonomy plumbing on the wire beyond the four codes. No error
  details, no correlation id in the fragment (FR-ERR-4 / §8 Security).
- No retry/backoff, no automatic re-attempt. The button is the retry.
- No `LoginNotice` / `Callout` component. One call site (§4.5).
- No change to `/onboarding` or `/invites/:token/accept` (PRD non-goal).
- No `deploy/` change. `APP_LOGIN_PATH` follows `APP_HOME_PATH` and
  `APP_ONBOARDING_PATH`, which are likewise absent from the manifests and rely on
  Go defaults.

---

## 7. Risks

| Risk | Mitigation |
| --- | --- |
| `Dependencies` field types change while the `return_to` branch edits the same struct | The two edits are in different fields; resolve by taking both. Rebase before the PR per PRD §7.4. |
| `LoginPage.tsx` / `LoginPage.test.tsx` / `resource_test.go` created by both branches | Reconcile, do not overwrite. The click wrapper in §4.5 is the designated seam for `login(from)`. |
| Module-scope memoisation surprises a future reader | The rationale is in the doc comment quoted in §4.1, not just here. |
| Google mark's in-SVG white disc read as a token violation in review | Comment in `GoogleMark.tsx` states the brand constraint and why `bg-white` is unavailable (§4.4). |
| Interface change makes `main.go` compile against the wrong thing | It cannot silently: the three concrete types either satisfy the interfaces or the build fails. `make build` is the check. |

---

## 8. Verification

Per CLAUDE.md, before the PR:

```sh
make ci
grep -c "http.Error" apps/auth-service/internal/oidc/resource.go   # expect 0
kustomize build deploy/k8s/overlays/local | kubectl apply --dry-run=server -f -
kustomize build deploy/k8s/overlays/main  | kubectl apply --dry-run=server -f -
```

Manual: `/login` at 320px and at desktop width, in light and dark; the three
states forced by visiting `/login#error=cancelled`, `#error=auth_failed`, and
`#error=nonsense`.

Then `superpowers:requesting-code-review` → `audit.md`.

---

## 9. Deviations from the PRD

One, recorded so the plan phase carries it forward rather than treating it as
drift:

- **FR-ERR-6** maps any Google `error` query parameter to `cancelled`. This
  design maps `access_denied` to `cancelled` and every other value to
  `auth_failed` (§3.3). Rationale: `invalid_scope` / `invalid_request` /
  `server_error` are misconfigurations, not user choices, and reporting them as
  "cancelled" hides a real fault behind a benign message. Cost: two lines and one
  test case. The PRD's acceptance criterion — "a callback carrying Google's
  `?error=access_denied` redirects with `cancelled`" — is unaffected.

Open questions 1–4 are resolved in §4.2 (1 and 3), §4.5 (2), and §4.3 (4).
