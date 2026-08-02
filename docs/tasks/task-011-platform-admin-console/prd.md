# Platform Admin Console — Product Requirements Document

Version: v1
Status: Draft
Created: 2026-08-02
---

## 1. Overview

MyFleet today has exactly one authorization model: **fleet-scoped membership**. A user's JWT
carries `active_fleet_id` and a `role` of `owner | member | viewer`, and every handler in
`fleet-service` calls `authz.RequireSameFleet` before doing anything. That guard deliberately
returns **404, not 403**, so a caller can never learn that a fleet they don't belong to exists
(`apps/fleet-service/internal/authz/scope.go:11`). The design has no notion of a person who
stands above fleets.

This task introduces that notion. A **platform administrator** is a named, allow-listed operator
who can see the whole solution at once: aggregate counts across every fleet, a browsable list of
all fleets with their detail, and the ability to delete data at three levels of granularity —
a single record, an entire fleet, or the whole system. The first and initially only administrator
is `jtumidanski@gmail.com`. This is an operator tool, not a product feature: it exists so the
person running the platform can inspect real usage and reclaim state without reaching for `psql`.

The destructive half of this is the risky half, so it is built on **soft deletion with a delayed
reaper** rather than immediate hard deletes. An admin purge stamps `deleted_at` on the affected
rows and records a **purge operation** with a `purge_after` timestamp; a background sweep
hard-deletes the rows once the recovery window elapses. Until then the operation can be cancelled,
restoring every row it touched. This makes the console fast (a purge is an indexed `UPDATE`, not a
cascading `DELETE` across four databases), recoverable (a misclick is undoable for days), and
consistent with the machinery that already exists for vehicles
(`apps/fleet-service/internal/vehicle/purge.go`).

Two structural facts about the codebase shape nearly every requirement below, and both were
verified against source rather than assumed:

- **Only 6 of the 16 purgeable tables currently have a `deleted_at` column.** Soft-delete is not
  a uniform capability today; it was added ad hoc where a product feature needed it. Ten tables
  must gain the column, and — far more dangerously — every existing read path over those tables
  must start filtering on it.
- **There are no foreign keys anywhere.** Every entity is registered via GORM `AutoMigrate` with
  plain indexed `uuid` columns and no `constraint:` tags. Nothing cascades in the database. This
  means cascade deletion must be written by hand, and it means the *existing* vehicle purge job is
  already orphaning child rows (see §11).

## 2. Goals

Primary goals:

- Establish a **platform-admin authorization tier** that is orthogonal to fleet membership, sourced
  from a database table so administrators can be granted and revoked without a redeploy.
- Give an administrator a **solution-wide aggregate view**: how many fleets, vehicles, maintenance
  records, fuel logs, mileage records, users, media objects, and notifications exist.
- Give an administrator a **cross-fleet drill-down**: list every fleet, open one, and see its
  members, vehicles, and per-domain record counts — bypassing the normal same-fleet guard.
- Provide **three tiers of deletion** — granular record, whole fleet, whole system — all expressed
  as a single uniform "purge operation" concept.
- Make every purge **recoverable within a bounded window** and **cancellable** before it is reaped.
- **Audit every administrative action** with actor, target, and timestamp.

Non-goals:

- A UI for managing the administrator allow-list. Granting admin is a deliberate out-of-band act
  (SQL insert or seed); the console displays who is an admin but does not grant or revoke.
- User impersonation or "log in as" functionality.
- GDPR-style per-user data export, or any export/download of data.
- Editing domain data through the console. The console reads and deletes; it does not create or
  update fleets, vehicles, or records.
- Billing, quotas, or per-tenant limits.
- Deleting user accounts. `auth.users` survives every purge tier (see §2 blast radius, §6.5).
- Retrofitting soft-delete onto `fleet.maintenance_categories`. It is seeded reference data and is
  explicitly out of the purge blast radius.

### Purge blast radius (decided)

| Outcome | Scope |
|---|---|
| **Purged** | All of `fleet.*` except `maintenance_categories`; all of `media.*` including the backing MinIO objects; all of `notification.*` |
| **Kept** | `auth.users`, `auth.refresh_tokens`, `auth.platform_admins`, `fleet.maintenance_categories` |

Keeping `auth.*` intact is what makes a system purge usable as a reset: the administrator stays
logged in, their token stays valid, and because their fleet is gone the existing `RequireAuth`
guard lands them on `/onboarding` to rebuild from scratch.

## 3. User Stories

- As a platform administrator, I want to open a console at `/admin` that only I can see, so that
  ordinary users have no entry point to destructive tooling.
- As a platform administrator, I want to see total counts of fleets, vehicles, maintenance records,
  fuel logs, mileage records, users, media objects, and notifications on one screen, so that I can
  tell at a glance how much real data the platform holds.
- As a platform administrator, I want to list every fleet in the system with its owner, member
  count, vehicle count, and creation date, so that I can find a specific fleet without querying the
  database directly.
- As a platform administrator, I want to open a single fleet and see its members, vehicles, and
  per-domain record counts, so that I can understand what purging it would destroy before I do it.
- As a platform administrator, I want to delete one specific record (a vehicle, a fuel log, a
  maintenance record, a membership, an invite), so that I can surgically remove bad data.
