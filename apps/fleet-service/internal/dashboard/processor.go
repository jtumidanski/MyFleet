package dashboard

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// ValidCatalog is the set of known widget types (design §12, plan §13.1).
var ValidCatalog = map[string]struct{}{
	"fleet-overview":       {},
	"vehicle-status":       {},
	"upcoming-maintenance": {},
	"overdue-maintenance":  {},
	"recent-activity":      {},
	"spend-by-vehicle":     {},
	"mileage-trends":       {},
}

// WidgetInput is the user-supplied widget descriptor used in layout PUT requests.
type WidgetInput struct {
	Type      string          `json:"type"`
	PositionX int             `json:"positionX"`
	PositionY int             `json:"positionY"`
	Width     int             `json:"width"`
	Height    int             `json:"height"`
	Config    json.RawMessage `json:"config,omitempty"`
}

// ValidateLayout returns server.ErrValidation if any widget type is not in the
// catalog. An empty layout is valid.
func ValidateLayout(widgets []WidgetInput) error {
	for _, w := range widgets {
		if _, ok := ValidCatalog[w.Type]; !ok {
			return fmt.Errorf("unknown widget type %q: %w", w.Type, server.ErrValidation)
		}
	}
	return nil
}

// Processor contains dashboard business logic.
type Processor struct {
	log  logrus.FieldLogger
	prov Provider
}

// NewProcessor constructs a Processor with the given logger and provider.
func NewProcessor(log logrus.FieldLogger, prov Provider) *Processor {
	return &Processor{log: log, prov: prov}
}

// GetDashboard returns the per-user dashboard for the given fleet. If no layout
// has been saved yet it returns an empty dashboard (not an error).
func (pr *Processor) GetDashboard(fleetID, userID string) (Dashboard, error) {
	return pr.prov.GetDashboard(fleetID, userID)
}

// ReplaceDashboard validates the widget set, upserts the dashboard row keyed by
// (fleet_id, user_id), then replaces widgets transactionally (delete + insert in
// ONE db.Transaction). Returns the saved dashboard.
func (pr *Processor) ReplaceDashboard(adm Administrator, fleetID, userID string, widgets []WidgetInput) (Dashboard, error) {
	if err := ValidateLayout(widgets); err != nil {
		return Dashboard{}, err
	}
	return adm.Replace(fleetID, userID, widgets)
}

// Provider is the read-only interface for dashboard data access.
type Provider interface {
	// GetDashboard returns the per-user dashboard. Returns an empty dashboard
	// (zero widgets) when none has been saved yet.
	GetDashboard(fleetID, userID string) (Dashboard, error)
}

// Administrator is the write interface for dashboard data access.
type Administrator interface {
	// Replace upserts the dashboard row and atomically replaces the widget set.
	Replace(fleetID, userID string, widgets []WidgetInput) (Dashboard, error)
}

// dbProvider is the concrete read-only provider.
type dbProvider struct{ db *gorm.DB }

// NewProvider returns a read-only Provider backed by the given database.
func NewProvider(db *gorm.DB) Provider { return &dbProvider{db: db} }

func (p *dbProvider) GetDashboard(fleetID, userID string) (Dashboard, error) {
	var e DashboardEntity
	err := p.db.Where("fleet_id = ? AND user_id = ?", fleetID, userID).First(&e).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			// No layout saved yet — return an empty dashboard (not an error).
			return Dashboard{fleetID: fleetID, userID: userID}, nil
		}
		return Dashboard{}, err
	}
	var ws []WidgetEntity
	if err := p.db.Where("dashboard_id = ?", e.ID).Find(&ws).Error; err != nil {
		return Dashboard{}, err
	}
	return MakeDashboard(e, ws), nil
}

// dbAdministrator is the concrete write administrator.
type dbAdministrator struct{ db *gorm.DB }

// NewAdministrator returns an Administrator backed by the given database.
func NewAdministrator(db *gorm.DB) Administrator { return &dbAdministrator{db: db} }

// Replace upserts the dashboard row by (fleet_id, user_id), then deletes and
// re-inserts the widget set inside ONE db.Transaction.
func (a *dbAdministrator) Replace(fleetID, userID string, widgets []WidgetInput) (Dashboard, error) {
	var result Dashboard
	err := a.db.Transaction(func(tx *gorm.DB) error {
		// Upsert dashboard row.
		var e DashboardEntity
		dbErr := tx.Where("fleet_id = ? AND user_id = ?", fleetID, userID).First(&e).Error
		if dbErr != nil {
			if dbErr != gorm.ErrRecordNotFound {
				return dbErr
			}
			// Create new dashboard row.
			e = DashboardEntity{
				ID:      uuid.NewString(),
				FleetID: fleetID,
				UserID:  userID,
			}
			if err := tx.Create(&e).Error; err != nil {
				return err
			}
		} else {
			// Touch updated_at.
			if err := tx.Model(&e).Updates(map[string]any{"updated_at": gorm.Expr("CURRENT_TIMESTAMP")}).Error; err != nil {
				return err
			}
		}

		// Delete existing widgets for this dashboard.
		if err := tx.Where("dashboard_id = ?", e.ID).Delete(&WidgetEntity{}).Error; err != nil {
			return err
		}

		// Insert new widget set.
		wes := make([]WidgetEntity, 0, len(widgets))
		for _, w := range widgets {
			wes = append(wes, WidgetEntity{
				ID:          uuid.NewString(),
				DashboardID: e.ID,
				Type:        w.Type,
				PositionX:   w.PositionX,
				PositionY:   w.PositionY,
				Width:       w.Width,
				Height:      w.Height,
				Config:      w.Config,
			})
		}
		if len(wes) > 0 {
			if err := tx.Create(&wes).Error; err != nil {
				return err
			}
		}

		// Reload final state.
		if err := tx.Where("fleet_id = ? AND user_id = ?", fleetID, userID).First(&e).Error; err != nil {
			return err
		}
		var finalWs []WidgetEntity
		if err := tx.Where("dashboard_id = ?", e.ID).Find(&finalWs).Error; err != nil {
			return err
		}
		result = MakeDashboard(e, finalWs)
		return nil
	})
	return result, err
}
