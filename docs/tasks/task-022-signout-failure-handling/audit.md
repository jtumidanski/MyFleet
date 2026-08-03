# Frontend Audit — task-022-signout-failure-handling

## Frontend guidelines

- **Audit Scope:** Frontend commits `6805858`, `c9bd3fc`, `ad7cae5` on branch `task-022-signout-failure-handling` (Go commit `08172bd` excluded):
  - `apps/web/src/lib/hooks/api/auth.ts` + `auth.test.ts`
  - `apps/web/src/context/AuthContext.tsx` + new `AuthProvider.logout.test.tsx`
  - `apps/web/src/components/frame/ProfileMenu.tsx` + `ProfileMenu.test.tsx`
  - `apps/web/src/components/AppLayout.test.tsx`, `apps/web/src/components/admin/AdminLayout.test.tsx` (fixture-only)
- **Guidelines Source:** `.claude/skills/frontend-dev-guidelines/` (SKILL.md + all `resources/*.md`)
- **Date:** 2026-08-03
- **Build:** PASS
- **Tests:** 728 passed, 0 failed (90 files); targeted re-run of the 5 in-scope files: 46 passed, 0 failed
- **Overall:** PASS

## Build & Test Results

Verified directly rather than taken on faith:

```
$ npm run -w apps/web build
> tsc -b && vite build
✓ 1934 modules transformed.
✓ built in 3.94s
```

```
$ cd apps/web && npm test -- --run
 Test Files  90 passed (90)
      Tests  728 passed (728)
```

```
$ cd apps/web && npm test -- --run src/lib/hooks/api/auth.test.ts \
  src/context/AuthProvider.logout.test.tsx src/components/frame/ProfileMenu.test.tsx \
  src/components/AppLayout.test.tsx src/components/admin/AdminLayout.test.tsx
 Test Files  5 passed (5)
      Tests  46 passed (46)
```

Both numbers match the claim in the task brief (728/728, `tsc -b` clean under strict + `noUncheckedIndexedAccess`).

## File Inventory

- `apps/web/src/lib/hooks/api/auth.ts` — Hook (React Query hooks / auth transport functions)
- `apps/web/src/lib/hooks/api/auth.test.ts` — Test (hook)
- `apps/web/src/context/AuthContext.tsx` — Other (React context provider)
- `apps/web/src/context/AuthProvider.logout.test.tsx` — Test (context), new file
- `apps/web/src/components/frame/ProfileMenu.tsx` — Component (feature/frame)
- `apps/web/src/components/frame/ProfileMenu.test.tsx` — Test (component)
- `apps/web/src/components/AppLayout.test.tsx` — Test (component), one-line fixture repair
- `apps/web/src/components/admin/AdminLayout.test.tsx` — Test (component), one-line fixture repair

No `types/models/`, `lib/schemas/`, or `services/api/` files are in scope, so the JSON:API model (FE-10), schema (FE-14), and BaseService (FE-11) checklist rows are marked N/A rather than PASS/FAIL — there is nothing in the diff to check them against.

## Anti-Pattern Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-01 | No `any` type | PASS | `git diff d3c9eaa..ad7cae5 -- apps/web` grepped for `: any` / `as any` across all 8 changed files — zero matches. |
| FE-02 | No manual class concatenation | PASS | No `className` touched by this diff at all; `ProfileMenu.tsx` gains only a `.catch()` handler and an import block. |
| FE-03 | No direct API client calls in components | PASS | `ProfileMenu.tsx:4` imports only `createErrorFromUnknown` from `@myfleet/shared-ts`, not `apiClient`; the actual request lives in the hook file (`auth.ts:80-85`), consistent with the file's pre-existing pattern (`fetchMe`, `updateThemePreference` in the same file already call `apiClient.request` directly — this is the established hook-layer pattern for `auth.ts`, not a new violation). |
| FE-04 | No inline Zod schemas in components | PASS | No `z.object(`/`z.string(` etc. anywhere in the diff. |
| FE-05 | No spinners for content loading | PASS | Zero `animate-spin` matches in the diff; no loading UI added. |
| FE-06 | No hardcoded colors | PASS | Zero matches for `bg-white|bg-black|bg-gray-\d|bg-red-\d` etc. in the diff. |
| FE-07 | No state mutation | PASS | Zero `.push(`/`.splice(`/`.sort(` in the diff. `AuthContext.tsx:72-80` and `ProfileMenu.tsx:52-59` are the only new logic, both effect/callback bodies with no array mutation. |
| FE-08 | No default exports for components | PASS | Zero `export default function` matches; `ProfileMenu.tsx` keeps its existing named `export function ProfileMenu()` (line 28, unchanged by diff). |
| FE-09 | Error handling with `createErrorFromUnknown` | PASS | `ProfileMenu.tsx:52-59` — `logout().catch((err: unknown) => { const apiError = createErrorFromUnknown(err); toast.error(...) })`. This is the only new `.catch(` in the diff and it both classifies the error and surfaces it via `toast.error`, satisfying the pattern. `AuthContext.tsx:72-79` deliberately does **not** catch — it lets the rejection propagate via `finally`'s implicit rethrow, which is the documented contract (interface doc at `AuthContext.tsx:25-30`) and is exercised by `AuthProvider.logout.test.tsx:59-74`. |

