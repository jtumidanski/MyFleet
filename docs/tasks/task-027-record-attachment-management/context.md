# Record Attachment Management — Implementation Context

Task: `task-027-record-attachment-management`
Worktree: `/home/tumidanski/source/MyFleet/.worktrees/task-027-record-attachment-management`
Branch: `task-027-record-attachment-management`
PRD: `prd.md` · Design: `design.md` · Plan: `plan.md`

---

## 1. What is actually broken today

`VehicleRecordDrawer.handleUpdateRecord` loops over files picked during an edit and calls
`maintenanceRecordService.appendDocumentMedia`, which POSTs to
`/api/fleet/maintenance-records/{id}/document-media`
(`apps/web/src/services/api/MaintenanceRecordService.ts:49-63`). **That route does not exist.**
`apps/fleet-service/internal/maintenancerecord/resource.go` registers `GET`/`POST` on the
`/vehicles/{id}/maintenance-records` collection and `GET`/`PATCH`/`DELETE` on
`/maintenance-records/{id}` — nothing else. The only two `document-media` references anywhere in the
repo are the two frontend ones.

So today: the file uploads to media-service and is confirmed, the attach 404s, the drawer counts a
failure and toasts *"Record updated, but 1 attachment could not be attached"*, and the confirmed
media object is left referenced by nothing.

The gap is symmetrical: there is no way at any layer to *remove* an attachment from a saved record.

---

## 2. Key files

### fleet-service — `apps/fleet-service/internal/maintenancerecord/`

| File | What matters in it |
| --- | --- |
| `entity.go` | `DocumentEntity` (`:29-37`) already has `DeletedAt`, so soft-detach needs no column. `Migration` (`:41`) is currently a bare `AutoMigrate` — Task 1 adds `ApplyPartialIndexes` to it. |
| `model.go` | `MaxDocuments = 10` (`:19`). `Validate` (`:105`) rejects `len(documentMediaIDs) > MaxDocuments`. `WithDocumentMediaIDs` (`:82`) is a whole-set replace used only by the create builder — attach/detach must not go through it. |
| `administrator.go` | `Update` (`:51`) uses an explicit column allow-list map and **re-reads** before returning; its doc comment explains why (a model built from the in-memory entity is how a response comes to disagree with the row). `AttachDocument` copies that re-read reasoning. `SoftDelete` (`:88`) is the `RowsAffected == 0 → ErrNotFound` template `DetachDocument` follows. |
| `provider.go` | `ErrNotFound` sentinel (`:11`). `GetByID` and `ListByVehicle` both filter `deleted_at IS NULL` on document rows, so a soft-detached row disappears from both paths with no further work. |
| `processor.go` | `SoftDelete` (`:71`) is the `ErrNotFound → server.ErrNotFound` translation template. |
| `resource.go` | `DocumentValidator` interface (`:41`, nil is explicitly legal). The create path's ownership check (`:167-176`) is the exact block the attach route copies with a one-element slice. The item routes' preamble — `proc.GetByID` → `vehicleAccessor.GetByID` → `RequireSameFleet` → `RequireWrite` — is repeated verbatim by both new routes. |
| `provider_test.go` | `newTestDB` (`:14`) and `insertRecord` (`:49`) — the fixtures every test in the package uses. Task 1 splits `newTestDB`. |

### web — `apps/web/src/`

