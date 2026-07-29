package database

import (
	"hash/fnv"

	"gorm.io/gorm"
)

// WithLeaderLock runs fn only if it acquires the named Postgres advisory lock,
// making background sweeps multi-replica-safe (design A9). If the lock is held
// elsewhere, fn is skipped and (false, nil) is returned.
func WithLeaderLock(db *gorm.DB, name string, fn func() error) (ran bool, err error) {
	h := fnv.New64a()
	_, _ = h.Write([]byte(name))
	key := int64(h.Sum64())

	var got bool
	if err = db.Raw("SELECT pg_try_advisory_lock(?)", key).Scan(&got).Error; err != nil {
		return false, err
	}
	if !got {
		return false, nil
	}
	defer db.Exec("SELECT pg_advisory_unlock(?)", key)
	return true, fn()
}
