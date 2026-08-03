# Sign-Out Failure Handling Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make a failed sign-out visible — the auth-service reports a failed refresh-token-family revoke as a 5xx, the web client stops swallowing it, and the user gets a toast saying the local session ended but the server's may not have.

**Architecture:** Three stacked suppressions are removed in the order a failure travels. The server's `logoutHandler` clears the cookie and then `server.WriteError`s instead of returning an unconditional `204`. `logoutRequest` drops its bespoke `fetch` + `.catch(() => undefined)` and delegates to the shared `apiClient`, which already checks `res.ok` and throws an `ApiError`. `AuthContext.logout` moves its local teardown into a `finally` so the user is signed out locally on both paths while the rejection still reaches the caller. `ProfileMenu` catches that rejection and raises a fixed-copy `toast.error`. No new modules, no new context fields, no shared-package changes.

**Tech Stack:** Go 1.x (chi, logrus, `packages/shared-go/server`), React 18 + TypeScript (Vite, Vitest, Testing Library, TanStack Query, Radix dropdown, sonner, `@myfleet/shared-ts`).

## Global Constraints

- **Toast copy** — the message must state that the local session ended but the server session may not have. Use exactly: `Signed out on this device, but the server may still have an active session.` (FR-LOGOUT-12). Do **not** use the house `toast.error(apiError.message || 'fallback')` pattern here; see Task 4's rationale.
- **No success toast** on sign-out (FR-LOGOUT-13).
- **`logout()` keeps the signature `() => Promise<void>`** — no new `AuthContextValue` fields (FR-LOGOUT-9).
- **The fix lives once, in `ProfileMenu`** — never duplicated into `AppLayout` or `AdminLayout` (FR-LOGOUT-14).
- **`clearRefreshCookie(w, cookieSecure)` must be called BEFORE `server.WriteError`** on the server failure path. `http.SetCookie` writes a header; `WriteError` flushes headers via `WriteHeader`. Reversing the two lines silently drops the `Set-Cookie` (FR-SRV-3).
- **Idempotency is untouched** — no refresh token present, and an unknown/expired token, both still return `204` (FR-SRV-4).
- **`credentials: 'include'` must survive** the move onto `apiClient` (FR-LOGOUT-4).
- **Verification gate:** `make ci`. No deployment manifests change, so no `kustomize` render or `kubectl --dry-run` is required for this task.
- **Node is not always on `PATH`.** Before any `npm`/`npx` command: `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22`.
- **Work happens in this worktree:** `/home/tumidanski/source/MyFleet/.worktrees/task-022-signout-failure-handling`, branch `task-022-signout-failure-handling`. All paths below are relative to it.

---

## File Structure

| File | Responsibility | Task |
|---|---|---|
| `apps/auth-service/internal/session/processor_test.go` | `fakeStore` gains an injectable `RevokeFamily` failure so the server failure path is reachable at all | 1 |
| `apps/auth-service/internal/session/resource_test.go` | Logout handler HTTP tests: 500 + cleared cookie on revoke failure; 204 on success / no cookie / unknown token | 1 |
| `apps/auth-service/internal/session/resource.go` | `logoutHandler` returns 5xx on revoke failure | 1 |
| `apps/web/src/lib/hooks/api/auth.test.ts` | `logoutRequest` transport tests | 2 |
| `apps/web/src/lib/hooks/api/auth.ts` | `logoutRequest` delegates to `apiClient` | 2 |
| `apps/web/src/context/AuthProvider.logout.test.tsx` | **New.** Real `AuthProvider`, driven through `logout()` | 3 |
| `apps/web/src/context/AuthContext.tsx` | `logout` teardown moves into `finally` | 3 |
| `apps/web/src/components/frame/ProfileMenu.test.tsx` | Toast-on-failure / no-toast-on-success tests | 4 |
| `apps/web/src/components/frame/ProfileMenu.tsx` | Catches the rejection, raises the toast | 4 |
| `apps/web/src/components/AppLayout.test.tsx` | Fixture repair: its sign-out test's `logout` must return a promise | 4 |
| `apps/web/src/components/admin/AdminLayout.test.tsx` | Same fixture repair | 4 |

---

### Task 1: Server — a failed family revoke returns 500

This is the load-bearing half. While `logoutHandler` returns `204` unconditionally, any client-side status check is checking a value that is constant by construction, so the client work in Tasks 2–4 would surface transport failures only and leave the failure the feature is about invisible.

**Files:**
- Modify: `apps/auth-service/internal/session/processor_test.go:19-24` (the `fakeStore` struct) and `:59-69` (`RevokeFamily`)
- Modify: `apps/auth-service/internal/session/resource_test.go:20-38` (the `newRefreshRouter` helper), `:61-65` and `:107-111` (its two call sites), plus new helpers and tests at the end of the file
- Modify: `apps/auth-service/internal/session/resource.go:85-96` (`logoutHandler`)

**Interfaces:**
- Consumes: `Processor.Logout(raw string) error` (`processor.go:136-145`) — already collapses `ErrNotFound` to `nil`; `server.WriteError(w http.ResponseWriter, err error)` and `server.InternalErrorTitle` (`packages/shared-go/server/jsonapi.go:34,95`); `server.SetErrorLogger(log logrus.FieldLogger)` (`jsonapi.go:54`); `clearRefreshCookie(w http.ResponseWriter, secure bool)` (`resource.go:130`).
- Produces: `newRefreshRouter(t *testing.T, resolve PrincipalResolver) (chi.Router, *Processor, *fakeStore, string)` — **note the widened return, the store is now returned third**; `postLogout(r chi.Router, raw string) *httptest.ResponseRecorder`; `unusedResolver`; `fakeStore.revokeErr error`.

