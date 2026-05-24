package vehicle

import (
	"errors"
	"testing"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

func TestBuild_requiresMakeModelYear(t *testing.T) {
	_, err := NewBuilder().SetFleetID("f1").Build() // missing make/model/year
	if !errors.Is(err, server.ErrValidation) {
		t.Fatalf("missing required fields must be 422, got %v", err)
	}
}

func TestBuild_okWithRequired(t *testing.T) {
	m, err := NewBuilder().SetFleetID("f1").SetMake("Toyota").SetModel("Tacoma").SetYear(2021).Build()
	if err != nil {
		t.Fatalf("valid build failed: %v", err)
	}
	if m.FleetID() != "f1" || m.Make() != "Toyota" || m.Year() != 2021 {
		t.Fatalf("unexpected model: %+v", m)
	}
}
