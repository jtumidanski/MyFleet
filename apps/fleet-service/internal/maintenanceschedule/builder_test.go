package maintenanceschedule

import (
	"errors"
	"testing"
	"time"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

var anchor = time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC)

// build applies the common identity fields so each case only states what it is
// actually exercising.
func build(fn func(*Builder)) (Model, error) {
	b := NewBuilder().SetVehicleID("v1").SetCategoryID("c1")
	fn(b)
	return b.Build()
}

func TestBuild_validationMatrix(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*Builder)
		wantErr bool
	}{
		// --- recurring: intervals AND a first-due anchor on every covered axis.
		{"recurring time: complete", func(b *Builder) {
			b.SetRecurrenceType("time").SetIntervalMonths(6).SetDuePoint(anchor, 0)
		}, false},
		{"recurring time: no interval", func(b *Builder) {
			b.SetRecurrenceType("time").SetDuePoint(anchor, 0)
		}, true},
		{"recurring time: no anchor", func(b *Builder) {
			b.SetRecurrenceType("time").SetIntervalMonths(6)
		}, true},
		{"recurring mileage: complete", func(b *Builder) {
			b.SetRecurrenceType("mileage").SetIntervalMiles(5000).SetDuePoint(time.Time{}, 60000)
		}, false},
		{"recurring mileage: no interval", func(b *Builder) {
			b.SetRecurrenceType("mileage").SetDuePoint(time.Time{}, 60000)
		}, true},
		{"recurring mileage: no anchor", func(b *Builder) {
			b.SetRecurrenceType("mileage").SetIntervalMiles(5000)
		}, true},
		{"recurring hybrid: complete", func(b *Builder) {
			b.SetRecurrenceType("hybrid").SetIntervalMonths(6).SetIntervalMiles(5000).SetDuePoint(anchor, 60000)
		}, false},
		{"recurring hybrid: missing the mileage anchor", func(b *Builder) {
			b.SetRecurrenceType("hybrid").SetIntervalMonths(6).SetIntervalMiles(5000).SetDuePoint(anchor, 0)
		}, true},
		{"recurring hybrid: missing the date anchor", func(b *Builder) {
			b.SetRecurrenceType("hybrid").SetIntervalMonths(6).SetIntervalMiles(5000).SetDuePoint(time.Time{}, 60000)
		}, true},

		// --- one-time: a due point on every covered axis and NO interval.
		{"one-time time: complete", func(b *Builder) {
			b.SetRecurrenceType("time").SetOneTime(true).SetDuePoint(anchor, 0)
		}, false},
		{"one-time time: no due date", func(b *Builder) {
			b.SetRecurrenceType("time").SetOneTime(true)
		}, true},
		{"one-time mileage: complete", func(b *Builder) {
			b.SetRecurrenceType("mileage").SetOneTime(true).SetDuePoint(time.Time{}, 60000)
		}, false},
		{"one-time mileage: zero due mileage", func(b *Builder) {
			b.SetRecurrenceType("mileage").SetOneTime(true).SetDuePoint(time.Time{}, 0)
		}, true},
		{"one-time hybrid: complete", func(b *Builder) {
			b.SetRecurrenceType("hybrid").SetOneTime(true).SetDuePoint(anchor, 60000)
		}, false},
		{"one-time with intervalMonths is rejected", func(b *Builder) {
			b.SetRecurrenceType("time").SetOneTime(true).SetDuePoint(anchor, 0).SetIntervalMonths(6)
		}, true},
		{"one-time with intervalMiles is rejected", func(b *Builder) {
			b.SetRecurrenceType("mileage").SetOneTime(true).SetDuePoint(time.Time{}, 60000).SetIntervalMiles(5000)
		}, true},

		// --- pre-existing invariants.
		{"unknown recurrence type", func(b *Builder) {
			b.SetRecurrenceType("once").SetDuePoint(anchor, 0)
		}, true},
		{"empty recurrence type", func(b *Builder) {
			b.SetDuePoint(anchor, 0)
		}, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := build(c.mutate)
			if c.wantErr {
				if !errors.Is(err, server.ErrValidation) {
					t.Fatalf("want server.ErrValidation, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestBuild_requiresIdentity(t *testing.T) {
	if _, err := NewBuilder().SetCategoryID("c1").SetRecurrenceType("time").
		SetIntervalMonths(6).SetDuePoint(anchor, 0).Build(); !errors.Is(err, server.ErrValidation) {
		t.Fatalf("missing vehicleID: want server.ErrValidation, got %v", err)
	}
	if _, err := NewBuilder().SetVehicleID("v1").SetRecurrenceType("time").
		SetIntervalMonths(6).SetDuePoint(anchor, 0).Build(); !errors.Is(err, server.ErrValidation) {
		t.Fatalf("missing categoryID: want server.ErrValidation, got %v", err)
	}
}

// The anchor requirement must NOT fire on a schedule that already has a
// completion point to derive from — FR-ANCHOR-3 clears the anchor on purpose,
// so without this carve-out the completion flow would write a row its own
// validator rejects.
func TestValidate_completedRecurringNeedsNoAnchor(t *testing.T) {
	m := Model{
		vehicleID:            "v1",
		categoryID:           "c1",
		recurrenceType:       "hybrid",
		intervalMonths:       12,
		intervalMiles:        5000,
		lastCompletedDate:    base,
		lastCompletedMileage: 42000,
		active:               true,
	}
	if err := validate(m); err != nil {
		t.Fatalf("a completed recurring schedule must validate without an anchor: %v", err)
	}
}

// A completed one-time schedule is deactivated with its anchor cleared. That is
// a valid terminal state and must validate, or the row becomes un-PATCHable.
func TestValidate_completedOneTimeIsInactiveAndValid(t *testing.T) {
	m := Model{
		vehicleID:            "v1",
		categoryID:           "c1",
		recurrenceType:       "mileage",
		oneTime:              true,
		lastCompletedMileage: 60000,
		active:               false,
	}
	if err := validate(m); err != nil {
		t.Fatalf("a completed, deactivated one-time schedule must validate: %v", err)
	}
}

// Reactivating a one-time schedule without giving it a due point would produce
// a live row with no resolvable due date — permanently overdue.
func TestValidate_reactivatedOneTimeNeedsAnAnchor(t *testing.T) {
	m := Model{
		vehicleID:            "v1",
		categoryID:           "c1",
		recurrenceType:       "mileage",
		oneTime:              true,
		lastCompletedMileage: 60000,
		active:               true,
	}
	if !errors.Is(validate(m), server.ErrValidation) {
		t.Fatal("an active one-time schedule with no due point must be rejected")
	}
}

// Build derives the stored status from the VEHICLE's current mileage, not from
// last_completed_mileage. RecomputeAll compares against this stored value to
// find the transition edge into overdue, so a wrong initial value suppresses or
// spuriously fires the first schedule.overdue event.
func TestBuild_usesCurrentMileageForInitialStatus(t *testing.T) {
	m, err := build(func(b *Builder) {
		b.SetRecurrenceType("mileage").SetOneTime(true).
			SetDuePoint(time.Time{}, 60000).SetCurrentMileage(59900)
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if m.Status() != "upcoming" {
		t.Fatalf("status = %q want upcoming (59900 is within 500 of 60000)", m.Status())
	}
	if m.Severity() != "recommended" {
		t.Fatalf("severity = %q want recommended", m.Severity())
	}
	if m.NextDueMileage() != 60000 {
		t.Fatalf("next_due_mileage = %d want 60000", m.NextDueMileage())
	}
}

// A brand-new recurring schedule anchored in the future is ok, not overdue —
// the defect FR-ANCHOR-1 exists to fix.
func TestBuild_newRecurringScheduleIsNotBornOverdue(t *testing.T) {
	future := time.Now().UTC().AddDate(0, 6, 0)
	m, err := build(func(b *Builder) {
		b.SetRecurrenceType("time").SetIntervalMonths(6).SetDuePoint(future, 0)
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if m.Status() != "ok" {
		t.Fatalf("status = %q want ok", m.Status())
	}
}
