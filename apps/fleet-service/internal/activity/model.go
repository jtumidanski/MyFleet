package activity

import "time"

// Model is the immutable activity-event domain model (design §8.2). The activity
// feed is APPEND-ONLY, with ONE exception: events are never updated or deleted,
// and this package exposes no way to do either. An admin vehicle transfer
// re-points fleet_id — and ONLY fleet_id — on the moved vehicle's rows, in raw
// SQL inside internal/admin (see that package's transfer.go). fleet_id is a
// denormalised routing column answering "whose feed does this appear in", not
// part of "what happened", so correcting it when the vehicle changes owners
// preserves the feed's meaning rather than revising it.
type Model struct {
	id          string
	fleetID     string
	vehicleID   *string // nil for fleet-level events (e.g. member.invited)
	actorUserID string
	eventType   string
	payload     []byte // JSON-encoded event payload
	createdAt   time.Time
}

func (m Model) ID() string           { return m.id }
func (m Model) FleetID() string      { return m.fleetID }
func (m Model) VehicleID() *string   { return m.vehicleID }
func (m Model) ActorUserID() string  { return m.actorUserID }
func (m Model) Type() string         { return m.eventType }
func (m Model) Payload() []byte      { return m.payload }
func (m Model) CreatedAt() time.Time { return m.createdAt }
