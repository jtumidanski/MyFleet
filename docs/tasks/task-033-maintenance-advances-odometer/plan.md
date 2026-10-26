# Maintenance Records Advance the Odometer — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Every mileage-bearing write (fuel log, manual reading, schedule completion, maintenance record create/edit) appends history and advances `current_mileage` through one transaction-scoped helper; maintenance-record and fuel-log deletes retract their readings; history is backfilled; the frontend refetches the vehicles list and stops showing duplicate odometer rows.

**Architecture:** `mileage.Administrator` gains `AppendTx` / `RetractTx` (caller-owned tx). Consumers (`fuel`, `maintenancerecord`, `maintenanceschedule`) declare narrow local interfaces and receive one shared `mileageAdmin` from `cmd/main.go` as a required constructor argument. Maintenance-record mileage rules are pure functions in its processor; the administrator runs record write + ledger call in one transaction. Frontend gets one `invalidateOdometerQueries` helper and a `sources` filter on the mileage list.

**Tech Stack:** Go (chi, GORM, sqlite-backed tests), React + TanStack Query + Vitest.

**Spec:** `docs/tasks/task-033-maintenance-advances-odometer/design.md` (PRD: `prd.md` in the same folder). Read both; the design's D-numbers are cited below.

## Global Constraints

- No schema migration / DDL in production code (PRD §6).
- Source values are exactly `fuel`, `maintenance`, `manual` (PRD FR-1.1); use the constants from Task 1, never string literals.
- Never-regress rule: `current_mileage` advances only when `current_mileage <= reading` (FR-1.1). `RetractTx` is the only path that may lower it (FR-1.2).
- Every parent write and its mileage side effect share one transaction (PRD §8).
- Response bodies and status codes of all touched endpoints are unchanged (FR-2.3, FR-1.5; DELETE stays `204`).
- Invalid `?source=` → `server.ErrValidation` (FR-7.1).
- Ledger/appender constructor parameters are required, never `With…` setters (D4).
- Backend tests: `cd apps/fleet-service && go test ./internal/<pkg>/...`. Frontend tests: `npm run -w apps/web test -- <file>` from the worktree root.
- Done = flagless `tools/verify.sh` exit 0 (CLAUDE.md).

## Review Focus

1. `?source=` with whitespace, empty segments, or wrong case (`manual, ,fuel`, `MANUAL`) — trimmed/empties dropped are accepted; wrong case is a 422, not silently "all sources". Test added in Task 3.
2. Deleting a record whose reading is *not* the vehicle's max — the odometer must stay at the higher remaining reading, not drop to the deleted one's neighbor. Test added in Task 1 (`RetractTx` non-max case).
3. Delete atomicity — if `RetractTx` fails, the maintenance record must still be live (no half-delete). Test added in Task 6.
4. A record edited twice then deleted — all three of its mileage rows are retracted together. Test added in Task 6.
5. Manual POST of a reading below the odometer — response still carries `flagged: true` with 201 after the move off the pre-read (FR-1.5). Test added in Task 2.

---

### Task 1: Mileage ledger — source constants, `AppendTx`, `RetractTx`

**Files:**
- Modify: `apps/fleet-service/internal/mileage/model.go` (constants, `ValidSource`, comment)
- Modify: `apps/fleet-service/internal/mileage/administrator.go`
- Create: `apps/fleet-service/internal/mileage/testdb_test.go` (sqlite fixture + helpers)
- Create: `apps/fleet-service/internal/mileage/administrator_test.go`

**Interfaces:**
- Produces:
  - `const SourceFuel = "fuel"`, `SourceMaintenance = "maintenance"`, `SourceManual = "manual"`; `func ValidSource(s string) bool`.
  - `Administrator` interface gains `AppendTx(tx *gorm.DB, m Model) (Model, error)` and `RetractTx(tx *gorm.DB, vehicleID, source, sourceRefID string) error`. `Insert` stays for now (removed in Task 2).
  - Test helpers (package `mileage`, `_test.go`): `newTestDB(t) *gorm.DB`, `seedVehicle(t, db, id string, miles int)`, `seedDeletedVehicle(t, db, id string, miles int)`, `currentMileage(t, db, id string) int`, `liveRows(t, db, vehicleID string) int64`.

- [ ] **Step 1: Write the fixture.** `newTestDB` opens `sqlite.Open(":memory:")`, runs `ATTACH DATABASE ':memory:' AS fleet`, and creates `fleet.vehicles` and `fleet.mileage_records` with the exact DDL used in `internal/maintenanceschedule/completion_db_test.go:31-41`. Seed helpers use raw `INSERT INTO fleet.vehicles (id, fleet_id, current_mileage, deleted_at) VALUES (…)`.

