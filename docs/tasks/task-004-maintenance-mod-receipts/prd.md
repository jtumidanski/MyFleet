# Maintenance & Modification Logging with Receipts — Product Requirements Document

Version: v1
Status: Draft
Created: 2026-07-31

---

## 1. Overview

MyFleet already stores maintenance records — `fleet.maintenance_records` carries date, mileage,
cost, vendor, notes and category, and `fleet.maintenance_record_documents` already exists to hold
media references. But the receipt loop is not closed: `MaintenanceRecordForm.tsx` always submits
`documentMediaIds: []`, there is no UI to view or download an attached document, and
`media-service` cannot physically accept a receipt that is not a JPEG or PNG. The processing
worker calls `image.Decode` on every confirmed upload (`apps/media-service/internal/processing/worker.go:206`);
a PDF fails that decode, the handler returns an error, the Kafka message redelivers forever, and
the media object is stranded in `processing` and never becomes `ready`. In practice, today a user
cannot file a receipt at all.

Separately, the platform has no concept of a **modification**. The eight seeded categories
(`apps/fleet-service/internal/maintenancecategory/entity.go:32`) are all repair/service work.
A user who installs a lift kit, a cat-back exhaust or a head unit has nowhere to record it, and no
place to keep the invoice for it. Mods matter to a household fleet for resale value, warranty
conversations and simply remembering what part went on which vehicle three years ago.

This task closes both gaps. It makes document attachment work end-to-end — pick files in the log
form, they upload, they attach, they are viewable and downloadable afterwards — and it broadens
`media-service` to accept PDFs and common office documents safely, which requires a server-side
content-type allowlist and hardened download headers that do not exist today. It then introduces
modifications as a **kind of category** rather than a new entity: `maintenance_categories` gains a
`kind` discriminator, mod categories are seeded alongside the maintenance ones, and the existing
record table, endpoints and history list serve both, filtered by kind.

> **Roadmap note.** `docs/roadmap.md` lists a future "task-007 — Maintenance system". That work
> has already shipped as part of task-001; the roadmap is stale on this point. This task extends
> the shipped maintenance system rather than building it.

## 2. Goals

Primary goals:

- Let a user attach one or more receipts/documents to a maintenance or modification record at the
  moment they log it, from the same form, without leaving the vehicle page.
- Let a user view and download the documents attached to any record they can already read.
- Make `media-service` accept non-image documents (PDF, docx, xlsx, csv) safely — meaning a
  server-enforced content-type allowlist, no variant generation for non-images, and a terminal
  status for every upload so nothing is ever stranded in `processing`.
- Introduce **modifications** as a first-class thing to log, sharing the maintenance record shape,
  distinguished by category `kind`.
- Add a short `description` to records so the history list reads as a log of what happened, not a
  list of category names.

Non-goals:

- Recurring **schedules** for modifications. `maintenance_schedules` stays maintenance-only.
- Editing a record's attachments after it is saved (add/remove files post-hoc). See §9.
- OCR, receipt parsing, or auto-extraction of cost/vendor/date from an uploaded document.
- Spend reporting, dashboard widgets, or activity-feed events for modifications.
- User-defined custom categories. `GET /maintenance-categories` stays read-only
  (`maintenancecategory/resource.go:20` is the only route) and categories stay global, not
  fleet-scoped.
- Warranty tracking, part numbers, "still installed / removed" lifecycle on mods.
- HEIC support. See §9.

## 3. User Stories

- As a fleet member, I want to log an oil change with the shop's PDF invoice attached, so that the
  proof of service lives with the record instead of in my email.
- As a fleet member, I want to photograph a paper receipt and attach it while I am logging the
  work, so that filing it is one action rather than two.
- As a fleet member, I want to record that I installed a cat-back exhaust on 2026-03-14 with a
  one-line description and the parts invoice, so that I have a history of what has been changed on
  the vehicle.
- As a fleet member, I want maintenance and modifications shown in one chronological history that I
  can filter, so that I can see everything that has been done to a vehicle in one place.
- As a fleet member, I want to open or download a receipt I filed last year, so that I can forward
  it to a buyer or a warranty claim.
- As a fleet viewer, I want to read records and their attachments but not create or delete them, so
  that read-only access stays read-only.

## 4. Functional Requirements

### 4.1 Category kinds and modification categories

