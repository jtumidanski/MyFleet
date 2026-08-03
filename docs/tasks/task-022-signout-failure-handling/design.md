# Sign-Out Failure Handling — Design

Task: task-022-signout-failure-handling
PRD: [`prd.md`](./prd.md) (approved, v1)
Status: Draft
Created: 2026-08-03

---

## 1. Shape of the change

The PRD identifies three stacked suppressions. The fix removes them in the order
a failure travels, and each layer's change is small enough to state in one line:

| Layer | File | Change |
|---|---|---|
| Server | `apps/auth-service/internal/session/resource.go` | `logoutHandler` clears the cookie, then `server.WriteError` on revoke failure instead of `204` |
| Transport | `apps/web/src/lib/hooks/api/auth.ts` | `logoutRequest` delegates to `apiClient.request`; bespoke `fetch` and swallowing `.catch` deleted |
| Context | `apps/web/src/context/AuthContext.tsx` | `logout` moves teardown into a `finally` so it runs on both paths and the rejection still propagates |
| Presentation | `apps/web/src/components/frame/ProfileMenu.tsx` | `void logout()` → a handler that catches and raises a fixed-copy `toast.error` |

No new modules, no new context fields, no shared-package changes. The net line
count is close to zero: the client half is mostly deletion, and the server half
swaps two statements for two statements.

The one thing this design adds that the PRD did not anticipate is **§4 — the
toast cannot use `apiError.message`**, because `server.WriteError` redacts 5xx
titles. That is a genuine constraint discovered in the source, and it changes
FR-LOGOUT-11's implementation (not its intent).

## 2. Data flow, before and after

**Before** — every arrow is lossy:

```
RevokeFamily fails ──▶ logoutHandler logs, returns 204
                          │
                          ▼
             logoutRequest .catch(() => undefined) ──▶ resolved promise
                          │
                          ▼
                   logout() completes normally
                          │
                          ▼
             void logout() ──▶ nothing to discard, nothing shown
```

**After** — the failure survives every hop, and the local teardown happens
regardless:

```
RevokeFamily fails ──▶ clearRefreshCookie(w) ──▶ WriteError ──▶ 500 + JSON:API envelope
                          │
                          ▼
      apiClient.request sees !res.ok ──▶ throw createErrorFromUnknown(...)
                          │
                          ▼
      logout(): finally { clearAccessToken; setHasToken(false); removeQueries }
                 then rethrows
                          │
                          ▼
      ProfileMenu catch ──▶ toast.error(fixed copy)
```

The user lands on the login screen in both diagrams. The difference is whether
anyone is told that the server may disagree.

## 3. Server: `logoutHandler`

### 3.1 The change

```go
func logoutHandler(log logrus.FieldLogger, proc *Processor, cookieSecure bool) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		raw := readRefreshToken(req)
		if raw != "" {
			if err := proc.Logout(raw); err != nil {
				log.WithError(err).Error("logout revoke family")
				// Cookie first: SetCookie writes a header, which is only
				// possible before WriteError calls WriteHeader.
				clearRefreshCookie(w, cookieSecure)
				server.WriteError(w, err)
				return
			}
		}
		clearRefreshCookie(w, cookieSecure)
		w.WriteHeader(http.StatusNoContent)
	}
}
```

### 3.2 Why the raw error is safe to pass to `WriteError`

`server.StatusFor` (`packages/shared-go/server/errors.go:18-43`) returns 500 for
anything that does not match a sentinel. The errors that can reach this line are
GORM/store errors from `RevokeFamily` and non-`ErrNotFound` errors from
`FindByHash` — `Processor.Logout` already collapses `ErrNotFound` to `nil`
(`processor.go:136-145`). Critically, `session.ErrNotFound` is the package's own
sentinel (`model.go:10`), **not** `server.ErrNotFound`, so even if it did reach
`WriteError` it could not be mapped to a 404 by accident. 500 is the only
reachable status here.

No error text leaks: `WriteError` replaces the title with
`InternalErrorTitle` for any status ≥ 500 (`jsonapi.go:100-112`).

### 3.3 The `clearRefreshCookie`-before-`WriteError` ordering

