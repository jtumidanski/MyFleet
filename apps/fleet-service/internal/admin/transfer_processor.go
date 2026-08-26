package admin

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// TransferPreview is the blast radius of a transfer that has not happened.
type TransferPreview struct {
	VehicleID            string
	VehicleLabel         string
	SourceFleetID        string
	SourceFleetName      string
	DestinationFleetID   string
	DestinationFleetName string
	Counts               map[string]int
	CategoriesToCreate   []CategoryToCreate
	// Warnings carries degradation notes, in the same spirit as the admin list
	// endpoints. It is always non-nil so it serialises as [] rather than null.
	Warnings []string
}

// TransferInput is the validated request body plus the caller's identity.
type TransferInput struct {
	VehicleID     string
	DestFleetID   string
	Confirmation  string
	ActorUserID   string
	ActorEmail    string
	CorrelationID string
}

// TransferResult is what the endpoint returns on success.
//
// AffectedCounts is NOT uniformly "rows this transfer moved". The
// fleet-service-local keys are exactly that, counted from the same predicates
// the writes use. The two downstream keys — media_objects and notifications —
// are the reassign endpoints' READ-BACK of the destination fleet; see
// mergeDownstreamCount for the consequence.
type TransferResult struct {
	VehicleID          string
	SourceFleetID      string
	DestinationFleetID string
	TransferredAt      time.Time
	AffectedCounts     map[string]int
}

// Conflict, validation and not-found errors, each carrying the sentence
// FR-XFER-UI-7 surfaces verbatim in the console.
//
// The 404s are Detailed too, not bare server.ErrNotFound: the API contract
// requires EVERY 4xx to carry an actionable detail, and a bare sentinel has no
// Detail() at all, so "unknown vehicle" would reach the operator as an empty
// message.
var (
	errTransferVehicleNotFound = server.Detailed(server.ErrNotFound,
		"vehicle not found")
	errTransferVehicleDeleted = server.Detailed(server.ErrNotFound,
		"vehicle has been deleted and cannot be transferred")
	errSourceFleetNotFound = server.Detailed(server.ErrNotFound,
		"the vehicle's current fleet no longer exists")
	errDestinationNotFound = server.Detailed(server.ErrNotFound,
		"destination fleet not found")
	errVehiclePendingPurge = server.Detailed(server.ErrConflict,
		"vehicle is pending purge and cannot be transferred")
	errSourcePendingPurge = server.Detailed(server.ErrConflict,
		"source fleet is pending purge and cannot be transferred from")
	errDestinationUnavailable = server.Detailed(server.ErrConflict,
		"destination fleet is not available")
	errSameFleet = server.Detailed(server.ErrValidation,
		"vehicle already belongs to that fleet")
	errDestinationRequired = server.Detailed(server.ErrValidation,
		"destination_fleet_id is required")
	// The 403 is Detailed for the same reason the 404s are: server.ErrForbidden
	// is a bare sentinel with no Detail(), so an operator whose platform-admin
	// grant was revoked mid-session would otherwise see an empty message and no
	// way to tell "you lost the role" from "the service is broken".
	errTransferForbidden = server.Detailed(server.ErrForbidden,
		"you are no longer a platform administrator; ask an existing platform administrator to restore the role, then sign in again")
)

// transferVehicleRow is the slice of fleet.vehicles the transfer reads.
//
// PurgeOperationID is read because "pending purge" and "deleted" are DIFFERENT
// states and the API contract answers them differently: pending purge is an
// admin STAMP (deleted_at AND purge_operation_id) and is a 409, while a
// user-deleted vehicle is a 404. Neither may be transferred, and neither is
// stopped by ApplyTransfer — its vehicles UPDATE keys on id alone — so this is
// the only guard.
type transferVehicleRow struct {
	ID               string
	FleetID          string
	Nickname         string
	Make             string
	Model            string
	Year             int
	DeletedAt        *time.Time
	PurgeOperationID *string
}

// pendingPurge is the admin-stamped state, not merely "deleted".
func (v transferVehicleRow) pendingPurge() bool {
	return v.DeletedAt != nil && v.PurgeOperationID != nil
}

