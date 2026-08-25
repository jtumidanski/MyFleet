package maintenanceschedule

import (
	"testing"
	"time"
)

// The due point round-trips through the entity boundary in both directions.
// due_date is nullable (a zero time is NULL) while due_mileage is a plain int
// where 0 means unset — the same split the file already uses for
// last_completed_* and next_due_*.
func TestEntityRoundTrip_duePoint(t *testing.T) {
	due := time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC)

	m := Model{
		id:             "s1",
		vehicleID:      "v1",
		categoryID:     "c1",
		recurrenceType: "hybrid",
		oneTime:        true,
		dueDate:        due,
		dueMileage:     60000,
		active:         true,
	}

	e := m.ToEntity()
	if !e.OneTime {
		t.Error("one_time must survive ToEntity")
	}
	if e.DueDate == nil || !e.DueDate.Equal(due) {
		t.Errorf("due_date = %v want %v", e.DueDate, due)
	}
	if e.DueMileage != 60000 {
		t.Errorf("due_mileage = %d want 60000", e.DueMileage)
	}

	back := Make(e)
	if !back.OneTime() || !back.DueDate().Equal(due) || back.DueMileage() != 60000 {
		t.Fatalf("round-trip lost the due point: %+v", back)
	}
}

func TestEntityRoundTrip_zeroDueDateIsNull(t *testing.T) {
	e := Model{id: "s1", recurrenceType: "mileage", dueMileage: 60000}.ToEntity()
	if e.DueDate != nil {
		t.Fatalf("a zero due date must map to NULL, got %v", e.DueDate)
	}
	if got := Make(e).DueDate(); !got.IsZero() {
		t.Fatalf("NULL must map back to a zero time, got %v", got)
	}
}

func TestModelCopyWiths_duePoint(t *testing.T) {
	due := time.Date(2027, 3, 1, 0, 0, 0, 0, time.UTC)
	original := Model{recurrenceType: "hybrid"}

	withOneTime := original.WithOneTime(true)
	if original.OneTime() {
		t.Error("WithOneTime must not mutate the receiver")
	}
	if !withOneTime.OneTime() {
		t.Error("WithOneTime(true) must set the flag on the copy")
	}

	anchored := original.WithDuePoint(due, 60000)
	if !original.DueDate().IsZero() || original.DueMileage() != 0 {
		t.Error("WithDuePoint must not mutate the receiver")
	}
	if !anchored.DueDate().Equal(due) || anchored.DueMileage() != 60000 {
		t.Errorf("WithDuePoint lost values: %v / %d", anchored.DueDate(), anchored.DueMileage())
	}

	cleared := anchored.WithDuePoint(time.Time{}, 0)
	if !cleared.DueDate().IsZero() || cleared.DueMileage() != 0 {
		t.Error("WithDuePoint(zero, 0) must clear the anchor")
	}
}

// AsSchedule is the ONLY bridge from the model into the recurrence engine. A
// field that stops at this boundary is invisible to NextDue and every consumer
// downstream of it.
func TestAsSchedule_carriesDuePoint(t *testing.T) {
	due := time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC)
	s := Model{recurrenceType: "hybrid", oneTime: true, dueDate: due, dueMileage: 60000}.AsSchedule()
	if !s.OneTime || !s.DueDate.Equal(due) || s.DueMileage != 60000 {
		t.Fatalf("AsSchedule dropped the due point: %+v", s)
	}
}
