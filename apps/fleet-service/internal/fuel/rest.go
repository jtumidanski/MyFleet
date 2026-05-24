package fuel

import "github.com/jtumidanski/myfleet/packages/shared-go/server"

// Attributes is the JSON:API attributes payload for a fuel log.
type Attributes struct {
	VehicleID       string  `json:"vehicleId"`
	Date            string  `json:"date"`
	Mileage         int     `json:"mileage"`
	Gallons         float64 `json:"gallons"`
	TotalCost       float64 `json:"totalCost"`
	PricePerGallon  float64 `json:"pricePerGallon"`
	CreatedByUserID string  `json:"createdByUserId,omitempty"`
	CreatedAt       string  `json:"createdAt"`
	UpdatedAt       string  `json:"updatedAt"`
}

// Transform converts a Model to a JSON:API Resource.
func Transform(m Model) server.Resource {
	return server.Resource{
		Type: "fuelLogs",
		ID:   m.ID(),
		Attributes: Attributes{
			VehicleID:       m.VehicleID(),
			Date:            m.Date().Format("2006-01-02T15:04:05Z07:00"),
			Mileage:         m.Mileage(),
			Gallons:         m.Gallons(),
			TotalCost:       m.TotalCost(),
			PricePerGallon:  m.PricePerGallon(),
			CreatedByUserID: m.CreatedByUserID(),
			CreatedAt:       m.CreatedAt().Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt:       m.UpdatedAt().Format("2006-01-02T15:04:05Z07:00"),
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
