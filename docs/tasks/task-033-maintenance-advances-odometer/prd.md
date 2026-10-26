# Maintenance Records Advance the Odometer — Product Requirements Document

Version: v1
Status: Draft
Created: 2026-10-26
---

## 1. Overview

The vehicles list page and the vehicle detail page disagree about a vehicle's
odometer after a maintenance record is logged. The list card renders the stored
`vehicles.current_mileage`
(`apps/web/src/components/features/vehicles/VehicleCard.tsx:142-145`). The
detail page renders `deriveOdometer` — the newest record reading across the
unified feed, falling back to `currentMileage`
(`apps/web/src/lib/vehicleStats.ts:11-22`,
`apps/web/src/pages/VehicleDetailPage.tsx:76`) — so it shows the new reading
immediately while the list stays stale.

The root cause is backend. Three of the four mileage-bearing write paths append
a `fleet.mileage_records` row and advance `current_mileage` with the
never-regress rule: fuel logs (`apps/fleet-service/internal/fuel/administrator.go:117-139`),
manual readings (`apps/fleet-service/internal/mileage/resource.go:130`,
`mileage/processor.go:30-37`), and schedule completion
(`apps/fleet-service/internal/maintenanceschedule/completion_db.go:127-145`).
Directly logging a maintenance record (`POST /vehicles/{id}/maintenance-records`,
`apps/fleet-service/internal/maintenancerecord/resource.go:108-199`) only inserts
the record (`maintenancerecord/processor.go:42-44`). The MVP PRD lists
maintenance records as a mileage source
(`docs/household-vehicle-platform-mvp-prd.md` §8), so this is a gap, not a
design choice.

A second, frontend cause compounds it: no record mutation invalidates the
vehicles **list** query. `useCreateMaintenanceRecord`
(`apps/web/src/lib/hooks/api/maintenance.ts:167-177`), `useCompleteMaintenanceSchedule`
(`maintenance.ts:394-409`), the fuel mutations (`fuel.ts`), and the manual
mileage mutation (`mileage.ts:93`) never touch `vehicleKeys.lists()`, so even a
correct backend value is not refetched by the list page until its cache goes
stale.

This task closes both gaps, makes maintenance-record edit and delete keep
mileage history consistent, extends the same delete semantics to fuel logs,
consolidates the three copies of the append/advance SQL into one helper,
repairs historical data with a startup backfill, and removes the duplicate
"Odometer reading" rows the detail feed shows for fuel- and maintenance-sourced
readings.

## 2. Goals

Primary goals:
- Logging a maintenance record with a mileage reading appends a mileage entry
  and advances `current_mileage` (never regressing), atomically with the insert.
- Editing a maintenance record's mileage appends a new entry under the same rule.
- Deleting a maintenance record or fuel log retracts its linked mileage entries
  and recomputes `current_mileage` from remaining history.
- One shared, transaction-scoped mileage helper used by every mileage-bearing
  write path.
- Existing maintenance records with mileage are backfilled into history once.
- The vehicles list, vehicle detail, and dashboard reflect a new reading without
  a manual reload.
- The detail feed shows each reading once.

Non-goals:
- Making mileage history mutable — it stays append-only for inserts; the only
  new write is soft-delete on parent retraction.
- Fuel log **edit** (`PATCH /fuel-logs/{id}`) mileage semantics — unchanged.
- Changing how vehicle create/PATCH set `current_mileage` (no baseline mileage
  entry is written).
- Persisting the `flagged` state (it remains a computed response field).
- Retroactively retracting entries for records/fuel logs deleted before this
  task ships (see Open Questions).

## 3. User Stories

- As a fleet member, I want logging an oil change at 52,310 miles to update the
  vehicle's odometer everywhere, so that the vehicles list is accurate.
- As a fleet member, I want correcting a record's mileage to update the odometer
  when the correction is higher, so that a typo fix sticks.
