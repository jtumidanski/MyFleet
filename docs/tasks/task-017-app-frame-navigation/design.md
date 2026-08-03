# Application Frame Navigation — Design

Task: task-017-app-frame-navigation
PRD: [prd.md](./prd.md)
Status: Proposed
Created: 2026-08-02

---

## 1. Scope and shape of the change

Everything in this task is `apps/web`. Nothing in it changes a route, an API call,
a payload, or a service. What changes is who owns the chrome around `<Outlet />`.

Today two files — `components/AppLayout.tsx` (72 lines) and
`components/admin/AdminLayout.tsx` (96 lines) — each hand-roll a complete shell:
a fixed `w-56` aside, a nav list, a header row, and (in the admin case) a mode
band. The two files are 80% identical by line and have already drifted once (the
`bg-card` rationale comment lives in `AppLayout` only, though `AdminLayout`
depends on the same fact).

The design replaces both hand-rolled shells with the shadcn `sidebar` primitive
and factors the duplicated parts — the header row, the profile menu, the
breadcrumb — into shared components. After the change each layout file states
only what is *different* about its shell: which nav table, which brand target,
and (admin only) the danger band.

Net file movement:

| Kind | Path | Note |
|---|---|---|
| New (vendored) | `src/components/ui/sidebar.tsx` | shadcn, Tailwind v3 variant |
| New (vendored) | `src/components/ui/dropdown-menu.tsx` | shadcn |
| New (vendored) | `src/components/ui/tooltip.tsx` | shadcn |
| New (vendored) | `src/components/ui/separator.tsx` | shadcn |
| New (vendored) | `src/components/ui/breadcrumb.tsx` | shadcn — see §6.1 |
| New | `src/lib/hooks/useIsMobile.ts` | sidebar's viewport hook |
| New | `src/components/frame/FrameHeader.tsx` | shared header row |
| New | `src/components/frame/ProfileMenu.tsx` | shared profile dropdown |
| New | `src/components/frame/BrandLink.tsx` | shared sidebar header lockup |
| New | `src/components/frame/FrameNav.tsx` | shared nav-table renderer |
| New | `src/components/frame/AppBreadcrumb.tsx` | trail renderer |
| New | `src/components/frame/breadcrumbTrails.ts` | the route→trail table (data) |
| New | `src/components/frame/crumbs/VehicleNameCrumb.tsx` | resolving crumb |
| New | `src/components/frame/crumbs/FleetNameCrumb.tsx` | resolving crumb |
| Rewritten | `src/components/AppLayout.tsx` | |
| Rewritten | `src/components/admin/AdminLayout.tsx` | |
| Amended | `src/index.css`, `tailwind.config.ts` | `--sidebar-*` family |
| Amended | `src/test/setup.ts` | Radix jsdom stubs (§8.3) |

`components/frame/` is a new folder. The alternative — leaving nine files loose
in the already-flat `src/components/` beside `BrandMark.tsx` and
`ThemeToggle.tsx` — was rejected: the frame is a coherent unit with one entry
point per shell, and the existing tree already groups by concern (`admin/`,
`features/`, `providers/`, `ui/`). `frame/` is that same convention applied to
the shell. Nothing goes into `packages/ui-components` for the reason
`PageHeader.tsx:9-12` already records: this is app-shell furniture, not a
design-system primitive.

---

## 2. Vendoring the shadcn primitives

### 2.1 Import style — relative, no alias

PRD open question 5 is resolved as the PRD assumed: **rewrite shadcn's `@/…`
imports to relative paths.** Verified: `apps/web/tsconfig.json` is a solution
file with no `paths`, and `vite.config.ts` has no `resolve.alias`. Every existing
vendored primitive already imports `../../lib/utils`. Introducing `@/` would
require a coordinated `tsconfig.app.json` + `vite.config.ts` + eslint change and
would leave the tree half-converted; that belongs in its own task if it is ever
wanted.

### 2.2 Which peer components are actually needed

