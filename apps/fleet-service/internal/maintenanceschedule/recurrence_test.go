package maintenanceschedule

import (
	"math"
	"testing"
	"time"
)

var base = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// urgency is computed at runtime from float64 operands, while a `want` written
// as an untyped constant expression is folded at arbitrary precision and
// rounded once. The two can differ by an ULP, so compare within a tolerance
// rather than exactly.
func assertUrgency(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("urgency = %v want %v", got, want)
	}
}

func TestNextDue(t *testing.T) {
	cases := []struct {
		name          string
		recur         string
		months, miles int
		lastDate      time.Time
		lastMiles     int
		wantDate      time.Time
		wantMiles     int
	}{
		{"time", "time", 12, 0, base, 0, base.AddDate(0, 12, 0), 0},
		{"mileage", "mileage", 0, 5000, base, 30000, time.Time{}, 35000},
		{"hybrid", "hybrid", 12, 5000, base, 30000, base.AddDate(0, 12, 0), 35000},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := Schedule{RecurrenceType: c.recur, IntervalMonths: c.months, IntervalMiles: c.miles, LastCompletedDate: c.lastDate, LastCompletedMileage: c.lastMiles}
			nd, nm := NextDue(s)
			if c.recur != "mileage" && !nd.Equal(c.wantDate) {
				t.Fatalf("next_due_date = %v want %v", nd, c.wantDate)
			}
			if c.recur != "time" && nm != c.wantMiles {
				t.Fatalf("next_due_mileage = %d want %d", nm, c.wantMiles)
			}
		})
	}
}

func TestDueState(t *testing.T) {
	s := Schedule{RecurrenceType: "hybrid", IntervalMonths: 12, IntervalMiles: 5000, LastCompletedDate: base, LastCompletedMileage: 30000}
	// today far before due, mileage well under → ok
	if got := DueState(s, base.AddDate(0, 1, 0), 31000, DefaultThresholds); got != "ok" {
		t.Fatalf("want ok, got %s", got)
	}
	// mileage within 500 of 35000 → upcoming
	if got := DueState(s, base.AddDate(0, 1, 0), 34600, DefaultThresholds); got != "upcoming" {
		t.Fatalf("want upcoming (within 500mi), got %s", got)
	}
	// past 35000 → overdue
	if got := DueState(s, base.AddDate(0, 1, 0), 35001, DefaultThresholds); got != "overdue" {
		t.Fatalf("want overdue (mileage exceeded), got %s", got)
	}
	// time past next_due_date → overdue
	if got := DueState(s, base.AddDate(0, 13, 0), 31000, DefaultThresholds); got != "overdue" {
		t.Fatalf("want overdue (time exceeded), got %s", got)
	}
}

func TestSeverity(t *testing.T) {
	if Severity("ok") != "informational" || Severity("upcoming") != "recommended" || Severity("overdue") != "urgent" {
		t.Fatal("severity bands per design §10.1")
	}
}

// mileageOnly is due at 35000: last completed at 30000 with a 5000-mile interval.
func mileageOnly() Schedule {
	return Schedule{RecurrenceType: "mileage", IntervalMiles: 5000, LastCompletedMileage: 30000}
}

// timeOnly is due 12 months after base.
func timeOnly() Schedule {
	return Schedule{RecurrenceType: "time", IntervalMonths: 12, LastCompletedDate: base}
}

// hybridBoth is due at 35000 miles AND 12 months after base.
func hybridBoth() Schedule {
	return Schedule{
		RecurrenceType:       "hybrid",
		IntervalMonths:       12,
		IntervalMiles:        5000,
		LastCompletedDate:    base,
		LastCompletedMileage: 30000,
	}
}

func TestDueBreaches_okScheduleHasNoBreaches(t *testing.T) {
	// 31000 is well under 35000 and base+1mo is well before base+12mo.
	if got := DueBreaches(hybridBoth(), base.AddDate(0, 1, 0), 31000, DefaultThresholds); got != nil {
		t.Fatalf("an ok schedule must breach on no axis, got %+v", got)
	}
}

func TestDueBreaches_mileageOverdue(t *testing.T) {
	got := DueBreaches(mileageOnly(), base, 36120, DefaultThresholds)
	if len(got) != 1 {
		t.Fatalf("want 1 breach, got %+v", got)
	}
	b := got[0]
	if b.Axis != "mileage" || b.Miles != 1120 || b.Days != 0 {
		t.Fatalf("want mileage/1120mi/0d, got %+v", b)
	}
	// 1 + 1120/500
	assertUrgency(t, b.Urgency, 1+1120.0/500.0)
}

func TestDueBreaches_mileageUpcoming(t *testing.T) {
	got := DueBreaches(mileageOnly(), base, 34690, DefaultThresholds)
	if len(got) != 1 {
		t.Fatalf("want 1 breach, got %+v", got)
	}
	b := got[0]
	if b.Axis != "mileage" || b.Miles != 310 {
		t.Fatalf("want mileage/310mi remaining, got %+v", b)
	}
	// 1 - 310/500
	assertUrgency(t, b.Urgency, 1-310.0/500.0)
}