- As a platform administrator, I want to purge an entire fleet and everything beneath it, so that I
  can remove a test or abandoned tenant in one action.
- As a platform administrator, I want to purge all domain data system-wide while keeping user
  accounts, so that I can reset a staging environment without re-authenticating.
- As a platform administrator, I want to type a confirmation phrase before any fleet-level or
  system-level purge executes, so that I cannot destroy a tenant by misclicking.
- As a platform administrator, I want to see pending purge operations and cancel one before it is
  reaped, so that a mistake is recoverable rather than terminal.
- As a platform administrator, I want every administrative action recorded in an audit log I can
  read in the console, so that there is a durable record of what was deleted, by whom, and when.
- As an ordinary user, I want soft-deleted data to be invisible to me immediately, so that a
  pending purge does not leave ghost records in my fleet during the recovery window.

## 4. Functional Requirements

### 4.1 Administrator identity and authorization

- **FR-ADMIN-AUTH-1** — `auth-service` owns a new table `auth.platform_admins` keyed by user id,
  recording who granted the privilege and when. Its migration seeds the row for
  `jtumidanski@gmail.com` if that user exists, and is idempotent across repeated startups.
- **FR-ADMIN-AUTH-2** — Because the seed is keyed on a user that may not exist yet at first
  migration, the seed must also apply at provisioning time: when
  `user.Processor.ProvisionFromGoogle` creates or updates a user whose email is in the bootstrap
  admin list, the `auth.platform_admins` row is created if absent.
- **FR-ADMIN-AUTH-3** — The bootstrap admin email list is read from config
  (`PLATFORM_ADMIN_BOOTSTRAP_EMAILS`, default `jtumidanski@gmail.com`). It seeds the table only; it
  is **not** consulted on every request. The table is the runtime source of truth, so an admin can
  be revoked by deleting the row without a redeploy.
- **FR-ADMIN-AUTH-4** — `session.Processor.MintAccess` stamps a boolean `platform_admin` claim on
  the access token, sourced from a `auth.platform_admins` lookup performed at mint time. The claim
  is emitted on both the initial login mint and the refresh mint, so it survives token rotation.
- **FR-ADMIN-AUTH-5** — `auth.Identity` in `packages/shared-go/auth/identity.go` gains a
  `PlatformAdmin bool` field, populated by the existing JWT middleware from the `platform_admin`
  claim. A missing or non-boolean claim parses to `false`.
- **FR-ADMIN-AUTH-6** — A new guard `authz.RequirePlatformAdmin(id auth.Identity) error` returns
  `server.ErrForbidden` (403) when `PlatformAdmin` is false. Unlike `RequireSameFleet`, it returns
  403 rather than 404: the existence of an admin API is not a secret, only the authority to use it.
- **FR-ADMIN-AUTH-7** — Because the claim is stamped at mint time, revoking an admin does not take
  effect until the access token expires (15 minutes) or is refreshed. This staleness is accepted
  and must be documented in the console UI. Purge endpoints — and only purge endpoints —
  additionally re-verify against `auth.platform_admins` over HTTP before executing, so a revoked
  admin cannot destroy data with a stale token.
- **FR-ADMIN-AUTH-8** — `GET /auth/me` includes `platform_admin` in its response so the web client
  can decide whether to render the console entry point.
- **FR-ADMIN-AUTH-9** — Admin endpoints must not require an `active_fleet_id`. An administrator with
  no fleet (including immediately after a system purge) must still reach every admin endpoint.

### 4.2 API topology

- **FR-ADMIN-API-1** — `fleet-service` hosts the entire public admin API under a `/admin` route
  prefix, registered as its **own `chi` route group**, distinct from the existing authenticated
  group. No handler under `/admin` may call `authz.RequireSameFleet`, and no handler outside
  `/admin` may call `authz.RequirePlatformAdmin`. This separation is structural, not conventional:
  the same-fleet guard is never relaxed, it is simply absent from a parallel tree.
- **FR-ADMIN-API-2** — `fleet-service` orchestrates. For data it does not own it calls
  `media-service`, `notification-service`, and `auth-service` over HTTP, following the existing
  `mediaclient` pattern (`apps/fleet-service/internal/mediaclient/client.go`).
- **FR-ADMIN-API-3** — Each of the other three services exposes the slice of admin functionality it
  owns on its **internal** route tree (the existing `InitializeInternalRoutes` pattern), reachable
  only from inside the cluster and never routed through the public gateway.
- **FR-ADMIN-API-4** — All admin responses follow JSON:API conventions, consistent with the rest of
  the platform.

### 4.3 Aggregate statistics

- **FR-ADMIN-STATS-1** — `GET /admin/stats` returns solution-wide counts: fleets, vehicles,
  memberships, pending invites, maintenance records, maintenance schedules, fuel logs, mileage
  records, activity events, users, media objects, and notifications.
- **FR-ADMIN-STATS-2** — Counts exclude soft-deleted rows (`deleted_at IS NULL`). A pending purge
  is reflected immediately in the numbers, so the console never reports data the product no longer
  shows.
- **FR-ADMIN-STATS-3** — Vehicle count is reported as two numbers: active, and soft-deleted-pending-
  purge. The second makes the recovery window visible rather than making data appear to vanish.
