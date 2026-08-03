# Media Purge Variant Cleanup — Design

Task: task-023 · Tracks [#28](https://github.com/jtumidanski/MyFleet/issues/28)
PRD: `docs/tasks/task-023-purge-variant-cleanup/prd.md` (approved, v1)
Status: Draft for review
Created: 2026-08-03

---

## 1. Starting point

Everything the sweep needs already exists in the service; nothing is wired
together.

| Fact | Location |
| --- | --- |
| The sweep is 15 lines in `package main`, untestable, and removes only the original object + the `media.media_objects` row | `apps/media-service/cmd/main.go:176-193` |
| The correct byte-before-row ordering, and the reason for it, is written down | `internal/admin/operations.go:131-148`, `internal/admin/resource.go:101-124` |
| Variant `object_key` is a stored column, authoritative for what was written | `internal/mediavariant/entity.go:18-37` |
| `RemoveObject` is a one-method surface, already ported as `admin.ObjectRemover` | `internal/storage/minio.go:207`, `internal/admin/resource.go:19-21` |
| The manifest arch test only parses files literally named `entity.go` | `internal/admin/arch_test.go:39` |
| `variantfailures.Entity` declares its table in `variantfailures.go`, so the arch test cannot see it | `internal/variantfailures/variantfailures.go:41` |
| Sibling-package independence is enforced today by composition-root adapters | `cmd/main.go:195-230` |

Verified counts: media-service declares exactly four `TableName()` methods —
`media.media_objects`, `media.media_variants` (both in `entity.go`, both in
`Manifest`), `media.processed_events` (in `processedevents.go`, in
`excludedTables`) and `media.media_variant_failures` (in `variantfailures.go`,
in neither). Widening the arch-test walk therefore surfaces exactly the two
tables FR-ADMIN-5 predicts, and no others.

There is also a close precedent in the sibling service. fleet-service hit this
exact defect one layer up — a bare `DELETE FROM fleet.vehicles` that stranded
every child row — and fixed it with `vehicle.PurgeExpired(db,
admin.DeleteVehicleChildren)` plus a permanent `admin.DeleteOrphans(db)` pass
driven by an `Orphan *OrphanRule` field on each manifest target
(`apps/fleet-service/internal/vehicle/purge.go`,
`apps/fleet-service/internal/admin/orphans.go:42-94`). That shape is the
baseline this design measures itself against.

---

## 2. Architecture decisions

### D1 — Where the sweep lives

**Decision: a new job package `apps/media-service/internal/purge`, which
imports the three domain packages directly and reaches MinIO through a
one-method port.**

Three options were weighed.

*Option A (chosen) — dedicated job package with direct domain imports.*
`internal/purge` owns the tick: the per-object pass, the reconciliation pass,
the ordering and failure rules, the cap, and the summary log. It calls
`mediaobject`, `mediavariant` and `variantfailures` by name for the SQL, and
declares one consumer-side interface, `ObjectRemover`, for the bytes.
`cmd/main.go` is reduced to `jobs.Every` + `WithLeaderLock` +
`purge.NewSweeper(log, db, store, cfg).RunOnce`.

*Option B — extend `mediaobject.PurgeExpired`, fleet-service style.* Mirrors
`vehicle.PurgeExpired(db, deleteChildren)` most literally. Rejected because the
reconciliation pass is not media-object-scoped at all — it is defined by the
*absence* of a media object — so it has no natural home in the package that owns
that aggregate, and because the sweep now carries a cap, a byte-removal port and
two-pass summary logging, which is more than a domain package's `purge.go`
should hold.

*Option C — put everything in `internal/admin`, manifest-driven.* Add
`Orphan *OrphanRule` and an object-key column to media's `Target`, then reuse
fleet's `DeleteOrphans` shape. Attractive because it makes `Manifest` the single
source of truth for both purge routes. Rejected as the primary structure because
the time-based sweep and the operator-driven protocol have genuinely different
lifecycles — the sweep must remove bytes per orphan row and honour a per-tick
cap, neither of which the protocol has any use for — and because folding a
background job into the package that serves unauthenticated internal DELETE
routes widens that package's blast radius for no gain. Its good idea is kept:
§6 still routes the *admin* path's failure-ledger handling through `Manifest`.

**Why direct imports rather than ports for the database side.** FR-TEST-2
prescribes ports satisfied by composition-root adapters. The invariant it
protects — `mediaobject` must not import `mediavariant`, and neither may import
`variantfailures` — is preserved exactly. The mechanism is deliberately
different, and the reason is testability, which is the other half of the same
requirement:

- FR-TEST-4 demands tests that assert *stored state* ("all four object keys
  removed, all rows for that object gone"). Those assertions are only worth
  anything if the test runs the production SQL.
- If the DB collaborators are ports whose only implementations are adapters
  inside `package main`, `internal/purge`'s tests cannot reach them. The test
  would have to either substitute in-memory fakes — making every row assertion
  vacuous — or re-declare the same SQL in the test file, which is a test that
  passes while production is broken. Both are the failure mode this task exists
  to correct.
- `internal/purge` is a job package with no consumers. Importing all three
  domain packages is composition, not coupling: there is no cycle risk and no
  domain package gains a sibling dependency.

Recorded as deviation **D-1** in §11, with an arch test (§8, T9) that enforces
the invariant FR-TEST-2 actually cares about — today it is prose in a comment,
not a check.

`ObjectRemover` stays a port for the ordinary reason: MinIO cannot be in a unit
test, and FR-TEST-3 requires a fake that records keys and can be made to fail on
a chosen one.

### D2 — One transaction per media object, not one per tick

fleet-service's `vehicle.PurgeExpired` wraps the whole sweep in a single
transaction. media-service must not: FR-PURGE-7 and FR-PURGE-9 both require the
sweep to log one object's failure and *continue*, and a tick-wide transaction
would roll the successful objects back with the failed one. One transaction per
media object, spanning `media_variants` → `media_variant_failures` →
`media_objects`, satisfies FR-PURGE-4 with the smallest scope that makes the
statement "the media row is never deleted while its variant rows survive" true.

Byte removals stay outside the transaction, before it. That is forced: MinIO is
not transactional, and holding a database transaction open across N network
round-trips is the wrong trade. The resulting window — bytes gone, rows still
present — is exactly the state FR-PURGE-8 makes safe, and §5 tabulates it.

### D3 — Reconciliation is a permanent second pass, not a startup migration

fleet-service's orphan cleanup runs once per boot (`cmd/main.go:143-159`).
FR-RECON-7 requires media's to run every tick, and that is the better shape here
for a reason fleet's case did not have: lazy card generation writes variant rows
from *request* context, outside the `media-purge` lock, so a variant row can in
principle appear for a media object the sweep is deleting. A per-tick pass makes
that race self-healing within an hour. A boot-time pass would leave it until the
next deploy.

The pass costs one anti-join per table per tick. Both are keyed on
`media_object_id`, indexed on `media.media_variants` and the leading primary-key
column on `media.media_variant_failures`. On a drained database it scans and
returns nothing. This is an honest recurring cost, not free — §9 gives the
operator an off switch rather than pretending otherwise.

### D4 — The cap bounds both reconciled tables, by media object

FR-RECON-5 caps orphaned *variant* rows because each costs a MinIO round-trip.
This design caps the failure-ledger pass too, at the same limit, selecting by
`media_object_id` rather than by row:

```sql
DELETE FROM media.media_variant_failures
 WHERE media_object_id IN (
     SELECT media_object_id FROM media.media_variant_failures f
      WHERE NOT EXISTS (SELECT 1 FROM media.media_objects o WHERE o.id = f.media_object_id)
      GROUP BY media_object_id
      LIMIT ?)
```

Rationale: it bounds the transaction size on first deployment against a large
backlog, it is portable to the SQLite test harness (no row-value tuples, no
`DELETE … AS alias`, which is Postgres-only — the trap fleet's `DeleteOrphans`
documents at `orphans.go:52-54`), and grouping by media object keeps an object's
ledger rows atomic rather than half-reconciled. Additive to the PRD; no
behaviour the PRD specifies changes.

### D5 — Variant rows are selected with no `deleted_at` predicate at all

FR-PURGE-1 requires soft-deleted variant rows to be included. The query is
therefore `WHERE media_object_id = ?` and nothing else. Note the one odd case
this admits: a variant row carrying a `purge_operation_id` whose parent media
object does not (the residue of a spared or partially-reaped admin operation).
Deleting it is still correct — the media object it describes is being
hard-deleted in the same transaction, so the row could only become an orphan.
`ListPurgeable`'s `purge_operation_id IS NULL` narrowing (FR-PURGE-5) is
untouched and keeps the sweep off *stamped media objects* themselves, which is
the guarantee that matters.

### D6 — Orphan detection tests the parent row, never `deleted_at`

FR-RECON-2. A variant belonging to a soft-deleted-but-recoverable media object
is not an orphan; deleting it would silently break restore, both the user-facing
five-day window and the admin console's cancel. The predicate is
`NOT EXISTS (SELECT 1 FROM media.media_objects o WHERE o.id = v.media_object_id)`
and the design's most important single test (T6) is the negative half of it.

### D7 — The failure ledger joins `Manifest` (OQ-2 resolved)

Confirmed as the PRD proposed, for the reason the PRD gives: excluding it would
leave ledger rows orphaned on the *admin reap* path, which is the same defect on
a different route, and would then need targeted cleanup machinery of its own.
The cost is two nullable columns and a narrowing of `Recorded`. §6 works through
the consequences, including one subtlety the PRD does not call out.

---

## 3. Package layout

```
apps/media-service/internal/
  purge/                    ← new
    sweeper.go              Sweeper, Config, ObjectRemover, RunOnce
    sweeper_test.go         FR-TEST-4 cases 1-5 (per-object pass)
    reconcile_test.go       FR-TEST-4 cases 6-7 (reconciliation pass)
    testdb_test.go          SQLite harness driven by the real Migration funcs
  mediaobject/purge.go      + no signature change (see below)
  mediavariant/purge.go     ← new file: the purge-side queries
  variantfailures/          + two entity columns, Recorded narrowed, purge queries
  admin/manifest.go         + media.media_variant_failures target
  admin/operations.go       + spare the ledger in ReapSparing
  admin/arch_test.go        walk widened; walk extracted for its own test
  admin/tablenames.go       ← new: the walk, so it is testable against a fixture
```

### 3.1 `internal/purge`

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
package purge

// ObjectRemover is the slice of storage.Client the sweep needs. Declaring the
// port here rather than importing the concrete client keeps the dependency
// one-way and makes the sweep testable without MinIO (FR-TEST-3).
type ObjectRemover interface {
    RemoveObject(ctx context.Context, key string) error
}

// Config is the sweep's tunable surface.
type Config struct {
    // ReconcileLimit bounds orphan rows processed per tick (FR-RECON-5).
    // 0 disables the reconciliation pass entirely.
    ReconcileLimit int
}

type Sweeper struct {
    log   logrus.FieldLogger
    db    *gorm.DB
    store ObjectRemover
    cfg   Config
}

func NewSweeper(log logrus.FieldLogger, db *gorm.DB, store ObjectRemover, cfg Config) *Sweeper

// RunOnce executes one tick: the per-object pass, then reconciliation.
// Call it under database.WithLeaderLock(db, "media-purge", …) (FR-RECON-8).
func (s *Sweeper) RunOnce(ctx context.Context) error
```

`RunOnce` returns an error only for failures that abort a whole pass (the
`ListPurgeable` query, the orphan query). Per-object and per-orphan failures are
logged and stepped over, never returned — returning them would abort the
remaining work, which is what FR-PURGE-7 forbids.

### 3.2 `internal/mediavariant/purge.go` (new)

```go
// Orphan is a variant row whose media object no longer exists, paired with the
// bytes it is the only remaining record of.
type Orphan struct {
    ID        string
    ObjectKey string
}

// ObjectKeysForMediaObject returns every stored key for a media object,
// including rows a purge has soft-deleted. Keys come from the object_key
// column, never recomputed from storage.ObjectKey: the column is the record of
// what was actually written, and a recomputed key that disagreed with a
// historical naming scheme would silently skip the bytes (FR-PURGE-2).
func ObjectKeysForMediaObject(db *gorm.DB, mediaObjectID string) ([]string, error)

// DeleteForMediaObject hard-deletes every variant row for a media object.
// db may be a transaction (FR-PURGE-4).
func DeleteForMediaObject(db *gorm.DB, mediaObjectID string) error

// ListOrphaned returns up to limit variant rows whose media object no longer
// exists. Absence of the parent row is the test, never deleted_at: a variant of
// a soft-deleted-but-recoverable object is not an orphan (FR-RECON-2).
func ListOrphaned(db *gorm.DB, limit int) ([]Orphan, error)

// DeleteByID hard-deletes one variant row, used after its bytes are gone.
func DeleteByID(db *gorm.DB, id string) error
```

### 3.3 `internal/variantfailures`

```go
// DeleteForMediaObject hard-deletes the ledger rows for a media object.
func DeleteForMediaObject(db *gorm.DB, mediaObjectID string) error

// DeleteOrphaned removes ledger rows whose media object no longer exists,
// bounded by limit media objects per call. The ledger carries no object_key,
// so there are no bytes to reclaim first (FR-RECON-4).
func DeleteOrphaned(db *gorm.DB, limit int) (int, error)
```

These are package-level functions on `*gorm.DB`, matching `mediaobject.DeleteRow`
and `admin`'s operations, rather than methods on `Store`. `Store` carries a
logger and serves the request-path ledger; the purge queries have no logging and
no request context, and a purge does not belong on the type lazy generation
depends on.

### 3.4 `internal/mediaobject/purge.go`

No signature change. `DeleteRow(db *gorm.DB, id string)` already accepts a
transaction — `*gorm.DB` is what `db.Transaction` hands its callback — so
FR-PURGE-4 is met by passing `tx`. The PRD's "gains a transaction-capable form"
is satisfied by what is already there; the design adds only a doc line recording
that the parameter may be a transaction, so nobody later "fixes" it by binding
the outer handle.

`ListPurgeable` is untouched (FR-PURGE-5), and
`TestListPurgeable_skipsAdminStampedObjects` must pass unmodified.

---

## 4. Data flow

### Pass 1 — per media object

```
for each o in mediaobject.ListPurgeable(db):              # purge_operation_id IS NULL
    keys  := [o.ObjectKey()] + mediavariant.ObjectKeysForMediaObject(db, o.ID())
    for k in keys:
        if store.RemoveObject(ctx, k) fails:
            log WARN media_id=o.ID() object_key=k
            continue to next media object          # nothing deleted for this one
    db.Transaction:
        mediavariant.DeleteForMediaObject(tx, o.ID())
        variantfailures.DeleteForMediaObject(tx, o.ID())
        mediaobject.DeleteRow(tx, o.ID())
    on transaction error: log WARN media_id=o.ID(); continue
```

Bytes before rows, always (FR-PURGE-6) — the rows are the only record of which
objects exist. Removal failure aborts *this object only*, before any row is
touched, so the handle on the remaining state survives intact (FR-PURGE-7).

### Pass 2 — reconciliation

```
if cfg.ReconcileLimit <= 0: skip
orphans := mediavariant.ListOrphaned(db, cfg.ReconcileLimit)
for v in orphans:
    if store.RemoveObject(ctx, v.ObjectKey) fails:
        log WARN variant_id=v.ID object_key=v.ObjectKey
        continue                                    # row survives for next tick
    mediavariant.DeleteByID(db, v.ID)
n := variantfailures.DeleteOrphaned(db, cfg.ReconcileLimit)
capped := len(orphans) == cfg.ReconcileLimit
```

Each orphaned variant row is deleted individually rather than in a batch, so one
failed byte removal spares exactly its own row. Volume is bounded by the cap, so
the per-row statement cost is acceptable; a batched delete would either lose the
sparing or need the failed set threaded back through, for no benefit at 500
rows/tick.

`capped` is reported, not silently swallowed (FR-RECON-6). It is a heuristic —
exactly `limit` orphans could be exactly all of them — so the log says "the cap
was reached; more may remain", not "more remain".

---

## 5. Failure semantics

| Situation | Bytes | Rows | Sweep | Next tick |
| --- | --- | --- | --- | --- |
| Original removal fails | untouched | all kept | continues to next object | retries object (FR-PURGE-7) |
| Variant removal fails | some removed | **all kept**, including the media row | continues to next object | retries object; already-removed keys are no-ops |
| All removals succeed, transaction fails | gone | all kept | continues (FR-PURGE-9) | rows deleted, removals are no-ops |
| Crash between bytes and transaction | gone | all kept | — | same as above (FR-PURGE-8) |
| Orphan variant byte removal fails | untouched | row kept | continues to next orphan | retried, subject to the cap |
| `ListPurgeable` query fails | — | — | pass aborts, `RunOnce` returns error | whole tick retried in an hour |

The retry-safety claim rests on `storage.Client.RemoveObject` returning nil for
a key that is not there. That is S3 `DELETE` semantics and minio-go passes it
through, and `admin/resource.go:114-118` already depends on it in production —
but FR-PURGE-8 requires it *asserted*, not assumed, so T4 (§8) is a test and not
a comment.

---

## 6. Admin purge coverage of the failure ledger

### 6.1 Manifest entry

```go
{
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

Placed between `media_variants` and `media_objects`. Ordering is readability
only — media's `Target.Where` never filters a parent's `deleted_at`, which is
what makes stamp order-independent — but child-to-parent is the documented
convention and FR-ADMIN-1 asks for it.

### 6.2 Entity columns (FR-ADMIN-2)

`DeletedAt *time.Time \`gorm:"index"\`` and `PurgeOperationID *string
\`gorm:"type:uuid;index"\``, mirroring `mediavariant.Entity` exactly. Both
nullable with no default, so `AutoMigrate` adds them without a rewrite; NULL is
correct for every existing row, so there is no backfill. Without them
`admin.Stamp`'s `SET deleted_at = …, purge_operation_id = …` and
`admin.Count`'s `deleted_at IS NULL` fail at *runtime*, not compile time — which
is why the arch test widening in §7 matters more than it looks.

No partial unique index, unlike `media.media_variants`. The composite primary key
`(media_object_id, variant)` is the uniqueness guarantee, and `Record` uses
`clause.OnConflict{DoNothing: true}` with no `Columns`, so Postgres needs no
inferred arbiter and a soft-deleted row cannot be silently updated in place.

### 6.3 The `Recorded` narrowing, and what it implies (FR-ADMIN-3)

`Recorded` gains `AND deleted_at IS NULL`. An admin purge that is still
cancellable must not suppress lazy generation for an object whose deletion may
yet be undone.

The consequence the PRD does not state: while a ledger row is soft-deleted, its
primary-key slot is still occupied, so a fresh `Record` for the same
`(media_object_id, variant)` is a silent no-op and `Recorded` keeps reporting
false. In principle that is an unbounded retry loop. In practice it is
unreachable: `Recorded` is consulted only by `processing.CardGenerator`, reached
only through `Content`, which resolves a *live* media object first — and a media
object whose ledger rows are stamped is itself stamped and therefore soft-deleted
(the manifest stamps parent and child in the same transaction). Restore clears
both columns and the ledger is live again. Documented in the code comment rather
than engineered around; adding a partial index to dodge an unreachable state
would be cost without benefit.

### 6.4 `ReapSparing` (beyond the PRD)

FR-ADMIN-4 correctly says `ReapableObjectKeys` needs no change — the ledger holds
no `object_key`. `ReapSparing` does need one. When a media object's bytes fail to
remove, its row and its variant rows are spared so the next tick can retry; its
ledger rows would be reaped anyway, leaving the operation partially applied. Add
a third case to the existing switch:

```go
case "media.media_variant_failures":
    q += " AND media_object_id NOT IN ?"
```

The surrounding `strings.HasSuffix(q, "NOT IN ?")` arg-append already handles it.
Recorded as addition **A-1** in §11.

### 6.5 API and UI

No route, request or response *shape* changes. `affected_counts` gains a
`media_variant_failures` key. Verified additive: the web type is
`Record<string, number>` (`apps/web/src/types/models/admin.ts:107,122`) and
`BlastRadiusPanel`'s `humanise` sentence-cases any key missing from `LABELS`
(`BlastRadiusPanel.tsx:64-76`), so the new key renders as "Media variant
failures" in the alphabetical tail — exactly where `media_variants` already
renders today. No frontend change, as PRD §5 states.

---

## 7. Arch test widening (FR-ADMIN-5/6)

The walk moves out of the test into `internal/admin/tablenames.go` as

```go
// CollectTableNames parses every non-test .go file under root and returns the
// string literal each `func (X) TableName() string` returns.
func CollectTableNames(root string) ([]string, error)
```

so the widening can itself be tested. `TestManifestCoversEveryTable` calls it
with `".."` and keeps both guards: a parse error is fatal, and an empty result
is fatal so the test cannot pass vacuously.

Three details that decide whether this works:

1. **Skip `_test.go`.** FR-ADMIN-5 says non-test files. Test files carry DDL and
   fixtures; including them would make the check report tables that do not exist
   in production.
2. **Skip `testdata/` directories.** FR-ADMIN-5's acceptance criterion — "adding
   a table in a file not named `entity.go` fails the arch test" — is only
   demonstrable against a fixture. If the fixture lives under
   `internal/admin/testdata/` *and* the production walk descends into
   `testdata/`, the real arch test finds the fixture's table and fails the build.
   The walk must skip `testdata` by name; the fixture test calls
   `CollectTableNames("testdata/…")` directly. This is a genuine build-breaking
   trap and is called out here so it is not discovered during execution.
3. **Parsing, not grepping**, is retained verbatim from the current test's
   reasoning: table names appear in comments, raw SQL and test DDL throughout the
   service.

Verified fallout: exactly two new tables, `media.processed_events` (already
excluded, with a reason) and `media.media_variant_failures` (§6.1 places it in
`Manifest`). The test goes green with no further edits.

---

## 8. Testing strategy

### Harness

`internal/purge`'s SQLite harness runs the **real** `Migration` functions —
`mediaobject.Migration`, `mediavariant.Migration`, `variantfailures.Migration` —
against an `ATTACH DATABASE ':memory:' AS media` connection, following
`variantfailures_test.go:11-27`. Hand-written DDL (the `admin_test.go` and
`mediaobject/purge_test.go` approach) is not used here: this task adds columns to
one of those entities, and a hand-written `CREATE TABLE` is precisely the thing
that drifts from the struct and turns a red test green for the wrong reason.
`mediavariant.Migration` already has a SQLite branch for its partial index.

The `ObjectRemover` fake follows `admin_test.go`'s `recordingRemover`: records
keys removed, `fail map[string]bool` to make a chosen key error.

### Cases

| # | Test | Requirement | Fails before the change because… |
| --- | --- | --- | --- |
| T1 | Object with `thumbnail` + `card` + `display` and a ledger row: 4 keys removed, zero rows left across all three tables | FR-PURGE-1/2/3 | today only the original key is removed and only the media row deleted |
| T2 | Variant byte removal fails → no rows deleted for that object; the object is still returned by the next `ListPurgeable` | FR-PURGE-7 | today variant bytes are never removed, so the failure cannot arise and the media row is deleted regardless |
| T3 | Original byte removal fails → variant rows and ledger rows still present | FR-PURGE-7 | today's `continue` covers the media row only; asserting variants survive would pass vacuously — so T3 also asserts the *keys were never offered* to the remover |
| T4 | Rows present, bytes already absent → removal succeeds and rows are then deleted | FR-PURGE-8 | asserts retry-safety explicitly rather than assuming S3 semantics |
| T5 | Admin-stamped object untouched: bytes and all rows survive | FR-PURGE-5 | guards the narrowing while the caller is rewritten |
| T6 | Orphaned variant: bytes removed, row deleted. Variant of a *soft-deleted but present* media object: untouched | FR-RECON-1/2/3 | no reconciliation exists today |
| T7 | More orphans than the cap → exactly cap processed, remainder survives, capped flag logged | FR-RECON-5/6 | no cap exists today |
| T8 | Orphaned ledger row deleted; ledger row of a present media object untouched | FR-RECON-1/4 | no reconciliation exists today |
| T9 | `mediaobject`, `mediavariant`, `variantfailures` import none of each other | FR-TEST-2 invariant | new — makes D-1's promise a check rather than a comment |
| T10 | `CollectTableNames` over a fixture with a `TableName` in a non-`entity.go` file returns it | FR-ADMIN-5 | the current walk filters on the filename |
| T11 | Fleet-scoped stamp → restore → reap round-trip over `media_variant_failures`, with `affected_counts` reporting the key | FR-ADMIN-1/2 | the table is not in `Manifest` today |
| T12 | `Recorded` is false for a soft-deleted ledger row | FR-ADMIN-3 | `Recorded` does not filter `deleted_at` today |

**FR-TEST-5 discipline.** Every one of T1-T12 is shown red before the
corresponding change lands — by writing it first, or by reverting the fix and
re-running. T3 deserves the extra care noted above: "variant rows survive" is
trivially true in the old code for the wrong reason, so the assertion must be on
what the remover was *asked* to do, not only on what survived. This is the
vacuous-negative-assertion failure mode task-019 exists to prevent.

T9's mechanism: `go/parser` over each package's non-test files, asserting the
import set excludes the other two. It lives in `internal/purge` (the package that
would otherwise be the only witness) or in a small `internal/arch` test — either
is fine; the check is what matters.

---

## 9. Configuration and observability

**`MEDIA_PURGE_RECONCILE_LIMIT`**, `config.GetInt`, default **500**. `<= 0`
disables the reconciliation pass, matching the `MEDIA_LAZY_VARIANT_CONCURRENCY`
convention already in `cmd/main.go:97-109`: a background pass that touches object
storage needs an off switch that is not a rollback. At hourly cadence 500/tick
drains 12,000 orphans/day. No deploy-manifest change — the variable is optional
and defaulted, so `deploy/k8s` is untouched and both `kustomize build` renders
are a no-regression check only.

**Logging.**

- Per-object failure: WARN, fields `media_id` and (for a byte failure)
  `object_key`. Matches the existing sweep's field names.
- Per-orphan failure: WARN, fields `variant_id`, `object_key`.
- One summary at INFO **only when the tick did something**: `media_objects_purged`,
  `objects_removed`, `orphan_variants_reconciled`, `orphan_failures_deleted`,
  `reconcile_capped`. A quiet tick logs nothing (PRD §8) — this runs hourly
  forever and finds nothing on almost every run.
- Fields stay IDs and object keys. No filenames, and error strings are attached
  via `WithError` only, never interpolated into messages, consistent with the
  `variantfailures` package doc's reasoning about user-supplied filenames.

---

## 10. Risks

| Risk | Mitigation |
| --- | --- |
| First deployment against a large backlog makes the first tick long | The cap bounds it; the pass is inside the leader lock so no second replica piles on; `MEDIA_PURGE_RECONCILE_LIMIT=0` stops it outright |
| The anti-join becomes a recurring full scan as `media_variants` grows | Accepted and documented (D3). `media_object_id` is indexed; hourly cadence; the off switch is the escape hatch if a deployment ever proves it costly |
| SQLite harness diverges from Postgres and hides a defect | Harness runs the real `Migration` functions; SQL avoids Postgres-only forms (`DELETE … AS alias`, row-value tuples) that fleet's `orphans.go` already documents as a portability trap |
| The widened arch test picks up a testdata fixture and breaks the build | §7 point 2: the walk skips `testdata` by name, and the fixture is only reached by the explicit-path test |
| Moving the sweep loses the `purge_operation_id IS NULL` narrowing during the rewrite | T5 is written before the move and must stay green throughout; `TestListPurgeable_skipsAdminStampedObjects` is untouched |
| Ledger rows reaped while their media object is spared | §6.4 adds the sparing case (A-1) |

---

## 11. Deviations from and additions to the PRD

- **D-1 (FR-TEST-2, mechanism).** `internal/purge` imports `mediaobject`,
  `mediavariant` and `variantfailures` directly instead of receiving
  composition-root adapters. The invariant FR-TEST-2 protects — the three domain
  packages stay independent of each other — is preserved and, for the first time,
  enforced by a test (T9). Rationale in D1: adapters confined to `package main`
  would leave `internal/purge`'s tests unable to exercise production SQL, which
  would hollow out FR-TEST-4. `ObjectRemover` remains a port per FR-TEST-3.
- **D-2 (Service Impact table).** `mediaobject.DeleteRow` needs no signature
  change; it already accepts a transaction. Only its doc comment changes.
- **A-1 (beyond FR-ADMIN-4).** `admin.ReapSparing` gains a
  `media.media_variant_failures` case so a spared media object keeps its ledger
  rows (§6.4).
- **A-2 (beyond FR-RECON-4/5).** The failure-ledger reconciliation is capped too,
  by media object, to bound the first-deployment transaction (D4).
- **A-3.** `MEDIA_PURGE_RECONCILE_LIMIT <= 0` disables the reconciliation pass —
  an operator off switch, matching the existing lazy-generation convention.
- **A-4.** T9, the sibling-independence arch test, and T10, the
  `CollectTableNames` fixture test, are new tests the PRD does not enumerate.

## 12. Open questions

- **OQ-1 — bytes with no surviving row.** Confirmed out of scope. Nothing in the
  service deletes a variant row without handling its bytes, so the only producer
  is a partially-completed admin reap, and that path already spares rather than
  strands (`ReapSparing`'s doc comment). **Recommendation: file a follow-up issue
  for an operator-triggered bucket audit; do not build it here.**
- **OQ-2 — Manifest versus exclusion.** Resolved: Manifest (D7).
- **OQ-3 — notification-service's twin blind spot.** Out of scope, unchanged.
  `apps/notification-service/internal/inbox/processed.go` declares `TableName`
  outside `entity.go`. **Recommendation: file as a separate issue**, and reuse
  this task's `CollectTableNames` shape when it is picked up.
- **OQ-4 — cap default.** 500 stands. A-3's off switch and the plain
  `config.GetInt` override mean a deployment with an unusually large backlog is
  an operator decision, not a code change.

## 13. Out of scope

Unchanged from PRD §2: the five-day window, the one-hour cadence, the admin
purge lifecycle and HTTP surface, bucket enumeration, and `ON DELETE CASCADE`
foreign keys. No frontend change. No deploy-manifest change.
