package mileage

import "github.com/jtumidanski/myfleet/packages/shared-go/server"

// Attributes is the JSON:API attributes payload for a mileage record.
type Attributes struct {
	VehicleID       string `json:"vehicleId"`
	Mileage         int    `json:"mileage"`
	RecordedAt      string `json:"recordedAt"`
	Source          string `json:"source"`
	SourceRefID     string `json:"sourceRefId,omitempty"`
	CreatedByUserID string `json:"createdByUserId,omitempty"`
	CreatedAt       string `json:"createdAt"`
	Flagged         bool   `json:"flagged"`
}

// Transform converts a Model to a JSON:API Resource.
func Transform(m Model) server.Resource {
	return server.Resource{
		Type: "mileageRecords",
		ID:   m.ID(),
		Attributes: Attributes{
			VehicleID:       m.VehicleID(),
			Mileage:         m.Mileage(),
			RecordedAt:      m.RecordedAt().Format("2006-01-02T15:04:05Z07:00"),
			Source:          m.Source(),
			SourceRefID:     m.SourceRefID(),
			CreatedByUserID: m.CreatedByUserID(),
			CreatedAt:       m.CreatedAt().Format("2006-01-02T15:04:05Z07:00"),
			Flagged:         m.Flagged(),
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
