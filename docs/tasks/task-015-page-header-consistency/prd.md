# Page Header & Layout Consistency — Product Requirements Document

Version: v1
Status: Draft
Created: 2026-08-02
---

## 1. Overview

The MyFleet web UI has accumulated per-page styling drift. Every authenticated page renders its own page title and its own header-to-content spacing with hand-written Tailwind classes, and no two pages agree. The most visible symptom is the Dashboard, whose title renders at `text-lg` while every other page uses `text-2xl`. Less visible but equally inconsistent is the gap between the page title and the content beneath it, which ranges from 16px to 32px depending on which page you are on.

Investigation of the current `main` turned up two defects beyond cosmetic drift. First, the Dashboard's title is an `<h2>`, not an `<h1>` — the dashboard route has no `<h1>` at all, which breaks the document outline for screen-reader and landmark navigation. Second, that title is rendered inside an `isOwner &&` guard in `DashboardGrid.tsx`, so users with the `member` or `viewer` role see the dashboard with **no page title whatsoever**. Both are user-facing bugs, not just inconsistencies.

The root cause is structural: there is no shared page-header primitive. Each page reimplements the same three concerns (title, optional action buttons, spacing to content) inline, so each new page is free to invent its own values — and each has. This task introduces a single `PageHeader` component, adopts it across every authenticated page, and codifies the spacing and content-width rules so future pages inherit the standard rather than re-deriving it.

## 2. Goals

Primary goals:

- Every authenticated page renders its title through one shared `PageHeader` component.
- Page titles are visually identical across all pages: `text-2xl font-semibold`, rendered as a semantic `<h1>`.
- The gap between page title and page content is exactly 24px (`space-y-6`) on every page.
- The Dashboard title is an `<h1>` and is visible to every role, not just owners.
- Loading, empty, and error states of a page use the same header treatment and spacing as that page's loaded state.
- Content max-width behaviour is an explicit, documented property of the page rather than an accident of copy-paste.
- Section-level (`<h2>`) headings use one consistent scale.

Non-goals:

- No redesign of page content, widgets, cards, tables, or forms. This task changes headers, spacing, and heading semantics only.
- No change to the app shell (sidebar, top bar, `<main>` padding). `AppLayout` already applies a consistent `p-6` and is correct as-is.
- No change to `LoginPage`, `OnboardingPage`, or `InviteAcceptPage`. These are unauthenticated/pre-fleet centered-card layouts outside the `AppLayout` shell and do not have page headers.
- No new colours, no typography scale changes beyond the heading sizes named here, no dark-mode work.
- No routing, data-fetching, state-management, or API changes.
- Not a design-system extraction into `packages/ui-components`. `PageHeader` is app-layout-specific and lives in `apps/web`.

## 3. User Stories

- As a user navigating between Dashboard, Vehicles, Activity, Notifications and Settings, I want every page title to look the same so the app feels like one product rather than several.
- As a member or viewer opening the Dashboard, I want to see the page title so I know where I am — today I see none.
- As a screen-reader user, I want each page to expose exactly one `<h1>` naming the page so I can orient myself with heading navigation.
- As a user, I want the space between a page title and its content to be the same everywhere so pages do not appear to shift when I navigate.
- As a user watching a page load, I want the header to stay put between the skeleton and loaded states rather than jumping.
- As a developer adding a new page, I want a `PageHeader` component to drop in so I do not have to guess the title size or spacing.

## 4. Functional Requirements

### 4.1 `PageHeader` component

**FR-1.** A new component `PageHeader` is added at `apps/web/src/components/PageHeader.tsx`, alongside the existing shell components (`AppLayout.tsx`, `RequireAuth.tsx`).

**FR-2.** Its props are:

| Prop | Type | Required | Meaning |
|---|---|---|---|
| `title` | `string` | yes | Page title text, rendered as the page's sole `<h1>` |
| `actions` | `ReactNode` | no | Right-aligned controls (buttons, menus) on the title row |
| `className` | `string` | no | Merged via `cn()` for per-page escape hatches |

**FR-3.** The title renders as `<h1 className="text-2xl font-semibold">{title}</h1>`.

**FR-4.** When `actions` is provided, the header is a flex row with the title left and actions right, vertically centered, with a `gap-2` minimum so a long title never collides with the actions. When `actions` is absent or falsy, no empty flex child is rendered.

**FR-5.** `PageHeader` renders only the header row. It does not render the surrounding page container and does not own the gap to the content; the gap is supplied by the page container (FR-7) so a single `space-y-6` governs the whole vertical rhythm.

**FR-6.** All conditional class composition uses the `cn()` helper. No manual string concatenation.

### 4.2 Page container and spacing

**FR-7.** Every authenticated page's root element is a `<div>` carrying `space-y-6`, producing a uniform 24px gap between the header and the first content block, and between subsequent top-level blocks.

