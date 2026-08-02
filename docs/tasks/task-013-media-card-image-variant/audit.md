# task-013 code review — audit

Branch: `task-013-media-card-image-variant` · range `43987ca..b493cc1`

Three modular reviewers were dispatched in parallel per CLAUDE.md's code-review pattern.
**All three returned PASS with zero blocking findings.**

| Reviewer | Verdict |
|---|---|
| plan-adherence-reviewer | 9/9 tasks DONE — full adherence, READY_TO_MERGE |
| backend-guidelines-reviewer | PASS — DOM-_/SUB-_/SEC-* clear, zero blocking findings |
| frontend-guidelines-reviewer | PASS — all applicable FE-* clear, no FAIL items |

---

## Plan Audit — task-013-media-card-image-variant (plan adherence)

**Plan Path:** docs/tasks/task-013-media-card-image-variant/plan.md
**Audit Date:** 2026-08-02
**Branch:** task-013-media-card-image-variant
**Base commit for the implementation range:** 43987ca (PRD/design/plan commits precede it)
**Range audited:** 43987ca..HEAD (11 commits)

## Executive Summary

All 9 tasks in the plan were faithfully implemented; the checkboxes in plan.md were never ticked but every step's code, tests, and documentation artifact is present and matches the plan's verbatim text (or an authorized deviation). `go build`, `go vet`, `go test ./apps/media-service/...` (including `-race -count=2` on the concurrency-sensitive `processing` package) and the frontend Vitest suite for the touched files all pass. Both `kustomize` overlays render, `main` has zero PVCs, and the new `MEDIA_LAZY_VARIANT_CONCURRENCY` key appears in its render. The three pre-authorized deviations from the plan's literal text (SQLite `SetMaxOpenConns(1)`, the `capDropsRatherThanQueues`/`ignoresACancelledCallerContext` test edits, and the `recover()` guard) are all present and correctly implemented. The final commit (`b493cc1`) is a legitimate post-plan addition from code review — operator documentation plus three hardening assertions — not a plan deviation. No gaps found.

## Task Completion

