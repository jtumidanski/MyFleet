package mileage

import (
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newTestDB opens an in-memory sqlite database with the "fleet" schema attached
// and the vehicles and mileage_records tables created with explicit DDL
// (GORM AutoMigrate mishandles schema-qualified index names on SQLite).
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	// ATTACH is per-connection; pin to one so every query sees the schema.
	sqlDB.SetMaxOpenConns(1)
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
	}
	for _, s := range ddl {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("ddl: %v", err)
		}
	}
	return db
}

func seedVehicle(t *testing.T, db *gorm.DB, id string, miles int) {
	t.Helper()
	if err := db.Exec(`INSERT INTO fleet.vehicles (id, fleet_id, current_mileage, deleted_at) VALUES (?, ?, ?, NULL)`,
		id, "fleet-1", miles).Error; err != nil {
		t.Fatalf("seed vehicle: %v", err)
	}
}

func seedDeletedVehicle(t *testing.T, db *gorm.DB, id string, miles int) {
	t.Helper()
	if err := db.Exec(`INSERT INTO fleet.vehicles (id, fleet_id, current_mileage, deleted_at) VALUES (?, ?, ?, ?)`,
		id, "fleet-1", miles, time.Now().UTC()).Error; err != nil {
		t.Fatalf("seed deleted vehicle: %v", err)
	}
}

func currentMileage(t *testing.T, db *gorm.DB, id string) int {
	t.Helper()
	var m int
	if err := db.Raw(`SELECT current_mileage FROM fleet.vehicles WHERE id = ?`, id).Scan(&m).Error; err != nil {
		t.Fatalf("read mileage: %v", err)
	}
	return m
}

func liveRows(t *testing.T, db *gorm.DB, vehicleID string) int64 {
	t.Helper()
	var n int64
	if err := db.Raw(`SELECT COUNT(*) FROM fleet.mileage_records WHERE vehicle_id = ? AND deleted_at IS NULL`, vehicleID).Scan(&n).Error; err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return n
}
