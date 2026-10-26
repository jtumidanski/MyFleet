package maintenancerecord

import (
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/mileage"
)

func bfDiscardLog() logrus.FieldLogger {
	l := logrus.New()
	l.SetOutput(io.Discard)
	return l
}

func bfSeedVehicle(t *testing.T, db *gorm.DB, id string, odometer int, deleted bool) {
	t.Helper()
	var deletedAt *time.Time
	if deleted {
		now := time.Now().UTC()
		deletedAt = &now
	}
	if err := db.Exec(`INSERT INTO fleet.vehicles (id, current_mileage, deleted_at) VALUES (?, ?, ?)`,
		id, odometer, deletedAt).Error; err != nil {
		t.Fatalf("seed vehicle: %v", err)
	}
}

func bfSeedMaintenance(t *testing.T, db *gorm.DB, vehicleID string, miles int, performedAt time.Time, userID string, deleted bool) string {
	t.Helper()
	id := uuid.NewString()
	var deletedAt *time.Time
	if deleted {
		now := time.Now().UTC()
		deletedAt = &now
	}
	if err := db.Exec(`INSERT INTO fleet.maintenance_records (id, vehicle_id, performed_at, mileage, created_by_user_id, deleted_at)
		VALUES (?, ?, ?, ?, ?, ?)`, id, vehicleID, performedAt, miles, userID, deletedAt).Error; err != nil {
		t.Fatalf("seed maintenance: %v", err)
	}
	return id
}

func bfSeedMileageRow(t *testing.T, db *gorm.DB, vehicleID, refID string, miles int, deleted bool) {
	t.Helper()
	var deletedAt *time.Time
	if deleted {
		now := time.Now().UTC()
		deletedAt = &now
	}
	if err := db.Exec(`INSERT INTO fleet.mileage_records (id, vehicle_id, mileage, recorded_at, source, source_ref_id, deleted_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, uuid.NewString(), vehicleID, miles, time.Now().UTC(),
		mileage.SourceMaintenance, refID, deletedAt).Error; err != nil {
		t.Fatalf("seed mileage row: %v", err)
	}
}

func bfCountRows(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	if err := db.Table("fleet.mileage_records").Count(&n).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func bfOdometer(t *testing.T, db *gorm.DB, vehicleID string) int {
	t.Helper()
	var v int
	if err := db.Raw(`SELECT current_mileage FROM fleet.vehicles WHERE id = ?`, vehicleID).Scan(&v).Error; err != nil {
		t.Fatalf("odometer: %v", err)
	}
	return v
}

func TestBackfillMileage_insertsUnlinkedAndAdvances(t *testing.T) {
	db := newTestDB(t)
	a, b := uuid.NewString(), uuid.NewString()
	bfSeedVehicle(t, db, a, 10000, false)
	bfSeedVehicle(t, db, b, 90000, false)
	day1 := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC)
	r1 := bfSeedMaintenance(t, db, a, 12000, day1, "user-1", false)
	r2 := bfSeedMaintenance(t, db, a, 15000, day2, "user-2", false)
	rb := bfSeedMaintenance(t, db, b, 80000, day1, "user-3", false)

	if err := BackfillMileage(db, bfDiscardLog()); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	var rows []mileage.Entity
	if err := db.Where("vehicle_id = ?", a).Find(&rows).Error; err != nil {
		t.Fatalf("find: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("vehicle A rows = %d, want 2", len(rows))
	}
	want := map[string]struct {
		miles int
		at    time.Time
		user  string
	}{r1: {12000, day1, "user-1"}, r2: {15000, day2, "user-2"}}
	for _, r := range rows {
		w, ok := want[r.SourceRefID]
		if !ok {
			t.Fatalf("unexpected source_ref_id %q", r.SourceRefID)
		}
		if r.Source != mileage.SourceMaintenance || r.Mileage != w.miles || !r.RecordedAt.Equal(w.at) || r.CreatedByUserID != w.user {
			t.Errorf("row %+v does not match %+v", r, w)
		}
	}
	if got := bfOdometer(t, db, a); got != 15000 {
		t.Errorf("A odometer = %d, want 15000", got)
	}

	var bRows []mileage.Entity
	if err := db.Where("vehicle_id = ?", b).Find(&bRows).Error; err != nil {
		t.Fatalf("find: %v", err)
	}
	if len(bRows) != 1 || bRows[0].SourceRefID != rb {
		t.Fatalf("vehicle B rows = %+v, want 1 for %s", bRows, rb)
	}
	if got := bfOdometer(t, db, b); got != 90000 {
		t.Errorf("B odometer = %d, want 90000 (never regress)", got)
	}
}

func TestBackfillMileage_skipsLinkedDeletedAndZero(t *testing.T) {
	db := newTestDB(t)
	v, dead := uuid.NewString(), uuid.NewString()
	bfSeedVehicle(t, db, v, 100, false)
	bfSeedVehicle(t, db, dead, 100, true)
	day := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)

	linked := bfSeedMaintenance(t, db, v, 500, day, "u", false)
	bfSeedMileageRow(t, db, v, linked, 500, false)
	retracted := bfSeedMaintenance(t, db, v, 600, day, "u", false)
	bfSeedMileageRow(t, db, v, retracted, 600, true)
	bfSeedMaintenance(t, db, v, 700, day, "u", true)
	bfSeedMaintenance(t, db, v, 0, day, "u", false)
	bfSeedMaintenance(t, db, dead, 800, day, "u", false)
	before := bfCountRows(t, db)

	if err := BackfillMileage(db, bfDiscardLog()); err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if after := bfCountRows(t, db); after != before {
		t.Errorf("rows = %d, want %d (nothing inserted)", after, before)
	}
	if got := bfOdometer(t, db, v); got != 100 {
		t.Errorf("odometer = %d, want 100", got)
	}
}

func TestBackfillMileage_isIdempotent(t *testing.T) {
	db := newTestDB(t)
	v := uuid.NewString()
	bfSeedVehicle(t, db, v, 100, false)
	day := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	bfSeedMaintenance(t, db, v, 500, day, "u", false)
	bfSeedMaintenance(t, db, v, 900, day, "u", false)

	if err := BackfillMileage(db, bfDiscardLog()); err != nil {
		t.Fatalf("first: %v", err)
	}
	first := bfCountRows(t, db)
	if first != 2 {
		t.Fatalf("first run rows = %d, want 2", first)
	}
	if err := BackfillMileage(db, bfDiscardLog()); err != nil {
		t.Fatalf("second: %v", err)
	}
	if second := bfCountRows(t, db); second != first {
		t.Errorf("second run rows = %d, want %d", second, first)
	}
}
