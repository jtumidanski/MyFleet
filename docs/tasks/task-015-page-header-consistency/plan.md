# Page Header & Layout Consistency Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Introduce a single `PageHeader` component, adopt it across all six authenticated pages, and normalise every page root to a `space-y-6` container with an explicitly declared max-width.

**Architecture:** `PageHeader` is a pure presentational function component at `apps/web/src/components/PageHeader.tsx` rendering only the header row (`<h1>` + optional adornment/description/actions). Each page owns its own `<div className="space-y-6 …">` container, which supplies both the header-to-content gap and the gap between subsequent top-level blocks. The Dashboard needs a state lift before it can comply: the *Add Widget* button must move from `DashboardGrid` up into `DashboardPage` so it can be passed as `PageHeader`'s `actions`, which is done by extracting a `useDashboardWidgets` hook and an `AddWidgetMenu` component.

**Tech Stack:** React 18, TypeScript (strict), Tailwind CSS, `cn()` (clsx + tailwind-merge), Vitest + React Testing Library + jsdom, TanStack React Query v5.

## Global Constraints

- **Working directory:** every path in this plan is relative to `/home/tumidanski/source/MyFleet/.worktrees/task-015-page-header-consistency`. Never edit the main checkout.
- **Node is not always on `PATH`.** Before any `npm`/`make fe-*` command in a fresh shell, run: `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22`
- **Title treatment is exactly** `<h1 className="text-2xl font-semibold">`. No other size, no other level.
- **Page-root container is exactly** `space-y-6`. No `space-y-4`, no `space-y-8` as a page root.
- **Max-width rule (FR-9):** full width (no max-width class at all — *not* `max-w-full`) for Dashboard, Vehicles, Activity, Notifications. `max-w-2xl` for Settings and Vehicle Detail. Identical across a page's loading, empty, error and loaded branches (FR-10).
- **TypeScript strict mode. No `any`.** No non-null assertions added.
- **All conditional class composition goes through `cn()`** from `apps/web/src/lib/utils.ts`. No manual string concatenation or template literals for classes.
- **No hardcoded palette classes.** `apps/web/src/test/conventions.test.ts` fails the build on `(bg|text|border|ring|divide)-(gray|slate|zinc|neutral|white|black|red|green|blue|amber|yellow|emerald|orange)`. Use the semantic tokens (`text-muted-foreground`, `bg-popover`, `border-border`, …).
- **Out of scope, do not touch:** `AppLayout.tsx`, `LoginPage.tsx`, `OnboardingPage.tsx`, `InviteAcceptPage.tsx`, any `CardTitle` usage, `packages/ui-components`.
- **Test file convention:** tests carry a comment naming the FR or design section they pin and *why* the behaviour matters (see `ThemeToggleButton.test.tsx`). Use `screen.getByRole`. No snapshot tests.
- **Commit after every task.** Conventional-commit prefixes (`feat:`, `refactor:`, `test:`, `chore:`).

---

## File Structure

**Created**

| File | Responsibility |
|---|---|
| `apps/web/src/components/PageHeader.tsx` | The shared header row. Owns the `<h1>`, the optional adornment/description/actions slots, and nothing else. |
| `apps/web/src/components/PageHeader.test.tsx` | Pins the FR-2/FR-3/FR-4/FR-6 contract and the two slot props. |
| `apps/web/src/components/features/dashboard/useDashboardWidgets.ts` | The dashboard widget list: server fetch, local optimistic copy, add/remove/reorder writers. Owns `GridWidget`, `toGridWidget`, `toWidgetInputs`. |
| `apps/web/src/components/features/dashboard/useDashboardWidgets.test.ts` | Pins that the hook's writers mutate the list correctly and persist through `saveLayout`. |
| `apps/web/src/components/features/dashboard/AddWidgetMenu.tsx` | The *Add Widget* button + dropdown. Owns its own open/close state. |
| `apps/web/src/pages/DashboardPage.test.tsx` | Pins the two role bugs: title visible to every role, Add Widget owner-only. |

**Modified**

| File | Change |
|---|---|
| `apps/web/src/pages/ActivityPage.tsx` | `<h1>` → `<PageHeader>`. |
| `apps/web/src/pages/NotificationsPage.tsx` | `<h1>` → `<PageHeader>`. |
| `apps/web/src/pages/VehiclesPage.tsx` | `<h1>` + button row → `<PageHeader actions>`; root gains `space-y-6`; two `mt-6`s and one wrapper `<div>` deleted. |
| `apps/web/src/pages/SettingsPage.tsx` | Both branches to `space-y-6 max-w-2xl` + `<PageHeader title="Fleet Settings">`; four `<h2>`s lose `mb-4`; four `CardContent`s gain `space-y-4`. |
| `apps/web/src/pages/VehicleDetailPage.tsx` | All three branches to `max-w-2xl space-y-6` + `<PageHeader>`; loaded branch's hand-written header block replaced by `titleAdornment`/`description`/`actions`. |
| `apps/web/src/components/features/dashboard/DashboardGrid.tsx` | Loses the title, the toolbar, the state, the data fetching and the title skeleton. Becomes a props-driven renderer. |
| `apps/web/src/pages/DashboardPage.tsx` | Calls `useDashboardWidgets`, renders `PageHeader` + `AddWidgetMenu` + `DashboardGrid`; both branches use `space-y-6`. |
| `apps/web/src/test/conventions.test.ts` | New block mechanising FR-17 (no hand-written `<h1>` in authenticated pages). |

---

### Task 1: The `PageHeader` component

**Files:**
- Create: `apps/web/src/components/PageHeader.tsx`
- Test: `apps/web/src/components/PageHeader.test.tsx`

**Interfaces:**
- Consumes: `cn` from `apps/web/src/lib/utils.ts`.
- Produces: `export function PageHeader(props: PageHeaderProps)` and `export interface PageHeaderProps { title: string; titleAdornment?: ReactNode; description?: ReactNode; actions?: ReactNode; className?: string }`. Every later task imports `PageHeader` from `'../components/PageHeader'` (pages) or `'./PageHeader'` (components).

Two props beyond FR-2's table — `titleAdornment` and `description` — are a deliberate, recorded widening (design §2.3) so Vehicle Detail's badge and subtitle keep their current position without landing inside the `<h1>`'s accessible name.

- [ ] **Step 1: Write the failing test**

Create `apps/web/src/components/PageHeader.test.tsx`:

```tsx
import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { PageHeader } from './PageHeader';

describe('PageHeader', () => {
  // FR-3 + the NFR that every authenticated route exposes exactly one <h1>
  // naming the page. The class assertion is not cosmetic pedantry: the whole
  // task exists because the Dashboard drifted to text-lg.
  it('renders the title as the page h1 at the standard size', () => {
    render(<PageHeader title="Vehicles" />);

    const heading = screen.getByRole('heading', { level: 1, name: 'Vehicles' });
    expect(heading).toHaveClass('text-2xl', 'font-semibold');
  });

  // FR-4: title left, actions right, on ONE row. Two stacked rows is the
  // failure mode this component exists to prevent.
  it('renders actions alongside the title in a flex row', () => {
    const { container } = render(
      <PageHeader title="Vehicles" actions={<button type="button">Add Vehicle</button>} />,
    );

    expect(screen.getByRole('heading', { level: 1 })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Add Vehicle' })).toBeInTheDocument();
    expect(container.firstElementChild).toHaveClass('flex', 'justify-between');
  });

  // FR-4: "no empty flex child is rendered" when actions is absent. An empty
  // <div> in a justify-between row is invisible but changes how a long title
  // wraps, so it is worth pinning.
  it('renders no actions container when actions is absent', () => {
    const { container } = render(<PageHeader title="Activity" />);

    expect(container.firstElementChild?.children).toHaveLength(1);
  });

  // Same for a falsy actions expression — pages pass `isOwner && <Menu/>`,
  // which is `false`, not `undefined`, for a non-owner.
  it('renders no actions container when actions is false', () => {
    const { container } = render(<PageHeader title="Dashboard" actions={false} />);

    expect(container.firstElementChild?.children).toHaveLength(1);
  });

  // FR-6: className merges via cn() rather than replacing the base classes.
  it('merges className with the base classes', () => {
    const { container } = render(<PageHeader title="Activity" className="max-w-md" />);

    expect(container.firstElementChild).toHaveClass('flex', 'max-w-md');
  });

  // Design §2.2: the badge sits ON the title line but OUTSIDE the heading, so
  // the h1's accessible name stays the page name. Widening `title` to a
  // ReactNode would have put "Sold" inside it.
  it('renders titleAdornment outside the h1', () => {
    render(<PageHeader title="Daily Driver" titleAdornment={<span>Sold</span>} />);

    const heading = screen.getByRole('heading', { level: 1 });
    expect(heading).toHaveAccessibleName('Daily Driver');
    expect(heading.textContent).not.toContain('Sold');
    expect(screen.getByText('Sold')).toBeInTheDocument();
  });

  // Design §2.2: description is optional and produces no empty <p> when unset.
  it('renders description only when provided', () => {
    const { container, rerender } = render(<PageHeader title="Daily Driver" />);
    expect(container.querySelector('p')).toBeNull();

    rerender(<PageHeader title="Daily Driver" description="2019 Subaru Outback" />);
    expect(screen.getByText('2019 Subaru Outback')).toBeInTheDocument();
  });

  // Design §2.2: a single-line title against a 36px Button needs items-center
  // or the button rides high; a two-line title block needs items-start. The
  // component derives it so no page has to remember.
  it('derives vertical alignment from whether a description is present', () => {
    const { container, rerender } = render(<PageHeader title="Vehicles" />);
    expect(container.firstElementChild).toHaveClass('items-center');

    rerender(<PageHeader title="Vehicles" description="Sub" />);
    expect(container.firstElementChild).toHaveClass('items-start');
  });
});
```

