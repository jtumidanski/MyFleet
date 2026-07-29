package maintenancerecord

import (
	"time"

	"github.com/google/uuid"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
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

// Build validates invariants and returns the model or a validation error.
func (b *Builder) Build() (Model, error) {
	if b.m.vehicleID == "" || b.m.categoryID == "" || b.m.performedAt.IsZero() {
		return Model{}, server.ErrValidation
	}
	return b.m, nil
}
