package notification

import (
	"time"

	"github.com/google/uuid"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// Builder constructs a valid notification Model. Build() returns (Model, error)
// because userID, type, title, and dedupeKey are invariants enforced at
// construction time. fleetID is optional (system/account-level notifications may
// not be fleet-scoped).
type Builder struct{ m Model }

func NewBuilder() *Builder {
	return &Builder{m: Model{
		id:        uuid.NewString(),
		createdAt: time.Now().UTC(),
	}}
}

func (b *Builder) SetID(id string) *Builder         { b.m.id = id; return b }
func (b *Builder) SetUserID(userID string) *Builder { b.m.userID = userID; return b }
func (b *Builder) SetType(typ string) *Builder      { b.m.typ = typ; return b }
func (b *Builder) SetTitle(title string) *Builder   { b.m.title = title; return b }
func (b *Builder) SetBody(body string) *Builder     { b.m.body = body; return b }
func (b *Builder) SetDedupeKey(key string) *Builder { b.m.dedupeKey = key; return b }
func (b *Builder) SetVehicleID(id string) *Builder  { b.m.vehicleID = id; return b }
func (b *Builder) SetFleetID(id string) *Builder    { b.m.fleetID = id; return b }

// Build validates invariants and returns the model or a validation error (422).
func (b *Builder) Build() (Model, error) {
	if b.m.userID == "" || b.m.typ == "" || b.m.title == "" || b.m.dedupeKey == "" {
		return Model{}, server.ErrValidation
	}
	return b.m, nil
}
