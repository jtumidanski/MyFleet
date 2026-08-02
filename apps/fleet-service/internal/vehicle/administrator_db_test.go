package vehicle

import (
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newVehicleDB builds the in-memory harness. Schema-qualified TableNames
// ("fleet.vehicles") target Postgres; SQLite has no schemas, so attach an
// in-memory database aliased "fleet". AutoMigrate is unusable here: Entity
// carries `index` tags and GORM emits CREATE INDEX with the schema prefix
// stripped under SQLite, which cannot resolve against an attached schema. Same
// approach as maintenanceschedule/completion_db_test.go.
func newVehicleDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec("ATTACH DATABASE ':memory:' AS fleet").Error; err != nil {
		t.Fatalf("attach fleet schema: %v", err)
	}
	// KEEP IN SYNC WITH entity.go.
	if err := db.Exec(`CREATE TABLE fleet.vehicles (
		id TEXT PRIMARY KEY, fleet_id TEXT, nickname TEXT, make TEXT, model TEXT,
		trim TEXT, year INTEGER, vin TEXT, current_mileage INTEGER,
		primary_image_media_id TEXT, notes TEXT, created_at DATETIME,
		updated_at DATETIME, deleted_at DATETIME, purge_after DATETIME)`).Error; err != nil {
		t.Fatalf("create fleet.vehicles: %v", err)
	}
	return db
}

func seedVehicle(t *testing.T, db *gorm.DB) Model {
	t.Helper()
	m, err := NewBuilder().SetFleetID("f1").SetMake("Honda").SetModel("Civic").
		SetYear(2020).SetNickname("before").Build()
	if err != nil {
		t.Fatalf("build vehicle: %v", err)
	}
	created, err := NewAdministrator(db).Insert(m)
	if err != nil {
		t.Fatalf("insert vehicle: %v", err)
	}
	if created.CreatedAt().IsZero() {
		t.Fatal("Insert left created_at zero; this harness cannot detect the defect it exists for")
	}
	return created
}

// readCreatedAt reads the column directly rather than through Make(), so the
// assertion is about what is IN the row, not about what the round-trip claims.
func readCreatedAt(t *testing.T, db *gorm.DB, id string) time.Time {
	t.Helper()
	var got time.Time
	if err := db.Raw("SELECT created_at FROM fleet.vehicles WHERE id = ?", id).Scan(&got).Error; err != nil {
		t.Fatalf("read created_at: %v", err)
	}
	return got
}

// The regression test for issue #7's "wider concern". Before the fix ToEntity()
// left CreatedAt at its zero value and Administrator.Update wrote it through
// db.Save, which UPDATEs every column — so one PATCH set created_at to
// 0001-01-01 permanently.
func TestUpdate_preservesCreatedAt(t *testing.T) {
	db := newVehicleDB(t)
	created := seedVehicle(t, db)
	want := readCreatedAt(t, db, created.ID())

	updated, err := NewAdministrator(db).Update(created.WithNickname("after"))
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	got := readCreatedAt(t, db, created.ID())
	if got.IsZero() {
		t.Fatal("Update zeroed created_at; a full-column Save must not write this column")
	}
	if !got.Equal(want) {
		t.Fatalf("created_at changed across Update: got %v, want %v", got, want)
	}
	// Layer two: the Model handed back must carry it as well, or a caller
	// deriving status from the returned Model sees a zero creation time even
	// though the row is fine.
	if updated.CreatedAt().IsZero() {
		t.Fatal("Update returned a Model with a zero CreatedAt(); ToEntity must assign the field")
	}
	// And the real change still landed.
	if updated.Nickname() != "after" {
		t.Fatalf("Nickname = %q, want %q — the update itself must still apply", updated.Nickname(), "after")
	}
}

type fakeScheduleStates struct{ states []string }

func (f fakeScheduleStates) ScheduleStatesByVehicle(string) ([]string, error) {
	return f.states, nil
}

// noActivity is the case DeriveStatus's created_at fallback exists for.
type noActivity struct{}

func (noActivity) LastActivityByVehicle(string) (time.Time, error) { return time.Time{}, nil }

// FR-FIX-1's user-visible symptom. Asserting created_at != zero alone would not
// prove the bug is gone; this is the acceptance criterion that does.
func TestDeriveStatus_editedVehicleWithoutActivityStaysHealthy(t *testing.T) {
	db := newVehicleDB(t)
	created := seedVehicle(t, db)

	if _, err := NewAdministrator(db).Update(created.WithNickname("after")); err != nil {
		t.Fatalf("update: %v", err)
	}
	reread, err := NewProvider(db).GetByID(created.ID())
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}

	deps := StatusDeps{Schedules: fakeScheduleStates{states: []string{"ok"}}, Activity: noActivity{}}
	if got := deps.DeriveStatus(reread, time.Now().UTC()); got != "Healthy" {
		t.Fatalf("DeriveStatus after an edit = %q, want %q — a zeroed created_at makes the "+
			"inactivity fallback compare against 0001-01-01 and report Inactive", got, "Healthy")
	}
}

// PRD Q5, resolved by test rather than by assumption: SoftDelete and RestoreRow
// use narrowed Updates(map{...}) writes, which design §2 V7 measured as
// unaffected by a `<-:create` field elsewhere on the struct. "Unaffected" is
// exactly the claim a tag change falsifies quietly, so pin it.
func TestRestoreRow_survivesTheInsertOnlyCreatedAtTag(t *testing.T) {
	db := newVehicleDB(t)
	created := seedVehicle(t, db)
	a := NewAdministrator(db)
	want := readCreatedAt(t, db, created.ID())

	if _, err := a.SoftDelete(created.ID()); err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	restored, err := a.RestoreRow(created.ID())
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if restored.DeletedAt() != nil {
		t.Fatalf("RestoreRow left deleted_at set: %v", restored.DeletedAt())
	}
	if _, err := NewProvider(db).GetByID(created.ID()); err != nil {
		t.Fatalf("restored vehicle is invisible to GetByID: %v", err)
	}
	if got := readCreatedAt(t, db, created.ID()); !got.Equal(want) {
		t.Fatalf("created_at changed across soft-delete/restore: got %v, want %v", got, want)
	}
}
