package preferences

import (
	"time"

	"gorm.io/gorm"
)

// Entity maps to notification.notification_preferences (PRD §6).
//
// (user_id, type) uniqueness is a PARTIAL index (ApplyPartialIndexes): after a
// system purge a user's preference row must be re-creatable (design F1).
type Entity struct {
	ID               string     `gorm:"type:uuid;primaryKey"`
	UserID           string     `gorm:"type:uuid;not null;index:idx_pref_user_type"`
	Type             string     `gorm:"not null;index:idx_pref_user_type"`
	InAppEnabled     bool       `gorm:"not null"`
	DeletedAt        *time.Time `gorm:"index"`
	PurgeOperationID *string    `gorm:"type:uuid;index"`
}

func (Entity) TableName() string { return "notification.notification_preferences" }

// Migration auto-migrates the preferences table and installs the partial
// unique index on (user_id, type).
func Migration(db *gorm.DB) error {
	if err := db.AutoMigrate(&Entity{}); err != nil {
		return err
	}
	return ApplyPartialIndexes(db)
}

// ApplyPartialIndexes replaces the legacy total unique index on (user_id, type)
// with one predicated on deleted_at IS NULL.
//
// It is split out of Migration so it can be tested without AutoMigrate, which
// cannot create schema-qualified indexes on SQLite.
//
// The DROP names the LEGACY index only (the name the entity used to tag
// before this change). AutoMigrate owns idx_pref_user_type and would recreate
// anything dropped from under it on the next boot; dropping only the name
// AutoMigrate no longer emits keeps both halves idempotent.
//
// CREATE INDEX is schema-qualified differently on the two dialects this runs
// against: Postgres puts the schema on the TABLE and never accepts a
// schema-qualified index name; SQLite, addressing the "notification" alias
// attached by the test harness, requires the schema on the INDEX name and
// resolves the table via the attached database search path if left unqualified.
// The DROP form (schema on the index) is valid on both, so only the CREATE
// branches.
func ApplyPartialIndexes(db *gorm.DB) error {
	create := `CREATE UNIQUE INDEX IF NOT EXISTS ux_pref_user_type_live
	 ON notification.notification_preferences (user_id, type) WHERE deleted_at IS NULL`
	if db.Dialector.Name() == "sqlite" {
		create = `CREATE UNIQUE INDEX IF NOT EXISTS notification.ux_pref_user_type_live
		 ON notification_preferences (user_id, type) WHERE deleted_at IS NULL`
	}
	stmts := []string{
		`DROP INDEX IF EXISTS notification.ux_pref_user_type`,
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
		id:           e.ID,
		userID:       e.UserID,
		typ:          e.Type,
		inAppEnabled: e.InAppEnabled,
	}
}

// ToEntity converts a Model to an Entity for persistence.
func (m Model) ToEntity() Entity {
	return Entity{
		ID:           m.id,
		UserID:       m.userID,
		Type:         m.typ,
		InAppEnabled: m.inAppEnabled,
	}
}
