package vehicle

import (
	"errors"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// OwnerChecker performs the authoritative DB-level owner check (stale-claim guard,
// design §9). Satisfied by *membership.Processor.
type OwnerChecker interface {
	RequireOwnerInFleet(fleetID, userID string) error
}

// Processor contains vehicle business logic, injected with Provider and Administrator.
type Processor struct {
	log logrus.FieldLogger
	p   Provider
	a   Administrator
}

func NewProcessor(log logrus.FieldLogger, p Provider, a Administrator) *Processor {
	return &Processor{log: log, p: p, a: a}
}

// GetByID fetches a vehicle by ID (only non-deleted rows).
func (pr *Processor) GetByID(id string) (Model, error) {
	// First check if it exists at all (including deleted) to distinguish 404 vs 410.
	m, err := pr.a.GetByIDIncludingDeleted(id)
	if err != nil {
		if errors.Is(err, ErrNotFound) || errors.Is(err, gorm.ErrRecordNotFound) {
			return Model{}, server.ErrNotFound
		}
		return Model{}, err
	}
	// Row is soft-deleted and past its purge window → 410 Gone.
	if m.DeletedAt() != nil && IsPurgeable(m.PurgeAfter()) {
		return Model{}, server.ErrGone
	}
	// Row is soft-deleted but still in recovery window → 404 (treat as not found).
	if m.DeletedAt() != nil {
		return Model{}, server.ErrNotFound
	}
	return m, nil
}

// ListByFleet returns a page of vehicles for a fleet.
func (pr *Processor) ListByFleet(fleetID string, page server.Page) ([]Model, int, error) {
	return pr.p.ListByFleet(fleetID, page)
}

// Create inserts a new vehicle.
func (pr *Processor) Create(m Model) (Model, error) {
	return pr.a.Insert(m)
}

// Update applies a partial update to an existing vehicle.
func (pr *Processor) Update(id string, apply func(Model) Model) (Model, error) {
	m, err := pr.GetByID(id)
	if err != nil {
		return Model{}, err
	}
	updated := apply(m)
	return pr.a.Update(updated)
}

// SoftDelete marks a vehicle as deleted (sets deleted_at + purge_after).
func (pr *Processor) SoftDelete(id string) error {
	_, err := pr.a.SoftDelete(id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return server.ErrNotFound
	}
	return err
}

// Restore un-deletes a vehicle only if it is still within the recovery window.
// Returns server.ErrGone (410) if the purge window has expired.
func (pr *Processor) Restore(id string) (Model, error) {
	m, err := pr.a.GetByIDIncludingDeleted(id)
	if err != nil {
		if errors.Is(err, ErrNotFound) || errors.Is(err, gorm.ErrRecordNotFound) {
			return Model{}, server.ErrNotFound
		}
		return Model{}, err
	}
	if IsPurgeable(m.PurgeAfter()) {
		return Model{}, server.ErrGone
	}
	return pr.a.RestoreRow(id)
}

// SetPrimaryImage mirrors a media_id into vehicles.primary_image_media_id.
func (pr *Processor) SetPrimaryImage(id, mediaID string) (Model, error) {
	_, err := pr.GetByID(id)
	if err != nil {
		return Model{}, err
	}
	return pr.a.SetPrimaryImage(id, mediaID)
}

// GetByIDIncludingDeleted fetches a vehicle regardless of soft-delete status.
// Used by restore handler to resolve fleetID for authz before acting.
func (pr *Processor) GetByIDIncludingDeleted(id string) (Model, error) {
	m, err := pr.a.GetByIDIncludingDeleted(id)
	if err != nil {
		if errors.Is(err, ErrNotFound) || errors.Is(err, gorm.ErrRecordNotFound) {
			return Model{}, server.ErrNotFound
		}
		return Model{}, err
	}
	return m, nil
}