## Architecture Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-10 | JSON:API model shape | N/A | No `types/models/` files in scope. |
| FE-11 | Service extends `BaseService` | N/A | No `services/api/` files in scope; `auth.ts`'s direct-`apiClient` pattern for `logoutRequest` (lines 80-85) matches the file's pre-existing sibling functions `fetchMe` (line 36) and `updateThemePreference` (lines 103-106), so no new architectural layer is introduced. |
| FE-12 | Query key factory uses `as const` | PASS | `auth.ts:7-10` — `export const authKeys = { all: ['auth'] as const, me: () => ['auth', 'me'] as const };`. Untouched by this diff, but `logoutRequest` correctly does not introduce its own ad hoc key and `AuthContext.tsx:78` reuses `authKeys.all` for invalidation, both before and after this change. |
| FE-13 | Forms use `react-hook-form` + `zodResolver` | N/A | No form components touched. |
| FE-14 | Schema in `lib/schemas/` with inferred type | N/A | No schema files touched. |

## Styling Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-15 | Interactive elements show `cursor-pointer` | PASS | `ProfileMenu.tsx:91` — `<DropdownMenuItem onSelect={handleSignOut}>Sign out</DropdownMenuItem>` is unchanged by this diff (only the `onSelect` handler's *implementation* changed, not the element or its class list). The base component already bakes in the affordance: `apps/web/src/components/ui/dropdown-menu.tsx:72` — `'relative flex cursor-pointer select-none items-center ...'` in `DropdownMenuItem`'s class list. No new custom clickable surface was introduced by this branch, so there is nothing new to require `cursor-pointer` on. |

## Testing Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-16 | Tests exist for changed components | PASS | `auth.ts` → `auth.test.ts:145-186` (`describe('logoutRequest', ...)`, 3 cases). `AuthContext.tsx` → new `AuthProvider.logout.test.tsx` (2 cases, lines 59-93). `ProfileMenu.tsx` → `ProfileMenu.test.tsx:139-158` (2 new cases) plus 9 pre-existing cases re-verified. `AppLayout.test.tsx`/`AdminLayout.test.tsx` fixture repairs verified to still pass (`AppLayout.test.tsx:106-112`, `AdminLayout.test.tsx:142-150`). |
| FE-17 | Mocks updated when services changed | PASS | No `services/api/` or `__mocks__/` module changed, so nothing there needed updating. The four `logout: vi.fn()` fixture repairs required by the new `Promise`-returning `logout()` signature are all present: `ProfileMenu.test.tsx:38` (`renderMenu` default) and `:79` (local override), `AppLayout.test.tsx:106`, `AdminLayout.test.tsx:142` — all changed from bare `vi.fn()` to `vi.fn().mockResolvedValue(undefined)`. |

## Disagreements on record (not defects)

Per the audit brief, these two decisions were adjudicated during planning and are not raised as blocking findings, though noting them for the record:

- **Fixed toast copy instead of the house `toast.error(apiError.message || fallback)` pattern** (`ProfileMenu.tsx:55-57`). Reasoning verified against source: `packages/shared-ts/src/errors.ts:23-37` (`createErrorFromUnknown`) constructs the `ApiError.message` from the JSON:API error's `title`, and the plan's claim that `server.WriteError` redacts every 5xx `title` to a fixed `InternalErrorTitle` was not independently re-verified against the Go source in this frontend-only audit (out of scope), but is consistent with the test fixture at `auth.test.ts:159-166` (`title: 'internal server error'`) and with `ProfileMenu.test.tsx:133-138`'s comment. Given that, the fixed-copy choice is defensible for this one path; I have no independent disagreement to add.
- **`logout()` rejects rather than resolving with a `{ ok, error }` result object.** Consistent with the documented `AuthContextValue.logout: () => Promise<void>` contract (`AuthContext.tsx:30`) and exercised end-to-end by `AuthProvider.logout.test.tsx`. No issue found.

## Known minors triaged

1. `logoutRequest`'s JSDoc (`auth.ts:63-79`, 17 lines) is heavy relative to its 5-line body (`auth.ts:80-85`). Confirmed present, purely stylistic, **non-blocking** — the comment documents a non-obvious interaction (inheriting the one-shot 401 refresh-and-retry on a route with no JWT middleware) that would otherwise not be discoverable from the code alone. No guideline requires comment-to-code ratio limits. Does not need to be fixed before merge.
2. `ProfileMenu.test.tsx:157` (`await Promise.resolve();` in `'raises no toast when sign-out succeeds'`) flushes only one microtask rather than a full macrotask (`setTimeout(r, 0)`). Confirmed present at line 157. This is the form the plan mandates verbatim (plan.md Task 4, Step 2) and the test passes deterministically in the full and targeted runs above. **Non-blocking** — no FE-* check requires a stronger flush, and changing it would deviate from the approved plan without a demonstrated flake.

## Summary

### Blocking (must fix)
- None. Zero FAIL rows across all three checklists.

