package admin

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// Administrator writes purge operations and audit events. Every method takes an
// explicit *gorm.DB so a caller can run the write inside the same transaction as
// the stamp it describes.
type Administrator interface {
	InsertOperation(tx *gorm.DB, o Operation) error
	SetStatus(tx *gorm.DB, id string, s Status, failed []string, at time.Time) error
	SetAffected(tx *gorm.DB, id string, counts map[string]int) error
	InsertAudit(tx *gorm.DB, a AuditEvent) error
	// MarkCancelRequested records that a cancel was ASKED FOR, separately from
	// whether it finished.
	MarkCancelRequested(tx *gorm.DB, id string, at time.Time) error
}

type dbAdministrator struct{ db *gorm.DB }

// NewAdministrator returns an Administrator backed by the database.
func NewAdministrator(db *gorm.DB) Administrator { return &dbAdministrator{db: db} }

func (a *dbAdministrator) InsertOperation(tx *gorm.DB, o Operation) error {
	e := o.ToEntity()
	return tx.Create(&e).Error
}

// SetStatus moves an operation to a new state, stamping the matching timestamp.
// failed replaces failed_services wholesale — a retry that succeeds must clear
// the list rather than accumulate history there; the audit log is where history
// lives.
func (a *dbAdministrator) SetStatus(tx *gorm.DB, id string, s Status, failed []string, at time.Time) error {
	if failed == nil {
		failed = []string{}
	}
	raw, err := json.Marshal(failed)
	if err != nil {
		return err
	}
	updates := map[string]any{"status": string(s), "failed_services": raw, "updated_at": at}
	switch s {
	case StatusCancelled:
		updates["cancelled_at"] = at
	case StatusReaped:
		updates["reaped_at"] = at
	}
	return tx.Model(&OperationEntity{}).Where("id = ?", id).Updates(updates).Error
}

// MarkCancelRequested stamps cancelled_at without touching status.
//
// The two facts are genuinely independent and conflating them cost us a
// data-loss bug: a cancel whose downstream restore failed left status as
// `partial`, which is also what a partly-applied PURGE looks like, so the
// reaper's pending/partial candidate set swept it up and destroyed the very
// rows the operator had asked to keep. cancelled_at is now the durable record
// of intent, and both the reaper and retry key off it rather than guessing from
// status.
func (a *dbAdministrator) MarkCancelRequested(tx *gorm.DB, id string, at time.Time) error {
	return tx.Model(&OperationEntity{}).Where("id = ? AND cancelled_at IS NULL", id).
		Update("cancelled_at", at).Error
}

// SetAffected records the blast radius the stamp actually took.
func (a *dbAdministrator) SetAffected(tx *gorm.DB, id string, counts map[string]int) error {
	if counts == nil {
		counts = map[string]int{}
	}
	raw, err := json.Marshal(counts)
	if err != nil {
		return err
	}
	return tx.Model(&OperationEntity{}).Where("id = ?", id).
		Update("affected_counts", raw).Error
}

// InsertAudit appends one audit row. There is no update or delete counterpart:
// the table is append-only by construction, not by convention (FR-ADMIN-AUDIT-2).
func (a *dbAdministrator) InsertAudit(tx *gorm.DB, ev AuditEvent) error {
	e := ev.ToEntity()
	return tx.Create(&e).Error
}
