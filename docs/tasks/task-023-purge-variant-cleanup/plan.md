# Media Purge Variant Cleanup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Make media-service's hourly purge sweep remove every byte and row derived from a purged media object, drain the backlog of already-orphaned derived state, and bring `media.media_variant_failures` under the admin purge protocol.

**Architecture:** A new `apps/media-service/internal/purge` package owns the tick — a per-object pass (bytes before rows, one transaction per media object) and a permanent reconciliation pass bounded by a configurable cap. It imports the three domain packages directly for their SQL and reaches MinIO through a one-method `ObjectRemover` port. `cmd/main.go` keeps only the `jobs.Every` + leader-lock registration. Separately, `media.media_variant_failures` joins `admin.Manifest`, and the arch test that enumerates purgeable tables is widened so no future table can hide from it.

**Tech Stack:** Go 1.x (go workspace, `apps/media-service` is its own module), GORM + Postgres in production, GORM + SQLite (`gorm.io/driver/sqlite`, mattn/go-sqlite3) in tests, logrus, chi, MinIO via `internal/storage`.

## Global Constraints

- **Task folder:** all documents live in `docs/tasks/task-023-purge-variant-cleanup/`. Work happens in the worktree `.worktrees/task-023-purge-variant-cleanup` on branch `task-023-purge-variant-cleanup`.
- **Bytes before rows, always.** The rows are the only record of which objects exist; deleting them first strands the bytes with nothing pointing at them (FR-PURGE-6).
- **A partial failure never destroys the handle.** If any byte removal for a media object fails, every row for that media object stays put (FR-PURGE-7).
- **Orphan detection tests for the absence of the parent row, never `deleted_at`** (FR-RECON-2). A variant of a soft-deleted-but-recoverable media object is not an orphan.
- **Variant keys come from the `object_key` column, never recomputed** from `storage.ObjectKey(...)` (FR-PURGE-2).
- **`mediaobject` must not import `mediavariant`, and neither may import `variantfailures`** (FR-TEST-2). `internal/purge` may import all three (design D-1).
- **`mediaobject.ListPurgeable`'s `purge_operation_id IS NULL` narrowing is untouched**, and `TestListPurgeable_skipsAdminStampedObjects` must pass unmodified (FR-PURGE-5).
- **Every new test must be demonstrated red** before the change that makes it green (FR-TEST-5). Each task below names the exact red demonstration. A test that cannot be made to go red is not evidence.
- **Log fields stay IDs and object keys.** No filenames; error strings attach via `WithError` only, never interpolated into messages.
- **A quiet tick must not log.** This job runs hourly forever and finds nothing on almost every run.
- **Reconciliation cap default: 500**, read as `config.GetInt("MEDIA_PURGE_RECONCILE_LIMIT", 500)`. `<= 0` disables the reconciliation pass entirely.
- **Go module commands:** the repo is a go workspace. Run tests as `go test github.com/jtumidanski/myfleet/apps/media-service/...` from the repo root, or `go test ./...` from `apps/media-service/`. Both forms appear below; either is fine.
- **Lint:** `.golangci.yml` enables the `standard` group (errcheck, govet, ineffassign, staticcheck, unused) plus gofumpt and goimports with `local-prefixes: github.com/jtumidanski/myfleet`. Intra-repo imports go in their own final group. `errcheck` is on — do not drop returned errors.

## Verified Facts That Change the Design

Three claims in `design.md` were checked against the running code during planning. Two are wrong, and the plan deviates deliberately. See `context.md` §"Deviations" for the full record; the short version:

- **P-1 — `variantfailures.Entity`'s new columns must NOT carry `gorm:"index"` tags.** Design §6.2 says to mirror `mediavariant.Entity` exactly, tags included. Verified: `AutoMigrate` on a schema-qualified table (`media.media_variant_failures`) emits `CREATE INDEX … ON \`media_variant_failures\`` **without** the schema qualifier, which fails on SQLite with `no such table: main.media_variant_failures`. Adding the tags therefore breaks the existing `variantfailures_test.go` harness, which calls `Migration(db)`. The plan declares the columns untagged and creates both indexes explicitly in `Migration` with a SQLite branch — exactly the `mediavariant.ApplyPartialIndexes` precedent. Postgres gets the same schema either way.
- **P-2 — the `internal/purge` harness cannot run the real `mediaobject.Migration` / `mediavariant.Migration`.** Design §8 specifies exactly that. Verified: both fail on SQLite for the same reason as P-1 (both entities carry index tags), which is precisely why `mediavariant/provider_test.go:14-17`, `mediaobject/purge_test.go` and `admin/admin_test.go` all hand-write DDL. The plan hand-writes DDL for those two tables, runs the real `variantfailures.Migration` (which works, and is the entity this task changes), and answers the design's legitimate anti-drift concern with an explicit **schema-drift guard test** that compares the harness DDL against the columns GORM derives from the entity structs. That guard is stronger than AutoMigrate would have been, because it fails loudly on drift rather than hiding it.
- **P-3 — the ConfigMap does change.** Design §9 says "No deploy-manifest change". The repo's own precedent disagrees: `MEDIA_WORKERS`, `MEDIA_MAX_UPLOAD_BYTES` and `MEDIA_LAZY_VARIANT_CONCURRENCY` all have in-code defaults and all appear in `deploy/k8s/base/media-service/configmap.yaml`, because that is where an operator looks for the knob (task-013 context.md §65 makes this exact argument). The plan adds `MEDIA_PURGE_RECONCILE_LIMIT: "500"` there. Additive and value-preserving.

Also verified during planning, and relied on below:

- media-service declares exactly four `TableName()` methods: `media.media_objects` and `media.media_variants` (in `entity.go`, both in `Manifest`), `media.processed_events` (in `processedevents.go`, in `excludedTables`), and `media.media_variant_failures` (in `variantfailures.go`, in neither). Widening the arch-test walk surfaces exactly the two the design predicts.
- The `ListOrphaned`, `DeleteOrphaned` (grouped `LIMIT`) and `ObjectKeysForMediaObject` SQL in this plan all execute correctly on SQLite.
- `db.Transaction` works on the harness under `SetMaxOpenConns(1)` provided every statement inside the callback uses `tx`, never the outer handle.
- The partial unique index on `(media_object_id, variant) WHERE deleted_at IS NULL` correctly admits both a live and a soft-deleted `card` row for the same media object — so a purge really does have four variant keys to remove in the T1 fixture.

## File Structure

| File | Responsibility |
| --- | --- |
| `apps/media-service/internal/admin/tablenames.go` | **New.** `CollectTableNames(root)` — the arch-test walk, extracted so it is itself testable. |
| `apps/media-service/internal/admin/testdata/fixture/ledger.go` | **New.** A `TableName` declared outside `entity.go`, for the fixture test. Under `testdata/` so the go tool ignores it. |
| `apps/media-service/internal/admin/arch_test.go` | Widened to call `CollectTableNames("..")`; gains the fixture test and the testdata-skip guard. |
| `apps/media-service/internal/admin/manifest.go` | `media.media_variant_failures` target added, child-to-parent ahead of `media.media_objects`. |
| `apps/media-service/internal/admin/operations.go` | `ReapSparing` gains the ledger case so a spared media object keeps its ledger rows (A-1). |
| `apps/media-service/internal/admin/admin_test.go` | Harness DDL + seed for the ledger table; stamp/restore/reap round-trip and the sparing test. |
| `apps/media-service/internal/variantfailures/variantfailures.go` | Two new entity columns; explicit index creation in `Migration`; `Recorded` narrowed to live rows. |
| `apps/media-service/internal/variantfailures/purge.go` | **New.** `DeleteForMediaObject`, `DeleteOrphaned`. |
| `apps/media-service/internal/variantfailures/purge_test.go` | **New.** Unit tests for the two queries. |
| `apps/media-service/internal/mediavariant/purge.go` | **New.** `Orphan`, `ObjectKeysForMediaObject`, `DeleteForMediaObject`, `ListOrphaned`, `DeleteByID`. |
| `apps/media-service/internal/mediavariant/purge_test.go` | **New.** Unit tests for the four queries. |
| `apps/media-service/internal/mediaobject/purge.go` | Doc line only: `DeleteRow`'s `db` may be a transaction (D-2). |
| `apps/media-service/internal/purge/sweeper.go` | **New.** `Sweeper`, `Config`, `ObjectRemover`, `RunOnce`, the per-object pass, the reconciliation pass, the summary. |
| `apps/media-service/internal/purge/testdb_test.go` | **New.** SQLite harness, the recording remover fake, seed helpers, the schema-drift guard. |
| `apps/media-service/internal/purge/sweeper_test.go` | **New.** T1–T5, the per-object pass. |
| `apps/media-service/internal/purge/reconcile_test.go` | **New.** T6–T8, the reconciliation pass. |
| `apps/media-service/internal/purge/arch_test.go` | **New.** T9, sibling-package independence. |
| `apps/media-service/cmd/main.go` | `purgeExpired` removed; `purge.NewSweeper` wired into the existing `jobs.Every` + leader lock. |
| `deploy/k8s/base/media-service/configmap.yaml` | `MEDIA_PURGE_RECONCILE_LIMIT: "500"` (P-3). |

---

### Task 1: Extract the arch-test walk into `CollectTableNames`

The walk currently lives inline in `TestManifestCoversEveryTable` and filters on `filepath.Base(path) != "entity.go"`, which is why `media.media_variant_failures` — declared in `variantfailures.go` — has been invisible to it. This task extracts the walk with its **final** semantics (every non-test `.go` file, `testdata/` skipped) and proves it against a fixture. `TestManifestCoversEveryTable` still uses its own inline walk here and is untouched, so the build stays green; Task 4 rewires it.

**Files:**
- Create: `apps/media-service/internal/admin/tablenames.go`
- Create: `apps/media-service/internal/admin/testdata/fixture/ledger.go`
- Test: `apps/media-service/internal/admin/arch_test.go` (append two tests)

**Interfaces:**
- Consumes: nothing.
- Produces: `func CollectTableNames(root string) ([]string, error)` in `package admin` — parses every non-test `.go` file under `root`, skipping any directory named `testdata`, and returns the string literal each `func (X) TableName() string` returns. Task 4 calls it with `".."`.

- [x] **Step 1: Write the fixture**

Create `apps/media-service/internal/admin/testdata/fixture/ledger.go`:

```go
// Package fixture exists only for TestCollectTableNames_seesDeclarationsOutsideEntityGo.
//
// It lives under testdata/ for two reasons: the go tool ignores testdata
// directories entirely, so this never enters a build or a vet run, and
// CollectTableNames skips them by name, so the production walk in
// TestManifestCoversEveryTable never reports this table and fails the build.
package fixture

// Entity stands in for an entity declared somewhere other than entity.go —
// the exact shape that kept media.media_variant_failures invisible to the arch
// test across every task that touched it.
type Entity struct{}

func (Entity) TableName() string { return "media.fixture_table" }
```

- [x] **Step 2: Write the failing tests**

Append to `apps/media-service/internal/admin/arch_test.go` (it is `package admin`, so `CollectTableNames` is reachable unqualified):

```go
// TestCollectTableNames_seesDeclarationsOutsideEntityGo is the whole point of
// FR-ADMIN-5. The previous walk filtered on the file being named entity.go, so
// a table declared anywhere else was invisible — which is exactly how
// media.media_variant_failures ended up in neither Manifest nor excludedTables.
func TestCollectTableNames_seesDeclarationsOutsideEntityGo(t *testing.T) {
	got, err := CollectTableNames("testdata/fixture")
	if err != nil {
		t.Fatalf("CollectTableNames: %v", err)
	}
	if len(got) != 1 || got[0] != "media.fixture_table" {
		t.Fatalf("CollectTableNames(testdata/fixture) = %v, want [media.fixture_table]", got)
	}
}

// TestCollectTableNames_skipsTestdata is the other half. The fixture above is
// demonstrably findable by the test that names its path directly, so this is a
// real check and not a vacuous one: if the walk descended into testdata, the
// production run in TestManifestCoversEveryTable would report a table that does
// not exist and fail the build.
func TestCollectTableNames_skipsTestdata(t *testing.T) {
	got, err := CollectTableNames("..")
	if err != nil {
		t.Fatalf("CollectTableNames: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("found no TableName declarations — the walk root is wrong, and this test would pass vacuously")
	}
	for _, name := range got {
		if name == "media.fixture_table" {
			t.Fatal("the walk descended into testdata and picked up the fixture table")
		}
	}
}
```

- [x] **Step 3: Run the tests to verify they fail**

```sh
go test github.com/jtumidanski/myfleet/apps/media-service/internal/admin/ -run TestCollectTableNames -v
```

Expected: FAIL to compile — `undefined: CollectTableNames`.

- [x] **Step 4: Write the implementation**

Create `apps/media-service/internal/admin/tablenames.go`:

```go
package admin

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
)

// CollectTableNames parses every non-test .go file under root and returns the
// string literal each `func (X) TableName() string` returns.
//
// It exists as production code rather than as a closure inside the arch test so
// that the walk itself can be tested against a fixture. FR-ADMIN-5's acceptance
// criterion — "adding a table in a file not named entity.go fails the arch
// test" — is only demonstrable that way.
//
// Two exclusions, both load-bearing:
//
//   - _test.go files are skipped. Test files carry DDL and fixtures, and
//     including them would report tables that do not exist in production.
//   - testdata directories are skipped BY NAME. The fixture proving this walk
//     sees non-entity.go declarations has to live somewhere, and if the
//     production run descended into testdata it would find the fixture's table,
//     report it as uncovered, and fail the build. The fixture test reaches it by
//     naming its path directly instead.
//
// Parsing rather than grepping: table names appear in comments, raw SQL and test
// DDL throughout the service, and a grep would produce false matches the first
// time someone documents one.
func CollectTableNames(root string) ([]string, error) {
	var found []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		f, perr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if perr != nil {
			// A parse failure is fatal, never a silent skip: a file the walk
			// cannot read is a file whose tables it cannot see (FR-ADMIN-6).
			return fmt.Errorf("parse %s: %w", path, perr)
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name.Name != "TableName" || fn.Recv == nil || fn.Body == nil {
				continue
			}
			for _, stmt := range fn.Body.List {
				ret, ok := stmt.(*ast.ReturnStmt)
				if !ok || len(ret.Results) != 1 {
					continue
				}
				lit, ok := ret.Results[0].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				table, uerr := strconv.Unquote(lit.Value)
				if uerr != nil {
					return fmt.Errorf("%s: unquote %s: %w", path, lit.Value, uerr)
				}
				found = append(found, table)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return found, nil
}
```

