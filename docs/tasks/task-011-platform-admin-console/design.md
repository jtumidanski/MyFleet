# Platform Admin Console — Design

Version: v1
Status: Approved for planning
Created: 2026-08-02
Companion to: `prd.md`, `risks.md`, `ui-directions.html`
---

## 1. What this document decides

The PRD left six open questions and named R4 ("media and notification rows have no fleet id")
as "the single most important thing to settle at `/design-task`". All six are resolved below
against source, and R4 turns out to rest on a false premise: **both tables already carry a
fleet id.**

Beyond that, this design makes one structural choice that everything else hangs off — a
**declarative purge manifest** that is the single source of truth for counting, stamping,
restoring, and reaping. That choice is what makes FR-ADMIN-UI-9 ("the blast-radius counts must
match the purge exactly") true by construction rather than by discipline, and it converts
FR-ADMIN-PURGE-4's "enumerate every table by hand" from a checklist into a data structure with a
compile-time completeness test.

The design also records four defects and constraints the PRD did not have, three of which change
what must be built (§4).

---

## 2. Open questions resolved

### OQ-1 — Media ownership. Resolved: media is already fleet-scoped.

`media.media_objects` carries `FleetID string \`gorm:"type:uuid;not null;index"\``
(`apps/media-service/internal/mediaobject/entity.go:13`). It is NOT NULL and indexed, and the
existing internal endpoint already filters on it
(`ListActiveByFleetAndIDs`, `apps/media-service/internal/mediaobject/provider.go:47`).

**Decision.** A fleet-scoped media purge is `WHERE fleet_id = ?`. `media.media_variants`
resolves through `media_object_id IN (SELECT id FROM media.media_objects WHERE fleet_id = ?)`.
No id-set passing is required for fleet or system scope, and **PRD §6.5 is superseded**.

Explicit id sets survive in exactly one place: a `record`-scope purge of a single vehicle, where
fleet-service must tell media-service which media objects belong to that vehicle. Those ids come
from `fleet.vehicle_media.media_id` plus `fleet.maintenance_record_documents.media_id` for the
vehicle's records. This is the only cross-service call that carries ids rather than a fleet id.

A cross-fleet leak is not reachable: media-service stamps `fleet_id` at upload time and
`ValidateOwnership` (`apps/fleet-service/internal/mediaclient/client.go:66`) already refuses to
attach a media object from another fleet, so every id fleet-service can produce for a vehicle is
in that vehicle's fleet.

### OQ-2 — Notification ownership. Resolved: notifications are fleet-scoped; preferences are not.

`notification.notifications` carries `FleetID string \`gorm:"type:uuid"\``
(`apps/notification-service/internal/notification/entity.go:20`), populated from the event's
fleet id on both generation paths (`consumer/consume.go:117`, `reminder/job.go:98`).

**Decision.** A fleet purge deletes notifications with `WHERE fleet_id = ?`. It **never** keys on
`user_id`. R4's cross-tenant data-loss trap — "a user in two fleets loses both streams" — is
avoided structurally: `user_id` appears in no purge predicate at any scope.

The column is nullable and currently unindexed; `Builder.SetFleetID` is optional and the doc
comment notes "system/account-level notifications may" omit it
(`notification/builder.go:13`). Rows with an empty `fleet_id` are account-level: they survive a
fleet purge and are taken only by a system purge. Add an index on `fleet_id` alongside the new
columns.

`notification.notification_preferences` has **no fleet linkage at all** — it is keyed
`(user_id, type)` (`preferences/entity.go:12-14`). It is therefore **excluded from fleet scope**
and included only in system scope, where the PRD's blast radius says "all of `notification.*`".
This is safe because preferences are regenerated with defaults on next read; nothing a fleet
purge should reach lives there.

### OQ-3 — Partial unique index migration. Resolved: no new pattern is needed.

`database.Migration` is `func(db *gorm.DB) error` (`packages/shared-go/database/database.go:11`).
Nothing constrains it to `AutoMigrate`. Raw SQL is already used in production paths
(`vehicle/purge.go:23`, `dashboard/aggregate.go:99`, `database/lock.go:18`), so a migration that
runs `AutoMigrate` and then hand-written DDL is idiomatic here, not novel.

**Decision.** Each affected package's `Migration` becomes:

```go
func Migration(db *gorm.DB) error {
    if err := db.AutoMigrate(&Entity{}); err != nil {
        return err
    }
    // Partial unique index: the constraint must ignore purged rows, or a purge
    // becomes a permanent lockout (risks.md R2).
    if err := db.Exec(`DROP INDEX IF EXISTS fleet.idx_fleet_user`).Error; err != nil {
        return err
    }
    return db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS ux_membership_fleet_user
                    ON fleet.fleet_memberships (fleet_id, user_id)
                    WHERE deleted_at IS NULL`).Error
}
```

**The non-obvious half:** the GORM struct tag must change from `uniqueIndex:idx_fleet_user` to
plain `index` in the same commit. If the tag stays, `AutoMigrate` re-creates the total unique
index on every boot and the partial index becomes decoration. The tag change and the DDL ship
together or neither ships.

### OQ-4 — Default visibility of soft-deleted data. Resolved: default to showing them.

The PRD contradicts itself: FR-ADMIN-FLEET-2 says the fleet list "default excludes soft-deleted
fleets", while FR-ADMIN-UI-7 says "fleets pending purge appear struck through with a countdown
chip rather than vanishing from the list". The second is right — a console whose recovery window
is invisible by default is a console that hides the thing it exists to let you undo.

**Decision.** Replace `?include_deleted=` with a tri-state `?deleted=include|exclude|only`,
default `include`. "Deleted" here means **admin-stamped only** (`purge_operation_id IS NOT NULL`):
fleets removed through ordinary product flows stay hidden, because they are not recoverable
through this console and showing them would imply otherwise. `?deleted=only` is the "what is
pending" view; `?deleted=exclude` is the "show me the live platform" view.

### OQ-5 — Reaper cadence. Resolved: hourly, not daily.

Three reasons the PRD's 24 h is wrong here:

1. `jobs.Every` fires its **first** tick at `T+interval`, not at startup
   (`packages/shared-go/jobs/scheduler.go:17`). A 24-hour job in a service that redeploys more often
   than daily never runs at all. The existing vehicle sweep has exactly this latent defect today.
2. The UI shows a countdown to permanence (FR-ADMIN-UI-11). A daily cadence makes that countdown
   wrong by up to 24 hours; hourly makes it wrong by up to an hour, which rounds to correct in the
   units the UI displays.
3. The tick is cheap: `WHERE status IN ('pending','partial') AND purge_after < now()` over an
   indexed column, no-op on almost every tick.

**Decision.** The admin reaper runs on `jobs.Every(ctx, 1*time.Hour, …)` under
`database.WithLeaderLock(db, "admin-purge-reap", …)`. Narrowing the existing vehicle sweep
(FR-ADMIN-RESTORE-7) is already in scope; move it to the same 1-hour cadence while it is being
touched.

### OQ-6 — A "purge including users" tier. Resolved: no design work.

Out of scope. The `scope` column is free text validated against a Go enum, so a fourth value costs
a constant and a manifest entry later. Nothing in this design forecloses it.

---

## 3. The core decision: one purge manifest, four operations

### 3.1 The problem with the PRD's shape

The PRD describes counting, stamping, restoring, and reaping as four separate pieces of work over
"every affected table, enumerated by hand" (FR-ADMIN-PURGE-4). Written that way, a table appears
in four places and can be forgotten in three of them. Two requirements make that unacceptable:

- FR-ADMIN-UI-9 demands the blast-radius counts match the purge's affected rows **exactly**. Two
  hand-written enumerations that must agree numerically will eventually disagree.
- The acceptance criterion is "zero orphans". A missed table is invisible until someone counts
  rows in a database.

### 3.2 The manifest

A single declarative table in `apps/fleet-service/internal/admin/manifest.go`:

```go
// Target is one purgeable table and how to resolve its rows from a purge root.
type Target struct {
    Key   string // the key used in affected_counts JSON, e.g. "mileage_records"
    Table string // "fleet.mileage_records"
    // Where returns the SQL predicate + args selecting this table's rows for a
    // given root. It never filters deleted_at — see §3.4.
    Where func(root Root) (string, []any)
}

