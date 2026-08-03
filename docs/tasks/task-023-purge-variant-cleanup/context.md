# Media Purge Variant Cleanup — Implementation Context

Task: task-023 · Tracks [#28](https://github.com/jtumidanski/MyFleet/issues/28)
Worktree: `.worktrees/task-023-purge-variant-cleanup`, branch `task-023-purge-variant-cleanup`
Artifacts: `prd.md` (approved v1) → `design.md` → `plan.md` (this phase) → execution

---

## 1. What this task actually changes

`purgeExpired` in `apps/media-service/cmd/main.go:176-193` removes a purged media
object's own MinIO object and its `media.media_objects` row, and nothing else.
Every derived artefact survives permanently: the `media.media_variants` rows, the
MinIO objects those rows name, and any `media.media_variant_failures` row. Once
the parent row is gone the variant rows are unreachable — `Content` resolves the
media object first — so they are dead weight that grows monotonically with every
purge.

Three things follow, and the plan does all three:

1. The sweep removes every byte and row derived from a purged media object, in
   the right order, with a partial failure never destroying the handle on what
   remains.
2. A permanent per-tick reconciliation pass drains the backlog already in
   deployed databases, without an operator-run migration.
3. `media.media_variant_failures` joins `admin.Manifest`, and the arch test that
   is supposed to catch exactly that omission is widened so no future table can
   hide from it by living outside `entity.go`.

## 2. Key files

| File | Why it matters |
| --- | --- |
| `apps/media-service/cmd/main.go:123-136` | The `jobs.Every` + `WithLeaderLock` registration the sweep keeps. Lines 176-193 are `purgeExpired`, which goes away. |
| `apps/media-service/cmd/main.go:195-230` | The `variantLookup` / `cardGenerator` composition-root adapters — the pattern FR-TEST-2 refers to, and the prose that Task 11's arch test turns into a check. |
| `apps/media-service/internal/mediaobject/purge.go` | `ListPurgeable` (with the `purge_operation_id IS NULL` narrowing) and `DeleteRow`. Only the `DeleteRow` doc comment changes. |
| `apps/media-service/internal/mediavariant/entity.go` | `object_key` is a stored column — authoritative for what was written. `ApplyPartialIndexes` is the precedent for creating schema-qualified indexes outside struct tags. |
| `apps/media-service/internal/variantfailures/variantfailures.go` | `Entity` (declared here, not in `entity.go` — the reason the arch test never saw it), `Store.Recorded`, `Store.Record`. |
| `apps/media-service/internal/admin/manifest.go` | `Manifest` (child-to-parent) and `excludedTables`. |
| `apps/media-service/internal/admin/operations.go:96-122` | `ReapSparing`'s per-table switch; `ReapableObjectKeys:131-148` carries the byte-before-row reasoning this task copies. |
| `apps/media-service/internal/admin/arch_test.go:39` | `filepath.Base(path) != "entity.go"` — the filter that created the blind spot. |
| `apps/media-service/internal/storage/minio.go:207` | `RemoveObject(ctx, key) error` — the one-method surface `ObjectRemover` mirrors. |
| `apps/fleet-service/internal/vehicle/purge.go`, `apps/fleet-service/internal/admin/orphans.go` | The sibling service's fix for the same class of defect. Useful as a reference; deliberately not copied wholesale (design D1/D2/D3). |

## 3. Decisions carried in from the design

- **D1 — `internal/purge` is a new job package** that imports `mediaobject`,
  `mediavariant` and `variantfailures` directly and reaches MinIO through a
  one-method `ObjectRemover` port. Recorded as deviation **D-1**: FR-TEST-2
  prescribes composition-root adapters for the database side, but adapters
  confined to `package main` would leave `internal/purge`'s tests unable to
  exercise the production SQL, hollowing out FR-TEST-4's stored-state
  assertions. The invariant FR-TEST-2 protects — the three domain packages stay
  independent of one another — is preserved and, for the first time, enforced by
  a test (Task 11).
- **D2 — one transaction per media object, not one per tick.** A tick-wide
  transaction would roll successful objects back with a failed one, which
  FR-PURGE-7/9 forbid. Byte removals stay outside and before it; MinIO is not
  transactional and holding a transaction open across N round-trips is the wrong
  trade. The resulting window (bytes gone, rows present) is what FR-PURGE-8 makes
  safe.
- **D3 — reconciliation runs every tick, permanently**, not once at boot like
  fleet-service's. Lazy card generation writes variant rows from request context,
  outside the `media-purge` lock, so a variant row can appear for a media object
  the sweep is deleting; a per-tick pass makes that race self-healing within an
  hour.
- **D4 — the ledger's orphan cleanup is capped by media object**, not by row, so
  an object's ledger is never half-reconciled and the first-deployment
  transaction stays bounded.
- **D5 — variant selection carries no `deleted_at` predicate at all.** A purge
  hard-deletes the parent, so a soft-deleted variant of it is not recoverable
  state; skipping it would leak its bytes.
- **D6 — orphan detection tests the parent row, never `deleted_at`.** A variant
  of a soft-deleted-but-recoverable media object is not an orphan.
- **D7 — the failure ledger joins `Manifest`** rather than `excludedTables`
  (OQ-2 resolved). Excluding it would leave ledger rows orphaned on the admin
  reap path — the same defect on a different route.

## 4. Deviations this plan introduces beyond the design

Three, all verified empirically during planning rather than reasoned about.

### P-1 — the new ledger columns carry no `gorm:"index"` tag

Design §6.2 says to mirror `mediavariant.Entity` exactly, index tags included.

**Verified false.** `AutoMigrate` on a schema-qualified table creates the table
correctly but then emits its index DDL **without** the schema qualifier:

```
CREATE INDEX `idx_media_media_variant_failures_purge_operation_id`
  ON `media_variant_failures`(`purge_operation_id`)
→ no such table: main.media_variant_failures
```

Adding the tags therefore breaks `variantfailures_test.go`, which is the one
harness in the service that calls a real `Migration`. The plan declares both
columns untagged and creates the indexes explicitly in `Migration` via a new
exported `ApplyIndexes`, with a SQLite branch — exactly the shape and exactly the
reason of `mediavariant.ApplyPartialIndexes`. Postgres gets the same schema
either way; only the mechanism differs.

### P-2 — the `internal/purge` harness hand-writes DDL for two of three tables

Design §8 specifies running the real `mediaobject.Migration`,
`mediavariant.Migration` and `variantfailures.Migration` against SQLite, and
argues against hand-written DDL on drift grounds.

**Verified false for the first two.** Both fail on SQLite for the same reason as
P-1 — both entities carry index tags:

```
mediaobject.Migration  → no such table: main.media_objects
mediavariant.Migration → no such table: main.media_variants
```

This is precisely why `mediavariant/provider_test.go:14-17`, `mediaobject/purge_test.go`
and `admin/admin_test.go` all hand-write their DDL; that comment has been in the
tree the whole time and the design missed it.

The design's drift concern is legitimate, so the plan answers it directly rather
than by ignoring it: `internal/purge` runs the real `variantfailures.Migration`
(the entity this task actually changes) and the real
`mediavariant.ApplyPartialIndexes`, hand-writes the other two tables, and adds
**`TestHarnessDDLMatchesTheEntities`** — a guard that parses each entity struct
through `schema.Parse` and compares the derived column set against
`pragma_table_info`. That fails loudly on drift, which is strictly better than
AutoMigrate would have been.

### P-3 — the ConfigMap does change

Design §9 says "No deploy-manifest change" because
`MEDIA_PURGE_RECONCILE_LIMIT` is optional and defaulted in code.

The repo's own precedent disagrees. `MEDIA_WORKERS`, `MEDIA_MAX_UPLOAD_BYTES`
and `MEDIA_LAZY_VARIANT_CONCURRENCY` all have in-code defaults and all appear in
`deploy/k8s/base/media-service/configmap.yaml`, because that is where an operator
looks for the knob — task-013's `context.md` §65 makes exactly this argument for
exactly this reason. The plan adds the key with a comment. Additive and
value-preserving; both overlays still render and the `main` overlay still has
zero PVCs, Secrets and ClusterRoles.

## 5. Facts verified during planning

Everything below was executed, not assumed.

- **media-service declares exactly four `TableName()` methods** —
  `media.media_objects` and `media.media_variants` (in `entity.go`, both in
  `Manifest`), `media.processed_events` (in `processedevents.go`, in
  `excludedTables`) and `media.media_variant_failures` (in `variantfailures.go`,
  in neither). Widening the arch-test walk surfaces exactly the two the design
  predicts and no others, so it goes green once the manifest entry lands.
- **`ListOrphaned`'s anti-join with `LIMIT` runs on SQLite**, returning the
  orphan and skipping rows whose parent exists.
- **`DeleteOrphaned`'s grouped `LIMIT` sub-query runs on SQLite**, deleting all
  of a capped media object's ledger rows and leaving the remainder.
- **`ObjectKeysForMediaObject` returns four keys** for a media object with three
  live variants plus one soft-deleted one — the partial unique index really does
  admit a live and a soft-deleted `card` row for the same object, so T1's fixture
  is realistic.
- **`db.Transaction` works on the harness under `SetMaxOpenConns(1)`** —
  `media_variants` → `media_variant_failures` → `media_objects` in one callback,
  with the admin-stamped object untouched — provided every statement inside uses
  `tx` and never the outer handle.
- **`schema.Parse(model, &sync.Map{}, db.NamingStrategy)` plus
  `SELECT name FROM media.pragma_table_info(?)`** produces matching, comparable
  column sets for both hand-written tables. This is the drift guard's mechanism.
- **`storage.Client.RemoveObject`** is `RemoveObject(ctx, key) error` and returns
  nil for an absent key (S3 `DELETE` semantics, passed through by minio-go).
  `admin/resource.go:114-118` already depends on this in production; FR-PURGE-8
  requires it asserted anyway, which T4 does.
- **Lint config** is `.golangci.yml` at the repo root: `standard` linter group
  (errcheck, govet, ineffassign, staticcheck, unused) plus gofumpt and goimports
  with `local-prefixes: github.com/jtumidanski/myfleet`.
- **`make ci`** is `lint-check vet test build fe-test fe-build manifests
  carfax-template`. `make test` runs `go test -race` across the whole workspace.

## 6. Task dependency order

Tasks 1–5 (the admin-purge half) and Tasks 6–11 (the sweep half) are
independent of each other, but within each half the order matters:

```
1 CollectTableNames ─┐
2 ledger columns ────┼─→ 3 manifest entry ─→ 4 widened arch test
                     └─────────────────────→ 5 ReapSparing

6 mediavariant queries ─┐
7 variantfailures queries ─┼─→ 8 move the sweep ─→ 9 per-object pass ─→ 10 reconciliation
                                                                    └─→ 11 arch test

                                                                       12 verify + review
```

Task 3 depends on Task 2 (the manifest entry fails at runtime without the
columns). Task 4 depends on Tasks 1 and 3 (it would otherwise leave the build
red). Task 8 must land before Task 9, because Task 8's verbatim port is the
pre-change baseline Task 9's tests go red against.

## 7. Testing discipline

FR-TEST-5 is not decorative: this defect survived every variant the service has
ever produced because the sweep lived in `package main` with no test file. Each
task in the plan names its red demonstration explicitly.

Four checks **cannot** go red against the pre-change code, because the old code
does not do the thing at all. For those the plan mutates the *new* code and
proves the test catches it — revert the fix, watch it go red, restore:

| Check | Mutation |
| --- | --- |
| Widened arch test (Task 4) | comment out the `media_variant_failures` manifest target |
| T3, failed-original abandons the object whole (Task 9) | change `break` to `continue` in the key loop |
| Reconciliation off switch (Task 10) | change `<= 0` to `< 0` |
| T9, sibling independence (Task 11) | add a blank import of `mediavariant` to `mediaobject/purge.go` |

None of those mutations may be committed.

This matters more than usual here because two assertions are trivially true for
the wrong reason in the old code — "the variant rows survived" is true when
nothing ever touches variant rows. T3 therefore asserts the stronger claim that
the variant keys were never *offered* to the remover, which is why
`recordingRemover` tracks `asked` separately from `removed`.

## 8. Out of scope, and what to say about it

Unchanged from PRD §2 and design §13: the five-day recovery window, the one-hour
cadence, the admin purge stamp/restore/reap lifecycle and HTTP surface, bucket
enumeration, and `ON DELETE CASCADE` foreign keys. No frontend change.

Two open questions resolve to follow-up issues rather than work in this task, and
the PR description should say so:

- **OQ-1 — bytes with no surviving row.** Objects leaked by a version that
  already deleted the variant rows are unreachable from the database and would
  need prefix enumeration of the bucket to find. Nothing in the service produces
  them today; the only candidate path is a partially-completed admin reap, and
  that path spares rather than strands. **File a follow-up issue for an
  operator-triggered bucket audit.**
- **OQ-3 — notification-service has the same arch-test blind spot.**
  `apps/notification-service/internal/inbox/processed.go` declares `TableName`
  outside `entity.go`, so that service's twin of `TestManifestCoversEveryTable`
  cannot see `inbox`'s table either. **File as a separate issue**, and reuse this
  task's `CollectTableNames` shape when it is picked up.

`affected_counts` in admin purge responses gains a `media_variant_failures` key.
Verified additive and needing no frontend change: the web type is
`Record<string, number>` (`apps/web/src/types/models/admin.ts:107,122`) and
`BlastRadiusPanel`'s `humanise` sentence-cases any key missing from `LABELS`
(`BlastRadiusPanel.tsx:64-76`), so it renders as "Media variant failures" in the
alphabetical tail, exactly where `media_variants` renders today.
