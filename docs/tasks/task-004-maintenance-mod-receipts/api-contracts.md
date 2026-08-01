# API Contracts — task-004

Companion to `prd.md` §5. Exact request/response shapes for every changed endpoint.

All request bodies follow the existing `server.RegisterInputHandler` envelope
(`packages/shared-go/server/handler.go:42`) — attributes are nested under `data.attributes`.
All responses follow `server.Document` / `server.Resource`. Errors use the `APIError` envelope in
`packages/shared-go/server/errors.go`.

Status codes available today: `401`, `403`, `404`, `409`, `410`, `413`, `422`. This task adds
`415` via a new `ErrUnsupportedMediaType` sentinel.

---

## 1. `GET /api/fleet/maintenance-categories`

**Changed:** optional `kind` filter; `kind` added to attributes.

Query parameters:

| Name | Required | Values | Behaviour |
|---|---|---|---|
| `kind` | no | `maintenance`, `modification` | Omitted → all categories. Any other value → `422`. |

Response `200`:

```json
{
  "data": [
    {
      "type": "maintenanceCategories",
      "id": "6f1c…",
      "attributes": {
        "name": "Oil Change",
        "description": "",
        "systemDefined": true,
        "kind": "maintenance"
      }
    },
    {
      "type": "maintenanceCategories",
      "id": "b204…",
      "attributes": {
        "name": "Exhaust",
        "description": "",
        "systemDefined": true,
        "kind": "modification"
      }
    }
  ]
}
```

`kind` is always present and non-empty — the column is `NOT NULL DEFAULT 'maintenance'`, so it is
never omitted and never `null`.

---

## 2. `GET /api/fleet/vehicles/{id}/maintenance-records`

**Changed:** optional `kind` filter; `description` added to attributes.

Query parameters:

| Name | Required | Values | Behaviour |
|---|---|---|---|
| `kind` | no | `maintenance`, `modification` | Omitted → all. Other → `422`. |
| `page[number]`, `page[size]` | no | existing `server.ParsePage` | Unchanged. |

The `kind` filter resolves the category IDs of that kind and constrains with
`category_id IN (…)`. `meta.total` is the count **after** filtering.

Response `200`:

```json
{
  "data": [
    {
      "type": "maintenanceRecords",
      "id": "a91f…",
      "attributes": {
        "vehicleId": "3c7e…",
        "categoryId": "b204…",
        "description": "Cat-back exhaust, Borla S-Type",
        "performedAt": "2026-03-14T00:00:00Z",
        "mileage": 48210,
        "cost": 1284.5,
        "vendor": "Redline Performance",
        "notes": "Kept the stock system in the garage.",
        "createdByUserId": "u-88…",
        "createdAt": "2026-03-15T18:02:11Z",
        "documentMediaIds": ["m-1a2b…", "m-9f0e…"]
      }
    }
  ],
  "meta": { "total": 1, "page": 1, "size": 25 }
}
```

`description` is emitted with `omitempty` semantics consistent with the existing `vendor`/`notes`
fields — absent when empty. Clients must treat an absent `description` as empty and fall back to
the category name (PRD FR-REC-2).

---

## 3. `POST /api/fleet/vehicles/{id}/maintenance-records`

**Changed:** accepts `description`; `performedAt` becomes required; `documentMediaIds` validated.

Request:

```json
{
  "data": {
    "attributes": {
      "categoryId": "b204…",
      "description": "Cat-back exhaust, Borla S-Type",
      "performedAt": "2026-03-14T00:00:00Z",
      "mileage": 48210,
      "cost": 1284.5,
      "vendor": "Redline Performance",
      "notes": "Kept the stock system in the garage.",
      "documentMediaIds": ["m-1a2b…", "m-9f0e…"]
    }
  }
}
```

| Field | Required | Constraint |
|---|---|---|
| `categoryId` | yes | Must resolve to an existing category. Either kind is accepted. |
| `performedAt` | yes | RFC3339. **Behaviour change** — previously defaulted to `time.Now().UTC()` when empty (`maintenancerecord/resource.go`). Now empty or unparseable → `422`. |
| `description` | no | ≤ 200 characters. Over-length → `422`, never truncated. |
| `mileage`, `cost` | no | Existing behaviour. |
| `vendor`, `notes` | no | Existing behaviour. |
| `documentMediaIds` | no | Every ID must exist and belong to the caller's active fleet → otherwise `422` and no record is created. |

Response `201` — same resource shape as §2.

Errors:

| Condition | Status | `code` |
|---|---|---|
| Missing/unparseable `performedAt` | `422` | `validation` |
| `description` > 200 chars | `422` | `validation` |
| Unknown `categoryId` | `422` | `validation` |
| `documentMediaIds` contains a foreign or unknown ID | `422` | `validation` |
| Caller not in the vehicle's fleet | `403` | `forbidden` |
| Caller lacks write role | `403` | `forbidden` |

The foreign-media case returns `422` rather than `403` deliberately: a `403` would confirm to the
caller that the media ID exists in some other fleet.