- [ ] **Step 2: Run the test and verify it fails**

```bash
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
npm run -w apps/web test -- src/components/PageHeader.test.tsx
```

Expected: FAIL — `Failed to resolve import "./PageHeader"`.

- [ ] **Step 3: Write the implementation**

Create `apps/web/src/components/PageHeader.tsx`:

```tsx
/**
 * PageHeader — the single owner of page-title markup for every authenticated
 * page (task-015).
 *
 * Renders ONLY the header row. It does not render the page container and does
 * not own the gap to the content below it (FR-5): the page's own
 * `space-y-6` container supplies that, so one class governs the whole vertical
 * rhythm instead of the header and the container each having an opinion.
 *
 * App-shell furniture, not a design-system primitive — it encodes this app's
 * h1 level, its type scale and the 24px rhythm it participates in, so it
 * deliberately stays out of packages/ui-components (PRD §7).
 */
import type { ReactNode } from 'react';
import { cn } from '../lib/utils';

export interface PageHeaderProps {
  /** Page title. Rendered as the page's sole <h1>. */
  title: string;
  /**
   * Inline element sitting immediately right of the title (e.g. a StatusBadge).
   * Deliberately a separate slot rather than widening `title` to ReactNode: a
   * badge inside the <h1> would become part of the heading's accessible name.
   */
  titleAdornment?: ReactNode;
  /** Secondary line beneath the title, muted. */
  description?: ReactNode;
  /** Right-aligned controls on the title row. */
  actions?: ReactNode;
  /** Per-page escape hatch; merged via cn() so a caller can override. */
  className?: string;
}

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
        // A single-line title against a 36px Button needs items-center or the
        // button rides ~2px high. A title-plus-description block needs
        // items-start. Derived, so no caller has to know.
        description ? 'items-start' : 'items-center',
        className,
      )}
    >
      {/* min-w-0 + shrink-0 below are what actually stop a long title from
          colliding with the actions — gap-2 alone only guarantees the gap once
          flex has decided which child shrinks. */}
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

- [ ] **Step 4: Run the test and verify it passes**

```bash
npm run -w apps/web test -- src/components/PageHeader.test.tsx
```

Expected: PASS, 8 tests.

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/components/PageHeader.tsx apps/web/src/components/PageHeader.test.tsx
git commit -m "feat(web): add shared PageHeader component"
```

---

### Task 2: Adopt `PageHeader` on Activity and Notifications

Both pages are already fully conformant (`space-y-6` root, `text-2xl font-semibold` `<h1>`, full width). This is a pure swap, and it is second in sequence precisely because it proves the component in situ with nothing else moving.

**Files:**
- Modify: `apps/web/src/pages/ActivityPage.tsx:23-24`
- Modify: `apps/web/src/pages/NotificationsPage.tsx:10-11`

**Interfaces:**
- Consumes: `PageHeader` from Task 1.
- Produces: nothing new.

- [ ] **Step 1: Swap the Activity header**

In `apps/web/src/pages/ActivityPage.tsx`, add the import after the existing `ActivityFeed` import:

```tsx
import { PageHeader } from '../components/PageHeader';
```

Replace:

```tsx
      <h1 className="text-2xl font-semibold">Activity</h1>
```

with:

```tsx
      <PageHeader title="Activity" />
```

Leave the `<div className="space-y-6">` root and the `<ActivityFeed …>` call exactly as they are — including its `title="Fleet Activity"` prop, which is the *feed's* internal heading, not the page title.

- [ ] **Step 2: Swap the Notifications header**

In `apps/web/src/pages/NotificationsPage.tsx`, add the import after the existing `NotificationPreferences` import:

```tsx
import { PageHeader } from '../components/PageHeader';
```

Replace:

```tsx
      <h1 className="text-2xl font-semibold">Notifications</h1>
```

with:

```tsx
      <PageHeader title="Notifications" />
```

Leave the `<div className="space-y-6">` root and the two-column grid untouched.

- [ ] **Step 3: Verify the swap changed nothing else**

```bash
git diff --stat apps/web/src/pages/ActivityPage.tsx apps/web/src/pages/NotificationsPage.tsx
```

Expected: 2 files changed, 4 insertions(+), 2 deletions(-) — one import and one line replaced per file. Anything larger means something was edited that should not have been.

- [ ] **Step 4: Run the frontend test suite and the type build**

```bash
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
npm run -w apps/web test
npm run -w apps/web build
```

Expected: all tests PASS, build succeeds. No existing test asserts on these pages' markup, so nothing should need updating; a failure here is signal, not noise.

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/pages/ActivityPage.tsx apps/web/src/pages/NotificationsPage.tsx
git commit -m "refactor(web): adopt PageHeader on Activity and Notifications"
```

---

### Task 3: Adopt `PageHeader` on Vehicles and drop its `mt-6` compensation

`VehiclesPage` currently has **no root spacing class at all** — it fakes the 24px gap with `mt-6` on the form card and on a wrapper `<div>` that exists for no other reason. Both go; `space-y-6` on the root replaces them (FR-8).

**Files:**
- Modify: `apps/web/src/pages/VehiclesPage.tsx:47-78`

**Interfaces:**
- Consumes: `PageHeader` from Task 1.
- Produces: nothing new.

- [ ] **Step 1: Add the import**

In `apps/web/src/pages/VehiclesPage.tsx`, after the `Card` import line:

```tsx
import { PageHeader } from '../components/PageHeader';
```

- [ ] **Step 2: Replace the whole returned JSX**

Replace lines 47–78 (`return (` through the closing `);`) with:

```tsx
  return (
    <div className="space-y-6">
      <PageHeader
        title="Vehicles"
        actions={
          canWrite &&
          !showForm && (
            <Button type="button" onClick={() => setShowForm(true)}>
              Add Vehicle
            </Button>
          )
        }
      />

      {canWrite && showForm && (
        <Card>
          <CardHeader>
            <CardTitle className="text-lg">New Vehicle</CardTitle>
          </CardHeader>
          <CardContent>
            <VehicleForm
              mode="create"
              onSubmit={handleCreate}
              onCancel={() => setShowForm(false)}
              submitting={createVehicle.isPending}
            />
          </CardContent>
        </Card>
      )}

      <VehicleList vehicles={data?.data ?? []} isLoading={isLoading} />
    </div>
  );