- [ ] **Step 1: Make the `RevokeFamily` failure path reachable in the fake store**

`fakeStore` has no way to fail today, so the handler's error branch cannot be exercised. Add one field and an early return. Record the call *before* the injected failure so `revokedFamilies` stays a true call log — that is what lets a test tell "never called" apart from "called and failed".

In `apps/auth-service/internal/session/processor_test.go`, change the struct (currently lines 19-24):

```go
type fakeStore struct {
	byHash map[string]Model // hash -> stored token
	byID   map[string]Model // id   -> stored token

	revokedFamilies []string // family ids passed to RevokeFamily, in order
	revokeErr       error    // when non-nil, RevokeFamily fails with it
}
```

and `RevokeFamily` (currently lines 59-69):

```go
func (f *fakeStore) RevokeFamily(familyID string, at time.Time) error {
	// Recorded BEFORE the injected failure so revokedFamilies remains a call
	// log rather than a success log: a test asserting "no family was revoked"
	// must be able to distinguish a call that failed from a call that never
	// happened.
	f.revokedFamilies = append(f.revokedFamilies, familyID)
	if f.revokeErr != nil {
		return f.revokeErr
	}
	for id, m := range f.byID {
		if m.FamilyID() == familyID && !m.IsRevoked() {
			m.revokedAt = &at
			f.byID[id] = m
			f.byHash[m.TokenHash()] = m
		}
	}
	return nil
}
```

- [ ] **Step 2: Widen the router helper to hand back the store, and quiet the 5xx logger**

In `apps/auth-service/internal/session/resource_test.go`, replace `newRefreshRouter` (currently lines 18-38) with:

```go
// newRefreshRouter mounts the real public routes over a store seeded with one
// valid refresh token, and returns the router, the processor, the store and
// that token's raw value. The store is returned so a test can inject a
// RevokeFamily failure and inspect what was revoked.
func newRefreshRouter(t *testing.T, resolve PrincipalResolver) (chi.Router, *Processor, *fakeStore, string) {
	t.Helper()
	store := newFakeStore()
	proc := newTestProcessor(store)

	raw := "valid-refresh-token"
	store.seed(NewBuilder().
		SetUserID("user-1").
		SetTokenHash(HashRefresh(raw)).
		SetExpiresAt(time.Now().Add(refreshTTL)).
		Build())

	log := logrus.New()
	log.SetOutput(io.Discard)

	// WriteError logs every 5xx through the package-level error logger, which
	// otherwise falls back to logrus' stderr standard logger. Point it at the
	// same discarded output so the logout-failure test does not print a fault
	// over an otherwise quiet `go test` run, and restore the default after.
	server.SetErrorLogger(log)
	t.Cleanup(func() { server.SetErrorLogger(nil) })

	r := chi.NewRouter()
	r.Group(InitializePublicRoutes(log, proc, resolve, false))
	return r, proc, store, raw
}
```

Add the shared-go import to the file's import block:

```go
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
```

Update the two existing call sites for the new arity:
- line 65: `r, proc, raw := newRefreshRouter(t, resolve)` → `r, proc, _, raw := newRefreshRouter(t, resolve)`
- line 111: `r, _, raw := newRefreshRouter(t, resolve)` → `r, _, _, raw := newRefreshRouter(t, resolve)`

- [ ] **Step 3: Add the logout request helper and the resolver stub**

Append to `apps/auth-service/internal/session/resource_test.go`:

```go
// postLogout posts to the logout route carrying raw as the refresh cookie. An
// empty raw sends no cookie at all — the "nothing to revoke" case.
func postLogout(r chi.Router, raw string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	if raw != "" {
		req.AddCookie(&http.Cookie{Name: RefreshCookieName, Value: raw})
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// unusedResolver satisfies InitializePublicRoutes for the logout tests. The
// logout route never resolves a principal, so reaching this is a bug in the
// test, not a scenario.
func unusedResolver(context.Context, string) (Principal, error) {
	return Principal{}, errors.New("the logout route must not resolve a principal")
}
```

- [ ] **Step 4: Write the failing test — a revoke failure must produce a 500 that still clears the cookie**

Append to `apps/auth-service/internal/session/resource_test.go`:

