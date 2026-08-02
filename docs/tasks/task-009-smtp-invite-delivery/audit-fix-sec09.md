# SEC-09 fix — error text disclosure in `WriteError`

## The finding

> **SEC-09 — Error text disclosure.** `packages/shared-go/server/jsonapi.go:35` puts
> `err.Error()` into the response `Title` for **any** error, and `errors.go:41` maps
> unrecognised errors to 500. Callers pass raw GORM/driver errors straight in, so a DB
> failure returns driver text — table names, SQLSTATE — to the caller.

Confirmed by reading. `StatusFor` (`packages/shared-go/server/errors.go:18-43`) has a
`default: return 500` arm, and `WriteError` unconditionally set `Title: err.Error()`.
Every `server.WriteError(w, err)` where `err` came from a repository/GORM call therefore
rendered the driver's message verbatim in the client-facing JSON:API envelope.

## What changed

**`packages/shared-go/server/jsonapi.go:29-68`** — the only production change.

- Added `const InternalErrorTitle = "internal server error"` (jsonapi.go:31). Exported so a
  service or test can assert against the redaction without re-typing the literal.
- `WriteError` now chooses the title by **mapped status**, never by inspecting the message:
  - `status >= 500` → `Title` is the fixed `InternalErrorTitle`; `err.Error()` is never
    called on that path.
  - `status < 500` → `Title` is `err.Error()`, exactly as before, and the `Detailed`
    `detail` field is attached exactly as before.
- The `detail` field is likewise only attached on `< 500`. A `Detail()` on a 4xx sentinel is
  a deliberately client-facing sentence (the invite-accept preconditions). On an arbitrary
  5xx error chain there is no way to know that, so it is suppressed with the title. No
  existing caller is affected: every `server.Detailed(...)` in the repo is built over
  `server.ErrConflict` (409) — `apps/fleet-service/internal/invite/processor.go:78-91`.
- Side effect worth noting: `WriteError(w, nil)` no longer panics. `StatusFor(nil)` returns
  500 and the 500 path never dereferences the error. Pinned by a test.

The split is by status precisely as required — no string matching, no heuristics on message
content.

## Blast radius measured

```
$ grep -rn "WriteError" --include='*.go' apps/ packages/ | wc -l
253
```

Broken down (at baseline, before my test additions):

- 241 references under `apps/`, 12 under `packages/`.
- 190 of the `apps/` references are literally `server.WriteError(w, err)` — a raw error
  variable straight from a processor/administrator, i.e. exactly the shape that maps to 500
  and was leaking.
- Top call-site files: `fleet-service/internal/invite/resource.go` (30),
  `maintenancerecord/resource.go` (28), `maintenanceschedule/resource.go` (27),
  `vehicle/resource.go` (26), `fuel/resource.go` (22), `media-service/internal/mediaobject/resource.go` (17),
  plus `auth-service` (12 across `user`/`session`) and `notification-service` (11).

Sampled call sites across three services to confirm the pattern holds:
`apps/fleet-service/internal/invite/resource.go:134-141` (logs, then `WriteError(w, err)`),
`apps/media-service/internal/mediaobject/resource.go:60/66/79/112/123` (raw `err` passed
through), `apps/auth-service/internal/user/resource.go`. All become safe with no edits.

## Tests updated

**None.** No test in the repo asserted on 500 body text, so none was encoding the bug.

I searched for them specifically:

```
$ grep -rn 'Title' --include='*_test.go' apps/ packages/
```

- `apps/fleet-service/internal/invite/resource_test.go:258` asserts `title == "conflict"` on
  a 409 — 4xx, must keep passing untouched. It does.
- `packages/shared-go/server/errors_test.go:101` asserts `title == "conflict"` on a 409 —
  same. Passes untouched.
