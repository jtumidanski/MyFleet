package mileage

import "time"

// Model is immutable; append-only (no updates or deletes).
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
