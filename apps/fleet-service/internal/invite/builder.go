package invite

import (
	"time"

	"github.com/google/uuid"
)

// Builder constructs an Invite model. Build() returns Model (no error) because
// the invite domain does not enforce non-emptiness of fields at build time
// (validation happens at the handler/processor layer). The unexported
// setAcceptedAt is only used in white-box tests within this package.
type Builder struct{ m Model }

func NewBuilder() *Builder { return &Builder{m: Model{id: uuid.NewString()}} }

func (b *Builder) SetFleetID(id string) *Builder          { b.m.fleetID = id; return b }
func (b *Builder) SetEmail(email string) *Builder         { b.m.email = email; return b }
func (b *Builder) SetRole(role string) *Builder           { b.m.role = role; return b }
func (b *Builder) SetToken(token string) *Builder         { b.m.token = token; return b }
func (b *Builder) SetExpiresAt(t time.Time) *Builder      { b.m.expiresAt = t; return b }
func (b *Builder) SetInvitedByUserID(uid string) *Builder { b.m.invitedByUserID = uid; return b }

// setAcceptedAt is unexported — used only by white-box tests in package invite.
func (b *Builder) setAcceptedAt(t *time.Time) *Builder { b.m.acceptedAt = t; return b }

// Build returns the Model. No invariants are enforced here; validation is in
// the handler/processor layer so the builder remains flexible for tests.
func (b *Builder) Build() Model { return b.m }
