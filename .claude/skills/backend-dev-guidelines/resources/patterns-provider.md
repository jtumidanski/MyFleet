---
title: Data Access — Provider and Administrator
description: The per-domain read (Provider) and write (Administrator) contracts, their constructors, error translation, pagination, and transaction hooks.
---

# Data Access — Provider and Administrator

Every domain package splits database access in two: `provider.go` owns reads,
`administrator.go` owns writes. Both are **interfaces** with a `db`-backed
implementation and a `New*` constructor.

On the ordinary request path, `db.Create` / `db.Save` / `db.Delete` appear
only in `administrator.go` — `processor.go` and `resource.go` never call
them directly. Batch and maintenance operations may issue their own SQL
outside that split when they have a documented reason: `purge.go:37-51` runs
a raw `tx.Exec` hard-delete inside its own transaction so the package does
not need to import the admin manifest.

The interface is not ceremony — it is the seam the processor tests substitute.
See [testing-guide.md](testing-guide.md) for the `fake*` / `stub*` doubles that
plug into it.

## Provider (reads)

```go
// apps/fleet-service/internal/vehicle/provider.go
var ErrNotFound = errors.New("vehicle not found")

// Provider is the read-only interface for vehicle data access.
type Provider interface {
	GetByID(id string) (Model, error)
	ListByFleet(fleetID string, page server.Page) ([]Model, int, error)
}

type dbProvider struct{ db *gorm.DB }

// NewProvider returns a read-only Provider backed by the given database.
func NewProvider(db *gorm.DB) Provider { return &dbProvider{db: db} }
```

**Rules:**

- The interface returns **domain `Model`s**, never `Entity`s. `Make(e)` runs
  inside the provider.
- Methods return `(Model, error)` — eager, plain values. There is no lazy
  provider type.
- IDs are `string` UUIDs (`uuid.NewString()`, `builder.go:13`), never `uint32` <!-- ALLOW-VOCAB:G-15 -->
  and never `uuid.UUID`. <!-- ALLOW-VOCAB:G-14 -->
- Providers never modify state.

### Translate `gorm.ErrRecordNotFound` at the boundary

A raw `gorm.ErrRecordNotFound` reaching a handler maps to **500**, not 404 —
`server.StatusFor` does not recognise it. Translate it in the provider, where
the query is:

```go
func (p *dbProvider) GetByID(id string) (Model, error) {
	var e Entity
	if err := p.db.First(&e, "id = ? AND deleted_at IS NULL", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Model{}, ErrNotFound
		}
		return Model{}, err
	}
	return Make(e), nil
}
```