- **FR-KIND-1** — `fleet.maintenance_categories` gains a `kind` column with exactly two permitted
  values: `maintenance` and `modification`. It is `NOT NULL` and defaults to `maintenance`, so the
  eight existing rows are correctly classified by the migration without a data backfill step.
- **FR-KIND-2** — The seed list gains modification categories, seeded by the same idempotent
  `FirstOrCreate`-keyed-by-name mechanism already in `maintenancecategory.Seed`. The seeded mod
  categories are: `Performance / Tune`, `Suspension`, `Wheels & Tires`, `Exhaust`, `Intake`,
  `Brake Upgrade`, `Exterior / Body`, `Interior`, `Audio & Electronics`, `Lighting`, `Towing`,
  `Other Modification`. None of these collide with an existing maintenance category name.
- **FR-KIND-3** — Re-running `Seed` must remain idempotent: seeding twice produces exactly
  `len(seeds)` rows. The existing `TestSeedIsIdempotent` must still pass with the enlarged list.
- **FR-KIND-4** — `GET /maintenance-categories` accepts an optional `kind` query parameter. Absent
  → all categories. `kind=maintenance` or `kind=modification` → only that kind. Any other value →
  `422` via the existing `server.ErrValidation`, not a silent empty list. (`422` rather than `400`
  because `shared-go/server/errors.go` has no `400` sentinel and `ErrValidation` is the
  established mapping for malformed input.)
- **FR-KIND-5** — The category JSON:API `Attributes` payload gains `kind`.

### 4.2 Record description

- **FR-REC-1** — `fleet.maintenance_records` gains a `description` column: free text, optional,
  maximum 200 characters, enforced server-side (over-length → validation error, not truncation).
- **FR-REC-2** — `description` is the primary line rendered for a record in the history list. When
  it is empty the list falls back to the category name, so existing records stay readable.
- **FR-REC-3** — `notes` is unchanged and remains the long-form field. `description` is a summary,
  not a replacement.
- **FR-REC-4** — `description` is settable on create and on `PATCH`, following the existing
  pointer-per-field partial-update pattern in `maintenancerecord/resource.go`.
- **FR-REC-5** — Required fields on create are `categoryId` and `performedAt`. `description`,
  `mileage`, `cost`, `vendor`, `notes` and documents are all optional. `performedAt` currently
  defaults to now when omitted; it becomes explicitly required — an omitted or empty
  `performedAt` is a validation error, because a maintenance log with a silently-guessed date is
  worse than one that refuses to save.

### 4.3 Record listing and filtering

- **FR-LIST-1** — `GET /vehicles/{id}/maintenance-records` accepts an optional `kind` query
  parameter with the same three-way semantics as FR-KIND-4. Filtering resolves the category IDs
  for that kind and constrains the query with `category_id IN (…)`; it must not perform a
  cross-service join.
- **FR-LIST-2** — Filtering must not break existing pagination: `meta.total` reflects the count
  **after** the kind filter is applied, not the unfiltered total.
- **FR-LIST-3** — A record resource exposes enough for the client to render a kind badge without
  an N+1 lookup per row. The client already loads the full category list via
  `useMaintenanceCategories()`, so the client resolves `categoryId → kind` locally; no new field is
  required on the record resource.

### 4.4 Document attachment on create

- **FR-DOC-1** — The log form accepts zero or more files. Each selected file is uploaded
  immediately via the existing three-step media flow (init → PUT content → confirm), and the
  resulting media IDs are submitted as `documentMediaIds` when the record is saved.
- **FR-DOC-2** — Before the record is saved, each pending attachment is listed with its filename
  and can be individually removed. Removing a pending attachment soft-deletes the uploaded media
  object so it does not linger.
- **FR-DOC-3** — Cancelling or abandoning the form soft-deletes every media object that was
  uploaded for it but never attached to a saved record. This is best-effort on the client; the
  authoritative backstop is the existing 5-day `purge_after` sweep on soft-deleted media.
- **FR-DOC-4** — A failed upload must not block the rest of the form. The failing file is reported
  by name with the reason, and the user can still save the record with the attachments that did
  succeed.
- **FR-DOC-5** — The submit action is disabled while any attachment upload is still in flight, so a
  record is never saved referencing a media object that has not been confirmed.
