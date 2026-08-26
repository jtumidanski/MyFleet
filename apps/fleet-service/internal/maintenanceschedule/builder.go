package maintenanceschedule

import (
	"time"

	"github.com/google/uuid"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// validRecurrence enumerates the allowed recurrence types (design §10.1).
var validRecurrence = map[string]bool{"time": true, "mileage": true, "hybrid": true}

// Builder constructs a valid maintenance schedule Model. Build() returns
// (Model, error) because vehicleID, categoryID, a valid recurrence type, and a
// sane interval and due point for that type are invariants enforced at
// construction.
//
// currentMileage is the VEHICLE's odometer, not part of the model. It exists so
// the initial stored status is derived against the same value the hourly
// recompute will use; without it a new mileage schedule is stored "ok"
// regardless of how close the vehicle already is to the due point.
type Builder struct {
	m              Model
	currentMileage int
}

func NewBuilder() *Builder {
	return &Builder{m: Model{id: uuid.NewString(), active: true}}
}

func (b *Builder) SetVehicleID(vehicleID string) *Builder    { b.m.vehicleID = vehicleID; return b }
func (b *Builder) SetCategoryID(categoryID string) *Builder  { b.m.categoryID = categoryID; return b }
func (b *Builder) SetRecurrenceType(t string) *Builder       { b.m.recurrenceType = t; return b }
func (b *Builder) SetIntervalMonths(months int) *Builder     { b.m.intervalMonths = months; return b }
func (b *Builder) SetIntervalMiles(miles int) *Builder       { b.m.intervalMiles = miles; return b }
func (b *Builder) SetActive(active bool) *Builder            { b.m.active = active; return b }
func (b *Builder) SetLastCompletedDate(t time.Time) *Builder { b.m.lastCompletedDate = t; return b }
func (b *Builder) SetLastCompletedMileage(m int) *Builder    { b.m.lastCompletedMileage = m; return b }
func (b *Builder) SetOneTime(v bool) *Builder                { b.m.oneTime = v; return b }

// SetDuePoint sets the absolute due point: the permanent due point of a
// one-time schedule, or the first-due anchor of a recurring one.
func (b *Builder) SetDuePoint(date time.Time, miles int) *Builder {
	b.m.dueDate = date
	b.m.dueMileage = miles
	return b
}

// SetCurrentMileage supplies the owning vehicle's odometer for the initial
// status derivation. It is not persisted on the model.
func (b *Builder) SetCurrentMileage(miles int) *Builder { b.currentMileage = miles; return b }

// validate enforces every maintenance-schedule invariant on a fully-formed
// model. Both construction (Builder.Build) and mutation (Processor.Update) run
// it, so an invariant cannot be satisfied at creation and violated by a PATCH.
//
// It returns server.ErrValidation for every failure and does not say which rule
// failed: that matches the rest of the service and server.WriteError's mapping,
// and the frontend Zod schema is the layer that produces per-field messages.
func validate(m Model) error {
	if m.vehicleID == "" || m.categoryID == "" {
		return server.ErrValidation
	}
	if !validRecurrence[m.recurrenceType] {
		return server.ErrValidation
	}
	timed := m.recurrenceType == "time" || m.recurrenceType == "hybrid"
	miled := m.recurrenceType == "mileage" || m.recurrenceType == "hybrid"

	// Intervals: required for a recurring schedule on every covered axis,
	// forbidden outright on a one-time one (FR-OT-3).
	if m.oneTime {
		if m.intervalMonths != 0 || m.intervalMiles != 0 {
			return server.ErrValidation
		}
	} else {
		if timed && m.intervalMonths <= 0 {
			return server.ErrValidation
		}
		if miled && m.intervalMiles <= 0 {
			return server.ErrValidation
		}
	}

	// The due point is required exactly where it is the ONLY thing that can
	// produce a due date: on every live one-time schedule (FR-OT-2), and on a
	// recurring schedule that has never been completed (FR-ANCHOR-1). It is not
	// required once there is a completion point to derive from — FR-ANCHOR-3
	// clears it on purpose — nor on a completed-and-deactivated one-time row,
	// whose anchor the completion flow deliberately cleared.
	neverCompleted := m.lastCompletedDate.IsZero() && m.lastCompletedMileage == 0
	if (m.oneTime && m.active) || (!m.oneTime && neverCompleted) {
		if timed && m.dueDate.IsZero() {
			return server.ErrValidation
		}
		if miled && m.dueMileage <= 0 {
			return server.ErrValidation
		}
	}
	return nil
}

// Build validates invariants and returns the model or a validation error.
func (b *Builder) Build() (Model, error) {
	if err := validate(b.m); err != nil {
		return Model{}, err
	}
	// Compute initial next-due + status from the due point / completion point.
	nd, nm := NextDue(b.m.AsSchedule())
	b.m.nextDueDate = nd
	b.m.nextDueMileage = nm
	state := DueState(b.m.AsSchedule(), time.Now().UTC(), b.currentMileage, DefaultThresholds)
	b.m.status = state
	b.m.severity = Severity(state)
	return b.m, nil
}