// Root is what a purge is rooted at: the whole system, one fleet, or one record.
type Root struct {
    Scope      Scope  // system | fleet | record
    TargetType string // vehicle | fuel_log | … (record scope only)
    TargetID   string
}
```

Four generic operations run over it, and they are the *only* code that writes purge state:

| Operation | SQL |
|---|---|
| `Count(root)` | `SELECT count(*) FROM <table> WHERE <pred> AND deleted_at IS NULL` |
| `Stamp(tx, root, opID)` | `UPDATE <table> SET deleted_at = now(), purge_operation_id = ? WHERE <pred> AND deleted_at IS NULL` |
| `Restore(tx, opID)` | `UPDATE <table> SET deleted_at = NULL, purge_operation_id = NULL WHERE purge_operation_id = ?` |
| `Reap(tx, opID)` | `DELETE FROM <table> WHERE purge_operation_id = ?` |

`Count` and `Stamp` share the same `Where`. The blast-radius panel calls `Count`; the purge calls
`Stamp`. They cannot diverge because there is one predicate. FR-ADMIN-UI-9 is satisfied by
construction.

`Restore` and `Reap` do not use `Where` at all — they key purely on `purge_operation_id`, which is
what makes them scope-independent, order-independent, and idempotent (R3's requirement, and
FR-ADMIN-RESTORE-3's guarantee that a row deleted earlier by a user is never resurrected: such a
row has a NULL `purge_operation_id` and no operation's restore can touch it).

### 3.3 Manifest completeness is a test, not a checklist

`apps/fleet-service/internal/admin/arch_test.go` — following the precedent at
`apps/auth-service/internal/arch/arch_test.go` — parses every `entity.go` under
`apps/fleet-service/internal/**`, extracts each `func (X) TableName() string { return "fleet.…" }`
literal, and asserts every returned table name is either in the manifest or in an explicit
`excludedTables` map with a one-line reason. A new table added anywhere in fleet-service fails
this test until someone decides whether a purge should reach it.

This is a source-level test, so it needs no database and runs in `make test`. The same test exists
in media-service and notification-service for their own manifests.

`excludedTables` documents the deliberate omissions:

| Table | Reason |
|---|---|
| `fleet.maintenance_categories` | Seeded reference data (PRD non-goal) |
| `fleet.purge_operations` | The operation log itself |
| `fleet.admin_audit_events` | Append-only; survives a system purge (FR-ADMIN-AUDIT-2) |
| `outbox` | Transient relay ledger; drained by the outbox relay, not owned by any fleet |
| `media.processed_events`, `notification.processed_events` | Idempotency ledgers. **Deleting these would let a Kafka replay regenerate notifications for data that was just purged.** |

The last row is a finding, not bookkeeping: the PRD's "all of `notification.*`" phrasing, taken
literally, would have made a system purge self-undoing on the next consumer replay.

### 3.4 Stamp order does not matter — and that is a design rule, not luck

The PRD requires the cascade to run "in child-to-parent order" (FR-ADMIN-PURGE-4). That
requirement exists to stop a parent's soft-delete from hiding its children from the resolution
query. This design removes the requirement instead of satisfying it:

> **Rule: a `Where` predicate may reference parent tables, but must never filter `deleted_at` on
> them.**

`mileage_records` for a fleet resolves as:

```sql
vehicle_id IN (SELECT id FROM fleet.vehicles WHERE fleet_id = ?)   -- no deleted_at filter
```

The `AND deleted_at IS NULL` guard belongs to the *target* table only, where it enforces
FR-ADMIN-PURGE-3 (leave rows already deleted by product flows alone). With that split, stamping
vehicles before or after their mileage records produces identical results. The manifest is still
written child-to-parent for readability, but correctness no longer depends on it — which matters
because ordering is the kind of invariant a later edit silently breaks.

A consequence worth stating: a fleet purge stamps live children of a vehicle a user had already
soft-deleted. That is correct — the whole fleet is going.

### 3.5 Alternatives considered

**A. Hand-written cascade functions per scope** (the PRD's literal shape). Rejected: four
enumerations that must agree, and no way to make the blast-radius counts provably equal to the
purge.

**B. Real foreign keys with `ON DELETE CASCADE`.** Genuinely tempting — it would fix §11's orphan
bug permanently and delete most of this design. Rejected: it is a schema-wide migration across
four databases on data that already contains orphans (the FK creation would fail until they are
cleaned), it makes soft-delete and cascade fight each other (an FK cascades on hard delete, which
is exactly the thing the recovery window defers), and it is a far larger blast radius than the
feature justifies. Worth revisiting as its own task; explicitly not this one.

**C. A purge worker that walks the object graph at runtime** rather than a static manifest.
Rejected as more machinery for the same result, with the completeness property (§3.3) lost.

---

## 4. Findings the PRD did not have

These changed the work. Each is verified against source.

### F1 — Two more unique indexes turn soft-delete into a lockout (extends R2)

The PRD's FR-ADMIN-DATA-4 names `fleet_memberships` and `fleet_invites`. Two more exist:

- `notification.notifications.dedupe_key` — `uniqueIndex`
  (`notification/entity.go:19`). Soft-deleting a notification leaves the key occupying the index,
  so **that notification can never be regenerated**. The reminder job's safety-net and event
  redelivery both depend on `ExistsByDedupeKey` (`notification/administrator.go:26`), which counts
  rows without a `deleted_at` filter — so a purged notification would also permanently suppress its
  own replacement.
- `notification.notification_preferences (user_id, type)` — `uniqueIndex:ux_pref_user_type`
  (`preferences/entity.go:12-14`). After a system purge, a user's preferences row cannot be
  re-created.

Both need the same partial-index treatment as memberships and invites, and `ExistsByDedupeKey`
needs the `deleted_at IS NULL` filter.

`fleet.dashboards` is *not* affected despite its doc comment claiming `UNIQUE(fleet_id, user_id)` —
the tags declare only `index` on `FleetID` (`dashboard/entity.go:14-16`). The comment is stale. See
§6.4 for what this design does about it.

### F2 — notification-service's internal routes would be publicly reachable

`notifications-stripprefix` strips the **full** `/api/notifications` prefix
(`deploy/k8s/base/routing/middlewares.yaml`), so a public request to
`/api/notifications/internal/admin/purge` arrives at notification-service as
`/internal/admin/purge`. notification-service has no `/internal/*` routes today, which is why the
`internal-deny` rule in `deploy/k8s/overlays/main/ingressroute.yaml` covers only fleet-service and
media-service.

**This task adds the first internal routes to notification-service, and they are destructive.**
Without a matching deny rule they would be an unauthenticated, internet-reachable
"delete everything for this fleet" endpoint.

**Decision.** A priority-200 `internal-deny` route for notification-service ships in the same
commit as the routes, mirroring the existing two:

```yaml
- match: (Host(…)) && PathRegexp(`(?i)^/+api/+notifications[^/]*/*internal`)
  kind: Rule
  priority: 200
  middlewares: [{ name: internal-deny }]
  services: [{ name: notification-service, port: 8080 }]
```

