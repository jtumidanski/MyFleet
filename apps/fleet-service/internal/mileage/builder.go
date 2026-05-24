package mileage

import (
	"time"

	"github.com/google/uuid"
)

// Builder constructs a valid mileage Model. Build() returns Model (no error)
// because the mileage domain has no hard invariants beyond what callers supply.
type Builder struct{ m Model }

func NewBuilder() *Builder {
	return &Builder{m: Model{id: uuid.NewString(), createdAt: time.Now().UTC()}}
}

func (b *Builder) SetVehicleID(vehicleID string) *Builder    { b.m.vehicleID = vehicleID; return b }
func (b *Builder) SetMileage(mileage int) *Builder           { b.m.mileage = mileage; return b }
func (b *Builder) SetRecordedAt(t time.Time) *Builder        { b.m.recordedAt = t; return b }
func (b *Builder) SetSource(source string) *Builder          { b.m.source = source; return b }
func (b *Builder) SetSourceRefID(ref string) *Builder        { b.m.sourceRefID = ref; return b }
func (b *Builder) SetCreatedByUserID(userID string) *Builder { b.m.createdByUserID = userID; return b }

// Build returns the model with no validation errors — mileage has no hard invariants
// at the builder level (validation is contextual, e.g. flagging by OnAppend).
func (b *Builder) Build() Model { return b.m }
