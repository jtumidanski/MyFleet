# Plan Audit — task-027-record-attachment-management

**Plan Path:** docs/tasks/task-027-record-attachment-management/plan.md
**Audit Date:** 2026-08-25
**Branch:** task-027-record-attachment-management
**Base:** 4f2fa29 → head 6afeb8e (9 commits)

## Plan Adherence

### Executive Summary

All 8 plan tasks are implemented; nothing was silently skipped or deferred. Every file named in the
plan's File Structure table exists with the described change, and every plan-mandated test name is
present in the tree. `make vet`, `make test`, `make build`, `make fe-test` and `make fe-build` all
pass on this worktree. `make lint-check` fails, but for an environmental reason unrelated to this
branch (a `golangci-lint` binary built with go1.26 panics against the repo's go1.27 toolchain, on all
six Go modules including five this branch never touched). `git diff --name-only main...HEAD --
deploy/` is empty, as required.

Two PRD §10 frontend acceptance criteria have no end-to-end automated coverage, both because
`apps/web/src/components/features/vehicles/detail/VehicleRecordDrawer.tsx` gained 67 lines of new
wiring while `VehicleRecordDrawer.test.tsx` was not touched. Confirmed independently — see the
Coverage Gap section.

### Task Completion

| # | Task | Status | Evidence |
|---|------|--------|----------|
| 1 | Partial unique index + de-duplication | DONE | `apps/fleet-service/internal/maintenancerecord/entity.go:41-92` (`Migration` → `ApplyPartialIndexes` → `dedupeLiveDocuments`, sqlite/Postgres dialect split on the index name, `CURRENT_TIMESTAMP` for cross-dialect dedupe). Wired into startup at `apps/fleet-service/cmd/main.go:54`. Tests: `entity_test.go:25,45,69,107` (unique, partial/reattach, dedupes pre-existing dupes, idempotent). Helper split done: `provider_test.go:17` `newTestDBWithoutIndexes`, `provider_test.go:56-60` `newTestDB` applies the real index. |
| 2 | `Administrator.AttachDocument` / `DetachDocument` | DONE | Interface `administrator.go:24-31`; `AttachDocument` `administrator.go:136-192` (single tx, `clause.Locking{Strength:"UPDATE"}` guarded by `tx.Name() != "sqlite"` at :140-141, idempotency check before cap check at :155-167, re-read at :178-188); `DetachDocument` `administrator.go:209-218`. Tests: `administrator_test.go:91,116,137,154,170,177,192,221,242` — 9 new cases covering add, idempotency, eleventh rejected with no write, reattach on a full record, missing/soft-deleted record, soft-delete leaves the row, unattached not-found, cross-record isolation. |
| 3 | `Processor` pass-throughs | DONE | `processor.go:70-95` (`AttachDocument`, `DetachDocument`, `ErrNotFound → server.ErrNotFound`). Tests: `processor_test.go:26,36,49,61,69` — not-found translation both ways, validation passed through untranslated, model returned. |
| 4 | Attach and detach routes | DONE | `resource.go:332-393` `POST /maintenance-records/{id}/document-media`; `resource.go:395-440` `DELETE /maintenance-records/{id}/document-media/{mediaId}`. Both run the `proc.GetByID` → `vehicleAccessor.GetByID` → `RequireSameFleet` (:356, :416) → `RequireWrite` (:360, :420) preamble. Frontend route comment updated: `apps/web/src/services/api/MaintenanceRecordService.ts:20-21`. Tests: `resource_test.go:239-521` — 13 new cases. Plan-noted rename applied: `TestAttachDocumentRoute_softDeletedRecordIsNotFound` at `resource_test.go:358`. |
| 5 | Client detach service method, hook, widened invalidation | DONE | `MaintenanceRecordService.ts:66-75` `removeDocumentMedia`; `lib/hooks/api/maintenance.ts:241-256` `useRemoveMaintenanceRecordDocument` (detach first, best-effort `mediaService.remove` with `console.warn` at :247-250, then invalidates lists + detail + vehicle detail); append hook widened at `maintenance.ts:215-217`. Tests: `lib/hooks/api/maintenance.test.tsx:48,69,80,90,117`. The plan's `expect(mediaService.remove).not.toHaveBeenCalled()` was replaced by `expectNoCall` (`maintenance.test.tsx:13,87`) to satisfy the repo's `no-restricted-syntax` rule — approved deviation, assertion equivalent. |
| 6 | Capacity-aware picker | DONE | `usePendingAttachments.ts:48` takes `existingCount = 0`; room arithmetic `:69-70`; `isFull` `:147`. Picker: `AttachmentPicker.tsx:30,45,48-63` (`remaining`, three-branch helper text) and `:74` `disabled={disabled || isFull}`. Threaded through `MaintenanceRecordForm.tsx:52,68,71,230` and the drawer at `VehicleRecordDrawer.tsx:270` (`existingAttachmentCount={record.attributes.documentMediaIds?.length ?? 0}`). Tests: `usePendingAttachments.test.ts:150,163,175`; `AttachmentPicker.test.tsx:21,30,38,53,69`. |
| 7 | Drawer remove control | DONE | `RecordAttachmentList.tsx:23-24,27,31,39-40,60,72,120,128-147` — optional `onRemove`/`canRemove`, remove button on all three row shapes including the unavailable row. Drawer: `pendingRemoval` state `VehicleRecordDrawer.tsx:96`, mutation `:110`, `handleConfirmRemoveAttachment` `:204-215`, `canRemove={canWrite} onRemove={setPendingRemoval}` `:304-308`, `AlertDialog` confirmation `:331-359`. Tests: `RecordAttachmentList.test.tsx:81,92,101,113,127`. |
| 8 | Full-branch verification | DONE (with caveat) | Commit `6afeb8e`. Independently re-verified: `deploy/` diff empty; vet/test/build/fe-test/fe-build pass. `make lint-check` currently fails for an environmental toolchain reason (below), not a branch defect. |

**Completion rate:** 8/8 (100%).
**Skipped without approval:** 0. **Partial:** 0.

Bookkeeping note: every step checkbox in `plan.md` is still `- [ ]`. The work is done; the plan file
was never marked off. Cosmetic, but it makes the plan misleading as a standalone artifact.

### Build & Test Results

| Target | Result | Notes |
|---|---|---|
| `make vet` | PASS | |
| `make test` | PASS | |
| `make build` | PASS | |
| `make fe-test` | PASS | |
| `make fe-build` | PASS | |
| `make lint-check` | FAIL (environmental) | `golangci-lint` panics: `file requires newer Go version go1.27 (application built with go1.26)`. Fails on `apps/auth-service`, `apps/fleet-service`, `apps/media-service`, `apps/notification-service`, `packages/dto-go`, `packages/shared-go` — five of the six are untouched by this branch, so the cause is a stale linter binary, not this code. The web half of `lint-check` (prettier + eslint) passes. |
| `deploy/` drift | NONE | `git diff --name-only main...HEAD -- deploy/` is empty. |

