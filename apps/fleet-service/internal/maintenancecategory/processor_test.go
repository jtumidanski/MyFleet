package maintenancecategory

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

const fleetA = "11111111-1111-1111-1111-111111111111"

func newTestProcessor(t *testing.T) (*Processor, *gorm.DB) {
	t.Helper()
	db := newTestDB(t)
	if err := Seed(db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return NewProcessor(logrus.New(), NewProvider(db), NewAdministrator(db)), db
}

// A new name creates a fleet-scoped, non-system row.
func TestCreate_newName(t *testing.T) {
	proc, _ := newTestProcessor(t)

	m, err := proc.Create(fleetA, "Rear Diff Fluid", KindMaintenance)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if m.Name() != "Rear Diff Fluid" {
		t.Fatalf("name = %q", m.Name())
	}
	if m.SystemDefined() {
		t.Fatal("user-created categories must not be system-defined")
	}
	if m.FleetID() == nil || *m.FleetID() != fleetA {
		t.Fatalf("fleetID = %v, want %s", m.FleetID(), fleetA)
	}
	if m.Kind() != KindMaintenance {
		t.Fatalf("kind = %q", m.Kind())
	}
}

// Creating a name that differs only in case from a SYSTEM row returns the
// system row rather than shadowing it.
func TestCreate_dedupesAgainstSystemRowCaseInsensitively(t *testing.T) {
	proc, _ := newTestProcessor(t)

	m, err := proc.Create(fleetA, "oil CHANGE", KindMaintenance)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if m.Name() != "Oil Change" {
		t.Fatalf("expected the seeded row, got %q", m.Name())
	}
	if !m.SystemDefined() {
		t.Fatal("expected the seeded system row")
	}
}

// Creating the same custom name twice returns the same row, not two.
func TestCreate_isIdempotentWithinAFleet(t *testing.T) {
	proc, db := newTestProcessor(t)

	first, err := proc.Create(fleetA, "Rear Diff Fluid", KindMaintenance)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := proc.Create(fleetA, "  rear diff fluid  ", KindMaintenance)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if first.ID() != second.ID() {
		t.Fatalf("expected the same row, got %s and %s", first.ID(), second.ID())
	}

	var count int64
	db.Model(&Entity{}).Where("name = ?", "Rear Diff Fluid").Count(&count)
	if count != 1 {
		t.Fatalf("expected exactly one row, got %d", count)
	}
}

// The same name in a DIFFERENT fleet is a genuinely different row.
func TestCreate_sameNameInAnotherFleetIsDistinct(t *testing.T) {
	proc, _ := newTestProcessor(t)
	const fleetB = "22222222-2222-2222-2222-222222222222"

	a, err := proc.Create(fleetA, "Rear Diff Fluid", KindMaintenance)
	if err != nil {
		t.Fatalf("fleet A: %v", err)
	}
	b, err := proc.Create(fleetB, "Rear Diff Fluid", KindMaintenance)
	if err != nil {
		t.Fatalf("fleet B: %v", err)
	}
	if a.ID() == b.ID() {
		t.Fatal("each fleet must get its own row")
	}
}

// The same name under a different KIND is a different row: "Exhaust" is a
// modification, but a fleet may also want it as a maintenance item.
func TestCreate_sameNameDifferentKindIsDistinct(t *testing.T) {
	proc, _ := newTestProcessor(t)

	a, err := proc.Create(fleetA, "Skid Plate", KindMaintenance)
	if err != nil {
		t.Fatalf("maintenance: %v", err)
	}
	b, err := proc.Create(fleetA, "Skid Plate", KindModification)
	if err != nil {
		t.Fatalf("modification: %v", err)
	}
	if a.ID() == b.ID() {
		t.Fatal("kind must discriminate")
	}
}

func TestCreate_rejectsBlankName(t *testing.T) {
	proc, _ := newTestProcessor(t)

	if _, err := proc.Create(fleetA, "   ", KindMaintenance); !errors.Is(err, server.ErrValidation) {
		t.Fatalf("blank name must be a validation error, got %v", err)
	}
}

func TestCreate_rejectsOverlongName(t *testing.T) {
	proc, _ := newTestProcessor(t)

	long := make([]byte, maxCategoryNameLen+1)
	for i := range long {
		long[i] = 'a'
	}
	if _, err := proc.Create(fleetA, string(long), KindMaintenance); !errors.Is(err, server.ErrValidation) {
		t.Fatalf("overlong name must be a validation error, got %v", err)
	}
}

func TestCreate_rejectsEmptyKind(t *testing.T) {
	proc, _ := newTestProcessor(t)

	if _, err := proc.Create(fleetA, "Rear Diff Fluid", ""); !errors.Is(err, server.ErrValidation) {
		t.Fatalf("empty kind must be a validation error, got %v", err)
	}
}

// An empty fleetID must be rejected by the Processor itself, not only by the
// HTTP layer: FindByName("", ...) silently matches nothing, but an insert
// with an empty FleetID would bind "" into a uuid column and fail at bind
// time on PostgreSQL (SQLSTATE 22P02) rather than at this validation step.
func TestCreate_rejectsEmptyFleetID(t *testing.T) {
	proc, _ := newTestProcessor(t)

	if _, err := proc.Create("", "Rear Diff Fluid", KindMaintenance); !errors.Is(err, server.ErrValidation) {
		t.Fatalf("empty fleetID must be a validation error, got %v", err)
	}
}

// The name limit is stated in characters (60), not bytes: a multibyte name
// well under 60 RUNES but over 60 BYTES must still be accepted.
func TestCreate_acceptsMultibyteNameUnderRuneLimit(t *testing.T) {
	proc, _ := newTestProcessor(t)

	name := strings.Repeat("Ж", 40) // 40 runes, 80 bytes (2 bytes/rune in UTF-8)
	if len(name) <= maxCategoryNameLen {
		t.Fatalf("test setup: name must exceed the BYTE limit to be meaningful, got %d bytes", len(name))
	}

	m, err := proc.Create(fleetA, name, KindMaintenance)
	if err != nil {
		t.Fatalf("expected a multibyte name under the rune limit to be accepted: %v", err)
	}
	if m.Name() != name {
		t.Fatalf("name = %q, want %q", m.Name(), name)
	}
}

// raceAdministrator wraps a real Administrator and, on Insert, first inserts
// an identical (fleet_id, name, kind) row directly — simulating a concurrent
// request that wins the race between this call's FindByName and its Insert.
// The wrapped Insert then collides with that row on the unique index, giving
// Processor.Create a real unique-constraint violation to recover from.
type raceAdministrator struct {
	db   *gorm.DB
	real Administrator
}

func (r *raceAdministrator) Insert(m Model) (Model, error) {
	winner := m
	winner.id = uuid.NewString() // distinct PK; same (fleet_id, name, kind)
	if err := r.db.Create(&Entity{
		ID:      winner.id,
		Name:    winner.name,
		Kind:    string(winner.kind),
		FleetID: winner.fleetID,
	}).Error; err != nil {
		return Model{}, err
	}
	return r.real.Insert(m)
}

// Two requests racing to create the same free-form category must not both
// succeed with distinct rows: the loser's Insert hits the unique index, and
// Processor.Create must recover by returning the winner instead of a 500 or
// a second, near-duplicate row — the exact split-history outcome this
// feature exists to prevent.
func TestCreate_losesRaceReturnsWinner(t *testing.T) {
	db := newTestDB(t)
	if err := Seed(db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	racer := &raceAdministrator{db: db, real: NewAdministrator(db)}
	proc := NewProcessor(logrus.New(), NewProvider(db), racer)

	m, err := proc.Create(fleetA, "Rear Diff Fluid", KindMaintenance)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if m.Name() != "Rear Diff Fluid" {
		t.Fatalf("name = %q", m.Name())
	}

	var count int64
	if err := db.Model(&Entity{}).
		Where("name = ? AND kind = ? AND fleet_id = ?", "Rear Diff Fluid", string(KindMaintenance), fleetA).
		Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("want exactly 1 row after the lost race, got %d — the unique index or the retry did not hold", count)
	}
}
