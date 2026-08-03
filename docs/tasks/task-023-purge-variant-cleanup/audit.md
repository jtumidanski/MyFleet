# Audit — task-023-purge-variant-cleanup

> This file is written by more than one reviewer agent. Each appends its own
> top-level section; do not overwrite another reviewer's section.

## Plan Adherence Review

**Plan Path:** `docs/tasks/task-023-purge-variant-cleanup/plan.md`
**Audit Date:** 2026-08-03
**Branch:** `task-023-purge-variant-cleanup`
**Base Branch:** `main` (merge-base `d3c9eaa`)
**Implementation range audited:** `ba33f33..125ca36` (12 commits) — diff scope
identical to `merge-base..HEAD`: 20 files, +1669 / -82, no stray files.

### Executive Summary

All 12 plan tasks are implemented. 11 of 12 are verbatim to the plan's
prescribed code; Task 10 carries one deliberate, human-approved deviation from
the plan's literal test bodies, which I judge faithful to — indeed required by —
the plan's own Global Constraint FR-TEST-5. Every requirement in the plan's
Self-Review table maps to landed code with file:line evidence. Working tree is
clean; the four touched packages plus `mediaobject` pass `go test -count=1`.
The only outstanding plan steps are Task 12 Steps 5–7 (this review, then
artifact commit and PR), which the plan sequences after the review by design.

**Recommendation: READY_TO_MERGE** (after Task 12 Steps 6–7 and the rebase noted
in the ledger).

### Task Completion

