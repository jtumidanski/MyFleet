package activity

import (
	"time"

	"github.com/google/uuid"
)

// Builder fluently constructs a valid activity Model. The activity feed is
// append-only, so the builder only ever produces new events.
type Builder struct {
	id          string
	fleetID     string
	vehicleID   *string
	actorUserID string
	eventType   string
	payload     []byte
	createdAt   time.Time
}

// NewBuilder returns a Builder seeded with a fresh id and creation time.
func NewBuilder() *Builder {
	return &Builder{id: uuid.NewString(), createdAt: time.Now().UTC()}
}

func (b *Builder) SetFleetID(id string) *Builder     { b.fleetID = id; return b }
func (b *Builder) SetVehicleID(id *string) *Builder  { b.vehicleID = id; return b }
func (b *Builder) SetActorUserID(id string) *Builder { b.actorUserID = id; return b }
func (b *Builder) SetType(t string) *Builder         { b.eventType = t; return b }
func (b *Builder) SetPayload(p []byte) *Builder      { b.payload = p; return b }

// Build assembles the immutable Model.
func (b *Builder) Build() Model {
	return Model{
		id:          b.id,
		fleetID:     b.fleetID,
		vehicleID:   b.vehicleID,
		actorUserID: b.actorUserID,
		eventType:   b.eventType,
		payload:     b.payload,
		createdAt:   b.createdAt,
	}
}
