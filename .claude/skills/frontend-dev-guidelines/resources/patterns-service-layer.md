# Service Layer Patterns

## Overview

The service layer (`services/api/`) provides typed abstractions over the API client. Services are **singletons** — instantiated once and exported as module-level constants (`export const vehicleService = new VehicleService()`).

Two shapes exist side by side in `apps/web/src/services/api/`. Of the 16 concrete services, 10 extend `BaseService` and 6 are plain classes that talk to `apiClient` directly. Both are legitimate; which one fits depends on whether the resource has a uniform, single-type CRUD shape.

## Types

Response typing comes from `@myfleet/shared-ts`: `JsonApiDocument`, `JsonApiResource`, `PageMeta`. Every service reaches the network through the shared `apiClient` singleton (`lib/api/client.ts`), directly or via `BaseService`.

```typescript
export interface ListResult<A> {
  data: Array<JsonApiResource<A>>;
  meta?: PageMeta;
}
```

This is the return type of every list call and how pagination `meta` reaches the calling hook (`BaseService.ts:4-7`).

### Gateway paths

`apiClient.baseUrl` is `''`, so every path a service passes must be an absolute gateway path. From `BaseService.ts:9-20`, copied verbatim because it is the kind of thing that is wrong silently:

> `apiClient.baseUrl` is '', so paths passed here must be absolute gateway
> paths. Traefik strips only `/api` before routing to auth-service and
> media-service (their routes already carry their own /auth or /media
> segment); fleet-service and notification-service still have their full
> `/api/<service>` prefix stripped.

## Pattern 1: BaseService (single-type CRUD resources)

Extend `BaseService` when the resource has one JSON:API `type` and the standard verbs (or a subset of them) apply. The base class (`apps/web/src/services/api/BaseService.ts`, 69 lines, reproduced in full):

```typescript
export interface ListResult<A> {
  data: Array<JsonApiResource<A>>;
  meta?: PageMeta;
}

export abstract class BaseService<A, CreateA = A, UpdateA = Partial<A>> {
  protected abstract readonly resourceType: string;
  protected abstract readonly basePath: string;

  /** GET a collection (optionally at a custom path, e.g. a nested route). */
  protected async listAt(path: string): Promise<ListResult<A>> {
    const doc = await apiClient.request<JsonApiDocument<Array<JsonApiResource<A>>>>(path);
    return { data: doc.data, meta: doc.meta };
  }

  list(): Promise<ListResult<A>> {
    return this.listAt(this.basePath);
  }

  async get(id: string): Promise<JsonApiResource<A>> {
    const doc = await apiClient.request<JsonApiDocument<JsonApiResource<A>>>(
      `${this.basePath}/${id}`,
    );
    return doc.data;
  }

  /** POST a create, optionally at a custom path (nested create routes). */
  protected async createAt(path: string, attributes: CreateA): Promise<JsonApiResource<A>> {
    const doc = await apiClient.request<JsonApiDocument<JsonApiResource<A>>>(path, {
      method: 'POST',
      body: JSON.stringify({ data: { type: this.resourceType, attributes } }),
    });
    return doc.data;
  }

  create(attributes: CreateA): Promise<JsonApiResource<A>> {
    return this.createAt(this.basePath, attributes);
  }

  async patch(id: string, attributes: UpdateA): Promise<JsonApiResource<A>> {
    const doc = await apiClient.request<JsonApiDocument<JsonApiResource<A>>>(
      `${this.basePath}/${id}`,
      {
        method: 'PATCH',
        body: JSON.stringify({ data: { type: this.resourceType, id, attributes } }),
      },
    );
    return doc.data;
  }

  async remove(id: string): Promise<void> {
    await apiClient.request<null>(`${this.basePath}/${id}`, { method: 'DELETE' });
  }
}
```

Two abstract members: `resourceType` (the JSON:API `type` string, used to build write envelopes) and `basePath` (the gateway path for `list`/`get`/`create`/`patch`/`remove`). `listAt` and `createAt` are `protected` escape hatches for nested routes — a subclass calls them with a different path while `basePath` stays the "canonical" one.

### Worked subclass: `VehicleService`

```typescript
class VehicleService extends BaseService<
  VehicleAttributes,
  CreateVehicleAttributes,
  UpdateVehicleAttributes
> {
  protected readonly resourceType = 'vehicles';
  protected readonly basePath = '/api/fleet/vehicles';

  // List and create are nested under the fleet, so override to the fleet path.
  listByFleet(fleetId: string): Promise<ListResult<VehicleAttributes>> {
    return this.listAt(`/api/fleet/fleets/${fleetId}/vehicles`);
  }

  createInFleet(fleetId: string, attributes: CreateVehicleAttributes): Promise<Vehicle> {
    return this.createAt(`/api/fleet/fleets/${fleetId}/vehicles`, attributes);
  }
}

export const vehicleService = new VehicleService();
```

