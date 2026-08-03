# Page Header & Layout Consistency — Design

Version: v1
Status: Proposed
Created: 2026-08-02
Source PRD: `prd.md` (approved)
Audit: `current-state-audit.md`

---

## 1. Summary

One new presentational component, `PageHeader`, becomes the single owner of page-title markup. Six page files adopt it and standardise on a `space-y-6` root container with an explicitly declared max-width. `DashboardGrid` gives up its title.

The work is almost entirely mechanical. Two places are not: the Dashboard's *Add Widget* control (whose state lives in the component the title is leaving) and the Vehicle Detail header (which is a composite of title + status badge + subtitle + three buttons, not a bare title). Those two are the substance of this document; everything else is a search-and-replace with a decision table attached.

## 2. Architecture

### 2.1 Component boundary

```
AppLayout                     <main className="flex-1 p-6">   ← unchanged, out of scope
  └─ <Page>                   <div className="space-y-6 [max-w-2xl]">   ← page owns container + width
       ├─ PageHeader          <h1> + optional adornment/description/actions   ← new, header row only
       └─ …content blocks     spaced by the container's space-y-6
```

`PageHeader` renders **only the header row**. It does not render the container, does not own the gap to the content, and knows nothing about width (PRD FR-5). The page container supplies the 24px rhythm via a single `space-y-6`, which also governs the gaps between subsequent top-level blocks. This is the whole reason the design does not introduce a `PageContainer` wrapper — see §6.1.

`PageHeader` lives at `apps/web/src/components/PageHeader.tsx`, alongside `AppLayout.tsx` and `RequireAuth.tsx`. It is app-shell furniture, not a reusable design-system primitive, so it stays out of `packages/ui-components` (PRD §7).

### 2.2 `PageHeader` contract

```tsx
interface PageHeaderProps {
  /** Page title. Rendered as the page's sole <h1>. */
  title: string;
  /** Inline element sitting immediately right of the title (e.g. a StatusBadge). */
  titleAdornment?: ReactNode;
  /** Secondary line beneath the title, muted. */
  description?: ReactNode;
  /** Right-aligned controls on the title row. */
  actions?: ReactNode;
  /** Per-page escape hatch; merged via cn(). */
  className?: string;
}
```

```tsx
export function PageHeader({
  title,
  titleAdornment,
  description,
  actions,
  className,
}: PageHeaderProps) {
  return (
    <div
      className={cn(
        'flex justify-between gap-2',
        description ? 'items-start' : 'items-center',
        className,
      )}
    >
      <div className="min-w-0">
        <div className="flex items-center gap-3">
          <h1 className="text-2xl font-semibold">{title}</h1>
          {titleAdornment}
        </div>
        {description && <p className="mt-1 text-sm text-muted-foreground">{description}</p>}
      </div>
      {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
    </div>
  );
}
```

Notes on the shape:

- **The outer element is always a flex row**, even with no actions. FR-4 requires flex *when* actions exist and forbids an empty flex child when they don't; it does not require branching the container. A single code path is simpler and, with one left-aligned child, renders identically to a plain block. The `actions &&` guard is what satisfies "no empty flex child".
- **`items-start` vs `items-center` is derived from `description`**, not passed in. A single-line title against a 36px `Button` needs `items-center` or the button rides 2px high (today's Vehicles page). A two-line title block against buttons needs `items-start` (today's Vehicle Detail). Deriving it means neither page has to remember. `cn()` runs `tailwind-merge`, so a caller passing `className="items-start"` still wins if a future page needs to override.
- **`min-w-0` + `shrink-0`** are what actually stop a long title from colliding with the actions; `gap-2` alone only guarantees the gap once flex has decided who shrinks.
- `title` stays a `string`, not a `ReactNode`, so the `<h1>`'s accessible name is exactly the page name (PRD §8, Accessibility). Anything else that must sit on the title line goes through `titleAdornment` and stays outside the heading.

### 2.3 Deviation from FR-2: two extra optional props

FR-2 specifies three props (`title`, `actions`, `className`). This design adds `titleAdornment` and `description`, both optional, both consumed only by Vehicle Detail today.

The reason is Vehicle Detail's existing header, which is not a title-plus-buttons row:

```
[ Vehicle Nickname  (Sold) ]                    [ Edit ] [ Delete ] [ Restore ]
[ 2019 Subaru Outback Limited ]
```

The alternatives, and why they lose:

| Option | Verdict |
|---|---|
| Widen `title` to `ReactNode`; Vehicle Detail passes a fragment | Rejected. The badge text ("Sold") lands inside the `<h1>` accessible name, and the NFR is that the `<h1>` *names the page*. Also breaks FR-3's literal markup. |
| Leave the badge and subtitle as siblings *below* the header | Rejected. The badge drops off the title line — a visual regression the PRD explicitly forbids ("no page's visual appearance changes"). |
| Vehicle Detail keeps hand-written `<h1>` markup | Rejected. Violates FR-11 and FR-17 outright, and Vehicle Detail is the page most likely to drift again. |
| **Two optional slot props** | **Chosen.** Costs two lines of interface, keeps all six pages on one component, preserves current pixels and heading semantics. |

