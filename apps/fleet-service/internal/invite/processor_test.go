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
	byID    map[string]Model
	byToken map[string]Model
	byFleet map[string][]Model
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

// TestValidateAccept_sentinelsAreMutuallyExclusive proves the three sentinels
// are actually distinguishable. Without it, three errors that all satisfy
// errors.Is(err, server.ErrConflict) could still be the same value and the
// per-case test above would pass vacuously.
func TestValidateAccept_sentinelsAreMutuallyExclusive(t *testing.T) {
	all := []error{ErrAlreadyAccepted, ErrInviteExpired, ErrEmailMismatch}
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
