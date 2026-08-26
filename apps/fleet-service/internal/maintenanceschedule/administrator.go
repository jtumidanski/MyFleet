package maintenanceschedule

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// Administrator is the write interface for maintenance schedule data access.
// All mutations (insert, update, advance, recompute, delete) go here.
type Administrator interface {
	Insert(Model) (Model, error)
	Update(Model) (Model, error)
	Delete(id string) error
	// Advance moves the schedule's completion point to (date, miles) and
	// recomputes next_due_* (via NextDue) + status/severity (via DueState).
	Advance(id string, date time.Time, miles int) error
	// AdvanceTx is Advance using the supplied transaction handle, so the
	// completion flow (design §10.3) can wrap it in one db.Transaction.
	AdvanceTx(tx *gorm.DB, id string, date time.Time, miles int) error
	// Recompute re-derives next_due_*, status, and severity for an existing
	// schedule given the vehicle's current mileage and "now" (FR-MAINT-6).
	Recompute(id string, currentMileage int, now time.Time) error
	// RecomputeTx is Recompute on the supplied transaction handle, so the
	// recompute job can append activity + enqueue an outbox event atomically with
	// the status update on an overdue transition (design A8).
	RecomputeTx(tx *gorm.DB, id string, currentMileage int, now time.Time) error
}

type dbAdministrator struct{ db *gorm.DB }

// NewAdministrator returns an Administrator backed by the given database.
func NewAdministrator(db *gorm.DB) Administrator { return &dbAdministrator{db: db} }

func (a *dbAdministrator) Insert(m Model) (Model, error) {
	// Entity.Active carries a `default:true` GORM tag, so GORM substitutes that
	// default for any explicit Active=false at Create time: Insert can never
	// persist an inactive row. Deactivation always goes through AdvanceTx's
	// column-map UPDATE instead, which the default tag has no effect on.
	e := m.ToEntity()
	if err := a.db.Create(&e).Error; err != nil {
		return Model{}, err
	}
	return Make(e), nil
}

func (a *dbAdministrator) Update(m Model) (Model, error) {
	e := m.ToEntity()
	if err := a.db.Model(&Entity{}).Where("id = ?", e.ID).
		Updates(map[string]any{
			"recurrence_type": e.RecurrenceType,
			"interval_months": e.IntervalMonths,
			"interval_miles":  e.IntervalMiles,
			"one_time":        e.OneTime,
			// e.DueDate is a *time.Time that ToEntity leaves nil for a zero
			// time, so a cleared anchor writes SQL NULL rather than year 1.
			"due_date":         e.DueDate,
			"due_mileage":      e.DueMileage,
			"active":           e.Active,
			"next_due_date":    e.NextDueDate,
			"next_due_mileage": e.NextDueMileage,
			"status":           e.Status,
			"severity":         e.Severity,
		}).Error; err != nil {
		return Model{}, err
	}
	return a.get(e.ID)
}

