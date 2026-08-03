
# Plan Adherence Audit — task-018-transient-upstream-503

> Section author: `plan-adherence-reviewer`. The backend and frontend guideline
> reviewers write their own sections to this same file; this section is
> self-contained and does not restate their findings.

**Plan Path:** `docs/tasks/task-018-transient-upstream-503/plan.md`
**PRD:** `docs/tasks/task-018-transient-upstream-503/prd.md` (§10 acceptance criteria)
**Audit Date:** 2026-08-03
**Branch:** `task-018-transient-upstream-503`
**Base:** `main` (`d3c9eaa`) · **Head:** `fd854ad` · 13 commits, 10 of them code

## Executive Summary

All eight implementation tasks (1–8) landed in full. Every file the plan's File
Structure table marks `modify` was modified, every code block the plan specifies
is present in the source essentially verbatim, and every one of the 22 new Go
tests and 9 new TS tests named in the plan exists on the branch. The five files
the plan explicitly places out of scope were not touched. The `404` guard test is
byte-identical to its `main` version. Go suites for both affected modules are
green.

The one gap is bookkeeping, not substance: **Task 9's Steps 5 and 6 are not
done.** All 56 plan checkboxes and all 18 PRD §10 checkboxes are still unchecked,
and the browser-verification record is not in the repository — the narrative
lives in a gitignored scratch path and the screenshot is untracked. The
verification work itself was performed and passed; only its durable record is
missing.

## Task Completion

| # | Task | Status | Evidence |
|---|------|--------|----------|
| 1 | `ErrServiceUnavailable` sentinel + `StatusFor`/`codeFor` → 503 | DONE | `packages/shared-go/server/errors.go:21` (sentinel), `:46` (`StatusFor` case), `packages/shared-go/server/server.go:33-34` (`codeFor`). Tests `TestWriteError_503KeepsTheRedactedEnvelope` + table rows in `errors_test.go`. Commit `aba0d51`. |
| 2 | `RetryAfter` wrapper + header emitted before commit | DONE | `errors.go:89-100` (`RetryAfter`, `retryAfterError` with `Error`/`Unwrap`/`RetryAfter`); `jsonapi.go:96-104` — the `errors.As` block sits immediately after `status := StatusFor(err)` and before `WriteJSON`, with the `> 0` guard. Four new tests incl. `TestWriteError_setsRetryAfterBeforeCommittingTheHeaderBlock`. Commit `5dfa252`. |
| 3 | `membership.Client.Active` classifies + times out | DONE | `client.go:38` (`var fleetLookupTimeout = 5 * time.Second`), `:41-42` (`WithTimeout`), `:48` (`url.QueryEscape`), `:66-72` (`*url.Error` unwrap, no URL in message), `:79-81` (404 → zero value), `:85-88` (`>=500 \|\| 429` transient), `:100-102` (other non-2xx permanent), `:104-109` (unparseable 2xx transient). `FleetMemberIDs` now shares the timeout at `:123`; the old `fleetMemberLookupTimeout` is gone. Tests: `TestActive_classifiesEveryResponseShape` (10 rows), `..._classifiesATransportFailureAsTransient`, `..._boundsAHangAndClassifiesItTransient`. Commit `03da0f9`. |
| 4 | Resolver carries + adds classification (design D9) | DONE | `cmd/main.go:179-182` (`user.ErrNotFound` bare), `:183-188` (`%w` sentinel, driver text dropped), `:197-200` (fleet error passes bare), `:205-207` (admin read transient). Four new tests incl. `TestNewPrincipalResolver_classifiesLocalInfrastructureFailuresAsTransient` with the SQLSTATE-leak loop. Commit `5314c4f`. |
| 5 | `POST /auth/refresh` → 503 + rotated cookie | DONE | `session/resource.go:28-33` (`refreshRetryAfterSeconds = 5`), `:68-83` — the transient branch does `Warn` → `SetRefreshCookie(w, newRaw, cookieSecure)` → `WriteError(w, RetryAfter(err, refreshRetryAfterSeconds))`, in that order; permanent branch at `:84-91` unchanged in kind. Tests: `TestRefresh_transientResolverFailureKeepsTheSessionAlive` (`resource_test.go:190-247`, incl. the `proc.Rotate(c.Value)` store assertion at `:225`), `..._transientFailureMintsNothingAndDisclosesNothing`, `..._transientFailureCannotReviveARevokedToken`; helpers `refreshCookieSet` (`:65`), `loggedAtLevel` (`:167`), `transientResolver` (`:178`); permanent-path `Retry-After`/log assertions appended at `:155-160`. Commits `13fd04b` + `0659202`. |
| 6 | OIDC callback → `#error=service_unavailable` | DONE | `oidc/resource.go:98` (`errServiceUnavailable`), `:268-281` (split branch: `errors.Is` → `Warn` + `failLogin(..., errServiceUnavailable)`; else `Error` + `errServerError`). `failLogin` and the single unconditional `clearStateCookie` untouched (FR-CALLBACK-3). Test `TestCallback_transientResolverFailureRedirectsWithServiceUnavailable`. Commit `19d6ba6`. |
| 7 | SPA keeps the session on a 503 refresh | DONE | `apps/web/src/lib/api/refresh.ts` rewritten: `RefreshOutcome` (`:17-18`), `inflight` resolving-only shared promise (`:28`, `:58-65`), `res.status === 503` checked before `!res.ok` (`:38-39`), `mintAccessToken` unchanged contract (`:79-82`), `refreshAccessToken` throws `ApiError(503, 'service_unavailable', …)` before any `clearAccessToken()` (`:96-104`). Five new `refresh.test.ts` cases + two test-only `apiClient.test.ts` cases. Commit `1eaf0bd`. |
| 8 | Login page outage notice | DONE | `apps/web/src/lib/auth/loginError.ts:3` (union member), `:15` (`CODES` allowlist), `:41-44` (`tone: 'danger'` + the exact copy "Sign-in is temporarily unavailable. Nothing was saved — try again in a moment."). `LoginPage.tsx` untouched, as specified. Two new tests incl. the CODES-allowlist test. Commit `a376f2e`. |
| 9 | Full verification, browser confirmation, code review | **PARTIAL** | Steps 1–4 done (see Build & Verification). **Step 5 not done** — all 18 PRD §10 boxes unchecked; the plan's own 56 boxes unchecked. **Step 6 not done** — no verification-record commit; see Gaps. |

