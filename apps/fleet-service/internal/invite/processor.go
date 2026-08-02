package invite

import (
	"errors"
	"net/mail"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// Processor contains invite business logic.
type Processor struct {
	log logrus.FieldLogger
	p   Provider
}

func NewProcessor(log logrus.FieldLogger, p Provider) *Processor {
	return &Processor{log: log, p: p}
}

// ListByFleet returns all invites for a fleet.
func (pr *Processor) ListByFleet(fleetID string) ([]Model, error) {
	return pr.p.ListByFleetID(fleetID)
}

// GetByID fetches an invite by ID.
func (pr *Processor) GetByID(id string) (Model, error) {
	m, err := pr.p.GetByID(id)
	if errors.Is(err, ErrNotFound) {
		return Model{}, server.ErrNotFound
	}
	return m, err
}

// GetByToken fetches an invite by its token.
func (pr *Processor) GetByToken(token string) (Model, error) {
	m, err := pr.p.GetByToken(token)
	if errors.Is(err, ErrNotFound) {
		return Model{}, server.ErrNotFound
	}
	return m, err
}

// ValidateAccept enforces FR-FLEET-3: invite must be for the same email, not
// yet accepted, and not expired. Any violation → server.ErrConflict (409).
func (pr *Processor) ValidateAccept(inv Model, authedEmail string) error {
	if inv.AcceptedAt() != nil {
		return server.ErrConflict
	}
	if !inv.ExpiresAt().After(time.Now()) {
		return server.ErrConflict
	}
	if !strings.EqualFold(inv.Email(), authedEmail) {
		return server.ErrConflict
	}
	return nil
}

// ValidateInviteEmail enforces that an invite address is a bare addr-spec safe
// to interpolate into a To: header (PRD §8 Security).
//
// CR/LF is checked first and separately so a header-injection attempt fails for
// an unambiguous reason. The a.Address != s comparison is the important half:
// mail.ParseAddress happily accepts "Bob <b@x.com>", whose display name would
// then be attacker-influenced header content. Requiring the input to equal the
// parsed addr-spec makes the header value a closed set of characters.
func ValidateInviteEmail(s string) error {
	if strings.ContainsAny(s, "\r\n") {
		return server.ErrValidation
	}
	a, err := mail.ParseAddress(s)
	if err != nil || a.Address != s {
		return server.ErrValidation
	}
	return nil
}
