# Frontend Audit — task-026-maintenance-widget-labels

- **Audit Scope:** Changed TS/React files in range `285e175..f8c4ed5` (11 code files, per `git diff --stat 285e175..f8c4ed5 -- apps/web/src`)
- **Guidelines Source:** frontend-dev-guidelines skill (all 12 files under `resources/` read in full)
- **Date:** 2026-08-25
- **Build:** PASS
- **Tests:** 843 passed, 0 failed (824 apps/web + 9 packages/shared-ts + 10 packages/ui-components), re-run live, not just cited from the task description
- **Overall:** PASS

## Build & Test Results

```
make fe-test
  apps/web:            Test Files  101 passed (101)  Tests  824 passed (824)
  packages/shared-ts:  Test Files    2 passed (2)     Tests    9 passed (9)
  packages/ui-components: Test Files 1 passed (1)     Tests   10 passed (10)

make fe-build
  tsc -b && vite build → succeeded, dist/ emitted, no type errors
```

## File Inventory

- `apps/web/src/lib/utils/vehicleTitle.ts` (new) — Helper (pure function, `lib/utils/`)
- `apps/web/src/lib/utils/vehicleTitle.test.ts` (new) — Test
- `apps/web/src/lib/hooks/api/labels.ts` (new) — Hook module (composes existing hooks; no `apiClient`/service access of its own)
- `apps/web/src/lib/hooks/api/labels.test.ts` (new) — Test
- `apps/web/src/components/features/vehicles/VehicleCard.tsx` (modified) — Component
- `apps/web/src/components/features/vehicles/maintenance/MaintenanceQueueView.tsx` (modified) — Component (confirmed dead code: `grep -rln "MaintenanceQueueView" apps/web/src` → only itself and its own test)
- `apps/web/src/components/features/vehicles/maintenance/MaintenanceQueueView.test.tsx` (new) — Test
- `apps/web/src/components/features/dashboard/widgets/OverdueMaintenanceWidget.tsx` (modified) — Component (confirmed live: imported by `widgetRegistry.tsx`)
- `apps/web/src/components/features/dashboard/widgets/OverdueMaintenanceWidget.test.tsx` (new) — Test
- `apps/web/src/components/features/dashboard/widgets/UpcomingMaintenanceWidget.tsx` (modified) — Component (confirmed live: imported by `widgetRegistry.tsx`)
- `apps/web/src/components/features/dashboard/widgets/UpcomingMaintenanceWidget.test.tsx` (new) — Test

Verified scope is exhaustive: `git diff --stat 285e175..f8c4ed5 -- apps/web/src` lists exactly these 11 files, 837 insertions / 16 deletions. No other `apps/web/src` file changed — confirms the plan's claims that `UpcomingScheduleStrip.tsx` and `VehicleNameCrumb.test.tsx` are untouched.

## Anti-Pattern Checklist