### Non-Blocking (should fix)
- (Already on record, triaged above, no action required for merge) `auth.ts:63-79` JSDoc weight; `ProfileMenu.test.tsx:157` single-microtask flush.

## Backend guidelines

- **Service:** `apps/auth-service`
- **Audit Scope:** Go commit `08172bd` only, three files (per audit brief; all other Go files in `session/` are untouched by this branch and are noted only where needed to evaluate the diff):
  - `apps/auth-service/internal/session/resource.go` — `logoutHandler`
  - `apps/auth-service/internal/session/resource_test.go` — four new `TestLogout_*` cases, widened `newRefreshRouter`
  - `apps/auth-service/internal/session/processor_test.go` — `fakeStore.revokeErr` injection point
- **Guidelines Source:** `.claude/skills/backend-dev-guidelines/resources/*.md`
- **Date:** 2026-08-03
- **Build:** PASS
- **Tests:** Package re-run independently below; full-branch numbers per task brief are also cited.
- **Overall:** PASS

### Build & Test Results

Run directly, not taken on faith:

```
$ cd apps/auth-service && go build ./...
(no output — success)

$ cd apps/auth-service && go test ./... -count=1
ok  	github.com/jtumidanski/myfleet/apps/auth-service/cmd	0.029s
ok  	github.com/jtumidanski/myfleet/apps/auth-service/internal/arch	0.003s
ok  	github.com/jtumidanski/myfleet/apps/auth-service/internal/jwks	0.071s
ok  	github.com/jtumidanski/myfleet/apps/auth-service/internal/membership	0.026s
ok  	github.com/jtumidanski/myfleet/apps/auth-service/internal/oidc	0.008s
ok  	github.com/jtumidanski/myfleet/apps/auth-service/internal/platformadmin	0.009s
ok  	github.com/jtumidanski/myfleet/apps/auth-service/internal/session	0.628s
ok  	github.com/jtumidanski/myfleet/apps/auth-service/internal/user	0.024s

$ cd apps/auth-service && go test ./internal/session/... -race -count=1 -v
--- PASS: TestMintAccess_setsRequiredClaims (0.04s)
--- PASS: TestHashRefresh_isStableAndNotPlaintext (0.00s)
--- PASS: TestRotate_happyPath (0.06s)
--- PASS: TestRotate_reuseDetection (0.04s)
--- PASS: TestLogout_revokesFamily (0.05s)
--- PASS: TestMintAccess_mapsEveryPrincipalField (0.03s)
--- PASS: TestMintAccess_setsPlatformAdminClaimAsBoolean (0.05s)
--- PASS: TestRefresh_mintsAccessTokenCarryingEmailClaim (0.07s)
--- PASS: TestRefresh_failsClosedAndClearsCookieWhenTheResolverErrors (0.07s)
--- PASS: TestLogout_returns500WhenTheFamilyRevokeFails (0.10s)
--- PASS: TestLogout_succeedsAndClearsCookie (0.11s)
--- PASS: TestLogout_returns204WithNoRefreshToken (0.11s)
--- PASS: TestLogout_returns204ForAnUnknownToken (0.04s)
PASS
ok  	github.com/jtumidanski/myfleet/apps/auth-service/internal/session	1.794s
```

### Domain Classification

`session` has `model.go` (`apps/auth-service/internal/session/model.go`) → domain package, full DOM checklist applies. Only the write path exercised by `logoutHandler` is in scope for this diff; other DOM rows for this package (builder, entity, rest, provider — all untouched by commit `08172bd`) are marked N/A rather than re-derived from memory, per the "verify, don't assume" rule.

### Domain Checklist Results

