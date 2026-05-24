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

// capturedUpdate records calls made to the fakeAdministrator.
type capturedUpdate struct {
	id        string
	isPrimary bool
}

// fakeAdministrator satisfies Administrator for unit tests.
type fakeAdministrator struct {
	updates []capturedUpdate
}

func (f *fakeAdministrator) Insert(m Model) (Model, error) { return m, nil }
func (f *fakeAdministrator) SetIsPrimary(id string, isPrimary bool) error {
	f.updates = append(f.updates, capturedUpdate{id: id, isPrimary: isPrimary})
	return nil
}
func (f *fakeAdministrator) UpdateVehiclePrimaryImage(vehicleID, mediaID string) error {
	return nil
}

func TestSetPrimary_unsetsPrevious(t *testing.T) {
	prev := NewBuilder().SetVehicleID("v1").SetMediaID("m1").SetIsPrimary(true).SetSortOrder(0).Build()
	next := NewBuilder().SetVehicleID("v1").SetMediaID("m2").SetIsPrimary(false).SetSortOrder(1).Build()

	fp := &fakeProvider{rows: []Model{prev, next}}
	fa := &fakeAdministrator{}
	proc := NewProcessor(logrus.New(), fp, fa)

	if err := proc.SetPrimary("v1", "m2"); err != nil {
		t.Fatalf("SetPrimary failed: %v", err)
	}

	// Expect: prev → false, next → true
	wantFalse := 0
	wantTrue := 0
	for _, u := range fa.updates {
		if u.id == prev.ID() && !u.isPrimary {
			wantFalse++
		}
		if u.id == next.ID() && u.isPrimary {
			wantTrue++
		}
	}
	if wantFalse != 1 {
		t.Errorf("previous media row should be set to is_primary=false, updates: %+v", fa.updates)
	}
	if wantTrue != 1 {
		t.Errorf("new media row should be set to is_primary=true, updates: %+v", fa.updates)
	}
}
