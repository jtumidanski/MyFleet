package invite

import (
	"errors"
	"testing"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// stubProvider satisfies the Provider interface for unit tests.
type stubProvider struct {
	byID         map[string]Model
	byToken      map[string]Model
	byFleet      map[string][]Model
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

func (s *stubProvider) CountByFleetSince(fleetID string, since time.Time) (int64, error) {
	s.lastSince = since
	return s.countByFleet[fleetID], nil
}

func mk(email string, expires time.Time, accepted *time.Time) Model {
	return NewBuilder().SetEmail(email).SetExpiresAt(expires).setAcceptedAt(accepted).Build()
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

	fresh := NewBuilder().setUpdatedAt(now.Add(-time.Minute)).Build()
	if err := p.CheckResendCooldown(fresh, 5*time.Minute, now); !errors.Is(err, server.ErrTooManyRequests) {
		t.Fatalf("1 minute into a 5 minute cooldown must be 429, got %v", err)
	}

	stale := NewBuilder().setUpdatedAt(now.Add(-6 * time.Minute)).Build()
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
