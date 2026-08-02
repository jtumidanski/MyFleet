// Package admin owns fleet-service's platform-admin surface: the purge manifest
// and its four operations, the purge-operation lifecycle, the audit log, and the
// /admin HTTP tree.
//
// Nothing outside this package may read auth.Identity.PlatformAdmin, and nothing
// inside it may call authz.RequireSameFleet. arch_test.go enforces both, because
// the whole safety argument for a cross-fleet API is that it lives in a parallel
// tree rather than in a relaxed guard.
package admin

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
