# Task 006 — Implementation Context

Companion to [`plan.md`](./plan.md). Everything here was verified against the source in this
worktree on 2026-08-02; nothing is cited from memory.

## The defect in one paragraph

Every `Administrator.Update` in this repo is `e := m.ToEntity(); db.Save(&e)`. `db.Save` with a
set primary key emits `UPDATE … SET <every column>`. GORM stamps `UpdatedAt` itself via its
`autoUpdateTime` callback, but gives `CreatedAt` and `DeletedAt` no such protection — so any
column `ToEntity()` leaves at its zero value is written as zero on every update.

## The five `Save` sites (the complete set)

`git grep -n "db.Save\|tx.Save" -- 'apps/**/*.go'` returns exactly these:

| File:line | Package | State before this task |
|---|---|---|
| `apps/auth-service/internal/user/administrator.go:26` | `user` | Safe — `Entity.CreatedAt` already tagged `gorm:"<-:create"` on `main` |
| `apps/fleet-service/internal/fleet/administrator.go:27` | `fleet` | **Lossy** — `ToEntity()` drops `CreatedAt` *and* `DeletedAt` |
| `apps/fleet-service/internal/vehicle/administrator.go:65` | `vehicle` | **Lossy** — `ToEntity()` drops `CreatedAt`; user-visible symptom |
| `apps/media-service/internal/mediaobject/administrator.go:35` | `mediaobject` | Lossless by luck — `ToEntity()` assigns every field |
| `apps/media-service/internal/mediaobject/administrator.go:47` | `mediaobject` | Same, inside `UpdateInTx`'s transaction |

## Files that change

| File | Change |
|---|---|
| `packages/shared-go/database/entityguard/entityguard.go` | **New.** `go/ast` analyzer, `Analyze(root string) ([]Finding, error)` |
| `packages/shared-go/database/entityguard/entityguard_test.go` | **New.** Fixture-driven unit tests (fixtures written to `t.TempDir()`) |
| `apps/fleet-service/internal/vehicle/entity.go` | Tag `Entity.CreatedAt` `gorm:"<-:create"`; assign `CreatedAt` in `ToEntity()` |
| `apps/fleet-service/internal/vehicle/administrator_db_test.go` | **New.** 3 behavioural tests |
| `apps/fleet-service/internal/fleet/model.go` | Add `deletedAt *time.Time` + `DeletedAt()` accessor |
| `apps/fleet-service/internal/fleet/entity.go` | Tag `CreatedAt`; assign `CreatedAt` + `DeletedAt` in `ToEntity()`; add `toGormDeletedAt` helper |
| `apps/fleet-service/internal/fleet/administrator_db_test.go` | **New.** 2 behavioural tests |
| `apps/media-service/internal/mediaobject/entity.go` | Tag `Entity.CreatedAt` `gorm:"<-:create"` |
| `apps/media-service/internal/mediaobject/administrator_db_test.go` | **New.** 2 behavioural tests |
| `apps/auth-service/internal/user/administrator_db_test.go` | **New.** 1 pinning test (no production change) |
| `apps/{auth,fleet,media,notification}-service/cmd/entityguard_test.go` | **New.** One three-line guard invocation per service |
| `docs/runbooks/created-at-repair.md` | **New.** Operator runbook for the data repair |

No schema change, no API change, no frontend change. `AutoMigrate` output is unaffected —
`<-:create` is a write-permission tag and takes no part in DDL generation.

## Analyzer preflight (measured, not predicted)

The analyzer in `plan.md` Task 1 was built and run in a scratch module while planning. Against the
**unfixed** tree it reports exactly three findings, all in `fleet-service`:

| Package | Field | Column | Save site |
|---|---|---|---|
| `fleet` | `Entity.CreatedAt` | `created_at` | `fleet/administrator.go:27` |
| `fleet` | `Entity.DeletedAt` | `deleted_at` | `fleet/administrator.go:27` |
| `vehicle` | `Entity.CreatedAt` | `created_at` | `vehicle/administrator.go:65` |

`auth-service`, `media-service`, and `notification-service` report zero — independently confirming
that `user` and `mediaobject` really are clean today, so Tasks 4 and 5 are hardening and pinning
rather than fixes. The seven latent domains correctly produce nothing, because none has a `Save`.

## Key design decisions carried into the plan

1. **Two layers, not one** (design §4). `gorm:"<-:create"` protects the *column*; assigning the
   field in `ToEntity()` protects the *`Model` returned by `Make(e)` after the write*. Each fixes
   something the other cannot, so `vehicle` and `fleet` get both. The analyzer accepts either as
   sufficient, so code and guard agree on what "protected" means.
2. **Static analyzer, not a per-domain helper** (design §6.1). The defect appears in packages
   nobody thought to test, so a guard requiring per-package opt-in is absent exactly where it is
   needed. One test per *service* covers every package under that service's `internal/`, forever.
