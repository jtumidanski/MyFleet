# Vehicle Card Photo & Quick Actions — Product Requirements Document

Version: v1
Status: Draft
Created: 2026-07-31
---

## 1. Overview

The vehicles list page (`/vehicles`) currently renders each vehicle as a text-only
card: a title line, a year/make/model/trim subtitle, an optional status badge, and
an optional mileage line. The entire card is wrapped in a `<Link>` to the vehicle
detail page. Vehicles already support photos — a user can upload images on the
detail page and mark one as primary — but that primary photo is invisible anywhere
outside the detail page's gallery. A household with five similar-looking vehicles
scans five nearly identical text blocks.

This task makes the card visually identifiable and directly actionable. Each card
gains a left-aligned thumbnail of the vehicle's primary photo, with the existing
text block moving to its right. Whole-card navigation is replaced by an explicit
icon button, and a second icon button opens the vehicle's Carfax history report
with the VIN pre-filled — a lookup that today requires opening the detail page,
copying the VIN, and pasting it into Carfax manually.

There is one backend prerequisite. `media-service` already generates and stores a
`thumbnail` variant (max edge 320px) and a `display` variant (max edge 1280px) for
every uploaded image (`apps/media-service/internal/processing/worker.go:156-174`),
persisting them to the object store and to the `media_variants` table. But no HTTP
route serves them: `GET /media/{id}/content`
(`apps/media-service/internal/mediaobject/resource.go:148`) streams the *original*
bytes only. Rendering the vehicles grid against that route would download a
full-size original — up to the 25 MiB upload cap — for every card on every visit.
This task therefore also exposes the already-generated variants over HTTP, which is
a small, self-contained addition to an existing route.

## 2. Goals

Primary goals:

- Make each vehicle visually identifiable at a glance on the vehicles list by
  showing its primary photo.
- Give the card explicit, discoverable actions (open detail, open Carfax) instead
  of one implicit whole-card click target.
- Remove a manual copy/paste step from the "check this vehicle's history" task.
- Serve the already-generated thumbnail variants so the list view costs kilobytes
  per card rather than megabytes.

Non-goals:

- No changes to photo upload, deletion, or primary-image selection — those stay on
  the vehicle detail page and are untouched.
- No inline photo editing, carousel, or multi-photo display on the card. One
  photo (the primary) per card.
- No new persisted fields. `primaryImageMediaId` and `vin` already exist on the
  vehicle resource and are already returned by the list endpoint.
- No Carfax API integration, no purchasing, no scraping, and no display of report
  data inside MyFleet. The button is a deep link out to Carfax's own site.
- No VIN decoding, VIN validation service, or VIN format enforcement beyond what
  already exists.
- No changes to the vehicle detail page's photo gallery layout.
- No image-variant generation changes (sizes, formats, and the worker are as-is).

## 3. User Stories

- As a fleet owner with several vehicles, I want to see each vehicle's photo on the
  vehicles list so that I can identify the right one without reading the trim line.
- As a fleet member, I want an obvious button to open a vehicle's detail page so
  that I know what is clickable on the card.
- As a fleet member, I want to jump to a vehicle's Carfax report with the VIN
  already filled in so that I do not have to copy the VIN by hand.
- As a fleet member viewing a vehicle with no photo uploaded, I want the card to
  still line up with its neighbours so that the grid does not look broken.
- As a fleet member on a slow connection, I want the vehicles list to load quickly
  so that showing photos does not make the page unusable.
- As a viewer-role user, I want the same photo and navigation affordances as
  members, since neither reveals nor changes anything I am not permitted to see.

## 4. Functional Requirements

### 4.1 Card layout

- **FR-1.1** The vehicle card is laid out as a horizontal flex row: a fixed-size
  image area on the left, followed by the existing text block.
- **FR-1.2** The image area is a fixed square, 80×80 CSS px (`h-20 w-20`), with
  rounded corners and `object-cover` so non-square photos fill the box without
  distortion.
- **FR-1.3** The image area does not shrink when the text block is long
  (`shrink-0`), and the text block truncates rather than wrapping the card wider.
- **FR-1.4** The text block retains its current content and order: title
  (nickname, else `year make model`), the `year make model trim` subtitle, and the
  mileage line when `currentMileage` is a number.
- **FR-1.5** The status badge, when present, remains at the card's top-right,
  visually separated from the action buttons.
- **FR-1.6** The card layout must hold at the grid's narrowest breakpoint
  (single-column, `grid-cols-1`) without horizontal overflow.

### 4.2 Primary photo

