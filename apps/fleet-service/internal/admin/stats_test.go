package admin_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin/admintest"
)

type stubSource struct {
	key, name string
	n         int
	err       error
}

func (s stubSource) Key() string  { return s.key }
func (s stubSource) Name() string { return s.name }
func (s stubSource) Count(context.Context) (int, error) {
	if s.err != nil {
		return 0, s.err
	}
	return s.n, nil
}

func newStatsProcessor(t *testing.T, db *gorm.DB, sources ...admin.StatsSource) *admin.Processor {
	t.Helper()
	return admin.NewProcessor(logrus.New(), admin.Deps{
		DB:            db,
		Provider:      admin.NewProvider(db),
		Administrator: admin.NewAdministrator(db),
		StatsSources:  sources,
		Now:           func() time.Time { return testNow },
	}, stubTargets{})
}

func TestStats_countsLocalDomainsAndSplitsVehicles(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-1")
	admintest.SeedFleet(t, db, "fleet-2")

	// One vehicle stamped by an admin operation, one deleted by a user. Only
	// the first is "pending purge": a user-deleted vehicle is neither active nor
	// recoverable HERE, so counting it would misstate what the console can undo.
	if err := db.Exec(`UPDATE fleet.vehicles SET deleted_at = ?, purge_operation_id = 'op-1'
	                   WHERE id = ?`, testNow, f.VehicleID).Error; err != nil {
		t.Fatalf("stamp: %v", err)
	}
	if err := db.Exec(`UPDATE fleet.vehicles SET deleted_at = ? WHERE id = ?`,
		testNow, f.SecondVehicleID).Error; err != nil {
		t.Fatalf("user delete: %v", err)
	}

	proc := newStatsProcessor(t, db,
		stubSource{key: "users", name: "auth-service", n: 21},
		stubSource{key: "media_objects", name: "media-service", n: 260},
		stubSource{key: "notifications", name: "notification-service", n: 74},
	)
	got, err := proc.Stats(context.Background())
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if got.Vehicles.Active != 2 {
		t.Errorf("active vehicles = %d, want 2 (fleet-2's pair)", got.Vehicles.Active)
	}
	if got.Vehicles.PendingPurge != 1 {
		t.Errorf("pending purge = %d, want 1 — a user-deleted vehicle is not pending purge",
			got.Vehicles.PendingPurge)
	}
	if v := got.Values["fleets"]; v == nil || *v != 2 {
		t.Errorf("fleets = %v, want 2", v)
	}
	if v := got.Values["users"]; v == nil || *v != 21 {
		t.Errorf("users = %v, want 21", v)
	}
	if len(got.Warnings) != 0 {
		t.Errorf("no source failed, so warnings must be empty: %v", got.Warnings)
	}
}

// FR-ADMIN-STATS-5: a single dead service must not blank the dashboard. The
// count is null and the source is NAMED — not zero, which would read as "there
// is no data" rather than "we could not ask".
func TestStats_unreachableSourceIsNullWithAWarning(t *testing.T) {
	db := admintest.NewDB(t)
	admintest.SeedFleet(t, db, "fleet-1")
	proc := newStatsProcessor(t, db,
		stubSource{key: "users", name: "auth-service", n: 21},
		stubSource{key: "notifications", name: "notification-service", err: errors.New("connection refused")},
	)

	got, err := proc.Stats(context.Background())
	if err != nil {
		t.Fatalf("a dead source must not fail the whole call: %v", err)
	}
	if v, ok := got.Values["notifications"]; !ok || v != nil {
		t.Errorf("an unreachable source must be present and null, got %v (present=%v)", v, ok)
	}
	if v := got.Values["users"]; v == nil || *v != 21 {
		t.Errorf("a healthy source must still report: %v", v)
	}
	if len(got.Warnings) != 1 {
		t.Fatalf("want one warning, got %v", got.Warnings)
	}
	if !strings.Contains(got.Warnings[0], "notification-service") {
		t.Errorf("the warning must name the failing source: %q", got.Warnings[0])
	}
}
