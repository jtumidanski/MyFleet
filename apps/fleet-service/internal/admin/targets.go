package admin

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// dbTargetResolver resolves a purge root's human label and, for a record-scope
// vehicle purge, the media ids media-service must be told about.
type dbTargetResolver struct{ db *gorm.DB }

// NewTargetResolver returns the production TargetResolver.
func NewTargetResolver(db *gorm.DB) TargetResolver { return &dbTargetResolver{db: db} }

// labelSQL maps a record target type to the query that names one of its rows.
// A target with no natural name falls back to its type and id, which is still
// more readable in an audit row six months later than a bare uuid.
var labelSQL = map[string]string{
	TargetVehicle:             `SELECT nickname FROM fleet.vehicles WHERE id = ?`,
	TargetMaintenanceRecord:   `SELECT description FROM fleet.maintenance_records WHERE id = ?`,
	TargetMaintenanceSchedule: `SELECT name FROM fleet.maintenance_schedules WHERE id = ?`,
	TargetInvite:              `SELECT email FROM fleet.fleet_invites WHERE id = ?`,
}

// existsSQL maps a record target type to its table, so an unknown id is a 404
// rather than a purge of nothing that still writes an operation row.
var existsSQL = map[string]string{
	TargetVehicle:             `SELECT count(*) FROM fleet.vehicles WHERE id = ?`,
	TargetMaintenanceRecord:   `SELECT count(*) FROM fleet.maintenance_records WHERE id = ?`,
	TargetMaintenanceSchedule: `SELECT count(*) FROM fleet.maintenance_schedules WHERE id = ?`,
	TargetFuelLog:             `SELECT count(*) FROM fleet.fuel_logs WHERE id = ?`,
	TargetMileageRecord:       `SELECT count(*) FROM fleet.mileage_records WHERE id = ?`,
	TargetMembership:          `SELECT count(*) FROM fleet.fleet_memberships WHERE id = ?`,
	TargetInvite:              `SELECT count(*) FROM fleet.fleet_invites WHERE id = ?`,
	TargetVehicleMedia:        `SELECT count(*) FROM fleet.vehicle_media WHERE id = ?`,
	TargetActivityEvent:       `SELECT count(*) FROM fleet.activity_events WHERE id = ?`,
}

// Resolve names the root and collects the media ids a record purge must carry.
//
// The label is captured at request time, while the target still has a name —
// after the purge the rows are gone and the audit log would otherwise read as a
// bare uuid (FR-ADMIN-AUDIT-4).
func (r *dbTargetResolver) Resolve(root Root) (string, []string, error) {
	switch root.Scope {
	case ScopeSystem:
		return "the entire platform", nil, nil

	case ScopeFleet:
		var names []string
		if err := r.db.Raw(`SELECT name FROM fleet.fleets WHERE id = ?`, root.TargetID).
			Scan(&names).Error; err != nil {
			return "", nil, fmt.Errorf("resolve fleet label: %w", err)
		}
		if len(names) == 0 {
			return "", nil, server.ErrNotFound
		}
		return names[0], nil, nil

	case ScopeRecord:
		if err := r.requireRecord(root); err != nil {
			return "", nil, err
		}
		label := r.recordLabel(root)
		// Only a vehicle owns media. Every other record type either has none or
		// reaches it through the vehicle, so naming ids for them would send
		// media-service a set it must not act on.
		var mediaIDs []string
		if root.TargetType == TargetVehicle {
			var err error
			if mediaIDs, err = r.vehicleMediaIDs(root.TargetID); err != nil {
				return "", nil, err
			}
		}
		return label, mediaIDs, nil
	}
	return "", nil, server.Detailed(server.ErrValidation, "unsupported scope")
}

// requireRecord turns an unknown id into a 404 before anything is written.
func (r *dbTargetResolver) requireRecord(root Root) error {
	q, ok := existsSQL[root.TargetType]
	if !ok {
		return server.Detailed(server.ErrValidation, "unsupported target_type")
	}
	var n int64
	if err := r.db.Raw(q, root.TargetID).Scan(&n).Error; err != nil {
		return fmt.Errorf("resolve %s: %w", root.TargetType, err)
	}
	if n == 0 {
		return server.ErrNotFound
	}
	return nil
}

func (r *dbTargetResolver) recordLabel(root Root) string {
	q, ok := labelSQL[root.TargetType]
	if !ok {
		return root.TargetType + " " + root.TargetID
	}
	var labels []string
	if err := r.db.Raw(q, root.TargetID).Scan(&labels).Error; err != nil || len(labels) == 0 || labels[0] == "" {
		return root.TargetType + " " + root.TargetID
	}
	return labels[0]
}

// vehicleMediaIDs is the one place an explicit id set crosses a service
// boundary (design OQ-1): media objects carry a fleet_id, but "the media
// belonging to this vehicle" is a fact only fleet-service holds. It is both the
// vehicle's own images and the documents attached to its maintenance records.
func (r *dbTargetResolver) vehicleMediaIDs(vehicleID string) ([]string, error) {
	var ids []string
	if err := r.db.Raw(`
		SELECT media_id FROM fleet.vehicle_media
		 WHERE vehicle_id = ? AND media_id IS NOT NULL AND media_id <> ''
		UNION
		SELECT d.media_id FROM fleet.maintenance_record_documents d
		  JOIN fleet.maintenance_records m ON m.id = d.maintenance_record_id
		 WHERE m.vehicle_id = ? AND d.media_id IS NOT NULL AND d.media_id <> ''`,
		vehicleID, vehicleID).Scan(&ids).Error; err != nil {
		return nil, fmt.Errorf("resolve vehicle media ids: %w", err)
	}
	return ids, nil
}
