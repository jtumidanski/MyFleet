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
	// fleet_id is NULL for the system rows and set for fleet-scoped ones,
	// which is what the transfer's category remap keys on.
	`CREATE TABLE fleet.maintenance_categories (
		id TEXT PRIMARY KEY, name TEXT, description TEXT,
		system_defined BOOLEAN, kind TEXT, fleet_id TEXT)`,
	`CREATE TABLE fleet.purge_operations (
		id TEXT PRIMARY KEY, scope TEXT, target_type TEXT, target_id TEXT,
		target_label TEXT, status TEXT, requested_by_user_id TEXT,
		requested_by_email TEXT, requested_at DATETIME, purge_after DATETIME,
		reaped_at DATETIME, cancelled_at DATETIME,
		affected_counts BLOB, failed_services BLOB,
		created_at DATETIME, updated_at DATETIME)`,
	// No deleted_at: append-only, and it survives a system purge
	// (FR-ADMIN-AUDIT-2). source_fleet_id/destination_fleet_id are NULL for
	// every purge.* row and populated only by a vehicle transfer.
	`CREATE TABLE fleet.admin_audit_events (
		id TEXT PRIMARY KEY, actor_user_id TEXT, actor_email TEXT, action TEXT,
		scope TEXT, target_type TEXT, target_id TEXT, target_label TEXT,
		purge_operation_id TEXT, affected_counts BLOB,
		source_fleet_id TEXT, destination_fleet_id TEXT,
		correlation_id TEXT, created_at DATETIME)`,
	// The outbox is created because domain writes enqueue into it; it is
	// explicitly NOT purgeable (see admin.excludedTables).
	`CREATE TABLE outbox (
		event_id TEXT PRIMARY KEY, type TEXT, payload BLOB,
		occurred_at DATETIME, sent_at DATETIME)`,
	// The partial unique indexes are part of the schema under test: the lockout
	// regression (FR-ADMIN-DATA-4) is only meaningful if they are present.
	// SQLite supports partial indexes, so these are the real thing. On SQLite
	// (via the ATTACHed "fleet" alias) the schema qualifies the INDEX name,
	// not the table — see membership.ApplyPartialIndexes for why.
	`CREATE UNIQUE INDEX fleet.ux_membership_fleet_user
	   ON fleet_memberships (fleet_id, user_id) WHERE deleted_at IS NULL`,
	`CREATE UNIQUE INDEX fleet.ux_invite_token
	   ON fleet_invites (token) WHERE deleted_at IS NULL`,
	`CREATE UNIQUE INDEX fleet.ux_dashboard_fleet_user
	   ON dashboards (fleet_id, user_id) WHERE deleted_at IS NULL`,
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
