# Task 018 — Implementation Context

Companion to [`plan.md`](./plan.md). Everything an implementer needs that is not
in the plan's task steps: where the code lives, what the surrounding invariants
are, and which mistakes this area has already made once.

---

## 1. The defect in one paragraph

`auth-service`'s refresh handler resolves a complete `session.Principal` before
minting an access token, and treats *every* resolver failure as a dead
credential: `401` plus a cookie-clearing `Set-Cookie`. A `fleet-service` outage
is not a dead credential. Access tokens live 15 minutes and the SPA refreshes on
any `401`, so a `fleet-service` restart that outlasts one request logs out the
entire active user base. The fix is to distinguish *transient* from *permanent*
at the point where the distinction is visible (the HTTP status in
`membership.Client.Active`), carry it through the injected resolver, and answer
`503` — **without clearing the cookie** — on the transient branch.

This became reachable in task-008/#14, which made `Active` fail closed on
non-`404` statuses. That fix is correct and stays; this task pays the debt it
knowingly deferred.

---

## 2. Key files

### Go — `packages/shared-go/server`

| File | What matters |
| --- | --- |
| `errors.go` | Sentinel var block + `StatusFor`. `Detailed`/`detailedError` is the exact pattern `RetryAfter` mirrors — read it before writing the wrapper. |
| `jsonapi.go` | `WriteError`. The `>= 500` branch logs and redacts (`InternalErrorTitle`, no `detail`); the `< 500` branch renders `err.Error()` as the title. `WriteJSON` calls `WriteHeader`, so **any header set after it is silently discarded**. |
| `server.go` | `codeFor(status)` and `itoa`. Both unexported helpers live here, not in `jsonapi.go`. |
| `errors_test.go` | 471 lines and the regression net for ~190 `WriteError` call sites across four services. Extend it; do not restructure it. `TestMain` parks a null logger in the error seam. |

### Go — `apps/auth-service`