### PRD §10 Acceptance Criteria

Backend:

| Criterion | Verdict | Covering test |
|---|---|---|
| Attach route exists, `201`, id in `documentMediaIds` | MET | `resource_test.go:239 TestAttachDocument_returns201WithTheUpdatedRecord` |
| Same `mediaId` twice → `201` both times, one live row | MET | `resource_test.go:303 TestAttachDocument_isIdempotent`; `administrator_test.go:116 TestAttachDocument_isIdempotentForAnAlreadyAttachedMedia` |
| Eleventh document → `422`, writes nothing | MET | `resource_test.go:321 TestAttachDocument_atTheCapIs422`; `administrator_test.go:137 TestAttachDocument_rejectsTheEleventhAndWritesNothing` |
| Cross-fleet media → `422`, writes nothing | MET | `resource_test.go:264 TestAttachDocument_rejectsMediaThatFailsOwnershipAndWritesNothing` |
| Attach `403` viewer / cross-fleet, `404` soft-deleted | MET (cross-fleet is `404` per Global Constraints, not `403`) | `resource_test.go:330 TestAttachDocument_viewerIsForbidden`; `:349 TestAttachDocument_otherFleetIsNotFound`; `:358 TestAttachDocumentRoute_softDeletedRecordIsNotFound` |
| Detach `204`, id gone from next `GET`, detail and list | MET | `resource_test.go:372 TestDetachDocument_returns204AndTheIDIsGoneFromTheNextGet`; `:395 TestDetachDocument_theIDIsGoneFromTheListPathToo` |
| Detach of unattached/already-detached → `404` | MET | `resource_test.go:429 TestDetachDocument_unattachedIsNotFound`; `administrator_test.go:221`, `:242` |
| Detach soft-deletes; row present with `deleted_at` | MET | `administrator_test.go:192 TestDetachDocument_soft_deletesTheRowAndLeavesItPresent` |
| `PATCH` unchanged, leaves document rows untouched | MET | `resource_test.go:496 TestPatch_leavesDocumentRowsUntouched` |
| Both routes identical for `modification` kind | MET | `resource_test.go:480 TestAttachAndDetach_behaveIdenticallyForAModificationKindRecord` |

Frontend:

| Criterion | Verdict | Covering test |
|---|---|---|
| Edit a saved record, add two files → both attach, no toast | **NOT COVERED end-to-end** | Server half `resource_test.go:239`, `:303`. Drawer attach loop (`VehicleRecordDrawer.tsx:166-181`) is pre-existing, unchanged, untested. |
| Attachment count updates without manual refresh (FR-UI-3) | MET | `maintenance.test.tsx:117` (append invalidates lists); `:90` (remove invalidates detail + lists) |
| 8 attachments → 2 more and no more; helper text before the pick | MET | `AttachmentPicker.test.tsx:30`, `:53`; `usePendingAttachments.test.ts:150`, `:163` |
| Remove control shown for member, hidden for viewer | PARTIAL — component contract only | `RecordAttachmentList.test.tsx:81`, `:92`, `:101`. The drawer's `canRemove={canWrite}` binding (`VehicleRecordDrawer.tsx:306`) is unasserted. |
| Removing asks for confirmation, then removes the row | **NOT COVERED** | Halves covered: `RecordAttachmentList.test.tsx:101` (`onRemove` fires) and `maintenance.test.tsx:48` (hook order). The `AlertDialog` at `VehicleRecordDrawer.tsx:331-359` has no test. |
| "Attachment unavailable" row can still be removed | MET | `RecordAttachmentList.test.tsx:127` |
| Failed media delete after successful detach is not a failed removal | MET | `maintenance.test.tsx:69` |

Verification:

| Criterion | Verdict |
|---|---|
| `make ci` passes | PARTIAL — 5/6 targets pass; `lint-check` blocked by a stale `golangci-lint` binary (environmental). |
| New Go tests cover FR-ATT-6/7/8, FR-DET-2/4 | MET (see backend table) |
| New frontend tests cover FR-CAP-1/2 and viewer gating in FR-UI-1 | MET at component level; drawer-level gating unasserted |

### Coverage Gap — independent confirmation

**Confirmed.** `apps/web/src/components/features/vehicles/detail/VehicleRecordDrawer.test.tsx` is
not in this branch's diff, and its service mock (`VehicleRecordDrawer.test.tsx:12-14`) declares only
`{ get, patch, remove }` — neither `appendDocumentMedia` nor `removeDocumentMedia` is stubbed. No
test in the branch mounts the drawer and exercises either attachment path.

The two halves are asymmetric, and that changes the recommendation:

- *"Adding two files attaches both; no toast"* — the drawer's sequential attach loop
  (`VehicleRecordDrawer.tsx:166-181`) is **pre-existing code from task-004, unchanged by this
  branch**. The only thing this branch changed for that criterion is the server route it was already
  calling. The route is well tested. Residual untested risk is low; a drawer test here would be
  regression insurance for code this branch did not write.
- *"Removing asks for confirmation, then removes the row"* — this is **67 lines of entirely new
  drawer code** with zero coverage: the `onRemove` → `setPendingRemoval` binding, the dialog's
  `open`/`onOpenChange` contract, the `e.preventDefault()` in `AlertDialogAction`, the cancel path,
  and `canRemove={canWrite}`. Nothing fails if any of that is broken or reordered.

**Recommendation: NEEDS_REVIEW, not a merge blocker — but write one drawer test file before merge if
the cost is acceptable, and it is.** Scope it to the removal path only:

1. Extend the existing mock at `VehicleRecordDrawer.test.tsx:13` with
   `appendDocumentMedia: vi.fn(), removeDocumentMedia: vi.fn()`, and mock `MediaService.remove`.
2. `it('asks for confirmation before removing an attachment')` — render with `canWrite` and a record
   holding one `documentMediaId`; click the row's remove control; assert the dialog title
   "Remove this attachment?" is visible and `maintenanceRecordService.removeDocumentMedia` has **not**
   been called (use `expectNoCall`, per the lint rule).
3. `it('removes the attachment once confirmed')` — click "Remove"; assert
   `removeDocumentMedia` was called with `(record.id, mediaId)` and the row is gone.
4. `it('does not remove when the dialog is cancelled')` — click "Cancel"; `expectNoCall`.
5. `it('offers no remove control to a viewer')` — `canWrite={false}`; assert no remove button.

