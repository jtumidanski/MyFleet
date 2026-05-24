package maintenancecategory

import "github.com/jtumidanski/myfleet/packages/shared-go/server"

// Attributes is the JSON:API attributes payload for a maintenance category.
type Attributes struct {
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	SystemDefined bool   `json:"systemDefined"`
}

// Transform converts a Model to a JSON:API Resource.
func Transform(m Model) server.Resource {
	return server.Resource{
		Type: "maintenanceCategories",
		ID:   m.ID(),
		Attributes: Attributes{
			Name:          m.Name(),
			Description:   m.Description(),
			SystemDefined: m.SystemDefined(),
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
