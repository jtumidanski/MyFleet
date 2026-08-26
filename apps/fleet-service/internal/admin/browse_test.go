package admin_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin/admintest"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/adminclient"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// echoUserResolver resolves every requested id to a synthetic identity, standing
// in for a healthy auth-service.
type echoUserResolver struct{}

func (echoUserResolver) Users(_ context.Context, ids []string) (map[string]adminclient.User, error) {
	out := make(map[string]adminclient.User, len(ids))
	for _, id := range ids {
		out[id] = adminclient.User{ID: id, Email: id + "@example.com", DisplayName: "User " + id}
	}
	return out, nil
}

func (echoUserResolver) ListUsers(_ context.Context, _ server.Page) ([]adminclient.User, int, error) {
	return []adminclient.User{
		{ID: "fleet-1-owner", Email: "fleet-1-owner@example.com", DisplayName: "Owner One"},
	}, 1, nil
}

// failingUserResolver stands in for an unreachable auth-service.
type failingUserResolver struct{}

func (failingUserResolver) Users(context.Context, []string) (map[string]adminclient.User, error) {
	return nil, errors.New("connection refused")
}

func (failingUserResolver) ListUsers(context.Context, server.Page) ([]adminclient.User, int, error) {
	return nil, 0, errors.New("connection refused")
}

func newBrowseProcessorWithUsers(t *testing.T, db *gorm.DB, r admin.UserResolver) *admin.Processor {
	t.Helper()
	return admin.NewProcessor(logrus.New(), admin.Deps{
		// Purge-only: these tests never transfer a vehicle, so the two
		// reassigners are explicitly no-ops rather than nil — NewProcessor
		// refuses nil, because a nil one silently skipped half a transfer.
		MediaReassign:        admin.NoopMediaReassign{},
		NotificationReassign: admin.NoopNotificationReassign{},
		DB:                   db,
		Provider:             admin.NewProvider(db),
		Administrator:        admin.NewAdministrator(db),
		AuthUsers:            r,
		Now:                  func() time.Time { return testNow },
	}, stubTargets{})
}

func newBrowseProcessor(t *testing.T, db *gorm.DB) *admin.Processor {
	t.Helper()
	return newBrowseProcessorWithUsers(t, db, echoUserResolver{})
}

// OQ-4: the default INCLUDES admin-stamped fleets, struck through in the UI. A
// console whose recovery window is invisible by default hides the thing it
// exists to let you undo.
func TestListFleets_deletedFilterTriState(t *testing.T) {
	db := admintest.NewDB(t)
	admintest.SeedFleet(t, db, "fleet-1")
	admintest.SeedFleet(t, db, "fleet-2")
	admintest.SeedFleet(t, db, "fleet-3")
	// fleet-2 is admin-stamped; fleet-3 was deleted through an ordinary product
	// flow and is NOT recoverable through this console.
	db.Exec(`UPDATE fleet.fleets SET deleted_at = ?, purge_operation_id = 'op-1' WHERE id = 'fleet-2'`, testNow)
	db.Exec(`UPDATE fleet.fleets SET deleted_at = ? WHERE id = 'fleet-3'`, testNow)

	proc := newBrowseProcessor(t, db)
	page := server.Page{Number: 1, Size: 25}

	ids := func(f admin.FleetPage) map[string]bool {
		out := map[string]bool{}
		for _, row := range f.Rows {
			out[row.ID] = true
		}
		return out
	}

	got, err := proc.ListFleets(context.Background(), "", admin.DeletedInclude, page)
	if err != nil {
		t.Fatalf("include: %v", err)
	}
	if !ids(got)["fleet-1"] || !ids(got)["fleet-2"] {
		t.Errorf("include must show live and admin-stamped fleets: %v", ids(got))
	}
	if ids(got)["fleet-3"] {
		t.Error("a product-deleted fleet is not recoverable here and must stay hidden")
	}

	got, _ = proc.ListFleets(context.Background(), "", admin.DeletedExclude, page)
	if ids(got)["fleet-2"] {
		t.Error("exclude must hide admin-stamped fleets")
	}
	got, _ = proc.ListFleets(context.Background(), "", admin.DeletedOnly, page)
	if !ids(got)["fleet-2"] || ids(got)["fleet-1"] {
		t.Errorf("only must show exactly the pending set: %v", ids(got))
	}
}

func TestParseDeletedFilter(t *testing.T) {
	if got, _ := admin.ParseDeletedFilter(""); got != admin.DeletedInclude {
		t.Errorf("the default must be include, got %q", got)
	}
	for _, raw := range []string{"include", "exclude", "only"} {
		if _, err := admin.ParseDeletedFilter(raw); err != nil {
			t.Errorf("%q must parse, got %v", raw, err)
		}
	}
	if _, err := admin.ParseDeletedFilter("true"); !errors.Is(err, server.ErrValidation) {
		t.Errorf("an unknown value must be 422, got %v", err)
	}
}

