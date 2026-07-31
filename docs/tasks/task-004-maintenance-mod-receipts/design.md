# Maintenance & Modification Logging with Receipts — Design

Version: v1
Status: Approved
Created: 2026-07-31
PRD: `prd.md` · Contracts: `api-contracts.md`

---

## 1. Scope and shape of the work

The PRD bundles four largely independent changes behind one user-visible feature:

| Slice | Service | Coupling |
|---|---|---|
| A — category `kind` + record `description` + kind filtering | fleet-service, web | none |
| B — content-type allowlist, document class, terminal processing state | media-service, shared-go | none |
| C — download hardening (`Content-Disposition`, `nosniff`) | media-service | depends on B's allowlist |
| D — attachment lifecycle (upload → validate → attach → view → download) | fleet-service, media-service, web, deploy | depends on B and C |

A, B and C can land independently; D is the integrating slice and carries the only new
cross-service seam. This design keeps that seam as small as possible: **one** new internal endpoint
on media-service, **one** new HTTP client in fleet-service, and no shared database access.

Nothing here changes the existing image path. The vehicle photo gallery, the variant worker, and
`media.uploaded` behave exactly as they do today for `image/jpeg` and `image/png`; every new
behaviour is gated on a content-type classification that those two types do not take.

---

## 2. Architecture at a glance

```
                       ┌──────────────────────────────────────────┐
  browser              │ apps/web                                 │
  ─────────            │  MaintenanceRecordForm                   │
                       │   └ usePendingAttachments ──┐            │
                       │  RecordAttachmentList ───┐  │            │
                       └──────────────────────────┼──┼────────────┘
                                                  │  │
                    GET /api/media/{id}(/content) │  │ POST /api/media
                                                  │  │ PUT  /api/media/{id}/content
                                                  │  │ POST /api/media/{id}/confirm
                                                  ▼  ▼
        ┌───────────────────────────────────────────────────────────────┐
        │ media-service                                                 │
        │  mediaobject.Allowlist ── classify(contentType)               │
        │    image    → uploaded → processing → (worker) → ready|failed │
        │    document → uploaded → ready            (no Kafka event)    │
        │  download.go  Content-Disposition / nosniff / octet-stream    │
        │                                                               │
        │  GET /internal/media?fleet_id=…&ids=…   ← network-restricted  │
        └───────────────────────────▲───────────────────────────────────┘
                                    │ (validate documentMediaIds on create)
        ┌───────────────────────────┴───────────────────────────────────┐
        │ fleet-service                                                 │
        │  maintenancecategory: Kind{maintenance|modification}          │
        │  maintenancerecord:   description, kind filter, doc validation│
        │  internal/mediaclient: batch ownership lookup                 │
        └───────────────────────────────────────────────────────────────┘
```

Two rules hold across the whole design:

1. **Classification happens once, at the door.** `POST /media` normalises and validates the
   content type, and the normalised value is what gets stored. Every later decision — publish an
   event or not, `inline` or `attachment`, decode or don't — is a pure function of the stored type
   and the allowlist. No downstream component re-derives it from the filename or sniffs bytes.
2. **Cross-service data moves over HTTP, never over the database.** fleet-service asks
   media-service about media ownership through an internal endpoint, in the same shape
   `notification-service` already uses to ask fleet-service about members.

---

## 3. Decision log

### D1 — `kind` is a column on `maintenance_categories`, not a new entity

Alternatives considered: (a) a `modifications` table with its own routes; (b) a
`maintenance_records.kind` column; (c) `kind` on the category.

(a) duplicates the record shape, the authz wiring, the transport layer and the history list for a
domain object with identical fields — the classic "two tables that are the same table". (b) allows a
record's kind to disagree with its category's kind, which makes the filter unfalsifiable. (c) has
exactly one source of truth: the category a record points at determines its kind, and a record
cannot be inconsistent because it stores no kind at all.

The cost of (c) is that a record's kind requires a category lookup. That is free on the client (the
full category list is already cached by `useMaintenanceCategories`, 20 rows) and one indexed
`IN (…)` on the server. Accepted.

```go
// maintenancecategory/model.go
type Kind string

const (
    KindMaintenance Kind = "maintenance"
    KindModification Kind = "modification"
)

// ParseKind maps a query-parameter value to a Kind. The empty string means
// "no filter" and yields ("", nil); anything else unrecognised is a validation
// error, never a silent empty result (FR-KIND-4).
func ParseKind(s string) (Kind, error)
```

`Entity.Kind` is `gorm:"type:varchar(20);not null;default:maintenance"`. On PostgreSQL,
`AutoMigrate` issues `ADD COLUMN … NOT NULL DEFAULT 'maintenance'`, which backfills the eight
existing rows in the same statement — no data migration step, satisfying FR-KIND-1.

The twelve modification seeds append to the existing `seeds` slice; `Seed`'s `FirstOrCreate` is
keyed by `Name` and already carries `Attrs`, so `Kind` joins `Description`/`SystemDefined` in the
`Attrs(...)` literal. Existing rows are untouched. `TestSeedIsIdempotent` asserts `len(seeds)`
rather than a hard-coded 8, so it passes unchanged if written that way; if it hard-codes 8, it is
updated to `len(seeds)` rather than to 20, so the next seed addition does not break it again.

### D2 — the record's kind filter uses an injected `CategoryAccessor`, mirroring `VehicleAccessor`

