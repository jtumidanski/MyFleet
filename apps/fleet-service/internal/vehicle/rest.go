package vehicle

import "github.com/jtumidanski/myfleet/packages/shared-go/server"

// Attributes is the JSON:API attributes payload for a vehicle.
//
// Status is DERIVED ON READ (design §10.2) and never stored on the entity. It is
// computed from the vehicle's active maintenance-schedule states + last activity
// time via status.Derive and exposed read-only here.
type Attributes struct {
	FleetID             string `json:"fleetId"`
	Nickname            string `json:"nickname,omitempty"`
	Make                string `json:"make"`
	Model               string `json:"model"`
	Trim                string `json:"trim,omitempty"`
	Year                int    `json:"year"`
	VIN                 string `json:"vin,omitempty"`
	CurrentMileage      int    `json:"currentMileage,omitempty"`
	PrimaryImageMediaID string `json:"primaryImageMediaId,omitempty"`
	Notes               string `json:"notes,omitempty"`
	Status              string `json:"status,omitempty"`
}

// Transform converts a Model to a JSON:API Resource (status omitted).
func Transform(m Model) server.Resource {
	return TransformWithStatus(m, "")
}

// TransformWithStatus converts a Model to a JSON:API Resource, attaching the
// read-only derived status when supplied.
func TransformWithStatus(m Model, status string) server.Resource {
	return server.Resource{
		Type: "vehicles",
		ID:   m.ID(),
		Attributes: Attributes{
			FleetID:             m.FleetID(),
			Nickname:            m.Nickname(),
			Make:                m.Make(),
			Model:               m.Model(),
			Trim:                m.Trim(),
			Year:                m.Year(),
			VIN:                 m.VIN(),
			CurrentMileage:      m.CurrentMileage(),
			PrimaryImageMediaID: m.PrimaryImageMediaID(),
			Notes:               m.Notes(),
			Status:              status,
		},
	}
}

// TransformSlice converts a slice of Models to JSON:API Resources (no status).
func TransformSlice(ms []Model) []server.Resource {
	out := make([]server.Resource, 0, len(ms))
	for _, m := range ms {
		out = append(out, Transform(m))
	}
	return out
}