`sidebar.tsx` imports `Button`, `Input`, `Separator`, `Sheet`, `Skeleton`,
`Tooltip`, `Slot`, `cva`, `cn`, and a `useIsMobile` hook. Of these, `button`,
`input`, `sheet`, and `skeleton` already exist and MUST be reused (FR-SIDEBAR-1).
Three are missing and are vendored: `separator`, `tooltip`, `dropdown-menu`.

`separator` is vendored even though the only place this design would otherwise
use it is the admin footer divider (§4.3), because `sidebar.tsx` imports it
directly. Deleting `SidebarSeparator` to avoid one small dependency means
editing the vendored file, which makes every future shadcn diff noisier than the
dependency is expensive. Vendor it faithfully.

`useIsMobile` goes to `src/lib/hooks/useIsMobile.ts`, not
`src/components/ui/use-mobile.tsx`. The repo's hook convention is
`src/lib/hooks/<camelCase>.ts` (`usePendingAttachments.ts`,
`useRuntimeConfig.ts`); `components/ui/` holds components. The vendored
`sidebar.tsx` gets its import path adjusted accordingly.

### 2.3 New dependencies

```
@radix-ui/react-dropdown-menu
@radix-ui/react-tooltip
@radix-ui/react-separator
```

Verified absent from `node_modules/@radix-ui/` in the main checkout and from
`package-lock.json`. `@radix-ui/react-slot`, `@radix-ui/react-dialog` (which
backs `sheet.tsx`, and therefore the mobile sidebar), `class-variance-authority`
and `lucide-react@0.400.0` are already present. Pin with the `^1.1.x` caret style
the sibling Radix entries use. Because this worktree has no `node_modules`, the
first implementation step is `npm install` at the repo root, and the root
`package-lock.json` is part of the diff.

### 2.4 `tailwindcss-animate` is not installed — and stays that way

Verified: `tailwind.config.ts` has `plugins: []`, and `tailwindcss-animate` is
not a dependency. The already-vendored `sheet.tsx`, `dialog.tsx`, `popover.tsx`
and `select.tsx` all carry `animate-in` / `fade-in-0` / `zoom-in-95` classes that
therefore resolve to nothing. The newly vendored tooltip and dropdown-menu carry
the same classes.

Decision: **vendor them verbatim, animations inert, and do not add the plugin.**
Adding it here would make three menus in the app animate while every existing
dialog and sheet does not — a visible inconsistency introduced as a side effect
of a navigation task. Stripping the classes would make the vendored files diverge
from upstream for no functional gain. Leaving them inert matches what every other
primitive in the tree already does. If the app wants motion, that is a
deliberate, app-wide task.

The sidebar's own collapse transition does **not** depend on the plugin — it uses
plain `transition-[width]` utilities, which are core Tailwind. Collapse animates;
menus appear instantly. That is the status quo for every other overlay.

---

## 3. Design tokens

### 3.1 Values

Eight custom properties in each of `:root` and `.dark` in `src/index.css`,
registered under `theme.extend.colors.sidebar` in `tailwind.config.ts`.

| Token | Light | Dark | Mirrors |
|---|---|---|---|
| `--sidebar` | `0 0% 100%` | `222.2 84% 4.9%` | `--card` |
| `--sidebar-foreground` | `222.2 84% 4.9%` | `210 40% 98%` | `--card-foreground` |
| `--sidebar-primary` | `222.2 47.4% 11.2%` | `210 40% 98%` | `--primary` |
| `--sidebar-primary-foreground` | `210 40% 98%` | `222.2 47.4% 11.2%` | `--primary-foreground` |
| `--sidebar-accent` | `210 40% 96.1%` | `217.2 32.6% 17.5%` | `--accent` |
| `--sidebar-accent-foreground` | `222.2 47.4% 11.2%` | `210 40% 98%` | `--accent-foreground` |
| `--sidebar-border` | `214.3 31.8% 91.4%` | `217.2 32.6% 17.5%` | `--border` |
| `--sidebar-ring` | `222.2 84% 4.9%` | `212.7 26.8% 83.9%` | `--ring` |

