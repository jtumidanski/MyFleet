package vehicle

import (
	"time"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/status"
)

// inactivityDays is the threshold past which a vehicle with no recent activity
// is considered "Inactive" (design §10.2).
const inactivityDays = 365

// LastActivityGatherer returns the most-recent activity timestamp for a vehicle
// (design §8.2). Satisfied by an adapter over *activity.Processor; falls back to
// the vehicle's created_at when there is no recorded activity.
type LastActivityGatherer interface {
	LastActivityByVehicle(vehicleID string) (time.Time, error)
}

// StatusDeps bundles the read-only accessors needed to derive a vehicle's
// read-time values. Both are injected from main.go (cross-domain) so the vehicle
// resource never calls another domain's internals directly.
type StatusDeps struct {
	Schedules ScheduleDueGatherer
	Activity  LastActivityGatherer
}

// Derived is everything the vehicle resource exposes that is computed on read
// rather than stored. A zero Derived means a gather failed; every derived
// attribute is then omitted from the response and the read still succeeds.
type Derived struct {
	Status         string    // "" when a gather failed
	LastActivityAt time.Time // zero when unavailable
	NextDue        *NextDue  // nil when no schedule is non-ok
}

// Derive computes a vehicle's read-only values in one pass: it gathers the
// vehicle's active schedules' due detail and its last activity time, applies the
// priority-ordered rule in status.Derive, and selects the single breach that
// explains the resulting status.
//
// On any gather error it returns the zero Derived, so the caller omits the
// attributes rather than failing the read. That is unchanged behaviour, applied
// to three values instead of one.
func (d StatusDeps) Derive(m Model, now time.Time) Derived {
	dues, err := d.Schedules.ScheduleDueByVehicle(m.ID())
	if err != nil {
		return Derived{}
	}
	last, err := d.Activity.LastActivityByVehicle(m.ID())
	if err != nil {
		return Derived{}
	}
	// Guard against a missing activity record: fall back to the vehicle's
	// creation time so a brand-new vehicle is "Healthy", not "Inactive". The
	// exposed timestamp is this post-fallback value — the same one status
	// derivation used — so the card can never show an em-dash beside a status
	// computed from a real timestamp. A vehicle whose created_at is also zero
	// leaves the field zero, and the attribute is omitted.
	if last.IsZero() {
		last = m.CreatedAt()
	}
	return Derived{
		Status: status.Derive(status.Input{
			ScheduleStates: scheduleStates(dues),
			LastActivityAt: last,
			Now:            now,
			InactivityDays: inactivityDays,
		}),
		LastActivityAt: last,
		NextDue:        selectNextDue(dues),
	}
}

// scheduleStates projects the widened due detail back down to the plain state
// strings status.Derive consumes. status.Derive's input, rule, and output are
// deliberately untouched: this projection is the only new code on the status
// path, which is what makes "status values are unchanged" a testable claim
// rather than a hope.
func scheduleStates(dues []ScheduleDue) []string {
	out := make([]string, 0, len(dues))
	for _, d := range dues {
		out = append(out, d.State)
	}
	return out
}
