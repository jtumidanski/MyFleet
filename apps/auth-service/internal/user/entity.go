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
	// EmailVerified persists Google's id_token email_verified claim so it
	// survives past the login request that observed it. platformadmin.SeedFromEmails
	// runs at boot with no id_token in hand — this column is the only way it can
	// honor the same verification gate user.Processor.maybeGrantAdmin applies at
	// login time (FR-ADMIN-AUTH-2's escalation risk otherwise resurfaces on every
	// restart). Defaults false so a row written before this column existed is
	// never silently treated as verified; it is refreshed on the user's next
	// login (ProvisionFromGoogle), same as ThemePreference's backfill above.
	EmailVerified bool `gorm:"not null;default:false"`
	// ToEntity never populates CreatedAt (Model carries no such field), so a
	// full-column gorm.Save from Administrator.Update would otherwise clobber
	// this with the zero value on every write — including the login-time
	// ProvisionFromGoogle path and, since this branch, PATCH /auth/me. `<-:create`
	// tells GORM to include the column on INSERT (where it's DB-default- or
	// callback-populated) but exclude it from every UPDATE.
	CreatedAt time.Time `gorm:"<-:create"`
	UpdatedAt time.Time
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
	return Model{id: e.ID, googleSub: e.GoogleSub, email: e.Email, displayName: e.DisplayName, avatarURL: e.AvatarURL, themePreference: theme, lastLoginAt: e.LastLoginAt, emailVerified: e.EmailVerified}
}

func (m Model) ToEntity() Entity {
	return Entity{ID: m.id, GoogleSub: m.googleSub, Email: m.email, DisplayName: m.displayName, AvatarURL: m.avatarURL, ThemePreference: m.themePreference, LastLoginAt: m.lastLoginAt, EmailVerified: m.emailVerified}
}
