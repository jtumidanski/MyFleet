# Record Attachment Management Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make a maintenance/modification record's attachment set editable for the record's whole life — implement the attach endpoint the frontend already calls, add a detach endpoint plus a drawer remove control, and make the ten-attachment cap enforceable on the edit path.

**Architecture:** Two new fleet-service routes under `apps/fleet-service/internal/maintenancerecord` follow the package's existing resource → processor → administrator split. Attach runs in one transaction that locks the parent record row (dialect-guarded so sqlite tests still work), checks idempotency, checks the cap, inserts, and re-reads. Detach is a single soft-delete `UPDATE`. A partial unique index on `(maintenance_record_id, media_id) WHERE deleted_at IS NULL` — preceded by a one-time de-duplication — is the durable invariant. On the client, the drawer owns a confirmation dialog and a `useRemoveMaintenanceRecordDocument` hook that detaches the reference first and best-effort deletes the media object second, exactly like `useRemoveVehiclePhoto`.

**Tech Stack:** Go 1.x, chi v5, GORM (Postgres in production, sqlite in-memory in tests), `packages/shared-go/server` hand-rolled JSON:API transport; React 19 + TypeScript, Vite, TanStack React Query v5, react-hook-form + Zod, shadcn/ui, Tailwind, vitest + React Testing Library.

**Spec:** `docs/tasks/task-027-record-attachment-management/design.md` (PRD: `docs/tasks/task-027-record-attachment-management/prd.md`)

## Global Constraints

- **Cross-fleet is `404`, not `403`.** The PRD §5.1/§5.2 tables and design §3.1 both say a cross-fleet caller gets `403`. That is wrong about this codebase: `authz.RequireSameFleet` (`apps/fleet-service/internal/authz/authz.go:12-17`) returns `server.ErrNotFound` specifically so cross-fleet existence is never leaked, and `TestPatch_otherFleetIsNotFound` pins it. **Implement 404 for cross-fleet and 403 for viewer**, matching the existing `PATCH`/`DELETE` item routes. Do not change `authz`.
- `MaxDocuments` is `10` and is not changed (`apps/fleet-service/internal/maintenancerecord/model.go:19`). The frontend mirror is `MAX_ATTACHMENTS = 10` in `apps/web/src/lib/hooks/usePendingAttachments.ts:11`.
- `PATCH /maintenance-records/{id}` gains no fields. Its attrs struct and `Update`'s column allow-list stay exactly as they are (PRD D2, design D-1).
- No media-service changes. Detach's object delete is issued by the browser against the existing `DELETE /api/media/{id}`.
- No new columns and no data migration beyond the idempotent de-duplication `UPDATE` in step 1. `DocumentEntity` is unchanged.
- `mediaclient` gains no delete capability. `DocumentValidator` keeps its `nil`-is-legal contract.
- Request body for attach is exactly `{"data":{"type":"mediaRefs","attributes":{"mediaId":"…"}}}`. The client is not changed to fit the server; the declared JSON:API `type` is not validated.
- Attach returns `201` with the full updated `maintenanceRecords` resource. Detach returns `204` with an empty body.
- Best-effort failures on the client (the media-object delete after a successful detach) must never surface as a failed removal, and must not be silently discarded — `console.warn` them.
- Verification command for the whole branch: `make ci`. Node may need `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22` first.
- All work happens in the worktree `/home/tumidanski/source/MyFleet/.worktrees/task-027-record-attachment-management` on branch `task-027-record-attachment-management`.

---

## File Structure

**fleet-service (`apps/fleet-service/internal/maintenancerecord/`)**

| File | Change | Responsibility |
| --- | --- | --- |
| `entity.go` | Modify | `Migration` gains `ApplyPartialIndexes`; new `ApplyPartialIndexes` + `dedupeLiveDocuments` own the uniqueness invariant. |
| `entity_test.go` | Create | Proves the index is unique, is partial (detach-then-reattach works), and that de-duplication runs before it. |
| `administrator.go` | Modify | `AttachDocument` / `DetachDocument` — the only place document rows are mutated outside `InsertTx`. |
| `administrator_test.go` | Modify | Cap, idempotency, soft-delete and not-found behavior at the data-access layer. |
| `processor.go` | Modify | `AttachDocument` / `DetachDocument` pass-throughs with `ErrNotFound → server.ErrNotFound`. |
| `processor_test.go` | Create | Error-translation only; the layer has no other behavior. |
| `resource.go` | Modify | The two new routes and their authorization + ownership preamble. |
| `resource_test.go` | Modify | Status codes, ownership rejection, viewer/cross-fleet gating, `modification`-kind parity, `PATCH` non-interference. |
| `provider_test.go` | Modify | `newTestDB` splits into `newTestDBWithoutIndexes` + index application so tests exercise the real index. |

**web (`apps/web/src/`)**

| File | Change | Responsibility |
| --- | --- | --- |
| `services/api/MaintenanceRecordService.ts` | Modify | Adds `removeDocumentMedia`. |
| `lib/hooks/api/maintenance.ts` | Modify | Adds `useRemoveMaintenanceRecordDocument`; widens `useAppendMaintenanceRecordDocument` invalidation. |
| `lib/hooks/api/maintenance.test.ts` | Create | Best-effort media-delete behavior and invalidation keys. |
| `lib/hooks/usePendingAttachments.ts` | Modify | Accepts `existingCount` in its room/full arithmetic. |
| `lib/hooks/usePendingAttachments.test.ts` | Modify | Combined-cap drop behavior. |
| `components/features/vehicles/maintenance/AttachmentPicker.tsx` | Modify | `existingCount` prop, capacity-aware full state and helper text. |
| `components/features/vehicles/maintenance/AttachmentPicker.test.tsx` | Create | Helper text and disabled "Add files" at the combined cap. |
| `components/features/vehicles/maintenance/MaintenanceRecordForm.tsx` | Modify | Threads `existingAttachmentCount` to hook and picker. |
| `components/features/vehicles/maintenance/RecordAttachmentList.tsx` | Modify | Optional `onRemove` / `canRemove`, remove control on all three row shapes. |
| `components/features/vehicles/maintenance/RecordAttachmentList.test.tsx` | Modify | Remove-control gating including the unavailable row. |
| `components/features/vehicles/detail/VehicleRecordDrawer.tsx` | Modify | Owns the confirmation dialog, the remove mutation, and the existing-count prop. |

---

### Task 1: Partial unique index and de-duplication

The uniqueness invariant must land before any code relies on it. It must be **partial** (`WHERE deleted_at IS NULL`) or detach-then-reattach — the exact flow this task creates — would fail forever against a soft-deleted row squatting the slot. It must be **preceded by de-duplication** or `Migration` fails at service startup for any fleet whose create call was handed the same media ID twice in `documentMediaIds`.

**Files:**
- Modify: `apps/fleet-service/internal/maintenancerecord/entity.go:41` (the `Migration` function)
- Modify: `apps/fleet-service/internal/maintenancerecord/provider_test.go:14-47` (`newTestDB`)
- Test: `apps/fleet-service/internal/maintenancerecord/entity_test.go` (create)

**Interfaces:**
- Consumes: nothing.
- Produces: `func ApplyPartialIndexes(db *gorm.DB) error` in package `maintenancerecord`. `func newTestDBWithoutIndexes(t *testing.T) *gorm.DB` in the package's test files; `newTestDB(t)` keeps its existing signature `func(t *testing.T) *gorm.DB` and now also applies the indexes.

- [ ] **Step 1: Split the test DB helper so a no-index database is available**

In `provider_test.go`, rename the existing `newTestDB` body to `newTestDBWithoutIndexes` and add a new `newTestDB` that layers the indexes on top. Replace lines 14-47 (the whole current `newTestDB` function) with:

```go
// newTestDBWithoutIndexes builds the schema-qualified sqlite fixture WITHOUT
// the partial unique indexes, so a test can seed rows that the index would
// reject and then prove ApplyPartialIndexes cleans them up.
func newTestDBWithoutIndexes(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// TableName is schema-qualified (fleet.maintenance_records) for Postgres.
	// SQLite has no schemas, so attach an in-memory database aliased "fleet".
	if err := db.Exec("ATTACH DATABASE ':memory:' AS fleet").Error; err != nil {
		t.Fatalf("attach fleet schema: %v", err)
	}
	// GORM's AutoMigrate emits CREATE INDEX with the schema prefix stripped (a
	// SQLite quirk), so Migration(db) fails here: Entity.DeletedAt and
	// DocumentEntity.MaintenanceRecordID both carry gorm:"index" tags. The
	// schema-qualified tables are created with explicit DDL instead, mirroring
	// the same workaround used in maintenanceschedule/completion_db_test.go,
	// dashboard/aggregate_test.go, and activity/processor_test.go.
	ddl := []string{
		`CREATE TABLE fleet.maintenance_records (
			id TEXT PRIMARY KEY, vehicle_id TEXT, category_id TEXT, description TEXT,
			performed_at DATETIME, mileage INTEGER, cost REAL, vendor TEXT, notes TEXT,
			created_by_user_id TEXT, created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
			purge_operation_id TEXT)`,
		`CREATE TABLE fleet.maintenance_record_documents (
			id TEXT PRIMARY KEY, maintenance_record_id TEXT, media_id TEXT,
			deleted_at DATETIME, purge_operation_id TEXT)`,
	}
	for _, stmt := range ddl {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("ddl: %v", err)
		}
	}
	return db
}

// newTestDB is the fixture every other test uses: the same schema production
// gets, indexes included. Applying the real ApplyPartialIndexes here rather
// than hand-writing the DDL is deliberate — a test database without the
// index would let a duplicate-row bug pass.
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := newTestDBWithoutIndexes(t)
	if err := ApplyPartialIndexes(db); err != nil {
		t.Fatalf("apply partial indexes: %v", err)
	}
	return db
}
```

- [ ] **Step 2: Write the failing tests**

Create `apps/fleet-service/internal/maintenancerecord/entity_test.go`:

```go
package maintenancerecord

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// countLiveDocs returns the number of non-deleted document rows for a record.
func countLiveDocs(t *testing.T, db *gorm.DB, recordID string) int64 {
	t.Helper()
	var n int64
	if err := db.Model(&DocumentEntity{}).
		Where("maintenance_record_id = ? AND deleted_at IS NULL", recordID).
		Count(&n).Error; err != nil {
		t.Fatalf("count docs: %v", err)
	}
	return n
}

// The index is what keeps a second live row for the same (record, media) pair
// out of the table even if an application-level check is ever bypassed.
func TestApplyPartialIndexes_rejectsASecondLiveRowForTheSamePair(t *testing.T) {
	db := newTestDB(t)

	first := DocumentEntity{ID: uuid.NewString(), MaintenanceRecordID: "r1", MediaID: "m1"}
	if err := db.Create(&first).Error; err != nil {
		t.Fatalf("first insert: %v", err)
	}

	second := DocumentEntity{ID: uuid.NewString(), MaintenanceRecordID: "r1", MediaID: "m1"}
	if err := db.Create(&second).Error; err == nil {
		t.Fatal("second live row for the same (record, media) pair was accepted; the unique index is missing")
	}
	if got := countLiveDocs(t, db, "r1"); got != 1 {
		t.Errorf("live rows = %d, want 1", got)
	}
}

// The index MUST be partial. A plain unique index would let a soft-deleted row
// occupy the slot forever, and detach-then-reattach is the flow this task
// exists to enable.
func TestApplyPartialIndexes_allowsReattachAfterSoftDelete(t *testing.T) {
	db := newTestDB(t)

	first := DocumentEntity{ID: uuid.NewString(), MaintenanceRecordID: "r1", MediaID: "m1"}
	if err := db.Create(&first).Error; err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if err := db.Model(&DocumentEntity{}).Where("id = ?", first.ID).
		Update("deleted_at", time.Now().UTC()).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	again := DocumentEntity{ID: uuid.NewString(), MaintenanceRecordID: "r1", MediaID: "m1"}
	if err := db.Create(&again).Error; err != nil {
		t.Fatalf("reattach after soft delete: %v — the index is not partial", err)
	}
	if got := countLiveDocs(t, db, "r1"); got != 1 {
		t.Errorf("live rows = %d, want 1", got)
	}
}

// Existing data can already violate the index (the create path could be handed
// the same id twice). Without de-duplication first, index creation fails and
// takes the service down at startup.
func TestApplyPartialIndexes_dedupesPreexistingLiveDuplicates(t *testing.T) {
	db := newTestDBWithoutIndexes(t)

	keep := DocumentEntity{ID: "00000000-aaaa", MaintenanceRecordID: "r1", MediaID: "m1"}
	dupe := DocumentEntity{ID: "ffffffff-zzzz", MaintenanceRecordID: "r1", MediaID: "m1"}
	other := DocumentEntity{ID: uuid.NewString(), MaintenanceRecordID: "r1", MediaID: "m2"}
	for _, d := range []DocumentEntity{keep, dupe, other} {
		if err := db.Create(&d).Error; err != nil {
			t.Fatalf("seed %s: %v", d.ID, err)
		}
	}

	if err := ApplyPartialIndexes(db); err != nil {
		t.Fatalf("ApplyPartialIndexes on dirty data: %v", err)
	}

	if got := countLiveDocs(t, db, "r1"); got != 2 {
		t.Errorf("live rows = %d, want 2 (one per distinct media id)", got)
	}
	// The lowest id survives, and the loser is soft-deleted rather than removed.
	var survivor DocumentEntity
	if err := db.Where("maintenance_record_id = ? AND media_id = ? AND deleted_at IS NULL", "r1", "m1").
		First(&survivor).Error; err != nil {
		t.Fatalf("find survivor: %v", err)
	}
	if survivor.ID != keep.ID {
		t.Errorf("survivor ID = %q, want the lowest id %q", survivor.ID, keep.ID)
	}
	var loser DocumentEntity
	if err := db.Where("id = ?", dupe.ID).First(&loser).Error; err != nil {
		t.Fatalf("find loser: %v", err)
	}
	if loser.DeletedAt == nil {
		t.Error("duplicate row was not soft-deleted")
	}
}

// Re-running the migration must be a no-op, not an error.
func TestApplyPartialIndexes_isIdempotent(t *testing.T) {
	db := newTestDB(t)
	if err := ApplyPartialIndexes(db); err != nil {
		t.Fatalf("second ApplyPartialIndexes: %v", err)
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./apps/fleet-service/internal/maintenancerecord/ -run 'TestApplyPartialIndexes' -v`
Expected: FAIL — `undefined: ApplyPartialIndexes`.

- [ ] **Step 4: Implement `ApplyPartialIndexes` and de-duplication**

In `entity.go`, replace the one-line `Migration` (line 41) with the following, and add `dedupeLiveDocuments`:

```go
func Migration(db *gorm.DB) error {
	if err := db.AutoMigrate(&Entity{}, &DocumentEntity{}); err != nil {
		return err
	}
	return ApplyPartialIndexes(db)
}

// ApplyPartialIndexes makes (maintenance_record_id, media_id) unique among
// LIVE document rows, following the same pattern as mediavariant, membership
// and invite.
//
// It cannot be a `gorm:"uniqueIndex"` struct tag: GORM has no way to express a
// WHERE clause, and a plain unique index is the wrong invariant here. A
// detached (soft-deleted) row would keep occupying the slot, so re-attaching a
// media object a user had previously removed would fail forever against a row
// no reader can see. Detach-then-reattach is an obvious user action and is the
// exact flow the detach endpoint creates.
//
// De-duplication runs first and is not optional. Existing data can already
// violate the index — nothing stopped the create path from being handed the
// same id twice in documentMediaIds — and CREATE UNIQUE INDEX against such a
// row fails, which would take the service down at startup rather than at the
// call site.
func ApplyPartialIndexes(db *gorm.DB) error {
	if err := dedupeLiveDocuments(db); err != nil {
		return err
	}
	create := `CREATE UNIQUE INDEX IF NOT EXISTS ux_maintenance_record_documents_record_media
	 ON fleet.maintenance_record_documents (maintenance_record_id, media_id) WHERE deleted_at IS NULL`
	if db.Name() == "sqlite" {
		// SQLite puts the schema on the INDEX name, not the table name.
		create = `CREATE UNIQUE INDEX IF NOT EXISTS fleet.ux_maintenance_record_documents_record_media
		 ON maintenance_record_documents (maintenance_record_id, media_id) WHERE deleted_at IS NULL`
	}
	return db.Exec(create).Error
}

// dedupeLiveDocuments soft-deletes all but the lowest-id live row in each
// (maintenance_record_id, media_id) group. Idempotent: a second run matches
// nothing. CURRENT_TIMESTAMP rather than NOW() so the one statement is valid
// under both Postgres and the sqlite test fixture.
func dedupeLiveDocuments(db *gorm.DB) error {
	return db.Exec(`UPDATE fleet.maintenance_record_documents
	 SET deleted_at = CURRENT_TIMESTAMP
	 WHERE deleted_at IS NULL
	   AND id NOT IN (
	     SELECT MIN(id) FROM fleet.maintenance_record_documents
	     WHERE deleted_at IS NULL
	     GROUP BY maintenance_record_id, media_id
	   )`).Error
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./apps/fleet-service/internal/maintenancerecord/ -v`
Expected: PASS — the four new `TestApplyPartialIndexes_*` tests plus every pre-existing test in the package (the `newTestDB` split must not have broken them).

- [ ] **Step 6: Commit**

```bash
git add apps/fleet-service/internal/maintenancerecord/entity.go \
        apps/fleet-service/internal/maintenancerecord/entity_test.go \
        apps/fleet-service/internal/maintenancerecord/provider_test.go
git commit -m "feat(fleet): partial unique index on live record document rows"
```

---

### Task 2: `Administrator.AttachDocument` / `DetachDocument`

**Files:**
- Modify: `apps/fleet-service/internal/maintenancerecord/administrator.go:11-20` (interface) and the end of the file (implementations)
- Test: `apps/fleet-service/internal/maintenancerecord/administrator_test.go` (append)

**Interfaces:**
- Consumes: `ApplyPartialIndexes` (Task 1) via `newTestDB`; existing `Make`, `MaxDocuments`, `ErrNotFound`, `DocumentEntity`.
- Produces: on the `Administrator` interface —
  - `AttachDocument(recordID, mediaID string) (Model, error)` — returns the re-read model. Errors: `ErrNotFound` (record missing or soft-deleted), `server.ErrValidation` (record already at `MaxDocuments`).
  - `DetachDocument(recordID, mediaID string) error` — errors: `ErrNotFound` (no live row for that pair).

- [ ] **Step 1: Write the failing tests**

Append to `administrator_test.go`. Its import block currently holds only `testing`; replace it with:

```go
import (
	"errors"
	"testing"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)
```

`countLiveDocs` comes from `entity_test.go` (Task 1) — same package, no import needed. Then append:

```go
// A fresh attach adds exactly one live row and the returned model reflects it.
func TestAttachDocument_addsOneLiveRowAndReturnsTheStoredSet(t *testing.T) {
	db := newTestDB(t)
	a := NewAdministrator(db)
	id := insertRecord(t, a, "v1", "cat-1", 5, 0)

	m, err := a.AttachDocument(id, "media-1")
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	if got := m.DocumentMediaIDs(); len(got) != 1 || got[0] != "media-1" {
		t.Errorf("DocumentMediaIDs() = %v, want [media-1]", got)
	}
	if got := countLiveDocs(t, db, id); got != 1 {
		t.Errorf("live rows = %d, want 1", got)
	}
	// The returned model must be a re-read, not the in-memory entity — a
	// zero createdAt is how the PATCH path used to lie about stored state.
	if m.CreatedAt().IsZero() {
		t.Error("returned model carries a zero CreatedAt; it was not re-read")
	}
}

// FR-ATT-8: the drawer retries its sequential loop after a partial failure, so
// re-attaching an id the record already holds must succeed and must not create
// a second row.
func TestAttachDocument_isIdempotentForAnAlreadyAttachedMedia(t *testing.T) {
	db := newTestDB(t)
	a := NewAdministrator(db)
	id := insertRecord(t, a, "v1", "cat-1", 5, 0)

	if _, err := a.AttachDocument(id, "media-1"); err != nil {
		t.Fatalf("first attach: %v", err)
	}
	m, err := a.AttachDocument(id, "media-1")
	if err != nil {
		t.Fatalf("second attach: %v", err)
	}
	if got := m.DocumentMediaIDs(); len(got) != 1 {
		t.Errorf("DocumentMediaIDs() = %v, want the id exactly once", got)
	}
	if got := countLiveDocs(t, db, id); got != 1 {
		t.Errorf("live rows = %d, want 1", got)
	}
}

// FR-ATT-7: the eleventh attachment is rejected and writes nothing.
func TestAttachDocument_rejectsTheEleventhAndWritesNothing(t *testing.T) {
	db := newTestDB(t)
	a := NewAdministrator(db)
	id := insertRecord(t, a, "v1", "cat-1", 5, MaxDocuments)

	_, err := a.AttachDocument(id, "one-too-many")
	if !errors.Is(err, server.ErrValidation) {
		t.Fatalf("err = %v, want server.ErrValidation", err)
	}
	if got := countLiveDocs(t, db, id); got != int64(MaxDocuments) {
		t.Errorf("live rows = %d, want %d", got, MaxDocuments)
	}
}

// Re-attaching an id a FULL record already holds must succeed: the idempotency
// check runs before the cap check, so a retry of an attach that actually landed
// is not punished with a 422.
func TestAttachDocument_reattachOnAFullRecordSucceeds(t *testing.T) {
	db := newTestDB(t)
	a := NewAdministrator(db)
	id := insertRecord(t, a, "v1", "cat-1", 5, MaxDocuments-1)
	if _, err := a.AttachDocument(id, "media-last"); err != nil {
		t.Fatalf("attach to fill: %v", err)
	}

	if _, err := a.AttachDocument(id, "media-last"); err != nil {
		t.Fatalf("reattach on a full record: %v, want success", err)
	}
	if got := countLiveDocs(t, db, id); got != int64(MaxDocuments) {
		t.Errorf("live rows = %d, want %d", got, MaxDocuments)
	}
}

func TestAttachDocument_missingRecordIsNotFound(t *testing.T) {
	db := newTestDB(t)
	if _, err := NewAdministrator(db).AttachDocument("no-such-record", "media-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestAttachDocument_softDeletedRecordIsNotFound(t *testing.T) {
	db := newTestDB(t)
	a := NewAdministrator(db)
	id := insertRecord(t, a, "v1", "cat-1", 5, 0)
	if err := a.SoftDelete(id); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	if _, err := a.AttachDocument(id, "media-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// FR-DET-2: the row is soft-deleted, not removed. admin/visibility_document_test
// already depends on a stamped row being invisible to readers.
func TestDetachDocument_soft_deletesTheRowAndLeavesItPresent(t *testing.T) {
	db := newTestDB(t)
	a := NewAdministrator(db)
	id := insertRecord(t, a, "v1", "cat-1", 5, 0)
	if _, err := a.AttachDocument(id, "media-1"); err != nil {
		t.Fatalf("attach: %v", err)
	}

	if err := a.DetachDocument(id, "media-1"); err != nil {
		t.Fatalf("detach: %v", err)
	}
	if got := countLiveDocs(t, db, id); got != 0 {
		t.Errorf("live rows = %d, want 0", got)
	}
	var row DocumentEntity
	if err := db.Where("maintenance_record_id = ? AND media_id = ?", id, "media-1").
		First(&row).Error; err != nil {
		t.Fatalf("the row must still exist: %v", err)
	}
	if row.DeletedAt == nil {
		t.Error("deleted_at was not stamped")
	}
	// And it must be re-attachable, which is the partial index's whole point.
	if _, err := a.AttachDocument(id, "media-1"); err != nil {
		t.Fatalf("reattach after detach: %v", err)
	}
}

// FR-DET-4: never attached and already detached collapse to the same answer.
func TestDetachDocument_unattachedMediaIsNotFound(t *testing.T) {
	db := newTestDB(t)
	a := NewAdministrator(db)
	id := insertRecord(t, a, "v1", "cat-1", 5, 0)

	if err := a.DetachDocument(id, "never-attached"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}

	if _, err := a.AttachDocument(id, "media-1"); err != nil {
		t.Fatalf("attach: %v", err)
	}
	if err := a.DetachDocument(id, "media-1"); err != nil {
		t.Fatalf("first detach: %v", err)
	}
	if err := a.DetachDocument(id, "media-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second detach err = %v, want ErrNotFound", err)
	}
}

// Detach must not reach across records.
func TestDetachDocument_doesNotDetachFromAnotherRecord(t *testing.T) {
	db := newTestDB(t)
	a := NewAdministrator(db)
	mine := insertRecord(t, a, "v1", "cat-1", 5, 0)
	theirs := insertRecord(t, a, "v1", "cat-1", 6, 0)
	if _, err := a.AttachDocument(theirs, "media-1"); err != nil {
		t.Fatalf("attach: %v", err)
	}

	if err := a.DetachDocument(mine, "media-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if got := countLiveDocs(t, db, theirs); got != 1 {
		t.Errorf("other record's live rows = %d, want 1 (untouched)", got)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./apps/fleet-service/internal/maintenancerecord/ -run 'TestAttachDocument|TestDetachDocument' -v`