- [x] **Step 5: Run the tests to verify they pass**

```sh
go test github.com/jtumidanski/myfleet/apps/media-service/internal/admin/ -v
```

Expected: PASS, including the pre-existing `TestManifestCoversEveryTable` and `TestManifestKeysAreUnique`, which are untouched.

- [x] **Step 6: Commit**

```sh
git add apps/media-service/internal/admin/tablenames.go \
        apps/media-service/internal/admin/testdata/fixture/ledger.go \
        apps/media-service/internal/admin/arch_test.go
git commit -m "test(media-service): extract the manifest arch-test walk as CollectTableNames"
```

---

### Task 2: Give the variant-failure ledger its purge columns

`admin.Stamp` writes `deleted_at` and `purge_operation_id`; `admin.Restore` and `admin.Reap` key on `purge_operation_id`; `admin.Count` filters `deleted_at IS NULL`. A table in `Manifest` without those columns fails at **runtime**, not compile time. This task adds them and narrows `Recorded` so an in-flight, still-cancellable admin purge cannot suppress lazy generation.

Per **P-1**, the columns carry no `gorm:"index"` tag and the indexes are created explicitly, following `mediavariant.ApplyPartialIndexes`.

**Files:**
- Modify: `apps/media-service/internal/variantfailures/variantfailures.go:32-44` (entity + migration), `:55-66` (`Recorded`)
- Test: `apps/media-service/internal/variantfailures/variantfailures_test.go` (append one test)

**Interfaces:**
- Consumes: nothing.
- Produces: `variantfailures.Entity` gains `DeletedAt *time.Time` and `PurgeOperationID *string`. `variantfailures.ApplyIndexes(db *gorm.DB) error` is exported alongside `Migration` so a test harness can create the indexes against a hand-written table. `Store.Recorded` keeps its `(mediaObjectID, variant string) (bool, error)` signature.

- [x] **Step 1: Write the failing test**

Append to `apps/media-service/internal/variantfailures/variantfailures_test.go`:

```go
// FR-ADMIN-3. An admin purge that is still cancellable must not suppress lazy
// generation: if the operator cancels, the object comes back, and a ledger row
// that was never a real failure would have permanently disabled its card.
func TestRecorded_ignoresSoftDeletedRows(t *testing.T) {
	db := newTestDB(t)
	s := New(logrus.New(), db)
	if err := s.Record("m1", "card", ReasonUndecodable); err != nil {
		t.Fatalf("Record: %v", err)
	}

	// Exactly what admin.Stamp writes.
	if err := db.Exec(`UPDATE media.media_variant_failures
	                   SET deleted_at = ?, purge_operation_id = ?
	                   WHERE media_object_id = 'm1' AND variant = 'card'`,
		time.Now().UTC(), "op-1").Error; err != nil {
		t.Fatalf("stamp the ledger row: %v", err)
	}

	recorded, err := s.Recorded("m1", "card")
	if err != nil {
		t.Fatalf("Recorded: %v", err)
	}
	if recorded {
		t.Fatal("a ledger row soft-deleted by an in-flight, still-cancellable admin purge " +
			"must not report as a permanent failure")
	}
}
```

Add `"time"` to that file's import block.

- [x] **Step 2: Run the test to verify it fails**

```sh
go test github.com/jtumidanski/myfleet/apps/media-service/internal/variantfailures/ -run TestRecorded_ignoresSoftDeletedRows -v
```

Expected: FAIL with `stamp the ledger row: no such column: deleted_at`.

- [x] **Step 3: Add the columns and the explicit indexes**

In `apps/media-service/internal/variantfailures/variantfailures.go`, replace the `Entity` declaration and `Migration` (currently lines 32-44) with:

```go
// Entity maps to media.media_variant_failures. The composite primary key is the
// uniqueness guarantee — no surrogate ID and no extra index needed.
//
// DeletedAt and PurgeOperationID exist because this table is in admin.Manifest:
// admin.Stamp writes both, admin.Restore and admin.Reap key on
// purge_operation_id, and admin.Count filters deleted_at IS NULL. Without them
// the manifest entry fails at RUNTIME, not at compile time.
//
// Neither carries a `gorm:"index"` tag, deliberately. AutoMigrate on a
// schema-qualified table emits its CREATE INDEX WITHOUT the schema qualifier
// (`ON media_variant_failures`, not `ON media.media_variant_failures`), which
// fails outright on the SQLite test harness. The indexes are therefore created
// explicitly in ApplyIndexes below — the same reason and the same shape as
// mediavariant.ApplyPartialIndexes.
//
// No unique index, unlike media.media_variants: the composite primary key is the
// uniqueness guarantee, and Record's ON CONFLICT DO NOTHING names no columns, so
// Postgres needs no inferred arbiter and a soft-deleted row cannot be silently
// updated in place.
type Entity struct {
	MediaObjectID    string `gorm:"type:uuid;primaryKey"`
	Variant          string `gorm:"primaryKey"`
	Reason           string
	FailedAt         time.Time
	DeletedAt        *time.Time
	PurgeOperationID *string `gorm:"type:uuid"`
}

func (Entity) TableName() string { return "media.media_variant_failures" }

// Migration auto-migrates the variant-failure ledger table and its indexes.
//
// Both columns are nullable with no default, so AutoMigrate adds them to a
// populated table without a rewrite, and NULL is the correct value for every
// existing row — live, not soft-deleted, attached to no purge operation — so
// there is no backfill.
func Migration(db *gorm.DB) error {
	if err := db.AutoMigrate(&Entity{}); err != nil {
		return err
	}
	return ApplyIndexes(db)
}

// ApplyIndexes creates the deleted_at and purge_operation_id indexes the admin
// purge protocol's Restore and Reap scans depend on.
//
// It is separate from Migration, and exported, for the same reason
// mediavariant.ApplyPartialIndexes is: a test harness that hand-writes the table
// still needs the real indexes, and struct tags cannot express the
// schema-qualified form SQLite requires.
func ApplyIndexes(db *gorm.DB) error {
	stmts := []string{
		`CREATE INDEX IF NOT EXISTS idx_media_variant_failures_deleted_at
		 ON media.media_variant_failures (deleted_at)`,
		`CREATE INDEX IF NOT EXISTS idx_media_variant_failures_purge_operation_id
		 ON media.media_variant_failures (purge_operation_id)`,
	}
	if db.Name() == "sqlite" {
		stmts = []string{
			`CREATE INDEX IF NOT EXISTS media.idx_media_variant_failures_deleted_at
			 ON media_variant_failures (deleted_at)`,
			`CREATE INDEX IF NOT EXISTS media.idx_media_variant_failures_purge_operation_id
			 ON media_variant_failures (purge_operation_id)`,
		}
	}
	for _, q := range stmts {
		if err := db.Exec(q).Error; err != nil {
			return err
		}
	}
	return nil
}
```

- [x] **Step 4: Narrow `Recorded`**

Replace `Recorded` (currently lines 55-66) with:

```go
// Recorded reports whether generation of this variant for this media object is
// known to be impossible.
//
// Soft-deleted rows do not count (FR-ADMIN-3). A ledger row an admin purge has
// stamped belongs to an operation that may still be cancelled, and a purge that
// gets undone must not leave the object's card permanently disabled.
//
// One consequence, documented rather than engineered around: while a row is
// soft-deleted its primary-key slot is still occupied, so a fresh Record for the
// same (media_object_id, variant) is a silent no-op and Recorded keeps returning
// false. In principle that is an unbounded retry loop; in practice it is
// unreachable, because Recorded is consulted only by processing.CardGenerator,
// which is reached only through Content, which resolves a LIVE media object
// first — and a media object whose ledger rows are stamped is itself stamped,
// and therefore soft-deleted, in the same transaction. Restore clears both
// columns and the ledger is live again.
func (s *Store) Recorded(mediaObjectID, variant string) (bool, error) {
	var count int64
	err := s.db.Model(&Entity{}).
		Where("media_object_id = ? AND variant = ? AND deleted_at IS NULL", mediaObjectID, variant).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
```

- [x] **Step 5: Run the tests to verify they pass**

```sh
go test github.com/jtumidanski/myfleet/apps/media-service/internal/variantfailures/ -v
```

Expected: PASS — the new test plus the three pre-existing ones (`TestRecordThenRecorded`, `TestRecorded_isScopedToTheObjectAndVariant`, `TestRecord_firstReasonWins`). If any pre-existing test fails with `no such table: main.media_variant_failures`, an index tag was left on the struct; remove it (P-1).

- [x] **Step 6: Verify the whole service still builds and passes**

```sh
go test github.com/jtumidanski/myfleet/apps/media-service/...
```

Expected: PASS. In particular `apps/media-service/cmd` — `TestNoLossySaveRoundTrips` walks `../internal` and must stay clean.

- [x] **Step 7: Commit**

```sh
git add apps/media-service/internal/variantfailures/
git commit -m "feat(media-service): give the variant-failure ledger its purge columns"
```

---

### Task 3: Put `media.media_variant_failures` in `admin.Manifest`

**Files:**
- Modify: `apps/media-service/internal/admin/manifest.go:40-70`
- Test: `apps/media-service/internal/admin/admin_test.go:34-78` (harness) and append one test

**Interfaces:**
- Consumes: `variantfailures.Entity`'s new columns (Task 2).
- Produces: a `Target` with `Key: "media_variant_failures"`, `Table: "media.media_variant_failures"` in `Manifest`, positioned between `media_variants` and `media_objects`. `affected_counts` in admin purge responses gains a `media_variant_failures` key.

- [x] **Step 1: Extend the test harness with the ledger table**

In `apps/media-service/internal/admin/admin_test.go`, add to the `ddl` slice in `newMediaDB` (after the `media_variants` statement):

```go
		`CREATE TABLE media.media_variant_failures (
			media_object_id TEXT, variant TEXT, reason TEXT, failed_at DATETIME,
			deleted_at DATETIME, purge_operation_id TEXT,
			PRIMARY KEY (media_object_id, variant))`,
```

and to the `seed` slice (after the `mv-2` insert):

```go
		`INSERT INTO media.media_variant_failures (media_object_id, variant, reason, failed_at)
		 VALUES ('mo-1', 'card', 'undecodable', CURRENT_TIMESTAMP)`,
		`INSERT INTO media.media_variant_failures (media_object_id, variant, reason, failed_at)
		 VALUES ('mo-2', 'card', 'undecodable', CURRENT_TIMESTAMP)`,
```

- [x] **Step 2: Write the failing test**

Append to `apps/media-service/internal/admin/admin_test.go`:

```go
// FR-ADMIN-1/2. The ledger joins the manifest so an admin reap does not leave
// its rows orphaned — the same defect the hourly sweep has, on a different
// route. The full stamp -> restore -> reap round-trip, because each phase keys
// on a different column and any one of them can be the thing that is missing.
func TestPurge_reachesTheVariantFailureLedger(t *testing.T) {
	db := newMediaDB(t)
	r := newMediaRouter(t, db, &recordingRemover{})
	const scope = `{"operation_id":"op-1","scope":"fleet","fleet_ids":["fleet-1"]}`

	rec := post(t, r, "/internal/admin/purge", scope)
	if rec.Code != http.StatusOK {
		t.Fatalf("stamp status = %d: %s", rec.Code, rec.Body.String())
	}
	var stamped struct {
		Affected map[string]int `json:"affected"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &stamped); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if stamped.Affected["media_variant_failures"] != 1 {
		t.Errorf("affected = %+v, want one media_variant_failures row", stamped.Affected)
	}

	// Restore must bring it back, or a cancelled purge leaves the ledger
	// soft-deleted forever and lazy generation silently re-enables.
	restore := httptest.NewRecorder()
	r.ServeHTTP(restore, httptest.NewRequest(http.MethodDelete, "/internal/admin/purge/op-1", nil))
	if restore.Code != http.StatusOK {
		t.Fatalf("restore status = %d: %s", restore.Code, restore.Body.String())
	}
	var live int64
	db.Raw(`SELECT count(*) FROM media.media_variant_failures
	        WHERE media_object_id = 'mo-1' AND deleted_at IS NULL`).Scan(&live)
	if live != 1 {
		t.Errorf("restore left %d of 1 ledger rows live", live)
	}

	// And a reap must actually remove it.
	post(t, r, "/internal/admin/purge", scope)
	if rec := post(t, r, "/internal/admin/reap/op-1", ""); rec.Code != http.StatusOK {
		t.Fatalf("reap status = %d: %s", rec.Code, rec.Body.String())
	}
	var left int64
	db.Raw(`SELECT count(*) FROM media.media_variant_failures WHERE media_object_id = 'mo-1'`).Scan(&left)
	if left != 0 {
		t.Errorf("reap left %d ledger rows behind for the purged media object", left)
	}
	// The other tenant's ledger row is untouched.
	db.Raw(`SELECT count(*) FROM media.media_variant_failures WHERE media_object_id = 'mo-2'`).Scan(&left)
	if left != 1 {
		t.Errorf("the purge reached another tenant's ledger rows: %d of 1 left", left)
	}
}
```

- [x] **Step 3: Run the test to verify it fails**

```sh
go test github.com/jtumidanski/myfleet/apps/media-service/internal/admin/ -run TestPurge_reachesTheVariantFailureLedger -v
```

Expected: FAIL — `affected = map[media_objects:1 media_variants:1], want one media_variant_failures row`. The table is not in `Manifest`, so nothing stamps it.

- [x] **Step 4: Add the manifest target**

In `apps/media-service/internal/admin/manifest.go`, insert between the `media_variants` and `media_objects` entries:

```go
	{
		// Child-to-parent, ahead of media_objects. The ordering is readability
		// only — media's Target.Where never filters a parent's deleted_at, which
		// is what makes stamp order-independent — but it is the documented
		// convention and FR-ADMIN-1 asks for it.
		Key: "media_variant_failures", Table: "media.media_variant_failures",
		Where: func(r Root) (string, []any) {
			switch r.Scope {
			case ScopeSystem:
				return all, nil
			case ScopeFleet:
				return "media_object_id IN (SELECT id FROM media.media_objects WHERE fleet_id IN ?)",
					[]any{r.FleetIDs}
			case ScopeMediaIDs:
				return "media_object_id IN ?", []any{r.MediaIDs}
			}
			return "", nil
		},
	},
