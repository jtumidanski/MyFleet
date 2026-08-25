package maintenancerecord

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// Builder constructs a valid maintenance record Model. Build() returns
// (Model, error) because vehicleID, categoryID, and performedAt are invariants
// enforced at construction time (design §8.2).
type Builder struct{ m Model }

func NewBuilder() *Builder {
	return &Builder{m: Model{id: uuid.NewString(), createdAt: time.Now().UTC()}}
}

func (b *Builder) SetVehicleID(vehicleID string) *Builder    { b.m.vehicleID = vehicleID; return b }
func (b *Builder) SetCategoryID(categoryID string) *Builder  { b.m.categoryID = categoryID; return b }
func (b *Builder) SetPerformedAt(t time.Time) *Builder       { b.m.performedAt = t; return b }
func (b *Builder) SetMileage(miles int) *Builder             { b.m.mileage = miles; return b }
func (b *Builder) SetCost(cost float64) *Builder             { b.m.cost = cost; return b }
func (b *Builder) SetVendor(vendor string) *Builder          { b.m.vendor = vendor; return b }
func (b *Builder) SetNotes(notes string) *Builder            { b.m.notes = notes; return b }
func (b *Builder) SetCreatedByUserID(userID string) *Builder { b.m.createdByUserID = userID; return b }

func (b *Builder) SetDocumentMediaIDs(ids []string) *Builder { b.m.documentMediaIDs = ids; return b }

// SetDescription sets the short summary, trimmed of surrounding whitespace.
func (b *Builder) SetDescription(d string) *Builder {
	b.m.description = strings.TrimSpace(d)
	return b
}

// Build validates invariants and returns the model or a validation error.
func (b *Builder) Build() (Model, error) {
	if err := Validate(b.m); err != nil {
		return Model{}, err
	}
	return b.m, nil
}
