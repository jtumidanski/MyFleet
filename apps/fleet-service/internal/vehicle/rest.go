package vehicle

import "github.com/jtumidanski/myfleet/packages/shared-go/server"

// Attributes is the JSON:API attributes payload for a vehicle.
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
}

// Transform converts a Model to a JSON:API Resource.
func Transform(m Model) server.Resource {
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
