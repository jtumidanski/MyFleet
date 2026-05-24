// Package consumer implements notification-service's idempotent event consumers
// (design §7, §8.4). For each subscribed topic it resolves the recipients of the
// event's fleet (via fleet-service's internal members API, D2) and generates one
// in-app notification per recipient, deduplicated by a per-user dedupe_key. The
// (event_id, "notification") ledger row is written ONLY after generation
// succeeds, so a mid-processing failure leaves the event eligible for redelivery
// (mirrors media-service's worker mark-after-success pattern).
package consumer

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/packages/shared-go/events"

	"github.com/jtumidanski/myfleet/apps/notification-service/internal/fleetclient"
	"github.com/jtumidanski/myfleet/apps/notification-service/internal/notification"
)

// consumerName is the ledger key under which this service records processed
// events, so the (event_id, consumer) composite isolates it from other consumers.
const consumerName = "notification"

// Topics is the set of fleet-service topics notification-service subscribes to.
var Topics = []string{
	"schedule.overdue",
	"maintenance.completed",
	"fuel.logged",
	"vehicle.created",
	"member.invited",
}

// Inbox is the idempotency ledger surface (satisfied by *inbox.Store).
type Inbox interface {
	Exists(eventID, consumer string) (bool, error)
	Mark(eventID, consumer string) error
}

// Members resolves a fleet's active recipients (satisfied by *fleetclient.Client).
type Members interface {
	ActiveMembers(ctx context.Context, fleetID string) ([]fleetclient.Member, error)
}

// Generator generates one notification (satisfied by *notification.Processor).
type Generator interface {
	Generate(in notification.GenerateInput) error
}

// Consumer turns one fleet event into per-recipient notifications idempotently.
type Consumer struct {
	log     logrus.FieldLogger
	inbox   Inbox
	members Members
	gen     Generator
}

// NewConsumer constructs a Consumer with its collaborators injected.
func NewConsumer(log logrus.FieldLogger, inbox Inbox, members Members, gen Generator) *Consumer {
	return &Consumer{log: log, inbox: inbox, members: members, gen: gen}
}

// Run blocks consuming the given topic with the "notification" consumer group
// until ctx is cancelled, dispatching each message to Handle.
func (c *Consumer) Run(ctx context.Context, brokers []string, topic string) {
	events.Consume(ctx, c.log, brokers, consumerName, topic, c.Handle)
}

// Handle processes one event with a mark-after-success pattern (design §7):
//
//  1. Read-only dedupe check: if (event_id, "notification") is already recorded,
//     this event was fully handled on a prior delivery — skip and commit.
//  2. Resolve recipients from the event's fleet_id via the internal members API.
//  3. Generate one notification per recipient (each deduped by a per-user key).
//  4. Only AFTER all generation succeeds, record the event. Any failure returns
//     an error WITHOUT recording, so the event is redelivered and retried.
func (c *Consumer) Handle(ctx context.Context, e events.Envelope) error {
	seen, err := c.inbox.Exists(e.EventID, consumerName)
	if err != nil {
		return fmt.Errorf("dedupe check: %w", err)
	}
	if seen {
		c.log.WithField("event_id", e.EventID).Debug("event already processed; skipping")
		return nil
	}

	typ, ok := notificationType(e.Type)
	if !ok {
		// Unsubscribed/unknown type — nothing to do, but record so a redelivery of
		// the same id is skipped cheaply.
		return c.inbox.Mark(e.EventID, consumerName)
	}

	if e.FleetID == "" {
		c.log.WithField("event_id", e.EventID).Warn("event missing fleet_id; recording and skipping")
		return c.inbox.Mark(e.EventID, consumerName)
	}

	recipients, err := c.members.ActiveMembers(ctx, e.FleetID)
	if err != nil {
		return fmt.Errorf("resolve recipients for fleet %s: %w", e.FleetID, err)
	}

	title, body := render(e)
	vehicleID, _ := e.Data["vehicle_id"].(string)

	for _, r := range recipients {
		key := dedupeKey(r.UserID, e)
		if err := c.gen.Generate(notification.GenerateInput{
			UserID:    r.UserID,
			Type:      typ,
			DedupeKey: key,
			Title:     title,
			Body:      body,
			VehicleID: vehicleID,
			FleetID:   e.FleetID,
		}); err != nil {
			return fmt.Errorf("generate notification for user %s: %w", r.UserID, err)
		}
	}

	// Mark processed ONLY after all generation succeeded (mark-after-success).
	if err := c.inbox.Mark(e.EventID, consumerName); err != nil {
		return fmt.Errorf("record processed event: %w", err)
	}
	return nil
}

// notificationType maps an event topic to the notification type stored on the
// row (and used for preference checks). schedule.overdue maps to "overdue";
// activity-type events keep their topic as the type. The bool is false for
// topics this service does not turn into notifications.
func notificationType(eventType string) (string, bool) {
	switch eventType {
	case "schedule.overdue":
		return "overdue", true
	case "maintenance.completed", "fuel.logged", "vehicle.created", "member.invited":
		return eventType, true
	default:
		return "", false
	}
}

// dedupeKey builds the per-user dedupe_key (design A6). It ALWAYS includes the
// recipient's user id so each user gets their own deduplicated copy.
//
//   - schedule.overdue: "<user>:overdue:<scheduleID>:<dueCycle>" — dueCycle is the
//     due-window token carried on the event, so re-firing the SAME overdue cycle
//     (or the reminder safety-net re-deriving the same cycle) dedupes to one row.
//   - activity types:   "<user>:<type>:<eventID>" — one notification per event.
func dedupeKey(userID string, e events.Envelope) string {
	if e.Type == "schedule.overdue" {
		scheduleID, _ := e.Data["schedule_id"].(string)
		dueCycle, _ := e.Data["due_cycle"].(string)
		return OverdueDedupeKey(userID, scheduleID, dueCycle)
	}
	return fmt.Sprintf("%s:%s:%s", userID, e.Type, e.EventID)
}

// OverdueDedupeKey builds the per-user overdue dedupe_key. Exported so the daily
// reminder safety-net builds a byte-identical key for the same (user, schedule,
// cycle), guaranteeing the event path and the reminder cannot double-fire.
func OverdueDedupeKey(userID, scheduleID, dueCycle string) string {
	return fmt.Sprintf("%s:overdue:%s:%s", userID, scheduleID, dueCycle)
}

// render produces a human title/body for an event. Kept deliberately simple; the
// UI can localize/format richer copy from the stored type + ids.
func render(e events.Envelope) (title, body string) {
	switch e.Type {
	case "schedule.overdue":
		return "Maintenance overdue", "A maintenance schedule is overdue."
	case "maintenance.completed":
		return "Maintenance completed", "A maintenance schedule was completed."
	case "fuel.logged":
		return "Fuel logged", "A fuel entry was logged."
	case "vehicle.created":
		return "Vehicle added", "A new vehicle was added to your fleet."
	case "member.invited":
		return "Member invited", "A new member was invited to your fleet."
	default:
		return "Notification", ""
	}
}
