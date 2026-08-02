package events

import (
	"encoding/json"
	"testing"
)

func TestVehicleCreatedData_jsonTags(t *testing.T) {
	b, _ := json.Marshal(VehicleCreatedData{VehicleID: "v1", FleetID: "f1"})
	if string(b) != `{"vehicle_id":"v1","fleet_id":"f1"}` {
		t.Fatalf("unexpected json: %s", b)
	}
}

// The token is a bearer credential and must never ride the event bus (FR-EVT-3).
// Pinning the exact JSON is what makes an accidentally-added Token field fail here.
func TestInviteCreatedData_jsonTags(t *testing.T) {
	b, _ := json.Marshal(InviteCreatedData{InviteID: "i1", Email: "a@b.com", Role: "member"})
	if string(b) != `{"invite_id":"i1","email":"a@b.com","role":"member"}` {
		t.Fatalf("unexpected json: %s", b)
	}
}