```go
// TestLogout_returns500WhenTheFamilyRevokeFails is the load-bearing test of
// this task. Today the handler logs the failure and returns 204 regardless, so
// a failed revocation and a successful one are indistinguishable to any client
// — no client-side status check, however correct, can observe the difference.
func TestLogout_returns500WhenTheFamilyRevokeFails(t *testing.T) {
	r, _, store, raw := newRefreshRouter(t, unusedResolver)
	store.revokeErr = errors.New("db down")

	rec := postLogout(r, raw)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500: %s", rec.Code, rec.Body.String())
	}
	if len(store.revokedFamilies) != 1 {
		t.Fatalf("RevokeFamily calls = %v, want exactly one attempt", store.revokedFamilies)
	}

	var body struct {
		Errors []struct {
			Status string `json:"status"`
			Title  string `json:"title"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Errors) != 1 {
		t.Fatalf("errors = %v, want exactly one JSON:API error", body.Errors)
	}
	if body.Errors[0].Status != "500" {
		t.Errorf("errors[0].status = %q, want \"500\"", body.Errors[0].Status)
	}
	// The store's error text must never reach the client (SEC-09): WriteError
	// replaces the title of every 5xx with the fixed one.
	if body.Errors[0].Title != server.InternalErrorTitle {
		t.Errorf("errors[0].title = %q, want %q", body.Errors[0].Title, server.InternalErrorTitle)
	}

	// The ordering trap. clearRefreshCookie must run BEFORE WriteError, because
	// SetCookie appends a header and WriteError flushes them. Swap the two lines
	// and Go drops the Set-Cookie without complaint, leaving the browser holding
	// a cookie for a family that is still live — the worst outcome available.
	if !refreshCookieCleared(rec) {
		t.Fatalf("the refresh cookie must still be cleared on the failure path; cookies = %v",
			rec.Result().Cookies())
	}
}

// TestLogout_succeedsAndClearsCookie pins the unchanged happy path (FR-SRV-5):
// 204, the family actually revoked, the browser's cookie cleared.
func TestLogout_succeedsAndClearsCookie(t *testing.T) {
	r, _, store, raw := newRefreshRouter(t, unusedResolver)

	rec := postLogout(r, raw)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204: %s", rec.Code, rec.Body.String())
	}
	if len(store.revokedFamilies) != 1 {
		t.Fatalf("RevokeFamily calls = %v, want exactly one", store.revokedFamilies)
	}
	revoked, err := store.FindByHash(HashRefresh(raw))
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if !revoked.IsRevoked() {
		t.Fatal("the presented token's family must be revoked after a successful logout")
	}
	if !refreshCookieCleared(rec) {
		t.Fatalf("refresh cookie must be cleared on success; cookies = %v", rec.Result().Cookies())
	}
}

// TestLogout_returns204WithNoRefreshToken — FR-SRV-4. revokeErr is armed so the
// 204 is evidence the processor was never reached, not merely that it happened
// to succeed.
func TestLogout_returns204WithNoRefreshToken(t *testing.T) {
	r, _, store, _ := newRefreshRouter(t, unusedResolver)
	store.revokeErr = errors.New("RevokeFamily must not be called without a token")

	rec := postLogout(r, "")

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204: %s", rec.Code, rec.Body.String())
	}
	if len(store.revokedFamilies) != 0 {
		t.Fatalf("nothing may be revoked when no token was presented; calls = %v", store.revokedFamilies)
	}
	if !refreshCookieCleared(rec) {
		t.Fatalf("refresh cookie must be cleared even with nothing to revoke; cookies = %v",
			rec.Result().Cookies())
	}
}

// TestLogout_returns204ForAnUnknownToken — the other half of FR-SRV-4. An
// expired or fabricated token is a no-op success, not a fault: Processor.Logout
// maps ErrNotFound to nil. revokeErr is armed to prove the lookup
// short-circuits before any revoke is attempted.
func TestLogout_returns204ForAnUnknownToken(t *testing.T) {
	r, _, store, _ := newRefreshRouter(t, unusedResolver)
	store.revokeErr = errors.New("RevokeFamily must not be called for an unknown token")

	rec := postLogout(r, "never-issued-token")

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204: %s", rec.Code, rec.Body.String())
	}
	if len(store.revokedFamilies) != 0 {
		t.Fatalf("an unknown token must revoke nothing; calls = %v", store.revokedFamilies)
	}
	if !refreshCookieCleared(rec) {
		t.Fatalf("refresh cookie must be cleared for an unknown token; cookies = %v",
			rec.Result().Cookies())
	}
}
```

- [ ] **Step 5: Run the tests and confirm the right one fails for the right reason**

Run: `go test -race -run 'TestLogout|TestRefresh' github.com/jtumidanski/myfleet/apps/auth-service/internal/session -v`

Expected:
- `TestLogout_returns500WhenTheFamilyRevokeFails` — **FAIL** with `status = 204, want 500`. It must fail on the status line, before it ever reaches the envelope or cookie checks.
- `TestLogout_succeedsAndClearsCookie`, `TestLogout_returns204WithNoRefreshToken`, `TestLogout_returns204ForAnUnknownToken`, and both `TestRefresh_*` — **PASS**. These three logout cases are regression pins for FR-SRV-4/5, not red-first tests; they are expected to be green both before and after, and the plan says so rather than implying otherwise.

If `TestLogout_returns500WhenTheFamilyRevokeFails` fails on anything other than the status assertion, stop and diagnose — the test harness is wrong, not the handler.

- [ ] **Step 6: Implement the handler change**

In `apps/auth-service/internal/session/resource.go`, replace `logoutHandler` (lines 85-96):

```go
func logoutHandler(log logrus.FieldLogger, proc *Processor, cookieSecure bool) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		raw := readRefreshToken(req)
		if raw != "" {
			if err := proc.Logout(raw); err != nil {
				// Kept alongside WriteError's own 5xx log line: this one names
				// the operation, that one only knows the status. One redundant
				// line beats an operator grepping for "logout" and finding
				// nothing.
				log.WithError(err).Error("logout revoke family")
				// Cookie FIRST. SetCookie appends a header and WriteError
				// flushes them, so the reverse order drops the Set-Cookie
				// silently. Clearing the browser's copy is strictly
				// risk-reducing even when the family survives in the database.
				clearRefreshCookie(w, cookieSecure)
				// The raw error is safe to pass through: StatusFor maps
				// anything without a matching sentinel to 500, and WriteError
				// redacts the text of every 5xx body. session.ErrNotFound is
				// this package's own sentinel, not server.ErrNotFound, so it
				// could not be mapped to a 404 by accident even if it reached
				// here — which it cannot, since Processor.Logout collapses it
				// to nil.
				server.WriteError(w, err)
				return
			}
		}
		clearRefreshCookie(w, cookieSecure)
		w.WriteHeader(http.StatusNoContent)
	}
}
```

- [ ] **Step 7: Run the package tests and confirm they all pass**

Run: `go test -race github.com/jtumidanski/myfleet/apps/auth-service/...`
Expected: PASS, including all four `TestLogout_*` cases.

- [ ] **Step 8: Vet and commit**

```bash
go vet github.com/jtumidanski/myfleet/apps/auth-service/...
git add apps/auth-service/internal/session/resource.go \
        apps/auth-service/internal/session/resource_test.go \
        apps/auth-service/internal/session/processor_test.go
