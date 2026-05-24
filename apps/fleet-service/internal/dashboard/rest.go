package dashboard

import (
	"encoding/json"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// WidgetAttributes is the JSON:API attributes payload for a single widget.
type WidgetAttributes struct {
	Type      string          `json:"type"`
	PositionX int             `json:"positionX"`
	PositionY int             `json:"positionY"`
	Width     int             `json:"width"`
	Height    int             `json:"height"`
	Config    json.RawMessage `json:"config,omitempty"`
}

// DashboardAttributes is the JSON:API attributes payload for a dashboard.
type DashboardAttributes struct {
	FleetID   string           `json:"fleetId"`
	UserID    string           `json:"userId"`
	Widgets   []WidgetResource `json:"widgets"`
	CreatedAt string           `json:"createdAt"`
	UpdatedAt string           `json:"updatedAt"`
}

// WidgetResource is a single widget as a JSON:API resource.
type WidgetResource struct {
	Type       string           `json:"type"`
	ID         string           `json:"id"`
	Attributes WidgetAttributes `json:"attributes"`
}

// Transform converts a Dashboard to a JSON:API Resource.
func Transform(d Dashboard) server.Resource {
	widgets := make([]WidgetResource, 0, len(d.Widgets()))
	for _, w := range d.Widgets() {
		widgets = append(widgets, WidgetResource{
			Type: "dashboardWidgets",
			ID:   w.ID(),
			Attributes: WidgetAttributes{
				Type:      w.Type(),
				PositionX: w.PositionX(),
				PositionY: w.PositionY(),
				Width:     w.Width(),
				Height:    w.Height(),
				Config:    w.Config(),
			},
		})
	}

	createdAt := ""
	if !d.CreatedAt().IsZero() {
		createdAt = d.CreatedAt().Format("2006-01-02T15:04:05Z07:00")
	}
	updatedAt := ""
	if !d.UpdatedAt().IsZero() {
		updatedAt = d.UpdatedAt().Format("2006-01-02T15:04:05Z07:00")
	}

	return server.Resource{
		Type: "dashboards",
		ID:   d.ID(),
		Attributes: DashboardAttributes{
			FleetID:   d.FleetID(),
			UserID:    d.UserID(),
			Widgets:   widgets,
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
		},
	}
}