func TestDueBreaches_timeOverdue(t *testing.T) {
	due := base.AddDate(0, 12, 0)
	got := DueBreaches(timeOnly(), due.AddDate(0, 0, 12), 0, DefaultThresholds)
	if len(got) != 1 {
		t.Fatalf("want 1 breach, got %+v", got)
	}
	b := got[0]
	if b.Axis != "time" || b.Days != 12 || b.Miles != 0 {
		t.Fatalf("want time/12d/0mi, got %+v", b)
	}
	assertUrgency(t, b.Urgency, 1+12.0/30.0)
}

func TestDueBreaches_timeUpcoming(t *testing.T) {
	due := base.AddDate(0, 12, 0)
	got := DueBreaches(timeOnly(), due.AddDate(0, 0, -20), 0, DefaultThresholds)
	if len(got) != 1 || got[0].Axis != "time" || got[0].Days != 20 {
		t.Fatalf("want time/20d remaining, got %+v", got)
	}
	assertUrgency(t, got[0].Urgency, 1-20.0/30.0)
}

func TestDueBreaches_overdueByPartOfADayFloorsToOne(t *testing.T) {
	// "overdue by 0 days" is nonsense in the card copy.
	due := base.AddDate(0, 12, 0)
	got := DueBreaches(timeOnly(), due.Add(2*time.Hour), 0, DefaultThresholds)
	if len(got) != 1 || got[0].Days != 1 {
		t.Fatalf("want a 1-day floor, got %+v", got)
	}
}

func TestDueBreaches_upcomingDueTodayIsZeroDays(t *testing.T) {
	// 0 is legal on the upcoming branch and means "due today".
	due := base.AddDate(0, 12, 0)
	got := DueBreaches(timeOnly(), due, 0, DefaultThresholds)
	if len(got) != 1 || got[0].Axis != "time" || got[0].Days != 0 {
		t.Fatalf("want time/0d, got %+v", got)
	}
}

func TestDueBreaches_hybridBreachingOnBothAxes(t *testing.T) {
	due := base.AddDate(0, 12, 0)
	got := DueBreaches(hybridBoth(), due.AddDate(0, 0, 5), 35600, DefaultThresholds)
	if len(got) != 2 {
		t.Fatalf("want 2 breaches, got %+v", got)
	}
	byAxis := map[string]AxisBreach{}
	for _, b := range got {
		byAxis[b.Axis] = b
	}
	if byAxis["time"].Days != 5 {
		t.Fatalf("time axis = %+v", byAxis["time"])
	}
	if byAxis["mileage"].Miles != 600 {
		t.Fatalf("mileage axis = %+v", byAxis["mileage"])
	}
	// Design D2's worked example: a 600-mile overrun outranks a 5-day overrun.
	if byAxis["mileage"].Urgency <= byAxis["time"].Urgency {
		t.Fatalf("600mi (%v) must outrank 5d (%v)", byAxis["mileage"].Urgency, byAxis["time"].Urgency)
	}
}

func TestDueBreaches_hybridOverdueOnMileageWithHealthyTimeAxisReportsOneBreach(t *testing.T) {
	// The time axis is nowhere near due, so it must not appear at all — and
	// certainly not as an "upcoming in -40 days" second entry.
	got := DueBreaches(hybridBoth(), base.AddDate(0, 1, 0), 36000, DefaultThresholds)
	if len(got) != 1 || got[0].Axis != "mileage" {
		t.Fatalf("want a single mileage breach, got %+v", got)
	}
}

func TestDueBreaches_hybridOverdueOnTimeWithHealthyMileageAxis(t *testing.T) {
	due := base.AddDate(0, 12, 0)
	got := DueBreaches(hybridBoth(), due.AddDate(0, 0, 3), 31000, DefaultThresholds)
	if len(got) != 1 || got[0].Axis != "time" {
		t.Fatalf("want a single time breach, got %+v", got)
	}
}

func TestDueBreaches_upcomingMoreImminentScoresHigher(t *testing.T) {
	// The reason upcoming inverts the ratio: 20mi remaining is more urgent than
	// 490mi remaining, and a naive "largest normalized value" would say the
	// opposite.
	near := DueBreaches(mileageOnly(), base, 34980, DefaultThresholds)
	far := DueBreaches(mileageOnly(), base, 34510, DefaultThresholds)
	if near[0].Urgency <= far[0].Urgency {
		t.Fatalf("20mi (%v) must outrank 490mi (%v)", near[0].Urgency, far[0].Urgency)
	}
}

