package maintenanceschedule

import (
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/maintenancerecord"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/mileage"
)

// RecordInserter inserts a maintenance record inside a transaction. Satisfied
// by *maintenancerecord.dbAdministrator via its InsertTx method.
type RecordInserter interface {
	InsertTx(tx *gorm.DB, m maintenancerecord.Model) (maintenancerecord.Model, error)
}

// ScheduleAdvancer advances a schedule inside a transaction. Satisfied by the
// schedule Administrator via its AdvanceTx method.
type ScheduleAdvancer interface {
	AdvanceTx(tx *gorm.DB, id string, date time.Time, miles int) error
}

// MileageAppender appends a mileage ledger row inside a transaction, advancing
// current_mileage per the ledger rule. Satisfied by the mileage Administrator.
type MileageAppender interface {
	AppendTx(tx *gorm.DB, m mileage.Model) (mileage.Model, error)
}

// ActivityRecorder appends an activity event on the supplied tx (design §8.2).
// Injected to avoid an import cycle. Satisfied by activity.Record.
type ActivityRecorder func(tx *gorm.DB, actorUserID, eventType, fleetID string, vehicleID *string, payload map[string]any) error

// CompletedEmitter enqueues a maintenance.completed event in the outbox on the
// supplied tx (design A8). Injected to avoid an import cycle.
type CompletedEmitter func(tx *gorm.DB, fleetID, actorID, traceID, scheduleID, vehicleID, recordID, categoryID string) error

// CompletionDeps holds the dependencies the concrete completion needs to run
// the cross-domain write set in one transaction (design §10.3).
type CompletionDeps struct {
	DB       *gorm.DB
	Records  RecordInserter
	Schedule ScheduleAdvancer
	Mileage  MileageAppender
	record   ActivityRecorder
	emit     CompletedEmitter
}

// NewCompletionDeps wires the concrete completion dependencies.
func NewCompletionDeps(db *gorm.DB, records RecordInserter, schedule ScheduleAdvancer, ledger MileageAppender) CompletionDeps {
	return CompletionDeps{DB: db, Records: records, Schedule: schedule, Mileage: ledger}
}

// WithActivityRecorder injects the activity recorder run after completion.
func (d CompletionDeps) WithActivityRecorder(rec ActivityRecorder) CompletionDeps {
	d.record = rec
	return d
}

// WithEmitter injects the maintenance.completed outbox emitter (A8).
func (d CompletionDeps) WithEmitter(emit CompletedEmitter) CompletionDeps {
	d.emit = emit
	return d
}

// CompleteInTransaction runs the full completion flow (record insert + mileage
// insert + current_mileage mirror + schedule advance/recompute) inside ONE
// db.Transaction. The orchestration order lives in CompletionProcessor.Complete;
// this binds a tx-scoped Completion to that processor (design §10.3).
func (d CompletionDeps) CompleteInTransaction(log logrus.FieldLogger, in CompletionInput) (CompletionOutput, error) {
	var out CompletionOutput
	err := d.DB.Transaction(func(tx *gorm.DB) error {
		c := &dbCompletion{tx: tx, records: d.Records, schedule: d.Schedule, ledger: d.Mileage}
		proc := NewCompletionProcessor(log, c)
		var perr error
		out, perr = proc.Complete(in)
		if perr != nil {
			return perr
		}
		// Append a maintenance.completed activity event in the SAME tx (§8.2).
		if d.record != nil {
			vid := in.VehicleID
			if err := d.record(tx, in.ActorUserID, "maintenance.completed", in.FleetID, &vid, map[string]any{
				"schedule_id":           in.ScheduleID,
				"vehicle_id":            in.VehicleID,
				"maintenance_record_id": out.MaintenanceRecordID,
				"category_id":           in.CategoryID,
			}); err != nil {
				return err
			}
		}
		// Enqueue the maintenance.completed outbox event in the SAME tx (A8).
		if d.emit != nil {
			if err := d.emit(tx, in.FleetID, in.ActorUserID, in.TraceID, in.ScheduleID, in.VehicleID, out.MaintenanceRecordID, in.CategoryID); err != nil {
				return err
			}
		}
		return nil
	})
	return out, err
}

// dbCompletion is the transaction-bound Completion. All three steps execute
// against the single tx supplied by CompleteInTransaction.
type dbCompletion struct {
	tx       *gorm.DB
	records  RecordInserter
	schedule ScheduleAdvancer
	ledger   MileageAppender
}

// CreateRecord inserts a pre-filled maintenance record on the bound tx.
func (c *dbCompletion) CreateRecord(vehicleID, categoryID string, date time.Time, miles int) (string, error) {
	m, err := maintenancerecord.NewBuilder().
		SetVehicleID(vehicleID).
		SetCategoryID(categoryID).
		SetPerformedAt(date).
		SetMileage(miles).
		Build()
	if err != nil {
		return "", err
	}
	created, err := c.records.InsertTx(c.tx, m)
	if err != nil {
		return "", err
	}
	return created.ID(), nil
}

// AppendMileage appends a mileage ledger row (source=maintenance, ref=record id)
// on the bound tx, recorded at the completion date and attributed to the acting
// user. The ledger's AppendTx advances vehicles.current_mileage only when the
// reading is >= the current value (never regresses; design §10.4).
func (c *dbCompletion) AppendMileage(vehicleID string, miles int, recordedAt time.Time, src, ref, actorUserID string) error {
	rec := mileage.NewBuilder().
		SetVehicleID(vehicleID).
		SetMileage(miles).
		SetRecordedAt(recordedAt).
		SetSource(src).
		SetSourceRefID(ref).
		SetCreatedByUserID(actorUserID).
		Build()
	_, err := c.ledger.AppendTx(c.tx, rec)
	return err
}

// AdvanceSchedule moves the schedule's completion point and recomputes next-due
// on the bound tx.
func (c *dbCompletion) AdvanceSchedule(scheduleID string, date time.Time, miles int) error {
	return c.schedule.AdvanceTx(c.tx, scheduleID, date, miles)
}
