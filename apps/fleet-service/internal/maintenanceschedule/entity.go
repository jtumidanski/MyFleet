package maintenanceschedule

import (
	"time"

	"gorm.io/gorm"
)

// Entity maps to fleet.maintenance_schedules (PRD §6, design §8.2, §10.1).
type Entity struct {
	ID             string `gorm:"type:uuid;primaryKey"`
	VehicleID      string `gorm:"type:uuid;not null;index"`
	CategoryID     string `gorm:"type:uuid;not null"`
	RecurrenceType string `gorm:"not null"` // time | mileage | hybrid
	IntervalMonths int
	IntervalMiles  int
	// OneTime marks a schedule that is due once and never repeats (FR-OT-1).
	// It is orthogonal to RecurrenceType, which continues to say which AXES
	// the schedule is judged on rather than how often it repeats.
	OneTime bool `gorm:"not null;default:false"`
	// DueDate / DueMileage are the absolute due point. For a one-time schedule
	// they are permanent; for a recurring schedule they are the first-due
	// anchor, cleared on the first completion (FR-ANCHOR-3).
	DueDate              *time.Time
	DueMileage           int
	LastCompletedDate    *time.Time
	LastCompletedMileage int
	NextDueDate          *time.Time
	NextDueMileage       int
	Status               string // ok | upcoming | overdue
	Severity             string // informational | recommended | urgent
	// No "default:true" tag: GORM substitutes a field's "default" tag value for
	// its Go zero value at Create time, which would silently turn an explicit
	// Active=false (e.g. a completed one-time schedule, or a test seeding an
	// inactive row) into true. Every write path already sets Active explicitly
	// (the builder defaults it to true), so no DB-level default is needed.
	Active           bool `gorm:"not null"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        *time.Time `gorm:"index"`
	PurgeOperationID *string    `gorm:"type:uuid;index"`
}

func (Entity) TableName() string { return "fleet.maintenance_schedules" }

func Migration(db *gorm.DB) error { return db.AutoMigrate(&Entity{}) }

// Make converts an Entity to a Model.
func Make(e Entity) Model {
	var lastDate, nextDate, dueDate time.Time
	if e.LastCompletedDate != nil {
		lastDate = *e.LastCompletedDate
	}
	if e.NextDueDate != nil {
		nextDate = *e.NextDueDate
	}
	if e.DueDate != nil {
		dueDate = *e.DueDate
	}
	return Model{
		id:                   e.ID,
		vehicleID:            e.VehicleID,
		categoryID:           e.CategoryID,
		recurrenceType:       e.RecurrenceType,
		intervalMonths:       e.IntervalMonths,
		intervalMiles:        e.IntervalMiles,
		oneTime:              e.OneTime,
		dueDate:              dueDate,
		dueMileage:           e.DueMileage,
		lastCompletedDate:    lastDate,
		lastCompletedMileage: e.LastCompletedMileage,
		nextDueDate:          nextDate,
		nextDueMileage:       e.NextDueMileage,
		status:               e.Status,
		severity:             e.Severity,
		active:               e.Active,
		createdAt:            e.CreatedAt,
		updatedAt:            e.UpdatedAt,
	}
}

// ToEntity converts a Model to an Entity for persistence.
func (m Model) ToEntity() Entity {
	var lastDate, nextDate, dueDate *time.Time
	if !m.lastCompletedDate.IsZero() {
		d := m.lastCompletedDate
		lastDate = &d
	}
	if !m.nextDueDate.IsZero() {
		d := m.nextDueDate
		nextDate = &d
	}
	if !m.dueDate.IsZero() {
		d := m.dueDate
		dueDate = &d
	}
	return Entity{
		ID:                   m.id,
		VehicleID:            m.vehicleID,
		CategoryID:           m.categoryID,
		RecurrenceType:       m.recurrenceType,
		IntervalMonths:       m.intervalMonths,
		IntervalMiles:        m.intervalMiles,
		OneTime:              m.oneTime,
		DueDate:              dueDate,
		DueMileage:           m.dueMileage,
		LastCompletedDate:    lastDate,
		LastCompletedMileage: m.lastCompletedMileage,
		NextDueDate:          nextDate,
		NextDueMileage:       m.nextDueMileage,
		Status:               m.status,
		Severity:             m.severity,
		Active:               m.active,
	}
}
