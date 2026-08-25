# Plan Audit — task-028-required-field-indicators

**Plan Path:** docs/tasks/task-028-required-field-indicators/plan.md
**Audit Date:** 2026-08-25
**Branch:** task-028-required-field-indicators
**Base Branch:** main

## Executive Summary

All 11 tasks in the plan were implemented, each as its own commit, matching the plan's stated files and behavior. The two controller-approved deviations from the plan's literal text (block comments in `MaintenanceScheduleForm.tsx`, conditional `RequiredLegend` in `VehicleForm.tsx`) were both applied correctly, with the latter landing as a dedicated follow-up fix commit (`3d2e41d`) rather than being silently baked in. `COST_REQUIREMENT`'s wording ("Enter price per gallon or total cost (or both).") differs from the frozen Zod schema's message ("Provide price per gallon or total cost (or both)") as expected — the schema itself was not touched (`git diff --stat main...HEAD -- apps/web/src/lib/schemas/` is empty). Independent re-runs of `make fe-test` (101 files / 828 tests in apps/web, 9 in shared-ts, 10 in ui-components — all passing) and `make fe-build` both succeeded on the current HEAD. The one outstanding item is Task 11 Step 5, the human visual/theme check of the marker in the Add Vehicle dialog — no evidence (commit, note, screenshot) that it was performed.

## Task Completion

