# Receipt Indicator on Vehicle Record Rows — Product Requirements Document

Version: v1
Status: Draft
Created: 2026-08-25

---

## 1. Overview

Task-004 closed the receipt loop: a user can attach documents to a maintenance or modification
record from the log form, and `fleet-service` stores them in `fleet.maintenance_record_documents`
and returns them as a `documentMediaIds` attribute. That task's plan also placed a `Paperclip`
affordance with an attachment count on the maintenance history row
(`docs/tasks/task-004-maintenance-mod-receipts/plan.md:5171`), so a user scanning their history
could see at a glance which records had paperwork behind them.

That affordance no longer exists. The vehicle-detail redesign (task-012) and the app-frame work
(task-017) replaced the maintenance-only history list with a unified feed —
`VehicleRecordsTable.tsx` — that renders maintenance, modification, fuel and mileage rows through a
single normalized `VehicleRecordRow` type. The redesign did not carry the attachment indicator
forward. A repo-wide grep for `Paperclip` in `apps/web/src` returns nothing, and
`VehicleRecordsTable.tsx` has no attachment column. Today the only way to learn whether a record
has a receipt is to click the row and wait for `VehicleRecordDrawer` to load the single-record
query.

This task restores the indicator in the new table. It is deliberately small and frontend-only. The
data is already on the wire: `provider.ListByVehicle` batches the document lookup for a whole page
(`apps/fleet-service/internal/maintenancerecord/provider.go:49-95`), `TransformSlice` runs the same
`Transform` as the single-record read (`rest.go:47-53`), and the list hook flat-maps whole resources
without pruning fields. `documentMediaIds` reaches the browser intact and is then discarded by the
row adapter at `apps/web/src/lib/hooks/api/vehicleRecords.ts:60-72`. The entire change is to stop
discarding it, thread a count through the normalized row, and render it.

## 2. Goals

Primary goals:

- Let a user scanning a vehicle's record feed see, without clicking, which records have documents
  attached and how many.
- Restore parity with the affordance task-004 designed and shipped, in the table that replaced it.
- Do so without regressing the deliberate performance property of `RecordAttachmentList`: no media
  metadata is fetched for collapsed rows.

Non-goals:

- **A distinct "no receipt" state.** Rows without documents render an empty cell. A muted or
  outlined icon on every receiptless row was considered and rejected as visual noise; absence of
  the icon is the negative signal.
- **Making the indicator interactive.** The row is already a click target that opens
  `VehicleRecordDrawer`, which lists the attachments. A nested button would duplicate that
  affordance and require `stopPropagation` plumbing for no user gain.
- Showing document *type*, thumbnail, filename, or size in the row. `RecordAttachmentList` in the
  drawer already does this and remains the place for it.
- Filtering or sorting the feed by "has attachments".
- Any change to `fleet-service`, the data model, or the API surface. See §5 and §7.
- Post-hoc attachment editing — still out of scope, as it was in task-004 §9.

## 3. User Stories

- As a vehicle owner reviewing my service history, I want to see which records have a receipt
  attached so that I can find the paperwork for a warranty claim without opening every row in turn.
- As a vehicle owner preparing to sell a vehicle, I want to see at a glance how much of my
  maintenance history is documented so that I know what evidence I can hand a buyer.
- As a vehicle owner who just logged a record with receipts, I want the feed to immediately reflect
  that the documents attached so that I have confirmation the upload landed.

## 4. Functional Requirements

### FR-ROW — Row data

- **FR-ROW-1.** `VehicleRecordRow` (`apps/web/src/lib/vehicleRecords.ts:5-27`) gains an optional
  numeric field carrying the count of attached documents for that record.
- **FR-ROW-2.** The field MUST be plain data — a number, not a component or icon reference.
  `lib/vehicleRecords.ts` is a pure-data module whose tests assert on plain values; the convention
  is documented at `apps/web/src/components/features/vehicles/VehicleCard.tsx:37-40`, which keeps
  `lucide-react` out of such modules.