**Completion Rate:** 8 / 9 tasks fully done (89%); Task 9 partial.
**Skipped without approval:** 0
**Partial implementations:** 1 (Task 9, bookkeeping only)

## Deviations from the plan's literal text (all adjudicated, none material)

1. **Task 5 test body (`0659202`).** The plan's literal test text for
   `TestRefresh_transientFailureMintsNothingAndDisclosesNothing` was vacuous:
   `json.NewDecoder(rec.Body).Decode` drains the `*bytes.Buffer`, so the
   subsequent `strings.Contains(rec.Body.String(), secret)` loop always compared
   against `""`. Commit `0659202` snapshots `rawBody` before decoding and adds
   the missing `Title != server.InternalErrorTitle` assertion
   (`resource_test.go:251`, `:278-281`). Human ruled the fix governs over the
   plan text. **Correct call** — the plan's own Global Constraint "prove every new
   test can fail" cannot be satisfied by the version as written.
2. **Prettier reflow (`fd854ad`).** Four TS files reformatted; `RefreshOutcome`
   (`refresh.ts:17-18`) and `LoginErrorCode` (`loginError.ts:2-3`) collapsed from
   the plan's multi-line unions to one line. No contract string changed —
   verified byte-exact against the plan for the `service_unavailable` literal,
   the notice copy, and the `ApiError(503, 'service_unavailable', 'Service
   temporarily unavailable')` construction.
3. **Design D5 `const` → `var fleetLookupTimeout`.** Pre-adjudicated in the
   plan's own Self-Review §1, documented in the declaration comment
   (`client.go:35-37`). Not written in production code.

## PRD §10 Acceptance Criteria

Behavioural:

| # | Criterion | Verdict | Evidence |
|---|-----------|---------|----------|
| 1 | fleet 500 → `503` + `Retry-After: 5`, cookie not cleared, no logout | **MET** | `TestRefresh_transientResolverFailureKeepsTheSessionAlive` asserts all three axes (`resource_test.go:198-217`); browser check 1 observed `503`, `Retry-After: 5`, rotated non-clearing `Set-Cookie`. |
| 2 | fleet unreachable (conn refused) → same | **MET** | `TestActive_classifiesATransportFailureAsTransient` (`client_test.go`) proves the classification; the refresh branch keys only on `errors.Is`, so the response is identical. |
| 3 | fleet hanging past timeout → same, ~5s not pinned | **MET** | `client.go:41-42` + `TestActive_boundsAHangAndClassifiesItTransient`, which pins the handler open and asserts both the transient classification and `elapsed < 1s` under a lowered `fleetLookupTimeout`. |
| 4 | Post-`503` refresh after recovery succeeds, no family revocation | **MET** | `resource_test.go:225-230` re-presents the cookie written alongside the `503` through `proc.Rotate` and asserts on the store, not the response. Browser check 3 confirmed end to end: `revoked_at` null on every family row after recovery. |
| 5 | Missing user row → `401` + cleared cookie | **MET** | `main.go:179-182` returns `user.ErrNotFound` bare; `TestNewPrincipalResolver_keepsAMissingUserPermanent` asserts it does *not* satisfy `errors.Is(…, ErrServiceUnavailable)`; `TestRefresh_failsClosedAndClearsCookieWhenTheResolverErrors` (extended at `:155-160`) asserts `401`, cleared cookie, no `Retry-After`, and `ErrorLevel`. |
| 6 | Membership `404` → empty `ActiveFleetID`; new user → `/onboarding` | **MET** | `client.go:79-81`; `TestNewPrincipalResolver_treatsNoMembershipAsEmptyNotError` verified **byte-identical** to its `main` version (23 lines, `diff` clean); `TestActive_classifiesEveryResponseShape` "no membership" row. No SPA onboarding-routing code was touched. |
| 7 | fleet `403` → `401` + clear path | **MET** | `client.go:100-102` (bare error, no sentinel), `TestActive_classifiesEveryResponseShape/forbidden`, `TestNewPrincipalResolver_keepsAPermanentUpstreamFaultPermanent`. |
| 8 | Callback during outage → `#error=service_unavailable` + try-again message | **MET** | `oidc/resource.go:273-276` + `TestCallback_transientResolverFailureRedirectsWithServiceUnavailable` (asserts exact `Location`, `WarnLevel` log, state-cookie clear unchanged, no refresh cookie, no token in fragment). Browser check 2 confirmed the danger band and the **"Try again"** button label against real Chromium — `jsdom` cannot see either. |
| 9 | SPA does not redirect to `/login` on a `503`; token survives; `503` `ApiError` not `401` | **MET** | `refresh.ts:38` + `:99-101`; `refresh.test.ts` asserts `getAccessToken()` still holds the token, not merely that the promise rejected; `apiClient.test.ts` pins the `503`-over-`401` propagation and the one-shot no-retry contract. Browser check 1 confirmed `window.location.pathname` stayed put and `localStorage.access_token` survived. |

