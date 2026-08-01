# Vehicle Card Photo & Quick Actions — Design

Task: `task-005-vehicle-card-photo-actions`
Version: v1
Status: Approved
Created: 2026-07-31
Source PRD: [`prd.md`](./prd.md)

---

## 1. Summary

Three separable pieces of work, in dependency order:

1. **media-service** learns to serve the already-generated `thumbnail` / `display`
   variants from the existing `GET /media/{id}/content` route, behind a `variant`
   query parameter. Authorization, caching, and the no-parameter behaviour are
   unchanged.
2. **web** gains a runtime configuration mechanism — a `config.json` fetched
   before the React tree mounts — because the Carfax URL template must be
   changeable without rebuilding the image.
3. **web** rebuilds `VehicleCard` around a photo thumbnail and two icon-button
   actions, and plumbs a `variant` through the media service client, query key,
   and content hook so the list requests kilobytes instead of megabytes.

Piece 1 is independently shippable and testable. Pieces 2 and 3 are both
frontend but touch disjoint files; 3 depends on 2 only for the template value.

## 2. Decisions taken during design

The PRD left five questions open (§9) and contained one factual error. All are
resolved here.

### D1 — Cross-fleet access returns **404**, not 403 (corrects PRD §5.1)

The PRD's API table and acceptance criteria say a media object belonging to
another fleet yields `403`. The code does not do that and should not start.
`AuthorizeAccess` (`apps/media-service/internal/mediaobject/processor.go:45`)
maps a fleet mismatch to `server.ErrNotFound`, with the comment *"if the object
belongs to a different fleet we return 404 so cross-fleet existence is never
leaked."*

A `403` would restore exactly the existence oracle that comment exists to
prevent: `403` means "this id exists, just not for you", `404` means "no such
id". The variant path inherits `AuthorizeAccess` unchanged and therefore
inherits `404`.

**Acceptance criterion, restated:** a variant request for a media object in
another fleet returns `404`, and no object-store read is attempted.

### D2 — Invalid `variant` returns 400 via a new `server.ErrBadRequest`

`packages/shared-go/server/errors.go` has no 400 sentinel — the closest is
`ErrValidation` (422). Rather than relax FR-7.3 to 422, add:

```go
ErrBadRequest = errors.New("bad request") // 400
```

plus its `StatusFor` case and a `codeFor(400) → "bad_request"` entry in
`server.go`. Additive, used by every service, and it lets the handler stay a
one-liner (`server.WriteError(w, err)`).

**`codeFor` gap, opportunistically fixed:** `codeFor` has no `413` case either,
so today a 413 body carries `"code": "internal_error"`. Since we are editing
that switch anyway, add `case 413: return "payload_too_large"`. Called out
explicitly so it is not mistaken for scope creep during review.

### D3 — Carfax template is **runtime**-configurable via `config.json`

The PRD assumed build-time `import.meta.env`. Rejected: Vite inlines
`import.meta.env` at build time, so a template change would require rebuilding
and republishing the `web` image and re-pinning the overlay digest — a full
release cycle to edit one URL.

Instead the SPA fetches a small JSON document before mounting. This is the first
runtime-config mechanism in the frontend, so §5 designs it as a general
facility with exactly one key today, not as a Carfax special case.

The cost is real and accepted: one same-origin request (~60 bytes, served from
disk by nginx) ahead of first paint, a new failure mode, and a `deploy/k8s`
change. §5.4 covers the failure handling; §5.5 covers the new injection surface
that runtime-supplied URLs create.

`VITE_CARFAX_URL_TEMPLATE` is **not** introduced. No `import.meta.env` usage is
added anywhere.

### D4 — The endpoint accepts all three variants

`original | thumbnail | display`. The `display` variant is generated and stored
on every image upload already
(`apps/media-service/internal/processing/worker.go:156-169`); refusing to serve
bytes we are already paying to produce buys nothing, and accepting it costs one
entry in a parse table. Nothing in this task requests `display`.

### D5 — VIN format is not validated (confirms PRD §9.2)

The Carfax button appears for any non-empty trimmed VIN. Requiring 17
characters would hide the button for legitimately non-standard VINs on older or
imported vehicles, which is a worse failure than landing on a Carfax error page.

## 3. Backend — media-service

### 3.1 Package boundary (resolves PRD §9.3)

