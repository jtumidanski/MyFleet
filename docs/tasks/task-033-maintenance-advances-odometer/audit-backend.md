# Backend Audit - fleet-service (task-033, 8613183..9c334f4)

- **Service Path:** apps/fleet-service
- **Guidelines Source:** backend-dev-guidelines skill (all 9 resources read)
- **Date:** 2026-10-26
- **Build:** PASS (`make build`, exit clean)
- **Tests:** `make test` (go test -race, workspace): every line `ok` or `no test files`, zero FAIL. Module-local `go test ./... -count=1` in apps/fleet-service: 21 packages ok, 0 failed. `go vet ./...` clean.
- **Overall:** NEEDS-WORK

Scope: 33 changed files under apps/fleet-service. Packages: mileage, fuel, maintenancerecord (incl. backfill.go), maintenanceschedule (completion files only), admin (test only), vehicle (administrator.go dead-code removal), cmd/main.go.
SEC-*: OUT-OF-SCOPE (fleet-service does not issue/validate tokens; no changed file touches JWT, redirect or secrets. Checked: `git diff --name-only` list above).

## Blocking

1. **DOM-12/13 mileage handler owns orchestration.** `internal/mileage/resource.go:150-153`: POST handler opens `db.Transaction` and calls `adm.AppendTx` directly. The diff deleted `mileage/processor.go`, so the package no longer has a processor layer at all (no `NewProcessor`; DOM-06 has no subject). Handler -> administrator bypasses the processor. Fix: reintroduce `mileage.Processor` (`NewProcessor(log logrus.FieldLogger, p Provider, a Administrator)`) with `Append(m Model) (Model, error)` wrapping the tx, and call it from the handler. Same processor should front the GET so `resource.go:93` (`prov.ListByVehicle`, DOM-13; pre-existing call, now in a package with nothing else) goes through it.
2. **DOM-12/15 fuel DELETE bypasses processor.** `internal/fuel/resource.go:237` (line modified by this diff): handler calls `adm.SoftDelete(id, current.VehicleID())` directly; `fuel/processor.go:42` `NewProcessor(log, prov)` has no administrator. Fix: give the fuel Processor the Administrator and add `SoftDelete(id)` (GetByID for vehicleID inside the processor, as `maintenancerecord/processor.go` SoftDelete does). `resource.go:211` `adm.Update` is the same defect, pre-existing; fix together.

## Non-Blocking

- DOM-18 (pre-existing pattern, touched files): bodied routes decode into anonymous inline structs (`mileage/resource.go:105`, `fuel/resource.go:66,153`, `maintenancerecord/resource.go:109,226,341`; 17 sites in fleet-service) rather than named unexported `createAttributes`/`patchAttributes` as in `vehicle/rest.go:35,48`. Structs are narrow, so not a read-model reuse.
- Layering of backfill: `maintenancerecord/backfill.go:29-80` writes `mileage.Entity` rows and `UPDATE fleet.vehicles` directly via `tx.Raw/CreateInBatches/Exec`, outside any administrator, duplicating the "advance never regresses" rule that D8 says lives only in `mileage/administrator.go`. D10 chose this deliberately for batch performance; the guidelines place writes in administrators. Consider moving the batch write into `mileage.Administrator`.

## Domain Checklist Results

### mileage (domain)

| ID | Status | Evidence |
|----|--------|----------|
| DOM-01 | PASS | `builder.go:11-13` NewBuilder; `:25` bare `Build() Model`; mileage is a listed no-invariant domain |
| DOM-02 | PASS | `entity.go:42` `func (m Model) ToEntity()` |
| DOM-03 | PASS | `entity.go:28` `func Make(e Entity) Model` |
| DOM-04/05 | PASS | `rest.go:18` Transform, `:36` TransformSlice; list handler `resource.go:96-99` uses TransformSlice |
| DOM-06 | FAIL | No `NewProcessor` exists (processor.go deleted in this diff); see Blocking 1 |
| DOM-07 | PASS | `resource.go:48` log first param; grep `StandardLogger` in package: 0 matches |
| DOM-08 | PASS | `resource.go:105` POST body via `server.RegisterInputHandler` |
| DOM-09 | PASS | grep `http.Error\|WriteHeader` in resource.go: 0 matches; errors via `server.WriteError` (e.g. `:62,:154`) |
| DOM-10 | PASS | `provider.go:16-29` Provider/dbProvider/NewProvider; list-only, no single fetch to translate |
| DOM-11 | PASS | grep `os.Getenv` resource.go: 0 matches |
| DOM-12 | FAIL | `resource.go:150` handler runs the transaction; Blocking 1 |
| DOM-13 | FAIL | `resource.go:93` `prov.ListByVehicle` called from handler |
| DOM-14 | PASS | grep `db.Create\|db.Save\|db.Delete` resource.go: 0 matches (but see DOM-12) |
| DOM-15 | PASS | `administrator.go:17-35` AppendTx/RetractTx |
| DOM-16 | PASS | `resource.go:30` bad source -> `server.ErrValidation`; no domain-wrapping 500s introduced |
| DOM-17 | PASS | `rest.go:20` `Type: "mileageRecords"` |
| DOM-18 | WARN | inline anonymous struct `resource.go:105` (non-blocking, above) |
| DOM-19 | PASS | administrator_test.go (9 AppendTx/RetractTx cases), provider_test.go:36, resource_test.go:82,99,123,150; no processor to test. No repeated-inline-assertion blocks found |

### fuel (domain)

