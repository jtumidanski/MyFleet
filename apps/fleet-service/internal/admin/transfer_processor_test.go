package admin_test

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin/admintest"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// fakeReassigner records every call so a test can assert a downstream was NOT
// reached — never merely "not yet", which is what task-019 is about.
type fakeReassigner struct {
	calls [][]string
	dests []string
	ret   map[string]int
	err   error
}

func (f *fakeReassigner) Reassign(_ context.Context, ids []string, dest string) (map[string]int, error) {
	f.calls = append(f.calls, ids)
	f.dests = append(f.dests, dest)
	if f.err != nil {
		return nil, f.err
	}
	return f.ret, nil
}

type fakeAuth struct {
	ok  bool
	err error
}

func (f fakeAuth) IsPlatformAdmin(context.Context, string) (bool, error) { return f.ok, f.err }

type transferHarness struct {
	db    *gorm.DB
	proc  *admin.Processor
	media *fakeReassigner
	notif *fakeReassigner
	src   admintest.Fixture
	dst   admintest.Fixture
}

func newTransferHarness(t *testing.T) transferHarness {
	t.Helper()
	db := admintest.NewDB(t)
	src := admintest.SeedFleet(t, db, "fleet-a")
	dst := admintest.SeedFleet(t, db, "fleet-b")
	media := &fakeReassigner{ret: map[string]int{"media_objects": 1}}
	notif := &fakeReassigner{ret: map[string]int{"notifications": 2}}

	log := logrus.New()
	log.SetOutput(io.Discard)
	proc := admin.NewProcessor(log, admin.Deps{
		DB:                   db,
		Provider:             admin.NewProvider(db),
		Administrator:        admin.NewAdministrator(db),
		Auth:                 fakeAuth{ok: true},
		MediaReassign:        media,
		NotificationReassign: notif,
		Now:                  func() time.Time { return time.Date(2026, 8, 25, 18, 0, 0, 0, time.UTC) },
	}, admin.NewTargetResolver(db))

	return transferHarness{db: db, proc: proc, media: media, notif: notif, src: src, dst: dst}
}

func (h transferHarness) input(confirmation string) admin.TransferInput {
	return admin.TransferInput{
		VehicleID:     h.src.VehicleID,
		DestFleetID:   h.dst.FleetID,
		Confirmation:  confirmation,
		ActorUserID:   "admin-1",
		ActorEmail:    "admin@example.com",
		CorrelationID: "corr-1",
	}
}

// SeedFleet's vehicle has no nickname, so the label is "{year} {make} {model}".
const seededLabel = "2020 Toyota Corolla"

func TestTransferVehicle_happyPath(t *testing.T) {
	h := newTransferHarness(t)

	res, err := h.proc.TransferVehicle(context.Background(), h.input(seededLabel))
	if err != nil {
		t.Fatalf("transfer: %v", err)
	}
	if res.SourceFleetID != "fleet-a" || res.DestinationFleetID != "fleet-b" {
		t.Errorf("result fleets = %s -> %s", res.SourceFleetID, res.DestinationFleetID)
	}
	if got := scanOne[string](t, h.db, `SELECT fleet_id FROM fleet.vehicles WHERE id = ?`,
		h.src.VehicleID); got != "fleet-b" {
		t.Errorf("fleet_id = %q, want fleet-b", got)
	}
	// Downstream counts are merged into affected_counts (FR-XFER-AUDIT-3).
	if res.AffectedCounts["media_objects"] != 1 {
		t.Errorf("media_objects = %d, want 1", res.AffectedCounts["media_objects"])
	}
	if res.AffectedCounts["notifications"] != 2 {
		t.Errorf("notifications = %d, want 2", res.AffectedCounts["notifications"])
	}
	if len(h.media.calls) != 1 || h.media.dests[0] != "fleet-b" {
		t.Errorf("media calls = %v to %v", h.media.calls, h.media.dests)
	}
	if len(h.notif.calls) != 1 || h.notif.calls[0][0] != h.src.VehicleID {
		t.Errorf("notification call = %v, want the vehicle id", h.notif.calls)
	}
}

