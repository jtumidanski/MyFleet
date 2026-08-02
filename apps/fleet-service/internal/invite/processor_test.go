package invite

import (
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

func (s *stubProvider) GetByID(id string) (Model, error) {
	if m, ok := s.byID[id]; ok {
		return m, nil
	}
	return Model{}, ErrNotFound
}

func (s *stubProvider) GetByToken(token string) (Model, error) {
	if m, ok := s.byToken[token]; ok {
		return m, nil
	}
	return Model{}, ErrNotFound
}

func (s *stubProvider) ListByFleetID(fleetID string) ([]Model, error) {
	return s.byFleet[fleetID], nil
}

// Records what the processor asked for, so a test can prove the blank-email
// guard short-circuits BEFORE the query rather than relying on the stub to
// return nothing.
func (s *stubProvider) ListRedeemableByEmail(email string, _ time.Time) ([]Model, error) {
	s.redeemableCalls = append(s.redeemableCalls, email)
	return s.byEmail[strings.ToLower(email)], nil
}

func (s *stubProvider) CountByFleetSince(fleetID string, since time.Time) (int64, error) {
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

	if err := p.CheckCreateLimit("f1", 20, 24*time.Hour, now); err != nil {
		t.Fatalf("19 of 20 must be allowed, got %v", err)
	}

	sp.countByFleet["f1"] = 20
	if err := p.CheckCreateLimit("f1", 20, 24*time.Hour, now); !errors.Is(err, server.ErrTooManyRequests) {
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

	got, err := p.ListRedeemableForEmail("")
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

	got, err := p.ListRedeemableForEmail("jane@example.com")
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
