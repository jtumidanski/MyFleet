# Plan Audit — task-016-add-vehicle-dialog

**Plan Path:** docs/tasks/task-016-add-vehicle-dialog/plan.md
**Audit Date:** 2026-08-02
**Branch:** task-016-add-vehicle-dialog (HEAD `25bef09`)
**Base Branch:** main (`5ff93cd`; merge-base `26cc9e9`)

## Executive Summary

All 6 plan tasks are implemented. 33 of 34 checkbox steps are complete; the single
unchecked box — Task 6 Step 5, the manual browser smoke check — genuinely requires a
real browser and is correctly left open rather than silently skipped. `make ci` exits 0
(verified with `set -o pipefail`, since the reported prior run piped through `tail` and
would have masked a failure). Test counts land exactly on the plan's predictions:
331 in 42 files for `apps/web`, 7 for `shared-ts`, 10 for `ui-components`, 43 Go
packages `ok`, 0 `FAIL`.

Two deviations were **not** on the handed-over list and are recorded below: the overlay
scrim classes (justified — the plan's literal would have failed an existing conventions
test) and an unconditional `aria-modal="true"` on `DialogContent` (necessary — verified
that Radix 1.1.23 does not emit it — but it carries a latent defect for a future
`modal={false}` consumer of this shared primitive). The out-of-scope `apiClient.test.ts`
fix was verified against main as a real pre-existing build break and is a correct,
minimal, test-only repair.

Recommendation: **NEEDS_REVIEW** — no blocking defect, but the manual smoke check is
still outstanding and one shared-primitive nit is worth a decision before merge.

## Task Completion

