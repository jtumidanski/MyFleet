package maintenancecategory

import (
	"testing"

	"github.com/google/uuid"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

func seededProvider(t *testing.T) Provider {
	t.Helper()
	db := newTestDB(t)
	if err := Seed(db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return NewProvider(db)
}

func TestList_filtersByKindAndCountsAfterFiltering(t *testing.T) {
	p := seededProvider(t)
	page := server.Page{Number: 1, Size: 100}

	all, allTotal, err := p.List("", "", page)
	if err != nil {
		t.Fatalf("List(all): %v", err)
	}
	if allTotal != 20 || len(all) != 20 {
		t.Fatalf("unfiltered: len=%d total=%d, want 20/20", len(all), allTotal)
	}

	mods, modTotal, err := p.List(KindModification, "", page)
	if err != nil {
		t.Fatalf("List(modification): %v", err)
	}
	if modTotal != 12 || len(mods) != 12 {
		t.Fatalf("modification: len=%d total=%d, want 12/12", len(mods), modTotal)
	}
	for _, m := range mods {
		if m.Kind() != KindModification {
			t.Fatalf("%q leaked into the modification filter with kind %q", m.Name(), m.Kind())
		}
	}

	maint, maintTotal, err := p.List(KindMaintenance, "", page)
	if err != nil {
		t.Fatalf("List(maintenance): %v", err)
	}
	if maintTotal != 8 || len(maint) != 8 {
		t.Fatalf("maintenance: len=%d total=%d, want 8/8", len(maint), maintTotal)
	}
}

// The total must reflect the count AFTER the filter, across more than one page.
func TestList_filteredTotalSurvivesPaging(t *testing.T) {
	p := seededProvider(t)

	first, total, err := p.List(KindModification, "", server.Page{Number: 1, Size: 5})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 12 {
		t.Fatalf("total = %d, want the filtered count 12", total)
	}
	if len(first) != 5 {
		t.Fatalf("page 1 len = %d, want 5", len(first))
	}

	third, _, err := p.List(KindModification, "", server.Page{Number: 3, Size: 5})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(third) != 2 {
		t.Fatalf("page 3 len = %d, want 2", len(third))
	}
}

func TestIDsByKind(t *testing.T) {
	p := seededProvider(t)

	ids, err := p.IDsByKind(KindModification, "")
	if err != nil {
		t.Fatalf("IDsByKind: %v", err)
	}
	if len(ids) != 12 {
		t.Fatalf("len(ids) = %d, want 12", len(ids))
	}
}

// A kind with no rows must yield an empty NON-NIL slice: the record provider
// reads nil as "no filter" and empty-non-nil as "match nothing" (design D3).
func TestIDsByKind_emptyResultIsNonNil(t *testing.T) {
	db := newTestDB(t) // no Seed — the table is empty
	ids, err := NewProvider(db).IDsByKind(KindModification, "")
	if err != nil {
		t.Fatalf("IDsByKind: %v", err)
	}
	if ids == nil {
		t.Fatal("IDsByKind returned nil for an empty result; nil means 'no filter' downstream")
	}
	if len(ids) != 0 {
		t.Fatalf("len(ids) = %d, want 0", len(ids))
	}
}

// TestListScopesToFleet proves a category created by fleet A is invisible to
// fleet B, while system rows (NULL fleet_id) stay visible to both.
func TestListScopesToFleet(t *testing.T) {
	db := newTestDB(t)
	if err := Seed(db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	fleetA := "11111111-1111-1111-1111-111111111111"
	custom := Entity{
		ID:      uuid.NewString(),
		Name:    "Rear Diff Fluid",
		Kind:    string(KindMaintenance),
		FleetID: &fleetA,
	}
	if err := db.Create(&custom).Error; err != nil {
		t.Fatalf("create custom: %v", err)
	}

	p := NewProvider(db)
	page := server.Page{Number: 1, Size: 100}

	aRows, _, err := p.List(KindMaintenance, fleetA, page)
	if err != nil {
		t.Fatalf("list fleet A: %v", err)
	}
	if !containsName(aRows, "Rear Diff Fluid") {
		t.Fatal("fleet A must see its own custom category")
	}
	if !containsName(aRows, "Oil Change") {
		t.Fatal("fleet A must still see system categories")
	}

	bRows, _, err := p.List(KindMaintenance, "22222222-2222-2222-2222-222222222222", page)
	if err != nil {
		t.Fatalf("list fleet B: %v", err)
	}
	if containsName(bRows, "Rear Diff Fluid") {
		t.Fatal("fleet B must NOT see fleet A's custom category")
	}
	if !containsName(bRows, "Oil Change") {
		t.Fatal("fleet B must still see system categories")
	}
}

// TestIDsByKindScopesToFleet proves the record ?kind= filter set is bounded to
// the caller's fleet, so one fleet's custom categories cannot widen another's.
func TestIDsByKindScopesToFleet(t *testing.T) {
	db := newTestDB(t)
	if err := Seed(db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	fleetA := "11111111-1111-1111-1111-111111111111"
	if err := db.Create(&Entity{
		ID:      uuid.NewString(),
		Name:    "Rear Diff Fluid",
		Kind:    string(KindMaintenance),
		FleetID: &fleetA,
	}).Error; err != nil {
		t.Fatalf("create custom: %v", err)
	}

	p := NewProvider(db)
	aIDs, err := p.IDsByKind(KindMaintenance, fleetA)
	if err != nil {
		t.Fatalf("ids fleet A: %v", err)
	}
	bIDs, err := p.IDsByKind(KindMaintenance, "22222222-2222-2222-2222-222222222222")
	if err != nil {
		t.Fatalf("ids fleet B: %v", err)
	}
	if len(aIDs) != len(bIDs)+1 {
		t.Fatalf("fleet A should have exactly one more id than fleet B, got %d vs %d",
			len(aIDs), len(bIDs))
	}
}

// TestList_noActiveFleetSeesSystemOnly proves the degradation path required
// when a caller has no active fleet (removed from their last fleet, or
// mid fleet-switch): List(kind, "", page) must still succeed and return
// exactly the system categories, never fleet A's custom row and never an
// error. This is the scenario visibleTo's empty-fleetID branch exists for —
// on PostgreSQL, binding "" as a uuid parameter fails at bind time, so this
// path must never fall through to "fleet_id = ?" with an empty string.
func TestList_noActiveFleetSeesSystemOnly(t *testing.T) {
	db := newTestDB(t)
	if err := Seed(db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	fleetA := "11111111-1111-1111-1111-111111111111"
	if err := db.Create(&Entity{
		ID:      uuid.NewString(),
		Name:    "Rear Diff Fluid",
		Kind:    string(KindMaintenance),
		FleetID: &fleetA,
	}).Error; err != nil {
		t.Fatalf("create custom: %v", err)
	}

	p := NewProvider(db)
	rows, total, err := p.List(KindMaintenance, "", server.Page{Number: 1, Size: 100})
	if err != nil {
		t.Fatalf("List(no active fleet): %v", err)
	}
	if total != 8 || len(rows) != 8 {
		t.Fatalf("no-active-fleet: len=%d total=%d, want 8/8 (system rows only)", len(rows), total)
	}
	if containsName(rows, "Rear Diff Fluid") {
		t.Fatal("a caller with no active fleet must NOT see fleet A's custom category")
	}
	if !containsName(rows, "Oil Change") {
		t.Fatal("a caller with no active fleet must still see system categories")
	}
}

func containsName(ms []Model, name string) bool {
	for _, m := range ms {
		if m.Name() == name {
			return true
		}
	}
	return false
}
