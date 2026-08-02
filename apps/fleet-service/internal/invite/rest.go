package invite

import (
	"time"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// Attributes is the JSON:API attributes payload for an invite.
//
// FleetName is populated only by the pending listing (TransformPending). An
// invitee has no membership anywhere yet, so they cannot read /fleets/{id} to
// resolve the id themselves — without the name they would be asked to accept an
// invitation to a bare uuid. It is `omitempty` so the owner-facing listing,
// where the fleet is already the page context, is unchanged.
type Attributes struct {
	FleetID         string  `json:"fleetId"`
	FleetName       string  `json:"fleetName,omitempty"`
	Email           string  `json:"email"`
	Role            string  `json:"role"`
	Token           string  `json:"token"`
	ExpiresAt       string  `json:"expiresAt"`
	AcceptedAt      *string `json:"acceptedAt,omitempty"`
	InvitedByUserID string  `json:"invitedByUserId"`
}

// CreateRequest is the flat JSON:API attributes payload of
// POST /fleets/{id}/invites.
//
// It lives here, next to the other transport types, rather than as an anonymous
// struct inline in the handler: rest.go owns "serialization and transformation
// between domain models and JSON:API" for this package, and a named type is
// what makes the request shape referenceable from a test or a second caller.
type CreateRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
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

// TransformPending renders an invite for its recipient, naming the fleet they
// are being asked to join.
func TransformPending(m Model, fleetName string) server.Resource {
	res := Transform(m)
	attrs, _ := res.Attributes.(Attributes)
	attrs.FleetName = fleetName
	res.Attributes = attrs
	return res
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
