# Transient Upstream Failures Must Not Log Users Out — Product Requirements Document

Version: v1
Status: Draft
Created: 2026-08-03
Issue: [#15](https://github.com/jtumidanski/MyFleet/issues/15)
Parent defect: #6 · Predecessor: #14 (task-008)
---

## 1. Overview

`auth-service`'s refresh handler resolves a complete `session.Principal` before
minting an access token: email from the local `users` table, active fleet and
role from `fleet-service`. That resolution has two failure modes with opposite
correct responses, and the handler currently collapses them into one.

A **permanent** failure — the user row is gone, `user.ErrNotFound` from
`users.GetByID` — genuinely means the credential is dead. `401` plus clearing
the refresh cookie is right. A **transient** failure — `fleet-service` is down,
restarting, or erroring — is an upstream outage, not an authentication failure.
Answering `401` and clearing the cookie converts someone else's brief outage
into a forced logout: the user must re-authenticate through Google rather than
simply retrying.

The blast radius is total. Access tokens live 15 minutes, and the SPA refreshes
on any `401` (`packages/shared-ts/src/apiClient.ts:35-37`), so within one token
lifetime every active session hits the refresh path. A `fleet-service` restart
that outlasts a single request logs out the entire user base.

This became reachable in #14. Before it, `membership.Client.Active`
special-cased only `404`; every other non-2xx had its JSON body decoded into an
empty `Membership` with `err == nil`. `fleet-service`'s error envelope *is*
JSON, so a `500` silently produced a fleet-less `Principal` and a *successful*
refresh — a different and worse bug, since the user got a valid token claiming
no fleet and the SPA sent them to `/onboarding` to create a duplicate. #14
fixed that by failing closed
(`apps/auth-service/internal/membership/client.go:50-52`). That fix is correct
and stays. Its consequence — transient failures now reach the
`401`-and-clear-cookie path — was accepted at the time on the grounds that the
path was mostly unreachable. It no longer is. This task pays that debt.

## 2. Goals

Primary goals:

- Distinguish transient upstream failures from permanent identity failures at
  the point where the distinction is known (`membership.Client`), and carry that
  distinction to every caller of the principal resolver.
- Return `503` with `Retry-After` from `POST /auth/refresh` on a transient
  failure, **preserving the refresh cookie** — the session is still valid.
- Keep `401` + clear-cookie for the permanent case, unchanged.
- Bound the `auth → fleet` membership lookup with a timeout, so a wedged
  upstream becomes a classifiable transient error instead of an indefinitely
  pinned handler.
- Stop the SPA treating a refresh `503` as a dead session.

Non-goals:

- Retry logic, backoff, or a circuit breaker inside `membership.Client`. One
  attempt, classified honestly, is the scope here.
- Automatic client-side retry of a failed refresh, or any "reconnecting…"
  outage UI. The SPA must stop *destroying* the session on `503`; riding out an
  outage invisibly is a separate concern.
- Any change to `fleet-service`, including its error envelope or status codes.
- Changing the `404 → (Membership{}, nil)` contract. See FR-CLASSIFY-4.
- Revisiting #14's fail-closed decision.

## 3. User Stories

- As a signed-in user, I want a brief `fleet-service` outage to leave me signed
  in, so that I am not thrown back through a Google sign-in for a problem that
  had nothing to do with my credentials.
- As a signed-in user whose account has genuinely been removed, I want to be
  signed out promptly, so that my browser stops presenting a credential that can
  only ever fail.
- As a brand-new user with no fleet yet, I want my first sign-in to route me to
  onboarding, so that I can create a fleet — unchanged by any of this.
- As a user signing in during a `fleet-service` outage, I want the login page to
  tell me the service is temporarily unavailable and to try again, rather than
  showing a generic failure that implies my account is broken.
- As an operator, I want a `fleet-service` outage to be visible in `auth-service`
  logs and status codes as an upstream failure, so that I diagnose the outage
  rather than a phantom wave of authentication failures.

## 4. Functional Requirements

### 4.1 Error classification (`membership` package)

- **FR-CLASSIFY-1** — `membership.Client.Active` MUST return an error that
  callers can classify as *transient* (upstream unavailable; the answer is
  unknown) or leave unclassified (a definitive negative answer).
- **FR-CLASSIFY-2** — These MUST classify as transient:
  - transport failure from `hc.Do` (connection refused, DNS failure, TLS error);
  - context deadline exceeded, including the FR-TIMEOUT-1 timeout;
  - any response status `>= 500`;
  - `429 Too Many Requests`.
- **FR-CLASSIFY-3** — A `4xx` status other than `404` and `429` (e.g. `400`,
  `403`) MUST NOT classify as transient. These indicate a contract or
  authorization fault between the two services that retrying will not fix; they
  keep today's fail-closed behaviour and reach the permanent path.
- **FR-CLASSIFY-4** — `404` MUST continue to return `(Membership{}, nil)`. This
  is load-bearing: the OIDC callback keys its onboarding redirect off the
  resulting empty `ActiveFleetID`, so turning "no membership" into an error
  breaks a brand-new user's first login.
  `apps/auth-service/cmd/main_test.go`'s
  `TestNewPrincipalResolver_treatsNoMembershipAsEmptyNotError` guards this and
  MUST still pass unmodified.
- **FR-CLASSIFY-5** — A JSON decode failure on a 2xx body MUST classify as
  transient. A 2xx with an unparseable body is a garbled or truncated response,
  not a definitive answer.
- **FR-CLASSIFY-6** — Classification MUST NOT widen what reaches logs or
  response bodies. Today's messages carry the status code and a fixed
  description only, deliberately: the upstream body is attacker-influenceable
  and the user id must not ride into a log line as an address. Any new error
  type MUST preserve that.
- **FR-CLASSIFY-7** — The classification MUST be inspectable through
  `errors.Is`/`errors.As` and MUST survive being wrapped by the principal
  resolver, so `session` and `oidc` can classify without importing the concrete
  `membership` client. The resolver is injected precisely so those packages stay
  decoupled (`session.PrincipalResolver`), and this task MUST NOT break that.
- **FR-CLASSIFY-8** — `user.ErrNotFound` from `users.GetByID` MUST NOT classify
  as transient. A missing user row is the permanent case.

### 4.2 Lookup timeout

- **FR-TIMEOUT-1** — `membership.Client.Active` MUST bound its request with a
  5-second timeout, matching `fleetMemberLookupTimeout` already applied to its
  sibling `FleetMemberIDs`. Today `Active` has none, and `Client` shares
  `http.DefaultClient`, which has no timeout either — a wedged `fleet-service`
  pins an `auth-service` refresh handler open indefinitely. Prefer a single
  shared constant over a second literal.
- **FR-TIMEOUT-2** — A timeout firing MUST classify as transient (FR-CLASSIFY-2),
  producing a `503` rather than a logout. This is the requirement that makes a
  *hang* — the worst and most likely outage shape — covered by the acceptance
  criteria rather than merely the *error* shape.

### 4.3 Shared 503 support (`packages/shared-go/server`)

- **FR-SHARED-1** — Add `ErrServiceUnavailable` to the sentinel set in
  `errors.go`, mapped to `503` by `StatusFor`. The package currently has no
  sentinel above `429`; unknown errors fall through to `500`.
- **FR-SHARED-2** — `codeFor` (`server.go:12`) MUST return
  `"service_unavailable"` for `503` rather than falling through to
  `"internal_error"`, so the SPA can key on `body.errors[0].code`.
- **FR-SHARED-3** — `WriteError` MUST be able to emit a `Retry-After` response
  header. The header MUST be set before the status line is written, since
  `WriteJSON` commits the header block. Follow the existing `Detailed` /
  `Detail() string` pattern — a wrapper error carrying the value, surfaced to
  `WriteError` via an `errors.As` interface check — rather than adding a
  parallel `WriteErrorWith…` entry point; the exact shape is a design decision.
- **FR-SHARED-4** — The `503` body MUST keep the existing `>= 500` redaction:
  `Title` stays `InternalErrorTitle` and no `detail` is emitted. The reason for
  the outage is upstream-controlled and does not belong in a response to an
  unauthenticated caller. The existing `>= 500` error log line still fires.
- **FR-SHARED-5** — The sentinel-to-status table test in `errors_test.go` MUST
  be extended to cover `503`, and the code-mapping test to cover
  `service_unavailable`.
- **FR-SHARED-6** — No existing status mapping, code string, or envelope shape
  may change. Every current caller of `WriteError` MUST be unaffected.

### 4.4 Refresh handler (`POST /auth/refresh`)

- **FR-REFRESH-1** — On a *transient* resolver error the handler MUST respond
  `503` with a `Retry-After: 5` header.
- **FR-REFRESH-2** — On a transient error the handler MUST NOT call
  `clearRefreshCookie`. The refresh token is still valid and the user must be
  able to retry with it. This is the single most important behavioural change in
  the task.
- **FR-REFRESH-3** — On a transient error the handler MUST NOT mint an access
  token and MUST NOT return a token document. Failing closed (FR-5 of task-008)
  is unchanged: a token with incomplete identity is never minted.
- **FR-REFRESH-4** — On a *permanent* resolver error the handler MUST continue
  to clear the refresh cookie and respond `401`, exactly as today.
- **FR-REFRESH-5** — Token rotation has already happened by the time the
  resolver runs (`proc.Rotate` precedes `resolve`). A `503` therefore leaves the
  client holding a **rotated** cookie whose new value was written by
  `SetRefreshCookie`… except that on the current error path `SetRefreshCookie`
  is never reached, so the browser keeps the **old** value while the store has
  advanced. Presenting the old value again is indistinguishable from replay, and
  `Processor.Rotate` revokes the whole token family on reuse — which would log
  the user out anyway, defeating the entire task. The design MUST resolve this:
  either persist the rotated value to the cookie before responding `503`, or
  resolve the principal before rotating. **A `503` that is followed by a
  family-revoking replay on the user's next attempt satisfies none of the
  acceptance criteria.** This is the highest-risk item in the task and the
  design phase MUST address it explicitly.
- **FR-REFRESH-6** — Transient failures MUST be logged distinguishably from
  permanent ones, so an outage does not read as a wave of authentication
  failures.
- **FR-REFRESH-7** — Every other refresh failure path is unchanged: missing
  token, `ErrNotFound`, `ErrTokenReuse`, `ErrTokenExpired`, and mint failure all
  keep their current status and cookie behaviour.

### 4.5 OIDC callback (`GET /auth/callback`)

> **Correction to the issue text.** #15 states the callback "currently answers
> `500` on any resolver error." It does not. `apps/auth-service/internal/oidc/resource.go:262-267`
> calls `failLogin(…, errServerError)`, which `302`-redirects the browser to
> `AppBaseURL + LoginPath + "#error=server_error"` (`resource.go:121-123`). The
> endpoint's response is a browser redirect, not an API response, so
> distinguishing transient failures here means a new `loginErrorCode` — not a
> `503` status. The intent of the decision (distinguish transient everywhere the
> resolver is called) is preserved; the mechanism follows the existing code.

- **FR-CALLBACK-1** — Add a `service_unavailable` value to the `loginErrorCode`
  set (`resource.go:88-93`) and redirect with it when `d.Resolve` fails
  transiently. All other `failLogin` call sites keep `errServerError`.
- **FR-CALLBACK-2** — `errServerError` remains the response for a *permanent*
  resolver failure and for every non-resolver failure.
- **FR-CALLBACK-3** — The callback MUST NOT clear the state cookie differently
  than it does today. `failLogin`'s deliberate non-clearing behaviour and the
  single unconditional clear after `verifyStateCookie` are documented at
  `resource.go:95-112` as a reviewed departure from task-010's FR-ERR-8. Do not
  disturb it.
- **FR-CALLBACK-4** — On this path no refresh token has been issued yet, so
  there is no cookie-preservation concern equivalent to FR-REFRESH-2.

### 4.6 SPA

- **FR-SPA-1** — `apps/web/src/lib/auth/loginError.ts` MUST accept
  `service_unavailable` in `LoginErrorCode`, in the `CODES` allowlist, and in the
  `NOTICES` table. An unrecognised code still degrades to `server_error`
  (`loginError.ts:58`), so shipping the backend ahead of the frontend is safe.
- **FR-SPA-2** — `service_unavailable` MUST render its own message —
  substantively "Sign-in is temporarily unavailable. Nothing was saved — try
  again in a moment." — rather than reusing `GENERIC_FAILURE`. Unlike the
  `invalid_state`/`auth_failed`/`server_error` split, which exists for log
  correlation and which the user cannot act on differently, this one changes the
  advice: wait and retry, rather than try a different Google account. Tone is a
  design decision; `LoginErrorNotice.tone` currently admits only
  `'neutral' | 'danger'`.
- **FR-SPA-3** — A `503` from `POST /api/auth/refresh` MUST NOT clear the stored
  access token. Today `refresh.ts:26` returns `null` for every non-`ok`
  response, and `refreshAccessToken` (`refresh.ts:57-61`) then calls
  `clearAccessToken()`; that empties the token, `useAuth().isAuthenticated` goes
  false, and `RequireAuth` (`RequireAuth.tsx:38`) navigates to `/login`. **That
  chain is the actual logout mechanism** and FR-SPA-3 is what breaks it.
- **FR-SPA-4** — A `503` refresh MUST surface to the caller as a retryable
  failure, distinguishable from "the session is dead". `mintAccessToken` and
  `refreshAccessToken` currently return `Promise<string | null>`, which cannot
  express the difference; the design must choose how to widen that without
  disturbing the shared in-flight dedupe at `refresh.ts:16-31` (concurrent
  refreshes must still collapse onto one request — two POSTs with the same
  cookie trip reuse detection and revoke the family).
- **FR-SPA-5** — `ApiClient.fetchAuthenticated`
  (`packages/shared-ts/src/apiClient.ts:18-38`) MUST NOT change its one-shot
  `401`-refresh-and-retry contract for any status other than the refresh `503`
  case, and MUST NOT introduce a retry loop.
- **FR-SPA-6** — When the original request `401`s and the refresh answers `503`,
  the original request MUST fail with a retryable `ApiError` carrying status
  `503`, not a `401`. A `401` would be a lie — the caller's credentials were
  never the problem — and downstream code keys on status.
- **FR-SPA-7** — `mintAccessToken`'s existing contract is unchanged: a failed
  mint leaves the current access token alone. Only `refreshAccessToken`'s
  clear-on-failure becomes conditional.

## 5. API Surface

No new endpoints. Two modified responses.

### `POST /auth/refresh`

| Condition | Status | `Retry-After` | Refresh cookie | Change |
| --- | --- | --- | --- | --- |
| Success | `200` | — | rotated | unchanged |
| No token presented | `401` | — | untouched | unchanged |
| Rotate failed (unknown / expired / reuse) | `401` | — | cleared | unchanged |
| Resolver: user row gone | `401` | — | cleared | unchanged |
| Resolver: non-retryable upstream 4xx | `401` | — | cleared | unchanged |
| **Resolver: upstream unavailable** | **`503`** | **`5`** | **preserved (see FR-REFRESH-5)** | **new** |
| Mint failed | `401` | — | untouched | unchanged |

`503` body — standard envelope, redacted per FR-SHARED-4:

```json
{ "errors": [ { "status": "503", "code": "service_unavailable", "title": "Internal Server Error" } ] }
```

`Retry-After: 5` is advisory. Per the non-goals the SPA does not auto-retry; the
header is for correctness, intermediaries, and any future client. Five seconds
is chosen to match FR-TIMEOUT-1 and the restart timescale of a `fleet-service`
pod.

### `GET /auth/callback`

Unchanged shape — always a `302` to `AppBaseURL + LoginPath + "#error=<code>"`
on failure. One new value in the closed `<code>` set: `service_unavailable`.

### Internal: `GET /internal/memberships/active?user_id=…` (fleet-service)

Unconsumed contract change only — `auth-service` now interprets the existing
statuses differently. `fleet-service` is not modified.

## 6. Data Model

No entities, fields, relationships, constraints, or migrations. This task is
entirely error classification and response shaping.

## 7. Service Impact

**`packages/shared-go/server`** — new `ErrServiceUnavailable` sentinel;
`StatusFor` and `codeFor` extended; `WriteError` gains `Retry-After` emission.
Additive only. Shared by every Go service, so the sentinel-table tests are the
regression net.

**`apps/auth-service`**
- `internal/membership/client.go` — typed transient error from `Active`; 5s
  timeout. `FleetMemberIDs` is untouched (its callers do not need the
  distinction), though the timeout constant is shared.
- `internal/session/resource.go` — `refreshHandler` splits the resolver error
  path; the cookie-vs-rotation problem in FR-REFRESH-5 lands here.
- `internal/oidc/resource.go` — new `loginErrorCode`; resolver error path splits.
- `cmd/main.go` — `newPrincipalResolver` must propagate classification through
  its wrapping (FR-CLASSIFY-7).

**`packages/shared-ts`** — `apiClient.ts` refresh-failure handling; possibly
`errors.ts` if a retryable marker is added to `ApiError`.

**`apps/web`** — `lib/api/refresh.ts` (the clear-on-failure decision);
`lib/auth/loginError.ts` (new code + message).

**`fleet-service`** — none.

**`deploy/k8s`** — none. No new configuration, environment variables, or
manifests. Both overlays must still render and pass server dry-run, per
`CLAUDE.md`.

## 8. Non-Functional Requirements

**Security**
- The `503` body stays redacted (FR-SHARED-4). `/auth/refresh` is a public
  endpoint; an unauthenticated caller learns only that something upstream is
  unavailable.
- No user id, upstream response body, or upstream error text may enter a log
  message as an address or free text (FR-CLASSIFY-6).
- Preserving the refresh cookie on `503` MUST NOT weaken revocation: a cookie
  the store has revoked (reuse, logout, expiry) still fails at `Rotate`, which
  runs before the resolver.
- `503` MUST NOT become a way to keep a dead session alive. It is reachable only
  from a transient *upstream* failure, never from a token-validity failure.

**Reliability**
- `FR-TIMEOUT-1` bounds the `auth → fleet` hop, so `auth-service` cannot be
  exhausted by a wedged `fleet-service`.
- One attempt per refresh. No retry storm against an already-struggling upstream
  (a direct consequence of the no-retry non-goals).

**Observability**
- Transient and permanent resolver failures MUST be distinguishable in logs
  (FR-REFRESH-6), so an outage reads as an outage.
- `503` responses are logged by the existing `>= 500` branch in `WriteError`.

**Performance** — no measurable change on the success path; classification is a
type check on an already-allocated error.

**Compatibility** — the backend can ship before the frontend: an unknown login
error code degrades to `server_error` (FR-SPA-1), and an unhandled `503` from
refresh is no worse than today's `401`.

## 9. Open Questions

1. **FR-REFRESH-5 — rotation vs. cookie on the `503` path.** The rotated token
   is committed to the store before the resolver runs, but the current error
   path never writes the new value to the cookie. Resolve-before-rotate is the
   cleaner ordering but moves an upstream call ahead of the cheap local token
   check; write-cookie-then-`503` keeps the ordering but returns an error
   response bearing a `Set-Cookie`. Design must pick one, with the reuse-
   detection consequence stated explicitly.
2. **How `refresh.ts` expresses "retryable" (FR-SPA-4).** Widening
   `Promise<string | null>` to a result object, throwing a typed error, or
   exposing a separate signal — each interacts differently with the in-flight
   dedupe and with `ApiClient`'s `onRefresh: () => Promise<string | null>`
   option type, which is part of `shared-ts`'s public surface.
3. **Tone for the `service_unavailable` login notice (FR-SPA-2).**
   `LoginErrorNotice.tone` admits only `'neutral' | 'danger'`. A temporary
   outage is arguably neither; adding a third tone is a design-system change
   beyond this task's remit, so `'neutral'` may be the pragmatic answer.
4. **Does `429` warrant its own handling?** FR-CLASSIFY-2 folds it into
   transient. `fleet-service` does not currently rate-limit its internal
   endpoints, so this is forward-looking; a distinct `Retry-After` passthrough
   is out of scope unless design finds it cheap.
5. **Should `403` from `fleet-service` alarm louder?** FR-CLASSIFY-3 routes it
   to the permanent path, which logs a user out for what is really a service
   misconfiguration. Correct per "fail closed," but arguably deserves a distinct
   log signal so it is not mistaken for ordinary account deletion.

## 10. Acceptance Criteria

Behavioural — these restate the issue's acceptance list and are the bar:

- [x] With `fleet-service` returning `500`, `POST /auth/refresh` responds `503`
      with `Retry-After: 5`, does not clear the refresh cookie, and does not log
      the user out.
- [x] With `fleet-service` unreachable (connection refused), the same holds.
- [x] With `fleet-service` hanging past the timeout, the same holds — the
      request completes in ~5s rather than pinning the handler (FR-TIMEOUT-1/2).
- [x] After a `503`, a subsequent refresh once `fleet-service` recovers succeeds
      and does **not** trip reuse detection or revoke the token family
      (FR-REFRESH-5).
- [x] A missing user row still returns `401` and still clears the cookie.
- [x] A membership `404` still resolves to an empty `ActiveFleetID`; a brand-new
      user's first login still lands on `/onboarding`.
- [x] `fleet-service` returning `403` still reaches the `401` + clear path.
- [x] An OIDC callback during a `fleet-service` outage redirects with
      `#error=service_unavailable` and the login page shows the try-again-shortly
      message, not the generic failure.
- [x] The SPA does not redirect to `/login` when a refresh answers `503`; the
      stored access token survives and the failed request surfaces a `503`
      `ApiError`, not a `401`. (Verified for an in-session refresh — a
      client-side navigation while the app is already mounted, per
      `verification.md` Check 1. Token survival holds unconditionally, but a
      **cold page reload** during the outage still lands on `/login`, because
      `AuthContext.tsx`'s `isAuthenticated = hasToken && !!data?.user` is
      false until `useMe` resolves; see `follow-ups.md`.)

Code and verification:

- [x] `TestNewPrincipalResolver_treatsNoMembershipAsEmptyNotError` passes
      unmodified.
- [x] `shared-go/server` sentinel-status and code-mapping table tests cover
      `503` / `service_unavailable`; no existing mapping changed.
- [x] `membership.Client.Active` has table-driven tests over transport error,
      timeout, `500`, `429`, `403`, `404`, malformed 2xx body, and success —
      each asserting the classification, not just that an error occurred.
- [x] `refreshHandler` tests assert **status, `Retry-After`, and the presence or
      absence of a cookie-clearing `Set-Cookie`** for both the transient and the
      permanent path. Asserting status alone would pass while still logging the
      user out.
- [x] Each new test is proven able to fail: revert the fix, watch it go red,
      restore. Assert on stored/observable state, not merely on a response code
      that happens to match.
- [x] `make ci` passes (lint-check, vet, test, build, fe-test, fe-build).
- [x] Both overlays render, and both server dry-runs pass:
      `kustomize build deploy/k8s/overlays/main | kubectl apply --dry-run=server -f -`
      and the same for `overlays/local`. (No manifest change is expected; this
      is the standing gate.)
- [x] Code review run before the PR — `plan-adherence-reviewer`,
      `backend-guidelines-reviewer`, `frontend-guidelines-reviewer`.
- [ ] Issue #15 closed by the PR.

Note on browser verification: `jsdom` cannot evaluate CSS, and the login-page
notice and redirect-suppression behaviours are user-visible. The FR-SPA
acceptance items should be confirmed against real Chromium via the local stack
(see `docs/runbooks/local-debugging.md`), not on the unit suite alone.
