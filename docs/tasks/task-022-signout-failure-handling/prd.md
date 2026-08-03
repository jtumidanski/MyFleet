# Sign-Out Failure Handling — Product Requirements Document

Version: v1
Status: Draft
Created: 2026-08-03
Issue: [#38](https://github.com/jtumidanski/MyFleet/issues/38)
---

## 1. Overview

Signing out of MyFleet cannot fail, as far as the user or any log reader can
tell. Every failure mode in the sign-out path is currently converted into a
success signal at some layer, and by the time the outcome reaches the UI there
is nothing left to report. The user clicks "Sign out", the menu closes, the app
returns them to the login screen — and that identical sequence plays whether the
server revoked their refresh-token family or never received the request at all.

The consequence is a session that is dead in the UI and alive on the wire. The
access token lives in `localStorage` and is cleared client-side, but the refresh
token lives in an `HttpOnly` cookie and can only be invalidated by the server.
When the logout request fails, that cookie survives in the browser and its token
family survives in the database. The visible state says "signed out"; the
recoverable state says "signed in". On a shared machine that gap is the whole
point of clicking the button.

Issue #38 diagnosed this as an unhandled promise rejection at the `ProfileMenu`
call site, on the theory that a rejected `logoutRequest()` would abort `logout()`
partway and strand the user in a signed-in UI. That mechanism is not real —
see §2.1 — but the conclusion that sign-out failures are invisible is correct,
and the true cause sits one layer lower and is the mirror image of the reported
one: failures are not dropped on the way to the user, they are relabelled as
successes before they ever start the journey. This task fixes the relabelling at
both ends, client and server, and then wires the resulting real error to a real
toast.

### 1.1 What the code actually does today

Three independent suppressions stack up, in the order a failure would travel:

**Server — `apps/auth-service/internal/session/resource.go:85-95`.** `logoutHandler`
calls `proc.Logout(raw)`, and on error logs it and continues to
`w.WriteHeader(http.StatusNoContent)`. A failed family revocation and a
successful one are indistinguishable to any client. This is the load-bearing
one: it means no client-side status check, however correct, can ever observe a
revoke failure.

**Client transport — `apps/web/src/lib/hooks/api/auth.ts:64-71`.** `logoutRequest`
bypasses `apiClient` and issues a bare `fetch`, terminated by
`.catch(() => undefined)`. That discards network-level rejections. And because
`fetch` does not reject on non-2xx responses, and the response's `status`/`ok`
are never read, an HTTP 500 or an HTML error page from the gateway is also
consumed as success. The function's only possible outcome is a resolved promise.

**Client call site — `apps/web/src/components/frame/ProfileMenu.tsx:64`.**
`onSelect={() => void logout()}` discards the returned promise. Harmless in
isolation given the two layers above, since there is no rejection left to
discard, but it is the point at which a real error — once the layers below start
producing one — would need to be caught.

### 1.2 Why this is worth fixing beyond the cosmetics

`apiClient.request` (`packages/shared-ts/src/apiClient.ts:39-50`) already
implements exactly the behaviour `logoutRequest` needs: it handles `204` by
skipping the body parse, checks `res.ok`, and throws
`createErrorFromUnknown({ status, body })` on failure. `logoutRequest` predates
or ignores that and hand-rolls a weaker version. The client-side fix is
therefore mostly deletion — dropping a bespoke `fetch` in favour of the shared
client — rather than new machinery. That also brings logout onto the same
`ApiError` type every other call site already surfaces via `toast.error`.

## 2. Goals

Primary goals:

- Make a failed sign-out visible to the user, through the repo's existing
  `createErrorFromUnknown()` + `toast.error()` path.
- Make a failed refresh-token-family revocation visible to the client, by
  ending the auth-service's unconditional `204`.
- Preserve the user's expressed intent — they asked to leave, so the local
  session ends regardless — while being honest that the server session may not
  have.
- Route `logoutRequest` through `apiClient` so logout obeys the same status and
  error contract as every other request in the app.
- Cover both halves with tests that fail against today's code.

Non-goals:

- "Sign out of all devices" / session-management UI.
- Retry, backoff, or an offline queue for the logout request.
- Changes to refresh-token rotation, reuse detection, or family semantics
  beyond the revoke call's error reporting.
- Reworking the OIDC login flow or `buildLoginUrl`.
- Any change to how the access token is stored (`localStorage` stays).
- Auditing other `void somePromise()` call sites in the app. FE-09 may well flag
  more; they are out of scope here.

### 2.1 Correcting the issue's stated mechanism

Issue #38 states that when `logoutRequest()` rejects, "the three lines after it
never run — so the access token is *not* cleared and the query cache is *not*
purged", leaving the user signed in.

This cannot occur. `logoutRequest`'s trailing `.catch(() => undefined)` means it
never rejects, so `logout()` never rejects, so the statements after the `await`
always run. The described symptom — user remains signed in with no feedback —
is not reachable, and a fix that only wraps the `ProfileMenu` call site in a
`try/catch` would change nothing observable.

The defect is the inverse and is recorded as such throughout this document. The
issue's *remedy* (`createErrorFromUnknown()` + `toast.error()`, plus a test) is
still the right shape; it just has to be preceded by removing the suppressions
that guarantee there is never anything to catch.

## 3. User Stories

- As a user on a shared machine, I want to be told when signing out did not
  fully take effect, so that I can take another step (close the browser, retry,
  use a different machine) instead of walking away from a live session.
- As a user, I want my sign-out to end my local session even when the server is
  unreachable, so that a network problem cannot force me to stay signed in.
- As a user, I want the sign-out error message to tell me what is actually still
  true — that the server may still consider me signed in — rather than a generic
  "something went wrong".
- As an operator, I want a failed token-family revocation to produce a non-2xx
  response, so that it shows up in gateway metrics and alerting rather than only
  in a log line nobody is reading.
- As a developer, I want logout to use `apiClient` like every other request, so
  that there is one status-and-error contract to reason about.

## 4. Functional Requirements

### 4.1 Transport (`apps/web/src/lib/hooks/api/auth.ts`)

- **FR-LOGOUT-1** — `logoutRequest()` MUST issue its request through
  `apiClient.request`, not a bare `fetch`.
- **FR-LOGOUT-2** — `logoutRequest()` MUST NOT contain a `.catch()` that
  converts a failure into a resolved promise.
- **FR-LOGOUT-3** — `logoutRequest()` MUST reject with an `ApiError` when the
  response status is not ok, and MUST reject when the underlying `fetch`
  rejects (offline, DNS failure, connection reset).
- **FR-LOGOUT-4** — The request MUST continue to send the refresh cookie. The
  `credentials: 'include'` option MUST be preserved; it reaches `fetch` through
  `apiClient`'s `init` spread (`packages/shared-ts/src/apiClient.ts:26-33`).
- **FR-LOGOUT-5** — A `204 No Content` response MUST be treated as success and
  MUST NOT trigger a body-parse error. `apiClient.request` already special-cases
  `204`; no additional handling is required, but the behaviour must be retained.

### 4.2 Context (`apps/web/src/context/AuthContext.tsx`)

- **FR-LOGOUT-6** — `logout()` MUST clear the access token, set `hasToken` to
  false, and remove the `authKeys.all` queries **whether or not**
  `logoutRequest()` rejected. The user asked to leave; a server failure does not
  revoke that request. (Decision D1.)
- **FR-LOGOUT-7** — After performing the local teardown, `logout()` MUST
  propagate the failure to its caller, so the UI layer can decide how to present
  it. `logout()` therefore rejects on a server failure *and* has fully signed the
  user out locally — both, not either.
- **FR-LOGOUT-8** — The local teardown MUST run even if it is the `logoutRequest`
  call that threw, i.e. it belongs in a `finally`-equivalent position rather than
  after the `await` on the happy path only.
- **FR-LOGOUT-9** — The `AuthContextValue.logout` signature stays
  `() => Promise<void>`. No new context fields.

### 4.3 Presentation (`apps/web/src/components/frame/ProfileMenu.tsx`)

- **FR-LOGOUT-10** — The "Sign out" item MUST NOT discard the `logout()` promise
  with a bare `void`. It MUST attach rejection handling.
- **FR-LOGOUT-11** — On failure the component MUST call
  `createErrorFromUnknown(err)` and surface the result through `toast.error`,
  matching the established pattern at e.g.
  `apps/web/src/components/features/settings/FleetNameForm.tsx:34-35`.
- **FR-LOGOUT-12** — The message MUST state that the local session ended but the
  server session may not have, rather than implying sign-out failed outright.
  The fallback string when `apiError.message` is empty MUST convey the same. A
  message of the shape *"Signed out on this device, but the server may still
  have an active session."* satisfies this; exact wording is the implementer's
  call within that constraint.
- **FR-LOGOUT-13** — There MUST be no success toast. A silent, successful
  sign-out that lands on the login screen is already unambiguous feedback.
- **FR-LOGOUT-14** — `ProfileMenu` is imported by both shells
  (`AppLayout`/`AdminLayout`) and MUST remain the single call site; the fix MUST
  NOT be duplicated into the layouts.

### 4.4 Server (`apps/auth-service/internal/session/resource.go`)

- **FR-SRV-1** — When `proc.Logout(raw)` returns a non-nil error,
  `logoutHandler` MUST respond with a 5xx via `server.WriteError`, not `204`.
  `server.StatusFor` maps any unmapped error to `500`
  (`packages/shared-go/server/errors.go:39-41`), so passing the revoke error
  through yields `500` with a JSON:API error envelope.
- **FR-SRV-2** — The existing `log.WithError(err).Error("logout revoke family")`
  line MUST be retained. The response is for the client; the log is for the
  operator.
- **FR-SRV-3** — `clearRefreshCookie(w, cookieSecure)` MUST still be called on
  the failure path, and MUST be called **before** `server.WriteError`. Two
  reasons: clearing the browser's copy is strictly risk-reducing even when the
  family survives in the database, and `http.SetCookie` writes a header, which
  is only possible before `WriteHeader` is called. Getting this order wrong
  silently drops the `Set-Cookie` header.
- **FR-SRV-4** — Idempotency MUST be preserved. A request with no refresh token
  still returns `204` without calling the processor, and an unknown token still
  returns `204` because `Processor.Logout` maps `ErrNotFound` to `nil`
  (`apps/auth-service/internal/session/processor.go:136-145`). Neither is a
  failure and neither may start returning 5xx.
- **FR-SRV-5** — The success path is unchanged: `204 No Content`, cookie
  cleared.

### 4.5 Behaviour matrix

| Scenario | Server response | Local token cleared | Cache purged | Toast |
|---|---|---|---|---|
| Revoke succeeds | `204` | yes | yes | none |
| No refresh cookie present | `204` | yes | yes | none |
| Unknown/expired refresh token | `204` | yes | yes | none |
| `RevokeFamily` errors (DB down) | `500` | yes | yes | error |
| Gateway 5xx / auth-service down | `502`/`503` | yes | yes | error |
| Browser offline, `fetch` rejects | — | yes | yes | error |

The "local token cleared" column is `yes` in every row by FR-LOGOUT-6. That is
the deliberate decision, not an oversight — see D1.

## 5. API Surface

### `POST /api/auth/logout`

Public route, no JWT required — the refresh token authenticates the caller
(`resource.go:26-36`). Request body is optional; the refresh token is read from
the `refresh_token` cookie, falling back to a JSON body
`{"refreshToken": "..."}` (`readRefreshToken`, `resource.go:98-113`).

**Before**

| Condition | Status |
|---|---|
| Any | `204 No Content` |

**After**

| Condition | Status | Body |
|---|---|---|
| Token revoked, or nothing to revoke | `204 No Content` | empty |
| `proc.Logout` returned an error | `500 Internal Server Error` | JSON:API error envelope via `server.WriteError` |

`Set-Cookie` clearing the refresh cookie is emitted on **both** paths.

This is a widening of the response contract, not a breaking change: no existing
client distinguishes statuses today, and the only consumer is this app.

## 6. Data Model

No changes. No new entities, fields, or migrations. Token-family revocation
semantics are untouched — only the reporting of a failed revocation changes.

## 7. Service Impact

**`apps/web`** — three files, all small:

- `src/lib/hooks/api/auth.ts` — `logoutRequest` rewritten onto `apiClient`;
  bespoke `fetch` and swallowing `.catch` deleted.
- `src/context/AuthContext.tsx` — `logout` restructured so teardown always runs
  and the error still propagates.
- `src/components/frame/ProfileMenu.tsx` — `void logout()` replaced with
  rejection handling that raises the toast.

**`apps/auth-service`** — one file:

- `internal/session/resource.go` — `logoutHandler` returns 5xx on revoke
  failure, after clearing the cookie.

**Not affected:** `packages/shared-ts` (`apiClient` and `createErrorFromUnknown`
already do what is needed), `packages/shared-go` (`WriteError`/`StatusFor`
already map unmapped errors to 500), `deploy/k8s`, every other service.

## 8. Non-Functional Requirements

**Security.** The residual risk this task narrows: today a user who believes
they have signed out may leave behind a live refresh-token family *and* a live
browser cookie, with no signal to anyone. After this change the family-revoke
failure is reported to the client and to gateway metrics, the browser cookie is
cleared on every path, and the user is told when the server side is uncertain.

The gap that remains after this task, and is accepted: when the request never
reaches the server (offline), the browser's `HttpOnly` refresh cookie cannot be
cleared by JavaScript, so it persists until it expires or the browser is closed.
Anyone able to run script on the origin could then `POST /api/auth/refresh` and
mint a fresh access token. Mitigating that requires a durable client-side
"pending logout" that retries — explicitly a non-goal. The toast copy in
FR-LOGOUT-12 exists precisely because this residue is real and the user is the
only one positioned to act on it.

**Observability.** The failed-revoke log line is retained (FR-SRV-2) and is now
accompanied by a 5xx, so the condition becomes visible in request metrics rather
than log-only.

**Performance.** Irrelevant at this scale — one request per sign-out, no new
round-trips, no new state.

**Accessibility.** The error surfaces through the app's existing `sonner`
toaster; no new UI primitive is introduced and no ARIA work is required.

**Backward compatibility.** An older web build talking to the new auth-service
ignores the 500 exactly as it ignores everything else — no worse than today. A
new web build against an old auth-service simply never sees a revoke-failure
500, only transport failures.

## 9. Open Questions

1. **Should the failure toast offer a "Try again" action?** It is the natural
   remedy for the state the toast describes, and `sonner` supports an action
   button. Deferred rather than specified: the chosen behaviour is "warn
   loudly", not "warn and retry", and a retry after the local token is already
   cleared would have to re-send the cookie alone — which does work, since the
   route is public and reads the cookie, but the interaction deserves its own
   design pass. Resolve during design.
2. **Should `logout()` reject at all, or return a result value?** FR-LOGOUT-7
   specifies rejection because it matches every other call site's
   `try/catch` + `createErrorFromUnknown` shape. A `{ ok, error }` return would
   avoid a promise that both succeeds in effect and rejects in signal, which is
   an odd contract to document. Weigh in design.
3. **Does anything else call `logout()`?** The 401-handling path in
   `AuthContext`'s `useEffect` (`me.isError`) clears the token directly rather
   than calling `logout()`. Confirm during implementation that `ProfileMenu` is
   the sole caller, so FR-LOGOUT-7's rejection cannot reach an unprepared
   caller and become the unhandled rejection this task is meant to remove.

## 10. Acceptance Criteria

Behaviour:

- [ ] With the auth-service stopped, clicking "Sign out" shows an error toast,
      clears the access token, and lands on the login screen.
- [ ] With the auth-service healthy, clicking "Sign out" shows **no** toast and
      lands on the login screen.
- [ ] `POST /api/auth/logout` returns `500` when the family revoke fails, and
      the response carries a JSON:API error envelope.
- [ ] `POST /api/auth/logout` still returns `204` with no cookie, with an
      unknown token, and on success.
- [ ] The refresh cookie is cleared by the `Set-Cookie` header on both the
      `204` and the `500` response.

Code:

- [ ] `logoutRequest` contains no bare `fetch` and no `.catch(() => undefined)`.
- [ ] `logout()`'s teardown runs on both the success and failure paths.
- [ ] `ProfileMenu.tsx` contains no `void logout()`.
- [ ] The fix exists once, in `ProfileMenu`, not duplicated into either layout.

Tests — each must fail against the current code:

- [ ] Vitest: `logoutRequest` rejects with an `ApiError` on a non-ok response.
- [ ] Vitest: `logoutRequest` rejects when `fetch` rejects.
- [ ] Vitest: `logout()` clears the token and purges the cache even when the
      request fails.
- [ ] Vitest: `ProfileMenu` calls `toast.error` when `logout()` rejects, and
      does not when it resolves.
- [ ] Go: `logoutHandler` returns 500 when the processor's revoke errors.
- [ ] Go: `logoutHandler` returns 204 on success, with no cookie, and with an
      unknown token.
- [ ] Go: the `Set-Cookie` clearing header is present on the 500 response.

Verification:

- [ ] `make ci` passes.

## Appendix A — Decisions taken at spec time

**D1 — On failure, clear the local session anyway and warn.** Considered and
rejected: aborting the sign-out to keep client and server consistent. Rejected
because the failure mode that matters is the shared machine, and refusing to
sign out locally is the worst outcome there — it leaves the session live in
*both* places, and does so specifically when the user has just told us they are
leaving. Clearing locally shrinks the exposure to "a cookie that a script on
this origin could still trade for a token", which is strictly better, and the
toast is what makes that residue known rather than hidden.

**D2 — The backend is in scope.** The frontend fix is close to pointless
without it: `logoutHandler` returning an unconditional `204` means a correct
client-side status check would be checking a value that is constant by
construction. Fixing only the client would surface transport failures while
leaving the failure the whole feature is about — the revoke that did not
happen — as invisible as it is today.

**D3 — Unit tests on both sides, no Playwright.** The assertions here are about
control flow and status codes, not rendering or layout, so jsdom's blindness to
CSS does not apply. A browser test would add cost without covering anything the
unit tests miss.
