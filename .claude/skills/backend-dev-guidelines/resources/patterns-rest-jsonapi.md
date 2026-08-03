---
title: REST and JSON:API Transport
description: Handler and transport conventions for JSON:API endpoints — chi routing, server.RegisterInputHandler for typed bodies, and the hand-rolled envelope in packages/shared-go/server.
---

# REST and JSON:API Pattern

## Principles

- Routes are registered on a **chi router**. Every route entry point in the tree
  returns `func(chi.Router)` (`apps/fleet-service/internal/vehicle/resource.go:28`)
  — 27 of them across 24 files: 19 `InitializeRoutes`, 7
  `InitializeInternalRoutes` (network-restricted endpoints; a domain may declare
  both), 1 `InitializePublicRoutes`.
- **Body-less routes** — GET, DELETE, and actions whose only input is the URL
  path — are a plain `func(w http.ResponseWriter, req *http.Request)` handed
  straight to `r.Get` / `r.Delete` / `r.Post`.
- **Bodied routes** — POST, PATCH, PUT — wrap that same func in
  `server.RegisterInputHandler`, which decodes `{"data":{"attributes":T}}` into
  a typed `T` before the handler runs
  (`packages/shared-go/server/handler.go:46-60`). It is a plain generic
  function, not a curried registrar, and it does no tracing or logging.
- **Handlers are thin — delegate business logic to the processor.**
- **Handlers call the processor, not the provider or administrator.**
- Success responses go out through
  `server.WriteJSON(w, status, server.Document{...})`.
- Errors go out through a single `server.WriteError(w, err)`;
  `server.StatusFor` maps the sentinel to a status. An explicit per-error
  `errors.Is` + `w.WriteHeader` ladder inside the handler is the anti-pattern
  this replaced.

The data-access vocabulary these handlers sit on — `Provider`,
`Administrator`, `NewProvider`, `NewAdministrator`, `server.Page`, the
per-domain `ErrNotFound` — is defined in
[patterns-provider.md](patterns-provider.md).

---

## Resource File Structure

### Route Registration

`apps/fleet-service/internal/vehicle/resource.go:28-58`, verbatim — the route
entry point plus one body-less handler:

```go
func InitializeRoutes(log logrus.FieldLogger, db *gorm.DB, ownerCheck OwnerChecker, primaryImage PrimaryImageSetter, statusDeps StatusDeps, record ActivityRecorder, emit EventEmitter) func(chi.Router) {
	proc := NewProcessor(log, NewProvider(db), NewAdministrator(db)).
		WithActivityRecorder(record).
		WithEventEmitter(emit)
	return func(r chi.Router) {
		// GET /fleets/{id}/vehicles — list vehicles (fleet-paged)
		r.Get("/fleets/{id}/vehicles", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			fleetID := chi.URLParam(req, "id")
			if err := authz.RequireSameFleet(identity, fleetID); err != nil {
				server.WriteError(w, err)
				return
			}
			page := server.ParsePage(req)
			ms, total, err := proc.ListByFleet(fleetID, page)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			// Status is derived on read (design §10.2). Per-vehicle gathering is
			// acceptable at household scale.
			now := time.Now().UTC()
			resources := make([]server.Resource, 0, len(ms))
			for _, m := range ms {
				resources = append(resources, TransformDerived(m, statusDeps.Derive(m, now)))
			}
			server.WriteJSON(w, http.StatusOK, server.Document{
				Data: resources,
				Meta: page.Meta(total),
			})
		})
```

**Key points:**

- Collaborators are **explicit parameters**, one interface per cross-domain
  dependency. There is no curried `func(db *gorm.DB) ...` wrapper for DI.
  Optional processor collaborators arrive by `With*` chaining on the processor
  (`resource.go:29-31`).
- The processor closure is built **once**, outside `return func(r chi.Router)`,
  and closed over by every handler in the domain. The logger is likewise
  captured from the `log` parameter — there is no per-request dependency object
  to ask for one.