| # | Task | Status | Evidence / Notes |
|---|------|--------|------------------|
| 1 | The `card` variant itself | DONE | `mediavariant/model.go:10` adds `VariantCard`; `mediaobject/contentvariant.go:12,32-33` adds `ContentCard` + parse arm; `processing/worker.go:29-34` adds `cardMaxEdge = 768` and doc-comment update; three-variant spec loop at `worker.go:186-193`. Tests: `contentvariant_test.go:20,34` (`"card"`/`"Card"`/`"cards"` cases), `worker_test.go:284-299` (`TestResizeDims_cardMaxEdge`), `worker_test.go:388-451` (`TestHandle_generatesThumbnailCardAndDisplay`, `TestHandle_pngOriginalProducesPngCard`), `fakeVariantAdmin` widened at `worker_test.go:64-80`. Commit `ee500ec`. |
| 2 | Additive `Upsert` + unique `(media_object_id, variant)` | DONE | `mediavariant/entity.go:9-23` composite `uniqueIndex:ux_media_variants_object_variant` on both columns; `administrator.go:15-24,47-62` adds `Upsert` to the interface and the `OnConflict`/`DoUpdates` implementation that explicitly excludes `created_at`; `provider_test.go:18-23,44-55` restates the `UNIQUE` DDL with rationale comment. New file `administrator_test.go` (211 lines) has all four planned tests (`TestUpsert_insertsThenUpdatesWithoutDuplicating`, `TestUpsert_preservesCreatedAt`, `TestUpsert_leavesOtherVariantsUntouched`, `TestUpsert_scopesConflictToTheVariant`). `fakeVariantAdmin.Upsert` stub added in `worker_test.go:76-79`. `deploy-preflight.md` created with the exact duplicate-row check and de-dupe SQL. Commit `a3574e9`. |
| 3 | Permanent-failure ledger | DONE | New package `internal/variantfailures/variantfailures.go` (83 lines) matches the plan's `Entity`, `TableName`, `Migration`, `Store`, `New`, `Recorded`, `Record` (first-failure-wins via `OnConflict{DoNothing: true}`), and `ReasonUndecodable`/`ReasonOriginalMissing` constants verbatim. `variantfailures_test.go` (100 lines) has all three planned tests. `cmd/main.go:28,41` wires the import and `variantfailures.Migration` into `database.SetMigrations`. `TestNoLossySaveRoundTrips` in `cmd/` still passes (verified independently). Commit `0979af2`. |
| 4 | Extract decode/build into package-level functions | DONE | `worker.go:80-93` adds the `Source` struct; `decodeOriginal` (line 246) and `buildVariant` (line 268) are converted to package-level functions taking `ObjectStore`/`Source` exactly as specified; `handle` (lines 176-201) builds a `Source` and calls both. Behaviour-preserving: pre-existing tests `TestHandle_undecodableBytesMarkFailedAndProcessed` and `TestHandle_missingOriginalMarksFailedAndProcessed` pass unedited (confirmed in the green `go test` run). Commit `d224bba`. |
| 5 | Lazy `CardGenerator` | DONE | New `processing/card.go` (208 lines, after the review-fix commit) implements `NewCardGenerator`, `Generate` with reserve/sem/goroutine exactly as planned: non-blocking single-flight (`reserve`/`release`), non-blocking semaphore acquire with drop-not-queue semantics, ledger check before decode, `ReasonUndecodable`/`ReasonOriginalMissing` classification, `Upsert`-only persistence, and `Info`/`Warn`/`Debug` log levels matching the plan's Global Constraints. `card_test.go` (initially 479 lines, +67 across two follow-up commits) contains every planned test: `doesNotDestroyExistingVariants`, `producesTheSameCardTheWorkerWould`, `singleFlightsPerMediaObject`, `releasesTheInFlightSlot`, `capDropsRatherThanQueues`, `zeroConcurrencyDisablesGeneration`, `negativeConcurrencyClampsToDisabled`, `undecodableOriginalIsRecordedAndNotRetried`, `missingOriginalRecordsItsOwnReason`, `transientFailureIsNotRecorded`, `ignoresACancelledCallerContext`. `-race -count=2` passes (verified independently, 46.6s, no race reports). Commit `52507ef`, hardened by `2e917a0` and `b493cc1` (see Authorized Deviations below). |
| 6 | Downgrade + scheduling in `Processor.Content` | DONE | `mediaobject/processor.go:70-107` adds `CardSource`, `CardGenerator` interface, `nopCardGenerator`; `processor.go:187-216` adds `cards CardGenerator` field, `ProcessorOption`, `WithCardGenerator`, and threads `opts ...ProcessorOption` through `NewProcessor`; `Content` (lines ~394-411) restructured to try `openVariant`, downgrade only on `ContentCard` + `server.ErrNotFound`, call `scheduleCard`, then retry with `ContentThumbnail` — matching the plan's downgrade-only-once contract; `openVariant` (extracted, lines ~417-451) and `scheduleCard` (lines ~460-483) match the plan's logic (status must be `StatusReady`, content type must classify as `ClassImage`). `resource.go:45-46` forwards `opts ...ProcessorOption` through `InitializeRoutes`. `processor_test.go` gained all planned tests: `TestContent_cardDowngradesToThumbnailOnly` (4 subtests), `TestContent_cardPresentServesTheCardBytes`, `TestContent_cardLookupErrorIs500WithNoDowngrade`, `TestContent_schedulesCardGenerationOnlyWhenEligible` (5 subtests), `TestContent_downgradesWithNoGeneratorWired`. Regression contract tests `TestContent_originalIsUnchanged`, `TestContent_variantMissingIs404AndServesNoOriginal`, `TestContent_crossFleetNeverTouchesLookupOrStore` verified still passing, unedited. Commit `bf9cf10`. |
| 7 | Wire generator in composition root | DONE | `cmd/main.go:96-108` reads `MEDIA_LAZY_VARIANT_CONCURRENCY` (default 4) and constructs `processing.NewCardGenerator` with the process-lifetime `ctx`; `main.go:154-158` passes `mediaobject.WithCardGenerator(cardGenerator{g: cardGen})` into `InitializeRoutes`; `main.go:211-224` adds the `cardGenerator` adapter translating `mediaobject.CardSource` → `processing.Source`, exactly as specified. `deploy/k8s/base/media-service/configmap.yaml:15-20` adds the `MEDIA_LAZY_VARIANT_CONCURRENCY: "4"` key with the planned comment. Verified independently: `go build`, `go vet`, `go test ./apps/media-service/...` all pass; both kustomize overlays render, `main` has 0 PVCs, and the new key appears in its render. Commit `4caa884`. |
| 8 | Frontend — request the `card` variant | DONE | `types/models/media.ts:12-21` widens `MediaVariant` to include `'card'` with the planned doc comment; `VehiclePhotoThumbnail.tsx:62-65,81` switches to `useMediaContentUrl(mediaId, 'card')` with the updated doc comment; `MediaService.ts:17,68-77` updates the route comment and the `getContentBlob` doc comment with the downgrade-exception paragraph verbatim; `lib/hooks/api/media.ts:16` refreshes the key-factory comment. Test files updated exactly as planned: `VehiclePhotoThumbnail.test.tsx` retargets the variant test to `'card'`; `MediaService.test.ts` adds the `?variant=card` test. Verified independently: `npm run test -- --run VehiclePhotoThumbnail MediaService` → 17/17 pass (2 files). Commit `c927f0c`. |
| 9 | Full verification and manual-verification notes | DONE | `docs/tasks/task-013-media-card-image-variant/manual-verification.md` created matching the plan's structure (pre-flight note, "photo becomes sharp" scenario, byte-cost comparison, "nothing else changed" checklist), later extended by `b493cc1` with the convergence-expectations section. `make ci` was reported already run and green by the requester (not re-run here); independent spot checks (`go build`, `go vet`, `go test ./apps/media-service/... -count=1`, `go test ./apps/media-service/internal/processing/ -race -count=2`, frontend Vitest, both kustomize overlays) all pass. Commit `53e16be`; step 7 (code review before PR) is satisfied by this audit itself plus the concurrent backend/frontend guideline reviews. |

**Completion Rate:** 9/9 tasks (100%)
**Skipped without approval:** 0
**Partial implementations:** 0

## Authorized Deviations (verified present and correct)

