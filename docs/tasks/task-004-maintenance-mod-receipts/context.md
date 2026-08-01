# Implementation Context — task-004 Maintenance & Modification Logging with Receipts

Companion to `plan.md`. Everything an implementer needs to know about the codebase
*before* touching it. Verified against source in this worktree on 2026-07-31.

---

## 1. Where the work lands

| Area | Path | Slice (design §1) |
|---|---|---|
| 415 sentinel | `packages/shared-go/server/errors.go` | B |
| Allowlist, download headers, terminal failure, internal endpoint | `apps/media-service/internal/mediaobject`, `.../internal/processing` | B, C, D |
| Category `kind`, record `description`, kind filter, media validation | `apps/fleet-service/internal/maintenancecategory`, `.../maintenancerecord`, `.../mediaclient` (new) | A, D |
| Attachment UI | `apps/web/src/lib/hooks`, `.../lib/utils`, `.../components/features/vehicles/maintenance` | A, D |
| Config + edge | `deploy/k8s/base/media-service/configmap.yaml`, `deploy/k8s/base/fleet-service/configmap.yaml`, `deploy/k8s/overlays/main/ingressroute.yaml`, `deploy/compose/*` | B, D |

## 2. Build, test, verify

Go is a **workspace** (`go.work`) of six modules: `packages/shared-go`, `packages/dto-go`,
`apps/{auth,fleet,media,notification}-service`. Module paths are
`github.com/jtumidanski/myfleet/<path>`.

```sh
make vet          # go vet github.com/jtumidanski/myfleet/...
make test         # go test -race github.com/jtumidanski/myfleet/...
make lint-check   # ./tools/lint.sh --check  (what CI runs)
make fe-test      # npm run -w apps/web test && npm run -w packages/shared-ts test
make fe-build
make manifests    # renders both kustomize overlays + asserts invariants
make ci           # lint-check vet test build fe-test fe-build manifests
```

Node is not always on `PATH`:

```sh
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
```

Single Go test, from the service directory:
`cd apps/media-service && go test ./internal/mediaobject/... -run TestContentDisposition -v`

Single web test, from the repo root:
`npm run -w apps/web test -- src/lib/utils/download.test.ts`

Go version is **1.25.0** in every module, so multi-`%w` `fmt.Errorf` wrapping is available.

## 3. Backend conventions that constrain every Go change

The DDD layering is strict and uniform across every `internal/<domain>` package:

- `model.go` — **immutable** domain model. Unexported fields, value-receiver accessors,
  `WithX` copy-mutators. No persistence, no HTTP.
- `entity.go` — GORM struct + `TableName()` (schema-qualified, e.g. `fleet.maintenance_records`)
  + `Migration(db)` + `Make(entity) Model` + `(Model) ToEntity()`.
- `builder.go` — `NewBuilder()`, `SetX` chain, `Build() (Model, error)` enforcing invariants.
- `provider.go` — **read-only** interface + `dbProvider`. Package-local `ErrNotFound`.
- `administrator.go` — **write** interface (`Insert`/`Update`/`SoftDelete`/`*Tx`).
- `processor.go` — business logic; injected with Provider/Administrator; maps package
  errors to `server.Err*` sentinels.
- `resource.go` — chi route wiring via `InitializeRoutes(...) func(chi.Router)`.
- `rest.go` — JSON:API `Attributes` struct + `Transform`/`TransformSlice`.

Cross-domain access goes through a **one-method interface declared in the consumer**
(`maintenancerecord.VehicleAccessor` at `resource.go:20` is the model to copy), never by
importing another domain's provider.

### Error → status mapping

`packages/shared-go/server/errors.go` holds sentinels; `StatusFor` maps them.
Currently: 401, 403, 404, 409, 410, 413, 422. **No 415** — this task adds it.

`server.WriteError` renders `{"errors":[{status, code, title}]}` where `title` is
`err.Error()` and `code` comes from `codeFor(status)` in `server.go:12`. `codeFor` has no
415 arm today (and, separately, no 413 arm — 413 currently renders `internal_error`; that
pre-existing gap is **out of scope**).

