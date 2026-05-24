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
	LastLoginAt time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (Entity) TableName() string { return "auth.users" }

func Migration(db *gorm.DB) error { return db.AutoMigrate(&Entity{}) }

func Make(e Entity) Model {
	return Model{id: e.ID, googleSub: e.GoogleSub, email: e.Email, displayName: e.DisplayName, avatarURL: e.AvatarURL, lastLoginAt: e.LastLoginAt}
}

func (m Model) ToEntity() Entity {
	return Entity{ID: m.id, GoogleSub: m.googleSub, Email: m.email, DisplayName: m.displayName, AvatarURL: m.avatarURL, LastLoginAt: m.lastLoginAt}
}