- As a fleet member, I want deleting a mistaken maintenance record or fuel log to
  remove its reading from history and roll the odometer back to the true latest
  reading, so that a bad entry does not permanently inflate the odometer.
- As a fleet member with records logged before this fix, I want my vehicles'
  odometers corrected automatically, so that I don't have to re-enter readings.
- As a fleet member, I want each reading to appear once in the vehicle's record
  feed, so that the history is readable.

## 4. Functional Requirements

### FR-1 Shared mileage helper (`internal/mileage`)

- **FR-1.1** A transaction-scoped append operation that, given a `*gorm.DB` tx,
  vehicle ID, mileage, recordedAt, source (`fuel | maintenance | manual`),
  sourceRefID, and createdByUserID: inserts one `fleet.mileage_records` row and
  advances `fleet.vehicles.current_mileage` to the reading **only if**
  `current_mileage <= reading` and the vehicle is not soft-deleted. It returns
  the created model with `flagged = reading < current_mileage-before-insert`.
- **FR-1.2** A transaction-scoped retract operation that, given a tx, vehicle ID,
  source, and sourceRefID: soft-deletes (`deleted_at = now`) every live
  mileage row matching `(vehicle_id, source, source_ref_id)`. If at least one row
  was soft-deleted, it sets `current_mileage` to the maximum `mileage` among the
  vehicle's remaining live mileage rows; if no live rows remain, or no rows were
  soft-deleted, `current_mileage` is left unchanged. This is the only code path
  permitted to lower `current_mileage`.
- **FR-1.3** Fuel logging (`fuel/administrator.go` step 2-3), schedule
  completion (`maintenanceschedule/completion_db.go` `AppendMileage`), the manual
  mileage POST, and the new maintenance-record paths all use FR-1.1. No
  hand-written `INSERT INTO fleet.mileage_records` or
  `Update("current_mileage", …)` remains outside the helper and
  `vehicle.Administrator.UpdateCurrentMileage` (vehicle PATCH).
- **FR-1.4** Schedule completion passes the completion date as `recordedAt`
  (today it uses `time.Now()`, `completion_db.go:131`). Fuel already passes the
  fuel date. This makes every derived entry's `recordedAt` the date the reading
  was taken.
- **FR-1.5** The manual mileage POST keeps its current response contract,
  including the `flagged` field.

### FR-2 Maintenance record create

- **FR-2.1** When `mileage > 0`, `POST /vehicles/{id}/maintenance-records`
  inserts the record (and its document rows) and calls FR-1.1 with
  `source = maintenance`, `sourceRefID = record ID`, `recordedAt = performedAt`,
  `createdByUserID = caller`, all in **one** database transaction. Failure of
  any step rolls back the whole write.
- **FR-2.2** When `mileage == 0`, no mileage entry is written and
  `current_mileage` is untouched.
- **FR-2.3** The response body and status code are unchanged.

### FR-3 Maintenance record edit

- **FR-3.1** When a `PATCH /maintenance-records/{id}` request includes
  `mileage`, the new value is `> 0`, and it differs from the stored value, the
  update and an FR-1.1 append (`recordedAt` = the record's post-update
  `performedAt`) run in one transaction.
- **FR-3.2** Prior mileage entries for the record are **not** modified; a
  record may therefore own several entries.
- **FR-3.3** An edited value below `current_mileage` is still appended (and is
  not reflected in `current_mileage`, per FR-1.1).
- **FR-3.4** A PATCH that omits `mileage`, sets it to its current value, or sets
  it to `0` writes no mileage entry.

### FR-4 Maintenance record delete

- **FR-4.1** `DELETE /maintenance-records/{id}` soft-deletes the record and
  calls FR-1.2 with `source = maintenance`, `sourceRefID = record ID` in one
  transaction. This covers records created by schedule completion, which
  already link their entry with the same `(source, ref)`.

### FR-5 Fuel log delete (parity)

- **FR-5.1** `DELETE /fuel-logs/{id}` soft-deletes the fuel log and calls FR-1.2
  with `source = fuel`, `sourceRefID = fuel log ID` in one transaction.