This is exactly today's palette re-expressed: surface = card, active/hover =
accent, edge = border. FR-TOKEN-2 is satisfied *by construction*, not by luck —
`--sidebar` differs from `--sidebar-accent` in both themes (white vs. `96.1%`
grey; `4.9%` navy vs. `17.5%` slate), which is precisely the property a
`bg-muted` sidebar would have destroyed. The comment at `AppLayout.tsx:24-28`
that records why gets carried into `index.css` beside the token block, because
that is now where the fact lives.

### 3.2 Literal values, not `var()` aliases

`--sidebar: var(--card)` would work — Tailwind expands `hsl(var(--sidebar))` and
CSS resolves the chain — and would guarantee the two never drift. It is
nonetheless rejected: `index.css` today contains zero `var()` indirection, and a
single aliased family would be an inconsistency a reviewer has to stop and
reason about. Literals with a `/* mirrors --card */` comment state the same
intent without introducing a second idiom. The cost is that a future change to
`--card` must remember `--sidebar`; the comment is what pays that cost.

### 3.3 Registration

```ts
sidebar: {
  DEFAULT: 'hsl(var(--sidebar))',
  foreground: 'hsl(var(--sidebar-foreground))',
  primary: 'hsl(var(--sidebar-primary))',
  'primary-foreground': 'hsl(var(--sidebar-primary-foreground))',
  accent: 'hsl(var(--sidebar-accent))',
  'accent-foreground': 'hsl(var(--sidebar-accent-foreground))',
  border: 'hsl(var(--sidebar-border))',
  ring: 'hsl(var(--sidebar-ring))',
},
```

This matches the nesting the existing `success` / `warning` / `danger` / `info`
families already use, and yields the `bg-sidebar`, `text-sidebar-foreground`,
`bg-sidebar-accent`, `border-sidebar-border` classes the vendored component
references. Note the shadcn source expects `bg-sidebar` (not `bg-sidebar-background`);
the `DEFAULT` key is what makes that resolve.

---

## 4. Shell composition

### 4.1 The frame both shells build

```
<SidebarProvider defaultOpen={readSidebarCookie()}>
  <Sidebar collapsible="icon">
    <SidebarHeader>   <BrandLink … />                       </SidebarHeader>
    <SidebarContent>  <FrameNav items={…} />                </SidebarContent>
    <SidebarFooter>   (admin only: Back to my fleet)        </SidebarFooter>
    <SidebarRail />
  </Sidebar>
  <SidebarInset>
    <FrameHeader />
    {/* admin only: danger band */}
    <main className="flex-1 p-6"><Outlet /></main>
  </SidebarInset>
</SidebarProvider>
```

`AppLayout` and `AdminLayout` remain two files (a task-011 structural decision
the PRD keeps as a non-goal). What they share is now *components*, not copied
JSX. Each layout is left holding: its nav table, its brand target, and — for
`AdminLayout` — the danger band and the footer link.

### 4.2 `FrameHeader` — one component, not two copies

FR-HEADER-1 specifies an identical five-part row for both shells and FR-HEADER-2
requires identical height; FR-PROFILE-6 already forces the profile menu to be
shared. Given all three, duplicating the row in two files would leave three
independent contracts to keep in sync by hand — exactly the drift that produced
the missing `bg-card` comment in `AdminLayout` today.

```tsx
<header className="flex h-14 shrink-0 items-center gap-2 border-b border-border px-6">
  <SidebarTrigger />
  <AppBreadcrumb />
  <div className="ml-auto flex items-center gap-2">
    <ThemeToggle />
    <ProfileMenu />
  </div>
</header>
```

`FrameHeader` takes no props. It reads the location itself (via `AppBreadcrumb`)
and the user itself (via `ProfileMenu` → `useAuth`). A propless component is the
right call here: every input it needs is ambient context that both shells
already provide, and threading them through would only give callers the chance
to pass something different.

`h-14` is fixed rather than shadcn's `h-16` shrinking to `h-12` on collapse. A
header that changes height when the sidebar collapses moves every page's content
up and down, which FR-HEADER-2 forbids. `px-6` keeps the header's content
aligned with `<main>`'s `p-6` (FR-HEADER-4). Today's `py-3` around a 36px button
yields ~60px, so 56px is a 4px change — visible only to a pixel diff.