- **FR-DOC-6** — `documentMediaIds` submitted on create must be validated server-side: every ID
  must resolve to a media object in the caller's active fleet. An ID belonging to another fleet, or
  one that does not exist, is a validation error and the record is not created. This is a new
  check — the current `POST` handler stores the IDs unvalidated.

### 4.5 Viewing and downloading attachments

- **FR-VIEW-1** — Every record in the history list indicates how many documents it has. Expanding
  the record lists each document by original filename with a type-appropriate icon.
- **FR-VIEW-2** — Image attachments render as an inline thumbnail, reusing the existing
  `MediaThumbnail` / `useMediaContentUrl` pattern.
- **FR-VIEW-3** — Non-image attachments render as a download action. Because
  `GET /api/media/{id}/content` requires an `Authorization` header, a plain `<a href>` cannot be
  used; the download fetches the blob through the authenticated API client and triggers a save via
  an object URL, revoking it afterwards.
- **FR-VIEW-4** — A document whose media object is missing, soft-deleted, or belongs to a fleet the
  caller cannot read renders as an explicit "unavailable" row rather than a broken control or a
  silently empty list.
- **FR-VIEW-5** — Viewers (read-only role) can view and download attachments. They cannot upload,
  attach, or delete.

### 4.6 media-service: non-image support

- **FR-MEDIA-1** — `POST /media` validates the client-supplied `contentType` against a
  server-side allowlist and rejects anything outside it with `415`. This requires a new
  `ErrUnsupportedMediaType` sentinel in `packages/shared-go/server/errors.go` and its `StatusFor`
  arm — the palette currently stops at `413`/`422`. Today `InitUpload`
  (`mediaobject/processor.go:88`) accepts whatever string the client sends and
  `GET /media/{id}/content` echoes it straight back as the response `Content-Type`
  (`mediaobject/resource.go:165`). Broadening the accepted file types without this check would turn
  media download into a same-origin stored-XSS vector.
- **FR-MEDIA-2** — The allowlist is configuration-driven (a new `MEDIA_ALLOWED_CONTENT_TYPES` key
  in `deploy/k8s/base/media-service/configmap.yaml`, with the default in `cmd/main.go`), and its
  default value is:

  | Class | Content types |
  |---|---|
  | Renderable image | `image/jpeg`, `image/png` |
  | Document | `application/pdf`, `application/vnd.openxmlformats-officedocument.wordprocessingml.document`, `application/vnd.openxmlformats-officedocument.spreadsheetml.sheet`, `text/csv` |

- **FR-MEDIA-3** — Content types are classified into **renderable images** (the two the worker's
  `image.Decode` can actually handle — `image/jpeg` and `image/png`) and **documents**
  (everything else on the allowlist). Only renderable images enter the variant-generation path.
- **FR-MEDIA-4** — Confirming a **document** upload transitions it `uploaded → ready` directly and
  does **not** publish a `media.uploaded` processing event. No thumbnail or display variant is
  produced, and `GET /media/{id}` reports `ready` with an empty variant set.
- **FR-MEDIA-5** — Every confirmed upload must reach a terminal state. A renderable image whose
  bytes fail to decode (corrupt or mislabelled file) must not redeliver indefinitely: after the
  existing consumer retry budget is exhausted the object is moved to a terminal failure state and
  the event is recorded as processed. The exact mechanism — a new `failed` status versus `ready`
  with zero variants — is a design decision (§9), but the invariant is non-negotiable: no object
  remains in `processing` forever, and no Kafka partition is blocked by one bad file.
- **FR-MEDIA-6** — The existing 25 MiB cap (`MEDIA_MAX_UPLOAD_BYTES = 26214400`) applies unchanged
  to documents. The client-side mirror constant in `apps/web/src/lib/hooks/api/media.ts` stays in
  sync.

### 4.7 media-service: download hardening

- **FR-DL-1** — `GET /media/{id}/content` sets `X-Content-Type-Options: nosniff` on every response.
- **FR-DL-2** — Renderable images are served `Content-Disposition: inline`. Documents are served
  `Content-Disposition: attachment`, so a PDF or CSV is never rendered in the origin's security
  context.
- **FR-DL-3** — The `Content-Disposition` header carries the original filename, escaped safely:
  quotes and control characters stripped or escaped, and a `filename*=UTF-8''…` form used for
  non-ASCII names per RFC 5987. A filename must never be able to inject a header.
