package maintenanceschedule

import (
	"errors"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/maintenancerecord"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/vehicle"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

func newCompletionDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// Schema-qualified TableNames target the "fleet" schema on Postgres; SQLite
	// has no schemas, so attach an in-memory database aliased "fleet". GORM's
	// AutoMigrate emits CREATE INDEX with the schema prefix stripped (a SQLite
	// quirk), so the schema-qualified tables that carry index tags are created
	// here with explicit DDL instead. The columns mirror the GORM entities.
	if err := db.Exec("ATTACH DATABASE ':memory:' AS fleet").Error; err != nil {
		t.Fatalf("attach fleet schema: %v", err)
	}
	ddl := []string{
		`CREATE TABLE fleet.vehicles (
			id TEXT PRIMARY KEY, fleet_id TEXT, nickname TEXT, make TEXT, model TEXT,
			trim TEXT, year INTEGER, vin TEXT, current_mileage INTEGER,
			primary_image_media_id TEXT, notes TEXT, created_at DATETIME,
			updated_at DATETIME, deleted_at DATETIME, purge_after DATETIME,
			purge_operation_id TEXT)`,
		`CREATE TABLE fleet.mileage_records (
			id TEXT PRIMARY KEY, vehicle_id TEXT, mileage INTEGER, recorded_at DATETIME,
			source TEXT, source_ref_id TEXT, created_by_user_id TEXT, created_at DATETIME,
			deleted_at DATETIME, purge_operation_id TEXT)`,
		`CREATE TABLE fleet.maintenance_records (
			id TEXT PRIMARY KEY, vehicle_id TEXT, category_id TEXT, description TEXT,
			performed_at DATETIME, mileage INTEGER, cost REAL, vendor TEXT, notes TEXT,
			created_by_user_id TEXT, created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
			purge_operation_id TEXT)`,
		`CREATE TABLE fleet.maintenance_record_documents (
			id TEXT PRIMARY KEY, maintenance_record_id TEXT, media_id TEXT,
			deleted_at DATETIME, purge_operation_id TEXT)`,
		`CREATE TABLE fleet.maintenance_schedules (
			id TEXT PRIMARY KEY, vehicle_id TEXT, category_id TEXT, recurrence_type TEXT,
			interval_months INTEGER, interval_miles INTEGER, one_time BOOLEAN DEFAULT 0,
			due_date DATETIME, due_mileage INTEGER DEFAULT 0, last_completed_date DATETIME,
			last_completed_mileage INTEGER, next_due_date DATETIME, next_due_mileage INTEGER,
			status TEXT, severity TEXT, active INTEGER, created_at DATETIME, updated_at DATETIME,
			deleted_at DATETIME, purge_operation_id TEXT)`,
	}
	for _, stmt := range ddl {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("ddl: %v", err)
		}
	}
	return db
}

