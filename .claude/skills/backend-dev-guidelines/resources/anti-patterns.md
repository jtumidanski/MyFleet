
---
title: Anti-Patterns
description: Common pitfalls to avoid when implementing Golang microservices.
---

# Anti-Patterns


| Anti-Pattern | Why It's Wrong |
|---------------|----------------|
| Business logic in handlers | Breaks separation of concerns |
| **Handlers calling provider functions directly** | **Breaks layer separation - handlers must call processors, not providers** |
| **Direct entity creation in handlers** (`db.Create(&e)` in resource.go) | **Bypasses both processor and administrator layers — all writes must go through administrator functions called by processors** |
| **Cross-domain business logic in handlers** (e.g., handler creating records in another domain) | **Move cross-domain orchestration to the processor layer** |
| Mutable public fields | Violates immutability |
| Ad hoc GORM queries (`db.Where(...)`, `db.Find(...)`, etc.) assembled inside a processor | Reads and writes belong in `Provider`/`Administrator` methods; a processor may still hold a `db` handle solely to wrap an `Administrator` call in a transaction (`maintenanceschedule/processor.go:29,233`) |
| Missing validation | Allows invalid domain states |
| **`logrus.StandardLogger()` in domain handlers** | **Use the `log logrus.FieldLogger` parameter `InitializeRoutes` already closes over — see §Handler Logger Anti-Pattern below** |
| **`*logrus.Logger` in processor constructors** | **Use `logrus.FieldLogger` — 18 of the 19 real `NewProcessor` functions take it as their first parameter (patterns-functional.md); the exception (`oidc.NewProcessor`) takes no logger at all, never a concrete `*logrus.Logger`** |
| **A plain `http.HandlerFunc` on a POST/PATCH route that hand-decodes the body** | **Use `server.RegisterInputHandler[T]` — see §Wrong Handler Type below** |
| **Hand-built error envelopes, bare `http.Error`, or a per-error `w.WriteHeader` ladder** | **Use `server.WriteError(w, err)` — it is the only path that redacts a 5xx's real error text and logs it server-side — see §Hand-Rolled Error Responses below** |
| **`os.Getenv()` in handlers** | **Read env vars once at startup via config struct, inject through constructors — per-request `os.Getenv` is wasteful and hard to test** |
| Global context usage | Breaks request isolation |
| Manual JSON:API envelope handling | Breaks JSON:API integration, adds boilerplate |
| Nested Data/Type/Attributes in requests | Use flat structures — `server.RegisterInputHandler[T]` (`packages/shared-go/server/handler.go:47-60`) decodes the `{"data":{"attributes":T}}` envelope into `T` |
| Type aliases for library migrations | Adds indirection; we control all services — update call sites directly |
| Leaving dead code after refactoring | Unused constants/structs/functions clutter the codebase and cause confusion |
| **`db.Save(&e)` in `administrator.go` when `Entity` has a field `ToEntity()` doesn't assign** | **`db.Save` UPDATEs every column, so the unassigned field is silently zeroed on every write — caught in production (`vehicle/model.go:21-27`); guarded by the `entityguard` static analyzer (`packages/shared-go/database/entityguard`)** |

**Always** prefer pure, context-aware, and testable functions.

**For REST:** Bodied routes (POST/PATCH/PUT) use `server.RegisterInputHandler[T]`
with flat JSON:API-compliant models; body-less routes (GET/DELETE) are a plain
`func(w http.ResponseWriter, req *http.Request)`.

---

## Handler Logger Anti-Pattern

### ❌ Using `logrus.StandardLogger()` in Domain Handlers

Nothing in a domain handler calls `logrus.StandardLogger()` today. The only two
real call sites in the tree are both deliberate, shared infrastructure:
`packages/shared-go/server/jsonapi.go:70`, the `errorLogger()` fallback used
only when `SetErrorLogger` was never called, and its test at
`packages/shared-go/server/errors_test.go:399`. A domain package reaching for
`logrus.StandardLogger()` throws away the logger every handler in the tree
already has available and loses whatever fields/trace context it carries.

**WRONG:**
```go
// resource.go - ANTI-PATTERN
func InitializeRoutes(log logrus.FieldLogger, db *gorm.DB, ...) func(chi.Router) {
	return func(r chi.Router) {
		r.Post("/fleets/{id}/vehicles", server.RegisterInputHandler(func(w http.ResponseWriter, req *http.Request, attrs createAttributes) {
			// ❌ WRONG - ignores the log parameter InitializeRoutes already has
			p := NewProcessor(logrus.StandardLogger(), NewProvider(db), NewAdministrator(db))
			// ...
		}))
	}
}
```