```

- [x] **Step 5: Run the tests to verify they pass**

```sh
go test github.com/jtumidanski/myfleet/apps/media-service/internal/admin/ -v
```

Expected: PASS, all of them. `TestManifestKeysAreUnique` confirms the new key does not collide. `TestManifestCoversEveryTable` still passes because it has not been widened yet.

- [x] **Step 6: Commit**

```sh
git add apps/media-service/internal/admin/manifest.go apps/media-service/internal/admin/admin_test.go
git commit -m "feat(media-service): bring the variant-failure ledger under the admin purge manifest"
```

---

### Task 4: Widen `TestManifestCoversEveryTable`

**Files:**
- Modify: `apps/media-service/internal/admin/arch_test.go:27-91`

**Interfaces:**
- Consumes: `CollectTableNames` (Task 1), the manifest entry (Task 3).
- Produces: nothing new.

- [x] **Step 1: Rewire the test to use the widened walk**

In `apps/media-service/internal/admin/arch_test.go`, replace `TestManifestCoversEveryTable` (lines 14-91, doc comment included) with:

```go
// TestManifestCoversEveryTable turns FR-ADMIN-PURGE-4's "every table, enumerated
// by hand" from a checklist into a compile-time-ish guarantee, exactly as
// fleet-service's twin of this test does for its own manifest.
//
// It walks every non-test .go file under internal/ — not only files named
// entity.go, which is how media.media_variant_failures stayed invisible to this
// check across every task that touched it (FR-ADMIN-5) — and requires each table
// to be in Manifest or in excludedTables with a reason. A new table added
// anywhere in media-service fails here until someone decides whether a purge
// should reach it.
func TestManifestCoversEveryTable(t *testing.T) {
	inManifest := map[string]bool{}
	for _, target := range Manifest {
		inManifest[target.Table] = true
	}

	found, err := CollectTableNames("..") // apps/media-service/internal
	if err != nil {
		// A parse failure fails the test, never a silent skip (FR-ADMIN-6).
		t.Fatalf("collect table names: %v", err)
	}
	if len(found) == 0 {
		t.Fatal("found no TableName declarations — the walk root is wrong, and this test would pass vacuously")
	}

	for _, name := range found {
		if inManifest[name] {
			continue
		}
		if reason, ok := excludedTables[name]; ok {
			if strings.TrimSpace(reason) == "" {
				t.Errorf("%s is excluded from the purge manifest with an empty reason", name)
			}
			continue
		}
		t.Errorf("%s is in neither admin.Manifest nor admin.excludedTables — "+
			"decide whether a purge should reach it, then add it to one of them", name)
	}
}
```

Then fix the import block: `go/ast`, `go/parser`, `go/token`, `os`, `path/filepath` and `strconv` are no longer used by this file. It should reduce to:

```go
import (
	"strings"
	"testing"
)
```

- [x] **Step 2: Run the test to verify it passes**

```sh
go test github.com/jtumidanski/myfleet/apps/media-service/internal/admin/ -run TestManifestCoversEveryTable -v
```

Expected: PASS. The widened walk now reports four tables — `media.media_objects` and `media.media_variants` (in `Manifest`), `media.processed_events` (in `excludedTables`) and `media.media_variant_failures` (added to `Manifest` in Task 3).

- [x] **Step 3: Demonstrate the test can go red**

The widened walk is only worth anything if it actually catches an uncovered table. Prove it:

```sh
# Temporarily comment out the media_variant_failures Target added in Task 3.
go test github.com/jtumidanski/myfleet/apps/media-service/internal/admin/ -run TestManifestCoversEveryTable -v
```

Expected: FAIL with `media.media_variant_failures is in neither admin.Manifest nor admin.excludedTables`. Restore the target and re-run to confirm PASS. **Do not commit the commented-out version.**

- [x] **Step 4: Verify nothing else in the service regressed**

```sh
go test github.com/jtumidanski/myfleet/apps/media-service/...
```

Expected: PASS.

- [x] **Step 5: Commit**

```sh
git add apps/media-service/internal/admin/arch_test.go
git commit -m "test(media-service): widen the manifest arch test past entity.go"
```

---

### Task 5: Spare the ledger rows of a spared media object

When a media object's bytes cannot be removed, `ReapSparing` keeps its row and its variant rows so the next tick retries. Its ledger rows would be reaped anyway, leaving the operation partially applied. This is design addition **A-1**, beyond FR-ADMIN-4.

**Files:**
- Modify: `apps/media-service/internal/admin/operations.go:96-122`
- Test: `apps/media-service/internal/admin/admin_test.go` (append one test)

**Interfaces:**
- Consumes: the manifest entry (Task 3).
- Produces: nothing new. `ReapSparing`'s signature is unchanged.

- [x] **Step 1: Write the failing test**

Append to `apps/media-service/internal/admin/admin_test.go`:

```go
// A-1. A media object spared because its bytes could not be removed keeps its
// row and its variant rows for the retry — its LEDGER rows must be spared with
// them, or the operation is left half-applied and the next tick reaps a media
// object whose failure ledger has already been thrown away.
func TestReap_sparedObjectKeepsItsFailureLedgerRows(t *testing.T) {
	db := newMediaDB(t)
	store := &recordingRemover{fail: map[string]bool{"k/mo-1": true}}
	r := newMediaRouter(t, db, store)
	post(t, r, "/internal/admin/purge", `{"operation_id":"op-1","scope":"fleet","fleet_ids":["fleet-1"]}`)

	if rec := post(t, r, "/internal/admin/reap/op-1", ""); rec.Code == http.StatusOK {
		t.Fatalf("a failed object removal must not report success, got %d", rec.Code)
	}

	var spared int64
	db.Raw(`SELECT count(*) FROM media.media_variant_failures
	        WHERE media_object_id = 'mo-1' AND purge_operation_id = 'op-1'`).Scan(&spared)
	if spared != 1 {
		t.Fatalf("the spared media object's ledger rows were reaped anyway: %d of 1 left", spared)
	}
}
```

- [x] **Step 2: Run the test to verify it fails**

```sh
go test github.com/jtumidanski/myfleet/apps/media-service/internal/admin/ -run TestReap_sparedObjectKeepsItsFailureLedgerRows -v
```

Expected: FAIL — `the spared media object's ledger rows were reaped anyway: 0 of 1 left`.

- [x] **Step 3: Add the sparing case**

In `apps/media-service/internal/admin/operations.go`, extend the switch inside `ReapSparing`:

```go
			switch t.Table {
			case "media.media_objects":
				q += " AND id NOT IN ?"
			case "media.media_variants":
				q += " AND media_object_id NOT IN ?"
			case "media.media_variant_failures":
				// A spared media object keeps its ledger too. Reaping the ledger
				// of an object whose bytes are still in the bucket leaves the
				// operation half-applied, and the next tick would retry a media
				// object whose failure record it had already discarded.
				q += " AND media_object_id NOT IN ?"
			}
```

Also extend the doc comment on `ReapSparing` — replace the sentence "media_objects are spared by their own id; their variants by the object they belong to, so a spared object never loses the variants that describe it." (in the inline comment above the switch) with:

```go
				// media_objects are spared by their own id; their variants and
				// their failure-ledger rows by the object they belong to, so a
				// spared object never loses the rows that describe it.
```

- [x] **Step 4: Run the tests to verify they pass**

```sh
go test github.com/jtumidanski/myfleet/apps/media-service/internal/admin/ -v
```

Expected: PASS, including `TestReap_sparedRowKeepsItsOperationIDSoTheNextTickCanRetry`, which exercises the retry after the bucket recovers.

- [x] **Step 5: Commit**

```sh
git add apps/media-service/internal/admin/operations.go apps/media-service/internal/admin/admin_test.go
git commit -m "fix(media-service): spare a spared media object's failure-ledger rows on reap"
```

---

### Task 6: The purge-side variant queries

**Files:**
- Create: `apps/media-service/internal/mediavariant/purge.go`
- Test: `apps/media-service/internal/mediavariant/purge_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces, all in `package mediavariant`:
  - `type Orphan struct { ID string; ObjectKey string }`
  - `func ObjectKeysForMediaObject(db *gorm.DB, mediaObjectID string) ([]string, error)`
  - `func DeleteForMediaObject(db *gorm.DB, mediaObjectID string) error`
  - `func ListOrphaned(db *gorm.DB, limit int) ([]Orphan, error)`
  - `func DeleteByID(db *gorm.DB, id string) error`

  Task 9 and Task 10 call all five.

- [x] **Step 1: Write the failing tests**

Create `apps/media-service/internal/mediavariant/purge_test.go`:

```go
package mediavariant

import (
	"testing"
	"time"

	"gorm.io/gorm"
)

// newPurgeVariantDB extends newVariantTestDB (provider_test.go) with the
// media_objects table the orphan anti-join needs. The DDL is hand-written for
// the same reason that helper's is: AutoMigrate emits its CREATE INDEX without
// the schema qualifier and fails on SQLite.
func newPurgeVariantDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := newVariantTestDB(t)
	if err := db.Exec(`CREATE TABLE media.media_objects (
		id TEXT PRIMARY KEY, fleet_id TEXT, uploaded_by_user_id TEXT, bucket TEXT,
		object_key TEXT, content_type TEXT, size INTEGER, original_filename TEXT,
		status TEXT, created_at DATETIME, deleted_at DATETIME, purge_after DATETIME,
		purge_operation_id TEXT)`).Error; err != nil {
		t.Fatalf("create media_objects: %v", err)
	}
	return db
}

func insertVariantRow(t *testing.T, db *gorm.DB, id, mediaObjectID, variant, key string, deletedAt *time.Time) {
	t.Helper()
	if err := db.Exec(`INSERT INTO media.media_variants
		(id, media_object_id, variant, object_key, deleted_at)
		VALUES (?, ?, ?, ?, ?)`, id, mediaObjectID, variant, key, deletedAt).Error; err != nil {
		t.Fatalf("insert variant %s: %v", id, err)
	}
}

func insertMediaObject(t *testing.T, db *gorm.DB, id string, deletedAt *time.Time) {
	t.Helper()
	if err := db.Exec(`INSERT INTO media.media_objects (id, fleet_id, bucket, object_key, status, deleted_at)
		VALUES (?, 'fleet-1', 'media', ?, 'ready', ?)`, id, "k/"+id, deletedAt).Error; err != nil {
		t.Fatalf("insert media object %s: %v", id, err)
	}
}

// FR-PURGE-1/2. Every stored key, INCLUDING rows a purge has soft-deleted, and
// read from the object_key column rather than recomputed — the column is the
// record of what was actually written.
func TestObjectKeysForMediaObject_includesSoftDeletedRows(t *testing.T) {
	db := newPurgeVariantDB(t)
	past := time.Now().UTC().Add(-time.Hour)
	insertVariantRow(t, db, "mv-t", "mo-1", "thumbnail", "k/mo-1-thumbnail", nil)
	insertVariantRow(t, db, "mv-c", "mo-1", "card", "k/mo-1-card", nil)
	insertVariantRow(t, db, "mv-old", "mo-1", "card", "k/mo-1-card-old", &past)
	insertVariantRow(t, db, "mv-other", "mo-2", "card", "k/mo-2-card", nil)

	got, err := ObjectKeysForMediaObject(db, "mo-1")
	if err != nil {
		t.Fatalf("ObjectKeysForMediaObject: %v", err)
	}
	want := map[string]bool{"k/mo-1-thumbnail": true, "k/mo-1-card": true, "k/mo-1-card-old": true}
	if len(got) != len(want) {
		t.Fatalf("got %d keys %v, want %d", len(got), got, len(want))
	}
	for _, k := range got {
		if !want[k] {
			t.Errorf("unexpected key %q", k)
		}
		delete(want, k)
	}
	if len(want) != 0 {
		t.Errorf("these keys were never returned, so their bytes would leak: %v", want)
	}
}

func TestDeleteForMediaObject_removesEveryRowForThatObjectOnly(t *testing.T) {
	db := newPurgeVariantDB(t)
	past := time.Now().UTC().Add(-time.Hour)
	insertVariantRow(t, db, "mv-c", "mo-1", "card", "k/mo-1-card", nil)
	insertVariantRow(t, db, "mv-old", "mo-1", "display", "k/mo-1-old", &past)
	insertVariantRow(t, db, "mv-other", "mo-2", "card", "k/mo-2-card", nil)

	if err := DeleteForMediaObject(db, "mo-1"); err != nil {
		t.Fatalf("DeleteForMediaObject: %v", err)
	}
	var mine, others int64
	db.Raw(`SELECT count(*) FROM media.media_variants WHERE media_object_id = 'mo-1'`).Scan(&mine)
	db.Raw(`SELECT count(*) FROM media.media_variants WHERE media_object_id = 'mo-2'`).Scan(&others)
	if mine != 0 {
		t.Errorf("left %d rows for the purged media object", mine)
	}
	if others != 1 {
		t.Errorf("deleted another media object's variants: %d of 1 left", others)
	}
}

// FR-RECON-2, the single most important behaviour in the reconciliation pass.
// The test is the ABSENCE of the parent row, never deleted_at: a variant of a
// soft-deleted-but-recoverable media object is not an orphan, and deleting it
// would silently break both the five-day restore and the admin console's cancel.
func TestListOrphaned_testsForTheParentRowNotDeletedAt(t *testing.T) {
	db := newPurgeVariantDB(t)
	past := time.Now().UTC().Add(-time.Hour)
	insertMediaObject(t, db, "mo-live", nil)
	insertMediaObject(t, db, "mo-soft", &past) // soft-deleted, still recoverable
	insertVariantRow(t, db, "mv-live", "mo-live", "card", "k/live", nil)
	insertVariantRow(t, db, "mv-soft", "mo-soft", "card", "k/soft", nil)
	insertVariantRow(t, db, "mv-orphan", "mo-gone", "card", "k/orphan", nil)

	got, err := ListOrphaned(db, 100)
	if err != nil {
		t.Fatalf("ListOrphaned: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ListOrphaned = %+v, want exactly the one row whose media object is gone", got)
	}
	if got[0].ID != "mv-orphan" || got[0].ObjectKey != "k/orphan" {
		t.Errorf("ListOrphaned = %+v, want {mv-orphan k/orphan}", got[0])
	}
}

// FR-RECON-5. One tick must not turn into an unbounded bucket-deletion loop on
// first deployment against a database with a large accumulated backlog.
func TestListOrphaned_honoursTheLimit(t *testing.T) {
	db := newPurgeVariantDB(t)
	// Distinct variant values: the partial unique index on
	// (media_object_id, variant) WHERE deleted_at IS NULL would reject three
	// live "card" rows for the same media object.
	for _, id := range []string{"mv-1", "mv-2", "mv-3"} {
		insertVariantRow(t, db, id, "mo-gone", "card-"+id, "k/"+id, nil)
	}
	got, err := ListOrphaned(db, 2)
	if err != nil {
		t.Fatalf("ListOrphaned: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListOrphaned(limit 2) returned %d rows, want 2", len(got))
	}
}

