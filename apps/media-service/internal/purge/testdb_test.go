package purge

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"github.com/jtumidanski/myfleet/apps/media-service/internal/mediaobject"
	"github.com/jtumidanski/myfleet/apps/media-service/internal/mediavariant"
	"github.com/jtumidanski/myfleet/apps/media-service/internal/variantfailures"
)

// newTestDB opens an in-memory SQLite database with a "media" schema attached.
//
// media_objects and media_variants are hand-written, matching every other test
// in this service. That is forced, not preferred: both entities carry
// gorm:"index" tags, and AutoMigrate emits its CREATE INDEX WITHOUT the schema
// qualifier (`ON media_variants`, not `ON media.media_variants`), which fails on
// SQLite with "no such table: main.media_variants". The real
// mediavariant.ApplyPartialIndexes and variantfailures.Migration DO run, so the
// index semantics the purge depends on are production's.
//
// Hand-written DDL drifts from the struct — that is a real hazard and it is why
// TestHarnessDDLMatchesTheEntities exists below rather than being left to
// vigilance.
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// The ":memory:" DSN carries no cache=shared, so every physical connection
	// is an independent empty database. Capping the pool at one keeps the ATTACH
	// and the DDL from applying to a different connection than a later query
	// lands on.
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.Exec("ATTACH DATABASE ':memory:' AS media").Error; err != nil {
		t.Fatalf("attach media schema: %v", err)
	}
	ddl := []string{
		`CREATE TABLE media.media_objects (
			id TEXT PRIMARY KEY, fleet_id TEXT, uploaded_by_user_id TEXT, bucket TEXT,
			object_key TEXT, content_type TEXT, size INTEGER, original_filename TEXT,
			status TEXT, created_at DATETIME, deleted_at DATETIME, purge_after DATETIME,
			purge_operation_id TEXT)`,
		`CREATE TABLE media.media_variants (
			id TEXT PRIMARY KEY, media_object_id TEXT NOT NULL, variant TEXT NOT NULL,
			object_key TEXT NOT NULL, width INTEGER, height INTEGER, content_type TEXT,
			created_at DATETIME, deleted_at DATETIME, purge_operation_id TEXT)`,
	}
	for _, q := range ddl {
		if err := db.Exec(q).Error; err != nil {
			t.Fatalf("ddl: %v", err)
		}
	}
	// The partial unique index is production's, not a plain UNIQUE: it is what
	// lets a live and a soft-deleted row share (media_object_id, variant), which
	// is exactly the case the purge has to remove both halves of.
	if err := mediavariant.ApplyPartialIndexes(db); err != nil {
		t.Fatalf("apply partial indexes: %v", err)
	}
	if err := variantfailures.Migration(db); err != nil {
		t.Fatalf("variantfailures migration: %v", err)
	}
	return db
}

// TestHarnessDDLMatchesTheEntities is the guard that makes hand-written DDL
// acceptable. A column added to an entity but not to the DDL above would make
// every row assertion in this package test a schema production does not have —
// a red test turning green for the wrong reason, which is the failure mode this
// whole task exists to correct.
func TestHarnessDDLMatchesTheEntities(t *testing.T) {
	db := newTestDB(t)
	for _, c := range []struct {
		table string
		model any
	}{
		{"media_objects", &mediaobject.Entity{}},
		{"media_variants", &mediavariant.Entity{}},
		{"media_variant_failures", &variantfailures.Entity{}},
	} {
		s, err := schema.Parse(c.model, &sync.Map{}, db.NamingStrategy)
		if err != nil {
			t.Fatalf("parse %s schema: %v", c.table, err)
		}
		var want []string
		for _, f := range s.Fields {
			if f.DBName != "" {
				want = append(want, f.DBName)
			}
		}
		var got []string
		if err := db.Raw(`SELECT name FROM media.pragma_table_info(?)`, c.table).Scan(&got).Error; err != nil {
			t.Fatalf("pragma_table_info(%s): %v", c.table, err)
		}
		sort.Strings(want)
		sort.Strings(got)
		if len(want) == 0 {
			t.Fatalf("%s: parsed no columns from the entity — this check would pass vacuously", c.table)
		}
		if len(want) != len(got) {
			t.Fatalf("%s columns drifted:\n  entity: %v\n  table : %v", c.table, want, got)
		}
		for i := range want {
			if want[i] != got[i] {
				t.Fatalf("%s columns drifted:\n  entity: %v\n  table : %v", c.table, want, got)
			}
		}
	}
}

