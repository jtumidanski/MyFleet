package mileage

import (
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

type fakeVehicleMileage struct{ set int }

func (f *fakeVehicleMileage) UpdateCurrentMileage(vehicleID string, m int) error { f.set = m; return nil }

func TestAppend_updatesCurrentMileageWhenHigher(t *testing.T) {
	vm := &fakeVehicleMileage{set: 1000}
	p := NewProcessor(logrus.New(), vm)
	rec := NewBuilder().SetVehicleID("v1").SetMileage(1500).SetRecordedAt(time.Now()).SetSource("manual").Build()
	flagged, err := p.OnAppend(rec, 1000)
	if err != nil || flagged {
		t.Fatalf("higher mileage should not be flagged; flagged=%v err=%v", flagged, err)
	}
	if vm.set != 1500 {
		t.Fatalf("current_mileage should advance to 1500, got %d", vm.set)
	}
}

func TestAppend_flagsBelowLatestButKeeps(t *testing.T) {
	vm := &fakeVehicleMileage{set: 2000}
	p := NewProcessor(logrus.New(), vm)
	rec := NewBuilder().SetVehicleID("v1").SetMileage(1500).SetRecordedAt(time.Now()).SetSource("manual").Build()
	flagged, err := p.OnAppend(rec, 2000)
	if err != nil {
		t.Fatal(err)
	}
	if !flagged {
		t.Fatal("below-latest entry should be flagged")
	}
	if vm.set != 2000 {
		t.Fatalf("current_mileage must not regress, stayed at latest; got %d", vm.set)
	}
}