1. **`SetMaxOpenConns(1)` on test-only SQLite DBs.** The plan's verbatim `newCardTestDB` fixture (in-memory SQLite without `cache=shared`) was flaky under the generator's real goroutines. Confirmed at `apps/media-service/internal/processing/card_test.go:97-101` (`sqlDB.SetMaxOpenConns(1)` with an explanatory comment) and, applied later to the sibling package for the same reason, at `apps/media-service/internal/mediavariant/provider_test.go:29-41` (`newVariantTestDB`). Both compile and both packages' test suites pass.
2. **`TestCardGenerator_capDropsRatherThanQueues` gained a `waitFor(... len(g.sem) == 0)`, and `TestCardGenerator_ignoresACancelledCallerContext` had its three dead lines removed.** Confirmed in commit `2e917a0`: `card_test.go` adds the `waitFor` call right before rescheduling `m2` (closing a race where the semaphore token hadn't yet been released when the reschedule assertion ran), and removes the `reqCtx, cancel := context.WithCancel(...)` / `cancel()` / `_ = reqCtx` lines from the cancelled-context test, replacing them with a comment noting the guarantee is now enforced by `Generate`'s signature (no context parameter) rather than by discarding an unused variable.
3. **`card.go`'s background goroutine gained a `recover()` guard, plus a covering test.** Confirmed in commit `2e917a0`: `card.go`'s `Generate` goroutine registers `defer func() { if r := recover(); r != nil { ... } }()` as the *first* defer (so it runs last, after the semaphore/in-flight releases), matching the plan-adjacent rationale about LIFO defer ordering and read-traffic-driven crash loops. Covering test `TestCardGenerator_recoversFromDecodePanic` (with a `panicStore`/`panicReader` fake) is present in `card_test.go` and passes.

All three deviations are additive corrections/hardening, not scope reductions — none weakens a plan guarantee, and each is accompanied by a test that pins the new behavior.

## Post-Plan Addition (not a deviation)

Commit `b493cc1` (after Task 9 was otherwise complete) added, per the plan's own final review-gate expectation:
- `deploy-preflight.md`: rollout-ordering constraint (media-service must finish rolling before web, since old pods 400 rather than 404 on `?variant=card`), an index-build-lock row-count note, and a ledger-clear runbook line for stale `original-missing` entries.
- `manual-verification.md`: a "convergence over several visits" section explaining why a cold grid needs ~3 visits past the cache window to fully sharpen, given the drop-not-queue concurrency cap.
- Three additional test assertions: `TestContent_cardDowngradesToThumbnailOnly`'s two relevant subtests now assert the store is never asked for the *original* object key during a downgrade; `TestCardGenerator_producesTheSameCardTheWorkerWould` now asserts the PUT lands at the card's own object key.
- The `SetMaxOpenConns(1)` fix applied to `mediavariant/provider_test.go` (covered under Deviation 1 above).

This is documentation and test-hardening arising from the code-review step the plan itself mandates (Task 9, Step 7) — not an unauthorized departure from the plan's scope.

## Skipped / Deferred Tasks

None. No task was skipped, and no task was left partially implemented.

## Requirements/Scope Notes (not omissions)

Per the plan's own "Requirements that need no code" and "Out of scope" sections, the following are deliberately absent and are **not** findings:
- FR-2.4 (PDF uploads produce zero variant rows) — satisfied by pre-existing `Confirm` routing through `MarkReadyDirect` for non-image classes (`processor.go`), exercised by Task 6's "a document schedules nothing" subtest.
- §6.1 (no schema change for the `variant` column) — `variant` remains a plain string column; the only schema change is the Task 2 unique index.
- No changes to the 320/1280 max edges, no batch/sweep backfill, no `srcset`/per-breakpoint selection, no variant `Content-Length`, no generalization of the downgrade beyond card→thumbnail, no re-encoding of existing variants, no `fleet-service` changes, and PRD §9.6/§9.7 left open — all confirmed absent from the diff, consistent with the plan.

## Build & Test Results

| Service | Build | Tests | Vet | Notes |
|---------|-------|-------|-----|-------|
| apps/media-service | PASS | PASS | PASS | `go build ./apps/media-service/...`; `go test ./apps/media-service/... -count=1` (all 7 packages ok); `go vet ./apps/media-service/...` clean. |
| apps/media-service/internal/processing (race) | — | PASS | — | `go test ./apps/media-service/internal/processing/ -race -count=2` → PASS, 46.6s, no race reports, per plan Task 9 Step 2. |
| apps/media-service/cmd (entityguard) | — | PASS | — | `TestNoLossySaveRoundTrips` passes; the new `variantfailures` package has no `.Save(` call sites to flag. |
| apps/web (targeted) | — | PASS | — | `npm run test -- --run VehiclePhotoThumbnail MediaService` → 2 files, 17 tests, all pass. Full `make fe-build`/`make fe-test`/`make ci` were reported already run and green by the requester and were not re-run in this audit. |
| deploy/k8s overlays | PASS (render) | n/a | n/a | Both `local` and `main` overlays render without error; `main` has 0 PersistentVolumeClaims; `MEDIA_LAZY_VARIANT_CONCURRENCY` appears in the `main` render. Server dry-run against a live cluster was not performed in this audit (no cluster context available/requested). |

## Overall Assessment

- **Plan Adherence:** FULL
- **Recommendation:** READY_TO_MERGE (pending the parallel backend-guidelines and frontend-guidelines reviews, and a full `make ci` re-run if the requester wants independent re-confirmation beyond the targeted checks run here)

## Action Items

None required for plan adherence. Optional, non-blocking:
1. If not already done as part of this PR cycle, run the live-cluster `kubectl apply --dry-run=server` checks against both overlays per CLAUDE.md's build-and-verification guidance (not performed in this audit due to no cluster context being available/requested).
2. Consider whether `docs/tasks/task-013-media-card-image-variant/plan.md`'s checkboxes should be ticked retroactively for documentation hygiene — purely cosmetic, no functional impact, and explicitly not a criterion this audit judged against.

---

## Backend Audit — media-service (task-013-media-card-image-variant)

- **Service Path:** apps/media-service
- **Scope:** Go changes in range `43987ca..HEAD` — `internal/mediaobject`, `internal/mediavariant`, `internal/processing`, `internal/variantfailures` (new), `cmd/main.go`
- **Guidelines Source:** backend-dev-guidelines skill
- **Date:** 2026-08-02
- **Build:** PASS
- **Tests:** all packages `ok` (`go test ./... -count=1`); race variant reported by the requester as already green, not re-run here
- **Overall:** PASS

## Build & Test Results

```
$ cd apps/media-service && go build ./...
(no output — success)

$ cd apps/media-service && go test ./... -count=1
ok  	.../apps/media-service/cmd
ok  	.../apps/media-service/internal/mediaobject
ok  	.../apps/media-service/internal/mediavariant
ok  	.../apps/media-service/internal/processedevents
ok  	.../apps/media-service/internal/processing
ok  	.../apps/media-service/internal/storage
ok  	.../apps/media-service/internal/variantfailures
```

`go vet ./internal/mediaobject/... ./internal/mediavariant/... ./internal/processing/... ./internal/variantfailures/... ./cmd/...` — clean, no output.

`TestNoLossySaveRoundTrips` (entityguard, `apps/media-service/cmd/entityguard_test.go:16`) — PASS. Verified by reading `packages/shared-go/database/entityguard/entityguard.go:203-228`: `findSaveCall` matches only `x.Save(...)` call sites syntactically, so `mediavariant/administrator.go:56-61`'s `db.Clauses(clause.OnConflict{...}).Create(&e)` is structurally invisible to it — the plan's claim that entityguard cannot catch a violation here is correct, not just asserted.

## Domain Discovery

| Package | Classification | Evidence |
|---|---|---|
| `internal/mediaobject` | Domain (has `model.go`) | full DOM checklist |
| `internal/mediavariant` | Domain (has `model.go`) | full DOM checklist |
| `internal/processing` | Support (no `model.go`, no `resource.go`; background worker pool, never HTTP-facing) | `internal/processing/{worker,card}.go` |
| `internal/variantfailures` | Support / ledger (no `model.go`, no `resource.go`; same shape as the pre-existing `internal/processedevents`, confirmed by `ls internal/processedevents` → only `processedevents.go` + test) | `internal/variantfailures/variantfailures.go:6-8` (doc comment states this explicitly, and it matches the codebase precedent, not just its own claim) |

`cmd/main.go` is the composition root; audited for wiring correctness, not against the domain checklist.

**Codebase-wide note (pre-existing, not introduced by this branch):** the guidelines' `patterns-rest-jsonapi.md` and `ai-guidance.md` describe an api2go-based convention (`server.RegisterHandler(l)(si)(...)`, `HandlerDependency`/`d.Logger()`, `RestModel.GetName()/GetID()/SetID()`). A repo-wide search (`grep -rl "d.Logger()\|HandlerDependency"` and `grep -rl "server.RegisterHandler(l)(si)"` across every `apps/*/internal`) returns zero matches anywhere in the repository. media-service (and apparently every service) actually uses a different, simpler shared convention: `chi.Router` + `server.RegisterInputHandler[T](fn)` (generic function, `packages/shared-go/server/handler.go:42`) + `server.Resource`/`server.Document` DTOs (`packages/shared-go/server/jsonapi.go:10,17`). This diff does not introduce or worsen this divergence — `internal/mediaobject/resource.go`'s only change in this range is the two-line `opts ...ProcessorOption` threading (`git diff 43987ca..HEAD -- internal/mediaobject/resource.go`, +2/-2 total). DOM items keyed to the documented api2go shape (DOM-07, DOM-17) are evaluated below against what the file actually contains, and are called out as pre-existing rather than blocking this branch.

## Domain Checklist Results

### mediaobject

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| DOM-01 | `builder.go` exists | PASS | `internal/mediaobject/builder.go:16` `NewBuilder()`, `:38` `Build() (Model, error)` (pre-existing, untouched by this diff) |
| DOM-02 | `ToEntity()` method | PASS | `internal/mediaobject/entity.go:53` `func (m Model) ToEntity() Entity` |
| DOM-03 | `Make(Entity)` function | PASS | `internal/mediaobject/entity.go:35` `func Make(e Entity) Model` |
| DOM-04 | `Transform` function | PASS | `internal/mediaobject/contentvariant... rest.go:18` `func Transform(m Model) server.Resource` (service-wide DTO shape, not api2go `RestModel`; unchanged by this diff) |
| DOM-05 | `TransformSlice` function | PASS (present); no list handler exercises it | `internal/mediaobject/rest.go:36` defines it; `resource.go` has no `GET /media` list route in this range, so it is currently unused — pre-existing, not touched here |
| DOM-06 | Processor accepts `FieldLogger` | PASS | `internal/mediaobject/processor.go:202` `func NewProcessor(log logrus.FieldLogger, ...)` |
| DOM-07 | Handlers pass a request-scoped logger | PRE-EXISTING DEVIATION, not from this diff | `resource.go:46` builds one `Processor` at route-registration time from the `log` parameter closed over the router, not per-request `d.Logger()` — because this service has no `HandlerDependency`/`d.Logger()` concept at all (see codebase-wide note above). Not changed by `43987ca..HEAD`. |
| DOM-08 | POST/PATCH use typed input handling | PASS (service convention) | `resource.go:49` `r.Post("/media", server.RegisterInputHandler(func(w, req, attrs struct{...}) {...}))` — the service's generic equivalent of `RegisterInputHandler[T]`; unchanged by this diff |
| DOM-09 | Transform errors handled | PASS (N/A shape) | `Transform` in this service returns a single value, not `(RestModel, error)` (`rest.go:18`), so there is no error to discard; call sites (`resource.go:69,115,131,143`) use it directly |
| DOM-10 | Providers use lazy evaluation | PASS | `internal/mediavariant/provider.go:37` `database.SliceQuery(...)`, `:51` `database.Query(...)` — the provider touched by this diff (`GetByMediaObjectAndVariant`) is properly lazy |
| DOM-11 | No `os.Getenv()` in handlers | PASS | `grep -n "os.Getenv" internal/mediaobject/resource.go internal/mediavariant/*.go internal/processing/*.go internal/variantfailures/*.go cmd/main.go` → zero matches. All new config (`MEDIA_LAZY_VARIANT_CONCURRENCY`) is read once in `cmd/main.go:107` via `config.GetInt` and injected through `NewCardGenerator` |
| DOM-12 | No cross-domain business logic in handlers | PASS | `resource.go`'s only new code is threading `opts ...ProcessorOption` through; all card-generation eligibility logic (`scheduleCard`) lives in `processor.go:487-507`, not in `resource.go` |
| DOM-13 | Handlers don't call providers directly | PASS for the touched route; pre-existing FAIL elsewhere, untouched by this diff | `resource.go:256-283` (`InitializeInternalRoutes`) builds `prov := NewProvider(db)` and its handler calls `prov.ListActiveByFleetAndIDs(...)` directly (line 276) — a genuine DOM-13/anti-pattern violation with no documented circular-dependency exception comment. This code is **not** part of `43987ca..HEAD` (resource.go's only diff in-range is the 2-line `opts` threading noted above), so it predates and is unaffected by this branch. Flagged for visibility, not counted against this PR. |
| DOM-14 | No direct entity creation in handlers | PASS | `grep -n "db\.\|\.Create(\|\.Save(\|\.Delete(" internal/mediaobject/resource.go` → no direct writes; all writes go through `proc.InitUpload/StoreContent/Confirm/SoftDelete` |
| DOM-15 | `administrator.go` exists for writes | PASS | `internal/mediaobject/administrator.go` (pre-existing, untouched) |
| DOM-16 | Domain error → HTTP status mapping | PASS | New sentinels used by this diff (`server.ErrNotFound`, `server.ErrBadRequest` via `ParseContentVariant`) map through the pre-existing, unmodified `packages/shared-go/server/errors.go:17-35` `StatusFor` — 400/403/404/409/410/413/415/422 all covered |
| DOM-17 | JSON:API interface on REST models | PRE-EXISTING DEVIATION, not from this diff | `rest.go`'s `Attributes`/`server.Resource` shape has no `GetName()/GetID()/SetID()` — the whole service uses `server.Resource{Type, ID, Attributes}` instead (see codebase-wide note); unchanged by this diff |
| DOM-18 | Request models use flat structure | PASS | `resource.go:50-52` inline `attrs struct{ ContentType, OriginalFilename string }` — flat, no nested Data/Type/Attributes |
| DOM-19 | Table-driven tests | PASS | `processor_test.go:890-1011` (`TestContent_cardDowngradesToThumbnailOnly`), `:1068-1182` (`TestContent_schedulesCardGenerationOnlyWhenEligible`) use `t.Run` subtests covering the downgrade/eligibility matrices; individual tests elsewhere are precisely named single-property tests (e.g. `TestUpsert_preservesCreatedAt`), consistent with the rest of the codebase |

