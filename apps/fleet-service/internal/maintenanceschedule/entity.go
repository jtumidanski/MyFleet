package maintenanceschedule

import (
	"time"

	"gorm.io/gorm"
)

// Entity maps to fleet.maintenance_schedules (PRD §6, design §8.2, §10.1).
type Entity struct {
	ID                   string `gorm:"type:uuid;primaryKey"`
	VehicleID            string `gorm:"type:uuid;not null;index"`
	CategoryID           string `gorm:"type:uuid;not null"`
	RecurrenceType       string `gorm:"not null"` // time | mileage | hybrid
	IntervalMonths       int
	IntervalMiles        int
	LastCompletedDate    *time.Time
	LastCompletedMileage int
	NextDueDate          *time.Time
	NextDueMileage       int
	Status               string // ok | upcoming | overdue
	Severity             string // informational | recommended | urgent
	Active               bool   `gorm:"not null;default:true"`
	CreatedAt            time.Time
	UpdatedAt            time.Time
	DeletedAt            *time.Time `gorm:"index"`
	PurgeOperationID     *string    `gorm:"type:uuid;index"`
}

func (Entity) TableName() string { return "fleet.maintenance_schedules" }

func Migration(db *gorm.DB) error { return db.AutoMigrate(&Entity{}) }

// Make converts an Entity to a Model.
func Make(e Entity) Model {
	var lastDate, nextDate time.Time
	if e.LastCompletedDate != nil {
		lastDate = *e.LastCompletedDate
	}
	if e.NextDueDate != nil {
		nextDate = *e.NextDueDate
	}
	return Model{
		id:                   e.ID,
		vehicleID:            e.VehicleID,
		categoryID:           e.CategoryID,
		recurrenceType:       e.RecurrenceType,
		intervalMonths:       e.IntervalMonths,
		intervalMiles:        e.IntervalMiles,
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
	var lastDate, nextDate *time.Time
	if !m.lastCompletedDate.IsZero() {
		d := m.lastCompletedDate
		lastDate = &d
	}
	if !m.nextDueDate.IsZero() {
		d := m.nextDueDate
		nextDate = &d
	}
	return Entity{
		ID:                   m.id,
		VehicleID:            m.vehicleID,
		CategoryID:           m.categoryID,
		RecurrenceType:       m.recurrenceType,
		IntervalMonths:       m.intervalMonths,
		IntervalMiles:        m.intervalMiles,
		LastCompletedDate:    lastDate,
		LastCompletedMileage: m.lastCompletedMileage,
		NextDueDate:          nextDate,
		NextDueMileage:       m.nextDueMileage,
		Status:               m.status,
		Severity:             m.severity,
		Active:               m.active,
	}
}
