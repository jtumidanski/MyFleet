# Page Header & Layout Consistency — Implementation Context

Companion to `plan.md`. Everything an implementer needs that is not a step in the plan: where things live, what was already decided and why, and what will bite.

Worktree: `/home/tumidanski/source/MyFleet/.worktrees/task-015-page-header-consistency`
Branch: `task-015-page-header-consistency`
Scope: `apps/web` only. No Go, no manifests, no API.

---

## 1. Why this task exists

Two of the three Dashboard problems are **bugs**, not drift, and they were not in the original report:

`DashboardGrid.tsx:139-141` on entry to this task:

```tsx
{isOwner && (
  <div className="flex items-center justify-between">
    <h2 className="text-lg font-semibold">Dashboard</h2>
```

1. `text-lg` where every other page uses `text-2xl` — the reported symptom.
2. `<h2>`, not `<h1>` — the dashboard route has **no `<h1>` at all**, breaking heading navigation.
3. Inside `isOwner &&` — **members and viewers see no page title whatsoever**.

`DashboardPage.test.tsx` (plan Task 8) exists to hold 2 and 3 down. If any test in this task is worth keeping, it is that one.

---

## 2. Key files

### Touched

| File | Role |
|---|---|
| `apps/web/src/components/PageHeader.tsx` | **New.** The header row. Sole owner of page-title markup. |
| `apps/web/src/components/features/dashboard/useDashboardWidgets.ts` | **New.** Widget list + writers, lifted out of `DashboardGrid`. |
| `apps/web/src/components/features/dashboard/AddWidgetMenu.tsx` | **New.** Add Widget button + dropdown, owns its own open state. |
| `apps/web/src/components/features/dashboard/DashboardGrid.tsx` | Becomes a props-driven renderer. Loses title, toolbar, state, fetching. |
| `apps/web/src/pages/{Dashboard,Vehicles,Activity,Notifications,Settings,VehicleDetail}Page.tsx` | Adopt `PageHeader` + a `space-y-6` root. |
| `apps/web/src/test/conventions.test.ts` | Gains the FR-17 ratchet. |

### Read but not touched

| File | Why it matters |
|---|---|
| `apps/web/src/lib/utils.ts` | `cn()` = `twMerge(clsx(...))`. Because `tailwind-merge` runs, a caller passing `className="items-start"` beats `PageHeader`'s derived `items-center`. That is the escape hatch, and it works without extra code. |
| `apps/web/src/components/AppLayout.tsx` | `<main className="flex-1 p-6">` — the 24px outer padding on every authenticated route. Already uniform. **Out of scope; do not touch.** The reported "different padding" was entirely the header-to-content gap inside `<main>`. |
| `apps/web/src/test/renderWithProviders.tsx` | Wraps in `QueryClientProvider` + `MemoryRouter`. **Does not provide `AuthContext`** — page tests `vi.mock('../context/AuthContext', …)`, the pattern in `AppLayout.test.tsx:11-13`. |
| `apps/web/src/test/conventions.test.ts` | Grep-as-test harness. Already bans hardcoded palette classes across every `.tsx` under `apps/web/src` and `packages/ui-components/src`. |
| `apps/web/src/lib/hooks/api/dashboard.ts` | `useDashboardLayout` is `enabled: !!fleetId`, which is why `DashboardPage` can call the hook unconditionally with `activeFleetId ?? ''` and issue no request. |
| `apps/web/src/components/features/dashboard/widgetCatalog.ts` | `WIDGET_CATALOG` — seven types, mirrors the backend's `ValidCatalog`. Pure TS, no React, safe to import from anywhere. |
| `apps/web/src/components/ThemeToggleButton.test.tsx` | The test-comment convention: name the FR *and* say why the behaviour matters. |

---

## 3. Decisions already made — do not relitigate

| Decision | Where argued | Short version |
|---|---|---|
| No `PageContainer` component | design §6.1 | Would foreclose per-page container variation while task-011 is adding unknown page shapes; the width rule is only two values today. The seam is left clean — every page ends up with an identical `<div className="space-y-6 …">` + `<PageHeader>` opening, so extracting it later is a mechanical sweep. |
| `PageHeader` stays in `apps/web`, not `packages/ui-components` | PRD §7, design §6.2 | It encodes *this app's* shell decisions (h1 level, `text-2xl`, the 24px rhythm). A shared version would need to be configurable enough to stop being a standard. |
| No ESLint rule for FR-17 | design §6.3 | No custom-rule scaffold exists; adding `eslint-plugin-local` to police six files is disproportionate. **The plan instead adds it to `conventions.test.ts`**, which already exists and already does this — a deliberate, recorded extension of the design. |
| `title` is `string`, not `ReactNode` | design §2.3 | So the `<h1>`'s accessible name is exactly the page name. Vehicle Detail's badge goes through `titleAdornment` and stays outside the heading. |
| Container is always a flex row, even without actions | design §2.2 | FR-4 forbids an *empty flex child*, not a flex container. One code path; with one left-aligned child it renders identically to a block. The `actions && …` guard is what satisfies the FR. |
| Add Widget moves via a lifted hook, not a render prop or portal | design §3.2 | Options B (leave it in the grid → two stacked rows, misses FR-13), C (portal → indirection to avoid a 60-line extraction) and D (`useImperativeHandle` → worst of both) were all considered and rejected. |
| Vehicle Detail's loading branch shows the literal text `Vehicle`, not a heading skeleton | design §5.2 | Both satisfy "no vertical shift", but text-first avoids a shimmer→text pop and leaves the route with a real `<h1>` while loading. |

---

## 4. Traps

