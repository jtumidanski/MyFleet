package maintenancerecord

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/mileage"
)

func TestMileageEntryForCreate(t *testing.T) {
	performed := time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC)
	build := func(miles int) Model {
		m, err := NewBuilder().SetVehicleID("v1").SetCategoryID("c").SetPerformedAt(performed).
			SetMileage(miles).SetCreatedByUserID("creator").Build()
		if err != nil {
			t.Fatalf("build: %v", err)
		}
		return m
	}
	if got := mileageEntryForCreate(build(0)); got != nil {
		t.Errorf("mileage 0: got %v, want nil", got)
	}
	m := build(52310)
	got := mileageEntryForCreate(m)
	if got == nil {
		t.Fatal("mileage 52310: got nil entry")
	}
	if got.Source() != mileage.SourceMaintenance || got.SourceRefID() != m.ID() ||
		!got.RecordedAt().Equal(m.PerformedAt()) || got.CreatedByUserID() != "creator" ||
		got.VehicleID() != "v1" || got.Mileage() != 52310 {
		t.Errorf("unexpected entry: %+v", got)
	}
}

func TestMileageEntryForUpdate(t *testing.T) {
	m, err := NewBuilder().SetVehicleID("v1").SetCategoryID("c").
		SetPerformedAt(time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC)).SetMileage(100).Build()
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if got := mileageEntryForUpdate(m, m, "editor"); got != nil {
		t.Errorf("unchanged: got %v, want nil", got)
	}
	if got := mileageEntryForUpdate(m, m.WithMileage(0), "editor"); got != nil {
		t.Errorf("to zero: got %v, want nil", got)
	}
	after := m.WithMileage(200)
	got := mileageEntryForUpdate(m, after, "editor")
	if got == nil {
		t.Fatal("changed: got nil entry")
	}
	if got.Mileage() != 200 || !got.RecordedAt().Equal(after.PerformedAt()) || got.CreatedByUserID() != "editor" ||
		got.Source() != mileage.SourceMaintenance || got.SourceRefID() != m.ID() {
		t.Errorf("unexpected entry: %+v", got)
	}
}

// --- DB-backed tests ---

type failingLedger struct{ appendErr, retractErr error }

func (f failingLedger) AppendTx(*gorm.DB, mileage.Model) (mileage.Model, error) {
	return mileage.Model{}, f.appendErr
}
func (f failingLedger) RetractTx(*gorm.DB, string, string, string) error { return f.retractErr }

func seedVehicle(t *testing.T, db *gorm.DB, miles int) {
	t.Helper()
	if err := db.Exec(`INSERT INTO fleet.vehicles (id, fleet_id, current_mileage) VALUES ('v1','f1',?)`, miles).Error; err != nil {
		t.Fatalf("seed vehicle: %v", err)
	}
}

func seedMileageRow(t *testing.T, db *gorm.DB, id string, miles int, source, ref string) {
	t.Helper()
	if err := db.Exec(`INSERT INTO fleet.mileage_records (id, vehicle_id, mileage, recorded_at, source, source_ref_id, created_at)
		VALUES (?, 'v1', ?, ?, ?, ?, ?)`, id, miles, time.Now().UTC(), source, ref, time.Now().UTC()).Error; err != nil {
		t.Fatalf("seed mileage: %v", err)
	}
}

func currentMileage(t *testing.T, db *gorm.DB) int {
	t.Helper()
	var n int
	if err := db.Raw(`SELECT current_mileage FROM fleet.vehicles WHERE id='v1'`).Scan(&n).Error; err != nil {
		t.Fatalf("read odometer: %v", err)
	}
	return n
}

type ledgerRow struct {
	ID              string
	Mileage         int
	RecordedAt      time.Time
	CreatedByUserID string
	DeletedAt       *time.Time
}

func ledgerRows(t *testing.T, db *gorm.DB, ref string, liveOnly bool) []ledgerRow {
	t.Helper()
	q := `SELECT id, mileage, recorded_at, created_by_user_id, deleted_at FROM fleet.mileage_records
		WHERE source = ? AND source_ref_id = ?`
	if liveOnly {
		q += ` AND deleted_at IS NULL`
	}
	var rows []ledgerRow
	if err := db.Raw(q+` ORDER BY created_at, rowid`, mileage.SourceMaintenance, ref).Scan(&rows).Error; err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	return rows
}

const performedAtStr = "2026-02-03T00:00:00Z"

