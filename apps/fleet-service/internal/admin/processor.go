package admin

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/adminclient"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// AuthVerifier re-checks the platform-admin privilege against the database
// behind auth-service. Declaring the port here rather than importing the
// concrete client keeps the processor testable.
type AuthVerifier interface {
	IsPlatformAdmin(ctx context.Context, userID string) (bool, error)
}

// Downstream is one other service's slice of the purge protocol. Name() is what
// lands in failed_services and, through it, in the console's
// "Media not deleted" wording.
type Downstream interface {
	Name() string
	Purge(ctx context.Context, req adminclient.PurgeRequest) (map[string]int, error)
	Restore(ctx context.Context, opID string) (map[string]int, error)
	Reap(ctx context.Context, opID string) (map[string]int, error)
}

// TargetResolver turns a purge root into the human label to denormalise and,
// for a record-scope vehicle purge, the media ids media-service must be told
// about — the only place in the whole design where an explicit id set crosses a
// service boundary (design OQ-1).
//
// It returns server.ErrNotFound for an unknown target.
type TargetResolver interface {
	Resolve(root Root) (label string, mediaIDs []string, err error)
}

// Deps bundles everything the lifecycle needs. Now is injected so tests can
// assert exact purge_after values.
type Deps struct {
	DB            *gorm.DB
	Provider      Provider
	Administrator Administrator
	Auth          AuthVerifier
	Downstream    []Downstream
	Window        time.Duration
	Now           func() time.Time
}

// Processor owns the purge lifecycle.
type Processor struct {
	log     logrus.FieldLogger
	d       Deps
	targets TargetResolver
}

// NewProcessor constructs the lifecycle processor.
func NewProcessor(log logrus.FieldLogger, d Deps, targets TargetResolver) *Processor {
	if d.Now == nil {
		d.Now = func() time.Time { return time.Now().UTC() }
	}
	if d.Window <= 0 {
		d.Window = DefaultRecoveryWindow
	}
	return &Processor{log: log, d: d, targets: targets}
}

// CreateInput is the validated request body plus the caller's identity.
type CreateInput struct {
	Scope         Scope
	TargetType    string
	TargetID      string
	Confirmation  string
	ActorUserID   string
	ActorEmail    string
	CorrelationID string
}

// Create runs the full purge-creation sequence (design §8.2).
func (p *Processor) Create(ctx context.Context, in CreateInput) (Operation, error) {
	now := p.d.Now()

	// 1. Re-verify against the database behind auth-service, FAIL CLOSED.
	//
	// The claim is stamped at mint time, so a revoked admin holds a valid token
	// for up to 15 minutes (FR-ADMIN-AUTH-7). Coupling this irreversible write
	// to auth-service's availability is the correct trade — the same reasoning
	// mediaclient.ValidateOwnership already applies. Cancel deliberately does
	// NOT do this: never block the recovery path (design §5.4).
	ok, err := p.d.Auth.IsPlatformAdmin(ctx, in.ActorUserID)
	if err != nil {
		p.log.WithError(err).WithField("actor", in.ActorUserID).
			Error("platform-admin re-verification failed; refusing the purge")
		return Operation{}, err
	}
	if !ok {
		return Operation{}, server.ErrForbidden
	}

	// 2. Validate the enums.
	if !ValidScopes[in.Scope] {
		return Operation{}, server.Detailed(server.ErrValidation, "unsupported scope")
	}
	if in.Scope == ScopeRecord && !ValidTargetTypes[in.TargetType] {
		return Operation{}, server.Detailed(server.ErrValidation, "unsupported target_type")
	}

	// 3. Resolve the root and capture the label WHILE THE TARGET STILL HAS ONE.
	root := Root{Scope: in.Scope, TargetType: in.TargetType, TargetID: in.TargetID}
	label, mediaIDs, err := p.targets.Resolve(root)
	if err != nil {
		return Operation{}, err
	}

	// 4. Confirmation, server-side. The disabled button is a courtesy.
	if err := MatchConfirmation(in.Scope, label, in.Confirmation); err != nil {
		return Operation{}, err
	}

	op, err := NewOperationBuilder().
		SetScope(in.Scope).
		SetTarget(targetTypeFor(in), in.TargetID).
		SetTargetLabel(label).
		SetRequestedBy(in.ActorUserID, in.ActorEmail).
		SetPurgeAfter(now.Add(p.d.Window)).
		Build()
	if err != nil {
		return Operation{}, err
	}

	// 5. ONE transaction: operation row + every local stamp + the audit row.
	// Any failure rolls back all three and the operation does not exist
	// (FR-ADMIN-PURGE-8).
	var affected map[string]int
	if err := p.d.DB.Transaction(func(tx *gorm.DB) error {
		if ierr := p.d.Administrator.InsertOperation(tx, op); ierr != nil {
			return ierr
		}
		var serr error
		affected, serr = Stamp(tx, root, op.ID(), now)
		if serr != nil {
			return serr
		}
		if uerr := p.d.Administrator.SetAffected(tx, op.ID(), affected); uerr != nil {
			return uerr
		}
		return p.d.Administrator.InsertAudit(tx, AuditEvent{
			ID:               uuid.NewString(),
			ActorUserID:      in.ActorUserID,
			ActorEmail:       in.ActorEmail,
			Action:           ActionPurgeCreated,
			Scope:            string(in.Scope),
			TargetType:       targetTypeFor(in),
			TargetID:         in.TargetID,
			TargetLabel:      label,
			PurgeOperationID: op.ID(),
			AffectedCounts:   affected,
			CorrelationID:    in.CorrelationID,
			CreatedAt:        now,
		})
	}); err != nil {
		p.log.WithError(err).WithField("correlation_id", in.CorrelationID).Error("local purge transaction")
		return Operation{}, err
	}

	// 6. AFTER commit, fan out. Failures mark the operation partial; they do not
	// roll back the local stamp (FR-ADMIN-PURGE-9), and the response is still
	// 201 with failed_services populated.
	downstreamCounts, failed := p.fanOutPurge(ctx, op, mediaIDs)
	for k, v := range downstreamCounts {
		affected[k] = v
	}
	status := StatusPending
	if len(failed) > 0 {
		status = StatusPartial
	}
	if err := p.d.Administrator.SetAffected(p.d.DB, op.ID(), affected); err != nil {
		p.log.WithError(err).WithField("operation_id", op.ID()).Error("record downstream counts")
	}
	if err := p.d.Administrator.SetStatus(p.d.DB, op.ID(), status, failed, now); err != nil {
		p.log.WithError(err).WithField("operation_id", op.ID()).Error("record operation status")
	}

	p.log.WithFields(logrus.Fields{
		"operation_id":    op.ID(),
		"scope":           in.Scope,
		"target_id":       in.TargetID,
		"actor":           in.ActorUserID,
		"correlation_id":  in.CorrelationID,
		"status":          status,
		"failed_services": failed,
		"affected":        affected,
	}).Info("admin purge created")

	return p.d.Provider.GetOperation(op.ID())
}