Expected: FAIL — `a.AttachDocument undefined (type Administrator has no field or method AttachDocument)`.

- [ ] **Step 3: Extend the `Administrator` interface**

In `administrator.go`, replace the interface (lines 11-20) with:

```go
// Administrator is the write interface for maintenance record data access.
// All mutations (insert, update, soft-delete) go here.
type Administrator interface {
	Insert(Model) (Model, error)
	Update(Model) (Model, error)
	SoftDelete(id string) error
	// InsertTx inserts a record (and its document rows) using the supplied
	// transaction handle, so cross-domain orchestrations (completion flow,
	// design §10.3) can wrap it in a single db.Transaction.
	InsertTx(tx *gorm.DB, m Model) (Model, error)
	// AttachDocument adds one media reference to an existing record and
	// returns the re-read model. Deliberately NOT folded into Update: the
	// column allow-list in Update is the mechanism that keeps PATCH from
	// touching documents, and threading documents through it would put that
	// guarantee at the mercy of a future edit to the map (design D-1).
	AttachDocument(recordID, mediaID string) (Model, error)
	// DetachDocument soft-deletes one media reference from a record.
	DetachDocument(recordID, mediaID string) error
}
```

- [ ] **Step 4: Implement `AttachDocument` and `DetachDocument`**

Append to `administrator.go`, and extend its import block to `errors`, `time`, `github.com/google/uuid`, `gorm.io/gorm`, `gorm.io/gorm/clause`, `github.com/jtumidanski/myfleet/packages/shared-go/server`:

```go
// AttachDocument attaches one media reference to a record, enforcing the
// per-record cap and idempotency inside a single transaction.
//
// The parent row is locked FOR UPDATE, and that is load-bearing, not
// decoration. Read-then-insert inside a transaction is NOT sufficient at
// Postgres's default READ COMMITTED isolation: two concurrent attaches to a
// record holding nine documents each count nine, neither sees the other's
// uncommitted insert, and the record ends with eleven. A transaction is not a
// lock. Locking the record serializes attaches to it, so the second
// transaction blocks until the first commits, then counts ten and gets its
// validation error. Locking the record (not the document table) also cleanly
// excludes a concurrent soft-delete of the record itself.
//
// The lock is dialect-guarded because this package's tests run on sqlite,
// which rejects FOR UPDATE as a syntax error. Skipping it there is safe:
// sqlite serializes writers at the database level, so the correctness the lock
// buys exists for free. Branching on the dialect for a gap like this is the
// same thing mediavariant.ApplyPartialIndexes already does.
func (a *dbAdministrator) AttachDocument(recordID, mediaID string) (Model, error) {
	var out Model
	err := a.db.Transaction(func(tx *gorm.DB) error {
		q := tx.Where("id = ? AND deleted_at IS NULL", recordID)
		if tx.Name() != "sqlite" {
			q = q.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		var locked Entity
		if err := q.First(&locked).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}

		// Idempotency BEFORE the cap check, deliberately: re-attaching an id a
		// full record already holds must succeed, or a retry of an attach that
		// actually landed would be punished with a 422.
		var already int64
		if err := tx.Model(&DocumentEntity{}).
			Where("maintenance_record_id = ? AND media_id = ? AND deleted_at IS NULL", recordID, mediaID).
			Count(&already).Error; err != nil {
			return err
		}
		if already == 0 {
			var live int64
			if err := tx.Model(&DocumentEntity{}).
				Where("maintenance_record_id = ? AND deleted_at IS NULL", recordID).
				Count(&live).Error; err != nil {
				return err
			}
			if live >= MaxDocuments {
				return server.ErrValidation
			}
			d := DocumentEntity{ID: uuid.NewString(), MaintenanceRecordID: recordID, MediaID: mediaID}
			if err := tx.Create(&d).Error; err != nil {
				return err
			}
		}

		// Re-read, for the same reason Update does: a model built from the
		// in-memory entity is how a response comes to disagree with the row.
		var stored Entity
		if err := tx.Where("id = ? AND deleted_at IS NULL", recordID).First(&stored).Error; err != nil {
			return err
		}
		var docs []DocumentEntity
		if err := tx.Where("maintenance_record_id = ? AND deleted_at IS NULL", recordID).
			Find(&docs).Error; err != nil {
			return err
		}
		out = Make(stored, docs)
		return nil
	})
	if err != nil {
		return Model{}, err
	}
	return out, nil
}

// DetachDocument stamps deleted_at on one live document row.
//
// Soft, not hard: every other delete in fleet-service is soft, and
// admin/visibility_document_test already asserts that a stamped row is
// invisible on both the detail and the list path.
//
// No transaction: it is one statement on one row. RowsAffected == 0 covers
// "never attached" and "already detached" with the same answer, which is also
// what keeps the response from confirming that a media id exists elsewhere.
//
// It deliberately does not verify the record exists — the resource layer's
// GetByID already did, and repeating it here would be a second round-trip for
// an invariant the handler holds.
func (a *dbAdministrator) DetachDocument(recordID, mediaID string) error {
	res := a.db.Model(&DocumentEntity{}).
		Where("maintenance_record_id = ? AND media_id = ? AND deleted_at IS NULL", recordID, mediaID).
		Update("deleted_at", time.Now().UTC())
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./apps/fleet-service/internal/maintenancerecord/ -v`
Expected: PASS, all tests in the package.

- [ ] **Step 6: Commit**

```bash
git add apps/fleet-service/internal/maintenancerecord/administrator.go \
        apps/fleet-service/internal/maintenancerecord/administrator_test.go
git commit -m "feat(fleet): attach and detach maintenance record documents"
```

---

### Task 3: `Processor` pass-throughs

**Files:**
- Modify: `apps/fleet-service/internal/maintenancerecord/processor.go` (append after `SoftDelete`)
- Test: `apps/fleet-service/internal/maintenancerecord/processor_test.go` (create)

**Interfaces:**
- Consumes: `Administrator.AttachDocument` / `DetachDocument` (Task 2).
- Produces: `func (pr *Processor) AttachDocument(recordID, mediaID string) (Model, error)` and `func (pr *Processor) DetachDocument(recordID, mediaID string) error`, both translating the package-local `ErrNotFound` to `server.ErrNotFound` and passing `server.ErrValidation` through untouched.

- [ ] **Step 1: Write the failing tests**

Create `apps/fleet-service/internal/maintenancerecord/processor_test.go`:

```go
package maintenancerecord

import (
	"errors"
	"io"
	"testing"

	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

func newTestProcessor(t *testing.T) (*Processor, string) {
	t.Helper()
	db := newTestDB(t)
	log := logrus.New()
	log.SetOutput(io.Discard)
	a := NewAdministrator(db)
	id := insertRecord(t, a, "v1", "cat-1", 5, 0)
	return NewProcessor(log, NewProvider(db), a), id
}

// The data-access layer's ErrNotFound is a package-local sentinel. Handlers
// only understand server.ErrNotFound; the translation is this layer's job and
// is the reason it exists at all for these two methods.
func TestProcessorAttachDocument_translatesNotFound(t *testing.T) {
	pr, _ := newTestProcessor(t)

	if _, err := pr.AttachDocument("no-such-record", "media-1"); !errors.Is(err, server.ErrNotFound) {
		t.Fatalf("err = %v, want server.ErrNotFound", err)
	}
}

// The cap error is already a server sentinel; it must arrive unchanged so
// StatusFor maps it to 422 rather than being swallowed as a 404 or a 500.
func TestProcessorAttachDocument_passesValidationThrough(t *testing.T) {
	db := newTestDB(t)
	log := logrus.New()
	log.SetOutput(io.Discard)
	a := NewAdministrator(db)
	id := insertRecord(t, a, "v1", "cat-1", 5, MaxDocuments)
	pr := NewProcessor(log, NewProvider(db), a)

	if _, err := pr.AttachDocument(id, "one-too-many"); !errors.Is(err, server.ErrValidation) {
		t.Fatalf("err = %v, want server.ErrValidation", err)
	}
}

func TestProcessorAttachDocument_returnsTheUpdatedModel(t *testing.T) {
	pr, id := newTestProcessor(t)

	m, err := pr.AttachDocument(id, "media-1")
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	if got := m.DocumentMediaIDs(); len(got) != 1 || got[0] != "media-1" {
		t.Errorf("DocumentMediaIDs() = %v, want [media-1]", got)
	}
}

func TestProcessorDetachDocument_translatesNotFound(t *testing.T) {
	pr, id := newTestProcessor(t)

	if err := pr.DetachDocument(id, "never-attached"); !errors.Is(err, server.ErrNotFound) {
		t.Fatalf("err = %v, want server.ErrNotFound", err)
	}
}

func TestProcessorDetachDocument_succeedsForALiveAttachment(t *testing.T) {
	pr, id := newTestProcessor(t)
	if _, err := pr.AttachDocument(id, "media-1"); err != nil {
		t.Fatalf("attach: %v", err)
	}

	if err := pr.DetachDocument(id, "media-1"); err != nil {
		t.Fatalf("detach: %v", err)
	}
	m, err := pr.GetByID(id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(m.DocumentMediaIDs()) != 0 {
		t.Errorf("DocumentMediaIDs() = %v, want empty", m.DocumentMediaIDs())
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./apps/fleet-service/internal/maintenancerecord/ -run 'TestProcessorAttachDocument|TestProcessorDetachDocument' -v`
Expected: FAIL — `pr.AttachDocument undefined`.

- [ ] **Step 3: Implement the pass-throughs**

Append to `processor.go`:

