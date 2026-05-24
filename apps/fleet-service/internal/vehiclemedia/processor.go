package vehiclemedia

import (
	"errors"

	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// Processor contains vehicle media business logic.
type Processor struct {
	log logrus.FieldLogger
	p   Provider
	a   Administrator
}

func NewProcessor(log logrus.FieldLogger, p Provider, a Administrator) *Processor {
	return &Processor{log: log, p: p, a: a}
}

// ListByVehicle returns all non-deleted media refs for a vehicle.
func (pr *Processor) ListByVehicle(vehicleID string) ([]Model, error) {
	return pr.p.ListByVehicle(vehicleID)
}

// AddMedia inserts a new media reference for a vehicle.
func (pr *Processor) AddMedia(m Model) (Model, error) {
	return pr.a.Insert(m)
}

// SetPrimary sets exactly one row is_primary=true and clears the others,
// then mirrors the chosen media_id into fleet.vehicles. Runs all updates
// sequentially (administrator handles each DB call; atomicity is best-effort
// at this layer — a transaction wrapper in administrator handles the real tx).
func (pr *Processor) SetPrimary(vehicleID, mediaID string) error {
	rows, err := pr.p.ListByVehicle(vehicleID)
	if err != nil {
		return err
	}

	// Verify the target media exists for this vehicle.
	var target *Model
	for i := range rows {
		if rows[i].MediaID() == mediaID {
			target = &rows[i]
			break
		}
	}
	if target == nil {
		return server.ErrNotFound
	}

	// Clear is_primary on all rows that are currently primary (other than target).
	for _, row := range rows {
		if row.IsPrimary() && row.ID() != target.ID() {
			if err := pr.a.SetIsPrimary(row.ID(), false); err != nil {
				return err
			}
		}
	}

	// Set is_primary on the target row.
	if err := pr.a.SetIsPrimary(target.ID(), true); err != nil {
		return err
	}

	// Mirror into fleet.vehicles.primary_image_media_id.
	return pr.a.UpdateVehiclePrimaryImage(vehicleID, mediaID)
}

// GetByVehicleAndMedia fetches a specific media ref.
func (pr *Processor) GetByVehicleAndMedia(vehicleID, mediaID string) (Model, error) {
	m, err := pr.p.GetByVehicleAndMedia(vehicleID, mediaID)
	if errors.Is(err, ErrNotFound) {
		return Model{}, server.ErrNotFound
	}
	return m, err
}