auth-service is safe by accident — `auth-stripprefix` strips only `/api`, and every auth route
lives under `/auth/…`, so reaching `/internal/…` would require the public path `/api/internal/…`,
which matches no `/api/*` router and falls through to the SPA catch-all. An equivalent deny rule
ships anyway, because "safe by accident" is one prefix change away from "not safe" and the rule
costs six lines.

### F3 — media-service has a second purge sweep that can collide (extends R8)

R8 covers `vehicle.PurgeExpired`. media-service runs its own 24-hour sweep,
`mediaobject.ListPurgeable` → `purge_after IS NOT NULL AND purge_after < now()`
(`apps/media-service/cmd/main.go:109`, `internal/mediaobject/purge.go:24`), which hard-deletes rows
*and their MinIO objects*.

**Decision — the rule that defuses both:** an admin stamp writes `deleted_at` and
`purge_operation_id` and **never writes `purge_after`**. Both legacy sweeps key on `purge_after`,
so they cannot see admin-stamped rows at all. The `purge_operation_id IS NULL` narrowing required
by FR-ADMIN-RESTORE-7 is then belt-and-braces rather than the sole control — and it is applied to
*both* sweeps, not just the vehicle one.

### F4 — the test harness is SQLite with an attached `fleet` schema

Tests run on `sqlite.Open(":memory:")` with `ATTACH DATABASE ':memory:' AS fleet`, and create
schema-qualified tables with **hand-written DDL** because GORM's SQLite driver emits
`CREATE INDEX` with the schema prefix stripped and `AutoMigrate` therefore fails
(`apps/fleet-service/internal/invite/resource_test.go:33-41`,
`apps/fleet-service/internal/fuel/processor_test.go:44-52`).

This matters because the cascade test (FR-ADMIN-PURGE-4) needs **every** purgeable table present
in one database — the largest fixture in the repo by a wide margin, currently duplicated per test
file.

**Decision.** A `apps/fleet-service/internal/admin/admintest` package owns that DDL once and
exposes `admintest.NewDB(t) *gorm.DB`. Every purge, restore, reap, and regression test uses it.
Hand-maintained DDL in N files is how a column gets added to production and forgotten in tests;
one file is checkable. SQLite supports partial indexes, so the partial-unique-index behaviour is
testable there too.

---

## 5. Authorization

### 5.1 The claim path

`session.Principal` gains `PlatformAdmin`. Every existing guarantee then applies for free:

- `newPrincipalResolver` (`apps/auth-service/cmd/main.go:120`) is the **sole** construction site
  for `Principal`, enforced by `TestNoPrincipalLiteralOutsideResolver`
  (`apps/auth-service/internal/arch/arch_test.go:28`). Adding the field there means **both** mint
  paths — initial login and refresh rotation — carry it. FR-ADMIN-AUTH-4 needs no per-path work.
