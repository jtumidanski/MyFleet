package fuel

import (
	"errors"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// Derived holds the price-derivation result (design §10.5).
type Derived struct {
	PricePerGallon float64
	TotalCost      float64
}

// DerivePrice fills the missing of {price_per_gallon, total_cost} (design §10.5).
func DerivePrice(price, total, gallons float64) (Derived, error) {
	if gallons <= 0 {
		return Derived{}, server.ErrValidation
	}
	switch {
	case price > 0 && total > 0:
		return Derived{PricePerGallon: price, TotalCost: total}, nil
	case total > 0:
		return Derived{PricePerGallon: total / gallons, TotalCost: total}, nil
	case price > 0:
		return Derived{PricePerGallon: price, TotalCost: price * gallons}, nil
	default:
		return Derived{}, server.ErrValidation
	}
}

// Processor contains fuel business logic.
type Processor struct {
	log  logrus.FieldLogger
	prov Provider
}

// NewProcessor constructs a Processor with the given logger and provider.
func NewProcessor(log logrus.FieldLogger, prov Provider) *Processor {
	return &Processor{log: log, prov: prov}
}

// GetByID fetches a fuel log by ID (only non-deleted rows).
func (pr *Processor) GetByID(id string) (Model, error) {
	m, err := pr.prov.GetByID(id)
	if err != nil {
		if errors.Is(err, ErrNotFound) || errors.Is(err, gorm.ErrRecordNotFound) {
			return Model{}, server.ErrNotFound
		}
		return Model{}, err
	}
	return m, nil
}

// ListByVehicle returns a page of fuel logs for a vehicle.
func (pr *Processor) ListByVehicle(vehicleID string, page server.Page) ([]Model, int, error) {
	return pr.prov.ListByVehicle(vehicleID, page)
}
