package maintenancerecord

import (
	"errors"

	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// Processor contains maintenance record business logic, injected with
// Provider and Administrator.
type Processor struct {
	log logrus.FieldLogger
	p   Provider
	a   Administrator
}

func NewProcessor(log logrus.FieldLogger, p Provider, a Administrator) *Processor {
	return &Processor{log: log, p: p, a: a}
}

// GetByID fetches a non-deleted maintenance record.
func (pr *Processor) GetByID(id string) (Model, error) {
	m, err := pr.p.GetByID(id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Model{}, server.ErrNotFound
		}
		return Model{}, err
	}
	return m, nil
}

// ListByVehicle returns a page of maintenance records for a vehicle.
func (pr *Processor) ListByVehicle(vehicleID string, page server.Page) ([]Model, int, error) {
	return pr.p.ListByVehicle(vehicleID, page)
}

// Create inserts a new maintenance record.
func (pr *Processor) Create(m Model) (Model, error) {
	return pr.a.Insert(m)
}

// Update applies a partial update to an existing maintenance record.
func (pr *Processor) Update(id string, apply func(Model) Model) (Model, error) {
	m, err := pr.GetByID(id)
	if err != nil {
		return Model{}, err
	}
	return pr.a.Update(apply(m))
}

// SoftDelete marks a maintenance record as deleted.
func (pr *Processor) SoftDelete(id string) error {
	err := pr.a.SoftDelete(id)
	if errors.Is(err, ErrNotFound) {
		return server.ErrNotFound
	}
	return err
}
