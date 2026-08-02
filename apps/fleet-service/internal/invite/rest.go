package invite

import (
	"time"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

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

// InternalResponse is the plain-JSON (NOT JSON:API) body of
// GET /internal/invites/{inviteID}, matching the convention the other internal
// endpoints use (membership/resource.go:108).
//
// This is the ONLY place outside the fleet-scoped list where the token is
// served. The endpoint is network-restricted; see design §4.5.
type InternalResponse struct {
	InviteID        string  `json:"invite_id"`
	FleetID         string  `json:"fleet_id"`
	FleetName       string  `json:"fleet_name"`
	Email           string  `json:"email"`
	Role            string  `json:"role"`
	Token           string  `json:"token"`
	ExpiresAt       string  `json:"expires_at"`
	AcceptedAt      *string `json:"accepted_at"`
	InvitedByUserID string  `json:"invited_by_user_id"`
}

// TransformInternal builds the internal response. fleetName may be empty when
// the fleet row is unresolvable; the email degrades to a generic subject.
func TransformInternal(m Model, fleetName string) InternalResponse {
	out := InternalResponse{
		InviteID:        m.ID(),
		FleetID:         m.FleetID(),
		FleetName:       fleetName,
		Email:           m.Email(),
		Role:            m.Role(),
		Token:           m.Token(),
		ExpiresAt:       m.ExpiresAt().Format(time.RFC3339),
		InvitedByUserID: m.InvitedByUserID(),
	}
	if m.AcceptedAt() != nil {
		s := m.AcceptedAt().Format(time.RFC3339)
		out.AcceptedAt = &s
	}
	return out
}