`maintenancerecord` already imports `vehicle` for `vehicle.Model` and injects a one-method
`VehicleAccessor` interface satisfied by `*vehicle.Processor` (`maintenancerecord/resource.go:20`).
The kind filter follows that precedent exactly rather than inventing a new one:

```go
// maintenancerecord/resource.go
type CategoryAccessor interface {
    IDsByKind(kind maintenancecategory.Kind) ([]string, error)
}
```

satisfied by a new `maintenancecategory.Processor.IDsByKind`, backed by
`Provider.IDsByKind(kind)` (`SELECT id FROM fleet.maintenance_categories WHERE kind = ?`).
The type-level import of `maintenancecategory.Kind`/`ParseKind` keeps parsing and the permitted
value set in the domain that owns them, so the two endpoints cannot drift on what `?kind=` accepts.

Rejected: reaching into `maintenancecategory.NewProvider(db)` from inside `maintenancerecord`
(couples to another domain's data access), and a raw SQL join in the record provider (same, plus it
buries the vocabulary in a query string).

### D3 — an empty category set filters to nothing, never to everything

`category_id IN ()` is not valid SQL, and a naive "skip the clause when the slice is empty" guard
silently degrades a filtered request into an unfiltered one. The provider takes
`categoryIDs []string` where **`nil` means no filter** and **empty-but-non-nil means match nothing**
— returning `([], 0, nil)` without touching the database. `meta.total` is therefore 0, satisfying
FR-LIST-2 in the degenerate case as well as the normal one.

This matters more than it looks: it is the difference between "a fleet with no modifications sees an
empty modification tab" and "a fleet with no modifications sees every maintenance record labelled as
a modification".

### D4 — `description` validation lives in a domain `Validate`, called from both write paths

The 200-character rule has to hold on `POST` (which goes through `Builder.Build`) and on `PATCH`
(which goes through `Processor.Update` and never touches the builder). Putting the check in the
handler duplicates it; putting it only in the builder leaves `PATCH` unguarded.

```go
// maintenancerecord/model.go
const MaxDescriptionRunes = 200

// Validate enforces the model's invariants. Called by Builder.Build and by
// Processor.Update after the mutation function is applied, so every write path
// is covered by construction.
func Validate(m Model) error
```

`Builder.Build()` becomes `Validate(b.m)` plus its existing required-field checks;
`Processor.Update` calls `Validate(apply(m))` before handing the model to the administrator.
Length is measured in **runes** (`utf8.RuneCountInString`), not bytes — a 200-character limit that
rejects 60 emoji is a bug, not a security control. The value is trimmed of surrounding whitespace
before measuring, consistent with the client's `z.string().trim()`.

`Administrator.Update` gains `"description"` in its `Updates(map[string]any{…})` literal; without it
the column would round-trip through the model and silently not persist.

### D5 — `performedAt` becomes required by deleting the default, not by adding a check

Today `resource.go:85` defaults to `time.Now().UTC()` when the attribute is empty. The change is to
delete those two lines and let the existing `time.Parse` failure path produce `422`, plus an
explicit empty-string check ahead of it. `Builder.Build` already rejects a zero `performedAt`, so
the invariant is enforced twice — deliberately: the handler gives the accurate status code, the
builder guarantees no code path can construct a dateless record.

This is the one deliberate behaviour break in the task. The only caller is `apps/web`, which always
sends the field. Documented in §9.

### D6 — cross-service media validation via one batch internal endpoint

FR-DOC-6 requires fleet-service to prove that every `documentMediaIds` entry belongs to the caller's
active fleet. fleet-service does not own the media table. Options:

| Option | Round trips | New surface | Verdict |
|---|---|---|---|
| **A. Batch internal endpoint on media-service** | 1 | 1 route + 1 client | **chosen** |
| B. Per-ID internal endpoint | N | 1 route + 1 client | same surface, N× the latency |
| C. Forward the caller's bearer token to media-service's public API | N | `shared-go/auth` must retain the raw token | widens the auth surface for every service to serve one caller |
| D. fleet-service reads `media.media_objects` directly | 0 | — | cross-service DB read; explicitly forbidden |
| E. Trust the client | 0 | — | this is the security requirement |

Chosen shape:

```
GET /internal/media?fleet_id=<uuid>&ids=<id>,<id>,…
200 → {"media": [{"id": "…", "status": "ready", "content_type": "application/pdf"}, …]}
```

The response contains **only** the requested IDs that are active (not soft-deleted) **and** belong
to `fleet_id`. fleet-service compares the returned set against the requested set; any missing ID —
whether it does not exist, was deleted, or belongs to another fleet — is indistinguishable, and all
three produce the same `422`. That is exactly the non-disclosure property `api-contracts.md` §3 asks
for, and it falls out of the endpoint's shape rather than needing handler-side care.

`fleet_id` is a **parameter**, not a trusted assertion: media-service filters by it. fleet-service
passes the value from the JWT's `active_fleet_id`; media-service never widens the result set on the
caller's say-so.

New package `apps/fleet-service/internal/mediaclient`, modelled directly on
`apps/notification-service/internal/fleetclient` — a `base` URL, an `*http.Client`, one method,
`getJSON`, non-200 becomes an error. Configured by `MEDIA_INTERNAL_URL`
(`http://media-service:8080`), the same way `FLEET_INTERNAL_URL` is.