// label is the confirmation phrase (FR-XFER-CONF-2): the nickname when there is
// one, otherwise "{year} {make} {model}".
//
// It is computed ONCE, server-side, and returned by the preview, so the console
// echoes the server's string rather than deriving its own. Two independent
// derivations of a phrase that must match exactly is a bug waiting for the
// first vehicle with a double space in its model name.
func (v transferVehicleRow) label() string {
	if v.Nickname != "" {
		return v.Nickname
	}
	return strconv.Itoa(v.Year) + " " + v.Make + " " + v.Model
}

// transferFleetRow is the slice of fleet.fleets the eligibility checks read.
type transferFleetRow struct {
	ID               string
	Name             string
	DeletedAt        *time.Time
	PurgeOperationID *string
}

// pendingPurge mirrors the console's own definition: admin-STAMPED, not merely
// deleted. A fleet a user deleted is a different state.
func (f transferFleetRow) pendingPurge() bool {
	return f.DeletedAt != nil && f.PurgeOperationID != nil
}

// loadTransferVehicle reads one vehicle INCLUDING soft-deleted ones, because
// telling "pending purge" (409) apart from "deleted" (404) apart from "never
// existed" (404, different sentence) requires seeing the row.
//
// notFound is the caller's sentinel for "no such row", so the same loader can
// answer a preview and a transfer without either inventing an error the other
// would have to translate.
//
// COALESCE rather than a nullable Go field: nickname, make and model are plain
// Go strings on the vehicle entity, so GORM leaves them nullable in Postgres
// and the SQLite fixture stores NULL for an unset nickname. COALESCE is
// portable to both, and "no nickname" is then the empty string everywhere —
// the same convention primary_image_media_id already uses.
func loadTransferVehicle(db *gorm.DB, vehicleID string, notFound error) (transferVehicleRow, error) {
	var rows []transferVehicleRow
	q := `SELECT id, fleet_id, COALESCE(nickname, '') AS nickname,
	             COALESCE(make, '') AS make, COALESCE(model, '') AS model,
	             COALESCE(year, 0) AS year, deleted_at, purge_operation_id
	        FROM fleet.vehicles WHERE id = ?`
	if err := db.Raw(q, vehicleID).Scan(&rows).Error; err != nil {
		return transferVehicleRow{}, fmt.Errorf("load vehicle: %w", err)
	}
	if len(rows) == 0 {
		return transferVehicleRow{}, notFound
	}
	return rows[0], nil
}

// loadTransferFleet reads one fleet, deleted or not. notFound is supplied by the
// caller so a missing SOURCE fleet and a missing DESTINATION fleet produce
// different sentences even though the query is the same.
func loadTransferFleet(db *gorm.DB, fleetID string, notFound error) (transferFleetRow, error) {
	var rows []transferFleetRow
	q := `SELECT id, COALESCE(name, '') AS name, deleted_at, purge_operation_id
	        FROM fleet.fleets WHERE id = ?`
	if err := db.Raw(q, fleetID).Scan(&rows).Error; err != nil {
		return transferFleetRow{}, fmt.Errorf("load fleet: %w", err)
	}
	if len(rows) == 0 {
		return transferFleetRow{}, notFound
	}
	return rows[0], nil
}

// lockVehicle serialises concurrent transfers of the same vehicle. It is the
// FIRST statement in the transaction, so every check and write after it is
// ordered per vehicle rather than interleaved.
//
// The dialect branch is on a LOCKING CLAUSE, not on a predicate: the rows
// selected are identical on both dialects, so the tested path and the
// production path still agree about WHAT they touch. SQLite has no FOR UPDATE
// and needs none — the harness holds a single connection and serialises whole
// transactions anyway.
//
// The selected id is USED, not discarded: an empty result is an unknown vehicle,
// and answering that here saves the load below and keeps the lock statement
// from being a query whose result nothing reads.
func lockVehicle(tx *gorm.DB, vehicleID string) error {
	q := `SELECT id FROM fleet.vehicles WHERE id = ? FOR UPDATE`
	if tx.Name() == "sqlite" {
		q = `SELECT id FROM fleet.vehicles WHERE id = ?`
	}
	var ids []string
	if err := tx.Raw(q, vehicleID).Scan(&ids).Error; err != nil {
		return fmt.Errorf("lock vehicle: %w", err)
	}
	if len(ids) == 0 {
		return errTransferVehicleNotFound
	}
	return nil
}

