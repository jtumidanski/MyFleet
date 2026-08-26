package maintenanceschedule

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/systemactor"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/vehicle"
)

// The recompute sweep's transition edge had no test at all, which is how it
// shipped writing a word into a uuid column.
//
// fleet.activity_events.actor_user_id is `uuid NOT NULL`. On Postgres the
// insert below is rejected with `invalid input syntax for type uuid: "system"`,
// and because the recorder runs inside the same transaction as RecomputeTx, the
// status update rolls back with it — so the schedule never reaches overdue,
// tries again an hour later, fails again, and RecomputeAll returns the error,
// abandoning every schedule after it in the list. Forever.
//
// This harness is SQLite, which has no uuid type and would store "system"
// happily, so the assertion is on the value rather than on the insert failing.
func TestRecomputeAll_overdueTransitionRecordsAUUIDActor(t *testing.T) {
	db := newCompletionDB(t)
	rec, sched := seedOverdueSchedule(t, db)

	pr := NewProcessor(logrus.New(), NewProvider(db), NewAdministrator(db)).
		WithOverdueHooks(db, rec.record, nil)

	if err := pr.RecomputeAll(time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("recompute: %v", err)
	}

	if rec.calls != 1 {
		t.Fatalf("activity recorder called %d times, want 1 on the overdue edge", rec.calls)
	}
	if rec.actor != systemactor.ID {
		t.Errorf("overdue activity actor = %q, want the system sentinel %q", rec.actor, systemactor.ID)
	}
	if _, err := uuid.Parse(rec.actor); err != nil {
		t.Errorf("overdue activity actor %q is not a uuid: %v — Postgres rejects the "+
			"insert and rolls the status update back with it, so the sweep never "+
			"completes and every later schedule is abandoned", rec.actor, err)
	}

	// The transition must actually land, not just be attempted.
	got, err := NewProvider(db).GetByID(sched)
	if err != nil {
		t.Fatalf("reload schedule: %v", err)
	}
	if got.Status() != "overdue" {
		t.Errorf("schedule status = %q, want overdue", got.Status())
	}
}

// A recorder failure must not be swallowed: the status update has to roll back
// with it, or the edge check ("prior was not overdue") would never fire again
// and the event would be lost for good.
func TestRecomputeAll_overdueTransitionIsAtomicWithTheActivityAppend(t *testing.T) {
	db := newCompletionDB(t)
	rec, sched := seedOverdueSchedule(t, db)
	rec.err = errTestRecorder

	pr := NewProcessor(logrus.New(), NewProvider(db), NewAdministrator(db)).
		WithOverdueHooks(db, rec.record, nil)

	if err := pr.RecomputeAll(time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)); err == nil {
		t.Fatal("a failed activity append must surface, not be swallowed")
	}
	got, err := NewProvider(db).GetByID(sched)
	if err != nil {
		t.Fatalf("reload schedule: %v", err)
	}
	if got.Status() == "overdue" {
		t.Error("status was committed while its activity event was not — the next " +
			"tick sees no transition and the event is lost")
	}
}

var errTestRecorder = &recorderError{}

type recorderError struct{}

func (*recorderError) Error() string { return "activity insert failed" }

// capturingRecorder stands in for activity.Record, keeping the actor it was
// handed.
type capturingRecorder struct {
	calls int
	actor string
	err   error
}

func (c *capturingRecorder) record(_ *gorm.DB, actorUserID, _, _ string, _ *string, _ map[string]any) error {
	c.calls++
	c.actor = actorUserID
	return c.err
}

// seedOverdueSchedule inserts a vehicle well past a mileage-based schedule's
// next due point, so the next recompute crosses the ok -> overdue edge.
func seedOverdueSchedule(t *testing.T, db *gorm.DB) (*capturingRecorder, string) {
	t.Helper()
	v, err := vehicle.NewBuilder().SetFleetID("f1").SetMake("Honda").SetModel("Civic").
		SetYear(2020).SetCurrentMileage(50000).Build()
	if err != nil {
		t.Fatalf("build vehicle: %v", err)
	}
	if _, err := vehicle.NewAdministrator(db).Insert(v); err != nil {
		t.Fatalf("insert vehicle: %v", err)
	}

	s, err := NewBuilder().SetVehicleID(v.ID()).SetCategoryID("c1").
		SetRecurrenceType("mileage").SetIntervalMiles(5000).
		SetLastCompletedMileage(35000).Build()
	if err != nil {
		t.Fatalf("build schedule: %v", err)
	}
	created, err := NewAdministrator(db).Insert(s)
	if err != nil {
		t.Fatalf("insert schedule: %v", err)
	}
	// Whatever the builder defaulted to, the stored status must not already be
	// overdue or there is no edge to cross.
	if err := db.Table("fleet.maintenance_schedules").Where("id = ?", created.ID()).
		Update("status", "ok").Error; err != nil {
		t.Fatalf("seed prior status: %v", err)
	}
	return &capturingRecorder{}, created.ID()
}
