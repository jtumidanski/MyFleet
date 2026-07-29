package vehiclemedia

import "time"

// Model is immutable; state changes return new instances.
type Model struct {
	id        string
	vehicleID string
	mediaID   string
	isPrimary bool
	sortOrder int
	createdAt time.Time
	deletedAt *time.Time
}

func (m Model) ID() string            { return m.id }
func (m Model) VehicleID() string     { return m.vehicleID }
func (m Model) MediaID() string       { return m.mediaID }
func (m Model) IsPrimary() bool       { return m.isPrimary }
func (m Model) SortOrder() int        { return m.sortOrder }
func (m Model) CreatedAt() time.Time  { return m.createdAt }
func (m Model) DeletedAt() *time.Time { return m.deletedAt }

// WithIsPrimary returns a copy with isPrimary changed.
func (m Model) WithIsPrimary(isPrimary bool) Model {
	m.isPrimary = isPrimary
	return m
}