- [ ] **Step 2: Write failing tests** in `administrator_test.go` (each builds the model with `NewBuilder()…Build()` and calls inside `db.Transaction`):
  - `TestAppendTx_advancesOnHigherReading`: vehicle at 1000, append 1500 → `currentMileage == 1500`, returned `Flagged() == false`, 1 live row with `source=manual`.
  - `TestAppendTx_equalReadingIsNotFlagged`: vehicle 1000, append 1000 → `Flagged() == false`, mileage 1000.
  - `TestAppendTx_lowerReadingIsKeptAndFlagged`: vehicle 2000, append 1500 → `Flagged() == true`, `currentMileage == 2000`, row exists.
  - `TestAppendTx_deletedVehicleUntouched`: `seedDeletedVehicle(…, 1000)`, append 5000 → `currentMileage == 1000`.
  - `TestRetractTx_recomputesToRemainingMax`: rows 1000 (manual), 3000 (maintenance ref `r1`), vehicle 3000; retract `(v, maintenance, r1)` → `currentMileage == 1000`; the `r1` row has non-null `deleted_at`.
  - `TestRetractTx_nonMaxReadingLeavesOdometer`: rows 3000 (manual), 1000 (maintenance `r1`), vehicle 3000; retract `r1` → `currentMileage == 3000`.
  - `TestRetractTx_noMatchIsNoOp`: vehicle 4000 with one row 1000; retract `(v, maintenance, "nope")` → `currentMileage == 4000`, `liveRows == 1`.
  - `TestRetractTx_noLiveRowsRemainLeavesOdometer`: one row 2000 (`fuel`, `f1`), vehicle 2500; retract it → `currentMileage == 2500`, `liveRows == 0`.
  - `TestRetractTx_onlyMatchingTupleDeleted`: rows `(maintenance,r1)`×2, `(fuel,r1)`, `(maintenance,r2)`, plus `(maintenance,r1)` on another vehicle; retract `(v, maintenance, r1)` → exactly the two matching rows deleted, others live.
  - `TestValidSource`: true for the three constants; false for `""`, `"MANUAL"`, `"obd"`.

- [ ] **Step 3: Run** `cd apps/fleet-service && go test ./internal/mileage/...` — Expected: FAIL (undefined `AppendTx`, `RetractTx`, `ValidSource`, constants).

- [ ] **Step 4: Implement** per design D2 (`tx.Create(&e)` → conditional `tx.Table("fleet.vehicles").Where("id = ? AND deleted_at IS NULL AND current_mileage <= ?", …).Update("current_mileage", reading)` → `Make(e).WithFlagged(res.RowsAffected == 0)`) and D3 (the four-step soft-delete → early return → dialect-guarded `SELECT id FROM fleet.vehicles WHERE id = ? FOR UPDATE` skipped when `tx.Name() == "sqlite"` → single `UPDATE … SET current_mileage = (SELECT MAX(mileage) …) WHERE … AND EXISTS (…)` via `tx.Exec`). Put D3's READ COMMITTED rationale in the `RetractTx` doc comment (it stands in for an untestable interleaving, like `maintenancerecord.AttachDocument`). Replace "Append-only — no update or delete operations exist" (`administrator.go:6`) and `model.go:5` with: append-only except `RetractTx`, which soft-deletes a parent's rows when the parent is deleted.

- [ ] **Step 5: Run** `cd apps/fleet-service && go test ./internal/mileage/...` — Expected: PASS.

- [ ] **Step 6: Commit** `feat(fleet-service): mileage AppendTx/RetractTx ledger helpers`

---

### Task 2: Manual POST on `AppendTx`; delete dead mileage processor and `UpdateCurrentMileage`

**Files:**
- Modify: `apps/fleet-service/internal/mileage/resource.go` (POST handler, `InitializeRoutes` signature)
- Modify: `apps/fleet-service/internal/mileage/administrator.go` (remove `Insert`)
- Delete: `apps/fleet-service/internal/mileage/processor.go`, `processor_test.go`
- Modify: `apps/fleet-service/internal/vehicle/administrator.go` (remove `UpdateCurrentMileage` at :26 and :130-136)
- Create: `apps/fleet-service/internal/mileage/resource_test.go`
- Modify: `apps/fleet-service/cmd/main.go` (create shared `mileageAdmin`, line 299 call)

