---
name: backend-dev-guidelines
description: Skill for creating and modifying MyFleet Go services — immutable domain models with fluent builders, Provider/Administrator data access over GORM, chi routing, and the hand-rolled JSON:API transport in packages/shared-go/server.
---


# Backend Dev Guidelines

## Purpose
Provide a composable entry point that activates when working on any Golang service. This skill aligns development and AI generation with architecture patterns and conventions.

## When to Use
Activate when working on:
- Any Go microservice
- Files: `model.go`, `entity.go`, `builder.go`, `processor.go`, `provider.go`, `administrator.go`, `resource.go`, `rest.go`
- REST JSON:API endpoints
- Testing domain logic, providers, or emission paths

---

## Quick Start Checklist
- [ ] Immutable **Model** with accessors
- [ ] **Entity** with GORM tags and migrations
- [ ] Fluent **Builder** enforcing invariants
- [ ] **Processor** orchestrating a Provider and an Administrator
- [ ] **Provider** interface for reads and **Administrator** interface for writes, each with a `New*(db)` constructor
- [ ] **Resource** file for route registration and handlers
- [ ] Table-driven **tests** for all logic layers


---

## Standard Implementation Workflow

**MANDATORY:** Follow this workflow for ALL code changes to ensure quality and prevent regressions.

### Implementation Steps

When modifying any service code:

1. **Implement changes** to primary files (model.go, processor.go, etc.)
2. **Update the in-package `fake*`/`stub*` doubles immediately** if any interface changed
   - Add the corresponding method to the double, declared in the `_test.go` file that uses it
   - There is no `mock/` directory convention — see [Testing Conventions](resources/testing-guide.md#interface-change-workflow) for details
3. **Verify each domain against the Commonly Missed Items Checklist** in [ai-guidance.md](resources/ai-guidance.md#commonly-missed-items-checklist) before moving to the next domain. Do not batch — check domain 1 fully, then domain 2, etc.
4. **Run tests BEFORE claiming completion**:
   ```bash
   make test
   ```
5. **Fix any failures** - Do NOT skip or ignore test failures
6. **Verify build**:
   ```bash
   make build
   ```
7. **Report test results** with actual command output, not assumptions

### Critical Rules

- **Never skip test execution** - Running tests is mandatory, not optional
- **Never assume tests will pass** - Always verify with actual execution
- **Always run the full test suite** (`make test`, `Makefile:11-12`) not just modified packages — a bare `go test ./...` from the repo root skips the other modules `go.work` lists
- **Always verify test output** before marking work complete

### When Tests Fail

If `make test` reports failures:

1. **Read the error message completely** - Understand what broke
2. **Check for a missing method on the `fake*`/`stub*` double** - most common cause of failures after an interface change
3. **Update the double to match the interface** - Add/modify methods as needed
4. **Re-run tests** - Verify the fix didn't break other tests
5. **Do not proceed** until all tests pass

See [Testing Conventions](resources/testing-guide.md) for comprehensive testing guidelines.

---

## Key Principles
1. **Immutability** — Models never mutate; all state changes yield new instances via `With*` methods that return a copy.
2. **Context Propagation** — `req.Context()` carries request-scoped identity and correlation ID (`auth.IdentityFromContext`, `telemetry.CorrelationIDFromContext`), not stashed in globals. Processors hold no `ctx` field — but that isn't a hard cap on processor state: `maintenanceschedule.Processor` also holds a `*gorm.DB`, an `ActivityRecorder`, and an `OverdueEmitter` for its overdue-transition hooks (`apps/fleet-service/internal/maintenanceschedule/processor.go:25-32`).
3. **Layer Separation** — Each file type has a clear single responsibility.
4. **Business Rules in the Processor, I/O in Injected Collaborators** — a processor enforces invariants and orchestrates a Provider (reads) and Administrator (writes); the I/O itself happens inside those injected collaborators, not inline in the processor.


---

## File Responsibilities

| File | Primary Responsibility | Key Dependencies |
|------|-------------------------|------------------|
| `model.go` | Domain model definition | None |
| `entity.go` | Database schema and migrations | GORM |
| `builder.go` | Fluent construction of valid models | Model |
| `processor.go` | Core business logic orchestration | Model, Provider, Administrator |
| `provider.go` | Read-only data access | GORM, Entity |
| `administrator.go` | Write operations (insert, update, soft-delete, restore) | GORM, Entity |
| `resource.go` | Route registration and handlers | REST, Processor |
| `rest.go` | JSON:API resource mappings | Model |

---


## Navigation Guide

| Topic | Reference |
|-------|------------|
| Architecture Overview | [resources/architecture-overview.md](resources/architecture-overview.md) |
| File Responsibilities | [resources/file-responsibilities.md](resources/file-responsibilities.md) |
| Model, Builder and Processor Patterns | [resources/patterns-functional.md](resources/patterns-functional.md) |
| Provider Pattern | [resources/patterns-provider.md](resources/patterns-provider.md) |
| REST JSON:API | [resources/patterns-rest-jsonapi.md](resources/patterns-rest-jsonapi.md) |
| Testing Conventions | [resources/testing-guide.md](resources/testing-guide.md) |
| **Service Scaffolding** | **[resources/scaffolding-checklist.md](resources/scaffolding-checklist.md)** |
| AI Code Guidance | [resources/ai-guidance.md](resources/ai-guidance.md) |
| Anti-Patterns | [resources/anti-patterns.md](resources/anti-patterns.md) |

---
