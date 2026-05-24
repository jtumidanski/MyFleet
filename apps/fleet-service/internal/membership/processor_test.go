package membership

import (
	"errors"
	"testing"

	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

type stubProvider struct{ owners int }

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
