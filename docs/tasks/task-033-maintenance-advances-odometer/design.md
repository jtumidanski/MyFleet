# Maintenance Records Advance the Odometer — Design

Task: task-033-maintenance-advances-odometer
PRD: [prd.md](prd.md) (approved)
Created: 2026-10-26

---

## 1. Intent

Every write that carries an odometer reading must go through one code path that
appends history and moves `fleet.vehicles.current_mileage` by the same rule, in
the same transaction as the parent write. Today three of four paths do this by
hand-copied SQL and the fourth (direct maintenance-record logging) does not do it
at all; deletes never undo anything. The frontend then compounds the backend gap
by never refetching the vehicles list after a mileage-bearing mutation.

The design's job is to pick where the shared helper lives, how each consumer
reaches it without breaking package boundaries, how the retract recompute stays
correct under concurrency, and how the frontend changes stay small.

## 2. Current state (verified)

| Path | Mileage row | Odometer advance | Atomic | Location |
|---|---|---|---|---|
| Fuel log create | raw `INSERT INTO fleet.mileage_records` | conditional `UPDATE … current_mileage <= ?` | yes (`LogInTransaction`) | `internal/fuel/administrator.go:107-171` |
| Schedule completion | `tx.Create(&mileage.Entity)`, `recordedAt = time.Now()` | conditional update | yes (`CompleteInTransaction`) | `internal/maintenanceschedule/completion_db.go:127-145` |
| Manual POST | `adm.Insert` (own statement) | `Processor.OnAppend` → `vehicle.Administrator.UpdateCurrentMileage` — **unconditional** set, guarded only by a stale pre-read of `v.CurrentMileage()` | **no** | `internal/mileage/resource.go:111-135`, `processor.go:30-37` |
| Maintenance record create | none | none | n/a | `internal/maintenancerecord/processor.go:42-44` |
| Maintenance record PATCH / DELETE, fuel DELETE | none | none | n/a | `maintenancerecord/resource.go`, `fuel/resource.go:221-243` |

Other facts that shape the design:

- `vehicle.Administrator.UpdateCurrentMileage` (`internal/vehicle/administrator.go:130-136`)
  has exactly one caller: `mileage.Processor.OnAppend`. The PRD's FR-1.3 names it
  as the vehicle-PATCH path; it is not — vehicle PATCH writes `current_mileage`
  through its own update. After consolidation it is dead code.
- Package imports: `mileage → {authz, vehicle}`; `fuel → {authz, vehicle}`;
  `maintenancerecord → {authz, maintenancecategory, vehicle}`;
  `maintenanceschedule → {…, maintenancerecord, mileage, …}`; `vehicle` imports
  none of these. `fuel → mileage` and `maintenancerecord → mileage` therefore
  introduce no cycle.
- `maintenancerecord.Administrator.InsertTx(tx, m)` is the existing precedent for
  a tx-taking write that a foreign orchestration (completion) wraps; the
  completion package consumes it through a local `RecordInserter` interface.
- `maintenancerecord.NewBuilder()` assigns the record ID up front
  (`builder.go:16`), so a mileage entry's `sourceRefID` is known before insert.
- Every reader of `fleet.mileage_records` outside the mileage provider already
  filters `deleted_at IS NULL` (`internal/dashboard/aggregate.go:146-148`, admin
  stats/manifest). Soft-deleting mileage rows therefore leaks nowhere.
