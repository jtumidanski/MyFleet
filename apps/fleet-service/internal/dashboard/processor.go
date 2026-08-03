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
	err := p.db.Where("fleet_id = ? AND user_id = ? AND deleted_at IS NULL", fleetID, userID).First(&e).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			// No layout saved yet — return an empty dashboard (not an error).
			// A soft-deleted layout takes this branch too, which is right: to an
			// ordinary user a purged dashboard is indistinguishable from one
			// they never saved.
			return Dashboard{fleetID: fleetID, userID: userID}, nil
		}
		return Dashboard{}, err
	}
	var ws []WidgetEntity
	if err := p.db.Where("dashboard_id = ? AND deleted_at IS NULL", e.ID).Find(&ws).Error; err != nil {
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
//
// The lookup deliberately INCLUDES soft-deleted rows. A fleet purge stamps the
// dashboard; if the user then saves a layout, inserting a second row would leave
// two live rows for one (fleet, user) once the purge is cancelled, and the read
// above — First with no ordering — would pick between them non-deterministically.
// Reviving instead is also the better outcome: the user re-created their layout,
// which is exactly what a later cancel would have produced (design §6.4).
func (a *dbAdministrator) Replace(fleetID, userID string, widgets []WidgetInput) (Dashboard, error) {
	var result Dashboard
	err := a.db.Transaction(func(tx *gorm.DB) error {
		var e DashboardEntity
		dbErr := tx.Where("fleet_id = ? AND user_id = ?", fleetID, userID).First(&e).Error
		if dbErr != nil {
			if dbErr != gorm.ErrRecordNotFound {
				return dbErr
			}
			e = DashboardEntity{ID: uuid.NewString(), FleetID: fleetID, UserID: userID}
			if err := tx.Create(&e).Error; err != nil {
				return err
			}
		} else {
			// Touch updated_at and revive: clearing both columns together is the
			// same shape the manifest's Restore uses, so a revived row is
			// indistinguishable from a restored one.
			if err := tx.Model(&DashboardEntity{}).Where("id = ?", e.ID).Updates(map[string]any{
				"updated_at":         gorm.Expr("CURRENT_TIMESTAMP"),
				"deleted_at":         nil,
				"purge_operation_id": nil,
			}).Error; err != nil {
				return err
			}
		}

		// Widgets are hard-deleted and recreated on every save, so their
		// deleted_at only ever matters between a stamp and its cancel.
		if err := tx.Where("dashboard_id = ?", e.ID).Delete(&WidgetEntity{}).Error; err != nil {
			return err
		}

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

		if err := tx.Where("fleet_id = ? AND user_id = ? AND deleted_at IS NULL", fleetID, userID).
			First(&e).Error; err != nil {
			return err
		}
		var finalWs []WidgetEntity
		if err := tx.Where("dashboard_id = ? AND deleted_at IS NULL", e.ID).Find(&finalWs).Error; err != nil {
			return err
		}
		result = MakeDashboard(e, finalWs)
		return nil
	})
	return result, err
}
