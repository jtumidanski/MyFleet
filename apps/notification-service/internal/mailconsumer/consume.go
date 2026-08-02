// Package mailconsumer turns one invite.created event into exactly one invite
// email, idempotently.
//
// It is deliberately SEPARATE from internal/consumer, and not merely a second
// ledger name on it (design §3). The Kafka consumer group is separate too, so
// offsets are independent: a stalled email consumer cannot hold back in-app
// notification offsets, and an SMTP failure cannot cause the in-app
// notification path to be re-run or vice versa — which is FR-MAIL-3's stated
// intent, satisfied more completely than its literal wording.
package mailconsumer

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/packages/shared-go/events"

	"github.com/jtumidanski/myfleet/apps/notification-service/internal/fleetclient"
	"github.com/jtumidanski/myfleet/apps/notification-service/internal/mailer"
)

// consumerName is BOTH the Kafka consumer group and the ledger key, so the two
// cannot drift apart.
const consumerName = "invite-email"

// Topic is the single topic this consumer subscribes to. Deliberately NOT added
// to consumer.Topics: the in-app consumer would only mark-and-skip it.
const Topic = "invite.created"

// Inbox is the idempotency ledger surface (satisfied by *inbox.Store).
type Inbox interface {
	Exists(eventID, consumer string) (bool, error)
	Mark(eventID, consumer string) error
}

// Invites fetches one invite including its token (satisfied by *fleetclient.Client).
type Invites interface {
	Invite(ctx context.Context, inviteID string) (fleetclient.Invite, error)
}

// Consumer sends one invite email per invite.created event.
type Consumer struct {
	log     logrus.FieldLogger
	inbox   Inbox
	invites Invites
	sender  mailer.Sender
	cfg     mailer.Config
	sleep   func(context.Context, time.Duration) error
}

// NewConsumer constructs a Consumer with its collaborators injected.
func NewConsumer(log logrus.FieldLogger, inbox Inbox, invites Invites, sender mailer.Sender, cfg mailer.Config) *Consumer {
	return &Consumer{log: log, inbox: inbox, invites: invites, sender: sender, cfg: cfg, sleep: sleepCtx}
}

// WithSleep replaces the backoff sleep. Tests inject an instant one so the retry
// test asserts the SCHEDULE in microseconds rather than sleeping ~42s in CI.
func (c *Consumer) WithSleep(fn func(context.Context, time.Duration) error) *Consumer {
	c.sleep = fn
	return c
}

// sleepCtx waits d, or returns early when ctx is cancelled so shutdown is prompt.
func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Run blocks consuming Topic in the "invite-email" group until ctx is cancelled.
func (c *Consumer) Run(ctx context.Context, brokers []string) {
	events.Consume(ctx, c.log, brokers, consumerName, Topic, c.Handle)
}

// Handle processes one invite.created event.
//
//  1. Read-only dedupe check on (event_id, "invite-email").
//  2. Skip if sending is disabled — BEFORE any network call (FR-CFG-4).
//  3. Fetch the invite, including the token, over internal HTTP. The token is
//     deliberately absent from the event; fetching it also means a delayed or
//     duplicated delivery mails the CURRENT token after a resend rotation.
//  4. Skip if already accepted or already expired (FR-MAIL-4).
//  5. Send with bounded backoff.
//  6. Mark the ledger.
//
// Mark-after-success means a crash between a successful Send and Mark produces
// one duplicate email on the next delivery. PRD §8 accepts that explicitly:
// marking first would silently DROP mail, which is strictly worse.
func (c *Consumer) Handle(ctx context.Context, e events.Envelope) error {
	log := c.log.WithFields(logrus.Fields{
		"event_id": e.EventID,
		"fleet_id": e.FleetID,
		"trace_id": e.TraceID,
	})

	seen, err := c.inbox.Exists(e.EventID, consumerName)
	if err != nil {
		return fmt.Errorf("dedupe check: %w", err)
	}
	if seen {
		log.Debug("invite email already sent for this event; skipping")
		return nil
	}

	if !c.cfg.Enabled {
		log.Debug("SMTP disabled; recording and skipping")
		mailer.RecordOutcome(mailer.OutcomeSkippedDisabled)
		return c.inbox.Mark(e.EventID, consumerName)
	}

	inviteID, _ := e.Data["invite_id"].(string)
	if inviteID == "" {
		log.Warn("invite.created carries no invite_id; recording and skipping")
		mailer.RecordOutcome(mailer.OutcomeSkippedStale)
		return c.inbox.Mark(e.EventID, consumerName)
	}
	log = log.WithField("invite_id", inviteID)

	inv, err := c.invites.Invite(ctx, inviteID)
	if err != nil {
		// A deleted invite will never come back — retrying is pure waste.
		if errors.Is(err, fleetclient.ErrInviteNotFound) {
			log.Warn("invite no longer exists; recording and skipping")
			mailer.RecordOutcome(mailer.OutcomeSkippedStale)
			return c.inbox.Mark(e.EventID, consumerName)
		}
		// Anything else is transient: leave UNMARKED so a redelivery retries.
		return fmt.Errorf("fetch invite %s: %w", inviteID, err)
	}

	if stale, reason := staleness(inv, time.Now()); stale {
		log.WithField("reason", reason).Info("invite is no longer live; not mailing a dead link")
		mailer.RecordOutcome(mailer.OutcomeSkippedStale)
		return c.inbox.Mark(e.EventID, consumerName)
	}

	msg, err := c.render(inv)
	if err != nil {
		// A render failure is deterministic; retrying reproduces it exactly.
		log.WithError(err).Error("could not render the invite email")
		mailer.RecordOutcome(mailer.OutcomeFailedPermanent)
		return c.inbox.Mark(e.EventID, consumerName)
	}

	c.send(ctx, log, msg)
	return c.inbox.Mark(e.EventID, consumerName)
}

