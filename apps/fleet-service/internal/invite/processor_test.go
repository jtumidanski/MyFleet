package invite

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// stubProvider satisfies the Provider interface for unit tests.
type stubProvider struct {
	byID    map[string]Model
	byToken map[string]Model
	byFleet map[string][]Model
	byEmail map[string][]Model

	redeemableCalls []string

	countByFleet map[string]int64
	lastSince    time.Time
}

func (s *stubProvider) GetByID(_ context.Context, id string) (Model, error) {
	if m, ok := s.byID[id]; ok {
		return m, nil
	}
	return Model{}, ErrNotFound
}

func (s *stubProvider) GetByToken(_ context.Context, token string) (Model, error) {
	if m, ok := s.byToken[token]; ok {
		return m, nil
	}
	return Model{}, ErrNotFound
}

func (s *stubProvider) ListByFleetID(_ context.Context, fleetID string) ([]Model, error) {
	return s.byFleet[fleetID], nil
}

// Records what the processor asked for, so a test can prove the blank-email
// guard short-circuits BEFORE the query rather than relying on the stub to
// return nothing.
func (s *stubProvider) ListRedeemableByEmail(_ context.Context, email string, _ time.Time) ([]Model, error) {
	s.redeemableCalls = append(s.redeemableCalls, email)
	return s.byEmail[strings.ToLower(email)], nil
}

func (s *stubProvider) CountByFleetSince(_ context.Context, fleetID string, since time.Time) (int64, error) {
	s.lastSince = since
	return s.countByFleet[fleetID], nil
}

// mk builds a Model the way one arrives in the code under test at runtime:
// straight out of a database row, via Make. ValidateAccept only ever sees
// Models that came back from the provider, and several cases below deliberately
// exercise a CORRUPT row (blank email) — which Builder.Build now rejects at
// construction, and which by definition could only reach the domain by being
// read back rather than built.
func mk(email string, expires time.Time, accepted *time.Time) Model {
	return Make(Entity{Email: email, ExpiresAt: expires, AcceptedAt: accepted})
}

func newTestProcessor() *Processor {
	return NewProcessor(logrus.New(), &stubProvider{})
}

func TestAccept_rejectsEmailMismatch(t *testing.T) {
	p := newTestProcessor()
	inv := mk("invited@b.com", time.Now().Add(time.Hour), nil)
	if err := p.ValidateAccept(inv, "other@b.com"); !errors.Is(err, server.ErrConflict) {
		t.Fatalf("email mismatch must be 409, got %v", err)
	}
}

func TestAccept_rejectsExpired(t *testing.T) {
	p := newTestProcessor()
	inv := mk("a@b.com", time.Now().Add(-time.Hour), nil)
	if err := p.ValidateAccept(inv, "a@b.com"); !errors.Is(err, server.ErrConflict) {
		t.Fatalf("expired must be 409, got %v", err)
	}
}

func TestAccept_rejectsAlreadyAccepted(t *testing.T) {
	now := time.Now()
	p := newTestProcessor()
	inv := mk("a@b.com", now.Add(time.Hour), &now)
	if err := p.ValidateAccept(inv, "a@b.com"); !errors.Is(err, server.ErrConflict) {
		t.Fatalf("already-accepted must be 409, got %v", err)
	}
}

func TestAccept_okWhenValid(t *testing.T) {
	p := newTestProcessor()
	inv := mk("a@b.com", time.Now().Add(time.Hour), nil)
	if err := p.ValidateAccept(inv, "a@b.com"); err != nil {
		t.Fatalf("valid accept should pass, got %v", err)
	}
}

func TestValidateInviteEmail(t *testing.T) {
	valid := []string{"b@x.com", "first.last+tag@sub.example.co.uk"}
	for _, s := range valid {
		if err := ValidateInviteEmail(s); err != nil {
			t.Fatalf("ValidateInviteEmail(%q) = %v, want nil", s, err)
		}
	}

	// Every one of these is a header-injection or display-name vector, or is
	// simply unsendable. mail.ParseAddress ACCEPTS "Bob <b@x.com>", which is
	// exactly why the addr-spec equality check below it exists.
	invalid := []string{
		"",
		"b@x.com\r\nBcc: victim@x.com",
		"b@x.com\nBcc: victim@x.com",
		"Bob <b@x.com>",
		"not-an-address",
		"@x.com",
		"b@",
	}
	for _, s := range invalid {
		if err := ValidateInviteEmail(s); !errors.Is(err, server.ErrValidation) {
			t.Fatalf("ValidateInviteEmail(%q) = %v, want ErrValidation", s, err)
		}
	}
}