func TestDeleteByID_removesExactlyOneRow(t *testing.T) {
	db := newPurgeVariantDB(t)
	insertVariantRow(t, db, "mv-1", "mo-gone", "card", "k/1", nil)
	insertVariantRow(t, db, "mv-2", "mo-gone", "display", "k/2", nil)

	if err := DeleteByID(db, "mv-1"); err != nil {
		t.Fatalf("DeleteByID: %v", err)
	}
	var left int64
	db.Raw(`SELECT count(*) FROM media.media_variants`).Scan(&left)
	if left != 1 {
		t.Errorf("DeleteByID left %d rows, want exactly the untouched one", left)
	}
}
```

- [x] **Step 2: Run the tests to verify they fail**

```sh
go test github.com/jtumidanski/myfleet/apps/media-service/internal/mediavariant/ -run 'TestObjectKeysForMediaObject|TestDeleteForMediaObject|TestListOrphaned|TestDeleteByID' -v
```

Expected: FAIL to compile — `undefined: ObjectKeysForMediaObject`, `undefined: DeleteForMediaObject`, `undefined: ListOrphaned`, `undefined: DeleteByID`.

- [x] **Step 3: Write the implementation**

Create `apps/media-service/internal/mediavariant/purge.go`:

```go
package mediavariant

import "gorm.io/gorm"

// Orphan is a variant row whose media object no longer exists, paired with the
// bytes it is the only remaining record of.
type Orphan struct {
	ID        string
	ObjectKey string
}

// ObjectKeysForMediaObject returns every stored key for a media object,
// including rows a purge has soft-deleted.
//
// There is deliberately no deleted_at predicate. A purge hard-deletes the media
// object, so a soft-deleted variant of it is not recoverable state — it is state
// whose bytes would leak if this query skipped it (FR-PURGE-1). It also admits
// one odd row: a variant carrying a purge_operation_id whose parent object does
// not, the residue of a spared or partially-reaped admin operation. Deleting it
// is still correct, because the object it describes is being hard-deleted in the
// same transaction and the row could only become an orphan.
//
// Keys come from the object_key column, never recomputed from
// storage.ObjectKey(fleetID, mediaObjectID, kind+ext): the column is the record
// of what was actually written, and a recomputed key that disagreed with a
// historical naming scheme would silently skip the bytes (FR-PURGE-2).
func ObjectKeysForMediaObject(db *gorm.DB, mediaObjectID string) ([]string, error) {
	var keys []string
	err := db.Raw(`SELECT object_key FROM media.media_variants WHERE media_object_id = ?`,
		mediaObjectID).Scan(&keys).Error
	if err != nil {
		return nil, err
	}
	return keys, nil
}

// DeleteForMediaObject hard-deletes every variant row for a media object.
//
// db may be a transaction — pass the tx so the variant rows, the ledger rows and
// the media-object row all go in one statement group (FR-PURGE-4).
func DeleteForMediaObject(db *gorm.DB, mediaObjectID string) error {
	return db.Exec(`DELETE FROM media.media_variants WHERE media_object_id = ?`, mediaObjectID).Error
}

// ListOrphaned returns up to limit variant rows whose media object no longer
// exists, with the object key each one is the last record of.
//
// The test is the ABSENCE of the parent row, never deleted_at (FR-RECON-2). A
// variant belonging to a soft-deleted-but-recoverable media object is not an
// orphan, and deleting it would silently break restore — both the user-facing
// five-day window and the admin console's cancel.
//
// The DELETE target in the sibling query carries no alias for portability
// (`DELETE FROM t AS c` is Postgres-only), but a SELECT alias is fine on both.
func ListOrphaned(db *gorm.DB, limit int) ([]Orphan, error) {
	var out []Orphan
	q := `SELECT id, object_key FROM media.media_variants v
	      WHERE NOT EXISTS (SELECT 1 FROM media.media_objects o WHERE o.id = v.media_object_id)
	      LIMIT ?`
	if err := db.Raw(q, limit).Scan(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// DeleteByID hard-deletes one variant row, used after its bytes are gone.
//
// One row at a time, deliberately: reconciliation removes each orphan's bytes
// individually, and a batched delete would either lose the ability to spare
// exactly the row whose removal failed or need the failed set threaded back
// through, for no benefit at the per-tick cap.
func DeleteByID(db *gorm.DB, id string) error {
	return db.Exec(`DELETE FROM media.media_variants WHERE id = ?`, id).Error
}
```

- [x] **Step 4: Run the tests to verify they pass**

```sh
go test github.com/jtumidanski/myfleet/apps/media-service/internal/mediavariant/ -v
```

Expected: PASS, including the pre-existing `TestUpsert_*`, `TestGetByMediaObjectAndVariant_*` and `TestMediaVariantReads_hideSoftDeleted`.

- [x] **Step 5: Commit**

```sh
git add apps/media-service/internal/mediavariant/purge.go apps/media-service/internal/mediavariant/purge_test.go
git commit -m "feat(media-service): add the purge-side media-variant queries"
```

---

### Task 7: The purge-side ledger queries

**Files:**
- Create: `apps/media-service/internal/variantfailures/purge.go`
- Test: `apps/media-service/internal/variantfailures/purge_test.go`

**Interfaces:**
- Consumes: the entity columns from Task 2.
- Produces, all in `package variantfailures`:
  - `func DeleteForMediaObject(db *gorm.DB, mediaObjectID string) error`
  - `func DeleteOrphaned(db *gorm.DB, limit int) (int, error)` — returns rows deleted.

  Both are package-level functions on `*gorm.DB`, matching `mediaobject.DeleteRow` and `admin`'s operations rather than methods on `Store`: `Store` carries a logger and serves the request-path ledger, and a purge does not belong on the type lazy generation depends on.

- [x] **Step 1: Write the failing tests**

Create `apps/media-service/internal/variantfailures/purge_test.go`:

```go
package variantfailures

import (
	"testing"
	"time"

	"gorm.io/gorm"
)

// newLedgerPurgeDB extends newTestDB (variantfailures_test.go) with the
// media_objects table the orphan anti-join needs. media_objects is hand-written
// because its entity's index tags make AutoMigrate fail on SQLite; the ledger
// table itself comes from the real Migration, which is the entity this task
// changes and therefore the one that must not drift.
func newLedgerPurgeDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := newTestDB(t)
	if err := db.Exec(`CREATE TABLE media.media_objects (
		id TEXT PRIMARY KEY, fleet_id TEXT, uploaded_by_user_id TEXT, bucket TEXT,
		object_key TEXT, content_type TEXT, size INTEGER, original_filename TEXT,
		status TEXT, created_at DATETIME, deleted_at DATETIME, purge_after DATETIME,
		purge_operation_id TEXT)`).Error; err != nil {
		t.Fatalf("create media_objects: %v", err)
	}
	return db
}

func seedLedgerRow(t *testing.T, db *gorm.DB, mediaObjectID, variant string) {
	t.Helper()
	if err := db.Exec(`INSERT INTO media.media_variant_failures (media_object_id, variant, reason, failed_at)
		VALUES (?, ?, ?, ?)`, mediaObjectID, variant, ReasonUndecodable, time.Now().UTC()).Error; err != nil {
		t.Fatalf("seed ledger row (%s,%s): %v", mediaObjectID, variant, err)
	}
}

func seedParent(t *testing.T, db *gorm.DB, id string, deletedAt *time.Time) {
	t.Helper()
	if err := db.Exec(`INSERT INTO media.media_objects (id, fleet_id, bucket, object_key, status, deleted_at)
		VALUES (?, 'fleet-1', 'media', ?, 'ready', ?)`, id, "k/"+id, deletedAt).Error; err != nil {
		t.Fatalf("seed media object %s: %v", id, err)
	}
}

func TestDeleteForMediaObject_removesTheLedgerForThatObjectOnly(t *testing.T) {
	db := newLedgerPurgeDB(t)
	seedLedgerRow(t, db, "mo-1", "card")
	seedLedgerRow(t, db, "mo-1", "display")
	seedLedgerRow(t, db, "mo-2", "card")

	if err := DeleteForMediaObject(db, "mo-1"); err != nil {
		t.Fatalf("DeleteForMediaObject: %v", err)
	}
	var mine, others int64
	db.Raw(`SELECT count(*) FROM media.media_variant_failures WHERE media_object_id = 'mo-1'`).Scan(&mine)
	db.Raw(`SELECT count(*) FROM media.media_variant_failures WHERE media_object_id = 'mo-2'`).Scan(&others)
	if mine != 0 {
		t.Errorf("left %d ledger rows for the purged media object", mine)
	}
	if others != 1 {
		t.Errorf("deleted another media object's ledger rows: %d of 1 left", others)
	}
}

// FR-RECON-1/2/4. Absence of the parent row is the test — a soft-deleted but
// still-present media object keeps its ledger.
func TestDeleteOrphaned_keepsRowsWhoseMediaObjectStillExists(t *testing.T) {
	db := newLedgerPurgeDB(t)
	past := time.Now().UTC().Add(-time.Hour)
	seedParent(t, db, "mo-live", nil)
	seedParent(t, db, "mo-soft", &past)
	seedLedgerRow(t, db, "mo-live", "card")
	seedLedgerRow(t, db, "mo-soft", "card")
	seedLedgerRow(t, db, "mo-gone", "card")

	n, err := DeleteOrphaned(db, 100)
	if err != nil {
		t.Fatalf("DeleteOrphaned: %v", err)
	}
	if n != 1 {
		t.Errorf("DeleteOrphaned removed %d rows, want exactly the one orphan", n)
	}
	var live, soft, gone int64
	db.Raw(`SELECT count(*) FROM media.media_variant_failures WHERE media_object_id = 'mo-live'`).Scan(&live)
	db.Raw(`SELECT count(*) FROM media.media_variant_failures WHERE media_object_id = 'mo-soft'`).Scan(&soft)
	db.Raw(`SELECT count(*) FROM media.media_variant_failures WHERE media_object_id = 'mo-gone'`).Scan(&gone)
	if live != 1 || soft != 1 {
		t.Errorf("reconciliation reached rows with a surviving parent: live=%d soft=%d, want 1 and 1", live, soft)
	}
	if gone != 0 {
		t.Errorf("the orphaned row survived: %d left", gone)
	}
}

// A-2. The cap counts MEDIA OBJECTS, not rows, so an object's ledger is never
// left half-reconciled — and the first deployment against a large backlog does
// not open one enormous transaction.
func TestDeleteOrphaned_capsByMediaObjectAndLeavesTheRemainder(t *testing.T) {
	db := newLedgerPurgeDB(t)
	for _, id := range []string{"mo-a", "mo-b", "mo-c"} {
		seedLedgerRow(t, db, id, "card")
		seedLedgerRow(t, db, id, "display")
	}

	n, err := DeleteOrphaned(db, 2)
	if err != nil {
		t.Fatalf("DeleteOrphaned: %v", err)
	}
	if n != 4 {
		t.Errorf("DeleteOrphaned(limit 2) removed %d rows, want 4 — both rows of each of two media objects", n)
	}
	var left int64
	db.Raw(`SELECT count(*) FROM media.media_variant_failures`).Scan(&left)
	if left != 2 {
		t.Errorf("%d rows survived, want the 2 belonging to the one media object over the cap", left)
	}
}
```

- [x] **Step 2: Run the tests to verify they fail**

```sh
go test github.com/jtumidanski/myfleet/apps/media-service/internal/variantfailures/ -run 'TestDeleteForMediaObject|TestDeleteOrphaned' -v
```

Expected: FAIL to compile — `undefined: DeleteForMediaObject`, `undefined: DeleteOrphaned`.

- [x] **Step 3: Write the implementation**

Create `apps/media-service/internal/variantfailures/purge.go`:

```go
package variantfailures

import "gorm.io/gorm"

// DeleteForMediaObject hard-deletes the ledger rows for a media object.
//
// db may be a transaction — the purge passes the tx so the ledger rows, the
// variant rows and the media-object row all go together (FR-PURGE-4).
func DeleteForMediaObject(db *gorm.DB, mediaObjectID string) error {
	return db.Exec(`DELETE FROM media.media_variant_failures WHERE media_object_id = ?`,
		mediaObjectID).Error
}

// DeleteOrphaned removes ledger rows whose media object no longer exists and
// returns how many rows went. The ledger carries no object_key, so there are no
// bytes to reclaim first (FR-RECON-4) and this is a single statement.
//
// limit bounds MEDIA OBJECTS, not rows (design A-2). Grouping by media object
// keeps an object's ledger atomic rather than half-reconciled, and still bounds
// the transaction on a first deployment against a large accumulated backlog.
//
// The DELETE target carries no alias: `DELETE FROM t AS f` is Postgres-only and
// SQLite — the whole test harness — rejects it, so the sub-query does the
// aliasing instead. This is the portability trap fleet-service's
// admin/orphans.go already documents.
func DeleteOrphaned(db *gorm.DB, limit int) (int, error) {
	q := `DELETE FROM media.media_variant_failures
	      WHERE media_object_id IN (
	          SELECT media_object_id FROM media.media_variant_failures f
	           WHERE NOT EXISTS (SELECT 1 FROM media.media_objects o WHERE o.id = f.media_object_id)
	           GROUP BY media_object_id
	           LIMIT ?)`
	res := db.Exec(q, limit)
	if res.Error != nil {
		return 0, res.Error
	}
	return int(res.RowsAffected), nil
}
```

- [x] **Step 4: Run the tests to verify they pass**

```sh
go test github.com/jtumidanski/myfleet/apps/media-service/internal/variantfailures/ -v
```

Expected: PASS, all of them.

- [x] **Step 5: Commit**

```sh
git add apps/media-service/internal/variantfailures/purge.go apps/media-service/internal/variantfailures/purge_test.go
git commit -m "feat(media-service): add the purge-side variant-failure ledger queries"
```

---

### Task 8: Move the sweep into `internal/purge`, behaviour unchanged

This task is a **pure move**. `RunOnce` is a verbatim port of today's `purgeExpired` — original object key only, media row only. That is deliberate: it separates the relocation from the behaviour change so a reviewer can gate each independently, and it gives Task 9's tests a genuine pre-change baseline to go red against (FR-TEST-5).

**Files:**
- Create: `apps/media-service/internal/purge/sweeper.go`
- Create: `apps/media-service/internal/purge/testdb_test.go`
- Create: `apps/media-service/internal/purge/sweeper_test.go`
- Modify: `apps/media-service/cmd/main.go:123-136` (wiring), `:176-193` (delete `purgeExpired`), import block

**Interfaces:**
- Consumes: `mediaobject.ListPurgeable`, `mediaobject.DeleteRow`.
- Produces, all in `package purge`:
  - `type ObjectRemover interface { RemoveObject(ctx context.Context, key string) error }`
  - `type Config struct { ReconcileLimit int }`
  - `type Sweeper struct{ … }` with `func NewSweeper(log logrus.FieldLogger, db *gorm.DB, store ObjectRemover, cfg Config) *Sweeper` and `func (s *Sweeper) RunOnce(ctx context.Context) error`.
  - Test helpers used by Tasks 9–10: `newTestDB(t) *gorm.DB`, `newRemover() *recordingRemover` with fields `asked []string`, `removed []string`, `fail map[string]bool` and methods `wasAsked(key string) bool` / `didRemove(key string) bool`, plus `seedMediaObject`, `seedVariant`, `seedLedgerRow`, `countRows`.

- [x] **Step 1: Write the package and the verbatim port**

Create `apps/media-service/internal/purge/sweeper.go`:

```go
// Package purge is media-service's time-based hard-delete sweep: it removes
// soft-deleted media objects whose five-day recovery window has elapsed,
// together with every byte and row derived from them.
//
// It is NOT the admin purge protocol. internal/admin serves an operator-driven,
// cancellable stamp/restore/reap lifecycle keyed on purge_operation_id; this
// package is the unattended hourly sweep keyed on purge_after, and
// ListPurgeable's `purge_operation_id IS NULL` is the seam that keeps the two
// off each other's rows.
//
// It imports mediaobject, mediavariant and variantfailures directly rather than
// receiving composition-root adapters (design D-1). The invariant that matters —
// those three packages stay independent of ONE ANOTHER — is unchanged and, for
// the first time, enforced by a test (arch_test.go). Ports whose only
// implementations lived in package main would leave this package's tests unable
// to exercise the production SQL, and assertions about stored rows are the whole
// point of the suite. ObjectRemover stays a port for the ordinary reason: MinIO
// cannot be in a unit test.
package purge

import (
	"context"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/apps/media-service/internal/mediaobject"
)

// ObjectRemover is the slice of storage.Client the sweep needs. Declaring the
// port here rather than importing the concrete client keeps the dependency
// one-way and makes the sweep testable without MinIO (FR-TEST-3).
type ObjectRemover interface {
	RemoveObject(ctx context.Context, key string) error
}

// Config is the sweep's tunable surface.
type Config struct {
	// ReconcileLimit bounds orphan rows processed per tick (FR-RECON-5).
	// 0 or negative disables the reconciliation pass entirely.
	ReconcileLimit int
}

// Sweeper runs one purge tick.
type Sweeper struct {
	log   logrus.FieldLogger
	db    *gorm.DB
	store ObjectRemover
	cfg   Config
}

// NewSweeper builds a Sweeper. Nothing is started; the caller owns the ticker.
func NewSweeper(log logrus.FieldLogger, db *gorm.DB, store ObjectRemover, cfg Config) *Sweeper {
	return &Sweeper{log: log, db: db, store: store, cfg: cfg}
}

// RunOnce executes one tick.
//
// Call it under database.WithLeaderLock(db, "media-purge", …) so only one
// replica sweeps per tick (FR-RECON-8).
func (s *Sweeper) RunOnce(ctx context.Context) error {
	objs, err := mediaobject.ListPurgeable(s.db)
	if err != nil {
		return err
	}
	for _, o := range objs {
		if err := s.store.RemoveObject(ctx, o.ObjectKey()); err != nil {
			s.log.WithError(err).WithField("media_id", o.ID()).
				Warn("remove minio object during purge failed")
			continue // leave the row so a later sweep retries
		}
		if err := mediaobject.DeleteRow(s.db, o.ID()); err != nil {
			s.log.WithError(err).WithField("media_id", o.ID()).
				Warn("delete media row during purge failed")
		}
	}
	return nil
}
```

- [x] **Step 2: Write the test harness**

Create `apps/media-service/internal/purge/testdb_test.go`:

```go
package purge

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"github.com/jtumidanski/myfleet/apps/media-service/internal/mediaobject"
	"github.com/jtumidanski/myfleet/apps/media-service/internal/mediavariant"
	"github.com/jtumidanski/myfleet/apps/media-service/internal/variantfailures"
)