### mediavariant

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| DOM-01 | `builder.go` exists | PASS | `internal/mediavariant/builder.go:15` `NewBuilder()`, `:27` `Build() (Model, error)` |
| DOM-02 | `ToEntity()` method | PASS | `internal/mediavariant/entity.go:48` `func (m Model) ToEntity() Entity` |
| DOM-03 | `Make(Entity)` function | PASS | `internal/mediavariant/entity.go:34` `func Make(e Entity) Model` |
| DOM-06 | Processor accepts `FieldLogger` | N/A | package has no `processor.go`; its `Administrator`/`Provider` are consumed directly by `mediaobject.Processor` and `processing.Worker`/`CardGenerator`, both of which do accept `logrus.FieldLogger` (`processor.go:202`, `card.go:65-72` `NewCardGenerator(base, log logrus.FieldLogger, ...)`) |
| DOM-10 | Providers use lazy evaluation | PASS | `provider.go:37,51` — `database.SliceQuery`/`database.Query`, `db.WithContext(ctx)` at `:53` |
| DOM-14/15 | Writes via administrator, not direct entity creation elsewhere | PASS | `administrator.go:33-46` (`ReplaceForMediaObject`, pre-existing) and `:48-61` (`Upsert`, new); only `Worker` and `CardGenerator` call it (`worker.go:214`, `card.go:174`), never a handler — mediavariant has no `resource.go` at all |
| DOM-16 | New unique-index-driven writes classified/mapped correctly | PASS | `entity.go:20-21` composite `uniqueIndex:ux_media_variants_object_variant`; `administrator.go:56-58` scopes `clause.OnConflict.Columns` to exactly that pair, verified against `administrator_test.go:184-211` (`TestUpsert_scopesConflictToTheVariant`) which proves two different variants of the same object do not collide |
| DOM-19 | Table-driven / property tests | PASS | `administrator_test.go` — five targeted tests, each naming the single property it guards (insert-then-update, `created_at` preservation, non-destruction of sibling variants, conflict scoped to variant) |

