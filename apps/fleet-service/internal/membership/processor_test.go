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
func (s stubProvider) ListByFleetID(fleetID string) ([]Model, error)       { return nil, nil }
func (s stubProvider) ListActiveByFleetID(fleetID string) ([]Model, error) { return nil, nil }
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

// IsValidRole guards the role vocabulary at the one place a role reaches the
// system from user input: invite creation. An unrecognised value would be
// copied verbatim onto the membership created at accept time, producing a
// membership whose role no authz gate understands.
func TestIsValidRole(t *testing.T) {
	for _, role := range []string{"owner", "member", "viewer"} {
		if !IsValidRole(role) {
			t.Errorf("%q must be a valid role", role)
		}
	}
	for _, role := range []string{"", "admin", "Owner", "superuser", "owner "} {
		if IsValidRole(role) {
			t.Errorf("%q must be rejected", role)
		}
	}
}

// activeMember builds the stub row shape ValidateRoleChange expects. Status is
// explicit because ValidateRoleChange requires "active": Status is vestigial
// today (every row is written active and never changed) and the check exists so
// the guard stays correct if a second status value is ever introduced.
func activeMember(role string) Model {
	return Model{id: "m-" + role, fleetID: "f1", userID: "u-" + role, role: role, status: "active"}
}

func TestValidateRoleChange_rejectsUnknownRole(t *testing.T) {
	stub := stubProvider{byFleetAndUser: map[string]Model{"f1:u-member": activeMember("member")}}
	p := NewProcessor(logrus.New(), stub)

	for _, role := range []string{"admin", "", "Owner", "superuser"} {
		if _, err := p.ValidateRoleChange("f1", "u-member", role); !errors.Is(err, ErrInvalidRole) {
			t.Errorf("role %q must be rejected with ErrInvalidRole, got %v", role, err)
		}
	}
}

// The role check runs BEFORE the lookup: an out-of-range value must cost no
// database round trip, mirroring user.Processor.UpdateTheme.
func TestValidateRoleChange_rejectsUnknownRoleWithoutLookup(t *testing.T) {
	p := NewProcessor(logrus.New(), stubProvider{})
	if _, err := p.ValidateRoleChange("f1", "nobody", "admin"); !errors.Is(err, ErrInvalidRole) {
		t.Fatalf("want ErrInvalidRole before the lookup, got %v", err)
	}
}

func TestValidateRoleChange_notFoundWhenTargetIsNotAMember(t *testing.T) {
	p := NewProcessor(logrus.New(), stubProvider{})
	if _, err := p.ValidateRoleChange("f1", "stranger", "owner"); !errors.Is(err, server.ErrNotFound) {
		t.Fatalf("non-member target must be 404, got %v", err)
	}
}

// A non-active membership is not a target. Today no row is ever written with
// another status, so this asserts intent rather than current behaviour.
func TestValidateRoleChange_notFoundWhenTargetIsNotActive(t *testing.T) {
	inactive := Model{id: "m1", fleetID: "f1", userID: "u1", role: "member", status: "revoked"}
	stub := stubProvider{byFleetAndUser: map[string]Model{"f1:u1": inactive}}
	p := NewProcessor(logrus.New(), stub)

	if _, err := p.ValidateRoleChange("f1", "u1", "owner"); !errors.Is(err, server.ErrNotFound) {
		t.Fatalf("inactive target must be 404, got %v", err)
	}
}

// FR-2.6. The fleet must never be left with zero owners.
func TestValidateRoleChange_rejectsDemotingTheSoleOwner(t *testing.T) {
	stub := stubProvider{
		owners:         1,
		byFleetAndUser: map[string]Model{"f1:u-owner": activeMember("owner")},
	}
	p := NewProcessor(logrus.New(), stub)

	if _, err := p.ValidateRoleChange("f1", "u-owner", "member"); !errors.Is(err, server.ErrConflict) {
		t.Fatalf("demoting the sole owner must be 409, got %v", err)
	}
}

// FR-2.5: multiple owners are permitted, so demoting one of two is fine.
func TestValidateRoleChange_allowsDemotingOneOfTwoOwners(t *testing.T) {
	stub := stubProvider{
		owners:         2,
		byFleetAndUser: map[string]Model{"f1:u-owner": activeMember("owner")},
	}
	p := NewProcessor(logrus.New(), stub)

	if _, err := p.ValidateRoleChange("f1", "u-owner", "member"); err != nil {
		t.Fatalf("demotion with a co-owner must pass, got %v", err)
	}
}

// Promotion never counts owners: adding an owner cannot orphan a fleet.
func TestValidateRoleChange_allowsPromotingWhenThereIsOneOwner(t *testing.T) {
	stub := stubProvider{
		owners:         1,
		byFleetAndUser: map[string]Model{"f1:u-member": activeMember("member")},
	}
	p := NewProcessor(logrus.New(), stub)

	m, err := p.ValidateRoleChange("f1", "u-member", "owner")
	if err != nil {
		t.Fatalf("promotion must pass, got %v", err)
	}
	if m.UserID() != "u-member" || m.Role() != "member" {
		t.Fatalf("must return the target AS IT IS TODAY so the caller can record from_role; got %+v", m)
	}
}

// FR-2.7: a no-op PATCH is a success, not a special case.
func TestValidateRoleChange_allowsSettingTheRoleAlreadyHeld(t *testing.T) {
	stub := stubProvider{
		owners:         1,
		byFleetAndUser: map[string]Model{"f1:u-owner": activeMember("owner")},
	}
	p := NewProcessor(logrus.New(), stub)

	if _, err := p.ValidateRoleChange("f1", "u-owner", "owner"); err != nil {
		t.Fatalf("owner -> owner must be a no-op success, got %v", err)
	}
}

func TestWithRole_returnsANewModelAndLeavesTheOriginalAlone(t *testing.T) {
	original := activeMember("member")
	updated := original.WithRole("owner")

	if updated.Role() != "owner" {
		t.Fatalf("WithRole did not apply the new role: %q", updated.Role())
	}
	if original.Role() != "member" {
		t.Fatalf("WithRole mutated the receiver; Model is immutable. original role = %q", original.Role())
	}
	if updated.ID() != original.ID() || updated.FleetID() != original.FleetID() ||
		updated.UserID() != original.UserID() || updated.Status() != original.Status() {
		t.Fatalf("WithRole changed a field other than role: %+v vs %+v", updated, original)
	}
}
