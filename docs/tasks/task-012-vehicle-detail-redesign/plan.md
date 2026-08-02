# Vehicle Detail Page Redesign — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rebuild `VehicleDetailPage` as a responsive two-column layout with dialog-based forms, a derived stat strip, and a unified records feed — and let users create their own maintenance/modification categories.

**Architecture:** A sticky 320px identity rail carries the vehicle; the fluid right column carries a stat strip, schedule tiles, and one merged records table. Every form moves into a Radix dialog. Maintenance categories become fleet-scoped so free-form entries stay inside the fleet that created them.

**Tech Stack:** Go 1.x + GORM + chi (fleet-service), React 18 + TypeScript + Vite + TanStack Query v5 + react-hook-form + Zod + Tailwind + shadcn/Radix (apps/web). Tests: `go test -race`, Vitest + Testing Library.

## Global Constraints

- Design doc: `docs/tasks/task-012-vehicle-detail-redesign/design.md`. Read it before starting.
- Backend follows the MyFleet backend guidelines: immutable domain models with unexported fields and accessor methods, `entity.go` / `model.go` / `provider.go` / `processor.go` / `resource.go` / `rest.go` split, GORM entities never leaving the provider, JSON:API transport via `server.Resource`.
- Frontend follows the MyFleet frontend guidelines: JSON:API resources typed as `JsonApiResource<A>`, React Query for all server state, react-hook-form + Zod for all forms, Tailwind semantic tokens (never raw hex).
- `server.ParsePage` defaults page size to **25** and hard-caps it at **100** (`packages/shared-go/server/pagination.go:25,29`). Any list request that needs more than 25 rows must pass `page[size]` explicitly.
- Cross-fleet access returns **404, not 403** (`authz.RequireSameFleet`), so fleet existence is never leaked.
- Never use raw hex colors. Status colors use the semantic token families (`--success`, `--warning`, `--danger`, `--info`) and their `-subtle` / `-subtle-foreground` / `-border` trios.
- Full gate before declaring done: `make ci`. Per-task gates are named in each task.
- End every commit message with:
  ```
  Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_016unzREtf7wifmiqyfKkEQD
  ```

## File Structure

**Backend — `apps/fleet-service/internal/maintenancecategory/`** (all modified)
- `entity.go` — add nullable `FleetID`
- `model.go` — add immutable `fleetID` + accessor
- `provider.go` — fleet-scoped `List` / `IDsByKind`, new `Create` / `FindByName`
- `processor.go` — new `Create` with case-insensitive dedupe
- `resource.go` — new `POST /maintenance-categories`, fleet-scoped `GET`
- `rest.go` — `CreateAttributes` for the POST body

**Backend — ripple**
- `apps/fleet-service/internal/maintenancerecord/resource.go:33` — `CategoryAccessor.IDsByKind` gains `fleetID`
- `apps/fleet-service/cmd/main.go:191` — route wiring

**Frontend — new files**
- `apps/web/src/lib/vehicleRecords.ts` — merge/sort/filter/watermark (pure)
- `apps/web/src/lib/vehicleRecords.test.ts`
- `apps/web/src/lib/vehicleStats.ts` — stat derivations (pure)
- `apps/web/src/lib/vehicleStats.test.ts`
- `apps/web/src/lib/hooks/api/vehicleRecords.ts` — composes the three sources
- `apps/web/src/components/ui/dialog.tsx`, `sheet.tsx`, `popover.tsx`, `command.tsx`
- `apps/web/src/components/features/vehicles/CategoryCombobox.tsx` (+ `.test.tsx`)
- `apps/web/src/components/features/vehicles/detail/` — `VehicleIdentityRail.tsx`, `VehicleQuickActions.tsx`, `VehicleStatStrip.tsx`, `UpcomingScheduleStrip.tsx`, `VehicleRecordsTable.tsx` (+ `.test.tsx`), `VehicleRecordDrawer.tsx`, `VehicleTrends.tsx`
- `apps/web/src/components/features/vehicles/dialogs/` — one wrapper per form

**Frontend — deleted at the end (Task 17)**
- `vehicles/mileage/VehicleMileageSection.tsx`, `vehicles/maintenance/VehicleMaintenanceSection.tsx`, `vehicles/fuel/VehicleFuelSection.tsx`, `vehicles/media/VehicleMediaGallery.tsx`

---

## Task 1: Fleet-scope the maintenance category domain

Categories are global today. This adds the tenancy column and threads a `fleetID` through every read path, without yet allowing creation. `NULL` fleet ID means system/global and stays visible to everyone.

**Files:**
- Modify: `apps/fleet-service/internal/maintenancecategory/entity.go`
- Modify: `apps/fleet-service/internal/maintenancecategory/model.go`
- Modify: `apps/fleet-service/internal/maintenancecategory/provider.go`
- Modify: `apps/fleet-service/internal/maintenancecategory/processor.go`
- Modify: `apps/fleet-service/internal/maintenancecategory/resource.go`
- Modify: `apps/fleet-service/internal/maintenancerecord/resource.go:33`
- Modify: `apps/fleet-service/cmd/main.go:191`
- Test: `apps/fleet-service/internal/maintenancecategory/provider_test.go`

**Interfaces:**
- Consumes: nothing (first task)
- Produces:
  - `Entity.FleetID *string` (`gorm:"type:uuid;index"`)
  - `Model.FleetID() *string`
  - `Provider.List(kind Kind, fleetID string, page server.Page) ([]Model, int, error)`
  - `Provider.IDsByKind(kind Kind, fleetID string) ([]string, error)`
  - `Processor.List(kind Kind, fleetID string, page server.Page) ([]Model, int, error)`
  - `Processor.IDsByKind(kind Kind, fleetID string) ([]string, error)`
  - `maintenancerecord.CategoryAccessor.IDsByKind(kind maintenancecategory.Kind, fleetID string) ([]string, error)`

- [ ] **Step 1: Write the failing provider test**

Append to `apps/fleet-service/internal/maintenancecategory/provider_test.go`:

```go
// TestListScopesToFleet proves a category created by fleet A is invisible to
// fleet B, while system rows (NULL fleet_id) stay visible to both.
func TestListScopesToFleet(t *testing.T) {
	db := newTestDB(t)
	if err := Seed(db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	fleetA := "11111111-1111-1111-1111-111111111111"
	custom := Entity{
		ID:    uuid.NewString(),
		Name:  "Rear Diff Fluid",
		Kind:  string(KindMaintenance),
		FleetID: &fleetA,
	}
	if err := db.Create(&custom).Error; err != nil {
		t.Fatalf("create custom: %v", err)
	}

	p := NewProvider(db)
	page := server.Page{Number: 1, Size: 100}

	aRows, _, err := p.List(KindMaintenance, fleetA, page)
	if err != nil {
		t.Fatalf("list fleet A: %v", err)
	}
	if !containsName(aRows, "Rear Diff Fluid") {
		t.Fatal("fleet A must see its own custom category")
	}
	if !containsName(aRows, "Oil Change") {
		t.Fatal("fleet A must still see system categories")
	}

	bRows, _, err := p.List(KindMaintenance, "22222222-2222-2222-2222-222222222222", page)
	if err != nil {
		t.Fatalf("list fleet B: %v", err)
	}
	if containsName(bRows, "Rear Diff Fluid") {
		t.Fatal("fleet B must NOT see fleet A's custom category")
	}
	if !containsName(bRows, "Oil Change") {
		t.Fatal("fleet B must still see system categories")
	}
}

// TestIDsByKindScopesToFleet proves the record ?kind= filter set is bounded to
// the caller's fleet, so one fleet's custom categories cannot widen another's.
func TestIDsByKindScopesToFleet(t *testing.T) {
	db := newTestDB(t)
	if err := Seed(db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	fleetA := "11111111-1111-1111-1111-111111111111"
	if err := db.Create(&Entity{
		ID:      uuid.NewString(),
		Name:    "Rear Diff Fluid",
		Kind:    string(KindMaintenance),
		FleetID: &fleetA,
	}).Error; err != nil {
		t.Fatalf("create custom: %v", err)
	}

	p := NewProvider(db)
	aIDs, err := p.IDsByKind(KindMaintenance, fleetA)
	if err != nil {
		t.Fatalf("ids fleet A: %v", err)
	}
	bIDs, err := p.IDsByKind(KindMaintenance, "22222222-2222-2222-2222-222222222222")
	if err != nil {
		t.Fatalf("ids fleet B: %v", err)
	}
	if len(aIDs) != len(bIDs)+1 {
		t.Fatalf("fleet A should have exactly one more id than fleet B, got %d vs %d",
			len(aIDs), len(bIDs))
	}
}

func containsName(ms []Model, name string) bool {
	for _, m := range ms {
		if m.Name() == name {
			return true
		}
	}
	return false
}
```

Add `"github.com/google/uuid"` and the `server` import to the test file's import block if not already present.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test github.com/jtumidanski/myfleet/apps/fleet-service/internal/maintenancecategory -run 'TestListScopesToFleet|TestIDsByKindScopesToFleet' -v`

Expected: FAIL to compile — `unknown field FleetID` and "too many arguments in call to p.List".

- [ ] **Step 3: Add the column to the entity**

In `entity.go`, add to `Entity` after `SystemDefined`:

```go
	// NULL means a system/global category visible to every fleet. A non-NULL
	// value scopes the row to one fleet, so free-form names entered by one
	// household never appear in another's picker (design §10.1).
	FleetID *string `gorm:"type:uuid;index"`
```

`Seed` is unchanged: it leaves `FleetID` nil, which is exactly the system-row semantic.

- [ ] **Step 4: Add the field to the immutable model**

In `model.go`, add `fleetID *string` to `Model`, an accessor, and wire it in `Make` (in `entity.go`):

```go
// model.go
func (m Model) FleetID() *string { return m.fleetID }
```

```go
// entity.go, inside Make
		fleetID:       e.FleetID,
```

- [ ] **Step 5: Fleet-scope the provider**

In `provider.go`, change the interface and both methods. The scoping predicate is identical in all three query builders:

```go
type Provider interface {
	// List returns a page of categories visible to fleetID: system rows plus
	// that fleet's own. An empty kind means no filter.
	List(kind Kind, fleetID string, page server.Page) ([]Model, int, error)
	// IDsByKind returns every visible category ID of a kind. It always returns
	// a non-nil slice, because the record provider reads nil as "no filter"
	// and empty-non-nil as "match nothing" (design D3).
	IDsByKind(kind Kind, fleetID string) ([]string, error)
}

// visibleTo scopes a query to system rows plus one fleet's own.
func visibleTo(q *gorm.DB, fleetID string) *gorm.DB {
	// The empty-fleetID branch is not a shortcut: fleet_id is uuid in
	// PostgreSQL, and binding "" fails at bind time (SQLSTATE 22P02) before any
	// row is evaluated, so the IS NULL disjunct would never rescue it. A caller
	// with no active fleet must still see system categories.
	//
	// No test in this package catches the removal of this branch — the suite
	// runs on SQLite, which does not type-check bind parameters.
	if fleetID == "" {
		return q.Where("fleet_id IS NULL")
	}
	return q.Where("fleet_id IS NULL OR fleet_id = ?", fleetID)
}

func (p *dbProvider) List(kind Kind, fleetID string, page server.Page) ([]Model, int, error) {
	count := visibleTo(p.db.Model(&Entity{}), fleetID)
	find := visibleTo(p.db.Model(&Entity{}), fleetID)
	if kind != "" {
		count = count.Where("kind = ?", string(kind))
		find = find.Where("kind = ?", string(kind))
	}

	var total int64
	if err := count.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var es []Entity
	if err := find.Order("name asc").Offset(page.Offset()).Limit(page.Size).
		Find(&es).Error; err != nil {
		return nil, 0, err
	}
	out := make([]Model, 0, len(es))
	for _, e := range es {
		out = append(out, Make(e))
	}
	return out, int(total), nil
}

func (p *dbProvider) IDsByKind(kind Kind, fleetID string) ([]string, error) {
	var ids []string
	if err := visibleTo(p.db.Model(&Entity{}), fleetID).
		Where("kind = ?", string(kind)).
		Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	if ids == nil {
		ids = []string{}
	}
	return ids, nil
}
```

