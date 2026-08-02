package fleet

import "time"

// Model is immutable; state changes return new instances.
type Model struct {
	id              string
	name            string
	createdByUserID string
	createdAt       time.Time
	updatedAt       time.Time
	deletedAt       *time.Time
}

func (m Model) ID() string              { return m.id }
func (m Model) Name() string            { return m.name }
func (m Model) CreatedByUserID() string { return m.createdByUserID }
func (m Model) CreatedAt() time.Time    { return m.createdAt }
func (m Model) UpdatedAt() time.Time    { return m.updatedAt }

// DeletedAt returns the soft-delete tombstone, or nil for a live fleet. The
// value is carried through the Entity round-trip rather than protected by a
// `<-:create` tag: tagging a gorm.DeletedAt makes a restore via Updates(map)
// report success while the row stays deleted (task-006 design §2, V6).
func (m Model) DeletedAt() *time.Time { return m.deletedAt }

// WithName returns a copy with the name changed.
func (m Model) WithName(name string) Model {
	m.name = name
	return m
}
