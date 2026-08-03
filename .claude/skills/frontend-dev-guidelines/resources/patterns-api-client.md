# API Client Patterns

## Overview

`apps/web/src/lib/api/client.ts` is 21 lines. It does not define an HTTP client — it **constructs** one. The `ApiClient` class lives in `packages/shared-ts/src/apiClient.ts`; `client.ts` wires it to MyFleet's auth contract and exports the single instance every service imports.

```typescript
// apps/web/src/lib/api/client.ts:17-21
export const apiClient = new ApiClient({
  baseUrl: '',
  getAccessToken,
  onRefresh: refreshAccessToken,
});
```

The export is named **`apiClient`**, not `api`. Nothing in `apps/web/src` imports it except `services/api/*.ts` (`FE-03`).

There is no response cache, no request deduplication, no exponential-backoff retry, no cancellation helper, and no progress-tracking helper. `packages/shared-ts/src/apiClient.ts` is 67 lines and has exactly two public methods. Application-level retry is React Query's `retry: 1` (`apps/web/src/components/providers/AppProviders.tsx:37`); caching and deduplication are React Query's job, keyed by query key (see `patterns-react-query.md`).

## The three constructor options

Each of the three is load-bearing and each has a hazard attached.

### `baseUrl: ''` — callers pass absolute gateway paths

Because the base URL is empty, every path handed to the client is the full gateway path, e.g. `/api/fleet/vehicles`. The prefix a service sees is **not** uniform, and the comment in `client.ts:8-12` records why:

> `baseUrl` is `''` so callers pass full gateway paths (`/api/<service>/...`).
> Traefik strips only `/api` before routing to auth-service and media-service
> (their routes already carry their own `/auth` or `/media` segment);
> fleet-service and notification-service still have their full
> `/api/<service>` prefix stripped.

`patterns-service-layer.md` documents the same asymmetry from the service side — a `basePath` copied from the wrong service is the most common way to get a 404 that looks like a backend bug.

### `getAccessToken` — the bearer token comes from localStorage

`apps/web/src/lib/api/token.ts:9-11` reads the key `access_token` from `localStorage`. The **refresh** token is an HttpOnly cookie the browser attaches automatically on `credentials: 'include'` requests; it is never read by JS (`token.ts:1-7`).

`ApiClient` attaches the token per request, and only when one is present:

```typescript
// packages/shared-ts/src/apiClient.ts:26-33
const token = this.opts.getAccessToken();
const res = await fetch(this.opts.baseUrl + path, {
  ...init,
  headers: {
    ...defaultHeaders,
    ...(token ? { Authorization: `Bearer ${token}` } : {}),
    ...(init.headers ?? {}),
  },
});
```

Note the ordering: `init.headers` is spread **last**, so a caller's own header always wins. That is what lets `MediaService.putContent` override the JSON:API `Content-Type` with the file's own type (`MediaService.ts:53`).

### `onRefresh` — the one-shot 401 retry

This is the single most consequential behaviour in the client.

```typescript
// packages/shared-ts/src/apiClient.ts:35-38
if (res.status === 401 && !retried) {
  const refreshed = await this.opts.onRefresh();
  if (refreshed) return this.fetchAuthenticated(path, init, defaultHeaders, true);
}
return res;
```

- **One shot only.** The recursive call passes `retried = true`, so a second 401 is returned to the caller rather than looping.
- **Failure is silent by design.** `refreshAccessToken` returns `null` when the mint fails (`apps/web/src/lib/api/refresh.ts:59-63`), and the `if (refreshed)` guard then falls through to `return res` — the original 401 surfaces to the caller as a normal error. Do not write code that assumes a refresh was attempted and succeeded.
- **Refreshes are deduped.** `mintAccessToken` shares one in-flight promise across all callers (`refresh.ts:15,45-52`). The reason is in the comment at `refresh.ts:7-13`: auth-service rotates the refresh token on use and treats a replay as reuse, revoking the whole token family — two concurrent POSTs to `/auth/refresh` with the same cookie log the user out everywhere.
- **Two entry points, different failure semantics.** `mintAccessToken()` leaves the existing token alone on failure; use it when you only want fresher claims (e.g. after accepting an invite, where `active_fleet_id` and `role` were fixed into the JWT at mint time). `refreshAccessToken()` additionally clears the stale token on failure, because a 401 already proved the session is dead (`refresh.ts:35-62`).

