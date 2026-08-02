package admin

import (
	"time"

	"github.com/google/uuid"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// OperationBuilder constructs a valid Operation. Build returns an error because
// scope, requester and purge_after are invariants enforced at construction.
type OperationBuilder struct{ o Operation }

// NewOperationBuilder starts a pending operation with a fresh id and
// requested_at of now.
func NewOperationBuilder() *OperationBuilder {
	return &OperationBuilder{o: Operation{
		id:             uuid.NewString(),
		status:         StatusPending,
		requestedAt:    time.Now().UTC(),
		affectedCounts: map[string]int{},
		failedServices: []string{},
	}}
}

// SetID overrides the generated id. Tests use it; production does not.
func (b *OperationBuilder) SetID(id string) *OperationBuilder { b.o.id = id; return b }

func (b *OperationBuilder) SetScope(s Scope) *OperationBuilder { b.o.scope = s; return b }

// SetTarget records what a non-system purge is rooted at.
func (b *OperationBuilder) SetTarget(targetType, targetID string) *OperationBuilder {
	b.o.targetType = targetType
	b.o.targetID = targetID
	return b
}

// SetTargetLabel denormalises the target's name, captured at request time so the
// log stays readable after the target is gone.
func (b *OperationBuilder) SetTargetLabel(l string) *OperationBuilder {
	b.o.targetLabel = l
	return b
}

func (b *OperationBuilder) SetRequestedBy(userID, email string) *OperationBuilder {
	b.o.requestedByUserID = userID
	b.o.requestedByEmail = email
	return b
}

func (b *OperationBuilder) SetPurgeAfter(t time.Time) *OperationBuilder {
	b.o.purgeAfter = t
	return b
}

// Build validates invariants and returns the operation or a 422.
func (b *OperationBuilder) Build() (Operation, error) {
	if !ValidScopes[b.o.scope] {
		return Operation{}, server.Detailed(server.ErrValidation, "unsupported scope")
	}
	if b.o.scope == ScopeRecord && !ValidTargetTypes[b.o.targetType] {
		return Operation{}, server.Detailed(server.ErrValidation, "unsupported target_type")
	}
	if b.o.scope != ScopeSystem && b.o.targetID == "" {
		return Operation{}, server.Detailed(server.ErrValidation, "target_id is required for this scope")
	}
	if b.o.requestedByUserID == "" || b.o.requestedByEmail == "" {
		return Operation{}, server.Detailed(server.ErrValidation, "requester is required")
	}
	if b.o.purgeAfter.IsZero() {
		return Operation{}, server.Detailed(server.ErrValidation, "purge_after is required")
	}
	return b.o, nil
}
