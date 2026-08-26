package maintenanceschedule

import (
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// seedOneTimeSchedule inserts an active one-time schedule and returns its id.
func seedOneTimeSchedule(t *testing.T, db *gorm.DB, vehicleID string) string {
	t.Helper()
	m, err := NewBuilder().SetVehicleID(vehicleID).SetCategoryID("c1").
		SetRecurrenceType("mileage").SetOneTime(true).SetDuePoint(time.Time{}, 60000).Build()
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	created, err := NewAdministrator(db).Insert(m)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	return created.ID()
}

// AdvanceTx's mutual exclusion does not rest on its in-Go `if !e.Active` check —
// that check is only a fast path, and under READ COMMITTED two concurrent
// completions both pass it. The guarantee is that the closing UPDATE re-asserts
// `active`, so a row deactivated since the SELECT matches zero rows and the
// whole completion transaction fails with server.ErrValidation.
//
// This test stages that window deterministically: it deactivates the row and
// then calls the write directly, which is exactly the state the losing
// transaction finds after the winner commits.
//
// What it does NOT cover: the row lock. The package's harness is sqlite
// in-memory, which has no SELECT ... FOR UPDATE and cannot schedule two
// concurrent writers, so the Postgres-only locking clause and the real
// interleaving are unproven here by construction.
func TestApplyAdvance_rejectsARowDeactivatedSinceTheSelect(t *testing.T) {
	db := newCompletionDB(t)
	vehicleID := seedVehicle(t, db, 40000)
	id := seedOneTimeSchedule(t, db, vehicleID)

	// The winning transaction's effect: the row is now inactive.
	if err := db.Table("fleet.maintenance_schedules").Where("id = ?", id).
		Update("active", false).Error; err != nil {
		t.Fatalf("deactivate: %v", err)
	}

	completedAt := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	err := applyAdvance(db, id, map[string]any{
		"last_completed_date":    completedAt,
		"last_completed_mileage": 41000,
		"active":                 false,
	})
	if !errors.Is(err, server.ErrValidation) {
		t.Fatalf("want server.ErrValidation, got %v", err)
	}

	after := readSchedule(t, db, id)
	if !after.LastCompletedDate().IsZero() || after.LastCompletedMileage() != 0 {
		t.Fatalf("the losing completion must write nothing, got %v / %d",
			after.LastCompletedDate(), after.LastCompletedMileage())
	}
}

// The same write against a still-active row succeeds, so the predicate narrows
// the UPDATE rather than disabling it.
func TestApplyAdvance_writesAnActiveRow(t *testing.T) {
	db := newCompletionDB(t)
	vehicleID := seedVehicle(t, db, 40000)
	id := seedOneTimeSchedule(t, db, vehicleID)

	completedAt := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	if err := applyAdvance(db, id, map[string]any{
		"last_completed_date":    completedAt,
		"last_completed_mileage": 41000,
		"active":                 false,
	}); err != nil {
		t.Fatalf("applyAdvance on an active row: %v", err)
	}

	after := readSchedule(t, db, id)
	if !after.LastCompletedDate().Equal(completedAt) || after.LastCompletedMileage() != 41000 {
		t.Fatalf("completion not written, got %v / %d",
			after.LastCompletedDate(), after.LastCompletedMileage())
	}
	if after.Active() {
		t.Fatal("a completed one-time schedule must be deactivated")
	}
}

// End to end through AdvanceTx: a second completion of an already-completed
// one-time schedule is refused. The in-Go guard reports it here, but the
// conditional UPDATE would refuse it just the same.
func TestAdvanceTx_refusesASecondCompletion(t *testing.T) {
	db := newCompletionDB(t)
	vehicleID := seedVehicle(t, db, 40000)
	id := seedOneTimeSchedule(t, db, vehicleID)
	admin := NewAdministrator(db)

	completedAt := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	if err := admin.Advance(id, completedAt, 41000); err != nil {
		t.Fatalf("first completion: %v", err)
	}
	err := admin.Advance(id, completedAt.AddDate(0, 0, 1), 42000)
	if !errors.Is(err, server.ErrValidation) {
		t.Fatalf("want server.ErrValidation on the second completion, got %v", err)
	}

	after := readSchedule(t, db, id)
	if after.LastCompletedMileage() != 41000 {
		t.Fatalf("the refused completion must not overwrite the first: %d", after.LastCompletedMileage())
	}
}
