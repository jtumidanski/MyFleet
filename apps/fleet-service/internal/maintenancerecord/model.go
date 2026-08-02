package maintenancerecord

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// MaxDescriptionRunes bounds the record's short summary (PRD FR-REC-1).
// Measured in runes, not bytes.
const MaxDescriptionRunes = 200

// MaxDocuments bounds attachments per record (design D9). It bounds three
// things at once: the ids= query string on media-service's internal endpoint,
// the per-record fan-out when an attachment list is expanded, and the size of a
// single InsertTx document loop.
const MaxDocuments = 10

// Model is the immutable maintenance record domain model (design §8.2).
type Model struct {
	id               string
	vehicleID        string
	categoryID       string
	description      string
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
func (m Model) Description() string        { return m.description }
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

// WithCategoryID returns a copy with the category changed.
//
// No emptiness check here: Validate owns that invariant and Processor.Update
// runs it after the mutation function, so a PATCH clearing the category is
// rejected as a validation error rather than silently persisted.
func (m Model) WithCategoryID(categoryID string) Model { m.categoryID = categoryID; return m }

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

// WithDescription returns a copy with the description changed. The value is
// trimmed here so measurement and storage always agree.
func (m Model) WithDescription(d string) Model {
	m.description = strings.TrimSpace(d)
	return m
}

// Validate enforces the model's invariants. It is called by Builder.Build and
// by Processor.Update after the mutation function is applied, so every write
// path is covered by construction — putting the check in the handler would
// duplicate it, and putting it only in the builder would leave PATCH unguarded
// (design D4).
func Validate(m Model) error {
	if m.vehicleID == "" || m.categoryID == "" || m.performedAt.IsZero() {
		return server.ErrValidation
	}
	if utf8.RuneCountInString(m.description) > MaxDescriptionRunes {
		return server.ErrValidation
	}
	if len(m.documentMediaIDs) > MaxDocuments {
		return server.ErrValidation
	}
	return nil
}
