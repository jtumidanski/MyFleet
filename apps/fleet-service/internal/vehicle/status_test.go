package vehicle

import (
	"errors"
	"testing"
	"time"
)

var statusNow = time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)

type fakeSchedules struct {
	dues []ScheduleDue
	err  error
}

func (f fakeSchedules) ScheduleDueByVehicle(string) ([]ScheduleDue, error) { return f.dues, f.err }

type fakeActivity struct {
	at  time.Time
	err error
}

func (f fakeActivity) LastActivityByVehicle(string) (time.Time, error) { return f.at, f.err }

// testVehicle builds a model directly; the builder has no created-at setter and
// these tests are in-package.
func testVehicle(createdAt time.Time) Model {
	return Model{id: "v1", fleetID: "f1", make: "Honda", model: "Civic", year: 2019, createdAt: createdAt}
}

func TestDerive_scheduleGatherErrorYieldsZeroDerived(t *testing.T) {
	d := StatusDeps{
		Schedules: fakeSchedules{err: errors.New("boom")},
		Activity:  fakeActivity{at: statusNow},
	}
	got := d.Derive(testVehicle(statusNow), statusNow)
	if got.Status != "" || !got.LastActivityAt.IsZero() || got.NextDue != nil {
		t.Fatalf("a gather error must omit every derived value, got %+v", got)
	}
}

func TestDerive_activityGatherErrorYieldsZeroDerived(t *testing.T) {
	d := StatusDeps{
		Schedules: fakeSchedules{dues: []ScheduleDue{{ScheduleID: "s1", State: "overdue", Breaches: []Breach{{Axis: "mileage", Miles: 100, Urgency: 1.2}}}}},
		Activity:  fakeActivity{err: errors.New("boom")},
	}
	got := d.Derive(testVehicle(statusNow), statusNow)
	if got.Status != "" || !got.LastActivityAt.IsZero() || got.NextDue != nil {
		t.Fatalf("a gather error must omit every derived value, got %+v", got)
	}
}

func TestDerive_reportsStatusLastActivityAndGoverningBreach(t *testing.T) {
	last := statusNow.AddDate(0, 0, -6)
	d := StatusDeps{
		Schedules: fakeSchedules{dues: []ScheduleDue{
			{ScheduleID: "s1", State: "ok"},
			{ScheduleID: "s2", State: "overdue", Breaches: []Breach{{Axis: "mileage", Miles: 1120, Urgency: 3.24}}},
		}},
		Activity: fakeActivity{at: last},
	}
	got := d.Derive(testVehicle(statusNow.AddDate(-2, 0, 0)), statusNow)

	if got.Status != "Overdue" {
		t.Fatalf("status = %q want Overdue", got.Status)
	}
	if !got.LastActivityAt.Equal(last) {
		t.Fatalf("lastActivityAt = %v want %v", got.LastActivityAt, last)
	}
	if got.NextDue == nil || got.NextDue.Axis != "mileage" || got.NextDue.Miles == nil || *got.NextDue.Miles != 1120 {
		t.Fatalf("nextDue = %+v", got.NextDue)
	}
}

func TestDerive_fallsBackToCreatedAtWhenNoActivityRecorded(t *testing.T) {
	// A brand-new vehicle with no activity must be Healthy, not Inactive, and
	// the exposed timestamp is the post-fallback value — so the card can never
	// show an em-dash beside a status computed from a real timestamp.
	created := statusNow.AddDate(0, 0, -3)
	d := StatusDeps{
		Schedules: fakeSchedules{dues: nil},
		Activity:  fakeActivity{at: time.Time{}},
	}
	got := d.Derive(testVehicle(created), statusNow)

	if got.Status != "Healthy" {
		t.Fatalf("status = %q want Healthy", got.Status)
	}
	if !got.LastActivityAt.Equal(created) {
		t.Fatalf("lastActivityAt = %v want the created-at fallback %v", got.LastActivityAt, created)
	}
}

func TestDerive_zeroCreatedAtLeavesLastActivityZero(t *testing.T) {
	// The task-006 case: a wiped created_at must surface as an omitted attribute
	// (an em-dash on the card), never as an absurd year-1 date.
	d := StatusDeps{
		Schedules: fakeSchedules{dues: nil},
		Activity:  fakeActivity{at: time.Time{}},
	}
	got := d.Derive(testVehicle(time.Time{}), statusNow)
	if !got.LastActivityAt.IsZero() {
		t.Fatalf("lastActivityAt = %v want zero", got.LastActivityAt)
	}
}

func TestDerive_inactiveNeverCarriesDueDetail(t *testing.T) {
	// Design D7: status.Derive only reaches Inactive after both the overdue and
	// upcoming scans fall through, so an Inactive vehicle has no non-ok schedule
	// by construction. Asserted rather than defended against, so a future change
	// to Derive that breaks the invariant fails loudly instead of producing an
	// Inactive card with a tinted banner.
	d := StatusDeps{
		Schedules: fakeSchedules{dues: []ScheduleDue{{ScheduleID: "s1", State: "ok"}}},
		Activity:  fakeActivity{at: statusNow.AddDate(0, 0, -400)},
	}
	got := d.Derive(testVehicle(statusNow.AddDate(-3, 0, 0)), statusNow)
	if got.Status != "Inactive" {
		t.Fatalf("status = %q want Inactive", got.Status)
	}
	if got.NextDue != nil {
		t.Fatalf("Inactive must carry no due detail, got %+v", got.NextDue)
	}
}

func TestDerive_statusValuesAreUnchanged(t *testing.T) {
	// NFR-16: widening the gatherer must not move a single status value. Each row
	// is the status today's DeriveStatus produced for the same inputs.
	cases := []struct {
		name       string
		states     []string
		lastOffset int // days before statusNow
		want       string
	}{
		{"overdue beats everything", []string{"ok", "upcoming", "overdue"}, 1, "Overdue"},
		{"upcoming beats healthy", []string{"ok", "upcoming"}, 1, "Upcoming Maintenance"},
		{"no schedules, recent activity", nil, 10, "Healthy"},
		{"no schedules, stale activity", nil, 400, "Inactive"},
		{"all ok, recent activity", []string{"ok", "ok"}, 10, "Healthy"},
		{"all ok, stale activity", []string{"ok"}, 400, "Inactive"},
		{"upcoming outranks inactivity", []string{"upcoming"}, 400, "Upcoming Maintenance"},
		{"overdue outranks inactivity", []string{"overdue"}, 400, "Overdue"},
		{"exactly at the 365-day boundary is still Healthy", nil, 365, "Healthy"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dues := make([]ScheduleDue, 0, len(c.states))
			for i, s := range c.states {
				dues = append(dues, ScheduleDue{ScheduleID: string(rune('a' + i)), State: s})
			}
			d := StatusDeps{
				Schedules: fakeSchedules{dues: dues},
				Activity:  fakeActivity{at: statusNow.AddDate(0, 0, -c.lastOffset)},
			}
			if got := d.Derive(testVehicle(statusNow), statusNow).Status; got != c.want {
				t.Fatalf("status = %q want %q", got, c.want)
			}
		})
	}
}
