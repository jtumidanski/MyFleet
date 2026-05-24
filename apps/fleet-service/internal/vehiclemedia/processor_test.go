package vehiclemedia

import (
	"testing"

	"github.com/sirupsen/logrus"
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
