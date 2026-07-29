package events

import (
	"context"
	"encoding/json"

	"github.com/segmentio/kafka-go"
	"github.com/sirupsen/logrus"
)

type Handler func(ctx context.Context, e Envelope) error

// Consume reads a topic with a consumer group and invokes h per message.
// Dedup/idempotency is the handler's responsibility (processed_events, design §7).
func Consume(ctx context.Context, log logrus.FieldLogger, brokers []string, group, topic string, h Handler) {
	r := kafka.NewReader(kafka.ReaderConfig{Brokers: brokers, GroupID: group, Topic: topic})
	defer func() {
		// Close commits pending offsets; a failure here means the group may
		// redeliver on restart, which consumers must already tolerate. Worth
		// surfacing, not worth failing shutdown over.
		if err := r.Close(); err != nil {
			log.WithError(err).WithField("topic", topic).Warn("closing kafka reader")
		}
	}()
	for {
		m, err := r.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.WithError(err).Warn("kafka fetch failed")
			continue
		}
		var e Envelope
		if err := json.Unmarshal(m.Value, &e); err != nil {
			log.WithError(err).Error("bad event payload; skipping")
			_ = r.CommitMessages(ctx, m)
			continue
		}
		if err := h(ctx, e); err != nil {
			log.WithError(err).WithField("event_id", e.EventID).Error("handler failed; will retry")
			continue // do not commit → redelivery
		}
		_ = r.CommitMessages(ctx, m)
	}
}
