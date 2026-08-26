# Admin Vehicle Fleet Transfer — Design

Version: v1
Status: Draft
Created: 2026-08-25
PRD: `docs/tasks/task-031-admin-vehicle-fleet-transfer/prd.md`

---

## 1. Summary

The transfer is one transaction in fleet-service plus one call to media-service.
Nothing about it is novel: `internal/admin` already rewrites rows across every
domain in the service, from one transaction, driven by a hand-enumerated
manifest of raw set-based SQL, with the blast-radius preview and the applied
write sharing a single set of predicates. This design reuses that machinery
rather than inventing a parallel one.

The whole feature is a new `TransferPlan` — a table of statements analogous to
`admin.Manifest` — plus `PreviewTransfer` / `ApplyTransfer` analogous to
`admin.Count` / `admin.Stamp`, a `Reassign` method on `adminclient.MediaClient`,
a `POST /internal/admin/reassign-fleet` route in media-service, and a dialog in
the console.

Section 3 records twelve decisions. Five of them deviate from the PRD; each is
flagged **PRD-DEVIATION** and needs sign-off before implementation.

---

## 2. Verified ground truth

Everything below was read from source in this worktree, not recalled. It
resolves all five of the PRD's open questions and corrects four statements in
the PRD.

### 2.1 Where `fleet_id` and `vehicle_id` actually live

Every `TableName()` in `apps/fleet-service/internal/*/entity.go`:

| Table | `fleet_id` | `vehicle_id` | Transfer treatment |
|---|---|---|---|
| `fleet.vehicles` | yes (`not null`) | — (is the vehicle) | **rewrite** |
| `fleet.activity_events` | yes (`not null`) | yes (`*string`, nullable) | **rewrite where `vehicle_id = ?`** |
| `fleet.maintenance_categories` | yes (`*string`, NULL = system) | — | **find-or-create in destination** |
| `fleet.dashboards` | yes (`not null`) | — | untouched |
| `fleet.dashboard_widgets` | **no** — joins via `dashboard_id` | no — `vehicleId` lives inside `config` jsonb | **delete source-fleet rows pinned to the vehicle** |
| `fleet.fleet_memberships` | yes | — | untouched (non-goal) |
| `fleet.fleet_invites` | yes | — | untouched (non-goal) |
| `fleet.maintenance_records` | no | yes (`not null`) | `category_id` remapped only |
| `fleet.maintenance_schedules` | no | yes (`not null`) | `category_id` remapped only |
| `fleet.maintenance_record_documents` | no | no (joins via `maintenance_record_id`) | untouched; source of receipt media IDs |
| `fleet.fuel_logs` | no | yes (`not null`) | untouched |
| `fleet.mileage_records` | no | yes (`not null`) | untouched |
| `fleet.vehicle_media` | no | yes (`not null`) | untouched; source of photo media IDs |
| `fleet.purge_operations`, `fleet.admin_audit_events` | n/a | n/a | audit row appended |

`media.media_objects.fleet_id` is `type:uuid;not null;index`
(`apps/media-service/internal/mediaobject/entity.go:12`). Its child tables key on
`media_object_id` (`apps/media-service/internal/admin/manifest.go:48,67`), so
rewriting `media_objects.fleet_id` alone re-homes the whole media subtree.

### 2.2 Resolution of the PRD's open questions

- **OQ-1 (receipt media columns) — RESOLVED.** `fleet.maintenance_record_documents`
  (`apps/fleet-service/internal/maintenancerecord/entity.go:31-39`):
  `MaintenanceRecordID uuid not null`, `MediaID uuid not null`, soft-deletable.
  It is the only attachment table. The full media-ID union for a vehicle is
  therefore:
  1. `fleet.vehicle_media.media_id WHERE vehicle_id = ? AND deleted_at IS NULL`
  2. `fleet.vehicles.primary_image_media_id WHERE id = ?` (a plain `string`
     column, **not** nullable — empty string means "none", so it must be
     filtered with `<> ''`, not `IS NOT NULL`;
     `apps/fleet-service/internal/vehicle/entity.go:20`)
  3. `fleet.maintenance_record_documents.media_id` joined to
     `fleet.maintenance_records WHERE vehicle_id = ?`, both live.

- **OQ-2 (audit source/destination columns) — DECIDED.** Add two nullable
  columns. See D3.

- **OQ-3 (notify destination members) — deferred**, as the PRD states. No change.

- **OQ-4 (stale vehicle→fleet caches) — RESOLVED, none exist.** There is no
  cache layer in fleet-service keyed on vehicle→fleet. The admin console's own
  staleness is handled by React Query invalidation, and every admin mutation
  already invalidates the whole `adminKeys.all` subtree on settle
  (`apps/web/src/lib/hooks/api/admin.ts:97-127`). Invalidation is purely
  client-side, as the PRD hoped.

- **OQ-5 (`mileage_records` fleet-derivation) — RESOLVED, vehicle-only.**
  `apps/fleet-service/internal/mileage/entity.go:10-21` has no `fleet_id`;
  neither does `fuel_logs` (`internal/fuel/entity.go:10-23`). Nothing re-derives
  a fleet from either. They follow the vehicle for free.

### 2.3 Corrections to the PRD

1. **FR-XFER-MEDIA-1 says `403`. The real status is `404`.**
   `AuthorizeAccess` returns `server.ErrNotFound`, deliberately, so cross-fleet
   existence is never leaked
   (`apps/media-service/internal/mediaobject/processor.go:135-143`). The
   acceptance criteria that mention a 403 from media must read 404.

2. **There is no `502` anywhere in this codebase.** `server.StatusFor`
   (`packages/shared-go/server/errors.go:24-50`) has no `ErrBadGateway`; the
   carrier for "a downstream is unavailable" is `ErrServiceUnavailable` → 503.
   See D2.

3. **FR-XFER-MOVE-2 contradicts FR-XFER-CAT-5.** MOVE-2 requires rows in
   `fleet.maintenance_records` and `fleet.maintenance_schedules` to be
   "byte-identical before and after"; CAT-5 requires their `category_id` to be
   rewritten. Both cannot hold. See D10.

4. **"notification-service — no changes. Nothing they own is vehicle-scoped" is
   wrong.** `notification.notifications` carries **both** `VehicleID` and a
   nullable, indexed `FleetID`
   (`apps/notification-service/internal/notification/entity.go:25-26`) — the
   column comment says it is indexed precisely "because a fleet-scoped admin
   purge selects on it". It is the same stored-not-derived staleness as
   `activity_events`, in another service. See D12.