func postRecord(t *testing.T, r chi.Router, attrs string) string {
	t.Helper()
	rec := serveAs(r, http.MethodPost, "/vehicles/v1/maintenance-records",
		`{"data":{"type":"maintenanceRecords","attributes":`+attrs+`}}`, identity("owner", "f1"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d: %s", rec.Code, rec.Body.String())
	}
	var doc struct{ Data struct{ ID string } }
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return doc.Data.ID
}

func TestCreate_withMileageAppendsAndAdvances(t *testing.T) {
	r, db := newRecordRouter(t)
	seedVehicle(t, db, 50000)
	id := postRecord(t, r, `{"categoryId":"c1","performedAt":"`+performedAtStr+`","mileage":52310}`)

	rows := ledgerRows(t, db, id, true)
	if len(rows) != 1 || rows[0].Mileage != 52310 {
		t.Fatalf("rows = %+v, want one at 52310", rows)
	}
	want, _ := time.Parse(time.RFC3339, performedAtStr)
	if !rows[0].RecordedAt.Equal(want) {
		t.Errorf("recorded_at = %v, want %v", rows[0].RecordedAt, want)
	}
	if got := currentMileage(t, db); got != 52310 {
		t.Errorf("current_mileage = %d, want 52310", got)
	}
}

func TestCreate_zeroMileageWritesNoEntry(t *testing.T) {
	r, db := newRecordRouter(t)
	seedVehicle(t, db, 50000)
	id := postRecord(t, r, `{"categoryId":"c1","performedAt":"`+performedAtStr+`"}`)

	if rows := ledgerRows(t, db, id, false); len(rows) != 0 {
		t.Errorf("rows = %+v, want none", rows)
	}
	if got := currentMileage(t, db); got != 50000 {
		t.Errorf("current_mileage = %d, want 50000", got)
	}
}

func TestCreate_belowOdometerStoredNotApplied(t *testing.T) {
	r, db := newRecordRouter(t)
	seedVehicle(t, db, 50000)
	id := postRecord(t, r, `{"categoryId":"c1","performedAt":"`+performedAtStr+`","mileage":40000}`)

	if rows := ledgerRows(t, db, id, true); len(rows) != 1 {
		t.Errorf("rows = %+v, want one", rows)
	}
	if got := currentMileage(t, db); got != 50000 {
		t.Errorf("current_mileage = %d, want 50000", got)
	}
}

func TestCreate_ledgerFailureRollsBackRecord(t *testing.T) {
	db := newTestDB(t)
	seedVehicle(t, db, 50000)
	a := NewAdministrator(db, failingLedger{appendErr: errors.New("boom")})
	m, err := NewBuilder().SetVehicleID("v1").SetCategoryID("c1").
		SetPerformedAt(time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC)).SetMileage(52310).Build()
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if _, err := a.Insert(m, mileageEntryForCreate(m)); err == nil {
		t.Fatal("Insert succeeded, want ledger error")
	}
	var n int64
	db.Raw(`SELECT COUNT(*) FROM fleet.maintenance_records WHERE id = ?`, m.ID()).Scan(&n)
	if n != 0 {
		t.Errorf("record rows = %d, want 0 (rolled back)", n)
	}
}

func TestPatch_changedMileageAppends(t *testing.T) {
	r, db := newRecordRouter(t)
	seedVehicle(t, db, 50000)
	id := postRecord(t, r, `{"categoryId":"c1","performedAt":"`+performedAtStr+`","mileage":52310}`)
	first := ledgerRows(t, db, id, true)[0]

	if rec := patchRecord(r, id, `{"mileage":53000}`, identity("owner", "f1")); rec.Code != http.StatusOK {
		t.Fatalf("patch status = %d: %s", rec.Code, rec.Body.String())
	}
	rows := ledgerRows(t, db, id, true)
	if len(rows) != 2 {
		t.Fatalf("rows = %+v, want 2", rows)
	}
	if rows[0].ID != first.ID || rows[0].Mileage != 52310 {
		t.Errorf("first row changed: %+v vs %+v", rows[0], first)
	}
	if rows[1].Mileage != 53000 || rows[1].CreatedByUserID != "u1" {
		t.Errorf("second row = %+v, want 53000 by u1", rows[1])
	}
	if got := currentMileage(t, db); got != 53000 {
		t.Errorf("current_mileage = %d, want 53000", got)
	}
}

func TestPatch_noMileageChangeAppendsNothing(t *testing.T) {
	r, db := newRecordRouter(t)
	seedVehicle(t, db, 50000)
	id := postRecord(t, r, `{"categoryId":"c1","performedAt":"`+performedAtStr+`","mileage":52310}`)

	for _, body := range []string{`{"vendor":"x"}`, `{"mileage":52310}`} {
		if rec := patchRecord(r, id, body, identity("owner", "f1")); rec.Code != http.StatusOK {
			t.Fatalf("patch %s status = %d", body, rec.Code)
		}
	}
	if rows := ledgerRows(t, db, id, true); len(rows) != 1 {
		t.Errorf("rows = %+v, want 1", rows)
	}
}

