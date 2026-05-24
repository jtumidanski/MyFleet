package maintenancerecord

import "time"

// Model is the immutable maintenance record domain model (design §8.2).
type Model struct {
	id               string
	vehicleID        string
	categoryID       string
	performedAt      time.Time
	mileage          int
	cost             float64
	vendor           string
	notes            string
	createdByUserID  string
	createdAt        time.Time
	updatedAt        time.Time
	deletedAt        *time.Time
	documentMediaIDs []string
}

func (m Model) ID() string                 { return m.id }
func (m Model) VehicleID() string          { return m.vehicleID }
func (m Model) CategoryID() string         { return m.categoryID }
func (m Model) PerformedAt() time.Time     { return m.performedAt }
func (m Model) Mileage() int               { return m.mileage }
func (m Model) Cost() float64              { return m.cost }
func (m Model) Vendor() string             { return m.vendor }
func (m Model) Notes() string              { return m.notes }
func (m Model) CreatedByUserID() string    { return m.createdByUserID }
func (m Model) CreatedAt() time.Time       { return m.createdAt }
func (m Model) UpdatedAt() time.Time       { return m.updatedAt }
func (m Model) DeletedAt() *time.Time      { return m.deletedAt }
func (m Model) DocumentMediaIDs() []string { return m.documentMediaIDs }

// WithMileage returns a copy with the mileage changed.
func (m Model) WithMileage(miles int) Model { m.mileage = miles; return m }

// WithCost returns a copy with the cost changed.
func (m Model) WithCost(cost float64) Model { m.cost = cost; return m }

// WithVendor returns a copy with the vendor changed.
func (m Model) WithVendor(vendor string) Model { m.vendor = vendor; return m }

// WithNotes returns a copy with the notes changed.
func (m Model) WithNotes(notes string) Model { m.notes = notes; return m }

// WithPerformedAt returns a copy with the performed-at timestamp changed.
func (m Model) WithPerformedAt(t time.Time) Model { m.performedAt = t; return m }

// WithDocumentMediaIDs returns a copy with the attached media references set.
func (m Model) WithDocumentMediaIDs(ids []string) Model { m.documentMediaIDs = ids; return m }