`get`, `patch`, and `remove` are used unmodified (`/api/fleet/vehicles/{id}` is a real, non-nested route); `list`/`create` are overridden via `listAt`/`createAt` because the fleet-scoped routes exist alongside the flat ones.

Extending `BaseService` doesn't require every inherited verb to see real use, though. `DashboardService` and `MemberService` both extend it, declare `resourceType` and `basePath` exactly as required — and then leave `basePath` unused:

```typescript
// DashboardService.ts:28-30
class DashboardService extends BaseService<DashboardAttributes> {
  protected readonly resourceType = 'dashboards';
  protected readonly basePath = '/api/fleet/dashboards'; // not used directly — all routes are nested
```

```typescript
// MemberService.ts:17-19
class MemberService extends BaseService<MembershipAttributes> {
  protected readonly resourceType = 'memberships';
  protected readonly basePath = '/api/fleet/memberships'; // not used directly
```

Every real route on both services is fleet-nested and hand-written with `apiClient.request` directly (or, for `MemberService.listByFleet`, via `listAt` with an explicit nested path). What extending buys them here is small — a `resourceType` constant referenced from their own hand-written methods (e.g. `MemberService.updateRole` uses `this.resourceType`) — not the inherited `list`/`get`/`create`/`patch`/`remove` methods themselves. Don't read "extends `BaseService`" as a promise that the inherited CRUD verbs are actually wired to real routes; check the class body.

### Template methods

| Method | Visibility | Purpose |
|--------|-----------|---------|
| `listAt(path)` | protected | GET a collection at an arbitrary path |
| `list()` | public | GET the collection at `basePath` (calls `listAt`) |
| `get(id)` | public | GET a single resource by id |
| `createAt(path, attributes)` | protected | POST a create at an arbitrary path |
| `create(attributes)` | public | POST a create at `basePath` (calls `createAt`) |
| `patch(id, attributes)` | public | PATCH a partial update |
| `remove(id)` | public | DELETE by id |

That's the whole surface. There is no `validate`, `transformResponse`, `getAll`, `getById`, `exists`, `update`, `createBatch`, `updateBatch`, or `deleteBatch` — validation happens with Zod at the form layer, and attribute coercion happens in the type layer, not in a service hook.

## Pattern 2: direct `apiClient` usage (no single-type CRUD shape)

The other 6 services (`ActivityService`, `AdminService`, `MediaService`, `MileageService`, `NotificationService`, `VehicleMediaService`) are plain classes that call `apiClient.request` directly, with no common base class. There is no single predicate that explains all six — "nested routes" doesn't work, since `DashboardService`/`MemberService` above are nested and still extend `BaseService`. Each one differs for its own reason:

