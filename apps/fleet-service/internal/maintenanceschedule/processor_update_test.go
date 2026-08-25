package maintenanceschedule

import (
	"errors"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// seedHybrid inserts a valid, anchored, never-completed hybrid schedule and
// returns the processor bound to the same db plus the created model.
func seedHybrid(t *testing.T, db *gorm.DB) (*Processor, Model) {
	t.Helper()
	m, err := NewBuilder().SetVehicleID("v1").SetCategoryID("c1").
		SetRecurrenceType("hybrid").SetIntervalMonths(12).SetIntervalMiles(5000).
		SetDuePoint(anchor, 60000).Build()
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	created, err := NewAdministrator(db).Insert(m)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	return NewProcessor(logrus.New(), NewProvider(db), NewAdministrator(db)), created
}

// A PATCH that would leave a hybrid schedule with a zero interval is rejected,
// and the stored row is untouched.
func TestProcessorUpdate_rejectsZeroIntervalOnHybrid(t *testing.T) {
	db := newCompletionDB(t)
	proc, created := seedHybrid(t, db)

	_, err := proc.Update(created.ID(), func(m Model) Model {
		return m.WithRecurrence("hybrid", 0, m.IntervalMiles())
	})
	if !errors.Is(err, server.ErrValidation) {
		t.Fatalf("want server.ErrValidation, got %v", err)
	}

	after, err := NewProvider(db).GetByID(created.ID())
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if after.IntervalMonths() != 12 {
		t.Fatalf("a rejected PATCH must not write: interval_months = %d want 12", after.IntervalMonths())
	}
}

// A PATCH that sets oneTime while leaving the intervals in place is rejected
// (FR-OT-3): a one-time schedule must not carry intervals.
func TestProcessorUpdate_rejectsOneTimeWithIntervals(t *testing.T) {
	db := newCompletionDB(t)
	proc, created := seedHybrid(t, db)

	_, err := proc.Update(created.ID(), func(m Model) Model { return m.WithOneTime(true) })
	if !errors.Is(err, server.ErrValidation) {
		t.Fatalf("want server.ErrValidation, got %v", err)
	}
}

// The conversion PATCH: oneTime=false, a recurrence type + interval, active,
// and a cleared due point. Next-due then derives from the completion point the
// completion flow already recorded. This fails outright unless
// Administrator.Update names one_time / due_date / due_mileage in its column
// map — the failure mode where the PATCH appears to succeed and changes nothing.
func TestProcessorUpdate_conversionDerivesFromCompletionPoint(t *testing.T) {
	db := newCompletionDB(t)
	completedAt := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	m, err := NewBuilder().SetVehicleID("v1").SetCategoryID("c1").
		SetRecurrenceType("time").SetOneTime(true).SetDuePoint(anchor, 0).Build()
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	created, err := NewAdministrator(db).Insert(
		m.WithLastCompleted(completedAt, 42000).WithDuePoint(time.Time{}, 0))
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	// Entity.Active carries a `default:true` GORM tag, so Insert cannot persist
	// active=false (GORM substitutes the tag's default for an explicit Go zero
	// value at Create time). Flip it with an explicit update instead.
	if err := db.Table("fleet.maintenance_schedules").Where("id = ?", created.ID()).
		Update("active", false).Error; err != nil {
		t.Fatalf("seed inactive: %v", err)
	}
	seeded, err := NewProvider(db).GetByID(created.ID())
	if err != nil {
		t.Fatalf("reload seeded schedule: %v", err)
	}
	if seeded.Active() {
		t.Fatalf("seed inactive: row is still active")
	}

	proc := NewProcessor(logrus.New(), NewProvider(db), NewAdministrator(db))
	updated, err := proc.Update(created.ID(), func(m Model) Model {
		return m.WithOneTime(false).WithRecurrence("time", 12, 0).
			WithDuePoint(time.Time{}, 0).WithActive(true)
	})
	if err != nil {
		t.Fatalf("conversion PATCH: %v", err)
	}
	if updated.OneTime() || !updated.Active() {
		t.Fatalf("want a recurring, active schedule, got oneTime=%v active=%v", updated.OneTime(), updated.Active())
	}
	if !updated.DueDate().IsZero() || updated.DueMileage() != 0 {
		t.Fatalf("the anchor must be cleared, got %v / %d", updated.DueDate(), updated.DueMileage())
	}
	if want := completedAt.AddDate(0, 12, 0); !updated.NextDueDate().Equal(want) {
		t.Fatalf("next_due_date = %v want %v", updated.NextDueDate(), want)
	}
}

// The inverse: a PATCH that sets a one-time schedule's due point must persist
// it rather than silently dropping it at the column map.
func TestProcessorUpdate_persistsDuePoint(t *testing.T) {
	db := newCompletionDB(t)
	proc, created := seedHybrid(t, db)

	moved := anchor.AddDate(0, 1, 0)
	updated, err := proc.Update(created.ID(), func(m Model) Model {
		return m.WithDuePoint(moved, 70000)
	})
	if err != nil {
		t.Fatalf("PATCH: %v", err)
	}
	if !updated.DueDate().Equal(moved) || updated.DueMileage() != 70000 {
		t.Fatalf("due point not persisted: %v / %d", updated.DueDate(), updated.DueMileage())
	}
}
