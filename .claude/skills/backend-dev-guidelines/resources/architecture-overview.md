
---
title: Architecture Overview
description: Core architectural principles of MyFleet Golang services.
---

# Architecture Overview

Services follow a strict layered design with immutability.

## Layers
1. **Domain Layer** — Core logic, models, and validation.
2. **Infrastructure Layer** — Database and external systems.
3. **Transport Layer** — REST endpoints.
4. **Application Layer** — Orchestration and configuration.

## Core Technologies
- Go 1.25 (`go.work:1` declares `go 1.25.0`; CI pins `go-version: '1.25'`
  at `.github/workflows/pr.yml:15,50`)
- chi router (`github.com/go-chi/chi/v5`, `apps/fleet-service/go.mod:6`)
- GORM ORM (PostgreSQL)
- JSON:API-shaped transport (hand-rolled envelope, no third-party marshaling
  library — see [patterns-rest-jsonapi.md](patterns-rest-jsonapi.md))
- Logrus structured logging
- OpenTelemetry tracing (`packages/shared-go/telemetry`; correlation IDs via
  `telemetry.CorrelationID` middleware and
  `telemetry.CorrelationIDFromContext`, `telemetry/correlation.go:30`)

## Principles

- **Immutability:** Domain models never mutate. See
  [patterns-functional.md](patterns-functional.md#immutability).
- **Separation of Concerns:** Domain logic isolated from persistence and transport.
- **Data access split:** `Provider` (reads) and `Administrator` (writes), each
  with a `New*` constructor — see
  [patterns-provider.md](patterns-provider.md).
- **Stateless Services:** All state in database, services are stateless.

## Sub-Domain / Action-Event Packages

Some services have lightweight sub-domain packages that record action events
(e.g., `vehiclemedia`, `mileage`, `purge`). These packages **still must
follow layer separation**:

- If the action is simple (single entity write), **fold it into the parent
  domain's processor** as a method rather than creating a separate package
  that violates layers.
- If a separate package is warranted, it must have its own `processor.go` and
  `administrator.go`. Handlers must not create entities directly or parse
  JSON manually.
- Sub-domain POST endpoints must use `server.RegisterInputHandler[T]`
  (`patterns-rest-jsonapi.md`), not a hand-rolled `json.NewDecoder`.

**Guideline:** Prefer fewer, well-structured packages over many thin packages
that cut corners on layer separation.

## Cross-Domain Orchestration

When a handler needs to coordinate across multiple domains (e.g., creating a
vehicle also records an activity entry and emits a `vehicle.created` event,
`apps/fleet-service/internal/vehicle/resource.go:29-31`):

- **Move the orchestration to the processor layer.** The owning domain's
  processor takes the collaborator via `With*` chaining or a constructor
  parameter and calls it — not the handler.
- The only exception is read-only aggregation handlers, which may call
  multiple processors directly. `apps/fleet-service/internal/dashboard` is
  the real example: it reads from the schedule, activity, and vehicle
  processors to assemble a summary. See [anti-patterns.md](anti-patterns.md)
  for the documented exception.

## Startup Example

Every service's `cmd/main.go` builds the database connection, wires
processors, and boots the server through the same `server` chain: `New` →
`Use` → `AddRouteInitializer` (called once per route group — internal routes
come as plain `func(chi.Router)` values, JWT-protected routes wrap a
`r.Group` with JWT middleware) → `Run()`, guarded by an
`if err := ...; err != nil` check. Reproduced from
`apps/fleet-service/cmd/main.go:46-64,266-308`, with the repetitive per-domain
lines inside the protected group elided (the migration list and the full
12-domain `InitializeRoutes` list are in the source; nothing here is
composed):

```go
db, err := database.Connect(log, database.SetMigrations(
    vehicle.Migration,
    vehiclemedia.Migration,
    // ...additional domain migrations
))
if err != nil {
    log.WithError(err).Fatal("db connect")
}

if err := server.New(log).
    Use(telemetry.CorrelationID).
    // Internal routes: no JWT, network-restricted (consumed by other services).
    AddRouteInitializer(membership.InitializeInternalRoutes(log, db)).
    AddRouteInitializer(maintenanceschedule.InitializeInternalRoutes(log, db)).
    AddRouteInitializer(invite.InitializeInternalRoutes(log, db, fleet.NewProvider(db))).
    // Protected routes: JWT required.
    AddRouteInitializer(func(r chi.Router) {
        r.Group(func(pr chi.Router) {
            pr.Use(authmw.JWT(keyfn, authmw.WithLogger(log)))
            fleet.InitializeRoutes(log, db, membershipAdmin, membershipProc)(pr)
            vehicle.InitializeRoutes(log, db, membershipProc, vehiclemediaProc, vehicleStatusDeps, activity.Record, emitVehicleCreated)(pr)
            // ...nine more domains' InitializeRoutes(pr), same group, same middleware
            dashboard.InitializeRoutes(log, db, scheduleProc, activityProc, vehicleProc)(pr)
        })
    }).
    // The admin tree: its own group, JWT and nothing else shared with the
    // tree above.
    AddRouteInitializer(func(r chi.Router) {
        r.Group(func(ar chi.Router) {
            ar.Use(authmw.JWT(keyfn, authmw.WithLogger(log)))
            admin.InitializeRoutes(log, adminProc)(ar)
        })
    }).
    AddRouteInitializer(func(r chi.Router) {
        r.Get("/healthz", health.Liveness())
        r.Get("/readyz", health.Readiness(func() error { d, _ := db.DB(); return d.Ping() }))
        r.Handle("/metrics", health.Metrics())
    }).
    Run(); err != nil {
    log.WithError(err).Fatal("server stopped")
}
```

`server.New(log)` returns `*Server`; `Use` and `AddRouteInitializer` each
return `*Server` too, so the calls chain
(`packages/shared-go/server/handler.go:21-36`). `AddRouteInitializer` is
called six times above, and that is the real count in `main.go` — three
internal-route registrations, one JWT-protected group, one admin group, and
one health/metrics registration. Besides the truncated migration list and a
shortened admin-tree comment (the full version explains the authorization
boundary — see `main.go:290-294`), the only other elision is the per-domain
`InitializeRoutes(pr)` lines inside the protected group, down from twelve
domains to three representative ones. Each call takes a `func(chi.Router)`,
the same shape `InitializeRoutes(...)` returns
(`patterns-rest-jsonapi.md`). `Run()` calls `Router()`, which runs every
queued initializer against the chi router (`handler.go:38-44`), then serves
on `PORT` (default `8080`) via `http.ListenAndServe`
(`packages/shared-go/server/server.go:39-44`).