// newTestDB opens an in-memory SQLite database with a "media" schema attached.
//
// media_objects and media_variants are hand-written, matching every other test
// in this service. That is forced, not preferred: both entities carry
// gorm:"index" tags, and AutoMigrate emits its CREATE INDEX WITHOUT the schema
// qualifier (`ON media_variants`, not `ON media.media_variants`), which fails on
// SQLite with "no such table: main.media_variants". The real
// mediavariant.ApplyPartialIndexes and variantfailures.Migration DO run, so the
// index semantics the purge depends on are production's.
//
// Hand-written DDL drifts from the struct — that is a real hazard and it is why
// TestHarnessDDLMatchesTheEntities exists below rather than being left to
// vigilance.
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// The ":memory:" DSN carries no cache=shared, so every physical connection
	// is an independent empty database. Capping the pool at one keeps the ATTACH
	// and the DDL from applying to a different connection than a later query
	// lands on.
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.Exec("ATTACH DATABASE ':memory:' AS media").Error; err != nil {
		t.Fatalf("attach media schema: %v", err)
	}
	ddl := []string{
		`CREATE TABLE media.media_objects (
			id TEXT PRIMARY KEY, fleet_id TEXT, uploaded_by_user_id TEXT, bucket TEXT,
			object_key TEXT, content_type TEXT, size INTEGER, original_filename TEXT,
			status TEXT, created_at DATETIME, deleted_at DATETIME, purge_after DATETIME,
			purge_operation_id TEXT)`,
		`CREATE TABLE media.media_variants (
			id TEXT PRIMARY KEY, media_object_id TEXT NOT NULL, variant TEXT NOT NULL,
			object_key TEXT NOT NULL, width INTEGER, height INTEGER, content_type TEXT,
			created_at DATETIME, deleted_at DATETIME, purge_operation_id TEXT)`,
	}
	for _, q := range ddl {
		if err := db.Exec(q).Error; err != nil {
			t.Fatalf("ddl: %v", err)
		}
	}
	// The partial unique index is production's, not a plain UNIQUE: it is what
	// lets a live and a soft-deleted row share (media_object_id, variant), which
	// is exactly the case the purge has to remove both halves of.
	if err := mediavariant.ApplyPartialIndexes(db); err != nil {
		t.Fatalf("apply partial indexes: %v", err)
	}
	if err := variantfailures.Migration(db); err != nil {
		t.Fatalf("variantfailures migration: %v", err)
	}
	return db
}

// TestHarnessDDLMatchesTheEntities is the guard that makes hand-written DDL
// acceptable. A column added to an entity but not to the DDL above would make
// every row assertion in this package test a schema production does not have —
// a red test turning green for the wrong reason, which is the failure mode this
// whole task exists to correct.
func TestHarnessDDLMatchesTheEntities(t *testing.T) {
	db := newTestDB(t)
	for _, c := range []struct {
		table string
		model any
	}{
		{"media_objects", &mediaobject.Entity{}},
		{"media_variants", &mediavariant.Entity{}},
		{"media_variant_failures", &variantfailures.Entity{}},
	} {
		s, err := schema.Parse(c.model, &sync.Map{}, db.NamingStrategy)
		if err != nil {
			t.Fatalf("parse %s schema: %v", c.table, err)
		}
		var want []string
		for _, f := range s.Fields {
			if f.DBName != "" {
				want = append(want, f.DBName)
			}
		}
		var got []string
		if err := db.Raw(`SELECT name FROM media.pragma_table_info(?)`, c.table).Scan(&got).Error; err != nil {
			t.Fatalf("pragma_table_info(%s): %v", c.table, err)
		}
		sort.Strings(want)
		sort.Strings(got)
		if len(want) == 0 {
			t.Fatalf("%s: parsed no columns from the entity — this check would pass vacuously", c.table)
		}
		if len(want) != len(got) {
			t.Fatalf("%s columns drifted:\n  entity: %v\n  table : %v", c.table, want, got)
		}
		for i := range want {
			if want[i] != got[i] {
				t.Fatalf("%s columns drifted:\n  entity: %v\n  table : %v", c.table, want, got)
			}
		}
	}
}

// recordingRemover captures the keys the sweep asks to remove, and can be told
// to fail for a chosen one (FR-TEST-3).
//
// asked and removed are separate on purpose. Several tests assert a key was
// never OFFERED to the remover, which is a different and stronger claim than
// "the row survived" — the latter is trivially true in code that never looks at
// variants at all.
type recordingRemover struct {
	asked   []string
	removed []string
	fail    map[string]bool
}

func newRemover(failKeys ...string) *recordingRemover {
	fail := map[string]bool{}
	for _, k := range failKeys {
		fail[k] = true
	}
	return &recordingRemover{fail: fail}
}

func (r *recordingRemover) RemoveObject(_ context.Context, key string) error {
	r.asked = append(r.asked, key)
	if r.fail[key] {
		return errors.New("object store unavailable")
	}
	r.removed = append(r.removed, key)
	return nil
}

func (r *recordingRemover) wasAsked(key string) bool  { return contains(r.asked, key) }
func (r *recordingRemover) didRemove(key string) bool { return contains(r.removed, key) }

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

// seedMediaObject inserts a media object. purgeAfter in the past with a nil
// operation ID is what ListPurgeable selects; opID non-nil is an admin-stamped
// object it must skip.
func seedMediaObject(t *testing.T, db *gorm.DB, id string, purgeAfter *time.Time, opID *string) {
	t.Helper()
	var deletedAt *time.Time
	if purgeAfter != nil {
		deletedAt = purgeAfter
	}
	if err := db.Exec(`INSERT INTO media.media_objects
		(id, fleet_id, uploaded_by_user_id, bucket, object_key, content_type, status,
		 created_at, deleted_at, purge_after, purge_operation_id)
		VALUES (?, 'fleet-1', 'user-1', 'media', ?, 'image/jpeg', 'ready', ?, ?, ?, ?)`,
		id, "k/"+id, time.Now().UTC(), deletedAt, purgeAfter, opID).Error; err != nil {
		t.Fatalf("seed media object %s: %v", id, err)
	}
}

func seedVariant(t *testing.T, db *gorm.DB, id, mediaObjectID, variant, key string, deletedAt *time.Time) {
	t.Helper()
	if err := db.Exec(`INSERT INTO media.media_variants
		(id, media_object_id, variant, object_key, content_type, created_at, deleted_at)
		VALUES (?, ?, ?, ?, 'image/jpeg', ?, ?)`,
		id, mediaObjectID, variant, key, time.Now().UTC(), deletedAt).Error; err != nil {
		t.Fatalf("seed variant %s: %v", id, err)
	}
}

func seedLedgerRow(t *testing.T, db *gorm.DB, mediaObjectID, variant string) {
	t.Helper()
	if err := db.Exec(`INSERT INTO media.media_variant_failures
		(media_object_id, variant, reason, failed_at) VALUES (?, ?, 'undecodable', ?)`,
		mediaObjectID, variant, time.Now().UTC()).Error; err != nil {
		t.Fatalf("seed ledger row (%s,%s): %v", mediaObjectID, variant, err)
	}
}

func countRows(t *testing.T, db *gorm.DB, query string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := db.Raw(query, args...).Scan(&n).Error; err != nil {
		t.Fatalf("count (%s): %v", query, err)
	}
	return n
}

func hourAgo() *time.Time {
	p := time.Now().UTC().Add(-time.Hour)
	return &p
}
```

- [x] **Step 3: Write T5 — the guard that protects the move**

Create `apps/media-service/internal/purge/sweeper_test.go`:

```go
package purge

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"
)

func newSweeper(t *testing.T, db *gorm.DB, store ObjectRemover, cfg Config) *Sweeper {
	t.Helper()
	log := logrus.New()
	log.SetOutput(io.Discard)
	return NewSweeper(log, db, store, cfg)
}

// T5 / FR-PURGE-5. An admin-stamped object belongs to a cancellable operation
// whose lifecycle the admin reaper owns. The sweep must not hard-delete it and,
// worse, must not remove its MinIO object, which no restore could bring back.
//
// This test is written BEFORE the sweep is rewritten and must stay green
// throughout: the one thing a relocation can silently lose is the
// purge_operation_id IS NULL narrowing.
func TestRunOnce_leavesAdminStampedObjectsAlone(t *testing.T) {
	db := newTestDB(t)
	opID := "op-1"
	seedMediaObject(t, db, "mo-user", hourAgo(), nil)
	seedMediaObject(t, db, "mo-admin", hourAgo(), &opID)

	store := newRemover()
	if err := newSweeper(t, db, store, Config{}).RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	if store.wasAsked("k/mo-admin") {
		t.Error("the sweep removed an admin-stamped object's bytes; no restore could bring them back")
	}
	if n := countRows(t, db, `SELECT count(*) FROM media.media_objects WHERE id = 'mo-admin'`); n != 1 {
		t.Errorf("the sweep hard-deleted an admin-stamped media object: %d of 1 left", n)
	}
	if n := countRows(t, db, `SELECT count(*) FROM media.media_objects WHERE id = 'mo-user'`); n != 0 {
		t.Errorf("the sweep did not purge the user-deleted object: %d rows left", n)
	}
	if !store.didRemove("k/mo-user") {
		t.Error("the sweep did not remove the purged object's own bytes")
	}
}
```

Add `"io"` and `"gorm.io/gorm"` to that file's imports.

- [x] **Step 4: Run the new package's tests**

```sh
go test github.com/jtumidanski/myfleet/apps/media-service/internal/purge/ -v
```

Expected: PASS — `TestHarnessDDLMatchesTheEntities` and `TestRunOnce_leavesAdminStampedObjectsAlone`. If the drift guard fails, the DDL in `newTestDB` is missing a column an entity declares; add it.

- [x] **Step 5: Wire the sweeper into `cmd/main.go` and delete `purgeExpired`**

In `apps/media-service/cmd/main.go`, replace the purge job block (lines 123-136) with:

```go
	// Media purge: hard-delete soft-deleted objects past purge_after, removing
	// both the rows and the MinIO objects. Under advisory lock so only one
	// replica runs per tick (design §10.6 / A9). Hourly, not daily: jobs.Every's
	// first tick is at T+interval, so a 24-hour sweep in a service that
	// redeploys more often than daily never runs (design OQ-5).
	sweeper := purge.NewSweeper(log, db, store, purge.Config{})
	go jobs.Every(ctx, 1*time.Hour, func(ctx context.Context) error {
		_, err := database.WithLeaderLock(db, "media-purge", func() error {
			return sweeper.RunOnce(ctx)
		})
		if err != nil {
			log.WithError(err).Warn("media purge sweep failed")
		}
		return err
	})