func TestCheckCreateLimit(t *testing.T) {
	now := time.Now()
	sp := &stubProvider{countByFleet: map[string]int64{"f1": 19}}
	p := NewProcessor(logrus.New(), sp)

	if err := p.CheckCreateLimit(context.Background(), "f1", 20, 24*time.Hour, now); err != nil {
		t.Fatalf("19 of 20 must be allowed, got %v", err)
	}

	sp.countByFleet["f1"] = 20
	if err := p.CheckCreateLimit(context.Background(), "f1", 20, 24*time.Hour, now); !errors.Is(err, server.ErrTooManyRequests) {
		t.Fatalf("at the limit must be 429, got %v", err)
	}

	// The window boundary is what the provider is asked for; assert we asked for
	// the right one rather than trusting the count alone.
	if got, want := sp.lastSince, now.Add(-24*time.Hour); !got.Equal(want) {
		t.Fatalf("counted since %v, want %v", got, want)
	}
}

func TestCheckResendCooldown(t *testing.T) {
	now := time.Now()
	p := newTestProcessor()

	// updated_at is stamped by the database (GORM on insert, Resend explicitly),
	// never by the builder, so the cooldown's input is built via Make.
	fresh := Make(Entity{UpdatedAt: now.Add(-time.Minute)})
	if err := p.CheckResendCooldown(fresh, 5*time.Minute, now); !errors.Is(err, server.ErrTooManyRequests) {
		t.Fatalf("1 minute into a 5 minute cooldown must be 429, got %v", err)
	}

	stale := Make(Entity{UpdatedAt: now.Add(-6 * time.Minute)})
	if err := p.CheckResendCooldown(stale, 5*time.Minute, now); err != nil {
		t.Fatalf("6 minutes into a 5 minute cooldown must be allowed, got %v", err)
	}
}

// Make must carry updated_at through to the Model, or the cooldown above reads a
// zero time and never fires.
func TestMake_carriesUpdatedAt(t *testing.T) {
	ts := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	m := Make(Entity{ID: "i1", UpdatedAt: ts})
	if !m.UpdatedAt().Equal(ts) {
		t.Fatalf("UpdatedAt()=%v want %v", m.UpdatedAt(), ts)
	}
	if !m.ToEntity().UpdatedAt.Equal(ts) {
		t.Fatalf("ToEntity dropped UpdatedAt")
	}
}

// The existing four tests above assert only errors.Is(err, server.ErrConflict)
// and must keep passing unmodified: they are the guard that this change did not
// alter the HTTP status contract. These tighten each case to its own sentinel.

func TestValidateAccept_returnsDistinctSentinelPerPrecondition(t *testing.T) {
	now := time.Now()
	p := newTestProcessor()

	cases := []struct {
		name string
		inv  Model
		as   string
		want error
	}{
		{"already accepted", mk("a@b.com", now.Add(time.Hour), &now), "a@b.com", ErrAlreadyAccepted},
		{"expired", mk("a@b.com", now.Add(-time.Hour), nil), "a@b.com", ErrInviteExpired},
		{"email mismatch", mk("invited@b.com", now.Add(time.Hour), nil), "other@b.com", ErrEmailMismatch},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := p.ValidateAccept(c.inv, c.as)
			if !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
			if !errors.Is(err, server.ErrConflict) {
				t.Fatal("every sentinel must still map to 409 (FR-8)")
			}
		})
	}
}

// TestValidateAccept_sentinelsAreMutuallyExclusive proves the sentinels are
// actually distinguishable. Without it, errors that all satisfy
// errors.Is(err, server.ErrConflict) could still be the same value and the
// per-case test above would pass vacuously.
func TestValidateAccept_sentinelsAreMutuallyExclusive(t *testing.T) {
	all := []error{ErrAlreadyAccepted, ErrInviteExpired, ErrEmailMismatch, ErrInviteUnusable}
	for i, a := range all {
		for j, b := range all {
			if i != j && errors.Is(a, b) {
				t.Fatalf("sentinel %d and %d are not distinguishable", i, j)
			}
		}
	}
}