`mediaobject` must resolve a variant's `object_key`, which lives in
`mediavariant`. Three options:

| Option | Shape | Verdict |
| --- | --- | --- |
| A | `mediaobject` imports `mediavariant` directly | Works — no import cycle, and `processing/worker.go` already imports both. But it makes `mediaobject` untestable without `mediavariant`, and points an arrow between two sibling domain packages that currently have none. |
| B | `mediaobject` declares a narrow port; adapter built in the composition root | **Chosen.** |
| C | New `internal/mediavariantport` package holding the adapter | A whole package for a ten-line translation. Rejected. |

Option B mirrors a precedent sitting in the very file being edited: `ObjectStore`
(`processor.go:36`) is declared in `mediaobject`, implemented by
`storage.Client`, and `storage` does not import `mediaobject`. Same shape here:

```go
// mediaobject/processor.go — the port and its value type are owned by mediaobject.

// VariantRef is what the processor needs to stream a derived image: where the
// bytes live and what they are. Nothing else about a variant is relevant here.
type VariantRef struct {
    ObjectKey   string
    ContentType string
}

// VariantLookup resolves a derived image for a media object. A miss is a normal
// outcome (found=false), not an error — variants do not exist until the
// processing worker has run, and never exist for non-image media.
type VariantLookup interface {
    Lookup(mediaObjectID, variant string) (VariantRef, bool, error)
}
```

`variant` crosses the port as a plain `string`, so the implementer does not need
`mediaobject`'s types either. The adapter is an unexported ten-line type in
`apps/media-service/cmd/main.go` — the composition root, which already imports
both packages — translating `mediavariant.Provider` into `VariantLookup`. The
dependency graph stays a tree, and `Processor` tests fake the port with a
struct literal.

`InitializeRoutes` gains the port as a parameter:

```go
mediaobject.InitializeRoutes(log, db, store, variantLookup, maxUploadBytes)(pr)
```

It cannot construct the adapter itself from its `db` argument without importing
`mediavariant`, which is the thing option B exists to avoid.

### 3.2 `mediavariant` gains a narrower read

`Provider` today offers only `ListByMediaObject`, which loads both rows to use
one. Add:

```go
// GetByMediaObjectAndVariant returns the named variant, or found=false when the
// worker has not produced it (or never will, for non-image media).
GetByMediaObjectAndVariant(mediaObjectID string, v Variant) (Model, bool, error)
```

Backed by a `WHERE media_object_id = ? AND variant = ?` query. `ListByMediaObject`
stays — the worker's replace path uses it.

### 3.3 Variant parsing — a pure function

```go
type ContentVariant string

const (
    ContentOriginal  ContentVariant = "original"
    ContentThumbnail ContentVariant = "thumbnail"
    ContentDisplay   ContentVariant = "display"
)

// ParseContentVariant maps the raw query value to a variant. An absent or empty
// parameter means the original, preserving the pre-existing contract exactly.
// Anything else is server.ErrBadRequest — never a silent fallback to the
// original, which would ship multi-megabyte responses for a typo (FR-7.3).
func ParseContentVariant(raw string) (ContentVariant, error)
```

- `""` (absent, or `?variant=` with an empty value) → `ContentOriginal`.
- Exact lowercase matches only. `?variant=Thumbnail` is a 400. Case-insensitive
  matching would be a courtesy that makes the accepted set fuzzy; strictness
  costs a caller one character and keeps the contract exact.

Pure and dependency-free, so it is unit-tested directly — the same pattern as
`classifyUploadError` (`resource.go:32`).

### 3.4 `Processor.Content` takes the variant — one path, not two

PRD §7 suggested adding a variant-aware read *alongside* `Content`. Rejected:
two content methods means two places that must each remember to call
`AuthorizeAccess` first, and a future edit to one can silently diverge from the
other. Authorization is the one thing here that must not have a second
implementation. The existing method gains a parameter instead; there is exactly
one caller (`resource.go:151`) plus tests.

```go
// ContentInfo describes the bytes actually being served, which is not always
// the media object's own metadata — a variant is re-encoded and has its own
// content type, and its length is not recorded anywhere.
type ContentInfo struct {
    ContentType string
    Size        int64          // 0 = unknown; the handler omits Content-Length
    Served      ContentVariant // what was served, which may be Original after a fallback
}

func (pr *Processor) Content(
    ctx context.Context, id, identityFleetID string, want ContentVariant,
) (ContentInfo, io.ReadCloser, error)
```

