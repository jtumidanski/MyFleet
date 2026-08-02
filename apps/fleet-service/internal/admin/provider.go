package admin

import (
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// Provider reads purge operations and audit events.
type Provider interface {
	// GetOperation returns one operation, or ErrOperationNotFound.
	GetOperation(id string) (Operation, error)
	// ListOperations returns one page of operations, newest first, optionally
	// narrowed to a status, plus the unpaged total.
	ListOperations(status string, page server.Page) ([]Operation, int, error)
	// ListDue returns the reaper's candidate set.
	ListDue(now time.Time) ([]Operation, error)
	// ListAudit returns one page of audit events, newest first, plus the total.
	ListAudit(f AuditFilter, page server.Page) ([]AuditEvent, int, error)
}

type dbProvider struct{ db *gorm.DB }

// NewProvider returns a Provider backed by the database.
func NewProvider(db *gorm.DB) Provider { return &dbProvider{db: db} }

func (p *dbProvider) GetOperation(id string) (Operation, error) {
	var e OperationEntity
	if err := p.db.Where("id = ?", id).First(&e).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Operation{}, ErrOperationNotFound
		}
		return Operation{}, err
	}
	return MakeOperation(e), nil
}

func (p *dbProvider) ListOperations(status string, page server.Page) ([]Operation, int, error) {
	q := p.db.Model(&OperationEntity{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var es []OperationEntity
	if err := q.Order("requested_at desc").
		Limit(page.Size).Offset(page.Offset()).Find(&es).Error; err != nil {
		return nil, 0, err
	}
	out := make([]Operation, 0, len(es))
	for _, e := range es {
		out = append(out, MakeOperation(e))
	}
	return out, int(total), nil
}

// ListDue returns operations whose recovery window has elapsed and that have
// not reached a terminal state. cancelled and reaped are excluded: the first
// was undone, the second is done (FR-ADMIN-RESTORE-4).
func (p *dbProvider) ListDue(now time.Time) ([]Operation, error) {
	var es []OperationEntity
	if err := p.db.Where("status IN ? AND purge_after < ?",
		[]string{string(StatusPending), string(StatusPartial)}, now).
		Order("purge_after asc").Find(&es).Error; err != nil {
		return nil, err
	}
	out := make([]Operation, 0, len(es))
	for _, e := range es {
		out = append(out, MakeOperation(e))
	}
	return out, nil
}

func (p *dbProvider) ListAudit(f AuditFilter, page server.Page) ([]AuditEvent, int, error) {
	q := p.db.Model(&AuditEntity{})
	if f.Action != "" {
		q = q.Where("action = ?", f.Action)
	}
	if f.Actor != "" {
		q = q.Where("actor_user_id = ?", f.Actor)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var es []AuditEntity
	if err := q.Order("created_at desc").
		Limit(page.Size).Offset(page.Offset()).Find(&es).Error; err != nil {
		return nil, 0, err
	}
	out := make([]AuditEvent, 0, len(es))
	for _, e := range es {
		out = append(out, MakeAudit(e))
	}
	return out, int(total), nil
}