- **FR-ROW-3.** The maintenance adapter (`apps/web/src/lib/hooks/api/vehicleRecords.ts:60-72`) MUST
  populate the field from `record.attributes.documentMediaIds?.length`. Because the backend
  serializes the attribute with `omitempty`, an absent key MUST be treated as zero, not as
  undefined-and-therefore-unknown.
- **FR-ROW-4.** The fuel and mileage adapters MUST leave the field unset. Those sources have no
  document concept; under FR-UI-1 an unset count and a zero count render identically (empty cell),
  so no special-casing is needed at the render layer.

### FR-UI — Rendering

- **FR-UI-1.** A row renders the indicator if and only if its document count is greater than zero.
  This single rule governs every row kind uniformly — maintenance, modification, fuel and mileage.
  Fuel and mileage therefore always render an empty cell, as a consequence of the rule rather than
  as an exception to it.
- **FR-UI-2.** The indicator is a `Paperclip` icon from `lucide-react` followed by the document
  count as text.
- **FR-UI-3.** The icon MUST be sized `h-4 w-4` and marked `aria-hidden="true"`, matching the
  established convention (`RecordAttachmentList.tsx:76`, `VehicleCard.tsx:115`).
- **FR-UI-4.** The indicator MUST NOT be a `button` and MUST NOT attach its own click or key
  handler. Clicking it falls through to the row's existing `onSelectRow`.
- **FR-UI-5.** The indicator MUST carry an accessible text label conveying the count — the icon is
  `aria-hidden`, so the count text alone must not be the only signal for a screen reader. The label
  must distinguish singular from plural (e.g. "1 attachment" vs "3 attachments").
- **FR-UI-6.** The count is rendered as-is. `MaxDocuments = 10`
  (`apps/fleet-service/internal/maintenancerecord/model.go:19`) caps it server-side, so no
  truncation or "9+" treatment is required.

### FR-TBL — Table structure

- **FR-TBL-1.** The table gains a sixth column for the indicator, added to the existing five
  (Date, Type, Item, Odometer, Cost — header at `VehicleRecordsTable.tsx:148-156`).
- **FR-TBL-2.** The skeleton rows (`VehicleRecordsTable.tsx:71-83`) and the empty state
  (`:161-165`) both hardcode `colSpan={5}` and MUST be updated to match the new column count.
  Missing either leaves a visibly misaligned table.
- **FR-TBL-3.** The new column header MUST NOT introduce a visible text label that crowds the
  header row; if the header cell is visually empty it MUST still be announced to assistive
  technology (e.g. a visually-hidden label).
- **FR-TBL-4.** Adding the column MUST NOT cause the Item column to lose its existing
  `max-w-[240px] truncate` behaviour or push Cost off-screen at the narrowest supported viewport.

## 5. API Surface

**No changes.** This is stated as a requirement, not an observation — the design phase should not
reopen it without cause.

The list endpoint `GET /vehicles/{id}/maintenance-records` already returns everything needed:

- `Attributes.DocumentMediaIDs` is serialized as `documentMediaIds,omitempty`
  (`apps/fleet-service/internal/maintenancerecord/rest.go:22`).
- `Transform` unconditionally sets it (`rest.go:41`), and `TransformSlice` (`rest.go:47-53`) applies
  the same `Transform` per model, so list and single-record responses are identical in shape.
- `provider.ListByVehicle` (`provider.go:49-95`) populates documents for the whole page with a
  single batched `WHERE maintenance_record_id IN (?)` query, grouped in memory. The comment at
  `provider.go:72-75` records that this deliberately replaced an N+1.

Consequently a record with zero attachments omits the key entirely, which FR-ROW-3 handles.

## 6. Data Model

**No changes.** `fleet.maintenance_record_documents` (`entity.go:31-39`) and the
`documentMediaIDs []string` field on the domain model (`model.go:21-37`) are sufficient. No
migration.

## 7. Service Impact

| Service | Change |
|---|---|
| `fleet-service` | **None.** Data already served on the list endpoint. |
| `media-service` | **None.** No media metadata is fetched for the indicator — only `documentMediaIds.length`. |
| `apps/web` | All changes, across three files (see below). |