This is the single trap in the server change and the reason FR-SRV-3 exists.
`http.SetCookie` appends to the response header map; `WriteError` →
`WriteJSON` → `WriteHeader` flushes it. Reversing the two lines produces a 500
with **no** `Set-Cookie`, silently — Go does not error on a late header write,
it drops it. A dedicated test asserts the header is present on the 500 response
(§6.3) rather than trusting review to catch a two-line ordering.

### 3.4 Accepted: the failure is logged twice

`WriteError` itself logs 5xx (`jsonapi.go:102-105`). Keeping FR-SRV-2's explicit
`log.WithError(err).Error("logout revoke family")` therefore produces two lines
for one failure. Keeping it anyway: the handler's line names the operation,
`WriteError`'s line only knows the status. One redundant line is a better trade
than an operator grepping for "logout" and finding nothing.

### 3.5 Idempotency is structurally preserved

The `raw == ""` and `ErrNotFound` paths never enter the new branch — the first
because it skips the processor entirely, the second because `Processor.Logout`
returns `nil`. Both still fall through to `clearRefreshCookie` + `204`. FR-SRV-4
holds by construction, and §6.3 pins it with tests anyway.

## 4. Client transport: `logoutRequest`

### 4.1 The change

```ts
export async function logoutRequest(): Promise<void> {
  await apiClient.request<void>('/api/auth/logout', {
    method: 'POST',
    credentials: 'include',
  });
}
```

`apiClient.request` already supplies the JSON:API `Content-Type`
(`apiClient.ts:46`), special-cases `204` before parsing a body
(`apiClient.ts:49`), checks `res.ok`, and throws
`createErrorFromUnknown({ status, body })` (`apiClient.ts:50`).
`credentials: 'include'` reaches `fetch` through the `...init` spread
(`apiClient.ts:27-33`), satisfying FR-LOGOUT-4. FR-LOGOUT-1/2/3/5 are all
discharged by deletion.

### 4.2 Tradeoff accepted: logout inherits the 401-refresh-and-retry

`apiClient.fetchAuthenticated` retries once on a 401 by calling `onRefresh`
(`apiClient.ts:35-38`), which for this app is `refreshAccessToken` — a
`POST /api/auth/refresh` that **rotates the refresh family and sets a new
refresh cookie**. Rotating a family in the middle of trying to revoke it is a
perverse-sounding interaction, so it is worth stating why it is acceptable
rather than designing around it:

- It is unreachable in practice. `/auth/logout` is a public route with no JWT
  middleware (`resource.go:27-36`), and `logoutHandler` can only emit `204` or
  `500`. Nothing in the path produces a 401.
- If it did fire, the outcome is still correct. The retry re-sends the *original*
  raw token from the request; `Processor.Logout` finds it by hash and revokes its
  whole family — including the token the refresh just minted, since rotation
  keeps the family id (`processor.go`, `SetFamilyID(m.FamilyID())`). The cookie
  is cleared on the way out either way.
- On refresh failure `refreshAccessToken` returns `null`, the original 401
  propagates, and the user gets the toast — the correct outcome.

**Alternative rejected:** adding a `skipRefresh` option to `ApiClient`. It would
widen a shared package that the PRD explicitly scopes out (§7 "Not affected"),
to guard a path that cannot occur, and every other caller would have to reason
about a new flag. A code comment on `logoutRequest` recording the above is the
proportionate response.

### 4.3 Alternative rejected: keep the bare `fetch`, just check `res.ok`

Mechanically sufficient for FR-LOGOUT-3, and about the same number of lines. It
was rejected because it leaves logout as the one call site in the app with its
own status-and-error contract, and it would hand-roll a second copy of
`createErrorFromUnknown({ status, body })` including the `204`-body special
case. The PRD's §1.2 argument stands: this is the fix that removes code.

## 5. Context and presentation

### 5.1 `logout()` — teardown in a `finally`

```ts
const logout = useCallback(async () => {
  try {
    await logoutRequest();
  } finally {
    clearAccessToken();
    setHasToken(false);
    queryClient.removeQueries({ queryKey: authKeys.all });
  }
}, [queryClient]);
```

`finally` rethrows implicitly, so this satisfies FR-LOGOUT-6, -7 and -8 in one
construct: teardown always runs, and the rejection still reaches the caller. The
signature stays `() => Promise<void>` (FR-LOGOUT-9).