| # | Task | Status | Evidence / Notes |
|---|------|--------|------------------|
| 1 | The `Dialog` primitive | DONE | `apps/web/src/components/ui/dialog.tsx` (172 lines, new); `apps/web/package.json:15`; 13 tests in `dialog.test.tsx`. Two class-level deviations — see D-1, D-2. |
| 1.1 | Install dependency | DONE | `apps/web/package.json:15` `"@radix-ui/react-dialog": "^1.1.23"`, sorted before `react-label:16`. Version literal differs from plan — see D-3. `package-lock.json` +349 lines, `react-dialog` + transitives only. |
| 1.2 | Write the failing test | DONE | `dialog.test.tsx` is **verbatim** from plan.md:129-306. |
| 1.3 | Verify it fails | DONE | Implied by commit order `fccfcc8` (test + impl same commit per plan Step 7); not independently re-observable post-hoc. |
| 1.4 | Write the implementation | DONE (with notes) | `dialog.tsx`. Four handlers destructured out of `props` as required (`:53-56`); close button rendered after `children` (`:107-116`); `DialogHeader`/`DialogFooter` carry literal `displayName` (`:127`, `:135`). Deviations D-1, D-2. |
| 1.5 | Verify it passes | DONE | 13 tests, all green in `make ci` run. |
| 1.6 | Lint and format | DONE | `prettier --check` and `eslint src --max-warnings 0` both pass in the ci log. |
| 1.7 | Commit | DONE | `fccfcc8` + follow-up `b908947`. |
| 2 | `VehicleList` empty-state action | DONE | `VehicleList.tsx:14` `emptyAction?: ReactNode`; copy branch at `:32`; action slot at `:34`. 4 tests. |
| 2.1 | Write the failing test | DONE | `VehicleList.test.tsx` is **verbatim** from plan.md:541-617 (diff shows only the code-fence lines). |
| 2.2 | Verify it fails | DONE | Implied by commit `3e917b5`. |
| 2.3 | Write the implementation | DONE | `VehicleList.tsx:1-46` matches the plan's replacement exactly. Copy moved into `<p>` as the plan required. No auth/role import (FR-3.3 holds). |
| 2.4 | Verify it passes | DONE | 4 tests green. |
| 2.5 | Commit | DONE | `3e917b5`. |
| 3 | Create dialog on `VehiclesPage` | DONE | `VehiclesPage.tsx:78-115`; inline `Card` import and JSX gone (`git diff` shows the `Card` import line removed). |
| 3.1 | Write the failing test | DONE (adapted) | `VehiclesPage.test.tsx`, 21 tests. Two typing helpers added and one assertion rewritten — see D-4, D-5. |
| 3.2 | Verify it fails | DONE | Implied by commit `6db2342`. |
| 3.3 | Write the implementation | DONE | `VehiclesPage.tsx:20-31` carries `toCreateAttributes` verbatim; dialog at `:91-113`; both triggers at `:72` and `:123`. |
| 3.4 | Verify it passes | DONE | Plan predicted 15 at this point; file now holds 21 after Tasks 4-5 appended, matching the plan's own arithmetic. |
| 3.5 | Commit | DONE | `6db2342`. |
| 4 | Not dismissible while in flight | DONE | `VehicleForm.tsx:181` `disabled={submitting}`; `VehiclesPage.tsx:81-87` `onOpenChange` backstop; `:92` `dismissible={!createVehicle.isPending}`. |
| 4.1 | Write the failing test | DONE | `VehiclesPage.test.tsx:258-300`, 4 cases, matches plan.md:1116-1160. |
| 4.2 | Verify it fails | DONE | Implied by commit `cf7a80a`. |
| 4.3 | Disable Cancel while submitting | DONE | `VehicleForm.tsx:181` — one attribute, exactly as scoped. `edit`-mode element tree unchanged (diff is a single line). |
| 4.4 | Lock the dialog while pending | DONE | `VehiclesPage.tsx:79-92`, including the plan's backstop comment. |
| 4.5 | Verify it passes | DONE | 4 cases green. |
| 4.6 | Commit | DONE | `cf7a80a`. |
| 5 | Focus after the empty state disappears | DONE | `VehiclesPage.tsx:39-49` (refs + `openFrom`), `:58` (`createdRef.current = true`), `:93-101` (`onCloseAutoFocus` redirect). |
| 5.1 | Write the failing test | DONE | `VehiclesPage.test.tsx:302-334`, 2 cases, matches plan.md:1247-1279 modulo the `emptyStateTrigger()` helper. |
| 5.2 | Verify it fails | DONE | Implied by commit `bea381e`. |
| 5.3 | Write the implementation | DONE | Condition is `openedFromRef.current === 'empty' && createdRef.current` (`:97`) — the outcome-based signal the plan mandated, not a DOM-liveness check. `Button` forwards refs (`button.tsx:37`), so `headerButtonRef` is live. |
| 5.4 | Verify it passes | DONE | 21 tests in the file, matching the plan's prediction exactly. |
| 5.5 | Commit | DONE | `bea381e`. |
| 6 | Full verification | PARTIAL | Steps 1-4 and 6 done; Step 5 outstanding. |
| 6.1 | Format and lint | DONE | ci log: `prettier --check` over all three workspaces, `eslint src --max-warnings 0`. |
| 6.2 | Type-check and build | DONE | ci log: `tsc -b && vite build`, `✓ 1806 modules transformed`, `✓ built in 4.46s`. Required the D-6 fix to pass. |
| 6.3 | Run the full test suite | DONE | 331/42 (`apps/web`), 7/2 (`shared-ts`), 10/1 (`ui-components`) — the plan's predicted numbers to the digit. Per-file: dialog 13, VehicleList 4, VehiclesPage 21 = +38 on the 293 baseline. |
| 6.4 | Confirm the blast radius | DONE | `git status --porcelain` empty. `git diff --stat 5ff93cd...HEAD` = the 9 planned paths + 4 task docs + `packages/shared-ts/src/apiClient.test.ts` (D-6). `git diff --name-only 5ff93cd...HEAD \| grep -c VehicleDetailPage` → **0**. Hard constraint holds. |
| 6.5 | Manual smoke check | **NOT RUN** | The only unchecked box in plan.md (line 1457). Requires a real browser. See "Outstanding" below. |
| 6.6 | Commit formatting rewrites | DONE | `ae258d1`, `6488cfe`, `25bef09`; tree clean. |

**Completion Rate:** 33/34 steps (97%); 5/6 tasks fully DONE, Task 6 PARTIAL.
**Skipped without approval:** 0
**Partial implementations:** 1 (Task 6, on its manual-verification step only)

## Deviations

### D-1 — Overlay scrim classes (undocumented; justified) — `dialog.tsx:24`

Plan Task 1 Step 4 specified `bg-black/80`. Implementation uses
`bg-foreground/80 dark:bg-background/80`.