```go
// AttachDocument attaches one media reference to a record.
//
// No validation lives here. Validate speaks about the model's fields, and an
// attach mutates rows rather than the model, so there is nothing for it to say
// — which is also why this does not go through Update.
func (pr *Processor) AttachDocument(recordID, mediaID string) (Model, error) {
	m, err := pr.a.AttachDocument(recordID, mediaID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Model{}, server.ErrNotFound
		}
		// server.ErrValidation from the cap check passes straight through;
		// StatusFor maps it to 422.
		return Model{}, err
	}
	return m, nil
}

// DetachDocument removes one media reference from a record.
func (pr *Processor) DetachDocument(recordID, mediaID string) error {
	err := pr.a.DetachDocument(recordID, mediaID)
	if errors.Is(err, ErrNotFound) {
		return server.ErrNotFound
	}
	return err
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./apps/fleet-service/internal/maintenancerecord/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/fleet-service/internal/maintenancerecord/processor.go \
        apps/fleet-service/internal/maintenancerecord/processor_test.go
git commit -m "feat(fleet): processor attach/detach document pass-throughs"
```

---

### Task 4: The attach and detach routes

This is the task that fixes the live 404: `MaintenanceRecordService.appendDocumentMedia` has been POSTing to a route that does not exist.

**Files:**
- Modify: `apps/fleet-service/internal/maintenancerecord/resource.go` (add two routes inside the existing `InitializeRoutes` closure, after the `DELETE /maintenance-records/{id}` route)
- Test: `apps/fleet-service/internal/maintenancerecord/resource_test.go` (append; also extend `newRecordRouter`)

**Interfaces:**
- Consumes: `Processor.AttachDocument` / `DetachDocument` (Task 3); the existing `DocumentValidator` interface (`resource.go:41`), `authz.RequireSameFleet`, `authz.RequireWrite`, `server.RegisterInputHandler`, `Transform`.
- Produces: HTTP routes `POST /maintenance-records/{id}/document-media` and `DELETE /maintenance-records/{id}/document-media/{mediaId}`. Test helper `func newRecordRouterWithDocs(t *testing.T, docs DocumentValidator) (chi.Router, *gorm.DB)`; `newRecordRouter(t)` keeps its existing signature and delegates with a `nil` validator.

- [ ] **Step 1: Write the failing tests**

First, in `resource_test.go`, replace the existing `newRecordRouter` (lines 39-52) with:

```go
func newRecordRouterWithDocs(t *testing.T, docs DocumentValidator) (chi.Router, *gorm.DB) {
	t.Helper()
	db := newTestDB(t)
	log := logrus.New()
	log.SetOutput(io.Discard)

	r := chi.NewRouter()
	r.Group(InitializeRoutes(log, db, stubVehicles{map[string]string{"v1": "f1"}}, stubCategories{}, docs))
	return r, db
}

func newRecordRouter(t *testing.T) (chi.Router, *gorm.DB) {
	t.Helper()
	// nil DocumentValidator is explicitly legal (see DocumentValidator's doc
	// comment); these tests attach no documents.
	return newRecordRouterWithDocs(t, nil)
}
```

Then append the new tests:

```go
// stubDocs satisfies DocumentValidator. err is what ValidateOwnership returns;
// calls records every id set it was asked about, so a test can prove the check
// ran BEFORE anything was written rather than after.
type stubDocs struct {
	err   error
	calls [][]string
}

func (s *stubDocs) ValidateOwnership(_ context.Context, _ string, mediaIDs []string) error {
	s.calls = append(s.calls, mediaIDs)
	return s.err
}

func attachDoc(r chi.Router, id, mediaID string, ident auth.Identity) *httptest.ResponseRecorder {
	return serveAs(r, http.MethodPost, "/maintenance-records/"+id+"/document-media",
		`{"data":{"type":"mediaRefs","attributes":{"mediaId":"`+mediaID+`"}}}`, ident)
}

func detachDoc(r chi.Router, id, mediaID string, ident auth.Identity) *httptest.ResponseRecorder {
	return serveAs(r, http.MethodDelete,
		"/maintenance-records/"+id+"/document-media/"+mediaID, "", ident)
}

// documentIDs decodes documentMediaIds out of a maintenanceRecords response.
func documentIDs(t *testing.T, rec *httptest.ResponseRecorder) []string {
	t.Helper()
	var env struct {
		Data struct {
			Attributes struct {
				DocumentMediaIDs []string `json:"documentMediaIds"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return env.Data.Attributes.DocumentMediaIDs
}

