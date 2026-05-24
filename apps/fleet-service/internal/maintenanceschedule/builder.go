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
// sane interval for that type are invariants enforced at construction.
type Builder struct{ m Model }

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

// Build validates invariants and returns the model or a validation error.
func (b *Builder) Build() (Model, error) {
	if b.m.vehicleID == "" || b.m.categoryID == "" {
		return Model{}, server.ErrValidation
	}
	if !validRecurrence[b.m.recurrenceType] {
		return Model{}, server.ErrValidation
	}
	timed := b.m.recurrenceType == "time" || b.m.recurrenceType == "hybrid"
	miled := b.m.recurrenceType == "mileage" || b.m.recurrenceType == "hybrid"
	if timed && b.m.intervalMonths <= 0 {
		return Model{}, server.ErrValidation
	}
	if miled && b.m.intervalMiles <= 0 {
		return Model{}, server.ErrValidation
	}
	// Compute initial next-due + status from the (possibly zero) completion point.
	nd, nm := NextDue(b.m.AsSchedule())
	b.m.nextDueDate = nd
	b.m.nextDueMileage = nm
	state := DueState(b.m.AsSchedule(), time.Now().UTC(), b.m.lastCompletedMileage, DefaultThresholds)
	b.m.status = state
	b.m.severity = Severity(state)
	return b.m, nil
}