// recordingRemover captures the keys the sweep asks to remove, and can be told
// to fail for a chosen one (FR-TEST-3).
//
// asked and removed are separate on purpose. Several tests assert a key was
// never OFFERED to the remover, which is a different and stronger claim than
// "the row survived" — the latter is trivially true in code that never looks at
// variants at all.
type recordingRemover struct {
	asked   []string
	removed []string
	fail    map[string]bool
}

func newRemover(failKeys ...string) *recordingRemover {
	fail := map[string]bool{}
	for _, k := range failKeys {
		fail[k] = true
	}
	return &recordingRemover{fail: fail}
}

func (r *recordingRemover) RemoveObject(_ context.Context, key string) error {
	r.asked = append(r.asked, key)
	if r.fail[key] {
		return errors.New("object store unavailable")
	}
	r.removed = append(r.removed, key)
	return nil
}

func (r *recordingRemover) wasAsked(key string) bool  { return contains(r.asked, key) }
func (r *recordingRemover) didRemove(key string) bool { return contains(r.removed, key) }

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

// seedMediaObject inserts a media object. purgeAfter in the past with a nil
// operation ID is what ListPurgeable selects; opID non-nil is an admin-stamped
// object it must skip.
func seedMediaObject(t *testing.T, db *gorm.DB, id string, purgeAfter *time.Time, opID *string) {
	t.Helper()
	var deletedAt *time.Time
	if purgeAfter != nil {
		deletedAt = purgeAfter
	}
	if err := db.Exec(`INSERT INTO media.media_objects
		(id, fleet_id, uploaded_by_user_id, bucket, object_key, content_type, status,
		 created_at, deleted_at, purge_after, purge_operation_id)
		VALUES (?, 'fleet-1', 'user-1', 'media', ?, 'image/jpeg', 'ready', ?, ?, ?, ?)`,
		id, "k/"+id, time.Now().UTC(), deletedAt, purgeAfter, opID).Error; err != nil {
		t.Fatalf("seed media object %s: %v", id, err)
	}
}

func seedVariant(t *testing.T, db *gorm.DB, id, mediaObjectID, variant, key string, deletedAt *time.Time) {
	t.Helper()
	if err := db.Exec(`INSERT INTO media.media_variants
		(id, media_object_id, variant, object_key, content_type, created_at, deleted_at)
		VALUES (?, ?, ?, ?, 'image/jpeg', ?, ?)`,
		id, mediaObjectID, variant, key, time.Now().UTC(), deletedAt).Error; err != nil {
		t.Fatalf("seed variant %s: %v", id, err)
	}
}

func seedLedgerRow(t *testing.T, db *gorm.DB, mediaObjectID, variant string) {
	t.Helper()
	if err := db.Exec(`INSERT INTO media.media_variant_failures
		(media_object_id, variant, reason, failed_at) VALUES (?, ?, 'undecodable', ?)`,
		mediaObjectID, variant, time.Now().UTC()).Error; err != nil {
		t.Fatalf("seed ledger row (%s,%s): %v", mediaObjectID, variant, err)
	}
}

func countRows(t *testing.T, db *gorm.DB, query string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := db.Raw(query, args...).Scan(&n).Error; err != nil {
		t.Fatalf("count (%s): %v", query, err)
	}
	return n
}

func hourAgo() *time.Time {
	p := time.Now().UTC().Add(-time.Hour)
	return &p
}