- `apps/auth-service/internal/user/resource_test.go:178`
  (`TestPatchMe_validationTitleNamesTheFieldNotTheInput`) asserts a 422 title contains
  `themePreference` and `light, dark, system` — a caller-authored wrap of `ErrValidation`.
  This is the strongest reason the split had to be by status and not by "is the message
  safe": that 4xx text is deliberate and must survive. It does, and I added a mirror of it
  in the shared package (`TestWriteError_4xxKeepsACallerAuthoredWrap`) so a future change to
  `WriteError` fails in the shared package rather than only in auth-service.
- `apps/auth-service/internal/membership/client_test.go:24,82` and
  `apps/auth-service/cmd/main_test.go:177` contain 500 envelopes with
  `"title":"internal server error"`, but those are **hand-written fixture bodies** served by
  an `httptest` stub standing in for fleet-service — they never go through `WriteError`.
  Unaffected. (Amusingly they already assumed the redacted title this change now makes real.)
- `apps/media-service/internal/mediaobject/resource_test.go:856` asserts a 500 *status* and
  that it was logged, not its body text. Unaffected.

## Tests added (`packages/shared-go/server/errors_test.go`)

Plus a `writeErrorBody(t, err)` helper that renders and decodes the envelope.

- `TestWriteError_500RedactsTheUnderlyingErrorText` — the required case. Renders
  `errors.New("pq: relation \"fleet.fleet_invites\" does not exist (SQLSTATE 42P01)")` and
  asserts the body contains **none** of `pq:`, `relation`, `fleet.fleet_invites`,
  `fleet_invites`, `SQLSTATE`, `42P01`, `does not exist` — absence of the distinctive
  substrings, not merely a non-empty title. Also pins `status`/`code` = `500`/`internal_error`.
- `TestWriteError_500RedactsAWrappedDriverError` — the same leak via
  `fmt.Errorf("insert invite for fleet %s: %w", ...)` around a unique-constraint violation;
  asserts `duplicate key`, `uniq_invites_token`, `SQLSTATE`, `23505`, the fleet id and the
  caller's own annotation are all absent.
- `TestWriteError_500DropsDetail` — a `Detailed` wrapper over a 500-class error renders no
  `detail` key and leaks none of its text.
- `TestWriteError_nilErrorDoesNotPanic` — pins the nil-safety the change introduced.
- `TestWriteError_4xxSentinelsKeepTheirMessageAndCode` — table over all ten sentinels in
  `errors.go` (400/401/403/404/409/410/413/415/422/429), asserting each still renders its
  exact status, code and message, and no `detail`.
- `TestWriteError_4xxKeepsACallerAuthoredWrap` — a 422 wrap keeps the caller's full sentence.
- `TestWriteError_4xxDetailSurvivesTheRedaction` — all four invite-accept preconditions
  (`invite has already been accepted`, `invite has expired`,
  `invite was issued to a different account`, `invite cannot be accepted`) still render as
  409 / title `conflict` / their own `detail`. This is the explicit guard on the recent
  `Detailed` work.

The two pre-existing `Detailed` tests
(`TestWriteError_rendersDetailWhenTheErrorCarriesOne`,
`TestWriteError_omitsDetailForAPlainSentinel`) were left exactly as they were and pass.

## Verification (real output)

```
$ go build github.com/jtumidanski/myfleet/... && go vet github.com/jtumidanski/myfleet/...
BUILD_VET_OK
```

```
$ go test github.com/jtumidanski/myfleet/packages/shared-go/... -v
...
--- PASS: TestWriteError_500RedactsTheUnderlyingErrorText (0.00s)
--- PASS: TestWriteError_500RedactsAWrappedDriverError (0.00s)
--- PASS: TestWriteError_500DropsDetail (0.00s)
--- PASS: TestWriteError_nilErrorDoesNotPanic (0.00s)
--- PASS: TestWriteError_4xxSentinelsKeepTheirMessageAndCode (0.00s)
    --- PASS: .../bad_request .../unauthorized .../forbidden .../not_found
    --- PASS: .../conflict .../gone .../request_entity_too_large
    --- PASS: .../unsupported_media_type .../validation .../too_many_requests
--- PASS: TestWriteError_4xxKeepsACallerAuthoredWrap (0.00s)
--- PASS: TestWriteError_4xxDetailSurvivesTheRedaction (0.00s)
--- PASS: TestDetailed_keepsStatusAndTitleWhileCarryingDetail (0.00s)
--- PASS: TestWriteError_rendersDetailWhenTheErrorCarriesOne (0.00s)
--- PASS: TestWriteError_omitsDetailForAPlainSentinel (0.00s)
PASS
ok  	github.com/jtumidanski/myfleet/packages/shared-go/server	0.006s
(all other shared-go packages: ok)
```

