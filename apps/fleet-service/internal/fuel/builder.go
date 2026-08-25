package fuel

import (
	"time"

	"github.com/google/uuid"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// Builder constructs a valid fuel log Model. Build() returns (Model, error)
// because fuel logs have invariants (gallons > 0, vehicleID required).
type Builder struct{ m Model }

func NewBuilder() *Builder {
	return &Builder{m: Model{id: uuid.NewString(), createdAt: time.Now().UTC()}}
}

func (b *Builder) SetVehicleID(vehicleID string) *Builder   { b.m.vehicleID = vehicleID; return b }
func (b *Builder) SetDate(date time.Time) *Builder          { b.m.date = date; return b }
func (b *Builder) SetMileage(mileage int) *Builder          { b.m.mileage = mileage; return b }
func (b *Builder) SetGallons(gallons float64) *Builder      { b.m.gallons = gallons; return b }
func (b *Builder) SetTotalCost(total float64) *Builder      { b.m.totalCost = total; return b }
func (b *Builder) SetPricePerGallon(price float64) *Builder { b.m.pricePerGallon = price; return b }

func (b *Builder) SetCreatedByUserID(userID string) *Builder { b.m.createdByUserID = userID; return b }

// Build validates invariants and returns the Model or server.ErrValidation.
func (b *Builder) Build() (Model, error) {
	if b.m.vehicleID == "" {
		return Model{}, server.ErrValidation
	}
	if b.m.gallons <= 0 {
		return Model{}, server.ErrValidation
	}
	return b.m, nil
}