- Path params are `chi.URLParam(req, "id")`; caller identity is
  `auth.IdentityFromContext(req.Context())`.
- **No handler-name string is registered anywhere.** Correlating a log line to a
  request is `telemetry.CorrelationIDFromContext(req.Context())`
  (`resource.go:87`), which reads the context and needs no name.
- Authorization runs in the handler, before the processor call, and its failure
  goes out through the same `server.WriteError`.

### Pagination round trip

All four steps are in the block above: `server.ParsePage(req)` (reads
`page[number]`/`page[size]`, defaults 1/25, caps size at 100 —
`packages/shared-go/server/pagination.go:20-33`) → `proc.ListByFleet(fleetID,
page)`, which returns `(models, total, error)` → `page.Meta(total)`, giving
`{total, totalPages, number, size}` → `server.Document{Data: ..., Meta: ...}`.
**Paginating a database query belongs to the provider**, which takes
`server.Page` and applies `Offset()`/`Limit()` to the query — do not assemble an
offset in a handler in order to run a query there.

Slicing an **in-memory** aggregate the handler has already assembled is a
different operation and is legitimate. `maintenanceschedule/resource.go:299-313`
is the live example: `queueHandler` calls `proc.Queue(...)` at `:293`, then pages
the returned slice with `entries[start:end]` and reports the true length through
`page.Meta(total)`. No SQL is involved — the code says so at `:301` ("Page the
already-filtered slice in memory") — and it is an ordinary JWT-protected domain
route (`GET /fleets/{id}/maintenance/upcoming` and `/overdue`, registered under
`InitializeRoutes` at `resource.go:257,259`), not an exception.

---

## Handler Choice Is a Frontend API Contract

**Wrapping an existing plain `http.HandlerFunc` in
`server.RegisterInputHandler` is a breaking change for every caller of that
endpoint, even if the URL and HTTP method are unchanged.** The reverse —
unwrapping — is not: a body sent to a handler that no longer reads it is simply
ignored. `POST /vehicles/{id}/restore` is exactly that case today
(`vehicle/resource.go:178` is a plain handler; `VehicleService.ts:40-49` still
sends it a full envelope, and it works). Only plain →
`server.RegisterInputHandler` needs a same-commit frontend change.

`server.RegisterInputHandler` reads the whole request body *before* the handler
runs and rejects it if it is not valid JSON, or is valid JSON that is neither an
object nor `null` (`packages/shared-go/server/handler.go:46-60`, verbatim):

```go
// RegisterInputHandler decodes a typed JSON:API attributes payload {data:{attributes:T}}.
func RegisterInputHandler[T any](fn func(http.ResponseWriter, *http.Request, T)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var doc struct {
			Data struct {
				Attributes T `json:"attributes"`
			} `json:"data"`
		}
		if err := json.NewDecoder(r.Body).Decode(&doc); err != nil {
			WriteError(w, ErrValidation)
			return
		}
		fn(w, r, doc.Data.Attributes)
	}
}
```

A decode failure writes `server.ErrValidation`, which `server.StatusFor` maps to
**422** with title `"validation"`, and `fn` is never invoked
(`apps/auth-service/internal/user/resource_test.go:180,190` asserts exactly
this for the body `{"data":`). What fails is narrower than it looks — verified
by running `encoding/json` against the anonymous struct at `handler.go:49-53`:

| Body | Result |
| --- | --- |
| *(empty)* | rejected — `EOF` |
| `{"data":` | rejected — `unexpected EOF` |
| `[1,2]`, `123`, `"x"`, `true` | rejected — cannot unmarshal into struct |
| `null`, `{"data":null}`, `{"data":{"attributes":null}}` | **accepted**, `fn` runs with a zero-valued `T` |
| `{}`, or any object without the expected keys | **accepted**, `fn` runs with a zero-valued `T` |

So a missing or `null` attribute is *not* a 422 from the decoder — it arrives as
a Go zero value. Required-field checks belong to the builder or the processor
(see [Validation Guidelines](#validation-guidelines)), never to the decoder.

This trips up "action" endpoints that have no real attributes (restore,
complete, set-primary). It is tempting to give them a typed attributes struct
just to satisfy a "every POST has a typed body" habit — that is fine, but a
caller that sends no body at all will start getting a 422, so callers must
change too.

**When you wire a new handler with `server.RegisterInputHandler`, or convert an
existing one:**

1. Define a narrow unexported attributes struct in `rest.go` —
   `createAttributes` / `patchAttributes`. For an action endpoint with no
   attributes, an inline anonymous struct is the local convention
   (`apps/fleet-service/internal/vehicle/resource.go:212-214`).
2. Update **every** frontend service wrapper that calls the endpoint, in the
   **same commit**, to send the JSON:API envelope:
   ```json
   { "data": { "type": "vehicles", "id": "<resource id>", "attributes": {} } }
   ```
   The `type` is the service's own `resourceType` literal — on the backend it is
   the string in the `server.Resource` literal (`vehicle/rest.go:75`), on the
   frontend the `resourceType` field
   (`apps/web/src/services/api/VehicleService.ts:26,40-49`). Nothing derives it
   from a Go method.
3. Run the frontend type-check and any service-layer tests as part of the same
   change.

**Conversely**, if you are keeping a handler body-less, register the plain
`func(w, r)` directly on chi and do not wrap it. That is the right choice for:

- Pure action endpoints with no parameters beyond the URL path
  (`POST /vehicles/{id}/restore`, `resource.go:178`).
- Endpoints whose only input comes from path or query parameters.

A backend-only change that wraps a handler in `server.RegisterInputHandler`
without a matching frontend update is the most likely source of a sudden 422 on
a previously working endpoint. **Backend tests will usually not catch it**: the
processor tests call `proc.Create` directly and never reach the decoder, and 13
of the 23 `resource.go` files — `vehicle` among them — have no router-level
`resource_test.go` at all. A sibling `resource_test.go` is not proof of cover
either; it has to drive a malformed body through the router, which only
`apps/auth-service/internal/user/resource_test.go:180,190` is verified here to
do. Make the frontend change part of the same commit.

---

## Handler Patterns

### Body-less handler

The `GET /fleets/{id}/vehicles` block under
[Route Registration](#route-registration) is the shape: resolve identity →
resolve path params → authorize → call the processor → `server.WriteError` on
failure → `server.WriteJSON` on success. Every early return is a bare
`server.WriteError(w, err); return`.

### Bodied handler

`apps/fleet-service/internal/vehicle/resource.go:60-94`, verbatim:

```go
		// POST /fleets/{id}/vehicles — create a vehicle
		r.Post("/fleets/{id}/vehicles", server.RegisterInputHandler(func(w http.ResponseWriter, req *http.Request, attrs createAttributes) {
			identity := auth.IdentityFromContext(req.Context())
			fleetID := chi.URLParam(req, "id")
			if err := authz.RequireSameFleet(identity, fleetID); err != nil {
				server.WriteError(w, err)
				return
			}
			if err := authz.RequireWrite(identity); err != nil {
				server.WriteError(w, err)
				return
			}
			m, err := NewBuilder().
				SetFleetID(fleetID).
				SetNickname(attrs.Nickname).
				SetMake(attrs.Make).
				SetModel(attrs.Model).
				SetTrim(attrs.Trim).
				SetYear(attrs.Year).
				SetVIN(attrs.VIN).
				SetCurrentMileage(attrs.CurrentMileage).
				SetNotes(attrs.Notes).
				Build()
			if err != nil {
				server.WriteError(w, err)
				return
			}
			traceID := telemetry.CorrelationIDFromContext(req.Context())
			created, err := proc.Create(m, identity.UserID, traceID)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusCreated, server.Document{Data: Transform(created)})
		}))
```

**Key points:**

- The type parameter is **inferred** from the callback's third argument.
  `server.RegisterInputHandler(func(w, req, attrs createAttributes) {...})` —
  no explicit `[T]`, no separate handler type.
- The handler maps `attrs` onto the domain **builder**, not onto the entity.
  `Build()` is where the invariants are checked.
- `201 Created` for a create; `200 OK` for update/restore; `204 No Content` with
  a bare `w.WriteHeader(http.StatusNoContent)` for a delete (`resource.go:174`).

---

## REST Model Structure

### Response attributes

There is no `Attributes`-implementing interface and no marshaling library. A
domain declares a plain `Attributes` struct and puts it inside a
`server.Resource` literal. `apps/fleet-service/internal/vehicle/rest.go:9-29`,
verbatim:

```go
// Attributes is the JSON:API attributes payload for a vehicle.
//
// Status, LastActivityAt, and NextDue are all DERIVED ON READ (design §10.2) and
// never stored on the entity. They are computed from the vehicle's active
// maintenance-schedule due detail and its last activity time, and are exposed
// read-only here.
type Attributes struct {
	FleetID             string   `json:"fleetId"`
	Nickname            string   `json:"nickname,omitempty"`
	Make                string   `json:"make"`
	Model               string   `json:"model"`
	Trim                string   `json:"trim,omitempty"`
	Year                int      `json:"year"`
	VIN                 string   `json:"vin,omitempty"`
	CurrentMileage      int      `json:"currentMileage,omitempty"`
	PrimaryImageMediaID string   `json:"primaryImageMediaId,omitempty"`
	Notes               string   `json:"notes,omitempty"`
	Status              string   `json:"status,omitempty"`
	LastActivityAt      string   `json:"lastActivityAt,omitempty"` // RFC 3339, UTC
	NextDue             *NextDue `json:"nextDue,omitempty"`
}
```

`server.Resource` carries `Type`, `ID`, `Attributes any` and optional
`Relationships` (`packages/shared-go/server/jsonapi.go:12-18`). The `type` is a
**string literal** in the `Transform` body (`rest.go:75`) and the `ID` is a
separate field — never an attribute, and never tagged `json:"-"` inside the
attributes struct.

### Request attributes — narrow named structs

Requests do **not** reuse `Attributes`. Each write endpoint gets its own
unexported struct naming the exact fields it accepts.
`rest.go:31-52`, verbatim — the comments are the rule:

```go
// createAttributes is the exact set of fields POST /fleets/{id}/vehicles accepts.
// Named rather than anonymous so a test can assert that no derived attribute is
// bindable — this narrow shape IS the read-only enforcement (FR-8.3, NFR-7): an
// unknown lastActivityAt or nextDue in a request body has nowhere to land.
type createAttributes struct {
	Nickname       string `json:"nickname"`
	Make           string `json:"make"`
	Model          string `json:"model"`
	Trim           string `json:"trim"`
	Year           int    `json:"year"`
	VIN            string `json:"vin"`
	CurrentMileage int    `json:"currentMileage"`
	Notes          string `json:"notes"`
}

// patchAttributes is the exact set of fields PATCH /vehicles/{id} accepts.
// Pointers distinguish "absent" from "set to zero" on a partial update.
type patchAttributes struct {
	Nickname       *string `json:"nickname"`
	CurrentMileage *int    `json:"currentMileage"`
	Notes          *string `json:"notes"`
}
```

**Key points:**

- **Pointer fields for optional attributes.** On a patch struct this is
  load-bearing: `*string` distinguishes "field absent" from "set to empty"
  (`rest.go:46-52`, and the nil checks at `resource.go:134-145`).
- **Flat structure** — no nested `Data` / `Type` / `Attributes` fields.
  `server.RegisterInputHandler` has already stripped the envelope, so the typed
  struct *is* the attributes object.
- The envelope is decoded by `encoding/json` inside
  `server.RegisterInputHandler` (`handler.go:47-58`). There is no unmarshal
  hook to implement.
- Prefer a **named** struct over an inline anonymous one when a test should be
  able to assert the accepted field set. Inline anonymous structs are used for
  single-field action bodies (`vehicle/resource.go:212-214`).

---

## Transform Functions

`Transform` and `TransformDerived` return a `server.Resource` and **cannot
fail** — there is no error to propagate. `rest.go:54-103`, verbatim:

```go
// Transform converts a Model to a JSON:API Resource carrying no derived
// attributes. Used by the write paths (create, update, restore, primary-image):
// those responses echo a write, and none of the derived values is a property of
// the write.
func Transform(m Model) server.Resource {
	return TransformDerived(m, Derived{})
}

// TransformDerived converts a Model to a JSON:API Resource, attaching the
// read-only values derived on read.
//
// LastActivityAt is carried as a string rather than a time.Time because
// encoding/json's omitempty has no effect on a struct: a time.Time field would
// emit "0001-01-01T00:00:00Z" for the absent case and defeat FR-8.4's
// "omitted, not zero-valued" contract.
func TransformDerived(m Model, d Derived) server.Resource {
	lastActivity := ""
	if !d.LastActivityAt.IsZero() {
		lastActivity = d.LastActivityAt.UTC().Format(time.RFC3339)
	}
	return server.Resource{
		Type: "vehicles",
		ID:   m.ID(),
		Attributes: Attributes{
			FleetID:             m.FleetID(),
			Nickname:            m.Nickname(),
			Make:                m.Make(),
			Model:               m.Model(),
			Trim:                m.Trim(),
			Year:                m.Year(),
			VIN:                 m.VIN(),
			CurrentMileage:      m.CurrentMileage(),
			PrimaryImageMediaID: m.PrimaryImageMediaID(),
			Notes:               m.Notes(),
			Status:              d.Status,
			LastActivityAt:      lastActivity,
			NextDue:             d.NextDue,
		},
	}
}

// TransformSlice converts a slice of Models to JSON:API Resources (no derived
// attributes).
func TransformSlice(ms []Model) []server.Resource {
	out := make([]server.Resource, 0, len(ms))
	for _, m := range ms {
		out = append(out, Transform(m))
	}
	return out
}
```

**Both `Transform` and `TransformSlice` are mandatory** (checked as `DOM-04` /
`DOM-05`). A list handler must call `TransformSlice`, not re-implement the
conversion inline — see `membership/resource.go:48` for the plain form:

```go
server.WriteJSON(w, http.StatusOK, server.Document{Data: TransformSlice(ms)})
```

**Caveat — decorating is not re-implementing.** `vehicle`'s list handler
legitimately writes its own loop (`resource.go:50-53`) because each row needs a
per-row derived value:

```go
resources := make([]server.Resource, 0, len(ms))
for _, m := range ms {
	resources = append(resources, TransformDerived(m, statusDeps.Derive(m, now)))
}
```

`TransformSlice` cannot express this — it has no per-element second argument.
A loop that calls `Transform`/`TransformDerived` per element and adds
per-element data is fine; a loop that rebuilds the `server.Resource` literal by
hand is not. `DOM-05` false-positives on `vehicle` without this distinction.

---

## Error Handling

Handlers do **not** map errors. They call `server.WriteError(w, err)` once per
failure path and return:

```go
current, err := proc.GetByID(id)
if err != nil {
	server.WriteError(w, err)
	return
}
```

`server.StatusFor` does the mapping from the sentinel
(`packages/shared-go/server/errors.go:5-43`):

| Sentinel | Status | Message (`title` on 4xx) |
| --- | --- | --- |
| `server.ErrBadRequest` | 400 | `bad request` |
| `server.ErrUnauthorized` | 401 | `unauthorized` |
| `server.ErrForbidden` | 403 | `forbidden` |
| `server.ErrNotFound` | 404 | `not found` |
| `server.ErrConflict` | 409 | `conflict` |
| `server.ErrGone` | 410 | `gone` |
| `server.ErrRequestEntityTooLarge` | 413 | `request entity too large` |
| `server.ErrUnsupportedMediaType` | 415 | `unsupported media type` |
| `server.ErrValidation` | 422 | `validation` |
| `server.ErrTooManyRequests` | 429 | `too many requests` |
| *anything else* | **500** | redacted — see below |

`StatusFor` uses `errors.Is`, so a wrapped sentinel still maps. **Anything it
does not recognise becomes a 500** — including a raw `gorm.ErrRecordNotFound`
and a domain's own `ErrNotFound`. Translate at the boundary: the provider turns
`gorm.ErrRecordNotFound` into the domain `ErrNotFound`
([patterns-provider.md](patterns-provider.md#translate-gormerrrecordnotfound-at-the-boundary)),
and the processor turns that into `server.ErrNotFound`
(`apps/fleet-service/internal/vehicle/processor.go:58-59`).

### Client-facing detail

`server.Detailed(base, detail)` wraps a sentinel with a human-readable
`detail` while leaving `title` as the base sentinel's message, so the response
shape does not change for existing callers (`errors.go:45-66`):

```go
return Operation{}, server.Detailed(server.ErrValidation, "unsupported scope")
```

The `detail` is rendered on 4xx only.

### 5xx bodies are redacted

`server.WriteError` replaces the title of any 5xx with the fixed
`server.InternalErrorTitle` (`"internal server error"`) and writes the real
error to the server-side error logger instead
(`packages/shared-go/server/jsonapi.go:73-116`). Callers pass raw repository
errors straight in, so `err.Error()` on a 500 is whatever GORM or the driver
produced — table names, column names, SQLSTATE codes, sometimes parameter
values. None of that goes to the client.

Consequences for handler code:

- Do **not** add `w.Write` of an error message alongside `server.WriteError`.
- Do **not** log 4xx separately. They are routine client mistakes;
  `WriteError` deliberately logs nothing below 500.
- The logger is installed once by `server.New` (`handler.go:21-24`), so no
  per-service wiring is needed. Call `server.SetErrorLogger` directly only for a
  handler mounted outside the shared bootstrap.

---

## Nested and Cross-Domain Routes

There are no `/relationships/` endpoints in this tree. Two shapes cover the
cases:

**Nested under the parent resource.** Collection routes live under their owner
(`GET`/`POST /fleets/{id}/vehicles`); item routes are flat (`GET
/vehicles/{id}`). Both are registered by the child's own domain. The path param
is always `{id}` for the resource owning the segment before it, so a nested
handler reads `chi.URLParam(req, "id")` as the *parent* id (`resource.go:34-36`
vs `resource.go:97-99`).

**Cross-domain delegation through an injected interface.** When a route on one
domain must mutate another, the owning domain declares a narrow interface and
takes it as an `InitializeRoutes` parameter — it never imports the other
package's provider or administrator (`resource.go:18-23`):

```go
// PrimaryImageSetter handles setting the primary image for a vehicle, updating
// both the vehiclemedia rows and mirroring into vehicles.primary_image_media_id.
// Satisfied by *vehiclemedia.Processor.
type PrimaryImageSetter interface {
	SetPrimary(vehicleID, mediaID string) error
}
```

`PUT /vehicles/{id}/primary-image` then authorizes locally, calls
`primaryImage.SetPrimary(id, attrs.MediaID)`, re-fetches, and responds
(`resource.go:209-242`). Wiring the concrete implementation together is
`cmd/main.go`'s job, not the domain's.

---

## Anti-Patterns to Avoid

❌ **Calling provider or administrator functions directly from a handler:**

```go
// DON'T DO THIS
r.Get("/vehicles/{id}", func(w http.ResponseWriter, req *http.Request) {
	// ❌ WRONG — bypasses the processor layer
	m, err := NewProvider(db).GetByID(chi.URLParam(req, "id"))
	// ...
})
```

✅ **Go through the processor closure built in `InitializeRoutes`:**

```go
// DO THIS
proc := NewProcessor(log, NewProvider(db), NewAdministrator(db))
r.Get("/vehicles/{id}", func(w http.ResponseWriter, req *http.Request) {
	// ✅ CORRECT — the processor owns error translation and business rules
	m, err := proc.GetByID(chi.URLParam(req, "id"))
	// ...
})
```

`vehicle/resource.go` is the reference: every data call goes through `proc.*`.
Nine handler-level `prov`/`adm` calls exist across five domains, and each falls
under one of two structural exceptions — check which one applies before flagging:

1. **The route is an internal (network-restricted) one.** Handlers registered by
   `InitializeInternalRoutes` go straight to the data layer as a class:
   `platformadmin/resource.go:143`, `mediaobject/resource.go:276`,
   `membership/resource.go:181`. (`mediaobject` has a full processor with both
   collaborators, so it is the route class, not a missing facade, that decides
   this.) `platformadmin/resource.go:124` is the same family — an internal route
   running its own `db.Raw` with `page.Size, page.Offset()`.
2. **The domain's `Processor` does not hold that collaborator**, so there is no
   facade to bypass: `mileage`'s processor holds neither
   (`mileage/processor.go:14-17` — only a `VehicleMileageUpdater`), and
   `membership`'s and `fuel`'s hold a `Provider` and no `Administrator`
   (`membership/processor.go:23-26`, `fuel/processor.go:36-39`). That covers
   `mileage/resource.go:67,123`, `membership/resource.go:87,151`, and
   `fuel/resource.go:211,237`.

Neither is a licence: on a domain JSON:API route whose processor exposes the
operation, go through `proc.*`.

❌ **Manual JSON decoding of the envelope** (checked as `SUB-04`). The check is
zero `json.NewDecoder` / `json.Unmarshal` / `io.ReadAll` **on a domain JSON:API
route** — not in `resource.go` outright. Three deliberate occurrences exist, all
on non-JSON:API endpoints that take a plain JSON body: the two internal admin
routes (`media-service/internal/admin/resource.go:62`,
`notification-service/internal/admin/resource.go:58`) and the session route
(`auth-service/internal/session/resource.go:110`).

```go
// DON'T DO THIS
var req struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			Nickname string `json:"nickname"`
		} `json:"attributes"`
	} `json:"data"`
}
json.NewDecoder(r.Body).Decode(&req)
```

✅ **Use `server.RegisterInputHandler` with a flat named attributes struct:**

```go
// DO THIS
type createAttributes struct {
	Nickname string `json:"nickname"`
	Make     string `json:"make"`
	Model    string `json:"model"`
}

r.Post("/fleets/{id}/vehicles", server.RegisterInputHandler(
	func(w http.ResponseWriter, req *http.Request, attrs createAttributes) { /* ... */ }))
```

❌ **A per-error `w.WriteHeader` ladder in the handler** — that is
`server.StatusFor`'s job, and hand-rolling it is how a 404 becomes a 500.

---

## Validation Guidelines

Validation happens in three places, in this order:

1. **Builder** — construction invariants. `Build()` returns
   `server.ErrValidation` when a required field is missing, so a bad create is
   rejected before the processor is reached (shown below).
2. **Processor** — cross-field and cross-entity rules, and translation of
   provider errors to `server.*` sentinels. Return a typed error; wrap with
   `server.Detailed` when the client needs a sentence.
3. **Handler** — authorization, transport-format parsing, and the single
   `server.WriteError(w, err)`. It does not duplicate the builder's invariant
   checks and it does not choose the status code. Parsing that belongs to the
   wire format does stay here: `mileage/resource.go:102-113` rejects a
   non-positive mileage and parses `RecordedAt` as RFC 3339 before building, and
   `fuel/resource.go:202-208` runs `DerivePrice` on the incoming trio.

The builder is the first line, ahead of the processor
(`apps/fleet-service/internal/vehicle/builder.go:25-31`):

```go
// Build validates invariants and returns the model or a validation error.
func (b *Builder) Build() (Model, error) {
	if b.m.make == "" || b.m.model == "" || b.m.year == 0 {
		return Model{}, server.ErrValidation
	}
	return b.m, nil
}
```

Do not log the error again in the handler: `server.WriteError` logs every 5xx
with the real text and deliberately logs nothing below 500
(`packages/shared-go/server/jsonapi.go:102-105`).
