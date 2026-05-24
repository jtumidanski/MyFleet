package maintenanceschedule

import (
	"errors"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// Processor contains maintenance schedule business logic, injected with
// Provider and Administrator.
type Processor struct {
	log logrus.FieldLogger
	p   Provider
	a   Administrator
}

func NewProcessor(log logrus.FieldLogger, p Provider, a Administrator) *Processor {
	return &Processor{log: log, p: p, a: a}
}

// GetByID fetches a maintenance schedule.
func (pr *Processor) GetByID(id string) (Model, error) {
	m, err := pr.p.GetByID(id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Model{}, server.ErrNotFound
		}
		return Model{}, err
	}
	return m, nil
}

// ListByVehicle returns a page of schedules for a vehicle.
func (pr *Processor) ListByVehicle(vehicleID string, page server.Page) ([]Model, int, error) {
	return pr.p.ListByVehicle(vehicleID, page)
}

// Create inserts a new schedule.
func (pr *Processor) Create(m Model) (Model, error) {
	return pr.a.Insert(m)
}

// Update applies a partial update to an existing schedule, recomputing next-due.
func (pr *Processor) Update(id string, apply func(Model) Model) (Model, error) {
	m, err := pr.GetByID(id)
	if err != nil {
		return Model{}, err
	}
	updated := apply(m)
	// Recompute next-due + status from the (possibly changed) recurrence params.
	nd, nm := NextDue(updated.AsSchedule())
	updated = updated.WithNextDue(nd, nm)
	state := DueState(updated.AsSchedule(), time.Now().UTC(), updated.LastCompletedMileage(), DefaultThresholds)
	updated = updated.WithStatus(state, Severity(state))
	return pr.a.Update(updated)
}

// Delete removes a schedule.
func (pr *Processor) Delete(id string) error {
	err := pr.a.Delete(id)
	if errors.Is(err, ErrNotFound) {
		return server.ErrNotFound
	}
	return err
}

// QueueEntry is one row of an upcoming/overdue queue: the schedule plus its
// freshly-computed due-state and severity (design A5 — computed on read).
type QueueEntry struct {
	Schedule Model
	State    string
	Severity string
}

// Queue returns active schedules in the fleet whose live DueState matches the
// requested state ("upcoming" or "overdue"). DueState is computed on read from
// the vehicle's current mileage and time.Now() (design A5).
func (pr *Processor) Queue(fleetID, wantState string, now time.Time) ([]QueueEntry, error) {
	rows, err := pr.p.ListActiveByFleet(fleetID)
	if err != nil {
		return nil, err
	}
	out := make([]QueueEntry, 0, len(rows))
	for _, r := range rows {
		state := DueState(r.Schedule.AsSchedule(), now, r.CurrentMileage, DefaultThresholds)
		if state != wantState {
			continue
		}
		out = append(out, QueueEntry{
			Schedule: r.Schedule.WithStatus(state, Severity(state)),
			State:    state,
			Severity: Severity(state),
		})
	}
	return out, nil
}

// ScheduleStatesByVehicle returns the live DueState ("ok"|"upcoming"|"overdue")
// of every active schedule for a vehicle, computed on read from the vehicle's
// current mileage and now (design A5 / §10.2). Used by the vehicle layer to
// derive a vehicle's status on read.
func (pr *Processor) ScheduleStatesByVehicle(vehicleID string) ([]string, error) {
	rows, err := pr.p.ListActiveByVehicle(vehicleID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, DueState(r.Schedule.AsSchedule(), now, r.CurrentMileage, DefaultThresholds))
	}
	return out, nil
}

// RecomputeAll re-derives stored status/severity/next_due_* for every active
// schedule using each vehicle's current mileage and "now" (FR-MAINT-6). Invoked
// by the hourly recompute job under an advisory lock.
func (pr *Processor) RecomputeAll(now time.Time) error {
	rows, err := pr.p.ListActive()
	if err != nil {
		return err
	}
	for _, r := range rows {
		if err := pr.a.Recompute(r.Schedule.ID(), r.CurrentMileage, now); err != nil {
			return err
		}
	}
	return nil
}
