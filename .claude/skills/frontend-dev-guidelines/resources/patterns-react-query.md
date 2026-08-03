# React Query & Hooks Patterns

## Overview

All server state lives in TanStack React Query hooks under `apps/web/src/lib/hooks/api/`, one module per resource, named for the resource rather than the hook: `vehicles.ts`, `mileage.ts`, `notifications.ts`, `dashboard.ts`, `admin.ts`, … (not `useVehicles.ts`).

Each module exports a key factory, its query hooks, its mutation hooks, and — where more than one call site needs to invalidate it — an invalidation-helper hook. Components never call a service directly; they call a hook (`FE-03`).

## Query key factory

Every module exports a hierarchical factory built with `as const` (`FE-12`). `vehicles.ts` is the canonical shape, and its comment names the test that pins it:

```typescript
// apps/web/src/lib/hooks/api/vehicles.ts:8-21
// Hierarchical query-key factory (frontend-dev-guidelines). The intermediate
// `lists()` / `details()` tiers enable scoped invalidation, while `list()` /
// `detail()` build on them. The shapes match the canonical test in
// vehicles.test.ts exactly:
//   all                     -> ['vehicles']
//   list({ fleetId: 'f1' }) -> ['vehicles', 'list', { fleetId: 'f1' }]
//   detail('v1')            -> ['vehicles', 'detail', 'v1']
export const vehicleKeys = {
  all: ['vehicles'] as const,
  lists: () => [...vehicleKeys.all, 'list'] as const,
  list: (params: { fleetId: string }) => [...vehicleKeys.lists(), params] as const,
  details: () => [...vehicleKeys.all, 'detail'] as const,
  detail: (id: string) => [...vehicleKeys.details(), id] as const,
};
```

**Rules:**

- `as const` on every tier. Without it the key widens to `string[]` and the tuple shape stops type-checking. `FE-12` greps for it.
- Build each tier by spreading the one above. That is what makes `invalidateQueries({ queryKey: vehicleKeys.lists() })` reach every list variant while leaving details alone.
- Only declare the tiers the resource actually has. `mileageKeys` stops at `list()` because mileage has no detail route (`mileage.ts:11-16`).
- The shapes are executable-tested, not just documented — `vehicles.test.ts:5-10` and `admin.test.ts:18` assert them. When you change a factory, that test is the thing that tells you what broke.

Parameterised lists take a single params object, not positional arguments (`list({ vehicleId, from, to })`, `mileage.ts:13-15`), so adding a filter does not reshape existing keys.

## Query hooks

```typescript
// vehicles.ts:25-43
export function useVehicles(fleetId: string | null | undefined) {
  return useQuery({
    queryKey: vehicleKeys.list({ fleetId: fleetId ?? '' }),
    queryFn: () => vehicleService.listByFleet(fleetId as string),
    enabled: !!fleetId,
    staleTime: 60 * 1000,
    gcTime: 5 * 60 * 1000,
  });
}

export function useVehicle(id: string | null | undefined) {
  return useQuery({
    queryKey: vehicleKeys.detail(id ?? ''),
    queryFn: () => vehicleService.get(id as string),
    enabled: !!id,
    staleTime: 60 * 1000,
    gcTime: 5 * 60 * 1000,
  });
}
```

**`enabled: !!x` is not optional for a nullable parameter.** The hook signature accepts `string | null | undefined` because the caller often does not have the id on first render (`useParams`, an unresolved active fleet). The key still has to be computed — hooks cannot be conditional — so it falls back to `''`. Without `enabled`, the query fires against `['vehicles', 'detail', '']` and caches a failure under a key a real id will never reach.

Infinite lists use `useInfiniteQuery` with the same key factory, plus `select` to flatten (`mileage.ts:54-78`). The reason mileage is infinite rather than paged is recorded at `mileage.ts:48-52`: it is merged with two other independently-paginated sources, and a merge over sources that *replace* their rows on page advance would drop the newest rows from view.

## Mutation hooks

```typescript
// vehicles.ts:47-68
export function useCreateVehicle(fleetId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (attributes: CreateVehicleAttributes) =>
      vehicleService.createInFleet(fleetId, attributes),
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: vehicleKeys.lists() });
    },
  });
}

export function useUpdateVehicle() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, attributes }: { id: string; attributes: UpdateVehicleAttributes }) =>
      vehicleService.patch(id, attributes),
    onSettled: (_data, _error, variables) => {
      void queryClient.invalidateQueries({ queryKey: vehicleKeys.lists() });
      void queryClient.invalidateQueries({ queryKey: vehicleKeys.detail(variables.id) });
    },
  });
}
```