That is roughly 60–80 lines in a file whose render harness, query-client wrapper and record fixture
already exist. Under an hour. It closes the only genuinely-new-and-untested surface on the branch.

### Other observations (non-blocking)

- **Handler preamble duplication** — the two new handlers repeat the
  `GetByID` → `vehicleAccessor.GetByID` → `RequireSameFleet` → `RequireWrite` preamble verbatim
  (`resource.go:344-362` and `:405-421`), matching the three pre-existing item routes in the same
  file. Consistent with the file; a five-route helper extraction is a separate refactor.
- **`dedupeLiveDocuments` runs on every startup** (`entity.go:82`) as a full-table `NOT IN` subquery
  update. Harmless at current scale; worth remembering if the table grows.
- **`MIN(id)` over UUID strings** (`entity.go:87`) picks the lexicographically lowest row, which is
  arbitrary but deterministic — fine for dedupe, but it is not "the oldest row" despite the comment
  reading that way.
- **`plan.md` checkboxes are all unchecked** despite the work being complete.

### Overall Assessment

- **Plan Adherence:** FULL — 8/8 tasks implemented, all deviations documented and approved.
- **PRD Satisfaction:** MOSTLY_COMPLETE — 17/19 functional criteria covered by automated tests.
- **Recommendation:** NEEDS_REVIEW — mergeable on substance; add the drawer removal test file first.

### Action Items

1. Add `VehicleRecordDrawer` integration tests for the attachment-removal path (5 cases above).
   Extend the service mock at `VehicleRecordDrawer.test.tsx:13`.
2. Re-run `make lint-check` with a `golangci-lint` built against go1.27 to confirm the Go lint half
   is clean. Not a code defect, but the branch has not actually been lint-verified.
3. Optional: check off the completed steps in `plan.md`.

---

## Frontend Guidelines

- **Audit Scope:** the 11 changed `apps/web/src/**` TS/TSX files on `task-027-record-attachment-management` (`4f2fa29..6afeb8e`)
- **Guidelines Source:** frontend-dev-guidelines skill (SKILL.md + all 12 `resources/*.md`)
- **Date:** 2026-08-25
- **Build:** PASS (not re-run — `make ci` reported green on this branch by the invoking session; no doubt raised by reading the code warranted a re-run)
- **Tests:** PASS (same provenance)
- **Overall:** NEEDS-WORK

### File Inventory

| File | Class |
|---|---|
| `apps/web/src/services/api/MaintenanceRecordService.ts` | Service |
| `apps/web/src/lib/hooks/api/maintenance.ts` | Hook |
| `apps/web/src/lib/hooks/api/maintenance.test.tsx` | Hook test (new) |
| `apps/web/src/lib/hooks/usePendingAttachments.ts` | Hook (non-API) |
| `apps/web/src/lib/hooks/usePendingAttachments.test.ts` | Hook test |
| `apps/web/src/components/features/vehicles/maintenance/AttachmentPicker.tsx` | Component |
| `apps/web/src/components/features/vehicles/maintenance/AttachmentPicker.test.tsx` | Component test (new) |
| `apps/web/src/components/features/vehicles/maintenance/MaintenanceRecordForm.tsx` | Component (form) |
| `apps/web/src/components/features/vehicles/maintenance/RecordAttachmentList.tsx` | Component |
| `apps/web/src/components/features/vehicles/maintenance/RecordAttachmentList.test.tsx` | Component test |
| `apps/web/src/components/features/vehicles/detail/VehicleRecordDrawer.tsx` | Component (container) |

No `types/models/*` and no `lib/schemas/*` file changed (`git diff --name-only 4f2fa29..6afeb8e | grep -E 'types/models|lib/schemas'` → empty).

### Anti-Pattern Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-01 | No `any` type | PASS | `grep -nE ': any\|as any'` over all 8 non-test changed files → 0 matches. Observation: the test files use `as never` casts on fixtures (`RecordAttachmentList.test.tsx:83`, `VehicleRecordDrawer.test.tsx:79`) — pre-existing house idiom, not `any`, not a FE-01 hit. |
| FE-02 | No manual class concatenation | PASS | `grep -nE 'className=\{[^}]*(\+\|\$\{)'` over the 8 non-test files → 0 matches. New classNames are literal strings (`RecordAttachmentList.tsx:36`, `VehicleRecordDrawer.tsx:350`). |
| FE-03 | No direct client/service calls in components | **FAIL** | `RecordAttachmentList.tsx:8` imports `mediaService` and `:83` calls `mediaService.getContentBlob(mediaId)` inside the download handler — component reaching two layers down. **Pre-existing**, not introduced here (`git show 4f2fa29:…RecordAttachmentList.tsx` → same import at `:8`, same call at `:56`), but the file is in scope and this branch edited around it. Everything else is clean: `VehicleRecordDrawer.tsx:22-30` imports only hooks; grep of the changed component set for `lib/api/client` → 0 matches. |
| FE-04 | No inline Zod in components | PASS | `grep -nE 'z\.(object\|string)\('` over the changed `.tsx` → 0 matches; `MaintenanceRecordForm.tsx:5` imports `maintenanceRecordSchema` from `lib/schemas/`. |
| FE-05 | No spinners for content loading | PASS | All 5 `animate-spin` hits are action-in-flight: `AttachmentPicker.tsx:114` (upload), `RecordAttachmentList.tsx:105` (download button), `VehicleRecordDrawer.tsx:325,412` and `MaintenanceRecordForm.tsx:241` (submit/delete). Content loading uses `Skeleton` (`RecordAttachmentList.tsx:28`). |
| FE-06 | No hardcoded colors | PASS | `src/test/conventions.test.ts` is not in the diff (`git diff --name-only … \| grep conventions` → empty), so neither the regex at `:113-115` nor the two-line allowlist at `:130-133` was touched, and the suite is green. New colour classes are semantic: `VehicleRecordDrawer.tsx:350` `bg-destructive text-destructive-foreground hover:bg-destructive/90`, `RecordAttachmentList.tsx:57` `text-muted-foreground`. |
| FE-07 | No state mutation | PASS | `grep -nE '\.(push\|splice\|sort)\('` over the 8 non-test files → 0 matches. `usePendingAttachments.ts:67-70` derives via `slice`, not in-place. |
| FE-08 | No default exports | PASS | `grep -n 'export default'` over the changed set → 0 matches. |
| FE-09 | `createErrorFromUnknown` error handling | PASS (with two recorded deviations) | Correct at `VehicleRecordDrawer.tsx:211-213` — `createErrorFromUnknown(err)` from `@myfleet/shared-ts` (`:3`), one argument, fallback via `apiError.message \|\|`. Deviation 1: `maintenance.ts:248-250` catches the best-effort media delete and only `console.warn`s the raw `err` — deliberate and documented (`:225-239`), precedent `useRemoveVehiclePhoto` at `media.ts:344-346` swallows it entirely with no log, so this is strictly an improvement; the user's action did succeed, so no toast is correct. It does not normalize through `createErrorFromUnknown` the way the other non-toast log site does (`lib/config/runtimeConfig.ts:112`). Deviation 2: `VehicleRecordDrawer.tsx:169-171` is a bare `catch {}` that only increments a counter — pre-existing, and the aggregate IS surfaced at `:174-177`, but the per-file server message is discarded. |