This is a widening of the FR-2 table, not a contradiction of it: every prop FR-2 names keeps its exact meaning and type. Flagged here so it is a decision on record rather than an implementation liberty.

## 3. The Dashboard problem

### 3.1 What makes it hard

FR-13 requires the *Add Widget* control to be passed to `PageHeader` as `actions`, which means it must be rendered by `DashboardPage`. But everything the button needs lives in `DashboardGrid`: the `showAddMenu` open/close state, the `widgets` array (to compute the next layout and to grey out already-placed types), and `save()` (the local-state-plus-mutation writer).

The title move is trivial. Moving the button means moving state across a component boundary.

### 3.2 Options considered

**Option A — lift widget state into a hook. (Chosen.)**

Extract `DashboardGrid.tsx:58–124` verbatim into `apps/web/src/components/features/dashboard/useDashboardWidgets.ts`:

```ts
export function useDashboardWidgets(fleetId: string): {
  widgets: GridWidget[];
  isLoading: boolean;
  addWidget: (type: WidgetType) => void;
  removeWidget: (id: string) => void;
  moveUp: (idx: number) => void;
  moveDown: (idx: number) => void;
};
```

Extract the dropdown into `AddWidgetMenu.tsx`, which owns `showAddMenu` locally and takes `{ placedTypes, onAdd }`. `DashboardPage` calls the hook once and wires both children:

```tsx
const grid = useDashboardWidgets(activeFleetId);

return (
  <div className="space-y-6">
    <PageHeader
      title="Dashboard"
      actions={
        isOwner && (
          <AddWidgetMenu
            placedTypes={grid.widgets.map((w) => w.type)}
            onAdd={grid.addWidget}
          />
        )
      }
    />
    <DashboardGrid {...grid} isOwner={isOwner} fleetId={activeFleetId} />
  </div>
);
```

`DashboardGrid` becomes a props-driven renderer of the widget list with no data fetching of its own. `GridWidget`, `toGridWidget` and `toWidgetInputs` move to the hook file; `DashboardGrid` imports the type.

Cost: two new files and a changed `DashboardGridProps`. The PRD's "Files touched" list did not anticipate them (it expected `DashboardGrid.tsx` to merely lose a title block), so this is an additive deviation worth naming. Benefit: FR-13 is satisfied literally, and the widget-list logic ends up somewhere testable without rendering a grid.

**Option B — leave the button in `DashboardGrid`.** `DashboardPage` renders `<PageHeader title="Dashboard" />` with no actions; `DashboardGrid` keeps a toolbar row containing only the right-aligned Add Widget button, still behind `isOwner`. Zero new files, no state moves. Rejected: it produces two stacked rows where the design calls for one, so the button sits *below* the title instead of beside it — a visible layout change on the page this task exists to fix, and a direct miss on FR-13.

**Option C — render-prop or portal so the grid injects into the header.** Rejected on principle; it is indirection bought to avoid a 60-line extraction.

**Option D — lift only `showAddMenu`, keep the list in the grid, expose `addWidget` via an imperative handle.** Rejected: `useImperativeHandle` for what is plainly parent-owned state is the worst of both worlds.

### 3.3 Consequential cleanups on the Dashboard

- The `isOwner &&` wrapper around the toolbar disappears from `DashboardGrid` entirely. The role gate moves to the `actions` expression in `DashboardPage`, which is the *only* thing that stays owner-gated (FR-13).
- `DashboardGrid`'s loading branch (`DashboardGrid.tsx:126–134`) currently renders `<Skeleton className="h-8 w-48" />` as a stand-in for the title. That skeleton must go — the real title is now always rendered by the page, and leaving the shimmer would reintroduce the exact vertical jump the acceptance criteria forbid. The two card skeletons stay.
- With the toolbar gone, `DashboardGrid`'s outer `<div className="space-y-4">` wraps a single child. Drop the wrapper and return the widget list (or the empty-state panel) directly. The inter-widget `gap-4` inside the grid is untouched — FR-7 governs the page root, not intra-component layout.

## 4. Per-page decision table

