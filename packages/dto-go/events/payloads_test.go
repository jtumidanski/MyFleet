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