- **FR-5.2** Fuel log create and edit behavior is otherwise unchanged.

### FR-6 Backfill

- **FR-6.1** A startup job, wired in `apps/fleet-service/cmd/main.go` under
  `database.WithLeaderLock(db, "maintenance-mileage-backfill", …)` following the
  `maintenance-schedule-backfill` pattern (`cmd/main.go:79-80`), fatal on error.
- **FR-6.2** For every live maintenance record on a live vehicle with
  `mileage > 0` and **no** mileage row (live or soft-deleted) where
  `source = 'maintenance' AND source_ref_id = record.id`, insert one mileage row
  with `recorded_at = performed_at`, `created_by_user_id = record.created_by_user_id`.
- **FR-6.3** For each vehicle that received rows, advance `current_mileage` to
  the max inserted reading only if it is greater (never regress).
- **FR-6.4** Idempotent: a second run selects nothing.

### FR-7 Feed de-duplication

- **FR-7.1** `GET /vehicles/{id}/mileage` accepts an optional `source` query
  parameter (comma-separated, each one of `fuel | maintenance | manual`). An
  unknown value returns a validation error. Absent, all sources are returned
  (current behavior). `meta` totals reflect the filter.
- **FR-7.2** The unified vehicle record feed
  (`apps/web/src/lib/hooks/api/vehicleRecords.ts`) requests `source=manual`, so
  fuel- and maintenance-sourced readings appear only as their parent fuel or
  maintenance row.
- **FR-7.3** Other mileage consumers — the trends sparkline (`VehicleTrends.tsx`),
  `CompleteScheduleDialog.tsx`, and `VehicleRecordDrawer.tsx` — keep requesting
  all sources.

### FR-8 Frontend cache invalidation

- **FR-8.1** Every mutation that can change `current_mileage` invalidates
  `vehicleKeys.lists()`, the affected `vehicleKeys.detail(vehicleId)` (or
  `vehicleKeys.all` where the hook has no vehicle ID), `mileageKeys.lists()`,
  and `dashboardKeys.all`: maintenance record create, update, and delete;
  schedule completion; fuel log create, update, and delete; manual mileage
  create.

## 5. API Surface

No new endpoints. Modified behavior:

| Endpoint | Change |
|---|---|
| `POST /vehicles/{id}/maintenance-records` | Side effect: appends mileage entry + advances odometer when `mileage > 0` (FR-2). Response unchanged. |
| `PATCH /maintenance-records/{id}` | Side effect: appends mileage entry on a changed, positive `mileage` (FR-3). Response unchanged. |
| `DELETE /maintenance-records/{id}` | Side effect: retracts linked mileage entries + recomputes odometer (FR-4). Still `204`. |
| `DELETE /fuel-logs/{id}` | Side effect: retracts linked mileage entries + recomputes odometer (FR-5). Status unchanged. |
| `GET /vehicles/{id}/mileage` | New optional `?source=` filter (FR-7.1). Invalid value → validation error (`server.ErrValidation`). |

Error cases: any failure inside a transaction returns the existing error mapping
for that endpoint with no partial write.

## 6. Data Model

- No schema changes. `fleet.mileage_records` already has `source`,
  `source_ref_id`, and `deleted_at` (`apps/fleet-service/internal/mileage/entity.go:10-21`).
- Semantic change: mileage rows may now be soft-deleted, but only by FR-1.2
  when their parent is deleted. The `mileage.Administrator` comment
  "Append-only — no update or delete operations exist"
  (`mileage/administrator.go:6`) and the model comment (`mileage/model.go:5`)
  are updated to say so.
- Relationship: a maintenance record owns 0..n mileage rows
  (`source = 'maintenance'`, `source_ref_id = record.id`); a fuel log owns 0..1.
- Data migration: FR-6 backfill, no DDL.

## 7. Service Impact

**fleet-service**
- `internal/mileage`: new append/retract helpers; `?source=` filter on the list
  provider and route; comment updates.