#### `session` (in-scope rows only — see Domain Classification)

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| DOM-06 | Processor/handler-injected logger is `logrus.FieldLogger`, not `*logrus.Logger` | PASS | `resource.go:85` — `func logoutHandler(log logrus.FieldLogger, proc *Processor, cookieSecure bool) http.HandlerFunc`. Interface type, not concrete. |
| DOM-07 | Handlers log with a request-scoped/service logger, not `logrus.StandardLogger()` | PASS (N/A framework note) | This codebase has no `HandlerDependency`/`d.Logger()` abstraction (confirmed: `grep -n "HandlerDependency" packages/shared-go/server/*.go` → no match), so the DOM-07 criterion is evaluated as: is the logger injected once at startup and threaded through, with zero `logrus.StandardLogger()` calls? `apps/auth-service/cmd/main.go:27` — `log := telemetry.NewLogger()`; `cmd/main.go:128` — `session.InitializePublicRoutes(log, sess, resolve, cookieSecure)`; `resource.go:94` — `log.WithError(err).Error("logout revoke family")` uses that same injected logger. `grep -n "logrus.StandardLogger" apps/auth-service/internal/session/resource.go` → no match. |
| DOM-09 | Transform errors checked (not discarded with `_`) | N/A | `resource.go`'s only `Transform(` call (`rest.go` usage at line 80) is untouched by commit `08172bd` — not in scope for this diff. Not re-audited. |
| DOM-11 | No `os.Getenv()` in `resource.go` | PASS | `grep -n "os.Getenv" apps/auth-service/internal/session/resource.go` → zero matches. |
| DOM-12 | No cross-domain business logic in handlers | PASS | `logoutHandler` (`resource.go:85-114`) calls only `proc.Logout(raw)` (session's own processor) and two session-local helpers (`clearRefreshCookie`, `readRefreshToken`). No calls into another domain's package. |
| DOM-13 | Handlers don't call providers directly | PASS | `grep -n "provider\.\|Provider(" apps/auth-service/internal/session/resource.go` → zero matches. `logoutHandler` calls `proc.Logout`, a processor method. |
| DOM-14 | No direct entity creation/write in handlers | PASS | `grep -n "db\.\(Create\|Save\|Delete\)" apps/auth-service/internal/session/resource.go` → zero matches. |
| DOM-16 | Domain error → HTTP status mapping | PASS | `processor.go:136-145` — `Logout` collapses `ErrNotFound` to `nil` (line 139-140) and returns any other error (including a `RevokeFamily` failure) unwrapped (line 142/144). `resource.go:107` passes that raw error straight to `server.WriteError`, which maps it via `StatusFor` (`packages/shared-go/server/errors.go:19-41`) — `session.ErrNotFound` is not `server.ErrNotFound` (different sentinel), so it cannot accidentally map to 404, and any non-sentinel error (e.g. the injected `"db down"`) falls to the `default: return 500` case (`errors.go:39-40`). Confirmed by the passing `TestLogout_returns500WhenTheFamilyRevokeFails`. |
| DOM-19 | Table-driven tests (`tests := []struct{...}` + `t.Run`) | WARN (non-blocking) | The four new tests (`resource_test.go:173` `TestLogout_returns500WhenTheFamilyRevokeFails`, `:216` `TestLogout_succeedsAndClearsCookie`, `:238` `TestLogout_returns204WithNoRefreshToken`, `:257` `TestLogout_returns204ForAnUnknownToken`) are four separate `func Test...(t *testing.T)` functions, not a `tests := []struct{...}` table with `t.Run`. This is a real deviation from the DOM-19 pass criterion. Mitigating: it matches this exact file's pre-existing convention — every test already in `session/processor_test.go` and `session/resource_test.go` before this diff (`TestMintAccess_setsRequiredClaims`, `TestRotate_happyPath`, `TestRotate_reuseDetection`, `TestRefresh_mintsAccessTokenCarryingEmailClaim`, etc.) is likewise a standalone function, whereas `apps/auth-service/internal/user/{theme_test.go:6,resource_test.go:173,entity_test.go:9}` do use the table-driven form — so the `session` package is the pre-existing outlier, not something this diff newly broke. Not raised as blocking. |

### Sub-Domain Checklist Results

Not applicable — `session` is a domain package (has `model.go`), not an action-event sub-domain package. No SUB-* rows apply to this diff.

### Security Review

The auth-service handles token issuance/revocation, so Phase 4 applies.

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| SEC-01 | JWT validation uses verified parsing | N/A (out of scope) | `logoutHandler` never parses a JWT — it looks up the refresh token by SHA-256 hash (`processor.go:137` — `p.p.FindByHash(HashRefresh(raw))`), not by decoding claims. `grep -n "ParseUnverified" apps/auth-service/internal/session/resource.go` → zero matches. The access-token `ParseWithClaims` call in `refreshHandler` (untouched by commit `08172bd`) is out of this diff's scope. |
| SEC-02 | Token revocation checks validated tokens, doesn't extract claims from unvalidated tokens | PASS | `logoutHandler` (`resource.go:87`) reads the raw cookie/body value only (`readRefreshToken`) and never decodes it as a JWT; revocation is keyed off `HashRefresh(raw)` (`processor.go:137`) against the stored hash, not off self-asserted claims. |
| SEC-03 | No open redirect | N/A | No redirect logic in any of the three changed files. |
| SEC-04 | Secrets not hardcoded | PASS | `grep -niE "secret|password|apikey"` over all three changed files → only matches are the string literal `"secret-token"` used as a test fixture value in `processor_test.go:106-107` (`TestHashRefresh_isStableAndNotPlaintext`), not a credential. |

**Project-specific check — "SEC-09" (error text disclosure), cited in the task brief and in `resource.go:100-106` / `resource_test.go:201`:** this identifier is not one of the four SEC-* rows above; it is this codebase's own established finding from `docs/tasks/task-009-smtp-invite-delivery/audit-backend.md:88` (a prior audit that found raw GORM/driver error text reaching clients through `server.WriteError`). Verified independently against source, not taken from the code comment's claim:

- `packages/shared-go/server/jsonapi.go:97-101` — `apiErr := APIError{..., Title: InternalErrorTitle}` sets the redacted title unconditionally as the default.
- `packages/shared-go/server/jsonapi.go:106-111` — the raw `err.Error()` only overwrites `apiErr.Title` when `status < 500` (`if status < 500 { apiErr.Title = err.Error() ... }`). For the store's injected `"db down"` error, `StatusFor` (not a recognized sentinel) returns 500, so this branch never executes and the redacted title stands.
- `resource_test.go:203-204` asserts `body.Errors[0].Title == server.InternalErrorTitle`, and this assertion passes (see test run above) — the store's raw error text does not reach the response body.
- **Empirically verified, not just read:** I copied the worktree to a scratch directory, swapped `server.WriteError(w, err)` to run before `clearRefreshCookie(w, cookieSecure)` (the deliberately-wrong order) in that copy only, and re-ran `TestLogout_returns500WhenTheFamilyRevokeFails`. Result: `resource_test.go:212: the refresh cookie must still be cleared on the failure path; cookies = []` — **FAIL**, confirming the test genuinely pins the ordering requirement rather than passing vacuously. This also confirms the mechanism cited in the task brief: `net/http/httptest.ResponseRecorder.WriteHeader` snapshots `HeaderMap` into `snapHeader` at the moment `WriteHeader` is called (`$GOROOT/src/net/http/httptest/recorder.go`, `WriteHeader` method), and `Result()` returns that snapshot — so a `Set-Cookie` added via `http.SetCookie` **after** `WriteError`'s call to `WriteJSON`→`w.WriteHeader` is silently absent from the recorded response, exactly as claimed. The scratch copy was discarded after the experiment; no worktree file was modified.
- **Ordering in the actual (unmodified) file:** `resource.go:99` `clearRefreshCookie(w, cookieSecure)` runs before `resource.go:107` `server.WriteError(w, err)` — correct order, confirmed both by direct reading and by the above experiment.

**Verdict: SEC-09 — PASS.** No internal error text reaches the client on the logout-failure path, and the cookie-clearing/WriteError ordering that this protection depends on is correct and test-pinned.

### Disagreements on record (not defects)

Per the audit brief, one decision was adjudicated during planning and is not raised as a defect: the revoke failure is logged twice — once in `logoutHandler` (`resource.go:94`, `log.WithError(err).Error("logout revoke family")`, names the operation) and once inside `server.WriteError` for any 5xx (`jsonapi.go:102-104`, `errorLogger().WithError(err).WithField("status", status).Error(...)`, knows only the status). This is redundant but not incorrect, and the documented rationale (an operator grepping for "logout" should find a line) is reasonable. Noting it, not blocking on it.

### Summary

#### Blocking (must fix)
- None. Zero FAIL rows.

#### Non-Blocking (should fix)
- DOM-19: the four new `TestLogout_*` functions in `resource_test.go` are not table-driven (`tests := []struct{...}` + `t.Run`), matching this package's pre-existing non-table-driven convention but diverging from the org-wide-preferred pattern used in sibling package `internal/user`. Not required for merge.

---

# Plan Audit — task-022-signout-failure-handling

## Plan adherence

**Plan Path:** `docs/tasks/task-022-signout-failure-handling/plan.md`
**Audit Date:** 2026-08-03
**Branch:** `task-022-signout-failure-handling` (tip `ad7cae5`)
**Base Branch:** `main` (merge-base `d3c9eaa`)
**Scope of this section:** whole-branch, cross-task. Tasks 1–4 were each reviewed
individually and came back clean; this pass audits the seams between them —
whether a failed `RevokeFamily` in auth-service actually reaches a toast in
`ProfileMenu`, and whether every row of `context.md`'s behaviour matrix holds.

### Executive Summary

All four code tasks landed, and they landed as written — the implementation is a
near-verbatim match to the plan's code blocks, with no widening and nothing
deferred. The end-to-end failure path was traced through all six layers
(handler → `WriteError` → `apiClient` → `logoutRequest` → `AuthContext.logout` →
`ProfileMenu` → `<Toaster>`) and it works: the toast is reachable and visible
after the sign-out redirect. All six rows of the behaviour matrix hold. Two
adjudicated decisions were re-verified against source and are confirmed correct,
not merely accepted. Two genuine gaps remain, both non-blocking: the plan's own
Task 5 Step 2 verification grep does not produce its stated expected output, and
all 30 plan checkboxes are still unchecked.

### Task Completion

| # | Task / Step | Status | Evidence |
|---|---|---|---|
| 1.1 | `fakeStore.revokeErr`, call recorded before the failure | DONE | `processor_test.go:24`, `:60-68` |
| 1.2 | `newRefreshRouter` widened to return the store; 5xx logger quieted | DONE | `resource_test.go:20-49` (`server.SetErrorLogger(log)` + `t.Cleanup`, `:39-41`); call sites `:76`, `:122` |
| 1.3 | `postLogout` + `unusedResolver` helpers | DONE | `resource_test.go:151-165` |
| 1.4 | Four logout HTTP tests | DONE | `resource_test.go:167-283` |
| 1.5 | Red-first run | DONE (inferred) | Not re-observable post-hoc. The diff at `08172bd` shows the pre-change handler returning `204` unconditionally, so `TestLogout_returns500WhenTheFamilyRevokeFails` could only have failed on its status assertion. |
| 1.6 | `logoutHandler` — log, clear cookie, then `WriteError` | DONE | `resource.go:88-110`; cookie at `:99` precedes `WriteError` at `:107` (FR-SRV-3) |
| 1.7 | Package tests green | DONE | `go test -race -run 'TestLogout\|TestRefresh' .../session -v` — 7/7 PASS, incl. pre-existing `TestLogout_revokesFamily` |
| 1.8 | Vet + commit | DONE | commit `08172bd` |
| 2.1 | `logoutRequest` transport tests | DONE | `auth.test.ts:146-205` (3 cases) |
| 2.2 | Red-first run | DONE (inferred) | Pre-change body was `fetch(...).catch(() => undefined)` (diff at `6805858`); both rejection tests could only have failed. |
| 2.3 | Delegate to `apiClient` | DONE | `auth.ts:80-85`; `credentials: 'include'` preserved (`:83`) |
| 2.4/2.5 | Green + commit | DONE | commit `6805858`; targeted run 8/8 PASS |
| 3.1 | `AuthProvider.logout.test.tsx` created | DONE | new file, 96 lines, real provider + `useMe` stubbed non-error (`:21-25`) |
| 3.2 | Red-first run | DONE (inferred) | Pre-change teardown ran after the `await` (diff at `c9bd3fc`); the `localStorage` assertion could only have failed. |
| 3.3 | Teardown into `finally`; interface JSDoc | DONE | `AuthContext.tsx:72-80`; interface doc `:25-29` |
| 3.4/3.5 | Green + commit | DONE | commit `c9bd3fc`; 2/2 PASS |
| 4.1 | Four `logout` fixtures return a promise | DONE | `ProfileMenu.test.tsx:38`, `:79`; `AppLayout.test.tsx:106`; `AdminLayout.test.tsx:142` |
| 4.2 | Toast-on-failure / no-toast-on-success tests | DONE | `ProfileMenu.test.tsx:133-160`; `vi.mock('sonner', …)` `:13`; `beforeEach` reset `:47` |
| 4.3 | Red-first run | DONE (inferred) | Pre-change item was `onSelect={() => void logout()}` (diff at `ad7cae5`); `toast.error` was unreachable. |
| 4.4 | `handleSignOut` + menu item rewired | DONE | `ProfileMenu.tsx:52-59`, `:91`. Copy is the plan's exact string (`:55`). |
| 4.5/4.6 | Green + commit | DONE | commit `ad7cae5`; targeted 5-file run 46/46 PASS, no unhandled-rejection report |
| 5.1 | `make ci` | DONE | Per the audit brief: clean on first attempt (728/728 fe-test, `tsc -b`, manifests). Corroborated by my targeted re-runs. |
| 5.2 | Acceptance-criteria walk + three greps | **PARTIAL** | Greps 2 and 3 return no matches as specified. **Grep 1 returns a match** — see Finding 1. |
| 5.3 | Commit gate fixes | NOT_APPLICABLE | Gate was clean; the step is an explicit no-op. |
| 6.1 | Request the review | DONE | This document. |
| 6.2 | Act on findings | PENDING | Outstanding by construction — the review is what produces them. |
| 6.3 | Re-run gate + commit | PENDING | Same. |

**Completion Rate:** 27/30 steps DONE, 1 PARTIAL, 1 NOT_APPLICABLE, 2 PENDING (Task 6, in flight).
**Skipped without approval:** 0
**Partial implementations:** 1 (Task 5 Step 2)
**Widened scope:** 0 — every "out of scope" item in plan §Out of scope was respected. No retry/backoff, no `skipRefresh` on `ApiClient` (`packages/shared-ts/src/apiClient.ts` is unchanged on this branch), no session-management UI, no toast "Try again" action, no other `void somePromise()` call sites touched.

### Cross-task verification (the part the per-task reviews could not do)

Each of these was traced against source rather than inferred from the plan:

1. **The end-to-end failure path works.** `logoutHandler` 500 → `WriteError`
   envelope → `apiClient.request` throws at `apiClient.ts:50` (`!res.ok`) →
   `logoutRequest` propagates → `AuthContext.logout`'s `finally` tears down and
   implicitly rethrows (`AuthContext.tsx:72-80`) → `ProfileMenu`'s `.catch`
   (`ProfileMenu.tsx:53`) → `toast.error`. No layer re-swallows.
2. **The toast can actually be seen.** This is the failure mode a mocked-`sonner`
   unit test cannot catch. `<ThemedToaster />` is rendered in `AppProviders.tsx:51`
   as a **sibling of `{children}`**, above the router — so it survives the
   sign-out unmount. `RequireAuth` redirects with react-router `<Navigate>`
   (`RequireAuth.tsx:43`), a client-side transition, not a page load. `LoginPage`
   does **not** auto-navigate to the IdP — it waits for a button click
   (`LoginPage.tsx:91-97`). The toast therefore renders on `/login` and stays.
   The claim in `ProfileMenu.tsx:48-51` is accurate.
3. **Moving onto `apiClient` cannot break logout.** `apiClient` now attaches
   `Authorization: Bearer …` (`apiClient.ts:31`) where the bare `fetch` did not.
   Verified inert: `/auth/logout` is mounted **outside** the JWT group in
   `apps/auth-service/cmd/main.go:128` (the `authmw.JWT` middleware is confined to
   the group at `:134-143`), and the gateway applies only `auth-stripprefix` to
   `/api/auth` (`deploy/k8s/overlays/main/ingressroute.yaml:169-176`,
   `deploy/k8s/infra-local/ingressroute.yaml:83-90`). No 401 is producible, so the
   one-shot refresh-and-retry is genuinely unreachable, as the JSDoc claims.
4. **OQ3 holds — `ProfileMenu` is still the only caller of `logout()`.**
   `grep -rn "logout(" apps/web/src --include='*.tsx' --include='*.ts'`, excluding
   tests, returns only `ProfileMenu.tsx:53`. No other site can strand an
   unhandled rejection now that `logout()` can reject.
5. **Trap #2's fixture list was complete.** Of the twelve remaining
   `logout: vi.fn()` fixtures, none belongs to a suite that activates "Sign out":
   `FrameHeader.test.tsx:92-95` only asserts the standalone button's *absence*,
   `dropdown-menu.test.tsx` renders its own literal item with no handler, and
   `ThemeSync.test.tsx:121` is a comment. No latent
   `Cannot read properties of undefined (reading 'catch')`.
6. **FR-LOGOUT-14 holds.** `grep -rn "toast" AppLayout.tsx admin/AdminLayout.tsx`
   → no matches; the fix exists once.

### Behaviour matrix — every row checked

| Scenario | Verified | Evidence |
|---|---|---|
| Revoke succeeds → 204, cleared, purged, no toast | YES | `TestLogout_succeedsAndClearsCookie`; `posts with credentials and treats 204 as success…`; `clears the local session and resolves…`; `raises no toast when sign-out succeeds` |
| No refresh cookie → 204, no revoke attempted | YES | `TestLogout_returns204WithNoRefreshToken` (`revokeErr` armed, `revokedFamilies` empty) |
| Unknown/expired token → 204 | YES | `TestLogout_returns204ForAnUnknownToken`; `processor.go:136-144` maps `ErrNotFound` → nil |
| `RevokeFamily` errors → 500, cleared, purged, toast | YES | `TestLogout_returns500WhenTheFamilyRevokeFails` + `rejects with an ApiError…` + `clears the local session and still rejects…` + `warns that the server session may survive…` |
| Gateway 5xx / service down → toast | BY CONSTRUCTION | No dedicated test, and none was planned. Shares the `!res.ok` branch at `apiClient.ts:50` with the tested 500 case. A non-JSON body (HTML error page) degrades safely: `res.json().catch(() => null)` at `:49` → `createErrorFromUnknown({status, body: null})` → `ApiError(0, 'unknown', 'Unknown error')` → still rejects → still toasts. |
| Browser offline → toast | YES | `rejects when the network request itself fails` + `clears the local session and still rejects…` + the `ProfileMenu` failure test |

"Local token cleared = yes" holds in every row: the `finally` at
`AuthContext.tsx:75-79` has no early exit.

### Findings

**Finding 1 (minor, non-blocking) — the plan's own Task 5 Step 2 grep fails as written.**
Plan line 886 specifies:

```sh
grep -n "catch(() => undefined)\|fetch('/api/auth/logout'" apps/web/src/lib/hooks/api/auth.ts   # expect: no matches
```

Actual output:

```
70: * bare fetch terminated by `.catch(() => undefined)`, never reading `ok` or
```

The match is inside `logoutRequest`'s JSDoc, which narrates the removed
behaviour verbatim. The underlying requirement (FR-LOGOUT-2 — no `.catch` that
converts a failure into a resolution) **is** satisfied: the function body is
`auth.ts:80-85` and contains no `.catch`. Only the plan-specified *check* is red.
This is the same root cause as SDD-ledger minor #1 (JSDoc weight), but it has a
concrete consequence the ledger entry did not state: a documented acceptance gate
does not pass, so anyone re-running the plan's verification will hit a false
positive. **Triage: fix-optional, not merge-blocking.** Cheapest fix is to strip
the code punctuation from the prose (e.g. "a bare fetch whose catch handler
resolved to undefined"), which clears the grep without losing the history. Do
*not* delete the paragraph — it explains why the rewrite happened.

**Finding 2 (note, no action) — `apiError.detail` on the toast is dead by construction.**
`ProfileMenu.tsx:56` passes `description: apiError.detail`, and the inline comment
at `:44-45` claims "its `detail` carries anything a gateway or transport error
supplied". That claim does not hold, for two independent reasons:

- `createErrorFromUnknown` (`packages/shared-ts/src/errors.ts:23-36`) is being
  handed an **already-constructed `ApiError`**, not a `{status, body}` envelope.
  An `ApiError` has no `.body`, so `first` is undefined, control falls to
  `e instanceof Error`, and the function returns a *fresh*
  `ApiError(0, 'unknown', e.message)` — discarding `status`, `code`, `detail` and
  `pointer` from the original. Any `detail` the server sent is dropped here.
- Independently, `server.WriteError` sets `Detail` only when `status < 500`
  (`packages/shared-go/server/jsonapi.go:106-112`), and 500 is the only status
  this path produces — so there is no detail to drop in the first place.

Net effect: `description` is `undefined` in all six matrix rows, sonner omits it,
and the user sees exactly the fixed copy. **The behaviour is correct and matches
the plan's intent**; the call is simply vestigial and the comment overstates it.
Worth knowing because it silently defeats the obvious future change ("make the
server send a detail and it will show up") — it will not, until the re-wrap is
removed. The `createErrorFromUnknown` re-wrap is pre-existing house style
(`FleetNameForm.tsx` does the same), so this is not a regression introduced here
and is out of scope for this task.

**Finding 3 (hygiene, should fix before the PR) — no plan checkboxes were ticked.**
All 30 `- [ ]` items in `plan.md` remain unchecked
(`grep -c '^- \[x\]' plan.md` → 0), despite Tasks 1–5 being complete. Prior tasks
on this repo close with a "mark the plan complete" commit (e.g. `36bf270` for
task-017). Purely documentary, but it makes the committed plan misrepresent the
branch state to anyone reading it after merge.

### Adjudicated items — re-verified, not merely accepted

Both were checked against Go source (which the frontend-only audit above
explicitly could not do) and both are **confirmed correct**:

- **Fixed toast copy.** `server.WriteError` sets
  `Title: InternalErrorTitle` unconditionally and only overwrites it for
  `status < 500` (`jsonapi.go:97-112`); `InternalErrorTitle` is the literal
  `"internal server error"` (`:34`). `createErrorFromUnknown` maps `title` →
  `message` (`errors.ts:31`). So on this path `apiError.message` is exactly
  `"internal server error"` — non-empty, hence
  `apiError.message || 'fallback'` would render that string and the fallback
  would be unreachable. The plan's reasoning is sound; the house pattern would
  be a genuine regression here. `ProfileMenu.test.tsx:141` (`/server may still/`)
  guards it.
- **Double logging.** `resource.go:94` (`log.WithError(err).Error("logout revoke family")`)
  plus `jsonapi.go:103-104` (`errorLogger()…Error("request failed; error text redacted…")`).
  Confirmed deliberate and confirmed useful: the second line carries only the
  status, so without the first, `"logout"` is ungreppable in the logs. Not a defect.

### Known minors — triage

1. **`logoutRequest` JSDoc weight.** Non-blocking as a style matter (the frontend
   audit reached the same conclusion). But see **Finding 1** — the same JSDoc
   breaks a plan-specified acceptance grep, which is a slightly stronger reason to
   reword one clause. Still not merge-blocking.
2. **`await Promise.resolve()` in `raises no toast when sign-out succeeds`.**
   **Not a defect here, and no change recommended.** The reviewer's general point
   is right, but it does not apply to this test: `renderMenu` injects the mock
   *directly* as the context's `logout`
   (`ProfileMenu.test.tsx:153`), so the chain from the click to any possible
   `toast.error` is exactly one `.catch` deep — there is no deeper chain for a
   single tick to miss. Moreover `await userEvents.click(...)` on the preceding
   line already flushes the queue, so the `await Promise.resolve()` is belt-and-braces
   rather than load-bearing. The realistic regression it must catch (an
   unconditional `toast.error` in `handleSignOut`) is synchronous and would be
   caught with no flush at all. Leave the plan-mandated form. *Caveat for a future
   editor:* if this fixture is ever rewired to go through the real `AuthProvider`,
   the single tick becomes insufficient and must be upgraded.

### Build & Test Results

| Component | Build | Tests | Vet | Notes |
|---|---|---|---|---|
| `apps/auth-service` | PASS | PASS | PASS | `go test -race -run 'TestLogout\|TestRefresh' .../internal/session -v` → 7/7 PASS |
| `apps/web` | PASS | PASS | n/a | Targeted 5-file run: 46/46 PASS, no unhandled-rejection reports. Full suite 728/728 per `make ci` and the frontend audit above. |
| `packages/shared-ts`, `packages/shared-go` | n/a | n/a | n/a | Unchanged on this branch — confirms the plan's "no shared-package changes". |
| `deploy/k8s` | n/a | n/a | n/a | No manifest changes; plan correctly waived `kustomize`/dry-run. |

Working tree carries one uncommitted file, `go.work.sum` (build side-effect, not
a source change).

### Overall Assessment

- **Plan Adherence:** FULL for Tasks 1–4 (verbatim, no drift, no widening);
  MOSTLY_COMPLETE overall (Task 5 Step 2's grep gate, plus unticked checkboxes).
- **Recommendation:** READY_TO_MERGE after the two documentary items below. No
  code defect found, and no behavioural gap between the plan and the branch.

### Action Items

1. *(optional, clears a red acceptance gate)* Reword the one clause in
   `apps/web/src/lib/hooks/api/auth.ts:70` that contains the literal
   `.catch(() => undefined)` so plan Task 5 Step 2's first grep returns no
   matches. Keep the explanation.
2. *(recommended)* Tick the 30 completed checkboxes in `plan.md` and commit, per
   this repo's convention.
3. *(no action, recorded for future editors)* `apiError.detail` at
   `ProfileMenu.tsx:56` is always `undefined`; see Finding 2 before assuming a
   server-supplied detail will ever reach that toast.
