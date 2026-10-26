package maintenancerecord

import (
	"errors"

	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/mileage"
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

// ListByVehicle returns a page of records for a vehicle, optionally constrained
// to a set of category IDs (design D3 for the nil/empty semantics).
func (pr *Processor) ListByVehicle(vehicleID string, categoryIDs []string, page server.Page) ([]Model, int, error) {
	return pr.p.ListByVehicle(vehicleID, categoryIDs, page)
}

// Create inserts a new maintenance record.
func (pr *Processor) Create(m Model) (Model, error) {
	return pr.a.Insert(m, mileageEntryForCreate(m))
}

// mileageEntryForCreate returns the ledger entry a new record contributes, or
// nil when the record carries no odometer reading (mileage 0 means unset).
func mileageEntryForCreate(m Model) *mileage.Model {
	if m.Mileage() <= 0 {
		return nil
	}
	e := mileage.NewBuilder().
		SetVehicleID(m.VehicleID()).
		SetMileage(m.Mileage()).
		SetRecordedAt(m.PerformedAt()).
		SetSource(mileage.SourceMaintenance).
		SetSourceRefID(m.ID()).
		SetCreatedByUserID(m.CreatedByUserID()).
		Build()
	return &e
}

// mileageEntryForUpdate returns the ledger entry an edit contributes: nil when
// the mileage did not change or the new value is unset (0); otherwise a new
// entry attributed to the editing actor. Earlier entries are never rewritten.
func mileageEntryForUpdate(before, after Model, actorUserID string) *mileage.Model {
	if after.Mileage() <= 0 || after.Mileage() == before.Mileage() {
		return nil
	}
	e := mileage.NewBuilder().
		SetVehicleID(after.VehicleID()).
		SetMileage(after.Mileage()).
		SetRecordedAt(after.PerformedAt()).
		SetSource(mileage.SourceMaintenance).
		SetSourceRefID(after.ID()).
		SetCreatedByUserID(actorUserID).
		Build()
	return &e
}

// Update applies a partial update to an existing maintenance record. The
// applied model is validated before it reaches the administrator, so PATCH is
// guarded by the same invariants as create (design D4).
func (pr *Processor) Update(id, actorUserID string, apply func(Model) Model) (Model, error) {
	m, err := pr.GetByID(id)
	if err != nil {
		return Model{}, err
	}
	updated := apply(m)
	if err := Validate(updated); err != nil {
		return Model{}, err
	}
	return pr.a.Update(updated, mileageEntryForUpdate(m, updated, actorUserID))
}

// SoftDelete marks a maintenance record as deleted.
func (pr *Processor) SoftDelete(id string) error {
	m, err := pr.GetByID(id)
	if err != nil {
		return err
	}
	err = pr.a.SoftDelete(id, m.VehicleID())
	if errors.Is(err, ErrNotFound) {
		return server.ErrNotFound
	}
	return err
}

// AttachDocument attaches one media reference to a record.
//
// No validation lives here. Validate speaks about the model's fields, and an
// attach mutates rows rather than the model, so there is nothing for it to say
// — which is also why this does not go through Update.
func (pr *Processor) AttachDocument(recordID, mediaID string) (Model, error) {
	m, err := pr.a.AttachDocument(recordID, mediaID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Model{}, server.ErrNotFound
		}
		// server.ErrValidation from the cap check passes straight through;
		// StatusFor maps it to 422.
		return Model{}, err
	}
	return m, nil
}

// DetachDocument removes one media reference from a record.
func (pr *Processor) DetachDocument(recordID, mediaID string) error {
	err := pr.a.DetachDocument(recordID, mediaID)
	if errors.Is(err, ErrNotFound) {
		return server.ErrNotFound
	}
	return err
}
