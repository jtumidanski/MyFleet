package fleet

import (
	"time"

	"gorm.io/gorm"
)

// Entity maps to fleet.fleets (PRD §6).
type Entity struct {
	ID              string `gorm:"type:uuid;primaryKey"`
	Name            string `gorm:"not null"`
	CreatedByUserID string `gorm:"not null"`
	// `<-:create` protects the COLUMN from db.Save's UPDATE-every-column write;
	// assigning it in ToEntity() protects the MODEL that Make(e) returns after
	// the write. Both layers are deliberate (task-006 design §4).
	CreatedAt        time.Time `gorm:"<-:create"`
	UpdatedAt        time.Time
	DeletedAt        gorm.DeletedAt `gorm:"index"`
	PurgeOperationID *string        `gorm:"type:uuid;index"`
}

func (Entity) TableName() string { return "fleet.fleets" }

func Migration(db *gorm.DB) error { return db.AutoMigrate(&Entity{}) }

// Make converts an Entity to a Model.
func Make(e Entity) Model {
	return Model{
		id:               e.ID,
		name:             e.Name,
		createdByUserID:  e.CreatedByUserID,
		createdAt:        e.CreatedAt,
		updatedAt:        e.UpdatedAt,
		deletedAt:        fromGormDeletedAt(e.DeletedAt),
		purgeOperationID: e.PurgeOperationID,
	}
}

// fromGormDeletedAt / toGormDeletedAt keep gorm.DeletedAt out of the domain
// layer: Model carries a *time.Time, matching vehicle.Model.deletedAt, and the
// conversion lives in entity.go, already the only file here that knows GORM.
func fromGormDeletedAt(d gorm.DeletedAt) *time.Time {
	if !d.Valid {
		return nil
	}
	t := d.Time
	return &t
}

func toGormDeletedAt(t *time.Time) gorm.DeletedAt {
	if t == nil {
		return gorm.DeletedAt{}
	}
	return gorm.DeletedAt{Time: *t, Valid: true}
}

// ToEntity converts a Model to an Entity for persistence.
func (m Model) ToEntity() Entity {
	return Entity{
		ID:               m.id,
		Name:             m.name,
		CreatedByUserID:  m.createdByUserID,
		CreatedAt:        m.createdAt,
		DeletedAt:        toGormDeletedAt(m.deletedAt),
		PurgeOperationID: m.purgeOperationID,
	}
}
