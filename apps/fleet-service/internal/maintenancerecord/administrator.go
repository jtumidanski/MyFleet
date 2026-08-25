package maintenancerecord

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
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
	// AttachDocument adds one media reference to an existing record and
	// returns the re-read model. Deliberately NOT folded into Update: the
	// column allow-list in Update is the mechanism that keeps PATCH from
	// touching documents, and threading documents through it would put that
	// guarantee at the mercy of a future edit to the map (design D-1).
	AttachDocument(recordID, mediaID string) (Model, error)
	// DetachDocument soft-deletes one media reference from a record.
	DetachDocument(recordID, mediaID string) error
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
	if err := a.db.Where("maintenance_record_id = ? AND deleted_at IS NULL", e.ID).Find(&docs).Error; err != nil {
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

// AttachDocument attaches one media reference to a record, enforcing the
// per-record cap and idempotency inside a single transaction.
//
// The parent row is locked FOR UPDATE, and that is load-bearing, not
// decoration. Read-then-insert inside a transaction is NOT sufficient at
// Postgres's default READ COMMITTED isolation: two concurrent attaches to a
// record holding nine documents each count nine, neither sees the other's
// uncommitted insert, and the record ends with eleven. A transaction is not a
// lock. Locking the record serializes attaches to it, so the second
// transaction blocks until the first commits, then counts ten and gets its
// validation error. Locking the record (not the document table) also cleanly
// excludes a concurrent soft-delete of the record itself.
//
// The lock is dialect-guarded because this package's tests run on sqlite,
// which rejects FOR UPDATE as a syntax error. Skipping it there is safe:
// sqlite serializes writers at the database level, so the correctness the lock
// buys exists for free. Branching on the dialect for a gap like this is the
// same thing mediavariant.ApplyPartialIndexes already does.
func (a *dbAdministrator) AttachDocument(recordID, mediaID string) (Model, error) {
	var out Model
	err := a.db.Transaction(func(tx *gorm.DB) error {
		q := tx.Where("id = ? AND deleted_at IS NULL", recordID)
		if tx.Name() != "sqlite" {
			q = q.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		var locked Entity
		if err := q.First(&locked).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}

		// Idempotency BEFORE the cap check, deliberately: re-attaching an id a
		// full record already holds must succeed, or a retry of an attach that
		// actually landed would be punished with a 422.
		var already int64
		if err := tx.Model(&DocumentEntity{}).
			Where("maintenance_record_id = ? AND media_id = ? AND deleted_at IS NULL", recordID, mediaID).
			Count(&already).Error; err != nil {
			return err
		}
		if already == 0 {
			var live int64
			if err := tx.Model(&DocumentEntity{}).
				Where("maintenance_record_id = ? AND deleted_at IS NULL", recordID).
				Count(&live).Error; err != nil {
				return err
			}
			if live >= MaxDocuments {
				return server.ErrValidation
			}
			d := DocumentEntity{ID: uuid.NewString(), MaintenanceRecordID: recordID, MediaID: mediaID}
			if err := tx.Create(&d).Error; err != nil {
				return err
			}
		}

		// Re-read, for the same reason Update does: a model built from the
		// in-memory entity is how a response comes to disagree with the row.
		var stored Entity
		if err := tx.Where("id = ? AND deleted_at IS NULL", recordID).First(&stored).Error; err != nil {
			return err
		}
		var docs []DocumentEntity
		if err := tx.Where("maintenance_record_id = ? AND deleted_at IS NULL", recordID).
			Find(&docs).Error; err != nil {
			return err
		}
		out = Make(stored, docs)
		return nil
	})
	if err != nil {
		return Model{}, err
	}
	return out, nil
}

// DetachDocument stamps deleted_at on one live document row.
//
// Soft, not hard: every other delete in fleet-service is soft, and
// admin/visibility_document_test already asserts that a stamped row is
// invisible on both the detail and the list path.
//
// No transaction: it is one statement on one row. RowsAffected == 0 covers
// "never attached" and "already detached" with the same answer, which is also
// what keeps the response from confirming that a media id exists elsewhere.
//
// It deliberately does not verify the record exists — the resource layer's
// GetByID already did, and repeating it here would be a second round-trip for
// an invariant the handler holds.
func (a *dbAdministrator) DetachDocument(recordID, mediaID string) error {
	res := a.db.Model(&DocumentEntity{}).
		Where("maintenance_record_id = ? AND media_id = ? AND deleted_at IS NULL", recordID, mediaID).
		Update("deleted_at", time.Now().UTC())
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
