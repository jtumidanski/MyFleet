package consumer

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/apps/notification-service/internal/fleetclient"
	"github.com/jtumidanski/myfleet/apps/notification-service/internal/notification"
	"github.com/jtumidanski/myfleet/packages/shared-go/events"
)

// fakeInbox is an in-memory processed-events ledger keyed by (eventID, consumer).
type fakeInbox struct {
	seen map[string]bool
}

func (f *fakeInbox) Exists(eventID, consumer string) (bool, error) {
	return f.seen[eventID+":"+consumer], nil
}
func (f *fakeInbox) Mark(eventID, consumer string) error {
	f.seen[eventID+":"+consumer] = true
	return nil
}

// fakeMembers resolves a fixed set of recipients for any fleet.
type fakeMembers struct{ members []fleetclient.Member }

func (f fakeMembers) ActiveMembers(_ context.Context, _ string) ([]fleetclient.Member, error) {
	return f.members, nil
}

// fakeGenerator counts Generate invocations.
type fakeGenerator struct{ calls int }

func (f *fakeGenerator) Generate(_ notification.GenerateInput) error {
	f.calls++
	return nil
}

// TestConsume_idempotentByEventID asserts that delivering the SAME event_id twice
// results in generation being invoked only once (design §7): the first delivery
// generates + marks processed; the redelivery sees the ledger row and skips.
func TestConsume_idempotentByEventID(t *testing.T) {
	inbox := &fakeInbox{seen: map[string]bool{}}
	members := fakeMembers{members: []fleetclient.Member{{UserID: "u1", Role: "owner"}}}
	gen := &fakeGenerator{}

	c := NewConsumer(logrus.New(), inbox, members, gen)

	env := events.Envelope{
		EventID: "evt-1",
		Type:    "vehicle.created",
		FleetID: "fleet-1",
		Data:    map[string]any{"vehicle_id": "veh-1", "fleet_id": "fleet-1"},
	}

	if err := c.Handle(context.Background(), env); err != nil {
		t.Fatalf("first delivery: %v", err)
	}
	if err := c.Handle(context.Background(), env); err != nil {
		t.Fatalf("redelivery: %v", err)
	}

	if gen.calls != 1 {
		t.Fatalf("generation must be invoked once for one recipient across two deliveries, got %d", gen.calls)
	}
}

// TestConsume_oncePerRecipient asserts that one event generates one notification
// per active recipient (idempotency is per event_id, not per recipient).
func TestConsume_oncePerRecipient(t *testing.T) {
	inbox := &fakeInbox{seen: map[string]bool{}}
	members := fakeMembers{members: []fleetclient.Member{
		{UserID: "u1", Role: "owner"},
		{UserID: "u2", Role: "member"},
	}}
	gen := &fakeGenerator{}

	c := NewConsumer(logrus.New(), inbox, members, gen)

	env := events.Envelope{
		EventID: "evt-2",
		Type:    "fuel.logged",
		FleetID: "fleet-1",
		Data:    map[string]any{"vehicle_id": "veh-1"},
	}

	if err := c.Handle(context.Background(), env); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if gen.calls != 2 {
		t.Fatalf("want one generation per recipient (2), got %d", gen.calls)
	}
}
