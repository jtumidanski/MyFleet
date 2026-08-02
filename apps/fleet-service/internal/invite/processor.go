package invite

import (
	"errors"
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

// The three preconditions ValidateAccept enforces, each carrying its own
// JSON:API detail so a 409 tells the caller which one failed. All three wrap
// server.ErrConflict, so StatusFor still renders 409 (FR-8) and any existing
// errors.Is(err, server.ErrConflict) check keeps working.
//
// ErrEmailMismatch's detail deliberately names NEITHER address, and is a
// constant with no format verb so no code path can interpolate one. The invite
// token is a bearer credential — anyone holding the link reaches this endpoint
// — so echoing the invited address would turn a leaked link into a
// who-was-this-for oracle (PRD FR-10).
var (
	ErrAlreadyAccepted = server.Detailed(server.ErrConflict, "invite has already been accepted")
	ErrInviteExpired   = server.Detailed(server.ErrConflict, "invite has expired")
	ErrEmailMismatch   = server.Detailed(server.ErrConflict, "invite was issued to a different account")
)

// ValidateAccept enforces FR-FLEET-3: invite must be for the same email, not
// yet accepted, and not expired. Each violation returns its own sentinel; all
// three render 409.
//
// Order is load-bearing (accepted → expired → email): a wrong-account caller
// presenting an already-accepted invite learns only "already accepted".
func (pr *Processor) ValidateAccept(inv Model, authedEmail string) error {
	if inv.AcceptedAt() != nil {
		return ErrAlreadyAccepted
	}
	if !inv.ExpiresAt().After(time.Now()) {
		return ErrInviteExpired
	}
	if !strings.EqualFold(inv.Email(), authedEmail) {
		return ErrEmailMismatch
	}
	return nil
}
