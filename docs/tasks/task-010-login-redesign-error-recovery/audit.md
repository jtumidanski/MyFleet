
---

# Backend Audit — auth-service (DOM-* / SUB-* / SEC-*)

- **Reviewer:** backend-guidelines-reviewer
- **Service Path:** `apps/auth-service`
- **Scope:** Go changes on `task-010-login-redesign-error-recovery` (merge-base `e9a76e6` → HEAD)
  - `apps/auth-service/internal/oidc/resource.go` (modified)
  - `apps/auth-service/internal/oidc/resource_test.go` (created)
  - `apps/auth-service/cmd/main.go` (+1 line)
- **Guidelines Source:** `backend-dev-guidelines` skill
- **Date:** 2026-08-02
- **Build:** PASS
- **Tests:** 71 passed, 0 failed
- **Overall:** PASS (no blocking findings in scope; 2 non-blocking, 2 informational)

## Build & Test Results

```
$ cd apps/auth-service && go build ./...
(exit 0, no output)

$ cd apps/auth-service && go test ./... -count=1
?   github.com/jtumidanski/myfleet/apps/auth-service/cmd               [no test files]
ok  github.com/jtumidanski/myfleet/apps/auth-service/internal/jwks       0.047s
ok  github.com/jtumidanski/myfleet/apps/auth-service/internal/membership 0.020s
ok  github.com/jtumidanski/myfleet/apps/auth-service/internal/oidc       0.008s
ok  github.com/jtumidanski/myfleet/apps/auth-service/internal/session    0.296s
ok  github.com/jtumidanski/myfleet/apps/auth-service/internal/user       0.013s

$ go vet ./...   → exit 0
$ gofmt -l internal/oidc/ → (empty)
```

## Package Classification (Phase 2)

| Package | `model.go`? | `resource.go`? | Class |
|---|---|---|---|
| `internal/oidc` | no | yes (`resource.go:1`) | **Sub-domain** → SUB-* checklist |
| `internal/session` | yes (`model.go`) | yes | Domain (unchanged this branch — out of scope) |
| `internal/user` | yes (`model.go`) | yes | Domain (unchanged this branch — out of scope) |
| `internal/jwks` | no | yes | Support (key material) — unchanged |
| `internal/membership` | no | no | Support (HTTP client) — unchanged |

`internal/oidc` has no `model.go` and no `rest.go`, so the DOM-01..DOM-19 checklist
does not apply to it. `session` and `user` are untouched by this branch and were
not audited.

### Guideline drift note (affects DOM-04/05/08/17/18 applicability)

The guideline text assumes `services/<name>/`, api2go/JSON:API `Transform` /
`TransformSlice`, and a curried `server.RegisterHandler(l)(si)(...)`. **None of
those exist in this repo.** Verified: `packages/shared-go/server/handler.go:42`
exposes a single non-curried `RegisterInputHandler[T any](fn func(http.ResponseWriter,
*http.Request, T)) http.HandlerFunc`, and `grep -rn 'RegisterHandler\|api2go\|
jsonapi.ServerInformation' --include='*.go' .` returns **zero** matches for
`server.RegisterHandler`, api2go, or `jsonapi.ServerInformation` anywhere in the
tree. The house pattern is plain `r.Get(path, http.HandlerFunc)` for body-less
routes (see `apps/fleet-service/internal/vehicle/resource.go:61`,
`apps/auth-service/internal/user/resource.go:76` for the `RegisterInputHandler`
counterpart). The checks below are evaluated against the pattern this codebase
actually implements, not against the absent API.

## Sub-Domain Checklist — `internal/oidc`

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| SUB-01 | Has processor / business logic not in handler | **WARN** | Processor exists: `internal/oidc/processor.go:22` (`type Processor`), `:42` `AuthCodeURL`, `:47` `Exchange`, `:63` `Verify`. **Second clause not met:** `callbackHandler` (`internal/oidc/resource.go:118-217`) orchestrates four collaborators inline — provisioning (`:171`), membership resolution (`:178`), access mint (`:185`), refresh issue (`:196`), destination selection (`:210-213`). Pre-existing; this diff neither added nor removed orchestration. See Finding C. |
| SUB-02 | Administrator for writes / no `db.Create`/`db.Save` in `resource.go` | **PASS** | `grep -n 'db.Create\|db.Save\|db.Delete' internal/oidc/*.go` → zero matches. Package imports no gorm (`internal/oidc/resource.go:3-20`, `processor.go:7-16`). Writes are delegated across package boundaries: `d.Users.ProvisionFromGoogle` (`resource.go:171`) and `d.Sessions.IssueRefresh` (`resource.go:196`). |
| SUB-03 | POST endpoints use typed input handler | **N/A (PASS)** | Package registers only GET routes: `internal/oidc/resource.go:71` (`r.Get("/auth/login/google", ...)`) and `:72` (`r.Get("/auth/callback", ...)`). No POST/PATCH route exists, so `server.RegisterInputHandler` is correctly not used. |
| SUB-04 | No manual JSON parsing | **PASS** | `grep -n 'json.NewDecoder\|json.Unmarshal\|io.ReadAll' internal/oidc/*.go` → zero matches. Callback input is read from query params only (`resource.go:123`, `:137`, `:138`) and the signed cookie (`resource.go:240`). |

## Selected DOM-* Checks Applicable to the Changed Code

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| DOM-06 | Constructor takes `logrus.FieldLogger`, not `*logrus.Logger` | **PASS** | `internal/oidc/resource.go:69` `InitializeRoutes(log logrus.FieldLogger, d Dependencies)`; `:107` `loginHandler(log logrus.FieldLogger, ...)`; `:118` `callbackHandler(log logrus.FieldLogger, ...)`. |
| DOM-07 | No `logrus.StandardLogger()` | **PASS** | `grep -rn 'StandardLogger' --include='*.go' apps/auth-service` → zero matches. The logger is injected from `cmd/main.go:26` (`telemetry.NewLogger()`) via `main.go:87`. |
| DOM-11 | No `os.Getenv()` in handlers | **PASS** | `grep -rn 'os.Getenv' --include='*.go' apps/auth-service` → zero matches. `LoginPath` is resolved once at startup: `cmd/main.go:80` `LoginPath: config.Get("APP_LOGIN_PATH", "/login")`, backed by `packages/shared-go/config/config.go:11-16` (`os.LookupEnv`), and injected via `Dependencies.LoginPath` (`internal/oidc/resource.go:61`). |
| DOM-13 | Handlers don't call providers directly | **PASS** | All collaborator calls go through the injected interfaces declared at `internal/oidc/resource.go:30-45`; no `*Provider` type is referenced in the package. |
| DOM-14 | No direct entity creation in handlers | **PASS** | Same evidence as SUB-02. |
| DOM-16 | Error → status mapping | **PASS (adapted)** | This is a browser redirect endpoint, not a JSON:API resource; the guideline's 400/404/409/500 table does not apply. Every failure exit returns a single `302` with a coarse fragment code (`internal/oidc/resource.go:104`), which is the correct shape for an OAuth callback consumed by a browser. All 11 failure exits are exhaustively covered by test (`resource_test.go:174-318`). |
| DOM-19 | Table-driven tests | **PASS** | `internal/oidc/resource_test.go:177` `cases := []struct{...}` with 10 cases, driven by `t.Run` at `:291`. Non-table cases at `:126`, `:137`, `:321`, `:334`, `:355`. |