git commit -m "fix(auth-service): report a failed logout revoke as 500 instead of 204"
```

---

### Task 2: Transport — `logoutRequest` goes through `apiClient`

`apiClient.request` already does everything `logoutRequest` needs: it skips the body parse on `204`, checks `res.ok`, and throws `createErrorFromUnknown({ status, body })`. This task is mostly deletion.

**Files:**
- Modify: `apps/web/src/lib/hooks/api/auth.test.ts` (imports at `:1-6`, new describe block appended)
- Modify: `apps/web/src/lib/hooks/api/auth.ts:63-71`

**Interfaces:**
- Consumes: `apiClient.request<T>(path: string, init?: RequestInit): Promise<T>` (`packages/shared-ts/src/apiClient.ts:42`); `ApiError` and `createErrorFromUnknown` from `@myfleet/shared-ts` (`packages/shared-ts/src/errors.ts:3,23`).
- Produces: `logoutRequest(): Promise<void>` — unchanged signature, now **rejecting** with an `ApiError` on a non-ok status and rejecting whatever `fetch` rejected with on a transport failure.

- [ ] **Step 1: Write the failing tests**

In `apps/web/src/lib/hooks/api/auth.test.ts`, extend the existing import of `./auth` to include `logoutRequest`:

```ts
import { logoutRequest, updateThemePreference, useMe } from './auth';
```

and add the `ApiError` import below the vitest imports:

```ts
import { ApiError } from '@myfleet/shared-ts';
```

Append this describe block to the end of the file:

```ts
describe('logoutRequest', () => {
  beforeEach(() => {
    localStorage.clear();
    clearAccessToken();
  });
  afterEach(() => vi.unstubAllGlobals());

  // FR-LOGOUT-3. fetch does not reject on a non-2xx, and the old bare-fetch
  // implementation never read `status`/`ok`, so a 500 — or an HTML error page
  // from the gateway — was consumed as a successful sign-out.
  it('rejects with an ApiError when the server reports a failure', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 500,
        json: async () => ({
          errors: [{ status: '500', code: 'internal_error', title: 'internal server error' }],
        }),
      }),
    );

    const outcome = await logoutRequest().then(
      () => null,
      (reason: unknown) => reason,
    );

    expect(outcome).toBeInstanceOf(ApiError);
    expect((outcome as ApiError).status).toBe(500);
  });

  // FR-LOGOUT-2/3: offline, DNS failure, connection reset. The old
  // `.catch(() => undefined)` turned every one of these into a resolved
  // promise, so the function had exactly one possible outcome.
  it('rejects when the network request itself fails', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('Failed to fetch')));

    await expect(logoutRequest()).rejects.toThrow('Failed to fetch');
  });

  // FR-LOGOUT-4/5. This one passes against the old implementation too — it is a
  // regression guard, not a red-first test. It pins the two things a rewrite
  // onto apiClient could quietly drop: the cookie being sent, and the 204
  // short-circuit that keeps an empty body from being parsed.
  it('posts with credentials and treats 204 as success without parsing a body', async () => {
    const json = vi.fn(() => {
      throw new Error('a 204 response has no body to parse');
    });
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, status: 204, json });
    vi.stubGlobal('fetch', fetchMock);

    await expect(logoutRequest()).resolves.toBeUndefined();

    expect(json).not.toHaveBeenCalled();
    const [path, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(path).toBe('/api/auth/logout');
    expect(init.method).toBe('POST');
    expect(init.credentials).toBe('include');
  });
});
```

- [ ] **Step 2: Run the tests to verify the first two fail**

```sh
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
npm run -w apps/web test -- src/lib/hooks/api/auth.test.ts
```

Expected:
- `rejects with an ApiError when the server reports a failure` — **FAIL** (`outcome` is `null`; the old implementation resolves).
- `rejects when the network request itself fails` — **FAIL** (resolves instead of rejecting).
- `posts with credentials and treats 204 as success without parsing a body` — **PASS** already, as documented above.

- [ ] **Step 3: Rewrite `logoutRequest` onto `apiClient`**

Replace `apps/web/src/lib/hooks/api/auth.ts:63-71` with:

```ts
/**
 * `POST /api/auth/logout` — revokes the refresh-token family server-side and
 * clears the HttpOnly refresh cookie. `credentials: 'include'` so the browser
 * sends the cookie to be invalidated; it reaches fetch through apiClient's
 * `init` spread.
 *
 * REJECTS on a non-2xx and on a transport failure. It used to swallow both — a
 * bare fetch terminated by `.catch(() => undefined)`, never reading `ok` or
 * `status` — which made every sign-out indistinguishable from a successful one.
 *
 * Going through apiClient means logout inherits the one-shot 401
 * refresh-and-retry. That is unreachable in practice: /api/auth/logout is a
 * public route with no JWT middleware and emits only 204 or 500. If it ever did
 * fire, the outcome is still correct — the retry re-sends the ORIGINAL raw
 * token, and the server revokes its whole family, including the token the
 * refresh just minted, since rotation keeps the family id.
 */
