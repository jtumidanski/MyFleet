package admin_test

import (
	"testing"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin/admintest"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/maintenancerecord"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// A purged attachment must vanish from the record it hangs off, on both the get
// and the list path — the list path batches documents in a separate query, so
// the two filters are genuinely separate code.
func TestMaintenanceRecordDocuments_hideSoftDeleted(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-1")
	prov := maintenancerecord.NewProvider(db)

	m, err := prov.GetByID(f.MaintenanceRecordID)
	if err != nil {
		t.Fatalf("fixture record should be readable: %v", err)
	}
	if len(m.DocumentMediaIDs()) != 1 {
		t.Fatalf("fixture expected one attached document, got %d", len(m.DocumentMediaIDs()))
	}

	if err := db.Exec(`UPDATE fleet.maintenance_record_documents SET deleted_at = CURRENT_TIMESTAMP,
	                   purge_operation_id = 'op-1' WHERE id = ?`, f.DocumentID).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	m, err = prov.GetByID(f.MaintenanceRecordID)
	if err != nil {
		t.Fatalf("get after soft delete: %v", err)
	}
	if len(m.DocumentMediaIDs()) != 0 {
		t.Errorf("GetByID still returns a soft-deleted document: %v", m.DocumentMediaIDs())
	}

	rows, _, err := prov.ListByVehicle(f.VehicleID, nil, server.Page{Number: 1, Size: 25})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected the record itself to remain listed, got %d", len(rows))
	}
	if len(rows[0].DocumentMediaIDs()) != 0 {
		t.Errorf("ListByVehicle still returns a soft-deleted document: %v", rows[0].DocumentMediaIDs())
	}
}