## Security Review

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| SEC-01 | JWT validation uses verified parsing | **PASS** | `internal/oidc/processor.go:64` `idtoken.Validate(ctx, rawIDToken, p.clientID)` — signature + audience verified against Google's keys. `grep -rn 'ParseUnverified\|ParseSigned' --include='*.go' apps/auth-service` → **zero** matches. `idtoken.Validate` does not check `nonce`, and the handler closes that gap itself at `internal/oidc/resource.go:165` with a constant-time `hmac.Equal` against the nonce bound into the signed state cookie. |
| SEC-02 | Revocation acts on validated tokens | **PASS** | `internal/session/resource.go:76-87` `logoutHandler` revokes by **opaque** refresh token read from an HttpOnly cookie (`:92`), not by claims lifted from an unvalidated JWT. Out of this diff's scope; verified anyway. |
| SEC-03 | No open redirect in `failLogin` | **PASS** | See the dedicated analysis below. |
| SEC-04 | No hardcoded secrets | **PASS** | All secret material comes from env at startup: `cmd/main.go:66-68` (`GOOGLE_CLIENT_ID`/`_SECRET`/`_REDIRECT_URL` via `config.MustGet`), `cmd/main.go:76` (`OIDC_STATE_SECRET`). The only literal key material is `internal/oidc/resource_test.go:58` `testSecret = []byte("test-state-secret")`, which is test-only and never referenced from non-`_test.go` files. |

### SEC-03 detail — open-redirect surface in `failLogin`

The redirect target is `internal/oidc/resource.go:104`:

```go
http.Redirect(w, req, d.AppBaseURL+d.LoginPath+"#error="+string(code), http.StatusFound)
```

Three operands, each proven non-request-derived:

1. `d.AppBaseURL` — struct field (`resource.go:57`), assigned once at
   `cmd/main.go:78` from `config.Get("APP_BASE_URL", "http://localhost")`.
2. `d.LoginPath` — struct field (`resource.go:61`), assigned once at
   `cmd/main.go:80` from `config.Get("APP_LOGIN_PATH", "/login")`.
3. `string(code)` — `code` is typed `loginErrorCode` (`resource.go:80`), a defined
   string type. Go will not implicitly convert a request `string` into it. All
   **11** call sites pass one of the four package constants declared at
   `resource.go:83-86`: `:129 errCancelled`, `:134 errAuthFailed`,
   `:140/:145 errInvalidState`, `:154/:160/:167 errAuthFailed`,
   `:174/:181/:193/:199 errServerError`. A repo-wide
   `grep -rn 'loginErrorCode(' --include='*.go' .` returns **zero** explicit
   conversions, so there is no path by which a request string can be laundered
   into the parameter.

**The "deliberately not `QueryEscape`d" claim checks out, and is moot.** All four
constant values (`resource.go:83-86`) are drawn from `[a-z_]` only — `cancelled`,
`invalid_state`, `auth_failed`, `server_error` — so `url.QueryEscape` would be a
no-op on the code itself and the only reason to omit it is the `#`, exactly as the
comment at `resource.go:98-101` states. That `http.Redirect` transmits the `#`
unescaped is not assumed: `resource_test.go:306-309` asserts the literal
`Location` header `http://app.test/login#error=<code>` for all ten failure cases
and passes, and `:132`, `:143`, `:327`, `:340`, `:358` pin it for the remaining
exits. Empirically confirmed, not inferred.

**No request-derived value reaches any response body.**
`grep -n 'http.Error\|w.Write\|fmt.Fprint' internal/oidc/*.go` returns zero
matches — the pre-change `http.Error` dead-ends are fully removed. The only
response bytes are the ones `http.Redirect` synthesizes from the `Location` URL,
which is constant-composed per above.

**No taxonomy leak.** The four codes are fixed at `resource.go:83-86` and are not
derived from any `err`. Underlying causes stay server-side in structured logs:
`resource.go:153`, `:159`, `:166`, `:173`, `:180`, `:192`, `:198`. Notably
`resource.go:171-175` maps *four distinct* internal failures (provisioning,
membership, mint, refresh) onto the single `server_error` code, so the browser
cannot distinguish them.

### Cookie-clear ordering — verified

`failLogin` calls `clearStateCookie(w, d.CookieSecure)` at `resource.go:103`
*before* `http.Redirect` at `:104`. `clearStateCookie` (`resource.go:266-276`)
is a `http.SetCookie` → `Header().Add`, so the `Set-Cookie` lands while headers
are still uncommitted; `http.Redirect`'s internal `WriteHeader` runs afterwards.
Ordering is correct. Every one of the 11 `failLogin` call sites is immediately
followed by `return` (`resource.go:130, 135, 141, 146, 155, 161, 168, 175, 182,
194, 200`), so no path can double-write headers. Behaviourally asserted at
`resource_test.go:310`, `:343`, `:361` via `clearsStateCookie` (`:152-159`),
which passes for all twelve failure scenarios. The success path is likewise
ordered correctly: `session.SetRefreshCookie` at `resource.go:207` precedes the
redirect at `:215`.

## Summary

### Blocking (must fix)

*None.* Build passes, `go vet` passes, all 71 tests pass, and no SUB-* or SEC-*
check fails in the changed scope.

### Non-Blocking (should fix)

