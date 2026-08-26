# Admin Vehicle Fleet Transfer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a platform admin move one vehicle — with its full history, media, categories and activity — from one fleet to another, in one transaction, with a server-verified typed confirmation and an audit row.

**Architecture:** All transfer logic lives in `apps/fleet-service/internal/admin` as a declarative `TransferPlan` of set-based SQL, mirroring the existing purge `Manifest`/`Count`/`Stamp` machinery so the preview and the applied write share predicates. One `db.Transaction` does every local write, then calls media-service and notification-service over new `/internal/admin/reassign-fleet` routes; either failure rolls the whole thing back. The console gets a `VehicleTransferDialog` on each vehicle row of the fleet inspector.

**Tech Stack:** Go 1.27, chi, GORM (Postgres in prod, SQLite in tests), hand-rolled JSON:API in `packages/shared-go/server`; React 19 + TypeScript + Vite, TanStack React Query, shadcn/ui, Tailwind, Vitest + Testing Library.

**Spec:** `docs/tasks/task-031-admin-vehicle-fleet-transfer/design.md` (PRD at `prd.md`)

## Global Constraints

- **`internal/admin` never imports another fleet-service domain package.** Cross-domain work is raw SQL against schema-qualified tables, or a port declared locally with its adapter in `cmd/main.go`. Enforced by `internal/admin/arch_test.go`.
- **`PlatformAdmin` may only be read inside `internal/admin` + `internal/adminclient`; `RequireSameFleet(` may never appear inside them.** Enforced by the separation arch test.
- **Every `TableName()` under `apps/fleet-service/internal/` must appear in `admin.Manifest` or `admin.excludedTables`.** This task adds no tables, so it stays satisfied.
- **All SQL must run on both Postgres and SQLite.** No `->>`, no `ON CONFLICT`, no `json_extract`. Every DB-backed `internal/admin` test uses `admintest.NewDB`. The one permitted dialect branch is the `FOR UPDATE` suffix (Task 9), because it is a locking clause, not a predicate.
- **No SQL migration files exist in this repo.** Schema changes are entity-struct edits picked up by the package's `Migration(db)` `AutoMigrate`. New columns must be nullable.
- **`Administrator` methods take an explicit `tx *gorm.DB`** so writes join the caller's transaction.
- **`now` is always a parameter, never SQL `now()`** — the test harness is SQLite.
- **Media-failure status is `503`** via `server.ErrServiceUnavailable`. There is no `502` sentinel in this codebase (design D2).
- **Confirmation comparison is exact** — no trimming, no case folding — reusing `admin.MatchConfirmation(ScopeFleet, label, supplied)`.
- **Audit action string:** `vehicle.transferred`. **Activity event types:** `vehicle.transferred_out`, `vehicle.transferred_in`.
- **Preview resource type:** `vehicle-transfer-previews`. **Transfer resource type:** `vehicle-transfers`.
- **Gateway:** `/internal/admin/reassign-fleet` needs no new deploy config. `tools/check-manifests.sh` (already in `make ci`) asserts the priority-200 `internal-deny` rule on both entrypoints for every service with an unauthenticated `/internal/*` surface.
- **Verification gate:** `make ci` must pass. Node may need `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22` first.

## Deviations from the design, decided during planning

1. **`BlastRadiusPanel` is NOT reused** (design §7.1 said it would be). Read it: it hard-codes the heading "Delete this fleet", a destructive "Purge this fleet" button and an `onPurge` prop. Bending it to serve a transfer would mean parameterising every one of those, changing a component that sits under a live purge control. The transfer dialog renders its own counts list instead, using the same `humanise`-style labelling. Task 14.
2. **The destination picker is a debounced search `Input` plus a list of selectable `Button` rows, not a Radix `<Select>`.** FR-XFER-UI-3 requires live search, which a `Select` of static options cannot do, and Radix `Select` is hostile to jsdom pointer events. Task 14.
3. **`telemetry.headerCorrelationID` is exported as `HeaderCorrelationID`** so `adminclient` can set it without duplicating the literal. Task 6.
4. **D12 (notification-service) is IN SCOPE** — confirmed with the user during planning.

## File Structure

**fleet-service**

| File | Responsibility |
|---|---|
| `internal/admin/entity.go` (modify) | two nullable audit columns, `ActionVehicleTransferred` |
| `internal/admin/model.go` (modify) | `AuditEvent` source/destination fields + round-trip |
| `internal/admin/rest.go` (modify) | audit attrs, `TransformTransferPreview`, `TransformTransfer` |
| `internal/admin/transfer.go` (create) | `TransferSpec`, `TransferPlan`, counts, media-ID union, category resolution, widget pruning, `ApplyTransfer` |
| `internal/admin/transfer_processor.go` (create) | `PreviewVehicleTransfer`, `TransferVehicle` — validation, confirmation, transaction, downstream, compensation, audit |
| `internal/admin/resource.go` (modify) | the two new routes |
| `internal/admin/processor.go` (modify) | `Reassigner` ports on `Deps` |
| `internal/admin/admintest/db.go` (modify) | DDL for the new audit columns and `maintenance_categories.fleet_id` |
| `internal/adminclient/http.go` (modify) | correlation-ID header |
| `internal/adminclient/media.go` (modify) | `Reassign` |
| `internal/adminclient/notification.go` (modify) | `Reassign` |
| `internal/activity/{entity,model,administrator}.go` (modify) | narrow the append-only comment |
| `cmd/main.go` (modify) | wire the two reassigners |

**media-service / notification-service**

| File | Responsibility |
|---|---|
| `apps/media-service/internal/admin/reassign.go` (create) | `Reassign` + request shape |
| `apps/media-service/internal/admin/resource.go` (modify) | route |
| `apps/notification-service/internal/admin/reassign.go` (create) | `Reassign` + request shape |
| `apps/notification-service/internal/admin/resource.go` (modify) | route |

**shared-go**

| File | Responsibility |
|---|---|
| `packages/shared-go/telemetry/correlation.go` (modify) | export `HeaderCorrelationID` |

**web**

| File | Responsibility |
|---|---|
| `src/types/models/admin.ts` (modify) | transfer types, widened `AuditAction`, audit fleet fields |
| `src/services/api/AdminService.ts` (modify) | `previewVehicleTransfer`, `transferVehicle` |
| `src/lib/hooks/api/admin.ts` (modify) | `useVehicleTransferPreview`, `useTransferVehicle`, key |
| `src/components/admin/VehicleTransferDialog.tsx` (create) | the dialog |
| `src/pages/admin/AdminFleetsPage.tsx` (modify) | Transfer action per vehicle row |
| `src/pages/admin/AdminAuditPage.tsx` (modify) | filter + badge entries |
| `src/test/requiredFieldMarkers.test.ts` (modify) | marker assertion for the dialog |

---

## Task 1: Audit columns, action constant, and the SQLite fixture

**Files:**
- Modify: `apps/fleet-service/internal/admin/entity.go`
- Modify: `apps/fleet-service/internal/admin/model.go`
- Modify: `apps/fleet-service/internal/admin/rest.go`
- Modify: `apps/fleet-service/internal/admin/admintest/db.go`
- Test: `apps/fleet-service/internal/admin/transfer_audit_test.go` (create)

**Interfaces:**
- Consumes: nothing.
- Produces: `admin.ActionVehicleTransferred` (string const `"vehicle.transferred"`); `admin.AuditEvent.SourceFleetID string`, `admin.AuditEvent.DestinationFleetID string`; JSON keys `source_fleet_id`, `destination_fleet_id` on `admin-audit-events` attributes.

- [ ] **Step 1: Write the failing test**

Create `apps/fleet-service/internal/admin/transfer_audit_test.go`:

```go
package admin_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin/admintest"
)

// A transfer audit row must carry BOTH fleet ids as their own columns.
// FR-XFER-AUDIT-4 forbids encoding them in target_label, which is human-facing
// text, so this asserts they survive a real write and read-back.
func TestAuditEvent_carriesSourceAndDestinationFleetIDs(t *testing.T) {
	db := admintest.NewDB(t)
	adm := admin.NewAdministrator(db)
	now := time.Date(2026, 8, 25, 18, 0, 0, 0, time.UTC)

	ev := admin.AuditEvent{
		ID:                 "audit-1",
		ActorUserID:        "admin-1",
		ActorEmail:         "admin@example.com",
		Action:             admin.ActionVehicleTransferred,
		TargetType:         "vehicle",
		TargetID:           "vehicle-1",
		TargetLabel:        "The Green Bean",
		SourceFleetID:      "fleet-a",
		DestinationFleetID: "fleet-b",
		AffectedCounts:     map[string]int{"fuel_logs": 3},
		CorrelationID:      "corr-1",
		CreatedAt:          now,
	}
	if err := adm.InsertAudit(db, ev); err != nil {
		t.Fatalf("insert audit: %v", err)
	}

	var got admin.AuditEntity
	if err := db.Raw(`SELECT * FROM fleet.admin_audit_events WHERE id = ?`, "audit-1").
		Scan(&got).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	back := admin.MakeAudit(got)
	if back.SourceFleetID != "fleet-a" {
		t.Errorf("source_fleet_id = %q, want fleet-a", back.SourceFleetID)
	}
	if back.DestinationFleetID != "fleet-b" {
		t.Errorf("destination_fleet_id = %q, want fleet-b", back.DestinationFleetID)
	}
	if back.Action != "vehicle.transferred" {
		t.Errorf("action = %q, want vehicle.transferred", back.Action)
	}
}

// An empty fleet id must become NULL, not "": Postgres rejects the empty
// string for a uuid column, which is why every other optional id on this
// entity is a *string.
func TestAuditEvent_emptyFleetIDsBecomeNull(t *testing.T) {
	e := admin.AuditEvent{ID: "audit-2", Action: admin.ActionPurgeCreated}.ToEntity()
	if e.SourceFleetID != nil {
		t.Errorf("SourceFleetID = %v, want nil", *e.SourceFleetID)
	}
	if e.DestinationFleetID != nil {
		t.Errorf("DestinationFleetID = %v, want nil", *e.DestinationFleetID)
	}
}

// The console reads these off the JSON:API attributes, so the transform must
// emit both keys.
func TestTransformAuditEvents_emitsFleetIDKeys(t *testing.T) {
	res := admin.TransformAuditEvents([]admin.AuditEvent{{
		ID: "audit-3", Action: admin.ActionVehicleTransferred,
		SourceFleetID: "fleet-a", DestinationFleetID: "fleet-b",
	}})
	raw, err := json.Marshal(res[0].Attributes)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var attrs map[string]any
	if err := json.Unmarshal(raw, &attrs); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if attrs["source_fleet_id"] != "fleet-a" {
		t.Errorf("source_fleet_id = %v, want fleet-a", attrs["source_fleet_id"])
	}
	if attrs["destination_fleet_id"] != "fleet-b" {
		t.Errorf("destination_fleet_id = %v, want fleet-b", attrs["destination_fleet_id"])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./apps/fleet-service/internal/admin/ -run 'Audit.*FleetID|TransformAuditEvents_emits' -v`
Expected: compile failure — `ev.SourceFleetID undefined`, `admin.ActionVehicleTransferred undefined`.

- [ ] **Step 3: Add the columns and the constant**

In `apps/fleet-service/internal/admin/entity.go`, inside `AuditEntity`, after `AffectedCounts`:

```go
	// Source and destination fleet for a vehicle transfer; NULL for every
	// purge.* row (FR-XFER-AUDIT-4). Nullable so the existing rows stay valid
	// — AutoMigrate adds the columns on the next boot, and this repo has no
	// migration files. They are their own columns rather than entries in
	// affected_counts (which is map[string]int) or in target_label (which is
	// human-facing text the operator typed).
	SourceFleetID      *string `gorm:"type:uuid"`
	DestinationFleetID *string `gorm:"type:uuid"`
```

In the audit action block, beside the four `purge.*` constants:

```go
	// ActionVehicleTransferred records an admin vehicle fleet transfer. Unlike
	// the purge actions it carries no purge_operation_id: a transfer is not a
	// purge.
	ActionVehicleTransferred = "vehicle.transferred"
```

- [ ] **Step 4: Round-trip them through the model**

In `apps/fleet-service/internal/admin/model.go`, add to `AuditEvent` after `AffectedCounts`:

```go
	// Empty unless Action is ActionVehicleTransferred.
	SourceFleetID      string
	DestinationFleetID string
```

In `MakeAudit`, after the `PurgeOperationID` block:

```go
	if e.SourceFleetID != nil {
		a.SourceFleetID = *e.SourceFleetID
	}
	if e.DestinationFleetID != nil {
		a.DestinationFleetID = *e.DestinationFleetID
	}
```

In `func (a AuditEvent) ToEntity()`, after the `PurgeOperationID` block:

```go
	if a.SourceFleetID != "" {
		s := a.SourceFleetID
		e.SourceFleetID = &s
	}
	if a.DestinationFleetID != "" {
		d := a.DestinationFleetID
		e.DestinationFleetID = &d
	}
```

- [ ] **Step 5: Expose them on the JSON:API attributes**

In `apps/fleet-service/internal/admin/rest.go`, add to `auditAttributes` after `PurgeOperationID`:

```go
	SourceFleetID      string `json:"source_fleet_id"`
	DestinationFleetID string `json:"destination_fleet_id"`
```

and in the `TransformAuditEvents` literal, after `PurgeOperationID: a.PurgeOperationID,`:

```go
				SourceFleetID:      a.SourceFleetID,
				DestinationFleetID: a.DestinationFleetID,
```

- [ ] **Step 6: Extend the SQLite fixture**

In `apps/fleet-service/internal/admin/admintest/db.go`, replace the `fleet.admin_audit_events` DDL statement with:

```go
	// No deleted_at: append-only, and it survives a system purge
	// (FR-ADMIN-AUDIT-2). source_fleet_id/destination_fleet_id are NULL for
	// every purge.* row and populated only by a vehicle transfer.
	`CREATE TABLE fleet.admin_audit_events (
		id TEXT PRIMARY KEY, actor_user_id TEXT, actor_email TEXT, action TEXT,
		scope TEXT, target_type TEXT, target_id TEXT, target_label TEXT,
		purge_operation_id TEXT, affected_counts BLOB,
		source_fleet_id TEXT, destination_fleet_id TEXT,
		correlation_id TEXT, created_at DATETIME)`,
```

and replace the `fleet.maintenance_categories` DDL statement with:

```go
	// Seeded reference data. Present so a cascade test can assert it SURVIVES.
	// fleet_id is NULL for the system rows and set for fleet-scoped ones,
	// which is what the transfer's category remap keys on.
	`CREATE TABLE fleet.maintenance_categories (
		id TEXT PRIMARY KEY, name TEXT, description TEXT,
		system_defined BOOLEAN, kind TEXT, fleet_id TEXT)`,
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test ./apps/fleet-service/internal/admin/... -v`
Expected: PASS, including the pre-existing purge/cascade tests.

- [ ] **Step 8: Commit**

```bash
git add apps/fleet-service/internal/admin/entity.go \
        apps/fleet-service/internal/admin/model.go \
        apps/fleet-service/internal/admin/rest.go \
        apps/fleet-service/internal/admin/admintest/db.go \
        apps/fleet-service/internal/admin/transfer_audit_test.go
git commit -m "feat(fleet-service): audit source/destination fleet columns and vehicle.transferred action"
```

---

## Task 2: The transfer plan — spec, steps, counts, and the media-ID union

**Files:**
- Create: `apps/fleet-service/internal/admin/transfer.go`
- Test: `apps/fleet-service/internal/admin/transfer_test.go` (create)

**Interfaces:**
- Consumes: `admintest.NewDB`, `admintest.SeedFleet` from Task 1's extended fixture.
- Produces:
  - `type TransferSpec struct { VehicleID, SourceFleetID, DestFleetID, Label, ActorUserID string; Now time.Time }`
  - `type TransferStep struct { Key, Table, Where, Set string }`
  - `var TransferPlan []TransferStep`
  - `func CountTransfer(db *gorm.DB, vehicleID string) (map[string]int, error)`
  - `func VehicleMediaIDs(db *gorm.DB, vehicleID string) ([]string, error)`

- [ ] **Step 1: Write the failing test**

Create `apps/fleet-service/internal/admin/transfer_test.go`:

```go
package admin_test

import (
	"sort"
	"testing"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin/admintest"
)

// countOnlyKeys are the plan steps that must NEVER carry a Set clause. They are
// FR-XFER-MOVE-2's "these follow the vehicle for free" list, expressed as an
// absence in the plan rather than left to a reviewer's memory.
var countOnlyKeys = map[string]bool{
	"maintenance_records":   true,
	"maintenance_schedules": true,
	"fuel_logs":             true,
	"mileage_records":       true,
	"vehicle_media":         true,
}

func TestTransferPlan_countOnlyStepsHaveNoSetClause(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range admin.TransferPlan {
		if seen[s.Key] {
			t.Errorf("duplicate plan key %q — keys are API surface in affected_counts", s.Key)
		}
		seen[s.Key] = true
		if countOnlyKeys[s.Key] && s.Set != "" {
			t.Errorf("%s must be count-only (FR-XFER-MOVE-2) but has Set = %q", s.Key, s.Set)
		}
		if !countOnlyKeys[s.Key] && s.Set == "" {
			t.Errorf("%s has no Set clause; if that is intended, add it to countOnlyKeys", s.Key)
		}
	}
	for k := range countOnlyKeys {
		if !seen[k] {
			t.Errorf("plan is missing count-only step %q", k)
		}
	}
}

func TestCountTransfer_countsTheVehiclesOwnRowsOnly(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-a")

	got, err := admin.CountTransfer(db, f.VehicleID)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	// SeedFleet gives vehicle-1 exactly one of each and vehicle-2 none.
	for _, k := range []string{
		"maintenance_records", "maintenance_schedules", "fuel_logs",
		"mileage_records", "vehicle_media", "activity_events",
	} {
		if got[k] != 1 {
			t.Errorf("%s = %d, want 1", k, got[k])
		}
	}

	second, err := admin.CountTransfer(db, f.SecondVehicleID)
	if err != nil {
		t.Fatalf("count second: %v", err)
	}
	for k, n := range second {
		if n != 0 {
			t.Errorf("second vehicle %s = %d, want 0", k, n)
		}
	}
}

// A fleet-level activity event (vehicle_id IS NULL) describes the fleet, not
// the car, and must never be counted or moved (FR-XFER-MOVE-4).
func TestCountTransfer_ignoresFleetLevelActivityEvents(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-a")
	if err := db.Exec(`INSERT INTO fleet.activity_events (id, fleet_id, vehicle_id, actor_user_id, type)
	                   VALUES ('ae-fleet', ?, NULL, ?, 'membership.joined')`,
		f.FleetID, f.OwnerUserID).Error; err != nil {
		t.Fatalf("seed fleet-level event: %v", err)
	}
	got, err := admin.CountTransfer(db, f.VehicleID)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if got["activity_events"] != 1 {
		t.Errorf("activity_events = %d, want 1 (the fleet-level row must not be counted)", got["activity_events"])
	}
}

func TestVehicleMediaIDs_unionsPhotosPrimaryImageAndReceipts(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-a")
	// SeedFleet already gives vehicle_media -> MediaID and a record document
	// -> MediaID (the same id). Add a distinct primary image and a distinct
	// receipt so the union has three members.
	if err := db.Exec(`UPDATE fleet.vehicles SET primary_image_media_id = 'media-primary' WHERE id = ?`,
		f.VehicleID).Error; err != nil {
		t.Fatalf("set primary image: %v", err)
	}
	if err := db.Exec(`INSERT INTO fleet.maintenance_record_documents (id, maintenance_record_id, media_id)
	                   VALUES ('doc-2', ?, 'media-receipt')`, f.MaintenanceRecordID).Error; err != nil {
		t.Fatalf("seed second document: %v", err)
	}

	ids, err := admin.VehicleMediaIDs(db, f.VehicleID)
	if err != nil {
		t.Fatalf("media ids: %v", err)
	}
	sort.Strings(ids)
	want := []string{f.MediaID, "media-primary", "media-receipt"}
	sort.Strings(want)
	if len(ids) != len(want) {
		t.Fatalf("media ids = %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("media ids = %v, want %v", ids, want)
		}
	}
}

// primary_image_media_id is a plain NOT NULL string column: "none" is the empty
// string, not NULL. Filtering it with IS NOT NULL would send media-service an
// empty id.
func TestVehicleMediaIDs_skipsEmptyPrimaryImage(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-a")
	if err := db.Exec(`UPDATE fleet.vehicles SET primary_image_media_id = '' WHERE id = ?`,
		f.VehicleID).Error; err != nil {
		t.Fatalf("clear primary image: %v", err)
	}
	ids, err := admin.VehicleMediaIDs(db, f.VehicleID)
	if err != nil {
		t.Fatalf("media ids: %v", err)
	}
	for _, id := range ids {
		if id == "" {
			t.Fatalf("media ids contains the empty string: %v", ids)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./apps/fleet-service/internal/admin/ -run 'Transfer|VehicleMediaIDs' -v`
Expected: compile failure — `undefined: admin.TransferPlan`.

- [ ] **Step 3: Write `transfer.go`**

Create `apps/fleet-service/internal/admin/transfer.go`:

```go
package admin

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

// TransferSpec is the resolved, validated input to a vehicle transfer — the
// analogue of Root for the purge path.
//
// Now is a parameter rather than SQL now() because the entire test harness is
// SQLite, which has no now(). The production call site passes time.Now().UTC().
type TransferSpec struct {
	VehicleID     string
	SourceFleetID string
	DestFleetID   string
	// Label is the confirmation phrase (FR-XFER-CONF-2): the vehicle's
	// nickname, or "{year} {make} {model}" when it has none.
	Label       string
	ActorUserID string
	Now         time.Time
}

// TransferStep is one table the transfer counts and, sometimes, rewrites.
//
// Where and Set are used by BOTH CountTransfer and ApplyTransfer, which is what
// makes the blast-radius preview and the rows actually touched provably equal
// rather than equal by discipline — the same property admin.Count/Stamp have.
type TransferStep struct {
	// Key is the name used in affected_counts JSON and in the console's
	// blast-radius panel. It is API surface: renaming one is breaking.
	Key   string
	Table string
	// Where selects this table's rows for the moved vehicle. It binds exactly
	// one argument: the vehicle id.
	Where string
	// Set is the assignment ApplyTransfer writes, binding exactly one
	// argument: the destination fleet id.
	//
	// EMPTY MEANS COUNT-ONLY. Those tables key on vehicle_id alone, so they
	// follow the car for free and the transfer must not rewrite them
	// (FR-XFER-MOVE-2). The absence is the enforcement, and
	// TestTransferPlan_countOnlyStepsHaveNoSetClause pins it.
	Set string
}

// TransferPlan is the hand-enumerated list of tables a vehicle transfer
// touches, mirroring Manifest's role for a purge.
//
// The full fleet-service table inventory, with the transfer question answered
// for each (design §2.1):
//
//	fleet.vehicles                       rewritten explicitly, not a step (see ApplyTransfer)
//	fleet.activity_events                rewritten where vehicle_id matches
//	fleet.maintenance_categories         find-or-create in the destination (ResolveCategories)
//	fleet.dashboard_widgets              source-fleet rows pinned to the vehicle are DELETED (PruneWidgets)
//	fleet.maintenance_records            category_id remapped only
//	fleet.maintenance_schedules          category_id remapped only
//	fleet.maintenance_record_documents   untouched; source of receipt media ids
//	fleet.fuel_logs                      untouched
//	fleet.mileage_records                untouched
//	fleet.vehicle_media                  untouched; source of photo media ids
//	fleet.dashboards                     untouched (fleet-scoped, not vehicle-scoped)
//	fleet.fleets, fleet_memberships, fleet_invites  untouched (PRD non-goals)
//	fleet.purge_operations               untouched
//	fleet.admin_audit_events             one row appended
//	media.media_objects                  delegated to media-service
//	notification.notifications           delegated to notification-service
//
// If a future table gains a fleet_id or a vehicle reference, answer the
// transfer question here at the same time arch_test.go forces you to answer the
// purge question in Manifest.
var TransferPlan = []TransferStep{
	{Key: "maintenance_records", Table: "fleet.maintenance_records", Where: "vehicle_id = ?"},
	{Key: "maintenance_schedules", Table: "fleet.maintenance_schedules", Where: "vehicle_id = ?"},
	{Key: "fuel_logs", Table: "fleet.fuel_logs", Where: "vehicle_id = ?"},
	{Key: "mileage_records", Table: "fleet.mileage_records", Where: "vehicle_id = ?"},
	{Key: "vehicle_media", Table: "fleet.vehicle_media", Where: "vehicle_id = ?"},
	{
		// The car's timeline follows the car (FR-XFER-MOVE-3). Rows with a NULL
		// vehicle_id describe the FLEET and are never matched by this
		// predicate, so FR-XFER-MOVE-4 holds by construction.
		Key: "activity_events", Table: "fleet.activity_events",
		Where: "vehicle_id = ?", Set: "fleet_id = ?",
	},
}

// CountTransfer returns, per plan key, how many LIVE rows the transfer covers.
// It is the preview's only source for these keys and uses COUNT aggregates —
// no counted row is ever loaded (PRD §8 Performance).
func CountTransfer(db *gorm.DB, vehicleID string) (map[string]int, error) {
	out := make(map[string]int, len(TransferPlan))
	for _, s := range TransferPlan {
		var n int64
		q := "SELECT count(*) FROM " + s.Table + " WHERE (" + s.Where + ") AND deleted_at IS NULL"
		if err := db.Raw(q, vehicleID).Scan(&n).Error; err != nil {
			return nil, fmt.Errorf("count transfer %s: %w", s.Table, err)
		}
		out[s.Key] = int(n)
	}
	return out, nil
}

// VehicleMediaIDs is the set of media objects that must be re-homed with the
// vehicle (FR-XFER-MEDIA-2). Media objects carry a fleet_id, but "the media
// belonging to this vehicle" is a fact only fleet-service holds.
//
// Three sources, unioned:
//
//  1. the vehicle's photos (fleet.vehicle_media),
//  2. its primary image — a plain NOT NULL string column where "none" is the
//     EMPTY STRING, so it is filtered with <> '' and not with IS NOT NULL,
//  3. receipts and attachments on its maintenance records
//     (fleet.maintenance_record_documents, the only attachment table).
//
// UNION, not UNION ALL: the primary image is usually also a vehicle_media row,
// and sending media-service the same id twice would double-count the read-back.
func VehicleMediaIDs(db *gorm.DB, vehicleID string) ([]string, error) {
	var ids []string
	q := `
		SELECT media_id FROM fleet.vehicle_media
		 WHERE vehicle_id = ? AND deleted_at IS NULL
		   AND media_id IS NOT NULL AND media_id <> ''
		UNION
		SELECT primary_image_media_id FROM fleet.vehicles
		 WHERE id = ? AND primary_image_media_id IS NOT NULL
		   AND primary_image_media_id <> ''
		UNION
		SELECT d.media_id FROM fleet.maintenance_record_documents d
		  JOIN fleet.maintenance_records m ON m.id = d.maintenance_record_id
		 WHERE m.vehicle_id = ? AND d.deleted_at IS NULL AND m.deleted_at IS NULL
		   AND d.media_id IS NOT NULL AND d.media_id <> ''`
	if err := db.Raw(q, vehicleID, vehicleID, vehicleID).Scan(&ids).Error; err != nil {
		return nil, fmt.Errorf("resolve transfer media ids: %w", err)
	}
	return ids, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./apps/fleet-service/internal/admin/ -run 'Transfer|VehicleMediaIDs' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/fleet-service/internal/admin/transfer.go \
        apps/fleet-service/internal/admin/transfer_test.go
git commit -m "feat(fleet-service): transfer plan, counts and vehicle media-id union"
```

