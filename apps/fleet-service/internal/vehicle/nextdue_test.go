package vehicle

import "testing"

func mileageBreach(miles int, urgency float64) Breach {
	return Breach{Axis: "mileage", Miles: miles, Urgency: urgency}
}

func timeBreach(days int, urgency float64) Breach {
	return Breach{Axis: "time", Days: days, Urgency: urgency}
}

func TestSelectNextDue(t *testing.T) {
	cases := []struct {
		name      string
		dues      []ScheduleDue
		wantNil   bool
		wantState string
		wantAxis  string
		wantMiles *int
		wantDays  *int
	}{
		{
			name:    "no schedules at all",
			dues:    nil,
			wantNil: true,
		},
		{
			name: "every schedule ok",
			dues: []ScheduleDue{
				{ScheduleID: "s1", State: "ok"},
				{ScheduleID: "s2", State: "ok"},
			},
			wantNil: true,
		},
		{
			name: "single mileage overdue",
			dues: []ScheduleDue{
				{ScheduleID: "s1", State: "overdue", Breaches: []Breach{mileageBreach(1120, 3.24)}},
			},
			wantState: "overdue",
			wantAxis:  "mileage",
			wantMiles: intPtr(1120),
		},
		{
			name: "single time upcoming due today keeps a zero day count",
			dues: []ScheduleDue{
				{ScheduleID: "s1", State: "upcoming", Breaches: []Breach{timeBreach(0, 1.0)}},
			},
			wantState: "upcoming",
			wantAxis:  "time",
			wantDays:  intPtr(0),
		},
		{
			name: "overdue governs even when an upcoming schedule scores high on its own scale",
			dues: []ScheduleDue{
				{ScheduleID: "s-up", State: "upcoming", Breaches: []Breach{mileageBreach(5, 0.99)}},
				{ScheduleID: "s-od", State: "overdue", Breaches: []Breach{timeBreach(2, 1.0667)}},
			},
			wantState: "overdue",
			wantAxis:  "time",
			wantDays:  intPtr(2),
		},
		{
			name: "max urgency wins across several overdue schedules",
			dues: []ScheduleDue{
				{ScheduleID: "s1", State: "overdue", Breaches: []Breach{timeBreach(5, 1+5.0/30.0)}},
				{ScheduleID: "s2", State: "overdue", Breaches: []Breach{mileageBreach(600, 1+600.0/500.0)}},
			},
			wantState: "overdue",
			wantAxis:  "mileage",
			wantMiles: intPtr(600),
		},
		{
			name: "a hybrid overdue on mileage also carrying an upcoming time breach reports the overdue axis",
			dues: []ScheduleDue{
				{ScheduleID: "s1", State: "overdue", Breaches: []Breach{
					mileageBreach(600, 1+600.0/500.0),
					timeBreach(9, 1-9.0/30.0),
				}},
			},
			wantState: "overdue",
			wantAxis:  "mileage",
			wantMiles: intPtr(600),
		},
		{
			name: "mileage wins the tiebreak on equal urgency",
			dues: []ScheduleDue{
				{ScheduleID: "s1", State: "overdue", Breaches: []Breach{timeBreach(30, 2.0)}},
				{ScheduleID: "s2", State: "overdue", Breaches: []Breach{mileageBreach(500, 2.0)}},
			},
			wantState: "overdue",
			wantAxis:  "mileage",
			wantMiles: intPtr(500),
		},
		{
			name: "lowest schedule id breaks a tie on equal urgency and axis",
			dues: []ScheduleDue{
				{ScheduleID: "s-b", State: "overdue", Breaches: []Breach{mileageBreach(700, 2.4)}},
				{ScheduleID: "s-a", State: "overdue", Breaches: []Breach{mileageBreach(700, 2.4)}},
			},
			wantState: "overdue",
			wantAxis:  "mileage",
			wantMiles: intPtr(700),
		},
		{
			name: "most imminent upcoming wins",
			dues: []ScheduleDue{
				{ScheduleID: "s1", State: "upcoming", Breaches: []Breach{mileageBreach(490, 1-490.0/500.0)}},
				{ScheduleID: "s2", State: "upcoming", Breaches: []Breach{mileageBreach(20, 1-20.0/500.0)}},
			},
			wantState: "upcoming",
			wantAxis:  "mileage",
			wantMiles: intPtr(20),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := selectNextDue(c.dues)
			if c.wantNil {
				if got != nil {
					t.Fatalf("want nil, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("want a NextDue, got nil")
			}
			if got.State != c.wantState || got.Axis != c.wantAxis {
				t.Fatalf("state/axis = %q/%q want %q/%q", got.State, got.Axis, c.wantState, c.wantAxis)
			}
			assertIntPtr(t, "miles", got.Miles, c.wantMiles)
			assertIntPtr(t, "days", got.Days, c.wantDays)
		})
	}
}

func TestSelectNextDue_setsExactlyOneMagnitudePointer(t *testing.T) {
	// The invariant the nested-object encoding exists to protect: an axis:"time"
	// object must never carry a miles value, and vice versa.
	mileage := selectNextDue([]ScheduleDue{
		{ScheduleID: "s1", State: "overdue", Breaches: []Breach{mileageBreach(100, 1.2)}},
	})
	if mileage.Miles == nil || mileage.Days != nil {
		t.Fatalf("mileage axis must set Miles and leave Days nil, got %+v", mileage)
	}

	timed := selectNextDue([]ScheduleDue{
		{ScheduleID: "s1", State: "overdue", Breaches: []Breach{timeBreach(10, 1.3)}},
	})
	if timed.Days == nil || timed.Miles != nil {
		t.Fatalf("time axis must set Days and leave Miles nil, got %+v", timed)
	}
}

func TestSelectNextDue_nonOkStateWithNoBreachesYieldsNil(t *testing.T) {
	// Defensive: a state/breach disagreement must not produce a NextDue with no
	// magnitude at all. It cannot happen through the real gatherer (Task 1's
	// agreement test proves it), but selectNextDue is fed across a port.
	if got := selectNextDue([]ScheduleDue{{ScheduleID: "s1", State: "overdue"}}); got != nil {
		t.Fatalf("want nil, got %+v", got)
	}
}

func intPtr(n int) *int { return &n }

func assertIntPtr(t *testing.T, name string, got, want *int) {
	t.Helper()
	switch {
	case want == nil && got != nil:
		t.Fatalf("%s = %d, want nil", name, *got)
	case want != nil && got == nil:
		t.Fatalf("%s = nil, want %d", name, *want)
	case want != nil && *got != *want:
		t.Fatalf("%s = %d, want %d", name, *got, *want)
	}
}