5. **The `/internal/admin/reassign-fleet` route needs no gateway change**, but
   the PRD was right to demand the check. Verified: `deploy/k8s/overlays/main/ingressroute.yaml:111-118`
   matches `PathRegexp("(?i)^/+api/+media[^/]*/*internal")` at priority 200 and
   applies the `internal-deny` middleware, an `ipAllowList` of
   `255.255.255.255/32` with `rejectStatusCode: 403` (`:15-23`) — an unroutable
   /32 matches no client. The local overlay has the identical rule
   (`deploy/k8s/infra-local/ingressroute.yaml:67-74`). Independently,
   `media-stripprefix` strips only `/api`
   (`deploy/k8s/base/routing/middlewares.yaml:28-38`), so a public
   `/api/media/internal/...` would arrive at chi as `/media/internal/...`, which
   matches no registered route. Two independent reasons; no new config, but a
   regression test (see §8).

### 2.4 Patterns this design must obey

- **`internal/admin` does not import other domain packages.** Cross-domain work
  is raw SQL against schema-qualified tables (`targets.go`, `operations.go`,
  `browse.go`) or a locally-declared port with the adapter in the composition
  root (`browse.go:42-51` + `cmd/main.go:333-355`). This is the single most
  important constraint on the design and it contradicts the PRD's §7 plan.
  See D1.
- **`arch_test.go:26-86`** requires every `TableName()` under `internal/` to
  appear in `Manifest` or `excludedTables`. This design adds no tables, so it is
  unaffected — but it is why D3 adds columns rather than a `vehicle_transfers`
  table.
- **`arch_test.go:99-181`** forbids `PlatformAdmin` outside
  `internal/admin` + `internal/adminclient`, and forbids `RequireSameFleet(` inside
  them. All new Go code stays in those two packages.
- **`Administrator` methods take an explicit `tx *gorm.DB`**
  (`internal/admin/administrator.go:13-21`) so a write can join the caller's
  transaction. New methods follow suit.
- **There are no SQL migration files in this repo.** Migrations are per-package
  `func Migration(db *gorm.DB) error` running `AutoMigrate`, registered in
  `cmd/main.go`. Adding nullable columns is a one-line entity change.
- **Tests run on SQLite** via hand-written DDL
  (`internal/admin/admintest/db.go`), because AutoMigrate cannot build
  schema-qualified tables there. Any SQL this design emits must be valid on both
  Postgres and SQLite. See D5.

---

## 3. Decisions

### D1 — Transfer logic lives in `internal/admin` as a declarative plan of set-based SQL

**PRD-DEVIATION** (supersedes the §7 Service Impact breakdown).

The PRD proposes adding admin-only methods to seven domain packages:
`vehicle`, `activity`, `maintenancecategory`, `maintenancerecord`,
`maintenanceschedule`, `dashboard`, plus `adminclient`. That would scatter an
admin-only concern across the service, add methods no ordinary caller uses, and
break the convention `browse.go:42-51` documents explicitly — *"this package
never calls another domain's internals directly"*.

The purge path already solves exactly this problem. `admin.Manifest`
(`internal/admin/manifest.go`) is a hand-enumerated table of targets, each with a
`Where` closure, and `Count`/`Stamp` (`internal/admin/operations.go:15-67`) walk
it to produce the preview and the write **from the same predicates**. That
shared-predicate property is what makes "the preview equals what happened" a
structural fact rather than a discipline, and it is precisely the property
FR-XFER-UI-4 and the acceptance criterion on preview parity need.

**Decision.** Add `internal/admin/transfer.go` containing a `TransferPlan`: an
ordered list of steps, each with a key, a table, a count statement and an apply
statement over the same predicate. `PreviewTransfer(db, spec)` walks it counting;
`ApplyTransfer(tx, spec)` walks it writing. No domain package changes at all.

`internal/admin` grows, and it is already dense (`processor.go` 454 lines,
`browse.go` 498). So the transfer gets its **own files** rather than being added
to existing ones:

| File | Contents |
|---|---|
| `internal/admin/transfer.go` | `TransferSpec`, `TransferPlan`, `PreviewTransfer`, `ApplyTransfer`, media-ID union query, category resolution, widget pruning |
| `internal/admin/transfer_processor.go` | `Processor.PreviewVehicleTransfer`, `Processor.TransferVehicle` — orchestration, validation, confirmation, audit, compensation |
| `internal/admin/transfer_test.go`, `transfer_processor_test.go` | table-driven and DB-backed tests |

Routes are added to the existing `resource.go` (the file is a single route
tree; splitting it would be gratuitous), REST transforms to `rest.go`, the
action constant to `entity.go`.