The danger band stays in `AdminLayout`, between `<FrameHeader />` and `<main>`,
verbatim including both comment blocks (FR-HEADER-3).

### 4.3 `FrameNav` — one renderer, two tables

```ts
export interface FrameNavItem {
  to: string;
  label: string;
  icon: LucideIcon;
  end?: boolean;
}
```

`FrameNav` maps items to `SidebarMenuItem` → `SidebarMenuButton asChild
isActive={…} tooltip={label}` wrapping a `<Link>`.

**Active state is computed, not delegated to `NavLink`.** `SidebarMenuButton`
needs `isActive` as a *value* so it can set `data-active` and apply the
primitive's own active styling; `NavLink`'s render-prop hands `isActive` to a
callback that runs inside the anchor, too late to reach the button. So `FrameNav`
computes it from `useLocation().pathname`:

```ts
const active = matchPath({ path: item.to, end: item.end ?? false }, pathname) !== null;
```

This reproduces `NavLink`'s semantics exactly — `NavLink` itself uses the same
matcher — so FR-NAV-5 holds: `/` and (in the fleet shell) `/admin` carry
`end: true` and stay dark on descendant routes; `/vehicles` and `/admin/fleets`
match on prefix and stay lit on `:id` routes. Because the button is now a plain
`<Link>`, the accessible name is still the label, and `getByRole('link', { name })`
in the existing tests keeps working.

The alternative — keeping `NavLink` and styling via its className callback —
would forfeit `data-active` and with it the primitive's collapsed-rail active
treatment, and would put two competing sources of active styling in the same
element. Rejected.

The admin shell's "Back to my fleet" (FR-NAV-7) is a `SidebarFooter` containing
a `SidebarSeparator` and a single `SidebarMenuButton` with `ArrowLeft`. Footer
rather than a sixth nav row is the structural expression of "this is the exit,
not a destination" — the same intent today's `border-t` block carries, now
carried by the primitive's own slot.

### 4.4 `BrandLink`

```tsx
<BrandLink to="/" label="MyFleet" ariaLabel="MyFleet home" />
<BrandLink to="/admin" label="MyFleet" suffix="admin" ariaLabel="MyFleet admin home" />
```

Rendered as a `SidebarMenuButton size="lg" asChild` wrapping a `<Link>`, which
is what gives it the primitive's hover state, its focus ring, and — critically —
the collapsed-rail behaviour: the `BrandMark` sits in the button's fixed-width
icon slot and the wordmark in a `<span>` the primitive hides at
`data-collapsible=icon`. FR-BRAND-3/4/5 all fall out of using the primitive
rather than being separately engineered.

`BrandMark` stays `aria-hidden` (its own doc comment explains why) — the
accessible name comes from the `aria-label` on the link, so nothing announces
twice.

### 4.5 `ProfileMenu`

Trigger: `Button variant="ghost" size="icon"` with `aria-label="Account menu"`,
matching `ThemeToggleButton`'s existing shape so the two sit as one pair.

Avatar: a plain `<img className="h-6 w-6 rounded-full object-cover" alt="">` when
`user.attributes.avatarUrl` is a non-empty string, falling back to a `CircleUser`
icon. A local `onError` state flips a broken URL back to the icon.

**No `avatar.tsx`.** Vendoring shadcn's avatar adds `@radix-ui/react-avatar`
solely to get an image-with-fallback inside a 24px circle; an `<img>` and one
boolean does the same thing with no dependency and no new primitive to maintain.
If avatars later need initials-fallback, cropping, or status dots, that is when
the primitive earns its place.

**Icon name:** `CircleUser`, not `UserCircle`. Verified in the installed
`lucide-react@0.400.0` d.ts — `UserCircle` exists only as a deprecated alias of
`CircleUser`. Same component; use the current name.

Menu body:

```tsx
<DropdownMenuLabel className="font-normal">
  <p className="truncate text-sm font-medium">{primary}</p>
  <p className="truncate text-xs text-muted-foreground">{secondary}</p>
</DropdownMenuLabel>
<DropdownMenuSeparator />
<DropdownMenuItem onSelect={() => void logout()}>Sign out</DropdownMenuItem>
```