| File | What matters in it |
| --- | --- |
| `services/api/MaintenanceRecordService.ts` | `appendDocumentMedia` (`:49`) already sends the exact envelope the new endpoint must accept. Do not change it. |
| `lib/hooks/api/maintenance.ts` | `maintenanceRecordKeys` (`:33`). `useAppendMaintenanceRecordDocument` (`:205`) omits `lists()` from `onSettled` — that is the stale-count bug. |
| `lib/hooks/api/media.ts` | `useRemoveVehiclePhoto` (`:322-350`) is the reference-first-then-best-effort-object-delete pattern, with the reasoning already written out in its doc comment. Copy it. |
| `lib/hooks/usePendingAttachments.ts` | `MAX_ATTACHMENTS = 10` (`:11`). `room` (`:62`) and `isFull` (`:143`) are the two places `existingCount` has to land. |
| `components/features/vehicles/maintenance/AttachmentPicker.tsx` | Currently counts only pending items. |
| `components/features/vehicles/maintenance/RecordAttachmentList.tsx` | Three row shapes: unavailable (`div`), image (`div` + thumbnail), document (a full-width `Button`). The last one forces a wrapper for the remove control. |
| `components/features/vehicles/detail/VehicleRecordDrawer.tsx` | Owns the record, the sequential attach loop and its explanatory comment, and the edit/view modes. |
| `components/features/settings/MemberList.tsx` | `AlertDialog` usage pattern to copy (`:208-234`), including the `onClick={(e) => { e.preventDefault(); … }}` on `AlertDialogAction`. |

---

## 3. Decisions carried in from the PRD and design

| ID | Decision |
| --- | --- |
| PRD D1 | Single-ID attach endpoint, not batch — it is the contract the client was already written against. |
| PRD D2 | `PATCH` gains no `documentMediaIds`. Attach/detach are the attachment mutation surface. |
| PRD D3 | Detach deletes the media object, issued **by the browser**, not by fleet-service. `mediaclient` is the no-JWT internal client; giving media-service's `/internal` surface delete authority would create an unauthenticated destructive endpoint. |
| PRD D4 | Orphans left by the currently-broken append path are the admin orphan sweep's and `purge_after`'s problem. No backfill, no compensating delete. |
| Design D-1 | Attachment mutation gets its own administrator methods. `Update`'s allow-list map is the mechanism that keeps `PATCH` from touching documents. |
| Design D-2 | The cap race is closed with `SELECT … FOR UPDATE` on the parent record, dialect-guarded for sqlite. Rejected: `SERIALIZABLE` (needs retry infrastructure the service lacks), a counting trigger, and accepting the race. |
| Design D-3 | Idempotency is enforced twice — an application check inside the locked transaction, plus a **partial** unique index preceded by a one-time de-duplication. |
| Design D-4 | `existingCount` is a prop, not a query, so `AttachmentPicker` stays presentational and testable without a `QueryClient`. |
| Design D-5 | The removal confirmation does not name the file. The filename is resolved inside `AttachmentRow`; the dialog lives in the drawer. |
| Design D-6 | A failed media delete after a successful detach reports **success**. |
| Design D-7 | The concurrency fix is not unit-tested. sqlite cannot exhibit the race; Postgres in CI is disproportionate for one assertion. |

---

## 4. Corrections to the spec, resolved in the plan

**A. Cross-fleet is `404`, not `403`.** PRD §5.1/§5.2's status tables and design §3.1 both claim a
cross-fleet caller gets `403`. `authz.RequireSameFleet`
(`apps/fleet-service/internal/authz/authz.go:12-17`) returns `server.ErrNotFound`, deliberately, so
cross-fleet existence is never leaked — its doc comment says so, `TestRequireSameFleet_404OnMismatch`
asserts it, and `TestPatch_otherFleetIsNotFound` pins it at the route level. The plan implements and
tests `404`. Viewer is still `403` (`RequireWrite` returns `server.ErrForbidden`). Do not change
`authz` to match the PRD.

**B. The download row needs a wrapper.** Design §4.2 says the remove control is added to each row
"rather than hoisted", and separately notes that turning the download row into a `div` with two
buttons would change the download hit area. A `<button>` cannot legally nest inside a `<button>`, so
a wrapper is the only valid structure. Resolution: add it **only when the row is removable**, so the
row a viewer sees is byte-identical to today's.

**C. The drawer does not currently use `AlertDialog`.** Design §4.3 says the confirmation uses "the
same `AlertDialog` pattern the drawer already uses for record deletion". It does not — record
deletion in `VehicleRecordDrawer` is a direct one-click `handleDeleteRecord`. `AlertDialog` exists at
`apps/web/src/components/ui/alert-dialog.tsx` and is used in `MemberList.tsx` and
`PhotoGalleryDialog.tsx`; the plan follows `MemberList`'s shape. Adding confirmation to record
*deletion* is out of scope.

