package mediavariant

import (
	"context"
	"testing"
	"time"
)

// TestUpsert_insertsThenUpdatesWithoutDuplicating is the contract lazy variant
// generation leans on: calling it twice for the same (media object, variant)
// leaves exactly one row, updated in place. Two rows would make the content
// endpoint's First() pick arbitrarily between them.
func TestUpsert_insertsThenUpdatesWithoutDuplicating(t *testing.T) {
	db := newVariantTestDB(t)
	admin := NewAdministrator(db)

	first, err := NewBuilder().
		SetMediaObjectID("m1").
		SetVariant(VariantCard).
		SetObjectKey("fleet-a/m1/card.jpg").
		SetWidth(768).
		SetHeight(384).
		SetContentType("image/jpeg").
		Build()
	if err != nil {
		t.Fatalf("build first: %v", err)
	}
	if err := admin.Upsert(first); err != nil {
		t.Fatalf("first Upsert: %v", err)
	}

	second, err := NewBuilder().
		SetMediaObjectID("m1").
		SetVariant(VariantCard).
		SetObjectKey("fleet-a/m1/card-regenerated.jpg").
		SetWidth(768).
		SetHeight(400).
		SetContentType("image/png").
		Build()
	if err != nil {
		t.Fatalf("build second: %v", err)
	}
	if err := admin.Upsert(second); err != nil {
		t.Fatalf("second Upsert: %v", err)
	}

	rows, err := NewProvider(db).ListByMediaObject("m1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("after two Upserts got %d rows, want exactly 1", len(rows))
	}
	got := rows[0]
	if got.ObjectKey() != "fleet-a/m1/card-regenerated.jpg" {
		t.Fatalf("ObjectKey = %q, want the second write's key", got.ObjectKey())
	}
	if got.Height() != 400 {
		t.Fatalf("Height = %d, want 400", got.Height())
	}
	if got.ContentType() != "image/png" {
		t.Fatalf("ContentType = %q, want image/png", got.ContentType())
	}
}

// TestUpsert_preservesCreatedAt guards the column-wipe class of defect task-006
// existed to eliminate, in its OnConflict disguise: listing created_at in
// DoUpdates (or using UpdateAll) would silently rewrite the row's age on every
// regeneration. entityguard cannot see this — it recognises .Save( call sites
// only — so this test is the only thing standing behind it.
func TestUpsert_preservesCreatedAt(t *testing.T) {
	db := newVariantTestDB(t)
	admin := NewAdministrator(db)

	first, err := NewBuilder().
		SetMediaObjectID("m1").
		SetVariant(VariantCard).
		SetObjectKey("fleet-a/m1/card.jpg").
		SetContentType("image/jpeg").
		Build()
	if err != nil {
		t.Fatalf("build first: %v", err)
	}
	if err := admin.Upsert(first); err != nil {
		t.Fatalf("first Upsert: %v", err)
	}
	stored, err := NewProvider(db).GetByMediaObjectAndVariant(context.Background(), "m1", VariantCard)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	originalCreatedAt := stored.CreatedAt()

	// Make the second model's own createdAt provably later, so "unchanged"
	// cannot pass by coincidence of clock resolution.
	time.Sleep(10 * time.Millisecond)
	second, err := NewBuilder().
		SetMediaObjectID("m1").
		SetVariant(VariantCard).
		SetObjectKey("fleet-a/m1/card-regenerated.jpg").
		SetContentType("image/jpeg").
		Build()
	if err != nil {
		t.Fatalf("build second: %v", err)
	}
	if second.CreatedAt().Equal(originalCreatedAt) {
		t.Fatal("test is not meaningful: the second model's createdAt matches the first")
	}
	if err := admin.Upsert(second); err != nil {
		t.Fatalf("second Upsert: %v", err)
	}

	after, err := NewProvider(db).GetByMediaObjectAndVariant(context.Background(), "m1", VariantCard)
	if err != nil {
		t.Fatalf("read back after update: %v", err)
	}
	if !after.CreatedAt().Equal(originalCreatedAt) {
		t.Fatalf("created_at moved from %v to %v; the upsert must not rewrite it",
			originalCreatedAt, after.CreatedAt())
	}
}

// TestUpsert_leavesOtherVariantsUntouched is the regression guard for the single
// most damaging way to get lazy generation wrong: reaching for
// ReplaceForMediaObject, which deletes every row for the media object first and
// would destroy the thumbnail and display this object already has.
func TestUpsert_leavesOtherVariantsUntouched(t *testing.T) {
	db := newVariantTestDB(t)
	admin := NewAdministrator(db)

	seeded := make([]Model, 0, 2)
	for _, spec := range []struct {
		v   Variant
		key string
	}{
		{VariantThumbnail, "fleet-a/m1/thumbnail.jpg"},
		{VariantDisplay, "fleet-a/m1/display.jpg"},
	} {
		m, err := NewBuilder().
			SetMediaObjectID("m1").
			SetVariant(spec.v).
			SetObjectKey(spec.key).
			SetContentType("image/jpeg").
			Build()
		if err != nil {
			t.Fatalf("build %s: %v", spec.v, err)
		}
		seeded = append(seeded, m)
	}
	if err := admin.ReplaceForMediaObject("m1", seeded); err != nil {
		t.Fatalf("seed: %v", err)
	}

	card, err := NewBuilder().
		SetMediaObjectID("m1").
		SetVariant(VariantCard).
		SetObjectKey("fleet-a/m1/card.jpg").
		SetContentType("image/jpeg").
		Build()
	if err != nil {
		t.Fatalf("build card: %v", err)
	}
	if err := admin.Upsert(card); err != nil {
		t.Fatalf("Upsert card: %v", err)
	}

	rows, err := NewProvider(db).ListByMediaObject("m1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	kinds := map[Variant]bool{}
	for _, r := range rows {
		kinds[r.Variant()] = true
	}
	for _, want := range []Variant{VariantThumbnail, VariantCard, VariantDisplay} {
		if !kinds[want] {
			t.Fatalf("after Upsert the %s row is gone; rows present: %v", want, kinds)
		}
	}
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(rows))
	}
}

// A different variant of the same object is a different row, not a conflict.
func TestUpsert_scopesConflictToTheVariant(t *testing.T) {
	db := newVariantTestDB(t)
	admin := NewAdministrator(db)

	for _, v := range []Variant{VariantThumbnail, VariantCard} {
		m, err := NewBuilder().
			SetMediaObjectID("m1").
			SetVariant(v).
			SetObjectKey("fleet-a/m1/" + string(v) + ".jpg").
			SetContentType("image/jpeg").
			Build()
		if err != nil {
			t.Fatalf("build %s: %v", v, err)
		}
		if err := admin.Upsert(m); err != nil {
			t.Fatalf("Upsert %s: %v", v, err)
		}
	}

	rows, err := NewProvider(db).ListByMediaObject("m1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2 — thumbnail and card must not collide", len(rows))
	}
}