Returning `ContentInfo` instead of the `Model` is what makes FR-7.6 and FR-7.8
fall out for free: the handler sets headers from the bytes it is about to write
and no longer has to know whether a `Model`'s `Size` describes them.

Order of operations, unchanged in its first two steps:

1. `pr.GetByID(id, identityFleetID)` — resolves and fleet-scopes. Cross-fleet
   exits here with `404` (D1) **before** any variant lookup or store read
   (FR-7.5).
2. If `want == ContentOriginal` → serve `m.ObjectKey()`, `ContentType:
   m.ContentType()`, `Size: m.Size()`. Byte-for-byte the current behaviour.
3. Otherwise `pr.variants.Lookup(id, string(want))`:
   - **found** → serve `ref.ObjectKey`, `ContentType: ref.ContentType`,
     `Size: 0`. If the variant row's `content_type` is empty (it should never
     be — variants are re-encoded), fall back to the media object's.
   - **not found** → serve the original (FR-7.4) and log at **debug** with
     `media_id` and the requested variant (NFR-12). This is the normal state
     for media whose processing has not finished and for anything that is not a
     processable image; it is not a warning.
4. `pr.storage.GetObject(ctx, key)`. A `storage.ErrObjectNotFound` on the
   **original** stays `404`, exactly as today (`processor.go:219-227`).

**Edge case the PRD does not cover:** a variant row exists but its object is
missing from MinIO. Serve the original instead, and log at **warn** — unlike
the step-3 miss, this is DB/store drift and someone should see it. A `404` here
would be a worse answer: the resource plainly exists and we are holding readable
bytes for it. Cost is one extra store round trip on a path that should never be
taken.

### 3.5 Route handler

`GET /media/{id}/content` (`resource.go:148`) gains four lines at the top:

```go
v, err := ParseContentVariant(req.URL.Query().Get("variant"))
if err != nil {
    server.WriteError(w, err) // 400 + JSON:API error envelope, no image bytes
    return
}
```

and the header block becomes:

```go
if info.ContentType != "" { w.Header().Set("Content-Type", info.ContentType) }
if info.Size > 0          { w.Header().Set("Content-Length", strconv.FormatInt(info.Size, 10)) }
w.Header().Set("Cache-Control", "private, max-age=300")
```

`Cache-Control: private, max-age=300` is set unconditionally, so it applies to
variants as well (FR-7.7, NFR-5). `Content-Length` is omitted for variants
because `media_variants` records `width`/`height`/`content_type` but no byte
count (`mediavariant/entity.go`) — satisfying FR-7.8, which requires that the
original's size must not be sent for a variant response.

**Not designed in:** a `Vary: ...` header. The variant is in the URL's query
string, so distinct variants are already distinct cache keys.

### 3.6 Backend tests (NFR-13)

- `ParseContentVariant` — table test: `""`, each valid value, an unknown value,
  a wrong-case value. Pure, no HTTP.
- `Processor.Content` — fake `VariantLookup` + fake `ObjectStore`: variant
  found; variant missing → original bytes; variant row present but object
  missing → original bytes; original requested → untouched behaviour.
- `resource_test.go` via the existing `testRouter` helper: `?variant=thumbnail`
  serves variant bytes with the variant's content type and **no**
  `Content-Length`; `?variant=display`; `?variant=original`; omitted parameter
  is byte-identical to today including `Content-Length`; `?variant=bogus` → 400
  with a JSON:API error body and an empty image payload; cross-fleet →
  404 with the store never called.

The "store never called" assertion needs the fake `ObjectStore` to record calls
— the existing fakes in `resource_test.go` are the place to add that.

## 4. Frontend — media variant plumbing

Four small, mechanical changes.

**Type** (`types/models/media.ts`):

```ts
export type MediaVariant = 'original' | 'thumbnail' | 'display';
```

**Service** (`services/api/MediaService.ts`):

```ts
async getContentBlob(id: string, variant: MediaVariant = 'original'): Promise<Blob> {
  const suffix = variant === 'original' ? '' : `?variant=${variant}`;
  return apiClient.requestBlob(`${this.basePath}/${id}/content${suffix}`);
}
```