`DropdownMenuLabel` is non-interactive by construction, so FR-PROFILE-3's
"non-interactive header" needs no extra work. The identity fallback chain
(FR-PROFILE-4) is a pure function worth naming and testing on its own:

```ts
// displayName → email → "Account". Today's header renders `?? ''`, which on a
// user with no display name produces an empty label.
function identityLines(user: User | null): { primary: string; secondary: string }
```

The secondary line is the email, omitted when it would repeat the primary (a
user with no display name would otherwise show their email twice).

Keyboard behaviour (FR-PROFILE-5) is Radix default and is not overridden —
notably, no `onCloseAutoFocus` handler, since preventing it is the usual way
focus-return gets broken.

`ThemeToggle` is imported unchanged (FR-PROFILE-7). The theme control is *not*
folded into the menu: `ThemeToggle` fires a session-bound mutation and
`ThemeToggleButton` exists precisely so the signed-out login page can render the
control without one. Merging them would drag the mutation toward the login page.

---

## 5. Collapse persistence — the one real deviation from upstream

FR-SIDEBAR-5 requires the collapse choice to survive a reload via the
`sidebar_state` cookie. This is the part of the shadcn sidebar that assumes a
framework MyFleet does not use.

Upstream's provider owns the open/closed state and writes `sidebar_state` on
every toggle, but derives its *initial* value from a `defaultOpen` prop. In the
Next.js reference app that prop is filled by reading the cookie **on the
server** during render, so the first paint is already correct. MyFleet is a Vite
SPA served as static files: there is no server render, so nothing fills
`defaultOpen`, and the sidebar would open expanded on every reload regardless of
what was written to the cookie. **The cookie would be written and never read.**

Two ways to close the loop:

**A — read the cookie inside the vendored provider (recommended).** Change
`SidebarProvider`'s `useState` initializer from `defaultOpen` to
`defaultOpen ?? readSidebarCookie() ?? true`, with `readSidebarCookie()` a small
function in the same file that parses `document.cookie`. Both shells then get
persistence by rendering `<SidebarProvider>` with no props, and there is exactly
one place where the cookie is read and written.

**B — read it in each layout and pass `defaultOpen`.** Keeps the vendored file
byte-identical to upstream, at the cost of two call sites that must agree, and a
read/write split across two modules.

Recommend **A**. The deviation is small, it is confined to one clearly commented
initializer, and it puts the read next to the write. B's advantage — a pristine
vendored file — is largely theoretical, since §2.1 already rewrites every import
in that file. The deviation gets a comment saying exactly this, so the next
person diffing against upstream knows the line is intentional.

Two details either way: the initializer must be lazy (`useState(() => …)`) so
the cookie is parsed once rather than on every render, and the write should add
`SameSite=Lax` to upstream's `path=/; max-age=…` — a free hardening on a cookie
that carries a UI boolean and never needs to travel cross-site.

Because the cookie is read at mount and never again, a second tab that toggles
the sidebar does not move this one. That is correct: FR-SIDEBAR-5 scopes the
requirement to reloads.

---

## 6. Breadcrumb

### 6.1 Vendor shadcn's `breadcrumb.tsx` too

FR-SIDEBAR-1 lists four primitives to vendor; this design adds a fifth.
`breadcrumb.tsx` has **no new dependency** — it is markup plus `ChevronRight`
(lucide, present) plus `Slot` (present) — and it supplies, for free, every
structural accessibility requirement in §8 of the PRD: `<nav
aria-label="breadcrumb">` on the root, `aria-current="page"` on
`BreadcrumbPage`, `role="presentation" aria-hidden="true"` on each separator.
Hand-rolling that markup means hand-rolling those attributes and hand-testing
them. It also makes PRD open question 3's `BreadcrumbEllipsis` option available
without a second vendoring pass, should a long trail ever need it.

### 6.2 `useMatches` is unavailable — verified, not assumed