- fleet-service DB tests run on sqlite with a `fleet` schema; Postgres-only
  syntax must be dialect-guarded (precedent: `AttachDocument`'s `FOR UPDATE`).

## 3. Decisions

### D1. Helper home: `mileage.Administrator` gains `AppendTx` and `RetractTx`

```go
// internal/mileage/administrator.go
type Administrator interface {
    // AppendTx inserts one mileage row on tx and advances the vehicle's
    // current_mileage to m.Mileage() only if current_mileage <= m.Mileage()
    // and the vehicle is live. Returns the created model with flagged set.
    AppendTx(tx *gorm.DB, m Model) (Model, error)
    // RetractTx soft-deletes every live row for (vehicleID, source, sourceRefID)
    // and, if any were deleted, recomputes current_mileage from remaining live
    // rows. The only code path permitted to lower current_mileage.
    RetractTx(tx *gorm.DB, vehicleID, source, sourceRefID string) error
}
```

`Insert` is removed (its sole caller, the manual POST, moves to `AppendTx`).

**Alternatives considered:**

- *Package-level functions* (`mileage.AppendTx(tx, …)`). Simpler call sites, but
  consumers could no longer be handed a fake, and it breaks the
  provider-reads / administrator-writes template the backend guidelines enforce.
- *A new `ledger` package.* No benefit over the existing mileage package, which
  already owns the entity and table.
- *Methods on `vehicle.Administrator`.* Puts mileage-history writes in the vehicle
  domain and inverts the dependency direction.

Chosen because it follows the `InsertTx(tx, m)` precedent exactly: the receiver
holds no state the method uses, the caller owns the transaction, and consumers
depend on a narrow local interface.

**Source constants.** Add `mileage.SourceFuel`, `SourceMaintenance`,
`SourceManual` string constants plus `ValidSource(string) bool`. Every caller
(including completion's `"maintenance"` literal) uses them; the `?source=` filter
validates against `ValidSource`.

**Input shape.** `AppendTx` takes a `mileage.Model` built with the existing
`mileage.NewBuilder()` (vehicleID, mileage, recordedAt, source, sourceRefID,
createdByUserID are all already settable). No new input struct.

### D2. `AppendTx` semantics

```
1. tx.Create(&entity)
2. res := tx.Table("fleet.vehicles").
       Where("id = ? AND deleted_at IS NULL AND current_mileage <= ?", vid, reading).
       Update("current_mileage", reading)
3. flagged := res.RowsAffected == 0
```

`flagged` comes from the conditional update's affected-row count rather than a
separate pre-read. The update matches iff `current_mileage <= reading`, so
`RowsAffected == 0` ⇔ `reading < current_mileage` at the instant of the update —
exactly FR-1.1's definition, with no read-then-write window. Both Postgres and
sqlite report matched rows (not changed rows), so an equal reading counts as 1
and is not flagged.

Edge: a soft-deleted vehicle also yields 0 rows → `flagged = true`. Every caller
has already resolved a live vehicle before writing, so this is unreachable in
practice and harmless if hit (flagged is a response-only hint).

This also fixes a latent bug in the manual POST: today it flags against a
pre-read `v.CurrentMileage()` and then writes unconditionally, so two concurrent
manual readings can regress the odometer. After the change no path writes
`current_mileage` unconditionally except vehicle PATCH.

### D3. `RetractTx` semantics and the concurrency lock

```
1. res := UPDATE fleet.mileage_records SET deleted_at = now
          WHERE vehicle_id = ? AND source = ? AND source_ref_id = ? AND deleted_at IS NULL
2. if res.RowsAffected == 0 → return nil         (nothing retracted; odometer untouched)
3. SELECT id FROM fleet.vehicles WHERE id = ? FOR UPDATE   (Postgres only; dialect-guarded)
4. UPDATE fleet.vehicles
     SET current_mileage = (SELECT MAX(mileage) FROM fleet.mileage_records
                            WHERE vehicle_id = ? AND deleted_at IS NULL)
   WHERE id = ? AND deleted_at IS NULL
     AND EXISTS (SELECT 1 FROM fleet.mileage_records
                 WHERE vehicle_id = ? AND deleted_at IS NULL)
```

The `EXISTS` guard implements "no live rows remain → unchanged" in the same
statement. Step 4 is the single-statement recompute the PRD's NFR requires.

**Why step 3 (an addition to the PRD's single-UPDATE wording).** Under Postgres
READ COMMITTED, if a concurrent append (tx B) holds the vehicle row lock when
tx A's step-4 UPDATE starts, A blocks; when B commits, Postgres re-checks A's
`WHERE` against the new row version but the `SET` subquery runs against A's
original statement snapshot, which does not include B's mileage row. A would
then write a max that excludes B's reading — silently lowering a valid advance.
Taking the row lock first in a separate statement means step 4 starts *after*
any competing writer has committed and so takes a fresh snapshot that includes
its row. In the opposite order (A locks first), B's conditional advance blocks,
then re-evaluates `current_mileage <= reading` against A's recomputed value and
advances correctly. Same lock shape and dialect guard as
`maintenancerecord.AttachDocument`; sqlite serializes writers and needs no lock.

The lock is taken only after step 1 proved something was retracted, so deletes
of records without mileage never touch the vehicle row.

### D4. How consumers reach the helper: narrow local interfaces, wired in `main.go`

Each consuming package declares the slice of the mileage administrator it needs,
mirroring `maintenanceschedule.RecordInserter`:

```go
// maintenancerecord, fuel
type MileageLedger interface {
    AppendTx(tx *gorm.DB, m mileage.Model) (mileage.Model, error)
    RetractTx(tx *gorm.DB, vehicleID, source, sourceRefID string) error
}
// maintenanceschedule
type MileageAppender interface {
    AppendTx(tx *gorm.DB, m mileage.Model) (mileage.Model, error)
}
```

`cmd/main.go` builds one `mileageAdmin := mileage.NewAdministrator(db)` and
passes it to fuel, maintenancerecord, completion, and mileage route wiring.
Constructors take it as a required argument (not an optional `With…` setter):
a nil ledger would silently reintroduce the bug this task fixes.

### D5. Maintenance records: processor decides, administrator executes atomically

The "should this write produce a mileage entry" rules are business logic and
live in `maintenancerecord/processor.go` as pure functions; the administrator
owns the transaction and calls the ledger.

```go
// processor.go — pure, unit-testable without a DB
func mileageEntryForCreate(m Model) *mileage.Model
    // nil when m.Mileage() <= 0; otherwise source=maintenance,
    // ref=m.ID(), recordedAt=m.PerformedAt(), createdBy=m.CreatedByUserID()
func mileageEntryForUpdate(before, after Model, actorUserID string) *mileage.Model
    // nil unless after.Mileage() > 0 && after.Mileage() != before.Mileage();
    // recordedAt = after.PerformedAt(), createdBy = actorUserID
```

Administrator changes (`maintenancerecord/administrator.go`):

| Method | Change |
|---|---|
| `NewAdministrator(db, ledger MileageLedger)` | ledger required |
| `Insert(m Model, entry *mileage.Model)` | one tx: `InsertTx` → if `entry != nil` `ledger.AppendTx` |
| `Update(m Model, entry *mileage.Model)` | one tx: the existing allow-list `Updates` + re-read, then `AppendTx` if `entry != nil`. The re-read moves onto `tx`. |
| `SoftDelete(id, vehicleID string)` | one tx: stamp `deleted_at` (RowsAffected 0 → `ErrNotFound`) → `ledger.RetractTx(tx, vehicleID, SourceMaintenance, id)` |
| `InsertTx(tx, m)` | **unchanged — does not append.** Completion calls it and then appends through its own `AppendMileage`; appending inside `InsertTx` would double-write the completion entry. |

Processor changes:

- `Create(m)` → `a.Insert(m, mileageEntryForCreate(m))`.
- `Update(id, actorUserID, apply)` → already loads `before`; computes `after`,
  validates, then `a.Update(after, mileageEntryForUpdate(before, after, actor))`.
  The resource passes `identity.UserID`. This is a signature change on the
  processor; the closure-based `apply` stays.
- `SoftDelete(id)` → `GetByID` (already the resource's pattern) for the vehicle
  ID, then `a.SoftDelete(id, m.VehicleID())`. Error mapping unchanged.

`createdByUserID` on a PATCH-derived entry is the editor, not the record's
original author: the entry records who supplied that reading.

**Concurrent PATCH race (accepted).** `before` is read outside the transaction.
Two simultaneous PATCHes setting the same new mileage could each append an
entry. Duplicates are harmless (same reading; FR-3.2 already allows several
entries per record) and retraction deletes them together. Not worth a
`FOR UPDATE` on the record.

**Alternative rejected:** letting `Administrator.Insert` decide `mileage > 0`
itself. Simpler call site, but it moves a PRD rule into the persistence layer
and leaves FR-3's "changed and positive" comparison — which needs `before` —
with no natural home.

### D6. Fuel

- `NewLoggingDeps(db, ledger)`; `LogInTransaction` steps 2–3 become one
  `ledger.AppendTx(tx, mileage.NewBuilder()…SetRecordedAt(created.Date())
  .SetSource(mileage.SourceFuel).SetSourceRefID(created.ID())…Build())`.
  The `uuid`/raw-SQL block is deleted. Activity and outbox steps unchanged.
- `fuel.NewAdministrator(db, ledger)`; `SoftDelete(id, vehicleID)` runs in a
  transaction: stamp → `RetractTx(tx, vehicleID, SourceFuel, id)`. The resource
  already holds `current` and passes `current.VehicleID()`.
- `fuel.InitializeRoutes` gains the ledger parameter.
- Fuel PATCH unchanged (PRD non-goal).

### D7. Schedule completion

- `Completion.AppendMileage(vehicleID string, miles int, recordedAt time.Time,
  src, ref, actorUserID string) error` — adds the date (FR-1.4) and the actor
  (FR-1.1 takes `createdByUserID`; completion currently leaves it empty).
- `CompletionProcessor.Complete` passes `in.Date` and `in.ActorUserID`, and
  `mileage.SourceMaintenance` instead of the `"maintenance"` literal.
- `NewCompletionDeps(db, records, schedule, mileageAppender)`; `dbCompletion`
  carries it and `AppendMileage` becomes a build-and-`AppendTx` one-liner.
- Existing behavior when `LatestMileage` is 0 is preserved as-is (out of scope;
  completion validation owns that).

### D8. Manual mileage POST

The handler opens `db.Transaction` and calls `adm.AppendTx`. The response is
`Transform(created)` where `created.Flagged()` came from D2 — same 201 status,
same body shape, including `flagged` (FR-1.5).

Deleted as dead code: `mileage.Processor`, `OnAppend`, `VehicleMileageUpdater`,
`processor_test.go` (its two cases are re-expressed against `AppendTx`), and
`vehicle.Administrator.UpdateCurrentMileage` plus its interface entry.
`mileage.InitializeRoutes` drops the `updater` parameter.

FR-1.3's acceptance check becomes a grep: outside `internal/mileage/administrator.go`
there is no `INSERT INTO fleet.mileage_records`, no `mileage.Entity` create, and
no `"current_mileage"` write except the vehicle domain's own PATCH path.

### D9. `?source=` filter

- Provider: `ListByVehicle(vehicleID string, f ListFilter, page server.Page)`
  with `type ListFilter struct { From, To *time.Time; Sources []string }`.
  Empty `Sources` = all; otherwise `source IN ?`. The `Count` and the page query
  share the filtered builder, so `meta` totals reflect the filter.
  A struct rather than a fourth positional pointer keeps the signature readable
  as filters accumulate.
- Resource: parse `source` as comma-separated, trim, drop empties, dedupe; any
  value failing `ValidSource` → `server.ErrValidation`. A present-but-empty
  value (`?source=`) is treated as absent.
- Index: rows are already narrowed by the `vehicle_id` index; the source
  predicate is a residual filter on a per-vehicle set. No new index.

### D10. Backfill

`maintenancerecord.BackfillMileage(db *gorm.DB, log logrus.FieldLogger) error`,
in a new `internal/maintenancerecord/backfill.go`, wired in `cmd/main.go`
immediately after the schedule backfill:

```go
if _, err := database.WithLeaderLock(db, "maintenance-mileage-backfill", func() error {
    return maintenancerecord.BackfillMileage(db, log)
}); err != nil {
    log.WithError(err).Fatal("backfill maintenance mileage")
}
```

Inside one transaction:

1. **One set-based selection:**
   ```sql
   SELECT r.id, r.vehicle_id, r.mileage, r.performed_at, r.created_by_user_id
   FROM fleet.maintenance_records r
   JOIN fleet.vehicles v ON v.id = r.vehicle_id AND v.deleted_at IS NULL
   WHERE r.deleted_at IS NULL AND r.mileage > 0
     AND NOT EXISTS (SELECT 1 FROM fleet.mileage_records m
                     WHERE m.source = 'maintenance' AND m.source_ref_id = r.id)
   ```
   The `NOT EXISTS` deliberately ignores `deleted_at` (FR-6.2): a record whose
   entry was retracted is never re-inserted.
2. Build `mileage.Entity` values in Go (UUIDs via `uuid.NewString()` — avoids
   Postgres-only `gen_random_uuid()` so the sqlite tests run) and
   `tx.CreateInBatches(rows, 500)`.
3. Compute max inserted reading per vehicle in Go; for each vehicle issue the
   same conditional never-regress `UPDATE` (`current_mileage < ?`); sum
   `RowsAffected` as "vehicles advanced".
4. `log.WithFields(rows_inserted, vehicles_advanced).Info(...)`.

Step 3 loops per *vehicle*, not per record; the PRD's "no per-record query loop"
is satisfied, and the vehicle count is small. It intentionally does **not** go
through `AppendTx`: that would be one insert + update per record.

Idempotency: every selected record gains a `(maintenance, record.id)` row, so
the `NOT EXISTS` selects nothing on a second run.

**Records created by schedule completion** already have an entry and are skipped.
**Historical deletions** (PRD open question) are untouched: soft-deleted records
are excluded by `r.deleted_at IS NULL`, and nothing retracts their old rows.

### D11. Frontend

**Invalidation (FR-8).** One helper, so the eight mutations cannot drift:

```ts
export function invalidateOdometerQueries(qc: QueryClient, vehicleId?: string) {
  void qc.invalidateQueries({ queryKey: vehicleKeys.lists() });
  void qc.invalidateQueries({
    queryKey: vehicleId ? vehicleKeys.detail(vehicleId) : vehicleKeys.all,
  });
  void qc.invalidateQueries({ queryKey: mileageKeys.lists() });
  void qc.invalidateQueries({ queryKey: dashboardKeys.all });
}
```

Home: `lib/hooks/api/mileage.ts`, provided `vehicles.ts` and `dashboard.ts` do
not import `mileage.ts` (builder confirms; otherwise a small
`lib/hooks/api/odometer.ts`). Called from `onSuccess`/`onSettled` of:
`useCreateMaintenanceRecord`, `useUpdateMaintenanceRecord`,
`useDeleteMaintenanceRecord`, `useCompleteMaintenanceSchedule` (have vehicleId);
`useCreateFuelLog`, `useCreateMileageRecord` (have vehicleId);
`useUpdateFuelLog`, `useDeleteFuelLog` (no vehicleId → `vehicleKeys.all`).
Existing domain-specific invalidations stay; redundant `vehicleKeys.detail`
calls those hooks already make are replaced by the helper.

Invalidating on fuel/record *update* even when mileage didn't change is
deliberate: the hook can't cheaply know, and an extra refetch is cheap.

**Source filter.**
- `MileageSource = 'fuel' | 'maintenance' | 'manual'` in `types/models/mileage`.
- `UseMileageParams.sources?: MileageSource[]`; included in
  `mileageKeys.list(...)` so filtered and unfiltered caches never collide; the
  service serializes it as `source=a,b` only when non-empty.
- `vehicleRecords.ts` calls `useMileageRecords({ vehicleId, sources: ['manual'] })`.
  Its summed `total` then counts manual rows only — correct, since the other
  readings already appear as their fuel/maintenance parents.
- `VehicleTrends.tsx`, `CompleteScheduleDialog.tsx`, `VehicleRecordDrawer.tsx`
  are untouched and keep all sources.

**`deriveOdometer` unchanged.** The detail page's derivation keeps working; the
fuel/maintenance parent rows still carry their mileage, so removing the derived
"Odometer reading" rows doesn't change its result. It can still differ from
`current_mileage` when the newest-dated reading is lower than an older one
(a flagged reading); that is pre-existing, out of PRD scope, and noted here only
so nobody mistakes it for a regression.

## 4. Testing strategy

Backend (sqlite, existing per-package harness):

- `mileage`: `AppendTx` — advance on higher, equal-not-flagged, lower-flagged-kept,
  deleted vehicle untouched. `RetractTx` — recompute to remaining max, no-op when
  nothing matched, unchanged when no live rows remain, only matching
  `(vehicle, source, ref)` rows deleted. Provider `Sources` filter + totals;
  resource `?source=` parsing including the unknown-value 422.
- `maintenancerecord`: pure-function tables for `mileageEntryForCreate/Update`;
  administrator tx tests for create (0 vs >0), update (changed / same / omitted /
  zero / lower than odometer), delete (retract + recompute; completion-created
  record). **Rollback test:** a ledger fake whose `AppendTx` errors leaves no
  maintenance record row — the PRD's atomicity acceptance criterion.
- `fuel`: existing `LogInTransaction` tests pass unchanged; new delete-retract test.
- `maintenanceschedule`: existing completion tests pass; `recorded_at` equals the
  completion date.
- Backfill: inserts for unlinked records, skips linked/soft-deleted-linked,
  skips deleted records and deleted vehicles, never regresses, second run
  inserts 0.
- The D3 lock is Postgres-only; its correctness argument is documented in code
  rather than tested (sqlite cannot reproduce the interleaving), matching how
  `AttachDocument`'s lock is handled.

Frontend (Vitest hook tests in `lib/hooks/api/*.test.*`): each of the eight
mutations invalidates the four key families; `useMileageRecords` with `sources`
sends `source=manual` and keys separately; `vehicleRecords` requests manual only.

Gate: flagless `tools/verify.sh`.

## 5. Change map

| File | Change |
|---|---|
| `fleet-service/internal/mileage/administrator.go` | `AppendTx`, `RetractTx`; drop `Insert`; update append-only comment |
| `…/mileage/model.go`, `entity.go` | source constants + `ValidSource`; comment update |
| `…/mileage/provider.go` | `ListFilter` with `Sources` |
| `…/mileage/resource.go` | tx-wrapped `AppendTx`; `?source=` parsing; drop `updater` param |
| `…/mileage/processor.go`, `processor_test.go` | delete |
| `…/vehicle/administrator.go` | delete `UpdateCurrentMileage` |
| `…/maintenancerecord/{processor,administrator,resource}.go` | D5 |
| `…/maintenancerecord/backfill.go` (new) | D10 |
| `…/fuel/{administrator,resource}.go` | D6 |
| `…/maintenanceschedule/{completion,completion_db}.go` | D7 |
| `fleet-service/cmd/main.go` | shared `mileageAdmin`; constructor args; backfill wiring |
| `web/src/lib/hooks/api/{mileage,maintenance,fuel,vehicleRecords}.ts` | D11 |
| `web/src/services/api/MileageService.ts`, `types/models/mileage.ts` | `source` param, `MileageSource` |

No schema migration. Service docs for fleet-service should mention the new
side effects and the `?source=` parameter.

## 6. Calls made in this design (for the reviewer)

1. Helpers are methods on `mileage.Administrator`, consumed via narrow local
   interfaces — not package functions (D1, D4).
2. `flagged` is derived from the conditional update's `RowsAffected`, not a
   pre-read (D2).
3. `RetractTx` takes a dialect-guarded `FOR UPDATE` on the vehicle before the
   recompute — an addition to the PRD's single-statement NFR, required for it to
   hold under Postgres READ COMMITTED (D3).
4. `maintenancerecord.InsertTx` stays mileage-free so completion does not
   double-append (D5).
5. PATCH-derived entries are attributed to the editor (D5).
6. `vehicle.Administrator.UpdateCurrentMileage` and `mileage.Processor` are
   deleted; the PRD's description of `UpdateCurrentMileage` as the vehicle-PATCH
   path does not match the code (§2, D8).
7. Backfill builds rows in Go and advances per vehicle, not via `AppendTx` and
   not via `gen_random_uuid()` (D10).
8. Completion entries now carry `createdByUserID = ActorUserID` (D7).