- [ ] **Step 6: Thread fleetID through the processor**

In `processor.go`, both methods gain the parameter and pass it straight down:

```go
// List returns a page of categories visible to the given fleet.
func (pr *Processor) List(kind Kind, fleetID string, page server.Page) ([]Model, int, error) {
	return pr.p.List(kind, fleetID, page)
}

// IDsByKind returns every category ID of a kind visible to the given fleet.
func (pr *Processor) IDsByKind(kind Kind, fleetID string) ([]string, error) {
	return pr.p.IDsByKind(kind, fleetID)
}
```

- [ ] **Step 7: Run the provider tests to verify they pass**

Run: `go test github.com/jtumidanski/myfleet/apps/fleet-service/internal/maintenancecategory -run 'TestListScopesToFleet|TestIDsByKindScopesToFleet' -v`

Expected: PASS (2 tests).

- [ ] **Step 8: Update the GET handler to pass the caller's fleet**

In `resource.go`, the list handler now reads identity. Replace the handler body's `proc.List(kind, page)` call and add the identity lookup above it:

```go
		r.Get("/maintenance-categories", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())

			kind, err := ParseKind(req.URL.Query().Get("kind"))
			if err != nil {
				server.WriteError(w, err)
				return
			}

			page := server.ParsePage(req)
			ms, total, err := proc.List(kind, identity.ActiveFleetID, page)
			if err != nil {
				log.WithError(err).Error("list maintenance categories")
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{
				Data: TransformSlice(ms),
				Meta: page.Meta(total),
			})
		})
```

Add `"github.com/jtumidanski/myfleet/packages/shared-go/auth"` to the imports. Update the package doc comment on `InitializeRoutes`: categories are no longer purely global.

A caller with no active fleet has `ActiveFleetID == ""`, which matches no `fleet_id` and therefore returns system categories only — the correct degradation, not an error.

- [ ] **Step 9: Update the CategoryAccessor interface and its caller**

In `apps/fleet-service/internal/maintenancerecord/resource.go`, change the interface at line 33:

```go
type CategoryAccessor interface {
	IDsByKind(kind maintenancecategory.Kind, fleetID string) ([]string, error)
}
```

In the same file, the record list handler already resolves `identity` and the vehicle `v` before it calls the accessor. Pass the vehicle's fleet, not the identity's — the vehicle is the resource being filtered and `RequireSameFleet` has already proven they match:

```go
			ids, err := categoryAccessor.IDsByKind(kind, v.FleetID())
```

Search the file for every `IDsByKind(` call site and update each the same way.

- [ ] **Step 10: Build to catch remaining call sites**

Run: `go build github.com/jtumidanski/myfleet/...`

Expected: PASS. If `cmd/main.go` errors, the wiring at line 191 needs no change (it passes `categoryProc`, which still satisfies the widened interface) — but any compile error here names a call site Step 9 missed. Fix and re-run.

- [ ] **Step 11: Run the full backend gate**

Run: `go test -race github.com/jtumidanski/myfleet/... && go vet github.com/jtumidanski/myfleet/...`

Expected: PASS.

- [ ] **Step 12: Commit**

```bash
git add apps/fleet-service/internal/maintenancecategory apps/fleet-service/internal/maintenancerecord/resource.go
git commit -m "feat(fleet): fleet-scope maintenance categories

Adds a nullable FleetID to the category entity (NULL = system/global) and
threads the caller's fleet through List and IDsByKind so one fleet's
categories can never appear in another's picker or filter set."
```

---

## Task 2: Create custom categories

Adds the POST route. Creation is idempotent by case-insensitive name within the visible set, so "oil change" resolves to the seeded "Oil Change" instead of creating a near-duplicate.

**Files:**
- Modify: `apps/fleet-service/internal/maintenancecategory/provider.go`
- Modify: `apps/fleet-service/internal/maintenancecategory/processor.go`
- Modify: `apps/fleet-service/internal/maintenancecategory/resource.go`
- Modify: `apps/fleet-service/internal/maintenancecategory/rest.go`
- Test: `apps/fleet-service/internal/maintenancecategory/processor_test.go` (create)

**Interfaces:**
- Consumes: `visibleTo`, `Provider`, `Processor`, `Model.FleetID()` from Task 1
- Produces:
  - `Provider.Create(e Entity) (Model, error)`
  - `Provider.FindByName(fleetID, name string, kind Kind) (Model, bool, error)`
  - `Processor.Create(fleetID, name string, kind Kind) (Model, error)`
  - `CreateAttributes{ Name string; Kind string }`
  - `POST /api/fleet/maintenance-categories` → 201 with a `maintenanceCategories` resource

- [ ] **Step 1: Write the failing processor test**

Create `apps/fleet-service/internal/maintenancecategory/processor_test.go`:

```go
package maintenancecategory

import (
	"errors"
	"testing"

	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

const fleetA = "11111111-1111-1111-1111-111111111111"

func newTestProcessor(t *testing.T) (*Processor, *gorm.DB) {
	t.Helper()
	db := newTestDB(t)
	if err := Seed(db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return NewProcessor(logrus.New(), NewProvider(db)), db
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
```

Add `"gorm.io/gorm"` to the import block.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test github.com/jtumidanski/myfleet/apps/fleet-service/internal/maintenancecategory -run TestCreate -v`

Expected: FAIL to compile — `proc.Create` undefined, `maxCategoryNameLen` undefined.

- [ ] **Step 3: Add provider create and lookup**

In `provider.go`, extend the interface and add both methods:

```go
	// Create inserts a category and returns the stored Model.
	Create(e Entity) (Model, error)
	// FindByName resolves a visible category by case-insensitive name and kind.
	// The bool reports whether one was found.
	FindByName(fleetID, name string, kind Kind) (Model, bool, error)
```

```go
func (p *dbProvider) Create(e Entity) (Model, error) {
	if err := p.db.Create(&e).Error; err != nil {
		return Model{}, err
	}
	return Make(e), nil
}

func (p *dbProvider) FindByName(fleetID, name string, kind Kind) (Model, bool, error) {
	var e Entity
	// LOWER() on both sides rather than ILIKE: ILIKE is PostgreSQL-only and
	// these providers are unit-tested against SQLite.
	err := visibleTo(p.db.Model(&Entity{}), fleetID).
		Where("LOWER(name) = LOWER(?) AND kind = ?", name, string(kind)).
		First(&e).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Model{}, false, nil
	}
	if err != nil {
		return Model{}, false, err
	}
	return Make(e), true, nil
}
```

Add `"errors"` to the imports.

- [ ] **Step 4: Add processor create**

In `processor.go`:

```go
// maxCategoryNameLen bounds a free-form category name. It is a UI-legibility
// limit, not a storage one — the column is unbounded text.
const maxCategoryNameLen = 60

// Create resolves a free-form category name to a category, creating a
// fleet-scoped one only when nothing visible already matches.
//
// Matching is case-insensitive against system rows AND the fleet's own, so a
// user typing "oil change" gets the seeded "Oil Change" instead of a
// near-duplicate that would split their history in two (design §10.1).
func (pr *Processor) Create(fleetID, name string, kind Kind) (Model, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > maxCategoryNameLen {
		return Model{}, server.ErrValidation
	}
	if kind != KindMaintenance && kind != KindModification {
		return Model{}, server.ErrValidation
	}

	existing, found, err := pr.p.FindByName(fleetID, name, kind)
	if err != nil {
		return Model{}, err
	}
	if found {
		return existing, nil
	}

	return pr.p.Create(Entity{
		ID:            uuid.NewString(),
		Name:          name,
		SystemDefined: false,
		Kind:          string(kind),
		FleetID:       &fleetID,
	})
}
```

Add `"strings"` and `"github.com/google/uuid"` to the imports.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test github.com/jtumidanski/myfleet/apps/fleet-service/internal/maintenancecategory -run TestCreate -v`

Expected: PASS (7 tests).

- [ ] **Step 6: Add the request payload type**

In `rest.go`:

```go
// CreateAttributes is the JSON:API attributes payload for creating a
// free-form category. FleetID is deliberately absent: the fleet comes from the
// caller's identity, never the body.
type CreateAttributes struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}
```

- [ ] **Step 7: Add the POST route**

In `resource.go`, inside the returned `func(r chi.Router)`, after the GET:

```go
		// POST /maintenance-categories — create a free-form category scoped to
		// the caller's fleet. Idempotent: an existing case-insensitive match is
		// returned instead of a duplicate.
		r.Post("/maintenance-categories", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			if err := authz.RequireWrite(identity); err != nil {
				server.WriteError(w, err)
				return
			}
			// No active fleet means there is nothing to scope the row to.
			if identity.ActiveFleetID == "" {
				server.WriteError(w, server.ErrValidation)
				return
			}

			var body struct {
				Data struct {
					Attributes CreateAttributes `json:"attributes"`
				} `json:"data"`
			}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				server.WriteError(w, server.ErrValidation)
				return
			}

			kind, err := ParseKind(body.Data.Attributes.Kind)
			if err != nil || kind == "" {
				server.WriteError(w, server.ErrValidation)
				return
			}

			m, err := proc.Create(identity.ActiveFleetID, body.Data.Attributes.Name, kind)
			if err != nil {
				log.WithError(err).Error("create maintenance category")
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusCreated, server.Document{Data: Transform(m)})
		})
```

Add `"encoding/json"` and `"github.com/jtumidanski/myfleet/apps/fleet-service/internal/authz"` to the imports.

Note `ParseKind("")` returns `("", nil)` — "no filter" is valid for the GET query param but not for a create body, which is why the handler rejects an empty kind explicitly.

- [ ] **Step 8: Run the full backend gate**

Run: `go build github.com/jtumidanski/myfleet/... && go test -race github.com/jtumidanski/myfleet/... && go vet github.com/jtumidanski/myfleet/...`

Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add apps/fleet-service/internal/maintenancecategory
git commit -m "feat(fleet): create free-form maintenance categories

POST /maintenance-categories creates a category scoped to the caller's
fleet. Creation is idempotent by case-insensitive name within the visible
set, so 'oil change' resolves to the seeded 'Oil Change'."
```

---

## Task 3: Frontend category create service and hook

**Files:**
- Modify: `apps/web/src/types/models/maintenanceCategory.ts`
- Modify: `apps/web/src/services/api/MaintenanceCategoryService.ts`
- Modify: `apps/web/src/lib/hooks/api/maintenance.ts`

**Interfaces:**
- Consumes: `POST /api/fleet/maintenance-categories` from Task 2
- Produces:
  - `CreateMaintenanceCategoryAttributes { name: string; kind: MaintenanceCategoryKind }`
  - `maintenanceCategoryService.create(attrs)` → `Promise<JsonApiResource<MaintenanceCategoryAttributes>>`
  - `useCreateMaintenanceCategory()` — a mutation returning the created `MaintenanceCategory`

- [ ] **Step 1: Add the create attributes type**

In `apps/web/src/types/models/maintenanceCategory.ts`:

```ts
/** Body for POST /api/fleet/maintenance-categories. Fleet comes from the JWT. */
export interface CreateMaintenanceCategoryAttributes {
  name: string;
  kind: MaintenanceCategoryKind;
}
```

- [ ] **Step 2: Type the service generic and expose create**

In `MaintenanceCategoryService.ts`, widen the `BaseService` generic so the
inherited `create` is typed, and import the new type:

```ts
class MaintenanceCategoryService extends BaseService<
  MaintenanceCategoryAttributes,
  CreateMaintenanceCategoryAttributes
> {
```

`BaseService.create(attributes)` already POSTs to `basePath` with the correct
JSON:API envelope, so no new method body is needed.

- [ ] **Step 3: Add the mutation hook**

In `apps/web/src/lib/hooks/api/maintenance.ts`, next to the other category code:

```ts
/**
 * POST /api/fleet/maintenance-categories — create a free-form category.
 *
 * The server is idempotent on case-insensitive name, so a "create" may return
 * a pre-existing system or fleet row. Callers must use the returned resource's
 * id rather than assuming a new one was minted.
 */
export function useCreateMaintenanceCategory() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (attrs: CreateMaintenanceCategoryAttributes) =>
      maintenanceCategoryService.create(attrs),
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: maintenanceCategoryKeys.all });
    },
  });
}
```

`maintenanceCategoryKeys` is the existing factory in that file (`{ all, lists, list }`); invalidating `.all` covers every kind-filtered list.

- [ ] **Step 4: Verify it compiles and lints**

Run: `npm run -w apps/web build && npm run -w apps/web lint`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/types/models/maintenanceCategory.ts apps/web/src/services/api/MaintenanceCategoryService.ts apps/web/src/lib/hooks/api/maintenance.ts
git commit -m "feat(web): add maintenance category create service and hook"
```

