# Application Frame Navigation — Product Requirements Document

Version: v1
Status: Draft
Created: 2026-08-02
---

## 1. Overview

The authenticated application frame has not changed since the first task built it. Both
shells — `AppLayout` (the fleet app) and `admin/AdminLayout` (the platform admin console) —
render a hand-rolled fixed-width `<aside class="w-56">` beside a header that holds the
user's display name on the left and a theme toggle plus a bare "Sign out" button on the
right. The sidebar cannot be collapsed, its links are text-only, the brand lockup is inert,
and nothing in the frame tells the user where they are beyond which nav link is
highlighted. On `/vehicles/:id` and `/admin/fleets/:id` the frame gives no indication at
all of *which* vehicle or fleet is open — that information lives only in the page body.

This task replaces the hand-rolled frame in both shells with the shadcn `sidebar` primitive
and adds the navigational furniture that primitive assumes: an icon per nav link, a
collapse trigger, a route-driven breadcrumb beside that trigger, a clickable brand header,
and a profile dropdown that absorbs the display name and the sign-out action so the header's
right side is a single row of icon buttons.

The work is confined to `apps/web`. No Go service, API contract, or database schema
changes. It does, however, extend two existing cross-cutting contracts: the design-token
set established by task-003 (the shadcn sidebar expects a `--sidebar-*` family that this
repository does not yet define) and the vendored shadcn component set under
`apps/web/src/components/ui/` (which today has no `sidebar`, `dropdown-menu`, `tooltip`,
or `separator`).

## 2. Goals

Primary goals:

- Collapse the sidebar to an icon rail and back, with the choice persisted across reloads.
- Give every sidebar link a distinguishing icon, so the collapsed rail remains navigable.
- Replace the header's loose name text and "Sign out" button with a single profile
  dropdown sitting immediately right of the theme toggle.
- Tell the user where they are, on every authenticated route, via a breadcrumb that names
  domain objects instead of showing their UUIDs.
- Make the sidebar brand lockup a link home.
- Keep the two shells visually consistent — whatever `AppLayout` gains, `AdminLayout`
  gains, minus the things that are deliberately admin-specific.

Non-goals:

- Changing the route tree. `/admin` stays a sibling of the authenticated shell, not a
  child of it (see `App.tsx:71-78` — nesting it would reintroduce the fleetless-admin
  redirect that task-011 solved structurally).
- Changing page content, page headers, or the `PageHeader` component. Task-015 owns the
  page-title contract and this task does not touch it. The breadcrumb is frame furniture
  that sits *above* the page, not a replacement for the page's `<h1>`.
- A mobile-specific redesign beyond the off-canvas sheet behaviour the shadcn sidebar
  primitive provides for free.
- Persisting the collapse choice server-side or syncing it across devices.
- Merging `AppLayout` and `AdminLayout` into one component. They stay separate files
  (task-011 made that a deliberate structural choice); they merely share the same
  primitives and the same header composition.
- Adding search, command palette, or notification badges to the frame.

## 3. User Stories

- As a signed-in user on a small laptop, I want to collapse the sidebar to an icon rail so
  that the content area gets the width, and I want that choice remembered the next time I
  open the app.
- As a signed-in user looking at the collapsed rail, I want each link to carry a
  recognisable icon and a tooltip so that I can still navigate without the labels.
- As a signed-in user, I want my name and the sign-out action gathered under one profile
  button beside the theme toggle so that the header reads as a tidy row of controls rather
  than a name floating opposite two mismatched buttons.
- As a signed-in user three levels deep, I want a breadcrumb across the top telling me the
  path I took so that I can step back one level without using the browser's back button.
- As a signed-in user viewing a specific vehicle, I want the breadcrumb to say
  "Weekend Truck", not `8f14e45f-ceea-467a-9f8e-1b2c3d4e5f60`.
- As a platform admin viewing a specific fleet, I want the same — the fleet's name in the
  breadcrumb, not its id.