- `TestMintAccess_mapsEveryPrincipalField`
  (`apps/auth-service/internal/session/processor_test.go:252`) fails if the field never reaches a
  claim.

**One catch.** That test asserts every `Principal` field is a `string` and `t.Fatalf`s otherwise:
*"Principal.X is a %s, not a string — extend this test's sentinel scheme"*. A `bool` field trips
it deliberately. The test's sentinel scheme must be extended to handle non-string fields as part
of this work; that is a required task, not an incidental fix.

`auth.Identity` (`packages/shared-go/auth/identity.go:6`) gains `PlatformAdmin bool`, parsed in
the shared JWT middleware with a `boolean` accessor mirroring the existing `str()` helper
(`packages/shared-go/auth/middleware.go:97`): a missing or non-boolean claim yields `false`.

`GET /auth/me` returns it in `meta`, alongside the `activeFleetId` and `role` already there
(`apps/auth-service/internal/user/resource.go`, the `server.Document{Meta: …}` write), sourced
from `Identity` rather than a second database read.

### 5.2 The guard

```go
// RequirePlatformAdmin returns 403, not 404: the existence of an admin API is
// not a secret, only the authority to use it. This is the deliberate inverse of
// RequireSameFleet's non-disclosure rule.
func RequirePlatformAdmin(id auth.Identity) error {
    if !id.PlatformAdmin {
        return server.ErrForbidden
    }
    return nil
}
```

### 5.3 Seeding, and why it needs two hooks

`auth.platform_admins` is keyed by `user_id`, but the bootstrap list is emails, and the bootstrap
user may not exist at first migration. So:

1. **Startup seed** — after migration, for each email in `PLATFORM_ADMIN_BOOTSTRAP_EMAILS`, insert
   a row if a user with that email exists. `FirstOrCreate` on the primary key: idempotent across
   restarts.
2. **Provision-time seed** — `user.Processor` gains an optional hook in the repo's established
   `With…` idiom (cf. `maintenanceschedule.WithOverdueHooks`,
   `NewCompletionDeps().WithActivityRecorder`):

   ```go
   func (pr *Processor) WithBootstrapAdmins(emails map[string]bool, grant func(userID string) error) *Processor
   ```

   `ProvisionFromGoogle` (`apps/auth-service/internal/user/processor.go:32`) calls `grant` when the
   provisioned email is in the set. A nil hook is a no-op, so every existing test compiles and
   passes untouched.

The env var seeds the table and is never consulted per-request (FR-ADMIN-AUTH-3). Parse it with
`strings.Split` + `TrimSpace` + `ToLower`, matching how `KAFKA_BROKERS` is handled
(`apps/fleet-service/cmd/main.go:156`).

### 5.4 Stale-claim re-verification — narrower than the PRD says

FR-ADMIN-AUTH-7 says "purge endpoints re-verify against the database". Applied literally that
includes **cancel**, which is the recovery path. If auth-service is unreachable, a fail-closed
re-verification would block the one action that undoes a mistake, during the window when undoing
it is still possible.

**Decision.**

| Endpoint | Re-verifies | On auth-service failure |
|---|---|---|
| `POST /admin/purge-operations` | yes | **fail closed** — 500, nothing stamped |
| `POST /admin/purge-operations/{id}/retry` | yes | fail closed |
| `DELETE /admin/purge-operations/{id}` (cancel) | **no** | n/a — restore always available |
| all read endpoints | no | n/a |

Failing closed on create mirrors `mediaclient.ValidateOwnership`'s reasoning
(`apps/fleet-service/internal/mediaclient/client.go:56-66`): coupling an irreversible write to a
dependency's availability is the correct trade; coupling a *reversible* one is not.

### 5.5 The R7 arch test

`apps/fleet-service/internal/admin/arch_test.go` additionally asserts, by parsing sources:

- no file outside `internal/admin/` references `Identity.PlatformAdmin` or `RequirePlatformAdmin`;
- no file inside `internal/admin/` references `RequireSameFleet`.

This is the structural control against someone "simplifying" by short-circuiting
`RequireSameFleet` for admins, which would make every ordinary endpoint cross-fleet capable.

---

## 6. Data model

### 6.1 New tables

`auth.platform_admins`, `fleet.purge_operations`, `fleet.admin_audit_events` exactly as PRD §6.1–6.3.
`purge_operations` indexes `status` and `purge_after`; `admin_audit_events` indexes `created_at`
and `purge_operation_id`. `affected_counts` and `failed_services` are `jsonb`.

### 6.2 Column additions

Per PRD §6.4, with these corrections:

| Table | `deleted_at` | Partial unique index needed |
|---|---|---|
| `fleet.fleet_memberships` | add | **yes** — `(fleet_id, user_id)` |
| `fleet.fleet_invites` | add | **yes** — `(token)` |
| `notification.notifications` | add | **yes — new (F1)**, `(dedupe_key)`; also index `fleet_id` |
| `notification.notification_preferences` | add | **yes — new (F1)**, `(user_id, type)` |
| `fleet.dashboards` | add | yes — `(fleet_id, user_id)`, see §6.4 |
| `fleet.mileage_records`, `fleet.maintenance_schedules`, `fleet.maintenance_record_documents`, `fleet.activity_events`, `fleet.dashboard_widgets`, `media.media_variants` | add | no |
| `fleet.fleets`, `fleet.vehicles`, `fleet.fuel_logs`, `fleet.maintenance_records`, `fleet.vehicle_media`, `media.media_objects` | exists | no |

Every table above also gains `purge_operation_id uuid` (nullable, indexed).

`deleted_at` is `*time.Time`, **not** `gorm.DeletedAt`, matching the existing convention on
`vehicles`, `fuel_logs`, `maintenance_records`, `vehicle_media` and `media_objects`. This is
deliberate: `gorm.DeletedAt` enables GORM's implicit soft-delete, which would silently change what
`Delete()` means at every existing call site (e.g. `membership/administrator.go:33`,
`maintenanceschedule/administrator.go:62`, both of which hard-delete today and must keep doing so).
Manual filters are more typing and less surprise.

