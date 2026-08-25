# Record Attachment Management — Design

Task: task-027-record-attachment-management
PRD: `docs/tasks/task-027-record-attachment-management/prd.md` (v1, approved)
Status: Draft
Created: 2026-08-25

---

## 1. Scope of this document

The PRD already fixed the externally-visible contract: two new fleet-service routes, a
client-issued media delete on detach, no schema migration, no `PATCH` change. This document
settles the parts the PRD left to design — how the attach transaction is made correct under
concurrency, where idempotency is enforced, how the cap becomes visible to the edit form without
duplicating the source of truth, and how the drawer's remove control behaves — and records the
alternatives rejected along the way.

Nothing here contradicts the PRD. Where this document is more specific than the PRD (D-2, D-3),
it is choosing among implementations the PRD deliberately did not name.

## 2. Architecture at a glance

```
VehicleRecordDrawer
   │  view mode                                  edit mode
   │    └─ RecordAttachmentList                    └─ MaintenanceRecordForm
   │         onRemove ──┐                               └─ AttachmentPicker(existingCount)
   │                    │                                     └─ usePendingAttachments(existingCount)
   │                    ▼                                            │ upload (media-service)
   │        useRemoveMaintenanceRecordDocument                       ▼ commit() → mediaIds
   │             1. DELETE /api/fleet/maintenance-records/{id}/document-media/{mediaId}
   │             2. DELETE /api/media/{mediaId}          (best-effort)
   │                                                     useAppendMaintenanceRecordDocument
   ▼                                                     POST …/{id}/document-media  (per id)

fleet-service  maintenancerecord
   resource.go     POST   /maintenance-records/{id}/document-media
                   DELETE /maintenance-records/{id}/document-media/{mediaId}
        │  GetByID → vehicleAccessor.GetByID → RequireSameFleet → RequireWrite
        │  (attach only) docs.ValidateOwnership(ctx, activeFleetID, []string{mediaId})
        ▼
   processor.go    AttachDocument / DetachDocument   — ErrNotFound → server.ErrNotFound
        ▼
   administrator.go AttachDocument / DetachDocument  — one transaction, cap + idempotency
        ▼
   fleet.maintenance_record_documents  (+ new partial unique index)
```

The three-layer split (resource → processor → administrator) is the existing shape of this
package; attachment mutation gets its own administrator methods rather than being folded into
`Update`, because `Update`'s allow-list-map contract is exactly what keeps `PATCH` from touching
documents (PRD D2).

## 3. Backend design

### 3.1 Route placement and the authorization preamble

Both routes are registered inside the existing `InitializeRoutes` closure in
`apps/fleet-service/internal/maintenancerecord/resource.go`, next to the `PATCH` and `DELETE`
item routes, and inherit the same JWT middleware from the router that mounts this closure.

Both repeat the item-route preamble verbatim, in this order:

1. `proc.GetByID(id)` — resolves the record; a soft-deleted or missing record surfaces as
   `server.ErrNotFound` → `404` (FR-ATT-4, FR-DET-4).
2. `vehicleAccessor.GetByID(m.VehicleID())` — resolves the owning fleet.
3. `authz.RequireSameFleet(identity, v.FleetID())` → `403`.
4. `authz.RequireWrite(identity)` → `403`.

Order matters and is not incidental: resolving the record before the authz check is what the
existing `PATCH`/`DELETE` handlers do, and it means a cross-fleet caller with a valid record ID
gets `403` rather than `404`. That asymmetry is already this service's established behavior; this
task does not relitigate it.

The attach handler then adds, **before any write** (FR-ATT-6):

```go
if docs != nil {
    if err := docs.ValidateOwnership(req.Context(), identity.ActiveFleetID, []string{attrs.MediaID}); err != nil {
        log.WithError(err).Warn("attachment ownership validation failed")
        server.WriteError(w, err)
        return
    }
}
```

This is byte-for-byte the create path's check (`resource.go:167-176`) with a one-element slice.
The `docs != nil` guard is kept because the `DocumentValidator` interface documents nil as legal
for unit tests. Empty `mediaId` is rejected with `server.ErrValidation` before the call, so a
blank ID never reaches media-service as `?ids=` (FR-ATT-3).

