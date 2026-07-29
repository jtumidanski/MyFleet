package activity

import (
	"encoding/json"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

const timeFormat = "2006-01-02T15:04:05Z07:00"

// Attributes is the JSON:API attributes payload for an activity event.
type Attributes struct {
	FleetID     string         `json:"fleetId"`
	VehicleID   string         `json:"vehicleId,omitempty"`
	ActorUserID string         `json:"actorUserId"`
	Type        string         `json:"type"`
	Payload     map[string]any `json:"payload,omitempty"`
	CreatedAt   string         `json:"createdAt"`
}

// Transform converts a Model to a JSON:API Resource. The stored JSON payload is
// decoded back into a map for transport; an unparseable payload yields a nil map.
func Transform(m Model) (server.Resource, error) {
	var payload map[string]any
	if len(m.Payload()) > 0 {
		if err := json.Unmarshal(m.Payload(), &payload); err != nil {
			return server.Resource{}, err
		}
	}
	vehicleID := ""
	if m.VehicleID() != nil {
		vehicleID = *m.VehicleID()
	}
	return server.Resource{
		Type: "activityEvents",
		ID:   m.ID(),
		Attributes: Attributes{
			FleetID:     m.FleetID(),
			VehicleID:   vehicleID,
			ActorUserID: m.ActorUserID(),
			Type:        m.Type(),
			Payload:     payload,
			CreatedAt:   m.CreatedAt().Format(timeFormat),
		},
	}, nil
}

// TransformSlice converts a slice of Models to JSON:API Resources.
func TransformSlice(ms []Model) ([]server.Resource, error) {
	out := make([]server.Resource, 0, len(ms))
	for _, m := range ms {
		r, err := Transform(m)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}
