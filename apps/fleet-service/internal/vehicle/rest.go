package vehicle

import (
	"time"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// Attributes is the JSON:API attributes payload for a vehicle.
//
// Status, LastActivityAt, and NextDue are all DERIVED ON READ (design §10.2) and
// never stored on the entity. They are computed from the vehicle's active
// maintenance-schedule due detail and its last activity time, and are exposed
// read-only here.
type Attributes struct {
	FleetID             string   `json:"fleetId"`
	Nickname            string   `json:"nickname,omitempty"`
	Make                string   `json:"make"`
	Model               string   `json:"model"`
	Trim                string   `json:"trim,omitempty"`
	Year                int      `json:"year"`
	VIN                 string   `json:"vin,omitempty"`
	CurrentMileage      int      `json:"currentMileage,omitempty"`
	PrimaryImageMediaID string   `json:"primaryImageMediaId,omitempty"`
	Notes               string   `json:"notes,omitempty"`
	Status              string   `json:"status,omitempty"`
	LastActivityAt      string   `json:"lastActivityAt,omitempty"` // RFC 3339, UTC
	NextDue             *NextDue `json:"nextDue,omitempty"`
}

// createAttributes is the exact set of fields POST /fleets/{id}/vehicles accepts.
// Named rather than anonymous so a test can assert that no derived attribute is
// bindable — this narrow shape IS the read-only enforcement (FR-8.3, NFR-7): an
// unknown lastActivityAt or nextDue in a request body has nowhere to land.
type createAttributes struct {
	Nickname       string `json:"nickname"`
	Make           string `json:"make"`
	Model          string `json:"model"`
	Trim           string `json:"trim"`
	Year           int    `json:"year"`
	VIN            string `json:"vin"`
	CurrentMileage int    `json:"currentMileage"`
	Notes          string `json:"notes"`
}

// patchAttributes is the exact set of fields PATCH /vehicles/{id} accepts.
// Pointers distinguish "absent" from "set to zero" on a partial update.
type patchAttributes struct {
	Nickname       *string `json:"nickname"`
	CurrentMileage *int    `json:"currentMileage"`
	Notes          *string `json:"notes"`
}

// Transform converts a Model to a JSON:API Resource carrying no derived
// attributes. Used by the write paths (create, update, restore, primary-image):
// those responses echo a write, and none of the derived values is a property of
// the write.
func Transform(m Model) server.Resource {
	return TransformDerived(m, Derived{})
}

// TransformDerived converts a Model to a JSON:API Resource, attaching the
// read-only values derived on read.
//
// LastActivityAt is carried as a string rather than a time.Time because
// encoding/json's omitempty has no effect on a struct: a time.Time field would
// emit "0001-01-01T00:00:00Z" for the absent case and defeat FR-8.4's
// "omitted, not zero-valued" contract.
func TransformDerived(m Model, d Derived) server.Resource {
	lastActivity := ""
	if !d.LastActivityAt.IsZero() {
		lastActivity = d.LastActivityAt.UTC().Format(time.RFC3339)
	}
	return server.Resource{
		Type: "vehicles",
		ID:   m.ID(),
		Attributes: Attributes{
			FleetID:             m.FleetID(),
			Nickname:            m.Nickname(),
			Make:                m.Make(),
			Model:               m.Model(),
			Trim:                m.Trim(),
			Year:                m.Year(),
			VIN:                 m.VIN(),
			CurrentMileage:      m.CurrentMileage(),
			PrimaryImageMediaID: m.PrimaryImageMediaID(),
			Notes:               m.Notes(),
			Status:              d.Status,
			LastActivityAt:      lastActivity,
			NextDue:             d.NextDue,
		},
	}
}

// TransformSlice converts a slice of Models to JSON:API Resources (no derived
// attributes).
func TransformSlice(ms []Model) []server.Resource {
	out := make([]server.Resource, 0, len(ms))
	for _, m := range ms {
		out = append(out, Transform(m))
	}
	return out
}
