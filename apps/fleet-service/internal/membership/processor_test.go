package membership

import (
	"errors"
	"testing"

	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// stubProvider satisfies the full Provider interface for tests.
type stubProvider struct {
	owners int
	// byFleetAndUser is keyed by fleetID+":"+userID
	byFleetAndUser map[string]Model
}

func (s stubProvider) GetActiveByUserID(userID string) (Model, error) {
	return Model{}, ErrNotFound
}
func (s stubProvider) ListByFleetID(fleetID string) ([]Model, error) { return nil, nil }
func (s stubProvider) GetByFleetAndUser(fleetID, userID string) (Model, error) {
	if s.byFleetAndUser != nil {
		if m, ok := s.byFleetAndUser[fleetID+":"+userID]; ok {
			return m, nil
		}
	}
	return Model{}, ErrNotFound
}
func (s stubProvider) CountOwners(fleetID string) (int, error) { return s.owners, nil }

func TestRemoveMember_blocksSoleOwnerSelfRemoval(t *testing.T) {
	p := NewProcessor(logrus.New(), stubProvider{owners: 1})
	err := p.ValidateRemoval("f1", "u-owner", "u-owner", "owner")
	if !errors.Is(err, server.ErrConflict) {
		t.Fatalf("sole owner self-removal must be 409, got %v", err)
	}
}

func TestRemoveMember_allowsWhenAnotherOwnerExists(t *testing.T) {
	p := NewProcessor(logrus.New(), stubProvider{owners: 2})
	if err := p.ValidateRemoval("f1", "u-owner", "u-owner", "owner"); err != nil {
		t.Fatalf("removal with co-owner should pass, got %v", err)
	}
}

func TestRequireOwnerInFleet_forbiddenWhenNotFound(t *testing.T) {
	p := NewProcessor(logrus.New(), stubProvider{})
	if err := p.RequireOwnerInFleet("fleet1", "user1"); !errors.Is(err, server.ErrForbidden) {
		t.Fatalf("missing membership must be 403, got %v", err)
	}
}

func TestRequireOwnerInFleet_forbiddenWhenMember(t *testing.T) {
	stub := stubProvider{
		byFleetAndUser: map[string]Model{
			"fleet1:user1": {role: "member"},
		},
	}
	p := NewProcessor(logrus.New(), stub)
	if err := p.RequireOwnerInFleet("fleet1", "user1"); !errors.Is(err, server.ErrForbidden) {
		t.Fatalf("member must be 403, got %v", err)
	}
}

func TestRequireOwnerInFleet_okWhenOwner(t *testing.T) {
	stub := stubProvider{
		byFleetAndUser: map[string]Model{
			"fleet1:user1": {role: "owner"},
		},
	}
	p := NewProcessor(logrus.New(), stub)
	if err := p.RequireOwnerInFleet("fleet1", "user1"); err != nil {
		t.Fatalf("owner must pass, got %v", err)
	}
}