The parameter is omitted rather than sent as `?variant=original`, so every
existing request stays byte-identical on the wire and the acceptance criterion
"verifiable as a `?variant=thumbnail` request in the network panel" reads
cleanly.

**Query key** (`lib/hooks/api/media.ts`):

```ts
content: (id: string, variant: MediaVariant = 'original') =>
  [...mediaKeys.contents(), id, variant] as const,
```

A thumbnail and an original for the same media id must not collide in the cache
— the entries hold different bytes and one would be served in place of the
other. The `mediaKeys.contents()` prefix is unchanged, so any prefix-based
invalidation still matches. The cache is in-memory only, so the key-shape change
needs no migration.

**Hook:** `useMediaContentUrl(id, variant: MediaVariant = 'original')`. The
object-URL lifecycle (`media.ts:120-152`) is untouched — its create/revoke
effect and the deliberate one-frame `isLoading` hold are exactly what
`VehiclePhotoThumbnail` needs to avoid flashing a placeholder. `staleTime`
stays at 5 minutes, which is what satisfies FR-2.5 and NFR-2.

**`MediaThumbnail` is left unchanged.** PRD §7 proposed giving it a `variant`
prop; §6.2 explains why the card gets its own component instead. The gallery
therefore keeps requesting originals — a known, deliberate inefficiency, now a
one-line fix whenever the detail page is next opened, since the plumbing is
here.

## 5. Frontend — runtime configuration

### 5.1 Delivery

A ConfigMap-backed `config.json`, served as a static file by the SPA's own
nginx:

```
image:      /usr/share/nginx/html/config/config.json   (baked-in default)
k8s:        ConfigMap `web-config` mounted read-only at /usr/share/nginx/html/config
URL:        /config/config.json
```

The ConfigMap is mounted as a **directory**, replacing the baked-in one, rather
than with `subPath`. `subPath` mounts do not receive ConfigMap updates without
a pod restart, and the manifests use plain `configmap.yaml` files rather than
a hash-suffixed `configMapGenerator`, so a `kubectl apply` of an edited
ConfigMap would otherwise leave every pod serving the old value with no signal
that anything had happened.

`readOnlyRootFilesystem: true` (`deploy/k8s/base/web/deployment.yaml`) is
unaffected — ConfigMap volumes are read-only mounts.

The baked-in default means the image works with no ConfigMap at all: local
`vite dev` (the file lives in `apps/web/public/config/config.json`, which Vite
serves at `/config/config.json` and copies into `dist/`), `docker run` of the
image alone, and any overlay that has not adopted the ConfigMap.

`nginx.conf` gains:

```nginx
location = /config/config.json {
    add_header Cache-Control "no-cache";
}
```

Without it a browser may serve a cached template long after the ConfigMap
changed, which would quietly defeat the entire point of choosing runtime
configuration.

### 5.2 Shape

```json
{ "carfaxUrlTemplate": "https://www.carfax.com/VehicleHistory/p/Report.cfx?vin={vin}" }
```

One key today. The module is designed as a general facility so the second key
does not require redesigning it.

### 5.3 Module (`lib/config/runtimeConfig.ts`)

```ts
export interface RuntimeConfig { carfaxUrlTemplate: string }

export const DEFAULT_RUNTIME_CONFIG: RuntimeConfig = {
  carfaxUrlTemplate: 'https://www.carfax.com/VehicleHistory/p/Report.cfx?vin={vin}',
};

/** Pure: validates one parsed document, falling back per field. Never throws. */
export function parseRuntimeConfig(raw: unknown): RuntimeConfig;

/** Fetches, parses, and latches the config. Never throws, never rejects. */
export function loadRuntimeConfig(): Promise<RuntimeConfig>;

/** The latched config. Returns defaults until loadRuntimeConfig has resolved. */
export function getRuntimeConfig(): RuntimeConfig;
```

`parseRuntimeConfig` validates with `zod` (already a dependency) and falls back
**per field**, so one malformed key does not discard a document that is
otherwise fine. Unknown keys are ignored. Being pure, it is unit-tested without
touching `fetch` or the module's state.

A module-level latched value rather than React context: the Carfax URL builder
must stay a pure function (NFR-15), and threading context into it would make it
a hook and its tests a render harness. The component reads
`getRuntimeConfig().carfaxUrlTemplate` and passes the string into the pure
builder, so the builder's tests never touch the singleton.