- **FR-2.1** When `vehicle.attributes.primaryImageMediaId` is a non-empty string,
  the card renders that media object's `thumbnail` variant in the image area.
- **FR-2.2** The image is fetched through the authenticated API and held as an
  object URL — a bare `<img src>` cannot be used, because the API requires an
  `Authorization` header that the browser will not send for image subresource
  requests. This reuses the existing `useMediaContentUrl` mechanism.
- **FR-2.3** While the image is loading, the image area shows a skeleton of
  identical dimensions, so the card does not reflow when the image arrives.
- **FR-2.4** If the image fails to load (403, 404, or 5xx), the card falls back to
  the same placeholder used for "no photo" (FR-3.1) rather than surfacing an error
  state. Rationale: a broken thumbnail on a list card is not actionable by the
  user, and N failing cards must not produce N toasts or N red error tiles. The
  failure is still distinguishable to assistive technology via the accessible
  label (FR-3.3).
- **FR-2.5** Image responses are cached by React Query. Navigating away from and
  back to the vehicles page within the cache window must not refetch the bytes.

### 4.3 No-photo placeholder

- **FR-3.1** When `primaryImageMediaId` is absent, empty, or the fetch failed, the
  image area renders a neutral placeholder: a muted background with a centred car
  icon (`Car` from `lucide-react`).
- **FR-3.2** The placeholder occupies exactly the same box as a real thumbnail, so
  cards with and without photos align within a grid row.
- **FR-3.3** The placeholder is exposed to assistive technology as an image with an
  accessible label — "No photo" for the absent case, and a distinct label for the
  load-failure case ("Photo unavailable").

### 4.4 Detail navigation

- **FR-4.1** The card is no longer wrapped in a `<Link>`. The card body, the
  thumbnail, and the text are not clickable.
- **FR-4.2** Navigation to `/vehicles/{id}` is via a single icon button rendered in
  the card's action area, using the existing `Button` primitive with
  `variant="ghost"` and `size="icon"`.
- **FR-4.3** The button is a real link (`Button asChild` wrapping a react-router
  `<Link>`), not an `onClick` handler on a `<button>`. This preserves
  middle-click-to-open-in-new-tab, cmd/ctrl-click, and the browser's link context
  menu.
- **FR-4.4** The button carries an `aria-label` naming the target vehicle (for
  example, `Open details for 2019 Honda Civic`) so that a screen-reader user
  traversing a grid of cards can distinguish one card's button from another's. A
  bare "Open details" label repeated N times is not acceptable.
- **FR-4.5** The icon is decorative and marked `aria-hidden="true"`.
- **FR-4.6** The button is shown for every role, including `viewer`.

### 4.5 Carfax action

- **FR-5.1** A second icon button in the action area opens the vehicle's Carfax
  report with the VIN substituted into a URL template.
- **FR-5.2** The button is rendered **only** when `vehicle.attributes.vin` is
  present and non-empty after trimming. When there is no VIN, the button is omitted
  entirely — not rendered disabled — so it does not occupy space or attract focus.
- **FR-5.3** The URL is built from a configurable template (§5.2) by replacing the
  `{vin}` placeholder with the URL-encoded, trimmed VIN.
- **FR-5.4** The link opens in a new tab (`target="_blank"`) and must set
  `rel="noopener noreferrer"` so the opened page cannot reach back through
  `window.opener` and does not receive MyFleet as a referrer.
- **FR-5.5** The button carries an `aria-label` naming the target vehicle (for
  example, `View Carfax report for 2019 Honda Civic`), for the same reason as
  FR-4.4.
- **FR-5.6** If the configured template does not contain the `{vin}` placeholder,
  the Carfax button is not rendered. A template that silently ignores the VIN would
  send every user to the same generic page; failing closed is preferable.
- **FR-5.7** No VIN is transmitted anywhere until the user actively clicks the
  button. The card must not prefetch, ping, or otherwise contact Carfax on render.

### 4.6 Action area

- **FR-6.1** Both icon buttons sit together in a consistent position on every card,
  aligned to the card's right side.
- **FR-6.2** Button order is stable: detail navigation first, Carfax second. Cards
  without a VIN show only the detail button, in the same position it occupies on
  cards that have both.
- **FR-6.3** Buttons remain keyboard-focusable in DOM order and show the existing
  focus ring from the `Button` primitive.

### 4.7 Media variant delivery (backend)

- **FR-7.1** `GET /media/{id}/content` accepts an optional `variant` query
  parameter.
