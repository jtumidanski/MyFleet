package fuel

import "time"

// Model is immutable; all state changes yield new instances.
type Model struct {
	id              string
	vehicleID       string
	date            time.Time
	mileage         int
	gallons         float64
	totalCost       float64
	pricePerGallon  float64
	createdByUserID string
	createdAt       time.Time
	updatedAt       time.Time
	deletedAt       *time.Time
}

func (m Model) ID() string              { return m.id }
func (m Model) VehicleID() string       { return m.vehicleID }
func (m Model) Date() time.Time         { return m.date }
func (m Model) Mileage() int            { return m.mileage }
func (m Model) Gallons() float64        { return m.gallons }
func (m Model) TotalCost() float64      { return m.totalCost }
func (m Model) PricePerGallon() float64 { return m.pricePerGallon }
func (m Model) CreatedByUserID() string { return m.createdByUserID }
func (m Model) CreatedAt() time.Time    { return m.createdAt }
func (m Model) UpdatedAt() time.Time    { return m.updatedAt }
func (m Model) DeletedAt() *time.Time   { return m.deletedAt }

// WithMileage returns a copy with the mileage changed.
func (m Model) WithMileage(miles int) Model { m.mileage = miles; return m }

// WithGallons returns a copy with the gallons changed.
func (m Model) WithGallons(gallons float64) Model { m.gallons = gallons; return m }

// WithTotalCost returns a copy with the total cost changed.
func (m Model) WithTotalCost(total float64) Model { m.totalCost = total; return m }

// WithPricePerGallon returns a copy with the price per gallon changed.
func (m Model) WithPricePerGallon(price float64) Model { m.pricePerGallon = price; return m }

// WithDate returns a copy with the date changed.
func (m Model) WithDate(date time.Time) Model { m.date = date; return m }

// WithNotes is a no-op placeholder (notes not in PRD schema but satisfies future extension).
// WithSoftDelete returns a copy stamped with deletedAt.
func (m Model) WithSoftDelete(deletedAt time.Time) Model {
	m.deletedAt = &deletedAt
	return m
}