// PreviewVehicleTransfer computes the blast radius without writing anything and
// without calling any other service.
//
// destFleetID is optional. Without it the destination fields and
// categories_to_create are omitted, because neither can be computed without
// knowing where the car is going — while widgets_removed, a SOURCE-fleet fact,
// is always available.
//
// counts.media_objects is the size of the vehicle's media-id union: "media
// references held by this vehicle". The applied transfer reports
// media-service's read-back instead, and the two agree whenever every reference
// resolves — which is the normal case. They diverge only for a PRE-EXISTING
// dangling reference, which the transfer surfaces rather than causes, and the
// handler logs the difference (design D7).
//
// notifications is deliberately absent: the preview makes no downstream call,
// and notification-service owns the vehicle -> notification relationship
// entirely, so there is no number fleet-service could honestly report here.
func (p *Processor) PreviewVehicleTransfer(ctx context.Context, vehicleID, destFleetID string) (TransferPreview, error) {
	// The read path honours cancellation: the console re-previews on every
	// keystroke of the destination picker, so an abandoned request should stop
	// costing the database rather than run to completion.
	db := p.d.DB.WithContext(ctx)

	v, err := loadTransferVehicle(db, vehicleID, errTransferVehicleNotFound)
	if err != nil {
		return TransferPreview{}, err
	}
	src, err := loadTransferFleet(db, v.FleetID, errSourceFleetNotFound)
	if err != nil {
		return TransferPreview{}, err
	}

	counts, err := CountTransfer(db, vehicleID)
	if err != nil {
		return TransferPreview{}, err
	}
	mediaIDs, err := VehicleMediaIDs(db, vehicleID)
	if err != nil {
		return TransferPreview{}, err
	}
	counts["media_objects"] = len(mediaIDs)

	widgetIDs, err := WidgetsPinnedToVehicle(db, v.FleetID, vehicleID)
	if err != nil {
		return TransferPreview{}, err
	}
	counts["widgets_removed"] = len(widgetIDs)

	out := TransferPreview{
		VehicleID:          v.ID,
		VehicleLabel:       v.label(),
		SourceFleetID:      src.ID,
		SourceFleetName:    src.Name,
		Counts:             counts,
		CategoriesToCreate: []CategoryToCreate{},
		Warnings:           []string{},
	}
	if destFleetID == "" {
		counts["categories_created"] = 0
		return out, nil
	}

	dst, err := loadTransferFleet(db, destFleetID, errDestinationNotFound)
	if err != nil {
		return TransferPreview{}, err
	}
	out.DestinationFleetID = dst.ID
	out.DestinationFleetName = dst.Name

	spec := TransferSpec{VehicleID: v.ID, SourceFleetID: v.FleetID, DestFleetID: destFleetID}
	toCreate, err := PreviewCategories(db, spec)
	if err != nil {
		return TransferPreview{}, err
	}
	out.CategoriesToCreate = toCreate
	counts["categories_created"] = len(toCreate)
	return out, nil
}

