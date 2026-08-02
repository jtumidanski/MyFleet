# `created_at` Wipe Sweep — Design

Version: v1
Status: Approved for planning
Created: 2026-08-02
PRD: [`prd.md`](./prd.md) · Repair plan: [`migration-plan.md`](./migration-plan.md)
Issue: [#7](https://github.com/jtumidanski/myfleet/issues/7)

---

## 1. Summary

Three decisions carry this design:

1. **Fix the defect at both layers, not one.** `gorm:"<-:create"` protects the *column*;
   a complete `ToEntity()` protects the *returned `Model`*. Each fixes something the other
   cannot. Apply both to `vehicle` and `fleet`.
2. **Make the guard a static analyzer, not a per-domain test.** The defect's defining
   property is that it appears in packages nobody thought to test. A guard that requires
   per-package opt-in cannot catch the case it exists for. A `go/ast` analyzer over
   `apps/*/internal/**`, run from one test per service, covers every package that exists
   today and every package added tomorrow, at zero marginal cost per domain.
3. **Ship the data repair as a runbook, not a migration.** Verified below: GORM
   **silently ignores** writes to a `<-:create` column, so the repair cannot use the ORM
   at all. It must be raw SQL — which removes the main argument for putting it in the
   service startup path.

Everything below is grounded in behaviour measured against GORM v1.31.1 in this
repository, not inferred from documentation. §2 records those measurements.

## 2. Verified GORM behaviour

The design rests on how GORM actually treats `<-:create`. Each claim below was measured
with a throwaway probe in `apps/fleet-service/internal/vehicle` (sqlite, GORM v1.31.1 —
the versions and driver the tests will use), then deleted. These findings are the reason
several PRD assumptions change.

| # | Question | Measured result |
|---|---|---|
| V1 | Does `<-:create` block GORM's auto-populate of `CreatedAt` on `Create`? | **No.** Insert still stamps the column. The tag is safe on an insert path. |
| V2 | Does `<-:create` protect `created_at` from a full-column `Save` while still applying the real change? | **Yes.** `created_at` held; `name` updated; `updated_at` re-stamped. |
| V3 | Does sqlite reproduce the original bug? | **Yes** — untagged `CreatedAt` + `Save` with a zero struct value wrote `0001-01-01`. The sqlite harness is a faithful reproduction, so the tests are not vacuous. |
| V4 | Does a full-column `Save` resurrect a soft-deleted row when the struct's `DeletedAt` is zero? | **Yes.** Confirms FR-FIX-3 is a real defect shape, not a theoretical one. |
| V5 | Does `<-:create` on a `gorm.DeletedAt` break soft **delete**? | **No.** GORM's delete callback builds its own `SET` clause and bypasses field write permission. |
| V6 | Does `<-:create` on a `gorm.DeletedAt` break **restore** via `Updates(map{...})`? | **Yes — silently.** The call returned `err=nil, RowsAffected=1` and the row *stayed deleted*. |
| V7 | Does a narrowed `Updates(map)` still work on a struct that has a `<-:create` field? | **Yes**, for other columns. An explicit map write *to* the tagged column is **silently discarded** — no error, no warning. |

**V6 and V7 are the load-bearing findings.** V6 decides Q2 on its own: a tag that makes a
restore path report success while doing nothing is a strictly worse failure mode than the
bug we are fixing. V7 decides Q1's mechanism: any ORM-based repair of `created_at` would
report success and change nothing.

## 3. The defect, restated precisely

`Administrator.Update` is uniformly:

```go
e := m.ToEntity()
if err := a.db.Save(&e).Error; err != nil { ... }
return Make(e), nil
```

`db.Save` on a struct with a set primary key issues `UPDATE … SET <every column>`. So any
column that `ToEntity()` leaves at its zero value is written as zero. GORM special-cases
`UpdatedAt` (its `autoUpdateTime` callback stamps the field before the write, and assigns
back into the struct). It applies no such protection to `CreatedAt`, `DeletedAt`, or
anything else.

The defect therefore requires exactly two ingredients, and the guard in §6 tests for
precisely that conjunction:

- a `Save`-based write path in the package, **and**
- an `Entity` field that `ToEntity()` does not assign and that is not otherwise protected.

## 4. Fix strategy: two layers, deliberately

For `vehicle` and `fleet` the `Model` already carries `createdAt` — `Make()` reads it, only
`ToEntity()` drops it. So there are two candidate fixes, and they are not equivalent.

**`gorm:"<-:create"` alone** protects the database column (V2) but leaves the in-memory
round-trip lossy: `Save` writes the correct row, then `Make(e)` returns a `Model` whose
`createdAt` is still zero, because `e.CreatedAt` was never populated. Today that is
survivable — `PATCH /vehicles/:id` responds via `Transform(updated)`
(`rest.go:26` → `TransformWithStatus(m, "")`), which omits `status`, so nothing reads the
zero value. It is survivable by luck. The moment anyone attaches a derived status to the
PATCH response — a one-line change that looks obviously correct — the response says
"Inactive" for a healthy vehicle while the database is perfectly fine. That is a nastier
bug than the original, because the data is right and only the response is wrong.

**A complete `ToEntity()` alone** fixes the returned `Model` and the persisted column, but
leaves no standing protection: the next person to add a field, or to reorder that composite
literal, silently reopens the hole.

**Decision: apply both** to `vehicle` and `fleet`. They are complementary, each is one
line, and the guard in §6 accepts either as sufficient — so the code and the analyzer agree
on what "protected" means. `mediaobject` already has a complete `ToEntity()`, so it needs
only the tag (§5.3).

This is also why the design does not simply narrow `Administrator.Update` to
`db.Model(...).Select(cols).Updates(...)`. That would fix all four sites at the root and is
the better long-term shape — but PRD §2 explicitly excludes it, and it changes the write
semantics of every mutation path in the repository. It stays a follow-up (§10).

## 5. Per-domain changes

### 5.1 `fleet-service/internal/vehicle` — the live, user-facing fix

`Entity.CreatedAt` gains `gorm:"<-:create"`, and `ToEntity()` gains
`CreatedAt: m.createdAt`.

`UpdatedAt` stays absent from `ToEntity()`: GORM owns it, stamps it on every write, and
assigns it back into the struct, so `Make(e)` returns the correct value. The analyzer
allowlists `UpdatedAt` for this reason and only this reason (§6.3).

**Interaction with the restore path (PRD Q5) — resolved, and it is safe.** `RestoreRow` and
`SoftDelete` use `db.Model(&e).Updates(map[string]any{...})` with maps that name only
`deleted_at` and `purge_after`. V7 confirms a narrowed map update is unaffected by a
`<-:create` field elsewhere on the struct. `SetPrimaryImage` and `UpdateCurrentMileage` use
single-column `Update` and are likewise unaffected. None of these maps mention `created_at`,
so V7's silent-discard hazard is not reachable here. This is nevertheless covered by test,
because "unaffected" is exactly the kind of claim a tag change falsifies quietly.

**The symptom test.** `DeriveStatus` (`status.go:40-60`) falls back to `m.CreatedAt()` when a
vehicle has no activity rows. A test must drive the real path — persist a vehicle, call
`Administrator.Update`, re-read it, and derive status with a `LastActivityGatherer` that
returns the zero time — and assert `"Healthy"`. Asserting `created_at != zero` alone would
not prove the user-visible bug is gone; this is the acceptance criterion that does.

### 5.2 `fleet-service/internal/fleet` — `CreatedAt` and the `DeletedAt` question

`Entity.CreatedAt` gains `gorm:"<-:create"`, and `ToEntity()` gains
`CreatedAt: m.createdAt`. `Model` already has the field and the accessor; only the
composite literal at `entity.go:35` is incomplete.

**Q2 — `DeletedAt`: carry it, do not tag it.** V5 showed tagging does not break soft
*delete*. V6 showed it breaks *restore* — and breaks it silently, returning `err=nil` and
`RowsAffected=1` on a row that stays deleted. `vehicle` already has a restore path, so a
future `fleet` restore is a plausible change, and it would be written by someone reading
`vehicle` as the template. Shipping a tag that turns that change into a silent no-op trades
an unreachable bug for a reachable one. Tagging is rejected.

So `DeletedAt` is carried through the round-trip instead. `fleet.Model` gains an unexported
`deletedAt *time.Time` with a `DeletedAt() *time.Time` accessor, converted at the entity
boundary:

- `Make`: `gorm.DeletedAt{Valid: true}` → `*time.Time`; invalid → `nil`
- `ToEntity`: `nil` → zero `gorm.DeletedAt`; non-nil → `{Time: *t, Valid: true}`

The `*time.Time` domain type is deliberate: it matches `vehicle.Model.deletedAt` exactly and
keeps the GORM type out of the domain layer, consistent with the `Model`/`Entity` split. The
conversion is confined to `entity.go`, which is already the only file that knows about GORM.

This is a slightly larger change than the PRD's assumed wording ("carry `DeletedAt` through
`ToEntity()`"), because `fleet.Model` has no such field today. It is the same decision, made
type-correct.

**Reachability, honestly.** Nothing in `apps/fleet-service/internal/fleet` calls `Delete` or
`Unscoped` today — `git grep` finds only the struct field. So this closes a hole that cannot
currently be reached. It is worth closing anyway because V4 proves the shape is real, and
because the analyzer (§6) will flag the package otherwise, which is the correct outcome: the
guard should not need an exemption on day one.

### 5.3 `media-service/internal/mediaobject` — hardening only

`ToEntity()` is already complete (it assigns `CreatedAt`, `DeletedAt`, `PurgeAfter`; the
entity has no `UpdatedAt` column at all). Add `gorm:"<-:create"` to `Entity.CreatedAt` as
standing protection.

Confirmed no behaviour change: `NewBuilder()` sets `createdAt: time.Now().UTC()`
(`builder.go:20`), so the value is non-zero at insert, and V1 confirms `<-:create` does not
interfere with `Create`. Both `Save` sites — `Update` and `UpdateInTx` — get a regression
test, and those tests assert the value is *preserved*, which is simultaneously the
no-behaviour-change assertion.

### 5.4 `auth-service/internal/user` — test only

No production change. `Entity.CreatedAt` already carries `<-:create` with a comment
explaining why. Add a behavioural test through `Administrator.Update` pinning it, so the
shipped fix cannot be reverted by someone tidying struct tags.

Note the existing `user/entity_test.go` tests round-trip *fields* (`ThemePreference`) rather
than database behaviour. The new test must go through `db.Save`; a `ToEntity()` assertion
would pass while the column got wiped, which is exactly the failure mode FR-GUARD-3 rules
out.

### 5.5 `notification-service` — no change

Audited: its single write path is an `OnConflict.DoNothing` `Create`, and `ToEntity()`
carries `CreatedAt`. The analyzer will confirm this continuously; nothing to do now.

## 6. The regression guard

### 6.1 Why not the round-trip helper

PRD §4.3 offers a reflective `Make(e).ToEntity()` round-trip helper as one candidate. It is
rejected as the primary mechanism, for one reason: **it requires each domain to opt in.**
The seven latent domains in PRD §4.2 are latent precisely because nobody was thinking about
this defect when they were written. A future eighth domain will be written by someone in the
same position — and they will not add a helper they have never heard of. The guard would be
absent exactly where it is needed.

It is also weaker than it looks: it would need an ignore-list for legitimately-dropped
fields (`UpdatedAt`, and any field a domain deliberately does not model), and that
ignore-list is a silent weakening point. A future developer facing a red test can add one
line to the ignore-list and move on.

### 6.2 The mechanism: a static analyzer

A `go/ast` analyzer that walks a service's `internal/` tree and, for each package,
reports the conjunction from §3:

```
package has a `.Save(` call site
    AND declares `func (m Model) ToEntity() Entity`
    AND some field of `Entity` is:
        - not assigned in ToEntity's returned composite literal, and
        - not tagged `<-:create` (or otherwise write-restricted), and
        - not in the auto-managed allowlist
→ FINDING
```

This needs no type checking — parsing each package's source and matching the `Entity` struct
declaration against the keys of the composite literal returned by `ToEntity` is sufficient,
because both live in the same package by construction.

**Home:** `packages/shared-go/database/entityguard`, exporting
`Analyze(root string) ([]Finding, error)`. All four services already depend on
`shared-go`, and `database/` already exists there.

**Invocation:** one test file per service, three lines of body:

```go
func TestNoLossySaveRoundTrips(t *testing.T) {
    findings, err := entityguard.Analyze("..")   // the service's internal/ root
    ...
}
```

This runs under the existing `make test` with no new CI wiring, and — critically — it needs
**no per-domain registration**. A new package under `internal/` is covered the moment it
exists.

**Failure message** (FR-GUARD-2) must name the domain, the field, the column, both file
locations, and both remedies:

```
fleet-service/internal/widget: Entity.CreatedAt (column "created_at") is not assigned by
ToEntity() (entity.go:41), but administrator.go:27 writes the row with db.Save, which
UPDATEs every column — so this column will be overwritten with its zero value on every
update.

Fix by either:
  - assigning it in ToEntity(), if Model carries the value; or
  - tagging the field `gorm:"<-:create"`, if the column must never change after insert.
Do NOT tag a gorm.DeletedAt this way: it silently breaks restore paths (task-006 design §2, V6).
```

The last line is there because V6 is a trap a developer will otherwise fall into while
fixing this very error.

**Unparseable input must fail, not pass.** If `ToEntity` does not return a composite literal
directly — it builds a `var e Entity` and mutates it, say — the analyzer cannot decide, and
must report that as a finding ("cannot verify") rather than skipping the package. A guard
that silently passes when it cannot run is worse than no guard; `tools/check-carfax-template.sh`
already takes this position explicitly for its missing-PyYAML case, so it is the house rule.

### 6.3 Allowlist

Exactly one entry: **`UpdatedAt`**, justified by GORM's `autoUpdateTime` callback stamping
it on every write (verified in V2 — `updated_at` advanced on a `Save` that never set it).
Any further exemption is a code change to `shared-go` with a comment explaining why, not a
per-domain flag. Keeping the allowlist in the analyzer rather than in per-domain call sites
is what stops it from eroding.

### 6.4 Coverage of a *new service*

The per-service test covers every package in that service, forever. It does not cover a
fifth service created later with no such test. Two options were considered: a repo-wide
shell check in `make ci` (precedent: `tools/check-carfax-template.sh`,
`tools/check-manifests.sh`), or accepting the gap.

**Decision: accept the gap, and record it.** A new Go service is a large, deliberate,
reviewed event — unlike a new domain package, which is routine. The scaffolding for a new
service will be copied from an existing one, which carries the test. Adding a fourth
repo-invariant shell script to guard a once-a-year event is not proportionate. This is noted
in §10 as the one thing the guard does not catch, so the omission is a decision rather than
an oversight.

### 6.5 Proving the guard works

FR-GUARD's acceptance criterion is that the guard *fires*, not that it exists. Verify by
scratch commit: remove `CreatedAt: m.createdAt` from `vehicle.ToEntity()` and its tag,
observe both the analyzer test and the `vehicle` behavioural test fail with actionable
messages, then revert. Record the observed output in the PR description — a guard nobody has
seen fail is an untested guard.

### 6.6 Layer two: behavioural tests (FR-GUARD-3)

The analyzer reasons about source; it cannot prove GORM does what the tag claims. So each of
the four `Save` sites also gets a test that persists a row, updates it through the real
`Administrator`, re-reads it, and asserts `created_at` is unchanged and non-zero. V3 confirms
sqlite reproduces the bug faithfully, so these tests genuinely fail before the fix.

Harness: the established in-memory sqlite pattern — `ATTACH DATABASE ':memory:' AS <schema>`
plus **explicit DDL**, not `AutoMigrate`. This is required, not stylistic: GORM emits
`CREATE INDEX` with the schema prefix stripped under sqlite, which fails against an attached
schema, and `vehicle`, `fleet`, and `mediaobject` all carry `index` tags.
`maintenanceschedule/completion_db_test.go:15-45` and `user/provider_test.go:18-51` are the
templates, and both already carry the KEEP-IN-SYNC comment this DDL duplication demands.

The five tests:

| Test | Asserts |
|---|---|
| `vehicle`: insert → `Update` → re-read | `created_at` unchanged, non-zero |
| `vehicle`: insert → `Update` → `DeriveStatus` with no activity | `"Healthy"`, not `"Inactive"` (the FR-FIX-1 symptom) |
| `vehicle`: `SoftDelete` → `RestoreRow` after the tag | restore still works, `created_at` intact (Q5) |
| `fleet`: insert → `Rename` → re-read | `created_at` unchanged; `deleted_at` still NULL |
| `fleet`: soft-delete → `Unscoped` load → `Update` → re-read | still soft-deleted (FR-FIX-3, the V4 shape) |
| `mediaobject`: `Update` **and** `UpdateInTx` | `created_at` unchanged, non-zero, both paths |
| `user`: insert → `Update` → re-read | `created_at` unchanged (pins the shipped fix) |

## 7. Q3 — the seven latent domains

**Decision: do not pre-emptively tag them.** Agrees with the PRD's assumption, but for a
sharper reason than "churn": with the analyzer in place, tagging them adds no safety
whatsoever. The analyzer fires on `Save` + lossy, and none of these packages has a `Save`.
The day one acquires one, the analyzer names the package and the column and tells the
developer both remedies. Tagging seven packages today would be seven diffs that change no
behaviour and that a reviewer cannot distinguish from noise.

There is one real counter-argument — a developer facing the analyzer failure might reach for
`<-:create` reflexively on a column that *should* be mutable. §6.2's failure message
addresses this directly by naming both remedies and calling out the `gorm.DeletedAt` trap.

## 8. Q1 / Q6 — data repair

**Decision: repair, via a runbook under `docs/runbooks/`, covering all three tables
including `auth.users`.**

**Why not a Go migration in the startup path.** V7 is decisive: GORM *silently discards* a
write to a `<-:create` column — `err=nil`, no rows changed, no warning. An ORM-based repair
would report complete success and change nothing, and there is no natural place a test would
catch that. The repair must therefore be raw SQL. Once it is raw SQL, judgement-laden, and
one-shot, the argument for embedding it in every service boot is gone.

Confirmed the SQL in `migration-plan.md` is executable as written: all four services share
one database (`myfleet`) and differ only by `search_path` — see
`deploy/k8s/secrets.example.yaml:18,34,44,57`. So the cross-schema single-transaction repair
is valid, and it must be run by a role that can see all three schemas, not by a service's own
connection.

**Q6 — include `auth.users`: yes.** Its code fix already shipped, but the corrupted row is
the one instance independently documented in issue #7, and per FR-DATA-5 the ordering
constraint is already satisfied for it (the fix is on `main`). Excluding the one row we can
prove is broken would make the repair pointless.

**Ordering (FR-DATA-5).** Deploy the code fix first, repair second. For `fleet.vehicles` and
`fleet.fleets` this is a hard constraint — repairing first means the next `PATCH` re-zeroes
the row, producing a repaired-then-recorrupted row that reads as a failed migration.

**Traceability.** PRD §6 forbids schema changes, so there is nowhere in the data to mark a
value as approximated. The record therefore lives in the runbook's output log: the operator
captures the before/after counts and the affected id list. The runbook must state plainly
that a backfilled `created_at` is an *upper bound with a known bias*, not a measurement —
`migration-plan.md`'s accuracy notes are the source text and should be reproduced in the
runbook rather than linked, since the operator running it a year from now will not read the
task folder.

**Structure.** Three passes, in order: a `SELECT`-only counting pass to eyeball; the
`UPDATE` transaction; the same counting query again for the FR-DATA-4 report. Idempotence
and the FR-DATA-2 guard both come from the `created_at < '1970-01-01'` predicate, which
cannot match a legitimate row and matches nothing after a successful run.

**The one row to check by hand.** Per `migration-plan.md`, verify user
`7a186017-d27e-4d65-90e3-6b240bf9880a` against its refresh-token history before the bulk run.
It is the only row whose corruption is independently documented, so it is the only available
sanity check on the weakest of the three proxies.

**Scope boundary:** the runbook is a document, not code. No Go changes, no CI involvement,
no automated test. Its correctness is established by the dry-run pass, which is why the
dry-run is mandatory rather than advisory.

## 9. Verification

Beyond `make ci`:

- `git grep -n "db.Save\|tx.Save" -- 'apps/**/*.go'` returns exactly the five known lines,
  each covered by a test in §6.6.
- The analyzer test passes on the fixed tree and fails on the §6.5 scratch commit.
- `AutoMigrate` output is unchanged — `<-:create` is a write-permission tag and takes no part
  in DDL generation. Verified structurally (no schema-affecting tag added), which is
  sufficient given no column definition changes.
- Baseline to hold: 37 packages ok / 0 failures.

## 10. Follow-ups, explicitly not in this task

- **Narrow `Administrator.Update` to `Select(cols).Updates(...)`** across all four services.
  This removes the defect class at its root rather than patching each instance; excluded by
  PRD §2 because it changes write semantics repo-wide and deserves its own task.
- **A new *service* is not covered by the guard** (§6.4). Accepted deliberately.
- **sqlite-vs-Postgres harness divergence** (issue #7's reproduction notes) is untouched.
  V3 confirms sqlite reproduces *this* defect faithfully, which is all this task needs; it is
  not a general claim about the harness.
- **`created_at` is still not exposed** on any JSON:API resource, per PRD §5. The only
  externally visible change is corrected `status` values on edited, activity-free vehicles —
  which must be called out in the PR description as an intended behavioural change.

## 11. Decisions resolved

| Q | PRD assumption | Resolution | Basis |
|---|---|---|---|
| Q1 | Backfill from best proxy, runbook | **Confirmed** — runbook, raw SQL, all three tables | V7: ORM writes to a `<-:create` column are silently discarded |
| Q2 | Carry `DeletedAt` through `ToEntity()` | **Confirmed**, typed as `*time.Time` in `Model` | V6: tagging silently breaks restore (`err=nil`, row stays deleted) |
| Q3 | Do not pre-emptively tag the seven | **Confirmed** | Analyzer makes tagging redundant; failure message covers the misfix risk |
| Q4 | Behavioural tests + round-trip helper | **Changed** — behavioural tests + static analyzer; helper dropped | A helper needs opt-in, so it is absent exactly where the defect appears |
| Q5 | Verify `RestoreRow` survives the tag | **Safe**, and covered by test | V7: narrowed `Updates(map)` unaffected by a `<-:create` field elsewhere |
| Q6 | Include `auth.users` in the repair | **Confirmed** | It is the one documented corrupted row; excluding it makes the repair pointless |
