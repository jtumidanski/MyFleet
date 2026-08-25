# Task 025 — Implementation Context

Companion to `plan.md`. Everything an executor needs that isn't a numbered step.

---

## Environment

- **Worktree:** `/home/tumidanski/source/MyFleet/.worktrees/task-025-record-receipt-indicator`, branch `task-025-record-receipt-indicator`. Never edit the main checkout.
- **Node:** not always on `PATH`. If `npm` is missing:
  ```sh
  export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
  ```
- **Commands:**
  | Purpose | Command |
  |---|---|
  | One test file | `npm run -w apps/web test -- <path>` |
  | All FE tests (3 workspaces) | `make fe-test` |
  | Type-check + build | `make fe-build` (`tsc -b && vite build`) |
  | Lint (zero-warning gate) | `npm run -w apps/web lint` |

  This branch touches no Go, so `make vet` / `make test` / `make build` and the `kustomize` manifest renders are not required.

---

## Key Files

| File | Role in this task |
|---|---|
| `apps/web/src/lib/vehicleRecords.ts` | Pure-data module: `VehicleRecordRow`, `mergeVehicleRecords`, `filterVehicleRecords`. Gains `documentCount?: number`. **No JSX, no `lucide-react` here.** |
| `apps/web/src/lib/hooks/api/vehicleRecords.ts` | `useVehicleRecords` — composes the three infinite-query source hooks into one feed. The maintenance branch of its `sources` `useMemo` gains one line. |
| `apps/web/src/components/features/vehicles/detail/VehicleRecordsTable.tsx` | The unified feed table. All rendering changes land here. |
| `apps/web/src/lib/hooks/api/vehicleRecords.test.ts` | Adapter tests. Source hooks are `vi.mock`ed; rich fixture builders already exist (`maintenanceRecord`, `fuelLog`, `mileageRecord`, `stub`, `setupSources`, `settledCategories`/`NO_CATEGORIES`). **Reuse them; add no new imports.** |
| `apps/web/src/components/features/vehicles/detail/VehicleRecordsTable.test.tsx` | Table tests. Renders the component directly with a plain `rows` fixture — no `QueryClientProvider` needed. |
| `apps/web/src/test/expectNoCall.ts` | `expectNoCall(spy, label)` — **async**, flushes microtasks + one macrotask before asserting. Mandatory for negative call assertions (see Gotchas). |

### Read-only references (do not modify)

| File | Why it matters |
|---|---|
| `apps/web/src/types/models/maintenanceRecord.ts:23` | `documentMediaIds?: string[]` — already typed, already on the wire. |
| `apps/web/src/components/features/vehicles/maintenance/RecordAttachmentList.tsx:75-79, 84-87` | The `h-4 w-4` + `aria-hidden` icon convention, and the docstring recording the "25 × N metadata requests" constraint this task must not undo. |
| `apps/web/src/components/features/vehicles/dialogs/PhotoGalleryDialog.tsx:138-141, 155-156` | The `aria-hidden` icon + single `sr-only` string pattern the indicator copies. |
| `apps/fleet-service/internal/maintenancerecord/rest.go:22, 41, 47-53` | `documentMediaIds,omitempty`; `TransformSlice` reuses `Transform`, so list and single-record shapes are identical. |
| `apps/fleet-service/internal/maintenancerecord/provider.go:49-95` | `ListByVehicle` already batches the document lookup for a whole page (one `WHERE ... IN (?)`), deliberately replacing an N+1. |
| `apps/fleet-service/internal/maintenancerecord/model.go:19` | `MaxDocuments = 10` — the server-side cap that makes "no 9+ truncation needed" (FR-UI-6) safe. |

---

## Decisions Already Made (don't relitigate)

| Decision | Rationale | Source |
|---|---|---|
| Store a **count**, not `hasDocuments: boolean` | FR-UI-2 requires rendering N; a boolean forces a second field. `> 0` recovers the boolean. | design D1 |
| Store a **count**, not the `documentMediaIds` array | The row never uses the ids; carrying them pollutes a normalized type and invites the per-attachment fan-out the NFR forbids. | design D1 |
| `documentCount?` **optional**, adapter writes explicit `0` | Fuel/mileage adapters stay untouched (`undefined`); maintenance asserts `0` so "server omitted the key" is pinnable in a test rather than ambiguous. | design D1/D2 |
| **Sixth column, appended after Cost** — not inline in the Item cell | Item is `max-w-[240px] truncate`; an inline sibling risks the indicator silently truncating away. A column is also scannable — the icon lands at the same x on every row. Last, not between Item and Odometer, so the two numeric columns stay adjacent. | design §3 (resolves PRD OQ 1) |
| Indicator is **module-local**, not exported or promoted to `components/ui` | Mirrors the existing `TypeBadge` precedent in the same file. Nothing else renders this. | design D3 |
| Both icon **and** visible digit are `aria-hidden`; one `sr-only` string | Otherwise a screen reader says "3, 3 attachments". | design D3 |
| Pluralization is a **ternary**, not `Intl.PluralRules` | No i18n layer in the app; one string. | design D3 |
| `COLUMN_COUNT` constant instead of bumping two `colSpan={5}` literals | Reduces two silent drift sites to one named one; three lines, confined to the file already being changed. | design D6 (addresses FR-TBL-2) |
| **No `row.kind` branch anywhere** | Makes "a modification row behaves identically to a maintenance row" true by construction, not by a second code path. | design D4 (FR-UI-1) |
| **Zero backend changes** | The data is already served. Stated as a requirement, not an observation. | PRD §5/§6/§7 |