// This is the defect the branch fixes at the layer it lived in: the route did
// not exist, so the frontend's append call 404'd and left a confirmed media
// object referenced by nothing.
func TestAttachDocument_returns201WithTheUpdatedRecord(t *testing.T) {
	docs := &stubDocs{}
	r, db := newRecordRouterWithDocs(t, docs)
	recID := insertRecord(t, NewAdministrator(db), "v1", "cat-1", 5, 0)

	rec := attachDoc(r, recID, "media-1", identity("owner", "f1"))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	if got := documentIDs(t, rec); len(got) != 1 || got[0] != "media-1" {
		t.Errorf("documentMediaIds = %v, want [media-1]", got)
	}
	// Read the row back rather than trusting the body.
	stored, err := NewProvider(db).GetByID(recID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(stored.DocumentMediaIDs()) != 1 {
		t.Errorf("stored documents = %v, want one", stored.DocumentMediaIDs())
	}
}

// FR-ATT-6: ownership must be proven BEFORE any write, so a rejection leaves
// nothing to roll back and cross-fleet media can never be grafted on.
func TestAttachDocument_rejectsMediaThatFailsOwnershipAndWritesNothing(t *testing.T) {
	docs := &stubDocs{err: server.ErrValidation}
	r, db := newRecordRouterWithDocs(t, docs)
	recID := insertRecord(t, NewAdministrator(db), "v1", "cat-1", 5, 0)

	rec := attachDoc(r, recID, "someone-elses-media", identity("owner", "f1"))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	if len(docs.calls) != 1 || len(docs.calls[0]) != 1 || docs.calls[0][0] != "someone-elses-media" {
		t.Errorf("ValidateOwnership calls = %v, want one call with the single id", docs.calls)
	}
	stored, err := NewProvider(db).GetByID(recID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(stored.DocumentMediaIDs()) != 0 {
		t.Errorf("a rejected attach wrote %v", stored.DocumentMediaIDs())
	}
}

// FR-ATT-3: a blank id must never reach media-service as `?ids=`.
func TestAttachDocument_emptyMediaIDIs422AndMakesNoOwnershipCall(t *testing.T) {
	docs := &stubDocs{}
	r, db := newRecordRouterWithDocs(t, docs)
	recID := insertRecord(t, NewAdministrator(db), "v1", "cat-1", 5, 0)

	rec := attachDoc(r, recID, "", identity("owner", "f1"))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	if len(docs.calls) != 0 {
		t.Errorf("ValidateOwnership was called with %v; a blank id must not reach media-service", docs.calls)
	}
}

// FR-ATT-8: the drawer's sequential loop retries after a partial failure.
func TestAttachDocument_isIdempotent(t *testing.T) {
	r, db := newRecordRouterWithDocs(t, &stubDocs{})
	recID := insertRecord(t, NewAdministrator(db), "v1", "cat-1", 5, 0)
	ident := identity("owner", "f1")

	if rec := attachDoc(r, recID, "media-1", ident); rec.Code != http.StatusCreated {
		t.Fatalf("first attach status = %d, want 201", rec.Code)
	}
	rec := attachDoc(r, recID, "media-1", ident)
	if rec.Code != http.StatusCreated {
		t.Fatalf("second attach status = %d, want 201", rec.Code)
	}
	if got := documentIDs(t, rec); len(got) != 1 {
		t.Errorf("documentMediaIds = %v, want the id exactly once", got)
	}
}

// FR-ATT-7 at the HTTP layer: the cap answers 422, not 500.
func TestAttachDocument_atTheCapIs422(t *testing.T) {
	r, db := newRecordRouterWithDocs(t, &stubDocs{})
	recID := insertRecord(t, NewAdministrator(db), "v1", "cat-1", 5, MaxDocuments)

	if rec := attachDoc(r, recID, "one-too-many", identity("owner", "f1")); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
}

func TestAttachDocument_viewerIsForbidden(t *testing.T) {
	docs := &stubDocs{}
	r, db := newRecordRouterWithDocs(t, docs)
	recID := insertRecord(t, NewAdministrator(db), "v1", "cat-1", 5, 0)

	if rec := attachDoc(r, recID, "media-1", identity("viewer", "f1")); rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if len(docs.calls) != 0 {
		t.Error("a forbidden call reached media-service")
	}
	stored, _ := NewProvider(db).GetByID(recID)
	if len(stored.DocumentMediaIDs()) != 0 {
		t.Error("a forbidden call mutated the row")
	}
}

// Cross-fleet is 404, NOT 403: authz.RequireSameFleet returns ErrNotFound so
// cross-fleet existence is never leaked. Same as TestPatch_otherFleetIsNotFound.
func TestAttachDocument_otherFleetIsNotFound(t *testing.T) {
	r, db := newRecordRouterWithDocs(t, &stubDocs{})
	recID := insertRecord(t, NewAdministrator(db), "v1", "cat-1", 5, 0)

	if rec := attachDoc(r, recID, "media-1", identity("owner", "f2")); rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestAttachDocument_softDeletedRecordIsNotFound(t *testing.T) {
	r, db := newRecordRouterWithDocs(t, &stubDocs{})
	a := NewAdministrator(db)
	recID := insertRecord(t, a, "v1", "cat-1", 5, 0)
	if err := a.SoftDelete(recID); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	if rec := attachDoc(r, recID, "media-1", identity("owner", "f1")); rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

// FR-DET-1: 204, and the id is gone from the next GET.
func TestDetachDocument_returns204AndTheIDIsGoneFromTheNextGet(t *testing.T) {
	r, db := newRecordRouterWithDocs(t, &stubDocs{})
	recID := insertRecord(t, NewAdministrator(db), "v1", "cat-1", 5, 0)
	ident := identity("owner", "f1")
	if rec := attachDoc(r, recID, "media-1", ident); rec.Code != http.StatusCreated {
		t.Fatalf("attach status = %d", rec.Code)
	}

	if rec := detachDoc(r, recID, "media-1", ident); rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}

	got := serveAs(r, http.MethodGet, "/maintenance-records/"+recID, "", ident)
	if got.Code != http.StatusOK {
		t.Fatalf("get status = %d", got.Code)
	}
	if ids := documentIDs(t, got); len(ids) != 0 {
		t.Errorf("documentMediaIds = %v, want empty", ids)
	}
}

// The list path groups documents separately from the detail path, so it needs
// its own assertion — task-025's attachment-count column reads from it.
func TestDetachDocument_theIDIsGoneFromTheListPathToo(t *testing.T) {
	r, db := newRecordRouterWithDocs(t, &stubDocs{})
	recID := insertRecord(t, NewAdministrator(db), "v1", "cat-1", 5, 0)
	ident := identity("owner", "f1")
	if rec := attachDoc(r, recID, "media-1", ident); rec.Code != http.StatusCreated {
		t.Fatalf("attach status = %d", rec.Code)
	}
	if rec := detachDoc(r, recID, "media-1", ident); rec.Code != http.StatusNoContent {
		t.Fatalf("detach status = %d", rec.Code)
	}

	rec := serveAs(r, http.MethodGet, "/vehicles/v1/maintenance-records", "", ident)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d", rec.Code)
	}
	var env struct {
		Data []struct {
			Attributes struct {
				DocumentMediaIDs []string `json:"documentMediaIds"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(env.Data) != 1 {
		t.Fatalf("list returned %d records, want 1", len(env.Data))
	}
	if ids := env.Data[0].Attributes.DocumentMediaIDs; len(ids) != 0 {
		t.Errorf("list documentMediaIds = %v, want empty", ids)
	}
}

// FR-DET-4: never attached and already detached are indistinguishable.
func TestDetachDocument_unattachedIsNotFound(t *testing.T) {
	r, db := newRecordRouterWithDocs(t, &stubDocs{})
	recID := insertRecord(t, NewAdministrator(db), "v1", "cat-1", 5, 0)
	ident := identity("owner", "f1")

	if rec := detachDoc(r, recID, "never-attached", ident); rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}

	if rec := attachDoc(r, recID, "media-1", ident); rec.Code != http.StatusCreated {
		t.Fatalf("attach status = %d", rec.Code)
	}
	if rec := detachDoc(r, recID, "media-1", ident); rec.Code != http.StatusNoContent {
		t.Fatalf("first detach status = %d", rec.Code)
	}
	if rec := detachDoc(r, recID, "media-1", ident); rec.Code != http.StatusNotFound {
		t.Fatalf("second detach status = %d, want 404", rec.Code)
	}
}

func TestDetachDocument_viewerIsForbidden(t *testing.T) {
	r, db := newRecordRouterWithDocs(t, &stubDocs{})
	recID := insertRecord(t, NewAdministrator(db), "v1", "cat-1", 5, 0)
	if rec := attachDoc(r, recID, "media-1", identity("owner", "f1")); rec.Code != http.StatusCreated {
		t.Fatalf("attach status = %d", rec.Code)
	}

	if rec := detachDoc(r, recID, "media-1", identity("viewer", "f1")); rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	stored, _ := NewProvider(db).GetByID(recID)
	if len(stored.DocumentMediaIDs()) != 1 {
		t.Error("a forbidden call detached the document")
	}
}

func TestDetachDocument_otherFleetIsNotFound(t *testing.T) {
	r, db := newRecordRouterWithDocs(t, &stubDocs{})
	recID := insertRecord(t, NewAdministrator(db), "v1", "cat-1", 5, 0)
	if rec := attachDoc(r, recID, "media-1", identity("owner", "f1")); rec.Code != http.StatusCreated {
		t.Fatalf("attach status = %d", rec.Code)
	}

	if rec := detachDoc(r, recID, "media-1", identity("owner", "f2")); rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

// Maintenance and modification are the same entity distinguished by the
// category's kind, and the routes must not care which it is. The handler never
// consults the category, so this pins that it stays that way.
func TestAttachAndDetach_behaveIdenticallyForAModificationKindRecord(t *testing.T) {
	r, db := newRecordRouterWithDocs(t, &stubDocs{})
	recID := insertRecord(t, NewAdministrator(db), "v1", "mod-category", 5, 0)
	ident := identity("owner", "f1")

	if rec := attachDoc(r, recID, "media-1", ident); rec.Code != http.StatusCreated {
		t.Fatalf("attach status = %d, want 201", rec.Code)
	}
	if rec := detachDoc(r, recID, "media-1", ident); rec.Code != http.StatusNoContent {
		t.Fatalf("detach status = %d, want 204", rec.Code)
	}
}

// PRD D2: PATCH is a pure field-patch. It accepts no documentMediaIds and must
// leave document rows entirely alone. This is the invariant that decision
// exists to protect, so it gets its own test.
func TestPatch_leavesDocumentRowsUntouched(t *testing.T) {
	r, db := newRecordRouterWithDocs(t, &stubDocs{})
	recID := insertRecord(t, NewAdministrator(db), "v1", "cat-1", 5, 0)
	ident := identity("owner", "f1")
	if rec := attachDoc(r, recID, "media-1", ident); rec.Code != http.StatusCreated {
		t.Fatalf("attach status = %d", rec.Code)
	}

	rec := patchRecord(r, recID, `{"vendor":"Corner Garage","documentMediaIds":[]}`, ident)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch status = %d, want 200", rec.Code)
	}

	stored, err := NewProvider(db).GetByID(recID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got := stored.DocumentMediaIDs(); len(got) != 1 || got[0] != "media-1" {
		t.Errorf("documents = %v, want [media-1] — PATCH must not touch them", got)
	}
	if stored.Vendor() != "Corner Garage" {
		t.Errorf("Vendor() = %q; the patch itself should still have applied", stored.Vendor())
	}
}
```

Add `"context"` to `resource_test.go`'s import block (it currently imports `encoding/json`, `io`, `net/http`, `net/http/httptest`, `strings`, `testing`, chi, logrus, gorm, auth, server, maintenancecategory, vehicle).

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./apps/fleet-service/internal/maintenancerecord/ -run 'TestAttachDocument|TestDetachDocument|TestAttachAndDetach|TestPatch_leavesDocumentRowsUntouched' -v`
Expected: FAIL — attach/detach return `405 Method Not Allowed` or `404` because the routes are not registered.

- [ ] **Step 3: Register the two routes**

In `resource.go`, inside the `return func(r chi.Router) {` closure, immediately after the closing `})` of the `DELETE /maintenance-records/{id}` route and before the closure's final `}`, add:

```go
		// POST /maintenance-records/{id}/document-media — attach one
		// already-uploaded media object to an existing record.
		//
		// The frontend has been calling this route since task-004; it simply
		// did not exist, so every edit-time attach 404'd and stranded a
		// confirmed media object. The request shape below is the one the
		// client already sends — the server is built to fit the client here,
		// not the other way round (PRD D1).
		r.Post("/maintenance-records/{id}/document-media", server.RegisterInputHandler(func(w http.ResponseWriter, req *http.Request, attrs struct {
			MediaID string `json:"mediaId"`
		},
		) {
			identity := auth.IdentityFromContext(req.Context())
			id := chi.URLParam(req, "id")
			current, err := proc.GetByID(id)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			v, err := vehicleAccessor.GetByID(current.VehicleID())
			if err != nil {
				server.WriteError(w, err)
				return
			}
			if err := authz.RequireSameFleet(identity, v.FleetID()); err != nil {
				server.WriteError(w, err)
				return
			}
			if err := authz.RequireWrite(identity); err != nil {
				server.WriteError(w, err)
				return
			}

			// Rejected before the ownership call so a blank id never reaches
			// media-service as `?ids=`, which would be an unbounded lookup for
			// what is really a malformed request.
			if attrs.MediaID == "" {
				server.WriteError(w, server.ErrValidation)
				return
			}

			// Prove the attachment is the caller's own BEFORE anything is
			// written, so a rejection leaves nothing to roll back. Without
			// this, the route is a way to graft another fleet's media onto
			// your own record and then read it back through
			// GET /api/media/{id}/content.
			if docs != nil {
				if err := docs.ValidateOwnership(req.Context(), identity.ActiveFleetID, []string{attrs.MediaID}); err != nil {
					log.WithError(err).Warn("attachment ownership validation failed")
					server.WriteError(w, err)
					return
				}
			}

			updated, err := proc.AttachDocument(id, attrs.MediaID)
			if err != nil {
				log.WithError(err).Error("attach maintenance record document")
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusCreated, server.Document{Data: Transform(updated)})
		}))

		// DELETE /maintenance-records/{id}/document-media/{mediaId} — detach.
		//
		// This removes the REFERENCE only. Deleting the media object itself is
		// the client's job, issued against media-service with the user's own
		// JWT: mediaclient is the no-JWT internal client, and giving
		// media-service's /internal surface delete authority would create an
		// unauthenticated destructive endpoint whose only protection is a
		// Traefik rule (PRD D3).
		r.Delete("/maintenance-records/{id}/document-media/{mediaId}", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			id := chi.URLParam(req, "id")
			current, err := proc.GetByID(id)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			v, err := vehicleAccessor.GetByID(current.VehicleID())
			if err != nil {
				server.WriteError(w, err)
				return
			}
			if err := authz.RequireSameFleet(identity, v.FleetID()); err != nil {
				server.WriteError(w, err)
				return
			}
			if err := authz.RequireWrite(identity); err != nil {
				server.WriteError(w, err)
				return
			}
			if err := proc.DetachDocument(id, chi.URLParam(req, "mediaId")); err != nil {
				server.WriteError(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
```

Then update the route list in `InitializeRoutes`' doc comment (`resource.go:45-48`) so it mentions that `docs` validates attachment ownership on create **and on attach**.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./apps/fleet-service/internal/maintenancerecord/ -v`
Expected: PASS.

- [ ] **Step 5: Verify the whole Go side**

Run: `make vet && make test && make build`
Expected: all green. `make test` covers `apps/fleet-service/internal/admin` too, whose `visibility_document_test.go` depends on soft-deleted document rows staying invisible.

- [ ] **Step 6: Update the service-route comment in the frontend client**

In `apps/web/src/services/api/MaintenanceRecordService.ts`, extend the class doc comment's route list (lines 12-20) so it names the two new routes:

```
 *   POST   /api/fleet/maintenance-records/{id}/document-media            — attach one media object
 *   DELETE /api/fleet/maintenance-records/{id}/document-media/{mediaId}  — detach one media object
```

Also fix the stale `appendDocumentMedia` doc comment (line 49), which says `POST /api/fleet/maintenance-records/{id} with documentMediaIds append`:

```ts
  /** POST /api/fleet/maintenance-records/{id}/document-media */
```

- [ ] **Step 7: Commit**

```bash
git add apps/fleet-service/internal/maintenancerecord/resource.go \
        apps/fleet-service/internal/maintenancerecord/resource_test.go \
        apps/web/src/services/api/MaintenanceRecordService.ts
git commit -m "feat(fleet): attach and detach document-media routes"
```

At this point the live 404 is fixed: editing a record and adding a file works end to end, with no frontend change.

---

### Task 5: Client detach — service method, hook, and widened invalidation

**Files:**
- Modify: `apps/web/src/services/api/MaintenanceRecordService.ts` (add `removeDocumentMedia` after `appendDocumentMedia`)
- Modify: `apps/web/src/lib/hooks/api/maintenance.ts:205-215` (widen `useAppendMaintenanceRecordDocument`), then append `useRemoveMaintenanceRecordDocument`
- Test: `apps/web/src/lib/hooks/api/maintenance.test.tsx` (create — `.tsx`, not `.ts`: the file contains the JSX `QueryClientProvider` wrapper)

**Interfaces:**
- Consumes: the `DELETE .../document-media/{mediaId}` route (Task 4); existing `apiClient.request`, `mediaService.remove(id: string): Promise<void>`, `maintenanceRecordKeys`, `vehicleKeys`.
- Produces:
  - `MaintenanceRecordService.removeDocumentMedia(id: string, mediaId: string): Promise<void>`
  - `useRemoveMaintenanceRecordDocument(vehicleId: string)` — a `useMutation` whose variables are `{ id: string; mediaId: string }` and whose `mutationFn` resolves `void`.

- [ ] **Step 1: Write the failing tests**

Create `apps/web/src/lib/hooks/api/maintenance.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import React from 'react';
import {
  useRemoveMaintenanceRecordDocument,
  useAppendMaintenanceRecordDocument,
  maintenanceRecordKeys,
} from './maintenance';
import { maintenanceRecordService } from '../../../services/api/MaintenanceRecordService';
import { mediaService } from '../../../services/api/MediaService';

vi.mock('../../../services/api/MaintenanceRecordService', () => ({
  maintenanceRecordService: {
    removeDocumentMedia: vi.fn(),
    appendDocumentMedia: vi.fn(),
  },
}));
vi.mock('../../../services/api/MediaService', () => ({
  mediaService: { remove: vi.fn() },
}));

function makeWrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const invalidate = vi.spyOn(qc, 'invalidateQueries');
  const wrapper = ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
  return { wrapper, invalidate };
}

/** The keys invalidateQueries was called with, flattened for easy matching. */
function invalidatedKeys(spy: ReturnType<typeof vi.spyOn>): string[] {
  return spy.mock.calls.map((call) =>
    JSON.stringify((call[0] as { queryKey: unknown }).queryKey),
  );
}

describe('useRemoveMaintenanceRecordDocument', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(maintenanceRecordService.removeDocumentMedia).mockResolvedValue(undefined);
    vi.mocked(mediaService.remove).mockResolvedValue(undefined);
  });

  it('detaches the reference first, then deletes the media object', async () => {
    const order: string[] = [];
    vi.mocked(maintenanceRecordService.removeDocumentMedia).mockImplementation(async () => {
      order.push('detach');
    });
    vi.mocked(mediaService.remove).mockImplementation(async () => {
      order.push('delete');
    });
    const { wrapper } = makeWrapper();

    const { result } = renderHook(() => useRemoveMaintenanceRecordDocument('veh-1'), { wrapper });
    await result.current.mutateAsync({ id: 'rec-1', mediaId: 'media-1' });

    expect(maintenanceRecordService.removeDocumentMedia).toHaveBeenCalledWith('rec-1', 'media-1');
    expect(mediaService.remove).toHaveBeenCalledWith('media-1');
    expect(order).toEqual(['detach', 'delete']);
  });

  // The reference is what the user can see. If the object delete fails the UI
  // is already correct and the orphan is media-service's purge to reap, so
  // reporting a failure here would be a lie the user cannot act on.
  it('reports success when the media object delete fails', async () => {
    vi.mocked(mediaService.remove).mockRejectedValue(new Error('gone'));
    const { wrapper } = makeWrapper();

    const { result } = renderHook(() => useRemoveMaintenanceRecordDocument('veh-1'), { wrapper });

    await expect(
      result.current.mutateAsync({ id: 'rec-1', mediaId: 'media-1' }),
    ).resolves.toBeUndefined();
  });

  it('does not delete the media object when the detach itself fails', async () => {
    vi.mocked(maintenanceRecordService.removeDocumentMedia).mockRejectedValue(new Error('404'));
    const { wrapper } = makeWrapper();

    const { result } = renderHook(() => useRemoveMaintenanceRecordDocument('veh-1'), { wrapper });

    await expect(
      result.current.mutateAsync({ id: 'rec-1', mediaId: 'media-1' }),
    ).rejects.toThrow();
    expect(mediaService.remove).not.toHaveBeenCalled();
  });

  it('invalidates the record detail and the record lists', async () => {
    const { wrapper, invalidate } = makeWrapper();

    const { result } = renderHook(() => useRemoveMaintenanceRecordDocument('veh-1'), { wrapper });
    await result.current.mutateAsync({ id: 'rec-1', mediaId: 'media-1' });

    await waitFor(() => {
      const keys = invalidatedKeys(invalidate);
      expect(keys).toContain(JSON.stringify(maintenanceRecordKeys.detail('rec-1')));
      expect(keys).toContain(JSON.stringify(maintenanceRecordKeys.lists()));
    });
  });
});

describe('useAppendMaintenanceRecordDocument', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(maintenanceRecordService.appendDocumentMedia).mockResolvedValue({
      id: 'rec-1',
      type: 'maintenanceRecords',
      attributes: {},
    } as never);
  });

  // The records table renders its attachment-count affordance from the LIST
  // payload's documentMediaIds. Without this invalidation, attaching a file
  // during an edit leaves that count stale until something else refetches.
  it('invalidates the record lists so the attachment count stays truthful', async () => {
    const { wrapper, invalidate } = makeWrapper();

    const { result } = renderHook(() => useAppendMaintenanceRecordDocument('veh-1'), { wrapper });
    await result.current.mutateAsync({ id: 'rec-1', mediaId: 'media-1' });

    await waitFor(() => {
      expect(invalidatedKeys(invalidate)).toContain(JSON.stringify(maintenanceRecordKeys.lists()));
    });
  });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd apps/web && npx vitest run src/lib/hooks/api/maintenance.test.tsx`
Expected: FAIL — `useRemoveMaintenanceRecordDocument is not a function`, and the append test fails because `lists()` is not invalidated.

(If `npx vitest` is not on `PATH`: `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22` first.)

- [ ] **Step 3: Add `removeDocumentMedia` to the service**

In `apps/web/src/services/api/MaintenanceRecordService.ts`, after `appendDocumentMedia`:

```ts
  /**
   * DELETE /api/fleet/maintenance-records/{id}/document-media/{mediaId}
   *
   * Removes the REFERENCE only. The media object itself is deleted separately
   * by the caller against media-service; fleet-service has no authority to do
   * it (PRD D3).
   */
  async removeDocumentMedia(id: string, mediaId: string): Promise<void> {
    await apiClient.request<null>(`${this.basePath}/${id}/document-media/${mediaId}`, {
      method: 'DELETE',
    });
  }
```

- [ ] **Step 4: Widen the append hook's invalidation and add the removal hook**

In `apps/web/src/lib/hooks/api/maintenance.ts`, add the media-service import to the import block at the top:

```ts
import { mediaService } from '../../../services/api/MediaService';
```

Replace `useAppendMaintenanceRecordDocument`'s `onSettled` (lines 210-213) with:

```ts
    onSettled: (_data, _error, variables) => {
      // lists() matters as much as detail(): the records table renders its
      // attachment-count affordance from the list payload's documentMediaIds,
      // so omitting it leaves that count stale after an edit-time attach.
      void queryClient.invalidateQueries({ queryKey: maintenanceRecordKeys.lists() });
      void queryClient.invalidateQueries({ queryKey: maintenanceRecordKeys.detail(variables.id) });
      void queryClient.invalidateQueries({ queryKey: vehicleKeys.detail(vehicleId) });
    },
```

Then append, immediately after that hook:

```ts
/**
 * DELETE /api/fleet/maintenance-records/{id}/document-media/{mediaId}, then
 * DELETE /api/media/{mediaId}.
 *
 * Two services own half of this each, and both halves are required:
 *   1. fleet-service holds the join row the drawer lists.
 *   2. media-service holds the object the bytes come from.
 *
 * Order matters. The reference goes first because it is the one the user can
 * see; if the object delete then fails, the drawer is still correct and the
 * orphan is media-service's purge_after sweep to reap. Doing it the other way
 * round would leave a listed reference pointing at bytes that are already
 * gone. That is also why the object delete is best-effort: a failure there is
 * invisible to the user and must not report the removal as failed. This is
 * useRemoveVehiclePhoto's shape, and the reasoning transfers verbatim.
 *
 * The swallowed error is logged rather than discarded — it is not worth a
 * toast, but it should not be invisible either.
 */
export function useRemoveMaintenanceRecordDocument(vehicleId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ id, mediaId }: { id: string; mediaId: string }) => {
      await maintenanceRecordService.removeDocumentMedia(id, mediaId);
      try {
        await mediaService.remove(mediaId);
      } catch (err) {
        console.warn('[attachments] detached reference removed, media delete failed:', err);
      }
    },
    onSettled: (_data, _error, variables) => {
      void queryClient.invalidateQueries({ queryKey: maintenanceRecordKeys.lists() });
      void queryClient.invalidateQueries({ queryKey: maintenanceRecordKeys.detail(variables.id) });
      void queryClient.invalidateQueries({ queryKey: vehicleKeys.detail(vehicleId) });
    },
  });
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd apps/web && npx vitest run src/lib/hooks/api/maintenance.test.tsx`
Expected: PASS, all six tests.

- [ ] **Step 6: Commit**

```bash
git add apps/web/src/services/api/MaintenanceRecordService.ts \
        apps/web/src/lib/hooks/api/maintenance.ts \
        apps/web/src/lib/hooks/api/maintenance.test.tsx
git commit -m "feat(web): detach hook and list invalidation for record attachments"
```

---

### Task 6: Capacity-aware picker on the edit path

Today `AttachmentPicker` counts only newly-picked files, so a record already holding eight attachments lets a user pick ten more and only learns better when the server rejects them.

**Files:**
- Modify: `apps/web/src/lib/hooks/usePendingAttachments.ts:42` (signature), `:62-63` (`room`), `:143` (`isFull`)
- Modify: `apps/web/src/components/features/vehicles/maintenance/AttachmentPicker.tsx`
- Modify: `apps/web/src/components/features/vehicles/maintenance/MaintenanceRecordForm.tsx`
- Modify: `apps/web/src/components/features/vehicles/detail/VehicleRecordDrawer.tsx` (edit-mode call site only)
- Test: `apps/web/src/lib/hooks/usePendingAttachments.test.ts` (append), `apps/web/src/components/features/vehicles/maintenance/AttachmentPicker.test.tsx` (create)

**Interfaces:**
- Consumes: `MAX_ATTACHMENTS`, `PendingAttachment` (unchanged).
- Produces:
  - `usePendingAttachments(existingCount?: number)` — `existingCount` defaults to `0`; return shape is unchanged except `isFull` now accounts for it.
  - `AttachmentPickerProps` gains `existingCount?: number` (default `0`).
  - `MaintenanceRecordFormProps` gains `existingAttachmentCount?: number` (default `0`).

- [ ] **Step 1: Write the failing hook test**

Append to `apps/web/src/lib/hooks/usePendingAttachments.test.ts`:

```tsx
  // The cap is per RECORD, not per picker session. Counting only newly-picked
  // files meant a record already holding eight let a user pick ten more and
  // find out from a 422.
  it('drops files that would exceed the combined existing-plus-pending cap', async () => {
    mockUploadSucceeds('m1');
    const existing = MAX_ATTACHMENTS - 2;
    const { result } = renderHook(() => usePendingAttachments(existing));

    act(() => {
      result.current.add([file('a.pdf'), file('b.pdf'), file('c.pdf'), file('d.pdf')]);
    });

    await waitFor(() => expect(result.current.items).toHaveLength(2));
    expect(result.current.items.map((i) => i.file.name)).toEqual(['a.pdf', 'b.pdf']);
  });

  it('reports isFull once existing plus pending reaches the cap', async () => {
    mockUploadSucceeds('m1');
    const { result } = renderHook(() => usePendingAttachments(MAX_ATTACHMENTS - 1));
    expect(result.current.isFull).toBe(false);

    act(() => {
      result.current.add([file('a.pdf')]);
    });

    await waitFor(() => expect(result.current.isFull).toBe(true));
  });

  it('accepts a full picker-load when the record has no existing attachments', async () => {
    mockUploadSucceeds('m1');
    const { result } = renderHook(() => usePendingAttachments());

    act(() => {
      result.current.add(Array.from({ length: MAX_ATTACHMENTS }, (_, i) => file(`f${i}.pdf`)));
    });

    await waitFor(() => expect(result.current.items).toHaveLength(MAX_ATTACHMENTS));
  });
```

Place these inside the existing `describe('usePendingAttachments', …)` block.

- [ ] **Step 2: Run it to verify it fails**

Run: `cd apps/web && npx vitest run src/lib/hooks/usePendingAttachments.test.ts`
Expected: FAIL — the first new test gets 4 items, not 2, because `existingCount` is ignored.

- [ ] **Step 3: Thread `existingCount` through the hook**

In `usePendingAttachments.ts`, change the signature and the two arithmetic sites.

Signature (line 42) — replace `export function usePendingAttachments() {` with:

```ts
/**
 * @param existingCount attachments the record ALREADY holds. The cap is per
 * record, so the picker's room is what is left after them. A snapshot taken
 * when the form renders: if an attach lands from another tab mid-edit this is
 * stale, which is exactly why the server cap stays authoritative and this is
 * only ergonomics.
 */
export function usePendingAttachments(existingCount = 0) {
```

`room` (line 62) — replace `const room = Math.max(MAX_ATTACHMENTS - itemsRef.current.length, 0);` with:

```ts
      const room = Math.max(MAX_ATTACHMENTS - existingCount - itemsRef.current.length, 0);
```

and add `existingCount` to the `add` callback's dependency array, so line 88's `[patch, write]` becomes `[existingCount, patch, write]`.

`isFull` (line 143) — replace `isFull: items.length >= MAX_ATTACHMENTS,` with:

```ts
    isFull: existingCount + items.length >= MAX_ATTACHMENTS,
```

- [ ] **Step 4: Run the hook test to verify it passes**

Run: `cd apps/web && npx vitest run src/lib/hooks/usePendingAttachments.test.ts`
Expected: PASS, including the pre-existing tests (they call `usePendingAttachments()` with no argument, which must still behave exactly as before).

- [ ] **Step 5: Write the failing picker test**

Create `apps/web/src/components/features/vehicles/maintenance/AttachmentPicker.test.tsx`:

```tsx
import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { AttachmentPicker } from './AttachmentPicker';
import { MAX_ATTACHMENTS, type PendingAttachment } from '../../../../lib/hooks/usePendingAttachments';

function pending(name: string): PendingAttachment {
  return {
    localId: name,
    file: new File(['x'], name, { type: 'application/pdf' }),
    status: 'ready',
    mediaId: `media-${name}`,
  };
}

const noop = vi.fn();

describe('AttachmentPicker', () => {
  it('keeps the create-flow helper text when the record has no attachments yet', () => {
    render(<AttachmentPicker items={[]} onAdd={noop} onRemove={noop} />);

    expect(screen.getByText(/PDF, image, Word, Excel or CSV/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /add files/i })).toBeEnabled();
  });

  // The whole point: the user must learn the remaining capacity BEFORE picking,
  // not from a 422 afterwards.
  it('reports remaining capacity against the existing attachments', () => {
    render(<AttachmentPicker items={[]} onAdd={noop} onRemove={noop} existingCount={3} />);

    expect(screen.getByText(`3 of ${MAX_ATTACHMENTS} attached. You can add 7 more.`)).toBeInTheDocument();
  });

  it('counts pending files against the same capacity', () => {
    render(
      <AttachmentPicker
        items={[pending('a.pdf'), pending('b.pdf')]}
        onAdd={noop}
        onRemove={noop}
        existingCount={3}
      />,
    );

    expect(screen.getByText(`3 of ${MAX_ATTACHMENTS} attached. You can add 5 more.`)).toBeInTheDocument();
  });

  it('disables adding and says the record is at the limit when existing plus pending fills it', () => {
    render(
      <AttachmentPicker
        items={[pending('a.pdf')]}
        onAdd={noop}
        onRemove={noop}
        existingCount={MAX_ATTACHMENTS - 1}
      />,
    );

    expect(screen.getByRole('button', { name: /add files/i })).toBeDisabled();
    expect(
      screen.getByText(`This record is at the ${MAX_ATTACHMENTS}-attachment limit.`),
    ).toBeInTheDocument();
  });

  it('keeps the create-flow at-limit copy when there are no existing attachments', () => {
    const items = Array.from({ length: MAX_ATTACHMENTS }, (_, i) => pending(`f${i}.pdf`));
    render(<AttachmentPicker items={items} onAdd={noop} onRemove={noop} />);

    expect(screen.getByRole('button', { name: /add files/i })).toBeDisabled();
    expect(
      screen.getByText(`Maximum ${MAX_ATTACHMENTS} attachments per record.`),
    ).toBeInTheDocument();
  });
});
```

- [ ] **Step 6: Run it to verify it fails**

Run: `cd apps/web && npx vitest run src/components/features/vehicles/maintenance/AttachmentPicker.test.tsx`
Expected: FAIL — `existingCount` is not a prop, so the capacity strings never render.

- [ ] **Step 7: Make the picker capacity-aware**

In `AttachmentPicker.tsx`, replace the props interface and the two lines that compute/render fullness.

Props interface:

```tsx
interface AttachmentPickerProps {
  items: PendingAttachment[];
  onAdd: (files: FileList | File[]) => void;
  onRemove: (localId: string) => void;
  /** Hides the picker for viewers (PRD FR-VIEW-5). */
  disabled?: boolean;
  /**
   * Attachments the record already holds. The cap is per record, so on the
   * edit path the picker's room is what is left after them. Defaults to 0, so
   * the create-flow call site is unaffected.
   *
   * A prop rather than a query: this is a presentational control inside a
   * form, and giving it a useMaintenanceRecord call would couple it to the
   * server cache and make it untestable without a QueryClient. The drawer
   * already holds the record.
   */
  existingCount?: number;
}
```

Replace the destructure and `isFull` line:

```tsx
export function AttachmentPicker({
  items,
  onAdd,
  onRemove,
  disabled,
  existingCount = 0,
}: AttachmentPickerProps) {
  const inputRef = useRef<HTMLInputElement>(null);
  const remaining = Math.max(MAX_ATTACHMENTS - existingCount - items.length, 0);
  const isFull = remaining === 0;

  // The server cap is authoritative (a client that ignores all of this still
  // gets a 422); this copy exists so the user finds out before picking rather
  // than after saving.
  let helperText: string;
  if (isFull) {
    helperText =
      existingCount > 0
        ? `This record is at the ${MAX_ATTACHMENTS}-attachment limit.`
        : `Maximum ${MAX_ATTACHMENTS} attachments per record.`;
  } else if (existingCount > 0) {
    helperText = `${existingCount} of ${MAX_ATTACHMENTS} attached. You can add ${remaining} more.`;
  } else {
    helperText = `PDF, image, Word, Excel or CSV. Up to ${formatUploadSize(MEDIA_MAX_UPLOAD_BYTES)} each, ${MAX_ATTACHMENTS} per record.`;
  }
```

Replace the helper-text paragraph (the `<p className="text-xs text-muted-foreground">…</p>` block) with:

```tsx
      <p className="text-xs text-muted-foreground">{helperText}</p>
```

Everything else in the component stays as it is.

- [ ] **Step 8: Run the picker test to verify it passes**

Run: `cd apps/web && npx vitest run src/components/features/vehicles/maintenance/AttachmentPicker.test.tsx`
Expected: PASS, all five tests.

- [ ] **Step 9: Thread the count through the form and the drawer**

In `MaintenanceRecordForm.tsx`, add to `MaintenanceRecordFormProps`:

```tsx
  /**
   * Attachments the record already holds, on the edit path. Threaded to both
   * the pending-upload hook and the picker so the ten-per-record cap counts
   * existing plus pending rather than pending alone. Defaults to 0 for the
   * create flow, where there is no record yet.
   */
  existingAttachmentCount?: number;
```

Add `existingAttachmentCount = 0,` to the destructured parameter list, and change the hook call:

```tsx
  const attachments = usePendingAttachments(existingAttachmentCount);
```

and the picker render:

```tsx
        <AttachmentPicker
          items={attachments.items}
          onAdd={attachments.add}
          onRemove={attachments.remove}
          existingCount={existingAttachmentCount}
        />
```

In `VehicleRecordDrawer.tsx`, in the edit-mode `<MaintenanceRecordForm …>` block, add the prop:

```tsx
          existingAttachmentCount={record.attributes.documentMediaIds?.length ?? 0}
```

- [ ] **Step 10: Run the frontend suite and typecheck**

Run: `cd apps/web && npx vitest run && npx tsc --noEmit -p tsconfig.json`
Expected: PASS / no type errors. `MaintenanceRecordForm.test.tsx` must still pass unchanged — the create flow passes no `existingAttachmentCount` and must behave exactly as before.

- [ ] **Step 11: Commit**

```bash
git add apps/web/src/lib/hooks/usePendingAttachments.ts \
        apps/web/src/lib/hooks/usePendingAttachments.test.ts \
        apps/web/src/components/features/vehicles/maintenance/AttachmentPicker.tsx \
        apps/web/src/components/features/vehicles/maintenance/AttachmentPicker.test.tsx \
        apps/web/src/components/features/vehicles/maintenance/MaintenanceRecordForm.tsx \
        apps/web/src/components/features/vehicles/detail/VehicleRecordDrawer.tsx
git commit -m "feat(web): count existing attachments against the per-record cap"
```

---

### Task 7: Remove control in the drawer

**Files:**
- Modify: `apps/web/src/components/features/vehicles/maintenance/RecordAttachmentList.tsx`
- Modify: `apps/web/src/components/features/vehicles/detail/VehicleRecordDrawer.tsx`
- Test: `apps/web/src/components/features/vehicles/maintenance/RecordAttachmentList.test.tsx` (append)

**Interfaces:**
- Consumes: `useRemoveMaintenanceRecordDocument(vehicleId)` from Task 5; `AlertDialog*` from `apps/web/src/components/ui/alert-dialog`.
- Produces: `RecordAttachmentList` props become `{ mediaIds: string[]; onRemove?: (mediaId: string) => void; canRemove?: boolean }`. `onRemove` is called with the media ID only.

- [ ] **Step 1: Write the failing tests**

Append to `apps/web/src/components/features/vehicles/maintenance/RecordAttachmentList.test.tsx`, inside the existing `describe('RecordAttachmentList', …)` block:

```tsx
  // Viewers keep read-only access: they see and download attachments and get
  // no remove control at all, rather than a control that fails late.
  it('omits the remove control when canRemove is false', async () => {
    vi.mocked(mediaService.get).mockResolvedValue(mediaResource('m1', 'application/pdf') as never);

    render(<RecordAttachmentList mediaIds={['m1']} onRemove={vi.fn()} canRemove={false} />, {
      wrapper,
    });

    await screen.findByRole('button', { name: /m1\.file/i });
    expect(screen.queryByRole('button', { name: /^remove/i })).not.toBeInTheDocument();
  });

  it('omits the remove control when no onRemove handler is supplied', async () => {
    vi.mocked(mediaService.get).mockResolvedValue(mediaResource('m1', 'application/pdf') as never);

    render(<RecordAttachmentList mediaIds={['m1']} canRemove />, { wrapper });

    await screen.findByRole('button', { name: /m1\.file/i });
    expect(screen.queryByRole('button', { name: /^remove/i })).not.toBeInTheDocument();
  });

  it('calls onRemove with the media id from a document row', async () => {
    const user = userEvent.setup();
    const onRemove = vi.fn();
    vi.mocked(mediaService.get).mockResolvedValue(mediaResource('m1', 'application/pdf') as never);

    render(<RecordAttachmentList mediaIds={['m1']} onRemove={onRemove} canRemove />, { wrapper });

    await user.click(await screen.findByRole('button', { name: 'Remove m1.file' }));

    expect(onRemove).toHaveBeenCalledWith('m1');
  });

  it('calls onRemove with the media id from an image row', async () => {
    const user = userEvent.setup();
    const onRemove = vi.fn();
    vi.mocked(mediaService.get).mockResolvedValue(mediaResource('m1', 'image/jpeg') as never);

    render(<RecordAttachmentList mediaIds={['m1']} onRemove={onRemove} canRemove />, { wrapper });

    await user.click(await screen.findByRole('button', { name: 'Remove m1.file' }));

    expect(onRemove).toHaveBeenCalledWith('m1');
  });

  // An unavailable attachment is exactly the one a user most wants to clear,
  // so the row that used to render nothing but a message must still offer it.
  it('offers removal on an unavailable attachment row', async () => {
    const user = userEvent.setup();
    const onRemove = vi.fn();
    vi.mocked(mediaService.get).mockRejectedValue(new Error('404'));

    render(<RecordAttachmentList mediaIds={['m1']} onRemove={onRemove} canRemove />, { wrapper });

    await screen.findByText(/attachment unavailable/i);
    await user.click(screen.getByRole('button', { name: 'Remove attachment' }));

    expect(onRemove).toHaveBeenCalledWith('m1');
  });
```

- [ ] **Step 2: Run them to verify they fail**

Run: `cd apps/web && npx vitest run src/components/features/vehicles/maintenance/RecordAttachmentList.test.tsx`
Expected: FAIL — TypeScript rejects the unknown `onRemove`/`canRemove` props and no remove buttons render.

- [ ] **Step 3: Add the remove control to all three row shapes**

Rewrite `RecordAttachmentList.tsx`'s `AttachmentRow` and the exported component. Change the lucide import to include `Trash2`:

```tsx
import { Download, FileText, Loader2, Trash2 } from 'lucide-react';
```

Then:

```tsx
interface AttachmentRowProps {
  mediaId: string;
  onRemove?: (mediaId: string) => void;
  canRemove?: boolean;
}

function AttachmentRow({ mediaId, onRemove, canRemove }: AttachmentRowProps) {
  const { data, isLoading, isError } = useMediaObject(mediaId);
  const [downloading, setDownloading] = useState(false);

  const removable = Boolean(canRemove && onRemove);
  const removeButton = (label: string) =>
    removable ? (
      <Button
        type="button"
        size="sm"
        variant="ghost"
        className="h-6 w-6 shrink-0 p-0"
        aria-label={label}
        onClick={() => onRemove?.(mediaId)}
      >
        <Trash2 className="h-4 w-4" aria-hidden="true" />
      </Button>
    ) : null;

  if (isLoading) {
    return <Skeleton className="h-10 w-full" />;
  }

  // Missing, soft-deleted, cross-fleet, or a terminal processing failure all
  // render the same explicit row rather than a broken control (PRD FR-VIEW-4).
  //
  // This row DOES offer removal, and the label is generic because there is no
  // filename to name — an unavailable attachment is exactly the one a user
  // most wants to clear.
  if (isError || !data || data.attributes.status === 'failed') {
    return (
      <div className="flex items-center gap-2 rounded-md border border-dashed px-2 py-1.5 text-xs text-muted-foreground">
        <span className="min-w-0 flex-1">Attachment unavailable</span>
        {removeButton('Remove attachment')}
      </div>
    );
  }

  const filename = data.attributes.originalFilename || mediaId;

  if (isRenderableImage(data.attributes.contentType)) {
    return (
      <div className="flex items-center gap-2 rounded-md border px-2 py-1.5">
        <MediaThumbnail mediaId={mediaId} className="h-12 w-12" />
        <span className="min-w-0 flex-1 truncate text-sm">{filename}</span>
        {removeButton(`Remove ${filename}`)}
      </div>
    );
  }

  const handleDownload = async () => {
    setDownloading(true);
    try {
      // Fetched through the authenticated API client: GET
      // /api/media/{id}/content needs an Authorization header, so a plain
      // <a href> cannot be used (PRD FR-VIEW-3).
      const blob = await mediaService.getContentBlob(mediaId);
      downloadBlob(blob, filename);
    } catch (err) {
      toast.error(createErrorFromUnknown(err).message || 'Could not download attachment');
    } finally {
      setDownloading(false);
    }
  };

  // The download affordance is itself a button, so the remove control cannot
  // be nested inside it — a wrapper is the only valid structure. It is added
  // only when removable, so the read-only row keeps its full-width hit area
  // exactly as before.
  const download = (
    <Button
      type="button"
      variant="outline"
      className="w-full justify-start gap-2"
      disabled={downloading}
      onClick={() => void handleDownload()}
    >
      {downloading ? (
        <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />
      ) : (
        <FileText className="h-4 w-4" aria-hidden="true" />
      )}
      <span className="min-w-0 flex-1 truncate text-left">{filename}</span>
      <Download className="h-4 w-4 shrink-0" aria-hidden="true" />
    </Button>
  );

  if (!removable) {
    return download;
  }
  return (
    <div className="flex items-center gap-2">
      <div className="min-w-0 flex-1">{download}</div>
      {removeButton(`Remove ${filename}`)}
    </div>
  );
}

interface RecordAttachmentListProps {
  mediaIds: string[];
  /** Called with the media id only; the caller owns confirmation. */
  onRemove?: (mediaId: string) => void;
  /** Gates the remove control on write access (PRD FR-VIEW-5). */
  canRemove?: boolean;
}

/**
 * The attachments of one record. Rendered only for the expanded record, which
 * is what keeps a 25-record page from issuing 25 × N metadata requests.
 *
 * Everything here renders for viewers too — only uploading, attaching and
 * removing are gated on write access (PRD FR-VIEW-5).
 */
export function RecordAttachmentList({ mediaIds, onRemove, canRemove }: RecordAttachmentListProps) {
  if (mediaIds.length === 0) {
    return <p className="text-xs text-muted-foreground">No attachments.</p>;
  }
  return (
    <div className="space-y-1.5">
      {mediaIds.map((id) => (
        <AttachmentRow key={id} mediaId={id} onRemove={onRemove} canRemove={canRemove} />
      ))}
    </div>
  );
}
```

- [ ] **Step 4: Run the list tests to verify they pass**

Run: `cd apps/web && npx vitest run src/components/features/vehicles/maintenance/RecordAttachmentList.test.tsx`
Expected: PASS, including the pre-existing download and thumbnail tests.

- [ ] **Step 5: Wire the drawer's confirmation and mutation**

In `VehicleRecordDrawer.tsx`:

Add to the imports:

```tsx
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '../../../ui/alert-dialog';
```

and add `useRemoveMaintenanceRecordDocument` to the existing `from '../../../../lib/hooks/api/maintenance'` import list.

Add state next to the existing `mode` state:

```tsx
  // The media id awaiting confirmation. Removal destroys the underlying media
  // object and is not undoable, so it is never one click.
  const [pendingRemoval, setPendingRemoval] = useState<string | null>(null);
```

Add the mutation next to `deleteRecord`:

```tsx
  const removeDocument = useRemoveMaintenanceRecordDocument(vehicleId);
```

Add the handler next to `handleDeleteRecord`:

```tsx
  const handleConfirmRemoveAttachment = async () => {
    const mediaId = pendingRemoval;
    if (!record || !mediaId) return;
    setPendingRemoval(null);
    try {
      await removeDocument.mutateAsync({ id: record.id, mediaId });
      toast.success('Attachment removed');
    } catch (err) {
      const apiError = createErrorFromUnknown(err);
      toast.error(apiError.message || 'Could not remove the attachment');
    }
  };
```

Replace the attachments block in the view body:

```tsx
          <div>
            <p className="mb-2 text-sm font-medium">Attachments</p>
            <RecordAttachmentList
              mediaIds={record.attributes.documentMediaIds ?? []}
              canRemove={canWrite}
              onRemove={setPendingRemoval}
            />
          </div>
```

And render the confirmation dialog. Put it at the end of the view-mode `<div className="space-y-4">` block, after the `{canWrite && (…)}` button row:

```tsx
          {/* The copy does not name the file: the filename is resolved inside
              AttachmentRow by useMediaObject, and lifting it up through
              onRemove for one sentence is not worth widening that contract —
              the unavailable row has no filename to lift anyway. */}
          <AlertDialog
            open={pendingRemoval !== null}
            onOpenChange={(open) => !open && setPendingRemoval(null)}
          >
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>Remove this attachment?</AlertDialogTitle>
                <AlertDialogDescription>
                  It will be taken off this record and the file itself will be deleted. This cannot
                  be undone.
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel disabled={removeDocument.isPending}>Cancel</AlertDialogCancel>
                <AlertDialogAction
                  className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
                  disabled={removeDocument.isPending}
                  onClick={(e) => {
                    e.preventDefault();
                    void handleConfirmRemoveAttachment();
                  }}
                >
                  Remove
                </AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
```

- [ ] **Step 6: Run the frontend suite, lint and typecheck**

Run: `cd apps/web && npx vitest run && npx tsc --noEmit -p tsconfig.json && npx eslint src --max-warnings=0`
Expected: PASS / clean.

- [ ] **Step 7: Commit**

```bash
git add apps/web/src/components/features/vehicles/maintenance/RecordAttachmentList.tsx \
        apps/web/src/components/features/vehicles/maintenance/RecordAttachmentList.test.tsx \
        apps/web/src/components/features/vehicles/detail/VehicleRecordDrawer.tsx
git commit -m "feat(web): remove attachments from a saved record"
```

---

### Task 8: Full branch verification

**Files:** none modified unless a check fails.

**Interfaces:**
- Consumes: everything from Tasks 1-7.
- Produces: a branch that satisfies the PRD §10 verification criteria.

- [ ] **Step 1: Run the full CI target**

```bash
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
make ci
```

Expected: `lint-check`, `vet`, `test`, `build`, `fe-test`, `fe-build` all pass. Fix anything that fails and re-run — do not proceed with a red build.

- [ ] **Step 2: Confirm no deployment manifest drift**

This task adds no service, config value, or route prefix — both routes sit under the already-routed `/api/fleet` prefix — so `deploy/k8s` must be untouched. Confirm:

```bash
git diff --name-only main...HEAD -- deploy/
```

Expected: empty output. If it is not empty, something was changed that should not have been.

- [ ] **Step 3: Walk the PRD acceptance criteria**

Open `docs/tasks/task-027-record-attachment-management/prd.md` §10 and confirm each backend and frontend checkbox is covered by a test written in Tasks 1-7, **with the one correction from Global Constraints**: the two "returns `403` for … a cross-fleet caller" criteria are satisfied by a `404`, which is what `authz.RequireSameFleet` produces and what the tests assert. Note that correction in the commit message below.

Not covered by an automated test, by design: FR-ATT-9's concurrency race (design D-7 — sqlite serializes writers so a test there proves nothing about Postgres, and standing up Postgres in CI for one assertion is disproportionate).

- [ ] **Step 4: Commit any fixes and record the verification**

```bash
git add -A
git commit -m "chore(task-027): verify record attachment management branch

Cross-fleet callers answer 404, not the 403 the PRD tables state:
authz.RequireSameFleet returns server.ErrNotFound so cross-fleet
existence is never leaked, which is this service's established
behavior and is pinned by TestPatch_otherFleetIsNotFound."
```

(If `git status` is clean, skip the commit — there is nothing to record.)

---

## Self-Review Notes

**Spec coverage.** Every PRD functional requirement maps to a task: FR-ATT-1/2 → Task 4 Step 3; FR-ATT-3 → Task 4 (empty-id guard + its test); FR-ATT-4 → Task 2/4 not-found tests; FR-ATT-5 → Task 4 preamble (with the 404 correction); FR-ATT-6 → Task 4 `ValidateOwnership`-before-write test; FR-ATT-7 → Task 2 cap test + Task 4 status test; FR-ATT-8 → Task 2 and Task 4 idempotency tests; FR-ATT-9 → Task 2's `FOR UPDATE`, deliberately untested per design D-7. FR-DET-1..4 → Tasks 2 and 4; FR-DET-5/6 → Task 5. FR-CAP-1..4 → Task 6. FR-UI-1..6 → Tasks 5, 6 and 7. Design §3.4's index and de-duplication → Task 1.

**Known deviations from the spec, both deliberate:**

1. **Cross-fleet status.** PRD §5.1/§5.2 and design §3.1 say `403`; the codebase says `404`. See Global Constraints. The plan implements the codebase's behavior.
2. **The document row's DOM shape.** Design §4.2 says the remove control is added to each row "rather than hoisted", and separately observes that making the download row a `div` containing two buttons would change the download hit area. A button cannot legally nest inside a button, so a wrapper is the only valid structure; the plan adds it *only when the row is removable*, so the read-only row a viewer sees is byte-identical to today's.
