package mediavariant

import "time"

// Variant is the kind of derived image generated from an original.
type Variant string

const (
	VariantThumbnail Variant = "thumbnail" // max edge 320
	VariantDisplay   Variant = "display"   // max edge 1280
)

// Model is immutable; state changes return new instances.
type Model struct {
	id            string
	mediaObjectID string
	variant       Variant
	objectKey     string
	width         int
	height        int
	contentType   string
	createdAt     time.Time
}

func (m Model) ID() string            { return m.id }
func (m Model) MediaObjectID() string { return m.mediaObjectID }
func (m Model) Variant() Variant      { return m.variant }
func (m Model) ObjectKey() string     { return m.objectKey }
func (m Model) Width() int            { return m.width }
func (m Model) Height() int           { return m.height }
func (m Model) ContentType() string   { return m.contentType }
func (m Model) CreatedAt() time.Time  { return m.createdAt }
