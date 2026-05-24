package events

import (
	"encoding/json"
	"testing"
)

func TestEnvelope_roundTrips(t *testing.T) {
	e := Envelope{EventID: "e1", Type: "vehicle.created", Version: 1, FleetID: "f1"}
	b, _ := json.Marshal(e)
	var got Envelope
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Type != "vehicle.created" || got.FleetID != "f1" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}

func TestNoopProducer_neverErrors(t *testing.T) {
	p := NoopProducer{}
	if err := p.Publish(nil, Envelope{Type: "x"}); err != nil {
		t.Fatalf("noop producer must not error: %v", err)
	}
}