3. **`fleet.DeletedAt` is carried, never tagged** (design §5.2, V6). Tagging a `gorm.DeletedAt`
   `<-:create` makes a restore via `Updates(map{...})` return `err=nil, RowsAffected=1` while the
   row stays deleted — a silent failure strictly worse than the bug being fixed.
4. **The repair is a runbook, not a migration** (design §8, V7). GORM *silently discards* writes
   to a `<-:create` column, so any ORM-based repair would report success and change nothing. Raw
   SQL only.

## GORM behaviours the tests depend on

Measured against GORM v1.31.1 in this repo (design §2):

- `<-:create` does **not** block GORM's auto-populate of `CreatedAt` on `Create` (V1).
- `<-:create` holds `created_at` through a full-column `Save` while the real change still lands,
  and `updated_at` is still re-stamped (V2).
- SQLite reproduces the original bug faithfully (V3) — the behavioural tests are not vacuous.
- A narrowed `Updates(map{...})` is unaffected by a `<-:create` field elsewhere on the struct
  (V7). This is why `vehicle.RestoreRow` / `SoftDelete` / `SetPrimaryImage` survive the tag —
  and it is still covered by test, because that is exactly the claim a tag change falsifies
  quietly.

## Test harness constraints

Schema-qualified `TableName()`s (`fleet.vehicles`, `auth.users`, `media.media_objects`) target
Postgres. SQLite has no schemas, so every DB test does `ATTACH DATABASE ':memory:' AS <schema>`
and then **explicit DDL, never `AutoMigrate`**: GORM emits `CREATE INDEX` with the schema prefix
stripped under SQLite, which cannot resolve against an attached schema, and `vehicle`, `fleet`,
and `mediaobject` all carry `index` tags.

Existing harnesses to copy or reuse:

- `apps/fleet-service/internal/maintenanceschedule/completion_db_test.go:15-56` — the
  `fleet.vehicles` DDL, verbatim.
- `apps/auth-service/internal/user/provider_test.go:18-52` — `newTestDB`, **reusable as-is**
  (same package).
- `apps/media-service/internal/mediaobject/processor_test.go:108-147` — `newConfirmTestDB`,
  **reusable as-is** (same package).

Both existing harnesses carry a `KEEP IN SYNC WITH entity.go` comment; new ones must too.

## Build / verification commands

```sh
make build        # go build github.com/jtumidanski/myfleet/...
make vet          # go vet  github.com/jtumidanski/myfleet/...
make test         # go test -race github.com/jtumidanski/myfleet/...
make lint-check   # ./tools/lint.sh --check  (golangci-lint v2, standard group + gofumpt/goimports)
make ci           # lint-check vet test build fe-test fe-build manifests carfax-template
```

Baseline to hold: **37 packages ok / 0 failures**. The new test files add packages to that count;
zero failures is the invariant.

## Module layout

Six Go modules joined by a root `go.work` (`apps/{auth,fleet,media,notification}-service`,
`packages/{dto-go,shared-go}`). All four services already `require` `packages/shared-go`, so the
new `database/entityguard` package needs **no** `go.mod` change. `gorm.io/gorm` is already a
direct dependency of `shared-go`, so importing `gorm.io/gorm/schema` for column-name derivation
adds nothing new either.

Service entrypoints are `apps/<svc>/cmd/main.go`, `package main` — that is where each guard test
file goes, so `Analyze("../internal")` resolves from the test's working directory.

## Data repair — what the operator must understand

Three tables, three proxies, all *upper bounds with a known bias* rather than measurements:

| Table | Proxy | Accuracy |
|---|---|---|
| `fleet.vehicles` | `MIN(created_at)` of its `vehicle.created` activity event | Strongest — written in the same transaction as the insert |
| `fleet.fleets` | `MIN(created_at)` of its memberships | Exact where the owner membership survives; skews late if it was hard-deleted |
| `auth.users` | `MIN(created_at)` of its refresh tokens | Weakest — tokens expire and get pruned, so it can post-date signup by months |

The proxies exist at all because the tables supplying them are insert-only, and insert-only
tables were never touched by this bug.

All four services share one Postgres database (`myfleet`) and differ only by `search_path` —
verified at `deploy/k8s/secrets.example.yaml:18,34,44,57`. So the cross-schema single-transaction
repair is valid, but it must be run by a role that can see all three schemas, not by a service's
own connection.

**Ordering is a hard constraint:** deploy the code fix first, repair second. Repairing first means
the next `PATCH` re-zeroes the row.

## Known scope boundaries

- Narrowing `Administrator.Update` to `db.Model(...).Select(cols).Updates(...)` — the root fix —
  is explicitly out of scope (PRD §2), recorded as a follow-up.
- The guard does not cover a *fifth service* created later with no guard test (design §6.4).
  Accepted deliberately; a new service is a reviewed event, a new domain package is routine.
- The seven latent domains (`session`, `fuel`, `invite`, `maintenancerecord`,
  `maintenanceschedule`, `membership`, `vehiclemedia`) are **not** pre-emptively tagged. None has
  a `Save`, so the analyzer covers them the day one acquires it.