### 5.4 Bootstrap and failure handling

`main.tsx` loads the config before mounting:

```ts
void loadRuntimeConfig().then(() => {
  createRoot(rootElement).render(/* ... */);
});
```

`loadRuntimeConfig` catches everything internally, so `.catch` is not the
mechanism — a `.then` that always runs is. Every failure resolves to
`DEFAULT_RUNTIME_CONFIG` and logs one `console.warn`:

| Failure | Result |
| --- | --- |
| `404` (no ConfigMap, older image) | defaults |
| Network error | defaults |
| Malformed JSON | defaults |
| Valid JSON, wrong field types | defaults for the bad fields only |
| Request hangs | defaults, after a 2s `AbortController` timeout |

The timeout matters more than it looks: without it, a wedged nginx worker turns
a missing 60-byte file into a permanent white screen. **Nothing about a config
failure may prevent the app from rendering** — the compiled-in default is a
correct Carfax URL, so the feature keeps working regardless.

Cost: one same-origin request ahead of first paint. The app already gates its
first meaningful render on the auth bootstrap (`AuthContext.isLoading`), so this
is not a new class of delay.

### 5.5 Runtime config is an injection surface — mitigated in the builder

Moving the template out of the bundle means whoever can edit the ConfigMap can
choose the URL an anchor's `href` points at. A `javascript:` template would
then be stored XSS. §6.1's builder therefore validates the **constructed URL's
scheme** and refuses anything that is not `https:`.

This is a consequence of D3 that the build-time option did not have, and it is
why the check belongs in the builder (where it is unit-testable) rather than in
config parsing alone.

## 6. Frontend — the card

### 6.1 Carfax URL builder (`lib/carfax.ts`)

```ts
export const VIN_PLACEHOLDER = '{vin}';

/**
 * Builds a Carfax report URL, or returns null when no button should render.
 */
export function buildCarfaxUrl(template: string, vin: string | null | undefined): string | null;
```

Returns `null` — meaning *render no button* — when any of:

- `vin` is null/undefined, or empty after `trim()` (FR-5.2).
- `template` does not contain `{vin}` (FR-5.6). A template that ignores the VIN
  would send every user to the same generic page; failing closed is the correct
  reading of "the button opens *this vehicle's* report".
- The constructed URL does not parse, or its scheme is not `https:` (§5.5).

Otherwise every occurrence of `{vin}` is replaced with
`encodeURIComponent(vin.trim())` (NFR-7).

`https:`-only is deliberately strict. Carfax is a public HTTPS site; permitting
`http:` would allow a config change to silently downgrade a link carrying a
VIN. Relaxing it later is a one-line change with a visible rationale attached.

Pure, no React, no config import — the template arrives as an argument.
Directly unit-testable (NFR-15).

### 6.2 `VehiclePhotoThumbnail` — a new component, not `MediaThumbnail`

`components/features/vehicles/VehiclePhotoThumbnail.tsx`. Reusing
`MediaThumbnail` was considered and rejected on four counts:

1. It calls `useMediaObject(mediaId)` for its `alt` text
   (`MediaThumbnail.tsx:43`) — one extra `GET /media/{id}` per card. On a
   twelve-vehicle list that is twelve avoidable requests, against NFR-2 and the
   intent of NFR-3. The card already knows the vehicle's name and needs no
   metadata at all.
2. Its failure state is a red `border-destructive` tile reading "Load failed"
   (`:49-65`). FR-2.4 requires the neutral placeholder instead — a wall of red
   tiles is not something a user can act on.
3. Its empty state is the text "No image" (`:67-78`); FR-3.1 requires the `Car`
   icon.
4. Every dimension is hardcoded `h-24 w-24` and overridden by `className`;
   the card wants `h-20 w-20` (FR-1.2).

What survives is the *hook*, which is where the value is. The two components
end up sharing `useMediaContentUrl` and nothing else — which is the correct
seam.

```
Props: { mediaId?: string; vehicleLabel: string; className?: string }
```

Four states, all occupying an identical `h-20 w-20 shrink-0 rounded-md` box so
cards align across a grid row whatever each one resolves to (FR-3.2, FR-1.3):

