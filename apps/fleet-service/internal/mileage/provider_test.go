package mileage

import (
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// seedMileageRow inserts a mileage record; deleted rows get a deleted_at stamp.
func seedMileageRow(t *testing.T, db *gorm.DB, id, vehicleID, source string, miles int, deleted bool) {
	t.Helper()
	var deletedAt any
	if deleted {
		deletedAt = time.Now().UTC()
	}
	if err := db.Exec(`INSERT INTO fleet.mileage_records (id, vehicle_id, mileage, recorded_at, source, created_at, deleted_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, vehicleID, miles, time.Now().UTC(), source, time.Now().UTC(), deletedAt).Error; err != nil {
		t.Fatalf("seed mileage row: %v", err)
	}
}

func seedMixedSources(t *testing.T, db *gorm.DB) {
	t.Helper()
	seedMileageRow(t, db, "m1", "v1", SourceManual, 100, false)
	seedMileageRow(t, db, "m2", "v1", SourceManual, 200, false)
	seedMileageRow(t, db, "f1", "v1", SourceFuel, 300, false)
	seedMileageRow(t, db, "r1", "v1", SourceMaintenance, 400, false)
	seedMileageRow(t, db, "m3", "v1", SourceManual, 500, true)
}

func TestListByVehicle_sourcesFilterAndTotal(t *testing.T) {
	db := newTestDB(t)
	seedMixedSources(t, db)
	prov := NewProvider(db)
	page := server.Page{Number: 1, Size: 25}

	tests := []struct {
		name    string
		sources []string
		want    int
	}{
		{"manual only", []string{SourceManual}, 2},
		{"no filter", nil, 4},
		{"fuel and maintenance", []string{SourceFuel, SourceMaintenance}, 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ms, total, err := prov.ListByVehicle("v1", ListFilter{Sources: tc.sources}, page)
			if err != nil {
				t.Fatalf("ListByVehicle: %v", err)
			}
			if len(ms) != tc.want || total != tc.want {
				t.Errorf("rows/total = %d/%d, want %d/%d", len(ms), total, tc.want, tc.want)
			}
			for _, m := range ms {
				if len(tc.sources) == 0 {
					continue
				}
				ok := false
				for _, s := range tc.sources {
					ok = ok || m.Source() == s
				}
				if !ok {
					t.Errorf("row source %s not in %s", m.Source(), fmt.Sprint(tc.sources))
				}
			}
		})
	}
}
