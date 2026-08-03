package admin_test

import (
	"testing"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin/admintest"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/mileage"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// FR-ADMIN-DATA-3: a soft-deleted mileage record must be absent from the list
// path AND from the total it feeds — the total drives pagination, so a stale
// count renders a page of nothing at the end of the list.
func TestMileageProvider_hidesSoftDeleted(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-1")
	prov := mileage.NewProvider(db)
	page := server.Page{Number: 1, Size: 25}

	before, totalBefore, err := prov.ListByVehicle(f.VehicleID, nil, nil, page)
	if err != nil {
		t.Fatalf("list before: %v", err)
	}
	if len(before) != 1 || totalBefore != 1 {
		t.Fatalf("fixture expected exactly one mileage record, got %d rows / total %d", len(before), totalBefore)
	}

	if err := db.Exec(`UPDATE fleet.mileage_records SET deleted_at = CURRENT_TIMESTAMP,
	                   purge_operation_id = 'op-1' WHERE id = ?`, f.MileageRecordID).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	after, totalAfter, err := prov.ListByVehicle(f.VehicleID, nil, nil, page)
	if err != nil {
		t.Fatalf("list after: %v", err)
	}
	if len(after) != 0 {
		t.Errorf("soft-deleted mileage record is still listed: %d rows", len(after))
	}
	if totalAfter != 0 {
		t.Errorf("soft-deleted mileage record still counts toward the page total: %d", totalAfter)
	}
}
