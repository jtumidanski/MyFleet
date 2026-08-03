package membership

// Model is immutable; state changes return new instances.
type Model struct {
	id      string
	fleetID string
	userID  string
	role    string // owner | member | viewer
	status  string // active | ...
}

func (m Model) ID() string      { return m.id }
func (m Model) FleetID() string { return m.fleetID }
func (m Model) UserID() string  { return m.userID }
func (m Model) Role() string    { return m.role }
func (m Model) Status() string  { return m.status }

// WithRole returns a copy carrying the new role. Value receiver and value
// return: the copy IS the new instance, which is what makes the transition
// immutable without a builder. Matches user.Model.WithLogin in auth-service.
func (m Model) WithRole(role string) Model {
	m.role = role
	return m
}