- **FR-7.2** Accepted values are `original`, `thumbnail`, and `display`. Omitting
  the parameter is equivalent to `original`, preserving the current behaviour of
  every existing caller byte-for-byte.
- **FR-7.3** Any other value is rejected with `400` and a JSON:API error body. It
  must not silently fall back to the original — a typo would otherwise ship
  multi-megabyte responses undetected.
- **FR-7.4** When a valid non-original variant is requested but no matching row
  exists in `media_variants`, the endpoint serves the **original** bytes with `200`.
  This is the normal state for a media object whose processing has not completed
  yet, and for any media that is not a processable image. Clients must not have to
  special-case it.
- **FR-7.5** Authorization is identical to the existing route: the media object is
  resolved and fleet-scoped against the caller's `ActiveFleetID` **before** any
  variant lookup or object-store read. A variant must never be reachable by a
  caller who could not read the original.
- **FR-7.6** The response sets `Content-Type` from the served bytes' own content
  type — the variant's `content_type` when a variant is served (variants are
  re-encoded: PNG stays PNG, everything else becomes JPEG), the media object's when
  the original is served.
- **FR-7.7** The response retains `Cache-Control: private, max-age=300`. These are
  per-fleet authorized bytes and must never enter a shared cache.
- **FR-7.8** `Content-Length` is set when the served bytes' size is known. The
  existing route sets it from the media object's `size`; that value describes the
  original only and must **not** be sent for a variant response.

## 5. API Surface

### 5.1 Modified endpoint

```
GET /api/media/{id}/content?variant={original|thumbnail|display}
```

| Aspect | Value |
| --- | --- |
| Auth | Bearer JWT, existing middleware; fleet-scoped by `ActiveFleetID` |
| New param | `variant` (query, optional, default `original`) |
| Success | `200` with image bytes; `Content-Type` per FR-7.6; `Cache-Control: private, max-age=300` |
| `400` | `variant` present but not one of the three accepted values |
| `403` | Media object belongs to a different fleet (unchanged) |
| `404` | Media object does not exist or is soft-deleted (unchanged) |
| Fallback | Valid variant requested, no variant row → original bytes, `200` (FR-7.4) |

Backwards compatibility: a request with no `variant` parameter behaves exactly as
today. The existing gallery and detail-page callers need no change to keep working.

### 5.2 Frontend configuration

A build-time Vite environment variable, read once into a module constant:

```
VITE_CARFAX_URL_TEMPLATE=https://www.carfax.com/VehicleHistory/p/Report.cfx?vin={vin}
```

- The value above is also the compiled-in default when the variable is unset, so no
  environment change is required for the feature to work.
- `{vin}` is the only supported placeholder and is replaced with
  `encodeURIComponent(vin.trim())`.
- The frontend currently uses **no** `import.meta.env` variables and has no runtime
  configuration mechanism, so this introduces the first one. Because Vite inlines
  env values at build time, changing the template requires rebuilding and
  redeploying the `web` image — acceptable for a URL that changes rarely, and
  called out in §9.

No new endpoints. No changes to `fleet-service`: `vin`
(`apps/fleet-service/internal/vehicle/rest.go:17`) and `primaryImageMediaId`
(`rest.go:19`) are already present on the vehicle resource returned by
`GET /fleets/{id}/vehicles`.

## 6. Data Model

No schema changes and no migrations.

Everything required already exists:

| Field / table | Location | Status |
| --- | --- | --- |
| `vehicles.primary_image_media_id` | `apps/fleet-service/internal/vehicle/entity.go:20` | Exists, exposed as `primaryImageMediaId` |
| `vehicles.vin` | fleet-service vehicle entity | Exists, exposed as `vin` |
| `media_variants` (`variant`, `object_key`, `width`, `height`, `content_type`) | `apps/media-service/internal/mediavariant/entity.go` | Exists, populated by the processing worker |

The `mediavariant.Provider` interface already exposes
`ListByMediaObject(mediaObjectID)`, which is sufficient to resolve a variant by
kind. Adding a narrower `GetByMediaObjectAndVariant` lookup is an implementation
choice for the design phase, not a data-model change.

## 7. Service Impact

### `apps/media-service` — small addition

- `internal/mediaobject/processor.go` — add a variant-aware content read
  alongside the existing `Content(ctx, id, identityFleetID)`. It must perform the
  same fleet-scoped authorization first, then resolve the variant's `object_key`
  and stream from the object store, falling back to the original per FR-7.4.
- `internal/mediaobject/resource.go` — parse and validate the `variant` query
  parameter on the existing `GET /media/{id}/content` route; return `400` on an
  unrecognised value; set headers per FR-7.6 / FR-7.8.