- As a signed-in user anywhere in the app, I want clicking the MyFleet lockup in the
  sidebar — or the "Home" crumb — to take me to the dashboard.

## 4. Functional Requirements

### 4.1 Sidebar primitive (FR-SIDEBAR)

- **FR-SIDEBAR-1** — Vendor the shadcn `sidebar` component into
  `apps/web/src/components/ui/sidebar.tsx`, along with its unmet peer components
  `dropdown-menu.tsx`, `tooltip.tsx`, and `separator.tsx`. `sheet.tsx`, `skeleton.tsx`,
  `input.tsx`, and `button.tsx` already exist and MUST be reused rather than re-vendored.
- **FR-SIDEBAR-2** — Vendored sources MUST be rewritten to this project's import style.
  `apps/web` has **no `@/` path alias** (verified: no `paths` entry in `tsconfig.json`, no
  `resolve.alias` in `vite.config.ts`); every import in the vendored files is relative
  today and MUST remain so. Either rewrite the shadcn `@/...` imports to relative paths, or
  introduce the alias as an explicit, separately-justified decision — not silently.
- **FR-SIDEBAR-3** — Both shells render `<Sidebar collapsible="icon">` inside a
  `SidebarProvider`, with the page content inside `SidebarInset`.
- **FR-SIDEBAR-4** — A `SidebarTrigger` sits at the far left of the header row in both
  shells and toggles between the expanded sidebar and the icon rail.
- **FR-SIDEBAR-5** — The expanded/collapsed state persists in the `sidebar_state` cookie
  written by `SidebarProvider` (the shadcn default). No server-side preference, no
  migration, no change to the auth preferences endpoint. On a fresh browser with no
  cookie, the sidebar starts expanded.
- **FR-SIDEBAR-6** — Below the primitive's mobile breakpoint the sidebar becomes the
  off-canvas sheet the primitive provides. The trigger remains visible and functional at
  every width.
- **FR-SIDEBAR-7** — `SidebarRail` is rendered so the sidebar edge is draggable/clickable
  to toggle, matching the reference implementation.

### 4.2 Design tokens (FR-TOKEN)

- **FR-TOKEN-1** — `apps/web/src/index.css` defines no `--sidebar-*` custom properties
  today. The shadcn sidebar consumes `--sidebar`, `--sidebar-foreground`,
  `--sidebar-primary`, `--sidebar-primary-foreground`, `--sidebar-accent`,
  `--sidebar-accent-foreground`, `--sidebar-border`, and `--sidebar-ring`. All MUST be
  defined for **both** the light `:root` block and the `.dark` block, and registered in
  `tailwind.config.ts` under the existing `colors:` extension so `bg-sidebar` and friends
  resolve.
- **FR-TOKEN-2** — The sidebar surface MUST read as the same surface it does today, which
  is `bg-card`, **not** `bg-muted`. `AppLayout.tsx:24-28` records why: `--muted` and
  `--accent` are the same value in both themes, so a muted sidebar swallows both the
  active-link and hover states and flattens the nav into one colour. The new
  `--sidebar-accent` MUST be distinguishable from `--sidebar` in both themes; a reviewer
  MUST be able to see the active link and the hover state as distinct from the surface.
- **FR-TOKEN-3** — Existing tokens are not redefined or repurposed. This requirement adds
  a token family; it does not amend task-003's contract for the tokens already there.

### 4.3 Sidebar navigation (FR-NAV)

- **FR-NAV-1** — Every nav item in both shells carries a `lucide-react` icon.
  `lucide-react@^0.400.0` is already a dependency; no new icon package.
- **FR-NAV-2** — Icons for the fleet shell (`AppLayout`):

  | Route | Label | Icon |
  |---|---|---|
  | `/` | Dashboard | `LayoutDashboard` |
  | `/vehicles` | Vehicles | `Car` |
  | `/activity` | Activity | `Activity` |
  | `/notifications` | Notifications | `Bell` |
  | `/settings` | Settings | `Settings` |
  | `/admin` | Admin | `Shield` |

