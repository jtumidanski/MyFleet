package mileage

import (
	"testing"
	"time"

	"gorm.io/gorm"
)

func reading(vehicleID string, miles int, source, ref string) Model {
	return NewBuilder().SetVehicleID(vehicleID).SetMileage(miles).SetRecordedAt(time.Now().UTC()).
		SetSource(source).SetSourceRefID(ref).Build()
}

func appendIn(t *testing.T, db *gorm.DB, m Model) Model {
	t.Helper()
	var out Model
	err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		out, err = NewAdministrator(db).AppendTx(tx, m)
		return err
	})
	if err != nil {
		t.Fatalf("AppendTx: %v", err)
	}
	return out
}

func retractIn(t *testing.T, db *gorm.DB, vehicleID, source, ref string) {
	t.Helper()
	err := db.Transaction(func(tx *gorm.DB) error {
		return NewAdministrator(db).RetractTx(tx, vehicleID, source, ref)
	})
	if err != nil {
		t.Fatalf("RetractTx: %v", err)
	}
}

func seedRow(t *testing.T, db *gorm.DB, vehicleID string, miles int, source, ref string) {
	t.Helper()
	m := reading(vehicleID, miles, source, ref)
	if err := db.Create(ptr(m.ToEntity())).Error; err != nil {
		t.Fatalf("seed row: %v", err)
	}
}

func ptr(e Entity) *Entity { return &e }

func deletedAtSet(t *testing.T, db *gorm.DB, vehicleID, source, ref string) int64 {
	t.Helper()
	var n int64
	if err := db.Raw(`SELECT COUNT(*) FROM fleet.mileage_records
		WHERE vehicle_id = ? AND source = ? AND source_ref_id = ? AND deleted_at IS NOT NULL`,
		vehicleID, source, ref).Scan(&n).Error; err != nil {
		t.Fatalf("count deleted: %v", err)
	}
	return n
}

func TestAppendTx_advancesOnHigherReading(t *testing.T) {
	db := newTestDB(t)
	seedVehicle(t, db, "v1", 1000)
	got := appendIn(t, db, reading("v1", 1500, SourceManual, ""))
	if got.Flagged() {
		t.Error("expected not flagged")
	}
	if c := currentMileage(t, db, "v1"); c != 1500 {
		t.Errorf("mileage = %d, want 1500", c)
	}
	var n int64
	db.Raw(`SELECT COUNT(*) FROM fleet.mileage_records WHERE vehicle_id = 'v1' AND deleted_at IS NULL AND source = 'manual'`).Scan(&n)
	if n != 1 {
		t.Errorf("manual live rows = %d, want 1", n)
	}
}

func TestAppendTx_equalReadingIsNotFlagged(t *testing.T) {
	db := newTestDB(t)
	seedVehicle(t, db, "v1", 1000)
	got := appendIn(t, db, reading("v1", 1000, SourceManual, ""))
	if got.Flagged() {
		t.Error("expected not flagged")
	}
	if c := currentMileage(t, db, "v1"); c != 1000 {
		t.Errorf("mileage = %d, want 1000", c)
	}
}

func TestAppendTx_lowerReadingIsKeptAndFlagged(t *testing.T) {
	db := newTestDB(t)
	seedVehicle(t, db, "v1", 2000)
	got := appendIn(t, db, reading("v1", 1500, SourceManual, ""))
	if !got.Flagged() {
		t.Error("expected flagged")
	}
	if c := currentMileage(t, db, "v1"); c != 2000 {
		t.Errorf("mileage = %d, want 2000", c)
	}
	if n := liveRows(t, db, "v1"); n != 1 {
		t.Errorf("live rows = %d, want 1", n)
	}
}

func TestAppendTx_deletedVehicleUntouched(t *testing.T) {
	db := newTestDB(t)
	seedDeletedVehicle(t, db, "v1", 1000)
	appendIn(t, db, reading("v1", 5000, SourceManual, ""))
	if c := currentMileage(t, db, "v1"); c != 1000 {
		t.Errorf("mileage = %d, want 1000", c)
	}
}

