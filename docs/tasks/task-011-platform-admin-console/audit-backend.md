# Backend Audit — task-011-platform-admin-console

- **Review range:** `f959cdc2d012acdd8666af361ab7778e7fd1216a..7ba674a161d91ddc755b88c1fd85b756f05cc024`
- **Services:** fleet-service, auth-service, media-service, notification-service
- **Guidelines source:** `backend-dev-guidelines` skill
- **Date:** 2026-08-02
- **Build:** PASS (all four services, `go build ./...`)
- **Tests:** 37 packages ok, 0 failed (`go test ./... -count=1`)
- **Overall:** **NEEDS-WORK** — build and tests are green, but two Critical data-loss defects and three Important findings are present.

---

## Build & Test Results

```
fleet-service        go build ./...  OK      go test ./... -count=1  19 ok, 0 fail
auth-service         go build ./...  OK      go test ./... -count=1   8 ok, 0 fail
media-service        go build ./...  OK      go test ./... -count=1   6 ok, 0 fail
notification-service go build ./...  OK      go test ./... -count=1   4 ok, 0 fail
```

The suite passing is not evidence of correctness for the findings below; each
Critical/Important finding was reproduced with a throwaway probe test (since
removed — working tree verified clean).

---

## Critical

### C1 — A cancelled purge is reaped anyway, permanently destroying downstream data

**Files:** `apps/fleet-service/internal/admin/processor.go:338-341`,
`apps/fleet-service/internal/admin/provider.go:67-68`,
`apps/fleet-service/internal/admin/reaper.go:75-81`

`Cancel` sets `StatusPartial` when any downstream restore fails
(`processor.go:338-341`). `ListDue` selects `status IN (pending, partial)`
(`provider.go:67-68`), so a partially-cancelled operation stays in the reaper's
candidate set. When `purge_after` elapses, `reapOne` calls
`d.Reap(ctx, op.ID())` on every downstream (`reaper.go:75-81`) — hard-deleting
exactly the rows the operator tried to restore.

Reproduced (probe against `admintest` fixture, `failingRestore` downstream):

```
after cancel: status="partial" live vehicles=2
after reaper: status="reaped"  downstream Reap calls=1  live vehicles=2
```

Consequences:
- The downstream data the operator explicitly cancelled is permanently gone.
- The operation is marked `reaped` (`reaper.go:90`), so every further `Cancel`
  now returns 409 "this operation has been reaped; its data is permanently
  deleted" (`processor.go:310-312`) — while local data is intact. The API
  states the opposite of the truth.
- Local `Reap` finds nothing (`Restore` already NULLed `purge_operation_id`,
  `operations.go:87-96`), so the audit row written at `reaper.go:96-108`
  records **all-zero** `affected_counts`. The audit trail understates the
  destruction to zero.

Root cause: `partial` is overloaded to mean both "the purge partially applied"
and "the cancel partially applied", and nothing records that a cancel was ever
requested. `SetStatus` only stamps `cancelled_at` for `StatusCancelled`
(`administrator.go:43-47`), so there is no durable marker either.

### C2 — Retry re-purges an operation the operator cancelled

**File:** `apps/fleet-service/internal/admin/processor.go:392-394`

The retry guard rejects only `StatusReaped` and `StatusCancelled`. A
partially-cancelled operation is `StatusPartial`, so `Retry` is accepted and
re-runs `fanOutPurge`, re-stamping downstream data that was just restored.

Reproduced:

```
PROBE HIT: retry accepted on a cancelled operation -> status="pending",
downstream Purge calls 1->2, live vehicles=2
```

The operation returns to `pending` with a `purge_after` that may already be in
the past, so the next reaper tick then hard-deletes downstream while local rows
stay live. `TestRetry_onACancelledOperationIs409`
(`processor_test.go:402-414`) covers only the fully-cancelled path and does not
catch this.

**Note on question 4 (create/retry re-verify, cancel does not):** the
asymmetry itself is sound — `Cancel` is the recovery path and must not be
coupled to auth-service availability (`processor.go:294-301`), and it is
correctly gated by `RequirePlatformAdmin` on a valid token
(`resource.go:187-191`). The asymmetry is **not** what is unsafe here. What is
unsafe is that cancel's outcome is not represented in the state machine, which
is C1/C2.

---

## Important

### I1 — A failed MinIO removal permanently strands the media row and its bytes

