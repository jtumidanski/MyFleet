# Type System Patterns

## Overview

TypeScript strict mode is enabled with enhanced checks. All domain types live in `types/models/`. There is no separate `types/api/` directory — the JSON:API envelope and error types come from the shared `@myfleet/shared-ts` package, not from a local folder.

## JSON:API Model Structure

**All domain models are a `JsonApiResource<A>` from `@myfleet/shared-ts`, not a hand-written interface:**

```typescript
// types/models/vehicle.ts
import type { JsonApiResource } from '@myfleet/shared-ts';

export interface VehicleAttributes {
  fleetId: string;
  nickname?: string;
  make: string;
  model: string;
  trim?: string;
  year: number;
  vin?: string;
  currentMileage?: number;
  primaryImageMediaId?: string;
  notes?: string;
  status?: string;
  lastActivityAt?: string;
  nextDue?: VehicleNextDue;
}

export type Vehicle = JsonApiResource<VehicleAttributes>;
```

`JsonApiResource` itself (`packages/shared-ts/src/jsonapi.ts:1-6`, whole file reproduced):

```typescript
export interface JsonApiResource<A, R = Record<string, unknown>> {
  type: string;
  id: string;
  attributes: A;
  relationships?: R;
}
```

**Pattern:**
- `id` is always `string`, and `type` carries the JSON:API resource type — the model interface itself never declares `id`/`type` by hand, they come from `JsonApiResource`
- `attributes` contains all data fields
- Nested types for complex attributes
- Optional `relationships` for related resources, typed via the generic `R` parameter

## Doc-Comment Convention on Model Files

Attribute files name the backend struct they mirror and mark which fields the server derives (so a client can never legally write them). From `apps/web/src/types/models/vehicle.ts:14-15`:

```typescript
// Mirrors fleet-service vehicle resource (apps/fleet-service/internal/vehicle/rest.go).
// `status`, `lastActivityAt`, and `nextDue` are derived read-only on the server and never written by the client.
export interface VehicleAttributes {
```

This comment is the anti-drift mechanism for the type layer: when the backend struct changes, the comment is the pointer back to the file that has to move first. Every model file should carry one.

## Attribute Enums: String Unions, Not Numeric Enums

This codebase has no numeric `enum` + label-map convention (`grep -rn "^export enum" apps/web/src/types/models/` returns nothing, and there is no `Labels: Record<...>` anywhere in `apps/web/src`). Discriminant-like attributes are plain string unions, narrowed inline where they're declared. From `apps/web/src/types/models/vehicle.ts:7-12`:

```typescript
export interface VehicleNextDue {
  state: 'upcoming' | 'overdue';
  axis: 'time' | 'mileage';
  miles?: number; // present iff axis === 'mileage'
  days?: number; // present iff axis === 'time'
}
```

Nested rather than four flat fields: `axis` determines which magnitude is present, so flattening would make illegal combinations (`axis: 'time'` with a `miles` value) representable.

## Helper Functions on Models

Attach domain logic as standalone functions in `lib/`, alongside the model rather than on it — not in `types/models/` itself. Each has a sibling `.test.ts` (except `utils.ts`):

- `apps/web/src/lib/vehicleStats.ts`, `vehicleStats.test.ts`
- `apps/web/src/lib/vehicleRecords.ts`, `vehicleRecords.test.ts`
- `apps/web/src/lib/carfax.ts`, `carfax.test.ts`
- `apps/web/src/lib/theme.ts`, `theme.test.ts`
- `apps/web/src/lib/utils.ts`

## JSON:API Envelope and Pagination

`JsonApiDocument<T>` and `PageMeta` (`packages/shared-ts/src/jsonapi.ts:8-19`):

```typescript
export interface PageMeta {
  total: number;
  totalPages: number;
  number: number;
  size: number;
}

export interface JsonApiDocument<T> {
  data: T;
  meta?: PageMeta;
  links?: Record<string, string>;
}
```

Service layer calls unwrap `JsonApiDocument` into `ListResult<A>` (`apps/web/src/services/api/BaseService.ts:4-7`, documented in full in `patterns-service-layer.md`):

```typescript
export interface ListResult<A> {
  data: Array<JsonApiResource<A>>;
  meta?: PageMeta;
}
```

`meta` is how pagination reaches the calling hook; it stays optional because `JsonApiDocument.meta` itself is optional (`jsonapi.ts:17`).

## Error Type

`ApiError` is a **class** extending `Error`, not an interface, and there is no `NetworkError`/`ValidationError`/`AuthenticationError`/`NotFoundError`/`ServerError` hierarchy or `isNetworkError`/`isNotFoundError` guards. The backend emits `{ errors: [APIError] }` (`packages/shared-go/server/jsonapi.go:113-115`), and the frontend converts that into an `ApiError` with a single constructor function. From `packages/shared-ts/src/errors.ts:3-16,23-37`, whole file reproduced:

```typescript
export class ApiError extends Error {
  status: number;
  code: string;
  detail?: string;
  pointer?: string;
  constructor(status: number, code: string, message: string, detail?: string, pointer?: string) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
    this.detail = detail;
    this.pointer = pointer;
  }
}

interface EnvelopeShape {
  status?: number;
  body?: { errors?: JsonApiError[] };
}

export function createErrorFromUnknown(e: unknown): ApiError {
  const env = e as EnvelopeShape;
  const first = env?.body?.errors?.[0];
  if (first) {
    return new ApiError(
      env.status ?? Number(first.status) ?? 0,
      first.code,
      first.title,
      first.detail,
      first.source?.pointer,
    );
  }
  if (e instanceof Error) return new ApiError(0, 'unknown', e.message);
  return new ApiError(0, 'unknown', 'Unknown error');
}
```

`JsonApiError` (`packages/shared-ts/src/jsonapi.ts:21-27`):

```typescript
export interface JsonApiError {
  status: string;
  code: string;
  title: string;
  detail?: string;
  source?: { pointer?: string };
}
```

## Create/Update Data Types

Separate types for create and update payloads — the frontend mirror of the backend's narrow `createAttributes` struct, so a create or patch call cannot carry a derived read-only field. From `apps/web/src/types/models/vehicle.ts:36-53`:

```typescript
// Create payload — fields accepted by POST /fleets/{id}/vehicles.
export interface CreateVehicleAttributes {
  nickname?: string;
  make: string;
  model: string;
  trim?: string;
  year: number;
  vin?: string;
  currentMileage?: number;
  notes?: string;
}

// Patch payload — only these are mutable via PATCH /vehicles/{id}.
export interface UpdateVehicleAttributes {
  nickname?: string;
  currentMileage?: number;
  notes?: string;
}
```

Neither carries `status`, `lastActivityAt`, or `nextDue` — those are exactly the fields the doc comment on `VehicleAttributes` marks as server-derived.

## Importing Types

No path alias exists for `src` — import types with a relative path:

```typescript
import type { Vehicle } from '../../types/models/vehicle';
```

## TypeScript Strict Mode Features

The project's enhanced checks (`apps/web/tsconfig.app.json:11-12`):

```json
{
  "strict": true,
  "noUncheckedIndexedAccess": true
}
```

**Implications:**
- Always check array index access results for `undefined`
- Use `!` assertion only when you've already validated
