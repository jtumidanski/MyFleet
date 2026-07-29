package fuel

import (
	"errors"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/vehicle"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

func TestDerivePrice(t *testing.T) {
	// price omitted → total/gallons
	if got, err := DerivePrice(0, 40.0, 10.0); err != nil || got.PricePerGallon != 4.0 {
		t.Fatalf("want 4.0/gal, got %v err %v", got.PricePerGallon, err)
	}
	// total omitted → price*gallons
	if got, err := DerivePrice(4.0, 0, 10.0); err != nil || got.TotalCost != 40.0 {
		t.Fatalf("want total 40.0, got %v err %v", got.TotalCost, err)
	}
	// neither derivable → 422
	if _, err := DerivePrice(0, 0, 10.0); !errors.Is(err, server.ErrValidation) {
		t.Fatalf("missing both must be 422, got %v", err)
	}
	// zero gallons → 422 (no divide-by-zero)
	if _, err := DerivePrice(0, 40.0, 0); !errors.Is(err, server.ErrValidation) {
		t.Fatalf("zero gallons must be 422, got %v", err)
	}
}

// newFuelDB sets up an in-memory SQLite database with the fuel, mileage, and
// vehicles schemas, mirroring the same pattern as maintenanceschedule's
// completion_db_test.go.
func newFuelDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec("ATTACH DATABASE ':memory:' AS fleet").Error; err != nil {
		t.Fatalf("attach fleet schema: %v", err)
	}
	ddl := []string{
		`CREATE TABLE fleet.vehicles (
			id TEXT PRIMARY KEY, fleet_id TEXT, nickname TEXT, make TEXT, model TEXT,
			trim TEXT, year INTEGER, vin TEXT, current_mileage INTEGER,
			primary_image_media_id TEXT, notes TEXT, created_at DATETIME,
			updated_at DATETIME, deleted_at DATETIME, purge_after DATETIME)`,
		`CREATE TABLE fleet.mileage_records (
			id TEXT PRIMARY KEY, vehicle_id TEXT, mileage INTEGER, recorded_at DATETIME,
			source TEXT, source_ref_id TEXT, created_by_user_id TEXT, created_at DATETIME)`,
		`CREATE TABLE fleet.fuel_logs (
			id TEXT PRIMARY KEY, vehicle_id TEXT, date DATETIME, mileage INTEGER,
			gallons REAL, total_cost REAL, price_per_gallon REAL,
			created_by_user_id TEXT, created_at DATETIME, updated_at DATETIME,
			deleted_at DATETIME)`,
		`CREATE TABLE outbox (
			event_id TEXT PRIMARY KEY, type TEXT, payload BLOB,
			occurred_at DATETIME, sent_at DATETIME)`,
	}
	for _, stmt := range ddl {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("ddl: %v", err)
		}
	}
	return db
}

// TestLogInTransaction_FuelAndMileageAtomic verifies that logging a fuel entry:
//  1. Creates a fuel log row.
//  2. Creates a mileage record with source="fuel" referencing the fuel log.
//  3. Advances current_mileage when the new reading is >= the current value.
//  4. Does NOT regress current_mileage when a lower reading is supplied.
func TestLogInTransaction_FuelAndMileageAtomic(t *testing.T) {
	db := newFuelDB(t)

	// Seed a vehicle at 40000 mi.
	v, err := vehicle.NewBuilder().
		SetFleetID("fleet-1").
		SetMake("Toyota").
		SetModel("Camry").
		SetYear(2022).
		SetCurrentMileage(40000).
		Build()
	if err != nil {
		t.Fatalf("build vehicle: %v", err)
	}
	if _, err := vehicle.NewAdministrator(db).Insert(v); err != nil {
		t.Fatalf("insert vehicle: %v", err)
	}

	deps := NewLoggingDeps(db)
	log := logrus.New()

	// --- Case 1: mileage advances (42000 > 40000) ---
	derived1, err := DerivePrice(4.0, 0, 10.0)
	if err != nil {
		t.Fatalf("derive price: %v", err)
	}
	fl1, err := NewBuilder().
		SetVehicleID(v.ID()).
		SetDate(time.Now().UTC()).
		SetMileage(42000).
		SetGallons(10.0).
		SetTotalCost(derived1.TotalCost).
		SetPricePerGallon(derived1.PricePerGallon).
		SetCreatedByUserID("user-1").
		Build()
	if err != nil {
		t.Fatalf("build fuel log: %v", err)
	}

	created1, err := deps.LogInTransaction(log, LogInput{FuelLog: fl1, FleetID: v.FleetID()})
	if err != nil {
		t.Fatalf("log fuel: %v", err)
	}
	if created1.ID() == "" {
		t.Fatal("expected a fuel log ID")
	}

	// Mileage record with source=fuel was created.
	var mileageCount int64
	if err := db.Table("fleet.mileage_records").
		Where("vehicle_id = ? AND source = ? AND source_ref_id = ? AND mileage = ?",
			v.ID(), "fuel", created1.ID(), 42000).
		Count(&mileageCount).Error; err != nil {
		t.Fatalf("count mileage: %v", err)
	}
	if mileageCount != 1 {
		t.Fatalf("want 1 mileage record, got %d", mileageCount)
	}

	// current_mileage advanced to 42000.
	var current int
	if err := db.Table("fleet.vehicles").Select("current_mileage").Where("id = ?", v.ID()).Scan(&current).Error; err != nil {
		t.Fatalf("read current_mileage: %v", err)
	}
	if current != 42000 {
		t.Fatalf("want current_mileage 42000, got %d", current)
	}

	// --- Case 2: lower mileage (38000 < 42000) — must NOT regress current_mileage ---
	derived2, err := DerivePrice(0, 60.0, 15.0)
	if err != nil {
		t.Fatalf("derive price 2: %v", err)
	}
	fl2, err := NewBuilder().
		SetVehicleID(v.ID()).
		SetDate(time.Now().UTC()).
		SetMileage(38000). // lower than current 42000
		SetGallons(15.0).
		SetTotalCost(derived2.TotalCost).
		SetPricePerGallon(derived2.PricePerGallon).
		SetCreatedByUserID("user-1").
		Build()
	if err != nil {
		t.Fatalf("build fuel log 2: %v", err)
	}

	created2, err := deps.LogInTransaction(log, LogInput{FuelLog: fl2, FleetID: v.FleetID()})
	if err != nil {
		t.Fatalf("log fuel 2: %v", err)
	}

	// Mileage record was still inserted (history kept).
	var mileageCount2 int64
	if err := db.Table("fleet.mileage_records").
		Where("source_ref_id = ?", created2.ID()).
		Count(&mileageCount2).Error; err != nil {
		t.Fatalf("count mileage 2: %v", err)
	}
	if mileageCount2 != 1 {
		t.Fatalf("want 1 mileage record for lower reading, got %d", mileageCount2)
	}

	// current_mileage must NOT have regressed; still 42000.
	var currentAfterLower int
	if err := db.Table("fleet.vehicles").Select("current_mileage").Where("id = ?", v.ID()).Scan(&currentAfterLower).Error; err != nil {
		t.Fatalf("read current_mileage after lower: %v", err)
	}
	if currentAfterLower != 42000 {
		t.Fatalf("current_mileage must not regress: want 42000, got %d", currentAfterLower)
	}
}