### Architecture Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-10 | JSON:API model shape | OUT-OF-SCOPE | `git diff --name-only 4f2fa29..6afeb8e \| grep types/models` → empty. `types/models/maintenanceRecord.ts` unchanged; no new model or write-payload type was needed because the detach route takes no body. |
| FE-11 | Service reaches network only via `apiClient` | PASS | `MaintenanceRecordService.ts:72-75` — `apiClient.request<null>(…, { method: 'DELETE' })`, path built from the inherited `basePath` (`:29`), class extends `BaseService` declaring `resourceType` (`:28`). No `fetch`, no self-constructed client, no token. Doc header updated to list both routes (`:17-18`). |
| FE-12 | Query key factory uses `as const` | PASS | `maintenance.ts:34-40` — every tier carries `as const` and spreads the tier above; unchanged by this branch, and the new hook consumes `lists()`/`detail()` rather than rebuilding literals (`:252-253`). |
| FE-13 | Forms use `react-hook-form` + `zodResolver` | PASS | `MaintenanceRecordForm.tsx:75-76` — `useForm<MaintenanceRecordFormInput>({ resolver: zodResolver(maintenanceRecordSchema) })`. This branch only threaded a prop (`:46-52`, `:68`, `:230`); the resolver wiring is intact. |
| FE-14 | Schema in `lib/schemas/` with inferred type | OUT-OF-SCOPE | `git diff --name-only … \| grep lib/schemas` → empty. No schema file changed. |

### React Query Specifics (`patterns-react-query.md`)

| Item | Status | Evidence |
|---|---|---|
| Invalidate in `onSettled`, not `onSuccess` | PASS | `maintenance.ts:251-255` and the widened append hook at `:212-217`. |
| `void` before `invalidateQueries` | PASS | `maintenance.ts:252-254`. |
| Narrowest key that covers the change | PARTIAL | `lists()` + `detail(id)` are both load-bearing and the append hook's comment says why (`maintenance.ts:213-215`). `vehicleKeys.detail(vehicleId)` (`:254`) is **not** load-bearing — nothing on the fleet-service vehicle resource derives from a record's `documentMediaIds`; contrast `useRemoveVehiclePhoto`, where it genuinely is because removing a primary promotes a successor into `primaryImageMediaId` (`media.ts:352-356`). Harmless (one extra refetch of an already-mounted query) and symmetric with the sibling hook, so I **refute** that it is load-bearing but do not call it a defect. |
| Optimistic updates | OUT-OF-SCOPE | No `onMutate` in the diff. |

### Styling / Accessibility Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-15 | Interactive elements show `cursor-pointer` | PASS | Every new interactive element is a `<Button>` or an `AlertDialog` action: `RecordAttachmentList.tsx:34-42` (`<Button>`), `VehicleRecordDrawer.tsx:348` (`AlertDialogCancel`), `:349-358` (`AlertDialogAction`). `cursor-pointer` is in the `buttonVariants` base string (`components/ui/button.tsx:7`) and both alert primitives compose it (`components/ui/alert-dialog.tsx:85,95`). No hand-rolled clickable `div`/row was added. |
| A11y — icon-only controls have accessible names | PASS | `RecordAttachmentList.tsx:32-42` — `aria-label={label}` on the `Button`, `aria-hidden="true"` on the `Trash2` glyph (`:41`); labels resolve to `Remove {filename}` (`:72`, `:120`) and `Remove attachment` on the unavailable row (`:60`). Asserted by name, not markup, at `RecordAttachmentList.test.tsx:107,119,133`. |
| A11y — no nested interactive elements | PASS | `RecordAttachmentList.tsx:94-121` — the flex wrapper is added only when `removable`, so the download `<Button>` and the remove `<Button>` are siblings, never nested; the non-removable row returns the bare `download` element unchanged (`:114-116`). |
| A11y — destructive action confirmed | PASS | `VehicleRecordDrawer.tsx:334-359` uses `AlertDialog` with `AlertDialogTitle` (`:340`) and `AlertDialogDescription` (`:341-344`), matching the `MemberList.tsx:213-233` shape. |

### Testing Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-16 | Tests exist for changed components | **FAIL** | `VehicleRecordDrawer.tsx` gained ~68 lines including a new confirmation dialog (`:334-359`), a new mutation handler (`:204-216`), and two new props passed down (`:273`, `:307-311`). `VehicleRecordDrawer.test.tsx` is **not in the diff** — its last `it(` is at `:199` and none of the seven cases touches attachments. `AttachmentPicker.test.tsx` (new, 78 lines) and `RecordAttachmentList.test.tsx:79-138` cover their own components properly; `maintenance.test.tsx` (new, 127 lines) covers the hook's ordering, best-effort semantics and invalidation set. The gap is exactly the container that wires them together. |
| FE-17 | `vi.mock` call sites updated when a service changes | **FAIL** | `MaintenanceRecordService` gained `removeDocumentMedia` (`:72`). Of its two stubs, `maintenance.test.tsx:15-20` was written fresh and is correct; **`VehicleRecordDrawer.test.tsx:12-14` still stubs `{ get, patch, remove }`** — it carries neither `removeDocumentMedia` nor `appendDocumentMedia`, even though the drawer now reaches both through its hooks. Worse, `MediaService` is not mocked in that file at all (no `vi.mock('…/MediaService')` anywhere in `:12-23`), yet `maintenance.ts:5` pulls `mediaService` into the module graph the drawer imports. Nothing fails today only because no drawer test exercises removal — which is the same silent-stub failure mode the guideline warns about (`SKILL.md:84`, `testing-guide.md` "Mocking"). |

### Findings

#### Important — copied idiom whose invariant was broken (new in this branch)

`VehicleRecordDrawer.tsx:204-216` calls `setPendingRemoval(null)` at `:207`, **before** awaiting the mutation. That closes the `AlertDialog` immediately (its `open` is `pendingRemoval !== null`, `:335`). Consequences:

- `e.preventDefault()` at `:353` is dead. In the `MemberList` precedent it is load-bearing, and the comment there says so verbatim: *"Keep the dialog mounted while the request is in flight so a failure can be retried from it"* (`MemberList.tsx:224-226`). Here the handler defeats it one line later.
- `disabled={removeDocument.isPending}` on both footer buttons (`:348`, `:351`) can never evaluate to `true` while the dialog is visible — dead code.
- The user gets no in-flight feedback and, on failure (`:211-213`), lands on a closed dialog with a toast and no retry affordance.

Fix is one line: move `setPendingRemoval(null)` into the success path after `mutateAsync` resolves (and keep the dialog open on error), matching `confirmRemove` in `MemberList.tsx`. Non-blocking on its own, but it is invisible to per-task review and untested.

#### Important — confirmation dialog is a child of the Sheet, against the documented precedent

`VehicleRecordDrawer.tsx:334-359` renders the `AlertDialog` inside the view-mode branch of `renderContent()`, which is rendered inside `<SheetContent>` (`:436-439`). `PhotoGalleryDialog.tsx:172-175` carries an explicit comment for the opposite arrangement: *"Sibling of the gallery, not a child: nesting it inside DialogContent would unmount the confirmation along with the dialog the moment the alert took focus away."* The drawer's `<Sheet open>` is unconditional so the specific unmount defect may not reproduce, but stacked Radix modal layers plus `pointer-events: none` on `body` are exactly the class of bug jsdom cannot see and no test here exercises. Recommend hoisting it to a sibling of `<Sheet>`, as `PhotoGalleryDialog` does, or verifying it by hand in a browser.

#### Important — FE-16/FE-17 (see rows above)

The attach half is untested at **every** seam, not just the drawer: `MaintenanceRecordForm.test.tsx` has four cases (`:53,65,82,98`) and none asserts that picked files reach `onSubmit`'s `documentMediaIds`. So no test anywhere asserts a picked file reaches `appendDocumentMedia`.

#### Minor / observations

- `maintenance.ts:254` — `vehicleKeys.detail(vehicleId)` invalidation is not load-bearing (see React Query table). Keep for symmetry or drop; either is defensible.
- `maintenance.ts:249` — consider `createErrorFromUnknown(err).message` in the `console.warn`, matching `runtimeConfig.ts:112`.
- `RecordAttachmentList.tsx:36` — the remove control is `h-6 w-6`; consistent with the existing picker control (`AttachmentPicker.tsx:123`), so no inconsistency, but both are a 24px touch target.
- `AttachmentPicker.tsx:47` — `remaining` counts `failed` pending items against capacity. A user with two failed uploads loses two slots until they clear them. Cosmetic.
- `existingCount` staleness (design D-4) is correctly scoped: the server cap stays authoritative (`AttachmentPicker.tsx:41-43`, `usePendingAttachments.ts:41-46`), so the prop-not-query choice is sound.
- FE-03's pre-existing hit at `RecordAttachmentList.tsx:8,83` predates this branch. Recommend a `useDownloadMediaObject` hook as separate cleanup; do not gate this branch on it.

### Summary

#### Blocking (must fix)
- **FE-17** — `VehicleRecordDrawer.test.tsx:12-14`: add `removeDocumentMedia` and `appendDocumentMedia` to the `MaintenanceRecordService` stub and add a `vi.mock` for `MediaService`. Required regardless of whether integration tests are added.
- **FE-16** — `VehicleRecordDrawer.tsx` changed substantively with no test added.

#### Non-Blocking (should fix)
- **FE-03** — `RecordAttachmentList.tsx:8,83` direct `mediaService` call (pre-existing).
- `VehicleRecordDrawer.tsx:207` — premature `setPendingRemoval(null)` makes `:353` and `:348,351` dead code and removes the retry path.
- `VehicleRecordDrawer.tsx:334` — `AlertDialog` nested inside `SheetContent`, against `PhotoGalleryDialog.tsx:172-175`.
- **FE-09** deviations at `maintenance.ts:248-250` and `VehicleRecordDrawer.tsx:169-171`.
- `maintenance.ts:254` — non-load-bearing `vehicleKeys.detail` invalidation.

---

## Backend Guidelines

- **Reviewer:** backend-guidelines-reviewer (DOM-* / SUB-* / SEC-*)
- **Scope:** `apps/fleet-service/internal/maintenancerecord` (the only Go package changed on
  `4f2fa29..6afeb8e`); `apps/fleet-service/internal/mediaclient` and `internal/authz` read as
  collaborators, unchanged on this branch.
- **Guidelines source:** `.claude/skills/backend-dev-guidelines/resources/` (all 9 files)
- **Date:** 2026-08-25
- **Build:** PASS — `make build` exit 0
- **Tests:** PASS — `make test` exit 0, 41 packages ok / 0 failed
- **Overall:** PASS (0 FAIL checks; 4 non-blocking observations)

### Build & Test Results

```
$ make build
go build github.com/jtumidanski/myfleet/...
[exit 0]

$ make test
ok  github.com/jtumidanski/myfleet/apps/fleet-service/internal/maintenancerecord   (cached)
... 41 packages ok, 2 [no test files], 0 FAIL
[exit 0]

$ make vet
go vet github.com/jtumidanski/myfleet/...
[exit 0]
```

`make lint-check` fails, but not for a code reason:
`panic: file requires newer Go version go1.27 (application built with go1.26)` — the installed
`golangci-lint` crashes in `go/types` while loading packages, and it fails all six Go targets
including modules this branch does not touch. Environment, not a branch defect. The Phase 1
objective gate (`make build`, `make test`) is the pass criterion and it is green.

### Phase 2 — Package Classification

| Package | Classification | Evidence |
|---------|---------------|----------|
| `maintenancerecord` | **Domain** — full DOM checklist applies | `model.go:22` declares `Model`; `entity.go:10,31` declare `Entity` + `DocumentEntity` |

No sub-domain (`resource.go` without `model.go`) package changed on this branch.

### Domain Checklist Results