func TestTransferVehicle_writesTheAuditRow(t *testing.T) {
	h := newTransferHarness(t)
	if _, err := h.proc.TransferVehicle(context.Background(), h.input(seededLabel)); err != nil {
		t.Fatalf("transfer: %v", err)
	}

	events, total, err := h.proc.ListAuditEvents(
		admin.AuditFilter{Action: admin.ActionVehicleTransferred}, server.Page{Number: 1, Size: 10})
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if total != 1 {
		t.Fatalf("audit rows = %d, want 1", total)
	}
	a := events[0]
	if a.ActorUserID != "admin-1" || a.ActorEmail != "admin@example.com" {
		t.Errorf("actor = %s/%s", a.ActorUserID, a.ActorEmail)
	}
	if a.TargetType != "vehicle" || a.TargetID != h.src.VehicleID {
		t.Errorf("target = %s/%s", a.TargetType, a.TargetID)
	}
	if a.TargetLabel != seededLabel {
		t.Errorf("target_label = %q, want %q", a.TargetLabel, seededLabel)
	}
	if a.CorrelationID != "corr-1" {
		t.Errorf("correlation_id = %q", a.CorrelationID)
	}
	if a.SourceFleetID != "fleet-a" || a.DestinationFleetID != "fleet-b" {
		t.Errorf("audit fleets = %s -> %s", a.SourceFleetID, a.DestinationFleetID)
	}
	if a.PurgeOperationID != "" {
		t.Errorf("purge_operation_id = %q, want empty — a transfer is not a purge", a.PurgeOperationID)
	}
	for _, k := range []string{
		"maintenance_records", "maintenance_schedules", "fuel_logs", "mileage_records",
		"vehicle_media", "media_objects", "notifications", "activity_events",
		"categories_created", "widgets_removed",
	} {
		if _, ok := a.AffectedCounts[k]; !ok {
			t.Errorf("affected_counts is missing %q (FR-XFER-AUDIT-3)", k)
		}
	}
}

// FR-XFER-CONF-3, asserted as three NEGATIVES: nothing local changed, no audit
// row exists, and neither downstream was called at all.
func TestTransferVehicle_confirmationMismatchWritesNothing(t *testing.T) {
	h := newTransferHarness(t)

	_, err := h.proc.TransferVehicle(context.Background(), h.input("2020 toyota corolla"))
	if !errors.Is(err, server.ErrConflict) {
		t.Fatalf("err = %v, want a 409 conflict", err)
	}
	if got := scanOne[string](t, h.db, `SELECT fleet_id FROM fleet.vehicles WHERE id = ?`,
		h.src.VehicleID); got != "fleet-a" {
		t.Errorf("fleet_id = %q, want the untouched fleet-a", got)
	}
	if n := scanOne[int](t, h.db, `SELECT count(*) FROM fleet.admin_audit_events`); n != 0 {
		t.Errorf("audit rows = %d, want 0", n)
	}
	if len(h.media.calls) != 0 {
		t.Errorf("media was called %d times, want 0", len(h.media.calls))
	}
	if len(h.notif.calls) != 0 {
		t.Errorf("notification was called %d times, want 0", len(h.notif.calls))
	}
}

func TestTransferVehicle_revokedAdminIsForbidden(t *testing.T) {
	h := newTransferHarness(t)
	log := logrus.New()
	log.SetOutput(io.Discard)
	proc := admin.NewProcessor(log, admin.Deps{
		DB: h.db, Provider: admin.NewProvider(h.db), Administrator: admin.NewAdministrator(h.db),
		Auth: fakeAuth{ok: false}, MediaReassign: h.media, NotificationReassign: h.notif,
	}, admin.NewTargetResolver(h.db))

	if _, err := proc.TransferVehicle(context.Background(), h.input(seededLabel)); !errors.Is(err, server.ErrForbidden) {
		t.Fatalf("err = %v, want forbidden", err)
	}
	if len(h.media.calls) != 0 {
		t.Error("media was called for a revoked admin")
	}
}

