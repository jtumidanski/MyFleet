package mediavariant

import (
	"time"

	"gorm.io/gorm"
)

// Entity maps to media.media_variants (PRD §6).
type Entity struct {
	ID            string `gorm:"type:uuid;primaryKey"`
	MediaObjectID string `gorm:"type:uuid;not null;index"`
	Variant       string `gorm:"not null"`
	ObjectKey     string `gorm:"not null"`
	Width         int
	Height        int
	ContentType   string
	CreatedAt     time.Time
}

func (Entity) TableName() string { return "media.media_variants" }

func Migration(db *gorm.DB) error { return db.AutoMigrate(&Entity{}) }

// Make converts an Entity to a Model.
func Make(e Entity) Model {
	return Model{
		id:            e.ID,
		mediaObjectID: e.MediaObjectID,
		variant:       Variant(e.Variant),
		objectKey:     e.ObjectKey,
		width:         e.Width,
		height:        e.Height,
		contentType:   e.ContentType,
		createdAt:     e.CreatedAt,
	}
}

// ToEntity converts a Model to an Entity for persistence.
func (m Model) ToEntity() Entity {
	return Entity{
		ID:            m.id,
		MediaObjectID: m.mediaObjectID,
		Variant:       string(m.variant),
		ObjectKey:     m.objectKey,
		Width:         m.width,
		Height:        m.height,
		ContentType:   m.contentType,
		CreatedAt:     m.createdAt,
	}
}