---

## Task 3: Maintenance category find-or-create and remap

**Files:**
- Modify: `apps/fleet-service/internal/admin/transfer.go`
- Test: `apps/fleet-service/internal/admin/transfer_category_test.go` (create)

**Interfaces:**
- Consumes: `TransferSpec` (Task 2).
- Produces:
  - `type CategoryToCreate struct { Name string \`json:"name"\`; Kind string \`json:"kind"\` }`
  - `func PreviewCategories(db *gorm.DB, spec TransferSpec) ([]CategoryToCreate, error)`
  - `func ResolveCategories(tx *gorm.DB, spec TransferSpec) (created int, err error)`
  - test helpers `transferSpec(vehicleID, src, dst string) admin.TransferSpec` and `scanOne[T any](t, db, q, args...) T`, reused by Tasks 4, 5 and 9.

- [ ] **Step 1: Write the failing test**

Create `apps/fleet-service/internal/admin/transfer_category_test.go`:

```go
package admin_test

import (
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin/admintest"
)

// seedNow is the timestamp admintest.SeedFleet stamps on every row it inserts.
const seedYear = 2026

func seedNow() time.Time { return time.Date(seedYear, 1, 1, 12, 0, 0, 0, time.UTC) }

func transferSpec(vehicleID, src, dst string) admin.TransferSpec {
	return admin.TransferSpec{
		VehicleID:     vehicleID,
		SourceFleetID: src,
		DestFleetID:   dst,
		Label:         "The Green Bean",
		ActorUserID:   "admin-1",
		Now:           time.Date(2026, 8, 25, 18, 0, 0, 0, time.UTC),
	}
}

func scanOne[T any](t *testing.T, db *gorm.DB, q string, args ...any) T {
	t.Helper()
	var out T
	if err := db.Raw(q, args...).Scan(&out).Error; err != nil {
		t.Fatalf("query %.60s: %v", q, err)
	}
	return out
}

// seedCustomCategory attaches a fleet-scoped category to the seeded vehicle's
// record and schedule, replacing the system category SeedFleet used.
func seedCustomCategory(t *testing.T, db *gorm.DB, f admintest.Fixture, id, name, kind string) {
	t.Helper()
	if err := db.Exec(`INSERT INTO fleet.maintenance_categories
		(id, name, description, system_defined, kind, fleet_id)
		VALUES (?, ?, 'Seasonal swap', 0, ?, ?)`, id, name, kind, f.FleetID).Error; err != nil {
		t.Fatalf("seed category: %v", err)
	}
	if err := db.Exec(`UPDATE fleet.maintenance_records SET category_id = ? WHERE id = ?`,
		id, f.MaintenanceRecordID).Error; err != nil {
		t.Fatalf("point record at category: %v", err)
	}
	if err := db.Exec(`UPDATE fleet.maintenance_schedules SET category_id = ? WHERE id = ?`,
		id, f.ScheduleID).Error; err != nil {
		t.Fatalf("point schedule at category: %v", err)
	}
}

func TestResolveCategories_createsDestinationEquivalent(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-a")
	admintest.SeedFleet(t, db, "fleet-b")
	seedCustomCategory(t, db, f, "cat-winter", "Winter Tires", "maintenance")

	created, err := admin.ResolveCategories(db, transferSpec(f.VehicleID, "fleet-a", "fleet-b"))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if created != 1 {
		t.Fatalf("created = %d, want 1", created)
	}

	newID := scanOne[string](t, db,
		`SELECT category_id FROM fleet.maintenance_records WHERE id = ?`, f.MaintenanceRecordID)
	if newID == "cat-winter" {
		t.Fatal("record still points at the source-fleet category")
	}
	if got := scanOne[string](t, db,
		`SELECT category_id FROM fleet.maintenance_schedules WHERE id = ?`, f.ScheduleID); got != newID {
		t.Errorf("schedule category = %q, want the same resolved id %q", got, newID)
	}

	// FR-XFER-CAT-4: name, description and kind copied; fleet_id destination;
	// system_defined false.
	var row struct {
		Name          string
		Description   string
		Kind          string
		FleetID       string
		SystemDefined bool
	}
	if err := db.Raw(`SELECT name, description, kind, fleet_id, system_defined
	                  FROM fleet.maintenance_categories WHERE id = ?`, newID).Scan(&row).Error; err != nil {
		t.Fatalf("read new category: %v", err)
	}
	if row.Name != "Winter Tires" || row.Description != "Seasonal swap" || row.Kind != "maintenance" {
		t.Errorf("new category = %+v, want the source's name/description/kind", row)
	}
	if row.FleetID != "fleet-b" {
		t.Errorf("new category fleet_id = %q, want fleet-b", row.FleetID)
	}
	if row.SystemDefined {
		t.Error("new category is system_defined; a copied fleet category never is")
	}

	// FR-XFER-CAT-6: the source category survives untouched.
	if n := scanOne[int](t, db,
		`SELECT count(*) FROM fleet.maintenance_categories
		  WHERE id = 'cat-winter' AND fleet_id = 'fleet-a'`); n != 1 {
		t.Error("the source-fleet category was deleted or re-scoped")
	}
}

// FR-XFER-CAT-3: the lookup is case-INSENSITIVE, so a destination category that
// differs only in case is reused rather than duplicated.
func TestResolveCategories_reusesCaseInsensitiveMatch(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-a")
	admintest.SeedFleet(t, db, "fleet-b")
	seedCustomCategory(t, db, f, "cat-winter", "Winter Tires", "maintenance")
	if err := db.Exec(`INSERT INTO fleet.maintenance_categories
		(id, name, description, system_defined, kind, fleet_id)
		VALUES ('cat-dest', 'winter tires', '', 0, 'maintenance', 'fleet-b')`).Error; err != nil {
		t.Fatalf("seed destination category: %v", err)
	}

	created, err := admin.ResolveCategories(db, transferSpec(f.VehicleID, "fleet-a", "fleet-b"))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if created != 0 {
		t.Errorf("created = %d, want 0 — the differing-case destination row must be reused", created)
	}
	if got := scanOne[string](t, db,
		`SELECT category_id FROM fleet.maintenance_records WHERE id = ?`,
		f.MaintenanceRecordID); got != "cat-dest" {
		t.Errorf("record category = %q, want cat-dest", got)
	}
}

// Kind is matched EXACTLY. A modification category named the same as a
// maintenance one is a different thing and must not be collapsed into it.
func TestResolveCategories_doesNotMatchAcrossKind(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-a")
	admintest.SeedFleet(t, db, "fleet-b")
	seedCustomCategory(t, db, f, "cat-winter", "Winter Tires", "modification")
	if err := db.Exec(`INSERT INTO fleet.maintenance_categories
		(id, name, description, system_defined, kind, fleet_id)
		VALUES ('cat-dest', 'Winter Tires', '', 0, 'maintenance', 'fleet-b')`).Error; err != nil {
		t.Fatalf("seed destination category: %v", err)
	}

	created, err := admin.ResolveCategories(db, transferSpec(f.VehicleID, "fleet-a", "fleet-b"))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if created != 1 {
		t.Errorf("created = %d, want 1 — a different kind is a different category", created)
	}
}

// FR-XFER-CAT-2: system categories (fleet_id IS NULL) are globally visible and
// must never be remapped. SeedFleet already points the record and schedule at
// 'category-1', a system row.
func TestResolveCategories_leavesSystemCategoriesAlone(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-a")
	admintest.SeedFleet(t, db, "fleet-b")

	created, err := admin.ResolveCategories(db, transferSpec(f.VehicleID, "fleet-a", "fleet-b"))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if created != 0 {
		t.Errorf("created = %d, want 0", created)
	}
	if got := scanOne[string](t, db,
		`SELECT category_id FROM fleet.maintenance_records WHERE id = ?`,
		f.MaintenanceRecordID); got != "category-1" {
		t.Errorf("record category = %q, want the untouched system category-1", got)
	}
}

// Another vehicle in the source fleet keeps pointing at the source category:
// the remap is scoped to the MOVED vehicle's rows only.
func TestResolveCategories_doesNotRemapOtherVehicles(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-a")
	admintest.SeedFleet(t, db, "fleet-b")
	seedCustomCategory(t, db, f, "cat-winter", "Winter Tires", "maintenance")
	if err := db.Exec(`INSERT INTO fleet.maintenance_records
		(id, vehicle_id, category_id, description, performed_at, mileage, cost, created_at)
		VALUES ('rec-other', ?, 'cat-winter', 'Tires', ?, 1, 1.0, ?)`,
		f.SecondVehicleID, seedNow(), seedNow()).Error; err != nil {
		t.Fatalf("seed other vehicle record: %v", err)
	}

	if _, err := admin.ResolveCategories(db, transferSpec(f.VehicleID, "fleet-a", "fleet-b")); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got := scanOne[string](t, db,
		`SELECT category_id FROM fleet.maintenance_records WHERE id = 'rec-other'`); got != "cat-winter" {
		t.Errorf("other vehicle's record category = %q, want the untouched cat-winter", got)
	}
}

// PreviewCategories must report exactly what ResolveCategories would create,
// without writing anything (FR-XFER-UI-4).
func TestPreviewCategories_namesWhatWouldBeCreatedAndWritesNothing(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-a")
	admintest.SeedFleet(t, db, "fleet-b")
	seedCustomCategory(t, db, f, "cat-winter", "Winter Tires", "maintenance")

	before := scanOne[int](t, db, `SELECT count(*) FROM fleet.maintenance_categories`)
	got, err := admin.PreviewCategories(db, transferSpec(f.VehicleID, "fleet-a", "fleet-b"))
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if len(got) != 1 || got[0].Name != "Winter Tires" || got[0].Kind != "maintenance" {
		t.Fatalf("preview = %+v, want one Winter Tires/maintenance entry", got)
	}
	if after := scanOne[int](t, db, `SELECT count(*) FROM fleet.maintenance_categories`); after != before {
		t.Errorf("preview inserted rows: %d -> %d", before, after)
	}
	if got := scanOne[string](t, db,
		`SELECT category_id FROM fleet.maintenance_records WHERE id = ?`,
		f.MaintenanceRecordID); got != "cat-winter" {
		t.Error("preview remapped a record")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./apps/fleet-service/internal/admin/ -run 'Categories' -v`
Expected: compile failure — `undefined: admin.ResolveCategories`.

- [ ] **Step 3: Implement category resolution**

Append to `apps/fleet-service/internal/admin/transfer.go`, and add `"github.com/google/uuid"` to its import block:

```go
// CategoryToCreate names a category the transfer would add to the destination
// fleet. The console lists these under the blast radius (FR-XFER-UI-4).
type CategoryToCreate struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// sourceCategory is one fleet-scoped category the moved vehicle references.
type sourceCategory struct {
	ID          string
	Name        string
	Description string
	Kind        string
}

// candidateCategories returns the DISTINCT source-fleet categories the moved
// vehicle's live records and schedules point at.
//
// The `c.fleet_id = ?` predicate is what makes FR-XFER-CAT-2 hold by
// construction rather than by a check: a system category has a NULL fleet_id
// and can never satisfy it, so it is never a candidate and never remapped.
func candidateCategories(db *gorm.DB, spec TransferSpec) ([]sourceCategory, error) {
	var out []sourceCategory
	q := `
		SELECT DISTINCT c.id, c.name, c.description, c.kind
		  FROM fleet.maintenance_categories c
		 WHERE c.fleet_id = ?
		   AND (c.id IN (SELECT category_id FROM fleet.maintenance_records
		                  WHERE vehicle_id = ? AND deleted_at IS NULL)
		     OR c.id IN (SELECT category_id FROM fleet.maintenance_schedules
		                  WHERE vehicle_id = ? AND deleted_at IS NULL))`
	if err := db.Raw(q, spec.SourceFleetID, spec.VehicleID, spec.VehicleID).Scan(&out).Error; err != nil {
		return nil, fmt.Errorf("resolve source categories: %w", err)
	}
	return out, nil
}

// findDestinationCategory returns the id of the destination-fleet category
// matching name (case-INSENSITIVELY) and kind (EXACTLY), or "" if there is none.
//
// The lookup and the backing unique index deliberately disagree:
// idx_maintenance_categories_scope is case-SENSITIVE on (fleet_id, name, kind)
// and is a backstop against a double-submit, while this LOWER() comparison is
// the real user-facing match, consistent with the domain's own FindByName.
func findDestinationCategory(db *gorm.DB, destFleetID, name, kind string) (string, error) {
	var ids []string
	q := `SELECT id FROM fleet.maintenance_categories
	       WHERE fleet_id = ? AND LOWER(name) = LOWER(?) AND kind = ?
	       LIMIT 1`
	if err := db.Raw(q, destFleetID, name, kind).Scan(&ids).Error; err != nil {
		return "", fmt.Errorf("find destination category: %w", err)
	}
	if len(ids) == 0 {
		return "", nil
	}
	return ids[0], nil
}

// PreviewCategories names the categories a transfer would create in the
// destination fleet. It writes nothing — it runs the same candidate query and
// the same destination lookup ResolveCategories uses, and simply stops there.
func PreviewCategories(db *gorm.DB, spec TransferSpec) ([]CategoryToCreate, error) {
	cands, err := candidateCategories(db, spec)
	if err != nil {
		return nil, err
	}
	out := make([]CategoryToCreate, 0, len(cands))
	for _, c := range cands {
		existing, ferr := findDestinationCategory(db, spec.DestFleetID, c.Name, c.Kind)
		if ferr != nil {
			return nil, ferr
		}
		if existing == "" {
			out = append(out, CategoryToCreate{Name: c.Name, Kind: c.Kind})
		}
	}
	return out, nil
}

// ResolveCategories find-or-creates a destination-fleet equivalent for every
// source-fleet category the moved vehicle references, then rewrites
// category_id on the vehicle's records and schedules to point at it
// (FR-XFER-CAT-3/4/5). It returns how many categories it CREATED.
//
// Source categories are only ever read. They are never deleted, renamed or
// re-scoped even when the moved vehicle was their only consumer, because other
// vehicles and future records in the source fleet may still use them
// (FR-XFER-CAT-6).
func ResolveCategories(tx *gorm.DB, spec TransferSpec) (int, error) {
	cands, err := candidateCategories(tx, spec)
	if err != nil {
		return 0, err
	}
	created := 0
	for _, c := range cands {
		destID, rerr := resolveOneCategory(tx, spec.DestFleetID, c, &created)
		if rerr != nil {
			return 0, rerr
		}
		if err := remapCategory(tx, spec.VehicleID, c.ID, destID); err != nil {
			return 0, err
		}
	}
	return created, nil
}

// resolveOneCategory returns the destination id for one source category,
// inserting it if absent.
//
// A unique violation on the insert means a concurrent transfer created the same
// category between our lookup and our write. That is "someone else created it,
// re-read it", never a 500 — so the lookup runs once more and the winner is
// used. The retry is bounded to one attempt; a second miss is a real error.
func resolveOneCategory(tx *gorm.DB, destFleetID string, c sourceCategory, created *int) (string, error) {
	existing, err := findDestinationCategory(tx, destFleetID, c.Name, c.Kind)
	if err != nil {
		return "", err
	}
	if existing != "" {
		return existing, nil
	}
	newID := uuid.NewString()
	ins := `INSERT INTO fleet.maintenance_categories
	          (id, name, description, system_defined, kind, fleet_id)
	        VALUES (?, ?, ?, ?, ?, ?)`
	if ierr := tx.Exec(ins, newID, c.Name, c.Description, false, c.Kind, destFleetID).Error; ierr != nil {
		winner, ferr := findDestinationCategory(tx, destFleetID, c.Name, c.Kind)
		if ferr != nil {
			return "", ferr
		}
		if winner == "" {
			return "", fmt.Errorf("create destination category %q: %w", c.Name, ierr)
		}
		return winner, nil
	}
	*created++
	return newID, nil
}

// remapCategory repoints the moved vehicle's rows from one source category to
// its destination equivalent. Two set-based statements; never a row-by-row loop.
func remapCategory(tx *gorm.DB, vehicleID, fromID, toID string) error {
	for _, table := range []string{"fleet.maintenance_records", "fleet.maintenance_schedules"} {
		q := "UPDATE " + table + " SET category_id = ?" +
			" WHERE vehicle_id = ? AND category_id = ? AND deleted_at IS NULL"
		if err := tx.Exec(q, toID, vehicleID, fromID).Error; err != nil {
			return fmt.Errorf("remap %s: %w", table, err)
		}
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./apps/fleet-service/internal/admin/ -run 'Categories' -v`
Expected: PASS (6 tests).

- [ ] **Step 5: Commit**

```bash
git add apps/fleet-service/internal/admin/transfer.go \
        apps/fleet-service/internal/admin/transfer_category_test.go
git commit -m "feat(fleet-service): maintenance category find-or-create and remap for transfers"
```

---

## Task 4: Source-fleet dashboard widget pruning

**Files:**
- Modify: `apps/fleet-service/internal/admin/transfer.go`
- Test: `apps/fleet-service/internal/admin/transfer_widget_test.go` (create)

**Interfaces:**
- Consumes: `scanOne` from Task 3's test file.
- Produces:
  - `func WidgetsPinnedToVehicle(db *gorm.DB, sourceFleetID, vehicleID string) ([]string, error)`
  - `func PruneWidgets(tx *gorm.DB, ids []string) (int, error)`

- [ ] **Step 1: Write the failing test**

Create `apps/fleet-service/internal/admin/transfer_widget_test.go`:

```go
package admin_test

import (
	"testing"

	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin/admintest"
)

func addWidget(t *testing.T, db *gorm.DB, id, dashboardID, config string) {
	t.Helper()
	if err := db.Exec(`INSERT INTO fleet.dashboard_widgets
		(id, dashboard_id, type, position_x, position_y, width, height, config)
		VALUES (?, ?, 'vehicle-summary', 0, 0, 2, 2, ?)`, id, dashboardID, config).Error; err != nil {
		t.Fatalf("seed widget %s: %v", id, err)
	}
}

// FR-XFER-SRC-1/2/3 in one fixture: only the source fleet's widgets pinned to
// the MOVED vehicle are candidates.
func TestWidgetsPinnedToVehicle_scopesToSourceFleetAndVehicle(t *testing.T) {
	db := admintest.NewDB(t)
	a := admintest.SeedFleet(t, db, "fleet-a")
	b := admintest.SeedFleet(t, db, "fleet-b")

	pinned := `{"vehicleId":"` + a.VehicleID + `"}`
	addWidget(t, db, "w-source-pinned", a.DashboardID, pinned)
	addWidget(t, db, "w-source-other-vehicle", a.DashboardID,
		`{"vehicleId":"`+a.SecondVehicleID+`"}`)
	addWidget(t, db, "w-source-no-vehicle", a.DashboardID, `{"range":"90d"}`)
	addWidget(t, db, "w-source-malformed", a.DashboardID, `{not json`)
	// Same vehicle id, but a DIFFERENT fleet's dashboard: never a candidate.
	addWidget(t, db, "w-dest-pinned", b.DashboardID, pinned)

	ids, err := admin.WidgetsPinnedToVehicle(db, a.FleetID, a.VehicleID)
	if err != nil {
		t.Fatalf("pinned widgets: %v", err)
	}
	if len(ids) != 1 || ids[0] != "w-source-pinned" {
		t.Fatalf("pinned = %v, want [w-source-pinned]", ids)
	}
}

func TestPruneWidgets_deletesOnlyTheNamedRows(t *testing.T) {
	db := admintest.NewDB(t)
	a := admintest.SeedFleet(t, db, "fleet-a")
	addWidget(t, db, "w-doomed", a.DashboardID, `{"vehicleId":"`+a.VehicleID+`"}`)
	addWidget(t, db, "w-kept", a.DashboardID, `{"vehicleId":"`+a.SecondVehicleID+`"}`)

	n, err := admin.PruneWidgets(db, []string{"w-doomed"})
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if n != 1 {
		t.Errorf("pruned = %d, want 1", n)
	}
	if got := scanOne[int](t, db,
		`SELECT count(*) FROM fleet.dashboard_widgets WHERE id = 'w-doomed'`); got != 0 {
		t.Error("the pinned widget survived; the delete is a HARD delete (design D5)")
	}
	if got := scanOne[int](t, db,
		`SELECT count(*) FROM fleet.dashboard_widgets WHERE id = 'w-kept'`); got != 1 {
		t.Error("an unrelated widget was deleted")
	}
	// The dashboard itself is fleet-scoped and stays.
	if got := scanOne[int](t, db,
		`SELECT count(*) FROM fleet.dashboards WHERE id = ?`, a.DashboardID); got != 1 {
		t.Error("the dashboard was deleted; only widgets are pruned")
	}
}

func TestPruneWidgets_emptyInputIsANoOp(t *testing.T) {
	db := admintest.NewDB(t)
	admintest.SeedFleet(t, db, "fleet-a")
	before := scanOne[int](t, db, `SELECT count(*) FROM fleet.dashboard_widgets`)

	n, err := admin.PruneWidgets(db, nil)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if n != 0 {
		t.Errorf("pruned = %d, want 0", n)
	}
	if after := scanOne[int](t, db, `SELECT count(*) FROM fleet.dashboard_widgets`); after != before {
		t.Errorf("widget count changed: %d -> %d", before, after)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./apps/fleet-service/internal/admin/ -run 'Widget' -v`
Expected: compile failure — `undefined: admin.WidgetsPinnedToVehicle`.

- [ ] **Step 3: Implement widget pruning**

Append to `apps/fleet-service/internal/admin/transfer.go`, and add `"encoding/json"` to its import block:

```go
// widgetConfig is the only part of a dashboard widget's jsonb config this
// package reads. A widget that pins no vehicle unmarshals to the empty string
// and is skipped.
type widgetConfig struct {
	VehicleID string `json:"vehicleId"`
}

// WidgetsPinnedToVehicle returns the ids of live SOURCE-fleet dashboard widgets
// whose config pins the moved vehicle (FR-XFER-SRC-1/2/3).
//
// fleet.dashboard_widgets has no fleet_id of its own; it joins to
// fleet.dashboards, which does. That join is what scopes the candidate set to
// the source fleet, so destination-fleet and third-fleet widgets are never even
// considered.
//
// The vehicle match is made in Go, on the PARSED config, rather than in SQL.
// Postgres would express it as config->>'vehicleId' = ?, but every DB-backed
// test in this package runs on SQLite, which has no ->> operator. A dialect
// branch on a PREDICATE would mean the tested path and the production path are
// different SQL — exactly the class of bug that hid a broken local overlay for
// ten reviews. A one-off DDL branch is a different thing; this is not that.
//
// The candidate set is bounded by (members × widgets per dashboard) — one live
// dashboard per (fleet, user) is enforced by a partial unique index — so this is
// tens of rows, not thousands. The NFR's "never a row-by-row loop" is about the
// WRITE, and PruneWidgets is a single set-based DELETE.
//
// A malformed config is skipped rather than fatal, matching how MakeAudit
// tolerates malformed affected_counts.
func WidgetsPinnedToVehicle(db *gorm.DB, sourceFleetID, vehicleID string) ([]string, error) {
	var rows []struct {
		ID     string
		Config []byte
	}
	q := `SELECT w.id, w.config
	        FROM fleet.dashboard_widgets w
	        JOIN fleet.dashboards d ON d.id = w.dashboard_id
	       WHERE d.fleet_id = ? AND w.deleted_at IS NULL AND d.deleted_at IS NULL`
	if err := db.Raw(q, sourceFleetID).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("list source dashboard widgets: %w", err)
	}
	var ids []string
	for _, r := range rows {
		var cfg widgetConfig
		if err := json.Unmarshal(r.Config, &cfg); err != nil {
			continue
		}
		if cfg.VehicleID == vehicleID {
			ids = append(ids, r.ID)
		}
	}
	return ids, nil
}

// PruneWidgets hard-deletes the named widgets and returns how many rows went.
//
// A HARD delete, deliberately: FR-XFER-SRC-1 says "deleted", these rows carry
// no history, and a soft-deleted widget would still occupy its slot in the
// layout's position grid. This is the one place a transfer destroys data. It is
// bounded, it is reported as widgets_removed, and the operator sees the number
// in the blast-radius panel before they confirm.
func PruneWidgets(tx *gorm.DB, ids []string) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	res := tx.Exec(`DELETE FROM fleet.dashboard_widgets WHERE id IN ?`, ids)
	if res.Error != nil {
		return 0, fmt.Errorf("prune dashboard widgets: %w", res.Error)
	}
	return int(res.RowsAffected), nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./apps/fleet-service/internal/admin/ -run 'Widget' -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add apps/fleet-service/internal/admin/transfer.go \
        apps/fleet-service/internal/admin/transfer_widget_test.go
git commit -m "feat(fleet-service): prune source-fleet dashboard widgets pinned to a transferred vehicle"
```

---

## Task 5: ApplyTransfer — the whole local write

**Files:**
- Modify: `apps/fleet-service/internal/admin/transfer.go`
- Test: `apps/fleet-service/internal/admin/transfer_apply_test.go` (create)

**Interfaces:**
- Consumes: `TransferSpec`, `TransferPlan`, `CountTransfer` (Task 2); `ResolveCategories` (Task 3); `WidgetsPinnedToVehicle`, `PruneWidgets` (Task 4).
- Produces: `func ApplyTransfer(tx *gorm.DB, spec TransferSpec) (map[string]int, error)` — returns `affected_counts` with every plan key plus `categories_created` and `widgets_removed`. It does **not** set `media_objects` or `notifications`; the processor fills those from the downstream responses (Task 9).
- Produces: exported constants `EventVehicleTransferredOut = "vehicle.transferred_out"`, `EventVehicleTransferredIn = "vehicle.transferred_in"`.

- [ ] **Step 1: Write the failing test**

Create `apps/fleet-service/internal/admin/transfer_apply_test.go`:

```go
package admin_test

import (
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin/admintest"
)

// rowSnapshot is every column of one row, keyed by column name, so a test can
// assert byte-identity without enumerating columns it does not care about.
func rowSnapshot(t *testing.T, db *gorm.DB, table, id string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := db.Raw("SELECT * FROM "+table+" WHERE id = ?", id).Scan(&out).Error; err != nil {
		t.Fatalf("snapshot %s/%s: %v", table, id, err)
	}
	if len(out) == 0 {
		t.Fatalf("snapshot %s/%s: no such row", table, id)
	}
	return out
}

func assertSameExcept(t *testing.T, label string, before, after map[string]any, except ...string) {
	t.Helper()
	skip := map[string]bool{}
	for _, k := range except {
		skip[k] = true
	}
	for k, v := range before {
		if skip[k] {
			continue
		}
		if got := after[k]; got != v {
			t.Errorf("%s: column %s changed %v -> %v; the transfer must not touch it", label, k, v, got)
		}
	}
}

func applyFixture(t *testing.T) (*gorm.DB, admintest.Fixture, admin.TransferSpec) {
	t.Helper()
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-a")
	admintest.SeedFleet(t, db, "fleet-b")
	return db, f, transferSpec(f.VehicleID, "fleet-a", "fleet-b")
}

// FR-XFER-MOVE-1: the vehicle moves and created_at is preserved. The UPDATE
// never mentions created_at at all, which is strictly stronger than relying on
// GORM's `<-:create` tag (that only protects db.Save).
func TestApplyTransfer_movesTheVehicleAndPreservesCreatedAt(t *testing.T) {
	db, f, spec := applyFixture(t)
	before := rowSnapshot(t, db, "fleet.vehicles", f.VehicleID)

	if _, err := admin.ApplyTransfer(db, spec); err != nil {
		t.Fatalf("apply: %v", err)
	}

	after := rowSnapshot(t, db, "fleet.vehicles", f.VehicleID)
	if after["fleet_id"] != "fleet-b" {
		t.Errorf("fleet_id = %v, want fleet-b", after["fleet_id"])
	}
	assertSameExcept(t, "vehicles", before, after, "fleet_id", "updated_at")
}

// FR-XFER-MOVE-2, narrowed by design D10. Three tables must be BYTE-IDENTICAL,
// plus maintenance_record_documents; records and schedules may differ in
// category_id and nothing else.
func TestApplyTransfer_leavesVehicleScopedHistoryUntouched(t *testing.T) {
	db, f, spec := applyFixture(t)

	type target struct{ table, id string }
	identical := []target{
		{"fleet.mileage_records", f.MileageRecordID},
		{"fleet.fuel_logs", f.FuelLogID},
		{"fleet.vehicle_media", f.VehicleMediaID},
		{"fleet.maintenance_record_documents", f.DocumentID},
	}
	categoryOnly := []target{
		{"fleet.maintenance_records", f.MaintenanceRecordID},
		{"fleet.maintenance_schedules", f.ScheduleID},
	}

	before := map[string]map[string]any{}
	for _, tg := range append(append([]target{}, identical...), categoryOnly...) {
		before[tg.table] = rowSnapshot(t, db, tg.table, tg.id)
	}

	if _, err := admin.ApplyTransfer(db, spec); err != nil {
		t.Fatalf("apply: %v", err)
	}

	for _, tg := range identical {
		assertSameExcept(t, tg.table, before[tg.table], rowSnapshot(t, db, tg.table, tg.id))
	}
	for _, tg := range categoryOnly {
		after := rowSnapshot(t, db, tg.table, tg.id)
		assertSameExcept(t, tg.table, before[tg.table], after, "category_id")
		// SeedFleet uses a SYSTEM category, so even category_id is unchanged
		// here (FR-XFER-CAT-2).
		if after["category_id"] != before[tg.table]["category_id"] {
			t.Errorf("%s: a system category was remapped", tg.table)
		}
	}
}

// FR-XFER-MOVE-3/4 and design D11: fleet_id is rewritten and NOTHING else is.
func TestApplyTransfer_repointsVehicleActivityAndOnlyItsFleetID(t *testing.T) {
	db, f, spec := applyFixture(t)
	if err := db.Exec(`INSERT INTO fleet.activity_events (id, fleet_id, vehicle_id, actor_user_id, type, created_at)
	                   VALUES ('ae-fleet', ?, NULL, ?, 'membership.joined', ?)`,
		f.FleetID, f.OwnerUserID, seedNow()).Error; err != nil {
		t.Fatalf("seed fleet-level event: %v", err)
	}
	before := rowSnapshot(t, db, "fleet.activity_events", f.ActivityEventID)
	beforeFleetLevel := rowSnapshot(t, db, "fleet.activity_events", "ae-fleet")

	if _, err := admin.ApplyTransfer(db, spec); err != nil {
		t.Fatalf("apply: %v", err)
	}

	after := rowSnapshot(t, db, "fleet.activity_events", f.ActivityEventID)
	if after["fleet_id"] != "fleet-b" {
		t.Errorf("vehicle event fleet_id = %v, want fleet-b", after["fleet_id"])
	}
	assertSameExcept(t, "activity_events", before, after, "fleet_id")

	assertSameExcept(t, "activity_events (fleet-level)", beforeFleetLevel,
		rowSnapshot(t, db, "fleet.activity_events", "ae-fleet"))
}

// FR-XFER-SRC-4, and the ordering that makes it work: both events are inserted
// AFTER the bulk rewrite, so the OUT event keeps the SOURCE fleet id even
// though its vehicle_id matches the rewrite predicate.
func TestApplyTransfer_writesBothTransferEventsWithTheRightFleets(t *testing.T) {
	db, f, spec := applyFixture(t)

	if _, err := admin.ApplyTransfer(db, spec); err != nil {
		t.Fatalf("apply: %v", err)
	}

	out := scanOne[string](t, db,
		`SELECT fleet_id FROM fleet.activity_events WHERE type = ? AND vehicle_id = ?`,
		admin.EventVehicleTransferredOut, f.VehicleID)
	if out != "fleet-a" {
		t.Errorf("transferred_out fleet_id = %q, want fleet-a (it must survive the bulk rewrite)", out)
	}
	in := scanOne[string](t, db,
		`SELECT fleet_id FROM fleet.activity_events WHERE type = ? AND vehicle_id = ?`,
		admin.EventVehicleTransferredIn, f.VehicleID)
	if in != "fleet-b" {
		t.Errorf("transferred_in fleet_id = %q, want fleet-b", in)
	}

	actor := scanOne[string](t, db,
		`SELECT actor_user_id FROM fleet.activity_events WHERE type = ?`,
		admin.EventVehicleTransferredOut)
	if actor != "admin-1" {
		t.Errorf("transferred_out actor = %q, want admin-1", actor)
	}
	payload := scanOne[string](t, db,
		`SELECT payload FROM fleet.activity_events WHERE type = ?`, admin.EventVehicleTransferredOut)
	if payload == "" {
		t.Error("transferred_out payload is empty; it must name the counterpart fleet")
	}
}

// The two transfer events are inserted, so they must NOT inflate the
// activity_events figure the operator was shown. The count is taken before the
// writes for exactly that reason.
func TestApplyTransfer_countsMatchThePreview(t *testing.T) {
	db, f, spec := applyFixture(t)
	preview, err := admin.CountTransfer(db, f.VehicleID)
	if err != nil {
		t.Fatalf("preview count: %v", err)
	}

	applied, err := admin.ApplyTransfer(db, spec)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	for k, want := range preview {
		if applied[k] != want {
			t.Errorf("%s: preview %d, applied %d", k, want, applied[k])
		}
	}
	if _, ok := applied["categories_created"]; !ok {
		t.Error("affected_counts is missing categories_created")
	}
	if _, ok := applied["widgets_removed"]; !ok {
		t.Error("affected_counts is missing widgets_removed")
	}
}

func TestApplyTransfer_reportsCategoriesCreatedAndWidgetsRemoved(t *testing.T) {
	db, f, spec := applyFixture(t)
	seedCustomCategory(t, db, f, "cat-winter", "Winter Tires", "maintenance")
	addWidget(t, db, "w-pinned", f.DashboardID, `{"vehicleId":"`+f.VehicleID+`"}`)

	applied, err := admin.ApplyTransfer(db, spec)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if applied["categories_created"] != 1 {
		t.Errorf("categories_created = %d, want 1", applied["categories_created"])
	}
	if applied["widgets_removed"] != 1 {
		t.Errorf("widgets_removed = %d, want 1", applied["widgets_removed"])
	}
}

// The whole operation must be atomic. A failure anywhere inside the caller's
// transaction leaves the vehicle where it was.
func TestApplyTransfer_rollsBackWithTheCallersTransaction(t *testing.T) {
	db, f, spec := applyFixture(t)
	sentinel := errForTest{}

	err := db.Transaction(func(tx *gorm.DB) error {
		if _, aerr := admin.ApplyTransfer(tx, spec); aerr != nil {
			return aerr
		}
		return sentinel
	})
	if err != sentinel {
		t.Fatalf("transaction err = %v, want the sentinel", err)
	}
	if got := scanOne[string](t, db, `SELECT fleet_id FROM fleet.vehicles WHERE id = ?`,
		f.VehicleID); got != "fleet-a" {
		t.Errorf("fleet_id = %q after rollback, want fleet-a", got)
	}
	if n := scanOne[int](t, db, `SELECT count(*) FROM fleet.activity_events WHERE type = ?`,
		admin.EventVehicleTransferredIn); n != 0 {
		t.Error("a transfer activity event survived the rollback")
	}
	_ = time.Now
}

type errForTest struct{}

func (errForTest) Error() string { return "rollback sentinel" }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./apps/fleet-service/internal/admin/ -run 'ApplyTransfer' -v`
Expected: compile failure — `undefined: admin.ApplyTransfer`.

- [ ] **Step 3: Implement ApplyTransfer**

Append to `apps/fleet-service/internal/admin/transfer.go`:

```go
// Activity event types the transfer emits (FR-XFER-SRC-4).
//
// Declared here, local to internal/admin, rather than in a shared constants
// block: the eight existing event types are inline literals in the six domains
// that emit them, a shared block would have to live in internal/activity and be
// imported by all six, and doing that as a side effect of a transfer feature is
// unrelated refactoring. These two are emitted here, so they live here.
const (
	EventVehicleTransferredOut = "vehicle.transferred_out"
	EventVehicleTransferredIn  = "vehicle.transferred_in"
)

// transferPayload is the activity event body for both halves of a transfer.
type transferPayload struct {
	CounterpartFleetID string `json:"counterpart_fleet_id"`
	VehicleLabel       string `json:"vehicle_label"`
}

// ApplyTransfer performs every LOCAL write a vehicle transfer needs and returns
// the affected counts.
//
// It must be called inside the caller's transaction — every statement below
// joins it, so any failure, including the downstream calls the processor makes
// afterwards, unwinds all of this (design D4).
//
// media_objects and notifications are NOT set here: those counts come from the
// downstream services' read-backs, and the processor merges them in.
func ApplyTransfer(tx *gorm.DB, spec TransferSpec) (map[string]int, error) {
	// Counted FIRST, before any write. The two transfer events inserted at the
	// end match the activity_events predicate, and counting after them would
	// report two more rows than the operator was shown in the preview.
	counts, err := CountTransfer(tx, spec.VehicleID)
	if err != nil {
		return nil, err
	}

	// The single UPDATE the whole operation exists to perform. It names
	// fleet_id and updated_at and nothing else — in particular it never
	// mentions created_at, which is a stronger guarantee than GORM's
	// `<-:create` tag, since that only protects db.Save (FR-XFER-MOVE-1).
	if err := tx.Exec(`UPDATE fleet.vehicles SET fleet_id = ?, updated_at = ? WHERE id = ?`,
		spec.DestFleetID, spec.Now, spec.VehicleID).Error; err != nil {
		return nil, fmt.Errorf("move vehicle: %w", err)
	}

	// Every plan step that carries a Set clause. The count-only steps are
	// skipped by the same emptiness test that documents them.
	for _, s := range TransferPlan {
		if s.Set == "" {
			continue
		}
		q := "UPDATE " + s.Table + " SET " + s.Set +
			" WHERE (" + s.Where + ") AND deleted_at IS NULL"
		if err := tx.Exec(q, spec.DestFleetID, spec.VehicleID).Error; err != nil {
			return nil, fmt.Errorf("apply transfer %s: %w", s.Table, err)
		}
	}

	created, err := ResolveCategories(tx, spec)
	if err != nil {
		return nil, err
	}
	counts["categories_created"] = created

	widgetIDs, err := WidgetsPinnedToVehicle(tx, spec.SourceFleetID, spec.VehicleID)
	if err != nil {
		return nil, err
	}
	removed, err := PruneWidgets(tx, widgetIDs)
	if err != nil {
		return nil, err
	}
	counts["widgets_removed"] = removed

	// AFTER the bulk fleet_id rewrite, deliberately. The OUT event carries the
	// SOURCE fleet id and its vehicle_id matches the rewrite predicate, so
	// inserting it earlier would sweep it into the destination fleet and the
	// source fleet would have no record that the car left.
	if err := recordTransferEvent(tx, spec, EventVehicleTransferredOut,
		spec.SourceFleetID, spec.DestFleetID); err != nil {
		return nil, err
	}
	if err := recordTransferEvent(tx, spec, EventVehicleTransferredIn,
		spec.DestFleetID, spec.SourceFleetID); err != nil {
		return nil, err
	}

	return counts, nil
}

// recordTransferEvent appends one activity row. It writes raw SQL rather than
// calling internal/activity, because internal/admin never touches another
// domain's internals — and because internal/activity deliberately exposes no
// way to write a row with an arbitrary fleet id.
func recordTransferEvent(tx *gorm.DB, spec TransferSpec, eventType, fleetID, counterpartID string) error {
	payload, err := json.Marshal(transferPayload{
		CounterpartFleetID: counterpartID,
		VehicleLabel:       spec.Label,
	})
	if err != nil {
		return fmt.Errorf("marshal %s payload: %w", eventType, err)
	}
	q := `INSERT INTO fleet.activity_events
	        (id, fleet_id, vehicle_id, actor_user_id, type, payload, created_at)
	      VALUES (?, ?, ?, ?, ?, ?, ?)`
	if err := tx.Exec(q, uuid.NewString(), fleetID, spec.VehicleID,
		spec.ActorUserID, eventType, payload, spec.Now).Error; err != nil {
		return fmt.Errorf("record %s: %w", eventType, err)
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./apps/fleet-service/internal/admin/ -run 'ApplyTransfer' -v`
Expected: PASS (7 tests).

- [ ] **Step 5: Run the whole package to check nothing regressed**

Run: `go test ./apps/fleet-service/internal/admin/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add apps/fleet-service/internal/admin/transfer.go \
        apps/fleet-service/internal/admin/transfer_apply_test.go
git commit -m "feat(fleet-service): ApplyTransfer performs the whole local vehicle transfer write"
```

---

## Task 6: Correlation-ID propagation and the two `Reassign` clients

**Files:**
- Modify: `packages/shared-go/telemetry/correlation.go`
- Modify: `apps/fleet-service/internal/adminclient/http.go`
- Modify: `apps/fleet-service/internal/adminclient/media.go`
- Modify: `apps/fleet-service/internal/adminclient/notification.go`
- Test: `apps/fleet-service/internal/adminclient/reassign_test.go` (create)

**Interfaces:**
- Consumes: `telemetry.CorrelationIDFromContext`.
- Produces:
  - `telemetry.HeaderCorrelationID` (exported const `"X-Correlation-ID"`)
  - `func (c *MediaClient) Reassign(ctx context.Context, mediaIDs []string, destFleetID string) (map[string]int, error)`
  - `func (c *NotificationClient) Reassign(ctx context.Context, vehicleIDs []string, destFleetID string) (map[string]int, error)`
  - `type ReassignRequest struct { MediaIDs []string \`json:"media_ids,omitempty"\`; VehicleIDs []string \`json:"vehicle_ids,omitempty"\`; DestinationFleetID string \`json:"destination_fleet_id"\` }`

- [ ] **Step 1: Write the failing test**

Create `apps/fleet-service/internal/adminclient/reassign_test.go`:

```go
package adminclient_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/adminclient"
	"github.com/jtumidanski/myfleet/packages/shared-go/telemetry"
)

func TestMediaClient_Reassign_postsIDsAndParsesAffected(t *testing.T) {
	var gotPath string
	var gotBody adminclient.ReassignRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"affected":{"media_objects":9}}`))
	}))
	defer srv.Close()

	got, err := adminclient.NewMediaClient(srv.URL).
		Reassign(context.Background(), []string{"m1", "m2"}, "fleet-b")
	if err != nil {
		t.Fatalf("reassign: %v", err)
	}
	if gotPath != "/internal/admin/reassign-fleet" {
		t.Errorf("path = %q", gotPath)
	}
	if len(gotBody.MediaIDs) != 2 || gotBody.DestinationFleetID != "fleet-b" {
		t.Errorf("body = %+v", gotBody)
	}
	if got["media_objects"] != 9 {
		t.Errorf("affected = %v, want media_objects 9", got)
	}
}

func TestNotificationClient_Reassign_sendsVehicleIDs(t *testing.T) {
	var gotBody adminclient.ReassignRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"affected":{"notifications":4}}`))
	}))
	defer srv.Close()

	got, err := adminclient.NewNotificationClient(srv.URL).
		Reassign(context.Background(), []string{"v1"}, "fleet-b")
	if err != nil {
		t.Fatalf("reassign: %v", err)
	}
	if len(gotBody.VehicleIDs) != 1 || gotBody.VehicleIDs[0] != "v1" {
		t.Errorf("vehicle_ids = %v", gotBody.VehicleIDs)
	}
	if len(gotBody.MediaIDs) != 0 {
		t.Errorf("media_ids should be omitted for notification-service, got %v", gotBody.MediaIDs)
	}
	if got["notifications"] != 4 {
		t.Errorf("affected = %v", got)
	}
}

// expectOK must not let a non-200 read as an empty result: a stranded transfer
// is the outcome that has to be impossible.
func TestMediaClient_Reassign_nonOKIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}))
	defer srv.Close()

	if _, err := adminclient.NewMediaClient(srv.URL).
		Reassign(context.Background(), []string{"m1"}, "fleet-b"); err == nil {
		t.Fatal("expected an error for a 422 response")
	}
}

// NFR Observability: the correlation id must reach the downstream service. This
// also retroactively covers Purge/Restore/Reap/Stats, which share the transport.
func TestTransport_propagatesCorrelationID(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get(telemetry.HeaderCorrelationID)
		_, _ = w.Write([]byte(`{"affected":{}}`))
	}))
	defer srv.Close()

	ctx := telemetry.ContextWithCorrelationID(context.Background(), "corr-42")
	if _, err := adminclient.NewMediaClient(srv.URL).
		Reassign(ctx, []string{"m1"}, "fleet-b"); err != nil {
		t.Fatalf("reassign: %v", err)
	}
	if got != "corr-42" {
		t.Errorf("X-Correlation-ID = %q, want corr-42", got)
	}
}

func TestTransport_omitsCorrelationIDWhenAbsent(t *testing.T) {
	var present bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, present = r.Header[http.CanonicalHeaderKey(telemetry.HeaderCorrelationID)]
		_, _ = w.Write([]byte(`{"affected":{}}`))
	}))
	defer srv.Close()

	if _, err := adminclient.NewMediaClient(srv.URL).
		Reassign(context.Background(), []string{"m1"}, "fleet-b"); err != nil {
		t.Fatalf("reassign: %v", err)
	}
	if present {
		t.Error("X-Correlation-ID was sent for a context that carries none")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./apps/fleet-service/internal/adminclient/ -v`
Expected: compile failure — `undefined: telemetry.HeaderCorrelationID`, `undefined: adminclient.ReassignRequest`.

- [ ] **Step 3: Export the header name and add a context setter**

In `packages/shared-go/telemetry/correlation.go`, replace

```go
const headerCorrelationID = "X-Correlation-ID"
```

with

```go
// HeaderCorrelationID is the header the middleware reads, echoes, and that
// outbound service-to-service clients set so one user action can be followed
// across services. It is exported because fleet-service's adminclient sets it
// on every internal call, and a duplicated literal there would drift.
const HeaderCorrelationID = "X-Correlation-ID"
```

and update the two uses inside `CorrelationID` (`r.Header.Get(...)` and
`w.Header().Set(...)`) to the exported name.

Append to the same file:

```go
// ContextWithCorrelationID seeds a context with an id. Production code gets one
// from the CorrelationID middleware; this exists so a client test can assert
// the header is propagated without standing up a server to mint one.
func ContextWithCorrelationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, correlationKey, id)
}
```

- [ ] **Step 4: Propagate the header in the shared transport**

In `apps/fleet-service/internal/adminclient/http.go`, inside `transport.do`, after the `Content-Type` block:

```go
	// One user action, one id, across every service it touches. Without this
	// the receiving service's CorrelationID middleware mints a FRESH id and the
	// two halves of a transfer — or of a purge fan-out — cannot be joined in the
	// logs. Set only when the context actually carries one, so an absent id
	// stays absent rather than becoming the empty string.
	if cid := telemetry.CorrelationIDFromContext(ctx); cid != "" {
		req.Header.Set(telemetry.HeaderCorrelationID, cid)
	}
```

Add `"github.com/jtumidanski/myfleet/packages/shared-go/telemetry"` to the import block, and add the shared request shape below `PurgeRequest`:

```go
// ReassignRequest is the body both reassign-fleet endpoints accept. Each
// service reads only its own id list — media-service MediaIDs, notification
// -service VehicleIDs — and omitempty keeps the other out of the wire entirely
// rather than sending a null the receiver has to ignore.
type ReassignRequest struct {
	MediaIDs           []string `json:"media_ids,omitempty"`
	VehicleIDs         []string `json:"vehicle_ids,omitempty"`
	DestinationFleetID string   `json:"destination_fleet_id"`
}
```

- [ ] **Step 5: Add the two client methods**

Append to `apps/fleet-service/internal/adminclient/media.go`:

```go
// Reassign re-homes the named media objects to another fleet, which is what
// keeps a transferred vehicle's photos and receipts readable by the destination
// fleet's members — media-service gates access on fleet_id equality and
// otherwise answers 404.
//
// Idempotent (FR-XFER-MEDIA-4): media-service reads the count back rather than
// taking RowsAffected, so a replay changes nothing and reports the same number.
// That is what makes the compensating reverse call safe to attempt.
//
// The ids are sent unchunked, like Purge's. MaxLookupIDs bounds QUERY-PARAMETER
// lookups; this is a POST body. Chunking would reintroduce partial application
// across chunks, which is the one outcome this operation must not have.
func (c *MediaClient) Reassign(ctx context.Context, mediaIDs []string, destFleetID string) (map[string]int, error) {
	var body affectedResponse
	req := ReassignRequest{MediaIDs: mediaIDs, DestinationFleetID: destFleetID}
	if err := c.t.expectOK(ctx, http.MethodPost, "/internal/admin/reassign-fleet", req, &body); err != nil {
		return nil, err
	}
	return body.Affected, nil
}
```

Append to `apps/fleet-service/internal/adminclient/notification.go`:

```go
// Reassign re-points the fleet_id on notifications about the named vehicles.
//
// It takes VEHICLE ids, not notification ids: notification-service owns the
// vehicle -> notification relationship, and enumerating it in fleet-service
// would mean a cross-service read of another service's rows.
//
// A stale fleet_id here breaks no read — notifications are per-user — but it
// would mis-scope a later fleet-scoped admin purge selecting on that column,
// which is precisely why the column is indexed (design D12).
func (c *NotificationClient) Reassign(ctx context.Context, vehicleIDs []string, destFleetID string) (map[string]int, error) {
	var body affectedResponse
	req := ReassignRequest{VehicleIDs: vehicleIDs, DestinationFleetID: destFleetID}
	if err := c.t.expectOK(ctx, http.MethodPost, "/internal/admin/reassign-fleet", req, &body); err != nil {
		return nil, err
	}
	return body.Affected, nil
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./apps/fleet-service/internal/adminclient/... ./packages/shared-go/telemetry/... -v`
Expected: PASS.

- [ ] **Step 7: Check nothing else used the unexported constant**

Run: `grep -rn "headerCorrelationID" packages apps`
Expected: no output.

- [ ] **Step 8: Commit**

```bash
git add packages/shared-go/telemetry/correlation.go \
        apps/fleet-service/internal/adminclient/
git commit -m "feat(fleet-service): propagate correlation id and add Reassign to the media and notification clients"
```

