package dashboard

import (
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newAggDB sets up an in-memory SQLite database with the tables needed for
// aggregation tests (maintenance_records, fuel_logs, vehicles, mileage_records).
func newAggDB(t *testing.T) *gorm.DB {
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
		`CREATE TABLE fleet.maintenance_records (
			id TEXT PRIMARY KEY, vehicle_id TEXT, category_id TEXT, description TEXT,
			performed_at DATETIME, mileage INTEGER, cost REAL, vendor TEXT,
			notes TEXT, created_by_user_id TEXT, created_at DATETIME,
			updated_at DATETIME, deleted_at DATETIME)`,
		`CREATE TABLE fleet.fuel_logs (
			id TEXT PRIMARY KEY, vehicle_id TEXT, date DATETIME, mileage INTEGER,
			gallons REAL, total_cost REAL, price_per_gallon REAL,
			created_by_user_id TEXT, created_at DATETIME, updated_at DATETIME,
			deleted_at DATETIME)`,
		`CREATE TABLE fleet.mileage_records (
			id TEXT PRIMARY KEY, vehicle_id TEXT, mileage INTEGER,
			recorded_at DATETIME, source TEXT, source_ref_id TEXT,
			created_by_user_id TEXT, created_at DATETIME)`,
	}
	for _, stmt := range ddl {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("ddl: %v", err)
		}
	}
	return db
}

// TestSpendByVehicle_GroupsAndSumsBothSources verifies that SpendByVehicle:
//  1. Groups costs by vehicle_id.
//  2. Sums both maintenance_records.cost AND fuel_logs.total_cost per vehicle.
//  3. Bounds the window to [from, to].
func TestSpendByVehicle_GroupsAndSumsBothSources(t *testing.T) {
	db := newAggDB(t)
	fleetID := "fleet-1"
	v1 := "vehicle-1"
	v2 := "vehicle-2"
	now := time.Now().UTC()
	inWindow := now.Add(-24 * time.Hour)
	outWindow := now.Add(-72 * time.Hour)
	from := now.Add(-48 * time.Hour)
	to := now.Add(time.Hour)

	// Seed vehicles.
	for _, id := range []string{v1, v2} {
		if err := db.Exec(
			`INSERT INTO fleet.vehicles (id,fleet_id,make,model,year,current_mileage,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?)`,
			id, fleetID, "Toyota", "Camry", 2020, 0, now, now).Error; err != nil {
			t.Fatalf("seed vehicle: %v", err)
		}
	}

	// Seed maintenance records — v1: 100+200 in-window; v1: 50 OUT of window; v2: 300 in-window.
	records := []struct {
		id, vehicleID string
		cost          float64
		performedAt   time.Time
	}{
		{"mr-1", v1, 100.0, inWindow},
		{"mr-2", v1, 200.0, inWindow},
		{"mr-3", v1, 50.0, outWindow}, // outside window — must be excluded
		{"mr-4", v2, 300.0, inWindow},
	}
	for _, r := range records {
		if err := db.Exec(
			`INSERT INTO fleet.maintenance_records (id,vehicle_id,category_id,performed_at,cost,created_at,updated_at) VALUES (?,?,'cat-1',?,?,?,?)`,
			r.id, r.vehicleID, r.performedAt, r.cost, now, now).Error; err != nil {
			t.Fatalf("seed maint record: %v", err)
		}
	}

	// Seed fuel logs — v1: 40 in-window; v2: 60 in-window; v2: 999 OUT of window.
	fuels := []struct {
		id, vehicleID string
		totalCost     float64
		date          time.Time
	}{
		{"fl-1", v1, 40.0, inWindow},
		{"fl-2", v2, 60.0, inWindow},
		{"fl-3", v2, 999.0, outWindow}, // outside window — must be excluded
	}
	for _, f := range fuels {
		if err := db.Exec(
			`INSERT INTO fleet.fuel_logs (id,vehicle_id,date,mileage,gallons,total_cost,price_per_gallon,created_at,updated_at) VALUES (?,?,?,0,10,?,4,?,?)`,
			f.id, f.vehicleID, f.date, f.totalCost, now, now).Error; err != nil {
			t.Fatalf("seed fuel log: %v", err)
		}
	}

	agg := NewAggregateProvider(db)
	rows, err := agg.SpendByVehicle(fleetID, from, to)
	if err != nil {
		t.Fatalf("SpendByVehicle: %v", err)
	}

	// Build result map for easy assertion.
	byVehicle := map[string]SpendRow{}
	for _, r := range rows {
		byVehicle[r.VehicleID] = r
	}

	// v1: maintenance 100+200=300; fuel 40 → total 340.
	v1Row, ok := byVehicle[v1]
	if !ok {
		t.Fatal("missing spend row for vehicle-1")
	}
	if v1Row.MaintenanceCost != 300.0 {
		t.Errorf("v1 maintenance cost: want 300, got %.2f", v1Row.MaintenanceCost)
	}
	if v1Row.FuelCost != 40.0 {
		t.Errorf("v1 fuel cost: want 40, got %.2f", v1Row.FuelCost)
	}
	if v1Row.TotalCost != 340.0 {
		t.Errorf("v1 total cost: want 340, got %.2f", v1Row.TotalCost)
	}

	// v2: maintenance 300; fuel 60 → total 360 (999 excluded).
	v2Row, ok := byVehicle[v2]
	if !ok {
		t.Fatal("missing spend row for vehicle-2")
	}
	if v2Row.MaintenanceCost != 300.0 {
		t.Errorf("v2 maintenance cost: want 300, got %.2f", v2Row.MaintenanceCost)
	}
	if v2Row.FuelCost != 60.0 {
		t.Errorf("v2 fuel cost: want 60, got %.2f", v2Row.FuelCost)
	}
	if v2Row.TotalCost != 360.0 {
		t.Errorf("v2 total cost: want 360, got %.2f", v2Row.TotalCost)
	}
}
