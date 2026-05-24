package activity

import (
	"time"

	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// Processor contains activity-feed read logic, injected with a Provider. Writes
// happen via the Record helper inside other domains' transactions (design §8.2),
// so the processor exposes thin reads only.
type Processor struct {
	log logrus.FieldLogger
	p   Provider
}

func NewProcessor(log logrus.FieldLogger, p Provider) *Processor {
	return &Processor{log: log, p: p}
}

// ListByFleet returns a page of fleet-level activity, newest first.
func (pr *Processor) ListByFleet(fleetID string, page server.Page) ([]Model, int, error) {
	return pr.p.ListByFleet(fleetID, page)
}

// ListByVehicle returns a page of a vehicle's activity timeline, newest first.
func (pr *Processor) ListByVehicle(vehicleID string, page server.Page) ([]Model, int, error) {
	return pr.p.ListByVehicle(vehicleID, page)
}

// LastActivityByVehicle returns the most-recent activity time for a vehicle
// (zero time if none). Satisfies vehicle.LastActivityGatherer for read-time
// status derivation (design §10.2).
func (pr *Processor) LastActivityByVehicle(vehicleID string) (time.Time, error) {
	return pr.p.LastActivityByVehicle(vehicleID)
}