| Service | Why it doesn't extend `BaseService` |
|---|---|
| `ActivityService` | No CRUD at all — only two paginated read-only feeds scoped to different parents (`/fleets/{id}/activity`, `/vehicles/{id}/activity`). Nothing is ever created, patched, or deleted, so there's no write envelope and no single `resourceType`. No `basePath` field either; both full paths are spelled out inline. |
| `AdminService` | Fans out across five distinct resource concepts under one `/api/fleet/admin` root — stats, fleets, users, purge operations, audit events — each with its own attribute shape; only the purge-operation write calls (`createPurge`, `retryPurge`) build an envelope, and both use the literal `type: 'purge-operations'`, so there is no one `resourceType` that covers the class. It also needs its own `AdminListMeta` (pagination plus `warnings?: string[]`), a shape `ListResult`'s plain `PageMeta` doesn't carry. Still declares `private readonly basePath = '/api/fleet/admin'` as a namespace root, even though it isn't itself a resource path. |
| `MediaService` | Several operations aren't JSON:API CRUD at all: `putContent` PUTs a raw `File` body with a `Content-Type` override, and `getContentBlob` returns a `Blob`, not a `JsonApiResource`. There is no `list()` — media objects are never listed as a collection. `resourceType` would only describe `initUpload`/`confirm`, a minority of the class. Still declares `private readonly basePath = '/api/media'`. |
| `MileageService` | Append-only and nested-only: only `listByVehicle` and `create` exist, both keyed by a `vehicleId` baked into the path on every call. There's no single-record `get`, `patch`, or `remove`, so most of the base class's contract would be dead weight. Still declares `private readonly basePath = '/api/fleet/vehicles'` (the *parent* resource's path, not a mileage-specific one). |
| `NotificationService` | Two distinct resource concepts in one service — notifications and notification preferences — with separate path constants (`notificationsPath`, `preferencesPath`), so there's no single `basePath`. Only the preferences write call builds an envelope, with literal `type: 'notification-preferences'`; `markRead`/`markAllRead` send a bare POST with no JSON:API envelope at all and return `void`, which doesn't fit any base verb's signature. |
| `VehicleMediaService` | An association resource addressed by a compound key (`vehicleId` + `mediaId`) — there's no single `id` for `get`/`patch`/`remove` to key off. It also colocates one unrelated action, `setPrimaryImage`, which mutates the *vehicle* resource (`type: 'vehicles'`), not its own. |

Representative example — `MileageService` (nested, nothing to inherit beyond a GET and a POST):

```typescript
class MileageService {
  private readonly basePath = '/api/fleet/vehicles';

  async listByVehicle(
    vehicleId: string,
    params?: { from?: string; to?: string; page?: number; pageSize?: number },
  ): Promise<{ data: Array<JsonApiResource<MileageRecordAttributes>>; meta?: PageMeta }> {
    const url = new URL(`${this.basePath}/${vehicleId}/mileage`, 'http://localhost');
    if (params?.from) url.searchParams.set('from', params.from);
    if (params?.to) url.searchParams.set('to', params.to);
    if (params?.page != null) url.searchParams.set('page[number]', String(params.page));
    if (params?.pageSize != null) url.searchParams.set('page[size]', String(params.pageSize));

    const doc = await apiClient.request<
      JsonApiDocument<Array<JsonApiResource<MileageRecordAttributes>>>
    >(`${this.basePath}/${vehicleId}/mileage${url.search}`);
    return { data: doc.data, meta: doc.meta };
  }

  async create(
    vehicleId: string,
    attrs: CreateMileageAttributes,
  ): Promise<JsonApiResource<MileageRecordAttributes>> {
    const doc = await apiClient.request<JsonApiDocument<JsonApiResource<MileageRecordAttributes>>>(
      `${this.basePath}/${vehicleId}/mileage`,
      {
        method: 'POST',
        body: JSON.stringify({ data: { type: 'mileageRecords', attributes: attrs } }),
      },
    );
    return doc.data;
  }
}

export const mileageService = new MileageService();
```

## JSON:API Request Format

All write operations use the JSON:API envelope:

```typescript
{
  data: {
    type: "resourceType",
    id: "identifier",        // present for patch, absent for create
    attributes: { /* data */ }
  }
}
```

`BaseService.createAt` (`BaseService.ts:46`) omits `id`; `BaseService.patch` (`BaseService.ts:60`) includes it.

### Action endpoints (no real attributes)

This applies even to "action" endpoints whose body has no meaningful attributes — restore, confirm, retry, and similar. If the backend route is wired through `server.RegisterInputHandler[T]` (`packages/shared-go/server/handler.go:47-60`), it decodes the body as a JSON:API envelope *before* the handler runs. A body that fails to decode (including a bare `{}` when a differently-shaped envelope was expected, or malformed JSON) returns **422 with `server.ErrValidation`** (`handler.go:54-56`), and the action never executes. This used to be documented as a `400 "Could not parse request body"`; it isn't — `RegisterInputHandler` always answers decode failures with `ErrValidation`, which serializes as 422.

For these endpoints, send the full envelope with empty (or minimal) attributes, using the service's own `resourceType` — not a backend `GetName()` call, which doesn't exist in this codebase:

```typescript
// VehicleService.ts:40-49
async restore(id: string): Promise<Vehicle> {
  const doc = await apiClient.request<JsonApiDocument<JsonApiResource<VehicleAttributes>>>(
    `${this.basePath}/${id}/restore`,
    {
      method: 'POST',
      body: JSON.stringify({ data: { type: this.resourceType, id, attributes: {} } }),
    },
  );
  return doc.data;
}
```

When in doubt, default to sending the full envelope: `RegisterInputHandler`-based routes require it, and a plain chi handler that ignores the request body is harmless to send it to anyway.

## Update Pattern (Immutable)

Services never mutate an existing object in place; a patch call returns a fresh `JsonApiResource<A>` from the server response. `BaseService.patch` (`BaseService.ts:55-64`):

```typescript
async patch(id: string, attributes: UpdateA): Promise<JsonApiResource<A>> {
  const doc = await apiClient.request<JsonApiDocument<JsonApiResource<A>>>(
    `${this.basePath}/${id}`,
    {
      method: 'PATCH',
      body: JSON.stringify({ data: { type: this.resourceType, id, attributes } }),
    },
  );
  return doc.data;
}
```

Callers pass only the changed `attributes` (typed as `UpdateA`, default `Partial<A>`) — not a merged copy of the old object — and use the server's returned resource as the new source of truth. There is no client-side merge-and-return-a-guessed-object step.

## Imports

There is no barrel file (`index.ts`) under `services/api/` — import the concrete service module directly by its relative path:

```typescript
// apps/web/src/lib/hooks/api/vehicles.ts:2
import { vehicleService } from '../../../services/api/VehicleService';
```

There is no `@/*` alias in this project; intra-`src` imports are relative, and shared code is imported by package name (`@myfleet/shared-ts`, `@myfleet/ui-components`).