// TestCompleteInTransaction verifies the concrete completion runs the full write
// set (record + mileage + current_mileage mirror + schedule advance) atomically.
func TestCompleteInTransaction(t *testing.T) {
	db := newCompletionDB(t)

	// Seed a vehicle at 40000 mi.
	v, err := vehicle.NewBuilder().SetFleetID("f1").SetMake("Honda").SetModel("Civic").
		SetYear(2020).SetCurrentMileage(40000).Build()
	if err != nil {
		t.Fatalf("build vehicle: %v", err)
	}
	if _, err := vehicle.NewAdministrator(db).Insert(v); err != nil {
		t.Fatalf("insert vehicle: %v", err)
	}

	// Seed a hybrid schedule completed at 35000/base.
	s, err := NewBuilder().SetVehicleID(v.ID()).SetCategoryID("c1").
		SetRecurrenceType("hybrid").SetIntervalMonths(12).SetIntervalMiles(5000).
		SetLastCompletedMileage(35000).SetLastCompletedDate(base).Build()
	if err != nil {
		t.Fatalf("build schedule: %v", err)
	}
	created, err := NewAdministrator(db).Insert(s)
	if err != nil {
		t.Fatalf("insert schedule: %v", err)
	}

	deps := NewCompletionDeps(db, maintenancerecord.NewAdministrator(db), NewAdministrator(db))
	at := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	out, err := deps.CompleteInTransaction(logrus.New(), CompletionInput{
		ScheduleID:    created.ID(),
		VehicleID:     v.ID(),
		CategoryID:    "c1",
		Date:          at,
		LatestMileage: 42000,
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if out.MaintenanceRecordID == "" {
		t.Fatal("expected a created maintenance record id")
	}

	// A maintenance record was created.
	var recCount int64
	if err := db.Table("fleet.maintenance_records").Where("id = ?", out.MaintenanceRecordID).Count(&recCount).Error; err != nil {
		t.Fatalf("count records: %v", err)
	}
	if recCount != 1 {
		t.Fatalf("want 1 maintenance record, got %d", recCount)
	}

	// A mileage record was appended with source=maintenance referencing the record.
	var mileageCount int64
	if err := db.Table("fleet.mileage_records").
		Where("vehicle_id = ? AND source = ? AND source_ref_id = ? AND mileage = ?", v.ID(), "maintenance", out.MaintenanceRecordID, 42000).
		Count(&mileageCount).Error; err != nil {
		t.Fatalf("count mileage: %v", err)
	}
	if mileageCount != 1 {
		t.Fatalf("want 1 maintenance mileage record, got %d", mileageCount)
	}

	// current_mileage advanced to 42000.
	var current int
	if err := db.Table("fleet.vehicles").Select("current_mileage").Where("id = ?", v.ID()).Scan(&current).Error; err != nil {
		t.Fatalf("read current_mileage: %v", err)
	}
	if current != 42000 {
		t.Fatalf("want current_mileage 42000, got %d", current)
	}

	// Schedule advanced: last_completed_mileage=42000, next_due_mileage=47000.
	advanced, err := NewProvider(db).GetByID(created.ID())
	if err != nil {
		t.Fatalf("get advanced schedule: %v", err)
	}
	if advanced.LastCompletedMileage() != 42000 {
		t.Fatalf("want last_completed_mileage 42000, got %d", advanced.LastCompletedMileage())
	}
	if advanced.NextDueMileage() != 47000 {
		t.Fatalf("want next_due_mileage 47000, got %d", advanced.NextDueMileage())
	}
	if !advanced.LastCompletedDate().Equal(at) {
		t.Fatalf("want last_completed_date %v, got %v", at, advanced.LastCompletedDate())
	}
}

// seedVehicle inserts a vehicle at the given odometer and returns its id.
func seedVehicle(t *testing.T, db *gorm.DB, miles int) string {
	t.Helper()
	v, err := vehicle.NewBuilder().SetFleetID("f1").SetMake("Honda").SetModel("Civic").
		SetYear(2020).SetCurrentMileage(miles).Build()
	if err != nil {
		t.Fatalf("build vehicle: %v", err)
	}
	if _, err := vehicle.NewAdministrator(db).Insert(v); err != nil {
		t.Fatalf("insert vehicle: %v", err)
	}
	return v.ID()
}

// countRows is a small helper so each assertion reads as one line.
func countRows(t *testing.T, db *gorm.DB, table, where string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := db.Table(table).Where(where, args...).Count(&n).Error; err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

// FR-COMPLETE-1 + FR-COMPLETE-2: completing a one-time schedule writes the
// record and the mileage row AND deactivates the schedule with its anchor
// cleared, all in one transaction.
func TestCompleteInTransaction_oneTimeDeactivates(t *testing.T) {
	db := newCompletionDB(t)
	vehicleID := seedVehicle(t, db, 40000)

	s, err := NewBuilder().SetVehicleID(vehicleID).SetCategoryID("c1").
		SetRecurrenceType("mileage").SetOneTime(true).
		SetDuePoint(time.Time{}, 60000).SetCurrentMileage(40000).Build()
	if err != nil {
		t.Fatalf("build schedule: %v", err)
	}
	created, err := NewAdministrator(db).Insert(s)
	if err != nil {
		t.Fatalf("insert schedule: %v", err)
	}

	deps := NewCompletionDeps(db, maintenancerecord.NewAdministrator(db), NewAdministrator(db))
	at := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	out, err := deps.CompleteInTransaction(logrus.New(), CompletionInput{
		ScheduleID: created.ID(), VehicleID: vehicleID, CategoryID: "c1",
		Date: at, LatestMileage: 60100,
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}

	if n := countRows(t, db, "fleet.maintenance_records", "id = ?", out.MaintenanceRecordID); n != 1 {
		t.Fatalf("want 1 maintenance record, got %d", n)
	}
	if n := countRows(t, db, "fleet.mileage_records",
		"vehicle_id = ? AND source = ? AND source_ref_id = ?", vehicleID, "maintenance", out.MaintenanceRecordID); n != 1 {
		t.Fatalf("want 1 mileage record, got %d", n)
	}

	after, err := NewProvider(db).GetByID(created.ID())
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if after.Active() {
		t.Error("a completed one-time schedule must be deactivated")
	}
	if !after.DueDate().IsZero() || after.DueMileage() != 0 {
		t.Errorf("the due point must be cleared, got %v / %d", after.DueDate(), after.DueMileage())
	}
	if after.LastCompletedMileage() != 60100 || !after.LastCompletedDate().Equal(at) {
		t.Errorf("completion point not recorded: %v / %d", after.LastCompletedDate(), after.LastCompletedMileage())
	}
	// A completed one-time schedule has no next due point; the generic DueState
	// would read its zero due-date as overdue, which is what the vehicle
	// schedule list would then render for the inactive row.
	if after.Status() != "ok" {
		t.Errorf("status = %q want ok", after.Status())
	}
}

// FR-COMPLETE-5 + FR-ANCHOR-3: a recurring schedule stays active, and its
// first-due anchor is cleared so interval arithmetic from the new completion
// point takes over permanently.
func TestCompleteInTransaction_recurringClearsAnchorAndStaysActive(t *testing.T) {
	db := newCompletionDB(t)
	vehicleID := seedVehicle(t, db, 40000)

	// Anchored two years out: if the anchor survived, next-due would still be
	// the anchor rather than completion + 12 months.
	farAnchor := time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC)
	s, err := NewBuilder().SetVehicleID(vehicleID).SetCategoryID("c1").
		SetRecurrenceType("hybrid").SetIntervalMonths(12).SetIntervalMiles(5000).
		SetDuePoint(farAnchor, 90000).SetCurrentMileage(40000).Build()
	if err != nil {
		t.Fatalf("build schedule: %v", err)
	}
	created, err := NewAdministrator(db).Insert(s)
	if err != nil {
		t.Fatalf("insert schedule: %v", err)
	}

	deps := NewCompletionDeps(db, maintenancerecord.NewAdministrator(db), NewAdministrator(db))
	at := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	if _, err := deps.CompleteInTransaction(logrus.New(), CompletionInput{
		ScheduleID: created.ID(), VehicleID: vehicleID, CategoryID: "c1",
		Date: at, LatestMileage: 42000,
	}); err != nil {
		t.Fatalf("complete: %v", err)
	}

	after, err := NewProvider(db).GetByID(created.ID())
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !after.Active() {
		t.Error("a completed recurring schedule must stay active")
	}
	if !after.DueDate().IsZero() || after.DueMileage() != 0 {
		t.Errorf("the first-due anchor must be cleared, got %v / %d", after.DueDate(), after.DueMileage())
	}
	if want := at.AddDate(0, 12, 0); !after.NextDueDate().Equal(want) {
		t.Errorf("next_due_date = %v want %v (completion point + interval)", after.NextDueDate(), want)
	}
	if after.NextDueMileage() != 47000 {
		t.Errorf("next_due_mileage = %d want 47000", after.NextDueMileage())
	}
}

// FR-COMPLETE-4: completing an already-inactive schedule is rejected, and the
// rejection rolls back the record insert and the mileage append with it. This
// is the acceptance criterion "an integration test that fails the last step and
// asserts nothing was written".
func TestCompleteInTransaction_inactiveScheduleWritesNothing(t *testing.T) {
	db := newCompletionDB(t)
	vehicleID := seedVehicle(t, db, 40000)

	s, err := NewBuilder().SetVehicleID(vehicleID).SetCategoryID("c1").
		SetRecurrenceType("mileage").SetOneTime(true).
		SetDuePoint(time.Time{}, 60000).SetCurrentMileage(40000).Build()
	if err != nil {
		t.Fatalf("build schedule: %v", err)
	}
	created, err := NewAdministrator(db).Insert(s.WithActive(false))
	if err != nil {
		t.Fatalf("insert schedule: %v", err)
	}

	deps := NewCompletionDeps(db, maintenancerecord.NewAdministrator(db), NewAdministrator(db))
	_, err = deps.CompleteInTransaction(logrus.New(), CompletionInput{
		ScheduleID: created.ID(), VehicleID: vehicleID, CategoryID: "c1",
		Date: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), LatestMileage: 60100,
	})
	if !errors.Is(err, server.ErrValidation) {
		t.Fatalf("want server.ErrValidation, got %v", err)
	}

	if n := countRows(t, db, "fleet.maintenance_records", "vehicle_id = ?", vehicleID); n != 0 {
		t.Errorf("a rejected completion wrote %d maintenance records", n)
	}
	if n := countRows(t, db, "fleet.mileage_records", "vehicle_id = ?", vehicleID); n != 0 {
		t.Errorf("a rejected completion wrote %d mileage records", n)
	}
	var current int
	if err := db.Table("fleet.vehicles").Select("current_mileage").Where("id = ?", vehicleID).Scan(&current).Error; err != nil {
		t.Fatalf("read current_mileage: %v", err)
	}
	if current != 40000 {
		t.Errorf("current_mileage = %d want 40000 (unchanged)", current)
	}
}
