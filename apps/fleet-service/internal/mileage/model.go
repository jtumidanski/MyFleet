package mileage

import "time"

// Source values for a mileage reading.
const (
	SourceFuel        = "fuel"
	SourceMaintenance = "maintenance"
	SourceManual      = "manual"
)

// ValidSource reports whether s is a recognised mileage source.
func ValidSource(s string) bool {
	switch s {
	case SourceFuel, SourceMaintenance, SourceManual:
		return true
	}
	return false
}

// Model is immutable; append-only except RetractTx, which soft-deletes a
// parent's rows when the parent is deleted.
type Model struct {
	id              string
	vehicleID       string
	mileage         int
	recordedAt      time.Time
	source          string // fuel | maintenance | manual
	sourceRefID     string
	createdByUserID string
	createdAt       time.Time
	flagged         bool
}

func (m Model) ID() string              { return m.id }
func (m Model) VehicleID() string       { return m.vehicleID }
func (m Model) Mileage() int            { return m.mileage }
func (m Model) RecordedAt() time.Time   { return m.recordedAt }
func (m Model) Source() string          { return m.source }
func (m Model) SourceRefID() string     { return m.sourceRefID }
func (m Model) CreatedByUserID() string { return m.createdByUserID }
func (m Model) CreatedAt() time.Time    { return m.createdAt }
func (m Model) Flagged() bool           { return m.flagged }

// WithFlagged returns a copy with the flagged state set.
func (m Model) WithFlagged(f bool) Model {
	m.flagged = f
	return m
}