- `cmd/main.go` — wire a `mediavariant.Provider` into the media-object processor.
  Note this makes `mediaobject` depend on `mediavariant`; the design phase should
  confirm the dependency direction is acceptable and, per the project's code
  patterns, avoid having one package reach into another's internals.
- No changes to the processing worker, variant generation, sizes, or encodings.

### `apps/web` — the bulk of the work

- `components/features/vehicles/VehicleCard.tsx` — new layout, thumbnail, and the
  two icon buttons; the wrapping `<Link>` is removed.
- `components/features/vehicles/VehicleList.tsx` — loading skeleton height updated
  to match the taller card, so the skeleton-to-content transition does not jump.
- `components/features/vehicles/media/MediaThumbnail.tsx` — accept a `variant` prop
  so the card can request `thumbnail` while the detail gallery's behaviour is
  unchanged.
- `lib/hooks/api/media.ts` — `useMediaContentUrl` takes a variant; the
  `mediaKeys.content(id)` query key becomes variant-aware (for example
  `mediaKeys.content(id, variant)`) so a thumbnail and an original for the same
  media id do not collide in the cache.
- `services/api/MediaService.ts` — `getContentBlob(id, variant?)` appends the query
  parameter.
- A new module for the Carfax URL template constant and the `{vin}` substitution
  helper (a pure function, unit-testable without rendering).
- Possibly a small presentational component for the thumbnail-or-placeholder box,
  to keep `VehicleCard` readable.

### `apps/fleet-service` — none

### `deploy/k8s` — none required

The Carfax template ships with a compiled-in default. If the deployment is later
made to override it, that is a `web` build-arg change, not a manifest change.

## 8. Non-Functional Requirements

### Performance

- **NFR-1** Rendering the vehicles list must fetch the `thumbnail` variant, not the
  original. With the 320px max edge, a typical card image is tens of kilobytes
  against a possible 25 MiB original — the reason §4.7 exists.
- **NFR-2** Each distinct media id is fetched at most once per cache window,
  regardless of how many components render it.
- **NFR-3** Adding photos must not change the number of vehicle-list API calls. The
  list response already carries `primaryImageMediaId`; the card must not issue a
  per-vehicle metadata request to discover it.
- **NFR-4** Image loading must not block the card's text. Title, subtitle, status,
  and both buttons render and are usable while the thumbnail is still loading.

### Security & privacy

- **NFR-5** Variant bytes are subject to the same fleet-scoped authorization as
  originals (FR-7.5), and carry the same `private` cache directive (FR-7.7).
- **NFR-6** Clicking the Carfax button discloses the vehicle's VIN to a third-party
  site. This is inherent to the feature and is user-initiated, but it means the
  button must never be auto-triggered, prefetched, or followed on hover. The
  `rel="noopener noreferrer"` requirement (FR-5.4) prevents the opened tab from
  reaching `window.opener` and suppresses the MyFleet referrer.
- **NFR-7** The VIN is interpolated into a URL and must be URL-encoded. It is never
  interpolated into HTML or used to construct markup.

### Accessibility

- **NFR-8** Both icon buttons have discernible names that include the vehicle
  identity (FR-4.4, FR-5.5) — a grid of buttons all labelled "Open details" is a
  regression for screen-reader users.
- **NFR-9** Icon-only buttons meet the minimum target size afforded by the existing
  `size="icon"` variant (40×40 px).
- **NFR-10** Removing whole-card navigation is a deliberate reduction in target
  size, accepted in exchange for eliminating nested interactive elements (a link
  inside a link) that made the Carfax button's click behaviour ambiguous.
- **NFR-11** Placeholder and error states are conveyed to assistive technology by
  label, not by colour or icon alone.

### Observability

- **NFR-12** A variant request that falls back to the original (FR-7.4) is logged
  at debug level with the media id and requested variant, so a systematically
  missing variant is diagnosable without a client-visible error.

### Testing

- **NFR-13** Backend: the variant route is covered for each of — valid variant with
  a row present, valid variant with no row (fallback to original), invalid variant
  value (`400`), omitted parameter (original, unchanged), and cross-fleet access
  (`403`).
- **NFR-14** Frontend: the card is covered for — vehicle with a primary photo,
  vehicle without one, vehicle with a VIN, vehicle without one, and the correctness
  of the generated Carfax URL including encoding.
- **NFR-15** The Carfax URL builder is a pure function with direct unit tests,
  including the missing-`{vin}`-placeholder case (FR-5.6).

## 9. Open Questions