// TestValidateAccept_reportsAlreadyAcceptedBeforeEmailMismatch pins the
// precondition order. It matters for disclosure: a wrong-account caller holding
// a leaked, already-accepted invite learns only "already accepted", never
// "...and it wasn't yours".
func TestValidateAccept_reportsAlreadyAcceptedBeforeEmailMismatch(t *testing.T) {
	now := time.Now()
	p := newTestProcessor()
	inv := mk("invited@b.com", now.Add(time.Hour), &now)

	if err := p.ValidateAccept(inv, "other@b.com"); !errors.Is(err, ErrAlreadyAccepted) {
		t.Fatalf("err = %v, want ErrAlreadyAccepted (order: accepted → expired → email)", err)
	}
}

// TestValidateAccept_rejectsAnInviteWithNoEmail closes a fail-open in the email
// precondition: strings.EqualFold("", "") is TRUE, so a row with a blank email
// would be accepted by ANY authenticated caller holding the token — including
// one carrying the empty `email` claim this branch exists to fix.
//
// resource.go rejects a blank email at invite creation, so this is unreachable
// today. The guard makes it structurally impossible rather than
// impossible-by-another-file's-validation.
func TestValidateAccept_rejectsAnInviteWithNoEmail(t *testing.T) {
	p := newTestProcessor()

	cases := []struct {
		name string
		as   string
	}{
		// The fail-open itself: blank invite email meets blank claim.
		{"caller has no email claim", ""},
		// Also blocked, so the guard does not depend on the caller's claim.
		{"caller has a real email", "anyone@b.com"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := p.ValidateAccept(mk("", time.Now().Add(time.Hour), nil), c.as)
			if !errors.Is(err, ErrInviteUnusable) {
				t.Fatalf("err = %v, want ErrInviteUnusable", err)
			}
			if !errors.Is(err, server.ErrConflict) {
				t.Fatal("the corrupt-row guard must still render 409 (FR-8)")
			}
		})
	}
}

// TestValidateAccept_reportsAlreadyAcceptedBeforeUnusable proves the corrupt-row
// guard was inserted AT the email precondition, not ahead of the earlier two.
// The order accepted → expired → email is a disclosure control: a caller
// presenting an already-accepted invite must learn only "already accepted",
// whatever else is wrong with the row.
func TestValidateAccept_reportsAlreadyAcceptedBeforeUnusable(t *testing.T) {
	now := time.Now()
	p := newTestProcessor()

	if err := p.ValidateAccept(mk("", now.Add(time.Hour), &now), "other@b.com"); !errors.Is(err, ErrAlreadyAccepted) {
		t.Fatalf("err = %v, want ErrAlreadyAccepted", err)
	}
	if err := p.ValidateAccept(mk("", now.Add(-time.Hour), nil), "other@b.com"); !errors.Is(err, ErrInviteExpired) {
		t.Fatalf("err = %v, want ErrInviteExpired", err)
	}
}

// TestInviteSentinelDetails_discloseNoAddress is the FR-10 guard applied to the
// whole sentinel set: every detail string a caller can see must be a fixed
// phrase with no format verb, so no future edit can interpolate an address into
// a response body that a leaked bearer link reaches.
func TestInviteSentinelDetails_discloseNoAddress(t *testing.T) {
	for _, err := range []error{ErrAlreadyAccepted, ErrInviteExpired, ErrEmailMismatch, ErrInviteUnusable} {
		var d interface{ Detail() string }
		if !errors.As(err, &d) {
			t.Fatalf("%v carries no detail", err)
		}
		if strings.ContainsAny(d.Detail(), "@%") {
			t.Fatalf("detail %q may carry an address or a format verb", d.Detail())
		}
	}
}

// The blank-email guard must short-circuit BEFORE the query, not merely return
// an empty result. A blank address folded through LOWER() matches every
// blank-address row, so a token that validates with no `email` claim — a documented
// failure mode, see packages/shared-go/auth/middleware.go — would otherwise be
// handed a listing of corrupt invites belonging to nobody.
func TestListRedeemableForEmail_neverQueriesForABlankEmail(t *testing.T) {
	stub := &stubProvider{byEmail: map[string][]Model{
		"": {mk("", time.Now().Add(time.Hour), nil)},
	}}
	p := NewProcessor(logrus.New(), stub)

	got, err := p.ListRedeemableForEmail(context.Background(), "")
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d invites for a blank email, want 0", len(got))
	}
	if len(stub.redeemableCalls) != 0 {
		t.Fatalf("the provider was queried with %q; the guard must run first", stub.redeemableCalls)
	}
}