- **FR-NAV-3** — Icons for the admin shell (`AdminLayout`):

  | Route | Label | Icon |
  |---|---|---|
  | `/admin` | Overview | `LayoutDashboard` |
  | `/admin/fleets` | Fleets | `Building2` |
  | `/admin/users` | Users | `Users` |
  | `/admin/purges` | Purges | `Trash2` |
  | `/admin/audit` | Audit log | `ScrollText` |
  | `/` | Back to my fleet | `ArrowLeft` |

- **FR-NAV-4** — Each nav item is a `SidebarMenuButton` with `tooltip={label}`, so the
  collapsed rail surfaces the label on hover. The tooltip MUST NOT appear while the
  sidebar is expanded (the primitive's default behaviour).
- **FR-NAV-5** — Active-state detection MUST preserve today's semantics. `/` uses exact
  matching (`end`) so it does not light up on every route; the rest match on prefix so
  `/vehicles/:id` keeps "Vehicles" active. `/admin` in the fleet shell uses exact matching
  for the same reason `/` does.
- **FR-NAV-6** — The `/admin` entry stays conditional on `platformAdmin` and stays in the
  sidebar. `AppLayout.tsx:18-20` records that this is a convenience door, not a control —
  the server refuses entry regardless (FR-ADMIN-UI-5). That comment MUST survive the
  rewrite.
- **FR-NAV-7** — The admin shell's "Back to my fleet" link keeps its visual separation
  from the admin nav proper (today: a `border-t` block below the nav). It is a departure
  from the console, not a sixth console destination.

### 4.4 Brand header (FR-BRAND)

- **FR-BRAND-1** — The sidebar header — `BrandMark` plus wordmark — becomes a
  `react-router` `Link`.
- **FR-BRAND-2** — In `AppLayout` the target is `/`. In `AdminLayout` the target is
  `/admin`, so the console's own lockup returns to the console overview rather than
  ejecting the operator from the console. Leaving the console is what "Back to my fleet"
  is for.
- **FR-BRAND-3** — The link carries an accessible name (e.g. `aria-label="MyFleet home"`,
  `aria-label="MyFleet admin home"`) because the wordmark is hidden in the collapsed rail.
- **FR-BRAND-4** — Collapsed, the header shows the `BrandMark` centred and hides the
  wordmark. The admin shell's `MyFleet admin` two-part wordmark collapses to the mark
  alone.
- **FR-BRAND-5** — The link has a visible hover state and a visible keyboard focus ring.

### 4.5 Profile dropdown (FR-PROFILE)

- **FR-PROFILE-1** — The header's left-aligned display-name text and its right-aligned
  "Sign out" `Button` are both removed. They are replaced by one profile dropdown placed
  **immediately right of the theme toggle**.
- **FR-PROFILE-2** — The trigger is an icon-sized `Button variant="ghost"` with an
  accessible name (e.g. `aria-label="Account menu"`). It renders `user.attributes.avatarUrl`
  when that is a non-empty string, and falls back to a `UserCircle` icon otherwise.
  `UserAttributes` already carries `email`, `displayName`, and `avatarUrl`
  (`types/models/user.ts`).
- **FR-PROFILE-3** — The menu contains exactly two regions:
  1. A non-interactive header showing `displayName` on the first line and `email` on the
     second, muted and smaller. Both truncate rather than widening the menu.
  2. A separator, then a "Sign out" item that calls the existing `logout()` from
     `useAuth()`.
  No Settings link and no admin link — Settings and Admin remain sidebar destinations.
- **FR-PROFILE-4** — When `displayName` is empty the header falls back to `email`; when
  both are empty it reads "Account". Today's header renders `?? ''`, i.e. nothing, which
  would leave an empty menu label.
- **FR-PROFILE-5** — The menu is keyboard-operable: the trigger is reachable by Tab,
  Enter/Space opens it, arrow keys move between items, Escape closes and returns focus to
  the trigger. This is Radix `DropdownMenu` default behaviour and MUST NOT be defeated.
- **FR-PROFILE-6** — Both shells use the same profile dropdown component. It is defined
  once and imported by both, not copy-pasted.
- **FR-PROFILE-7** — The theme toggle is unchanged. `ThemeToggle` wraps
  `ThemeToggleButton` and fires the persistence mutation; the login page uses
  `ThemeToggleButton` directly because there is no session there (FR-PRETOGGLE-3). This
  task MUST NOT fold the theme control into the profile menu — doing so would drag the
  session-bound mutation onto the signed-out login page.

### 4.6 Breadcrumb (FR-CRUMB)

- **FR-CRUMB-1** — A breadcrumb renders in the header row, immediately right of the
  `SidebarTrigger`, in both shells.
- **FR-CRUMB-2** — The first crumb is always **Home** and always links to `/`, in both
  shells — including inside the admin console. This is the user's stated requirement and
  is consistent with the console's existing "Back to my fleet" affordance.
- **FR-CRUMB-3** — The last crumb represents the current page. It is rendered as
  non-interactive current-page text (`aria-current="page"`), not a link. All preceding
  crumbs are links.
- **FR-CRUMB-4** — Every authenticated route resolves to a defined trail. The complete
  map:

  | Route | Trail |
  |---|---|
  | `/` | Home |
  | `/vehicles` | Home / Vehicles |
  | `/vehicles/:id` | Home / Vehicles / *«vehicle name»* |
  | `/activity` | Home / Activity |
  | `/notifications` | Home / Notifications |
  | `/settings` | Home / Settings |
  | `/admin` | Home / Admin |
  | `/admin/fleets` | Home / Admin / Fleets |
  | `/admin/fleets/:id` | Home / Admin / Fleets / *«fleet name»* |
  | `/admin/users` | Home / Admin / Users |
  | `/admin/purges` | Home / Admin / Purges |
  | `/admin/audit` | Home / Admin / Audit log |

- **FR-CRUMB-5** — On `/` the breadcrumb renders the single "Home" crumb as current-page
  text. It does not render an empty region that collapses the header height.
- **FR-CRUMB-6** — The intermediate "Admin" crumb links to `/admin`, and the intermediate
  "Vehicles" and "Fleets" crumbs link to `/vehicles` and `/admin/fleets` respectively.
- **FR-CRUMB-7** — Routes outside both shells — `/login`, `/onboarding`, and
  `/invites/:token/accept` — render no breadcrumb, because they render no shell.
- **FR-CRUMB-8** — The route-to-trail mapping is data, defined in one module, not a chain
  of conditionals spread through the layouts. Adding a route means adding a table entry.
- **FR-CRUMB-9** — A trail longer than the header width MUST NOT push the theme toggle and
  profile menu off-screen or force the page body to scroll horizontally. Truncate the
  crumb labels (each crumb `truncate` with a sensible `max-w-*`) rather than wrapping the
  header to two lines.

### 4.7 Object-name resolution in the breadcrumb (FR-CRUMBNAME)

- **FR-CRUMBNAME-1** — A crumb standing for a `:id` route parameter displays the object's
  name, never its UUID, in the success case.
- **FR-CRUMBNAME-2** — Vehicle name resolution uses the existing `useVehicle(id)` hook
  (`lib/hooks/api/vehicles.ts:35`). The displayed name is
  `attributes.nickname?.trim() || \`${attributes.year} ${attributes.make} ${attributes.model}\``
  — byte-for-byte the rule `VehicleDetailPage.tsx:129` already uses for the page title.
  The two MUST agree; a page titled "Weekend Truck" under a breadcrumb reading
  "2019 Ford F-150" is a defect.
- **FR-CRUMBNAME-3** — Fleet name resolution uses the existing `useAdminFleet(id)` hook
  (`lib/hooks/api/admin.ts:51`) and displays `fleet.name`.
- **FR-CRUMBNAME-4** — **While the lookup is in flight**, the crumb renders a `Skeleton`
  sized to a short label. It does not render the UUID, and it does not render an empty
  space that makes the trail jump when the name lands.
- **FR-CRUMBNAME-5** — **When the lookup fails, errors, or 404s**, the crumb falls back to
  the raw UUID from the URL, so the segment still identifies the object for support and
  debugging. (Chosen deliberately over a generic "Vehicle" label: a breadcrumb that
  silently degrades to a category name hides that a lookup failed.)
- **FR-CRUMBNAME-6** — Name resolution MUST reuse the React Query cache the detail page
  populates. Navigating to `/vehicles/:id` MUST NOT issue a second, separate request for
  the same vehicle solely to fill the breadcrumb. Same query key, same hook.
- **FR-CRUMBNAME-7** — Resolution hooks are only invoked on routes that need them. The
  breadcrumb MUST NOT call `useAdminFleet` while on `/vehicles`, or `useVehicle` while on
  `/settings`. React's rules of hooks make this a structural requirement, not an
  optimisation: the resolving crumb is its own component, mounted only when the matching
  route pattern is active.

### 4.8 Header layout (FR-HEADER)

- **FR-HEADER-1** — The header row in both shells reads, left to right:
  `SidebarTrigger` · breadcrumb · *(flexible gap)* · `ThemeToggle` · profile dropdown.
- **FR-HEADER-2** — The header keeps a fixed height across every route, whether or not a
  crumb is currently showing a skeleton.
- **FR-HEADER-3** — The admin shell's platform-admin danger band is preserved verbatim —
  its copy, its `danger-subtle` tokens, and its position directly beneath the header and
  above `<main>`. `AdminLayout.tsx:75-89` records why it uses `danger-subtle` rather than
  `--destructive` (task-003 reserves `--destructive` for destructive *controls*) and why
  it states the 15-minute stale-claim caveat in plain words. Both comments MUST survive.
- **FR-HEADER-4** — `<main>` keeps its current `p-6` padding so no page's content shifts.

## 5. API Surface

None. No new endpoints, no modified request or response shapes, no new error cases.

The breadcrumb consumes two existing read endpoints through their existing hooks:

- `GET /api/fleet/vehicles/{id}` via `useVehicle` — already called by `VehicleDetailPage`
  on exactly the routes where the breadcrumb needs it, so the cache is warm.
- the admin single-fleet read via `useAdminFleet` — likewise already called by
  `AdminFleetsPage` on `/admin/fleets/:id`.

Neither call is new traffic (FR-CRUMBNAME-6).

## 6. Data Model

No entity, field, relationship, constraint, or migration changes.

Client-side persisted state gains exactly one item: the `sidebar_state` cookie written by
shadcn's `SidebarProvider` (FR-SIDEBAR-5). It is a browser-local UI preference, not user
data, and it is not synced to the server.

## 7. Service Impact

| Service | Change |
|---|---|
| `apps/web` | All of it. See below. |
| `apps/auth-service` | None. |
| `apps/fleet-service` | None. |
| all other Go services | None. |
| `deploy/k8s` | None. |
| `packages/ui-components` | None — see note. |

**`apps/web` changes:**

- New: `components/ui/sidebar.tsx`, `components/ui/dropdown-menu.tsx`,
  `components/ui/tooltip.tsx`, `components/ui/separator.tsx` (vendored shadcn).
- New: a breadcrumb component and its route→trail table module.
- New: a shared profile-dropdown component consumed by both shells (FR-PROFILE-6).
- Rewritten: `components/AppLayout.tsx`, `components/admin/AdminLayout.tsx`.
- Amended: `index.css` (the `--sidebar-*` families, light and dark),
  `tailwind.config.ts` (register them).
- New deps: `@radix-ui/react-dropdown-menu`, `@radix-ui/react-tooltip`,
  `@radix-ui/react-separator`. `lucide-react`, `@radix-ui/react-dialog` (for the mobile
  sheet), and `@radix-ui/react-slot` are already present.
- Updated tests: `components/AppLayout.test.tsx`,
  `components/admin/AdminLayout.test.tsx`.

**Note on `packages/ui-components`:** these components stay in `apps/web`. The frame is
app-shell furniture, not a design-system primitive — the same reasoning `PageHeader.tsx:9-12`
records for keeping `PageHeader` out of the shared package.

## 8. Non-Functional Requirements

**Accessibility**

- Sidebar nav is a `<nav>` landmark; the breadcrumb is a `<nav aria-label="Breadcrumb">`.
- The current page's crumb carries `aria-current="page"`.
- Every icon-only control — the sidebar trigger, the theme toggle, the profile trigger,
  and each collapsed nav link — has an accessible name. Decorative icons are
  `aria-hidden="true"`.
- Keyboard: the whole frame is reachable and operable by Tab/Shift-Tab, with a visible
  focus ring on the brand link, every nav link, the trigger, and the profile menu.
- The collapse toggle is not the only route to any destination — the collapsed rail keeps
  all links.

**Performance**

- No new network requests on any route (FR-CRUMBNAME-6).
- No layout shift when a resolved name replaces its skeleton beyond the crumb's own width
  (FR-HEADER-2).
- The sidebar's collapse transition must not re-mount `<Outlet />` content or drop React
  Query state.

**Security**

- The `/admin` sidebar entry stays a visibility convenience, never an authorization
  decision (FR-NAV-6). Hiding it does not protect anything; the server does.
- The profile menu renders `displayName` and `email` as text. No `dangerouslySetInnerHTML`.
- The `sidebar_state` cookie holds a boolean UI preference and nothing identifying.

**Observability**

- None required. No new telemetry, metrics, or log lines.

**Testing**

- `AppLayout.test.tsx` and `AdminLayout.test.tsx` are updated, not deleted, and cover:
  collapse toggling, the brand link's target, the profile menu's contents and its
  sign-out call, and the breadcrumb trail for a representative route in each shell.
- Breadcrumb name resolution is tested for all three states: loading (skeleton), resolved
  (name), and failed (UUID fallback).
- **jsdom cannot see CSS.** Anything that depends on the collapsed rail's *appearance* —
  the hidden wordmark, the icon-only nav, the tooltip surfacing on hover — cannot be
  proven by the vitest suite. Verify those in real Chromium via the Playwright container
  per `docs/runbooks/local-debugging.md`, and assert in vitest only on state, attributes,
  and roles.

## 9. Open Questions

1. **Icon choices** (FR-NAV-2, FR-NAV-3) are proposals. `Car` for Vehicles and `Building2`
   for Fleets are the least certain; confirm during design.
2. **Admin brand-link target** (FR-BRAND-2): this PRD sends the console's lockup to
   `/admin` rather than `/`, reasoning that "Back to my fleet" is the existing way out of
   the console. The user's original request said "the root url" without distinguishing the
   shells. Confirm during design.
3. **Crumb truncation width** (FR-CRUMB-9): the exact `max-w-*` per crumb, and whether
   long trails should collapse middle crumbs to an ellipsis (shadcn ships a
   `BreadcrumbEllipsis`) rather than truncating each label. The longest real trail is four
   crumbs, so truncation is probably sufficient.
4. **`/admin/fleets/:id`** renders the same `AdminFleetsPage` component as
   `/admin/fleets` (`App.tsx:88-89`). Confirm the fleet-detail crumb reads sensibly given
   that the page is a list-or-detail hybrid.
5. **`@/` path alias** (FR-SIDEBAR-2): rewriting shadcn's imports to relative paths is the
   low-risk default and what this PRD assumes. Introducing the alias would be a
   repo-wide convention change and belongs in its own task if wanted.

## 10. Acceptance Criteria

**Sidebar**

- [ ] Clicking the trigger collapses the sidebar to an icon rail; clicking again expands it.
- [ ] Reloading the page preserves the collapsed/expanded choice.
- [ ] A fresh browser with no `sidebar_state` cookie loads with the sidebar expanded.
- [ ] Both `AppLayout` and `AdminLayout` are collapsible.
- [ ] `--sidebar-*` tokens are defined in both the light and dark blocks of `index.css`
      and registered in `tailwind.config.ts`.
- [ ] In both themes, the active nav link and the hover state are visibly distinct from
      the sidebar surface (FR-TOKEN-2).

**Nav icons**

- [ ] Every link in both sidebars renders an icon.
- [ ] Collapsed, hovering a rail icon surfaces its label as a tooltip.
- [ ] `/` does not show as active when on `/vehicles`; `/vehicles` shows as active when on
      `/vehicles/:id`.
- [ ] The Admin link appears only for `platformAdmin` users.

**Brand header**

- [ ] Clicking the MyFleet lockup in `AppLayout` navigates to `/`.
- [ ] Clicking the lockup in `AdminLayout` navigates to `/admin`.
- [ ] Collapsed, the mark remains visible and the wordmark is hidden.
- [ ] The link has an accessible name and a visible focus ring.

**Profile dropdown**

- [ ] The header shows no loose display-name text and no standalone "Sign out" button.
- [ ] A profile trigger sits immediately right of the theme toggle in both shells.
- [ ] Opening it shows the display name and the email.
- [ ] "Sign out" calls `logout()` and ends the session.
- [ ] The trigger shows the avatar when `avatarUrl` is set and a fallback icon otherwise.
- [ ] The menu opens with Enter, moves with arrow keys, and closes with Escape.
- [ ] The login page's theme toggle still works signed-out and fires no session mutation.

**Breadcrumb**

- [ ] A breadcrumb sits immediately right of the sidebar trigger in both shells.
- [ ] Every route in the FR-CRUMB-4 table renders its exact trail.
- [ ] "Home" links to `/` from every authenticated route, including inside `/admin`.
- [ ] The final crumb is non-interactive and carries `aria-current="page"`.
- [ ] `/vehicles/:id` shows the vehicle's name and never its UUID on a successful load.
- [ ] `/admin/fleets/:id` shows the fleet's name and never its UUID on a successful load.
- [ ] The vehicle crumb's text matches `VehicleDetailPage`'s `<h1>` exactly.
- [ ] While a name resolves, the crumb shows a skeleton.
- [ ] When a name lookup fails, the crumb shows the raw UUID.
- [ ] Navigating to a vehicle detail page issues no additional network request for the
      breadcrumb (verified in the network panel).
- [ ] A long name truncates; the theme toggle and profile menu stay on screen and the page
      body does not scroll horizontally.

**Preserved behaviour**

- [ ] `/admin` remains a route-tree sibling of the authenticated shell; a fleetless
      platform admin still reaches every admin screen without being redirected to
      `/onboarding`.
- [ ] The platform-admin danger band renders unchanged, with its copy and its
      `danger-subtle` tokens intact.
- [ ] "Back to my fleet" still leaves the console for `/`.
- [ ] No page's content position changes (`<main>` keeps `p-6`).

**Verification**

- [ ] `make ci` passes.
- [ ] `AppLayout.test.tsx` and `AdminLayout.test.tsx` cover collapse, brand link, profile
      menu, and breadcrumb.
- [ ] Collapsed-rail appearance and tooltip behaviour verified in real Chromium, not
      asserted in jsdom.