- **[SEC-03-adj] Unauthenticated pre-state-check cookie clearing.**
  `resource.go:123-142` handles `?error=` and the missing-`code`/`state` case
  *before* `verifyStateCookie` runs (`:143`), and both exits now clear the state
  cookie via `failLogin`. Because `GET /auth/callback` is a public, CSRF-tokenless
  route (`main.go:87` registers it outside the JWT group), any third party who can
  cause a victim's browser to issue `GET /auth/callback?error=access_denied` can
  destroy that browser's in-flight `oidc_state` cookie and abort the login. This
  is a behaviour change: the pre-diff code returned `http.Error` on these paths
  and left the cookie intact. Impact is a retry-able login nuisance, not
  credential exposure — the victim clicks "Try again" and a fresh state is
  minted at `resource.go:113`. Deferring the clear on the two pre-verification
  exits would close it.

- **[LOG-1] Unbounded request-derived value in a log field.**
  `resource.go:127` logs `req.URL.Query().Get("error")` verbatim with no length
  cap or character filtering. The endpoint is public and unauthenticated, so an
  attacker can drive arbitrary multi-kilobyte values into the log pipeline. It
  does **not** reach the response body (verified above) and logrus escapes it
  structurally, so this is log-volume hygiene rather than injection. Truncating
  to a sane length, or only logging the value when it matches a known OAuth2
  error token, would resolve it.

### Informational (pre-existing, not introduced by this diff)

- **[C] Orchestration in the handler.** `callbackHandler`
  (`resource.go:118-217`) is ~100 lines coordinating provisioning, membership
  resolution, token minting, cookie writes, and destination selection — the
  guidelines' "Business logic in handlers" anti-pattern, and the reason SUB-01
  is WARN rather than PASS. This diff did not introduce it and in fact improved
  the situation: the consumer-side interfaces at `resource.go:30-45` are what
  made `resource_test.go` possible without Google or a database. Extracting a
  `Processor.CompleteLogin(ctx, code, nonce)` returning `(access, refresh,
  fleetID, error)` would leave the handler with only cookie/redirect concerns.

- **[D] Manual body decoding outside the diff.**
  `internal/session/resource.go:91-105` `readRefreshToken` uses
  `json.NewDecoder(req.Body).Decode(...)` directly rather than
  `server.RegisterInputHandler`, and reads `req.Body` with no
  `http.MaxBytesReader` cap. Unchanged on this branch and outside the audited
  scope; flagged only so it is not mistaken for having been cleared.

---

# Frontend Audit — task-010-login-redesign-error-recovery (FE-* checklist)

- **Audit Scope:** branch `task-010-login-redesign-error-recovery`, merge-base `e9a76e6` → HEAD `25dfd88`, TypeScript/React changes only (8 files under `apps/web/src`)
- **Guidelines Source:** `frontend-dev-guidelines` skill
- **Date:** 2026-08-02
- **Build:** PASS
- **Tests:** 208 passed (32 files), 0 failed
- **Overall:** PASS (zero blocking FE-* failures; 2 medium + 4 low non-blocking findings)

## Build & Test Results

```
> build
> tsc -b && vite build
✓ 1783 modules transformed.
dist/assets/index-CGK79bGx.js   555.95 kB │ gzip: 163.43 kB
(!) Some chunks are larger than 500 kB after minification.   <-- pre-existing, not introduced here
✓ built in 4.37s
```

```
 Test Files  32 passed (32)
      Tests  208 passed (208)
   Duration  3.98s
```

`src/components/ThemeToggle.test.tsx` was not modified and still passes (acceptance criterion confirmed).

## File Inventory

| File | Classification |
|------|----------------|
| `apps/web/src/pages/LoginPage.tsx` | Page (rewritten) |
| `apps/web/src/pages/LoginPage.test.tsx` | Test (new) |
| `apps/web/src/components/ThemeToggleButton.tsx` | Component (new, presentational) |
| `apps/web/src/components/ThemeToggleButton.test.tsx` | Test (new) |
| `apps/web/src/components/ThemeToggle.tsx` | Component (reduced to mutation wrapper) |
| `apps/web/src/components/GoogleMark.tsx` | Component (new, presentational SVG) |
| `apps/web/src/lib/auth/loginError.ts` | Other (module-scope fragment parser) |
| `apps/web/src/lib/auth/loginError.test.ts` | Test (new) |

No services, schemas, hooks, or model types were changed on this branch.

