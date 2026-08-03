package authz

import (
	"errors"
	"testing"

	"github.com/jtumidanski/myfleet/packages/shared-go/auth"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

func TestRequireSameFleet_404OnMismatch(t *testing.T) {
	id := auth.Identity{ActiveFleetID: "f1", Role: "owner"}
	if err := RequireSameFleet(id, "f2"); !errors.Is(err, server.ErrNotFound) {
		t.Fatalf("cross-fleet must be 404 (no leak), got %v", err)
	}
}

func TestRequireSameFleet_okOnMatch(t *testing.T) {
	id := auth.Identity{ActiveFleetID: "f1", Role: "viewer"}
	if err := RequireSameFleet(id, "f1"); err != nil {
		t.Fatalf("same fleet should pass, got %v", err)
	}
}

func TestRequireSameFleet_emptyActiveFleetID(t *testing.T) {
	// Empty ActiveFleetID → no active fleet → 404 (not 403, no leak)
	id := auth.Identity{ActiveFleetID: "", Role: "owner"}
	if err := RequireSameFleet(id, "f1"); !errors.Is(err, server.ErrNotFound) {
		t.Fatalf("empty ActiveFleetID must be 404, got %v", err)
	}
}

func TestRequireWrite_viewerForbidden(t *testing.T) {
	if err := RequireWrite(auth.Identity{Role: "viewer"}); !errors.Is(err, server.ErrForbidden) {
		t.Fatalf("viewer write must be 403, got %v", err)
	}
	if err := RequireWrite(auth.Identity{Role: "member"}); err != nil {
		t.Fatalf("member write should pass, got %v", err)
	}
}

func TestRequireWrite_ownerAllowed(t *testing.T) {
	if err := RequireWrite(auth.Identity{Role: "owner"}); err != nil {
		t.Fatalf("owner write must be allowed, got %v", err)
	}
}

func TestRequireOwner_memberForbidden(t *testing.T) {
	if err := RequireOwner(auth.Identity{Role: "member"}); !errors.Is(err, server.ErrForbidden) {
		t.Fatalf("member must be 403, got %v", err)
	}
}

func TestRequireOwner_ownerAllowed(t *testing.T) {
	if err := RequireOwner(auth.Identity{Role: "owner"}); err != nil {
		t.Fatalf("owner must pass, got %v", err)
	}
}

// FR-ADMIN-AUTH-6: 403, not 404. This is the deliberate INVERSE of
// RequireSameFleet's non-disclosure rule — the existence of an admin API is not
// a secret, only the authority to use it.
func TestRequirePlatformAdmin(t *testing.T) {
	if err := RequirePlatformAdmin(auth.Identity{PlatformAdmin: true}); err != nil {
		t.Errorf("an admin must be allowed, got %v", err)
	}
	if err := RequirePlatformAdmin(auth.Identity{}); !errors.Is(err, server.ErrForbidden) {
		t.Errorf("a non-admin must get 403, got %v", err)
	}
	// A fleet role is not a substitute: owner of a fleet is not owner of the
	// platform (FR-ADMIN-AUTH-9's converse).
	if err := RequirePlatformAdmin(auth.Identity{Role: "owner", ActiveFleetID: "f1"}); !errors.Is(err, server.ErrForbidden) {
		t.Errorf("a fleet owner must not be a platform admin, got %v", err)
	}
	if errors.Is(RequirePlatformAdmin(auth.Identity{}), server.ErrNotFound) {
		t.Error("RequirePlatformAdmin must not hide behind a 404 the way RequireSameFleet does")
	}
}