**FR-8.** Ad-hoc spacing that exists purely to compensate for a missing container gap is removed. Specifically, the `mt-6` on `VehiclesPage`'s form card and vehicle list is deleted, since `space-y-6` on the container now supplies that spacing.

**FR-9.** Content max-width is expressed once per page on the page container, not scattered across children. The rule is:

- **Full width** (no max-width) for list, grid and feed pages: Dashboard, Vehicles, Activity, Notifications.
- **`max-w-2xl`** for form- and detail-centric pages: Settings, Vehicle Detail.

This preserves each page's current effective width; the requirement is that the constraint is declared consistently on the container rather than inconsistently inside branches.

**FR-10.** A page's max-width is identical across its loading, empty, error and loaded states.

### 4.3 Per-page adoption

**FR-11.** The following pages adopt `PageHeader` and the FR-7 container:

| Page | File | Title | Actions |
|---|---|---|---|
| Dashboard | `pages/DashboardPage.tsx` | `Dashboard` | Add Widget (owners only) |
| Vehicles | `pages/VehiclesPage.tsx` | `Vehicles` | Add Vehicle (owner/member, when form hidden) |
| Activity | `pages/ActivityPage.tsx` | `Activity` | none |
| Notifications | `pages/NotificationsPage.tsx` | `Notifications` | none |
| Settings | `pages/SettingsPage.tsx` | `Fleet Settings` | none |
| Vehicle Detail | `pages/VehicleDetailPage.tsx` | vehicle title (dynamic) | existing detail actions |

**FR-12.** The Dashboard page title moves out of `DashboardGrid.tsx` and into `DashboardPage.tsx`. `DashboardGrid` no longer renders a page title.

**FR-13.** The Dashboard title renders for **every** role. Only the *Add Widget* control remains gated behind `isOwner`; it is passed to `PageHeader` as `actions`.

**FR-14.** `DashboardPage`'s no-active-fleet branch uses the same `PageHeader` and container as its loaded branch, differing only in body content.

**FR-15.** `SettingsPage`'s loading branch uses the same title (`Fleet Settings`), the same `space-y-6` container and the same `max-w-2xl` as its loaded branch. The current divergence — a `Settings` title with `space-y-4` while loaded shows `Fleet Settings` with `space-y-8` — is eliminated, including the title-text mismatch.

**FR-16.** `VehicleDetailPage`'s loading and error branches render `PageHeader` with the same container as the loaded branch.

**FR-17.** After adoption, no page file contains a hand-written `<h1>` for its page title. Grepping `apps/web/src/pages` for `<h1` returns no page-title matches.

### 4.4 Section headings

**FR-18.** Section-level headings within a page use `<h2 className="text-base font-semibold">` — matching Settings' current treatment, which is the only page with true non-card section headings.

**FR-19.** Section headings do not carry their own bottom margin (`mb-4`); vertical rhythm inside a section comes from that section's own `space-y-*` container, consistent with FR-7's approach.

**FR-20.** `CardTitle` usages are left alone. Cards are a separate primitive with their own internal spacing and are out of scope.

## 5. API Surface

No API surface changes. This task is presentation-layer only: no endpoints are added, modified, or removed, and no request or response shape changes.

The only new "interface" is the `PageHeader` component contract defined in FR-2.

## 6. Data Model

No data model changes. No entities, fields, relationships, constraints, or migrations are affected.

## 7. Service Impact

| Service | Impact |
|---|---|
| `apps/web` | All changes. New `PageHeader` component; six page files refactored; `DashboardGrid.tsx` loses its title block. |
| `apps/auth-service` | None |
| `apps/fleet-service` | None |
| `apps/media-service` | None |
| `apps/notification-service` | None |
| `packages/ui-components` | None — `PageHeader` is app-layout-specific and stays in `apps/web` |
| `packages/shared-ts` | None |
| `deploy/` | None |

### Files touched

**Added**
- `apps/web/src/components/PageHeader.tsx`
- `apps/web/src/components/PageHeader.test.tsx`

**Modified**
- `apps/web/src/pages/DashboardPage.tsx`
- `apps/web/src/pages/VehiclesPage.tsx`
- `apps/web/src/pages/ActivityPage.tsx`
- `apps/web/src/pages/NotificationsPage.tsx`
- `apps/web/src/pages/SettingsPage.tsx`
- `apps/web/src/pages/VehicleDetailPage.tsx`
- `apps/web/src/components/features/dashboard/DashboardGrid.tsx`

### Coordination risk

Three task worktrees are in flight against overlapping files and will need to rebase onto this work:

- `task-012-vehicle-detail-redesign` — actively rewriting `VehicleDetailPage.tsx`. Highest conflict likelihood. Its redesign should adopt `PageHeader` when it rebases.
- `task-014-member-names-ownership-transfer` — rewrites the Settings **Members** card (display names, ownership transfer, member removal). Overlaps `SettingsPage.tsx`, which this task also modifies for FR-15 (loading branch) and FR-18/FR-19 (section headings). The two changes are in different regions of the file — this task touches the page container and `<h2>` elements, task-014 touches the Members card's contents — so conflicts should be mechanical. Whichever lands second must ensure any section heading it adds follows FR-18.
- `task-011-platform-admin-console` — adding new pages. Any new page it introduces must use `PageHeader` rather than a hand-written `<h1>`.

