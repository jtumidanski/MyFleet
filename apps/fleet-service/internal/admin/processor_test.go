package admin_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin/admintest"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/adminclient"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

type stubAuth struct {
	admin bool
	err   error
}

func (s stubAuth) IsPlatformAdmin(context.Context, string) (bool, error) { return s.admin, s.err }

type stubDownstream struct {
	name      string
	purgeErr  error
	purgeCall int
	restored  int
	reaped    int
}

func (s *stubDownstream) Name() string { return s.name }
func (s *stubDownstream) Purge(context.Context, adminclient.PurgeRequest) (map[string]int, error) {
	s.purgeCall++
	if s.purgeErr != nil {
		return nil, s.purgeErr
	}
	return map[string]int{s.name + "_rows": 7}, nil
}

func (s *stubDownstream) Restore(context.Context, string) (map[string]int, error) {
	s.restored++
	return map[string]int{}, nil
}

func (s *stubDownstream) Reap(context.Context, string) (map[string]int, error) {
	s.reaped++
	return map[string]int{}, nil
}

// stubTargets resolves a label and the media ids a record purge must name.
type stubTargets struct {
	label    string
	mediaIDs []string
	err      error
}

func (s stubTargets) Resolve(admin.Root) (string, []string, error) {
	return s.label, s.mediaIDs, s.err
}

func newProcessor(t *testing.T, db *gorm.DB, auth admin.AuthVerifier, down ...admin.Downstream) *admin.Processor {
	t.Helper()
	return admin.NewProcessor(logrus.New(), admin.Deps{
		DB:            db,
		Provider:      admin.NewProvider(db),
		Administrator: admin.NewAdministrator(db),
		Auth:          auth,
		Downstream:    down,
		Window:        admin.DefaultRecoveryWindow,
		Now:           func() time.Time { return testNow },
	}, stubTargets{label: "Fleet fleet-1"})
}

func fleetInput() admin.CreateInput {
	return admin.CreateInput{
		Scope:         admin.ScopeFleet,
		TargetType:    "fleet",
		TargetID:      "fleet-1",
		Confirmation:  "Fleet fleet-1",
		ActorUserID:   "admin-1",
		ActorEmail:    "admin@example.com",
		CorrelationID: "corr-1",
	}
}

func TestCreate_stampsLocallyAndRecordsTheOperation(t *testing.T) {
	db := admintest.NewDB(t)
	admintest.SeedFleet(t, db, "fleet-1")
	media := &stubDownstream{name: "media"}
	notif := &stubDownstream{name: "notification"}
	proc := newProcessor(t, db, stubAuth{admin: true}, media, notif)

	op, err := proc.Create(context.Background(), fleetInput())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if op.Status() != admin.StatusPending {
		t.Errorf("status = %q, want pending", op.Status())
	}
	if !op.PurgeAfter().Equal(testNow.Add(admin.DefaultRecoveryWindow)) {
		t.Errorf("purge_after = %v, want now + the recovery window", op.PurgeAfter())
	}
	if op.AffectedCounts()["vehicles"] != 2 {
		t.Errorf("affected counts must include the local stamp: %v", op.AffectedCounts())
	}
	if op.AffectedCounts()["media_rows"] != 7 || op.AffectedCounts()["notification_rows"] != 7 {
		t.Errorf("affected counts must merge downstream results: %v", op.AffectedCounts())
	}
	if got := admintest.CountLive(t, db, "fleet.vehicles"); got != 0 {
		t.Errorf("the local stamp did not run: %d live vehicles", got)
	}

	var audits int64
	db.Raw(`SELECT count(*) FROM fleet.admin_audit_events WHERE action = ?`, admin.ActionPurgeCreated).
		Scan(&audits)
	if audits != 1 {
		t.Errorf("want one purge.created audit row, got %d", audits)
	}
}

// FR-ADMIN-PURGE-9: a downstream failure leaves the LOCAL stamp in place, marks
// the operation partial, and names the service. It does NOT roll back.
func TestCreate_downstreamFailureIsPartialNotRollback(t *testing.T) {
	db := admintest.NewDB(t)
	admintest.SeedFleet(t, db, "fleet-1")
	media := &stubDownstream{name: "media", purgeErr: errors.New("connection refused")}
	notif := &stubDownstream{name: "notification"}
	proc := newProcessor(t, db, stubAuth{admin: true}, media, notif)

	op, err := proc.Create(context.Background(), fleetInput())
	if err != nil {
		t.Fatalf("a downstream failure must not fail the request: %v", err)
	}
	if op.Status() != admin.StatusPartial {
		t.Errorf("status = %q, want partial", op.Status())
	}
	if len(op.FailedServices()) != 1 || op.FailedServices()[0] != "media" {
		t.Errorf("failed services = %v, want [media]", op.FailedServices())
	}
	if got := admintest.CountLive(t, db, "fleet.vehicles"); got != 0 {
		t.Errorf("the local stamp must survive a downstream failure: %d live", got)
	}
	if notif.purgeCall != 1 {
		t.Errorf("one service failing must not skip the others, notification called %d times", notif.purgeCall)
	}
}