| Page | Container | Header | Notes |
|---|---|---|---|
| Dashboard | `space-y-6` | `title="Dashboard"`, actions = `AddWidgetMenu` when `isOwner` | Title now unconditional. No-fleet branch (`DashboardPage.tsx:12–21`) gets the identical container + header, body text only differs (FR-14). |
| Vehicles | `space-y-6` | `title="Vehicles"`, actions = Add Vehicle when `canWrite && !showForm` | Delete `mt-6` from the form `Card` and delete the `<div className="mt-6">` wrapper around `VehicleList` — the wrapper exists only to carry that margin, so `VehicleList` becomes a direct child (FR-8). |
| Activity | `space-y-6` | `title="Activity"` | Already conformant; header swap only. |
| Notifications | `space-y-6` | `title="Notifications"` | Already conformant; header swap only. |
| Settings | `space-y-6 max-w-2xl` | `title="Fleet Settings"` | `space-y-8` → `space-y-6`; see §5.1. Both branches. |
| Vehicle Detail | `max-w-2xl space-y-6` | `title` dynamic, `titleAdornment={status && <StatusBadge/>}`, `description` = year/make/model/trim, actions = Edit/Delete/Restore | Loading and error branches adopt the same container + header; see §5.2. |

Full width means *no* max-width class, not `max-w-full` — Dashboard, Vehicles, Activity and Notifications get nothing.

## 5. State-branch details

### 5.1 Settings

**Terminology correction.** FR-15 and the audit call `SettingsPage.tsx:19–28` the "loading branch". It is not: it is the `if (!activeFleetId)` **no-active-fleet** branch. Actual loading is handled per-card by `isLoading ? <Skeleton/> : <FleetNameForm/>` inside the Fleet Name card. FR-15's intent — same title, same container, same width as the loaded branch — applies to the no-fleet branch, and it is the branch to change. Naming it here so implementation does not go hunting for a page-level loading state that does not exist.

Result: `<div className="space-y-6 max-w-2xl"><PageHeader title="Fleet Settings" /><p …>No fleet selected…</p></div>`. This fixes the title-text mismatch (`Settings` → `Fleet Settings`), the gap (16px → 24px) and the missing width constraint in one edit.

**Accepted visual change.** The loaded branch's `space-y-8` becomes `space-y-6`, so the gaps *between the four settings cards* tighten from 32px to 24px, not just the header gap. FR-7 and the acceptance criterion "no `space-y-8` remains as a page-root container" both require this, and the alternative — a `space-y-6` header gap with `space-y-8` card gaps — needs a nested container to express and reintroduces exactly the bespoke-spacing pattern this task removes. Called out explicitly because PRD §8 says no unintended visual changes; this one is intended.

**Section headings.** The four `<h2 className="text-base font-semibold mb-4">` lose their `mb-4` (FR-19). Each lives in a `<CardContent className="pt-6">` with the heading and its body as bare siblings and no vertical rhythm of its own, so dropping `mb-4` would collapse the gap to zero. Compensate on the parent: `<CardContent className="pt-6 space-y-4">`. Same 16px, now container-driven — which is the point of FR-19 — and it satisfies "spacing within Settings sections is unchanged in effect".

### 5.2 Vehicle Detail

Three branches, one container (`max-w-2xl space-y-6`) and one header component across all of them.

- **Loading.** The vehicle name is not known yet, so the header renders `title="Vehicle"` with the body skeletons beneath. The title text swaps when data lands, but its box does not move — which is what the "no visible vertical shift" criterion is actually about. The alternative, a `Skeleton` shaped like an `<h1>`, would satisfy the pixel requirement too but costs a shimmer→text pop *and* leaves the route with no `<h1>` while loading. Text-first wins on accessibility.
- **Error / not found.** Currently a bare `<p className="text-muted-foreground">Vehicle not found.</p>` with no container at all. Becomes the standard container with `title="Vehicle"` and the muted line as the body. The `<h1>` is not "Vehicle not found" — the heading names the page, the body reports the condition.
- **Loaded.** As the table in §4. The existing markup maps onto the props one-to-one, which is the check that §2.3's slot props are the right shape and not invented cover for a special case.

## 6. Rejected alternatives

### 6.1 A `PageContainer` component

The obvious next abstraction: one component owning `space-y-6`, the width rule, and the header together, so a page is `<PageContainer width="narrow" title="…">`. Rejected for this task, matching PRD Open Question 3.