The decision on record is to perform the full sweep now and let those branches resolve conflicts at rebase time.

## 8. Non-Functional Requirements

**Accessibility**

- Every authenticated page exposes exactly one `<h1>`, and it names the page.
- Heading levels descend without gaps: page `<h1>`, sections `<h2>`.
- The Dashboard's title is not conditional on role — a non-owner must not land on an untitled page.
- No change to existing focus order or keyboard behaviour.

**Performance**

- `PageHeader` is a pure presentational function component with no state, effects, hooks, or data fetching. Its render cost is negligible and it introduces no additional re-render triggers.
- No change to bundle size beyond the component itself; the refactor is net-neutral to slightly negative on total markup.

**Visual regression**

- Aside from the intended changes (Dashboard title size/level/visibility, per-page gap normalisation to 24px, Settings loading-state title text), no page's visual appearance changes.
- Content max-width per page is unchanged in effect (FR-9).

**Code quality**

- TypeScript strict mode; no `any`.
- Conditional classes via `cn()` only.
- Follows the presentational-component conventions in the `frontend-dev-guidelines` skill.

**Observability**

- No logging, metrics, or tracing changes.

## 9. Open Questions

1. **`max-w-2xl` for Vehicle Detail.** FR-9 preserves the current `max-w-2xl` on Vehicle Detail, but `task-012-vehicle-detail-redesign` may widen that page as part of its redesign. If task-012 lands first, its chosen width wins and FR-9's table should be updated rather than reverting task-012's decision.

2. **Settings title text.** FR-15 standardises Settings on `Fleet Settings` (the loaded-state text) over `Settings` (the loading-state text), since the loaded state is what users see the vast majority of the time. Worth a sanity check that `Fleet Settings` is the desired label given the sidebar nav item reads `Settings`.

3. **Assumption — no `width` prop.** This PRD deliberately keeps max-width on the page container rather than adding a `width` variant prop to `PageHeader`, because `PageHeader` renders only the header row (FR-5) and does not own the container. If a later task extracts a full `PageContainer`, the width rule in FR-9 is the natural thing to encode there.

## 10. Acceptance Criteria

**Component**

- [ ] `apps/web/src/components/PageHeader.tsx` exists and exports `PageHeader`.
- [ ] It accepts `title`, optional `actions`, and optional `className`, per FR-2.
- [ ] It renders the title as `<h1 className="text-2xl font-semibold">`.
- [ ] With `actions`, title and actions sit on one row, title left, actions right.
- [ ] Without `actions`, no empty flex child is rendered.
- [ ] `className` is merged via `cn()`.

**Titles**

- [ ] All six authenticated pages render their title via `PageHeader`.
- [ ] Grepping `apps/web/src/pages` for `<h1` yields no page-title matches (FR-17).
- [ ] `DashboardGrid.tsx` no longer contains a `Dashboard` title element.
- [ ] Every authenticated route renders exactly one `<h1>`.
- [ ] The Dashboard title is visible when `role` is `owner`, `member`, and `viewer`.
- [ ] The *Add Widget* control still appears only for `owner`.

**Spacing and width**

- [ ] Every authenticated page's root container carries `space-y-6`.
- [ ] No `space-y-4` or `space-y-8` remains as a page-root container on these pages.
- [ ] `VehiclesPage` no longer uses `mt-6` to space its form card or list.
- [ ] Dashboard, Vehicles, Activity and Notifications are full-width; Settings and Vehicle Detail carry `max-w-2xl` on the container.
- [ ] A page's max-width is identical across loading, empty, error and loaded states.

**States**

- [ ] `DashboardPage`'s no-fleet branch uses the same header and container as its loaded branch.
- [ ] `SettingsPage`'s loading branch shows `Fleet Settings` with `space-y-6` and `max-w-2xl`.
- [ ] `VehicleDetailPage`'s loading and error branches render `PageHeader` with the loaded-state container.
- [ ] Navigating to a page and watching it load produces no visible vertical shift of the title.

**Section headings**

- [ ] Settings section headings are `<h2 className="text-base font-semibold">` with no `mb-4`.
- [ ] Spacing within Settings sections is unchanged in effect after removing `mb-4`.
- [ ] `CardTitle` usages are untouched.

**Verification**

- [ ] `PageHeader.test.tsx` covers: title renders as `h1`; actions render when provided; no actions container when absent; `className` merges.
- [ ] Existing page tests still pass, updated only where they asserted on old header markup.
- [ ] `make fe-test` passes, with output reported.
- [ ] `make fe-build` passes, with output reported.
- [ ] `make ci` passes.
- [ ] Manual pass over all six pages in both light and dark mode confirms identical title size and identical title-to-content gap.
