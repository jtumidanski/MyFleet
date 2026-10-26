package mileage

import (
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// Processor contains mileage business logic.
type Processor struct {
	log  logrus.FieldLogger
	db   *gorm.DB
	prov Provider
	adm  Administrator
}

// NewProcessor constructs a Processor. db is the handle used to open the
// transaction that wraps an append.
func NewProcessor(log logrus.FieldLogger, db *gorm.DB, prov Provider, adm Administrator) *Processor {
	return &Processor{log: log, db: db, prov: prov, adm: adm}
}

// ListByVehicle returns a page of mileage readings for a vehicle.
func (pr *Processor) ListByVehicle(vehicleID string, f ListFilter, page server.Page) ([]Model, int, error) {
	return pr.prov.ListByVehicle(vehicleID, f, page)
}

// Append records a reading and advances the vehicle odometer in one transaction.
func (pr *Processor) Append(m Model) (Model, error) {
	var created Model
	err := pr.db.Transaction(func(tx *gorm.DB) error {
		var txErr error
		created, txErr = pr.adm.AppendTx(tx, m)
		return txErr
	})
	if err != nil {
		return Model{}, err
	}
	return created, nil
}