**Open Question 2 resolved — `logout()` rejects; it does not return `{ ok, error }`.**
The objection in the PRD is real: a promise that rejects while having fully
succeeded at its local job is an odd contract. It wins anyway, on three counts.
First, every error-surfacing call site in the app is
`try { await … } catch (err) { createErrorFromUnknown(err) }` — e.g.
`FleetNameForm.tsx:29-36` — and a lone result-object contract would be the one
shape a reader has to learn. Second, `AuthContextValue.logout` is consumed
through a shared type; changing its return type churns the eight-plus test files
that construct a `baseAuth()` fixture (`ProfileMenu.test.tsx:36`,
`AppLayout.test.tsx:42`, `AdminLayout.test.tsx:46`, and others) for no
behavioural gain. Third, the oddity is a documentation problem, and documentation
solves it: a JSDoc block on `logout` stating "signs out locally in all cases;
rejects to report that the *server* side may not have completed."

### 5.2 Open Question 3 resolved — `ProfileMenu` is the only caller

Verified by grep across `apps/web/src`: the only non-test reference to
`logout()` outside `AuthContext.tsx` itself is `ProfileMenu.tsx:64`. The
401-handling `useEffect` in `AuthContext` (`AuthContext.tsx:43-48`) calls
`clearAccessToken()` and `setHasToken(false)` directly and does **not** route
through `logout()`, so making `logout()` reject cannot produce a new unhandled
rejection anywhere. `AppLayout` and `AdminLayout` reach sign-out only by
rendering `ProfileMenu`, which is what keeps FR-LOGOUT-14 true without effort.

### 5.3 `ProfileMenu` — the toast, and why it ignores `apiError.message`

```tsx
const handleSignOut = () => {
  logout().catch((err: unknown) => {
    const apiError = createErrorFromUnknown(err);
    toast.error('Signed out on this device, but the server may still have an active session.', {
      description: apiError.detail,
    });
  });
};
…
<DropdownMenuItem onSelect={handleSignOut}>Sign out</DropdownMenuItem>
```

**The load-bearing decision here is the fixed message.** The app's established
pattern is `toast.error(apiError.message || 'fallback copy')`. That pattern is
wrong for this case, and quietly so: `server.WriteError` overwrites the title
with `InternalErrorTitle` — the literal string `"internal server error"` — for
every status ≥ 500 (`jsonapi.go:34`, `:100-112`). Since 500 is the *only* status
this feature produces, `apiError.message` is a constant, and following the house
pattern would show the user "internal server error" on the exact path FR-LOGOUT-12
says must explain that the local session ended and the server's may not have.
The fallback string would never be reached, because `message` is never empty.

So the copy is unconditional. The `ApiError` is still constructed (FR-LOGOUT-11)
and still carries information into the toast via `description`: `detail` is
populated only by `server.Detailed` on sub-500 errors (`errors.go:55-66`,
`jsonapi.go:106-111`), so today it is `undefined` and `sonner` omits the
description — but a transport-layer or gateway error that does carry a detail
surfaces it without the copy changing. No success toast (FR-LOGOUT-13).

`onSelect` is Radix's synchronous callback, hence `.catch` on the returned
promise rather than an `async` handler. Raising the toast after `RequireAuth`
has already redirected and unmounted `ProfileMenu` is safe: `sonner`'s `toast`
is a module-level imperative API rendered by the app-root `<Toaster>`, not
component state.

## 6. Testing

Every test below must be shown to fail before the corresponding change and pass
after — not assumed to. Concretely: write the test, run it red against the
current code, then implement. This is the repo's standing rule and the reason
`logout()`'s "runs on both paths" assertion is worth writing at all — it passes
trivially against a `finally` and passes *accidentally* against today's code on
the success path, so only the failure-path variant is evidence.

### 6.1 Transport — `apps/web/src/lib/hooks/api/auth.test.ts`

Extend the existing file, which already stubs the global `fetch` with
`vi.stubGlobal` (`auth.test.ts:39-52`) — `apiClient` calls global `fetch`, so
the harness is unchanged.

- non-ok response (`{ ok: false, status: 500, json: … }` with a JSON:API error
  envelope) → `await expect(logoutRequest()).rejects.toBeInstanceOf(ApiError)`,
  and `status === 500`.
- `fetch` rejects (`mockRejectedValue(new TypeError('Failed to fetch'))`) →
  `rejects.toThrow()`.
