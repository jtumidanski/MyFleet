package notification

import (
	"time"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// Attributes is the JSON:API attributes payload for a notification.
type Attributes struct {
	UserID    string     `json:"userId"`
	Type      string     `json:"type"`
	Title     string     `json:"title"`
	Body      string     `json:"body,omitempty"`
	VehicleID string     `json:"vehicleId,omitempty"`
	FleetID   string     `json:"fleetId"`
	Read      bool       `json:"read"`
	ReadAt    *time.Time `json:"readAt,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
}

// Transform converts a Model to a JSON:API Resource.
func Transform(m Model) server.Resource {
	return server.Resource{
		Type: "notifications",
		ID:   m.ID(),
		Attributes: Attributes{
			UserID:    m.UserID(),
			Type:      m.Type(),
			Title:     m.Title(),
			Body:      m.Body(),
			VehicleID: m.VehicleID(),
			FleetID:   m.FleetID(),
			Read:      m.Read(),
			ReadAt:    m.ReadAt(),
			CreatedAt: m.CreatedAt(),
		},
	}
}

// TransformSlice converts a slice of Models to JSON:API Resources.
func TransformSlice(ms []Model) []server.Resource {
	out := make([]server.Resource, 0, len(ms))
	for _, m := range ms {
		out = append(out, Transform(m))
	}
	return out
}