Code and verification:

| # | Criterion | Verdict | Note |
|---|-----------|---------|------|
| 10 | `TestNewPrincipalResolver_treatsNoMembershipAsEmptyNotError` unmodified | **MET** | Byte-identical (verified by extracting the function from `d3c9eaa` and `HEAD` and diffing). |
| 11 | shared-go table tests cover `503` / `service_unavailable`; nothing existing changed | **MET** | Rows added to both maps; `codeFor`'s existing eleven cases and `StatusFor`'s existing ten are untouched; `WriteError`'s `>= 500` redaction and `< 500` title/detail logic unchanged. |
| 12 | `Active` table-driven over all eight shapes, asserting classification | **MET** | 10-row table + 2 dedicated tests; every row asserts `errors.Is(err, server.ErrServiceUnavailable)` against an expected value, not `err != nil`. |
| 13 | `refreshHandler` tests assert status + `Retry-After` + cookie-clearing on both paths | **MET** | Transient: `:198-217`. Permanent: `:145-160`. |
| 14 | Each new test proven able to fail | **MET (with one correction)** | Recorded per-task in `.superpowers/sdd/plan/task-N-report.md`. The one case where the proof was hollow was caught and repaired in `0659202`. |
| 15 | `make ci` passes | **MET** | Green per the execution record. Independently re-confirmed here: `go test ./... -count=1` green for `packages/shared-go/server` and all 9 `apps/auth-service` packages. |
| 16 | Both overlays render; both server dry-runs pass | **MET** | Both clean per the execution record. No manifest file is in the diff (`deploy/k8s` untouched), so this is the standing gate only. |
| 17 | Three reviewer agents run before the PR | **IN PROGRESS** | This section is one of the three; the backend and frontend reviewers are writing concurrently. |
| 18 | Issue #15 closed by the PR | **PENDING** | No PR opened yet. |

**Boxes actually ticked in `prd.md`: 0 of 18.** The criteria are satisfied on the
evidence above; the document does not yet say so.

## Gaps

### G1 — Plan and PRD checkboxes are entirely unchecked (Task 9, Step 5)

`grep -c '\- \[x\]'` returns **0** for both `plan.md` (56 open boxes) and
`prd.md` §10 (18 open boxes). Task 9's own **Files** line names
`plan.md (check off completed steps)` as its deliverable, and Step 5 requires
walking §10 and checking each box against evidence. Neither happened. Anyone
reading the branch's own artifacts would conclude nothing was implemented.
Impact: presentational, but it is the deliverable Task 9 exists to produce.

### G2 — The browser-verification record is not in the repository (Task 9, Steps 3 and 6)

Step 3 says "Record what was observed (including screenshots …) **in the task
folder**." Instead:

- `.superpowers/sdd/plan/task-9-browser-report.md` — the full narrative, but
  `git check-ignore` confirms it is ignored by `.superpowers/sdd/.gitignore:1`.
  It will not survive into the PR.
- `docs/tasks/task-018-transient-upstream-503/login-outage.png` — present on
  disk (36 KB) but **untracked**; it is the only file in `git status`.

Step 6's `git add docs/tasks/… && git commit` was never run. Impact: the
evidence for acceptance criteria 1, 2, 3, 4, 8 and 9 — the ones the unit suite
provably cannot reach, per the standing "jsdom cannot see CSS" rule — is
invisible to a reviewer of the PR. Fixing this is a `git add` and a commit; the
report body should be copied into the task folder first.

### G3 — Two out-of-scope defects surfaced during verification are recorded nowhere durable

Both are in the gitignored report and will vanish with it. Neither is caused by
this branch, and neither should be fixed here — but both should outlive it:

1. **`deploy/compose/docker-compose.yml` `web` service is unreachable as
   committed.** The healthcheck and the Traefik
   `loadbalancer.server.port` label both target port `80`, while
   `apps/web/nginx.conf` binds `8080` (`nginx-unprivileged`). Traefik excludes
   the unhealthy container, so `http://localhost/` 404s on a clean bring-up. The
   verification pass worked around it with a gitignored
   `docker-compose.override.yml`; the base file is still broken for the next
   person following `docs/runbooks/local-debugging.md`.
2. **fleet-service accepts an expired JWT.** `GET
   /api/fleet/fleets/{id}/activity` returned `200` for a token whose `exp` was
   an hour in the past, while auth-service correctly `401`s the same token on
   `/auth/me`. This is a security defect independent of task-018 and warrants
   its own issue.

## Build & Test Results

| Module | Build | Tests | Notes |
|--------|-------|-------|-------|
| `packages/shared-go` | PASS | PASS | `go test ./server/ -count=1` → ok (0.019s) |
| `apps/auth-service` | PASS | PASS | `go test ./... -count=1` → ok across all 9 packages (`cmd`, `arch`, `jwks`, `membership`, `oidc`, `platformadmin`, `session`, `user`) |
| `apps/web`, `packages/shared-ts` | PASS | PASS | Per the execution record's `make ci` run (`fe-test`, `fe-build`); not re-run here |
| `deploy/k8s` | n/a | PASS | Both overlays render; both `kubectl apply --dry-run=server` clean. No manifest change in the diff. |

## Overall Assessment

- **Plan Adherence:** MOSTLY_COMPLETE — Tasks 1–8 FULL; Task 9 substance done,
  bookkeeping outstanding.
- **Recommendation:** NEEDS_FIXES (documentation only — no production code
  change is required or advised).

## Action Items