The attach handler uses `server.RegisterInputHandler` with `struct { MediaID string
\`json:"mediaId"\` }`, which is what unwraps the `{"data":{"type":"mediaRefs","attributes":…}}`
envelope the client already sends. The declared `type` is not validated — no existing handler in
this codebase validates the JSON:API type on input, and inventing that check here would be a
one-off.

Detach reads `mediaId` from `chi.URLParam(req, "mediaId")` and writes `204` with no body.

### 3.2 Administrator: `AttachDocument`

```go
AttachDocument(recordID, mediaID string) (Model, error)
DetachDocument(recordID, mediaID string) error
```

`AttachDocument` runs entirely inside `a.db.Transaction(...)`:

1. **Lock the parent record row.** `SELECT … FROM fleet.maintenance_records WHERE id = ? AND
   deleted_at IS NULL FOR UPDATE`, via `tx.Clauses(clause.Locking{Strength: "UPDATE"})`. This is
   the serialization point for FR-ATT-9 (§3.3 explains why the count alone is not enough). Not
   found → `ErrNotFound`.
2. **Idempotency check.** Count live rows matching `(recordID, mediaID)`. If one exists, skip the
   insert and fall through to step 5 — `201` with the ID present exactly once (FR-ATT-8).
3. **Cap check.** Count live rows for `recordID`. `>= MaxDocuments` → `server.ErrValidation`,
   which rolls the transaction back having written nothing (FR-ATT-7). The count is deliberately
   taken *after* the idempotency check, so re-attaching an ID already present on a full record
   succeeds rather than failing at the cap.
4. **Insert** a `DocumentEntity{ID: uuid.NewString(), MaintenanceRecordID: recordID, MediaID:
   mediaID}`.
5. **Re-read** the record row and its live document rows inside the same transaction and return
   `Make(stored, docs)`.

Step 5 mirrors the reasoning already written into `dbAdministrator.Update`: returning a model
built from the in-memory entity is how a response comes to disagree with the row. Re-reading
means the `documentMediaIds` in the `201` body is what the next `GET` will return.

`DetachDocument` is a single statement:

```go
res := a.db.Model(&DocumentEntity{}).
    Where("maintenance_record_id = ? AND media_id = ? AND deleted_at IS NULL", recordID, mediaID).
    Update("deleted_at", time.Now().UTC())
```

`RowsAffected == 0` → `ErrNotFound`, which covers both "never attached" and "already detached"
with one status (FR-DET-4). No transaction is needed: one row, one statement, idempotent under
retry in the sense that matters — a second detach returns `404` and changes nothing.

Note that detach does **not** verify the record itself exists; the resource layer's `GetByID`
already did that, and duplicating it in the administrator would be a second round-trip for an
invariant the handler holds.

### 3.3 Concurrency: why the row lock, and why not something cheaper

Read-then-insert inside one transaction is *not* sufficient at Postgres's default READ COMMITTED
isolation. Two concurrent attaches on a record holding nine documents each count nine — neither
sees the other's uncommitted insert — both insert, both commit, and the record ends with eleven.
Wrapping the two statements in `BEGIN`/`COMMIT` changes nothing about that; a transaction is not
a lock.

`SELECT … FOR UPDATE` on the parent `maintenance_records` row makes attaches to the same record
strictly serial: the second transaction blocks on the lock until the first commits, then counts
ten and gets its `422`. The lock is on the record, not the document table, so it also cleanly
excludes a concurrent record soft-delete. Contention is nil in practice — the only real
concurrent-attach case is the drawer's own sequential loop, which is sequential.

**sqlite in tests.** This package's tests open `sqlite::memory:` (`provider_test.go:14`), and
sqlite rejects `FOR UPDATE` as a syntax error. The lock is therefore applied dialect-guarded:

```go
q := tx.Where("id = ? AND deleted_at IS NULL", recordID)
if tx.Name() != "sqlite" {
    q = q.Clauses(clause.Locking{Strength: "UPDATE"})
}
```

This is not a new pattern — `mediavariant.ApplyPartialIndexes`
(`apps/media-service/internal/mediavariant/entity.go:61-68`) already branches on `db.Name() ==
"sqlite"` for exactly this class of dialect gap. Skipping the lock under sqlite is safe because
sqlite serializes writers at the database level anyway; the correctness the lock buys exists
under sqlite for free.