| State | Render |
| --- | --- |
| No `mediaId` | Placeholder, `role="img" aria-label="No photo"` |
| Loading | `<Skeleton>` of the same size (FR-2.3) |
| `isError` | Placeholder, `role="img" aria-label="Photo unavailable"` |
| Has url | `<img alt={`Photo of ${vehicleLabel}`} className="object-cover">` |

Placeholder = `bg-muted` with a centred `Car` icon marked `aria-hidden`, the
label carried by `aria-label` on the wrapper. The two placeholder labels differ
(FR-3.3, NFR-11) so the failure remains distinguishable to assistive technology
even though it is visually identical — the visual sameness is the point of
FR-2.4, and the label is what keeps that from erasing information.

No toast on error, by construction: the component has no toast call. N broken
thumbnails produce N placeholders and zero notifications.

### 6.3 `VehicleCard` layout

The wrapping `<Link>` (`VehicleCard.tsx:27`) is deleted. Structure:

```
<Card className="p-4">
  <div className="flex items-start gap-4">
    <VehiclePhotoThumbnail mediaId={…} vehicleLabel={title} />   {/* shrink-0 */}
    <div className="min-w-0 flex-1">
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">      {/* title + subtitle, truncate */}
        {status && <StatusBadge />}    {/* FR-1.5 */}
      </div>
      {mileage}
    </div>
  </div>
  <div className="mt-3 flex items-center justify-end gap-1">
    {/* detail button, then Carfax button — FR-6.2 */}
  </div>
</Card>
```

`min-w-0` on both flex children is what makes `truncate` work at all inside a
flex row; without it the text sets a minimum content width and the card
overflows horizontally at the single-column breakpoint (FR-1.6). This is the
one layout detail most likely to be dropped and it is the one FR-1.6 tests.

The action row sits at the bottom rather than beside the status badge: it keeps
actions visually separate from the badge (FR-1.5), gives both buttons a stable
position regardless of whether mileage renders (FR-6.2), and avoids crowding
three elements into the top-right corner.

Both buttons use the existing `Button` primitive with `variant="ghost"
size="icon"` — 40×40 (`button.tsx:24`), satisfying NFR-9, and carrying the
existing `focus-visible:ring-2` focus ring (FR-6.3, `button.tsx:6`).

**Detail button** — a real anchor, not an `onClick` (FR-4.3):

```tsx
<Button asChild variant="ghost" size="icon">
  <Link to={`/vehicles/${vehicle.id}`} aria-label={`Open details for ${title}`}>
    <ChevronRight aria-hidden="true" />
  </Link>
</Button>
```

`Button asChild` renders through a Radix `Slot` (`button.tsx:39`), so the
element is the react-router `<Link>`'s `<a>` and middle-click, cmd/ctrl-click,
and the link context menu all work. Shown for every role including `viewer`
(FR-4.6) — nothing here is gated on write permission, and `requireWrite` is not
consulted.

**Carfax button** — rendered only when `buildCarfaxUrl` returns non-null:

```tsx
<Button asChild variant="ghost" size="icon">
  <a href={carfaxUrl} target="_blank" rel="noopener noreferrer"
     aria-label={`View Carfax report for ${title} (opens in a new tab)`}>
    <History aria-hidden="true" />
  </a>
</Button>
```

A plain `<a>`, not a `<Link>` — this leaves the SPA. `rel="noopener
noreferrer"` blocks `window.opener` access and suppresses the referrer
(FR-5.4). No `onMouseEnter` handler, no prefetch, no `<link rel="prefetch">`:
nothing contacts Carfax until a click, so no VIN leaves the browser before then
(FR-5.7, NFR-6).

`title` (nickname, else `year make model`) is what the user reads on the card,
so it is what both `aria-label`s name — a grid of buttons all labelled "Open
details" is the regression FR-4.4 and NFR-8 exist to prevent.

Both icons are decorative and `aria-hidden` (FR-4.5); the accessible name comes
entirely from the label.

**Accepted regression:** the whole-card click target is gone (NFR-10). Two
40×40 targets replace one card-sized one. The trade is eliminating a link
nested inside a link, which made the Carfax button's click behaviour ambiguous
and is invalid HTML besides.

### 6.4 `VehicleList` skeleton

