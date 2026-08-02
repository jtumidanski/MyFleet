package invite

import (
	"time"

	"github.com/google/uuid"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// Builder constructs a valid Invite model. Build() returns (Model, error)
// because every field it validates is NOT NULL in fleet.fleet_invites
// (entity.go) and load-bearing for the row to mean anything:
//
//   - fleetID          — which fleet the membership would be minted in
//   - email            — who the invite is for; the accept path matches on it
//   - role             — copied verbatim onto the membership at accept time
//   - token            — the bearer credential the accept route is keyed on; a
//     blank one would collide with every other blank one
//   - invitedByUserID  — the actor recorded on the activity/outbox events
//   - expiresAt        — an invite with no expiry is not an invite
//
// Build deliberately does NOT re-check the email's SYNTAX. ValidateInviteEmail
// owns that (it enforces a bare addr-spec safe to interpolate into a To:
// header); duplicating the rule here would create a second source of truth that
// can drift from the one the resource boundary actually enforces. Build only
// asserts an address is present.
type Builder struct{ m Model }

func NewBuilder() *Builder { return &Builder{m: Model{id: uuid.NewString()}} }

func (b *Builder) SetFleetID(id string) *Builder          { b.m.fleetID = id; return b }
func (b *Builder) SetEmail(email string) *Builder         { b.m.email = email; return b }
func (b *Builder) SetRole(role string) *Builder           { b.m.role = role; return b }
func (b *Builder) SetToken(token string) *Builder         { b.m.token = token; return b }
func (b *Builder) SetExpiresAt(t time.Time) *Builder      { b.m.expiresAt = t; return b }
func (b *Builder) SetInvitedByUserID(uid string) *Builder { b.m.invitedByUserID = uid; return b }

// Build validates the invariants above and returns the Model or
// server.ErrValidation.
//
// It is the CONSTRUCTION path only. A Model read back out of the database goes
// through Make (entity.go), which does not validate — a corrupt row that
// predates this check must still be readable so the domain can reject it
// deliberately (see ErrInviteUnusable in processor.go) rather than failing to
// load at all.
func (b *Builder) Build() (Model, error) {
	if b.m.fleetID == "" {
		return Model{}, server.ErrValidation
	}
	if b.m.email == "" {
		return Model{}, server.ErrValidation
	}
	if b.m.role == "" {
		return Model{}, server.ErrValidation
	}
	if b.m.token == "" {
		return Model{}, server.ErrValidation
	}
	if b.m.invitedByUserID == "" {
		return Model{}, server.ErrValidation
	}
	if b.m.expiresAt.IsZero() {
		return Model{}, server.ErrValidation
	}
	return b.m, nil
}