## Sub-Domain / Support Package Review

### processing (support — background worker pool + lazy generator, no HTTP surface)

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| SUB-02 (writes via administrator) | PASS | `card.go:174` `g.variants.Upsert(v)` — never a raw `db.Create`/`.Save()` inside `processing` itself; the sole write in `card.go` and `worker.go` (`worker.go:214`) both go through `mediavariant.Administrator` |
| SUB-04 (no manual JSON parsing) | N/A | no HTTP handler in this package |
| CardGenerator context lifetime | PASS | `card.go:65-72` `NewCardGenerator(base context.Context, ...)`; `cmd/main.go:35` `ctx := context.Background()` is the value actually passed at `cmd/main.go:102` (`processing.NewCardGenerator(ctx, ...)`) — a process-lifetime context, not a request context. `card.go:128` derives the per-generation timeout from `g.base`, never from a caller-supplied context — confirmed structurally: `Generate(src Source)` (`card.go:95`) takes no `context.Context` parameter at all, so a request context cannot reach the goroutine even by mistake. `TestCardGenerator_ignoresACancelledCallerContext` (`card_test.go:450-463`) is the behavioral proof. |
| Panic-recovery / slot-release ordering | PASS | `card.go:120-126`: defers registered in order (1) `recover()`, (2) `<-g.sem`, (3) `g.release(...)`; Go defers run LIFO, so execution order is release-inFlight → release-sem → recover — i.e. both the semaphore token and the in-flight key are freed *before* the panic is swallowed, matching the comment at `card.go:111-113`. `TestCardGenerator_recoversFromDecodePanic` (`card_test.go:470-479`) asserts both `!g.inFlightFor("m1")` and `len(g.sem) == 0` after a `panicStore` induces a panic — proves the ordering empirically, not just by reading the source. |
| Concurrency cap semantics (drop, not queue) | PASS | `card.go:102-108`: non-blocking `select { case g.sem <- struct{}{}: default: ...drop... }`, acquired *before* the goroutine is spawned. `TestCardGenerator_capDropsRatherThanQueues` (`card_test.go:303-339`) proves a second object's `Generate` is dropped while the cap is held and is picked up again on a later call (not silently lost). |
| Single-flight per media object | PASS | `card.go:184-198` (`reserve`/`release`, mutex-guarded map); `TestCardGenerator_singleFlightsPerMediaObject` (`card_test.go:250-281`) drives 8 concurrent `Generate` calls for the same object and asserts exactly one `GetObject` call and one row. |
| `ErrPermanent` / `storage.ErrObjectNotFound` classification | PASS | `worker.go:246-260` `decodeOriginal` wraps both `fmt.Errorf("%w: original bytes were never stored: %w", ErrPermanent, err)` (missing-original path, both sentinels wrapped) and `fmt.Errorf("%w: %w", ErrPermanent, err)` (decode-failure path, `ErrPermanent` only) — Go 1.25 (`go.mod:3`) supports multiple `%w` verbs, so both `errors.Is(err, ErrPermanent)` and `errors.Is(err, storage.ErrObjectNotFound)` resolve correctly at `card.go:149-153`, which is exactly what distinguishes `ReasonUndecodable` from `ReasonOriginalMissing`. `TestCardGenerator_undecodableOriginalIsRecordedAndNotRetried` and `TestCardGenerator_missingOriginalRecordsItsOwnReason` (`card_test.go:374-420`) each assert the correct reason lands in the ledger, and `TestCardGenerator_transientFailureIsNotRecorded` (`card_test.go:424-441`) proves a non-`ErrPermanent` failure is never recorded. |
| Never calls `ReplaceForMediaObject` | PASS | `card.go:172-177` uses only `g.variants.Upsert(v)`; `TestCardGenerator_doesNotDestroyExistingVariants` (`card_test.go:153-207`) seeds thumbnail+display via `ReplaceForMediaObject`, runs lazy generation, and asserts all three rows survive. |
| Identical bytes to the upload-worker path | PASS | `worker.go:80-94` documents `Source` as the shared shape; `worker.go:189,206` and `card.go:147,167` both call the same package-level `decodeOriginal`/`buildVariant` (not methods on `Worker`, confirmed by their function signatures at `worker.go:246,266` — no `(w *Worker)` receiver) |

