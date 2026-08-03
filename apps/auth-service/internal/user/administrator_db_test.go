package user

import (
	"testing"
	"time"

	"gorm.io/gorm"
)

func readUserCreatedAt(t *testing.T, db *gorm.DB, id string) time.Time {
	t.Helper()
	var got time.Time
	if err := db.Raw("SELECT created_at FROM auth.users WHERE id = ?", id).Scan(&got).Error; err != nil {
		t.Fatalf("read created_at: %v", err)
	}
	return got
}

// Pins the fix already shipped on main (Entity.CreatedAt `gorm:"<-:create"`).
// The originally reported defect: ProvisionFromGoogle calls Administrator.Update
// on every re-login, ToEntity() carries no createdAt, and db.Save UPDATEs every
// column — so created_at became 0001-01-01 on each login. This test drives the
// real write path; an entity round-trip assertion would pass while the column
// still got wiped.
func TestUpdate_preservesCreatedAt(t *testing.T) {
	db := newTestDB(t)
	m := NewBuilder().SetGoogleSub("sub-1").SetEmail("a@b.com").SetDisplayName("A").Build()
	a := NewAdministrator(db)
	if _, err := a.Insert(m); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	want := readUserCreatedAt(t, db, m.ID())
	if want.IsZero() {
		t.Fatal("Insert left created_at zero; `<-:create` must not block the insert path")
	}

	// The re-login write.
	if _, err := a.Update(m.WithLogin("A2", "https://example.test/a.png", time.Now().UTC(), true)); err != nil {
		t.Fatalf("update user: %v", err)
	}

	got := readUserCreatedAt(t, db, m.ID())
	if got.IsZero() {
		t.Fatal("re-login zeroed created_at; Entity.CreatedAt must stay `gorm:\"<-:create\"`")
	}
	if !got.Equal(want) {
		t.Fatalf("created_at changed across a re-login: got %v, want %v", got, want)
	}

	// The update itself must still land.
	reread, err := NewProvider(db).GetByID(m.ID())
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if reread.DisplayName() != "A2" {
		t.Fatalf("DisplayName = %q, want %q", reread.DisplayName(), "A2")
	}
}
