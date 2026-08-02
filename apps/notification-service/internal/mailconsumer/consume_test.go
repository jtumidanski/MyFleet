package mailconsumer

import (
	"context"
	"errors"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	logrustest "github.com/sirupsen/logrus/hooks/test"

	"github.com/jtumidanski/myfleet/packages/shared-go/events"

	"github.com/jtumidanski/myfleet/apps/notification-service/internal/fleetclient"
	"github.com/jtumidanski/myfleet/apps/notification-service/internal/mailer"
)

const testToken = "deadbeefcafe0123"

type fakeInbox struct{ seen map[string]bool }

func (f *fakeInbox) Exists(eventID, consumer string) (bool, error) {
	return f.seen[eventID+":"+consumer], nil
}

func (f *fakeInbox) Mark(eventID, consumer string) error {
	f.seen[eventID+":"+consumer] = true
	return nil
}

type fakeInvites struct {
	inv   fleetclient.Invite
	err   error
	calls int
}

func (f *fakeInvites) Invite(context.Context, string) (fleetclient.Invite, error) {
	f.calls++
	return f.inv, f.err
}

func liveInvite() fleetclient.Invite {
	return fleetclient.Invite{
		InviteID: "inv-1", FleetID: "f1", FleetName: "The Smiths",
		Email: "a@b.com", Role: "member", Token: testToken,
		ExpiresAt:       time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339),
		InvitedByUserID: "u1",
	}
}

func enabledConfig() mailer.Config {
	return mailer.Config{
		Enabled: true, PublicWebURL: "https://myfleet.example.com",
		SendAttempts: 4, RetryBase: 2 * time.Second, FromAddress: "invites@myfleet.example.com",
	}
}

func envelope() events.Envelope {
	return events.Envelope{
		EventID: "evt-1", Type: "invite.created", FleetID: "f1", TraceID: "trace-1",
		Data: map[string]any{"invite_id": "inv-1", "email": "a@b.com", "role": "member"},
	}
}

// newTestConsumer wires a consumer whose sleep is instant, so a backoff test
// asserts the SCHEDULE in microseconds instead of sleeping 42 seconds in CI.
func newTestConsumer(t *testing.T, inv *fakeInvites, s mailer.Sender, cfg mailer.Config) (*Consumer, *fakeInbox, *[]time.Duration) {
	t.Helper()
	ib := &fakeInbox{seen: map[string]bool{}}
	var slept []time.Duration
	c := NewConsumer(logrus.New(), ib, inv, s, cfg).
		WithSleep(func(_ context.Context, d time.Duration) error {
			slept = append(slept, d)
			return nil
		})
	return c, ib, &slept
}

func TestHandle_sendsOneEmailWithAWorkingAcceptLink(t *testing.T) {
	sender := &mailer.FakeSender{}
	c, ib, _ := newTestConsumer(t, &fakeInvites{inv: liveInvite()}, sender, enabledConfig())

	if err := c.Handle(context.Background(), envelope()); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(sender.Sent) != 1 {
		t.Fatalf("want 1 send, got %d", len(sender.Sent))
	}
	got := sender.Sent[0]
	if got.To != "a@b.com" {
		t.Fatalf("To=%q", got.To)
	}
	// FR-TPL-2: the link is built from PUBLIC_WEB_URL and matches the SPA route
	// at apps/web/src/pages/InviteAcceptPage.tsx.
	want := "https://myfleet.example.com/invites/" + testToken + "/accept"
	if !strings.Contains(got.Text, want) || !strings.Contains(got.HTML, want) {
		t.Fatalf("accept URL missing from one of the parts:\n%s\n%s", got.Text, got.HTML)
	}
	if !ib.seen["evt-1:"+consumerName] {
		t.Fatal("the ledger must be marked after a successful send")
	}
}

