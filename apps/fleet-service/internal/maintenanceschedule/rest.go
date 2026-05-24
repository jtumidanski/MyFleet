package maintenanceschedule

import "github.com/jtumidanski/myfleet/packages/shared-go/server"

const timeFormat = "2006-01-02T15:04:05Z07:00"

// Attributes is the JSON:API attributes payload for a maintenance schedule.
type Attributes struct {
	VehicleID            string `json:"vehicleId"`
	CategoryID           string `json:"categoryId"`
	RecurrenceType       string `json:"recurrenceType"`
	IntervalMonths       int    `json:"intervalMonths,omitempty"`
	IntervalMiles        int    `json:"intervalMiles,omitempty"`
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

// TransformQueue converts queue entries (schedule + live state/severity) to
// JSON:API Resources, overriding the stored status/severity with the live ones.
func TransformQueue(entries []QueueEntry) []server.Resource {
	out := make([]server.Resource, 0, len(entries))
	for _, e := range entries {
		out = append(out, Transform(e.Schedule.WithStatus(e.State, e.Severity)))
	}
	return out
}
