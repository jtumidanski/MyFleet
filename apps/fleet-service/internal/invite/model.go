package invite

import "time"

// Model is immutable; state changes return new instances.
type Model struct {
	id              string
	fleetID         string
	email           string
	role            string
	token           string
	expiresAt       time.Time
	acceptedAt      *time.Time
	invitedByUserID string
}

func (m Model) ID() string              { return m.id }
func (m Model) FleetID() string         { return m.fleetID }
func (m Model) Email() string           { return m.email }
func (m Model) Role() string            { return m.role }
func (m Model) Token() string           { return m.token }
func (m Model) ExpiresAt() time.Time    { return m.expiresAt }
func (m Model) AcceptedAt() *time.Time  { return m.acceptedAt }
func (m Model) InvitedByUserID() string { return m.invitedByUserID }