---

## Task 4: CategoryCombobox

A searchable picker that offers to create what you typed. This replaces a Radix `Select`, so it needs `popover` + `cmdk` primitives that the repo does not yet have.

**Files:**
- Create: `apps/web/src/components/ui/popover.tsx`
- Create: `apps/web/src/components/ui/command.tsx`
- Create: `apps/web/src/components/features/vehicles/CategoryCombobox.tsx`
- Test: `apps/web/src/components/features/vehicles/CategoryCombobox.test.tsx`
- Modify: `apps/web/package.json`

**Interfaces:**
- Consumes: `useCreateMaintenanceCategory` (Task 3), `MaintenanceCategory` type
- Produces:
```ts
interface CategoryComboboxProps {
  categories: MaintenanceCategory[];   // already filtered to `kind` by the caller
  kind: MaintenanceCategoryKind;       // kind assigned to anything created here
  value: string;                       // selected category id, '' when unset
  onChange: (categoryId: string) => void;
  disabled?: boolean;
  /** Rendered as the trigger's accessible name. */
  ariaLabel?: string;
}
export function CategoryCombobox(props: CategoryComboboxProps): JSX.Element
```

- [ ] **Step 1: Install the primitives**

Run: `npm install -w apps/web @radix-ui/react-popover@^1.1.0 @radix-ui/react-dialog@^1.1.0 cmdk@^1.0.0`

`@radix-ui/react-dialog` is installed here because `cmdk`'s dialog surface depends on it, and Tasks 10 and 15 need it for `dialog.tsx` and `sheet.tsx`.

- [ ] **Step 2: Add the shadcn primitives**

Create `apps/web/src/components/ui/popover.tsx` and `apps/web/src/components/ui/command.tsx` using the standard shadcn/ui source for each (Popover: `Popover`, `PopoverTrigger`, `PopoverContent`; Command: `Command`, `CommandInput`, `CommandList`, `CommandEmpty`, `CommandGroup`, `CommandItem`). Match the conventions already in `apps/web/src/components/ui/select.tsx`: `'use client'` is not used, `cn` is imported from `../../lib/utils`, and colors come from semantic tokens only.

- [ ] **Step 3: Write the failing test**

Create `apps/web/src/components/features/vehicles/CategoryCombobox.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeAll } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { CategoryCombobox } from './CategoryCombobox';
import type { MaintenanceCategory } from '../../../types/models/maintenanceCategory';

// cmdk measures its list; jsdom implements neither method.
beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn();
});

const categories: MaintenanceCategory[] = [
  {
    id: 'c1',
    type: 'maintenanceCategories',
    attributes: { name: 'Oil Change', systemDefined: true, kind: 'maintenance' },
  },
  {
    id: 'c2',
    type: 'maintenanceCategories',
    attributes: { name: 'Rear Diff Fluid', systemDefined: false, kind: 'maintenance' },
  },
];

function renderCombobox(props: Partial<React.ComponentProps<typeof CategoryCombobox>> = {}) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <CategoryCombobox
        categories={categories}
        kind="maintenance"
        value=""
        onChange={vi.fn()}
        ariaLabel="Category"
        {...props}
      />
    </QueryClientProvider>,
  );
}

describe('CategoryCombobox', () => {
  it('selects an existing category by id', async () => {
    const onChange = vi.fn();
    const user = userEvent.setup();
    renderCombobox({ onChange });

    await user.click(screen.getByRole('combobox', { name: /category/i }));
    await user.click(screen.getByText('Rear Diff Fluid'));

    expect(onChange).toHaveBeenCalledWith('c2');
  });

  it('offers to create a name that does not exist', async () => {
    const user = userEvent.setup();
    renderCombobox();

    await user.click(screen.getByRole('combobox', { name: /category/i }));
    await user.type(screen.getByPlaceholderText(/search/i), 'Skid Plate');

    expect(screen.getByText(/create "Skid Plate"/i)).toBeInTheDocument();
  });

  it('does NOT offer to create a name that already exists, ignoring case', async () => {
    const user = userEvent.setup();
    renderCombobox();

    await user.click(screen.getByRole('combobox', { name: /category/i }));
    await user.type(screen.getByPlaceholderText(/search/i), 'oil CHANGE');

    expect(screen.queryByText(/create "/i)).not.toBeInTheDocument();
    expect(screen.getByText('Oil Change')).toBeInTheDocument();
  });

  it('separates suggested from custom categories', async () => {
    const user = userEvent.setup();
    renderCombobox();

    await user.click(screen.getByRole('combobox', { name: /category/i }));

    expect(screen.getByText(/suggested/i)).toBeInTheDocument();
    expect(screen.getByText(/custom/i)).toBeInTheDocument();
  });
});
```

- [ ] **Step 4: Run the test to verify it fails**

Run: `npm run -w apps/web test -- src/components/features/vehicles/CategoryCombobox.test.tsx`

Expected: FAIL — cannot resolve `./CategoryCombobox`.

- [ ] **Step 5: Implement the combobox**

Create `apps/web/src/components/features/vehicles/CategoryCombobox.tsx`:

```tsx
import { useMemo, useState } from 'react';
import { Check, ChevronsUpDown, Plus, Loader2 } from 'lucide-react';
import { toast } from 'sonner';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import { cn } from '../../../lib/utils';
import { Button } from '../../ui/button';
import { Popover, PopoverContent, PopoverTrigger } from '../../ui/popover';
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '../../ui/command';
import { useCreateMaintenanceCategory } from '../../../lib/hooks/api/maintenance';
import type {
  MaintenanceCategory,
  MaintenanceCategoryKind,
} from '../../../types/models/maintenanceCategory';

interface CategoryComboboxProps {
  /** Already filtered to `kind` by the caller. */
  categories: MaintenanceCategory[];
  /** Kind assigned to any category created from here. */
  kind: MaintenanceCategoryKind;
  /** Selected category id; '' when unset. */
  value: string;
  onChange: (categoryId: string) => void;
  disabled?: boolean;
  ariaLabel?: string;
}

/**
 * Category picker with free-form entry. The seeded list is not comprehensive,
 * so anything the user types that does not already exist can be created inline
 * as a fleet-scoped category (design §10).
 *
 * The create affordance is suppressed for a case-insensitive match against the
 * visible list. The server dedupes too, but showing "Create 'oil change'" next
 * to an existing "Oil Change" would be a UI lie regardless of what the server
 * does with it.
 */
export function CategoryCombobox({
  categories,
  kind,
  value,
  onChange,
  disabled,
  ariaLabel = 'Category',
}: CategoryComboboxProps) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState('');
  const createCategory = useCreateMaintenanceCategory();

  const selected = categories.find((c) => c.id === value);

  const suggested = useMemo(
    () => categories.filter((c) => c.attributes.systemDefined),
    [categories],
  );
  const custom = useMemo(
    () => categories.filter((c) => !c.attributes.systemDefined),
    [categories],
  );

  const trimmed = query.trim();
  const exists = categories.some(
    (c) => c.attributes.name.toLowerCase() === trimmed.toLowerCase(),
  );
  const canCreate = trimmed.length > 0 && trimmed.length <= 60 && !exists;

  const handleSelect = (categoryId: string) => {
    onChange(categoryId);
    setOpen(false);
    setQuery('');
  };

  const handleCreate = async () => {
    try {
      const created = await createCategory.mutateAsync({ name: trimmed, kind });
      // The server is idempotent on case-insensitive name, so this id may be
      // an existing row's. Always use what came back.
      handleSelect(created.id);
    } catch (err) {
      const apiError = createErrorFromUnknown(err);
      toast.error(apiError.message || 'Could not create category');
    }
  };

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="outline"
          role="combobox"
          aria-expanded={open}
          aria-label={ariaLabel}
          disabled={disabled}
          className="w-full justify-between font-normal"
        >
          {selected ? selected.attributes.name : 'Select a category'}
          <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" aria-hidden="true" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-[--radix-popover-trigger-width] p-0" align="start">
        <Command>
          <CommandInput
            placeholder="Search or type a new category…"
            value={query}
            onValueChange={setQuery}
          />
          <CommandList>
            {!canCreate && <CommandEmpty>No category found.</CommandEmpty>}

            {suggested.length > 0 && (
              <CommandGroup heading="Suggested">
                {suggested.map((c) => (
                  <CommandItem key={c.id} value={c.attributes.name} onSelect={() => handleSelect(c.id)}>
                    <Check
                      className={cn('mr-2 h-4 w-4', c.id === value ? 'opacity-100' : 'opacity-0')}
                      aria-hidden="true"
                    />
                    {c.attributes.name}
                  </CommandItem>
                ))}
              </CommandGroup>
            )}

            {custom.length > 0 && (
              <CommandGroup heading="Custom">
                {custom.map((c) => (
                  <CommandItem key={c.id} value={c.attributes.name} onSelect={() => handleSelect(c.id)}>
                    <Check
                      className={cn('mr-2 h-4 w-4', c.id === value ? 'opacity-100' : 'opacity-0')}
                      aria-hidden="true"
                    />
                    {c.attributes.name}
                  </CommandItem>
                ))}
              </CommandGroup>
            )}

            {canCreate && (
              <CommandGroup>
                <CommandItem
                  value={`__create__${trimmed}`}
                  onSelect={() => void handleCreate()}
                  disabled={createCategory.isPending}
                >
                  {createCategory.isPending ? (
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" aria-hidden="true" />
                  ) : (
                    <Plus className="mr-2 h-4 w-4" aria-hidden="true" />
                  )}
                  Create &quot;{trimmed}&quot;
                </CommandItem>
              </CommandGroup>
            )}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `npm run -w apps/web test -- src/components/features/vehicles/CategoryCombobox.test.tsx`

Expected: PASS (4 tests). If cmdk's filtering hides items the test looks for, pass `shouldFilter={false}` to `Command` and filter `suggested`/`custom` by `query` yourself — cmdk's built-in scoring is not part of this component's contract.

- [ ] **Step 7: Commit**

```bash
git add apps/web/package.json package-lock.json apps/web/src/components/ui/popover.tsx apps/web/src/components/ui/command.tsx apps/web/src/components/features/vehicles/CategoryCombobox.tsx apps/web/src/components/features/vehicles/CategoryCombobox.test.tsx
git commit -m "feat(web): add CategoryCombobox with inline category creation"
```

---

## Task 5: Wire the combobox into the maintenance forms

**Files:**
- Modify: `apps/web/src/components/features/vehicles/maintenance/MaintenanceRecordForm.tsx`
- Modify: `apps/web/src/components/features/vehicles/maintenance/MaintenanceScheduleForm.tsx`
- Modify: `apps/web/src/components/features/vehicles/maintenance/MaintenanceRecordForm.test.tsx`

**Interfaces:**
- Consumes: `CategoryCombobox` (Task 4)
- Produces: no prop changes. Both forms keep their existing props, Zod schemas, and submit payloads — `categoryId` remains a required string.

- [ ] **Step 1: Replace the Select in MaintenanceRecordForm**

Read the current category field. It renders a Radix `Select` inside a react-hook-form `FormField`. Replace only the control, keeping `FormField` / `FormItem` / `FormLabel` / `FormMessage` intact:

```tsx
<FormField
  control={form.control}
  name="categoryId"
  render={({ field }) => (
    <FormItem>
      <FormLabel>Category</FormLabel>
      <CategoryCombobox
        categories={visibleCategories}
        kind={kind}
        value={field.value}
        onChange={field.onChange}
        ariaLabel="Category"
      />
      <FormMessage />
    </FormItem>
  )}
