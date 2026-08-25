package maintenancerecord

import (
	"time"

	"gorm.io/gorm"
)

// Entity maps to fleet.maintenance_records (PRD §6, design §8.2).
type Entity struct {
	ID               string    `gorm:"type:uuid;primaryKey"`
	VehicleID        string    `gorm:"type:uuid;not null;index"`
	CategoryID       string    `gorm:"type:uuid;not null"`
	Description      string    `gorm:"type:varchar(200)"`
	PerformedAt      time.Time `gorm:"not null"`
	Mileage          int
	Cost             float64
	Vendor           string
	Notes            string
	CreatedByUserID  string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        *time.Time `gorm:"index"`
	PurgeOperationID *string    `gorm:"type:uuid;index"`
}

func (Entity) TableName() string { return "fleet.maintenance_records" }

// DocumentEntity maps to fleet.maintenance_record_documents — attached media
// references for a maintenance record (receipts, photos, etc.).
type DocumentEntity struct {
	ID                  string     `gorm:"type:uuid;primaryKey"`
	MaintenanceRecordID string     `gorm:"type:uuid;not null;index"`
	MediaID             string     `gorm:"type:uuid;not null"`
	DeletedAt           *time.Time `gorm:"index"`
	PurgeOperationID    *string    `gorm:"type:uuid;index"`
}

func (DocumentEntity) TableName() string { return "fleet.maintenance_record_documents" }

func Migration(db *gorm.DB) error {
	if err := db.AutoMigrate(&Entity{}, &DocumentEntity{}); err != nil {
		return err
	}
	return ApplyPartialIndexes(db)
}

// ApplyPartialIndexes makes (maintenance_record_id, media_id) unique among
// LIVE document rows, following the same pattern as mediavariant, membership
// and invite.
//
// It cannot be a `gorm:"uniqueIndex"` struct tag: GORM has no way to express a
// WHERE clause, and a plain unique index is the wrong invariant here. A
// detached (soft-deleted) row would keep occupying the slot, so re-attaching a
// media object a user had previously removed would fail forever against a row
// no reader can see. Detach-then-reattach is an obvious user action and is the
// exact flow the detach endpoint creates.
//
// De-duplication runs first and is not optional. Existing data can already
// violate the index — nothing stopped the create path from being handed the
// same id twice in documentMediaIds — and CREATE UNIQUE INDEX against such a
// row fails, which would take the service down at startup rather than at the
// call site.
func ApplyPartialIndexes(db *gorm.DB) error {
	if err := dedupeLiveDocuments(db); err != nil {
		return err
	}
	create := `CREATE UNIQUE INDEX IF NOT EXISTS ux_maintenance_record_documents_record_media
	 ON fleet.maintenance_record_documents (maintenance_record_id, media_id) WHERE deleted_at IS NULL`
	if db.Name() == "sqlite" {
		// SQLite puts the schema on the INDEX name, not the table name.
		create = `CREATE UNIQUE INDEX IF NOT EXISTS fleet.ux_maintenance_record_documents_record_media
		 ON maintenance_record_documents (maintenance_record_id, media_id) WHERE deleted_at IS NULL`
	}
	return db.Exec(create).Error
}

// dedupeLiveDocuments soft-deletes all but the lowest-id live row in each
// (maintenance_record_id, media_id) group. Idempotent: a second run matches
// nothing. CURRENT_TIMESTAMP rather than NOW() so the one statement is valid
// under both Postgres and the sqlite test fixture.
//
// "Lowest id" is expressed as a correlated EXISTS over `o.id < d.id` rather
// than the more obvious `id NOT IN (SELECT MIN(id) ... GROUP BY ...)`. Postgres
// types `id` as uuid and has no min() aggregate for uuid, so the aggregate form
// fails with SQLSTATE 42883 — and because Migration failure is fatal at
// startup, that took fleet-service into a crash loop. The comparison operator
// `<` is defined for uuid, and for the TEXT columns the sqlite fixture uses, so
// this form is correct under both. See entity_postgres_test.go.
func dedupeLiveDocuments(db *gorm.DB) error {
	return db.Exec(`UPDATE fleet.maintenance_record_documents AS d
	 SET deleted_at = CURRENT_TIMESTAMP
	 WHERE d.deleted_at IS NULL
	   AND EXISTS (
	     SELECT 1 FROM fleet.maintenance_record_documents AS o
	     WHERE o.deleted_at IS NULL
	       AND o.maintenance_record_id = d.maintenance_record_id
	       AND o.media_id = d.media_id
	       AND o.id < d.id
	   )`).Error
}

// Make converts an Entity (and its document rows) to a Model.
func Make(e Entity, docs []DocumentEntity) Model {
	ids := make([]string, 0, len(docs))
	for _, d := range docs {
		ids = append(ids, d.MediaID)
	}
	return Model{
		id:               e.ID,
		vehicleID:        e.VehicleID,
		categoryID:       e.CategoryID,
		description:      e.Description,
		performedAt:      e.PerformedAt,
		mileage:          e.Mileage,
		cost:             e.Cost,
		vendor:           e.Vendor,
		notes:            e.Notes,
		createdByUserID:  e.CreatedByUserID,
		createdAt:        e.CreatedAt,
		updatedAt:        e.UpdatedAt,
		deletedAt:        e.DeletedAt,
		documentMediaIDs: ids,
	}
}

// ToEntity converts a Model to an Entity for persistence (documents handled separately).
func (m Model) ToEntity() Entity {
	return Entity{
		ID:              m.id,
		VehicleID:       m.vehicleID,
		CategoryID:      m.categoryID,
		Description:     m.description,
		PerformedAt:     m.performedAt,
		Mileage:         m.mileage,
		Cost:            m.cost,
		Vendor:          m.vendor,
		Notes:           m.notes,
		CreatedByUserID: m.createdByUserID,
		DeletedAt:       m.deletedAt,
	}
}