---

## 4. `PATCH /api/fleet/maintenance-records/{id}`

**Changed:** accepts `description`. Attachments remain immutable in this task.

Follows the existing pointer-per-field partial-update pattern — a field absent from the body is
left unchanged, and an explicit `null`/empty string clears it.

```json
{ "data": { "attributes": { "description": "Corrected: Borla ATAK, not S-Type" } } }
```

`documentMediaIds` is **not** accepted on `PATCH` (PRD §2 non-goals, §9 open question 1). Sending
it is ignored rather than erroring, consistent with how the handler already ignores unknown
attributes.

Response `200` — resource shape as §2.

---

## 5. `POST /api/media`

**Changed:** `contentType` validated against a server-side allowlist.

Request (unchanged shape):

```json
{ "data": { "attributes": { "contentType": "application/pdf", "originalFilename": "invoice.pdf" } } }
```

Default allowlist (`MEDIA_ALLOWED_CONTENT_TYPES`):

| Class | Content type | Variants generated | Disposition on download |
|---|---|---|---|
| Renderable image | `image/jpeg` | yes | `inline` |
| Renderable image | `image/png` | yes | `inline` |
| Document | `application/pdf` | no | `attachment` |
| Document | `application/vnd.openxmlformats-officedocument.wordprocessingml.document` | no | `attachment` |
| Document | `application/vnd.openxmlformats-officedocument.spreadsheetml.sheet` | no | `attachment` |
| Document | `text/csv` | no | `attachment` |

Errors:

| Condition | Status | `code` |
|---|---|---|
| `contentType` absent, empty, or not on the allowlist | `415` | `unsupported_media_type` |
| Caller lacks write role | `403` | `forbidden` |

The `415` `detail` names the accepted types so the client can render an actionable message. The
web client mirrors the list in its `accept` attribute as a convenience only — the server check is
authoritative.

---

## 6. `POST /api/media/{id}/confirm`

**Changed:** documents short-circuit to `ready`.

| Stored content type | Transition | Kafka event |
|---|---|---|
| Renderable image | `uploaded → processing` | `media.uploaded` published (unchanged) |
| Document | `uploaded → ready` | none published |

Response `200` — existing media resource. For a document the response already reads
`"status": "ready"`, so the client's existing poll-until-ready loop resolves on the first read
rather than waiting on a worker that would never run.

---

## 7. `GET /api/media/{id}/content`

**Changed:** adds `Content-Disposition` and `X-Content-Type-Options`.

Response headers:

| Header | Value |
|---|---|
| `Content-Type` | The stored type if it is on the allowlist, else `application/octet-stream`. |
| `Content-Disposition` | `inline` for renderable images; `attachment` for documents and for any stored type not on the allowlist. Always carries the escaped original filename. |
| `X-Content-Type-Options` | `nosniff` — on every response, both classes. |
| `Content-Length` | Existing behaviour (set when `size > 0`). |
| `Cache-Control` | `private, max-age=300` (unchanged). |

Filename escaping (PRD FR-DL-3):

- Strip CR, LF and all other control characters — a filename must never inject a header.
- Escape `"` and `\` inside the quoted `filename="…"` form.
- When the name contains non-ASCII, additionally emit `filename*=UTF-8''<percent-encoded>` per
  RFC 5987, keeping a sanitised ASCII `filename=` fallback for older clients.
- An empty or fully-stripped filename falls back to the media ID.

Example for a PDF:

```
Content-Type: application/pdf
Content-Disposition: attachment; filename="invoice.pdf"
X-Content-Type-Options: nosniff
Cache-Control: private, max-age=300
```

Example for a legacy row whose stored type is not on the allowlist:

```
Content-Type: application/octet-stream
Content-Disposition: attachment; filename="unknown.bin"
X-Content-Type-Options: nosniff
```

Errors are unchanged: `404` when the row exists but no bytes were ever PUT
(`mediaobject/resource.go` documents this case), `403` when the object is outside the caller's
active fleet.

---

## 8. Client upload sequence for a receipt

Unchanged in shape from the existing three-step flow in `performMediaUpload`
(`apps/web/src/lib/hooks/api/media.ts`), but the terminal step differs by class:

```
1. POST /api/media                    → { id, status: "uploaded" }
2. PUT  /api/media/{id}/content       → bytes
3. POST /api/media/{id}/confirm       → document: { status: "ready" }   ← done
                                        image:    { status: "processing" }
4. (images only) poll GET /api/media/{id} until status === "ready"
5. POST /api/fleet/vehicles/{id}/maintenance-records with documentMediaIds
```

Step 4 is skipped entirely for documents. Step 5 must not run until every attachment has reached
`ready`, which is what PRD FR-DOC-5 gates the submit button on.

If the user removes a pending attachment or abandons the form, the client issues
`DELETE /api/media/{id}` for each orphan (PRD FR-DOC-2, FR-DOC-3). The existing 5-day
`purge_after` sweep is the backstop when that best-effort cleanup does not run.