1. Check off the completed steps in `plan.md` (Task 9's stated deliverable).
2. Walk `prd.md` §10 and tick each box against the evidence in the table above;
   leave #18 (issue closed) open until the PR merges.
3. Copy `task-9-browser-report.md` into
   `docs/tasks/task-018-transient-upstream-503/`, `git add` it together with the
   already-present `login-outage.png`, and commit — Task 9 Step 6.
4. File a follow-up issue for the `docker-compose.yml` `web` port mismatch (80
   vs 8080) so the local runbook works without a private override.
5. File a security issue for fleet-service accepting an expired JWT. Do not fix
   it on this branch.

---

# Backend Guidelines Audit — task-018-transient-upstream-503

> Section author: `backend-guidelines-reviewer`. Self-contained; does not restate
> the plan-adherence or frontend sections.

- **Scope:** Go changes on `d3c9eaa..fd854ad` — `packages/shared-go/server`,
  `apps/auth-service/internal/{membership,session,oidc}`, `apps/auth-service/cmd`
- **Guidelines Source:** `backend-dev-guidelines` skill
- **Date:** 2026-08-03
- **Build:** PASS (`make ci` green, reported by the invoking session)
- **Tests:** PASS (all Go suites in both affected modules)
- **Overall:** NEEDS-WORK — no blocking defect; 2 medium and 2 low findings

## Scope Note on the DOM/SUB Checklist

`packages/shared-go/server` implements a deliberately smaller handler surface
than the checklist assumes. `RegisterInputHandler` exists but with a different
signature (`handler.go:47` — `func(http.ResponseWriter, *http.Request, T)`), and
there is **no** `RegisterHandler`, no `HandlerDependency`, no `d.Logger()`, and
no `MarshalResponse` anywhere in `packages/shared-go/` (verified by grep). The
checklist items phrased in terms of those symbols (DOM-06/07/08 as written) are
therefore evaluated against what this codebase actually provides, and any gap is
a pre-existing platform divergence rather than something this branch introduced.

## Domain Checklist — `internal/session` (domain: has `model.go`)

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| DOM-01 | `builder.go` exists | PASS | `internal/session/builder.go:13` — `NewBuilder()` |
| DOM-02 | `ToEntity()` on Model | PASS | `internal/session/entity.go:39` |
| DOM-03 | `Make(Entity)` | PASS | `internal/session/entity.go:27` |
| DOM-04 | `Transform` in `rest.go` | PASS | `internal/session/rest.go:12` |
| DOM-05 | `TransformSlice` in `rest.go` | WARN (pre-existing) | Absent from `internal/session/rest.go`. The package has no list endpoint — `InitializePublicRoutes` (`resource.go:39-44`) registers only `POST /auth/refresh` and `POST /auth/logout`. Not touched by this branch. |
| DOM-06 | Processor takes `FieldLogger` | PASS | `internal/session/processor.go:52` — `NewProcessor(log logrus.FieldLogger, …)`. Zero `*logrus.Logger` in `apps/auth-service/internal/`. |
| DOM-07 | Handlers use injected logger | PASS | `resource.go:46` takes `log logrus.FieldLogger` by injection; zero `logrus.StandardLogger()` in `apps/auth-service/internal/`. |
| DOM-08 | POST uses `RegisterInputHandler[T]` | WARN (pre-existing) | `resource.go:41-42` uses `r.Post(…, http.HandlerFunc)`. Shared-go's `RegisterInputHandler` decodes a JSON:API `{data:{attributes:T}}` envelope (`handler.go:49-53`), which `/auth/refresh`'s cookie-or-`{"refreshToken"}` contract does not use. Unchanged by this branch. |
| DOM-09 | Transform errors handled | PASS (N/A) | `Transform` returns a single value (`rest.go:12`); no discard possible. Zero `, _ := Transform` in `apps/auth-service/internal/`. |
| DOM-10 | Providers lazy | PASS | `internal/user/provider.go:40,54,75` — `database.Query(...)()`. |
| DOM-11 | No `os.Getenv()` in handlers | PASS | Zero matches across `internal/*/resource.go`. |
| DOM-12 | No cross-domain logic in handlers | PASS | Cross-domain composition is in `cmd/main.go:174-215` (`newPrincipalResolver`) and injected as `session.PrincipalResolver` (`resource.go:23`). Enforced by `internal/arch/arch_test.go:29`. |
| DOM-13 | Handlers don't call providers | PASS | `resource.go:54,66,95` call `proc.Rotate`, the injected `resolve`, and `proc.MintAccess`. |
| DOM-14 | No direct entity writes in handlers | PASS | Zero `db.Create`/`db.Save`/`db.Delete` in any `internal/*/resource.go`. |
| DOM-15 | `administrator.go` for writes | PASS | `internal/session/administrator.go:24` |
| DOM-16 | Error → status mapping | PASS | `resource.go:50` (401 no cookie), `:62` (401 replay/expiry), `:82` (503 transient), `:91` (401 permanent), `:98` (401 mint failure) |
| DOM-17 | JSON:API interface | PASS (adapted) | `rest.go:12-18` returns `server.Resource{Type,ID,Attributes}` — this codebase's struct-based equivalent of the interface methods. |
| DOM-18 | Flat request models | PASS | `resource.go:131-133` — flat `{refreshToken}`; no nested Data/Type/Attributes. |
| DOM-19 | Table-driven tests | PASS | `client_test.go:181-208`, `errors_test.go:247-265`, `cmd/main_test.go:379` |

## Sub-Domain Checklist — `internal/oidc` (`resource.go`, no `model.go`)

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| SUB-01 | Logic outside the handler | PASS | `internal/oidc/processor.go` exists; identity composition is delegated to `d.Resolve` (`resource.go:268`). |
| SUB-02 | No writes in `resource.go` | PASS | Zero `db.Create`/`db.Save`; provisioning goes through `d.Users.ProvisionFromGoogle` (`resource.go:257`). |
| SUB-03 | POST via typed input handler | PASS (N/A) | `InitializeRoutes` registers only `GET` routes (`resource.go:79-80`). |
| SUB-04 | No manual JSON parsing | PASS | Zero `json.NewDecoder`/`json.Unmarshal`/`io.ReadAll` in `internal/oidc/resource.go`. |

`internal/membership` is a support package (outbound HTTP client only — no
`model.go`, no `resource.go`); no DOM/SUB checklist applies.

Note: `internal/session/resource.go:134` uses `json.NewDecoder` directly, which
is the SUB-04 shape. It is the documented cookie fallback in `readRefreshToken`
(`:122-138`), predates this branch, and is unchanged by it.

## Security Review

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| SEC-01 | Verified token parsing | PASS (unchanged) | Zero `ParseUnverified` in `apps/auth-service`. `cmd/main.go:138` — `authmw.JWT(ks.Keyfunc(), …)`. Not touched by this branch. |
| SEC-02 | Revocation acts on validated tokens | PASS | `session/resource.go:54` — `proc.Rotate` runs BEFORE the resolver at `:66`, so the 503 branch is unreachable until the presented token has been validated and rotated. A replay still ends at `:61-62` (clear + 401) regardless of upstream health. Proven by `resource_test.go:292+` (`TestRefresh_transientFailureCannotReviveARevokedToken`). |
| SEC-03 | No open redirect | PASS (undisturbed) | `oidc/resource.go:127-129` — `failLogin` composes `d.AppBaseURL + d.LoginPath + "#error=" + code` from config plus a typed constant (`:88-99`); the new `errServiceUnavailable` is a member of that closed set. `safeReturnPath` (`:153-184`) unchanged. |
| SEC-03b | `failLogin` still does not clear the state cookie | PASS | `oidc/resource.go:127-129` contains no clear; the single unconditional clear stays at `:234`, after `verifyStateCookie`. The branch diff adds only the `errServiceUnavailable` constant and the `callbackHandler` branch at `:269-277` — the reviewed departure documented at `:104-121` is untouched. |
| SEC-04 | No hardcoded secrets | PASS | No secret literals introduced; the diff adds only sentinels, a duration, and an int constant. |
| SEC-05 | 503 body stays redacted on a public endpoint | PASS | `jsonapi.go:106-121` — `Title` is `InternalErrorTitle` and `Detail` is populated only under `if status < 500`, so a 503 gets neither. Asserted end-to-end at `resource_test.go:277-288` (title/detail checks plus a raw-body leak loop over `membership`, `lookup`, `user-1`). |
| SEC-06 | `Active` keeps 404 → `(Membership{}, nil)` | PASS | `membership/client.go:79-81`, and the 404 test is ahead of both the transient (`:85`) and permanent (`:100`) status checks. |
| SEC-07 | `Active`'s error messages carry no user id / body | PASS | `client.go:66-72` unwraps `*url.Error` to `urlErr.Err` before formatting, so the query string never enters the message; `:100-101` and `:104-108` emit status codes and fixed text only. Asserted at `client_test.go:86-99` and `:246-248`. |
| SEC-08 | Resolver drops driver text | PASS | `cmd/main.go:188` and `:206` format with a fixed string and do not `%w` the driver error, so no SQLSTATE or table name can reach a log line. |
| SEC-09 | `FleetMemberIDs`'s error messages carry no fleet id | **FAIL** | `client.go:135-137` returns the bare `*url.Error` on transport failure — see finding B-2. |

## Findings

### B-1 (MEDIUM, non-blocking) — every 503 still emits an ERROR-level log line, defeating the branch's own warn-not-error intent

`session/resource.go:78-80` states the intent: *"Warn with its own message, not
Error: an outage must read as an outage rather than as a wave of authentication
failures."* But `WriteError` logs unconditionally for the whole 5xx range —
`jsonapi.go:111-114`:

```go
if status >= 500 {
    errorLogger().WithError(err).WithField("status", status).
        Error("request failed; error text redacted from the response body")
}
```

There is no 503 exemption, and `server.New` installs the service's own logger as
that error logger (`handler.go:21-24`), which for auth-service is the same `log`
the handler holds (`cmd/main.go:125`). So each refresh during a fleet-service
outage produces one WARN *and* one ERROR line. The outage still reads as an
error-rate spike — the exact signal FR-REFRESH-6 was written to remove.

The existing test cannot catch this: `resource_test.go:234-239` asserts only
against the handler's own `logrus` hook, not against the logger
`SetErrorLogger` installed.

### B-2 (MEDIUM, non-blocking) — `FleetMemberIDs` leaks the fleet id into a log line; the identical bug was fixed 60 lines above it

`membership/client.go:134-137`:

```go
res, err := c.hc.Do(req)
if err != nil {
    return nil, err
}
```

`http.Client.Do` wraps transport failures in `*url.Error`, whose `Error()`
embeds the request URL — here `…/internal/fleets/<fleetID>/members`
(`client.go:129`). That error is logged verbatim at
`internal/user/resource.go:226` (`log.WithError(err).Error("auth/users fleet
member lookup failed")`), so the fleet id lands in a log line as an address.

This is the same defect this branch deliberately fixed in `Active`
(`client.go:66-72`), and it contradicts `FleetMemberIDs`'s own doc comment at
`client.go:144-145` — *"the fleet id must not ride along into a log line"*.
The branch did touch this function (`client.go:123`, timeout constant). Existing
coverage misses it: `client_test.go:163-172` exercises only the non-2xx path,
which was already safe.

The response body is unaffected — `user/resource.go:227` writes `errInternal`,
not the raw error.

### B-3 (LOW) — `Active`'s request-construction error is returned unredacted

`membership/client.go:49-54` returns the `http.NewRequestWithContext` error bare.
On a URL that will not parse, that error is a `*url.Error` whose message is
`parse "<full endpoint>"` — including `?user_id=<id>` from `client.go:48`. It
reaches `session/resource.go:89` or `oidc/resource.go:278` at Error level.
Reachable only via a malformed `base` at startup (`QueryEscape` guarantees the
userID itself cannot break parsing), so the exposure is narrow — but the
redaction applied 12 lines below is not applied here, and the added comment at
`:50-51` addresses only the transient/permanent question.

### B-4 (LOW, consistency) — the transient classification stops at `Active`

`FleetMemberIDs` shares the hop, the client, and now the same
`fleetLookupTimeout` (`client.go:38`, `:123`), but returns unclassified errors
(`client.go:136`, `:147`, `:154`). The same fleet-service outage that now yields
`503 + Retry-After: 5` on `POST /auth/refresh` still yields a bare `500` with no
`Retry-After` on `GET /auth/users` (`user/resource.go:222-228`). Not a
regression, and possibly deliberate plan scope — recorded so the asymmetry is a
decision rather than an oversight.

### B-5 (INFO) — the underlying database error is discarded entirely

`cmd/main.go:188` and `:206` neither wrap nor log the driver error, so a Postgres
outage leaves auth-service with `"user lookup failed"` / `"platform admin lookup
failed"` and no SQLSTATE anywhere in its logs. This is the direct consequence of
the stated redaction invariant and is correct as specified; noted only so the
diagnosability cost is a known trade-off.

## Summary

### Blocking (must fix)
- None.

### Non-Blocking (should fix)
- **B-1** — `jsonapi.go:111-114`: 503 responses log at ERROR, contradicting
  `session/resource.go:78-80`; `resource_test.go:234-239` cannot detect it.
- **B-2** — `membership/client.go:135-137`: bare `*url.Error` leaks the fleet id
  into `user/resource.go:226`; the same pattern was fixed in `Active` at
  `client.go:66-72`.
- **B-3** — `membership/client.go:49-54`: request-construction error can carry
  `?user_id=` into a log line.
- **B-4** — `membership/client.go:136,147,154`: `FleetMemberIDs` returns
  unclassified errors, so `GET /auth/users` answers 500 for the same outage.

### Pre-existing (not introduced by this branch)
- **DOM-05** — `session/rest.go` has no `TransformSlice` (no list endpoint).
- **DOM-08 / SUB-04** — `session/resource.go:41-42,134` uses `r.Post` +
  `json.NewDecoder` rather than `RegisterInputHandler[T]`; shared-go's variant
  expects a JSON:API envelope the refresh contract does not use.

---

# Frontend Guidelines Audit — task-018-transient-upstream-503

> Section author: `frontend-guidelines-reviewer`. Scoped to the FE-* checklist over
> the changed TypeScript files only; does not restate the plan-adherence or
> backend sections in this file.

- **Audit Scope:** TypeScript/React changes in `d3c9eaa..HEAD` (13 commits)
- **Guidelines Source:** `frontend-dev-guidelines` skill (`.claude/skills/frontend-dev-guidelines/`)
- **Date:** 2026-08-03
- **Build:** PASS (not re-run — `make ci` reported green by the invoking session, including `fe-build`, `fe-test`, prettier)
- **Tests:** PASS (same provenance)
- **Overall:** PASS (zero blocking FAIL checks; 4 non-blocking items)

## Build & Test Results

Per the invoking session, `make ci` is green and the login page was verified in real Chromium
(danger-band computed styles + "Try again" label). Gates were not re-executed in this audit;
this section is scoped to guideline conformance. Static verification performed here:

- `grep` for `: any` / `as any` / `<any>` across all five changed TS files — zero real matches
  (`loginError.test.ts:31` matched only on the word "anything" inside a comment).
- Em dash byte check on the required copy: `hexdump` of `loginError.ts:43` yields
  `e2 80 94` = U+2014. **Byte-exact as required.**

## File Inventory

| File | Classification | Note |
|---|---|---|
| `apps/web/src/lib/api/refresh.ts` | Other (lib/api — auth token primitive) | Not a Page/Component/Hook/Service/Schema/Type |
| `apps/web/src/lib/api/refresh.test.ts` | Test | |
| `apps/web/src/lib/auth/loginError.ts` | Other (lib/auth — pure module) | |
| `apps/web/src/lib/auth/loginError.test.ts` | Test | |
| `packages/shared-ts/src/apiClient.test.ts` | Test | TEST-ONLY, per stated invariant |

No `.tsx` files were changed. No `pages/`, `components/`, `lib/hooks/api/`, `services/api/`,
`lib/schemas/`, or `types/models/` files were changed. Several checks are therefore N/A rather
than PASS, and are marked as such — an N/A is absence of scope, not evidence of compliance.

## Anti-Pattern Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-01 | No `any` type | PASS | Grep over all 5 changed files: zero `: any`, `as any`, `<any>`. Test doubles use narrowed double-casts instead — `refresh.test.ts:10` (`as unknown as Response`), `apiClient.test.ts:17` (`as RequestInit` / `as Record<string, string>`) — the guideline-sanctioned alternative, not `any`. |
| FE-02 | No manual class concatenation | N/A | No `className` in any changed file (grep: zero matches). No JSX changed. |
| FE-03 | No direct API client calls in components | PASS | No components changed. `refresh.ts` is `lib/api/` — the client's own layer, not a consumer; it imports only `ApiError` from `@myfleet/shared-ts` (`refresh.ts:1`) and `./token` (`refresh.ts:2`). Dependency direction is correct: `client.ts:3` imports `refreshAccessToken`, not the reverse. |
| FE-04 | No inline Zod schemas in components | N/A | Grep for `z.object(` / `z.string(` across changed files: zero matches. |
| FE-05 | No spinners for content loading | N/A | Grep for `animate-spin`: zero matches in changed files. |
| FE-06 | No hardcoded colors | N/A | Grep for `bg-` / `text-gray`: zero matches. The danger band lives in `LoginPage.tsx`, deliberately unmodified. |
| FE-07 | No state mutation | PASS | Grep for `.push(` / `.splice(` / `.sort(`: zero matches. `RefreshOutcome` values are constructed fresh at each return (`refresh.ts:38,39,42,44,46`); `NOTICES` (`loginError.ts:26`) is a module constant never written to. |
| FE-08 | No default exports | PASS | Grep for `export default` in `refresh.ts` and `loginError.ts`: zero matches. All exports named — `mintAccessToken` (`refresh.ts:79`), `refreshAccessToken` (`refresh.ts:96`), `noticeFor` (`loginError.ts:48`), `consumeLoginError` (`loginError.ts:93`). |
| FE-09 | Error handling with `createErrorFromUnknown` | **WARN** | `refresh.ts:45` uses a bare `catch { return { status: 'dead' }; }` and `refresh.ts:40` uses `.catch(() => null)`; neither routes through `createErrorFromUnknown()`. Graded WARN not FAIL — see Non-Blocking #1. The thrown error is a hand-constructed `new ApiError(503, …)` (`refresh.ts:100`) rather than a `createErrorFromUnknown` product, which is correct here: `createErrorFromUnknown` (`packages/shared-ts/src/errors.ts:23`) expects a `{status, body}` HTTP envelope and would degrade a raw `TypeError` to `ApiError(0, 'unknown')`. |

## Architecture Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-10 | JSON:API model shape | N/A | No files under `apps/web/src/types/models/` changed. `RefreshResponse` (`refresh.ts:4-6`) models the `data.attributes` envelope and is untouched by this diff. |
| FE-11 | Service extends `BaseService` | N/A | No files under `services/api/` changed. |
| FE-12 | Query key factory uses `as const` | N/A | No query key factories changed. |
| FE-13 | Forms use `react-hook-form` + `zodResolver` | N/A | No form components changed. |
| FE-14 | Schema in `lib/schemas/` with inferred type | N/A | No Zod schemas changed. |
| FE-15 | Interactive elements show `cursor-pointer` | N/A | No JSX changed; `LoginPage.tsx` deliberately untouched. |

### Invariant Verification (task-specific)

The invariants this change is bound by, each checked against source rather than assumed.

| Invariant | Status | Evidence |
|---|---|---|
| `shared-ts` production code unmodified; rejection propagates via `onRefresh` | **VERIFIED** | `git diff --stat d3c9eaa..HEAD` lists only `packages/shared-ts/src/apiClient.test.ts` (+58) — `apiClient.ts` and `errors.ts` are absent from the diff. `apiClient.ts:6` still declares `onRefresh: () => Promise<string \| null>` (unwidened); `apiClient.ts:36` `await this.opts.onRefresh()` sits inside no try/catch, so a rejection escapes `fetchAuthenticated` → `request`. Proven by `apiClient.test.ts:181-203`, asserting `rejects.toMatchObject({ status: 503 })`. |
| Concurrent refreshes collapse onto ONE POST; `inflight` RESOLVES, never rejects | **VERIFIED** | `refresh.ts:58-65` — `refreshOnce()` stores `requestToken()`, whose every path `return`s a `RefreshOutcome` value (`refresh.ts:38,39,42,44,46`); there is no `throw` inside `requestToken`. The throw is at `refresh.ts:100`, in `refreshAccessToken`, strictly after `await refreshOnce()` at line 97 — each caller gets its own rejection, so no shared rejected promise and no double-settle. `.finally` (lines 60-62) still frees the slot. Guarded by `refresh.test.ts:158-166` (two `refreshAccessToken` calls, one fetch, both rejected) and `refresh.test.ts:58-73`. |
| One attempt, no client-side auto-retry | **VERIFIED** | `apiClient.ts:35` guards on `!retried`; `apiClient.ts:37` only recurses when `refreshed` is truthy. `apiClient.test.ts:202` and `:224` both assert `fetchMock` called exactly once. |
| `mintAccessToken` never throws and never clears | **VERIFIED** | `refresh.ts:79-82` — no `throw`, no `clearAccessToken()`; returns `null` for both `dead` and `unavailable`. Return type `Promise<string \| null>` unchanged from pre-diff. Guarded by `refresh.test.ts:89-95`, asserting `resolves.toBeNull()` **and** `getAccessToken()` still `'still-valid'` under a 503. |
| 503 path does not clear the stored token | **VERIFIED** | `refresh.ts:99-101` throws before reaching `clearAccessToken()` at line 102. Asserted on stored state, not merely on the rejection: `refresh.test.ts:134` `expect(getAccessToken()).toBe('still-valid')`. The 401 path still clears — `refresh.test.ts:147-153`. |
| 503 check ordered before the `ok` check | **VERIFIED** | `refresh.ts:38` precedes `refresh.ts:39`, so a 503 can never fall into the `dead` bucket. |
| Notice message byte-exact incl. U+2014 | **VERIFIED** | `loginError.ts:43`; `hexdump` confirms `e2 80 94`. Test asserts the same literal at `loginError.test.ts:116`. |
| `service_unavailable` matches the Go side character for character | **VERIFIED** | TS: `loginError.ts:3,15,41`, `refresh.ts:100`. Go: `apps/auth-service/internal/oidc/resource.go:98` (`errServiceUnavailable loginErrorCode = "service_unavailable"`) and `packages/shared-go/server/server.go:35`. Cross-checked by the Go test at `apps/auth-service/internal/oidc/resource_test.go:697` asserting the redirect `…/login#error=service_unavailable`. |
| `LoginPage.tsx` unmodified; `tone: 'danger'` drives more than colour | **VERIFIED** | Absent from `git diff --stat`. The claim in the code comment (`loginError.ts:36-38`) is accurate, not aspirational: `LoginPage.tsx:44` `const failed = notice?.tone === 'danger'`, gating `role="alert"` (`LoginPage.tsx:77`) and the `'Try again'` label (`LoginPage.tsx:104`). Tone choice not re-litigated. |

## Testing Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-16 | Tests exist for changed components | PASS | Both changed source modules have colocated test files, both extended in this diff: `refresh.test.ts` (+65, new `describe('refreshAccessToken')` at `:98-167`, five cases) and `loginError.test.ts` (+43, `:111-131`). `shared-ts` gained `apiClient.test.ts:170-226`. Note: the guideline doc (`testing-guide.md:5`) describes Jest 30; this repo runs Vitest (`apps/web/package.json:8` → `"test": "vitest run"`). Audited against actual repo convention. |
| FE-17 | Mocks updated when services changed | PASS | No `__mocks__/` directory exists in `apps/web`; mocks are inline `vi.mock`. The three existing mocks of this module — `MemberList.test.tsx:33-34`, `members.test.ts:89-90`, `OnboardingPage.test.tsx:22-23` — mock both `mintAccessToken` and `refreshAccessToken` with `mockResolvedValue`, still satisfying the unchanged `Promise<string \| null>` signature. No mock is stale. |

## Summary

### Blocking (must fix)

None. No FAIL checks.

### Non-Blocking (should fix)

1. **FE-09 — `catch` blocks bypass `createErrorFromUnknown` (`refresh.ts:40`, `refresh.ts:45`).**
   Graded WARN rather than FAIL: the anti-pattern this rule encodes (`anti-patterns.md:120-138`)
   is *missing* handling — a `.then()` with no `.catch()`. Here the `catch` is present and
   translates the failure into a typed `RefreshOutcome`, and user-facing surfacing happens
   downstream (`clearAccessToken()` → `isAuthenticated` false → `RequireAuth` navigates).
   Feeding a raw fetch `TypeError` to `createErrorFromUnknown` would yield `ApiError(0, 'unknown')`
   (`packages/shared-ts/src/errors.ts:35`) — strictly less information than the current outcome
   type. Recommend leaving as-is and recording the exemption, or amending the rule's wording.

2. **Stale comment not updated alongside the code it describes (`loginError.ts:18-22`).**
   It reads "One sentence for all **three** failure codes" and "The table is keyed on all
   **four** codes anyway". After this change there are four failure codes and five total codes,
   and `service_unavailable` is explicitly the code that does *not* take the shared sentence.
   The comment now contradicts `loginError.ts:41-44` two lines below it. One-line fix.

3. **`CODES` can silently drift from `LoginErrorCode` (`loginError.ts:10`).**
   `CODES` is typed `readonly string[]`, so it is hand-maintained with no compiler link to the
   union at `loginError.ts:3`. Adding a union member without adding the string compiles clean
   and degrades that code to `server_error` at `loginError.ts:78`. The author knew this and
   wrote a guard test (`loginError.test.ts:128-131`, reasoning at `:122-127`) — so this instance
   is covered, but the structure permits recurrence. `NOTICES` is already
   `Record<LoginErrorCode, LoginErrorNotice>` and therefore exhaustiveness-checked; deriving
   `const CODES = Object.keys(NOTICES)` would make drift impossible and retire the guard test.

4. **No page-level regression guard for the new notice (`LoginPage.test.tsx`).**
   `service_unavailable` appears nowhere in `apps/web/src` outside `lib/` and its two unit test
   files. `LoginPage.test.tsx:176` covers the danger branch using `auth_failed`, so the shared
   rendering machinery (`role="alert"`, "Try again") is guarded — but nothing asserts the *new*
   code reaches it. Chromium verification confirmed it today; it is not protected tomorrow.
   Adding `'service_unavailable'` to an `it.each` alongside `LoginPage.test.tsx:176` closes it.

### Informational (no action on this branch)

- **Network-error → forced logout is unchanged and deliberate.** `refresh.ts:45-47` maps a thrown
  `fetch` (offline, DNS, TLS) to `{ status: 'dead' }`, so `refreshAccessToken` clears the token
  and the user is signed out — the same class of "transient failure that says nothing about the
  session" this task fixed for 503. This is **not a regression**: the pre-diff code did the same
  (`catch { return null }` → `if (!token) clearAccessToken()`), and it is documented as an
  intentional mapping at `design.md:241` ("`{ status: 'dead' }` // 401, unparseable body, missing
  token, network error"). Recorded for the backlog.
- **`mintAccessToken`'s "leaves the existing token ALONE" doc (`refresh.ts:68-69`) is true of the
  function itself, not of a concurrent window.** Because `inflight` is shared, a
  `refreshAccessToken` racing on the same dead outcome will clear the token while
  `mintAccessToken` is in flight. Pre-existing structure, unchanged by this diff.