- **FR-ADMIN-STATS-4** — User count comes from `auth-service`, media object count from
  `media-service`, notification count from `notification-service`, each over HTTP.
- **FR-ADMIN-STATS-5** — If a downstream service is unreachable, `/admin/stats` still returns 200
  with the counts it could gather; unavailable counts are reported as `null` with the failing
  source named in a `warnings` array. A single dead service must not blank the whole dashboard.

### 4.4 Fleet browsing

- **FR-ADMIN-FLEET-1** — `GET /admin/fleets` returns a paginated list of every fleet in the system
  regardless of the caller's membership, with: id, name, created date, owner display name and
  email, member count, active vehicle count, and soft-delete state.
- **FR-ADMIN-FLEET-2** — The list supports `?q=` substring search on fleet name and owner email, and
  `?include_deleted=true` to surface fleets pending purge. Default excludes soft-deleted fleets.
- **FR-ADMIN-FLEET-3** — Pagination follows the platform's existing page/size convention with a
  default size of 25 and a hard maximum of 100.
- **FR-ADMIN-FLEET-4** — `GET /admin/fleets/{fleetId}` returns fleet detail: attributes, full member
  list (user id, email, display name, role, joined date), vehicle list (id, nickname, make, model,
  year, current mileage, status, soft-delete state), pending invites, and per-domain record counts
  for maintenance records, schedules, fuel logs, mileage records, and activity events.
- **FR-ADMIN-FLEET-5** — Member emails and display names are resolved from `auth-service`. If that
  lookup fails the endpoint still returns 200 with user ids and null names, flagged in `warnings`.
- **FR-ADMIN-FLEET-6** — `GET /admin/users` returns a paginated list of all users (id, email,
  display name, created date, last login, platform-admin flag, and the fleets they belong to),
  proxied from `auth-service` and joined against fleet memberships.

### 4.5 Purge operations

All three deletion tiers are expressed as one resource so the semantics, audit trail, recovery
window, and cancellation path are identical regardless of blast radius.

- **FR-ADMIN-PURGE-1** — `POST /admin/purge-operations` creates a purge operation. The request
  carries `scope` (`system | fleet | record`), and for non-system scopes a `target_type` and
  `target_id`. It returns the created operation including `purge_after`.
- **FR-ADMIN-PURGE-2** — `scope: record` supports these `target_type` values: `vehicle`,
  `maintenance_record`, `maintenance_schedule`, `fuel_log`, `mileage_record`, `membership`,
  `invite`, `vehicle_media`, `activity_event`. Any other value is rejected with 422.
- **FR-ADMIN-PURGE-3** — A purge **soft-deletes**: it stamps `deleted_at = now()` and
  `purge_operation_id = <op id>` on every affected row across every affected table and service.
  It performs no hard deletion. Rows already soft-deleted by ordinary product flows
  (`purge_operation_id IS NULL`) are left untouched by the stamping and are not restorable via
  this mechanism.
- **FR-ADMIN-PURGE-4** — `scope: fleet` cascades explicitly, in child-to-parent order, to every row
  belonging to that fleet: vehicles and everything under them (mileage records, fuel logs,
  maintenance records and their documents, maintenance schedules, vehicle media), plus memberships,
  invites, activity events, dashboards, and dashboard widgets, and finally the fleet row itself.
  Because the database has no foreign keys, each table is enumerated by hand; the cascade must be
  covered by a test that asserts **no orphan rows remain** for the purged fleet.
- **FR-ADMIN-PURGE-5** — `scope: record` with `target_type: vehicle` cascades to that vehicle's
  children using the same enumeration.
- **FR-ADMIN-PURGE-6** — `scope: system` applies the fleet cascade to every fleet, and additionally
  covers all `media.*` and `notification.*` rows. It never touches `auth.*` or
  `fleet.maintenance_categories`.
- **FR-ADMIN-PURGE-7** — `scope: fleet` and `scope: system` require a `confirmation` attribute in
  the request body. For a fleet purge it must exactly equal the fleet's name; for a system purge it
  must equal the literal `PURGE EVERYTHING`. A mismatch returns 409 and performs no writes.
  `scope: record` requires no confirmation string.
- **FR-ADMIN-PURGE-8** — The fleet-service portion of a purge executes inside a single database
  transaction. If any statement fails, nothing is stamped and the operation is not created.
- **FR-ADMIN-PURGE-9** — Downstream services (`media`, `notification`) are stamped after the local
  transaction commits, by calling their internal admin endpoints with the operation id. A
  downstream failure leaves the operation in status `partial` with the failed services named; it
  does **not** roll back the local stamp. The console surfaces `partial` prominently and offers a
  retry that is safe to run repeatedly.
- **FR-ADMIN-PURGE-10** — Every downstream stamp endpoint is **idempotent** on
  `purge_operation_id`: re-invoking it for an operation that already stamped those rows is a no-op
  returning 200. This is what makes FR-ADMIN-PURGE-9's retry safe.
- **FR-ADMIN-PURGE-11** — `purge_after` is `now() + recovery window`, where the window is
  configurable (`ADMIN_PURGE_RECOVERY_WINDOW`) and defaults to **5 days**, matching the existing
  vehicle recovery window in `apps/fleet-service/internal/vehicle/purge.go:9`.