```

Delete the `purgeExpired` function entirely (lines 176-193). Add
`"github.com/jtumidanski/myfleet/apps/media-service/internal/purge"` to the intra-repo import group, and remove the now-unused `"gorm.io/gorm"` import. (`logrus` stays — `mustJWKSKeyfunc` still takes a `*logrus.Logger`.)

- [x] **Step 6: Verify the service builds and the whole suite passes**

```sh
go build github.com/jtumidanski/myfleet/apps/media-service/...
go vet  github.com/jtumidanski/myfleet/apps/media-service/...
go test github.com/jtumidanski/myfleet/apps/media-service/...
```

Expected: all PASS. `apps/media-service/internal/mediaobject` still passes `TestListPurgeable_skipsAdminStampedObjects` unmodified.

- [x] **Step 7: Commit**

```sh
git add apps/media-service/internal/purge/ apps/media-service/cmd/main.go
git commit -m "refactor(media-service): move the purge sweep into internal/purge"
```

---

### Task 9: Per-object purge completeness

The behaviour change. Every byte and every row derived from a purged media object goes, in the right order, with a partial failure never destroying the handle on what remains.

**Files:**
- Modify: `apps/media-service/internal/purge/sweeper.go`
- Modify: `apps/media-service/internal/mediaobject/purge.go:41-44` (doc line, D-2)
- Test: `apps/media-service/internal/purge/sweeper_test.go` (append T1–T4)

**Interfaces:**
- Consumes: `mediavariant.ObjectKeysForMediaObject`, `mediavariant.DeleteForMediaObject` (Task 6); `variantfailures.DeleteForMediaObject` (Task 7); `mediaobject.DeleteRow`.
- Produces: `RunOnce` keeps its signature. A private `summary` type and `func (s *Sweeper) purgeExpired(ctx context.Context, sum *summary) error` are added; Task 10 adds `reconcile` alongside them.

- [x] **Step 1: Write the failing tests**

Append to `apps/media-service/internal/purge/sweeper_test.go`:

```go
// T1 / FR-PURGE-1/2/3. The whole point: four keys removed — the original and
// all three variants, including the soft-deleted one — and nothing left in any
// of the three tables.
func TestRunOnce_removesEveryByteAndRowForAPurgedObject(t *testing.T) {
	db := newTestDB(t)
	seedMediaObject(t, db, "mo-1", hourAgo(), nil)
	seedVariant(t, db, "mv-t", "mo-1", "thumbnail", "k/mo-1-thumbnail", nil)
	seedVariant(t, db, "mv-c", "mo-1", "card", "k/mo-1-card", nil)
	seedVariant(t, db, "mv-d", "mo-1", "display", "k/mo-1-display", nil)
	// A soft-deleted card left behind by an earlier admin purge. The partial
	// unique index admits it alongside the live one, and its bytes leak unless
	// the sweep removes them too (FR-PURGE-1).
	seedVariant(t, db, "mv-old", "mo-1", "card", "k/mo-1-card-old", hourAgo())
	seedLedgerRow(t, db, "mo-1", "card")

	store := newRemover()
	if err := newSweeper(t, db, store, Config{}).RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	for _, key := range []string{
		"k/mo-1", "k/mo-1-thumbnail", "k/mo-1-card", "k/mo-1-display", "k/mo-1-card-old",
	} {
		if !store.didRemove(key) {
			t.Errorf("%s was never removed — its bytes leak forever once the rows are gone", key)
		}
	}
	for _, c := range []struct {
		what  string
		query string
	}{
		{"variant", `SELECT count(*) FROM media.media_variants WHERE media_object_id = 'mo-1'`},
		{"ledger", `SELECT count(*) FROM media.media_variant_failures WHERE media_object_id = 'mo-1'`},
		{"media object", `SELECT count(*) FROM media.media_objects WHERE id = 'mo-1'`},
	} {
		if n := countRows(t, db, c.query); n != 0 {
			t.Errorf("%d %s rows survived the purge", n, c.what)
		}
	}
}

// T2 / FR-PURGE-7. A failing VARIANT removal must leave every row in place —
// including the media object's, which today's code deletes regardless. The
// object must still be purgeable on the next tick.
func TestRunOnce_aFailedVariantRemovalKeepsEveryRow(t *testing.T) {
	db := newTestDB(t)
	seedMediaObject(t, db, "mo-1", hourAgo(), nil)
	seedVariant(t, db, "mv-c", "mo-1", "card", "k/mo-1-card", nil)
	seedLedgerRow(t, db, "mo-1", "card")
	// A second object that must still be processed: one object's failure must
	// not abort the sweep.
	seedMediaObject(t, db, "mo-2", hourAgo(), nil)

	store := newRemover("k/mo-1-card")
	if err := newSweeper(t, db, store, Config{}).RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce must not return a per-object failure: %v", err)
	}

	if n := countRows(t, db, `SELECT count(*) FROM media.media_objects WHERE id = 'mo-1'`); n != 1 {
		t.Error("the media row was deleted even though a variant's bytes survive — " +
			"the variant row is now unreachable and its bytes are stranded")
	}
	if n := countRows(t, db, `SELECT count(*) FROM media.media_variants WHERE media_object_id = 'mo-1'`); n != 1 {
		t.Errorf("%d of 1 variant rows left; a partial failure must delete nothing", n)
	}
	if n := countRows(t, db, `SELECT count(*) FROM media.media_variant_failures WHERE media_object_id = 'mo-1'`); n != 1 {
		t.Errorf("%d of 1 ledger rows left; a partial failure must delete nothing", n)
	}
	if n := countRows(t, db, `SELECT count(*) FROM media.media_objects WHERE id = 'mo-2'`); n != 0 {
		t.Error("one object's failure aborted the sweep; the remaining objects must still be processed")
	}

	// And the next sweep must still see it.
	objs, err := mediaobject.ListPurgeable(db)
	if err != nil {
		t.Fatalf("ListPurgeable: %v", err)
	}
	if len(objs) != 1 || objs[0].ID() != "mo-1" {
		t.Errorf("ListPurgeable = %+v, want mo-1 still awaiting retry", objs)
	}
}

// T3 / FR-PURGE-7. A failing ORIGINAL removal behaves identically. "The variant
// rows survive" would pass vacuously in code that never touches variants, so
// this asserts the stronger claim: the variant keys were never OFFERED to the
// remover, because the object was abandoned before any of them was reached.
func TestRunOnce_aFailedOriginalRemovalOffersNoVariantKeys(t *testing.T) {
	db := newTestDB(t)
	seedMediaObject(t, db, "mo-1", hourAgo(), nil)
	seedVariant(t, db, "mv-c", "mo-1", "card", "k/mo-1-card", nil)
	seedLedgerRow(t, db, "mo-1", "card")

	store := newRemover("k/mo-1")
	if err := newSweeper(t, db, store, Config{}).RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	if store.wasAsked("k/mo-1-card") {
		t.Error("the sweep kept removing bytes after the original failed; " +
			"the object is abandoned as a unit or not at all")
	}
	if n := countRows(t, db, `SELECT count(*) FROM media.media_variants WHERE media_object_id = 'mo-1'`); n != 1 {
		t.Errorf("%d of 1 variant rows left", n)
	}
	if n := countRows(t, db, `SELECT count(*) FROM media.media_objects WHERE id = 'mo-1'`); n != 1 {
		t.Errorf("%d of 1 media rows left", n)
	}
}

// T4 / FR-PURGE-8. Retry safety, asserted rather than assumed. A crash between
// the byte removals and the transaction leaves rows pointing at absent objects;
// the next sweep re-issues removal for keys that are already gone. S3 DELETE is
// idempotent and storage.Client.RemoveObject returns nil for a missing key, so
// the retry must proceed all the way to the row deletion.
func TestRunOnce_retryAfterBytesAreAlreadyGoneCompletesThePurge(t *testing.T) {
	db := newTestDB(t)
	seedMediaObject(t, db, "mo-1", hourAgo(), nil)
	seedVariant(t, db, "mv-c", "mo-1", "card", "k/mo-1-card", nil)
	seedLedgerRow(t, db, "mo-1", "card")

	// First tick: the bytes go, then the process "crashes" before the rows do.
	failing := newRemover("k/mo-1-card")
	if err := newSweeper(t, db, failing, Config{}).RunOnce(context.Background()); err != nil {
		t.Fatalf("first RunOnce: %v", err)
	}
	if n := countRows(t, db, `SELECT count(*) FROM media.media_objects WHERE id = 'mo-1'`); n != 1 {
		t.Fatalf("setup: the object should still be awaiting retry, got %d rows", n)
	}

	// Second tick: the bucket is healthy and the already-absent keys are no-ops.
	store := newRemover()
	if err := newSweeper(t, db, store, Config{}).RunOnce(context.Background()); err != nil {
		t.Fatalf("retry RunOnce: %v", err)
	}
	for _, c := range []struct {
		what  string
		query string
	}{
		{"variant", `SELECT count(*) FROM media.media_variants WHERE media_object_id = 'mo-1'`},
		{"ledger", `SELECT count(*) FROM media.media_variant_failures WHERE media_object_id = 'mo-1'`},
		{"media object", `SELECT count(*) FROM media.media_objects WHERE id = 'mo-1'`},
	} {
		if n := countRows(t, db, c.query); n != 0 {
			t.Errorf("the retry left %d %s rows behind", n, c.what)
		}
	}
}
```

Add `"github.com/jtumidanski/myfleet/apps/media-service/internal/mediaobject"` to that file's intra-repo import group.

- [x] **Step 2: Run the tests to verify they fail**

```sh
go test github.com/jtumidanski/myfleet/apps/media-service/internal/purge/ -v
```

Expected: FAIL, and specifically:
- `TestRunOnce_removesEveryByteAndRowForAPurgedObject` — `k/mo-1-thumbnail was never removed …` plus surviving variant and ledger rows. Only the original is handled today.
- `TestRunOnce_aFailedVariantRemovalKeepsEveryRow` — `the media row was deleted even though a variant's bytes survive …`. Today the variant removal never happens, so nothing stops the media row from going.
- `TestRunOnce_retryAfterBytesAreAlreadyGoneCompletesThePurge` — `the retry left 1 variant rows behind`.

`TestRunOnce_aFailedOriginalRemovalOffersNoVariantKeys` **passes** at this point, because today's sweep never offers variant keys at all. Step 5 makes it a real check.

- [x] **Step 3: Rewrite the per-object pass**

Replace `RunOnce` in `apps/media-service/internal/purge/sweeper.go` with the following, and extend the import block with `"fmt"`, `"github.com/jtumidanski/myfleet/apps/media-service/internal/mediavariant"` and `"github.com/jtumidanski/myfleet/apps/media-service/internal/variantfailures"`:

```go
// summary accumulates what one tick actually completed, so a quiet tick can log
// nothing. This job runs hourly forever and finds nothing on almost every run.
type summary struct {
	mediaObjectsPurged int
	objectsRemoved     int
}

func (sum summary) empty() bool {
	return sum.mediaObjectsPurged == 0 && sum.objectsRemoved == 0
}

func (sum summary) log(log logrus.FieldLogger) {
	if sum.empty() {
		return
	}
	log.WithFields(logrus.Fields{
		"media_objects_purged": sum.mediaObjectsPurged,
		"objects_removed":      sum.objectsRemoved,
	}).Info("media purge sweep completed")
}

// RunOnce executes one tick.
//
// Call it under database.WithLeaderLock(db, "media-purge", …) so only one
// replica sweeps per tick (FR-RECON-8).
//
// It returns an error only for a failure that aborts a whole pass — the
// ListPurgeable query itself. Per-object failures are logged and stepped over,
// never returned: returning one would abandon every media object behind it in
// the list, which is exactly what FR-PURGE-7 forbids.
func (s *Sweeper) RunOnce(ctx context.Context) error {
	var sum summary
	err := s.purgeExpired(ctx, &sum)
	sum.log(s.log)
	return err
}

// purgeExpired is the per-object pass: for each media object whose recovery
// window has elapsed, remove every byte it owns and then every row that
// describes it.
func (s *Sweeper) purgeExpired(ctx context.Context, sum *summary) error {
	objs, err := mediaobject.ListPurgeable(s.db)
	if err != nil {
		return fmt.Errorf("list purgeable media objects: %w", err)
	}
	for _, o := range objs {
		variantKeys, err := mediavariant.ObjectKeysForMediaObject(s.db, o.ID())
		if err != nil {
			s.log.WithError(err).WithField("media_id", o.ID()).
				Warn("list variant object keys during purge failed")
			continue
		}

		// Bytes before rows, always (FR-PURGE-6). The rows are the only record
		// of which objects exist, so deleting them first would strand the bytes
		// in the bucket with nothing left pointing at them.
		//
		// The original goes first so that a bucket-wide outage abandons the
		// object before any of its derived bytes are touched.
		removed := 0
		failed := false
		for _, key := range append([]string{o.ObjectKey()}, variantKeys...) {
			if rerr := s.store.RemoveObject(ctx, key); rerr != nil {
				s.log.WithError(rerr).WithFields(logrus.Fields{
					"media_id": o.ID(), "object_key": key,
				}).Warn("remove minio object during purge failed")
				failed = true
				break
			}
			removed++
		}
		if failed {
			// EVERY row for this object stays put, so the handle on whatever
			// bytes remain survives and a later sweep retries the object whole
			// (FR-PURGE-7). Deleting the subset whose bytes did go would strand
			// the rest — the defect this task exists to eliminate.
			continue
		}

		// One transaction per media object, not one per tick: FR-PURGE-7 and
		// FR-PURGE-9 both require the sweep to log one object's failure and
		// CONTINUE, and a tick-wide transaction would roll the successful
		// objects back with the failed one. Everything inside uses tx.
		if terr := s.db.Transaction(func(tx *gorm.DB) error {
			if err := mediavariant.DeleteForMediaObject(tx, o.ID()); err != nil {
				return err
			}
			if err := variantfailures.DeleteForMediaObject(tx, o.ID()); err != nil {
				return err
			}
			return mediaobject.DeleteRow(tx, o.ID())
		}); terr != nil {
			// The bytes are already gone. The surviving rows are retried on the
			// next sweep, and removal of an absent key is a no-op, so that retry
			// completes rather than wedging (FR-PURGE-8/9).
			s.log.WithError(terr).WithField("media_id", o.ID()).
				Warn("delete rows during purge failed")
			continue
		}

		// Counted only on a completed object, so the summary reads as "what this
		// tick finished". A partial failure has already logged at WARN.
		sum.mediaObjectsPurged++
		sum.objectsRemoved += removed
	}
	return nil
}
```

