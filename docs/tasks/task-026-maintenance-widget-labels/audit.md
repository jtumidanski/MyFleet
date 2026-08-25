# task-026 Maintenance Queue Labels — Code Review

Three reviewers ran over `285e175..f8c4ed5` (merge-base with `main` .. HEAD).
Their detailed reports are alongside this file:

| Reviewer | Verdict | Report |
| --- | --- | --- |
| `plan-adherence-reviewer` | FULL adherence — all 6 tasks, no skips | `audit-plan-adherence.md` |
| `frontend-guidelines-reviewer` | PASS — no blocking or non-blocking findings | `audit-frontend.md` |
| Whole-branch code review (opus) | **Ready to merge: Yes** — 0 Critical, 0 Important, 5 Minor | below |

## Whole-branch review

### Verified, not merely asserted

- All three traps the design names were checked against the real source, not
  against the code's own comments: `useVehicles` genuinely has no `select`
  (`vehicles.ts:25-33`) while the maintenance hooks do (`maintenance.ts:80-88`,
  `:258-279`), so `labels.ts:62-67`'s `data?.data` is on the right side of the
  asymmetry; `||` is used in all three fallbacks with the empty-string cases
  unit-tested; `nextDueMileage` uses `? :` at all four render sites while
  `nextDueDate` correctly stays on `&&`.
- **The zero-mileage negative assertion is not vacuous.** With an `&&` bug React
  emits `0` as a direct text child of the stacked `<div>`, and Testing Library's
  `getNodeText` concatenates only direct text-node children — so that div's node
  text would be exactly `"0"` and `queryByText('0')` would match. The test
  genuinely discriminates.
- **Query dedupe holds.** Every `useMaintenanceCategories()` call site in the repo
  passes no `kind`, so all share `maintenanceCategoryKeys.list({kind: undefined})`.
  Both widgets mounted together issue one categories fetch and one vehicles
  fetch, not four.
- **The loading gate's disabled-query defence is moot in practice** —
  `DashboardPage.tsx:22-31` early-returns on a null `activeFleetId`, so a widget
  never mounts without one. `retry: 1` (`AppProviders.tsx:37`) means a failed
  supporting query settles quickly and rows come through with the fallbacks.
- **Tests exercise real code.** The widget tests use `importOriginal` and spread
  `...actual`, so `categoryLabel`/`vehicleLabel` are the real functions — the
  Unknown-* tests prove the actual fallback path, not a stubbed string.
- `f8c4ed5` is genuinely formatting-only: three prettier reflows, zero behavior
  change.

### Minor findings (all parked — see the rulings below)

1. **Truncated labels have no `title` attribute** —
   `OverdueMaintenanceWidget.tsx:55-60`, `UpcomingMaintenanceWidget.tsx:55-60`.
   The design reasoned about `min-w-0` making truncation *work*, but not about the
   user who then cannot read a long category name: no tooltip, no expansion, and
   the row is not navigable. `title={label}` on both `<p>` elements is a one-line
   mitigation.
2. **The vehicle-list fetch is not always free.** `labels.ts:11-13` justifies
   client-side resolution with "the fleet's vehicle list is already warm on the
   dashboard," but widgets are user-configurable and only `VehicleStatusWidget`
   and `MileageTrendsWidget` call `useVehicles`. A dashboard holding only the two
   maintenance widgets now issues a fleet vehicle-list fetch that did not
   previously happen, and blocks both skeletons on it. Bounded (one request,
   `staleTime` 60s, `retry: 1`), but the comment overstates the guarantee.
3. **The widget-test module mock is fragile.** The test files replace the whole
   `lib/hooks/api/maintenance` module with a one-hook factory, while the real
   `labels.ts` (loaded via `importOriginal`) imports `useMaintenanceCategories`
   from that now-stubbed module. Nothing calls it today, so it passes — but
   unmocking `useCategoryNameMap` later would fail with an opaque
   "not a function".
4. **Failed queue vs. empty queue are indistinguishable.** A failed request
   renders "No overdue maintenance.", reassuring the user that nothing is
   overdue. Pre-existing and untouched by this branch, but the most user-hostile
   failure mode on the surface.
5. **Raw UUIDs still reach the DOM on two other surfaces** —
   `UpcomingScheduleStrip.tsx:72` (note `??`, so an empty-string name renders a
   *blank* label, the exact trap this branch's `||` avoids) and
   `vehicleRecords.ts:68`. Both explicitly out of scope per the plan.

### Plan-mandated, flagged for the human's decision (not defects)

- **Duplicated widget markup** — ~35 lines byte-identical across the two widgets
  apart from four scalars. The reviewer agreed extracting speculatively before a
  third caller was the wrong bet at write time; revisit when one appears.
- **`MaintenanceQueueView` is dead code, fixed in place** — confirmed
  independently: nothing outside its own definition and its new test imports it.
  The branch adds 119 lines of test for a component no user can reach. The
  mount-or-delete follow-up should become a real ticket, not a paragraph in
  `plan.md`.
- **Only `VehicleCard` adopts `vehicleTitle()`** — five duplicate sites remain,
  one of them (`VehicleStatusWidget`) known-buggy. The `VehicleNameCrumb.test.tsx`
  source pin makes migrating them a genuinely separate change.

### Go lint

The reviewer independently agreed the `make lint-check` Go failure is
environmental: the branch changes zero Go files, and a toolchain-vs-linter
version skew failing while type-checking the stdlib is exactly the reported
symptom. Local Homebrew Go is 1.27.0; `tools/lint.versions` pins golangci-lint
v2.12.2, built with Go 1.26. Bumping the pin is a separate, repo-wide change —
but `make ci` is red on that machine for everyone until it happens.

## Recommended follow-ups

1. `title={...}` on the two truncating `<p>` elements in each widget.
2. Soften the "already warm on the dashboard" claim in `labels.ts:11-13`.
3. Open real tickets for: mount-or-delete `MaintenanceQueueView`; migrate the
   five `vehicleTitle` holdouts and convert the source pin into an "every site
   calls `vehicleTitle()`" assertion; route `UpcomingScheduleStrip` and
   `vehicleRecords.ts` through `categoryLabel` (which also fixes the `??`
   blank-label bug).
4. Bump `GOLANGCI_LINT_VERSION` in `tools/lint.versions` to a release built with
   Go 1.27.
