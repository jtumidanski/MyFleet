package vehicle

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func restVehicle() Model {
	return Model{
		id: "v1", fleetID: "f1", make: "Honda", model: "Civic", year: 2019,
		currentMileage: 42000,
	}
}

// attributesJSON marshals a resource and returns its attributes object as a map,
// so absence of a key is distinguishable from a zero value.
func attributesJSON(t *testing.T, r interface{}) map[string]any {
	t.Helper()
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var doc struct {
		Attributes map[string]any `json:"attributes"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return doc.Attributes
}

func TestTransformDerived_omitsBothAttributesWhenDerivedIsZero(t *testing.T) {
	attrs := attributesJSON(t, TransformDerived(restVehicle(), Derived{}))
	if _, ok := attrs["lastActivityAt"]; ok {
		t.Fatalf("lastActivityAt must be omitted, got %v", attrs["lastActivityAt"])
	}
	if _, ok := attrs["nextDue"]; ok {
		t.Fatalf("nextDue must be omitted, got %v", attrs["nextDue"])
	}
	if _, ok := attrs["status"]; ok {
		t.Fatalf("status must be omitted, got %v", attrs["status"])
	}
}

func TestTransformDerived_lastActivityIsRFC3339UTC(t *testing.T) {
	last := time.Date(2026, 4, 2, 14, 31, 7, 0, time.UTC)
	attrs := attributesJSON(t, TransformDerived(restVehicle(), Derived{Status: "Healthy", LastActivityAt: last}))
	if attrs["lastActivityAt"] != "2026-04-02T14:31:07Z" {
		t.Fatalf("lastActivityAt = %v", attrs["lastActivityAt"])
	}
}

func TestTransformDerived_lastActivityIsNormalisedToUTC(t *testing.T) {
	// A non-UTC timestamp reaching the transport must still serialise as UTC, so
	// the client never has to reason about a server's local zone.
	zone := time.FixedZone("EST", -5*60*60)
	last := time.Date(2026, 4, 2, 9, 31, 7, 0, zone)
	attrs := attributesJSON(t, TransformDerived(restVehicle(), Derived{LastActivityAt: last}))
	if attrs["lastActivityAt"] != "2026-04-02T14:31:07Z" {
		t.Fatalf("lastActivityAt = %v", attrs["lastActivityAt"])
	}
}

func TestTransformDerived_nextDueEmitsMilesXorDays(t *testing.T) {
	miles := 1120
	attrs := attributesJSON(t, TransformDerived(restVehicle(), Derived{
		Status:  "Overdue",
		NextDue: &NextDue{State: "overdue", Axis: "mileage", Miles: &miles},
	}))
	nd, ok := attrs["nextDue"].(map[string]any)
	if !ok {
		t.Fatalf("nextDue = %v", attrs["nextDue"])
	}
	if nd["state"] != "overdue" || nd["axis"] != "mileage" || nd["miles"] != float64(1120) {
		t.Fatalf("nextDue = %v", nd)
	}
	if _, present := nd["days"]; present {
		t.Fatalf("a mileage axis must not emit days, got %v", nd)
	}
}

func TestTransformDerived_nextDueKeepsAZeroDayCount(t *testing.T) {
	// The reason Days is a pointer: "due today" is days == 0, and omitempty on a
	// plain int would drop the key and leave an axis:"time" object with no
	// magnitude at all.
	days := 0
	attrs := attributesJSON(t, TransformDerived(restVehicle(), Derived{
		Status:  "Upcoming Maintenance",
		NextDue: &NextDue{State: "upcoming", Axis: "time", Days: &days},
	}))
	nd := attrs["nextDue"].(map[string]any)
	if nd["days"] != float64(0) {
		t.Fatalf("days must be present and zero, got %v", nd)
	}
	if _, present := nd["miles"]; present {
		t.Fatalf("a time axis must not emit miles, got %v", nd)
	}
}

func TestTransform_writePathEmitsNoDerivedAttributes(t *testing.T) {
	// Create/update/restore/primary-image responses echo a write, and none of the
	// derived values is a property of the write.
	attrs := attributesJSON(t, Transform(restVehicle()))
	for _, k := range []string{"status", "lastActivityAt", "nextDue"} {
		if _, ok := attrs[k]; ok {
			t.Fatalf("write path must not emit %q, got %v", k, attrs[k])
		}
	}
}

func TestCreateAndPatchBindingsRejectDerivedAttributes(t *testing.T) {
	// FR-8.3 / NFR-7: a client must not be able to write a derived value. The
	// handlers bind these narrow types, which simply have nowhere to put the
	// derived keys — asserted against the real types rather than trusted.
	body := `{"nickname":"Mine","currentMileage":1,"status":"Healthy",` +
		`"lastActivityAt":"2020-01-01T00:00:00Z","nextDue":{"state":"overdue","axis":"mileage","miles":9}}`

	var create createAttributes
	if err := json.Unmarshal([]byte(body), &create); err != nil {
		t.Fatalf("unmarshal create: %v", err)
	}
	var patch patchAttributes
	if err := json.Unmarshal([]byte(body), &patch); err != nil {
		t.Fatalf("unmarshal patch: %v", err)
	}

	for _, typ := range []reflect.Type{reflect.TypeOf(create), reflect.TypeOf(patch)} {
		for i := 0; i < typ.NumField(); i++ {
			tag := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
			switch tag {
			case "status", "lastActivityAt", "nextDue":
				t.Fatalf("%s must not accept a derived attribute %q", typ.Name(), tag)
			}
		}
	}

	// And the fields that DO exist still bind, so the assertion above is not
	// passing because the types are empty.
	if create.Nickname != "Mine" || create.CurrentMileage != 1 {
		t.Fatalf("create bindings broke: %+v", create)
	}
	if patch.Nickname == nil || *patch.Nickname != "Mine" {
		t.Fatalf("patch bindings broke: %+v", patch)
	}
}