```
$ go test github.com/jtumidanski/myfleet/...
(every package ok / no test files — zero failures across auth-service, fleet-service,
 media-service, notification-service, dto-go, shared-go)
```

```
$ export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22 && make ci
...
manifest checks passed
carfax template check passed
MAKE_CI_EXIT=0
```

`gofmt -l packages/shared-go/server/` is clean.

## Diagnosability — read this

**The redacted error is now visible nowhere in the shared package.** `WriteError` is a free
function with no logger (`Server` owns the `logrus.FieldLogger`, `handler.go:11-21`), so
there is no logging seam to use. I did not invent one — adding logging to handlers is DOM-07's
job and is being fixed separately in fleet-service's invite resource.

Concretely, of the 190 `server.WriteError(w, err)` sites, roughly **141 have no logging
statement within the six lines preceding the call**, spread across:

| file | unlogged sites |
| --- | --- |
| `apps/fleet-service/internal/vehicle/resource.go` | 25 |
| `apps/fleet-service/internal/maintenanceschedule/resource.go` | 20 |
| `apps/fleet-service/internal/maintenancerecord/resource.go` | 19 |
| `apps/fleet-service/internal/fuel/resource.go` | 16 |
| `apps/media-service/internal/mediaobject/resource.go` | 9 |
| `apps/fleet-service/internal/membership/resource.go` | 9 |
| `apps/fleet-service/internal/invite/resource.go` | 9 |
| `apps/fleet-service/internal/mileage/resource.go` | 8 |
| `apps/fleet-service/internal/vehiclemedia/resource.go` | 7 |
| `apps/fleet-service/internal/fleet/resource.go` | 7 |
| `apps/fleet-service/internal/dashboard/resource.go` | 7 |
| `apps/fleet-service/internal/activity/resource.go` | 3 |
| others | 2 |

(Heuristic count — a `log.WithError(err)` further than six lines above, or a log inside the
processor, would not be counted. Treat it as an upper bound with the right order of
magnitude.)

Before this change, a 500 at one of those sites at least surfaced the driver text to whoever
made the request. After it, that text is gone entirely unless the caller logs it. That is the
correct security tradeoff — the client is the wrong place for it — but it makes
**"log the error before calling `WriteError` on a 5xx path" a hard requirement for every
handler**, not a nicety. `apps/fleet-service/internal/invite/resource.go` and
`apps/media-service/internal/mediaobject/resource.go` already do this for most paths
(`media-service` even has a test asserting it:
`TestGetContent_lookupFailureIs500AndIsLogged`). `fleet-service`'s `vehicle`,
`maintenanceschedule`, `maintenancerecord` and `fuel` resources largely do not, and are the
biggest gap.

**Recommended follow-up (deliberately not done here):** a repo-wide sweep adding
`log.WithError(err)...Error(...)` before every 5xx `WriteError`, or a `WriteErrorLogged(log,
w, err)` helper in the shared package that does it centrally. Either is a change to callers,
which this task's scope explicitly excludes.

## Deliberately not done

- **No caller edits.** Zero files outside `packages/shared-go/server` were touched. The fix is
  entirely at the boundary, as scoped.
- **No logging added to `WriteError` or to any handler.** No seam exists, and DOM-07 covers
  fleet-service's invite resource separately.
- **No change to `StatusFor`, the sentinels, `codeFor`, or the `Detailed` type.** The 500
  default arm is correct behaviour; the leak was in rendering, not mapping.