Because `title` is `err.Error()`, wrapping with `fmt.Errorf("%w: …", server.ErrX)` is how a
handler adds detail to a response while keeping `errors.Is` working.

### Input handlers

`server.RegisterInputHandler[T]` (`handler.go:42`) decodes `{"data":{"attributes":T}}` and
answers `422` on a decode failure. Unknown attributes are ignored — which is why
`api-contracts.md` §4 can say `documentMediaIds` on `PATCH` is ignored rather than rejected:
that falls out of not declaring the field.

### Paging

`server.ParsePage` reads `page[number]`/`page[size]`, defaults 1/25, **caps size at 100**.
`page.Meta(total)` builds `{total,totalPages,number,size}`.

### Internal (no-JWT) endpoints

Two precedents, both in fleet-service: `membership.InitializeInternalRoutes`
(`membership/resource.go:86`) and `maintenanceschedule.InitializeInternalRoutes`. They are
registered in `apps/fleet-service/cmd/main.go:167-168` **outside** the
`pr.Use(authmw.JWT(keyfn))` group. Their payloads are **flat JSON, not JSON:API**
(`ActiveResponse{FleetID, Role}`, `[]Member{user_id, role}` — snake_case).

The consumer precedent is `apps/notification-service/internal/fleetclient/client.go`:
`Client{base, hc}` + `NewClient(base)` + one method per endpoint + a private `getJSON`
that turns any non-200 into an error. Configured via `FLEET_INTERNAL_URL`.

**Security coupling:** every unauthenticated `/internal/*` surface must have a matching
priority-200 `internal-deny` route in `deploy/k8s/overlays/main/ingressroute.yaml`. The
existing fleet-service rule (line 89) carries a long comment explaining why it is
`PathRegexp(...^/+api/+fleet[^/]*/*internal)` and not `PathPrefix` — Traefik normalises the
path before matching, and `stripprefix` removes a literal string rather than a path
segment. media-service needs the mirrored rule; see plan Task 10.

Note the TLS twin `myfleet-routes-tls` has `routes: []` **on purpose** — kustomize
`replacements` copies `spec.routes` from `myfleet-routes` at build time. Never hand-write
routes into the twin.

### Config

`packages/shared-go/config`: `Get(key, fallback)`, `MustGet(key)` (panics), `GetInt(key, fallback)`.
`os.Getenv` in handlers is forbidden.

## 4. Frontend conventions that constrain every TS change

- **Services** (`src/services/api/*.ts`) extend `BaseService<A, CreateA, UpdateA>` with
  `resourceType` + `basePath`, and add nested-route methods via the protected `listAt` /
  `createAt` helpers. `apiClient.baseUrl` is `''`, so paths are absolute gateway paths.
- **Hooks** (`src/lib/hooks/api/*.ts`) own a hierarchical query-key factory and wrap
  React Query. List hooks use `select: (result) => result.data`, so consumers get
  `JsonApiResource<A>[]`, not the envelope.
- **Types** (`src/types/models/*.ts`) mirror the Go `rest.go` `Attributes` struct exactly,
  and say so in a doc comment.
- **Forms** use `react-hook-form` + `zodResolver` with the schema in `src/lib/schemas/`.
- **Errors** go through `createErrorFromUnknown(err)` from `@myfleet/shared-ts`, then
  `toast.error(...)`. Per-tile failures render inline instead (see `MediaThumbnail`) so a
  gallery of N broken items does not fire N toasts.
- **UI kit** is shadcn under `src/components/ui/`. Present: button, card, form, input,
  label, select, skeleton, switch, textarea. **There is no `badge` component** — chips are
  hand-rolled spans; `SeverityChip.tsx` is the in-repo pattern to copy.