**✅ CORRECT** (`apps/fleet-service/internal/vehicle/resource.go:28-33`, verbatim):
```go
func InitializeRoutes(log logrus.FieldLogger, db *gorm.DB, ownerCheck OwnerChecker, primaryImage PrimaryImageSetter, statusDeps StatusDeps, record ActivityRecorder, emit EventEmitter) func(chi.Router) {
	proc := NewProcessor(log, NewProvider(db), NewAdministrator(db)).
		WithActivityRecorder(record).
		WithEventEmitter(emit)
	return func(r chi.Router) {
		// GET /fleets/{id}/vehicles — list vehicles (fleet-paged)
```

The processor is built **once**, closed over `log`, outside `return func(r
chi.Router)`. Every handler in the domain reuses that same `proc` — there is
no per-request dependency object to fetch a logger from.

This requires processors to accept `logrus.FieldLogger` (not `*logrus.Logger`):
```go
// processor.go - CORRECT
type Processor struct {
	log logrus.FieldLogger // ✅ interface, not concrete type
	// ...
}

func NewProcessor(log logrus.FieldLogger, p Provider, a Administrator) *Processor {
	return &Processor{log: log /* ... */}
}
```

---

## Wrong Handler Type for POST/PATCH Endpoints

### ❌ Hand-Parsing the Body Instead of `server.RegisterInputHandler[T]`

**WRONG:**
```go
// resource.go - ANTI-PATTERN: bypasses server.RegisterInputHandler
r.Post("/fleets/{id}/vehicles", func(w http.ResponseWriter, req *http.Request) {
	body, _ := io.ReadAll(req.Body)      // ❌ manual body reading, error ignored
	var attrs createAttributes
	json.Unmarshal(body, &attrs)          // ❌ manual JSON parsing, error ignored
	// ...
})
```

**✅ CORRECT** (`apps/fleet-service/internal/vehicle/resource.go:60-94`, condensed):
```go
// POST /fleets/{id}/vehicles — create a vehicle
r.Post("/fleets/{id}/vehicles", server.RegisterInputHandler(func(w http.ResponseWriter, req *http.Request, attrs createAttributes) {
	// attrs is already deserialized from {"data":{"attributes":{...}}} — use it directly
	// ... build the model, call proc.Create ...
	server.WriteJSON(w, http.StatusCreated, server.Document{Data: Transform(created)})
}))
```

`server.RegisterInputHandler[T]` (`packages/shared-go/server/handler.go:47-60`)
decodes the JSON:API envelope into `T` before the handler runs. It is a plain
generic function — the type parameter is inferred from the closure's third
argument, so real call sites never spell `[T]` explicitly, and it does no
tracing or logging of its own.

---

## Hand-Rolled Error Responses

### ❌ Writing Status Codes or Error Envelopes Directly

**WRONG:**
```go
// resource.go - ANTI-PATTERN
m, err := proc.GetByID(id)
if err != nil {
	w.WriteHeader(http.StatusInternalServerError)                        // ❌ no envelope, no server-side log
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})   // ❌ leaks internal error text (SEC-09)
	return
}
```

**✅ CORRECT** (`apps/fleet-service/internal/vehicle/resource.go:100-104`, verbatim):
```go
m, err := proc.GetByID(id)
if err != nil {
	server.WriteError(w, err)
	return
}
```

`server.WriteError` (`packages/shared-go/server/jsonapi.go:95-116`) is the only
path that gets both protections a hand-rolled response does not:

- On a 5xx, the real error text is **redacted from the response body** and
  instead written to the server-side error logger
  (`errorLogger().WithError(err)...`, `jsonapi.go:103-104`) — a raw GORM/pq
  error can contain table and column names, which are not the client's
  business (SEC-09).
- On a 4xx, the message is always a sentinel written to be shown to the
  client (`err.Error()`, plus an optional `Detail()`), so it is safe to
  return as-is.

A hand-rolled `w.WriteHeader` + `json.NewEncoder` gets neither: it either
leaks a 5xx's internal error text to the client, or silently drops the fault
with no server-side record of it.

---

## Sub-Domain / Action-Event Packages

Even lightweight packages (e.g., `vehiclemedia`, `mileage`, `activity`) that
record action events **must follow layer separation**:

