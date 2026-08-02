package maintenanceschedule

import (
	"errors"

	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

var ErrNotFound = errors.New("maintenance schedule not found")

// QueueRow pairs an active schedule with its vehicle's current mileage so the
// processor can compute DueState live (design A5). The join matches
// vehicles.fleet_id in the query (intra-service join within the fleet schema;
// D2 forbids only CROSS-SERVICE joins).
type QueueRow struct {
	Schedule       Model
	CurrentMileage int
	FleetID        string
}

// Provider is the read-only interface for maintenance schedule data access.
type Provider interface {
	GetByID(id string) (Model, error)
	ListByVehicle(vehicleID string, page server.Page) ([]Model, int, error)
	// ListActiveByFleet returns every active schedule whose vehicle belongs to
	// the fleet, paired with the vehicle's current mileage.
	ListActiveByFleet(fleetID string) ([]QueueRow, error)
	// ListActiveByVehicle returns every active schedule for a single vehicle,
	// paired with that vehicle's current mileage (for read-time DueState).
	ListActiveByVehicle(vehicleID string) ([]QueueRow, error)
	// ListActive returns every active schedule across all fleets, paired with
	// the vehicle's current mileage (used by the recompute job).
	ListActive() ([]QueueRow, error)
}

type dbProvider struct{ db *gorm.DB }

// NewProvider returns a read-only Provider backed by the given database.
func NewProvider(db *gorm.DB) Provider { return &dbProvider{db: db} }

func (p *dbProvider) GetByID(id string) (Model, error) {
	var e Entity
	if err := p.db.First(&e, "id = ? AND deleted_at IS NULL", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Model{}, ErrNotFound
		}
		return Model{}, err
	}
	return Make(e), nil
}

func (p *dbProvider) ListByVehicle(vehicleID string, page server.Page) ([]Model, int, error) {
	q := p.db.Model(&Entity{}).Where("vehicle_id = ? AND deleted_at IS NULL", vehicleID)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var es []Entity
	if err := p.db.Where("vehicle_id = ? AND deleted_at IS NULL", vehicleID).
		Order("created_at asc").Offset(page.Offset()).Limit(page.Size).Find(&es).Error; err != nil {
		return nil, 0, err
	}

	out := make([]Model, 0, len(es))
	for _, e := range es {
		out = append(out, Make(e))
	}
	return out, int(total), nil
}

// joinRow is the flat projection of the schedules→vehicles join. It embeds the
// schedule Entity columns plus the vehicle's current mileage.
type joinRow struct {
	Entity
	CurrentMileage int
	FleetID        string
}

func (p *dbProvider) ListActiveByFleet(fleetID string) ([]QueueRow, error) {
	return p.queryActive(&fleetID, nil)
}

func (p *dbProvider) ListActiveByVehicle(vehicleID string) ([]QueueRow, error) {
	return p.queryActive(nil, &vehicleID)
}

func (p *dbProvider) ListActive() ([]QueueRow, error) {
	return p.queryActive(nil, nil)
}

// queryActive joins active schedules to their (non-deleted) vehicle, optionally
// scoping to a single fleet and/or a single vehicle. Returns each schedule with
// the vehicle's current mileage for live DueState computation.
//
// `s.deleted_at IS NULL` is not cosmetic: the no-argument form backs
// /internal/maintenance/due, which drives notification-service's reminder job.
// Without it a purged schedule keeps generating notifications for a fleet that
// no longer exists.
func (p *dbProvider) queryActive(fleetID, vehicleID *string) ([]QueueRow, error) {
	q := p.db.Table("fleet.maintenance_schedules AS s").
		Select("s.*, v.current_mileage AS current_mileage, v.fleet_id AS fleet_id").
		Joins("JOIN fleet.vehicles v ON v.id = s.vehicle_id AND v.deleted_at IS NULL").
		Where("s.active = ? AND s.deleted_at IS NULL", true)
	if fleetID != nil {
		q = q.Where("v.fleet_id = ?", *fleetID)
	}
	if vehicleID != nil {
		q = q.Where("s.vehicle_id = ?", *vehicleID)
	}

	var rows []joinRow
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]QueueRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, QueueRow{Schedule: Make(r.Entity), CurrentMileage: r.CurrentMileage, FleetID: r.FleetID})
	}
	return out, nil
}
