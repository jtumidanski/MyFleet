package notification

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Administrator is the write interface for notification data access. It also
// satisfies the processor's Store interface (Insert + ExistsByDedupeKey), so the
// same instance backs both generation and the read/mark endpoints.
type Administrator interface {
	ExistsByDedupeKey(k string) (bool, error)
	Insert(m Model) error
	MarkRead(userID, id string) error
	MarkAllRead(userID string) error
}

type dbAdministrator struct{ db *gorm.DB }

// NewAdministrator returns an Administrator backed by the given database.
func NewAdministrator(db *gorm.DB) Administrator { return &dbAdministrator{db: db} }

func (a *dbAdministrator) ExistsByDedupeKey(k string) (bool, error) {
	var count int64
	if err := a.db.Model(&Entity{}).Where("dedupe_key = ?", k).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// Insert persists a new notification. A unique-constraint violation on
// dedupe_key (a concurrent generator) is surfaced as ErrDuplicate so the caller
// can treat it as a successful dedupe.
func (a *dbAdministrator) Insert(m Model) error {
	e := m.ToEntity()
	err := a.db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "dedupe_key"}}, DoNothing: true}).Create(&e)
	if err.Error != nil {
		if errors.Is(err.Error, gorm.ErrDuplicatedKey) {
			return ErrDuplicate
		}
		return err.Error
	}
	if err.RowsAffected == 0 {
		return ErrDuplicate
	}
	return nil
}

// MarkRead stamps a single notification owned by userID as read. Returns
// ErrNotFound when no such (unread or read) notification exists for the user.
func (a *dbAdministrator) MarkRead(userID, id string) error {
	now := time.Now().UTC()
	res := a.db.Model(&Entity{}).
		Where("id = ? AND user_id = ? AND read_at IS NULL", id, userID).
		Update("read_at", now)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		// Either it does not exist / is not owned by the user, or it is already
		// read. Distinguish: a row that exists-but-already-read is a no-op success.
		var count int64
		if err := a.db.Model(&Entity{}).Where("id = ? AND user_id = ?", id, userID).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return ErrNotFound
		}
	}
	return nil
}

// MarkAllRead stamps every unread notification owned by userID as read.
func (a *dbAdministrator) MarkAllRead(userID string) error {
	now := time.Now().UTC()
	return a.db.Model(&Entity{}).
		Where("user_id = ? AND read_at IS NULL", userID).
		Update("read_at", now).Error
}
