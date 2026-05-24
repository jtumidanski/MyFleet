package processedevents

import (
	"testing"

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
	// The entity's TableName is schema-qualified (media.processed_events) for
	// Postgres. SQLite has no schemas, so attach an in-memory database aliased
	// "media" to make the qualified name resolve in the test.
	if err := db.Exec("ATTACH DATABASE ':memory:' AS media").Error; err != nil {
		t.Fatalf("attach media schema: %v", err)
	}
	if err := db.AutoMigrate(&Entity{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestMarkProcessed_idempotent(t *testing.T) {
	db := newTestDB(t)
	store := New(logrus.New(), db)

	// First mark: not previously processed.
	already, err := store.MarkProcessed("evt-1")
	if err != nil {
		t.Fatalf("first mark: %v", err)
	}
	if already {
		t.Fatal("first mark must report alreadyProcessed=false")
	}

	// Second mark of the same event: reported as already processed.
	already, err = store.MarkProcessed("evt-1")
	if err != nil {
		t.Fatalf("second mark: %v", err)
	}
	if !already {
		t.Fatal("re-processing the same event must report alreadyProcessed=true")
	}

	// A different event is fresh.
	already, err = store.MarkProcessed("evt-2")
	if err != nil {
		t.Fatalf("third mark: %v", err)
	}
	if already {
		t.Fatal("a new event must report alreadyProcessed=false")
	}
}
