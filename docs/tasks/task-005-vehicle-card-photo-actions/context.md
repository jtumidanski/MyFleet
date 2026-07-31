# Vehicle Card Photo & Quick Actions — Implementation Context

Companion to [`plan.md`](./plan.md). Everything here was verified against the
source in this worktree on 2026-07-31, not recalled.

Task: `task-005-vehicle-card-photo-actions`
Worktree: `/home/tumidanski/source/MyFleet/.worktrees/task-005-vehicle-card-photo-actions`
Branch: `task-005-vehicle-card-photo-actions`
Upstream docs: [`prd.md`](./prd.md), [`design.md`](./design.md)

---

## 1. What exists today

### Backend — the route being extended

`GET /media/{id}/content` lives at `apps/media-service/internal/mediaobject/resource.go:148-178`.
It calls `Processor.Content` (`processor.go:212`), which authorizes by fleet via
`GetByID` → `AuthorizeAccess`, then streams `m.ObjectKey()` from the object
store. Headers today come from the `Model`: `Content-Type` from
`m.ContentType()`, `Content-Length` from `m.Size()`, plus a hardcoded
`Cache-Control: private, max-age=300`.

The variants the feature needs are **already generated and persisted**.
`apps/media-service/internal/processing/worker.go:156-174` builds a `thumbnail`
(max edge 320) and a `display` (max edge 1280) for every uploaded image and
writes them via `mediavariant.Administrator.ReplaceForMediaObject`. Nothing
serves them over HTTP — that is the entire backend half of this task.

Key files and what they contain:

| File | Relevant contents |
| --- | --- |
| `internal/mediaobject/processor.go` | `ObjectStore` port (`:36`) — the precedent the new `VariantLookup` port copies; `AuthorizeAccess` (`:45`); `Content` (`:212`); `getActive` (`:265`) |
| `internal/mediaobject/resource.go` | `requireWrite` (`:20`), `classifyUploadError` (`:32`) — the pure-function-for-testability precedent `ParseContentVariant` follows; `InitializeRoutes` (`:44`) |
| `internal/mediaobject/model.go` | `Model.Size() int64` (`:37`), `Model.ContentType() string` (`:36`), `Model.ObjectKey() string` (`:35`) |
| `internal/mediavariant/model.go` | `Variant` type with `VariantThumbnail` / `VariantDisplay`; `Model` accessors including `ObjectKey()` and `ContentType()` |
| `internal/mediavariant/entity.go` | Table `media.media_variants`, columns `id, media_object_id, variant, object_key, width, height, content_type, created_at` — **no byte-count column**, which is why `Content-Length` is omitted for a variant |
| `internal/mediavariant/provider.go` | `Provider` with only `ListByMediaObject` |
| `internal/mediavariant/builder.go` | `NewBuilder()` with `SetMediaObjectID/SetVariant/SetObjectKey/SetWidth/SetHeight/SetContentType`; `Build()` requires the first three |
| `cmd/main.go:124` | The single production call to `mediaobject.InitializeRoutes` — it already imports `mediavariant` (`:23`), so it is the natural home for the adapter |

### Backend — shared error handling

`packages/shared-go/server/errors.go` defines sentinels for 401/403/404/409/410/413/422.
**There is no 400 sentinel** — the closest, `ErrValidation`, is 422.
`packages/shared-go/server/server.go:12-29` maps a status to a JSON:API `code`
string; it has no `400` case and no `413` case, so both currently render as
`"internal_error"`. `server.WriteError` (`jsonapi.go:29`) writes
`{"errors":[{"status","code","title"}]}`.

### Frontend