// TransferVehicle moves one vehicle, with its history, to another fleet.
//
// Structure (design D4):
//
//  1. Re-verify platform admin against auth-service, FAIL CLOSED — this is an
//     irreversible-ish write, so it gets the same treatment purge Create does.
//  2. ONE transaction. Lock the vehicle FIRST, then run every eligibility and
//     confirmation check, then every local write, then the two downstream calls,
//     then the audit row.
//  3. Any failure inside the transaction rolls back all of it. If a downstream
//     had already succeeded, compensate it with a reverse Reassign — safe
//     because both reassign endpoints are idempotent.
//
// The downstream calls are made LAST so that a local failure short-circuits
// before either is issued, which is what turns compensation from the common
// path into a rare one. The audit row goes after them, not before, because
// FR-XFER-AUDIT-3 requires media_objects and notifications inside
// affected_counts and those numbers only exist once the calls have returned.
//
// This does hold a database transaction open across two HTTP calls. That is
// normally an anti-pattern; here it is bounded by adminclient's 5s timeout, the
// operation is platform-admin-only and rare, and the alternative ordering
// leaves a concurrency gap the vehicle lock exists to close. If it ever becomes
// a problem the fix is a queue, not a reordering.
//
// The transaction deliberately runs on p.d.DB and NOT p.d.DB.WithContext(ctx),
// the opposite of PreviewVehicleTransfer: the preview is a throwaway read that
// should stop costing the database the moment the console abandons it, whereas
// this is a write that has already moved rows and may have moved media in
// another service. A cancellation mid-transaction would abort the local
// statements — including the compensating path's ability to run — so the write
// is allowed to finish or fail on its own terms.
func (p *Processor) TransferVehicle(ctx context.Context, in TransferInput) (TransferResult, error) {
	now := p.d.Now()

	ok, err := p.d.Auth.IsPlatformAdmin(ctx, in.ActorUserID)
	if err != nil {
		p.log.WithError(err).WithField("actor", in.ActorUserID).
			Error("platform-admin re-verification failed; refusing the transfer")
		return TransferResult{}, err
	}
	if !ok {
		return TransferResult{}, errTransferForbidden
	}
	if in.DestFleetID == "" {
		return TransferResult{}, errDestinationRequired
	}

	var (
		spec     TransferSpec
		counts   map[string]int
		mediaIDs []string
		done     downstreamState
	)

	terr := p.d.DB.Transaction(func(tx *gorm.DB) error {
		if lerr := lockVehicle(tx, in.VehicleID); lerr != nil {
			return lerr
		}

		v, cerr := p.checkEligibility(tx, in)
		if cerr != nil {
			return cerr
		}

		spec = TransferSpec{
			VehicleID:     v.ID,
			SourceFleetID: v.FleetID,
			DestFleetID:   in.DestFleetID,
			Label:         v.label(),
			ActorUserID:   in.ActorUserID,
			Now:           now,
		}

		var merr error
		if mediaIDs, merr = VehicleMediaIDs(tx, v.ID); merr != nil {
			return merr
		}
		if counts, merr = ApplyTransfer(tx, spec); merr != nil {
			return merr
		}

		var derr error
		if done, derr = p.callDownstreams(ctx, in, spec, mediaIDs, counts); derr != nil {
			return derr
		}

		return p.d.Administrator.InsertAudit(tx, AuditEvent{
			ID:                 uuid.NewString(),
			ActorUserID:        in.ActorUserID,
			ActorEmail:         in.ActorEmail,
			Action:             ActionVehicleTransferred,
			TargetType:         "vehicle",
			TargetID:           spec.VehicleID,
			TargetLabel:        spec.Label,
			SourceFleetID:      spec.SourceFleetID,
			DestinationFleetID: spec.DestFleetID,
			AffectedCounts:     counts,
			CorrelationID:      in.CorrelationID,
			CreatedAt:          now,
		})
	})

	if terr != nil {
		// Reached either because a downstream failed after its predecessor
		// succeeded, or because the COMMIT itself failed after both did. Both
		// are the same repair: put the downstreams back the way they were.
		p.compensate(ctx, spec, mediaIDs, done)
		if !isClientError(terr) && !errorsIsUnavailable(terr) {
			p.log.WithError(terr).WithFields(logrus.Fields{
				"vehicle_id": in.VehicleID, "correlation_id": in.CorrelationID,
			}).Error("vehicle transfer transaction")
		}
		return TransferResult{}, terr
	}

	p.log.WithFields(logrus.Fields{
		"vehicle_id":     spec.VehicleID,
		"source_fleet":   spec.SourceFleetID,
		"dest_fleet":     spec.DestFleetID,
		"actor":          in.ActorUserID,
		"correlation_id": in.CorrelationID,
		"affected":       counts,
	}).Info("admin vehicle transferred")

	return TransferResult{
		VehicleID:          spec.VehicleID,
		SourceFleetID:      spec.SourceFleetID,
		DestinationFleetID: spec.DestFleetID,
		TransferredAt:      now,
		AffectedCounts:     counts,
	}, nil
}

