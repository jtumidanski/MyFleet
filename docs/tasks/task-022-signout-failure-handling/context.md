# Sign-Out Failure Handling — Implementation Context

Companion to [`plan.md`](./plan.md). Everything an implementer needs that is not
in the plan's task steps: where the relevant code lives, what was already
decided and why, and which traps are known.

Worktree: `/home/tumidanski/source/MyFleet/.worktrees/task-022-signout-failure-handling`
Branch: `task-022-signout-failure-handling`
Issue: [#38](https://github.com/jtumidanski/MyFleet/issues/38)

## The defect in one paragraph

Signing out cannot fail as far as the user or any log reader can tell. Three
independent suppressions stack up along the path a failure would travel, so the
UI shows the identical sequence whether the server revoked the refresh-token
family or never received the request. The consequence is a session that is dead
in the UI and alive on the wire: the access token is cleared client-side, but the
refresh token lives in an `HttpOnly` cookie that only the server can invalidate.
Note that issue #38's stated mechanism (an unhandled rejection stranding the user
signed in) is **not** what happens — see PRD §2.1. The defect is the inverse:
failures are relabelled as successes before they ever start the journey.

## Files that matter

### Being changed

| File | Current state | Target |
|---|---|---|
| `apps/auth-service/internal/session/resource.go:85-96` | `logoutHandler` logs a revoke error and returns `204` anyway | clears the cookie, then `server.WriteError(w, err)` → `500` |
| `apps/web/src/lib/hooks/api/auth.ts:63-71` | bare `fetch` terminated by `.catch(() => undefined)`; never reads `ok`/`status` | `apiClient.request<void>` with `credentials: 'include'` |
| `apps/web/src/context/AuthContext.tsx:55-60` | `await logoutRequest()` then teardown — teardown skipped if the await throws | teardown in a `finally`, rejection still propagates |
| `apps/web/src/components/frame/ProfileMenu.tsx:64` | `onSelect={() => void logout()}` | `onSelect={handleSignOut}` with `.catch` → `toast.error` |

### Read-only, but load-bearing

- `packages/shared-ts/src/apiClient.ts:42-52` — `request` already special-cases
  `204` before parsing (`:49`), checks `res.ok`, and throws
  `createErrorFromUnknown({ status, body })` (`:50`). `credentials: 'include'`
  reaches `fetch` through the `...init` spread at `:27-33`. The client-side fix
  is mostly deletion because this already exists.
- `packages/shared-ts/src/errors.ts:3-37` — `ApiError` (exported from
  `@myfleet/shared-ts` via `src/index.ts`) and `createErrorFromUnknown`. `detail`
  is populated only when the envelope carries one.
- `packages/shared-go/server/errors.go:18-43` — `StatusFor` maps anything without
  a matching sentinel to `500`. `session.ErrNotFound` is the session package's
  own sentinel (`model.go`), **not** `server.ErrNotFound`, so it could not be
  mapped to a 404 by accident even if it reached `WriteError`.
- `packages/shared-go/server/jsonapi.go:95-116` — `WriteError`. For any status
  ≥ 500 it replaces the title with `InternalErrorTitle` (`"internal server
  error"`, `:34`) and logs the redacted text through `errorLogger()`. This is
  why the toast copy cannot use `apiError.message`.
- `apps/auth-service/internal/session/processor.go:136-145` — `Processor.Logout`
  collapses `ErrNotFound` to `nil`, which is what keeps an unknown token a `204`.
- `apps/web/src/components/features/settings/FleetNameForm.tsx:29-36` — the house
  `try/catch` + `createErrorFromUnknown` + `toast.error` pattern. Followed in
  shape; deliberately **not** followed in the `message || fallback` detail.

## Decisions already taken (do not relitigate)

| # | Decision | Where |
|---|---|---|
| D1 | On failure, clear the local session anyway and warn. Refusing to sign out locally leaves the session live in *both* places, precisely when the user said they are leaving. | PRD App. A |
| D2 | The backend is in scope. Fixing only the client would check a status that is constant by construction. | PRD App. A |
| D3 | Unit tests both sides, no Playwright. The assertions are control flow and status codes; jsdom's CSS blindness does not apply. | PRD App. A, design §6.5 |
| OQ1 | **No "Try again" action on the toast.** It would pull in state the feature does not need and a successful retry would want a confirmation toast, reintroducing what FR-LOGOUT-13 removes. | design §6.4 |
| OQ2 | **`logout()` rejects**, it does not return `{ ok, error }`. Matches every other call site's shape; a result object would churn the eight-plus `baseAuth()` test fixtures for no behavioural gain. Documented via JSDoc instead. | design §5.1 |
| OQ3 | **`ProfileMenu` is the only caller of `logout()`** — verified again during planning by grep over `apps/web/src`. The provider's 401 effect (`AuthContext.tsx:43-48`) calls `clearAccessToken()`/`setHasToken(false)` directly. | design §5.2 |
| — | Keep the handler's explicit `log.WithError(err).Error("logout revoke family")` even though `WriteError` also logs 5xx. The handler's line names the operation; `WriteError`'s only knows the status. | design §3.4 |
| — | No `skipRefresh` option on `ApiClient`. Logout inherits the one-shot 401 retry, which is unreachable (public route, only 204/500) and harmless if it fired (the retry re-sends the original raw token; rotation keeps the family id). | design §4.2 |