// FR-ADMIN-FLEET-1: the caller is a member of nothing, and sees everything.
// That is the entire point of the admin tier, and the one behaviour a
// mistakenly-copied RequireSameFleet would break silently.
func TestListFleets_returnsFleetsTheCallerIsNotAMemberOf(t *testing.T) {
	db := admintest.NewDB(t)
	admintest.SeedFleet(t, db, "fleet-1")
	admintest.SeedFleet(t, db, "fleet-2")
	proc := newBrowseProcessor(t, db)

	got, err := proc.ListFleets(context.Background(), "", admin.DeletedInclude,
		server.Page{Number: 1, Size: 25})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if got.Total != 2 || len(got.Rows) != 2 {
		t.Fatalf("want both fleets, got %d rows / total %d", len(got.Rows), got.Total)
	}
	for _, row := range got.Rows {
		if row.MemberCount != 1 {
			t.Errorf("%s member count = %d, want 1", row.ID, row.MemberCount)
		}
		if row.VehicleCount != 2 {
			t.Errorf("%s vehicle count = %d, want 2", row.ID, row.VehicleCount)
		}
	}
}

// FR-ADMIN-FLEET-5: a failed user lookup still yields the fleet, with ids and
// empty names, flagged in warnings. Failing the whole request because
// auth-service is slow would make the console useless exactly when an operator
// is trying to diagnose why.
func TestGetFleet_degradesWhenAuthServiceIsUnreachable(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-1")
	proc := newBrowseProcessorWithUsers(t, db, failingUserResolver{})

	got, err := proc.GetFleet(context.Background(), f.FleetID)
	if err != nil {
		t.Fatalf("an unreachable auth-service must not fail the detail view: %v", err)
	}
	if len(got.Members) != 1 {
		t.Fatalf("want one member row, got %d", len(got.Members))
	}
	if got.Members[0].UserID != f.OwnerUserID {
		t.Errorf("the member row must still carry its user id, got %q", got.Members[0].UserID)
	}
	if got.Members[0].Email != "" {
		t.Errorf("an unresolved email must be empty, not invented: %q", got.Members[0].Email)
	}
	if len(got.Warnings) == 0 {
		t.Error("a degraded lookup must be flagged in warnings")
	}
}

// FR-ADMIN-UI-9: the detail counts ARE the blast radius, by construction.
func TestGetFleet_countsAreTheBlastRadius(t *testing.T) {
	db := admintest.NewDB(t)
	f := admintest.SeedFleet(t, db, "fleet-1")
	proc := newBrowseProcessor(t, db)

	got, err := proc.GetFleet(context.Background(), f.FleetID)
	if err != nil {
		t.Fatalf("get fleet: %v", err)
	}
	want, err := proc.BlastRadius(admin.Root{Scope: admin.ScopeFleet, TargetID: f.FleetID})
	if err != nil {
		t.Fatalf("blast radius: %v", err)
	}
	for k, v := range want {
		if got.Counts[k] != v {
			t.Errorf("%s: detail says %d, blast radius says %d", k, got.Counts[k], v)
		}
	}
	if got.Counts["vehicles"] != 2 {
		t.Errorf("vehicles = %d, want 2", got.Counts["vehicles"])
	}
}

func TestGetFleet_unknownIsNotFound(t *testing.T) {
	db := admintest.NewDB(t)
	proc := newBrowseProcessor(t, db)
	if _, err := proc.GetFleet(context.Background(), "nope"); !errors.Is(err, server.ErrNotFound) {
		t.Errorf("want 404, got %v", err)
	}
}

// FR-ADMIN-FLEET-6: identity over HTTP, memberships joined locally.
func TestListUsers_joinsMembershipsLocally(t *testing.T) {
	db := admintest.NewDB(t)
	admintest.SeedFleet(t, db, "fleet-1")
	proc := newBrowseProcessor(t, db)

	got, err := proc.ListUsers(context.Background(), server.Page{Number: 1, Size: 25})
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	if len(got.Rows) != 1 || got.Total != 1 {
		t.Fatalf("want one user row, got %d / total %d", len(got.Rows), got.Total)
	}
	if len(got.Rows[0].Fleets) != 1 || got.Rows[0].Fleets[0].FleetID != "fleet-1" {
		t.Errorf("the owner's membership must be joined locally, got %v", got.Rows[0].Fleets)
	}
}
