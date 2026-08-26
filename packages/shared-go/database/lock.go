package database

import (
	"context"
	"fmt"
	"hash/fnv"

	"gorm.io/gorm"
)

// WithLeaderLock runs fn only if it acquires the named Postgres advisory lock,
// making background sweeps multi-replica-safe (design A9). If the lock is held
// elsewhere, fn is skipped and (false, nil) is returned.
//
// THE LOCK AND THE UNLOCK MUST RUN ON THE SAME CONNECTION. A Postgres advisory
// lock belongs to a SESSION, and pg_advisory_unlock issued from any other
// session releases nothing — it logs a warning server-side and returns false.
// This function used to take the lock through the *gorm.DB handle, which draws
// an arbitrary connection from the pool for every statement, so the unlock
// usually landed on a different one. The lock then stayed held by an idle
// pooled connection until the process exited, and every later tick got
// (false, nil) and quietly did nothing.
//
// The symptom is brutal to diagnose because it looks like nothing at all: the
// job is scheduled, the goroutine is alive, the tick fires, no error is logged,
// and the work silently never happens. In production it left the admin purge
// reaper running exactly once per pod lifetime and the hourly maintenance
// recompute not running at all for days.
//
// So the connection is checked out explicitly and every statement goes through
// it. conn.Close returns it to the pool AFTER the deferred unlock has run.
func WithLeaderLock(db *gorm.DB, name string, fn func() error) (ran bool, err error) {
	h := fnv.New64a()
	_, _ = h.Write([]byte(name))
	key := int64(h.Sum64())

	sqlDB, err := db.DB()
	if err != nil {
		return false, err
	}
	ctx := context.Background()
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return false, err
	}
	// Runs last, after the unlock below: a connection handed back to the pool
	// still holding the lock is the exact failure this function exists to avoid.
	defer func() { _ = conn.Close() }()

	// $1 rather than ?: this is database/sql against pgx directly, without
	// GORM's placeholder translation.
	var got bool
	if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&got); err != nil {
		return false, err
	}
	if !got {
		return false, nil
	}

	defer func() {
		var released bool
		uerr := conn.QueryRowContext(ctx, "SELECT pg_advisory_unlock($1)", key).Scan(&released)
		switch {
		case uerr != nil:
			// Never mask fn's error with the cleanup's: fn's is the one that
			// says what the sweep did.
			if err == nil {
				err = uerr
			}
		case !released && err == nil:
			// This session held the lock a moment ago, so a false here means the
			// invariant above has been broken again. Say so loudly rather than
			// letting the job go quiet for a week.
			err = fmt.Errorf("advisory lock %q was not released; the next tick will be skipped", name)
		}
	}()
	return true, fn()
}
