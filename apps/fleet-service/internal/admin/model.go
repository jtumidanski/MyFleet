package admin

import (
	"encoding/json"
	"errors"
	"time"
)

// ErrOperationNotFound is the package-local sentinel; the resource layer maps it
// to server.ErrNotFound.
var ErrOperationNotFound = errors.New("purge operation not found")

// ErrAlreadyReaped is returned when a cancel targets a reaped operation.
// Reaping is irreversible and the API says so rather than pretending to succeed
// (FR-ADMIN-RESTORE-2).
var ErrAlreadyReaped = errors.New("purge operation already reaped")

// Operation is the immutable representation of a purge operation.
type Operation struct {
	id                string
	scope             Scope
	targetType        string
	targetID          string
	targetLabel       string
	status            Status
	requestedByUserID string
	requestedByEmail  string
	requestedAt       time.Time
	purgeAfter        time.Time
	reapedAt          *time.Time
	cancelledAt       *time.Time
	affectedCounts    map[string]int
	failedServices    []string
}

func (o Operation) ID() string                     { return o.id }
func (o Operation) Scope() Scope                   { return o.scope }
func (o Operation) TargetType() string             { return o.targetType }
func (o Operation) TargetID() string               { return o.targetID }
func (o Operation) TargetLabel() string            { return o.targetLabel }
func (o Operation) Status() Status                 { return o.status }
func (o Operation) RequestedByUserID() string      { return o.requestedByUserID }
func (o Operation) RequestedByEmail() string       { return o.requestedByEmail }
func (o Operation) RequestedAt() time.Time         { return o.requestedAt }
func (o Operation) PurgeAfter() time.Time          { return o.purgeAfter }
func (o Operation) ReapedAt() *time.Time           { return o.reapedAt }
func (o Operation) CancelledAt() *time.Time        { return o.cancelledAt }
func (o Operation) AffectedCounts() map[string]int { return o.affectedCounts }
func (o Operation) FailedServices() []string       { return o.failedServices }

// Root returns the manifest root this operation purges.
func (o Operation) Root() Root {
	return Root{Scope: o.scope, TargetType: o.targetType, TargetID: o.targetID}
}

// AuditEvent is the immutable representation of one audit row.
type AuditEvent struct {
	ID               string
	ActorUserID      string
	ActorEmail       string
	Action           string
	Scope            string
	TargetType       string
	TargetID         string
	TargetLabel      string
	PurgeOperationID string
	AffectedCounts   map[string]int
	// Empty unless Action is ActionVehicleTransferred.
	SourceFleetID      string
	DestinationFleetID string
	CorrelationID      string
	CreatedAt          time.Time
}

// AuditFilter narrows the audit list (FR-ADMIN-AUDIT-3). Empty strings mean
// "any".
type AuditFilter struct {
	Action string
	Actor  string
}

// MakeOperation converts an entity to a model, tolerating malformed jsonb: a
// row whose counts cannot be decoded still renders in the console with the rest
// of its fields, which is strictly better than a 500 over the whole list.
func MakeOperation(e OperationEntity) Operation {
	o := Operation{
		id:                e.ID,
		scope:             Scope(e.Scope),
		targetLabel:       e.TargetLabel,
		status:            Status(e.Status),
		requestedByUserID: e.RequestedByUserID,
		requestedByEmail:  e.RequestedByEmail,
		requestedAt:       e.RequestedAt,
		purgeAfter:        e.PurgeAfter,
		reapedAt:          e.ReapedAt,
		cancelledAt:       e.CancelledAt,
		affectedCounts:    map[string]int{},
		failedServices:    []string{},
	}
	if e.TargetType != nil {
		o.targetType = *e.TargetType
	}
	if e.TargetID != nil {
		o.targetID = *e.TargetID
	}
	if len(e.AffectedCounts) > 0 {
		_ = json.Unmarshal(e.AffectedCounts, &o.affectedCounts)
	}
	if len(e.FailedServices) > 0 {
		_ = json.Unmarshal(e.FailedServices, &o.failedServices)
	}
	return o
}

// ToEntity converts a model to an entity for persistence. Empty target fields
// become NULL rather than the empty string, which Postgres rejects for uuid.
func (o Operation) ToEntity() OperationEntity {
	e := OperationEntity{
		ID:                o.id,
		Scope:             string(o.scope),
		TargetLabel:       o.targetLabel,
		Status:            string(o.status),
		RequestedByUserID: o.requestedByUserID,
		RequestedByEmail:  o.requestedByEmail,
		RequestedAt:       o.requestedAt,
		PurgeAfter:        o.purgeAfter,
		ReapedAt:          o.reapedAt,
		CancelledAt:       o.cancelledAt,
	}
	if o.targetType != "" {
		tt := o.targetType
		e.TargetType = &tt
	}
	if o.targetID != "" {
		ti := o.targetID
		e.TargetID = &ti
	}
	e.AffectedCounts, _ = json.Marshal(o.affectedCounts)
	e.FailedServices, _ = json.Marshal(o.failedServices)
	return e
}

// MakeAudit converts an audit entity to a model.
func MakeAudit(e AuditEntity) AuditEvent {
	a := AuditEvent{
		ID:             e.ID,
		ActorUserID:    e.ActorUserID,
		ActorEmail:     e.ActorEmail,
		Action:         e.Action,
		Scope:          e.Scope,
		TargetLabel:    e.TargetLabel,
		CorrelationID:  e.CorrelationID,
		CreatedAt:      e.CreatedAt,
		AffectedCounts: map[string]int{},
	}
	if e.TargetType != nil {
		a.TargetType = *e.TargetType
	}
	if e.TargetID != nil {
		a.TargetID = *e.TargetID
	}
	if e.PurgeOperationID != nil {
		a.PurgeOperationID = *e.PurgeOperationID
	}
	if e.SourceFleetID != nil {
		a.SourceFleetID = *e.SourceFleetID
	}
	if e.DestinationFleetID != nil {
		a.DestinationFleetID = *e.DestinationFleetID
	}
	if len(e.AffectedCounts) > 0 {
		_ = json.Unmarshal(e.AffectedCounts, &a.AffectedCounts)
	}
	return a
}

// ToEntity converts an audit model to an entity for persistence.
func (a AuditEvent) ToEntity() AuditEntity {
	e := AuditEntity{
		ID:            a.ID,
		ActorUserID:   a.ActorUserID,
		ActorEmail:    a.ActorEmail,
		Action:        a.Action,
		Scope:         a.Scope,
		TargetLabel:   a.TargetLabel,
		CorrelationID: a.CorrelationID,
		CreatedAt:     a.CreatedAt,
	}
	if a.TargetType != "" {
		tt := a.TargetType
		e.TargetType = &tt
	}
	if a.TargetID != "" {
		ti := a.TargetID
		e.TargetID = &ti
	}
	if a.PurgeOperationID != "" {
		p := a.PurgeOperationID
		e.PurgeOperationID = &p
	}
	if a.SourceFleetID != "" {
		s := a.SourceFleetID
		e.SourceFleetID = &s
	}
	if a.DestinationFleetID != "" {
		d := a.DestinationFleetID
		e.DestinationFleetID = &d
	}
	e.AffectedCounts, _ = json.Marshal(a.AffectedCounts)
	return e
}
