package maintenanceschedule

import (
	"errors"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// OverdueEmitter enqueues a schedule.overdue event in the outbox on the supplied
// tx (design A8). Injected to avoid an import cycle. dueCycle is the due-window
// token (see DueCycleToken) carried on the event so consumers can build a
// per-user dedupe_key identical to the reminder safety-net's (design A6).
type OverdueEmitter func(tx *gorm.DB, fleetID, scheduleID, vehicleID, severity, dueCycle string) error

// systemActor is the actor recorded for system-generated transitions (the
// recompute job has no human actor).
const systemActor = "system"

// Processor contains maintenance schedule business logic, injected with
// Provider and Administrator.
type Processor struct {
	log    logrus.FieldLogger
	p      Provider
	a      Administrator
	db     *gorm.DB
	record ActivityRecorder
	emit   OverdueEmitter
}

func NewProcessor(log logrus.FieldLogger, p Provider, a Administrator) *Processor {
	return &Processor{log: log, p: p, a: a}
}

// WithOverdueHooks injects the db handle + recorder + emitter the recompute job
// uses to record/emit a schedule.overdue event on the transition edge (A8).
func (pr *Processor) WithOverdueHooks(db *gorm.DB, rec ActivityRecorder, emit OverdueEmitter) *Processor {
	pr.db = db
	pr.record = rec
	pr.emit = emit
	return pr
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

// DueEntry is one non-ok schedule across all fleets, with its live DueState and
// fleet id resolved. Used by the internal /internal/maintenance/due endpoint that
// feeds notification-service's daily reminder safety-net (design §11, A6).
type DueEntry struct {
	Schedule Model
	FleetID  string
	State    string // upcoming | overdue
}

// DueAcrossAllFleets returns every active schedule (across ALL fleets) whose
// live DueState is upcoming or overdue, computed on read from the vehicle's
// current mileage and now (design A5). Used by the internal reminder feed.
func (pr *Processor) DueAcrossAllFleets(now time.Time) ([]DueEntry, error) {
	rows, err := pr.p.ListActive()
	if err != nil {
		return nil, err
	}
	out := make([]DueEntry, 0, len(rows))
	for _, r := range rows {
		state := DueState(r.Schedule.AsSchedule(), now, r.CurrentMileage, DefaultThresholds)
		if state == "ok" {
			continue // only surface non-ok schedules
		}
		out = append(out, DueEntry{
			Schedule: r.Schedule.WithStatus(state, Severity(state)),
			FleetID:  r.FleetID,
			State:    state,
		})
	}
	return out, nil
}

// ListActiveByFleet returns the raw QueueRows for all active schedules in a
// fleet. Exposed so the dashboard overview can derive per-vehicle status counts
// without re-entering the schedule processor's DueState logic.
func (pr *Processor) ListActiveByFleet(fleetID string) ([]QueueRow, error) {
	return pr.p.ListActiveByFleet(fleetID)
}

// ScheduleDue is one active schedule's live due-state plus, when the state is
// non-ok, the per-axis breach detail behind it.
type ScheduleDue struct {
	ScheduleID string
	State      string // ok | upcoming | overdue
	Breaches   []AxisBreach
}

// ScheduleDueByVehicle returns the live DueState of every active schedule for a
// vehicle together with the breach detail behind a non-ok state, computed on
// read from the vehicle's current mileage and now. Used by the vehicle layer to
// derive a vehicle's status and the reason behind it.
//
// This is the same single ListActiveByVehicle read the narrower
// ScheduleStatesByVehicle issued: the breach detail was already in hand and was
// discarded at the return boundary.
//
// State and breach magnitude are both computed from AsSchedule(), never from the
// stored next_due_* columns. Those columns are refreshed by the hourly recompute
// job, so reading one beside a freshly computed state can contradict — a
// schedule completed twenty minutes ago reports "ok" from fresh math while the
// stored next_due_mileage still describes the previous cycle.
func (pr *Processor) ScheduleDueByVehicle(vehicleID string) ([]ScheduleDue, error) {
	rows, err := pr.p.ListActiveByVehicle(vehicleID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	out := make([]ScheduleDue, 0, len(rows))
	for _, r := range rows {
		s := r.Schedule.AsSchedule()
		out = append(out, ScheduleDue{
			ScheduleID: r.Schedule.ID(),
			State:      DueState(s, now, r.CurrentMileage, DefaultThresholds),
			Breaches:   DueBreaches(s, now, r.CurrentMileage, DefaultThresholds),
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
//
// When a schedule transitions INTO overdue (prior stored status was not overdue
// but the freshly computed state is), it appends a schedule.overdue activity
// event and enqueues a schedule.overdue outbox event in the SAME transaction as
// the status update (design §8.2, A8). The edge check (only on the
// ok/upcoming→overdue transition) ensures the event fires once, not every hour.
func (pr *Processor) RecomputeAll(now time.Time) error {
	rows, err := pr.p.ListActive()
	if err != nil {
		return err
	}
	for _, r := range rows {
		prior := r.Schedule.Status()
		newState := DueState(r.Schedule.AsSchedule(), now, r.CurrentMileage, DefaultThresholds)
		transitionedToOverdue := prior != "overdue" && newState == "overdue"

		if !transitionedToOverdue || pr.db == nil {
			// No transition (or no hooks wired): plain recompute.
			if err := pr.a.Recompute(r.Schedule.ID(), r.CurrentMileage, now); err != nil {
				return err
			}
			continue
		}

		// Transition edge: recompute + activity + emit atomically (A8).
		sched := r.Schedule
		fleetID := r.FleetID
		if err := pr.db.Transaction(func(tx *gorm.DB) error {
			if err := pr.a.RecomputeTx(tx, sched.ID(), r.CurrentMileage, now); err != nil {
				return err
			}
			if pr.record != nil {
				vid := sched.VehicleID()
				if err := pr.record(tx, systemActor, "schedule.overdue", fleetID, &vid, map[string]any{
					"schedule_id": sched.ID(),
					"vehicle_id":  sched.VehicleID(),
					"severity":    Severity(newState),
				}); err != nil {
					return err
				}
			}
			if pr.emit != nil {
				if err := pr.emit(tx, fleetID, sched.ID(), sched.VehicleID(), Severity(newState), DueCycleToken(sched)); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}
