package maintenancecategory

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/mattn/go-sqlite3"
	"gorm.io/gorm"
)

// Administrator is the write interface for maintenance category data access.
type Administrator interface {
	// Insert stores a new category and returns the persisted Model. Fails
	// with a unique-constraint violation (detectable via isUniqueViolation)
	// when a concurrent request already inserted the identical
	// (fleet_id, name, kind) row — see the idx_maintenance_categories_scope
	// comment on Entity.
	Insert(m Model) (Model, error)
}

type dbAdministrator struct{ db *gorm.DB }

// NewAdministrator returns an Administrator backed by the given database.
func NewAdministrator(db *gorm.DB) Administrator { return &dbAdministrator{db: db} }

func (a *dbAdministrator) Insert(m Model) (Model, error) {
	e := m.ToEntity()
	if err := a.db.Create(&e).Error; err != nil {
		return Model{}, err
	}
	return Make(e), nil
}

// isUniqueViolation reports whether err is a unique-constraint violation on
// either PostgreSQL (production, via pgx) or SQLite (this package's unit
// tests, via mattn/go-sqlite3). Neither GORM driver in use implements
// GORM's ErrorTranslator, so gorm.ErrDuplicatedKey is never produced here —
// the underlying driver errors must be inspected directly.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" // unique_violation
	}
	var sqliteErr sqlite3.Error
	if errors.As(err, &sqliteErr) {
		return sqliteErr.ExtendedCode == sqlite3.ErrConstraintUnique
	}
	return false
}
