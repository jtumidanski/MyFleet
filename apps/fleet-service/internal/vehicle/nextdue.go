package vehicle

// ScheduleDue mirrors maintenanceschedule.ScheduleDue across the domain
// boundary. The shape is duplicated rather than aliased on purpose: an alias
// would make this package import maintenanceschedule transitively, which is the
// exact coupling the port exists to prevent. The mapping lives in the
// composition root (cmd/main.go), field for field, so a change on either side is
// a compile error rather than a silent drop.
type ScheduleDue struct {
	ScheduleID string
	State      string // ok | upcoming | overdue
	Breaches   []Breach
}

// Breach mirrors maintenanceschedule.AxisBreach. Urgency arrives already
// normalized against the schedule domain's own thresholds, so this package
// selects a maximum without ever learning what a threshold is.
type Breach struct {
	Axis    string // "time" | "mileage"
	Days    int
	Miles   int
	Urgency float64
}

// NextDue is the single governing due detail exposed on the vehicle resource.
// Exactly one of Miles/Days is non-nil, chosen by Axis.
//
// Pointers rather than plain ints with omitempty: an upcoming time-axis schedule
// due today has Days == 0, and omitempty on a plain int would drop the key and
// hand the client an axis:"time" object carrying no magnitude at all.
type NextDue struct {
	State string `json:"state"`           // upcoming | overdue
	Axis  string `json:"axis"`            // time | mileage
	Miles *int   `json:"miles,omitempty"` // non-nil iff Axis == "mileage"
	Days  *int   `json:"days,omitempty"`  // non-nil iff Axis == "time"
}

// ScheduleDueGatherer returns the live due-state and per-axis breach detail of
// every active maintenance schedule for a vehicle. Injected (read-only) so the
// vehicle layer can derive on read without owning schedule internals. Satisfied
// by an adapter over *maintenanceschedule.Processor, wired in cmd/main.go.
type ScheduleDueGatherer interface {
	ScheduleDueByVehicle(vehicleID string) ([]ScheduleDue, error)
}

// selectNextDue picks the single breach that explains the vehicle's status: the
// governing state first (overdue beats upcoming — the same priority
// status.Derive applies), then the highest-urgency breach among the schedules in
// that state.
//
// Because Urgency is monotone across the state boundary (overdue is always above
// 1, upcoming always at or below it), an overdue hybrid that also carries an
// upcoming breach on its healthy axis reports the overdue axis with no extra
// filtering.
//
// Ties resolve to the mileage axis, then to the lowest schedule ID. The second
// tiebreak exists because the first is not total: two mileage schedules with
// identical breaches would otherwise be ordered by slice iteration.
//
// Returns nil when no schedule is non-ok, and also when a non-ok schedule
// carries no breach at all — reporting a state with no magnitude would give the
// card a tinted banner it cannot caption.
func selectNextDue(dues []ScheduleDue) *NextDue {
	governing := ""
	for _, d := range dues {
		if d.State == "overdue" {
			governing = "overdue"
			break
		}
		if d.State == "upcoming" {
			governing = "upcoming"
		}
	}
	if governing == "" {
		return nil
	}

	var best Breach
	bestID := ""
	found := false
	for _, d := range dues {
		if d.State != governing {
			continue
		}
		for _, b := range d.Breaches {
			if !found || outranks(b, d.ScheduleID, best, bestID) {
				best, bestID, found = b, d.ScheduleID, true
			}
		}
	}
	if !found {
		return nil
	}

	out := &NextDue{State: governing, Axis: best.Axis}
	switch best.Axis {
	case "mileage":
		miles := best.Miles
		out.Miles = &miles
	case "time":
		days := best.Days
		out.Days = &days
	}
	return out
}

// outranks reports whether candidate c, from schedule cID, beats the incumbent b
// from schedule bID: higher urgency, then the mileage axis, then the lower ID.
func outranks(c Breach, cID string, b Breach, bID string) bool {
	if c.Urgency != b.Urgency {
		return c.Urgency > b.Urgency
	}
	if (c.Axis == "mileage") != (b.Axis == "mileage") {
		return c.Axis == "mileage"
	}
	return cID < bID
}
