package session

import (
	"time"

	"gorm.io/gorm"
)

// Entity is the GORM mapping for a hashed, rotating refresh token
// (design §8.1: single-use, family-based reuse detection).
type Entity struct {
	ID         string `gorm:"type:uuid;primaryKey"`
	UserID     string `gorm:"type:uuid;index;not null"`
	TokenHash  string `gorm:"uniqueIndex;not null"`
	FamilyID   string `gorm:"type:uuid;index;not null"`
	ExpiresAt  time.Time
	RevokedAt  *time.Time
	ConsumedAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (Entity) TableName() string { return "auth.refresh_tokens" }

func Migration(db *gorm.DB) error { return db.AutoMigrate(&Entity{}) }

func Make(e Entity) Model {
	return Model{
		id:         e.ID,
		userID:     e.UserID,
		tokenHash:  e.TokenHash,
		familyID:   e.FamilyID,
		expiresAt:  e.ExpiresAt,
		revokedAt:  e.RevokedAt,
		consumedAt: e.ConsumedAt,
	}
}

func (m Model) ToEntity() Entity {
	return Entity{
		ID:         m.id,
		UserID:     m.userID,
		TokenHash:  m.tokenHash,
		FamilyID:   m.familyID,
		ExpiresAt:  m.expiresAt,
		RevokedAt:  m.revokedAt,
		ConsumedAt: m.consumedAt,
	}
}