func TestListRedeemableForEmail_queriesWithTheAuthenticatedAddress(t *testing.T) {
	stub := &stubProvider{byEmail: map[string][]Model{
		"jane@example.com": {mk("jane@example.com", time.Now().Add(time.Hour), nil)},
	}}
	p := NewProcessor(logrus.New(), stub)

	got, err := p.ListRedeemableForEmail(context.Background(), "jane@example.com")
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d invites, want 1", len(got))
	}
	if len(stub.redeemableCalls) != 1 || stub.redeemableCalls[0] != "jane@example.com" {
		t.Fatalf("provider queried with %q, want the authenticated address", stub.redeemableCalls)
	}
}

// stubAdministrator records what the processor handed the write layer, so the
// tests below can assert on the values the domain COMPUTED rather than on
// whatever ended up in a database.
type stubAdministrator struct {
	inserted  []Model
	insertErr error

	resendCalls []struct {
		inv            Model
		token          string
		expiresAt, now time.Time
		traceID        string
	}
	accepted []Model
	deleted  []string
}

func (s *stubAdministrator) Insert(_ context.Context, m Model, _ string) (Model, error) {
	if s.insertErr != nil {
		return Model{}, s.insertErr
	}
	s.inserted = append(s.inserted, m)
	return m, nil
}

func (s *stubAdministrator) Resend(_ context.Context, inv Model, newToken string, expiresAt, now time.Time, traceID string) (Model, error) {
	s.resendCalls = append(s.resendCalls, struct {
		inv            Model
		token          string
		expiresAt, now time.Time
		traceID        string
	}{inv, newToken, expiresAt, now, traceID})
	e := inv.ToEntity()
	e.Token, e.ExpiresAt, e.UpdatedAt = newToken, expiresAt, now
	return Make(e), nil
}

func (s *stubAdministrator) Delete(_ context.Context, id string) error {
	s.deleted = append(s.deleted, id)
	return nil
}

func (s *stubAdministrator) Accept(_ context.Context, inv Model, _, _ string) (Model, error) {
	s.accepted = append(s.accepted, inv)
	return inv, nil
}

func newWritingProcessor(limits Limits) (*Processor, *stubAdministrator) {
	adm := &stubAdministrator{}
	return NewProcessor(logrus.New(), &stubProvider{}).WithAdministrator(adm).WithLimits(limits), adm
}

// Create owns the role vocabulary check that used to sit in the HTTP handler.
// It must reject before anything is written.
func TestProcessorCreate_rejectsAnUnknownRoleBeforeWriting(t *testing.T) {
	p, adm := newWritingProcessor(Limits{CreatePerWindow: 10, CreateWindow: time.Hour})

	_, err := p.Create(context.Background(), "f1", "a@b.com", "wizard", "owner-1", "trace-1")
	if !errors.Is(err, server.ErrValidation) {
		t.Fatalf("err = %v, want server.ErrValidation", err)
	}
	if len(adm.inserted) != 0 {
		t.Fatalf("an invite was written for an unknown role: %+v", adm.inserted)
	}
}

// Create owns the address check too, and must reject before writing.
func TestProcessorCreate_rejectsAMalformedAddressBeforeWriting(t *testing.T) {
	p, adm := newWritingProcessor(Limits{CreatePerWindow: 10, CreateWindow: time.Hour})

	_, err := p.Create(context.Background(), "f1", "Bob <b@x.com>", "member", "owner-1", "trace-1")
	if !errors.Is(err, server.ErrValidation) {
		t.Fatalf("err = %v, want server.ErrValidation", err)
	}
	if len(adm.inserted) != 0 {
		t.Fatalf("an invite was written for a malformed address: %+v", adm.inserted)
	}
}

