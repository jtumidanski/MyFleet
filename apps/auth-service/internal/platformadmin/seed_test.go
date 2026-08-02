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
			email_verified BOOLEAN NOT NULL DEFAULT 0,
			created_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE auth.platform_admins (
			user_id TEXT PRIMARY KEY, granted_by TEXT, granted_at DATETIME, revoked_at DATETIME)`,
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
	if err := db.Exec(`INSERT INTO auth.users (id, google_sub, email, email_verified)
	                   VALUES ('u1', 'sub-1', 'jtumidanski@gmail.com', 1)`).Error; err != nil {
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
	if err := db.Exec(`INSERT INTO auth.users (id, google_sub, email, email_verified)
	                   VALUES ('u1', 'sub-1', 'JTumidanski@GMAIL.com', 1)`).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := SeedFromEmails(db, ParseBootstrapEmails("jtumidanski@gmail.com")); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if ok, _ := NewProvider(db).IsAdmin("u1"); !ok {
		t.Error("a differently-cased stored email must still match the bootstrap list")
	}
}

// Revocation is an out-of-band UPDATE setting revoked_at (PRD non-goal: no
// UI). The provider is the runtime source of truth, so it must see the
// tombstone immediately. A DELETE is deliberately NOT the revocation
// mechanism — see TestSeedFromEmails_doesNotRegrantARevokedAdmin for why.
func TestProvider_reflectsRevocation(t *testing.T) {
	db := newSeedDB(t)
	if err := NewAdministrator(db).Grant("u1", BootstrapGrantedBy); err != nil {
		t.Fatalf("grant: %v", err)
	}
	if err := db.Exec(`UPDATE auth.platform_admins SET revoked_at = CURRENT_TIMESTAMP WHERE user_id = 'u1'`).Error; err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if ok, err := NewProvider(db).IsAdmin("u1"); err != nil || ok {
		t.Errorf("a revoked admin must read as false, got %v err %v", ok, err)
	}
}

// Revocation must be DURABLE across a restart. A plain DELETE is NOT durable:
// the row is simply gone, so the next startup seed re-reads the bootstrap
// list, finds no row keyed on this user, and grants it right back — which is
// exactly the bug this test guards against. revoked_at is a tombstone: the
// row stays present so the seed can see "this user was explicitly revoked"
// and skip it.
func TestSeedFromEmails_doesNotRegrantARevokedAdmin(t *testing.T) {
	db := newSeedDB(t)
	if err := db.Exec(`INSERT INTO auth.users (id, google_sub, email, email_verified)
	                   VALUES ('u1', 'sub-1', 'jtumidanski@gmail.com', 1)`).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := NewAdministrator(db).Grant("u1", BootstrapGrantedBy); err != nil {
		t.Fatalf("grant: %v", err)
	}
	if err := db.Exec(`UPDATE auth.platform_admins SET revoked_at = CURRENT_TIMESTAMP WHERE user_id = 'u1'`).Error; err != nil {
		t.Fatalf("revoke: %v", err)
	}

	if _, err := SeedFromEmails(db, ParseBootstrapEmails("jtumidanski@gmail.com")); err != nil {
		t.Fatalf("re-seed: %v", err)
	}

	if ok, err := NewProvider(db).IsAdmin("u1"); err != nil || ok {
		t.Errorf("a revoked admin must stay revoked across a re-seed, got %v err %v", ok, err)
	}
}

// The email_verified gate user.Processor.maybeGrantAdmin applies at login
// time must also hold at boot: SeedFromEmails runs with no id_token in hand,
// so it can only honor verification if the flag was persisted at login. A
// bootstrap-listed address whose stored row is NOT verified must not become
// an admin — otherwise the escalation the login-time gate exists to close
// (a corporate address Google marks email_verified: false) reopens on every
// restart, which is exactly what the login-time fix alone does not prevent.
func TestSeedFromEmails_skipsUnverifiedEmail(t *testing.T) {
	db := newSeedDB(t)
	if err := db.Exec(`INSERT INTO auth.users (id, google_sub, email, email_verified)
	                   VALUES ('u1', 'sub-1', 'jtumidanski@gmail.com', 0)`).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}

	granted, err := SeedFromEmails(db, ParseBootstrapEmails("jtumidanski@gmail.com"))
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if granted != 0 {
		t.Errorf("want 0 grants for an unverified email, got %d", granted)
	}
	if ok, err := NewProvider(db).IsAdmin("u1"); err != nil || ok {
		t.Errorf("an unverified bootstrap email must not become an admin, got %v err %v", ok, err)
	}
}