`fleet.fleets` is the one table already on `gorm.DeletedAt`, so it auto-filters; the admin fleet
list must use `Unscoped()` and filter by hand to honour `?deleted=`.

### 6.3 Uniform stamp shape

Both new columns are written together and nothing else is:

```sql
SET deleted_at = now(), purge_operation_id = $1
```

Never `purge_after` (F3).

### 6.4 Dashboards: making restore safe

`dashboards` has no real unique index (F1), and its save path deletes-and-recreates widgets
(`dashboard/processor.go:144-163`) while reading `First(&e)` with no ordering
(`processor.go:94`). If a fleet purge soft-deletes a dashboard and the user then visits their
dashboard, the current code inserts a **second** row; a later cancel restores the first, leaving
two live rows and a non-deterministic read.

**Decision.** Two changes, both small:

1. The save path looks up including soft-deleted rows for `(fleet_id, user_id)` and, if it finds
   one, **revives** it (clears `deleted_at` and `purge_operation_id`) rather than inserting. A
   revived row simply leaves the pending operation — the user re-created their layout, which is the
   outcome a later cancel would have produced anyway.
2. Add the partial unique index the stale comment already claims, so the one-row-per-(fleet,user)
   invariant is enforced rather than assumed.

Widgets need no revive logic: they are hard-deleted and recreated on every save, so their
`deleted_at` only ever matters between a stamp and its cancel.

---

## 7. API topology

### 7.1 Public admin API — fleet-service, `/admin`

Registered as its own `chi` route group in `apps/fleet-service/cmd/main.go`, a sibling of the
existing authenticated group, with `authmw.JWT(keyfn, …)` and nothing else shared:

```go
AddRouteInitializer(func(r chi.Router) {
    r.Group(func(ar chi.Router) {
        ar.Use(authmw.JWT(keyfn, authmw.WithLogger(log)))
        admin.InitializeRoutes(log, db, deps)(ar)   // every handler calls RequirePlatformAdmin
    })
}).
```

Public paths are gateway-prefixed `/api/fleet/admin/…`. These match the existing priority-100
`PathPrefix('/api/fleet')` router and are stripped to `/admin/…`; the priority-200 `internal-deny`
regex (`^/+api/+fleet[^/]*/*internal`) does not match them. No routing change is required for the
public surface.

Endpoints are as PRD §5, with one correction: pagination is `?page[number]=&page[size]=` via
`server.ParsePage` (`packages/shared-go/server/pagination.go:22`), which already defaults to 25 and
caps at 100 — the PRD's "page/size convention" is not the convention in this codebase.

### 7.2 Internal service-to-service contracts

Registered on each service's existing internal tree (no JWT), per
`membership.InitializeInternalRoutes` (`apps/fleet-service/internal/membership/resource.go:89`).
**Every one ships with its `internal-deny` routing rule (F2).**

```
auth-service
  GET    /internal/admin/stats                     → { "users": 21 }
  GET    /internal/admin/users?ids=a,b,c           → [{ id, email, display_name }]
  GET    /internal/admin/platform-admins/{userId}  → 200 | 404

media-service
  GET    /internal/admin/stats                     → { "media_objects": 260 }
  POST   /internal/admin/purge                     → stamp;  200 { affected: {…} }
  DELETE /internal/admin/purge/{opId}              → restore; 200 { affected: {…} }
  POST   /internal/admin/reap/{opId}               → hard delete rows + MinIO objects

notification-service
  GET    /internal/admin/stats                     → { "notifications": 74 }
  POST   /internal/admin/purge                     → stamp
  DELETE /internal/admin/purge/{opId}              → restore
  POST   /internal/admin/reap/{opId}               → hard delete
```

**Stamp request body** (one shape, both services):

```json
{ "operation_id": "9c1b…", "scope": "system" }
{ "operation_id": "9c1b…", "scope": "fleet",     "fleet_ids": ["3f2a…"] }
{ "operation_id": "9c1b…", "scope": "media_ids", "media_ids": ["…"] }   // media-service only
```

**Idempotency (FR-ADMIN-PURGE-10), stated precisely.** The `UPDATE` guards on
`deleted_at IS NULL`, so a replay updates zero rows. The response counts are then computed
*after* the update as `SELECT count(*) … WHERE purge_operation_id = $1`, so a replay returns the
**same** numbers as the first call rather than zeros. That distinction is what makes
FR-ADMIN-PURGE-9's retry safe to run repeatedly *and* leaves `affected_counts` correct afterwards.

`GET /internal/admin/users` bounds its id list the way media-service already does
(`MaxInternalLookupIDs = 50`, `apps/media-service/internal/mediaobject/resource.go:238`);
fleet-service chunks larger member sets.

### 7.3 Clients

`apps/fleet-service/internal/adminclient/` holds three clients (`auth`, `media`, `notification`),
each modelled on `mediaclient` (`apps/fleet-service/internal/mediaclient/client.go`): explicit
5-second timeout, no `http.DefaultClient`, context-aware, non-200 becomes an error. New config:
`AUTH_INTERNAL_URL`, `NOTIFICATION_INTERNAL_URL` (`MEDIA_INTERNAL_URL` already exists).

### 7.4 `/admin/stats` fan-out

Local counts run as one `Count` pass over the manifest at system root. The three remote counts are
issued **concurrently** with `sync.WaitGroup` and per-source error capture (no new dependency; the
repo has no `errgroup`). A failed source sets its attribute to `null` and appends a string to
`warnings`; the response is still 200 (FR-ADMIN-STATS-5). `vehicles` is reported as
`{active, pending_purge}`, where `pending_purge` is `deleted_at IS NOT NULL AND purge_operation_id
IS NOT NULL` — a vehicle deleted by a user is neither active nor recoverable here, so counting it
as pending would misstate what the console can undo.

---

## 8. Purge lifecycle

### 8.1 States

```
                 downstream ok
  (create) ──────────────────────► pending ──── purge_after elapsed ──► reaped
      │                              │  ▲                                 ▲
      │ downstream failed            │  │ retry ok                        │
      └────────────► partial ────────┘  │                                 │
                        │               │                                 │
                        └── retry ──────┘                                 │
                                                                          │
  pending|partial ── DELETE ──► cancelled          reaped ── DELETE ──► 409
```