func TestTransferVehicle_eligibilityBranches(t *testing.T) {
	cases := []struct {
		name    string
		setup   func(t *testing.T, h transferHarness) admin.TransferInput
		wantErr error
	}{
		{
			name: "unknown vehicle",
			setup: func(_ *testing.T, h transferHarness) admin.TransferInput {
				in := h.input(seededLabel)
				in.VehicleID = "no-such-vehicle"
				return in
			},
			wantErr: server.ErrNotFound,
		},
		{
			name: "vehicle pending purge",
			setup: func(t *testing.T, h transferHarness) admin.TransferInput {
				if err := h.db.Exec(`UPDATE fleet.vehicles SET deleted_at = ?, purge_operation_id = 'op-1' WHERE id = ?`,
					seedNow(), h.src.VehicleID).Error; err != nil {
					t.Fatalf("stamp vehicle: %v", err)
				}
				return h.input(seededLabel)
			},
			wantErr: server.ErrConflict,
		},
		{
			// A user-deleted vehicle is a DIFFERENT state from pending purge:
			// deleted_at is set but no admin stamped a purge_operation_id. It is
			// not transferable either, but it is not a 409 — the console cannot
			// see it at all, so it reads as unknown.
			name: "user-deleted vehicle",
			setup: func(t *testing.T, h transferHarness) admin.TransferInput {
				if err := h.db.Exec(`UPDATE fleet.vehicles SET deleted_at = ? WHERE id = ?`,
					seedNow(), h.src.VehicleID).Error; err != nil {
					t.Fatalf("delete vehicle: %v", err)
				}
				return h.input(seededLabel)
			},
			wantErr: server.ErrNotFound,
		},
		{
			name: "source fleet pending purge",
			setup: func(t *testing.T, h transferHarness) admin.TransferInput {
				if err := h.db.Exec(`UPDATE fleet.fleets SET deleted_at = ?, purge_operation_id = 'op-1' WHERE id = 'fleet-a'`,
					seedNow()).Error; err != nil {
					t.Fatalf("stamp source fleet: %v", err)
				}
				return h.input(seededLabel)
			},
			wantErr: server.ErrConflict,
		},
		{
			name: "destination unavailable",
			setup: func(t *testing.T, h transferHarness) admin.TransferInput {
				if err := h.db.Exec(`UPDATE fleet.fleets SET deleted_at = ? WHERE id = 'fleet-b'`,
					seedNow()).Error; err != nil {
					t.Fatalf("delete destination: %v", err)
				}
				return h.input(seededLabel)
			},
			wantErr: server.ErrConflict,
		},
		{
			name: "unknown destination",
			setup: func(_ *testing.T, h transferHarness) admin.TransferInput {
				in := h.input(seededLabel)
				in.DestFleetID = "no-such-fleet"
				return in
			},
			wantErr: server.ErrNotFound,
		},
		{
			name: "destination equals current fleet",
			setup: func(_ *testing.T, h transferHarness) admin.TransferInput {
				in := h.input(seededLabel)
				in.DestFleetID = "fleet-a"
				return in
			},
			wantErr: server.ErrValidation,
		},
		{
			name: "destination missing",
			setup: func(_ *testing.T, h transferHarness) admin.TransferInput {
				in := h.input(seededLabel)
				in.DestFleetID = ""
				return in
			},
			wantErr: server.ErrValidation,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newTransferHarness(t)
			in := tc.setup(t, h)

			_, err := h.proc.TransferVehicle(context.Background(), in)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			// Every 4xx MUST carry a detail: FR-XFER-UI-7 surfaces it verbatim.
			// Asserted as "errors.As succeeds AND the detail is non-empty", not
			// as the weaker "if it succeeds", which would pass vacuously for a
			// bare sentinel that has no Detail() at all.
			var detailed interface{ Detail() string }
			if !errors.As(err, &detailed) {
				t.Fatalf("err = %v carries no Detail(); FR-XFER-UI-7 surfaces it verbatim", err)
			}
			if detailed.Detail() == "" {
				t.Error("error carries an empty detail; FR-XFER-UI-7 surfaces it verbatim")
			}
			if len(h.media.calls) != 0 || len(h.notif.calls) != 0 {
				t.Error("a downstream was called for an ineligible transfer")
			}
			// Nothing may have moved: every branch above rejects BEFORE
			// ApplyTransfer, whose vehicles UPDATE has no eligibility guard.
			if got := scanOne[string](t, h.db, `SELECT fleet_id FROM fleet.vehicles WHERE id = ?`,
				h.src.VehicleID); got != "fleet-a" {
				t.Errorf("fleet_id = %q, want the untouched fleet-a", got)
			}
			if n := scanOne[int](t, h.db, `SELECT count(*) FROM fleet.admin_audit_events`); n != 0 {
				t.Errorf("audit rows = %d, want 0", n)
			}
		})
	}
}