---

## 5. Non-obvious technical notes

- **The sqlite test fixture is hand-written DDL, not `Migration`.** `newTestDB` creates
  `fleet.maintenance_records` and `fleet.maintenance_record_documents` with explicit `CREATE TABLE`
  after `ATTACH DATABASE ':memory:' AS fleet`, because GORM's `AutoMigrate` emits `CREATE INDEX`
  with the schema prefix stripped under sqlite. This means a new index added to `Migration` is
  **not** in the test database unless it is applied explicitly — which is why Task 1 has
  `newTestDB` call `ApplyPartialIndexes` rather than hand-writing the index DDL a second time.
- **SQLite puts the schema on the index name, not the table name.** `CREATE UNIQUE INDEX
  fleet.ux_… ON maintenance_record_documents (…)`. Postgres is the other way round. This is exactly
  what `mediavariant.ApplyPartialIndexes`
  (`apps/media-service/internal/mediavariant/entity.go:61-68`) already branches on, and is the
  precedent for the dialect guard.
- **`CURRENT_TIMESTAMP`, not `NOW()`,** in the de-duplication `UPDATE`, so one statement is valid
  under both Postgres and sqlite.
- **`db.Name()`** returns the dialector name (`"sqlite"`, `"postgres"`) and is promoted onto
  `*gorm.DB`; `mediavariant` already relies on it.
- **`server.RegisterInputHandler[T]`** (`packages/shared-go/server/handler.go:47`) decodes
  `{"data":{"attributes": T}}`. The declared JSON:API `type` is discarded — no handler in this
  codebase validates it, and inventing that check on this one route would be a one-off.
- **`server.ErrValidation` → 422, `server.ErrNotFound` → 404, `server.ErrForbidden` → 403** via
  `StatusFor`; existing package tests already depend on those mappings.
- **`mediaService` is not a `BaseService`.** It defines its own `remove(id)` at
  `apps/web/src/services/api/MediaService.ts:105`. `MaintenanceRecordService` *does* extend
  `BaseService`, whose `remove(id)` targets `/api/fleet/maintenance-records/{id}` — do not confuse it
  with the new `removeDocumentMedia`.
- **`console.warn` is permitted** in `apps/web` (`src/lib/config/runtimeConfig.ts:112` uses it), so
  the best-effort media-delete failure can be logged without an eslint fight.

---

## 6. Dependencies and ordering

Task 1 → 2 → 3 → 4 must run in order: the index must exist before the administrator relies on
uniqueness, and each layer consumes the one below. **Tasks 1–4 are independently shippable and fix
the live 404 with zero frontend change.**

Tasks 5 → 6 → 7 add removal and cap ergonomics on top. Task 5 depends on Task 4's detach route
existing. Task 6 and Task 7 both touch `VehicleRecordDrawer.tsx`; Task 6 adds the
`existingAttachmentCount` prop to the edit-mode form, Task 7 adds the remove wiring to the view mode
— run them in order to avoid a conflict in the same file.

Task 8 is the whole-branch verification gate.

---

## 7. Verification

```sh
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
make ci    # lint-check, vet, test, build, fe-test, fe-build
```

Per-layer while iterating:

```sh
go test ./apps/fleet-service/internal/maintenancerecord/ -v
cd apps/web && npx vitest run && npx tsc --noEmit -p tsconfig.json
```

No `deploy/k8s` changes are expected — both routes sit under the already-routed `/api/fleet` prefix,
so no kustomize render or server dry-run is required for this task. `git diff --name-only main...HEAD
-- deploy/` should be empty.

Before opening a PR, run the code-review step (`/audit-plan` or
`superpowers:requesting-code-review`); both a Go and a TypeScript reviewer apply to this branch.
