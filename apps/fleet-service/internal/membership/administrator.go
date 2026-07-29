package membership

import (
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/fleet"
)

// Administrator is the write interface for membership data access.
// It also owns the cross-domain fleet onboarding transaction.
type Administrator interface {
	Insert(Model) (Model, error)
	Delete(id string) error
	// CreateFleetWithOwner creates a fleet + owner membership in one transaction.
	// Implements fleet.OnboardingAdmin.
	CreateFleetWithOwner(db *gorm.DB, fleetName, userID string) (fleet.Model, error)
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
