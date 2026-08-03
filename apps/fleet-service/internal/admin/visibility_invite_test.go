package admin_test

import (
	"context"
	"testing"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin/admintest"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/invite"
)

// A purged invite must not be acceptable. GetByToken is the acceptance path's
// only lookup, so a miss here means a token belonging to a purged fleet still
// grants membership.
func TestInviteProvider_hidesSoftDeleted(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-1")
	prov := invite.NewProvider(db)
	token := f.FleetID + "-token"

	if _, err := prov.GetByToken(context.Background(), token); err != nil {
		t.Fatalf("fixture invite should be readable by token: %v", err)
	}

	if err := db.Exec(`UPDATE fleet.fleet_invites SET deleted_at = CURRENT_TIMESTAMP,
	                   purge_operation_id = 'op-1' WHERE id = ?`, f.InviteID).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	if _, err := prov.GetByID(context.Background(), f.InviteID); err != invite.ErrNotFound {
		t.Errorf("GetByID must report a soft-deleted invite as not found, got %v", err)
	}
	if _, err := prov.GetByToken(context.Background(), token); err != invite.ErrNotFound {
		t.Errorf("a soft-deleted invite must not be acceptable by token, got %v", err)
	}
	if is, err := prov.ListByFleetID(context.Background(), f.FleetID); err != nil || len(is) != 0 {
		t.Errorf("ListByFleetID must ignore soft-deleted rows, got %d err %v", len(is), err)
	}
}
