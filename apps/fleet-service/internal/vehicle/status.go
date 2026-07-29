package vehicle

import (
	"time"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/status"
)

// inactivityDays is the threshold past which a vehicle with no recent activity
// is considered "Inactive" (design §10.2).
const inactivityDays = 365

// ScheduleStateGatherer returns the live DueState ("ok"|"upcoming"|"overdue")
// of every active maintenance schedule for a vehicle. Injected (read-only) so
// the vehicle layer can derive status on read without owning schedule internals.
// Satisfied by an adapter over *maintenanceschedule.Processor.
type ScheduleStateGatherer interface {
	ScheduleStatesByVehicle(vehicleID string) ([]string, error)
}

// LastActivityGatherer returns the most-recent activity timestamp for a vehicle
// (design §8.2). Satisfied by an adapter over *activity.Processor; falls back to
// the vehicle's created_at when there is no recorded activity.
type LastActivityGatherer interface {
	LastActivityByVehicle(vehicleID string) (time.Time, error)
}

// StatusDeps bundles the read-only accessors needed to derive a vehicle's status
// on read. Both are injected from main.go (cross-domain) so the vehicle resource
// never calls another domain's internals directly.
type StatusDeps struct {
	Schedules ScheduleStateGatherer
	Activity  LastActivityGatherer
}

// DeriveStatus computes a vehicle's read-only status by gathering its active
// schedules' due-states and its last activity time, then applying the
// priority-ordered rule in status.Derive (design §10.2). On any gather error it
// returns "" so the caller can omit status rather than fail the read.
func (d StatusDeps) DeriveStatus(m Model, now time.Time) string {
	states, err := d.Schedules.ScheduleStatesByVehicle(m.ID())
	if err != nil {
		return ""
	}
	last, err := d.Activity.LastActivityByVehicle(m.ID())
	if err != nil {
		return ""
	}
	// Guard against a missing activity record: fall back to the vehicle's
	// creation time so a brand-new vehicle is "Healthy", not "Inactive".
	if last.IsZero() {
		last = m.CreatedAt()
	}
	return status.Derive(status.Input{
		ScheduleStates: states,
		LastActivityAt: last,
		Now:            now,
		InactivityDays: inactivityDays,
	})
}