// FR-XFER-MEDIA-5, with 503 rather than 502 (design D2): a media failure rolls
// the whole transfer back.
func TestTransferVehicle_mediaFailureRollsBackCompletely(t *testing.T) {
	h := newTransferHarness(t)
	// A category the transfer WOULD create in the destination, and a widget it
	// WOULD hard-delete, so the post-state assertions below are not vacuous.
	seedCustomCategory(t, h.db, h.src, "cat-winter", "Winter Tires", "maintenance")
	addWidget(t, h.db, "w-pinned", h.src.DashboardID, `{"vehicleId":"`+h.src.VehicleID+`"}`)
	h.media.err = errors.New("media-service is down")

	_, err := h.proc.TransferVehicle(context.Background(), h.input(seededLabel))
	if !errors.Is(err, server.ErrServiceUnavailable) {
		t.Fatalf("err = %v, want service unavailable", err)
	}
	if got := scanOne[string](t, h.db, `SELECT fleet_id FROM fleet.vehicles WHERE id = ?`,
		h.src.VehicleID); got != "fleet-a" {
		t.Errorf("fleet_id = %q, want the rolled-back fleet-a", got)
	}
	if n := scanOne[int](t, h.db, `SELECT count(*) FROM fleet.admin_audit_events`); n != 0 {
		t.Errorf("audit rows = %d, want 0", n)
	}
	if n := scanOne[int](t, h.db, `SELECT count(*) FROM fleet.activity_events WHERE type = ?`,
		admin.EventVehicleTransferredIn); n != 0 {
		t.Error("a transfer activity event survived the rollback")
	}
	// The hard-deleted widget and the created destination category must be back
	// too — a rollback that only restored fleet_id would still have destroyed
	// data.
	if n := scanOne[int](t, h.db,
		`SELECT count(*) FROM fleet.maintenance_categories WHERE fleet_id = 'fleet-b'`); n != 0 {
		t.Errorf("destination categories = %d, want 0 after the rollback", n)
	}
	if n := scanOne[int](t, h.db,
		`SELECT count(*) FROM fleet.dashboard_widgets WHERE id = 'w-pinned'`); n != 1 {
		t.Error("the hard-deleted widget did not come back with the rollback")
	}
	if len(h.notif.calls) != 0 {
		t.Error("notification-service was called after media-service failed")
	}
}

// If notification-service fails after media-service succeeded, the transaction
// rolls back AND the media move is reversed, so both sides end up as they were.
func TestTransferVehicle_notificationFailureCompensatesTheMediaMove(t *testing.T) {
	h := newTransferHarness(t)
	h.notif.err = errors.New("notification-service is down")

	_, err := h.proc.TransferVehicle(context.Background(), h.input(seededLabel))
	if !errors.Is(err, server.ErrServiceUnavailable) {
		t.Fatalf("err = %v, want service unavailable", err)
	}
	if got := scanOne[string](t, h.db, `SELECT fleet_id FROM fleet.vehicles WHERE id = ?`,
		h.src.VehicleID); got != "fleet-a" {
		t.Errorf("fleet_id = %q, want the rolled-back fleet-a", got)
	}
	if n := scanOne[int](t, h.db, `SELECT count(*) FROM fleet.admin_audit_events`); n != 0 {
		t.Errorf("audit rows = %d, want 0", n)
	}
	if len(h.media.calls) != 2 {
		t.Fatalf("media calls = %d, want 2 (the move and its reversal)", len(h.media.calls))
	}
	if h.media.dests[1] != "fleet-a" {
		t.Errorf("compensating call sent destination %q, want the SOURCE fleet-a", h.media.dests[1])
	}
	if len(h.media.calls[1]) == 0 || h.media.calls[1][0] != h.src.MediaID {
		t.Errorf("compensating call carried ids %v, want the vehicle's media ids", h.media.calls[1])
	}
}

// A compensating call that ALSO fails cannot be repaired in-process; it must not
// change the error the operator sees or panic the request.
func TestTransferVehicle_compensationFailureStillReturns503(t *testing.T) {
	h := newTransferHarness(t)
	// The media move succeeds, notification fails, and then the COMPENSATING
	// media call fails as well.
	h.notif.err = errors.New("notification-service is down")
	h.media.err = nil
	failing := &failAfterFirst{inner: h.media}
	h.proc = newProcessorWith(t, h.db, failing, h.notif)

	_, err := h.proc.TransferVehicle(context.Background(), h.input(seededLabel))
	if !errors.Is(err, server.ErrServiceUnavailable) {
		t.Fatalf("err = %v, want service unavailable", err)
	}
	if len(h.media.calls) != 2 {
		t.Errorf("media calls = %d, want 2 (the move and its failed reversal)", len(h.media.calls))
	}
	if got := scanOne[string](t, h.db, `SELECT fleet_id FROM fleet.vehicles WHERE id = ?`,
		h.src.VehicleID); got != "fleet-a" {
		t.Errorf("fleet_id = %q, want the rolled-back fleet-a", got)
	}
}

