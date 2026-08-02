package platformadmin

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newSeedDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec("ATTACH DATABASE ':memory:' AS auth").Error; err != nil {
		t.Fatalf("attach auth schema: %v", err)
	}
	ddl := []string{
		`CREATE TABLE auth.users (
			id TEXT PRIMARY KEY, google_sub TEXT, email TEXT, display_name TEXT,
			avatar_url TEXT, theme_preference TEXT, last_login_at DATETIME,
			created_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE auth.platform_admins (
			user_id TEXT PRIMARY KEY, granted_by TEXT, granted_at DATETIME)`,
	}
	for _, stmt := range ddl {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("ddl: %v", err)
		}
	}
	return db
}

func TestParseBootstrapEmails_normalises(t *testing.T) {
	got := ParseBootstrapEmails("  JTumidanski@Gmail.com , second@example.com ,,")
	if len(got) != 2 {
		t.Fatalf("want 2 emails, got %d: %v", len(got), got)
	}
	if !got["jtumidanski@gmail.com"] {
		t.Errorf("emails must be lower-cased and trimmed: %v", got)
	}
	if got[""] {
		t.Errorf("empty segments must be dropped: %v", got)
	}
	if n := len(ParseBootstrapEmails("")); n != 0 {
		t.Errorf("an empty list must parse to no emails, got %d", n)
	}
}

// FR-ADMIN-AUTH-1: the startup seed grants only to users that already exist,
// and re-running it changes nothing.
func TestSeedFromEmails_grantsExistingUsersAndIsIdempotent(t *testing.T) {
	db := newSeedDB(t)
	if err := db.Exec(`INSERT INTO auth.users (id, google_sub, email)
	                   VALUES ('u1', 'sub-1', 'jtumidanski@gmail.com')`).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	emails := ParseBootstrapEmails("jtumidanski@gmail.com,absent@example.com")

	granted, err := SeedFromEmails(db, emails)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if granted != 1 {
		t.Errorf("want 1 grant (the absent user has no row to key on), got %d", granted)
	}

	prov := NewProvider(db)
	if ok, err := prov.IsAdmin("u1"); err != nil || !ok {
		t.Errorf("u1 should be a platform admin, got %v err %v", ok, err)
	}

	if _, err := SeedFromEmails(db, emails); err != nil {
		t.Fatalf("second seed must succeed: %v", err)
	}
	var rows int64
	if err := db.Raw(`SELECT count(*) FROM auth.platform_admins`).Scan(&rows).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if rows != 1 {
		t.Errorf("re-seeding duplicated the grant: %d rows", rows)
	}
}

// Case folding matters: Google returns whatever casing the user typed, and the
// allow-list is hand-written in a ConfigMap.
func TestSeedFromEmails_isCaseInsensitive(t *testing.T) {
	db := newSeedDB(t)
	if err := db.Exec(`INSERT INTO auth.users (id, google_sub, email)
	                   VALUES ('u1', 'sub-1', 'JTumidanski@GMAIL.com')`).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := SeedFromEmails(db, ParseBootstrapEmails("jtumidanski@gmail.com")); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if ok, _ := NewProvider(db).IsAdmin("u1"); !ok {
		t.Error("a differently-cased stored email must still match the bootstrap list")
	}
}

// Revocation is an out-of-band DELETE (PRD non-goal: no UI). The provider is
// the runtime source of truth, so it must see the deletion immediately.
func TestProvider_reflectsRevocation(t *testing.T) {
	db := newSeedDB(t)
	if err := NewAdministrator(db).Grant("u1", BootstrapGrantedBy); err != nil {
		t.Fatalf("grant: %v", err)
	}
	if err := db.Exec(`DELETE FROM auth.platform_admins WHERE user_id = 'u1'`).Error; err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if ok, err := NewProvider(db).IsAdmin("u1"); err != nil || ok {
		t.Errorf("a revoked admin must read as false, got %v err %v", ok, err)
	}
}
