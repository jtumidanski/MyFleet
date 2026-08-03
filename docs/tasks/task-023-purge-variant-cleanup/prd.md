# Media Purge Variant Cleanup — Product Requirements Document

Version: v1
Status: Draft
Created: 2026-08-03
Tracks: [#28](https://github.com/jtumidanski/MyFleet/issues/28)
---

## 1. Overview

media-service hard-deletes soft-deleted media objects once their five-day
recovery window elapses. The hourly sweep — `purgeExpired` in
`apps/media-service/cmd/main.go:170-188` — removes the original object from MinIO
and the `media.media_objects` row, and nothing else. Every piece of derived state
survives the purge permanently: the `media.media_variants` rows, the MinIO
objects those rows point at, and any `media.media_variant_failures` row for the
object.

The orphaned rows are the more damaging half. Once the parent
`media.media_objects` row is gone there is no reachable path to a variant —
`Content` resolves the media object first — so the rows are unreachable dead
weight that grows monotonically with every purge, forever. The MinIO objects are
straightforward wasted storage. Because the orphaned variant rows still carry
their own `object_key`, both halves are recoverable from the database alone; only
after the rows are deleted do the bytes become genuinely unreachable.

The correct pattern already exists inside this service. The admin purge protocol's
`admin.ReapableObjectKeys` unions the media objects' keys with their variants'
keys and is documented as "must be called BEFORE Reap: the rows are the only
record of which objects exist, so deleting them first would strand the bytes in
the bucket with nothing left pointing at them"
(`apps/media-service/internal/admin/operations.go:135-153`). The time-based sweep
simply never adopted it. This task brings the two paths into agreement, adds a
bounded reconciliation pass that drains the backlog already accumulated in
deployed databases, and closes an adjacent gap: `media.media_variant_failures`
is in neither `admin.Manifest` nor `admin.excludedTables`, and the architecture
test that is supposed to catch exactly that cannot see it.

This is a pre-existing defect, not a regression. `thumbnail` and `display`
already leaked the same way; task-013 (#26) widened it from two orphaned objects
to three and introduced `media_variant_failures` as a second orphaned table.

## 2. Goals

Primary goals:

- The hourly purge sweep removes **all** state belonging to a purged media
  object: the original object, every variant object, every variant row, and every
  variant-failure row.
- A partial failure never destroys the handle on the remaining state. If any byte
  removal fails, the media object is left entirely intact for the next sweep.
- The backlog of already-orphaned rows and bytes in deployed databases drains
  automatically, without an operator-run migration.
- The sweep becomes unit-testable. It currently lives in `package main` with no
  test file, which is why the defect survived undetected across every variant
  the service has ever produced.
- `media.media_variant_failures` is brought under the admin purge protocol, and
  the architecture test that enumerates purgeable tables is widened so no future
  table can hide from it the same way.

Non-goals:

- Changing the five-day recovery window (`mediaobject.recoveryWindow`) or the
  one-hour sweep interval.
- Changing the admin purge protocol's stamp / restore / reap lifecycle, its
  cancellation semantics, or its HTTP surface.
- Scanning the MinIO bucket for objects that no database row references. Every
  leak this task can observe is described by a surviving row; bytes whose rows
  were already deleted are out of scope (see §9 OQ-1).
- Adding database-level `ON DELETE CASCADE` foreign keys. A cascade cannot remove
  MinIO objects, so explicit deletion is required regardless, and adding
  constraints to populated tables via `AutoMigrate` is risk without benefit.
- Fixing the same architecture-test blind spot in notification-service
  (`internal/inbox/processed.go` declares `TableName` outside `entity.go`). Noted
  in §9 OQ-3, not fixed here.

## 3. User Stories

- As a platform operator, I want a purged media object to leave nothing behind, so
  that a user exercising their right to delete a photo actually has every
  rendition of it removed rather than three derived copies left in the bucket.
- As a platform operator, I want `media.media_variants` to stop growing
  monotonically, so that table size tracks live media rather than all media that
  has ever existed.
- As a platform operator, I want storage consumed by purged media reclaimed, so
  that bucket growth reflects live content.
- As an operator upgrading an existing deployment, I want the rows and bytes
  already leaked by previous versions cleaned up automatically, so that fixing
  the defect does not require me to run a manual migration against production.
- As a developer, I want the purge sweep covered by tests, so that the next
  variant kind added to the service cannot silently reintroduce this leak.
- As a developer adding a table to media-service, I want the architecture test to
  force me to decide whether a purge should reach it, regardless of which file I
  declare the entity in.

## 4. Functional Requirements

### 4.1 Per-object purge completeness

- **FR-PURGE-1** — For each media object returned by `mediaobject.ListPurgeable`,
  the sweep MUST remove from MinIO: the object's own `object_key`, and the
  `object_key` of every `media.media_variants` row whose `media_object_id`
  matches — including rows whose `deleted_at` is non-NULL.
- **FR-PURGE-2** — Variant keys MUST be read from the `object_key` column, never
  recomputed from `storage.ObjectKey(fleetID, mediaObjectID, kind+ext)`. The
  column is the authoritative record of what was written; a recomputed key that
  disagrees with a historical naming scheme would silently skip the bytes.
- **FR-PURGE-3** — After all byte removals succeed, the sweep MUST delete, for
  that media object: all `media.media_variants` rows, all
  `media.media_variant_failures` rows, and the `media.media_objects` row.
- **FR-PURGE-4** — The row deletions in FR-PURGE-3 MUST occur in a single
  database transaction. A failure part-way through must not leave the
  `media.media_objects` row deleted with variant rows surviving — that is
  precisely the state this task exists to eliminate.
- **FR-PURGE-5** — `mediaobject.ListPurgeable`'s existing narrowing
  (`purge_operation_id IS NULL`) MUST be preserved unchanged. Admin-stamped
  objects belong to a cancellable operation the admin reaper owns, and
  `TestListPurgeable_skipsAdminStampedObjects` must continue to pass untouched.

### 4.2 Ordering and failure semantics

- **FR-PURGE-6** — Byte removals MUST precede row deletions. The rows are the
  only record of which objects exist; deleting them first strands the bytes with
  nothing pointing at them.
- **FR-PURGE-7** — If any byte removal for a media object fails, the sweep MUST
  log a warning including the media object's ID and skip that media object
  entirely, leaving **every** row for it in place so a later sweep retries. It
  MUST NOT delete a subset of the rows, and MUST NOT abort the sweep — remaining
  media objects are still processed. This extends the `continue` behaviour already
  used for the original object in `purgeExpired`.
- **FR-PURGE-8** — Retry MUST be safe. A crash between byte removal and row
  deletion leaves rows pointing at absent objects; the next sweep re-issues
  removal for keys that no longer exist. S3 `DELETE` is idempotent and
  `storage.Client.RemoveObject` returns no error for a missing key, so the retry
  proceeds to the row deletion. This property MUST be asserted by a test, not
  assumed.
- **FR-PURGE-9** — If the row-deletion transaction fails, the sweep MUST log a
  warning with the media object's ID and continue to the next media object. The
  bytes are already gone; the surviving rows are retried on the next sweep and
  FR-PURGE-8 makes that safe.

### 4.3 Orphan reconciliation

- **FR-RECON-1** — Each sweep tick MUST, after the per-object pass, reconcile
  orphaned derived state: `media.media_variants` rows whose `media_object_id`
  has no row in `media.media_objects`, and `media.media_variant_failures` rows
  whose `media_object_id` has no row in `media.media_objects`.
- **FR-RECON-2** — Orphan detection MUST test for the **absence of the parent
  row**, not for `deleted_at`. A variant belonging to a soft-deleted-but-still-
  recoverable media object is not an orphan, and deleting it would break restore.
- **FR-RECON-3** — For each orphaned `media.media_variants` row, the
  reconciliation MUST remove the MinIO object named by its `object_key` before
  deleting the row, following FR-PURGE-6/7 ordering: on removal failure, log and
  leave that row for the next tick. Orphaned variant rows are self-describing, so
  their bytes are reclaimable without any bucket enumeration.
- **FR-RECON-4** — `media.media_variant_failures` rows carry no `object_key`;
  orphans there are deleted directly.
- **FR-RECON-5** — Reconciliation MUST process at most a bounded number of
  orphaned variant rows per tick (default 500, configurable). One tick must not
  turn into an unbounded bucket-deletion loop on first deployment against a
  database with a large accumulated backlog.
- **FR-RECON-6** — When the FR-RECON-5 cap is reached, the sweep MUST log at INFO
  with the number processed and the fact that more remain. A silently truncated
  cleanup reads as "finished" when it has not.
- **FR-RECON-7** — Reconciliation MUST remain in place permanently, not behind a
  one-shot migration flag. Once the backlog drains it matches zero rows per tick
  and costs one anti-join query, and it stays as a safety net for any future
  path that deletes a media object without its derived state.
- **FR-RECON-8** — Reconciliation MUST run under the same
  `database.WithLeaderLock(db, "media-purge", …)` as the existing sweep, so only
  one replica reconciles per tick.

### 4.4 Placement and testability

- **FR-TEST-1** — The sweep MUST move out of `apps/media-service/cmd/main.go`
  into a testable package under `apps/media-service/internal/`. `cmd/main.go`
  retains only the `jobs.Every` registration, the leader lock, and the wiring of
  the collaborators.
- **FR-TEST-2** — `mediaobject` MUST NOT import `mediavariant`, and neither MUST
  import `variantfailures`. Cross-package access MUST go through ports satisfied
  by adapters in the composition root, exactly as the existing `variantLookup`
  and `cardGenerator` adapters do (`cmd/main.go:180-210`).
- **FR-TEST-3** — Byte removal MUST be reached through an interface (the method
  set `storage.Client` already satisfies), so tests can substitute a fake that
  records the keys it was asked to remove and can be made to fail on a chosen
  key.
- **FR-TEST-4** — Tests MUST cover, at minimum:
  1. A purgeable object with all three variants (`thumbnail`, `card`, `display`)
     and a `media_variant_failures` row: all four object keys removed, all rows
     for that object gone.
  2. Variant byte-removal failure: no rows deleted for that object, the media
     object still returned by the next `ListPurgeable`.
  3. Original byte-removal failure: variant rows still present (today's
     `continue` behaviour, now covering variants too).
  4. Retry after a crash between bytes and rows: removal of an absent key
     succeeds and the rows are then deleted (FR-PURGE-8).
  5. An admin-stamped object is untouched by the sweep (FR-PURGE-5).
  6. Reconciliation: an orphaned variant row's bytes and row are removed; a
     variant belonging to a soft-deleted-but-present media object is not
     (FR-RECON-2).
  7. Reconciliation cap: with more orphans than the cap, exactly the cap is
     processed and the remainder survives to the next tick (FR-RECON-5).
- **FR-TEST-5** — Each new test MUST be shown to fail against the pre-change
  behaviour. Per project practice, a test that cannot be made to go red is not
  evidence.

### 4.5 Admin purge coverage of the variant-failure ledger

- **FR-ADMIN-1** — `media.media_variant_failures` MUST be added to
  `admin.Manifest`, positioned child-to-parent ahead of `media.media_objects`,
  with `Where` resolving all three scopes:
  - `ScopeSystem` → `1 = 1`
  - `ScopeFleet` → `media_object_id IN (SELECT id FROM media.media_objects WHERE fleet_id IN ?)`
  - `ScopeMediaIDs` → `media_object_id IN ?`
- **FR-ADMIN-2** — Manifest membership requires the columns the protocol operates
  on. `variantfailures.Entity` MUST gain `DeletedAt *time.Time` and
  `PurgeOperationID *string` (indexed, nullable), because `admin.Stamp` writes
  both, `admin.Restore` and `admin.Reap` key on `purge_operation_id`, and
  `admin.Count` filters `deleted_at IS NULL`. Without them the new Manifest entry
  fails at runtime, not at compile time.
- **FR-ADMIN-3** — `Store.Recorded` and `Store.Record` MUST account for the new
  soft-delete column. `Recorded` MUST NOT count a soft-deleted row: an admin
  purge in flight must not suppress lazy generation for an object whose purge is
  still cancellable. `Record`'s `ON CONFLICT DO NOTHING` keys on the composite
  primary key, which is unaffected.
- **FR-ADMIN-4** — `ReapableObjectKeys` needs no change: the failure ledger holds
  no `object_key`.
- **FR-ADMIN-5** — `TestManifestCoversEveryTable`
  (`internal/admin/arch_test.go:27`) MUST be widened to parse every non-test
  `.go` file under `internal/`, not only files named `entity.go`. The current
  filter is why this gap existed: `variantfailures` declares its `Entity` in
  `variantfailures.go`. Verified: widening surfaces exactly two additional
  tables in media-service — `media.processed_events`, already in
  `excludedTables`, and `media.media_variant_failures`, which FR-ADMIN-1
  addresses. No other fallout.
- **FR-ADMIN-6** — The widened walk MUST continue to fail loudly on a parse error
  and MUST retain the guard that fails when zero `TableName` declarations are
  found, so it cannot pass vacuously.

## 5. API Surface

None. No HTTP route, request shape, response shape, or error code changes. The
sweep is an internal background job; the admin purge protocol's internal routes
are unchanged in signature. `affected_counts` in admin purge responses gains a
`media_variant_failures` key, which is additive — the field is a map keyed by
manifest key and consumers already iterate it.

## 6. Data Model

### Changed: `media.media_variant_failures`

| Column | Type | Notes |
| --- | --- | --- |
| `deleted_at` | `timestamptz` NULL, indexed | New. Required by `admin.Stamp` / `admin.Count` (FR-ADMIN-2). |
| `purge_operation_id` | `uuid` NULL, indexed | New. Required by `admin.Restore` / `admin.Reap` (FR-ADMIN-2). |

The composite primary key `(media_object_id, variant)` is unchanged. Unlike
`media.media_variants`, no partial unique index is needed: the primary key is the
uniqueness guarantee and `Record` uses `ON CONFLICT DO NOTHING` rather than an
inferred arbiter, so a soft-deleted row occupying the slot cannot be silently
updated in place.

### Unchanged

`media.media_objects`, `media.media_variants`, and `media.processed_events` gain
no columns. No foreign keys or `ON DELETE CASCADE` constraints are added
(§2 non-goals).

### Migration notes

- Both columns are nullable with no default, so `AutoMigrate` adds them without a
  table rewrite. In normal operation this table is empty by design — its own
  package doc notes it is "cheap at rest" — so even a rewrite would be trivial.
- No backfill: NULL is the correct value for every existing row (live, not
  soft-deleted, not attached to a purge operation).
- Deployed databases hold orphaned `media.media_variants` and
  `media.media_variant_failures` rows from previous versions. These are drained by
  FR-RECON-1 rather than by a migration, so the rollout requires no manual step.

## 7. Service Impact

**`apps/media-service`** — the only service affected.

| Area | Change |
| --- | --- |
| `cmd/main.go` | `purgeExpired` removed; replaced by wiring plus the `jobs.Every` + leader-lock registration (FR-TEST-1). New composition-root adapters for the variant and failure-ledger ports (FR-TEST-2). |
| new `internal/…` purge package | Owns the per-object sweep and the reconciliation pass, driven through ports. Carries the tests in FR-TEST-4. |
| `internal/mediaobject/purge.go` | `DeleteRow` gains a transaction-capable form so FR-PURGE-4 can delete the media row inside the same transaction as the derived rows. `ListPurgeable` unchanged. |
| `internal/mediavariant` | New purge-side operations: list `(object_key)` for a media object including soft-deleted rows; delete all rows for a media object; list and delete orphaned rows for reconciliation. |
| `internal/variantfailures` | Two new entity columns (FR-ADMIN-2); `Recorded` narrowed to live rows (FR-ADMIN-3); new delete-for-media-object and orphan-reconciliation operations. |
| `internal/admin` | `media.media_variant_failures` added to `Manifest` (FR-ADMIN-1); `TestManifestCoversEveryTable` widened (FR-ADMIN-5). |
| `internal/storage` | No change. `RemoveObject` already has the needed shape; FR-TEST-3 introduces the interface on the consumer side. |

No frontend changes. No deploy-manifest changes. No new configuration is required
beyond the optional reconciliation cap (§8).

## 8. Non-Functional Requirements

### Performance

- The per-object pass adds one variant-key query and one transaction per
  purgeable object. Purge volume is bounded by user deletions over a five-day
  window; this is not a throughput path.
- Reconciliation adds two anti-join queries per tick. Both are keyed on
  `media_object_id`, which is indexed on `media.media_variants` and is the
  leading primary-key column on `media.media_variant_failures`. Once the backlog
  drains, both match zero rows.
- The FR-RECON-5 cap (default 500, `MEDIA_PURGE_RECONCILE_LIMIT`) bounds
  per-tick MinIO deletions. At an hourly cadence that drains 12,000 orphans per
  day, which comfortably exceeds any plausible accumulated backlog while keeping
  a single tick short.

### Correctness under concurrency

- Both passes run under the existing `media-purge` advisory lock, so replicas do
  not race each other (FR-RECON-8).
- Lazy card generation (`processing.CardGenerator`) runs in request context, not
  under that lock, so in principle it could insert a variant row for a media
  object the sweep is deleting. In practice card generation is reachable only via
  `Content`, which resolves a live media object first, and a purgeable object has
  been soft-deleted for five days. If it did happen, FR-RECON-1 collects the
  result on the next tick — the reconciliation pass is what makes this class of
  race self-healing rather than permanent.
- `mediavariant.Upsert`'s partial unique index means a soft-deleted variant is
  not a conflict target, so a regeneration inserts a fresh live row rather than
  resurrecting a purge-owned one. FR-PURGE-1's "including soft-deleted rows"
  ensures both rows and both keys are removed at purge time.

### Security and privacy

- This is a data-deletion completeness fix: it is what makes a user's deletion of
  a photo actually remove every rendition of that photo rather than leaving three
  derived copies in object storage indefinitely.
- Log fields MUST remain IDs and object keys — no filenames, no error strings
  that could carry user-supplied filenames — consistent with the reasoning in the
  `variantfailures` package doc.

### Observability

- Per-object failures log at WARN with `media_id` (FR-PURGE-7, FR-PURGE-9),
  matching the existing sweep's fields.
- Each tick that removes anything logs a summary at INFO: media objects purged,
  variant objects removed, orphans reconciled, and whether the cap was hit
  (FR-RECON-6).
- A quiet tick MUST NOT log. This job runs hourly forever and is expected to
  find nothing most of the time.

## 9. Open Questions

- **OQ-1 — Bytes with no surviving row.** Objects leaked by versions that already
  deleted the variant rows (there are none today — nothing deletes variant rows
  except `ReplaceForMediaObject` and the admin reaper, both of which handle their
  own bytes — but a partially-completed admin reap could produce one) are
  unreachable from the database and would need prefix enumeration of the bucket
  to find. Deliberately out of scope. Should a follow-up issue be filed for an
  operator-triggered bucket audit, or is this acceptable to leave?
- **OQ-2 — Manifest versus exclusion for the failure ledger.** §4.5 puts
  `media.media_variant_failures` in `admin.Manifest`, which costs two new
  columns. The alternative is `excludedTables` with a reason — its own package doc
  calls it "a ledger, not a domain aggregate — the same kind of thing as
  `processedevents`", and `media.processed_events` is excluded. Manifest was
  chosen for uniformity: exclusion would leave the ledger rows orphaned on the
  admin reap path, which is the same defect on a different route, and would then
  need targeted cleanup machinery of its own. Design phase may revisit.
- **OQ-3 — notification-service has the same arch-test blind spot.**
  `apps/notification-service/internal/inbox/processed.go` declares `TableName`
  outside `entity.go`, so notification-service's twin of
  `TestManifestCoversEveryTable` cannot see `inbox`'s table either. Out of scope
  here. File as a separate issue?
- **OQ-4 — Reconciliation cap default.** 500/tick is a judgement call. If a real
  deployment's `media.media_variants` orphan count is known to be far larger, the
  default may want raising, or the cap may want to apply only to byte removals
  rather than to row deletions.

## 10. Acceptance Criteria

Sweep completeness:

- [ ] Purging a media object with `thumbnail`, `card`, and `display` variants
      removes four MinIO keys (original + three variants) and leaves no
      `media.media_variants`, `media.media_variant_failures`, or
      `media.media_objects` row for that object.
- [ ] Variant keys come from the `object_key` column, not from recomputation.
- [ ] Soft-deleted variant rows for a purged object are removed along with their
      bytes.
- [ ] Row deletions for one media object happen in a single transaction.
- [ ] `TestListPurgeable_skipsAdminStampedObjects` passes unmodified.

Failure and retry semantics:

- [ ] A failing variant byte removal leaves every row for that media object in
      place, logs at WARN with `media_id`, and does not stop the sweep from
      processing the remaining media objects.
- [ ] A failing original byte removal behaves identically.
- [ ] Re-running the sweep after bytes are gone but rows survive completes the
      purge (removal of an absent key is a no-op).
- [ ] A failing row-deletion transaction logs at WARN and the sweep continues.

Reconciliation:

- [ ] An orphaned `media.media_variants` row has its bytes removed and its row
      deleted.
- [ ] An orphaned `media.media_variant_failures` row is deleted.
- [ ] A variant whose parent media object exists but is soft-deleted is left
      untouched.
- [ ] With more orphans than the cap, exactly the cap is processed, the remainder
      survives, and the truncation is logged at INFO.
- [ ] Reconciliation runs inside the `media-purge` leader lock.

Placement and tests:

- [ ] `purgeExpired` no longer exists in `cmd/main.go`; the logic lives in an
      `internal/` package with its own tests.
- [ ] `mediaobject` imports neither `mediavariant` nor `variantfailures`; the
      wiring is composition-root adapters.
- [ ] Every test in FR-TEST-4 exists, and each was demonstrated to fail against
      the pre-change behaviour (FR-TEST-5).

Admin purge coverage:

- [ ] `media.media_variant_failures` is in `admin.Manifest` with all three scopes
      resolved, ahead of `media.media_objects`.
- [ ] `variantfailures.Entity` has indexed nullable `deleted_at` and
      `purge_operation_id`; `Store.Recorded` ignores soft-deleted rows.
- [ ] A fleet-scoped admin purge stamps, restores, and reaps failure-ledger rows,
      with `affected_counts` reporting a `media_variant_failures` count.
- [ ] `TestManifestCoversEveryTable` parses every non-test `.go` file under
      `internal/`, still fails on a parse error, and still fails when it finds no
      `TableName` declarations.
- [ ] Adding a table in a file not named `entity.go` fails the arch test until it
      is placed in `Manifest` or `excludedTables`.

Project gates:

- [ ] `make ci` passes (`lint-check`, `vet`, `test`, `build`, `fe-test`,
      `fe-build`).
- [ ] Code review run per CLAUDE.md before the PR is opened
      (`plan-adherence-reviewer` + `backend-guidelines-reviewer`; no frontend
      files change).
- [ ] Both `kustomize build` renders still succeed — no manifest change is
      expected, so this is a no-regression check only.
- [ ] Issue #28 is referenced in the PR and closed by it.