// FR-ADMIN-PURGE-7 / risks.md R9: a wrong confirmation writes NOTHING.
func TestCreate_confirmationMismatchWritesNothing(t *testing.T) {
	db := admintest.NewDB(t)
	admintest.SeedFleet(t, db, "fleet-1")
	media := &stubDownstream{name: "media"}
	proc := newProcessor(t, db, stubAuth{admin: true}, media)

	in := fleetInput()
	in.Confirmation = "fleet fleet-1"
	if _, err := proc.Create(context.Background(), in); !errors.Is(err, server.ErrConflict) {
		t.Fatalf("want 409, got %v", err)
	}
	if got := admintest.CountLive(t, db, "fleet.vehicles"); got != 2 {
		t.Errorf("a rejected confirmation stamped rows: %d of 2 live", got)
	}
	if got := admintest.CountRows(t, db, "fleet.purge_operations"); got != 0 {
		t.Errorf("a rejected confirmation created an operation row: %d rows", got)
	}
	if media.purgeCall != 0 {
		t.Errorf("a rejected confirmation called downstream %d times", media.purgeCall)
	}
}

// FR-ADMIN-AUTH-7: a revoked admin holding a valid token cannot destroy data.
func TestCreate_revokedAdminIsForbidden(t *testing.T) {
	db := admintest.NewDB(t)
	admintest.SeedFleet(t, db, "fleet-1")
	proc := newProcessor(t, db, stubAuth{admin: false})

	if _, err := proc.Create(context.Background(), fleetInput()); !errors.Is(err, server.ErrForbidden) {
		t.Fatalf("want 403, got %v", err)
	}
	if got := admintest.CountLive(t, db, "fleet.vehicles"); got != 2 {
		t.Errorf("a revoked admin stamped rows: %d of 2 live", got)
	}
}

// design §5.4: create fails CLOSED. Coupling an irreversible write to a
// dependency's availability is the correct trade.
func TestCreate_failsClosedWhenAuthServiceIsUnreachable(t *testing.T) {
	db := admintest.NewDB(t)
	admintest.SeedFleet(t, db, "fleet-1")
	proc := newProcessor(t, db, stubAuth{err: errors.New("connection refused")})

	if _, err := proc.Create(context.Background(), fleetInput()); err == nil {
		t.Fatal("an unreachable auth-service must fail the create, not proceed")
	}
	if got := admintest.CountLive(t, db, "fleet.vehicles"); got != 2 {
		t.Errorf("nothing may be stamped when re-verification could not run: %d of 2 live", got)
	}
}

func TestCreate_rejectsAnUnknownTargetType(t *testing.T) {
	db := admintest.NewDB(t)
	proc := newProcessor(t, db, stubAuth{admin: true})
	in := fleetInput()
	in.Scope = admin.ScopeRecord
	in.TargetType = "spaceship"
	in.TargetID = "x"
	if _, err := proc.Create(context.Background(), in); !errors.Is(err, server.ErrValidation) {
		t.Errorf("want 422, got %v", err)
	}
}

