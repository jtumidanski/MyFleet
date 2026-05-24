// Package dashboard implements per-user widget-layout persistence + aggregation
// endpoints (design §12, plan phase 13).
package dashboard

import (
	"encoding/json"
	"time"
)

// Dashboard is the immutable per-user, per-fleet widget layout.
type Dashboard struct {
	id        string
	fleetID   string
	userID    string
	widgets   []Widget
	createdAt time.Time
	updatedAt time.Time
}

func (d Dashboard) ID() string           { return d.id }
func (d Dashboard) FleetID() string      { return d.fleetID }
func (d Dashboard) UserID() string       { return d.userID }
func (d Dashboard) Widgets() []Widget    { return d.widgets }
func (d Dashboard) CreatedAt() time.Time { return d.createdAt }
func (d Dashboard) UpdatedAt() time.Time { return d.updatedAt }

// Widget is a single positioned widget within a dashboard.
type Widget struct {
	id          string
	dashboardID string
	widgetType  string
	positionX   int
	positionY   int
	width       int
	height      int
	config      json.RawMessage
}

func (w Widget) ID() string              { return w.id }
func (w Widget) DashboardID() string     { return w.dashboardID }
func (w Widget) Type() string            { return w.widgetType }
func (w Widget) PositionX() int          { return w.positionX }
func (w Widget) PositionY() int          { return w.positionY }
func (w Widget) Width() int              { return w.width }
func (w Widget) Height() int             { return w.height }
func (w Widget) Config() json.RawMessage { return w.config }
