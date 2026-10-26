package mileage

import (
	"io"
	"testing"

	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

func newTestProcessor(t *testing.T) (*Processor, func() int) {
	t.Helper()
	db := newTestDB(t)
	seedVehicle(t, db, "v1", 2000)
	log := logrus.New()
	log.SetOutput(io.Discard)
	return NewProcessor(log, db, NewProvider(db), NewAdministrator(db)), func() int { return currentMileage(t, db, "v1") }
}

func TestProcessor_Append_advancesOdometer(t *testing.T) {
	pr, odo := newTestProcessor(t)
	got, err := pr.Append(reading("v1", 2500, SourceManual, ""))
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if got.Flagged() {
		t.Error("expected not flagged")
	}
	if odo() != 2500 {
		t.Errorf("odometer = %d, want 2500", odo())
	}
}

func TestProcessor_Append_belowOdometerIsKeptAndFlagged(t *testing.T) {
	pr, odo := newTestProcessor(t)
	got, err := pr.Append(reading("v1", 1500, SourceManual, ""))
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if !got.Flagged() {
		t.Error("expected flagged")
	}
	if odo() != 2000 {
		t.Errorf("odometer = %d, want 2000", odo())
	}
	ms, total, err := pr.ListByVehicle("v1", ListFilter{}, server.Page{Number: 1, Size: 10})
	if err != nil || total != 1 || len(ms) != 1 {
		t.Fatalf("list = %d rows total %d err %v, want 1", len(ms), total, err)
	}
}