## The real method surface

Two methods. That is the whole API.

```typescript
// packages/shared-ts/src/apiClient.ts:42,59
request<T>(path: string, init: RequestInit = {}, retried = false): Promise<T>
requestBlob(path: string, init: RequestInit = {}, retried = false): Promise<Blob>
```

There is no `getList` / `getOne` / `post` / `put` / `patch` / `delete` / `upload` / `download`. The HTTP verb is passed through `init`, exactly as with `fetch`:

```typescript
// BaseService.ts:44-47
const doc = await apiClient.request<JsonApiDocument<JsonApiResource<A>>>(path, {
  method: 'POST',
  body: JSON.stringify({ data: { type: this.resourceType, attributes } }),
});
```

`request` sets `Content-Type: application/vnd.api+json` by default and parses the body as JSON, treating a `204` as `null` (`apiClient.ts:46,49`). `requestBlob` sets **no** `Content-Type` and returns the raw `Blob` — it exists for media bytes proxied through the API (`apiClient.ts:54-66`, used by `MediaService.getContentBlob`, `MediaService.ts:81`).

There is no third options parameter. `RequestInit` is the standard DOM type; anything you want to set — `method`, `body`, `headers`, `signal` — goes there.

## File uploads

Upload is an ordinary `request` call with a `File` as the body and an overridden `Content-Type`. There is no progress callback, because there is no XHR layer to report progress from — `ApiClient` uses `fetch`.

```typescript
// MediaService.ts:47-57
async putContent(id: string, file: File): Promise<JsonApiResource<MediaObjectAttributes>> {
  const doc = await apiClient.request<JsonApiDocument<JsonApiResource<MediaObjectAttributes>>>(
    `${this.basePath}/${id}/content`,
    {
      method: 'PUT',
      body: file,
      headers: { 'Content-Type': file.type || 'application/octet-stream' },
    },
  );
  return doc.data;
}
```

Bytes are proxied through media-service rather than presigned to MinIO directly, because MinIO is a shared cluster service that is never reachable from the browser (`MediaService.ts:20-21`). Going through `apiClient` is what applies the bearer token and the 401 refresh to the upload.

## Error handling

`request` throws on any non-2xx, converting the JSON:API error envelope into a typed `ApiError`:

```typescript
// packages/shared-ts/src/apiClient.ts:49-51
const body = res.status === 204 ? null : await res.json().catch(() => null);
if (!res.ok) throw createErrorFromUnknown({ status: res.status, body });
return body as T;
```

`createErrorFromUnknown` is exported from **`@myfleet/shared-ts`** (`packages/shared-ts/src/errors.ts:23`) — there is no `lib/api/errors.ts`, and no `isRetryableError`, `requiresAuthentication`, `getErrorActions` or `transformError` anywhere in the tree. `lib/api/` holds exactly four modules: `authRoutes.ts`, `client.ts`, `refresh.ts`, `token.ts`.

`ApiError` carries `status`, `code`, `message`, and optionally `detail` and `pointer` — the last read from the JSON:API `source.pointer`, so a field-level validation error can be routed back to the form field that caused it (`errors.ts:3-16,27-33`). When the response has no JSON:API error envelope it degrades to `status: 0, code: 'unknown'` rather than throwing (`errors.ts:35-36`).

Call sites catch and surface, they do not classify (`FE-09`):

```typescript
try {
  await mutateAsync(values);
} catch (e) {
  toast.error(createErrorFromUnknown(e).message);
}
```

## Login is not an `apiClient` call

The OAuth entry point is a browser navigation, not a fetch. `buildLoginUrl(returnTo?)` (`apps/web/src/lib/api/authRoutes.ts:23-26`) builds `/api/auth/login/google`, optionally with a `return_to` query parameter, and rejects protocol-relative values (`//host`, `/\host`) that browsers would resolve off-site (`authRoutes.ts:10-12`). auth-service sanitizes `return_to` authoritatively; the client-side check only keeps an off-site value from leaving the browser.
