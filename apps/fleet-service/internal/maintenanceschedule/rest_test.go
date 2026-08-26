package maintenanceschedule

import (
	"encoding/json"
	"testing"
	"time"
)

// attrsJSON renders a model's attributes to a generic map so the test can
// assert on KEY PRESENCE, which is the actual contract with the frontend —
// `oneTime` must always be emitted, the due fields only when set.
func attrsJSON(t *testing.T, m Model) map[string]any {
	t.Helper()
	b, err := json.Marshal(Transform(m).Attributes)
	if err != nil {
		t.Fatalf("marshal attributes: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal attributes: %v", err)
	}
	return out
}

func TestTransform_alwaysEmitsOneTime(t *testing.T) {
	recurring := attrsJSON(t, Model{recurrenceType: "time", intervalMonths: 12})
	v, ok := recurring["oneTime"]
	if !ok {
		t.Fatal("oneTime must be emitted even when false: an omitted key is " +
			"indistinguishable from a server predating the field")
	}
	if v != false {
		t.Fatalf("oneTime = %v want false", v)
	}

	oneTime := attrsJSON(t, Model{recurrenceType: "mileage", oneTime: true, dueMileage: 60000})
	if oneTime["oneTime"] != true {
		t.Fatalf("oneTime = %v want true", oneTime["oneTime"])
	}
}

func TestTransform_emitsDueFieldsOnlyWhenSet(t *testing.T) {
	bare := attrsJSON(t, Model{recurrenceType: "time", intervalMonths: 12})
	if _, ok := bare["dueDate"]; ok {
		t.Error("dueDate must be omitted when unset")
	}
	if _, ok := bare["dueMileage"]; ok {
		t.Error("dueMileage must be omitted when unset")
	}

	due := time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC)
	anchored := attrsJSON(t, Model{
		recurrenceType: "hybrid", oneTime: true, dueDate: due, dueMileage: 60000,
	})
	if got := anchored["dueDate"]; got != due.Format(timeFormat) {
		t.Errorf("dueDate = %v want %s", got, due.Format(timeFormat))
	}
	if got := anchored["dueMileage"]; got != float64(60000) {
		t.Errorf("dueMileage = %v want 60000", got)
	}
}

// A one-time schedule's due point never moves, so its feed row — and therefore
// the DueCycleToken notification-service derives from it — must be identical
// across repeated recomputes. If this ever drifts, every recompute mints a new
// dedupe key and the user gets a fresh overdue notification every hour.
func TestTransformInternalDue_oneTimeTokenIsStableAcrossRecomputes(t *testing.T) {
	db := newCompletionDB(t)
	vehicleID := seedVehicle(t, db, 59000)

	due := time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC)
	s, err := NewBuilder().SetVehicleID(vehicleID).SetCategoryID("c1").
		SetRecurrenceType("hybrid").SetOneTime(true).
		SetDuePoint(due, 60000).SetCurrentMileage(59000).Build()
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	created, err := NewAdministrator(db).Insert(s)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	admin := NewAdministrator(db)
	feedRow := func(now time.Time, currentMileage int) InternalDueSchedule {
		t.Helper()
		if err := admin.Recompute(created.ID(), currentMileage, now); err != nil {
			t.Fatalf("recompute: %v", err)
		}
		m, err := NewProvider(db).GetByID(created.ID())
		if err != nil {
			t.Fatalf("reload: %v", err)
		}
		rows := TransformInternalDue([]DueEntry{{Schedule: m, FleetID: "f1", State: "overdue"}})
		if len(rows) != 1 {
			t.Fatalf("want 1 feed row, got %d", len(rows))
		}
		return rows[0]
	}

	first := feedRow(time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC), 60500)
	// A later sweep, a moved odometer, a deeper overdue — none of it may move
	// the due point.
	second := feedRow(time.Date(2027, 6, 1, 0, 0, 0, 0, time.UTC), 71000)

	if first.NextDueDate != second.NextDueDate || first.NextDueMileage != second.NextDueMileage {
		t.Fatalf("feed due point drifted: %+v -> %+v", first, second)
	}
	if first.NextDueDate != due.Format(timeFormat) || first.NextDueMileage != 60000 {
		t.Fatalf("feed row does not carry the stored anchor: %+v", first)
	}

	m, err := NewProvider(db).GetByID(created.ID())
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got, want := DueCycleToken(m), due.Format(timeFormat)+"|60000"; got != want {
		t.Fatalf("DueCycleToken = %q want %q", got, want)
	}
}
