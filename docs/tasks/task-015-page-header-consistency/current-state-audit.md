# Current-State Audit — Page Headers & Spacing

Captured 2026-08-02 against `main` @ `a113e86`. Line numbers are as of that commit and will drift once refactoring starts; use them as a starting map, not as gospel.

## Page titles

| Page | File:line | Element | Classes | Notes |
|---|---|---|---|---|
| Dashboard | `components/features/dashboard/DashboardGrid.tsx:141` | `h2` | `text-lg font-semibold` | Wrong level, wrong size, **owner-only** |
| Vehicles | `pages/VehiclesPage.tsx:50` | `h1` | `text-2xl font-semibold` | reference treatment |
| Activity | `pages/ActivityPage.tsx:24` | `h1` | `text-2xl font-semibold` | reference treatment |
| Notifications | `pages/NotificationsPage.tsx:11` | `h1` | `text-2xl font-semibold` | reference treatment |
| Settings (loaded) | `pages/SettingsPage.tsx:32` | `h1` | `text-2xl font-semibold` | title text `Fleet Settings` |
| Settings (loading) | `pages/SettingsPage.tsx:22` | `h1` | `text-2xl font-semibold` | title text `Settings` — mismatch |
| Vehicle Detail | `pages/VehicleDetailPage.tsx:100` | `h1` | `text-2xl font-semibold` | dynamic title |

`text-2xl font-semibold` is the clear majority and becomes the standard.

### The three Dashboard defects

`DashboardGrid.tsx:139-141`:

```tsx
{isOwner && (
  <div className="flex items-center justify-between">
    <h2 className="text-lg font-semibold">Dashboard</h2>
```

1. `text-lg` where every other page uses `text-2xl` — the reported symptom.
2. `h2` not `h1` — the dashboard route has no `h1`, breaking the heading outline.
3. Inside `isOwner &&` — members and viewers see no page title at all.

Defects 2 and 3 were not in the original report and are user-facing bugs, not drift.

## Header → content gap

| Page | File:line | Container | Gap |
|---|---|---|---|
| Dashboard | `pages/DashboardPage.tsx:24` | `space-y-4` | 16px |
| Dashboard (no fleet) | `pages/DashboardPage.tsx:14` | `space-y-4` | 16px |
| Vehicles | `pages/VehiclesPage.tsx:48` | *(none)* | 24px via `mt-6` on children |
| Activity | `pages/ActivityPage.tsx:23` | `space-y-6` | 24px |
| Notifications | `pages/NotificationsPage.tsx:10` | `space-y-6` | 24px |
| Settings (loaded) | `pages/SettingsPage.tsx:31` | `space-y-8` | 32px |
| Settings (loading) | `pages/SettingsPage.tsx:21` | `space-y-4` | 16px |
| Vehicle Detail | `pages/VehicleDetailPage.tsx:96` | `space-y-6` | 24px |

Three distinct gap values plus one page achieving the gap by a different mechanism entirely. 24px is the plurality and becomes the standard.

Note that Settings and Dashboard each disagree **with themselves** between loading and loaded states, so the title visibly shifts as data arrives.

## Content max-width

| Page | Constraint |
|---|---|
| Dashboard | full width |
| Vehicles | full width |
| Activity | full width |
| Notifications | full width |
| Settings | `max-w-2xl` (loaded only — loading branch is unconstrained) |
| Vehicle Detail | `max-w-2xl` |

The full-width-for-lists / constrained-for-forms split is coherent; it is simply undocumented and applied inconsistently across states. The PRD formalises it rather than changing it.

## App shell — already consistent, leave alone

`components/AppLayout.tsx`:

- line 52 — top bar: `flex items-center justify-between border-b border-border px-6 py-3`
- line 63 — content: `<main className="flex-1 p-6">`

Outer page padding is uniform at 24px on all sides for every authenticated route. The reported "different padding" is entirely the header-to-content gap inside `<main>`, not the shell. No `AppLayout` changes are needed.

## Section headings

`SettingsPage.tsx` lines 38, 51, 61, 68 — all `<h2 className="text-base font-semibold mb-4">`. Internally consistent already; the only change is dropping `mb-4` in favour of container-driven spacing.

Other pages use `CardTitle` for section-level labelling, which is a separate primitive and out of scope.

## Out-of-scope pages

`LoginPage`, `OnboardingPage` and `InviteAcceptPage` render centered `Card` layouts outside the `AppLayout` shell and have no page header. Confirmed no `h1` page titles in these files.