Frontend files affected:

1. `apps/web/src/lib/vehicleRecords.ts` — add the count field to `VehicleRecordRow` (FR-ROW-1/2).
2. `apps/web/src/lib/hooks/api/vehicleRecords.ts` — populate it in the maintenance adapter
   (FR-ROW-3).
3. `apps/web/src/components/features/vehicles/detail/VehicleRecordsTable.tsx` — new column, header,
   cell, and the two `colSpan` fixes (FR-UI-*, FR-TBL-*).

## 8. Non-Functional Requirements

- **Performance.** The indicator MUST derive purely from `documentMediaIds.length` and MUST NOT
  trigger any per-attachment request. `RecordAttachmentList` fans out one `useMediaObject` call per
  attachment and its docstring (`RecordAttachmentList.tsx:85-87`) states it is mounted only for the
  expanded record specifically to avoid "25 × N metadata requests" on a full page. Rendering a
  count per row must not undo that.
- **No new network calls.** The change adds zero requests; it consumes a field already present in
  the list response.
- **Accessibility.** Decorative icon is `aria-hidden`; the count is conveyed by an accessible label
  (FR-UI-5). The row remains a single keyboard-focusable target — the indicator must not add a tab
  stop, which is a direct consequence of FR-UI-4.
- **Security.** No new data is exposed. `documentMediaIds` already crosses the wire on this exact
  endpoint under the existing `RequireSameFleet` authorization, and the count reveals strictly less
  than the ids already sent.

## 9. Open Questions

1. **Column vs. inline placement.** This PRD specifies a dedicated sixth column (FR-TBL-1), which
   keeps the indicator vertically aligned and scannable. The alternative — rendering the icon
   inline after `row.title` in the existing Item cell — adds no column and no `colSpan` churn, but
   competes with the title's `truncate`. To be settled in `/design-task`.
2. **Phantom `document-media` endpoint.** The frontend defines
   `POST /api/fleet/maintenance-records/{id}/document-media`
   (`apps/web/src/services/api/MaintenanceRecordService.ts:49-61`, called by
   `useAppendMaintenanceRecordDocument` at `lib/hooks/api/maintenance.ts:206-216`), but **no such
   route is registered in `fleet-service`** — `InitializeRoutes` (`resource.go:49-331`) registers
   only list, create, get, patch and delete, and a repo-wide grep for `document-media` in `*.go`
   returns nothing. Any call would 404. This is out of scope for this task and does not affect the
   indicator, but it is a live defect discovered during this spec and should be filed separately.
3. **Post-log cache freshness.** User story 3 assumes the feed reflects a just-logged record's
   attachments. Whether the existing React Query invalidation after logging already guarantees this
   should be confirmed during design rather than assumed.

## 10. Acceptance Criteria

- [ ] A maintenance record with N ≥ 1 attached documents renders a `Paperclip` icon and the number
      N in its row in `VehicleRecordsTable`.
- [ ] A maintenance record with zero attachments renders an empty indicator cell — no icon, no
      "0", no muted placeholder.
- [ ] A modification record behaves identically to a maintenance record (same rule, per FR-UI-1).
- [ ] Fuel and mileage rows render an empty indicator cell.
- [ ] The indicator is not focusable, is not a `button`, and clicking it opens the record drawer
      via the row's existing handler.
- [ ] A screen reader announces the attachment count with correct singular/plural wording.
- [ ] Skeleton rows and the empty state span the full table width with no visible misalignment.
- [ ] No additional network request is issued when a page of 25 records containing attachments
      renders — verifiable by asserting the media-object hook is not called from the table.
- [ ] `VehicleRecordsTable.test.tsx` covers: icon+count present for a record with attachments,
      absent for one without, absent for fuel/mileage, and the non-interactive requirement.
- [ ] The adapter test in `lib/hooks/api/vehicleRecords.test.ts` asserts the count is populated
      from `documentMediaIds`, including the absent-key-means-zero case (FR-ROW-3).
- [ ] `make fe-test` and `make fe-build` pass.
