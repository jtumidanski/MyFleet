# Platform Admin Console Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give the platform operator a `/admin` console that can see every fleet in the system and purge data at record, fleet, or system granularity — recoverably, auditably, and without ever weakening the fleet-scoped guard ordinary users run under.

**Architecture:** A single declarative *purge manifest* in `fleet-service` lists every purgeable table once; four generic operations (`Count`, `Stamp`, `Restore`, `Reap`) run over it, so the blast-radius counts and the purge itself cannot diverge. A purge soft-deletes by stamping `deleted_at` + `purge_operation_id`; an hourly reaper hard-deletes past the recovery window. `fleet-service` orchestrates and calls `auth`, `media`, and `notification` over their network-restricted internal routes. Authorization is a `platform_admin` JWT claim sourced from `auth.platform_admins`, guarded in a route tree that is a structural sibling of the ordinary one — on the server and in the SPA.

**Tech Stack:** Go 1.22+ (chi v5, GORM, logrus, Postgres, SQLite for tests), React 18 + TypeScript + Vite, TanStack React Query v5, shadcn/ui + Tailwind, Radix primitives, Traefik IngressRoute + Kustomize.

## Global Constraints

- **Design over PRD.** Where `design.md` and `prd.md` disagree, design.md wins; its §14 enumerates all nine deviations. Requirement ids (`FR-ADMIN-*`) still refer to prd.md §4.
- **Recovery window default: `120h`** (5 days), from `ADMIN_PURGE_RECOVERY_WINDOW`. An unparseable value falls back to 120h rather than panicking.
- **Reaper cadence: `1 * time.Hour`**, under `database.WithLeaderLock(db, "admin-purge-reap", …)`. The existing vehicle sweep moves to the same cadence.
- **System confirmation phrase: the exact literal `PURGE EVERYTHING`.** Fleet confirmation is the fleet's exact name. Matching is server-side; a mismatch is 409 with no writes.
- **An admin stamp writes `deleted_at` and `purge_operation_id` and NEVER `purge_after`.** Both legacy sweeps key on `purge_after`; this is what keeps them from eating admin-stamped rows (design F3).
- **A manifest `Where` predicate may reference parent tables but must never filter `deleted_at` on them.** The `AND deleted_at IS NULL` guard belongs to the target table only (design §3.4).
- **`deleted_at` is `*time.Time`, never `gorm.DeletedAt`**, on every table this task touches. `gorm.DeletedAt` would silently change what `Delete()` means at existing call sites that hard-delete today. `fleet.fleets` is the one pre-existing exception and must be read with `Unscoped()` in admin paths.
- **Every nullable `uuid` column is `*string` in Go.** Postgres rejects `''` for a `uuid`.
- **Pagination is `?page[number]=` / `?page[size]=` via `server.ParsePage`** — default 25, hard cap 100. Not `?page=&size=`.
- **`RequirePlatformAdmin` returns 403, never 404** — the deliberate inverse of `RequireSameFleet`'s non-disclosure rule.
- **Every new internal route ships in the same commit as its `internal-deny` routing rule** (design F2). Never separately.
- **No hard-coded palette classes in `.tsx`.** `apps/web/src/test/conventions.test.ts` fails the build on `bg-red-500` and friends. Use the semantic tokens in `src/index.css`. The admin mode band uses `danger-subtle` / `danger-subtle-foreground` / `danger-border`, **not** `--destructive` (reserved for destructive controls under the task-003 token contract).
- **`make ci` is the gate.** Node may need `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22` first.

---

## File Structure

New Go packages, one responsibility each:

| Path | Responsibility |
|---|---|
| `apps/fleet-service/internal/admin/manifest.go` | the declarative table list + `Scope`/`Root`/`Target`/`OrphanRule` types |
| `…/admin/operations.go` | `Count`, `CountByOperation`, `Stamp`, `Restore`, `Reap` — the only writers of purge state |
| `…/admin/orphans.go` | `DeleteVehicleChildren`, `DeleteOrphans` — manifest-driven cascade for the §11 defect |
| `…/admin/confirmation.go` | `MatchConfirmation`, `RecoveryWindow`, scope/target enums |
| `…/admin/entity.go` `model.go` `builder.go` `provider.go` `administrator.go` | `purge_operations` + `admin_audit_events` persistence |
| `…/admin/processor.go` | create / cancel / retry lifecycle orchestration |
| `…/admin/stats.go` | `/admin/stats` local counts + concurrent remote fan-out |
| `…/admin/reaper.go` | the hourly sweep |
| `…/admin/resource.go` `rest.go` | chi handlers + JSON:API transforms |
| `…/admin/arch_test.go` | manifest completeness + the R7 separation test |
| `…/admin/admintest/db.go` | the one hand-written SQLite DDL fixture every purge test uses |
| `apps/fleet-service/internal/adminclient/` | HTTP clients for auth / media / notification internal admin routes |
| `apps/auth-service/internal/platformadmin/` | `auth.platform_admins` entity, provider, administrator, seed, internal routes |
| `apps/media-service/internal/admin/` | media manifest + stamp/restore/reap + internal routes |
| `apps/notification-service/internal/admin/` | notification manifest + stamp/restore/reap + internal routes |

New web modules:

| Path | Responsibility |
|---|---|
| `src/components/ui/{dialog,table,badge}.tsx` | the three missing kit primitives |
| `src/components/admin/AdminLayout.tsx` | dedicated shell: own sidebar + persistent mode band |
| `src/components/admin/RequirePlatformAdmin.tsx` | route guard (auth + `platformAdmin`, **no** fleet requirement) |
| `src/components/admin/BlastRadiusPanel.tsx` | per-domain breakdown + the purge control it gates |
| `src/components/admin/PurgeConfirmDialog.tsx` | exact-match typing gate, absolute deadline, survivors list |
| `src/pages/admin/*.tsx` | overview, fleets, users, purges, audit |
| `src/services/api/AdminService.ts` | typed `/api/fleet/admin` calls |
| `src/lib/hooks/api/admin.ts` | React Query hooks + `adminKeys` factory |
| `src/lib/admin/purgeStatus.ts` | **the single** API-vocabulary → user-language map |
| `src/types/models/admin.ts` | attribute interfaces for every admin resource |

---

# Phase 1 — Foundation (Tasks 1–8)

These have no visible payoff and the highest risk of being deferred. Deferring them means building the console on data that lies.

---

### Task 1: fleet-service schema — soft-delete columns and partial unique indexes

**Files:**
- Modify: `apps/fleet-service/internal/mileage/entity.go`
- Modify: `apps/fleet-service/internal/maintenanceschedule/entity.go`
- Modify: `apps/fleet-service/internal/maintenancerecord/entity.go`
- Modify: `apps/fleet-service/internal/membership/entity.go`
- Modify: `apps/fleet-service/internal/invite/entity.go`
- Modify: `apps/fleet-service/internal/activity/entity.go`
- Modify: `apps/fleet-service/internal/dashboard/entity.go`
- Modify: `apps/fleet-service/internal/fleet/entity.go`
- Modify: `apps/fleet-service/internal/vehicle/entity.go`
- Modify: `apps/fleet-service/internal/fuel/entity.go`
- Modify: `apps/fleet-service/internal/vehiclemedia/entity.go`
- Test: `apps/fleet-service/internal/membership/migration_test.go` (create)
- Test: `apps/fleet-service/internal/invite/migration_test.go` (create)

**Interfaces:**
- Consumes: nothing.
- Produces: every table listed in design §6.2 carries `DeletedAt *time.Time` and `PurgeOperationID *string`; the partial unique indexes `ux_membership_fleet_user`, `ux_invite_token`, `ux_dashboard_fleet_user` exist. Later tasks assume the column names `deleted_at` and `purge_operation_id` verbatim.

**Why the index dance is not optional.** Soft-deleting a membership while a total
unique index on `(fleet_id, user_id)` still exists means the purged row keeps
occupying the index and the user can never rejoin — a purge becomes a permanent
lockout (risks.md R2). And flipping the GORM tag from `uniqueIndex:idx_fleet_user`
to `index` while the DDL drops the *same* name produces index churn on every boot:
`AutoMigrate` recreates what the DDL then drops, forever. So each GORM-managed
index gets a **new** name and the DDL drops only the **legacy** name. Both halves
are then idempotent, and they ship together or neither ships.

- [ ] **Step 1: Write the failing migration test**

Create `apps/fleet-service/internal/membership/migration_test.go`:

```go
package membership

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newMigrationDB gives Migration a schema-qualified "fleet" database to run
// against. SQLite has no schemas, so one is attached under that alias.
func newMigrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec("ATTACH DATABASE ':memory:' AS fleet").Error; err != nil {
		t.Fatalf("attach fleet schema: %v", err)
	}
	// AutoMigrate cannot create a schema-qualified index on SQLite (GORM's
	// driver strips the prefix), so the table is created by hand and only the
	// partial-index half of Migration is exercised here. That half is the one
	// with the lockout consequence.
	ddl := `CREATE TABLE fleet.fleet_memberships (
		id TEXT PRIMARY KEY, fleet_id TEXT, user_id TEXT, role TEXT, status TEXT,
		created_at DATETIME, updated_at DATETIME,
		deleted_at DATETIME, purge_operation_id TEXT)`
	if err := db.Exec(ddl).Error; err != nil {
		t.Fatalf("ddl: %v", err)
	}
	return db
}

// TestPartialUniqueIndex_allowsRejoinAfterSoftDelete is R2 in test form: a
// soft-deleted membership must not occupy the unique index, or purging a member
// locks them out of the fleet permanently.
func TestPartialUniqueIndex_allowsRejoinAfterSoftDelete(t *testing.T) {
	db := newMigrationDB(t)
	if err := ApplyPartialIndexes(db); err != nil {
		t.Fatalf("apply partial indexes: %v", err)
	}

	insert := `INSERT INTO fleet.fleet_memberships (id, fleet_id, user_id, role, status)
	           VALUES (?, 'fleet-1', 'user-1', 'member', 'active')`
	if err := db.Exec(insert, "m1").Error; err != nil {
		t.Fatalf("first insert: %v", err)
	}

	// A live duplicate is still rejected.
	if err := db.Exec(insert, "m2").Error; err == nil {
		t.Fatal("a second LIVE membership for the same (fleet, user) must violate the unique index")
	}

	// Soft-delete the first, then rejoin.
	if err := db.Exec(`UPDATE fleet.fleet_memberships SET deleted_at = CURRENT_TIMESTAMP,
	                   purge_operation_id = 'op-1' WHERE id = 'm1'`).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	if err := db.Exec(insert, "m3").Error; err != nil {
		t.Fatalf("rejoin after soft delete must succeed, got %v", err)
	}
}

// TestApplyPartialIndexes_isIdempotent guards the boot path: Migration runs on
// every startup, and a non-idempotent DDL step would fail the second boot.
func TestApplyPartialIndexes_isIdempotent(t *testing.T) {
	db := newMigrationDB(t)
	for i := 0; i < 3; i++ {
		if err := ApplyPartialIndexes(db); err != nil {
			t.Fatalf("apply %d: %v", i+1, err)
		}
	}
}
```

And the matching one for invites — `apps/fleet-service/internal/invite/migration_test.go`:

```go
package invite

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newInviteMigrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec("ATTACH DATABASE ':memory:' AS fleet").Error; err != nil {
		t.Fatalf("attach fleet schema: %v", err)
	}
	ddl := `CREATE TABLE fleet.fleet_invites (
		id TEXT PRIMARY KEY, fleet_id TEXT, email TEXT, role TEXT, token TEXT,
		expires_at DATETIME, accepted_at DATETIME, invited_by_user_id TEXT,
		created_at DATETIME, updated_at DATETIME,
		deleted_at DATETIME, purge_operation_id TEXT)`
	if err := db.Exec(ddl).Error; err != nil {
		t.Fatalf("ddl: %v", err)
	}
	return db
}

// A purged invite must release its token. Tokens are generated, so a collision
// is not the worry — the worry is a re-invite failing with an unexplained
// constraint error days after an unrelated purge.
func TestPartialTokenIndex_freesTheTokenAfterSoftDelete(t *testing.T) {
	db := newInviteMigrationDB(t)
	if err := ApplyPartialIndexes(db); err != nil {
		t.Fatalf("apply partial indexes: %v", err)
	}

	insert := `INSERT INTO fleet.fleet_invites (id, fleet_id, email, role, token, invited_by_user_id)
	           VALUES (?, 'fleet-1', 'a@example.com', 'member', 'tok-1', 'owner-1')`
	if err := db.Exec(insert, "i1").Error; err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if err := db.Exec(insert, "i2").Error; err == nil {
		t.Fatal("a second LIVE invite with the same token must be rejected")
	}

	if err := db.Exec(`UPDATE fleet.fleet_invites SET deleted_at = CURRENT_TIMESTAMP,
	                   purge_operation_id = 'op-1' WHERE id = 'i1'`).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	if err := db.Exec(insert, "i3").Error; err != nil {
		t.Fatalf("re-issuing the token after a purge must succeed, got %v", err)
	}
}

func TestInviteApplyPartialIndexes_isIdempotent(t *testing.T) {
	db := newInviteMigrationDB(t)
	for i := 0; i < 3; i++ {
		if err := ApplyPartialIndexes(db); err != nil {
			t.Fatalf("apply %d: %v", i+1, err)
		}
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test ./apps/fleet-service/internal/membership/ -run TestPartialUniqueIndex -v`
Expected: FAIL — `undefined: ApplyPartialIndexes`.

- [ ] **Step 3: Add the columns and rewrite `membership.Migration`**

In `apps/fleet-service/internal/membership/entity.go`, change the struct tags and
the migration. Note the index **rename** — `idx_fleet_user` becomes
`idx_membership_fleet_user`, so `AutoMigrate` manages a name the DDL never touches:

```go
// Entity maps to fleet.fleet_memberships (PRD §6).
//
// The (fleet_id, user_id) uniqueness constraint is NOT expressed here. GORM can
// only emit a total unique index, and a total index over a soft-deletable table
// turns a purge into a permanent lockout (risks.md R2): the purged row keeps
// occupying the index and the user can never rejoin. The real constraint is the
// partial index in ApplyPartialIndexes below, predicated on deleted_at IS NULL.
// The tag here is a plain composite index under a DIFFERENT name so AutoMigrate
// and the hand-written DDL never fight over the same object.
type Entity struct {
	ID               string `gorm:"type:uuid;primaryKey"`
	FleetID          string `gorm:"type:uuid;not null;index:idx_membership_fleet_user"`
	UserID           string `gorm:"not null;index:idx_membership_fleet_user"`
	Role             string `gorm:"not null"` // owner | member | viewer
	Status           string `gorm:"not null"` // active
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        *time.Time `gorm:"index"`
	PurgeOperationID *string    `gorm:"type:uuid;index"`
}

func (Entity) TableName() string { return "fleet.fleet_memberships" }

func Migration(db *gorm.DB) error {
	if err := db.AutoMigrate(&Entity{}); err != nil {
		return err
	}
	return ApplyPartialIndexes(db)
}

// ApplyPartialIndexes replaces the legacy TOTAL unique index on
// (fleet_id, user_id) with one predicated on deleted_at IS NULL.
//
// It is split out of Migration so it can be tested without AutoMigrate, which
// cannot create schema-qualified indexes on SQLite.
//
// The DROP names the LEGACY index only. AutoMigrate owns
// idx_membership_fleet_user and would recreate anything dropped from under it on
// the next boot; dropping only the name AutoMigrate no longer emits keeps both
// halves idempotent.
func ApplyPartialIndexes(db *gorm.DB) error {
	stmts := []string{
		`DROP INDEX IF EXISTS fleet.idx_fleet_user`,
		`CREATE UNIQUE INDEX IF NOT EXISTS ux_membership_fleet_user
		 ON fleet.fleet_memberships (fleet_id, user_id) WHERE deleted_at IS NULL`,
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			return err
		}
	}
	return nil
}
```

Also add `Make`/`ToEntity` passthrough for the two new columns is **not** needed:
`membership.Model` carries no deleted state and no code reads it. Leave
`Make`/`ToEntity` untouched — but note `ToEntity` omits `DeletedAt`, so
`Administrator.Insert` (a full-column `Create`) writes NULL, which is correct.

- [ ] **Step 4: Run the test to green**

Run: `go test ./apps/fleet-service/internal/membership/ -v`
Expected: PASS, including the two new tests and the existing processor tests.

- [ ] **Step 5: Apply the same column pair to every other fleet-service table**

Add these two fields to each entity struct listed below. Where a `deleted_at`
already exists, add only `PurgeOperationID`.

```go
	DeletedAt        *time.Time `gorm:"index"`
	PurgeOperationID *string    `gorm:"type:uuid;index"`
```

| File | `DeletedAt` | `PurgeOperationID` | Notes |
|---|---|---|---|
| `mileage/entity.go` | add | add | |
| `maintenanceschedule/entity.go` | add | add | |
| `maintenancerecord/entity.go` (`Entity`) | exists | add | |
| `maintenancerecord/entity.go` (`DocumentEntity`) | add | add | |
| `activity/entity.go` | add | add | |
| `dashboard/entity.go` (`DashboardEntity`) | add | add | |
| `dashboard/entity.go` (`WidgetEntity`) | add | add | |
| `fleet/entity.go` | exists (`gorm.DeletedAt`) — **leave as is** | add | the one table on GORM soft-delete; admin reads use `Unscoped()` |
| `vehicle/entity.go` | exists | add | keep `PurgeAfter` |
| `fuel/entity.go` | exists | add | |
| `vehiclemedia/entity.go` | exists | add | |
| `invite/entity.go` | add | add | plus the index change below |

For `invite/entity.go`, the `Token` field also changes and `Migration` gains DDL.
GORM's default name for the unnamed `uniqueIndex` on `Entity.Token` is
`idx_fleet_fleet_invites_token` (`idx_` + table with `.`→`_` + column):

```go
	// Token's uniqueness moves to a partial index for the same reason as
	// fleet_memberships: a soft-deleted invite must not reserve its token
	// forever. The tag is a plain index under a name AutoMigrate owns.
	Token            string `gorm:"not null;index:idx_invite_token"`
	...
	DeletedAt        *time.Time `gorm:"index"`
	PurgeOperationID *string    `gorm:"type:uuid;index"`
```

```go
func Migration(db *gorm.DB) error {
	if err := db.AutoMigrate(&Entity{}); err != nil {
		return err
	}
	return ApplyPartialIndexes(db)
}

// ApplyPartialIndexes replaces the legacy total unique index on token with one
// predicated on deleted_at IS NULL. See membership.ApplyPartialIndexes for why
// the dropped name and the AutoMigrate-managed name must differ.
func ApplyPartialIndexes(db *gorm.DB) error {
	stmts := []string{
		`DROP INDEX IF EXISTS fleet.idx_fleet_fleet_invites_token`,
		`CREATE UNIQUE INDEX IF NOT EXISTS ux_invite_token
		 ON fleet.fleet_invites (token) WHERE deleted_at IS NULL`,
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			return err
		}
	}
	return nil
}
```

For `dashboard/entity.go`, add the index the stale doc comment already claims
(design F1 / §6.4) — it does not exist today:

```go
// DashboardEntity maps to fleet.dashboards (plan §13.1).
//
// One layout per (fleet, user). That invariant was only ever a comment: the
// tags declared a plain index on FleetID and nothing enforced uniqueness. It is
// enforced now, as a PARTIAL index, so a purged dashboard does not block the
// user's next save (design §6.4).
type DashboardEntity struct {
	ID               string `gorm:"type:uuid;primaryKey"`
	FleetID          string `gorm:"type:uuid;not null;index:idx_dashboard_fleet_user"`
	UserID           string `gorm:"type:uuid;not null;index:idx_dashboard_fleet_user"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        *time.Time `gorm:"index"`
	PurgeOperationID *string    `gorm:"type:uuid;index"`
}
```

```go
func Migration(db *gorm.DB) error {
	if err := db.AutoMigrate(&DashboardEntity{}, &WidgetEntity{}); err != nil {
		return err
	}
	return ApplyPartialIndexes(db)
}

// ApplyPartialIndexes enforces one live dashboard per (fleet, user).
func ApplyPartialIndexes(db *gorm.DB) error {
	return db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS ux_dashboard_fleet_user
	                ON fleet.dashboards (fleet_id, user_id) WHERE deleted_at IS NULL`).Error
}
```

Where an entity's `Make`/`ToEntity` already round-trips `deletedAt` (vehicle,
fuel, maintenancerecord, vehiclemedia, mediaobject), leave those alone. Do **not**
add `purgeOperationID` to any `Model`: no domain logic reads it, and the manifest
operates on raw SQL.

- [ ] **Step 6: Verify the whole service still builds and passes**

Run: `go build github.com/jtumidanski/myfleet/... && go test ./apps/fleet-service/...`
Expected: PASS. Existing tests that hand-write DDL will still pass — they create
tables without the new columns, and nothing reads them yet.

- [ ] **Step 7: Commit**

```bash
git add apps/fleet-service/internal
git commit -m "feat(fleet): add soft-delete and purge-operation columns with partial unique indexes"
```

---

### Task 2: media-service and notification-service schema

**Files:**
- Modify: `apps/media-service/internal/mediaobject/entity.go`
- Modify: `apps/media-service/internal/mediavariant/entity.go`
- Modify: `apps/notification-service/internal/notification/entity.go`
- Modify: `apps/notification-service/internal/preferences/entity.go`
- Test: `apps/notification-service/internal/notification/migration_test.go` (create)

**Interfaces:**
- Consumes: the column-naming convention from Task 1.
- Produces: `media.media_objects`, `media.media_variants`,
  `notification.notifications`, `notification.notification_preferences` all carry
  `deleted_at` + `purge_operation_id`; partial unique indexes
  `ux_notification_dedupe_key` and `ux_pref_user_type_live` exist;
  `notification.notifications.fleet_id` is indexed.

**Why two more indexes than the PRD names (design F1).** `notifications.dedupe_key`
is `uniqueIndex`. Soft-delete a notification and the key stays in the index, so
**that notification can never be regenerated** — and `ExistsByDedupeKey`, which
the reminder job's safety-net and event redelivery both depend on, would keep
reporting it as present. `notification_preferences (user_id, type)` has the same
shape: after a system purge the user's preference row cannot be re-created.

- [ ] **Step 1: Write the failing lockout test**

Create `apps/notification-service/internal/notification/migration_test.go`:

```go
package notification

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newNotificationMigrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec("ATTACH DATABASE ':memory:' AS notification").Error; err != nil {
		t.Fatalf("attach notification schema: %v", err)
	}
	ddl := `CREATE TABLE notification.notifications (
		id TEXT PRIMARY KEY, user_id TEXT, type TEXT, title TEXT, body TEXT,
		dedupe_key TEXT, vehicle_id TEXT, fleet_id TEXT, read_at DATETIME,
		created_at DATETIME, deleted_at DATETIME, purge_operation_id TEXT)`
	if err := db.Exec(ddl).Error; err != nil {
		t.Fatalf("ddl: %v", err)
	}
	return db
}

// TestPartialDedupeIndex_allowsRegenerationAfterSoftDelete is design F1 in test
// form. A purged notification whose dedupe_key still occupies a total unique
// index can never be regenerated — the reminder safety-net and event
// redelivery would both be permanently suppressed for it.
func TestPartialDedupeIndex_allowsRegenerationAfterSoftDelete(t *testing.T) {
	db := newNotificationMigrationDB(t)
	if err := ApplyPartialIndexes(db); err != nil {
		t.Fatalf("apply partial indexes: %v", err)
	}

	insert := `INSERT INTO notification.notifications (id, user_id, type, title, dedupe_key)
	           VALUES (?, 'user-1', 'schedule.overdue', 'Oil change due', 'dk-1')`
	if err := db.Exec(insert, "n1").Error; err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if err := db.Exec(insert, "n2").Error; err == nil {
		t.Fatal("a second LIVE notification with the same dedupe_key must be rejected")
	}

	if err := db.Exec(`UPDATE notification.notifications SET deleted_at = CURRENT_TIMESTAMP,
	                   purge_operation_id = 'op-1' WHERE id = 'n1'`).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	if err := db.Exec(insert, "n3").Error; err != nil {
		t.Fatalf("regeneration after soft delete must succeed, got %v", err)
	}
}

func TestNotificationApplyPartialIndexes_isIdempotent(t *testing.T) {
	db := newNotificationMigrationDB(t)
	for i := 0; i < 3; i++ {
		if err := ApplyPartialIndexes(db); err != nil {
			t.Fatalf("apply %d: %v", i+1, err)
		}
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test ./apps/notification-service/internal/notification/ -run PartialDedupe -v`
Expected: FAIL — `undefined: ApplyPartialIndexes`.

- [ ] **Step 3: Update the notification entity and migration**

`apps/notification-service/internal/notification/entity.go`. The legacy index name
GORM emitted for the unnamed `uniqueIndex` on `DedupeKey` is
`idx_notification_notifications_dedupe_key`:

```go
// Entity maps to notification.notifications (PRD §6).
//
// dedupe_key's uniqueness is a PARTIAL index (see ApplyPartialIndexes), not a
// tag: a soft-deleted notification must release its key, or the reminder
// safety-net and event redelivery are permanently suppressed for it (design F1).
//
// fleet_id is indexed because a fleet-scoped admin purge selects on it. It stays
// nullable: account-level notifications carry no fleet and are taken only by a
// system purge.
type Entity struct {
	ID               string `gorm:"type:uuid;primaryKey"`
	UserID           string `gorm:"type:uuid;not null;index"`
	Type             string `gorm:"not null"`
	Title            string `gorm:"not null"`
	Body             string
	DedupeKey        string `gorm:"not null;index:idx_notification_dedupe_key"`
	VehicleID        string `gorm:"type:uuid"`
	FleetID          string `gorm:"type:uuid;index"`
	ReadAt           *time.Time
	CreatedAt        time.Time
	DeletedAt        *time.Time `gorm:"index"`
	PurgeOperationID *string    `gorm:"type:uuid;index"`
}

func (Entity) TableName() string { return "notification.notifications" }

// Migration auto-migrates the notifications table and installs the partial
// unique index on dedupe_key.
func Migration(db *gorm.DB) error {
	if err := db.AutoMigrate(&Entity{}); err != nil {
		return err
	}
	return ApplyPartialIndexes(db)
}

// ApplyPartialIndexes replaces the legacy total unique index on dedupe_key with
// one predicated on deleted_at IS NULL.
func ApplyPartialIndexes(db *gorm.DB) error {
	stmts := []string{
		`DROP INDEX IF EXISTS notification.idx_notification_notifications_dedupe_key`,
		`CREATE UNIQUE INDEX IF NOT EXISTS ux_notification_dedupe_key
		 ON notification.notifications (dedupe_key) WHERE deleted_at IS NULL`,
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			return err
		}
	}
	return nil
}
```

`Administrator.Insert` uses `clause.OnConflict{Columns: [dedupe_key]}`. Postgres
resolves that against a unique index on `dedupe_key`; a **partial** index does not
satisfy an `ON CONFLICT (dedupe_key)` inference. Change the insert to target the
partial index explicitly with `Where`:

```go
// Insert persists a new notification. The conflict target names the PARTIAL
// unique index (dedupe_key WHERE deleted_at IS NULL) — an unqualified
// ON CONFLICT (dedupe_key) no longer infers an index now that the constraint is
// partial, and Postgres would reject the statement outright.
func (a *dbAdministrator) Insert(m Model) error {
	e := m.ToEntity()
	err := a.db.Clauses(clause.OnConflict{
		Columns:     []clause.Column{{Name: "dedupe_key"}},
		TargetWhere: clause.Where{Exprs: []clause.Expression{clause.Expr{SQL: "deleted_at IS NULL"}}},
		DoNothing:   true,
	}).Create(&e)
	if err.Error != nil {
		if errors.Is(err.Error, gorm.ErrDuplicatedKey) {
			return ErrDuplicate
		}
		return err.Error
	}
	if err.RowsAffected == 0 {
		return ErrDuplicate
	}
	return nil
}
```

- [ ] **Step 4: Update preferences, media object, and media variant entities**

`apps/notification-service/internal/preferences/entity.go`:

```go
// Entity maps to notification.notification_preferences (PRD §6).
//
// (user_id, type) uniqueness is a PARTIAL index (ApplyPartialIndexes): after a
// system purge a user's preference row must be re-creatable (design F1).
type Entity struct {
	ID               string `gorm:"type:uuid;primaryKey"`
	UserID           string `gorm:"type:uuid;not null;index:idx_pref_user_type"`
	Type             string `gorm:"not null;index:idx_pref_user_type"`
	InAppEnabled     bool   `gorm:"not null"`
	DeletedAt        *time.Time `gorm:"index"`
	PurgeOperationID *string    `gorm:"type:uuid;index"`
}

func (Entity) TableName() string { return "notification.notification_preferences" }

func Migration(db *gorm.DB) error {
	if err := db.AutoMigrate(&Entity{}); err != nil {
		return err
	}
	return ApplyPartialIndexes(db)
}

// ApplyPartialIndexes replaces the legacy total unique index on (user_id, type)
// with one predicated on deleted_at IS NULL.
func ApplyPartialIndexes(db *gorm.DB) error {
	stmts := []string{
		`DROP INDEX IF EXISTS notification.ux_pref_user_type`,
		`CREATE UNIQUE INDEX IF NOT EXISTS ux_pref_user_type_live
		 ON notification.notification_preferences (user_id, type) WHERE deleted_at IS NULL`,
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			return err
		}
	}
	return nil
}
```

`preferences.Administrator.Upsert` has the same `ON CONFLICT` problem — give it
the same `TargetWhere`:

```go
	if err := a.db.Clauses(clause.OnConflict{
		Columns:     []clause.Column{{Name: "user_id"}, {Name: "type"}},
		TargetWhere: clause.Where{Exprs: []clause.Expression{clause.Expr{SQL: "deleted_at IS NULL"}}},
		DoUpdates:   clause.AssignmentColumns([]string{"in_app_enabled"}),
	}).Create(&e).Error; err != nil {
```

`apps/media-service/internal/mediaobject/entity.go` — add `PurgeOperationID`
only (it already has `DeletedAt` and `PurgeAfter`, both of which stay):

```go
	DeletedAt        *time.Time `gorm:"index"`
	PurgeAfter       *time.Time
	PurgeOperationID *string `gorm:"type:uuid;index"`
```

`apps/media-service/internal/mediavariant/entity.go` — add both:

```go
	CreatedAt        time.Time
	DeletedAt        *time.Time `gorm:"index"`
	PurgeOperationID *string    `gorm:"type:uuid;index"`
```

- [ ] **Step 5: Run the tests**

Run: `go test ./apps/notification-service/... ./apps/media-service/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add apps/media-service apps/notification-service
git commit -m "feat(media,notification): add soft-delete and purge-operation columns with partial unique indexes"
```

---

### Task 3: the shared SQLite test harness

**Files:**
- Create: `apps/fleet-service/internal/admin/admintest/db.go`
- Create: `apps/fleet-service/internal/admin/admintest/db_test.go`

**Interfaces:**
- Consumes: the column set from Task 1.
- Produces: `admintest.NewDB(t *testing.T) *gorm.DB` — an in-memory SQLite
  database with a `fleet` schema attached and **every** purgeable fleet-service
  table present, plus `fleet.purge_operations` and `fleet.admin_audit_events`.
  Also `admintest.SeedFleet(t, db, fleetID string) admintest.Fixture`.

**Why one file (design F4).** The cascade test needs every purgeable table in one
database — by a wide margin the largest fixture in the repo, currently duplicated
per test file. Hand-maintained DDL in N files is how a column gets added to
production and forgotten in tests; one file is checkable.

- [ ] **Step 1: Write the harness self-test first**

Create `apps/fleet-service/internal/admin/admintest/db_test.go`:

```go
package admintest_test

import (
	"testing"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin/admintest"
)

// TestNewDB_createsEveryPurgeableTable is the harness's own regression test: a
// table added to the manifest but forgotten here would otherwise surface as a
// confusing "no such table" deep inside a cascade test.
func TestNewDB_createsEveryPurgeableTable(t *testing.T) {
	db := admintest.NewDB(t)
	for _, table := range []string{
		"fleet.fleets", "fleet.vehicles", "fleet.fleet_memberships", "fleet.fleet_invites",
		"fleet.mileage_records", "fleet.fuel_logs", "fleet.maintenance_records",
		"fleet.maintenance_record_documents", "fleet.maintenance_schedules",
		"fleet.vehicle_media", "fleet.activity_events", "fleet.dashboards",
		"fleet.dashboard_widgets", "fleet.maintenance_categories",
		"fleet.purge_operations", "fleet.admin_audit_events",
	} {
		var n int64
		if err := db.Raw("SELECT count(*) FROM " + table).Scan(&n).Error; err != nil {
			t.Errorf("%s is missing from admintest.NewDB: %v", table, err)
		}
	}
}

// TestSeedFleet_populatesEveryTable makes the fixture usable as a cascade
// oracle: every purgeable table must have at least one row, or a cascade test
// can pass by simply never touching an empty table.
func TestSeedFleet_populatesEveryTable(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-1")
	if f.VehicleID == "" || f.MaintenanceRecordID == "" || f.DashboardID == "" {
		t.Fatalf("fixture is incomplete: %+v", f)
	}
	for _, table := range []string{
		"fleet.vehicles", "fleet.fleet_memberships", "fleet.fleet_invites",
		"fleet.mileage_records", "fleet.fuel_logs", "fleet.maintenance_records",
		"fleet.maintenance_record_documents", "fleet.maintenance_schedules",
		"fleet.vehicle_media", "fleet.activity_events", "fleet.dashboards",
		"fleet.dashboard_widgets",
	} {
		var n int64
		if err := db.Raw("SELECT count(*) FROM " + table).Scan(&n).Error; err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if n == 0 {
			t.Errorf("%s has no seeded rows — a cascade test over this fixture would pass vacuously", table)
		}
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test ./apps/fleet-service/internal/admin/admintest/ -v`
Expected: FAIL — the package does not exist.

- [ ] **Step 3: Write the harness**

Create `apps/fleet-service/internal/admin/admintest/db.go`:

```go
// Package admintest owns the one hand-written SQLite fixture every purge,
// restore, reap, and cascade test runs against.
//
// It exists because AutoMigrate cannot build this schema on SQLite: GORM's
// SQLite driver emits CREATE INDEX with the schema prefix stripped, so a
// schema-qualified table like fleet.fleet_invites can never receive its index
// and AutoMigrate fails. Every existing DB-backed test in fleet-service works
// around that with its own copy of the DDL. The cascade test needs EVERY
// purgeable table in one database, which makes that duplication untenable:
// N hand-maintained copies is how a column reaches production and is forgotten
// in tests. One file is checkable, and db_test.go checks it.
package admintest

import (
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Fixture holds the ids SeedFleet generated, so a test can assert against
// specific rows rather than re-querying for them.
type Fixture struct {
	FleetID             string
	OwnerUserID         string
	MemberUserID        string
	VehicleID           string
	SecondVehicleID     string
	MaintenanceRecordID string
	DocumentID          string
	ScheduleID          string
	FuelLogID           string
	MileageRecordID     string
	VehicleMediaID      string
	MediaID             string
	MembershipID        string
	InviteID            string
	ActivityEventID     string
	DashboardID         string
	WidgetID            string
}

// ddl is the complete purgeable fleet schema. Column lists mirror the GORM
// entities exactly; a column added to an entity must be added here in the same
// commit. Types are SQLite's, which is loose enough that TEXT/DATETIME/REAL
// cover every Postgres type in play (jsonb rides as TEXT).
var ddl = []string{
	`CREATE TABLE fleet.fleets (
		id TEXT PRIMARY KEY, name TEXT, created_by_user_id TEXT,
		created_at DATETIME, updated_at DATETIME,
		deleted_at DATETIME, purge_operation_id TEXT)`,
	`CREATE TABLE fleet.vehicles (
		id TEXT PRIMARY KEY, fleet_id TEXT, nickname TEXT, make TEXT, model TEXT,
		trim TEXT, year INTEGER, vin TEXT, current_mileage INTEGER,
		primary_image_media_id TEXT, notes TEXT, created_at DATETIME,
		updated_at DATETIME, deleted_at DATETIME, purge_after DATETIME,
		purge_operation_id TEXT)`,
	`CREATE TABLE fleet.fleet_memberships (
		id TEXT PRIMARY KEY, fleet_id TEXT, user_id TEXT, role TEXT, status TEXT,
		created_at DATETIME, updated_at DATETIME,
		deleted_at DATETIME, purge_operation_id TEXT)`,
	`CREATE TABLE fleet.fleet_invites (
		id TEXT PRIMARY KEY, fleet_id TEXT, email TEXT, role TEXT, token TEXT,
		expires_at DATETIME, accepted_at DATETIME, invited_by_user_id TEXT,
		created_at DATETIME, updated_at DATETIME,
		deleted_at DATETIME, purge_operation_id TEXT)`,
	`CREATE TABLE fleet.mileage_records (
		id TEXT PRIMARY KEY, vehicle_id TEXT, mileage INTEGER, recorded_at DATETIME,
		source TEXT, source_ref_id TEXT, created_by_user_id TEXT, created_at DATETIME,
		deleted_at DATETIME, purge_operation_id TEXT)`,
	`CREATE TABLE fleet.fuel_logs (
		id TEXT PRIMARY KEY, vehicle_id TEXT, date DATETIME, mileage INTEGER,
		gallons REAL, total_cost REAL, price_per_gallon REAL,
		created_by_user_id TEXT, created_at DATETIME, updated_at DATETIME,
		deleted_at DATETIME, purge_operation_id TEXT)`,
	`CREATE TABLE fleet.maintenance_records (
		id TEXT PRIMARY KEY, vehicle_id TEXT, category_id TEXT, description TEXT,
		performed_at DATETIME, mileage INTEGER, cost REAL, vendor TEXT, notes TEXT,
		created_by_user_id TEXT, created_at DATETIME, updated_at DATETIME,
		deleted_at DATETIME, purge_operation_id TEXT)`,
	`CREATE TABLE fleet.maintenance_record_documents (
		id TEXT PRIMARY KEY, maintenance_record_id TEXT, media_id TEXT,
		deleted_at DATETIME, purge_operation_id TEXT)`,
	`CREATE TABLE fleet.maintenance_schedules (
		id TEXT PRIMARY KEY, vehicle_id TEXT, category_id TEXT, recurrence_type TEXT,
		interval_months INTEGER, interval_miles INTEGER, last_completed_date DATETIME,
		last_completed_mileage INTEGER, next_due_date DATETIME, next_due_mileage INTEGER,
		status TEXT, severity TEXT, active BOOLEAN, created_at DATETIME,
		updated_at DATETIME, deleted_at DATETIME, purge_operation_id TEXT)`,
	`CREATE TABLE fleet.vehicle_media (
		id TEXT PRIMARY KEY, vehicle_id TEXT, media_id TEXT, is_primary BOOLEAN,
		sort_order INTEGER, created_at DATETIME,
		deleted_at DATETIME, purge_operation_id TEXT)`,
	`CREATE TABLE fleet.activity_events (
		id TEXT PRIMARY KEY, fleet_id TEXT, vehicle_id TEXT, actor_user_id TEXT,
		type TEXT, payload BLOB, created_at DATETIME,
		deleted_at DATETIME, purge_operation_id TEXT)`,
	`CREATE TABLE fleet.dashboards (
		id TEXT PRIMARY KEY, fleet_id TEXT, user_id TEXT,
		created_at DATETIME, updated_at DATETIME,
		deleted_at DATETIME, purge_operation_id TEXT)`,
	`CREATE TABLE fleet.dashboard_widgets (
		id TEXT PRIMARY KEY, dashboard_id TEXT, type TEXT, position_x INTEGER,
		position_y INTEGER, width INTEGER, height INTEGER, config BLOB,
		deleted_at DATETIME, purge_operation_id TEXT)`,
	// Seeded reference data. Present so a cascade test can assert it SURVIVES.
	`CREATE TABLE fleet.maintenance_categories (
		id TEXT PRIMARY KEY, name TEXT, description TEXT,
		system_defined BOOLEAN, kind TEXT)`,
	`CREATE TABLE fleet.purge_operations (
		id TEXT PRIMARY KEY, scope TEXT, target_type TEXT, target_id TEXT,
		target_label TEXT, status TEXT, requested_by_user_id TEXT,
		requested_by_email TEXT, requested_at DATETIME, purge_after DATETIME,
		reaped_at DATETIME, cancelled_at DATETIME,
		affected_counts BLOB, failed_services BLOB,
		created_at DATETIME, updated_at DATETIME)`,
	// No deleted_at: append-only, and it survives a system purge
	// (FR-ADMIN-AUDIT-2).
	`CREATE TABLE fleet.admin_audit_events (
		id TEXT PRIMARY KEY, actor_user_id TEXT, actor_email TEXT, action TEXT,
		scope TEXT, target_type TEXT, target_id TEXT, target_label TEXT,
		purge_operation_id TEXT, affected_counts BLOB, correlation_id TEXT,
		created_at DATETIME)`,
	// The outbox is created because domain writes enqueue into it; it is
	// explicitly NOT purgeable (see admin.excludedTables).
	`CREATE TABLE outbox (
		event_id TEXT PRIMARY KEY, type TEXT, payload BLOB,
		occurred_at DATETIME, sent_at DATETIME)`,
	// The partial unique indexes are part of the schema under test: the lockout
	// regression (FR-ADMIN-DATA-4) is only meaningful if they are present.
	// SQLite supports partial indexes, so these are the real thing.
	`CREATE UNIQUE INDEX ux_membership_fleet_user
	   ON fleet.fleet_memberships (fleet_id, user_id) WHERE deleted_at IS NULL`,
	`CREATE UNIQUE INDEX ux_invite_token
	   ON fleet.fleet_invites (token) WHERE deleted_at IS NULL`,
	`CREATE UNIQUE INDEX ux_dashboard_fleet_user
	   ON fleet.dashboards (fleet_id, user_id) WHERE deleted_at IS NULL`,
}

// NewDB returns an in-memory database carrying the complete purgeable fleet
// schema under an attached "fleet" alias.
func NewDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec("ATTACH DATABASE ':memory:' AS fleet").Error; err != nil {
		t.Fatalf("attach fleet schema: %v", err)
	}
	for _, stmt := range ddl {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("ddl %.60s…: %v", stmt, err)
		}
	}
	return db
}

// SeedFleet inserts one row in every purgeable table for the given fleet, plus
// one system-defined maintenance category (which must survive every purge).
//
// Ids are derived from fleetID rather than random so a failing assertion names
// something a human can find.
func SeedFleet(t *testing.T, db *gorm.DB, fleetID string) Fixture {
	t.Helper()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	f := Fixture{
		FleetID:             fleetID,
		OwnerUserID:         fleetID + "-owner",
		MemberUserID:        fleetID + "-member",
		VehicleID:           fleetID + "-vehicle-1",
		SecondVehicleID:     fleetID + "-vehicle-2",
		MaintenanceRecordID: fleetID + "-record",
		DocumentID:          fleetID + "-document",
		ScheduleID:          fleetID + "-schedule",
		FuelLogID:           fleetID + "-fuel",
		MileageRecordID:     fleetID + "-mileage",
		VehicleMediaID:      fleetID + "-vehicle-media",
		MediaID:             fleetID + "-media",
		MembershipID:        fleetID + "-membership",
		InviteID:            fleetID + "-invite",
		ActivityEventID:     fleetID + "-activity",
		DashboardID:         fleetID + "-dashboard",
		WidgetID:            fleetID + "-widget",
	}

	exec := func(q string, args ...any) {
		t.Helper()
		if err := db.Exec(q, args...).Error; err != nil {
			t.Fatalf("seed %.50s…: %v", q, err)
		}
	}

	exec(`INSERT INTO fleet.fleets (id, name, created_by_user_id, created_at)
	      VALUES (?, ?, ?, ?)`, f.FleetID, "Fleet "+fleetID, f.OwnerUserID, now)
	exec(`INSERT INTO fleet.vehicles (id, fleet_id, make, model, year, current_mileage, created_at)
	      VALUES (?, ?, 'Toyota', 'Corolla', 2020, 50000, ?)`, f.VehicleID, f.FleetID, now)
	exec(`INSERT INTO fleet.vehicles (id, fleet_id, make, model, year, current_mileage, created_at)
	      VALUES (?, ?, 'Honda', 'Civic', 2018, 90000, ?)`, f.SecondVehicleID, f.FleetID, now)
	exec(`INSERT INTO fleet.fleet_memberships (id, fleet_id, user_id, role, status, created_at)
	      VALUES (?, ?, ?, 'owner', 'active', ?)`, f.MembershipID, f.FleetID, f.OwnerUserID, now)
	exec(`INSERT INTO fleet.fleet_invites (id, fleet_id, email, role, token, expires_at, invited_by_user_id, created_at)
	      VALUES (?, ?, 'invitee@example.com', 'member', ?, ?, ?, ?)`,
		f.InviteID, f.FleetID, fleetID+"-token", now.Add(48*time.Hour), f.OwnerUserID, now)
	exec(`INSERT INTO fleet.mileage_records (id, vehicle_id, mileage, recorded_at, source, created_at)
	      VALUES (?, ?, 50000, ?, 'manual', ?)`, f.MileageRecordID, f.VehicleID, now, now)
	exec(`INSERT INTO fleet.fuel_logs (id, vehicle_id, date, mileage, gallons, total_cost, price_per_gallon, created_at)
	      VALUES (?, ?, ?, 50000, 10.0, 40.0, 4.0, ?)`, f.FuelLogID, f.VehicleID, now, now)
	exec(`INSERT INTO fleet.maintenance_records (id, vehicle_id, category_id, description, performed_at, mileage, cost, created_at)
	      VALUES (?, ?, 'category-1', 'Oil change', ?, 50000, 60.0, ?)`,
		f.MaintenanceRecordID, f.VehicleID, now, now)
	exec(`INSERT INTO fleet.maintenance_record_documents (id, maintenance_record_id, media_id)
	      VALUES (?, ?, ?)`, f.DocumentID, f.MaintenanceRecordID, f.MediaID)
	exec(`INSERT INTO fleet.maintenance_schedules (id, vehicle_id, category_id, recurrence_type, interval_months, active, created_at)
	      VALUES (?, ?, 'category-1', 'time', 6, 1, ?)`, f.ScheduleID, f.VehicleID, now)
	exec(`INSERT INTO fleet.vehicle_media (id, vehicle_id, media_id, is_primary, sort_order, created_at)
	      VALUES (?, ?, ?, 1, 0, ?)`, f.VehicleMediaID, f.VehicleID, f.MediaID, now)
	exec(`INSERT INTO fleet.activity_events (id, fleet_id, vehicle_id, actor_user_id, type, created_at)
	      VALUES (?, ?, ?, ?, 'vehicle.created', ?)`,
		f.ActivityEventID, f.FleetID, f.VehicleID, f.OwnerUserID, now)
	exec(`INSERT INTO fleet.dashboards (id, fleet_id, user_id, created_at)
	      VALUES (?, ?, ?, ?)`, f.DashboardID, f.FleetID, f.OwnerUserID, now)
	exec(`INSERT INTO fleet.dashboard_widgets (id, dashboard_id, type, position_x, position_y, width, height)
	      VALUES (?, ?, 'fleet-overview', 0, 0, 2, 2)`, f.WidgetID, f.DashboardID)
	// Reference data, seeded once and shared by every fleet. A purge must never
	// touch it (PRD non-goal), so tests assert its survival.
	exec(`INSERT OR IGNORE INTO fleet.maintenance_categories (id, name, system_defined, kind)
	      VALUES ('category-1', 'Oil Change', 1, 'maintenance')`)

	return f
}

// CountLive returns the number of rows in table whose deleted_at is NULL.
func CountLive(t *testing.T, db *gorm.DB, table string) int {
	t.Helper()
	var n int64
	if err := db.Raw("SELECT count(*) FROM " + table + " WHERE deleted_at IS NULL").Scan(&n).Error; err != nil {
		t.Fatalf("count live %s: %v", table, err)
	}
	return int(n)
}

// CountRows returns the total row count in table, deleted or not.
func CountRows(t *testing.T, db *gorm.DB, table string) int {
	t.Helper()
	var n int64
	if err := db.Raw("SELECT count(*) FROM " + table).Scan(&n).Error; err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return int(n)
}
```

- [ ] **Step 4: Run the harness tests to green**

Run: `go test ./apps/fleet-service/internal/admin/admintest/ -v`
Expected: PASS — both tests.

- [ ] **Step 5: Commit**

```bash
git add apps/fleet-service/internal/admin/admintest
git commit -m "test(fleet): add shared admintest SQLite fixture for purge tests"
```

---

### Task 4: data-visibility sweep — mileage, maintenance schedules, membership, invites

**Files:**
- Modify: `apps/fleet-service/internal/mileage/provider.go:22`
- Modify: `apps/fleet-service/internal/maintenanceschedule/provider.go:45,55,63,99`
- Modify: `apps/fleet-service/internal/maintenanceschedule/administrator.go:132`
- Modify: `apps/fleet-service/internal/membership/provider.go:33,44,56,68,79`
- Modify: `apps/fleet-service/internal/invite/provider.go:25,36,47`
- Test: `apps/fleet-service/internal/admin/visibility_mileage_test.go` (create)
- Test: `apps/fleet-service/internal/admin/visibility_schedule_test.go` (create)
- Test: `apps/fleet-service/internal/admin/visibility_membership_test.go` (create)
- Test: `apps/fleet-service/internal/admin/visibility_invite_test.go` (create)

**Interfaces:**
- Consumes: `admintest.NewDB`, `admintest.SeedFleet` from Task 3.
- Produces: nothing new; every read path over these four domains filters
  `deleted_at IS NULL`.

**Why this is the riskiest group (R1).** Adding `deleted_at` to a table silently
changes the meaning of every existing query over it. The table below is where to
look; **the tests are the control**. Two sites deserve naming because a miss there
hurts most and review attention naturally does not go there:

- `maintenanceschedule.queryActive` (`provider.go:99`) already joins
  `v.deleted_at IS NULL` on the vehicle but has no predicate on the schedule
  itself. Its no-argument form `ListActive()` backs `/internal/maintenance/due`,
  which drives the reminder job — so a purged schedule would keep **generating
  notifications** for a fleet that no longer exists.
- `membership.CountOwners` (`provider.go:79`) feeds the sole-owner guard. Counting
  purged owners would let the last live owner be removed.

The visibility tests live in `internal/admin` rather than each domain package so
they share `admintest.NewDB` without every domain importing a test helper from a
sibling. They exercise the real providers.

- [ ] **Step 1: Write the four failing regression tests**

Create `apps/fleet-service/internal/admin/visibility_mileage_test.go`:

```go
package admin_test

import (
	"testing"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin/admintest"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/mileage"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// FR-ADMIN-DATA-3: a soft-deleted mileage record must be absent from the list
// path AND from the total it feeds — the total drives pagination, so a stale
// count renders a page of nothing at the end of the list.
func TestMileageProvider_hidesSoftDeleted(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-1")
	prov := mileage.NewProvider(db)
	page := server.Page{Number: 1, Size: 25}

	before, totalBefore, err := prov.ListByVehicle(f.VehicleID, nil, nil, page)
	if err != nil {
		t.Fatalf("list before: %v", err)
	}
	if len(before) != 1 || totalBefore != 1 {
		t.Fatalf("fixture expected exactly one mileage record, got %d rows / total %d", len(before), totalBefore)
	}

	if err := db.Exec(`UPDATE fleet.mileage_records SET deleted_at = CURRENT_TIMESTAMP,
	                   purge_operation_id = 'op-1' WHERE id = ?`, f.MileageRecordID).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	after, totalAfter, err := prov.ListByVehicle(f.VehicleID, nil, nil, page)
	if err != nil {
		t.Fatalf("list after: %v", err)
	}
	if len(after) != 0 {
		t.Errorf("soft-deleted mileage record is still listed: %d rows", len(after))
	}
	if totalAfter != 0 {
		t.Errorf("soft-deleted mileage record still counts toward the page total: %d", totalAfter)
	}
}
```

Create `apps/fleet-service/internal/admin/visibility_schedule_test.go`:

```go
package admin_test

import (
	"testing"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin/admintest"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/maintenanceschedule"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// The ListActive path backs /internal/maintenance/due, which drives the
// notification-service reminder job. A purged schedule that stays visible here
// keeps generating notifications for a fleet that no longer exists — the single
// worst miss in the whole visibility sweep (design §9).
func TestMaintenanceScheduleProvider_hidesSoftDeleted(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-1")
	prov := maintenanceschedule.NewProvider(db)

	if _, err := prov.GetByID(f.ScheduleID); err != nil {
		t.Fatalf("fixture schedule should be readable: %v", err)
	}

	if err := db.Exec(`UPDATE fleet.maintenance_schedules SET deleted_at = CURRENT_TIMESTAMP,
	                   purge_operation_id = 'op-1' WHERE id = ?`, f.ScheduleID).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	if _, err := prov.GetByID(f.ScheduleID); err != maintenanceschedule.ErrNotFound {
		t.Errorf("GetByID must report a soft-deleted schedule as not found, got %v", err)
	}

	rows, total, err := prov.ListByVehicle(f.VehicleID, server.Page{Number: 1, Size: 25})
	if err != nil {
		t.Fatalf("list by vehicle: %v", err)
	}
	if len(rows) != 0 || total != 0 {
		t.Errorf("soft-deleted schedule still listed: %d rows / total %d", len(rows), total)
	}

	active, err := prov.ListActive()
	if err != nil {
		t.Fatalf("list active: %v", err)
	}
	if len(active) != 0 {
		t.Errorf("soft-deleted schedule still reaches the reminder feed: %d rows", len(active))
	}

	byFleet, err := prov.ListActiveByFleet(f.FleetID)
	if err != nil {
		t.Fatalf("list active by fleet: %v", err)
	}
	if len(byFleet) != 0 {
		t.Errorf("soft-deleted schedule still in the fleet queue: %d rows", len(byFleet))
	}

	byVehicle, err := prov.ListActiveByVehicle(f.VehicleID)
	if err != nil {
		t.Fatalf("list active by vehicle: %v", err)
	}
	if len(byVehicle) != 0 {
		t.Errorf("soft-deleted schedule still in the vehicle queue: %d rows", len(byVehicle))
	}
}
```

Create `apps/fleet-service/internal/admin/visibility_membership_test.go`:

```go
package admin_test

import (
	"testing"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin/admintest"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/membership"
)

// CountOwners feeds the sole-owner guard. Counting a purged owner would let the
// last LIVE owner remove themselves and leave the fleet ownerless.
func TestMembershipProvider_hidesSoftDeleted(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-1")
	prov := membership.NewProvider(db)

	if n, err := prov.CountOwners(f.FleetID); err != nil || n != 1 {
		t.Fatalf("fixture expected one owner, got %d err %v", n, err)
	}

	if err := db.Exec(`UPDATE fleet.fleet_memberships SET deleted_at = CURRENT_TIMESTAMP,
	                   purge_operation_id = 'op-1' WHERE id = ?`, f.MembershipID).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	if n, err := prov.CountOwners(f.FleetID); err != nil || n != 0 {
		t.Errorf("CountOwners must ignore soft-deleted rows, got %d err %v", n, err)
	}
	if _, err := prov.GetActiveByUserID(f.OwnerUserID); err != membership.ErrNotFound {
		t.Errorf("GetActiveByUserID must ignore soft-deleted rows, got %v", err)
	}
	if _, err := prov.GetByFleetAndUser(f.FleetID, f.OwnerUserID); err != membership.ErrNotFound {
		t.Errorf("GetByFleetAndUser must ignore soft-deleted rows, got %v", err)
	}
	if ms, err := prov.ListByFleetID(f.FleetID); err != nil || len(ms) != 0 {
		t.Errorf("ListByFleetID must ignore soft-deleted rows, got %d err %v", len(ms), err)
	}
	if ms, err := prov.ListActiveByFleetID(f.FleetID); err != nil || len(ms) != 0 {
		t.Errorf("ListActiveByFleetID must ignore soft-deleted rows, got %d err %v", len(ms), err)
	}
}
```

Create `apps/fleet-service/internal/admin/visibility_invite_test.go`:

```go
package admin_test

import (
	"testing"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin/admintest"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/invite"
)

// A purged invite must not be acceptable. GetByToken is the acceptance path's
// only lookup, so a miss here means a token belonging to a purged fleet still
// grants membership.
func TestInviteProvider_hidesSoftDeleted(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-1")
	prov := invite.NewProvider(db)
	token := f.FleetID + "-token"

	if _, err := prov.GetByToken(token); err != nil {
		t.Fatalf("fixture invite should be readable by token: %v", err)
	}

	if err := db.Exec(`UPDATE fleet.fleet_invites SET deleted_at = CURRENT_TIMESTAMP,
	                   purge_operation_id = 'op-1' WHERE id = ?`, f.InviteID).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	if _, err := prov.GetByID(f.InviteID); err != invite.ErrNotFound {
		t.Errorf("GetByID must report a soft-deleted invite as not found, got %v", err)
	}
	if _, err := prov.GetByToken(token); err != invite.ErrNotFound {
		t.Errorf("a soft-deleted invite must not be acceptable by token, got %v", err)
	}
	if is, err := prov.ListByFleetID(f.FleetID); err != nil || len(is) != 0 {
		t.Errorf("ListByFleetID must ignore soft-deleted rows, got %d err %v", len(is), err)
	}
}
```

- [ ] **Step 2: Run them and watch them fail**

Run: `go test ./apps/fleet-service/internal/admin/ -run Visibility -v` — or, since
the test names do not start with `Visibility`, run the whole package:
`go test ./apps/fleet-service/internal/admin/ -v`
Expected: FAIL — every assertion after the soft-delete, because no provider
filters `deleted_at` yet.

- [ ] **Step 3: Add the filters**

`mileage/provider.go` — one line:

```go
func (p *dbProvider) ListByVehicle(vehicleID string, from, to *time.Time, page server.Page) ([]Model, int, error) {
	q := p.db.Model(&Entity{}).Where("vehicle_id = ? AND deleted_at IS NULL", vehicleID)
```

`maintenanceschedule/provider.go` — four sites:

```go
func (p *dbProvider) GetByID(id string) (Model, error) {
	var e Entity
	if err := p.db.First(&e, "id = ? AND deleted_at IS NULL", id).Error; err != nil {
```

```go
func (p *dbProvider) ListByVehicle(vehicleID string, page server.Page) ([]Model, int, error) {
	q := p.db.Model(&Entity{}).Where("vehicle_id = ? AND deleted_at IS NULL", vehicleID)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var es []Entity
	if err := p.db.Where("vehicle_id = ? AND deleted_at IS NULL", vehicleID).
		Order("created_at asc").Offset(page.Offset()).Limit(page.Size).Find(&es).Error; err != nil {
		return nil, 0, err
	}
```

```go
// queryActive joins active schedules to their (non-deleted) vehicle …
//
// `s.deleted_at IS NULL` is not cosmetic: the no-argument form backs
// /internal/maintenance/due, which drives notification-service's reminder job.
// Without it a purged schedule keeps generating notifications for a fleet that
// no longer exists.
func (p *dbProvider) queryActive(fleetID, vehicleID *string) ([]QueueRow, error) {
	q := p.db.Table("fleet.maintenance_schedules AS s").
		Select("s.*, v.current_mileage AS current_mileage, v.fleet_id AS fleet_id").
		Joins("JOIN fleet.vehicles v ON v.id = s.vehicle_id AND v.deleted_at IS NULL").
		Where("s.active = ? AND s.deleted_at IS NULL", true)
```

`maintenanceschedule/administrator.go:132` — read the surrounding function and add
`AND deleted_at IS NULL` to its `Where` clause. It is the recompute/advance write
path; a purged schedule must not be recomputed.

`membership/provider.go` — five sites; append ` AND deleted_at IS NULL` to each
`Where`:

```go
	p.db.Where("user_id = ? AND status = ? AND deleted_at IS NULL", userID, "active")   // GetActiveByUserID
	p.db.Where("fleet_id = ? AND deleted_at IS NULL", fleetID)                          // ListByFleetID
	p.db.Where("fleet_id = ? AND status = ? AND deleted_at IS NULL", fleetID, "active") // ListActiveByFleetID
	p.db.Where("fleet_id = ? AND user_id = ? AND deleted_at IS NULL", fleetID, userID)  // GetByFleetAndUser
	// CountOwners feeds the sole-owner guard: counting a purged owner would let
	// the last live owner remove themselves.
	p.db.Model(&Entity{}).Where("fleet_id = ? AND role = ? AND deleted_at IS NULL", fleetID, "owner")
```

`invite/provider.go` — three sites:

```go
	p.db.First(&e, "id = ? AND deleted_at IS NULL", id)        // GetByID
	p.db.First(&e, "token = ? AND deleted_at IS NULL", token)  // GetByToken
	p.db.Where("fleet_id = ? AND deleted_at IS NULL", fleetID) // ListByFleetID
```

- [ ] **Step 4: Run the tests to green**

Run: `go test ./apps/fleet-service/... -count=1`
Expected: PASS, including the four new tests and every pre-existing test.

- [ ] **Step 5: Commit**

```bash
git add apps/fleet-service/internal
git commit -m "fix(fleet): filter soft-deleted rows in mileage, schedule, membership and invite reads"
```

---

### Task 5: data-visibility sweep — activity, maintenance-record documents, dashboards

**Files:**
- Modify: `apps/fleet-service/internal/activity/provider.go:29,33,55`
- Modify: `apps/fleet-service/internal/maintenancerecord/provider.go:43,83`
- Modify: `apps/fleet-service/internal/maintenancerecord/administrator.go:67`
- Modify: `apps/fleet-service/internal/dashboard/processor.go:94,103,169,173`
- Modify: `apps/fleet-service/internal/dashboard/aggregate.go:144`
- Test: `apps/fleet-service/internal/admin/visibility_activity_test.go` (create)
- Test: `apps/fleet-service/internal/admin/visibility_document_test.go` (create)
- Test: `apps/fleet-service/internal/admin/visibility_dashboard_test.go` (create)

**Interfaces:**
- Consumes: `admintest` from Task 3.
- Produces: `dashboard.Administrator.Replace` **revives** a soft-deleted layout
  instead of inserting a second row (design §6.4); everything else just filters.

**The dashboard revive, and why it is required.** `dashboards` had no real unique
index and its save path reads `First(&e)` with no ordering. If a fleet purge
soft-deletes a dashboard and the user then visits their dashboard, today's code
inserts a **second** row; a later cancel restores the first, leaving two live rows
and a non-deterministic read. The fix is small: look up including soft-deleted
rows and revive. A revived row simply leaves the pending operation — the user
re-created their layout, which is the outcome a later cancel would have produced
anyway. Widgets need no revive: they are hard-deleted and recreated on every save.

`dashboard/aggregate.go:144` is raw SQL over `fleet.mileage_records`, bypassing the
provider layer entirely — the second site design §9 names explicitly.

- [ ] **Step 1: Write the three failing tests**

Create `apps/fleet-service/internal/admin/visibility_activity_test.go`:

```go
package admin_test

import (
	"testing"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/activity"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin/admintest"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// LastActivityByVehicle feeds vehicle status derivation: a purged event that
// still counts as "recent activity" makes a dormant vehicle look healthy.
func TestActivityProvider_hidesSoftDeleted(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-1")
	prov := activity.NewProvider(db)
	page := server.Page{Number: 1, Size: 25}

	if rows, total, err := prov.ListByFleet(f.FleetID, page); err != nil || len(rows) != 1 || total != 1 {
		t.Fatalf("fixture expected one activity event, got %d/%d err %v", len(rows), total, err)
	}

	if err := db.Exec(`UPDATE fleet.activity_events SET deleted_at = CURRENT_TIMESTAMP,
	                   purge_operation_id = 'op-1' WHERE id = ?`, f.ActivityEventID).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	if rows, total, err := prov.ListByFleet(f.FleetID, page); err != nil || len(rows) != 0 || total != 0 {
		t.Errorf("ListByFleet must ignore soft-deleted events, got %d/%d err %v", len(rows), total, err)
	}
	if rows, total, err := prov.ListByVehicle(f.VehicleID, page); err != nil || len(rows) != 0 || total != 0 {
		t.Errorf("ListByVehicle must ignore soft-deleted events, got %d/%d err %v", len(rows), total, err)
	}
	last, err := prov.LastActivityByVehicle(f.VehicleID)
	if err != nil {
		t.Fatalf("last activity: %v", err)
	}
	if !last.IsZero() {
		t.Errorf("LastActivityByVehicle must ignore soft-deleted events, got %v", last)
	}
}
```

Create `apps/fleet-service/internal/admin/visibility_document_test.go`:

```go
package admin_test

import (
	"testing"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin/admintest"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/maintenancerecord"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// A purged attachment must vanish from the record it hangs off, on both the get
// and the list path — the list path batches documents in a separate query, so
// the two filters are genuinely separate code.
func TestMaintenanceRecordDocuments_hideSoftDeleted(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-1")
	prov := maintenancerecord.NewProvider(db)

	m, err := prov.GetByID(f.MaintenanceRecordID)
	if err != nil {
		t.Fatalf("fixture record should be readable: %v", err)
	}
	if len(m.DocumentMediaIDs()) != 1 {
		t.Fatalf("fixture expected one attached document, got %d", len(m.DocumentMediaIDs()))
	}

	if err := db.Exec(`UPDATE fleet.maintenance_record_documents SET deleted_at = CURRENT_TIMESTAMP,
	                   purge_operation_id = 'op-1' WHERE id = ?`, f.DocumentID).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	m, err = prov.GetByID(f.MaintenanceRecordID)
	if err != nil {
		t.Fatalf("get after soft delete: %v", err)
	}
	if len(m.DocumentMediaIDs()) != 0 {
		t.Errorf("GetByID still returns a soft-deleted document: %v", m.DocumentMediaIDs())
	}

	rows, _, err := prov.ListByVehicle(f.VehicleID, nil, server.Page{Number: 1, Size: 25})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected the record itself to remain listed, got %d", len(rows))
	}
	if len(rows[0].DocumentMediaIDs()) != 0 {
		t.Errorf("ListByVehicle still returns a soft-deleted document: %v", rows[0].DocumentMediaIDs())
	}
}
```

Create `apps/fleet-service/internal/admin/visibility_dashboard_test.go`:

```go
package admin_test

import (
	"testing"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin/admintest"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/dashboard"
)

func TestDashboardProvider_hidesSoftDeleted(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-1")
	prov := dashboard.NewProvider(db)

	d, err := prov.GetDashboard(f.FleetID, f.OwnerUserID)
	if err != nil {
		t.Fatalf("fixture dashboard should be readable: %v", err)
	}
	if len(d.Widgets()) != 1 {
		t.Fatalf("fixture expected one widget, got %d", len(d.Widgets()))
	}

	if err := db.Exec(`UPDATE fleet.dashboards SET deleted_at = CURRENT_TIMESTAMP,
	                   purge_operation_id = 'op-1' WHERE id = ?`, f.DashboardID).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	d, err = prov.GetDashboard(f.FleetID, f.OwnerUserID)
	if err != nil {
		t.Fatalf("get after soft delete: %v", err)
	}
	if len(d.Widgets()) != 0 {
		t.Errorf("a soft-deleted dashboard must read as an empty layout, got %d widgets", len(d.Widgets()))
	}
}

// design §6.4: saving a layout while the dashboard row is soft-deleted must
// REVIVE that row, not insert a second one. Two live rows for one (fleet, user)
// makes the read non-deterministic and survives a later cancel.
func TestDashboardAdministrator_revivesSoftDeletedLayout(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-1")
	adm := dashboard.NewAdministrator(db)

	if err := db.Exec(`UPDATE fleet.dashboards SET deleted_at = CURRENT_TIMESTAMP,
	                   purge_operation_id = 'op-1' WHERE id = ?`, f.DashboardID).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	saved, err := adm.Replace(f.FleetID, f.OwnerUserID, []dashboard.WidgetInput{
		{Type: "vehicle-status", PositionX: 0, PositionY: 0, Width: 2, Height: 2},
	})
	if err != nil {
		t.Fatalf("replace after soft delete: %v", err)
	}
	if saved.ID() != f.DashboardID {
		t.Errorf("Replace inserted a new dashboard row %q instead of reviving %q", saved.ID(), f.DashboardID)
	}
	if got := admintest.CountRows(t, db, "fleet.dashboards"); got != 1 {
		t.Errorf("expected exactly one dashboard row after revive, got %d", got)
	}
	if got := admintest.CountLive(t, db, "fleet.dashboards"); got != 1 {
		t.Errorf("the revived dashboard must be live, got %d live rows", got)
	}

	var opID *string
	if err := db.Raw(`SELECT purge_operation_id FROM fleet.dashboards WHERE id = ?`, f.DashboardID).
		Scan(&opID).Error; err != nil {
		t.Fatalf("read purge_operation_id: %v", err)
	}
	if opID != nil {
		t.Errorf("a revived dashboard must leave its purge operation, got %v", *opID)
	}
}
```

- [ ] **Step 2: Run them and watch them fail**

Run: `go test ./apps/fleet-service/internal/admin/ -count=1 -v`
Expected: FAIL on the new assertions.

- [ ] **Step 3: Add the activity and document filters**

`activity/provider.go`:

```go
func (p *dbProvider) ListByFleet(fleetID string, page server.Page) ([]Model, int, error) {
	return p.list(p.db.Where("fleet_id = ? AND deleted_at IS NULL", fleetID), page)
}

func (p *dbProvider) ListByVehicle(vehicleID string, page server.Page) ([]Model, int, error) {
	return p.list(p.db.Where("vehicle_id = ? AND deleted_at IS NULL", vehicleID), page)
}
```

```go
// LastActivityByVehicle feeds status.Derive's inactivity check, so a purged
// event that still counted here would make a dormant vehicle look healthy.
func (p *dbProvider) LastActivityByVehicle(vehicleID string) (time.Time, error) {
	var e Entity
	err := p.db.Model(&Entity{}).
		Where("vehicle_id = ? AND deleted_at IS NULL", vehicleID).
```

`maintenancerecord/provider.go` — both document lookups:

```go
	if err := p.db.Where("maintenance_record_id = ? AND deleted_at IS NULL", e.ID).Find(&docs).Error; err != nil {  // :43
	...
		if err := p.db.Where("maintenance_record_id IN ? AND deleted_at IS NULL", ids).Find(&docs).Error; err != nil {  // :83
```

`maintenancerecord/administrator.go:67` — read the surrounding function (the
document replace-on-update path) and add `AND deleted_at IS NULL` so it does not
hard-delete rows another operation has stamped.

- [ ] **Step 4: Add the dashboard filters and the revive**

`dashboard/processor.go` — the read path (`:92-107`):

```go
func (p *dbProvider) GetDashboard(fleetID, userID string) (Dashboard, error) {
	var e DashboardEntity
	err := p.db.Where("fleet_id = ? AND user_id = ? AND deleted_at IS NULL", fleetID, userID).First(&e).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			// No layout saved yet — return an empty dashboard (not an error).
			// A soft-deleted layout takes this branch too, which is right: to an
			// ordinary user a purged dashboard is indistinguishable from one
			// they never saved.
			return Dashboard{fleetID: fleetID, userID: userID}, nil
		}
		return Dashboard{}, err
	}
	var ws []WidgetEntity
	if err := p.db.Where("dashboard_id = ? AND deleted_at IS NULL", e.ID).Find(&ws).Error; err != nil {
		return Dashboard{}, err
	}
	return MakeDashboard(e, ws), nil
}
```

`dashboard/processor.go` — the write path (`:117-179`). Replace the upsert block:

```go
// Replace upserts the dashboard row by (fleet_id, user_id), then deletes and
// re-inserts the widget set inside ONE db.Transaction.
//
// The lookup deliberately INCLUDES soft-deleted rows. A fleet purge stamps the
// dashboard; if the user then saves a layout, inserting a second row would leave
// two live rows for one (fleet, user) once the purge is cancelled, and the read
// above — First with no ordering — would pick between them non-deterministically.
// Reviving instead is also the better outcome: the user re-created their layout,
// which is exactly what a later cancel would have produced (design §6.4).
func (a *dbAdministrator) Replace(fleetID, userID string, widgets []WidgetInput) (Dashboard, error) {
	var result Dashboard
	err := a.db.Transaction(func(tx *gorm.DB) error {
		var e DashboardEntity
		dbErr := tx.Where("fleet_id = ? AND user_id = ?", fleetID, userID).First(&e).Error
		if dbErr != nil {
			if dbErr != gorm.ErrRecordNotFound {
				return dbErr
			}
			e = DashboardEntity{ID: uuid.NewString(), FleetID: fleetID, UserID: userID}
			if err := tx.Create(&e).Error; err != nil {
				return err
			}
		} else {
			// Touch updated_at and revive: clearing both columns together is the
			// same shape the manifest's Restore uses, so a revived row is
			// indistinguishable from a restored one.
			if err := tx.Model(&DashboardEntity{}).Where("id = ?", e.ID).Updates(map[string]any{
				"updated_at":         gorm.Expr("CURRENT_TIMESTAMP"),
				"deleted_at":         nil,
				"purge_operation_id": nil,
			}).Error; err != nil {
				return err
			}
		}

		// Widgets are hard-deleted and recreated on every save, so their
		// deleted_at only ever matters between a stamp and its cancel.
		if err := tx.Where("dashboard_id = ?", e.ID).Delete(&WidgetEntity{}).Error; err != nil {
			return err
		}

		wes := make([]WidgetEntity, 0, len(widgets))
		for _, w := range widgets {
			wes = append(wes, WidgetEntity{
				ID:          uuid.NewString(),
				DashboardID: e.ID,
				Type:        w.Type,
				PositionX:   w.PositionX,
				PositionY:   w.PositionY,
				Width:       w.Width,
				Height:      w.Height,
				Config:      w.Config,
			})
		}
		if len(wes) > 0 {
			if err := tx.Create(&wes).Error; err != nil {
				return err
			}
		}

		if err := tx.Where("fleet_id = ? AND user_id = ? AND deleted_at IS NULL", fleetID, userID).
			First(&e).Error; err != nil {
			return err
		}
		var finalWs []WidgetEntity
		if err := tx.Where("dashboard_id = ? AND deleted_at IS NULL", e.ID).Find(&finalWs).Error; err != nil {
			return err
		}
		result = MakeDashboard(e, finalWs)
		return nil
	})
	return result, err
}
```

`dashboard/aggregate.go:144` — the raw mileage query:

```go
	// deleted_at IS NULL is hand-written here because this query bypasses the
	// mileage provider entirely (design §9).
	q := p.db.Table("fleet.mileage_records").
		Select("recorded_at, mileage, source").
		Where("vehicle_id = ? AND deleted_at IS NULL", vehicleID)
```

`aggregate.go:66-91` already filters `deleted_at` for vehicles, maintenance
records and fuel logs — leave those untouched.

- [ ] **Step 5: Run the tests to green**

Run: `go test ./apps/fleet-service/... -count=1`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add apps/fleet-service/internal
git commit -m "fix(fleet): filter soft-deleted rows in activity, documents and dashboards; revive purged layouts"
```

---

### Task 6: data-visibility sweep — notification, preferences, media variants

**Files:**
- Modify: `apps/notification-service/internal/notification/provider.go:38`
- Modify: `apps/notification-service/internal/notification/administrator.go:26,54,66,77`
- Modify: `apps/notification-service/internal/preferences/provider.go:27,38`
- Modify: `apps/media-service/internal/mediavariant/provider.go:39,53`
- Modify: `apps/media-service/internal/mediaobject/purge.go:24`
- Test: `apps/notification-service/internal/notification/visibility_test.go` (create)
- Test: `apps/media-service/internal/mediavariant/visibility_test.go` (create)
- Test: `apps/media-service/internal/mediaobject/purge_test.go` (create)

**Interfaces:**
- Consumes: the columns from Task 2.
- Produces: `notification.Administrator.ExistsByDedupeKey` filters
  `deleted_at IS NULL`; `mediaobject.ListPurgeable` excludes admin-stamped rows.

**Two things here are more than filters.** `ExistsByDedupeKey` counting purged rows
would permanently suppress a notification's own replacement even after the partial
index freed the key (design F1) — the index and the query have to agree.
And `mediaobject.ListPurgeable` gains `purge_operation_id IS NULL`: the
"never write `purge_after`" rule already keeps admin-stamped rows out of this
sweep, but FR-ADMIN-RESTORE-7 asks for the narrowing explicitly and it costs one
clause (design F3).

- [ ] **Step 1: Write the failing tests**

Create `apps/notification-service/internal/notification/visibility_test.go`:

```go
package notification

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

func newVisibilityDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec("ATTACH DATABASE ':memory:' AS notification").Error; err != nil {
		t.Fatalf("attach notification schema: %v", err)
	}
	ddl := `CREATE TABLE notification.notifications (
		id TEXT PRIMARY KEY, user_id TEXT, type TEXT, title TEXT, body TEXT,
		dedupe_key TEXT, vehicle_id TEXT, fleet_id TEXT, read_at DATETIME,
		created_at DATETIME, deleted_at DATETIME, purge_operation_id TEXT)`
	if err := db.Exec(ddl).Error; err != nil {
		t.Fatalf("ddl: %v", err)
	}
	if err := db.Exec(`INSERT INTO notification.notifications
		(id, user_id, type, title, dedupe_key, fleet_id, created_at)
		VALUES ('n1', 'user-1', 'schedule.overdue', 'Oil change due', 'dk-1', 'fleet-1', CURRENT_TIMESTAMP)`).
		Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	return db
}

func TestNotificationReads_hideSoftDeleted(t *testing.T) {
	db := newVisibilityDB(t)
	prov := NewProvider(db)
	adm := NewAdministrator(db)
	page := server.Page{Number: 1, Size: 25}

	if rows, total, err := prov.ListByUser("user-1", ListFilter{}, page); err != nil || len(rows) != 1 || total != 1 {
		t.Fatalf("fixture expected one notification, got %d/%d err %v", len(rows), total, err)
	}
	if exists, err := adm.ExistsByDedupeKey("dk-1"); err != nil || !exists {
		t.Fatalf("fixture dedupe key should exist, got %v err %v", exists, err)
	}

	if err := db.Exec(`UPDATE notification.notifications SET deleted_at = CURRENT_TIMESTAMP,
	                   purge_operation_id = 'op-1' WHERE id = 'n1'`).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	if rows, total, err := prov.ListByUser("user-1", ListFilter{}, page); err != nil || len(rows) != 0 || total != 0 {
		t.Errorf("ListByUser must ignore soft-deleted rows, got %d/%d err %v", len(rows), total, err)
	}
	// design F1: the partial index frees the key, but ExistsByDedupeKey would
	// still veto regeneration unless it agrees with the index.
	if exists, err := adm.ExistsByDedupeKey("dk-1"); err != nil || exists {
		t.Errorf("ExistsByDedupeKey must ignore soft-deleted rows, got %v err %v", exists, err)
	}
	if err := adm.MarkRead("user-1", "n1"); err != ErrNotFound {
		t.Errorf("MarkRead on a soft-deleted notification must be ErrNotFound, got %v", err)
	}
}
```

Create `apps/media-service/internal/mediavariant/visibility_test.go`:

```go
package mediavariant

import (
	"context"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newVariantVisibilityDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec("ATTACH DATABASE ':memory:' AS media").Error; err != nil {
		t.Fatalf("attach media schema: %v", err)
	}
	ddl := `CREATE TABLE media.media_variants (
		id TEXT PRIMARY KEY, media_object_id TEXT, variant TEXT, object_key TEXT,
		width INTEGER, height INTEGER, content_type TEXT, created_at DATETIME,
		deleted_at DATETIME, purge_operation_id TEXT)`
	if err := db.Exec(ddl).Error; err != nil {
		t.Fatalf("ddl: %v", err)
	}
	if err := db.Exec(`INSERT INTO media.media_variants
		(id, media_object_id, variant, object_key, content_type)
		VALUES ('v1', 'mo-1', 'thumb', 'k/thumb', 'image/webp')`).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	return db
}

func TestMediaVariantReads_hideSoftDeleted(t *testing.T) {
	db := newVariantVisibilityDB(t)
	prov := NewProvider(db)
	ctx := context.Background()

	if vs, err := prov.ListByMediaObject("mo-1"); err != nil || len(vs) != 1 {
		t.Fatalf("fixture expected one variant, got %d err %v", len(vs), err)
	}

	if err := db.Exec(`UPDATE media.media_variants SET deleted_at = CURRENT_TIMESTAMP,
	                   purge_operation_id = 'op-1' WHERE id = 'v1'`).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	if vs, err := prov.ListByMediaObject("mo-1"); err != nil || len(vs) != 0 {
		t.Errorf("ListByMediaObject must ignore soft-deleted rows, got %d err %v", len(vs), err)
	}
	if _, err := prov.GetByMediaObjectAndVariant(ctx, "mo-1", Variant("thumb")); err != ErrNotFound {
		t.Errorf("GetByMediaObjectAndVariant must ignore soft-deleted rows, got %v", err)
	}
}
```

- [ ] **Step 2: Run them and watch them fail**

Run: `go test ./apps/notification-service/internal/notification/ ./apps/media-service/internal/mediavariant/ -count=1 -v`
Expected: FAIL on the post-soft-delete assertions.

- [ ] **Step 3: Add the filters**

`notification/provider.go:39`:

```go
	q := p.db.Model(&Entity{}).Where("user_id = ? AND deleted_at IS NULL", userID)
```

`notification/administrator.go` — four sites:

```go
// ExistsByDedupeKey ignores soft-deleted rows. The partial unique index frees a
// purged notification's key so it CAN be regenerated (design F1); this query has
// to agree, or the reminder safety-net and event redelivery would keep vetoing
// the replacement forever.
func (a *dbAdministrator) ExistsByDedupeKey(k string) (bool, error) {
	var count int64
	if err := a.db.Model(&Entity{}).
		Where("dedupe_key = ? AND deleted_at IS NULL", k).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}
```

```go
	res := a.db.Model(&Entity{}).
		Where("id = ? AND user_id = ? AND read_at IS NULL AND deleted_at IS NULL", id, userID).
		Update("read_at", now)                                                                    // MarkRead :56
	...
		if err := a.db.Model(&Entity{}).
			Where("id = ? AND user_id = ? AND deleted_at IS NULL", id, userID).
			Count(&count).Error; err != nil {                                                     // MarkRead :66
	...
	return a.db.Model(&Entity{}).
		Where("user_id = ? AND read_at IS NULL AND deleted_at IS NULL", userID).
		Update("read_at", now).Error                                                              // MarkAllRead :79
```

`preferences/provider.go` — both sites:

```go
	p.db.Where("user_id = ? AND type = ? AND deleted_at IS NULL", userID, typ)  // GetByUserAndType
	p.db.Where("user_id = ? AND deleted_at IS NULL", userID)                    // ListByUser
```

`mediavariant/provider.go` — both sites:

```go
	p.db.Where("media_object_id = ? AND deleted_at IS NULL", mediaObjectID)                        // ListByMediaObject
	p.db.WithContext(ctx).
		Where("media_object_id = ? AND variant = ? AND deleted_at IS NULL", mediaObjectID, string(v))  // GetByMediaObjectAndVariant
```

- [ ] **Step 4: Narrow the media purge sweep (FR-ADMIN-RESTORE-7 / design F3)**

Write the sweep-isolation test first —
`apps/media-service/internal/mediaobject/purge_test.go`:

```go
package mediaobject

import (
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newPurgeDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec("ATTACH DATABASE ':memory:' AS media").Error; err != nil {
		t.Fatalf("attach media schema: %v", err)
	}
	ddl := `CREATE TABLE media.media_objects (
		id TEXT PRIMARY KEY, fleet_id TEXT, uploaded_by_user_id TEXT, bucket TEXT,
		object_key TEXT, content_type TEXT, size INTEGER, original_filename TEXT,
		status TEXT, created_at DATETIME, deleted_at DATETIME, purge_after DATETIME,
		purge_operation_id TEXT)`
	if err := db.Exec(ddl).Error; err != nil {
		t.Fatalf("ddl: %v", err)
	}
	return db
}

// FR-ADMIN-RESTORE-7 / design F3. An admin-stamped object belongs to a
// cancellable operation whose lifecycle the admin reaper owns; the legacy sweep
// must not hard-delete it — and, worse, must not remove its MinIO object, which
// no restore could bring back.
func TestListPurgeable_skipsAdminStampedObjects(t *testing.T) {
	db := newPurgeDB(t)
	past := time.Now().UTC().Add(-time.Hour)
	seed := `INSERT INTO media.media_objects (id, fleet_id, bucket, object_key, status,
	         deleted_at, purge_after, purge_operation_id) VALUES (?, 'f1', 'media', ?, 'ready', ?, ?, ?)`
	if err := db.Exec(seed, "mo-user", "k/user", past, past, nil).Error; err != nil {
		t.Fatalf("seed user-deleted object: %v", err)
	}
	// purge_after is set here deliberately: an admin stamp never writes it, so
	// this row could only exist if someone later "helpfully" did. The explicit
	// purge_operation_id IS NULL narrowing is what still saves it.
	if err := db.Exec(seed, "mo-admin", "k/admin", past, past, "op-1").Error; err != nil {
		t.Fatalf("seed admin-stamped object: %v", err)
	}

	got, err := ListPurgeable(db)
	if err != nil {
		t.Fatalf("list purgeable: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want exactly one purgeable object, got %d", len(got))
	}
	if got[0].ID() != "mo-user" {
		t.Errorf("the legacy sweep picked up an admin-stamped object: %q", got[0].ID())
	}
}
```

Run it and watch it fail (`want exactly one purgeable object, got 2`), then make
the change to `apps/media-service/internal/mediaobject/purge.go`:

```go
// ListPurgeable returns soft-deleted objects whose purge window has elapsed.
// The purge job uses these to remove both the rows and the MinIO objects.
//
// purge_operation_id IS NULL keeps this sweep off rows an admin purge stamped.
// An admin stamp never writes purge_after, so such rows could not match anyway
// — this is the explicit belt to that structural brace (FR-ADMIN-RESTORE-7),
// and it is what makes the guarantee survive someone later "helpfully" setting
// purge_after in the admin path.
func ListPurgeable(db *gorm.DB) ([]Model, error) {
	var es []Entity
	if err := db.Where("purge_after IS NOT NULL AND purge_after < now() AND purge_operation_id IS NULL").
		Find(&es).Error; err != nil {
		return nil, err
	}
	...
```

- [ ] **Step 5: Run the tests to green**

Run: `go test ./apps/notification-service/... ./apps/media-service/... -count=1`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add apps/notification-service apps/media-service
git commit -m "fix(notification,media): filter soft-deleted rows and narrow the media purge sweep"
```

---

### Task 7: the purge manifest and its four operations

**Files:**
- Create: `apps/fleet-service/internal/admin/manifest.go`
- Create: `apps/fleet-service/internal/admin/operations.go`
- Create: `apps/fleet-service/internal/admin/operations_test.go`
- Create: `apps/fleet-service/internal/admin/arch_test.go`

**Interfaces:**
- Consumes: `admintest.NewDB`, `admintest.SeedFleet`.
- Produces, all in package `admin`:
  - `type Scope string`; `ScopeSystem`, `ScopeFleet`, `ScopeRecord`.
  - Target-type constants `TargetVehicle`, `TargetMaintenanceRecord`,
    `TargetMaintenanceSchedule`, `TargetFuelLog`, `TargetMileageRecord`,
    `TargetMembership`, `TargetInvite`, `TargetVehicleMedia`,
    `TargetActivityEvent`; `ValidTargetTypes map[string]bool`.
  - `type Root struct { Scope Scope; TargetType string; TargetID string }`
  - `type OrphanRule struct { Column, ParentTable string }`
  - `type Target struct { Key, Table string; Orphan *OrphanRule; Where func(Root) (string, []any) }`
  - `var Manifest []Target`, `var excludedTables map[string]string`
  - `func Count(db *gorm.DB, root Root) (map[string]int, error)`
  - `func CountByOperation(db *gorm.DB, opID string) (map[string]int, error)`
  - `func Stamp(tx *gorm.DB, root Root, opID string, now time.Time) (map[string]int, error)`
  - `func Restore(tx *gorm.DB, opID string) error`
  - `func Reap(tx *gorm.DB, opID string) (map[string]int, error)`

- [ ] **Step 1: Write the failing operations test**

Create `apps/fleet-service/internal/admin/operations_test.go`:

```go
package admin_test

import (
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin/admintest"
)

var testNow = time.Date(2026, 8, 2, 14, 3, 11, 0, time.UTC)

// everyPurgeableTable is the assertion surface for "zero orphans". It is
// spelled out rather than derived from the manifest on purpose: deriving it
// would make the test agree with the manifest by construction, which is exactly
// the property under test.
var everyPurgeableTable = []string{
	"fleet.fleets", "fleet.vehicles", "fleet.fleet_memberships", "fleet.fleet_invites",
	"fleet.mileage_records", "fleet.fuel_logs", "fleet.maintenance_records",
	"fleet.maintenance_record_documents", "fleet.maintenance_schedules",
	"fleet.vehicle_media", "fleet.activity_events", "fleet.dashboards",
	"fleet.dashboard_widgets",
}

// FR-ADMIN-PURGE-4: a fleet purge leaves ZERO live rows anywhere beneath the
// fleet, and leaves every other fleet completely untouched.
func TestStamp_fleetScope_cascadesWithNoOrphans(t *testing.T) {
	db := admintest.NewDB(t)
	target := admintest.SeedFleet(t, db, "fleet-1")
	admintest.SeedFleet(t, db, "fleet-2")

	root := admin.Root{Scope: admin.ScopeFleet, TargetID: target.FleetID}

	counts, err := admin.Count(db, root)
	if err != nil {
		t.Fatalf("count: %v", err)
	}

	var stamped map[string]int
	if err := db.Transaction(func(tx *gorm.DB) error {
		var serr error
		stamped, serr = admin.Stamp(tx, root, "op-1", testNow)
		return serr
	}); err != nil {
		t.Fatalf("stamp: %v", err)
	}

	// FR-ADMIN-UI-9: the blast-radius numbers and the purge's affected rows are
	// the same query, so they must be equal for every key.
	for key, want := range counts {
		if got := stamped[key]; got != want {
			t.Errorf("%s: blast radius said %d, purge stamped %d", key, want, got)
		}
	}

	for _, table := range everyPurgeableTable {
		if got := admintest.CountLive(t, db, table); got != 0 {
			t.Errorf("%s still has %d live rows after a fleet purge", table, got)
		}
	}
	// The other fleet is untouched. Counting total rows in the second fleet is
	// the cross-tenant assertion: a predicate that resolved too widely would
	// stamp them.
	var liveElsewhere int64
	if err := db.Raw(`SELECT count(*) FROM fleet.vehicles
	                  WHERE fleet_id = 'fleet-2' AND deleted_at IS NULL`).Scan(&liveElsewhere).Error; err != nil {
		t.Fatalf("count other fleet: %v", err)
	}
	if liveElsewhere != 2 {
		t.Errorf("a fleet purge touched another fleet: %d of 2 vehicles remain live", liveElsewhere)
	}
	// Seeded reference data is a PRD non-goal and must survive.
	if got := admintest.CountRows(t, db, "fleet.maintenance_categories"); got != 1 {
		t.Errorf("maintenance categories must survive a purge, got %d rows", got)
	}
}

// FR-ADMIN-PURGE-3: rows already soft-deleted by ordinary product flows carry a
// NULL purge_operation_id and must be left alone by the stamp — and therefore
// must NOT come back when the operation is cancelled (FR-ADMIN-RESTORE-3).
func TestStampAndRestore_leaveIndependentlyDeletedRowsDeleted(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-1")

	if err := db.Exec(`UPDATE fleet.fuel_logs SET deleted_at = ? WHERE id = ?`,
		testNow.Add(-24*time.Hour), f.FuelLogID).Error; err != nil {
		t.Fatalf("pre-existing user delete: %v", err)
	}

	root := admin.Root{Scope: admin.ScopeFleet, TargetID: f.FleetID}
	if err := db.Transaction(func(tx *gorm.DB) error {
		_, serr := admin.Stamp(tx, root, "op-1", testNow)
		return serr
	}); err != nil {
		t.Fatalf("stamp: %v", err)
	}

	var opID *string
	if err := db.Raw(`SELECT purge_operation_id FROM fleet.fuel_logs WHERE id = ?`, f.FuelLogID).
		Scan(&opID).Error; err != nil {
		t.Fatalf("read purge_operation_id: %v", err)
	}
	if opID != nil {
		t.Fatalf("the stamp claimed a row the user had already deleted (op %v)", *opID)
	}

	if err := db.Transaction(func(tx *gorm.DB) error { return admin.Restore(tx, "op-1") }); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if got := admintest.CountLive(t, db, "fleet.fuel_logs"); got != 0 {
		t.Errorf("cancelling a purge resurrected a row the user had deleted: %d live", got)
	}
	// Everything the operation DID stamp comes back.
	if got := admintest.CountLive(t, db, "fleet.vehicles"); got != 2 {
		t.Errorf("restore did not return the vehicles: %d live of 2", got)
	}
	if got := admintest.CountLive(t, db, "fleet.fleets"); got != 1 {
		t.Errorf("restore did not return the fleet: %d live of 1", got)
	}
}

// FR-ADMIN-PURGE-10: a replayed stamp changes nothing and returns the SAME
// counts as the first call, not zeros. That is what makes the retry endpoint
// safe to press repeatedly AND leaves affected_counts correct afterwards.
func TestStamp_isIdempotent(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-1")
	root := admin.Root{Scope: admin.ScopeFleet, TargetID: f.FleetID}

	var first, second map[string]int
	run := func(now time.Time) map[string]int {
		var out map[string]int
		if err := db.Transaction(func(tx *gorm.DB) error {
			var serr error
			out, serr = admin.Stamp(tx, root, "op-1", now)
			return serr
		}); err != nil {
			t.Fatalf("stamp: %v", err)
		}
		return out
	}
	first = run(testNow)
	second = run(testNow.Add(time.Hour))

	for key, want := range first {
		if got := second[key]; got != want {
			t.Errorf("%s: replay returned %d, first call returned %d", key, got, want)
		}
	}
	var stampedAt time.Time
	if err := db.Raw(`SELECT deleted_at FROM fleet.vehicles WHERE id = ?`, f.VehicleID).
		Scan(&stampedAt).Error; err != nil {
		t.Fatalf("read deleted_at: %v", err)
	}
	if !stampedAt.UTC().Equal(testNow) {
		t.Errorf("a replayed stamp rewrote deleted_at: %v, want %v", stampedAt.UTC(), testNow)
	}
}

// scope:record with target_type:vehicle cascades to that vehicle's children and
// to nothing else (FR-ADMIN-PURGE-5).
func TestStamp_recordScopeVehicle_cascadesToChildrenOnly(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-1")

	root := admin.Root{Scope: admin.ScopeRecord, TargetType: admin.TargetVehicle, TargetID: f.VehicleID}
	if err := db.Transaction(func(tx *gorm.DB) error {
		_, serr := admin.Stamp(tx, root, "op-1", testNow)
		return serr
	}); err != nil {
		t.Fatalf("stamp: %v", err)
	}

	for _, table := range []string{
		"fleet.mileage_records", "fleet.fuel_logs", "fleet.maintenance_records",
		"fleet.maintenance_record_documents", "fleet.maintenance_schedules", "fleet.vehicle_media",
	} {
		if got := admintest.CountLive(t, db, table); got != 0 {
			t.Errorf("%s still has %d live rows beneath the purged vehicle", table, got)
		}
	}
	if got := admintest.CountLive(t, db, "fleet.vehicles"); got != 1 {
		t.Errorf("expected the sibling vehicle to survive, %d live of 1", got)
	}
	if got := admintest.CountLive(t, db, "fleet.fleets"); got != 1 {
		t.Errorf("a record purge must not touch the fleet, %d live", got)
	}
	if got := admintest.CountLive(t, db, "fleet.fleet_memberships"); got != 1 {
		t.Errorf("a record purge must not touch memberships, %d live", got)
	}
}

// Reap is keyed purely on purge_operation_id, so it is idempotent and order
// independent (FR-ADMIN-RESTORE-6).
func TestReap_hardDeletesAndIsIdempotent(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-1")
	root := admin.Root{Scope: admin.ScopeFleet, TargetID: f.FleetID}

	if err := db.Transaction(func(tx *gorm.DB) error {
		_, serr := admin.Stamp(tx, root, "op-1", testNow)
		return serr
	}); err != nil {
		t.Fatalf("stamp: %v", err)
	}

	deleted, err := admin.Reap(db, "op-1")
	if err != nil {
		t.Fatalf("reap: %v", err)
	}
	if deleted["vehicles"] != 2 {
		t.Errorf("reap reported %d vehicles deleted, want 2", deleted["vehicles"])
	}
	for _, table := range everyPurgeableTable {
		if got := admintest.CountRows(t, db, table); got != 0 {
			t.Errorf("%s still has %d rows after reap", table, got)
		}
	}

	again, err := admin.Reap(db, "op-1")
	if err != nil {
		t.Fatalf("second reap must succeed, got %v", err)
	}
	for key, n := range again {
		if n != 0 {
			t.Errorf("second reap deleted %d more %s rows", n, key)
		}
	}
}

// design §3.4: the ordering rule is a property, not a convention. Stamping in
// the exact reverse of the manifest's order must produce the same result.
func TestStamp_isOrderIndependent(t *testing.T) {
	forward := admintest.NewDB(t)
	f1 := admintest.SeedFleet(t, forward, "fleet-1")
	if err := forward.Transaction(func(tx *gorm.DB) error {
		_, serr := admin.Stamp(tx, admin.Root{Scope: admin.ScopeFleet, TargetID: f1.FleetID}, "op-1", testNow)
		return serr
	}); err != nil {
		t.Fatalf("forward stamp: %v", err)
	}

	reverse := admintest.NewDB(t)
	f2 := admintest.SeedFleet(t, reverse, "fleet-1")
	if err := reverse.Transaction(func(tx *gorm.DB) error {
		return admin.StampReversedForTest(tx, admin.Root{Scope: admin.ScopeFleet, TargetID: f2.FleetID}, "op-1", testNow)
	}); err != nil {
		t.Fatalf("reverse stamp: %v", err)
	}

	for _, table := range everyPurgeableTable {
		if a, b := admintest.CountLive(t, forward, table), admintest.CountLive(t, reverse, table); a != b {
			t.Errorf("%s: forward order left %d live, reverse order left %d — the cascade depends on manifest order", table, a, b)
		}
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test ./apps/fleet-service/internal/admin/ -count=1 -v`
Expected: FAIL — `undefined: admin.Root`, `admin.Count`, …

- [ ] **Step 3: Write the manifest**

Create `apps/fleet-service/internal/admin/manifest.go`:

```go
// Package admin owns fleet-service's platform-admin surface: the purge manifest
// and its four operations, the purge-operation lifecycle, the audit log, and the
// /admin HTTP tree.
//
// Nothing outside this package may read auth.Identity.PlatformAdmin, and nothing
// inside it may call authz.RequireSameFleet. arch_test.go enforces both, because
// the whole safety argument for a cross-fleet API is that it lives in a parallel
// tree rather than in a relaxed guard.
package admin

import "time"

// Scope is the blast radius of a purge operation.
type Scope string

const (
	ScopeSystem Scope = "system"
	ScopeFleet  Scope = "fleet"
	ScopeRecord Scope = "record"
)

// ValidScopes is the whitelist POST /admin/purge-operations validates against.
// Anything else is 422 (FR-ADMIN-PURGE-2).
var ValidScopes = map[Scope]bool{ScopeSystem: true, ScopeFleet: true, ScopeRecord: true}

// Target types accepted by scope:record (FR-ADMIN-PURGE-2).
const (
	TargetVehicle             = "vehicle"
	TargetMaintenanceRecord   = "maintenance_record"
	TargetMaintenanceSchedule = "maintenance_schedule"
	TargetFuelLog             = "fuel_log"
	TargetMileageRecord       = "mileage_record"
	TargetMembership          = "membership"
	TargetInvite              = "invite"
	TargetVehicleMedia        = "vehicle_media"
	TargetActivityEvent       = "activity_event"
)

// ValidTargetTypes is the whitelist for scope:record.
var ValidTargetTypes = map[string]bool{
	TargetVehicle: true, TargetMaintenanceRecord: true, TargetMaintenanceSchedule: true,
	TargetFuelLog: true, TargetMileageRecord: true, TargetMembership: true,
	TargetInvite: true, TargetVehicleMedia: true, TargetActivityEvent: true,
}

// Root is what a purge is rooted at: the whole system, one fleet, or one record.
type Root struct {
	Scope      Scope
	TargetType string // record scope only
	TargetID   string // fleet id or record id; empty for system scope
}

// OrphanRule describes how to detect a row whose parent no longer exists. It is
// nil for tables with no single hard parent (fleets; activity_events, whose
// vehicle_id is nullable and whose real owner is the fleet).
type OrphanRule struct {
	Column      string // FK column on this table
	ParentTable string // the table Column references
}

// Target is one purgeable table and how to resolve its rows from a purge root.
type Target struct {
	// Key is the name used in affected_counts JSON and in the console's
	// blast-radius panel. It is API surface: renaming one is a breaking change.
	Key    string
	Table  string
	Orphan *OrphanRule
	// Where returns the SQL predicate + args selecting this table's rows for a
	// given root, or ("", nil) when the table is out of scope for that root.
	//
	// It NEVER filters deleted_at — not on this table (the operations add that
	// guard) and not on any parent it references. Filtering a parent's
	// deleted_at is what would make the cascade order-dependent: stamping a
	// vehicle would hide its own children from the next predicate. See §3.4 of
	// design.md; TestStamp_isOrderIndependent is the enforcement.
	Where func(root Root) (string, []any)
}

// all matches every row in the table — the system-scope predicate. "1 = 1"
// rather than an empty string because the operations wrap the predicate in
// parentheses and an empty one would produce invalid SQL.
const all = "1 = 1"

// vehicleChild builds a Where for a table keyed to fleet.vehicles by col, whose
// rows are additionally addressable as their own record type (selfType, "" if
// the table is not a record target).
func vehicleChild(col, selfType string) func(Root) (string, []any) {
	return func(r Root) (string, []any) {
		switch r.Scope {
		case ScopeSystem:
			return all, nil
		case ScopeFleet:
			return col + " IN (SELECT id FROM fleet.vehicles WHERE fleet_id = ?)", []any{r.TargetID}
		case ScopeRecord:
			if selfType != "" && r.TargetType == selfType {
				return "id = ?", []any{r.TargetID}
			}
			if r.TargetType == TargetVehicle {
				return col + " = ?", []any{r.TargetID}
			}
		}
		return "", nil
	}
}

// fleetChild builds a Where for a table keyed directly to fleet.fleets by col.
func fleetChild(col, selfType string) func(Root) (string, []any) {
	return func(r Root) (string, []any) {
		switch r.Scope {
		case ScopeSystem:
			return all, nil
		case ScopeFleet:
			return col + " = ?", []any{r.TargetID}
		case ScopeRecord:
			if selfType != "" && r.TargetType == selfType {
				return "id = ?", []any{r.TargetID}
			}
		}
		return "", nil
	}
}

// Manifest is the single source of truth for what a purge reaches. It is
// written child-to-parent for readability only: correctness does not depend on
// the order (design §3.4).
//
// Adding a table to fleet-service and not to this list (or to excludedTables)
// fails arch_test.go's completeness check.
var Manifest = []Target{
	{
		Key: "mileage_records", Table: "fleet.mileage_records",
		Orphan: &OrphanRule{Column: "vehicle_id", ParentTable: "fleet.vehicles"},
		Where:  vehicleChild("vehicle_id", TargetMileageRecord),
	},
	{
		Key: "fuel_logs", Table: "fleet.fuel_logs",
		Orphan: &OrphanRule{Column: "vehicle_id", ParentTable: "fleet.vehicles"},
		Where:  vehicleChild("vehicle_id", TargetFuelLog),
	},
	{
		Key: "maintenance_record_documents", Table: "fleet.maintenance_record_documents",
		Orphan: &OrphanRule{Column: "maintenance_record_id", ParentTable: "fleet.maintenance_records"},
		Where: func(r Root) (string, []any) {
			switch r.Scope {
			case ScopeSystem:
				return all, nil
			case ScopeFleet:
				return `maintenance_record_id IN (
					SELECT id FROM fleet.maintenance_records
					WHERE vehicle_id IN (SELECT id FROM fleet.vehicles WHERE fleet_id = ?))`, []any{r.TargetID}
			case ScopeRecord:
				switch r.TargetType {
				case TargetMaintenanceRecord:
					return "maintenance_record_id = ?", []any{r.TargetID}
				case TargetVehicle:
					return `maintenance_record_id IN (
						SELECT id FROM fleet.maintenance_records WHERE vehicle_id = ?)`, []any{r.TargetID}
				}
			}
			return "", nil
		},
	},
	{
		Key: "maintenance_records", Table: "fleet.maintenance_records",
		Orphan: &OrphanRule{Column: "vehicle_id", ParentTable: "fleet.vehicles"},
		Where:  vehicleChild("vehicle_id", TargetMaintenanceRecord),
	},
	{
		Key: "maintenance_schedules", Table: "fleet.maintenance_schedules",
		Orphan: &OrphanRule{Column: "vehicle_id", ParentTable: "fleet.vehicles"},
		Where:  vehicleChild("vehicle_id", TargetMaintenanceSchedule),
	},
	{
		Key: "vehicle_media", Table: "fleet.vehicle_media",
		Orphan: &OrphanRule{Column: "vehicle_id", ParentTable: "fleet.vehicles"},
		Where:  vehicleChild("vehicle_id", TargetVehicleMedia),
	},
	{
		Key: "vehicles", Table: "fleet.vehicles",
		Orphan: &OrphanRule{Column: "fleet_id", ParentTable: "fleet.fleets"},
		Where:  fleetChild("fleet_id", TargetVehicle),
	},
	{
		Key: "dashboard_widgets", Table: "fleet.dashboard_widgets",
		Orphan: &OrphanRule{Column: "dashboard_id", ParentTable: "fleet.dashboards"},
		Where: func(r Root) (string, []any) {
			switch r.Scope {
			case ScopeSystem:
				return all, nil
			case ScopeFleet:
				return "dashboard_id IN (SELECT id FROM fleet.dashboards WHERE fleet_id = ?)", []any{r.TargetID}
			}
			return "", nil
		},
	},
	{
		Key: "dashboards", Table: "fleet.dashboards",
		Orphan: &OrphanRule{Column: "fleet_id", ParentTable: "fleet.fleets"},
		Where:  fleetChild("fleet_id", ""),
	},
	{
		Key: "activity_events", Table: "fleet.activity_events",
		// No orphan rule: vehicle_id is nullable and the row's real owner is the
		// fleet, so "vehicle is gone" does not make an activity event an orphan.
		Orphan: nil,
		Where: func(r Root) (string, []any) {
			switch r.Scope {
			case ScopeSystem:
				return all, nil
			case ScopeFleet:
				return "fleet_id = ?", []any{r.TargetID}
			case ScopeRecord:
				switch r.TargetType {
				case TargetActivityEvent:
					return "id = ?", []any{r.TargetID}
				case TargetVehicle:
					return "vehicle_id = ?", []any{r.TargetID}
				}
			}
			return "", nil
		},
	},
	{
		Key: "invites", Table: "fleet.fleet_invites",
		Orphan: &OrphanRule{Column: "fleet_id", ParentTable: "fleet.fleets"},
		Where:  fleetChild("fleet_id", TargetInvite),
	},
	{
		Key: "memberships", Table: "fleet.fleet_memberships",
		Orphan: &OrphanRule{Column: "fleet_id", ParentTable: "fleet.fleets"},
		Where:  fleetChild("fleet_id", TargetMembership),
	},
	{
		Key: "fleets", Table: "fleet.fleets",
		Orphan: nil,
		Where: func(r Root) (string, []any) {
			switch r.Scope {
			case ScopeSystem:
				return all, nil
			case ScopeFleet:
				return "id = ?", []any{r.TargetID}
			}
			return "", nil
		},
	},
}

// excludedTables documents every table a purge deliberately does not reach.
// arch_test.go requires each of fleet-service's tables to appear either here or
// in Manifest, so an omission is a decision someone made rather than one they
// forgot.
var excludedTables = map[string]string{
	"fleet.maintenance_categories": "seeded reference data shared by every fleet (PRD non-goal)",
	"fleet.purge_operations":       "the operation log itself; deleting it would erase the record of the purge",
	"fleet.admin_audit_events":     "append-only; survives a system purge (FR-ADMIN-AUDIT-2)",
	"outbox":                       "transient relay ledger drained by the outbox relay; owned by no fleet",
}
```

`manifest.go` imports nothing but the standard `gorm` types it does not actually
name — it has **no imports at all**. Drop the `import "time"` line shown in the
package header above; `RecoveryWindow` lives in `confirmation.go` (Task 16).

The `excludedTables` entry for `media.processed_events` / `notification.processed_events`
belongs to those services' own manifests (Tasks 13–14), not this one. It is worth
restating there in full, because it is a finding rather than bookkeeping: the
PRD's "all of `notification.*`" phrasing, taken literally, would delete the
idempotency ledger and let a Kafka replay regenerate notifications for data that
was just purged — a system purge that undoes itself on the next consumer replay.

- [ ] **Step 4: Write the four operations**

Create `apps/fleet-service/internal/admin/operations.go`:

```go
package admin

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Count returns, per manifest key, how many LIVE rows a purge at this root
// would take. It is the blast-radius panel's only source (FR-ADMIN-UI-9).
//
// Count and Stamp share the same Where, which is what makes the displayed
// figures and the affected rows provably equal rather than equal by discipline.
func Count(db *gorm.DB, root Root) (map[string]int, error) {
	out := make(map[string]int, len(Manifest))
	for _, t := range Manifest {
		pred, args := t.Where(root)
		if pred == "" {
			continue
		}
		var n int64
		q := "SELECT count(*) FROM " + t.Table + " WHERE (" + pred + ") AND deleted_at IS NULL"
		if err := db.Raw(q, args...).Scan(&n).Error; err != nil {
			return nil, fmt.Errorf("count %s: %w", t.Table, err)
		}
		out[t.Key] = int(n)
	}
	return out, nil
}

// Stamp soft-deletes every row the root resolves to, marking it with opID, and
// returns the per-table counts now carried by the operation.
//
// It writes deleted_at and purge_operation_id and NOTHING ELSE. In particular it
// never writes purge_after: both legacy sweeps (vehicle.PurgeExpired and
// media-service's ListPurgeable) key on that column, so leaving it NULL is what
// makes an admin-stamped row invisible to them (design F3).
//
// now is a parameter rather than SQL now() because the entire test harness is
// SQLite, which has no now(). The single production call site passes
// time.Now().UTC().
//
// The counts are read back AFTER the update, keyed on purge_operation_id rather
// than on rows affected. That is what makes a replay return the SAME numbers as
// the first call instead of zeros: the UPDATE guards on deleted_at IS NULL, so a
// replay touches nothing, but affected_counts must still be correct
// (FR-ADMIN-PURGE-10).
func Stamp(tx *gorm.DB, root Root, opID string, now time.Time) (map[string]int, error) {
	for _, t := range Manifest {
		pred, args := t.Where(root)
		if pred == "" {
			continue
		}
		q := "UPDATE " + t.Table +
			" SET deleted_at = ?, purge_operation_id = ?" +
			" WHERE (" + pred + ") AND deleted_at IS NULL"
		full := append([]any{now, opID}, args...)
		if err := tx.Exec(q, full...).Error; err != nil {
			return nil, fmt.Errorf("stamp %s: %w", t.Table, err)
		}
	}
	return CountByOperation(tx, opID)
}

// CountByOperation returns the per-table rows currently carrying opID.
func CountByOperation(db *gorm.DB, opID string) (map[string]int, error) {
	out := make(map[string]int, len(Manifest))
	for _, t := range Manifest {
		var n int64
		q := "SELECT count(*) FROM " + t.Table + " WHERE purge_operation_id = ?"
		if err := db.Raw(q, opID).Scan(&n).Error; err != nil {
			return nil, fmt.Errorf("count by operation %s: %w", t.Table, err)
		}
		out[t.Key] = int(n)
	}
	return out, nil
}

// Restore clears the soft-delete on every row carrying opID.
//
// It does not use Where at all. Keying purely on purge_operation_id is what
// makes restore scope-independent, order-independent, idempotent — and, most
// importantly, incapable of resurrecting a row a user deleted before the purge:
// such a row has a NULL purge_operation_id and no operation's restore can match
// it (FR-ADMIN-RESTORE-3).
func Restore(tx *gorm.DB, opID string) error {
	for _, t := range Manifest {
		q := "UPDATE " + t.Table +
			" SET deleted_at = NULL, purge_operation_id = NULL WHERE purge_operation_id = ?"
		if err := tx.Exec(q, opID).Error; err != nil {
			return fmt.Errorf("restore %s: %w", t.Table, err)
		}
	}
	return nil
}

// Reap hard-deletes every row carrying opID and returns per-table counts
// removed. Like Restore it keys only on the operation id, so it is idempotent:
// a second call deletes nothing and reports zeros.
func Reap(tx *gorm.DB, opID string) (map[string]int, error) {
	out := make(map[string]int, len(Manifest))
	for _, t := range Manifest {
		res := tx.Exec("DELETE FROM "+t.Table+" WHERE purge_operation_id = ?", opID)
		if res.Error != nil {
			return nil, fmt.Errorf("reap %s: %w", t.Table, res.Error)
		}
		out[t.Key] = int(res.RowsAffected)
	}
	return out, nil
}

// StampReversedForTest runs Stamp's updates in the exact reverse of the
// manifest's order. It exists so TestStamp_isOrderIndependent can prove design
// §3.4's rule holds rather than assuming it; production code must call Stamp.
func StampReversedForTest(tx *gorm.DB, root Root, opID string, now time.Time) error {
	for i := len(Manifest) - 1; i >= 0; i-- {
		t := Manifest[i]
		pred, args := t.Where(root)
		if pred == "" {
			continue
		}
		q := "UPDATE " + t.Table +
			" SET deleted_at = ?, purge_operation_id = ?" +
			" WHERE (" + pred + ") AND deleted_at IS NULL"
		if err := tx.Exec(q, append([]any{now, opID}, args...)...).Error; err != nil {
			return fmt.Errorf("stamp %s: %w", t.Table, err)
		}
	}
	return nil
}
```

- [ ] **Step 5: Write the manifest-completeness arch test**

Create `apps/fleet-service/internal/admin/arch_test.go`:

```go
package admin

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestManifestCoversEveryTable turns FR-ADMIN-PURGE-4's "every table, enumerated
// by hand" from a checklist into a compile-time-ish guarantee.
//
// It parses every entity.go under internal/, extracts each
// `func (X) TableName() string { return "…" }` literal, and requires the table
// to be in Manifest or in excludedTables with a reason. A new table added
// anywhere in fleet-service fails here until someone decides whether a purge
// should reach it.
//
// Parsing rather than grepping: table names appear in comments, raw SQL and
// test DDL all over the service, and a grep would produce false matches the
// first time someone documents one.
func TestManifestCoversEveryTable(t *testing.T) {
	inManifest := map[string]bool{}
	for _, target := range Manifest {
		inManifest[target.Table] = true
	}

	root := ".." // apps/fleet-service/internal
	var found []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Base(path) != "entity.go" {
			return nil
		}
		fset := token.NewFileSet()
		f, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			// A parse failure must fail the test, not silently skip the file.
			t.Fatalf("parse %s: %v", path, perr)
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name.Name != "TableName" || fn.Recv == nil || fn.Body == nil {
				continue
			}
			for _, stmt := range fn.Body.List {
				ret, ok := stmt.(*ast.ReturnStmt)
				if !ok || len(ret.Results) != 1 {
					continue
				}
				lit, ok := ret.Results[0].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				name, uerr := strconv.Unquote(lit.Value)
				if uerr != nil {
					t.Fatalf("%s: unquote %s: %v", path, lit.Value, uerr)
				}
				found = append(found, name)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	if len(found) == 0 {
		t.Fatal("found no TableName declarations — the walk root is wrong, and this test would pass vacuously")
	}

	for _, name := range found {
		if inManifest[name] {
			continue
		}
		if reason, ok := excludedTables[name]; ok {
			if strings.TrimSpace(reason) == "" {
				t.Errorf("%s is excluded from the purge manifest with an empty reason", name)
			}
			continue
		}
		t.Errorf("%s is in neither admin.Manifest nor admin.excludedTables — "+
			"decide whether a purge should reach it, then add it to one of them", name)
	}
}

// TestManifestKeysAreUnique guards affected_counts: two targets sharing a key
// would silently overwrite each other's count and understate the blast radius.
func TestManifestKeysAreUnique(t *testing.T) {
	seen := map[string]string{}
	for _, target := range Manifest {
		if prev, dup := seen[target.Key]; dup {
			t.Errorf("manifest key %q is used by both %s and %s", target.Key, prev, target.Table)
		}
		seen[target.Key] = target.Table
	}
}
```

- [ ] **Step 6: Run everything to green**

Run: `go test ./apps/fleet-service/internal/admin/... -count=1 -v`
Expected: PASS — the six operations tests plus both arch tests.

- [ ] **Step 7: Commit**

```bash
git add apps/fleet-service/internal/admin
git commit -m "feat(fleet): add the declarative purge manifest and its four operations"
```

---

### Task 8: fix the pre-existing orphan defect

**Files:**
- Create: `apps/fleet-service/internal/admin/orphans.go`
- Create: `apps/fleet-service/internal/admin/orphans_test.go`
- Modify: `apps/fleet-service/internal/vehicle/purge.go`
- Modify: `apps/fleet-service/cmd/main.go:130-138`

**Interfaces:**
- Consumes: `Manifest`, `OrphanRule` from Task 7.
- Produces:
  - `func DeleteVehicleChildren(tx *gorm.DB, vehicleIDs []string) error`
  - `func DeleteOrphans(db *gorm.DB) (map[string]int, error)`
  - `vehicle.PurgeExpired(db *gorm.DB, deleteChildren func(tx *gorm.DB, vehicleIDs []string) error) error`
    — **signature change**; the one existing call site is `cmd/main.go`.

**What is broken today.** `vehicle.PurgeExpired` runs
`DELETE FROM fleet.vehicles WHERE purge_after IS NOT NULL AND purge_after < now()`
with no cascade and no foreign keys anywhere, so every mileage record, fuel log,
maintenance record, schedule, and vehicle-media row belonging to a purged vehicle
is left orphaned forever, referencing a `vehicle_id` that no longer exists. They
are invisible to the product (all reads are vehicle-scoped) but they accumulate
and `/admin/stats` would count them. This must land before `/admin/stats` is
trusted (risks.md R10) — the console's first act would otherwise be to report
numbers no fleet can reconcile.

`vehicle` takes the cascade as an injected function rather than importing
`admin`, matching the repo's `StatusDeps` / `WithOverdueHooks` idiom and keeping
the dependency arrow pointing one way.

- [ ] **Step 1: Write the failing tests**

Create `apps/fleet-service/internal/admin/orphans_test.go`:

```go
package admin_test

import (
	"testing"
	"time"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin/admintest"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/vehicle"
)

// PRD §11: hard-deleting a vehicle must take its children with it. Before this
// task the DELETE cascaded to nothing and the children lived forever.
func TestPurgeExpired_cascadesToChildren(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-1")

	past := time.Now().UTC().Add(-time.Hour)
	if err := db.Exec(`UPDATE fleet.vehicles SET deleted_at = ?, purge_after = ? WHERE id = ?`,
		past, past, f.VehicleID).Error; err != nil {
		t.Fatalf("expire vehicle: %v", err)
	}

	if err := vehicle.PurgeExpired(db, admin.DeleteVehicleChildren); err != nil {
		t.Fatalf("purge expired: %v", err)
	}

	if got := admintest.CountRows(t, db, "fleet.vehicles"); got != 1 {
		t.Errorf("expected only the un-expired vehicle to remain, got %d rows", got)
	}
	for _, table := range []string{
		"fleet.mileage_records", "fleet.fuel_logs", "fleet.maintenance_records",
		"fleet.maintenance_record_documents", "fleet.maintenance_schedules", "fleet.vehicle_media",
	} {
		if got := admintest.CountRows(t, db, table); got != 0 {
			t.Errorf("%s left %d orphaned rows behind the purged vehicle", table, got)
		}
	}
}

// FR-ADMIN-RESTORE-7 / design F3: the legacy sweep must not eat a vehicle that
// belongs to a pending, still-cancellable admin operation.
func TestPurgeExpired_skipsAdminStampedVehicles(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-1")

	past := time.Now().UTC().Add(-time.Hour)
	// purge_after is set here deliberately: an admin stamp never writes it, so
	// this row could only exist if someone later "helpfully" did. The explicit
	// purge_operation_id IS NULL narrowing is what still saves it.
	if err := db.Exec(`UPDATE fleet.vehicles SET deleted_at = ?, purge_after = ?,
	                   purge_operation_id = 'op-1' WHERE id = ?`, past, past, f.VehicleID).Error; err != nil {
		t.Fatalf("stamp vehicle: %v", err)
	}

	if err := vehicle.PurgeExpired(db, admin.DeleteVehicleChildren); err != nil {
		t.Fatalf("purge expired: %v", err)
	}
	if got := admintest.CountRows(t, db, "fleet.vehicles"); got != 2 {
		t.Errorf("the legacy sweep hard-deleted an admin-stamped vehicle: %d of 2 rows remain", got)
	}
	if got := admintest.CountRows(t, db, "fleet.mileage_records"); got != 1 {
		t.Errorf("the legacy sweep cascaded into an admin-stamped vehicle: %d of 1 rows remain", got)
	}
}

// The one-time cleanup for rows the old sweep already orphaned (PRD §11b).
func TestDeleteOrphans_removesRowsWithNoParentAndIsANoOpWhenClean(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-1")

	if removed, err := admin.DeleteOrphans(db); err != nil {
		t.Fatalf("clean-database cleanup: %v", err)
	} else {
		for key, n := range removed {
			if n != 0 {
				t.Errorf("cleanup removed %d %s rows from a clean database", n, key)
			}
		}
	}

	// Simulate the historical defect: the vehicle row is gone, its children stay.
	if err := db.Exec(`DELETE FROM fleet.vehicles WHERE id = ?`, f.VehicleID).Error; err != nil {
		t.Fatalf("orphan the children: %v", err)
	}

	removed, err := admin.DeleteOrphans(db)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if removed["mileage_records"] != 1 || removed["fuel_logs"] != 1 || removed["maintenance_records"] != 1 {
		t.Errorf("cleanup under-reported: %+v", removed)
	}
	for _, table := range []string{
		"fleet.mileage_records", "fleet.fuel_logs", "fleet.maintenance_records",
		"fleet.maintenance_record_documents", "fleet.maintenance_schedules", "fleet.vehicle_media",
	} {
		if got := admintest.CountRows(t, db, table); got != 0 {
			t.Errorf("%s still has %d orphaned rows", table, got)
		}
	}
	// activity_events has a nullable vehicle_id and belongs to the FLEET, so it
	// is deliberately not orphan-swept by vehicle deletion.
	if got := admintest.CountRows(t, db, "fleet.activity_events"); got != 1 {
		t.Errorf("activity events must survive a vehicle's deletion, got %d rows", got)
	}
	// The surviving fleet's own rows are untouched.
	if got := admintest.CountRows(t, db, "fleet.fleet_memberships"); got != 1 {
		t.Errorf("cleanup removed a live membership, %d rows remain", got)
	}
}
```

- [ ] **Step 2: Run and watch it fail**

Run: `go test ./apps/fleet-service/internal/admin/ -run 'PurgeExpired|DeleteOrphans' -count=1 -v`
Expected: FAIL — `undefined: admin.DeleteVehicleChildren`, and
`vehicle.PurgeExpired` takes one argument.

- [ ] **Step 3: Write the cascade helpers**

Create `apps/fleet-service/internal/admin/orphans.go`.

Two things to get right, both easy to get wrong: `maintenance_record_documents`
hangs off `maintenance_records`, not off vehicles, so it must be deleted **before**
the records are — otherwise its parent disappears and only a later orphan sweep
catches it. And the manifest's record-scope predicates take one id, while this
helper takes a set, so the `IN` form is built from each target's `OrphanRule`
rather than by calling `Where` once per vehicle per table.

```go
package admin

import (
	"fmt"

	"gorm.io/gorm"
)

// DeleteVehicleChildren hard-deletes every row beneath the given vehicles. It
// does NOT delete the vehicles themselves — the caller owns that, because the
// caller is what decided they should go.
//
// It is injected into vehicle.PurgeExpired rather than called from it, so the
// vehicle package does not import this one and the dependency arrow keeps
// pointing one way.
//
// Documents go first and explicitly: they hang off maintenance_records, not off
// vehicles, so deleting the records first would strand them exactly the way the
// original defect stranded everything else.
func DeleteVehicleChildren(tx *gorm.DB, vehicleIDs []string) error {
	if len(vehicleIDs) == 0 {
		return nil
	}
	docs := `DELETE FROM fleet.maintenance_record_documents
	         WHERE maintenance_record_id IN (
	             SELECT id FROM fleet.maintenance_records WHERE vehicle_id IN ?)`
	if err := tx.Exec(docs, vehicleIDs).Error; err != nil {
		return fmt.Errorf("delete maintenance_record_documents for vehicles: %w", err)
	}
	for _, t := range Manifest {
		if t.Orphan == nil || t.Orphan.ParentTable != "fleet.vehicles" {
			continue
		}
		q := "DELETE FROM " + t.Table + " WHERE " + t.Orphan.Column + " IN ?"
		if err := tx.Exec(q, vehicleIDs).Error; err != nil {
			return fmt.Errorf("delete %s for vehicles: %w", t.Table, err)
		}
	}
	return nil
}

// DeleteOrphans hard-deletes rows whose parent no longer exists, using each
// manifest target's OrphanRule. It returns per-key counts removed and is a no-op
// on a clean database.
//
// This is the one-time cleanup for rows the pre-cascade vehicle sweep already
// stranded (PRD §11b). It runs at startup under an advisory lock and logs what
// it removed; on a healthy database it removes nothing and says so.
//
// Targets are walked in manifest order — child to parent — so a row is judged
// against a parent that has not yet been swept away in the same pass.
//
// The DELETE target carries no alias: `DELETE FROM t AS c` is Postgres-only and
// SQLite (the whole test harness) rejects it, so the correlated sub-query
// qualifies the column with the full table name instead.
func DeleteOrphans(db *gorm.DB) (map[string]int, error) {
	out := make(map[string]int, len(Manifest))
	for _, t := range Manifest {
		if t.Orphan == nil {
			continue
		}
		q := "DELETE FROM " + t.Table + " WHERE NOT EXISTS (" +
			"SELECT 1 FROM " + t.Orphan.ParentTable + " p WHERE p.id = " +
			t.Table + "." + t.Orphan.Column + ")"
		res := db.Exec(q)
		if res.Error != nil {
			return nil, fmt.Errorf("delete orphans in %s: %w", t.Table, res.Error)
		}
		out[t.Key] = int(res.RowsAffected)
	}
	return out, nil
}
```

- [ ] **Step 4: Rewrite `vehicle.PurgeExpired`**

`apps/fleet-service/internal/vehicle/purge.go`:

```go
package vehicle

import (
	"time"

	"gorm.io/gorm"
)

const recoveryWindow = 5 * 24 * time.Hour

// ComputePurgeAfter returns the time after which a soft-deleted vehicle may be
// hard-deleted (deletedAt + 5 days).
func ComputePurgeAfter(deletedAt time.Time) time.Time { return deletedAt.Add(recoveryWindow) }

// IsPurgeable reports whether a soft-deleted vehicle has passed its recovery window.
func IsPurgeable(purgeAfter *time.Time) bool {
	return purgeAfter != nil && time.Now().After(*purgeAfter)
}

// PurgeExpired hard-deletes vehicles past purge_after together with every row
// beneath them. Run under database.WithLeaderLock for multi-replica safety.
//
// It used to be a bare DELETE over fleet.vehicles. There are no foreign keys
// anywhere in this schema, so nothing cascaded and every mileage record, fuel
// log, maintenance record, schedule and vehicle-media row belonging to a purged
// vehicle was left referencing a vehicle_id that no longer existed — invisible
// to the product, because every read is vehicle-scoped, and accumulating
// forever (PRD §11).
//
// deleteChildren is injected so this package does not import the admin manifest.
// The composition root passes admin.DeleteVehicleChildren.
//
// purge_operation_id IS NULL keeps this sweep off rows an admin purge stamped:
// those belong to a cancellable operation and the admin reaper owns their
// lifecycle (FR-ADMIN-RESTORE-7).
func PurgeExpired(db *gorm.DB, deleteChildren func(tx *gorm.DB, vehicleIDs []string) error) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var ids []string
		if err := tx.Raw(`SELECT id FROM fleet.vehicles
		                  WHERE purge_after IS NOT NULL AND purge_after < ?
		                    AND purge_operation_id IS NULL`, time.Now().UTC()).
			Scan(&ids).Error; err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		if err := deleteChildren(tx, ids); err != nil {
			return err
		}
		return tx.Exec("DELETE FROM fleet.vehicles WHERE id IN ?", ids).Error
	})
}
```

`now()` becomes a bound parameter for the same reason `Stamp` takes one: the test
harness is SQLite.

- [ ] **Step 5: Wire it in the composition root**

`apps/fleet-service/cmd/main.go`. Add the `admin` import, run the one-time
cleanup, and move the sweep to the hourly cadence (design OQ-5 — the 24-hour
timer's first tick is at `T+24h`, so in a service that redeploys daily this job
never ran at all):

```go
	// One-time cleanup of rows the pre-cascade vehicle sweep already orphaned
	// (PRD §11b). Under the leader lock so only one replica runs it, and a no-op
	// on a healthy database. It must precede /admin/stats being trusted: until
	// it runs, the console reports numbers no fleet can reconcile (risks.md R10).
	if _, err := database.WithLeaderLock(db, "admin-orphan-cleanup", func() error {
		removed, cerr := admin.DeleteOrphans(db)
		if cerr != nil {
			return cerr
		}
		total := 0
		for _, n := range removed {
			total += n
		}
		if total > 0 {
			log.WithField("removed", removed).Warn("deleted orphaned rows left by the pre-cascade vehicle sweep")
		}
		return nil
	}); err != nil {
		log.WithError(err).Fatal("orphan cleanup")
	}

	// Background sweep: hard-delete soft-deleted vehicles past their purge
	// window, cascading to their children. Hourly, not daily: jobs.Every fires
	// its FIRST tick at T+interval, so a 24-hour job in a service that redeploys
	// more often than daily never runs at all (design OQ-5).
	ctx := context.Background()
	go jobs.Every(ctx, 1*time.Hour, func(ctx context.Context) error {
		_, err := database.WithLeaderLock(db, "vehicle-purge", func() error {
			return vehicle.PurgeExpired(db, admin.DeleteVehicleChildren)
		})
		if err != nil {
			log.WithError(err).Warn("vehicle purge sweep failed")
		}
		return err
	})
```

Also move media-service's sweep to hourly for the same reason —
`apps/media-service/cmd/main.go:109`:

```go
	// Hourly, not daily: jobs.Every's first tick is at T+interval, so a
	// 24-hour sweep in a service that redeploys more often than daily never
	// runs (design OQ-5).
	go jobs.Every(ctx, 1*time.Hour, func(ctx context.Context) error {
```

- [ ] **Step 6: Run the tests and the build**

Run: `go build github.com/jtumidanski/myfleet/... && go test ./apps/fleet-service/... ./apps/media-service/... -count=1`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add apps/fleet-service apps/media-service
git commit -m "fix(fleet): cascade the vehicle purge sweep and clean up pre-existing orphans"
```

---

# Phase 2 — The authorization tier (Tasks 9–12)

---

### Task 9: `auth.platform_admins` and its two seeding hooks

**Files:**
- Create: `apps/auth-service/internal/platformadmin/entity.go`
- Create: `apps/auth-service/internal/platformadmin/provider.go`
- Create: `apps/auth-service/internal/platformadmin/administrator.go`
- Create: `apps/auth-service/internal/platformadmin/seed.go`
- Create: `apps/auth-service/internal/platformadmin/seed_test.go`
- Modify: `apps/auth-service/internal/user/processor.go`
- Modify: `apps/auth-service/internal/user/processor_test.go`
- Modify: `apps/auth-service/cmd/main.go`

**Interfaces:**
- Produces, in package `platformadmin`:
  - `type Entity struct { UserID, GrantedBy string; GrantedAt time.Time }`, `TableName() "auth.platform_admins"`, `Migration(db) error`
  - `type Provider interface { IsAdmin(userID string) (bool, error) }`, `NewProvider(db) Provider`
  - `type Administrator interface { Grant(userID, grantedBy string) error }`, `NewAdministrator(db) Administrator`
  - `func ParseBootstrapEmails(raw string) map[string]bool`
  - `func SeedFromEmails(db *gorm.DB, emails map[string]bool) (int, error)`
  - `const BootstrapGrantedBy = "bootstrap"`
- Produces, in package `user`:
  - `func (pr *Processor) WithBootstrapAdmins(emails map[string]bool, grant func(userID string) error) *Processor`

**Why two hooks (FR-ADMIN-AUTH-1/2).** The table is keyed by `user_id`, the
bootstrap list is emails, and the bootstrap user may not exist at first
migration. A startup seed alone would never fire for a fresh database; a
provision-time hook alone would never fire for a user who already exists. Both,
and both idempotent.

- [ ] **Step 1: Write the failing seed test**

Create `apps/auth-service/internal/platformadmin/seed_test.go`:

```go
package platformadmin

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newSeedDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec("ATTACH DATABASE ':memory:' AS auth").Error; err != nil {
		t.Fatalf("attach auth schema: %v", err)
	}
	ddl := []string{
		`CREATE TABLE auth.users (
			id TEXT PRIMARY KEY, google_sub TEXT, email TEXT, display_name TEXT,
			avatar_url TEXT, theme_preference TEXT, last_login_at DATETIME,
			created_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE auth.platform_admins (
			user_id TEXT PRIMARY KEY, granted_by TEXT, granted_at DATETIME)`,
	}
	for _, stmt := range ddl {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("ddl: %v", err)
		}
	}
	return db
}

func TestParseBootstrapEmails_normalises(t *testing.T) {
	got := ParseBootstrapEmails("  JTumidanski@Gmail.com , second@example.com ,,")
	if len(got) != 2 {
		t.Fatalf("want 2 emails, got %d: %v", len(got), got)
	}
	if !got["jtumidanski@gmail.com"] {
		t.Errorf("emails must be lower-cased and trimmed: %v", got)
	}
	if got[""] {
		t.Errorf("empty segments must be dropped: %v", got)
	}
	if n := len(ParseBootstrapEmails("")); n != 0 {
		t.Errorf("an empty list must parse to no emails, got %d", n)
	}
}

// FR-ADMIN-AUTH-1: the startup seed grants only to users that already exist,
// and re-running it changes nothing.
func TestSeedFromEmails_grantsExistingUsersAndIsIdempotent(t *testing.T) {
	db := newSeedDB(t)
	if err := db.Exec(`INSERT INTO auth.users (id, google_sub, email)
	                   VALUES ('u1', 'sub-1', 'jtumidanski@gmail.com')`).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	emails := ParseBootstrapEmails("jtumidanski@gmail.com,absent@example.com")

	granted, err := SeedFromEmails(db, emails)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if granted != 1 {
		t.Errorf("want 1 grant (the absent user has no row to key on), got %d", granted)
	}

	prov := NewProvider(db)
	if ok, err := prov.IsAdmin("u1"); err != nil || !ok {
		t.Errorf("u1 should be a platform admin, got %v err %v", ok, err)
	}

	if _, err := SeedFromEmails(db, emails); err != nil {
		t.Fatalf("second seed must succeed: %v", err)
	}
	var rows int64
	if err := db.Raw(`SELECT count(*) FROM auth.platform_admins`).Scan(&rows).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if rows != 1 {
		t.Errorf("re-seeding duplicated the grant: %d rows", rows)
	}
}

// Case folding matters: Google returns whatever casing the user typed, and the
// allow-list is hand-written in a ConfigMap.
func TestSeedFromEmails_isCaseInsensitive(t *testing.T) {
	db := newSeedDB(t)
	if err := db.Exec(`INSERT INTO auth.users (id, google_sub, email)
	                   VALUES ('u1', 'sub-1', 'JTumidanski@GMAIL.com')`).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := SeedFromEmails(db, ParseBootstrapEmails("jtumidanski@gmail.com")); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if ok, _ := NewProvider(db).IsAdmin("u1"); !ok {
		t.Error("a differently-cased stored email must still match the bootstrap list")
	}
}

// Revocation is an out-of-band DELETE (PRD non-goal: no UI). The provider is
// the runtime source of truth, so it must see the deletion immediately.
func TestProvider_reflectsRevocation(t *testing.T) {
	db := newSeedDB(t)
	if err := NewAdministrator(db).Grant("u1", BootstrapGrantedBy); err != nil {
		t.Fatalf("grant: %v", err)
	}
	if err := db.Exec(`DELETE FROM auth.platform_admins WHERE user_id = 'u1'`).Error; err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if ok, err := NewProvider(db).IsAdmin("u1"); err != nil || ok {
		t.Errorf("a revoked admin must read as false, got %v err %v", ok, err)
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test ./apps/auth-service/internal/platformadmin/ -v`
Expected: FAIL — the package does not exist.

- [ ] **Step 3: Write the package**

`apps/auth-service/internal/platformadmin/entity.go`:

```go
// Package platformadmin owns auth.platform_admins — the runtime source of truth
// for who may use the platform-admin console.
//
// The bootstrap email list SEEDS this table and is never consulted per request
// (FR-ADMIN-AUTH-3). That distinction is the whole point: an admin can be
// revoked by deleting a row, with no redeploy and no config change.
package platformadmin

import (
	"time"

	"gorm.io/gorm"
)

// BootstrapGrantedBy is the granted_by value for rows the seed creates. A real
// grant records the granting user's id instead.
const BootstrapGrantedBy = "bootstrap"

// Entity maps to auth.platform_admins (PRD §6.1). user_id references
// auth.users.id logically; there are no foreign keys in this schema.
type Entity struct {
	UserID    string    `gorm:"type:uuid;primaryKey"`
	GrantedBy string    `gorm:"not null"`
	GrantedAt time.Time `gorm:"not null"`
}

func (Entity) TableName() string { return "auth.platform_admins" }

func Migration(db *gorm.DB) error { return db.AutoMigrate(&Entity{}) }
```

`provider.go`:

```go
package platformadmin

import "gorm.io/gorm"

// Provider is the read-only interface for platform-admin lookups.
type Provider interface {
	// IsAdmin reports whether the user currently holds the privilege. It reads
	// the table on every call by design: this is the check that bounds the
	// stale-claim window on irreversible operations (FR-ADMIN-AUTH-7), so a
	// cache would defeat its only purpose.
	IsAdmin(userID string) (bool, error)
}

type dbProvider struct{ db *gorm.DB }

// NewProvider returns a read-only Provider backed by the given database.
func NewProvider(db *gorm.DB) Provider { return &dbProvider{db: db} }

func (p *dbProvider) IsAdmin(userID string) (bool, error) {
	if userID == "" {
		return false, nil
	}
	var count int64
	if err := p.db.Model(&Entity{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}
```

`administrator.go`:

```go
package platformadmin

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Administrator is the write interface for platform-admin grants.
type Administrator interface {
	// Grant makes userID a platform admin. Idempotent: re-granting leaves the
	// original granted_by and granted_at intact, so the audit value of "who
	// first granted this, and when" is not overwritten by a restart.
	Grant(userID, grantedBy string) error
}

type dbAdministrator struct{ db *gorm.DB }

// NewAdministrator returns an Administrator backed by the given database.
func NewAdministrator(db *gorm.DB) Administrator { return &dbAdministrator{db: db} }

func (a *dbAdministrator) Grant(userID, grantedBy string) error {
	e := Entity{UserID: userID, GrantedBy: grantedBy, GrantedAt: time.Now().UTC()}
	return a.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&e).Error
}
```

`seed.go`:

```go
package platformadmin

import (
	"strings"

	"gorm.io/gorm"
)

// ParseBootstrapEmails splits PLATFORM_ADMIN_BOOTSTRAP_EMAILS into a lookup set,
// trimming and lower-casing each entry and dropping empties.
//
// Same shape as KAFKA_BROKERS parsing in the other composition roots, with the
// case fold added because Google returns whatever casing the user typed while
// the list is hand-written in a ConfigMap.
func ParseBootstrapEmails(raw string) map[string]bool {
	out := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		if e := strings.ToLower(strings.TrimSpace(part)); e != "" {
			out[e] = true
		}
	}
	return out
}

// SeedFromEmails grants the privilege to every EXISTING user whose email is in
// the set, and returns how many grants it made.
//
// A bootstrap email with no user row is silently skipped — that is the normal
// case on a fresh database, and user.Processor's provision-time hook is what
// covers it at first login (FR-ADMIN-AUTH-2).
//
// The users read is raw SQL rather than a user.Provider call so this package
// does not import user; both live in the same service and schema, so this is
// not the cross-service DB read that design D6 forbids.
func SeedFromEmails(db *gorm.DB, emails map[string]bool) (int, error) {
	if len(emails) == 0 {
		return 0, nil
	}
	list := make([]string, 0, len(emails))
	for e := range emails {
		list = append(list, e)
	}

	var ids []string
	if err := db.Raw(`SELECT id FROM auth.users WHERE lower(email) IN ?`, list).Scan(&ids).Error; err != nil {
		return 0, err
	}

	adm := NewAdministrator(db)
	granted := 0
	for _, id := range ids {
		if err := adm.Grant(id, BootstrapGrantedBy); err != nil {
			return granted, err
		}
		granted++
	}
	return granted, nil
}
```

- [ ] **Step 4: Add the provision-time hook to `user.Processor`**

Write the test first, appended to `apps/auth-service/internal/user/processor_test.go`:

```go
// FR-ADMIN-AUTH-2: a bootstrap admin who does not exist at first migration gets
// the grant when they first sign in.
func TestProvisionFromGoogle_grantsBootstrapAdmin(t *testing.T) {
	var granted []string
	proc := NewProcessor(logrus.New(), &stubProvider{}, &stubAdministrator{}).
		WithBootstrapAdmins(
			map[string]bool{"jtumidanski@gmail.com": true},
			func(userID string) error { granted = append(granted, userID); return nil },
		)

	if _, err := proc.ProvisionFromGoogle(GoogleProfile{
		Sub: "sub-1", Email: "JTumidanski@Gmail.com", Name: "J",
	}); err != nil {
		t.Fatalf("provision: %v", err)
	}
	if len(granted) != 1 {
		t.Fatalf("want one grant, got %v", granted)
	}
}

func TestProvisionFromGoogle_doesNotGrantOtherUsers(t *testing.T) {
	var granted []string
	proc := NewProcessor(logrus.New(), &stubProvider{}, &stubAdministrator{}).
		WithBootstrapAdmins(
			map[string]bool{"jtumidanski@gmail.com": true},
			func(userID string) error { granted = append(granted, userID); return nil },
		)
	if _, err := proc.ProvisionFromGoogle(GoogleProfile{
		Sub: "sub-2", Email: "someone@example.com", Name: "Someone",
	}); err != nil {
		t.Fatalf("provision: %v", err)
	}
	if len(granted) != 0 {
		t.Errorf("a non-bootstrap email must not be granted admin: %v", granted)
	}
}

// A failing grant must not fail the login. The startup seed re-runs on every
// boot and will catch it; refusing to log the user in would be a worse outcome
// than a delayed grant.
func TestProvisionFromGoogle_survivesAFailingGrant(t *testing.T) {
	proc := NewProcessor(logrus.New(), &stubProvider{}, &stubAdministrator{}).
		WithBootstrapAdmins(
			map[string]bool{"jtumidanski@gmail.com": true},
			func(string) error { return errors.New("database down") },
		)
	if _, err := proc.ProvisionFromGoogle(GoogleProfile{
		Sub: "sub-1", Email: "jtumidanski@gmail.com", Name: "J",
	}); err != nil {
		t.Fatalf("a failing admin grant must not fail login, got %v", err)
	}
}
```

Reuse whatever stub `Provider`/`Administrator` the file already defines; if the
existing test file names them differently, use those names rather than adding
duplicates.

Then `apps/auth-service/internal/user/processor.go`:

```go
type Processor struct {
	log logrus.FieldLogger
	p   Provider
	a   Administrator
	// bootstrapEmails and grantAdmin are the provision-time half of platform
	// admin seeding (FR-ADMIN-AUTH-2). Both nil by default, so every existing
	// construction site compiles and behaves exactly as before.
	bootstrapEmails map[string]bool
	grantAdmin      func(userID string) error
}

// WithBootstrapAdmins returns a copy of the processor that grants platform
// admin to any provisioned user whose email is in emails.
//
// It follows the repo's established With… idiom (cf.
// maintenanceschedule.WithOverdueHooks, NewCompletionDeps().WithActivityRecorder)
// so this package never imports platformadmin: the composition root supplies the
// grant as a function value.
func (pr *Processor) WithBootstrapAdmins(emails map[string]bool, grant func(userID string) error) *Processor {
	cp := *pr
	cp.bootstrapEmails = emails
	cp.grantAdmin = grant
	return &cp
}

// ProvisionFromGoogle upserts a user by google_sub (FR-AUTH-2). Idempotent.
func (pr *Processor) ProvisionFromGoogle(gp GoogleProfile) (Model, error) {
	existing, err := pr.p.GetBySub(gp.Sub)
	if errors.Is(err, ErrNotFound) {
		m := NewBuilder().SetGoogleSub(gp.Sub).SetEmail(gp.Email).SetDisplayName(gp.Name).SetAvatarURL(gp.Avatar).Build()
		m = m.WithLogin(gp.Name, gp.Avatar, time.Now())
		created, ierr := pr.a.Insert(m)
		if ierr != nil {
			return Model{}, ierr
		}
		pr.maybeGrantAdmin(created)
		return created, nil
	}
	if err != nil {
		return Model{}, err
	}
	updated, uerr := pr.a.Update(existing.WithLogin(gp.Name, gp.Avatar, time.Now()))
	if uerr != nil {
		return Model{}, uerr
	}
	pr.maybeGrantAdmin(updated)
	return updated, nil
}

// maybeGrantAdmin grants the platform-admin privilege when the provisioned user
// is on the bootstrap list.
//
// A failure is logged, not returned. Refusing the login because a grant failed
// would be a worse outcome than a delayed grant, and the startup seed re-runs on
// every boot — so the failure is transient by construction.
func (pr *Processor) maybeGrantAdmin(m Model) {
	if pr.grantAdmin == nil || !pr.bootstrapEmails[strings.ToLower(m.Email())] {
		return
	}
	if err := pr.grantAdmin(m.ID()); err != nil {
		pr.log.WithError(err).WithField("user_id", m.ID()).
			Warn("bootstrap platform-admin grant failed; the startup seed will retry on the next boot")
	}
}
```

Add `"strings"` to the imports.

- [ ] **Step 5: Wire both hooks in the composition root**

`apps/auth-service/cmd/main.go`:

```go
	db, err := database.Connect(log, database.SetMigrations(
		user.Migration, session.Migration, platformadmin.Migration))
	if err != nil {
		log.WithError(err).Fatal("db connect")
	}

	// PLATFORM_ADMIN_BOOTSTRAP_EMAILS seeds auth.platform_admins and is never
	// consulted per request (FR-ADMIN-AUTH-3): the TABLE is the runtime source
	// of truth, so an admin can be revoked with a DELETE and no redeploy.
	bootstrapAdmins := platformadmin.ParseBootstrapEmails(
		config.Get("PLATFORM_ADMIN_BOOTSTRAP_EMAILS", "jtumidanski@gmail.com"))
	adminProv := platformadmin.NewProvider(db)
	adminAdm := platformadmin.NewAdministrator(db)

	// Hook 1 of 2: grant to bootstrap users that already exist. Idempotent
	// across restarts (FR-ADMIN-AUTH-1).
	if granted, serr := platformadmin.SeedFromEmails(db, bootstrapAdmins); serr != nil {
		log.WithError(serr).Fatal("seed platform admins")
	} else if granted > 0 {
		log.WithField("granted", granted).Info("seeded platform admins")
	}
	...
	// Hook 2 of 2: grant at provisioning time, for a bootstrap user who did not
	// exist when the seed ran (FR-ADMIN-AUTH-2).
	users := user.NewProcessor(log, userProv, user.NewAdministrator(db)).
		WithBootstrapAdmins(bootstrapAdmins, func(userID string) error {
			return adminAdm.Grant(userID, platformadmin.BootstrapGrantedBy)
		})
```

`adminProv` is unused until Task 10 wires the claim; declare it there instead if
the compiler objects, or leave the declaration and use it in the same commit as
Task 10.

- [ ] **Step 6: Run to green**

Run: `go test ./apps/auth-service/... -count=1 && go build github.com/jtumidanski/myfleet/...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add apps/auth-service
git commit -m "feat(auth): add auth.platform_admins with startup and provision-time seeding"
```

---

### Task 10: the `platform_admin` claim, end to end

**Files:**
- Modify: `apps/auth-service/internal/session/processor.go`
- Modify: `apps/auth-service/internal/session/processor_test.go`
- Modify: `apps/auth-service/cmd/main.go`
- Modify: `packages/shared-go/auth/identity.go`
- Modify: `packages/shared-go/auth/middleware.go`
- Modify: `packages/shared-go/auth/middleware_test.go`
- Modify: `apps/auth-service/internal/user/resource.go`

**Interfaces:**
- Consumes: `platformadmin.Provider.IsAdmin` from Task 9.
- Produces:
  - `session.Principal` gains `PlatformAdmin bool`; `MintAccess` emits the
    `platform_admin` claim.
  - `auth.Identity` gains `PlatformAdmin bool`, populated by the shared JWT
    middleware; a missing or non-boolean claim is `false`.
  - `GET /auth/me` returns `meta.platformAdmin`.

**Why this is nearly free (design §5.1).** `newPrincipalResolver` is the sole
construction site for `Principal`, enforced by `TestNoPrincipalLiteralOutsideResolver`.
Adding the field there means **both** mint paths — initial login and refresh
rotation — carry it, so FR-ADMIN-AUTH-4 needs no per-path work.

**The one catch.** `TestMintAccess_mapsEveryPrincipalField` asserts every
`Principal` field is a `string` and `t.Fatalf`s otherwise:
*"Principal.X is a %s, not a string — extend this test's sentinel scheme"*. A
`bool` field trips it deliberately. Extending the scheme is a required part of
this task, not an incidental fix.

- [ ] **Step 1: Extend the sentinel scheme (the test changes first)**

In `apps/auth-service/internal/session/processor_test.go`, replace the field loop
inside `TestMintAccess_mapsEveryPrincipalField`:

```go
	v := reflect.New(reflect.TypeOf(Principal{})).Elem()
	wantStrings := map[string]string{}
	var boolFields []string
	for i := 0; i < v.NumField(); i++ {
		f := v.Type().Field(i)
		switch f.Type.Kind() {
		case reflect.String:
			sentinel := "sentinel-" + f.Name
			v.Field(i).SetString(sentinel)
			wantStrings[f.Name] = sentinel
		case reflect.Bool:
			// A bool carries only two values, so it cannot hold a per-field
			// sentinel. Exactly ONE bool field is still attributable — set it
			// true and require a true-valued claim. A second would be
			// ambiguous, so fail loudly and make whoever adds it extend this.
			boolFields = append(boolFields, f.Name)
			if len(boolFields) > 1 {
				t.Fatalf("Principal now has %d bool fields (%v) — a true-valued claim can no "+
					"longer be attributed to one of them; extend this test's sentinel scheme",
					len(boolFields), boolFields)
			}
			v.Field(i).SetBool(true)
		default:
			t.Fatalf("Principal.%s is a %s — extend this test's sentinel scheme", f.Name, f.Type.Kind())
		}
	}
```

and replace the assertion block:

```go
	gotStrings := map[string]bool{}
	gotTrue := 0
	for _, cv := range claims {
		switch typed := cv.(type) {
		case string:
			gotStrings[typed] = true
		case bool:
			if typed {
				gotTrue++
			}
		}
	}
	for field, sentinel := range wantStrings {
		if !gotStrings[sentinel] {
			t.Errorf("Principal.%s never reaches a claim — MintAccess drops it. "+
				"Every Principal field must appear in MintAccess's claim map.", field)
		}
	}
	if len(boolFields) == 1 && gotTrue == 0 {
		t.Errorf("Principal.%s never reaches a claim — MintAccess drops it. "+
			"Every Principal field must appear in MintAccess's claim map.", boolFields[0])
	}
```

Add to the same file the explicit claim test:

```go
// FR-ADMIN-AUTH-4: the claim is emitted on every mint, and it is a JSON boolean
// rather than a string — the shared middleware parses it with a boolean
// accessor and would read "true" as false.
func TestMintAccess_setsPlatformAdminClaimAsBoolean(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	ks := jwks.NewKeySet(priv, "kid-1")
	p := NewProcessor(logrus.New(), ks, "myfleet-auth", "myfleet")

	for _, want := range []bool{true, false} {
		tokenStr, err := p.MintAccess(Principal{UserID: "u1", Email: "e@x", PlatformAdmin: want})
		if err != nil {
			t.Fatal(err)
		}
		claims := jwt.MapClaims{}
		if _, perr := jwt.ParseWithClaims(tokenStr, claims, func(*jwt.Token) (any, error) {
			return &priv.PublicKey, nil
		}); perr != nil {
			t.Fatal(perr)
		}
		got, ok := claims["platform_admin"].(bool)
		if !ok {
			t.Fatalf("platform_admin must be a JSON boolean, got %T (%v)",
				claims["platform_admin"], claims["platform_admin"])
		}
		if got != want {
			t.Errorf("platform_admin = %v, want %v", got, want)
		}
	}
}
```

- [ ] **Step 2: Run and watch it fail**

Run: `go test ./apps/auth-service/internal/session/ -run MintAccess -v`
Expected: FAIL — `unknown field PlatformAdmin in struct literal`.

- [ ] **Step 3: Add the field and the claim**

`apps/auth-service/internal/session/processor.go`:

```go
type Principal struct {
	UserID        string
	Email         string
	ActiveFleetID string
	Role          string
	// PlatformAdmin is orthogonal to Role: Role is a position inside one fleet,
	// this is a position above all of them. It is stamped at mint time from
	// auth.platform_admins, so revoking it does not take effect until the access
	// token expires or is refreshed — a staleness the console states in plain
	// words and the purge endpoints re-verify away (FR-ADMIN-AUTH-7).
	PlatformAdmin bool
}
```

```go
func (p *Processor) MintAccess(pr Principal) (string, error) {
	now := time.Now()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"sub":             pr.UserID,
		"email":           pr.Email,
		"active_fleet_id": pr.ActiveFleetID,
		"role":            pr.Role,
		"platform_admin":  pr.PlatformAdmin,
		"iss":             p.issuer,
		"aud":             p.aud,
		"iat":             now.Unix(),
		"exp":             now.Add(accessTTL).Unix(),
	})
```

- [ ] **Step 4: Populate it in the sole resolver**

`apps/auth-service/cmd/main.go` — `newPrincipalResolver` gains a third source:

```go
// newPrincipalResolver composes the three sources of identity — the local users
// table for email, fleet-service for the active membership, auth.platform_admins
// for the platform tier — into the single construction site for
// session.Principal. Every access token this service mints, on either path, is
// built here (FR-1, FR-2, FR-ADMIN-AUTH-4).
func newPrincipalResolver(
	users user.Provider,
	fleet *membership.Client,
	admins platformadmin.Provider,
) session.PrincipalResolver {
	return func(ctx context.Context, userID string) (session.Principal, error) {
		u, err := users.GetByID(userID)
		if err != nil {
			return session.Principal{}, err
		}
		m, err := fleet.Active(ctx, userID)
		if err != nil {
			return session.Principal{}, err
		}
		// Fail closed: a lookup error must not mint a token that silently
		// claims false, because the console's absence would then read as
		// "you are not an admin" rather than "we could not tell".
		isAdmin, err := admins.IsAdmin(userID)
		if err != nil {
			return session.Principal{}, err
		}
		return session.Principal{
			UserID:        userID,
			Email:         u.Email(),
			ActiveFleetID: m.FleetID,
			Role:          m.Role,
			PlatformAdmin: isAdmin,
		}, nil
	}
}
```

and its call site: `resolve := newPrincipalResolver(userProv, fleetClient, adminProv)`.

- [ ] **Step 5: Parse the claim in the shared middleware**

First the test, appended to `packages/shared-go/auth/middleware_test.go` — follow
whatever token-minting helper that file already uses:

```go
// FR-ADMIN-AUTH-5: a missing or non-boolean platform_admin claim parses to
// false. "true" as a STRING is the realistic failure — a hand-rolled token, or a
// mint path that stringified the value — and it must not grant anything.
func TestJWT_parsesPlatformAdminClaim(t *testing.T) {
	cases := []struct {
		name  string
		claim any
		want  bool
	}{
		{"true", true, true},
		{"false", false, false},
		{"absent", nil, false},
		{"string true", "true", false},
		{"number one", float64(1), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			extra := map[string]any{}
			if tc.claim != nil {
				extra["platform_admin"] = tc.claim
			}
			var got Identity
			srv := JWT(keyfnForTest(t))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = IdentityFromContext(r.Context())
			}))
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Authorization", "Bearer "+signTestToken(t, extra))
			srv.ServeHTTP(httptest.NewRecorder(), req)

			if got.PlatformAdmin != tc.want {
				t.Errorf("PlatformAdmin = %v, want %v", got.PlatformAdmin, tc.want)
			}
		})
	}
}
```

`keyfnForTest` / `signTestToken` stand in for the helpers already in that file —
read it and use the real names, adding an `extra claims` parameter if the
existing helper does not take one.

Then `packages/shared-go/auth/identity.go`:

```go
type Identity struct {
	UserID        string
	Email         string
	ActiveFleetID string
	Role          string // owner | member | viewer
	// PlatformAdmin is orthogonal to Role and to ActiveFleetID: an admin with no
	// fleet is a normal state, including immediately after a system purge
	// (FR-ADMIN-AUTH-9).
	PlatformAdmin bool
}
```

and `packages/shared-go/auth/middleware.go`:

```go
			id := Identity{
				UserID:        str(claims["sub"]),
				Email:         str(claims["email"]),
				ActiveFleetID: str(claims["active_fleet_id"]),
				Role:          str(claims["role"]),
				PlatformAdmin: boolean(claims["platform_admin"]),
			}
```

```go
// boolean mirrors str: anything that is not a JSON boolean — absent, a string,
// a number — reads as false. Failing closed here means a hand-rolled or
// half-migrated token can never grant the platform tier (FR-ADMIN-AUTH-5).
func boolean(v any) bool {
	b, ok := v.(bool)
	return ok && b
}
```

- [ ] **Step 6: Surface it on `/auth/me` (FR-ADMIN-AUTH-8)**

`apps/auth-service/internal/user/resource.go`, in the GET handler's write:

```go
			server.WriteJSON(w, http.StatusOK, server.Document{
				Data: Transform(m),
				// platformAdmin comes from the validated token's Identity, not
				// a second database read: the claim IS the authority the request
				// carried, so reporting anything else would tell the client it
				// has a capability its own token does not grant.
				Meta: map[string]any{
					"activeFleetId": id.ActiveFleetID,
					"role":          id.Role,
					"platformAdmin": id.PlatformAdmin,
				},
			})
```

If `apps/auth-service/internal/user/resource_test.go` asserts the meta block's
exact shape, update that assertion in the same commit.

- [ ] **Step 7: Run everything to green**

Run: `go test ./packages/shared-go/... ./apps/auth-service/... -count=1 && go build github.com/jtumidanski/myfleet/...`
Expected: PASS, including `TestNoPrincipalLiteralOutsideResolver` and the
extended `TestMintAccess_mapsEveryPrincipalField`.

- [ ] **Step 8: Commit**

```bash
git add packages/shared-go apps/auth-service
git commit -m "feat(auth): carry a platform_admin claim through mint, middleware and /auth/me"
```

---

### Task 11: `RequirePlatformAdmin` and the separation arch test

**Files:**
- Modify: `apps/fleet-service/internal/authz/scope.go`
- Modify: `apps/fleet-service/internal/authz/scope_test.go`
- Modify: `apps/fleet-service/internal/admin/arch_test.go`

**Interfaces:**
- Consumes: `auth.Identity.PlatformAdmin` from Task 10.
- Produces: `func authz.RequirePlatformAdmin(id auth.Identity) error`.

**Deviation from design §5.5, stated plainly.** The design puts the guard in
`internal/admin`; PRD FR-ADMIN-AUTH-6 names `authz.RequirePlatformAdmin`
explicitly and the guard belongs beside `RequireSameFleet` / `RequireOwner`. It
goes in `authz`, and the arch test allowlists `internal/authz/scope.go` and its
test as legitimate reference sites. The property the test protects is unchanged:
no **handler** outside `/admin` may read `PlatformAdmin`, and no `/admin` handler
may call `RequireSameFleet`.

- [ ] **Step 1: Write the failing guard test**

Append to `apps/fleet-service/internal/authz/scope_test.go`:

```go
// FR-ADMIN-AUTH-6: 403, not 404. This is the deliberate INVERSE of
// RequireSameFleet's non-disclosure rule — the existence of an admin API is not
// a secret, only the authority to use it.
func TestRequirePlatformAdmin(t *testing.T) {
	if err := RequirePlatformAdmin(auth.Identity{PlatformAdmin: true}); err != nil {
		t.Errorf("an admin must be allowed, got %v", err)
	}
	if err := RequirePlatformAdmin(auth.Identity{}); !errors.Is(err, server.ErrForbidden) {
		t.Errorf("a non-admin must get 403, got %v", err)
	}
	// A fleet role is not a substitute: owner of a fleet is not owner of the
	// platform (FR-ADMIN-AUTH-9's converse).
	if err := RequirePlatformAdmin(auth.Identity{Role: "owner", ActiveFleetID: "f1"}); !errors.Is(err, server.ErrForbidden) {
		t.Errorf("a fleet owner must not be a platform admin, got %v", err)
	}
	if errors.Is(RequirePlatformAdmin(auth.Identity{}), server.ErrNotFound) {
		t.Error("RequirePlatformAdmin must not hide behind a 404 the way RequireSameFleet does")
	}
}
```

- [ ] **Step 2: Run and watch it fail**

Run: `go test ./apps/fleet-service/internal/authz/ -run RequirePlatformAdmin -v`
Expected: FAIL — `undefined: RequirePlatformAdmin`.

- [ ] **Step 3: Add the guard**

Append to `apps/fleet-service/internal/authz/scope.go`:

```go
// RequirePlatformAdmin returns 403 when the caller is not a platform admin.
//
// 403, not 404: the existence of an admin API is not a secret, only the
// authority to use it. That is the deliberate inverse of RequireSameFleet's
// rule above, and the difference is intentional — RequireSameFleet hides other
// tenants' resources; there is only one platform and everyone knows it exists.
//
// It deliberately ignores ActiveFleetID. An administrator with no fleet —
// including one standing in the wreckage of the system purge they just ran —
// must still reach every admin endpoint (FR-ADMIN-AUTH-9).
func RequirePlatformAdmin(id auth.Identity) error {
	if !id.PlatformAdmin {
		return server.ErrForbidden
	}
	return nil
}
```

- [ ] **Step 4: Add the R7 separation arch test**

Append to `apps/fleet-service/internal/admin/arch_test.go`:

```go
// TestAdminTreeIsSeparate is the structural control behind the whole
// cross-fleet API (risks.md R7, design §5.5).
//
// The admin API bypasses RequireSameFleet. That bypass is safe ONLY because it
// lives in a parallel route tree rather than in a relaxed guard. Two ways that
// could rot:
//
//   - someone "simplifies" an ordinary handler by short-circuiting
//     RequireSameFleet when Identity.PlatformAdmin is true, which would make
//     every ordinary endpoint cross-fleet capable for an admin;
//   - someone adds RequireSameFleet inside /admin, which would silently break
//     the console for every fleet the operator is not a member of and invite the
//     first fix above.
//
// authz/scope.go is allowlisted: it DEFINES both guards, and a definition is not
// a use.
func TestAdminTreeIsSeparate(t *testing.T) {
	const internalRoot = ".."
	allowedPlatformAdminRefs := map[string]bool{
		"../authz/scope.go":      true,
		"../authz/scope_test.go": true,
	}

	err := filepath.WalkDir(internalRoot, func(path string, d os.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		inAdmin := strings.HasPrefix(filepath.ToSlash(path), "../admin/")
		src, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		text := string(src)

		if !inAdmin && !allowedPlatformAdminRefs[filepath.ToSlash(path)] {
			for _, ref := range []string{"PlatformAdmin", "RequirePlatformAdmin"} {
				if strings.Contains(text, ref) {
					t.Errorf("%s references %s outside internal/admin — the platform tier must not "+
						"leak into an ordinary handler; that is how RequireSameFleet gets relaxed", path, ref)
				}
			}
		}
		if inAdmin && strings.Contains(text, "RequireSameFleet") {
			t.Errorf("%s calls RequireSameFleet inside internal/admin — the admin tree is "+
				"deliberately fleet-agnostic; adding the guard here breaks every fleet the "+
				"operator is not a member of", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", internalRoot, err)
	}
}
```

This one greps rather than parses, unlike `TestManifestCoversEveryTable`. That is
deliberate: a *mention* of `PlatformAdmin` in a comment outside `/admin` is
itself a smell worth failing on, whereas a table name in a comment is not.

- [ ] **Step 5: Run to green**

Run: `go test ./apps/fleet-service/... -count=1`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add apps/fleet-service/internal
git commit -m "feat(fleet): add RequirePlatformAdmin and the admin-tree separation arch test"
```

---

### Task 12: auth-service internal admin routes (+ its `internal-deny` rule)

**Files:**
- Create: `apps/auth-service/internal/platformadmin/resource.go`
- Create: `apps/auth-service/internal/platformadmin/resource_test.go`
- Modify: `apps/auth-service/cmd/main.go`
- Modify: `deploy/k8s/overlays/main/ingressroute.yaml`

**Interfaces:**
- Produces:
  - `func platformadmin.InitializeInternalRoutes(log logrus.FieldLogger, db *gorm.DB) func(chi.Router)`
  - `GET /internal/admin/stats` → `{"users": 21}`
  - `GET /internal/admin/users?ids=a,b,c` → `{"users":[{"id","email","display_name"}]}`
  - `GET /internal/admin/platform-admins/{userId}` → 200 or 404
  - `const MaxInternalLookupIDs = 50`

**Why the deny rule ships here (design F2).** `auth-stripprefix` strips only
`/api`, and every auth route lives under `/auth/…`, so reaching `/internal/…`
would require the public path `/api/internal/…`, which matches no `/api/*` router
and falls through to the SPA catch-all. auth-service is safe **by accident**. The
rule ships anyway, because "safe by accident" is one prefix change away from "not
safe" and the rule costs six lines.

- [ ] **Step 1: Write the failing route test**

Create `apps/auth-service/internal/platformadmin/resource_test.go`:

```go
package platformadmin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
)

func newRouter(t *testing.T) (chi.Router, *gorm.DB) {
	t.Helper()
	db := newSeedDB(t)
	r := chi.NewRouter()
	InitializeInternalRoutes(logrus.New(), db)(r)
	return r, db
}

func TestInternalStats_countsUsers(t *testing.T) {
	r, db := newRouter(t)
	for _, id := range []string{"u1", "u2", "u3"} {
		if err := db.Exec(`INSERT INTO auth.users (id, google_sub, email) VALUES (?, ?, ?)`,
			id, "sub-"+id, id+"@example.com").Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/internal/admin/stats", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Users int `json:"users"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Users != 3 {
		t.Errorf("users = %d, want 3", body.Users)
	}
}

func TestInternalUsers_resolvesRequestedIDsOnly(t *testing.T) {
	r, db := newRouter(t)
	for _, id := range []string{"u1", "u2"} {
		if err := db.Exec(`INSERT INTO auth.users (id, google_sub, email, display_name)
		                   VALUES (?, ?, ?, ?)`, id, "sub-"+id, id+"@example.com", "Name "+id).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/internal/admin/users?ids=u1,missing", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		Users []struct {
			ID          string `json:"id"`
			Email       string `json:"email"`
			DisplayName string `json:"display_name"`
		} `json:"users"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Users) != 1 || body.Users[0].ID != "u1" || body.Users[0].Email != "u1@example.com" {
		t.Errorf("want only u1 resolved, got %+v", body.Users)
	}
}

// The paginated mode backs /admin/users (FR-ADMIN-FLEET-6). Total is the whole
// directory, not the page, so the console can render a page count.
func TestInternalUsers_paginatedMode(t *testing.T) {
	r, db := newRouter(t)
	for _, id := range []string{"u1", "u2", "u3"} {
		if err := db.Exec(`INSERT INTO auth.users (id, google_sub, email) VALUES (?, ?, ?)`,
			id, "sub-"+id, id+"@example.com").Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/internal/admin/users?page[number]=1&page[size]=2", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Users []struct {
			ID string `json:"id"`
		} `json:"users"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Users) != 2 {
		t.Errorf("page size 2 returned %d users", len(body.Users))
	}
	if body.Total != 3 {
		t.Errorf("total = %d, want the whole directory (3), not the page", body.Total)
	}
}

// The endpoint is unauthenticated, so its input must be bounded — the same
// ceiling media-service already applies to /internal/media.
func TestInternalUsers_rejectsAnOversizedIDList(t *testing.T) {
	r, _ := newRouter(t)
	ids := make([]string, MaxInternalLookupIDs+1)
	for i := range ids {
		ids[i] = "u"
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/internal/admin/users?ids="+strings.Join(ids, ","), nil))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", rec.Code)
	}
}

// FR-ADMIN-AUTH-7: this is the stale-claim re-verification fleet-service calls
// before an irreversible purge.
func TestInternalPlatformAdmins_reflectsTheTable(t *testing.T) {
	r, db := newRouter(t)
	if err := NewAdministrator(db).Grant("u1", BootstrapGrantedBy); err != nil {
		t.Fatalf("grant: %v", err)
	}

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/internal/admin/platform-admins/u1", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("a granted admin must be 200, got %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/internal/admin/platform-admins/u2", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("a non-admin must be 404, got %d", rec.Code)
	}
}
```

Add the `gorm.io/gorm` import.

- [ ] **Step 2: Run and watch it fail**

Run: `go test ./apps/auth-service/internal/platformadmin/ -count=1 -v`
Expected: FAIL — `undefined: InitializeInternalRoutes`.

- [ ] **Step 3: Write the routes**

Create `apps/auth-service/internal/platformadmin/resource.go`:

```go
package platformadmin

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// MaxInternalLookupIDs bounds a single /internal/admin/users lookup. The
// endpoint is unauthenticated, so its input must be bounded; fleet-service
// chunks larger member sets. Mirrors mediaobject.MaxInternalLookupIDs.
const MaxInternalLookupIDs = 50

// InternalUser is one resolved user in the internal lookup response.
type InternalUser struct {
	ID          string     `json:"id"`
	Email       string     `json:"email"`
	DisplayName string     `json:"display_name"`
	CreatedAt   time.Time  `json:"created_at"`
	LastLoginAt *time.Time `json:"last_login_at"`
}

// internalUsersResponse carries Total so the paginated mode can report a page
// count. In the ids mode Total is simply the number resolved.
type internalUsersResponse struct {
	Users []InternalUser `json:"users"`
	Total int            `json:"total"`
}

type internalStatsResponse struct {
	Users int `json:"users"`
}

// InitializeInternalRoutes wires the network-restricted admin endpoints.
// Register this initializer WITHOUT JWT middleware, exactly as fleet-service
// does for membership.InitializeInternalRoutes.
//
// SECURITY: these routes have no authentication. auth-stripprefix strips only
// `/api`, and every public auth route lives under `/auth/…`, so a public request
// would have to be `/api/internal/…` — which matches no `/api/*` router and
// falls through to the SPA catch-all. That makes this surface safe BY ACCIDENT,
// which is one prefix change away from not safe, so the priority-200
// internal-deny rule in deploy/k8s/overlays/main/ingressroute.yaml ships with
// it. The two go together; never separately (design F2).
func InitializeInternalRoutes(log logrus.FieldLogger, db *gorm.DB) func(chi.Router) {
	prov := NewProvider(db)
	return func(r chi.Router) {
		// GET /internal/admin/stats → { "users": N }
		r.Get("/internal/admin/stats", func(w http.ResponseWriter, req *http.Request) {
			var n int64
			if err := db.Raw(`SELECT count(*) FROM auth.users`).Scan(&n).Error; err != nil {
				log.WithError(err).Error("internal admin user count")
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, internalStatsResponse{Users: int(n)})
		})

		// GET /internal/admin/users — two modes on one route.
		//
		//   ?ids=a,b,c              resolve a known set (fleet detail's member
		//                           names). A missing id is simply absent rather
		//                           than an error: fleet-service resolves names
		//                           best-effort and warns if the lookup came
		//                           back short (FR-ADMIN-FLEET-5).
		//   ?page[number]=&page[size]=  the paginated directory behind
		//                           /admin/users (FR-ADMIN-FLEET-6).
		//
		// One route rather than two because the shape returned is identical and
		// the caller's intent is unambiguous from the parameters.
		r.Get("/internal/admin/users", func(w http.ResponseWriter, req *http.Request) {
			if raw := req.URL.Query().Get("ids"); raw != "" || req.URL.Query().Get("page[size]") == "" {
				ids := splitIDs(raw)
				if len(ids) > MaxInternalLookupIDs {
					server.WriteError(w, server.ErrValidation)
					return
				}
				if len(ids) == 0 {
					server.WriteJSON(w, http.StatusOK, internalUsersResponse{Users: []InternalUser{}})
					return
				}
				var rows []InternalUser
				if err := db.Raw(`SELECT id, email, display_name, created_at, last_login_at
				                  FROM auth.users WHERE id IN ?`, ids).Scan(&rows).Error; err != nil {
					log.WithError(err).Error("internal admin user lookup")
					server.WriteError(w, err)
					return
				}
				if rows == nil {
					rows = []InternalUser{}
				}
				server.WriteJSON(w, http.StatusOK, internalUsersResponse{Users: rows, Total: len(rows)})
				return
			}

			page := server.ParsePage(req)
			var total int64
			if err := db.Raw(`SELECT count(*) FROM auth.users`).Scan(&total).Error; err != nil {
				log.WithError(err).Error("internal admin user count")
				server.WriteError(w, err)
				return
			}
			var rows []InternalUser
			if err := db.Raw(`SELECT id, email, display_name, created_at, last_login_at
			                  FROM auth.users ORDER BY created_at DESC LIMIT ? OFFSET ?`,
				page.Size, page.Offset()).Scan(&rows).Error; err != nil {
				log.WithError(err).Error("internal admin user list")
				server.WriteError(w, err)
				return
			}
			if rows == nil {
				rows = []InternalUser{}
			}
			server.WriteJSON(w, http.StatusOK, internalUsersResponse{Users: rows, Total: int(total)})
		})

		// GET /internal/admin/platform-admins/{userId} → 200 | 404.
		//
		// This is the stale-claim re-verification (FR-ADMIN-AUTH-7): the claim
		// is stamped at mint time, so a revoked admin holds a valid token for up
		// to 15 minutes. fleet-service calls this before an irreversible purge
		// and fails closed on an error.
		r.Get("/internal/admin/platform-admins/{userId}", func(w http.ResponseWriter, req *http.Request) {
			userID := chi.URLParam(req, "userId")
			ok, err := prov.IsAdmin(userID)
			if err != nil {
				log.WithError(err).Error("internal platform-admin lookup")
				server.WriteError(w, err)
				return
			}
			if !ok {
				server.WriteError(w, server.ErrNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
		})
	}
}

// splitIDs parses the comma-separated ids parameter, dropping empty segments so
// a trailing comma is not a lookup for "".
func splitIDs(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if id := strings.TrimSpace(p); id != "" {
			out = append(out, id)
		}
	}
	return out
}
```

- [ ] **Step 4: Register the routes without JWT**

`apps/auth-service/cmd/main.go`, alongside the other public initializers:

```go
		AddRouteInitializer(session.InitializePublicRoutes(log, sess, resolve, cookieSecure)).
		// Internal routes: no JWT, network-restricted (consumed by
		// fleet-service's admin console). Kept off the public internet by the
		// priority-200 internal-deny rule in the main overlay's ingressroute.
		AddRouteInitializer(platformadmin.InitializeInternalRoutes(log, db)).
```

- [ ] **Step 5: Ship the deny rule in the same commit**

`deploy/k8s/overlays/main/ingressroute.yaml`, immediately after the media-service
deny rule and before the priority-100 routes:

```yaml
    # auth-service registers /internal/admin/* WITHOUT JWT
    # (apps/auth-service/internal/platformadmin/resource.go). Those routes are a
    # cross-tenant user directory and the stale-claim oracle the purge path
    # depends on.
    #
    # auth-service is safe by accident today: auth-stripprefix strips only
    # `/api`, and every public auth route lives under `/auth/…`, so reaching
    # /internal/… would need the public path /api/internal/…, which matches no
    # /api/* router and falls through to the SPA catch-all. "Safe by accident"
    # is one prefix change away from "not safe" and this rule costs six lines.
    #
    # PathRegexp with `[^/]*/*` for the same reason as the two rules above:
    # stripPrefix removes a literal string, not a path segment.
    #
    # The service reference is inert: internal-deny short-circuits with 403
    # before anything is proxied.
    - match: (Host(`myfleet.tumidanski.com`) || Host(`myfleet.tumidanski.me`) || Host(`myfleet.home`)) && PathRegexp(`(?i)^/+api/+auth[^/]*/*internal`)
      kind: Rule
      priority: 200
      middlewares:
        - name: internal-deny
      services:
        - name: auth-service
          port: 8080
```

It goes in `myfleet-routes` only — the `replacements` block copies `spec.routes`
verbatim into the TLS twin, so both entrypoints get it and cannot drift.

- [ ] **Step 6: Verify code and manifests**

Run:
```sh
go test ./apps/auth-service/... -count=1
kustomize build deploy/k8s/overlays/main | grep -c 'api/+auth\[\^/\]\*/\*internal'
```
Expected: tests PASS; the grep returns `2` (the rule appears in both the plain
and the TLS IngressRoute).

- [ ] **Step 7: Commit**

```bash
git add apps/auth-service deploy/k8s
git commit -m "feat(auth): add internal admin routes with their internal-deny routing rule"
```

---

# Phase 3 — Downstream purge contracts (Tasks 13–15)

---

### Task 13: media-service admin manifest and internal routes (+ reuse of its deny rule)

**Files:**
- Create: `apps/media-service/internal/admin/manifest.go`
- Create: `apps/media-service/internal/admin/operations.go`
- Create: `apps/media-service/internal/admin/resource.go`
- Create: `apps/media-service/internal/admin/admin_test.go`
- Create: `apps/media-service/internal/admin/arch_test.go`
- Modify: `apps/media-service/cmd/main.go`

**Interfaces:**
- Produces, in `apps/media-service/internal/admin`:
  - `type Scope string`; `ScopeSystem`, `ScopeFleet`, `ScopeMediaIDs`
  - `type Root struct { Scope Scope; FleetIDs []string; MediaIDs []string }`
  - `type Target struct { Key, Table string; Where func(Root) (string, []any) }`, `var Manifest []Target`
  - `func Count(db, root) (map[string]int, error)`, `CountByOperation`, `Stamp(tx, root, opID, now)`, `Restore(tx, opID)`, `Reap(tx, opID)`
  - `func ReapableObjectKeys(db *gorm.DB, opID string) ([]string, error)` — MinIO keys to remove, read **before** the rows go
  - `func InitializeInternalRoutes(log, db, store ObjectRemover) func(chi.Router)`
  - `type ObjectRemover interface { RemoveObject(ctx context.Context, key string) error }`
- HTTP: `GET /internal/admin/stats`, `POST /internal/admin/purge`,
  `DELETE /internal/admin/purge/{opId}`, `POST /internal/admin/reap/{opId}`

**Request shape (one body, both downstream services):**

```json
{ "operation_id": "9c1b…", "scope": "system" }
{ "operation_id": "9c1b…", "scope": "fleet",     "fleet_ids": ["3f2a…"] }
{ "operation_id": "9c1b…", "scope": "media_ids", "media_ids": ["…"] }
```

`media_ids` is the only place explicit id sets survive: a `record`-scope purge of
one vehicle, where fleet-service must name the media objects belonging to that
vehicle. Everything else resolves through `fleet_id`, which `media.media_objects`
carries as a NOT NULL indexed column (design OQ-1 — PRD §6.5 is superseded).

media-service already has a priority-200 `internal-deny` rule covering
`^/+api/+media[^/]*/*internal`, so these routes are covered the moment they exist.
**Verify that, do not assume it** — Step 6.

- [ ] **Step 1: Write the failing test**

Create `apps/media-service/internal/admin/admin_test.go`:

```go
package admin_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/apps/media-service/internal/admin"
)

var testNow = time.Date(2026, 8, 2, 14, 3, 11, 0, time.UTC)

// recordingRemover captures the MinIO keys a reap asks to delete, and can be
// told to fail for one of them.
type recordingRemover struct {
	removed []string
	fail    map[string]bool
}

func (r *recordingRemover) RemoveObject(_ context.Context, key string) error {
	if r.fail[key] {
		return context.DeadlineExceeded
	}
	r.removed = append(r.removed, key)
	return nil
}

func newMediaDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec("ATTACH DATABASE ':memory:' AS media").Error; err != nil {
		t.Fatalf("attach media schema: %v", err)
	}
	ddl := []string{
		`CREATE TABLE media.media_objects (
			id TEXT PRIMARY KEY, fleet_id TEXT, uploaded_by_user_id TEXT, bucket TEXT,
			object_key TEXT, content_type TEXT, size INTEGER, original_filename TEXT,
			status TEXT, created_at DATETIME, deleted_at DATETIME, purge_after DATETIME,
			purge_operation_id TEXT)`,
		`CREATE TABLE media.media_variants (
			id TEXT PRIMARY KEY, media_object_id TEXT, variant TEXT, object_key TEXT,
			width INTEGER, height INTEGER, content_type TEXT, created_at DATETIME,
			deleted_at DATETIME, purge_operation_id TEXT)`,
		`CREATE TABLE media.processed_events (event_id TEXT PRIMARY KEY, processed_at DATETIME)`,
	}
	for _, stmt := range ddl {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("ddl: %v", err)
		}
	}
	seed := []string{
		`INSERT INTO media.media_objects (id, fleet_id, bucket, object_key, status)
		 VALUES ('mo-1', 'fleet-1', 'media', 'k/mo-1', 'ready')`,
		`INSERT INTO media.media_objects (id, fleet_id, bucket, object_key, status)
		 VALUES ('mo-2', 'fleet-2', 'media', 'k/mo-2', 'ready')`,
		`INSERT INTO media.media_variants (id, media_object_id, variant, object_key)
		 VALUES ('mv-1', 'mo-1', 'thumb', 'k/mo-1-thumb')`,
		`INSERT INTO media.media_variants (id, media_object_id, variant, object_key)
		 VALUES ('mv-2', 'mo-2', 'thumb', 'k/mo-2-thumb')`,
		`INSERT INTO media.processed_events (event_id, processed_at)
		 VALUES ('evt-1', CURRENT_TIMESTAMP)`,
	}
	for _, stmt := range seed {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	return db
}

func newMediaRouter(t *testing.T, db *gorm.DB, store admin.ObjectRemover) chi.Router {
	t.Helper()
	r := chi.NewRouter()
	admin.InitializeInternalRoutes(logrus.New(), db, store)(r)
	return r
}

func post(t *testing.T, r chi.Router, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	return rec
}

// design OQ-1: a fleet-scoped media purge is WHERE fleet_id = ?. No id-set
// passing, and no way to reach another tenant's media.
func TestPurge_fleetScope_takesOnlyThatFleet(t *testing.T) {
	db := newMediaDB(t)
	r := newMediaRouter(t, db, &recordingRemover{})

	rec := post(t, r, "/internal/admin/purge",
		`{"operation_id":"op-1","scope":"fleet","fleet_ids":["fleet-1"]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Affected map[string]int `json:"affected"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Affected["media_objects"] != 1 || body.Affected["media_variants"] != 1 {
		t.Errorf("affected = %+v, want one object and one variant", body.Affected)
	}

	var liveOther int64
	db.Raw(`SELECT count(*) FROM media.media_objects
	        WHERE fleet_id = 'fleet-2' AND deleted_at IS NULL`).Scan(&liveOther)
	if liveOther != 1 {
		t.Errorf("a fleet purge reached another tenant's media: %d of 1 live", liveOther)
	}
}

// FR-ADMIN-PURGE-10: a replay is a no-op that returns the SAME counts.
func TestPurge_isIdempotent(t *testing.T) {
	db := newMediaDB(t)
	r := newMediaRouter(t, db, &recordingRemover{})
	const body = `{"operation_id":"op-1","scope":"fleet","fleet_ids":["fleet-1"]}`

	decode := func(rec *httptest.ResponseRecorder) map[string]int {
		t.Helper()
		var out struct {
			Affected map[string]int `json:"affected"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return out.Affected
	}
	first := decode(post(t, r, "/internal/admin/purge", body))
	second := decode(post(t, r, "/internal/admin/purge", body))
	for k, want := range first {
		if second[k] != want {
			t.Errorf("%s: replay returned %d, first call %d", k, second[k], want)
		}
	}
}

// The record-scope path: fleet-service names the media objects belonging to one
// vehicle, and only those (plus their variants) are taken.
func TestPurge_mediaIDsScope(t *testing.T) {
	db := newMediaDB(t)
	r := newMediaRouter(t, db, &recordingRemover{})

	rec := post(t, r, "/internal/admin/purge",
		`{"operation_id":"op-1","scope":"media_ids","media_ids":["mo-1"]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var live int64
	db.Raw(`SELECT count(*) FROM media.media_variants WHERE deleted_at IS NULL`).Scan(&live)
	if live != 1 {
		t.Errorf("want mo-2's variant still live, got %d live variants", live)
	}
}

func TestRestore_returnsEverythingTheOperationTook(t *testing.T) {
	db := newMediaDB(t)
	r := newMediaRouter(t, db, &recordingRemover{})
	post(t, r, "/internal/admin/purge", `{"operation_id":"op-1","scope":"system"}`)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/internal/admin/purge/op-1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var live int64
	db.Raw(`SELECT count(*) FROM media.media_objects WHERE deleted_at IS NULL`).Scan(&live)
	if live != 2 {
		t.Errorf("restore returned %d of 2 media objects", live)
	}
}

// FR-ADMIN-RESTORE-5: reap removes the MinIO objects too — the media object's
// key AND every variant's key.
func TestReap_removesRowsAndObjects(t *testing.T) {
	db := newMediaDB(t)
	store := &recordingRemover{}
	r := newMediaRouter(t, db, store)
	post(t, r, "/internal/admin/purge", `{"operation_id":"op-1","scope":"fleet","fleet_ids":["fleet-1"]}`)

	rec := post(t, r, "/internal/admin/reap/op-1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	want := map[string]bool{"k/mo-1": true, "k/mo-1-thumb": true}
	for _, key := range store.removed {
		delete(want, key)
	}
	if len(want) != 0 {
		t.Errorf("reap did not remove these MinIO objects: %v (removed %v)", want, store.removed)
	}
	var rows int64
	db.Raw(`SELECT count(*) FROM media.media_objects WHERE purge_operation_id = 'op-1'`).Scan(&rows)
	if rows != 0 {
		t.Errorf("reap left %d stamped rows behind", rows)
	}
}

// A MinIO object that cannot be removed must leave its ROW in place, so the
// next tick retries. Deleting the row would strand the object forever with
// nothing left pointing at it.
func TestReap_keepsRowsWhoseObjectCouldNotBeRemoved(t *testing.T) {
	db := newMediaDB(t)
	store := &recordingRemover{fail: map[string]bool{"k/mo-1": true}}
	r := newMediaRouter(t, db, store)
	post(t, r, "/internal/admin/purge", `{"operation_id":"op-1","scope":"fleet","fleet_ids":["fleet-1"]}`)

	rec := post(t, r, "/internal/admin/reap/op-1", "")
	if rec.Code == http.StatusOK {
		t.Errorf("a failed object removal must not report success, got %d", rec.Code)
	}
	var rows int64
	db.Raw(`SELECT count(*) FROM media.media_objects WHERE id = 'mo-1'`).Scan(&rows)
	if rows != 1 {
		t.Errorf("the row whose object survived must be kept for the next tick, got %d rows", rows)
	}
}

// design §3.3: deleting the idempotency ledger would let a Kafka replay
// regenerate variants for media that was just purged.
func TestSystemPurge_leavesProcessedEventsAlone(t *testing.T) {
	db := newMediaDB(t)
	r := newMediaRouter(t, db, &recordingRemover{})
	post(t, r, "/internal/admin/purge", `{"operation_id":"op-1","scope":"system"}`)
	post(t, r, "/internal/admin/reap/op-1", "")

	var rows int64
	db.Raw(`SELECT count(*) FROM media.processed_events`).Scan(&rows)
	if rows != 1 {
		t.Errorf("processed_events must survive a system purge, got %d rows", rows)
	}
}

func TestStats_countsLiveMediaObjects(t *testing.T) {
	db := newMediaDB(t)
	r := newMediaRouter(t, db, &recordingRemover{})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/internal/admin/stats", nil))
	var body struct {
		MediaObjects int `json:"media_objects"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.MediaObjects != 2 {
		t.Errorf("media_objects = %d, want 2", body.MediaObjects)
	}
}
```

- [ ] **Step 2: Run and watch it fail**

Run: `go test ./apps/media-service/internal/admin/ -count=1 -v`
Expected: FAIL — the package does not exist.

- [ ] **Step 3: Write the manifest and operations**

Create `apps/media-service/internal/admin/manifest.go`:

```go
// Package admin is media-service's slice of the platform purge protocol: stamp,
// restore, reap, and a stats count, all reachable only on the internal route
// tree.
//
// media.media_objects carries a NOT NULL indexed fleet_id, so a fleet-scoped
// purge is simply WHERE fleet_id IN (…) — no id-set passing (design OQ-1).
// Explicit ids survive in exactly one place: a record-scope purge of a single
// vehicle, where fleet-service must name the media objects that vehicle owns.
package admin

// Scope is what a downstream purge is rooted at.
type Scope string

const (
	ScopeSystem   Scope = "system"
	ScopeFleet    Scope = "fleet"
	ScopeMediaIDs Scope = "media_ids"
)

// Root is the resolved purge root for this service.
type Root struct {
	Scope    Scope
	FleetIDs []string
	MediaIDs []string
}

// Target is one purgeable table and how to resolve its rows.
type Target struct {
	Key   string
	Table string
	// Where returns the predicate + args, or ("", nil) when out of scope.
	// It never filters deleted_at on itself or on a parent — see fleet-service's
	// admin.Target for why the ordering property depends on that.
	Where func(Root) (string, []any)
}

const all = "1 = 1"

// Manifest is media-service's purge surface, child to parent.
var Manifest = []Target{
	{
		Key: "media_variants", Table: "media.media_variants",
		Where: func(r Root) (string, []any) {
			switch r.Scope {
			case ScopeSystem:
				return all, nil
			case ScopeFleet:
				return "media_object_id IN (SELECT id FROM media.media_objects WHERE fleet_id IN ?)",
					[]any{r.FleetIDs}
			case ScopeMediaIDs:
				return "media_object_id IN ?", []any{r.MediaIDs}
			}
			return "", nil
		},
	},
	{
		Key: "media_objects", Table: "media.media_objects",
		Where: func(r Root) (string, []any) {
			switch r.Scope {
			case ScopeSystem:
				return all, nil
			case ScopeFleet:
				return "fleet_id IN ?", []any{r.FleetIDs}
			case ScopeMediaIDs:
				return "id IN ?", []any{r.MediaIDs}
			}
			return "", nil
		},
	},
}

// excludedTables documents the deliberate omissions.
var excludedTables = map[string]string{
	// This is a finding, not bookkeeping. The PRD's "all of media.*" phrasing,
	// taken literally, would truncate the idempotency ledger — and a Kafka
	// replay would then regenerate variants for media that was just purged,
	// making a system purge undo itself on the next consumer restart.
	"media.processed_events": "idempotency ledger; deleting it lets a Kafka replay resurrect purged media",
	"outbox":                 "transient relay ledger drained by the outbox relay; owned by no fleet",
}
```

Create `apps/media-service/internal/admin/operations.go`. The bodies are the same
four generics as fleet-service's, over this manifest, plus one media-specific
helper:

```go
package admin

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Count returns per-key LIVE row counts for the root.
func Count(db *gorm.DB, root Root) (map[string]int, error) {
	out := make(map[string]int, len(Manifest))
	for _, t := range Manifest {
		pred, args := t.Where(root)
		if pred == "" {
			continue
		}
		var n int64
		q := "SELECT count(*) FROM " + t.Table + " WHERE (" + pred + ") AND deleted_at IS NULL"
		if err := db.Raw(q, args...).Scan(&n).Error; err != nil {
			return nil, fmt.Errorf("count %s: %w", t.Table, err)
		}
		out[t.Key] = int(n)
	}
	return out, nil
}

// CountByOperation returns per-key rows currently carrying opID.
func CountByOperation(db *gorm.DB, opID string) (map[string]int, error) {
	out := make(map[string]int, len(Manifest))
	for _, t := range Manifest {
		var n int64
		if err := db.Raw("SELECT count(*) FROM "+t.Table+" WHERE purge_operation_id = ?", opID).
			Scan(&n).Error; err != nil {
			return nil, fmt.Errorf("count by operation %s: %w", t.Table, err)
		}
		out[t.Key] = int(n)
	}
	return out, nil
}

// Stamp soft-deletes the root's rows under opID and returns the counts the
// operation now carries.
//
// It never writes purge_after. media-service runs its OWN 24-hour sweep keyed on
// that column, which hard-deletes rows and their MinIO objects; leaving it NULL
// is what keeps an admin-stamped, still-cancellable object out of that sweep's
// reach (design F3).
//
// The counts are read back after the update rather than summed from rows
// affected, so a replay returns the same numbers instead of zeros
// (FR-ADMIN-PURGE-10).
func Stamp(tx *gorm.DB, root Root, opID string, now time.Time) (map[string]int, error) {
	for _, t := range Manifest {
		pred, args := t.Where(root)
		if pred == "" {
			continue
		}
		q := "UPDATE " + t.Table + " SET deleted_at = ?, purge_operation_id = ?" +
			" WHERE (" + pred + ") AND deleted_at IS NULL"
		if err := tx.Exec(q, append([]any{now, opID}, args...)...).Error; err != nil {
			return nil, fmt.Errorf("stamp %s: %w", t.Table, err)
		}
	}
	return CountByOperation(tx, opID)
}

// Restore clears the soft-delete on every row carrying opID.
func Restore(tx *gorm.DB, opID string) error {
	for _, t := range Manifest {
		q := "UPDATE " + t.Table +
			" SET deleted_at = NULL, purge_operation_id = NULL WHERE purge_operation_id = ?"
		if err := tx.Exec(q, opID).Error; err != nil {
			return fmt.Errorf("restore %s: %w", t.Table, err)
		}
	}
	return nil
}

// Reap hard-deletes every row carrying opID.
func Reap(tx *gorm.DB, opID string) (map[string]int, error) {
	out := make(map[string]int, len(Manifest))
	for _, t := range Manifest {
		res := tx.Exec("DELETE FROM "+t.Table+" WHERE purge_operation_id = ?", opID)
		if res.Error != nil {
			return nil, fmt.Errorf("reap %s: %w", t.Table, res.Error)
		}
		out[t.Key] = int(res.RowsAffected)
	}
	return out, nil
}

// ObjectKey pairs a stored object with the row that owns it, so a failed removal
// can spare exactly that row.
type ObjectKey struct {
	MediaObjectID string
	Key           string
}

// ReapableObjectKeys returns every MinIO key belonging to opID — the media
// objects' own keys and their variants' — grouped by owning media object.
//
// It must be called BEFORE Reap: the rows are the only record of which objects
// exist, so deleting them first would strand the bytes in the bucket with
// nothing left pointing at them.
func ReapableObjectKeys(db *gorm.DB, opID string) ([]ObjectKey, error) {
	var out []ObjectKey
	q := `SELECT id AS media_object_id, object_key AS key
	      FROM media.media_objects WHERE purge_operation_id = ?
	      UNION ALL
	      SELECT media_object_id AS media_object_id, object_key AS key
	      FROM media.media_variants WHERE purge_operation_id = ?`
	if err := db.Raw(q, opID, opID).Scan(&out).Error; err != nil {
		return nil, fmt.Errorf("list reapable object keys: %w", err)
	}
	return out, nil
}
```

- [ ] **Step 4: Write the internal routes**

Create `apps/media-service/internal/admin/resource.go`:

```go
package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// ObjectRemover is the slice of storage.Client this package needs. Declaring the
// port here rather than importing the concrete client keeps the dependency
// one-way and makes the reap testable without MinIO.
type ObjectRemover interface {
	RemoveObject(ctx context.Context, key string) error
}

// PurgeRequest is the one body shape both downstream services accept.
type PurgeRequest struct {
	OperationID string   `json:"operation_id"`
	Scope       Scope    `json:"scope"`
	FleetIDs    []string `json:"fleet_ids,omitempty"`
	MediaIDs    []string `json:"media_ids,omitempty"`
}

type affectedResponse struct {
	Affected map[string]int `json:"affected"`
}

type statsResponse struct {
	MediaObjects int `json:"media_objects"`
}

// InitializeInternalRoutes wires media-service's slice of the purge protocol.
// Register WITHOUT JWT middleware.
//
// SECURITY: these routes have no authentication and they DELETE DATA. The
// priority-200 internal-deny rule matching ^/+api/+media[^/]*/*internal in
// deploy/k8s/overlays/main/ingressroute.yaml is what keeps them off the public
// internet. It predates this task because /internal/media already existed —
// confirm it still matches before shipping; never assume it (design F2).
func InitializeInternalRoutes(log logrus.FieldLogger, db *gorm.DB, store ObjectRemover) func(chi.Router) {
	return func(r chi.Router) {
		r.Get("/internal/admin/stats", func(w http.ResponseWriter, req *http.Request) {
			var n int64
			if err := db.Raw(`SELECT count(*) FROM media.media_objects WHERE deleted_at IS NULL`).
				Scan(&n).Error; err != nil {
				log.WithError(err).Error("internal admin media count")
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, statsResponse{MediaObjects: int(n)})
		})

		r.Post("/internal/admin/purge", func(w http.ResponseWriter, req *http.Request) {
			var body PurgeRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				server.WriteError(w, server.ErrValidation)
				return
			}
			root, err := rootFrom(body)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			var affected map[string]int
			if terr := db.Transaction(func(tx *gorm.DB) error {
				var serr error
				affected, serr = Stamp(tx, root, body.OperationID, time.Now().UTC())
				return serr
			}); terr != nil {
				log.WithError(terr).WithField("operation_id", body.OperationID).Error("media admin stamp")
				server.WriteError(w, terr)
				return
			}
			server.WriteJSON(w, http.StatusOK, affectedResponse{Affected: affected})
		})

		r.Delete("/internal/admin/purge/{opId}", func(w http.ResponseWriter, req *http.Request) {
			opID := chi.URLParam(req, "opId")
			if terr := db.Transaction(func(tx *gorm.DB) error { return Restore(tx, opID) }); terr != nil {
				log.WithError(terr).WithField("operation_id", opID).Error("media admin restore")
				server.WriteError(w, terr)
				return
			}
			affected, err := CountByOperation(db, opID)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, affectedResponse{Affected: affected})
		})

		r.Post("/internal/admin/reap/{opId}", func(w http.ResponseWriter, req *http.Request) {
			opID := chi.URLParam(req, "opId")

			// Objects first, rows second. The rows are the only record of which
			// objects exist, so deleting them first would strand the bytes in
			// the bucket with nothing left pointing at them.
			keys, err := ReapableObjectKeys(db, opID)
			if err != nil {
				log.WithError(err).WithField("operation_id", opID).Error("list reapable objects")
				server.WriteError(w, err)
				return
			}
			failed := map[string]bool{}
			for _, k := range keys {
				if rerr := store.RemoveObject(req.Context(), k.Key); rerr != nil {
					// An object already absent is NOT an error
					// (FR-ADMIN-RESTORE-5) — storage.Client.RemoveObject is
					// idempotent on a missing key, so anything that reaches here
					// is a real failure. Keep the owning row so the next tick
					// retries.
					log.WithError(rerr).WithFields(logrus.Fields{
						"operation_id": opID, "object_key": k.Key,
					}).Warn("remove minio object during admin reap failed")
					failed[k.MediaObjectID] = true
				}
			}

			var deleted map[string]int
			if terr := db.Transaction(func(tx *gorm.DB) error {
				if len(failed) > 0 {
					ids := make([]string, 0, len(failed))
					for id := range failed {
						ids = append(ids, id)
					}
					// Spare the objects whose bytes are still in the bucket, and
					// their variants with them.
					if err := tx.Exec(`UPDATE media.media_objects SET purge_operation_id = NULL
					                   WHERE purge_operation_id = ? AND id IN ?`, opID, ids).Error; err != nil {
						return err
					}
					if err := tx.Exec(`UPDATE media.media_variants SET purge_operation_id = NULL
					                   WHERE purge_operation_id = ? AND media_object_id IN ?`, opID, ids).Error; err != nil {
						return err
					}
				}
				var rerr error
				deleted, rerr = Reap(tx, opID)
				return rerr
			}); terr != nil {
				log.WithError(terr).WithField("operation_id", opID).Error("media admin reap")
				server.WriteError(w, terr)
				return
			}

			if len(failed) > 0 {
				// Report failure so fleet-service leaves the operation pending
				// and the next hourly tick retries the survivors.
				server.WriteError(w, server.Detailed(server.ErrConflict,
					"some stored objects could not be removed; their rows were kept for retry"))
				return
			}
			server.WriteJSON(w, http.StatusOK, affectedResponse{Affected: deleted})
		})
	}
}

// rootFrom validates the request body into a Root, or returns 422.
func rootFrom(body PurgeRequest) (Root, error) {
	if body.OperationID == "" {
		return Root{}, server.Detailed(server.ErrValidation, "operation_id is required")
	}
	switch body.Scope {
	case ScopeSystem:
		return Root{Scope: ScopeSystem}, nil
	case ScopeFleet:
		if len(body.FleetIDs) == 0 {
			return Root{}, server.Detailed(server.ErrValidation, "fleet scope requires fleet_ids")
		}
		return Root{Scope: ScopeFleet, FleetIDs: body.FleetIDs}, nil
	case ScopeMediaIDs:
		if len(body.MediaIDs) == 0 {
			return Root{}, server.Detailed(server.ErrValidation, "media_ids scope requires media_ids")
		}
		return Root{Scope: ScopeMediaIDs, MediaIDs: body.MediaIDs}, nil
	}
	return Root{}, server.Detailed(server.ErrValidation, "unsupported scope")
}
```

- [ ] **Step 5: Add the completeness arch test and wire the routes**

Create `apps/media-service/internal/admin/arch_test.go` — the same shape as
fleet-service's `TestManifestCoversEveryTable`, walking `..` (i.e.
`apps/media-service/internal`) and checking each `TableName()` literal against
`Manifest` / `excludedTables`. Copy that test verbatim, changing only the package
clause and the two identifiers it references.

`apps/media-service/cmd/main.go`:

```go
		// Internal routes: no JWT, network-restricted. Both are kept off the
		// public internet by the priority-200 internal-deny rule in the main
		// overlay's ingressroute.
		AddRouteInitializer(mediaobject.InitializeInternalRoutes(log, db)).
		AddRouteInitializer(admin.InitializeInternalRoutes(log, db, store)).
```

`storage.Client` already has `RemoveObject(ctx, key) error`
(`internal/storage/minio.go:207`), so it satisfies `admin.ObjectRemover` with no
adapter.

- [ ] **Step 6: Verify the tests AND the deny rule**

Run:
```sh
go test ./apps/media-service/... -count=1
kustomize build deploy/k8s/overlays/main | grep -c 'api/+media\[\^/\]\*/\*internal'
```
Expected: tests PASS; the grep returns `2`. If it returns anything else, the
media deny rule is not covering these routes and must be fixed before merging —
that is an unauthenticated, internet-reachable "delete this fleet's media"
endpoint.

- [ ] **Step 7: Commit**

```bash
git add apps/media-service
git commit -m "feat(media): add the internal admin purge, restore and reap contract"
```

---

### Task 14: notification-service admin manifest and internal routes (+ its `internal-deny` rule)

**Files:**
- Create: `apps/notification-service/internal/admin/manifest.go`
- Create: `apps/notification-service/internal/admin/operations.go`
- Create: `apps/notification-service/internal/admin/resource.go`
- Create: `apps/notification-service/internal/admin/admin_test.go`
- Create: `apps/notification-service/internal/admin/arch_test.go`
- Modify: `apps/notification-service/cmd/main.go`
- Modify: `deploy/k8s/overlays/main/ingressroute.yaml`

**Interfaces:** identical to Task 13 minus the MinIO half: `Scope` is
`system | fleet`, `Root` is `{Scope; FleetIDs []string}`, and the four operations
plus `InitializeInternalRoutes(log, db)`.

**This is the critical one (design F2).** `notifications-stripprefix` strips the
**full** `/api/notifications` prefix, so a public request to
`/api/notifications/internal/admin/purge` arrives at notification-service as
`/internal/admin/purge`. notification-service has no `/internal/*` routes today,
which is exactly why the `internal-deny` rule covers only fleet-service and
media-service. **This task adds the first internal routes to notification-service,
and they are destructive.** Without a matching deny rule they are an
unauthenticated, internet-reachable "delete everything for this fleet" endpoint.
The rule ships in this commit or the routes do not ship.

**And notifications are never keyed on `user_id`** (design OQ-2). A fleet purge is
`WHERE fleet_id = ?`. That is what structurally avoids R4's cross-tenant trap —
a user in two fleets losing both streams — rather than avoiding it by care.
`notification_preferences` has no fleet linkage at all, so it is **excluded from
fleet scope** and taken only by a system purge; that is safe because preferences
regenerate with defaults on next read.

- [ ] **Step 1: Write the failing test**

Create `apps/notification-service/internal/admin/admin_test.go`:

```go
package admin_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/apps/notification-service/internal/admin"
)

func newNotificationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec("ATTACH DATABASE ':memory:' AS notification").Error; err != nil {
		t.Fatalf("attach notification schema: %v", err)
	}
	ddl := []string{
		`CREATE TABLE notification.notifications (
			id TEXT PRIMARY KEY, user_id TEXT, type TEXT, title TEXT, body TEXT,
			dedupe_key TEXT, vehicle_id TEXT, fleet_id TEXT, read_at DATETIME,
			created_at DATETIME, deleted_at DATETIME, purge_operation_id TEXT)`,
		`CREATE TABLE notification.notification_preferences (
			id TEXT PRIMARY KEY, user_id TEXT, type TEXT, in_app_enabled BOOLEAN,
			deleted_at DATETIME, purge_operation_id TEXT)`,
		`CREATE TABLE notification.processed_events (event_id TEXT PRIMARY KEY, processed_at DATETIME)`,
	}
	for _, stmt := range ddl {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("ddl: %v", err)
		}
	}
	// user-1 is in BOTH fleets. That is the whole point of these fixtures: R4's
	// trap is a purge that keys on user_id and takes both streams.
	seed := []string{
		`INSERT INTO notification.notifications (id, user_id, type, title, dedupe_key, fleet_id)
		 VALUES ('n1', 'user-1', 'schedule.overdue', 'A', 'dk-1', 'fleet-1')`,
		`INSERT INTO notification.notifications (id, user_id, type, title, dedupe_key, fleet_id)
		 VALUES ('n2', 'user-1', 'schedule.overdue', 'B', 'dk-2', 'fleet-2')`,
		`INSERT INTO notification.notifications (id, user_id, type, title, dedupe_key, fleet_id)
		 VALUES ('n3', 'user-1', 'account.notice', 'C', 'dk-3', '')`,
		`INSERT INTO notification.notification_preferences (id, user_id, type, in_app_enabled)
		 VALUES ('p1', 'user-1', 'schedule.overdue', 1)`,
		`INSERT INTO notification.processed_events (event_id, processed_at) VALUES ('evt-1', CURRENT_TIMESTAMP)`,
	}
	for _, stmt := range seed {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	return db
}

func newNotificationRouter(t *testing.T, db *gorm.DB) chi.Router {
	t.Helper()
	r := chi.NewRouter()
	admin.InitializeInternalRoutes(logrus.New(), db)(r)
	return r
}

func post(t *testing.T, r chi.Router, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	return rec
}

// risks.md R4, dissolved structurally: a fleet purge keys on fleet_id and NEVER
// on user_id, so a user in two fleets keeps the other fleet's stream.
func TestPurge_fleetScope_neverKeysOnUser(t *testing.T) {
	db := newNotificationDB(t)
	r := newNotificationRouter(t, db)

	rec := post(t, r, "/internal/admin/purge",
		`{"operation_id":"op-1","scope":"fleet","fleet_ids":["fleet-1"]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Affected map[string]int `json:"affected"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Affected["notifications"] != 1 {
		t.Errorf("affected notifications = %d, want exactly 1", body.Affected["notifications"])
	}

	var otherFleet, accountLevel int64
	db.Raw(`SELECT count(*) FROM notification.notifications
	        WHERE id = 'n2' AND deleted_at IS NULL`).Scan(&otherFleet)
	db.Raw(`SELECT count(*) FROM notification.notifications
	        WHERE id = 'n3' AND deleted_at IS NULL`).Scan(&accountLevel)
	if otherFleet != 1 {
		t.Error("purging one fleet took the same user's OTHER fleet stream — this is R4")
	}
	if accountLevel != 1 {
		t.Error("purging a fleet took an account-level notification (empty fleet_id)")
	}
}

// design OQ-2: preferences have no fleet linkage, so they are out of fleet scope
// entirely — there is no correct predicate for them at that root.
func TestPurge_fleetScope_leavesPreferencesAlone(t *testing.T) {
	db := newNotificationDB(t)
	r := newNotificationRouter(t, db)
	post(t, r, "/internal/admin/purge", `{"operation_id":"op-1","scope":"fleet","fleet_ids":["fleet-1"]}`)

	var live int64
	db.Raw(`SELECT count(*) FROM notification.notification_preferences WHERE deleted_at IS NULL`).Scan(&live)
	if live != 1 {
		t.Errorf("a fleet purge must not touch preferences, %d of 1 live", live)
	}
}

// A system purge DOES take preferences, and takes account-level notifications
// (empty fleet_id) with them.
func TestPurge_systemScope_takesEverythingIncludingAccountLevel(t *testing.T) {
	db := newNotificationDB(t)
	r := newNotificationRouter(t, db)
	post(t, r, "/internal/admin/purge", `{"operation_id":"op-1","scope":"system"}`)

	for _, table := range []string{
		"notification.notifications", "notification.notification_preferences",
	} {
		var live int64
		db.Raw("SELECT count(*) FROM " + table + " WHERE deleted_at IS NULL").Scan(&live)
		if live != 0 {
			t.Errorf("%s has %d live rows after a system purge", table, live)
		}
	}
}

func TestRestoreAndReap(t *testing.T) {
	db := newNotificationDB(t)
	r := newNotificationRouter(t, db)
	post(t, r, "/internal/admin/purge", `{"operation_id":"op-1","scope":"fleet","fleet_ids":["fleet-1"]}`)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/internal/admin/purge/op-1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("restore status = %d: %s", rec.Code, rec.Body.String())
	}
	var live int64
	db.Raw(`SELECT count(*) FROM notification.notifications WHERE deleted_at IS NULL`).Scan(&live)
	if live != 3 {
		t.Errorf("restore returned %d of 3 notifications", live)
	}

	post(t, r, "/internal/admin/purge", `{"operation_id":"op-1","scope":"fleet","fleet_ids":["fleet-1"]}`)
	if rec := post(t, r, "/internal/admin/reap/op-1", ""); rec.Code != http.StatusOK {
		t.Fatalf("reap status = %d: %s", rec.Code, rec.Body.String())
	}
	var rows int64
	db.Raw(`SELECT count(*) FROM notification.notifications WHERE id = 'n1'`).Scan(&rows)
	if rows != 0 {
		t.Errorf("reap left the stamped notification behind")
	}
	// Idempotent.
	if rec := post(t, r, "/internal/admin/reap/op-1", ""); rec.Code != http.StatusOK {
		t.Errorf("a second reap must succeed, got %d", rec.Code)
	}
}

// design §3.3: truncating the idempotency ledger lets a Kafka replay regenerate
// notifications for data that was just purged — a system purge that undoes
// itself on the next consumer restart.
func TestSystemPurge_leavesProcessedEventsAlone(t *testing.T) {
	db := newNotificationDB(t)
	r := newNotificationRouter(t, db)
	post(t, r, "/internal/admin/purge", `{"operation_id":"op-1","scope":"system"}`)
	post(t, r, "/internal/admin/reap/op-1", "")

	var rows int64
	db.Raw(`SELECT count(*) FROM notification.processed_events`).Scan(&rows)
	if rows != 1 {
		t.Errorf("processed_events must survive a system purge, got %d rows", rows)
	}
}

func TestStats_countsLiveNotifications(t *testing.T) {
	db := newNotificationDB(t)
	r := newNotificationRouter(t, db)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/internal/admin/stats", nil))
	var body struct {
		Notifications int `json:"notifications"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Notifications != 3 {
		t.Errorf("notifications = %d, want 3", body.Notifications)
	}
}
```

- [ ] **Step 2: Run and watch it fail**

Run: `go test ./apps/notification-service/internal/admin/ -count=1 -v`
Expected: FAIL — the package does not exist.

- [ ] **Step 3: Write the manifest**

Create `apps/notification-service/internal/admin/manifest.go`:

```go
// Package admin is notification-service's slice of the platform purge protocol.
//
// A fleet purge is WHERE fleet_id = ? and keys on user_id NOWHERE, at any scope.
// That is what dissolves risks.md R4 — "a user in two fleets loses both
// streams" — structurally rather than by care: there is no predicate in this
// file that could take another fleet's notifications.
package admin

// Scope is what a downstream purge is rooted at. There is no media_ids
// equivalent here: notifications are reachable from a fleet id alone.
type Scope string

const (
	ScopeSystem Scope = "system"
	ScopeFleet  Scope = "fleet"
)

// Root is the resolved purge root for this service.
type Root struct {
	Scope    Scope
	FleetIDs []string
}

// Target is one purgeable table and how to resolve its rows.
type Target struct {
	Key   string
	Table string
	Where func(Root) (string, []any)
}

const all = "1 = 1"

// Manifest is notification-service's purge surface.
var Manifest = []Target{
	{
		Key: "notifications", Table: "notification.notifications",
		Where: func(r Root) (string, []any) {
			switch r.Scope {
			case ScopeSystem:
				return all, nil
			case ScopeFleet:
				// Rows with an empty fleet_id are account-level (the builder
				// makes fleet id optional). They survive a fleet purge and are
				// taken only by a system purge.
				return "fleet_id IN ?", []any{r.FleetIDs}
			}
			return "", nil
		},
	},
	{
		Key: "notification_preferences", Table: "notification.notification_preferences",
		Where: func(r Root) (string, []any) {
			// System scope only. The table is keyed (user_id, type) and carries
			// no fleet linkage at all, so there is no correct fleet predicate
			// (design OQ-2). Excluding it is safe: preferences regenerate with
			// defaults on the next read, so nothing a fleet purge should reach
			// lives here.
			if r.Scope == ScopeSystem {
				return all, nil
			}
			return "", nil
		},
	},
}

// excludedTables documents the deliberate omissions.
var excludedTables = map[string]string{
	// A finding, not bookkeeping: the PRD's "all of notification.*", taken
	// literally, truncates the idempotency ledger and lets a Kafka replay
	// regenerate notifications for data that was just purged.
	"notification.processed_events": "idempotency ledger; deleting it lets a Kafka replay resurrect purged notifications",
}
```

- [ ] **Step 4: Write operations, routes, and the arch test**

`operations.go` is Task 13's file verbatim, minus `ObjectKey` /
`ReapableObjectKeys` and with this package's `Manifest`. `resource.go` is Task
13's minus the MinIO block — the reap handler is just:

```go
		r.Post("/internal/admin/reap/{opId}", func(w http.ResponseWriter, req *http.Request) {
			opID := chi.URLParam(req, "opId")
			var deleted map[string]int
			if terr := db.Transaction(func(tx *gorm.DB) error {
				var rerr error
				deleted, rerr = Reap(tx, opID)
				return rerr
			}); terr != nil {
				log.WithError(terr).WithField("operation_id", opID).Error("notification admin reap")
				server.WriteError(w, terr)
				return
			}
			server.WriteJSON(w, http.StatusOK, affectedResponse{Affected: deleted})
		})
```

and `rootFrom` drops the `media_ids` case. `statsResponse` becomes
`struct{ Notifications int \`json:"notifications"\` }`, counting
`notification.notifications WHERE deleted_at IS NULL`.

`arch_test.go` is fleet-service's `TestManifestCoversEveryTable` with the package
clause changed, walking `..`.

The route registration in `apps/notification-service/cmd/main.go` goes **outside**
the JWT group:

```go
	if err := server.New(log).
		Use(telemetry.CorrelationID).
		// Internal routes: no JWT, network-restricted (consumed by
		// fleet-service's admin console).
		//
		// SECURITY: notifications-stripprefix strips the FULL /api/notifications
		// prefix, so a public request to /api/notifications/internal/admin/purge
		// arrives here as /internal/admin/purge. These are the FIRST internal
		// routes this service has ever had and they DELETE DATA. The
		// priority-200 internal-deny rule in the main overlay's ingressroute is
		// what keeps them off the public internet; the two ship together and
		// never separately (design F2).
		AddRouteInitializer(admin.InitializeInternalRoutes(log, db)).
		AddRouteInitializer(func(r chi.Router) {
			r.Group(func(pr chi.Router) {
```

- [ ] **Step 5: Ship the deny rule in the same commit**

`deploy/k8s/overlays/main/ingressroute.yaml`, after the auth-service rule from
Task 12:

```yaml
    # notification-service's FIRST internal routes arrive with this task, and
    # they are destructive: POST /internal/admin/purge, DELETE
    # /internal/admin/purge/{opId}, POST /internal/admin/reap/{opId}
    # (apps/notification-service/internal/admin/resource.go), all registered
    # WITHOUT JWT.
    #
    # notifications-stripprefix strips the FULL /api/notifications prefix, so
    # without this rule a public request to
    # /api/notifications/internal/admin/purge arrives at the service as
    # /internal/admin/purge — an unauthenticated, internet-reachable
    # "delete everything for this fleet" endpoint.
    #
    # PathRegexp with `[^/]*/*` for the same reason as the rules above: Traefik
    # normalises the path before matching and stripPrefix removes a literal
    # string rather than a path segment.
    #
    # The service reference is inert: internal-deny short-circuits with 403
    # before anything is proxied.
    - match: (Host(`myfleet.tumidanski.com`) || Host(`myfleet.tumidanski.me`) || Host(`myfleet.home`)) && PathRegexp(`(?i)^/+api/+notifications[^/]*/*internal`)
      kind: Rule
      priority: 200
      middlewares:
        - name: internal-deny
      services:
        - name: notification-service
          port: 8080
```

- [ ] **Step 6: Verify the tests and the rule**

Run:
```sh
go test ./apps/notification-service/... -count=1
kustomize build deploy/k8s/overlays/main | grep -c 'api/+notifications\[\^/\]\*/\*internal'
kustomize build deploy/k8s/overlays/main | kubectl apply --dry-run=server -f -
```
Expected: tests PASS; grep returns `2`; the dry-run succeeds.

- [ ] **Step 7: Commit**

```bash
git add apps/notification-service deploy/k8s
git commit -m "feat(notification): add the internal admin purge contract with its internal-deny rule"
```

---

### Task 15: the `adminclient` package

**Files:**
- Create: `apps/fleet-service/internal/adminclient/http.go`
- Create: `apps/fleet-service/internal/adminclient/auth.go`
- Create: `apps/fleet-service/internal/adminclient/media.go`
- Create: `apps/fleet-service/internal/adminclient/notification.go`
- Create: `apps/fleet-service/internal/adminclient/client_test.go`

**Interfaces:**
- Produces:
  - `type AuthClient struct{…}`, `NewAuthClient(base string) *AuthClient`
    - `Stats(ctx) (users int, err error)`
    - `Users(ctx, ids []string) (map[string]User, error)` where
      `type User struct { ID, Email, DisplayName string; CreatedAt time.Time; LastLoginAt *time.Time }`
    - `ListUsers(ctx, page server.Page) ([]User, int, error)` — the paginated
      directory behind `/admin/users` (FR-ADMIN-FLEET-6)
    - `IsPlatformAdmin(ctx, userID string) (bool, error)`
  - `type MediaClient struct{…}`, `NewMediaClient(base string) *MediaClient`
    - `Stats(ctx) (mediaObjects int, err error)`
    - `Purge(ctx, req PurgeRequest) (map[string]int, error)`
    - `Restore(ctx, opID string) (map[string]int, error)`
    - `Reap(ctx, opID string) (map[string]int, error)`
  - `type NotificationClient struct{…}`, same four methods minus `MediaIDs`
  - `type PurgeRequest struct { OperationID, Scope string; FleetIDs, MediaIDs []string }`
  - `const MaxLookupIDs = 50`

Modelled on `mediaclient` (`apps/fleet-service/internal/mediaclient/client.go`):
explicit 5-second timeout, never `http.DefaultClient`, context-aware, non-200
becomes an error.

- [ ] **Step 1: Write the failing client test**

Create `apps/fleet-service/internal/adminclient/client_test.go`:

```go
package adminclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAuthClient_UsersChunksLargeIDSets(t *testing.T) {
	var batches [][]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ids := strings.Split(r.URL.Query().Get("ids"), ",")
		batches = append(batches, ids)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"users":[]}`))
	}))
	defer srv.Close()

	ids := make([]string, MaxLookupIDs+5)
	for i := range ids {
		ids[i] = "u" + string(rune('a'+i%26))
	}
	if _, err := NewAuthClient(srv.URL).Users(context.Background(), ids); err != nil {
		t.Fatalf("users: %v", err)
	}
	if len(batches) != 2 {
		t.Fatalf("want 2 chunked requests, got %d", len(batches))
	}
	if len(batches[0]) != MaxLookupIDs {
		t.Errorf("first chunk = %d ids, want %d", len(batches[0]), MaxLookupIDs)
	}
}

// FR-ADMIN-AUTH-7 depends on this distinction: 404 means "not an admin", any
// other failure means "we could not tell" and must NOT read as false.
func TestAuthClient_IsPlatformAdmin(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		want    bool
		wantErr bool
	}{
		{"granted", http.StatusOK, true, false},
		{"revoked", http.StatusNotFound, false, false},
		{"service error", http.StatusInternalServerError, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
			}))
			defer srv.Close()
			got, err := NewAuthClient(srv.URL).IsPlatformAdmin(context.Background(), "u1")
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("IsPlatformAdmin = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMediaClient_PurgeReturnsAffectedCounts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/internal/admin/purge" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"affected":{"media_objects":12,"media_variants":24}}`))
	}))
	defer srv.Close()

	got, err := NewMediaClient(srv.URL).Purge(context.Background(), PurgeRequest{
		OperationID: "op-1", Scope: "fleet", FleetIDs: []string{"f1"},
	})
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if got["media_objects"] != 12 || got["media_variants"] != 24 {
		t.Errorf("affected = %v", got)
	}
}

func TestMediaClient_PurgePropagatesANon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	if _, err := NewMediaClient(srv.URL).Purge(context.Background(), PurgeRequest{
		OperationID: "op-1", Scope: "system",
	}); err == nil {
		t.Error("a 503 must surface as an error so the operation is marked partial")
	}
}
```

- [ ] **Step 2: Run and watch it fail**

Run: `go test ./apps/fleet-service/internal/adminclient/ -count=1 -v`
Expected: FAIL — the package does not exist.

- [ ] **Step 3: Write the shared transport**

Create `apps/fleet-service/internal/adminclient/http.go`:

```go
// Package adminclient holds fleet-service's HTTP clients for the other three
// services' internal admin routes.
//
// Modelled on internal/mediaclient: explicit timeout, never http.DefaultClient,
// context-aware, non-200 becomes an error. Cross-service data is fetched over
// the API, never via a cross-service DB read (design D6).
package adminclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// clientTimeout bounds every call. Two of these run on a user-facing request
// path (the purge create and the stats fan-out), so they cannot inherit
// http.DefaultClient's no-timeout behaviour: a stalled connection would hang the
// handler goroutine indefinitely, because the request context only cancels if
// the browser disconnects.
const clientTimeout = 5 * time.Second

// MaxLookupIDs bounds a single id-list query parameter, matching the ceiling
// auth-service and media-service enforce. Callers chunk larger sets.
const MaxLookupIDs = 50

// PurgeRequest is the one body shape both downstream purge endpoints accept.
// MediaIDs is media-service only.
type PurgeRequest struct {
	OperationID string   `json:"operation_id"`
	Scope       string   `json:"scope"`
	FleetIDs    []string `json:"fleet_ids,omitempty"`
	MediaIDs    []string `json:"media_ids,omitempty"`
}

// affectedResponse is the shape both services return from purge/restore/reap.
type affectedResponse struct {
	Affected map[string]int `json:"affected"`
}

type transport struct {
	base string
	hc   *http.Client
}

func newTransport(base string) transport {
	return transport{base: base, hc: &http.Client{Timeout: clientTimeout}}
}

func (t transport) do(ctx context.Context, method, path string, body, dst any) (int, error) {
	var rdr *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		rdr = bytes.NewReader(raw)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, t.base+path, rdr)
	if err != nil {
		return 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := t.hc.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode == http.StatusOK && dst != nil {
		if derr := json.NewDecoder(res.Body).Decode(dst); derr != nil {
			return res.StatusCode, fmt.Errorf("%s %s: decode: %w", method, path, derr)
		}
	}
	return res.StatusCode, nil
}

// expectOK turns any non-200 into an error, so a caller cannot mistake a 503 for
// an empty result.
func (t transport) expectOK(ctx context.Context, method, path string, body, dst any) error {
	status, err := t.do(ctx, method, path, body, dst)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("%s %s%s: status %d", method, t.base, path, status)
	}
	return nil
}

// chunk splits ids into batches of at most MaxLookupIDs.
func chunk(ids []string, size int) [][]string {
	var out [][]string
	for len(ids) > size {
		out = append(out, ids[:size])
		ids = ids[size:]
	}
	if len(ids) > 0 {
		out = append(out, ids)
	}
	return out
}
```

- [ ] **Step 4: Write the three clients**

`auth.go`:

```go
package adminclient

import (
	"context"
	"net/http"
	"net/url"
	"strings"
)

// User is one resolved user from auth-service's internal lookup.
type User struct {
	ID          string     `json:"id"`
	Email       string     `json:"email"`
	DisplayName string     `json:"display_name"`
	CreatedAt   time.Time  `json:"created_at"`
	LastLoginAt *time.Time `json:"last_login_at"`
}

// AuthClient calls auth-service's internal admin routes. base comes from
// AUTH_INTERNAL_URL.
type AuthClient struct{ t transport }

// NewAuthClient returns a client targeting the given auth-service base URL.
func NewAuthClient(base string) *AuthClient { return &AuthClient{t: newTransport(base)} }

// Stats returns the total user count.
func (c *AuthClient) Stats(ctx context.Context) (int, error) {
	var body struct {
		Users int `json:"users"`
	}
	if err := c.t.expectOK(ctx, http.MethodGet, "/internal/admin/stats", nil, &body); err != nil {
		return 0, err
	}
	return body.Users, nil
}

// Users resolves ids to email and display name, chunking the request so no
// single call exceeds the endpoint's bound. Ids that do not resolve are simply
// absent from the map; the caller decides whether that is a warning.
func (c *AuthClient) Users(ctx context.Context, ids []string) (map[string]User, error) {
	out := make(map[string]User, len(ids))
	for _, batch := range chunk(ids, MaxLookupIDs) {
		q := url.Values{}
		q.Set("ids", strings.Join(batch, ","))
		var body struct {
			Users []User `json:"users"`
			Total int    `json:"total"`
		}
		if err := c.t.expectOK(ctx, http.MethodGet, "/internal/admin/users?"+q.Encode(), nil, &body); err != nil {
			return nil, err
		}
		for _, u := range body.Users {
			out[u.ID] = u
		}
	}
	return out, nil
}

// ListUsers returns one page of the user directory plus the total
// (FR-ADMIN-FLEET-6).
//
// It is a distinct method from Users rather than an overload: the two differ in
// failure semantics. An unresolved id in Users is normal and produces a warning;
// a failure here means the directory page cannot be rendered at all.
func (c *AuthClient) ListUsers(ctx context.Context, page server.Page) ([]User, int, error) {
	q := url.Values{}
	q.Set("page[number]", strconv.Itoa(page.Number))
	q.Set("page[size]", strconv.Itoa(page.Size))
	var body struct {
		Users []User `json:"users"`
		Total int    `json:"total"`
	}
	if err := c.t.expectOK(ctx, http.MethodGet, "/internal/admin/users?"+q.Encode(), nil, &body); err != nil {
		return nil, 0, err
	}
	return body.Users, body.Total, nil
}

// IsPlatformAdmin re-verifies the privilege against auth.platform_admins
// (FR-ADMIN-AUTH-7).
//
// 404 means "not an admin" and is NOT an error. Anything else is: the caller
// must be able to tell "revoked" from "we could not reach auth-service", because
// the first is a 403 and the second is a 500 that stamps nothing.
func (c *AuthClient) IsPlatformAdmin(ctx context.Context, userID string) (bool, error) {
	status, err := c.t.do(ctx, http.MethodGet, "/internal/admin/platform-admins/"+url.PathEscape(userID), nil, nil)
	if err != nil {
		return false, err
	}
	switch status {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("GET platform-admins/%s: status %d", userID, status)
	}
}
```

That file imports `context`, `fmt`, `net/http`, `net/url`, `strconv`, `strings`,
`time`, and `packages/shared-go/server`.

`media.go`:

```go
package adminclient

import (
	"context"
	"net/http"
	"net/url"
)

// MediaClient calls media-service's internal admin routes. base comes from
// MEDIA_INTERNAL_URL.
type MediaClient struct{ t transport }

// NewMediaClient returns a client targeting the given media-service base URL.
func NewMediaClient(base string) *MediaClient { return &MediaClient{t: newTransport(base)} }

// Stats returns the live media-object count.
func (c *MediaClient) Stats(ctx context.Context) (int, error) {
	var body struct {
		MediaObjects int `json:"media_objects"`
	}
	if err := c.t.expectOK(ctx, http.MethodGet, "/internal/admin/stats", nil, &body); err != nil {
		return 0, err
	}
	return body.MediaObjects, nil
}

// Purge stamps media-service's rows for the operation. Idempotent: a replay
// returns the same counts (FR-ADMIN-PURGE-10), which is what makes the retry
// endpoint safe to press repeatedly.
func (c *MediaClient) Purge(ctx context.Context, req PurgeRequest) (map[string]int, error) {
	var body affectedResponse
	if err := c.t.expectOK(ctx, http.MethodPost, "/internal/admin/purge", req, &body); err != nil {
		return nil, err
	}
	return body.Affected, nil
}

// Restore clears media-service's stamp for the operation.
func (c *MediaClient) Restore(ctx context.Context, opID string) (map[string]int, error) {
	var body affectedResponse
	if err := c.t.expectOK(ctx, http.MethodDelete,
		"/internal/admin/purge/"+url.PathEscape(opID), nil, &body); err != nil {
		return nil, err
	}
	return body.Affected, nil
}

// Reap hard-deletes media-service's rows for the operation and removes the
// backing MinIO objects.
func (c *MediaClient) Reap(ctx context.Context, opID string) (map[string]int, error) {
	var body affectedResponse
	if err := c.t.expectOK(ctx, http.MethodPost,
		"/internal/admin/reap/"+url.PathEscape(opID), nil, &body); err != nil {
		return nil, err
	}
	return body.Affected, nil
}
```

`notification.go` is `media.go` with `MediaClient`→`NotificationClient`,
`NewMediaClient`→`NewNotificationClient`, and `Stats` decoding
`{"notifications": N}`. It sends the same `PurgeRequest`; `MediaIDs` is simply
never populated for it.

- [ ] **Step 5: Run to green**

Run: `go test ./apps/fleet-service/internal/adminclient/ -count=1 -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add apps/fleet-service/internal/adminclient
git commit -m "feat(fleet): add internal admin HTTP clients for auth, media and notification"
```

---

# Phase 4 — Purge lifecycle and the admin API (Tasks 16–21)

---

### Task 16: purge operations, audit events, confirmation, recovery window

**Files:**
- Create: `apps/fleet-service/internal/admin/confirmation.go`
- Create: `apps/fleet-service/internal/admin/confirmation_test.go`
- Create: `apps/fleet-service/internal/admin/entity.go`
- Create: `apps/fleet-service/internal/admin/model.go`
- Create: `apps/fleet-service/internal/admin/builder.go`
- Create: `apps/fleet-service/internal/admin/provider.go`
- Create: `apps/fleet-service/internal/admin/administrator.go`
- Create: `apps/fleet-service/internal/admin/store_test.go`
- Modify: `apps/fleet-service/cmd/main.go` (register `admin.Migration`)

**Interfaces:**
- Produces:
  - `const SystemConfirmation = "PURGE EVERYTHING"`, `DefaultRecoveryWindow = 120 * time.Hour`
  - `func MatchConfirmation(scope Scope, targetLabel, supplied string) error`
  - `func RecoveryWindow(raw string) time.Duration`
  - `type Status string`; `StatusPending`, `StatusPartial`, `StatusCancelled`, `StatusReaped`
  - `type OperationEntity`, `type AuditEntity`, `func Migration(db) error`
  - `type Operation` (immutable model) with getters `ID() Scope() TargetType() TargetID() TargetLabel() Status() RequestedByUserID() RequestedByEmail() RequestedAt() PurgeAfter() ReapedAt() CancelledAt() AffectedCounts() FailedServices()`
  - `func NewOperationBuilder() *OperationBuilder` with `SetScope`, `SetTarget`, `SetTargetLabel`, `SetRequestedBy`, `SetPurgeAfter`, `Build() (Operation, error)`
  - `type Provider interface { GetOperation(id string) (Operation, error); ListOperations(status string, page server.Page) ([]Operation, int, error); ListDue(now time.Time) ([]Operation, error); ListAudit(f AuditFilter, page server.Page) ([]AuditEvent, int, error) }`
  - `type Administrator interface { InsertOperation(tx *gorm.DB, o Operation) error; SetStatus(tx *gorm.DB, id string, s Status, failed []string, at time.Time) error; SetAffected(tx *gorm.DB, id string, counts map[string]int) error; InsertAudit(tx *gorm.DB, a AuditEvent) error }`
  - `var ErrOperationNotFound`, `var ErrAlreadyReaped`

**Two shapes worth stating.** `target_id` is `*string` because Postgres rejects
`''` for a `uuid` and system-scope operations have no target. And
`admin_audit_events` carries **no** `deleted_at` and appears in `excludedTables`:
a system purge must not erase its own audit trail (FR-ADMIN-AUDIT-2).

- [ ] **Step 1: Write the failing confirmation and window tests**

Create `apps/fleet-service/internal/admin/confirmation_test.go`:

```go
package admin

import (
	"errors"
	"testing"
	"time"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// FR-ADMIN-PURGE-7: the disabled button in the UI is a courtesy; THIS is the
// control (risks.md R9). A mismatch is 409 with no writes.
func TestMatchConfirmation(t *testing.T) {
	cases := []struct {
		name     string
		scope    Scope
		label    string
		supplied string
		wantErr  bool
	}{
		{"record needs nothing", ScopeRecord, "", "", false},
		{"fleet exact name", ScopeFleet, "The Tumidanski Fleet", "The Tumidanski Fleet", false},
		{"fleet wrong name", ScopeFleet, "The Tumidanski Fleet", "the tumidanski fleet", true},
		{"fleet trailing space", ScopeFleet, "The Tumidanski Fleet", "The Tumidanski Fleet ", true},
		{"fleet empty", ScopeFleet, "The Tumidanski Fleet", "", true},
		{"system exact phrase", ScopeSystem, "", SystemConfirmation, false},
		{"system near miss", ScopeSystem, "", "purge everything", true},
		{"system fleet name", ScopeSystem, "The Tumidanski Fleet", "The Tumidanski Fleet", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := MatchConfirmation(tc.scope, tc.label, tc.supplied)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr && !errors.Is(err, server.ErrConflict) {
				t.Errorf("a mismatch must map to 409, got %v", err)
			}
		})
	}
}

// FR-ADMIN-PURGE-11. An unparseable value falls back rather than panicking,
// following the COOKIE_SECURE precedent: a typo in a ConfigMap must not stop
// the service booting.
func TestRecoveryWindow(t *testing.T) {
	cases := []struct {
		raw  string
		want time.Duration
	}{
		{"120h", 120 * time.Hour},
		{"1h", time.Hour},
		{"", DefaultRecoveryWindow},
		{"five days", DefaultRecoveryWindow},
		{"-3h", DefaultRecoveryWindow},
		{"0", DefaultRecoveryWindow},
	}
	for _, tc := range cases {
		if got := RecoveryWindow(tc.raw); got != tc.want {
			t.Errorf("RecoveryWindow(%q) = %v, want %v", tc.raw, got, tc.want)
		}
	}
	if DefaultRecoveryWindow != 5*24*time.Hour {
		t.Errorf("the default must be the 5 days the PRD specifies and the vehicle sweep already uses, got %v",
			DefaultRecoveryWindow)
	}
}
```

- [ ] **Step 2: Run and watch it fail**

Run: `go test ./apps/fleet-service/internal/admin/ -run 'Confirmation|RecoveryWindow' -v`
Expected: FAIL — undefined identifiers.

- [ ] **Step 3: Write `confirmation.go`**

```go
package admin

import (
	"time"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// SystemConfirmation is the exact literal a system purge requires. It is
// deliberately not the fleet name of anything, not derivable, and not
// case-insensitive.
const SystemConfirmation = "PURGE EVERYTHING"

// DefaultRecoveryWindow is the 5 days the PRD specifies, matching
// vehicle.recoveryWindow so the two recovery stories tell users the same thing.
const DefaultRecoveryWindow = 5 * 24 * time.Hour

// ErrConfirmationMismatch is the 409 a wrong confirmation phrase produces. It
// carries a detail so the console can say WHY without the client guessing.
var ErrConfirmationMismatch = server.Detailed(server.ErrConflict,
	"confirmation does not match the required phrase")

// MatchConfirmation is the server-side gate on destructive scopes
// (FR-ADMIN-PURGE-7).
//
// Comparison is exact — no trimming, no case folding. Both are tempting and both
// are wrong: the phrase exists to make the operator read what they are about to
// destroy, and a forgiving comparison makes it a formality. The disabled button
// in the console is a courtesy; this is the control (risks.md R9).
func MatchConfirmation(scope Scope, targetLabel, supplied string) error {
	switch scope {
	case ScopeRecord:
		// A single record is recoverable for five days and destroys nothing a
		// user cannot recreate; requiring a phrase here would train the operator
		// to type past the ones that matter.
		return nil
	case ScopeFleet:
		if supplied == targetLabel && targetLabel != "" {
			return nil
		}
	case ScopeSystem:
		if supplied == SystemConfirmation {
			return nil
		}
	}
	return ErrConfirmationMismatch
}

// RecoveryWindow parses ADMIN_PURGE_RECOVERY_WINDOW, falling back to the
// default on anything unparseable or non-positive.
//
// It does not panic. A typo in a ConfigMap must not stop the service booting —
// the same call the COOKIE_SECURE parse already makes in auth-service's
// composition root.
func RecoveryWindow(raw string) time.Duration {
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return DefaultRecoveryWindow
	}
	return d
}
```

- [ ] **Step 4: Write the persistence layer**

Write the store test first — `apps/fleet-service/internal/admin/store_test.go`:

```go
package admin_test

import (
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin/admintest"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

func newOperation(t *testing.T, scope admin.Scope, label string) admin.Operation {
	t.Helper()
	b := admin.NewOperationBuilder().
		SetScope(scope).
		SetTargetLabel(label).
		SetRequestedBy("admin-1", "admin@example.com").
		SetPurgeAfter(testNow.Add(admin.DefaultRecoveryWindow))
	if scope == admin.ScopeFleet {
		b = b.SetTarget("fleet", "fleet-1")
	}
	o, err := b.Build()
	if err != nil {
		t.Fatalf("build operation: %v", err)
	}
	return o
}

func TestOperationRoundTrip(t *testing.T) {
	db := admintest.NewDB(t)
	adm := admin.NewAdministrator(db)
	prov := admin.NewProvider(db)

	o := newOperation(t, admin.ScopeFleet, "Fleet fleet-1")
	if err := db.Transaction(func(tx *gorm.DB) error { return adm.InsertOperation(tx, o) }); err != nil {
		t.Fatalf("insert: %v", err)
	}

	got, err := prov.GetOperation(o.ID())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status() != admin.StatusPending {
		t.Errorf("a new operation must be pending, got %q", got.Status())
	}
	if got.TargetLabel() != "Fleet fleet-1" {
		t.Errorf("target label = %q", got.TargetLabel())
	}
	if got.RequestedByEmail() != "admin@example.com" {
		t.Errorf("requested_by_email = %q", got.RequestedByEmail())
	}
}

// A system-scope operation has no target at all. target_id must be NULL, not
// the empty string: Postgres rejects '' for a uuid column.
func TestSystemOperation_hasANullTarget(t *testing.T) {
	db := admintest.NewDB(t)
	adm := admin.NewAdministrator(db)
	o := newOperation(t, admin.ScopeSystem, "the entire platform")
	if err := db.Transaction(func(tx *gorm.DB) error { return adm.InsertOperation(tx, o) }); err != nil {
		t.Fatalf("insert: %v", err)
	}
	var targetID *string
	if err := db.Raw(`SELECT target_id FROM fleet.purge_operations WHERE id = ?`, o.ID()).
		Scan(&targetID).Error; err != nil {
		t.Fatalf("read target_id: %v", err)
	}
	if targetID != nil {
		t.Errorf("system-scope target_id must be NULL, got %q", *targetID)
	}
}

func TestSetStatusAndAffected(t *testing.T) {
	db := admintest.NewDB(t)
	adm := admin.NewAdministrator(db)
	prov := admin.NewProvider(db)
	o := newOperation(t, admin.ScopeFleet, "Fleet fleet-1")
	if err := db.Transaction(func(tx *gorm.DB) error { return adm.InsertOperation(tx, o) }); err != nil {
		t.Fatalf("insert: %v", err)
	}

	if err := adm.SetAffected(db, o.ID(), map[string]int{"vehicles": 4, "fuel_logs": 130}); err != nil {
		t.Fatalf("set affected: %v", err)
	}
	if err := adm.SetStatus(db, o.ID(), admin.StatusPartial, []string{"media"}, testNow); err != nil {
		t.Fatalf("set status: %v", err)
	}

	got, err := prov.GetOperation(o.ID())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status() != admin.StatusPartial {
		t.Errorf("status = %q, want partial", got.Status())
	}
	if got.AffectedCounts()["vehicles"] != 4 {
		t.Errorf("affected = %v", got.AffectedCounts())
	}
	if len(got.FailedServices()) != 1 || got.FailedServices()[0] != "media" {
		t.Errorf("failed services = %v", got.FailedServices())
	}
}

// FR-ADMIN-RESTORE-4: the reaper's candidate set.
func TestListDue_selectsOnlyPendingAndPartialPastTheWindow(t *testing.T) {
	db := admintest.NewDB(t)
	adm := admin.NewAdministrator(db)
	prov := admin.NewProvider(db)

	mk := func(id string, status admin.Status, purgeAfter time.Time) {
		t.Helper()
		o, err := admin.NewOperationBuilder().
			SetID(id).
			SetScope(admin.ScopeSystem).
			SetTargetLabel("the entire platform").
			SetRequestedBy("admin-1", "admin@example.com").
			SetPurgeAfter(purgeAfter).
			Build()
		if err != nil {
			t.Fatalf("build: %v", err)
		}
		if err := db.Transaction(func(tx *gorm.DB) error { return adm.InsertOperation(tx, o) }); err != nil {
			t.Fatalf("insert: %v", err)
		}
		if status != admin.StatusPending {
			if err := adm.SetStatus(db, id, status, nil, testNow); err != nil {
				t.Fatalf("set status: %v", err)
			}
		}
	}
	past, future := testNow.Add(-time.Hour), testNow.Add(time.Hour)
	mk("due-pending", admin.StatusPending, past)
	mk("due-partial", admin.StatusPartial, past)
	mk("not-yet", admin.StatusPending, future)
	mk("cancelled", admin.StatusCancelled, past)
	mk("reaped", admin.StatusReaped, past)

	due, err := prov.ListDue(testNow)
	if err != nil {
		t.Fatalf("list due: %v", err)
	}
	got := map[string]bool{}
	for _, o := range due {
		got[o.ID()] = true
	}
	if !got["due-pending"] || !got["due-partial"] {
		t.Errorf("pending and partial operations past purge_after must be due, got %v", got)
	}
	if got["not-yet"] || got["cancelled"] || got["reaped"] {
		t.Errorf("a not-yet / cancelled / reaped operation must not be due, got %v", got)
	}
}

func TestGetOperation_missingIsNotFound(t *testing.T) {
	db := admintest.NewDB(t)
	if _, err := admin.NewProvider(db).GetOperation("nope"); err != admin.ErrOperationNotFound {
		t.Errorf("want ErrOperationNotFound, got %v", err)
	}
	_ = server.ErrNotFound // the resource layer maps the sentinel; the store returns its own
}
```

Then the four files. `entity.go`:

```go
package admin

import (
	"time"

	"gorm.io/gorm"
)

// Status is a purge operation's lifecycle state. cancelled and reaped are
// terminal.
type Status string

const (
	StatusPending   Status = "pending"
	StatusPartial   Status = "partial"
	StatusCancelled Status = "cancelled"
	StatusReaped    Status = "reaped"
)

// OperationEntity maps to fleet.purge_operations (PRD §6.2).
//
// TargetID is *string, not string: Postgres rejects '' for a uuid column and a
// system-scope operation genuinely has no target.
//
// AffectedCounts and FailedServices are jsonb. They are captured at stamp time
// so the log stays readable after the rows are gone, which is the same reason
// TargetLabel is denormalised.
type OperationEntity struct {
	ID                string  `gorm:"type:uuid;primaryKey"`
	Scope             string  `gorm:"not null"`
	TargetType        *string
	TargetID          *string `gorm:"type:uuid"`
	TargetLabel       string
	Status            string    `gorm:"not null;index"`
	RequestedByUserID string    `gorm:"type:uuid;not null"`
	RequestedByEmail  string    `gorm:"not null"`
	RequestedAt       time.Time `gorm:"not null"`
	PurgeAfter        time.Time `gorm:"not null;index"`
	ReapedAt          *time.Time
	CancelledAt       *time.Time
	AffectedCounts    []byte `gorm:"type:jsonb"`
	FailedServices    []byte `gorm:"type:jsonb"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (OperationEntity) TableName() string { return "fleet.purge_operations" }

// AuditEntity maps to fleet.admin_audit_events (PRD §6.3).
//
// APPEND-ONLY, and deliberately without a deleted_at: there is no API to modify
// or delete these rows, and a system purge must not erase its own audit trail
// (FR-ADMIN-AUDIT-2). That is also why the table is in excludedTables.
type AuditEntity struct {
	ID               string `gorm:"type:uuid;primaryKey"`
	ActorUserID      string `gorm:"type:uuid;not null"`
	ActorEmail       string `gorm:"not null"`
	Action           string `gorm:"not null;index"`
	Scope            string
	TargetType       *string
	TargetID         *string `gorm:"type:uuid"`
	TargetLabel      string
	PurgeOperationID *string `gorm:"type:uuid;index"`
	AffectedCounts   []byte  `gorm:"type:jsonb"`
	CorrelationID    string
	CreatedAt        time.Time `gorm:"index"`
}

func (AuditEntity) TableName() string { return "fleet.admin_audit_events" }

// Audit action values (FR-ADMIN-AUDIT-1).
const (
	ActionPurgeCreated   = "purge.created"
	ActionPurgeCancelled = "purge.cancelled"
	ActionPurgeRetried   = "purge.retried"
	ActionPurgeReaped    = "purge.reaped"
)

// ActorSystem is the actor_user_id and actor_email the reaper writes, so the
// console can render "system" rather than attributing a scheduled deletion to
// the person who requested it days earlier (FR-ADMIN-UI-13).
const ActorSystem = "system"

// Migration creates both admin-owned tables.
func Migration(db *gorm.DB) error {
	return db.AutoMigrate(&OperationEntity{}, &AuditEntity{})
}
```

`model.go` — immutable models plus `Make`/`ToEntity`, following the repo's
convention (unexported fields, getters, JSON marshalled in `ToEntity`):

```go
package admin

import (
	"encoding/json"
	"errors"
	"time"
)

// ErrOperationNotFound is the package-local sentinel; the resource layer maps it
// to server.ErrNotFound.
var ErrOperationNotFound = errors.New("purge operation not found")

// ErrAlreadyReaped is returned when a cancel targets a reaped operation.
// Reaping is irreversible and the API says so rather than pretending to succeed
// (FR-ADMIN-RESTORE-2).
var ErrAlreadyReaped = errors.New("purge operation already reaped")

// Operation is the immutable representation of a purge operation.
type Operation struct {
	id                string
	scope             Scope
	targetType        string
	targetID          string
	targetLabel       string
	status            Status
	requestedByUserID string
	requestedByEmail  string
	requestedAt       time.Time
	purgeAfter        time.Time
	reapedAt          *time.Time
	cancelledAt       *time.Time
	affectedCounts    map[string]int
	failedServices    []string
}

func (o Operation) ID() string                  { return o.id }
func (o Operation) Scope() Scope                { return o.scope }
func (o Operation) TargetType() string          { return o.targetType }
func (o Operation) TargetID() string            { return o.targetID }
func (o Operation) TargetLabel() string         { return o.targetLabel }
func (o Operation) Status() Status              { return o.status }
func (o Operation) RequestedByUserID() string   { return o.requestedByUserID }
func (o Operation) RequestedByEmail() string    { return o.requestedByEmail }
func (o Operation) RequestedAt() time.Time      { return o.requestedAt }
func (o Operation) PurgeAfter() time.Time       { return o.purgeAfter }
func (o Operation) ReapedAt() *time.Time        { return o.reapedAt }
func (o Operation) CancelledAt() *time.Time     { return o.cancelledAt }
func (o Operation) AffectedCounts() map[string]int { return o.affectedCounts }
func (o Operation) FailedServices() []string    { return o.failedServices }

// Root returns the manifest root this operation purges.
func (o Operation) Root() Root {
	return Root{Scope: o.scope, TargetType: o.targetType, TargetID: o.targetID}
}

// AuditEvent is the immutable representation of one audit row.
type AuditEvent struct {
	ID               string
	ActorUserID      string
	ActorEmail       string
	Action           string
	Scope            string
	TargetType       string
	TargetID         string
	TargetLabel      string
	PurgeOperationID string
	AffectedCounts   map[string]int
	CorrelationID    string
	CreatedAt        time.Time
}

// AuditFilter narrows the audit list (FR-ADMIN-AUDIT-3). Empty strings mean
// "any".
type AuditFilter struct {
	Action string
	Actor  string
}

// MakeOperation converts an entity to a model, tolerating malformed jsonb: a
// row whose counts cannot be decoded still renders in the console with the rest
// of its fields, which is strictly better than a 500 over the whole list.
func MakeOperation(e OperationEntity) Operation {
	o := Operation{
		id:                e.ID,
		scope:             Scope(e.Scope),
		targetLabel:       e.TargetLabel,
		status:            Status(e.Status),
		requestedByUserID: e.RequestedByUserID,
		requestedByEmail:  e.RequestedByEmail,
		requestedAt:       e.RequestedAt,
		purgeAfter:        e.PurgeAfter,
		reapedAt:          e.ReapedAt,
		cancelledAt:       e.CancelledAt,
		affectedCounts:    map[string]int{},
		failedServices:    []string{},
	}
	if e.TargetType != nil {
		o.targetType = *e.TargetType
	}
	if e.TargetID != nil {
		o.targetID = *e.TargetID
	}
	if len(e.AffectedCounts) > 0 {
		_ = json.Unmarshal(e.AffectedCounts, &o.affectedCounts)
	}
	if len(e.FailedServices) > 0 {
		_ = json.Unmarshal(e.FailedServices, &o.failedServices)
	}
	return o
}

// ToEntity converts a model to an entity for persistence. Empty target fields
// become NULL rather than '' (Postgres rejects '' for uuid).
func (o Operation) ToEntity() OperationEntity {
	e := OperationEntity{
		ID:                o.id,
		Scope:             string(o.scope),
		TargetLabel:       o.targetLabel,
		Status:            string(o.status),
		RequestedByUserID: o.requestedByUserID,
		RequestedByEmail:  o.requestedByEmail,
		RequestedAt:       o.requestedAt,
		PurgeAfter:        o.purgeAfter,
		ReapedAt:          o.reapedAt,
		CancelledAt:       o.cancelledAt,
	}
	if o.targetType != "" {
		tt := o.targetType
		e.TargetType = &tt
	}
	if o.targetID != "" {
		ti := o.targetID
		e.TargetID = &ti
	}
	e.AffectedCounts, _ = json.Marshal(o.affectedCounts)
	e.FailedServices, _ = json.Marshal(o.failedServices)
	return e
}

// MakeAudit converts an audit entity to a model.
func MakeAudit(e AuditEntity) AuditEvent {
	a := AuditEvent{
		ID:             e.ID,
		ActorUserID:    e.ActorUserID,
		ActorEmail:     e.ActorEmail,
		Action:         e.Action,
		Scope:          e.Scope,
		TargetLabel:    e.TargetLabel,
		CorrelationID:  e.CorrelationID,
		CreatedAt:      e.CreatedAt,
		AffectedCounts: map[string]int{},
	}
	if e.TargetType != nil {
		a.TargetType = *e.TargetType
	}
	if e.TargetID != nil {
		a.TargetID = *e.TargetID
	}
	if e.PurgeOperationID != nil {
		a.PurgeOperationID = *e.PurgeOperationID
	}
	if len(e.AffectedCounts) > 0 {
		_ = json.Unmarshal(e.AffectedCounts, &a.AffectedCounts)
	}
	return a
}

// ToEntity converts an audit model to an entity for persistence.
func (a AuditEvent) ToEntity() AuditEntity {
	e := AuditEntity{
		ID:            a.ID,
		ActorUserID:   a.ActorUserID,
		ActorEmail:    a.ActorEmail,
		Action:        a.Action,
		Scope:         a.Scope,
		TargetLabel:   a.TargetLabel,
		CorrelationID: a.CorrelationID,
		CreatedAt:     a.CreatedAt,
	}
	if a.TargetType != "" {
		tt := a.TargetType
		e.TargetType = &tt
	}
	if a.TargetID != "" {
		ti := a.TargetID
		e.TargetID = &ti
	}
	if a.PurgeOperationID != "" {
		p := a.PurgeOperationID
		e.PurgeOperationID = &p
	}
	e.AffectedCounts, _ = json.Marshal(a.AffectedCounts)
	return e
}
```

`builder.go`:

```go
package admin

import (
	"time"

	"github.com/google/uuid"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// OperationBuilder constructs a valid Operation. Build returns an error because
// scope, requester and purge_after are invariants enforced at construction.
type OperationBuilder struct{ o Operation }

// NewOperationBuilder starts a pending operation with a fresh id and
// requested_at of now.
func NewOperationBuilder() *OperationBuilder {
	return &OperationBuilder{o: Operation{
		id:             uuid.NewString(),
		status:         StatusPending,
		requestedAt:    time.Now().UTC(),
		affectedCounts: map[string]int{},
		failedServices: []string{},
	}}
}

// SetID overrides the generated id. Tests use it; production does not.
func (b *OperationBuilder) SetID(id string) *OperationBuilder { b.o.id = id; return b }

func (b *OperationBuilder) SetScope(s Scope) *OperationBuilder { b.o.scope = s; return b }

// SetTarget records what a non-system purge is rooted at.
func (b *OperationBuilder) SetTarget(targetType, targetID string) *OperationBuilder {
	b.o.targetType = targetType
	b.o.targetID = targetID
	return b
}

// SetTargetLabel denormalises the target's name, captured at request time so the
// log stays readable after the target is gone.
func (b *OperationBuilder) SetTargetLabel(l string) *OperationBuilder {
	b.o.targetLabel = l
	return b
}

func (b *OperationBuilder) SetRequestedBy(userID, email string) *OperationBuilder {
	b.o.requestedByUserID = userID
	b.o.requestedByEmail = email
	return b
}

func (b *OperationBuilder) SetPurgeAfter(t time.Time) *OperationBuilder {
	b.o.purgeAfter = t
	return b
}

// Build validates invariants and returns the operation or a 422.
func (b *OperationBuilder) Build() (Operation, error) {
	if !ValidScopes[b.o.scope] {
		return Operation{}, server.Detailed(server.ErrValidation, "unsupported scope")
	}
	if b.o.scope == ScopeRecord && !ValidTargetTypes[b.o.targetType] {
		return Operation{}, server.Detailed(server.ErrValidation, "unsupported target_type")
	}
	if b.o.scope != ScopeSystem && b.o.targetID == "" {
		return Operation{}, server.Detailed(server.ErrValidation, "target_id is required for this scope")
	}
	if b.o.requestedByUserID == "" || b.o.requestedByEmail == "" {
		return Operation{}, server.Detailed(server.ErrValidation, "requester is required")
	}
	if b.o.purgeAfter.IsZero() {
		return Operation{}, server.Detailed(server.ErrValidation, "purge_after is required")
	}
	return b.o, nil
}
```

`provider.go` and `administrator.go` follow the repo's shape. Key points:

```go
// ListDue returns operations whose recovery window has elapsed and that have
// not reached a terminal state. cancelled and reaped are excluded: the first
// was undone, the second is done (FR-ADMIN-RESTORE-4).
func (p *dbProvider) ListDue(now time.Time) ([]Operation, error) {
	var es []OperationEntity
	if err := p.db.Where("status IN ? AND purge_after < ?",
		[]string{string(StatusPending), string(StatusPartial)}, now).
		Order("purge_after asc").Find(&es).Error; err != nil {
		return nil, err
	}
	out := make([]Operation, 0, len(es))
	for _, e := range es {
		out = append(out, MakeOperation(e))
	}
	return out, nil
}
```

```go
// SetStatus moves an operation to a new state, stamping the matching timestamp.
// failed replaces failed_services wholesale — a retry that succeeds must clear
// the list rather than accumulate history there; the audit log is where history
// lives.
func (a *dbAdministrator) SetStatus(tx *gorm.DB, id string, s Status, failed []string, at time.Time) error {
	if failed == nil {
		failed = []string{}
	}
	raw, err := json.Marshal(failed)
	if err != nil {
		return err
	}
	updates := map[string]any{"status": string(s), "failed_services": raw, "updated_at": at}
	switch s {
	case StatusCancelled:
		updates["cancelled_at"] = at
	case StatusReaped:
		updates["reaped_at"] = at
	}
	return tx.Model(&OperationEntity{}).Where("id = ?", id).Updates(updates).Error
}
```

`ListOperations` filters on `status` when non-empty, orders `requested_at desc`,
and returns `server.Page`-shaped totals. `ListAudit` orders `created_at desc` and
filters `action` / `actor_user_id`.

- [ ] **Step 5: Register the migration**

`apps/fleet-service/cmd/main.go`, appended to the `SetMigrations` list:

```go
		dashboard.Migration,
		admin.Migration,
	))
```

- [ ] **Step 6: Run to green**

Run: `go test ./apps/fleet-service/... -count=1`
Expected: PASS, including `TestManifestCoversEveryTable` — both new tables are
already in `excludedTables`.

- [ ] **Step 7: Commit**

```bash
git add apps/fleet-service
git commit -m "feat(fleet): add purge-operation and audit persistence, confirmation and recovery window"
```

---

### Task 17: create a purge operation

**Files:**
- Create: `apps/fleet-service/internal/admin/processor.go`
- Create: `apps/fleet-service/internal/admin/processor_test.go`

**Interfaces:**
- Consumes: `Manifest`/`Stamp`, `Provider`/`Administrator`, `adminclient`.
- Produces:
  ```go
  type Deps struct {
      DB            *gorm.DB
      Provider      Provider
      Administrator Administrator
      Auth          AuthVerifier
      Downstream    []Downstream
      Window        time.Duration
      Now           func() time.Time
  }
  type AuthVerifier interface { IsPlatformAdmin(ctx context.Context, userID string) (bool, error) }
  type Downstream interface {
      Name() string
      Purge(ctx context.Context, req adminclient.PurgeRequest) (map[string]int, error)
      Restore(ctx context.Context, opID string) (map[string]int, error)
      Reap(ctx context.Context, opID string) (map[string]int, error)
  }
  type TargetResolver interface { Resolve(root Root) (label string, mediaIDs []string, err error) }
  func NewProcessor(log logrus.FieldLogger, d Deps, targets TargetResolver) *Processor
  func (p *Processor) Create(ctx context.Context, in CreateInput) (Operation, error)
  type CreateInput struct { Scope Scope; TargetType, TargetID, Confirmation string; ActorUserID, ActorEmail, CorrelationID string }
  ```

**The order the steps must run in (design §8.2), and why each is where it is:**

1. `RequirePlatformAdmin` (the handler), then **re-verify against auth-service**.
   Fail **closed** — 500, nothing stamped. Coupling an *irreversible* write to a
   dependency's availability is the correct trade; coupling a *reversible* one is
   not, which is why cancel does no re-verification at all (design §5.4).
2. Validate scope and target type → 422.
3. Resolve the root; unknown id → 404. Capture `target_label` **now**, while the
   target still has a name.
4. Compare `confirmation` server-side → 409, no writes.
5. **One transaction** (FR-ADMIN-PURGE-8): insert the operation row, `Stamp` every
   manifest target, write the audit row. Any failure rolls back all three and the
   operation does not exist.
6. **After commit**, call the downstream stamps. Failures do **not** roll back the
   local stamp; they set `status = 'partial'` and populate `failed_services`, and
   the response is still 201.

- [ ] **Step 1: Write the failing processor test**

Create `apps/fleet-service/internal/admin/processor_test.go`:

```go
package admin_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin/admintest"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/adminclient"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

type stubAuth struct {
	admin bool
	err   error
}

func (s stubAuth) IsPlatformAdmin(context.Context, string) (bool, error) { return s.admin, s.err }

type stubDownstream struct {
	name      string
	purgeErr  error
	purgeCall int
	restored  int
	reaped    int
}

func (s *stubDownstream) Name() string { return s.name }
func (s *stubDownstream) Purge(context.Context, adminclient.PurgeRequest) (map[string]int, error) {
	s.purgeCall++
	if s.purgeErr != nil {
		return nil, s.purgeErr
	}
	return map[string]int{s.name + "_rows": 7}, nil
}
func (s *stubDownstream) Restore(context.Context, string) (map[string]int, error) {
	s.restored++
	return map[string]int{}, nil
}
func (s *stubDownstream) Reap(context.Context, string) (map[string]int, error) {
	s.reaped++
	return map[string]int{}, nil
}

// stubTargets resolves a label and the media ids a record purge must name.
type stubTargets struct {
	label    string
	mediaIDs []string
	err      error
}

func (s stubTargets) Resolve(admin.Root) (string, []string, error) {
	return s.label, s.mediaIDs, s.err
}

func newProcessor(t *testing.T, db *gorm.DB, auth admin.AuthVerifier, down ...admin.Downstream) *admin.Processor {
	t.Helper()
	return admin.NewProcessor(logrus.New(), admin.Deps{
		DB:            db,
		Provider:      admin.NewProvider(db),
		Administrator: admin.NewAdministrator(db),
		Auth:          auth,
		Downstream:    down,
		Window:        admin.DefaultRecoveryWindow,
		Now:           func() time.Time { return testNow },
	}, stubTargets{label: "Fleet fleet-1"})
}

func fleetInput() admin.CreateInput {
	return admin.CreateInput{
		Scope:         admin.ScopeFleet,
		TargetType:    "fleet",
		TargetID:      "fleet-1",
		Confirmation:  "Fleet fleet-1",
		ActorUserID:   "admin-1",
		ActorEmail:    "admin@example.com",
		CorrelationID: "corr-1",
	}
}

func TestCreate_stampsLocallyAndRecordsTheOperation(t *testing.T) {
	db := admintest.NewDB(t)
	admintest.SeedFleet(t, db, "fleet-1")
	media := &stubDownstream{name: "media"}
	notif := &stubDownstream{name: "notification"}
	proc := newProcessor(t, db, stubAuth{admin: true}, media, notif)

	op, err := proc.Create(context.Background(), fleetInput())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if op.Status() != admin.StatusPending {
		t.Errorf("status = %q, want pending", op.Status())
	}
	if !op.PurgeAfter().Equal(testNow.Add(admin.DefaultRecoveryWindow)) {
		t.Errorf("purge_after = %v, want now + 120h", op.PurgeAfter())
	}
	if op.AffectedCounts()["vehicles"] != 2 {
		t.Errorf("affected counts must include the local stamp: %v", op.AffectedCounts())
	}
	if op.AffectedCounts()["media_rows"] != 7 || op.AffectedCounts()["notification_rows"] != 7 {
		t.Errorf("affected counts must merge downstream results: %v", op.AffectedCounts())
	}
	if got := admintest.CountLive(t, db, "fleet.vehicles"); got != 0 {
		t.Errorf("the local stamp did not run: %d live vehicles", got)
	}

	var audits int64
	db.Raw(`SELECT count(*) FROM fleet.admin_audit_events WHERE action = ?`, admin.ActionPurgeCreated).
		Scan(&audits)
	if audits != 1 {
		t.Errorf("want one purge.created audit row, got %d", audits)
	}
}

// FR-ADMIN-PURGE-9: a downstream failure leaves the LOCAL stamp in place, marks
// the operation partial, and names the service. It does NOT roll back.
func TestCreate_downstreamFailureIsPartialNotRollback(t *testing.T) {
	db := admintest.NewDB(t)
	admintest.SeedFleet(t, db, "fleet-1")
	media := &stubDownstream{name: "media", purgeErr: errors.New("connection refused")}
	notif := &stubDownstream{name: "notification"}
	proc := newProcessor(t, db, stubAuth{admin: true}, media, notif)

	op, err := proc.Create(context.Background(), fleetInput())
	if err != nil {
		t.Fatalf("a downstream failure must not fail the request: %v", err)
	}
	if op.Status() != admin.StatusPartial {
		t.Errorf("status = %q, want partial", op.Status())
	}
	if len(op.FailedServices()) != 1 || op.FailedServices()[0] != "media" {
		t.Errorf("failed services = %v, want [media]", op.FailedServices())
	}
	if got := admintest.CountLive(t, db, "fleet.vehicles"); got != 0 {
		t.Errorf("the local stamp must survive a downstream failure: %d live", got)
	}
	if notif.purgeCall != 1 {
		t.Errorf("one service failing must not skip the others, notification called %d times", notif.purgeCall)
	}
}

// FR-ADMIN-PURGE-7 / risks.md R9: a wrong confirmation writes NOTHING.
func TestCreate_confirmationMismatchWritesNothing(t *testing.T) {
	db := admintest.NewDB(t)
	admintest.SeedFleet(t, db, "fleet-1")
	media := &stubDownstream{name: "media"}
	proc := newProcessor(t, db, stubAuth{admin: true}, media)

	in := fleetInput()
	in.Confirmation = "fleet fleet-1"
	if _, err := proc.Create(context.Background(), in); !errors.Is(err, server.ErrConflict) {
		t.Fatalf("want 409, got %v", err)
	}
	if got := admintest.CountLive(t, db, "fleet.vehicles"); got != 2 {
		t.Errorf("a rejected confirmation stamped rows: %d of 2 live", got)
	}
	if got := admintest.CountRows(t, db, "fleet.purge_operations"); got != 0 {
		t.Errorf("a rejected confirmation created an operation row: %d rows", got)
	}
	if media.purgeCall != 0 {
		t.Errorf("a rejected confirmation called downstream %d times", media.purgeCall)
	}
}

// FR-ADMIN-AUTH-7: a revoked admin holding a valid token cannot destroy data.
func TestCreate_revokedAdminIsForbidden(t *testing.T) {
	db := admintest.NewDB(t)
	admintest.SeedFleet(t, db, "fleet-1")
	proc := newProcessor(t, db, stubAuth{admin: false})

	if _, err := proc.Create(context.Background(), fleetInput()); !errors.Is(err, server.ErrForbidden) {
		t.Fatalf("want 403, got %v", err)
	}
	if got := admintest.CountLive(t, db, "fleet.vehicles"); got != 2 {
		t.Errorf("a revoked admin stamped rows: %d of 2 live", got)
	}
}

// design §5.4: create fails CLOSED. Coupling an irreversible write to a
// dependency's availability is the correct trade.
func TestCreate_failsClosedWhenAuthServiceIsUnreachable(t *testing.T) {
	db := admintest.NewDB(t)
	admintest.SeedFleet(t, db, "fleet-1")
	proc := newProcessor(t, db, stubAuth{err: errors.New("connection refused")})

	if _, err := proc.Create(context.Background(), fleetInput()); err == nil {
		t.Fatal("an unreachable auth-service must fail the create, not proceed")
	}
	if got := admintest.CountLive(t, db, "fleet.vehicles"); got != 2 {
		t.Errorf("nothing may be stamped when re-verification could not run: %d of 2 live", got)
	}
}

func TestCreate_rejectsAnUnknownTargetType(t *testing.T) {
	db := admintest.NewDB(t)
	proc := newProcessor(t, db, stubAuth{admin: true})
	in := fleetInput()
	in.Scope = admin.ScopeRecord
	in.TargetType = "spaceship"
	in.TargetID = "x"
	if _, err := proc.Create(context.Background(), in); !errors.Is(err, server.ErrValidation) {
		t.Errorf("want 422, got %v", err)
	}
}

// The system purge's confirmation is the literal phrase, and its operation has
// no target at all.
func TestCreate_systemScope(t *testing.T) {
	db := admintest.NewDB(t)
	admintest.SeedFleet(t, db, "fleet-1")
	admintest.SeedFleet(t, db, "fleet-2")
	proc := newProcessor(t, db, stubAuth{admin: true}, &stubDownstream{name: "media"})

	op, err := proc.Create(context.Background(), admin.CreateInput{
		Scope:        admin.ScopeSystem,
		Confirmation: admin.SystemConfirmation,
		ActorUserID:  "admin-1",
		ActorEmail:   "admin@example.com",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if op.TargetID() != "" {
		t.Errorf("a system operation must have no target, got %q", op.TargetID())
	}
	if got := admintest.CountLive(t, db, "fleet.fleets"); got != 0 {
		t.Errorf("a system purge left %d live fleets", got)
	}
	// PRD blast radius: seeded reference data survives.
	if got := admintest.CountRows(t, db, "fleet.maintenance_categories"); got != 1 {
		t.Errorf("maintenance categories must survive a system purge, got %d", got)
	}
}
```

Add the `gorm.io/gorm` import.

- [ ] **Step 2: Run and watch it fail**

Run: `go test ./apps/fleet-service/internal/admin/ -run Create -count=1 -v`
Expected: FAIL — `undefined: admin.NewProcessor`.

- [ ] **Step 3: Write the processor**

Create `apps/fleet-service/internal/admin/processor.go`:

```go
package admin

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/adminclient"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// AuthVerifier re-checks the platform-admin privilege against the database
// behind auth-service. Declaring the port here rather than importing the
// concrete client keeps the processor testable.
type AuthVerifier interface {
	IsPlatformAdmin(ctx context.Context, userID string) (bool, error)
}

// Downstream is one other service's slice of the purge protocol. Name() is what
// lands in failed_services and, through it, in the console's
// "Media not deleted" wording.
type Downstream interface {
	Name() string
	Purge(ctx context.Context, req adminclient.PurgeRequest) (map[string]int, error)
	Restore(ctx context.Context, opID string) (map[string]int, error)
	Reap(ctx context.Context, opID string) (map[string]int, error)
}

// TargetResolver turns a purge root into the human label to denormalise and,
// for a record-scope vehicle purge, the media ids media-service must be told
// about — the only place in the whole design where an explicit id set crosses a
// service boundary (design OQ-1).
//
// It returns server.ErrNotFound for an unknown target.
type TargetResolver interface {
	Resolve(root Root) (label string, mediaIDs []string, err error)
}

// Deps bundles everything the lifecycle needs. Now is injected so tests can
// assert exact purge_after values.
type Deps struct {
	DB            *gorm.DB
	Provider      Provider
	Administrator Administrator
	Auth          AuthVerifier
	Downstream    []Downstream
	Window        time.Duration
	Now           func() time.Time
}

// Processor owns the purge lifecycle.
type Processor struct {
	log     logrus.FieldLogger
	d       Deps
	targets TargetResolver
}

// NewProcessor constructs the lifecycle processor.
func NewProcessor(log logrus.FieldLogger, d Deps, targets TargetResolver) *Processor {
	if d.Now == nil {
		d.Now = func() time.Time { return time.Now().UTC() }
	}
	if d.Window <= 0 {
		d.Window = DefaultRecoveryWindow
	}
	return &Processor{log: log, d: d, targets: targets}
}

// CreateInput is the validated request body plus the caller's identity.
type CreateInput struct {
	Scope         Scope
	TargetType    string
	TargetID      string
	Confirmation  string
	ActorUserID   string
	ActorEmail    string
	CorrelationID string
}

// Create runs the full purge-creation sequence (design §8.2).
func (p *Processor) Create(ctx context.Context, in CreateInput) (Operation, error) {
	now := p.d.Now()

	// 1. Re-verify against the database behind auth-service, FAIL CLOSED.
	//
	// The claim is stamped at mint time, so a revoked admin holds a valid token
	// for up to 15 minutes (FR-ADMIN-AUTH-7). Coupling this irreversible write
	// to auth-service's availability is the correct trade — the same reasoning
	// mediaclient.ValidateOwnership already applies. Cancel deliberately does
	// NOT do this: never block the recovery path (design §5.4).
	ok, err := p.d.Auth.IsPlatformAdmin(ctx, in.ActorUserID)
	if err != nil {
		p.log.WithError(err).WithField("actor", in.ActorUserID).
			Error("platform-admin re-verification failed; refusing the purge")
		return Operation{}, err
	}
	if !ok {
		return Operation{}, server.ErrForbidden
	}

	// 2. Validate the enums.
	if !ValidScopes[in.Scope] {
		return Operation{}, server.Detailed(server.ErrValidation, "unsupported scope")
	}
	if in.Scope == ScopeRecord && !ValidTargetTypes[in.TargetType] {
		return Operation{}, server.Detailed(server.ErrValidation, "unsupported target_type")
	}

	// 3. Resolve the root and capture the label WHILE THE TARGET STILL HAS ONE.
	root := Root{Scope: in.Scope, TargetType: in.TargetType, TargetID: in.TargetID}
	label, mediaIDs, err := p.targets.Resolve(root)
	if err != nil {
		return Operation{}, err
	}

	// 4. Confirmation, server-side. The disabled button is a courtesy.
	if err := MatchConfirmation(in.Scope, label, in.Confirmation); err != nil {
		return Operation{}, err
	}

	op, err := NewOperationBuilder().
		SetScope(in.Scope).
		SetTarget(targetTypeFor(in), in.TargetID).
		SetTargetLabel(label).
		SetRequestedBy(in.ActorUserID, in.ActorEmail).
		SetPurgeAfter(now.Add(p.d.Window)).
		Build()
	if err != nil {
		return Operation{}, err
	}

	// 5. ONE transaction: operation row + every local stamp + the audit row.
	// Any failure rolls back all three and the operation does not exist
	// (FR-ADMIN-PURGE-8).
	var affected map[string]int
	if err := p.d.DB.Transaction(func(tx *gorm.DB) error {
		if ierr := p.d.Administrator.InsertOperation(tx, op); ierr != nil {
			return ierr
		}
		var serr error
		affected, serr = Stamp(tx, root, op.ID(), now)
		if serr != nil {
			return serr
		}
		if uerr := p.d.Administrator.SetAffected(tx, op.ID(), affected); uerr != nil {
			return uerr
		}
		return p.d.Administrator.InsertAudit(tx, AuditEvent{
			ID:               uuid.NewString(),
			ActorUserID:      in.ActorUserID,
			ActorEmail:       in.ActorEmail,
			Action:           ActionPurgeCreated,
			Scope:            string(in.Scope),
			TargetType:       targetTypeFor(in),
			TargetID:         in.TargetID,
			TargetLabel:      label,
			PurgeOperationID: op.ID(),
			AffectedCounts:   affected,
			CorrelationID:    in.CorrelationID,
			CreatedAt:        now,
		})
	}); err != nil {
		p.log.WithError(err).WithField("correlation_id", in.CorrelationID).Error("local purge transaction")
		return Operation{}, err
	}

	// 6. AFTER commit, fan out. Failures mark the operation partial; they do not
	// roll back the local stamp (FR-ADMIN-PURGE-9), and the response is still
	// 201 with failed_services populated.
	downstreamCounts, failed := p.fanOutPurge(ctx, op, mediaIDs)
	for k, v := range downstreamCounts {
		affected[k] = v
	}
	status := StatusPending
	if len(failed) > 0 {
		status = StatusPartial
	}
	if err := p.d.Administrator.SetAffected(p.d.DB, op.ID(), affected); err != nil {
		p.log.WithError(err).WithField("operation_id", op.ID()).Error("record downstream counts")
	}
	if err := p.d.Administrator.SetStatus(p.d.DB, op.ID(), status, failed, now); err != nil {
		p.log.WithError(err).WithField("operation_id", op.ID()).Error("record operation status")
	}

	p.log.WithFields(logrus.Fields{
		"operation_id":    op.ID(),
		"scope":           in.Scope,
		"target_id":       in.TargetID,
		"actor":           in.ActorUserID,
		"correlation_id":  in.CorrelationID,
		"status":          status,
		"failed_services": failed,
		"affected":        affected,
	}).Info("admin purge created")

	return p.d.Provider.GetOperation(op.ID())
}

// fanOutPurge calls every downstream stamp, collecting counts and the names of
// the services that failed. One service failing never skips the others: a
// partial purge that reached two of three is strictly better than one that
// reached one, and every call is idempotent so the retry costs nothing.
func (p *Processor) fanOutPurge(ctx context.Context, op Operation, mediaIDs []string) (map[string]int, []string) {
	counts := map[string]int{}
	var failed []string
	req := downstreamRequest(op, mediaIDs)
	for _, d := range p.d.Downstream {
		got, err := d.Purge(ctx, req)
		if err != nil {
			p.log.WithError(err).WithFields(logrus.Fields{
				"operation_id": op.ID(), "service": d.Name(),
			}).Warn("downstream purge failed; operation is partial")
			failed = append(failed, d.Name())
			continue
		}
		for k, v := range got {
			counts[k] = v
		}
	}
	return counts, failed
}

// downstreamRequest translates a local root into the downstream body.
//
// A record-scope vehicle purge is the ONE case that carries explicit ids: media
// objects have a fleet_id, but "the media belonging to this vehicle" is a fact
// only fleet-service holds (design OQ-1). A cross-fleet leak is not reachable —
// media-service stamps fleet_id at upload and mediaclient.ValidateOwnership
// already refuses to attach another fleet's media — so every id fleet-service
// can produce for a vehicle is in that vehicle's fleet.
func downstreamRequest(op Operation, mediaIDs []string) adminclient.PurgeRequest {
	req := adminclient.PurgeRequest{OperationID: op.ID()}
	switch op.Scope() {
	case ScopeSystem:
		req.Scope = "system"
	case ScopeFleet:
		req.Scope = "fleet"
		req.FleetIDs = []string{op.TargetID()}
	case ScopeRecord:
		req.Scope = "media_ids"
		req.MediaIDs = mediaIDs
	}
	return req
}

// targetTypeFor normalises the stored target_type: a fleet-scope operation
// records "fleet" even when the caller omitted it, so the audit log reads the
// same way for every scope.
func targetTypeFor(in CreateInput) string {
	if in.Scope == ScopeFleet {
		return "fleet"
	}
	return in.TargetType
}
```

A record-scope purge whose `mediaIDs` is empty must **not** reach media-service
with an empty `media_ids` list — the endpoint rejects that as 422. Guard it in
`fanOutPurge`:

```go
	req := downstreamRequest(op, mediaIDs)
	for _, d := range p.d.Downstream {
		// A record purge with no media touches nothing downstream. Sending an
		// empty media_ids list would be a 422 that reads as a failed service.
		if req.Scope == "media_ids" && len(req.MediaIDs) == 0 {
			continue
		}
```

Note also that a `record`-scope purge sends nothing meaningful to
notification-service (notifications are not addressable by record), so
`NotificationClient` must be skipped for that scope. Add to the same loop:

```go
		if req.Scope == "media_ids" && d.Name() != "media" {
			continue
		}
```

- [ ] **Step 4: Run to green**

Run: `go test ./apps/fleet-service/internal/admin/ -count=1 -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/fleet-service/internal/admin
git commit -m "feat(fleet): create purge operations with local transaction and downstream fan-out"
```

---

### Task 18: cancel and retry

**Files:**
- Modify: `apps/fleet-service/internal/admin/processor.go`
- Modify: `apps/fleet-service/internal/admin/processor_test.go`

**Interfaces:**
- Produces:
  - `func (p *Processor) Cancel(ctx context.Context, opID string, actor Actor) (Operation, error)`
  - `func (p *Processor) Retry(ctx context.Context, opID string, actor Actor) (Operation, error)`
  - `type Actor struct { UserID, Email, CorrelationID string }`

**Cancel does not re-verify (design §5.4).** Applied literally, FR-ADMIN-AUTH-7
would include cancel — the recovery path. If auth-service is unreachable, a
fail-closed re-verification would block the one action that undoes a mistake,
during the window when undoing it is still possible. Retry **does** re-verify:
it re-attempts a destructive stamp.

**Cancel's status rule.** Downstream failure leaves the operation `partial` and
still cancellable; restore is idempotent, so the correct user action is to press
it again. Status becomes `cancelled` only when **every** service has restored.

- [ ] **Step 1: Write the failing tests**

Append to `processor_test.go`:

```go
func TestCancel_restoresEverywhereAndMarksCancelled(t *testing.T) {
	db := admintest.NewDB(t)
	admintest.SeedFleet(t, db, "fleet-1")
	media := &stubDownstream{name: "media"}
	proc := newProcessor(t, db, stubAuth{admin: true}, media)

	op, err := proc.Create(context.Background(), fleetInput())
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := proc.Cancel(context.Background(), op.ID(),
		admin.Actor{UserID: "admin-1", Email: "admin@example.com"})
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if got.Status() != admin.StatusCancelled {
		t.Errorf("status = %q, want cancelled", got.Status())
	}
	if got.CancelledAt() == nil {
		t.Error("cancelled_at must be stamped")
	}
	if media.restored != 1 {
		t.Errorf("media restore called %d times, want 1", media.restored)
	}
	for _, table := range []string{"fleet.fleets", "fleet.vehicles", "fleet.fleet_memberships"} {
		if admintest.CountLive(t, db, table) == 0 {
			t.Errorf("%s was not restored", table)
		}
	}

	var audits int64
	db.Raw(`SELECT count(*) FROM fleet.admin_audit_events WHERE action = ?`,
		admin.ActionPurgeCancelled).Scan(&audits)
	if audits != 1 {
		t.Errorf("want one purge.cancelled audit row, got %d", audits)
	}
}

// design §5.4: cancel must work even when auth-service is down. It is the
// recovery path; blocking it during the window when recovery is still possible
// is the worst available outcome.
func TestCancel_worksWhenAuthServiceIsUnreachable(t *testing.T) {
	db := admintest.NewDB(t)
	admintest.SeedFleet(t, db, "fleet-1")
	proc := newProcessor(t, db, stubAuth{admin: true}, &stubDownstream{name: "media"})
	op, err := proc.Create(context.Background(), fleetInput())
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Auth-service falls over between the create and the cancel.
	broken := newProcessor(t, db, stubAuth{err: errors.New("connection refused")},
		&stubDownstream{name: "media"})
	if _, cerr := broken.Cancel(context.Background(), op.ID(),
		admin.Actor{UserID: "admin-1", Email: "admin@example.com"}); cerr != nil {
		t.Fatalf("cancel must not depend on auth-service: %v", cerr)
	}
	if admintest.CountLive(t, db, "fleet.vehicles") != 2 {
		t.Error("cancel did not restore the vehicles")
	}
}

// A downstream restore that fails leaves the operation PARTIAL and still
// cancellable — restore is idempotent, so pressing it again is the fix.
func TestCancel_downstreamFailureStaysCancellable(t *testing.T) {
	db := admintest.NewDB(t)
	admintest.SeedFleet(t, db, "fleet-1")
	media := &failingRestore{stubDownstream: stubDownstream{name: "media"}}
	proc := newProcessor(t, db, stubAuth{admin: true}, media)
	op, _ := proc.Create(context.Background(), fleetInput())

	got, err := proc.Cancel(context.Background(), op.ID(), admin.Actor{UserID: "a", Email: "a@x"})
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if got.Status() != admin.StatusPartial {
		t.Errorf("status = %q, want partial while a service has not restored", got.Status())
	}
	// Local rows come back regardless: a downstream failure must not hold the
	// product hostage.
	if admintest.CountLive(t, db, "fleet.vehicles") != 2 {
		t.Error("local restore must run even when a downstream restore fails")
	}
	// And a second cancel completes it.
	media.failRestore = false
	got, err = proc.Cancel(context.Background(), op.ID(), admin.Actor{UserID: "a", Email: "a@x"})
	if err != nil {
		t.Fatalf("second cancel: %v", err)
	}
	if got.Status() != admin.StatusCancelled {
		t.Errorf("a repeated cancel must complete the operation, got %q", got.Status())
	}
}

// FR-ADMIN-RESTORE-2: reaping is irreversible and the API says so.
func TestCancel_onAReapedOperationIs409(t *testing.T) {
	db := admintest.NewDB(t)
	admintest.SeedFleet(t, db, "fleet-1")
	proc := newProcessor(t, db, stubAuth{admin: true}, &stubDownstream{name: "media"})
	op, _ := proc.Create(context.Background(), fleetInput())
	if err := admin.NewAdministrator(db).SetStatus(db, op.ID(), admin.StatusReaped, nil, testNow); err != nil {
		t.Fatalf("mark reaped: %v", err)
	}

	if _, err := proc.Cancel(context.Background(), op.ID(),
		admin.Actor{UserID: "a", Email: "a@x"}); !errors.Is(err, server.ErrConflict) {
		t.Errorf("want 409, got %v", err)
	}
}

// FR-ADMIN-PURGE-9: retry re-attempts the failed downstream stamps without
// double-stamping, and clears the failure once they succeed.
func TestRetry_completesAPartialOperation(t *testing.T) {
	db := admintest.NewDB(t)
	admintest.SeedFleet(t, db, "fleet-1")
	media := &stubDownstream{name: "media", purgeErr: errors.New("connection refused")}
	proc := newProcessor(t, db, stubAuth{admin: true}, media)

	op, err := proc.Create(context.Background(), fleetInput())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if op.Status() != admin.StatusPartial {
		t.Fatalf("fixture expected a partial operation, got %q", op.Status())
	}

	media.purgeErr = nil
	got, err := proc.Retry(context.Background(), op.ID(), admin.Actor{UserID: "a", Email: "a@x"})
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if got.Status() != admin.StatusPending {
		t.Errorf("status = %q, want pending once every service has stamped", got.Status())
	}
	if len(got.FailedServices()) != 0 {
		t.Errorf("failed services must clear on a successful retry, got %v", got.FailedServices())
	}
	if got.AffectedCounts()["media_rows"] != 7 {
		t.Errorf("retry must record the downstream counts: %v", got.AffectedCounts())
	}
	// Local rows are untouched by a retry — the local stamp already succeeded.
	if admintest.CountLive(t, db, "fleet.vehicles") != 0 {
		t.Error("retry must not disturb the local stamp")
	}
}

func TestRetry_onACancelledOperationIs409(t *testing.T) {
	db := admintest.NewDB(t)
	admintest.SeedFleet(t, db, "fleet-1")
	proc := newProcessor(t, db, stubAuth{admin: true}, &stubDownstream{name: "media"})
	op, _ := proc.Create(context.Background(), fleetInput())
	if _, err := proc.Cancel(context.Background(), op.ID(), admin.Actor{UserID: "a", Email: "a@x"}); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if _, err := proc.Retry(context.Background(), op.ID(),
		admin.Actor{UserID: "a", Email: "a@x"}); !errors.Is(err, server.ErrConflict) {
		t.Errorf("want 409 retrying a cancelled operation, got %v", err)
	}
}
```

Add the `failingRestore` stub near the others:

```go
type failingRestore struct {
	stubDownstream
	failRestore bool
}

func (f *failingRestore) Restore(ctx context.Context, opID string) (map[string]int, error) {
	if f.failRestore {
		return nil, errors.New("connection refused")
	}
	return f.stubDownstream.Restore(ctx, opID)
}
```

and initialise it with `failRestore: true` in the test that needs it — set the
field explicitly at construction: `&failingRestore{stubDownstream: stubDownstream{name: "media"}, failRestore: true}`.

- [ ] **Step 2: Run and watch it fail**

Run: `go test ./apps/fleet-service/internal/admin/ -run 'Cancel|Retry' -count=1 -v`
Expected: FAIL — `undefined: admin.Actor`, `Cancel`, `Retry`.

- [ ] **Step 3: Implement both**

Append to `processor.go`:

```go
// Actor is who is performing a lifecycle action, for the audit row.
type Actor struct {
	UserID        string
	Email         string
	CorrelationID string
}

// Cancel restores every row the operation stamped and, once every service has
// restored, marks it cancelled (FR-ADMIN-RESTORE-1).
//
// It deliberately performs NO platform-admin re-verification. Applied literally,
// FR-ADMIN-AUTH-7 would include this endpoint — the recovery path. If
// auth-service is unreachable, failing closed here would block the one action
// that undoes a mistake, during the window when undoing it is still possible.
// The caller has already passed RequirePlatformAdmin on a valid token; that is
// the right amount of authority for a REVERSIBLE action (design §5.4).
func (p *Processor) Cancel(ctx context.Context, opID string, actor Actor) (Operation, error) {
	now := p.d.Now()
	op, err := p.d.Provider.GetOperation(opID)
	if err != nil {
		return Operation{}, err
	}
	switch op.Status() {
	case StatusReaped:
		// Irreversible, and the API says so rather than pretending to succeed.
		return Operation{}, server.Detailed(server.ErrConflict,
			"this operation has been reaped; its data is permanently deleted")
	case StatusCancelled:
		// Already done. Idempotent success rather than a confusing 409: the
		// console offers restore on a list that may be a few seconds stale.
		return op, nil
	}

	// Local restore first and unconditionally: a downstream failure must not
	// hold the product's own data hostage.
	if err := p.d.DB.Transaction(func(tx *gorm.DB) error { return Restore(tx, opID) }); err != nil {
		p.log.WithError(err).WithField("operation_id", opID).Error("local restore")
		return Operation{}, err
	}

	var failed []string
	for _, d := range p.d.Downstream {
		if _, rerr := d.Restore(ctx, opID); rerr != nil {
			p.log.WithError(rerr).WithFields(logrus.Fields{
				"operation_id": opID, "service": d.Name(),
			}).Warn("downstream restore failed; operation stays cancellable")
			failed = append(failed, d.Name())
		}
	}

	// cancelled only when EVERY service has restored. Restore is idempotent, so
	// the correct user action for a partial cancel is to press it again.
	status := StatusCancelled
	if len(failed) > 0 {
		status = StatusPartial
	}
	if err := p.d.Administrator.SetStatus(p.d.DB, opID, status, failed, now); err != nil {
		return Operation{}, err
	}
	if err := p.d.Administrator.InsertAudit(p.d.DB, AuditEvent{
		ID:               uuid.NewString(),
		ActorUserID:      actor.UserID,
		ActorEmail:       actor.Email,
		Action:           ActionPurgeCancelled,
		Scope:            string(op.Scope()),
		TargetType:       op.TargetType(),
		TargetID:         op.TargetID(),
		TargetLabel:      op.TargetLabel(),
		PurgeOperationID: opID,
		AffectedCounts:   op.AffectedCounts(),
		CorrelationID:    actor.CorrelationID,
		CreatedAt:        now,
	}); err != nil {
		p.log.WithError(err).WithField("operation_id", opID).Error("write cancel audit row")
	}

	p.log.WithFields(logrus.Fields{
		"operation_id": opID, "status": status, "failed_services": failed,
		"actor": actor.UserID, "correlation_id": actor.CorrelationID,
	}).Info("admin purge cancelled")

	return p.d.Provider.GetOperation(opID)
}

// Retry re-attempts the downstream stamps for a partial operation
// (FR-ADMIN-PURGE-9). Every downstream stamp is idempotent on
// purge_operation_id, so this is safe to run repeatedly — which is exactly how
// the console presents it.
//
// It DOES re-verify the platform-admin privilege: unlike cancel, this
// re-attempts a destructive write.
func (p *Processor) Retry(ctx context.Context, opID string, actor Actor) (Operation, error) {
	now := p.d.Now()
	ok, err := p.d.Auth.IsPlatformAdmin(ctx, actor.UserID)
	if err != nil {
		p.log.WithError(err).Error("platform-admin re-verification failed; refusing the retry")
		return Operation{}, err
	}
	if !ok {
		return Operation{}, server.ErrForbidden
	}

	op, err := p.d.Provider.GetOperation(opID)
	if err != nil {
		return Operation{}, err
	}
	if op.Status() == StatusReaped || op.Status() == StatusCancelled {
		return Operation{}, server.Detailed(server.ErrConflict,
			"only a pending or partial operation can be retried")
	}

	// Re-resolve the media ids for a record purge. The target rows are
	// soft-deleted, not gone, so the resolver still finds them.
	_, mediaIDs, rerr := p.targets.Resolve(op.Root())
	if rerr != nil && !errors.Is(rerr, server.ErrNotFound) {
		return Operation{}, rerr
	}

	counts, failed := p.fanOutPurge(ctx, op, mediaIDs)
	affected := op.AffectedCounts()
	for k, v := range counts {
		affected[k] = v
	}
	status := StatusPending
	if len(failed) > 0 {
		status = StatusPartial
	}
	if err := p.d.Administrator.SetAffected(p.d.DB, opID, affected); err != nil {
		return Operation{}, err
	}
	if err := p.d.Administrator.SetStatus(p.d.DB, opID, status, failed, now); err != nil {
		return Operation{}, err
	}
	if err := p.d.Administrator.InsertAudit(p.d.DB, AuditEvent{
		ID:               uuid.NewString(),
		ActorUserID:      actor.UserID,
		ActorEmail:       actor.Email,
		Action:           ActionPurgeRetried,
		Scope:            string(op.Scope()),
		TargetType:       op.TargetType(),
		TargetID:         op.TargetID(),
		TargetLabel:      op.TargetLabel(),
		PurgeOperationID: opID,
		AffectedCounts:   affected,
		CorrelationID:    actor.CorrelationID,
		CreatedAt:        now,
	}); err != nil {
		p.log.WithError(err).WithField("operation_id", opID).Error("write retry audit row")
	}

	p.log.WithFields(logrus.Fields{
		"operation_id": opID, "status": status, "failed_services": failed,
		"actor": actor.UserID, "correlation_id": actor.CorrelationID,
	}).Info("admin purge retried")

	return p.d.Provider.GetOperation(opID)
}
```

Add `"errors"` to the imports.

- [ ] **Step 4: Run to green**

Run: `go test ./apps/fleet-service/internal/admin/ -count=1 -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/fleet-service/internal/admin
git commit -m "feat(fleet): add purge cancel and retry"
```

---

### Task 19: the hourly reaper

**Files:**
- Create: `apps/fleet-service/internal/admin/reaper.go`
- Create: `apps/fleet-service/internal/admin/reaper_test.go`
- Modify: `apps/fleet-service/cmd/main.go`

**Interfaces:**
- Produces: `func (p *Processor) ReapDue(ctx context.Context) error`

**The one place order matters (design §8.4).** Downstream **before** local,
because the local `Reap` destroys the `purge_operation_id` values and a crash
between the two must leave enough state to retry. A crash anywhere leaves the
operation `pending`/`partial` and the next tick re-runs it; every step keys on
`purge_operation_id` and is therefore idempotent (FR-ADMIN-RESTORE-6).

- [ ] **Step 1: Write the failing reaper test**

Create `apps/fleet-service/internal/admin/reaper_test.go`:

```go
package admin_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin/admintest"
)

func TestReapDue_hardDeletesPastTheWindowAndMarksReaped(t *testing.T) {
	db := admintest.NewDB(t)
	admintest.SeedFleet(t, db, "fleet-1")
	media := &stubDownstream{name: "media"}
	proc := newProcessor(t, db, stubAuth{admin: true}, media)
	op, err := proc.Create(context.Background(), fleetInput())
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Wind the clock past the recovery window.
	later := admin.NewProcessorClockForTest(proc, testNow.Add(admin.DefaultRecoveryWindow+time.Hour))
	if err := later.ReapDue(context.Background()); err != nil {
		t.Fatalf("reap: %v", err)
	}

	got, err := admin.NewProvider(db).GetOperation(op.ID())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status() != admin.StatusReaped {
		t.Errorf("status = %q, want reaped", got.Status())
	}
	if got.ReapedAt() == nil {
		t.Error("reaped_at must be stamped")
	}
	for _, table := range []string{"fleet.fleets", "fleet.vehicles", "fleet.mileage_records"} {
		if n := admintest.CountRows(t, db, table); n != 0 {
			t.Errorf("%s still has %d rows after the reap", table, n)
		}
	}
	if media.reaped != 1 {
		t.Errorf("media reap called %d times, want 1", media.reaped)
	}
	// FR-ADMIN-AUDIT-1 + FR-ADMIN-UI-13: the reaper's row is attributed to the
	// system, not to the admin who requested the purge days earlier.
	var actor string
	db.Raw(`SELECT actor_user_id FROM fleet.admin_audit_events WHERE action = ?`,
		admin.ActionPurgeReaped).Scan(&actor)
	if actor != admin.ActorSystem {
		t.Errorf("reaper audit actor = %q, want %q", actor, admin.ActorSystem)
	}
	// FR-ADMIN-AUDIT-2: the audit trail survives its own purge.
	if n := admintest.CountRows(t, db, "fleet.admin_audit_events"); n < 2 {
		t.Errorf("audit rows must survive the reap, got %d", n)
	}
}

func TestReapDue_leavesOperationsInsideTheWindowAlone(t *testing.T) {
	db := admintest.NewDB(t)
	admintest.SeedFleet(t, db, "fleet-1")
	proc := newProcessor(t, db, stubAuth{admin: true}, &stubDownstream{name: "media"})
	op, _ := proc.Create(context.Background(), fleetInput())

	if err := proc.ReapDue(context.Background()); err != nil {
		t.Fatalf("reap: %v", err)
	}
	got, _ := admin.NewProvider(db).GetOperation(op.ID())
	if got.Status() != admin.StatusPending {
		t.Errorf("status = %q, want the operation still recoverable", got.Status())
	}
	if admintest.CountRows(t, db, "fleet.vehicles") != 2 {
		t.Error("the reaper hard-deleted rows still inside their recovery window")
	}
}

// design §8.4: downstream BEFORE local. A downstream failure must leave the
// local rows — and therefore the purge_operation_id the next tick needs — in
// place.
func TestReapDue_downstreamFailureLeavesTheOperationRetryable(t *testing.T) {
	db := admintest.NewDB(t)
	admintest.SeedFleet(t, db, "fleet-1")
	media := &failingReap{stubDownstream: stubDownstream{name: "media"}, failReap: true}
	proc := newProcessor(t, db, stubAuth{admin: true}, media)
	op, _ := proc.Create(context.Background(), fleetInput())

	later := admin.NewProcessorClockForTest(proc, testNow.Add(admin.DefaultRecoveryWindow+time.Hour))
	if err := later.ReapDue(context.Background()); err != nil {
		t.Fatalf("a downstream failure must not abort the whole run: %v", err)
	}

	got, _ := admin.NewProvider(db).GetOperation(op.ID())
	if got.Status() == admin.StatusReaped {
		t.Error("an operation whose downstream reap failed must not be marked reaped")
	}
	if admintest.CountRows(t, db, "fleet.vehicles") == 0 {
		t.Error("local rows were destroyed before the downstream reap succeeded — the next tick has nothing to retry")
	}

	// The next tick completes it.
	media.failReap = false
	if err := later.ReapDue(context.Background()); err != nil {
		t.Fatalf("second run: %v", err)
	}
	got, _ = admin.NewProvider(db).GetOperation(op.ID())
	if got.Status() != admin.StatusReaped {
		t.Errorf("the retry tick must complete the reap, got %q", got.Status())
	}
}

// FR-ADMIN-RESTORE-6: running the reaper twice is harmless.
func TestReapDue_isIdempotent(t *testing.T) {
	db := admintest.NewDB(t)
	admintest.SeedFleet(t, db, "fleet-1")
	proc := newProcessor(t, db, stubAuth{admin: true}, &stubDownstream{name: "media"})
	proc.Create(context.Background(), fleetInput())

	later := admin.NewProcessorClockForTest(proc, testNow.Add(admin.DefaultRecoveryWindow+time.Hour))
	for i := 0; i < 2; i++ {
		if err := later.ReapDue(context.Background()); err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
	}
}

type failingReap struct {
	stubDownstream
	failReap bool
}

func (f *failingReap) Reap(ctx context.Context, opID string) (map[string]int, error) {
	if f.failReap {
		return nil, errors.New("connection refused")
	}
	return f.stubDownstream.Reap(ctx, opID)
}
```

- [ ] **Step 2: Run and watch it fail**

Run: `go test ./apps/fleet-service/internal/admin/ -run ReapDue -count=1 -v`
Expected: FAIL — `undefined: ReapDue`, `NewProcessorClockForTest`.

- [ ] **Step 3: Write the reaper**

Create `apps/fleet-service/internal/admin/reaper.go`:

```go
package admin

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// NewProcessorClockForTest returns a copy of the processor with a fixed clock.
// Tests use it to wind past the recovery window without sleeping; production
// code uses the injected Deps.Now.
func NewProcessorClockForTest(p *Processor, now time.Time) *Processor {
	cp := *p
	cp.d.Now = func() time.Time { return now }
	return &cp
}

// ReapDue hard-deletes every operation whose recovery window has elapsed.
//
// Run hourly under database.WithLeaderLock(db, "admin-purge-reap", …). Hourly,
// not daily: jobs.Every's FIRST tick is at T+interval, so a 24-hour job in a
// service that redeploys more often than daily never runs at all — and the
// console shows a countdown to permanence, which a daily cadence would make
// wrong by up to 24 hours (design OQ-5). The tick is cheap: an indexed
// status/purge_after scan that is a no-op on almost every run.
//
// One operation failing never aborts the run: the others are independent, and a
// failure simply leaves that operation pending for the next tick.
func (p *Processor) ReapDue(ctx context.Context) error {
	now := p.d.Now()
	due, err := p.d.Provider.ListDue(now)
	if err != nil {
		return err
	}
	if len(due) == 0 {
		return nil
	}

	var reaped int
	totals := map[string]int{}
	for _, op := range due {
		deleted, rerr := p.reapOne(ctx, op, now)
		if rerr != nil {
			p.log.WithError(rerr).WithField("operation_id", op.ID()).
				Warn("reap failed; the operation stays due and the next tick retries it")
			continue
		}
		reaped++
		for k, v := range deleted {
			totals[k] += v
		}
	}

	// One summary line per run (PRD §8 Observability).
	p.log.WithFields(logrus.Fields{
		"operations_due":    len(due),
		"operations_reaped": reaped,
		"rows_deleted":      totals,
	}).Info("admin purge reaper run complete")
	return nil
}

// reapOne runs the destructive sequence for a single operation.
//
// Order matters exactly once, here: downstream BEFORE local. The local Reap
// destroys the purge_operation_id values, which are the only handle the
// downstream calls have; a crash between the two must leave enough state to
// retry. Every step keys on that id and is idempotent, so a crash anywhere
// leaves the operation pending and the next tick re-runs it
// (FR-ADMIN-RESTORE-6).
func (p *Processor) reapOne(ctx context.Context, op Operation, now time.Time) (map[string]int, error) {
	for _, d := range p.d.Downstream {
		if _, err := d.Reap(ctx, op.ID()); err != nil {
			// Abort THIS operation. Marking it reaped now would strip the ids
			// the next attempt needs and strand the downstream rows forever.
			return nil, err
		}
	}

	var deleted map[string]int
	if err := p.d.DB.Transaction(func(tx *gorm.DB) error {
		var rerr error
		deleted, rerr = Reap(tx, op.ID())
		if rerr != nil {
			return rerr
		}
		if serr := p.d.Administrator.SetStatus(tx, op.ID(), StatusReaped, nil, now); serr != nil {
			return serr
		}
		// Actor is the system: attributing a scheduled deletion to the person
		// who requested it days earlier would misread the trail
		// (FR-ADMIN-UI-13).
		return p.d.Administrator.InsertAudit(tx, AuditEvent{
			ID:               uuid.NewString(),
			ActorUserID:      ActorSystem,
			ActorEmail:       ActorSystem,
			Action:           ActionPurgeReaped,
			Scope:            string(op.Scope()),
			TargetType:       op.TargetType(),
			TargetID:         op.TargetID(),
			TargetLabel:      op.TargetLabel(),
			PurgeOperationID: op.ID(),
			AffectedCounts:   deleted,
			CreatedAt:        now,
		})
	}); err != nil {
		return nil, err
	}
	return deleted, nil
}
```

- [ ] **Step 4: Register the job**

`apps/fleet-service/cmd/main.go`, alongside the other sweeps (the processor is
built in Task 21; add this line in the same commit as that wiring if the
identifier does not exist yet):

```go
	// Admin purge reaper: hard-delete operations past their recovery window.
	// Hourly for the same reason as the vehicle sweep — jobs.Every's first tick
	// is at T+interval — and additionally because the console shows a countdown
	// to permanence, which a daily cadence would make wrong by up to a day
	// (design OQ-5).
	go jobs.Every(ctx, 1*time.Hour, func(ctx context.Context) error {
		_, err := database.WithLeaderLock(db, "admin-purge-reap", func() error {
			return adminProc.ReapDue(ctx)
		})
		if err != nil {
			log.WithError(err).Warn("admin purge reaper failed")
		}
		return err
	})
```

- [ ] **Step 5: Run to green**

Run: `go test ./apps/fleet-service/internal/admin/ -count=1 -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add apps/fleet-service
git commit -m "feat(fleet): add the hourly admin purge reaper"
```

---

### Task 20: the read surface — stats, fleets, users, operations, audit

**Files:**
- Create: `apps/fleet-service/internal/admin/stats.go`
- Create: `apps/fleet-service/internal/admin/stats_test.go`
- Create: `apps/fleet-service/internal/admin/browse.go`
- Create: `apps/fleet-service/internal/admin/browse_test.go`
- Create: `apps/fleet-service/internal/admin/rest.go`
- Create: `apps/fleet-service/internal/admin/resource.go`
- Create: `apps/fleet-service/internal/admin/resource_test.go`

**Interfaces:**
- Produces:
  - `type StatsSource interface { Name() string; Count(ctx context.Context) (int, error); Key() string }`
  - `func (p *Processor) Stats(ctx context.Context) (Stats, error)`; `type Stats struct { Values map[string]*int; Vehicles VehicleCounts; Warnings []string }`; `type VehicleCounts struct { Active, PendingPurge int }`
  - `type DeletedFilter string`; `DeletedInclude`, `DeletedExclude`, `DeletedOnly`; `func ParseDeletedFilter(raw string) (DeletedFilter, error)`
  - `func (p *Processor) ListFleets(ctx context.Context, q string, deleted DeletedFilter, page server.Page) (FleetPage, error)`
  - `func (p *Processor) GetFleet(ctx context.Context, id string) (FleetDetail, error)`
  - `func (p *Processor) ListUsers(ctx context.Context, page server.Page) (UserPage, error)`
  - `func (p *Processor) BlastRadius(root Root) (map[string]int, error)` — thin wrapper over `Count`
  - `func InitializeRoutes(log logrus.FieldLogger, proc *Processor) func(chi.Router)`

**Endpoints** — all under `/admin`, all calling `authz.RequirePlatformAdmin` first:

```
GET    /admin/stats
GET    /admin/fleets?q=&deleted=include|exclude|only&page[number]=&page[size]=
GET    /admin/fleets/{fleetId}
GET    /admin/users?page[number]=&page[size]=
GET    /admin/purge-operations?status=&page[number]=&page[size]=
GET    /admin/purge-operations/{id}
POST   /admin/purge-operations
DELETE /admin/purge-operations/{id}
POST   /admin/purge-operations/{id}/retry
GET    /admin/audit-events?action=&actor=&page[number]=&page[size]=
```

**Three decisions carried from the design:**

- **`?deleted=` is a tri-state, default `include`** (OQ-4). The PRD contradicts
  itself — FR-ADMIN-FLEET-2 says the list defaults to excluding soft-deleted
  fleets, FR-ADMIN-UI-7 says fleets pending purge appear struck through rather
  than vanishing. The second is right: a console whose recovery window is
  invisible by default hides the thing it exists to let you undo. "Deleted" here
  means **admin-stamped only** (`purge_operation_id IS NOT NULL`); fleets removed
  through ordinary product flows stay hidden, because they are not recoverable
  through this console and showing them would imply otherwise.
- **`fleet.fleets` is the one table on `gorm.DeletedAt`**, so every admin fleet
  query must use `Unscoped()` and filter by hand.
- **The list view never loads any fleet's vehicles** (PRD §8 Performance). Counts
  come from aggregate queries; derived vehicle status is computed only in the
  detail view, for one fleet.

- [ ] **Step 1: Write the failing stats test**

Create `apps/fleet-service/internal/admin/stats_test.go`:

```go
package admin_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin/admintest"
)

type stubSource struct {
	key, name string
	n         int
	err       error
}

func (s stubSource) Key() string  { return s.key }
func (s stubSource) Name() string { return s.name }
func (s stubSource) Count(context.Context) (int, error) {
	if s.err != nil {
		return 0, s.err
	}
	return s.n, nil
}

func TestStats_countsLocalDomainsAndSplitsVehicles(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-1")
	admintest.SeedFleet(t, db, "fleet-2")

	// One vehicle stamped by an admin operation, one deleted by a user. Only
	// the first is "pending purge": a user-deleted vehicle is neither active nor
	// recoverable HERE, so counting it would misstate what the console can undo.
	if err := db.Exec(`UPDATE fleet.vehicles SET deleted_at = ?, purge_operation_id = 'op-1'
	                   WHERE id = ?`, testNow, f.VehicleID).Error; err != nil {
		t.Fatalf("stamp: %v", err)
	}
	if err := db.Exec(`UPDATE fleet.vehicles SET deleted_at = ? WHERE id = ?`,
		testNow, f.SecondVehicleID).Error; err != nil {
		t.Fatalf("user delete: %v", err)
	}

	proc := newStatsProcessor(t, db,
		stubSource{key: "users", name: "auth-service", n: 21},
		stubSource{key: "media_objects", name: "media-service", n: 260},
		stubSource{key: "notifications", name: "notification-service", n: 74},
	)
	got, err := proc.Stats(context.Background())
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if got.Vehicles.Active != 2 {
		t.Errorf("active vehicles = %d, want 2 (fleet-2's pair)", got.Vehicles.Active)
	}
	if got.Vehicles.PendingPurge != 1 {
		t.Errorf("pending purge = %d, want 1 — a user-deleted vehicle is not pending purge",
			got.Vehicles.PendingPurge)
	}
	if v := got.Values["fleets"]; v == nil || *v != 2 {
		t.Errorf("fleets = %v, want 2", v)
	}
	if v := got.Values["users"]; v == nil || *v != 21 {
		t.Errorf("users = %v, want 21", v)
	}
	if len(got.Warnings) != 0 {
		t.Errorf("no source failed, so warnings must be empty: %v", got.Warnings)
	}
}

// FR-ADMIN-STATS-5: a single dead service must not blank the dashboard. The
// count is null and the source is NAMED — not zero, which would read as "there
// is no data" rather than "we could not ask".
func TestStats_unreachableSourceIsNullWithAWarning(t *testing.T) {
	db := admintest.NewDB(t)
	admintest.SeedFleet(t, db, "fleet-1")
	proc := newStatsProcessor(t, db,
		stubSource{key: "users", name: "auth-service", n: 21},
		stubSource{key: "notifications", name: "notification-service", err: errors.New("connection refused")},
	)

	got, err := proc.Stats(context.Background())
	if err != nil {
		t.Fatalf("a dead source must not fail the whole call: %v", err)
	}
	if v, ok := got.Values["notifications"]; !ok || v != nil {
		t.Errorf("an unreachable source must be present and null, got %v (present=%v)", v, ok)
	}
	if v := got.Values["users"]; v == nil || *v != 21 {
		t.Errorf("a healthy source must still report: %v", v)
	}
	if len(got.Warnings) != 1 {
		t.Fatalf("want one warning, got %v", got.Warnings)
	}
	if !strings.Contains(got.Warnings[0], "notification-service") {
		t.Errorf("the warning must name the failing source: %q", got.Warnings[0])
	}
}
```

Add `"strings"` and the helper:

```go
func newStatsProcessor(t *testing.T, db *gorm.DB, sources ...admin.StatsSource) *admin.Processor {
	t.Helper()
	return admin.NewProcessor(logrus.New(), admin.Deps{
		DB:            db,
		Provider:      admin.NewProvider(db),
		Administrator: admin.NewAdministrator(db),
		StatsSources:  sources,
		Now:           func() time.Time { return testNow },
	}, stubTargets{})
}
```

- [ ] **Step 2: Run and watch it fail**

Run: `go test ./apps/fleet-service/internal/admin/ -run Stats -count=1 -v`
Expected: FAIL — `undefined: admin.Stats`.

- [ ] **Step 3: Write `stats.go`**

```go
package admin

import (
	"context"
	"fmt"
	"sync"

	"gorm.io/gorm"
)

// StatsSource is one remote count. Key is the attribute name in the response;
// Name is the service, used in the warning text a human reads.
type StatsSource interface {
	Key() string
	Name() string
	Count(ctx context.Context) (int, error)
}

// VehicleCounts splits vehicles into what exists and what the console can still
// undo (FR-ADMIN-STATS-3).
type VehicleCounts struct {
	Active       int `json:"active"`
	PendingPurge int `json:"pending_purge"`
}

// Stats is the /admin/stats payload. A nil value means "we could not ask", and
// the console renders it as an em dash — never as 0, which would read as
// "there is no data" (FR-ADMIN-UI-6).
type Stats struct {
	Values   map[string]*int
	Vehicles VehicleCounts
	Warnings []string
}

// localCounts maps a stats attribute to the table it counts. Vehicles are
// handled separately because they are reported as two numbers.
var localCounts = map[string]string{
	"fleets":                "fleet.fleets",
	"memberships":           "fleet.fleet_memberships",
	"maintenance_records":   "fleet.maintenance_records",
	"maintenance_schedules": "fleet.maintenance_schedules",
	"fuel_logs":             "fleet.fuel_logs",
	"mileage_records":       "fleet.mileage_records",
	"activity_events":       "fleet.activity_events",
}

// Stats gathers solution-wide counts (FR-ADMIN-STATS-1).
//
// Local counts are one indexed pass each; the remote counts are issued
// CONCURRENTLY with per-source error capture, so a slow service costs the
// response one timeout rather than three (PRD §8: under 2s).
//
// Every count excludes soft-deleted rows, so a pending purge is reflected
// immediately and the console never reports data the product no longer shows
// (FR-ADMIN-STATS-2).
func (p *Processor) Stats(ctx context.Context) (Stats, error) {
	out := Stats{Values: map[string]*int{}, Warnings: []string{}}

	for key, table := range localCounts {
		var n int64
		if err := p.d.DB.Raw("SELECT count(*) FROM " + table + " WHERE deleted_at IS NULL").
			Scan(&n).Error; err != nil {
			return Stats{}, fmt.Errorf("count %s: %w", table, err)
		}
		v := int(n)
		out.Values[key] = &v
	}

	// Pending invites are the unaccepted, unexpired ones — the number an
	// operator can act on, not every invite row ever written.
	var invites int64
	if err := p.d.DB.Raw(`SELECT count(*) FROM fleet.fleet_invites
	                      WHERE deleted_at IS NULL AND accepted_at IS NULL`).Scan(&invites).Error; err != nil {
		return Stats{}, fmt.Errorf("count pending invites: %w", err)
	}
	pending := int(invites)
	out.Values["pending_invites"] = &pending

	// pending_purge is admin-stamped only. A vehicle a USER deleted is neither
	// active nor recoverable through this console, so counting it as pending
	// would misstate what the operator can undo.
	if err := p.d.DB.Raw(`SELECT count(*) FROM fleet.vehicles WHERE deleted_at IS NULL`).
		Scan(&out.Vehicles.Active).Error; err != nil {
		return Stats{}, fmt.Errorf("count active vehicles: %w", err)
	}
	if err := p.d.DB.Raw(`SELECT count(*) FROM fleet.vehicles
	                      WHERE deleted_at IS NOT NULL AND purge_operation_id IS NOT NULL`).
		Scan(&out.Vehicles.PendingPurge).Error; err != nil {
		return Stats{}, fmt.Errorf("count pending-purge vehicles: %w", err)
	}

	// Remote sources, concurrently. sync.WaitGroup rather than errgroup: the
	// repo has no errgroup dependency and adding one for six lines is not worth
	// it. Per-source capture, because a failure is a WARNING, not an error —
	// errgroup's first-error semantics are the wrong shape here.
	type result struct {
		key, name string
		n         int
		err       error
	}
	results := make([]result, len(p.d.StatsSources))
	var wg sync.WaitGroup
	for i, s := range p.d.StatsSources {
		wg.Add(1)
		go func(i int, s StatsSource) {
			defer wg.Done()
			n, err := s.Count(ctx)
			results[i] = result{key: s.Key(), name: s.Name(), n: n, err: err}
		}(i, s)
	}
	wg.Wait()

	for _, r := range results {
		if r.err != nil {
			p.log.WithError(r.err).WithField("source", r.name).Warn("admin stats source unreachable")
			out.Values[r.key] = nil
			out.Warnings = append(out.Warnings,
				fmt.Sprintf("%s unreachable; %s count omitted", r.name, r.key))
			continue
		}
		n := r.n
		out.Values[r.key] = &n
	}
	return out, nil
}

// BlastRadius is the per-domain breakdown the console shows above the purge
// control. It is literally the same Count the purge's Stamp will use, which is
// what makes the displayed figures and the affected rows provably equal
// (FR-ADMIN-UI-9).
func (p *Processor) BlastRadius(root Root) (map[string]int, error) {
	return Count(p.d.DB, root)
}
```

This file imports only `context`, `fmt` and `sync`.

**Extend `Deps` (declared in Task 17) with the two fields this task needs.** Add
them to the struct in `processor.go` rather than declaring a second dependency
bundle:

```go
type Deps struct {
	DB            *gorm.DB
	Provider      Provider
	Administrator Administrator
	Auth          AuthVerifier
	Downstream    []Downstream
	// StatsSources are the remote counts /admin/stats fans out to. Separate
	// from Downstream because the sets differ: auth-service contributes a count
	// but is never purged.
	StatsSources []StatsSource
	// AuthUsers resolves member ids to email and display name for the fleet
	// detail view. A failure here is a warning, not an error (FR-ADMIN-FLEET-5).
	AuthUsers UserResolver
	Window    time.Duration
	Now       func() time.Time
}

// UserResolver is the slice of adminclient.AuthClient the browse endpoints need.
type UserResolver interface {
	Users(ctx context.Context, ids []string) (map[string]adminclient.User, error)
	ListUsers(ctx context.Context, page server.Page) ([]adminclient.User, int, error)
}
```

- [ ] **Step 4: Write `browse.go` — fleets, fleet detail, users**

Write the test first, `browse_test.go`, covering the four properties that matter:

```go
// OQ-4: the default INCLUDES admin-stamped fleets, struck through in the UI. A
// console whose recovery window is invisible by default hides the thing it
// exists to let you undo.
func TestListFleets_deletedFilterTriState(t *testing.T) {
	db := admintest.NewDB(t)
	admintest.SeedFleet(t, db, "fleet-1")
	admintest.SeedFleet(t, db, "fleet-2")
	admintest.SeedFleet(t, db, "fleet-3")
	// fleet-2 is admin-stamped; fleet-3 was deleted through an ordinary product
	// flow and is NOT recoverable through this console.
	db.Exec(`UPDATE fleet.fleets SET deleted_at = ?, purge_operation_id = 'op-1' WHERE id = 'fleet-2'`, testNow)
	db.Exec(`UPDATE fleet.fleets SET deleted_at = ? WHERE id = 'fleet-3'`, testNow)

	proc := newBrowseProcessor(t, db)
	page := server.Page{Number: 1, Size: 25}

	ids := func(f admin.FleetPage) map[string]bool {
		out := map[string]bool{}
		for _, row := range f.Rows {
			out[row.ID] = true
		}
		return out
	}

	got, err := proc.ListFleets(context.Background(), "", admin.DeletedInclude, page)
	if err != nil {
		t.Fatalf("include: %v", err)
	}
	if !ids(got)["fleet-1"] || !ids(got)["fleet-2"] {
		t.Errorf("include must show live and admin-stamped fleets: %v", ids(got))
	}
	if ids(got)["fleet-3"] {
		t.Error("a product-deleted fleet is not recoverable here and must stay hidden")
	}

	got, _ = proc.ListFleets(context.Background(), "", admin.DeletedExclude, page)
	if ids(got)["fleet-2"] {
		t.Error("exclude must hide admin-stamped fleets")
	}
	got, _ = proc.ListFleets(context.Background(), "", admin.DeletedOnly, page)
	if !ids(got)["fleet-2"] || ids(got)["fleet-1"] {
		t.Errorf("only must show exactly the pending set: %v", ids(got))
	}
}

func TestParseDeletedFilter(t *testing.T) {
	if got, _ := admin.ParseDeletedFilter(""); got != admin.DeletedInclude {
		t.Errorf("the default must be include, got %q", got)
	}
	for _, raw := range []string{"include", "exclude", "only"} {
		if _, err := admin.ParseDeletedFilter(raw); err != nil {
			t.Errorf("%q must parse, got %v", raw, err)
		}
	}
	if _, err := admin.ParseDeletedFilter("true"); !errors.Is(err, server.ErrValidation) {
		t.Errorf("an unknown value must be 422, got %v", err)
	}
}

// FR-ADMIN-FLEET-1: the caller is a member of nothing, and sees everything.
// That is the entire point of the admin tier, and the one behaviour a
// mistakenly-copied RequireSameFleet would break silently.
func TestListFleets_returnsFleetsTheCallerIsNotAMemberOf(t *testing.T) {
	db := admintest.NewDB(t)
	admintest.SeedFleet(t, db, "fleet-1")
	admintest.SeedFleet(t, db, "fleet-2")
	proc := newBrowseProcessor(t, db)

	got, err := proc.ListFleets(context.Background(), "", admin.DeletedInclude,
		server.Page{Number: 1, Size: 25})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if got.Total != 2 || len(got.Rows) != 2 {
		t.Fatalf("want both fleets, got %d rows / total %d", len(got.Rows), got.Total)
	}
	for _, row := range got.Rows {
		if row.MemberCount != 1 {
			t.Errorf("%s member count = %d, want 1", row.ID, row.MemberCount)
		}
		if row.VehicleCount != 2 {
			t.Errorf("%s vehicle count = %d, want 2", row.ID, row.VehicleCount)
		}
	}
}

// FR-ADMIN-FLEET-5: a failed user lookup still yields the fleet, with ids and
// empty names, flagged in warnings. Failing the whole request because
// auth-service is slow would make the console useless exactly when an operator
// is trying to diagnose why.
func TestGetFleet_degradesWhenAuthServiceIsUnreachable(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-1")
	proc := newBrowseProcessorWithUsers(t, db, failingUserResolver{})

	got, err := proc.GetFleet(context.Background(), f.FleetID)
	if err != nil {
		t.Fatalf("an unreachable auth-service must not fail the detail view: %v", err)
	}
	if len(got.Members) != 1 {
		t.Fatalf("want one member row, got %d", len(got.Members))
	}
	if got.Members[0].UserID != f.OwnerUserID {
		t.Errorf("the member row must still carry its user id, got %q", got.Members[0].UserID)
	}
	if got.Members[0].Email != "" {
		t.Errorf("an unresolved email must be empty, not invented: %q", got.Members[0].Email)
	}
	if len(got.Warnings) == 0 {
		t.Error("a degraded lookup must be flagged in warnings")
	}
}

// failingUserResolver stands in for an unreachable auth-service.
type failingUserResolver struct{}

func (failingUserResolver) Users(context.Context, []string) (map[string]adminclient.User, error) {
	return nil, errors.New("connection refused")
}

func (failingUserResolver) ListUsers(context.Context, server.Page) ([]adminclient.User, int, error) {
	return nil, 0, errors.New("connection refused")
}
```

`newBrowseProcessor(t, db)` builds a `Processor` with a `UserResolver` that
returns an entry for every requested id; `newBrowseProcessorWithUsers(t, db, r)`
takes the resolver explicitly. Both mirror `newProcessor` from
`processor_test.go`, differing only in which `Deps` fields they populate.

Then `browse.go`:

```go
package admin

import (
	"context"
	"strings"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// DeletedFilter is the tri-state ?deleted= parameter.
//
// It replaces the PRD's ?include_deleted= boolean because the PRD contradicted
// itself: FR-ADMIN-FLEET-2 wanted soft-deleted fleets hidden by default,
// FR-ADMIN-UI-7 wanted them struck through with a countdown rather than
// vanishing. The second is right — a console whose recovery window is invisible
// by default hides the thing it exists to let you undo (design OQ-4).
type DeletedFilter string

const (
	// DeletedInclude is the default: live fleets plus those pending purge.
	DeletedInclude DeletedFilter = "include"
	// DeletedExclude is the "show me the live platform" view.
	DeletedExclude DeletedFilter = "exclude"
	// DeletedOnly is the "what is pending" view.
	DeletedOnly DeletedFilter = "only"
)

// ParseDeletedFilter reads the query parameter, defaulting to include.
func ParseDeletedFilter(raw string) (DeletedFilter, error) {
	switch DeletedFilter(raw) {
	case "":
		return DeletedInclude, nil
	case DeletedInclude, DeletedExclude, DeletedOnly:
		return DeletedFilter(raw), nil
	}
	return "", server.Detailed(server.ErrValidation, "deleted must be include, exclude or only")
}

// FleetRow is one row of the admin fleet list. Counts come from aggregate
// queries — the list NEVER loads any fleet's vehicles (PRD §8 Performance).
type FleetRow struct {
	ID               string
	Name             string
	CreatedAt        time.Time
	OwnerUserID      string
	OwnerEmail       string
	OwnerDisplayName string
	MemberCount      int
	VehicleCount     int
	PendingPurge     bool
	PurgeAfter       *time.Time
}

// FleetPage is a page of the fleet list plus any degradation warnings.
type FleetPage struct {
	Rows     []FleetRow
	Total    int
	Warnings []string
}

// ListFleets returns every fleet in the system, regardless of the caller's
// membership (FR-ADMIN-FLEET-1).
//
// fleet.fleets is the one table on gorm.DeletedAt, so this query is Unscoped and
// filters deleted_at by hand — otherwise GORM would silently hide exactly the
// rows the ?deleted= filter exists to show.
//
// "deleted" means ADMIN-STAMPED (purge_operation_id IS NOT NULL). A fleet removed
// through an ordinary product flow is not recoverable through this console, and
// showing it would imply otherwise.
func (p *Processor) ListFleets(ctx context.Context, q string, deleted DeletedFilter, page server.Page) (FleetPage, error) {
	where := []string{}
	args := []any{}
	switch deleted {
	case DeletedExclude:
		where = append(where, "f.deleted_at IS NULL")
	case DeletedOnly:
		where = append(where, "f.deleted_at IS NOT NULL AND f.purge_operation_id IS NOT NULL")
	default: // DeletedInclude
		where = append(where, "(f.deleted_at IS NULL OR f.purge_operation_id IS NOT NULL)")
	}
	if s := strings.TrimSpace(q); s != "" {
		// Name search only at the SQL layer; owner-email search is applied after
		// the auth-service lookup below, because emails do not live in this
		// database and a cross-service join is forbidden (design D2).
		where = append(where, "lower(f.name) LIKE ?")
		args = append(args, "%"+strings.ToLower(s)+"%")
	}
	pred := strings.Join(where, " AND ")

	var total int64
	if err := p.d.DB.Raw("SELECT count(*) FROM fleet.fleets f WHERE "+pred, args...).
		Scan(&total).Error; err != nil {
		return FleetPage{}, fmt.Errorf("count fleets: %w", err)
	}

	// The counts are correlated sub-queries, not a join with GROUP BY: the list
	// must never load any fleet's vehicles (PRD §8 Performance), and two
	// count(*) over indexed fleet_id columns is what "never" looks like.
	//
	// purge_after comes from the operation row so the console can render the
	// countdown chip without a second round trip (FR-ADMIN-UI-7).
	const listSQL = `
		SELECT f.id, f.name, f.created_at, f.deleted_at, f.purge_operation_id,
		       f.created_by_user_id AS owner_user_id,
		       (SELECT count(*) FROM fleet.fleet_memberships m
		          WHERE m.fleet_id = f.id AND m.deleted_at IS NULL) AS member_count,
		       (SELECT count(*) FROM fleet.vehicles v
		          WHERE v.fleet_id = f.id AND v.deleted_at IS NULL) AS vehicle_count,
		       (SELECT o.purge_after FROM fleet.purge_operations o
		          WHERE o.id = f.purge_operation_id) AS purge_after
		FROM fleet.fleets f
		WHERE ` + "%s" + `
		ORDER BY f.created_at DESC
		LIMIT ? OFFSET ?`

	type row struct {
		ID               string
		Name             string
		CreatedAt        time.Time
		DeletedAt        *time.Time
		PurgeOperationID *string
		OwnerUserID      string
		MemberCount      int
		VehicleCount     int
		PurgeAfter       *time.Time
	}
	var rows []row
	listArgs := append(append([]any{}, args...), page.Size, page.Offset())
	if err := p.d.DB.Raw(fmt.Sprintf(listSQL, pred), listArgs...).Scan(&rows).Error; err != nil {
		return FleetPage{}, fmt.Errorf("list fleets: %w", err)
	}

	out := FleetPage{Rows: make([]FleetRow, 0, len(rows)), Total: int(total), Warnings: []string{}}
	ownerIDs := make([]string, 0, len(rows))
	for _, r := range rows {
		ownerIDs = append(ownerIDs, r.OwnerUserID)
	}

	// Owner identity comes over HTTP, never from a cross-service join
	// (design D2). A failure degrades the row rather than the request: ids stay,
	// names are empty, and the caller is told (FR-ADMIN-FLEET-5).
	owners := map[string]adminclient.User{}
	if len(ownerIDs) > 0 && p.d.AuthUsers != nil {
		resolved, err := p.d.AuthUsers.Users(ctx, ownerIDs)
		if err != nil {
			p.log.WithError(err).Warn("owner lookup failed; fleet list will omit owner names")
			out.Warnings = append(out.Warnings, "auth-service unreachable; owner names omitted")
		} else {
			owners = resolved
		}
	}

	for _, r := range rows {
		o := owners[r.OwnerUserID]
		out.Rows = append(out.Rows, FleetRow{
			ID:               r.ID,
			Name:             r.Name,
			CreatedAt:        r.CreatedAt,
			OwnerUserID:      r.OwnerUserID,
			OwnerEmail:       o.Email,
			OwnerDisplayName: o.DisplayName,
			MemberCount:      r.MemberCount,
			VehicleCount:     r.VehicleCount,
			PendingPurge:     r.DeletedAt != nil && r.PurgeOperationID != nil,
			PurgeAfter:       r.PurgeAfter,
		})
	}
	return out, nil
}
```

`?q=` matches fleet name in SQL. Owner-email search is applied **after** the
auth-service lookup, in Go, over the page's resolved owners — emails do not live
in this database and a cross-service join is forbidden. That means an
email-only match can fall outside the current page; the console's search box is
labelled "fleet name or owner email on this page" rather than implying a
global search it cannot deliver.

`GetFleet` returns:

```go
// FleetDetail is everything the fleet inspector's right pane needs.
type FleetDetail struct {
	FleetRow
	Members  []MemberRow  // user id, email, display name, role, joined
	Vehicles []VehicleRow // id, nickname, make, model, year, mileage, status, pending purge
	Invites  []InviteRow  // email, role, expires, pending only
	Counts   map[string]int
	Warnings []string
}
```

with `Counts` produced by `BlastRadius(Root{Scope: ScopeFleet, TargetID: id})`,
so the detail counts and the blast-radius panel are the same numbers by
construction. Vehicle status is derived server-side using the existing
`vehicle.StatusDeps` machinery — affordable for one fleet, which is why the list
view carries counts only.

`ListUsers` (FR-ADMIN-FLEET-6) calls `AuthUsers.ListUsers(ctx, page)` — the
paginated form of auth-service's internal lookup, added in Task 12 and exposed on
the client in Task 15 — then joins fleet memberships **locally**, per user id,
against `fleet.fleet_memberships` and `fleet.fleets`. The join is local because a
cross-service database join is forbidden (design D2) and because memberships are
this service's own data; only the identity half comes over HTTP.

- [ ] **Step 5: Write `rest.go` and `resource.go`**

`rest.go` holds the JSON:API transforms. Types are `admin-stats`, `admin-fleets`,
`admin-users`, `purge-operations`, `admin-audit-events`. Attribute names match
PRD §5 exactly (`snake_case` inside `attributes`, matching the PRD's examples).

`resource.go` wires the ten routes. Every handler starts identically:

```go
// InitializeRoutes wires the /admin tree. Register it in its OWN chi group with
// authmw.JWT and nothing else shared — the separation from the ordinary
// authenticated group is the safety argument for the whole cross-fleet API, and
// arch_test.go enforces it (risks.md R7).
func InitializeRoutes(log logrus.FieldLogger, proc *Processor) func(chi.Router) {
	return func(r chi.Router) {
		r.Get("/admin/stats", func(w http.ResponseWriter, req *http.Request) {
			id := auth.IdentityFromContext(req.Context())
			if err := authz.RequirePlatformAdmin(id); err != nil {
				server.WriteError(w, err)
				return
			}
			s, err := proc.Stats(req.Context())
			if err != nil {
				log.WithError(err).Error("admin stats")
				server.WriteError(w, errInternal)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{Data: TransformStats(s)})
		})
		// … the other nine, same guard-first shape …
	}
}
```

Declare `errInternal` in this package the way `user/resource.go` does — a bare
sentinel, because `server.WriteError` copies `err.Error()` into the response
title and returning the underlying error would publish database internals.

The purge-create handler decodes with `server.RegisterInputHandler` and maps
sentinels:

```go
		r.Post("/admin/purge-operations", server.RegisterInputHandler(
			func(w http.ResponseWriter, req *http.Request, attrs struct {
				Scope        string `json:"scope"`
				TargetType   string `json:"target_type"`
				TargetID     string `json:"target_id"`
				Confirmation string `json:"confirmation"`
			}) {
				identity := auth.IdentityFromContext(req.Context())
				if err := authz.RequirePlatformAdmin(identity); err != nil {
					server.WriteError(w, err)
					return
				}
				op, err := proc.Create(req.Context(), CreateInput{
					Scope:         Scope(attrs.Scope),
					TargetType:    attrs.TargetType,
					TargetID:      attrs.TargetID,
					Confirmation:  attrs.Confirmation,
					ActorUserID:   identity.UserID,
					ActorEmail:    identity.Email,
					CorrelationID: telemetry.CorrelationIDFromContext(req.Context()),
				})
				if err != nil {
					// Client errors are not incidents — do not log them.
					if errors.Is(err, server.ErrValidation) || errors.Is(err, server.ErrConflict) ||
						errors.Is(err, server.ErrNotFound) || errors.Is(err, server.ErrForbidden) {
						server.WriteError(w, err)
						return
					}
					log.WithError(err).Error("create purge operation")
					server.WriteError(w, errInternal)
					return
				}
				server.WriteJSON(w, http.StatusCreated,
					server.Document{Data: TransformOperation(op)})
			}))
```

`DELETE /admin/purge-operations/{id}` maps `ErrOperationNotFound` → 404 and the
reaped conflict → 409; `POST …/{id}/retry` the same.

- [ ] **Step 6: Write `resource_test.go`**

Cover the authorization surface, which is the part a reviewer must see proven:

```go
// PRD §10: every /admin endpoint is 403 for a non-admin and 401 for an
// anonymous caller. The table is exhaustive on purpose — a new endpoint added
// without the guard is exactly the failure this catches.
func TestAdminRoutes_rejectNonAdmins(t *testing.T) {
	routes := []struct{ method, path string }{
		{http.MethodGet, "/admin/stats"},
		{http.MethodGet, "/admin/fleets"},
		{http.MethodGet, "/admin/fleets/fleet-1"},
		{http.MethodGet, "/admin/users"},
		{http.MethodGet, "/admin/purge-operations"},
		{http.MethodGet, "/admin/purge-operations/op-1"},
		{http.MethodPost, "/admin/purge-operations"},
		{http.MethodDelete, "/admin/purge-operations/op-1"},
		{http.MethodPost, "/admin/purge-operations/op-1/retry"},
		{http.MethodGet, "/admin/audit-events"},
	}
	for _, rt := range routes {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			rec := serveAs(t, auth.Identity{UserID: "u1", Role: "owner", ActiveFleetID: "f1"}, rt)
			if rec.Code != http.StatusForbidden {
				t.Errorf("status = %d, want 403 for a non-admin", rec.Code)
			}
		})
	}
}

// FR-ADMIN-AUTH-9: no admin endpoint may require an active fleet. An
// administrator standing in the wreckage of the system purge they just ran must
// still reach every one of them.
func TestAdminRoutes_doNotRequireAnActiveFleet(t *testing.T) {
	for _, rt := range readRoutes {
		rec := serveAs(t, auth.Identity{UserID: "u1", PlatformAdmin: true}, rt)
		if rec.Code == http.StatusNotFound && strings.Contains(rec.Body.String(), "not found") {
			// A 404 from a missing FLEET is fine; a 404 from the guard is not.
			continue
		}
		if rec.Code == http.StatusForbidden {
			t.Errorf("%s %s returned 403 for a fleetless admin", rt.method, rt.path)
		}
	}
}
```

`serveAs` builds the chi router with `InitializeRoutes` and injects the identity
via `auth.WithIdentity` on the request context, bypassing the JWT middleware.

- [ ] **Step 7: Run to green**

Run: `go test ./apps/fleet-service/... -count=1`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add apps/fleet-service
git commit -m "feat(fleet): add the admin read surface and HTTP routes"
```

---

### Task 21: wire the `/admin` route group and ship the config

**Files:**
- Modify: `apps/fleet-service/cmd/main.go`
- Modify: `deploy/k8s/base/fleet-service/configmap.yaml`
- Modify: `deploy/k8s/base/auth-service/configmap.yaml`
- Modify: `deploy/compose/docker-compose.yml` (if it sets service env; check first)

**Interfaces:**
- Consumes: everything from Tasks 15–20.
- Produces: a running `/admin` tree at the public path `/api/fleet/admin/…`.

**No routing change is needed for the public surface.** `/api/fleet/admin/…`
matches the existing priority-100 `PathPrefix('/api/fleet')` router and is
stripped to `/admin/…`; the priority-200 `internal-deny` regex
(`^/+api/+fleet[^/]*/*internal`) does not match it. Verify that in Step 4 rather
than assuming it.

- [ ] **Step 1: Build the admin dependency graph in the composition root**

`apps/fleet-service/cmd/main.go`:

```go
	// ---- Platform admin console ------------------------------------------
	//
	// Registered as its OWN chi group below, a sibling of the authenticated
	// group. Nothing is shared but the JWT middleware: the separation from the
	// ordinary tree is the entire safety argument for a cross-fleet API, and
	// internal/admin/arch_test.go enforces it in both directions (risks.md R7).
	authAdmin := adminclient.NewAuthClient(config.Get("AUTH_INTERNAL_URL", "http://auth-service:8080"))
	mediaAdmin := adminclient.NewMediaClient(config.Get("MEDIA_INTERNAL_URL", "http://media-service:8080"))
	notifAdmin := adminclient.NewNotificationClient(
		config.Get("NOTIFICATION_INTERNAL_URL", "http://notification-service:8080"))

	adminProc := admin.NewProcessor(log, admin.Deps{
		DB:            db,
		Provider:      admin.NewProvider(db),
		Administrator: admin.NewAdministrator(db),
		Auth:          authAdmin,
		AuthUsers:     authAdmin,
		Downstream: []admin.Downstream{
			admin.NamedDownstream{Label: "media", Client: mediaAdmin},
			admin.NamedDownstream{Label: "notification", Client: notifAdmin},
		},
		StatsSources: []admin.StatsSource{
			admin.NamedStatsSource{AttrKey: "users", Service: "auth-service", Fn: authAdmin.Stats},
			admin.NamedStatsSource{AttrKey: "media_objects", Service: "media-service", Fn: mediaAdmin.Stats},
			admin.NamedStatsSource{AttrKey: "notifications", Service: "notification-service", Fn: notifAdmin.Stats},
		},
		Window: admin.RecoveryWindow(config.Get("ADMIN_PURGE_RECOVERY_WINDOW", "120h")),
	}, admin.NewTargetResolver(db, vehicleStatusDeps))
```

`admin.NamedDownstream` and `admin.NamedStatsSource` are thin adapters in
`admin/adapters.go` that give a client a `Name()`/`Key()` — the composition root
is the only place that knows both the client and its label, exactly as
`variantLookup` does in media-service's main:

```go
// NamedDownstream adapts an adminclient to the Downstream port, attaching the
// label that lands in failed_services and, through it, in the console's
// "Media not deleted" wording.
//
// It lives here rather than in adminclient so that package stays a transport
// concern and this one owns the vocabulary the operator sees.
type NamedDownstream struct {
	Label  string
	Client interface {
		Purge(ctx context.Context, req adminclient.PurgeRequest) (map[string]int, error)
		Restore(ctx context.Context, opID string) (map[string]int, error)
		Reap(ctx context.Context, opID string) (map[string]int, error)
	}
}

func (n NamedDownstream) Name() string { return n.Label }
func (n NamedDownstream) Purge(ctx context.Context, req adminclient.PurgeRequest) (map[string]int, error) {
	return n.Client.Purge(ctx, req)
}
func (n NamedDownstream) Restore(ctx context.Context, opID string) (map[string]int, error) {
	return n.Client.Restore(ctx, opID)
}
func (n NamedDownstream) Reap(ctx context.Context, opID string) (map[string]int, error) {
	return n.Client.Reap(ctx, opID)
}

// NamedStatsSource adapts a bare count function to the StatsSource port.
type NamedStatsSource struct {
	AttrKey string
	Service string
	Fn      func(ctx context.Context) (int, error)
}

func (n NamedStatsSource) Key() string  { return n.AttrKey }
func (n NamedStatsSource) Name() string { return n.Service }
func (n NamedStatsSource) Count(ctx context.Context) (int, error) { return n.Fn(ctx) }
```

`admin.NewTargetResolver(db, vehicleStatusDeps)` resolves a root's label and, for
a record-scope vehicle purge, the media ids: `fleet.vehicle_media.media_id` plus
`fleet.maintenance_record_documents.media_id` for that vehicle's records
(design OQ-1). It returns `server.ErrNotFound` for an unknown id.

- [ ] **Step 2: Register the route group**

```go
		AddRouteInitializer(func(r chi.Router) {
			r.Group(func(pr chi.Router) {
				pr.Use(authmw.JWT(keyfn, authmw.WithLogger(log)))
				fleet.InitializeRoutes(...)(pr)
				// … the existing eleven …
			})
		}).
		// The admin tree: its own group, JWT and nothing else shared. Every
		// handler calls authz.RequirePlatformAdmin; none calls RequireSameFleet.
		// Public paths are gateway-prefixed /api/fleet/admin/…, which match the
		// existing priority-100 /api/fleet router and are NOT matched by the
		// priority-200 internal-deny regex.
		AddRouteInitializer(func(r chi.Router) {
			r.Group(func(ar chi.Router) {
				ar.Use(authmw.JWT(keyfn, authmw.WithLogger(log)))
				admin.InitializeRoutes(log, adminProc)(ar)
			})
		}).
```

- [ ] **Step 3: Ship the ConfigMap keys**

`deploy/k8s/base/fleet-service/configmap.yaml`:

```yaml
  # Platform admin console. The recovery window is how long a purge stays
  # cancellable; 120h is the 5 days the PRD specifies and matches the vehicle
  # sweep's own window, so both recovery stories tell users the same thing.
  # An unparseable value falls back to 120h rather than failing the boot.
  ADMIN_PURGE_RECOVERY_WINDOW: "120h"
  # Cluster-internal ClusterVIPs for the admin fan-out. Never traverse the edge;
  # the priority-200 internal-deny rules keep the far ends off the internet.
  AUTH_INTERNAL_URL: "http://auth-service:8080"
  NOTIFICATION_INTERNAL_URL: "http://notification-service:8080"
```

`deploy/k8s/base/auth-service/configmap.yaml`:

```yaml
  # Seeds auth.platform_admins at startup and at first login; NEVER consulted
  # per request (FR-ADMIN-AUTH-3). The table is the runtime source of truth, so
  # revoking an admin is a DELETE with no redeploy.
  PLATFORM_ADMIN_BOOTSTRAP_EMAILS: "jtumidanski@gmail.com"
```

Check `deploy/compose/docker-compose.yml` for per-service `environment:` blocks;
if the services read the same keys there, add all four with the same values.

- [ ] **Step 4: Verify routing and manifests**

Run:
```sh
go build github.com/jtumidanski/myfleet/... && go test ./apps/fleet-service/... -count=1
kustomize build deploy/k8s/overlays/main  > /tmp/claude-1000/main.yaml
kustomize build deploy/k8s/overlays/local > /tmp/claude-1000/local.yaml
grep -c 'ADMIN_PURGE_RECOVERY_WINDOW' /tmp/claude-1000/main.yaml
grep -c 'PLATFORM_ADMIN_BOOTSTRAP_EMAILS' /tmp/claude-1000/main.yaml
kubectl apply --dry-run=server -f /tmp/claude-1000/main.yaml
kubectl apply --dry-run=server -f /tmp/claude-1000/local.yaml
make manifests
```
Expected: build and tests PASS; both greps return `1`; both dry-runs succeed;
`make manifests` passes (no PVCs, Secrets, ClusterRole, or placeholders in
`main`).

Confirm the public admin path is **not** caught by the internal-deny rule. The
regex is `(?i)^/+api/+fleet[^/]*/*internal`; `/api/fleet/admin/stats` has `admin`
where the regex needs `internal`, so it falls through to the priority-100 router.
Sanity-check it against the rendered manifest rather than reasoning alone:

```sh
grep -A2 'api/+fleet\[\^/\]\*/\*internal' /tmp/claude-1000/main.yaml
```

- [ ] **Step 5: Commit**

```bash
git add apps/fleet-service deploy
git commit -m "feat(fleet): wire the /admin route group and ship its configuration"
```

---

# Phase 5 — The web console (Tasks 22–28)

`ui-directions.html` in this task folder is the visual reference — Direction C,
a dedicated shell with the fleet inspector at the centre and the purge queue as a
peer section. **Open it before building any screen.**

---

### Task 22: the three missing UI primitives

**Files:**
- Create: `apps/web/src/components/ui/dialog.tsx`
- Create: `apps/web/src/components/ui/table.tsx`
- Create: `apps/web/src/components/ui/badge.tsx`
- Create: `apps/web/src/components/ui/badge.test.tsx`
- Modify: `apps/web/package.json`

**Interfaces:**
- Produces: `Dialog`, `DialogTrigger`, `DialogContent`, `DialogHeader`,
  `DialogTitle`, `DialogDescription`, `DialogFooter`, `DialogClose`;
  `Table`, `TableHeader`, `TableBody`, `TableRow`, `TableHead`, `TableCell`,
  `TableCaption`; `Badge` with `variant: 'default' | 'secondary' | 'outline' | 'success' | 'warning' | 'danger' | 'info'`.

**Conventions to match** (the nine components already in `components/ui/`):
`cva` for variants, `cn` from `../../lib/utils`, `React.forwardRef`, a
`displayName`, named exports, **theme tokens only**. `conventions.test.ts` fails
the build on any `bg-red-500`-style class, so the status variants must use the
semantic families from `index.css`.

`dialog` needs `@radix-ui/react-dialog` — a new dependency, consistent with the
`react-label` / `react-select` / `react-switch` / `react-slot` already present.
`table` and `badge` are plain markup and need nothing new.

- [ ] **Step 1: Write the failing badge test**

Create `apps/web/src/components/ui/badge.test.tsx`:

```tsx
import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { Badge } from './badge';

describe('Badge', () => {
  it('renders its children', () => {
    render(<Badge>Recoverable</Badge>);
    expect(screen.getByText('Recoverable')).toBeInTheDocument();
  });

  // The console leans on these three for purge status, so they must exist as
  // named variants rather than as ad-hoc classNames at each call site — that is
  // how the same status ends up two different colours on two screens.
  it.each(['success', 'warning', 'danger', 'info'] as const)(
    'supports the %s status variant',
    (variant) => {
      const { container } = render(<Badge variant={variant}>x</Badge>);
      expect(container.firstChild).toHaveClass(`bg-${variant}-subtle`);
    },
  );
});
```

- [ ] **Step 2: Run and watch it fail**

Run: `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22 && npm run -w apps/web test -- badge`
Expected: FAIL — cannot resolve `./badge`.

- [ ] **Step 3: Write `badge.tsx`**

```tsx
import * as React from 'react';
import { cva, type VariantProps } from 'class-variance-authority';
import { cn } from '../../lib/utils';

/**
 * Status chip.
 *
 * The four status variants use the -subtle / -subtle-foreground / -border token
 * trio from index.css, not the bare --success/--warning/--danger/--info tokens:
 * the bare ones are for TEXT on --background, and a chip needs a fill. `danger`
 * is deliberately not `destructive` — that token is reserved for destructive
 * CONTROLS under the task-003 contract, and a chip is a label, not a button.
 */
const badgeVariants = cva(
  'inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-medium transition-colors focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-2',
  {
    variants: {
      variant: {
        default: 'border-transparent bg-primary text-primary-foreground',
        secondary: 'border-transparent bg-secondary text-secondary-foreground',
        outline: 'border-border text-foreground',
        success: 'bg-success-subtle text-success-subtle-foreground border-success-border',
        warning: 'bg-warning-subtle text-warning-subtle-foreground border-warning-border',
        danger: 'bg-danger-subtle text-danger-subtle-foreground border-danger-border',
        info: 'bg-info-subtle text-info-subtle-foreground border-info-border',
      },
    },
    defaultVariants: { variant: 'default' },
  },
);

export interface BadgeProps
  extends React.HTMLAttributes<HTMLSpanElement>,
    VariantProps<typeof badgeVariants> {}

const Badge = React.forwardRef<HTMLSpanElement, BadgeProps>(
  ({ className, variant, ...props }, ref) => (
    <span ref={ref} className={cn(badgeVariants({ variant, className }))} {...props} />
  ),
);
Badge.displayName = 'Badge';

export { Badge, badgeVariants };
```

Confirm `tailwind.config.js` maps the `success/warning/danger/info` `-subtle`,
`-subtle-foreground` and `-border` tokens to utility classes. Task 003 added the
CSS variables; if the Tailwind theme does not expose them as
`bg-success-subtle`/`border-success-border`/`text-success-subtle-foreground`, add
them there in this step — the badge test will tell you.

- [ ] **Step 4: Write `table.tsx` and `dialog.tsx`**

`table.tsx` is plain semantic markup with token classes:

```tsx
import * as React from 'react';
import { cn } from '../../lib/utils';

/**
 * Data table primitives.
 *
 * Table wraps itself in an overflow-x container: the admin fleet and audit
 * tables carry more columns than the inspector's right pane is wide, and
 * without this the page body scrolls horizontally instead of the table.
 */
const Table = React.forwardRef<HTMLTableElement, React.HTMLAttributes<HTMLTableElement>>(
  ({ className, ...props }, ref) => (
    <div className="relative w-full overflow-x-auto">
      <table ref={ref} className={cn('w-full caption-bottom text-sm', className)} {...props} />
    </div>
  ),
);
Table.displayName = 'Table';
// … TableHeader (border-b), TableBody, TableFooter, TableRow
//   (border-b border-border hover:bg-muted/50 data-[state=selected]:bg-muted),
//   TableHead (h-10 px-2 text-left align-middle font-medium text-muted-foreground),
//   TableCell (p-2 align-middle), TableCaption (mt-4 text-sm text-muted-foreground),
//   each a forwardRef with a displayName, exported by name.
```

`dialog.tsx` wraps `@radix-ui/react-dialog`, following the standard shadcn shape:
`Dialog`/`DialogTrigger`/`DialogPortal`/`DialogClose` re-exported from the
primitive, plus a styled `DialogOverlay` (`fixed inset-0 z-50 bg-background/80
backdrop-blur-sm`), `DialogContent` (centred, `border border-border bg-background
p-6 shadow-lg`, with a close button using `lucide-react`'s `X`),
`DialogHeader`/`DialogFooter`/`DialogTitle`/`DialogDescription`. Keep every colour
a token.

Add the dependency:

```json
    "@radix-ui/react-dialog": "^1.1.0",
```

then `npm install` from the repo root so the lockfile updates.

- [ ] **Step 5: Run the whole web suite**

Run:
```sh
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
npm install
npm run -w apps/web test
npm run -w apps/web build
```
Expected: PASS — including `conventions.test.ts`'s hardcoded-palette scan, which
now walks three more `.tsx` files.

- [ ] **Step 6: Commit**

```bash
git add apps/web package-lock.json
git commit -m "feat(web): add dialog, table and badge UI primitives"
```

---

### Task 23: `platformAdmin` in auth context, the admin shell, and the route tree

**Files:**
- Modify: `apps/web/src/types/models/user.ts`
- Modify: `apps/web/src/lib/hooks/api/auth.ts`
- Modify: `apps/web/src/context/AuthContext.tsx`
- Modify: `apps/web/src/context/AuthContext.test.tsx`
- Create: `apps/web/src/components/admin/RequirePlatformAdmin.tsx`
- Create: `apps/web/src/components/admin/RequirePlatformAdmin.test.tsx`
- Create: `apps/web/src/components/admin/AdminLayout.tsx`
- Create: `apps/web/src/components/admin/AdminLayout.test.tsx`
- Modify: `apps/web/src/App.tsx`
- Modify: `apps/web/src/components/AppLayout.tsx`
- Modify: `apps/web/src/components/AppLayout.test.tsx`

**Interfaces:**
- Produces:
  - `AuthMeta` gains `platformAdmin: boolean`; `MeResult` gains
    `platformAdmin: boolean`; `AuthContextValue` gains `platformAdmin: boolean`.
  - `<RequirePlatformAdmin>{children}</RequirePlatformAdmin>`
  - `<AdminLayout />` (renders an `<Outlet />`)

**The structural point (design §10.1, risks.md R5).** The `/admin` branch is a
**sibling** of `<RequireAuth><AppLayout/></RequireAuth>` in `App.tsx`, not nested
inside it. `RequireAuth`'s fleetless redirect at `RequireAuth.tsx:29` therefore
never applies, and **`RequireAuth.tsx` is not modified**. An exemption flag is the
kind of thing a later refactor drops silently; a route-tree position is not. The
post-system-purge acceptance test in Task 28 is what catches a regression.

- [ ] **Step 1: Write the failing guard and layout tests**

Create `apps/web/src/components/admin/RequirePlatformAdmin.test.tsx`:

```tsx
import { describe, it, expect, vi } from 'vitest';
import { screen } from '@testing-library/react';
import { Routes, Route } from 'react-router-dom';
import { renderWithProviders } from '../../test/renderWithProviders';
import { RequirePlatformAdmin } from './RequirePlatformAdmin';
import * as authContext from '../../context/AuthContext';

function renderAt(route: string) {
  return renderWithProviders(
    <Routes>
      <Route path="/" element={<div>home</div>} />
      <Route path="/login" element={<div>login</div>} />
      <Route path="/onboarding" element={<div>onboarding</div>} />
      <Route
        path="/admin"
        element={
          <RequirePlatformAdmin>
            <div>console</div>
          </RequirePlatformAdmin>
        }
      />
    </Routes>,
    { route },
  );
}

function mockAuth(value: Partial<authContext.AuthContextValue>) {
  vi.spyOn(authContext, 'useAuth').mockReturnValue({
    user: null,
    activeFleetId: null,
    role: null,
    platformAdmin: false,
    isAuthenticated: false,
    isLoading: false,
    login: vi.fn(),
    logout: vi.fn(),
    ...value,
  } as authContext.AuthContextValue);
}

describe('RequirePlatformAdmin', () => {
  it('sends an anonymous visitor to /login', () => {
    mockAuth({});
    renderAt('/admin');
    expect(screen.getByText('login')).toBeInTheDocument();
  });

  it('sends a signed-in non-admin home, not to a 403 page', () => {
    mockAuth({ isAuthenticated: true, activeFleetId: 'f1', role: 'owner' });
    renderAt('/admin');
    expect(screen.getByText('home')).toBeInTheDocument();
    expect(screen.queryByText('console')).not.toBeInTheDocument();
  });

  it('admits an admin', () => {
    mockAuth({ isAuthenticated: true, platformAdmin: true, activeFleetId: 'f1' });
    renderAt('/admin');
    expect(screen.getByText('console')).toBeInTheDocument();
  });

  // FR-ADMIN-AUTH-9 / risks.md R5. This is the scenario that matters: the admin
  // has just run a system purge, their fleet is gone, and they need to stay in
  // the console to verify the result and cancel within the recovery window.
  it('admits a FLEETLESS admin and does not redirect to onboarding', () => {
    mockAuth({ isAuthenticated: true, platformAdmin: true, activeFleetId: null });
    renderAt('/admin');
    expect(screen.getByText('console')).toBeInTheDocument();
    expect(screen.queryByText('onboarding')).not.toBeInTheDocument();
  });
});
```

Create `apps/web/src/components/admin/AdminLayout.test.tsx`:

```tsx
describe('AdminLayout', () => {
  it('shows the persistent mode band with an explicit exit', () => {
    // FR-ADMIN-UI-3: the band states the caller's scope in plain words and
    // offers a way out, on every screen.
    renderAdminLayout();
    expect(screen.getByText(/platform admin/i)).toBeInTheDocument();
    expect(screen.getByRole('link', { name: /back to my fleet/i })).toBeInTheDocument();
  });

  it('states the stale-claim caveat in plain words', () => {
    // FR-ADMIN-AUTH-7: the console must say that revoking admin does not take
    // effect until the token refreshes. Burying it in a tooltip defeats it.
    renderAdminLayout();
    expect(screen.getByText(/up to 15 minutes/i)).toBeInTheDocument();
  });

  it('links every admin section', () => {
    renderAdminLayout();
    for (const label of ['Overview', 'Fleets', 'Users', 'Purges', 'Audit log']) {
      expect(screen.getByRole('link', { name: label })).toBeInTheDocument();
    }
  });
});
```

And append to `apps/web/src/components/AppLayout.test.tsx`:

```tsx
// FR-ADMIN-UI-5: the nav entry is a convenience, not a control — its absence
// hides the door, the server refuses entry.
it('hides the Admin nav entry from non-admins', () => {
  mockAuth({ isAuthenticated: true, platformAdmin: false });
  renderAppLayout();
  expect(screen.queryByRole('link', { name: 'Admin' })).not.toBeInTheDocument();
});

it('shows the Admin nav entry to admins', () => {
  mockAuth({ isAuthenticated: true, platformAdmin: true });
  renderAppLayout();
  expect(screen.getByRole('link', { name: 'Admin' })).toBeInTheDocument();
});
```

- [ ] **Step 2: Run and watch them fail**

Run: `npm run -w apps/web test -- admin`
Expected: FAIL — modules do not exist.

- [ ] **Step 3: Thread `platformAdmin` through the identity chain**

`types/models/user.ts`:

```ts
// `GET /api/auth/me` meta block.
export interface AuthMeta {
  activeFleetId: string | null;
  role: FleetRole | null;
  // Sourced from the validated token's claim, not a second lookup. It is
  // orthogonal to `role`: role is a position inside one fleet, this is a
  // position above all of them.
  platformAdmin: boolean;
}
```

`lib/hooks/api/auth.ts`:

```ts
export interface MeResult {
  user: User;
  activeFleetId: string | null;
  role: AuthMeta['role'];
  platformAdmin: boolean;
}

async function fetchMe(): Promise<MeResult> {
  const doc = await apiClient.request<JsonApiDocument<User> & { meta?: AuthMeta }>('/api/auth/me');
  return {
    user: doc.data,
    activeFleetId: doc.meta?.activeFleetId ?? null,
    role: doc.meta?.role ?? null,
    // Defaults to false: an older server that does not send the field must not
    // accidentally reveal the console's entry point.
    platformAdmin: doc.meta?.platformAdmin ?? false,
  };
}
```

`context/AuthContext.tsx` — add to the interface and the memo:

```ts
export interface AuthContextValue {
  user: User | null;
  activeFleetId: string | null;
  role: FleetRole | null;
  platformAdmin: boolean;
  isAuthenticated: boolean;
  ...
}
```
```ts
      platformAdmin: data?.platformAdmin ?? false,
```

- [ ] **Step 4: Write the guard**

Create `apps/web/src/components/admin/RequirePlatformAdmin.tsx`:

```tsx
import type { ReactNode } from 'react';
import { Navigate } from 'react-router-dom';
import { useAuth } from '../../context/AuthContext';
import { Skeleton } from '../ui/skeleton';

/**
 * Admin route guard.
 *
 * Requires authentication and `platformAdmin`; sends everyone else to `/`.
 *
 * It deliberately does NOT require an activeFleetId, and — the structural point
 * — the /admin branch is a SIBLING of the RequireAuth/AppLayout branch in
 * App.tsx rather than nested inside it, so RequireAuth's fleetless redirect to
 * /onboarding never applies here. An administrator standing in the wreckage of
 * the system purge they just ran must stay in the console to verify it and, if
 * they were wrong, cancel it (FR-ADMIN-UI-4, FR-ADMIN-UI-14).
 *
 * An exemption flag on the shared guard would have worked today and been
 * dropped silently by a later refactor; a route-tree position is harder to lose.
 *
 * Server-side authz remains authoritative; this is navigation convenience only.
 */
export function RequirePlatformAdmin({ children }: { children: ReactNode }) {
  const { isAuthenticated, isLoading, platformAdmin } = useAuth();

  if (isLoading) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <Skeleton className="h-8 w-48" />
      </div>
    );
  }
  if (!isAuthenticated) {
    return <Navigate to="/login" replace />;
  }
  // Home, not a 403 page: a non-admin has no business knowing what is here, and
  // the server returns 403 to anyone who asks anyway.
  if (!platformAdmin) {
    return <Navigate to="/" replace />;
  }
  return <>{children}</>;
}
```

- [ ] **Step 5: Write the shell**

Create `apps/web/src/components/admin/AdminLayout.tsx`:

```tsx
import { NavLink, Outlet, Link } from 'react-router-dom';
import { useAuth } from '../../context/AuthContext';
import { cn } from '../../lib/utils';
import { BrandMark } from '../BrandMark';
import { ThemeToggle } from '../ThemeToggle';
import { Button } from '../ui/button';

const ADMIN_NAV = [
  { to: '/admin', label: 'Overview', end: true },
  { to: '/admin/fleets', label: 'Fleets' },
  { to: '/admin/users', label: 'Users' },
  { to: '/admin/purges', label: 'Purges' },
  { to: '/admin/audit', label: 'Audit log' },
];

/**
 * The admin shell — deliberately NOT AppLayout.
 *
 * A dedicated shell gives destructive tooling an unmistakable mode boundary,
 * makes fleet browsing the centre of the console rather than a side trip, and
 * resolves the fleetless-admin routing problem structurally (FR-ADMIN-UI-2).
 */
export function AdminLayout() {
  const { user, logout } = useAuth();

  return (
    <div className="flex min-h-screen">
      <aside className="w-56 shrink-0 border-r border-border bg-card p-4">
        <div className="mb-6 flex items-center gap-2 text-lg font-semibold">
          <BrandMark className="h-5 w-5" />
          <span>
            MyFleet <span className="text-muted-foreground">admin</span>
          </span>
        </div>
        <nav className="flex flex-col gap-1">
          {ADMIN_NAV.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.end}
              className={({ isActive }) =>
                cn(
                  'rounded px-3 py-2 text-sm font-medium',
                  isActive
                    ? 'bg-accent text-accent-foreground'
                    : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground',
                )
              }
            >
              {item.label}
            </NavLink>
          ))}
        </nav>
        <div className="mt-6 border-t border-border pt-4">
          <Link
            to="/"
            className="rounded px-3 py-2 text-sm font-medium text-muted-foreground hover:bg-accent hover:text-accent-foreground"
          >
            Back to my fleet
          </Link>
        </div>
      </aside>
      <div className="flex flex-1 flex-col">
        <header className="flex items-center justify-between border-b border-border px-6 py-3">
          <span className="text-sm text-muted-foreground">
            {user?.attributes.displayName ?? ''}
          </span>
          <div className="flex items-center gap-2">
            <ThemeToggle />
            <Button type="button" variant="outline" size="sm" onClick={() => void logout()}>
              Sign out
            </Button>
          </div>
        </header>
        {/*
          The persistent mode band (FR-ADMIN-UI-3). danger-subtle, NOT
          --destructive: that token is reserved for destructive CONTROLS under
          the task-003 contract, and this is a mode indicator, not a button.

          It also states the stale-claim caveat in plain words rather than a
          tooltip. An operator who does not know that revoking admin takes up to
          15 minutes will assume a revocation took effect immediately, which is
          the one misunderstanding with an irreversible consequence.
        */}
        <div className="border-b border-danger-border bg-danger-subtle px-6 py-2 text-sm text-danger-subtle-foreground">
          <strong className="font-semibold">Platform admin.</strong> You can see and delete data
          across every fleet on this platform. Admin access is read from your sign-in token, so
          granting or revoking it takes up to 15 minutes to take effect.
        </div>
        <main className="flex-1 p-6">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
```

- [ ] **Step 6: Add the route branch and the nav entry**

`App.tsx` — a sibling branch, after the authenticated shell:

```tsx
        {/*
          Admin console. A SIBLING of the authenticated shell above, not a child:
          RequireAuth redirects fleetless users to /onboarding, and an admin with
          no fleet — including one who has just run a system purge — must still
          reach every admin screen (FR-ADMIN-UI-4, risks.md R5). Nesting this
          under RequireAuth would reintroduce that redirect; RequireAuth itself
          is deliberately unmodified.
        */}
        <Route
          path="/admin"
          element={
            <RequirePlatformAdmin>
              <AdminLayout />
            </RequirePlatformAdmin>
          }
        >
          <Route index element={<AdminOverviewPage />} />
          <Route path="fleets" element={<AdminFleetsPage />} />
          <Route path="fleets/:id" element={<AdminFleetsPage />} />
          <Route path="users" element={<AdminUsersPage />} />
          <Route path="purges" element={<AdminPurgesPage />} />
          <Route path="audit" element={<AdminAuditPage />} />
        </Route>
```

The page components arrive in Tasks 25–28; stub each as
`export function AdminOverviewPage() { return null; }` in this task so the route
tree compiles, and fill them in as their tasks land.

`AppLayout.tsx` — the nav array becomes a function of the flag:

```tsx
const NAV = [
  { to: '/', label: 'Dashboard', end: true },
  { to: '/vehicles', label: 'Vehicles' },
  { to: '/activity', label: 'Activity' },
  { to: '/notifications', label: 'Notifications' },
  { to: '/settings', label: 'Settings' },
];

export function AppLayout() {
  const { user, logout, platformAdmin } = useAuth();
  // The entry point is a convenience, not a control: its absence hides the
  // door, and the server refuses entry regardless (FR-ADMIN-UI-5).
  const nav = platformAdmin ? [...NAV, { to: '/admin', label: 'Admin' }] : NAV;
```

and map over `nav` instead of `NAV`.

- [ ] **Step 7: Run to green**

Run: `npm run -w apps/web test && npm run -w apps/web build`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add apps/web
git commit -m "feat(web): add the admin shell, its route guard and the sibling route branch"
```

---

### Task 24: admin service, hooks, and the purge-status vocabulary

**Files:**
- Create: `apps/web/src/types/models/admin.ts`
- Create: `apps/web/src/services/api/AdminService.ts`
- Create: `apps/web/src/lib/hooks/api/admin.ts`
- Create: `apps/web/src/lib/admin/purgeStatus.ts`
- Create: `apps/web/src/lib/admin/purgeStatus.test.ts`
- Create: `apps/web/src/lib/hooks/api/admin.test.ts`

**Interfaces:**
- Produces:
  - `adminService` with `stats()`, `listFleets(params)`, `getFleet(id)`,
    `listUsers(params)`, `listPurges(params)`, `getPurge(id)`,
    `createPurge(attrs)`, `cancelPurge(id)`, `retryPurge(id)`,
    `listAuditEvents(params)`
  - `adminKeys` hierarchical factory
  - `useAdminStats`, `useAdminFleets`, `useAdminFleet`, `useAdminUsers`,
    `usePurgeOperations`, `usePurgeOperation`, `useCreatePurge`,
    `useCancelPurge`, `useRetryPurge`, `useAuditEvents`
  - `purgeStatusLabel(status, failedServices)`, `purgeStatusVariant(status)`

**FR-ADMIN-UI-12 lives in one module.** `pending` → "Recoverable", `partial` → a
*specific* failure ("Media not deleted"), `reaped` → "Deleted for good",
`cancelled` → "Restored". Nothing else in the UI reads a raw status string, so
the API and the UI can diverge without drift.

- [ ] **Step 1: Write the failing vocabulary test**

Create `apps/web/src/lib/admin/purgeStatus.test.ts`:

```ts
import { describe, it, expect } from 'vitest';
import { purgeStatusLabel, purgeStatusVariant } from './purgeStatus';

describe('purgeStatusLabel', () => {
  it('speaks in outcomes, not API vocabulary', () => {
    expect(purgeStatusLabel('pending', [])).toBe('Recoverable');
    expect(purgeStatusLabel('reaped', [])).toBe('Deleted for good');
    expect(purgeStatusLabel('cancelled', [])).toBe('Restored');
  });

  // "Partial" tells an operator nothing actionable. Naming the service does.
  it('names what actually failed for a partial operation', () => {
    expect(purgeStatusLabel('partial', ['media'])).toBe('Media not deleted');
    expect(purgeStatusLabel('partial', ['notification'])).toBe('Notifications not deleted');
    expect(purgeStatusLabel('partial', ['media', 'notification'])).toBe(
      'Media and notifications not deleted',
    );
  });

  it('degrades safely on an unknown status rather than rendering a blank chip', () => {
    expect(purgeStatusLabel('something-new' as never, [])).toBe('Unknown');
  });
});

describe('purgeStatusVariant', () => {
  it('maps each status to a badge variant', () => {
    expect(purgeStatusVariant('pending')).toBe('info');
    expect(purgeStatusVariant('partial')).toBe('warning');
    expect(purgeStatusVariant('reaped')).toBe('danger');
    expect(purgeStatusVariant('cancelled')).toBe('success');
  });
});
```

- [ ] **Step 2: Run and watch it fail**

Run: `npm run -w apps/web test -- purgeStatus`
Expected: FAIL — module not found.

- [ ] **Step 3: Write the vocabulary module**

```ts
import type { BadgeProps } from '../../components/ui/badge';
import type { PurgeStatus } from '../../types/models/admin';

/**
 * THE mapping from API vocabulary to user language (FR-ADMIN-UI-12).
 *
 * Nothing else in the console reads a raw status string. Two reasons: the words
 * an operator needs ("Recoverable", "Deleted for good") are not the words the
 * state machine uses, and keeping the translation in one place lets the API and
 * the UI diverge later without drift.
 */

const SERVICE_LABELS: Record<string, string> = {
  media: 'Media',
  notification: 'Notifications',
};

/**
 * A human sentence for a purge's state.
 *
 * `partial` is special: the word tells an operator nothing actionable, so the
 * label names the service that did not finish. That is the difference between
 * "something went wrong" and "press retry, it is media-service".
 */
export function purgeStatusLabel(status: PurgeStatus, failedServices: string[]): string {
  switch (status) {
    case 'pending':
      return 'Recoverable';
    case 'reaped':
      return 'Deleted for good';
    case 'cancelled':
      return 'Restored';
    case 'partial': {
      const names = failedServices.map((s) => SERVICE_LABELS[s] ?? s);
      if (names.length === 0) return 'Partly deleted';
      if (names.length === 1) return `${names[0]} not deleted`;
      const last = names[names.length - 1].toLowerCase();
      return `${names.slice(0, -1).join(', ')} and ${last} not deleted`;
    }
    default:
      // A status this build does not know about must still render something a
      // human can read, rather than an empty chip.
      return 'Unknown';
  }
}

/** Badge variant for a purge status. */
export function purgeStatusVariant(status: PurgeStatus): NonNullable<BadgeProps['variant']> {
  switch (status) {
    case 'pending':
      return 'info';
    case 'partial':
      return 'warning';
    case 'reaped':
      return 'danger';
    case 'cancelled':
      return 'success';
    default:
      return 'secondary';
  }
}
```

- [ ] **Step 4: Write the types and the service**

`types/models/admin.ts` mirrors the Go transforms — one interface per resource,
`snake_case` attribute names matching PRD §5:

```ts
export type PurgeStatus = 'pending' | 'partial' | 'cancelled' | 'reaped';
export type PurgeScope = 'system' | 'fleet' | 'record';
export type DeletedFilter = 'include' | 'exclude' | 'only';

export interface AdminStatsAttributes {
  fleets: number | null;
  vehicles: { active: number; pending_purge: number };
  memberships: number | null;
  pending_invites: number | null;
  maintenance_records: number | null;
  maintenance_schedules: number | null;
  fuel_logs: number | null;
  mileage_records: number | null;
  activity_events: number | null;
  users: number | null;
  media_objects: number | null;
  notifications: number | null;
  warnings: string[];
}

export interface AdminFleetAttributes {
  name: string;
  created_at: string;
  owner_user_id: string;
  owner_email: string;
  owner_display_name: string;
  member_count: number;
  vehicle_count: number;
  /** Admin-stamped only. A fleet a user deleted is not recoverable here. */
  pending_purge: boolean;
  /** ISO deadline; null unless pending_purge. Drives the countdown chip. */
  purge_after: string | null;
}

export interface AdminMemberRow {
  user_id: string;
  /** Empty when auth-service could not be reached — see `warnings`. */
  email: string;
  display_name: string;
  role: 'owner' | 'member' | 'viewer';
  joined_at: string;
}

export interface AdminVehicleRow {
  id: string;
  nickname: string;
  make: string;
  model: string;
  year: number;
  current_mileage: number;
  /** Derived server-side via vehicle.StatusDeps; detail view only. */
  status: string;
  pending_purge: boolean;
}

export interface AdminInviteRow {
  id: string;
  email: string;
  role: 'member' | 'viewer';
  expires_at: string;
}

export interface AdminFleetDetailAttributes extends AdminFleetAttributes {
  members: AdminMemberRow[];
  vehicles: AdminVehicleRow[];
  pending_invites: AdminInviteRow[];
  /** Same numbers the purge will report — one Count, one predicate. */
  counts: Record<string, number>;
  warnings: string[];
}

export interface PurgeOperationAttributes {
  scope: PurgeScope;
  target_type: string | null;
  target_id: string | null;
  /** Denormalised at request time, so the log reads after the target is gone. */
  target_label: string;
  status: PurgeStatus;
  requested_by_user_id: string;
  requested_by_email: string;
  requested_at: string;
  purge_after: string;
  reaped_at: string | null;
  cancelled_at: string | null;
  affected: Record<string, number>;
  failed_services: string[];
}

export interface AuditEventAttributes {
  actor_user_id: string;
  actor_email: string;
  action: 'purge.created' | 'purge.cancelled' | 'purge.retried' | 'purge.reaped';
  scope: string;
  target_type: string | null;
  target_id: string | null;
  target_label: string;
  purge_operation_id: string | null;
  affected: Record<string, number>;
  correlation_id: string;
  created_at: string;
}

export interface AdminUserAttributes {
  email: string;
  display_name: string;
  created_at: string;
  last_login_at: string | null;
  platform_admin: boolean;
  fleets: Array<{ id: string; name: string; role: string }>;
}

export interface CreatePurgeInput {
  scope: PurgeScope;
  target_type?: string;
  target_id?: string;
  confirmation?: string;
}
```

`AdminService.ts` extends `BaseService` with base path `/api/fleet/admin`, adding
one method per endpoint and building query strings with `page[number]` /
`page[size]` — **not** `page` / `size`:

```ts
/**
 * AdminService — the platform admin console's API surface.
 *
 * Backend routes (apps/fleet-service/internal/admin/resource.go,
 * gateway-prefixed). Every one returns 403 when the caller's platform_admin
 * claim is false; the hidden nav entry is cosmetic and the server is
 * authoritative.
 *
 * Pagination is page[number]/page[size] — the platform's actual convention
 * (packages/shared-go/server/pagination.go), not the page/size the PRD sketched.
 */
```

- [ ] **Step 5: Write the hooks**

`lib/hooks/api/admin.ts` with a hierarchical key factory shaped like `memberKeys`:

```ts
export const adminKeys = {
  all: ['admin'] as const,
  stats: () => [...adminKeys.all, 'stats'] as const,
  fleets: () => [...adminKeys.all, 'fleets'] as const,
  fleetList: (params: { q: string; deleted: DeletedFilter; page: number }) =>
    [...adminKeys.fleets(), 'list', params] as const,
  fleet: (id: string) => [...adminKeys.fleets(), 'detail', id] as const,
  users: (params: { page: number }) => [...adminKeys.all, 'users', params] as const,
  purges: () => [...adminKeys.all, 'purges'] as const,
  purgeList: (params: { status: string; page: number }) =>
    [...adminKeys.purges(), 'list', params] as const,
  purge: (id: string) => [...adminKeys.purges(), 'detail', id] as const,
  audit: (params: { action: string; actor: string; page: number }) =>
    [...adminKeys.all, 'audit', params] as const,
};
```

Every mutation invalidates on **settle** (not success) and surfaces
`createErrorFromUnknown(err).message` in a `toast.error`, matching the pattern
established on `fix/invite-accept-flow`:

```ts
/**
 * POST /api/fleet/admin/purge-operations.
 *
 * Invalidates broadly on settle: a purge changes fleets, stats, the purge queue
 * and the audit log at once, and a stale count next to a destructive control is
 * worse than a redundant refetch.
 */
export function useCreatePurge() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (attrs: CreatePurgeInput) => adminService.createPurge(attrs),
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: adminKeys.all });
    },
    onError: (err) => {
      const apiError = createErrorFromUnknown(err);
      if (apiError.status === 409) {
        toast.error('That confirmation did not match. Nothing was deleted.');
      } else if (apiError.status === 403) {
        toast.error('Your platform-admin access has been revoked. Nothing was deleted.');
      } else {
        toast.error(apiError.message || 'Could not start the purge');
      }
    },
  });
}
```

`useCancelPurge` maps 409 to "This purge has already been completed and cannot be
undone."; `useRetryPurge` is presented as safe to repeat and needs no special
error case.

- [ ] **Step 6: Write `admin.test.ts`**

Cover the two behaviours that would silently break: the query keys are stable
across renders, and `useCreatePurge` invalidates `adminKeys.all` on settle even
when the request fails (a failed create may still have stamped locally and
marked the operation partial — the queue must refetch).

- [ ] **Step 7: Run to green**

Run: `npm run -w apps/web test && npm run -w apps/web build`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add apps/web
git commit -m "feat(web): add the admin API service, hooks and purge-status vocabulary"
```

---

### Task 25: the overview screen

**Files:**
- Create: `apps/web/src/pages/admin/AdminOverviewPage.tsx` (replace the stub)
- Create: `apps/web/src/pages/admin/AdminOverviewPage.test.tsx`

**Interfaces:**
- Consumes: `useAdminStats` from Task 24.
- Produces: the `/admin` index screen.

**The one rule that is easy to get wrong (FR-ADMIN-UI-6).** A `null` count renders
as an **em dash with the reason beneath it**, never as `0`. Zero says "there is no
data"; the em dash says "we could not ask". Those are different facts and an
operator about to purge needs to tell them apart.

- [ ] **Step 1: Write the failing test**

```tsx
describe('AdminOverviewPage', () => {
  it('renders a stat tile per domain', async () => {
    mockStats({ fleets: 12, users: 21, vehicles: { active: 47, pending_purge: 3 } });
    renderWithProviders(<AdminOverviewPage />);
    expect(await screen.findByText('12')).toBeInTheDocument();
    expect(screen.getByText('47')).toBeInTheDocument();
  });

  it('shows the recovery window in the vehicle tile', async () => {
    // FR-ADMIN-STATS-3: pending-purge is what the console can still undo, so it
    // belongs next to the number it will become.
    mockStats({ vehicles: { active: 47, pending_purge: 3 } });
    renderWithProviders(<AdminOverviewPage />);
    expect(await screen.findByText(/3 pending purge/i)).toBeInTheDocument();
  });

  // FR-ADMIN-UI-6. Rendering 0 here would tell an operator there are no
  // notifications, when the truth is that nobody could ask.
  it('renders an unavailable count as an em dash with the reason, never as 0', async () => {
    mockStats({
      notifications: null,
      warnings: ['notification-service unreachable; notifications count omitted'],
    });
    renderWithProviders(<AdminOverviewPage />);
    const tile = await screen.findByTestId('stat-notifications');
    expect(tile).toHaveTextContent('—');
    expect(tile).not.toHaveTextContent('0');
    expect(screen.getByText(/notification-service unreachable/i)).toBeInTheDocument();
  });

  it('renders warnings as a non-blocking banner, not an error state', async () => {
    mockStats({ notifications: null, warnings: ['notification-service unreachable'] });
    renderWithProviders(<AdminOverviewPage />);
    expect(await screen.findByRole('status')).toHaveTextContent(/unreachable/i);
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run it, watch it fail, then build the page**

Twelve `Card`-based tiles in a responsive grid (`grid gap-4 sm:grid-cols-2
lg:grid-cols-4`). Each tile: label in `text-sm text-muted-foreground`, value in
`text-3xl font-semibold`, `data-testid={`stat-${key}`}`. A `null` value renders
`—` plus the matching warning in `text-xs text-muted-foreground`. The warnings
banner uses `role="status"` and the `warning-subtle` token trio.

- [ ] **Step 3: Run to green and commit**

```bash
npm run -w apps/web test && npm run -w apps/web build
git add apps/web && git commit -m "feat(web): add the admin overview screen"
```

---

### Task 26: the fleet inspector and the blast-radius panel

**Files:**
- Create: `apps/web/src/pages/admin/AdminFleetsPage.tsx` (replace the stub)
- Create: `apps/web/src/pages/admin/AdminFleetsPage.test.tsx`
- Create: `apps/web/src/components/admin/BlastRadiusPanel.tsx`
- Create: `apps/web/src/components/admin/BlastRadiusPanel.test.tsx`

**Interfaces:**
- Consumes: `useAdminFleets`, `useAdminFleet`.
- Produces: the two-pane inspector and the blast-radius panel that gates the
  purge control.

**Three requirements with teeth:**

- **FR-ADMIN-UI-7** — two panes above `md`, single column with back-navigation
  below. Fleets pending purge render **struck through with a countdown chip**
  rather than vanishing; they are in the default result set because `?deleted=`
  defaults to `include`.
- **FR-ADMIN-UI-8** — the owner's remove action is **permanently inert**. A fleet
  must never lose its only owner.
- **FR-ADMIN-UI-9** — if the counts cannot be computed, the panel shows an error
  and the purge control is **unavailable**. Never a live destructive button above
  stale or approximate numbers.

- [ ] **Step 1: Write the failing tests**

```tsx
describe('AdminFleetsPage', () => {
  it('shows list and detail side by side above md, and a single column below', () => {
    // The two panes are one grid whose columns collapse; assert the classes
    // rather than the viewport, which jsdom does not model.
    const { container } = renderWithProviders(<AdminFleetsPage />, { route: '/admin/fleets' });
    expect(container.querySelector('[data-testid="fleet-inspector"]')).toHaveClass('md:grid-cols-[320px_1fr]');
  });

  it('shows a pending-purge fleet struck through with a countdown', async () => {
    mockFleets([{ id: 'f1', name: 'Test Fleet', pending_purge: true, purge_after: futureIso }]);
    renderWithProviders(<AdminFleetsPage />, { route: '/admin/fleets' });
    const row = await screen.findByText('Test Fleet');
    expect(row).toHaveClass('line-through');
    expect(screen.getByText(/\d+ days? left/i)).toBeInTheDocument();
  });

  it('offers back-navigation from the detail view on small screens', async () => {
    renderWithProviders(<AdminFleetsPage />, { route: '/admin/fleets/f1' });
    expect(await screen.findByRole('link', { name: /all fleets/i })).toBeInTheDocument();
  });

  // FR-ADMIN-UI-8: a fleet must never lose its only owner, and the console must
  // not offer an action it will refuse.
  it('renders the owner row without an enabled remove action', async () => {
    mockFleet({ members: [{ user_id: 'u1', role: 'owner' }, { user_id: 'u2', role: 'member' }] });
    renderWithProviders(<AdminFleetsPage />, { route: '/admin/fleets/f1' });
    const ownerRow = await screen.findByTestId('member-u1');
    expect(within(ownerRow).queryByRole('button', { name: /remove/i })).toBeDisabled();
    const memberRow = screen.getByTestId('member-u2');
    expect(within(memberRow).getByRole('button', { name: /remove/i })).toBeEnabled();
  });
});

describe('BlastRadiusPanel', () => {
  it('lists what a purge would delete, per domain', () => {
    render(<BlastRadiusPanel counts={{ vehicles: 4, fuel_logs: 130 }} fleetName="Test" onPurge={vi.fn()} />);
    expect(screen.getByText('130')).toBeInTheDocument();
  });

  // FR-ADMIN-UI-9. A live destructive button above numbers nobody could compute
  // is the worst state this screen can be in.
  it('withholds the purge control when the counts could not be computed', () => {
    render(<BlastRadiusPanel counts={undefined} error fleetName="Test" onPurge={vi.fn()} />);
    expect(screen.getByRole('alert')).toHaveTextContent(/could not/i);
    expect(screen.queryByRole('button', { name: /purge this fleet/i })).not.toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Build the screens**

The inspector is one grid: `data-testid="fleet-inspector"` with
`grid gap-6 md:grid-cols-[320px_1fr]`. Below `md` the detail pane is shown only
when `:id` is present and the list is hidden, giving separate views with a
"← All fleets" link. Above `md` both render.

Detail sections, each a `Card` with a `Table`: members (role `Badge`, remove
action per row, owner's disabled), vehicles (derived status `Badge`, record
counts), pending invites, per-domain counts. Then `BlastRadiusPanel` last, with
"Purge this fleet" beneath the numbers.

The countdown reads from `purge_after`; render whole days remaining and the
**absolute** deadline as a `title` attribute so hovering gives the exact time.

- [ ] **Step 3: Run to green and commit**

```bash
npm run -w apps/web test && npm run -w apps/web build
git add apps/web && git commit -m "feat(web): add the admin fleet inspector and blast-radius panel"
```

---

### Task 27: the confirmation dialog and the purge flow

**Files:**
- Create: `apps/web/src/components/admin/PurgeConfirmDialog.tsx`
- Create: `apps/web/src/components/admin/PurgeConfirmDialog.test.tsx`
- Modify: `apps/web/src/pages/admin/AdminFleetsPage.tsx`
- Modify: `apps/web/src/pages/admin/AdminOverviewPage.tsx` (the system-purge entry point)

**Interfaces:**
- Produces:
  ```tsx
  interface PurgeConfirmDialogProps {
    open: boolean;
    onOpenChange: (open: boolean) => void;
    scope: 'fleet' | 'system';
    /** The exact phrase the operator must type: the fleet name, or PURGE EVERYTHING. */
    confirmationPhrase: string;
    counts: Record<string, number>;
    /** People affected — stated separately because rows are not the point. */
    peopleCount: number;
    recoveryDeadline: string; // ISO
    onConfirm: () => void;
    isPending: boolean;
  }
  ```

**FR-ADMIN-UI-10, in full:** the confirm control stays unavailable until the typed
value matches **exactly**; the dialog states what will be deleted in terms of
**people as well as rows**; the recovery deadline is an **absolute date and
time**, not a duration; and a system purge additionally names what **survives** —
user accounts, sign-ins, and seeded maintenance categories. The disabled control
is a courtesy; the server-side 409 is the real control.

- [ ] **Step 1: Write the failing test**

```tsx
describe('PurgeConfirmDialog', () => {
  it('keeps confirm unavailable until the phrase matches exactly', async () => {
    const user = userEvent.setup();
    render(<PurgeConfirmDialog {...props} confirmationPhrase="The Tumidanski Fleet" />);
    const confirm = screen.getByRole('button', { name: /purge/i });
    expect(confirm).toBeDisabled();

    await user.type(screen.getByLabelText(/type the fleet name/i), 'the tumidanski fleet');
    expect(confirm).toBeDisabled();

    await user.clear(screen.getByLabelText(/type the fleet name/i));
    await user.type(screen.getByLabelText(/type the fleet name/i), 'The Tumidanski Fleet');
    expect(confirm).toBeEnabled();
  });

  it('states the blast radius in people as well as rows', () => {
    render(<PurgeConfirmDialog {...props} peopleCount={3} counts={{ vehicles: 4 }} />);
    expect(screen.getByText(/3 people/i)).toBeInTheDocument();
    expect(screen.getByText(/4/)).toBeInTheDocument();
  });

  // A duration ("recoverable for 5 days") makes an operator do arithmetic under
  // pressure and get it wrong. An absolute deadline does not.
  it('gives the recovery deadline as an absolute date and time', () => {
    render(<PurgeConfirmDialog {...props} recoveryDeadline="2026-08-07T14:03:11Z" />);
    expect(screen.getByText(/august 7, 2026/i)).toBeInTheDocument();
    expect(screen.queryByText(/5 days/i)).not.toBeInTheDocument();
  });

  it('names what survives a system purge', () => {
    render(<PurgeConfirmDialog {...props} scope="system" confirmationPhrase="PURGE EVERYTHING" />);
    expect(screen.getByText(/user accounts/i)).toBeInTheDocument();
    expect(screen.getByText(/sign-ins/i)).toBeInTheDocument();
    expect(screen.getByText(/maintenance categories/i)).toBeInTheDocument();
  });

  it('does not name survivors for a fleet purge', () => {
    render(<PurgeConfirmDialog {...props} scope="fleet" />);
    expect(screen.queryByText(/what survives/i)).not.toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Build the dialog**

Radix `Dialog` from Task 22. The typed value is local state compared with `===`
against `confirmationPhrase` — **no trimming, no case folding**, mirroring the
server. Confirm is `<Button variant="destructive" disabled={typed !== phrase || isPending}>`.
Format the deadline with `Intl.DateTimeFormat(undefined, { dateStyle: 'long',
timeStyle: 'short' })`. The survivors block renders only for `scope === 'system'`.

Wire it into the fleet detail (fleet scope, phrase = fleet name) and the overview
(system scope, phrase = `PURGE EVERYTHING`), both calling `useCreatePurge`.

- [ ] **Step 3: Handle the post-system-purge state (FR-ADMIN-UI-14)**

On a successful system purge the client clears the React Query cache and refetches
`/auth/me`. The admin **stays in the console** — which works only because of the
sibling route branch from Task 23:

```tsx
  const createPurge = useCreatePurge();

  async function confirmSystemPurge() {
    await createPurge.mutateAsync({ scope: 'system', confirmation: SYSTEM_CONFIRMATION });
    // Everything the cache holds is now soft-deleted. Clearing rather than
    // invalidating avoids rendering a frame of stale fleet data between the
    // purge and the refetch.
    queryClient.clear();
    // The admin's own fleet is gone, so /auth/me now returns a null
    // activeFleetId. They stay HERE — RequirePlatformAdmin does not require a
    // fleet, and the /admin branch is not nested under RequireAuth's fleetless
    // redirect (FR-ADMIN-UI-14, risks.md R5).
    await queryClient.refetchQueries({ queryKey: authKeys.me() });
  }
```

- [ ] **Step 4: Run to green and commit**

```bash
npm run -w apps/web test && npm run -w apps/web build
git add apps/web && git commit -m "feat(web): add the purge confirmation dialog and purge flow"
```

---

### Task 28: purges, audit, users, and the post-purge acceptance test

**Files:**
- Create: `apps/web/src/pages/admin/AdminPurgesPage.tsx` (replace the stub)
- Create: `apps/web/src/pages/admin/AdminPurgesPage.test.tsx`
- Create: `apps/web/src/pages/admin/AdminAuditPage.tsx` (replace the stub)
- Create: `apps/web/src/pages/admin/AdminAuditPage.test.tsx`
- Create: `apps/web/src/pages/admin/AdminUsersPage.tsx` (replace the stub)
- Create: `apps/web/src/pages/admin/AdminUsersPage.test.tsx`
- Create: `apps/web/src/components/admin/postPurgeRouting.test.tsx`

**Interfaces:** consumes `usePurgeOperations`, `useCancelPurge`, `useRetryPurge`,
`useAuditEvents`, `useAdminUsers`, `purgeStatusLabel`, `purgeStatusVariant`.

- [ ] **Step 1: Write the failing tests**

```tsx
describe('AdminPurgesPage', () => {
  it('renders status in user language, never the API vocabulary', async () => {
    mockPurges([
      { id: 'op-1', status: 'pending', failed_services: [] },
      { id: 'op-2', status: 'partial', failed_services: ['media'] },
    ]);
    renderWithProviders(<AdminPurgesPage />);
    expect(await screen.findByText('Recoverable')).toBeInTheDocument();
    expect(screen.getByText('Media not deleted')).toBeInTheDocument();
    expect(screen.queryByText('pending')).not.toBeInTheDocument();
    expect(screen.queryByText('partial')).not.toBeInTheDocument();
  });

  it('shows a countdown to permanence for recoverable operations', async () => {
    mockPurges([{ id: 'op-1', status: 'pending', purge_after: futureIso }]);
    renderWithProviders(<AdminPurgesPage />);
    expect(await screen.findByText(/left to restore/i)).toBeInTheDocument();
  });

  it('offers restore only while the operation is recoverable', async () => {
    mockPurges([
      { id: 'op-1', status: 'pending' },
      { id: 'op-2', status: 'reaped' },
    ]);
    renderWithProviders(<AdminPurgesPage />);
    expect(within(await screen.findByTestId('purge-op-1')).getByRole('button', { name: /restore/i }))
      .toBeEnabled();
    expect(within(screen.getByTestId('purge-op-2')).queryByRole('button', { name: /restore/i }))
      .not.toBeInTheDocument();
  });

  // FR-ADMIN-UI-11: retry must READ as safe to repeat, or an operator will not
  // press it after the first failure.
  it('presents retry as safe to repeat', async () => {
    mockPurges([{ id: 'op-1', status: 'partial', failed_services: ['media'] }]);
    renderWithProviders(<AdminPurgesPage />);
    const retry = await screen.findByRole('button', { name: /retry/i });
    expect(retry).toHaveAccessibleDescription(/safe to run again/i);
  });
});

describe('AdminAuditPage', () => {
  it('renders newest first and attributes reaper rows to "system"', async () => {
    mockAudit([
      { id: 'a1', action: 'purge.reaped', actor_user_id: 'system', actor_email: 'system', created_at: newer },
      { id: 'a2', action: 'purge.created', actor_email: 'admin@example.com', created_at: older },
    ]);
    renderWithProviders(<AdminAuditPage />);
    const rows = await screen.findAllByRole('row');
    expect(within(rows[1]).getByText('system')).toBeInTheDocument();
    expect(within(rows[2]).getByText('admin@example.com')).toBeInTheDocument();
  });

  it('surfaces the correlation id so a row can be tied back to service logs', async () => {
    mockAudit([{ id: 'a1', correlation_id: 'corr-123' }]);
    renderWithProviders(<AdminAuditPage />);
    expect(await screen.findByText('corr-123')).toBeInTheDocument();
  });
});
```

Create `apps/web/src/components/admin/postPurgeRouting.test.tsx` — **the R5
residual-risk test**, and the reason it is its own file:

```tsx
/**
 * risks.md R5's residual risk: a future refactor renesting /admin under
 * RequireAuth. That would compile, pass every other test, and only fail in
 * production at the exact moment recovery matters — an admin who has just run a
 * system purge would be bounced to /onboarding with a five-day window ticking.
 *
 * This test exercises the POST-SYSTEM-PURGE state specifically, not merely a
 * fleetless account, because those are different bugs with the same symptom and
 * only one of them is catastrophic.
 */
describe('after a system purge', () => {
  it('keeps a now-fleetless admin inside the console', async () => {
    // /auth/me now answers with a null activeFleetId — the admin's own fleet
    // was in the blast radius.
    mockMe({ activeFleetId: null, platformAdmin: true });

    renderWithProviders(<App />, { route: '/admin/purges' });

    expect(await screen.findByText(/platform admin/i)).toBeInTheDocument();
    expect(screen.queryByText(/onboarding/i)).not.toBeInTheDocument();
  });

  it('lets them reach every admin screen without a fleet', async () => {
    mockMe({ activeFleetId: null, platformAdmin: true });
    for (const route of ['/admin', '/admin/fleets', '/admin/users', '/admin/purges', '/admin/audit']) {
      const { unmount } = renderWithProviders(<App />, { route });
      expect(await screen.findByText(/platform admin/i)).toBeInTheDocument();
      unmount();
    }
  });
});
```

- [ ] **Step 2: Build the three screens**

**Purges** — `Table` of operations, status `Badge` from `purgeStatusVariant`,
label from `purgeStatusLabel`, a status filter (`Select`), a countdown to
`purge_after` with the absolute deadline as `title`, restore for
`pending`/`partial`, retry for `partial`. Give retry an
`aria-describedby` pointing at helper text: "Safe to run again — it only
re-attempts the parts that did not finish."

**Audit** — `Table`, newest first, with `?action=` and `?actor=` filters. Render
`actor_email` except when `actor_user_id === 'system'`, where it renders `system`
in `text-muted-foreground`. Correlation id in a `font-mono text-xs` cell.

**Users** — `Table` of id, email, display name, created, last login, a
platform-admin `Badge`, and the fleets they belong to. Read-only: granting admin
is a deliberate out-of-band act and the console displays it without offering it
(PRD non-goal).

- [ ] **Step 3: Run to green and commit**

```bash
npm run -w apps/web test && npm run -w apps/web build
git add apps/web && git commit -m "feat(web): add the purges, audit and users screens"
```

---

# Phase 6 — Verification (Task 29)

---

### Task 29: the full gate

**Files:** none created; this task verifies and fixes.

- [ ] **Step 1: Run the Go suite with the race detector**

```sh
go build github.com/jtumidanski/myfleet/...
go vet github.com/jtumidanski/myfleet/...
go test -race github.com/jtumidanski/myfleet/...
```
Expected: PASS. `/admin/stats` fans out with a `sync.WaitGroup` writing into a
pre-sized slice, so `-race` is the check that matters there.

- [ ] **Step 2: Confirm the arch tests actually ran**

```sh
go test ./apps/fleet-service/internal/admin/ -run 'TestManifestCoversEveryTable|TestAdminTreeIsSeparate|TestManifestKeysAreUnique' -v
go test ./apps/media-service/internal/admin/ -run TestManifestCoversEveryTable -v
go test ./apps/notification-service/internal/admin/ -run TestManifestCoversEveryTable -v
go test ./apps/auth-service/internal/arch/ -v
go test ./apps/auth-service/internal/session/ -run TestMintAccess -v
```
Expected: every one PASS and **reported as run** — an arch test that silently
skips because its walk root moved is worse than no arch test. `TestManifestCoversEveryTable`
fails loudly on an empty walk for exactly this reason; confirm the `-v` output
shows it executing.

- [ ] **Step 3: Run the web suite and the build**

```sh
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
npm run -w apps/web test
npm run -w packages/shared-ts test
npm run -w apps/web build
```
Expected: PASS, including `conventions.test.ts`'s hardcoded-palette scan across
every new `.tsx`.

- [ ] **Step 4: Render and dry-run both overlays**

```sh
kustomize build deploy/k8s/overlays/local > /tmp/claude-1000/local.yaml
kustomize build deploy/k8s/overlays/main  > /tmp/claude-1000/main.yaml
kubectl apply --dry-run=server -f /tmp/claude-1000/local.yaml
kubectl apply --dry-run=server -f /tmp/claude-1000/main.yaml
```

**Both**, not just `main`. CLAUDE.md records a missing `namespace:` in
`deploy/k8s/infra-local/kustomization.yaml` that broke `kubectl apply -k` outright
and slipped through ten reviews because only the `main` dry-run was ever run.

- [ ] **Step 5: Assert the three deny rules are present on both entrypoints**

```sh
for svc in fleet media auth notifications; do
  n=$(grep -c "api/+$svc\[\^/\]\*/\*internal" /tmp/claude-1000/main.yaml)
  echo "$svc: $n"
done
```
Expected: `2` for every one — each rule appears in `myfleet-routes` and, via the
`replacements` block, in the TLS twin. **A `1` is a security bug**: the rule is
present on one port and missing on the other, and notification-service's
unauthenticated purge endpoint is reachable on the port that lost it.

Confirm the public admin path is not caught by the fleet deny rule:

```sh
grep -A3 'api/+fleet\[\^/\]\*/\*internal' /tmp/claude-1000/main.yaml
```
The regex requires the literal `internal`; `/api/fleet/admin/stats` does not
contain it and falls through to the priority-100 router.

- [ ] **Step 6: Assert the `main` overlay is still clean**

```sh
make manifests
grep -c 'kind: PersistentVolumeClaim' /tmp/claude-1000/main.yaml   # expect 0
grep -c 'kind: Secret'                /tmp/claude-1000/main.yaml   # expect 0
grep -c 'kind: ClusterRole'           /tmp/claude-1000/main.yaml   # expect 0
grep -nE 'REPLACE|CHANGEME|TODO|xxx'  /tmp/claude-1000/main.yaml   # expect no output
grep -c 'ADMIN_PURGE_RECOVERY_WINDOW\|PLATFORM_ADMIN_BOOTSTRAP_EMAILS\|AUTH_INTERNAL_URL\|NOTIFICATION_INTERNAL_URL' /tmp/claude-1000/main.yaml
```
This task adds no Secrets, no ClusterRole and no PVCs, so the first three must
stay at zero.

- [ ] **Step 7: Run the whole gate**

```sh
make ci
```
Expected: PASS — `lint-check vet test build fe-test fe-build manifests
carfax-template`.

- [ ] **Step 8: Walk the PRD's acceptance criteria**

Open `prd.md` §10 and tick each box against a test name or a manual check. The
ones no automated test covers, which must be exercised by hand against a running
stack:

- Stopping `notification-service` still yields 200 from `/admin/stats` with a
  populated `warnings` array and a null notifications count.
- A system purge leaves `auth.users`, `auth.refresh_tokens` and
  `fleet.maintenance_categories` intact, the admin stays logged in, and the
  console remains reachable.
- Every admin surface is legible in **both** themes, with the destructive
  surfaces verified on the dark ground where `--danger` sits in the 400 band.

Record the outcome of each manual check in the commit message or the PR body —
"verified by hand" with nothing behind it is how the local-overlay defect
survived ten reviews.

- [ ] **Step 9: Code review before the PR**

Per CLAUDE.md, run `superpowers:requesting-code-review` (or `/audit-plan`) before
opening a PR. It dispatches `plan-adherence-reviewer`,
`backend-guidelines-reviewer` and `frontend-guidelines-reviewer` in parallel and
writes findings to `docs/tasks/task-011-platform-admin-console/audit.md`. Do not
skip it because the plan looks complete.

- [ ] **Step 10: Commit any fixes**

```bash
git add -A
git commit -m "chore(task-011): address verification findings"
```

---

## Appendix — requirement coverage

Every `FR-ADMIN-*` in `prd.md` §4, mapped to the task that implements it.

| Requirement | Task |
|---|---|
| AUTH-1 `auth.platform_admins` + idempotent seed | 9 |
| AUTH-2 provision-time seed | 9 |
| AUTH-3 bootstrap list seeds only | 9 |
| AUTH-4 claim on both mint paths | 10 |
| AUTH-5 `Identity.PlatformAdmin`, absent → false | 10 |
| AUTH-6 `RequirePlatformAdmin` → 403 | 11 |
| AUTH-7 stale-claim re-verification on create and retry | 17, 18 (cancel exempt, design §5.4) |
| AUTH-8 `/auth/me` exposes it | 10 |
| AUTH-9 no admin endpoint requires a fleet | 11, 20, 23 |
| API-1 own route group, no `RequireSameFleet` | 11 (arch test), 21 (wiring) |
| API-2 fleet-service orchestrates over HTTP | 15 |
| API-3 internal route trees | 12, 13, 14 |
| API-4 JSON:API | 20 |
| STATS-1…5 | 20 |
| FLEET-1…6 | 20 |
| PURGE-1…8 | 16, 17 |
| PURGE-9, 10 partial + idempotent retry | 17, 18 |
| PURGE-11 recovery window | 16 |
| PURGE-12, 13 list and get | 20 |
| RESTORE-1…3 cancel and fidelity | 7, 18 |
| RESTORE-4 reaper (hourly, design OQ-5) | 19 |
| RESTORE-5 MinIO removal | 13 |
| RESTORE-6 resumable | 19 |
| RESTORE-7 both legacy sweeps narrowed | 6, 8 |
| DATA-1, 2 visibility sweep | 4, 5, 6 |
| DATA-3 per-domain regression tests | 4, 5, 6 |
| DATA-4 partial unique indexes (+ two more, design F1) | 1, 2 |
| DATA-5 no events from a purge | 17 (the purge path emits nothing but the audit row) |
| AUDIT-1…4 | 16, 17, 18, 19, 28 |
| UI-1…5 | 23 |
| UI-6 | 25 |
| UI-7, 8, 9 | 26 |
| UI-10 | 27 |
| UI-11, 12, 13 | 24, 28 |
| UI-14 post-purge routing | 27, 28 |
| UI-15 invalidate + toast | 24 |
| UI-16 three primitives | 22 |
| PRD §11 pre-existing orphan defect | 8 |
| design F2 notification internal routes public | 12, 14 |
