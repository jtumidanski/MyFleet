package mediavariant

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Administrator is the write interface for media-variant data access.
type Administrator interface {
	// ReplaceForMediaObject deletes any existing variants for the media object
	// and inserts the given set, in one transaction. This keeps variant
	// generation idempotent: a re-delivered media.uploaded event regenerates
	// and overwrites the variants rather than duplicating them.
	ReplaceForMediaObject(mediaObjectID string, variants []Model) error

	// Upsert writes a single variant additively, leaving every other variant of
	// the same media object untouched. It keys on the unique
	// (media_object_id, variant) index, so two processes racing to generate the
	// same variant leave exactly one row rather than two.
	//
	// This is the ONLY write lazy generation may use. ReplaceForMediaObject
	// deletes every row for the media object before inserting, so calling it
	// with a single card model would destroy that object's thumbnail and
	// display.
	Upsert(m Model) error
}

type dbAdministrator struct{ db *gorm.DB }

// NewAdministrator returns an Administrator backed by the given database.
func NewAdministrator(db *gorm.DB) Administrator { return &dbAdministrator{db: db} }

func (a *dbAdministrator) ReplaceForMediaObject(mediaObjectID string, variants []Model) error {
	return a.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("media_object_id = ?", mediaObjectID).Delete(&Entity{}).Error; err != nil {
			return err
		}
		for _, m := range variants {
			e := m.ToEntity()
			if err := tx.Create(&e).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (a *dbAdministrator) Upsert(m Model) error {
	e := m.ToEntity()
	// DoUpdates names its columns explicitly. created_at is deliberately absent
	// and UpdateAll is deliberately not used: either would rewrite the row's age
	// on every regeneration, which is the column-wipe defect task-006 existed to
	// eliminate. entityguard will not catch it here — it recognises .Save( call
	// sites only — so the explicit list is the whole guard, alongside
	// TestUpsert_preservesCreatedAt.
	return a.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "media_object_id"}, {Name: "variant"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"object_key", "width", "height", "content_type",
		}),
	}).Create(&e).Error
}
