package maintenancerecord

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Administrator is the write interface for maintenance record data access.
// All mutations (insert, update, soft-delete) go here.
type Administrator interface {
	Insert(Model) (Model, error)
	Update(Model) (Model, error)
	SoftDelete(id string) error
	// InsertTx inserts a record (and its document rows) using the supplied
	// transaction handle, so cross-domain orchestrations (completion flow,
	// design §10.3) can wrap it in a single db.Transaction.
	InsertTx(tx *gorm.DB, m Model) (Model, error)
}

type dbAdministrator struct{ db *gorm.DB }

// NewAdministrator returns an Administrator backed by the given database.
func NewAdministrator(db *gorm.DB) Administrator { return &dbAdministrator{db: db} }

func (a *dbAdministrator) Insert(m Model) (Model, error) {
	var out Model
	err := a.db.Transaction(func(tx *gorm.DB) error {
		var err error
		out, err = a.InsertTx(tx, m)
		return err
	})
	return out, err
}

func (a *dbAdministrator) InsertTx(tx *gorm.DB, m Model) (Model, error) {
	e := m.ToEntity()
	if err := tx.Create(&e).Error; err != nil {
		return Model{}, err
	}
	docs := make([]DocumentEntity, 0, len(m.DocumentMediaIDs()))
	for _, mediaID := range m.DocumentMediaIDs() {
		d := DocumentEntity{ID: uuid.NewString(), MaintenanceRecordID: e.ID, MediaID: mediaID}
		if err := tx.Create(&d).Error; err != nil {
			return Model{}, err
		}
		docs = append(docs, d)
	}
	return Make(e, docs), nil
}

func (a *dbAdministrator) Update(m Model) (Model, error) {
	e := m.ToEntity()
	if err := a.db.Model(&Entity{}).Where("id = ? AND deleted_at IS NULL", e.ID).
		Updates(map[string]any{
			// This map is an explicit allow-list, so a field missing from it is
			// silently dropped — and because the return below is built from the
			// in-memory entity rather than a re-read, the response still showed
			// the new value. That is what made an edited category look saved
			// until the next fetch.
			"category_id":  e.CategoryID,
			"performed_at": e.PerformedAt,
			"mileage":      e.Mileage,
			"cost":         e.Cost,
			"vendor":       e.Vendor,
			"notes":        e.Notes,
			"description":  e.Description,
		}).Error; err != nil {
		return Model{}, err
	}
	// Re-read rather than returning Make(e, docs) from the entity we just built.
	// Two reasons, both bugs that were live before this:
	//   1. ToEntity carries no CreatedAt, so the echoed model had a zero time
	//      and every PATCH response advertised "createdAt":"0001-01-01T00:00:00Z".
	//   2. The Updates map above is an allow-list. A column missing from it is
	//      silently not written, and echoing the in-memory entity made the
	//      response agree with the caller anyway — which is exactly how an
	//      edited category looked saved until the next fetch. Returning stored
	//      state means the response can no longer disagree with the row.
	var stored Entity
	if err := a.db.Where("id = ? AND deleted_at IS NULL", e.ID).First(&stored).Error; err != nil {
		return Model{}, err
	}
	var docs []DocumentEntity
	if err := a.db.Where("maintenance_record_id = ?", e.ID).Find(&docs).Error; err != nil {
		return Model{}, err
	}
	return Make(stored, docs), nil
}

// SoftDelete stamps deleted_at on the maintenance record.
func (a *dbAdministrator) SoftDelete(id string) error {
	res := a.db.Model(&Entity{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Update("deleted_at", time.Now().UTC())
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