func TestRetractTx_recomputesToRemainingMax(t *testing.T) {
	db := newTestDB(t)
	seedVehicle(t, db, "v1", 3000)
	seedRow(t, db, "v1", 1000, SourceManual, "")
	seedRow(t, db, "v1", 3000, SourceMaintenance, "r1")
	retractIn(t, db, "v1", SourceMaintenance, "r1")
	if c := currentMileage(t, db, "v1"); c != 1000 {
		t.Errorf("mileage = %d, want 1000", c)
	}
	if n := deletedAtSet(t, db, "v1", SourceMaintenance, "r1"); n != 1 {
		t.Errorf("deleted r1 rows = %d, want 1", n)
	}
}

func TestRetractTx_nonMaxReadingLeavesOdometer(t *testing.T) {
	db := newTestDB(t)
	seedVehicle(t, db, "v1", 3000)
	seedRow(t, db, "v1", 3000, SourceManual, "")
	seedRow(t, db, "v1", 1000, SourceMaintenance, "r1")
	retractIn(t, db, "v1", SourceMaintenance, "r1")
	if c := currentMileage(t, db, "v1"); c != 3000 {
		t.Errorf("mileage = %d, want 3000", c)
	}
}

func TestRetractTx_noMatchIsNoOp(t *testing.T) {
	db := newTestDB(t)
	seedVehicle(t, db, "v1", 4000)
	seedRow(t, db, "v1", 1000, SourceManual, "")
	retractIn(t, db, "v1", SourceMaintenance, "nope")
	if c := currentMileage(t, db, "v1"); c != 4000 {
		t.Errorf("mileage = %d, want 4000", c)
	}
	if n := liveRows(t, db, "v1"); n != 1 {
		t.Errorf("live rows = %d, want 1", n)
	}
}

func TestRetractTx_noLiveRowsRemainLeavesOdometer(t *testing.T) {
	db := newTestDB(t)
	seedVehicle(t, db, "v1", 2500)
	seedRow(t, db, "v1", 2000, SourceFuel, "f1")
	retractIn(t, db, "v1", SourceFuel, "f1")
	if c := currentMileage(t, db, "v1"); c != 2500 {
		t.Errorf("mileage = %d, want 2500", c)
	}
	if n := liveRows(t, db, "v1"); n != 0 {
		t.Errorf("live rows = %d, want 0", n)
	}
}

func TestRetractTx_onlyMatchingTupleDeleted(t *testing.T) {
	db := newTestDB(t)
	seedVehicle(t, db, "v1", 5000)
	seedVehicle(t, db, "v2", 5000)
	seedRow(t, db, "v1", 1000, SourceMaintenance, "r1")
	seedRow(t, db, "v1", 2000, SourceMaintenance, "r1")
	seedRow(t, db, "v1", 3000, SourceFuel, "r1")
	seedRow(t, db, "v1", 4000, SourceMaintenance, "r2")
	seedRow(t, db, "v2", 1000, SourceMaintenance, "r1")
	retractIn(t, db, "v1", SourceMaintenance, "r1")
	if n := deletedAtSet(t, db, "v1", SourceMaintenance, "r1"); n != 2 {
		t.Errorf("deleted matching rows = %d, want 2", n)
	}
	if n := liveRows(t, db, "v1"); n != 2 {
		t.Errorf("v1 live rows = %d, want 2", n)
	}
	if n := liveRows(t, db, "v2"); n != 1 {
		t.Errorf("v2 live rows = %d, want 1", n)
	}
}

func TestValidSource(t *testing.T) {
	for _, s := range []string{SourceFuel, SourceMaintenance, SourceManual} {
		if !ValidSource(s) {
			t.Errorf("ValidSource(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"", "MANUAL", "obd"} {
		if ValidSource(s) {
			t.Errorf("ValidSource(%q) = true, want false", s)
		}
	}
}