**Interfaces:**
- Consumes: Task 1 `AppendTx`, `SourceManual`, test fixture.
- Produces: `func InitializeRoutes(log logrus.FieldLogger, db *gorm.DB, vehicleAccessor VehicleAccessor, adm Administrator) func(chi.Router)`. In `main.go`, `mileageAdmin := mileage.NewAdministrator(db)` declared next to `vehicleAdmin` (≈line 95) — later tasks pass this same variable.
- Test helpers (resource_test.go): `stubVehicles` satisfying `VehicleAccessor` (vehicle with fleet `f1`, built via `vehicle.NewBuilder()` like `maintenancerecord/resource_test.go:25-32`), `newMileageRouter(t) (chi.Router, *gorm.DB)`, `serveAs(r, method, path, body string, id auth.Identity) *httptest.ResponseRecorder`.

- [ ] **Step 1: Write failing resource tests:**
  - `TestPostMileage_advancesOdometer`: vehicle `v1` at 1000 seeded in the DB and in the stub; POST `{"data":{"type":"mileageRecords","attributes":{"mileage":1500}}}` as owner of `f1` → 201, body `attributes.flagged == false`, `currentMileage == 1500`.
  - `TestPostMileage_lowerReadingFlaggedAndKept` (Review Focus 5): vehicle at 2000, POST 1500 → 201, `flagged == true`, `currentMileage == 2000`, one live row with `source == "manual"`.
- [ ] **Step 2: Run** `go test ./internal/mileage/...` — Expected: FAIL (compile: `InitializeRoutes` arity).
- [ ] **Step 3: Implement.** POST handler: build the model with `SetSource(SourceManual)`, then `db.Transaction(func(tx) { created, err = adm.AppendTx(tx, rec) })`, respond `Transform(created)` with 201. Remove the `proc`/`OnAppend` lines, the `updater` parameter, `Insert`, `processor.go`, `processor_test.go`, and `vehicle.Administrator.UpdateCurrentMileage` (interface + impl). Update `main.go:299` to `mileage.InitializeRoutes(log, db, vehicleProc, mileageAdmin)`. Fix the `InitializeRoutes` doc comment (no updater).
- [ ] **Step 4: Run** `cd apps/fleet-service && go build ./... && go test ./internal/mileage/... ./internal/vehicle/...` — Expected: PASS.
- [ ] **Step 5: Commit** `refactor(fleet-service): manual mileage POST uses AppendTx; drop OnAppend`

---

### Task 3: `?source=` filter on the mileage list

**Files:**
- Modify: `apps/fleet-service/internal/mileage/provider.go`
- Modify: `apps/fleet-service/internal/mileage/resource.go` (GET handler)
- Create: `apps/fleet-service/internal/mileage/provider_test.go`
- Modify: `apps/fleet-service/internal/mileage/resource_test.go`

**Interfaces:**
- Consumes: `ValidSource`, constants (Task 1); router helpers (Task 2).
- Produces: `type ListFilter struct { From, To *time.Time; Sources []string }`; `Provider.ListByVehicle(vehicleID string, f ListFilter, page server.Page) ([]Model, int, error)`; unexported `parseSources(raw string) ([]string, error)` in `resource.go`.

- [ ] **Step 1: Write failing tests.**
  - `provider_test.go` `TestListByVehicle_sourcesFilterAndTotal`: seed 2 manual, 1 fuel, 1 maintenance rows for `v1` (plus one soft-deleted manual) → `Sources: []string{SourceManual}` returns 2 rows, total 2; `Sources: nil` returns 4, total 4; `Sources: {fuel, maintenance}` returns 2.
  - `resource_test.go` table `TestParseSources`: `""` → nil, no error; `"manual"` → `[manual]`; `"manual, ,fuel "` → `[manual fuel]`; `"manual,manual"` → `[manual]`; `"MANUAL"` → `server.ErrValidation`; `"manual,obd"` → `server.ErrValidation` (Review Focus 1).
  - `TestGetMileage_sourceFilter`: seed rows as above, GET `/vehicles/v1/mileage?source=manual` → 200, every `attributes.source == "manual"`, `meta.total == 2`; GET `?source=bogus` → 422.
