package maintenancecategory

import "gorm.io/gorm"

// Administrator is the write interface for maintenance category data access.
type Administrator interface {
	// Insert stores a new category and returns the persisted Model. Fails
	// with an error satisfying errors.Is(err, gorm.ErrDuplicatedKey) when a
	// concurrent request already inserted the identical (fleet_id, name,
	// kind) row — see the idx_maintenance_categories_scope comment on
	// Entity. gorm.ErrDuplicatedKey is only produced when the connection was
	// opened with gorm.Config{TranslateError: true} (which
	// packages/shared-go/database.Connect sets, and this package's own test
	// DB helper mirrors): that flag lets each driver translate its own raw
	// error (pgconn.PgError on PostgreSQL, sqlite3.Error on SQLite) onto this
	// portable sentinel, instead of this package importing either driver's
	// error type directly. Importing github.com/mattn/go-sqlite3 directly
	// broke CGO_ENABLED=0 builds (that package's error types are cgo-gated);
	// this is why TranslateError exists.
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