| # | Task | Status | Evidence / Notes |
|---|------|--------|------------------|
| 1 | Marker primitive and `FormItem` `required` flag | DONE | `apps/web/src/components/ui/required.tsx` (new, `RequiredMarker`/`RequiredLegend`), `apps/web/src/components/ui/form.tsx:38-44` (`FormItemContextValue.required`), `:76-91` (`FormItem` destructures/provides `required`), `:97-115` (`FormLabel` renders marker), `:119-135` (`FormControl` emits `aria-required={required \|\| undefined}`); `apps/web/src/components/ui/form.test.tsx` (new, 76 lines); commit `10dc8ba`. |
| 2 | Forward `aria-required` through `CategoryCombobox` | DONE | `apps/web/src/components/features/vehicles/CategoryCombobox.tsx:42` (`'aria-required'?: boolean` in props), forwarded to trigger button; `CategoryCombobox.test.tsx` +20 lines; commit `ebc3271`. |
| 3 | `VehicleForm` create-only marking, legend, affected page test | DONE | `VehicleForm.tsx:65,78,91` (`<FormItem required={isCreate}>` on make/model/year), `:182` (`{isCreate && <RequiredLegend />}` — see deviation note below), `VehiclesPage.test.tsx` updated (+25/-… to accessible-name queries); commits `10f2c76`, `3d2e41d`. |
| 4 | `FuelForm` marking, either/or note, legend | DONE | `FuelForm.tsx:48` (`export const COST_REQUIREMENT = 'Enter price per gallon or total cost (or both).'`), `:78,93,116` (`<FormItem required>` on date/mileage/gallons), `:162,192` (`sr-only` `FormDescription` inside each cost `FormItem`), `:200` (visible `<p>` using `COST_REQUIREMENT` + "the server derives the missing value" suffix), `RequiredLegend` present; commit `4842832`. |
| 5 | `MaintenanceRecordForm` marking, legend, combobox slot coverage | DONE | `MaintenanceRecordForm.tsx:93,113` (`<FormItem required>` on categoryId, performedAt only — description/mileage/cost/vendor/notes intentionally unmarked per plan), `:225` `<RequiredLegend />`; `MaintenanceRecordForm.test.tsx` +32 lines covering the combobox `aria-required` case; commit `1b7f268`; `performedAt` runtime coverage added later in `3d2e41d`. |
| 6 | `MaintenanceScheduleForm` reactive interval marking | DONE | `MaintenanceScheduleForm.tsx:60,80` (`<FormItem required>` categoryId/recurrenceType), `:108` (`<FormItem required={showMonths}>`), `:136` (`<FormItem required={showMiles}>`), bound to pre-existing `showMonths`/`showMiles` booleans (`:50-51`); block-comment deviation applied at `:104,134` — see below; commit `7dbe3f3`; `MaintenanceScheduleForm.test.tsx` +94 lines. |
| 7 | The five short forms | DONE | `FleetNameForm.tsx:16`, `InviteForm.tsx:51,60`, `CompleteScheduleDialog.tsx:73`, `MileageForm.tsx:86`, `OnboardingPage.tsx:99` all gained `<FormItem required>`; matching test additions in `InviteForm.test.tsx`; commit `49cae2f`. |
| 8 | The two hand-rolled surfaces | DONE | `MemberList.tsx` — `<RequiredMarker />` + `aria-required="true"` on the successor `SelectTrigger` (`id="successor"`); `PurgeConfirmDialog.tsx` — `<RequiredMarker />` on the label + `aria-required="true"` on the confirmation input; both covered by new tests in `MemberList.test.tsx`/`PurgeConfirmDialog.test.tsx`; commit `b102fee`. |
| 9 | Convention guard test | DONE | `apps/web/src/test/requiredFieldMarkers.test.ts` (new, 156 lines) — source-scans every listed form file, splits on `<FormField`, asserts `EXPECTED` field-by-field map (including the three annotated deviations: `VehicleForm` make/model/year → `'isCreate'`, `FuelForm` totalCost/pricePerGallon → `false`, `MaintenanceScheduleForm` intervalMonths/intervalMiles → `'showMonths'`/`'showMiles'`), plus `EXPECTED_LEGEND` map and hand-rolled-surface region checks for `MemberList.tsx`/`PurgeConfirmDialog.tsx`; commit `895827d`. |
| 10 | Guidelines and reviewer rule | DONE | `.claude/skills/frontend-dev-guidelines/resources/patterns-forms-validation.md:216-218` (new "Required field indicators" section, references `FE-18`); `.claude/agents/frontend-guidelines-reviewer.md:137` (`FE-18` row added to checklist, text matches the design's mandated wording); commit `5f88c29`. |
| 11 | Full verification | PARTIAL | Step 1 (fe-test/fe-build): DONE, independently re-run — see Build & Test Results below. Step 2 (schema untouched): DONE, verified empty diff. Step 3 (regression files untouched, `getByText('Category')` intact): DONE, verified directly (`MaintenanceRecordForm.test.tsx:59`). Step 4 (no native `required`): DONE, verified via grep — no matches outside `aria-required`. **Step 5 (manual visual/theme check in Add Vehicle dialog): NOT PERFORMED** — no commit, note, or artifact evidences this was done; it is a human-only check the automated suite cannot substitute for. Step 6/7 (commit findings, request code review): no separate commit was needed since steps 1-4 passed clean; code review step not evidenced in this worktree's history at audit time. |

**Completion Rate:** 11/11 tasks implemented (100%); 10/11 fully clean, 1/11 (`Task 11`) has one outstanding manual sub-step.
**Skipped without approval:** 0
**Partial implementations:** 1 (Task 11, Step 5 only)

## Skipped / Deferred Tasks

- **Task 11, Step 5 (manual visual check in both themes).** The plan explicitly calls this out as one of "two manual checks the automated suite cannot make" (design.md §14) and requires opening the Add Vehicle dialog in the dev server and confirming the asterisk is legible in both light and dark themes, including in the error state after an empty submit. No evidence (commit message, screenshot, note in this audit's predecessor state, or plan checkbox) shows this was carried out. Impact: low-to-moderate — the color choice (`text-danger`) is backed by measured contrast ratios cited in `docs/tasks/task-003-dark-mode-branding/contrast.md` (6.67:1 light / 7.23:1 dark), so the risk of an actual accessibility failure is low, but the plan's own acceptance bar for this task has not been met and should not be represented as done without someone actually looking at it.

No other task was skipped, deferred, or left partial. The two deliberate deviations from the plan's literal wording — `/* … */` in `MaintenanceScheduleForm.tsx` (required by JSX-expression-context syntax rules inside `render={({ field }) => (`) and the conditional `{isCreate && <RequiredLegend />}` in `VehicleForm.tsx` (fixing an orphan "* Required" legend with no asterisk in Edit mode) — were both ruled on and applied as described; they are not gaps.

## Build & Test Results

| Service | Build | Tests | Vet | Notes |
|---------|-------|-------|-----|-------|
| apps/web (frontend) | PASS | PASS | N/A (no Go) | `make fe-build`: `tsc -b && vite build` succeeded, `dist/` produced (789.95 kB main chunk, expected pre-existing warning about chunk size, unrelated to this change). `make fe-test`: 101 files / 828 tests passed in apps/web, 2 files / 9 tests in packages/shared-ts, 1 file / 10 tests in packages/ui-components — all green, independently re-run on current HEAD. |

No Go service or `packages/*` Go code was touched (plan's Global Constraints scoped this to `apps/web` plus two `.claude/` files); `make ci`, `go build`/`go test`/`go vet`, and kustomize renders are correctly out of scope per the plan and were not run.

## Overall Assessment

- **Plan Adherence:** MOSTLY_COMPLETE
- **Recommendation:** NEEDS_REVIEW

The only gap is a human-only visual/theme confirmation (Task 11 Step 5) that has not yet been performed. All code, tests, and documentation deliverables are complete, correct, and independently verified as passing. This is not a code defect — it's an unperformed manual verification step the plan itself flags as required before calling the task done.

## Action Items

1. Perform Task 11 Step 5: run `apps/web` locally (`npm run dev`), open the Add Vehicle dialog, and visually confirm the required-field asterisk is legible in both light and dark themes, including in the error state after submitting the form empty. Record that this was done (e.g., in the plan checklist or a follow-up note) before treating the branch as ready to merge.
2. Confirm Task 11 Step 7 (dispatch `superpowers:requesting-code-review` / `/audit-plan` and resolve findings) has been or will be run before opening the PR, per `CLAUDE.md`'s "Code Review Before PR" rule — this audit covers plan adherence only, not the backend/frontend guideline reviewer passes.

---

# Frontend Guidelines Audit (FE-*)

# Frontend Audit — task-028-required-field-indicators

- **Audit Scope:** `git diff --name-only main...HEAD` (apps/web + 2 `.claude/` docs; doc commits at branch base excluded)
- **Guidelines Source:** frontend-dev-guidelines skill (including newly added FE-18)
- **Date:** 2026-08-25
- **Build:** PASS
- **Tests:** apps/web 828 passed (101 files); packages/shared-ts 9 passed (2 files); packages/ui-components 10 passed (1 file). 0 failed.
- **Overall:** PASS

## Build & Test Results

```
make fe-build → tsc -b && vite build → built in 865ms, no errors
make fe-test  → apps/web: Test Files 101 passed (101), Tests 828 passed (828)
              → packages/shared-ts: Test Files 2 passed (2), Tests 9 passed (9)
              → packages/ui-components: Test Files 1 passed (1), Tests 10 passed (10)
```

## File Inventory

- **Component (form primitive):** `apps/web/src/components/ui/form.tsx` — `FormItem`/`FormLabel`/`FormControl` gain the `required` context flag
- **Component (new primitive):** `apps/web/src/components/ui/required.tsx` — `RequiredMarker`, `RequiredLegend`
- **Component:** `apps/web/src/components/features/vehicles/CategoryCombobox.tsx` — forwards `aria-required` through Slot injection to its trigger `Button`
- **Component (forms):** `VehicleForm.tsx`, `FuelForm.tsx`, `MaintenanceRecordForm.tsx`, `MaintenanceScheduleForm.tsx`, `MileageForm.tsx`, `FleetNameForm.tsx`, `InviteForm.tsx`, `CompleteScheduleDialog.tsx` — each adds `required`/`required={expr}` to the `FormItem`s that need it
- **Component (hand-rolled, non-RHF):** `MemberList.tsx` (successor `<Select>`), `PurgeConfirmDialog.tsx` (type-to-confirm `<Input>`) — manual `RequiredMarker` + `aria-required="true"`
- **Page:** `OnboardingPage.tsx` — fleet-name field marked required
- **Other (tests):** `form.test.tsx`, `CategoryCombobox.test.tsx`, `InviteForm.test.tsx`, `MemberList.test.tsx`, `VehicleForm.test.tsx`, `FuelForm.test.tsx`, `MaintenanceRecordForm.test.tsx`, `MaintenanceScheduleForm.test.tsx`, `VehiclesPage.test.tsx`, `PurgeConfirmDialog.test.tsx`, and the new drift-guard `test/requiredFieldMarkers.test.ts`
- **Other (docs, authored by this branch, not code under FE-* review but cross-checked for accuracy):** `.claude/agents/frontend-guidelines-reviewer.md` (adds FE-18 row), `.claude/skills/frontend-dev-guidelines/resources/patterns-forms-validation.md` (adds "Required field indicators" section)
- **Not in scope:** no files under `lib/schemas/`, `services/api/`, `types/models/`, or `lib/hooks/api/` were touched (`git diff --name-only main...HEAD -- apps/web/src/lib/schemas/ apps/web/src/services/api/ apps/web/src/types/models/ apps/web/src/lib/hooks/api/` → empty)

## Anti-Pattern Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-01 | No `any` type | PASS | `grep -nE ': any\|as any'` over all 25 in-scope non-doc files → no matches |
| FE-02 | No manual class concatenation | PASS | `grep -nE 'className=\{"'` over all 25 files → no matches; e.g. `required.tsx:17,29` use plain string `className` literals |
| FE-03 | No direct API client/service calls in components | PASS | `grep -nE "from '(\.\./)+lib/api/client'\|from '(\.\./)+services/api/"` → 5 matches, all in `*.test.tsx` files (`InviteForm.test.tsx:6`, `MemberList.test.tsx:6-7`, `MaintenanceRecordForm.test.tsx:6`, `VehiclesPage.test.tsx:7`) importing service singletons for `vi.mock(...)`, not from component/page source files. No component or page file in scope imports a service or the client directly. |
| FE-04 | No inline Zod schemas in components | PASS | `grep -nE 'z\.object\(\|z\.string\('` over all 25 files → no matches; every form imports its schema from `lib/schemas/` (e.g. `VehicleForm.tsx:4`) |
| FE-05 | No spinners for content loading | PASS | `grep -n 'animate-spin'` → 11 matches, all `<Loader2 ... animate-spin>` gated on `submitting`/`*.isPending`/`isSubmitting` inside submit `<Button>`s (e.g. `VehicleForm.tsx:191`, `FuelForm.tsx:212`, `OnboardingPage.tsx:126`) or the category-create action in `CategoryCombobox.tsx:216` (an action spinner inside a `CommandItem`, gated on `createCategory.isPending`, pre-existing and untouched by this diff — `git diff` on the file shows only `aria-required` additions). None render on page/content load. |
| FE-06 | No hardcoded colors | PASS | `make fe-test` covers `src/test/conventions.test.ts:113-134`; the new `text-danger`/`text-muted-foreground` classes in `required.tsx:17,29` and `form.tsx` are semantic tokens, not matches for the `(bg\|text\|border\|ring\|divide)-(gray\|slate\|zinc\|neutral\|white\|black\|red\|green\|blue\|amber\|yellow\|emerald\|orange)` regex at `conventions.test.ts:114`. The 2-line dialog/sheet allowlist at `conventions.test.ts:130-133` is unmodified (`git diff --name-only` excludes `dialog.tsx`/`sheet.tsx`). |
| FE-07 | No state mutation | PASS | `grep -nE '\.push\(\|\.splice\(\|\.sort\('` → 1 file: `MemberList.test.tsx:290,294` (`order.push('patch')` / `order.push('delete')`, a plain test-tracking array asserting call order, not component state) |
| FE-08 | No default exports for components | PASS | `grep -n 'export default function'` over all 25 files → no matches; every component/page/hook is a named export (e.g. `export function VehicleForm`, `export function RequiredMarker`) |
| FE-09 | Error handling with `createErrorFromUnknown` | PASS | 5 `catch (err)` blocks in scope: `FleetNameForm.tsx:33-36`, `InviteForm.tsx:35-37` (delegates to `inviteErrorMessage`, which itself calls `createErrorFromUnknown` at `lib/hooks/api/invites.ts:102`, unchanged by this branch), `CategoryCombobox.tsx:113-116` (pre-existing, untouched), `CompleteScheduleDialog.tsx:65-68`, `OnboardingPage.tsx:94-97`. Each imports `createErrorFromUnknown` from `@myfleet/shared-ts` (e.g. `OnboardingPage.tsx:6`) and surfaces `apiError.message \|\| 'Could not …'` via `toast.error` — single-argument call, no bespoke fallback param. |

## Architecture Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-10 | JSON:API model shape | OUT-OF-SCOPE | `git diff --name-only main...HEAD -- apps/web/src/types/models/` → empty; no model file changed |
| FE-11 | Service reaches network only via `apiClient` | OUT-OF-SCOPE | `git diff --name-only main...HEAD -- apps/web/src/services/api/` → empty; no service file changed |
| FE-12 | Query key factory uses `as const` | OUT-OF-SCOPE | `git diff --name-only main...HEAD -- apps/web/src/lib/hooks/api/` → empty; no hook file changed |
| FE-13 | Forms use `react-hook-form` + `zodResolver` | PASS | Every changed form retains its existing `useForm({ resolver: zodResolver(...) })` call, unmodified by this branch except for the `required` prop additions on `FormItem` — e.g. `VehicleForm.tsx:29-30`, `FuelForm.tsx:59-60`, `CompleteScheduleDialog.tsx:46-47`. No form's resolver wiring was touched. |
| FE-14 | Schema in `lib/schemas/` with inferred type | PASS (unchanged) | `git diff --name-only main...HEAD -- apps/web/src/lib/schemas/` → empty, confirming the branch's own stated constraint. Existing schemas already comply (`lib/schemas/vehicle.ts:25` exports `VehicleFormInput`, `lib/schemas/fuel.ts:39` exports `FuelFormInput`, etc.), unaffected by this diff. |
| FE-18 | Required fields are marked | PASS | Cross-checked every changed form's `<FormItem>`/`<FormItem required>`/`<FormItem required={expr}>` against its schema's static optionality and against `test/requiredFieldMarkers.test.ts`'s `EXPECTED` table — all 9 RHF forms match exactly (`VehicleForm.tsx:65,78,91` → `required={isCreate}` matching `requiredFieldMarkers.test.ts:29-31`'s documented FR-15 deviation; `FuelForm.tsx:78,93,116` → `required` on date/mileage/gallons, `totalCost`/`pricePerGallon` correctly unmarked per the FR-21 either/or deviation at `requiredFieldMarkers.test.ts:41-46`, with `COST_REQUIREMENT` prose stated once and duplicated `sr-only` in each `FormItem` at `FuelForm.tsx:162,192`; `MaintenanceScheduleForm.tsx:108,136` → `required={showMonths}`/`required={showMiles}` bound to the same booleans that gate rendering, matching FR-19 at `requiredFieldMarkers.test.ts:60-64`). The two non-RHF surfaces both carry `RequiredMarker` + `aria-required="true"`: `MemberList.tsx:275-280` (successor picker, only rendered when `needsSuccessor`) and `PurgeConfirmDialog.tsx:199-205` (always-required confirmation input) — both exercised by dedicated assertions at `requiredFieldMarkers.test.ts:135-155`. `CategoryCombobox.tsx:42,67,152` forwards `aria-required` from `FormControl`'s Slot injection to the focusable trigger `Button`, verified by `CategoryCombobox.test.tsx:192-207`. No native HTML `required` attribute anywhere in scope (`grep -rn '<Input[^>]*\brequired\b'` / `<Textarea...>` / `<SelectTrigger...>` → only `aria-required` matches). The marker uses `text-danger` (`required.tsx:17,29`), not `text-destructive`. `aria-required={required \|\| undefined}` in `form.tsx:133` avoids the string-`"false"` bug. Legend rendering matches `EXPECTED_LEGEND` exactly (3+ field forms render `<RequiredLegend />`; `MileageForm`, `CompleteScheduleDialog`, `FleetNameForm`, `InviteForm`, `OnboardingPage` correctly omit it at 1-2 fields). |

### FE-18 rule accuracy (branch also authored this rule)

The rule as added to `.claude/agents/frontend-guidelines-reviewer.md:137` and the "Required field indicators" section in `patterns-forms-validation.md` accurately describe what shipped: single declaration on `FormItem`, `RequiredMarker`/`aria-required` both sourced from `FormItemContext`, `text-danger` not `text-destructive`, `aria-hidden` marker, no native `required`, `aria-required={required || undefined}`, custom-control forwarding requirement, conditional-required binding to the same visibility boolean, cross-field either/or handled in prose rather than per-field marks, and the drift-guard test. One minor gap: the rule's "How to Verify" column (`grep -n "<FormItem" <form file>`) only describes the RHF-form recipe and doesn't mention the two hand-rolled surfaces (`MemberList.tsx`, `PurgeConfirmDialog.tsx`) that the Pass Criteria and the patterns doc both cover — a future reviewer following only the verify-column literally would miss those two files. Non-blocking; the Pass Criteria and linked doc make the full scope recoverable.

## Styling Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-15 | Interactive elements show `cursor-pointer` | PASS | All `onClick` handlers added/touched by this diff are on `<Button>` (`PurgeConfirmDialog.tsx:214,221`; `MemberList.tsx:173,186,196,225,251,306`) or `AlertDialogAction`, both of which carry `cursor-pointer` unconditionally via the shared variant class (`components/ui/button.tsx:7`). No new custom clickable `<div>`/non-button interactive element was introduced by this branch; the branch's only new interactive surface is `CategoryCombobox`'s `PopoverTrigger asChild` wrapping a `Button` (pre-existing, cursor already covered by the same base class), confirmed unchanged by `git diff` on that file's trigger markup. |

## Testing Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-16 | Tests exist for changed components | NEEDS-WORK (non-blocking) | 10 of 13 changed component/page files have a sibling `*.test.tsx` updated or already present (`PurgeConfirmDialog.test.tsx`, `InviteForm.test.tsx`, `MemberList.test.tsx`, `CategoryCombobox.test.tsx`, `VehicleForm.test.tsx`, `FuelForm.test.tsx`, `MaintenanceRecordForm.test.tsx`, `MaintenanceScheduleForm.test.tsx`, `form.test.tsx`, `OnboardingPage.test.tsx`). Three files have no sibling test: `FleetNameForm.tsx`, `CompleteScheduleDialog.tsx`, `MileageForm.tsx`. Each of those three changes is a single-line `<FormItem>` → `<FormItem required>` diff (`git diff main...HEAD` on each shows exactly `+1 -1`), and all three are exercised by the source-scanning drift guard `test/requiredFieldMarkers.test.ts:66-73` (`MileageForm.tsx`, `CompleteScheduleDialog.tsx`) and `:74-76` (`FleetNameForm.tsx`), which fails if the `required` marking on these exact fields regresses. Calling this non-blocking: the drift guard is a real, executing assertion against these files' source, not a documentation promise, even though it isn't a component-behavior test in the conventional RTL sense. |
| FE-17 | `vi.mock` call sites updated when a service changes | OUT-OF-SCOPE | No file under `apps/web/src/services/api/` changed in this branch (see FE-11 evidence), so there is no service-signature drift for a `vi.mock` stub to fall behind. |

## Summary

### Blocking (must fix)

None.

### Non-Blocking (should fix)

- FE-16: `FleetNameForm.tsx`, `CompleteScheduleDialog.tsx`, `MileageForm.tsx` have no sibling `*.test.tsx`; coverage for their `required` marking comes only from the source-scanning `requiredFieldMarkers.test.ts`, not a component-behavior test. Low risk given the single-line nature of each diff, but worth a follow-up if these files gain more form logic later.
- FE-18 rule text: the "How to Verify" column in `.claude/agents/frontend-guidelines-reviewer.md:137` only names the RHF-form grep recipe and omits the two hand-rolled surfaces (`MemberList.tsx`, `PurgeConfirmDialog.tsx`) that the Pass Criteria and linked doc section do cover. Consider adding a one-line pointer to those two files in the Verify column so a reviewer following it literally doesn't skip them.
