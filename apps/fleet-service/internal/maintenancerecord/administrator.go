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
			"performed_at": e.PerformedAt,
			"mileage":      e.Mileage,
			"cost":         e.Cost,
			"vendor":       e.Vendor,
			"notes":        e.Notes,
			"description":  e.Description,
		}).Error; err != nil {
		return Model{}, err
	}
	var docs []DocumentEntity
	if err := a.db.Where("maintenance_record_id = ? AND deleted_at IS NULL", e.ID).Find(&docs).Error; err != nil {
		return Model{}, err
	}
	return Make(e, docs), nil
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