The card grows to roughly `p-4` (32) + 80 thumbnail + `mt-3` (12) + 40 button
row ≈ 164px. The loading skeleton moves from `h-28` to `h-44` (176px). This is
an estimate; it should be eyeballed against the rendered card and nudged, since
its only job is to stop the skeleton-to-content transition from jumping.

## 7. Deployment

- **New** `deploy/k8s/base/web/configmap.yaml` — ConfigMap `web-config`, one
  key `config.json`, holding the default template. Real value, not a
  placeholder, so the `main` overlay stays placeholder-free.
- `deploy/k8s/base/web/deployment.yaml` — a `configMap` volume, mounted
  `readOnly` at `/usr/share/nginx/html/config`.
- `deploy/k8s/base/kustomization.yaml` — add `web/configmap.yaml` alongside the
  existing `web/` resources.

No Secret, no PVC, no ClusterRole — the `main` overlay's constraints are
unaffected.

Per CLAUDE.md, manifests have no test suite, so verification is both renders
**and** both server dry-runs — the local overlay is not exempt:

```sh
kustomize build deploy/k8s/overlays/main
kustomize build deploy/k8s/overlays/local
kustomize build deploy/k8s/overlays/main  | kubectl apply --dry-run=server -f -
kustomize build deploy/k8s/overlays/local | kubectl apply --dry-run=server -f -
```

## 8. Testing

### Backend

Covered in §3.6.

### Frontend

- `lib/carfax.test.ts` — pure: happy path; VIN needing encoding; VIN with
  surrounding whitespace; empty/whitespace VIN → null; undefined VIN → null;
  template without `{vin}` → null; template with multiple `{vin}` occurrences;
  non-`https:` template → null (§5.5).
- `lib/config/runtimeConfig.test.ts` — `parseRuntimeConfig` against a valid
  document, a wrong-typed field, an unknown extra key, and a non-object; then
  `loadRuntimeConfig` against a mocked `fetch` for 404, network error,
  malformed body, and timeout, each asserting the default and no throw.
- `VehiclePhotoThumbnail` component tests — photo, no `mediaId`, error →
  placeholder with the right accessible label, and loading.
- `VehicleCard` component tests — with/without photo, with/without VIN
  (asserting the Carfax button's presence, `href`, `target`, and `rel`), and
  that the detail button's `aria-label` includes the vehicle's name.

Two pieces of test infrastructure are needed, both small and both reusable:

- `src/test/renderWithProviders.tsx` — wraps a render in `QueryClientProvider`
  (with `retry: false`) and `MemoryRouter`. This is the first component test
  under `components/`, so no such helper exists; every future one wants it.
- The `URL.createObjectURL` / `revokeObjectURL` stub. jsdom implements neither.
  `lib/hooks/api/media.test.ts:126-133` already has exactly the right stub —
  lift it into `src/test/` rather than copying it.

## 9. Risks

| Risk | Mitigation |
| --- | --- |
| The runtime-config fetch delays or blocks first paint | 2s abort timeout; every failure resolves to the compiled-in default; the app renders regardless (§5.4) |
| A ConfigMap edit injects a hostile URL scheme | `buildCarfaxUrl` accepts only `https:` and returns null otherwise (§5.5, §6.1) |
| ConfigMap edits do not reach running pods | Directory mount rather than `subPath`, so kubelet propagates updates (§5.1) |
| Stale `config.json` cached in browsers | `Cache-Control: no-cache` on that exact location (§5.1) |
| Cache collision between an original and a thumbnail | Variant is part of the React Query key (§4) |
| Cards become too tall at `lg:grid-cols-3` (PRD §9.4) | A visual judgement, not a spec item — assess against the running UI; the grid class is a one-token change |
| The `Content` signature change misses a caller | Exactly one production caller (`resource.go:151`); the compiler finds the rest |

## 10. Out of scope

Unchanged from PRD §2, plus, as consequences of the decisions above:

- The vehicle detail gallery keeps requesting **original** bytes. The plumbing
  to switch it is in place (§4) but the change is not made here, honouring the
  PRD's "no changes to the vehicle detail page's photo gallery" non-goal.
- No `import.meta.env` / `VITE_*` variable is introduced anywhere (D3).
- No VIN validation, decoding, or format enforcement (D5).
- No `Vary` header, no CDN or shared-cache work — variant bytes are `private`
  by design (§3.5).