## Anti-Pattern Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-01 | No `any` type | PASS | Grep for `: any` / `as any` / `<any` over all 8 in-scope files returns only the English word "anything" in a comment at `loginError.test.ts:28`. Zero type-level matches. |
| FE-02 | No manual class concatenation | PASS | Grep for `className={"` and `` className={` `` over all in-scope `.tsx` returns zero. Every `className` is a static string literal: `LoginPage.tsx:41,46,47,54,55,56,59,68,73,76,79,90,92,96,104,109`; `ThemeToggleButton.tsx:46`. `GoogleMark.tsx:22` and `BrandMark.tsx:16` pass a `className` prop straight through with no base class to merge, so `cn()` is not required. |
| FE-03 | No direct API client calls in components | PASS | Grep for `lib/api/client` across `LoginPage.tsx`, `ThemeToggle.tsx`, `ThemeToggleButton.tsx`, `GoogleMark.tsx` returns zero. `ThemeToggle.tsx:3` reaches the network only via the `useUpdateTheme` hook. |
| FE-04 | No inline Zod schemas in components | PASS | Zero `z.object(` / `z.string(` in scope. No forms were added. |
| FE-05 | No spinners for content loading | PASS | Single `animate-spin` in scope, `LoginPage.tsx:90`, inside the sign-in `<Button>` and gated on `redirecting`. This is the submit-button exemption in `anti-patterns.md:87-91`. No content area uses a spinner. |
| FE-06 | No hardcoded colors | PASS | `LoginPage.tsx` uses only semantic tokens: `bg-background` (41), `text-muted-foreground` (47,73,96,109), `text-foreground` (55), `bg-border` (59), `border-danger-border bg-danger-subtle text-danger-subtle-foreground` (68). All four `danger-*` tokens are defined — `tailwind.config.ts:63-67` mapping to `index.css:41-44` (light) and `index.css:86-89` (dark). `GoogleMark.tsx:23-38` uses fixed hex `fill` **SVG attributes**, not Tailwind palette classes; FE-06 governs class names, so this is out of scope and is documented at `GoogleMark.tsx:1-19`. Independently confirmed by `src/test/conventions.test.ts:113-153`, which passes. |
| FE-07 | No state mutation | PASS | Zero `.push(` / `.splice(` / `.sort(` in scope. The only state writes are `setRedirecting(true)` (`LoginPage.tsx:85`) and `setPreference(next)` (`ThemeToggle.tsx:28`), both scalar replacements. |
| FE-08 | No default exports for components | PASS | Zero `export default` in scope. Named exports at `loginError.ts:28`, `loginError.ts:66`, `ThemeToggleButton.tsx:33`, `ThemeToggle.tsx:20`, `GoogleMark.tsx:20`, `LoginPage.tsx:22`. |
| FE-09 | Error handling with `createErrorFromUnknown` | PASS (unchanged) | Zero `.catch(` in scope. The one failure path is the theme mutation, surfaced via `onError` + `toast.error(...)` at `ThemeToggle.tsx:29-33`. The branch diff confirms this block was moved verbatim from the pre-existing `onClick` handler, not authored here — no regression. It carries a fixed message rather than `createErrorFromUnknown()` because the raw error holds nothing user-actionable; flagged as pre-existing, not introduced. |

## Architecture Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-10 | JSON:API model shape | N/A | No model types added or changed. `ThemeToggleButton.tsx:2` imports the pre-existing `ThemePreference` from `types/models/user`. |
| FE-11 | Service extends `BaseService` | N/A | No files under `services/api/` changed on this branch. |
| FE-12 | Query key factory uses `as const` | N/A | No query keys added. Pre-existing `authKeys` at `lib/hooks/api/auth.ts:7-10` is already `as const`; untouched. |
| FE-13 | Forms use `react-hook-form` + `zodResolver` | N/A | No forms in scope. The sign-in control is a `<Button type="button">` (`LoginPage.tsx:77-78`) performing a full navigation, not a form submit. |
| FE-14 | Schema in `lib/schemas/` with inferred type | N/A | No Zod schemas added. `LoginErrorCode` (`loginError.ts:2`) is a hand-written union over a closed server-supplied set, not user input requiring validation. |

## Styling Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-15 | Interactive elements show `cursor-pointer` | PASS | The only two clickable surfaces added are shadcn `<Button>`s — `LoginPage.tsx:77` and `ThemeToggleButton.tsx:38` — and the `buttonVariants` cva base string at `components/ui/button.tsx:7` ends with `cursor-pointer`, so it is applied explicitly rather than relying on Preflight. No clickable `<div>`, table row, or `render`-prop trigger was added. |

## Testing Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-16 | Tests exist for changed components | PASS | `LoginPage.test.tsx` (9 tests), `ThemeToggleButton.test.tsx` (5 tests), `loginError.test.ts` (13 tests). `GoogleMark.tsx` has no dedicated test — acceptable: it is a static, prop-less-but-for-`className` SVG with `aria-hidden`, covered transitively by the button-name query at `LoginPage.test.tsx:66-68`. |
| FE-17 | Mocks updated when services changed | N/A | No service interfaces changed. `LoginPage.test.tsx:11-13` mocks `../context/AuthContext` exporting only `useAuth`, which matches the single import at `LoginPage.tsx:4`. |

## Findings (most severe first)

### F1 — MEDIUM (a11y): the `role="status"` live region is mounted together with its content, so FR-A11Y-2 is not actually delivered

`LoginPage.tsx:103-107` renders the live region only when `redirecting` is true:

```tsx
{redirecting && (
  <span className="sr-only" role="status">Redirecting to Google…</span>
)}
```

Assistive tech announces *changes inside an already-present* live region. Inserting the region and its text in the same commit is unreliable across NVDA/JAWS/VoiceOver. This is compounded by `disabled={redirecting}` at `LoginPage.tsx:80`: the button the user just activated becomes disabled in the same commit, which drops focus to `<body>` and further suppresses announcement. The comment at `LoginPage.tsx:101-102` correctly diagnoses why a relabelled disabled button is not announced, then adopts a mitigation with the same failure mode.

Fix: render the region unconditionally and toggle only its text — `<span className="sr-only" role="status">{redirecting ? 'Redirecting to Google…' : ''}</span>`.

Note the test cannot catch this: `LoginPage.test.tsx:106` asserts DOM presence and text content, both of which hold either way.

### F2 — MEDIUM (test quality): the `fetch` spy in the theme-cycle test cannot fail for the regression it claims to guard

`LoginPage.test.tsx:169` — `expect(fetchSpy).not.toHaveBeenCalled()` — with the comment at `LoginPage.test.tsx:148-149` claiming "it fails if ANY request appears".

**Transport is correct.** `apiClient.request` uses global `fetch` (`packages/shared-ts/src/apiClient.ts:27`), as do `logoutRequest` (`lib/hooks/api/auth.ts:46`) and the refresh hook (`lib/api/client.ts:27`). There is no axios/XHR anywhere, so `vi.spyOn(globalThis, 'fetch')` does cover every network path.

**The assertion is nonetheless not load-bearing.** `updateThemePreference` short-circuits before any transport call at `lib/hooks/api/auth.ts:68` (`if (!getAccessToken()) return null;`), `getAccessToken` reads `localStorage` (`lib/api/token.ts:9-11`), and `beforeEach` calls `localStorage.clear()` at `LoginPage.test.tsx:72`. So no access token exists during the test — meaning even if `LoginPage` rendered the mutation-bearing `ThemeToggle`, zero fetches would fire and the assertion would still pass.

The test *would* still fail if `ThemeToggle` were substituted, but only incidentally: `useUpdateTheme` → `useMutation` throws "No QueryClient set" because the render tree at `LoginPage.test.tsx:52-59` has no `QueryClientProvider`. That is an accident of the harness, not the stated claim.

Fix: seed a token before rendering (`localStorage.setItem('access_token', 'x')` after the `clear()`), which makes the spy genuinely load-bearing for FR-PRETOGGLE-3.

### F3 — PASS with evidence (security): no attacker-supplied fragment value can reach the DOM

Full trace, since this was the specific concern:

1. `loginError.ts:47` reads the raw value: `const raw = params.get('error') ?? ''`.
2. `loginError.ts:51` narrows it: `return isLoginErrorCode(raw) ? raw : 'server_error'`. `isLoginErrorCode` (`loginError.ts:32-34`) tests membership in `CODES` (`loginError.ts:9`), a source-literal array. The return type is `LoginErrorCode | null` — the raw string is unreachable past this line.
3. `noticeFor` (`loginError.ts:28-30`) indexes the constant `NOTICES` table (`loginError.ts:19-25`), whose every `message` is a source string literal.
4. `LoginPage.tsx:69` and `LoginPage.tsx:73` render `{notice.message}` as a React text child (auto-escaped). Zero `dangerouslySetInnerHTML` anywhere in scope.
5. The strip at `loginError.ts:45` rebuilds the URL from `window.location.pathname + window.location.search`, both browser-normalised, so `replaceState` is not an injection sink either.

Covered by `loginError.test.ts:31-39` (unknown / empty / malformed `%zz` / injected `<script>` all → `server_error`) and `LoginPage.test.tsx:133-138` (`document.body.textContent` does not contain `alert(1)`). Verdict: no XSS reachable.

### F4 — LOW: `readAndStrip` destroys the whole fragment, and FR-STATE-8 currently holds only by render ordering

`loginError.ts:45` strips the entire hash whenever an `error` key is present. A crafted `#access_token=…&error=…` would therefore take the token with it.

In practice the token survives, but only because of ordering that nothing asserts: `AuthProvider` calls `captureTokenFromHash()` inside a `useState` lazy initializer **during its own render** (`context/AuthContext.tsx:31-34`), and React renders the parent provider before the child `LoginPage` reaches `useState(consumeLoginError)` (`LoginPage.tsx:30`). So the token is captured and the hash cleared first, and it is the *error* that is silently dropped — the safe direction.

`loginError.test.ts:48-52` only covers the token-alone case, so this ordering dependency is untested. Cheap hardening: `if (params.has('access_token')) return null;` before the strip at `loginError.ts:45`, making the mutual exclusion structural rather than incidental.

### F5 — LOW (a11y): the `role="alert"` callout is present at first paint rather than inserted as a change

`LoginPage.tsx:64-71` renders the alert during the initial render, because `errorCode` comes from a `useState` initializer (`LoginPage.tsx:30`). Live regions already present at document load are inconsistently announced across SR/browser pairs — the same class of issue as F1. It degrades gracefully (the callout precedes the button in DOM order, so a sequential reader reaches it), but FR-A11Y-1's "announced" is not guaranteed. Consider moving focus to the callout on mount when `failed`, or rendering the region and filling its text in an effect.

The design note that `role="alert"` belongs only on the danger branch, with a cancellation as a plain muted `<p>` (`LoginPage.tsx:72-74`), is correct and was not flagged.

### F6 — LOW: module-scope memoisation makes the notice permanent for the page load

`captured` (`loginError.ts:54`) lives as long as the module instance, so any SPA re-entry to `/login` without a full navigation would re-show a stale notice, and there is no dismiss affordance.

Currently unreachable, and the memoisation rationale at `loginError.ts:56-65` is sound (StrictMode double-mount would otherwise swallow the notice in dev only). Verified the escape paths: the only exit from `/login` is `login()`, which is `window.location.href = '/api/auth/login/google'` (`context/AuthContext.tsx:47-50`) — a full navigation that yields a fresh module instance. `logout()` (`context/AuthContext.tsx:52-57`) is a client-side state change, but it can only run from an authenticated session in which `LoginPage` never rendered, so `captured` is still `undefined` on arrival. Recorded because the invariant is behavioural, not structural — a future "soft logout" or in-page route back to `/login` breaks it silently.

### F7 — NIT (test quality): the "no card" assertion is close to vacuous

`LoginPage.test.tsx:86` — `expect(container.querySelector('.bg-card')).toBeNull()` — passes for any markup that does not literally carry the `.bg-card` class. A re-introduced card built from `<div className="rounded-lg border bg-background p-6 shadow">` would sail through. It cannot fail for the design regression FR-PAGE-4 describes. Assert on the intended typographic structure instead (e.g. the headline is a direct descendant of the page container).

### F8 — NIT: `useState` at `LoginPage.tsx:30` is redundant

`const [errorCode] = useState(consumeLoginError)` discards the setter, and the value is already memoised at module scope (`loginError.ts:54-70`). `const errorCode = consumeLoginError()` is equivalent and one concept lighter. Harmless as written.

### F9 — NIT: duplicated test matrices after the extraction

`ThemeToggleButton.test.tsx:16-30` duplicates `ThemeToggle.test.tsx:37-53`, and `ThemeToggleButton.test.tsx:36-42` duplicates `ThemeToggle.test.tsx:97-107`. Not a violation — leaving `ThemeToggle.test.tsx` untouched was an explicit acceptance criterion — but the `ThemeToggle` copies now assert the child's contract through a wrapper. Worth trimming to the mutation/toast behaviour (`ThemeToggle.test.tsx:63-91`) once the criterion is discharged.

## Summary

### Blocking (must fix)
- None. All 15 applicable FE-* checks PASS; FE-10 through FE-14 and FE-17 are N/A for this branch.

### Non-Blocking (should fix)
- **F1** — `role="status"` live region mounted with its content at `LoginPage.tsx:103-107`; FR-A11Y-2 not reliably delivered.
- **F2** — `fetch` spy at `LoginPage.test.tsx:169` cannot fail for its stated regression (`lib/hooks/api/auth.ts:68` short-circuits with no token).
- **F4** — `loginError.ts:45` fragment strip relies on untested render ordering for FR-STATE-8.
- **F5** — `role="alert"` present at first paint (`LoginPage.tsx:64-71`); announcement not guaranteed.
- **F6** — module-scope memoisation (`loginError.ts:54`) is invariant-dependent, not structurally safe.
- **F7 / F8 / F9** — nits: near-vacuous "no card" assertion, redundant `useState`, duplicated test matrices.

---

# Plan Audit — task-010-login-redesign-error-recovery

- **Reviewer:** plan-adherence-reviewer
- **Plan Path:** `docs/tasks/task-010-login-redesign-error-recovery/plan.md`
- **Branch:** `task-010-login-redesign-error-recovery` (HEAD `25dfd88`)
- **Base Branch:** `main` (merge-base `e9a76e6`)
- **Implementation commits:** `0b37ac1`, `3f2c31f`, `1c99ebe`, `632f98a`, `9766dd9`, `25dfd88`
- **Date:** 2026-08-02
- **Overall:** MOSTLY_COMPLETE / NEEDS_REVIEW

## Executive Summary

Tasks 1–6 were implemented faithfully and essentially verbatim against the plan's
specified code. All 11 source files in the plan's File Structure table exist with
the specified content, no extra source files were touched, and no dependencies
were added. `make ci` passes end to end (exit 0), and I additionally ran both
kustomize server dry-runs against the `bee` cluster — both apply cleanly, which
closes Task 7 Step 3. Task 7 is the only PARTIAL: Step 4 (manual 320px light/dark
check) requires a human, and Step 5 (reconcile with the `return_to` work) was
deliberately deferred.

The one finding that materially matters is Step 5. The `return_to` work has now
landed on `origin/main` (commit `18eda67`) and collides with this branch on every
file the plan anticipated. Two of those collisions fail loudly (a Go
duplicate-symbol error and a changed `setStateCookie` signature), but one fails
**silently**: this branch's `LoginPage.tsx` drops `useLocation`/`from` and calls
bare `login()`, and its `LoginPage.test.tsx` replaces the file on `origin/main`
that guards that behaviour. A naive "take ours" merge regresses post-login return
paths with no test failure and no type error.

## Task Completion

| # | Task | Status | Evidence / Notes |
|---|------|--------|------------------|
| 1 | Consumer-side interfaces on `Dependencies` | DONE | `apps/auth-service/internal/oidc/resource.go:27-45` declares `Authenticator`, `UserProvisioner`, `TokenIssuer`; fields narrowed at `:51-53`. Fakes + `okDeps`/`okAuthenticator`/`stateCookie`/`callback` at `resource_test.go:22-119`. Success-path tests at `:126-146`. `cmd/main.go:72-83` compiles unchanged against the concrete processors. Commit `0b37ac1`. |
| 2 | Redirect every failure exit to `/login#error=` | DONE | `loginErrorCode` + four constants at `resource.go:80-87`; `failLogin` at `:102-105`, clearing the cookie **before** `http.Redirect` as required. All nine exits rewritten: `:140`, `:145`, `:154`, `:160`, `:167`, `:174`, `:181`, `:193`, `:199`. `grep -c "http.Error" resource.go` → **0** (base had exactly 9). `LoginPath` at `:61`; wired at `cmd/main.go:80`. Tests `resource_test.go:174-330`. Commit `3f2c31f`. |
| 3 | Handle Google's `?error=` first | DONE | Pre-check at `resource.go:120-136`, above the `code`/`state` read; `access_denied` → `errCancelled`, everything else → `errAuthFailed`; logged at **Info** with an `oauth_error` field. Tests `resource_test.go:334-368`. Matches the design §9 deviation the plan carried forward. Commit `1c99ebe`. |
| 4 | `lib/auth/loginError.ts` | DONE | `loginError.ts:1-70` — closed code set (`:9`), module-scope memo (`:54`, `:66-70`), read-and-strip preserving path+search (`:45`), unknown/empty/malformed/injected → `server_error` (`:51`). 16 tests pass. Plan text said "14 tests" — an adjudicated arithmetic slip; the plan's own test code expands to 16. Commit `632f98a`. |
| 5 | Extract `ThemeToggleButton` | DONE | `ThemeToggleButton.tsx:1-49` holds the cycle map, icon map and aria contract verbatim; `ThemeToggle.tsx:20-37` is now only the mutation wrapper. `ThemeToggle.test.tsx` is **byte-for-byte unmodified** (`git diff e9a76e6..HEAD -- ThemeToggle.test.tsx` is empty) and its 5 tests pass, including mutation and toast-on-failure at `:64-92` — the direct evidence for FR-PRETOGGLE-4. Commit `9766dd9`. |
| 6 | Rewrite `/login` | DONE | `GoogleMark.tsx:1-42` (in-SVG `fill="#FFFFFF"` disc at `:23`); `LoginPage.tsx:1-115` matches the plan's composition line for line — `bg-background` (`:41`), headline `:54-57`, danger callout with exactly `border-danger-border bg-danger-subtle text-danger-subtle-foreground` (`:68`), `role="status"` region `:103-107`, label precedence `:94`. 9 tests pass (plan said "10" — same adjudicated slip). `vi.resetModules()` at `LoginPage.test.tsx:46` **does** precede the dynamic imports at `:47-50`. Commit `25dfd88`. |
| 7 | Full verification | PARTIAL | Steps 1–3 satisfied; Steps 4–5 outstanding; Step 6 is this audit; Step 7 pending. |

**Task-level:** 6 / 7 DONE, 1 PARTIAL.
**Step-level:** 41 / 46 steps evidenced.
**Skipped without approval:** 0. **Partial implementations:** 1 (Task 7).

### Task 7 step detail

| Step | Status | Evidence |
|---|---|---|
| 1 — `make ci` | DONE | Exit code 0. `lint-check`, `vet`, `test`, `build`, `fe-test` (208 web + 7 shared-ts), `fe-build`, `manifests`, `carfax-template` all pass. |
| 2 — no `http.Error` survives | DONE | `grep -c "http.Error" apps/auth-service/internal/oidc/resource.go` → `0`. |
| 3 — render + dry-run both overlays | DONE | `tools/check-manifests.sh` (inside `make ci`) reports both overlays render and `main` ships no PVC / Secret / ClusterRole / ClusterRoleBinding / placeholders. I additionally ran both `kubectl apply --dry-run=server` passes against context `bee`: every resource reports `configured`/`created (server dry run)`, no errors (only benign `last-applied-configuration` warnings). |
| 4 — manual 320px light/dark check | OUTSTANDING | Requires a human at a browser. Not a defect. FR-PAGE-9 / FR-PAGE-10 / FR-A11Y-5 rest entirely on this step. |
| 5 — reconcile with `return_to` | OUTSTANDING | Deliberately not performed; strategy pending a human decision. See Finding P1 — the conflict is now concrete and partly silent. |
| 6 — code review | IN PROGRESS | This document. |
| 7 — commit review fixes | PENDING | Working tree clean (`git status --porcelain` empty). |

## Acceptance Criteria Coverage — the plan's own table, row by row

| PRD criterion | Plan claims | Verdict |
|---|---|---|
| No `Card`, left-aligned, `bg-background` | Task 6 Step 4 + "renders the headline…" test | **PARTIAL evidence.** Implementation is correct (`LoginPage.tsx:41` `bg-background`; `max-w-xl` with no `mx-auto` inside a `flex flex-col` → left-aligned). But the cited test only asserts `.bg-card` is null plus the text — it asserts neither `bg-background` nor alignment. Both rest on the unrun Step 4. |
| Headline `Your cars.` / `One place.`, second line muted | Task 6 Step 4 + same test | PASS. Text asserted (`LoginPage.test.tsx:83-84`); `text-muted-foreground` at `LoginPage.tsx:56` (mutedness itself is visual → Step 4). |
| Solid button, Google mark, three scopes named | Task 6 Steps 3-4 + same test | PASS. `Button` with no `variant` → `default` → `bg-primary` (`ui/button.tsx:11,26`); `GoogleMark` at `LoginPage.tsx:92`; scopes line asserted at `LoginPage.test.tsx:85`. |
| Press disables and shows a redirecting state | Task 6 test | PASS — `LoginPage.test.tsx:100-112`, including the double-click guard. |
| `#error=auth_failed` → `role="alert"` + "Try again" | Task 6 test | PASS — `LoginPage.test.tsx:115-120`. |
| `#error=cancelled` → neutral line, label unchanged | Task 6 test | PASS — `LoginPage.test.tsx:124-130`, asserts `queryByRole('alert')` is null. |
| `#error=<garbage>` → generic copy, not echoed | Task 4 + Task 6 tests | PASS — `loginError.test.ts:31-39` (4 shapes) + `LoginPage.test.tsx:133-138`. |
| Fragment stripped after reading | Task 4 + Task 6 tests | PASS — `loginError.test.ts:55-61` (path+query preserved) + `LoginPage.test.tsx:141-145`. |
| Theme control on `/login`, cycles three, no `PATCH` | Task 5 + Task 6 fetch-spy test | PASS as a plan item — `LoginPage.test.tsx:150-171`. I separately verified that `ThemeProvider` really does wrap `/login` in production (`AppProviders.tsx` places it **above** `AuthProvider`, and `App.tsx:19` routes `/login` inside it), so `useTheme()` on a signed-out page cannot throw. (The frontend reviewer's F2 notes the spy is weaker than intended; that is a test-strength point, not a plan-adherence one.) |
| `ThemeToggle` unchanged behaviour, tests unmodified | Task 5 Steps 4-6 | PASS — empty diff on `ThemeToggle.test.tsx`; mutation and toast assertions (`:64-92`) still pass. |
| All nine exits 302 with right fragment; `grep -c` → 0 | Task 2 Steps 4-5 + failure table | PASS. Base had exactly 9 `http.Error` calls; 10 subtests cover all 9 exits (the missing-`code`/missing-`state` exit is split in two). `grep -c` → 0. |
| `?error=access_denied` → `cancelled` | Task 3 test | PASS — `resource_test.go:334-351`, also asserts Info-level logging. |
| Failure log lines still fire | Task 2 `loggedAt(...ErrorLevel...)` | PASS — every table row with a `logMsg` asserts the exact original message at Error level (`resource_test.go:313-315`); all messages match `resource.go` verbatim. |
| Early exits clear the state cookie | Task 2 `clearsStateCookie` + Task 3 | PASS — asserted on all 10 failure subtests and both provider-error tests. |
| `APP_LOGIN_PATH` defaults and overrides | Task 2 Step 7 + `TestCallback_failureHonoursLoginPath` | PASS. Default `/login` at `cmd/main.go:80`; override asserted at `resource_test.go:321-330`. Absent from `deploy/` exactly as the plan predicted — consistent with `APP_HOME_PATH`/`APP_ONBOARDING_PATH`, which are likewise absent and rely on Go defaults. |
| Legible at 320px, light and dark | Task 7 Step 4 | **OUTSTANDING** — manual step not run. |
| `make ci` passes | Task 7 Step 1 | PASS — exit 0. |
| Both overlays render and dry-run | Task 7 Step 3 | PASS — verified by this audit against `bee`. |
| Code review before PR, findings in `audit.md` | Task 7 Step 6 | In progress (this file, alongside the backend and frontend sections above). |

## Findings

### P1 — HIGH: merging this branch as-is silently regresses the `return_to` feature

Task 7 Step 5 was deliberately deferred, which is fine. What is worth surfacing is
that the conflict is no longer hypothetical, and it is not uniformly loud. The
`return_to` work landed on `origin/main` in `18eda67` ("fix(web,auth): make fleet
invites acceptable end to end") and touches exactly the four files the plan
anticipated, plus two more.

**Loud collisions** — a merge cannot compile, so these will be noticed:

- `apps/auth-service/internal/oidc/resource_test.go` — add/add. Both branches
  declare package-level `var testSecret = []byte("test-state-secret")` (ours at
  `:58`, theirs at `:16`) → duplicate declaration.
- `setStateCookie` gained a `returnPath` parameter on `origin/main`
  (`resource.go:217`) and `verifyStateCookie` now returns
  `(nonce, returnPath string, ok bool)` (`:237`). Our `stateCookie` helper
  (`resource_test.go:94-105`) calls the 5-arg form and our `callbackHandler`
  consumes the 2-value form.
- `resource.go` itself: `origin/main` restructured the state cookie into a
  5-field format with legacy-4-field compatibility and added `safeReturnPath`
  (`:82`), while still containing all nine `http.Error` exits (`:134`–`:192`)
  that this branch deleted. Every one of the nine is a conflict hunk.

**Silent collision** — this is the one that matters:

- `apps/web/src/pages/LoginPage.tsx` — `origin/main` added `useLocation`, derived
  `const from = (location.state as { from?: string } | null)?.from`, changed the
  bounce to `navigate(from ?? '/', { replace: true })`, and changed the click to
  `onClick={() => login(from)}`. This branch's rewrite has **none** of that: no
  `useLocation`, hardcoded `navigate('/')` at `:34`, bare `login()` at `:86`.
- `apps/web/src/pages/LoginPage.test.tsx` — add/add. `origin/main`'s version
  exists solely to guard the above (`forwards the attempted path to login()` /
  `calls login() with no return path on a direct visit`). Ours replaces it and
  contains no return-path case.
- `apps/web/src/context/AuthContext.tsx` — `login` is now
  `(returnTo?: string) => void`. Because the parameter is **optional**, our bare
  `login()` call and our `baseAuth()` fixture both still typecheck.

Consequence: if the reconciliation resolves `LoginPage.tsx` and
`LoginPage.test.tsx` by taking this branch's version — the natural move, since
ours is a whole-file rewrite and theirs is a 14-line delta — then `tsc`, `vitest`
and `make ci` all stay green while invite-accept deep links stop returning the
user to the invite after sign-in. The plan named `onClick` as "the designated
seam" for exactly this; that guidance now needs to be executed, not merely
recorded.

The reconciled `LoginPage.tsx` needs `useLocation` + `from` restored,
`navigate(from ?? '/')`, and `login(from)` inside the existing `onClick` wrapper
so `setRedirecting(true)` is preserved; the reconciled `LoginPage.test.tsx` needs
both branches' case lists merged.

### P2 — LOW: FR-STATE-8 is tested in only one direction

`loginError.test.ts:48-52` proves `consumeLoginError` leaves an `#access_token=`
fragment untouched. Nothing proves the converse — that `captureTokenFromHash`
leaves an `#error=` fragment untouched — even though `AuthProvider` runs it
*first*, in a `useState` initializer (`AuthContext.tsx:31-34`), before
`LoginPage` ever mounts. If it stripped unconditionally, every error callout
would vanish in production while every unit test stayed green, because
`LoginPage.test.tsx:11-13` mocks the entire `AuthContext` module and never
exercises the real provider.

I verified by inspection that the behaviour is correct: `lib/api/token.ts:28`
early-returns when the hash does not contain `access_token=`, before any
`replaceState`. So this is a coverage gap, not a bug. (The frontend reviewer
reached the same place independently — see F4 above; this note adds the
resolution.) A one-line case asserting an `#error=` hash survives
`captureTokenFromHash` would close it cheaply.

### P3 — LOW: the plan file was never updated to reflect execution

All 46 checkboxes in `plan.md` are still `- [ ]`; none were ticked
(`grep -c '^- \[x\]'` → 0). The work was done, so this is a traceability concern
rather than a completion one, but the plan is the tracking artifact and a reader
cannot distinguish "not started" from "finished" by looking at it — nor can they
see that Task 7 Steps 4–5 are the two genuinely outstanding items.

### P4 — INFO: two plan arithmetic slips, already adjudicated

Task 4 Step 4 says "PASS (14 tests)" and Task 6 Step 5 says "(10 tests)". Actual
counts are 16 and 9. The plan's own inlined test code expands to those numbers;
the prose miscounted. Recorded so a future reader does not re-flag it.

### P5 — INFO: two documented, sound deviations

- `LoginPage.test.tsx:43-50` imports `ThemeProvider` inside the same
  post-`vi.resetModules()` `Promise.all` as `LoginPage`, rather than statically
  as the plan wrote it. **The reset at `:46` correctly precedes both imports.**
  Without this, `LoginPage`'s transitive post-reset import of `ThemeContext`
  would create a different React Context object than a pre-reset `ThemeProvider`
  binding, and `useTheme` would throw. Rationale is documented in the file.
- `GoogleMark.tsx:13` reworded the doc comment to avoid the literal substring
  `bg-white`, which `src/test/conventions.test.ts` matches against raw file text.
  `conventions.test.ts` passes (11 tests) with `GoogleMark.tsx` and
  `LoginPage.tsx` in scope.

## Build & Test Results

| Target | Build | Tests | Vet / Lint | Notes |
|---|---|---|---|---|
| `apps/auth-service` | PASS | PASS | PASS | `go test -race ./... -count=1`: all packages `ok`. `internal/oidc` reports 16 `TestCallback*` PASS lines (6 top-level + 10 subtests), matching the plan's intent. |
| `apps/web` | PASS | PASS | PASS | `tsc -b && vite build` clean (pre-existing >500 kB chunk warning only). 208 tests. Affected files: `loginError.test.ts` 16, `LoginPage.test.tsx` 9, `ThemeToggleButton.test.tsx` 5, `ThemeToggle.test.tsx` 5 (unmodified), `conventions.test.ts` 11. |
| `packages/shared-ts` | n/a | PASS | n/a | 7 tests. |
| `deploy/k8s` | n/a | PASS | n/a | `check-manifests.sh` passes; both overlays render; both `kubectl apply --dry-run=server` runs against `bee` apply cleanly. |
| `make ci` (whole) | PASS | PASS | PASS | Exit code 0. |

No dependencies added: `package.json`, `package-lock.json` and `go.mod` are absent
from the branch diff. No `TODO`/`FIXME`/`.skip`/`.only` markers introduced.

## Overall Assessment

- **Plan Adherence:** MOSTLY_COMPLETE — Tasks 1–6 are faithful, near-verbatim
  implementations of the specified code with the specified test coverage; Task 7
  is the only partial, and its two missing steps are the two consciously deferred.
- **Recommendation:** NEEDS_REVIEW — not because the implemented work is
  deficient, but because P1 must be settled by a human before merge, and Task 7
  Step 4 needs human eyes at 320px.

## Action Items

1. **Reconcile with `origin/main`'s `return_to` work before opening the PR
   (Task 7 Step 5).** Treat `LoginPage.tsx` and `LoginPage.test.tsx` as the risky
   pair: restore `useLocation` / `from`, `navigate(from ?? '/')` and `login(from)`
   inside the existing `onClick` wrapper, and merge both branches' test case
   lists. Expect loud conflicts in `resource.go` / `resource_test.go` (duplicate
   `testSecret`, changed `setStateCookie` / `verifyStateCookie` signatures, nine
   `http.Error` hunks) and take both branches' `Dependencies` fields. Re-run
   `make ci` afterwards.
2. **Run the manual visual check (Task 7 Step 4)** at 320px and desktop width, in
   both light and dark, across `/login`, `#error=cancelled`, `#error=auth_failed`
   and `#error=nonsense`, plus the theme cycle with the network panel open.
   FR-PAGE-9, FR-PAGE-10, FR-A11Y-5 and the "left-aligned / `bg-background`"
   criterion have no automated backstop.
3. **Add a `captureTokenFromHash` test asserting an `#error=` fragment survives**,
   closing the untested half of FR-STATE-8 (P2 / frontend F4).
4. **Tick the plan's checkboxes** and mark Task 7 Steps 4–5 explicitly as
   outstanding, so they are distinguishable from unstarted work (P3).