This was **required**, not stylistic. `apps/web/src/test/conventions.test.ts` scans every
`.tsx` under `apps/web/src` for `/(bg|text|border|ring|divide)-(gray|slate|…|white|black|…)/`
and fails on any hit; `bg-black/80` matches. The plan's literal would have broken an
existing test. The substitution is also semantically correct in both themes:
`foreground` is near-black in light mode, `background` is near-black in dark mode, so the
scrim stays dark either way rather than going light-on-light. Landed as `b908947`
("correct Dialog overlay scrim for light mode") with the reasoning in a code comment.

Not on the handed-over deviation list — recorded here so it is not mistaken for drift.
No action needed beyond noting it in the PR description.

### D-2 — Unconditional `aria-modal="true"` (undocumented; necessary, with a latent defect) — `dialog.tsx:76`

Not in the plan's implementation listing. **The claim in the code comment is true** — I
verified it independently rather than taking it on trust:

- `grep -c 'aria-modal' node_modules/@radix-ui/react-dialog/dist/index.mjs` → **0**
- The only occurrence anywhere in the package is inside a source comment in
  `index.mjs.map` (`… (aria-modal)\n React.useEffect(…)`), attached to the
  `hideOthers`/`aria-hidden` effect — a comment, not an emitted attribute.
- Installed version is 1.1.23.

So Radix 1.1.23 hides the rest of the page with `aria-hidden` instead of announcing
`aria-modal`, and the plan's own test (plan.md:175) plus the PRD acceptance criterion
"exposes `role="dialog"` with `aria-modal="true"`" (prd.md:174) could not pass without
this line. Adding it was correct.

**Latent defect, low severity today.** It is set unconditionally on `DialogContent`,
ignoring the Root's `modal` prop. `<Dialog modal={false}>` renders Radix's
`DialogContentNonModal`, which neither aria-hides the page nor traps focus — such a
consumer would inherit `aria-modal="true"`, a false promise to assistive tech. That
matters because this file is explicitly a shared primitive the PRD requires to be
"generic enough to serve future modals … without modification" (prd.md:128) and the plan
constrains to contain no consumer-specific assumptions (plan.md:23). It is also placed
*before* `{...props}` (`:103`), so an override is possible but undiscoverable.

Only consumer today is `VehiclesPage`, which is modal, so nothing is currently wrong.
Recommend deriving the attribute from the Root's `modal` context or documenting the
constraint in the `DialogContentProps` doc comment.

### D-3 — Dependency literal `^1.1.23` vs the plan's `^1.1.0` — `apps/web/package.json:15`

Cosmetic. FR-1.1 requires the dependency present and the `@radix-ui/*` run alphabetical;
both hold (`react-dialog:15` precedes `react-label:16`). `^1.1.23` is a strictly narrower
but compatible range inside the same 1.1.x line the plan pinned, and the plan's own
"Verified environment facts" §1 records that `^1.1.0` resolves to 1.1.23 anyway.
Non-blocking.

### D-4 — Rewritten `keeps the header trigger rendered while the dialog is open` — `VehiclesPage.test.tsx:115-126`

**The rewrite is sound and does prove FR-3.1.** The plan's version was not merely
unlucky, it was unrunnable: `headerTrigger()` routes through `getAllByRole`, and Radix
`aria-hidden`s the page behind an open modal, so `getAllByRole` *throws* before the
`toBeInTheDocument()` assertion is ever reached.

The rewrite captures the node pre-open and asserts `toBeInTheDocument()` (a
`document.contains()` check, unaffected by the accessibility tree), plus
`closest('[aria-hidden="true"]')` as a bonus proof of FR-4.1. FR-3.1's wording is "the
header **Add Vehicle** button remains … still rendered while the dialog is open"
(prd.md:71, :159) — rendered, which is exactly what is asserted.

Caveat: it proves *rendered*, not *visually visible* (no computed-style assertion).
Adequate here because nothing applies `display:none` and `VehiclesPage.tsx:71` gates the
button on `canWrite` alone with no `open` condition — the removal of the old `!showForm`
condition is visible directly in the source.

### D-5 — Test typing helpers under `noUncheckedIndexedAccess` — `VehiclesPage.test.tsx:65-71, 74, 207`

