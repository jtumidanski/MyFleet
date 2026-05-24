package activity

import (
	"encoding/json"

	"gorm.io/gorm"
)

// Administrator is the write interface for activity-event data access. The feed
// is APPEND-ONLY: there is no Update or Delete.
type Administrator interface {
	// Insert appends a single activity event.
	Insert(Model) (Model, error)
	// InsertTx appends a single activity event on the supplied transaction handle,
	// so the append is atomic with the domain write that triggered it.
	InsertTx(tx *gorm.DB, m Model) (Model, error)
}

type dbAdministrator struct{ db *gorm.DB }

// NewAdministrator returns an Administrator backed by the given database.
func NewAdministrator(db *gorm.DB) Administrator { return &dbAdministrator{db: db} }

func (a *dbAdministrator) Insert(m Model) (Model, error) {
	return a.InsertTx(a.db, m)
}

func (a *dbAdministrator) InsertTx(tx *gorm.DB, m Model) (Model, error) {
	e := m.ToEntity()
	if err := tx.Create(&e).Error; err != nil {
		return Model{}, err
	}
	return Make(e), nil
}

// Record appends a single activity event inside the caller's transaction
// (design §8.2). It MUST be called with the same tx as the domain write so the
// activity append commits or rolls back atomically with that write. Other
// domain processors call this from within their existing transactions.
//
// payload is marshaled to JSON for the activity_events.payload (jsonb) column.
func Record(tx *gorm.DB, actorUserID, eventType, fleetID string, vehicleID *string, payload map[string]any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	m := NewBuilder().
		SetFleetID(fleetID).
		SetVehicleID(vehicleID).
		SetActorUserID(actorUserID).
		SetType(eventType).
		SetPayload(encoded).
		Build()
	_, err = (&dbAdministrator{db: tx}).InsertTx(tx, m)
	return err
}
