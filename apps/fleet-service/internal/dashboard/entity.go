package dashboard

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// DashboardEntity maps to fleet.dashboards (plan §13.1).
//
// One layout per (fleet, user). That invariant was only ever a comment: the
// tags declared a plain index on FleetID and nothing enforced uniqueness. It is
// enforced now, as a PARTIAL index, so a purged dashboard does not block the
// user's next save (design §6.4).
type DashboardEntity struct {
	ID               string `gorm:"type:uuid;primaryKey"`
	FleetID          string `gorm:"type:uuid;not null;index:idx_dashboard_fleet_user"`
	UserID           string `gorm:"type:uuid;not null;index:idx_dashboard_fleet_user"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        *time.Time `gorm:"index"`
	PurgeOperationID *string    `gorm:"type:uuid;index"`
}

func (DashboardEntity) TableName() string { return "fleet.dashboards" }

// WidgetEntity maps to fleet.dashboard_widgets (plan §13.1).
type WidgetEntity struct {
	ID               string          `gorm:"type:uuid;primaryKey"`
	DashboardID      string          `gorm:"type:uuid;not null;index"`
	Type             string          `gorm:"not null"`
	PositionX        int             `gorm:"not null"`
	PositionY        int             `gorm:"not null"`
	Width            int             `gorm:"not null"`
	Height           int             `gorm:"not null"`
	Config           json.RawMessage `gorm:"type:jsonb"`
	DeletedAt        *time.Time      `gorm:"index"`
	PurgeOperationID *string         `gorm:"type:uuid;index"`
}

func (WidgetEntity) TableName() string { return "fleet.dashboard_widgets" }

// Migration runs AutoMigrate for both tables, then applies the partial unique
// index that enforces one live dashboard per (fleet, user).
func Migration(db *gorm.DB) error {
	if err := db.AutoMigrate(&DashboardEntity{}, &WidgetEntity{}); err != nil {
		return err
	}
	return ApplyPartialIndexes(db)
}

// ApplyPartialIndexes enforces one live dashboard per (fleet, user). See
// membership.ApplyPartialIndexes for why the CREATE branches by dialect: on
// Postgres the schema qualifies the table (index name is never qualified); on
// SQLite (the ATTACHed "fleet" alias used by tests) it's the reverse.
func ApplyPartialIndexes(db *gorm.DB) error {
	create := `CREATE UNIQUE INDEX IF NOT EXISTS ux_dashboard_fleet_user
	                ON fleet.dashboards (fleet_id, user_id) WHERE deleted_at IS NULL`
	if db.Name() == "sqlite" {
		create = `CREATE UNIQUE INDEX IF NOT EXISTS fleet.ux_dashboard_fleet_user
		                ON dashboards (fleet_id, user_id) WHERE deleted_at IS NULL`
	}
	return db.Exec(create).Error
}

// MakeDashboard assembles a Dashboard domain model from entity + widget rows.
func MakeDashboard(e DashboardEntity, ws []WidgetEntity) Dashboard {
	widgets := make([]Widget, 0, len(ws))
	for _, w := range ws {
		widgets = append(widgets, Widget{
			id:          w.ID,
			dashboardID: w.DashboardID,
			widgetType:  w.Type,
			positionX:   w.PositionX,
			positionY:   w.PositionY,
			width:       w.Width,
			height:      w.Height,
			config:      w.Config,
		})
	}
	return Dashboard{
		id:        e.ID,
		fleetID:   e.FleetID,
		userID:    e.UserID,
		widgets:   widgets,
		createdAt: e.CreatedAt,
		updatedAt: e.UpdatedAt,
	}
}