- **FR-DL-4** — The `Content-Type` written to the response is the **stored** type, and because of
  FR-MEDIA-1 that value is now guaranteed to be on the allowlist. Existing rows created before this
  change may hold arbitrary strings; any stored type not on the allowlist is served as
  `application/octet-stream` with `Content-Disposition: attachment`.
- **FR-DL-5** — The existing `Cache-Control: private, max-age=300` is retained.

## 5. API Surface

Detailed request/response shapes are in `api-contracts.md`. Summary of changes:

| Method | Path | Change |
|---|---|---|
| `GET` | `/api/fleet/maintenance-categories` | New optional `?kind=maintenance\|modification`. Response attributes gain `kind`. |
| `GET` | `/api/fleet/vehicles/{id}/maintenance-records` | New optional `?kind=…`. Response attributes gain `description`. |
| `POST` | `/api/fleet/vehicles/{id}/maintenance-records` | Accepts `description`. `performedAt` now required. `documentMediaIds` now validated against the caller's fleet. |
| `GET` | `/api/fleet/maintenance-records/{id}` | Response attributes gain `description`. |
| `PATCH` | `/api/fleet/maintenance-records/{id}` | Accepts `description`. Attachments remain immutable (§9). |
| `POST` | `/api/media` | Rejects content types outside the allowlist with `415`. |
| `POST` | `/api/media/{id}/confirm` | Documents go straight to `ready`; images unchanged. |
| `GET` | `/api/media/{id}/content` | Adds `X-Content-Type-Options` and `Content-Disposition`. |

Error cases:

| Condition | Status | Notes |
|---|---|---|
| `kind` query value not `maintenance`/`modification` | `422` | Not a silent empty result. |
| `performedAt` missing or unparseable | `422` | Was previously defaulted to now. |
| `description` over 200 characters | `422` | Not truncated. |
| `documentMediaIds` contains an ID outside the caller's fleet | `422` | Record not created. Deliberately not `403` — it must not confirm the ID exists elsewhere. |
| Upload content type not on the allowlist | `415` | Message names the accepted types. |
| Upload exceeds 25 MiB | `413` | Existing behaviour, unchanged. |
| Caller lacks write role | `403` | Existing `authz.RequireWrite`. |
| Caller outside the vehicle's fleet | `403`/`404` | Existing `authz.RequireSameFleet`. |

## 6. Data Model

### 6.1 `fleet.maintenance_categories` (modified)

| Column | Type | Notes |
|---|---|---|
| `kind` | `varchar` `NOT NULL DEFAULT 'maintenance'` | **New.** `maintenance` \| `modification`. |

Migration is a GORM `AutoMigrate` column addition. The `DEFAULT` classifies the eight existing
rows correctly with no backfill. Seeding then inserts the twelve modification categories from
FR-KIND-2. Because seeding is `FirstOrCreate` keyed by `Name`, existing rows are untouched and
re-running is safe.

### 6.2 `fleet.maintenance_records` (modified)

| Column | Type | Notes |
|---|---|---|
| `description` | `varchar(200)` nullable | **New.** Short summary. |

Existing rows get `NULL`/empty and fall back to the category name in the UI (FR-REC-2).

### 6.3 `fleet.maintenance_record_documents` (unchanged)

Already exists with `id`, `maintenance_record_id`, `media_id`. No schema change — this task makes
it reachable rather than altering it.

### 6.4 media-service (no schema change)

`media_objects.status` is a string column, so introducing a terminal failure state (if that is the
route chosen for FR-MEDIA-5) requires no migration — only the `Status` constant set and the
transition guards in `mediaobject.MarkReady` / the processing worker.

## 7. Service Impact

**`apps/fleet-service`**

- `internal/maintenancecategory` — `kind` on `Entity`, `Model`, and `Attributes`; twelve new seed
  rows; `kind` filter on the provider and the `GET` route.
- `internal/maintenancerecord` — `description` on `Entity`, `Model`, `Builder`, `Attributes`;
  `WithDescription`; required-`performedAt` and length validation; `kind` filter on
  `ListByVehicle` with a correct filtered `total`; fleet-scoped validation of `documentMediaIds` on
  create. The kind filter needs category-ID resolution — inject a small `CategoryAccessor`
  interface mirroring the existing `VehicleAccessor` pattern in `resource.go` rather than reaching
  into the other package's provider.
