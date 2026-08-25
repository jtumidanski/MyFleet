package maintenanceschedule

import (
	"testing"
	"time"

	"gorm.io/gorm"
)

// insertRaw writes a schedule row directly, bypassing the builder, so the test
// can create exactly the pre-task-030 shapes the backfill exists to repair.
func insertRaw(t *testing.T, db *gorm.DB, cols map[string]any) {
	t.Helper()
	if err := db.Table("fleet.maintenance_schedules").Create(cols).Error; err != nil {
		t.Fatalf("insert raw schedule: %v", err)
	}
}

func readSchedule(t *testing.T, db *gorm.DB, id string) Model {
	t.Helper()
	m, err := NewProvider(db).GetByID(id)
	if err != nil {
		t.Fatalf("read schedule %s: %v", id, err)
	}
	return m
}

func TestBackfill_anchorsUncompletedSchedules(t *testing.T) {
	db := newCompletionDB(t)
	vehicleID := seedVehicle(t, db, 40000)
	createdAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

	insertRaw(t, db, map[string]any{
		"id": "s-time", "vehicle_id": vehicleID, "category_id": "c1",
		"recurrence_type": "time", "interval_months": 6, "interval_miles": 0,
		"one_time": false, "due_mileage": 0, "last_completed_mileage": 0,
		"active": true, "status": "overdue", "severity": "urgent", "created_at": createdAt,
	})
	insertRaw(t, db, map[string]any{
		"id": "s-mileage", "vehicle_id": vehicleID, "category_id": "c1",
		"recurrence_type": "mileage", "interval_months": 0, "interval_miles": 5000,
		"one_time": false, "due_mileage": 0, "last_completed_mileage": 0,
		"active": true, "status": "overdue", "severity": "urgent", "created_at": createdAt,
	})
	insertRaw(t, db, map[string]any{
		"id": "s-hybrid", "vehicle_id": vehicleID, "category_id": "c1",
		"recurrence_type": "hybrid", "interval_months": 6, "interval_miles": 5000,
		"one_time": false, "due_mileage": 0, "last_completed_mileage": 0,
		"active": true, "status": "overdue", "severity": "urgent", "created_at": createdAt,
	})

	if err := backfill(db, now); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	wantDate := createdAt.AddDate(0, 6, 0)

	timed := readSchedule(t, db, "s-time")
	if !timed.DueDate().Equal(wantDate) {
		t.Errorf("s-time due_date = %v want %v", timed.DueDate(), wantDate)
	}
	if timed.DueMileage() != 0 {
		t.Errorf("s-time must get no mileage anchor, got %d", timed.DueMileage())
	}
	// The stored next-due and status are refreshed in the same pass, so the
	// internal reminder feed (which reads the stored columns) does not carry a
	// stale token until the next hourly recompute.
	if !timed.NextDueDate().Equal(wantDate) {
		t.Errorf("s-time next_due_date = %v want %v", timed.NextDueDate(), wantDate)
	}
	if timed.Status() != "ok" {
		t.Errorf("s-time status = %q want ok", timed.Status())
	}

	miled := readSchedule(t, db, "s-mileage")
	if miled.DueMileage() != 45000 {
		t.Errorf("s-mileage due_mileage = %d want 45000 (40000 + 5000)", miled.DueMileage())
	}
	if !miled.DueDate().IsZero() {
		t.Errorf("s-mileage must get no date anchor, got %v", miled.DueDate())
	}
	if miled.NextDueMileage() != 45000 {
		t.Errorf("s-mileage next_due_mileage = %d want 45000", miled.NextDueMileage())
	}

	hybrid := readSchedule(t, db, "s-hybrid")
	if !hybrid.DueDate().Equal(wantDate) || hybrid.DueMileage() != 45000 {
		t.Errorf("s-hybrid anchors = %v / %d want %v / 45000", hybrid.DueDate(), hybrid.DueMileage(), wantDate)
	}
}