`triggerAt(index)` throws on a miss rather than returning `undefined`, `emptyStateTrigger()`
names index 1, and `dismissals` widens to `Array<[string, () => Promise<unknown>]>`
(`userEvent.keyboard` returns `Promise<System>`, not `Promise<void>`). All three are
behaviour-preserving and the throwing helper is strictly better than the plan's silent
`triggers()[i]` — a miscount now fails on that fact rather than on a downstream assertion
about a node that was never there. Landed as `ae258d1`. Approved.

### D-6 — Out-of-scope fix to `packages/shared-ts/src/apiClient.test.ts` (`25bef09`)

**Verified independently as a real pre-existing break.** Running `npx tsc -b packages/shared-ts`
from the *main* checkout at `/home/tumidanski/source/MyFleet` reports:

```
apps/…/apiClient.test.ts(33,21): error TS2532: Object is possibly 'undefined'.
  (also 51,21 / 81,21 / 114,26 / 115,27)
```

The break arrived with `9c93b6b` ("test(shared-ts): pin caller Content-Type override in
request()") on main, an ancestor of both branches. Since `npm run build` at the workspace
root is a Task 6 Step 2 gate *and* a PRD build-hygiene acceptance criterion (prd.md:183),
the branch could not be verified without repairing it.

**The deviation was reasonable and correctly executed.** The fix is type-only: a
`headersOfCall(fetchMock, index)` helper that throws on a missing call, replacing five
`fetchMock.mock.calls[n][1]` indexings. No production code touched, no assertion
weakened or removed, `shared-ts` still reports 7 tests. Commit message documents it.

Two notes:
- It widens the PR's review surface beyond `apps/web`, contradicting the plan's "Scope is
  `apps/web` only" (plan.md:13). Worth one line in the PR description so a reviewer is
  not surprised by a `packages/shared-ts` file.
- Nit: `headersOfCall` casts `call[1] as RequestInit` with no guard, so an absent second
  argument would surface as a `TypeError` rather than the helper's named error. Cosmetic,
  test-only.

## Outstanding

### Task 6 Step 5 — manual browser smoke check (NOT RUN)

Correctly left unchecked, not silently skipped. It is the sole gate on requirements jsdom
cannot prove, because jsdom performs no layout:

- **FR-1.5** — centring, the `max-h-[85vh]` cap, internal body scroll with the close
  button staying pinned. Classes are structurally present and correct
  (`dialog.tsx:78` `fixed left-1/2 top-1/2 max-h-[85vh] max-w-lg -translate-x-1/2
  -translate-y-1/2 flex flex-col`; `dialog.tsx:107` `min-h-0 flex-1 overflow-y-auto`),
  but "the classes are there" is not "the layout is right".
- **FR-1.4 / theming NFR** — dark-mode legibility of the surface, border, text and close
  button, including the D-1 scrim substitution which no test exercises visually.
- **FR-4.2 / accessibility NFR** — the visible `ring-ring` focus ring on the close button.
- **Acceptance criterion "the vehicle grid does not shift when the dialog opens"** —
  structurally guaranteed by the portal (`dialog.tsx:69`) but unproven visually.

**Warning for whoever runs it:** `tailwindcss-animate` is not installed
(`tailwind.config.ts` has `plugins: []`), so every `data-[state=*]:animate-in`,
`fade-in-0` and `zoom-in-95` class in `dialog.tsx:24` and `:78` generates no CSS and
there will be **no open/close transition at runtime**. This is a deliberate, documented
decision (plan.md:62 — installing the plugin would retroactively animate the existing
Select). FR-1.7 is satisfied literally: the `data-state` selectors are present and mirror
`select.tsx`. Do not report the missing animation as a bug.

## PRD Coverage Map Verification

Every row of the plan's coverage map (plan.md:1486-1518) was checked against the code.
All 30 rows hold. Two are weaker than they read:

| Requirement | Verdict | Note |
|---|---|---|
| FR-1.1 … FR-1.2 | HOLDS | 10 named exports at `dialog.tsx:161-172`, exactly the FR-1.2 list. |
| FR-1.3 | HOLDS | `forwardRef` on Overlay/Content/Title/Description; `cn()` merge with `className` last in every wrapper; `displayName` from the Radix primitive on all four, literal strings on Header/Footer. |
| FR-1.4 | HOLDS (pending smoke) | Semantic tokens only; enforced by `conventions.test.ts`. Visual check is Step 5.4. |
| FR-1.5 | HOLDS structurally (pending smoke) | See Outstanding. |
| FR-1.6 | HOLDS | `dialog.test.tsx:51-55`; `sr-only` "Close" at `dialog.tsx:115`. |
| FR-1.7 | HOLDS literally | Selectors present; **no CSS generated** — see the Outstanding warning. |
| FR-2.1 … FR-2.9 | HOLDS | Each named test located and green. FR-2.9's call-argument assertion is at `VehiclesPage.test.tsx:153-162`. |
| FR-3.1 | HOLDS | Via the D-4 rewrite; also visible directly at `VehiclesPage.tsx:71`. |
| FR-3.2 … FR-3.5 | HOLDS | `VehicleList.tsx` imports no auth module (confirmed by grep) — FR-3.3's structural half. |
| FR-4.1 | HOLDS | Via D-2's explicit attribute plus Radix's `aria-hidden`, asserted at `VehiclesPage.test.tsx:125`. |
| FR-4.2 | UNPROVEN by suite | Mapped to Radix `FocusScope` + Step 5.5. No test tabs out of the dialog. Acceptable — it is Radix's own guarantee — but it is the manual check that closes it. |
| FR-4.3 | **INDIRECT** | The only initial-focus assertion is `dialog.test.tsx:59-65`, against the synthetic harness's `First field`, not the real `VehicleForm`. The PRD names the Nickname input specifically; `VehicleForm.tsx:49` confirms Nickname *is* the first field and the mechanism (close button after `children`, `dialog.tsx:107-116`) is shared, so the requirement is met. But nothing in the suite would catch a future reordering of `VehicleForm`'s fields or a stray `autoFocus`. Minor coverage gap; the plan mapped it this way deliberately. |
| FR-4.4 … FR-4.6 | HOLDS | Three focus-restoration tests located across both suites. |
| NFRs | HOLDS | `dialog.tsx` contains no vehicle string (grep clean); `does not mount its children before the first open` green; build hygiene per §Build below. |

## Build & Test Results

`make ci` re-run under `set -o pipefail` (the previously reported run piped through
`tail`, whose exit code would have masked any failure). **`MAKE_CI_EXIT=0`.**

| Target | Build | Tests | Vet/Lint | Notes |
|---|---|---|---|---|
| Go (all services) | PASS | PASS | PASS | `go vet ./...` clean; `go test -race` → 43 packages `ok`, 0 `FAIL`. No Go file changed on this branch. |
| apps/web | PASS | PASS | PASS | `tsc -b && vite build` → 1806 modules, built in 4.46s. **331 tests / 42 files.** `eslint src --max-warnings 0` clean. |
| packages/shared-ts | PASS | PASS | PASS | 7 tests / 2 files. Required D-6 to type-check. |
| packages/ui-components | PASS | PASS | PASS | 10 tests / 1 file. |
| prettier | — | — | PASS | `--check` across all three workspaces. |
| manifests / carfax-template | PASS | — | — | `check-manifests.sh` ran as part of `ci`. |

Vite's >500 kB chunk-size warning is pre-existing and unrelated.

## Overall Assessment

- **Plan Adherence:** MOSTLY_COMPLETE — every task implemented, one manual verification
  step outstanding by necessity.
- **Recommendation:** NEEDS_REVIEW

Nothing here blocks on correctness. The implementation is faithful: the two large test
files are verbatim from the plan, the deviations that exist are each forced by something
real (a conventions test, a Radix behaviour, `noUncheckedIndexedAccess`, a broken main)
rather than by convenience, and the hard constraint on `VehicleDetailPage.tsx` holds.
What keeps this off READY_TO_MERGE is that FR-1.5, FR-1.4's visual half and FR-4.2 are
still unverified, and one shared-primitive decision deserves an explicit call.

## Action Items

1. **Run Task 6 Step 5** (`npm run dev --workspace apps/web`) and check all five points.
   Expect no open/close animation — that is by design, not a defect.
2. **Decide on `aria-modal` (D-2):** either derive it from the Root's `modal` prop or add
   a line to the `DialogContentProps` doc comment stating that this primitive assumes a
   modal Root. Cheap now; a trap for the first `modal={false}` consumer.
3. **Note D-1 and D-6 in the PR description** — the overlay-class substitution and the
   `packages/shared-ts` repair are both outside the plan's stated blast radius and will
   otherwise read as scope creep to a reviewer.
4. *(Optional)* Add an initial-focus assertion against the real form in
   `VehiclesPage.test.tsx` to close the FR-4.3 gap, so a future reordering of
   `VehicleForm`'s fields cannot silently break it.
5. *(Optional, cosmetic)* Guard the `call[1] as RequestInit` cast in `headersOfCall`
   (`packages/shared-ts/src/apiClient.test.ts`) so a missing second argument fails by
   name.

---

# Frontend Guidelines Audit

- **Audit Scope:** 7 changed TS/React files under `apps/web/src` + `packages/shared-ts/src/apiClient.test.ts` (diff `26cc9e9`..`25bef09`)
- **Guidelines Source:** `.claude/skills/frontend-dev-guidelines` (SKILL.md + 10 resource docs)
- **Date:** 2026-08-02
- **Build:** PASS (`tsc -b && vite build`, exit 0)
- **Tests:** 331 passed / 0 failed (42 files)
- **Lint:** PASS (`eslint src --max-warnings 0`, exit 0)
- **Overall:** PASS

## Build & Test Results

```
> build
> tsc -b && vite build
vite v5.4.21 building for production...
✓ 1806 modules transformed.
✓ built in 3.62s

Test Files  42 passed (42)
     Tests  331 passed (331)
  Duration  5.23s
```

## File Inventory

| File | Classification |
|------|----------------|
| `apps/web/src/components/ui/dialog.tsx` | Component (ui primitive, new) |
| `apps/web/src/components/ui/dialog.test.tsx` | Test (new, 14 tests) |
| `apps/web/src/components/features/vehicles/VehicleList.tsx` | Component (feature) |
| `apps/web/src/components/features/vehicles/VehicleList.test.tsx` | Test (new, 4 tests) |
| `apps/web/src/components/features/vehicles/VehicleForm.tsx` | Component (feature, 1-line) |
| `apps/web/src/pages/VehiclesPage.tsx` | Page |
| `apps/web/src/pages/VehiclesPage.test.tsx` | Test (new, 21 tests) |
| `packages/shared-ts/src/apiClient.test.ts` | Test (type-only repair) |

No Hook, Service, Schema, or Type files changed.

## Anti-Pattern Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-01 | No `any` type | PASS | grep `: any\|as any` across all 8 files — zero matches. `apiClient.test.ts:16` uses `call[1] as RequestInit`, a narrowing cast, not `any` |
| FE-02 | No manual class concatenation | PASS | `cn()` used at `dialog.tsx:17,77,125,131,143,155`; zero `+`/template concatenation in any `className` |
| FE-03 | No direct API client in components | PASS | `VehiclesPage.tsx:5` imports `useVehicles, useCreateVehicle` from `lib/hooks/api/vehicles`; zero `lib/api/client` imports |
| FE-04 | No inline Zod schemas | PASS | zero `z.object(` / `from 'zod'` in components; `VehicleForm.tsx:4` imports `vehicleSchema` from `lib/schemas/vehicle` |
| FE-05 | No spinners for content loading | PASS | Only `animate-spin` is `VehicleForm.tsx:186`, on the submit button. Content loading uses `VehicleCardSkeleton` (`VehicleList.tsx:18-26`) |
| FE-06 | No hardcoded colors | PASS | Zero palette-class matches. Overlay uses `bg-foreground/80 dark:bg-background/80` (`dialog.tsx:24`); content uses `bg-background border-border` (`dialog.tsx:78`). Enforced repo-wide by `src/test/conventions.test.ts:113-153`, which passes |
| FE-07 | No state mutation | PASS | Zero `.push(` / `.splice(` / `.sort(`. `VehiclesPage.tsx:37-43` uses `useState` + `useRef` only |
| FE-08 | No default exports | PASS | Zero `export default`. `dialog.tsx:161-172` named export block; `VehicleList.tsx:17`, `VehiclesPage.tsx:33` named |
| FE-09 | `createErrorFromUnknown` error handling | PASS | `VehiclesPage.tsx:60-64` — catch → `createErrorFromUnknown(err)` → `toast.error(...)`. Success path `:57` `toast.success` |

## Architecture Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-10 | JSON:API model shape | PASS | `types/models/vehicle.ts:36` `export type Vehicle = JsonApiResource<VehicleAttributes>`. Test fixtures honour it (`VehiclesPage.test.tsx:29-33`, `VehicleList.test.tsx:8-12` — `{ type, id, attributes }`) |
| FE-11 | Service extends `BaseService` | N/A | No `services/api/` file changed |
| FE-12 | Query key factory uses `as const` | PASS | `lib/hooks/api/vehicles.ts:15-21` — all five members `as const`. Unchanged by this task; `useCreateVehicle` (`:48-58`) invalidates `vehicleKeys.lists()` |
| FE-13 | Forms use `react-hook-form` + `zodResolver` | PASS | `VehicleForm.tsx:28-29` `useForm<VehicleFormInput>({ resolver: zodResolver(vehicleSchema), ... })` |
| FE-14 | Schema in `lib/schemas/` with inferred type | PASS | `lib/schemas/vehicle.ts:7` `vehicleSchema`, `:26` `export type VehicleFormInput = z.infer<typeof vehicleSchema>` |

## Styling Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-15 | Interactive elements show `cursor-pointer` | PASS (1 note) | Every interactive element in the diff is the shared `<Button>`, whose CVA carries `cursor-pointer` (`components/ui/button.tsx:7`): `VehiclesPage.tsx:72,123`, `VehicleForm.tsx:181,185`. See NB-3 for the close button |

## Testing Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-16 | Tests exist for changed components | PASS | `dialog.test.tsx` (14 tests: announcement, focus, dismissal, `dismissible={false}`, lifecycle); `VehicleList.test.tsx` (4 tests: both empty-copy variants, populated, loading); `VehiclesPage.test.tsx` (21 tests). The `VehicleForm.tsx:181` one-liner is covered by `VehiclesPage.test.tsx:292-299` |
| FE-17 | Mocks updated when services changed | N/A | No service interfaces changed. `VehiclesPage.test.tsx:20-22` mocks at the service boundary per `testing-guide.md:188-197` |

Test quality is above the bar in `testing-guide.md:214-223`: queries are role/text based, `userEvent` is preferred over `fireEvent` (`fireEvent.pointerDown` is used only where jsdom cannot drive the overlay, and says so at `VehiclesPage.test.tsx:227`), and accessibility is asserted directly via `toHaveAccessibleName` / `toHaveAccessibleDescription` (`VehiclesPage.test.tsx:102-103`, `dialog.test.tsx:46-48`).

## Response to the Two Findings Already Raised

### (a) `aria-modal="true"` is unconditional — CORROBORATED (non-blocking)

`dialog.tsx:76` hardcodes `aria-modal="true"`. Verified against the installed
`@radix-ui/react-dialog@1.1.23`: `dist/index.mjs` contains no `aria-modal` string at all
(so the code comment's justification for asserting it is accurate), but it *does* branch
Content on the Root's modality — both `ContentModal` and `ContentNonModal` are present and
`context.modal` is read. A `<Dialog modal={false}>` would therefore render the non-modal
content path while this wrapper still claims `aria-modal="true"` to assistive tech.

Mitigating: the attribute is written at `:76`, *before* `{...props}` at `:103`, so a
consumer can override it per-`DialogContent`. No current caller passes `modal={false}`
(`VehiclesPage.tsx:79-88` omits it, defaulting to `modal`), so this is latent, not live.

### (b) Overlay uses `bg-foreground/80 dark:bg-background/80` — CONFIRMED CORRECT, not a defect

Two independent things check out:

1. The ban is real. `src/test/conventions.test.ts:114-115` matches
   `/(bg|text|border|ring)-(gray|slate|zinc|neutral|white|black|red|...)/` over every
   `.tsx` under `apps/web/src`, so `bg-black/80` would fail that test.
2. The substitution actually works in both themes, which is the part worth verifying
   rather than assuming. `src/index.css:8` sets light `--foreground: 222.2 84% 4.9%`
   (near-black) and `:53` sets dark `--background: 222.2 84% 4.9%` (also near-black).
   The scrim is a dark, ~80%-opaque backdrop in light *and* dark mode — it does not go
   light-on-light in either.

No action needed beyond the PR note already recommended in Action Item 3 above.

## Additional Findings

### NB-1 — Animation utility classes are inert (non-blocking, no action required)

`dialog.tsx:24` and `dialog.tsx:78` apply `animate-in`, `animate-out`, `fade-in-0`,
`fade-out-0`, `zoom-in-95`, `zoom-out-95`. These ship with `tailwindcss-animate`, which is
**not** a dependency (`apps/web/package.json` has no such entry) and **not** registered
(`tailwind.config.ts` ends with `plugins: []`). Verified empirically rather than by
inference: grepping the production bundle `dist/assets/index-*.css` for those six class
names returns zero hits. The dialog will open and close with no transition.

This is *not* a new deviation. The stated reference primitive does exactly the same thing
at `components/ui/select.tsx:66`, which carries the identical class list plus
`slide-in-from-*`. The new file is consistent with the convention it was told to follow,
and the plan-adherence audit already flags "expect no open/close animation — that is by
design". Recording it here only so the dead classes are not mistaken for working
animation by a later reader.

### NB-2 — Dialog close button focus order does not match visual order (non-blocking)

`dialog.tsx:110-116` renders `DialogPrimitive.Close` *after* `{children}` specifically so
Radix's initial autofocus lands on the first form control rather than on Close — the
comment at `:108-109` states this, and `dialog.test.tsx:59-65` pins it. The cost is that
the button is visually at top-right (`absolute right-4 top-4`, `:112`) but is the **last**
element in tab order. The trade is deliberate and defensible (initial focus matters more
than Close's tab position, and Escape remains the primary dismissal); noted so it is a
recorded decision rather than an oversight.

### NB-3 — Dialog close button has no `cursor-pointer` (non-blocking)

`dialog.tsx:112` styles `DialogPrimitive.Close` with a raw class string that omits
`cursor-pointer`. `patterns-styling.md:232` exempts native `<button>` elements on the
grounds that "native `<button>` and `<a>` elements get a pointer from the browser" — which
is the letter of the rule, and Radix's Close does render a native `<button>`. Worth
flagging anyway because the repo's own practice contradicts that premise: the shared
`<Button>` CVA adds `cursor-pointer` explicitly at `components/ui/button.tsx:7`, which it
would not need if the browser supplied one (browsers default `<button>` to
`cursor: default`).

Consistent with the reference primitive either way — `select.tsx:17` `SelectTrigger`
likewise omits it. Cosmetic; one class if you want it.

### NB-4 — `DialogFooter` is exported but unused (informational)

`dialog.tsx:169` exports `DialogFooter`, which has zero consumers across `apps/web/src`
(same for `DialogTrigger`, `DialogPortal`, `DialogOverlay`, `DialogClose` — those four are
standard shadcn re-exports of building blocks, so their being unused is expected).

`DialogFooter` is unused because `VehicleForm.tsx:179` owns its own
`<div className="flex justify-end gap-2">` button row rather than the dialog chrome's
footer. That is **correct**, not a miss: `VehicleForm` is rendered both inside the dialog
(`VehiclesPage.tsx:107`) and standalone on a page (`VehicleDetailPage.tsx:140`), so it must
not depend on dialog-only components. The canonical `DialogFooter` usage in
`patterns-forms-validation.md:222` assumes a dialog-only form, which this is not.

## Summary

### Blocking (must fix)
- None. Build, tests, and lint all pass, and every FE-* check is PASS or N/A.

### Non-Blocking (should fix)
- **(a) / D-2 — `dialog.tsx:76`**: derive `aria-modal` from the Root's `modal` prop, or
  document in the `DialogContentProps` doc comment that this primitive assumes a modal
  Root. Corroborates the existing Action Item 2.
- **NB-3 — `dialog.tsx:112`**: add `cursor-pointer` to the close button for consistency
  with `button.tsx:7`.

### Verified, No Action
- **(b)** overlay scrim token choice — correct in both themes, and required by
  `conventions.test.ts`.
- **NB-1** inert animation classes — matches the reference primitive `select.tsx:66`.
- **NB-2** close-button tab position — deliberate, tested, documented in-file.
- **NB-4** unused `DialogFooter` — correct consequence of `VehicleForm` being shared
  between dialog and page contexts.
