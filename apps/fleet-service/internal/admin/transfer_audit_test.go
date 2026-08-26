package admin_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/admin/admintest"
)

// A transfer audit row must carry BOTH fleet ids as their own columns.
// FR-XFER-AUDIT-4 forbids encoding them in target_label, which is human-facing
// text, so this asserts they survive a real write and read-back.
func TestAuditEvent_carriesSourceAndDestinationFleetIDs(t *testing.T) {
	db := admintest.NewDB(t)
	adm := admin.NewAdministrator(db)
	now := time.Date(2026, 8, 25, 18, 0, 0, 0, time.UTC)

	ev := admin.AuditEvent{
		ID:                 "audit-1",
		ActorUserID:        "admin-1",
		ActorEmail:         "admin@example.com",
		Action:             admin.ActionVehicleTransferred,
		TargetType:         "vehicle",
		TargetID:           "vehicle-1",
		TargetLabel:        "The Green Bean",
		SourceFleetID:      "fleet-a",
		DestinationFleetID: "fleet-b",
		AffectedCounts:     map[string]int{"fuel_logs": 3},
		CorrelationID:      "corr-1",
		CreatedAt:          now,
	}
	if err := adm.InsertAudit(db, ev); err != nil {
		t.Fatalf("insert audit: %v", err)
	}

	var got admin.AuditEntity
	if err := db.Raw(`SELECT * FROM fleet.admin_audit_events WHERE id = ?`, "audit-1").
		Scan(&got).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	back := admin.MakeAudit(got)
	if back.SourceFleetID != "fleet-a" {
		t.Errorf("source_fleet_id = %q, want fleet-a", back.SourceFleetID)
	}
	if back.DestinationFleetID != "fleet-b" {
		t.Errorf("destination_fleet_id = %q, want fleet-b", back.DestinationFleetID)
	}
	if back.Action != "vehicle.transferred" {
		t.Errorf("action = %q, want vehicle.transferred", back.Action)
	}
}

// An empty fleet id must become NULL, not "": Postgres rejects the empty
// string for a uuid column, which is why every other optional id on this
// entity is a *string.
func TestAuditEvent_emptyFleetIDsBecomeNull(t *testing.T) {
	e := admin.AuditEvent{ID: "audit-2", Action: admin.ActionPurgeCreated}.ToEntity()
	if e.SourceFleetID != nil {
		t.Errorf("SourceFleetID = %v, want nil", *e.SourceFleetID)
	}
	if e.DestinationFleetID != nil {
		t.Errorf("DestinationFleetID = %v, want nil", *e.DestinationFleetID)
	}
}

// The console reads these off the JSON:API attributes, so the transform must
// emit both keys.
func TestTransformAuditEvents_emitsFleetIDKeys(t *testing.T) {
	res := admin.TransformAuditEvents([]admin.AuditEvent{{
		ID: "audit-3", Action: admin.ActionVehicleTransferred,
		SourceFleetID: "fleet-a", DestinationFleetID: "fleet-b",
	}})
	raw, err := json.Marshal(res[0].Attributes)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var attrs map[string]any
	if err := json.Unmarshal(raw, &attrs); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if attrs["source_fleet_id"] != "fleet-a" {
		t.Errorf("source_fleet_id = %v, want fleet-a", attrs["source_fleet_id"])
	}
	if attrs["destination_fleet_id"] != "fleet-b" {
		t.Errorf("destination_fleet_id = %v, want fleet-b", attrs["destination_fleet_id"])
	}
}
