package platformadmin

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Administrator is the write interface for platform-admin grants.
type Administrator interface {
	// Grant makes userID a platform admin. Idempotent: re-granting an EXISTING,
	// non-revoked row leaves the original granted_by and granted_at intact, so
	// the audit value of "who first granted this, and when" is not overwritten
	// by a restart.
	//
	// A revoked row (revoked_at set) is NOT re-granted by this call: the row's
	// primary key already exists, so OnConflict{DoNothing} makes Grant a no-op
	// that returns nil while doing nothing — the same as any other repeat call.
	// This method has no un-revoke path by design; the current mechanism is the
	// out-of-band `UPDATE auth.platform_admins SET revoked_at = NULL WHERE
	// user_id = ?` that mirrors how a revocation is applied. A caller — Task 12's
	// internal admin routes among them — that needs to tell "granted" apart from
	// "silently a no-op because this user is revoked" must check IsRevoked
	// itself; Grant's nil return does not make that distinction.
	Grant(userID, grantedBy string) error
}

type dbAdministrator struct{ db *gorm.DB }

// NewAdministrator returns an Administrator backed by the given database.
func NewAdministrator(db *gorm.DB) Administrator { return &dbAdministrator{db: db} }

func (a *dbAdministrator) Grant(userID, grantedBy string) error {
	e := Entity{UserID: userID, GrantedBy: grantedBy, GrantedAt: time.Now().UTC()}
	return a.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&e).Error
}