- **FR-ADMIN-PURGE-12** — `GET /admin/purge-operations` lists operations newest-first with scope,
  target, requesting admin, timestamps, status, and affected row counts per table. Supports
  `?status=` filtering.
- **FR-ADMIN-PURGE-13** — `GET /admin/purge-operations/{id}` returns one operation with its full
  per-table affected-row breakdown.

### 4.6 Restore and reaping

- **FR-ADMIN-RESTORE-1** — `DELETE /admin/purge-operations/{id}` cancels a pending operation: it
  clears `deleted_at` and `purge_operation_id` on every row stamped with that operation id, across
  every service, and sets status to `cancelled`. The data is fully visible to ordinary users again.
- **FR-ADMIN-RESTORE-2** — Cancelling an operation whose status is `reaped` returns 409. Reaping is
  irreversible and the API must say so rather than pretending to succeed.
- **FR-ADMIN-RESTORE-3** — Restore only clears rows carrying the matching `purge_operation_id`.
  A row that was already soft-deleted by an ordinary user action before the purge stays deleted;
  cancelling a purge must never resurrect data the product had legitimately removed.
- **FR-ADMIN-RESTORE-4** — A new background sweep, running under `database.WithLeaderLock` on a
  distinct lock name and on the existing 24-hour `jobs.Every` cadence, finds operations whose
  status is `pending` or `partial` and whose `purge_after` has passed, hard-deletes every row
  carrying that operation id, instructs downstream services to do the same, and marks the operation
  `reaped`.
- **FR-ADMIN-RESTORE-5** — The reaper deletes MinIO objects for reaped media rows via the existing
  `storage.Client.RemoveObject` (`apps/media-service/internal/storage/minio.go:207`). An object
  that is already absent from the bucket is not an error.
- **FR-ADMIN-RESTORE-6** — The reaper is resumable: a crash partway through leaves the operation
  `pending`/`partial` and the next tick re-runs it. Hard deletion by operation id is naturally
  idempotent.
- **FR-ADMIN-RESTORE-7** — The existing vehicle purge sweep and the new admin reaper must not
  conflict. The existing sweep continues to handle vehicles soft-deleted through ordinary product
  flows (`purge_operation_id IS NULL`) and must be narrowed to exclude admin-stamped rows, so a
  vehicle inside a cancellable admin operation is never hard-deleted early by the old job.

### 4.7 Data-visibility regression control

This section exists because adding `deleted_at` to ten tables silently changes the meaning of every
existing query over them. It is the highest-risk requirement group in this task.

- **FR-ADMIN-DATA-1** — Every table gaining a `deleted_at` column must have **every existing read
  path** updated to filter `deleted_at IS NULL`. The affected domains are: `mileage`,
  `maintenanceschedule`, `membership`, `invite`, `activity`, `dashboard`,
  `maintenancerecord` (documents only), plus `mediavariant`, `notification`, and `preferences`.
- **FR-ADMIN-DATA-2** — Aggregation SQL must be updated alongside providers. `dashboard/aggregate.go`
  already filters `deleted_at` for vehicles, maintenance records, and fuel logs but will now also
  need it for the newly-soft-deletable tables it touches.
- **FR-ADMIN-DATA-3** — Each affected domain gains a regression test asserting that a soft-deleted
  row is absent from that domain's list and get paths, and absent from any count it feeds.
- **FR-ADMIN-DATA-4** — Uniqueness semantics must be preserved. Where a unique index exists on a
  table gaining soft-delete (notably `fleet.fleet_memberships` on fleet+user, and
  `fleet.fleet_invites` on its token), the index must become a partial index predicated on
  `deleted_at IS NULL`, so that purging a membership does not permanently block that user from
  rejoining the fleet.
- **FR-ADMIN-DATA-5** — Soft-deleted rows must not emit domain events or produce notifications. The
  purge path performs no event emission of its own beyond the audit record.

### 4.8 Audit log

- **FR-ADMIN-AUDIT-1** — Every state-changing admin action (purge created, purge cancelled, purge
  reaped) writes a row to `fleet.admin_audit_events` recording actor user id, actor email, action,
  scope, target type and id, affected row counts, correlation id, and timestamp.
- **FR-ADMIN-AUDIT-2** — Audit rows are **append-only**. There is no API to modify or delete them,
  and they carry no `deleted_at` — a system purge does not erase its own audit trail.
- **FR-ADMIN-AUDIT-3** — `GET /admin/audit-events` returns the log newest-first, paginated, with
  `?action=` and `?actor=` filters.
- **FR-ADMIN-AUDIT-4** — Every admin action is also logged at `Info` through the structured logger
  with the correlation id, so the trail exists in logs independent of the database.

### 4.9 Web console

- **FR-ADMIN-UI-1** — `AuthContext` exposes `platformAdmin` from `/auth/me`.
- **FR-ADMIN-UI-2** — An "Admin" nav entry appears in `AppLayout` **only** when `platformAdmin` is
  true. Its absence is a convenience, not a control; the server remains authoritative.