**Settings has no page-level loading state.** FR-15 and the audit both call `SettingsPage.tsx:19-28` "the loading branch". It is `if (!activeFleetId)` — the **no-active-fleet** branch. Loading is per-card (`isLoading ? <Skeleton/> : <FleetNameForm/>` inside the Fleet Name card). Design §5.1 corrects the naming. Do not go hunting for a page-level loading state; there isn't one.

**Dropping `mb-4` from the Settings `<h2>`s collapses the gap to zero** unless `space-y-4` goes on the parent `CardContent` in the same edit. Each `CardContent` holds exactly two bare siblings — the heading and its body — with no rhythm of its own. Same 16px, now container-driven, which is the point of FR-19.

**`DashboardGrid`'s loading branch has a title skeleton** (`<Skeleton className="h-8 w-48" />`). It must be deleted in plan Task 8. Leaving it means a shimmer sitting under the real `<h1>` — the exact vertical jump the acceptance criteria forbid.

**`LoginPage.tsx:60` has a legitimate `<h1>`** — a marketing hero at `text-4xl`/`sm:text-5xl`/`lg:text-6xl`, outside the `AppLayout` shell. Any FR-17 grep or convention test must exclude `LoginPage.tsx`, `OnboardingPage.tsx` and `InviteAcceptPage.tsx`.

**"Full width" means no max-width class at all**, not `max-w-full`. Dashboard, Vehicles, Activity and Notifications get nothing.

**Hooks before early returns.** `DashboardPage` calls `useDashboardWidgets` above the `if (!activeFleetId)` return, because React forbids conditional hooks. Safe because `useDashboardLayout` is `enabled: !!fleetId`.

**`Date.now()` in `addWidget`** generates the optimistic widget id (`new-${Date.now()}`). It moves to the hook verbatim. Don't try to improve it in this task.

**The hardcoded-palette convention test is aggressive.** `bg-white`, `text-gray-500`, `border-black` and friends fail `make fe-test` across every `.tsx` in `apps/web/src`. All the markup this task moves already uses semantic tokens (`text-muted-foreground`, `bg-popover`, `border-border`); keep it that way.

---

## 5. Build and verification

Node is not always on `PATH`. In a fresh shell:

```sh
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
```

| Command | Scope |
|---|---|
| `npm run -w apps/web test -- <path>` | One test file — the inner loop. |
| `npm run -w apps/web test` | All `apps/web` tests. |
| `npm run -w apps/web build` | `tsc -b && vite build` — the type check. |
| `make fe-test` | `apps/web` **plus** `packages/shared-ts` **plus** `packages/ui-components`. |
| `make fe-build` | `npm run -w apps/web build`. |
| `make ci` | `lint-check vet test build fe-test fe-build manifests carfax-template`. |

`make ci` also runs the Go gates and manifest renders. This task touches neither, so a failure there is pre-existing — report it as such rather than fixing it silently.

Report actual command output when claiming a gate passes. The repo's rule is evidence before assertions.

---

## 6. Dependencies and sequencing

Task ordering in `plan.md` is about keeping the tree green, not about hard dependencies — only Task 1 genuinely blocks the rest.

```
Task 1  PageHeader + test          ← everything depends on this
  ├─ Task 2  Activity, Notifications  (proves the component in situ)
  ├─ Task 3  Vehicles
  ├─ Task 4  Settings
  ├─ Task 5  Vehicle Detail           (exercises titleAdornment + description)
  └─ Task 6  useDashboardWidgets      ← Dashboard chain, strictly ordered
       └─ Task 7  AddWidgetMenu
            └─ Task 8  DashboardPage wiring + role tests
Task 9  FR-17 ratchet + full verification   ← must run last
```

Tasks 6–8 are strictly sequential and are last among the implementation tasks because they are the largest and most likely to need iteration — better on an otherwise-green tree. Tasks 6 and 7 are pure extractions with **zero** user-visible change; if anything moves on screen after either, something went wrong.

---

## 7. In-flight branches this collides with

| Branch | Overlap | Resolution |
|---|---|---|
| `task-012-vehicle-detail-redesign` | Rewriting `VehicleDetailPage.tsx` wholesale. **Highest conflict risk.** | Adopts `PageHeader` on rebase. If it changes the page's max-width, **its choice wins** and FR-9's table is updated rather than task-012 reverted (PRD Open Question 1). `titleAdornment`/`description` were designed with that redesign in mind. |
| `task-014-member-names-ownership-transfer` | Settings *Members* card contents vs. this task's container, `<h2>`s and `CardContent`. | Different regions of the file; conflicts should be mechanical. Whichever lands second: new section headings follow FR-18 (`text-base font-semibold`, no `mb-4`), and `space-y-4` on `CardContent` supplies the spacing. |
| `task-011-platform-admin-console` | New pages. | Every new page uses `PageHeader` and a `space-y-6` root; none writes its own `<h1>`. |

Decision on record (PRD §7): this branch does the full sweep now; those three resolve at rebase time.

---

## 8. Open items to carry into review

1. **Settings title text.** FR-15 standardises on `Fleet Settings` while the sidebar nav item reads `Settings`. The plan implements `Fleet Settings` as specified and flags the mismatch during manual verification (plan Task 9 Step 7). If it reads badly in context, it is a one-string edit — but it is the user's call, not the implementer's.
2. **`titleAdornment` / `description` uptake.** Both exist for exactly one consumer (Vehicle Detail). If task-012's redesign removes the need for either, delete the unused prop rather than leaving a slot nobody fills.
3. **`max-w-2xl` on Vehicle Detail.** Preserved as-is here, but see task-012 above.
