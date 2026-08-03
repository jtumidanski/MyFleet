# Routing & Pages Patterns

## Overview

React Router v6. The route tree lives in `apps/web/src/App.tsx` and is **exported separately from the router that hosts it**:

```tsx
// App.tsx:29,98-104
export function AppRoutes() { … }

export function App() {
  return (
    <BrowserRouter>
      <AppRoutes />
    </BrowserRouter>
  );
}
```

That split exists so tests can mount the real tree inside a `MemoryRouter`. The comment at `App.tsx:21-28` says why it matters: `postPurgeRouting.test.tsx` guards the actual nesting, and a test that rebuilt the tree by hand would keep passing through exactly the regression that matters.

`main.tsx` mounts `<AppProviders><App /></AppProviders>` — providers wrap the router, not the other way round.

## Pages

`apps/web/src/pages/`:

| Page | Route |
| --- | --- |
| `LoginPage` | `/login` — public |
| `InviteAcceptPage` | `/invites/:token/accept` — authenticated, fleetless allowed |
| `OnboardingPage` | `/onboarding` — authenticated, fleetless allowed |
| `DashboardPage` | `/` (index of the app shell) |
| `VehiclesPage` | `/vehicles` |
| `VehicleDetailPage` | `/vehicles/:id` |
| `ActivityPage` | `/activity` |
| `NotificationsPage` | `/notifications` |
| `SettingsPage` | `/settings` |

`apps/web/src/pages/admin/`: `AdminOverviewPage` (`/admin`), `AdminFleetsPage` (`/admin/fleets`, `/admin/fleets/:id`), `AdminUsersPage`, `AdminPurgesPage`, `AdminAuditPage`.

## Route configuration

Four tiers, and the boundaries between them are load-bearing:

```tsx
// App.tsx:31-94 (abridged)
<Routes>
  {/* Public */}
  <Route path="/login" element={<LoginPage />} />

  {/* Authenticated, allowed without a fleet */}
  <Route path="/invites/:token/accept" element={<RequireAuth><InviteAcceptPage /></RequireAuth>} />
  <Route path="/onboarding" element={<RequireAuth><OnboardingPage /></RequireAuth>} />

  {/* Authenticated app shell — layout route, no path of its own */}
  <Route element={<RequireAuth><AppLayout /></RequireAuth>}>
    <Route index element={<DashboardPage />} />
    <Route path="/vehicles" element={<VehiclesPage />} />
    <Route path="/vehicles/:id" element={<VehicleDetailPage />} />
    <Route path="/activity" element={<ActivityPage />} />
    <Route path="/notifications" element={<NotificationsPage />} />
    <Route path="/settings" element={<SettingsPage />} />
  </Route>

  {/* Admin console — a SIBLING of the shell, not a child */}
  <Route path="/admin" element={<RequirePlatformAdmin><AdminLayout /></RequirePlatformAdmin>}>
    <Route index element={<AdminOverviewPage />} />
    <Route path="fleets" element={<AdminFleetsPage />} />
    …
  </Route>
</Routes>
```

**The admin console is a sibling, not a child.** `RequireAuth` redirects fleetless users to `/onboarding`, and a platform admin with no fleet — including one who has just run a system purge — must still reach every admin screen. Nesting `/admin` under `RequireAuth` reintroduces that redirect. The reasoning is at `App.tsx:71-78`; do not "tidy" the two guarded subtrees into one.

**Paths are absolute in the shell, relative under `/admin`.** The shell's layout route has no `path`, so its children carry `/vehicles`; the admin layout route has `path="/admin"`, so its children carry bare `fleets`.

## Route guards

`RequireAuth` (`components/RequireAuth.tsx`) is the only auth guard, and it makes three decisions in order:

```tsx
// RequireAuth.tsx:31-54 (abridged)
if (isLoading) return <Skeleton … />;                        // never redirect mid-resolution

if (!isAuthenticated) {
  // Carry the attempted path so LoginPage can ask auth-service to return here.
  return <Navigate to="/login" replace state={{ from: location.pathname + location.search }} />;
}

if (!activeFleetId && !allowsFleetlessAccess(location.pathname)) {
  return <Navigate to="/onboarding" replace />;
}

return <>{children}</>;
```

Four details that are easy to get wrong and are each pinned by a comment:

- **Render a loading state while `isLoading`.** Redirecting before auth resolves bounces every authenticated user to `/login` on a hard refresh.
- **Carry `from` in navigation state** (`:40-43`) — otherwise an invite link clicked while logged out is lost in the OAuth round-trip.
- **`!activeFleetId`, not `=== null`** (`:46-49`) — a guard that recognised only `null` disagreed with the pages the moment the wire carried `""`, letting the user past the guard and onto pages that all said "No fleet selected".
- **The fleetless allowlist is a route table**, `FLEETLESS_ROUTES` (`:13`), with trailing slashes tolerated because React Router matches with or without one.

`RequirePlatformAdmin` (`components/admin/RequirePlatformAdmin.tsx`) guards the console. Both guards are navigation convenience only — server-side authz remains authoritative (`RequireAuth.tsx:25`).

## List page pattern

A page owns the hooks, the dialog state, and the permission decisions; it renders feature components that take data as props. It does **not** call a service directly (`FE-03`).

