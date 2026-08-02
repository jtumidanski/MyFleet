package maintenanceschedule

import (
	"testing"

	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// countingProvider records how many times each read is issued, so the
// "no additional queries" requirement is measured rather than eyeballed.
type countingProvider struct {
	rows            []QueueRow
	byVehicleCalls  int
	byFleetCalls    int
	listActiveCalls int
}

func (p *countingProvider) GetByID(string) (Model, error) { return Model{}, ErrNotFound }

func (p *countingProvider) ListByVehicle(string, server.Page) ([]Model, int, error) {
	return nil, 0, nil
}

func (p *countingProvider) ListActiveByFleet(string) ([]QueueRow, error) {
	p.byFleetCalls++
	return p.rows, nil
}

func (p *countingProvider) ListActiveByVehicle(string) ([]QueueRow, error) {
	p.byVehicleCalls++
	return p.rows, nil
}

func (p *countingProvider) ListActive() ([]QueueRow, error) {
	p.listActiveCalls++
	return p.rows, nil
}

// mileageSchedule is due at lastMiles+5000 and carries no time axis, so its
// due-state does not depend on the wall clock.
func mileageSchedule(id string, lastMiles int) Model {
	return Model{
		id:                   id,
		vehicleID:            "v1",
		recurrenceType:       "mileage",
		intervalMiles:        5000,
		lastCompletedMileage: lastMiles,
		active:               true,
	}
}

func TestScheduleDueByVehicle_returnsStateAndBreachesPerSchedule(t *testing.T) {
	// All three share a current mileage of 36120; each schedule's own
	// last-completed value is what puts it in a different state.
	p := &countingProvider{rows: []QueueRow{
		{Schedule: mileageSchedule("s-overdue", 30000), CurrentMileage: 36120, FleetID: "f1"},
		{Schedule: mileageSchedule("s-upcoming", 31500), CurrentMileage: 36120, FleetID: "f1"},
		{Schedule: mileageSchedule("s-ok", 36000), CurrentMileage: 36120, FleetID: "f1"},
	}}
	pr := NewProcessor(logrus.New(), p, nil)

	dues, err := pr.ScheduleDueByVehicle("v1")
	if err != nil {
		t.Fatalf("ScheduleDueByVehicle: %v", err)
	}
	if len(dues) != 3 {
		t.Fatalf("want 3 dues, got %d", len(dues))
	}

	// s-overdue: due at 35000, currently 36120 -> 1120mi over.
	if dues[0].ScheduleID != "s-overdue" || dues[0].State != "overdue" {
		t.Fatalf("dues[0] = %+v", dues[0])
	}
	if len(dues[0].Breaches) != 1 || dues[0].Breaches[0].Miles != 1120 {
		t.Fatalf("dues[0].Breaches = %+v", dues[0].Breaches)
	}

	// s-upcoming: due at 36500, currently 36120 -> 380mi remaining, inside the
	// 500-mile window.
	if dues[1].ScheduleID != "s-upcoming" || dues[1].State != "upcoming" {
		t.Fatalf("dues[1] = %+v", dues[1])
	}
	if len(dues[1].Breaches) != 1 || dues[1].Breaches[0].Miles != 380 {
		t.Fatalf("dues[1].Breaches = %+v", dues[1].Breaches)
	}

	// s-ok: due at 41000, currently 36120 -> 4880 remaining, well outside it.
	if dues[2].State != "ok" || len(dues[2].Breaches) != 0 {
		t.Fatalf("an ok schedule must carry no breaches, got %+v", dues[2])
	}
}

func TestScheduleDueByVehicle_upcomingCarriesRemainingDistance(t *testing.T) {
	p := &countingProvider{rows: []QueueRow{
		// due at 35000, currently 34690 -> 310 remaining, inside the 500 window.
		{Schedule: mileageSchedule("s1", 30000), CurrentMileage: 34690, FleetID: "f1"},
	}}
	pr := NewProcessor(logrus.New(), p, nil)

	dues, err := pr.ScheduleDueByVehicle("v1")
	if err != nil {
		t.Fatalf("ScheduleDueByVehicle: %v", err)
	}
	if dues[0].State != "upcoming" {
		t.Fatalf("state = %q want upcoming", dues[0].State)
	}
	if len(dues[0].Breaches) != 1 || dues[0].Breaches[0].Axis != "mileage" || dues[0].Breaches[0].Miles != 310 {
		t.Fatalf("breaches = %+v", dues[0].Breaches)
	}
}

func TestScheduleDueByVehicle_issuesExactlyOneQuery(t *testing.T) {
	// NFR-1 / the acceptance criterion that says "verified by test, not by
	// inspection": widening the return must not widen the read.
	p := &countingProvider{rows: []QueueRow{
		{Schedule: mileageSchedule("s1", 30000), CurrentMileage: 36120, FleetID: "f1"},
		{Schedule: mileageSchedule("s2", 31000), CurrentMileage: 36120, FleetID: "f1"},
	}}
	pr := NewProcessor(logrus.New(), p, nil)

	if _, err := pr.ScheduleDueByVehicle("v1"); err != nil {
		t.Fatalf("ScheduleDueByVehicle: %v", err)
	}
	if p.byVehicleCalls != 1 {
		t.Fatalf("ListActiveByVehicle called %d times, want exactly 1", p.byVehicleCalls)
	}
	if p.byFleetCalls != 0 || p.listActiveCalls != 0 {
		t.Fatalf("no other read may be issued: byFleet=%d listActive=%d", p.byFleetCalls, p.listActiveCalls)
	}
}
