# Application Frame Navigation — Implementation Context

Task: task-017-app-frame-navigation
Worktree: `.worktrees/task-017-app-frame-navigation`, branch `task-017-app-frame-navigation`
PRD: [prd.md](./prd.md) · Design: [design.md](./design.md) · Plan: [plan.md](./plan.md)

---

## 1. What this task is

Replace the hand-rolled shells in `apps/web/src/components/AppLayout.tsx` (72 lines) and
`apps/web/src/components/admin/AdminLayout.tsx` (96 lines) with the shadcn `sidebar`
primitive, and add the furniture that primitive assumes: per-link icons, a collapse
trigger with cookie persistence, a clickable brand lockup, a route-driven breadcrumb that
names objects rather than showing UUIDs, and a shared profile dropdown.

The two files are ~80% identical today and have already drifted once — the `bg-card`
rationale comment exists only in `AppLayout`, though `AdminLayout` depends on the same
fact. The rewrite factors the shared parts into `src/components/frame/` so each layout is
left holding only what differs: its nav table, its brand target, and (admin only) the
danger band and the exit link.

**Scope is `apps/web` alone.** No Go service, no API contract, no database, no
`deploy/k8s`, no route-tree change, nothing in `packages/ui-components`.

---

## 2. Files, and why each exists

### New — vendored shadcn primitives (`src/components/ui/`)

| File | Note |
|---|---|
| `sidebar.tsx` | The primitive. Three documented deviations — see §5. |
| `dropdown-menu.tsx` | Backs `ProfileMenu`. |
| `tooltip.tsx` | Imported by `sidebar.tsx`; surfaces labels on the collapsed rail. |
| `separator.tsx` | Imported by `sidebar.tsx`; also the admin footer divider. |
| `breadcrumb.tsx` | The design's addition beyond FR-SIDEBAR-1's four. No new dependency; supplies every structural a11y attribute the PRD asks for. |

`button.tsx`, `input.tsx`, `sheet.tsx`, `skeleton.tsx` already exist and are **reused**, not
re-vendored (FR-SIDEBAR-1).

### New — the frame (`src/components/frame/`)

| File | Responsibility |
|---|---|
| `FrameHeader.tsx` | The header row both shells render. Propless. |
| `ProfileMenu.tsx` | Identity + sign out. Defined once, imported by both shells (FR-PROFILE-6). |
| `identityLines.ts` | `displayName → email → "Account"`. A pure function worth testing alone. |
| `BrandLink.tsx` | The sidebar lockup, as a link home. |
| `FrameNav.tsx` | One renderer, two tables. Computes `isActive` itself. |
| `AppBreadcrumb.tsx` | Renders the resolved trail. Propless. |
| `breadcrumbTrails.ts` | The route→trail table. Data, not conditionals. |
| `crumbs/VehicleNameCrumb.tsx` | Resolves `/vehicles/:id` via `useVehicle`. |
| `crumbs/FleetNameCrumb.tsx` | Resolves `/admin/fleets/:id` via `useAdminFleet`. |

`frame/` is a new folder. The alternative — nine loose files beside `BrandMark.tsx` and
`ThemeToggle.tsx` in an already-flat `src/components/` — was rejected: the tree already
groups by concern (`admin/`, `features/`, `providers/`, `ui/`), and the frame is a coherent
unit with one entry point per shell.

### New — elsewhere

- `src/lib/hooks/useIsMobile.ts` — shadcn ships this as `hooks/use-mobile.tsx`; this repo
  puts hooks in `src/lib/hooks/<camelCase>.ts`.
- `src/test/sidebarTokens.test.ts` — pins the token family and its mirror relationships.

### Rewritten

- `src/components/AppLayout.tsx`, `src/components/admin/AdminLayout.tsx` and both their
  test files.

### Amended

- `src/index.css` — the `--sidebar-*` family, light and dark, plus the `bg-card` rationale
  comment moved here from `AppLayout`.
- `tailwind.config.ts` — registers the family under `theme.extend.colors.sidebar`.
- `src/test/setup.ts` — Radix's jsdom gaps (`scrollIntoView`, pointer capture).
- `package.json` (`apps/web`) and the root `package-lock.json` — three Radix packages.

---

## 3. Decisions already made — do not relitigate

