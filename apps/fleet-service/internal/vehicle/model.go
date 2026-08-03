package vehicle

import "time"

// Model is immutable; state changes return new instances.
type Model struct {
	id                  string
	fleetID             string
	nickname            string
	make                string
	model               string
	trim                string
	year                int
	vin                 string
	currentMileage      int
	primaryImageMediaID string
	notes               string
	createdAt           time.Time
	updatedAt           time.Time
	deletedAt           *time.Time
	// purgeOperationID must round-trip through the Model: Administrator writes
	// with db.Save, which UPDATEs every column, so a field the Model does not
	// carry is silently zeroed on any ordinary write. Zeroing THIS one detaches
	// the row from the purge that owns it — it stays soft-deleted but becomes
	// unreachable by both restore and reap. entityguard caught it.
	purgeOperationID *string
	purgeAfter       *time.Time
}

func (m Model) ID() string                  { return m.id }
func (m Model) FleetID() string             { return m.fleetID }
func (m Model) Nickname() string            { return m.nickname }
func (m Model) Make() string                { return m.make }
func (m Model) Model() string               { return m.model }
func (m Model) Trim() string                { return m.trim }
func (m Model) Year() int                   { return m.year }
func (m Model) VIN() string                 { return m.vin }
func (m Model) CurrentMileage() int         { return m.currentMileage }
func (m Model) PrimaryImageMediaID() string { return m.primaryImageMediaID }
func (m Model) Notes() string               { return m.notes }
func (m Model) CreatedAt() time.Time        { return m.createdAt }
func (m Model) UpdatedAt() time.Time        { return m.updatedAt }
func (m Model) DeletedAt() *time.Time       { return m.deletedAt }

// PurgeOperationID is the admin purge operation that owns this row, if any.
func (m Model) PurgeOperationID() *string { return m.purgeOperationID }
func (m Model) PurgeAfter() *time.Time    { return m.purgeAfter }

// WithNickname returns a copy with the nickname changed.
func (m Model) WithNickname(nickname string) Model {
	m.nickname = nickname
	return m
}

// WithCurrentMileage returns a copy with the current mileage changed.
func (m Model) WithCurrentMileage(miles int) Model {
	m.currentMileage = miles
	return m
}

// WithNotes returns a copy with the notes changed.
func (m Model) WithNotes(notes string) Model {
	m.notes = notes
	return m
}

// WithPrimaryImageMediaID returns a copy with the primary image media ID changed.
func (m Model) WithPrimaryImageMediaID(mediaID string) Model {
	m.primaryImageMediaID = mediaID
	return m
}

// WithSoftDelete returns a copy stamped with deletedAt and purgeAfter.
func (m Model) WithSoftDelete(deletedAt time.Time) Model {
	purge := ComputePurgeAfter(deletedAt)
	m.deletedAt = &deletedAt
	m.purgeAfter = &purge
	return m
}

// WithRestored returns a copy cleared of soft-delete fields.
func (m Model) WithRestored() Model {
	m.deletedAt = nil
	m.purgeAfter = nil
	return m
}
