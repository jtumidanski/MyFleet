# task-026 Maintenance Queue Labels — Context

Companion to `plan.md`. Everything an implementer needs to know about the
surrounding code that the plan's tasks assume but do not re-derive.

**Worktree:** `/home/tumidanski/source/MyFleet/.worktrees/task-026-maintenance-widget-labels`
**Branch:** `task-026-maintenance-widget-labels`
**Scope:** `apps/web` only. No Go, no manifests, no endpoints.

---

## 1. The bug in one line

Four render sites interpolate `item.attributes.categoryId` — a UUID — as the
row's primary label. The queue endpoints return foreign keys, and nobody
resolved them.

| Site | Line today |
| --- | --- |
| `apps/web/src/components/features/dashboard/widgets/OverdueMaintenanceWidget.tsx` | 38 |
| `apps/web/src/components/features/dashboard/widgets/UpcomingMaintenanceWidget.tsx` | 38 |
| `apps/web/src/components/features/vehicles/maintenance/MaintenanceQueueView.tsx` | 51 (overdue) |
| `apps/web/src/components/features/vehicles/maintenance/MaintenanceQueueView.tsx` | 88 (upcoming) |

---

## 2. Key files to read before starting

| File | Why it matters |
| --- | --- |
| `apps/web/src/lib/hooks/api/maintenance.ts:80-88` | `useMaintenanceCategories(kind?)`. Has `select: (result) => result.data`, so it returns `MaintenanceCategory[]`. `staleTime` 10 min, `gcTime` 30 min. Query key `maintenanceCategoryKeys.list({ kind })`. |
| `apps/web/src/lib/hooks/api/maintenance.ts:258-279` | `useUpcomingMaintenanceQueue` / `useOverdueMaintenanceQueue`. Both `select` to `.data`, both `enabled: !!fleetId`. |
| `apps/web/src/lib/hooks/api/vehicles.ts:25-33` | `useVehicles(fleetId)`. **No `select`** — returns `ListResult<VehicleAttributes>`, so rows are at `data.data`. This asymmetry is the single most likely implementation mistake. |
| `apps/web/src/lib/utils/displayName.ts` | The house pattern this task copies: a pure fallback-chain helper with a `\|\|`-not-`??` warning in its header. `displayName.test.ts` is the model for `vehicleTitle.test.ts`. |
| `apps/web/src/pages/VehicleDetailPage.tsx:65-66` | The existing one-off `categoryId → name` `useMemo` map that Task 2 promotes into a shared hook. Same query key, so the two dedupe. |
| `apps/web/src/components/features/vehicles/detail/UpcomingScheduleStrip.tsx:72-73` | Consumes a caller-supplied map and falls back to the **raw id**. Deliberately left alone (FR-SHARED-4). |
| `apps/web/src/components/frame/crumbs/VehicleNameCrumb.test.tsx` | Two things: the `vi.mock` the hook module, don't stand up a QueryClient testing pattern the new component tests follow; and a source-pin assertion (lines 84-99) that must stay green untouched. |
| `apps/web/src/components/features/vehicles/maintenance/SeverityChip.tsx` | Accepts `className`, so `shrink-0` can be passed from the row. Falls back to a muted chip for an unrecognised severity. |
| `apps/web/src/services/api/BaseService.ts:4-7` | `ListResult<A> = { data: JsonApiResource<A>[]; meta?: PageMeta }`. |

---

## 3. Decisions carried in from design.md

| Decision | Rationale |
| --- | --- |
| Resolve client-side from cached queries | `packages/shared-go/server` has no `Included` field, so `?include=category` does not exist. Denormalizing names into Attributes is an explicit PRD non-goal. Category list is ~20 static rows cached 10 min; the vehicle list is warm on the dashboard from sibling widgets. |
| Each component calls the hooks itself | The widget registry mounts widgets with only `{ fleetId }`; there is no parent to prop-drill a map from. React Query dedupes the shared keys, so N widgets still issue one category and one vehicle request. |
| Two hooks, not one | `useCategoryNameMap` is fleet-independent and takes no argument; `useVehicleTitleMap` takes a `fleetId`. `MaintenanceQueueView` needs only the first — fusing them would make it fetch a vehicle list it never renders. |
| Hooks and pure label functions in one module | The map and its fallback are one contract. The functions take the map as an argument, so they stay testable without a render. |
| `useCategoryNameMap` takes no parameter | Structural enforcement of FR-LABEL-3: a caller cannot pass a `kind` and silently break `modification` categories. |
| `Unknown category` / `Unknown vehicle`, styled like a resolved value | Italics would read as emphasis, not degradation, and there is no state in which the user can act on the distinction. |
| Vehicle title is plain text, not a link | Widget rows are inert today; linking one line of one row is an inconsistent half-measure, and the `VehicleCard` row-as-link overlay is a layout change the PRD rules out. |
| Only `VehicleCard` adopts `vehicleTitle()` | See §5. |
| `isLoading`, not `isPending`, in the gate | A disabled query (`enabled: !!fleetId`) reports `isPending: true, isFetching: false` → `isLoading: false`. Using `isPending` would wedge the skeleton forever on a null `fleetId`. |

