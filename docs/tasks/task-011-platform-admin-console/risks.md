# Platform Admin Console — Risk Register

Companion to `prd.md`. Ordered by expected damage, not likelihood.

---

## R1 — Retrofitting `deleted_at` onto ten tables silently changes every existing query

**Severity: critical. Likelihood: near-certain without deliberate control.**

Six of the sixteen purgeable tables have `deleted_at` today. Ten do not, and every provider,
aggregation query, and count over those ten currently assumes *every row it sees is live*. Adding
the column does not break compilation, does not fail a test that isn't looking for it, and does not
error at runtime. It just makes purged data keep showing up in the product.

The failure is silent and asymmetric: an admin purges a fleet, the console reports success, the
audit row is written, and the fleet's members continue to see their mileage history, schedules,
activity feed, and dashboards exactly as before. The bug is invisible to the person who caused it.

Worst affected are the high-traffic read paths: `mileage` (history + trend graph + latest-mileage
autofill), `maintenanceschedule` (the upcoming/overdue queues, which also drive notifications),
`activity` (the feed), and `dashboard` (aggregation SQL that already hand-writes `deleted_at`
filters for *some* joined tables and would now need more).

**Controls:** FR-ADMIN-DATA-1 through FR-ADMIN-DATA-3. The per-table regression test is the real
control — the audit checklist alone will not catch a missed query. Treat the enumeration of read
paths as a mechanical sweep of every `p.db.` call site in the affected packages, not a judgement
call.

**Residual risk:** raw SQL in `dashboard/aggregate.go` and
`maintenanceschedule/completion_db.go` is the likeliest place for a miss, because those bypass the
provider layer where a reviewer's attention naturally goes.

---

## R2 — Unique constraints turn soft-deletes into permanent lockouts

**Severity: high. Likelihood: high if not explicitly handled.**

`fleet.fleet_memberships` and `fleet.fleet_invites` carry unique constraints. A plain soft-delete
leaves the row physically present, so the constraint still fires. Purge a membership and that user
can never rejoin the fleet — the insert collides with an invisible row. Purge an invite and its
token is burned forever.

This converts a *recoverable* operation into an *unrecoverable* one, which defeats the entire
rationale for choosing soft-delete.

**Control:** FR-ADMIN-DATA-4 — partial unique indexes predicated on `deleted_at IS NULL`.

**Complication:** GORM `AutoMigrate` cannot express a partial index, and this codebase has no
precedent for raw DDL. Open question 3 in the PRD. Expect this to need a hand-written migration
step, which is a new pattern for the repo and should be reviewed as such.

---

## R3 — Cross-service purge has no distributed transaction

**Severity: high. Likelihood: moderate.**

A fleet purge spans four databases. `fleet-service` commits its own transaction, then calls
`media-service` and `notification-service` over HTTP. There is no two-phase commit and there will
not be one.

If a downstream call fails, fleet data is soft-deleted while media rows are not. The operation is
inconsistent until retried. If the *reap* phase partially fails, it is worse: some rows are hard-
deleted and unrecoverable while the operation still shows as incomplete.

**Controls:** FR-ADMIN-PURGE-9 (explicit `partial` status rather than pretending success),
FR-ADMIN-PURGE-10 (idempotent stamping keyed on `purge_operation_id`), FR-ADMIN-RESTORE-6
(resumable reaper).

**Design note:** idempotency-on-operation-id is what makes this tolerable. Every downstream mutation
must be keyed on the operation id and safe to replay — this is a hard requirement, not an
optimisation. A downstream endpoint that stamps by "all rows for fleet X" without recording *which*
operation stamped them cannot support correct restore, because it cannot distinguish rows it
deleted from rows deleted earlier by a user.

---

## R4 — Media and notification rows have no fleet id

**Severity: high. Likelihood: certain — this is a known gap, not a hypothetical.**

`media.*` and `notification.*` tables carry no `fleet_id`. A fleet-scoped purge cannot simply say
"delete where fleet = X" in those services. The PRD proposes passing explicit id sets (§6.5), but
this is unresolved (open questions 1 and 2).