The obvious way to build a breadcrumb in React Router 6 is `useMatches()` with a
`handle` on each route. **It does not work in this app.** Verified in
`node_modules/react-router/dist/index.js:752`: `useMatches` calls
`useDataRouterState`, which at line 700-704 throws an invariant when
`DataRouterStateContext` is absent. That context exists only under
`RouterProvider` (`createBrowserRouter`). `App.tsx:100` uses `<BrowserRouter>`
wrapping `<Routes>` — the non-data router. `useMatches()` would throw on mount
for every authenticated route.

Migrating the app to `createBrowserRouter` to unlock it is out of scope and
would touch `App.tsx`'s route tree, which `postPurgeRouting.test.tsx` guards for
a reason spelled out at `App.tsx:21-28`.

So the trail is derived by matching `useLocation().pathname` against a table with
`matchPath` — the same primitive `NavLink` and `FrameNav` use.

### 6.3 The table: explicit trails, one row per route

```ts
type Crumb =
  | { kind: 'static'; label: string; to?: string }
  | { kind: 'vehicle' }   // resolves :id via VehicleNameCrumb
  | { kind: 'fleet' };    // resolves :id via FleetNameCrumb

export const TRAILS: ReadonlyArray<{ pattern: string; trail: readonly Crumb[] }> = [
  { pattern: '/',                  trail: [HOME] },
  { pattern: '/vehicles',          trail: [HOME, VEHICLES] },
  { pattern: '/vehicles/:id',      trail: [HOME, VEHICLES, { kind: 'vehicle' }] },
  …
  { pattern: '/admin',             trail: [ADMIN] },
  { pattern: '/admin/fleets',      trail: [ADMIN, FLEETS] },
  { pattern: '/admin/fleets/:id',  trail: [ADMIN, FLEETS, { kind: 'fleet' }] },
  …
];
```

Twelve rows, matching FR-CRUMB-4's twelve-row table one-for-one. Matching is
`matchPath({ path: pattern, end: true }, pathname)`, first hit wins; ordering is
irrelevant under `end: true` because no two patterns can match the same path.
Unmatched paths render nothing, which is the correct outcome for the routes in
FR-CRUMB-7 (they render no shell, so no breadcrumb exists to suppress).

The alternative — deriving the trail by walking pathname prefixes and looking up
a label per segment — is fewer lines but implicit: `/admin/fleets/:id` would
produce Admin / Fleets / «id» only if every ancestor happened to be a real route,
and the FR-CRUMB-2 rule that the admin trail is rooted at *Admin* rather than
*Home* becomes a special case rather than a table row. Explicit trails make
FR-CRUMB-8 literal — adding a route is adding a row — and make the whole
requirement testable as data (§8.2).

**One table, not one per shell.** The pattern determines the root crumb, so
`/admin/*` rows begin at `ADMIN` and fleet rows at `HOME`; the two shells cannot
disagree because they read the same table. `AppBreadcrumb` therefore takes no
props, exactly like `FrameHeader`.

### 6.4 Resolving crumbs

Each dynamic crumb is **its own component**, mounted only when its trail is the
active one. That is what makes FR-CRUMBNAME-7 structural rather than aspirational:
`VehicleNameCrumb` calls `useVehicle`, `FleetNameCrumb` calls `useAdminFleet`,
and neither component exists in the tree on a route whose trail does not name it.
No conditional hooks, no `enabled: false` gymnastics.

```tsx
function VehicleNameCrumb({ id }: { id: string }) {
  const { data, isLoading } = useVehicle(id);
  if (isLoading) return <Skeleton className="h-4 w-24" />;
  const a = data?.attributes;
  if (!a) return <>{id}</>;                                  // FR-CRUMBNAME-5
  return <>{a.nickname?.trim() || `${a.year} ${a.make} ${a.model}`}</>;
}
```

Three points of care:

- **The title rule is duplicated deliberately, then pinned by test.** The
  expression is byte-for-byte `VehicleDetailPage.tsx:129`. Extracting it into a
  shared `vehicleTitle(attributes)` helper is tempting and was considered; it is
  not done here because it means editing `VehicleDetailPage` during a frame task,
  and task-015 owns that page's title contract. Instead, a test asserts the two
  agree for nickname-present and nickname-absent inputs, which is what
  FR-CRUMBNAME-2 actually demands. If a third caller ever appears, extract then.