- **Tests** are vitest + `@testing-library/react`, colocated as `*.test.ts(x)`. Setup at
  `src/test/setup.ts`. Hook tests wrap in a `QueryClientProvider` and `vi.mock` the service
  module (`src/lib/hooks/api/media.test.ts` is the reference).

### The existing media upload flow

`performMediaUpload(file, deps)` in `src/lib/hooks/api/media.ts:67` runs init → PUT → confirm
and rejects oversize files client-side before step 1 (`MEDIA_MAX_UPLOAD_BYTES = 26214400`,
explicitly documented as a UX affordance, not a control).

`useMediaContentUrl(id)` fetches the bytes as a Blob through `apiClient.requestBlob` and
hands back an object URL, creating and revoking as a matched pair inside one effect. It is
deliberately one render behind `data`; read the comment at `media.ts:104` before touching it.

`useUploadMedia(vehicleId)` hard-codes invalidation of `mediaKeys.vehicleMedia(vehicleId)`,
which is what makes it wrong for a receipt — hence the extraction in plan Task 19.

## 5. State of the feature today (why each change is needed)

- `MaintenanceRecordForm.tsx` has `documentMediaIds: []` in `defaultValues` and no control
  that ever writes to it. `VehicleMaintenanceSection.handleCreateRecord` forwards
  `values.documentMediaIds ?? []`. So the field exists end-to-end and is always empty.
- `fleet.maintenance_record_documents` and `maintenancerecord.DocumentEntity` already
  exist, and `InsertTx` already writes document rows. Nothing reads them in the UI.
- `POST /media` stores whatever `contentType` string the client sends
  (`mediaobject/processor.go:88`), and `GET /media/{id}/content` echoes it straight back as
  the response `Content-Type` (`resource.go:164`). That pair is why broadening the accepted
  file types without an allowlist would be a same-origin stored-XSS vector.
- `processing/worker.go:206` calls `image.Decode` on every confirmed upload. A PDF fails
  it, `handle` returns an error, and `events.Consume`
  (`packages/shared-go/events/consumer.go`) `continue`s **without committing the offset** —
  so it redelivers forever *and blocks the partition*. There is no retry budget anywhere;
  design D13 corrects the PRD on this point.
- `maintenancerecord/provider.go:55` issues one document query per record in the page — 26
  queries for a 25-record page. Harmless until now because no record has documents.
- `lib/schemas/maintenanceRecord.ts` declares `mileage`/`cost` non-optional while the form
  writes `undefined` when they are cleared, so a record with no cost cannot be submitted
  today (design D22).
- `MaintenanceRecordService.appendDocumentMedia` calls
  `POST /maintenance-records/{id}/document-media`, a route that **does not exist** in
  `maintenancerecord/resource.go`. It is dead code with no callers. Out of scope for this
  task; noted so nobody builds on it.

## 6. Decisions carried from the design that are easy to get wrong

| # | Decision | Failure mode if ignored |
|---|---|---|
| D3 | `categoryIDs`: `nil` = no filter, empty-non-nil = match nothing | A fleet with no modifications sees every maintenance record labelled a modification |
| D8 | The server validates **ownership**, not `status == ready` | A JPEG whose worker is a second behind loses the user's form |
| D10 | Content types normalised via `mime.ParseMediaType`, stored normalised | `text/csv; charset=utf-8` (what browsers actually send) gets a 415 |
| D11 | `ClassImage` is hard-coded to `{image/jpeg, image/png}`, never configurable | An operator could add `image/heic` and hand `image.Decode` bytes it cannot read |
| D12 | `ClassUnknown` confirms like a document, not like an image | A legacy row with an unrecognised type gets fed to `image.Decode` |
| D13 | Retry only *transient* failures; permanent ones go to `failed` + `MarkProcessed` | One corrupt file blocks its Kafka partition forever |
| D15 | Download `Content-Type` re-resolved through the allowlist on every read | Legacy rows keep serving arbitrary client-supplied types |
| D20 | media-service's `internal-deny` Traefik rule ships **with** the endpoint | A public, unauthenticated cross-fleet media-existence oracle |

