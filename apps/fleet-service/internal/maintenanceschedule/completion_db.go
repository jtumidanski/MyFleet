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

// CompletionDeps holds the dependencies the concrete completion needs to run
// the cross-domain write set in one transaction (design §10.3).
type CompletionDeps struct {
	DB       *gorm.DB
	Records  RecordInserter
	Schedule ScheduleAdvancer
}

// NewCompletionDeps wires the concrete completion dependencies.
func NewCompletionDeps(db *gorm.DB, records RecordInserter, schedule ScheduleAdvancer) CompletionDeps {
	return CompletionDeps{DB: db, Records: records, Schedule: schedule}
}

// CompleteInTransaction runs the full completion flow (record insert + mileage
// insert + current_mileage mirror + schedule advance/recompute) inside ONE
// db.Transaction. The orchestration order lives in CompletionProcessor.Complete;
// this binds a tx-scoped Completion to that processor (design §10.3).
func (d CompletionDeps) CompleteInTransaction(log logrus.FieldLogger, in CompletionInput) (CompletionOutput, error) {
	var out CompletionOutput
	err := d.DB.Transaction(func(tx *gorm.DB) error {
		c := &dbCompletion{tx: tx, records: d.Records, schedule: d.Schedule}
		proc := NewCompletionProcessor(log, c)
		var perr error
		out, perr = proc.Complete(in)
		return perr
	})
	return out, err
}

// dbCompletion is the transaction-bound Completion. All three steps execute
// against the single tx supplied by CompleteInTransaction.
type dbCompletion struct {
	tx       *gorm.DB
	records  RecordInserter
	schedule ScheduleAdvancer
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

// AppendMileage inserts a mileage record (source=maintenance, ref=record id) on
// the bound tx and mirrors current_mileage per the mileage OnAppend rule: only
// advance vehicles.current_mileage when the new reading is >= the current value
// (do not regress; lower readings are kept in history but flagged — design §10.4).
func (c *dbCompletion) AppendMileage(vehicleID string, miles int, src, ref string) error {
	rec := mileage.NewBuilder().
		SetVehicleID(vehicleID).
		SetMileage(miles).
		SetRecordedAt(time.Now().UTC()).
		SetSource(src).
		SetSourceRefID(ref).
		Build()
	e := rec.ToEntity()
	if err := c.tx.Create(&e).Error; err != nil {
		return err
	}
	// Mirror current_mileage on the vehicle, but never regress it (OnAppend rule).
	return c.tx.Table("fleet.vehicles").
		Where("id = ? AND deleted_at IS NULL AND current_mileage <= ?", vehicleID, miles).
		Update("current_mileage", miles).Error
}

// AdvanceSchedule moves the schedule's completion point and recomputes next-due
// on the bound tx.
func (c *dbCompletion) AdvanceSchedule(scheduleID string, date time.Time, miles int) error {
	return c.schedule.AdvanceTx(c.tx, scheduleID, date, miles)
}
