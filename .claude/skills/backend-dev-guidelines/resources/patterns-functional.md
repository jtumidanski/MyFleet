
---
title: Model, Builder and Processor Patterns
description: Immutable models, builders, and processor constructors for services.
---

# Model, Builder and Processor Patterns


## Immutability
- Domain models have private fields.
- Public getters expose read-only state.
- Instances are built via builders (see Builder Pattern below), and modified
  via `With*` methods that return a copy with the changed field(s) set —
  never via in-place mutation of the receiver. Most `With*` methods change a
  single field, but several change more than one together where the fields
  are related: `vehicle.Model.WithSoftDelete` sets `deletedAt` and
  `purgeAfter` together (`model.go:74-79`), `user.Model.WithLogin` sets four
  fields together (`apps/auth-service/internal/user/model.go:45-48`), and
  `maintenanceschedule.Model.WithRecurrence` (`model.go:52-57`),
  `WithLastCompleted` (`:63-67`), `WithNextDue` (`:70-73`), and `WithStatus`
  (`:77-80`) each set two or three fields together
  (`apps/fleet-service/internal/maintenanceschedule/model.go`). Example
  (`apps/fleet-service/internal/vehicle/model.go:49-53`):

```go
// WithNickname returns a copy with the nickname changed.
func (m Model) WithNickname(nickname string) Model {
    m.nickname = nickname
    return m
}
```


## Builder Pattern
```go
m, err := NewBuilder().
  SetNickname("Example").
  SetMake("Honda").
  SetModel("Civic").
  SetYear(2020).
  Build()
```
- Validation occurs in `Build()` (`apps/fleet-service/internal/vehicle/builder.go:26-31`).
- Builders are fluent and chainable.
- `NewBuilder()` takes no ID parameter — the ID is generated inside the
  constructor (`uuid.NewString()`, `apps/fleet-service/internal/vehicle/builder.go:13`),
  a pattern every domain's builder follows.
- To change a field on an existing instance rather than construct a new one,
  use the model's `With*` methods (Immutability, above) — there is no
  `Model.Builder()` re-entry point into the builder.


## Processor Constructor Pattern

A processor takes its *required* collaborators as constructor parameters and
holds no `context.Context`. (Optional collaborators are supplied later —
see Optional dependencies, below.) The common shape — 8 of the 19 `NewProcessor` functions in
this tree (`vehicle`, `user`, `fleet`, `maintenancerecord`,
`maintenancecategory`, `maintenanceschedule`, `vehiclemedia`, `preferences`) —
is `NewProcessor(log, Provider, Administrator)`:

```go
// apps/fleet-service/internal/maintenancerecord/processor.go:19
func NewProcessor(log logrus.FieldLogger, p Provider, a Administrator) *Processor {
    return &Processor{log: log, p: p, a: a}
}
```

This is the common case, not a universal signature. Other domains take fewer
or different collaborators: `activity`, `fuel`, `membership`, `invite`, and
`dashboard` take `(log, Provider)` only; `mileage`, `notification`, `admin`,
`session`, and `mediaobject` each take their own collaborator set. One
processor takes no logger at all — `apps/auth-service/internal/oidc/processor.go:28`
is `NewProcessor(clientID, clientSecret, redirectURL string)`.

Every processor that does take a logger takes it as the `logrus.FieldLogger`
interface, never the concrete `*logrus.Logger` (`DOM-06`). `InitializeRoutes`
receives `log logrus.FieldLogger` and passes it straight into `NewProcessor`
(`apps/fleet-service/internal/vehicle/resource.go:28-29`) — there is no
handler-dependency logger accessor in this codebase; the logger flows through
as a plain parameter.

One processor, `maintenanceschedule`, does hold a `*gorm.DB` field, but it is
never a constructor parameter: it is injected after construction via
`WithOverdueHooks`, for a single recompute-job codepath that needs a
transaction handle (`apps/fleet-service/internal/maintenanceschedule/processor.go:29,40`).

### Optional dependencies

Two mechanisms exist for collaborators that are not required at construction
time:

- **`With*` chaining**, returning the processor for fluent use — the common
  case:
  ```go
  // apps/fleet-service/internal/vehicle/resource.go:29-31
  proc := NewProcessor(log, NewProvider(db), NewAdministrator(db)).
      WithActivityRecorder(record).
      WithEventEmitter(emit)
  ```
- **`ProcessorOption` functional options**, threaded through the variadic
  constructor itself — used in exactly one domain, `mediaobject`, where a
  lazy card-variant generator is genuinely optional
  (`MEDIA_LAZY_VARIANT_CONCURRENCY=0` wires none, and that is a supported
  deployment, not a degraded one):
  ```go
  // apps/media-service/internal/mediaobject/processor.go:191-202
  type ProcessorOption func(*Processor)

  func WithCardGenerator(g CardGenerator) ProcessorOption {
      return func(pr *Processor) { pr.cards = g }
  }

  func NewProcessor(log logrus.FieldLogger, p Provider, a Administrator, st ObjectStore, variants VariantLookup, allow Allowlist, opts ...ProcessorOption) *Processor
  ```

Default to `With*` chaining for post-construction wiring (recorders,
emitters, hooks). Reach for `ProcessorOption` only when the option must be
resolved at construction time, as `mediaobject` does — it is not a general
convention across domains.