---

## Task 7: media-service `/internal/admin/reassign-fleet`

**Files:**
- Create: `apps/media-service/internal/admin/reassign.go`
- Modify: `apps/media-service/internal/admin/resource.go`
- Test: `apps/media-service/internal/admin/reassign_test.go` (create)

**Interfaces:**
- Consumes: the request body `adminclient.ReassignRequest` produces (Task 6).
- Produces:
  - `type ReassignRequest struct { MediaIDs []string \`json:"media_ids"\`; DestinationFleetID string \`json:"destination_fleet_id"\` }`
  - `func Reassign(tx *gorm.DB, mediaIDs []string, destFleetID string) (map[string]int, error)` returning `{"media_objects": N}`
  - route `POST /internal/admin/reassign-fleet` returning `{"affected":{"media_objects":N}}`

- [ ] **Step 1: Write the failing test**

Create `apps/media-service/internal/admin/reassign_test.go`:

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
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/apps/media-service/internal/admin"
)

// reassignServer wires the internal routes over a seeded database, matching how
// admin_test.go stands up the purge routes.
func reassignServer(t *testing.T, db *gorm.DB) *httptest.Server {
	t.Helper()
	r := chi.NewRouter()
	log := logrus.New()
	log.SetOutput(io.Discard)
	admin.InitializeInternalRoutes(log, db, &recordingRemover{})(r)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

func postReassign(t *testing.T, srv *httptest.Server, body string) (int, map[string]int) {
	t.Helper()
	res, err := http.Post(srv.URL+"/internal/admin/reassign-fleet",
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	var decoded struct {
		Affected map[string]int `json:"affected"`
	}
	_ = json.NewDecoder(res.Body).Decode(&decoded)
	return res.StatusCode, decoded.Affected
}

func fleetOf(t *testing.T, db *gorm.DB, id string) string {
	t.Helper()
	var out []string
	if err := db.Raw(`SELECT fleet_id FROM media.media_objects WHERE id = ?`, id).Scan(&out).Error; err != nil {
		t.Fatalf("read fleet_id: %v", err)
	}
	if len(out) == 0 {
		t.Fatalf("no media object %s", id)
	}
	return out[0]
}

func TestReassign_movesTheNamedObjects(t *testing.T) {
	db := newMediaDB(t)
	srv := reassignServer(t, db)

	status, affected := postReassign(t, srv,
		`{"media_ids":["mo-1"],"destination_fleet_id":"fleet-9"}`)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if affected["media_objects"] != 1 {
		t.Errorf("affected = %v, want media_objects 1", affected)
	}
	if got := fleetOf(t, db, "mo-1"); got != "fleet-9" {
		t.Errorf("mo-1 fleet_id = %q, want fleet-9", got)
	}
	// An object that was not named must not move.
	if got := fleetOf(t, db, "mo-2"); got != "fleet-2" {
		t.Errorf("mo-2 fleet_id = %q, want the untouched fleet-2", got)
	}
}

// FR-XFER-MEDIA-4. The count is READ BACK, not taken from RowsAffected, so a
// replay reports the same number instead of zero — which is what makes the
// compensating reverse call safe to attempt.
func TestReassign_replayIsANoOpWithTheSameCount(t *testing.T) {
	db := newMediaDB(t)
	srv := reassignServer(t, db)
	body := `{"media_ids":["mo-1"],"destination_fleet_id":"fleet-9"}`

	_, first := postReassign(t, srv, body)
	_, second := postReassign(t, srv, body)
	if first["media_objects"] != second["media_objects"] {
		t.Errorf("replay reported %v, first call reported %v", second, first)
	}
	if second["media_objects"] != 1 {
		t.Errorf("replay affected = %v, want media_objects 1", second)
	}
}

// Unknown ids are ignored rather than an error, matching the tolerance the
// purge path already shows.
func TestReassign_ignoresUnknownIDs(t *testing.T) {
	db := newMediaDB(t)
	srv := reassignServer(t, db)

	status, affected := postReassign(t, srv,
		`{"media_ids":["mo-1","does-not-exist"],"destination_fleet_id":"fleet-9"}`)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if affected["media_objects"] != 1 {
		t.Errorf("affected = %v, want media_objects 1", affected)
	}
}

// A pending-purge media object is not re-homed: it is on its way out, and
// moving it would drag it into a fleet that never had it.
func TestReassign_skipsSoftDeletedObjects(t *testing.T) {
	db := newMediaDB(t)
	if err := db.Exec(`UPDATE media.media_objects SET deleted_at = CURRENT_TIMESTAMP WHERE id = 'mo-1'`).
		Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	srv := reassignServer(t, db)

	_, affected := postReassign(t, srv, `{"media_ids":["mo-1"],"destination_fleet_id":"fleet-9"}`)
	if affected["media_objects"] != 0 {
		t.Errorf("affected = %v, want media_objects 0", affected)
	}
	if got := fleetOf(t, db, "mo-1"); got != "fleet-1" {
		t.Errorf("mo-1 fleet_id = %q, want the untouched fleet-1", got)
	}
}

func TestReassign_rejectsEmptyInput(t *testing.T) {
	db := newMediaDB(t)
	srv := reassignServer(t, db)

	for _, body := range []string{
		`{"media_ids":[],"destination_fleet_id":"fleet-9"}`,
		`{"media_ids":["mo-1"],"destination_fleet_id":""}`,
	} {
		if status, _ := postReassign(t, srv, body); status != http.StatusUnprocessableEntity {
			t.Errorf("body %s: status = %d, want 422", body, status)
		}
	}
}
```

Add `"io"` to the test file's import block.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./apps/media-service/internal/admin/ -run Reassign -v`
Expected: 404 from the router — the route does not exist.

- [ ] **Step 3: Implement `Reassign`**

Create `apps/media-service/internal/admin/reassign.go`:

```go
package admin

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// ReassignRequest re-homes a set of media objects to another fleet. It is the
// body of POST /internal/admin/reassign-fleet, issued by fleet-service when a
// platform admin transfers a vehicle between fleets.
type ReassignRequest struct {
	MediaIDs           []string `json:"media_ids"`
	DestinationFleetID string   `json:"destination_fleet_id"`
}

// Reassign rewrites media_objects.fleet_id for the named objects and returns
// the read-back count.
//
// Rewriting media_objects alone re-homes the whole media subtree: variants and
// the variant-failure ledger key on media_object_id, not on fleet_id.
//
// It is IDEMPOTENT, achieved exactly the way Stamp achieves it — the count is
// READ BACK after the update rather than taken from RowsAffected. A replay
// updates zero rows because of the `fleet_id <> ?` guard, but must still report
// the same number, or fleet-service's compensating reverse call could not tell
// "already done" from "did nothing" (FR-XFER-MEDIA-4).
//
// Unknown ids simply do not match and are ignored, matching the purge path's
// tolerance. Soft-deleted objects are left alone: a pending-purge object is on
// its way out and must not be dragged into a fleet that never had it.
func Reassign(tx *gorm.DB, mediaIDs []string, destFleetID string) (map[string]int, error) {
	upd := `UPDATE media.media_objects SET fleet_id = ?
	         WHERE id IN ? AND fleet_id <> ? AND deleted_at IS NULL`
	if err := tx.Exec(upd, destFleetID, mediaIDs, destFleetID).Error; err != nil {
		return nil, fmt.Errorf("reassign media objects: %w", err)
	}
	var n int64
	cnt := `SELECT count(*) FROM media.media_objects
	         WHERE id IN ? AND fleet_id = ? AND deleted_at IS NULL`
	if err := tx.Raw(cnt, mediaIDs, destFleetID).Scan(&n).Error; err != nil {
		return nil, fmt.Errorf("count reassigned media objects: %w", err)
	}
	return map[string]int{"media_objects": int(n)}, nil
}

// reassignRootFrom validates the request body, or returns 422 — the same
// treatment rootFrom gives a purge body.
func reassignRootFrom(body ReassignRequest) error {
	if len(body.MediaIDs) == 0 {
		return server.Detailed(server.ErrValidation, "media_ids is required")
	}
	if body.DestinationFleetID == "" {
		return server.Detailed(server.ErrValidation, "destination_fleet_id is required")
	}
	return nil
}
```

- [ ] **Step 4: Register the route**

In `apps/media-service/internal/admin/resource.go`, inside `InitializeInternalRoutes`, after the `POST /internal/admin/purge` handler:

```go
		// SECURITY: like its neighbours this route has no authentication, and
		// it is a HIGHER-value target than they are — it can move any media
		// object into any fleet, which is a read-access grant rather than a
		// deletion. It is kept off the public internet by two independent
		// mechanisms: the priority-200 internal-deny rule matching
		// ^/+api/+media[^/]*/*internal (an ipAllowList of 255.255.255.255/32,
		// which no client can match), and media-stripprefix stripping only
		// /api, so a public /api/media/internal/... would arrive here as
		// /media/internal/... and match no registered route.
		// tools/check-manifests.sh asserts the first of those on both
		// entrypoints and runs as part of `make ci`.
		r.Post("/internal/admin/reassign-fleet", func(w http.ResponseWriter, req *http.Request) {
			var body ReassignRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				server.WriteError(w, server.ErrValidation)
				return
			}
			if err := reassignRootFrom(body); err != nil {
				server.WriteError(w, err)
				return
			}
			var affected map[string]int
			if terr := db.Transaction(func(tx *gorm.DB) error {
				var rerr error
				affected, rerr = Reassign(tx, body.MediaIDs, body.DestinationFleetID)
				return rerr
			}); terr != nil {
				log.WithError(terr).WithField("destination_fleet_id", body.DestinationFleetID).
					Error("media admin reassign")
				server.WriteError(w, terr)
				return
			}
			server.WriteJSON(w, http.StatusOK, affectedResponse{Affected: affected})
		})
```

- [ ] **Step 5: Prove the access decision actually flipped**

The whole point of FR-XFER-MEDIA-1 is that the destination fleet can now read
the media and the source fleet cannot. Asserting the column alone would pass
even if the authorisation rule read some other field, so assert the decision
itself. `mediaobject.AuthorizeAccess(m Model, identityFleetID string) error` is
a pure function over a `Model`, and `mediaobject.NewProvider(db).GetByID(id)`
returns one — no processor or HTTP identity is needed.

Append to `apps/media-service/internal/admin/reassign_test.go`:

```go
// The access DECISION follows the reassign, not merely the column.
//
// The refusal is ErrNotFound, not ErrForbidden: AuthorizeAccess answers 404
// deliberately, so cross-fleet EXISTENCE is never leaked. The PRD says 403 in
// FR-XFER-MEDIA-1; the code says 404 and the code is right (design §2.3.1).
func TestReassign_flipsTheAccessDecision(t *testing.T) {
	db := newMediaDB(t)
	srv := reassignServer(t, db)
	provider := mediaobject.NewProvider(db)

	before, err := provider.GetByID("mo-1")
	if err != nil {
		t.Fatalf("load before: %v", err)
	}
	if aerr := mediaobject.AuthorizeAccess(before, "fleet-1"); aerr != nil {
		t.Fatalf("the source fleet could not read its own object before the move: %v", aerr)
	}

	if status, _ := postReassign(t, srv,
		`{"media_ids":["mo-1"],"destination_fleet_id":"fleet-9"}`); status != http.StatusOK {
		t.Fatalf("reassign status = %d", status)
	}

	after, err := provider.GetByID("mo-1")
	if err != nil {
		t.Fatalf("load after: %v", err)
	}
	if aerr := mediaobject.AuthorizeAccess(after, "fleet-9"); aerr != nil {
		t.Errorf("the destination fleet cannot read the object: %v", aerr)
	}
	if aerr := mediaobject.AuthorizeAccess(after, "fleet-1"); !errors.Is(aerr, server.ErrNotFound) {
		t.Errorf("the source fleet still has access: err = %v, want ErrNotFound", aerr)
	}
}
```

Add `"errors"`, `"github.com/jtumidanski/myfleet/apps/media-service/internal/mediaobject"`
and `"github.com/jtumidanski/myfleet/packages/shared-go/server"` to the test
file's imports.

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./apps/media-service/... -v -run 'Reassign|AuthorizeAccess'`
Expected: PASS.

Run: `go test ./apps/media-service/...`
Expected: PASS — in particular `arch_test.go` and `tablenames_test.go`, which this adds no table to.

- [ ] **Step 7: Commit**

```bash
git add apps/media-service/internal/admin/
git commit -m "feat(media-service): idempotent /internal/admin/reassign-fleet route"
```

---

## Task 8: notification-service `/internal/admin/reassign-fleet`

**Files:**
- Create: `apps/notification-service/internal/admin/reassign.go`
- Modify: `apps/notification-service/internal/admin/resource.go`
- Test: `apps/notification-service/internal/admin/reassign_test.go` (create)

**Interfaces:**
- Produces:
  - `type ReassignRequest struct { VehicleIDs []string \`json:"vehicle_ids"\`; DestinationFleetID string \`json:"destination_fleet_id"\` }`
  - `func Reassign(tx *gorm.DB, vehicleIDs []string, destFleetID string) (map[string]int, error)` returning `{"notifications": N}`
  - route `POST /internal/admin/reassign-fleet`

- [ ] **Step 1: Write the failing test**

Create `apps/notification-service/internal/admin/reassign_test.go`:

```go
package admin_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/apps/notification-service/internal/admin"
)

// seedVehicleNotifications adds rows carrying a vehicle_id; newNotificationDB's
// own fixtures deliberately have none, because a purge keys on fleet_id.
func seedVehicleNotifications(t *testing.T, db *gorm.DB) {
	t.Helper()
	stmts := []string{
		`INSERT INTO notification.notifications
		 (id, user_id, type, title, dedupe_key, vehicle_id, fleet_id)
		 VALUES ('nv1', 'user-1', 'schedule.overdue', 'D', 'dk-4', 'veh-1', 'fleet-1')`,
		`INSERT INTO notification.notifications
		 (id, user_id, type, title, dedupe_key, vehicle_id, fleet_id)
		 VALUES ('nv2', 'user-2', 'schedule.overdue', 'E', 'dk-5', 'veh-1', 'fleet-1')`,
		`INSERT INTO notification.notifications
		 (id, user_id, type, title, dedupe_key, vehicle_id, fleet_id)
		 VALUES ('nv3', 'user-1', 'schedule.overdue', 'F', 'dk-6', 'veh-2', 'fleet-1')`,
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
}

func reassignSrv(t *testing.T, db *gorm.DB) *httptest.Server {
	t.Helper()
	r := chi.NewRouter()
	log := logrus.New()
	log.SetOutput(io.Discard)
	admin.InitializeInternalRoutes(log, db)(r)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

func post(t *testing.T, srv *httptest.Server, body string) (int, map[string]int) {
	t.Helper()
	res, err := http.Post(srv.URL+"/internal/admin/reassign-fleet",
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	var decoded struct {
		Affected map[string]int `json:"affected"`
	}
	_ = json.NewDecoder(res.Body).Decode(&decoded)
	return res.StatusCode, decoded.Affected
}

func fleetOfNotification(t *testing.T, db *gorm.DB, id string) string {
	t.Helper()
	var out []string
	if err := db.Raw(`SELECT fleet_id FROM notification.notifications WHERE id = ?`, id).
		Scan(&out).Error; err != nil {
		t.Fatalf("read fleet_id: %v", err)
	}
	if len(out) == 0 {
		t.Fatalf("no notification %s", id)
	}
	return out[0]
}

func TestReassign_repointsNotificationsForTheVehicle(t *testing.T) {
	db := newNotificationDB(t)
	seedVehicleNotifications(t, db)
	srv := reassignSrv(t, db)

	status, affected := post(t, srv, `{"vehicle_ids":["veh-1"],"destination_fleet_id":"fleet-9"}`)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if affected["notifications"] != 2 {
		t.Errorf("affected = %v, want notifications 2", affected)
	}
	for _, id := range []string{"nv1", "nv2"} {
		if got := fleetOfNotification(t, db, id); got != "fleet-9" {
			t.Errorf("%s fleet_id = %q, want fleet-9", id, got)
		}
	}
	// A different vehicle's notification stays put.
	if got := fleetOfNotification(t, db, "nv3"); got != "fleet-1" {
		t.Errorf("nv3 fleet_id = %q, want the untouched fleet-1", got)
	}
	// So does a notification with no vehicle at all.
	if got := fleetOfNotification(t, db, "n1"); got != "fleet-1" {
		t.Errorf("n1 fleet_id = %q, want the untouched fleet-1", got)
	}
}

func TestReassign_replayIsANoOpWithTheSameCount(t *testing.T) {
	db := newNotificationDB(t)
	seedVehicleNotifications(t, db)
	srv := reassignSrv(t, db)
	body := `{"vehicle_ids":["veh-1"],"destination_fleet_id":"fleet-9"}`

	_, first := post(t, srv, body)
	_, second := post(t, srv, body)
	if first["notifications"] != second["notifications"] || second["notifications"] != 2 {
		t.Errorf("first %v, replay %v; both must report notifications 2", first, second)
	}
}

func TestReassign_ignoresUnknownVehicleIDs(t *testing.T) {
	db := newNotificationDB(t)
	seedVehicleNotifications(t, db)
	srv := reassignSrv(t, db)

	status, affected := post(t, srv,
		`{"vehicle_ids":["veh-1","nope"],"destination_fleet_id":"fleet-9"}`)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if affected["notifications"] != 2 {
		t.Errorf("affected = %v, want notifications 2", affected)
	}
}

func TestReassign_rejectsEmptyInput(t *testing.T) {
	db := newNotificationDB(t)
	srv := reassignSrv(t, db)

	for _, body := range []string{
		`{"vehicle_ids":[],"destination_fleet_id":"fleet-9"}`,
		`{"vehicle_ids":["veh-1"],"destination_fleet_id":""}`,
	} {
		if status, _ := post(t, srv, body); status != http.StatusUnprocessableEntity {
			t.Errorf("body %s: status = %d, want 422", body, status)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./apps/notification-service/internal/admin/ -run Reassign -v`
Expected: 404 from the router — the route does not exist.

- [ ] **Step 3: Implement `Reassign`**

Create `apps/notification-service/internal/admin/reassign.go`:

```go
package admin

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// ReassignRequest re-points the fleet_id on notifications about a set of
// vehicles. It is the body of POST /internal/admin/reassign-fleet, issued by
// fleet-service when a platform admin transfers a vehicle between fleets.
//
// It takes VEHICLE ids rather than notification ids because the vehicle ->
// notification relationship is this service's to know, not fleet-service's.
type ReassignRequest struct {
	VehicleIDs         []string `json:"vehicle_ids"`
	DestinationFleetID string   `json:"destination_fleet_id"`
}

// Reassign rewrites notifications.fleet_id for the named vehicles and returns
// the read-back count.
//
// notifications.fleet_id is a stored, denormalised routing column — the same
// stored-not-derived staleness fleet.activity_events has. A stale value breaks
// no read, since notifications are per-user, but it would mis-scope a later
// fleet-scoped admin purge selecting on that column, which is exactly why the
// column is indexed.
//
// Idempotent for the same reason and by the same means as media-service's twin:
// the count is READ BACK, not taken from RowsAffected, so a replay reports the
// same number. Unknown vehicle ids simply do not match. Soft-deleted rows are
// left alone.
func Reassign(tx *gorm.DB, vehicleIDs []string, destFleetID string) (map[string]int, error) {
	upd := `UPDATE notification.notifications SET fleet_id = ?
	         WHERE vehicle_id IN ? AND fleet_id <> ? AND deleted_at IS NULL`
	if err := tx.Exec(upd, destFleetID, vehicleIDs, destFleetID).Error; err != nil {
		return nil, fmt.Errorf("reassign notifications: %w", err)
	}
	var n int64
	cnt := `SELECT count(*) FROM notification.notifications
	         WHERE vehicle_id IN ? AND fleet_id = ? AND deleted_at IS NULL`
	if err := tx.Raw(cnt, vehicleIDs, destFleetID).Scan(&n).Error; err != nil {
		return nil, fmt.Errorf("count reassigned notifications: %w", err)
	}
	return map[string]int{"notifications": int(n)}, nil
}

// reassignRootFrom validates the request body, or returns 422 — the same
// treatment rootFrom gives a purge body.
func reassignRootFrom(body ReassignRequest) error {
	if len(body.VehicleIDs) == 0 {
		return server.Detailed(server.ErrValidation, "vehicle_ids is required")
	}
	if body.DestinationFleetID == "" {
		return server.Detailed(server.ErrValidation, "destination_fleet_id is required")
	}
	return nil
}
```

- [ ] **Step 4: Register the route**

In `apps/notification-service/internal/admin/resource.go`, inside
`InitializeInternalRoutes`, after the `POST /internal/admin/purge` handler:

```go
		// SECURITY: unauthenticated, like its neighbours, and this service is
		// the one that is not safe by accident — notifications-stripprefix
		// strips the FULL /api/notifications prefix, so the priority-200
		// internal-deny rule matching ^/+api/+notifications[^/]*/*internal is
		// the ONLY thing keeping this off the public internet.
		// tools/check-manifests.sh asserts it on both entrypoints and runs as
		// part of `make ci`.
		r.Post("/internal/admin/reassign-fleet", func(w http.ResponseWriter, req *http.Request) {
			var body ReassignRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				server.WriteError(w, server.ErrValidation)
				return
			}
			if err := reassignRootFrom(body); err != nil {
				server.WriteError(w, err)
				return
			}
			var affected map[string]int
			if terr := db.Transaction(func(tx *gorm.DB) error {
				var rerr error
				affected, rerr = Reassign(tx, body.VehicleIDs, body.DestinationFleetID)
				return rerr
			}); terr != nil {
				log.WithError(terr).WithField("destination_fleet_id", body.DestinationFleetID).
					Error("notification admin reassign")
				server.WriteError(w, terr)
				return
			}
			server.WriteJSON(w, http.StatusOK, affectedResponse{Affected: affected})
		})
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./apps/notification-service/... -v -run Reassign`
Expected: PASS (4 tests).

Run: `go test ./apps/notification-service/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add apps/notification-service/internal/admin/
git commit -m "feat(notification-service): idempotent /internal/admin/reassign-fleet route"
```

---

## Task 9: The transfer processor — eligibility, confirmation, transaction, compensation, audit

**Files:**
- Create: `apps/fleet-service/internal/admin/transfer_processor.go`
- Modify: `apps/fleet-service/internal/admin/processor.go` (two new `Deps` fields + ports)
- Test: `apps/fleet-service/internal/admin/transfer_processor_test.go` (create)

**Interfaces:**
- Consumes: `ApplyTransfer`, `CountTransfer`, `PreviewCategories`, `VehicleMediaIDs`, `WidgetsPinnedToVehicle` (Tasks 2–5); `MatchConfirmation`, `Administrator.InsertAudit`, `AuthVerifier` (existing).
- Produces:
  - `type MediaReassigner interface { Reassign(ctx context.Context, mediaIDs []string, destFleetID string) (map[string]int, error) }`
  - `type NotificationReassigner interface { Reassign(ctx context.Context, vehicleIDs []string, destFleetID string) (map[string]int, error) }`
  - `Deps.MediaReassign MediaReassigner`, `Deps.NotificationReassign NotificationReassigner`
  - `type TransferPreview struct { VehicleID, VehicleLabel, SourceFleetID, SourceFleetName, DestinationFleetID, DestinationFleetName string; Counts map[string]int; CategoriesToCreate []CategoryToCreate; Warnings []string }`
  - `type TransferInput struct { VehicleID, DestFleetID, Confirmation, ActorUserID, ActorEmail, CorrelationID string }`
  - `type TransferResult struct { VehicleID, SourceFleetID, DestinationFleetID string; TransferredAt time.Time; AffectedCounts map[string]int }`
  - `func (p *Processor) PreviewVehicleTransfer(ctx context.Context, vehicleID, destFleetID string) (TransferPreview, error)`
  - `func (p *Processor) TransferVehicle(ctx context.Context, in TransferInput) (TransferResult, error)`

**Planning decision — `notifications` is absent from the preview.** The preview
makes no downstream call (design D7), and unlike `media_objects` there is no
fleet-service-side proxy for the notification count: notification-service owns
the vehicle → notification relationship entirely. Reporting a number we cannot
compute, or calling a second service on a read path the operator may hit on
every keystroke, are both worse than omitting it. `notifications` appears in the
transfer's `affected_counts` only, and the dialog does not list it.

- [ ] **Step 1: Write the failing test**

Create `apps/fleet-service/internal/admin/transfer_processor_test.go`:

```go
package admin_test

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin/admintest"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// fakeReassigner records every call so a test can assert a downstream was NOT
// reached — never merely "not yet", which is what task-019 is about.
type fakeReassigner struct {
	calls [][]string
	dests []string
	ret   map[string]int
	err   error
}

func (f *fakeReassigner) Reassign(_ context.Context, ids []string, dest string) (map[string]int, error) {
	f.calls = append(f.calls, ids)
	f.dests = append(f.dests, dest)
	if f.err != nil {
		return nil, f.err
	}
	return f.ret, nil
}

type fakeAuth struct {
	ok  bool
	err error
}

func (f fakeAuth) IsPlatformAdmin(context.Context, string) (bool, error) { return f.ok, f.err }

type transferHarness struct {
	db    *gorm.DB
	proc  *admin.Processor
	media *fakeReassigner
	notif *fakeReassigner
	src   admintest.Fixture
	dst   admintest.Fixture
}

func newTransferHarness(t *testing.T) transferHarness {
	t.Helper()
	db := admintest.NewDB(t)
	src := admintest.SeedFleet(t, db, "fleet-a")
	dst := admintest.SeedFleet(t, db, "fleet-b")
	media := &fakeReassigner{ret: map[string]int{"media_objects": 1}}
	notif := &fakeReassigner{ret: map[string]int{"notifications": 2}}

	log := logrus.New()
	log.SetOutput(io.Discard)
	proc := admin.NewProcessor(log, admin.Deps{
		DB:                   db,
		Provider:             admin.NewProvider(db),
		Administrator:        admin.NewAdministrator(db),
		Auth:                 fakeAuth{ok: true},
		MediaReassign:        media,
		NotificationReassign: notif,
		Now:                  func() time.Time { return time.Date(2026, 8, 25, 18, 0, 0, 0, time.UTC) },
	}, admin.NewTargetResolver(db))

	return transferHarness{db: db, proc: proc, media: media, notif: notif, src: src, dst: dst}
}

func (h transferHarness) input(confirmation string) admin.TransferInput {
	return admin.TransferInput{
		VehicleID:     h.src.VehicleID,
		DestFleetID:   h.dst.FleetID,
		Confirmation:  confirmation,
		ActorUserID:   "admin-1",
		ActorEmail:    "admin@example.com",
		CorrelationID: "corr-1",
	}
}

// SeedFleet's vehicle has no nickname, so the label is "{year} {make} {model}".
const seededLabel = "2020 Toyota Corolla"

func TestTransferVehicle_happyPath(t *testing.T) {
	h := newTransferHarness(t)

	res, err := h.proc.TransferVehicle(context.Background(), h.input(seededLabel))
	if err != nil {
		t.Fatalf("transfer: %v", err)
	}
	if res.SourceFleetID != "fleet-a" || res.DestinationFleetID != "fleet-b" {
		t.Errorf("result fleets = %s -> %s", res.SourceFleetID, res.DestinationFleetID)
	}
	if got := scanOne[string](t, h.db, `SELECT fleet_id FROM fleet.vehicles WHERE id = ?`,
		h.src.VehicleID); got != "fleet-b" {
		t.Errorf("fleet_id = %q, want fleet-b", got)
	}
	// Downstream counts are merged into affected_counts (FR-XFER-AUDIT-3).
	if res.AffectedCounts["media_objects"] != 1 {
		t.Errorf("media_objects = %d, want 1", res.AffectedCounts["media_objects"])
	}
	if res.AffectedCounts["notifications"] != 2 {
		t.Errorf("notifications = %d, want 2", res.AffectedCounts["notifications"])
	}
	if len(h.media.calls) != 1 || h.media.dests[0] != "fleet-b" {
		t.Errorf("media calls = %v to %v", h.media.calls, h.media.dests)
	}
	if len(h.notif.calls) != 1 || h.notif.calls[0][0] != h.src.VehicleID {
		t.Errorf("notification call = %v, want the vehicle id", h.notif.calls)
	}
}

func TestTransferVehicle_writesTheAuditRow(t *testing.T) {
	h := newTransferHarness(t)
	if _, err := h.proc.TransferVehicle(context.Background(), h.input(seededLabel)); err != nil {
		t.Fatalf("transfer: %v", err)
	}

	events, total, err := h.proc.ListAuditEvents(
		admin.AuditFilter{Action: admin.ActionVehicleTransferred}, server.Page{Number: 1, Size: 10})
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if total != 1 {
		t.Fatalf("audit rows = %d, want 1", total)
	}
	a := events[0]
	if a.ActorUserID != "admin-1" || a.ActorEmail != "admin@example.com" {
		t.Errorf("actor = %s/%s", a.ActorUserID, a.ActorEmail)
	}
	if a.TargetType != "vehicle" || a.TargetID != h.src.VehicleID {
		t.Errorf("target = %s/%s", a.TargetType, a.TargetID)
	}
	if a.TargetLabel != seededLabel {
		t.Errorf("target_label = %q, want %q", a.TargetLabel, seededLabel)
	}
	if a.CorrelationID != "corr-1" {
		t.Errorf("correlation_id = %q", a.CorrelationID)
	}
	if a.SourceFleetID != "fleet-a" || a.DestinationFleetID != "fleet-b" {
		t.Errorf("audit fleets = %s -> %s", a.SourceFleetID, a.DestinationFleetID)
	}
	if a.PurgeOperationID != "" {
		t.Errorf("purge_operation_id = %q, want empty — a transfer is not a purge", a.PurgeOperationID)
	}
	for _, k := range []string{
		"maintenance_records", "maintenance_schedules", "fuel_logs", "mileage_records",
		"vehicle_media", "media_objects", "notifications", "activity_events",
		"categories_created", "widgets_removed",
	} {
		if _, ok := a.AffectedCounts[k]; !ok {
			t.Errorf("affected_counts is missing %q (FR-XFER-AUDIT-3)", k)
		}
	}
}

// FR-XFER-CONF-3, asserted as three NEGATIVES: nothing local changed, no audit
// row exists, and neither downstream was called at all.
func TestTransferVehicle_confirmationMismatchWritesNothing(t *testing.T) {
	h := newTransferHarness(t)

	_, err := h.proc.TransferVehicle(context.Background(), h.input("2020 toyota corolla"))
	if !errors.Is(err, server.ErrConflict) {
		t.Fatalf("err = %v, want a 409 conflict", err)
	}
	if got := scanOne[string](t, h.db, `SELECT fleet_id FROM fleet.vehicles WHERE id = ?`,
		h.src.VehicleID); got != "fleet-a" {
		t.Errorf("fleet_id = %q, want the untouched fleet-a", got)
	}
	if n := scanOne[int](t, h.db, `SELECT count(*) FROM fleet.admin_audit_events`); n != 0 {
		t.Errorf("audit rows = %d, want 0", n)
	}
	if len(h.media.calls) != 0 {
		t.Errorf("media was called %d times, want 0", len(h.media.calls))
	}
	if len(h.notif.calls) != 0 {
		t.Errorf("notification was called %d times, want 0", len(h.notif.calls))
	}
}

func TestTransferVehicle_revokedAdminIsForbidden(t *testing.T) {
	h := newTransferHarness(t)
	log := logrus.New()
	log.SetOutput(io.Discard)
	proc := admin.NewProcessor(log, admin.Deps{
		DB: h.db, Provider: admin.NewProvider(h.db), Administrator: admin.NewAdministrator(h.db),
		Auth: fakeAuth{ok: false}, MediaReassign: h.media, NotificationReassign: h.notif,
	}, admin.NewTargetResolver(h.db))

	if _, err := proc.TransferVehicle(context.Background(), h.input(seededLabel)); !errors.Is(err, server.ErrForbidden) {
		t.Fatalf("err = %v, want forbidden", err)
	}
	if len(h.media.calls) != 0 {
		t.Error("media was called for a revoked admin")
	}
}

func TestTransferVehicle_eligibilityBranches(t *testing.T) {
	cases := []struct {
		name    string
		setup   func(t *testing.T, h transferHarness) admin.TransferInput
		wantErr error
	}{
		{
			name: "unknown vehicle",
			setup: func(_ *testing.T, h transferHarness) admin.TransferInput {
				in := h.input(seededLabel)
				in.VehicleID = "no-such-vehicle"
				return in
			},
			wantErr: server.ErrNotFound,
		},
		{
			name: "vehicle pending purge",
			setup: func(t *testing.T, h transferHarness) admin.TransferInput {
				if err := h.db.Exec(`UPDATE fleet.vehicles SET deleted_at = ?, purge_operation_id = 'op-1' WHERE id = ?`,
					seedNow(), h.src.VehicleID).Error; err != nil {
					t.Fatalf("stamp vehicle: %v", err)
				}
				return h.input(seededLabel)
			},
			wantErr: server.ErrConflict,
		},
		{
			name: "source fleet pending purge",
			setup: func(t *testing.T, h transferHarness) admin.TransferInput {
				if err := h.db.Exec(`UPDATE fleet.fleets SET deleted_at = ?, purge_operation_id = 'op-1' WHERE id = 'fleet-a'`,
					seedNow()).Error; err != nil {
					t.Fatalf("stamp source fleet: %v", err)
				}
				return h.input(seededLabel)
			},
			wantErr: server.ErrConflict,
		},
		{
			name: "destination unavailable",
			setup: func(t *testing.T, h transferHarness) admin.TransferInput {
				if err := h.db.Exec(`UPDATE fleet.fleets SET deleted_at = ? WHERE id = 'fleet-b'`,
					seedNow()).Error; err != nil {
					t.Fatalf("delete destination: %v", err)
				}
				return h.input(seededLabel)
			},
			wantErr: server.ErrConflict,
		},
		{
			name: "unknown destination",
			setup: func(_ *testing.T, h transferHarness) admin.TransferInput {
				in := h.input(seededLabel)
				in.DestFleetID = "no-such-fleet"
				return in
			},
			wantErr: server.ErrNotFound,
		},
		{
			name: "destination equals current fleet",
			setup: func(_ *testing.T, h transferHarness) admin.TransferInput {
				in := h.input(seededLabel)
				in.DestFleetID = "fleet-a"
				return in
			},
			wantErr: server.ErrValidation,
		},
		{
			name: "destination missing",
			setup: func(_ *testing.T, h transferHarness) admin.TransferInput {
				in := h.input(seededLabel)
				in.DestFleetID = ""
				return in
			},
			wantErr: server.ErrValidation,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newTransferHarness(t)
			in := tc.setup(t, h)

			_, err := h.proc.TransferVehicle(context.Background(), in)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			var detailed interface{ Detail() string }
			if errors.As(err, &detailed) && detailed.Detail() == "" {
				t.Error("error carries an empty detail; FR-XFER-UI-7 surfaces it verbatim")
			}
			if len(h.media.calls) != 0 || len(h.notif.calls) != 0 {
				t.Error("a downstream was called for an ineligible transfer")
			}
		})
	}
}

// FR-XFER-MEDIA-5, with 503 rather than 502 (design D2): a media failure rolls
// the whole transfer back.
func TestTransferVehicle_mediaFailureRollsBackCompletely(t *testing.T) {
	h := newTransferHarness(t)
	h.media.err = errors.New("media-service is down")

	_, err := h.proc.TransferVehicle(context.Background(), h.input(seededLabel))
	if !errors.Is(err, server.ErrServiceUnavailable) {
		t.Fatalf("err = %v, want service unavailable", err)
	}
	if got := scanOne[string](t, h.db, `SELECT fleet_id FROM fleet.vehicles WHERE id = ?`,
		h.src.VehicleID); got != "fleet-a" {
		t.Errorf("fleet_id = %q, want the rolled-back fleet-a", got)
	}
	if n := scanOne[int](t, h.db, `SELECT count(*) FROM fleet.admin_audit_events`); n != 0 {
		t.Errorf("audit rows = %d, want 0", n)
	}
	if n := scanOne[int](t, h.db, `SELECT count(*) FROM fleet.activity_events WHERE type = ?`,
		admin.EventVehicleTransferredIn); n != 0 {
		t.Error("a transfer activity event survived the rollback")
	}
	if len(h.notif.calls) != 0 {
		t.Error("notification-service was called after media-service failed")
	}
}

// If notification-service fails after media-service succeeded, the transaction
// rolls back AND the media move is reversed, so both sides end up as they were.
func TestTransferVehicle_notificationFailureCompensatesTheMediaMove(t *testing.T) {
	h := newTransferHarness(t)
	h.notif.err = errors.New("notification-service is down")

	_, err := h.proc.TransferVehicle(context.Background(), h.input(seededLabel))
	if !errors.Is(err, server.ErrServiceUnavailable) {
		t.Fatalf("err = %v, want service unavailable", err)
	}
	if got := scanOne[string](t, h.db, `SELECT fleet_id FROM fleet.vehicles WHERE id = ?`,
		h.src.VehicleID); got != "fleet-a" {
		t.Errorf("fleet_id = %q, want the rolled-back fleet-a", got)
	}
	if len(h.media.calls) != 2 {
		t.Fatalf("media calls = %d, want 2 (the move and its reversal)", len(h.media.calls))
	}
	if h.media.dests[1] != "fleet-a" {
		t.Errorf("compensating call sent destination %q, want the SOURCE fleet-a", h.media.dests[1])
	}
}

// A vehicle with no media must not send an empty media_ids list, which the
// downstream answers 422 to — a request that would read as a failed service.
func TestTransferVehicle_skipsMediaCallWhenThereIsNoMedia(t *testing.T) {
	h := newTransferHarness(t)
	if err := h.db.Exec(`DELETE FROM fleet.vehicle_media WHERE vehicle_id = ?`, h.src.VehicleID).
		Error; err != nil {
		t.Fatalf("clear vehicle media: %v", err)
	}
	if err := h.db.Exec(`DELETE FROM fleet.maintenance_record_documents`).Error; err != nil {
		t.Fatalf("clear documents: %v", err)
	}

	res, err := h.proc.TransferVehicle(context.Background(), h.input(seededLabel))
	if err != nil {
		t.Fatalf("transfer: %v", err)
	}
	if len(h.media.calls) != 0 {
		t.Errorf("media was called with an empty id set: %v", h.media.calls)
	}
	if res.AffectedCounts["media_objects"] != 0 {
		t.Errorf("media_objects = %d, want 0", res.AffectedCounts["media_objects"])
	}
}

// FR-XFER-CONF-2 and the preview-parity acceptance criterion.
func TestPreviewVehicleTransfer_returnsLabelCountsAndCategories(t *testing.T) {
	h := newTransferHarness(t)
	seedCustomCategory(t, h.db, h.src, "cat-winter", "Winter Tires", "maintenance")
	addWidget(t, h.db, "w-pinned", h.src.DashboardID, `{"vehicleId":"`+h.src.VehicleID+`"}`)

	pv, err := h.proc.PreviewVehicleTransfer(context.Background(), h.src.VehicleID, h.dst.FleetID)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if pv.VehicleLabel != seededLabel {
		t.Errorf("vehicle_label = %q, want %q", pv.VehicleLabel, seededLabel)
	}
	if pv.SourceFleetID != "fleet-a" || pv.SourceFleetName == "" {
		t.Errorf("source = %s/%q", pv.SourceFleetID, pv.SourceFleetName)
	}
	if pv.DestinationFleetID != "fleet-b" || pv.DestinationFleetName == "" {
		t.Errorf("destination = %s/%q", pv.DestinationFleetID, pv.DestinationFleetName)
	}
	if pv.Counts["widgets_removed"] != 1 {
		t.Errorf("widgets_removed = %d, want 1", pv.Counts["widgets_removed"])
	}
	if pv.Counts["media_objects"] != 1 {
		t.Errorf("media_objects = %d, want 1", pv.Counts["media_objects"])
	}
	if len(pv.CategoriesToCreate) != 1 || pv.CategoriesToCreate[0].Name != "Winter Tires" {
		t.Errorf("categories_to_create = %+v", pv.CategoriesToCreate)
	}
	if pv.Warnings == nil {
		t.Error("warnings must be an empty slice, not nil — it is serialised as [] not null")
	}

	// The preview writes nothing.
	if got := scanOne[string](t, h.db, `SELECT fleet_id FROM fleet.vehicles WHERE id = ?`,
		h.src.VehicleID); got != "fleet-a" {
		t.Error("the preview moved the vehicle")
	}
}

// The preview's counts must equal what the transfer then reports, for every key
// the preview produces.
func TestPreviewVehicleTransfer_countsMatchTheAppliedTransfer(t *testing.T) {
	h := newTransferHarness(t)
	seedCustomCategory(t, h.db, h.src, "cat-winter", "Winter Tires", "maintenance")
	addWidget(t, h.db, "w-pinned", h.src.DashboardID, `{"vehicleId":"`+h.src.VehicleID+`"}`)

	pv, err := h.proc.PreviewVehicleTransfer(context.Background(), h.src.VehicleID, h.dst.FleetID)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	res, err := h.proc.TransferVehicle(context.Background(), h.input(seededLabel))
	if err != nil {
		t.Fatalf("transfer: %v", err)
	}
	for k, want := range pv.Counts {
		if res.AffectedCounts[k] != want {
			t.Errorf("%s: preview %d, applied %d", k, want, res.AffectedCounts[k])
		}
	}
}

// Without a destination the preview still answers, omitting the destination
// fields and categories_to_create, which cannot be computed without one.
func TestPreviewVehicleTransfer_withoutADestination(t *testing.T) {
	h := newTransferHarness(t)

	pv, err := h.proc.PreviewVehicleTransfer(context.Background(), h.src.VehicleID, "")
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if pv.DestinationFleetID != "" || pv.DestinationFleetName != "" {
		t.Errorf("destination = %s/%q, want empty", pv.DestinationFleetID, pv.DestinationFleetName)
	}
	if len(pv.CategoriesToCreate) != 0 {
		t.Errorf("categories_to_create = %+v, want empty without a destination", pv.CategoriesToCreate)
	}
	if pv.Counts["widgets_removed"] != 0 {
		t.Errorf("widgets_removed = %d, want 0", pv.Counts["widgets_removed"])
	}
}

func TestPreviewVehicleTransfer_unknownVehicleIsNotFound(t *testing.T) {
	h := newTransferHarness(t)
	if _, err := h.proc.PreviewVehicleTransfer(context.Background(), "nope", h.dst.FleetID); !errors.Is(err, server.ErrNotFound) {
		t.Fatalf("err = %v, want not found", err)
	}
}

// A nickname wins over the year/make/model fallback (FR-XFER-CONF-2).
func TestPreviewVehicleTransfer_prefersTheNickname(t *testing.T) {
	h := newTransferHarness(t)
	if err := h.db.Exec(`UPDATE fleet.vehicles SET nickname = 'The Green Bean' WHERE id = ?`,
		h.src.VehicleID).Error; err != nil {
		t.Fatalf("set nickname: %v", err)
	}
	pv, err := h.proc.PreviewVehicleTransfer(context.Background(), h.src.VehicleID, h.dst.FleetID)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if pv.VehicleLabel != "The Green Bean" {
		t.Errorf("vehicle_label = %q, want The Green Bean", pv.VehicleLabel)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./apps/fleet-service/internal/admin/ -run 'TransferVehicle|PreviewVehicleTransfer' -v`
Expected: compile failure — `unknown field MediaReassign in admin.Deps`.

- [ ] **Step 3: Add the two ports to `Deps`**

In `apps/fleet-service/internal/admin/processor.go`, after the `Downstream` interface:

```go
// MediaReassigner re-homes a set of MEDIA objects to another fleet. Declared
// here as a port rather than importing the concrete client, matching how
// Downstream and AuthVerifier are declared, so the processor stays testable
// without an HTTP server. *adminclient.MediaClient satisfies it.
type MediaReassigner interface {
	Reassign(ctx context.Context, mediaIDs []string, destFleetID string) (map[string]int, error)
}

// NotificationReassigner re-points notifications for a set of VEHICLES.
//
// Vehicle ids, not notification ids: notification-service owns the
// vehicle -> notification relationship, and enumerating it here would mean
// fleet-service reading another service's rows.
// *adminclient.NotificationClient satisfies it.
type NotificationReassigner interface {
	Reassign(ctx context.Context, vehicleIDs []string, destFleetID string) (map[string]int, error)
}
```

and add to `Deps`, after `StatsSources`:

```go
	// MediaReassign and NotificationReassign are the two downstream halves of a
	// vehicle transfer. They are separate from Downstream because the protocols
	// differ: a purge fans out the same PurgeRequest to every service, while a
	// transfer sends media-service media ids and notification-service vehicle
	// ids. Nil disables the corresponding call, which is what the purge-only
	// tests rely on.
	MediaReassign        MediaReassigner
	NotificationReassign NotificationReassigner
```

- [ ] **Step 4: Write `transfer_processor.go`**

Create `apps/fleet-service/internal/admin/transfer_processor.go`:

```go
package admin

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// TransferPreview is the blast radius of a transfer that has not happened.
type TransferPreview struct {
	VehicleID            string
	VehicleLabel         string
	SourceFleetID        string
	SourceFleetName      string
	DestinationFleetID   string
	DestinationFleetName string
	Counts               map[string]int
	CategoriesToCreate   []CategoryToCreate
	// Warnings carries degradation notes, in the same spirit as the admin list
	// endpoints. It is always non-nil so it serialises as [] rather than null.
	Warnings []string
}

// TransferInput is the validated request body plus the caller's identity.
type TransferInput struct {
	VehicleID     string
	DestFleetID   string
	Confirmation  string
	ActorUserID   string
	ActorEmail    string
	CorrelationID string
}

// TransferResult is what the endpoint returns on success.
type TransferResult struct {
	VehicleID          string
	SourceFleetID      string
	DestinationFleetID string
	TransferredAt      time.Time
	AffectedCounts     map[string]int
}

// Conflict and validation errors, each carrying the sentence FR-XFER-UI-7
// surfaces verbatim in the console.
var (
	errVehiclePendingPurge = server.Detailed(server.ErrConflict,
		"vehicle is pending purge and cannot be transferred")
	errSourcePendingPurge = server.Detailed(server.ErrConflict,
		"source fleet is pending purge and cannot be transferred from")
	errDestinationUnavailable = server.Detailed(server.ErrConflict,
		"destination fleet is not available")
	errSameFleet = server.Detailed(server.ErrValidation,
		"vehicle already belongs to that fleet")
	errDestinationRequired = server.Detailed(server.ErrValidation,
		"destination_fleet_id is required")
)

// transferVehicleRow is the slice of fleet.vehicles the transfer reads.
type transferVehicleRow struct {
	ID        string
	FleetID   string
	Nickname  string
	Make      string
	Model     string
	Year      int
	DeletedAt *time.Time
}

// label is the confirmation phrase (FR-XFER-CONF-2): the nickname when there is
// one, otherwise "{year} {make} {model}".
//
// It is computed ONCE, server-side, and returned by the preview, so the console
// echoes the server's string rather than deriving its own. Two independent
// derivations of a phrase that must match exactly is a bug waiting for the
// first vehicle with a double space in its model name.
func (v transferVehicleRow) label() string {
	if v.Nickname != "" {
		return v.Nickname
	}
	return strconv.Itoa(v.Year) + " " + v.Make + " " + v.Model
}

// transferFleetRow is the slice of fleet.fleets the eligibility checks read.
type transferFleetRow struct {
	ID               string
	Name             string
	DeletedAt        *time.Time
	PurgeOperationID *string
}

// pendingPurge mirrors the console's own definition: admin-STAMPED, not merely
// deleted. A fleet a user deleted is a different state.
func (f transferFleetRow) pendingPurge() bool {
	return f.DeletedAt != nil && f.PurgeOperationID != nil
}

func loadTransferVehicle(db *gorm.DB, vehicleID string) (transferVehicleRow, error) {
	var rows []transferVehicleRow
	q := `SELECT id, fleet_id, nickname, make, model, year, deleted_at
	        FROM fleet.vehicles WHERE id = ?`
	if err := db.Raw(q, vehicleID).Scan(&rows).Error; err != nil {
		return transferVehicleRow{}, fmt.Errorf("load vehicle: %w", err)
	}
	if len(rows) == 0 {
		return transferVehicleRow{}, server.ErrNotFound
	}
	return rows[0], nil
}

func loadTransferFleet(db *gorm.DB, fleetID string) (transferFleetRow, error) {
	var rows []transferFleetRow
	q := `SELECT id, name, deleted_at, purge_operation_id FROM fleet.fleets WHERE id = ?`
	if err := db.Raw(q, fleetID).Scan(&rows).Error; err != nil {
		return transferFleetRow{}, fmt.Errorf("load fleet: %w", err)
	}
	if len(rows) == 0 {
		return transferFleetRow{}, server.ErrNotFound
	}
	return rows[0], nil
}

// lockVehicle serialises concurrent transfers of the same vehicle. It is the
// FIRST statement in the transaction, so every check and write after it is
// ordered per vehicle rather than interleaved.
//
// The dialect branch is on a LOCKING CLAUSE, not on a predicate: the rows
// selected are identical on both dialects, so the tested path and the
// production path still agree about WHAT they touch. SQLite has no FOR UPDATE
// and needs none — the harness holds a single connection and serialises whole
// transactions anyway.
func lockVehicle(tx *gorm.DB, vehicleID string) error {
	q := `SELECT id FROM fleet.vehicles WHERE id = ? FOR UPDATE`
	if tx.Name() == "sqlite" {
		q = `SELECT id FROM fleet.vehicles WHERE id = ?`
	}
	var ids []string
	if err := tx.Raw(q, vehicleID).Scan(&ids).Error; err != nil {
		return fmt.Errorf("lock vehicle: %w", err)
	}
	return nil
}

// PreviewVehicleTransfer computes the blast radius without writing anything and
// without calling any other service.
//
// destFleetID is optional. Without it the destination fields and
// categories_to_create are omitted, because neither can be computed without
// knowing where the car is going — while widgets_removed, a SOURCE-fleet fact,
// is always available.
//
// counts.media_objects is the size of the vehicle's media-id union: "media
// references held by this vehicle". The applied transfer reports
// media-service's read-back instead, and the two agree whenever every reference
// resolves — which is the normal case. They diverge only for a PRE-EXISTING
// dangling reference, which the transfer surfaces rather than causes, and the
// handler logs the difference (design D7).
func (p *Processor) PreviewVehicleTransfer(ctx context.Context, vehicleID, destFleetID string) (TransferPreview, error) {
	_ = ctx
	v, err := loadTransferVehicle(p.d.DB, vehicleID)
	if err != nil {
		return TransferPreview{}, err
	}
	src, err := loadTransferFleet(p.d.DB, v.FleetID)
	if err != nil {
		return TransferPreview{}, err
	}

	counts, err := CountTransfer(p.d.DB, vehicleID)
	if err != nil {
		return TransferPreview{}, err
	}
	mediaIDs, err := VehicleMediaIDs(p.d.DB, vehicleID)
	if err != nil {
		return TransferPreview{}, err
	}
	counts["media_objects"] = len(mediaIDs)

	widgetIDs, err := WidgetsPinnedToVehicle(p.d.DB, v.FleetID, vehicleID)
	if err != nil {
		return TransferPreview{}, err
	}
	counts["widgets_removed"] = len(widgetIDs)

	out := TransferPreview{
		VehicleID:          v.ID,
		VehicleLabel:       v.label(),
		SourceFleetID:      src.ID,
		SourceFleetName:    src.Name,
		Counts:             counts,
		CategoriesToCreate: []CategoryToCreate{},
		Warnings:           []string{},
	}
	if destFleetID == "" {
		counts["categories_created"] = 0
		return out, nil
	}

	dst, err := loadTransferFleet(p.d.DB, destFleetID)
	if err != nil {
		return TransferPreview{}, err
	}
	out.DestinationFleetID = dst.ID
	out.DestinationFleetName = dst.Name

	spec := TransferSpec{VehicleID: v.ID, SourceFleetID: v.FleetID, DestFleetID: destFleetID}
	toCreate, err := PreviewCategories(p.d.DB, spec)
	if err != nil {
		return TransferPreview{}, err
	}
	out.CategoriesToCreate = toCreate
	counts["categories_created"] = len(toCreate)
	return out, nil
}

// TransferVehicle moves one vehicle, with its history, to another fleet.
//
// Structure (design D4):
//
//  1. Re-verify platform admin against auth-service, FAIL CLOSED — this is an
//     irreversible-ish write, so it gets the same treatment purge Create does.
//  2. ONE transaction. Lock the vehicle FIRST, then run every eligibility and
//     confirmation check, then every local write, then the two downstream calls,
//     then the audit row.
//  3. Any failure inside the transaction rolls back all of it. If a downstream
//     had already succeeded, compensate it with a reverse Reassign — safe
//     because both reassign endpoints are idempotent.
//
// The downstream calls are made LAST so that a local failure short-circuits
// before either is issued, which is what turns compensation from the common
// path into a rare one. The audit row goes after them, not before, because
// FR-XFER-AUDIT-3 requires media_objects and notifications inside
// affected_counts and those numbers only exist once the calls have returned.
//
// This does hold a database transaction open across two HTTP calls. That is
// normally an anti-pattern; here it is bounded by adminclient's 5s timeout, the
// operation is platform-admin-only and rare, and the alternative ordering
// leaves a concurrency gap the vehicle lock exists to close. If it ever becomes
// a problem the fix is a queue, not a reordering.
func (p *Processor) TransferVehicle(ctx context.Context, in TransferInput) (TransferResult, error) {
	now := p.d.Now()

	ok, err := p.d.Auth.IsPlatformAdmin(ctx, in.ActorUserID)
	if err != nil {
		p.log.WithError(err).WithField("actor", in.ActorUserID).
			Error("platform-admin re-verification failed; refusing the transfer")
		return TransferResult{}, err
	}
	if !ok {
		return TransferResult{}, server.ErrForbidden
	}
	if in.DestFleetID == "" {
		return TransferResult{}, errDestinationRequired
	}

	var (
		spec         TransferSpec
		counts       map[string]int
		mediaIDs     []string
		mediaApplied bool
		notifApplied bool
	)

	terr := p.d.DB.Transaction(func(tx *gorm.DB) error {
		if err := lockVehicle(tx, in.VehicleID); err != nil {
			return err
		}

		// Cheapest and most specific first, so the operator gets the most
		// actionable message. Confirmation is LAST: a mismatched phrase on an
		// otherwise-invalid request should report the real problem.
		v, err := loadTransferVehicle(tx, in.VehicleID)
		if err != nil {
			return err
		}
		if v.DeletedAt != nil {
			return errVehiclePendingPurge
		}
		if in.DestFleetID == v.FleetID {
			return errSameFleet
		}
		src, err := loadTransferFleet(tx, v.FleetID)
		if err != nil {
			return err
		}
		if src.pendingPurge() {
			// Refused because the outcome would depend on whether the reaper
			// runs before or after the move (FR-XFER-ELIG-5).
			return errSourcePendingPurge
		}
		dst, err := loadTransferFleet(tx, in.DestFleetID)
		if err != nil {
			return err
		}
		if dst.DeletedAt != nil {
			return errDestinationUnavailable
		}
		if err := MatchConfirmation(ScopeFleet, v.label(), in.Confirmation); err != nil {
			// ScopeFleet is passed for its COMPARISON RULE — exact, no trimming,
			// no case folding — not because a transfer is a fleet purge. Adding
			// a ScopeVehicleTransfer would leak into ValidScopes and the purge
			// builder for no gain.
			return err
		}

		spec = TransferSpec{
			VehicleID:     v.ID,
			SourceFleetID: v.FleetID,
			DestFleetID:   in.DestFleetID,
			Label:         v.label(),
			ActorUserID:   in.ActorUserID,
			Now:           now,
		}

		if mediaIDs, err = VehicleMediaIDs(tx, v.ID); err != nil {
			return err
		}
		if counts, err = ApplyTransfer(tx, spec); err != nil {
			return err
		}

		// A vehicle with no media must not send an empty list: both reassign
		// endpoints answer 422 to one, which would read as a failed service.
		counts["media_objects"] = 0
		if p.d.MediaReassign != nil && len(mediaIDs) > 0 {
			got, merr := p.d.MediaReassign.Reassign(ctx, mediaIDs, spec.DestFleetID)
			if merr != nil {
				p.log.WithError(merr).WithFields(logrus.Fields{
					"vehicle_id": spec.VehicleID, "correlation_id": in.CorrelationID,
				}).Error("media reassign failed; rolling the transfer back")
				return server.Detailed(server.ErrServiceUnavailable,
					"media-service could not reassign the vehicle's media; the transfer was rolled back")
			}
			mediaApplied = true
			counts["media_objects"] = got["media_objects"]
			if got["media_objects"] != len(mediaIDs) {
				// A pre-existing dangling reference, surfaced rather than
				// hidden: the preview counted references, media-service counted
				// objects that exist.
				p.log.WithFields(logrus.Fields{
					"vehicle_id": spec.VehicleID,
					"references": len(mediaIDs),
					"objects":    got["media_objects"],
				}).Info("vehicle media references exceed the media objects that exist")
			}
		}

		counts["notifications"] = 0
		if p.d.NotificationReassign != nil {
			got, nerr := p.d.NotificationReassign.Reassign(ctx, []string{spec.VehicleID}, spec.DestFleetID)
			if nerr != nil {
				p.log.WithError(nerr).WithFields(logrus.Fields{
					"vehicle_id": spec.VehicleID, "correlation_id": in.CorrelationID,
				}).Error("notification reassign failed; rolling the transfer back")
				return server.Detailed(server.ErrServiceUnavailable,
					"notification-service could not reassign the vehicle's notifications; the transfer was rolled back")
			}
			notifApplied = true
			counts["notifications"] = got["notifications"]
		}

		return p.d.Administrator.InsertAudit(tx, AuditEvent{
			ID:                 uuid.NewString(),
			ActorUserID:        in.ActorUserID,
			ActorEmail:         in.ActorEmail,
			Action:             ActionVehicleTransferred,
			TargetType:         "vehicle",
			TargetID:           spec.VehicleID,
			TargetLabel:        spec.Label,
			SourceFleetID:      spec.SourceFleetID,
			DestinationFleetID: spec.DestFleetID,
			AffectedCounts:     counts,
			CorrelationID:      in.CorrelationID,
			CreatedAt:          now,
		})
	})

	if terr != nil {
		// Reached either because a downstream failed after its predecessor
		// succeeded, or because the COMMIT itself failed after both did. Both
		// are the same repair: put the downstreams back the way they were.
		p.compensate(ctx, spec, mediaIDs, mediaApplied, notifApplied)
		if !isClientError(terr) && !errorsIsUnavailable(terr) {
			p.log.WithError(terr).WithFields(logrus.Fields{
				"vehicle_id": in.VehicleID, "correlation_id": in.CorrelationID,
			}).Error("vehicle transfer transaction")
		}
		return TransferResult{}, terr
	}

	p.log.WithFields(logrus.Fields{
		"vehicle_id":     spec.VehicleID,
		"source_fleet":   spec.SourceFleetID,
		"dest_fleet":     spec.DestFleetID,
		"actor":          in.ActorUserID,
		"correlation_id": in.CorrelationID,
		"affected":       counts,
	}).Info("admin vehicle transferred")

	return TransferResult{
		VehicleID:          spec.VehicleID,
		SourceFleetID:      spec.SourceFleetID,
		DestinationFleetID: spec.DestFleetID,
		TransferredAt:      now,
		AffectedCounts:     counts,
	}, nil
}

// compensate reverses whichever downstream moves succeeded before the
// transaction failed, sending everything back to the SOURCE fleet.
//
// Safe to attempt because both reassign endpoints are idempotent
// (FR-XFER-MEDIA-4). If a compensating call ALSO fails there is nothing further
// this process can do, so it logs at error naming both fleets and every id — an
// operator with that line can finish the repair by hand, which they cannot do
// from a bare "transfer failed".
func (p *Processor) compensate(ctx context.Context, spec TransferSpec, mediaIDs []string, media, notif bool) {
	if media && p.d.MediaReassign != nil {
		if _, err := p.d.MediaReassign.Reassign(ctx, mediaIDs, spec.SourceFleetID); err != nil {
			p.log.WithError(err).WithFields(logrus.Fields{
				"vehicle_id": spec.VehicleID, "source_fleet": spec.SourceFleetID,
				"dest_fleet": spec.DestFleetID, "media_ids": mediaIDs,
			}).Error("compensating media reassign FAILED; media is stranded in the destination fleet")
		}
	}
	if notif && p.d.NotificationReassign != nil {
		if _, err := p.d.NotificationReassign.Reassign(ctx, []string{spec.VehicleID}, spec.SourceFleetID); err != nil {
			p.log.WithError(err).WithFields(logrus.Fields{
				"vehicle_id": spec.VehicleID, "source_fleet": spec.SourceFleetID,
				"dest_fleet": spec.DestFleetID,
			}).Error("compensating notification reassign FAILED; notifications are stranded in the destination fleet")
		}
	}
}

// errorsIsUnavailable keeps a 503 out of the generic transaction error log: it
// is already logged, with the downstream's own error attached, at the point it
// happened.
func errorsIsUnavailable(err error) bool {
	return errors.Is(err, server.ErrServiceUnavailable)
}
```

Add `"errors"` to the import block.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./apps/fleet-service/internal/admin/ -run 'TransferVehicle|PreviewVehicleTransfer' -v`
Expected: PASS.

- [ ] **Step 6: Run the whole package plus the arch tests**

Run: `go test ./apps/fleet-service/...`
Expected: PASS — in particular `arch_test.go`'s manifest-completeness and
`PlatformAdmin`-separation checks.

- [ ] **Step 7: Commit**

```bash
git add apps/fleet-service/internal/admin/transfer_processor.go \
        apps/fleet-service/internal/admin/processor.go \
        apps/fleet-service/internal/admin/transfer_processor_test.go
git commit -m "feat(fleet-service): vehicle transfer processor with eligibility, confirmation and compensation"
```

---

## Task 10: Routes, REST transforms, composition-root wiring, and the append-only amendment

**Files:**
- Modify: `apps/fleet-service/internal/admin/rest.go`
- Modify: `apps/fleet-service/internal/admin/resource.go`
- Modify: `apps/fleet-service/cmd/main.go`
- Modify: `apps/fleet-service/internal/activity/entity.go`
- Modify: `apps/fleet-service/internal/activity/model.go`
- Modify: `apps/fleet-service/internal/activity/administrator.go`
- Test: `apps/fleet-service/internal/admin/transfer_resource_test.go` (create)
- Test: `apps/fleet-service/internal/activity/appendonly_test.go` (create)

**Interfaces:**
- Consumes: `Processor.PreviewVehicleTransfer`, `Processor.TransferVehicle` (Task 9).
- Produces:
  - `const TypeTransferPreview = "vehicle-transfer-previews"`, `const TypeTransfer = "vehicle-transfers"`
  - `func TransformTransferPreview(p TransferPreview) server.Resource`
  - `func TransformTransfer(r TransferResult) server.Resource`
  - routes `GET /admin/vehicles/{vehicleId}/transfer-preview` and `POST /admin/vehicles/{vehicleId}/transfer`

- [ ] **Step 1: Write the failing test**

Create `apps/fleet-service/internal/admin/transfer_resource_test.go`. Follow the
existing `resource_test.go` for how it builds a router with an injected
identity; read it first and reuse its helper rather than writing a second one.

```go
package admin_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin/admintest"
	"github.com/jtumidanski/myfleet/packages/shared-go/auth"
)

// transferRouter mounts the admin tree with the given identity injected.
// resource_test.go's own `serveAs` builds a router from a bare Processor; this
// needs the harness's fully-wired one, hence the second helper. Read `serveAs`
// first and keep the two consistent.
func transferRouter(t *testing.T, h transferHarness, id auth.Identity) http.Handler {
	t.Helper()
	log := logrus.New()
	log.SetOutput(io.Discard)
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(auth.WithIdentity(req.Context(), id)))
		})
	})
	admin.InitializeRoutes(log, h.proc)(r)
	return r
}