// Rows that have been completed at least once derive next-due from interval
// arithmetic and legitimately hold no anchor — giving them one would pin them
// to a due point they already passed.
func TestBackfill_leavesCompletedAndAnchoredRowsAlone(t *testing.T) {
	db := newCompletionDB(t)
	vehicleID := seedVehicle(t, db, 40000)
	createdAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	completedAt := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	existing := time.Date(2027, 5, 5, 0, 0, 0, 0, time.UTC)

	insertRaw(t, db, map[string]any{
		"id": "s-completed", "vehicle_id": vehicleID, "category_id": "c1",
		"recurrence_type": "hybrid", "interval_months": 6, "interval_miles": 5000,
		"one_time": false, "due_mileage": 0, "last_completed_date": completedAt,
		"last_completed_mileage": 38000, "active": true, "created_at": createdAt,
	})
	insertRaw(t, db, map[string]any{
		"id": "s-anchored", "vehicle_id": vehicleID, "category_id": "c1",
		"recurrence_type": "time", "interval_months": 6, "interval_miles": 0,
		"one_time": false, "due_date": existing, "due_mileage": 0,
		"last_completed_mileage": 0, "active": true, "created_at": createdAt,
	})
	insertRaw(t, db, map[string]any{
		"id": "s-onetime", "vehicle_id": vehicleID, "category_id": "c1",
		"recurrence_type": "mileage", "interval_months": 0, "interval_miles": 0,
		"one_time": true, "due_mileage": 60000, "last_completed_mileage": 0,
		"active": true, "created_at": createdAt,
	})

	if err := backfill(db, time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	completed := readSchedule(t, db, "s-completed")
	if !completed.DueDate().IsZero() || completed.DueMileage() != 0 {
		t.Errorf("a completed row must keep no anchor, got %v / %d", completed.DueDate(), completed.DueMileage())
	}
	if got := readSchedule(t, db, "s-anchored").DueDate(); !got.Equal(existing) {
		t.Errorf("an already-anchored row must be untouched, got %v want %v", got, existing)
	}
	if got := readSchedule(t, db, "s-onetime").DueMileage(); got != 60000 {
		t.Errorf("a one-time row must be untouched, got %d want 60000", got)
	}
}

// The selection predicate must make a second run a no-op: after the first pass
// every repaired row has at least one anchor set and no longer matches.
func TestBackfill_isIdempotent(t *testing.T) {
	db := newCompletionDB(t)
	vehicleID := seedVehicle(t, db, 40000)
	createdAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	for _, id := range []string{"s-time", "s-mileage", "s-hybrid"} {
		recur := map[string]string{"s-time": "time", "s-mileage": "mileage", "s-hybrid": "hybrid"}[id]
		insertRaw(t, db, map[string]any{
			"id": id, "vehicle_id": vehicleID, "category_id": "c1",
			"recurrence_type": recur, "interval_months": 6, "interval_miles": 5000,
			"one_time": false, "due_mileage": 0, "last_completed_mileage": 0,
			"active": true, "created_at": createdAt,
		})
	}

	first := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	if err := backfill(db, first); err != nil {
		t.Fatalf("first backfill: %v", err)
	}
	before := map[string]Model{}
	for _, id := range []string{"s-time", "s-mileage", "s-hybrid"} {
		before[id] = readSchedule(t, db, id)
	}

	// A LATER "now" and a moved odometer would both change the result if the
	// second run touched anything.
	if err := db.Table("fleet.vehicles").Where("id = ?", vehicleID).
		Update("current_mileage", 55000).Error; err != nil {
		t.Fatalf("move odometer: %v", err)
	}
	if err := backfill(db, first.AddDate(1, 0, 0)); err != nil {
		t.Fatalf("second backfill: %v", err)
	}

	for id, was := range before {
		now := readSchedule(t, db, id)
		if !now.DueDate().Equal(was.DueDate()) || now.DueMileage() != was.DueMileage() {
			t.Errorf("%s changed on the second run: %v/%d -> %v/%d",
				id, was.DueDate(), was.DueMileage(), now.DueDate(), now.DueMileage())
		}
	}
}

// The exported entry point exists and is safe to call against a schema with no
// matching rows — the shape every boot after the first one sees.
func TestBackfill_noMatchingRows(t *testing.T) {
	db := newCompletionDB(t)
	if err := Backfill(db); err != nil {
		t.Fatalf("Backfill on an empty table: %v", err)
	}
}
