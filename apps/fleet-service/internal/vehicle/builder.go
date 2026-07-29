package vehicle

import (
	"github.com/google/uuid"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// Builder constructs a valid Vehicle model. Build() returns (Model, error) because
// make, model, and year are invariants enforced at construction time (design §6).
type Builder struct{ m Model }

func NewBuilder() *Builder { return &Builder{m: Model{id: uuid.NewString()}} }

func (b *Builder) SetFleetID(fleetID string) *Builder   { b.m.fleetID = fleetID; return b }
func (b *Builder) SetNickname(nickname string) *Builder { b.m.nickname = nickname; return b }
func (b *Builder) SetMake(make string) *Builder         { b.m.make = make; return b }
func (b *Builder) SetModel(model string) *Builder       { b.m.model = model; return b }
func (b *Builder) SetTrim(trim string) *Builder         { b.m.trim = trim; return b }
func (b *Builder) SetYear(year int) *Builder            { b.m.year = year; return b }
func (b *Builder) SetVIN(vin string) *Builder           { b.m.vin = vin; return b }
func (b *Builder) SetCurrentMileage(miles int) *Builder { b.m.currentMileage = miles; return b }
func (b *Builder) SetNotes(notes string) *Builder       { b.m.notes = notes; return b }

// Build validates invariants and returns the model or a validation error.
func (b *Builder) Build() (Model, error) {
	if b.m.make == "" || b.m.model == "" || b.m.year == 0 {
		return Model{}, server.ErrValidation
	}
	return b.m, nil
}
