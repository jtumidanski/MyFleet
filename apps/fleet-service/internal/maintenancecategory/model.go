package maintenancecategory

// Model is the immutable maintenance category domain model. Categories are
// global/system data (not fleet-scoped); see design §8.2.
type Model struct {
	id            string
	name          string
	description   string
	systemDefined bool
}

func (m Model) ID() string          { return m.id }
func (m Model) Name() string        { return m.name }
func (m Model) Description() string { return m.description }
func (m Model) SystemDefined() bool { return m.systemDefined }