**Rejected alternative — per-domain Administrator methods (the PRD's plan).**
Honest upside: each domain owns its own SQL, and a future non-admin transfer
would reuse it. But there is no planned non-admin transfer (it is an explicit
non-goal), it violates the documented boundary, and it splits one atomic
operation across seven packages whose only common caller is the one being
written. YAGNI.

### D2 — Media-failure status is `503`, not `502`

**PRD-DEVIATION** (supersedes §5.2's error table and FR-XFER-MEDIA-5).

`packages/shared-go/server/errors.go` has no 502 sentinel. `ErrServiceUnavailable`
(→503) exists and its doc comment states its purpose exactly: *"the answer is
UNKNOWN because something this request depends on is unavailable — not that the
request was wrong. It is the carrier for that distinction across package
boundaries."* That is this case precisely.

**Decision.** Return `server.Detailed(server.ErrServiceUnavailable, "media-service
could not reassign the vehicle's media; the transfer was rolled back")` → **503**.
Add `server.ErrServiceUnavailable` to `isClientError`? **No** — a 503 is an
incident and must stay logged at error level.

**Rejected alternative — add `ErrBadGateway` to shared-go.** A new sentinel in a
shared package with exactly one call site, purely to match a number written in a
PRD, when a semantically correct one already exists. The console surfaces the
server's `detail` verbatim (FR-XFER-UI-7), so the operator sees the same
sentence either way.

### D3 — Audit gains two nullable columns

Resolves **OQ-2**. FR-XFER-AUDIT-4 forbids encoding-only in `target_label`.

**Decision.** Add to `AuditEntity` (`internal/admin/entity.go:52-70`) and
`AuditEvent` (`model.go:58-71`):

```go
SourceFleetID      *string `gorm:"type:uuid"`
DestinationFleetID *string `gorm:"type:uuid"`
```

Nullable so every existing `purge.*` row stays valid, exactly as the PRD's
migration note requires. `admin.Migration`'s `AutoMigrate` adds them on the next
boot; no migration file, because this repo has none. `MakeAudit`/`ToEntity`
round-trip them, `TransformAudit` exposes them as `source_fleet_id` /
`destination_fleet_id`, and the frontend `AuditEventAttributes` gains two
optional fields.

New action constant beside the existing four (`entity.go:72-78`):

```go
ActionVehicleTransferred = "vehicle.transferred"
```

`target_type = "vehicle"`, `target_id` = vehicle ID, `target_label` = the
confirmation label (FR-XFER-CONF-2) — human-facing text only, per AUDIT-4.
`purge_operation_id` stays NULL: a transfer is not a purge.

**Rejected alternative — a sibling jsonb column, or reusing `affected_counts`.**
`affected_counts` is `map[string]int`; fleet IDs are strings. Overloading it
would break its type and the `TestManifestKeysAreUnique` invariant that protects
it. A third jsonb column for two scalar UUIDs is strictly worse than two
columns.

### D4 — One transaction, with the downstream calls inside it

**PRD-DEVIATION** (refines §8's ordering).

The PRD specifies: call media **first**, then run the local transaction, and on
local failure compensate with a reverse media reassign.

That ordering has a concurrency gap the PRD's own NFR asks us to close. §8
requires `SELECT ... FOR UPDATE` on the vehicle "for the duration of the
transaction" so two concurrent transfers serialise. But if the media call
happens *before* the transaction opens, the lock is not held during it — two
concurrent transfers of the same vehicle to *different* destinations can both
call media, and the last writer wins in media-service while the first wins in
fleet-service, or vice versa. The vehicle ends up in one fleet with its photos
in another. Idempotency does not help: the two calls have different destinations,
so they are not replays of each other.

It also compensates too often. Under the PRD's ordering, *every* local failure —
a category unique violation, a widget delete error, a serialization failure —
requires a reverse media call.

**Decision.** One `db.Transaction`, structured as:

1. `SELECT ... FROM fleet.vehicles WHERE id = ? FOR UPDATE` — lock first, so
   everything after it is serialised per vehicle.
2. All eligibility checks (§4.1) and the confirmation check (§4.2). Any failure
   returns before a single write or cross-service call — satisfying
   FR-XFER-CONF-3's "no audit row, no cross-service call" literally.
3. Compute the media-ID union (read-only).
4. All local writes: `vehicles.fleet_id`, `activity_events.fleet_id`, category
   find-or-create, `category_id` remaps, widget deletes, the two activity events,
   the audit row.
5. **Then** call media-service. On failure, return the error → the whole
   transaction rolls back → nothing local changed and media never changed.
6. Commit.

Compensation is now needed only if the *commit itself* fails after media
succeeded — a genuinely rare window rather than the common path. That case keeps
the PRD's handling: attempt a reverse `Reassign` back to the source fleet; if it
also fails, log at error naming both fleet IDs and every media ID, and return
503. FR-XFER-MEDIA-4's idempotency is what makes the reverse call safe.

**Cost, stated plainly.** This holds a Postgres transaction open across an HTTP
call. That is normally an anti-pattern — it ties up a pooled connection for the
duration. Here it is bounded and acceptable: `adminclient`'s
`clientTimeout = 5 * time.Second` (`internal/adminclient/http.go:23`) caps the
window, the operation is platform-admin-only and rare (a handful per month, not
per second), and both services sit on the same cluster network. If it ever
becomes a problem the fix is a queue, not a reordering.

**Rejected alternative — the PRD's ordering.** Cheaper on connections, but leaves
the concurrency gap above and compensates on every local error. Given that
transfers are rare, connection-hold is the cheaper of the two costs.

**Rejected alternative — a two-phase / saga protocol in media-service.** Correct
and unnecessary. It would mean a new state column on `media_objects` and a
reaper to time out prepared transfers, for an operation performed a few times a
month.

### D5 — Widgets are pruned by parsing config in Go, not by jsonb SQL

`fleet.dashboard_widgets` has no `fleet_id`; it joins via `dashboard_id` to
`fleet.dashboards`, which does (`internal/dashboard/entity.go:16-40`). `Config` is
`json.RawMessage` with `gorm:"type:jsonb"`.

FR-XFER-SRC-3 requires the match to be on the **parsed** `vehicleId`, never a
substring scan. Postgres would express that as `config->>'vehicleId' = ?`. But
every DB-backed test in `internal/admin` runs on SQLite
(`internal/admin/admintest/db.go`), which has no `->>` operator, and the codebase
already branches on `db.Name() == "sqlite"` where it must
(`internal/dashboard/entity.go:56-60`). A dialect branch for a *predicate* — as
opposed to a one-off DDL statement — would mean the tested path and the
production path are different SQL, which is exactly the class of bug the
`kubectl --dry-run` lesson in `CLAUDE.md` is about.

**Decision.** Two statements, no dialect branch:

```sql
-- 1. candidates: live widgets on live dashboards of the SOURCE fleet
SELECT w.id, w.config
  FROM fleet.dashboard_widgets w
  JOIN fleet.dashboards d ON d.id = w.dashboard_id
 WHERE d.fleet_id = ? AND w.deleted_at IS NULL AND d.deleted_at IS NULL
```

Unmarshal each `config` in Go into `struct { VehicleID string \`json:"vehicleId"\` }`,
keep the IDs where it equals the moved vehicle, then:

```sql
-- 2. delete exactly those
DELETE FROM fleet.dashboard_widgets WHERE id IN ?
```

This satisfies SRC-1 (source-fleet widgets pinned to the vehicle go), SRC-2 (the
join scopes it to the source fleet — destination and third-fleet widgets are
never candidates), and SRC-3 (parsed, and a config with no `vehicleId` unmarshals
to the empty string and is skipped). A malformed config is skipped, not fatal —
matching how `MakeAudit` tolerates a malformed `affected_counts`.

The NFR's "never a row-by-row loop" is about the *writes*; the delete is a single
set-based statement. One dashboard per (fleet, user) is enforced by a partial
unique index (`dashboard/entity.go:52-60`), so the candidate set is bounded by
members × widgets-per-dashboard — tens of rows, not thousands.

**Rejected alternative — `config->>'vehicleId' = ?` with a SQLite
`json_extract` branch.** Production SQL that no test exercises.

**Decision on DELETE vs soft-delete.** Hard `DELETE`. FR-XFER-SRC-1 says
"deleted", these rows carry no history, and a soft-deleted widget would still
occupy the layout's position grid. Note this is the one place the transfer
destroys data; it is bounded, it is counted in `widgets_removed`, and it is what
the operator is shown in the blast-radius panel before confirming.

### D6 — Category find-or-create resolves a unique violation by re-reading

FR-XFER-CAT-3 matches on case-insensitive `name` **and** exact `kind`, against
the destination fleet. The backing index
`idx_maintenance_categories_scope` is `(fleet_id, name, kind)` and
case-**sensitive** (`internal/maintenancecategory/entity.go:24-35`), so the
lookup and the constraint disagree by design — the index is a backstop against
double-submit, the `LOWER()` lookup is the real match.

**Decision.** Per distinct source-fleet category referenced by the moved
vehicle's live records or schedules:

```sql
SELECT id FROM fleet.maintenance_categories
 WHERE fleet_id = ? AND LOWER(name) = LOWER(?) AND kind = ?
 LIMIT 1
```

On a hit, reuse. On a miss, `INSERT` a new row copying `name`, `description`,
`kind`, with `fleet_id` = destination and `system_defined = false`
(FR-XFER-CAT-4). If the insert returns a unique violation, re-run the SELECT and
use the winner — the PRD's "someone else created it, re-read it", never a 500.
The retry is bounded to one attempt; a second violation is a real error.

System categories (`fleet_id IS NULL`) are excluded from the candidate set by the
predicate itself, so CAT-2 holds by construction rather than by a check. Source
categories are only ever read, never written — CAT-6.

Both remaps are single set-based statements per resolved pair:

```sql
UPDATE fleet.maintenance_records
   SET category_id = ?
 WHERE vehicle_id = ? AND category_id = ? AND deleted_at IS NULL
```

…and the same shape for `fleet.maintenance_schedules`. `categories_created`
counts only the inserts.

**Rejected alternative — `ON CONFLICT DO NOTHING` / GORM `FirstOrCreate`.**
`ON CONFLICT` is Postgres-specific and the tests are SQLite. `FirstOrCreate`
matches case-sensitively, which is the wrong match.

### D7 — Preview and apply share predicates; media count is the one honest exception

`PreviewTransfer` and `ApplyTransfer` walk the same `TransferPlan`, so every
count in §5.1 is produced by `COUNT(*)` over the identical `WHERE` the `UPDATE`
uses. No row is loaded to be counted (NFR Performance).

The one figure that cannot be guaranteed identical is `media_objects`. Preview
computes it in fleet-service as the size of the distinct media-ID union (§2.2),
without calling media-service at all — which is cheaper and removes the PRD's
anticipated "media-service unreachable" warning from the normal path. The
transfer's reported `media_objects` comes from media-service's read-back count.
The two agree whenever every referenced media ID actually exists, which is the
normal case. They diverge only when a fleet-service row references a media object
that no longer exists in media-service — a pre-existing dangling reference, not
something the transfer causes.

**Decision.** Report both honestly rather than pretend. The preview attribute
`counts.media_objects` is documented as "media references held by this vehicle";
when the applied count is lower, the transfer response's `affected_counts`
carries the real number and the handler logs the difference at info. `warnings`
is retained in the preview response shape for future degradation notes and is
`[]` today.

### D8 — `adminclient` starts propagating the correlation ID

NFR Observability requires the correlation ID to reach media-service. It does not
today: `transport.do` (`internal/adminclient/http.go:52-81`) sets only
`Content-Type`, and `grep -rn "X-Correlation" apps/fleet-service` returns
nothing. media-service's `telemetry.CorrelationID` middleware therefore mints a
*fresh* ID for every internal call, so the existing purge fan-out does not
correlate across services either.

**Decision.** In `transport.do`, set
`X-Correlation-ID: telemetry.CorrelationIDFromContext(ctx)` when non-empty. The
header constant is `packages/shared-go/telemetry/correlation.go:14`; the
middleware already honours an inbound value
(`correlation_test.go:24-32`). This is a three-line change that fixes the gap for
`Reassign` **and** retroactively for `Purge`/`Restore`/`Reap`/`Stats`. Covered by
a test asserting the outbound header.

Scope note: this is a small, in-scope fix to code the feature depends on, in the
spirit of "improve the code you're working in". It changes no behaviour beyond
the header.

### D9 — media-service reassign mirrors the purge stamp exactly

`mediaobject.Administrator` has **no bulk-update method**; every mutation is
single-row `db.Save`. Bulk work in media-service is raw SQL in
`internal/admin/operations.go`. The new route follows that precedent, not the
Administrator.

**Decision.** `POST /internal/admin/reassign-fleet` registered in
`InitializeInternalRoutes` (`apps/media-service/internal/admin/resource.go:39`)
beside the purge routes, with a new `internal/admin/reassign.go` holding:

```go
func Reassign(tx *gorm.DB, mediaIDs []string, destFleetID string) (map[string]int, error)
```

Body `{ "media_ids": [...], "destination_fleet_id": "..." }`; empty `media_ids`
or a missing destination → 422 (`server.ErrValidation`), matching `rootFrom`'s
handling. Response `{ "affected": { "media_objects": N } }`.

Idempotency (FR-XFER-MEDIA-4) is achieved the same way `Stamp` achieves it — the
count is **read back**, not taken from `RowsAffected`:

```sql
UPDATE media.media_objects
   SET fleet_id = ?
 WHERE id IN ? AND fleet_id <> ? AND deleted_at IS NULL;

SELECT count(*) FROM media.media_objects
 WHERE id IN ? AND fleet_id = ? AND deleted_at IS NULL;
```

A replay updates zero rows and returns the same N. Unknown IDs simply do not
match and are ignored, matching the purge path's tolerance. Soft-deleted objects
are left alone — a pending-purge media object is not re-homed.

The route inherits the zero-auth posture of its neighbours; §2.3 item 4 is the
argument that this is safe, and §8 adds the test that keeps it true.

**On chunking:** `adminclient.MaxLookupIDs = 50` and `chunk` exist for
**query-parameter** lookups (`auth.go:48`); `Purge` sends `MediaIDs` in a POST
body unchunked. `Reassign` does the same. A vehicle with hundreds of photos sends
one larger body, which is fine and keeps the operation single-shot — chunking
would reintroduce partial-application across chunks.

### D10 — Resolving MOVE-2 vs CAT-5

**PRD-DEVIATION** (narrows FR-XFER-MOVE-2's table list).

FR-XFER-MOVE-2 requires `maintenance_records` and `maintenance_schedules` rows to
be unchanged; FR-XFER-CAT-5 requires their `category_id` to be rewritten.

**Decision.** CAT-5 wins; MOVE-2's table list is narrowed. The purpose of MOVE-2
is to assert that vehicle-scoped history is *not* rewritten wholesale — that the
transfer does not touch `created_at`, ownership, values, or soft-delete state.
Narrow it to the three tables where "byte-identical" is literally true, and state
the weaker-but-true property for the other two:

- **Byte-identical, asserted:** `fleet.mileage_records`, `fleet.fuel_logs`,
  `fleet.vehicle_media`, `fleet.maintenance_record_documents`.
- **Every column except `category_id` identical, asserted:**
  `fleet.maintenance_records`, `fleet.maintenance_schedules` — and `category_id`
  itself unchanged for rows referencing a **system** category.

That is a stronger test than the PRD's, because it pins down exactly which single
column may move and proves nothing else did.

### D11 — The append-only rule on `activity_events` is narrowed, not silently broken

`fleet.activity_events` is documented as append-only in three places:
*"Append-only: rows are inserted once and never updated or deleted"*
(`internal/activity/entity.go:9-10`), the same sentence on the model
(`model.go:11`), and *"The feed is APPEND-ONLY: there is no Update or Delete"* on
the Administrator (`administrator.go:9-11`). The package exposes only
`Record(tx, …)` — there is no update path at all.

FR-XFER-MOVE-3 requires rewriting `fleet_id` on those rows. That is a genuine
conflict with a stated invariant, and the PRD does not acknowledge it.

**Decision.** Do the rewrite, and narrow the invariant explicitly rather than
leave a contradiction in the tree. The invariant's purpose is that *the feed is
not editable* — no one revises what happened, and no ordinary code path mutates
history. `fleet_id` is not part of "what happened"; it is a denormalised routing
column that answers "whose feed does this appear in". Correcting it when the
vehicle changes owners preserves the feed's meaning rather than revising it.

Concretely:
- The rewrite lives in `internal/admin`, as raw SQL, like every other admin
  cross-domain write. `internal/activity` gains **no** update method, so the
  domain's own API stays append-only and no ordinary caller can mutate an event.
- The three doc comments are amended to say: append-only except for
  `fleet_id`, which an admin vehicle transfer re-points, with a pointer to this
  decision. A comment that is quietly false is worse than one that states its
  exception.
- The rewrite touches `fleet_id` and nothing else — not `type`, not `payload`,
  not `actor_user_id`, not `created_at`. A test asserts every other column is
  byte-identical.

This also fixes a live bug the inventory surfaced: `activity.Provider` has both
`ListByFleet` (`fleet_id = ?`) and `ListByVehicle` (`vehicle_id = ?`)
(`provider.go:29,33`). Without the rewrite, after a transfer the vehicle's
timeline would return events stamped with the *old* fleet — leaking one
household's activity into another's vehicle detail view. MOVE-3 is not a
nice-to-have; it closes a cross-fleet leak.

**Rejected alternative — leave the rows and derive the fleet at read time.**
Correct in the abstract, but it would mean changing every activity read path to
join `fleet.vehicles`, in a feature whose whole point is a one-column
correction, and it would break the fleet-level events that legitimately have no
vehicle.

### D12 — notification-service is in scope, minimally

**PRD-DEVIATION** (supersedes §7's "notification-service — no changes").

`notification.notifications` has `VehicleID` and a nullable indexed `FleetID`
(`apps/notification-service/internal/notification/entity.go:25-26`). After a
transfer, a notification about the moved vehicle still carries the source
fleet's ID. The consequences are smaller than the media case — notifications are
per-user and the affected rows are historical — but "no changes" is not
accurate, and leaving it undecided means it gets discovered in production.

**Decision.** Rewrite it, by the same mechanism as media: a
`POST /internal/admin/reassign-fleet` on notification-service taking
`{vehicle_ids, destination_fleet_id}`, and a `Reassign` method on
`adminclient.NotificationClient` (which already exists —
`internal/adminclient/notification.go` — alongside the media one, with the same
purge/restore/reap shape). It is called inside the same transaction as the media
call, and a failure rolls the transfer back identically.

Two things make this cheap rather than a second feature: the notification client,
the internal route group, the zero-auth posture and the
`^/+api/+notifications[^/]*/*internal` deny rule
(`deploy/k8s/overlays/main/ingressroute.yaml:161-167`) all already exist, and the
count joins `affected_counts` as `notifications` with no new plumbing.

**Honest alternative, if scope must be cut:** defer this and file it, because
unlike the media case it causes no access failure — a stale `fleet_id` on a
notification does not break a read, it only mis-scopes an admin purge selecting
on that column later. If the user prefers to hold v1 to the PRD's stated
surface, this is the one deviation that is safe to drop; it is called out here so
the choice is deliberate rather than accidental. **Recommendation: include it** —
it is a few dozen lines on top of machinery being written anyway.

---

## 4. Architecture

### 4.1 Component view

```
POST /api/fleet/admin/vehicles/{id}/transfer
        │  (Traefik strips /api/fleet)
        ▼
internal/admin/resource.go ── RequirePlatformAdmin ──▶ 403
        │
        ▼
internal/admin/transfer_processor.go
   TransferVehicle(ctx, TransferInput)
        │
        ├─ Auth.IsPlatformAdmin  (fail-closed re-auth, as purge Create does)
        │
        └─ db.Transaction:
               ├─ SELECT ... FOR UPDATE  (vehicle row)
               ├─ eligibility + confirmation  ──▶ 404 / 409 / 422
               ├─ transfer.go: media-ID union       (read)
               ├─ transfer.go: ApplyTransfer        (all local writes)
               ├─ Administrator.InsertAudit(tx, …)
               ├─ adminclient.MediaClient.Reassign         ──▶ err ⇒ rollback ⇒ 503
               ├─ adminclient.NotificationClient.Reassign  ──▶ err ⇒ rollback ⇒ 503  (D12)
               └─ COMMIT      (failure here ⇒ compensating reverse Reassign, both)
                       │
                       ▼
              media-service        POST /internal/admin/reassign-fleet
              notification-service POST /internal/admin/reassign-fleet
                       │
                       ▼
              UPDATE media.media_objects        SET fleet_id = ?
              UPDATE notification.notifications SET fleet_id = ?
```

Both downstream calls are made last, after every local write, so any local
failure short-circuits before either is issued. Media is called first; if
notification then fails, the rollback plus the reverse media `Reassign` restores
both sides.

### 4.2 The transfer plan

`TransferSpec` is the resolved, validated input — the analogue of `admin.Root`:

```go
type TransferSpec struct {
    VehicleID     string
    SourceFleetID string
    DestFleetID   string
    Label         string    // FR-XFER-CONF-2
    Now           time.Time // parameter, not SQL now() — the harness is SQLite
}
```

`TransferPlan` is the ordered list of steps. Each step owns its key, its table
and one predicate used by both the count and the apply:

| Key | Table | Predicate | Apply |
|---|---|---|---|
| `maintenance_records` | `fleet.maintenance_records` | `vehicle_id = ? AND deleted_at IS NULL` | none (counted only) |
| `maintenance_schedules` | `fleet.maintenance_schedules` | same | none (counted only) |
| `fuel_logs` | `fleet.fuel_logs` | same | none (counted only) |
| `mileage_records` | `fleet.mileage_records` | same | none (counted only) |
| `vehicle_media` | `fleet.vehicle_media` | same | none (counted only) |
| `activity_events` | `fleet.activity_events` | `vehicle_id = ? AND deleted_at IS NULL` | `SET fleet_id = dest` |
| `media_objects` | (media-ID union) | §2.2 union | delegated to media-service |
| `notifications` | `notification.notifications` | `vehicle_id = ?` | delegated to notification-service (D12) |
| `categories_created` | `fleet.maintenance_categories` | resolved pairs | find-or-create (D6) |
| `widgets_removed` | `fleet.dashboard_widgets` | source-fleet join + parsed config (D5) | `DELETE` |

The first five steps carry no apply statement on purpose: they are what
FR-XFER-MOVE-2 says must not be rewritten, and the plan makes that visible as an
absence rather than leaving it to a reviewer's memory. A test asserts every
count-only step has a nil apply.

`fleet.vehicles` itself is not a plan step — it is the single `UPDATE` the whole
operation exists to perform, done explicitly with the `created_at` guarantee
(FR-XFER-MOVE-1). Because the write is a targeted
`UPDATE fleet.vehicles SET fleet_id = ?, updated_at = ? WHERE id = ?`, it never
mentions `created_at` at all — strictly stronger than relying on GORM's
`<-:create` tag, which only protects `db.Save`.

Fleet-scoped activity events (`vehicle_id IS NULL`) are never matched by the
`activity_events` predicate, so FR-XFER-MOVE-4 holds by construction.

### 4.3 The two transfer activity events (FR-XFER-SRC-4)

Two rows inserted into `fleet.activity_events`, inside the same transaction,
after the bulk `fleet_id` rewrite (so they are not themselves rewritten):

| | source event | destination event |
|---|---|---|
| `fleet_id` | source | destination |
| `vehicle_id` | the vehicle | the vehicle |
| `type` | `vehicle.transferred_out` | `vehicle.transferred_in` |
| `payload` | `{"counterpart_fleet_id": dest, "vehicle_label": …}` | `{"counterpart_fleet_id": source, …}` |
| `actor_user_id` | the admin | the admin |

Ordering matters: the destination event carries the destination `fleet_id`, and
the source event must survive with the *source* `fleet_id` even though its
`vehicle_id` matches the rewrite predicate. Inserting both after the rewrite is
what makes that true, and a test pins it.

Activity event types are inline string literals today — there are eight of them
across six packages (`vehicle/processor.go:87`, `fuel/administrator.go:145`,
`maintenanceschedule/completion_db.go:77`, `.../processor.go:239`,
`invite/administrator.go:203`, `membership/administrator.go:89,127,131`) and no
constants exist. This design adds two more literals rather than introducing a
constants block: the existing eight live in the domains that emit them, a shared
constants file would have to live in `internal/activity` and be imported by all
six, and doing that as a side-effect of a transfer feature is unrelated
refactoring. The two new literals are declared as constants **local to
`internal/admin`**, which is where they are emitted.

These are activity-feed rows only. No outbox/Kafka event is emitted — the outbox
emitters are enumerated in `internal/events/emit.go:52-84` and a transfer is an
admin correction, not a domain event other services subscribe to. Adding one is
a fast-follow if a consumer ever needs it.

### 4.4 Preview

`GET /admin/vehicles/{vehicleId}/transfer-preview?destination_fleet_id=`

Read-only, no transaction, no writes, no media-service call (D7). Without
`destination_fleet_id` it omits `categories_to_create` and the destination
fields, exactly as §5.1 specifies. `widgets_removed` does not need the
destination — it is a source-fleet fact — so it is always present.

`vehicle_label` is computed once, server-side:
`nickname` if non-empty, else `"{year} {make} {model}"`. It is the string the
console must echo and the string `MatchConfirmation` compares. The console
already derives the identical label for display
(`AdminFleetsPage.tsx:294`), but the dialog must use the **preview's** value, so
there is exactly one source of truth (FR-XFER-CONF-2).

### 4.5 Confirmation

`MatchConfirmation(ScopeFleet, label, supplied)` is reused verbatim
(`internal/admin/confirmation.go:34-49`) — `ScopeFleet` is the branch that
compares `supplied == targetLabel` exactly, with no trimming and no case folding,
and returns `ErrConfirmationMismatch` (409) otherwise. No new confirmation code,
and no new `Scope` value: `Scope` is the *purge* vocabulary
(`manifest.go:15-25`) and adding a `ScopeVehicleTransfer` would leak into
`ValidScopes` and the purge builder. Passing `ScopeFleet` is a deliberate reuse
of a comparison rule, documented at the call site.

---

## 5. Error handling

| Condition | Error value | Status |
|---|---|---|
| not a platform admin | `authz.RequirePlatformAdmin` → `server.ErrForbidden` | 403 |
| admin revoked since token mint | `Auth.IsPlatformAdmin` → `!ok` | 403 |
| unknown vehicle | `server.ErrNotFound` | 404 |
| unknown destination fleet | `server.ErrNotFound` | 404 |
| confirmation mismatch | `admin.ErrConfirmationMismatch` | 409 |
| vehicle soft-deleted / pending purge | `Detailed(ErrConflict, "vehicle is pending purge and cannot be transferred")` | 409 |
| source fleet pending purge | `Detailed(ErrConflict, "source fleet is pending purge and cannot be transferred from")` | 409 |
| destination unavailable | `Detailed(ErrConflict, "destination fleet is not available")` | 409 |
| destination == current fleet | `Detailed(ErrValidation, "vehicle already belongs to that fleet")` | 422 |
| `destination_fleet_id` missing/malformed | `Detailed(ErrValidation, …)` | 422 |
| media reassign failed | `Detailed(ErrServiceUnavailable, …)` (D2) | 503 |
| anything else | `errInternal` | 500 |

The handler uses the established idiom: `if isClientError(err) { WriteError(w, err); return }`,
otherwise log at error and return the redacted `errInternal`
(`resource.go:24-28`). A 503 is deliberately **not** a client error, so it is
logged. `server.Detailed` preserves the sentinel for `errors.Is` while carrying
the human sentence in the JSON:API `detail` field
(`packages/shared-go/server/errors.go:52-75`) — which is what FR-XFER-UI-7's
verbatim surfacing consumes.

**Eligibility ordering.** Checks run cheapest-and-most-specific first so the
operator gets the most actionable message: vehicle exists → vehicle not pending
purge → destination provided and well-formed → destination ≠ current → source
fleet healthy → destination fleet healthy → confirmation. Confirmation is last
because a mismatched phrase on an otherwise-invalid request should report the
real problem.

---

## 6. API contract

### 6.1 `GET /api/fleet/admin/vehicles/{vehicleId}/transfer-preview`

As §5.1 of the PRD, with `warnings: []` (D7) and `counts.media_objects`
documented as media references held by the vehicle. Type
`vehicle-transfer-previews`, `id` = vehicle ID.

### 6.2 `POST /api/fleet/admin/vehicles/{vehicleId}/transfer`

Request body via `server.RegisterInputHandler`
(`packages/shared-go/server/handler.go:47`), which decodes
`{data:{attributes:T}}` and returns 422 on a malformed body:

```go
func(w http.ResponseWriter, req *http.Request, attrs struct {
    DestinationFleetID string `json:"destination_fleet_id"`
    Confirmation       string `json:"confirmation"`
})
```

Because `RegisterInputHandler` takes the `(w, req, attrs)` shape, the handler
inlines `auth.IdentityFromContext` + `RequirePlatformAdmin` rather than using the
`authorized` helper — matching `POST /admin/purge-operations`
(`resource.go:151-161`) exactly.

Response `200`, type `vehicle-transfers`, attributes `vehicle_id`,
`source_fleet_id`, `destination_fleet_id`, `transferred_at`, `affected_counts`.

### 6.3 `POST /internal/admin/reassign-fleet` (media-service)

Per D9. Not JSON:API — the internal admin routes use plain JSON, matching their
neighbours.

---

## 7. Frontend

### 7.1 Components

**`components/admin/VehicleTransferDialog.tsx`** — new, modelled on
`PurgeConfirmDialog.tsx`, reusing its established mechanics:

- reset-on-open **during render**, not in an effect
  (`PurgeConfirmDialog.tsx:109-113`) — the existing test asserts the first frame;
- `dismissible={!isPending}` so it cannot be dismissed mid-flight;
- exact comparison `typed === confirmationPhrase`, no trim, no case fold;
- `onConfirm(typed)` receives **what was typed**, never the expected phrase, so
  the server performs the real comparison (the disabled button is a courtesy);
- `<RequiredMarker />` + `aria-required="true"` on both the destination picker
  and the confirmation input.

It composes the existing **`BlastRadiusPanel`** for counts rather than
re-implementing one, and adds a short list of `categories_to_create` names below
it (FR-XFER-UI-4).

**Destination picker.** `useAdminFleets({ q, deleted: 'exclude', page: 1 })` —
`deleted: 'exclude'` is the existing "live only" filter, which satisfies
FR-XFER-UI-3's "excludes soft-deleted fleets". The source fleet is filtered out
client-side from the results. Precedent for the markup is the successor picker
at `MemberList.tsx:274-297`.

One real problem to handle: `adminKeys.fleetList` inlines the params object into
the query key (`admin.ts:14-16`) and nothing in the admin console debounces, so
search-as-you-type would create one cache entry and one request per keystroke.
The dialog debounces the search term (~250 ms) before passing it to the hook.

**Preview fetching.** `useVehicleTransferPreview(vehicleId, destFleetId)` — a
query enabled only while the dialog is open, re-running when the destination
changes. `enabled: !!vehicleId` follows `useAdminFleet`'s pattern
(`admin.ts:51-58`).

**Confirm enabled** iff a destination is selected **and**
`typed === preview.vehicle_label` **and** not pending (FR-XFER-UI-5).

### 7.2 Page wiring

`AdminFleetsPage.tsx` gains a fourth `<TableHead />` on the Vehicles table
(empty, matching the Members table's action column at `:237`) and a Transfer
button per row. For `v.pending_purge` the button is `disabled` with a `title`
explaining why — the exact pattern the Members table already uses for the
owner-row Remove button (`:254-263`), satisfying FR-XFER-UI-8.

State follows `FleetDetail`'s existing shape, with the target vehicle instead of
a boolean:

```tsx
const [transferTarget, setTransferTarget] = useState<AdminVehicleRow | null>(null);
```

`AdminVehicleRow` has no `fleet_id`, so the source fleet ID comes from the
enclosing `FleetDetail`'s `id` — which is correct by construction, since the row
is being rendered inside that fleet's detail.

### 7.3 Hook

`useTransferVehicle()` in `lib/hooks/api/admin.ts`, following `useCreatePurge`:
`onSettled` invalidates `adminKeys.all` — deliberately the whole subtree, because
a transfer changes the source fleet detail, the destination fleet detail, both
fleets' vehicle counts in the list, stats, and the audit log at once. That
satisfies FR-XFER-UI-6's "vehicle disappears without a manual refresh".

`onError` surfaces the server's `detail` **verbatim** (FR-XFER-UI-7). This is a
deliberate departure from `useCreatePurge`, which maps 409/403 to fixed strings
(`admin.ts:112-124`): a purge has exactly one 409 meaning, whereas a transfer has
four distinct 409/422 conditions whose whole value is the specific sentence.
`ApiError` already carries `detail`
(`packages/shared-ts/src/errors.ts:3-16,23-35`), it is simply unused by the admin
hooks today:

```ts
const e = createErrorFromUnknown(err);
toast.error(e.detail || e.message || 'Could not transfer the vehicle');
```

On success, a `toast.success` naming the destination fleet (FR-XFER-UI-6). This
is a new convention for the admin console — no admin hook currently shows a
success toast — and it is justified: unlike a purge, which lands the operator on
a queue page that confirms it happened, a transfer has no such destination. The
precedent for the call itself is `invites.ts:192`.

### 7.4 Types

`types/models/admin.ts`:

- widen `AuditAction` (`:111`) with `'vehicle.transferred'`;
- add two optional fields to `AuditEventAttributes` for the new columns (D3);
- add `VehicleTransferPreviewAttributes`, `VehicleTransferAttributes`,
  `TransferVehicleInput`.

`AdminService.ts`: `previewVehicleTransfer(vehicleId, destFleetId?)` following
`withQuery`, and `transferVehicle(vehicleId, attributes)` following
`createPurge`'s `{ data: { type, attributes } }` body shape.

`AdminAuditPage.tsx` has **two** parallel structures needing an entry —
`ACTIONS` (`:24-30`, the filter buttons) and `ACTION_LABELS` (`:32-37`, the
badge). Adding only the second would satisfy the badge and silently omit the
filter, so both get `'vehicle.transferred' → 'Transferred'`. `ACTION_LABELS` has
a safe fallback at `:115` (`?? a.action`), so a missed entry degrades rather than
blanks — which is precisely why it needs an explicit test.

---

## 8. Testing

Backend tests are DB-backed against the shared SQLite fixture
(`internal/admin/admintest/db.go`), which must gain DDL for any column added in
D3. Frontend tests are Vitest + Testing Library with `vi.mock` on the hooks
module; there is no MSW.

**fleet-service — transfer mechanics**

- 403 for a non-platform-admin caller; 403 when `IsPlatformAdmin` reports revoked.
- Mismatched confirmation → 409 **and** three negative assertions: `fleet_id`
  unchanged, no `admin_audit_events` row, and the media client never called
  (a fake recording invocations, asserted zero — not merely "not yet").
- Happy path: `fleet_id` = destination, `created_at` byte-identical.
- D10's split assertion: byte-identical for `mileage_records`, `fuel_logs`,
  `vehicle_media`, `maintenance_record_documents`; every-column-but-`category_id`
  for records and schedules.
- `activity_events` with the vehicle carry the destination fleet; a fleet-level
  event with `vehicle_id IS NULL` is untouched.
- Category remap: a source-fleet custom category produces a destination category
  with the same name/kind/description and `system_defined = false`; a system
  category (`fleet_id IS NULL`) is **not** remapped; the source category row still
  exists afterwards.
- Case-insensitive reuse: a destination category differing only in case is
  reused, not duplicated (`categories_created` = 0).
- Unique-violation path re-reads instead of 500.
- Widgets: source-fleet widget pinned to the vehicle is gone; a destination-fleet
  widget, a third-fleet widget, a widget pinned to a *different* vehicle, and a
  widget whose config has no `vehicleId` all survive. A widget whose config is
  malformed JSON is skipped without failing the transfer.
- Both transfer activity events exist with the correct `fleet_id` each, and the
  source event was **not** swept up by the bulk rewrite.
- Every eligibility branch in §5 returns its documented status and detail.
- Audit row: action, actor, target label, correlation ID, both fleet ID columns,
  and every key of `affected_counts`.
- Preview parity: `PreviewTransfer` counts equal the subsequent
  `ApplyTransfer` counts for every key except `media_objects`, and equal that too
  when no reference is dangling.
- Plan integrity: every count-only step has a nil apply (guards MOVE-2 from
  being quietly weakened later).
- Media failure → 503, full rollback: vehicle still in the source fleet, audit
  table empty, categories not created, widgets intact.
- Concurrency: two transfers of the same vehicle serialise. Given the SQLite
  harness, this is asserted at the SQL level (the lock statement is issued before
  any write) rather than by racing goroutines, and noted as such.

**media-service**

- `Reassign` moves the named objects and returns the read-back count.
- Replay is a no-op returning the **same** count (FR-XFER-MEDIA-4).
- Unknown media IDs are ignored, not an error.
- Soft-deleted objects are not re-homed.
- Empty `media_ids` / missing destination → 422.
- A destination-fleet identity passes `AuthorizeAccess`; the source-fleet
  identity now gets **404** (§2.3 correction).

**notification-service (D12)**

- `Reassign` re-points `fleet_id` for the vehicle's notifications; replay is a
  no-op with the same count; unknown vehicle IDs are ignored; empty input → 422.

**activity append-only (D11)**

- The rewrite changes `fleet_id` and nothing else: `type`, `payload`,
  `actor_user_id` and `created_at` are byte-identical afterwards.
- `internal/activity` still exposes no update or delete method — asserted by the
  absence of one on the `Administrator` interface, so a future addition is a
  deliberate act rather than a drift.

**adminclient**

- `Reassign` posts the expected body to the expected path and parses `affected`.
- Non-200 becomes an error (`expectOK`).
- `X-Correlation-ID` is set from context (D8), and absent when the context has
  none.

**Gateway (deploy)**

- The PRD demands verification rather than assumption. Beyond the manual
  `kustomize build` + `kubectl --dry-run=server` gate in `CLAUDE.md` (both
  overlays), add an assertion that the priority-200 `internal-deny` rule matching
  `^/+api/+media[^/]*/*internal` is present in both `overlays/main` and
  `infra-local`. The rule predates this task; the test keeps it from being
  deleted by someone who does not know a zero-auth fleet-reassignment endpoint
  now sits behind it.

**web**

- `AdminFleetsPage.test.tsx`'s `vi.mock` factory is **exhaustive** — it will
  break the moment the page calls `useTransferVehicle`, so the factory gains the
  new hook.
- Transfer button renders per row; disabled with a tooltip when `pending_purge`.
- Dialog: confirm disabled until destination chosen **and** phrase typed exactly;
  near-miss casing keeps it disabled; `onConfirm` receives the typed value.
- Destination picker excludes the source fleet and requests `deleted: 'exclude'`.
- Blast-radius panel renders preview counts and `categories_to_create`.
- Negative assertions use `test/expectNoCall.ts` (task-019's rule), e.g. the
  mutation is not called while the phrase is wrong.
- Hook test: `onSettled` invalidates `adminKeys.all`; `onError` surfaces
  `detail` verbatim.
- `AdminAuditPage`: `vehicle.transferred` appears in the filter row **and**
  renders its badge label.
- `requiredFieldMarkers.test.ts` gains an entry for the new dialog's required
  fields (it source-scans hand-rolled, non-react-hook-form surfaces; the existing
  entry for `PurgeConfirmDialog` at `:145-158` is the template).

**Gate:** `make ci` (lint-check, vet, test, build, fe-test, fe-build), plus both
`kustomize build` renders.

---

## 9. Risks

| # | Risk | Mitigation |
|---|---|---|
| R1 | Transaction held open across an HTTP call exhausts the connection pool | Bounded by the 5 s `clientTimeout`; admin-only and rare. D4 states the trade explicitly. |
| R2 | Commit fails after media succeeded, leaving photos in the destination and the car in the source | Compensating reverse `Reassign`, safe because idempotent; on double failure, an error log naming both fleets and every media ID, and a 503. |
| R3 | A future table gains a `fleet_id` or a vehicle reference and is silently missed | `arch_test.go:26-86` already forces every new table into `Manifest`/`excludedTables`; §2.1's table is reproduced as a comment on `TransferPlan` so the next author sees the transfer question at the same moment. |
| R4 | `/internal/admin/reassign-fleet` becomes publicly reachable | Two independent gateway reasons (§2.3.4) plus the new deploy assertion. It is a higher-value target than the purge routes — it can move any media object into any fleet — so it is called out here rather than left implicit. |
| R5 | Widget config schema changes and `vehicleId` moves | Parsed, not substring-matched (D5); a config without the key is skipped rather than mis-deleted. |
| R6 | Operator transfers the wrong vehicle | Server-verified typed label, preview-supplied so client and server agree on one string; blast radius shown before confirm; audit row records everything; the reverse transfer is the documented undo. |
| R7 | Dangling media references make preview and applied counts differ | D7 reports both honestly and logs the difference rather than hiding it. |
| R8 | A user holding a JWT minted before the transfer keeps the old `active_fleet_id` and can still read the vehicle until the token is re-issued | Pre-existing platform behaviour, not introduced here; the codebase already treats claims as potentially stale and re-checks against the DB where it matters (`vehicle/resource.go:196-199`). Ordinary vehicle routes call `RequireSameFleet(identity, v.FleetID())` against the **live** row, so a source-fleet member loses access on their next request regardless of their token. Documented so it is not rediscovered as a bug. |
| R9 | `activity_events`' append-only comment becomes quietly false | D11 amends all three comments rather than leaving a contradiction, and keeps the domain API mutation-free. |

---

## 10. Out of scope

Unchanged from the PRD's non-goals: no self-service transfer, no bulk transfer,
no undo endpoint (the reverse transfer is the correction), no membership/user/
invite movement, no create-on-transfer, and no *new* notification to the
destination fleet's members announcing an arrival (OQ-3, deferred).
auth-service is untouched.

notification-service **is** touched, contrary to the PRD — but only to re-point
the `fleet_id` on existing notification rows (D12), not to send anything new.

Also explicitly out of scope, surfaced during research and deliberately not
addressed here:

- **Activity event type constants.** Eight inline literals across six packages;
  unrelated refactoring (§4.3).
- **A Kafka/outbox event for transfers.** No consumer needs one today (§4.3).
- **Chunking large media-ID sets.** `Purge` does not chunk either; chunking would
  reintroduce partial application (D9).
- **Debouncing the admin fleet search generally.** The transfer dialog debounces
  its own picker (§7.1); fixing the console-wide pattern is its own task.