| File | Relevant contents |
| --- | --- |
| `src/components/features/vehicles/VehicleCard.tsx` | The whole card is wrapped in `<Link to={`/vehicles/${id}`}>` (`:27`) — this is what gets deleted |
| `src/components/features/vehicles/VehicleList.tsx:15` | Loading skeleton is `h-28` |
| `src/components/features/vehicles/media/MediaThumbnail.tsx` | Deliberately **not** reused. Calls `useMediaObject(mediaId)` (`:43`) for alt text — one extra request per tile; renders a red `border-destructive` "Load failed" tile (`:49-65`); empty state is the text "No image" (`:67-78`); hardcoded `h-24 w-24` |
| `src/lib/hooks/api/media.ts` | `mediaKeys` factory (`:13-21`); `useMediaContentUrl` (`:120-152`) with its deliberate one-frame `isLoading` hold and matched create/revoke effect — **do not touch that logic**, only the query key and `queryFn` |
| `src/services/api/MediaService.ts:55` | `getContentBlob(id)` — no variant |
| `src/types/models/vehicle.ts` | `VehicleAttributes` already carries `vin?: string` and `primaryImageMediaId?: string` |
| `src/components/ui/button.tsx` | `asChild` renders through a Radix `Slot` (`:39`); `size: 'icon'` is `h-10 w-10` (40×40, NFR-9); focus ring is `focus-visible:ring-2` (`:6`) |
| `src/main.tsx` | Mounts synchronously today; gains the config load |
| `src/test/setup.ts` | Only a `localStorage` polyfill. No render helper, no object-URL stub — both are created in Task 10 |
| `packages/shared-ts/src/apiClient.ts:59` | `requestBlob(path, init?)` — authenticated binary GET, one-shot 401 refresh, returns a `Blob` |
| `apps/web/vite.config.ts` | Vitest config: `jsdom`, globals on, `setupFiles: ['./src/test/setup.ts']` |
| `apps/web/nginx.conf` | One `location /` with the SPA fallback. No `public/` directory exists yet |
| `apps/web/Dockerfile` | Copies `dist` → `/usr/share/nginx/html`, so `public/config/config.json` lands at `/config/config.json` |

**No `import.meta.env` usage exists anywhere in the frontend today** — Task 7
introduces the first runtime-config mechanism, and deliberately does *not*
introduce a `VITE_*` variable.

### Deployment

`deploy/k8s/base/web/` holds only `deployment.yaml` and `service.yaml`. The
deployment already runs `readOnlyRootFilesystem: true` with a `nginx-tmp`
`emptyDir` — a ConfigMap volume is a read-only mount and does not conflict.
`deploy/k8s/base/kustomization.yaml` lists resources explicitly (no globbing),
so a new file must be registered there.

`make manifests` runs `tools/check-manifests.sh`, which renders both overlays,
asserts the `main` overlay ships zero PVCs/Secrets/ClusterRoles/ClusterRoleBindings
and no `REPLACE_ME`, and checks IngressRoute route-set parity between `:80` and
`:443`. It is already part of `make ci`.

---

## 2. Decisions this task inherits

From the design (see [`design.md`](./design.md) §2 for the full reasoning):

- **D1 — cross-fleet is `404`, not `403`.** The PRD's §5.1 table and its
  acceptance criteria say `403`; the code does not do that and must not start.
  `AuthorizeAccess` maps a fleet mismatch to `server.ErrNotFound` precisely so
  cross-fleet existence is never leaked — `403` means "this id exists, just not
  for you". **The PRD is wrong here; the plan implements `404`.**
- **D2 — add `server.ErrBadRequest` (400)** rather than relaxing FR-7.3 to 422,
  and opportunistically name the 413 code while editing that switch.
- **D3 — the Carfax template is runtime config**, delivered as
  `/config/config.json`. No `VITE_CARFAX_URL_TEMPLATE`, no `import.meta.env`.
- **D4 — all three variants are accepted** (`original|thumbnail|display`), even
  though nothing requests `display` yet; the bytes are already being produced.
- **D5 — VIN format is not validated.** Any non-empty trimmed VIN gets a button.

---

## 3. Decisions made while planning

Three things the design left implicit or stated incorrectly. Each is called out
in the plan at the point it matters.

**A `VariantLookup` error propagates; it does not fall back.** The design covers
a *missing* variant (fall back to the original, log at debug) and a *missing
object* for a present variant row (fall back, log at warn) but is silent on a
lookup that errors. The plan returns it, producing a 500. Rationale: `GetByID`
read the same database successfully immediately beforehand, so a lookup error
means the database is genuinely broken; the port's contract already makes the
normal miss `found=false`, so an error means an error, and masking it would hide
a real fault behind a silently-degraded image.

**Design §3.2's stated reason for keeping `ListByMediaObject` is wrong.** It says
"the worker's replace path uses it"; the worker actually calls
`Administrator.ReplaceForMediaObject`, and `ListByMediaObject` has **no
production caller at all** (verified by grep across `apps/media-service`). The
conclusion is unchanged — keep it, because removing it would leave `Provider`
empty — but Task 2 records the real reason. Removing it is explicitly out of
scope.

**`NewProcessor` gains a parameter rather than gaining a second constructor.**
That means editing eight call sites in `processor_test.go`. They are updated to
pass `&fakeVariants{}` rather than `nil`, so a future test that does request a
variant gets a normal miss instead of a nil-pointer panic. `testRouter` keeps
its existing three-argument signature (delegating to a new
`testRouterWithVariants`), so the six existing HTTP tests need no edits at all.

---

## 4. Dependencies between tasks

