# Frontend Audit — task-019-vacuous-negative-assertions

- **Audit Scope:** `d3c9eaa..ba04ed5`, 23 changed files under `apps/web` (1 new test-only helper, 1 new helper test, 1 ESLint config, 20 existing test files)
- **Guidelines Source:** `frontend-dev-guidelines` skill (`SKILL.md` + `resources/anti-patterns.md`, `testing-guide.md`)
- **Date:** 2026-08-03
- **Build:** PASS (reported green by dispatching context; not independently re-run per instruction)
- **Tests:** PASS (`make ci` exit 0, all targets incl. `lint-check`; not independently re-run per instruction)
- **Overall:** PASS

## Build & Test Results

`make ci` (lint-check, vet, test, build, fe-test, fe-build) was confirmed green by the
dispatching context and was not re-run. Two claims that `make ci` green
*implies* were verified independently and read-only, because they are the
load-bearing ones for this branch:

1. **The lint rule actually fires.** Piped through `eslint --stdin
   --stdin-filename src/probe.test.ts` (no working-tree mutation):

   ```
   2:1  error  Use expectNoCall(spy) ... no-restricted-syntax   <- expect(spy).not.toHaveBeenCalled()
   3:1  error  Use expectNoCall(spy) ... no-restricted-syntax   <- expect(spy).not.toHaveBeenCalledWith(1)
   4:1  error  toHaveBeenCalledTimes(0) ... no-restricted-syntax <- expect(spy).toHaveBeenCalledTimes(1) NOT flagged
   ✖ 3 problems (3 errors, 0 warnings)
   ```

   All three banned spellings flagged; the positive spelling
   `toHaveBeenCalledTimes(1)` and `expect(spy.mock.calls.length).toBe(0)` are not.

2. **The exemption block is scoped to exactly two files.** Same stdin probe,
   varying only the filename: `src/test/expectNoCall.ts` → 0 errors,
   `src/test/expectNoCall.test.ts` → 0 errors, `src/test/objectUrl.ts` → 1
   error. The flat-config ordering (exemption at `eslint.config.js:72-78`
   *after* the test-files block at `eslint.config.js:40-71`) is therefore
   correct and does not over-exempt the rest of `src/test/**`.

Note also that `apps/web/package.json:9` runs `eslint src --max-warnings 0`,
and ESLint 9 defaults `linterOptions.reportUnusedDisableDirectives` to `"warn"`
(no `linterOptions` override exists in `eslint.config.js`). A green
`lint-check` therefore also proves the two inline
`eslint-disable-next-line no-restricted-syntax` directives are *used* — i.e.
the rule genuinely matches `input.test.tsx:73` and `download.test.ts:50`.

## File Inventory

**Other / test infrastructure (2)**
- `apps/web/src/test/expectNoCall.ts` — new shared test helper (test-only module)
- `apps/web/eslint.config.js` — lint configuration

**Test files (21)**
- `apps/web/src/test/expectNoCall.test.ts` (new)
- `apps/web/src/components/features/activity/ActivityFeed.test.tsx`
- `apps/web/src/components/features/dashboard/useDashboardWidgets.test.ts`
- `apps/web/src/components/features/settings/MemberList.test.tsx`
- `apps/web/src/components/features/vehicles/CategoryCombobox.test.tsx`
- `apps/web/src/components/features/vehicles/VehicleCard.test.tsx`
- `apps/web/src/components/features/vehicles/VehiclePhotoThumbnail.test.tsx`
- `apps/web/src/components/features/vehicles/detail/VehicleRecordsTable.test.tsx`
- `apps/web/src/components/features/vehicles/dialogs/PhotoGalleryDialog.test.tsx`
- `apps/web/src/components/features/vehicles/maintenance/MaintenanceRecordForm.test.tsx`
- `apps/web/src/components/ui/input.test.tsx`
- `apps/web/src/lib/hooks/api/auth.test.ts`
- `apps/web/src/lib/hooks/api/media.test.ts`
- `apps/web/src/lib/hooks/api/members.test.ts`
- `apps/web/src/lib/hooks/api/users.test.ts`
- `apps/web/src/lib/hooks/api/vehicleRecords.test.ts`
- `apps/web/src/lib/hooks/usePendingAttachments.test.ts`
- `apps/web/src/lib/utils/download.test.ts`
- `apps/web/src/pages/LoginPage.test.tsx`
- `apps/web/src/pages/VehiclesPage.test.tsx`
- `apps/web/src/pages/admin/AdminFleetsPage.test.tsx`