- `{ ok: true, status: 204 }` → resolves, and the body is never parsed (a `json`
  stub that throws proves the `204` short-circuit is intact, FR-LOGOUT-5).

### 6.2 Context and presentation

- **New file `apps/web/src/context/AuthProvider.logout.test.tsx`.** The existing
  `AuthContext.test.tsx` mocks the whole `./AuthContext` module
  (`AuthContext.test.tsx:10-12`) to exercise `RequireAuth`, so the real provider
  cannot be rendered in it. The new file mocks `../lib/hooks/api/auth`'s
  `logoutRequest` to reject, renders a real `AuthProvider` + `QueryClientProvider`
  around a probe that calls `logout()`, and asserts: the promise rejects, the
  access token is gone from `localStorage`, and `authKeys.all` has been removed
  from the query cache. Asserting on stored state, not on the returned promise
  alone, is what makes this test able to fail.
- **`apps/web/src/components/frame/ProfileMenu.test.tsx`.** Add `vi.mock('sonner')`
  and two cases: `logout` rejecting → `toast.error` called once with copy
  matching `/server may still/`; `logout` resolving → `toast.error` not called
  (FR-LOGOUT-13). The existing `renderMenu({ logout })` fixture
  (`ProfileMenu.test.tsx:27-40`) already takes the override.

### 6.3 Server — `apps/auth-service/internal/session/resource_test.go`

`fakeStore` currently has no way to fail (`processor_test.go:59-69`,
`RevokeFamily` always returns `nil`). Add one field:

```go
type fakeStore struct {
	…
	revokeErr error // when non-nil, RevokeFamily fails with it
}
```

`RevokeFamily` returns it before mutating anything. This is the smallest change
that makes the failure path reachable and mirrors how the package's other fakes
work; no mocking framework is introduced.

Cases, all through the real router built by `newRefreshRouter`
(`resource_test.go:20-37`):

| Test | Setup | Asserts |
|---|---|---|
| revoke fails | `store.revokeErr = errors.New("db down")`, valid cookie | status 500; body is a JSON:API error envelope; **`Set-Cookie` clearing header present** (the §3.3 trap) |
| success | valid cookie | 204; family revoked in the store; cookie cleared |
| no cookie | no cookie, no body | 204; processor never called |
| unknown token | cookie with an unseeded value | 204 (`ErrNotFound` → `nil`) |

The "revoke fails" case is the one that must be run red first — against today's
handler it returns 204, so the assertion fails on status before it ever reaches
the header check.

### 6.4 Open Question 1 resolved — no "Try again" action on the toast

Deferred, not adopted. A retry would work mechanically (the route is public and
reads the cookie, which the browser still holds on the failure paths), but it
pulls in state this feature does not otherwise need: what the button does on a
second failure, whether it re-toasts or replaces, and whether a retry that
succeeds should then confirm — which would reintroduce the success toast
FR-LOGOUT-13 removes. The task's stated behaviour is "warn loudly", and the copy
already tells the user the actionable remedy (close the browser). If the failure
turns out to be common in practice, it earns its own design pass.

### 6.5 Not doing: browser verification

Per PRD D3. The assertions are control flow and status codes; jsdom's blindness
to CSS — the usual reason this repo reaches for Playwright — does not apply.
`make ci` is the gate.

## 7. Risks

**The `Set-Cookie` ordering regresses silently.** Highest-consequence, lowest-
visibility failure in the change. Mitigated by the §6.3 header assertion, which
is the only thing standing between a future refactor and a 500 that leaves the
browser cookie in place.

**The toast copy drifts back to `apiError.message`.** A reviewer applying the
house pattern from `FleetNameForm` would "fix" §5.3 into a regression that shows
"internal server error". Mitigated by the `/server may still/` assertion in
§6.2 and by an inline comment at the call site explaining the redaction.

**Widening the response contract.** Accepted in PRD §5: no client distinguishes
logout statuses today, and this app is the only consumer. An older web build
ignores the 500 exactly as it ignores the 204.

**Residual, out of scope and unchanged by this task:** when the request never
reaches the server, the `HttpOnly` refresh cookie survives in the browser and
JavaScript cannot clear it. The toast exists because of this residue, not in
spite of it — see PRD §8.

## 8. Verification

`make ci` (lint-check, vet, test, build, fe-test, fe-build). No deployment
manifests change, so no `kustomize` render or server dry-run is required for
this task.