- **FR-ADMIN-UI-3** — Admin routes are guarded by a `RequirePlatformAdmin` route wrapper that
  redirects non-admins to `/`. It must also bypass the existing fleetless-user redirect in
  `RequireAuth` (`apps/web/src/components/RequireAuth.tsx:29`), which today sends any user without
  an `activeFleetId` to `/onboarding` — otherwise the console becomes unreachable in exactly the
  situation it is most needed, immediately after a system purge.
- **FR-ADMIN-UI-4** — `/admin` shows the aggregate stat tiles from `GET /admin/stats`, with any
  `warnings` rendered as a non-blocking banner naming the unreachable service.
- **FR-ADMIN-UI-5** — `/admin/fleets` shows the searchable, paginated fleet list; each row links to
  detail.
- **FR-ADMIN-UI-6** — `/admin/fleets/:id` shows fleet detail with members, vehicles, and record
  counts, plus per-row delete actions and a "Purge this fleet" action.
- **FR-ADMIN-UI-7** — Fleet and system purges open a confirmation dialog that requires typing the
  exact confirmation phrase; the confirm button stays disabled until the typed value matches. The
  dialog states plainly what will be deleted, that it is recoverable for N days, and when it will
  become permanent.
- **FR-ADMIN-UI-8** — `/admin/purges` lists purge operations with status, countdown to
  `purge_after`, and a cancel action for pending ones. `partial` operations show which services
  failed and offer retry.
- **FR-ADMIN-UI-9** — `/admin/audit` shows the audit log.
- **FR-ADMIN-UI-10** — After a successful system purge the client clears its React Query cache and
  refetches `/auth/me`, so the UI reflects the now-fleetless state rather than rendering stale data.
- **FR-ADMIN-UI-11** — All admin mutations invalidate the relevant React Query keys on success, and
  surface API error detail in the toast, consistent with existing error handling on this branch.

## 5. API Surface

All public admin endpoints are served by `fleet-service` under `/admin`, require a valid JWT, and
return 403 when `platform_admin` is false.

### 5.1 Statistics

```
GET /admin/stats
200 {
  "data": {
    "type": "admin-stats",
    "id": "current",
    "attributes": {
      "fleets": 12,
      "vehicles": { "active": 47, "pending_purge": 3 },
      "memberships": 19,
      "pending_invites": 2,
      "maintenance_records": 431,
      "maintenance_schedules": 88,
      "fuel_logs": 1204,
      "mileage_records": 1893,
      "activity_events": 3310,
      "users": 21,
      "media_objects": 260,
      "notifications": 74,
      "warnings": []
    }
  }
}
```

`warnings` carries entries like `"notification-service unreachable; notifications count omitted"`,
with the corresponding attribute set to `null` (FR-ADMIN-STATS-5).

### 5.2 Fleets and users

```
GET /admin/fleets?page=1&size=25&q=smith&include_deleted=false
GET /admin/fleets/{fleetId}
GET /admin/users?page=1&size=25
```

`GET /admin/fleets/{fleetId}` returns the fleet as primary data with `members`, `vehicles`, and
`pending_invites` as included resources, and a `counts` attribute object.

### 5.3 Purge operations

```
POST /admin/purge-operations
{
  "data": {
    "type": "purge-operations",
    "attributes": {
      "scope": "fleet",
      "target_type": "fleet",
      "target_id": "3f2a…",
      "confirmation": "The Tumidanski Fleet"
    }
  }
}

201 {
  "data": {
    "type": "purge-operations",
    "id": "9c1b…",
    "attributes": {
      "scope": "fleet",
      "target_type": "fleet",
      "target_id": "3f2a…",
      "target_label": "The Tumidanski Fleet",
      "status": "pending",
      "requested_by_user_id": "…",
      "requested_by_email": "jtumidanski@gmail.com",
      "requested_at": "2026-08-02T14:03:11Z",
      "purge_after": "2026-08-07T14:03:11Z",
      "affected": {
        "fleets": 1, "vehicles": 4, "mileage_records": 210,
        "fuel_logs": 130, "maintenance_records": 44,
        "maintenance_schedules": 9, "vehicle_media": 12,
        "memberships": 3, "invites": 1, "activity_events": 380,
        "dashboards": 3, "dashboard_widgets": 14,
        "media_objects": 12, "notifications": 6
      },
      "failed_services": []
    }
  }
}
```

A system purge omits `target_type` / `target_id` and sends `"confirmation": "PURGE EVERYTHING"`.

```
GET    /admin/purge-operations?status=pending
GET    /admin/purge-operations/{id}
DELETE /admin/purge-operations/{id}     → cancel + restore
POST   /admin/purge-operations/{id}/retry   → re-attempt failed downstream stamps
```

### 5.4 Audit

```
GET /admin/audit-events?page=1&size=50&action=purge.created&actor=<userId>
```

### 5.5 Error cases

| Condition | Status | Notes |
|---|---|---|
| No/invalid JWT | 401 | Existing middleware |
| Valid JWT, `platform_admin` false | 403 | `RequirePlatformAdmin` |
| Admin revoked since token mint, purge attempted | 403 | Re-verified against `auth.platform_admins` (FR-ADMIN-AUTH-7) |
| Unknown fleet / operation id | 404 | |
| `confirmation` missing or mismatched | 409 | No writes performed |
| Cancel an already-reaped operation | 409 | Irreversible |
| Unsupported `scope` or `target_type` | 422 | |
| Downstream service failed during stamp | 201 | Operation status `partial`, `failed_services` populated |

