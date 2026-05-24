package events

import (
	"context"
	"encoding/json"

	"github.com/segmentio/kafka-go"
)

type KafkaProducer struct{ w *kafka.Writer }

func NewKafkaProducer(brokers []string) *KafkaProducer {
	return &KafkaProducer{w: &kafka.Writer{
		Addr:                   kafka.TCP(brokers...),
		Balancer:               &kafka.Hash{},
		AllowAutoTopicCreation: true,
	}}
}

func (p *KafkaProducer) Publish(ctx context.Context, e Envelope) error {
	payload, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return p.w.WriteMessages(ctx, kafka.Message{
		Topic: e.Type,
		Key:   []byte(e.FleetID),
		Value: payload,
		Headers: []kafka.Header{
			{Key: "X-Correlation-ID", Value: []byte(e.TraceID)},
			{Key: "event_id", Value: []byte(e.EventID)},
		},
	})
}

func (p *KafkaProducer) Close() error { return p.w.Close() }
