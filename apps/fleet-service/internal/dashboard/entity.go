package dashboard

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// DashboardEntity maps to fleet.dashboards (plan §13.1).
// UNIQUE(fleet_id, user_id) — each user has one layout per fleet.
type DashboardEntity struct {
	ID        string `gorm:"type:uuid;primaryKey"`
	FleetID   string `gorm:"type:uuid;not null;index"`
	UserID    string `gorm:"type:uuid;not null"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (DashboardEntity) TableName() string { return "fleet.dashboards" }

// WidgetEntity maps to fleet.dashboard_widgets (plan §13.1).
type WidgetEntity struct {
	ID          string          `gorm:"type:uuid;primaryKey"`
	DashboardID string          `gorm:"type:uuid;not null;index"`
	Type        string          `gorm:"not null"`
	PositionX   int             `gorm:"not null"`
	PositionY   int             `gorm:"not null"`
	Width       int             `gorm:"not null"`
	Height      int             `gorm:"not null"`
	Config      json.RawMessage `gorm:"type:jsonb"`
}

func (WidgetEntity) TableName() string { return "fleet.dashboard_widgets" }

// Migration runs AutoMigrate for both tables.
func Migration(db *gorm.DB) error {
	return db.AutoMigrate(&DashboardEntity{}, &WidgetEntity{})
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
