package maintenanceschedule

import (
	"fmt"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

const timeFormat = "2006-01-02T15:04:05Z07:00"

// DueCycleToken builds the canonical due-window token for a schedule from its
// next-due date + mileage: "<RFC3339 next_due_date>|<next_due_mileage>" (the
// date part is empty for pure-mileage schedules). This MUST stay byte-identical
// to the token notification-service derives from the /internal/maintenance/due
// feed's next_due_date + next_due_mileage, so the event path and the reminder
// safety-net build the same per-user dedupe_key (design A6).
func DueCycleToken(m Model) string {
	date := ""
	if !m.NextDueDate().IsZero() {
		date = m.NextDueDate().Format(timeFormat)
	}
	return fmt.Sprintf("%s|%d", date, m.NextDueMileage())
}

// Attributes is the JSON:API attributes payload for a maintenance schedule.
type Attributes struct {
	VehicleID      string `json:"vehicleId"`
	CategoryID     string `json:"categoryId"`
	RecurrenceType string `json:"recurrenceType"`
	IntervalMonths int    `json:"intervalMonths,omitempty"`
	IntervalMiles  int    `json:"intervalMiles,omitempty"`
	// oneTime carries no omitempty on purpose: with it, a false would be
	// indistinguishable from a server that predates the field, and the
	// frontend keys the whole one-time treatment off this value.
	OneTime              bool   `json:"oneTime"`
	DueDate              string `json:"dueDate,omitempty"`
	DueMileage           int    `json:"dueMileage,omitempty"`
	LastCompletedDate    string `json:"lastCompletedDate,omitempty"`
	LastCompletedMileage int    `json:"lastCompletedMileage,omitempty"`
	NextDueDate          string `json:"nextDueDate,omitempty"`
	NextDueMileage       int    `json:"nextDueMileage,omitempty"`
	Status               string `json:"status"`
	Severity             string `json:"severity"`
	Active               bool   `json:"active"`
}

// Transform converts a Model to a JSON:API Resource.
func Transform(m Model) server.Resource {
	a := Attributes{
		VehicleID:            m.VehicleID(),
		CategoryID:           m.CategoryID(),
		RecurrenceType:       m.RecurrenceType(),
		IntervalMonths:       m.IntervalMonths(),
		IntervalMiles:        m.IntervalMiles(),
		OneTime:              m.OneTime(),
		DueMileage:           m.DueMileage(),
		LastCompletedMileage: m.LastCompletedMileage(),
		NextDueMileage:       m.NextDueMileage(),
		Status:               m.Status(),
		Severity:             m.Severity(),
		Active:               m.Active(),
	}
	if !m.LastCompletedDate().IsZero() {
		a.LastCompletedDate = m.LastCompletedDate().Format(timeFormat)
	}
	if !m.NextDueDate().IsZero() {
		a.NextDueDate = m.NextDueDate().Format(timeFormat)
	}
	if !m.DueDate().IsZero() {
		a.DueDate = m.DueDate().Format(timeFormat)
	}
	return server.Resource{Type: "maintenanceSchedules", ID: m.ID(), Attributes: a}
}

// TransformSlice converts a slice of Models to JSON:API Resources.
func TransformSlice(ms []Model) []server.Resource {
	out := make([]server.Resource, 0, len(ms))
	for _, m := range ms {
		out = append(out, Transform(m))
	}
	return out
}

// InternalDueSchedule is one row of the internal /internal/maintenance/due
// endpoint, consumed by notification-service's reminder safety-net. Snake-case
// keys match the notification-service fleetclient decoder. next_due_date is
// RFC3339 (empty for pure-mileage schedules); next_due_mileage is 0 for
// pure-time schedules.
type InternalDueSchedule struct {
	ScheduleID     string `json:"schedule_id"`
	VehicleID      string `json:"vehicle_id"`
	FleetID        string `json:"fleet_id"`
	CategoryID     string `json:"category_id"`
	State          string `json:"state"`
	NextDueDate    string `json:"next_due_date,omitempty"`
	NextDueMileage int    `json:"next_due_mileage,omitempty"`
}

// TransformInternalDue projects live due-entries onto the internal reminder-feed
// response shape.
func TransformInternalDue(entries []DueEntry) []InternalDueSchedule {
	out := make([]InternalDueSchedule, 0, len(entries))
	for _, e := range entries {
		row := InternalDueSchedule{
			ScheduleID:     e.Schedule.ID(),
			VehicleID:      e.Schedule.VehicleID(),
			FleetID:        e.FleetID,
			CategoryID:     e.Schedule.CategoryID(),
			State:          e.State,
			NextDueMileage: e.Schedule.NextDueMileage(),
		}
		if !e.Schedule.NextDueDate().IsZero() {
			row.NextDueDate = e.Schedule.NextDueDate().Format(timeFormat)
		}
		out = append(out, row)
	}
	return out
}

// TransformQueue converts queue entries (schedule + live state/severity) to
// JSON:API Resources, overriding the stored status/severity with the live ones.
func TransformQueue(entries []QueueEntry) []server.Resource {
	out := make([]server.Resource, 0, len(entries))
	for _, e := range entries {
		out = append(out, Transform(e.Schedule.WithStatus(e.State, e.Severity)))
	}
	return out
}