| Decision | Where it was settled | Why |
|---|---|---|
| Relative imports, no `@/` alias | PRD Q5, design §2.1 | `tsconfig.app.json` has no `paths`; `vite.config.ts` has no `resolve.alias`. Adding one is a repo-wide convention change and belongs in its own task. |
| Tailwind **v3** shadcn sources | design §10 | This repo is Tailwind 3.4 with HSL triples. A v4 source fails silently — an unstyled sidebar, not an error. |
| `tailwindcss-animate` stays uninstalled | design §2.4 | Adding it would make three menus animate while every existing dialog and sheet does not. Stripping the classes would diverge from upstream for no gain. Inert is the status quo. |
| Sidebar surface mirrors `--card`, not `--muted` | FR-TOKEN-2, design §3.1 | `--muted` and `--accent` are the **same value** in both themes, so a muted sidebar swallows both the active-link and the hover state. |
| Literal token values, not `var(--card)` aliases | design §3.2 | `index.css` has zero `var()` indirection today; one aliased family would be a second idiom. `sidebarTokens.test.ts` pays the drift cost. |
| Console lockup **and** first crumb both target `/admin` | PRD Q2 (resolved), FR-BRAND-2, FR-CRUMB-2 | The two "go to the top" affordances must agree. The console is a sibling of the dashboard, not a descendant. "Back to my fleet" is the single, deliberate exit. |
| Explicit trails, not a pathname-prefix walk | design §6.3 | Prefix walking makes FR-CRUMB-2's admin rooting a special case and the whole thing untestable as data. |
| `matchPath` over `useMatches()` | design §6.2 | `useMatches` throws under `<BrowserRouter>` — verified at `node_modules/react-router/dist/index.js:700-704, 752`. Migrating to `createBrowserRouter` would touch the route tree `postPurgeRouting.test.tsx` guards. |
| Active state computed, `NavLink` dropped | design §4.3 | `SidebarMenuButton` needs `isActive` as a value to set `data-active`; `NavLink` only hands it to a render prop inside the anchor. |
| `FrameHeader` and `AppBreadcrumb` take no props | design §4.2, §6.3 | Every input is ambient context both shells already provide. Props would only be a chance to disagree. |
| One `ProfileMenu`, two layout files | FR-PROFILE-6, task-011 | The shells stay separate (a structural decision task-011 made); what they share is components, not copied JSX. |
| `CircleUser`, not `UserCircle` | design §7 | Verified in `lucide-react@0.400.0`: `UserCircle` is a deprecated alias. |
| No `avatar.tsx` | design §4.5 | Would add `@radix-ui/react-avatar` solely for an image-with-fallback in a 24px circle. |
| Theme toggle stays out of the profile menu | FR-PROFILE-7 | `ThemeToggle` fires a session-bound mutation; `ThemeToggleButton` exists so the signed-out login page can render the control without one. |
| Per-crumb truncation, no ellipsis collapse | PRD Q3, design §6.5 | The longest real trail is three crumbs. `BreadcrumbEllipsis` stays available in the vendored file. |
| `Car` for Vehicles, `Building2` for Fleets | PRD Q1, design §7 | `Truck` would misdescribe most household vehicles. `Home` for Fleets would sit inches from "Back to my fleet" and read as that link's icon. |

---

## 4. Dependencies

**Added:** `@radix-ui/react-dropdown-menu`, `@radix-ui/react-tooltip`,
`@radix-ui/react-separator` — verified absent from `package-lock.json` and from
`node_modules/@radix-ui/` in the main checkout. Pin with the `^1.1.x` caret style the
sibling Radix entries use (npm does this automatically).

**Already present:** `@radix-ui/react-slot`, `@radix-ui/react-dialog` (backs `sheet.tsx`
and therefore the mobile sidebar), `class-variance-authority`, `lucide-react@^0.400.0`.

**This worktree has no `node_modules`.** `npm install` at the repo root is the first step of
Task 1; `make ci` fails confusingly until it runs, and the root `package-lock.json` is part
of the diff.

---

## 5. Deviations from upstream shadcn

All five are commented in the vendored files themselves so a future diff against upstream
shows them as intentional.

1. **`sidebar.tsx` reads the `sidebar_state` cookie** (design §5). Upstream writes it on
   every toggle but derives its *initial* value from a `defaultOpen` prop that its Next.js
   reference app fills by reading the cookie **on the server**. MyFleet is a Vite SPA: with
   no server render nothing fills the prop, so the cookie would be written and never read
   and the sidebar would reopen expanded on every reload. `readSidebarCookie()` lives in
   the same file as the write, and the `useState` initializer is lazy so the cookie is
   parsed once. `SameSite=Lax` is added — free hardening on a UI boolean.
   **This is the one place where skipping the work still *looks* right:** the cookie is
   written, the sidebar collapses, and the failure only shows on reload. It has its own
   tests (Task 4).
2. **`SidebarInset` renders a `<div>`, not a `<main>`.** Both layouts render their own
   `<main className="flex-1 p-6">` inside it (FR-HEADER-4); nesting `<main>` is invalid.
3. **`SidebarRail` is `aria-hidden="true"`.** It duplicates `SidebarTrigger` and is already
   `tabIndex={-1}`; upstream's `aria-label="Toggle Sidebar"` would put two identically
   named controls in the a11y tree (and make role queries ambiguous in tests).
