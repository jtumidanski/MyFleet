package maintenanceschedule

import "time"

// Model is the immutable maintenance schedule domain model (design §8.2, §10.1).
type Model struct {
	id                   string
	vehicleID            string
	categoryID           string
	recurrenceType       string // time | mileage | hybrid
	intervalMonths       int
	intervalMiles        int
	oneTime              bool
	dueDate              time.Time
	dueMileage           int
	lastCompletedDate    time.Time
	lastCompletedMileage int
	nextDueDate          time.Time
	nextDueMileage       int
	status               string // ok | upcoming | overdue
	severity             string // informational | recommended | urgent
	active               bool
	createdAt            time.Time
	updatedAt            time.Time
}

func (m Model) ID() string                   { return m.id }
func (m Model) VehicleID() string            { return m.vehicleID }
func (m Model) CategoryID() string           { return m.categoryID }
func (m Model) RecurrenceType() string       { return m.recurrenceType }
func (m Model) IntervalMonths() int          { return m.intervalMonths }
func (m Model) IntervalMiles() int           { return m.intervalMiles }
func (m Model) OneTime() bool                { return m.oneTime }
func (m Model) DueDate() time.Time           { return m.dueDate }
func (m Model) DueMileage() int              { return m.dueMileage }
func (m Model) LastCompletedDate() time.Time { return m.lastCompletedDate }
func (m Model) LastCompletedMileage() int    { return m.lastCompletedMileage }
func (m Model) NextDueDate() time.Time       { return m.nextDueDate }
func (m Model) NextDueMileage() int          { return m.nextDueMileage }
func (m Model) Status() string               { return m.status }
func (m Model) Severity() string             { return m.severity }
func (m Model) Active() bool                 { return m.active }
func (m Model) CreatedAt() time.Time         { return m.createdAt }
func (m Model) UpdatedAt() time.Time         { return m.updatedAt }

// AsSchedule projects the model onto the pure recurrence-engine input.
func (m Model) AsSchedule() Schedule {
	return Schedule{
		RecurrenceType:       m.recurrenceType,
		IntervalMonths:       m.intervalMonths,
		IntervalMiles:        m.intervalMiles,
		OneTime:              m.oneTime,
		DueDate:              m.dueDate,
		DueMileage:           m.dueMileage,
		LastCompletedDate:    m.lastCompletedDate,
		LastCompletedMileage: m.lastCompletedMileage,
	}
}

// WithRecurrence returns a copy with recurrence parameters changed.
func (m Model) WithRecurrence(recurrenceType string, months, miles int) Model {
	m.recurrenceType = recurrenceType
	m.intervalMonths = months
	m.intervalMiles = miles
	return m
}

// WithOneTime returns a copy with the one-time flag changed.
func (m Model) WithOneTime(v bool) Model { m.oneTime = v; return m }

// WithDuePoint returns a copy with the absolute due point set (or, with a zero
// date and 0 miles, cleared). Both axes move together because the completion
// flow clears both together (FR-ANCHOR-3).
func (m Model) WithDuePoint(date time.Time, miles int) Model {
	m.dueDate = date
	m.dueMileage = miles
	return m
}

// WithActive returns a copy with the active flag changed.
func (m Model) WithActive(active bool) Model { m.active = active; return m }

// WithLastCompleted returns a copy advanced to a completion point.
func (m Model) WithLastCompleted(date time.Time, miles int) Model {
	m.lastCompletedDate = date
	m.lastCompletedMileage = miles
	return m
}

// WithNextDue returns a copy with computed next-due values set.
func (m Model) WithNextDue(date time.Time, miles int) Model {
	m.nextDueDate = date
	m.nextDueMileage = miles
	return m
}

// WithStatus returns a copy with status + severity set.
func (m Model) WithStatus(status, severity string) Model {
	m.status = status
	m.severity = severity
	return m
}