Grep run against all 11 files as an array (`"${FILES[@]}"`, zsh does not word-split an unquoted scalar, so this needed an explicit array — noted for reproducibility):

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-01 | No `any` type | PASS | `grep -n ": any\|as any" <11 files>` → zero matches |
| FE-02 | No manual class concatenation | PASS | `grep -nE "className=\{[\"'].*\+\|className=\{\`" <11 files>` → zero matches; all `className` uses are plain string literals or `cn(...)` (e.g. `VehicleCard.tsx:81-94`) |
| FE-03 | No direct API client/service calls in components | PASS | `grep -nE "from '(\.\./)+lib/api/client'\|from '(\.\./)+services/api/" <11 files>` → zero matches. `labels.ts:2-3` imports only `useMaintenanceCategories` (from `./maintenance`) and `useVehicles` (from `./vehicles`) — hooks, not services or `apiClient`. Layering is intact: component → `labels.ts` hooks → existing resource hooks → service → client. |
| FE-04 | No inline Zod schemas | PASS | `grep -n "z\.object(\|z\.string(" <11 files>` → zero matches (no forms touched in this branch) |
| FE-05 | No spinners for content loading | PASS | `grep -n "animate-spin" <11 files>` → zero matches. All three components use `<Skeleton>` for the loading branch (`MaintenanceQueueView.tsx:30-36`, `OverdueMaintenanceWidget.tsx:27-35`, `UpcomingMaintenanceWidget.tsx:27-35`); tests assert `.animate-pulse` (Skeleton's own class) is present, e.g. `OverdueMaintenanceWidget.test.tsx:142-145`. |
| FE-06 | No hardcoded colors | PASS | Executable check — `make fe-test` includes `src/test/conventions.test.ts:113-115`'s palette scan over every `.tsx` in `apps/web/src`, confirmed green in this run. New color-bearing classes introduced are all semantic tokens (`text-danger`, `text-warning`, `text-muted-foreground`, `border-danger-border`, `bg-danger-subtle` — `OverdueMaintenanceWidget.tsx:41,56-70`, `MaintenanceQueueView.tsx:45,53,84`). |
| FE-07 | No state mutation | PASS | `grep -n "\.push(\|\.splice(\|\.sort(" <11 files>` → zero matches. `labels.ts:45-48,63-67` builds `Map`s via `useMemo` from `.map(...)` over query data, never mutating the cached array/objects. |
| FE-08 | No default exports | PASS | `grep -n "export default" <11 files>` → zero matches; every component/function is a named export (`VehicleCard`, `MaintenanceQueueView`, `OverdueMaintenanceWidget`, `UpcomingMaintenanceWidget`, `vehicleTitle`, `useCategoryNameMap`, `useVehicleTitleMap`, `categoryLabel`, `vehicleLabel`). |
| FE-09 | Error handling via `createErrorFromUnknown` | OUT-OF-SCOPE | `grep -n "catch\|createErrorFromUnknown" <11 files>` → zero matches. This branch touches only read-side query hooks and presentational components; no `try/catch` or `.catch(` exists anywhere in the 11 files, so there is no async-failure-swallowing subject to check. A failed query in these files settles to `data: undefined` and the components' own fallback logic (`categoryLabel`/`vehicleLabel` → `Unknown category`/`Unknown vehicle`) handles it, which is React Query's own error-to-`undefined` contract, not a swallowed `catch`. |

## Architecture Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-10 | JSON:API model shape | OUT-OF-SCOPE | No files under `types/models/` changed in this branch (`git diff --stat` above lists no `types/models/*`). |
| FE-11 | Service reaches network only via `apiClient` | OUT-OF-SCOPE | No files under `services/api/` changed in this branch. |
| FE-12 | Query key factory uses `as const` | OUT-OF-SCOPE (for `labels.ts`), N/A distinction explained | `grep -n "as const" <11 files>` → zero matches. `labels.ts` declares no query key factory of its own — `useCategoryNameMap`/`useVehicleTitleMap` call the *existing* `useMaintenanceCategories()`/`useVehicles()` hooks (`labels.ts:44,62`), which already own their `as const` key factories in `maintenance.ts`/`vehicles.ts` (unchanged files, out of this diff). There is nothing here for FE-12 to check — `labels.ts` is a composition layer, not a new resource module, so the check has no subject rather than a silently-passing one. |
| FE-13 | Forms use react-hook-form + zodResolver | OUT-OF-SCOPE | No `useForm` call sites in the 11 files; no form component touched. |
| FE-14 | Schema in `lib/schemas/` with inferred type | OUT-OF-SCOPE | No files under `lib/schemas/` changed. |

Additional architecture note (not a checklist ID, but load-bearing for this branch): `useVehicles` has no `select`, so `useVehicleTitleMap` reads `data?.data` (`labels.ts:65`), confirmed against `lib/hooks/api/vehicles.ts` — `useVehicles` returns the raw `ListResult<VehicleAttributes>` envelope, unlike `useMaintenanceCategories`, which does have a `select` and is read directly as `data` (`labels.ts:46`, `data ?? []`). Both read paths are exercised by dedicated test cases (`labels.test.ts:62-95` for the categories map, `labels.test.ts:117-147` for the vehicle map, which explicitly nests `data: { data: [...] }` in its mock — comment at `labels.test.ts:114-116` calls this out as a guard against exactly this projection mismatch).

## Styling Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-15 | Interactive elements show `cursor-pointer` | OUT-OF-SCOPE | `grep -n "onClick\|cursor-pointer\|PopoverTrigger\|DialogTrigger" <11 files>` → zero matches. None of the changed/added markup is interactive: `MaintenanceQueueView`'s and both widgets' rows are static `<div>`/`<li>` display rows with no click handler, no trigger. `VehicleCard`'s existing interactive elements (`<Link data-card-link>`, the Carfax `<a>`) are unchanged by this diff (the only `VehicleCard.tsx` edit is the `vehicleTitle()` import/call, per `git diff` above) and were not touched here. |

## Testing Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-16 | Tests exist for changed components | PASS | Every non-trivial change has a sibling test: `vehicleTitle.ts` ↔ `vehicleTitle.test.ts`, `labels.ts` ↔ `labels.test.ts`, `MaintenanceQueueView.tsx` ↔ `MaintenanceQueueView.test.tsx` (new), `OverdueMaintenanceWidget.tsx` ↔ `OverdueMaintenanceWidget.test.tsx` (new), `UpcomingMaintenanceWidget.tsx` ↔ `UpcomingMaintenanceWidget.test.tsx` (new). `VehicleCard.tsx`'s change (swap an inline expression for `vehicleTitle()`, `VehicleCard.tsx:19,54`) is already covered by its pre-existing `VehicleCard.test.tsx:252-253` nickname assertion, which exercises the same code path unchanged. |
| FE-17 | `vi.mock` call sites updated when a service changes | OUT-OF-SCOPE | No file under `services/api/` changed in this branch, so there is no service whose mock could have gone stale. The `vi.mock` calls added in this branch (`labels.test.ts:18-23`, `MaintenanceQueueView.test.tsx:10-18`, `OverdueMaintenanceWidget.test.tsx:10-21`, `UpcomingMaintenanceWidget.test.tsx:10-21`) all target hook modules (`./maintenance`, `./vehicles`, `./labels`), not services, and are new mocks for new/changed subjects rather than updates to a pre-existing stub. |

### Test-quality check (requested specifically): real assertions vs. self-mocked vacuity

Verified this is not vacuous. The three component test files (`MaintenanceQueueView.test.tsx`, `OverdueMaintenanceWidget.test.tsx`, `UpcomingMaintenanceWidget.test.tsx`) use `vi.mock('.../labels', async (importOriginal) => ({ ...actual, useCategoryNameMap: ... }))` — this mocks only the *hook* (`useCategoryNameMap`/`useVehicleTitleMap`) while re-exporting the real `categoryLabel`/`vehicleLabel` functions via `importOriginal` (`MaintenanceQueueView.test.tsx:15-18`, `OverdueMaintenanceWidget.test.tsx:14-21`). The component under test genuinely calls the real `categoryLabel`/`vehicleLabel` at render time; the mock only removes the network/React-Query dependency the hook would otherwise need. The negative assertions (`expect(screen.queryByText(CATEGORY_ID)).not.toBeInTheDocument()`) are non-vacuous because `CATEGORY_ID`/`VEHICLE_ID` are UUID literals that appear nowhere in the component's static markup — the only way they could appear is if the real label-resolution logic were bypassed, which is exactly the regression these tests exist to catch (confirmed by reading `MaintenanceQueueView.tsx:57-59`, `OverdueMaintenanceWidget.tsx:56,59` — the id is only ever passed *into* `categoryLabel`/`vehicleLabel`, never rendered directly).

Plain `render()` (not `renderWithProviders`) is used in all three component test files. This is correct per `testing-guide.md`'s "A component with no hooks and no links may use plain `render()`" rule *as applied here*: every hook the subject calls (`useUpcomingMaintenanceQueue`, `useOverdueMaintenanceQueue`, `useCategoryNameMap`, `useVehicleTitleMap`) is mocked out via `vi.mock`, so no real `QueryClientProvider` is needed, and none of the three components render a `<Link>`.

## Summary

### Blocking (must fix)

None. Zero FAIL checks found across all applicable rows.

### Non-Blocking (should fix)

None found. Two items worth recording as informational, not defects:

- `MaintenanceQueueView` remains dead code after this branch (confirmed: only its own test imports it) — this is explicitly called out in the plan/design as a known, deliberate follow-up, not something this branch was asked to fix.
- `labels.ts` sits in `lib/hooks/api/` alongside per-resource hook modules but is not itself a resource module (no query key factory, no `apiClient`/service dependency) — a naming/placement choice the plan and design docs discuss explicitly as deliberate; flagged here only for the next reader's benefit, not as a violation of any FE-* rule as written.