4. **`BreadcrumbPage` drops `role="link" aria-disabled="true"`.** FR-CRUMB-3 wants
   non-interactive text, and a disabled link still answers `getByRole('link', …)`.
5. **`Breadcrumb`'s label is `"Breadcrumb"`,** capitalised to match the PRD's §8 markup,
   rather than upstream's lowercase.

---

## 6. Testing notes

- **`breadcrumbTrails.test.ts` is the highest-value test in the task**: pure data, no
  render, no providers, and exhaustive against all twelve rows of FR-CRUMB-4.
- **Mock the hook module, don't stand up a `QueryClient`.** That matches the established
  style in the existing layout tests and makes the loading state — otherwise a race —
  trivially reachable.
- **`document.cookie` persists across tests within a file** (design §8.3). Every test file
  that renders a shell clears `sidebar_state` in `beforeEach`, or one test's collapse leaks
  into the next.
- **Radix needs jsdom stubs** `Element.prototype.scrollIntoView`, `hasPointerCapture`,
  `setPointerCapture`, `releasePointerCapture` — added to `src/test/setup.ts` alongside the
  existing `matchMedia` and `ResizeObserver` stubs.
- **One quirk of the existing `matchMedia` stub:** every listener, whatever its query, goes
  into one shared set and is called by `setPrefersDark`. `useIsMobile`'s handler re-reads
  `window.innerWidth` and ignores the event, so the cross-talk is harmless — worth knowing
  before debugging a surprising re-render.
- **`src/test/conventions.test.ts` scans every `.tsx` under `src`** for hardcoded palette
  classes. A vendored file is exactly where one could sneak in; Task 4 runs that file
  explicitly.
- **`postPurgeRouting.test.tsx` imports the real route tree** and renders the console at
  five routes. It is unmodified by this task and is the check that the rewritten shells
  still work inside `App.tsx`. Its existing `vi.mock('../../lib/hooks/api/admin', …)` block
  already exports `useAdminFleet`, which is what `FleetNameCrumb` needs.
- **jsdom has no CSS engine.** The collapsed rail's appearance, the hidden wordmark, the
  tooltip on hover, and the responsive crumb hiding are invisible to vitest. Assert in
  vitest only on state, attributes and roles; verify the rest in real Chromium via the
  Playwright container per `docs/runbooks/local-debugging.md` §4. The same applies to the
  FR-CRUMBNAME-6 network check, which is a devtools observation.

---

## 7. Risks

| Risk | Mitigation |
|---|---|
| Vendoring the Tailwind **v4** variant of `sidebar.tsx` | Fails silently — unstyled sidebar, no error. The plan's listings are already v3. If you re-fetch from upstream, check for `@theme`/OKLCH and reject. |
| The cookie deviation gets skipped | Everything still looks right until you reload. Task 4's cookie tests are the guard. |
| `data-active` regression from a wrong `end` flag | `/` and the fleet shell's `/admin` need `end: true`, or every nav row lights on every route. Task 12 tests both directions. |
| `npm install` forgotten | `make ci` fails confusingly. It is Task 1, step 1. |
| The `bg-card` comment gets dropped rather than moved | It is the record of a real trap. It moves to `index.css` beside the tokens in Task 1 and must not simply vanish with the old `AppLayout`. |
| Two `main` landmarks | Handled by rendering `SidebarInset` as a `div`; `<main>` stays in the layouts with its `p-6`. |

---

## 8. Key source references

| What | Where |
|---|---|
| Route tree (and why `/admin` is a sibling) | `apps/web/src/App.tsx:21-28`, `:71-93` |
| Vehicle title rule the crumb must match | `apps/web/src/pages/VehicleDetailPage.tsx:128-129` |
| `useVehicle` / `vehicleKeys.detail` | `apps/web/src/lib/hooks/api/vehicles.ts:15-21`, `:35-43` |
| `useAdminFleet` / `adminKeys.fleet` | `apps/web/src/lib/hooks/api/admin.ts:11-25`, `:51-58` |
| Fleet name lives on `.attributes` | `apps/web/src/services/api/AdminService.ts:77-83`; `AdminFleetsPage.tsx:188-190` |
| `UserAttributes` (`email`, `displayName`, `avatarUrl`) | `apps/web/src/types/models/user.ts:10-17` |
| Why `BrandMark` is `aria-hidden` | `apps/web/src/components/BrandMark.tsx:3-14` |
| Why `ThemeToggle` and `ThemeToggleButton` are separate | `apps/web/src/components/ThemeToggle.tsx:6-19` |
| Why frame components stay out of `packages/ui-components` | `apps/web/src/components/PageHeader.tsx:9-12` |
| Existing token contract and its rationale comments | `apps/web/src/index.css:30-51`, `:79-83` |
| Existing convention tests (the style to follow) | `apps/web/src/test/conventions.test.ts` |
| Local stack, token minting, browser driving | `docs/runbooks/local-debugging.md` |