// The system purge's confirmation is the literal phrase, and its operation has
// no target at all.
func TestCreate_systemScope(t *testing.T) {
	db := admintest.NewDB(t)
	admintest.SeedFleet(t, db, "fleet-1")
	admintest.SeedFleet(t, db, "fleet-2")
	proc := newProcessor(t, db, stubAuth{admin: true}, &stubDownstream{name: "media"})

	op, err := proc.Create(context.Background(), admin.CreateInput{
		Scope:        admin.ScopeSystem,
		Confirmation: admin.SystemConfirmation,
		ActorUserID:  "admin-1",
		ActorEmail:   "admin@example.com",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if op.TargetID() != "" {
		t.Errorf("a system operation must have no target, got %q", op.TargetID())
	}
	if got := admintest.CountLive(t, db, "fleet.fleets"); got != 0 {
		t.Errorf("a system purge left %d live fleets", got)
	}
	// PRD blast radius: seeded reference data survives.
	if got := admintest.CountRows(t, db, "fleet.maintenance_categories"); got != 1 {
		t.Errorf("maintenance categories must survive a system purge, got %d", got)
	}
}

type failingRestore struct {
	stubDownstream
	failRestore bool
}

func (f *failingRestore) Restore(ctx context.Context, opID string) (map[string]int, error) {
	if f.failRestore {
		return nil, errors.New("connection refused")
	}
	return f.stubDownstream.Restore(ctx, opID)
}

func TestCancel_restoresEverywhereAndMarksCancelled(t *testing.T) {
	db := admintest.NewDB(t)
	admintest.SeedFleet(t, db, "fleet-1")
	media := &stubDownstream{name: "media"}
	proc := newProcessor(t, db, stubAuth{admin: true}, media)

	op, err := proc.Create(context.Background(), fleetInput())
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := proc.Cancel(context.Background(), op.ID(),
		admin.Actor{UserID: "admin-1", Email: "admin@example.com"})
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if got.Status() != admin.StatusCancelled {
		t.Errorf("status = %q, want cancelled", got.Status())
	}
	if got.CancelledAt() == nil {
		t.Error("cancelled_at must be stamped")
	}
	if media.restored != 1 {
		t.Errorf("media restore called %d times, want 1", media.restored)
	}
	for _, table := range []string{"fleet.fleets", "fleet.vehicles", "fleet.fleet_memberships"} {
		if admintest.CountLive(t, db, table) == 0 {
			t.Errorf("%s was not restored", table)
		}
	}

	var audits int64
	db.Raw(`SELECT count(*) FROM fleet.admin_audit_events WHERE action = ?`,
		admin.ActionPurgeCancelled).Scan(&audits)
	if audits != 1 {
		t.Errorf("want one purge.cancelled audit row, got %d", audits)
	}
}

// design §5.4: cancel must work even when auth-service is down. It is the
// recovery path; blocking it during the window when recovery is still possible
// is the worst available outcome.
func TestCancel_worksWhenAuthServiceIsUnreachable(t *testing.T) {
	db := admintest.NewDB(t)
	admintest.SeedFleet(t, db, "fleet-1")
	proc := newProcessor(t, db, stubAuth{admin: true}, &stubDownstream{name: "media"})
	op, err := proc.Create(context.Background(), fleetInput())
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Auth-service falls over between the create and the cancel.
	broken := newProcessor(t, db, stubAuth{err: errors.New("connection refused")},
		&stubDownstream{name: "media"})
	if _, cerr := broken.Cancel(context.Background(), op.ID(),
		admin.Actor{UserID: "admin-1", Email: "admin@example.com"}); cerr != nil {
		t.Fatalf("cancel must not depend on auth-service: %v", cerr)
	}
	if admintest.CountLive(t, db, "fleet.vehicles") != 2 {
		t.Error("cancel did not restore the vehicles")
	}
}

// A downstream restore that fails leaves the operation PARTIAL and still
// cancellable — restore is idempotent, so pressing it again is the fix.
func TestCancel_downstreamFailureStaysCancellable(t *testing.T) {
	db := admintest.NewDB(t)
	admintest.SeedFleet(t, db, "fleet-1")
	media := &failingRestore{stubDownstream: stubDownstream{name: "media"}, failRestore: true}
	proc := newProcessor(t, db, stubAuth{admin: true}, media)
	op, _ := proc.Create(context.Background(), fleetInput())

	got, err := proc.Cancel(context.Background(), op.ID(), admin.Actor{UserID: "a", Email: "a@x"})
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if got.Status() != admin.StatusPartial {
		t.Errorf("status = %q, want partial while a service has not restored", got.Status())
	}
	// Local rows come back regardless: a downstream failure must not hold the
	// product hostage.
	if admintest.CountLive(t, db, "fleet.vehicles") != 2 {
		t.Error("local restore must run even when a downstream restore fails")
	}
	// And a second cancel completes it.
	media.failRestore = false
	got, err = proc.Cancel(context.Background(), op.ID(), admin.Actor{UserID: "a", Email: "a@x"})
	if err != nil {
		t.Fatalf("second cancel: %v", err)
	}
	if got.Status() != admin.StatusCancelled {
		t.Errorf("a repeated cancel must complete the operation, got %q", got.Status())
	}
}

// FR-ADMIN-RESTORE-2: reaping is irreversible and the API says so.
func TestCancel_onAReapedOperationIs409(t *testing.T) {
	db := admintest.NewDB(t)
	admintest.SeedFleet(t, db, "fleet-1")
	proc := newProcessor(t, db, stubAuth{admin: true}, &stubDownstream{name: "media"})
	op, _ := proc.Create(context.Background(), fleetInput())
	if err := admin.NewAdministrator(db).SetStatus(db, op.ID(), admin.StatusReaped, nil, testNow); err != nil {
		t.Fatalf("mark reaped: %v", err)
	}

	if _, err := proc.Cancel(context.Background(), op.ID(),
		admin.Actor{UserID: "a", Email: "a@x"}); !errors.Is(err, server.ErrConflict) {
		t.Errorf("want 409, got %v", err)
	}
}

// FR-ADMIN-PURGE-9: retry re-attempts the failed downstream stamps without
// double-stamping, and clears the failure once they succeed.
func TestRetry_completesAPartialOperation(t *testing.T) {
	db := admintest.NewDB(t)
	admintest.SeedFleet(t, db, "fleet-1")
	media := &stubDownstream{name: "media", purgeErr: errors.New("connection refused")}
	proc := newProcessor(t, db, stubAuth{admin: true}, media)

	op, err := proc.Create(context.Background(), fleetInput())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if op.Status() != admin.StatusPartial {
		t.Fatalf("fixture expected a partial operation, got %q", op.Status())
	}

	media.purgeErr = nil
	got, err := proc.Retry(context.Background(), op.ID(), admin.Actor{UserID: "a", Email: "a@x"})
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if got.Status() != admin.StatusPending {
		t.Errorf("status = %q, want pending once every service has stamped", got.Status())
	}
	if len(got.FailedServices()) != 0 {
		t.Errorf("failed services must clear on a successful retry, got %v", got.FailedServices())
	}
	if got.AffectedCounts()["media_rows"] != 7 {
		t.Errorf("retry must record the downstream counts: %v", got.AffectedCounts())
	}
	// Local rows are untouched by a retry — the local stamp already succeeded.
	if admintest.CountLive(t, db, "fleet.vehicles") != 0 {
		t.Error("retry must not disturb the local stamp")
	}
}

func TestRetry_onACancelledOperationIs409(t *testing.T) {
	db := admintest.NewDB(t)
	admintest.SeedFleet(t, db, "fleet-1")
	proc := newProcessor(t, db, stubAuth{admin: true}, &stubDownstream{name: "media"})
	op, _ := proc.Create(context.Background(), fleetInput())
	if _, err := proc.Cancel(context.Background(), op.ID(), admin.Actor{UserID: "a", Email: "a@x"}); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if _, err := proc.Retry(context.Background(), op.ID(),
		admin.Actor{UserID: "a", Email: "a@x"}); !errors.Is(err, server.ErrConflict) {
		t.Errorf("want 409 retrying a cancelled operation, got %v", err)
	}
}

// Retry re-attempts a DESTRUCTIVE stamp, so it must refuse an operation whose
// cancel has been requested — including one left `partial` because a downstream
// restore failed. Without this the console offers "Retry" on a row the operator
// just asked to restore, and pressing it re-purges everything.
func TestRetry_refusesAnOperationWhoseCancelWasRequested(t *testing.T) {
	db := admintest.NewDB(t)
	admintest.SeedFleet(t, db, "fleet-1")
	media := &failingRestore{stubDownstream: stubDownstream{name: "media"}, failRestore: true}
	proc := newProcessor(t, db, stubAuth{admin: true}, media)
	op, err := proc.Create(context.Background(), fleetInput())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	before := media.purgeCall

	got, err := proc.Cancel(context.Background(), op.ID(), admin.Actor{UserID: "a", Email: "a@x"})
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if got.Status() != admin.StatusPartial {
		t.Fatalf("fixture expected a partial cancel, got %q", got.Status())
	}

	if _, err := proc.Retry(context.Background(), op.ID(),
		admin.Actor{UserID: "a", Email: "a@x"}); !errors.Is(err, server.ErrConflict) {
		t.Errorf("want 409 retrying a cancelled operation, got %v", err)
	}
	if media.purgeCall != before {
		t.Errorf("retry re-purged downstream on a cancelled operation: %d → %d calls",
			before, media.purgeCall)
	}
	if admintest.CountLive(t, db, "fleet.vehicles") != 2 {
		t.Error("retry re-stamped local rows the cancel had restored")
	}
}
