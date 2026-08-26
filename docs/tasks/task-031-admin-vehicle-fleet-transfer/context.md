# Task 031 — Admin Vehicle Fleet Transfer: Implementation Context

Companion to `plan.md`. Read this first if you are picking the task up cold.

**Worktree:** `.worktrees/task-031-admin-vehicle-fleet-transfer`, branch
`task-031-admin-vehicle-fleet-transfer`. Run everything from there.

---

## 1. What this builds, in one paragraph

A platform admin picks a vehicle in the fleet inspector, chooses a destination
fleet, sees exactly what will move, types the vehicle's name, and the vehicle —
with its maintenance records, schedules, fuel logs, mileage readings, photos,
receipts, custom categories and activity timeline — lands in the other fleet.
One transaction in fleet-service plus one call each to media-service and
notification-service. Any failure leaves everything where it was.

---

## 2. The three things that make this non-trivial

Most of the vehicle's history keys on `vehicle_id` alone and follows the car for
free. Three categories of data are scoped to the **fleet** and would break
silently:

1. **Media objects** live in media-service with their own `fleet_id`, and
   `mediaobject.AuthorizeAccess` refuses on a mismatch. Without a reassign, the
   destination fleet's members get a **404** (not 403 — the refusal is
   deliberately indistinguishable from "no such object", so cross-fleet
   existence is never leaked).
2. **Custom maintenance categories** are fleet-scoped rows. A moved record
   pointing at a source-fleet category cannot be resolved to a name by the
   destination fleet.
3. **Activity events and dashboard widgets** carry the source fleet's identity.
   Activity events are the sharper of the two: `activity.Provider.ListByVehicle`
   selects on `vehicle_id` alone, so leaving `fleet_id` stale leaks one
   household's activity into another's vehicle detail view.

---

## 3. Key files

### fleet-service — `apps/fleet-service/internal/admin/`

| File | Why it matters |
|---|---|
| `manifest.go` | The purge `Manifest` this design's `TransferPlan` is modelled on. Read the `Target` doc comment. |
| `operations.go` | `Count`/`Stamp` — the shared-predicate pattern that makes preview and apply provably equal. |
| `processor.go` | `Deps`, the `AuthVerifier`/`Downstream`/`TargetResolver` ports. Two new ports go here. |
| `resource.go` | The `/admin` route tree, `authorized`, `isClientError`, `errInternal`. |
| `rest.go` | JSON:API transforms; attribute names are snake_case inside `attributes`. |
| `entity.go` / `model.go` | `AuditEntity`/`AuditEvent` and the four `purge.*` action constants. |
| `confirmation.go` | `MatchConfirmation` — reused verbatim, with `ScopeFleet` for its exact-comparison rule. |
| `targets.go` | `vehicleMediaIDs` — note it does **not** include `primary_image_media_id`; the transfer's union does. |
| `admintest/db.go` | The single hand-written SQLite fixture every DB-backed admin test uses. |
| `arch_test.go` | Manifest completeness + key uniqueness. A separate test owns the `PlatformAdmin` separation. |

### Other

- `apps/fleet-service/internal/adminclient/` — `http.go` (shared transport,
  5 s timeout), `media.go`, `notification.go`.
- `apps/media-service/internal/admin/` — `operations.go` shows the read-back
  idempotency pattern; `resource.go` shows the unauthenticated internal routes.
- `apps/notification-service/internal/admin/` — the same, one service over.
- `apps/web/src/components/admin/PurgeConfirmDialog.tsx` — the template for the
  transfer dialog's mechanics.
- `apps/web/src/pages/admin/AdminFleetsPage.tsx` — `FleetDetail` is where the
  Transfer action lands.
- `apps/web/src/lib/hooks/api/admin.ts` — `adminKeys` and the purge mutations.

---

## 4. Decisions inherited from `design.md`

| # | Decision |
|---|---|
| D1 | All transfer logic lives in `internal/admin` as set-based SQL, **not** spread across seven domain packages as the PRD's §7 proposed. |
| D2 | Media/notification failure returns **503** (`ErrServiceUnavailable`), not the PRD's 502 — there is no 502 sentinel in this codebase. |
| D3 | `admin_audit_events` gains two nullable columns rather than overloading `target_label` or `affected_counts`. |
| D4 | One transaction; downstream calls **last**, inside it; compensation only when a downstream succeeded and something after it failed. |
| D5 | Widgets matched by parsing config in Go, not `config->>'vehicleId'` — the tests run on SQLite and a dialect branch on a *predicate* is the bug class this codebase has already been bitten by. |
| D6 | Category find-or-create: case-insensitive `name`, exact `kind`; a unique violation re-reads once. |
| D7 | Preview and apply share predicates. `media_objects` is the one honest exception, reported both ways. |
| D8 | `adminclient` starts sending `X-Correlation-ID` — fixes the transfer *and* retroactively the purge fan-out. |
| D9 | media-service's reassign mirrors `Stamp`: read the count back, never `RowsAffected`. |
| D10 | FR-XFER-MOVE-2's "byte-identical" list is narrowed; records and schedules are asserted identical *except* `category_id`. |
| D11 | The `activity_events` append-only comment is narrowed explicitly rather than left quietly false. `internal/activity` gains no update method. |
| D12 | notification-service **is** in scope (user-confirmed during planning), contrary to the PRD's "no changes". |

## 5. Decisions made during planning

1. **`BlastRadiusPanel` is not reused.** It hard-codes a "Delete this fleet"
   heading and a destructive purge button. The dialog renders its own counts
   list.
