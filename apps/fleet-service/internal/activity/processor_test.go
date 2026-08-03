package activity

import (
	"encoding/json"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newActivityDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// Schema-qualified TableName targets the "fleet" schema on Postgres; SQLite
	// has no schemas, so attach an in-memory database aliased "fleet" and create
	// the activity_events table with explicit DDL mirroring the GORM entity.
	if err := db.Exec("ATTACH DATABASE ':memory:' AS fleet").Error; err != nil {
		t.Fatalf("attach fleet schema: %v", err)
	}
	if err := db.Exec(`CREATE TABLE fleet.activity_events (
		id TEXT PRIMARY KEY, fleet_id TEXT, vehicle_id TEXT, actor_user_id TEXT,
		type TEXT, payload BLOB, created_at DATETIME,
		deleted_at DATETIME, purge_operation_id TEXT)`).Error; err != nil {
		t.Fatalf("ddl: %v", err)
	}
	return db
}

// TestRecord verifies Record builds a well-formed activity_events row inside the
// caller's transaction: actor/type/fleet are correct and the payload is stored
// as JSON-serializable bytes.
func TestRecord(t *testing.T) {
	db := newActivityDB(t)

	vehicleID := "veh-1"
	err := db.Transaction(func(tx *gorm.DB) error {
		return Record(tx, "user-1", "vehicle.created", "fleet-1", &vehicleID, map[string]any{
			"vehicle_id": vehicleID,
			"nickname":   "Daily Driver",
		})
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}

	var e Entity
	if err := db.Table("fleet.activity_events").First(&e).Error; err != nil {
		t.Fatalf("read row: %v", err)
	}
	if e.ID == "" {
		t.Fatal("expected a generated id")
	}
	if e.FleetID != "fleet-1" {
		t.Fatalf("fleet_id=%q want fleet-1", e.FleetID)
	}
	if e.ActorUserID != "user-1" {
		t.Fatalf("actor_user_id=%q want user-1", e.ActorUserID)
	}
	if e.Type != "vehicle.created" {
		t.Fatalf("type=%q want vehicle.created", e.Type)
	}
	if e.VehicleID == nil || *e.VehicleID != vehicleID {
		t.Fatalf("vehicle_id=%v want %s", e.VehicleID, vehicleID)
	}
	if e.CreatedAt.IsZero() {
		t.Fatal("expected created_at to be set")
	}

	// Payload must be valid JSON carrying the supplied fields.
	var decoded map[string]any
	if err := json.Unmarshal(e.Payload, &decoded); err != nil {
		t.Fatalf("payload not JSON: %v", err)
	}
	if decoded["nickname"] != "Daily Driver" {
		t.Fatalf("payload nickname=%v want Daily Driver", decoded["nickname"])
	}
}

// TestRecord_nilVehicle verifies fleet-level events (no vehicle) are accepted.
func TestRecord_nilVehicle(t *testing.T) {
	db := newActivityDB(t)
	err := db.Transaction(func(tx *gorm.DB) error {
		return Record(tx, "user-1", "member.invited", "fleet-1", nil, map[string]any{"email": "x@y.z"})
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	var e Entity
	if err := db.Table("fleet.activity_events").First(&e).Error; err != nil {
		t.Fatalf("read row: %v", err)
	}
	if e.VehicleID != nil {
		t.Fatalf("vehicle_id=%v want nil", e.VehicleID)
	}
}
