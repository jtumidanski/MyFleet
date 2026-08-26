# MyFleet Service Documentation Contract (DOCS.md)

## 1. Purpose

This document defines the mandatory documentation structure, scope, and
constraints for every MyFleet Go service under `apps/`.

Documentation is a first-class artifact and must:
- Reflect code as implemented
- Follow strict file responsibilities
- Avoid inference, improvement, or speculation
- Remain consistent across services

This document is authoritative for what may and may not appear in
per-service documentation.

**This contract does not itself produce any per-service documentation.**
It prescribes where such documentation belongs (`apps/<svc>/docs/...`) and
what each file may contain; filling in those files for any given service is
separate work, done under this contract, not by this document.

---

## 2. Core Principles

### 2.1 Documentation mirrors architecture

Documentation structure must directly reflect:
- Domain boundaries
- File semantics
- Transport mechanisms
- Storage models

### 2.2 Documentation is descriptive, not prescriptive

Documentation:
- Describes what exists
- Never describes what should exist
- Never proposes alternatives
- Never improves or rationalizes design choices

### 2.3 Code is the single source of truth

Documentation is derived from code, never the other way around. When
documentation and code disagree, the code is correct and the documentation
is out of date. Documenting agents and human authors must read the actual
`.go` files before writing or updating a document — never write from memory
or from what a feature was originally intended to do.

### 2.4 Document only what exists

Never infer intent, never describe future or planned behavior, and never
document a capability because it seems like it should exist. If a file,
endpoint, topic, or table is not present in the code today, it does not
belong in the documentation.

### 2.5 No cross-layer leakage

Each documentation artifact has a single concern. Cross-references between
files are allowed. Explanations that duplicate another file's concern are
not.

---

## 3. Required Documentation Artifacts

Each service under `apps/<svc>/` MUST contain the following files:

```
apps/<svc>/README.md
apps/<svc>/docs/domain.md
apps/<svc>/docs/rest.md
apps/<svc>/docs/storage.md
```

`apps/<svc>/docs/kafka.md` is REQUIRED for any service that produces or
consumes Kafka messages, and MUST NOT exist for a service that does
neither. As of this writing, the services that import
`packages/shared-go/events` (the shared producer/consumer wrapper around
`segmentio/kafka-go`) are `apps/fleet-service`, `apps/notification-service`,
and `apps/media-service`; `packages/shared-go` itself hosts the shared
Kafka code. `apps/auth-service` and `apps/web` do not carry Kafka and MUST
NOT have a `docs/kafka.md`. This list reflects the code at the time this
contract was written — always verify against each service's `go.mod` and
imports rather than trusting this paragraph, since services and their
dependencies change over time.

Optional artifacts MAY exist only if explicitly justified by something
present in the code:

```
apps/<svc>/docs/migrations.md
```

No other top-level `docs/` file may be added to a service without updating
this contract first.

**None of the paths above exist yet.** This section is a prescription for
where documentation belongs when it is written, not a claim that it has
been written. At the time this contract was authored, no service under
`apps/` has a `docs/` directory.

---

## 4. README.md

### Purpose

High-level orientation for humans landing on the service for the first
time.

### Allowed Content

- Service responsibility (1-2 paragraphs)
- External dependencies (database, Kafka, Redis, other services, etc.)
- Runtime configuration overview
- Links to the deeper documents in `docs/`

### Forbidden Content

- Business rules
- Domain invariants
- Kafka message schemas
- JSON:API request or response details
- Database schema definitions

---

## 5. docs/domain.md

### Purpose

Describe domain logic and invariants, independent of transport or storage.

### Required Structure

```
## <domain-name>

### Responsibility
### Core Models
### Invariants
### State Transitions (if applicable)
### Processors
```

### Allowed Content

- Domain model responsibilities (the immutable models built via fluent
  builders, e.g. `model.go` / `builder.go` in a service's `internal/<domain>`
  package)
- Immutable model invariants
- Processor responsibilities (`processor.go`)
- High-level state transitions, where the domain has them

### Forbidden Content

- JSON:API resource or endpoint details
- Kafka topics or payloads
- Database tables or queries
- Infrastructure or deployment concerns

---

## 6. docs/rest.md

### Purpose

