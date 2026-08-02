package mediaobject

import (
	"time"

	"gorm.io/gorm"
)

// Entity maps to media.media_objects (PRD §6).
type Entity struct {
	ID               string `gorm:"type:uuid;primaryKey"`
	FleetID          string `gorm:"type:uuid;not null;index"`
	UploadedByUserID string `gorm:"type:uuid;not null"`
	Bucket           string `gorm:"not null"`
	ObjectKey        string `gorm:"not null"`
	ContentType      string
	Size             int64
	OriginalFilename string
	Status           string `gorm:"not null"`
	// Standing protection, not a bug fix: ToEntity() already assigns this, so
	// nothing here was ever corrupted. That correctness is incidental — it holds
	// only while Model happens to carry createdAt — and two db.Save call sites
	// (Update and UpdateInTx) depend on it. `<-:create` makes it structural
	// (task-006 design §5.3).
	CreatedAt  time.Time  `gorm:"<-:create"`
	DeletedAt  *time.Time `gorm:"index"`
	PurgeAfter *time.Time
}

func (Entity) TableName() string { return "media.media_objects" }

func Migration(db *gorm.DB) error { return db.AutoMigrate(&Entity{}) }

// Make converts an Entity to a Model.
func Make(e Entity) Model {
	return Model{
		id:               e.ID,
		fleetID:          e.FleetID,
		uploadedByUserID: e.UploadedByUserID,
		bucket:           e.Bucket,
		objectKey:        e.ObjectKey,
		contentType:      e.ContentType,
		size:             e.Size,
		originalFilename: e.OriginalFilename,
		status:           Status(e.Status),
		createdAt:        e.CreatedAt,
		deletedAt:        e.DeletedAt,
		purgeAfter:       e.PurgeAfter,
	}
}

// ToEntity converts a Model to an Entity for persistence.
func (m Model) ToEntity() Entity {
	return Entity{
		ID:               m.id,
		FleetID:          m.fleetID,
		UploadedByUserID: m.uploadedByUserID,
		Bucket:           m.bucket,
		ObjectKey:        m.objectKey,
		ContentType:      m.contentType,
		Size:             m.size,
		OriginalFilename: m.originalFilename,
		Status:           string(m.status),
		CreatedAt:        m.createdAt,
		DeletedAt:        m.deletedAt,
		PurgeAfter:       m.purgeAfter,
	}
}