/>
```

`visibleCategories` is whatever the file already computes to filter `categories` by `kind` — keep that logic exactly as it is. Remove the now-unused `Select` imports.

- [ ] **Step 2: Replace the Select in MaintenanceScheduleForm**

Same substitution. This form is always maintenance-kind (`maintenance_schedules` is maintenance-only per PRD §2 non-goals), so pass `kind="maintenance"` and keep the existing maintenance-only category filtering.

- [ ] **Step 3: Update the form test**

In `MaintenanceRecordForm.test.tsx`:

1. Delete the jsdom polyfill block for `hasPointerCapture` / `releasePointerCapture` (comment at lines 19-20) — it exists only for Radix `Select`.
2. Add `Element.prototype.scrollIntoView = vi.fn();` in a `beforeAll`, which cmdk needs.
3. Rewrite the kind-filtering assertion to drive the new picker. The behavior under test is unchanged: a `modification` form must not offer maintenance categories.

```tsx
  it('offers only the categories of the requested kind', async () => {
    const user = userEvent.setup();
    render(
      <MaintenanceRecordForm categories={categories} kind="modification" onSubmit={vi.fn()} />,
    );

    await user.click(screen.getByRole('combobox', { name: /category/i }));

    // The modification category is offered; the maintenance one is not.
    expect(screen.getByText('Suspension')).toBeInTheDocument();
    expect(screen.queryByText('Oil Change')).not.toBeInTheDocument();
  });
```

Adjust the two category names to whatever the file's existing `categories` fixture defines — read it, do not assume. If the fixture has only one kind, add a second entry so the assertion is meaningful.

The form now renders a React Query mutation hook, so wrap every `render` in this file with a `QueryClientProvider` exactly as `CategoryCombobox.test.tsx` does.

- [ ] **Step 4: Run the form tests to verify they pass**

Run: `npm run -w apps/web test -- src/components/features/vehicles/maintenance/MaintenanceRecordForm.test.tsx`

Expected: PASS. Every previously passing assertion in this file must still pass — if a submit-payload or attachment test breaks, the substitution changed something it should not have.

- [ ] **Step 5: Run the whole frontend suite**

Run: `npm run -w apps/web test`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add apps/web/src/components/features/vehicles/maintenance
git commit -m "feat(web): use CategoryCombobox in the maintenance forms

The seeded 20 categories are not comprehensive; both forms now accept
free-form entry. Props, schemas and submit payloads are unchanged."
```

---

## Task 6: Honest pagination in the list hooks

None of the three list hooks passes `page[size]`, so each silently shows the newest 25 rows as if complete. The merged feed cannot be built on that.

**Files:**
- Modify: `apps/web/src/services/api/MaintenanceRecordService.ts`
- Modify: `apps/web/src/services/api/FuelService.ts`
- Modify: `apps/web/src/lib/hooks/api/mileage.ts`
- Modify: `apps/web/src/lib/hooks/api/fuel.ts`
- Modify: `apps/web/src/lib/hooks/api/maintenance.ts`

**Interfaces:**
- Consumes: nothing
- Produces: all three become `useInfiniteQuery`. Each accumulates pages and exposes a flattened array plus the server's total:
```ts
// apps/web/src/lib/hooks/api/pageSize.ts
export const RECORD_PAGE_SIZE = 100;   // server.ParsePage hard-caps page[size] at 100

// Each hook's `data` (after select):
interface PagedRows<A> { rows: Array<JsonApiResource<A>>; total: number }

useMileageRecords({ vehicleId, from?, to? })   // → PagedRows<MileageRecordAttributes>
useFuelLogs(vehicleId)                          // → PagedRows<FuelLogAttributes>
useMaintenanceRecords(vehicleId, kind?)         // → PagedRows<MaintenanceRecordAttributes>
```
Each also returns React Query's `hasNextPage`, `fetchNextPage`, and `isFetchingNextPage`.

`RECORD_PAGE_SIZE` lives in its own module so all three hooks and Task 8 share one constant rather than each redeclaring it.

- [ ] **Step 1: Add page params to the services**

`MaintenanceRecordService.listByVehicle` gains `page` and `pageSize`:

```ts
  /** GET /api/fleet/vehicles/{vehicleId}/maintenance-records[?kind=…] */
  listByVehicle(
    vehicleId: string,
    kind?: MaintenanceCategoryKind,
    params?: { page?: number; pageSize?: number },
  ) {
    const search = new URLSearchParams();
    if (kind) search.set('kind', kind);
    if (params?.page != null) search.set('page[number]', String(params.page));
    if (params?.pageSize != null) search.set('page[size]', String(params.pageSize));
    const qs = search.toString();
    const path = `/api/fleet/vehicles/${vehicleId}/maintenance-records`;
    return this.listAt(qs ? `${path}?${qs}` : path);
  }
```

Apply the same shape to `FuelService.listByVehicle`. `MileageService.listByVehicle` already accepts `page` and `pageSize` — leave it.

- [ ] **Step 2: Add the shared page-size constant**

Create `apps/web/src/lib/hooks/api/pageSize.ts`:

```ts
/**
 * server.ParsePage defaults page[size] to 25 and hard-caps it at 100
 * (packages/shared-go/server/pagination.go:25,29). Requesting more is silently
 * clamped, so 100 is the largest page a client can actually get.
 */
export const RECORD_PAGE_SIZE = 100;
```

- [ ] **Step 3: Convert the three hooks to useInfiniteQuery**

Each hook currently ends with `select: (result) => result.data`, which throws away `meta` — the exact field needed to know whether more rows exist. Each becomes an infinite query that accumulates pages. For `useFuelLogs`:

```ts
/**
 * GET /api/fleet/vehicles/{vehicleId}/fuel-logs — list fuel logs (newest first).
 *
 * Infinite rather than single-page: the unified records feed merges this with
 * two other independently-paginated sources, and a merge over sources that
 * REPLACE their rows on page advance would drop the newest rows from view.
 * Pages accumulate; `rows` is every page fetched so far.
 */
export function useFuelLogs(vehicleId: string | null | undefined) {
  return useInfiniteQuery({
    queryKey: fuelKeys.list({ vehicleId: vehicleId ?? '' }),
    queryFn: ({ pageParam }) =>
      fuelService.listByVehicle(vehicleId as string, {
        page: pageParam,
        pageSize: RECORD_PAGE_SIZE,
      }),
    initialPageParam: 1,
    getNextPageParam: (lastPage, allPages) =>
      allPages.length < (lastPage.meta?.totalPages ?? 1) ? allPages.length + 1 : undefined,
    enabled: !!vehicleId,
    staleTime: 60 * 1000,
    gcTime: 5 * 60 * 1000,
    select: (data) => ({
      rows: data.pages.flatMap((p) => p.data),
      total: data.pages[0]?.meta?.total ?? 0,
    }),
  });
}
```

Import `useInfiniteQuery` from `@tanstack/react-query` and `RECORD_PAGE_SIZE` from `./pageSize`. Apply the same shape to `useMileageRecords` and `useMaintenanceRecords`, keeping each one's existing query-key parameters (`from`/`to` for mileage, `kind` for maintenance records) so the caches stay correctly partitioned.

`getNextPageParam` returning `undefined` is what sets `hasNextPage` to false — that is the signal Task 8 reads to know a source is exhausted.

- [ ] **Step 4: Fix every consumer**

The return shape changes from `A[]` to `{ rows, total }`. Find all call sites:

Run: `grep -rn "useFuelLogs\|useMileageRecords\|useMaintenanceRecords" apps/web/src --include=*.tsx --include=*.ts`

At each site, `const { data: records } = useMileageRecords(...)` becomes `const { data } = useMileageRecords(...)` with `const records = data?.rows ?? []`. `getLatestMileage(records)` takes an array, so it now receives `data?.rows ?? []`.

- [ ] **Step 5: Verify accumulation with a hook test**

Add to `apps/web/src/lib/hooks/api/mileage.test.ts` (the file already exists):

```ts
it('accumulates pages rather than replacing them', async () => {
  // Two pages of one row each, meta reporting two total pages.
  const listByVehicle = vi
    .spyOn(mileageService, 'listByVehicle')
    .mockResolvedValueOnce({
      data: [mileageResource('r1', '2026-06-01T00:00:00Z', 84000)],
      meta: { total: 2, totalPages: 2, number: 1, size: 100 },
    })
    .mockResolvedValueOnce({
      data: [mileageResource('r2', '2026-01-01T00:00:00Z', 80000)],
      meta: { total: 2, totalPages: 2, number: 2, size: 100 },
    });

  const { result } = renderHook(() => useMileageRecords({ vehicleId: 'v1' }), { wrapper });

  await waitFor(() => expect(result.current.data?.rows).toHaveLength(1));
  expect(result.current.hasNextPage).toBe(true);

  await act(() => result.current.fetchNextPage().then(() => undefined));

  // Page 2 ADDS to page 1 — the newest row must not disappear.
  await waitFor(() => expect(result.current.data?.rows).toHaveLength(2));
  expect(result.current.data?.rows.map((r) => r.id)).toEqual(['r1', 'r2']);
  expect(result.current.hasNextPage).toBe(false);
  expect(listByVehicle).toHaveBeenCalledTimes(2);
});
```

Match the existing file's fixture helpers and `wrapper` — read it first and reuse what it already defines rather than introducing a parallel setup. If it has no `mileageResource` helper, write one that builds a `JsonApiResource<MileageRecordAttributes>`.

Run: `npm run -w apps/web test -- src/lib/hooks/api/mileage.test.ts`

Expected: PASS, including every test already in the file.

- [ ] **Step 6: Verify the build and the suite**

Run: `npm run -w apps/web build && npm run -w apps/web test`

Expected: PASS. TypeScript will name any consumer missed in Step 4.

- [ ] **Step 7: Commit**

```bash
git add apps/web/src
git commit -m "fix(web): request explicit page sizes for vehicle list hooks

server.ParsePage defaults page[size] to 25 and no hook overrode it, so the
vehicle page showed the newest 25 mileage records, 25 maintenance records
and 25 fuel logs while presenting each as a complete list. The hooks now
request 100 and surface meta so callers can see what is truncated."
```

---

## Task 7: The record merge (pure function)

The heart of the feed and the subtlest logic in this plan. Kept free of React and the network so it can be tested directly.

**Files:**
- Create: `apps/web/src/lib/vehicleRecords.ts`
- Test: `apps/web/src/lib/vehicleRecords.test.ts`

**Interfaces:**
- Consumes: nothing
- Produces:
```ts
export type VehicleRecordKind = 'maintenance' | 'modification' | 'fuel' | 'mileage';
export interface VehicleRecordRow {
  id: string;        // `${kind}:${sourceId}` — unique across sources
  sourceId: string;
  kind: VehicleRecordKind;
  date: string;      // ISO 8601; the sort key
  title: string;
  mileage?: number;
  cost?: number;
}
export interface RecordSource { rows: VehicleRecordRow[]; hasMore: boolean }
export interface MergeResult { rows: VehicleRecordRow[]; withheldCount: number }
export function mergeVehicleRecords(sources: RecordSource[]): MergeResult;
export function filterVehicleRecords(rows: VehicleRecordRow[], kind: VehicleRecordKind | 'all'): VehicleRecordRow[];
```

- [ ] **Step 1: Write the failing test**

Create `apps/web/src/lib/vehicleRecords.test.ts`:

