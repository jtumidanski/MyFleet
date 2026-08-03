package preferences

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Administrator is the write interface for preference data access. Upsert keys
// on (user_id, type) so toggling a preference is idempotent.
type Administrator interface {
	Upsert(userID, typ string, enabled bool) (Model, error)
}

type dbAdministrator struct{ db *gorm.DB }

// NewAdministrator returns an Administrator backed by the given database.
func NewAdministrator(db *gorm.DB) Administrator { return &dbAdministrator{db: db} }

// Upsert inserts or updates the in-app toggle for a (user, type) pair, keying on
// the unique (user_id, type) index.
//
// The conflict target names the PARTIAL unique index (user_id, type WHERE
// deleted_at IS NULL) — an unqualified ON CONFLICT (user_id, type) no longer
// infers an index now that the constraint is partial, and Postgres would
// reject the statement outright.
func (a *dbAdministrator) Upsert(userID, typ string, enabled bool) (Model, error) {
	e := Entity{
		ID:           uuid.NewString(),
		UserID:       userID,
		Type:         typ,
		InAppEnabled: enabled,
	}
	if err := a.db.Clauses(clause.OnConflict{
		Columns:     []clause.Column{{Name: "user_id"}, {Name: "type"}},
		TargetWhere: clause.Where{Exprs: []clause.Expression{clause.Expr{SQL: "deleted_at IS NULL"}}},
		DoUpdates:   clause.AssignmentColumns([]string{"in_app_enabled"}),
	}).Create(&e).Error; err != nil {
		return Model{}, err
	}
	// Re-read so the row's persisted id (which may differ from the generated one
	// on an update) is reflected back to the caller.
	//
	// deleted_at IS NULL is required, not cosmetic: (user_id, type) is only
	// unique among LIVE rows (the partial index), so a purged historical row can
	// coexist with the new live one. Without this filter, First()'s implicit
	// "order by primary key" tie-breaks on a random UUID — unrelated to which
	// row is live — and can silently hand the caller the purged row's stale
	// InAppEnabled and dead ID instead of the one just written.
	var stored Entity
	if err := a.db.Where("user_id = ? AND type = ? AND deleted_at IS NULL", userID, typ).First(&stored).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return Make(e), nil
		}
		return Model{}, err
	}
	return Make(stored), nil
}