// Token minting and expiry computation moved out of the handler; this pins that
// the processor actually does both, and that two invites never share a token.
func TestProcessorCreate_mintsAFreshTokenAndAnExpiry(t *testing.T) {
	p, adm := newWritingProcessor(Limits{CreatePerWindow: 10, CreateWindow: time.Hour})

	before := time.Now()
	first, err := p.Create(context.Background(), "f1", "a@b.com", "member", "owner-1", "trace-1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	second, err := p.Create(context.Background(), "f1", "b@b.com", "member", "owner-1", "trace-1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if first.Token() == "" || first.Token() == second.Token() {
		t.Fatal("Create must mint a distinct, non-empty token per invite")
	}
	if len(first.Token()) != 64 {
		t.Fatalf("token is %d hex chars, want 64 (32 random bytes)", len(first.Token()))
	}
	if got := first.ExpiresAt(); got.Before(before.Add(defaultExpiry)) || got.After(time.Now().Add(defaultExpiry)) {
		t.Fatalf("expires_at = %v, want ~now+%v", got, defaultExpiry)
	}
	if first.FleetID() != "f1" || first.Email() != "a@b.com" ||
		first.Role() != "member" || first.InvitedByUserID() != "owner-1" {
		t.Fatalf("Create lost a field: %+v", first)
	}
	// The administrator, not the handler, is what receives the built model.
	if len(adm.inserted) != 2 {
		t.Fatalf("administrator saw %d inserts, want 2", len(adm.inserted))
	}
	if adm.inserted[0].Token() != first.Token() {
		t.Fatal("the model handed to the administrator is not the one returned")
	}
}

// The rate limit is checked BEFORE a token is minted, so a throttled request
// costs no entropy and no write (FR-RATE-1).
func TestProcessorCreate_overTheWindowLimitWritesNothing(t *testing.T) {
	adm := &stubAdministrator{}
	prov := &stubProvider{countByFleet: map[string]int64{"f1": 20}}
	p := NewProcessor(logrus.New(), prov).WithAdministrator(adm).
		WithLimits(Limits{CreatePerWindow: 20, CreateWindow: 24 * time.Hour})

	_, err := p.Create(context.Background(), "f1", "a@b.com", "member", "owner-1", "trace-1")
	if !errors.Is(err, server.ErrTooManyRequests) {
		t.Fatalf("err = %v, want server.ErrTooManyRequests", err)
	}
	if len(adm.inserted) != 0 {
		t.Fatalf("a throttled request still wrote: %+v", adm.inserted)
	}
}

// FR-RSND-3, moved out of the handler: the accepted check runs BEFORE the
// cooldown, so an accepted invite never reports a cooldown it could never
// satisfy. updated_at here is old enough that the cooldown would otherwise
// pass — proving the order, not just the outcome.
func TestProcessorResend_reportsAcceptedBeforeCooldown(t *testing.T) {
	accepted := time.Now().Add(-time.Hour)
	inv := Make(Entity{
		ID: "i1", FleetID: "f1", Email: "a@b.com", Role: "member", Token: "tok-1",
		ExpiresAt: time.Now().Add(time.Hour), AcceptedAt: &accepted,
		InvitedByUserID: "owner-1", UpdatedAt: time.Now(),
	})
	p, adm := newWritingProcessor(Limits{ResendCooldown: time.Hour})

	if _, err := p.Resend(context.Background(), inv, "trace-1"); !errors.Is(err, server.ErrConflict) {
		t.Fatalf("err = %v, want server.ErrConflict (409), not the 429 the fresh updated_at would give", err)
	}
	if len(adm.resendCalls) != 0 {
		t.Fatalf("an accepted invite reached the administrator: %+v", adm.resendCalls)
	}
}

func TestProcessorResend_insideTheCooldownWritesNothing(t *testing.T) {
	inv := Make(Entity{
		ID: "i1", FleetID: "f1", Email: "a@b.com", Role: "member", Token: "tok-1",
		ExpiresAt: time.Now().Add(time.Hour), InvitedByUserID: "owner-1",
		UpdatedAt: time.Now().Add(-time.Minute),
	})
	p, adm := newWritingProcessor(Limits{ResendCooldown: time.Hour})

	if _, err := p.Resend(context.Background(), inv, "trace-1"); !errors.Is(err, server.ErrTooManyRequests) {
		t.Fatalf("err = %v, want server.ErrTooManyRequests", err)
	}
	if len(adm.resendCalls) != 0 {
		t.Fatalf("a throttled resend reached the administrator: %+v", adm.resendCalls)
	}
}

// The property the resend flow is built around: ONE `now` is computed, used as
// the cooldown clock, handed to the administrator as the value to persist in
// updated_at, and used as the base for the new expiry. The next
// CheckResendCooldown reads exactly that persisted value, so the returned
// updated_at must be provably the one written — not a second time.Now().
func TestProcessorResend_passesOneNowForBothUpdatedAtAndExpiry(t *testing.T) {
	inv := Make(Entity{
		ID: "i1", FleetID: "f1", Email: "a@b.com", Role: "member", Token: "tok-1",
		ExpiresAt: time.Now().Add(time.Hour), InvitedByUserID: "owner-1",
		UpdatedAt: time.Now().Add(-2 * time.Hour),
	})
	p, adm := newWritingProcessor(Limits{ResendCooldown: time.Minute})

	updated, err := p.Resend(context.Background(), inv, "trace-1")
	if err != nil {
		t.Fatalf("Resend: %v", err)
	}
	if len(adm.resendCalls) != 1 {
		t.Fatalf("administrator called %d times, want 1", len(adm.resendCalls))
	}
	call := adm.resendCalls[0]
	if !call.expiresAt.Equal(call.now.Add(defaultExpiry)) {
		t.Fatalf("expires_at %v is not now(%v)+%v — a second time.Now() crept in",
			call.expiresAt, call.now, defaultExpiry)
	}
	if !updated.UpdatedAt().Equal(call.now) {
		t.Fatalf("returned updated_at %v is not the value handed to the administrator (%v); "+
			"the cooldown reads the persisted one, so they must be identical",
			updated.UpdatedAt(), call.now)
	}
	if call.token == "" || call.token == inv.Token() {
		t.Fatalf("Resend must mint a fresh token, got %q", call.token)
	}
	if call.traceID != "trace-1" {
		t.Fatalf("traceID = %q, want trace-1", call.traceID)
	}
}

// Accept enforces the preconditions before writing, and passes the sentinel
// through unchanged so the handler can still tell the cases apart.
func TestProcessorAccept_rejectsAMismatchWithoutWriting(t *testing.T) {
	inv := mk("invited@b.com", time.Now().Add(time.Hour), nil)
	p, adm := newWritingProcessor(Limits{})

	_, err := p.Accept(context.Background(), inv, "user-1", "other@b.com", "trace-1")
	if !errors.Is(err, ErrEmailMismatch) {
		t.Fatalf("err = %v, want ErrEmailMismatch", err)
	}
	if len(adm.accepted) != 0 {
		t.Fatalf("a rejected accept still wrote: %+v", adm.accepted)
	}
}

func TestProcessorAccept_writesWhenThePreconditionsHold(t *testing.T) {
	inv := mk("a@b.com", time.Now().Add(time.Hour), nil)
	p, adm := newWritingProcessor(Limits{})

	if _, err := p.Accept(context.Background(), inv, "user-1", "a@b.com", "trace-1"); err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if len(adm.accepted) != 1 {
		t.Fatalf("administrator saw %d accepts, want 1", len(adm.accepted))
	}
}

// The invite token is a bearer credential: no error the domain returns may
// carry it, or it lands in whatever the caller logs.
func TestProcessorErrors_neverCarryTheToken(t *testing.T) {
	tok := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	inv := Make(Entity{
		ID: "i1", FleetID: "f1", Email: "invited@b.com", Role: "member", Token: tok,
		ExpiresAt: time.Now().Add(time.Hour), InvitedByUserID: "owner-1",
		UpdatedAt: time.Now(),
	})
	p, _ := newWritingProcessor(Limits{ResendCooldown: time.Hour, CreateWindow: time.Hour})

	var errs []error
	_, err := p.Resend(context.Background(), inv, "trace-1")
	errs = append(errs, err)
	_, err = p.Accept(context.Background(), inv, "user-1", "other@b.com", "trace-1")
	errs = append(errs, err)
	_, err = p.Create(context.Background(), "f1", "a@b.com", "member", "owner-1", "trace-1")
	errs = append(errs, err)

	for _, e := range errs {
		if e == nil {
			t.Fatal("expected every call above to fail")
		}
		if strings.Contains(e.Error(), tok) {
			t.Fatalf("error text carries the invite token: %v", e)
		}
	}
}
