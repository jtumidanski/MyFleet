package maintenanceschedule

import "time"

// Schedule is the pure input to recurrence math (mirrors the entity's fields).
type Schedule struct {
	RecurrenceType       string // time | mileage | hybrid
	IntervalMonths       int
	IntervalMiles        int
	LastCompletedDate    time.Time
	LastCompletedMileage int
}

type Thresholds struct {
	DueSoonDays  int
	DueSoonMiles int
}

// DefaultThresholds: 30 days / 500 mi (design §10.1, FR-STATUS-2). Configurable.
var DefaultThresholds = Thresholds{DueSoonDays: 30, DueSoonMiles: 500}

func NextDue(s Schedule) (nextDate time.Time, nextMiles int) {
	if s.RecurrenceType == "time" || s.RecurrenceType == "hybrid" {
		nextDate = s.LastCompletedDate.AddDate(0, s.IntervalMonths, 0)
	}
	if s.RecurrenceType == "mileage" || s.RecurrenceType == "hybrid" {
		nextMiles = s.LastCompletedMileage + s.IntervalMiles
	}
	return
}

// DueState classifies a schedule given today + current mileage (design §10.1).
func DueState(s Schedule, today time.Time, currentMileage int, th Thresholds) string {
	nd, nm := NextDue(s)
	timed := s.RecurrenceType == "time" || s.RecurrenceType == "hybrid"
	miled := s.RecurrenceType == "mileage" || s.RecurrenceType == "hybrid"

	if timed && today.After(nd) {
		return "overdue"
	}
	if miled && currentMileage > nm {
		return "overdue"
	}
	if timed && !today.Before(nd.AddDate(0, 0, -th.DueSoonDays)) {
		return "upcoming"
	}
	if miled && currentMileage >= nm-th.DueSoonMiles {
		return "upcoming"
	}
	return "ok"
}

func Severity(state string) string {
	switch state {
	case "overdue":
		return "urgent"
	case "upcoming":
		return "recommended"
	default:
		return "informational"
	}
}
