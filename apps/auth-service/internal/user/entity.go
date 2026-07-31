package user

import (
	"time"

	"gorm.io/gorm"
)

type Entity struct {
	ID          string `gorm:"type:uuid;primaryKey"`
	GoogleSub   string `gorm:"uniqueIndex;not null"`
	Email       string `gorm:"uniqueIndex;not null"`
	DisplayName string
	AvatarURL   string
	// AutoMigrate issues ADD COLUMN theme_preference ... NOT NULL DEFAULT
	// 'system', which Postgres backfills across existing rows. The default is
	// what migrates old rows; new rows get their value from NewBuilder
	// (design §3.4), so no insert path depends on it.
	ThemePreference string `gorm:"not null;default:'system'"`
	LastLoginAt     time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (Entity) TableName() string { return "auth.users" }

func Migration(db *gorm.DB) error { return db.AutoMigrate(&Entity{}) }

func Make(e Entity) Model {
	// FR-DATA-4: normalise on read. A row written before the column existed, or
	// edited out of band, must never reach a client as an out-of-range value —
	// GET /auth/me promises one of exactly three.
	theme := e.ThemePreference
	if !IsValidTheme(theme) {
		theme = ThemeSystem
	}
	return Model{id: e.ID, googleSub: e.GoogleSub, email: e.Email, displayName: e.DisplayName, avatarURL: e.AvatarURL, themePreference: theme, lastLoginAt: e.LastLoginAt}
}

func (m Model) ToEntity() Entity {
	return Entity{ID: m.id, GoogleSub: m.googleSub, Email: m.email, DisplayName: m.displayName, AvatarURL: m.avatarURL, ThemePreference: m.themePreference, LastLoginAt: m.lastLoginAt}
}
