package mediavariant

import (
	"time"

	"gorm.io/gorm"
)

// Entity maps to media.media_variants (PRD §6). (media_object_id, variant) is
// unique: a media object has at most one rendition of each kind. The constraint
// is what makes Upsert's additive write safe against two processes racing to
// generate the same variant — which a rolling update can transiently produce,
// since each pod has its own in-process single-flight map.
//
// The plain index on MediaObjectID is kept even though the composite index
// leads with the same column: AutoMigrate never drops indexes, so removing the
// tag would leave an orphan in deployed databases while changing nothing.
type Entity struct {
	ID            string `gorm:"type:uuid;primaryKey"`
	MediaObjectID string `gorm:"type:uuid;not null;index;uniqueIndex:ux_media_variants_object_variant"`
	Variant       string `gorm:"not null;uniqueIndex:ux_media_variants_object_variant"`
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
