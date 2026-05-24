package fuel

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// Administrator is the write interface for fuel log data access.
// All writes and cross-domain transactions go here.
type Administrator interface {
	// Insert persists a new fuel log and returns the created model.
	Insert(m Model) (Model, error)
	// Update applies changes to an existing fuel log.
	Update(m Model) (Model, error)
	// SoftDelete stamps deleted_at on a fuel log.
	SoftDelete(id string) error
}

type dbAdministrator struct{ db *gorm.DB }

// NewAdministrator returns an Administrator backed by the given database.
func NewAdministrator(db *gorm.DB) Administrator { return &dbAdministrator{db: db} }

func (a *dbAdministrator) Insert(m Model) (Model, error) {
	e := m.ToEntity()
	if err := a.db.Create(&e).Error; err != nil {
		return Model{}, err
	}
	return Make(e), nil
}

func (a *dbAdministrator) Update(m Model) (Model, error) {
	e := m.ToEntity()
	if err := a.db.Model(&Entity{}).Where("id = ? AND deleted_at IS NULL", e.ID).
		Updates(map[string]any{
			"date":             e.Date,
			"mileage":          e.Mileage,
			"gallons":          e.Gallons,
			"total_cost":       e.TotalCost,
			"price_per_gallon": e.PricePerGallon,
		}).Error; err != nil {
		return Model{}, err
	}
	return m, nil
}

// SoftDelete stamps deleted_at on the fuel log.
func (a *dbAdministrator) SoftDelete(id string) error {
	res := a.db.Model(&Entity{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Update("deleted_at", time.Now().UTC())
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return server.ErrNotFound
	}
	return nil
}

// ActivityRecorder appends an activity event on the supplied tx (design §8.2).
// Injected to keep the fuel package decoupled. Satisfied by activity.Record.
type ActivityRecorder func(tx *gorm.DB, actorUserID, eventType, fleetID string, vehicleID *string, payload map[string]any) error

// FuelLoggedEmitter enqueues a fuel.logged event in the outbox on the supplied
// tx (design A8). Injected to avoid coupling. Satisfied by events.EmitFuelLogged.
type FuelLoggedEmitter func(tx *gorm.DB, fleetID, actorID, traceID, fuelLogID, vehicleID string, mileage int, totalCost float64) error

// LoggingDeps holds the cross-domain dependencies for the fuel→mileage
// orchestration (design §8.2, §10.5). All writes execute inside one transaction.
type LoggingDeps struct {
	DB     *gorm.DB
	record ActivityRecorder
	emit   FuelLoggedEmitter
}

// NewLoggingDeps wires the concrete logging dependencies.
func NewLoggingDeps(db *gorm.DB) LoggingDeps {
	return LoggingDeps{DB: db}
}

// WithActivityRecorder injects the activity recorder run on each fuel log.
func (d LoggingDeps) WithActivityRecorder(rec ActivityRecorder) LoggingDeps {
	d.record = rec
	return d
}

// WithEmitter injects the fuel.logged outbox emitter (A8).
func (d LoggingDeps) WithEmitter(emit FuelLoggedEmitter) LoggingDeps {
	d.emit = emit
	return d
}

// LogInTransaction runs the full fuel-log flow in ONE db.Transaction:
//  1. Insert the fuel log row.
//  2. Insert a mileage record (source=fuel, ref=fuel log id).
//  3. Advance vehicles.current_mileage via the OnAppend rule (never regress).
//  4. Enqueue a fuel.logged event in the transactional outbox.
//
// Mirror of maintenanceschedule.CompleteInTransaction (design §10.3).
func (d LoggingDeps) LogInTransaction(log logrus.FieldLogger, in LogInput) (Model, error) {
	var created Model
	err := d.DB.Transaction(func(tx *gorm.DB) error {
		// Step 1: insert fuel log row.
		e := in.FuelLog.ToEntity()
		if err := tx.Create(&e).Error; err != nil {
			return err
		}
		created = Make(e)

		// Step 2: insert mileage record (source=fuel, ref=fuel log id).
		mileageID := uuid.NewString()
		if err := tx.Exec(
			`INSERT INTO fleet.mileage_records
				(id, vehicle_id, mileage, recorded_at, source, source_ref_id, created_by_user_id, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			mileageID,
			created.VehicleID(),
			created.Mileage(),
			created.Date(),
			"fuel",
			created.ID(),
			created.CreatedByUserID(),
			time.Now().UTC(),
		).Error; err != nil {
			return err
		}

		// Step 3: advance current_mileage — never regress (OnAppend rule, design §10.4).
		if err := tx.Table("fleet.vehicles").
			Where("id = ? AND deleted_at IS NULL AND current_mileage <= ?", created.VehicleID(), created.Mileage()).
			Update("current_mileage", created.Mileage()).Error; err != nil {
			return err
		}

		// Step 3b: append a fuel.logged activity event in the SAME tx (§8.2).
		if d.record != nil {
			vid := created.VehicleID()
			if err := d.record(tx, created.CreatedByUserID(), "fuel.logged", in.FleetID, &vid, map[string]any{
				"fuel_log_id": created.ID(),
				"vehicle_id":  created.VehicleID(),
				"mileage":     created.Mileage(),
				"total_cost":  created.TotalCost(),
			}); err != nil {
				return err
			}
		}

		// Step 4: enqueue fuel.logged event in the transactional outbox (A8).
		// FATAL: an enqueue failure rolls back the whole transaction so the
		// domain write and the outbox row stay atomic (no silent event loss).
		if d.emit != nil {
			if err := d.emit(tx, in.FleetID, created.CreatedByUserID(), in.TraceID, created.ID(), created.VehicleID(), created.Mileage(), created.TotalCost()); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Model{}, server.ErrNotFound
		}
		return Model{}, err
	}
	return created, nil
}

// LogInput carries the pre-validated inputs to LogInTransaction.
type LogInput struct {
	FuelLog Model
	FleetID string // needed for the outbox event envelope
	TraceID string // correlation id for the emitted event
}
