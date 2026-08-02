package notification

import (
	"time"

	"gorm.io/gorm"
)

// Entity maps to notification.notifications (PRD §6).
//
// dedupe_key's uniqueness is a PARTIAL index (see ApplyPartialIndexes), not a
// tag: a soft-deleted notification must release its key, or the reminder
// safety-net and event redelivery are permanently suppressed for it (design F1).
//
// fleet_id is indexed because a fleet-scoped admin purge selects on it. It stays
// nullable: account-level notifications carry no fleet and are taken only by a
// system purge.
type Entity struct {
	ID               string `gorm:"type:uuid;primaryKey"`
	UserID           string `gorm:"type:uuid;not null;index"`
	Type             string `gorm:"not null"`
	Title            string `gorm:"not null"`
	Body             string
	DedupeKey        string `gorm:"not null;index:idx_notification_dedupe_key"`
	VehicleID        string `gorm:"type:uuid"`
	FleetID          string `gorm:"type:uuid;index"`
	ReadAt           *time.Time
	CreatedAt        time.Time
	DeletedAt        *time.Time `gorm:"index"`
	PurgeOperationID *string    `gorm:"type:uuid;index"`
}

func (Entity) TableName() string { return "notification.notifications" }

// Migration auto-migrates the notifications table and installs the partial
// unique index on dedupe_key.
func Migration(db *gorm.DB) error {
	if err := db.AutoMigrate(&Entity{}); err != nil {
		return err
	}
	return ApplyPartialIndexes(db)
}

// ApplyPartialIndexes replaces the legacy total unique index on dedupe_key with
// one predicated on deleted_at IS NULL.
//
// It is split out of Migration so it can be tested without AutoMigrate, which
// cannot create schema-qualified indexes on SQLite.
//
// The DROP names the LEGACY index only. AutoMigrate owns
// idx_notification_dedupe_key and would recreate anything dropped from under it
// on the next boot; dropping only the name AutoMigrate no longer emits keeps
// both halves idempotent.
//
// CREATE INDEX is schema-qualified differently on the two dialects this runs
// against: Postgres puts the schema on the TABLE and never accepts a
// schema-qualified index name; SQLite, addressing the "notification" alias
// attached by the test harness, requires the schema on the INDEX name and
// resolves the table via the attached database search path if left unqualified.
// The DROP form (schema on the index) is valid on both, so only the CREATE
// branches.
func ApplyPartialIndexes(db *gorm.DB) error {
	create := `CREATE UNIQUE INDEX IF NOT EXISTS ux_notification_dedupe_key
	 ON notification.notifications (dedupe_key) WHERE deleted_at IS NULL`
	if db.Name() == "sqlite" {
		create = `CREATE UNIQUE INDEX IF NOT EXISTS notification.ux_notification_dedupe_key
		 ON notifications (dedupe_key) WHERE deleted_at IS NULL`
	}
	stmts := []string{
		`DROP INDEX IF EXISTS notification.idx_notification_notifications_dedupe_key`,
		create,
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			return err
		}
	}
	return nil
}

// Make converts an Entity to a Model.
func Make(e Entity) Model {
	return Model{
		id:        e.ID,
		userID:    e.UserID,
		typ:       e.Type,
		title:     e.Title,
		body:      e.Body,
		dedupeKey: e.DedupeKey,
		vehicleID: e.VehicleID,
		fleetID:   e.FleetID,
		readAt:    e.ReadAt,
		createdAt: e.CreatedAt,
	}
}

// ToEntity converts a Model to an Entity for persistence.
func (m Model) ToEntity() Entity {
	return Entity{
		ID:        m.id,
		UserID:    m.userID,
		Type:      m.typ,
		Title:     m.title,
		Body:      m.body,
		DedupeKey: m.dedupeKey,
		VehicleID: m.vehicleID,
		FleetID:   m.fleetID,
		ReadAt:    m.readAt,
		CreatedAt: m.createdAt,
	}
}
