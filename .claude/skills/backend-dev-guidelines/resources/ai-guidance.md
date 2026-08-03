
---
title: AI Code Generation Guidance
description: Rules for AI agents generating or editing Golang services.
---

# AI Code Generation Guidance

## Mandatory Implementation Workflow

**CRITICAL:** Before implementing ANY code changes, review the [Standard Implementation Workflow](../SKILL.md#standard-implementation-workflow) in the main skill document.

**Key Requirements:**
- ✅ Update the in-package `fake*`/`stub*` doubles immediately when the interface they implement changes (see testing-guide.md — example: `apps/auth-service/internal/user/processor_test.go:14` declares `fakeProvider`)
- ✅ Run `make test` BEFORE claiming completion
- ✅ Fix all test failures before proceeding
- ✅ Report actual test output, not assumptions
- ❌ NEVER skip test execution
- ❌ NEVER assume tests will pass

## Core Rules
1. Respect file responsibilities (see file-responsibilities.md).
2. Maintain immutability (see patterns-functional.md#immutability).
3. Wrap bodied routes (the request carries a payload) in `server.RegisterInputHandler[T]`; body-less routes (GET, DELETE, and actions whose only input is the URL path) register a plain handler directly on the chi router (see patterns-rest-jsonapi.md).
4. **Always verify referenced types exist before using them** - Never assume a type/constant/operation exists.
5. **Always run builds AND tests after code changes** - Verify ALL affected services compile and pass tests.
6. **Always ask before implementing new features** - Get user approval before adding new functionality.

## Validation Rules

### Before Using Any Type/Constant/Operation:
1. **Always verify it exists** - Read the relevant file to confirm the type/constant is implemented
2. **Check where it's declared** - e.g. an error sentinel must be declared in `packages/shared-go/server/errors.go`; a `Provider` method must be declared on that domain's `Provider` interface in `provider.go`
3. **Never assume** - Just because it makes logical sense doesn't mean it's implemented

### Examples:
❌ **BAD** - Using a sentinel without verification:
```go
// Assuming server.ErrPreconditionFailed exists without checking
return Model{}, server.ErrPreconditionFailed
```

✅ **GOOD** - Verify first, ask if missing:
```
1. Read packages/shared-go/server/errors.go
2. Search for ErrPreconditionFailed among the declared sentinels
3. If not found: "The sentinel 'server.ErrPreconditionFailed' doesn't exist in
   errors.go. Should I add it (with its StatusFor mapping), or map this case to
   an existing sentinel like server.ErrConflict instead?"
```

❌ **BAD** - Calling a provider method without verification:
```go
// Assuming Provider has a ListActiveByFleet method without checking
ms, err := p.ListActiveByFleet(fleetID)
```

✅ **GOOD** - Verify first, ask if missing:
```
1. Read provider.go
2. Search for ListActiveByFleet in the Provider interface
3. If not found: "The method 'ListActiveByFleet' doesn't exist on this domain's
   Provider interface. To implement it, I would need to:
   - Add the method to the Provider interface and its db-backed implementation
   - Add a matching fake in the processor's _test.go

   Should I implement this first?"
```

## Testing Rules

### After ANY Code Change:
1. **Always run builds for ALL affected services** - Not just the one you modified
2. **Always run tests** for all modified and dependent services
3. **Report failures immediately** - Never commit/continue with failing builds or tests
4. **Update the in-package `fake*`/`stub*` doubles** - When a `Provider`/`Administrator`/`Processor` interface changes, update every `fake*`/`stub*` implementation declared in the `_test.go` files that use it
5. **No partial implementations** - A feature isn't done until all services build and test successfully

### Build & Test Workflow:
```bash
# Run from the repo root. `make build`/`make vet`/`make test` build the
# workspace-wide package pattern `go.work` resolves across every module — not
# a loop over modules, and not the same as a bare `go build ./...` at the root.
make build

# If the build fails:
# 1. Report the failure to the user with error details
# 2. Fix ALL compilation errors (missing methods, type mismatches, etc.)
# 3. Re-run `make build`
# 4. Only proceed when the build succeeds

make vet
make test

# If tests fail:
# 1. Report the failure to the user
# 2. Fix the tests or code (often a fake*/stub* double missing a new method)
# 3. Re-run `make test`
# 4. Only proceed when all tests pass
```

### Common Build Failures & Fixes:
| Error | Cause | Solution |
|-------|-------|----------|
| `does not implement Provider (missing method X)` | Interface method added but a `fake*`/`stub*` double in an affected `_test.go` not updated | Add the method to the `fake*`/`stub*` struct and implement it |
| `redeclared in this block` | Duplicate function declarations | Remove old/duplicate version |
| `cannot use X as Y value` | Function signature changed incompletely | Update ALL call sites (use `grep -r`) |

### When to Run Tests:
- ✅ After adding new files (model.go, processor.go, etc.)
- ✅ After modifying existing files
- ✅ After adding new dependencies
- ✅ Before creating a pull request
- ✅ After implementing new features
- ❌ Never skip tests "to save time"

## Implementation Rules

### Before Implementing New Features:
1. **ALWAYS ask the user first** if you identify a missing feature during any task
2. **Explain what's missing** and what would need to be implemented
3. **Provide options** for how to proceed
4. **Wait for explicit approval** before implementing

### Example Dialog:
❌ **WRONG** - Implementing without asking:
```
"I noticed the notification feature doesn't exist. Let me implement it for you..."
[Proceeds to implement without approval]
```

✅ **CORRECT** - Ask first:
```
"I need to add a notification feature for this task update, but there is no
notification system implemented yet.

To implement it, I would need to:
1. Add a notification model and entity
2. Add processor methods for creating notifications
3. Add REST endpoints for retrieving notifications

This would add a new domain.

How would you like to proceed?
1. Implement the notification feature first
2. Skip notifications for now
3. Use a different approach
"
```

## Migration & Refactoring Rules

### No Type Aliases During Migrations
When migrating types/functions to a shared library, update ALL service call sites to import from the new library directly. Never leave type aliases (`type Foo = lib.Foo`), re-exports, or thin wrappers that just delegate. We control the full lifecycle of all services — there is no backwards-compatibility concern.

### Clean Up Dead Code After Extraction
After extracting code to a shared library, review every modified service file for symbols that are no longer referenced: unused constants, structs, functions, imports, and variables. Use `grep` across the service to confirm nothing depends on them, then delete. Do not leave dead code behind.

## Commonly Missed Items Checklist

Before reporting any domain package as complete, verify **every item** below. These are the items most frequently missed across all service audits:

- [ ] **`builder.go` exists** for every domain with a model — with `NewBuilder()`, fluent setters, and `Build()` that validates invariants (`DOM-01`)
- [ ] **`ToEntity()` method** exists on the Model type in `entity.go` — `func (m Model) ToEntity() Entity`, present on every domain's Model, e.g. `apps/fleet-service/internal/vehicle/entity.go:63` (`DOM-02`)
- [ ] **`Make` constructor** in `entity.go` returns `Model` with no error and is not uniform: 16 of 17 domains are `func Make(e Entity) Model`; the exception is `maintenancerecord`, whose model needs its child rows too — `func Make(e Entity, docs []DocumentEntity) Model` (`apps/fleet-service/internal/maintenancerecord/entity.go:44`) (`DOM-03`)
- [ ] **`TransformSlice`** exists in `rest.go` alongside `Transform` — list handlers use it, not an inline loop, unless the loop is decorating each element with a per-row derived value via `TransformDerived` (e.g. `apps/fleet-service/internal/vehicle/resource.go:50-53`) rather than rebuilding the `server.Resource` literal by hand (`DOM-05`)
- [ ] **Every processor that takes a logger takes `logrus.FieldLogger`**, not `*logrus.Logger`, in its constructor — 18 of 19 `NewProcessor` functions take a logger; the exception, `apps/auth-service/internal/oidc/processor.go:28`, takes no logger at all (`DOM-06`)
- [ ] **Handlers use the `log` parameter `InitializeRoutes` receives** (passed straight into `NewProcessor`, `apps/fleet-service/internal/vehicle/resource.go:28-29`) — never `logrus.StandardLogger()`, which is reserved for the shared error-logger fallback in `packages/shared-go/server/jsonapi.go:70` (`DOM-07`)
- [ ] **Bodied routes wrap their handler in `server.RegisterInputHandler[T]`**; body-less routes — GET, DELETE, and action endpoints whose only input is the URL path (e.g. `POST /vehicles/{id}/restore`, `apps/fleet-service/internal/vehicle/resource.go:178`) — register a plain `func(w, r)` directly on the chi router (`DOM-08`)
- [ ] **Every domain error goes out through `server.WriteError(w, err)`** — no hand-built envelope, no bare `http.Error`, no per-error `w.WriteHeader` ladder (`DOM-09`)
- [ ] **`provider.go` declares a `Provider` interface** with a `db`-backed implementation and a `NewProvider(db)` constructor — universal, 19 of 19. **Any method that fetches a single record** additionally translates `gorm.ErrRecordNotFound` at the boundary rather than letting the raw GORM error reach the handler — `server.StatusFor` does not recognise it and would map it to 500. 16 of 19 `provider.go` files have a single-record fetch method; the remaining 3 have none and nothing to translate because they expose only list or existence queries, which never surface `gorm.ErrRecordNotFound`: `mileage/provider.go:12-16` (`ListByVehicle` only), `notification/provider.go:29-31` (`ListByUser` only), `platformadmin/provider.go:6-19` (`IsAdmin`/`IsRevoked` only). Of those 16, most translate to a domain not-found sentinel (usually `ErrNotFound`; `admin/provider.go:33` is `ErrOperationNotFound`) — but 2 legitimately express not-found as a bool flag or zero value instead, because the caller doesn't need it as an error: `activity/provider.go:63-65` (`LastActivityByVehicle` returns `time.Time{}, nil`) and `maintenancecategory/provider.go:102-104` (`FindByName` returns `Model{}, false, nil`) (`DOM-10`)
- [ ] **No `os.Getenv()` in handlers** — env vars read once in config, injected via constructors (`DOM-11`)
- [ ] **No cross-domain business logic in handlers** — orchestration belongs in the processor layer (`DOM-12`)
- [ ] **Sub-domain packages have proper layers** — even simple action-event packages need a `processor.go` and `administrator.go` (or fold into the parent domain's processor) (`SUB-01`/`SUB-02`)

---

## Generation Workflow
1. **Validate dependencies** - Verify all types/operations you plan to use exist
2. Create `model.go` - Immutable domain model with accessors
3. Map `entity.go` to DB - GORM entities with migrations, including `Make()` and `ToEntity()`
4. Implement `builder.go` - Fluent API for model construction with `Build()` validation
5. Define `provider.go` (reads) and `administrator.go` (writes) - each an interface with a `db`-backed implementation and a `New*` constructor
6. Define `processor.go` - Pure business logic, `logrus.FieldLogger` in the constructor
7. Add `rest.go` - JSON:API DTOs with `Transform` AND `TransformSlice` functions
8. Add `resource.go` - Route registration and thin handlers using the `log` parameter `InitializeRoutes` receives
9. Write table-driven tests
10. **Run the Commonly Missed Items Checklist above**
11. **Build the project** - From the repo root, run `make build`
12. **Run all tests** - Verify nothing broke with `make test`
13. **Report build/test results** - Show pass/fail status to user
14. **Fix ALL issues before proceeding** - No partial implementations allowed

## REST Generation Specifics

### When generating `rest.go`:
- ✅ Declare a plain `Attributes` struct and put it inside a `server.Resource{Type, ID, Attributes}` literal — there is no interface to implement and no marshaling library
- ✅ Use flat, narrow named structs for request models (`createAttributes` / `patchAttributes`) — no nested Data/Type/Attributes
- ✅ Use pointer fields on a patch struct so a nil distinguishes "absent" from "set to zero"
- ✅ Create `Transform(Model) server.Resource` — no error return in 45 of 47 `Transform*` functions; the two exceptions (`activity`'s `Transform`/`TransformSlice`) return an error because they unmarshal a stored JSON payload
- ✅ Create `TransformSlice([]Model) []server.Resource` alongside `Transform`
- ❌ Never use jsonapi struct tags on fields
- ❌ Never create nested Data/Type/Attributes structures

### When generating `resource.go`:
- ✅ Return `func(chi.Router)` from `InitializeRoutes`
- ✅ Build the processor once, closed over the `log` parameter, outside `return func(r chi.Router) {...}`
- ✅ Register body-less handlers (GET, DELETE, path-only actions) as a plain `func(w, r)` directly on `r.Get`/`r.Delete`/`r.Post`
- ✅ Wrap bodied handlers (the request carries a payload) in `server.RegisterInputHandler[T]`
- ✅ Map every domain error to a status with a single `server.WriteError(w, err)` call
- ❌ Never manually decode JSON with nested structures
- ❌ Never create custom error response helpers or a per-error `w.WriteHeader` ladder

## Common Anti-Patterns to Avoid

### ❌ Manual JSON:API Envelope Handling
```go
// DON'T DO THIS
var req struct {
    Data struct {
        Type       string `json:"type"`
        Attributes struct { ... } `json:"attributes"`
    } `json:"data"`
}
json.NewDecoder(r.Body).Decode(&req)
```

### ✅ Use Flat, Narrow Request Models
`apps/fleet-service/internal/vehicle/rest.go:31-44`, verbatim:
```go
// createAttributes is the exact set of fields POST /fleets/{id}/vehicles accepts.
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
// Let server.RegisterInputHandler[T] handle JSON:API unmarshaling; the ID is
// never part of the attributes struct — it comes from NewBuilder(), never the
// request body.
```