// render builds the message. The accept URL matches the SPA route at
// apps/web/src/pages/InviteAcceptPage.tsx and is built from PUBLIC_WEB_URL,
// never from an inbound request header (FR-TPL-2). PathEscape is a no-op for
// today's 64-hex-char token and a guard if generation ever changes.
func (c *Consumer) render(inv fleetclient.Invite) (mailer.Message, error) {
	expires, err := time.Parse(time.RFC3339, inv.ExpiresAt)
	if err != nil {
		return mailer.Message{}, fmt.Errorf("parse expires_at: %w", err)
	}
	acceptURL := fmt.Sprintf("%s/invites/%s/accept",
		strings.TrimRight(c.cfg.PublicWebURL, "/"), url.PathEscape(inv.Token))

	return mailer.RenderInvite(mailer.InviteData{
		To:        inv.Email,
		FleetName: inv.FleetName,
		Role:      inv.Role,
		AcceptURL: acceptURL,
		ExpiresAt: expires,
	})
}

// send attempts delivery with bounded backoff (design §5.2). It never returns
// an error: on exhaustion the caller marks the ledger regardless.
//
// Marking after exhausted retries means a relay outage longer than the budget
// drops that invite's email permanently. That is chosen because the alternative
// under the events.Consume defect (design §5.1) is not "retry later" — it is
// "return an error into a loop that discards the message anyway", i.e. the same
// mail loss plus an unbounded error rate. FR-UI-1's copy-link is the documented
// recovery path.
//
// The backoff schedule is RetryBase * 4^(attempt-1) → 2s, 8s, 32s by default.
func (c *Consumer) send(ctx context.Context, log logrus.FieldLogger, msg mailer.Message) {
	backoff := c.cfg.RetryBase
	for attempt := 1; attempt <= c.cfg.SendAttempts; attempt++ {
		err := c.sender.Send(ctx, msg)
		if err == nil {
			mailer.RecordOutcome(mailer.OutcomeSent)
			log.WithField("attempts", attempt).Info("invite email sent")
			return
		}

		// A rejected recipient is not retryable; retrying it forever wedges a
		// partition (FR-MAIL-5).
		var perm *mailer.PermanentError
		if errors.As(err, &perm) {
			mailer.RecordOutcome(mailer.OutcomeFailedPermanent)
			log.WithError(err).WithField("attempts", attempt).
				Error("invite email permanently rejected; not retrying")
			return
		}

		if attempt == c.cfg.SendAttempts {
			mailer.RecordOutcome(mailer.OutcomeFailedTransient)
			log.WithError(err).WithField("attempts", attempt).
				Error("invite email gave up after exhausting retries")
			return
		}

		log.WithError(err).WithFields(logrus.Fields{"attempt": attempt, "retry_in": backoff.String()}).
			Warn("invite email send failed; retrying")
		if err := c.sleep(ctx, backoff); err != nil {
			mailer.RecordOutcome(mailer.OutcomeFailedTransient)
			log.WithError(err).Warn("shutting down mid-backoff; invite email abandoned")
			return
		}
		backoff *= 4
	}
}

// staleness reports whether the invite is no longer worth mailing, and why. The
// reason is a fixed label, never interpolated data.
func staleness(inv fleetclient.Invite, now time.Time) (bool, string) {
	if inv.AcceptedAt != nil && *inv.AcceptedAt != "" {
		return true, "already_accepted"
	}
	expires, err := time.Parse(time.RFC3339, inv.ExpiresAt)
	if err != nil {
		return true, "unparseable_expiry"
	}
	if !expires.After(now) {
		return true, "expired"
	}
	return false, ""
}
