
---
title: Testing Conventions
description: Testing patterns and practices for MyFleet Golang services.
---

# Testing Conventions

## Test DB Setup

```go
import (
    "testing"
    "gorm.io/driver/sqlite"
    "gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
    t.Helper()
    db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
    if err != nil {
        t.Fatalf("open sqlite: %v", err)
    }
    if err := db.AutoMigrate(&Entity{}); err != nil {
        t.Fatalf("migrate: %v", err)
    }
    return db
}
```

In-memory SQLite is the real pattern, recurring across every service —
`apps/media-service/internal/processedevents/processedevents_test.go:11-27` is
this function almost exactly. Schema-qualified tables (Postgres schemas like
`fleet.`) attach a named in-memory schema and use explicit DDL instead of
bare `AutoMigrate` — see `apps/fleet-service/internal/maintenancecategory/entity_test.go`.
Assert on `err` with `t.Fatalf`, not an assertion library — see
[Test Doubles](#test-doubles).

---

## Focus Areas

1. **Builders** — Validate invariants.
2. **Processors** — Test pure business logic functions.
3. **Providers** — Validate retrieval and error paths.
4. **REST** — Verify status mapping and JSON:API output.

## Guidelines
- Prefer table-driven tests.
- Use in-package `fake*`/`stub*` structs for `Provider`/`Administrator`
  interfaces — see [Test Doubles](#test-doubles) below. There is no `mock/`
  directory convention.

## Example

```go
func TestBuild_requiresMakeModelYear(t *testing.T) {
    _, err := NewBuilder().SetFleetID("f1").Build() // missing make/model/year
    if !errors.Is(err, server.ErrValidation) {
        t.Fatalf("missing required fields must be 422, got %v", err)
    }
}
```

`apps/fleet-service/internal/vehicle/processor_test.go:10-15`, validating the
invariant enforced by `Build()` in
`apps/fleet-service/internal/vehicle/builder.go:26-31`: every builder is a
zero-argument `NewBuilder()` (the id is generated internally via
`uuid.NewString()`), and `vehicle`'s `Build()` rejects an empty make, model,
or year with `server.ErrValidation`.

---

## Interface Change Workflow

**CRITICAL:** When modifying any interface, follow this checklist.

### Checklist for Interface Changes

When adding, modifying, or removing methods from an interface:

- [ ] **Update the interface definition** in the primary file (e.g., `processor.go`)
- [ ] **Update every in-package `fake*`/`stub*` double** that implements the
      changed interface — they live in the `_test.go` files of the same
      package (see [Test Doubles](#test-doubles))
- [ ] **Run the full test suite**: `make test`
- [ ] **Search for other usages** across services that may depend on this interface
- [ ] **Update integration tests** that may depend on the interface behavior
- [ ] **Document the change** if it affects service contracts or behavior

---

## Test Doubles

MyFleet has no `mock/` directories and no generated mocks. The convention is
a small unexported `fake*`/`stub*` struct, declared in the `_test.go` file
that uses it, in the same package, asserted with stdlib `t.Fatalf`/`t.Errorf`.

Canonical example — `apps/auth-service/internal/user/processor_test.go:10-40`:

```go
// Note: a fake keyed by an opaque map cannot express which column the real
// query filters on, which is why the /auth/me login-loop bug was invisible
// here. The column-level guarantees live in provider_test.go, against a real
// database.
type fakeProvider struct {
    byID  map[string]Model
    bySub map[string]Model
}

func (f *fakeProvider) GetByID(id string) (Model, error) {
    if m, ok := f.byID[id]; ok {
        return m, nil
    }
    return Model{}, ErrNotFound
}

type fakeAdmin struct{ created, updated int }

func (f *fakeAdmin) Insert(m Model) (Model, error) { f.created++; return m, nil }
func (f *fakeAdmin) Update(m Model) (Model, error) { f.updated++; return m, nil }
```

(`fakeProvider` implements the rest of `Provider` the same way — see the
source file.) Same pattern elsewhere: `stubProvider`/`stubAdministrator` in
`apps/fleet-service/internal/invite/processor_test.go:16,354`; `fakeInbox`/
`fakeMembers`/`fakeGenerator` in `apps/notification-service/internal/consumer/consume_test.go:15,29,36`.

A double only needs to satisfy the interface it stands in for — the compiler
rejects an incomplete one at build time, so there's no manual
"every method implemented" step to track.

**No assertion library.** `stretchr/testify` appears only transitively in
`go.sum`, never imported — don't add it to match this guide.

### The DB-Backed Counterweight

A fake keyed by a map can express *that* a call happened, but not *which
column* a real query filters on. `apps/auth-service/internal/user/provider_test.go:11-17`
exists for exactly this reason:

```go
// These tests run against a real database rather than a fake Provider on
// purpose. The login-loop bug this file guards against was invisible to
// processor_test.go's fakeProvider, because that fake keys an opaque map: it
// cannot express WHICH COLUMN the real query filters on, which was the entire
// defect. GET /auth/me passed the JWT `sub` claim — our internal user id — to
// GetBySub, which filters on google_sub. The row was always missed, /auth/me
// always 404'd, and the SPA bounced every authenticated user back to login.
```

When a fake's simplification could hide a defect like this, add a
database-backed test alongside it (`setupTestDB`) — `administrator_db_test.go`
in `apps/fleet-service/internal/vehicle` is the same counterweight for a
different package.

### Executable Convention Checks

Some conventions are cheap enough to pin in a test instead of a doc line:
`apps/auth-service/internal/arch/arch_test.go:29`
(`TestNoPrincipalLiteralOutsideResolver`) guarantees `session.Principal` is
built in one place; `apps/fleet-service/internal/admin/arch_test.go:26,94,131,201`
and the matching pair (`:27,95`) in `media-service`/`notification-service`
`internal/admin/arch_test.go` guarantee every admin table has a manifest
entry. Neither enforces this guide's layering rules — they model pinning one
narrow invariant in a test that fails the build instead of drifting silently,
not proof the rest of this guide is mechanically checked.

---

## Test Execution Standards

### When to Run Tests

Run the **full test suite** before committing in these situations:

1. **Interface modifications** - Any change to a Processor or Provider interface
2. **Shared package changes** - Modifications to code used across multiple services
3. **Business logic changes** - Any processor or administrator function modification
4. **Breaking changes** - Changes that could affect callers or consumers
5. **Before creating a PR** - Always run full suite before pushing

### Running Tests

**Full test suite** — `make test` (`Makefile:11-12`), not a bare
`go test ./...` from the repo root (it skips the other modules `go.work`
lists):
```bash
make test
```

**Single package, race detection, no cache (fast iteration before `make test`):**
```bash
go test ./internal/vehicle/... -race -count=1
```

**Specific package, verbose, from that service's directory** (e.g.
`apps/fleet-service`):
```bash
go test ./internal/vehicle/... -v -count=1
```

Before opening a PR, run `make ci` (`Makefile:51`) — it also covers the
frontend and manifest rendering that a Go-only run would miss.

### Test Failure Protocol

When tests fail:

1. **Do not ignore** - Failing tests indicate broken contracts
2. **Understand the failure** - Read the error message completely
3. **Fix the root cause** - Don't just update tests to pass
4. **Re-run the full suite** (`make test`) - Verify the fix didn't break other tests
5. **Check dependent services** - Some changes may affect other microservices

---

## Pre-Commit Checklist

Before committing changes, especially to core business logic:

- [ ] Run `make test` and verify all tests pass
- [ ] Run `make build` to ensure no compilation errors
- [ ] If you added new business logic, ensure corresponding tests exist
- [ ] Review changed files for accidental debug code or commented-out logic
- [ ] Ensure no secrets, credentials, or sensitive data in code
- [ ] If the provider you touched takes a `context.Context` (2 of 19
      `provider.go` files — `invite` and `mediavariant`; the other 17,
      including `vehicle`, `fleet`, `user`, and `session`, have no
      `context.Context` parameter on their provider methods at all, so there
      is no context to attach and this rule does not apply to them), verify
      it runs its query on `db.WithContext(ctx)`, not a bare `db` call:
      `apps/fleet-service/internal/invite/provider.go:17-21` — "Every method
      takes the caller's context as its first argument and runs its query on
      db.WithContext(ctx). The *gorm.DB the provider holds is captured once
      at startup; without WithContext every query would run on that bare
      connection, so a client that disconnected mid-request would leave the
      query running, and no query would carry a deadline or join the
      request's trace."
- [ ] Run `make ci` before opening a PR

---

## Common Testing Pitfalls

### 1. Skipping the Full Test Suite
**Problem:** Only running tests in the modified package.
**Solution:** Always run `make test` to catch cross-package issues.

### 2. Using Cached Test Results
**Problem:** Tests pass due to cache, not because code is correct.
**Solution:** Use the `-count=1` flag on per-package runs to disable test caching.

### 3. Not Testing Error Paths
**Problem:** Only testing happy paths, errors go untested.
**Solution:** Write table-driven tests with both success and failure cases.

### 4. Ignoring Race Conditions
**Problem:** Tests pass normally but fail with `-race`.
**Solution:** `make test` already runs with `-race`; don't skip it in favor of a bare `go test ./...`.

---

## AI Coding Assistant Guidance

When using AI tools to modify code:

1. **Always request test execution** after interface changes
2. **Ask AI to update every `fake*`/`stub*` double** in the same package when
   interface methods are added or modified
3. **Request full test suite runs** (`make test`) not just single packages
4. **Verify AI updated every double that implements the changed interface** -
   search the package's `_test.go` files yourself
5. **Don't accept "tests will pass"** - require actual test execution output