// Acceptance criterion 3.
func TestHandle_isIdempotentByEventID(t *testing.T) {
	sender := &mailer.FakeSender{}
	c, _, _ := newTestConsumer(t, &fakeInvites{inv: liveInvite()}, sender, enabledConfig())

	for i := 0; i < 2; i++ {
		if err := c.Handle(context.Background(), envelope()); err != nil {
			t.Fatalf("delivery %d: %v", i, err)
		}
	}
	if len(sender.Sent) != 1 {
		t.Fatalf("replaying one event_id must send exactly once, got %d", len(sender.Sent))
	}
}

// FR-CFG-4: the disabled check comes BEFORE the fleet-service fetch, so a
// cluster with no relay configured makes no network calls at all.
func TestHandle_disabledMakesNoCalls(t *testing.T) {
	sender := &mailer.FakeSender{}
	invites := &fakeInvites{inv: liveInvite()}
	cfg := enabledConfig()
	cfg.Enabled = false
	c, ib, _ := newTestConsumer(t, invites, sender, cfg)

	if err := c.Handle(context.Background(), envelope()); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(sender.Sent) != 0 {
		t.Fatal("a disabled mailer must not send")
	}
	if invites.calls != 0 {
		t.Fatal("a disabled mailer must not call fleet-service")
	}
	if !ib.seen["evt-1:"+consumerName] {
		t.Fatal("a skipped event must still be marked, or it is reprocessed forever")
	}
}

// FR-MAIL-4: a delayed redelivery must not mail a dead link.
func TestHandle_skipsAcceptedAndExpired(t *testing.T) {
	accepted := time.Now().UTC().Format(time.RFC3339)
	stale := []struct {
		name string
		inv  fleetclient.Invite
	}{
		{"accepted", func() fleetclient.Invite { i := liveInvite(); i.AcceptedAt = &accepted; return i }()},
		{"expired", func() fleetclient.Invite {
			i := liveInvite()
			i.ExpiresAt = time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
			return i
		}()},
	}
	for _, tc := range stale {
		t.Run(tc.name, func(t *testing.T) {
			sender := &mailer.FakeSender{}
			c, ib, _ := newTestConsumer(t, &fakeInvites{inv: tc.inv}, sender, enabledConfig())
			if err := c.Handle(context.Background(), envelope()); err != nil {
				t.Fatalf("Handle: %v", err)
			}
			if len(sender.Sent) != 0 {
				t.Fatal("a stale invite must not be mailed")
			}
			if !ib.seen["evt-1:"+consumerName] {
				t.Fatal("a skipped event must still be marked")
			}
		})
	}
}

// FR-MAIL-5 transient half + design §5.2: bounded attempts with backoff, then
// mark. Marking on exhaustion is the uncomfortable half — see the plan's note.
func TestHandle_transientFailureRetriesThenGivesUp(t *testing.T) {
	sender := &mailer.FakeSender{Err: errors.New("dial tcp: connection refused")}
	c, ib, slept := newTestConsumer(t, &fakeInvites{inv: liveInvite()}, sender, enabledConfig())

	if err := c.Handle(context.Background(), envelope()); err != nil {
		t.Fatalf("Handle must not return an error after a bounded give-up: %v", err)
	}
	if sender.Calls() != 4 {
		t.Fatalf("want 4 attempts, got %d", sender.Calls())
	}
	want := []time.Duration{2 * time.Second, 8 * time.Second, 32 * time.Second}
	if len(*slept) != len(want) {
		t.Fatalf("want %d backoffs, got %v", len(want), *slept)
	}
	for i, d := range want {
		if (*slept)[i] != d {
			t.Fatalf("backoff[%d]=%v want %v (schedule %v)", i, (*slept)[i], d, *slept)
		}
	}
	if !ib.seen["evt-1:"+consumerName] {
		t.Fatal("exhausted retries must mark the ledger; see design §5.2")
	}
}

// A transient failure that clears must not burn the whole budget.
func TestHandle_transientFailureThatRecoversSendsOnce(t *testing.T) {
	sender := &mailer.FakeSender{Errs: []error{errors.New("timeout"), nil}}
	c, _, slept := newTestConsumer(t, &fakeInvites{inv: liveInvite()}, sender, enabledConfig())

	if err := c.Handle(context.Background(), envelope()); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if sender.Calls() != 2 {
		t.Fatalf("want 2 attempts, got %d", sender.Calls())
	}
	if len(*slept) != 1 {
		t.Fatalf("want 1 backoff, got %v", *slept)
	}
}

