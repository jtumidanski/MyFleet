package preferences

import "github.com/jtumidanski/myfleet/packages/shared-go/server"

// Attributes is the JSON:API attributes payload for a notification preference.
type Attributes struct {
	UserID       string `json:"userId"`
	Type         string `json:"type"`
	InAppEnabled bool   `json:"inAppEnabled"`
}

// Transform converts a Model to a JSON:API Resource.
func Transform(m Model) server.Resource {
	return server.Resource{
		Type: "notification-preferences",
		ID:   m.ID(),
		Attributes: Attributes{
			UserID:       m.UserID(),
			Type:         m.Type(),
			InAppEnabled: m.InAppEnabled(),
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