```ts
import { describe, it, expect } from 'vitest';
import {
  mergeVehicleRecords,
  filterVehicleRecords,
  type RecordSource,
  type VehicleRecordRow,
} from './vehicleRecords';

function row(kind: VehicleRecordRow['kind'], date: string): VehicleRecordRow {
  return { id: `${kind}:${date}`, sourceId: date, kind, date, title: `${kind} ${date}` };
}

describe('mergeVehicleRecords', () => {
  it('returns an empty result for no sources', () => {
    expect(mergeVehicleRecords([])).toEqual({ rows: [], withheldCount: 0 });
  });

  it('sorts newest first across sources when everything is loaded', () => {
    const sources: RecordSource[] = [
      { rows: [row('fuel', '2026-03-01'), row('fuel', '2026-01-01')], hasMore: false },
      { rows: [row('mileage', '2026-02-01')], hasMore: false },
    ];

    const { rows, withheldCount } = mergeVehicleRecords(sources);

    expect(rows.map((r) => r.date)).toEqual(['2026-03-01', '2026-02-01', '2026-01-01']);
    expect(withheldCount).toBe(0);
  });

  // The watermark: a source with unloaded pages may still hold rows older than
  // its oldest loaded one, so nothing below that date can be safely ordered.
  it('withholds rows older than an incomplete source oldest loaded row', () => {
    const sources: RecordSource[] = [
      // Incomplete, oldest loaded = 2026-02-01. Rows below that may interleave.
      { rows: [row('fuel', '2026-03-01'), row('fuel', '2026-02-01')], hasMore: true },
      { rows: [row('mileage', '2026-04-01'), row('mileage', '2026-01-01')], hasMore: false },
    ];

    const { rows, withheldCount } = mergeVehicleRecords(sources);

    expect(rows.map((r) => r.date)).toEqual(['2026-04-01', '2026-03-01', '2026-02-01']);
    expect(withheldCount).toBe(1); // mileage 2026-01-01 is below the watermark
  });

  // The MOST constraining incomplete source wins: the shallowest coverage.
  it('uses the newest oldest-loaded date among incomplete sources', () => {
    const sources: RecordSource[] = [
      { rows: [row('fuel', '2026-05-01')], hasMore: true },              // oldest = 05-01
      { rows: [row('mileage', '2026-06-01'), row('mileage', '2026-01-01')], hasMore: true }, // oldest = 01-01
    ];

    const { rows } = mergeVehicleRecords(sources);

    // Watermark is 2026-05-01, so mileage 2026-01-01 is withheld.
    expect(rows.map((r) => r.date)).toEqual(['2026-06-01', '2026-05-01']);
  });

  it('withholds everything when an incomplete source has loaded nothing', () => {
    const sources: RecordSource[] = [
      { rows: [], hasMore: true },
      { rows: [row('mileage', '2026-06-01')], hasMore: false },
    ];

    const { rows, withheldCount } = mergeVehicleRecords(sources);

    expect(rows).toEqual([]);
    expect(withheldCount).toBe(1);
  });

  it('ignores empty sources that have nothing more to load', () => {
    const sources: RecordSource[] = [
      { rows: [], hasMore: false },
      { rows: [row('mileage', '2026-06-01')], hasMore: false },
    ];

    expect(mergeVehicleRecords(sources).rows.map((r) => r.date)).toEqual(['2026-06-01']);
  });

  it('breaks date ties deterministically by id', () => {
    const sources: RecordSource[] = [
      { rows: [row('fuel', '2026-03-01')], hasMore: false },
      { rows: [row('mileage', '2026-03-01')], hasMore: false },
    ];

    const first = mergeVehicleRecords(sources).rows.map((r) => r.id);
    const second = mergeVehicleRecords([...sources].reverse()).rows.map((r) => r.id);

    expect(first).toEqual(second);
  });
});

describe('filterVehicleRecords', () => {
  const rows = [row('fuel', '2026-03-01'), row('mileage', '2026-02-01'), row('modification', '2026-01-01')];

  it('returns everything for "all"', () => {
    expect(filterVehicleRecords(rows, 'all')).toHaveLength(3);
  });

  it('narrows to one kind', () => {
    expect(filterVehicleRecords(rows, 'fuel').map((r) => r.kind)).toEqual(['fuel']);
  });

  it('keeps maintenance and modification distinct', () => {
    expect(filterVehicleRecords(rows, 'maintenance')).toHaveLength(0);
    expect(filterVehicleRecords(rows, 'modification')).toHaveLength(1);
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `npm run -w apps/web test -- src/lib/vehicleRecords.test.ts`

Expected: FAIL — cannot resolve `./vehicleRecords`.

- [ ] **Step 3: Implement the merge**

Create `apps/web/src/lib/vehicleRecords.ts`:

```ts
/** The record types that share the unified vehicle feed. */
export type VehicleRecordKind = 'maintenance' | 'modification' | 'fuel' | 'mileage';

/** One row in the unified feed, normalized from any of the four sources. */
export interface VehicleRecordRow {
  /** `${kind}:${sourceId}` — unique across sources, which may reuse ids. */
  id: string;
  sourceId: string;
  kind: VehicleRecordKind;
  /** ISO 8601. The sort key. */
  date: string;
  title: string;
  mileage?: number;
  cost?: number;
}

/** One paginated source's currently-loaded rows. */
export interface RecordSource {
  /** Loaded rows, any order. */
  rows: VehicleRecordRow[];
  /** True when the source has pages that have not been fetched. */
  hasMore: boolean;
}

export interface MergeResult {
  /** Safe rows, newest first. */
  rows: VehicleRecordRow[];
  /** Loaded rows suppressed because they fall below the watermark. */
  withheldCount: number;
}

/**
 * Merges independently-paginated sources into one newest-first feed.
 *
 * Each source is paginated on its own, so simply concatenating loaded pages
 * produces an order that is wrong past the point where the shallowest source
 * runs out of loaded rows: that source may still hold unfetched rows that
 * belong between the ones already shown.
 *
 * The watermark is the NEWEST "oldest loaded row" among sources that still
 * have unloaded pages — the most constraining source. Rows at or above it are
 * provably ordered; rows below it are withheld until more pages load.
 */
export function mergeVehicleRecords(sources: RecordSource[]): MergeResult {
  const all = sources.flatMap((s) => s.rows);
  const sorted = [...all].sort(compareRows);

  const incomplete = sources.filter((s) => s.hasMore);

  // A source that has more to load but has loaded nothing tells us nothing
  // about where its rows belong, so no row can be trusted yet.
  if (incomplete.some((s) => s.rows.length === 0)) {
    return { rows: [], withheldCount: sorted.length };
  }

  const watermarks = incomplete.map((s) =>
    s.rows.reduce((oldest, r) => (r.date < oldest ? r.date : oldest), s.rows[0].date),
  );

  if (watermarks.length === 0) {
    return { rows: sorted, withheldCount: 0 };
  }

  const safeUntil = watermarks.reduce((newest, d) => (d > newest ? d : newest));
  const rows = sorted.filter((r) => r.date >= safeUntil);
  return { rows, withheldCount: sorted.length - rows.length };
}

/** Newest first, with a stable id tiebreak so equal dates never reorder. */
function compareRows(a: VehicleRecordRow, b: VehicleRecordRow): number {
  if (a.date !== b.date) return a.date < b.date ? 1 : -1;
  return a.id < b.id ? -1 : a.id > b.id ? 1 : 0;
}

/** Narrows the feed to one kind. 'all' passes everything through. */
export function filterVehicleRecords(
  rows: VehicleRecordRow[],
  kind: VehicleRecordKind | 'all',
): VehicleRecordRow[] {
  return kind === 'all' ? rows : rows.filter((r) => r.kind === kind);
}
```

ISO 8601 strings compare correctly with `<` and `>` as long as they share an offset; the API returns UTC (`Z`) throughout, so string comparison is used deliberately instead of constructing a `Date` per comparison.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `npm run -w apps/web test -- src/lib/vehicleRecords.test.ts`

Expected: PASS (10 tests).

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/lib/vehicleRecords.ts apps/web/src/lib/vehicleRecords.test.ts
git commit -m "feat(web): add the unified vehicle record merge

Merges independently-paginated maintenance, fuel and mileage sources into
one newest-first feed, withholding rows below the watermark where ordering
cannot yet be proven."
```

---

## Task 8: useVehicleRecords

Adapts the three API shapes into `VehicleRecordRow`s and drives page loading.

**Files:**
- Create: `apps/web/src/lib/hooks/api/vehicleRecords.ts`

**Interfaces:**
- Consumes: `mergeVehicleRecords` (Task 7); the infinite hooks from Task 6
- Produces:
```ts
export function useVehicleRecords(vehicleId: string, categories: MaintenanceCategory[]): {
  rows: VehicleRecordRow[];
  withheldCount: number;
  total: number;          // sum of per-source totals
  isLoading: boolean;
  hasMore: boolean;
  loadMore: () => void;
}
```

- [ ] **Step 1: Implement the hook**

Create `apps/web/src/lib/hooks/api/vehicleRecords.ts`:

```ts
import { useMemo } from 'react';
import { useMaintenanceRecords } from './maintenance';
import { useFuelLogs } from './fuel';
import { useMileageRecords } from './mileage';
import { mergeVehicleRecords, type RecordSource, type VehicleRecordRow } from '../../vehicleRecords';
import type { MaintenanceCategory } from '../../../types/models/maintenanceCategory';

/**
 * Composes the three paginated record sources into one feed.
 *
 * Each source paginates independently. "Load more" advances only the sources
 * currently constraining the watermark — advancing an already-exhausted source
 * would do nothing, and advancing a source whose oldest row is far below the
 * watermark just fetches rows that stay withheld (see mergeVehicleRecords).
 */
export function useVehicleRecords(vehicleId: string, categories: MaintenanceCategory[]) {
  const maintenance = useMaintenanceRecords(vehicleId);
  const fuel = useFuelLogs(vehicleId);
  const mileage = useMileageRecords({ vehicleId });

  const categoryById = useMemo(
    () => new Map(categories.map((c) => [c.id, c])),
    [categories],
  );

  const sources = useMemo<RecordSource[]>(() => {
    const maintenanceRows: VehicleRecordRow[] = (maintenance.data?.rows ?? []).map((r) => {
      const category = categoryById.get(r.attributes.categoryId);
      return {
        id: `maintenance:${r.id}`,
        sourceId: r.id,
        // The category owns kind — a record stores none (design D1).
        kind: category?.attributes.kind === 'modification' ? 'modification' : 'maintenance',
        date: r.attributes.performedAt,
        title:
          r.attributes.description || category?.attributes.name || r.attributes.categoryId,
        mileage: r.attributes.mileage,
        cost: r.attributes.cost,
      };
    });

    const fuelRows: VehicleRecordRow[] = (fuel.data?.rows ?? []).map((l) => ({
      id: `fuel:${l.id}`,
      sourceId: l.id,
      kind: 'fuel',
      date: l.attributes.date,
      title: `${l.attributes.gallons.toFixed(3)} gal @ $${l.attributes.pricePerGallon.toFixed(3)}`,
      mileage: l.attributes.mileage,
      // Task 9's deriveAvgEconomy needs gallons; nothing else reads it.
      gallons: l.attributes.gallons,
      cost: l.attributes.totalCost,
    }));

    const mileageRows: VehicleRecordRow[] = (mileage.data?.rows ?? []).map((m) => ({
      id: `mileage:${m.id}`,
      sourceId: m.id,
      kind: 'mileage',
      date: m.attributes.recordedAt,
      title: 'Odometer reading',
      mileage: m.attributes.mileage,
    }));

    return [
      { rows: maintenanceRows, hasMore: maintenance.hasNextPage },
      { rows: fuelRows, hasMore: fuel.hasNextPage },
      { rows: mileageRows, hasMore: mileage.hasNextPage },
    ];
  }, [
    maintenance.data, maintenance.hasNextPage,
    fuel.data, fuel.hasNextPage,
    mileage.data, mileage.hasNextPage,
    categoryById,
  ]);

  const { rows, withheldCount } = useMemo(() => mergeVehicleRecords(sources), [sources]);

  const total =
    (maintenance.data?.total ?? 0) + (fuel.data?.total ?? 0) + (mileage.data?.total ?? 0);

  const anyHasMore = sources.some((s) => s.hasMore);

  return {
    rows,
    withheldCount,
    total,
    isLoading: maintenance.isLoading || fuel.isLoading || mileage.isLoading,
    // More to show means either unfetched pages or rows held below the watermark.
    hasMore: anyHasMore || withheldCount > 0,
    loadMore: () => {
      // Fetch the next page of every source that still has one. Pages
      // accumulate, so this widens coverage and lowers the watermark, which is
      // what releases the withheld rows.
      if (maintenance.hasNextPage) void maintenance.fetchNextPage();
      if (fuel.hasNextPage) void fuel.fetchNextPage();
      if (mileage.hasNextPage) void mileage.fetchNextPage();
    },
  };
}
```

- [ ] **Step 2: Verify the build**

