package fleet

import (
	"github.com/google/uuid"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// Builder constructs a valid Fleet model. Build() returns (Model, error) because
// fleet name non-emptiness is an invariant enforced at construction time.
type Builder struct{ m Model }

func NewBuilder() *Builder { return &Builder{m: Model{id: uuid.NewString()}} }

func (b *Builder) SetName(name string) *Builder              { b.m.name = name; return b }
func (b *Builder) SetCreatedByUserID(uid string) *Builder    { b.m.createdByUserID = uid; return b }

// Build validates invariants and returns the model or a validation error.
func (b *Builder) Build() (Model, error) {
	if b.m.name == "" {
		return Model{}, server.ErrValidation
	}
	return b.m, nil
}
