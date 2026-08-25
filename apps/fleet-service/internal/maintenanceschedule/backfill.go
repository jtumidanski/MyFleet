package maintenanceschedule

import (
	"time"

	"gorm.io/gorm"
)

// backfillRow is one candidate schedule joined to its vehicle's odometer.
type backfillRow struct {
	ID             string
	RecurrenceType string
	IntervalMonths int
	IntervalMiles  int
	CreatedAt      time.Time
	CurrentMileage int
}

// Backfill assigns a first-due anchor to schedules that have never been
// completed, repairing rows created before task-030 that would otherwise read
// as permanently overdue (their next-due derives from a zero completion date,
// putting it in year 1) and that would now also fail validation on any PATCH.
//
// Idempotent: it only touches rows where BOTH new columns are still at their
// defaults, and every row it touches ends up with at least one of them set, so
// a second run selects nothing. Wired at startup on the pattern of
// maintenancecategory.Seed and fatal on error for the same reason — a service
// whose schedules are all falsely overdue is worse than one that refuses to
// start.
func Backfill(db *gorm.DB) error { return backfill(db, time.Now().UTC()) }

func backfill(db *gorm.DB, now time.Time) error {
	var rows []backfillRow
	if err := db.Table("fleet.maintenance_schedules AS s").
		Select("s.id, s.recurrence_type, s.interval_months, s.interval_miles, s.created_at, v.current_mileage").
		Joins("JOIN fleet.vehicles v ON v.id = s.vehicle_id").
		Where("s.one_time = ?", false).
		Where("s.due_date IS NULL AND s.due_mileage = 0").
		Where("s.last_completed_date IS NULL AND s.last_completed_mileage = 0").
		Where("s.deleted_at IS NULL").
		Scan(&rows).Error; err != nil {
		return err
	}

	for _, r := range rows {
		// The anchor is computed through the same Schedule/NextDue path every
		// other caller uses, so an anchored row and a completed row land on
		// identical date math.
		s := Schedule{
			RecurrenceType: r.RecurrenceType,
			IntervalMonths: r.IntervalMonths,
			IntervalMiles:  r.IntervalMiles,
		}
		if r.RecurrenceType == "time" || r.RecurrenceType == "hybrid" {
			s.DueDate = r.CreatedAt.AddDate(0, r.IntervalMonths, 0)
		}
		if r.RecurrenceType == "mileage" || r.RecurrenceType == "hybrid" {
			s.DueMileage = r.CurrentMileage + r.IntervalMiles
		}
		if s.DueDate.IsZero() && s.DueMileage == 0 {
			// An unrecognized recurrence type: nothing to anchor, and writing
			// nothing keeps the row selectable if the type is later corrected.
			continue
		}

		nd, nm := NextDue(s)
		state := DueState(s, now, r.CurrentMileage, DefaultThresholds)
		updates := map[string]any{
			"due_mileage":      s.DueMileage,
			"next_due_mileage": nm,
			"status":           state,
			"severity":         Severity(state),
		}
		if s.DueDate.IsZero() {
			updates["due_date"] = nil
		} else {
			updates["due_date"] = s.DueDate
		}
		if nd.IsZero() {
			updates["next_due_date"] = nil
		} else {
			updates["next_due_date"] = nd
		}
		if err := db.Table("fleet.maintenance_schedules").
			Where("id = ?", r.ID).Updates(updates).Error; err != nil {
			return err
		}
	}
	return nil
}