// fanOutPurge calls every downstream stamp, collecting counts and the names of
// the services that failed. One service failing never skips the others: a
// partial purge that reached two of three is strictly better than one that
// reached one, and every call is idempotent so the retry costs nothing.
func (p *Processor) fanOutPurge(ctx context.Context, op Operation, mediaIDs []string) (map[string]int, []string) {
	counts := map[string]int{}
	var failed []string
	req := downstreamRequest(op, mediaIDs)
	for _, d := range p.d.Downstream {
		// A record purge with no media touches nothing downstream. Sending an
		// empty media_ids list would be a 422 that reads as a failed service.
		if req.Scope == "media_ids" && len(req.MediaIDs) == 0 {
			continue
		}
		// Notifications are not addressable by record, so a record purge has
		// nothing to say to any service but media.
		if req.Scope == "media_ids" && d.Name() != "media" {
			continue
		}
		got, err := d.Purge(ctx, req)
		if err != nil {
			p.log.WithError(err).WithFields(logrus.Fields{
				"operation_id": op.ID(), "service": d.Name(),
			}).Warn("downstream purge failed; operation is partial")
			failed = append(failed, d.Name())
			continue
		}
		for k, v := range got {
			counts[k] = v
		}
	}
	return counts, failed
}

// downstreamRequest translates a local root into the downstream body.
//
// A record-scope vehicle purge is the ONE case that carries explicit ids: media
// objects have a fleet_id, but "the media belonging to this vehicle" is a fact
// only fleet-service holds (design OQ-1). A cross-fleet leak is not reachable —
// media-service stamps fleet_id at upload and mediaclient.ValidateOwnership
// already refuses to attach another fleet's media — so every id fleet-service
// can produce for a vehicle is in that vehicle's fleet.
func downstreamRequest(op Operation, mediaIDs []string) adminclient.PurgeRequest {
	req := adminclient.PurgeRequest{OperationID: op.ID()}
	switch op.Scope() {
	case ScopeSystem:
		req.Scope = "system"
	case ScopeFleet:
		req.Scope = "fleet"
		req.FleetIDs = []string{op.TargetID()}
	case ScopeRecord:
		req.Scope = "media_ids"
		req.MediaIDs = mediaIDs
	}
	return req
}

// targetTypeFor normalises the stored target_type: a fleet-scope operation
// records "fleet" even when the caller omitted it, so the audit log reads the
// same way for every scope.
func targetTypeFor(in CreateInput) string {
	if in.Scope == ScopeFleet {
		return "fleet"
	}
	return in.TargetType
}