```
Task 1 (ErrBadRequest) ──► Task 3 (ParseContentVariant) ──┐
Task 2 (mediavariant read) ───────────────────────────────┤
                           Task 4 (port + Content) ───────┴──► Task 5 (route + main.go)
                                                                      │
                                                                      ▼
                                                    Task 6 (variant plumbing)
                                                                      │
Task 7 (runtime config) ──► Task 8 (ConfigMap)                        │
       │                                                              │
       └──────────► Task 9 (buildCarfaxUrl) ──┐                       │
                                              │                       │
                    Task 10 (thumbnail + test helpers) ◄──────────────┘
                                              │
                                              ▼
                                     Task 11 (VehicleCard)
                                              │
                                              ▼
                                     Task 12 (verification)
```

Tasks 4 and 5 are the one pair that cannot be independently green: Task 4 changes
`NewProcessor` and `Content`, which `resource.go` calls, so the package only
compiles again once Task 5 Steps 1-2 land. The plan says so at Task 4 Step 7.

Task 7 and Tasks 1-6 are fully independent and could be done in either order.

---

## 5. Verification commands

Node is not always on `PATH`:

```sh
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
```

| Scope | Command |
| --- | --- |
| One Go package | `go test github.com/jtumidanski/myfleet/apps/media-service/internal/mediaobject/...` |
| All Go | `make vet && make test && make build` |
| One web test file | `npm run -w apps/web test -- src/lib/carfax.test.ts` |
| All web | `npm run -w apps/web lint && npm run -w apps/web test && npm run -w apps/web build` |
| Manifests | `make manifests` |
| Everything | `make ci` |

Manifests have no test suite, so the renders are the test — and rendering alone
does **not** catch namespace or cross-resource-reference errors. Both server
dry-runs are required, and the local overlay is not exempt (a missing
`namespace:` in `infra-local/kustomization.yaml` once slipped through ten reviews
because only `main` was ever dry-run):

```sh
kustomize build deploy/k8s/overlays/main  | kubectl apply --dry-run=server -f -
kustomize build deploy/k8s/overlays/local | kubectl apply --dry-run=server -f -
```

`--dry-run=server` persists nothing, so it is safe against the shared `bee`
context; it needs the `traefik.io` CRDs, which bee has.

---

## 6. Things that are easy to get wrong

- **`min-w-0` on both flex children** in `VehicleCard`. Without it, `truncate`
  does nothing and the card overflows horizontally at `grid-cols-1`. This is
  what FR-1.6 tests.
- **Do not send `Content-Length` for a variant.** `media_variants` has no size
  column; sending the original's size would truncate or hang the response.
- **Do not touch `useMediaContentUrl`'s object-URL effect.** The one-frame
  `isLoading` hold is deliberate and documented at `media.ts:104-118`; it is what
  stops the thumbnail flashing a placeholder. Only the query key and `queryFn`
  change.
- **The Carfax anchor must be a plain `<a>`, not a react-router `<Link>`** — it
  leaves the SPA — and it must carry both `target="_blank"` and
  `rel="noopener noreferrer"`.
- **Nothing may contact Carfax before a click.** No prefetch, no hover handler,
  no `<link rel="prefetch">`. Clicking discloses the VIN to a third party, so it
  must stay user-initiated.
- **The ConfigMap is a directory mount, not `subPath`.** `subPath` mounts do not
  receive ConfigMap updates without a pod restart, and these manifests use plain
  `configmap.yaml` files rather than a hash-suffixed `configMapGenerator`.
- **`?variant=` with an unknown value is a 400, never a fallback.** A silent
  fallback would ship multi-megabyte responses for a typo.
- **Matching is exact and lowercase.** `?variant=Thumbnail` is a 400.
- **`mediaKeys.content` must keep the `contents()` prefix** so prefix-based
  invalidation still matches every variant of an id.
- **`make ci` includes `make manifests`** — a broken manifest fails CI even
  though no Go or TS file changed.

---

## 7. Known follow-ups, deliberately out of scope

- The vehicle detail gallery (`MediaThumbnail`) keeps requesting **original**
  bytes. The plumbing to switch it lands in Task 6, making it a one-line change
  whenever that page is next touched — but the PRD's "no changes to the vehicle
  detail page's photo gallery" non-goal is honoured here.
- The `display` variant has no consumer. The endpoint accepts it because the
  bytes already exist.
- Grid density at `lg:grid-cols-3` with the taller card is a visual judgement
  (PRD §9.4). Task 11 Step 6 asks for an assessment against the running UI and
  an explicit report — not a silent change.
- `mediavariant.Provider.ListByMediaObject` remains with no production caller.
