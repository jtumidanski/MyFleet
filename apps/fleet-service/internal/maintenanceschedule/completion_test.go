package maintenanceschedule

import (
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

type captureCompletion struct {
	recordCreated   bool
	mileageAppended bool
	lastDate        time.Time
	lastMiles       int
}

func (c *captureCompletion) CreateRecord(vehicleID, categoryID string, date time.Time, miles int) (string, error) {
	c.recordCreated = true
	return "rec-1", nil
}
func (c *captureCompletion) AppendMileage(vehicleID string, miles int, src, ref string) error {
	c.mileageAppended = true
	return nil
}
func (c *captureCompletion) AdvanceSchedule(scheduleID string, date time.Time, miles int) error {
	c.lastDate, c.lastMiles = date, miles
	return nil
}

func TestComplete_orchestratesRecordMileageAndAdvance(t *testing.T) {
	cap := &captureCompletion{}
	p := NewCompletionProcessor(logrus.New(), cap)
	at := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	out, err := p.Complete(CompletionInput{
		ScheduleID: "s1", VehicleID: "v1", CategoryID: "c1", Date: at, LatestMileage: 42000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !cap.recordCreated || !cap.mileageAppended {
		t.Fatal("completion must create a record and append mileage")
	}
	if !cap.lastDate.Equal(at) || cap.lastMiles != 42000 {
		t.Fatalf("schedule must advance to completion point; got %v/%d", cap.lastDate, cap.lastMiles)
	}
	if out.MaintenanceRecordID != "rec-1" {
		t.Fatalf("want created record id, got %q", out.MaintenanceRecordID)
	}
}