```

Three things changed and nothing else: the header row became `PageHeader`, `<Card className="mt-6">` lost its `mt-6`, and the `<div className="mt-6">` around `VehicleList` is gone entirely — the wrapper existed only to carry that margin, so `VehicleList` is now a direct child of the `space-y-6` container.

- [ ] **Step 3: Verify no `mt-6` survives on this page**

```bash
grep -n "mt-6" apps/web/src/pages/VehiclesPage.tsx
```

Expected: no output (exit code 1). This is a literal acceptance criterion.

- [ ] **Step 4: Run tests and the type build**

```bash
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
npm run -w apps/web test
npm run -w apps/web build
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/pages/VehiclesPage.tsx
git commit -m "refactor(web): adopt PageHeader on Vehicles and drop mt-6 spacing"
```

---

### Task 4: Adopt `PageHeader` on Settings, unify both branches, and de-margin the section headings

Three separate corrections land together because they are all in one file and all in the same regions:

1. The `if (!activeFleetId)` branch — the design (§5.1) corrects the PRD/audit's naming here: this is the **no-active-fleet** branch, not a loading branch. There is no page-level loading state on Settings; loading is per-card (`isLoading ? <Skeleton/> : <FleetNameForm/>`). Do not go hunting for one. This branch gets the same title text, container and width as the loaded branch, fixing the `Settings`→`Fleet Settings` mismatch, the 16px→24px gap and the missing `max-w-2xl` at once.
2. `space-y-8` → `space-y-6` on the loaded branch. **This is an accepted, intended visual change:** the gaps between the four settings cards tighten from 32px to 24px, not just the header gap. FR-7 and the "no `space-y-8` remains as a page-root container" criterion both require it, and the alternative (24px header gap, 32px card gaps) needs a nested container — reintroducing exactly the bespoke spacing this task removes.
3. The four `<h2>`s lose `mb-4` (FR-19), and each `CardContent` gains `space-y-4` to supply the same 16px from the container instead. Each `CardContent` holds exactly two children — the heading and its body — with no vertical rhythm of its own, so dropping `mb-4` without compensating would collapse the gap to zero.

**Files:**
- Modify: `apps/web/src/pages/SettingsPage.tsx:19-75`

**Interfaces:**
- Consumes: `PageHeader` from Task 1.
- Produces: nothing new.

- [ ] **Step 1: Add the import**

In `apps/web/src/pages/SettingsPage.tsx`, after the `InviteList` import:

```tsx
import { PageHeader } from '../components/PageHeader';
```

- [ ] **Step 2: Rewrite the no-active-fleet branch**

Replace lines 19–28:

```tsx
  if (!activeFleetId) {
    return (
      <div className="space-y-4">
        <h1 className="text-2xl font-semibold">Settings</h1>
        <p className="text-sm text-muted-foreground">
          No fleet selected. Complete onboarding to get started.
        </p>
      </div>
    );
  }
```

with:

```tsx
  if (!activeFleetId) {
    return (
      <div className="space-y-6 max-w-2xl">
        <PageHeader title="Fleet Settings" />
        <p className="text-sm text-muted-foreground">
          No fleet selected. Complete onboarding to get started.
        </p>
      </div>
    );
  }
```

- [ ] **Step 3: Rewrite the loaded branch**

Replace lines 30–75 (`return (` through the closing `);`) with:

```tsx
  return (
    <div className="space-y-6 max-w-2xl">
      <PageHeader title="Fleet Settings" />

      {/* Fleet name — owner-only */}
      {isOwner && (
        <Card>
          <CardContent className="pt-6 space-y-4">
            <h2 className="text-base font-semibold">Fleet Name</h2>
            {isLoading ? (
              <Skeleton className="h-10 w-64" />
            ) : (
              <FleetNameForm fleetId={activeFleetId} currentName={fleet?.attributes.name ?? ''} />
            )}
          </CardContent>
        </Card>
      )}

      {/* Members */}
      <Card>
        <CardContent className="pt-6 space-y-4">
          <h2 className="text-base font-semibold">Members</h2>
          <MemberList fleetId={activeFleetId} isOwner={isOwner} />
        </CardContent>
      </Card>

      {/* Invites — owner-only */}
      {isOwner && (
        <>
          <Card>
            <CardContent className="pt-6 space-y-4">
              <h2 className="text-base font-semibold">Pending Invites</h2>
              <InviteList fleetId={activeFleetId} isOwner={isOwner} />
            </CardContent>
          </Card>

          <Card>
            <CardContent className="pt-6 space-y-4">
              <h2 className="text-base font-semibold">Invite a Member</h2>
              <InviteForm fleetId={activeFleetId} />
            </CardContent>
          </Card>
        </>
      )}
    </div>
  );
```

- [ ] **Step 4: Verify the file has no stale spacing or headings left**

```bash
grep -n "mb-4\|space-y-8\|space-y-4\"\|<h1" apps/web/src/pages/SettingsPage.tsx
```

Expected: no output (exit code 1). `space-y-4` appears only inside `"pt-6 space-y-4"`, which the `space-y-4"` pattern (trailing quote) will not match; `mb-4`, `space-y-8` and `<h1` must be entirely gone.

- [ ] **Step 5: Run tests and the type build**

```bash
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
npm run -w apps/web test
npm run -w apps/web build
```

Expected: all PASS. `InviteForm.test.tsx` and `InviteList.test.tsx` render those components directly, not through `SettingsPage`, so they are unaffected.

- [ ] **Step 6: Commit**

```bash
git add apps/web/src/pages/SettingsPage.tsx
git commit -m "refactor(web): adopt PageHeader on Settings and unify container spacing"
```

---

### Task 5: Adopt `PageHeader` on Vehicle Detail across all three branches

This is the page that justifies `titleAdornment` and `description` existing. Its header is a composite — title + status badge on one line, year/make/model/trim beneath, three buttons on the right — and the existing markup maps onto the props one-to-one, which is the check that the slot props are the right shape rather than invented cover for a special case.

The loading branch renders `title="Vehicle"` rather than a heading-shaped `Skeleton`: the title text swaps when data lands but its box does not move, which is what "no visible vertical shift" is actually about, and it leaves the route with a real `<h1>` while loading instead of none.

The error branch currently has **no container at all** — a bare `<p>`. It gets the standard container and `title="Vehicle"`, with the muted line as the body. The `<h1>` is not "Vehicle not found": the heading names the page, the body reports the condition.

**Files:**
- Modify: `apps/web/src/pages/VehicleDetailPage.tsx:40-51` (loading + error branches)
- Modify: `apps/web/src/pages/VehicleDetailPage.tsx:96-135` (loaded branch header)

**Interfaces:**
- Consumes: `PageHeader` from Task 1.
- Produces: nothing new.

- [ ] **Step 1: Add the import**

In `apps/web/src/pages/VehicleDetailPage.tsx`, after the `asVehicleStatus` import:

```tsx
import { PageHeader } from '../components/PageHeader';
```

- [ ] **Step 2: Rewrite the loading and error branches**

Replace lines 40–51:

```tsx
  if (isLoading) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-8 w-64" />
        <Skeleton className="h-40 w-full max-w-2xl" />
      </div>
    );
  }

  if (!vehicle) {
    return <p className="text-muted-foreground">Vehicle not found.</p>;
  }
```

with:

```tsx
  // Both non-loaded branches carry the SAME container and width as the loaded
  // branch (FR-10/FR-16), and a real <h1> rather than a heading-shaped
  // skeleton: the title text swaps when data lands, but its box does not move.
  if (isLoading) {
    return (
      <div className="max-w-2xl space-y-6">
        <PageHeader title="Vehicle" />
        <Skeleton className="h-40 w-full" />
      </div>
    );
  }

  if (!vehicle) {
    return (
      <div className="max-w-2xl space-y-6">
        <PageHeader title="Vehicle" />
        <p className="text-muted-foreground">Vehicle not found.</p>
      </div>
    );
  }
```

Note the body skeleton loses its own `max-w-2xl` — the container now supplies the width, which is the point of FR-9. The title skeleton is gone because `PageHeader` renders the real title.

- [ ] **Step 3: Replace the loaded branch's header block**

Replace lines 97–135 — the entire `<div className="flex items-start justify-between gap-2">…</div>` block, from `<div className="flex items-start justify-between gap-2">` through its matching `</div>` immediately before the `<Card>` — with:

```tsx
      <PageHeader
        title={title}
        titleAdornment={status && <StatusBadge status={status} />}
        description={
          <>
            {attributes.year} {attributes.make} {attributes.model}
            {attributes.trim ? ` ${attributes.trim}` : ''}
          </>
        }
        actions={
          <>
            {canWrite && !editing && (
              <Button type="button" variant="outline" onClick={() => setEditing(true)}>
                Edit
              </Button>
            )}
            {canWrite && (
              <Button
                type="button"
                variant="destructive"
                onClick={() => void handleDelete()}
                disabled={softDelete.isPending}
              >
                Delete
              </Button>
            )}
            {canRestore && (
              <Button
                type="button"
                variant="outline"
                onClick={() => void handleRestore()}
                disabled={restore.isPending}
              >
                Restore
              </Button>
            )}
          </>
        }
      />
```

Leave `<div className="max-w-2xl space-y-6">` (line 96) as the root — it is already correct — and leave everything from `<Card>` onward untouched.

Two details worth not getting wrong: the `description` is a fragment, not a string, because it interpolates four attributes with conditional trim — `description` is typed `ReactNode` for exactly this. And `PageHeader` supplies the `mt-1 text-sm text-muted-foreground` on the description, so those classes do **not** get passed in.

- [ ] **Step 4: Verify no hand-written header markup survives**

```bash
grep -n "<h1\|text-2xl" apps/web/src/pages/VehicleDetailPage.tsx
```

Expected: no output (exit code 1).

- [ ] **Step 5: Run tests and the type build**

