package maintenanceschedule

import (
	"time"

	"gorm.io/gorm"
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
			"recurrence_type":  e.RecurrenceType,
			"interval_months":  e.IntervalMonths,
			"interval_miles":   e.IntervalMiles,
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
	if err := tx.First(&e, "id = ?", id).Error; err != nil {
		return err
	}
	m := Make(e).WithLastCompleted(date, miles)
	// Recompute next-due from the new completion point (design §10.1).
	nd, nm := NextDue(m.AsSchedule())
	m = m.WithNextDue(nd, nm)
	// Re-derive status/severity from the new schedule at completion mileage/now.
	state := DueState(m.AsSchedule(), time.Now().UTC(), miles, DefaultThresholds)
	m = m.WithStatus(state, Severity(state))

	updates := map[string]any{
		"last_completed_date":    date,
		"last_completed_mileage": miles,
		"next_due_mileage":       m.NextDueMileage(),
		"status":                 m.Status(),
		"severity":               m.Severity(),
	}
	if !m.NextDueDate().IsZero() {
		updates["next_due_date"] = m.NextDueDate()
	} else {
		updates["next_due_date"] = nil
	}
	return tx.Model(&Entity{}).Where("id = ?", id).Updates(updates).Error
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
	if err := a.db.First(&e, "id = ?", id).Error; err != nil {
		return Model{}, err
	}
	return Make(e), nil
}