Document the public HTTP interface only, as implemented by the hand-rolled
JSON:API transport in `packages/shared-go/server` (see `jsonapi.go`,
`handler.go`, `errors.go`, `pagination.go`). This is not a generic REST
description: MyFleet services expose JSON:API resources, not ad-hoc JSON
payloads, so this file documents resource types and attribute sets rather
than free-form request/response shapes. The transport's `Document` carries
only `data`, `meta`, and `links` (`jsonapi.go`) — there is no `included`
member and no compound documents, so a service's `rest.md` must not
document either. A `Resource` does carry a `relationships` field, but no
MyFleet service currently populates it; document relationships only where
a service actually emits them.

### Required Structure

```
## Resources

### <resource-type>

- Attributes
- Relationships, if the service populates them

## Endpoints

### <METHOD> <PATH>

- Parameters
- Request resource / attributes
- Response resource / attributes
- Error conditions
```

### Allowed Content

- HTTP methods and paths (`resource.go`, `rest.go`)
- JSON:API resource types and attributes
- Relationships (`Resource.Relationships`), only where a service actually
  populates them — the field exists but is unpopulated in every service
  today
- Validation rules
- Error codes and meanings (as produced by
  `packages/shared-go/server/errors.go`)

### Forbidden Content

- Processor logic
- Database queries
- Kafka emission details
- Domain invariants

---

## 7. docs/storage.md

### Purpose

Describe persistent storage representation, not access logic.

### Required Structure

```
## Tables
## Relationships
## Indexes
## Migration Rules
```

### Allowed Content

- Table names
- Columns and types
- Relationships
- Indexing strategy
- Migration guarantees, if the service has an optional `docs/migrations.md`
  or migration files worth summarizing

### Forbidden Content

- Query logic (Provider/Administrator implementation details)
- Caching strategies
- Business rules
- JSON:API or Kafka references

---

## 8. docs/kafka.md

### Purpose

Document the Kafka integration surface only, for services that produce or
consume Kafka messages via `packages/shared-go/events`.

### Required Structure

```
## Topics Consumed
## Topics Produced
## Message Types
## Transaction Semantics
```

### Allowed Content

- Topic names
- Direction (produced or consumed)
- Message struct names
- Required headers
- Ordering or partitioning notes

### Forbidden Content

- Business logic explanations
- Processor behavior
- State transitions
- Retry or compensation logic

---

## 9. File-to-Documentation Mapping Rules

| Code Artifact | Documentation |
|---|---|
| `model.go` | `docs/domain.md` |
| `builder.go` | `docs/domain.md` |
| `processor.go` | `docs/domain.md` |
| `producer.go` (`packages/shared-go/events`) | `docs/kafka.md` |
| `consumer.go` (`packages/shared-go/events`) | `docs/kafka.md` |
| `resource.go` | `docs/rest.md` |
| `rest.go` | `docs/rest.md` |
| `entity.go` | `docs/storage.md` |

If a file exists without a corresponding documentation entry, the
documentation is incomplete.

---

## 10. Documentation Update Rules

### Required Updates

Documentation MUST be updated when:
- A new domain is added
- A new Kafka topic is produced or consumed
- A JSON:API resource or endpoint is added or modified
- A database schema changes

### Forbidden Updates

Documentation MUST NOT be updated:
- During design-only planning
- Based on intended or future behavior
- To explain implementation rationale

---

## 11. AI Usage Rules

When documentation is generated or updated by an AI agent:

### The agent MUST

- Follow this document exactly
- Use code as the source of truth
- Ask before adding new sections
- Preserve existing structure and tone

### The agent MUST NOT

- Infer missing behavior
- Improve clarity beyond restating facts
- Merge sections across concerns
- Reorganize files without instruction

---

## 12. Validation Criteria

Documentation for a service is considered complete and valid if:
- All required files exist for that service (including `docs/kafka.md`
  when and only when the service carries Kafka)
- All required sections are present in each file
- No forbidden content is included in any file
- All code-to-documentation mappings in Section 9 are satisfied

---

## 13. Non-Goals

This documentation contract does not:
- Teach Go
- Explain Kafka or JSON:API fundamentals
- Justify architectural decisions
- Serve as onboarding training material