- Validating `documentMediaIds` requires fleet-service to ask media-service whether an ID belongs
  to the active fleet. `notification-service` already has an `internal/fleetclient` precedent for
  a service-to-service client; the equivalent here is a small media client. This is the main new
  cross-service seam in the task.

**`apps/media-service`**

- `internal/mediaobject` — allowlist validation in `InitUpload`; image/document classification;
  `Confirm` short-circuits documents to `ready`; download handler gains `Content-Disposition`,
  `nosniff`, and the octet-stream fallback for legacy rows.
- `internal/processing/worker.go` — must never be handed a document; the terminal-state guarantee
  for undecodable images (FR-MEDIA-5).
- `cmd/main.go` — `MEDIA_ALLOWED_CONTENT_TYPES` config key.

**`apps/web`**

- `lib/hooks/api/media.ts` — a fleet/vehicle-agnostic upload hook. The current
  `useUploadMedia(vehicleId)` hard-codes invalidation of the vehicle media gallery, which is wrong
  for a receipt; extract the shared upload orchestration and let callers supply invalidation.
- `lib/schemas/maintenanceRecord.ts` — `description`, required `performedAt`.
- `components/features/vehicles/maintenance/MaintenanceRecordForm.tsx` — description field, receipt
  picker, pending-attachment list with per-item removal, submit gating.
- New `RecordAttachmentList` component for viewing/downloading attachments (FR-VIEW-1..4).
- `VehicleMaintenanceSection.tsx` — kind-grouped category picker, kind filter on the history list,
  kind badges, a "Log Modification" entry point, and passing `description`/`documentMediaIds`
  through (it currently hard-codes `documentMediaIds: values.documentMediaIds ?? []` against a form
  that never populates it).

**`packages/shared-go`** — `server/errors.go` gains `ErrUnsupportedMediaType` (415) and the
matching `StatusFor` arm. This is the only shared-Go change; both services consume it.

**`packages/shared-ts`** — download-blob helper if one does not already exist.

**`deploy/k8s`** — `MEDIA_ALLOWED_CONTENT_TYPES` in the media-service ConfigMap; both overlays must
still render clean per CLAUDE.md.

## 8. Non-Functional Requirements

**Security**

- The content-type allowlist is a security control, not a UX affordance. It is enforced
  server-side; the client `accept` attribute is a convenience only.
- `Content-Disposition: attachment` plus `nosniff` on documents is what prevents an uploaded file
  from executing in the application's origin. Both are required; neither alone is sufficient.
- Filename escaping in `Content-Disposition` must be treated as untrusted input (FR-DL-3).
- Fleet scoping on attachments must be enforced on **write** as well as read (FR-DOC-6) — a record
  must not be able to hold a reference to another fleet's media.

**Performance**

- Rendering a history page of 25 records with attachments must not issue an unbounded number of
  requests. Attachment metadata is fetched only for the expanded record, and thumbnail/content
  fetches are keyed and cached by the existing `mediaKeys` factory.
- Document uploads skip variant generation entirely, so a PDF's confirm→ready latency is a single
  synchronous transition rather than a Kafka round trip.

**Observability**

- Rejected uploads log the offending content type (not the file bytes) at `warn`, with the media ID
  and fleet ID, so an over-narrow allowlist is diagnosable from logs.
- A terminal processing failure logs at `error` with the media ID and the decode error.

**Compatibility**

- All schema changes are additive with defaults. Existing records, existing media objects and the
  existing client continue to work unchanged if deployed out of order.
- Media rows created before the allowlist existed may hold arbitrary content types; FR-DL-4 defines
  their behaviour rather than leaving it to chance.

## 9. Open Questions

1. **Post-hoc attachment editing.** The chosen entry flow is form-first attach-inline, and adding
   or removing files on an already-saved record is explicitly out of scope. This leaves a real
   product hole: attach the wrong receipt and the only recourse is deleting the record and
   re-creating it. Flagging it rather than silently expanding scope — worth confirming this is
   acceptable for v1, or promoting `PATCH documentMediaIds` into this task.
2. **FR-MEDIA-5 mechanism.** A new `failed` status is cleaner and surfaces the problem to the user,
   but adds a state the web client must handle everywhere media status is read. Reusing `ready`
   with zero variants requires no client change but silently presents a broken image. To be settled
   in `/design-task`.
