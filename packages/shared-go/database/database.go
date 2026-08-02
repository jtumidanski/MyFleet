package database

import (
	"github.com/sirupsen/logrus"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/config"
)

type Migration func(db *gorm.DB) error

type options struct{ migrations []Migration }

type Option func(*options)

// SetMigrations registers AutoMigrate funcs run on Connect (design §6/§13).
func SetMigrations(m ...Migration) Option {
	return func(o *options) { o.migrations = append(o.migrations, m...) }
}

// Connect opens the service DB (DATABASE_URL) and runs registered migrations.
func Connect(log logrus.FieldLogger, opts ...Option) (*gorm.DB, error) {
	o := &options{}
	for _, opt := range opts {
		opt(o)
	}
	// TranslateError lets each driver map its own raw errors (pgconn.PgError,
	// sqlite3.Error, ...) onto GORM's portable sentinels (gorm.ErrDuplicatedKey
	// et al.), so callers can use errors.Is against those sentinels instead of
	// inspecting driver-specific, cgo-gated error types directly.
	db, err := gorm.Open(postgres.Open(config.MustGet("DATABASE_URL")), &gorm.Config{TranslateError: true})
	if err != nil {
		return nil, err
	}
	for _, m := range o.migrations {
		if err := m(db); err != nil {
			return nil, err
		}
	}
	log.Info("database connected and migrated")
	return db, nil
}