- `internal/maintenancerecord`: create, update, and delete run inside a
  transaction that calls the helpers; new backfill function.
- `internal/fuel`: logging uses the helper; delete runs in a transaction with
  retract.
- `internal/maintenanceschedule`: `AppendMileage` delegates to the helper and
  receives the completion date.
- `cmd/main.go`: wire the backfill under a leader lock.

**web**
- `lib/hooks/api/maintenance.ts`, `fuel.ts`, `mileage.ts`: invalidations
  (FR-8).
- `lib/hooks/api/mileage.ts` / mileage service: optional `source` filter.
- `lib/hooks/api/vehicleRecords.ts`: request `source=manual`.

auth-service, media-service, notification-service: no changes.

## 8. Non-Functional Requirements

- **Atomicity:** every parent write and its mileage side effect share one
  transaction (FR-2.1, FR-3.1, FR-4.1, FR-5.1).
- **Concurrency:** the never-regress advance remains a single conditional
  `UPDATE … WHERE current_mileage <= ?`. The recompute in FR-1.2 is a single
  `UPDATE … SET current_mileage = (SELECT max(...))` statement so a concurrent
  append cannot be overwritten by a stale read.
- **Performance:** the backfill runs one set-based selection, not a per-record
  query loop; the `?source=` filter uses the existing `vehicle_id` index.
- **Security:** existing fleet-scope and write-permission checks run before any
  write; no new authz surface.
- **Observability:** the backfill logs the number of rows inserted and vehicles
  advanced.

## 9. Open Questions

- **Delete recompute vs. vehicle-level baseline.** Vehicle create and PATCH set
  `current_mileage` without a mileage row. FR-1.2 sets the odometer to the max
  remaining reading, which can be lower than a mileage entered on vehicle create
  or PATCH (or re-raise a value a user manually lowered with PATCH). Accepted for
  v1 as chosen; revisit if users report it.
- **Historical deletions.** Maintenance records and fuel logs deleted before
  this task still have live mileage rows. They are not retracted
  retroactively; doing so would lower existing odometers without the user taking
  an action.
- **Fuel log edit.** `PATCH /fuel-logs/{id}` does not append a mileage entry
  when its mileage changes, unlike maintenance records after FR-3. Out of scope
  here.

## 10. Acceptance Criteria

- [ ] Logging a maintenance record with `mileage > current_mileage` updates the
      vehicles list card's odometer without a manual reload.
- [ ] Logging a maintenance record with `mileage > 0` creates exactly one
      mileage row (`source=maintenance`, `source_ref_id=record id`,
      `recorded_at=performedAt`) in the same transaction; a simulated failure
      of the mileage insert leaves no maintenance record.
- [ ] A record with `mileage == 0` creates no mileage row.
- [ ] A reading below `current_mileage` is stored but does not lower it.
- [ ] PATCH with a changed positive mileage appends one new row; prior rows are
      untouched; PATCH without a mileage change appends none.
- [ ] DELETE of a maintenance record soft-deletes all its mileage rows and sets
      `current_mileage` to the max remaining live reading; with no remaining
      rows, `current_mileage` is unchanged.
- [ ] DELETE of a fuel log behaves the same for `source=fuel`.
- [ ] Fuel logging, schedule completion, and manual mileage POST go through the
      shared helper, and their existing tests still pass. Schedule completion
      records `recorded_at` = completion date.
- [ ] The backfill inserts rows for unlinked historical records, advances stale
      odometers, and a second run inserts nothing.
- [ ] `GET /vehicles/{id}/mileage?source=manual` returns only manual rows with a
      matching total; an unknown source returns a validation error.
- [ ] The vehicle detail feed shows no "Odometer reading" row for fuel- or
      maintenance-sourced readings; the trends sparkline still includes them.
- [ ] Every mutation listed in FR-8.1 invalidates the listed query keys (covered
      by hook tests).
- [ ] Flagless `tools/verify.sh` exits 0.
