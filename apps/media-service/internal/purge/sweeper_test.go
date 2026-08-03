package purge

import (
	"context"
	"io"
	"testing"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

func newSweeper(t *testing.T, db *gorm.DB, store ObjectRemover, cfg Config) *Sweeper {
	t.Helper()
	log := logrus.New()
	log.SetOutput(io.Discard)
	return NewSweeper(log, db, store, cfg)
}

// T5 / FR-PURGE-5. An admin-stamped object belongs to a cancellable operation
// whose lifecycle the admin reaper owns. The sweep must not hard-delete it and,
// worse, must not remove its MinIO object, which no restore could bring back.
//
// This test is written BEFORE the sweep is rewritten and must stay green
// throughout: the one thing a relocation can silently lose is the
// purge_operation_id IS NULL narrowing.
func TestRunOnce_leavesAdminStampedObjectsAlone(t *testing.T) {
	db := newTestDB(t)
	opID := "op-1"
	seedMediaObject(t, db, "mo-user", hourAgo(), nil)
	seedMediaObject(t, db, "mo-admin", hourAgo(), &opID)

	store := newRemover()
	if err := newSweeper(t, db, store, Config{}).RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	if store.wasAsked("k/mo-admin") {
		t.Error("the sweep removed an admin-stamped object's bytes; no restore could bring them back")
	}
	if n := countRows(t, db, `SELECT count(*) FROM media.media_objects WHERE id = 'mo-admin'`); n != 1 {
		t.Errorf("the sweep hard-deleted an admin-stamped media object: %d of 1 left", n)
	}
	if n := countRows(t, db, `SELECT count(*) FROM media.media_objects WHERE id = 'mo-user'`); n != 0 {
		t.Errorf("the sweep did not purge the user-deleted object: %d rows left", n)
	}
	if !store.didRemove("k/mo-user") {
		t.Error("the sweep did not remove the purged object's own bytes")
	}
}