**Page / Component / Hook / Service / Schema / Type: zero.** No production
source file is modified in this range (verified: `git diff --name-only
d3c9eaa..ba04ed5 -- apps/web` yields only the 23 files above). Every FE-*
item that judges production code therefore has **no applicable surface** and is
recorded N/A with that reason rather than a fabricated verdict.

## Anti-Pattern Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-01 | No `any` type | PASS | `grep -n ': any\|as any'` over all 23 changed files: zero matches. `expectNoCall.ts:49` uses `args: unknown[]`, not `any[]`; `expectNoCall.ts:34,48` type the spy as `MockInstance` (imported as a type-only import at `expectNoCall.ts:8`). |
| FE-02 | No manual class concatenation | N/A | No `className` is authored anywhere in the diff (`grep -n 'className={"'`: zero matches). No JSX styling surface changed. |
| FE-03 | No direct API client calls in components | N/A → PASS | `grep -n 'lib/api/client'` over changed files: zero matches. No component changed; the added imports are all `test/expectNoCall` (18 files) plus existing service mocks. |
| FE-04 | No inline Zod schemas in components | N/A | `grep -n 'z\.object(\|z\.string('`: zero matches. No form or component changed. |
| FE-05 | No spinners for content loading | N/A | `grep -n 'animate-spin'`: zero matches. No loading-state code changed. |
| FE-06 | No hardcoded colors | N/A | `grep -n 'animate-spin\|bg-white\|bg-gray'`: zero matches. Separately, this rule is enforced repo-wide by a convention test at `apps/web/src/test/conventions.test.ts:113-176`, which this branch does not touch or weaken. |
| FE-07 | No state mutation | PASS | `grep -n '\.push(\|\.splice(\|\.sort('` returns 11 hits, all local test-recorder arrays declared in the same scope, none React state and none followed by a setState: `expectNoCall.test.ts:11,12` (`order`), `download.test.ts:22` (`clicks`), `PhotoGalleryDialog.test.tsx:15,18` (`calls`), `MemberList.test.tsx:290,294` (`order`), `media.test.ts:37,52,61,164` (`calls`, `renders`). |
| FE-08 | No default exports for components | PASS | `grep -n 'export default'` over changed files returns exactly one hit, `eslint.config.js:11` (`export default tseslint.config(`) — a build-config module, not a component. `expectNoCall.ts:21,34,47` are all named exports (`flushPending`, `expectNoCall`, `expectNoCallWith`), matching the sibling helper `src/test/objectUrl.ts`. |
| FE-09 | Error handling with `createErrorFromUnknown` | N/A | `grep -n '\.catch('` over changed files: zero matches. No async error-handling path was added or modified. |

## Architecture Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-10 | JSON:API model shape | N/A | No file under `types/models/` changed. Test fixtures consume existing model types unchanged (e.g. `ActivityFeed.test.tsx:6` imports `ActivityEvent` from `types/models/activity` — import only). |
| FE-11 | Service extends `BaseService` | N/A | No file under `services/api/` changed. |
| FE-12 | Query key factory uses `as const` | N/A | No query-key factory changed. `members.test.ts` and `users.test.ts` assert against the existing factories but do not define or alter them. |
| FE-13 | Forms use `react-hook-form` + `zodResolver` | N/A | No form component changed. `MaintenanceRecordForm.test.tsx:93` is a one-line assertion swap in an existing test. |
| FE-14 | Schema in `lib/schemas/` with inferred type | N/A | No file under `lib/schemas/` changed. |

## Styling Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-15 | Interactive elements show `cursor-pointer` | N/A | No interactive element is authored in the diff. All rendered JSX in the changed test files is pre-existing component usage. |