#### maintenancerecord

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| DOM-01 | `builder.go` exists, `Build()` shape correct | PASS | `builder.go:15` `NewBuilder()`, `:19-33` fluent setters, `:36-41` `Build() (Model, error)` delegating to `Validate` — and `model.go:91-101` `Validate` genuinely checks three invariants (`vehicleID`/`categoryID`/`performedAt`), the description rune cap and `MaxDocuments`. This is the `(Model, error)` form and it validates, so it is not the DOM-01 FAIL case. |
| DOM-02 | `ToEntity()` on Model | PASS | `entity.go:118` `func (m Model) ToEntity() Entity` |
| DOM-03 | `Make(Entity)` with no error return | PASS | `entity.go:94` `func Make(e Entity, docs []DocumentEntity) Model`. The extra child-entity parameter is the documented single exception across 17 domains — `ai-guidance.md` Commonly Missed Items names this exact function. Returns `Model` alone, never `(Model, error)`. |
| DOM-04 | `Transform` in `rest.go` | PASS | `rest.go:26` `func Transform(m Model) server.Resource` |
| DOM-05 | `TransformSlice` used by list handlers | PASS | `rest.go:47` declares it; `resource.go:102` `Data: TransformSlice(ms)` in the only list handler. Zero inline `Transform` loops in `resource.go`. |
| DOM-06 | Processor takes `logrus.FieldLogger` | PASS | `processor.go:19` `func NewProcessor(log logrus.FieldLogger, p Provider, a Administrator) *Processor`; field at `processor.go:14`. No `*logrus.Logger` anywhere in the package. |
| DOM-07 | Logger threaded, not fetched | PASS | `resource.go:49-56` — `InitializeRoutes(log logrus.FieldLogger, ...)` passes `log` straight into `NewProcessor` at `:56`. `grep -rn StandardLogger apps/fleet-service/internal/maintenancerecord` → 0 matches across 13 files. Both new handlers use the closed-over `log` (`resource.go:380,388`). |
| DOM-08 | Bodied routes use `RegisterInputHandler` | PASS | All 7 routes enumerated (`resource.go:59,108,204,225,304,340,403`). The 3 bodied ones are wrapped: `:108` create, `:225` patch, and the new `:340` `POST /maintenance-records/{id}/document-media`. The 4 body-less ones (`:59`, `:204` GET; `:304`, `:403` DELETE) are correctly plain — the new detach route at `:403` takes its only input from the URL path (`{id}`, `{mediaId}`), which is exactly the `POST /vehicles/{id}/restore` carve-out. |
| DOM-09 | Errors via `server.WriteError` | PASS | 40 `server.WriteError(w, err)` calls in `resource.go`; the two new handlers account for 12 of them (`:348,354,358,362,369,381,389` attach; `:409,414,418,422,425` detach). `grep -n "http.Error(" *.go` → 0 matches across the package. The only two `w.WriteHeader` calls (`:329`, `:428`) are `http.StatusNoContent` success paths on DELETE, not error ladders. |
| DOM-10 | Provider contract | PASS | `provider.go:14-27` `Provider` interface, `:29` `dbProvider{db}`, `:32` `NewProvider(db *gorm.DB) Provider`. Single-record fetch `GetByID` translates the sentinel at `:37-39` (`errors.Is(err, gorm.ErrRecordNotFound)` → package-local `ErrNotFound` declared `:11`). Unchanged on this branch, verified in scope because the new processor methods sit alongside it. |
| DOM-11 | No `os.Getenv()` in handlers | PASS | `grep -n "os.Getenv" resource.go` → 0 matches. `os` is not imported by `resource.go` (`:3-18`). The one env read is centralised at `cmd/main.go:209` (`MEDIA_INTERNAL_URL`) and injected as the `docs DocumentValidator` constructor parameter (`resource.go:54`). |
| DOM-12 | No cross-domain logic in handlers | PASS | Both new handlers call only `proc.*` for record state (`resource.go:346,386,406,424`). The cross-domain reads they do make go through injected *processor-shaped* interfaces, not another domain's provider: `VehicleAccessor` (`resource.go:22-24`, satisfied by `*vehicle.Processor`) and `DocumentValidator` (`:41-43`, satisfied by `*mediaclient.Client`, an HTTP client — cross-service data over the API, never a cross-service DB read). |
| DOM-13 | Handlers don't call providers directly | PASS | `grep -n "NewProvider(\|NewAdministrator(" resource.go` → one match, `resource.go:56`, inside `InitializeRoutes` and outside `return func(r chi.Router)`, feeding `NewProcessor`. No handler body references `Provider` or `Administrator`. |
| DOM-14 | No direct entity creation in handlers | PASS | `grep -n "db.Create\|db.Save\|db.Delete" resource.go processor.go` → 0 matches. Every write on this branch lands in `administrator.go` (`:170` `tx.Create(&d)`, `:212` `Update("deleted_at", ...)`). |
| DOM-15 | `administrator.go` exists for writes | PASS | `administrator.go:16-32` `Administrator` interface, with the two new methods declared at `:29` `AttachDocument(recordID, mediaID string) (Model, error)` and `:31` `DetachDocument(recordID, mediaID string) error`; `dbAdministrator` implementations at `:136` and `:209`; called from `processor.go:76` and `:90`. Correct layer flow resource → processor → administrator. |
| DOM-16 | Domain returns the right sentinel | PASS | Attach: `administrator.go:146` returns package-local `ErrNotFound`, translated to `server.ErrNotFound` at `processor.go:78-80`; the cap returns `server.ErrValidation` directly at `administrator.go:168` and is passed through unchanged at `processor.go:81-83` (comment names the 422 mapping). Detach: `administrator.go:217` `ErrNotFound` → `processor.go:91-93` `server.ErrNotFound`. Nothing raw reaches `WriteError`. Verified end-to-end at the HTTP layer: `resource_test.go` asserts 201/422/403/404/204 for these paths. |
| DOM-17 | Resource type is a literal | PASS | `rest.go:28` `Type: "maintenanceRecords"` — a string literal, with `ID: m.ID()` at `:29`. Not computed or reflected. The new attach route reuses this same `Transform` (`resource.go:392`), so it does not introduce a second type string. |
| DOM-18 | Input structs narrow and unexported | PASS (with note) | The attach body struct is `struct{ MediaID string \`json:"mediaId"\` }` (`resource.go:340-342`) — one field, unexported, flat, with no `Data`/`Type`/`Attributes` wrapper (`RegisterInputHandler` strips the envelope at `packages/shared-go/server/handler.go:47-60`) and no reuse of the read model `Attributes` (`rest.go:8`). **Note:** it is an inline anonymous struct at the call site rather than a named struct in `rest.go`. That is the tree's dominant convention — `grep -rn "RegisterInputHandler(func(w http.ResponseWriter, req \*http.Request, attrs struct {" apps/` returns 18 sites, and only `vehicle/rest.go` uses the named `createAttributes`/`patchAttributes` form. The two pre-existing bodied routes in this same file (`resource.go:108`, `:225`) are inline too. Not a defect; recorded so the divergence from the doc's example is on the record. |
| DOM-19 | Tests cover the domain's logic layers | PASS | All four `testing-guide.md` layers have tests for the new code. Builder invariants: `model_test.go` + `builder.go:36` exercised via `insertRecord` (`provider_test.go:66-80`). Processor: `processor_test.go:26,36,49,61,69` — five cases covering both sentinel translations, the pass-through of `server.ErrValidation`, and the returned model. Administrator/provider error paths: `administrator_test.go:90-257` — nine cases including cap-at-11, idempotent re-attach, re-attach on a full record, soft-deleted parent, cross-record detach isolation. REST status mapping: `resource_test.go:229-518` — twelve cases pinning 201/422/403/404/204 plus the empty-`mediaId`-makes-no-ownership-call assertion. Migration: `entity_test.go:25,45,69,107`. No table is used, consistent with the local idiom (one named `TestX_scenario` per case); no case repeats the same assertions inline three or more times, so this is not the DOM-19 FAIL. |