- [ ] **Step 2: Run** `go test ./internal/mileage/...` — Expected: FAIL.
- [ ] **Step 3: Implement** per D9: provider adds `source IN ?` when `len(f.Sources) > 0`, on the same builder used for `Count`. Handler builds `ListFilter` from the existing from/to parsing plus `parseSources(req.URL.Query().Get("source"))`; error → `server.WriteError(w, server.ErrValidation)`. Update the route comment to mention `?source=`.
- [ ] **Step 4: Run** `go test ./internal/mileage/...` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(fleet-service): ?source= filter on vehicle mileage list`

---

### Task 4: Fuel logging and fuel delete through the ledger

**Files:**
- Modify: `apps/fleet-service/internal/fuel/administrator.go`
- Modify: `apps/fleet-service/internal/fuel/resource.go` (`InitializeRoutes`, DELETE handler)
- Modify: `apps/fleet-service/internal/fuel/processor_test.go` (`NewLoggingDeps` call at :98)
- Create: `apps/fleet-service/internal/fuel/administrator_test.go`
- Modify: `apps/fleet-service/cmd/main.go:300`

**Interfaces:**
- Consumes: `mileage.Model`, `mileage.NewBuilder`, `mileage.SourceFuel`, `AppendTx`, `RetractTx`; `mileageAdmin` in main.
- Produces:
  - `type MileageLedger interface { AppendTx(tx *gorm.DB, m mileage.Model) (mileage.Model, error); RetractTx(tx *gorm.DB, vehicleID, source, sourceRefID string) error }` in `fuel/administrator.go`.
  - `func NewLoggingDeps(db *gorm.DB, ledger MileageLedger) LoggingDeps`
  - `func NewAdministrator(db *gorm.DB, ledger MileageLedger) Administrator`; `Administrator.SoftDelete(id, vehicleID string) error`
  - `func InitializeRoutes(log, db, vehicleAccessor VehicleAccessor, record ActivityRecorder, emit FuelLoggedEmitter, ledger MileageLedger) func(chi.Router)`

- [ ] **Step 1: Write failing tests** in `administrator_test.go` (reuse `newFuelDB` from `processor_test.go`; seed via `vehicle.NewAdministrator(db).Insert`; log two fuel entries via `NewLoggingDeps(db, mileage.NewAdministrator(db)).LogInTransaction` at 41000 and 42000 on a vehicle at 40000):
  - `TestSoftDelete_retractsFuelMileageAndRecomputes`: `NewAdministrator(db, mileage.NewAdministrator(db)).SoftDelete(second.ID(), v.ID())` → fuel row has `deleted_at`, its mileage row (`source=fuel`, `source_ref_id=second.ID()`) has `deleted_at`, `current_mileage == 41000`.
  - `TestSoftDelete_unknownIDIsNotFound`: → `errors.Is(err, server.ErrNotFound)`, and `current_mileage` unchanged.
- [ ] **Step 2: Update** `processor_test.go:98` to `NewLoggingDeps(db, mileage.NewAdministrator(db))`. **Run** `go test ./internal/fuel/...` — Expected: FAIL (compile).
- [ ] **Step 3: Implement** D6: in `LogInTransaction` replace steps 2–3 (`administrator.go:117-139`, the `uuid` + raw `INSERT` + conditional update) with one `ledger.AppendTx(tx, mileage.NewBuilder().SetVehicleID(created.VehicleID()).SetMileage(created.Mileage()).SetRecordedAt(created.Date()).SetSource(mileage.SourceFuel).SetSourceRefID(created.ID()).SetCreatedByUserID(created.CreatedByUserID()).Build())`; drop the now-unused `uuid` import; update the step-list doc comment. `SoftDelete(id, vehicleID)` runs `db.Transaction`: stamp (`RowsAffected == 0` → `server.ErrNotFound`) → `ledger.RetractTx(tx, vehicleID, mileage.SourceFuel, id)`. DELETE handler passes `current.VehicleID()`. `main.go:300` → `fuel.InitializeRoutes(log, db, vehicleProc, activity.Record, emitFuelLogged, mileageAdmin)`.
- [ ] **Step 4: Run** `cd apps/fleet-service && go build ./... && go test ./internal/fuel/...` — Expected: PASS, including the unchanged `TestLogInTransaction_FuelAndMileageAtomic`.
- [ ] **Step 5: Commit** `feat(fleet-service): fuel logging uses ledger; fuel delete retracts mileage`

---

### Task 5: Schedule completion through the ledger

**Files:**
- Modify: `apps/fleet-service/internal/maintenanceschedule/completion.go`
- Modify: `apps/fleet-service/internal/maintenanceschedule/completion_db.go`
- Modify: `apps/fleet-service/internal/maintenanceschedule/completion_test.go`, `completion_db_test.go` (4 `NewCompletionDeps` calls at :93, :196, :256, :316)
- Modify: `apps/fleet-service/cmd/main.go:136`

**Interfaces:**
- Consumes: `mileage.Model`, `NewBuilder`, `SourceMaintenance`, `AppendTx`.
- Produces:
  - `type MileageAppender interface { AppendTx(tx *gorm.DB, m mileage.Model) (mileage.Model, error) }` in `completion_db.go`.
  - `Completion.AppendMileage(vehicleID string, miles int, recordedAt time.Time, src, ref, actorUserID string) error`
  - `func NewCompletionDeps(db *gorm.DB, records RecordInserter, schedule ScheduleAdvancer, mileage MileageAppender) CompletionDeps` (field name of your choice; don't shadow the package inside methods).

- [ ] **Step 1: Write failing tests.**
  - `completion_test.go`: `captureCompletion.AppendMileage` records its args; extend `TestComplete_orchestratesRecordMileageAndAdvance` with `ActorUserID: "u9"` and assert captured `recordedAt.Equal(at)`, `src == mileage.SourceMaintenance`, `ref == "rec-1"`, `actor == "u9"`.
  - `completion_db_test.go` `TestCompleteInTransaction`: pass `ActorUserID: "u9"`; additionally assert the mileage row's `recorded_at` equals `at` and `created_by_user_id == "u9"` (scan into a struct `{RecordedAt time.Time; CreatedByUserID string}`).
  - Update all four `NewCompletionDeps` calls to pass `mileage.NewAdministrator(db)`.
- [ ] **Step 2: Run** `go test ./internal/maintenanceschedule/...` — Expected: FAIL.
- [ ] **Step 3: Implement** D7: `Complete` calls `AppendMileage(in.VehicleID, in.LatestMileage, in.Date, mileage.SourceMaintenance, recordID, in.ActorUserID)`; `dbCompletion` carries the appender and `AppendMileage` becomes build-and-`AppendTx` on `c.tx` (discard the returned model). Update the `Completion` and `AppendMileage` doc comments (no more "OnAppend rule" / `time.Now()`). `main.go:136` passes `mileageAdmin` as the fourth argument.
- [ ] **Step 4: Run** `cd apps/fleet-service && go build ./... && go test ./internal/maintenanceschedule/...` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(fleet-service): schedule completion appends via ledger with completion date and actor`

