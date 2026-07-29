package invite

import "github.com/jtumidanski/myfleet/packages/shared-go/server"

// Attributes is the JSON:API attributes payload for an invite.
type Attributes struct {
	FleetID         string  `json:"fleetId"`
	Email           string  `json:"email"`
	Role            string  `json:"role"`
	Token           string  `json:"token"`
	ExpiresAt       string  `json:"expiresAt"`
	AcceptedAt      *string `json:"acceptedAt,omitempty"`
	InvitedByUserID string  `json:"invitedByUserId"`
}

// Transform converts a Model to a JSON:API Resource.
func Transform(m Model) server.Resource {
	attrs := Attributes{
		FleetID:         m.FleetID(),
		Email:           m.Email(),
		Role:            m.Role(),
		Token:           m.Token(),
		ExpiresAt:       m.ExpiresAt().Format("2006-01-02T15:04:05Z07:00"),
		InvitedByUserID: m.InvitedByUserID(),
	}
	if m.AcceptedAt() != nil {
		s := m.AcceptedAt().Format("2006-01-02T15:04:05Z07:00")
		attrs.AcceptedAt = &s
	}
	return server.Resource{
		Type:       "invites",
		ID:         m.ID(),
		Attributes: attrs,
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
