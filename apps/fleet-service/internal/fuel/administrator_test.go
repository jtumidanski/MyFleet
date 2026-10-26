package fuel

import (
	"errors"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/mileage"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/vehicle"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

func seedVehicleAt(t *testing.T, db *gorm.DB, miles int) vehicle.Model {
	t.Helper()
	v, err := vehicle.NewBuilder().
		SetFleetID("fleet-1").
		SetMake("Toyota").
		SetModel("Camry").
		SetYear(2022).
		SetCurrentMileage(miles).
		Build()
	if err != nil {
		t.Fatalf("build vehicle: %v", err)
	}
	created, err := vehicle.NewAdministrator(db).Insert(v)
	if err != nil {
		t.Fatalf("insert vehicle: %v", err)
	}
	return created
}

func logFuelAt(t *testing.T, deps LoggingDeps, v vehicle.Model, miles int) Model {
	t.Helper()
	fl, err := NewBuilder().
		SetVehicleID(v.ID()).
		SetDate(time.Now().UTC()).
		SetMileage(miles).
		SetGallons(10.0).
		SetTotalCost(40.0).
		SetPricePerGallon(4.0).
		SetCreatedByUserID("user-1").
		Build()
	if err != nil {
		t.Fatalf("build fuel log: %v", err)
	}
	created, err := deps.LogInTransaction(logrus.New(), LogInput{FuelLog: fl, FleetID: v.FleetID()})
	if err != nil {
		t.Fatalf("log fuel: %v", err)
	}
	return created
}

func currentMileage(t *testing.T, db *gorm.DB, vehicleID string) int {
	t.Helper()
	var cm int
	if err := db.Table("fleet.vehicles").Select("current_mileage").Where("id = ?", vehicleID).Scan(&cm).Error; err != nil {
		t.Fatalf("read current_mileage: %v", err)
	}
	return cm
}

func TestSoftDelete_retractsFuelMileageAndRecomputes(t *testing.T) {
	db := newFuelDB(t)
	ledger := mileage.NewAdministrator(db)
	v := seedVehicleAt(t, db, 40000)
	deps := NewLoggingDeps(db, ledger)
	logFuelAt(t, deps, v, 41000)
	second := logFuelAt(t, deps, v, 42000)
	if got := currentMileage(t, db, v.ID()); got != 42000 {
		t.Fatalf("precondition: current_mileage = %d, want 42000", got)
	}

	if err := NewAdministrator(db, ledger).SoftDelete(second.ID(), v.ID()); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	var fuelDeleted int64
	db.Table("fleet.fuel_logs").Where("id = ? AND deleted_at IS NOT NULL", second.ID()).Count(&fuelDeleted)
	if fuelDeleted != 1 {
		t.Fatalf("fuel row deleted_at count = %d, want 1", fuelDeleted)
	}
	var mileageDeleted int64
	db.Table("fleet.mileage_records").
		Where("source = ? AND source_ref_id = ? AND deleted_at IS NOT NULL", mileage.SourceFuel, second.ID()).
		Count(&mileageDeleted)
	if mileageDeleted != 1 {
		t.Fatalf("mileage row deleted_at count = %d, want 1", mileageDeleted)
	}
	if got := currentMileage(t, db, v.ID()); got != 41000 {
		t.Fatalf("current_mileage = %d, want 41000", got)
	}
}

func TestSoftDelete_unknownIDIsNotFound(t *testing.T) {
	db := newFuelDB(t)
	ledger := mileage.NewAdministrator(db)
	v := seedVehicleAt(t, db, 40000)

	err := NewAdministrator(db, ledger).SoftDelete("missing-id", v.ID())
	if !errors.Is(err, server.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if got := currentMileage(t, db, v.ID()); got != 40000 {
		t.Fatalf("current_mileage = %d, want 40000", got)
	}
}
