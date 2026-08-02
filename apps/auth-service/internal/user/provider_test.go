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
		id               text primary key,
		google_sub       text not null unique,
		email            text not null unique,
		display_name     text,
		avatar_url       text,
		theme_preference text not null default 'system',
		last_login_at    datetime,
		created_at       datetime,
		updated_at       datetime
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

func seedUserWith(t *testing.T, db *gorm.DB, id, sub, email, name string) {
	t.Helper()
	e := Entity{ID: id, GoogleSub: sub, Email: email, DisplayName: name, ThemePreference: ThemeSystem}
	if err := db.Create(&e).Error; err != nil {
		t.Fatalf("seed %s: %v", id, err)
	}
}

func TestListByIDs_returnsOnlyTheRequestedUsers(t *testing.T) {
	db := newTestDB(t)
	seedUserWith(t, db, "u1", "sub-1", "one@example.com", "One")
	seedUserWith(t, db, "u2", "sub-2", "two@example.com", "Two")
	seedUserWith(t, db, "u3", "sub-3", "three@example.com", "Three")

	ms, err := NewProvider(db).ListByIDs([]string{"u1", "u3"})
	if err != nil {
		t.Fatalf("ListByIDs: %v", err)
	}
	got := map[string]string{}
	for _, m := range ms {
		got[m.ID()] = m.DisplayName()
	}
	if len(got) != 2 || got["u1"] != "One" || got["u3"] != "Three" {
		t.Fatalf("got %+v, want exactly u1 and u3", got)
	}
}

// FR-1.4: an id with no users row is simply absent. The handler treats that as
// a normal result, not an error, so the provider must not invent one.
func TestListByIDs_omitsUnknownIDsWithoutError(t *testing.T) {
	db := newTestDB(t)
	seedUserWith(t, db, "u1", "sub-1", "one@example.com", "One")

	ms, err := NewProvider(db).ListByIDs([]string{"u1", "ghost"})
	if err != nil {
		t.Fatalf("ListByIDs must not error on an unknown id: %v", err)
	}
	if len(ms) != 1 || ms[0].ID() != "u1" {
		t.Fatalf("got %+v, want only u1", ms)
	}
}

// An empty argument must not become `WHERE id IN ()` — some drivers turn
// that into a syntax error, and the caller's "nothing allowed" case is
// legitimate. Passing a nil *gorm.DB is what makes this a real assertion:
// the short-circuit is the only way to return without dereferencing it, so
// deleting that guard turns this test into a panic rather than a pass.
func TestListByIDs_returnsEmptyForNoIDs(t *testing.T) {
	ms, err := NewProvider(nil).ListByIDs(nil)
	if err != nil {
		t.Fatalf("ListByIDs(nil): %v", err)
	}
	if len(ms) != 0 {
		t.Fatalf("got %+v, want an empty result", ms)
	}
}
