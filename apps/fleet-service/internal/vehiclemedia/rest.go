package vehiclemedia

import "github.com/jtumidanski/myfleet/packages/shared-go/server"

// Attributes is the JSON:API attributes payload for a vehicle media reference.
type Attributes struct {
	VehicleID string `json:"vehicleId"`
	MediaID   string `json:"mediaId"`
	IsPrimary bool   `json:"isPrimary"`
	SortOrder int    `json:"sortOrder"`
}

// Transform converts a Model to a JSON:API Resource.
func Transform(m Model) server.Resource {
	return server.Resource{
		Type: "vehicleMedia",
		ID:   m.ID(),
		Attributes: Attributes{
			VehicleID: m.VehicleID(),
			MediaID:   m.MediaID(),
			IsPrimary: m.IsPrimary(),
			SortOrder: m.SortOrder(),
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
