package vehiclemedia

import "github.com/google/uuid"

// Builder constructs a VehicleMedia model. Build() returns Model (no invariants beyond UUID).
type Builder struct{ m Model }

func NewBuilder() *Builder { return &Builder{m: Model{id: uuid.NewString()}} }

func (b *Builder) SetVehicleID(vehicleID string) *Builder { b.m.vehicleID = vehicleID; return b }
func (b *Builder) SetMediaID(mediaID string) *Builder     { b.m.mediaID = mediaID; return b }
func (b *Builder) SetIsPrimary(p bool) *Builder           { b.m.isPrimary = p; return b }
func (b *Builder) SetSortOrder(order int) *Builder        { b.m.sortOrder = order; return b }

// Build returns the constructed model. No invariants are enforced beyond UUID presence.
func (b *Builder) Build() Model { return b.m }
