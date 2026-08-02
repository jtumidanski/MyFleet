package events

import (
	"encoding/json"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	dtoevents "github.com/jtumidanski/myfleet/packages/dto-go/events"
	sharedevents "github.com/jtumidanski/myfleet/packages/shared-go/events"
)

func newOutboxDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := sharedevents.MigrateOutbox(db); err != nil {
		t.Fatalf("migrate outbox: %v", err)
	}
	return db
}

// TestEmitVehicleCreated verifies emitting within a transaction produces exactly
// one unsent outbox row of type vehicle.created whose decoded data matches the
// supplied VehicleCreatedData (design A8).
func TestEmitVehicleCreated(t *testing.T) {
	db := newOutboxDB(t)

	data := dtoevents.VehicleCreatedData{VehicleID: "veh-1", FleetID: "fleet-1"}
	err := db.Transaction(func(tx *gorm.DB) error {
		return EmitVehicleCreated(tx, "fleet-1", "user-1", "trace-1", data)
	})
	if err != nil {
		t.Fatalf("EmitVehicleCreated: %v", err)
	}

	var rows []sharedevents.OutboxRow
	if err := db.Find(&rows).Error; err != nil {
		t.Fatalf("read outbox: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want exactly 1 outbox row, got %d", len(rows))
	}
	row := rows[0]
	if row.Type != "vehicle.created" {
		t.Fatalf("type=%q want vehicle.created", row.Type)
	}
	if row.SentAt != nil {
		t.Fatalf("sent_at must be NULL on enqueue, got %v", row.SentAt)
	}

	// The envelope's data must round-trip back to VehicleCreatedData.
	var env sharedevents.Envelope
	if err := json.Unmarshal(row.Payload, &env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if env.FleetID != "fleet-1" || env.ActorUserID != "user-1" || env.TraceID != "trace-1" {
		t.Fatalf("envelope fields mismatch: %+v", env)
	}
	dataBytes, _ := json.Marshal(env.Data)
	var decoded dtoevents.VehicleCreatedData
	if err := json.Unmarshal(dataBytes, &decoded); err != nil {
		t.Fatalf("decode data: %v", err)
	}
	if decoded != data {
		t.Fatalf("data=%+v want %+v", decoded, data)
	}
}

// TestEmitInviteCreated mirrors TestEmitVehicleCreated and adds the assertion
// that matters most for this event: the serialized payload contains no "token"
// key, at any nesting level (FR-EVT-3).
func TestEmitInviteCreated(t *testing.T) {
	db := newOutboxDB(t)

	data := dtoevents.InviteCreatedData{InviteID: "inv-1", Email: "a@b.com", Role: "member"}
	err := db.Transaction(func(tx *gorm.DB) error {
		return EmitInviteCreated(tx, "fleet-1", "user-1", "trace-1", data)
	})
	if err != nil {
		t.Fatalf("EmitInviteCreated: %v", err)
	}

	var rows []sharedevents.OutboxRow
	if err := db.Find(&rows).Error; err != nil {
		t.Fatalf("read outbox: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want exactly 1 outbox row, got %d", len(rows))
	}
	row := rows[0]
	if row.Type != "invite.created" {
		t.Fatalf("type=%q want invite.created", row.Type)
	}
	if row.SentAt != nil {
		t.Fatalf("sent_at must be NULL on enqueue, got %v", row.SentAt)
	}

	if strings.Contains(strings.ToLower(string(row.Payload)), "token") {
		t.Fatalf("payload must not mention a token, got %s", row.Payload)
	}

	var env sharedevents.Envelope
	if err := json.Unmarshal(row.Payload, &env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if env.FleetID != "fleet-1" || env.ActorUserID != "user-1" || env.TraceID != "trace-1" {
		t.Fatalf("envelope fields mismatch: %+v", env)
	}
	dataBytes, _ := json.Marshal(env.Data)
	var decoded dtoevents.InviteCreatedData
	if err := json.Unmarshal(dataBytes, &decoded); err != nil {
		t.Fatalf("decode data: %v", err)
	}
	if decoded != data {
		t.Fatalf("data=%+v want %+v", decoded, data)
	}
}