- [x] **Step 4: Record that `DeleteRow` may take a transaction**

In `apps/media-service/internal/mediaobject/purge.go`, replace the `DeleteRow` doc comment (line 41) with:

```go
// DeleteRow hard-deletes a single media-object row by ID (purge job).
//
// db may be a transaction — *gorm.DB is what db.Transaction hands its callback,
// and the sweep passes the tx so the media row goes in the same transaction as
// the variant and ledger rows that describe it (FR-PURGE-4). Do not bind this to
// an outer handle.
```

- [x] **Step 5: Run the tests to verify they pass**

```sh
go test github.com/jtumidanski/myfleet/apps/media-service/internal/purge/ -v
```

Expected: PASS, all of them, including `TestRunOnce_leavesAdminStampedObjectsAlone` from Task 8.

- [x] **Step 6: Demonstrate T3 can go red**

T3 could not fail against the pre-change code, so prove it against the new code instead — revert the fix and watch it go red:

```sh
# In purgeExpired's key loop, change `break` to a plain continue so a failed
# original no longer abandons the object's remaining keys.
go test github.com/jtumidanski/myfleet/apps/media-service/internal/purge/ -run TestRunOnce_aFailedOriginalRemovalOffersNoVariantKeys -v
```

Expected: FAIL — `the sweep kept removing bytes after the original failed; the object is abandoned as a unit or not at all`. Restore the `break` and re-run to confirm PASS. **Do not commit the mutated version.**

- [x] **Step 7: Verify the whole service**

```sh
go vet  github.com/jtumidanski/myfleet/apps/media-service/...
go test github.com/jtumidanski/myfleet/apps/media-service/...
```

Expected: PASS.

- [x] **Step 8: Commit**

```sh
git add apps/media-service/internal/purge/ apps/media-service/internal/mediaobject/purge.go
git commit -m "fix(media-service): purge every variant byte and row with the media object"
```

---

### Task 10: The reconciliation pass

Drains the backlog of orphaned derived state already accumulated in deployed databases, and stays permanently as the safety net that makes the lazy-generation race self-healing.

**Files:**
- Modify: `apps/media-service/internal/purge/sweeper.go`
- Modify: `apps/media-service/cmd/main.go` (the `purge.Config` literal)
- Modify: `deploy/k8s/base/media-service/configmap.yaml` (P-3)
- Test: `apps/media-service/internal/purge/reconcile_test.go`

**Interfaces:**
- Consumes: `mediavariant.ListOrphaned`, `mediavariant.DeleteByID` (Task 6); `variantfailures.DeleteOrphaned` (Task 7); `Config.ReconcileLimit` (Task 8).
- Produces: `func (s *Sweeper) reconcile(ctx context.Context, sum *summary) error`; `summary` gains `orphanVariants int`, `orphanFailures int`, `reconcileCapped bool`.

- [x] **Step 1: Write the failing tests**

Create `apps/media-service/internal/purge/reconcile_test.go`:

```go
package purge

import (
	"context"
	"testing"
)

// T6 / FR-RECON-1/2/3. The orphan's bytes and row go; a variant whose media
// object is soft-deleted but still PRESENT is untouched. The negative half is
// the important one: that object is still inside its five-day recovery window,
// and reconciling its variants would silently break restore.
func TestReconcile_takesOrphansAndSparesRecoverableVariants(t *testing.T) {
	db := newTestDB(t)
	// Live parent.
	seedMediaObject(t, db, "mo-live", nil, nil)
	seedVariant(t, db, "mv-live", "mo-live", "card", "k/live", nil)
	// Soft-deleted but present parent — inside the recovery window.
	seedMediaObject(t, db, "mo-soft", hourAgo(), nil)
	seedVariant(t, db, "mv-soft", "mo-soft", "card", "k/soft", nil)
	// True orphan: no parent row at all.
	seedVariant(t, db, "mv-orphan", "mo-gone", "card", "k/orphan", nil)

	store := newRemover()
	// mo-soft's purge_after is in the past, so exclude it from the per-object
	// pass by giving the sweeper only the reconciliation job to do: seed it as
	// soft-deleted with NO purge_after instead.
	if err := db.Exec(`UPDATE media.media_objects SET purge_after = NULL WHERE id = 'mo-soft'`).Error; err != nil {
		t.Fatalf("clear purge_after: %v", err)
	}

	if err := newSweeper(t, db, store, Config{ReconcileLimit: 500}).RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	if !store.didRemove("k/orphan") {
		t.Error("the orphaned variant's bytes were never removed")
	}
	if n := countRows(t, db, `SELECT count(*) FROM media.media_variants WHERE id = 'mv-orphan'`); n != 0 {
		t.Error("the orphaned variant row survived")
	}
	if store.wasAsked("k/soft") {
		t.Error("reconciliation removed the bytes of a variant whose media object is still " +
			"recoverable; restore is now broken and no cancel can undo it")
	}
	if n := countRows(t, db, `SELECT count(*) FROM media.media_variants WHERE id = 'mv-soft'`); n != 1 {
		t.Error("reconciliation deleted a variant of a soft-deleted-but-present media object")
	}
	if n := countRows(t, db, `SELECT count(*) FROM media.media_variants WHERE id = 'mv-live'`); n != 1 {
		t.Error("reconciliation deleted a variant of a live media object")
	}
}

// T7 / FR-RECON-5. With more orphans than the cap, exactly the cap is processed
// and the remainder survives to the next tick. One tick must not turn into an
// unbounded bucket-deletion loop on first deployment.
func TestReconcile_processesAtMostTheCapPerTick(t *testing.T) {
	db := newTestDB(t)
	for _, id := range []string{"mv-1", "mv-2", "mv-3", "mv-4", "mv-5"} {
		seedVariant(t, db, id, "mo-gone", "card-"+id, "k/"+id, nil)
	}

	store := newRemover()
	if err := newSweeper(t, db, store, Config{ReconcileLimit: 2}).RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	if len(store.removed) != 2 {
		t.Errorf("removed %d objects (%v), want exactly the cap of 2", len(store.removed), store.removed)
	}
	if n := countRows(t, db, `SELECT count(*) FROM media.media_variants`); n != 3 {
		t.Errorf("%d orphan rows left, want the 3 over the cap", n)
	}

	// The remainder must drain on subsequent ticks, not be lost.
	next := newRemover()
	if err := newSweeper(t, db, next, Config{ReconcileLimit: 2}).RunOnce(context.Background()); err != nil {
		t.Fatalf("second RunOnce: %v", err)
	}
	if n := countRows(t, db, `SELECT count(*) FROM media.media_variants`); n != 1 {
		t.Errorf("%d orphan rows left after the second tick, want 1", n)
	}
}

// FR-RECON-3. A failed byte removal spares exactly its own row, so the bytes
// stay reachable — the row's object_key is the only record of them.
func TestReconcile_aFailedRemovalKeepsThatOrphanRow(t *testing.T) {
	db := newTestDB(t)
	seedVariant(t, db, "mv-bad", "mo-gone", "card", "k/bad", nil)
	seedVariant(t, db, "mv-ok", "mo-gone", "display", "k/ok", nil)

	store := newRemover("k/bad")
	if err := newSweeper(t, db, store, Config{ReconcileLimit: 500}).RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	if n := countRows(t, db, `SELECT count(*) FROM media.media_variants WHERE id = 'mv-bad'`); n != 1 {
		t.Error("the row whose bytes could not be removed was deleted; those bytes are now unreachable")
	}
	if n := countRows(t, db, `SELECT count(*) FROM media.media_variants WHERE id = 'mv-ok'`); n != 0 {
		t.Error("one orphan's failure spared an unrelated orphan")
	}
}

// T8 / FR-RECON-1/4. Orphaned ledger rows are deleted directly — they carry no
// object_key, so there are no bytes to reclaim first — and a ledger row whose
// media object is still present is untouched.
func TestReconcile_takesOrphanedLedgerRowsOnly(t *testing.T) {
	db := newTestDB(t)
	seedMediaObject(t, db, "mo-live", nil, nil)
	seedLedgerRow(t, db, "mo-live", "card")
	seedLedgerRow(t, db, "mo-gone", "card")

	if err := newSweeper(t, db, newRemover(), Config{ReconcileLimit: 500}).RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	if n := countRows(t, db, `SELECT count(*) FROM media.media_variant_failures WHERE media_object_id = 'mo-gone'`); n != 0 {
		t.Error("the orphaned ledger row survived")
	}
	if n := countRows(t, db, `SELECT count(*) FROM media.media_variant_failures WHERE media_object_id = 'mo-live'`); n != 1 {
		t.Error("reconciliation deleted a ledger row whose media object still exists")
	}
}

// A-3. The operator off switch. A background pass that touches object storage
// needs one that is not a rollback.
func TestReconcile_isDisabledByANonPositiveLimit(t *testing.T) {
	db := newTestDB(t)
	seedVariant(t, db, "mv-orphan", "mo-gone", "card", "k/orphan", nil)
	seedLedgerRow(t, db, "mo-gone", "card")

	store := newRemover()
	if err := newSweeper(t, db, store, Config{ReconcileLimit: 0}).RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	if store.wasAsked("k/orphan") {
		t.Error("reconciliation ran with the limit at 0; the off switch does not work")
	}
	if n := countRows(t, db, `SELECT count(*) FROM media.media_variants`); n != 1 {
		t.Error("reconciliation deleted a row with the limit at 0")
	}
	if n := countRows(t, db, `SELECT count(*) FROM media.media_variant_failures`); n != 1 {
		t.Error("ledger reconciliation ran with the limit at 0")
	}
}
```

- [x] **Step 2: Run the tests to verify they fail**

```sh
go test github.com/jtumidanski/myfleet/apps/media-service/internal/purge/ -run TestReconcile -v
```

Expected: FAIL on all but `TestReconcile_isDisabledByANonPositiveLimit` (which passes vacuously today — there is no reconciliation to disable; Step 4 makes it real). The others fail with `the orphaned variant's bytes were never removed`, `removed 0 objects …, want exactly the cap of 2`, and `the orphaned ledger row survived`.

- [x] **Step 3: Extend `summary`**

In `apps/media-service/internal/purge/sweeper.go`, replace the `summary` type and its two methods with:

```go
// summary accumulates what one tick actually completed, so a quiet tick can log
// nothing. This job runs hourly forever and finds nothing on almost every run.
type summary struct {
	mediaObjectsPurged int
	objectsRemoved     int
	orphanVariants     int
	orphanFailures     int
	// reconcileCapped is a heuristic, not a fact: exactly `limit` orphans could
	// be exactly all of them. It is reported rather than swallowed because a
	// silently truncated cleanup reads as "finished" when it is not
	// (FR-RECON-6).
	reconcileCapped bool
}

func (sum summary) empty() bool {
	return sum.mediaObjectsPurged == 0 && sum.objectsRemoved == 0 &&
		sum.orphanVariants == 0 && sum.orphanFailures == 0
}

func (sum summary) log(log logrus.FieldLogger) {
	if sum.empty() {
		return
	}
	log.WithFields(logrus.Fields{
		"media_objects_purged":       sum.mediaObjectsPurged,
		"objects_removed":            sum.objectsRemoved,
		"orphan_variants_reconciled": sum.orphanVariants,
		"orphan_failures_deleted":    sum.orphanFailures,
		"reconcile_capped":           sum.reconcileCapped,
	}).Info("media purge sweep completed")
}
```

- [x] **Step 4: Add the reconciliation pass and call it from `RunOnce`**

In `apps/media-service/internal/purge/sweeper.go`, replace `RunOnce` with:

```go
// RunOnce executes one tick: the per-object pass, then reconciliation.
//
// Call it under database.WithLeaderLock(db, "media-purge", …) so only one
// replica sweeps per tick (FR-RECON-8).
//
// It returns an error only for a failure that aborts a whole pass — the
// ListPurgeable query, or the orphan query. Per-object and per-orphan failures
// are logged and stepped over, never returned: returning one would abandon
// every item behind it, which is exactly what FR-PURGE-7 forbids.
//
// Both passes run even if the first one fails. They are independent — the
// reconciliation pass is defined by the ABSENCE of a media object, so it shares
// no state with the per-object pass — and the self-healing safety net should not
// be silenced by an unrelated query failure.
func (s *Sweeper) RunOnce(ctx context.Context) error {
	var sum summary
	perObjectErr := s.purgeExpired(ctx, &sum)
	reconcileErr := s.reconcile(ctx, &sum)
	sum.log(s.log)
	if perObjectErr != nil {
		return perObjectErr
	}
	return reconcileErr
}

// reconcile removes derived state whose media object no longer exists: variant
// rows (and the bytes they are the last record of) and variant-failure ledger
// rows.
//
// It is permanent, not a one-shot migration (FR-RECON-7). Once the backlog left
// by earlier versions drains it matches zero rows per tick and costs one
// anti-join per table — an honest recurring cost, which is why ReconcileLimit
// <= 0 is an off switch rather than something an operator has to roll back to
// escape. It stays because lazy card generation writes variant rows from REQUEST
// context, outside the media-purge lock, so a variant row can in principle
// appear for a media object the sweep is deleting. A per-tick pass makes that
// race self-healing within an hour.
func (s *Sweeper) reconcile(ctx context.Context, sum *summary) error {
	if s.cfg.ReconcileLimit <= 0 {
		return nil
	}

	orphans, err := mediavariant.ListOrphaned(s.db, s.cfg.ReconcileLimit)
	if err != nil {
		return fmt.Errorf("list orphaned variants: %w", err)
	}
	// Each row is handled individually so one failed byte removal spares exactly
	// its own row. Volume is bounded by the cap, so the per-row statement cost is
	// acceptable; a batched delete would lose the sparing.
	for _, v := range orphans {
		if rerr := s.store.RemoveObject(ctx, v.ObjectKey); rerr != nil {
			// The row survives, so its object_key — the only remaining record of
			// these bytes — stays reachable for the next tick (FR-RECON-3).
			s.log.WithError(rerr).WithFields(logrus.Fields{
				"variant_id": v.ID, "object_key": v.ObjectKey,
			}).Warn("remove orphaned variant object failed")
			continue
		}
		if derr := mediavariant.DeleteByID(s.db, v.ID); derr != nil {
			s.log.WithError(derr).WithField("variant_id", v.ID).
				Warn("delete orphaned variant row failed")
			continue
		}
		sum.orphanVariants++
	}
	sum.reconcileCapped = len(orphans) == s.cfg.ReconcileLimit

	// The ledger carries no object_key, so orphans there are deleted directly
	// (FR-RECON-4).
	n, err := variantfailures.DeleteOrphaned(s.db, s.cfg.ReconcileLimit)
	if err != nil {
		return fmt.Errorf("delete orphaned variant failures: %w", err)
	}
	sum.orphanFailures += n
	return nil
}
```

