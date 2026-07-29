package membership

import "github.com/jtumidanski/myfleet/packages/shared-go/server"

// Attributes is the JSON:API attributes payload for a membership.
type Attributes struct {
	FleetID string `json:"fleetId"`
	UserID  string `json:"userId"`
	Role    string `json:"role"`
	Status  string `json:"status"`
}

// Transform converts a Model to a JSON:API Resource.
func Transform(m Model) server.Resource {
	return server.Resource{
		Type: "memberships",
		ID:   m.ID(),
		Attributes: Attributes{
			FleetID: m.FleetID(),
			UserID:  m.UserID(),
			Role:    m.Role(),
			Status:  m.Status(),
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

// ActiveResponse is the response shape for the internal membership endpoint.
// Keys match what auth-service membership client expects: fleet_id, role.
type ActiveResponse struct {
	FleetID string `json:"fleet_id"`
	Role    string `json:"role"`
}

// InternalMember is one row of the internal fleet-members endpoint, consumed by
// notification-service for recipient resolution. Snake-case keys match the
// notification-service fleetclient decoder.
type InternalMember struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
}

// TransformInternalMembers projects active memberships onto the internal
// recipient-resolution response shape.
func TransformInternalMembers(ms []Model) []InternalMember {
	out := make([]InternalMember, 0, len(ms))
	for _, m := range ms {
		out = append(out, InternalMember{UserID: m.UserID(), Role: m.Role()})
	}
	return out
}
