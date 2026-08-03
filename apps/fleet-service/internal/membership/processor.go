package membership

import (
	"errors"

	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// ErrInvalidRole is the DOMAIN error for a role outside the vocabulary
// IsValidRole owns. The resource layer renders it as the 422 transport
// envelope; the processor knows nothing about HTTP. Same pairing as
// auth-service's user.ErrInvalidTheme / errThemeValidation.
var ErrInvalidRole = errors.New("invalid membership role")

// OwnerCounter is satisfied by both Provider (real) and stubProvider (test).
type OwnerCounter interface {
	CountOwners(fleetID string) (int, error)
}

// Processor contains membership business logic.
type Processor struct {
	log logrus.FieldLogger
	p   Provider
}

func NewProcessor(log logrus.FieldLogger, p Provider) *Processor {
	return &Processor{log: log, p: p}
}

// RequireOwnerInFleet fetches the actor's membership from the DB and returns
// server.ErrForbidden if the actor is not found or is not an owner. This is the
// authoritative stale-claim guard (design §9) used by all owner-only mutations.
func (pr *Processor) RequireOwnerInFleet(fleetID, userID string) error {
	m, err := pr.p.GetByFleetAndUser(fleetID, userID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return server.ErrForbidden
		}
		return err
	}
	if m.Role() != "owner" {
		return server.ErrForbidden
	}
	return nil
}

// ListMembers returns all memberships in a fleet.
func (pr *Processor) ListMembers(fleetID string) ([]Model, error) {
	return pr.p.ListByFleetID(fleetID)
}

// ListActiveMembers returns the active memberships in a fleet. Used by the
// internal recipient-resolution endpoint (notification-service resolves a
// fleet's recipients via this API rather than a cross-service DB join; D2).
func (pr *Processor) ListActiveMembers(fleetID string) ([]Model, error) {
	return pr.p.ListActiveByFleetID(fleetID)
}

// GetActiveByUserID returns the single active membership for a user, translating
// the package's ErrNotFound into server.ErrNotFound the same way GetMember does.
//
// The internal route used to call the provider directly and repeat that
// translation inline (DOM-13). Keeping it here means the two "resolve a
// membership" paths cannot drift apart in how a missing row is reported.
func (pr *Processor) GetActiveByUserID(userID string) (Model, error) {
	m, err := pr.p.GetActiveByUserID(userID)
	if errors.Is(err, ErrNotFound) {
		return Model{}, server.ErrNotFound
	}
	return m, err
}

// GetMember returns the membership for a specific user in a fleet.
func (pr *Processor) GetMember(fleetID, userID string) (Model, error) {
	m, err := pr.p.GetByFleetAndUser(fleetID, userID)
	if errors.Is(err, ErrNotFound) {
		return Model{}, server.ErrNotFound
	}
	return m, err
}

// ValidateRemoval enforces FR-FLEET-4: an owner cannot remove themselves if they
// are the only owner.
func (pr *Processor) ValidateRemoval(fleetID, actorUserID, targetUserID, targetRole string) error {
	if actorUserID == targetUserID && targetRole == "owner" {
		n, err := pr.p.CountOwners(fleetID)
		if err != nil {
			return err
		}
		if n <= 1 {
			return server.ErrConflict
		}
	}
	return nil
}

// ValidateRoleChange enforces FR-2.3 (role vocabulary), FR-2.4 (target must be
// an active member of this fleet) and FR-2.6 (a fleet is never left with zero
// owners). It returns the target membership AS IT IS TODAY so the caller does
// not re-read it and can record from_role in the activity payload.
//
// The vocabulary check runs before the lookup on purpose: an out-of-range role
// then costs no database round trip. Same ordering as user.Processor.UpdateTheme.
func (pr *Processor) ValidateRoleChange(fleetID, targetUserID, role string) (Model, error) {
	if !IsValidRole(role) {
		return Model{}, ErrInvalidRole
	}
	m, err := pr.p.GetByFleetAndUser(fleetID, targetUserID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Model{}, server.ErrNotFound
		}
		return Model{}, err
	}
	// Status is vestigial today — every row is written "active" and never
	// changed — so this is a statement of intent, not a live branch. It costs
	// one comparison and keeps the guard correct if a second value appears.
	if m.Status() != "active" {
		return Model{}, server.ErrNotFound
	}
	// Only a demotion can orphan a fleet. Promotions never count owners.
	if m.Role() == "owner" && role != "owner" {
		n, err := pr.p.CountOwners(fleetID)
		if err != nil {
			return Model{}, err
		}
		if n <= 1 {
			return Model{}, server.ErrConflict
		}
	}
	return m, nil
}