Alternatives considered and rejected:

- **`SERIALIZABLE` isolation for the transaction.** Correct, but it makes the caller responsible
  for retrying serialization failures, and this service has no retry infrastructure. A `40001`
  would surface to the user as a `500`.
- **A `CHECK` constraint or trigger counting rows.** Not expressible as a simple constraint;
  a trigger puts business rules in the database, which nothing else in this codebase does.
- **Accepting the race.** The cap is 10 and the consequence of 11 is cosmetic. But FR-ATT-9 is an
  explicit acceptance criterion, the fix is six lines, and "cosmetic" stops being true if a client
  ever fires the loop with `Promise.all`.

### 3.4 Idempotency: application check plus a partial unique index

Step 2 of `AttachDocument` handles the ordinary case. It is a read-then-write, so it has the same
theoretical race as the cap — two simultaneous attaches of the *same* ID could both miss. The
parent-row lock from §3.3 closes that too, since both go through it.

A partial unique index is added anyway as the durable invariant:

```sql
CREATE UNIQUE INDEX IF NOT EXISTS ux_maintenance_record_documents_record_media
  ON fleet.maintenance_record_documents (maintenance_record_id, media_id)
  WHERE deleted_at IS NULL;
```

It must be *partial*, not a plain `uniqueIndex` struct tag, for the reason
`mediavariant.ApplyPartialIndexes` spells out: a soft-deleted row would otherwise permanently
occupy the slot, and detach-then-reattach — an obvious user action, and the exact flow FR-DET plus
FR-ATT create — would fail forever. GORM's `uniqueIndex` tag cannot express a `WHERE` clause, so
this goes in an `ApplyPartialIndexes(db)` function called from `maintenancerecord.Migration`,
with the same sqlite spelling fallback the mediavariant version uses.

The PRD says "no schema change" and means no *column* change. An index is additive, `IF NOT
EXISTS`, and creates no migration ordering problem — but existing data could violate it, since the
currently-broken append path never wrote duplicates but the create path could in principle be
handed the same ID twice in `documentMediaIds`. **The index creation must therefore be preceded by
a de-duplication statement** that soft-deletes all but the lowest-`id` live row per
`(maintenance_record_id, media_id)` group; otherwise `Migration` fails on startup for any fleet
that has such a row and takes the service down. This is a one-time idempotent `UPDATE` in the
same function.

### 3.5 Processor

Thin pass-throughs, matching `Processor.SoftDelete`:

```go
func (pr *Processor) AttachDocument(recordID, mediaID string) (Model, error)
func (pr *Processor) DetachDocument(recordID, mediaID string) error
```

Each translates `ErrNotFound` → `server.ErrNotFound` and returns everything else unchanged;
`server.ErrValidation` from the cap check passes through untouched and `StatusFor` maps it to
`422`. No validation lives here — the model is not being mutated, so `Validate` has nothing to
say about an attach.

`Model` gains no mutator. `WithDocumentMediaIDs` stays what it is: a whole-set replace used by the
create path's builder. Attach and detach operate on rows, and the model's document slice is
populated only by `Make` from what was read — which is what keeps the response honest.

## 4. Frontend design

### 4.1 Threading the existing count

`AttachmentPicker` gains `existingCount?: number` defaulting to `0`, and
`usePendingAttachments(existingCount = 0)` uses it in its `room` computation:

```ts
const room = Math.max(MAX_ATTACHMENTS - existingCount - itemsRef.current.length, 0);
```

`MaintenanceRecordForm` passes `record?.attributes.documentMediaIds?.length ?? 0` down to both.
The create call site passes nothing and is unchanged (FR-CAP-1, FR-CAP-2).

The count is a **prop threaded from the record**, not read from a query inside the picker. The
picker is a presentational component in a form; giving it a `useMaintenanceRecord` call would
couple a form control to a server cache and make it untestable without a QueryClient. The drawer
already holds the record.

Helper text becomes capacity-aware (FR-CAP-3):

- `existingCount > 0` and room remains → *"3 of 10 attached. You can add 7 more."*
- no room → *"This record is at the 10-attachment limit."*
- `existingCount === 0` → the current create-flow text, verbatim.