func TestPatch_zeroMileageAppendsNothing(t *testing.T) {
	r, db := newRecordRouter(t)
	seedVehicle(t, db, 50000)
	id := postRecord(t, r, `{"categoryId":"c1","performedAt":"`+performedAtStr+`","mileage":52310}`)

	if rec := patchRecord(r, id, `{"mileage":0}`, identity("owner", "f1")); rec.Code != http.StatusOK {
		t.Fatalf("patch status = %d: %s", rec.Code, rec.Body.String())
	}
	if rows := ledgerRows(t, db, id, true); len(rows) != 1 {
		t.Errorf("rows = %+v, want 1", rows)
	}
	if got := currentMileage(t, db); got != 52310 {
		t.Errorf("current_mileage = %d, want 52310", got)
	}
}

func TestDelete_retractsAllEntriesAndRecomputes(t *testing.T) {
	r, db := newRecordRouter(t)
	seedVehicle(t, db, 50000)
	seedMileageRow(t, db, "manual-1", 51000, mileage.SourceManual, "")
	id := postRecord(t, r, `{"categoryId":"c1","performedAt":"`+performedAtStr+`","mileage":52310}`)
	for _, body := range []string{`{"mileage":53000}`, `{"mileage":54000}`} {
		if rec := patchRecord(r, id, body, identity("owner", "f1")); rec.Code != http.StatusOK {
			t.Fatalf("patch status = %d", rec.Code)
		}
	}
	if rec := serveAs(r, http.MethodDelete, "/maintenance-records/"+id, "", identity("owner", "f1")); rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d: %s", rec.Code, rec.Body.String())
	}
	all := ledgerRows(t, db, id, false)
	if len(all) != 3 {
		t.Fatalf("rows = %d, want 3", len(all))
	}
	for _, row := range all {
		if row.DeletedAt == nil {
			t.Errorf("row %s not retracted", row.ID)
		}
	}
	if got := currentMileage(t, db); got != 51000 {
		t.Errorf("current_mileage = %d, want 51000", got)
	}
}

func TestDelete_noRemainingRowsLeavesOdometer(t *testing.T) {
	r, db := newRecordRouter(t)
	seedVehicle(t, db, 50000)
	id := postRecord(t, r, `{"categoryId":"c1","performedAt":"`+performedAtStr+`","mileage":52310}`)
	if rec := serveAs(r, http.MethodDelete, "/maintenance-records/"+id, "", identity("owner", "f1")); rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", rec.Code)
	}
	if got := currentMileage(t, db); got != 52310 {
		t.Errorf("current_mileage = %d, want 52310", got)
	}
}

func TestDelete_completionCreatedRecordRetracts(t *testing.T) {
	r, db := newRecordRouter(t)
	seedVehicle(t, db, 50000)
	m, err := NewBuilder().SetVehicleID("v1").SetCategoryID("c1").
		SetPerformedAt(time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC)).SetMileage(52000).Build()
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if _, err := NewAdministrator(db, mileage.NewAdministrator(db)).InsertTx(db, m); err != nil {
		t.Fatalf("InsertTx: %v", err)
	}
	seedMileageRow(t, db, "completion-1", 52000, mileage.SourceMaintenance, m.ID())

	if rec := serveAs(r, http.MethodDelete, "/maintenance-records/"+m.ID(), "", identity("owner", "f1")); rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d: %s", rec.Code, rec.Body.String())
	}
	if rows := ledgerRows(t, db, m.ID(), true); len(rows) != 0 {
		t.Errorf("live rows = %+v, want none", rows)
	}
}

func TestDelete_retractFailureKeepsRecord(t *testing.T) {
	db := newTestDB(t)
	seedVehicle(t, db, 50000)
	id := insertRecord(t, NewAdministrator(db, mileage.NewAdministrator(db)), "v1", "c1", 5, 0)

	a := NewAdministrator(db, failingLedger{retractErr: errors.New("boom")})
	if err := a.SoftDelete(id, "v1"); err == nil {
		t.Fatal("SoftDelete succeeded, want retract error")
	}
	var n int64
	db.Raw(`SELECT COUNT(*) FROM fleet.maintenance_records WHERE id = ? AND deleted_at IS NULL`, id).Scan(&n)
	if n != 1 {
		t.Errorf("live record rows = %d, want 1 (rolled back)", n)
	}
}
