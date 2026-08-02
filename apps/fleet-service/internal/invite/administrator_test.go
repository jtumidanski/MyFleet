package invite

import (
	"errors"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	sharedevents "github.com/jtumidanski/myfleet/packages/shared-go/events"
)

// newInviteDB returns an in-memory sqlite DB with the invite table and the
// shared outbox table migrated. No socket, no container (FR-DEV-4).
func newInviteDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// TableName is schema-qualified (fleet.fleet_invites) for Postgres. SQLite
	// has no schemas, so attach an in-memory database aliased "fleet" so the
	// qualified name resolves in the test (same workaround as
	// maintenancecategory/entity_test.go). Entity.FleetID and Entity.Token both
	// carry gorm:"index"/"uniqueIndex" tags, and GORM's AutoMigrate emits
	// CREATE INDEX with the schema prefix stripped (a SQLite quirk) — the same
	// failure documented in maintenancerecord/provider_test.go and
	// maintenanceschedule/completion_db_test.go — so Migration(db) (AutoMigrate)
	// cannot be used here; the table is created with explicit DDL instead,
	// mirroring the Entity struct fields exactly.
	if err := db.Exec("ATTACH DATABASE ':memory:' AS fleet").Error; err != nil {
		t.Fatalf("attach fleet schema: %v", err)
	}
	if err := db.Exec(`CREATE TABLE fleet.fleet_invites (
		id TEXT PRIMARY KEY, fleet_id TEXT, email TEXT, role TEXT, token TEXT,
		expires_at DATETIME, accepted_at DATETIME, invited_by_user_id TEXT,
		created_at DATETIME, updated_at DATETIME)`).Error; err != nil {
		t.Fatalf("migrate invites: %v", err)
	}
	if err := sharedevents.MigrateOutbox(db); err != nil {
		t.Fatalf("migrate outbox: %v", err)
	}
	return db
}

func newInvite(fleetID, email, token string) Model {
	return NewBuilder().
		SetFleetID(fleetID).
		SetEmail(email).
		SetRole("member").
		SetToken(token).
		SetExpiresAt(time.Now().Add(7 * 24 * time.Hour)).
		SetInvitedByUserID("user-1").
		Build()
}

func countRows(t *testing.T, db *gorm.DB, model any) int64 {
	t.Helper()
	var n int64
	if err := db.Model(model).Count(&n).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

// FR-EVT-1/FR-EVT-2: the invite row and the outbox row are one unit of work.
func TestInsert_commitsInviteAndOutboxTogether(t *testing.T) {
	db := newInviteDB(t)
	var seen struct {
		fleetID, actorID, traceID, inviteID, email, role string
	}
	adm := NewAdministrator(db).WithCreatedEmitter(
		func(tx *gorm.DB, fleetID, actorID, traceID, inviteID, email, role string) error {
			seen.fleetID, seen.actorID, seen.traceID = fleetID, actorID, traceID
			seen.inviteID, seen.email, seen.role = inviteID, email, role
			return sharedevents.Enqueue(tx, sharedevents.Envelope{EventID: "e1", Type: "invite.created", FleetID: fleetID})
		})

	m := newInvite("f1", "a@b.com", "tok-1")
	created, err := adm.Insert(m, "trace-1")
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if created.ID() != m.ID() {
		t.Fatalf("Insert returned id %q want %q", created.ID(), m.ID())
	}
	if countRows(t, db, &Entity{}) != 1 {
		t.Fatal("want 1 invite row")
	}
	if countRows(t, db, &sharedevents.OutboxRow{}) != 1 {
		t.Fatal("want 1 outbox row")
	}
	// The emitter must be handed the invite's own identity, not the builder's
	// inputs — a mismatch here means the email consumer looks up the wrong row.
	if seen.inviteID != m.ID() || seen.email != "a@b.com" || seen.role != "member" {
		t.Fatalf("emitter got %+v", seen)
	}
	if seen.fleetID != "f1" || seen.actorID != "user-1" || seen.traceID != "trace-1" {
		t.Fatalf("emitter envelope args %+v", seen)
	}
}

// FR-EVT-1: "A failed enqueue rolls back the invite creation." Without the
// transaction this test leaves an invite row behind whose email is never sent
// and whose creation nothing downstream ever hears about.
func TestInsert_rollsBackWhenEmitFails(t *testing.T) {
	db := newInviteDB(t)
	boom := errors.New("outbox unavailable")
	adm := NewAdministrator(db).WithCreatedEmitter(
		func(*gorm.DB, string, string, string, string, string, string) error { return boom })

	if _, err := adm.Insert(newInvite("f1", "a@b.com", "tok-1"), "trace-1"); !errors.Is(err, boom) {
		t.Fatalf("Insert err = %v, want %v", err, boom)
	}
	if n := countRows(t, db, &Entity{}); n != 0 {
		t.Fatalf("want 0 invite rows after rollback, got %d", n)
	}
	if n := countRows(t, db, &sharedevents.OutboxRow{}); n != 0 {
		t.Fatalf("want 0 outbox rows after rollback, got %d", n)
	}
}
