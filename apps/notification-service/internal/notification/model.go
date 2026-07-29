package notification

import "time"

// Model is the immutable notification domain model (design §8.4, PRD §6). A
// notification is owned by a single user; dedupe_key is globally unique and is
// constructed by the producer to include the user id so each user gets their
// own deduplicated copy (design A6).
type Model struct {
	id        string
	userID    string
	typ       string // overdue | maintenance.completed | fuel.logged | vehicle.created | member.invited
	title     string
	body      string
	dedupeKey string
	vehicleID string // optional ("" when not vehicle-scoped)
	fleetID   string
	readAt    *time.Time // nil until the user marks it read
	createdAt time.Time
}

func (m Model) ID() string           { return m.id }
func (m Model) UserID() string       { return m.userID }
func (m Model) Type() string         { return m.typ }
func (m Model) Title() string        { return m.title }
func (m Model) Body() string         { return m.body }
func (m Model) DedupeKey() string    { return m.dedupeKey }
func (m Model) VehicleID() string    { return m.vehicleID }
func (m Model) FleetID() string      { return m.fleetID }
func (m Model) ReadAt() *time.Time   { return m.readAt }
func (m Model) CreatedAt() time.Time { return m.createdAt }
func (m Model) Read() bool           { return m.readAt != nil }

// WithReadAt returns a copy stamped read at the given time.
func (m Model) WithReadAt(t time.Time) Model {
	m.readAt = &t
	return m
}