1. **Carfax template is build-time, not runtime.** Vite inlines `import.meta.env`
   at build time, so changing the template requires rebuilding the `web` image. If
   the intent was runtime configurability (an env var on the deployment, or a
   served `config.json`), that is a larger change and should be decided in the
   design phase. The PRD assumes build-time with a compiled-in default.
2. **VIN format is not validated.** The button appears for any non-empty VIN, so a
   partial or mistyped VIN produces a Carfax URL that lands on an error page.
   Should the button require a 17-character VIN? Enforcing that would hide the
   button for legitimately non-standard VINs on older or imported vehicles, which
   is why the PRD does not require it.
3. **`mediaobject` → `mediavariant` package dependency.** Wiring the variant
   provider into the media-object processor couples two internal packages. The
   design phase should confirm this respects the project's service/package boundary
   conventions, or introduce a thin read port instead.
4. **Grid density.** An 80px thumbnail plus the action row makes cards noticeably
   taller. Whether the `lg:grid-cols-3` breakpoint still reads well at that height,
   or wants a different column count, is a visual judgement best made against the
   running UI rather than specified here.
5. **`display` variant has no consumer yet.** The endpoint will accept `display`
   because the variant exists and the parameter should be complete, but nothing in
   this task requests it. Acceptable, or should the accepted set be narrowed to
   `original|thumbnail` until something needs `display`?

## 10. Acceptance Criteria

Backend:

- [ ] `GET /api/media/{id}/content` with no `variant` parameter returns exactly what
      it returns today (original bytes, same headers).
- [ ] `?variant=thumbnail` returns the stored 320px-max-edge variant bytes with the
      variant's own `Content-Type`.
- [ ] `?variant=display` returns the stored 1280px-max-edge variant bytes.
- [ ] `?variant=original` returns the original bytes.
- [ ] `?variant=bogus` returns `400` with a JSON:API error body and no image bytes.
- [ ] A valid variant request for a media object with no variant rows returns `200`
      with the original bytes, and logs the fallback at debug level.
- [ ] A variant request for a media object in another fleet returns `403`, and no
      object-store read is attempted.
- [ ] `Cache-Control: private, max-age=300` is present on every success response.
- [ ] No `Content-Length` describing the original is sent on a variant response.
- [ ] `make vet` and `make test` pass.

Frontend — card layout:

- [ ] Each vehicle card shows an 80×80 image area on the left with the text block to
      its right.
- [ ] A vehicle with a primary photo shows that photo, fetched via the `thumbnail`
      variant (verifiable as a `?variant=thumbnail` request in the network panel).
- [ ] A vehicle with no primary photo shows the car-icon placeholder in an
      identically sized box.
- [ ] A vehicle whose photo fails to load shows the placeholder, not an error tile,
      and fires no toast.
- [ ] Cards in the same grid row align regardless of which have photos.
- [ ] A skeleton of the correct size occupies the image area while loading; the card
      does not reflow when the image arrives.
- [ ] The list renders without horizontal overflow at the single-column breakpoint.

Frontend — actions:

- [ ] The card body and thumbnail are not clickable; clicking them navigates
      nowhere.
- [ ] An icon button navigates to `/vehicles/{id}`.
- [ ] That button is a real anchor: middle-click and cmd/ctrl-click open the detail
      page in a new tab.
- [ ] Its `aria-label` includes the vehicle's identity.
- [ ] A vehicle **with** a VIN shows a second icon button that opens the configured
      Carfax URL with the VIN substituted and URL-encoded.
- [ ] The Carfax link has `target="_blank"` and `rel="noopener noreferrer"`.
- [ ] A vehicle **without** a VIN shows no Carfax button, and its detail button sits
      in the same position as on a card that has both.
- [ ] No network request is made to Carfax until the button is clicked.
- [ ] Both buttons are reachable and operable by keyboard, with a visible focus
      ring.
- [ ] All of the above behave identically for a `viewer`-role user.

Frontend — configuration & tests:

- [ ] With `VITE_CARFAX_URL_TEMPLATE` unset, the compiled-in default URL is used.
- [ ] Setting `VITE_CARFAX_URL_TEMPLATE` at build time changes the generated URL.
- [ ] A template lacking `{vin}` results in no Carfax button being rendered.
- [ ] Unit tests cover the URL builder, including encoding and the missing-
      placeholder case.
- [ ] Component tests cover photo/no-photo and VIN/no-VIN card variations.
- [ ] `make fe-test` and `make fe-build` pass.

Whole branch:

- [ ] `make ci` passes.
