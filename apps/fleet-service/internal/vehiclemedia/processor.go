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
// then mirrors the chosen media_id into fleet.vehicles. The three mutations
// (clear old primaries, set new primary, update vehicle mirror) execute inside
// a single database transaction via SetPrimaryAtomic; a partial failure rolls
// back entirely, preventing zero-primary or dual-primary inconsistencies.
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

	// Collect IDs of rows that are currently primary and need to be cleared.
	var clearIDs []string
	for _, row := range rows {
		if row.IsPrimary() && row.ID() != target.ID() {
			clearIDs = append(clearIDs, row.ID())
		}
	}

	// Perform the atomic mutation: clear + set + mirror in one transaction.
	return pr.a.SetPrimaryAtomic(vehicleID, target.ID(), mediaID, clearIDs)
}

// RemoveMedia detaches one media object from a vehicle.
//
// Deliberately thin: the row lookup, the delete, and the primary-successor
// decision all belong to one transaction, so they live in the administrator.
// Reading the row here first — as this did originally — reintroduces a
// time-of-check/time-of-use gap against a concurrent SetPrimary.
//
// Scoping by (vehicleID, mediaID) rather than by row id is what stops a caller
// removing another vehicle's reference by guessing an id: the route authorized
// the vehicle, not the row.
func (pr *Processor) RemoveMedia(vehicleID, mediaID string) error {
	if err := pr.a.SoftDelete(vehicleID, mediaID); err != nil {
		// ErrNotFound is package-private; callers get the shared sentinel the
		// HTTP layer maps to 404 rather than a raw gorm error mapped to 500.
		if errors.Is(err, ErrNotFound) {
			return server.ErrNotFound
		}
		return err
	}
	return nil
}

// GetByVehicleAndMedia fetches a specific media ref.
func (pr *Processor) GetByVehicleAndMedia(vehicleID, mediaID string) (Model, error) {
	m, err := pr.p.GetByVehicleAndMedia(vehicleID, mediaID)
	if errors.Is(err, ErrNotFound) {
		return Model{}, server.ErrNotFound
	}
	return m, err
}