---

## 4. Three traps that will bite silently

1. **`useVehicles` has no `select`.** Reading `data` where the code means
   `data.data` produces an empty map and a widget full of `Unknown vehicle` —
   which the FR-LOAD-3 fallback makes indistinguishable from a legitimate
   lookup miss. `labels.test.ts` feeds the realistic `{ data: [...] }` envelope
   so this fails loudly.
2. **Go marshals unset strings as `""`, not `null`.** Every presence check uses
   `||`. `??` lets an empty name through and renders a blank cell.
3. **`nextDueMileage` is a Go `int` with `omitempty`.** Absent and zero are
   indistinguishable, so falsy means "no mileage line" — and the guard must be
   `? :`, never `&&`, because `&&` on a number renders a literal `0` into the row.

A fourth, layout-only: `min-w-0` on the flex left column. A flex child defaults
to `min-width: auto`, so without it `truncate` is inert and a long category name
pushes the severity chip out of the card.

---

## 5. Why the vehicle-title extraction stops at one call site

Six files duplicate the `nickname || year make model` rule — `VehicleCard.tsx:53`,
`VehicleNameCrumb.tsx:29`, `VehicleDetailPage.tsx:129`,
`VehicleIdentityRail.tsx:37`, `ActivityPage.tsx:29`, `VehicleStatusWidget.tsx:39`
— and they are not identical: `VehicleCard` appends a second `.trim()` to the
template literal, and `VehicleStatusWidget` has no `.trim()` at all.

`VehicleNameCrumb.test.tsx:84-99` reads `VehicleDetailPage.tsx` and
`VehicleNameCrumb.tsx` off disk and asserts each contains the literal expression.
Its comment says the duplication is deliberate — extracting the rule would mean
editing `VehicleDetailPage`, which task-015 owns.

So: extract the helper, migrate `VehicleCard` only, leave the pin test and the
files it reads untouched and green. `VehicleCard` is not named by the pin and
could not be, since its expression carries the extra `.trim()`. Migrating all six
is the better end state and is a separate task — it changes
`VehicleStatusWidget`'s rendered output (a `"   "` nickname currently renders as
blank space and would start rendering year/make/model) and edits a page this task
does not own.

---

## 6. `MaintenanceQueueView` is dead code

A repo-wide grep for `MaintenanceQueueView` returns only its own definition. It
is absent from `widgetRegistry.tsx`, from every page, and from every barrel. Two
of the four FR-LABEL-1 sites — and the tests written for them — exercise a
component no user can reach.

It is fixed in place anyway: the PRD names those exact lines, deletion is a scope
call the PRD did not make, and it is the reference implementation the two widgets
are being aligned to. Whether to mount or delete it is a recorded follow-up.

---

## 7. Dependencies between tasks

```
Task 1 (vehicleTitle) ──► Task 2 (labels) ──┬──► Task 3 (MaintenanceQueueView)
        │                                   ├──► Task 4 (OverdueMaintenanceWidget)
        └──► VehicleCard                    └──► Task 5 (UpcomingMaintenanceWidget)
                                                        │
                                            Task 6 (verification) ◄──┘
```

Tasks 3, 4, and 5 are mutually independent once Task 2 lands and may be worked in
parallel. Task 6 requires all five.

---

## 8. Verification commands

```sh
# Node is not always on PATH:
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22

npm run -w apps/web test -- <path>   # single file, during TDD
npm run -w apps/web test             # apps/web suite
make fe-test                         # apps/web + shared-ts + ui-components
make fe-build
make ci                              # the gate: lint-check vet test build fe-test fe-build manifests carfax-template
```

No `kustomize build` beyond what `make ci`'s `manifests` target runs — this task
changes no manifest.

Before the PR: `superpowers:requesting-code-review`
(`frontend-guidelines-reviewer` at minimum, plus `plan-adherence-reviewer`),
findings recorded in `docs/tasks/task-026-maintenance-widget-labels/audit.md`.