---

## Gotchas

1. **`expect(spy).not.toHaveBeenCalled()` is an eslint error.** `apps/web/eslint.config.js:56-77` bans it (and `toHaveBeenCalledTimes(0)`) via `no-restricted-syntax`, because a bare negative call assertion runs *before* React Query's promise-continuation dispatch and can pass vacuously (issue #22, task-019). Use `await expectNoCall(spy, 'label')` from `src/test/expectNoCall`. It is incompatible with `vi.useFakeTimers()` — not a concern here, nothing in these tests uses fake timers.

2. **`react-hooks/exhaustive-deps` is an *error*, not a warning.** The `sources` `useMemo` dependency array must stay exactly as it is: `maintenance.data` already covers the new `documentMediaIds` read. Adding anything will not help and may churn identities — the memoized-identity tests in `vehicleRecords.test.ts` exist precisely to catch that.

3. **Mock `lib/hooks/api/media` partially, not wholesale.** It exports ~18 symbols. Use the `importOriginal` spread form shown in the plan and override only `useMediaObject`. A bare factory would break the moment anything transitively imports another export.

4. **The no-fan-out test is meaningful even though the table never imports the media hook.** `vi.mock` registers the module in the test's registry, so if someone later adds a `useMediaObject` call inside the table, the spy sees it and the test fails. That is the guard.

5. **`getAllByRole` throws on zero matches.** Use `queryAllByRole` for the "no nested button" assertion.

6. **Adding a fourth default row breaks the existing footer test.** `showing 3 of 41` becomes `showing 4 of 41`. The plan calls this out explicitly; don't skip it.

7. **The `visible.map` callback must become a block body.** It currently uses an implicit-return arrow. The `documentCount` narrowing needs a `const` before the `return (`.

8. **`vitest run` does not type-check.** A `documentCount` typo can pass tests and fail `make fe-build`. Task 3 Step 2 is not optional.

---

## Dependencies

- **On other tasks:** none. This is self-contained and frontend-only.
- **Between plan tasks:** Task 2 consumes `VehicleRecordRow.documentCount` from Task 1; run them in order. Task 3 gates both.
- **New packages:** none. `lucide-react` (`^1.0.0`) is already a dependency and `Paperclip` ships with it. `package.json` and `package-lock.json` must not change.

---

## Verification Checklist (map to PRD §10)

- [ ] Maintenance record with N ≥ 1 documents shows `Paperclip` + N — Task 2 test "announces the attachment count".
- [ ] Zero attachments → empty cell, no icon, no "0" — Task 2 test "renders nothing for a record with zero attachments".
- [ ] Modification behaves identically to maintenance — the `documentCount: 1` mod row in the fixture, plus the absence of any `kind` branch.
- [ ] Fuel and mileage → empty cell — Task 2 test "renders nothing for fuel and mileage rows".
- [ ] Not focusable, not a button, click opens the drawer — Task 2 test "is not interactive and lets clicks fall through".
- [ ] Correct singular/plural announcement — Task 2 tests "announces…" and "uses the singular form".
- [ ] Skeleton and empty state span the full width — Task 2 `colSpan` tests, asserted against the real `<thead>` cell count rather than a hardcoded 6 alone.
- [ ] No extra network request — Task 2 test "fetches no media metadata".
- [ ] Adapter populates from `documentMediaIds`, absent key means zero — Task 1's four tests.
- [ ] `make fe-test` and `make fe-build` pass — Task 3.

---

## Follow-ups to file separately (do NOT implement here)

1. `POST /api/fleet/maintenance-records/{id}/document-media` is called by the frontend (`MaintenanceRecordService.ts:49-61`, `lib/hooks/api/maintenance.ts:206-216`) but is registered nowhere in `fleet-service` — `InitializeRoutes` in `resource.go` registers only list/create/get/patch/delete. Any call 404s. Either register the route or delete the dead client.
2. `useAppendMaintenanceRecordDocument` (`maintenance.ts:204-216`) invalidates `detail(id)` and `vehicleKeys.detail(vehicleId)` but **not** `maintenanceRecordKeys.lists()`, so an appended document would leave the new row count stale. Fix alongside (1) — fixing the cache key for a route that doesn't exist would be untestable and would imply the flow works.

The **create** path needs nothing: `useCreateMaintenanceRecord`'s `onSettled` already invalidates `lists()` (`maintenance.ts:172`), so a record logged with receipts refetches the feed with its count correct. Verified, not assumed (design §6, resolving PRD OQ 3).
