package admin_test

import (
	"testing"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin/admintest"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/membership"
)

// CountOwners feeds the sole-owner guard. Counting a purged owner would let the
// last LIVE owner remove themselves and leave the fleet ownerless.
func TestMembershipProvider_hidesSoftDeleted(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-1")
	prov := membership.NewProvider(db)

	if n, err := prov.CountOwners(f.FleetID); err != nil || n != 1 {
		t.Fatalf("fixture expected one owner, got %d err %v", n, err)
	}

	if err := db.Exec(`UPDATE fleet.fleet_memberships SET deleted_at = CURRENT_TIMESTAMP,
	                   purge_operation_id = 'op-1' WHERE id = ?`, f.MembershipID).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	if n, err := prov.CountOwners(f.FleetID); err != nil || n != 0 {
		t.Errorf("CountOwners must ignore soft-deleted rows, got %d err %v", n, err)
	}
	if _, err := prov.GetActiveByUserID(f.OwnerUserID); err != membership.ErrNotFound {
		t.Errorf("GetActiveByUserID must ignore soft-deleted rows, got %v", err)
	}
	if _, err := prov.GetByFleetAndUser(f.FleetID, f.OwnerUserID); err != membership.ErrNotFound {
		t.Errorf("GetByFleetAndUser must ignore soft-deleted rows, got %v", err)
	}
	if ms, err := prov.ListByFleetID(f.FleetID); err != nil || len(ms) != 0 {
		t.Errorf("ListByFleetID must ignore soft-deleted rows, got %d err %v", len(ms), err)
	}
	if ms, err := prov.ListActiveByFleetID(f.FleetID); err != nil || len(ms) != 0 {
		t.Errorf("ListActiveByFleetID must ignore soft-deleted rows, got %d err %v", len(ms), err)
	}
}