---

### Task 6: Maintenance record create / edit / delete through the ledger

**Files:**
- Modify: `apps/fleet-service/internal/maintenancerecord/processor.go`
- Modify: `apps/fleet-service/internal/maintenancerecord/administrator.go`
- Modify: `apps/fleet-service/internal/maintenancerecord/resource.go` (`InitializeRoutes`, PATCH passes actor)
- Modify: `apps/fleet-service/internal/maintenancerecord/provider_test.go` (fixture DDL)
- Modify: every `NewAdministrator(db)` call in `maintenancerecord/*_test.go` and `maintenancerecord.NewAdministrator(db)` in `maintenanceschedule/completion_db_test.go`
- Create: `apps/fleet-service/internal/maintenancerecord/mileage_test.go`
- Modify: `apps/fleet-service/cmd/main.go:136` and `:302`

**Interfaces:**
- Consumes: `mileage.Model`, `NewBuilder`, `SourceMaintenance`, `AppendTx`, `RetractTx`, `mileageAdmin`.
- Produces:
  - `type MileageLedger interface { AppendTx(…); RetractTx(…) }` (same method set as Task 4's) in `administrator.go`.
  - `func NewAdministrator(db *gorm.DB, ledger MileageLedger) Administrator`
  - `Administrator.Insert(m Model, entry *mileage.Model) (Model, error)`; `Update(m Model, entry *mileage.Model) (Model, error)`; `SoftDelete(id, vehicleID string) error`; `InsertTx(tx, m)` **unchanged and mileage-free** (D5).
  - `func mileageEntryForCreate(m Model) *mileage.Model`; `func mileageEntryForUpdate(before, after Model, actorUserID string) *mileage.Model` in `processor.go`.
  - `Processor.Update(id, actorUserID string, apply func(Model) Model) (Model, error)`
  - `func InitializeRoutes(log, db, vehicleAccessor, categoryAccessor, docs DocumentValidator, ledger MileageLedger) func(chi.Router)`

- [ ] **Step 1: Mechanical fixture update.** Add `fleet.vehicles` and `fleet.mileage_records` DDL (copy from `completion_db_test.go:31-41`) to `newTestDBWithoutIndexes` in `provider_test.go`. Rewrite test call sites with `sed`: `NewAdministrator(db)` → `NewAdministrator(db, mileage.NewAdministrator(db))` in `internal/maintenancerecord/*_test.go`, and `maintenancerecord.NewAdministrator(db)` → `maintenancerecord.NewAdministrator(db, mileage.NewAdministrator(db))` in `completion_db_test.go`; add the `mileage` import where needed. `insertRecord` and `admin.Update(...)` calls in existing tests gain a `nil` entry argument; `a.SoftDelete(id)` calls gain `"v1"`. `resource_test.go:48` passes `mileage.NewAdministrator(db)` last.

- [ ] **Step 2: Write failing tests** in `mileage_test.go`. Pure tables:
  - `TestMileageEntryForCreate`: mileage 0 → nil; mileage 52310 → entry with `Source()==SourceMaintenance`, `SourceRefID()==m.ID()`, `RecordedAt()==m.PerformedAt()`, `CreatedByUserID()==m.CreatedByUserID()`, `VehicleID()==m.VehicleID()`, `Mileage()==52310`.
  - `TestMileageEntryForUpdate`: (before 100, after 100) → nil; (100 → 0) → nil; (100 → 200) → entry with `Mileage()==200`, `RecordedAt()==after.PerformedAt()`, `CreatedByUserID()=="editor"`.
  DB tests (seed vehicle `v1` with raw insert at 50000; record mileage via the router or administrator as noted):
  - `TestCreate_withMileageAppendsAndAdvances`: POST record with `"mileage":52310` → 201; one mileage row `(maintenance, record id)` with `recorded_at == performedAt`; `current_mileage == 52310`.
  - `TestCreate_zeroMileageWritesNoEntry`: POST without mileage → 0 mileage rows, `current_mileage == 50000`.
  - `TestCreate_belowOdometerStoredNotApplied`: POST `"mileage":40000` → row exists, `current_mileage == 50000`.
  - `TestCreate_ledgerFailureRollsBackRecord`: `NewAdministrator(db, failingLedger{})` (fake whose `AppendTx` returns `errors.New("boom")`) → `Insert(m, entry)` errors and `fleet.maintenance_records` count for that id is 0.
  - `TestPatch_changedMileageAppends`: record created at 52310; PATCH `{"mileage":53000}` → 2 live rows for the record, the first untouched (same id, mileage 52310), `current_mileage == 53000`, new row `created_by_user_id == "u1"`.
  - `TestPatch_noMileageChangeAppendsNothing`: PATCH `{"vendor":"x"}`, then `{"mileage":52310}` (same) → still 1 row.
  - `TestPatch_zeroMileageAppendsNothing`: PATCH `{"mileage":0}` → 200 (`Validate`, `model.go:91-101`, does not check mileage) and still 1 mileage row; `current_mileage` unchanged.
  - `TestDelete_retractsAllEntriesAndRecomputes` (Review Focus 4): a manual mileage row at 51000 seeded raw; record at 52310 PATCHed to 53000 then 54000 → DELETE `/maintenance-records/{id}` → 204, all 3 record rows soft-deleted, `current_mileage == 51000`.
  - `TestDelete_noRemainingRowsLeavesOdometer`: only the record's row exists → after DELETE, `current_mileage` unchanged (52310 → stays 52310).
  - `TestDelete_completionCreatedRecordRetracts`: record inserted via `InsertTx` plus a raw mileage row `(maintenance, id)` → DELETE retracts it.
  - `TestDelete_retractFailureKeepsRecord` (Review Focus 3): fake ledger whose `RetractTx` errors → `SoftDelete(id, "v1")` errors and record `deleted_at IS NULL`.
- [ ] **Step 3: Run** `cd apps/fleet-service && go test ./internal/maintenancerecord/...` — Expected: FAIL.
- [ ] **Step 4: Implement** D5. Administrator: `Insert` = one tx, `InsertTx` then `ledger.AppendTx` when `entry != nil`; `Update` = one tx, existing allow-list `Updates` + re-read **on `tx`** (keep the existing explanatory comments), then `AppendTx` when `entry != nil`; `SoftDelete(id, vehicleID)` = one tx, stamp (`RowsAffected == 0` → `ErrNotFound`) then `RetractTx(tx, vehicleID, mileage.SourceMaintenance, id)`. Add a comment on `InsertTx` that it deliberately does not append (completion appends itself). Processor: `Create` → `a.Insert(m, mileageEntryForCreate(m))`; `Update(id, actor, apply)` → `a.Update(after, mileageEntryForUpdate(before, after, actor))`; `SoftDelete(id)` → `GetByID` for the vehicle ID then `a.SoftDelete(id, m.VehicleID())`, same error translation. Resource: PATCH calls `proc.Update(id, identity.UserID, …)`; `InitializeRoutes` passes `ledger` into `NewAdministrator`. `main.go`: `:136` → `maintenancerecord.NewAdministrator(db, mileageAdmin)`; `:302` appends `mileageAdmin`.
- [ ] **Step 5: Run** `cd apps/fleet-service && go build ./... && go test ./internal/maintenancerecord/... ./internal/maintenanceschedule/...` — Expected: PASS.
- [ ] **Step 6: Commit** `feat(fleet-service): maintenance records append/retract mileage atomically`

---

### Task 7: Maintenance mileage backfill

**Files:**
- Create: `apps/fleet-service/internal/maintenancerecord/backfill.go`
- Create: `apps/fleet-service/internal/maintenancerecord/backfill_test.go`
- Modify: `apps/fleet-service/cmd/main.go` (after the `maintenance-schedule-backfill` block, ≈line 82)

**Interfaces:**
- Consumes: `mileage.Entity`, `mileage.SourceMaintenance`; fixture from Task 6.
- Produces: `func BackfillMileage(db *gorm.DB, log logrus.FieldLogger) error`.

- [ ] **Step 1: Write failing tests** (raw-insert vehicles and records; `log` with `io.Discard` output):
  - `TestBackfillMileage_insertsUnlinkedAndAdvances`: vehicle A at 10000 with live records 12000 and 15000 (no rows); vehicle B at 90000 with a record at 80000 → A gets 2 rows (`source=maintenance`, `source_ref_id=record id`, `recorded_at=performed_at`, `created_by_user_id=record's`), `current_mileage(A)==15000`; B gets 1 row, `current_mileage(B)==90000`.
  - `TestBackfillMileage_skipsLinkedDeletedAndZero`: record with an existing live `(maintenance,id)` row; record whose only row is soft-deleted; soft-deleted record; record with mileage 0; record on a soft-deleted vehicle → 0 rows inserted.
  - `TestBackfillMileage_isIdempotent`: run twice → total row count after the second run equals the count after the first.
- [ ] **Step 2: Run** `go test ./internal/maintenancerecord/ -run Backfill` — Expected: FAIL.
- [ ] **Step 3: Implement** D10 inside one `db.Transaction`: the single set-based select from the design (its `NOT EXISTS` ignores `m.deleted_at` on purpose — say so in a comment), build `mileage.Entity` rows in Go with `uuid.NewString()` and `CreatedAt: time.Now().UTC()`, `tx.CreateInBatches(rows, 500)` (skip when empty), per-vehicle max → `UPDATE fleet.vehicles SET current_mileage = ? WHERE id = ? AND deleted_at IS NULL AND current_mileage < ?`, sum `RowsAffected`, then `log.WithFields(logrus.Fields{"rows_inserted": n, "vehicles_advanced": k}).Info("backfilled maintenance mileage")`. Wire in `main.go` exactly as D10's snippet (`"maintenance-mileage-backfill"`, `.Fatal("backfill maintenance mileage")`), with a short comment matching the neighboring schedule-backfill comment's style.
- [ ] **Step 4: Run** `cd apps/fleet-service && go build ./... && go test ./internal/maintenancerecord/...` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(fleet-service): backfill mileage history from maintenance records`

---

### Task 8: Frontend odometer invalidation helper on all eight mutations

**Files:**
- Modify: `apps/web/src/lib/hooks/api/mileage.ts`
- Modify: `apps/web/src/lib/hooks/api/maintenance.ts` (`useCreateMaintenanceRecord` :167, `useUpdateMaintenanceRecord` :179, `useDeleteMaintenanceRecord` :261, `useCompleteMaintenanceSchedule` :394)
- Modify: `apps/web/src/lib/hooks/api/fuel.ts` (`useCreateFuelLog` :83, `useUpdateFuelLog` :94, `useDeleteFuelLog` :107)
- Create: `apps/web/src/lib/hooks/api/odometerInvalidation.test.tsx`

**Interfaces:**
- Consumes: `vehicleKeys` (`./vehicles`), `dashboardKeys` (`./dashboard`), `mileageKeys`. Verified at plan time: neither `vehicles.ts` nor `dashboard.ts` imports `mileage.ts`, so the helper lives in `mileage.ts`.
- Produces: `export function invalidateOdometerQueries(qc: QueryClient, vehicleId?: string): void` — body is D11's snippet.

- [ ] **Step 1: Write failing tests** in `odometerInvalidation.test.tsx`, using the `makeWrapper` / `invalidatedKeys` pattern from `maintenance.test.tsx:25-39`, mocking `MaintenanceRecordService`, `MaintenanceScheduleService`, `FuelService`, `MileageService` methods to resolve. One `it` per hook (`useCreateMaintenanceRecord('v1')`, `useUpdateMaintenanceRecord('v1')`, `useDeleteMaintenanceRecord('v1')`, `useCompleteMaintenanceSchedule('v1')`, `useCreateFuelLog('v1')`, `useCreateMileageRecord('v1')`): after `mutateAsync` settles, `invalidatedKeys` contains `JSON.stringify(vehicleKeys.lists())`, `vehicleKeys.detail('v1')`, `mileageKeys.lists()`, `dashboardKeys.all`. For `useUpdateFuelLog()` / `useDeleteFuelLog()`: contains `vehicleKeys.lists()`, `vehicleKeys.all`, `mileageKeys.lists()`, `dashboardKeys.all`. Plus a direct test of `invalidateOdometerQueries(qc)` without a vehicleId → `vehicleKeys.all`.
- [ ] **Step 2: Run** `npm run -w apps/web test -- src/lib/hooks/api/odometerInvalidation.test.tsx` — Expected: FAIL.
- [ ] **Step 3: Implement.** Add the helper; call it from each listed hook's existing `onSettled`; delete the now-redundant `vehicleKeys.detail(vehicleId)` lines in those four maintenance hooks (keep domain-specific invalidations). Update `useCreateMileageRecord`'s doc comment.
- [ ] **Step 4: Run** `npm run -w apps/web test -- src/lib/hooks/api/` and `npm run -w apps/web lint` — Expected: PASS, 0 warnings.
- [ ] **Step 5: Commit** `fix(web): refetch vehicle list, detail, mileage, dashboard after odometer-changing mutations`

---

### Task 9: Frontend source filter; records feed shows manual readings only

**Files:**
- Modify: `apps/web/src/types/models/mileage.ts`
- Modify: `apps/web/src/services/api/MileageService.ts`
- Modify: `apps/web/src/lib/hooks/api/mileage.ts` (`mileageKeys.list`, `UseMileageParams`, `useMileageRecords`)
- Modify: `apps/web/src/lib/hooks/api/vehicleRecords.ts:53`
- Modify: `apps/web/src/lib/hooks/api/mileage.test.ts`, `vehicleRecords.test.ts`

**Interfaces:**
- Produces: `export type MileageSource = 'fuel' | 'maintenance' | 'manual'`; `MileageRecordAttributes.source: MileageSource` (replace the stale `'manual' | 'obd'` comment); `listByVehicle(vehicleId, params?: { from?; to?; sources?: MileageSource[]; page?; pageSize? })`; `mileageKeys.list(params: { vehicleId: string; from?: string; to?: string; sources?: MileageSource[] })`; `UseMileageParams.sources?: MileageSource[]`.

- [ ] **Step 1: Write failing tests.**
  - `mileage.test.ts`: `useMileageRecords({ vehicleId: 'v1', sources: ['manual'] })` → `mileageService.listByVehicle` called with `expect.objectContaining({ sources: ['manual'] })`; `mileageKeys.list({ vehicleId: 'v1', sources: ['manual'] })` differs from `mileageKeys.list({ vehicleId: 'v1' })`.
  - Service serialization (in `mileage.test.ts` or a new `services/api/MileageService.test.ts` mocking `apiClient.request`): `sources: ['manual','fuel']` → request URL contains `source=manual%2Cfuel` (or `source=manual,fuel` — assert via `new URL(...).searchParams.get('source') === 'manual,fuel'`); `sources: []` and absent → no `source` param.
  - `vehicleRecords.test.ts`: `mockedUseMileageRecords` called with `{ vehicleId: 'v1', sources: ['manual'] }`.
- [ ] **Step 2: Run** `npm run -w apps/web test -- src/lib/hooks/api/mileage.test.ts src/lib/hooks/api/vehicleRecords.test.ts` — Expected: FAIL.
- [ ] **Step 3: Implement** per D11 "Source filter". `VehicleTrends.tsx`, `CompleteScheduleDialog.tsx`, `VehicleRecordDrawer.tsx` are not touched (FR-7.3). Update the `MileageService` route doc comment to list `?source=`.
- [ ] **Step 4: Run** `npm run -w apps/web test` and `npm run -w apps/web lint` — Expected: PASS.
- [ ] **Step 5: Commit** `fix(web): vehicle record feed lists manual odometer readings only`

---

### Task 10: Consolidation check and full verification

**Files:** none expected; fix anything the checks surface in the owning file.

- [ ] **Step 1: FR-1.3 grep.** Run from the worktree root:
  `grep -rn 'INSERT INTO fleet.mileage_records\|"current_mileage"\|mileage.Entity{\|UpdateCurrentMileage' apps/fleet-service --include='*.go' | grep -v _test.go`
  Expected: matches only in `internal/mileage/administrator.go`, `internal/maintenancerecord/backfill.go` (D10's deliberate set-based path), and the vehicle domain's own create/PATCH code; no `UpdateCurrentMileage`. Quote the output in the task report.
- [ ] **Step 2: Literal check.** `grep -rn '"maintenance"\|"fuel"\|"manual"' apps/fleet-service/internal/{mileage,fuel,maintenancerecord,maintenanceschedule} --include='*.go' | grep -v _test.go` — Expected: only the constant definitions in `mileage/model.go` (plus unrelated non-source uses, e.g. event-type strings, which must be named in the report).
- [ ] **Step 3: Gate.** Run flagless `tools/verify.sh` — Expected: exit 0. Quote the final summary lines.
- [ ] **Step 4: Commit** any fixes: `chore(task-033): consolidation cleanups`
