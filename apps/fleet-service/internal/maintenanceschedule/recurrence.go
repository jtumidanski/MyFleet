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

// AxisBreach describes one axis of one schedule that is currently breaching,
// with its magnitude and a threshold-normalized urgency.
//
// Urgency is a single monotone scale spanning both due-states, so a max over a
// mixed set always picks the more pressing item:
//
//	overdue:  1 + breach/threshold     (always > 1)
//	upcoming: 1 - remaining/threshold  (always within [0, 1])
//
// "remaining" can never exceed the threshold, because an axis only qualifies as
// upcoming inside the due-soon window, so the two ranges never overlap. Ranking
// upcoming schedules by raw normalized magnitude would surface the LEAST
// imminent one; the inversion is what makes "max urgency" correct in both
// directions.
type AxisBreach struct {
	Axis    string // "time" | "mileage"
	Days    int    // set when Axis == "time"
	Miles   int    // set when Axis == "mileage"
	Urgency float64
}

// DueBreaches returns one entry per axis that is itself breaching. It returns
// nil for a schedule whose DueState is "ok": an ok schedule breaches on no axis,
// so the correspondence holds by construction rather than by an explicit check.
//
// The per-axis conditions are re-tested here rather than inferred from the
// schedule's state. A hybrid schedule is "overdue" when EITHER axis is overdue,
// so the time axis of a mileage-overdue hybrid may be perfectly fine.
//
// The upcoming branches carry an upper bound (today not after nd; mileage not
// above nm) that the matching DueState branches omit. DueState only reaches its
// upcoming branches after the overdue branches have already returned, so the
// bound is implicit there. Here each axis is judged independently, so the bound
// has to be explicit — otherwise a hybrid overdue on mileage would also report
// its time axis as "upcoming in -40 days".
func DueBreaches(s Schedule, today time.Time, currentMileage int, th Thresholds) []AxisBreach {
	nd, nm := NextDue(s)
	timed := s.RecurrenceType == "time" || s.RecurrenceType == "hybrid"
	miled := s.RecurrenceType == "mileage" || s.RecurrenceType == "hybrid"

	var out []AxisBreach

	if timed && today.After(nd) {
		// Anything past the due instant reads as at least a day late in the
		// card copy; "overdue by 0 days" is nonsense.
		days := wholeDays(today.Sub(nd))
		if days < 1 {
			days = 1
		}
		out = append(out, AxisBreach{Axis: "time", Days: days, Urgency: overdueUrgency(days, th.DueSoonDays)})
	} else if timed && !today.Before(nd.AddDate(0, 0, -th.DueSoonDays)) && !today.After(nd) {
		// 0 is legal here and means "due today".
		days := wholeDays(nd.Sub(today))
		out = append(out, AxisBreach{Axis: "time", Days: days, Urgency: upcomingUrgency(days, th.DueSoonDays)})
	}

	if miled && currentMileage > nm {
		miles := currentMileage - nm
		out = append(out, AxisBreach{Axis: "mileage", Miles: miles, Urgency: overdueUrgency(miles, th.DueSoonMiles)})
	} else if miled && currentMileage >= nm-th.DueSoonMiles && currentMileage <= nm {
		miles := nm - currentMileage
		out = append(out, AxisBreach{Axis: "mileage", Miles: miles, Urgency: upcomingUrgency(miles, th.DueSoonMiles)})
	}

	return out
}

// wholeDays truncates a duration to whole days, matching the day granularity the
// card copy speaks in.
func wholeDays(d time.Duration) int { return int(d.Hours() / 24) }

// overdueUrgency and upcomingUrgency are computed from the same integer
// magnitude that is reported, not from the underlying duration, so a test can
// predict the score exactly from the value the user will see.
func overdueUrgency(breach, threshold int) float64 {
	if threshold <= 0 {
		return 1
	}
	return 1 + float64(breach)/float64(threshold)
}

func upcomingUrgency(remaining, threshold int) float64 {
	if threshold <= 0 {
		return 0
	}
	return float64(threshold-remaining) / float64(threshold)
}