- **Fleet name is `data.attributes.name`.** Verified: `AdminService.getFleet`
  (`services/api/AdminService.ts:78-83`) returns `doc.data`, a
  `JsonApiResource<AdminFleetDetailAttributes>`, and `AdminFleetsPage.tsx:190`
  reads `data.attributes` before touching `.name`. The PRD's shorthand
  "`fleet.name`" means the attributes object, not the resource.
- **Failure falls back to the raw id, loading falls back to a skeleton.** The
  `data === undefined` branch covers error, 404, and soft-deleted alike.
  `Skeleton` is `aria-hidden` by construction (`ui/skeleton.tsx`), so the crumb
  announces nothing until the name lands — better than announcing a UUID.

**No extra network traffic (FR-CRUMBNAME-6).** `VehicleNameCrumb` calls the same
`useVehicle(id)` hook with the same `vehicleKeys.detail(id)` key that
`VehicleDetailPage.tsx:50` uses, and both mount in the same commit, so React
Query dedupes them into one in-flight request. Likewise `useAdminFleet(id)` /
`adminKeys.fleet(id)` against `AdminFleetsPage.tsx:174`. Note `useVehicle` also
carries `staleTime: 60_000`, so back-navigation to a recently viewed vehicle
resolves the crumb from cache with no request at all.

### 6.5 Overflow (open question 3)

Recommend **per-crumb truncation, no ellipsis collapse**: each crumb gets
`truncate max-w-[12rem]`, and intermediate crumbs are hidden below `sm` so a
phone shows only the current page's crumb beside the trigger. The longest real
trail is three crumbs (FR-CRUMB-4), so `BreadcrumbEllipsis` would be machinery
for a case that does not occur; the vendored primitive keeps it available if a
deeper route ever lands.

Responsive hiding is CSS-only. jsdom cannot see it, so the full trail stays in
the DOM at every width and the vitest assertions in §8.2 are unaffected — which
is also why FR-CRUMB-4's "renders its exact trail" stays honest.

---

## 7. Icons (open question 1)

All names verified present in the installed `lucide-react@0.400.0` d.ts.

Confirm the PRD's table as written, with one substitution: `CircleUser` for
`UserCircle` (§4.5).

- **`Car` for Vehicles** — confirmed. MyFleet's fleets are households with mixed
  vehicles; `Truck` would misdescribe most of them.
- **`Building2` for Fleets** — confirmed, with the reasoning recorded because the
  PRD flags it as uncertain. A MyFleet "fleet" is a household, which argues for
  `Home` — but `Home` in the admin sidebar would sit inches from "Back to my
  fleet" and read as that link's icon rather than as the fleets *collection*.
  `Building2` is the conventional tenant/organization glyph in admin consoles,
  and the admin console's job is to look at tenants. `Warehouse` is the runner-up
  and is a fine substitute if `Building2` reads too corporate in situ.

---

## 8. Testing

### 8.1 What the existing tests demand

Both layout tests break, and it is worth being explicit about how, since these
are the assertions the rewrite must consciously replace:

- `AppLayout.test.tsx` reaches for `screen.getByText('MyFleet').parentElement`
  and asserts an `svg` inside it. The wordmark moves inside a `<Link>`, so the
  parent chain changes. Replace with an assertion on the link's accessible name
  and `href` — which is what FR-BRAND-1/2/3 actually specify.
- Both tests assert `getByRole('button', { name: 'Sign out' })`. Sign out becomes
  a `menuitem` behind a closed dropdown; the replacement opens the menu first.
- `AdminLayout.test.tsx`'s `getByRole('link', { name: label })` loop survives
  unchanged — `SidebarMenuButton asChild` + `<Link>` keeps both the role and the
  name. That is a deliberate benefit of §4.3's `asChild` composition.

### 8.2 What gets added

- **`breadcrumbTrails.test.ts`** — a pure data test walking all twelve rows of
  FR-CRUMB-4 and asserting each concrete path resolves to its exact trail, plus
  that `/login`, `/onboarding` and an unknown path resolve to nothing. No render,
  no providers, exhaustive against the requirement. This is the highest-value
  test in the task.