| # | Task | Status | Evidence |
|---|------|--------|----------|
| 1 | Extract arch-test walk as `CollectTableNames` | DONE | `apps/media-service/internal/admin/tablenames.go:35-84` (verbatim to plan, incl. `testdata` SkipDir at :42 and fatal parse error at :51-56); fixture `internal/admin/testdata/fixture/ledger.go:14`; tests `internal/admin/arch_test.go:60-91`. Commit `8a48887`. |
| 2 | Ledger purge columns + `Recorded` narrowing | DONE | `internal/variantfailures/variantfailures.go:48-53` (`DeletedAt`, `PurgeOperationID`, no `gorm:"index"` — P-1 honoured), `:64-71` `Migration` → `ApplyIndexes`, `:73-99` `ApplyIndexes` with the SQLite schema-qualified branch, `:132-138` `Recorded` gains `AND deleted_at IS NULL`. Test `variantfailures_test.go:105-129`. Migration is wired at startup: `cmd/main.go:42`. Commit `0bfdb19`. |
| 3 | `media.media_variant_failures` in `admin.Manifest` | DONE | `internal/admin/manifest.go:56-75` — target between `media_variants` and `media_objects`, all three scopes. Round-trip test `internal/admin/admin_test.go:316-372`; harness DDL `:53-56`, seed `:73-76`. Commit `fa3defe`. |
| 4 | Widen `TestManifestCoversEveryTable` | DONE | `internal/admin/arch_test.go:12-27` now calls `CollectTableNames("..")`; the inline walk and its six imports are deleted (`arch_test.go:3-6` reduced to `strings`, `testing`); vacuity guard retained at `:24-26`. Commit `b94d272`. |
| 5 | Spare a spared object's ledger rows | DONE | `internal/admin/operations.go:109-115` adds the `media.media_variant_failures` case; doc comment updated at `:102-104`. Test `internal/admin/admin_test.go:374-390`. Commit `9d1f0da`. |
| 6 | Purge-side variant queries | DONE | `internal/mediavariant/purge.go` — `Orphan` :6-9, `ObjectKeysForMediaObject` :28-36 (no `deleted_at` predicate; reads `object_key`, FR-PURGE-1/2), `DeleteForMediaObject` :42-44, `ListOrphaned` :56-65 (`NOT EXISTS` anti-join, FR-RECON-2), `DeleteByID` :73. Five tests in `purge_test.go`. Commit `6009715`. |
| 7 | Purge-side ledger queries | DONE | `internal/variantfailures/purge.go:8-11` `DeleteForMediaObject`; `:25-38` `DeleteOrphaned` with the grouped, unaliased-DELETE form and `GROUP BY … LIMIT ?` (A-2 cap by media object). Three tests in `variantfailures/purge_test.go`. Commit `6086895`. |
| 8 | Move the sweep into `internal/purge` | DONE | `internal/purge/sweeper.go:19-58` (package doc, `ObjectRemover` port :36-38, `Config` :41-45, `Sweeper`/`NewSweeper` :48-58); harness `testdb_test.go:33-215` incl. the schema-drift guard `TestHarnessDDLMatchesTheEntities` :84-122 (P-2's answer) and the `recordingRemover` asked/removed split :131-155; T5 `sweeper_test.go:28-51`. Wiring `cmd/main.go:128-140`; `purgeExpired` deleted from `cmd/main.go` and `gorm.io/gorm` import dropped (diff `d5dc413`). |
| 9 | Per-object purge completeness | DONE | `internal/purge/sweeper.go:169-238` — `ObjectKeysForMediaObject` first :178, original-then-variants byte loop with `break` on failure :193-202, `if failed { continue }` :203-209 (FR-PURGE-7), one transaction per object :215-223 using `tx` throughout (FR-PURGE-4), transaction failure logs and continues :223-230 (FR-PURGE-9), summary counted only on completion :234-235. `summary` :62-90 with the quiet-tick guard :80-82. D-2 doc line `internal/mediaobject/purge.go:41-46`. T1–T4 `sweeper_test.go:56-199`. Commit `54716c4`. |
| 10 | Reconciliation pass | DONE (deliberate deviation — see below) | `internal/purge/sweeper.go:106-115` `RunOnce` runs both passes and prefers the per-object error; `:129-167` `reconcile` with the `<= 0` off switch :130-132, per-row sparing :141-156, `reconcileCapped` :157, ledger pass :161-165. Config read `cmd/main.go:132-134` (`config.GetInt("MEDIA_PURGE_RECONCILE_LIMIT", 500)`); ConfigMap `deploy/k8s/base/media-service/configmap.yaml:21-28`. Tests `internal/purge/reconcile_test.go`. Commit `d624f8d` + `400d291`. |
| 11 | Sibling-package independence check | DONE | `internal/purge/arch_test.go:22-62` — parses non-test files of the three siblings with `parser.ImportsOnly`, fails on a cross-sibling import, plus the `parsed == 0` vacuity guard :59-61. Commit `125ca36`. |
| 12 | Full verification and code review | DONE through Step 4; Steps 5–7 by design pending | Steps 1–4 evidence in `.superpowers/sdd/plan/task-12-report.md` and `progress.md:38`: full `make ci` green (8 targets), both overlays render, both passed `kubectl apply --dry-run=server` against `bee`, `main` overlay 0 PVC / 0 Secret / 0 ClusterRole and carries `MEDIA_PURGE_RECONCILE_LIMIT`. Step 5 is this review; Steps 6–7 follow it. |

**Completion Rate:** 12/12 tasks (100%)
**Skipped without approval:** 0
**Partial implementations:** 0

### The Task 10 Deviation — Judged Faithful

The plan (plan.md:2202-2221) prescribes `TestReconcile_isDisabledByANonPositiveLimit`
exercising only `ReconcileLimit: 0`, and (plan.md:2131-2157)
`TestReconcile_processesAtMostTheCapPerTick` driving `RunOnce` with no
assertion on `reconcileCapped`. The implementation departs on both points:

- The off-switch test is parameterised over `{0, -1}`
  (`reconcile_test.go:170-193`), with the reasoning recorded in the test's own
  doc comment at `:156-169`.
- The cap test calls `reconcile` directly so `summary` is observable, and
  asserts `reconcileCapped` true on two capped ticks and **false** on an
  uncapped one (`reconcile_test.go:63-113`).

I independently confirm the premise. With the `s.cfg.ReconcileLimit <= 0` guard
at `sweeper.go:130` deleted, a limit of `0` flows into
`mediavariant.ListOrphaned(db, 0)` → `… LIMIT 0`, which returns zero rows on
both SQLite and Postgres, and into `variantfailures.DeleteOrphaned(db, 0)`,
whose sub-query `GROUP BY media_object_id LIMIT 0` selects nothing. Every
assertion in the plan's literal test would still pass. The plan's own Global
Constraint (plan.md:20) states: *"Every new test must be demonstrated red before
the change that makes it green (FR-TEST-5). … A test that cannot be made to go
red is not evidence."* A test that survives deletion of the code it claims to
cover is precisely that. `-1` is the falsifiable case and is reachable in
production — `config.GetInt` does a bare `strconv.Atoi` with no clamping, so
`MEDIA_PURGE_RECONCILE_LIMIT="-1"` reaches `Config.ReconcileLimit` unmodified,
where SQLite's `LIMIT -1` is unbounded and Postgres rejects it.

Likewise, the plan's cap test never observed `reconcileCapped`, so
`sweeper.go:157` could have been hardcoded `false` (or used `>=`) with the whole
package still green. The replacement covers both polarities.

**Verdict:** the deviation subordinates the plan's prescribed test *code* to the
plan's prescribed test *standard*, and strengthens coverage of `A-3` and
`FR-RECON-6` rather than weakening it. Faithful to intent. The deviation is
recorded in the code, in `.superpowers/sdd/plan/task-10-report.md`, and in
`progress.md:29-31`.

Two smaller, defensible departures from the plan's literal text, both
inconsequential:

- `reconcile` is placed above `purgeExpired` in `sweeper.go` rather than below.
  No behavioural difference.
- The cap test no longer exercises `RunOnce`. Its fixture has no parent media
  objects at all, so `purgeExpired` was a no-op in that test either way; the
  per-object pass remains covered by T1–T5 (`sweeper_test.go`).

### Not Gaps

- **Task 12 Steps 5–7.** Step 5 is this review; Step 6 (commit artifacts) and
  Step 7 (open the PR referencing #28) are sequenced after it by the plan.
- **Plan checkboxes.** All 75 `- [ ]` are still unticked; ticking them is Task 12
  Step 6, i.e. after this review. It must still be done before merge.
- **P-1 / P-2 / P-3.** All three planning-time deviations are implemented as
  declared in `context.md` §4 and are visible in the code cited above
  (untagged columns + explicit `ApplyIndexes`; hand-written DDL guarded by
  `TestHarnessDDLMatchesTheEntities`; the ConfigMap key).
- **FR-ADMIN-4.** `admin.ReapableObjectKeys` (`internal/admin/operations.go:143`)
  is untouched by the diff, which is correct — the ledger holds no `object_key`.
- **FR-PURGE-5.** `internal/mediaobject/purge.go` changed by exactly five lines,
  all doc comment on `DeleteRow` (:41-46). `ListPurgeable` and
  `TestListPurgeable_skipsAdminStampedObjects` are untouched.

### Deferred Minors Carried by the Ledger

`.superpowers/sdd/plan/progress.md` records 12 deferred minors. I reviewed each
against the plan; none corresponds to a plan step that was skipped — all are
observations beyond the plan's scope. Worth surfacing for the PR description
rather than blocking:

1. `progress.md:25` — T4's red demonstration failed at its setup guard
   (`got 0 rows`) rather than on its retry-specific assertions, so the
   retry-completion assertions themselves were never shown red. The test is
   still meaningful (it passes only when the second tick clears all three
   tables), but it is the weakest red demo on the branch.
2. `progress.md:32` — `reconcileCapped` (`sweeper.go:157`) is derived from the
   variant query only; a truncated *ledger* cleanup logs
   `reconcile_capped=false`. Observability gap, not a correctness one.
3. `progress.md:34` — `ListOrphaned` has no `ORDER BY`, so a durably-failing
   orphan could starve the backlog behind it.
4. `progress.md:24` — the transaction-failure branch (`sweeper.go:223-230`) has
   no test.
5. `progress.md:39` — the branch is behind `main`; rebase before merge.

### Build & Test Results

Focused re-run of the affected packages (full `make ci` was already run and
green per `task-12-report.md`; not repeated):

| Package | Tests |
|---------|-------|
| `internal/purge` | PASS |
| `internal/admin` | PASS |
| `internal/variantfailures` | PASS |
| `internal/mediavariant` | PASS |
| `internal/mediaobject` | PASS |

Working tree clean at `125ca36`; no uncommitted mutation left over from any of
the red demonstrations.

### Overall Assessment

- **Plan Adherence:** FULL
- **Recommendation:** READY_TO_MERGE

### Action Items

1. Task 12 Step 6 — tick the 75 plan checkboxes and commit the task artifacts.
2. Rebase onto `main` (branch is behind; `progress.md:39`).
3. Task 12 Step 7 — open the PR, referencing and closing issue #28, and carry
   the deferred-minor list and the OQ-1 / OQ-3 follow-ups from `context.md` into
   the PR body.

---

# Backend Guidelines Audit — media-service (task-023)

- **Service Path:** `apps/media-service`
- **Scope:** changed Go packages on `task-023-purge-variant-cleanup`, range `ba33f33..125ca36`
- **Guidelines Source:** `.claude/skills/backend-dev-guidelines/resources/` (all 8 resource docs read in full)
- **Date:** 2026-08-03
- **Build:** PASS
- **Tests:** 9 packages passed, 0 failed
- **Overall:** NEEDS-WORK

---

## Build & Test Results

```
$ cd apps/media-service && go build ./...
(no output, exit 0)

$ cd apps/media-service && go test ./... -count=1
ok  	.../apps/media-service/cmd	0.014s
ok  	.../apps/media-service/internal/admin	0.023s
ok  	.../apps/media-service/internal/mediaobject	0.051s
ok  	.../apps/media-service/internal/mediavariant	0.022s
ok  	.../apps/media-service/internal/processedevents	0.005s
ok  	.../apps/media-service/internal/processing	1.132s
ok  	.../apps/media-service/internal/purge	0.021s
ok  	.../apps/media-service/internal/storage	0.018s
ok  	.../apps/media-service/internal/variantfailures	0.008s
```

Objective gate passes. Proceeding to Phase 2+.

---

## Phase 2: Package Classification

| Package | `model.go` | `resource.go` | Classification | In scope |
|---|---|---|---|---|
| `internal/mediaobject` | yes (`model.go:1`) | yes (`resource.go:1`) | **Domain** → DOM | yes (doc-comment change only) |
| `internal/mediavariant` | yes (`model.go:15`) | absent | **Domain** → DOM | yes (new `purge.go`) |
| `internal/variantfailures` | absent | absent | **Support (ledger)** — self-declared at `variantfailures.go:6-8` | yes (new `purge.go`, entity columns) |
| `internal/admin` | absent | yes (`resource.go:47`) | **Sub-domain** → SUB | yes (`tablenames.go`, manifest target, `ReapSparing` case) |
| `internal/purge` | absent | absent | **Support (job)** — no REST surface, driven by `jobs.Every` | yes (new package) |
| `internal/processedevents`, `internal/processing`, `internal/storage` | — | — | Support, unchanged | no |

**Note on checklist applicability.** This service does not use the JSON:API/api2go transport the guidelines describe. `packages/shared-go/server` exposes no `RegisterHandler`, no `MarshalResponse`, and its `RegisterInputHandler[T]` (`packages/shared-go/server/handler.go:47`) has a different signature; `Transform` returns `server.Resource` with no error (`mediaobject/rest.go:18`). Checks that presuppose the absent API are marked **N/A** with the evidence of absence, not silently passed.

---

## Domain Checklist Results

### `internal/mediavariant` (domain — the package this branch adds `purge.go` to)

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| DOM-01 | `builder.go` exists | PASS | `internal/mediavariant/builder.go:15` `NewBuilder()`; fluent setters `:19-24`; `Build()` validates invariants `:27-32` |
| DOM-02 | `ToEntity()` method | PASS | `internal/mediavariant/entity.go:86` `func (m Model) ToEntity() Entity` |
| DOM-03 | `Make(Entity)` function | WARN | `internal/mediavariant/entity.go:72` — exists, but signature is `Make(e Entity) Model`, not `(Model, error)` as `file-responsibilities.md:17` requires. Pre-existing; unchanged by this branch |
| DOM-04 | `Transform` in `rest.go` | N/A | No `rest.go` in package — `ls internal/mediavariant/` returns no `rest.go`. Package has no REST surface; variants are served through `mediaobject` |
| DOM-05 | `TransformSlice` | N/A | Same as DOM-04 |
| DOM-06 | Processor accepts `FieldLogger` | N/A | No `processor.go` in package. `Provider`/`Administrator` are injected into `processing` and `mediaobject` processors from the composition root (`cmd/main.go:84-87`) |
| DOM-07 | Handlers pass `d.Logger()` | N/A | No `resource.go` in package |
| DOM-08 | POST/PATCH `RegisterInputHandler` | N/A | No `resource.go` in package |
| DOM-09 | Transform errors handled | N/A | No `Transform` in package |
| DOM-10 | Providers use lazy evaluation | WARN | `internal/mediavariant/provider.go:37` and `:51` wrap in `database.SliceQuery`/`database.Query` but immediately invoke the returned closure (`}) ()` at `:47`, `:63`) — the lazy wrapper is defeated. The five NEW purge queries (`purge.go:27`, `:41`, `:55`, `:72`; `variantfailures/purge.go:26`) are plainly eager and return concrete values. Pre-existing house pattern, extended by this branch |
| DOM-11 | No `os.Getenv()` in handlers | PASS | `grep -rn "os.Getenv" internal/ cmd/` → zero matches service-wide. The new knob is read once at startup: `cmd/main.go:134` `config.GetInt("MEDIA_PURGE_RECONCILE_LIMIT", 500)` and injected via `purge.Config` |
| DOM-12 | No cross-domain logic in handlers | N/A | No `resource.go`. Cross-domain orchestration for the sweep lives in `internal/purge/sweeper.go:172-238`, not in a handler |
| DOM-13 | Handlers don't call providers | N/A | No `resource.go` |
| DOM-14 | No direct entity creation in handlers | N/A | No `resource.go` |
| DOM-15 | `administrator.go` holds write operations | **FAIL** | `administrator.go` exists (`internal/mediavariant/administrator.go:31` `NewAdministrator`), but this branch adds two **state-changing** functions outside it: `internal/mediavariant/purge.go:41` `DeleteForMediaObject` and `internal/mediavariant/purge.go:72` `DeleteByID`, both raw `db.Exec("DELETE …")`. `file-responsibilities.md:45` — "administrator.go … handles **all** state-changing database access." Same defect in `internal/variantfailures/purge.go:9` `DeleteForMediaObject` and `:26` `DeleteOrphaned`. Reads are likewise outside `provider.go` (`mediavariant/purge.go:27` `ObjectKeysForMediaObject`, `:55` `ListOrphaned`), against `file-responsibilities.md:65` |
| DOM-16 | Domain error → HTTP status mapping | N/A | No `resource.go`. `Build()` returns `server.ErrValidation` (`builder.go:29`), which the shared error mapper resolves |
| DOM-17 | JSON:API interface on REST models | N/A | No `rest.go` |
| DOM-18 | Request models flat | N/A | No `rest.go` |
| DOM-19 | Table-driven tests | WARN | `internal/mediavariant/purge_test.go` has 5 tests, none using the `tests := []struct{…}` + `t.Run` form (`:47`, `:74`, `:99`, `:122`, `:139`). Each is a single-scenario function. `testing-guide.md:39` — "Prefer table-driven tests" |

### `internal/mediaobject` (domain — doc-comment change only)

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| DOM-01 | `builder.go` exists | PASS | `internal/mediaobject/builder.go:16` `NewBuilder()` |
| DOM-02 | `ToEntity()` method | PASS | `internal/mediaobject/entity.go:55` |
| DOM-03 | `Make(Entity)` function | WARN | `internal/mediaobject/entity.go:36` — returns `Model`, not `(Model, error)`. Pre-existing, unchanged |
| DOM-04 | `Transform` in `rest.go` | PASS (variant form) | `internal/mediaobject/rest.go:18` `func Transform(m Model) server.Resource` — no error return, so `patterns-rest-jsonapi`'s `(RestModel, error)` shape does not apply |
| DOM-05 | `TransformSlice` | PASS | `internal/mediaobject/rest.go:36`; used by the list path at `resource.go:282` via `TransformInternalMedia`, no inline loop in a handler |
| DOM-06 | Processor accepts `FieldLogger` | PASS | `internal/mediaobject/processor.go:202` `func NewProcessor(log logrus.FieldLogger, …)` |
| DOM-07 | Handlers pass `d.Logger()` | N/A → PASS-equivalent | No `HandlerDependency` in this stack. `grep -rn "logrus.StandardLogger" internal/ cmd/` → zero matches. Processor is constructed once with the injected `log` at `internal/mediaobject/resource.go:46` |
| DOM-08 | POST/PATCH `RegisterInputHandler` | WARN | `resource.go:49` POST `/media` uses `server.RegisterInputHandler`. `resource.go:119` POST `/media/{id}/confirm` and `resource.go:75` PUT `/media/{id}/content` use raw `http.HandlerFunc` — defensible (confirm has no body; content streams raw bytes), but neither is the prescribed form. Pre-existing, unchanged |
| DOM-09 | Transform errors handled | N/A | `Transform` returns no error (`rest.go:18`); `grep` finds no `_, _ :=` or `_ =` on a `Transform(` call in `resource.go` |
| DOM-10 | Providers lazy | WARN | Same eager-invocation pattern as mediavariant; `internal/mediaobject/provider.go` unchanged by this branch |
| DOM-11 | No `os.Getenv()` in handlers | PASS | Zero matches service-wide |
| DOM-12 | No cross-domain logic in handlers | PASS | `resource.go:63`, `:105`, `:126`, `:138`, `:164`, `:225` — every handler calls `proc.<Method>`; cross-domain access goes through the `VariantLookup`/`CardGenerator` ports satisfied in `cmd/main.go:192` and `:210` |
| DOM-13 | Handlers don't call providers directly | **FAIL** (pre-existing) | `internal/mediaobject/resource.go:257` constructs `prov := NewProvider(db)` and `:276` calls `prov.ListActiveByFleetAndIDs(fleetID, ids)` straight from the internal-route handler, bypassing the processor. `anti-patterns.md:13` and `:210`. Unchanged by this branch |
| DOM-14 | No direct entity creation in handlers | PASS | `grep "db\.Create\|db\.Save\|db\.Delete" internal/mediaobject/resource.go` → zero matches |
| DOM-15 | `administrator.go` for writes | PASS | `internal/mediaobject/administrator.go:26`, `:34`, `:46` — writes go through `ToEntity()` + administrator |
| DOM-16 | Error → HTTP status mapping | PASS | `server.WriteError` + `server.StatusFor` at `resource.go:170`; sentinels `server.ErrForbidden` (`:25`), `server.ErrValidation` (`:262`), `server.ErrRequestEntityTooLarge` (`:37`) |
| DOM-17 | JSON:API interface on REST models | N/A | House transport uses `server.Resource{Type, ID, Attributes}` (`rest.go:19-21`) rather than `GetName()/GetID()/SetID()`; that interface does not exist in `packages/shared-go/server` |
| DOM-18 | Request models flat | PASS | `resource.go:49-52` — inline anonymous struct with flat `contentType`/`originalFilename`; no `Data/Type/Attributes` nesting |
| DOM-19 | Table-driven tests | PASS | `internal/mediaobject/resource_test.go` and `processor_test.go` (1208 lines) use the pattern; unchanged by this branch |

---

## Sub-Domain Checklist Results

### `internal/admin` (sub-domain — `resource.go`, no `model.go`)

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| SUB-01 | Processor or parent processor holds business logic | **FAIL** (pre-existing) | No `processor.go` in `internal/admin/`. `operations.go` is a bare function library, and the reap handler carries real orchestration inline: `resource.go:105` key listing, `:112-124` per-object byte-removal loop with a failure set, `:132-135` spare-set construction, `:138-142` the reap transaction. `anti-patterns.md:150` — "Must have a `processor.go` (or use the parent domain's processor) for business logic". `resource.go` is unchanged by this branch, but `ReapSparing`'s new `media.media_variant_failures` case (`operations.go:110-115`) is consumed by exactly that handler |
| SUB-02 | Administrator for writes; no `db.Create`/`db.Save` in `resource.go` | PARTIAL | No `db.Create`/`db.Save`/`db.Delete` in `resource.go` (grep → zero). But `resource.go:51` issues `db.Raw("SELECT count(*) FROM media.media_objects …")` directly from a handler — `anti-patterns.md:211` "`resource.go` → `entity.go`/database (WRONG)". There is no `administrator.go`; `operations.go:96` `ReapSparing` fills that role. Pre-existing |
| SUB-03 | `RegisterInputHandler[T]` for POST | **FAIL** (pre-existing) | `internal/admin/resource.go:60` `r.Post("/internal/admin/purge", func(w http.ResponseWriter, req *http.Request){…})` and `:99` `r.Post("/internal/admin/reap/{opId}", …)` — raw handlers, not `server.RegisterInputHandler`. `anti-patterns.md:21`, `:153`. Unchanged by this branch |
| SUB-04 | No manual JSON parsing | **FAIL** (pre-existing) | `internal/admin/resource.go:62` `json.NewDecoder(req.Body).Decode(&body)`. Direct hit on `ai-guidance.md:249` / `anti-patterns.md:98-101`. Unchanged by this branch |

Changed-in-this-branch admin surface is clean on its own terms:
- `manifest.go:61` adds the `media_variant_failures` target child-to-parent, ahead of `media_objects`, matching the documented convention at `manifest.go:57-60`.
- `operations.go:110-115` adds the `ReapSparing` case; the `strings.HasSuffix(q, "NOT IN ?")` arg-append at `:117` still binds exactly one arg per target.
- `tablenames.go:35` `CollectTableNames` is real production code with both exclusions enforced (`:42` testdata skip, `:48` `_test.go` skip) and a hard error on parse failure (`:51-56`) rather than a silent skip.
- Vacuity guards are present on every new arch assertion: `arch_test.go:29-31`, `:84-86`.

### `internal/variantfailures` (support/ledger)

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| SUB-01 | Business logic out of handlers | PASS | No `resource.go`; `Store` at `variantfailures.go:106-112` holds the read/write surface |
| SUB-02 | Writes segregated | **FAIL** | New write functions live in `purge.go`, not an `administrator.go`: `internal/variantfailures/purge.go:9` `DeleteForMediaObject`, `:26` `DeleteOrphaned`. Same DOM-15 finding, restated for this package |
| SUB-03 | POST typed input handler | N/A | No HTTP surface |
| SUB-04 | No manual JSON parsing | PASS | `grep "json.NewDecoder\|json.Unmarshal\|io.ReadAll" internal/variantfailures/` → zero matches |

Entity change verified against its stated contract: `variantfailures.go:56-57` adds `DeletedAt *time.Time` and `PurgeOperationID *string`, which are exactly the columns `admin.Count` (`operations.go:20`), `admin.Stamp` (`:60`), `admin.Restore` (`:72`) and `admin.ReapSparing` (`:99`) reference by name — without them the manifest entry fails at runtime, as the comment at `variantfailures.go:34-38` claims. `Recorded`'s narrowing to `deleted_at IS NULL` (`variantfailures.go:133`) is covered by a falsifiable test at `variantfailures_test.go:105-129`.

### `internal/purge` (support/job — new package)

No DOM/SUB checklist applies (no `model.go`, no `resource.go`). Checked against `anti-patterns.md` and `testing-guide.md`:

| Check | Status | Evidence |
|---|---|---|
| Constructor takes `logrus.FieldLogger` not `*logrus.Logger` | PASS | `internal/purge/sweeper.go:56` `func NewSweeper(log logrus.FieldLogger, …)`; field typed at `:49` |
| No `logrus.StandardLogger()` | PASS | Zero matches service-wide |
| No `os.Getenv` | PASS | Zero matches; knob injected via `Config` (`sweeper.go:41-45`) from `cmd/main.go:134` |
| Ports for untestable dependencies | PASS | `sweeper.go:36` `ObjectRemover` is a one-method port; `cmd/main.go:133` satisfies it with the concrete `*storage.Client` |
| **`db.WithContext(ctx)` on data access** | **FAIL** | `testing-guide.md:213` — "Verify providers use `db.WithContext(ctx)` not bare `db`"; `file-responsibilities.md:36` — "Always use `p.db.WithContext(p.ctx)` when calling providers or administrators". `RunOnce(ctx)` (`sweeper.go:106`) has the context in scope and forwards it **only** to `s.store.RemoveObject` (`:142`, `:194`). Every database call takes the bare handle: `sweeper.go:134` `mediavariant.ListOrphaned(s.db, …)`, `:150` `mediavariant.DeleteByID(s.db, …)`, `:161` `variantfailures.DeleteOrphaned(s.db, …)`, `:173` `mediaobject.ListPurgeable(s.db)`, `:178` `mediavariant.ObjectKeysForMediaObject(s.db, …)`, `:215` `s.db.Transaction(…)`. A cancelled tick cancels the MinIO calls and nothing else |
| Table-driven tests | WARN | Of the 10 new tests across `sweeper_test.go` / `reconcile_test.go`, one uses `t.Run` over a case slice (`reconcile_test.go:171-192`). Two use anonymous case slices without `t.Run` (`sweeper_test.go:80-91`, `:187-198`); the remaining seven are single-scenario functions (`sweeper_test.go:28`, `:56`, `:97`, `:139`, `:167`; `reconcile_test.go:13`, `:63`, `:117`, `:138`) |
| Tests assert stored state, not return values | PASS | `countRows` assertions against the three tables throughout, e.g. `sweeper_test.go:84-86`, `reconcile_test.go:39`, `:148-152` |
| Anti-drift guard on hand-written DDL | PASS | `testdb_test.go:84` `TestHarnessDDLMatchesTheEntities` parses all three entities via `schema.Parse` and diffs against `pragma_table_info`, with a vacuity guard at `:110-112`. It compares column **names** only — not types or nullability — which is narrower than the comment at `:79-83` implies, but sufficient for the drift class the plan cites (P-2) |
| Sibling-independence invariant enforced | PASS | `internal/purge/arch_test.go:22` `TestDomainPackagesDoNotImportEachOther` parses every non-test file in the three sibling packages (`:33-43`) and errors on a cross-sibling import (`:53`), with a vacuity guard at `:59-61`. This substantiates deviation D-1 (`context.md` §3): the invariant FR-TEST-2 protects is now checked, and `internal/purge` is explicitly exempt as a job package with no consumers |

---

## Security Review (SEC-*)

media-service is not an auth service, but it validates JWTs and exposes unauthenticated internal routes, so the checks were run.

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| SEC-01 | JWT validation uses verified parsing | PASS | `grep -rn "ParseUnverified" apps/media-service` → zero matches. Validation is `authmw.JWT(keyfn)` (`cmd/main.go:166`) with a JWKS-backed keyfunc built at `:76` / `:230` |
| SEC-02 | Revocation/claims from validated tokens only | N/A | Service issues and revokes no tokens |
| SEC-03 | No open redirect | PASS | No redirect handler exists; `grep` finds no `http.Redirect` in `internal/` |
| SEC-04 | No hardcoded secrets | PASS | All credentials come from config at `cmd/main.go:51-55` (`config.MustGet` for `MINIO_ACCESS_KEY`, `MINIO_SECRET_KEY`) |
| SEC-extra | Unauthenticated internal routes still gated | PASS (documented, not verified here) | `internal/admin/resource.go:41-46` and `internal/mediaobject/resource.go:252-255` both name the priority-200 `internal-deny` rule in `deploy/k8s/overlays/main/ingressroute.yaml` as the control. Manifest verification is outside this audit's scope |
| SEC-extra | Purge sweep cannot touch admin-owned rows | PASS | `internal/mediaobject/purge.go:30` `purge_operation_id IS NULL` narrowing, with a falsifiable test at `internal/purge/sweeper_test.go:28-51` asserting both the row survives **and** its bytes were never offered to the remover (`:39`) |

---

## Additional Observations (not guideline checks)

1. **`reconcileCapped` only reflects the variant half of reconciliation.** `sweeper.go:157` sets it from `len(orphans) == s.cfg.ReconcileLimit`, before the ledger pass at `:161`. If the ledger cap is hit but the variant cap is not, the summary logs `reconcile_capped=false` for a tick that was truncated — the exact "reads as finished when it is not" failure the field's own comment at `sweeper.go:67-71` says it exists to prevent. `variantfailures.DeleteOrphaned` returns row count (`purge.go:37`), not whether the media-object cap bound, so detecting it would need a signature change.
2. **`internal/purge` has no `arch_test` coverage of its own import set.** `arch_test.go:22` proves the three siblings do not import each other, which is the invariant D-1 trades against — but nothing prevents a fourth package from taking the same exemption `internal/purge` claims for itself.

---

## Summary

### Blocking (must fix)

- **DOM-15 / SUB-02** — state-changing DB access outside `administrator.go`: `internal/mediavariant/purge.go:41` (`DeleteForMediaObject`), `internal/mediavariant/purge.go:72` (`DeleteByID`), `internal/variantfailures/purge.go:9` (`DeleteForMediaObject`), `internal/variantfailures/purge.go:26` (`DeleteOrphaned`). Reads likewise outside `provider.go`: `internal/mediavariant/purge.go:27`, `:55`. Contradicts `file-responsibilities.md:45` and `:65`. Introduced by this branch (follows the pre-existing `internal/mediaobject/purge.go` precedent, which has the same shape).
- **`db.WithContext(ctx)` never applied in the sweep** — `internal/purge/sweeper.go:134`, `:150`, `:161`, `:173`, `:178`, `:215` all take the bare `s.db` while `ctx` is in scope from `RunOnce` (`sweeper.go:106`). Contradicts `testing-guide.md:213` and `file-responsibilities.md:36`. Introduced by this branch (the deleted `purgeExpired` had the same defect, but the new code has the context threaded to the object-store side and could thread it here).

### Blocking but pre-existing (unchanged by this branch — flagged, not attributed)

- **SUB-04** — `internal/admin/resource.go:62` manual `json.NewDecoder(req.Body).Decode(&body)`.
- **SUB-03** — `internal/admin/resource.go:60`, `:99` POST routes on raw `http.HandlerFunc` instead of `server.RegisterInputHandler`.
- **SUB-01** — no `internal/admin/processor.go`; reap orchestration inline at `internal/admin/resource.go:105-142`.
- **DOM-13** — `internal/mediaobject/resource.go:257`, `:276` handler calls a provider directly.
- **SUB-02 (partial)** — `internal/admin/resource.go:51` raw `db.Raw` query in a handler.

### Non-Blocking (should fix)

- **DOM-19** — new tests are single-scenario functions rather than table-driven: `internal/mediavariant/purge_test.go:47`, `:74`, `:99`, `:122`, `:139`; `internal/purge/sweeper_test.go:28`, `:56`, `:97`, `:139`, `:167`; `internal/purge/reconcile_test.go:13`, `:63`, `:117`, `:138`. Only `reconcile_test.go:171` uses `t.Run` over a case slice. (`testing-guide.md:39`)
- **DOM-03** — `Make` returns `Model`, not `(Model, error)`: `internal/mediavariant/entity.go:72`, `internal/mediaobject/entity.go:36`. Service-wide, pre-existing.
- **DOM-10** — eager query execution: `internal/mediavariant/purge.go:27`, `:41`, `:55`, `:72`; `internal/variantfailures/purge.go:9`, `:26`. The existing `provider.go` wrappers are also eager in effect (`internal/mediavariant/provider.go:47`, `:63` immediately invoke the returned provider).
- **`reconcileCapped` under-reports** — `internal/purge/sweeper.go:157` vs. the ledger pass at `:161`.

### Verified Correct (adversarially, with evidence)

- Sweep stays off admin-stamped rows: `internal/mediaobject/purge.go:30`, proven by `internal/purge/sweeper_test.go:39-47`.
- Bytes-before-rows ordering with whole-object abandonment on failure: `internal/purge/sweeper.go:191-209`, proven by `sweeper_test.go:97-133` (rows all survive) and `:139-160` (variant keys never *offered*, the stronger claim).
- One transaction per media object, not per tick: `internal/purge/sweeper.go:215-223`, proven by `sweeper_test.go:121-123`.
- Orphan detection keys on parent absence, never `deleted_at`: `internal/mediavariant/purge.go:58`, `internal/variantfailures/purge.go:30`, proven by `mediavariant/purge_test.go:99-118` and `reconcile_test.go:42-51`.
- The `ReconcileLimit <= 0` off switch is falsifiable: `internal/purge/sweeper.go:130`, proven at `reconcile_test.go:170-193` — and the test's own comment (`:157-169`) correctly identifies that `limit=0` alone would pass with the guard deleted, which is why `-1` is exercised.
- `reconcileCapped` has a genuine negative case: `reconcile_test.go:110-112`.
- Arch tests are not vacuous: `internal/purge/arch_test.go:59-61`, `internal/admin/arch_test.go:29-31`, `:84-86`, `internal/purge/testdb_test.go:110-112`.
- No `os.Getenv`, no `logrus.StandardLogger`, no `ParseUnverified` anywhere in the service.

---

# Resolution — final fix wave

Both audits above, plus a whole-branch code review, were run against
`ba33f33..125ca36`. Five findings were fixed in `125ca36..7ba0546`; a scoped
re-review verdicted every one ADDRESSED with no new Critical/Important breakage,
and `make ci` passes end to end. The backend audit's **NEEDS-WORK** verdict and
the whole-branch review's **With fixes** verdict both refer to the pre-fix state.

| Finding | Source | Resolution |
| --- | --- | --- |
| Transaction-failure branch untested (PRD §10 / FR-PURGE-9) | whole-branch | `660646f` — `TestRunOnce_aFailedRowDeletionKeepsEveryRowAndContinues`; rollback assertion independently mutation-tested |
| `db.WithContext(ctx)` never applied in the sweep (DOM-15) | backend | `b536779` — both passes derive a context-bound handle; no bare `s.db` remains; deterministic cancellation test added |
| `MEDIA_PURGE_RECONCILE_LIMIT` misdescribed as bounding "rows" | whole-branch | `fe3b408` — corrected in the ConfigMap, `cmd/main.go` and `purge.Config` |
| State-changing DB access outside `administrator.go` (DOM-15/SUB-02) | backend | `f30b019` — `mediavariant/purge.go` split into `administrator.go` / `provider.go`, verbatim; `variantfailures` exempted (no such seam exists in that package — it is an `Entity`+`Store` ledger, like `processedevents`) |
| `admin/tablenames.go` is test-only code in the production binary | whole-branch | `7ba0546` — `git mv` to `tablenames_test.go`, unexported |

Findings deliberately **not** fixed, carried as follow-ups (full list and
rationale in `.superpowers/sdd/plan/progress.md`): the `ReapSparing`
exhaustiveness guard, `reconcileCapped`'s blindness to the ledger pass,
`ListOrphaned`'s missing `ORDER BY`, and the pre-existing `internal/admin`
resource-layer FAILs (SUB-01/03/04), none of which this task touches.