The notification case has a genuine correctness trap: notifications appear to be **user-scoped**.
A user who belongs to two fleets has one notification stream. Purging fleet A must not delete their
fleet B notifications. A naive "delete all notifications for these user ids" implementation
destroys data belonging to a fleet that was never targeted — a cross-tenant data-loss bug
introduced by a tool whose whole purpose is careful deletion.

**Control:** resolve both ownership rules in the design phase, before any implementation. This is
the single most important thing to settle at `/design-task`.

---

## R5 — The admin console is unreachable exactly when it is most needed

**Severity: moderate. Likelihood: high without the exemption.**

`RequireAuth` (`apps/web/src/components/RequireAuth.tsx:29`) redirects any authenticated user
without an `activeFleetId` to `/onboarding`. After a successful system purge the admin has no
fleet. Without an explicit exemption they are bounced to onboarding and cannot return to `/admin`
to inspect the result, cancel the purge, or read the audit log — during the recovery window when
cancellation is still possible.

**Control:** FR-ADMIN-UI-3. Cheap to implement, easy to forget, and only discovered by testing the
post-purge state rather than the purge itself.

---

## R6 — Stale `platform_admin` claim after revocation

**Severity: moderate. Likelihood: low but consequential.**

The claim is stamped at mint time and access tokens live 15 minutes. Revoking an admin by deleting
their `auth.platform_admins` row does not invalidate tokens already issued, so a just-revoked admin
retains full destructive capability for up to 15 minutes.

**Control:** FR-ADMIN-AUTH-7 — purge endpoints re-verify against the database. Read-only endpoints
accept the staleness, which is the right trade: an extra HTTP hop on every stats page load is not
worth it, while an extra hop before irreversible deletion plainly is.

**Accepted:** a revoked admin can still *read* platform data for up to 15 minutes.

---

## R7 — Deliberately bypassing the same-fleet guard

**Severity: moderate. Likelihood: low.**

`authz.RequireSameFleet` returns 404 rather than 403 specifically so cross-fleet existence never
leaks. The admin API exists to violate that property. The risk is that the bypass leaks outward —
someone later "simplifies" by adding a `PlatformAdmin` short-circuit *inside* `RequireSameFleet`,
at which point every ordinary endpoint in the service silently becomes cross-fleet capable for
admins, and any bug in claim handling becomes a tenant-isolation breach.

**Control:** structural separation (FR-ADMIN-API-1) plus an arch test asserting that no handler
outside `/admin` reads `Identity.PlatformAdmin` and no `/admin` handler calls `RequireSameFleet`.
This repo already has an arch test precedent at `apps/auth-service/internal/arch/arch_test.go`.

---

## R8 — Reaper and existing vehicle sweep collide

**Severity: moderate. Likelihood: high without the narrowing.**

`vehicle.PurgeExpired` hard-deletes any vehicle whose `purge_after` has passed. An admin purge
stamps `deleted_at` on vehicles — if it also sets `purge_after`, or if the existing sweep is left
unnarrowed, the old job can hard-delete vehicles belonging to a *cancellable* admin operation.
The admin then cancels, everything else restores, and the vehicles are simply gone — a partial
restore that leaves orphaned children and a fleet missing its cars.

**Control:** FR-ADMIN-RESTORE-7 — narrow the existing sweep to `purge_operation_id IS NULL`.

---

## R9 — Confirmation phrases are the only barrier to total data loss

**Severity: critical impact, low likelihood.**

`PURGE EVERYTHING` typed into a box is the last line of defence before every fleet in the system is
soft-deleted. The recovery window is what makes this survivable; without it, a single request would
be unrecoverable.

**Controls:** FR-ADMIN-PURGE-7 (server-side confirmation matching — the disabled button is UI
convenience, the 409 is the real control), FR-ADMIN-PURGE-11 (5-day window), FR-ADMIN-UI-7 (dialog
states blast radius and permanence date explicitly).

**Note:** confirmation must be validated server-side. A UI-only check is not a control.

---

## R10 — Orphan rows already exist in production data

**Severity: low. Likelihood: certain if any vehicle has ever been purged.**

Documented in PRD §11. The existing vehicle purge already orphans children. `/admin/stats` will
count those orphans and report numbers that do not reconcile with what any fleet can see — the
console's first act would be to display wrong data.

**Control:** the one-time cleanup in PRD §11, sequenced *before* the stats endpoint is trusted.