The processor gains a `DocumentValidator` interface so the domain does not import an HTTP client:

```go
// maintenancerecord — satisfied by *mediaclient.Client
type DocumentValidator interface {
    ValidateOwnership(ctx context.Context, fleetID string, mediaIDs []string) error
}
```

`nil` is a legal value, meaning "no validator wired" — used by the existing unit tests and by any
future caller that has already validated. The `POST` handler skips the call entirely when
`len(documentMediaIds) == 0`, so the common case (logging an oil change with no receipt) makes no
cross-service call at all.

### D7 — media-service unreachable means the record is not created

`ValidateOwnership` returning a transport error propagates unchanged, so `StatusFor` maps it to
`500` and no record is written. The alternative — create the record and drop the attachments, or
create it with unvalidated IDs — trades a visible failure for a silent one, on the exact path the
check exists to protect. Failing closed is correct here even though it couples record creation to
media-service availability, because the only requests affected are those that carry attachments.

### D8 — ownership is validated; `ready` is not required

The validator checks "active and in this fleet". It deliberately does not require
`status == "ready"`.

Requiring `ready` would reject a legitimate save when a JPEG's variant worker is a second behind the
user's click. FR-DOC-5 already gates the submit button on the client for UX; making the server
enforce it converts a worker-latency hiccup into a lost form. Ownership is the security property;
readiness is a UX property. They are enforced in different places on purpose.

A consequence worth stating: a record can reference a media object that later reaches
`failed` (D13). FR-VIEW-4's "unavailable" row is what the user sees, which is the same treatment a
deleted attachment gets. No new state to handle.

### D9 — at most 10 attachments per record

Not in the PRD; added here. It bounds three things at once: the `ids=` query string on the internal
endpoint (10 UUIDs ≈ 370 bytes, far inside any URL limit), the per-record fan-out when an
attachment list is expanded, and the size of a single `InsertTx` document loop. `MaxDocuments = 10`
lives beside `MaxDescriptionRunes`; over-limit is `422`. The client's picker stops accepting files
at 10 and says why.

### D10 — content types are normalised at the door and stored normalised

Browsers send `text/csv`, but they also send `text/csv; charset=utf-8`, and `APPLICATION/PDF` is a
legal spelling. A naive `allowed[ct]` map lookup rejects the first two and accepts nothing useful.

`InitUpload` runs the client string through `mime.ParseMediaType`, lowercases the media type,
**discards the parameters**, and matches the bare type against the allowlist. The bare, normalised
type is what is persisted. Two consequences fall out for free:

- `GET /media/{id}/content` can no longer echo an arbitrary client string as `Content-Type` for any
  row created after this change, because no such string was ever stored. FR-DL-4's octet-stream
  fallback then only has to cover *legacy* rows, which is what it is for.
- The download `Content-Type` is byte-identical to an allowlist entry, so `nosniff` has something
  unambiguous to pin.

An unparseable, empty, or non-allowlisted type is `415` — `ErrUnsupportedMediaType` — logged at
`warn` with the offending type, the fleet ID and the user ID (never the bytes, never the filename),
per the PRD's observability requirement.

### D11 — the allowlist is a value object, not a package-level map

```go
// mediaobject/contenttype.go
type Class int
const (
    ClassUnknown Class = iota  // not on the allowlist
    ClassImage                 // renderable: the worker's image.Decode handles it
    ClassDocument              // everything else on the allowlist
)

type Allowlist struct { allowed map[string]Class }

func ParseAllowlist(csv string) (Allowlist, error)   // from MEDIA_ALLOWED_CONTENT_TYPES
func (a Allowlist) Normalize(raw string) (string, bool)
func (a Allowlist) Classify(contentType string) Class
```

`ClassImage` is **not** configurable — it is exactly `{image/jpeg, image/png}`, hard-coded, because
it is a statement about what `image.Decode` in `processing/worker.go` can actually do, not a
deployment preference. A config key that could add `image/heic` to the renderable set would let an
operator hand the worker bytes it cannot decode. Anything on the configured allowlist that is not in
that fixed pair is `ClassDocument`. Adding a new document type is a ConfigMap edit; adding a new
*renderable* type is a code change, which is correct, because it needs a decoder.

The `Allowlist` is constructed once in `cmd/main.go` and injected into `NewProcessor`. It is a pure
value with no dependencies, so `Classify` and `Normalize` are unit-tested directly with no database,
no HTTP and no MinIO.

### D12 — documents short-circuit `Confirm` with a new transition guard

`MarkReady` (processing → ready) is left untouched so the worker's tests and behaviour are
unchanged. A second guard is added:

```go
// MarkReadyDirect transitions uploaded → ready for objects that need no
// processing (documents). Any other source state is a conflict.
func MarkReadyDirect(m Model) (Model, error)
```

`Confirm` branches on `pr.allow.Classify(m.ContentType())`:

- `ClassImage` → existing path: `MarkProcessing` + outbox enqueue in one transaction.
- `ClassDocument` **or** `ClassUnknown` → `MarkReadyDirect`, plain `a.Update`, **no outbox row**.

Folding `ClassUnknown` in with documents is the safe default for legacy rows: a pre-allowlist object
with a content type nobody recognises must never be handed to `image.Decode`. Legacy JPEG/PNG rows
still classify as `ClassImage` — their stored type is on the allowlist — so nothing regresses.