Run: `npm run -w apps/web build`

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add apps/web/src/lib/hooks/api/vehicleRecords.ts
git commit -m "feat(web): add useVehicleRecords composing the three record sources"
```

---

## Task 9: Stat derivations (pure functions)

**Files:**
- Create: `apps/web/src/lib/vehicleStats.ts`
- Test: `apps/web/src/lib/vehicleStats.test.ts`

**Interfaces:**
- Consumes: `VehicleRecordRow` (Task 7)
- Produces:
```ts
export function deriveOdometer(mileageRows: VehicleRecordRow[], currentMileage?: number): number | undefined;
export function deriveTrailingCost(rows: VehicleRecordRow[], now: Date): number;
export function deriveAvgEconomy(fuelRows: VehicleRecordRow[]): number | undefined;
export interface NextService { label: string; severity: 'ok' | 'warning' | 'danger' }
export function deriveNextService(schedules: MaintenanceSchedule[], odometer: number | undefined, now: Date): NextService | undefined;
```

- [ ] **Step 1: Write the failing test**

Create `apps/web/src/lib/vehicleStats.test.ts`:

```ts
import { describe, it, expect } from 'vitest';
import { deriveOdometer, deriveTrailingCost, deriveAvgEconomy } from './vehicleStats';
import type { VehicleRecordRow } from './vehicleRecords';

function mileageRow(date: string, mileage: number): VehicleRecordRow {
  return { id: `mileage:${date}`, sourceId: date, kind: 'mileage', date, title: 'Odometer', mileage };
}
function fuelRow(date: string, mileage: number, cost: number, gallons: number): VehicleRecordRow {
  return {
    id: `fuel:${date}`, sourceId: date, kind: 'fuel', date,
    title: `${gallons} gal`, mileage, cost,
  };
}

describe('deriveOdometer', () => {
  it('prefers the newest mileage row', () => {
    expect(deriveOdometer([mileageRow('2026-01-01', 80000), mileageRow('2026-06-01', 84210)], 1))
      .toBe(84210);
  });

  it('falls back to the vehicle current mileage when there are no rows', () => {
    expect(deriveOdometer([], 84210)).toBe(84210);
  });

  it('returns undefined when there is nothing at all', () => {
    expect(deriveOdometer([], undefined)).toBeUndefined();
  });
});

describe('deriveTrailingCost', () => {
  const now = new Date('2026-08-02T00:00:00Z');

  it('sums costs inside the trailing twelve months', () => {
    const rows = [
      fuelRow('2026-07-01T00:00:00Z', 84000, 58.31, 16),
      { ...mileageRow('2026-06-01T00:00:00Z', 83000), cost: undefined },
      { id: 'maintenance:a', sourceId: 'a', kind: 'maintenance' as const,
        date: '2026-05-01T00:00:00Z', title: 'Brakes', cost: 612.4 },
    ];
    expect(deriveTrailingCost(rows, now)).toBeCloseTo(670.71, 2);
  });

  it('excludes rows older than twelve months', () => {
    const rows = [
      { id: 'maintenance:old', sourceId: 'old', kind: 'maintenance' as const,
        date: '2025-01-01T00:00:00Z', title: 'Old', cost: 1000 },
    ];
    expect(deriveTrailingCost(rows, now)).toBe(0);
  });

  it('returns 0 for no rows', () => {
    expect(deriveTrailingCost([], now)).toBe(0);
  });
});

describe('deriveAvgEconomy', () => {
  it('is undefined with fewer than two fill-ups', () => {
    expect(deriveAvgEconomy([])).toBeUndefined();
    expect(deriveAvgEconomy([fuelRow('2026-07-01', 84000, 58.31, 16)])).toBeUndefined();
  });

  it('is undefined when the odometer did not advance', () => {
    const rows = [
      fuelRow('2026-07-15', 84000, 58.31, 16),
      fuelRow('2026-07-01', 84000, 55.02, 16),
    ];
    expect(deriveAvgEconomy(rows)).toBeUndefined();
  });
});
```

`deriveAvgEconomy` needs gallons. Add an optional `gallons?: number` to `VehicleRecordRow` in `vehicleRecords.ts` — Task 8's fuel adapter already populates it from `l.attributes.gallons`, so this only adds the field to the type. Then extend this test with a positive case:

```ts
  it('averages miles per gallon across consecutive fill-ups', () => {
    const rows = [
      { ...fuelRow('2026-07-15T00:00:00Z', 84300, 58.31, 16), gallons: 16 },
      { ...fuelRow('2026-07-01T00:00:00Z', 84000, 55.02, 15), gallons: 15 },
    ];
    // 300 miles on the 16 gallons that filled the tank after the first reading.
    expect(deriveAvgEconomy(rows)).toBeCloseTo(18.75, 2);
  });
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `npm run -w apps/web test -- src/lib/vehicleStats.test.ts`

Expected: FAIL — cannot resolve `./vehicleStats`.

- [ ] **Step 3: Implement the derivations**

Create `apps/web/src/lib/vehicleStats.ts`:

```ts
import type { VehicleRecordRow } from './vehicleRecords';
import type { MaintenanceSchedule } from '../types/models/maintenanceSchedule';

/**
 * Every derivation here returns undefined rather than a placeholder number
 * when its inputs are missing. The stat tiles render an em dash for undefined;
 * a confident-looking 0 would be worse than an obvious gap.
 */

/** Newest odometer reading, falling back to the vehicle's stored mileage. */
export function deriveOdometer(
  mileageRows: VehicleRecordRow[],
  currentMileage?: number,
): number | undefined {
  const newest = mileageRows
    .filter((r) => typeof r.mileage === 'number')
    .reduce<VehicleRecordRow | undefined>(
      (best, r) => (best == null || r.date > best.date ? r : best),
      undefined,
    );
  return newest?.mileage ?? currentMileage;
}

/** Sum of every cost in the trailing twelve months. */
export function deriveTrailingCost(rows: VehicleRecordRow[], now: Date): number {
  const cutoff = new Date(now);
  cutoff.setFullYear(cutoff.getFullYear() - 1);
  const cutoffIso = cutoff.toISOString();

  return rows.reduce((sum, r) => (r.date >= cutoffIso ? sum + (r.cost ?? 0) : sum), 0);
}

/**
 * Average miles per gallon across consecutive fill-ups.
 *
 * A tank's gallons are attributed to the distance travelled SINCE the previous
 * fill-up, so the oldest reading contributes distance but no gallons — which
 * is why two fill-ups are the minimum.
 */
export function deriveAvgEconomy(fuelRows: VehicleRecordRow[]): number | undefined {
  const usable = fuelRows
    .filter((r) => typeof r.mileage === 'number' && typeof r.gallons === 'number')
    .sort((a, b) => (a.date < b.date ? -1 : 1));

  if (usable.length < 2) return undefined;

  let miles = 0;
  let gallons = 0;
  for (let i = 1; i < usable.length; i += 1) {
    const delta = (usable[i].mileage as number) - (usable[i - 1].mileage as number);
    if (delta <= 0) continue; // a non-advancing odometer is bad data, not zero economy
    miles += delta;
    gallons += usable[i].gallons as number;
  }

  if (miles <= 0 || gallons <= 0) return undefined;
  return miles / gallons;
}

export interface NextService {
  label: string;
  severity: 'ok' | 'warning' | 'danger';
}

/**
 * The most urgent schedule, expressed as distance or time remaining.
 * Severity comes from the server-computed schedule severity, which already
 * owns the overdue/upcoming thresholds.
 */
export function deriveNextService(
  schedules: MaintenanceSchedule[],
  odometer: number | undefined,
  now: Date,
): NextService | undefined {
  if (schedules.length === 0) return undefined;

  const ranked = [...schedules].sort((a, b) => rank(a) - rank(b));
  const next = ranked[0];
  const severity = severityOf(next);

  const dueMileage = next.attributes.nextDueMileage;
  if (typeof dueMileage === 'number' && typeof odometer === 'number') {
    const remaining = dueMileage - odometer;
    return {
      label:
        remaining >= 0
          ? `${remaining.toLocaleString()} mi`
          : `${Math.abs(remaining).toLocaleString()} mi over`,
      severity,
    };
  }

  const dueDate = next.attributes.nextDueDate;
  if (dueDate) {
    const days = Math.round((new Date(dueDate).getTime() - now.getTime()) / 86_400_000);
    return {
      label: days >= 0 ? `${days} days` : `${Math.abs(days)} days over`,
      severity,
    };
  }

  return undefined;
}

/**
 * Urgency comes from `status`, NOT `severity`. They are different vocabularies
 * on the same resource: status is 'ok' | 'upcoming' | 'overdue' (how due it is)
 * while severity is 'urgent' | 'recommended' | 'informational' (how much it
 * matters). Only status answers "what is due next".
 */
function severityOf(s: MaintenanceSchedule): NextService['severity'] {
  switch (s.attributes.status) {
    case 'overdue':
      return 'danger';
    case 'upcoming':
      return 'warning';
    default:
      return 'ok';
  }
}

/** Overdue first, then upcoming, then everything else. */
export function rankSchedule(s: MaintenanceSchedule): number {
  switch (s.attributes.status) {
    case 'overdue':
      return 0;
    case 'upcoming':
      return 1;
    default:
      return 2;
  }
}
```

Replace the two `rank(...)` call sites in `deriveNextService` with `rankSchedule(...)`. It is exported because Task 13's schedule strip sorts by the same order and must not duplicate the switch.

The field names above are verified against `apps/web/src/types/models/maintenanceSchedule.ts`: `status`, `severity`, `nextDueMileage`, and `nextDueDate` all exist, and both `nextDue*` fields are optional.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `npm run -w apps/web test -- src/lib/vehicleStats.test.ts`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/lib/vehicleStats.ts apps/web/src/lib/vehicleStats.test.ts apps/web/src/lib/vehicleRecords.ts apps/web/src/lib/hooks/api/vehicleRecords.ts
git commit -m "feat(web): derive vehicle stat-strip metrics

Odometer, trailing 12-month cost, average economy and next service, each
returning undefined rather than a misleading zero when inputs are missing."
```

---

## Task 10: Dialog and sheet primitives

**Files:**
- Create: `apps/web/src/components/ui/dialog.tsx`
- Create: `apps/web/src/components/ui/sheet.tsx`

**Interfaces:**
- Consumes: `@radix-ui/react-dialog` (installed in Task 4)
- Produces: `Dialog`, `DialogTrigger`, `DialogContent`, `DialogHeader`, `DialogTitle`, `DialogDescription`, `DialogFooter`; `Sheet`, `SheetTrigger`, `SheetContent`, `SheetHeader`, `SheetTitle`, `SheetDescription`

- [ ] **Step 1: Add the primitives**

Create both files from the standard shadcn/ui source. `sheet.tsx` is the same Radix Dialog with side-anchored positioning; this project needs `side="right"` only, but keep the standard variant API. Match `select.tsx` conventions: `cn` from `../../lib/utils`, semantic tokens only, no raw hex.

- [ ] **Step 2: Verify the build**

Run: `npm run -w apps/web build && npm run -w apps/web lint`

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add apps/web/src/components/ui/dialog.tsx apps/web/src/components/ui/sheet.tsx
git commit -m "feat(web): add dialog and sheet primitives"
```

---

## Task 11: Dialog wrappers for the existing forms

Each wrapper owns one mutation and closes on success. The forms themselves are not modified.

**Files:**
- Create: `apps/web/src/components/features/vehicles/dialogs/EditVehicleDialog.tsx`
- Create: `apps/web/src/components/features/vehicles/dialogs/LogMileageDialog.tsx`
- Create: `apps/web/src/components/features/vehicles/dialogs/LogFuelDialog.tsx`
- Create: `apps/web/src/components/features/vehicles/dialogs/LogMaintenanceDialog.tsx`
- Create: `apps/web/src/components/features/vehicles/dialogs/AddScheduleDialog.tsx`
- Create: `apps/web/src/components/features/vehicles/dialogs/CompleteScheduleDialog.tsx`
- Create: `apps/web/src/components/features/vehicles/dialogs/DeleteVehicleDialog.tsx`

**Interfaces:**
- Consumes: `Dialog*` (Task 10); the existing form components; the existing mutation hooks
- Produces: every dialog takes the same controlled-open shape:
```ts
interface DialogProps { open: boolean; onOpenChange: (open: boolean) => void; vehicleId: string }
```
`LogMaintenanceDialog` additionally takes `kind: MaintenanceCategoryKind` and `defaultMileage?: number`. `CompleteScheduleDialog` takes `schedule: MaintenanceSchedule` instead of `vehicleId` alone.