```bash
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
npm run -w apps/web test
npm run -w apps/web build
```

Expected: all PASS. If the build complains that `Skeleton` or `StatusBadge` is now unused, an edit went too far — both are still used.

- [ ] **Step 6: Commit**

```bash
git add apps/web/src/pages/VehicleDetailPage.tsx
git commit -m "refactor(web): adopt PageHeader on Vehicle Detail across all states"
```

---

### Task 6: Extract `useDashboardWidgets`

Pure extraction, no behaviour change, no rendering change. `DashboardGrid` keeps rendering exactly what it renders today; it just gets its widget list and writers from a hook instead of holding them itself. This is the first of three Dashboard tasks, split so the tree stays green and a reviewer can reject the state lift without rejecting the header swap.

Why it is needed at all: FR-13 requires the *Add Widget* control to be passed to `PageHeader` as `actions`, which means `DashboardPage` must render it — but everything the button needs (`widgets`, `addWidget`, the open/close state) lives in `DashboardGrid` today. Moving the button means moving the state.

**Files:**
- Create: `apps/web/src/components/features/dashboard/useDashboardWidgets.ts`
- Test: `apps/web/src/components/features/dashboard/useDashboardWidgets.test.ts`
- Modify: `apps/web/src/components/features/dashboard/DashboardGrid.tsx`

**Interfaces:**
- Consumes: `useDashboardLayout`, `useSaveDashboardLayout` from `apps/web/src/lib/hooks/api/dashboard.ts`; `WIDGET_CATALOG`, `WidgetType` from `./widgetCatalog`; `widgetRegistry` from `./widgetRegistry`; `WidgetInput` from `apps/web/src/types/models/dashboard.ts`.
- Produces:
  ```ts
  export interface GridWidget {
    id: string;
    type: WidgetType;
    positionX: number;
    positionY: number;
    width: number;
    height: number;
  }

  export interface DashboardWidgets {
    widgets: GridWidget[];
    isLoading: boolean;
    addWidget: (type: WidgetType) => void;
    removeWidget: (id: string) => void;
    moveUp: (idx: number) => void;
    moveDown: (idx: number) => void;
  }

  export function useDashboardWidgets(fleetId: string): DashboardWidgets;
  ```
  `GridWidget` is now **exported** (it was file-private in `DashboardGrid.tsx`); Tasks 7 and 8 import it from here.

- [ ] **Step 1: Write the failing test**

Create `apps/web/src/components/features/dashboard/useDashboardWidgets.test.ts`:

```ts
/**
 * useDashboardWidgets — the widget-list state lifted out of DashboardGrid so
 * DashboardPage can own the Add Widget control (task-015, design §3.2).
 *
 * These tests exist because the extraction moved live logic across a file
 * boundary: an off-by-one in the reorder writers or a dropped save() call would
 * otherwise only surface as a user noticing their layout did not persist.
 */
import { describe, it, expect, vi, beforeEach } from 'vitest';
import React from 'react';
import { renderHook, act, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { dashboardService } from '../../../services/api/DashboardService';
import { useDashboardWidgets } from './useDashboardWidgets';
import type { Dashboard, WidgetResource } from '../../../types/models/dashboard';

vi.mock('../../../services/api/DashboardService', () => ({
  dashboardService: { getLayout: vi.fn(), saveLayout: vi.fn() },
}));

function widget(id: string, type: string, positionY: number): WidgetResource {
  return {
    type: 'dashboardWidgets',
    id,
    attributes: { type, positionX: 0, positionY, width: 1, height: 1 },
  };
}

function layout(widgets: WidgetResource[]): Dashboard {
  return {
    type: 'dashboards',
    id: 'd1',
    attributes: {
      fleetId: 'f1',
      userId: 'u1',
      widgets,
      createdAt: '2026-01-01T00:00:00Z',
      updatedAt: '2026-01-01T00:00:00Z',
    },
  };
}

function wrapper({ children }: { children: React.ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 }, mutations: { retry: false } },
  });
  return React.createElement(QueryClientProvider, { client }, children);
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(dashboardService.saveLayout).mockResolvedValue(layout([]));
});

describe('useDashboardWidgets', () => {
  it('maps the server layout into the widget list', async () => {
    vi.mocked(dashboardService.getLayout).mockResolvedValue(
      layout([widget('w1', 'fleet-overview', 0), widget('w2', 'recent-activity', 1)]),
    );

    const { result } = renderHook(() => useDashboardWidgets('f1'), { wrapper });

    await waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(result.current.widgets.map((w) => w.type)).toEqual([
      'fleet-overview',
      'recent-activity',
    ]);
  });

  // A widget type the frontend does not know how to render would crash the
  // registry lookup. Dropping it is the existing behaviour and must survive
  // the extraction.
  it('drops widgets whose type is not in the catalog', async () => {
    vi.mocked(dashboardService.getLayout).mockResolvedValue(
      layout([widget('w1', 'fleet-overview', 0), widget('w2', 'not-a-real-widget', 1)]),
    );

    const { result } = renderHook(() => useDashboardWidgets('f1'), { wrapper });

    await waitFor(() => expect(result.current.widgets).toHaveLength(1));
    expect(result.current.widgets[0]?.type).toBe('fleet-overview');
  });

  it('appends a widget and persists the new layout', async () => {
    vi.mocked(dashboardService.getLayout).mockResolvedValue(
      layout([widget('w1', 'fleet-overview', 0)]),
    );

    const { result } = renderHook(() => useDashboardWidgets('f1'), { wrapper });
    await waitFor(() => expect(result.current.widgets).toHaveLength(1));

    act(() => result.current.addWidget('recent-activity'));

    expect(result.current.widgets.map((w) => w.type)).toEqual([
      'fleet-overview',
      'recent-activity',
    ]);
    await waitFor(() => expect(dashboardService.saveLayout).toHaveBeenCalledTimes(1));
    expect(vi.mocked(dashboardService.saveLayout).mock.calls[0]?.[1]).toEqual([
      expect.objectContaining({ type: 'fleet-overview', positionY: 0 }),
      expect.objectContaining({ type: 'recent-activity', positionY: 1 }),
    ]);
  });

  it('removes a widget by id and persists', async () => {
    vi.mocked(dashboardService.getLayout).mockResolvedValue(
      layout([widget('w1', 'fleet-overview', 0), widget('w2', 'recent-activity', 1)]),
    );

    const { result } = renderHook(() => useDashboardWidgets('f1'), { wrapper });
    await waitFor(() => expect(result.current.widgets).toHaveLength(2));

    act(() => result.current.removeWidget('w1'));

    expect(result.current.widgets.map((w) => w.id)).toEqual(['w2']);
    await waitFor(() => expect(dashboardService.saveLayout).toHaveBeenCalledTimes(1));
  });

  // positionY is rewritten from array order on every save, so a swap that only
  // reorders the array but never persists would look right until a reload.
  it('swaps adjacent widgets on moveUp and persists the new order', async () => {
    vi.mocked(dashboardService.getLayout).mockResolvedValue(
      layout([widget('w1', 'fleet-overview', 0), widget('w2', 'recent-activity', 1)]),
    );

    const { result } = renderHook(() => useDashboardWidgets('f1'), { wrapper });
    await waitFor(() => expect(result.current.widgets).toHaveLength(2));

    act(() => result.current.moveUp(1));

    expect(result.current.widgets.map((w) => w.id)).toEqual(['w2', 'w1']);
    await waitFor(() => expect(dashboardService.saveLayout).toHaveBeenCalledTimes(1));
    expect(vi.mocked(dashboardService.saveLayout).mock.calls[0]?.[1]).toEqual([
      expect.objectContaining({ type: 'recent-activity', positionY: 0 }),
      expect.objectContaining({ type: 'fleet-overview', positionY: 1 }),
    ]);
  });

  it('does nothing at the list boundaries', async () => {
    vi.mocked(dashboardService.getLayout).mockResolvedValue(
      layout([widget('w1', 'fleet-overview', 0), widget('w2', 'recent-activity', 1)]),
    );

    const { result } = renderHook(() => useDashboardWidgets('f1'), { wrapper });
    await waitFor(() => expect(result.current.widgets).toHaveLength(2));

    act(() => result.current.moveUp(0));
    act(() => result.current.moveDown(1));

    expect(result.current.widgets.map((w) => w.id)).toEqual(['w1', 'w2']);
    expect(dashboardService.saveLayout).not.toHaveBeenCalled();
  });
});
```

- [ ] **Step 2: Run the test and verify it fails**

```bash
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
npm run -w apps/web test -- src/components/features/dashboard/useDashboardWidgets.test.ts
```

Expected: FAIL — `Failed to resolve import "./useDashboardWidgets"`.

