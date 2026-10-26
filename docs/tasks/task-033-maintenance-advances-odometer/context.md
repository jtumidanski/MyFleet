# task-033 — Context

Companion to [plan.md](plan.md). Spec: [design.md](design.md); requirements: [prd.md](prd.md).

## Key files (verified at plan time)

| File | Role today | Change |
|---|---|---|
| `apps/fleet-service/internal/mileage/administrator.go` | `Insert` only, "append-only" comment | `AppendTx`, `RetractTx`; `Insert` removed |
| `apps/fleet-service/internal/mileage/processor.go` (+ `_test`) | `OnAppend` — pre-read flag + unconditional set | deleted |
| `apps/fleet-service/internal/mileage/provider.go` | `ListByVehicle(vehicleID, from, to, page)` | `ListFilter{From, To, Sources}` |
| `apps/fleet-service/internal/mileage/resource.go` | `InitializeRoutes(log, db, vehicleAccessor, updater)` | `…, adm Administrator`; `?source=` |
| `apps/fleet-service/internal/vehicle/administrator.go:26,130-136` | `UpdateCurrentMileage` (sole caller: `OnAppend`) | deleted |
| `apps/fleet-service/internal/fuel/administrator.go:107-171` | raw `INSERT INTO fleet.mileage_records` + conditional update | `ledger.AppendTx`; `SoftDelete(id, vehicleID)` retracts |
| `apps/fleet-service/internal/maintenanceschedule/completion_db.go:127-145` | `tx.Create(&mileage.Entity)`, `recordedAt = time.Now()` | `AppendTx` with completion date + actor |
| `apps/fleet-service/internal/maintenancerecord/{administrator,processor,resource}.go` | create/update/delete write no mileage | ledger in one tx; pure entry rules in processor |
| `apps/fleet-service/cmd/main.go:79-82, 95, 136, 299-303` | schedule backfill; constructor wiring | shared `mileageAdmin`; new backfill |
| `apps/web/src/lib/hooks/api/{mileage,maintenance,fuel,vehicleRecords}.ts` | mutations never invalidate `vehicleKeys.lists()`; feed requests all sources | `invalidateOdometerQueries`; feed `sources: ['manual']` |
| `apps/web/src/services/api/MileageService.ts`, `types/models/mileage.ts` | no source param; `source: string` | `sources` param; `MileageSource` |

## Decisions carried from the design

- Helpers are methods on `mileage.Administrator`, consumed via narrow local interfaces (`fuel.MileageLedger`, `maintenancerecord.MileageLedger`, `maintenanceschedule.MileageAppender`), required constructor args (D1, D4).
- `flagged` = conditional update `RowsAffected == 0` (D2).
- `RetractTx` takes a dialect-guarded `FOR UPDATE` on the vehicle before the single-statement recompute (D3).
- `maintenancerecord.InsertTx` stays mileage-free so completion doesn't double-append (D5).
- PATCH-derived entries are attributed to the editor (D5); completion entries to `ActorUserID` (D7).
- Backfill builds rows in Go (`uuid.NewString()`, sqlite-safe) and advances per vehicle (D10).

## Facts found while planning (not in the design)

- `maintenancerecord.Validate` (`model.go:91-101`) does not check mileage, so PATCH `mileage: 0` is a 200 that writes no entry.
- `maintenancerecord` test fixture (`provider_test.go:17-63`) has no `fleet.vehicles` / `fleet.mileage_records` tables; Task 6 adds them (DDL copied from `maintenanceschedule/completion_db_test.go:31-41`, also used by `fuel/processor_test.go`).
- ~40 test call sites use `maintenancerecord.NewAdministrator(db)`; Task 6 rewrites them mechanically with `sed`.
- The `mileage` package has no DB or resource tests today; Tasks 1–3 create them.
- `vehicles.ts` and `dashboard.ts` don't import `mileage.ts`, so the invalidation helper goes in `mileage.ts` (D11's condition holds).
- No `apps/fleet-service/docs/` exists yet (DOCS.md §3: "None of the paths above exist yet"), so this task has no service-doc update.

## Dependencies / ordering

Task 1 → 2 → 3 (mileage package). Tasks 4, 5 depend on Task 1 and on `mileageAdmin` from Task 2's `main.go`. Task 6 depends on Task 5 (it edits `completion_db_test.go` call sites after Task 5's `NewCompletionDeps` change). Task 7 depends on Task 6's fixture. Tasks 8–9 (frontend) depend only on the backend `?source=` contract (Task 3) and can run after it. Task 10 last.

## Verification

- Per task: module-local `go test` / `npm run -w apps/web test` as listed in each task.
- Branch done: flagless `tools/verify.sh` exit 0 (Task 10).
