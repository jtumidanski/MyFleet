# Admin Vehicle Fleet Transfer — Product Requirements Document

Version: v1
Status: Draft
Created: 2026-08-25
---

## 1. Overview

A vehicle belongs to exactly one fleet, and today that binding is permanent.
`fleet.vehicles.fleet_id` is set at creation and no code path anywhere in the
platform ever changes it. When a household splits, when a car is sold between
two households that both use MyFleet, or when a vehicle was simply added to the
wrong fleet during onboarding, the only remedy available to an operator is to
delete the vehicle and have the destination fleet re-create it — which discards
every maintenance record, schedule, fuel log, mileage reading and photo attached
to it. That is a bad trade for what is fundamentally a one-column correction.

This feature adds a platform-admin-only **vehicle transfer** operation: reassign
a vehicle, together with its full history, from a source fleet to a destination
fleet. It is surfaced from the vehicle rows already rendered in the fleet
inspector (`apps/web/src/pages/admin/AdminFleetsPage.tsx:276`) and is gated by
the same `platform_admin` claim that gates every other admin console route.

The operation is not a single UPDATE. Most of the vehicle's history keys on
`vehicle_id` alone and follows the car for free, but three categories of data are
scoped to the *fleet* rather than the vehicle and would break silently on a naive
move: media objects (fleet-scoped in a different service, with access gated on
`fleet_id` equality), custom maintenance categories (fleet-scoped rows referenced
by moved records and schedules), and activity events plus dashboard widgets (both
carrying the source fleet's identity). Getting those three right is the substance
of this task; the `fleet_id` write itself is trivial.

## 2. Goals

Primary goals:

- Let a platform admin move one vehicle from one fleet to another without data loss.
- Carry the vehicle's complete history with it: maintenance records, maintenance
  schedules, fuel logs, mileage readings, media attachments and activity events.
- Keep the destination fleet's view of the transferred vehicle fully functional —
  photos render, receipts download, maintenance categories resolve to names.
- Leave the source fleet in a consistent state: no dangling dashboard widgets, no
  vehicle count drift, and a visible record that the car left.
- Record every transfer in the existing admin audit log with enough detail to
  reconstruct what moved.
- Make the operation deliberate: a server-verified typed confirmation phrase,
  preceded by a blast-radius summary of exactly what will move.

Non-goals:

- Self-service transfer by fleet owners or members. This is admin-only in v1.
- Bulk transfer of multiple vehicles, or transfer of an entire fleet's contents.
- An undo or reverse operation. A transfer is corrected by performing a second
  transfer in the opposite direction, which the same endpoint already supports.
- Moving memberships, users, invites, or fleet-level settings.
- Transferring a vehicle to a fleet that does not yet exist (no create-on-transfer).

## 3. User Stories

- As a platform admin, I want to move a vehicle to a different fleet so that a
  car added to the wrong household during onboarding can be corrected without
  destroying its service history.
- As a platform admin, I want to see exactly what will move before I confirm, so
  that I understand the blast radius of an operation I cannot undo with one click.
- As a platform admin, I want to type a confirmation phrase, so that I cannot
  transfer a vehicle by reflex from a list view.
- As a platform admin, I want the transfer recorded in the audit log with source
  fleet, destination fleet, vehicle and affected counts, so that I can answer
  "where did this car go" months later.
- As a member of the destination fleet, I want the transferred vehicle's photos,
  receipts and maintenance history to work exactly as if the car had always been
  ours, so that the transfer is invisible in day-to-day use.
- As a member of the source fleet, I want the vehicle to disappear cleanly from
  my dashboard and vehicle list, so that I am not left with broken widgets
  pointing at a car I no longer own.

## 4. Functional Requirements

### 4.1 Eligibility and validation (FR-XFER-ELIG-*)

- **FR-XFER-ELIG-1** — The transfer endpoint MUST reject any caller whose
  `platform_admin` claim is false with `403`, using the existing
  `authz.RequirePlatformAdmin` guard.
- **FR-XFER-ELIG-2** — The source vehicle MUST exist and MUST NOT be soft-deleted
  (`deleted_at IS NULL`). A soft-deleted or pending-purge vehicle returns `409`
  with detail `vehicle is pending purge and cannot be transferred`.
- **FR-XFER-ELIG-3** — The destination fleet MUST exist, MUST NOT be soft-deleted,
  and MUST NOT be pending purge. Otherwise `409` with detail
  `destination fleet is not available`.
- **FR-XFER-ELIG-4** — The destination fleet MUST differ from the vehicle's
  current fleet. A same-fleet transfer returns `422` with detail
  `vehicle already belongs to that fleet` rather than succeeding as a no-op.
- **FR-XFER-ELIG-5** — A transfer MUST be refused with `409` if the source fleet
  itself is pending purge, because the outcome would depend on whether the reaper
  runs before or after the move.

### 4.2 Confirmation (FR-XFER-CONF-*)

- **FR-XFER-CONF-1** — The request MUST carry a `confirmation` string. The server
  compares it **exactly** — no trimming, no case folding — against the source
  vehicle's display label, mirroring `admin.MatchConfirmation` for `ScopeFleet`
  (`apps/fleet-service/internal/admin/confirmation.go`).
- **FR-XFER-CONF-2** — The confirmation label is the vehicle's `nickname` when
  non-empty, otherwise `"{year} {make} {model}"`. The label the client must type
  MUST be returned by the preview endpoint (§5.1) so the console never has to
  derive it independently.
- **FR-XFER-CONF-3** — A mismatched confirmation returns `409` and MUST write
  nothing — no `fleet_id` change, no audit row, no cross-service call.
- **FR-XFER-CONF-4** — The console's disabled confirm button is a courtesy. The
  server check is the control and MUST be independently tested.

### 4.3 What moves (FR-XFER-MOVE-*)

- **FR-XFER-MOVE-1** — `fleet.vehicles.fleet_id` MUST be set to the destination
  fleet ID. `created_at` MUST be preserved (the `<-:create` column tag and the
  `ToEntity` assignment in `apps/fleet-service/internal/vehicle/entity.go` both
  already guard this; the transfer path MUST NOT bypass them).
- **FR-XFER-MOVE-2** — Vehicle-scoped children require no rewrite because they key
  on `vehicle_id` only. The implementation MUST NOT touch them, and a test MUST
  assert that their rows are byte-identical before and after a transfer, for:
  `fleet.mileage_records`, `fleet.fuel_logs`, `fleet.maintenance_records`,
  `fleet.maintenance_schedules`, `fleet.vehicle_media`.
- **FR-XFER-MOVE-3** — Activity events for the vehicle
  (`fleet.activity_events WHERE vehicle_id = ?`) MUST have their `fleet_id`
  rewritten to the destination fleet, so the car's timeline follows the car.
- **FR-XFER-MOVE-4** — Fleet-scoped activity events with a NULL `vehicle_id` MUST
  NOT be touched; they describe the fleet, not the vehicle.

### 4.4 Media reassignment (FR-XFER-MEDIA-*)

- **FR-XFER-MEDIA-1** — Every media object referenced by the vehicle MUST have its
  `media.media_objects.fleet_id` rewritten to the destination fleet. Without this,
  destination-fleet members receive `403` from
  `apps/media-service/internal/mediaobject/processor.go:139`, which gates access
  on `m.FleetID() != identityFleetID`.
- **FR-XFER-MEDIA-2** — The set of media IDs to reassign is the union of:
  - `fleet.vehicle_media.media_id` for the vehicle,
  - `fleet.vehicles.primary_image_media_id` for the vehicle,
  - every receipt/attachment media ID referenced by the vehicle's maintenance
    records (task-027 attachment management is the source of truth for that
    reference; the design phase MUST enumerate the exact columns).
- **FR-XFER-MEDIA-3** — fleet-service MUST call media-service over the existing
  internal admin channel (`adminclient.MediaClient`, base `MEDIA_INTERNAL_URL`),
  adding a `Reassign` method. It MUST NOT reach into media-service's database.
- **FR-XFER-MEDIA-4** — The media reassign call MUST be idempotent: replaying it
  with the same media IDs and destination fleet returns the same affected counts
  and changes nothing further, matching the contract `MediaClient.Purge` already
  honours (FR-ADMIN-PURGE-10).
- **FR-XFER-MEDIA-5** — If the media reassign call fails, the fleet-service
  transaction MUST roll back and the transfer MUST return `502` with detail
  naming media-service. A partial transfer — vehicle moved, photos stranded — is
  not an acceptable outcome. See §8 for the ordering that makes this achievable.

### 4.5 Maintenance category remapping (FR-XFER-CAT-*)

- **FR-XFER-CAT-1** — Maintenance categories with a non-NULL `fleet_id` are scoped
  to one fleet (`apps/fleet-service/internal/maintenancecategory/entity.go:35`).
  Records and schedules on the moved vehicle that reference a **source-fleet**
  category MUST be remapped, or the destination fleet cannot resolve the name.
- **FR-XFER-CAT-2** — References to system categories (`fleet_id IS NULL`) MUST be
  left untouched; they are globally visible.
- **FR-XFER-CAT-3** — For each distinct source-fleet category referenced by the
  moved vehicle's records or schedules, the transfer MUST find-or-create an
  equivalent category in the destination fleet, matching on case-insensitive
  `name` **and** exact `kind`, consistent with the existing `FindByName`
  case-insensitive dedupe.
- **FR-XFER-CAT-4** — A newly created destination category MUST copy `name`,
  `description` and `kind` from the source, set `fleet_id` to the destination
  fleet, and set `system_defined = false`.
- **FR-XFER-CAT-5** — `fleet.maintenance_records.category_id` and
  `fleet.maintenance_schedules.category_id` for the moved vehicle MUST be rewritten
  to the resolved destination category IDs.
- **FR-XFER-CAT-6** — Source-fleet categories MUST NOT be deleted, renamed or
  re-scoped, even when the moved vehicle was their only consumer. Other vehicles
  or future records in the source fleet may still use them.

### 4.6 Source fleet cleanup (FR-XFER-SRC-*)

- **FR-XFER-SRC-1** — Dashboard widgets in the **source** fleet whose config pins
  the moved `vehicleId` (`apps/web/src/types/models/dashboard.ts:47`) MUST be
  deleted, so the source fleet is not left with widgets resolving to a vehicle it
  can no longer read.
- **FR-XFER-SRC-2** — Widget deletion MUST be scoped to dashboards belonging to
  the source fleet. Widgets in the destination fleet, or in any third fleet, MUST
  NOT be touched.
- **FR-XFER-SRC-3** — Widgets whose config does not reference a vehicle, or
  references a different vehicle, MUST be left intact. Config is `jsonb`; the
  match MUST be on the parsed `vehicleId` value, not a substring scan.
- **FR-XFER-SRC-4** — The transfer MUST write one activity event into the **source**
  fleet recording that the vehicle left, and one into the **destination** fleet
  recording that it arrived. Both carry the vehicle ID and the counterpart fleet.

### 4.7 Audit (FR-XFER-AUDIT-*)

- **FR-XFER-AUDIT-1** — A successful transfer MUST write one row to
  `fleet.admin_audit_events` with `action = "vehicle.transferred"`, added
  alongside the existing `purge.*` constants in
  `apps/fleet-service/internal/admin/entity.go:73`.
- **FR-XFER-AUDIT-2** — The audit row MUST carry `actor_user_id`, `actor_email`,
  `target_type = "vehicle"`, `target_id` = the vehicle ID, a denormalised
  `target_label` (the confirmation label from FR-XFER-CONF-2), the correlation ID
  from `telemetry.CorrelationIDFromContext`, and `affected_counts`.
- **FR-XFER-AUDIT-3** — `affected_counts` MUST include per-table counts of what
  moved: `maintenance_records`, `maintenance_schedules`, `fuel_logs`,
  `mileage_records`, `vehicle_media`, `media_objects`, `activity_events`,
  `categories_created`, `widgets_removed`.
- **FR-XFER-AUDIT-4** — Source and destination fleet IDs MUST be recoverable from
  the audit row. Since `AuditEvent` has no dedicated columns for them, the design
  phase MUST choose between adding columns and encoding them in `target_label`;
  encoding-only is NOT acceptable because the label is human-facing text.
- **FR-XFER-AUDIT-5** — The transfer action MUST be selectable in the existing
  `?action=` filter on `GET /admin/audit-events` and MUST render on
  `AdminAuditPage` without a client change beyond widening the `AuditAction` union.

### 4.8 Console UX (FR-XFER-UI-*)

- **FR-XFER-UI-1** — Each vehicle row in the fleet inspector's Vehicles table
  (`AdminFleetsPage.tsx:289`) gains a **Transfer** action.
- **FR-XFER-UI-2** — The action opens a dialog modelled on `PurgeConfirmDialog`
  containing: a destination fleet picker, a blast-radius panel, and a typed
  confirmation input.
- **FR-XFER-UI-3** — The destination picker MUST search live fleets by name using
  the existing `GET /admin/fleets?q=&deleted=exclude` endpoint. The source fleet
  MUST be excluded from results.
- **FR-XFER-UI-4** — The blast-radius panel MUST be populated from the preview
  endpoint (§5.1), not computed client-side, and MUST show counts for each
  category in FR-XFER-AUDIT-3 plus the names of any categories that will be
  created in the destination fleet.
- **FR-XFER-UI-5** — The confirm button MUST be disabled until the typed input
  exactly equals the label returned by the preview endpoint and a destination is
  selected.
- **FR-XFER-UI-6** — On success the dialog closes, a toast confirms the move
  naming the destination fleet, and the fleet detail query is invalidated so the
  vehicle disappears from the source fleet's list.
- **FR-XFER-UI-7** — Server errors MUST be surfaced verbatim from the JSON:API
  error detail rather than replaced with a generic message; `409` and `422` cases
  in §4.1 all carry actionable detail.
- **FR-XFER-UI-8** — Vehicles that are pending purge MUST render the Transfer
  action disabled with a tooltip explaining why, matching FR-XFER-ELIG-2. The
  existing `AdminVehicleRow.pending_purge` field already carries this.

## 5. API Surface

Traefik strips `/api/fleet`, so a service-registered `/admin/...` route is the
gateway path `/api/fleet/admin/...`. All routes below are registered in
`apps/fleet-service/internal/admin/resource.go` and return `403` when the
caller's `platform_admin` claim is false.

### 5.1 Preview

```
GET /api/fleet/admin/vehicles/{vehicleId}/transfer-preview?destination_fleet_id={id}
```

`destination_fleet_id` is optional. Without it, the response omits
`categories_to_create` (which cannot be computed without a destination) and
returns the move counts only.

```json
{
  "data": {
    "type": "vehicle-transfer-previews",
    "id": "<vehicleId>",
    "attributes": {
      "vehicle_label": "The Green Bean",
      "source_fleet_id": "…",
      "source_fleet_name": "Tumidanski Household",
      "destination_fleet_id": "…",
      "destination_fleet_name": "Smith Household",
      "counts": {
        "maintenance_records": 42,
        "maintenance_schedules": 6,
        "fuel_logs": 118,
        "mileage_records": 90,
        "vehicle_media": 7,
        "media_objects": 9,
        "activity_events": 210,
        "widgets_removed": 2
      },
      "categories_to_create": [
        { "name": "Winter Tires", "kind": "maintenance" }
      ],
      "warnings": []
    }
  }
}
```

`vehicle_label` is the exact string the confirmation must match (FR-XFER-CONF-2).
`warnings` carries degradation notes in the same spirit as the existing admin
list endpoints — e.g. media-service unreachable, so `media_objects` is an estimate
from `vehicle_media` alone.

Errors: `403` non-admin, `404` unknown vehicle, `404` unknown destination fleet.

### 5.2 Transfer

```
POST /api/fleet/admin/vehicles/{vehicleId}/transfer
```

Request (JSON:API, matching the `server.RegisterInputHandler` shape used by
`POST /admin/purge-operations`):

```json
{
  "data": {
    "type": "vehicle-transfers",
    "attributes": {
      "destination_fleet_id": "…",
      "confirmation": "The Green Bean"
    }
  }
}
```

Response `200` — the updated vehicle row as the admin console already types it
(`AdminVehicleRow`), plus the affected counts actually applied:

```json
{
  "data": {
    "type": "vehicle-transfers",
    "id": "<vehicleId>",
    "attributes": {
      "vehicle_id": "…",
      "source_fleet_id": "…",
      "destination_fleet_id": "…",
      "transferred_at": "2026-08-25T18:00:00Z",
      "affected_counts": { "…": 0 }
    }
  }
}
```

Error cases:

| Status | Condition |
|--------|-----------|
| `403` | caller is not a platform admin |
| `404` | vehicle does not exist |
| `409` | confirmation phrase mismatch (nothing written) |
| `409` | vehicle pending purge, source fleet pending purge, or destination fleet unavailable |
| `422` | `destination_fleet_id` missing, malformed, or equal to the current fleet |
| `502` | media-service reassign failed; transfer rolled back |

### 5.3 media-service internal admin

```
POST /internal/admin/reassign-fleet
```

Registered in `apps/media-service/internal/admin/resource.go` beside the existing
`/internal/admin/purge` routes. Not exposed through the gateway.

```json
{ "media_ids": ["…"], "destination_fleet_id": "…" }
```

Response: `{ "affected": { "media_objects": 9 } }`

Idempotent (FR-XFER-MEDIA-4): media objects already carrying
`destination_fleet_id` are counted as affected but produce no write. Media IDs
that do not exist are ignored rather than erroring, matching the tolerance the
purge path already shows.

## 6. Data Model

No new tables. Column-level changes:

| Table | Change |
|-------|--------|
| `fleet.vehicles` | `fleet_id` rewritten (existing column, no migration) |
| `fleet.activity_events` | `fleet_id` rewritten for rows with the moved `vehicle_id` |
| `fleet.maintenance_records` | `category_id` remapped where it referenced a source-fleet category |
| `fleet.maintenance_schedules` | `category_id` remapped where it referenced a source-fleet category |
| `fleet.maintenance_categories` | new rows inserted in the destination fleet (find-or-create) |
| `fleet.dashboard_widgets` | source-fleet rows pinned to the moved vehicle deleted |
| `fleet.admin_audit_events` | one new row per transfer; possibly two new columns per FR-XFER-AUDIT-4 |
| `media.media_objects` | `fleet_id` rewritten for the vehicle's media |

Migration notes:

- The only candidate schema migration is the source/destination fleet columns on
  `admin_audit_events` (FR-XFER-AUDIT-4). If the design phase adds them, they MUST
  be nullable so existing `purge.*` rows remain valid, following the pattern used
  for `maintenance_categories.kind`.
- The find-or-create in FR-XFER-CAT-3 must respect the existing composite unique
  index `idx_maintenance_categories_scope` on `(fleet_id, name, kind)`. A
  concurrent duplicate insert surfaces as a unique violation and MUST be handled
  as "someone else created it, re-read it", not as a 500.

## 7. Service Impact

**fleet-service** — the bulk of the work.

- `internal/admin/` — new transfer processor, resource routes, preview assembly,
  confirmation reuse, `ActionVehicleTransferred` audit constant. The package's
  `arch_test.go` forbids reaching into other domains' internals, so cross-domain
  work goes through ports/adapters exactly as `VehicleStatusDeriver` already does
  (`internal/admin/browse.go:44`).
- `internal/vehicle/` — an administrator method to reassign `fleet_id` that
  preserves `created_at`.
- `internal/activity/` — bulk `fleet_id` rewrite by `vehicle_id`, plus the two
  transfer events (FR-XFER-SRC-4).
- `internal/maintenancecategory/` — find-or-create in a target fleet.
- `internal/maintenancerecord/`, `internal/maintenanceschedule/` — bulk
  `category_id` remap for one vehicle.
- `internal/dashboard/` — delete source-fleet widgets pinned to a vehicle.
- `internal/adminclient/media.go` — new `Reassign` method.

**media-service**

- `internal/admin/` — new `/internal/admin/reassign-fleet` route.
- `internal/mediaobject/` — administrator method to rewrite `fleet_id` for a set
  of media IDs, idempotently.

**web**

- `services/api/AdminService.ts` — `previewVehicleTransfer`, `transferVehicle`.
- `lib/hooks/api/admin.ts` — corresponding query and mutation hooks with cache
  invalidation of the fleet detail and fleet list queries.
- `components/admin/VehicleTransferDialog.tsx` — new, modelled on
  `PurgeConfirmDialog`.
- `pages/admin/AdminFleetsPage.tsx` — Transfer action on each vehicle row.
- `types/models/admin.ts` — preview/transfer attribute types; widen `AuditAction`.

**auth-service, notification-service** — no changes. Nothing they own is
vehicle-scoped.

## 8. Non-Functional Requirements

**Atomicity and ordering.** All fleet-service writes MUST occur inside one
transaction. The media-service call cannot join that transaction, so ordering
matters: perform the media reassign **first**, and only commit the local
transaction if it succeeded. If the local transaction then fails, the compensating
action is a reverse media reassign back to the source fleet; if that compensation
also fails, the operation MUST log an error naming both fleet IDs and the media
IDs, and return `502`. FR-XFER-MEDIA-4's idempotency is what makes the
compensation safe to attempt.

**Performance.** Every rewrite MUST be a set-based `UPDATE ... WHERE vehicle_id =
?`, never a row-by-row loop. The preview endpoint MUST use `COUNT` aggregates and
MUST NOT load any of the counted rows. A vehicle with 10k combined history rows
must transfer within the service's normal request timeout.

**Security.** `platform_admin` is the only authorisation. The hidden nav entry is
cosmetic and the server is authoritative, as the existing admin routes already
document. The `/internal/admin/reassign-fleet` route MUST NOT be routable through
Traefik — the design phase MUST verify this against the gateway config rather
than assume it, since it grants the ability to move any media object into any
fleet.

**Observability.** The transfer MUST log at info on success with vehicle ID,
source fleet, destination fleet, actor and affected counts, and at error on the
compensation path. The correlation ID MUST propagate to the media-service call.

**Concurrency.** The vehicle row MUST be locked for the duration of the
transaction (`SELECT ... FOR UPDATE`) so two concurrent transfers of the same
vehicle serialise rather than interleave.

## 9. Open Questions

- **OQ-1** — Which columns carry maintenance-record receipt/attachment media IDs?
  FR-XFER-MEDIA-2 depends on enumerating them exactly; task-027 (record attachment
  management) is the source of truth and must be read during design.
- **OQ-2** — FR-XFER-AUDIT-4: add `source_fleet_id`/`destination_fleet_id` columns
  to `admin_audit_events`, or store them in `affected_counts`' sibling jsonb? A
  migration is cheap; overloading an existing column is not.
- **OQ-3** — Should the destination fleet's members receive a notification that a
  vehicle arrived? Deferred — notification-service is untouched in v1, but this is
  a plausible fast-follow.
- **OQ-4** — Does any fleet-scoped read path cache vehicle→fleet mappings (for
  example a dashboard summary or the activity feed) in a way that would serve
  stale data after a transfer? Design must check before assuming invalidation is
  purely client-side.
- **OQ-5** — Is `mileage_records` genuinely vehicle-only, or does any downstream
  consumer join it back to a fleet? FR-XFER-MOVE-2 asserts vehicle-only based on
  `internal/mileage/entity.go:12`; design should confirm no view or query
  re-derives a fleet from it.

## 10. Acceptance Criteria

- [ ] `POST /api/fleet/admin/vehicles/{id}/transfer` returns `403` for a
      non-platform-admin caller, verified by test.
- [ ] A transfer with a mismatched confirmation returns `409` and leaves
      `fleet.vehicles.fleet_id`, the audit table and media-service untouched.
- [ ] A successful transfer sets the vehicle's `fleet_id` to the destination and
      preserves `created_at` unchanged.
- [ ] Maintenance records, schedules, fuel logs, mileage records and vehicle_media
      rows are unchanged by the transfer (asserted, not assumed).
- [ ] The vehicle's media objects carry the destination `fleet_id` afterwards, and
      a destination-fleet identity can fetch them without a `403`.
- [ ] A source-fleet identity can no longer fetch the transferred vehicle's media.
- [ ] A record referencing a source-fleet custom category resolves, after transfer,
      to a destination-fleet category with the same name, kind and description.
- [ ] A record referencing a system category (`fleet_id IS NULL`) is not remapped.
- [ ] The source-fleet category rows still exist after transfer.
- [ ] Activity events for the vehicle carry the destination `fleet_id`; fleet-level
      events with NULL `vehicle_id` are untouched.
- [ ] Source-fleet dashboard widgets pinned to the moved vehicle are gone;
      destination-fleet and unrelated-vehicle widgets are intact.
- [ ] One `vehicle.transferred` audit row exists with actor, target label,
      correlation ID and per-table `affected_counts`, and source/destination fleet
      IDs are recoverable from it.
- [ ] The audit row is returned by `GET /admin/audit-events?action=vehicle.transferred`
      and renders on `AdminAuditPage`.
- [ ] Transfer to a pending-purge destination, from a pending-purge source, of a
      pending-purge vehicle, and to the vehicle's own current fleet each return the
      documented `409`/`422` with an actionable detail.
- [ ] A simulated media-service failure rolls the transfer back completely and
      returns `502`; the vehicle is still in the source fleet and its media still
      readable there.
- [ ] Replaying `/internal/admin/reassign-fleet` with the same input is a no-op
      that reports the same counts.
- [ ] The preview endpoint returns counts matching what a subsequent transfer
      actually reports, and returns the exact confirmation label.
- [ ] The Transfer action appears on each vehicle row in the fleet inspector, is
      disabled with a tooltip for pending-purge vehicles, and its dialog blocks
      confirmation until a destination is chosen and the label typed exactly.
- [ ] The destination picker excludes the source fleet and soft-deleted fleets.
- [ ] After a successful transfer the vehicle disappears from the source fleet's
      detail view without a manual refresh.
- [ ] `make ci` passes.
