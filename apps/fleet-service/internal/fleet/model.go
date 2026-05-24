package fleet

import "time"

// Model is immutable; state changes return new instances.
type Model struct {
	id              string
	name            string
	createdByUserID string
	createdAt       time.Time
	updatedAt       time.Time
}

func (m Model) ID() string              { return m.id }
func (m Model) Name() string            { return m.name }
func (m Model) CreatedByUserID() string { return m.createdByUserID }
func (m Model) CreatedAt() time.Time    { return m.createdAt }
func (m Model) UpdatedAt() time.Time    { return m.updatedAt }

// WithName returns a copy with the name changed.
func (m Model) WithName(name string) Model {
	m.name = name
	return m
}