### variantfailures (support / ledger, mirrors `processedevents`)

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| Ledger shape matches stated precedent | PASS | `internal/processedevents` contains only `processedevents.go` + test (no `model.go`/`resource.go`), exactly mirrored by `variantfailures.go` + test |
| First-failure-wins / no write amplification | PASS | `variantfailures.go:76-77` `clause.OnConflict{DoNothing: true}`; `TestRecord_firstReasonWins` (`variantfailures_test.go:79-100`) proves a second `Record` call does not overwrite the first reason |
| Scoping | PASS | `variantfailures.go:57-61` keys on `(media_object_id, variant)`; `TestRecorded_isScopedToTheObjectAndVariant` (`variantfailures_test.go:55-74`) checks it does not leak across variants or objects |
| Transient failures never recorded | PASS (verified from the `processing` side, since `variantfailures.Store.Record` has no classification logic of its own — that's `card.go`'s job) | see `TestCardGenerator_transientFailureIsNotRecorded` above |

## Security Review (SEC-*)

media-service handles fleet-scoped access to stored media; `Processor.Content` is the authorization boundary this task extends.

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| SEC-01 (fleet-scoping before any privileged action) | PASS | `processor.go:392-393`: `Content` calls `pr.GetByID(id, identityFleetID)` — which internally runs `AuthorizeAccess` (`processor.go:130-135`) — as its **first** statement, before `openOriginal`, `openVariant`, or `scheduleCard` can run. `scheduleCard` (`processor.go:487`) is only ever invoked from within `Content` after that authorization has already succeeded (`processor.go:413`), so no unauthenticated or cross-fleet caller can reach it. Proven behaviorally by `TestContent_crossFleetNeverTouchesLookupOrStore` (`processor_test.go:841-861`) and, specific to the new card-scheduling path, `TestContent_schedulesCardGenerationOnlyWhenEligible/a_cross-fleet_caller_schedules_nothing_and_never_reaches_the_lookup` (`processor_test.go:1162-1181`) — asserts zero variant lookups, zero store reads, and zero scheduled generations for a cross-fleet caller. |
| SEC-01b (cross-fleet is 404, never 403 — no existence oracle) | PASS | `processor.go:130-134` `AuthorizeAccess` returns `server.ErrNotFound` (never `ErrForbidden`) on fleet mismatch; `TestAuthorizeAccess_404CrossFleet` (`processor_test.go:24-46`) and `TestContent_crossFleetNeverTouchesLookupOrStore` (`processor_test.go:852-853`, asserting `server.ErrNotFound`, "never 403") both pin this. |
| SEC (downgrade path preserves scoping and the size ceiling) | PASS | `processor.go:405-420`: the downgrade fires only when `want == ContentCard && errors.Is(err, server.ErrNotFound)` — every other outcome (display/thumbnail miss, or any 500) returns unchanged. `TestContent_cardDowngradesToThumbnailOnly/display_missing_still_404s_even_with_a_thumbnail_present` (`processor_test.go:945-966`) proves the downgrade does not generalize past card→thumbnail. |
| SEC (drift-repair path re-authorizes nothing extra, but doesn't need to — it runs inside the already-authorized `Content` call) | PASS | `TestContent_cardDowngradesToThumbnailOnly/card_row_present_but_object_missing_downgrades_AND_reschedules` (`processor_test.go:968-1010`) exercises the DB/store-drift repair branch and confirms it is reached only via the same authorized `Content` call — no separate/unauthenticated entry point exists for it. |
| SEC (500s are never silently downgraded to a served thumbnail) | PASS | `processor.go:409` guards on `errors.Is(err, server.ErrNotFound)` specifically; `TestContent_cardLookupErrorIs500WithNoDowngrade` (`processor_test.go:1045-1065`) proves a database fault propagates as-is with zero scheduled generations. |
| SEC (background work cannot be triggered pre-authorization by the internal, unauthenticated endpoint) | PASS | `InitializeInternalRoutes` (`resource.go:256-284`) calls only `prov.ListActiveByFleetAndIDs` — it never touches `Processor.Content`, `scheduleCard`, or any `CardGenerator`; there is no code path from the unauthenticated `/internal/media` route into card scheduling. |
| JWT/token handling | N/A | this diff does not touch JWT parsing/validation/revocation; media-service trusts the token's `active_fleet_id`/role claims as already established (`resource.go:20-21`, `AuthorizeAccess` doc at `processor.go:127-129`), unchanged in this range |
| Open redirect | N/A | no redirect/callback handling in this diff |
| Hardcoded secrets | PASS | `grep -n "os.Getenv\|secret\|password\|apikey" ` across the touched files turned up nothing beyond config-driven values (`MEDIA_LAZY_VARIANT_CONCURRENCY` via `config.GetInt`, `cmd/main.go:107`) |

## Wiring / Composition-Root Review (`cmd/main.go`)

| Item | Status | Evidence |
|---|---|---|
| `mediaobject` never imports `processing` (port stays a tree) | PASS | `internal/mediaobject/processor.go:1-22` imports — no `processing`, no `mediavariant` |
| `processing/card.go` never imports `mediaobject` | PASS | `internal/processing/card.go:1-14` imports — no `mediaobject` |
| Adapter lives in the composition root | PASS | `cmd/main.go:216-225` `type cardGenerator struct{ g *processing.CardGenerator }` implementing `mediaobject.CardGenerator`, mirroring the pre-existing `variantLookup` adapter at `cmd/main.go:198-209` |
| `MEDIA_LAZY_VARIANT_CONCURRENCY` default/clamp asymmetry vs `MEDIA_WORKERS` | PASS, verified not just claimed | `cmd/main.go:88-91` clamps `MEDIA_WORKERS < 1` up to `1`; `cmd/main.go:107` passes `MEDIA_LAZY_VARIANT_CONCURRENCY` straight through to `NewCardGenerator`, which clamps negatives to `0` at `card.go:73-75` (`0` stays `0`, disabling lazy generation) — confirmed by `TestCardGenerator_zeroConcurrencyDisablesGeneration` and `TestCardGenerator_negativeConcurrencyClampsToDisabled` (`card_test.go:343-370`) |
| Migration registered | PASS | `cmd/main.go:41` `variantfailures.Migration` added to the `AutoMigrate` chain alongside the pre-existing three |

## Summary

### Blocking (must fix)
- None found in the code introduced or modified by `43987ca..HEAD`.

### Non-Blocking (should fix)
- DOM-13: `internal/mediaobject/resource.go:256-283` (`InitializeInternalRoutes`) has a handler calling `mediaobject.Provider.ListActiveByFleetAndIDs` directly, bypassing the processor layer, with no documented circular-dependency exception comment per `anti-patterns.md`'s "Exception: Cross-Domain Read-Only Views" requirements. **Pre-existing** — not part of this diff (resource.go's only change in-range is the 2-line `opts ...ProcessorOption` addition) — flagged for a future cleanup, not for this branch.
- DOM-07 / DOM-17: media-service (and, by repo-wide grep, every MyFleet service) uses a `chi` + `server.RegisterInputHandler[T]` + `server.Resource` convention rather than the api2go/`HandlerDependency`/`d.Logger()` shape the skill's `patterns-rest-jsonapi.md` documents. This is a codebase-wide, pre-existing divergence, not something this branch introduced or could reasonably be asked to fix in isolation. Worth a follow-up to reconcile the skill documentation with actual practice, or vice versa.
- DOM-05: `mediaobject.TransformSlice` (`rest.go:36`) remains unused — no list handler exists for `/media`. Pre-existing, unrelated to this diff.