2. **The destination picker is a debounced search input over a list of buttons,
   not a Radix `<Select>`.** Live search needs a query; Radix `Select` is hostile
   to jsdom.
3. **`telemetry.headerCorrelationID` is exported** so `adminclient` does not
   duplicate the literal.
4. **`notifications` is absent from the preview.** The preview calls no other
   service, and unlike `media_objects` there is no fleet-service-side proxy for
   the count. It appears in the transfer's `affected_counts` only.
5. **`createErrorFromUnknown` is fixed** (Task 12). It currently discards
   `status` and `detail` when handed an `ApiError` — which is exactly what
   `ApiClient.request` throws — so FR-XFER-UI-7 cannot work without it. The same
   bug makes `useCreatePurge`'s `status === 409` and `=== 403` branches dead
   code today.

---

## 6. Constraints that will bite you

- **`internal/admin` imports no other fleet-service domain package.** Cross-domain
  work is raw schema-qualified SQL, or a port declared locally with the adapter
  in `cmd/main.go`. `arch_test.go` enforces it.
- **Every DB-backed `internal/admin` test runs on SQLite** via
  `admintest.NewDB`, using hand-written DDL. A column added to an entity must be
  added to that DDL in the same commit. No `->>`, no `ON CONFLICT`, no
  `json_extract`.
- **There are no migration files.** Schema changes are entity edits picked up by
  the package's `AutoMigrate`. New columns must be nullable.
- **`now` is a parameter, never SQL `now()`.**
- **`fleet.maintenance_categories` has no `deleted_at`, `created_at` or
  `purge_operation_id`.** Its columns are exactly
  `(id, name, description, system_defined, kind, fleet_id)`.
- **`fleet.vehicles.primary_image_media_id` is a NOT NULL string.** "None" is the
  empty string, so it is filtered with `<> ''`, never `IS NOT NULL`.
- **`pending_purge` means admin-STAMPED:** `deleted_at IS NOT NULL AND
  purge_operation_id IS NOT NULL`. A user-deleted row is a different state.
- **`resource_test.go`'s `adminRoutes` table is exhaustive by design** — the two
  new routes must be added to it or the guard test is silently incomplete.
- **`AdminFleetsPage.test.tsx`'s `vi.mock` factory is exhaustive** — the page
  throws the moment it calls a hook the factory omits.
- **Node is not always on `PATH`:**
  `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22`.

---

## 7. Task dependency order

```
1  audit columns + fixture DDL
2  transfer plan, counts, media-id union
3  categories            ─┐
4  widgets               ─┼─ all consume 2
5  ApplyTransfer         ─┘  consumes 3 + 4
6  adminclient (correlation id + Reassign ×2)
7  media-service reassign          ─┐ independent of 1–5;
8  notification-service reassign   ─┘ can run in parallel with them
9  transfer processor        consumes 5 + 6
10 routes, transforms, wiring, append-only amendment   consumes 9
11 frontend types + AdminService    consumes 10's contract
12 hooks (+ createErrorFromUnknown fix)    consumes 11
13 VehicleTransferDialog            consumes 12
14 AdminFleetsPage wiring           consumes 13
15 audit page + required markers    consumes 11, 13
16 make ci + overlays + code review
```

Tasks 6–8 have no dependency on 1–5 and are the obvious parallel branch if you
are dispatching subagents.

---

## 8. API contract, for the frontend half

```
GET  /api/fleet/admin/vehicles/{vehicleId}/transfer-preview?destination_fleet_id=
     -> 200 { data: { type: "vehicle-transfer-previews", id: <vehicleId>,
                      attributes: { vehicle_label, source_fleet_id,
                                    source_fleet_name, destination_fleet_id,
                                    destination_fleet_name, counts,
                                    categories_to_create, warnings } } }
     403 non-admin · 404 unknown vehicle or destination

POST /api/fleet/admin/vehicles/{vehicleId}/transfer
     body { data: { type: "vehicle-transfers",
                    attributes: { destination_fleet_id, confirmation } } }
     -> 200 { data: { type: "vehicle-transfers", id: <vehicleId>,
                      attributes: { vehicle_id, source_fleet_id,
                                    destination_fleet_id, transferred_at,
                                    affected_counts } } }
     403 non-admin/revoked
     404 unknown vehicle or destination fleet
     409 confirmation mismatch · vehicle pending purge · source fleet pending
         purge · destination unavailable
     422 destination missing, malformed, or equal to the current fleet
     503 a downstream refused; the transfer was rolled back whole
```

Every 4xx/503 carries an actionable `detail` and the console surfaces it
verbatim.

`affected_counts` keys: `maintenance_records`, `maintenance_schedules`,
`fuel_logs`, `mileage_records`, `vehicle_media`, `activity_events`,
`media_objects`, `notifications`, `categories_created`, `widgets_removed`.
The preview's `counts` carries the same set minus `notifications`.

---

## 9. Verification

```sh
make ci                                        # the gate
kustomize build deploy/k8s/overlays/local >/dev/null
kustomize build deploy/k8s/overlays/main  >/dev/null
kustomize build deploy/k8s/overlays/main  | kubectl apply --dry-run=server -f -
kustomize build deploy/k8s/overlays/local | kubectl apply --dry-run=server -f -
```

`make ci` already includes `tools/check-manifests.sh`, which is where the
gateway assertion the PRD demands lives — it requires exactly one priority-200
`internal-deny` route, with the `internal-deny` middleware, on **each**
entrypoint for every service exposing `/internal/*`. This task adds no service
to that set, so no deploy change is needed.

Run the code review (`superpowers:requesting-code-review` or `/audit-plan`)
before opening a PR. Findings go to `audit.md` in this folder.
