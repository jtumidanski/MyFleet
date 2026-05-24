package invite

import (
	"time"

	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/membership"
	"github.com/jtumidanski/myfleet/packages/shared-go/events"
)

// Administrator is the write interface for invite data access.
type Administrator interface {
	Insert(Model) (Model, error)
	Delete(id string) error
	// Accept stamps accepted_at and creates an active membership in one transaction.
	// Emits a member.invited event via the injected Producer.
	Accept(inv Model, userID string, producer events.Producer) (Model, error)
}

type dbAdministrator struct{ db *gorm.DB }

// NewAdministrator returns an Administrator backed by the given database.
func NewAdministrator(db *gorm.DB) Administrator { return &dbAdministrator{db: db} }

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

// Accept runs the accept workflow in a single transaction:
//  1. Stamp accepted_at on the invite row.
//  2. Create an active membership for the accepting user.
//  3. Enqueue a member.invited event in the outbox.
func (a *dbAdministrator) Accept(inv Model, userID string, producer events.Producer) (Model, error) {
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

		// 3. Enqueue member.invited event in the transactional outbox
		env := events.Envelope{
			EventID:     inv.ID(),
			Type:        "member.invited",
			Version:     1,
			OccurredAt:  now,
			FleetID:     inv.FleetID(),
			ActorUserID: userID,
		}
		return events.Enqueue(tx, env)
	})
	if err != nil {
		return Model{}, err
	}
	return updated, nil
}