Reasons: it forecloses per-page container variation before we know what pages need (task-011's admin console is adding unknown page shapes right now); the width rule is currently only two values and encoding it as a prop enum is speculative; and a container that owns both the header and the children makes the loading/error branches *more* awkward, not less, because each branch would nest a full container.

The seam is left clean, though — every page ends up with the identical `<div className="space-y-6 …">` + `<PageHeader>` opening, so extracting `PageContainer` later is a mechanical sweep over six files. That is the right time to encode FR-9's width rule as a prop.

### 6.2 Hoisting `PageHeader` into `packages/ui-components`

Rejected per PRD §7. `PageHeader` encodes this app's shell decisions (h1 level, `text-2xl`, the 24px rhythm it participates in). A shared package would need it configurable enough to stop being a standard.

### 6.3 A lint rule banning `<h1>` in `pages/`

Tempting as an FR-17 enforcement mechanism. Out of scope: the codebase has no custom ESLint rules today, and adding an `eslint-plugin-local` scaffold to police six files is disproportionate. The acceptance-criteria grep covers it for this task; if pages drift again, that is the moment to automate.

## 7. Testing

**`apps/web/src/components/PageHeader.test.tsx`** — new, Vitest + React Testing Library, matching the conventions in `ThemeToggleButton.test.tsx` (FR-referencing comments, `screen.getByRole`, no snapshot tests):

| Case | Assertion |
|---|---|
| FR-3 | `getByRole('heading', { level: 1 })` has the title text and `text-2xl font-semibold` |
| FR-4 | With `actions`, both the heading and the action element render, and the container is a flex row |
| FR-4 | Without `actions`, the container has exactly one child element |
| FR-6 | `className="max-w-md"` appears on the root alongside the base classes |
| §2.2 | `titleAdornment` renders outside the `<h1>` (the heading's accessible name is the title alone) |
| §2.2 | `description` renders when provided; no `<p>` when absent |

**`apps/web/src/pages/DashboardPage.test.tsx`** — new, and the highest-value test in this task because it pins the two user-facing bugs the PRD found:

| Case | Assertion |
|---|---|
| FR-13 | With `role: 'viewer'` and `role: 'member'`, the `Dashboard` `<h1>` renders |
| FR-13 | With `role: 'viewer'`, no *Add Widget* control |
| FR-13 | With `role: 'owner'`, the *Add Widget* control renders |

This needs `AuthContext` and a React Query provider stubbed. If wiring those turns out to cost more than the test is worth, the fallback is testing `AddWidgetMenu` in isolation plus a manual role check — but the role regression is precisely what a unit test should hold, so attempt it first.

**Existing tests.** `AppLayout`, `ThemeToggle*`, `LoginPage` and `OnboardingPage` are the only test files, and none touch the six pages or assert on page-header markup. Expect zero updates to existing tests; if any break, that is signal, not noise.

**Verification gates.** `make fe-test`, `make fe-build`, then `make ci`, each with output reported. Then a manual pass over all six pages in light and dark mode confirming identical title size and identical title-to-content gap, plus one page navigated cold to watch for title shift between skeleton and loaded.

## 8. Sequencing

The changes are independent per page, so ordering is about keeping the tree green rather than dependency:

1. `PageHeader.tsx` + `PageHeader.test.tsx`. Nothing depends on anything; land the primitive first.
2. Activity, Notifications. Two-line diffs; they prove the component in situ.
3. Vehicles. Header + the `mt-6` deletions.
4. Settings. Container, both branches, the four `<h2>`s and the `CardContent` compensation.
5. Vehicle Detail. Exercises `titleAdornment` and `description`, plus the loading/error branches.
6. Dashboard. The hook extraction, `AddWidgetMenu`, `DashboardGrid` slimming, `DashboardPage` wiring, `DashboardPage.test.tsx`. Largest and most likely to need iteration — last, on a green tree.
7. Sweep: grep `apps/web/src/pages` for `<h1`, for `space-y-4`/`space-y-8` as page roots, and for `mt-6`.

## 9. Coordination

Per PRD §7, this branch does the full sweep and the three in-flight branches rebase onto it.

- **task-012 (vehicle-detail-redesign)** — highest conflict risk; it is rewriting `VehicleDetailPage.tsx` wholesale. On rebase it should adopt `PageHeader` and may legitimately change the page's max-width, in which case its choice wins and FR-9's table is updated rather than reverted (PRD Open Question 1). §2.3's slot props were designed with that redesign in mind: a redesigned header that still needs a badge and a subtitle already has somewhere to put them.
- **task-014 (member-names-ownership-transfer)** — touches the Settings *Members* card contents; this task touches the Settings container, the `<h2>` elements and `CardContent`. Different regions, mechanical conflicts. Whichever lands second: any new section heading follows FR-18 (`text-base font-semibold`, no `mb-4`), and the `space-y-4` on `CardContent` is what supplies its spacing.
- **task-011 (platform-admin-console)** — adding new pages. Every new page uses `PageHeader` and a `space-y-6` root; none writes its own `<h1>`.

## 10. Open items carried forward

1. **Settings title text.** FR-15 standardises on `Fleet Settings` while the sidebar nav item reads `Settings`. This design implements `Fleet Settings` as specified. Worth a look during the manual verification pass — if the mismatch reads badly in context, changing it is a one-string edit in one file.
2. **`titleAdornment` / `description` uptake.** Both exist for a single consumer. If task-012's redesign removes the need for either, delete the unused prop rather than leaving it as a slot nobody fills.