---

## Frontend Audit — task-013-media-card-image-variant

- **Audit Scope:** Frontend commit `c927f0c` ("feat(web): request the card variant for the vehicles-list hero"), the only commit in `43987ca..HEAD` touching `apps/web/src`. Six files:
  - `apps/web/src/types/models/media.ts` (Type)
  - `apps/web/src/components/features/vehicles/VehiclePhotoThumbnail.tsx` (Component)
  - `apps/web/src/components/features/vehicles/VehiclePhotoThumbnail.test.tsx` (Test)
  - `apps/web/src/services/api/MediaService.ts` (Service)
  - `apps/web/src/services/api/MediaService.test.ts` (Test)
  - `apps/web/src/lib/hooks/api/media.ts` (Hook)
  Later commits `53e16be` and `b493cc1` on this branch touch only Go tests and docs, confirmed via `git show --stat` — no additional frontend surface in scope.
- **Guidelines Source:** frontend-dev-guidelines skill
- **Date:** 2026-08-02
- **Build:** Not re-run (per instructions: full suite / `make fe-build` already verified passing on this branch)
- **Tests:** Not re-run in full (per instructions). Targeted reasoning-verification performed on `VehiclePhotoThumbnail.test.tsx`'s variant assertion (see Verification Beyond Checklist below); not executed live to avoid mutating the tree.
- **Overall:** PASS

## Build & Test Results

Per task instructions, the full suite (37 files / 260 tests) and `make fe-build` were already confirmed passing on this branch and were not re-run. No doubt arose during this audit that warranted a focused `vitest run`.

## File Inventory

- `apps/web/src/types/models/media.ts` — Type. Widens `MediaVariant` to add `'card'` and rewrites the doc comment describing rendition sizes.
- `apps/web/src/components/features/vehicles/VehiclePhotoThumbnail.tsx` — Component. Switches `useMediaContentUrl(mediaId, 'thumbnail')` → `useMediaContentUrl(mediaId, 'card')`; updates its doc comment.
- `apps/web/src/components/features/vehicles/VehiclePhotoThumbnail.test.tsx` — Test. Renames and rewrites the variant-assertion test to expect `'card'`.
- `apps/web/src/services/api/MediaService.ts` — Service. Documentation-only change: rewrites `getContentBlob`'s doc comment to describe the card-variant lazy-downgrade contract, and updates the route comment's variant list.
- `apps/web/src/services/api/MediaService.test.ts` — Test. Appends one new case asserting `?variant=card` is sent.
- `apps/web/src/lib/hooks/api/media.ts` — Hook. Comment-only: the query-key-factory example comment is updated from a `'thumbnail'` example to a `'card'` example. No functional change (`content()` was already generic over `MediaVariant`).

