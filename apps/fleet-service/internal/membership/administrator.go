package membership

import (
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/fleet"
)

// Administrator is the write interface for membership data access.
// It also owns the cross-domain fleet onboarding transaction.
type Administrator interface {
	Insert(Model) (Model, error)
	// Delete removes a membership by id WITHOUT recording activity. Retained
	// because it is part of the existing contract; new call sites use Remove,
	// which is transactional and auditable.
	Delete(id string) error
	// UpdateRole writes a new role and appends member.role_changed in the SAME
	// transaction (FR-5.2).
	UpdateRole(m Model, role, actorUserID string) (Model, error)
	// Remove hard-deletes a membership and appends member.removed (or
	// member.left when the actor is the target) in the SAME transaction.
	Remove(m Model, actorUserID string) error
	// CreateFleetWithOwner creates a fleet + owner membership in one transaction.
	// Implements fleet.OnboardingAdmin.
	CreateFleetWithOwner(db *gorm.DB, fleetName, userID string) (fleet.Model, error)
}

// ActivityRecorder appends an activity event on the supplied tx (design §8.2).
// Injected as a function value so the membership package never imports the
// activity package. Satisfied by activity.Record.
type ActivityRecorder func(tx *gorm.DB, actorUserID, eventType, fleetID string, vehicleID *string, payload map[string]any) error

type dbAdministrator struct {
	db     *gorm.DB
	record ActivityRecorder
}

// NewAdministrator returns an Administrator backed by the given database.
// It returns the concrete type so WithActivityRecorder can be chained, matching
// invite.NewAdministrator.
func NewAdministrator(db *gorm.DB) *dbAdministrator { return &dbAdministrator{db: db} }

// WithActivityRecorder injects the recorder run inside UpdateRole and Remove.
// Leaving it nil is supported: tests and the onboarding path construct the
// administrator bare, and the call sites nil-check before recording.
func (a *dbAdministrator) WithActivityRecorder(rec ActivityRecorder) *dbAdministrator {
	a.record = rec
	return a
}

func (a *dbAdministrator) Insert(m Model) (Model, error) {
	e := m.ToEntity()
	if err := a.db.Create(&e).Error; err != nil {
		return Model{}, err
	}
	return Make(e), nil
}

func (a *dbAdministrator) Delete(id string) error {
	return a.db.Delete(&Entity{}, "id = ?", id).Error
}

// UpdateRole persists the new role and appends member.role_changed in the same
// transaction (FR-5.2).
//
// Update("role", …) rather than Save(&entity): a full-row save would rewrite
// created_at and status from a model that was read OUTSIDE this transaction.
// Narrow update, narrow window.
func (a *dbAdministrator) UpdateRole(m Model, role, actorUserID string) (Model, error) {
	updated := m.WithRole(role)
	err := a.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&Entity{}).Where("id = ?", m.ID()).Update("role", role)
		if res.Error != nil {
			return res.Error
		}
		// m was read OUTSIDE this transaction, so the row can have been deleted
		// in between. Without this check the update matches nothing, the
		// transaction still commits, and member.role_changed is recorded for a
		// membership that no longer exists — while the handler returns 200 with
		// a model computed in memory and never read back.
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		if a.record == nil {
			return nil
		}
		// from_role comes off the pre-change model, so the entry is
		// self-contained: the feed never has to replay history.
		return a.record(tx, actorUserID, "member.role_changed", m.FleetID(), nil, map[string]any{
			"target_user_id": m.UserID(),
			"from_role":      m.Role(),
			"to_role":        role,
		})
	})
	if err != nil {
		return Model{}, err
	}
	return updated, nil
}

// Remove hard-deletes the membership and appends the departure event in the
// same transaction.
//
// Memberships carry no deleted_at (see entity.go), so this activity row is the
// ONLY record that the membership ever existed. That is why the append is
// transactional rather than a best-effort follow-up write.
//
// The actor == target predicate that picks member.left over member.removed is
// the same one that relaxes the DELETE authorization guard, so the audit trail
// and the authorization decision cannot disagree.
func (a *dbAdministrator) Remove(m Model, actorUserID string) error {
	return a.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Delete(&Entity{}, "id = ?", m.ID())
		if res.Error != nil {
			return res.Error
		}
		// See UpdateRole: m was read outside this transaction. Recording a
		// departure for a row someone else already deleted would put a second,
		// fictitious event in the only record that the membership ever existed.
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		if a.record == nil {
			return nil
		}
		if actorUserID == m.UserID() {
			return a.record(tx, actorUserID, "member.left", m.FleetID(), nil, map[string]any{
				"role": m.Role(),
			})
		}
		return a.record(tx, actorUserID, "member.removed", m.FleetID(), nil, map[string]any{
			"target_user_id": m.UserID(),
			"role":           m.Role(),
		})
	})
}

// CreateFleetWithOwner wraps fleet insert + owner membership insert in one
// database transaction (FR-FLEET-1, design §8.2 POST /fleets).
func (a *dbAdministrator) CreateFleetWithOwner(db *gorm.DB, fleetName, userID string) (fleet.Model, error) {
	var created fleet.Model
	err := db.Transaction(func(tx *gorm.DB) error {
		f, err := fleet.NewBuilder().SetName(fleetName).SetCreatedByUserID(userID).Build()
		if err != nil {
			return err
		}
		fe := f.ToEntity()
		if err := tx.Create(&fe).Error; err != nil {
			return err
		}
		created = fleet.Make(fe)

		me := NewBuilder().SetFleetID(created.ID()).SetUserID(userID).SetRole("owner").Build().ToEntity()
		return tx.Create(&me).Error
	})
	if err != nil {
		return fleet.Model{}, err
	}
	return created, nil
}