## Traps found while planning

1. **`clearRefreshCookie` before `WriteError`.** `http.SetCookie` appends a
   header; `WriteError` → `WriteJSON` → `WriteHeader` flushes them. Reversing
   the two lines produces a 500 with no `Set-Cookie`, and Go does not error on a
   late header write — it drops it silently. Pinned by an assertion in
   `TestLogout_returns500WhenTheFamilyRevokeFails`.
2. **Four `logout` test fixtures return `undefined`.** `ProfileMenu.test.tsx:36`
   and `:76`, `AppLayout.test.tsx:106`, `AdminLayout.test.tsx:142` are bare
   `vi.fn()`. The moment `ProfileMenu` calls `logout().catch(...)` they throw
   `TypeError: Cannot read properties of undefined (reading 'catch')`. The two
   layout tests genuinely click "Sign out", so they are not incidental. Plan
   Task 4 Step 1 fixes all four before the component changes.
3. **The toast copy will look "wrong" to a reviewer.** Applying the house
   `apiError.message || 'fallback'` pattern here is a regression that shows
   "internal server error" — `WriteError` redacts every 5xx title and 500 is the
   only status this path produces, so the fallback is unreachable. Guarded by
   the `/server may still/` assertion and an inline comment at the call site.
4. **`fakeStore` records the revoke call before returning the injected error.**
   Otherwise `revokedFamilies` becomes a success log and a test cannot tell
   "never called" from "called and failed" — which is exactly what the
   no-cookie and unknown-token cases assert.
5. **Test files are type-checked.** `apps/web/tsconfig.app.json` includes all of
   `src` and `npm run build` runs `tsc -b`, under `strict`, `noUnusedLocals`,
   `noUnusedParameters` and `noUncheckedIndexedAccess`. A test that compiles
   under Vitest can still fail `make fe-build` — hence the `?.[0]` on
   `mock.calls[0]` and the `as [string, RequestInit]` cast copied from the
   existing tests.
6. **`AuthContext.test.tsx` cannot host the provider test.** It mocks the whole
   `./AuthContext` module (`:10-12`) to drive `RequireAuth`, so the real
   provider never runs there. Hence the new `AuthProvider.logout.test.tsx`.
7. **Stub `useMe` to a non-error result in that new test.** The provider clears
   the access token from a `me.isError` effect; letting it fire would empty the
   token regardless of what `logout()` did, and the assertion would pass for the
   wrong reason.
8. **`server.SetErrorLogger` is package-global.** `newRefreshRouter` points it at
   the discarded logger and restores it via `t.Cleanup`, so a 500 test does not
   spray a fault over `go test` output. Tests in the package are sequential, so
   `-race` is unaffected.

## Which tests can actually fail

The repo's standing rule is that a test must be shown red before the fix, not
assumed to be. Of the nine tests this plan adds, **four** are genuinely red-first
and the plan says so explicitly for the rest:

| Test | Red first? |
|---|---|
| `TestLogout_returns500WhenTheFamilyRevokeFails` | **yes** — fails on `status = 204, want 500` |
| `logoutRequest` rejects with an `ApiError` on a non-ok response | **yes** — old code resolves |
| `logoutRequest` rejects when `fetch` rejects | **yes** — old code resolves |
| `logout()` clears the session and still rejects on failure | **yes** — token survives in `localStorage` today |
| `ProfileMenu` warns that the server session may survive | **yes** — `toast.error` never called |
| `TestLogout_succeedsAndClearsCookie` / `…204WithNoRefreshToken` / `…204ForAnUnknownToken` | no — FR-SRV-4/5 regression pins |
| `logoutRequest` posts with credentials, 204 without a body parse | no — FR-LOGOUT-4/5 regression guard |
| `logout()` clears the session and resolves on success | no — happy-path pin |
| `ProfileMenu` raises no toast on success | no — FR-LOGOUT-13 pin |

## Behaviour matrix (target state)

| Scenario | Server | Local token cleared | Cache purged | Toast |
|---|---|---|---|---|
| Revoke succeeds | `204` | yes | yes | none |
| No refresh cookie | `204` | yes | yes | none |
| Unknown/expired token | `204` | yes | yes | none |
| `RevokeFamily` errors | `500` | yes | yes | error |
| Gateway 5xx / service down | `502`/`503` | yes | yes | error |
| Browser offline (`fetch` rejects) | — | yes | yes | error |

"Local token cleared" is `yes` in every row by FR-LOGOUT-6 — that is D1, not an
oversight.

## Accepted residual risk

When the request never reaches the server, the `HttpOnly` refresh cookie cannot
be cleared by JavaScript, so it survives until it expires or the browser closes.
Anyone able to run script on the origin could trade it for a fresh access token.
Closing that needs a durable client-side "pending logout" that retries — an
explicit non-goal. The toast copy exists precisely because this residue is real
and the user is the only one positioned to act on it (close the browser).

## Commands

```sh
# Node is not always on PATH
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22

make ci                                                   # the gate

go test -race github.com/jtumidanski/myfleet/apps/auth-service/...
npm run -w apps/web test -- src/lib/hooks/api/auth.test.ts   # single-file run
```

No deployment manifests change, so no `kustomize build` or
`kubectl apply --dry-run=server` is required for this task.