## Testing Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-16 | Tests exist for changed components | PASS | The one new non-test module, `src/test/expectNoCall.ts`, has a dedicated co-located test at `src/test/expectNoCall.test.ts` covering all three exports. It is a genuine falsifiability test, not a smoke test: `expectNoCall.test.ts:23-31` asserts the bare form passes vacuously (line 28) while `expectNoCall` rejects (line 30), and `expectNoCall.test.ts:56-64` does the same for `expectNoCallWith`. `expectNoCall.test.ts:38-47` pins the label and call-count in the failure message; `expectNoCall.test.ts:9-17` pins the microtask-then-macrotask flush ordering. |
| FE-17 | Mocks updated when services changed | N/A | No service changed, so no mock needed updating. `grep` confirms no `services/api/*` file in the diff. |

## Supplementary review (items the dispatcher asked for judgement on)

These are not FE-* checklist items; they are recorded so the judgement is on
the record.

### `src/test/expectNoCall.ts` as a shared test utility — PASS

Matches the house pattern in `src/test/` exactly. The directory contains one
small module per concern with a co-located test and no barrel file
(`objectUrl.ts` + `objectUrl.test.ts`, `renderWithProviders.tsx`, `setup.ts`);
`expectNoCall.ts` + `expectNoCall.test.ts` is the same shape. All 18 consumers
import it directly by relative path (e.g. `ActivityFeed.test.tsx:5`,
`LoginPage.test.tsx:9`); no barrel was introduced. Named exports only
(`expectNoCall.ts:21,34,47`). The `act`-import constraint is documented with its
reason at `expectNoCall.ts:1-6`, and the fake-timer incompatibility at
`expectNoCall.ts:18-20`.

### The `mockName` side effect — acceptable, with a caveat

`expectNoCall.ts:36` and `:53` call `spy.mockName(label)`, and the name is not
reset by `mockClear`/`mockReset`/`mockRestore` — confirmed by reading
`node_modules/@vitest/spy/dist/index.js:63-86`, where `mockName` assigns a
closure variable `name` that `mockClear` (line 69) and `mockReset` (line 76)
never touch. The label therefore persists on module-level mocks for the rest of
the file, past the `vi.clearAllMocks()` in `afterEach` (e.g.
`VehiclePhotoThumbnail.test.tsx:36`).

Judged acceptable because the effect is monotone-improving and never
contradictory: all 38 call sites pass a label (verified — no single-argument
call site exists), every label accurately names its spy, and no spy is given two
different labels anywhere. The consequence is that an unrelated *later* failure
on the same spy reports `mediaService.getContentBlob` instead of `vi.fn()`,
which is strictly better. The docblock at `expectNoCall.ts:29-33` explains the
purpose but does not mention that the rename persists; see Non-Blocking below.

### The two inline suppressions — PASS

Both carry falsification evidence, not an assertion of correctness, which is the
right standard.

- `input.test.tsx:68-72`: names the specific mutation that turns the line red
  (`forcing isPicker = true in input.tsx`) and the mechanism (a direct DOM call
  from `onClick`, not a promise continuation). Verified against the code under
  test at `input.test.tsx:61-75`.
- `download.test.ts:42-49`: gives two independent reasons — the assertion is an
  *ordering* assertion whose positive half is two lines below
  (`vi.runAllTimers()` at `download.test.ts:52`, then
  `toHaveBeenCalledWith('blob:test-url')` at `:53`), and the file runs under
  `vi.useFakeTimers()` (`download.test.ts:6`), which would make the helper's
  `setTimeout(0)` never fire. Both verified: the fake-timer claim is confirmed
  at `download.test.ts:6`, and no file that imports `expectNoCall` uses fake
  timers (checked across all 18 importers).

Both are `eslint-disable-next-line`, i.e. single-line and non-contagious, rather
than a file-level `eslint-disable`.

### Migration fidelity across the 20 files — PASS

Reviewed all 38 replaced sites in the full diff. Every one is a 1:1 swap of the
banned spelling for the helper; no positive assertion was dropped and no
assertion order changed. Specifically checked:

- **`async` added where needed:** four `it()` callbacks gained `async`
  (`VehicleCard.test.tsx:83`, `VehiclePhotoThumbnail.test.tsx:62`,
  `users.test.ts:76`, `vehicleRecords.test.ts:313`); the other 34 sites were
  already inside `async` callbacks. A missing `async` would be a compile error,
  and `fe-build`/`tsc` is green.
- **`expectNoCallWith`'s "never called at all" hole is covered at both sites.**
  `CategoryCombobox.test.tsx:158` is preceded by the positive
  `await waitFor(() => expect(onChange).toHaveBeenCalledWith('server-assigned-id'))`
  at `:157`. `media.test.ts:220` is preceded by
  `expect(revokeObjectURL).toHaveBeenCalledWith(firstUrl)` and
  `toHaveBeenCalledTimes(1)` at `:218-219`. Both follow the pairing rule the
  helper's own docblock prescribes at `expectNoCall.ts:42-45`.
- **`LoginPage.test.tsx:235-241`** replaces a hand-rolled
  `await act(async () => { await new Promise(r => setTimeout(r, 0)); })` with the
  helper — semantically identical, and the explanatory comment was preserved
  rather than deleted.
- **No site was left behind.** `grep -rn 'not\.toHaveBeenCalled' src` returns
  exactly six hits: the two documented exemptions
  (`input.test.tsx:73`, `download.test.ts:50`), the helper's own two
  implementation lines (`expectNoCall.ts:37,54`), and the helper test's two
  deliberate contrast lines (`expectNoCall.test.ts:28,61`).
  `grep -rn 'toHaveBeenCalledTimes(0)' src` returns zero.

## Summary

### Blocking (must fix)

None. Zero FAIL checks across FE-01..FE-17.

### Non-Blocking (should fix)

- **The lint message does not mention the fake-timer escape hatch**
  (`eslint.config.js:56-59`). The helper is unusable under
  `vi.useFakeTimers()` — documented at `expectNoCall.ts:18-20` — but the
  message a future author actually sees says only "Use expectNoCall(spy)". An
  author adding a negative assertion to a fake-timer file will follow the
  message and get a test that hangs to timeout rather than a clear failure.
  One clause pointing at the `download.test.ts:42-49` precedent would close it.
- **`expectNoCall`'s docblock does not mention that `mockName` persists**
  (`expectNoCall.ts:29-33`). Harmless today (see above), but the persistence is
  non-obvious and survives `vi.clearAllMocks()`.

### Informational (no action expected)

- **`flushPending` is exported but has no consumer outside its own test.**
  `grep -rn flushPending src` hits only `expectNoCall.ts:21,35,52` and
  `expectNoCall.test.ts:2,8,14`. Defensible as the documented primitive for
  "flush, then assert something other than a call count", and it is tested
  (`expectNoCall.test.ts:9-17`), so this is an observation rather than a defect.
- **Rule coverage gaps are known and disclosed.** `packages/shared-ts` and
  `packages/ui-components` have no ESLint config at all (only
  `apps/web/package.json:9` defines a `lint` script), so the rule cannot reach
  them; and `expect(spy.mock.calls.length).toBe(0)` is not matched by either
  selector. Both are already written up under "Residual gaps" in
  `probe-results.md`, and neither spelling occurs anywhere in the tree today
  (verified by grep across `packages/`). Not undisclosed debt.
- **The `testing-guide.md` resource is stale relative to the repo.** It
  documents Jest 30 with a `jest.config.js` (`testing-guide.md:5-17`), while the
  code under audit is Vitest throughout (`expectNoCall.ts:7`, and every changed
  test imports from `vitest`). This is a guidelines-doc issue, not a defect in
  this branch, but it means the FE-16/FE-17 criteria are written against a test
  runner the project no longer uses.

### Verdict

**Merge.** The branch does exactly what it claims, the central claim
(that the rule fires and the exemption is narrowly scoped) was verified
independently rather than taken on report, and the two suppressions carry
falsification evidence rather than assertions. The two non-blocking items are
comment-only improvements and do not warrant holding the merge.