`existingCount` is a snapshot taken when the form renders. If an attach lands from another tab
mid-edit, the client's arithmetic is stale — which is precisely why FR-CAP-4 keeps the server
authoritative. The client cap is ergonomics; a stale client gets a `422` and the existing partial
failure message (FR-UI-5).

### 4.2 The remove control

`RecordAttachmentList` gains two optional props:

```ts
{ mediaIds: string[]; onRemove?: (mediaId: string) => void; canRemove?: boolean }
```

`AttachmentRow` renders a trash-icon `Button` (ghost, `aria-label={`Remove ${filename}`}`) when
`canRemove && onRemove`. The drawer passes `canRemove={canWrite}`, so viewers see the list exactly
as they do today (FR-UI-1, and task-004's FR-VIEW-5 continues to hold).

The unavailable row gets the same control (FR-UI-6), with `aria-label="Remove attachment"` since
there is no filename to name. That row currently returns early before any control is rendered, so
this is a real change to its shape, not a prop pass-through.

The image row and the document row differ structurally — one is a `div` with a thumbnail, the
other is a full-width `Button` — so the remove control is added to each rather than hoisted. Making
the download row a `div` containing two buttons would be the cleaner structure, but it changes the
download affordance's hit area for a task that is not about downloads.

### 4.3 Removal flow

`VehicleRecordDrawer` owns the confirmation and the mutation. Confirmation uses the same
`AlertDialog` pattern the drawer already uses for record deletion, holding the pending
`mediaId` in state; the copy names the file when the row resolved one and says "this attachment"
otherwise (PRD §9, resolved in favor of naming-when-known).

Because the drawer does not have the filename — `AttachmentRow` resolves it via `useMediaObject` —
`onRemove` is invoked with the `mediaId` only, and the dialog copy uses a generic phrasing. Passing
the filename up through the callback would work but adds a second parameter used for one sentence;
the simpler contract wins, and the confirmation still names the record.

`useRemoveMaintenanceRecordDocument(vehicleId)` in `lib/hooks/api/maintenance.ts`:

```ts
mutationFn: async ({ id, mediaId }) => {
  await maintenanceRecordService.removeDocumentMedia(id, mediaId);   // reference first
  await mediaService.remove(mediaId).catch(() => undefined);         // object, best-effort
}
```

Reference-first-then-object, with the object delete swallowing its error, is `useRemoveVehiclePhoto`
(`lib/hooks/api/media.ts:322-350`) exactly (FR-DET-5, FR-DET-6). The reference is what the user can
see; if the object delete fails the UI is already correct and the orphan is media-service's
`purge_after` sweep to reap. The swallowed error is logged to the console rather than being
silently discarded — an observability requirement from PRD §8, not a toast.

`onSettled` invalidates `maintenanceRecordKeys.detail(id)`, `maintenanceRecordKeys.lists()` and
`vehicleKeys.detail(vehicleId)`. The same three-key set is added to
`useAppendMaintenanceRecordDocument`, whose current `onSettled` omits `lists()` — that omission is
what would leave task-025's attachment-count column stale after an edit-time attach (FR-UI-3).

`MaintenanceRecordService.removeDocumentMedia(id, mediaId): Promise<void>` issues the `DELETE`
through `apiClient.request` and discards the empty body, alongside the existing
`appendDocumentMedia`, which is untouched.

## 5. Error handling

| Situation | Server | Client |
| --- | --- | --- |
| Empty `mediaId` on attach | `422` before any call | not reachable from the UI |
| Media not active / other fleet | `422`, nothing written | counted as an attach failure, existing toast |
| Record at cap | `422`, nothing written | same; picker normally prevents it |
| Record missing / soft-deleted | `404` | error toast |
| Viewer or cross-fleet | `403` | control not rendered; toast if forced |
| media-service unreachable on attach | `500`, fails closed | attach failure toast |
| Detach of unattached ID | `404` | error toast; row stays |
| Media delete fails after detach | n/a | **success** — row is gone, console warning only |

The one deliberate asymmetry is the last row: a partial failure on removal reports success,
because the half that failed is invisible to the user and self-healing.

## 6. Testing

**Go** (`maintenancerecord` package, sqlite in-memory, existing `newTestDB` helper):

- `administrator_test.go` — attach inserts one row; attaching the same ID twice leaves exactly one
  live row and returns it once (FR-ATT-8); the eleventh attach returns a validation error and
  leaves ten rows (FR-ATT-7); detach stamps `deleted_at` and leaves the row present (FR-DET-2);
  detach of an unattached ID returns `ErrNotFound` (FR-DET-4); detach-then-reattach succeeds,
  which is the regression test for the partial index being partial.
- `resource_test.go` — attach returns `201` with the ID in `documentMediaIds`; a stub
  `DocumentValidator` returning an error yields `422` and writes nothing (FR-ATT-6); `403` for a
  viewer and for a cross-fleet caller; `404` for a soft-deleted record; detach returns `204` and
  the ID is absent from a subsequent `GET`; a `modification`-kind record behaves identically.
- A test asserting `PATCH` still leaves document rows untouched — cheap, and it is the invariant
  PRD D2 exists to protect.

The FR-ATT-9 race is **not** unit-tested. sqlite serializes writers, so a test there proves
nothing about Postgres, and standing up a real Postgres for one test is out of proportion. The
lock is justified by reasoning in §3.3 and by the code being visibly present; that is the honest
state of it.

**Frontend** (vitest + RTL):

- `usePendingAttachments` drops files beyond `MAX_ATTACHMENTS - existingCount` (FR-CAP-2).
- `AttachmentPicker` disables "Add files" and shows the at-limit text when
  `existingCount + items.length >= MAX_ATTACHMENTS` (FR-CAP-1, FR-CAP-3).
- `RecordAttachmentList` renders the remove control with `canRemove` and omits it without
  (FR-UI-1); the unavailable row still offers it (FR-UI-6).
- `useRemoveMaintenanceRecordDocument` resolves successfully when the media delete rejects
  (FR-DET-6).

## 7. Implementation order

1. Partial index + de-duplication in `Migration` (§3.4) — it must land before any code can rely on
   uniqueness.
2. `Administrator.AttachDocument` / `DetachDocument` + tests.
3. `Processor` pass-throughs.
4. Routes + resource tests.
5. `MaintenanceRecordService.removeDocumentMedia`, `useRemoveMaintenanceRecordDocument`, widened
   `useAppendMaintenanceRecordDocument` invalidation.
6. `usePendingAttachments` / `AttachmentPicker` / `MaintenanceRecordForm` capacity threading.
7. `RecordAttachmentList` / `VehicleRecordDrawer` remove control and confirmation.

Steps 1–4 are independently shippable and immediately fix the 404 the frontend already hits;
steps 5–7 add removal on top.

## 8. Decisions

- **D-1 — Attachment mutation lives in dedicated administrator methods, not in `Update`.** The
  `Updates(map[string]any{…})` allow-list in `dbAdministrator.Update` is the mechanism that keeps
  `PATCH` from touching documents. Threading documents through it would put the PRD's D2 guarantee
  at the mercy of a future edit to that map.
- **D-2 — The cap race is closed with `SELECT … FOR UPDATE` on the parent record, dialect-guarded
  for sqlite.** Rejected: `SERIALIZABLE` (needs retry infrastructure this service lacks), a
  counting trigger (business logic in the database), and accepting the race (FR-ATT-9 is an
  acceptance criterion).
- **D-3 — Idempotency is enforced twice: an application check inside the locked transaction, and a
  partial unique index.** The index must be partial or detach-then-reattach breaks permanently,
  and it must be preceded by a one-time de-duplication or `Migration` can fail at startup on
  existing data.
- **D-4 — `existingCount` is a prop, not a query.** Keeps `AttachmentPicker` presentational and
  testable without a QueryClient; the drawer already holds the record.
- **D-5 — Removal confirmation does not name the file.** The filename lives inside `AttachmentRow`
  (resolved by `useMediaObject`) and the dialog lives in the drawer. Lifting it for one sentence
  is not worth widening the `onRemove` contract, and the unavailable row has no filename anyway.
- **D-6 — A failed media delete after a successful detach reports success.** Copied wholesale from
  `useRemoveVehiclePhoto`. The user's mental model is the reference; the orphan is the sweep's.
- **D-7 — The concurrency fix is not unit-tested.** sqlite cannot exhibit the race and Postgres in
  CI is disproportionate for one assertion. Stated here rather than papered over with a test that
  would pass either way.
