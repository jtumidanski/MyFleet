package maintenancerecord

import "github.com/jtumidanski/myfleet/packages/shared-go/server"

const timeFormat = "2006-01-02T15:04:05Z07:00"

// Attributes is the JSON:API attributes payload for a maintenance record.
type Attributes struct {
	VehicleID  string `json:"vehicleId"`
	CategoryID string `json:"categoryId"`
	// omitempty, consistent with the existing vendor/notes fields. Clients treat
	// an absent description as empty and fall back to the category name
	// (PRD FR-REC-2, api-contracts §2).
	Description      string   `json:"description,omitempty"`
	PerformedAt      string   `json:"performedAt"`
	Mileage          int      `json:"mileage"`
	Cost             float64  `json:"cost"`
	Vendor           string   `json:"vendor,omitempty"`
	Notes            string   `json:"notes,omitempty"`
	CreatedByUserID  string   `json:"createdByUserId,omitempty"`
	CreatedAt        string   `json:"createdAt"`
	DocumentMediaIDs []string `json:"documentMediaIds,omitempty"`
}

// Transform converts a Model to a JSON:API Resource.
func Transform(m Model) server.Resource {
	return server.Resource{
		Type: "maintenanceRecords",
		ID:   m.ID(),
		Attributes: Attributes{
			VehicleID:        m.VehicleID(),
			CategoryID:       m.CategoryID(),
			Description:      m.Description(),
			PerformedAt:      m.PerformedAt().Format(timeFormat),
			Mileage:          m.Mileage(),
			Cost:             m.Cost(),
			Vendor:           m.Vendor(),
			Notes:            m.Notes(),
			CreatedByUserID:  m.CreatedByUserID(),
			CreatedAt:        m.CreatedAt().Format(timeFormat),
			DocumentMediaIDs: m.DocumentMediaIDs(),
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