// failAfterFirst delegates to inner but fails every call after the first, which
// is exactly "the move worked, the reversal did not".
type failAfterFirst struct {
	inner *fakeReassigner
	n     int
}

func (f *failAfterFirst) Reassign(ctx context.Context, ids []string, dest string) (map[string]int, error) {
	f.n++
	got, err := f.inner.Reassign(ctx, ids, dest)
	if f.n > 1 {
		return nil, errors.New("media-service is down")
	}
	return got, err
}

func newProcessorWith(t *testing.T, db *gorm.DB, media admin.MediaReassigner, notif admin.NotificationReassigner) *admin.Processor {
	t.Helper()
	log := logrus.New()
	log.SetOutput(io.Discard)
	return admin.NewProcessor(log, admin.Deps{
		DB: db, Provider: admin.NewProvider(db), Administrator: admin.NewAdministrator(db),
		Auth: fakeAuth{ok: true}, MediaReassign: media, NotificationReassign: notif,
		Now: func() time.Time { return time.Date(2026, 8, 25, 18, 0, 0, 0, time.UTC) },
	}, admin.NewTargetResolver(db))
}

// A vehicle with no media must not send an empty media_ids list, which the
// downstream answers 422 to — a request that would read as a failed service.
func TestTransferVehicle_skipsMediaCallWhenThereIsNoMedia(t *testing.T) {
	h := newTransferHarness(t)
	if err := h.db.Exec(`DELETE FROM fleet.vehicle_media WHERE vehicle_id = ?`, h.src.VehicleID).
		Error; err != nil {
		t.Fatalf("clear vehicle media: %v", err)
	}
	if err := h.db.Exec(`DELETE FROM fleet.maintenance_record_documents`).Error; err != nil {
		t.Fatalf("clear documents: %v", err)
	}

	res, err := h.proc.TransferVehicle(context.Background(), h.input(seededLabel))
	if err != nil {
		t.Fatalf("transfer: %v", err)
	}
	if len(h.media.calls) != 0 {
		t.Errorf("media was called with an empty id set: %v", h.media.calls)
	}
	if res.AffectedCounts["media_objects"] != 0 {
		t.Errorf("media_objects = %d, want 0", res.AffectedCounts["media_objects"])
	}
}

// FR-XFER-CONF-2 and the preview-parity acceptance criterion.
func TestPreviewVehicleTransfer_returnsLabelCountsAndCategories(t *testing.T) {
	h := newTransferHarness(t)
	seedCustomCategory(t, h.db, h.src, "cat-winter", "Winter Tires", "maintenance")
	addWidget(t, h.db, "w-pinned", h.src.DashboardID, `{"vehicleId":"`+h.src.VehicleID+`"}`)

	pv, err := h.proc.PreviewVehicleTransfer(context.Background(), h.src.VehicleID, h.dst.FleetID)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if pv.VehicleLabel != seededLabel {
		t.Errorf("vehicle_label = %q, want %q", pv.VehicleLabel, seededLabel)
	}
	if pv.SourceFleetID != "fleet-a" || pv.SourceFleetName == "" {
		t.Errorf("source = %s/%q", pv.SourceFleetID, pv.SourceFleetName)
	}
	if pv.DestinationFleetID != "fleet-b" || pv.DestinationFleetName == "" {
		t.Errorf("destination = %s/%q", pv.DestinationFleetID, pv.DestinationFleetName)
	}
	if pv.Counts["widgets_removed"] != 1 {
		t.Errorf("widgets_removed = %d, want 1", pv.Counts["widgets_removed"])
	}
	if pv.Counts["media_objects"] != 1 {
		t.Errorf("media_objects = %d, want 1", pv.Counts["media_objects"])
	}
	if len(pv.CategoriesToCreate) != 1 || pv.CategoriesToCreate[0].Name != "Winter Tires" {
		t.Errorf("categories_to_create = %+v", pv.CategoriesToCreate)
	}
	if pv.Warnings == nil {
		t.Error("warnings must be an empty slice, not nil — it is serialised as [] not null")
	}

	// The preview writes nothing.
	if got := scanOne[string](t, h.db, `SELECT fleet_id FROM fleet.vehicles WHERE id = ?`,
		h.src.VehicleID); got != "fleet-a" {
		t.Error("the preview moved the vehicle")
	}
}