- [x] **Step 5: Run the tests to verify they pass**

```sh
go test github.com/jtumidanski/myfleet/apps/media-service/internal/purge/ -v
```

Expected: PASS, all of them — T1–T8 plus the harness drift guard.

- [x] **Step 6: Demonstrate the off-switch test can go red**

`TestReconcile_isDisabledByANonPositiveLimit` passed vacuously in Step 2. Prove it is now a real check:

```sh
# In reconcile, change `if s.cfg.ReconcileLimit <= 0` to `if s.cfg.ReconcileLimit < 0`.
go test github.com/jtumidanski/myfleet/apps/media-service/internal/purge/ -run TestReconcile_isDisabledByANonPositiveLimit -v
```

Expected: FAIL — `reconciliation ran with the limit at 0; the off switch does not work`. Restore `<= 0` and re-run to confirm PASS. **Do not commit the mutated version.**

- [x] **Step 7: Read the cap from configuration**

In `apps/media-service/cmd/main.go`, replace the `purge.Config{}` literal added in Task 8 with:

```go
	// MEDIA_PURGE_RECONCILE_LIMIT bounds the orphan rows reconciled per tick.
	// "0" turns the reconciliation pass off entirely, matching the
	// MEDIA_LAZY_VARIANT_CONCURRENCY convention: a background pass that deletes
	// from object storage needs an off switch that is not a rollback. At an
	// hourly cadence the default drains 12,000 orphans a day.
	sweeper := purge.NewSweeper(log, db, store, purge.Config{
		ReconcileLimit: config.GetInt("MEDIA_PURGE_RECONCILE_LIMIT", 500),
	})
```

- [x] **Step 8: Add the knob to the ConfigMap**

In `deploy/k8s/base/media-service/configmap.yaml`, after the `MEDIA_LAZY_VARIANT_CONCURRENCY` block:

```yaml
  # How many orphaned derived rows the hourly purge sweep reconciles per tick
  # (task-023). Orphans are media_variants and media_variant_failures rows whose
  # media object no longer exists — state earlier versions of the sweep left
  # behind. The cap keeps the first tick after a deploy short instead of turning
  # it into an unbounded bucket-deletion loop against an accumulated backlog.
  # "0" turns the reconciliation pass off entirely; the per-object purge is
  # unaffected either way.
  MEDIA_PURGE_RECONCILE_LIMIT: "500"
```

- [x] **Step 9: Verify the service and the manifests**

```sh
go vet  github.com/jtumidanski/myfleet/apps/media-service/...
go test github.com/jtumidanski/myfleet/apps/media-service/...
kustomize build deploy/k8s/overlays/local >/dev/null
kustomize build deploy/k8s/overlays/main  | grep -c PersistentVolumeClaim
kustomize build deploy/k8s/overlays/main  | grep MEDIA_PURGE_RECONCILE_LIMIT
```

Expected: tests PASS; the local overlay renders; the `main` overlay reports `0` PVCs; the new key appears in the `main` render.

- [x] **Step 10: Commit**

```sh
git add apps/media-service/internal/purge/ apps/media-service/cmd/main.go \
        deploy/k8s/base/media-service/configmap.yaml
git commit -m "feat(media-service): reconcile orphaned variant rows and ledger rows each tick"
```

---

### Task 11: Make sibling-package independence a check

FR-TEST-2's invariant has only ever been prose in `cmd/main.go`'s adapter comments. `internal/purge` imports all three domain packages directly (design D-1), which is acceptable precisely because the three stay independent of one another — so that has to stop being an honour system.

**Files:**
- Create: `apps/media-service/internal/purge/arch_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: nothing.

- [x] **Step 1: Write the test**

Create `apps/media-service/internal/purge/arch_test.go`:

```go
package purge

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestDomainPackagesDoNotImportEachOther makes FR-TEST-2's invariant a check
// rather than a comment.
//
// mediaobject, mediavariant and variantfailures are siblings: cross-package
// access goes through ports satisfied by composition-root adapters (the
// variantLookup and cardGenerator adapters in cmd/main.go), never through a
// direct import. This package is deliberately exempt — it is a job package with
// no consumers, and importing all three is composition, not coupling (design
// D-1) — which is exactly why the invariant needs enforcing somewhere.
func TestDomainPackagesDoNotImportEachOther(t *testing.T) {
	const prefix = "github.com/jtumidanski/myfleet/apps/media-service/internal/"
	siblings := []string{"mediaobject", "mediavariant", "variantfailures"}

	parsed := 0
	for _, pkg := range siblings {
		dir := filepath.Join("..", pkg)
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read %s: %v", dir, err)
		}
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			path := filepath.Join(dir, name)
			f, perr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
			if perr != nil {
				t.Fatalf("parse %s: %v", path, perr)
			}
			parsed++
			for _, imp := range f.Imports {
				p, uerr := strconv.Unquote(imp.Path.Value)
				if uerr != nil {
					t.Fatalf("%s: unquote %s: %v", path, imp.Path.Value, uerr)
				}
				for _, other := range siblings {
					if other == pkg || p != prefix+other {
						continue
					}
					t.Errorf("%s imports its sibling %s — cross-package access must go "+
						"through a port satisfied in the composition root (FR-TEST-2)", path, other)
				}
			}
		}
	}
	if parsed == 0 {
		t.Fatal("parsed no files — the walk root is wrong, and this test would pass vacuously")
	}
}
```

- [x] **Step 2: Run the test to verify it passes**

```sh
go test github.com/jtumidanski/myfleet/apps/media-service/internal/purge/ -run TestDomainPackagesDoNotImportEachOther -v
```

Expected: PASS.

- [x] **Step 3: Demonstrate the test can go red**

```sh
# Temporarily add this to apps/media-service/internal/mediaobject/purge.go's imports:
#   _ "github.com/jtumidanski/myfleet/apps/media-service/internal/mediavariant"
go test github.com/jtumidanski/myfleet/apps/media-service/internal/purge/ -run TestDomainPackagesDoNotImportEachOther -v
```

Expected: FAIL — `../mediaobject/purge.go imports its sibling mediavariant …`. Remove the import and re-run to confirm PASS. **Do not commit the temporary import.**

- [x] **Step 4: Commit**

```sh
git add apps/media-service/internal/purge/arch_test.go
git commit -m "test(media-service): enforce sibling-package independence in the domain layer"
```

---

### Task 12: Full verification and code review

**Files:**
- Modify: `docs/tasks/task-023-purge-variant-cleanup/plan.md` (tick the boxes)
- Create: `docs/tasks/task-023-purge-variant-cleanup/audit.md` (written by the reviewer agents)

- [x] **Step 1: Run the full CI gate**

Node is not always on `PATH`:

```sh
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
make ci
```

Expected: PASS — `lint-check`, `vet`, `test`, `build`, `fe-test`, `fe-build`, `manifests`, `carfax-template`. No frontend file changed in this task, so `fe-test` / `fe-build` are a no-regression check only.

If `lint-check` reports import-grouping or gofumpt findings in the new files, run `make lint` and re-run `make ci`.

- [x] **Step 2: Render both overlays and check the `main` invariants**

```sh
kustomize build deploy/k8s/overlays/local >/dev/null && echo "local OK"
kustomize build deploy/k8s/overlays/main > /tmp/main-render.yaml && echo "main OK"
grep -c 'kind: PersistentVolumeClaim' /tmp/main-render.yaml   # must be 0
grep -c 'kind: Secret'                 /tmp/main-render.yaml   # must be 0
grep -c 'kind: ClusterRole'            /tmp/main-render.yaml   # must be 0
grep    'MEDIA_PURGE_RECONCILE_LIMIT'  /tmp/main-render.yaml
```

Expected: both render; the three counts are `0` (`grep -c` exits non-zero when it finds nothing — that is the passing case here); the new key is present.

- [x] **Step 3: Server dry-run both overlays, if a cluster is reachable**

```sh
kubectl config current-context   # confirm this is the intended cluster before proceeding
kustomize build deploy/k8s/overlays/main  | kubectl apply --dry-run=server -f -
kustomize build deploy/k8s/overlays/local | kubectl apply --dry-run=server -f -
```

`--dry-run=server` persists nothing, so it is safe against the shared `bee` context. **Both** are required — the local overlay is not exempt; a missing `namespace:` once slipped through ten reviews because only the `main` dry-run was ever run. If no cluster is reachable, say so explicitly in the summary rather than silently skipping.

- [x] **Step 4: Confirm the acceptance criteria**

Walk PRD §10 and check each box against the work. In particular verify by inspection:

```sh
grep -n 'purgeExpired' apps/media-service/cmd/main.go   # must find nothing
git -C . diff main --stat -- apps/media-service/internal/mediaobject/purge.go
```

Expected: no `purgeExpired` in `cmd/main.go`; the only change to `mediaobject/purge.go` is the `DeleteRow` doc comment, with `ListPurgeable` and `TestListPurgeable_skipsAdminStampedObjects` untouched.

- [x] **Step 5: Run code review**

Per CLAUDE.md this is mandatory before a PR, even when the plan looks complete. Go files changed and no frontend files did, so dispatch the plan-adherence and backend reviewers:

Use `superpowers:requesting-code-review`, or dispatch `plan-adherence-reviewer` and `backend-guidelines-reviewer` directly in parallel. Both write to `docs/tasks/task-023-purge-variant-cleanup/audit.md`.

Address any findings before opening the PR. Findings that are wrong should be argued with evidence, not implemented reflexively — use `superpowers:receiving-code-review`.

- [ ] **Step 6: Tick the plan and commit the artifacts**

```sh
git add docs/tasks/task-023-purge-variant-cleanup/
git commit -m "docs(task-023): record the code-review audit and completed plan"
```

- [ ] **Step 7: Open the PR**

The PR body must reference and close issue #28. Do not open it until Steps 1–6 have all passed.

---

## Self-Review

Run against `design.md` and `prd.md` with fresh eyes.

**Spec coverage.** Every functional requirement maps to a task:

| Requirement | Task |
| --- | --- |
| FR-PURGE-1, -2, -3 | 6 (queries), 9 (T1) |
| FR-PURGE-4 (one transaction) | 9 |
| FR-PURGE-5 (`ListPurgeable` untouched) | 8 (T5), 12 Step 4 |
| FR-PURGE-6 (bytes before rows) | 9 |
| FR-PURGE-7 (partial failure keeps everything) | 9 (T2, T3) |
| FR-PURGE-8 (retry safety asserted) | 9 (T4) |
| FR-PURGE-9 (transaction failure logs and continues) | 9 |
| FR-RECON-1, -3, -4 | 10 (T6, T8) |
| FR-RECON-2 (parent-row test, not `deleted_at`) | 6, 7, 10 (T6) |
| FR-RECON-5, -6 (cap and its logging) | 7, 10 (T7) |
| FR-RECON-7 (permanent, not one-shot) | 10 |
| FR-RECON-8 (inside the leader lock) | 8 Step 5, 10 Step 7 |
| FR-TEST-1 (out of `cmd/main.go`) | 8 |
| FR-TEST-2 (sibling independence) | 11 (T9) |
| FR-TEST-3 (`ObjectRemover` port) | 8 |
| FR-TEST-4 cases 1–7 | 9 (T1–T4), 8 (T5), 10 (T6–T8) |
| FR-TEST-5 (each test shown red) | every task names its red demonstration; Tasks 4, 9, 10, 11 use mutation because the pre-change code cannot fail those checks |
| FR-ADMIN-1 (manifest entry) | 3 |
| FR-ADMIN-2 (entity columns) | 2 — with deviation P-1 on the index mechanism |
| FR-ADMIN-3 (`Recorded` narrowing) | 2 (T12) |
| FR-ADMIN-4 (`ReapableObjectKeys` unchanged) | not a task; verified — the ledger holds no `object_key` |
| FR-ADMIN-5 (widened walk) | 1, 4 (T10) |
| FR-ADMIN-6 (parse errors and vacuity fatal) | 1, 4 |
| A-1 (`ReapSparing`) | 5 |
| A-2 (ledger cap by media object) | 7 |
| A-3 (off switch) | 10 |
| A-4 (T9, T10) | 11, 1 |
| D-2 (`DeleteRow` doc only) | 9 Step 4 |
| PRD §8 observability | 9, 10 (`summary`), 12 |

Design §12 open questions OQ-1 and OQ-3 recommend filing follow-up issues; both are out of scope here and are recorded in `context.md` for the PR description rather than implemented.

**Placeholder scan.** No "TBD", no "add error handling", no "similar to Task N". Every code step carries the actual code. The three mutation-based red demonstrations (Tasks 4, 9, 10, 11) name the exact edit to make and the exact message to expect.

**Type consistency.** `ObjectRemover.RemoveObject(ctx context.Context, key string) error` is identical in Task 8's port and Task 8's `recordingRemover`. `mediavariant.Orphan{ID, ObjectKey}` (Task 6) is consumed as `v.ID` / `v.ObjectKey` in Task 10. `variantfailures.DeleteOrphaned` returns `(int, error)` in Task 7 and is consumed as `n, err` in Task 10. `Config.ReconcileLimit` is declared in Task 8 and read in Tasks 10. `summary` is declared in Task 9 with two fields and extended in Task 10 with three more; Task 10 Step 3 replaces the whole type rather than assuming a partial edit. `CollectTableNames(root string) ([]string, error)` is declared in Task 1 and called in Task 4. `ApplyIndexes` is introduced in Task 2 and is not called by any later task — `Migration` calls it, and no harness in this plan hand-writes the ledger table, so that is correct and not a dangling reference.