3. **HEIC.** iPhones shoot HEIC by default. It is excluded from the allowlist, which means an HEIC
   upload now fails fast with a clear `415` instead of today's silent hang — strictly better, but
   still a rejection. Most mobile browsers transcode to JPEG on file-picker upload, so this may
   never be hit in practice. Whether to add HEIC transcoding is deferred.
4. **Office document sizes.** 25 MiB is generous for a receipt but the cap is shared with vehicle
   photos. No change proposed; noting that a separate per-class cap is possible if it becomes an
   issue.
5. **Modification category list.** The twelve seeded mod categories are a first guess. They are
   system-defined and read-only, so getting them wrong means a follow-up migration rather than a
   user workaround.

## 10. Acceptance Criteria

**Categories and kinds**

- [ ] `maintenance_categories.kind` exists, is `NOT NULL`, and the eight pre-existing rows read as
      `maintenance` after migration without a manual backfill.
- [ ] The twelve modification categories are seeded and `TestSeedIsIdempotent` passes with the
      enlarged seed list.
- [ ] `GET /maintenance-categories?kind=modification` returns only mod categories;
      `?kind=maintenance` only maintenance ones; no parameter returns all; `?kind=bogus` returns
      `422`.

**Records**

- [ ] A record can be created with a `description`; it round-trips through `POST`, `GET` and
      `PATCH`.
- [ ] A `description` over 200 characters is rejected with `422` and is not truncated.
- [ ] `POST` without `performedAt` returns `422` rather than defaulting to now.
- [ ] `GET /vehicles/{id}/maintenance-records?kind=modification` returns only mod records and
      `meta.total` equals the filtered count, verified with more records than one page.
- [ ] `POST` with a `documentMediaIds` entry belonging to another fleet returns `422` and creates
      no record.

**media-service**

- [ ] `POST /media` with `application/pdf` succeeds; with `text/html` returns `415`.
- [ ] Confirming a PDF upload results in `status: ready` with no variants, and publishes no
      processing event.
- [ ] Confirming a JPEG upload still produces thumbnail and display variants and reaches `ready` —
      the existing `processing` package tests still pass.
- [ ] A confirmed upload whose bytes cannot be decoded reaches a terminal state and its event is
      recorded as processed; it does not redeliver indefinitely. Verified by a worker test with
      undecodable bytes.
- [ ] `GET /media/{id}/content` for a PDF returns `Content-Disposition: attachment` with the
      escaped original filename and `X-Content-Type-Options: nosniff`.
- [ ] `GET /media/{id}/content` for a JPEG returns `Content-Disposition: inline` and still renders
      in the existing vehicle gallery.
- [ ] A legacy media row with a content type outside the allowlist is served as
      `application/octet-stream` with `attachment`.
- [ ] A filename containing quotes, newlines or non-ASCII characters cannot corrupt or inject the
      `Content-Disposition` header. Covered by a unit test.

**Web**

- [ ] The log form has a description field and a receipt picker; selecting files uploads them and
      lists them by name before save.
- [ ] Removing a pending attachment before save removes it from the list and soft-deletes the media
      object.
- [ ] Cancelling the form soft-deletes attachments uploaded but never attached.
- [ ] Submit is disabled while an attachment upload is in flight.
- [ ] A failed attachment upload is reported by filename and does not prevent saving the record
      with the successful ones.
- [ ] A saved record shows its attachment count; expanding it shows image attachments as
      thumbnails and documents as working download actions.
- [ ] Downloading a document from the UI produces the original file with its original filename.
- [ ] An attachment whose media object is unavailable renders an explicit unavailable state.
- [ ] A modification can be logged, appears with a modification badge, and the history list can be
      filtered to maintenance-only or modification-only.
- [ ] A viewer-role user sees records and can download attachments but sees no log/upload/delete
      controls.

**Project gates**

- [ ] `make ci` passes (lint-check, vet, test, build, fe-test, fe-build).
- [ ] `kustomize build deploy/k8s/overlays/local` and `…/overlays/main` both render; the `main`
      overlay still has no PVCs, no Secrets, no ClusterRole and no placeholder values.
- [ ] Both server dry-runs pass against a reachable cluster.
- [ ] Code review run per CLAUDE.md before the PR — `plan-adherence-reviewer`,
      `backend-guidelines-reviewer` and `frontend-guidelines-reviewer`.