Each domain declares its own `ErrNotFound` sentinel. Map it to
`server.ErrNotFound` at the handler, or wrap it with `server.Detailed` — see
[patterns-rest-jsonapi.md](patterns-rest-jsonapi.md#error-handling).

### Pagination is a provider parameter

List methods take `server.Page` and return `(models, total, error)`. The total
is a separate `COUNT`, because the page query is offset-limited:

```go
func (p *dbProvider) ListByFleet(fleetID string, page server.Page) ([]Model, int, error) {
	var total int64
	if err := p.db.Model(&Entity{}).Where("fleet_id = ? AND deleted_at IS NULL", fleetID).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var es []Entity
	if err := p.db.Where("fleet_id = ? AND deleted_at IS NULL", fleetID).
		Offset(page.Offset()).Limit(page.Size).Find(&es).Error; err != nil {
		return nil, 0, err
	}
	out := make([]Model, 0, len(es))
	for _, e := range es {
		out = append(out, Make(e))
	}
	return out, int(total), nil
}
```

The handler builds the page with `server.ParsePage(req)` and the response meta
with `page.Meta(total)` (`packages/shared-go/server/pagination.go`).

### Accepted variant: `database.Query`

`packages/shared-go/database/query.go` offers a thin wrapper:

```go
type Provider[T any] func() (T, error)
func Query[T any](fetch func() (T, error)) Provider[T]
func SliceQuery[T any](fetch func() ([]T, error)) Provider[[]T]
```

`database.Provider[T]` is an unrelated type to the per-domain `Provider`
interface defined earlier in this file — same name, different package, no
relationship. Its own doc comment (`query.go:3`) still calls it "a lazy,
re-runnable data fetch", which is the "lazy evaluation" concept this file
tells you does not exist for the domain `Provider`; that phrasing describes
only this generic function wrapper, not the interface above.

Four domains wrap their query bodies in it and invoke immediately — note the
trailing `()`:

```go
// apps/auth-service/internal/user/provider.go:39-50
func (s *dbProvider) GetByID(id string) (Model, error) {
	return database.Query(func() (Model, error) {
		var e Entity
		if err := s.db.Where("id = ?", id).First(&e).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return Model{}, ErrNotFound
			}
			return Model{}, err
		}
		return Make(e), nil
	})()
}
```

Behaviourally identical to the plain form. **Both are acceptable**; the plain
form is the majority (15 of 19 `provider.go` files) and is what a new domain
should use. Do not convert existing code between the two.

## Administrator (writes)

```go
// apps/fleet-service/internal/vehicle/administrator.go:14-32
// Administrator is the write interface for vehicle data access.
// All mutations (inserts, updates, soft-delete, restore, primary-image) go here.
type Administrator interface {
	Insert(Model) (Model, error)
	InsertWithHooks(m Model, hooks ...TxHook) (Model, error)
	Update(Model) (Model, error)
	SoftDelete(id string) (Model, error)
	RestoreRow(id string) (Model, error)
	SetPrimaryImage(id, mediaID string) (Model, error)
	GetByIDIncludingDeleted(id string) (Model, error)
	UpdateCurrentMileage(vehicleID string, mileage int) error
}

type dbAdministrator struct{ db *gorm.DB }

// NewAdministrator returns an Administrator backed by the given database.
func NewAdministrator(db *gorm.DB) Administrator { return &dbAdministrator{db: db} }
```

**Rules:**

- Takes a `Model` and returns a `Model`. The administrator calls `m.ToEntity()`
  on the way in and `Make(e)` on the way out — the processor never sees an
  `Entity`.
- Every state change lives here. `db.Create` / `db.Save` / `db.Delete` must not
  appear in `processor.go` or `resource.go`.

```go
func (a *dbAdministrator) Insert(m Model) (Model, error) {
	e := m.ToEntity()
	if err := a.db.Create(&e).Error; err != nil {
		return Model{}, err
	}
	return Make(e), nil
}
```

### `db.Save` UPDATEs every column

`db.Save` writes **all** columns, so any column the `Model` does not carry is
silently zeroed on an ordinary write. The fix is to round-trip the field through
the `Model` even when nothing reads it, as `vehicle` does
(`apps/fleet-service/internal/vehicle/model.go:21-27`):

```go
// purgeOperationID must round-trip through the Model: Administrator writes
// with db.Save, which UPDATEs every column, so a field the Model does not
// carry is silently zeroed on any ordinary write. Zeroing THIS one detaches
// the row from the purge that owns it — it stays soft-deleted but becomes
// unreachable by both restore and reap. entityguard caught it.
purgeOperationID *string
purgeAfter       *time.Time
```

`packages/shared-go/database/entityguard` is the executable guard for this
class of bug. When you add a column to an `Entity`, add the matching field to
the `Model` in the same commit.

### `TxHook` — side effects that must commit with the write

When work must commit or roll back atomically with the insert, pass it as a
hook rather than running it after the write returns
(`apps/fleet-service/internal/vehicle/administrator.go:9-12,42-61`):

```go
// TxHook runs side-effecting work (activity append + event emission) on the same
// transaction as the vehicle insert, so the writes commit/rollback atomically.
// Errors are FATAL: they roll back the whole transaction.
type TxHook func(tx *gorm.DB, created Model) error

func (a *dbAdministrator) InsertWithHooks(m Model, hooks ...TxHook) (Model, error) {
	var created Model
	err := a.db.Transaction(func(tx *gorm.DB) error {
		e := m.ToEntity()
		if err := tx.Create(&e).Error; err != nil {
			return err
		}
		created = Make(e)
		for _, h := range hooks {
			if err := h(tx, created); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Model{}, err
	}
	return created, nil
}
```

A hook error is fatal to the whole transaction. Do not use a hook for work that
is allowed to fail independently of the write — do that in the processor after
the administrator returns.

## Why this separation

- **Testability** — the processor takes `Provider` and `Administrator` as
  constructor arguments, so a test substitutes a `fakeProvider` / `fakeAdmin`
  with no database.
- **Clear intent** — a reviewer finds every state change by opening one file.
- **Single responsibility** — `provider.go` has no way to write.
