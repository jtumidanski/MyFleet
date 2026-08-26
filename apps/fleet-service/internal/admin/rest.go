package admin

import (
	"time"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// JSON:API resource types for the admin tier (PRD §5). Attribute names are
// snake_case inside attributes, matching the PRD's examples exactly.
const (
	TypeStats      = "admin-stats"
	TypeFleet      = "admin-fleets"
	TypeUser       = "admin-users"
	TypeOperation  = "purge-operations"
	TypeAuditEvent = "admin-audit-events"

	TypeTransferPreview = "vehicle-transfer-previews"
	TypeTransfer        = "vehicle-transfers"
)

// statsAttributes carries nullable counts. A null is "we could not ask", which
// the console renders as an em dash rather than 0 (FR-ADMIN-UI-6), so these
// pointers must NOT be given omitempty — an absent key and a null key would
// read the same to the client, and only one of them is true.
type statsAttributes struct {
	Fleets               *int          `json:"fleets"`
	Memberships          *int          `json:"memberships"`
	MaintenanceRecords   *int          `json:"maintenance_records"`
	MaintenanceSchedules *int          `json:"maintenance_schedules"`
	FuelLogs             *int          `json:"fuel_logs"`
	MileageRecords       *int          `json:"mileage_records"`
	ActivityEvents       *int          `json:"activity_events"`
	PendingInvites       *int          `json:"pending_invites"`
	Users                *int          `json:"users"`
	MediaObjects         *int          `json:"media_objects"`
	Notifications        *int          `json:"notifications"`
	Vehicles             VehicleCounts `json:"vehicles"`
	Warnings             []string      `json:"warnings"`
}

// TransformStats renders /admin/stats. The id is fixed: there is exactly one
// stats resource and JSON:API requires an id.
func TransformStats(s Stats) server.Resource {
	return server.Resource{
		Type: TypeStats,
		ID:   "platform",
		Attributes: statsAttributes{
			Fleets:               s.Values["fleets"],
			Memberships:          s.Values["memberships"],
			MaintenanceRecords:   s.Values["maintenance_records"],
			MaintenanceSchedules: s.Values["maintenance_schedules"],
			FuelLogs:             s.Values["fuel_logs"],
			MileageRecords:       s.Values["mileage_records"],
			ActivityEvents:       s.Values["activity_events"],
			PendingInvites:       s.Values["pending_invites"],
			Users:                s.Values["users"],
			MediaObjects:         s.Values["media_objects"],
			Notifications:        s.Values["notifications"],
			Vehicles:             s.Vehicles,
			Warnings:             s.Warnings,
		},
	}
}

type fleetAttributes struct {
	Name             string     `json:"name"`
	CreatedAt        time.Time  `json:"created_at"`
	OwnerUserID      string     `json:"owner_user_id"`
	OwnerEmail       string     `json:"owner_email"`
	OwnerDisplayName string     `json:"owner_display_name"`
	MemberCount      int        `json:"member_count"`
	VehicleCount     int        `json:"vehicle_count"`
	PendingPurge     bool       `json:"pending_purge"`
	PurgeAfter       *time.Time `json:"purge_after"`
}

// TransformFleet renders one row of the admin fleet list.
func TransformFleet(f FleetRow) server.Resource {
	return server.Resource{
		Type: TypeFleet,
		ID:   f.ID,
		Attributes: fleetAttributes{
			Name:             f.Name,
			CreatedAt:        f.CreatedAt,
			OwnerUserID:      f.OwnerUserID,
			OwnerEmail:       f.OwnerEmail,
			OwnerDisplayName: f.OwnerDisplayName,
			MemberCount:      f.MemberCount,
			VehicleCount:     f.VehicleCount,
			PendingPurge:     f.PendingPurge,
			PurgeAfter:       f.PurgeAfter,
		},
	}
}

// TransformFleets renders a page of fleet rows.
func TransformFleets(rows []FleetRow) []server.Resource {
	out := make([]server.Resource, 0, len(rows))
	for _, r := range rows {
		out = append(out, TransformFleet(r))
	}
	return out
}

type fleetDetailAttributes struct {
	fleetAttributes
	Members  []MemberRow    `json:"members"`
	Vehicles []VehicleRow   `json:"vehicles"`
	Invites  []InviteRow    `json:"invites"`
	Counts   map[string]int `json:"counts"`
	Warnings []string       `json:"warnings"`
}

// TransformFleetDetail renders the fleet inspector's right pane.
func TransformFleetDetail(d FleetDetail) server.Resource {
	base := TransformFleet(d.FleetRow)
	return server.Resource{
		Type: TypeFleet,
		ID:   d.ID,
		Attributes: fleetDetailAttributes{
			fleetAttributes: base.Attributes.(fleetAttributes),
			Members:         d.Members,
			Vehicles:        d.Vehicles,
			Invites:         d.Invites,
			Counts:          d.Counts,
			Warnings:        d.Warnings,
		},
	}
}

type userAttributes struct {
	Email       string     `json:"email"`
	DisplayName string     `json:"display_name"`
	CreatedAt   time.Time  `json:"created_at"`
	LastLoginAt *time.Time `json:"last_login_at"`
	// FR-ADMIN-FLEET-6 requires this. The console displays it and offers no way
	// to change it — granting is a deliberate out-of-band act, but an operator
	// still needs to see who currently holds the privilege.
	PlatformAdmin bool           `json:"platform_admin"`
	Fleets        []UserFleetRow `json:"fleets"`
}

// TransformUsers renders a page of the cross-fleet user directory.
func TransformUsers(rows []UserRow) []server.Resource {
	out := make([]server.Resource, 0, len(rows))
	for _, u := range rows {
		out = append(out, server.Resource{
			Type: TypeUser,
			ID:   u.ID,
			Attributes: userAttributes{
				Email:         u.Email,
				DisplayName:   u.DisplayName,
				CreatedAt:     u.CreatedAt,
				LastLoginAt:   u.LastLoginAt,
				PlatformAdmin: u.PlatformAdmin,
				Fleets:        u.Fleets,
			},
		})
	}
	return out
}

type operationAttributes struct {
	Scope             string         `json:"scope"`
	TargetType        string         `json:"target_type"`
	TargetID          string         `json:"target_id"`
	TargetLabel       string         `json:"target_label"`
	Status            string         `json:"status"`
	RequestedByUserID string         `json:"requested_by_user_id"`
	RequestedByEmail  string         `json:"requested_by_email"`
	RequestedAt       time.Time      `json:"requested_at"`
	PurgeAfter        time.Time      `json:"purge_after"`
	ReapedAt          *time.Time     `json:"reaped_at"`
	CancelledAt       *time.Time     `json:"cancelled_at"`
	AffectedCounts    map[string]int `json:"affected_counts"`
	FailedServices    []string       `json:"failed_services"`
}

// TransformOperation renders one purge operation.
func TransformOperation(o Operation) server.Resource {
	return server.Resource{
		Type: TypeOperation,
		ID:   o.ID(),
		Attributes: operationAttributes{
			Scope:             string(o.Scope()),
			TargetType:        o.TargetType(),
			TargetID:          o.TargetID(),
			TargetLabel:       o.TargetLabel(),
			Status:            string(o.Status()),
			RequestedByUserID: o.RequestedByUserID(),
			RequestedByEmail:  o.RequestedByEmail(),
			RequestedAt:       o.RequestedAt(),
			PurgeAfter:        o.PurgeAfter(),
			ReapedAt:          o.ReapedAt(),
			CancelledAt:       o.CancelledAt(),
			AffectedCounts:    o.AffectedCounts(),
			FailedServices:    o.FailedServices(),
		},
	}
}

// TransformOperations renders a page of purge operations.
func TransformOperations(ops []Operation) []server.Resource {
	out := make([]server.Resource, 0, len(ops))
	for _, o := range ops {
		out = append(out, TransformOperation(o))
	}
	return out
}

type auditAttributes struct {
	ActorUserID        string         `json:"actor_user_id"`
	ActorEmail         string         `json:"actor_email"`
	Action             string         `json:"action"`
	Scope              string         `json:"scope"`
	TargetType         string         `json:"target_type"`
	TargetID           string         `json:"target_id"`
	TargetLabel        string         `json:"target_label"`
	PurgeOperationID   string         `json:"purge_operation_id"`
	AffectedCounts     map[string]int `json:"affected_counts"`
	SourceFleetID      string         `json:"source_fleet_id"`
	DestinationFleetID string         `json:"destination_fleet_id"`
	CorrelationID      string         `json:"correlation_id"`
	CreatedAt          time.Time      `json:"created_at"`
}

// TransformAuditEvents renders a page of audit rows.
func TransformAuditEvents(events []AuditEvent) []server.Resource {
	out := make([]server.Resource, 0, len(events))
	for _, a := range events {
		out = append(out, server.Resource{
			Type: TypeAuditEvent,
			ID:   a.ID,
			Attributes: auditAttributes{
				ActorUserID:        a.ActorUserID,
				ActorEmail:         a.ActorEmail,
				Action:             a.Action,
				Scope:              a.Scope,
				TargetType:         a.TargetType,
				TargetID:           a.TargetID,
				TargetLabel:        a.TargetLabel,
				PurgeOperationID:   a.PurgeOperationID,
				AffectedCounts:     a.AffectedCounts,
				SourceFleetID:      a.SourceFleetID,
				DestinationFleetID: a.DestinationFleetID,
				CorrelationID:      a.CorrelationID,
				CreatedAt:          a.CreatedAt,
			},
		})
	}
	return out
}

type transferPreviewAttributes struct {
	VehicleLabel string `json:"vehicle_label"`
	// The label the console must echo. Computed once, server-side, so the
	// client never derives the confirmation phrase independently
	// (FR-XFER-CONF-2).
	SourceFleetID        string             `json:"source_fleet_id"`
	SourceFleetName      string             `json:"source_fleet_name"`
	DestinationFleetID   string             `json:"destination_fleet_id"`
	DestinationFleetName string             `json:"destination_fleet_name"`
	Counts               map[string]int     `json:"counts"`
	CategoriesToCreate   []CategoryToCreate `json:"categories_to_create"`
	// Degradation notes. No omitempty: [] and absent must not read the same.
	Warnings []string `json:"warnings"`
}

// TransformTransferPreview renders the blast radius of a transfer that has not
// happened. The id is the vehicle's, because that is what the preview is of.
func TransformTransferPreview(p TransferPreview) server.Resource {
	return server.Resource{
		Type: TypeTransferPreview,
		ID:   p.VehicleID,
		Attributes: transferPreviewAttributes{
			VehicleLabel:         p.VehicleLabel,
			SourceFleetID:        p.SourceFleetID,
			SourceFleetName:      p.SourceFleetName,
			DestinationFleetID:   p.DestinationFleetID,
			DestinationFleetName: p.DestinationFleetName,
			Counts:               p.Counts,
			CategoriesToCreate:   p.CategoriesToCreate,
			Warnings:             p.Warnings,
		},
	}
}

type transferAttributes struct {
	VehicleID          string         `json:"vehicle_id"`
	SourceFleetID      string         `json:"source_fleet_id"`
	DestinationFleetID string         `json:"destination_fleet_id"`
	TransferredAt      time.Time      `json:"transferred_at"`
	AffectedCounts     map[string]int `json:"affected_counts"`
}

// TransformTransfer renders a completed transfer.
func TransformTransfer(r TransferResult) server.Resource {
	return server.Resource{
		Type:       TypeTransfer,
		ID:         r.VehicleID,
		Attributes: transferAttributes(r),
	}
}

// downstreamCountSemantics is the sentence attached to the two affected_counts
// keys that do not mean what the rest of the map means.
//
// Every fleet-service-local key in affected_counts is "rows THIS transfer
// moved", counted from the same predicate the UPDATE used. media_objects and
// notifications are not: both reassign endpoints return count(*) of live rows
// NOW ON THE DESTINATION for the named ids, so a row that was already there —
// a re-run, a prior partial transfer, or pre-existing state — is included and
// was not moved by this call (see mergeDownstreamCount).
//
// mergeDownstreamCount documents that in Go. A Go doc comment does not travel
// into the JSON an operator reads, and the console renders these numbers to a
// human who will otherwise read them as "moved" — so the response carries the
// correction with them.
const downstreamCountSemantics = "live rows now in the destination fleet for the ids this transfer named, " +
	"not the number moved by this transfer"

// downstreamCountKeys are exactly the affected_counts keys sourced from a
// downstream read-back. The fleet-service-local keys are deliberately NOT
// annotated: labelling every key would bury the two that need it.
var downstreamCountKeys = []string{"media_objects", "notifications"}

// TransferMeta annotates the counts whose names overstate what they measure.
//
// It rides in the document's `meta` rather than in `attributes` because it is
// information ABOUT the primary data rather than a property of the transfer,
// and because the attribute set is the published contract the console codes
// against — an annotation must not have to be added to every client's resource
// type before the numbers become honest.
//
// Renaming the keys would be blunter and better, but affected_counts is carried
// verbatim into the audit row (FR-XFER-AUDIT-3) and read back by the
// audit-event view, so the key names are not this layer's to change.
func TransferMeta(r TransferResult) map[string]any {
	semantics := make(map[string]string, len(downstreamCountKeys))
	for _, k := range downstreamCountKeys {
		if _, ok := r.AffectedCounts[k]; ok {
			semantics[k] = downstreamCountSemantics
		}
	}
	if len(semantics) == 0 {
		return nil
	}
	return map[string]any{"count_semantics": semantics}
}