```tsx
// pages/VehiclesPage.tsx:34-53 (abridged)
export function VehiclesPage() {
  const { activeFleetId, role } = useAuth();
  const { data, isLoading } = useVehicles(activeFleetId);
  const createVehicle = useCreateVehicle(activeFleetId ?? '');
  const [open, setOpen] = useState(false);

  // Viewers are read-only; only members/owners can add vehicles.
  const canWrite = role === 'owner' || role === 'member';
  …
  return (
    <div className="space-y-6">
      <PageHeader title="Vehicles" actions={canWrite && <Button …>Add Vehicle</Button>} />
      …
      <VehicleList vehicles={data?.data ?? []} isLoading={isLoading} emptyAction={…} />
    </div>
  );
}
```

- **No `useState` + `useEffect` + `service.getAll()`.** The hook owns the fetch, the cache and the loading flag.
- **`useVehicles(activeFleetId)` takes the nullable value directly** — the hook's `enabled: !!fleetId` handles the not-yet-known case (see `patterns-react-query.md`).
- **The page container is `space-y-6`** and owns the vertical rhythm; `PageHeader` renders only the header row (`PageHeader.tsx:5-8`).
- **Permission gating happens here**, and is passed down as a node, not a role.
- **Form-input → API-attributes mapping lives at the page boundary** (`VehiclesPage.tsx:20-32`).

## Detail page pattern

Same shape, plus `useParams` and three explicit render branches:

```tsx
// pages/VehicleDetailPage.tsx:47-51,103-119 (abridged)
export function VehicleDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { role } = useAuth();
  const { data: vehicle, isLoading } = useVehicle(id);
  …
  if (isLoading) {
    return (
      <div className={PAGE_CONTAINER}>
        <PageHeader title="Vehicle" />
        <Skeleton className="h-40 w-full" />
      </div>
    );
  }

  if (!vehicle) {
    return (
      <div className={PAGE_CONTAINER}>
        <PageHeader title="Vehicle" />
        <p className="text-muted-foreground">Vehicle not found.</p>
      </div>
    );
  }
  …
}
```

`useParams` returns `string | undefined`, which is exactly what `useVehicle` accepts — no non-null assertion is needed anywhere.

All three branches share one container constant (`PAGE_CONTAINER`, `:45`) and all three render `PageHeader`. That is deliberate: the title's box must not move when data lands (`VehicleDetailPage.tsx:39-44`). A loading branch with a different container width is a visible jump on every navigation.

Where a page opens several dialogs, model it as one `OpenDialog` union rather than a boolean per dialog (`:36`) — only one can be open at a time, and a union makes that unrepresentable rather than merely unlikely.

## App shell

`AppLayout` (`components/AppLayout.tsx`) is the shell rendered by the layout route. It holds two things and delegates the rest:

```tsx
// AppLayout.tsx:16-26
const NAV: readonly FrameNavItem[] = [
  { to: '/', label: 'Dashboard', icon: LayoutDashboard, end: true },
  { to: '/vehicles', label: 'Vehicles', icon: Car },
  { to: '/activity', label: 'Activity', icon: Activity },
  { to: '/notifications', label: 'Notifications', icon: Bell },
  { to: '/settings', label: 'Settings', icon: Settings },
];

// The entry point is a convenience, not a control: its absence hides the
// door, and the server refuses entry regardless (FR-ADMIN-UI-5).
const ADMIN_ENTRY: FrameNavItem = { to: '/admin', label: 'Admin', icon: Shield, end: true };
```

The nav table and the brand target are all that differ between the fleet shell and the admin console; the header row, profile menu and breadcrumb are shared components under `components/frame/` (`AppLayout.tsx:28-38`). `<Outlet />` renders the matched child route inside `SidebarInset` (`:55-60`).

Adding a route means three edits: a `<Route>` in `App.tsx`, a `NAV` entry in `AppLayout.tsx` if it is top-level, and a breadcrumb trail.

## Breadcrumbs

The breadcrumb system is `components/frame/` — `breadcrumbTrails.ts` (the data) and `AppBreadcrumb.tsx` (the renderer), with per-kind crumb components under `frame/crumbs/`.

Every authenticated route's trail is spelled out as an explicit row keyed by route pattern (`breadcrumbTrails.ts:30-38`). The comment records why explicit beats walking pathname prefixes: with a prefix walk, `/admin/fleets/:id` only produces the right trail if every ancestor happens to be a real route, and the rule that the console is rooted at *Admin* rather than *Home* becomes a special case instead of a row. **Adding a route means adding a row, and the whole requirement is testable as data** (`breadcrumbTrails.test.ts`).

Crumbs are one of three kinds (`:11-12`): `static` carries its own label and target; `vehicle` and `fleet` stand for a `:id` segment and carry no label, because their component resolves the object's name from a query.

## Navigation

- **`<Link to=…>`** for in-app navigation — `VehicleCard.tsx:126`, `AdminFleetsPage.tsx:141,194`.
- **`useNavigate()`** when navigation is a consequence rather than a click: after accepting an invite (`InviteAcceptPage.tsx:18`), after onboarding (`OnboardingPage.tsx:32`), after deleting the vehicle you are looking at (`DeleteVehicleDialog.tsx:27`).
- **A plain `<a>`** when the destination leaves the SPA. `VehicleCard.tsx:156` marks this explicitly — a `<Link>` to an external URL is a routing error React Router cannot report.
- **`<Navigate replace>`** inside a guard, so the rejected URL does not end up in history (`RequireAuth.tsx:43,51`).
