package maintenanceschedule

import (
	"errors"
	"testing"

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
