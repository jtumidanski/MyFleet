package maintenanceschedule

import (
	"time"

	"github.com/sirupsen/logrus"
)

// CompletionInput is the request to complete a maintenance schedule (design §10.3).
type CompletionInput struct {
	ScheduleID    string
	VehicleID     string
	CategoryID    string
	Date          time.Time
	LatestMileage int
}

// CompletionOutput is the result of a completion: the created record's ID.
type CompletionOutput struct {
	MaintenanceRecordID string
}

// Completion is the set of side-effecting steps the completion flow orchestrates.
// It is an interface so the orchestration in CompletionProcessor.Complete is
// unit-testable with a fake (plan Task 9.3). The concrete implementation
// (dbCompletion) runs all three steps inside ONE db.Transaction.
type Completion interface {
	// CreateRecord inserts a pre-filled maintenance record and returns its ID.
	CreateRecord(vehicleID, categoryID string, date time.Time, miles int) (string, error)
	// AppendMileage inserts a mileage record (source=maintenance, ref=record id)
	// and mirrors current_mileage per the mileage OnAppend rule.
	AppendMileage(vehicleID string, miles int, src, ref string) error
	// AdvanceSchedule moves the schedule's completion point and recomputes next-due.
	AdvanceSchedule(scheduleID string, date time.Time, miles int) error
}

// CompletionProcessor orchestrates the completion flow (design §10.3). The
// orchestration is pure sequencing over a Completion; the DB transaction is the
// concern of the concrete Completion.
type CompletionProcessor struct {
	log logrus.FieldLogger
	c   Completion
}

// NewCompletionProcessor constructs a CompletionProcessor over a Completion.
func NewCompletionProcessor(log logrus.FieldLogger, c Completion) *CompletionProcessor {
	return &CompletionProcessor{log: log, c: c}
}

// Complete runs the completion flow IN ORDER (design §10.3):
//  1. create a pre-filled maintenance record
//  2. append a mileage record (source=maintenance, ref=the record id)
//  3. advance the schedule to the completion point (next-due recompute happens
//     in the administrator on advance, via NextDue/DueState)
//
// Event emission + activity append are stubbed for now (Phase 11).
func (pr *CompletionProcessor) Complete(in CompletionInput) (CompletionOutput, error) {
	recordID, err := pr.c.CreateRecord(in.VehicleID, in.CategoryID, in.Date, in.LatestMileage)
	if err != nil {
		return CompletionOutput{}, err
	}
	if err := pr.c.AppendMileage(in.VehicleID, in.LatestMileage, "maintenance", recordID); err != nil {
		return CompletionOutput{}, err
	}
	if err := pr.c.AdvanceSchedule(in.ScheduleID, in.Date, in.LatestMileage); err != nil {
		return CompletionOutput{}, err
	}
	return CompletionOutput{MaintenanceRecordID: recordID}, nil
}