- **No new error taxonomy** (e.g. a `503`/`502` distinction, or an operator-facing
  correlation id echoed in the 500 body). Echoing the correlation id in the redacted body
  would materially help support triage and is safe, but it is a contract addition beyond this
  finding — worth raising separately.
- **No frontend change.** `packages/shared-ts/src/errors.ts:30` passes `title` through
  generically and nothing in `apps/web` asserts on a 500 title, so the redacted string flows
  through unchanged.

---

# Follow-up: 5xx logging

Closing concern #1 above rather than deferring it. Confirmed the coordinator's reading first:
neither `packages/shared-go/server` nor `packages/shared-go/telemetry` wraps the response
writer or observes status — `grep -rn "WrapResponseWriter\|middleware.Logger\|Recoverer"` over
`apps/` and `packages/` returns nothing. Nothing else in the stack saw a 5xx, so redacting the
body really did make ~141 failures invisible to everyone.

## Bootstrap wiring: it worked out

No fallback needed, and no caller churn. `server.New(log logrus.FieldLogger)`
(`packages/shared-go/server/handler.go:17-25`) already receives the service logger and is
called exactly once per service:

```
$ grep -rn "server.New(" --include='*.go' apps/ packages/
apps/fleet-service/cmd/main.go:191:	if err := server.New(log).
apps/auth-service/cmd/main.go:81:	if err := server.New(log).
apps/notification-service/cmd/main.go:89:	if err := server.New(log).
apps/media-service/cmd/main.go:129:	if err := server.New(log).
```

`New` now calls `SetErrorLogger(log)` before building the `Server`. All four services get 5xx
logging — with their own configured logger, formatter and level — from this one line. Zero
files outside `packages/shared-go/server` were touched, still.

## What changed

**`packages/shared-go/server/jsonapi.go:33-70`** — the seam:

- `var errorLog atomic.Pointer[logrus.FieldLogger]` (jsonapi.go:43).
- `SetErrorLogger(logrus.FieldLogger)` (jsonapi.go:52-58) — exported for handlers mounted
  outside the shared bootstrap and for tests. `nil` restores the fallback.
- `errorLogger()` (jsonapi.go:65-70) — **never returns nil**. An unset seam falls back to
  `logrus.StandardLogger()`, which writes to stderr, i.e. where a container's logs already go.
  The error is surfaced, never silently discarded, and a missing logger cannot panic an error
  path.

**`packages/shared-go/server/jsonapi.go:95-98`** — `WriteError` logs at Error level when
`status >= 500`, immediately before writing the redacted body:

```go
errorLogger().WithError(err).WithField("status", status).
	Error("request failed; error text redacted from the response body")
```

Only the error value and the status. No request body, no headers, no credentials, no user or
fleet ids. Sub-500 logs nothing.

**`packages/shared-go/server/handler.go:17-25`** — `New` installs the logger.

The response contract established above is untouched: 5xx still renders the generic
`InternalErrorTitle` and no `detail`. The log and the body deliberately diverge.

## Why atomic.Pointer, not a mutex

The value is written once at startup and read on every erroring request. A `sync.RWMutex`
would put a lock on the hot path to guard a write that happens once per process; an atomic
load is race-free with no contention and no critical section. `atomic.Pointer` holds
`*logrus.FieldLogger` because the generic needs a pointer type — hence the `p != nil && *p != nil`
guard in `errorLogger`, which also covers someone storing a typed-nil interface.

**I verified the race test has teeth rather than assuming it.** Temporarily demoting
`errorLog` to a plain `var errorLogUnsafe logrus.FieldLogger` and re-running only
`TestSetErrorLogger_isSafeUnderConcurrentUse` under `-race` produced:

```
WARNING: DATA RACE
Write at 0x000000abd270 by goroutine 20:
  ...server.SetErrorLogger()  jsonapi.go:54
Previous read at 0x000000abd270 by goroutine 11:
  ...server.errorLogger()     jsonapi.go:62
  ...server.WriteError()      jsonapi.go:98
```