### Sub-Domain Checklist Results

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| SUB-01..04 | — | OUT-OF-SCOPE | `ls apps/fleet-service/internal/maintenancerecord/` shows `model.go` present, so the package is a Domain, not a sub-domain. The tree's only genuine sub-domain packages are `notification-service/internal/admin` and `media-service/internal/admin`, neither of which is in this branch's diff (`git diff --stat 4f2fa29 6afeb8e` lists 9 Go files, all under `maintenancerecord/`). |

### Security Review

fleet-service does not issue, parse or revoke tokens — it consumes an `auth.Identity` already
resolved by shared middleware — so SEC-01..03 have no subject in this package. They are recorded
as OUT-OF-SCOPE with their commands, not as silent passes.

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| SEC-01 | JWT validation uses verified parsing | OUT-OF-SCOPE | `grep -niE "ParseUnverified|jwt\." apps/fleet-service/internal/maintenancerecord/*.go` → 0 matches across 13 files. No token parsing in scope; the package reads `auth.IdentityFromContext(req.Context())` (`resource.go:344,404`). |
| SEC-02 | Revocation checks validated tokens | OUT-OF-SCOPE | No logout/revocation handler exists in the changed package; the 7 routes are enumerated under DOM-08. |
| SEC-03 | No open redirect | OUT-OF-SCOPE | No callback/redirect handler in scope. `grep -n "http.Redirect\|Location" resource.go` → 0 matches. |
| SEC-04 | Secrets not hardcoded | PASS | `grep -niE "secret *=\|password *=\|apikey" apps/fleet-service/internal/maintenancerecord/*.go` → 0 matches across 13 files. The one external URL is `config.Get("MEDIA_INTERNAL_URL", "http://media-service:8080")` at `cmd/main.go:209` — a service DNS name, not a credential. |
| SEC-A | Attach proves media ownership before any write | PASS | `resource.go:378-384` — `docs.ValidateOwnership(req.Context(), identity.ActiveFleetID, []string{attrs.MediaID})` runs before `proc.AttachDocument` at `:386`, so a rejection has nothing to roll back. `identity.ActiveFleetID` is safe to use as the fleet scope because `authz.RequireSameFleet` (`resource.go:356`, `authz/authz.go:12-17`) has already proven `identity.ActiveFleetID == v.FleetID()`. Pinned by `resource_test.go:270-292` which asserts 422 *and* that the row was not written. The nil-`docs` branch is test-only: `cmd/main.go:209,284` always wires a non-nil `*mediaclient.Client`, and `config.Get` supplies a non-empty default so the client can never be constructed with a blank base URL. |
| SEC-B | Ownership check fails closed | PASS | `mediaclient/client.go:77-79` — a transport error or non-200 propagates unchanged (`getJSON` returns `fmt.Errorf(...status %d)` at `:104`), so `server.StatusFor` maps it to 500 and no document row is written. A short result set is `server.ErrValidation` (`:87`), deliberately indistinguishable between "does not exist", "deleted" and "another fleet's", so a 422 never confirms a media id exists elsewhere. Pinned at `mediaclient/client_test.go:171-187`. |
| SEC-C | Blank `mediaId` rejected before the cross-service call | PASS | `resource.go:368-371` returns `server.ErrValidation` before line 378, so a blank id never becomes an unbounded `?ids=` on media-service. `resource_test.go:296-309` asserts 422 **and** `len(docs.calls) != 0` → zero ownership calls. |
| SEC-D | Cross-fleet is 404, not 403 | PASS (established behavior, not flagged) | `authz/authz.go:12-17` returns `server.ErrNotFound`; both new routes call it (`resource.go:356`, `:416`) before `RequireWrite`. `resource_test.go:363-370` and `:496-505` pin 404 for a `f2` identity. Matches the three pre-existing item routes. Per the task brief the PRD's 403 tables are wrong about the codebase; not recorded as a defect. |
| SEC-E | Viewer cannot attach or detach | PASS | `authz.RequireWrite` (`authz/authz.go:20-25`, member\|owner only) at `resource.go:360` and `:420`. `resource_test.go:345-360` additionally asserts a forbidden attach made **zero** ownership calls and mutated no row; `:479-492` asserts a forbidden detach left the document in place. |
| SEC-F | Detach cannot reach across records | PASS | `administrator.go:211` scopes the UPDATE with `maintenance_record_id = ? AND media_id = ? AND deleted_at IS NULL`. `administrator_test.go:240-257` seeds two records and proves detaching from the wrong one is `ErrNotFound` and leaves the other's row live. `RowsAffected == 0 → ErrNotFound` (`:216-218`) collapses "never attached" and "already detached", so the response cannot confirm a media id exists elsewhere (`resource_test.go:449-467`). |
| SEC-G | Detached media still covered by purge | PASS | Assessed because "detach removes the reference only" could otherwise strand media beyond a fleet purge's reach. It does not: `admin/targets.go:117-127` `vehicleMediaIDs` unions `fleet.maintenance_record_documents` **without** a `deleted_at IS NULL` filter, so a soft-deleted (detached) row still names its media id for deletion; `admin/manifest.go:141-143` hard-deletes the table's rows regardless of `deleted_at`. The residual exposure is bounded and same-fleet only — see Observation 4. |

### Locking Review (design D-7 — the one piece with no automated test)

Read line by line at `administrator.go:136-194`, since the brief flags it as the highest-risk code
on the branch.

- **The lock is correctly placed.** `administrator.go:139-149` takes `SELECT ... FOR UPDATE` on the
  **parent** `fleet.maintenance_records` row inside `a.db.Transaction` (`:138`), not on the child
  document table. Locking the parent is what serializes attaches to the same record and, as a free
  side effect, excludes a concurrent soft-delete of the record itself (which `SoftDelete` at `:106-108`
  would otherwise interleave with).