- [ ] **Step 1: Write LogMileageDialog as the reference implementation**

Create `apps/web/src/components/features/vehicles/dialogs/LogMileageDialog.tsx`:

```tsx
import { toast } from 'sonner';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '../../../ui/dialog';
import { MileageForm } from '../mileage/MileageForm';
import { useCreateMileageRecord } from '../../../../lib/hooks/api/mileage';
import type { MileageFormInput } from '../../../../lib/schemas/mileage';

interface LogMileageDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  vehicleId: string;
  defaultMileage?: number;
}

/**
 * Wraps MileageForm in a dialog. The form is unchanged — it already exposes
 * onSubmit/onCancel/submitting, so the dialog only owns the mutation and the
 * close-on-success rule. Errors keep the dialog open so the user's input
 * survives the failure.
 */
export function LogMileageDialog({
  open,
  onOpenChange,
  vehicleId,
  defaultMileage,
}: LogMileageDialogProps) {
  const createRecord = useCreateMileageRecord(vehicleId);

  const handleSubmit = async (values: MileageFormInput) => {
    try {
      await createRecord.mutateAsync({ mileage: values.mileage });
      toast.success('Mileage logged');
      onOpenChange(false);
    } catch (err) {
      const apiError = createErrorFromUnknown(err);
      toast.error(apiError.message || 'Could not log mileage');
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Log mileage</DialogTitle>
        </DialogHeader>
        <MileageForm
          defaultMileage={defaultMileage}
          onSubmit={handleSubmit}
          onCancel={() => onOpenChange(false)}
          submitting={createRecord.isPending}
        />
      </DialogContent>
    </Dialog>
  );
}
```

- [ ] **Step 2: Write the remaining six wrappers on the same pattern**

Each lifts its `handleSubmit` verbatim from the section component being replaced, so the mutation payloads and toast copy stay identical:

| Dialog | Form | Hook | Handler source |
|---|---|---|---|
| `EditVehicleDialog` | `VehicleForm` mode="edit" | `useUpdateVehicle` | `VehicleDetailPage.tsx:70-84` |
| `LogFuelDialog` | `FuelForm` | `useCreateFuelLog` | `VehicleFuelSection.tsx` |
| `LogMaintenanceDialog` | `MaintenanceRecordForm` | `useCreateMaintenanceRecord` | `VehicleMaintenanceSection.tsx:85-108` |
| `AddScheduleDialog` | `MaintenanceScheduleForm` | `useCreateMaintenanceSchedule` | `VehicleMaintenanceSection.tsx:110-124` |
| `CompleteScheduleDialog` | new inline form | `useCompleteMaintenanceSchedule` | `VehicleMaintenanceSection.tsx:126-143` |
| `DeleteVehicleDialog` | confirmation only | `useSoftDeleteVehicle` | `VehicleDetailPage.tsx:86-95` |

`LogMaintenanceDialog` needs the category list — call `useMaintenanceCategories()` inside it and pass the result to the form, exactly as `VehicleMaintenanceSection` does today, including the `maintenance`-only filter for `AddScheduleDialog`.

`CompleteScheduleDialog` has no existing form. Build it with react-hook-form + Zod like its siblings: a date field defaulting to today and an odometer field defaulting to the auto-fill mileage, submitting `{ date: ISO string, latestMileage: number }`.

`DeleteVehicleDialog` navigates to `/vehicles` with `replace: true` on success, as the current handler does.

- [ ] **Step 3: Verify the build**

Run: `npm run -w apps/web build && npm run -w apps/web lint`

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add apps/web/src/components/features/vehicles/dialogs
git commit -m "feat(web): add dialog wrappers for the vehicle forms"
```

---

## Task 12: Identity rail and quick actions

**Files:**
- Create: `apps/web/src/components/features/vehicles/detail/VehicleIdentityRail.tsx`
- Create: `apps/web/src/components/features/vehicles/detail/VehicleQuickActions.tsx`

**Interfaces:**
- Consumes: `useVehicleMedia`, `useMediaContentUrl` (existing media hooks); `StatusBadge`, `formatMileage` from `@myfleet/ui-components`
- Produces:
```ts
interface VehicleIdentityRailProps {
  vehicle: Vehicle;
  odometer?: number;
  canWrite: boolean;
  onEdit: () => void;
  onViewGallery: () => void;
}
export type QuickAction =
  | 'mileage' | 'fuel' | 'maintenance' | 'modification'
  | 'schedule' | 'upload' | 'delete';
interface VehicleQuickActionsProps {
  canWrite: boolean;
  canDelete: boolean;
  onAction: (action: QuickAction) => void;
}
```

- [ ] **Step 1: Build the rail**

Create `VehicleIdentityRail.tsx`. It renders, inside a `Card`:
- the primary photo (from `useVehicleMedia`, the resource flagged primary; a muted placeholder when absent),
- the title (`nickname` or `year make model`) plus `StatusBadge`,
- the spec line `{year} {make} {model} {trim}`,
- a `<dl>` of Odometer / VIN / Notes, each falling back to `—`,
- a three-thumbnail strip whose last tile shows `+N` and calls `onViewGallery`,
- an Edit button rendered only when `canWrite`.

Reuse the existing `VehiclePhotoThumbnail` component for the tiles rather than writing new image handling.

- [ ] **Step 2: Build the quick actions**

Create `VehicleQuickActions.tsx`: a `Card` containing one full-width `Button` per action, each calling `onAction(<action>)`. Layout classes are `flex flex-col gap-1 lg:flex-col` at rail width and `flex-row flex-wrap` below `lg`, so the stacked list becomes a wrapping button row on narrow screens. Hide every write action unless `canWrite`; show Delete only when `canDelete`.

- [ ] **Step 3: Verify the build**

Run: `npm run -w apps/web build && npm run -w apps/web lint`

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add apps/web/src/components/features/vehicles/detail
git commit -m "feat(web): add the vehicle identity rail and quick actions"
```

---

## Task 13: Stat strip and schedule tiles

**Files:**
- Create: `apps/web/src/components/features/vehicles/detail/VehicleStatStrip.tsx`
- Create: `apps/web/src/components/features/vehicles/detail/UpcomingScheduleStrip.tsx`

**Interfaces:**
- Consumes: `deriveOdometer`, `deriveTrailingCost`, `deriveAvgEconomy`, `deriveNextService` (Task 9); `SeverityChip` (existing)
- Produces:
```ts
interface VehicleStatStripProps {
  rows: VehicleRecordRow[];
  schedules: MaintenanceSchedule[];
  currentMileage?: number;
  /** True when rows are a partial window, which softens the cost/economy tiles. */
  partial: boolean;
}
interface UpcomingScheduleStripProps {
  schedules: MaintenanceSchedule[];
  canWrite: boolean;
  onAddSchedule: () => void;
  onComplete: (schedule: MaintenanceSchedule) => void;
}
```

- [ ] **Step 1: Build the stat strip**

Four tiles in `grid gap-3 grid-cols-[repeat(auto-fit,minmax(150px,1fr))]`. Each tile is label / value / subtitle. Every value renders `—` when its derivation returns `undefined`. When `partial` is true, the cost and economy tiles carry the subtitle "based on recent records" instead of a record count — the numbers are computed over loaded rows only and must not imply completeness.

Severity colors come from the semantic token families: `text-danger` for an overdue next-service value, `text-warning` for upcoming, default foreground otherwise.

- [ ] **Step 2: Build the schedule strip**

A `Card` whose body is `grid gap-3 grid-cols-[repeat(auto-fit,minmax(210px,1fr))]`. Each tile shows the category name, a `SeverityChip`, the recurrence line, and a Complete button gated on `canWrite`. Tiles sort overdue first, then upcoming, then healthy — import `rankSchedule` from `vehicleStats.ts` rather than duplicating the switch. Sort on `status`, never `severity`: the two fields carry different vocabularies (`'ok' | 'upcoming' | 'overdue'` versus `'urgent' | 'recommended' | 'informational'`) and only `status` describes urgency. `SeverityChip` renders `severity` and stays as-is. Overdue tiles take `border-danger-border bg-danger-subtle/45`; upcoming take the warning equivalents.

The category name requires the category map — accept resolved names as a prop rather than fetching inside the tile, so the strip stays presentational.

- [ ] **Step 3: Verify the build**

Run: `npm run -w apps/web build && npm run -w apps/web lint`

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add apps/web/src/components/features/vehicles/detail
git commit -m "feat(web): add the vehicle stat strip and schedule tiles"
```

---

## Task 14: Records table

**Files:**
- Create: `apps/web/src/components/features/vehicles/detail/VehicleRecordsTable.tsx`
- Test: `apps/web/src/components/features/vehicles/detail/VehicleRecordsTable.test.tsx`

**Interfaces:**
- Consumes: `filterVehicleRecords`, `VehicleRecordRow` (Task 7)
- Produces:
```ts
interface VehicleRecordsTableProps {
  rows: VehicleRecordRow[];
  total: number;
  hasMore: boolean;
  isLoading: boolean;
  onLoadMore: () => void;
  onSelectRow: (row: VehicleRecordRow) => void;
}
```

- [ ] **Step 1: Write the failing test**

Create `apps/web/src/components/features/vehicles/detail/VehicleRecordsTable.test.tsx`:

```tsx
import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { VehicleRecordsTable } from './VehicleRecordsTable';
import type { VehicleRecordRow } from '../../../../lib/vehicleRecords';

const rows: VehicleRecordRow[] = [
  { id: 'fuel:1', sourceId: '1', kind: 'fuel', date: '2026-07-28T00:00:00Z',
    title: '16.204 gal', mileage: 84010, cost: 58.31 },
  { id: 'maintenance:2', sourceId: '2', kind: 'maintenance', date: '2026-07-12T00:00:00Z',
    title: 'Front brake pads', mileage: 82940, cost: 612.4 },
  { id: 'modification:3', sourceId: '3', kind: 'modification', date: '2026-05-18T00:00:00Z',
    title: 'Rock sliders', mileage: 79430, cost: 980 },
];

function renderTable(props: Partial<React.ComponentProps<typeof VehicleRecordsTable>> = {}) {
  return render(
    <VehicleRecordsTable
      rows={rows}
      total={41}
      hasMore
      isLoading={false}
      onLoadMore={vi.fn()}
      onSelectRow={vi.fn()}
      {...props}
    />,
  );
}