`cancelled` and `reaped` are terminal. `DELETE` on `reaped` is 409 (FR-ADMIN-RESTORE-2).

### 8.2 Create

1. `RequirePlatformAdmin`, then re-verify against auth-service (§5.4).
2. Validate `scope` and `target_type` against the enums → 422.
3. Resolve the root; unknown fleet/record id → 404. Capture `target_label` now, while the target
   still has a name.
4. For `fleet`/`system`, compare `confirmation` **server-side** (fleet name, or the literal
   `PURGE EVERYTHING`) → 409 with no writes on mismatch. The disabled button is UI courtesy; this
   is the control (R9).
5. **One transaction** (FR-ADMIN-PURGE-8): insert the operation row, `Stamp` every manifest target,
   write the audit row. Any failure rolls back the whole thing and the operation does not exist.
6. **After commit**, call the downstream stamps. Failures do not roll back the local stamp; they
   set `status = 'partial'` and populate `failed_services`, and the response is still 201
   (PRD §5.5).
7. Log at `Info` with the correlation id (FR-ADMIN-AUDIT-4); increment the per-scope counter.

`purge_after = now() + window`, where the window is
`time.ParseDuration(config.Get("ADMIN_PURGE_RECOVERY_WINDOW", "120h"))` — 120 h is the 5 days the
PRD specifies and matches `vehicle.recoveryWindow` (`apps/fleet-service/internal/vehicle/purge.go:9`).
An unparseable value falls back to 120 h rather than panicking, following the `COOKIE_SECURE`
precedent (`apps/auth-service/cmd/main.go:56`).

### 8.3 Cancel

`Restore` locally in one transaction, then call each downstream restore. Downstream failure here
leaves the operation `partial` with the operation still cancellable — restore is idempotent, so the
correct user action is to press it again. Status becomes `cancelled` only when every service has
restored.

### 8.4 Reap

Hourly (OQ-5), under `WithLeaderLock(db, "admin-purge-reap", …)`. For each operation with
`status IN ('pending','partial') AND purge_after < now()`:

1. Call each downstream `POST /internal/admin/reap/{opId}`. media-service hard-deletes rows and
   removes MinIO objects via `storage.Client.RemoveObject`
   (`apps/media-service/internal/storage/minio.go:207`); an object already absent is not an error.
2. `Reap` locally over the manifest.
3. Mark `reaped`, write the audit row with actor `system`.

Order matters exactly once: downstream **before** local, because the local `Reap` destroys the
`purge_operation_id` values, and a crash between the two must leave enough state to retry. A crash
anywhere leaves the operation `pending`/`partial` and the next tick re-runs it; every step keys on
`purge_operation_id` and is therefore idempotent (FR-ADMIN-RESTORE-6).

The run logs one summary line: operations reaped, rows deleted per table, MinIO objects removed.

### 8.5 The pre-existing defect (PRD §11)

`vehicle.PurgeExpired` (`apps/fleet-service/internal/vehicle/purge.go:22`) hard-deletes vehicles
with no cascade, orphaning every mileage record, fuel log, maintenance record, schedule, and
vehicle-media row. With the manifest in hand the fix is small:

- `PurgeExpired` becomes: select expiring vehicle ids `WHERE purge_after < now() AND
  purge_operation_id IS NULL`, run the manifest's vehicle-root `Reap` predicates against that id
  set, then delete the vehicles — all in one transaction.
- A one-time cleanup at startup, guarded by `WithLeaderLock`, deletes rows in child tables whose
  `vehicle_id` has no matching row in `fleet.vehicles`. It logs the count it removed and is a no-op
  on a clean database.

This must land **before** `/admin/stats` is trusted (R10): until it does, the console's first act
is to report numbers no fleet can reconcile.

---

## 9. The data-visibility sweep (R1)

The riskiest requirement group, and — usefully — a small and fully enumerable one. Every read site
over a newly-soft-deletable table, from a mechanical sweep of `p.db.` / `tx.` call sites:

| Package | Sites needing `deleted_at IS NULL` |
|---|---|
| `mileage` | `provider.go:22` (history + trend + latest-mileage autofill) |
| `maintenanceschedule` | `provider.go:45`, `:55`, `:63`, `queryActive` at `:99`; `administrator.go:132` |
| `membership` | `provider.go:33`, `:44`, `:56`, `:68`, `:79` (owner count) |
| `invite` | `provider.go:25`, `:36`, `:47` |
| `activity` | `provider.go:29`, `:33`, `:55` |
| `maintenancerecord` (documents) | `provider.go:43`, `:83`; `administrator.go:67` |
| `dashboard` | `processor.go:94`, `:103`, `:169`, `:173`; `aggregate.go:144` (raw SQL over `mileage_records`) |
| `notification` | `provider.go:38` (`ListByUser`); `administrator.go:26` (`ExistsByDedupeKey` — see F1), `:54`, `:77` |
| `preferences` | its provider's get/list |
| `mediavariant` | its provider's lookup by `media_object_id` |

Two sites deserve naming because they are where a miss would hurt most and where review attention
naturally does not go:

- **`maintenanceschedule.queryActive` (`provider.go:99`)** already joins `v.deleted_at IS NULL` on
  the vehicle but has no predicate on the schedule itself. Its no-argument form, `ListActive()`,
  backs `/internal/maintenance/due`, which drives the reminder job — so a purged schedule would keep
  **generating notifications** for a fleet that no longer exists.
- **`dashboard/aggregate.go:144`** is raw SQL over `fleet.mileage_records`, bypassing the provider
  layer entirely.

`aggregate.go:66-91` already filters `deleted_at` for vehicles, maintenance records and fuel logs;
those need no change.

**The control is the test, not the table above.** FR-ADMIN-DATA-3 gets one regression test per
affected domain, each asserting a soft-deleted row is absent from that domain's list path, its get
path, and any count it feeds. The table tells the implementer where to look; the tests are what
keep it true.

---

## 10. Web console

