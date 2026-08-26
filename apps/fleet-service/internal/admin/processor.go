package admin

import (
	"context"
	"errors"
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

// MediaReassigner re-homes a set of MEDIA objects to another fleet. Declared
// here as a port rather than importing the concrete client, matching how
// Downstream and AuthVerifier are declared, so the processor stays testable
// without an HTTP server. *adminclient.MediaClient satisfies it.
//
// COUNT SEMANTICS. The returned map's "media_objects" is the number of LIVE
// rows that are NOW on destFleetID for the named ids — a read-back of the end
// state, not a count of rows this call changed. Re-running the same reassign
// returns the same number, and a media object that was ALREADY on the
// destination (a prior partial transfer, or pre-existing state) is included.
// Whoever surfaces this number to a human must say "media now in the
// destination fleet", never "media moved by this transfer".
type MediaReassigner interface {
	Reassign(ctx context.Context, mediaIDs []string, destFleetID string) (map[string]int, error)
}

// NotificationReassigner re-points notifications for a set of VEHICLES.
//
// Vehicle ids, not notification ids: notification-service owns the
// vehicle -> notification relationship, and enumerating it here would mean
// fleet-service reading another service's rows.
// *adminclient.NotificationClient satisfies it.
//
// COUNT SEMANTICS: identical to MediaReassigner's — "notifications" is the live
// row count now on destFleetID for the named vehicles, not the number of rows
// this call rewrote.
type NotificationReassigner interface {
	Reassign(ctx context.Context, vehicleIDs []string, destFleetID string) (map[string]int, error)
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
	// StatsSources are the remote counts /admin/stats fans out to. Separate
	// from Downstream because the sets differ: auth-service contributes a count
	// but is never purged.
	StatsSources []StatsSource
	// MediaReassign and NotificationReassign are the two downstream halves of a
	// vehicle transfer. They are separate from Downstream because the protocols
	// differ: a purge fans out the same PurgeRequest to every service, while a
	// transfer sends media-service media ids and notification-service vehicle
	// ids. Nil disables the corresponding call, which is what the purge-only
	// tests rely on.
	MediaReassign        MediaReassigner
	NotificationReassign NotificationReassigner
	// AuthUsers resolves member ids to email and display name for the fleet
	// detail view. A failure here is a warning, not an error (FR-ADMIN-FLEET-5).
	AuthUsers UserResolver
	// VehicleStatus derives a vehicle's status in the fleet detail view. Nil
	// simply omits status; the list view never uses it at all.
	VehicleStatus VehicleStatusDeriver
	Window        time.Duration
	Now           func() time.Time
}

// UserResolver is the slice of adminclient.AuthClient the browse endpoints need.
type UserResolver interface {
	Users(ctx context.Context, ids []string) (map[string]adminclient.User, error)
	ListUsers(ctx context.Context, page server.Page) ([]adminclient.User, int, error)
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

// Actor is who is performing a lifecycle action, for the audit row.
type Actor struct {
	UserID        string
	Email         string
	CorrelationID string
}

// Cancel restores every row the operation stamped and, once every service has
// restored, marks it cancelled (FR-ADMIN-RESTORE-1).
//
// It deliberately performs NO platform-admin re-verification. Applied literally,
// FR-ADMIN-AUTH-7 would include this endpoint — the recovery path. If
// auth-service is unreachable, failing closed here would block the one action
// that undoes a mistake, during the window when undoing it is still possible.
// The caller has already passed RequirePlatformAdmin on a valid token; that is
// the right amount of authority for a REVERSIBLE action (design §5.4).
func (p *Processor) Cancel(ctx context.Context, opID string, actor Actor) (Operation, error) {
	now := p.d.Now()
	op, err := p.d.Provider.GetOperation(opID)
	if err != nil {
		return Operation{}, err
	}
	switch op.Status() {
	case StatusReaped:
		// Irreversible, and the API says so rather than pretending to succeed.
		return Operation{}, server.Detailed(server.ErrConflict,
			"this operation has been reaped; its data is permanently deleted")
	case StatusCancelled:
		// Already done. Idempotent success rather than a confusing 409: the
		// console offers restore on a list that may be a few seconds stale.
		return op, nil
	}

	// Record the INTENT before doing any work. A crash between here and the end
	// of this function must still leave the operation out of the reaper's reach:
	// the operator asked for their data back, and that fact cannot depend on the
	// restore having finished.
	if err := p.d.Administrator.MarkCancelRequested(p.d.DB, opID, now); err != nil {
		p.log.WithError(err).WithField("operation_id", opID).Error("record cancel request")
		return Operation{}, err
	}

	// Local restore first and unconditionally: a downstream failure must not
	// hold the product's own data hostage.
	if err := p.d.DB.Transaction(func(tx *gorm.DB) error { return Restore(tx, opID) }); err != nil {
		p.log.WithError(err).WithField("operation_id", opID).Error("local restore")
		return Operation{}, err
	}

	var failed []string
	for _, d := range p.d.Downstream {
		if _, rerr := d.Restore(ctx, opID); rerr != nil {
			p.log.WithError(rerr).WithFields(logrus.Fields{
				"operation_id": opID, "service": d.Name(),
			}).Warn("downstream restore failed; operation stays cancellable")
			failed = append(failed, d.Name())
		}
	}

	// cancelled only when EVERY service has restored. Restore is idempotent, so
	// the correct user action for a partial cancel is to press it again.
	status := StatusCancelled
	if len(failed) > 0 {
		status = StatusPartial
	}
	if err := p.d.Administrator.SetStatus(p.d.DB, opID, status, failed, now); err != nil {
		return Operation{}, err
	}
	if err := p.d.Administrator.InsertAudit(p.d.DB, AuditEvent{
		ID:               uuid.NewString(),
		ActorUserID:      actor.UserID,
		ActorEmail:       actor.Email,
		Action:           ActionPurgeCancelled,
		Scope:            string(op.Scope()),
		TargetType:       op.TargetType(),
		TargetID:         op.TargetID(),
		TargetLabel:      op.TargetLabel(),
		PurgeOperationID: opID,
		AffectedCounts:   op.AffectedCounts(),
		CorrelationID:    actor.CorrelationID,
		CreatedAt:        now,
	}); err != nil {
		p.log.WithError(err).WithField("operation_id", opID).Error("write cancel audit row")
	}

	p.log.WithFields(logrus.Fields{
		"operation_id": opID, "status": status, "failed_services": failed,
		"actor": actor.UserID, "correlation_id": actor.CorrelationID,
	}).Info("admin purge cancelled")

	return p.d.Provider.GetOperation(opID)
}

// Retry re-attempts the downstream stamps for a partial operation
// (FR-ADMIN-PURGE-9). Every downstream stamp is idempotent on
// purge_operation_id, so this is safe to run repeatedly — which is exactly how
// the console presents it.
//
// It DOES re-verify the platform-admin privilege: unlike cancel, this
// re-attempts a destructive write.
func (p *Processor) Retry(ctx context.Context, opID string, actor Actor) (Operation, error) {
	now := p.d.Now()
	ok, err := p.d.Auth.IsPlatformAdmin(ctx, actor.UserID)
	if err != nil {
		p.log.WithError(err).Error("platform-admin re-verification failed; refusing the retry")
		return Operation{}, err
	}
	if !ok {
		return Operation{}, server.ErrForbidden
	}

	op, err := p.d.Provider.GetOperation(opID)
	if err != nil {
		return Operation{}, err
	}
	// CancelledAt, not just the status: a cancel whose downstream restore failed
	// is still `partial`, and retrying it would re-purge everything the operator
	// had just asked to have back.
	if op.Status() == StatusReaped || op.Status() == StatusCancelled || op.CancelledAt() != nil {
		return Operation{}, server.Detailed(server.ErrConflict,
			"only a pending or partial operation that has not been cancelled can be retried")
	}

	// Re-resolve the media ids for a record purge. The target rows are
	// soft-deleted, not gone, so the resolver still finds them.
	_, mediaIDs, rerr := p.targets.Resolve(op.Root())
	if rerr != nil && !errors.Is(rerr, server.ErrNotFound) {
		return Operation{}, rerr
	}

	counts, failed := p.fanOutPurge(ctx, op, mediaIDs)
	affected := op.AffectedCounts()
	for k, v := range counts {
		affected[k] = v
	}
	status := StatusPending
	if len(failed) > 0 {
		status = StatusPartial
	}
	if err := p.d.Administrator.SetAffected(p.d.DB, opID, affected); err != nil {
		return Operation{}, err
	}
	if err := p.d.Administrator.SetStatus(p.d.DB, opID, status, failed, now); err != nil {
		return Operation{}, err
	}
	if err := p.d.Administrator.InsertAudit(p.d.DB, AuditEvent{
		ID:               uuid.NewString(),
		ActorUserID:      actor.UserID,
		ActorEmail:       actor.Email,
		Action:           ActionPurgeRetried,
		Scope:            string(op.Scope()),
		TargetType:       op.TargetType(),
		TargetID:         op.TargetID(),
		TargetLabel:      op.TargetLabel(),
		PurgeOperationID: opID,
		AffectedCounts:   affected,
		CorrelationID:    actor.CorrelationID,
		CreatedAt:        now,
	}); err != nil {
		p.log.WithError(err).WithField("operation_id", opID).Error("write retry audit row")
	}

	p.log.WithFields(logrus.Fields{
		"operation_id": opID, "status": status, "failed_services": failed,
		"actor": actor.UserID, "correlation_id": actor.CorrelationID,
	}).Info("admin purge retried")

	return p.d.Provider.GetOperation(opID)
}
