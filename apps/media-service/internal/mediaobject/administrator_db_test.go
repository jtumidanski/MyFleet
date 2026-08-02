package mediaobject

import (
	"testing"
	"time"

	"gorm.io/gorm"
)

// Reuses newConfirmTestDB from processor_test.go — same package, same DDL, and
// duplicating a KEEP-IN-SYNC schema block is how the two drift apart.
func seedMediaObject(t *testing.T, db *gorm.DB) Model {
	t.Helper()
	m, err := NewBuilder().SetFleetID("f1").SetUploadedByUserID("u1").
		SetBucket("bucket").SetObjectKey("key").Build()
	if err != nil {
		t.Fatalf("build media object: %v", err)
	}
	created, err := NewAdministrator(db).Insert(m)
	if err != nil {
		t.Fatalf("insert media object: %v", err)
	}
	if created.CreatedAt().IsZero() {
		t.Fatal("Insert left created_at zero; the `<-:create` tag must not block the insert path")
	}
	return created
}

func readMediaCreatedAt(t *testing.T, db *gorm.DB, id string) time.Time {
	t.Helper()
	var got time.Time
	if err := db.Raw("SELECT created_at FROM media.media_objects WHERE id = ?", id).
		Scan(&got).Error; err != nil {
		t.Fatalf("read created_at: %v", err)
	}
	return got
}

// FR-FIX-4, call site one (administrator.go:35). Hardening: this path is
// correct today only because Model happens to carry createdAt. The test pins
// both the behaviour and the absence of any change to it.
func TestUpdate_preservesCreatedAt(t *testing.T) {
	db := newConfirmTestDB(t)
	created := seedMediaObject(t, db)
	want := readMediaCreatedAt(t, db, created.ID())

	updated, err := NewAdministrator(db).Update(created.WithStatus(StatusProcessing))
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	got := readMediaCreatedAt(t, db, created.ID())
	if got.IsZero() {
		t.Fatal("Update zeroed created_at")
	}
	if !got.Equal(want) {
		t.Fatalf("created_at changed across Update: got %v, want %v", got, want)
	}
	if updated.Status() != StatusProcessing {
		t.Fatalf("Status = %q, want %q — the update itself must still apply", updated.Status(), StatusProcessing)
	}
}

// FR-FIX-4, call site two (administrator.go:47, inside the transaction).
func TestUpdateInTx_preservesCreatedAt(t *testing.T) {
	db := newConfirmTestDB(t)
	created := seedMediaObject(t, db)
	want := readMediaCreatedAt(t, db, created.ID())

	hookRan := false
	updated, err := NewAdministrator(db).UpdateInTx(created.WithStatus(StatusProcessing),
		func(tx *gorm.DB) error {
			hookRan = true
			return nil
		})
	if err != nil {
		t.Fatalf("update in tx: %v", err)
	}
	if !hookRan {
		t.Fatal("UpdateInTx did not run its hook")
	}

	got := readMediaCreatedAt(t, db, created.ID())
	if got.IsZero() {
		t.Fatal("UpdateInTx zeroed created_at")
	}
	if !got.Equal(want) {
		t.Fatalf("created_at changed across UpdateInTx: got %v, want %v", got, want)
	}
	if updated.Status() != StatusProcessing {
		t.Fatalf("Status = %q, want %q", updated.Status(), StatusProcessing)
	}
}
