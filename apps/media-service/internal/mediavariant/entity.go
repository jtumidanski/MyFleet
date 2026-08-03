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
	MediaObjectID string `gorm:"type:uuid;not null;index"`
	Variant       string `gorm:"not null"`
	ObjectKey     string `gorm:"not null"`
	Width         int
	Height        int
	ContentType   string
	CreatedAt     time.Time
	// Soft-delete and purge attribution arrived with the admin console. They
	// interact with the (media_object_id, variant) uniqueness main added: a
	// PLAIN unique index would let a soft-deleted row keep occupying the slot,
	// so Upsert's ON CONFLICT would silently write into a row a purge owns —
	// invisible to readers, and hard-deleted by the reaper along with the bytes
	// it now points at. The index is therefore created as a PARTIAL one in
	// ApplyPartialIndexes below rather than by a struct tag, which cannot
	// express a WHERE clause.
	DeletedAt        *time.Time `gorm:"index"`
	PurgeOperationID *string    `gorm:"type:uuid;index"`
}

func (Entity) TableName() string { return "media.media_variants" }

func Migration(db *gorm.DB) error {
	if err := db.AutoMigrate(&Entity{}); err != nil {
		return err
	}
	return ApplyPartialIndexes(db)
}

// ApplyPartialIndexes creates the (media_object_id, variant) uniqueness as a
// PARTIAL index, following the same pattern as membership, invite and dashboard.
//
// It cannot be a struct tag: GORM's uniqueIndex has no way to express a WHERE
// clause, and a plain unique index is wrong here. A variant soft-deleted by an
// admin purge would keep occupying the slot, so a regeneration would conflict
// with a row that is invisible to every reader and owned by a purge operation —
// Upsert would quietly update it, the reader would still see nothing, and the
// reaper would later hard-delete the row along with the bytes it had just been
// pointed at.
//
// Upsert's ON CONFLICT names the same predicate so Postgres can infer this
// index rather than failing to find a matching arbiter.
func ApplyPartialIndexes(db *gorm.DB) error {
	create := `CREATE UNIQUE INDEX IF NOT EXISTS ux_media_variants_object_variant
	 ON media.media_variants (media_object_id, variant) WHERE deleted_at IS NULL`
	if db.Name() == "sqlite" {
		create = `CREATE UNIQUE INDEX IF NOT EXISTS media.ux_media_variants_object_variant
		 ON media_variants (media_object_id, variant) WHERE deleted_at IS NULL`
	}
	return db.Exec(create).Error
}

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
