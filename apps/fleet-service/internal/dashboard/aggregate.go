package dashboard

import (
	"time"

	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/maintenanceschedule"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/status"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// SpendRow is a per-vehicle spend total within the query window.
// Both maintenance costs and fuel costs are summed and combined.
type SpendRow struct {
	VehicleID       string  `json:"vehicleId"`
	MaintenanceCost float64 `json:"maintenanceCost"`
	FuelCost        float64 `json:"fuelCost"`
	TotalCost       float64 `json:"totalCost"`
}

// MileagePoint is a single mileage reading over time.
type MileagePoint struct {
	RecordedAt string `json:"recordedAt"`
	Mileage    int    `json:"mileage"`
	Source     string `json:"source"`
}

// OverviewCounts holds status-based vehicle counts + schedule totals.
type OverviewCounts struct {
	Healthy             int `json:"healthy"`
	UpcomingMaintenance int `json:"upcomingMaintenance"`
	Overdue             int `json:"overdue"`
	Inactive            int `json:"inactive"`
	TotalVehicles       int `json:"totalVehicles"`
	UpcomingSchedules   int `json:"upcomingSchedules"`
	OverdueSchedules    int `json:"overdueSchedules"`
}

// AggregateProvider exposes read aggregations computed on read (design A5).
type AggregateProvider interface {
	SpendByVehicle(fleetID string, from, to time.Time) ([]SpendRow, error)
	MileageTrends(vehicleID string, from, to time.Time) ([]MileagePoint, error)
}

type dbAggregateProvider struct{ db *gorm.DB }

// NewAggregateProvider returns an AggregateProvider backed by the given database.
func NewAggregateProvider(db *gorm.DB) AggregateProvider { return &dbAggregateProvider{db: db} }

// spendVehicleRow is the projection from the maintenance cost sub-query.
type spendVehicleRow struct {
	VehicleID string
	Total     float64
}

// SpendByVehicle sums SUM(maintenance_records.cost) + SUM(fuel_logs.total_cost)
// per vehicle within [from, to], bounded by fleet_id.
// Both cost sources are unioned then grouped by vehicle_id (design A5, §13).
func (p *dbAggregateProvider) SpendByVehicle(fleetID string, from, to time.Time) ([]SpendRow, error) {
	// Maintenance cost sub-query scoped to fleet via vehicles join.
	maintSQL := `
		SELECT mr.vehicle_id AS vehicle_id, SUM(mr.cost) AS total
		FROM fleet.maintenance_records mr
		JOIN fleet.vehicles v ON v.id = mr.vehicle_id AND v.deleted_at IS NULL
		WHERE v.fleet_id = ?
		  AND mr.deleted_at IS NULL`
	maintArgs := []any{fleetID}
	if !from.IsZero() {
		maintSQL += " AND mr.performed_at >= ?"
		maintArgs = append(maintArgs, from)
	}
	if !to.IsZero() {
		maintSQL += " AND mr.performed_at <= ?"
		maintArgs = append(maintArgs, to)
	}
	maintSQL += " GROUP BY mr.vehicle_id"

	// Fuel cost sub-query scoped to fleet via vehicles join.
	fuelSQL := `
		SELECT fl.vehicle_id AS vehicle_id, SUM(fl.total_cost) AS total
		FROM fleet.fuel_logs fl
		JOIN fleet.vehicles v ON v.id = fl.vehicle_id AND v.deleted_at IS NULL
		WHERE v.fleet_id = ?
		  AND fl.deleted_at IS NULL`
	fuelArgs := []any{fleetID}
	if !from.IsZero() {
		fuelSQL += " AND fl.date >= ?"
		fuelArgs = append(fuelArgs, from)
	}
	if !to.IsZero() {
		fuelSQL += " AND fl.date <= ?"
		fuelArgs = append(fuelArgs, to)
	}
	fuelSQL += " GROUP BY fl.vehicle_id"

	// Fetch maintenance rows.
	var maintRows []spendVehicleRow
	if err := p.db.Raw(maintSQL, maintArgs...).Scan(&maintRows).Error; err != nil {
		return nil, err
	}

	// Fetch fuel rows.
	var fuelRows []spendVehicleRow
	if err := p.db.Raw(fuelSQL, fuelArgs...).Scan(&fuelRows).Error; err != nil {
		return nil, err
	}

	// Merge both cost sources by vehicle_id.
	totals := map[string]*SpendRow{}
	for _, r := range maintRows {
		sr := spendRowFor(totals, r.VehicleID)
		sr.MaintenanceCost += r.Total
		sr.TotalCost += r.Total
	}
	for _, r := range fuelRows {
		sr := spendRowFor(totals, r.VehicleID)
		sr.FuelCost += r.Total
		sr.TotalCost += r.Total
	}

	out := make([]SpendRow, 0, len(totals))
	for _, sr := range totals {
		out = append(out, *sr)
	}
	return out, nil
}

func spendRowFor(m map[string]*SpendRow, vehicleID string) *SpendRow {
	if _, ok := m[vehicleID]; !ok {
		m[vehicleID] = &SpendRow{VehicleID: vehicleID}
	}
	return m[vehicleID]
}

// MileageTrends returns per-vehicle mileage points from fleet.mileage_records
// ordered by recorded_at, optionally bounded by [from, to].
func (p *dbAggregateProvider) MileageTrends(vehicleID string, from, to time.Time) ([]MileagePoint, error) {
	type row struct {
		RecordedAt time.Time
		Mileage    int
		Source     string
	}
	// deleted_at IS NULL is hand-written here because this query bypasses the
	// mileage provider entirely (design §9).
	q := p.db.Table("fleet.mileage_records").
		Select("recorded_at, mileage, source").
		Where("vehicle_id = ? AND deleted_at IS NULL", vehicleID)
	if !from.IsZero() {
		q = q.Where("recorded_at >= ?", from)
	}
	if !to.IsZero() {
		q = q.Where("recorded_at <= ?", to)
	}
	q = q.Order("recorded_at asc")

	var rows []row
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]MileagePoint, 0, len(rows))
	for _, r := range rows {
		out = append(out, MileagePoint{
			RecordedAt: r.RecordedAt.Format("2006-01-02T15:04:05Z07:00"),
			Mileage:    r.Mileage,
			Source:     r.Source,
		})
	}
	return out, nil
}