The web client's poll-until-ready loop resolves on the first read for documents because `Confirm`
already returns `"status": "ready"`.

### D13 — FR-MEDIA-5: classify failures as permanent or transient; add `StatusFailed`

**The PRD's premise is wrong and this is the design's most consequential correction.** FR-MEDIA-5
says "after the existing consumer retry budget is exhausted". There is no retry budget.
`packages/shared-go/events/consumer.go` does this:

```go
if err := h(ctx, e); err != nil {
    log.WithError(err).Error("handler failed; will retry")
    continue // do not commit → redelivery
}
```

A handler that keeps returning an error loops forever without committing, and because it never
advances the offset it **blocks the partition for every other media object behind it**. A single
corrupt PDF today stalls variant generation for every image in that partition. Adding a budget to
`events.Consume` would mean tracking delivery counts across all four services' consumers — far more
blast radius than this task should carry, and `kafka-go` does not expose a delivery count, so it
would need its own attempt ledger.

The fix is better than a budget anyway: **retry only errors that can plausibly succeed later.**

| Failure | Classification | Action |
|---|---|---|
| `image.Decode` returns an error | permanent — the bytes will not become decodable | → `failed`, `MarkProcessed`, commit |
| `storage.ErrObjectNotFound` on the original | permanent — the PUT never landed; it never will | → `failed`, `MarkProcessed`, commit |
| MinIO unreachable / PUT of a variant fails | transient | return error → redeliver (today's behaviour) |
| database error | transient | return error → redeliver |

```go
// mediaobject/model.go
StatusFailed Status = "failed"

// MarkFailed is terminal: it accepts uploaded or processing and is the only
// transition out of the pipeline that is not "ready".
func MarkFailed(m Model) (Model, error)
```

Chosen over "`ready` with zero variants" (the PRD's other candidate) because `ready` is a promise
the object cannot keep — every consumer of `ready` assumes displayable bytes, and an object that
lies about its state is a bug that surfaces far from its cause. The cost is one new status string
the client must handle, and the client surface is small: `MediaThumbnail` renders the existing
error state, the upload poll stops instead of spinning, and `AttachmentRow` shows the FR-VIEW-4
unavailable row. Three call sites, all of which already have a failure branch.

The permanent path logs at `error` with the media ID and the decode error, per the PRD's
observability requirement.

### D14 — `Content-Disposition` is built by a pure function

The acceptance criterion "a filename containing quotes, newlines or non-ASCII characters cannot
corrupt or inject the header … covered by a unit test" is only cheap to satisfy if the logic is not
inside an HTTP handler.

```go
// mediaobject/download.go
// ContentDisposition renders the header value for a download. filename is
// untrusted input from the uploader.
func ContentDisposition(class Class, filename, fallback string) string
```

Sanitisation order, all in this one function:

1. Strip every character below `0x20` plus `0x7F` — this is what makes header injection
   structurally impossible, and it comes first so nothing downstream sees a CR or LF.
2. Take the base name only (drop anything up to the last `/` or `\`), so a filename cannot suggest
   a path to the client.
3. Build the ASCII form: escape `\` and `"`, replace every non-ASCII rune with `_`.
4. If the sanitised name is empty, use `fallback` (the media ID).
5. Emit `disposition; filename="<ascii>"`, and when the original contained non-ASCII also append
   `; filename*=UTF-8''<pct>`, where `<pct>` percent-encodes everything outside RFC 5987's
   `attr-char` set.

The RFC 5987 encoder is written by hand (~15 lines). `url.QueryEscape` is wrong — it encodes space
as `+`; `url.PathEscape` is wrong — it leaves `?`, `&` and `=` unescaped. Both produce headers that
mostly work, which is the worst outcome for a security-adjacent function.

`ContentDisposition` is called with `attachment` for `ClassDocument` and `ClassUnknown`, `inline`
for `ClassImage`.

### D15 — the download `Content-Type` is re-resolved through the allowlist on every read

```go
ct, class := allow.Resolve(m.ContentType())   // returns ("application/octet-stream", ClassUnknown) when off-list
w.Header().Set("Content-Type", ct)
w.Header().Set("X-Content-Type-Options", "nosniff")
w.Header().Set("Content-Disposition", ContentDisposition(class, m.OriginalFilename(), m.ID()))
w.Header().Set("Cache-Control", "private, max-age=300")
```

Re-resolving on read rather than trusting the stored value means shrinking the allowlist
retroactively downgrades already-stored objects to `attachment` + `octet-stream` on the next
request. That is the behaviour an operator who removes a type from the list expects, and it is what
protects rows created before D10 existed.

`nosniff` is set on **every** response including images, per FR-DL-1. That is safe for the gallery:
`inline` + a correct `image/jpeg` renders normally in an `<img>`; `nosniff` only stops the browser
from second-guessing the declared type.

### D16 — pending attachment state lives in a hook, not in the form component

`MaintenanceRecordForm` is already 180 lines of `react-hook-form` wiring. Adding upload
orchestration, per-file status, per-file error text and orphan cleanup inline would roughly double
it and make none of it testable without rendering a form.

```ts
// lib/hooks/usePendingAttachments.ts
interface PendingAttachment {
  localId: string;                                 // crypto.randomUUID, stable across re-render
  file: File;
  status: 'uploading' | 'ready' | 'failed';
  mediaId?: string;
  error?: string;
}

function usePendingAttachments(): {
  items: PendingAttachment[];
  add(files: FileList | File[]): void;
  remove(localId: string): void;                   // soft-deletes the media object if uploaded
  commit(): string[];                              // returns mediaIds; disarms unmount cleanup
  mediaIds: string[];                              // ready items only
  isUploading: boolean;
};
```

The form owns one instance, renders `items`, disables submit while `isUploading` (FR-DOC-5), and
calls `commit()` in its submit handler to obtain the IDs. `documentMediaIds` stays on the Zod schema
for shape compatibility but is populated from `commit()` at submit time rather than being a
controlled field — a `useFieldArray` of IDs would put upload state into form state, where
`react-hook-form` would try to validate and reset it.

A failed upload keeps its row with the filename and the reason and is simply absent from `mediaIds`,
which is exactly FR-DOC-4: the save proceeds with what succeeded.

### D17 — orphan cleanup: explicit removal, unmount cleanup, purge backstop

Three layers, in order of reliability:

1. `remove(localId)` issues `DELETE /api/media/{id}` when the item had reached `ready` (FR-DOC-2).
2. An unmount effect deletes every `ready` item that was never committed (FR-DOC-3). `commit()` sets
   a ref that disarms this, so a successful save never deletes the media it just attached.
3. The existing 5-day `purge_after` sweep catches everything else.

No `beforeunload` handler. It cannot reliably issue authenticated requests during teardown, it is
the kind of thing that survives in a codebase long after it stops working, and layer 3 already
covers the case. The PRD names client cleanup as best-effort and the sweep as authoritative; this
implements exactly that and no more.

### D18 — the blob-download helper lives in `apps/web`, not `packages/shared-ts`

Deviation from PRD §7. `packages/shared-ts` is transport and typing — `apiClient`, `jsonapi`,
`errors` — and its only DOM dependency is `fetch`, which exists outside browsers.
`downloadBlob` needs `document.createElement('a')`, `URL.createObjectURL` and a click, none of which
exist off the DOM. Putting it in `shared-ts` makes the package browser-only for the sake of eleven
lines. It goes in `apps/web/src/lib/utils/download.ts`:

```ts
export function downloadBlob(blob: Blob, filename: string): void
```

Create the object URL, click a detached anchor, `revokeObjectURL` in a `setTimeout(…, 0)` so the
navigation has started. Unit-tested against jsdom.

### D19 — kind badges and grouping are resolved client-side

FR-LIST-3 already settles this: the record resource gains no `kind` field. `useMaintenanceCategories`
caches all 20 categories for 10 minutes; the section builds a `Map<categoryId, MaintenanceCategory>`
once with `useMemo` and every badge, group header and filter label reads from it. No per-row fetch,
no new endpoint, no denormalised column that can go stale.

Two entry points ("Log Record" / "Log Modification") pass a `kind` prop to the form, which filters
the category picker to that kind. The picker is not grouped by kind, because it is never showing
more than one kind at a time.

### D20 — media-service's `/internal` surface must be denied at the edge (security-critical)

`deploy/k8s/overlays/main/ingressroute.yaml` carries a priority-200 `internal-deny` route for
fleet-service, with a long comment explaining that `/internal/*` is registered without JWT on the
assumption that it is unreachable from outside. Adding `/internal/media` to media-service inherits
that assumption and therefore inherits the obligation.

A mirrored route is required:

```yaml
- match: (Host(`myfleet.tumidanski.com`) || Host(`myfleet.tumidanski.me`) || Host(`myfleet.home`)) && PathRegexp(`(?i)^/+api/+media[^/]*/*internal`)
  kind: Rule
  priority: 200
  middlewares:
    - name: internal-deny
  services:
    - name: media-service
      port: 8080
```

The regex mirrors the fleet-service one for the same reasons documented there: Traefik normalises
the path before matching, and `media-stripprefix` removes the literal string `/api/media` rather
than a path segment, so `/api/mediainternal/media?...` would otherwise reach the handler as
`/internal/media`. It goes in the `myfleet-routes` object only — the `replacements` block copies
`spec.routes` onto the TLS twin, so both entrypoints get it and cannot drift.

Without this rule the endpoint is an unauthenticated cross-fleet media-existence oracle on the
public internet. **This is not an optional deployment detail; it ships in the same change as the
endpoint.** The local overlay needs no equivalent — it has no `internal-deny` for fleet-service
either, and it is not internet-facing.

`deploy/compose/docker-compose.yml` and `.env.example` gain `MEDIA_INTERNAL_URL` for fleet-service
alongside the existing `FLEET_INTERNAL_URL`.

### D21 — collapse the N+1 document fetch in `ListByVehicle` (targeted improvement)

`provider.go:55` loops over the page's records issuing one document query each: 26 queries for a
25-record page. It has been harmless because no record has ever had a document. This task makes
attachments the point, so it stops being harmless.

One query for the page's record IDs, grouped in memory:

```go
var docs []DocumentEntity
p.db.Where("maintenance_record_id IN ?", ids).Find(&docs)
byRecord := map[string][]DocumentEntity{}
```

Bounded by page size, so it is in scope and cheap. `GetByID` is left alone — it is already one
record, one query.

### D22 — `mileage` and `cost` become optional in the Zod schema (bug fix)

`lib/schemas/maintenanceRecord.ts` declares `mileage: z.number(…)` and `cost: z.number(…)` with no
`.optional()`, while the form's `defaultValues` set both to `undefined` and the number inputs write
`undefined` when cleared. A user who logs an oil change without a cost currently cannot submit —
`zodResolver` reports "Cost must be a number" on an untouched field. FR-REC-5 states both are
optional. Both gain `.optional()`. The existing `?? 0` coercions in
`VehicleMaintenanceSection.handleCreateRecord` already handle the undefined case on the way out.

Also in this file: `description: z.string().trim().max(200).optional().or(z.literal(''))`, matching
the server's rune-based limit closely enough for a client-side affordance, and `performedAt` keeps
its existing `.min(1)` — now backed by a server that agrees.

### D23 — the category list request becomes explicit about page size

`GET /maintenance-categories` is paged with `server.ParsePage`, whose default size is 25.
`maintenanceCategoryService.list()` sends no page parameters, so it currently gets 25 of 8. After
seeding it will get 20 of 20 — correct, but five rows from silently truncating the picker the next
time someone adds categories, in a way that would look like "the new category didn't seed".

`list()` sends `page[size]=100` explicitly. One line, removes a latent trap.

---

## 4. Component design

### 4.1 `packages/shared-go/server`

```go
ErrUnsupportedMediaType = errors.New("unsupported media type") // 415
```

plus its `StatusFor` arm between `413` and `422`. `errors_test.go` gains the case. Nothing else in
shared-go changes.

### 4.2 `apps/fleet-service/internal/maintenancecategory`

| File | Change |
|---|---|
| `model.go` | `Kind` type, two constants, `ParseKind`, `Model.kind` + accessor |
| `entity.go` | `Entity.Kind` column; `Make` maps it; twelve seeds; `Attrs` carries `Kind` |
| `provider.go` | `List(kind Kind, page)`; new `IDsByKind(kind Kind) ([]string, error)` |
| `processor.go` | passthrough for both |
| `resource.go` | parse `?kind=`; `422` on an unrecognised value |
| `rest.go` | `Kind string \`json:"kind"\`` — no `omitempty`, it is `NOT NULL` |

### 4.3 `apps/fleet-service/internal/maintenancerecord`

| File | Change |
|---|---|
| `model.go` | `description` field + accessor + `WithDescription`; `MaxDescriptionRunes`, `MaxDocuments`, `Validate` |
| `entity.go` | `Description string \`gorm:"type:varchar(200)"\``; `Make`/`ToEntity` map it |
| `builder.go` | `SetDescription`; `Build` delegates to `Validate` |
| `provider.go` | `ListByVehicle(vehicleID, categoryIDs []string, page)` (D3); batched document fetch (D21) |
| `processor.go` | `Validate` in `Update`; `Create` unchanged |
| `administrator.go` | `"description"` in the `Updates` map |
| `resource.go` | `CategoryAccessor` + `DocumentValidator` params; `?kind=` on GET; `description` on POST/PATCH; required `performedAt`; ownership validation on POST |
| `rest.go` | `Description string \`json:"description,omitempty"\`` |

### 4.4 `apps/fleet-service/internal/mediaclient` (new)

One file, ~70 lines, structurally identical to `notification-service/internal/fleetclient`:
`Client{base, hc}`, `NewClient(base)`, `ValidateOwnership(ctx, fleetID, ids)`, `getJSON`.
`ValidateOwnership` returns `server.ErrValidation` when the returned set is smaller than the
requested set, and the transport error otherwise (D7). Empty `ids` returns `nil` without a request.

### 4.5 `apps/media-service/internal/mediaobject`

| File | Change |
|---|---|
| `contenttype.go` **(new)** | `Class`, `Allowlist`, `ParseAllowlist`, `Normalize`, `Classify`, `Resolve` |
| `download.go` **(new)** | `ContentDisposition`, the RFC 5987 encoder, the sanitiser |
| `model.go` | `StatusFailed` |
| `processor.go` | `allow Allowlist` field; `InitUpload` validates + normalises → `415`; `Confirm` branches; `MarkReadyDirect`, `MarkFailed` |
| `resource.go` | download headers; new `InitializeInternalRoutes(log, db)` for `GET /internal/media` |
| `rest.go` | unchanged — `status` is already a plain string |

`InitializeInternalRoutes` follows `fleet-service/internal/membership`'s convention exactly,
including the comment that it must be registered **without** JWT middleware, and it lives beside a
pointer to D20's Traefik rule so the two are not separable in a reader's mind.

### 4.6 `apps/media-service/internal/processing`

`handle` gains the permanent/transient split from D13. `decodeOriginal` returns a sentinel-wrapped
error so the caller can distinguish a decode failure from a storage failure:

```go
var ErrPermanent = errors.New("permanent processing failure")
```

`decodeOriginal` wraps both `image.Decode` failures and `storage.ErrObjectNotFound` with
`ErrPermanent`; `handle` checks `errors.Is(err, ErrPermanent)` and takes the terminal path. The
happy path and the already-ready short-circuit are untouched, so the existing worker tests keep
passing unmodified.

### 4.7 `apps/media-service/cmd/main.go`

```go
allow, err := mediaobject.ParseAllowlist(config.Get("MEDIA_ALLOWED_CONTENT_TYPES", defaultAllowlist))
```

Fatal on a parse error — a malformed allowlist must not boot into "allow nothing" or "allow
everything". The `Allowlist` is passed to `InitializeRoutes`. `InitializeInternalRoutes` is
registered on the unauthenticated router group, mirroring fleet-service's wiring.

### 4.8 `apps/web`

| Path | Change |
|---|---|
| `lib/hooks/api/media.ts` | extract `useMediaUpload(opts?)`; `useUploadMedia(vehicleId)` becomes a wrapper; `useDeleteMedia` gains a fleet-agnostic sibling |
| `lib/hooks/usePendingAttachments.ts` **(new)** | D16 |
| `lib/utils/download.ts` **(new)** | D18 |
| `lib/schemas/maintenanceRecord.ts` | `description`; `mileage`/`cost` optional (D22) |
| `lib/hooks/api/maintenance.ts` | `useMaintenanceCategories(kind?)`, `useMaintenanceRecords(vehicleId, kind?)` — `kind` in both query keys |
| `services/api/MaintenanceCategoryService.ts` | `page[size]=100` (D23), `?kind=` |
| `types/models/maintenanceCategory.ts` | `kind: 'maintenance' \| 'modification'` |
| `types/models/maintenanceRecord.ts` | `description?` on all three interfaces |
| `types/models/media.ts` | `status` union gains `'failed'` |
| `…/maintenance/MaintenanceRecordForm.tsx` | description field, `AttachmentPicker`, submit gating, `kind` prop |
| `…/maintenance/AttachmentPicker.tsx` **(new)** | pending list, per-item remove, per-item error |
| `…/maintenance/RecordAttachmentList.tsx` **(new)** | expanded-record view; `AttachmentRow` per media ID |
| `…/maintenance/VehicleMaintenanceSection.tsx` | two entry points, kind filter, kind badges, description as the primary line, attachment count + expansion |

`RecordAttachmentList` is rendered only for the expanded record, which is what keeps a 25-record
page from issuing 25 × N metadata requests (the performance NFR). `AttachmentRow` uses
`useMediaObject(id)` for metadata and branches: `ClassImage`-ish content type → `MediaThumbnail`;
anything else → a download button; `isError`, soft-deleted, or `status === 'failed'` → the
unavailable row (FR-VIEW-4). All of it renders for viewers; only the picker and the remove controls
are behind `canWrite` (FR-VIEW-5).

---

## 5. Data flow

**Logging a modification with a PDF invoice**

```
user picks invoice.pdf
  └ usePendingAttachments.add
      POST /api/media            {contentType:"application/pdf"}  → 201 uploaded
      PUT  /api/media/{id}/content                                → 200
      POST /api/media/{id}/confirm                                → 200 ready   ← no Kafka
      item.status = 'ready'                              (submit re-enabled)

user submits
  └ commit() → ["m-1a2b"]
      POST /api/fleet/vehicles/{id}/maintenance-records
           {categoryId, description, performedAt, documentMediaIds:["m-1a2b"]}
        fleet-service
          authz: same fleet + write
          GET http://media-service:8080/internal/media?fleet_id=F&ids=m-1a2b
            → {"media":[{"id":"m-1a2b",…}]}    set matches → proceed
          Validate(model)  → insert record + document rows in one tx
        → 201
```

If the internal call returns a smaller set: `422`, no record, no partial write — the ownership check
runs before `Create`, so there is nothing to roll back.

**Downloading it later**

```
expand record → RecordAttachmentList
  GET /api/media/m-1a2b                → contentType application/pdf, status ready
  (not an image → download button)
  click → mediaService.getContentBlob   (Authorization header applied by apiClient)
      ← Content-Type: application/pdf
        Content-Disposition: attachment; filename="invoice.pdf"
        X-Content-Type-Options: nosniff
  downloadBlob(blob, "invoice.pdf")
```

---

## 6. Error handling

| Condition | Where | Result |
|---|---|---|
| `?kind=bogus` | `ParseKind` | `422` |
| no categories of a kind | provider | empty page, `total: 0` (D3) |
| `description` > 200 runes | `Validate` | `422`, never truncated |
| `performedAt` missing/unparseable | POST handler | `422` |
| > 10 `documentMediaIds` | `Validate` | `422` |
| media ID foreign / unknown / deleted | `mediaclient` | `422`, indistinguishable (D6) |
| media-service unreachable | `mediaclient` | `500`, no record (D7) |
| content type off-allowlist | `InitUpload` | `415` + `warn` log naming the type |
| body > 25 MiB | existing `MaxBytesReader` | `413`, unchanged |
| undecodable image bytes | worker | `failed`, event marked processed, `error` log (D13) |
| MinIO down mid-variant | worker | error returned → redelivery (D13) |
| legacy content type on download | `Allowlist.Resolve` | `octet-stream` + `attachment` |
| attachment upload fails | `usePendingAttachments` | row shows filename + reason; save still allowed |
| attachment media unavailable | `AttachmentRow` | explicit unavailable row |

---

## 7. Testing strategy

Pure functions carry the security-critical logic precisely so they can be tested without
infrastructure.

**Unit, no dependencies** — `ContentDisposition` (quotes, backslash, CR/LF, `0x7F`, path
separators, non-ASCII → `filename*`, empty → media ID), `Allowlist.Classify`/`Normalize`
(parameters, casing, unknown, empty), `ParseAllowlist` (malformed input), `ParseKind`,
`maintenancerecord.Validate` (rune counting with multi-byte input, attachment cap), `ResizeDims`
(existing).

**Unit with in-memory SQLite** (existing `worker_test.go` pattern) — `IDsByKind`; `ListByVehicle`
with `nil` vs empty vs populated `categoryIDs`, asserting the filtered `total` across more than one
page; `Seed` idempotency at `len(seeds)`.

**Handler tests with `httptest`** (existing `mediaobject/resource_test.go` pattern) — `415` on
`text/html` and `200` on `application/pdf` for `POST /media`; PDF `confirm` yields `ready` with no
outbox row; download headers for a PDF, a JPEG and a legacy row; `GET /internal/media` returns only
same-fleet active IDs.

**Worker** — undecodable bytes reach `StatusFailed` **and** `MarkProcessed`, proving the event is
committed and cannot redeliver; a storage error does **not** mark processed, proving transient
failures still retry.

**`mediaclient`** — against an `httptest.Server`: full match passes, short set is
`server.ErrValidation`, non-200 propagates, empty input makes no request.

**Frontend (vitest)** — `usePendingAttachments` (add/remove/commit, failed upload keeps the row and
is excluded from `mediaIds`, unmount deletes uncommitted items, `commit` disarms cleanup);
`downloadBlob` against jsdom; the record form (submit disabled while uploading, description
validation); `RecordAttachmentList` (image → thumbnail, document → download, error → unavailable).

**Manual, before the PR** — a real PDF end-to-end at 25 MiB−1 and 25 MiB+1; an HEIC upload showing a
clear `415` rather than today's silent hang; a viewer-role session confirming download works and no
write control renders.

---

## 8. Rollout and compatibility

Every schema change is additive with a default, so deploy order does not matter for correctness.
The order that minimises visible breakage is:

1. **shared-go + media-service.** The allowlist begins rejecting non-allowlisted types immediately;
   the current web client only ever uploads JPEG/PNG, so nothing user-visible changes. Download
   headers and the terminal processing state take effect at once. `/internal/media` ships together
   with D20's Traefik rule — never separately.
2. **fleet-service.** Needs `MEDIA_INTERNAL_URL` present before it starts validating. The
   `performedAt` change lands here; the deployed web client already sends it.
3. **web.** The only step that requires the previous two.

Deploying web first would surface `415`s on PDF uploads; deploying fleet-service before
media-service's internal route would make attachment-bearing saves fail with `500`. Neither corrupts
data.

Rollback is clean in the same reverse order. The `failed` status is the one asymmetry: objects that
reached it while the new code was live remain `failed` after a rollback, and an old client renders
them as a spinning poll rather than an error. Acceptable — it requires both a corrupt upload and a
rollback in the same window.

---

## 9. Deviations from the PRD

| # | PRD | This design | Why |
|---|---|---|---|
| 1 | FR-MEDIA-5 "after the existing consumer retry budget is exhausted" | No retry budget exists; classify failures as permanent vs transient instead (D13) | `events.Consume` retries forever and blocks the partition. Retrying only what can succeed is a better fix than a budget, and does not touch a shared consumer used by four services. |
| 2 | §9 open question 2 left open | Resolved: new `StatusFailed` (D13) | `ready` with zero variants makes the status field lie; three client call sites already have a failure branch. |
| 3 | §7 "shared-ts — download-blob helper" | Lives in `apps/web/src/lib/utils/download.ts` (D18) | It is DOM-only; `shared-ts` is otherwise DOM-agnostic. |
| 4 | Silent on attachment count | Hard cap of 10 per record (D9) | Bounds the internal-endpoint URL, the expanded-row fan-out, and the insert loop. |
| 5 | Silent on content-type parameters | Normalised via `mime.ParseMediaType`, stored normalised (D10) | `text/csv; charset=utf-8` is what browsers actually send; a bare map lookup would reject it. |
| 6 | Silent on the Traefik edge | `internal-deny` route for media-service is part of this change (D20) | Otherwise the new endpoint is a public unauthenticated media-existence oracle. |
| 7 | Not mentioned | Zod `mileage`/`cost` become optional (D22) | Existing bug that blocks submitting a record without a cost; FR-REC-5 says both are optional. |
| 8 | Not mentioned | `ListByVehicle` document fetch batched (D21) | 26 queries per page, about to start mattering. |
| 9 | Not mentioned | Category list requests `page[size]=100` (D23) | 20 of a default 25 is five rows from silent truncation. |

## 10. Open questions carried forward

1. **Post-hoc attachment editing** stays out of scope (PRD §9.1). Attaching the wrong receipt still
   means deleting and re-creating the record. Nothing in this design blocks a later
   `PATCH documentMediaIds`: the ownership validator is already a reusable interface, and the
   document rows are a separate table with no ordering or uniqueness constraints to unwind.
2. **HEIC** stays off the allowlist. The improvement this task delivers is that it now fails fast
   with `415` and a message naming the accepted types, rather than hanging in `processing` forever.
3. **Per-class upload caps** — 25 MiB remains shared between receipts and vehicle photos. No change.
4. **The twelve modification category names** are a first guess and are system-defined, so revising
   them is a follow-up seed change plus a rename migration, not a user workaround.
