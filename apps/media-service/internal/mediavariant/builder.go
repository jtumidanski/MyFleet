package mediavariant

import (
	"time"

	"github.com/google/uuid"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// Builder constructs a valid media-variant Model. Build() returns (Model, error)
// because mediaObjectID, variant, and objectKey are invariants.
type Builder struct{ m Model }

func NewBuilder() *Builder {
	return &Builder{m: Model{id: uuid.NewString(), createdAt: time.Now().UTC()}}
}

func (b *Builder) SetMediaObjectID(id string) *Builder { b.m.mediaObjectID = id; return b }
func (b *Builder) SetVariant(v Variant) *Builder       { b.m.variant = v; return b }
func (b *Builder) SetObjectKey(key string) *Builder    { b.m.objectKey = key; return b }
func (b *Builder) SetWidth(w int) *Builder             { b.m.width = w; return b }
func (b *Builder) SetHeight(h int) *Builder            { b.m.height = h; return b }
func (b *Builder) SetContentType(ct string) *Builder   { b.m.contentType = ct; return b }

// Build validates invariants and returns the model or a validation error (422).
func (b *Builder) Build() (Model, error) {
	if b.m.mediaObjectID == "" || b.m.variant == "" || b.m.objectKey == "" {
		return Model{}, server.ErrValidation
	}
	return b.m, nil
}