- **The cap check is sound specifically because Postgres defaults to READ COMMITTED.** After the
  blocked transaction acquires the lock, the `Count` at `:161-166` takes a **fresh statement
  snapshot** and therefore sees the first transaction's committed insert. It returns 10 and the
  second attach gets `server.ErrValidation`. This correctness is isolation-level-dependent and
  undocumented as such: under REPEATABLE READ or SERIALIZABLE the count would read the
  transaction-start snapshot, miss the committed insert, and the record could reach eleven.
  Nothing in the tree raises the isolation level today, so this is latent, not live — recorded as
  Observation 3.
- **The dialect guard is safe in the failing direction.** `tx.Name() != "sqlite"` (`:140`) means any
  non-sqlite dialect *gets* the lock; only sqlite skips it. `gorm.io/driver/sqlite` reports
  `"sqlite"` (`provider_test.go:8` is the only driver in the package's tests), and production is
  Postgres, so the branch resolves correctly on both sides. The same dialect-branch shape is
  precedented at `media-service/internal/mediavariant/entity.go` `ApplyPartialIndexes`.
- **`First` + `Clauses(clause.Locking)` composes correctly.** GORM emits
  `... ORDER BY id LIMIT 1 FOR UPDATE`, which is valid Postgres. `q` is derived from `tx` per
  statement (`:139`), so no condition leaks into the later `tx.Model(...)` calls at `:155` and `:162`
  — confirmed by the counts asserted in `administrator_test.go:97-108`.
- **Idempotency ordering is deliberate and right.** The already-attached count at `:154-159` runs
  *before* the cap check, so retrying an attach that actually landed on a full record succeeds
  rather than 422-ing. Pinned by `administrator_test.go:152-167`.
- **The partial unique index is the durable backstop.** `entity.go:68-69` makes a second live
  `(maintenance_record_id, media_id)` row impossible even if the application check is bypassed;
  `entity_test.go:25-40` proves the reject, `:45-64` proves the partial predicate allows
  detach-then-reattach (which a plain unique index would block forever).

### Summary

#### Blocking (must fix)

None. Zero DOM-*, SUB-* or SEC-* checks FAIL.

#### Non-Blocking (should fix / verify before deploy)

1. **`entity.go:68-69,83-90` — the Postgres branch of `ApplyPartialIndexes` and the entire
   `dedupeLiveDocuments` statement have zero test coverage, and they run inside `Migration`
   (`entity.go:41-46`), so a defect there fails fleet-service at **startup**, not at a call site.**
   Every test goes through the sqlite branch: `provider_test.go:17-49` creates
   `fleet.maintenance_record_documents` with `id TEXT PRIMARY KEY`, while production is
   `gorm:"type:uuid"` (`entity.go:32`). So `MIN(id)` is only ever exercised over TEXT affinity,
   never over a Postgres `uuid` column, and `entity_test.go:72-73` even seeds non-UUID ids
   (`"00000000-aaaa"`, `"ffffffff-zzzz"`) that a `uuid` column would reject outright.
   `min(uuid)` has existed since PostgreSQL 9.0 and the DDL string reads as valid Postgres, so this
   is a verification gap rather than a known bug — but it is the branch's largest untested surface
   and the precedent it cites (`mediavariant.ApplyPartialIndexes`) has no dedupe UPDATE at all, so
   the novel statement is the unprecedented part. Boot fleet-service once against a real Postgres
   (staging or a throwaway container) before merging.
2. **`administrator.go:209-219` — `DetachDocument` does not participate in the serialization
   `AttachDocument` establishes.** Detach takes no transaction and no lock on the parent record,
   by design (`:202-204`). The consequence is narrow but real: a concurrent attach and detach of
   the *same* media id can interleave so that the attach's idempotency count at `:154-159` sees
   the row live, skips the insert, re-reads at `:182-186` before the detach commits, and returns
   201 with the id present — while the committed end state has it detached. No cap violation and
   no duplicate row (the index at `entity.go:68` still holds); the only damage is a response that
   disagrees with the row, which is the exact failure mode `Update` at `:84-92` was rewritten to
   eliminate. The comment at `:126-129` says "Locking the record serializes attaches to it", which
   is accurate as written — worth making explicit that detach is deliberately outside that
   guarantee.
3. **`administrator.go:161-169` — the cap check's correctness depends on READ COMMITTED and does
   not say so.** See the Locking Review above. One sentence in the doc comment at `:121-129`
   naming the isolation-level assumption would keep a future `SET TRANSACTION ISOLATION LEVEL`
   from silently re-opening the eleven-attachment gap the lock was added to close.
4. **Orphan media on a failed browser-side delete — bounded, worth naming.** Detach removes only
   the reference (`administrator.go:210-212`); the media object is deleted by the browser against
   `DELETE /api/media/{id}` with the user's own JWT. If that second call never fires, the object
   survives in the owning fleet's media library, still readable by same-fleet users through
   `GET /api/media/{id}/content`. This is **not** a cross-tenant exposure — the object never left
   its original `fleet_id` — and it is **not** a purge leak, because `admin/targets.go:117-127`
   resolves document media ids without filtering `deleted_at`, so the soft-deleted row still names
   the object for deletion when the vehicle or fleet is purged (SEC-G). The residual is storage
   retention plus a stale entry in the fleet's own picker. Giving fleet-service delete authority
   would be worse (`resource.go:396-402` is right that it would create an unauthenticated
   destructive endpoint on media-service's `/internal` surface); if this is worth closing, the
   place is a media-service reaper for objects no record references, not this branch.
5. **`administrator.go:160-169` — the `MaxDocuments` cap counts only live rows, so attach/detach
   cycling grows `fleet.maintenance_record_documents` without bound** for any authenticated member.
   Each cycle leaves a permanent soft-deleted row that nothing reaps short of a purge
   (`admin/manifest.go:141-143`). Bounded by request rate and by the row being 4 small columns;
   noted for completeness, not proposed as a fix on this branch.

#### Explicitly not flagged (per reviewer instruction)

- The verbatim `proc.GetByID` → `vehicleAccessor.GetByID` → `RequireSameFleet` → `RequireWrite`
  preamble repeated at `resource.go:346-363` and `:406-423`. Already ruled on; matches the three
  pre-existing item routes at `:204-220`, `:242-259`, `:307-324`. A five-route helper extraction
  is a separate refactor.
- The unvalidated JSON:API `type` in the attach request body. No handler in this tree validates it;
  `resource_test.go:243` sends `"type":"mediaRefs"` and it is ignored, consistent with every other
  route.
- Cross-fleet returning 404 rather than 403 (`authz/authz.go:12-17`). Established, deliberate.