// computeOverview derives status counts across all vehicles in a fleet.
// It reuses status.Derive over each vehicle's active-schedule DueStates + last
// activity (design §10.2, plan §13.2).
func computeOverview(
	fleetID string,
	vehicles VehicleListReader,
	schedules ScheduleStateReader,
	activity LastActivityReader,
) (OverviewCounts, error) {
	// Fetch all active schedules for the fleet (one query).
	queueRows, err := schedules.ListActiveByFleet(fleetID)
	if err != nil {
		return OverviewCounts{}, err
	}

	// Build per-vehicle schedule state map + count upcoming/overdue schedules.
	vehicleStates := map[string][]string{} // vehicleID → []DueState
	var upcomingSchedules, overdueSchedules int
	for _, qr := range queueRows {
		ds := maintenanceschedule.DueState(
			qr.Schedule.AsSchedule(),
			time.Now().UTC(),
			qr.CurrentMileage,
			maintenanceschedule.DefaultThresholds,
		)
		vehicleStates[qr.Schedule.VehicleID()] = append(vehicleStates[qr.Schedule.VehicleID()], ds)
		switch ds {
		case "upcoming":
			upcomingSchedules++
		case "overdue":
			overdueSchedules++
		}
	}

	// Fetch all vehicles in the fleet (unbounded for overview).
	const maxVehicles = 10000
	vs, _, err := vehicles.ListByFleet(fleetID, server.Page{Number: 1, Size: maxVehicles})
	if err != nil {
		return OverviewCounts{}, err
	}

	var healthy, upcoming, overdue, inactive int
	for _, v := range vs {
		lastActivity, err := activity.LastActivityByVehicle(v.ID())
		if err != nil {
			return OverviewCounts{}, err
		}
		derived := status.Derive(status.Input{
			ScheduleStates: vehicleStates[v.ID()],
			LastActivityAt: lastActivity,
			Now:            time.Now().UTC(),
			InactivityDays: 365,
		})
		switch derived {
		case "Healthy":
			healthy++
		case "Upcoming Maintenance":
			upcoming++
		case "Overdue":
			overdue++
		case "Inactive":
			inactive++
		}
	}

	return OverviewCounts{
		Healthy:             healthy,
		UpcomingMaintenance: upcoming,
		Overdue:             overdue,
		Inactive:            inactive,
		TotalVehicles:       len(vs),
		UpcomingSchedules:   upcomingSchedules,
		OverdueSchedules:    overdueSchedules,
	}, nil
}