export async function logoutRequest(): Promise<void> {
  await apiClient.request<void>('/api/auth/logout', {
    method: 'POST',
    credentials: 'include',
  });
}
```

The `Content-Type: application/vnd.api+json` header is no longer passed explicitly — `apiClient.request` supplies it as a default header (`apiClient.ts:46`).

- [ ] **Step 4: Run the tests to verify they pass**

```sh
npm run -w apps/web test -- src/lib/hooks/api/auth.test.ts
```
Expected: PASS, all three new cases plus the pre-existing `useMe` and `updateThemePreference` suites.

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/lib/hooks/api/auth.ts apps/web/src/lib/hooks/api/auth.test.ts
git commit -m "fix(web): route logoutRequest through apiClient so failures reject"
```

---

### Task 3: Context — the teardown runs on both paths, the failure still propagates

**Files:**
- Create: `apps/web/src/context/AuthProvider.logout.test.tsx`
- Modify: `apps/web/src/context/AuthContext.tsx:25` (the interface member's doc comment) and `:55-60` (`logout`)

**Interfaces:**
- Consumes: `logoutRequest(): Promise<void>` from Task 2; `authKeys.all` (`lib/hooks/api/auth.ts:7-10`); `clearAccessToken()`, `setAccessToken()`, `ACCESS_TOKEN_KEY` (`lib/api/token.ts`).
- Produces: `AuthContextValue.logout: () => Promise<void>` — signature unchanged. New contract: always signs out locally; rejects when `logoutRequest` rejected.

Why a new test file rather than extending `AuthContext.test.tsx`: that file mocks the whole `./AuthContext` module (`AuthContext.test.tsx:10-12`) so it can drive `RequireAuth` against arbitrary auth states, which means the real provider never runs there and cannot be rendered in it.

- [ ] **Step 1: Write the failing test**

Create `apps/web/src/context/AuthProvider.logout.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { act, render, screen, fireEvent } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { AuthProvider, useAuth } from './AuthContext';
import { ACCESS_TOKEN_KEY, setAccessToken } from '../lib/api/token';

/**
 * The REAL AuthProvider, exercised through logout().
 *
 * AuthContext.test.tsx cannot host this: it mocks the whole ./AuthContext
 * module to drive RequireAuth, so the actual provider never runs there.
 *
 * useMe is stubbed to a settled, non-error result on purpose. The provider
 * clears the access token from a `me.isError` effect, and a test that let that
 * fire would find an empty token no matter what logout() did — the assertion
 * would pass for the wrong reason.
 */
const logoutRequestMock = vi.fn<() => Promise<void>>();

vi.mock('../lib/hooks/api/auth', () => ({
  authKeys: { all: ['auth'], me: () => ['auth', 'me'] },
  useMe: () => ({ data: undefined, isError: false, isLoading: false }),
  logoutRequest: () => logoutRequestMock(),
}));

function SignOutProbe({ onSettled }: { onSettled: (outcome: unknown) => void }) {
  const { logout } = useAuth();
  return (
    <button
      type="button"
      onClick={() => {
        logout().then(() => onSettled(null), onSettled);
      }}
    >
      sign out
    </button>
  );
}

function renderProbe(onSettled: (outcome: unknown) => void): QueryClient {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  queryClient.setQueryData(['auth', 'me'], { user: { id: 'u1' } });
  render(
    <QueryClientProvider client={queryClient}>
      <AuthProvider>
        <SignOutProbe onSettled={onSettled} />
      </AuthProvider>
    </QueryClientProvider>,
  );
  return queryClient;
}

describe('AuthProvider logout', () => {
  beforeEach(() => {
    localStorage.clear();
    logoutRequestMock.mockReset();
  });

  // Decision D1, and the one test here that can fail. The user asked to leave,
  // so the local session ends even when the server never heard about it — AND
  // the caller still learns the request failed, so it can say so. Asserting on
  // stored state rather than on the promise alone is what gives it teeth:
  // against the old `await logoutRequest(); clearAccessToken(); …` the
  // rejection aborted the teardown and the token survived in localStorage.
  it('clears the local session and still rejects when the request fails', async () => {
    setAccessToken('tok-123');
    logoutRequestMock.mockRejectedValue(new Error('network down'));
    const outcomes: unknown[] = [];
    const queryClient = renderProbe((outcome) => outcomes.push(outcome));

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'sign out' }));
    });

    expect(outcomes).toHaveLength(1);
    expect(outcomes[0]).toBeInstanceOf(Error);
    expect(localStorage.getItem(ACCESS_TOKEN_KEY)).toBeNull();
    expect(queryClient.getQueryData(['auth', 'me'])).toBeUndefined();
  });

  it('clears the local session and resolves when the request succeeds', async () => {
    setAccessToken('tok-123');
    logoutRequestMock.mockResolvedValue(undefined);
    const outcomes: unknown[] = [];
    const queryClient = renderProbe((outcome) => outcomes.push(outcome));

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'sign out' }));
    });

    expect(outcomes).toHaveLength(1);
    expect(outcomes[0]).toBeNull();
    expect(localStorage.getItem(ACCESS_TOKEN_KEY)).toBeNull();
    expect(queryClient.getQueryData(['auth', 'me'])).toBeUndefined();
  });
});
```

- [ ] **Step 2: Run the test to verify the failure case fails**

```sh
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
npm run -w apps/web test -- src/context/AuthProvider.logout.test.tsx
```

Expected:
- `clears the local session and still rejects when the request fails` — **FAIL** on `expect(localStorage.getItem(ACCESS_TOKEN_KEY)).toBeNull()` (it is still `'tok-123'`). The `toBeInstanceOf(Error)` line above it passes already; the teardown assertions are the ones that matter.
- `clears the local session and resolves when the request succeeds` — **PASS** already (happy-path regression pin).

- [ ] **Step 3: Move the teardown into a `finally`**

In `apps/web/src/context/AuthContext.tsx`, replace `logout` (lines 55-60):

```tsx
  /**
   * Ends the session.
   *
   * Signs out LOCALLY in every case — access token cleared, identity cache
   * purged — because the user asked to leave and a server failure does not
   * revoke that request. It then REJECTS to report that the *server* side may
   * not have completed: the refresh-token family may still be live, and only
   * the caller is positioned to tell the user so. Both happen, not either.
   *
   * `finally` rethrows implicitly, which is what makes "teardown always runs"
   * and "the rejection still propagates" one construct rather than two.
   */
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

and document the contract on the interface member (line 25):

```tsx
  /**
   * Ends the local session unconditionally; rejects when the server-side
   * revoke failed, so the caller can warn that the session may still be live
   * on the server. See AuthProvider for the full contract.
   */
  logout: () => Promise<void>;
```

- [ ] **Step 4: Run the test to verify it passes**

```sh
npm run -w apps/web test -- src/context/AuthProvider.logout.test.tsx
```
Expected: PASS, both cases.

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/context/AuthContext.tsx apps/web/src/context/AuthProvider.logout.test.tsx
git commit -m "fix(web): sign out locally even when the logout request fails"
```

> **Note for the next task:** as of this commit `logout()` can reject while `ProfileMenu` still does `void logout()`, which is an unhandled rejection at runtime. Task 4 closes that in the very next commit; do not stop here. `ProfileMenu.tsx:64` is the only non-test caller of `logout()` (verified by grep across `apps/web/src`) — the provider's own 401 effect calls `clearAccessToken()`/`setHasToken(false)` directly and does not route through `logout()`.

---

### Task 4: Presentation — catch the rejection and raise the toast

**Files:**
- Modify: `apps/web/src/components/frame/ProfileMenu.test.tsx` (imports `:1-2`, `renderMenu` fixture `:36`, `beforeEach` `:43-45`, the existing sign-out test `:76`, plus two new tests)
- Modify: `apps/web/src/components/frame/ProfileMenu.tsx` (imports `:1-13`, body `:26-33`, item `:64`)
- Modify: `apps/web/src/components/AppLayout.test.tsx:106`
- Modify: `apps/web/src/components/admin/AdminLayout.test.tsx:142`

**Interfaces:**
- Consumes: `useAuth().logout: () => Promise<void>` from Task 3; `createErrorFromUnknown(e: unknown): ApiError` from `@myfleet/shared-ts`; `toast.error(message, options?)` from `sonner`.
- Produces: nothing consumed by later tasks.

**The load-bearing decision here is the fixed message.** The app's house pattern is `toast.error(apiError.message || 'fallback copy')` (e.g. `FleetNameForm.tsx:34-35`). It is wrong here, and quietly so: `server.WriteError` overwrites the title with `InternalErrorTitle` — the literal string `"internal server error"` — for every status ≥ 500 (`packages/shared-go/server/jsonapi.go:34,100-112`). Since 500 is the only status this feature produces, `apiError.message` is a constant, the `||` fallback is never reached, and following the house pattern would show the user "internal server error" on the exact path that is supposed to explain what is still true. The `ApiError` is still constructed and still carries information into the toast via `description`.

- [ ] **Step 1: Repair the fixtures that assume `logout` returns nothing**

`renderMenu`'s default and three test-local `logout` mocks are bare `vi.fn()`, which returns `undefined`. Once the component calls `logout().catch(...)` those throw `TypeError: Cannot read properties of undefined (reading 'catch')`. Fix all four **before** touching the component:

In `apps/web/src/components/frame/ProfileMenu.test.tsx`:
- line 36: `logout: vi.fn(),` → `logout: vi.fn().mockResolvedValue(undefined),`
- line 76: `const logout = vi.fn();` → `const logout = vi.fn().mockResolvedValue(undefined);`

In `apps/web/src/components/AppLayout.test.tsx` line 106 (inside `it('still signs the user out from the profile menu', …)`):
- `const logout = vi.fn();` → `const logout = vi.fn().mockResolvedValue(undefined);`

In `apps/web/src/components/admin/AdminLayout.test.tsx` line 142 (inside `it('gathers the identity and the sign-out action under one profile menu', …)`):
- `const logout = vi.fn();` → `const logout = vi.fn().mockResolvedValue(undefined);`

The other `logout: vi.fn()` occurrences in the suite (`DashboardPage`, `ThemeSync`, `VehiclesPage`, `LoginPage`, `FrameHeader`, `RequirePlatformAdmin`, `postPurgeRouting`, `AuthContext`) never click Sign out and need no change.

- [ ] **Step 2: Write the failing test**

In `apps/web/src/components/frame/ProfileMenu.test.tsx`, extend the two top imports:

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { toast } from 'sonner';
```

and add the module mock next to the existing `vi.mock('../../context/AuthContext', …)` block:

```tsx
vi.mock('sonner', () => ({ toast: { error: vi.fn(), success: vi.fn() } }));
```

Extend `beforeEach` (line 43-45):

```tsx
  beforeEach(() => {
    mockAuth.mockReset();
    vi.mocked(toast.error).mockReset();
  });
```

and append these two cases inside the `describe('ProfileMenu', …)` block:

```tsx
  // FR-LOGOUT-11/12. The copy is FIXED, not `apiError.message || fallback`:
  // WriteError redacts the title of every 5xx to the literal "internal server
  // error", and 500 is the only status this path produces — so the house
  // pattern would show the user that string on the exact path that exists to
  // explain what is still true. A reviewer "fixing" this back to the house
  // pattern is the regression this assertion catches.
  it('warns that the server session may survive when sign-out fails', async () => {
    const userEvents = userEvent.setup();
    renderMenu({ logout: vi.fn().mockRejectedValue(new Error('network down')) });

    await userEvents.click(screen.getByRole('button', { name: 'Account menu' }));
    await userEvents.click(screen.getByRole('menuitem', { name: 'Sign out' }));

    await waitFor(() => expect(toast.error).toHaveBeenCalledTimes(1));
    expect(vi.mocked(toast.error).mock.calls[0]?.[0]).toMatch(/server may still/);
  });

  // FR-LOGOUT-13: landing on the login screen is already unambiguous feedback.
  it('raises no toast when sign-out succeeds', async () => {
    const userEvents = userEvent.setup();
    renderMenu({ logout: vi.fn().mockResolvedValue(undefined) });

    await userEvents.click(screen.getByRole('button', { name: 'Account menu' }));
    await userEvents.click(screen.getByRole('menuitem', { name: 'Sign out' }));

    // Let the resolved promise's continuation run first — otherwise the
    // absence of a toast is only true because nothing has happened yet.
    await Promise.resolve();
    expect(toast.error).not.toHaveBeenCalled();
  });
```

- [ ] **Step 3: Run the tests to verify the failure case fails**

```sh
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
npm run -w apps/web test -- src/components/frame/ProfileMenu.test.tsx
```

Expected:
- `warns that the server session may survive when sign-out fails` — **FAIL**: `toast.error` is never called, so the `waitFor` times out. Expect Vitest to also report an unhandled rejection from `void logout()` — that is the current code discarding the promise, and it is exactly what this task removes.
- `raises no toast when sign-out succeeds` — **PASS** already.
- Every pre-existing case in the file — **PASS**.

- [ ] **Step 4: Implement the handler**

In `apps/web/src/components/frame/ProfileMenu.tsx`, add the two imports at the top of the import block:

```tsx
import { toast } from 'sonner';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
```

Add the handler inside the component, after the `showAvatar` line (currently line 32):

```tsx
  // logout() signs out locally in every case and rejects only to report that
  // the server may not have revoked the refresh-token family. Hence no success
  // toast, and copy that says what is still true rather than "sign-out failed".
  //
  // The message is FIXED rather than apiError.message: WriteError redacts the
  // title of every 5xx to "internal server error", and 500 is the only status
  // this path produces, so the house `message || fallback` pattern would show
  // the user that string and never reach the fallback. The ApiError is still
  // built — its `detail` carries anything a gateway or transport error
  // supplied, and sonner omits the description when it is undefined.
  //
  // .catch rather than an async handler because onSelect is Radix's
  // synchronous callback. Raising the toast after RequireAuth has already
  // redirected and unmounted this menu is safe: sonner's toast is a
  // module-level imperative API rendered by the app-root <Toaster>, not
  // component state.
  const handleSignOut = () => {
    logout().catch((err: unknown) => {
      const apiError = createErrorFromUnknown(err);
      toast.error('Signed out on this device, but the server may still have an active session.', {
        description: apiError.detail,
      });
    });
  };
```

and replace the menu item (line 64):

```tsx
        <DropdownMenuItem onSelect={handleSignOut}>Sign out</DropdownMenuItem>
```

- [ ] **Step 5: Run the affected suites to verify they pass**

```sh
npm run -w apps/web test -- src/components/frame/ProfileMenu.test.tsx \
  src/components/AppLayout.test.tsx src/components/admin/AdminLayout.test.tsx
```
Expected: PASS, with no unhandled-rejection report.

- [ ] **Step 6: Commit**

```bash
git add apps/web/src/components/frame/ProfileMenu.tsx \
        apps/web/src/components/frame/ProfileMenu.test.tsx \
        apps/web/src/components/AppLayout.test.tsx \
        apps/web/src/components/admin/AdminLayout.test.tsx
git commit -m "fix(web): surface a failed sign-out through an error toast"
```

---

### Task 5: Full verification

**Files:** none modified unless a gate fails.

- [ ] **Step 1: Run the full CI gate**

```sh
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
make ci
```
Expected: PASS for `lint-check`, `vet`, `test`, `build`, `fe-test`, `fe-build`, `manifests`, `carfax-template`.

`fe-build` runs `tsc -b`, and `tsconfig.app.json` includes all of `src` — so the new test file is type-checked too, under `strict`, `noUnusedLocals`, `noUnusedParameters` and `noUncheckedIndexedAccess`. If `tsc` flags an unused import or an unchecked index access in the new tests, fix it there rather than loosening the config.

- [ ] **Step 2: Walk the acceptance criteria against the evidence**

Confirm each of these has a named test that ran green in Step 1 (no browser run — PRD D3 and design §6.5: the assertions here are control flow and status codes, and jsdom's blindness to CSS does not apply):

| Acceptance criterion | Evidence |
|---|---|
| Logout returns 500 when the family revoke fails, with a JSON:API envelope | `TestLogout_returns500WhenTheFamilyRevokeFails` |
| `Set-Cookie` clears the refresh cookie on the 500 | same test, cookie assertion |
| Logout still returns 204 on success / no cookie / unknown token | `TestLogout_succeedsAndClearsCookie`, `TestLogout_returns204WithNoRefreshToken`, `TestLogout_returns204ForAnUnknownToken` |
| `logoutRequest` rejects with an `ApiError` on a non-ok response | `rejects with an ApiError when the server reports a failure` |
| `logoutRequest` rejects when `fetch` rejects | `rejects when the network request itself fails` |
| `logout()` clears the token and purges the cache even when the request fails | `clears the local session and still rejects when the request fails` |
| `ProfileMenu` toasts on rejection and not on success | `warns that the server session may survive when sign-out fails`, `raises no toast when sign-out succeeds` |

Then confirm the code-level criteria by grep:

```sh
grep -n "catch(() => undefined)\|fetch('/api/auth/logout'" apps/web/src/lib/hooks/api/auth.ts   # expect: no matches
grep -n "void logout()" apps/web/src/components/frame/ProfileMenu.tsx                          # expect: no matches
grep -rn "toast.error" apps/web/src/components/AppLayout.tsx apps/web/src/components/admin/AdminLayout.tsx  # expect: no matches (FR-LOGOUT-14)
```

- [ ] **Step 3: Commit any fixes the gate required**

If Step 1 was clean, there is nothing to commit and this step is a no-op. Otherwise:

```bash
git add -A
git commit -m "fix(task-022): address make ci findings"
```

---

### Task 6: Code review

**Files:** `docs/tasks/task-022-signout-failure-handling/audit.md` (written by the reviewer agents).

CLAUDE.md requires this before a PR, and it is not optional even when the plan looks complete.

- [ ] **Step 1: Request the review**

Invoke `superpowers:requesting-code-review`. Both a Go service and TypeScript/React files changed, so it should dispatch `plan-adherence-reviewer`, `backend-guidelines-reviewer` and `frontend-guidelines-reviewer`. Findings land in `docs/tasks/task-022-signout-failure-handling/audit.md`.

- [ ] **Step 2: Act on the findings**

Use `superpowers:receiving-code-review` — verify each finding against the source before implementing it. Two findings are predictable and must be pushed back on with the reasoning already recorded here rather than silently accepted:
- "Use the house `toast.error(apiError.message || 'fallback')` pattern" — no; see Task 4's rationale. `apiError.message` is the constant `"internal server error"` on this path.
- "The failure is logged twice (handler + `WriteError`)" — accepted deliberately (design §3.4): the handler's line names the operation, `WriteError`'s only knows the status.

- [ ] **Step 3: Re-run the gate and commit**

```sh
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
make ci
git add -A && git commit -m "docs(task-022): record the code-review audit"
```

---

## Out of scope (do not do these)

From PRD §2 — flagging them so a well-meaning implementer does not widen the change:

- "Sign out of all devices" or any session-management UI.
- Retry, backoff, or an offline queue for the logout request. In particular, **no "Try again" action on the toast** — resolved in design §6.4: it pulls in state this feature does not need (what a second failure does, whether a successful retry then confirms, which would reintroduce the success toast FR-LOGOUT-13 removes).
- A `skipRefresh` option on `ApiClient` — rejected in design §4.2: it widens a shared package to guard a path that cannot occur.
- Changes to refresh-token rotation, reuse detection, or family semantics beyond the revoke call's error reporting.
- Reworking the OIDC login flow or `buildLoginUrl`; moving the access token out of `localStorage`.
- Auditing other `void somePromise()` call sites in the app.