### 5.6 Internal service-to-service endpoints

Registered on each service's existing internal route tree; never exposed through the gateway.

```
auth-service
  GET    /internal/admin/stats                     → { users: N }
  GET    /internal/admin/users?ids=a,b,c           → id → {email, displayName}
  GET    /internal/admin/platform-admins/{userId}  → 200 | 404  (stale-claim re-verification)

media-service
  GET    /internal/admin/stats                     → { mediaObjects: N }
  POST   /internal/admin/purge                     → stamp by fleet/vehicle/media ids + opId
  DELETE /internal/admin/purge/{opId}              → restore
  POST   /internal/admin/reap/{opId}               → hard delete rows + MinIO objects

notification-service
  GET    /internal/admin/stats                     → { notifications: N }
  POST   /internal/admin/purge                     → stamp by fleet/user ids + opId
  DELETE /internal/admin/purge/{opId}              → restore
  POST   /internal/admin/reap/{opId}               → hard delete
```

## 6. Data Model

### 6.1 New — `auth.platform_admins` (auth-service)

| Column | Type | Notes |
|---|---|---|
| `user_id` | uuid | primary key, references `auth.users.id` logically |
| `granted_by` | text | user id or `"bootstrap"` for the seeded row |
| `granted_at` | timestamptz | not null |

### 6.2 New — `fleet.purge_operations` (fleet-service)

| Column | Type | Notes |
|---|---|---|
| `id` | uuid | primary key |
| `scope` | text | `system \| fleet \| record`, not null |
| `target_type` | text | null for system scope |
| `target_id` | uuid | null for system scope |
| `target_label` | text | denormalised name captured at request time, so the log stays readable after the target is gone |
| `status` | text | `pending \| partial \| cancelled \| reaped`, not null, indexed |
| `requested_by_user_id` | uuid | not null |
| `requested_by_email` | text | not null |
| `requested_at` | timestamptz | not null |
| `purge_after` | timestamptz | not null, indexed |
| `reaped_at` | timestamptz | nullable |
| `cancelled_at` | timestamptz | nullable |
| `affected_counts` | jsonb | per-table counts captured at stamp time |
| `failed_services` | jsonb | array of service names for `partial` |

### 6.3 New — `fleet.admin_audit_events` (fleet-service)

| Column | Type | Notes |
|---|---|---|
| `id` | uuid | primary key |
| `actor_user_id` | uuid | not null |
| `actor_email` | text | not null |
| `action` | text | `purge.created \| purge.cancelled \| purge.reaped \| purge.retried` |
| `scope` | text | |
| `target_type` | text | nullable |
| `target_id` | uuid | nullable |
| `target_label` | text | nullable |
| `purge_operation_id` | uuid | nullable, indexed |
| `affected_counts` | jsonb | |
| `correlation_id` | text | |
| `created_at` | timestamptz | not null, indexed |

Append-only; no `deleted_at`.

### 6.4 Modified — soft-delete columns

Every table below gains `purge_operation_id uuid` (nullable, indexed). Tables marked **add** also
gain `deleted_at timestamptz` (nullable, indexed).

| Table | Service | `deleted_at` | Action |
|---|---|---|---|
| `fleet.fleets` | fleet | exists (`gorm.DeletedAt`) | add `purge_operation_id` |
| `fleet.vehicles` | fleet | exists (+ `purge_after`) | add `purge_operation_id` |
| `fleet.fuel_logs` | fleet | exists | add `purge_operation_id` |
| `fleet.maintenance_records` | fleet | exists | add `purge_operation_id` |
| `fleet.vehicle_media` | fleet | exists | add `purge_operation_id` |
| `fleet.mileage_records` | fleet | **add** | + `purge_operation_id` |
| `fleet.maintenance_schedules` | fleet | **add** | + `purge_operation_id` |
| `fleet.maintenance_record_documents` | fleet | **add** | + `purge_operation_id` |
| `fleet.fleet_memberships` | fleet | **add** | + partial unique index (FR-ADMIN-DATA-4) |
| `fleet.fleet_invites` | fleet | **add** | + partial unique index (FR-ADMIN-DATA-4) |
| `fleet.activity_events` | fleet | **add** | + `purge_operation_id` |
| `fleet.dashboards` | fleet | **add** | + `purge_operation_id` |
| `fleet.dashboard_widgets` | fleet | **add** | + `purge_operation_id` |
| `media.media_objects` | media | exists | add `purge_operation_id` |
| `media.media_variants` | media | **add** | + `purge_operation_id` |
| `notification.notifications` | notification | **add** | + `purge_operation_id` |
| `notification.notification_preferences` | notification | **add** | + `purge_operation_id` |

Ten tables gain `deleted_at`. Explicitly **excluded** from soft-delete and from all purge scopes:
`fleet.maintenance_categories`, `auth.users`, `auth.refresh_tokens`, `auth.platform_admins`,
`fleet.admin_audit_events`, `fleet.purge_operations`.

All columns are added via GORM `AutoMigrate`, consistent with every existing migration in this
codebase. Nullable columns with no `NOT NULL` constraint backfill cleanly across existing rows.

