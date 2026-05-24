package mediaobject

import (
	"time"

	"github.com/google/uuid"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// Builder constructs a valid media-object Model. Build() returns (Model, error)
// because fleetID, uploadedByUserID, bucket, and objectKey are invariants
// enforced at construction time. New objects start in the uploaded state.
type Builder struct{ m Model }

func NewBuilder() *Builder {
	return &Builder{m: Model{
		id:        uuid.NewString(),
		status:    StatusUploaded,
		createdAt: time.Now().UTC(),
	}}
}

func (b *Builder) SetID(id string) *Builder                   { b.m.id = id; return b }
func (b *Builder) SetFleetID(fleetID string) *Builder         { b.m.fleetID = fleetID; return b }
func (b *Builder) SetUploadedByUserID(userID string) *Builder { b.m.uploadedByUserID = userID; return b }
func (b *Builder) SetBucket(bucket string) *Builder           { b.m.bucket = bucket; return b }
func (b *Builder) SetObjectKey(key string) *Builder           { b.m.objectKey = key; return b }
func (b *Builder) SetContentType(ct string) *Builder          { b.m.contentType = ct; return b }
func (b *Builder) SetSize(size int64) *Builder                { b.m.size = size; return b }
func (b *Builder) SetOriginalFilename(name string) *Builder   { b.m.originalFilename = name; return b }
func (b *Builder) SetStatus(s Status) *Builder                { b.m.status = s; return b }

// Build validates invariants and returns the model or a validation error (422).
func (b *Builder) Build() (Model, error) {
	if b.m.fleetID == "" || b.m.uploadedByUserID == "" || b.m.bucket == "" || b.m.objectKey == "" {
		return Model{}, server.ErrValidation
	}
	if b.m.status == "" {
		b.m.status = StatusUploaded
	}
	return b.m, nil
}
