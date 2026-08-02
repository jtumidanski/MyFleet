package invite

import (
	"time"

	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/membership"
)

// Administrator is the write interface for invite data access.
type Administrator interface {
	// Insert creates the invite row and enqueues an invite.created outbox event
	// in the SAME transaction (FR-EVT-1, FR-EVT-2). A failed enqueue rolls the
	// invite back — an invite nobody is told about is worse than no invite.
	Insert(m Model, traceID string) (Model, error)
	// Resend rotates the token and resets expires_at, enqueuing a fresh
	// invite.created in the SAME transaction (FR-RSND-2). now is passed in
	// rather than read internally so the returned Model's updated_at is exactly
	// the value written — which is what the resend cooldown then reads.
	Resend(inv Model, newToken string, expiresAt, now time.Time, traceID string) (Model, error)
	Delete(id string) error
	// Accept stamps accepted_at and creates an active membership in one transaction.
	// Appends a member.invited activity event and enqueues a member.invited
	// outbox event in the SAME transaction.
	Accept(inv Model, userID, traceID string) (Model, error)
}

// ActivityRecorder appends an activity event on the supplied tx (design §8.2).
// Injected to keep the invite package decoupled. Satisfied by activity.Record.
type ActivityRecorder func(tx *gorm.DB, actorUserID, eventType, fleetID string, vehicleID *string, payload map[string]any) error

// InvitedEmitter enqueues a member.invited event in the outbox on the supplied
// tx (design A8). Injected to avoid coupling. Satisfied by events.EmitMemberInvited.
type InvitedEmitter func(tx *gorm.DB, fleetID, actorID, traceID, inviteID, email, role string) error

// CreatedEmitter enqueues an invite.created event in the outbox on the supplied
// tx. Injected to avoid coupling, exactly like InvitedEmitter. Satisfied by
// events.EmitInviteCreated. Deliberately a SEPARATE seam from InvitedEmitter:
// member.invited fires on ACCEPT, invite.created fires on CREATE/RESEND.
type CreatedEmitter func(tx *gorm.DB, fleetID, actorID, traceID, inviteID, email, role string) error

type dbAdministrator struct {
	db          *gorm.DB
	record      ActivityRecorder
	emit        InvitedEmitter
	emitCreated CreatedEmitter
}

// NewAdministrator returns an Administrator backed by the given database.
func NewAdministrator(db *gorm.DB) *dbAdministrator { return &dbAdministrator{db: db} }

// WithActivityRecorder injects the activity recorder run on invite acceptance.
func (a *dbAdministrator) WithActivityRecorder(rec ActivityRecorder) *dbAdministrator {
	a.record = rec
	return a
}

// WithEmitter injects the member.invited outbox emitter (A8).
func (a *dbAdministrator) WithEmitter(emit InvitedEmitter) *dbAdministrator {
	a.emit = emit
	return a
}

// WithCreatedEmitter injects the invite.created outbox emitter (FR-EVT-1).
func (a *dbAdministrator) WithCreatedEmitter(emit CreatedEmitter) *dbAdministrator {
	a.emitCreated = emit
	return a
}

func (a *dbAdministrator) Insert(m Model, traceID string) (Model, error) {
	e := m.ToEntity()
	err := a.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&e).Error; err != nil {
			return err
		}
		// FATAL: a failed enqueue rolls back the invite row. An invite the
		// invitee is never told about is undeliverable by any in-product path.
		if a.emitCreated != nil {
			return a.emitCreated(tx, e.FleetID, e.InvitedByUserID, traceID, e.ID, e.Email, e.Role)
		}
		return nil
	})
	if err != nil {
		return Model{}, err
	}
	return Make(e), nil
}

func (a *dbAdministrator) Resend(inv Model, newToken string, expiresAt, now time.Time, traceID string) (Model, error) {
	var updated Model
	err := a.db.Transaction(func(tx *gorm.DB) error {
		// updated_at is set explicitly rather than left to GORM so the value we
		// return is provably the value persisted.
		res := tx.Model(&Entity{}).Where("id = ?", inv.ID()).Updates(map[string]any{
			"token":      newToken,
			"expires_at": expiresAt,
			"updated_at": now,
		})
		if res.Error != nil {
			return res.Error
		}
		updated = Make(Entity{
			ID:              inv.ID(),
			FleetID:         inv.FleetID(),
			Email:           inv.Email(),
			Role:            inv.Role(),
			Token:           newToken,
			ExpiresAt:       expiresAt,
			AcceptedAt:      inv.AcceptedAt(),
			InvitedByUserID: inv.InvitedByUserID(),
			UpdatedAt:       now,
		})
		// FATAL: a failed enqueue rolls the rotation back. Rotating without
		// emitting would kill the previously mailed link and send nothing new.
		if a.emitCreated != nil {
			return a.emitCreated(tx, inv.FleetID(), inv.InvitedByUserID(), traceID, inv.ID(), inv.Email(), inv.Role())
		}
		return nil
	})
	if err != nil {
		return Model{}, err
	}
	return updated, nil
}

func (a *dbAdministrator) Delete(id string) error {
	return a.db.Delete(&Entity{}, "id = ?", id).Error
}

// Accept runs the accept workflow in a single transaction:
//  1. Stamp accepted_at on the invite row.
//  2. Create an active membership for the accepting user.
//  3. Enqueue a member.invited event in the outbox.
func (a *dbAdministrator) Accept(inv Model, userID, traceID string) (Model, error) {
	now := time.Now()
	var updated Model
	err := a.db.Transaction(func(tx *gorm.DB) error {
		// 1. Stamp accepted_at
		if err := tx.Model(&Entity{}).Where("id = ?", inv.ID()).Update("accepted_at", &now).Error; err != nil {
			return err
		}
		updated = Make(Entity{
			ID:              inv.ID(),
			FleetID:         inv.FleetID(),
			Email:           inv.Email(),
			Role:            inv.Role(),
			Token:           inv.Token(),
			ExpiresAt:       inv.ExpiresAt(),
			AcceptedAt:      &now,
			InvitedByUserID: inv.InvitedByUserID(),
		})

		// 2. Create active membership
		me := membership.NewBuilder().
			SetFleetID(inv.FleetID()).
			SetUserID(userID).
			SetRole(inv.Role()).
			SetStatus("active").
			Build().
			ToEntity()
		if err := tx.Create(&me).Error; err != nil {
			return err
		}

		// 3. Append a member.invited activity event in the SAME tx (§8.2).
		if a.record != nil {
			if err := a.record(tx, userID, "member.invited", inv.FleetID(), nil, map[string]any{
				"invite_id": inv.ID(),
				"email":     inv.Email(),
				"role":      inv.Role(),
			}); err != nil {
				return err
			}
		}

		// 4. Enqueue member.invited event in the transactional outbox (A8).
		// FATAL: a failed enqueue rolls back the whole accept transaction.
		if a.emit != nil {
			if err := a.emit(tx, inv.FleetID(), userID, traceID, inv.ID(), inv.Email(), inv.Role()); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Model{}, err
	}
	return updated, nil
}