The atomic version was then restored and the test passes. So the construction is not merely
"probably fine" — the guard demonstrably fails on the unsafe variant.

## Tests added

- `TestWriteError_500LogsTheErrorItRedacts` — the required both-halves-in-one test. Asserts
  the hook captured exactly one Error-level entry whose `error` field equals the **full**
  `pq: relation "fleet.fleet_invites" ... (SQLSTATE 42P01)` text and whose `status` field is
  500, **and** that the response body contains none of `pq:`, `relation`,
  `fleet.fleet_invites`, `SQLSTATE`, `42P01`. The divergence is the whole point, so both are
  pinned in one test.
- `TestWriteError_4xxLogsNothing` — all ten sentinels plus a `Detailed` 409; each must produce
  zero log entries.
- `TestWriteError_withNoLoggerInstalledDoesNotPanic` — clears the seam, captures
  `logrus.StandardLogger().Out`, and asserts the error **still reaches the log** while still
  not reaching the body. Stronger than "does not panic": it proves the fallback surfaces
  rather than swallows.
- `TestNew_installsTheErrorLogger` — pins the zero-caller-churn property. If `New` stops
  installing, every service silently drops to the stderr fallback and loses its log fields.
- `TestSetErrorLogger_isSafeUnderConcurrentUse` — 8 goroutines × 200 `WriteError` calls
  against 4 goroutines × 200 `SetErrorLogger` calls.
- `TestMain` parks a null logger so the 5xx tests do not spray simulated failures over the
  test output; tests that assert on the log install their own and restore on cleanup.

## Verification (real output)

```
$ go build github.com/jtumidanski/myfleet/... && go vet github.com/jtumidanski/myfleet/...
BUILD_VET_OK
```

```
$ go test github.com/jtumidanski/myfleet/... -race
GO_TEST_RACE_EXIT=0
46 packages ok; no failures, no data races
```

```
$ export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22 && make ci
(make ci runs: go test -race github.com/jtumidanski/myfleet/...)
manifest checks passed
carfax template check passed
MAKE_CI_EXIT=0
```

Nothing failed anywhere, including `apps/fleet-service/internal/invite`, which another agent
is editing concurrently. I stayed out of that package.

## Re-answering concern #1: is the gap closed?

**Yes, and for all of them — including the ~141 that log nothing themselves.**

Every 5xx response in every service now produces an Error-level log entry carrying the full
underlying error, because the log happens inside `WriteError` itself rather than depending on
the caller remembering. The measurement that motivated the concern — 141 of 190 raw-`err`
call sites with no nearby log — is no longer the relevant number: those sites are covered by
the boundary, exactly as the redaction is. The information moved from the response body to
the server log; it was not destroyed.

Two honest residuals, neither reopening the concern:

1. **Handlers that already log now log twice** for the same failure (fleet-service's invite
   and media-service's mediaobject resources, among others). Duplication, not loss — and the
   handler's entry is the richer one because it carries `fleet_id` / `trace_id` /
   `correlation_id` context that `WriteError` deliberately does not gather. Worth
   de-duplicating eventually by dropping the redundant handler-side log, but that is caller
   churn and out of scope here. It does not affect correctness.
2. **The shared entry has no request context** — no correlation id, route or method, since
   `WriteError` only receives an `http.ResponseWriter` and an `error`. For the 141 previously
   silent sites this is still a large net gain (a stack-less "something failed with this
   driver error" beats nothing). Threading the correlation id through would need either a
   `*http.Request` parameter — a signature change at 253 call sites — or the response-status
   middleware the coordinator confirmed is absent. That middleware is the better long-term
   answer and is worth raising as its own item.

## Deliberately not done (follow-up)

- No caller edits, again — still zero files touched outside `packages/shared-go/server`.
- No de-duplication of the handler-side logs described above.
- No response-status/correlation-id logging middleware. It would subsume both residuals but
  is a new component, not a fix to this finding.