| File | What matters |
| --- | --- |
| `internal/membership/client.go` | `Active` (no timeout today, raw URL concatenation) and `FleetMemberIDs` (has the timeout, escapes its path, and its comment explicitly names `Active`'s concatenation as the habit not to inherit). `Client` shares `http.DefaultClient` — **which has no timeout of its own**. |
| `cmd/main.go` | `newPrincipalResolver` — the single construction site for `session.Principal`, enforced by `internal/arch/arch_test.go`'s `TestNoPrincipalLiteralOutsideResolver`. Returns errors bare today. |
| `internal/session/resource.go` | `refreshHandler`. Order is: read cookie → `proc.Rotate` → `resolve` → `MintAccess` → `SetRefreshCookie` → 200. The resolver-error branch is the one that splits. `SetRefreshCookie` / `clearRefreshCookie` are both here. |
| `internal/session/processor.go:101` | `Rotate`. Consumes the presented token, inserts a new one, and **revokes the whole family** if the presented token is already consumed or revoked. This is why the rotated value has to reach the browser on the `503` branch. |
| `internal/oidc/resource.go` | `loginErrorCode` constants (~:88), `failLogin` (~:120), the single unconditional `clearStateCookie` after `verifyStateCookie` (~:228), and the `d.Resolve` branch (~:262). |

### TypeScript

| File | What matters |
| --- | --- |
| `apps/web/src/lib/api/refresh.ts` | `inflight` dedupe + `mintAccessToken` (never clears) + `refreshAccessToken` (clears on failure). The clear is the logout mechanism. |
| `apps/web/src/lib/api/client.ts` | Wires `onRefresh: refreshAccessToken` into the singleton `ApiClient`. |
| `packages/shared-ts/src/apiClient.ts:35-38` | `fetchAuthenticated`'s one-shot `401`-refresh-and-retry. **Not modified** — a rejection from `onRefresh` already propagates out. |
| `packages/shared-ts/src/errors.ts` | `ApiError(status, code, message, detail?, pointer?)`, re-exported from `@myfleet/shared-ts`. |
| `apps/web/src/lib/auth/loginError.ts` | `LoginErrorCode` union, the hand-maintained `CODES` allowlist, and the `NOTICES` table. `isLoginErrorCode` degrades anything unknown to `server_error`. |
| `apps/web/src/pages/LoginPage.tsx:44` | `const failed = notice?.tone === 'danger'` — drives `role="alert"`, the danger band, **and** the button label. Not modified. |

---

## 3. Decisions already made (do not re-litigate)

| # | Decision | Why |
| --- | --- | --- |
| D1 | The carrier is `server.ErrServiceUnavailable`, not a `membership` sentinel | `session` and `oidc` must classify **without importing the concrete membership client** — that decoupling is the entire reason `PrincipalResolver` is injected. Precedent: `apps/fleet-service/internal/mediaclient/client.go:87` already wraps an upstream failure with `server.ErrValidation`. |
| D2 | `Retry-After` rides on a wrapper error, not a `WriteErrorWithRetryAfter` | The header is a property *of the error*, constructible once at the point of knowledge. Mirrors `Detailed` exactly. A second entry point would have to be duplicated for every future response concern. |
| D3 | Write the **rotated** cookie, then `503` (not resolve-before-rotate) | Resolve-first would put a 5-second outbound HTTP call ahead of the token check on a **public, unauthenticated** endpoint — request amplification against the very upstream this task protects — and would need a validate-without-consuming path that does not exist. |
| D6 | `refresh.ts` throws; `shared-ts` is untouched | The rejection channel already carries the distinction end-to-end. Widening `onRefresh: () => Promise<string \| null>` would break a published type and force every implementor to handle a third case. |
| D7 | The new notice's tone is `danger`, not `neutral` | `tone` is not cosmetic in `LoginPage` — `'neutral'` would render an outage as muted body text under a "Continue with Google" button. |
| D8 | `newPrincipalResolver` needs no edit for FR-CLASSIFY-7, only a test | It returns the membership error bare, so `errors.Is` already reaches through. The test guards tomorrow's `%v` wrap. |
| D9 | Local `users` / `platform_admins` failures classify transient too | Same defect one layer over: an auth-Postgres blip logs out every session through the same closure. Flagged in the design as an extension; confined to two `if` blocks. |
| D10 | `429` is transient with no upstream `Retry-After` passthrough; `403` stays permanent with no new machinery | `fleet-service` does not rate-limit internal endpoints, and the permanent log line already carries the status. |

---

## 4. Invariants this area has already broken once

Each of these has a test guarding it. Breaking one is a regression, not a
judgement call.

1. **`Active`'s `404` → `(Membership{}, nil)`.** A brand-new user's first login
   depends on it: the OIDC callback keys its `/onboarding` redirect off the
   resulting empty `ActiveFleetID`. `TestNewPrincipalResolver_treatsNoMembershipAsEmptyNotError`
   must pass **completely unmodified**.
2. **Fail closed on every other non-2xx.** `fleet-service`'s error envelope *is*
   JSON, so before #14 a `500` decoded cleanly into a zero `Membership` with
   `err == nil` and minted a valid token claiming no fleet.
3. **No user id, fleet id, or upstream body in an error message.** Two existing
   tests assert this (`TestActive_errorDisclosesNeitherBodyNorUser`,
   `TestFleetMemberIDs_errorCarriesNoIDAndNoBody`). `url.Error`'s message embeds
   the request URL, and this request's URL carries `user_id` as a query
   parameter — that is the trap on the transport path.
4. **`failLogin` must not clear the state cookie.** `GET /auth/callback` is
   public and carries no CSRF token, so a third party can reach the exits above
   the state check; clearing there would let them destroy a victim's in-flight
   login. The single unconditional clear after `verifyStateCookie` is the only
   clearing site. This is a *reviewed departure* from task-010's FR-ERR-8,
   documented at `resource.go:95-120`, and the comment says not to "fix" it.
5. **Concurrent refreshes must collapse onto one POST.** Two POSTs with the same
   cookie are reuse, and `Rotate` answers reuse by revoking the family. The
   `inflight` promise must keep *resolving* rather than rejecting for this to
   hold.
6. **`WriteJSON` commits the header block.** Set `Retry-After` before it or the
   response is a correct `503` with the header silently missing — and every
   status-only assertion still passes.

---

## 5. Dependencies between tasks

```
Task 1 (sentinel) ──┬─▶ Task 2 (RetryAfter) ──┐
                    │                          ├─▶ Task 5 (refresh handler)
                    ├─▶ Task 3 (membership) ───┤
                    │            │             └─▶ Task 6 (oidc callback)
                    └────────────┴─▶ Task 4 (resolver)

Task 5 ──▶ Task 7 (refresh.ts)      Task 6 ──▶ Task 8 (loginError.ts)
                    └──────────┬───────────────────┘
                               ▼
                          Task 9 (verification)
```

Tasks 1→6 are strictly sequential (each consumes the previous one's names).
Tasks 7 and 8 are independent of each other and could run in parallel, but both
need the backend contract from Tasks 5 and 6 to be settled. Task 9 needs
everything.

---

## 6. Commands

```sh
# Go, per package
cd apps/auth-service && go test ./internal/membership/ -race -v
cd packages/shared-go && go test ./... -race

# Web — Node is NOT always on PATH
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
npm run -w apps/web test -- src/lib/api/refresh.test.ts
npm run -w packages/shared-ts test        # fe-test runs this too

# Full gate
make ci     # lint-check vet test build fe-test fe-build manifests carfax-template

# Manifests (no change expected — standing gate, and `local` is not exempt)
kustomize build deploy/k8s/overlays/main  | kubectl apply --dry-run=server -f -
kustomize build deploy/k8s/overlays/local | kubectl apply --dry-run=server -f -
```

`make fe-test` runs `apps/web`, `packages/shared-ts` **and**
`packages/ui-components` — `shared-ts` owns `fetchAuthenticated` and previously
ran in no automated gate at all.

---

## 7. Out of scope

Retry, backoff, or circuit-breaking inside `membership.Client`. Client-side
auto-retry or any "reconnecting…" outage UI. Any `fleet-service` change,
including its error envelope. The `404 → (Membership{}, nil)` contract. #14's
fail-closed decision. A third `LoginErrorNotice.tone`. A `retryable` field on
`ApiError`. Classification for `FleetMemberIDs` (its callers do not need the
distinction; it shares only the timeout constant). Any `deploy/k8s` change — no
new configuration or environment variables are introduced.

---

## 8. Browser verification is required, not optional

`jsdom` cannot evaluate CSS, and two acceptance criteria are user-visible: the
SPA **not** bouncing to `/login` on a refresh `503`, and the login page rendering
the try-again-shortly notice with a "Try again" button. Confirm both against
real Chromium via the local stack — see `docs/runbooks/local-debugging.md` — with
`fleet-service` scaled to zero. Unit tests alone do not close those two boxes.
