package fleet

import "github.com/jtumidanski/myfleet/packages/shared-go/server"

// Attributes is the JSON:API attributes payload for a fleet.
type Attributes struct {
	Name            string `json:"name"`
	CreatedByUserID string `json:"createdByUserId"`
}

// Transform converts a Model to a JSON:API Resource.
func Transform(m Model) server.Resource {
	return server.Resource{
		Type: "fleets",
		ID:   m.ID(),
		Attributes: Attributes{
			Name:            m.Name(),
			CreatedByUserID: m.CreatedByUserID(),
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