### 6.5 Ownership resolution

`media.*` and `notification.*` rows have no `fleet_id`. A fleet-scoped purge must therefore pass
explicit id sets to those services: media is reached via the vehicle and maintenance-record ids
that reference it, notifications via the fleet id and its member user ids. The design phase must
confirm each service can resolve its rows from those inputs; see §9.

## 7. Service Impact

**`auth-service`**
- New `platformadmin` domain package (entity, model, provider, migration, seed).
- `session.Processor.MintAccess` gains the `platform_admin` claim; both the login and refresh mint
  paths must supply it.
- `user.Processor.ProvisionFromGoogle` seeds the admin row for bootstrap emails.
- `/auth/me` response gains `platform_admin`.
- New internal routes: stats, user lookup, platform-admin re-verification.

**`packages/shared-go`**
- `auth.Identity` gains `PlatformAdmin bool`; JWT middleware parses the claim.

**`fleet-service`**
- New `admin` domain package: stats, fleet browsing, purge operations, audit, and the HTTP clients
  for the other three services.
- New `/admin` route group wired with `RequirePlatformAdmin`.
- `authz` package gains `RequirePlatformAdmin`.
- New reaper job registered alongside the three existing `jobs.Every` sweeps in `cmd/main.go`.
- Existing vehicle purge sweep narrowed to `purge_operation_id IS NULL` (FR-ADMIN-RESTORE-7).
- Eight entities gain soft-delete columns; their providers and the dashboard aggregation SQL gain
  `deleted_at IS NULL` filters.

**`media-service`**
- `media_variants` gains soft-delete; both entities gain `purge_operation_id`.
- New internal admin routes: stats, stamp, restore, reap (including MinIO object removal).

**`notification-service`**
- Both entities gain soft-delete and `purge_operation_id`; providers gain filters.
- New internal admin routes: stats, stamp, restore, reap.

**`web`**
- `AuthContext` exposes `platformAdmin`; `RequireAuth` gains an admin-route exemption.
- New `RequirePlatformAdmin` route wrapper, admin nav entry, and five pages: overview, fleets,
  fleet detail, purges, audit.
- New admin API service module and React Query hooks.

**`deploy/k8s`**
- `PLATFORM_ADMIN_BOOTSTRAP_EMAILS` and `ADMIN_PURGE_RECOVERY_WINDOW` added to the appropriate
  ConfigMaps for both overlays. No new Secrets, no new ClusterRole — the `main` overlay must
  continue to render clean per CLAUDE.md.

## 8. Non-Functional Requirements

**Security**
- Platform-admin authority is checked server-side on every admin request; the hidden nav entry is
  cosmetic only.
- Purge endpoints re-verify admin status against the database, bounding the stale-claim window on
  the only irreversible operations (FR-ADMIN-AUTH-7).
- Internal admin routes are never exposed through the public gateway; this must be verified in the
  rendered manifests, not assumed.
- The admin API deliberately bypasses `RequireSameFleet`. That bypass lives in a separate route
  tree so the ordinary guard is never weakened. An arch test should assert that no handler
  registered outside `/admin` reads `Identity.PlatformAdmin`, and no `/admin` handler calls
  `RequireSameFleet`.
- Logs must never contain raw tokens. Actor email in audit rows is intentional and required.

**Performance**
- `/admin/stats` must return within 2s for the current data scale. Counts are simple indexed
  aggregates; downstream calls are issued concurrently, not serially.
- A purge is an `UPDATE ... SET deleted_at`, so even a system purge completes in a single
  short transaction. The expensive hard-delete work happens off the request path in the reaper —
  this is the principal reason soft-delete was chosen over immediate cascade deletion.
- The fleet list must paginate; it must never load every fleet's vehicles to render counts.

**Observability**
- Counter metrics for purge operations created, cancelled, and reaped, labelled by scope.
- The reaper logs a summary per run: operations reaped, rows deleted, MinIO objects removed.
- Purge failures log at `Warn` with correlation id and the failing service.

**Testing**
- Unit tests for `RequirePlatformAdmin`, the claim round-trip through JWT middleware, confirmation
  matching, and `purge_after` computation.
- A cascade test per purge scope asserting no orphan rows remain (FR-ADMIN-PURGE-4).
- A restore test asserting round-trip fidelity: soft-delete then cancel returns exactly the
  original visible row set, and does **not** resurrect independently-deleted rows
  (FR-ADMIN-RESTORE-3).
- Regression tests per FR-ADMIN-DATA-3 for all ten newly-soft-deletable tables.
- Frontend tests for admin-gated nav rendering, the confirmation dialog's disabled-until-match
  behaviour, and the fleetless-admin routing exemption.

## 9. Open Questions

1. **Media ownership resolution.** `media.*` rows carry no fleet id. The proposal in §6.5 is for
   `fleet-service` to pass explicit vehicle and maintenance-record id sets. The design phase must
   confirm `media-service` can resolve its rows from those inputs, or else add an owner-scope column
   to `media.media_objects`. This is the least-settled part of the cascade.
2. **Notification ownership.** Same question. Notifications appear to be user-scoped rather than
   fleet-scoped; if a user belongs to two fleets, purging one fleet must not delete notifications
   belonging to the other. The resolution rule needs to be pinned down before implementation.