- [ ] **Step 3: Create the hook**

Create `apps/web/src/components/features/dashboard/useDashboardWidgets.ts`. This is `DashboardGrid.tsx:23-124` moved verbatim, minus `showAddMenu` (which becomes `AddWidgetMenu`'s own state in Task 7) and minus the `setShowAddMenu(false)` line at the end of `addWidget`:

```ts
/**
 * useDashboardWidgets — the dashboard's widget list and its writers.
 *
 * Lifted out of DashboardGrid (task-015, design §3.2) so DashboardPage can own
 * both the page header's Add Widget control and the grid beneath it from a
 * single source of state. DashboardGrid became a props-driven renderer.
 *
 * Layout is persisted via PUT /fleets/{id}/dashboard on every mutation; the
 * local copy exists so the UI updates before the save settles.
 */
import { useCallback, useState } from 'react';
import { widgetRegistry } from './widgetRegistry';
import { WIDGET_CATALOG, type WidgetType } from './widgetCatalog';
import { useDashboardLayout, useSaveDashboardLayout } from '../../../lib/hooks/api/dashboard';
import type { WidgetInput } from '../../../types/models/dashboard';

export interface GridWidget {
  id: string;
  type: WidgetType;
  positionX: number;
  positionY: number;
  width: number;
  height: number;
}

export interface DashboardWidgets {
  widgets: GridWidget[];
  isLoading: boolean;
  addWidget: (type: WidgetType) => void;
  removeWidget: (id: string) => void;
  moveUp: (idx: number) => void;
  moveDown: (idx: number) => void;
}

function toGridWidget(w: {
  id: string;
  attributes: { type: string; positionX: number; positionY: number; width: number; height: number };
}): GridWidget | null {
  if (!WIDGET_CATALOG.includes(w.attributes.type as WidgetType)) return null;
  return {
    id: w.id,
    type: w.attributes.type as WidgetType,
    positionX: w.attributes.positionX,
    positionY: w.attributes.positionY,
    width: w.attributes.width,
    height: w.attributes.height,
  };
}

function toWidgetInputs(widgets: GridWidget[]): WidgetInput[] {
  return widgets.map((w, idx) => ({
    type: w.type,
    positionX: 0,
    positionY: idx,
    width: w.width,
    height: w.height,
  }));
}

export function useDashboardWidgets(fleetId: string): DashboardWidgets {
  const { data: dashboard, isLoading } = useDashboardLayout(fleetId);
  const saveLayout = useSaveDashboardLayout(fleetId);

  // Local widget list derived from the server layout on first load.
  // We keep a local copy for immediate UI updates before the save settles.
  const [localWidgets, setLocalWidgets] = useState<GridWidget[] | null>(null);

  const serverWidgets: GridWidget[] = (dashboard?.attributes.widgets ?? [])
    .map(toGridWidget)
    .filter((w): w is GridWidget => w !== null);

  const widgets = localWidgets ?? serverWidgets;

  const save = useCallback(
    (next: GridWidget[]) => {
      setLocalWidgets(next);
      saveLayout.mutate(toWidgetInputs(next));
    },
    [saveLayout],
  );

  const addWidget = (type: WidgetType) => {
    const entry = widgetRegistry[type];
    const next: GridWidget[] = [
      ...widgets,
      {
        id: `new-${Date.now()}`,
        type,
        positionX: 0,
        positionY: widgets.length,
        width: entry.defaultWidth,
        height: entry.defaultHeight,
      },
    ];
    save(next);
  };

  const removeWidget = (id: string) => {
    const next = widgets.filter((w) => w.id !== id);
    save(next);
  };

  const moveUp = (idx: number) => {
    if (idx === 0) return;
    const next = [...widgets];
    const above = next[idx - 1];
    const current = next[idx];
    if (!above || !current) return;
    next[idx - 1] = current;
    next[idx] = above;
    save(next);
  };

  const moveDown = (idx: number) => {
    if (idx === widgets.length - 1) return;
    const next = [...widgets];
    const current = next[idx];
    const below = next[idx + 1];
    if (!current || !below) return;
    next[idx] = below;
    next[idx + 1] = current;
    save(next);
  };

  return { widgets, isLoading, addWidget, removeWidget, moveUp, moveDown };
}
```

- [ ] **Step 4: Point `DashboardGrid` at the hook**

In `apps/web/src/components/features/dashboard/DashboardGrid.tsx`:

Replace the import block (lines 8–16) with:

```tsx
import { useState } from 'react';
import { Plus, Trash2, ChevronUp, ChevronDown } from 'lucide-react';
import { Skeleton } from '../../ui/skeleton';
import { Button } from '../../ui/button';
import { cn } from '../../../lib/utils';
import { widgetRegistry } from './widgetRegistry';
import { WIDGET_CATALOG, type WidgetType } from './widgetCatalog';
import { useDashboardWidgets } from './useDashboardWidgets';
```

Delete lines 23–55 entirely (the `GridWidget` interface, `toGridWidget` and `toWidgetInputs` — they now live in the hook).

Replace the body's opening (old lines 57–124, everything from `export function DashboardGrid(...) {` down to the closing brace of `moveDown`) with:

```tsx
export function DashboardGrid({ fleetId, isOwner }: DashboardGridProps) {
  const { widgets, isLoading, addWidget, removeWidget, moveUp, moveDown } =
    useDashboardWidgets(fleetId);
  const [showAddMenu, setShowAddMenu] = useState(false);

  const handleAdd = (type: WidgetType) => {
    addWidget(type);
    setShowAddMenu(false);
  };
```

Then change the dropdown's click handler from `onClick={() => addWidget(type)}` to `onClick={() => handleAdd(type)}`, so the menu still closes on select exactly as it does today.

Everything from `if (isLoading)` down is unchanged in this task.

- [ ] **Step 5: Run the tests and the type build**

```bash
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
npm run -w apps/web test -- src/components/features/dashboard/useDashboardWidgets.test.ts
npm run -w apps/web test
npm run -w apps/web build
```

Expected: the new hook test PASSES (6 tests), the full suite PASSES, and the build succeeds with no unused-import errors. `DashboardGrid` renders identically to before this task — nothing user-visible changed yet.

- [ ] **Step 6: Commit**

```bash
git add apps/web/src/components/features/dashboard/useDashboardWidgets.ts \
        apps/web/src/components/features/dashboard/useDashboardWidgets.test.ts \
        apps/web/src/components/features/dashboard/DashboardGrid.tsx
git commit -m "refactor(web): extract useDashboardWidgets from DashboardGrid"
```

---

### Task 7: Extract `AddWidgetMenu`

Second pure extraction. The button and its dropdown move into their own component that owns `showAddMenu` locally, so Task 8 can render it from `DashboardPage` without moving any more state. `DashboardGrid` still renders it in the same place; nothing on screen changes.

**Files:**
- Create: `apps/web/src/components/features/dashboard/AddWidgetMenu.tsx`
- Modify: `apps/web/src/components/features/dashboard/DashboardGrid.tsx`

**Interfaces:**
- Consumes: `WIDGET_CATALOG`, `WidgetType` from `./widgetCatalog`; `widgetRegistry` from `./widgetRegistry`; `Button`, `cn`.
- Produces:
  ```ts
  export interface AddWidgetMenuProps {
    placedTypes: WidgetType[];
    onAdd: (type: WidgetType) => void;
  }
  export function AddWidgetMenu(props: AddWidgetMenuProps): JSX.Element;
  ```
  `placedTypes` replaces the old `widgets.some((w) => w.type === type)` check — the menu needs to know which types are already on the board to grey them out, and that is all it needs to know about the widget list.

- [ ] **Step 1: Create the component**

Create `apps/web/src/components/features/dashboard/AddWidgetMenu.tsx`:

```tsx
/**
 * AddWidgetMenu — the dashboard's "Add Widget" button and its dropdown.
 *
 * Split out of DashboardGrid (task-015, design §3.2) so it can be rendered as
 * PageHeader's `actions` from DashboardPage. It owns only its own open/close
 * state; the widget list lives in useDashboardWidgets.
 *
 * Already-placed types are dimmed rather than disabled — adding a second copy
 * of a widget is legal, it just usually is not what you meant.
 */
import { useState } from 'react';
import { Plus } from 'lucide-react';
import { Button } from '../../ui/button';
import { cn } from '../../../lib/utils';
import { widgetRegistry } from './widgetRegistry';
import { WIDGET_CATALOG, type WidgetType } from './widgetCatalog';

export interface AddWidgetMenuProps {
  /** Types already on the board — rendered dimmed in the menu. */
  placedTypes: WidgetType[];
  onAdd: (type: WidgetType) => void;
}

export function AddWidgetMenu({ placedTypes, onAdd }: AddWidgetMenuProps) {
  const [showAddMenu, setShowAddMenu] = useState(false);

  const handleAdd = (type: WidgetType) => {
    onAdd(type);
    setShowAddMenu(false);
  };

  return (
    <div className="relative">
      <Button variant="outline" size="sm" onClick={() => setShowAddMenu((v) => !v)}>
        <Plus className="mr-1 h-4 w-4" />
        Add Widget
      </Button>
      {showAddMenu && (
        <div className="absolute right-0 z-10 mt-1 w-52 rounded-md border bg-popover shadow-lg">
          <ul className="py-1">
            {WIDGET_CATALOG.map((type) => (
              <li key={type}>
                <Button
                  variant="ghost"
                  size="sm"
                  className={cn(
                    'w-full justify-start px-4 py-2 text-sm font-normal',
                    placedTypes.includes(type) && 'text-muted-foreground',
                  )}
                  onClick={() => handleAdd(type)}
                >
                  {widgetRegistry[type].label}
                </Button>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}
```

- [ ] **Step 2: Use it from `DashboardGrid`**

In `apps/web/src/components/features/dashboard/DashboardGrid.tsx`:

Update the imports — drop `useState`, `Plus`, `cn` and the whole `./widgetCatalog` import (`WIDGET_CATALOG` and `WidgetType` are now only used by `AddWidgetMenu`), and add `AddWidgetMenu`:

```tsx
import { Trash2, ChevronUp, ChevronDown } from 'lucide-react';
import { Skeleton } from '../../ui/skeleton';
import { Button } from '../../ui/button';
import { widgetRegistry } from './widgetRegistry';
import { useDashboardWidgets } from './useDashboardWidgets';
import { AddWidgetMenu } from './AddWidgetMenu';
```

Delete the `showAddMenu` state and the `handleAdd` helper added in Task 6, leaving:

```tsx
export function DashboardGrid({ fleetId, isOwner }: DashboardGridProps) {
  const { widgets, isLoading, addWidget, removeWidget, moveUp, moveDown } =
    useDashboardWidgets(fleetId);
```

Replace the toolbar's `<div className="relative">…</div>` block (the button and the whole `{showAddMenu && …}` dropdown) with:

```tsx
          <AddWidgetMenu placedTypes={widgets.map((w) => w.type)} onAdd={addWidget} />
```

The surrounding `{isOwner && (<div className="flex items-center justify-between"><h2 …>Dashboard</h2> … </div>)}` stays for now — Task 8 removes it.

- [ ] **Step 3: Run the tests and the type build**

```bash
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
npm run -w apps/web test
npm run -w apps/web build
```

Expected: all PASS, no unused-import errors. Still nothing user-visible has changed.

- [ ] **Step 4: Commit**

```bash
git add apps/web/src/components/features/dashboard/AddWidgetMenu.tsx \
        apps/web/src/components/features/dashboard/DashboardGrid.tsx
git commit -m "refactor(web): extract AddWidgetMenu from DashboardGrid"
```

---

### Task 8: Move the Dashboard header into `DashboardPage`

The payload task. Three defects die here: the title becomes an `<h1>` at `text-2xl` instead of an `<h2>` at `text-lg`, and it renders for **every** role instead of owners only. Only *Add Widget* stays behind `isOwner`.

`DashboardGrid` gives up its data fetching entirely and becomes a props-driven renderer, so the page and the grid read one widget list from one `useDashboardWidgets` call rather than two.

Two consequential cleanups the design calls out explicitly:

- `DashboardGrid`'s loading branch currently renders `<Skeleton className="h-8 w-48" />` as a stand-in for the title. **It must go** — the real title is now always rendered by the page, and leaving the shimmer above it would reintroduce the exact vertical jump the acceptance criteria forbid. The two card skeletons stay.
- With the toolbar gone, the loaded branch's outer `<div className="space-y-4">` wraps a single child. Drop the wrapper and return the widget list (or the empty-state panel) directly. The inter-widget `gap-4` is untouched — FR-7 governs the page root, not intra-component layout.

**Files:**
- Modify: `apps/web/src/components/features/dashboard/DashboardGrid.tsx`
- Modify: `apps/web/src/pages/DashboardPage.tsx`
- Test: `apps/web/src/pages/DashboardPage.test.tsx`

**Interfaces:**
- Consumes: `PageHeader` (Task 1), `useDashboardWidgets` + `GridWidget` (Task 6), `AddWidgetMenu` (Task 7).
- Produces: the new `DashboardGridProps` shape:
  ```ts
  interface DashboardGridProps {
    fleetId: string;
    isOwner: boolean;
    widgets: GridWidget[];
    isLoading: boolean;
    removeWidget: (id: string) => void;
    moveUp: (idx: number) => void;
    moveDown: (idx: number) => void;
  }
  ```
  Note `addWidget` is deliberately **not** a `DashboardGrid` prop — the page passes it to `AddWidgetMenu` directly. Pass the props explicitly rather than spreading the hook result, so an accidentally-renamed field is a compile error.

- [ ] **Step 1: Write the failing test**

Create `apps/web/src/pages/DashboardPage.test.tsx`:

```tsx
/**
 * DashboardPage — the two role bugs task-015 exists to fix.
 *
 * Before this task the page title lived inside `isOwner &&` in DashboardGrid,
 * so a member or a viewer landed on a page with no title at all. These tests
 * are what stop that regressing: the title is unconditional, only the Add
 * Widget control is owner-gated (FR-13).
 */
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { screen } from '@testing-library/react';
import { renderWithProviders } from '../test/renderWithProviders';
import type { AuthContextValue } from '../context/AuthContext';
import type { FleetRole } from '../types/models/user';
import { DashboardPage } from './DashboardPage';

const mockAuth = vi.fn<() => AuthContextValue>();
vi.mock('../context/AuthContext', () => ({
  useAuth: () => mockAuth(),
}));

// The grid is not under test here; stubbing it keeps this file away from the
// widget components' own data fetching.
vi.mock('../components/features/dashboard/DashboardGrid', () => ({
  DashboardGrid: () => <div data-testid="dashboard-grid" />,
}));

vi.mock('../components/features/dashboard/useDashboardWidgets', () => ({
  useDashboardWidgets: () => ({
    widgets: [],
    isLoading: false,
    addWidget: vi.fn(),
    removeWidget: vi.fn(),
    moveUp: vi.fn(),
    moveDown: vi.fn(),
  }),
}));

// Built out in full rather than cast, mirroring baseAuth in AppLayout.test.tsx
// — a cast would silently keep compiling if AuthContextValue gained a field the
// page starts depending on.
function authAs(role: FleetRole | null, activeFleetId: string | null = 'f1'): AuthContextValue {
  return {
    user: null,
    activeFleetId,
    role,
    isAuthenticated: true,
    isLoading: false,
    login: vi.fn(),
    logout: vi.fn(),
  };
}

beforeEach(() => vi.clearAllMocks());

describe('DashboardPage', () => {
  it.each<FleetRole>(['owner', 'member', 'viewer'])('renders the h1 title for a %s', (role) => {
    mockAuth.mockReturnValue(authAs(role));

    renderWithProviders(<DashboardPage />);

    expect(screen.getByRole('heading', { level: 1, name: 'Dashboard' })).toBeInTheDocument();
  });

  it.each<FleetRole>(['member', 'viewer'])('hides Add Widget from a %s', (role) => {
    mockAuth.mockReturnValue(authAs(role));

    renderWithProviders(<DashboardPage />);

    expect(screen.queryByRole('button', { name: /add widget/i })).not.toBeInTheDocument();
  });

  it('shows Add Widget to an owner', () => {
    mockAuth.mockReturnValue(authAs('owner'));

    renderWithProviders(<DashboardPage />);

    expect(screen.getByRole('button', { name: /add widget/i })).toBeInTheDocument();
  });

  // FR-14: the no-fleet branch differs from the loaded branch in body content
  // only — same header, same container.
  it('renders the same title when no fleet is active', () => {
    mockAuth.mockReturnValue(authAs('owner', null));

    renderWithProviders(<DashboardPage />);

    expect(screen.getByRole('heading', { level: 1, name: 'Dashboard' })).toBeInTheDocument();
    expect(screen.getByText(/no fleet selected/i)).toBeInTheDocument();
  });
});
```

`FleetRole` is `'owner' | 'member' | 'viewer'`, exported from `apps/web/src/types/models/user.ts:20` — not from `fleet.ts`, despite the name.

- [ ] **Step 2: Run the test and verify it fails**

```bash
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
npm run -w apps/web test -- src/pages/DashboardPage.test.tsx
```

Expected: FAIL — the `member`/`viewer` title cases and the Add Widget cases fail because the page renders no heading at all yet (it renders only `<DashboardGrid>`, which is stubbed out).

- [ ] **Step 3: Slim `DashboardGrid` down to a renderer**

Rewrite `apps/web/src/components/features/dashboard/DashboardGrid.tsx` entirely as:

```tsx
/**
 * DashboardGrid — renders the widget list. Add, remove and reorder controls
 * only; the list itself and its writers come from useDashboardWidgets, and the
 * page header (title + Add Widget) is DashboardPage's (task-015, design §3).
 *
 * No drag-and-drop library added — uses simple buttons per guidelines.
 */
import { Trash2, ChevronUp, ChevronDown } from 'lucide-react';
import { Skeleton } from '../../ui/skeleton';
import { Button } from '../../ui/button';
import { widgetRegistry } from './widgetRegistry';
import type { GridWidget } from './useDashboardWidgets';

interface DashboardGridProps {
  fleetId: string;
  isOwner: boolean;
  widgets: GridWidget[];
  isLoading: boolean;
  removeWidget: (id: string) => void;
  moveUp: (idx: number) => void;
  moveDown: (idx: number) => void;
}

export function DashboardGrid({
  fleetId,
  isOwner,
  widgets,
  isLoading,
  removeWidget,
  moveUp,
  moveDown,
}: DashboardGridProps) {
  if (isLoading) {
    // No title skeleton here: the page renders the real <h1> unconditionally,
    // so a shimmer above these cards would reintroduce the vertical jump this
    // task exists to remove.
    return (
      <div className="space-y-4">
        <Skeleton className="h-40 w-full" />
        <Skeleton className="h-40 w-full" />
      </div>
    );
  }

  if (widgets.length === 0) {
    return (
      <div className="rounded-lg border border-dashed p-12 text-center">
        <p className="text-sm text-muted-foreground">
          {isOwner
            ? 'No widgets yet. Click "Add Widget" to customize your dashboard.'
            : 'Dashboard is empty.'}
        </p>
      </div>
    );
  }

  return (
    <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
      {widgets.map((widget, idx) => {
        const entry = widgetRegistry[widget.type];
        const WidgetComponent = entry.component;
        return (
          <div key={widget.id} className="relative group">
            <WidgetComponent fleetId={fleetId} />
            {isOwner && (
              <div className="absolute top-2 right-2 hidden group-hover:flex items-center gap-1 bg-background rounded border shadow-sm p-0.5">
                <Button
                  variant="ghost"
                  size="icon"
                  title="Move up"
                  disabled={idx === 0}
                  onClick={() => moveUp(idx)}
                  className="h-6 w-6"
                >
                  <ChevronUp className="h-3 w-3" />
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  title="Move down"
                  disabled={idx === widgets.length - 1}
                  onClick={() => moveDown(idx)}
                  className="h-6 w-6"
                >
                  <ChevronDown className="h-3 w-3" />
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  title="Remove widget"
                  onClick={() => removeWidget(widget.id)}
                  className="h-6 w-6 text-destructive hover:text-destructive hover:bg-destructive/10"
                >
                  <Trash2 className="h-3 w-3" />
                </Button>
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}
```

The `space-y-4` wrapper is gone from both non-loading branches (each returns a single element now); the loading branch keeps its own because it stacks two skeletons.

- [ ] **Step 4: Wire up `DashboardPage`**

Rewrite `apps/web/src/pages/DashboardPage.tsx` entirely as:

```tsx
/**
 * Dashboard page — customizable widget grid.
 * Route: / (index)
 *
 * The title is unconditional (FR-13): it used to live inside `isOwner &&` in
 * DashboardGrid, which left members and viewers on an untitled page. Only the
 * Add Widget control is owner-gated.
 */
import { useAuth } from '../context/AuthContext';
import { PageHeader } from '../components/PageHeader';
import { DashboardGrid } from '../components/features/dashboard/DashboardGrid';
import { AddWidgetMenu } from '../components/features/dashboard/AddWidgetMenu';
import { useDashboardWidgets } from '../components/features/dashboard/useDashboardWidgets';

export function DashboardPage() {
  const { activeFleetId, role } = useAuth();
  const isOwner = role === 'owner';
  const { widgets, isLoading, addWidget, removeWidget, moveUp, moveDown } = useDashboardWidgets(
    activeFleetId ?? '',
  );

  if (!activeFleetId) {
    return (
      <div className="space-y-6">
        <PageHeader title="Dashboard" />
        <p className="text-sm text-muted-foreground">
          No fleet selected. Complete onboarding to get started.
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="Dashboard"
        actions={
          isOwner && (
            <AddWidgetMenu placedTypes={widgets.map((w) => w.type)} onAdd={addWidget} />
          )
        }
      />
      <DashboardGrid
        fleetId={activeFleetId}
        isOwner={isOwner}
        widgets={widgets}
        isLoading={isLoading}
        removeWidget={removeWidget}
        moveUp={moveUp}
        moveDown={moveDown}
      />
    </div>
  );
}
```

The hook is called before the early return because React hooks cannot be called conditionally. It is passed `activeFleetId ?? ''`, and `useDashboardLayout` is `enabled: !!fleetId`, so with no active fleet it issues no request — the same behaviour as today, where `DashboardGrid` simply was not rendered.

- [ ] **Step 5: Run the new test and verify it passes**

```bash
npm run -w apps/web test -- src/pages/DashboardPage.test.tsx
```

Expected: PASS, 7 tests (3 role × title, 2 role × hidden, 1 owner-visible, 1 no-fleet).

- [ ] **Step 6: Verify the Dashboard title is really gone from the grid**

```bash
grep -n "Dashboard\|text-lg\|<h2" apps/web/src/components/features/dashboard/DashboardGrid.tsx
```

Expected: matches only in the file's doc comment and the `'Dashboard is empty.'` empty-state string. No `<h2>`, no `text-lg`, no title element — a literal acceptance criterion.

- [ ] **Step 7: Run the full suite and the type build**

```bash
npm run -w apps/web test
npm run -w apps/web build
```

Expected: all PASS.

- [ ] **Step 8: Commit**

```bash
git add apps/web/src/pages/DashboardPage.tsx \
        apps/web/src/pages/DashboardPage.test.tsx \
        apps/web/src/components/features/dashboard/DashboardGrid.tsx
git commit -m "fix(web): render Dashboard title as h1 for every role"
```

---

### Task 9: Mechanise FR-17 and run the full verification sweep

FR-17 ("no page file contains a hand-written `<h1>`") is stated in the PRD as a grep the reviewer runs by hand. This repo already has a home for exactly this kind of rule — `apps/web/src/test/conventions.test.ts` pins the pre-paint theme script, the brand mark and the palette ban the same way — so the grep becomes a test instead of a habit.

This goes beyond the design, which considered and rejected a custom **ESLint** rule (§6.3) on the grounds that scaffolding `eslint-plugin-local` to police six files is disproportionate. That reasoning does not extend to a convention test: the harness already exists, the addition is ~20 lines, and it costs no new dependency.

`LoginPage.tsx:60` has a legitimate `<h1>` — a marketing hero at `text-4xl`/`sm:text-5xl`/`lg:text-6xl`, outside the `AppLayout` shell and explicitly out of scope. The test must scope to the authenticated pages only.

**Files:**
- Modify: `apps/web/src/test/conventions.test.ts`

**Interfaces:**
- Consumes: nothing from earlier tasks (it reads files from disk).
- Produces: nothing.

- [ ] **Step 1: Write the failing test**

Append to `apps/web/src/test/conventions.test.ts`:

```ts
// FR-17. Every authenticated page's title goes through PageHeader; a
// hand-written <h1> is how the Dashboard drifted to text-lg and an <h2> in the
// first place. The unauthenticated pages are centered-card layouts outside the
// AppLayout shell — LoginPage's hero <h1> is a deliberate exception, not drift.
describe('authenticated pages do not hand-write their title', () => {
  const UNAUTHENTICATED = ['LoginPage.tsx', 'OnboardingPage.tsx', 'InviteAcceptPage.tsx'];

  it('contain no <h1> element', () => {
    const pagesDir = resolve(WEB_ROOT, 'src/pages');

    const offenders = readdirSync(pagesDir)
      .filter((f) => f.endsWith('.tsx') && !f.endsWith('.test.tsx'))
      .filter((f) => !UNAUTHENTICATED.includes(f))
      .flatMap((file) =>
        readFileSync(join(pagesDir, file), 'utf8')
          .split('\n')
          .map((text, index) => ({ file, line: index + 1, text }))
          .filter((entry) => entry.text.includes('<h1')),
      )
      .map((entry) => `${entry.file}:${entry.line}  ${entry.text.trim()}`);

    expect(offenders, 'render the page title via <PageHeader title="…" /> instead').toEqual([]);
  });
});
```

`readdirSync`, `readFileSync`, `join` and `resolve` are already imported at the top of the file, as is `WEB_ROOT`. No new imports are needed.

- [ ] **Step 2: Run it and verify it passes**

```bash
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
npm run -w apps/web test -- src/test/conventions.test.ts
```

Expected: PASS. Tasks 2–8 already removed every `<h1>` from the authenticated pages, so this is green on arrival — which is the point; it is a ratchet, not a discovery tool.

- [ ] **Step 3: Prove the ratchet actually bites**

A convention test that passes for the wrong reason (an empty file walk, a filter that excludes everything) is worse than none. Verify it fails when it should:

```bash
sed -i 's|<PageHeader title="Activity" />|<h1 className="text-2xl font-semibold">Activity</h1>|' apps/web/src/pages/ActivityPage.tsx
npm run -w apps/web test -- src/test/conventions.test.ts
```

Expected: FAIL, naming `ActivityPage.tsx:<line>`. Now restore:

```bash
git checkout apps/web/src/pages/ActivityPage.tsx
npm run -w apps/web test -- src/test/conventions.test.ts
```

Expected: PASS, and `git status` shows `ActivityPage.tsx` clean.

- [ ] **Step 4: Run the acceptance-criteria greps**

```bash
echo "--- FR-17: no <h1> in authenticated pages ---"
grep -rn "<h1" apps/web/src/pages/ --include="*.tsx" | grep -v ".test." | grep -v "LoginPage"

echo "--- no space-y-4 or space-y-8 as a page root ---"
grep -rn 'className="space-y-4"\|className="space-y-8"\|space-y-8' apps/web/src/pages/

echo "--- no mt-6 compensation ---"
grep -rn "mt-6" apps/web/src/pages/

echo "--- every page root declares space-y-6 ---"
grep -c "space-y-6" apps/web/src/pages/DashboardPage.tsx apps/web/src/pages/VehiclesPage.tsx \
  apps/web/src/pages/ActivityPage.tsx apps/web/src/pages/NotificationsPage.tsx \
  apps/web/src/pages/SettingsPage.tsx apps/web/src/pages/VehicleDetailPage.tsx

echo "--- max-width: only Settings and Vehicle Detail ---"
grep -rn "max-w-2xl" apps/web/src/pages/
```

Expected: the first three greps produce **no output**. The fourth counts one `space-y-6` line per rendered branch, so:

| File | Branches | Expected count |
|---|---|---|
| `DashboardPage.tsx` | no-fleet, loaded | 2 |
| `VehiclesPage.tsx` | loaded | 1 |
| `ActivityPage.tsx` | loaded | 1 |
| `NotificationsPage.tsx` | loaded | 1 |
| `SettingsPage.tsx` | no-fleet, loaded | 2 |
| `VehicleDetailPage.tsx` | loading, error, loaded | 3 |

The fifth grep shows `max-w-2xl` in `SettingsPage.tsx` (2 hits) and `VehicleDetailPage.tsx` (3 hits) only — one per branch, matching the table above. Any hit in Dashboard, Vehicles, Activity or Notifications is a bug, as is a `max-w-2xl` count that does not match that file's branch count (FR-10).

- [ ] **Step 5: Run the full verification gates and report the output**

```bash
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
make fe-test
make fe-build
make ci
```

Expected: all three PASS. **Report the actual output of each**, per the repo's evidence-before-assertions rule — do not summarise as "tests pass" without pasting the result. `make ci` also runs the Go gates and the manifest checks; this task touches no Go and no manifests, so a failure there is pre-existing and should be reported as such rather than fixed silently.

- [ ] **Step 6: Manual verification pass**

```bash
npm run -w apps/web dev
```

Walk all six pages in **both light and dark mode** and confirm, with the browser's element inspector where it helps:

1. Every page's title is the same size (`text-2xl font-semibold`) and is an `<h1>`.
2. The title-to-first-content gap is 24px on every page.
3. Dashboard, Vehicles, Activity and Notifications run full width; Settings and Vehicle Detail are constrained to `max-w-2xl`.
4. Navigate to Vehicle Detail and to the Dashboard **cold** (hard reload) and watch the title through the skeleton→loaded transition — it must not move.
5. Settings: the four cards are 24px apart (down from 32px — this is the intended change) and the gap between each `<h2>` and its content is still 16px.
6. Vehicles: with the add-form open and closed, spacing is even at 24px throughout.
7. Vehicle Detail on a soft-deleted vehicle: the status badge still sits on the title line, the subtitle beneath it, the buttons top-right and vertically aligned to the top of the title block.

Also confirm the two role fixes by hand if a non-owner account is available: a `member` or `viewer` on the Dashboard sees the title and does **not** see *Add Widget*.

- [ ] **Step 7: Settle the open question on the Settings title**

Design §10.1 / PRD Open Question 2: FR-15 standardises on `Fleet Settings` while the sidebar nav item reads `Settings`. This plan implements `Fleet Settings` as specified. During Step 6, look at the two side by side. If the mismatch reads badly, it is a one-string edit in `SettingsPage.tsx` (two occurrences — both branches) plus the `PageHeader` title in each. **Flag it to the user rather than deciding unilaterally**; the PRD asked for a sanity check, not a free hand.

- [ ] **Step 8: Commit**

```bash
git add apps/web/src/test/conventions.test.ts
git commit -m "test(web): pin FR-17 — no hand-written h1 in authenticated pages"
```

---

## Post-Plan Notes

**Coordination (design §9).** This branch does the full sweep; three in-flight branches rebase onto it.

- `task-012-vehicle-detail-redesign` — highest conflict risk; it is rewriting `VehicleDetailPage.tsx` wholesale. On rebase it should adopt `PageHeader`, and it may legitimately change that page's max-width — in which case **its choice wins** and FR-9's table gets updated rather than task-012's decision reverted (PRD Open Question 1).
- `task-014-member-names-ownership-transfer` — touches the Settings *Members* card contents; this task touches the Settings container, the `<h2>`s and `CardContent`. Different regions, so conflicts should be mechanical. Whichever lands second: any new section heading follows FR-18 (`text-base font-semibold`, no `mb-4`), and the `space-y-4` on `CardContent` is what supplies its spacing.
- `task-011-platform-admin-console` — adding new pages. Every new page uses `PageHeader` and a `space-y-6` root; none writes its own `<h1>`.

**Deliberate deviations from the spec documents, all recorded here so review has them on record:**

| Deviation | Where | Why |
|---|---|---|
| `PageHeader` takes two props beyond FR-2's table (`titleAdornment`, `description`) | Task 1 | Vehicle Detail's header is title + badge + subtitle + buttons. Widening `title` to `ReactNode` would put "Sold" inside the `<h1>`'s accessible name. Design §2.3 records this as a decision, not a liberty. |
| Two new dashboard files (`useDashboardWidgets.ts`, `AddWidgetMenu.tsx`) beyond the PRD's "Files touched" list | Tasks 6–7 | FR-13 requires the Add Widget control to be `PageHeader`'s `actions`, which means the page renders it, which means the state moves. Design §3.2 Option A. |
| Design §8 treats the Dashboard as one step; this plan splits it into three | Tasks 6, 7, 8 | Two pure extractions with no behaviour change, then the behaviour change. Keeps the tree green and lets a reviewer reject the state lift without rejecting the header swap. |
| FR-17 mechanised as a convention test | Task 9 | The design rejected a custom **ESLint** rule as disproportionate. `conventions.test.ts` already exists and already does exactly this kind of check, so the cost is ~20 lines and no new dependency. |
| `DashboardGrid` receives explicit props rather than `{...grid}` | Task 8 | The design's snippet is illustrative shorthand. Explicit props make a renamed hook field a compile error, and `addWidget` is not a grid concern. |

**Accepted visual changes** (everything else must look identical):

1. Dashboard title: `text-lg` `<h2>` → `text-2xl` `<h1>`, and visible to every role rather than owners only.
2. Settings: gaps between the four cards tighten 32px → 24px (design §5.1, required by FR-7).
3. Settings no-fleet branch: title text `Settings` → `Fleet Settings`, gap 16px → 24px, gains `max-w-2xl`.
4. Dashboard: header-to-content gap 16px → 24px, both branches.
5. Vehicle Detail loading branch: the title skeleton is replaced by the literal text `Vehicle`; the body skeleton's own `max-w-2xl` moves to the container.
6. Vehicle Detail error branch: gains the standard container and an `<h1>` where it previously rendered a bare `<p>`.
