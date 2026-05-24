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
