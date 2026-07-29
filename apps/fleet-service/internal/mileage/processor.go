package mileage

import (
	"github.com/sirupsen/logrus"
)

// VehicleMileageUpdater is satisfied by vehicle.Administrator.UpdateCurrentMileage.
// Injected so the processor does not import the vehicle package directly.
type VehicleMileageUpdater interface {
	UpdateCurrentMileage(vehicleID string, m int) error
}

// Processor holds mileage business logic.
type Processor struct {
	log     logrus.FieldLogger
	updater VehicleMileageUpdater
}

// NewProcessor constructs a Processor with the given logger and mileage updater.
func NewProcessor(log logrus.FieldLogger, updater VehicleMileageUpdater) *Processor {
	return &Processor{log: log, updater: updater}
}

// OnAppend is the reusable hook called after inserting a mileage record.
// If rec.Mileage() < currentLatest the record is flagged and current_mileage
// is NOT advanced (history is kept — insert already happened).
// If rec.Mileage() >= currentLatest current_mileage is advanced via the updater.
//
// This hook is designed for reuse by fuel and maintenance processors (design §10.4).
func (pr *Processor) OnAppend(rec Model, currentLatest int) (flagged bool, err error) {
	if rec.Mileage() < currentLatest {
		return true, nil
	}
	if err := pr.updater.UpdateCurrentMileage(rec.VehicleID(), rec.Mileage()); err != nil {
		return false, err
	}
	return false, nil
}
