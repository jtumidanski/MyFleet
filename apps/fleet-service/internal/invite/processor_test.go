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