Direction C from `ui-directions.html` — dedicated shell, fleet inspector at the centre, purge queue
as a peer section. That file is the visual reference; open it before building any screen.

### 10.1 Route tree

```
/admin                    RequirePlatformAdmin → AdminLayout
  index                   AdminOverviewPage      (stat tiles)
  /admin/fleets           AdminFleetsPage        (two-pane inspector)
  /admin/fleets/:id       AdminFleetsPage        (detail pane / mobile detail view)
  /admin/users            AdminUsersPage
  /admin/purges           AdminPurgesPage
  /admin/audit            AdminAuditPage
```

`RequirePlatformAdmin` requires authentication and `platformAdmin` and redirects others to `/`. It
**does not** require `activeFleetId`, and — the structural point — the `/admin` branch is a sibling
of the `<RequireAuth><AppLayout/></RequireAuth>` branch in `App.tsx`, not nested inside it. The
fleetless redirect at `RequireAuth.tsx:29` therefore never applies, and `RequireAuth` is not
modified. This is R5's resolution: an exemption flag is the kind of thing a later refactor drops
silently; a route-tree position is not.

### 10.2 Context and chrome

`AuthContextValue` gains `platformAdmin: boolean`, read from `doc.meta.platformAdmin` in `fetchMe`
(`apps/web/src/lib/hooks/api/auth.ts:22`) and defaulting to `false`.

`AdminLayout` has its own sidebar (Overview, Fleets, Users, Purges, Audit log, "Back to my fleet")
and a persistent mode band below the header using `danger-subtle` / `danger-subtle-foreground` /
`danger-border` (`apps/web/src/index.css:41-44`, `:86-89`). Not `--destructive` — that token is
reserved for destructive *controls* under the task-003 contract, and the band is a mode indicator,
not a button. The band also states the stale-claim caveat (FR-ADMIN-AUTH-7) in plain words.

`AppLayout`'s `NAV` array (`apps/web/src/components/AppLayout.tsx:8`) gains a conditional "Admin"
entry. Its absence is convenience; the server is authoritative.

### 10.3 New UI primitives

`dialog`, `table`, `badge` in `apps/web/src/components/ui/`, following the nine existing components:
`cva` variants, `cn` from `../../lib/utils`, `React.forwardRef`, theme tokens only, no hard-coded
colours. `dialog` needs `@radix-ui/react-dialog` — a new dependency, consistent with the
`react-label` / `react-select` / `react-switch` / `react-slot` already present. `table` and `badge`
are plain markup and need nothing new.

### 10.4 Services and hooks

`AdminService extends BaseService` in `apps/web/src/services/api/AdminService.ts`, base path
`/api/fleet/admin`. Hooks in `apps/web/src/lib/hooks/api/admin.ts` with a hierarchical key factory
matching the `memberKeys` shape (`lib/hooks/api/members.ts:28`). Mutations invalidate on settle and
surface `createErrorFromUnknown(err).message` in a `toast.error`, matching the pattern established
on `fix/invite-accept-flow`.

`apps/web/src/lib/admin/purgeStatus.ts` is the **single** module mapping API vocabulary to user
language (FR-ADMIN-UI-12): `pending` → "Recoverable", `partial` → a specific failure
("Media not deleted"), `reaped` → "Deleted for good", `cancelled` → "Restored". Nothing else in the
UI reads a raw status string.

### 10.5 Screens

- **Overview** — stat tiles from `/admin/stats`. A `null` count renders as an em dash with the
  reason beneath, never `0`. `warnings` render as a non-blocking banner.
- **Fleets** — two-pane above `md`, single-column with back-navigation below. Fleets pending purge
  render struck through with a countdown chip (they are in the default result set per OQ-4).
- **Fleet detail** — members with role, vehicles with derived status and record counts, pending
  invites, per-domain counts. The owner's remove action is permanently inert. Vehicle status is
  derived server-side using the existing `vehicle.StatusDeps` machinery
  (`apps/fleet-service/cmd/main.go:122`) — affordable for one fleet, which is why the **list** view
  carries counts only and never loads any fleet's vehicles (NFR, §8).
- **Blast-radius panel** — per-domain breakdown from the manifest's `Count`, with "Purge this
  fleet" beneath it. If the counts cannot be computed, the panel shows an error and the control is
  unavailable — never stale or approximate numbers behind a live destructive button.
- **Confirmation dialog** — exact-match typing gate, blast radius stated in people as well as rows,
  recovery deadline as an **absolute date and time**, and for a system purge an explicit list of
  what survives (user accounts, sign-ins, seeded maintenance categories).
- **Purges** — status filters, countdown to `purge_after`, restore, and a retry presented as safe
  to repeat.
- **Audit** — newest-first, reaper rows attributed to "system", correlation id surfaced.

After a successful system purge the client clears the React Query cache and refetches `/auth/me`.
The admin stays in the console (FR-ADMIN-UI-14) — which works only because of §10.1's route-tree
placement, and is the exact scenario the R5 acceptance test must exercise.

---

## 11. Deployment

`deploy/k8s/base/auth-service/configmap.yaml`: `PLATFORM_ADMIN_BOOTSTRAP_EMAILS:
"jtumidanski@gmail.com"`.

`deploy/k8s/base/fleet-service/configmap.yaml`: `ADMIN_PURGE_RECOVERY_WINDOW: "120h"`,
`AUTH_INTERNAL_URL: "http://auth-service:8080"`,
`NOTIFICATION_INTERNAL_URL: "http://notification-service:8080"`.

`deploy/k8s/overlays/main/ingressroute.yaml`: two new priority-200 `internal-deny` routes for
notification-service and auth-service (F2). They go in `myfleet-routes` only — the `replacements`
block copies `spec.routes` verbatim into the TLS twin, so both entrypoints get them and cannot
drift.

No new Secrets, no new ClusterRole, no PVCs: the `main` overlay renders clean per CLAUDE.md. Both
overlays must render **and** pass `kubectl apply --dry-run=server`, including the local overlay —
the missing-`namespace` failure documented in CLAUDE.md slipped through ten reviews because only
the `main` dry-run was ever run.

---

## 12. Testing