func TestDueBreaches_agreesWithDueState(t *testing.T) {
	// The invariant the whole design rests on: DueBreaches is non-empty for
	// exactly the schedules DueState calls non-ok. If this ever fails, a card
	// can show a tinted banner with no detail, or a quiet banner on an overdue
	// vehicle.
	sched := hybridBoth()
	for _, days := range []int{0, 30, 300, 335, 360, 366, 400} {
		for _, miles := range []int{0, 30000, 34400, 34600, 35000, 35001, 40000} {
			today := base.AddDate(0, 0, days)
			state := DueState(sched, today, miles, DefaultThresholds)
			breaches := DueBreaches(sched, today, miles, DefaultThresholds)
			if (state == "ok") != (len(breaches) == 0) {
				t.Fatalf("day %d mileage %d: state=%q but %d breaches", days, miles, state, len(breaches))
			}
		}
	}
}

// The due-point override is per AXIS, not per schedule: a hybrid row can hold a
// date anchor and no mileage anchor mid-migration, and each axis must resolve
// independently rather than one anchor suppressing the other's arithmetic.
//
// The !OneTime guard is what makes a one-time axis terminal. Without it, a
// completed one-time schedule whose anchor was cleared falls back to
// lastCompleted + 0 — "due again the instant it was completed".
func TestNextDue_duePointAndOneTime(t *testing.T) {
	anchorDate := time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name      string
		s         Schedule
		wantDate  time.Time
		wantMiles int
	}{
		{
			name:      "one-time time: the anchor verbatim",
			s:         Schedule{RecurrenceType: "time", OneTime: true, DueDate: anchorDate},
			wantDate:  anchorDate,
			wantMiles: 0,
		},
		{
			name:      "one-time mileage: the anchor verbatim",
			s:         Schedule{RecurrenceType: "mileage", OneTime: true, DueMileage: 60000},
			wantDate:  time.Time{},
			wantMiles: 60000,
		},
		{
			name:      "one-time hybrid: both anchors verbatim",
			s:         Schedule{RecurrenceType: "hybrid", OneTime: true, DueDate: anchorDate, DueMileage: 60000},
			wantDate:  anchorDate,
			wantMiles: 60000,
		},
		{
			name: "completed one-time with a cleared anchor yields the zero due point",
			s: Schedule{
				RecurrenceType:       "hybrid",
				OneTime:              true,
				LastCompletedDate:    base,
				LastCompletedMileage: 42000,
			},
			wantDate:  time.Time{},
			wantMiles: 0,
		},
		{
			name: "recurring anchored: the anchor wins over interval arithmetic",
			s: Schedule{
				RecurrenceType: "hybrid",
				IntervalMonths: 12,
				IntervalMiles:  5000,
				DueDate:        anchorDate,
				DueMileage:     60000,
			},
			wantDate:  anchorDate,
			wantMiles: 60000,
		},
		{
			name: "recurring unanchored: interval arithmetic from the completion point",
			s: Schedule{
				RecurrenceType:       "hybrid",
				IntervalMonths:       12,
				IntervalMiles:        5000,
				LastCompletedDate:    base,
				LastCompletedMileage: 30000,
			},
			wantDate:  base.AddDate(0, 12, 0),
			wantMiles: 35000,
		},
		{
			name: "recurring half-anchored: each axis resolves independently",
			s: Schedule{
				RecurrenceType:       "hybrid",
				IntervalMonths:       12,
				IntervalMiles:        5000,
				DueDate:              anchorDate,
				LastCompletedDate:    base,
				LastCompletedMileage: 30000,
			},
			wantDate:  anchorDate,
			wantMiles: 35000,
		},
		{
			name: "a mileage anchor is ignored on a pure-time schedule",
			s: Schedule{
				RecurrenceType:    "time",
				IntervalMonths:    12,
				DueMileage:        60000,
				LastCompletedDate: base,
			},
			wantDate:  base.AddDate(0, 12, 0),
			wantMiles: 0,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			nd, nm := NextDue(c.s)
			if !nd.Equal(c.wantDate) {
				t.Errorf("next_due_date = %v want %v", nd, c.wantDate)
			}
			if nm != c.wantMiles {
				t.Errorf("next_due_mileage = %d want %d", nm, c.wantMiles)
			}
		})
	}
}

// A one-time time-axis schedule with no anchor resolves to the zero time, which
// DueState reads as overdue. Validation (Task 4) makes this unreachable on any
// row this service can create and the backfill (Task 7) repairs legacy rows;
// this test pins the behaviour so neither safeguard is quietly relied upon.
func TestNextDue_oneTimeWithoutAnchorIsZero(t *testing.T) {
	nd, nm := NextDue(Schedule{RecurrenceType: "hybrid", OneTime: true})
	if !nd.IsZero() || nm != 0 {
		t.Fatalf("want the zero due point, got %v / %d", nd, nm)
	}
	if got := DueState(Schedule{RecurrenceType: "time", OneTime: true}, base, 0, DefaultThresholds); got != "overdue" {
		t.Fatalf("an unanchored one-time time axis reads as %s, want overdue", got)
	}
}