## Anti-Pattern Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-01 | No `any` type | PASS | `grep -n ": any\b\|as any\b"` across all six changed files returned zero matches (exit code 1). |
| FE-02 | No manual class concatenation | PASS | No `className` touched by this diff; N/A but verified zero matches in the diff hunks. |
| FE-03 | No direct API client calls in components | PASS | `VehiclePhotoThumbnail.tsx` imports only `useMediaContentUrl` from `lib/hooks/api/media` (line 4) and `cn`/`Skeleton` — no `@/lib/api/client` import. |
| FE-04 | No inline Zod schemas in components | PASS | No `z.object`/`z.string` etc. added or present in any changed file. |
| FE-05 | No spinners for content loading | PASS | `VehiclePhotoThumbnail.tsx:87` uses `<Skeleton>` for the loading state; no `animate-spin` introduced. |
| FE-06 | No hardcoded colors | PASS | No new class names introduced by this diff (pre-existing `bg-muted` etc. at `VehiclePhotoThumbnail.tsx:49` are semantic and unchanged). |
| FE-07 | No state mutation | PASS | No `.push`/`.splice`/`.sort` in any changed file. |
| FE-08 | No default exports for components | PASS | `VehiclePhotoThumbnail.tsx:75` — `export function VehiclePhotoThumbnail(...)`, named export. |
| FE-09 | Error handling with `createErrorFromUnknown` | N/A | No new `.catch(` or error-handling branch added by this diff — the component's existing `isError` passthrough (unchanged) is out of scope. |

## Architecture Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-10 | JSON:API model shape | N/A | `MediaVariant` (`types/models/media.ts:22`) is a string-literal union describing a query parameter, not a JSON:API resource model; the actual `MediaObject` model (line 40, unchanged) already follows `{ id, attributes }`. |
| FE-11 | Service extends `BaseService` (when applicable) | PASS | `MediaService.ts:23` (`class MediaService { private readonly basePath = '/api/media'; ... }`) follows the documented "Direct API Client Pattern" (`.claude/skills/frontend-dev-guidelines/resources/patterns-service-layer.md` §2) — a simple resource with no validation/transformation needs. Pre-existing pattern, unchanged by this diff. |
| FE-12 | Query key factory uses `as const` | PASS | `lib/hooks/api/media.ts:19-28` — every key builder (`all`, `details()`, `detail()`, `contents()`, `content()`) ends in `as const`; `content(id, variant)` at line 27-28 includes `variant` in the key, so `'card'` and `'thumbnail'` produce distinct cache entries for the same media id. |
| FE-13 | Forms use `react-hook-form` + `zodResolver` | N/A | No form touched by this diff. |
| FE-14 | Schema in `lib/schemas/` with inferred type | N/A | No Zod schema touched by this diff. |

## Styling Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-15 | Interactive elements show `cursor-pointer` | N/A | No interactive element added or changed by this diff. The rendered `<img>` (`VehiclePhotoThumbnail.tsx:104-108`) carries no `onClick`. |

## Testing Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-16 | Tests exist for changed components | PASS | `VehiclePhotoThumbnail.test.tsx:48-59` covers the variant change; `MediaService.test.ts:43-46` covers the new `'card'` value at the service boundary. |
| FE-17 | Mocks updated when services changed | N/A | `MediaService.getContentBlob`'s signature/interface is unchanged (still `(id, variant?)`); only its doc comment changed. Inline `vi.mock('../../../services/api/MediaService', ...)` blocks in `VehiclePhotoThumbnail.test.tsx:11-13`, `VehicleCard.test.tsx:12-14`, `media.test.ts:14`, etc. all still satisfy the same mocked shape (`{ getContentBlob: vi.fn() }` or similar) — no mock needed updating. |

## Verification Beyond Checklist (per task instructions)

**Newly-unhandled case from the widened union:** searched all of `apps/web/src` for switch/equality dispatch over `MediaVariant` (`grep -rn "variant ===" ` and `case '...'` patterns). The only equality check is `MediaService.ts:80` (`variant === 'original' ? '' : ...`), which is not exhaustive by design — every non-`'original'` value (including the new `'card'`) correctly falls through to the `?variant=` suffix branch. No switch statement anywhere branches over the full `MediaVariant` union, so widening it to include `'card'` introduces no unhandled case at any call site. `MediaThumbnail.tsx:42` (the gallery tile, deliberately untouched) calls `useMediaContentUrl(mediaId)` with no variant argument at all, confirmed unaffected.

**Query key factory variant distinction:** `mediaKeys.content(id, variant)` (`lib/hooks/api/media.ts:27-28`) includes `variant` as a distinct key segment, so `'card'` and `'thumbnail'` are cached under different keys from each other. The known, accepted gap (per task framing, PRD §9.6 open) is that a `'card'` request downgraded server-side to thumbnail bytes is still cached under the `'card'` key — indistinguishable from an eventual sharp `'card'` response until invalidated/refetched. Nothing worse than that was found: the key factory itself is correct and does not conflate `'card'` with `'thumbnail'`.

**Would the changed tests catch a revert to `'thumbnail'`?** Yes for the component-level regression test: `VehiclePhotoThumbnail.test.tsx:56-58` asserts `mediaService.getContentBlob` was called with the literal tuple `('m1', 'card')`; reverting `VehiclePhotoThumbnail.tsx:81` back to `'thumbnail'` would make this assertion fail. The new `MediaService.test.ts:43-46` case passes `'card'` directly as a literal argument to `getContentBlob` and does not depend on the component, so it would not catch a component-level revert — it only pins the service's own query-string mapping, which is a legitimate, narrower purpose. `VehicleCard.test.tsx` (not part of this diff) does not assert on the variant argument at all, so it neither catches nor conflicts with the change.

**Documentation accuracy against the stated backend contract:** `MediaService.ts:71-77` states the downgrade is exactly one exception (missing `card` → `thumbnail` bytes), does not generalize (missing `display` still 404s), never substitutes anything larger than requested, and carries no signal of the downgrade to the caller. This matches the contract given for this audit verbatim and was not found to overstate or understate it.

## Summary

### Blocking (must fix)
- None.

### Non-Blocking (should fix)
- None identified beyond the already-acknowledged, deliberately-open PRD §9.6 cache-staleness gap (downgraded `card` response cached under the same key as the eventual sharp one), which this diff does not make worse.
