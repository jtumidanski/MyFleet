package events

import (
	"context"
	"time"
)

// Envelope is the canonical event shape (design §7), mirrored in dto-go.
type Envelope struct {
	EventID     string         `json:"event_id"`
	Type        string         `json:"type"`
	Version     int            `json:"version"`
	OccurredAt  time.Time      `json:"occurred_at"`
	FleetID     string         `json:"fleet_id"`
	ActorUserID string         `json:"actor_user_id"`
	TraceID     string         `json:"trace_id"`
	Data        map[string]any `json:"data"`
}

// Producer publishes events. Domain phases depend on this interface so they can
// run with NoopProducer before Kafka is wired (design §7 last paragraph).
type Producer interface {
	Publish(ctx context.Context, e Envelope) error
}

type NoopProducer struct{}

func (NoopProducer) Publish(context.Context, Envelope) error { return nil }