**Unit.** `RequirePlatformAdmin`; the `platform_admin` claim round-trip through the shared JWT
middleware including missing and non-boolean claims; confirmation matching; `purge_after`
computation; the purge-status vocabulary map.

**Arch (source-parsing, no DB).** Manifest completeness per service (§3.3). The R7 separation test
(§5.5). The extended `TestMintAccess_mapsEveryPrincipalField` sentinel scheme for non-string fields
(§5.1).

**Cascade.** One test per scope over `admintest.NewDB`, asserting **zero orphan rows** after a
purge: for every child table, no row whose parent id is absent from the live parent set.

**Restore fidelity.** Soft-delete then cancel returns exactly the original visible row set; a row
soft-deleted independently *before* the purge stays deleted (FR-ADMIN-RESTORE-3).

**Idempotency.** Downstream stamp called twice returns identical counts and changes nothing the
second time. Reap called twice succeeds.

**Lockout.** Purge a membership, cancel, rejoin the fleet — the partial unique index works. Same
shape for an invite token and for a notification `dedupe_key` (F1).

**Regression (FR-ADMIN-DATA-3).** One per affected domain in §9: a soft-deleted row is absent from
list, get, and every count it feeds.

**Sweep isolation.** An admin-stamped vehicle is not hard-deleted by `vehicle.PurgeExpired`; an
admin-stamped media object is not hard-deleted by media-service's sweep (F3).

**Frontend.** Admin nav hidden for non-admins; `/admin/*` redirects non-admins to `/`; the
confirmation dialog stays disabled until exact match; **a fleetless admin reaches every admin
screen** — tested against the post-system-purge state, not merely a fleetless account (R5's
residual risk).

**Manifests.** Both overlays render; both server dry-runs pass; `main` has no PVCs, Secrets,
ClusterRole, or placeholders. Probe the new `internal-deny` rules on `:80` and `:443`.

`make ci` is the gate.

---

## 13. Sequencing

The dependency order that matters, for the plan phase to build on:

1. **Orphan cleanup and `PurgeExpired` cascade** (§8.5) — before `/admin/stats` reports anything.
2. **Schema + partial indexes + the DATA-1 sweep and its regression tests** (§6, §9) — before
   anything can be soft-deleted safely.
3. **Auth tier** (§5) — claim, guard, seed, `/auth/me`.
4. **Manifest and the four operations** (§3) — with the completeness arch test.
5. **Purge lifecycle and downstream contracts** (§7.2, §8) — with the `internal-deny` rules in the
   same commit as the routes.
6. **Reaper** (§8.4).
7. **Console** (§10).

Steps 1 and 2 are the ones with no visible payoff and the highest risk of being deferred. They are
the foundation; deferring them means building the console on data that lies.

---

## 14. Deviations from the PRD

Each is deliberate and argued above.

| # | PRD says | This design says | Why |
|---|---|---|---|
| 1 | §6.5 — media/notification rows have no fleet id; pass explicit id sets | Both tables have `fleet_id`; fleet scope is `WHERE fleet_id = ?` | OQ-1, OQ-2 — verified against source |
| 2 | FR-ADMIN-FLEET-2 — list defaults to excluding soft-deleted | `?deleted=` tri-state, default `include` (admin-stamped only) | OQ-4 — resolves the contradiction with FR-ADMIN-UI-7 |
| 3 | FR-ADMIN-RESTORE-4 — reaper on the 24-hour cadence | Hourly | OQ-5 — `jobs.Every`'s first tick is at `T+interval` |
| 4 | FR-ADMIN-PURGE-4 — cascade in child-to-parent order | Order-independent by construction | §3.4 — a stronger property than the ordering rule |
| 5 | FR-ADMIN-AUTH-7 — purge endpoints re-verify | Create and retry re-verify; cancel does not | §5.4 — never block the recovery path |
| 6 | FR-ADMIN-DATA-4 — partial indexes on memberships and invites | Also `notifications.dedupe_key`, `notification_preferences`, `dashboards` | F1, §6.4 |
| 7 | §4.4 — "existing page/size convention" | `?page[number]=` / `?page[size]=` via `server.ParsePage` | That is the actual convention |
| 8 | §6.4 — blast radius is "all of `notification.*`" | `processed_events` and `outbox` excluded everywhere | §3.3 — truncating them lets a replay undo the purge |
| 9 | §7 — deploy changes are ConfigMap keys only | Also two new `internal-deny` routing rules | F2 — the notification internal routes would otherwise be public |

---

## 15. Risk register, re-scored

| Risk | Status after this design |
|---|---|
| R1 — retrofitting `deleted_at` | **Controlled.** §9 enumerates all 25-ish sites; per-domain regression tests are the control. Residual: raw SQL in `aggregate.go` and `queryActive`, both named explicitly. |
| R2 — unique-constraint lockouts | **Controlled and widened.** Two more constraints found (F1). The tag-plus-DDL pairing (OQ-3) is the part most likely to be half-done. |
| R3 — no distributed transaction | **Accepted, bounded.** Idempotency keyed on `purge_operation_id`, `partial` status is explicit, retry is safe. Unchanged in kind. |
| R4 — media/notification have no fleet id | **Dissolved.** Both have one. The cross-tenant trap is avoided by never keying on `user_id`. |
| R5 — console unreachable post-purge | **Designed out structurally.** Residual: a future refactor renesting the routes; the post-purge acceptance test is what catches it. |
| R6 — stale admin claim | **Accepted, narrowed.** Re-verification on create and retry only (§5.4). |
| R7 — same-fleet bypass leaking outward | **Controlled** by the arch test (§5.5). |
| R8 — reaper vs. existing sweeps | **Controlled and widened.** The "never write `purge_after`" rule (F3) defuses both sweeps; the narrowing applies to media's too. |
| R9 — confirmation phrase is the only barrier | **Unchanged.** Server-side matching, 5-day window, absolute deadline in the dialog. |
| R10 — orphans already exist | **Controlled** by sequencing (§13 step 1). |
| **F2 — notification internal routes public** | **New, critical, controlled** by shipping the deny rule with the routes. |
