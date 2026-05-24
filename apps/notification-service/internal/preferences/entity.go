package preferences

import "gorm.io/gorm"

// Entity maps to notification.notification_preferences (PRD §6). The composite
// (user_id, type) is unique so each user has at most one row per notification
// type.
type Entity struct {
	ID           string `gorm:"type:uuid;primaryKey"`
	UserID       string `gorm:"type:uuid;not null;uniqueIndex:ux_pref_user_type"`
	Type         string `gorm:"not null;uniqueIndex:ux_pref_user_type"`
	InAppEnabled bool   `gorm:"not null"`
}

func (Entity) TableName() string { return "notification.notification_preferences" }

// Migration auto-migrates the preferences table.
func Migration(db *gorm.DB) error { return db.AutoMigrate(&Entity{}) }

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