3. **Partial-unique-index migration.** GORM `AutoMigrate` cannot express a partial unique index. The
   existing unique constraints on memberships and invites will need a hand-written migration
   statement (FR-ADMIN-DATA-4). Worth confirming whether this codebase has any precedent for raw
   DDL outside `AutoMigrate`.
4. **Should the console show soft-deleted data by default?** Currently specified as opt-in via
   `?include_deleted=true`. An operator mid-recovery might reasonably want the opposite default.
5. **Reaper cadence.** Specified as 24 hours to match the existing vehicle sweep, meaning actual
   deletion happens up to 24 hours after `purge_after`. Acceptable, but worth an explicit decision.
6. **Do we need a "purge everything including users" tier later?** Explicitly out of scope now; the
   `scope` enum leaves room for it.

## 10. Acceptance Criteria

Authorization
- [ ] `auth.platform_admins` exists, is seeded with `jtumidanski@gmail.com`, and seeding is
      idempotent across restarts and applies at first login if the user did not previously exist.
- [ ] Access tokens carry `platform_admin`; the claim survives refresh rotation.
- [ ] `auth.Identity.PlatformAdmin` is populated by the shared JWT middleware; absent claim → false.
- [ ] Every `/admin` endpoint returns 403 for a non-admin and 401 for an anonymous caller.
- [ ] Deleting the `auth.platform_admins` row causes purge endpoints to 403 within the same access
      token's lifetime.
- [ ] An admin with no fleet can reach every admin endpoint and every admin page.

Read surface
- [ ] `GET /admin/stats` returns all twelve counts, excluding soft-deleted rows, with vehicles split
      active vs pending-purge.
- [ ] Stopping `notification-service` still yields a 200 from `/admin/stats` with a populated
      `warnings` array and a null notifications count.
- [ ] `GET /admin/fleets` lists fleets the caller is not a member of, paginated and searchable.
- [ ] `GET /admin/fleets/{id}` returns members, vehicles, invites, and per-domain counts.

Purge
- [ ] A `record`-scope purge soft-deletes exactly one record (plus children for a vehicle) and it
      disappears from the ordinary product UI immediately.
- [ ] A `fleet`-scope purge with a correct confirmation soft-deletes the fleet and every descendant
      row across all three services, leaving **zero orphans**, verified by test.
- [ ] A `fleet`-scope purge with a wrong confirmation returns 409 and writes nothing.
- [ ] A `system`-scope purge clears all domain data while `auth.users`, `auth.refresh_tokens`, and
      `fleet.maintenance_categories` remain intact, and the admin remains logged in and is routed
      to `/onboarding`.
- [ ] A downstream service being down yields status `partial` with `failed_services` populated, and
      the retry endpoint completes it without double-stamping.

Restore and reap
- [ ] Cancelling a pending operation restores every row it stamped, across all services.
- [ ] Cancelling does not resurrect rows that were soft-deleted independently beforehand.
- [ ] Cancelling a reaped operation returns 409.
- [ ] The reaper hard-deletes rows past `purge_after`, removes the corresponding MinIO objects, and
      marks the operation `reaped`.
- [ ] The pre-existing vehicle purge sweep no longer hard-deletes vehicles that belong to a pending
      admin operation.

Data integrity
- [ ] All ten tables gaining `deleted_at` filter it in every list, get, and count path, with a
      regression test each.
- [ ] Purging and then restoring a membership leaves the user able to rejoin the fleet (partial
      unique index works).
- [ ] Audit rows are written for create, cancel, retry, and reap, and survive a system purge.

Console
- [ ] The Admin nav entry is invisible to non-admins and the routes redirect them to `/`.
- [ ] Fleet and system purge dialogs keep the confirm button disabled until the typed phrase matches
      exactly.
- [ ] `/admin/purges` shows a countdown to permanence and a working cancel action.

Build and deploy
- [ ] `make ci` passes.
- [ ] Both overlays render, and `kubectl apply --dry-run=server` passes for both.
- [ ] The `main` overlay still renders with no PVCs, no Secrets, no ClusterRole, and no placeholders.

## 11. Pre-existing Defect Discovered

While scoping the cascade, one existing bug surfaced and should be fixed as part of this task
because the fix is a strict subset of work already required.

`vehicle.PurgeExpired` (`apps/fleet-service/internal/vehicle/purge.go:22`) executes:

```sql
DELETE FROM fleet.vehicles WHERE purge_after IS NOT NULL AND purge_after < now()
```

There are **no foreign keys** on any fleet table — every entity uses GORM `AutoMigrate` with plain
indexed `uuid` columns and no `constraint:` tags — so nothing cascades. Every mileage record, fuel
log, maintenance record, maintenance schedule, and vehicle-media row belonging to a purged vehicle
is left orphaned in the database indefinitely, referencing a `vehicle_id` that no longer exists.
These rows are invisible to the product (all reads are vehicle-scoped) but they accumulate forever
and will be counted by `/admin/stats` unless handled.

This task must (a) extend `PurgeExpired` to delete the same child set the admin cascade enumerates,
and (b) provide a one-time cleanup of already-orphaned rows. Both share the cascade enumeration
that FR-ADMIN-PURGE-4 requires anyway.
