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

func TestRequireWrite_viewerForbidden(t *testing.T) {
	if err := RequireWrite(auth.Identity{Role: "viewer"}); !errors.Is(err, server.ErrForbidden) {
		t.Fatalf("viewer write must be 403, got %v", err)
	}
	if err := RequireWrite(auth.Identity{Role: "member"}); err != nil {
		t.Fatalf("member write should pass, got %v", err)
	}
}