// checkEligibility runs every gate a transfer must pass, inside the caller's
// transaction and after the vehicle lock, and returns the vehicle it validated.
//
// It exists as its own function because ApplyTransfer does NOT re-check any of
// this: its vehicles UPDATE keys on id alone, with no deleted_at or
// purge_operation_id predicate, so a soft-deleted or admin-stamped vehicle
// would be moved without complaint. Every guard below is load-bearing.
//
// Order is cheapest and most specific first, so the operator gets the most
// actionable message. Confirmation is LAST: a mismatched phrase on an
// otherwise-invalid request should report the real problem.
func (p *Processor) checkEligibility(tx *gorm.DB, in TransferInput) (transferVehicleRow, error) {
	v, err := loadTransferVehicle(tx, in.VehicleID, errTransferVehicleNotFound)
	if err != nil {
		return transferVehicleRow{}, err
	}
	switch {
	case v.pendingPurge():
		return transferVehicleRow{}, errVehiclePendingPurge
	case v.DeletedAt != nil:
		// Deleted by a user, not stamped by an admin. It is not pending purge
		// and the console cannot see it, so it answers as unknown rather than
		// as a conflict.
		return transferVehicleRow{}, errTransferVehicleDeleted
	}
	if in.DestFleetID == v.FleetID {
		return transferVehicleRow{}, errSameFleet
	}

	src, err := loadTransferFleet(tx, v.FleetID, errSourceFleetNotFound)
	if err != nil {
		return transferVehicleRow{}, err
	}
	if src.pendingPurge() {
		// Refused because the outcome would depend on whether the reaper
		// runs before or after the move (FR-XFER-ELIG-5).
		return transferVehicleRow{}, errSourcePendingPurge
	}

	dst, err := loadTransferFleet(tx, in.DestFleetID, errDestinationNotFound)
	if err != nil {
		return transferVehicleRow{}, err
	}
	if dst.DeletedAt != nil {
		// Deleted OR pending purge: either way the vehicle would land somewhere
		// that is on its way out, so both are "not available".
		return transferVehicleRow{}, errDestinationUnavailable
	}

	// ScopeFleet is passed for its COMPARISON RULE — exact, no trimming, no
	// case folding — not because a transfer is a fleet purge. Adding a
	// ScopeVehicleTransfer would leak into ValidScopes and the purge builder
	// for no gain.
	if err := MatchConfirmation(ScopeFleet, v.label(), in.Confirmation); err != nil {
		return transferVehicleRow{}, err
	}
	return v, nil
}

// downstreamState records which downstream moves actually landed, so a later
// failure knows exactly what to compensate. It is returned even on error —
// that is the whole point: the media move can have succeeded and the
// notification move failed.
type downstreamState struct {
	media bool
	notif bool
}

// callDownstreams issues the media and notification reassigns, in that order,
// and merges their read-backs into counts.
func (p *Processor) callDownstreams(ctx context.Context, in TransferInput, spec TransferSpec,
	mediaIDs []string, counts map[string]int,
) (downstreamState, error) {
	var done downstreamState

	// A vehicle with no media must not send an empty list: both reassign
	// endpoints answer 422 to one, which would read as a failed service.
	counts["media_objects"] = 0
	if p.d.MediaReassign != nil && len(mediaIDs) > 0 {
		got, merr := p.d.MediaReassign.Reassign(ctx, mediaIDs, spec.DestFleetID)
		if merr != nil {
			p.log.WithError(merr).WithFields(logrus.Fields{
				"vehicle_id": spec.VehicleID, "correlation_id": in.CorrelationID,
			}).Error("media reassign failed; rolling the transfer back")
			return done, server.Detailed(server.ErrServiceUnavailable,
				"media-service could not reassign the vehicle's media; the transfer was rolled back")
		}
		done.media = true
		mergeDownstreamCount(counts, "media_objects", got)
		if got["media_objects"] != len(mediaIDs) {
			// A pre-existing dangling reference, surfaced rather than
			// hidden: the preview counted references, media-service counted
			// objects that exist.
			p.log.WithFields(logrus.Fields{
				"vehicle_id": spec.VehicleID,
				"references": len(mediaIDs),
				"objects":    got["media_objects"],
			}).Info("vehicle media references exceed the media objects that exist")
		}
	}

	counts["notifications"] = 0
	if p.d.NotificationReassign != nil {
		got, nerr := p.d.NotificationReassign.Reassign(ctx, []string{spec.VehicleID}, spec.DestFleetID)
		if nerr != nil {
			p.log.WithError(nerr).WithFields(logrus.Fields{
				"vehicle_id": spec.VehicleID, "correlation_id": in.CorrelationID,
			}).Error("notification reassign failed; rolling the transfer back")
			return done, server.Detailed(server.ErrServiceUnavailable,
				"notification-service could not reassign the vehicle's notifications; the transfer was rolled back")
		}
		done.notif = true
		mergeDownstreamCount(counts, "notifications", got)
	}
	return done, nil
}