- **Must have** a `processor.go` (or use the parent domain's processor) for business logic
- **Must have** an `administrator.go` for write operations
- **Must use** `server.RegisterInputHandler[T]` for POST endpoints
- **Must NOT** create entities directly in handlers or parse JSON manually

If the sub-domain is simple enough that a standalone processor adds no value, fold the action into the parent domain's processor as a method instead of creating a separate package with layer violations.

---

## Critical Layer Violations

### ❌ Handlers Calling Providers Directly

**WRONG - Handler bypassing processor:**
```go
// resource.go - ANTI-PATTERN
func handleGetVehicleRequest(db *gorm.DB) func(w http.ResponseWriter, req *http.Request) {
	return func(w http.ResponseWriter, req *http.Request) {
		// ❌ WRONG - calling provider function directly from handler
		v, err := NewProvider(db).GetByID(chi.URLParam(req, "id"))
		// ...
	}
}
```

**Correct layer flow:**
```
resource.go (handler) → processor.go (business logic) → provider.go (data access) → database
```

**✅ CORRECT - Handler calling processor** (`apps/fleet-service/internal/vehicle/resource.go:96-104`, verbatim):
```go
// GET /vehicles/{id}
r.Get("/vehicles/{id}", func(w http.ResponseWriter, req *http.Request) {
	identity := auth.IdentityFromContext(req.Context())
	id := chi.URLParam(req, "id")
	m, err := proc.GetByID(id)
	if err != nil {
		server.WriteError(w, err)
		return
	}
	// ...
```

**Why this matters:**
1. **Separation of concerns** - Handlers parse requests and marshal responses, processors contain business logic
2. **Testability** - Business logic in processors can be tested without HTTP infrastructure
3. **Reusability** - Processor methods can be called from handlers or other processors
4. **Maintainability** - Changes to data access don't affect handlers
5. **Single responsibility** - Each layer has a clear, focused purpose

**Valid dependencies:**
- ✅ `resource.go` → `processor.go`
- ✅ `processor.go` → `provider.go`
- ✅ `provider.go` → `entity.go` + GORM

**Invalid dependencies:**
- ❌ `resource.go` → `provider.go` (bypasses processor layer)
- ❌ `resource.go` → `entity.go` (bypasses both processor and provider)
- ❌ `processor.go` → `entity.go` directly for database queries (should use provider)

### Exception: Cross-Domain Read-Only Views with Circular Dependencies

In rare cases a read-only view aggregates across domains that have no single
natural owner for the query — two domains that would otherwise need to import
each other's processors, or a value computed across tables that belongs to no
one domain. Read-only view handlers MAY use providers directly, or raw DB
queries, for cross-domain orchestration.

**When this exception applies:**
- Handler aggregates data from multiple domains
- Circular package dependency prevents calling processors
- Operation is read-only (no state changes)
- Alternative would require significant architectural refactoring

**Example** — the live instance of this exception is
`apps/fleet-service/internal/dashboard`. Its spend-by-vehicle endpoint sums
costs from the `vehicle`, `maintenancerecord` and `fuel` domains via a
dedicated read-only `AggregateProvider` (`dashboard/aggregate.go:40-44`)
called directly from the handler — no single domain's processor owns that
cross-table read (`dashboard/resource.go:96-119`, verbatim, one comment added):
```go
// GET /fleets/{id}/dashboard/spend-by-vehicle?from=&to=
r.Get("/fleets/{id}/dashboard/spend-by-vehicle", func(w http.ResponseWriter, req *http.Request) {
	identity := auth.IdentityFromContext(req.Context())
	fleetID := chi.URLParam(req, "id")

	if err := authz.RequireSameFleet(identity, fleetID); err != nil {
		server.WriteError(w, err)
		return
	}

	from, to, err := parseWindow(req)
	if err != nil {
		server.WriteError(w, server.ErrValidation)
		return
	}

	// ⚠️ EXCEPTION: agg is dashboard's own AggregateProvider, called directly — no single domain's processor owns this cross-table read (aggregate.go:60-127).
	rows, err := agg.SpendByVehicle(fleetID, from, to)
	if err != nil {
		log.WithError(err).Error("spend-by-vehicle aggregate")
		server.WriteError(w, err)
		return
	}
	server.WriteJSON(w, http.StatusOK, server.Document{Data: rows})
})
```

**Requirements for using this exception:**
1. Add a comment explaining WHY the circular dependency exists
2. Keep the raw query minimal (single table, simple where clause)
3. Consider architectural refactoring if this pattern appears frequently
4. Never use this exception for write operations - those MUST go through processors

---
