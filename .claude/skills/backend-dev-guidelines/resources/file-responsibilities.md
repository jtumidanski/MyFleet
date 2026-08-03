
---
title: File Responsibilities
description: Responsibilities of each standard file in a domain package.
---

# File Responsibilities

## `model.go`
Defines immutable domain objects with private fields and accessor methods.

## `entity.go`

Database entity definitions and migration helpers using GORM.

**Required functions:**
- `Make(e Entity) Model` — converts a database entity (plus any child entities
  the model needs) to an immutable domain model. No error return — this holds
  for 17 of 17 domains. The signature is not uniform across those 17: sixteen
  take only the entity; one also takes a child-entity slice —
  `func Make(e Entity, docs []DocumentEntity) Model`
  (`apps/fleet-service/internal/maintenancerecord/entity.go:44`) — but every
  domain returns `Model` alone, never `(Model, error)`.
- `ToEntity() Entity` — method on Model that converts back to a database entity

Both directions are mandatory. `Make` is used after reads; `ToEntity()` is used before writes.

## `builder.go`

Fluent API for constructing validated domain models. `Build()` enforces invariants.

`Build()` returns `server.ErrValidation` when an invariant fails
(`apps/fleet-service/internal/vehicle/builder.go:26-30`) — that sentinel is
what makes the 422 mapping in
[patterns-rest-jsonapi.md](patterns-rest-jsonapi.md#validation-guidelines)
work end to end.

## `processor.go`
Business logic orchestration.

**Constructor signature:** `NewProcessor(log logrus.FieldLogger, p Provider, a Administrator)`
(`apps/fleet-service/internal/maintenancerecord/processor.go:19`). `Provider`
and `Administrator` are the interfaces documented in
[patterns-provider.md](patterns-provider.md).
- The logger parameter **must** be `logrus.FieldLogger` (interface), **not** `*logrus.Logger` (concrete type).

**Key Responsibilities:**
- Orchestrate providers (reads) and administrators (writes)
- Enforce business rules and invariants

**Critical Rules:**
- ✅ `processor.go` → `provider.go` for reads (correct)
- ✅ `processor.go` → `administrator.go` for writes (correct)
- ❌ `processor.go` → direct `db.Create`/`db.Save`/`db.Delete` (WRONG - use administrator functions)

## `administrator.go`

**Write operations** — every insert, update, soft-delete and restore. Exposes an
`Administrator` interface with a `db`-backed implementation and a
`NewAdministrator(db)` constructor; takes and returns domain `Model`s. Side
effects that must commit atomically with the write go through `TxHook`.

Full contract and worked example: [patterns-provider.md](patterns-provider.md).

**Why This Separation:**
- **Testability** — Read and write operations can be mocked independently
- **Clear intent** — Code review can quickly identify state-changing operations
- **Single responsibility** — Each file has one job

## `provider.go`

**Read operations** (queries) that fetch data without side effects. This file handles all read-only database access.

**Key Responsibilities:**
- Exposes a `Provider` interface with a `db`-backed implementation and a
  `NewProvider(db)` constructor — full contract in
  [patterns-provider.md](patterns-provider.md).
- Methods return `(Model, error)` directly — eager, plain values. There is no
  lazy or curried provider type.
- `database.Query[T]` / `database.SliceQuery[T]` are an accepted variant (used
  by 4 of 19 providers), not a requirement — see
  [patterns-provider.md](patterns-provider.md#accepted-variant-databasequery).
- `Make(e)` (see `entity.go` above) runs inside the provider, converting the
  entity to a `Model` before it returns.
- Never modify database state

**Typical Signatures** (`apps/fleet-service/internal/vehicle/provider.go:24,35`):
```go
func (p *dbProvider) GetByID(id string) (Model, error)
func (p *dbProvider) ListByFleet(fleetID string, page server.Page) ([]Model, int, error)
```

**Why This Separation:**
- **Testability** - Read and write operations can be mocked independently
- **Read path stays side-effect free** - the read path never issues a write query
- **Clear intent** - Code review can quickly identify state-changing operations


## `resource.go`
Route registration and handler definitions for REST endpoints.

**Key Responsibilities:**
- Register routes on a chi router; every route entry point returns
  `func(chi.Router)` — full contract in
  [patterns-rest-jsonapi.md](patterns-rest-jsonapi.md).
- Body-less routes (GET/DELETE, and actions with no attributes) register a
  plain `func(w http.ResponseWriter, req *http.Request)` directly on chi.
- Bodied routes (POST/PATCH/PUT) wrap that same func in
  `server.RegisterInputHandler` (`packages/shared-go/server/handler.go:47`),
  which infers its type parameter from the callback's third argument — there
  is no separate handler type to match.
- Errors go out through a single `server.WriteError(w, err)`;
  `server.StatusFor` maps the sentinel to a status code.
- **Delegate ALL business logic to processors - NEVER call provider functions directly**
- Success responses go out through `server.WriteJSON(w, status, server.Document{...})`.
- `server.WriteError` already logs every 5xx itself, so a handler-level log of
  that same error is redundant — but handlers that log to attach request
  context (`log.WithError(err).Error("list fuel logs")`,
  `fuel/resource.go:53`) are the common pattern in this tree, not a
  violation.

**Pattern:** Thin handlers that parse input, invoke processors, handle errors, and marshal responses.

**Critical Rules:**
- ✅ `resource.go` → `processor.go` (correct)
- ❌ `resource.go` → `provider.go` (WRONG - bypasses business logic layer)
- ❌ `resource.go` → database/GORM (WRONG - bypasses all layers)


## `rest.go`
Serialization and transformation between domain models and JSON:API.

**Key Responsibilities:**
- Define an `Attributes` struct and put it inside a `server.Resource` literal
  — there is no `Attributes`-implementing interface and no marshaling
  library. Full contract: [patterns-rest-jsonapi.md](patterns-rest-jsonapi.md#rest-model-structure).
- Define a narrow unexported request struct per write endpoint
  (`createAttributes`, `patchAttributes`) — they do not reuse `Attributes`.
- Implement `Transform(Model) server.Resource` to convert domain models to
  REST representations. 45 of the 47 `Transform*` functions in the tree
  return no error — mapping model getters onto a struct cannot fail. The
  exception is `activity`, whose `Transform`/`TransformSlice` return
  `(server.Resource, error)` / `([]server.Resource, error)` because they
  unmarshal a stored JSON payload that can be malformed
  (`apps/fleet-service/internal/activity/rest.go:23,49`).
- Implement `TransformSlice([]Model) []server.Resource` for bulk transformations.
- Use flat structure for request models (no nested Data/Type/Attributes)
- The resource ID is a separate `server.Resource` field, never an attribute
  and never tagged `json:"-"` inside the attributes struct.
- Use pointer fields for optional attributes with `omitempty`

**Both `Transform` and `TransformSlice` are mandatory.** List handlers must
use `TransformSlice` — do not inline transform loops in `resource.go`, unless
each row needs a per-row derived value `TransformSlice` cannot express; in
that case call `TransformDerived` per element instead (see
[patterns-rest-jsonapi.md](patterns-rest-jsonapi.md#transform-functions)).

**Pattern:** Plain Go structs marshaled into a `server.Resource` /
`server.Document` — there is no code-generation or reflection-based
marshaling library.

