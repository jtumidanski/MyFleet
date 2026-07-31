package user

import (
	"errors"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// These tests run against a real database rather than a fake Provider on
// purpose. The login-loop bug this file guards against was invisible to
// processor_test.go's fakeProvider, because that fake keys an opaque map: it
// cannot express WHICH COLUMN the real query filters on, which was the entire
// defect. GET /auth/me passed the JWT `sub` claim — our internal user id — to
// GetBySub, which filters on google_sub. The row was always missed, /auth/me
// always 404'd, and the SPA bounced every authenticated user back to login.
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// TableName is schema-qualified (auth.users) for Postgres. SQLite has no
	// schemas, so attach an in-memory database aliased "auth" so the qualified
	// name resolves. Same approach as fleet-service's provider tests.
	if err := db.Exec("ATTACH DATABASE ':memory:' AS auth").Error; err != nil {
		t.Fatalf("attach auth schema: %v", err)
	}
	// KEEP IN SYNC WITH entity.go. Explicit DDL rather than
	// Migration(db)/AutoMigrate: the uniqueIndex tags
	// on GoogleSub and Email make GORM emit `CREATE UNIQUE INDEX … ON users`
	// unqualified, which cannot resolve against the attached `auth` schema and
	// fails with "no such table: main.users". fleet-service's entities carry no
	// uniqueIndex tags, so its tests can call AutoMigrate directly. The indexes
	// are irrelevant to what these tests assert — which column each lookup
	// filters on.
	if err := db.Exec(`CREATE TABLE auth.users (
		id            text primary key,
		google_sub    text not null unique,
		email         text not null unique,
		display_name  text,
		avatar_url    text,
		last_login_at datetime,
		created_at    datetime,
		updated_at    datetime
	)`).Error; err != nil {
		t.Fatalf("create table: %v", err)
	}
	return db
}

const (
	testUserID    = "7a186017-d27e-4d65-90e3-6b240bf9880a"
	testGoogleSub = "103014855620585076741"
)

func seedUser(t *testing.T, db *gorm.DB) {
	t.Helper()
	e := Entity{ID: testUserID, GoogleSub: testGoogleSub, Email: "a@b.com", DisplayName: "A"}
	if err := db.Create(&e).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
}

// The regression test for the login loop. GET /auth/me holds a user id, not a
// Google sub, so the provider must be able to resolve one.
func TestGetByID_findsUserByPrimaryKey(t *testing.T) {
	db := newTestDB(t)
	seedUser(t, db)

	m, err := NewProvider(db).GetByID(testUserID)
	if err != nil {
		t.Fatalf("GetByID(%q) returned %v; the /auth/me lookup must resolve a user id", testUserID, err)
	}
	if m.GoogleSub() != testGoogleSub {
		t.Fatalf("GetByID returned the wrong user: google_sub = %q, want %q", m.GoogleSub(), testGoogleSub)
	}
}

// Pins the distinction the two identifiers actually have. If GetByID were ever
// reimplemented in terms of google_sub (the original bug), this fails.
func TestGetByID_doesNotMatchOnGoogleSub(t *testing.T) {
	db := newTestDB(t)
	seedUser(t, db)

	if _, err := NewProvider(db).GetByID(testGoogleSub); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetByID(googleSub) = %v; want ErrNotFound — a Google sub is not a user id", err)
	}
}

// auth.JWT builds Identity.UserID via str(claims["sub"]), which yields "" for a
// missing or non-string claim. Empty must not act as "no filter" and return an
// arbitrary user — that would hand the first row in the table to any token
// lacking a sub.
func TestGetByID_emptyIDMatchesNothing(t *testing.T) {
	db := newTestDB(t)
	seedUser(t, db)

	if _, err := NewProvider(db).GetByID(""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetByID(\"\") = %v; want ErrNotFound — an absent sub claim must "+
			"not resolve to a user", err)
	}
}

// The OAuth provisioning path legitimately looks users up by Google sub. Pin
// that too, so a fix to one lookup cannot silently break the other.
func TestGetBySub_matchesGoogleSubOnly(t *testing.T) {
	db := newTestDB(t)
	seedUser(t, db)
	p := NewProvider(db)

	m, err := p.GetBySub(testGoogleSub)
	if err != nil {
		t.Fatalf("GetBySub(googleSub) returned %v; the OAuth callback depends on this", err)
	}
	if m.ID() != testUserID {
		t.Fatalf("GetBySub returned id %q, want %q", m.ID(), testUserID)
	}

	// The exact call GET /auth/me used to make. It must NOT resolve.
	if _, err := p.GetBySub(testUserID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetBySub(userID) = %v; want ErrNotFound — this returning a user "+
			"would mean google_sub and id are being conflated again", err)
	}
}
