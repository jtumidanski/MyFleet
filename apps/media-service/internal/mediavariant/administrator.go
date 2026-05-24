package mediavariant

import "gorm.io/gorm"

// Administrator is the write interface for media-variant data access.
type Administrator interface {
	// ReplaceForMediaObject deletes any existing variants for the media object
	// and inserts the given set, in one transaction. This keeps variant
	// generation idempotent: a re-delivered media.uploaded event regenerates
	// and overwrites the variants rather than duplicating them.
	ReplaceForMediaObject(mediaObjectID string, variants []Model) error
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
