package platformadmin

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Administrator is the write interface for platform-admin grants.
type Administrator interface {
	// Grant makes userID a platform admin. Idempotent: re-granting leaves the
	// original granted_by and granted_at intact, so the audit value of "who
	// first granted this, and when" is not overwritten by a restart.
	Grant(userID, grantedBy string) error
}

type dbAdministrator struct{ db *gorm.DB }

// NewAdministrator returns an Administrator backed by the given database.
func NewAdministrator(db *gorm.DB) Administrator { return &dbAdministrator{db: db} }

func (a *dbAdministrator) Grant(userID, grantedBy string) error {
	e := Entity{UserID: userID, GrantedBy: grantedBy, GrantedAt: time.Now().UTC()}
	return a.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&e).Error
}
