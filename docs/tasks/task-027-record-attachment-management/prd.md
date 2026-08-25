# Record Attachment Management — Product Requirements Document

Version: v1
Status: Draft
Created: 2026-08-25

---

## 1. Overview

Task-004 built multi-attachment support into the *create* path for maintenance and modification
records, and it works. `fleet.maintenance_record_documents` is a one-to-many table,
`maintenancerecord.MaxDocuments` is 10 (`apps/fleet-service/internal/maintenancerecord/model.go:19`),
`POST /vehicles/{id}/maintenance-records` accepts a `documentMediaIds` array and validates every ID
against media-service before anything is written, and the record drawer renders and downloads all of
them through `RecordAttachmentList`. A user filing a new record can attach up to ten files today.

What does not work is *changing* a record's attachments after it is saved. The frontend already
believes it can: `VehicleRecordDrawer.handleUpdateRecord` loops over the files picked during an edit
and calls `maintenanceRecordService.appendDocumentMedia`, which POSTs to
`/api/fleet/maintenance-records/{id}/document-media`
(`apps/web/src/services/api/MaintenanceRecordService.ts:50`). **That route does not exist in
fleet-service.** The only two references to `document-media` anywhere in the repository are the two
frontend ones; `apps/fleet-service/internal/maintenancerecord/resource.go` registers `GET`/`POST` on
the collection, `GET`/`PATCH`/`DELETE` on the item, and nothing else. So the current behavior for a
user who edits a record and adds a file is: the file uploads to media-service and is confirmed, the
attach request 404s, the drawer counts a failure and shows *"Record updated, but 1 attachment could
not be attached"*, and the confirmed media object is left referenced by nothing.

The gap is symmetrical on the other side: there is no path at any layer — no endpoint, no
administrator method, no UI control — to *remove* an attachment from a saved record. `PATCH
/maintenance-records/{id}` deliberately does not accept `documentMediaIds` (`resource.go:225-238`),
and `dbAdministrator.Update` never touches document rows; it re-reads them after updating the record
columns. A misfiled receipt is permanent.

This task makes a record's attachment set editable for its whole life: implement the attach endpoint
the frontend already calls, add a matching detach endpoint, wire a remove control into the drawer,
and make the ten-attachment cap enforceable on the edit path — which today it is not, because
`AttachmentPicker` counts only newly-picked files and knows nothing about what the record already
holds.

## 2. Goals

Primary goals:

- Implement `POST /api/fleet/maintenance-records/{id}/document-media` so the attach call the frontend
  already makes succeeds instead of 404ing.
- Add a detach path — endpoint, administrator method, and a per-attachment remove control in the
  record drawer — so an attachment can be taken off a saved record.
- Enforce `MaxDocuments` (10) against *existing plus pending* attachments on the edit path, not just
  against pending ones.
- Keep the create path's ownership guarantee on the attach path: an attachment must belong to the
  caller's active fleet, proven before anything is written.
- Apply detach to both kinds of record — `maintenance` and `modification` — since they are the same
  entity distinguished by `maintenanceCategory.kind`.

Non-goals:

- Changing `MaxDocuments` from 10.
- Reordering attachments, or giving them captions, labels or a designated "primary".
- Attachments on fuel or mileage records.
- Adding `documentMediaIds` to the `PATCH` body. The append/detach endpoints are the attachment
  mutation surface; `PATCH` stays a pure field-patch. (See §11, D2.)
- Batch attach. The endpoint takes one `mediaId`; the drawer's existing sequential loop stays.
- Changes to media-service. Detach reuses the existing `DELETE /api/media/{id}`.
- Retroactive cleanup of media already orphaned by the broken append path. Those objects are already
  reachable by the admin orphan sweep (`apps/fleet-service/internal/admin/orphans.go`,
  `targets.go:114-121`) and media-service's `purge_after` reaping.

## 3. User Stories

- As a fleet member, I want to add a receipt to a maintenance record I logged last week, so that I
  don't have to delete and re-create the record just to attach paperwork I found later.
- As a fleet member, I want to attach several documents to a modification record over time — the
  invoice now, the alignment sheet next month — so that everything about that mod lives in one place.
- As a fleet member, I want to remove an attachment I put on the wrong record, so that my history
  isn't permanently wrong because of one misclick.
- As a fleet member, I want the form to tell me I'm at the ten-attachment limit *counting what's
  already there*, so that I don't pick files that will be rejected on save.
- As a viewer, I want to see and download a record's attachments but not add or remove them, so that
  read-only access stays read-only.
