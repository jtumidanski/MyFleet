package database_test

import (
	"fmt"
	"hash/fnv"
	"os"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/database"
)

// The leader lock's central invariant is unobservable without a real Postgres:
// SQLite has no advisory locks, and a fake would just re-implement the bug. So
// this test is opt-in, driven by MYFLEET_TEST_DATABASE_URL, and skipped in CI.
//
// Run it against a scratch database (or a port-forwarded one) with:
//
//	MYFLEET_TEST_DATABASE_URL=postgres://... go test ./database/ -run LeaderLock -v
//
// It only ever takes and releases advisory locks under a name unique to the
// run, and writes nothing.
func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	url := os.Getenv("MYFLEET_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set MYFLEET_TEST_DATABASE_URL to run the advisory-lock tests")
	}
	db, err := gorm.Open(postgres.Open(url), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	return db
}

func lockName(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("leader-lock-selftest-%s-%d", t.Name(), time.Now().UnixNano())
}

// The regression itself. The lock has to be gone once WithLeaderLock returns —
// not held by whichever pooled connection happened to acquire it.
//
// The busy pool is the whole point of the fixture. A sweep never runs alone:
// fleet-service relays its outbox every two seconds and serves HTTP throughout,
// so by the time a sweep finishes, the connection that took the lock is long
// since back in the pool and probably in use. An unlock issued through the
// pooled handle then lands on a DIFFERENT session, releases nothing, and
// returns false — which the old implementation discarded. Without the load this
// test passes against the broken code, because a quiet pool hands back the same
// connection every time.
func TestWithLeaderLock_releasesTheLockForTheNextTick(t *testing.T) {
	db := testDB(t)
	name := lockName(t)

	for tick := 1; tick <= 5; tick++ {
		ran, err := database.WithLeaderLock(db, name, func() error {
			// Occupy the pool, and keep occupying it past this function's
			// return so the deferred unlock cannot draw the acquiring
			// connection.
			var wg sync.WaitGroup
			for i := 0; i < 8; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					_ = db.Exec("SELECT pg_sleep(0.2)").Error
				}()
			}
			t.Cleanup(wg.Wait)
			time.Sleep(20 * time.Millisecond)
			return nil
		})
		if err != nil {
			t.Fatalf("tick %d: %v", tick, err)
		}
		if !ran {
			t.Fatalf("tick %d was skipped: the lock from tick %d was never released, "+
				"so this sweep would silently stop running until the process restarts", tick, tick-1)
		}
		if held := advisoryLocksHeld(t, db, name); held != 0 {
			t.Fatalf("tick %d returned with %d advisory lock(s) under %q still held",
				tick, held, name)
		}
	}
}

// fn's error must not swallow the release.
func TestWithLeaderLock_releasesTheLockWhenTheJobFails(t *testing.T) {
	db := testDB(t)
	name := lockName(t)

	if _, err := database.WithLeaderLock(db, name, func() error {
		return fmt.Errorf("sweep exploded")
	}); err == nil {
		t.Fatal("the job's error must surface")
	}

	ran, err := database.WithLeaderLock(db, name, func() error { return nil })
	if err != nil {
		t.Fatalf("second acquisition: %v", err)
	}
	if !ran {
		t.Error("a failed job left its lock behind")
	}
}

// ...and neither must a panic.
func TestWithLeaderLock_releasesTheLockOnPanic(t *testing.T) {
	db := testDB(t)
	name := lockName(t)

	func() {
		defer func() { _ = recover() }()
		_, _ = database.WithLeaderLock(db, name, func() error {
			panic("sweep panicked")
		})
	}()

	ran, err := database.WithLeaderLock(db, name, func() error { return nil })
	if err != nil {
		t.Fatalf("acquisition after panic: %v", err)
	}
	if !ran {
		t.Error("a panicking job left its lock behind")
	}
}

// A second holder must be refused while the lock is genuinely held — the
// property the whole mechanism exists for, and the one a "always return true"
// fix would break.
func TestWithLeaderLock_skipsWhenHeldElsewhere(t *testing.T) {
	db := testDB(t)
	name := lockName(t)

	var inner bool
	if _, err := database.WithLeaderLock(db, name, func() error {
		ran, err := database.WithLeaderLock(db, name, func() error {
			inner = true
			return nil
		})
		if err != nil {
			return err
		}
		if ran {
			return fmt.Errorf("a second holder acquired a lock that was already held")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if inner {
		t.Error("the guarded body ran twice concurrently")
	}
}

// advisoryLocksHeld counts holders of THIS name's key only. Counting every
// advisory lock in the database would make the test depend on whatever else is
// connected to it — including the very services whose sweeps use this function.
//
// The key derivation mirrors WithLeaderLock's; pg_locks splits the bigint into
// classid (high 32 bits) and objid (low 32).
func advisoryLocksHeld(t *testing.T, db *gorm.DB, name string) int {
	t.Helper()
	h := fnv.New64a()
	_, _ = h.Write([]byte(name))
	key := uint64(h.Sum64())

	var n int
	if err := db.Raw(`SELECT count(*) FROM pg_locks l
		JOIN pg_database d ON d.oid = l.database
		WHERE l.locktype = 'advisory' AND d.datname = current_database()
		  AND l.classid = ? AND l.objid = ?`,
		uint32(key>>32), uint32(key)).Scan(&n).Error; err != nil {
		t.Fatalf("count advisory locks: %v", err)
	}
	return n
}