| ID | Status | Evidence |
|----|--------|----------|
| DOM-01 | PASS | `builder.go:11` documents `Build() (Model, error)` (invariants; unchanged by diff) |
| DOM-02/03 | PASS | `entity.go:47` ToEntity, `:30` `Make(e Entity) Model` |
| DOM-04/05 | PASS | `rest.go:19,38`; list uses proc.ListByVehicle at `resource.go:51` |
| DOM-06 | PASS | `processor.go:42` `log logrus.FieldLogger` |
| DOM-07 | PASS | `resource.go:27` log param, `:28` passed to NewProcessor; StandardLogger grep: 0 |
| DOM-08 | PASS | `resource.go:66,153` RegisterInputHandler; `:222` DELETE is body-less |
| DOM-09 | PASS | `:241` WriteHeader(204) is a success status, not an error path; no `http.Error` |
| DOM-10 | PASS | `provider.go:23` NewProvider, `:28` translates ErrRecordNotFound |
| DOM-11 | PASS | grep os.Getenv: 0 |
| DOM-12/13 | FAIL | `resource.go:237` (and `:211`) `adm.*` called from handler; Blocking 2 |
| DOM-14 | PASS | grep db.Create/Save/Delete: 0 matches |
| DOM-15 | PASS | `administrator.go:66-81` SoftDelete in a single tx with `ledger.RetractTx`; `:133-145` LogInTransaction uses ledger.AppendTx (raw SQL deleted) |
| DOM-16 | PASS | `administrator.go:77` returns `server.ErrNotFound` |
| DOM-17 | PASS | literal Type in `rest.go` Transform (unchanged) |
| DOM-18 | WARN | inline structs `resource.go:66,153` |
| DOM-19 | PASS | `administrator_test.go:65` retract+recompute, `:97` unknown id -> not found |

### maintenancerecord (domain, incl. backfill.go)

| ID | Status | Evidence |
|----|--------|----------|
| DOM-01 | PASS | `builder.go:37` `Build() (Model, error)`, validation via `Validate` called at `processor.go:91` |
| DOM-02/03 | PASS | `entity.go:128` ToEntity; `:104` `Make(e, docs)` (documented sole exception, ai-guidance DOM-03) |
| DOM-04/05 | PASS | `rest.go:26,47` |
| DOM-06 | PASS | `processor.go:20` `log logrus.FieldLogger` |
| DOM-07 | PASS | `resource.go:~49-57` log param feeds NewProcessor at `:57`; StandardLogger grep: 0 |
| DOM-08 | PASS | `resource.go:109,226,341` wrapped; `:305,:404` DELETE body-less |
| DOM-09 | PASS | WriteHeader hits `:330,:429` are 204 success; no `http.Error` |
| DOM-10 | PASS | `provider.go:32` NewProvider, `:37` ErrRecordNotFound translated |
| DOM-11 | PASS | grep os.Getenv: 0 |
| DOM-12 | PASS | handlers call only `proc` (`resource.go:272` `proc.Update(id, identity.UserID, ...)`); rules in `processor.go:48-79` |
| DOM-13/14 | PASS | grep `adm\.` and `db.(Create|Save|Delete)` in resource.go: 0 matches |
| DOM-15 | PASS | `administrator.go:60-76` Insert, `:93-111` Update, `:157-168` SoftDelete each one tx with ledger call; `ledger` required by constructor `:55` |
| DOM-16 | PASS | `processor.go:103` maps ErrNotFound to server.ErrNotFound |
| DOM-17 | PASS | rest.go unchanged by diff; Transform at `:26` |
| DOM-18 | WARN | inline structs `resource.go:109,226,341` |
| DOM-19 | PASS | `mileage_test.go:16,41` pure-function cases, `:135-313` tx/rollback/retract at the DB; `backfill_test.go:81,133,160` |

Backfill (`backfill.go`): idempotent NOT EXISTS ignoring `deleted_at` (`:32-40`), runs under `WithLeaderLock` (`cmd/main.go:~93`). Layering WARN above.

### maintenanceschedule (completion files only)

| ID | Status | Evidence |
|----|--------|----------|
| DOM-12 | PASS | Cross-domain orchestration stays in the processor/deps layer: `completion.go:68` `AppendMileage(...)`, `completion_db.go:133-146` build + `ledger.AppendTx` |
| DOM-14 | PASS | `completion_db.go` no longer holds `tx.Create(&mileage.Entity)` or the vehicles UPDATE (deleted in diff) |
| DOM-15 | PASS | writes via injected `MileageAppender` (`completion_db.go:25-27`), wired `cmd/main.go` NewCompletionDeps with `mileageAdmin` |
| DOM-19 | PASS | completion_test.go, completion_db_test.go updated for new signature (module tests pass) |
| Others | OUT-OF-SCOPE | no resource.go/rest.go/provider.go changed here (resource_test.go only) |

### admin, vehicle, cmd

- admin: only `visibility_mileage_test.go` changed (constructor update). Other checks OUT-OF-SCOPE.
- vehicle: `administrator.go` removes `UpdateCurrentMileage`. Confirmed dead: `grep -rn UpdateCurrentMileage apps/fleet-service` returns no non-doc hits (build passes). PASS.
- cmd/main.go: `mileageAdmin := mileage.NewAdministrator(db)` (`:108`) injected into mileage, fuel, maintenancerecord, completion routes (`:312-315`, `:149`). D4 narrow local interfaces `fuel/administrator.go:27`, `maintenancerecord/administrator.go:43`, `maintenanceschedule/completion_db.go:27` match the `RecordInserter` precedent; accepted.

## Other observations (no rule violated)

- `appendTx` flagging via conditional-update RowsAffected (`mileage/administrator.go:47-54`) and the dialect-guarded lock in `retractTx` (`:77`) mirror the `AttachDocument` precedent.
- `MileageLedger` is declared identically in fuel and maintenancerecord; acceptable under D4.
