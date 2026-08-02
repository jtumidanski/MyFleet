package notification

import (
	"errors"

	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

var (
	// ErrNotFound is returned when a notification does not exist (or is not owned
	// by the requesting user).
	ErrNotFound = errors.New("notification not found")
	// ErrDuplicate is returned when an insert violates the dedupe_key unique
	// constraint (a concurrent generator won the race).
	ErrDuplicate = errors.New("notification already exists")
)

// ListFilter narrows a user's notification list. Read is tri-state: nil means
// "any", &true means read-only, &false means unread-only. Type is "" for any.
type ListFilter struct {
	Read *bool
	Type string
}

// Provider is the read-only interface for notification data access. Always
// scoped to a user id so a user only ever sees their own notifications.
type Provider interface {
	ListByUser(userID string, filter ListFilter, page server.Page) ([]Model, int, error)
}

type dbProvider struct{ db *gorm.DB }

// NewProvider returns a read-only Provider backed by the given database.
func NewProvider(db *gorm.DB) Provider { return &dbProvider{db: db} }

func (p *dbProvider) ListByUser(userID string, filter ListFilter, page server.Page) ([]Model, int, error) {
	q := p.db.Model(&Entity{}).Where("user_id = ? AND deleted_at IS NULL", userID)
	if filter.Type != "" {
		q = q.Where("type = ?", filter.Type)
	}
	if filter.Read != nil {
		if *filter.Read {
			q = q.Where("read_at IS NOT NULL")
		} else {
			q = q.Where("read_at IS NULL")
		}
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var es []Entity
	if err := q.Order("created_at desc").Offset(page.Offset()).Limit(page.Size).Find(&es).Error; err != nil {
		return nil, 0, err
	}

	out := make([]Model, 0, len(es))
	for _, e := range es {
		out = append(out, Make(e))
	}
	return out, int(total), nil
}
