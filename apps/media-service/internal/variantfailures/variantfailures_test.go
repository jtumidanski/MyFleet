package variantfailures

import (
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// The entity's TableName is schema-qualified for Postgres; SQLite has no
	// schemas, so attach an in-memory database aliased "media" to make the
	// qualified name resolve — the same approach processedevents takes.
	if err := db.Exec("ATTACH DATABASE ':memory:' AS media").Error; err != nil {
		t.Fatalf("attach media schema: %v", err)
	}
	if err := Migration(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestRecordThenRecorded(t *testing.T) {
	s := New(logrus.New(), newTestDB(t))

	recorded, err := s.Recorded("m1", "card")
	if err != nil {
		t.Fatalf("Recorded before any write: %v", err)
	}
	if recorded {
		t.Fatal("Recorded must be false for a media object with no failure")
	}

	if err := s.Record("m1", "card", ReasonUndecodable); err != nil {
		t.Fatalf("Record: %v", err)
	}

	recorded, err = s.Recorded("m1", "card")
	if err != nil {
		t.Fatalf("Recorded after write: %v", err)
	}
	if !recorded {
		t.Fatal("Recorded must be true after Record")
	}
}

// The failure is scoped to (media object, variant): it must not suppress
// generation of any other variant, and it must not leak to another object.
func TestRecorded_isScopedToTheObjectAndVariant(t *testing.T) {
	s := New(logrus.New(), newTestDB(t))
	if err := s.Record("m1", "card", ReasonUndecodable); err != nil {
		t.Fatalf("Record: %v", err)
	}

	for _, c := range []struct{ id, variant string }{
		{"m1", "display"},
		{"m1", "thumbnail"},
		{"m2", "card"},
	} {
		recorded, err := s.Recorded(c.id, c.variant)
		if err != nil {
			t.Fatalf("Recorded(%s,%s): %v", c.id, c.variant, err)
		}
		if recorded {
			t.Fatalf("Recorded(%s,%s) = true; the record must not leak beyond (m1,card)", c.id, c.variant)
		}
	}
}

// First failure wins. Re-recording is a no-op so the original, most informative
// reason is never overwritten by a later one — and so a repeated attempt cannot
// turn into a write amplification loop.
func TestRecord_firstReasonWins(t *testing.T) {
	db := newTestDB(t)
	s := New(logrus.New(), db)

	if err := s.Record("m1", "card", ReasonUndecodable); err != nil {
		t.Fatalf("first Record: %v", err)
	}
	if err := s.Record("m1", "card", ReasonOriginalMissing); err != nil {
		t.Fatalf("second Record must not error: %v", err)
	}

	var rows []Entity
	if err := db.Find(&rows).Error; err != nil {
		t.Fatalf("read rows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want exactly 1", len(rows))
	}
	if rows[0].Reason != ReasonUndecodable {
		t.Fatalf("Reason = %q, want the first reason %q", rows[0].Reason, ReasonUndecodable)
	}
}

// FR-ADMIN-3. An admin purge that is still cancellable must not suppress lazy
// generation: if the operator cancels, the object comes back, and a ledger row
// that was never a real failure would have permanently disabled its card.
func TestRecorded_ignoresSoftDeletedRows(t *testing.T) {
	db := newTestDB(t)
	s := New(logrus.New(), db)
	if err := s.Record("m1", "card", ReasonUndecodable); err != nil {
		t.Fatalf("Record: %v", err)
	}

	// Exactly what admin.Stamp writes.
	if err := db.Exec(`UPDATE media.media_variant_failures
	                   SET deleted_at = ?, purge_operation_id = ?
	                   WHERE media_object_id = 'm1' AND variant = 'card'`,
		time.Now().UTC(), "op-1").Error; err != nil {
		t.Fatalf("stamp the ledger row: %v", err)
	}

	recorded, err := s.Recorded("m1", "card")
	if err != nil {
		t.Fatalf("Recorded: %v", err)
	}
	if recorded {
		t.Fatal("a ledger row soft-deleted by an in-flight, still-cancellable admin purge " +
			"must not report as a permanent failure")
	}
}