// FR-MAIL-5 permanent half: exactly ONE attempt. Retrying a rejected mailbox is
// what wedges a partition.
func TestHandle_permanentFailureAttemptsOnce(t *testing.T) {
	sender := &mailer.FakeSender{Err: &mailer.PermanentError{Err: &textproto.Error{Code: 550, Msg: "no such user"}}}
	c, ib, slept := newTestConsumer(t, &fakeInvites{inv: liveInvite()}, sender, enabledConfig())

	if err := c.Handle(context.Background(), envelope()); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if sender.Calls() != 1 {
		t.Fatalf("want exactly 1 attempt, got %d", sender.Calls())
	}
	if len(*slept) != 0 {
		t.Fatalf("a permanent failure must not back off, got %v", *slept)
	}
	if !ib.seen["evt-1:"+consumerName] {
		t.Fatal("a permanent failure must mark the ledger")
	}
}

// A deleted invite will never come back; four lookups against it are waste.
func TestHandle_inviteNotFoundIsPermanent(t *testing.T) {
	sender := &mailer.FakeSender{}
	invites := &fakeInvites{err: fleetclient.ErrInviteNotFound}
	c, ib, _ := newTestConsumer(t, invites, sender, enabledConfig())

	if err := c.Handle(context.Background(), envelope()); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if invites.calls != 1 {
		t.Fatalf("want 1 lookup, got %d", invites.calls)
	}
	if len(sender.Sent) != 0 {
		t.Fatal("nothing to send")
	}
	if !ib.seen["evt-1:"+consumerName] {
		t.Fatal("a 404 must mark the ledger")
	}
}

// Any other fleet-service failure is transient: return the error UNMARKED so a
// redelivery can still do the work.
func TestHandle_lookupFailureReturnsErrorUnmarked(t *testing.T) {
	sender := &mailer.FakeSender{}
	c, ib, _ := newTestConsumer(t, &fakeInvites{err: errors.New("502")}, sender, enabledConfig())

	if err := c.Handle(context.Background(), envelope()); err == nil {
		t.Fatal("a transient lookup failure must surface as an error")
	}
	if ib.seen["evt-1:"+consumerName] {
		t.Fatal("a transient lookup failure must NOT mark the ledger")
	}
}

// FR-OBS-2 / acceptance criterion 9, made mechanical rather than a hand grep:
// the token appears in no log entry — message OR field — across a full failing
// Handle, which is the path most likely to dump a message.
func TestHandle_neverLogsTheToken(t *testing.T) {
	log, hook := logrustest.NewNullLogger()
	log.SetLevel(logrus.DebugLevel)

	ib := &fakeInbox{seen: map[string]bool{}}
	sender := &mailer.FakeSender{Err: errors.New("dial tcp: connection refused")}
	c := NewConsumer(log, ib, &fakeInvites{inv: liveInvite()}, sender, enabledConfig()).
		WithSleep(func(context.Context, time.Duration) error { return nil })

	if err := c.Handle(context.Background(), envelope()); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	if len(hook.AllEntries()) == 0 {
		t.Fatal("expected the failure path to log something")
	}
	for _, e := range hook.AllEntries() {
		if strings.Contains(e.Message, testToken) {
			t.Fatalf("token leaked into a log message: %s", e.Message)
		}
		for k, v := range e.Data {
			if strings.Contains(strings.ToLower(fmtValue(v)), testToken) {
				t.Fatalf("token leaked into log field %q: %v", k, v)
			}
		}
		// The accept URL contains the token, so it is banned outright.
		if strings.Contains(e.Message, "/invites/") {
			t.Fatalf("an accept URL leaked into a log message: %s", e.Message)
		}
	}
}

func fmtValue(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	if err, ok := v.(error); ok {
		return err.Error()
	}
	return ""
}