// mergeDownstreamCount copies one downstream read-back into affected_counts.
//
// It exists to hold this comment, because the number it copies does not mean
// what its neighbours in the map mean. Every fleet-service-local key in
// affected_counts is "rows THIS transfer moved", counted from the same
// predicate the UPDATE used. media_objects and notifications are not: both
// reassign endpoints return count(*) of LIVE rows now on the DESTINATION fleet
// for the named ids (see MediaReassigner). A media object or notification that
// was already on the destination — a re-run, a prior partial transfer, or
// pre-existing state — is included in that number and was not moved by this
// call.
//
// Reconciling it is not possible from here: the downstream would have to report
// rows-changed, which neither endpoint does and which would also make them
// non-idempotent to read. So the semantic is documented at the point the
// admin-facing value is produced, and the REST layer (Task 10) and the console
// must label these two keys as "now in the destination fleet", not "moved".
func mergeDownstreamCount(counts map[string]int, key string, got map[string]int) {
	counts[key] = got[key]
}

// compensate reverses whichever downstream moves succeeded before the
// transaction failed, sending everything back to the SOURCE fleet.
//
// Safe to attempt because both reassign endpoints are idempotent
// (FR-XFER-MEDIA-4). If a compensating call ALSO fails there is nothing further
// this process can do, so it logs at error naming both fleets and every id — an
// operator with that line can finish the repair by hand, which they cannot do
// from a bare "transfer failed".
//
// The compensating calls run on a context DETACHED from the request's
// cancellation. The commonest reason the notification call failed is that the
// client disconnected or the request deadline expired — and that is exactly the
// context that would be handed to the repair, which would then fail instantly
// with context.Canceled and strand media on the destination fleet in the one
// scenario compensation exists for. context.WithoutCancel (not
// context.Background) because the request's VALUES must survive: the
// correlation id rides on the context and the downstream clients propagate it,
// so a fresh Background context would make the repair unattributable in the
// logs of the very service that has the stranded rows.
func (p *Processor) compensate(ctx context.Context, spec TransferSpec, mediaIDs []string, done downstreamState) {
	ctx = context.WithoutCancel(ctx)

	if done.media && p.d.MediaReassign != nil {
		if _, err := p.d.MediaReassign.Reassign(ctx, mediaIDs, spec.SourceFleetID); err != nil {
			p.log.WithError(err).WithFields(logrus.Fields{
				"vehicle_id": spec.VehicleID, "source_fleet": spec.SourceFleetID,
				"dest_fleet": spec.DestFleetID, "media_ids": mediaIDs,
			}).Error("compensating media reassign FAILED; media is stranded in the destination fleet")
		}
	}
	if done.notif && p.d.NotificationReassign != nil {
		if _, err := p.d.NotificationReassign.Reassign(ctx, []string{spec.VehicleID}, spec.SourceFleetID); err != nil {
			p.log.WithError(err).WithFields(logrus.Fields{
				"vehicle_id": spec.VehicleID, "source_fleet": spec.SourceFleetID,
				"dest_fleet": spec.DestFleetID,
			}).Error("compensating notification reassign FAILED; notifications are stranded in the destination fleet")
		}
	}
}

// errorsIsUnavailable keeps a 503 out of the generic transaction error log: it
// is already logged, with the downstream's own error attached, at the point it
// happened.
func errorsIsUnavailable(err error) bool {
	return errors.Is(err, server.ErrServiceUnavailable)
}