**Invalidate in `onSettled`, not `onSuccess`.** A mutation that fails may still have changed server state — a partially applied write, a 500 after the commit, a timeout on a request that landed. `onSettled` fires on both outcomes, so the cache is reconciled against the server either way; `onSuccess` would leave a failed mutation's stale rows on screen. `admin.test.ts:65-73` pins this: *"invalidates even when the create fails."*

**`void` before `invalidateQueries` is deliberate.** `invalidateQueries` returns a promise; awaiting it inside `onSettled` would hold the mutation in its settling state until every refetch completed. `void` marks the promise as intentionally unawaited so the lint rule stays satisfied. The one place that *does* await is `fleets.ts:50`, where the next query must not run against the old access token.

**Invalidate the narrowest key that covers the change.** A create touches lists only; an update touches lists plus that one detail. Reach for `keys.all` only when the change is genuinely cross-cutting — `fleetSettings.ts:45-46` invalidates both `fleetKeys.all` and `memberKeys.all` because a fleet rename changes rows in both.

## Optimistic updates

Rare, and used only where a refetch would visibly undo the user's action. The one instance is `useUpdateTheme` (`auth.ts:109-127`), and its comment records a deliberate departure from the textbook pattern:

```typescript
// auth.ts:113-125
onMutate: (themePreference: ThemePreference) => {
  queryClient.setQueryData<MeResult>(authKeys.me(), (previous) =>
    previous ? { ...previous, user: { ...previous.user,
      attributes: { ...previous.user.attributes, themePreference } } } : previous,
  );
},
```

There is **no `onError` rollback**. Restoring the snapshot on failure would make the next `ThemeSync` pass re-adopt the old value and flip the theme out from under the user. The cache stays knowingly optimistic-but-wrong until a genuine refetch, and the toggle's toast tells the user the preference will not survive the session (`auth.ts:102-107`).

If you reach for `onMutate`, write down why a plain `onSettled` invalidation is not enough. Note the update is immutable — spread, never mutate the cached object (`FE-07`).

## Invalidation helpers

Modules whose cache is invalidated from outside their own mutations export a helper hook rather than making callers reconstruct keys:

```typescript
// vehicles.ts:94-101
export function useInvalidateVehicles() {
  const queryClient = useQueryClient();
  return {
    invalidateAll: () => queryClient.invalidateQueries({ queryKey: vehicleKeys.all }),
    invalidateLists: () => queryClient.invalidateQueries({ queryKey: vehicleKeys.lists() }),
    invalidateVehicle: (id: string) =>
      queryClient.invalidateQueries({ queryKey: vehicleKeys.detail(id) }),
  };
}
```

`dashboard.ts:127-132` follows the same shape. This is *in addition to* inline invalidation inside the module's own mutations, not a replacement for it — a mutation invalidates its own keys inline.

## Staleness is per hook

There is no standalone query-client module and no global staleness policy. <!-- ALLOW-VOCAB:G-10 --> The `QueryClient` is created inline by `AppProviders.tsx:32-42`, with exactly two defaults:

```typescript
new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      refetchOnWindowFocus: false,
    },
  },
})
```

Everything else is declared per hook. The values in use:

| `staleTime` | `gcTime` | Used for | Example |
| --- | --- | --- | --- |
| `30 * 1000` | `5 * 60 * 1000` | Unread counts, notification lists | `notifications.ts:63,77` |
| `60 * 1000` | `5 * 60 * 1000` | Ordinary resource lists and details | `vehicles.ts:30`, `mileage.ts:68`, `activity.ts:74`, `invites.ts:50` |
| `2 * 60 * 1000` | `5 * 60 * 1000` | Aggregated dashboard panels | `dashboard.ts:103,116` |
| `5 * 60 * 1000` | `10 * 60 * 1000` | Settings and configuration | `fleetSettings.ts:25`, `dashboard.ts:51` |
| `0` | — | Data the user navigated here specifically to see change | `invites.ts:64` |

Pick from this table rather than inventing a new interval. When a hook needs something outside it, say why in a comment — `invites.ts:64` does, and that is the model.

## Module structure

Order within a hook module, as in `vehicles.ts`:

1. Imports — the service singleton and the attribute types, both by relative path.
2. Key factory, with the comment stating the resolved key shapes.
3. `// --- Queries ---`
4. `// --- Mutations (invalidate <keys>) ---`
5. `// --- Invalidation helper ---`, if the module has one.

There is no prefetch tier: `prefetchQuery` has 0 hits in `apps/web/src`. Do not add prefetch helpers speculatively.
