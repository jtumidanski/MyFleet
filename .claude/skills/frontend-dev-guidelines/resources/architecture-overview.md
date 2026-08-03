# Architecture Overview

## Tech Stack

| Technology | Purpose |
|-----------|---------|
| React | UI framework |
| Vite | Build tool, dev server |
| TypeScript | Type safety (strict mode) |
| React Router | Client-side routing |
| Tailwind CSS | Utility-first styling |
| shadcn/ui | Radix-based component library |
| TanStack React Query | Server state management |
| react-hook-form | Form state management |
| Zod | Schema validation |
| sonner | Toast notifications |
| Lucide React | Icon library |

## Project Structure

```
apps/web/src/
├── App.tsx                 # AppRoutes wrapped in BrowserRouter
├── main.tsx                 # Entry point: mounts AppProviders > App
├── pages/                   # Route pages, incl. pages/admin/
├── components/
│   ├── ui/                  # shadcn/ui base primitives
│   ├── features/            # Feature-specific containers (activity/, dashboard/,
│   │                         #   notifications/, settings/, vehicles/)
│   ├── frame/                # App shell chrome: FrameHeader, FrameNav,
│   │                         #   AppBreadcrumb, ProfileMenu, breadcrumbTrails.ts
│   ├── admin/                # Platform-admin screens (AdminLayout, BlastRadiusPanel,
│   │                         #   PurgeConfirmDialog, RequirePlatformAdmin)
│   ├── providers/            # AppProviders.tsx (root provider stack)
│   └── AppLayout.tsx, PageHeader.tsx, RequireAuth.tsx, ThemeSync.tsx,
│       ThemeToggle.tsx, BrandMark.tsx, GoogleMark.tsx  # top-level shared components
├── lib/
│   ├── api/                 # API client + error utilities (client.ts, refresh.ts, token.ts)
│   ├── hooks/api/            # React Query hooks, one file per resource (vehicles.ts, ...)
│   ├── schemas/              # Zod validation schemas
│   ├── admin/, auth/, config/, invites/, utils/  # domain-scoped helpers
│   └── carfax.ts, theme.ts, utils.ts, vehicleRecords.ts, vehicleStats.ts  # top-level helpers
├── services/api/             # Service classes (BaseService + one concrete class per resource)
├── types/models/             # Domain model interfaces (vehicle.ts, fleet.ts, invite.ts, ...)
├── context/                  # AuthContext.tsx, ThemeContext.tsx
└── test/                     # Test setup and shared test helpers (setup.ts, renderWithProviders.tsx)
```

`components/` has no `common/` subdirectory (real split: `admin/`,
`features/`, `frame/`, `providers/`, `ui/`, plus the top-level files above).
`lib/` has no `breadcrumbs/` subdirectory. `types/` contains only `models/`
— API response/error types come from the shared `@myfleet/shared-ts`
package, not a local subdirectory here.

### Imports

There is no `@/*` path alias. Neither `tsconfig.app.json` nor
`vite.config.ts` configures one — all intra-`src` imports are relative:

```ts
// apps/web/src/lib/hooks/api/vehicles.ts:2
import { vehicleService } from '../../../services/api/VehicleService';
// apps/web/src/services/api/BaseService.ts:2
import { apiClient } from '../../lib/api/client';
```

Shared code is imported by package name, not by path: `@myfleet/shared-ts`
and `@myfleet/ui-components` (`apps/web/package.json:13-14`).

Adding a `@/*` alias would remove the need to count `../` segments, but it is
application config (a `tsconfig.app.json` `paths` entry plus a matching Vite
`resolve.alias`) plus rewriting every existing relative import — out of
scope here (PRD §2). Until that lands, write relative imports as shown above.

## Architectural Layers

```
┌─────────────────────────────────────────────┐
│  Route Pages (pages/*.tsx)                    │  ← Data fetching, composition
├─────────────────────────────────────────────┤
│  Feature Components (components/features/)   │  ← Business UI logic
├─────────────────────────────────────────────┤
│  React Query Hooks (lib/hooks/api/)          │  ← Server state, cache
├─────────────────────────────────────────────┤
│  Service Layer (services/api/)               │  ← API abstraction
├─────────────────────────────────────────────┤
│  API Client (lib/api/client.ts)              │  ← HTTP, retry, dedup
├─────────────────────────────────────────────┤
│  Backend Service                             │  ← Go service via ingress
└─────────────────────────────────────────────┘
```

Data flows top-down: pages compose feature components, which use hooks,
which call services, which use the API client. Skipping a layer — e.g. a
component calling `apiClient` directly — breaks this chain (`FE-03`).

## Provider Hierarchy (Root Layout)

`main.tsx` mounts `AppProviders` around `App`; `App` itself renders
`BrowserRouter` around the route tree:

```tsx
// apps/web/src/main.tsx:26-28
<AppProviders>
  <App />
</AppProviders>

// apps/web/src/App.tsx:98-104 — App() itself
<BrowserRouter>
  <AppRoutes />
</BrowserRouter>
```

So `AppProviders` is outermost, wrapping `BrowserRouter`, not the other way
around. Inside `AppProviders` (`apps/web/src/components/providers/AppProviders.tsx:44-54`):

```tsx
<QueryClientProvider client={queryClient}>
  <ThemeProvider>
    <AuthProvider>
      <ThemeSync />
      {children}
    </AuthProvider>
    <ThemedToaster />
  </ThemeProvider>
</QueryClientProvider>
```

`ThemedToaster` (a local wrapper around sonner's `Toaster`) is a sibling of
`AuthProvider` inside `ThemeProvider`, not a separate top-level provider.
`AppProviders.tsx:27-29` explains why the nesting is this shape:

> ThemeProvider sits ABOVE the toaster (FR-3P-2) and above the app shell, but
> the authoritative preference arrives from useMe(), which lives BELOW it —
> ThemeSync bridges the two without either context importing the other.

The `QueryClient` itself is created once per app instance via a `useState`
initializer (`AppProviders.tsx:32-42`), not a module-level `query-client.ts`
— see React Query defaults below.

## Key Configuration

### TypeScript (`tsconfig.app.json`)
- `strict: true` (`:11`) — all strict checks enabled
- `noUncheckedIndexedAccess: true` (`:12`) — safe index access
- No `paths` entry — see Imports, above

### React Query

`AppProviders.tsx:32-42` creates the `QueryClient` with:
- `retry: 1`
- `refetchOnWindowFocus: false`
- No default `staleTime` or `gcTime` — each query sets its own

Staleness is a per-hook decision, not a global default. For example,
`apps/web/src/lib/hooks/api/vehicles.ts:30-31` sets `staleTime: 60 * 1000`
and `gcTime: 5 * 60 * 1000` on `useVehicles`. Follow the same pattern: pick
values for the query you are adding rather than relying on a shared default.

### Vite (`vite.config.ts`)
- React plugin enabled (`:10`)
- Dev server proxy: `/api` forwards to the Traefik gateway, which strips
  `/api/<service>` and routes to the matching backend service (`:5-8`,
  `:12-17`); the API client's `baseUrl` stays `''` so the same paths work in
  production behind the gateway
- No `resolve.alias` — see Imports, above