func (a *dbAdministrator) Delete(id string) error {
	res := a.db.Where("id = ?", id).Delete(&Entity{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (a *dbAdministrator) Advance(id string, date time.Time, miles int) error {
	return a.AdvanceTx(a.db, id, date, miles)
}

func (a *dbAdministrator) AdvanceTx(tx *gorm.DB, id string, date time.Time, miles int) error {
	var e Entity
	// FR-COMPLETE-4. Two concurrent completions of the same one-time schedule
	// must not both succeed, and a bare SELECT-then-check-then-UPDATE does not
	// prevent that: under READ COMMITTED both readers see active=true and both
	// pass the guard. Mutual exclusion comes from two things, not from the
	// in-Go check:
	//
	//  1. this SELECT takes a row lock (FOR UPDATE), so the second transaction
	//     blocks here until the first commits and then re-reads active=false;
	//  2. the closing UPDATE re-asserts `active` in its WHERE, so even without
	//     the lock the loser matches zero rows and fails.
	//
	// The in-Go check below is only a fast path that avoids doing the recurrence
	// math for a row already known to be inactive.
	//
	// The lock clause is emitted only on Postgres: SQLite (the package's test
	// harness) has no FOR UPDATE and would reject the statement outright. It
	// serialises writers by database-level locking instead, and guarantee (2)
	// holds on every dialect.
	sel := tx
	if d := tx.Dialector; d != nil && d.Name() == "postgres" {
		sel = tx.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := sel.First(&e, "id = ?", id).Error; err != nil {
		return err
	}
	if !e.Active {
		return server.ErrValidation
	}

	// Clearing the anchor happens for BOTH kinds (FR-ANCHOR-3, FR-COMPLETE-2).
	// For a recurring schedule it hands next-due permanently to interval
	// arithmetic from the new completion point; for a one-time schedule the row
	// is deactivated in the same update, so the cleared anchor is never read
	// again. Clearing before NextDue is what stops a stale anchor from
	// outranking the completion point.
	m := Make(e).WithLastCompleted(date, miles).WithDuePoint(time.Time{}, 0)
	nd, nm := NextDue(m.AsSchedule())
	m = m.WithNextDue(nd, nm)
	state := DueState(m.AsSchedule(), time.Now().UTC(), miles, DefaultThresholds)
	if m.OneTime() {
		// A completed one-time schedule has no next due point at all, and the
		// generic DueState reads a zero due-date as overdue. The row is being
		// deactivated in this same update, but the stored status is what the
		// vehicle's schedule list renders for an inactive row.
		state = "ok"
	}
	m = m.WithStatus(state, Severity(state))

	updates := map[string]any{
		"last_completed_date":    date,
		"last_completed_mileage": miles,
		"due_date":               nil,
		"due_mileage":            0,
		"next_due_mileage":       m.NextDueMileage(),
		"status":                 m.Status(),
		"severity":               m.Severity(),
	}
	if !m.NextDueDate().IsZero() {
		updates["next_due_date"] = m.NextDueDate()
	} else {
		updates["next_due_date"] = nil
	}
	if m.OneTime() {
		updates["active"] = false
	}
	return applyAdvance(tx, id, updates)
}

// applyAdvance writes a completion's column map, re-asserting `active` so a row
// deactivated since AdvanceTx loaded it matches nothing. Zero rows affected is
// the same validation failure the in-Go guard reports, which rolls the whole
// completion transaction back.
//
// It is a separate function so the SELECT→UPDATE window can be exercised
// directly against an already-inactive row.
func applyAdvance(tx *gorm.DB, id string, updates map[string]any) error {
	res := tx.Model(&Entity{}).Where("id = ? AND active = ?", id, true).Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return server.ErrValidation
	}
	return nil
}

func (a *dbAdministrator) Recompute(id string, currentMileage int, now time.Time) error {
	return a.RecomputeTx(a.db, id, currentMileage, now)
}

func (a *dbAdministrator) RecomputeTx(tx *gorm.DB, id string, currentMileage int, now time.Time) error {
	var e Entity
	if err := tx.First(&e, "id = ?", id).Error; err != nil {
		return err
	}
	m := Make(e)
	nd, nm := NextDue(m.AsSchedule())
	state := DueState(m.AsSchedule(), now, currentMileage, DefaultThresholds)

	updates := map[string]any{
		"next_due_mileage": nm,
		"status":           state,
		"severity":         Severity(state),
	}
	if !nd.IsZero() {
		updates["next_due_date"] = nd
	} else {
		updates["next_due_date"] = nil
	}
	return tx.Model(&Entity{}).Where("id = ?", id).Updates(updates).Error
}

func (a *dbAdministrator) get(id string) (Model, error) {
	var e Entity
	if err := a.db.First(&e, "id = ? AND deleted_at IS NULL", id).Error; err != nil {
		return Model{}, err
	}
	return Make(e), nil
}
