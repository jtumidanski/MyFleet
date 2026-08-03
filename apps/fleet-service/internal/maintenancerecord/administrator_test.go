package maintenancerecord

import (
	"testing"
)

// Update's column allow-list is the layer that silently dropped an edited
// category: the map named six columns and category_id was not among them, so
// the write was a no-op. It looked like it worked because Update returns a
// Model built from the in-memory entity rather than a re-read — the response
// carried the new category while the row kept the old one.
//
// So this asserts against a RE-READ through the Provider, not against Update's
// return value. Asserting on the return would have passed even before the fix.
func TestUpdate_persistsAReassignedCategory(t *testing.T) {
	db := newTestDB(t)
	admin := NewAdministrator(db)
	provider := NewProvider(db)

	id := insertRecord(t, admin, "v1", "cat-original", 5, 0)

	before, err := provider.GetByID(id)
	if err != nil {
		t.Fatalf("get before: %v", err)
	}
	if _, err := admin.Update(before.WithCategoryID("cat-reassigned")); err != nil {
		t.Fatalf("update: %v", err)
	}

	after, err := provider.GetByID(id)
	if err != nil {
		t.Fatalf("get after: %v", err)
	}
	if after.CategoryID() != "cat-reassigned" {
		t.Errorf("persisted CategoryID() = %q, want %q", after.CategoryID(), "cat-reassigned")
	}
}

// Adding a column to the allow-list is easy to do destructively (a typo'd key
// writes a zero value over a neighbouring column), so pin the fields that were
// already there.
func TestUpdate_leavesTheOtherEditableFieldsIntact(t *testing.T) {
	db := newTestDB(t)
	admin := NewAdministrator(db)
	provider := NewProvider(db)

	id := insertRecord(t, admin, "v1", "cat-original", 5, 0)
	seed, err := provider.GetByID(id)
	if err != nil {
		t.Fatalf("get seed: %v", err)
	}
	if _, err := admin.Update(seed.
		WithVendor("Corner Garage").
		WithNotes("torque to spec").
		WithMileage(84999).
		WithCost(129.5).
		WithDescription("  front pads  ")); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := provider.GetByID(id)
	if err != nil {
		t.Fatalf("get after: %v", err)
	}
	if got.Vendor() != "Corner Garage" {
		t.Errorf("Vendor() = %q", got.Vendor())
	}
	if got.Notes() != "torque to spec" {
		t.Errorf("Notes() = %q", got.Notes())
	}
	if got.Mileage() != 84999 {
		t.Errorf("Mileage() = %d", got.Mileage())
	}
	if got.Cost() != 129.5 {
		t.Errorf("Cost() = %v", got.Cost())
	}
	// WithDescription trims, so storage and measurement agree.
	if got.Description() != "front pads" {
		t.Errorf("Description() = %q, want %q", got.Description(), "front pads")
	}
	// Untouched by this update.
	if got.CategoryID() != "cat-original" {
		t.Errorf("CategoryID() = %q, want it unchanged", got.CategoryID())
	}
}
