package membership

import (
	"errors"

	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// OwnerCounter is satisfied by both Provider (real) and stubProvider (test).
type OwnerCounter interface{ CountOwners(fleetID string) (int, error) }

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
