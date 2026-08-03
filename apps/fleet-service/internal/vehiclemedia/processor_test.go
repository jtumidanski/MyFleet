package vehiclemedia

import (
	"errors"
	"testing"

	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// fakeProvider satisfies Provider for unit tests.
type fakeProvider struct {
	rows []Model
}

func (f *fakeProvider) ListByVehicle(vehicleID string) ([]Model, error) { return f.rows, nil }
func (f *fakeProvider) GetByVehicleAndMedia(vehicleID, mediaID string) (Model, error) {
	for _, m := range f.rows {
		if m.VehicleID() == vehicleID && m.MediaID() == mediaID {
			return m, nil
		}
	}
	return Model{}, ErrNotFound
}

// fakeAdministrator satisfies Administrator for unit tests.
// SetPrimaryAtomic applies the mutations to an in-memory copy of rows so that
// end-state assertions remain meaningful without a real database.
type fakeAdministrator struct {
	// rows mirrors fakeProvider.rows and is updated atomically by SetPrimaryAtomic.
	rows []Model
	// lastVehicleID and lastMediaID capture the mirror-update arguments.
	lastVehicleID string
	lastTargetID  string
	lastMediaID   string
	lastClearIDs  []string

	// SoftDelete arguments, captured for assertion. deleteErr forces the
	// already-removed branch.
	deletedVehicleID string
	deletedMediaID   string
	deleteCalls      int
	deleteErr        error
}

func (f *fakeAdministrator) Insert(m Model) (Model, error) { return m, nil }

// SetPrimaryAtomic applies clear + set + mirror in memory, simulating the
// transactional behaviour of the real administrator.
func (f *fakeAdministrator) SetPrimaryAtomic(vehicleID, targetID, targetMediaID string, clearIDs []string) error {
	// Capture arguments for assertion.
	f.lastVehicleID = vehicleID
	f.lastTargetID = targetID
	f.lastMediaID = targetMediaID
	f.lastClearIDs = clearIDs

	// Apply mutations to in-memory rows.
	clearSet := make(map[string]struct{}, len(clearIDs))
	for _, id := range clearIDs {
		clearSet[id] = struct{}{}
	}
	for i, row := range f.rows {
		if _, ok := clearSet[row.ID()]; ok {
			f.rows[i] = row.WithIsPrimary(false)
		}
		if row.ID() == targetID {
			f.rows[i] = row.WithIsPrimary(true)
		}
	}
	return nil
}

func (f *fakeAdministrator) SoftDelete(vehicleID, mediaID string) error {
	f.deleteCalls++
	f.deletedVehicleID = vehicleID
	f.deletedMediaID = mediaID
	return f.deleteErr
}

// RemoveMedia is a delegation seam: the lookup, the delete and the successor
// decision all live in one transaction inside the administrator, so the only
// behaviour left to assert here is that the scope is forwarded intact and that
// the package-private sentinel is translated. Everything else is covered
// against a real database in administrator_test.go.
func TestRemoveMedia_forwardsTheVehicleScopedIdentifiers(t *testing.T) {
	fa := &fakeAdministrator{}
	proc := NewProcessor(logrus.New(), &fakeProvider{}, fa)

	if err := proc.RemoveMedia("v1", "m1"); err != nil {
		t.Fatalf("RemoveMedia failed: %v", err)
	}

	// Scoping by vehicle AND media is what stops a caller reaching another
	// vehicle's row; dropping either argument would be an IDOR.
	if fa.deletedVehicleID != "v1" || fa.deletedMediaID != "m1" {
		t.Errorf("SoftDelete(%q, %q), want (\"v1\", \"m1\")", fa.deletedVehicleID, fa.deletedMediaID)
	}
}

// The sentinel is package-private; leaking it would map to 500 instead of 404.
func TestRemoveMedia_translatesNotFound(t *testing.T) {
	fa := &fakeAdministrator{deleteErr: ErrNotFound}
	proc := NewProcessor(logrus.New(), &fakeProvider{}, fa)

	if err := proc.RemoveMedia("v1", "m1"); !errors.Is(err, server.ErrNotFound) {
		t.Fatalf("RemoveMedia error = %v, want server.ErrNotFound", err)
	}
}

func TestSetPrimary_unsetsPrevious(t *testing.T) {
	prev := NewBuilder().SetVehicleID("v1").SetMediaID("m1").SetIsPrimary(true).SetSortOrder(0).Build()
	next := NewBuilder().SetVehicleID("v1").SetMediaID("m2").SetIsPrimary(false).SetSortOrder(1).Build()

	rows := []Model{prev, next}
	fp := &fakeProvider{rows: rows}
	fa := &fakeAdministrator{rows: rows}
	proc := NewProcessor(logrus.New(), fp, fa)

	if err := proc.SetPrimary("v1", "m2"); err != nil {
		t.Fatalf("SetPrimary failed: %v", err)
	}

	// Verify the transactional call received the correct arguments.
	if fa.lastTargetID != next.ID() {
		t.Errorf("SetPrimaryAtomic target ID = %q, want %q", fa.lastTargetID, next.ID())
	}
	if fa.lastMediaID != "m2" {
		t.Errorf("SetPrimaryAtomic targetMediaID = %q, want %q", fa.lastMediaID, "m2")
	}
	if len(fa.lastClearIDs) != 1 || fa.lastClearIDs[0] != prev.ID() {
		t.Errorf("SetPrimaryAtomic clearIDs = %v, want [%s]", fa.lastClearIDs, prev.ID())
	}

	// Verify the in-memory end state: exactly one row is primary.
	primaryCount := 0
	for _, row := range fa.rows {
		if row.IsPrimary() {
			primaryCount++
			if row.ID() != next.ID() {
				t.Errorf("unexpected primary row: %s (expected %s)", row.ID(), next.ID())
			}
		}
	}
	if primaryCount != 1 {
		t.Errorf("expected exactly 1 primary row after SetPrimary, got %d (rows: %+v)", primaryCount, fa.rows)
	}
}