describe('VehicleRecordsTable', () => {
  it('shows every row under the All filter', () => {
    renderTable();
    expect(screen.getByText('16.204 gal')).toBeInTheDocument();
    expect(screen.getByText('Front brake pads')).toBeInTheDocument();
    expect(screen.getByText('Rock sliders')).toBeInTheDocument();
  });

  it('narrows to one kind when a chip is pressed', async () => {
    const user = userEvent.setup();
    renderTable();

    await user.click(screen.getByRole('button', { name: /^fuel$/i }));

    expect(screen.getByText('16.204 gal')).toBeInTheDocument();
    expect(screen.queryByText('Front brake pads')).not.toBeInTheDocument();
  });

  it('keeps maintenance and mods on separate chips', async () => {
    const user = userEvent.setup();
    renderTable();

    await user.click(screen.getByRole('button', { name: /^mods$/i }));

    expect(screen.getByText('Rock sliders')).toBeInTheDocument();
    expect(screen.queryByText('Front brake pads')).not.toBeInTheDocument();
  });

  it('reports how many of the total are shown', () => {
    renderTable();
    expect(screen.getByText(/showing 3 of 41/i)).toBeInTheDocument();
  });

  it('passes the clicked row to onSelectRow', async () => {
    const onSelectRow = vi.fn();
    const user = userEvent.setup();
    renderTable({ onSelectRow });

    await user.click(screen.getByText('Front brake pads'));

    expect(onSelectRow).toHaveBeenCalledWith(expect.objectContaining({ id: 'maintenance:2' }));
  });

  it('hides load more when there is nothing left', () => {
    renderTable({ hasMore: false });
    expect(screen.queryByRole('button', { name: /load more/i })).not.toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `npm run -w apps/web test -- src/components/features/vehicles/detail/VehicleRecordsTable.test.tsx`

Expected: FAIL — cannot resolve `./VehicleRecordsTable`.

- [ ] **Step 3: Implement the table**

Create the component. It holds one piece of state — the active chip (`VehicleRecordKind | 'all'`, default `'all'`) — and derives its visible rows with `filterVehicleRecords`. Structure:

- A `Card` header with the title "Records" and five chips: All, Maintenance, Mods, Fuel, Mileage. Each chip is a `Button` with `aria-pressed` and the accessible names the test asserts (`All`, `Maintenance`, `Mods`, `Fuel`, `Mileage`).
- A table with columns Date / Type / Item / Odometer / Cost. Each `<tr>` is clickable and calls `onSelectRow(row)`; give it `role="button"` and a keyboard handler so it is not mouse-only.
- Type cells render a badge per kind, using semantic tokens: mileage `info`, fuel `muted`, maintenance `success`, modification the violet chip already used in `VehicleMaintenanceSection.tsx:312-322` (copy that exact class list and its "intentional status colors" comment).
- A footer reading `Showing {visible.length} of {total}` plus a Load more button rendered only when `hasMore`.
- Skeleton rows while `isLoading`; "No records yet." when `rows` is empty and not loading.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `npm run -w apps/web test -- src/components/features/vehicles/detail/VehicleRecordsTable.test.tsx`

Expected: PASS (6 tests).

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/components/features/vehicles/detail/VehicleRecordsTable.tsx apps/web/src/components/features/vehicles/detail/VehicleRecordsTable.test.tsx
git commit -m "feat(web): add the unified vehicle records table"
```

---

## Task 15: Record drawer

**Files:**
- Create: `apps/web/src/components/features/vehicles/detail/VehicleRecordDrawer.tsx`
- Modify: `apps/web/src/lib/hooks/api/maintenance.ts`

**Interfaces:**
- Consumes: `Sheet*` (Task 10); `useMaintenanceRecord`, `useFuelLog`, `useDeleteMaintenanceRecord`, `useUpdateFuelLog`, `useDeleteFuelLog` (existing); `RecordAttachmentList` (existing)
- Produces:
  - `useUpdateMaintenanceRecord(vehicleId)` — `PATCH /api/fleet/maintenance-records/{id}`
  - ```ts
    interface VehicleRecordDrawerProps {
      row: VehicleRecordRow | null;   // null closes the drawer
      onClose: () => void;
      vehicleId: string;
      canWrite: boolean;
    }
    ```

- [ ] **Step 1: Add the missing update hook**

In `apps/web/src/lib/hooks/api/maintenance.ts`, beside `useDeleteMaintenanceRecord`:

```ts
/** PATCH /api/fleet/maintenance-records/{id} — partial update. */
export function useUpdateMaintenanceRecord(vehicleId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, attributes }: { id: string; attributes: UpdateMaintenanceRecordAttributes }) =>
      maintenanceRecordService.patch(id, attributes),
    onSettled: (_data, _error, variables) => {
      void queryClient.invalidateQueries({ queryKey: maintenanceRecordKeys.lists() });
      void queryClient.invalidateQueries({ queryKey: maintenanceRecordKeys.detail(variables.id) });
      void queryClient.invalidateQueries({ queryKey: vehicleKeys.detail(vehicleId) });
    },
  });
}
```

Import `UpdateMaintenanceRecordAttributes` from `../../../types/models/maintenanceRecord`.

- [ ] **Step 2: Build the drawer**

Create `VehicleRecordDrawer.tsx`. It renders a `Sheet` with `side="right"`, open whenever `row != null`. The body switches on `row.kind`:

- **maintenance / modification** — fetches the full record with `useMaintenanceRecord(row.sourceId)` and shows performed date, odometer, cost, vendor, notes, and `RecordAttachmentList` for `documentMediaIds`. Actions when `canWrite`: Edit (swaps the body for `MaintenanceRecordForm` in edit mode, submitting through `useUpdateMaintenanceRecord`) and Delete (through `useDeleteMaintenanceRecord`, closing the drawer on success).

  **Pass `kind` explicitly** — derive it from the row (`row.kind === 'modification' ? 'modification' : 'maintenance'`), never omit it. Task 5 made the prop required precisely so this cannot be forgotten: an unfiltered picker would let a user editing a modification record create a category written as `maintenance`, which then disappears from the modification picker and cannot be corrected from the UI.
- **fuel** — fetches with `useFuelLog(row.sourceId)` and shows date, gallons, price per gallon, total, odometer. Actions when `canWrite`: Edit via `FuelForm` + `useUpdateFuelLog`, Delete via `useDeleteFuelLog`.
- **mileage** — read-only: date, mileage, source, and the flagged marker. **Render no Edit or Delete button.** No update or delete endpoint exists for mileage records; offering either would produce a 404 or 405.

Fetch only for the open row — the drawer mounts its query hooks with `enabled` driven by `row?.kind`, so closing it stops the request.

- [ ] **Step 3: Verify the build**

Run: `npm run -w apps/web build && npm run -w apps/web lint`

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add apps/web/src/components/features/vehicles/detail/VehicleRecordDrawer.tsx apps/web/src/lib/hooks/api/maintenance.ts
git commit -m "feat(web): add the vehicle record drawer

Offers only the mutations that exist per record kind; mileage records are
read-only because no update or delete endpoint exists for them."
```

---

## Task 16: Trends block and gallery dialog

**Files:**
- Create: `apps/web/src/components/features/vehicles/detail/VehicleTrends.tsx`
- Create: `apps/web/src/components/features/vehicles/dialogs/PhotoGalleryDialog.tsx`

**Interfaces:**
- Consumes: `MileageSparkline` (existing), `VehicleActivityTimeline` (existing), the media hooks
- Produces:
```ts
interface VehicleTrendsProps { vehicleId: string; mileageRows: VehicleRecordRow[] }
interface PhotoGalleryDialogProps { open: boolean; onOpenChange: (open: boolean) => void; vehicleId: string; canWrite: boolean }
```

- [ ] **Step 1: Build the trends block**

A `Card` with a two-up responsive body (`grid gap-5 grid-cols-[repeat(auto-fit,minmax(260px,1fr))]`): the mileage sparkline on one side, `VehicleActivityTimeline` on the other. `MileageSparkline` takes mileage records, not `VehicleRecordRow`s — pass the raw records from `useMileageRecords` rather than reshaping the merged rows.

- [ ] **Step 2: Build the gallery dialog**

Lift the body of `VehicleMediaGallery` into a dialog: the responsive thumbnail grid, `MediaUploadButton`, set-primary, and delete. Behavior and hooks are unchanged; only the container differs.

- [ ] **Step 3: Verify the build**

Run: `npm run -w apps/web build && npm run -w apps/web lint`

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add apps/web/src/components/features/vehicles/detail/VehicleTrends.tsx apps/web/src/components/features/vehicles/dialogs/PhotoGalleryDialog.tsx
git commit -m "feat(web): add the vehicle trends block and photo gallery dialog"
```

---

## Task 17: Assemble the page and delete the old sections

**Files:**
- Modify: `apps/web/src/pages/VehicleDetailPage.tsx`
- Delete: `apps/web/src/components/features/vehicles/mileage/VehicleMileageSection.tsx`
- Delete: `apps/web/src/components/features/vehicles/maintenance/VehicleMaintenanceSection.tsx`
- Delete: `apps/web/src/components/features/vehicles/fuel/VehicleFuelSection.tsx`
- Delete: `apps/web/src/components/features/vehicles/media/VehicleMediaGallery.tsx`

**Interfaces:**
- Consumes: every component from Tasks 11-16
- Produces: the finished page

- [ ] **Step 1: Rewrite the page**

`VehicleDetailPage` keeps its existing loading, not-found, and role logic (`canWrite`, `canRestore`) verbatim. It gains:

- one `useState` for which dialog is open (`type OpenDialog = QuickAction | 'edit' | 'gallery' | 'complete' | null`),
- one `useState<VehicleRecordRow | null>` for the drawer,
- `useVehicleRecords(vehicle.id, categories)`, `useMaintenanceSchedules(vehicle.id)`, and `useMaintenanceCategories()`.

The container becomes:

```tsx
<div className="mx-auto w-full max-w-[1600px]">
  <div className="grid gap-6 lg:grid-cols-[320px_minmax(0,1fr)] lg:items-start">
    <aside className="grid gap-4 lg:sticky lg:top-16">
      <VehicleIdentityRail … />
      <VehicleQuickActions … />
    </aside>
    <div className="grid gap-4">
      <VehicleStatStrip … />
      <UpcomingScheduleStrip … />
      <VehicleRecordsTable … />
      <VehicleTrends … />
    </div>
  </div>
  {/* dialogs + drawer, rendered once at page level */}
</div>
```

`minmax(0,1fr)` on the content column is required — without it the records table's intrinsic width prevents the grid from shrinking below its content.

The Restore button (owner-only) keeps its current placement logic; put it in the rail beside Edit.

- [ ] **Step 2: Delete the four superseded sections**

```bash
git rm apps/web/src/components/features/vehicles/mileage/VehicleMileageSection.tsx \
       apps/web/src/components/features/vehicles/maintenance/VehicleMaintenanceSection.tsx \
       apps/web/src/components/features/vehicles/fuel/VehicleFuelSection.tsx \
       apps/web/src/components/features/vehicles/media/VehicleMediaGallery.tsx
```

- [ ] **Step 3: Confirm nothing else referenced them**

Run: `grep -rn "VehicleMileageSection\|VehicleMaintenanceSection\|VehicleFuelSection\|VehicleMediaGallery" apps/web/src`

Expected: no matches. Any hit is a consumer that must be migrated, not a file to re-add.

- [ ] **Step 4: Run the full gate**

Run: `make ci`

Expected: PASS — `lint-check`, `vet`, `test`, `build`, `fe-test`, `fe-build`, `manifests`, `carfax-template`.

- [ ] **Step 5: Verify in the running app**

Run: `make up`, open the app, and check on a desktop viewport:
- the page fills the width and the rail stays put while the records table scrolls,
- every quick action opens a dialog and nothing shifts the page,
- a record row opens the drawer; a mileage row shows no Edit or Delete,
- typing a new category name in the Log maintenance dialog offers to create it, and the created category is selected on return,
- narrowing the window below 1024px stacks the rail above the content and turns quick actions into a wrapping row.

- [ ] **Step 6: Commit**

```bash
git add apps/web/src
git commit -m "feat(web): rebuild the vehicle detail page as rail + records

Two-column responsive layout replacing the max-w-2xl single column, with
every form in a dialog and a unified records feed. Removes the four section
components whose content the new layout absorbs."
```

---

## Self-Review Notes

**Spec coverage.** Design §4 layout → Task 17. §5 decomposition → Tasks 12-17. §6 merged feed → Tasks 7-8. §6.1 pagination → Task 6. §7 stat strip → Tasks 9, 13. §8 drawer → Task 15. §9 dialogs → Tasks 10-11, 16. §10.1 category backend → Tasks 1-2. §10.2 category frontend → Tasks 3-5. §11 testing → distributed.

**Two gaps found and closed while reviewing:**

1. `deriveAvgEconomy` needs gallons, which `VehicleRecordRow` did not carry. Task 9 Step 1 now adds `gallons?: number` to the type and populates it in Task 8's adapter.
2. Task 6 removes the `select: (result) => result.data` from three hooks, changing their return type. Step 3 is the explicit consumer sweep; without it the task compiles nowhere.

**Third gap, found on a verification pass:** `deriveNextService` originally switched on `MaintenanceScheduleAttributes.severity` to find the most urgent schedule. That field is `'urgent' | 'recommended' | 'informational'`; the overdue/upcoming vocabulary lives on `status`. Tasks 9 and 13 now both switch on `status` and share an exported `rankSchedule`.

**Fourth gap, found in the pre-flight scan before execution:** Task 8 originally gave each source its own page cursor via `useState`, which made "load more" *replace* a source's rows rather than append them — dropping the newest records from view once any source exceeded 100 rows. Task 6 now converts all three hooks to `useInfiniteQuery` with a `select` that flattens accumulated pages, and Task 8 calls `fetchNextPage()` per source. Task 7's merge is untouched: it takes plain arrays precisely so the pagination strategy could change without it.
