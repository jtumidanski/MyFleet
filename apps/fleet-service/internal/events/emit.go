// Package events provides thin emit helpers that build the canonical event
// Envelope (design §7) from dto-go payload structs and enqueue it in the
// transactional outbox (design A8). Every Emit runs inside the caller's domain
// transaction so the domain write and the outbox row commit/roll back together.
package events

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	dtoevents "github.com/jtumidanski/myfleet/packages/dto-go/events"
	sharedevents "github.com/jtumidanski/myfleet/packages/shared-go/events"
)

// structToMap marshals a dto payload struct to JSON then back into a
// map[string]any, so the outbox envelope's Data carries exactly the dto-go json
// tags (design §7).
func structToMap(v any) (map[string]any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// enqueue builds the canonical Envelope and writes it to the outbox on tx.
func enqueue(tx *gorm.DB, eventType, fleetID, actorID, traceID string, data any) error {
	m, err := structToMap(data)
	if err != nil {
		return err
	}
	env := sharedevents.Envelope{
		EventID:     uuid.NewString(),
		Type:        eventType,
		Version:     1,
		OccurredAt:  time.Now().UTC(),
		FleetID:     fleetID,
		ActorUserID: actorID,
		TraceID:     traceID,
		Data:        m,
	}
	return sharedevents.Enqueue(tx, env)
}

// EmitVehicleCreated enqueues a vehicle.created event.
func EmitVehicleCreated(tx *gorm.DB, fleetID, actorID, traceID string, d dtoevents.VehicleCreatedData) error {
	return enqueue(tx, "vehicle.created", fleetID, actorID, traceID, d)
}

// EmitMaintenanceCompleted enqueues a maintenance.completed event.
func EmitMaintenanceCompleted(tx *gorm.DB, fleetID, actorID, traceID string, d dtoevents.MaintenanceCompletedData) error {
	return enqueue(tx, "maintenance.completed", fleetID, actorID, traceID, d)
}

// EmitFuelLogged enqueues a fuel.logged event.
func EmitFuelLogged(tx *gorm.DB, fleetID, actorID, traceID string, d dtoevents.FuelLoggedData) error {
	return enqueue(tx, "fuel.logged", fleetID, actorID, traceID, d)
}

// EmitMemberInvited enqueues a member.invited event.
func EmitMemberInvited(tx *gorm.DB, fleetID, actorID, traceID string, d dtoevents.MemberInvitedData) error {
	return enqueue(tx, "member.invited", fleetID, actorID, traceID, d)
}

// EmitInviteCreated enqueues an invite.created event. Emitted when an invite row
// is created AND when a resend rotates its token — a resend produces a fresh
// event_id, which is what lets it past the consumer's (event_id, consumer)
// ledger (FR-EVT-4). Distinct from member.invited, which fires on ACCEPT.
func EmitInviteCreated(tx *gorm.DB, fleetID, actorID, traceID string, d dtoevents.InviteCreatedData) error {
	return enqueue(tx, "invite.created", fleetID, actorID, traceID, d)
}

// EmitScheduleOverdue enqueues a schedule.overdue event. The recompute job emits
// this only on the ok/upcoming→overdue transition edge (no human actor).
func EmitScheduleOverdue(tx *gorm.DB, fleetID, actorID, traceID string, d dtoevents.ScheduleOverdueData) error {
	return enqueue(tx, "schedule.overdue", fleetID, actorID, traceID, d)
}
