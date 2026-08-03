package admin

import (
	"time"

	"gorm.io/gorm"
)

// Status is a purge operation's lifecycle state. cancelled and reaped are
// terminal.
type Status string

const (
	StatusPending   Status = "pending"
	StatusPartial   Status = "partial"
	StatusCancelled Status = "cancelled"
	StatusReaped    Status = "reaped"
)

// OperationEntity maps to fleet.purge_operations (PRD §6.2).
//
// TargetID is *string, not string: Postgres rejects the empty string for a uuid
// column and a system-scope operation genuinely has no target.
//
// AffectedCounts and FailedServices are jsonb. They are captured at stamp time
// so the log stays readable after the rows are gone, which is the same reason
// TargetLabel is denormalised.
type OperationEntity struct {
	ID                string `gorm:"type:uuid;primaryKey"`
	Scope             string `gorm:"not null"`
	TargetType        *string
	TargetID          *string `gorm:"type:uuid"`
	TargetLabel       string
	Status            string    `gorm:"not null;index"`
	RequestedByUserID string    `gorm:"type:uuid;not null"`
	RequestedByEmail  string    `gorm:"not null"`
	RequestedAt       time.Time `gorm:"not null"`
	PurgeAfter        time.Time `gorm:"not null;index"`
	ReapedAt          *time.Time
	CancelledAt       *time.Time
	AffectedCounts    []byte `gorm:"type:jsonb"`
	FailedServices    []byte `gorm:"type:jsonb"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (OperationEntity) TableName() string { return "fleet.purge_operations" }

// AuditEntity maps to fleet.admin_audit_events (PRD §6.3).
//
// APPEND-ONLY, and deliberately without a deleted_at: there is no API to modify
// or delete these rows, and a system purge must not erase its own audit trail
// (FR-ADMIN-AUDIT-2). That is also why the table is in excludedTables.
type AuditEntity struct {
	ID               string `gorm:"type:uuid;primaryKey"`
	ActorUserID      string `gorm:"type:uuid;not null"`
	ActorEmail       string `gorm:"not null"`
	Action           string `gorm:"not null;index"`
	Scope            string
	TargetType       *string
	TargetID         *string `gorm:"type:uuid"`
	TargetLabel      string
	PurgeOperationID *string `gorm:"type:uuid;index"`
	AffectedCounts   []byte  `gorm:"type:jsonb"`
	CorrelationID    string
	CreatedAt        time.Time `gorm:"index"`
}

func (AuditEntity) TableName() string { return "fleet.admin_audit_events" }

// Audit action values (FR-ADMIN-AUDIT-1).
const (
	ActionPurgeCreated   = "purge.created"
	ActionPurgeCancelled = "purge.cancelled"
	ActionPurgeRetried   = "purge.retried"
	ActionPurgeReaped    = "purge.reaped"
)

// ActorSystem is the actor_user_id and actor_email the reaper writes, so the
// console can render "system" rather than attributing a scheduled deletion to
// the person who requested it days earlier (FR-ADMIN-UI-13).
const ActorSystem = "system"

// Migration creates both admin-owned tables.
func Migration(db *gorm.DB) error {
	return db.AutoMigrate(&OperationEntity{}, &AuditEntity{})
}