## 7. Deviations this plan makes from the design

1. **`kind` GORM default is `default:'maintenance'` (quoted), not `default:maintenance`.**
   Design D1 writes it unquoted. GORM passes the tag value verbatim into the DDL, so the
   unquoted form emits `DEFAULT maintenance` and PostgreSQL reads that as a column
   reference (`column "maintenance" does not exist`) and fails `AutoMigrate`. The quoted
   form is correct on both PostgreSQL and the SQLite used in tests.
2. **`maintenancecategory.Processor` is constructed twice in `fleet-service/cmd/main.go`** —
   once inside `maintenancecategory.InitializeRoutes` (unchanged) and once in `main` to pass
   as `maintenancerecord`'s `CategoryAccessor`. The processor is stateless, so this costs
   nothing and avoids reshaping a working initializer signature.
3. **`MaintenanceScheduleForm` is given maintenance-kind categories only.** Not in the
   design, but PRD §2 makes "recurring schedules for modifications" a non-goal; seeding
   twelve modification categories would otherwise make them selectable in the schedule
   picker, which would create modification *schedules*. Plan Task 25 filters them out.
4. **The 415 `detail` is carried in the error envelope's `title`**, because `WriteError`
   sets `Title: err.Error()` and never populates `Detail`. `api-contracts.md` §5 asks that
   the accepted types be named to the client; wrapping the sentinel achieves that without
   reshaping the shared error envelope.

## 8. Files created by this task

```
apps/media-service/internal/mediaobject/contenttype.go
apps/media-service/internal/mediaobject/contenttype_test.go
apps/media-service/internal/mediaobject/download.go
apps/media-service/internal/mediaobject/download_test.go
apps/fleet-service/internal/mediaclient/client.go
apps/fleet-service/internal/mediaclient/client_test.go
apps/fleet-service/internal/maintenancecategory/provider_test.go
apps/fleet-service/internal/maintenancerecord/model_test.go
apps/fleet-service/internal/maintenancerecord/provider_test.go
apps/web/src/lib/utils/download.ts
apps/web/src/lib/utils/download.test.ts
apps/web/src/lib/hooks/usePendingAttachments.ts
apps/web/src/lib/hooks/usePendingAttachments.test.ts
apps/web/src/components/features/vehicles/maintenance/AttachmentPicker.tsx
apps/web/src/components/features/vehicles/maintenance/RecordAttachmentList.tsx
apps/web/src/components/features/vehicles/maintenance/RecordAttachmentList.test.tsx
apps/web/src/components/features/vehicles/maintenance/MaintenanceRecordForm.test.tsx
```

## 9. Signature changes that ripple into existing tests

| Change | Task | Existing files that must be updated in the same task |
|---|---|---|
| `mediaobject.NewProcessor` gains an `Allowlist` param | 5 | `mediaobject/processor_test.go`, `mediaobject/resource_test.go` |
| `mediaobject.InitializeRoutes` gains an `Allowlist` param | 5 | `mediaobject/resource_test.go`, `media-service/cmd/main.go` |
| `mediaobject.Provider` gains `ListActiveByFleetAndIDs` | 9 | `processing/worker_test.go` (`fakeProvider`) |
| `maintenancecategory.Provider.List` gains a `kind` param | 12 | `maintenancecategory/processor.go`, `.../resource.go` |
| `maintenancerecord.Provider.ListByVehicle` gains `categoryIDs` | 14 | `maintenancerecord/processor.go`, `.../resource.go` |
| `maintenancerecord.InitializeRoutes` gains two params | 16 | `fleet-service/cmd/main.go` |
| `maintenanceCategoryService.list()` gains an optional `kind` | 18 | `lib/hooks/api/maintenance.ts` |

`maintenancerecord` has **no** test files today, so its provider/processor changes break
nothing; `maintenancecategory` has only `entity_test.go`, which asserts `len(seeds)` and so
survives the enlarged seed list unchanged.