- **Crumb resolution** — three states per resolver (skeleton / name / id
  fallback), driven by mocking the hook module the way the existing layout tests
  mock `../lib/hooks/api/auth`. Mocking the hook rather than standing up a
  `QueryClient` matches the file's established style and makes the loading state
  trivially reachable.
- **`identityLines`** — a unit test for the display-name → email → "Account"
  chain (FR-PROFILE-4), including the both-empty case that today's `?? ''` gets
  wrong.
- **Layout tests** — collapse toggling asserted on `data-state` / the provider's
  state rather than on width; brand link target; profile menu contents and its
  `logout()` call; the representative breadcrumb trail per shell.

### 8.3 jsdom gaps to expect

`src/test/setup.ts` already stubs `matchMedia` (which `useIsMobile` needs) and
`ResizeObserver` (added for cmdk). Radix's dropdown menu additionally touches
APIs jsdom does not implement — `Element.prototype.scrollIntoView`, and pointer
capture (`hasPointerCapture` / `setPointerCapture` / `releasePointerCapture`).
The app's existing Radix usage may not have hit these, so expect to add the
stubs to `setup.ts` alongside the existing ones, with a comment in the same style
explaining which library needs them. Flagging it here so it is a planned step
rather than a mid-implementation surprise.

Also: `document.cookie` persists across tests within a file. Any test file that
renders a shell must clear `sidebar_state` in `beforeEach`, or one test's
collapse leaks into the next.

One quirk of the existing `matchMedia` stub: every listener, whatever its query,
goes into one shared set and is called by `setPrefersDark`. `useIsMobile`'s
handler re-reads `window.innerWidth` and ignores the event, so the cross-talk is
harmless — but it is worth knowing before debugging a surprising re-render.

### 8.4 What vitest cannot prove

Per the PRD's testing section and the standing project note: **jsdom has no CSS
engine.** The collapsed rail's appearance, the hidden wordmark, the tooltip
surfacing on hover, and the responsive crumb hiding are all CSS-driven and are
invisible to vitest. Assert in vitest only on state, attributes, and roles;
verify the visual behaviour in real Chromium via the Playwright container per
`docs/runbooks/local-debugging.md`. The same applies to the FR-CRUMBNAME-6
network check, which is a devtools observation, not an assertion.

---

## 9. Open questions, resolved

| # | Question | Resolution |
|---|---|---|
| 1 | Icon choices | Confirmed as specified; `CircleUser` replaces the deprecated `UserCircle` alias (§7). |
| 2 | Admin brand-link target | Already resolved in the PRD: `/admin` for both lockup and first crumb. |
| 3 | Crumb truncation | `truncate max-w-[12rem]` per crumb; intermediate crumbs hidden below `sm`; no ellipsis collapse (§6.5). |
| 4 | `/admin/fleets/:id` hybrid page | Reads correctly. `AdminFleetsPage` renders list and detail as two panes of one grid (`AdminFleetsPage.tsx:23-32`); below `md` the list hides and only the detail shows. The URL identifies one fleet in both cases, so "Admin / Fleets / «name»" describes the page at every width. |
| 5 | `@/` path alias | Rewrite to relative imports (§2.1). No alias. |

## 10. Risks

- **The cookie deviation (§5) is the one place upstream's assumptions do not
  survive contact.** If it is skipped, everything still *looks* right — the
  cookie is written, the sidebar collapses — and the failure only shows on
  reload. It needs its own test.
- **`npm install` is a required first step.** This worktree has no
  `node_modules`, so `make ci` will fail confusingly until dependencies are
  installed and the root lockfile is updated.
- **Vendoring the Tailwind v4 variant of `sidebar.tsx` by mistake.** Current
  shadcn ships a v4-first source using `@theme` / OKLCH tokens. This repo is
  Tailwind 3.4.19 with HSL triples. The v3 variant is the one to take; a v4
  source will fail silently, rendering an unstyled sidebar rather than an error.
- **`data-active` regressions.** §4.3 replaces `NavLink`'s matching with an
  explicit `matchPath`. Getting `end` wrong on `/` or `/admin` lights every nav
  row on every route — cheap to test, easy to miss by eye.