// The preview's counts must equal what the transfer then reports, for every key
// the preview produces.
func TestPreviewVehicleTransfer_countsMatchTheAppliedTransfer(t *testing.T) {
	h := newTransferHarness(t)
	seedCustomCategory(t, h.db, h.src, "cat-winter", "Winter Tires", "maintenance")
	addWidget(t, h.db, "w-pinned", h.src.DashboardID, `{"vehicleId":"`+h.src.VehicleID+`"}`)

	pv, err := h.proc.PreviewVehicleTransfer(context.Background(), h.src.VehicleID, h.dst.FleetID)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	res, err := h.proc.TransferVehicle(context.Background(), h.input(seededLabel))
	if err != nil {
		t.Fatalf("transfer: %v", err)
	}
	checked := 0
	for k, want := range pv.Counts {
		// media_objects is EXEMPT by design D7: the preview counts the media
		// REFERENCES this vehicle holds, while the applied transfer reports
		// media-service's own read-back. They agree whenever every reference
		// resolves — which the fake makes true here — but the design explicitly
		// permits them to diverge for a pre-existing dangling reference, so
		// asserting equality would pin a property the design disclaims.
		if k == "media_objects" {
			continue
		}
		if res.AffectedCounts[k] != want {
			t.Errorf("%s: preview %d, applied %d", k, want, res.AffectedCounts[k])
		}
		checked++
	}
	if checked < 8 {
		t.Fatalf("only %d preview counts were compared; the preview lost keys", checked)
	}
	// media_objects is asserted separately, against what the DOWNSTREAM said —
	// not against the preview.
	if res.AffectedCounts["media_objects"] != 1 {
		t.Errorf("media_objects = %d, want media-service's read-back of 1 (design D7)",
			res.AffectedCounts["media_objects"])
	}
}

// Without a destination the preview still answers, omitting the destination
// fields and categories_to_create, which cannot be computed without one.
func TestPreviewVehicleTransfer_withoutADestination(t *testing.T) {
	h := newTransferHarness(t)

	pv, err := h.proc.PreviewVehicleTransfer(context.Background(), h.src.VehicleID, "")
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if pv.DestinationFleetID != "" || pv.DestinationFleetName != "" {
		t.Errorf("destination = %s/%q, want empty", pv.DestinationFleetID, pv.DestinationFleetName)
	}
	if len(pv.CategoriesToCreate) != 0 {
		t.Errorf("categories_to_create = %+v, want empty without a destination", pv.CategoriesToCreate)
	}
	if pv.Counts["widgets_removed"] != 0 {
		t.Errorf("widgets_removed = %d, want 0", pv.Counts["widgets_removed"])
	}
}

func TestPreviewVehicleTransfer_unknownVehicleIsNotFound(t *testing.T) {
	h := newTransferHarness(t)
	_, err := h.proc.PreviewVehicleTransfer(context.Background(), "nope", h.dst.FleetID)
	if !errors.Is(err, server.ErrNotFound) {
		t.Fatalf("err = %v, want not found", err)
	}
	var detailed interface{ Detail() string }
	if !errors.As(err, &detailed) || detailed.Detail() == "" {
		t.Errorf("err = %v carries no detail; FR-XFER-UI-7 surfaces it verbatim", err)
	}
}

// A nickname wins over the year/make/model fallback (FR-XFER-CONF-2).
func TestPreviewVehicleTransfer_prefersTheNickname(t *testing.T) {
	h := newTransferHarness(t)
	if err := h.db.Exec(`UPDATE fleet.vehicles SET nickname = 'The Green Bean' WHERE id = ?`,
		h.src.VehicleID).Error; err != nil {
		t.Fatalf("set nickname: %v", err)
	}
	pv, err := h.proc.PreviewVehicleTransfer(context.Background(), h.src.VehicleID, h.dst.FleetID)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if pv.VehicleLabel != "The Green Bean" {
		t.Errorf("vehicle_label = %q, want The Green Bean", pv.VehicleLabel)
	}
}