- As a fleet member, I want a failed attach to tell me it failed rather than silently losing my file,
  so that I know to retry.

## 4. Functional Requirements

### 4.1 Attach (FR-ATT)

- **FR-ATT-1** — `POST /api/fleet/maintenance-records/{id}/document-media` attaches one
  already-uploaded media object to an existing, non-deleted maintenance record. It returns `201` with
  the full updated `maintenanceRecord` resource, whose `documentMediaIds` includes the new ID.
- **FR-ATT-2** — The request body is JSON:API-shaped: `{"data":{"type":"mediaRefs","attributes":
  {"mediaId":"<uuid>"}}}`. This is exactly what `MaintenanceRecordService.appendDocumentMedia`
  already sends; the client is not to be changed to fit the server.
- **FR-ATT-3** — A missing or empty `mediaId` is `422`.
- **FR-ATT-4** — The record must exist and not be soft-deleted, or the response is `404`.
- **FR-ATT-5** — The caller must be in the same fleet as the record's vehicle (`RequireSameFleet`
  against `vehicleAccessor.GetByID(record.VehicleID())`) and must have write access
  (`RequireWrite`). Same two checks, in the same order, as `PATCH` and `DELETE` on the item route.
- **FR-ATT-6** — The `mediaId` must be active and belong to the caller's active fleet, proven via
  `mediaclient.ValidateOwnership` **before any row is written**, mirroring `POST
  /vehicles/{id}/maintenance-records` (`resource.go:167-176`). A failure is `422` and leaves nothing
  to roll back. Whether the ID does not exist, was deleted, or belongs to another fleet stays
  indistinguishable.
- **FR-ATT-7** — If the record already holds `MaxDocuments` live attachments, the request is `422`
  and nothing is written. The count is of rows with `deleted_at IS NULL`.
- **FR-ATT-8** — Attaching a `mediaId` the record already holds is idempotent: it succeeds with `201`
  and does not create a second join row. The response's `documentMediaIds` contains the ID once. This
  matters because the drawer's sequential loop can be retried after a partial failure.
- **FR-ATT-9** — The cap check (FR-ATT-7) and the insert must not race into an eleventh row. The
  read-count-then-insert runs inside one transaction.

### 4.2 Detach (FR-DET)

- **FR-DET-1** — `DELETE /api/fleet/maintenance-records/{id}/document-media/{mediaId}` removes one
  attachment from a record. It returns `204`.
- **FR-DET-2** — The join row is **soft-deleted** (`deleted_at` stamped), consistent with every other
  soft-delete in fleet-service and with what `admin/visibility_document_test.go` already asserts about
  a soft-deleted document row being invisible on both the detail and list paths.
- **FR-DET-3** — Authorization is identical to FR-ATT-5: `RequireSameFleet` on the record's vehicle,
  then `RequireWrite`.
- **FR-DET-4** — A `mediaId` not currently attached to that record (never attached, or already
  detached) is `404`. A missing or soft-deleted record is `404`.
- **FR-DET-5** — After a successful detach, the media object itself is deleted, best-effort, by the
  **client**, via the existing `DELETE /api/media/{id}`. fleet-service does not call media-service to
  delete; `mediaclient` is a no-JWT internal client with a single read endpoint, and giving it delete
  authority would mean an unauthenticated internal route that can destroy media. See §11, D3.
- **FR-DET-6** — Order is reference-first, then object, and the object delete is best-effort: a
  failure there must not report the removal as failed. This is the established pattern in
  `useRemoveVehiclePhoto` (`apps/web/src/lib/hooks/api/media.ts:322-350`) and its reasoning applies
  verbatim — the reference is what the user can see, so if the object delete fails the UI is still
  correct and the orphan is media-service's to reap.

### 4.3 Cap enforcement on the edit path (FR-CAP)

- **FR-CAP-1** — `AttachmentPicker` accepts an existing-attachment count and treats the picker as full
  when `existingCount + pendingCount >= MAX_ATTACHMENTS`. It defaults to `0` so the create-flow call
  site is unaffected.
- **FR-CAP-2** — `usePendingAttachments` accepts the same count so its own `room` calculation
  (`usePendingAttachments.ts:62`) drops files that would exceed the combined cap, rather than
  uploading them and letting the server reject them.
- **FR-CAP-3** — The picker's helper text reflects remaining capacity, e.g. *"3 of 10 attached. You
  can add 7 more."* When full it states the record is at the limit rather than the generic
  *"Maximum 10 attachments per record."*