var adminIdentity = auth.Identity{UserID: "admin-1", Email: "admin@example.com", PlatformAdmin: true}
var plainIdentity = auth.Identity{UserID: "user-1", Email: "user@example.com"}

// FR-XFER-ELIG-1. Both routes, both verbs.
func TestTransferRoutes_forbidNonPlatformAdmins(t *testing.T) {
	h := newTransferHarness(t)
	r := transferRouter(t, h, plainIdentity)

	for _, tc := range []struct{ method, path, body string }{
		{http.MethodGet, "/admin/vehicles/" + h.src.VehicleID + "/transfer-preview", ""},
		{http.MethodPost, "/admin/vehicles/" + h.src.VehicleID + "/transfer",
			`{"data":{"type":"vehicle-transfers","attributes":{"destination_fleet_id":"fleet-b","confirmation":"x"}}}`},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Errorf("%s %s: status = %d, want 403", tc.method, tc.path, w.Code)
		}
	}
	if len(h.media.calls) != 0 {
		t.Error("media was called for a non-admin request")
	}
}

func TestTransferPreviewRoute_returnsTheDocumentedShape(t *testing.T) {
	h := newTransferHarness(t)
	r := transferRouter(t, h, adminIdentity)

	req := httptest.NewRequest(http.MethodGet,
		"/admin/vehicles/"+h.src.VehicleID+"/transfer-preview?destination_fleet_id=fleet-b", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	var doc struct {
		Data struct {
			Type       string `json:"type"`
			ID         string `json:"id"`
			Attributes struct {
				VehicleLabel         string         `json:"vehicle_label"`
				SourceFleetID        string         `json:"source_fleet_id"`
				SourceFleetName      string         `json:"source_fleet_name"`
				DestinationFleetID   string         `json:"destination_fleet_id"`
				DestinationFleetName string         `json:"destination_fleet_name"`
				Counts               map[string]int `json:"counts"`
				CategoriesToCreate   []struct {
					Name string `json:"name"`
					Kind string `json:"kind"`
				} `json:"categories_to_create"`
				Warnings []string `json:"warnings"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if doc.Data.Type != "vehicle-transfer-previews" {
		t.Errorf("type = %q", doc.Data.Type)
	}
	if doc.Data.ID != h.src.VehicleID {
		t.Errorf("id = %q, want the vehicle id", doc.Data.ID)
	}
	if doc.Data.Attributes.VehicleLabel != seededLabel {
		t.Errorf("vehicle_label = %q", doc.Data.Attributes.VehicleLabel)
	}
	if doc.Data.Attributes.SourceFleetName == "" || doc.Data.Attributes.DestinationFleetName == "" {
		t.Error("fleet names are missing; the dialog names the destination in its toast")
	}
	if doc.Data.Attributes.Counts["fuel_logs"] != 1 {
		t.Errorf("counts = %v", doc.Data.Attributes.Counts)
	}
	if doc.Data.Attributes.Warnings == nil {
		t.Error("warnings serialised as null; it must be []")
	}
	if doc.Data.Attributes.CategoriesToCreate == nil {
		t.Error("categories_to_create serialised as null; it must be []")
	}
}

func TestTransferPreviewRoute_unknownVehicleIs404(t *testing.T) {
	h := newTransferHarness(t)
	r := transferRouter(t, h, adminIdentity)

	req := httptest.NewRequest(http.MethodGet, "/admin/vehicles/nope/transfer-preview", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestTransferRoute_happyPathReturnsTheAppliedCounts(t *testing.T) {
	h := newTransferHarness(t)
	r := transferRouter(t, h, adminIdentity)

	body := `{"data":{"type":"vehicle-transfers","attributes":{` +
		`"destination_fleet_id":"fleet-b","confirmation":"` + seededLabel + `"}}}`
	req := httptest.NewRequest(http.MethodPost,
		"/admin/vehicles/"+h.src.VehicleID+"/transfer", strings.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	var doc struct {
		Data struct {
			Type       string `json:"type"`
			ID         string `json:"id"`
			Attributes struct {
				VehicleID          string         `json:"vehicle_id"`
				SourceFleetID      string         `json:"source_fleet_id"`
				DestinationFleetID string         `json:"destination_fleet_id"`
				TransferredAt      time.Time      `json:"transferred_at"`
				AffectedCounts     map[string]int `json:"affected_counts"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if doc.Data.Type != "vehicle-transfers" || doc.Data.ID != h.src.VehicleID {
		t.Errorf("type/id = %s/%s", doc.Data.Type, doc.Data.ID)
	}
	if doc.Data.Attributes.DestinationFleetID != "fleet-b" {
		t.Errorf("destination_fleet_id = %q", doc.Data.Attributes.DestinationFleetID)
	}
	if doc.Data.Attributes.AffectedCounts["media_objects"] != 1 {
		t.Errorf("affected_counts = %v", doc.Data.Attributes.AffectedCounts)
	}
	if doc.Data.Attributes.TransferredAt.IsZero() {
		t.Error("transferred_at is zero")
	}
}

// FR-XFER-UI-7 depends on the detail reaching the client verbatim rather than
// being replaced by the redacted errInternal.
func TestTransferRoute_surfacesTheServersDetail(t *testing.T) {
	h := newTransferHarness(t)
	r := transferRouter(t, h, adminIdentity)

	body := `{"data":{"type":"vehicle-transfers","attributes":{` +
		`"destination_fleet_id":"fleet-a","confirmation":"` + seededLabel + `"}}}`
	req := httptest.NewRequest(http.MethodPost,
		"/admin/vehicles/"+h.src.VehicleID+"/transfer", strings.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", w.Code)
	}
	if !strings.Contains(w.Body.String(), "vehicle already belongs to that fleet") {
		t.Errorf("body = %s, want the server's own detail", w.Body.String())
	}
}

// A 409 confirmation mismatch must be a 409 through the whole stack, and must
// still have written nothing.
func TestTransferRoute_confirmationMismatchIs409(t *testing.T) {
	h := newTransferHarness(t)
	r := transferRouter(t, h, adminIdentity)

	body := `{"data":{"type":"vehicle-transfers","attributes":{` +
		`"destination_fleet_id":"fleet-b","confirmation":"wrong"}}}`
	req := httptest.NewRequest(http.MethodPost,
		"/admin/vehicles/"+h.src.VehicleID+"/transfer", strings.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", w.Code)
	}
	if n := scanOne[int](t, h.db, `SELECT count(*) FROM fleet.admin_audit_events`); n != 0 {
		t.Errorf("audit rows = %d, want 0", n)
	}
}

// admintest.SeedFleet is shared with the other transfer tests; this keeps the
// import used when the file is read in isolation.
var _ = admintest.SeedFleet
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./apps/fleet-service/internal/admin/ -run 'TransferRoute|TransferPreviewRoute|TransferRoutes' -v`
Expected: 404 from the router — the routes do not exist.

- [ ] **Step 3: Add the REST transforms**

In `apps/fleet-service/internal/admin/rest.go`, add to the resource-type block:

```go
	TypeTransferPreview = "vehicle-transfer-previews"
	TypeTransfer        = "vehicle-transfers"
```

and append:

```go
type transferPreviewAttributes struct {
	VehicleLabel string `json:"vehicle_label"`
	// The label the console must echo. Computed once, server-side, so the
	// client never derives the confirmation phrase independently
	// (FR-XFER-CONF-2).
	SourceFleetID        string             `json:"source_fleet_id"`
	SourceFleetName      string             `json:"source_fleet_name"`
	DestinationFleetID   string             `json:"destination_fleet_id"`
	DestinationFleetName string             `json:"destination_fleet_name"`
	Counts               map[string]int     `json:"counts"`
	CategoriesToCreate   []CategoryToCreate `json:"categories_to_create"`
	// Degradation notes. No omitempty: [] and absent must not read the same.
	Warnings []string `json:"warnings"`
}

// TransformTransferPreview renders the blast radius of a transfer that has not
// happened. The id is the vehicle's, because that is what the preview is of.
func TransformTransferPreview(p TransferPreview) server.Resource {
	return server.Resource{
		Type: TypeTransferPreview,
		ID:   p.VehicleID,
		Attributes: transferPreviewAttributes{
			VehicleLabel:         p.VehicleLabel,
			SourceFleetID:        p.SourceFleetID,
			SourceFleetName:      p.SourceFleetName,
			DestinationFleetID:   p.DestinationFleetID,
			DestinationFleetName: p.DestinationFleetName,
			Counts:               p.Counts,
			CategoriesToCreate:   p.CategoriesToCreate,
			Warnings:             p.Warnings,
		},
	}
}

type transferAttributes struct {
	VehicleID          string         `json:"vehicle_id"`
	SourceFleetID      string         `json:"source_fleet_id"`
	DestinationFleetID string         `json:"destination_fleet_id"`
	TransferredAt      time.Time      `json:"transferred_at"`
	AffectedCounts     map[string]int `json:"affected_counts"`
}

// TransformTransfer renders a completed transfer.
func TransformTransfer(r TransferResult) server.Resource {
	return server.Resource{
		Type: TypeTransfer,
		ID:   r.VehicleID,
		Attributes: transferAttributes{
			VehicleID:          r.VehicleID,
			SourceFleetID:      r.SourceFleetID,
			DestinationFleetID: r.DestinationFleetID,
			TransferredAt:      r.TransferredAt,
			AffectedCounts:     r.AffectedCounts,
		},
	}
}
```

- [ ] **Step 4: Register the routes**

In `apps/fleet-service/internal/admin/resource.go`, inside `InitializeRoutes`,
after the `/admin/purge-operations/{id}/retry` handler:

```go
		r.Get("/admin/vehicles/{vehicleId}/transfer-preview", func(w http.ResponseWriter, req *http.Request) {
			if !authorized(w, req) {
				return
			}
			// destination_fleet_id is optional: without it the response omits
			// the destination fields and categories_to_create, which cannot be
			// computed without knowing where the car is going.
			pv, err := proc.PreviewVehicleTransfer(req.Context(),
				chi.URLParam(req, "vehicleId"), req.URL.Query().Get("destination_fleet_id"))
			if err != nil {
				if isClientError(err) {
					server.WriteError(w, err)
					return
				}
				log.WithError(err).Error("admin vehicle transfer preview")
				server.WriteError(w, errInternal)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{Data: TransformTransferPreview(pv)})
		})

		r.Post("/admin/vehicles/{vehicleId}/transfer", server.RegisterInputHandler(
			func(w http.ResponseWriter, req *http.Request, attrs struct {
				DestinationFleetID string `json:"destination_fleet_id"`
				Confirmation       string `json:"confirmation"`
			},
			) {
				// RegisterInputHandler's (w, req, attrs) shape leaves no room
				// for the `authorized` helper, so the guard is inlined —
				// exactly as POST /admin/purge-operations does.
				identity := auth.IdentityFromContext(req.Context())
				if err := authz.RequirePlatformAdmin(identity); err != nil {
					server.WriteError(w, err)
					return
				}
				res, err := proc.TransferVehicle(req.Context(), TransferInput{
					VehicleID:     chi.URLParam(req, "vehicleId"),
					DestFleetID:   attrs.DestinationFleetID,
					Confirmation:  attrs.Confirmation,
					ActorUserID:   identity.UserID,
					ActorEmail:    identity.Email,
					CorrelationID: telemetry.CorrelationIDFromContext(req.Context()),
				})
				if err != nil {
					// A 503 is deliberately NOT a client error: it is an
					// incident and must stay logged. Its detail still reaches
					// the console, because WriteError carries it either way.
					if isClientError(err) {
						server.WriteError(w, err)
						return
					}
					if errors.Is(err, server.ErrServiceUnavailable) {
						server.WriteError(w, err)
						return
					}
					log.WithError(err).Error("transfer vehicle")
					server.WriteError(w, errInternal)
					return
				}
				server.WriteJSON(w, http.StatusOK, server.Document{Data: TransformTransfer(res)})
			}))
```

- [ ] **Step 5: Extend the exhaustive route guard table**

`apps/fleet-service/internal/admin/resource_test.go` carries an `adminRoutes`
table documented as *"exhaustive on purpose — a new endpoint added without the
guard is exactly the failure this catches"*. Add both new routes to it:

```go
	{http.MethodGet, "/admin/vehicles/vehicle-1/transfer-preview"},
	{http.MethodPost, "/admin/vehicles/vehicle-1/transfer"},
```

Leave `readRoutes` alone: both new routes need a seeded target, which is what
that second table excludes.

- [ ] **Step 6: Wire the reassigners in the composition root**

In `apps/fleet-service/cmd/main.go`, add to the `admin.Deps` literal, after
`StatsSources`:

```go
		// The transfer's two downstream halves. They are the same clients the
		// purge fan-out uses; only the protocol differs.
		MediaReassign:        mediaAdmin,
		NotificationReassign: notifAdmin,
```

- [ ] **Step 7: Narrow the append-only invariant (design D11)**

`fleet.activity_events` is documented as append-only in three places. The
transfer rewrites `fleet_id` on those rows, so the comments must state the
exception rather than be quietly false.

In `apps/fleet-service/internal/activity/entity.go`, replace the doc comment on
`Entity`:

```go
// Entity maps to fleet.activity_events (PRD §6). payload is JSONB on Postgres.
//
// Append-only, with ONE exception: rows are inserted once and never updated or
// deleted by any ordinary code path, and this package exposes no way to do so.
// An admin vehicle transfer re-points fleet_id — and ONLY fleet_id — on the
// moved vehicle's rows, in raw SQL inside internal/admin (see that package's
// transfer.go). fleet_id is not part of "what happened"; it is a denormalised
// routing column answering "whose feed does this appear in", and correcting it
// when the vehicle changes owners preserves the feed's meaning rather than
// revising it. Leaving it stale would leak one household's activity into
// another's vehicle detail view, because Provider.ListByVehicle selects on
// vehicle_id alone.
```

Apply the same narrowing to the `Model` doc comment in
`apps/fleet-service/internal/activity/model.go` and to the `Administrator` doc
comment in `apps/fleet-service/internal/activity/administrator.go` (whose
current text is *"The feed is APPEND-ONLY: there is no Update or Delete"* —
extend it with the same exception and the same pointer).

- [ ] **Step 8: Pin the invariant with a test**

Create `apps/fleet-service/internal/activity/appendonly_test.go`:

```go
package activity_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/activity"
)

// The domain's own API stays mutation-free even though an admin transfer
// re-points fleet_id in raw SQL. Adding an Update or Delete here must be a
// deliberate act, not drift — so this fails the moment one appears.
func TestAdministrator_exposesNoMutationMethods(t *testing.T) {
	typ := reflect.TypeOf((*activity.Administrator)(nil)).Elem()
	for i := 0; i < typ.NumMethod(); i++ {
		name := typ.Method(i).Name
		if strings.HasPrefix(name, "Update") || strings.HasPrefix(name, "Delete") ||
			strings.HasPrefix(name, "Set") {
			t.Errorf("activity.Administrator gained %q; the feed is append-only "+
				"except for the admin transfer's fleet_id rewrite, which lives in internal/admin", name)
		}
	}
}
```

- [ ] **Step 9: Run the full backend suite**

Run: `go test ./apps/fleet-service/... ./apps/media-service/... ./apps/notification-service/... ./packages/shared-go/...`
Expected: PASS.

Run: `make vet && make build`
Expected: no output / clean build.

- [ ] **Step 10: Commit**

```bash
git add apps/fleet-service/internal/admin/rest.go \
        apps/fleet-service/internal/admin/resource.go \
        apps/fleet-service/internal/admin/transfer_resource_test.go \
        apps/fleet-service/internal/admin/resource_test.go \
        apps/fleet-service/cmd/main.go \
        apps/fleet-service/internal/activity/
git commit -m "feat(fleet-service): vehicle transfer routes, transforms and wiring"
```

---

## Task 11: Frontend types and `AdminService` methods

**Files:**
- Modify: `apps/web/src/types/models/admin.ts`
- Modify: `apps/web/src/services/api/AdminService.ts`
- Test: `apps/web/src/services/api/AdminService.transfer.test.ts` (create)

Node may need loading first: `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22`.

**Interfaces:**
- Consumes: the Go transforms from Task 10 — these types mirror them field for field.
- Produces:
  - `interface VehicleTransferPreviewAttributes`
  - `interface VehicleTransferAttributes`
  - `interface TransferVehicleInput { destination_fleet_id: string; confirmation: string }`
  - `interface CategoryToCreate { name: string; kind: string }`
  - `AuditAction` widened with `'vehicle.transferred'`
  - `adminService.previewVehicleTransfer(vehicleId: string, destinationFleetId?: string): Promise<JsonApiResource<VehicleTransferPreviewAttributes>>`
  - `adminService.transferVehicle(vehicleId: string, attributes: TransferVehicleInput): Promise<JsonApiResource<VehicleTransferAttributes>>`

- [ ] **Step 1: Write the failing test**

Create `apps/web/src/services/api/AdminService.transfer.test.ts`:

```ts
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { adminService } from './AdminService';
import { apiClient } from '../../lib/api/client';

vi.mock('../../lib/api/client', () => ({
  apiClient: { request: vi.fn() },
}));

const request = vi.mocked(apiClient.request);

beforeEach(() => {
  request.mockReset();
  request.mockResolvedValue({ data: { id: 'v1', type: 'x', attributes: {} } });
});

describe('previewVehicleTransfer', () => {
  it('omits the destination when none is given', async () => {
    await adminService.previewVehicleTransfer('v1');
    expect(request).toHaveBeenCalledWith('/api/fleet/admin/vehicles/v1/transfer-preview');
  });

  it('passes the destination as a query parameter', async () => {
    await adminService.previewVehicleTransfer('v1', 'fleet-b');
    expect(request).toHaveBeenCalledWith(
      '/api/fleet/admin/vehicles/v1/transfer-preview?destination_fleet_id=fleet-b',
    );
  });

  it('encodes an id containing URL-significant characters', async () => {
    await adminService.previewVehicleTransfer('v/1', 'a b');
    const [path] = request.mock.calls[0];
    expect(path).toContain('v%2F1');
    expect(path).toContain('destination_fleet_id=a+b');
  });
});

describe('transferVehicle', () => {
  it('posts a JSON:API document with the vehicle-transfers type', async () => {
    await adminService.transferVehicle('v1', {
      destination_fleet_id: 'fleet-b',
      confirmation: 'The Green Bean',
    });
    expect(request).toHaveBeenCalledWith('/api/fleet/admin/vehicles/v1/transfer', {
      method: 'POST',
      body: JSON.stringify({
        data: {
          type: 'vehicle-transfers',
          attributes: { destination_fleet_id: 'fleet-b', confirmation: 'The Green Bean' },
        },
      }),
    });
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npx vitest run src/services/api/AdminService.transfer.test.ts --root apps/web`
Expected: FAIL — `adminService.previewVehicleTransfer is not a function`.

- [ ] **Step 3: Add the types**

In `apps/web/src/types/models/admin.ts`:

Widen the audit union and add the two audit fields:

```ts
export type AuditAction =
  | 'purge.created'
  | 'purge.cancelled'
  | 'purge.retried'
  | 'purge.reaped'
  | 'vehicle.transferred';
```

In `AuditEventAttributes`, after `purge_operation_id`:

```ts
  /** Populated only for `vehicle.transferred`; empty string otherwise. */
  source_fleet_id: string;
  destination_fleet_id: string;
```

Append:

```ts
/** A category the transfer would add to the destination fleet. */
export interface CategoryToCreate {
  name: string;
  kind: string;
}

/**
 * GET /api/fleet/admin/vehicles/{id}/transfer-preview.
 *
 * `vehicle_label` is the EXACT string the confirmation input must match. It is
 * computed server-side and echoed here precisely so the console never derives
 * its own copy of a phrase that has to match byte for byte.
 *
 * The destination fields and `categories_to_create` are empty until a
 * destination is chosen — neither can be computed without one.
 *
 * `counts.media_objects` is "media references held by this vehicle". The
 * completed transfer reports media-service's own read-back, which is lower only
 * when a reference was already dangling before the transfer.
 *
 * `counts` carries no `notifications` key: notification-service owns that
 * relationship and the preview deliberately calls no other service.
 */
export interface VehicleTransferPreviewAttributes {
  vehicle_label: string;
  source_fleet_id: string;
  source_fleet_name: string;
  destination_fleet_id: string;
  destination_fleet_name: string;
  counts: Record<string, number>;
  categories_to_create: CategoryToCreate[];
  warnings: string[];
}

/** POST /api/fleet/admin/vehicles/{id}/transfer — the completed move. */
export interface VehicleTransferAttributes {
  vehicle_id: string;
  source_fleet_id: string;
  destination_fleet_id: string;
  transferred_at: string;
  affected_counts: Record<string, number>;
}

export interface TransferVehicleInput {
  destination_fleet_id: string;
  confirmation: string;
}
```

- [ ] **Step 4: Add the service methods**

In `apps/web/src/services/api/AdminService.ts`, import the three new types, then
add to the class after `getFleet`:

```ts
  /**
   * GET /api/fleet/admin/vehicles/{id}/transfer-preview?destination_fleet_id=
   *
   * Read-only and cheap: the server counts with aggregates and calls no other
   * service, so this is safe to re-run whenever the chosen destination changes.
   */
  async previewVehicleTransfer(
    vehicleId: string,
    destinationFleetId?: string,
  ): Promise<JsonApiResource<VehicleTransferPreviewAttributes>> {
    const search = new URLSearchParams();
    if (destinationFleetId) search.set('destination_fleet_id', destinationFleetId);
    const doc = await apiClient.request<
      JsonApiDocument<JsonApiResource<VehicleTransferPreviewAttributes>>
    >(
      withQuery(`${this.basePath}/vehicles/${encodeURIComponent(vehicleId)}/transfer-preview`, search),
    );
    return doc.data;
  }

  /**
   * POST /api/fleet/admin/vehicles/{id}/transfer
   *
   * `confirmation` must be WHAT THE OPERATOR TYPED. The server compares it
   * exactly — no trimming, no case folding — so sending the expected phrase
   * instead would make its 409 unreachable and the disabled button the only
   * gate.
   *
   * 409 covers a confirmation mismatch, a pending-purge vehicle or source
   * fleet, and an unavailable destination; 422 covers a missing, malformed or
   * same-fleet destination; 503 means a downstream refused and the transfer was
   * rolled back whole. Every one carries an actionable `detail`, which is why
   * the hook surfaces it verbatim.
   */
  async transferVehicle(
    vehicleId: string,
    attributes: TransferVehicleInput,
  ): Promise<JsonApiResource<VehicleTransferAttributes>> {
    const doc = await apiClient.request<JsonApiDocument<JsonApiResource<VehicleTransferAttributes>>>(
      `${this.basePath}/vehicles/${encodeURIComponent(vehicleId)}/transfer`,
      {
        method: 'POST',
        body: JSON.stringify({ data: { type: 'vehicle-transfers', attributes } }),
      },
    );
    return doc.data;
  }
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `npx vitest run src/services/api/AdminService.transfer.test.ts --root apps/web`
Expected: PASS (4 tests).

- [ ] **Step 6: Commit**

```bash
git add apps/web/src/types/models/admin.ts \
        apps/web/src/services/api/AdminService.ts \
        apps/web/src/services/api/AdminService.transfer.test.ts
git commit -m "feat(web): vehicle transfer types and AdminService methods"
```

---

## Task 12: React Query hooks (and the error-detail bug they depend on)

**Files:**
- Modify: `packages/shared-ts/src/errors.ts`
- Modify: `packages/shared-ts/src/errors.test.ts`
- Modify: `apps/web/src/lib/hooks/api/admin.ts`
- Test: `apps/web/src/lib/hooks/api/admin.transfer.test.tsx` (create)

**Blocker found while planning — `createErrorFromUnknown` loses `detail` and
`status`.** `ApiClient.request` already converts a failed response into an
`ApiError` before throwing (`packages/shared-ts/src/apiClient.ts:50`). Every
hook then calls `createErrorFromUnknown(err)` on that value — but that function
only unwraps a raw `{status, body:{errors}}` envelope, and an `ApiError` has
neither, so it falls through to the `e instanceof Error` branch and returns
`new ApiError(0, 'unknown', e.message)`. `detail` is dropped and `status`
becomes `0`.

FR-XFER-UI-7 requires the server's `detail` verbatim, so this must be fixed
rather than worked around. The fix is two lines and retroactively repairs
`useCreatePurge`'s `status === 409` / `=== 403` branches, which are dead code
today for the same reason.

**Interfaces:**
- Consumes: `adminService.previewVehicleTransfer`, `adminService.transferVehicle` (Task 11).
- Produces:
  - `adminKeys.transferPreview(vehicleId: string, destinationFleetId: string)`
  - `useVehicleTransferPreview(vehicleId: string | undefined, destinationFleetId: string, enabled: boolean)`
  - `useTransferVehicle()` — mutation taking `{ vehicleId: string; attributes: TransferVehicleInput; destinationName: string }`

- [ ] **Step 1: Write the failing test for the error-conversion bug**

Append to `packages/shared-ts/src/errors.test.ts`:

```ts
  it('passes an ApiError through unchanged', () => {
    // ApiClient.request already converts a failed response into an ApiError
    // before throwing, so every hook that calls createErrorFromUnknown on a
    // caught error hands it one of these. Rebuilding it would drop `detail`
    // and reset `status` to 0 — which is exactly what the console needs to
    // decide what to tell the operator.
    const original = new ApiError(409, 'conflict', 'conflict', 'vehicle is pending purge');
    const err = createErrorFromUnknown(original);
    expect(err).toBe(original);
    expect(err.status).toBe(409);
    expect(err.detail).toBe('vehicle is pending purge');
  });
```

- [ ] **Step 2: Run it to verify it fails**

Run: `npx vitest run src/errors.test.ts --root packages/shared-ts`
Expected: FAIL — `expected 0 to be 409`.

- [ ] **Step 3: Fix `createErrorFromUnknown`**

In `packages/shared-ts/src/errors.ts`, at the top of `createErrorFromUnknown`:

```ts
export function createErrorFromUnknown(e: unknown): ApiError {
  // Already converted — by ApiClient.request, which throws an ApiError rather
  // than a raw envelope. Rebuilding it would fall through to the generic
  // branch below and silently reset status to 0 and drop detail.
  if (e instanceof ApiError) return e;
  const env = e as EnvelopeShape;
```

- [ ] **Step 4: Run it to verify it passes**

Run: `npx vitest run src/errors.test.ts --root packages/shared-ts`
Expected: PASS (3 tests).

- [ ] **Step 5: Write the failing hook test**

Create `apps/web/src/lib/hooks/api/admin.transfer.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { toast } from 'sonner';
import { ApiError } from '@myfleet/shared-ts';
import { adminService } from '../../../services/api/AdminService';
import { adminKeys, useTransferVehicle, useVehicleTransferPreview } from './admin';

vi.mock('../../../services/api/AdminService', () => ({
  adminService: { previewVehicleTransfer: vi.fn(), transferVehicle: vi.fn() },
}));
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

function wrapper(client: QueryClient) {
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
  };
}

function newClient() {
  return new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
}

beforeEach(() => {
  vi.mocked(adminService.previewVehicleTransfer).mockReset();
  vi.mocked(adminService.transferVehicle).mockReset();
  vi.mocked(toast.error).mockReset();
  vi.mocked(toast.success).mockReset();
});

describe('useVehicleTransferPreview', () => {
  it('does not fetch while disabled', async () => {
    renderHook(() => useVehicleTransferPreview('v1', 'fleet-b', false), {
      wrapper: wrapper(newClient()),
    });
    await waitFor(() => {
      expect(adminService.previewVehicleTransfer).not.toHaveBeenCalled();
    });
  });

  it('refetches when the destination changes', async () => {
    vi.mocked(adminService.previewVehicleTransfer).mockResolvedValue({
      id: 'v1',
      type: 'vehicle-transfer-previews',
      attributes: { vehicle_label: 'The Green Bean' },
    } as never);
    const client = newClient();
    const { rerender } = renderHook(
      ({ dest }: { dest: string }) => useVehicleTransferPreview('v1', dest, true),
      { wrapper: wrapper(client), initialProps: { dest: 'fleet-b' } },
    );
    await waitFor(() => expect(adminService.previewVehicleTransfer).toHaveBeenCalledWith('v1', 'fleet-b'));

    rerender({ dest: 'fleet-c' });
    await waitFor(() => expect(adminService.previewVehicleTransfer).toHaveBeenCalledWith('v1', 'fleet-c'));
  });
});

describe('useTransferVehicle', () => {
  it('invalidates the whole admin subtree on settle', async () => {
    vi.mocked(adminService.transferVehicle).mockResolvedValue({
      id: 'v1',
      type: 'vehicle-transfers',
      attributes: {},
    } as never);
    const client = newClient();
    const spy = vi.spyOn(client, 'invalidateQueries');
    const { result } = renderHook(() => useTransferVehicle(), { wrapper: wrapper(client) });

    result.current.mutate({
      vehicleId: 'v1',
      attributes: { destination_fleet_id: 'fleet-b', confirmation: 'The Green Bean' },
      destinationName: 'Smith Household',
    });

    await waitFor(() => expect(spy).toHaveBeenCalledWith({ queryKey: adminKeys.all }));
  });

  it('names the destination in the success toast', async () => {
    vi.mocked(adminService.transferVehicle).mockResolvedValue({
      id: 'v1',
      type: 'vehicle-transfers',
      attributes: {},
    } as never);
    const { result } = renderHook(() => useTransferVehicle(), { wrapper: wrapper(newClient()) });

    result.current.mutate({
      vehicleId: 'v1',
      attributes: { destination_fleet_id: 'fleet-b', confirmation: 'The Green Bean' },
      destinationName: 'Smith Household',
    });

    await waitFor(() => {
      expect(vi.mocked(toast.success).mock.calls[0][0]).toContain('Smith Household');
    });
  });

  it('surfaces the server detail verbatim', async () => {
    vi.mocked(adminService.transferVehicle).mockRejectedValue(
      new ApiError(409, 'conflict', 'conflict', 'vehicle is pending purge and cannot be transferred'),
    );
    const { result } = renderHook(() => useTransferVehicle(), { wrapper: wrapper(newClient()) });

    result.current.mutate({
      vehicleId: 'v1',
      attributes: { destination_fleet_id: 'fleet-b', confirmation: 'x' },
      destinationName: 'Smith Household',
    });

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith('vehicle is pending purge and cannot be transferred');
    });
  });

  it('falls back to a generic message when there is no detail', async () => {
    vi.mocked(adminService.transferVehicle).mockRejectedValue(new Error('boom'));
    const { result } = renderHook(() => useTransferVehicle(), { wrapper: wrapper(newClient()) });

    result.current.mutate({
      vehicleId: 'v1',
      attributes: { destination_fleet_id: 'fleet-b', confirmation: 'x' },
      destinationName: 'Smith Household',
    });

    await waitFor(() => expect(toast.error).toHaveBeenCalled());
  });
});
```

`ApiError`'s constructor is `(status, code, message, detail?, pointer?)` — note
the status comes FIRST, which is easy to get backwards.

- [ ] **Step 6: Run test to verify it fails**

Run: `npx vitest run src/lib/hooks/api/admin.transfer.test.tsx --root apps/web`
Expected: FAIL — `useVehicleTransferPreview is not exported`.

- [ ] **Step 7: Add the query key**

In `apps/web/src/lib/hooks/api/admin.ts`, add to `adminKeys`:

```ts
  transferPreview: (vehicleId: string, destinationFleetId: string) =>
    [...adminKeys.all, 'transfer-preview', vehicleId, destinationFleetId] as const,
```

- [ ] **Step 8: Add the two hooks**

Append to `apps/web/src/lib/hooks/api/admin.ts`:

```ts
/**
 * GET /api/fleet/admin/vehicles/{id}/transfer-preview.
 *
 * `enabled` is passed in rather than derived, because the dialog wants the
 * query to run only while it is open — a preview fetched behind a closed dialog
 * is wasted work whose result nobody reads.
 *
 * The destination is part of the key, so choosing a different one refetches
 * rather than reusing counts computed against the previous fleet.
 *
 * staleTime is 0, deliberately, unlike every other admin query here. These
 * counts sit directly above a confirmation input for an operation with no
 * one-click undo; a cached figure from thirty seconds ago is exactly the wrong
 * thing to show an operator about to type a phrase.
 */
export function useVehicleTransferPreview(
  vehicleId: string | undefined,
  destinationFleetId: string,
  enabled: boolean,
) {
  return useQuery({
    queryKey: adminKeys.transferPreview(vehicleId ?? '', destinationFleetId),
    queryFn: () => adminService.previewVehicleTransfer(vehicleId as string, destinationFleetId),
    enabled: enabled && !!vehicleId,
    staleTime: 0,
  });
}

/**
 * POST /api/fleet/admin/vehicles/{id}/transfer.
 *
 * Invalidates the WHOLE admin subtree on settle. A transfer changes the source
 * fleet's detail, the destination fleet's detail, both fleets' vehicle counts
 * in the list, the platform stats and the audit log — all at once. Naming them
 * individually would be a list to keep in sync forever, and a stale count in
 * this console is worse than a redundant fetch (FR-XFER-UI-6).
 *
 * onError surfaces the server's `detail` VERBATIM, departing from
 * useCreatePurge's fixed strings. A purge has exactly one 409 meaning; a
 * transfer has four distinct 409/422 conditions whose whole value is the
 * specific sentence — "vehicle is pending purge" and "destination fleet is not
 * available" call for different actions from the operator (FR-XFER-UI-7).
 *
 * onSuccess shows a toast naming the destination. No other admin hook does,
 * and this one needs it: a purge lands the operator on a queue page that
 * confirms it happened, whereas a transfer closes a dialog over an unchanged
 * screen.
 */
export function useTransferVehicle() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (vars: {
      vehicleId: string;
      attributes: TransferVehicleInput;
      destinationName: string;
    }) => adminService.transferVehicle(vars.vehicleId, vars.attributes),
    onSuccess: (_data, vars) => {
      toast.success(`Vehicle transferred to ${vars.destinationName}.`);
    },
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: adminKeys.all });
    },
    onError: (err) => {
      const apiError = createErrorFromUnknown(err);
      toast.error(apiError.detail || apiError.message || 'Could not transfer the vehicle');
    },
  });
}
```

Add `TransferVehicleInput` to the existing type import from
`'../../../types/models/admin'`.

- [ ] **Step 9: Run tests to verify they pass**

Run: `npx vitest run src/lib/hooks/api/admin.transfer.test.tsx --root apps/web`
Expected: PASS (6 tests).

- [ ] **Step 10: Check the fix did not disturb the existing admin hook tests**

Run: `npx vitest run src/lib/hooks/api --root apps/web`
Expected: PASS. `useCreatePurge`'s 409/403 branches now actually fire; if a test
asserted the generic fallback message for a 409, it was asserting the bug and
should be updated to the specific message.

- [ ] **Step 11: Commit**

```bash
git add packages/shared-ts/src/errors.ts \
        packages/shared-ts/src/errors.test.ts \
        apps/web/src/lib/hooks/api/admin.ts \
        apps/web/src/lib/hooks/api/admin.transfer.test.tsx
git commit -m "fix(shared-ts): keep status and detail when converting an ApiError; add transfer hooks"
```

---

## Task 13: `VehicleTransferDialog`

**Files:**
- Create: `apps/web/src/components/admin/VehicleTransferDialog.tsx`
- Test: `apps/web/src/components/admin/VehicleTransferDialog.test.tsx` (create)

**Interfaces:**
- Consumes: `useAdminFleets`, `useVehicleTransferPreview` (Task 12); `Dialog`, `Input`, `Label`, `Button`, `RequiredMarker` from `components/ui`.
- Produces:

```ts
export interface VehicleTransferDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  vehicleId: string;
  sourceFleetId: string;
  /** Receives WHAT WAS TYPED plus the chosen destination and its name. */
  onConfirm: (args: { destinationFleetId: string; destinationName: string; typed: string }) => void;
  isPending: boolean;
}
```

**Planning decision — no `BlastRadiusPanel`, no Radix `Select`.** See the
"Deviations" section at the top of this plan; both choices are argued there.

- [ ] **Step 1: Write the failing test**

Create `apps/web/src/components/admin/VehicleTransferDialog.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { renderWithProviders } from '../../test/renderWithProviders';
import { VehicleTransferDialog } from './VehicleTransferDialog';

const useAdminFleets = vi.fn();
const useVehicleTransferPreview = vi.fn();
vi.mock('../../lib/hooks/api/admin', () => ({
  useAdminFleets: (params: unknown) => useAdminFleets(params),
  useVehicleTransferPreview: (...args: unknown[]) => useVehicleTransferPreview(...args),
}));

const LABEL = 'The Green Bean';

function mockFleetOptions(rows: Array<{ id: string; name: string }>) {
  useAdminFleets.mockReturnValue({
    data: {
      data: rows.map((r) => ({ id: r.id, type: 'admin-fleets', attributes: { name: r.name } })),
      meta: {},
    },
    isLoading: false,
    isError: false,
  });
}

function mockPreview(over: Record<string, unknown> = {}) {
  useVehicleTransferPreview.mockReturnValue({
    data: {
      id: 'v1',
      type: 'vehicle-transfer-previews',
      attributes: {
        vehicle_label: LABEL,
        source_fleet_id: 'fleet-a',
        source_fleet_name: 'Tumidanski Household',
        destination_fleet_id: '',
        destination_fleet_name: '',
        counts: { fuel_logs: 118, maintenance_records: 42, widgets_removed: 2 },
        categories_to_create: [],
        warnings: [],
        ...over,
      },
    },
    isLoading: false,
    isError: false,
  });
}

function renderDialog(over: Partial<React.ComponentProps<typeof VehicleTransferDialog>> = {}) {
  const onConfirm = vi.fn();
  renderWithProviders(
    <VehicleTransferDialog
      open
      onOpenChange={vi.fn()}
      vehicleId="v1"
      sourceFleetId="fleet-a"
      onConfirm={onConfirm}
      isPending={false}
      {...over}
    />,
  );
  return { onConfirm };
}

beforeEach(() => {
  useAdminFleets.mockReset();
  useVehicleTransferPreview.mockReset();
  mockFleetOptions([
    { id: 'fleet-a', name: 'Tumidanski Household' },
    { id: 'fleet-b', name: 'Smith Household' },
  ]);
  mockPreview();
});

function confirmButton() {
  return screen.getByRole('button', { name: /transfer vehicle/i });
}

// FR-XFER-UI-3: the source fleet is never an option, and the query asks for
// live fleets only.
it('excludes the source fleet and requests live fleets only', () => {
  renderDialog();
  expect(screen.queryByRole('button', { name: 'Tumidanski Household' })).toBeNull();
  expect(screen.getByRole('button', { name: 'Smith Household' })).toBeInTheDocument();
  expect(useAdminFleets).toHaveBeenCalledWith(expect.objectContaining({ deleted: 'exclude' }));
});

// FR-XFER-UI-4: the counts come from the preview, not from anything computed
// here.
it('renders the preview counts and the categories it would create', () => {
  mockPreview({ categories_to_create: [{ name: 'Winter Tires', kind: 'maintenance' }] });
  renderDialog();
  const radius = screen.getByTestId('transfer-blast-radius');
  expect(within(radius).getByText('118')).toBeInTheDocument();
  expect(within(radius).getByText('42')).toBeInTheDocument();
  expect(screen.getByText(/Winter Tires/)).toBeInTheDocument();
});

// FR-XFER-UI-5, both halves.
it('keeps confirm disabled until a destination is chosen AND the label is typed', async () => {
  const user = userEvent.setup();
  renderDialog();
  expect(confirmButton()).toBeDisabled();

  await user.type(screen.getByLabelText(/type the vehicle name/i), LABEL);
  expect(confirmButton()).toBeDisabled(); // no destination yet

  await user.click(screen.getByRole('button', { name: 'Smith Household' }));
  expect(confirmButton()).toBeEnabled();
});

it('keeps confirm disabled for a near-miss in casing', async () => {
  const user = userEvent.setup();
  renderDialog();
  await user.click(screen.getByRole('button', { name: 'Smith Household' }));
  await user.type(screen.getByLabelText(/type the vehicle name/i), 'the green bean');
  expect(confirmButton()).toBeDisabled();
});

it('keeps confirm disabled for a trailing space', async () => {
  const user = userEvent.setup();
  renderDialog();
  await user.click(screen.getByRole('button', { name: 'Smith Household' }));
  await user.type(screen.getByLabelText(/type the vehicle name/i), `${LABEL} `);
  expect(confirmButton()).toBeDisabled();
});

// The server performs the real comparison, so it must receive what was TYPED.
it('passes the typed value and the chosen destination to onConfirm', async () => {
  const user = userEvent.setup();
  const { onConfirm } = renderDialog();
  await user.click(screen.getByRole('button', { name: 'Smith Household' }));
  await user.type(screen.getByLabelText(/type the vehicle name/i), LABEL);
  await user.click(confirmButton());

  expect(onConfirm).toHaveBeenCalledWith({
    destinationFleetId: 'fleet-b',
    destinationName: 'Smith Household',
    typed: LABEL,
  });
});

it('disables confirm while a transfer is in flight', async () => {
  const user = userEvent.setup();
  renderDialog({ isPending: true });
  await user.click(screen.getByRole('button', { name: 'Smith Household' }));
  await user.type(screen.getByLabelText(/type the vehicle name/i), LABEL);
  expect(confirmButton()).toBeDisabled();
});

// The preview is the single source of truth for the phrase; without it there is
// nothing safe to compare against, so the control is WITHHELD rather than shown
// live over numbers nobody could produce.
it('withholds the confirm control when the preview could not be loaded', () => {
  useVehicleTransferPreview.mockReturnValue({ data: undefined, isLoading: false, isError: true });
  renderDialog();
  expect(screen.queryByRole('button', { name: /transfer vehicle/i })).toBeNull();
  expect(screen.getByRole('alert')).toBeInTheDocument();
});

// The confirmation input must be marked required for the same reason every
// other required field in this app is.
it('marks the destination and confirmation as required', () => {
  renderDialog();
  expect(screen.getByLabelText(/search fleets/i)).toHaveAttribute('aria-required', 'true');
  expect(screen.getByLabelText(/type the vehicle name/i)).toHaveAttribute('aria-required', 'true');
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npx vitest run src/components/admin/VehicleTransferDialog.test.tsx --root apps/web`
Expected: FAIL — the module does not exist.

- [ ] **Step 3: Write the component**

Create `apps/web/src/components/admin/VehicleTransferDialog.tsx`:

```tsx
import { useEffect, useState } from 'react';
import { Button } from '../ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '../ui/dialog';
import { Input } from '../ui/input';
import { Label } from '../ui/label';
import { RequiredMarker } from '../ui/required';
import { useAdminFleets, useVehicleTransferPreview } from '../../lib/hooks/api/admin';
import { cn } from '../../lib/utils';

/**
 * Move one vehicle, with its whole history, to another fleet.
 *
 * Modelled on PurgeConfirmDialog and keeping its three load-bearing mechanics:
 * the box is cleared during RENDER rather than from an effect, so the first
 * frame after opening is empty; the dialog cannot be dismissed while a request
 * is in flight; and onConfirm receives WHAT WAS TYPED, never the expected
 * phrase, so the server performs the real comparison and its 409 stays
 * reachable.
 *
 * It does NOT reuse BlastRadiusPanel: that component hard-codes a "Delete this
 * fleet" heading and a destructive purge button, and bending it to serve a
 * second, non-destructive caller would mean changing a component that sits
 * under a live purge control. The counts list here is a dozen lines.
 *
 * The destination picker is a search box over a result list rather than a
 * <Select>, because FR-XFER-UI-3 requires searching LIVE fleets by name — a
 * select of preloaded options cannot do that against a platform with more
 * fleets than one page.
 */
export interface VehicleTransferDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  vehicleId: string;
  sourceFleetId: string;
  onConfirm: (args: {
    destinationFleetId: string;
    destinationName: string;
    typed: string;
  }) => void;
  isPending: boolean;
}

/** Fixed order so the list does not reshuffle between fetches. */
const COUNT_ORDER = [
  'vehicle_media',
  'media_objects',
  'maintenance_records',
  'maintenance_schedules',
  'fuel_logs',
  'mileage_records',
  'activity_events',
  'categories_created',
  'widgets_removed',
];

const COUNT_LABELS: Record<string, string> = {
  vehicle_media: 'Photos',
  media_objects: 'Media files',
  maintenance_records: 'Maintenance records',
  maintenance_schedules: 'Maintenance schedules',
  fuel_logs: 'Fuel logs',
  mileage_records: 'Mileage records',
  activity_events: 'Activity events',
  categories_created: 'Categories to create',
  widgets_removed: 'Dashboard widgets removed',
};

/**
 * A key the server sent that this build does not label still renders, appended
 * after the known order. Omitting it would UNDERSTATE the blast radius, which
 * is the one error that matters on this screen.
 */
function humanise(key: string): string {
  const label = COUNT_LABELS[key];
  if (label) return label;
  const words = key.replace(/_/g, ' ');
  return words.charAt(0).toUpperCase() + words.slice(1);
}

/** Debounce delay for the fleet search, in ms. */
const SEARCH_DEBOUNCE_MS = 250;

export function VehicleTransferDialog({
  open,
  onOpenChange,
  vehicleId,
  sourceFleetId,
  onConfirm,
  isPending,
}: VehicleTransferDialogProps) {
  const [query, setQuery] = useState('');
  const [debouncedQuery, setDebouncedQuery] = useState('');
  const [destination, setDestination] = useState<{ id: string; name: string } | null>(null);
  const [typed, setTyped] = useState('');

  // Reset during render off a remembered `open`, not from an effect. React
  // re-runs this component before committing, so an empty box is what the
  // operator's FIRST frame shows; an effect would paint the previous phrase —
  // and its live confirm button — once.
  const [wasOpen, setWasOpen] = useState(open);
  if (wasOpen !== open) {
    setWasOpen(open);
    if (open) {
      setQuery('');
      setDebouncedQuery('');
      setDestination(null);
      setTyped('');
    }
  }

  // adminKeys.fleetList inlines the params object into the query key and
  // nothing in this console debounces, so search-as-you-type would mint one
  // cache entry and one request PER KEYSTROKE. Debounced here rather than in
  // the hook, because the fleet list's other callers are filter buttons where
  // a delay would feel broken.
  useEffect(() => {
    const t = setTimeout(() => setDebouncedQuery(query), SEARCH_DEBOUNCE_MS);
    return () => clearTimeout(t);
  }, [query]);

  const fleets = useAdminFleets({ q: debouncedQuery, deleted: 'exclude', page: 1 });
  const preview = useVehicleTransferPreview(vehicleId, destination?.id ?? '', open);

  const attrs = preview.data?.attributes;
  // The phrase comes from the PREVIEW, never derived here, so client and server
  // agree on one string (FR-XFER-CONF-2).
  const phrase = attrs?.vehicle_label ?? '';
  // Exact comparison, deliberately: no trim, no case fold, matching the server.
  const matches = phrase !== '' && typed === phrase;

  const options = (fleets.data?.data ?? []).filter((f) => f.id !== sourceFleetId);

  const counts = attrs?.counts ?? {};
  const known = COUNT_ORDER.filter((k) => k in counts);
  const extra = Object.keys(counts)
    .filter((k) => !COUNT_ORDER.includes(k))
    .sort();
  const countKeys = [...known, ...extra];

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      {/* Escape, outside-click and the close button are refused together while
          the request is in flight, so the dialog cannot be half-dismissed over
          a server that is already acting on it. */}
      <DialogContent dismissible={!isPending}>
        <DialogHeader>
          <DialogTitle>Transfer this vehicle</DialogTitle>
          <DialogDescription>
            The vehicle and its full history move to another fleet. There is no undo — the
            correction is a second transfer back.
          </DialogDescription>
        </DialogHeader>

        {preview.isError || !attrs ? (
          <div
            role="alert"
            className="rounded-sm border border-danger-border bg-danger-subtle p-3 text-sm text-danger-subtle-foreground"
          >
            {preview.isLoading
              ? 'Working out what would move…'
              : 'We could not work out what would move, so the transfer control is unavailable. Close this and try again.'}
          </div>
        ) : (
          <div className="space-y-4 text-sm">
            <div className="space-y-1">
              <Label htmlFor="transfer-destination-search">
                Search fleets by name
                <RequiredMarker />
              </Label>
              <Input
                id="transfer-destination-search"
                aria-required="true"
                autoComplete="off"
                placeholder="Destination fleet"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
              />
              <ul className="max-h-40 overflow-y-auto rounded-sm border border-border">
                {options.length === 0 ? (
                  <li className="px-3 py-2 text-muted-foreground">No other fleets match.</li>
                ) : (
                  options.map((f) => (
                    <li key={f.id}>
                      <button
                        type="button"
                        className={cn(
                          'block w-full px-3 py-2 text-left hover:bg-accent hover:text-accent-foreground',
                          destination?.id === f.id && 'bg-accent text-accent-foreground',
                        )}
                        aria-pressed={destination?.id === f.id}
                        onClick={() => setDestination({ id: f.id, name: f.attributes.name })}
                      >
                        {f.attributes.name}
                      </button>
                    </li>
                  ))
                )}
              </ul>
            </div>

            <div data-testid="transfer-blast-radius">
              <p className="font-medium">What moves with it</p>
              <dl className="mt-2 grid gap-2 sm:grid-cols-2">
                {countKeys.map((k) => (
                  <div key={k} className="flex items-baseline justify-between gap-4">
                    <dt className="text-muted-foreground">{humanise(k)}</dt>
                    <dd className="font-semibold tabular-nums">{counts[k]}</dd>
                  </div>
                ))}
              </dl>
            </div>

            {attrs.categories_to_create.length > 0 ? (
              <div>
                <p className="font-medium">Categories created in the destination fleet</p>
                <ul className="mt-1 list-inside list-disc text-muted-foreground">
                  {attrs.categories_to_create.map((c) => (
                    <li key={`${c.kind}:${c.name}`}>
                      {c.name} ({c.kind})
                    </li>
                  ))}
                </ul>
              </div>
            ) : null}

            {attrs.warnings.length > 0 ? (
              <div
                role="status"
                className="rounded-sm border border-warning-border bg-warning-subtle p-3 text-warning-subtle-foreground"
              >
                {attrs.warnings.join(' ')}
              </div>
            ) : null}

            <div className="space-y-1">
              <Label htmlFor="transfer-confirmation">
                Type the vehicle name to confirm
                <RequiredMarker />
              </Label>
              <Input
                id="transfer-confirmation"
                aria-required="true"
                autoComplete="off"
                value={typed}
                onChange={(e) => setTyped(e.target.value)}
              />
              <p className="text-muted-foreground">{phrase}</p>
            </div>
          </div>
        )}

        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          {attrs && !preview.isError ? (
            <Button
              type="button"
              disabled={!matches || !destination || isPending}
              onClick={() =>
                destination &&
                onConfirm({
                  // `typed`, never `phrase`: the server compares this exactly,
                  // and sending the expected value would make its 409
                  // unreachable from the UI.
                  destinationFleetId: destination.id,
                  destinationName: destination.name,
                  typed,
                })
              }
            >
              Transfer vehicle
            </Button>
          ) : null}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `npx vitest run src/components/admin/VehicleTransferDialog.test.tsx --root apps/web`
Expected: PASS (9 tests).

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/components/admin/VehicleTransferDialog.tsx \
        apps/web/src/components/admin/VehicleTransferDialog.test.tsx
git commit -m "feat(web): VehicleTransferDialog with destination search and typed confirmation"
```

---

## Task 14: Wire the Transfer action into the fleet inspector

**Files:**
- Modify: `apps/web/src/pages/admin/AdminFleetsPage.tsx`
- Modify: `apps/web/src/pages/admin/AdminFleetsPage.test.tsx`

**Interfaces:**
- Consumes: `VehicleTransferDialog` (Task 13), `useTransferVehicle` (Task 12).
- Produces: no new exports; a fourth column on the Vehicles table with a
  **Transfer** button per row.

- [ ] **Step 1: Extend the test file's mock factory and write the failing tests**

`AdminFleetsPage.test.tsx`'s `vi.mock` factory is exhaustive — the page will
throw the moment it calls a hook the factory does not provide. Add the new hook
first:

```tsx
const transferMutate = vi.fn();
vi.mock('../../lib/hooks/api/admin', () => ({
  useAdminFleets: () => useAdminFleets(),
  useAdminFleet: () => useAdminFleet(),
  useCreatePurge: () => ({ mutate: createPurgeMutate, isPending: false }),
  useTransferVehicle: () => ({ mutate: transferMutate, isPending: false }),
  useVehicleTransferPreview: () => ({ data: undefined, isLoading: true, isError: false }),
}));
```

Then append the new cases:

```tsx
// FR-XFER-UI-1: an action on every vehicle row.
it('renders a Transfer action on each vehicle row', () => {
  mockFleet({
    vehicles: [
      { id: 'veh-1', nickname: 'The Green Bean', make: 'Toyota', model: 'Corolla',
        year: 2020, mileage: 50000, status: 'Active', pending_purge: false },
      { id: 'veh-2', nickname: '', make: 'Honda', model: 'Civic',
        year: 2018, mileage: 90000, status: 'Active', pending_purge: false },
    ],
  });
  renderPage('f1');

  expect(within(screen.getByTestId('vehicle-veh-1')).getByRole('button', { name: /transfer/i }))
    .toBeEnabled();
  expect(within(screen.getByTestId('vehicle-veh-2')).getByRole('button', { name: /transfer/i }))
    .toBeEnabled();
});

// FR-XFER-UI-8: a pending-purge vehicle cannot be transferred, and the button
// says why rather than being a dead control.
it('disables Transfer for a pending-purge vehicle and explains why', () => {
  mockFleet({
    vehicles: [
      { id: 'veh-1', nickname: 'Doomed', make: 'Toyota', model: 'Corolla',
        year: 2020, mileage: 1, status: '', pending_purge: true },
    ],
  });
  renderPage('f1');

  const button = within(screen.getByTestId('vehicle-veh-1')).getByRole('button', { name: /transfer/i });
  expect(button).toBeDisabled();
  expect(button).toHaveAttribute('title', expect.stringMatching(/pending purge/i));
});

// The mutation must not fire from merely opening the dialog.
it('does not transfer until the dialog is confirmed', async () => {
  const user = userEvent.setup();
  mockFleet({
    vehicles: [
      { id: 'veh-1', nickname: 'The Green Bean', make: 'Toyota', model: 'Corolla',
        year: 2020, mileage: 1, status: 'Active', pending_purge: false },
    ],
  });
  renderPage('f1');

  await user.click(
    within(screen.getByTestId('vehicle-veh-1')).getByRole('button', { name: /transfer/i }),
  );
  await expectNoCall(transferMutate);
});
```

Reset `transferMutate` in the existing `beforeEach` alongside `createPurgeMutate`,
and reuse whatever `renderPage`/`mockFleet` helpers the file already defines
rather than adding new ones.

- [ ] **Step 2: Run test to verify it fails**

Run: `npx vitest run src/pages/admin/AdminFleetsPage.test.tsx --root apps/web`
Expected: FAIL — no Transfer button exists.

- [ ] **Step 3: Wire the page**

In `apps/web/src/pages/admin/AdminFleetsPage.tsx`:

Import the dialog and the hook:

```tsx
import { VehicleTransferDialog } from '../../components/admin/VehicleTransferDialog';
import { useAdminFleet, useAdminFleets, useCreatePurge, useTransferVehicle } from '../../lib/hooks/api/admin';
import type { AdminVehicleRow, DeletedFilter } from '../../types/models/admin';
```

Inside `FleetDetail`, beside the existing `confirmOpen` state:

```tsx
  // The target vehicle rather than a boolean, following FleetDetail's own
  // shape: the dialog needs to know WHICH row opened it.
  const [transferTarget, setTransferTarget] = useState<AdminVehicleRow | null>(null);
  const transferVehicle = useTransferVehicle();
```

Add a fourth, empty header cell to the Vehicles table — matching the Members
table's action column:

```tsx
                <TableHead>Status</TableHead>
                <TableHead />
```

and a cell per row, after the Status cell:

```tsx
                  <TableCell className="text-right">
                    {/*
                      FR-XFER-UI-8: a vehicle on its way out cannot be moved, and
                      the server would refuse. Saying why beats a dead button —
                      the same treatment the owner's Remove action gets above.
                    */}
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      disabled={v.pending_purge || transferVehicle.isPending}
                      title={
                        v.pending_purge
                          ? 'This vehicle is pending purge and cannot be transferred.'
                          : undefined
                      }
                      onClick={() => setTransferTarget(v)}
                    >
                      Transfer
                    </Button>
                  </TableCell>
```

At the end of the returned tree, beside `PurgeConfirmDialog`:

```tsx
      {/*
        `id` is the SOURCE fleet by construction: this row is being rendered
        inside that fleet's detail view. AdminVehicleRow carries no fleet_id of
        its own, and inventing one would be a second source of truth.
      */}
      {transferTarget ? (
        <VehicleTransferDialog
          open
          onOpenChange={(next) => {
            if (!next) setTransferTarget(null);
          }}
          vehicleId={transferTarget.id}
          sourceFleetId={id}
          isPending={transferVehicle.isPending}
          onConfirm={({ destinationFleetId, destinationName, typed }) => {
            transferVehicle.mutate(
              {
                vehicleId: transferTarget.id,
                // `typed`, never the label: the server compares it exactly.
                attributes: { destination_fleet_id: destinationFleetId, confirmation: typed },
                destinationName,
              },
              { onSuccess: () => setTransferTarget(null) },
            );
          }}
        />
      ) : null}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `npx vitest run src/pages/admin/AdminFleetsPage.test.tsx --root apps/web`
Expected: PASS, including every pre-existing case in the file.

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/pages/admin/AdminFleetsPage.tsx \
        apps/web/src/pages/admin/AdminFleetsPage.test.tsx
git commit -m "feat(web): Transfer action on each vehicle row in the fleet inspector"
```

---

## Task 15: Audit page filter + badge, and the required-marker assertion

**Files:**
- Modify: `apps/web/src/pages/admin/AdminAuditPage.tsx`
- Modify: `apps/web/src/pages/admin/AdminAuditPage.test.tsx`
- Modify: `apps/web/src/test/requiredFieldMarkers.test.ts`

**Interfaces:**
- Consumes: `AuditAction` widened in Task 11.
- Produces: no new exports.

`AdminAuditPage` has **two** parallel structures that both need an entry:
`ACTIONS` (the filter buttons) and `ACTION_LABELS` (the badge). `ACTION_LABELS`
has a `?? a.action` fallback, so a missing entry degrades to the raw string
rather than blanking — which is exactly why it needs an explicit test: the
symptom is subtle enough to ship. `ACTIONS` has no fallback at all; a missing
entry means the filter simply does not exist.

- [ ] **Step 1: Write the failing tests**

Append to `apps/web/src/pages/admin/AdminAuditPage.test.tsx`, reusing the file's
existing render and mock helpers:

```tsx
// FR-XFER-AUDIT-5, both halves. ACTIONS and ACTION_LABELS are separate lists
// and it is entirely possible to update one and not the other; the badge's
// `?? a.action` fallback would then hide the omission.
it('offers a filter for vehicle transfers', () => {
  renderPage();
  expect(screen.getByRole('button', { name: 'Transferred' })).toBeInTheDocument();
});

it('renders the transfer badge label rather than the raw action string', () => {
  mockEvents([
    {
      id: 'a1',
      action: 'vehicle.transferred',
      actor_email: 'admin@example.com',
      target_label: 'The Green Bean',
      correlation_id: 'corr-1',
      created_at: '2026-08-25T18:00:00Z',
      source_fleet_id: 'fleet-a',
      destination_fleet_id: 'fleet-b',
    },
  ]);
  renderPage();

  const row = screen.getByTestId('audit-a1');
  expect(within(row).getByText('Transferred')).toBeInTheDocument();
  expect(within(row).queryByText('vehicle.transferred')).toBeNull();
});
```

Match the shape `mockEvents` (or whatever the file calls it) already expects; if
it builds full `AuditEventAttributes` objects, add the two new fields to its
default factory so every existing case keeps type-checking.

- [ ] **Step 2: Run test to verify it fails**

Run: `npx vitest run src/pages/admin/AdminAuditPage.test.tsx --root apps/web`
Expected: FAIL — no "Transferred" filter button.

- [ ] **Step 3: Add both entries**

In `apps/web/src/pages/admin/AdminAuditPage.tsx`:

```tsx
const ACTIONS = [
  { value: '', label: 'All' },
  { value: 'purge.created', label: 'Created' },
  { value: 'purge.cancelled', label: 'Restored' },
  { value: 'purge.retried', label: 'Retried' },
  { value: 'purge.reaped', label: 'Deleted for good' },
  { value: 'vehicle.transferred', label: 'Transferred' },
];

// Kept in step with ACTIONS above by AdminAuditPage.test.tsx: the badge has a
// `?? a.action` fallback, so an omission here degrades quietly to a raw action
// string instead of failing.
const ACTION_LABELS: Record<string, string> = {
  'purge.created': 'Created',
  'purge.cancelled': 'Restored',
  'purge.retried': 'Retried',
  'purge.reaped': 'Deleted for good',
  'vehicle.transferred': 'Transferred',
};
```

- [ ] **Step 4: Add the required-marker assertion**

`apps/web/src/test/requiredFieldMarkers.test.ts` source-scans the surfaces
outside react-hook-form by hand. Append a case per input, following the
`PurgeConfirmDialog` template exactly — including its comment about delimiting
the region by the wrapping `</div>` rather than the first `/>`, since
`<RequiredMarker />` is itself self-closing:

```ts
  it('marks the vehicle transfer destination search', () => {
    const source = read('components/admin/VehicleTransferDialog.tsx');
    const start = source.indexOf('htmlFor="transfer-destination-search"');
    expect(start).toBeGreaterThan(-1);
    const region = source.slice(start, source.indexOf('</div>', start));

    expect(region).toContain('<RequiredMarker />');
    expect(region).toContain('aria-required="true"');
  });

  it('marks the vehicle transfer confirmation input', () => {
    const source = read('components/admin/VehicleTransferDialog.tsx');
    const start = source.indexOf('htmlFor="transfer-confirmation"');
    expect(start).toBeGreaterThan(-1);
    const region = source.slice(start, source.indexOf('</div>', start));

    expect(region).toContain('<RequiredMarker />');
    expect(region).toContain('aria-required="true"');
  });
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `npx vitest run src/pages/admin/AdminAuditPage.test.tsx src/test/requiredFieldMarkers.test.ts --root apps/web`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add apps/web/src/pages/admin/AdminAuditPage.tsx \
        apps/web/src/pages/admin/AdminAuditPage.test.tsx \
        apps/web/src/test/requiredFieldMarkers.test.ts
git commit -m "feat(web): surface vehicle.transferred in the audit log filter and badge"
```

---

## Task 16: Full verification

**Files:** none — this task only runs and fixes.

- [ ] **Step 1: Load Node if it is not on PATH**

```sh
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
```

- [ ] **Step 2: Run the full gate**

Run: `make ci`
Expected: PASS — `lint-check`, `vet`, `test`, `build`, `fe-test`, `fe-build`,
`manifests`, `carfax-template`.

`manifests` runs `tools/check-manifests.sh`, which is where the gateway
assertion the PRD's §8 Security demands already lives: it requires exactly one
priority-200 `internal-deny` route, carrying the `internal-deny` middleware, on
**each** entrypoint for every service with an unauthenticated `/internal/*`
surface. `/internal/admin/reassign-fleet` adds no new service to that set, so no
deploy change is needed — but the check is what keeps the rule from being
deleted by someone who does not know a zero-auth fleet-reassignment endpoint now
sits behind it.

- [ ] **Step 3: Render both overlays**

```sh
kustomize build deploy/k8s/overlays/local >/dev/null
kustomize build deploy/k8s/overlays/main  >/dev/null
```

Expected: both render clean. The `main` overlay must contain no
PersistentVolumeClaims, no Secrets, no ClusterRole and no placeholder values:

```sh
kustomize build deploy/k8s/overlays/main | grep -E 'kind: (PersistentVolumeClaim|Secret|ClusterRole)$'
```

Expected: no output.

- [ ] **Step 4: Server dry-run both overlays, if a cluster is reachable**

```sh
kustomize build deploy/k8s/overlays/main  | kubectl apply --dry-run=server -f -
kustomize build deploy/k8s/overlays/local | kubectl apply --dry-run=server -f -
```

Expected: every resource reports `(server dry run)`. **Both** are required —
rendering alone does not catch namespace or cross-resource-reference errors, and
skipping the local one is exactly how a missing `namespace:` slipped through ten
reviews. If no cluster is reachable, say so explicitly in the completion report
rather than claiming this step passed.

- [ ] **Step 5: Code review before the PR**

Run `superpowers:requesting-code-review` (or `/audit-plan`). Both Go and
TypeScript changed, so it should dispatch `plan-adherence-reviewer`,
`backend-guidelines-reviewer` and `frontend-guidelines-reviewer`. Findings land
in `docs/tasks/task-031-admin-vehicle-fleet-transfer/audit.md`. Do not open a PR
before this runs.

- [ ] **Step 6: Commit any fixes**

```bash
git add -A
git commit -m "chore(task-031): fixes from the verification gate"
```
