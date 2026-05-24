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

// ActivityRecorder appends an activity event on the supplied transaction handle
// (design §8.2). Injected so the vehicle package never imports the activity
// package directly (would create an import cycle). Satisfied by activity.Record.
type ActivityRecorder func(tx *gorm.DB, actorUserID, eventType, fleetID string, vehicleID *string, payload map[string]any) error

// EventEmitter enqueues a domain event in the transactional outbox on the
// supplied tx (design A8). Injected to avoid an import cycle. Satisfied by an
// adapter over events.EmitVehicleCreated.
type EventEmitter func(tx *gorm.DB, fleetID, actorID, traceID, vehicleID string) error

// Processor contains vehicle business logic, injected with Provider and Administrator.
type Processor struct {
	log    logrus.FieldLogger
	p      Provider
	a      Administrator
	record ActivityRecorder
	emit   EventEmitter
}

func NewProcessor(log logrus.FieldLogger, p Provider, a Administrator) *Processor {
	return &Processor{log: log, p: p, a: a}
}

// WithActivityRecorder injects the activity recorder used on vehicle creation.
func (pr *Processor) WithActivityRecorder(rec ActivityRecorder) *Processor {
	pr.record = rec
	return pr
}

// WithEventEmitter injects the outbox emitter used on vehicle creation (A8).
func (pr *Processor) WithEventEmitter(emit EventEmitter) *Processor {
	pr.emit = emit
	return pr
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

// Create inserts a new vehicle and, in the SAME transaction, appends a
// vehicle.created activity event and enqueues a vehicle.created outbox event
// (design §8.2, A8). Any hook error rolls the insert back.
func (pr *Processor) Create(m Model, actorUserID, traceID string) (Model, error) {
	var hooks []TxHook
	if pr.record != nil {
		hooks = append(hooks, func(tx *gorm.DB, created Model) error {
			vid := created.ID()
			return pr.record(tx, actorUserID, "vehicle.created", created.FleetID(), &vid, map[string]any{
				"vehicle_id": created.ID(),
				"nickname":   created.Nickname(),
				"make":       created.Make(),
				"model":      created.Model(),
			})
		})
	}
	if pr.emit != nil {
		hooks = append(hooks, func(tx *gorm.DB, created Model) error {
			return pr.emit(tx, created.FleetID(), actorUserID, traceID, created.ID())
		})
	}
	return pr.a.InsertWithHooks(m, hooks...)
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