- **FR-CAP-4** — The server-side cap (FR-ATT-7) remains authoritative. FR-CAP-1..3 are ergonomics; a
  client that ignores them still gets `422`.

### 4.4 Drawer behavior (FR-UI)

- **FR-UI-1** — In view mode, each row of `RecordAttachmentList` carries a remove control for users
  with write access. Viewers see the list unchanged, with no remove control (FR-VIEW-5 from task-004
  continues to hold).
- **FR-UI-2** — Removal asks for confirmation before firing, because it destroys the underlying media
  (FR-DET-5) and is not undoable.
- **FR-UI-3** — A successful attach or detach invalidates `maintenanceRecordKeys.detail(id)`,
  `maintenanceRecordKeys.lists()` and `vehicleKeys.detail(vehicleId)`. The `lists()` invalidation is
  new relative to `useAppendMaintenanceRecordDocument`'s current `onSettled` and is required so the
  paperclip/attachment-count affordance on the records table stays truthful — task-025 renders that
  count from the list payload's `documentMediaIds`.
- **FR-UI-4** — The edit form's `AttachmentPicker` is seeded with the record's current attachment
  count (FR-CAP-1) whenever `MaintenanceRecordForm` is rendered in edit mode.
- **FR-UI-5** — A partial attach failure keeps the existing message shape (*"Record updated, but N
  attachments could not be attached"*). With FR-ATT-1 implemented this stops being the default
  outcome, but it remains the correct message when the server genuinely rejects one.
- **FR-UI-6** — An attachment that is unavailable (missing, cross-fleet, `status: failed`) still
  renders the "Attachment unavailable" row from task-004 FR-VIEW-4, and that row must still offer
  removal — an unavailable attachment is exactly the one a user most wants to clear.

## 5. API Surface

All routes are under fleet-service, registered in
`apps/fleet-service/internal/maintenancerecord/resource.go` alongside the existing item routes, and
carry the same JWT middleware.

### 5.1 `POST /api/fleet/maintenance-records/{id}/document-media`

Request:

```json
{ "data": { "type": "mediaRefs", "attributes": { "mediaId": "6f1c…" } } }
```

Response `201`:

```json
{ "data": { "type": "maintenanceRecords", "id": "…",
            "attributes": { "…": "…", "documentMediaIds": ["a…", "6f1c…"] } } }
```

| Status | Cause |
| --- | --- |
| `201` | Attached, or already attached (FR-ATT-8) |
| `401` | No/invalid JWT (middleware) |
| `403` | Different fleet, or viewer role |
| `404` | Record missing or soft-deleted |
| `422` | Empty `mediaId`; `mediaId` not active-and-owned; record already at `MaxDocuments` |
| `500` | media-service unreachable — fails closed, nothing written (design D7 from task-004) |

### 5.2 `DELETE /api/fleet/maintenance-records/{id}/document-media/{mediaId}`

No request body. Response `204`, empty.

| Status | Cause |
| --- | --- |
| `204` | Detached |
| `401` | No/invalid JWT |
| `403` | Different fleet, or viewer role |
| `404` | Record missing/soft-deleted, or `mediaId` not attached to it |

### 5.3 Unchanged

- `POST /api/fleet/vehicles/{id}/maintenance-records` — still takes `documentMediaIds` on create.
- `PATCH /api/fleet/maintenance-records/{id}` — body gains no fields (§2 non-goals).
- `GET /api/fleet/maintenance-records/{id}` and the list route — already return `documentMediaIds`.
- `DELETE /api/media/{id}` in media-service — reused as-is by the client for FR-DET-5.

### 5.4 Frontend client

`MaintenanceRecordService.appendDocumentMedia` is unchanged. One method is added:

```ts
/** DELETE /api/fleet/maintenance-records/{id}/document-media/{mediaId} */
async removeDocumentMedia(id: string, mediaId: string): Promise<void>
```

## 6. Data Model

No schema change. `fleet.maintenance_record_documents` already has everything required:

```go
type DocumentEntity struct {
    ID                  string     `gorm:"type:uuid;primaryKey"`
    MaintenanceRecordID string     `gorm:"type:uuid;not null;index"`
    MediaID             string     `gorm:"type:uuid;not null"`
    DeletedAt           *time.Time `gorm:"index"`
    PurgeOperationID    *string    `gorm:"type:uuid;index"`
}
```

`DeletedAt` exists and is already honored by the read paths — `dbAdministrator.Update` and the
provider both filter `deleted_at IS NULL`, and `admin/visibility_document_test.go` covers it. So
FR-DET-2 needs no migration.

New `Administrator` methods (`administrator.go`):

- `AttachDocument(recordID, mediaID string) (Model, error)` — in one transaction: count live rows,
  reject with `server.ErrValidation` at `MaxDocuments`, no-op if `mediaID` is already live on the
  record (FR-ATT-8), otherwise insert a `DocumentEntity` with a fresh `uuid.NewString()`. Returns the
  re-read model so the response cannot disagree with the row, for the same reason `Update` re-reads.
- `DetachDocument(recordID, mediaID string) error` — stamps `deleted_at` on the matching live row;
  `ErrNotFound` when `RowsAffected == 0`.

These are deliberately separate from `Update`, which stays a record-columns-only allow-list.

New `Processor` methods wrapping them with `ErrNotFound → server.ErrNotFound` translation, matching
`Processor.SoftDelete`.

`Model` gains no new mutator. `WithDocumentMediaIDs` already exists but is a whole-set replace used by
the create path; attach/detach operate on rows in the administrator, so the model's document slice is
only ever populated by `Make` from what was read.

## 7. Service Impact

**fleet-service** — the bulk of the work.

- `internal/maintenancerecord/resource.go`: two new routes (§5.1, §5.2), each repeating the
  `GetByID` → `vehicleAccessor.GetByID` → `RequireSameFleet` → `RequireWrite` preamble the item routes
  already use. The attach route additionally calls `docs.ValidateOwnership` before writing.
- `internal/maintenancerecord/administrator.go`: `AttachDocument`, `DetachDocument`.
- `internal/maintenancerecord/processor.go`: pass-throughs with error translation.
- No change to `mediaclient` — `ValidateOwnership` is already exactly the check FR-ATT-6 needs.

**media-service** — no change. `DELETE /media/{id}` (`internal/mediaobject/resource.go:245`) already
soft-deletes with `RequireWrite` and fleet scoping.

**apps/web**

- `services/api/MaintenanceRecordService.ts`: add `removeDocumentMedia`.
- `lib/hooks/api/maintenance.ts`: add `useRemoveMaintenanceRecordDocument`, which detaches then
  best-effort deletes the media (FR-DET-5/6); widen `useAppendMaintenanceRecordDocument`'s
  `onSettled` to include `lists()` (FR-UI-3).
- `lib/hooks/usePendingAttachments.ts`: accept an existing-attachment count (FR-CAP-2).
- `components/features/vehicles/maintenance/AttachmentPicker.tsx`: `existingCount` prop, capacity-aware
  full state and helper text (FR-CAP-1, FR-CAP-3).
- `components/features/vehicles/maintenance/MaintenanceRecordForm.tsx`: thread `existingCount` through
  to the picker (FR-UI-4).
- `components/features/vehicles/maintenance/RecordAttachmentList.tsx`: optional `onRemove` +
  `canRemove`, remove control per row including the unavailable row (FR-UI-1, FR-UI-6).
- `components/features/vehicles/detail/VehicleRecordDrawer.tsx`: pass the remove handler and the
  existing count; confirmation before removal (FR-UI-2).

**deploy/k8s** — no change. No new service, config value, or route prefix; both new routes sit under
the already-routed `/api/fleet` prefix.

## 8. Non-Functional Requirements

**Security**

- Both routes are fleet-scoped and write-gated. A viewer gets `403`, not a hidden control that fails
  late.
- FR-ATT-6 is the one that matters most: without it, `POST .../document-media` is a way to graft
  another fleet's media onto your own record and then read it back through `GET
  /api/media/{id}/content`. The ownership proof must run before the insert, not after.
- FR-DET-4 returning `404` for "not attached to this record" avoids confirming that a media ID exists
  elsewhere, consistent with the non-disclosure property `mediaclient.ValidateOwnership` documents.
- No new unauthenticated surface. In particular, no delete capability is added to media-service's
  `/internal/media` route, which the `internal-deny` Traefik rule keeps off the public internet.

**Performance**

- Attach is one cross-service call (a single-ID `ValidateOwnership`) plus one short transaction. The
  ten-row cap bounds the count query trivially.
- Detach is a single `UPDATE`. No cross-service call server-side.
- No change to the list path's fan-out; documents are still batched in one query and grouped in
  memory (provider design D21).

**Observability**

- Attach logs a warning on ownership-validation failure, matching the create path's
  `log.WithError(err).Warn("attachment ownership validation failed")`.
- The best-effort media delete on detach swallows its error by design (FR-DET-6); it must not surface
  a toast, but it should not be silent in the console either.

**Correctness**

- FR-ATT-9's transaction is what stops two concurrent attaches from both seeing nine rows.
- FR-ATT-8's idempotency is what makes the drawer's retry-after-partial-failure safe.

## 9. Open Questions

- **Should detach be offered from the list/table row, or only from the expanded drawer?** This PRD
  specifies drawer-only. Adding it to `VehicleRecordsTable` rows would need the row to carry media
  IDs, not just a count.
- **Should the confirmation in FR-UI-2 name the file?** `RecordAttachmentList` already resolves
  `originalFilename` per row, so it can — but the unavailable row (FR-UI-6) has no filename to show,
  so the copy needs a fallback.
- **Retention of the detached media.** FR-DET-5 deletes the object outright. If a "detach but keep the
  file in a vehicle's media library" concept ever lands, this decision would need revisiting; there
  is no such library for documents today, only `vehicle_media` for photos.

## 10. Acceptance Criteria

Backend:

- [ ] `POST /api/fleet/maintenance-records/{id}/document-media` exists and returns `201` with the
      updated record; the ID appears in `documentMediaIds`.
- [ ] Attaching the same `mediaId` twice yields `201` both times and exactly one live join row.
- [ ] Attaching an eleventh document returns `422` and writes nothing.
- [ ] Attaching a media object owned by another fleet returns `422` and writes nothing.
- [ ] Attach returns `403` for a viewer and for a cross-fleet caller; `404` for a soft-deleted record.
- [ ] `DELETE /api/fleet/maintenance-records/{id}/document-media/{mediaId}` returns `204`, and the ID
      is gone from `documentMediaIds` on the next `GET` — for both the detail and the list path.
- [ ] Detach of an unattached or already-detached `mediaId` returns `404`.
- [ ] Detach soft-deletes the join row; the row is still present in the table with `deleted_at` set.
- [ ] `PATCH /maintenance-records/{id}` still accepts no `documentMediaIds` and still leaves document
      rows untouched.
- [ ] Both routes work identically for a `modification`-kind record.

Frontend:

- [ ] Editing a saved record and adding two files attaches both; no "could not be attached" toast.
- [ ] The attachment count on the records table updates without a manual refresh after attach and
      after detach (FR-UI-3).
- [ ] A record with 8 attachments lets the edit form pick 2 more and no more; the helper text says so
      before the pick, not after.
- [ ] The drawer's attachment rows show a remove control for a member and none for a viewer.
- [ ] Removing an attachment asks for confirmation, then removes the row from the list.
- [ ] An "Attachment unavailable" row can still be removed.
- [ ] A failed media delete after a successful detach does not report the removal as failed.

Verification:

- [ ] `make ci` passes (lint-check, vet, test, build, fe-test, fe-build).
- [ ] New Go tests cover FR-ATT-6, FR-ATT-7, FR-ATT-8, FR-DET-2 and FR-DET-4.
- [ ] New frontend tests cover FR-CAP-1/2 and the viewer gating in FR-UI-1.

## 11. Decisions Taken

- **D1 — Single-ID attach endpoint, not batch.** `POST .../document-media` takes one `mediaId`,
  because that is the contract `MaintenanceRecordService.appendDocumentMedia` and
  `useAppendMaintenanceRecordDocument` were already written against. Building the endpoint the
  frontend already calls means zero client-contract churn and no risk of a second mismatch. The
  drawer keeps its deliberate sequential loop, whose existing comment explains why it is not
  `Promise.all`.
- **D2 — `PATCH` does not gain `documentMediaIds`.** A replace-set on `PATCH` would make attachment
  editing free, but it also makes a stale client able to silently drop attachments it never knew
  about, and it breaks the property that `PATCH`'s body is a set of independent optional field
  patches. Attach/detach stay explicit operations on an explicit sub-resource.
- **D3 — Detach deletes the media, from the client.** The user's decision is that a detached
  attachment's bytes should go, not linger for a sweep. The delete is issued by the browser against
  `DELETE /api/media/{id}` with the user's own JWT, not by fleet-service: `mediaclient` is the no-JWT
  internal client, and adding delete capability to media-service's `/internal/media` surface would
  create an unauthenticated destructive endpoint whose only protection is a Traefik rule. Client-side
  also matches `useRemoveVehiclePhoto`, which solves precisely this two-service-halves problem
  already.
- **D4 — Orphans are the sweep's problem.** Media uploaded but never successfully attached — including
  everything stranded by the currently-broken append path — is left to `admin/orphans.go` and
  media-service's `purge_after` reaping. No compensating-delete on attach failure, no backfill
  migration.