**File:** `apps/media-service/internal/admin/resource.go:135-142` vs `:145`

On a `RemoveObject` failure the handler NULLs `purge_operation_id` for the
affected rows and their variants, then calls `Reap(tx, opID)` — which keys on
`purge_operation_id`. The row survives, but it is now detached from the
operation: `Restore` (`operations.go:69-78`), `Reap` (`operations.go:81-91`)
and `ReapableObjectKeys` (`operations.go:106-117`) all key on
`purge_operation_id`, and media's own sweep keys on `purge_after`, which an
admin stamp never writes (`mediaobject/purge.go:28-33`).

Reproduced:

```
first reap -> 409
after failed reap: rows still carrying op-1 = 0 ; mo-1 soft-deleted = 1
second reap (storage healthy) -> 200 body={"affected":{"media_objects":0,"media_variants":0}}
objects removed across both reaps: [k/mo-1-thumb]
mo-1 rows remaining after the retry tick: 1
```

The retry tick reports success, `k/mo-1` is never removed, and `mo-1` remains
soft-deleted, invisible to every read, unreachable by restore, reap or sweep.
fleet-service takes the 200 and marks the operation `reaped`.

This directly contradicts the handler's own comment at `resource.go:117-118`
("Keep the owning row so the next tick retries") and at `:154-155` ("the next
hourly tick retries the survivors"). `TestReap_keepsRowsWhoseObjectCouldNotBeRemoved`
(`admin_test.go:214-229`) asserts only that the row exists — it never asserts
the row still carries the operation id, which is why the defect passes.

### I2 — The new unauthenticated destructive routes ship with no deny rule outside the `main` overlay

**Files:** `deploy/k8s/infra-local/ingressroute.yaml` (whole file),
`deploy/compose/docker-compose.yml:197-201`

Verified per-service against actual stripprefix behaviour rather than assumed:

| Service | stripPrefix (`base/routing/middlewares.yaml`) | Public path reaching `/internal/...` | Covered in `overlays/main`? |
|---|---|---|---|
| notification | `/api/notifications` (`:50-52`) | `/api/notifications/internal/admin/purge` → **reaches the handler** | Yes — `ingressroute.yaml:161` |
| fleet | `/api/fleet` (`:23-25`) | `/api/fleet/internal/...` → reaches the handler | Yes — `ingressroute.yaml:89` |
| media | `/api` only (`:36-38`) | `/api/media/...` strips to `/media/...`; unreachable | Yes (defence in depth) — `:111` |
| auth | `/api` only (`:13-15`) | `/api/auth/...` strips to `/auth/...`; unreachable | Yes (defence in depth) — `:135` |

The `main` overlay is correct, and the TLS twin inherits the rules via the
`replacements` block — verified by rendering:

```
kustomize build deploy/k8s/overlays/main  | grep -c internal-deny   -> 9
kustomize build deploy/k8s/overlays/local | grep -c internal-deny   -> 0
```

`infra-local/ingressroute.yaml` has no deny rule at all, and
`docker-compose.yml` defines the same `notifications-stripprefix` with no
equivalent. So in both dev environments
`POST /api/notifications/internal/admin/purge` and
`POST /api/notifications/internal/admin/reap/{opId}` are reachable,
unauthenticated, and destructive. The code comments assert "The two ship
together; never separately"
(`notification-service/internal/admin/resource.go:42`,
`auth-service/internal/platformadmin/resource.go:50`) — that holds for `main`
and demonstrably does not for `local` or compose.

### I3 — Admin handlers call the Provider directly, bypassing the processor

**File:** `apps/fleet-service/internal/admin/resource.go:122, 138, 228`

```go
ops, total, err := proc.d.Provider.ListOperations(...)   // :122
op, err := proc.d.Provider.GetOperation(...)             // :138
events, total, err := proc.d.Provider.ListAudit(...)     // :228
```

Three handlers reach through the processor's private `d` field into the
Provider. `anti-patterns.md:13` and `file-responsibilities.md:106-108` forbid
`resource.go → provider.go`. No `Processor` method exists for these reads —
`processor.go`, `browse.go`, `stats.go` and `reaper.go` define none. Every
other fleet-service domain goes through the processor (e.g.
`fuel/resource.go:52`). The circular-dependency exception in
`anti-patterns.md:214-248` does not apply: `Processor` and `Provider` are in
the same package.

---

## Minor

| ID | Finding | Evidence |
|---|---|---|
| M1 | The media deny-rule comment is arithmetically wrong: it claims `/api/mediainternal/media` "would otherwise reach the handler as `/internal/media`", but `media-stripprefix` strips `/api`, yielding `/mediainternal/media`. Media is safe for the same reason auth is (`/api/internal/...` matches no router), not for the reason stated. A future reader relying on this comment would mis-model the risk. | `deploy/k8s/overlays/main/ingressroute.yaml:105-107` |
| M2 | `TestAdminTreeIsSeparate` walks `..` = `apps/fleet-service/internal` only. `packages/shared-go` and the other three services' `internal/` trees are unchecked, so the package doc's claim that "Nothing outside this package may read `auth.Identity.PlatformAdmin`" is enforced only within fleet-service. Currently no violation exists (verified by grep across `apps` + `packages`: production refs are only the minting path, the shared middleware, and `/auth/me` metadata). | `admin/arch_test.go:132`; `auth-service/internal/user/resource.go:72` |
| M3 | Admin handlers log with the composition-root logger and no correlation id, though `telemetry.CorrelationIDFromContext` is already used two lines away for the audit row. | `admin/resource.go:54, 73, 93, 107, 124, 144, 179, 198, 216, 233` vs `:171, 267` |
| M4 | `POST /admin/purge-operations` uses an anonymous inline struct as its request type rather than a named flat request model in `rest.go`. Flat (so not the nested-envelope anti-pattern), but untestable and undiscoverable from `rest.go`. | `admin/resource.go:152-157` |
| M5 | DOM-03 deviation: `MakeOperation`/`MakeAudit` return `Operation`/`AuditEvent`, not `(Model, error)`; jsonb decode errors are swallowed with `_ =`. Deliberate and documented, but a malformed `affected_counts` renders as an empty blast radius with no signal. | `admin/model.go:79-81, 104, 107, 163` |
| M6 | The three internal route trees pass raw database errors to `server.WriteError`, which copies `err.Error()` into the response `title`. The fleet `/admin` tree explicitly guards against this with `errInternal`; the internal trees do not. Internal-only surface, so exposure is bounded. | `media-service/internal/admin/resource.go:54, 78, 93`; `notification-service/internal/admin/resource.go:50, 74, 90, 104`; `auth-service/internal/platformadmin/resource.go:59, 92`; contrast `fleet-service/internal/admin/resource.go:16-19` |
| M7 | `requireRecord` does not filter `deleted_at`, so a record purge can be created against a row an earlier operation already stamped. Benign (the `Stamp` guard `deleted_at IS NULL` prevents double-stamping, so the second operation carries zero counts) but it mints a real operation row over rows it does not own. | `admin/targets.go:84-97` |

---

## Checks That PASS (with evidence)

### Question 1 — Is the `RequireSameFleet` bypass contained?

**PASS.** `RequireSameFleet` (`authz/scope.go:12-17`) does not consult
`PlatformAdmin` — it is a pure `ActiveFleetID` comparison, unchanged by this
task. `RequirePlatformAdmin` (`authz/scope.go:46-51`) is a separate function.
`TestAdminTreeIsSeparate` (`admin/arch_test.go:131-186`) enforces in both
directions that no file outside `internal/admin` + `internal/adminclient`
names `PlatformAdmin`, and that no file inside calls `RequireSameFleet(`; the
test passes. A repo-wide grep confirms the only production references outside
the admin tier are the minting path
(`auth-service/internal/session/processor.go:72`), the shared middleware
(`packages/shared-go/auth/middleware.go:50`) and `/auth/me` metadata
(`auth-service/internal/user/resource.go:72`) — none of which gates a resource.
The admin tree is mounted in its own chi group
(`fleet-service/cmd/main.go:279-283`). See M2 for the guard's scope limit.

### Question 3 (partial) — Reaper ordering and crash window

**PASS for the crash case.** `reapOne` runs downstream before local
(`reaper.go:75-81`) and aborts the operation on any downstream failure, so a
crash between the two leaves the operation `pending`/`partial` with
`purge_operation_id` intact and the next tick re-runs it. Every step keys on
the operation id and is idempotent (`operations.go:87-111`).
`TestReapDue_downstreamFailureLeavesTheOperationRetryable`
(`reaper_test.go:82-111`) covers it. The stranding case that is **not**
covered is I1 (media detaches the row from the operation itself), and the
ordering interacts badly with cancel — C1.

### Question 5 — SQL injection

**PASS.** Every raw query interpolates only package-level constants and
parameterises all caller-controlled values:
- `operations.go:23, 55, 71, 89, 104` — `t.Table` from `Manifest`
  (`manifest.go:130-247`); predicates from `Target.Where` closures returning
  constant strings + `?` args.
- `browse.go:146-155` — `deletedPredicate` returns constants against a
  hard-coded `"f"` alias; the search term is parameterised at `:170-171`; the
  composed `pred` is `fmt.Sprintf`'d at `:214` but contains no user data.
- `stats.go:59` — `table` from the package-level `localCounts` map (`:35-43`).
- `orphans.go:34, 77-79` — table/column names from `Manifest`.
- `targets.go:90, 105` — query text from `existsSQL`/`labelSQL` maps
  (`:21-40`) keyed by a `TargetType` already validated against
  `ValidTargetTypes` (`processor.go:125`); ids are always `?` args.
- media/notification manifests (`media-service/internal/admin/manifest.go:40-70`,
  `notification-service/internal/admin/manifest.go:34-64`) — same shape.

No dynamic predicate anywhere in `browse.go` is built from request input.

### Question 6 (partial) — Soft-delete and tri-state filtering

**PASS.** `deletedPredicate` (`browse.go:146-155`) is correct in all three
branches and composes safely with the name filter (all conjunctions;
`DeletedInclude`'s disjunction is parenthesised at `:153`). `Count`
(`operations.go:23`) and `Stamp` (`operations.go:55-57`) both guard
`deleted_at IS NULL`, which is what makes blast radius and affected rows equal.
`Restore` keys only on `purge_operation_id` (`operations.go:87-96`), so it
cannot resurrect a user-deleted row. Both legacy sweeps are correctly excluded
from admin-stamped rows: `vehicle/purge.go:39-41` and
`mediaobject/purge.go:30`. Order independence is enforced by
`TestStamp_isOrderIndependent` (`operations_test.go:246`) backed by
`StampReversedForTest` (`operations.go:116-131`).

**No cross-tenant leak found.** Every fleet-scope predicate keys on
`fleet_id` / a `fleet_id` sub-select
(`manifest.go:94, 114, 150-151, 192, 212`); notification's manifest keys on
`fleet_id` and on `user_id` nowhere
(`notification-service/internal/admin/manifest.go:45`); media's keys on
`fleet_id` or an explicit id set fleet-service can only produce from within one
fleet (`media-service/internal/admin/manifest.go:48, 63`;
`targets.go:115-128`). `ListUsers`' membership join filters both
`m.deleted_at` and `f.deleted_at` (`browse.go:444`).

### Domain checklist — `fleet-service/internal/admin`

| ID | Check | Status | Evidence |
|---|---|---|---|
| DOM-01 | `builder.go` with `Build()` validation | PASS | `builder.go:17, 58-75` |
| DOM-02 | `ToEntity()` on the model | PASS | `model.go:114`, `model.go:169` |
| DOM-03 | `Make(Entity)` | WARN | `model.go:82`, `:141` — named `MakeOperation`/`MakeAudit`, no error return (M5) |
| DOM-04/05 | `Transform` + slice variant | PASS | `rest.go:173, 196, 95, 138, 219` |
| DOM-06 | Processor takes `logrus.FieldLogger` | PASS | `processor.go:73, 79` |
| DOM-07 | No `logrus.StandardLogger()` in handlers | PASS | zero matches in `resource.go` |
| DOM-08 | POST uses `RegisterInputHandler` | PASS | `resource.go:151` (house signature, `packages/shared-go/server/handler.go:42`) |
| DOM-09 | Transform errors handled | PASS | Transform functions are total (`rest.go`); no `_` discards |
| DOM-11 | No `os.Getenv()` in handlers | PASS | config read at `cmd/main.go:212-233` |
| DOM-12 | No cross-domain logic in handlers | PASS | vehicle status injected as a port (`browse.go:49-51`, `cmd/main.go:325-330`) |
| DOM-13 | Handlers don't call providers | **FAIL** | `resource.go:122, 138, 228` (I3) |
| DOM-14 | No `db.Create`/`Save`/`Delete` in handlers | PASS | zero matches in `resource.go` |
| DOM-15 | `administrator.go` for writes | PASS | `administrator.go:13-70`, called only from `processor.go`/`reaper.go` |
| DOM-16 | Domain error → HTTP status | PASS | `resource.go:24-35`, sentinels at `errors.go:6-15`, `StatusFor` at `:17-40` |
| DOM-18 | Flat request models | PASS | `resource.go:152-157` (flat; see M4) |
| DOM-19 | Table-driven tests | WARN | mostly scenario tests; table-driven at `browse_test.go:66, 108` |

### Sub-domain checklist — the three internal route trees

| ID | Check | Status | Evidence |
|---|---|---|---|
| SUB-01 | Logic outside the handler | PASS (media/notification) | `operations.go` per service |
| SUB-01 | | WARN (auth) | `platformadmin/resource.go:57, 89-90, 104, 110-112` — raw SQL inline in handlers rather than in `provider.go`, which exists (`provider.go:26-46`) |
| SUB-02 | Writes not in `resource.go` | WARN (media) | `media-service/internal/admin/resource.go:135-142` executes UPDATEs inline (and is the I1 defect) |
| SUB-03 | Typed input for POST | WARN | `json.NewDecoder` used deliberately: these are internal service-to-service routes, not JSON:API (`media resource.go:62`, `notification resource.go:58`) |
| SUB-04 | No manual JSON parsing | WARN | same as SUB-03 |

### SEC checks

| ID | Check | Status | Evidence |
|---|---|---|---|
| SEC-01 | Verified JWT parsing | PASS | `packages/shared-go/auth/middleware.go:40-44` — `ParseWithClaims` with keyfunc, `WithValidMethods(["RS256"])`, `!tok.Valid` rejected. No `ParseUnverified` anywhere. |
| SEC-01b | Claim fails closed | PASS | `middleware.go:95-98` — non-boolean `platform_admin` reads as false; covered by `middleware_test.go:118-152` |
| SEC-02 | Revocation is durable and re-verified | PASS | tombstone not delete (`platformadmin/entity.go:33-39`), both seed hooks check `IsRevoked` (`seed.go:64-67`, `cmd/main.go:96-103`), purge re-verifies and fails closed (`processor.go:111-119`, `adminclient/auth.go:90-103`) |
| SEC-03 | No open redirect | PASS | no redirect handler added by this task; `oidc` redirect targets come from config (`cmd/main.go:116-118`) |
| SEC-04 | No hardcoded secrets | PASS | `config.MustGet` for every secret (`cmd/main.go:106-115`); no default for `PLATFORM_ADMIN_BOOTSTRAP_EMAILS` (`cmd/main.go:48`) |

---

## Summary

### Blocking

- **C1** — `admin/processor.go:338-341` + `provider.go:67-68` + `reaper.go:75-81`: a partially-cancelled purge stays in `ListDue` and is hard-deleted downstream after the window, then marked `reaped` with an all-zero audit row.
- **C2** — `admin/processor.go:392-394`: `Retry` is accepted on a partially-cancelled operation and re-purges downstream.
- **I1** — `media-service/internal/admin/resource.go:135-142`: a failed MinIO removal detaches the row from the operation, making it permanently unreachable by restore, reap or sweep, while the retry reports success.
- **I2** — `deploy/k8s/infra-local/ingressroute.yaml`, `deploy/compose/docker-compose.yml:197-201`: the new unauthenticated destructive notification routes have no internal-deny rule in the local overlay or in compose (`kustomize build overlays/local | grep -c internal-deny` → 0).
- **I3** — `admin/resource.go:122, 138, 228`: handlers call `proc.d.Provider` directly, violating `anti-patterns.md:13`.

### Non-blocking

- **M1** `ingressroute.yaml:105-107` — media stripprefix comment is arithmetically wrong.
- **M2** `admin/arch_test.go:132` — separation guard covers only `fleet-service/internal`.
- **M3** `admin/resource.go` (10 sites) — handler logs carry no correlation id.
- **M4** `admin/resource.go:152-157` — anonymous inline request struct.
- **M5** `admin/model.go:79-81, 104, 107, 163` — `Make*` returns no error; jsonb decode errors swallowed.
- **M6** three internal route trees leak raw DB error text into the response title.
- **M7** `admin/targets.go:84-97` — `requireRecord` does not filter `deleted_at`.
