# `created_at` Wipe Sweep — Product Requirements Document

Version: v1
Status: Draft
Created: 2026-08-02
Issue: [#7](https://github.com/jtumidanski/myfleet/issues/7) — "auth-service: created_at is wiped to zero on every re-login"
---

## 1. Overview

GitHub issue #7 reports that `auth.users.created_at` is silently overwritten with the
zero value (`0001-01-01`) on every re-login. The mechanism is a mismatch between two
halves of the persistence layer: `Model` is the immutable domain type and does not carry
`createdAt`, so `Model.ToEntity()` returns an `Entity` whose `CreatedAt` is the zero
`time.Time`; `Administrator.Update` then persists that entity with `db.Save(&e)`, which
GORM expands into an UPDATE of **every** column. GORM auto-manages `UpdatedAt` on update
but applies no such protection to `CreatedAt`, so the zero value is written straight to
the column.

**The auth-service instance described in the issue is already fixed on `main`.** Commit
`7642e28` ("fix(review): address pre-PR review findings on dark-mode-branding") added
`gorm:"<-:create"` to `user.Entity.CreatedAt`, which tells GORM to include the column on
INSERT and exclude it from every UPDATE. That fix is correct and needs no rework.

What remains unaddressed is the issue's closing paragraph — the "wider concern" that the
same `db.Save`-writes-every-column shape exists in other services, and that any domain
whose `Model` omits a persisted column has the same defect. A full sweep of the four Go
services confirms this concern is justified: there are two further **live** instances, one
of which has a user-visible symptom that the auth-service instance never had. This task
closes those, hardens the one correct-but-fragile site, and adds a regression guard so the
defect class cannot silently return.

## 2. Goals

Primary goals:

- Eliminate every live instance of the full-column-write column-wipe defect across all four
  Go services.
- Restore correct "Inactive vs. Healthy" status derivation for vehicles that have been
  edited (see §4.1 — the one user-facing symptom).
- Repair, as far as the data allows, rows whose `created_at` has already been zeroed in the
  bee cluster.
- Make the defect class non-recurring via an automated regression guard, so a future domain
  that adds a `Save` write path cannot reintroduce it unnoticed.

Non-goals:

- Re-fixing auth-service. `user.Entity.CreatedAt` already carries `<-:create` on `main`;
  this task only adds a regression test pinning that behaviour.
- Exposing `createdAt` in any API response that does not already expose it. The repair is
  to the persistence layer only; no JSON:API attribute changes.
- Refactoring the `Administrator` interface shape, the `Model`/`Entity` split, or the
  builder pattern across services. The defect is fixed within the existing architecture.
- Migrating `Administrator.Update` implementations to narrowed
  `db.Model(...).Select(cols).Updates(...)` writes. This is recorded as a follow-up in §9,
  not done here.
- Addressing the sqlite-test-harness vs. PostgreSQL behavioural differences noted in the
  issue's reproduction section.

## 3. User Stories

- As a **vehicle owner**, I want a vehicle I have edited to keep reporting its true status,
  so that renaming my car does not make the dashboard claim it is "Inactive".
- As a **fleet owner**, I want my fleet's creation date to survive a rename, so that
  fleet-age data is not destroyed by an unrelated edit.
- As an **operator**, I want the already-corrupted rows on the bee cluster repaired to the
  most accurate value the surviving data supports, and I want to know exactly which rows
  could not be recovered.
- As a **developer**, I want a test that fails loudly if I add a domain whose entity
  round-trip drops a persisted column, so that I do not ship this bug a fourth time.

## 4. Functional Requirements

### 4.1 Confirmed live defects (must fix)

A sweep of all `db.Save` / `tx.Save` call sites in `apps/` found exactly four. Every other
write path in the repository uses `Create` (insert-only, where GORM correctly auto-populates
`CreatedAt`) or a narrowed `db.Model(...).Updates(map[string]any{...})`, both of which are
safe.

| # | Call site | Columns dropped by `ToEntity()` | Status |
|---|---|---|---|
| 1 | `apps/auth-service/internal/user/administrator.go:24` | `CreatedAt` | **Already fixed** on `main` via `<-:create` |
| 2 | `apps/fleet-service/internal/vehicle/administrator.go:65` | `CreatedAt` | **Live — user-facing** |
| 3 | `apps/fleet-service/internal/fleet/administrator.go:27` | `CreatedAt`, `DeletedAt` | **Live** |
| 4 | `apps/media-service/internal/mediaobject/administrator.go:35` and `:47` (`UpdateInTx`) | none — round-trip is lossless | Correct today, fragile by construction |

**FR-FIX-1 — `vehicle.created_at` must survive an update.**
`vehicle.Entity.CreatedAt` must be excluded from UPDATE statements. After any number of
`Administrator.Update` calls, the row's `created_at` must equal the value written at INSERT.

This is the one instance with a live user-visible symptom, and it is why this task is not
merely hygiene. `apps/fleet-service/internal/vehicle/status.go:45-52` derives a vehicle's
read-only status and falls back to the vehicle's creation time when it has no activity
records:

```go
// Guard against a missing activity record: fall back to the vehicle's
// creation time so a brand-new vehicle is "Healthy", not "Inactive".
if last.IsZero() {
    last = m.CreatedAt()
}
```

The reachable failure path is: create a vehicle → `PATCH /vehicles/:id` (any field;
`resource.go:147` → `processor.go:110` → `Administrator.Update` → `db.Save`) → `created_at`
becomes `0001-01-01`. If that vehicle has no activity rows, `DeriveStatus` now compares a
zero timestamp against `inactivityDays` and returns **"Inactive"** where it previously
returned "Healthy" — permanently, and with the true creation timestamp unrecoverable from
the row itself.

**FR-FIX-2 — `fleet.created_at` must survive an update.**
`fleet.Entity.CreatedAt` must be excluded from UPDATE statements. The reachable path is
`processor.go:40` (`Update(m.WithName(name))`) — i.e. renaming a fleet destroys its
creation date. No current read consumes `fleet.CreatedAt()`, so there is no user-visible
symptom today, but `Model.CreatedAt()` is exported at `fleet/model.go:17` and the data loss
is unrecoverable once it happens.

**FR-FIX-3 — `fleet.deleted_at` must not be resurrected by an update.**
`fleet.Entity.DeletedAt` is a `gorm.DeletedAt` and is likewise dropped by `ToEntity()`, so a
full-column `Save` writes NULL over any soft-delete tombstone. In practice GORM's
soft-delete scope means a deleted fleet is never loaded to be renamed in the first place, so
this is currently unreachable — but it is the same defect shape on the same struct and must
be closed alongside FR-FIX-2 rather than left as a trap for whoever later adds an
`Unscoped()` read path.

**FR-FIX-4 — `mediaobject` must be hardened.**
`mediaobject.ToEntity()` is presently lossless only because `Model` happens to carry
`createdAt`. Its correctness is incidental, not structural, and it sits behind two `Save`
call sites (`Update` and `UpdateInTx`). Apply the same protection so the column is safe
regardless of future `Model` changes. This is defence in depth: it must not change current
behaviour, and a test must assert that.

### 4.2 Latent instances (must be documented, not necessarily changed)

Seven further domains have a `ToEntity()` that drops `CreatedAt` (and usually `UpdatedAt`),
but are written **only** via `Create` or narrowed `Updates(map)`, so they are not currently
buggy:

`auth-service/internal/session`, `fleet-service/internal/fuel`,
`fleet-service/internal/invite`, `fleet-service/internal/maintenancerecord`,
`fleet-service/internal/maintenanceschedule`, `fleet-service/internal/membership`,
`fleet-service/internal/vehiclemedia`.

**FR-LATENT-1.** Each of these becomes a live data-loss bug the moment anyone adds a
`Save`-based `Update`. The regression guard in §4.3 must cover them so that such a change
fails CI rather than shipping. Whether to also apply `<-:create` pre-emptively to all seven
is left to design (§9, Q3); the guard is the requirement, the pre-emptive tagging is
optional.

Three domains are already lossless and need nothing: `activity`, `mileage`, `mediavariant`,
plus `notification` (all four carry `CreatedAt` through `ToEntity()`).

### 4.3 Regression guard

**FR-GUARD-1.** Adding a `Save`-based write path to a domain whose entity round-trip drops
a persisted column must fail an automated check.

**FR-GUARD-2.** The guard must be specific enough to name the offending domain and column,
not merely fail with "a test broke". A developer seeing the failure must be able to act on
it without reading this PRD.

**FR-GUARD-3.** For each of the four `Save` sites, a test must persist a row, update it, and
assert `created_at` is unchanged and non-zero. These are behavioural tests against the real
GORM write path — not assertions about struct tags, which would pass while the column still
got wiped.

Two candidate mechanisms, to be chosen at design time (§9, Q4):

- a shared round-trip helper asserting `Make(e).ToEntity()` preserves every field, invoked
  per domain; or
- a CI check that flags any `db.Save`/`tx.Save` in a package whose round-trip is lossy.

These are complementary; the design may adopt one or both.

### 4.4 Data repair

**FR-DATA-1.** Rows already zeroed on the bee cluster must be repaired where a defensible
proxy for the true creation time exists, and left untouched where none does.

**FR-DATA-2.** The repair must only touch rows where `created_at` is the zero value
(`0001-01-01` / `created_at < '1970-01-01'`). It must never overwrite a surviving good
timestamp.

**FR-DATA-3.** The repair must be idempotent — re-running it must be a no-op.

**FR-DATA-4.** The repair must report how many rows were repaired and how many were left
unrecoverable, per table.

**FR-DATA-5.** The repair must run only *after* the code fix is deployed. Repairing first
would let the next `PATCH` re-zero the row.

The available proxies are genuinely good, because the insert-only tables that would supply
them are exactly the tables this bug never touched. See `migration-plan.md` for the full
derivation, the SQL, and the honest accounting of what each proxy is and is not.

## 5. API Surface

**No changes.** No endpoint gains, loses, or alters a field. `created_at` is not currently
exposed as a JSON:API attribute on `user`, `fleet`, `vehicle`, or `mediaobject`, and this
task does not add it.

The only externally observable change is a **corrected value** in an existing computed
field: `GET /vehicles` and `GET /vehicles/:id` return a derived `status` attribute, and
edited vehicles with no activity history will begin reporting their true status ("Healthy")
instead of "Inactive". This is the intended fix, not a regression, but it is a visible
behavioural change and should be called out in the PR description.

## 6. Data Model

No schema changes — no columns added, dropped, renamed, or retyped. `AutoMigrate` output
must be byte-identical before and after the code fix.

The changes are to GORM **write permissions** on existing columns, expressed as struct tags:

| Entity | Field | Change |
|---|---|---|
| `fleet-service/internal/vehicle.Entity` | `CreatedAt` | add `gorm:"<-:create"` |
| `fleet-service/internal/fleet.Entity` | `CreatedAt` | add `gorm:"<-:create"` |
| `fleet-service/internal/fleet.Entity` | `DeletedAt` | see §9 Q2 — tag vs. carry through `ToEntity()` |
| `media-service/internal/mediaobject.Entity` | `CreatedAt` | add `gorm:"<-:create"` (hardening; no behaviour change) |

`<-:create` is a write-permission tag only; it does not participate in DDL generation, so
`AutoMigrate` is unaffected. This is the same mechanism already proven on `main` for
`user.Entity` — the fix is deliberately the boring, precedented one.

Data repair (backfilling already-zeroed rows) is specified separately in
`migration-plan.md`.

## 7. Service Impact

**`fleet-service`** — the substantive work.
`internal/vehicle`: tag `Entity.CreatedAt`; add a regression test through the real
`Administrator.Update` path; add a `DeriveStatus` test proving an edited, activity-free
vehicle still derives "Healthy".
`internal/fleet`: tag `Entity.CreatedAt`; resolve `DeletedAt` per §9 Q2; add a rename
regression test.

**`media-service`** — hardening only.
`internal/mediaobject`: tag `Entity.CreatedAt`; add regression tests covering both `Update`
and `UpdateInTx`. No behaviour change expected, and the tests must confirm that.

**`auth-service`** — verification only.
`internal/user`: add a regression test pinning the existing `<-:create` behaviour so the
already-shipped fix cannot silently regress. No production-code change.

**`notification-service`** — audit only. Its single write path is an
`OnConflict.DoNothing` `Create`, and its `ToEntity()` already carries `CreatedAt`. No
change; recorded here so the sweep is provably exhaustive.

**`apps/web`** — no change. No frontend type or component reads `createdAt` for `user`,
`fleet`, `vehicle`, or `mediaobject`.

## 8. Non-Functional Requirements

**Correctness.** The primary requirement. Each fix must be demonstrated by a test that
fails against the unfixed code and passes after — not merely by inspection of struct tags.

**Performance.** Neutral. `<-:create` narrows the UPDATE column list slightly; no query
plans, indexes, or round-trip counts change.

**Security / privacy.** No new data is exposed. Repairing `created_at` does not surface a
timestamp through any API. The repair SQL touches only timestamp columns and must not be
able to alter identity, ownership, or soft-delete state.

**Observability.** The repair migration must log per-table repaired and unrecoverable
counts (FR-DATA-4) so the operator has a record of what could not be recovered.

**Backward compatibility.** No API or schema change; deploys and rolls back cleanly. The
code fix is independently deployable and correct without the data repair — the repair only
improves already-broken rows.

**Testing.** All four `Save` sites covered by behavioural tests (FR-GUARD-3). Existing
suite must stay green: baseline on this branch is `make build` clean, `make test` 37
packages ok / 0 failures.

## 9. Open Questions

These were deferred at spec time so the interview could proceed; each has a working
assumption recorded so implementation is unblocked if none are revisited. **Q1 is the one
most worth challenging before design.**

**Q1 — Data repair strategy for already-corrupted rows.** *Assumed:* backfill from the
best available proxy per table, leave genuinely unrecoverable rows at zero, document the
imprecision in a runbook. The alternatives are to leave all rows as-is (simplest, keeps the
corruption visible and honest) or to stamp a sentinel. This assumption is the one that most
changes the size of the task — it turns a pure code fix into a code fix plus a migration
plus a runbook. See `migration-plan.md` for what each proxy actually means before deciding.

**Q2 — `fleet.Entity.DeletedAt` treatment.** *Assumed:* carry `DeletedAt` through
`ToEntity()` rather than tagging it, since unlike `CreatedAt` it is legitimately mutable —
soft-delete and restore must be able to write it. A blanket `<-:create` would break those
paths. Needs confirmation against how fleet soft-delete is actually invoked.

**Q3 — Pre-emptive tagging of the seven latent domains (§4.2).** *Assumed:* no. Add the
regression guard, leave the code alone, on the grounds that changing seven packages with no
live defect is churn that dilutes the diff. The counter-argument — that consistency is
itself the guard, and that a tag is cheaper than trusting a CI check to be maintained — is
reasonable and may win at design time.

**Q4 — Regression guard mechanism.** *Assumed:* per-domain behavioural tests (FR-GUARD-3)
as the floor, plus a shared round-trip helper. Whether to additionally add a CI grep banning
lossy `Save` is a design decision; a grep is blunt but catches the case a helper misses,
namely a *new* package that never adopts the helper.

**Q5 — Is `<-:create` right for `vehicle`/`fleet`, given they are soft-deletable and have a
restore path?** `vehicle` has `RestoreRow` (`administrator.go`) which uses a narrowed
`Updates`, so it should be unaffected — but this must be verified by test, not assumed, as
it is precisely the kind of interaction a struct-tag change can silently break.

**Q6 — Should the already-zeroed auth-service production rows be repaired in this task?**
The issue documents at least one such row on the bee cluster
(`7a186017-…`, `created_at = 0001-01-01`). *Assumed:* yes — including auth-service in the
repair is what makes the repair worth doing, even though its code fix already shipped.

## 10. Acceptance Criteria

**Code fixes**

- [ ] `vehicle.Entity.CreatedAt` excluded from UPDATE; a test persists a vehicle, calls
      `Administrator.Update`, and asserts `created_at` is unchanged and non-zero.
- [ ] A `DeriveStatus` test proves an updated vehicle with zero activity rows derives
      "Healthy", not "Inactive" (the FR-FIX-1 symptom).
- [ ] `fleet.Entity.CreatedAt` excluded from UPDATE; a rename test asserts `created_at`
      survives.
- [ ] `fleet.Entity.DeletedAt` is no longer clobbered by `Update`, per the Q2 resolution,
      with a test covering it.
- [ ] `mediaobject` hardened; tests cover both `Update` and `UpdateInTx` and confirm no
      behaviour change.
- [ ] `auth-service/internal/user` has a regression test pinning the existing `<-:create`
      behaviour.

**Regression guard**

- [ ] A guard exists that fails when a domain with a lossy entity round-trip acquires a
      `Save`-based write path.
- [ ] The failure message names the domain and the dropped column (FR-GUARD-2).
- [ ] The guard covers all seven latent domains listed in §4.2.
- [ ] Verified by deliberately introducing a lossy `Save` in a scratch commit and observing
      the guard fail — then reverting.

**Data repair** (subject to Q1)

- [ ] Repair procedure exists, is idempotent, and touches only rows with a zero
      `created_at`.
- [ ] It reports repaired and unrecoverable counts per table.
- [ ] The runbook states plainly what each proxy timestamp does and does not mean, so nobody
      later mistakes a backfilled value for the original.
- [ ] Ordering constraint documented: deploy code fix first, repair second (FR-DATA-5).

**Whole-repo verification**

- [ ] `make build` clean.
- [ ] `make test` green — at least the 37 packages passing at baseline, plus the new tests.
- [ ] `make vet` and `make lint-check` clean.
- [ ] `git grep -n "db.Save\|tx.Save" -- 'apps/**/*.go'` returns only sites proven safe by
      test.
- [ ] Issue #7 can be closed: both the reported defect and its stated "wider concern" are
      resolved.
