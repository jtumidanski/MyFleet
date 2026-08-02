# Runbook — repairing zeroed `created_at` rows

A defect in the persistence layer wrote `0001-01-01` over `created_at` on every full-column
`UPDATE` (issue #7). The code fix ships in task-006; this runbook repairs the rows that were
already corrupted before it shipped.

**Run this once, by hand, against the bee cluster's Postgres. It is not part of any deploy.**

## Before you start

**Ordering is a hard constraint: the code fix must already be deployed.** Repairing first means
the next `PATCH /vehicles/:id`, fleet rename, or re-login re-zeroes the row — producing a
repaired-then-recorrupted row that reads as a failed migration. Confirm the running images
include the task-006 fix before continuing.

**Connect as a role that can see all three schemas.** All four services share one database
(`myfleet`) and differ only by `search_path` (`deploy/k8s/secrets.example.yaml:18,34,44,57`), so
the cross-schema transaction below is valid — but it cannot be run through a service's own
connection.

## What a repaired value means, and what it does not

Each backfilled `created_at` is an **upper bound with a known bias**, not a measurement. Nothing
in the schema distinguishes a repaired value from an original one afterwards (adding such a
column would be a schema change, which task-006 forbids), so this document and the output you
capture below are the only record. Do not later treat a repaired timestamp as ground truth.

The proxies work at all for a structural reason rather than a lucky one: the tables that supply
them are insert-only (`fleet.activity_events`, `fleet.fleet_memberships`, `auth.refresh_tokens`),
and insert-only tables were never touched by this bug — GORM populates `CreatedAt` correctly on
`Create`. The corruption was confined to exactly the tables with a `Save`-based update.

| Table | Proxy | What it means |
|---|---|---|
| `fleet.vehicles` | earliest `vehicle.created` row in `fleet.activity_events` | **Strongest.** The event is written in the same transaction as the vehicle insert, so where the event exists the value is exact to within that transaction. Vehicles created before activity recording existed have no such event and are unrecoverable. |
| `fleet.fleets` | earliest `fleet.fleet_memberships` row for the fleet | The owner membership is created with the fleet in one transaction, so this is exact where that membership survives. If the original owner was removed and memberships hard-deleted, the value skews **late**. |
| `auth.users` | earliest `auth.refresh_tokens` row for the user | **Weakest.** A refresh token is minted at first login, which for a Google-provisioned user is the same request that created the row — accurate to sub-second *while the earliest token survives*. Tokens expire and get pruned, so for long-tenured users this can post-date signup by an arbitrary margin, potentially months. |

`media.media_objects` is **not** in scope: its round-trip always carried `CreatedAt`, so no media
row was ever corrupted.

## Pass 1 — count (read-only, mandatory)

Run this first and keep the output. It is the "before" half of the FR-DATA-4 report.

```sql
SELECT 'auth.users'     AS tbl, COUNT(*) AS still_zero FROM auth.users     WHERE created_at < '1970-01-01'
UNION ALL
SELECT 'fleet.vehicles',        COUNT(*)               FROM fleet.vehicles  WHERE created_at < '1970-01-01'
UNION ALL
SELECT 'fleet.fleets',          COUNT(*)               FROM fleet.fleets    WHERE created_at < '1970-01-01';
```

Also capture the affected ids, since nothing in the data will mark them afterwards:

```sql
SELECT 'auth.users' AS tbl, id FROM auth.users     WHERE created_at < '1970-01-01'
UNION ALL
SELECT 'fleet.vehicles',    id FROM fleet.vehicles WHERE created_at < '1970-01-01'
UNION ALL
SELECT 'fleet.fleets',      id FROM fleet.fleets   WHERE created_at < '1970-01-01';
```

If every count is zero, stop — there is nothing to repair.

## Pass 2 — the one row to check by hand

User `7a186017-d27e-4d65-90e3-6b240bf9880a` is the only row whose corruption is independently
documented (issue #7), which makes it the only available sanity check on the weakest of the three
proxies. Inspect its token history before the bulk run and satisfy yourself the proxy is
plausible:

```sql
SELECT MIN(created_at) AS earliest_token, MAX(created_at) AS latest_token, COUNT(*) AS tokens
  FROM auth.refresh_tokens
 WHERE user_id = '7a186017-d27e-4d65-90e3-6b240bf9880a';
```

If `earliest_token` is implausibly recent (e.g. this week, for an account you know is older), the
tokens have been pruned and the proxy will overstate the creation date. Decide deliberately
whether to accept that or to leave the row zeroed by removing the `auth.users` statement from
Pass 3.

## Pass 3 — repair

Every statement is idempotent (re-running is a no-op) and guarded to touch only zeroed rows. The
`< '1970-01-01'` predicate is deliberately looser than `= '0001-01-01'` so it also catches any
other pre-epoch garbage, while remaining incapable of matching a legitimate row. Nothing here can
alter identity, ownership, or soft-delete state — only timestamp columns are written.

```sql
BEGIN;

-- fleet.vehicles ← vehicle.created activity event (strongest proxy)
UPDATE fleet.vehicles v
   SET created_at = a.created_at
  FROM (
        SELECT vehicle_id, MIN(created_at) AS created_at
          FROM fleet.activity_events
         WHERE type = 'vehicle.created' AND vehicle_id IS NOT NULL
      GROUP BY vehicle_id
       ) a
 WHERE v.id = a.vehicle_id
   AND v.created_at < '1970-01-01';

-- fleet.fleets ← earliest membership
UPDATE fleet.fleets f
   SET created_at = m.created_at
  FROM (
        SELECT fleet_id, MIN(created_at) AS created_at
          FROM fleet.fleet_memberships
      GROUP BY fleet_id
       ) m
 WHERE f.id = m.fleet_id
   AND f.created_at < '1970-01-01';

-- auth.users ← earliest refresh token (weakest proxy; see the table above)
UPDATE auth.users u
   SET created_at = t.created_at
  FROM (
        SELECT user_id, MIN(created_at) AS created_at
          FROM auth.refresh_tokens
      GROUP BY user_id
       ) t
 WHERE u.id = t.user_id
   AND u.created_at < '1970-01-01';

COMMIT;
```

Note each statement's reported row count as you go — that is the "repaired" half of the report.

## Pass 4 — report

Re-run the Pass 1 counting query. Rows still counted are **genuinely unrecoverable**: their proxy
table has no surviving row. Report them as such; do not retry them with a weaker proxy.

Record, wherever this cluster's operational history lives:

- repaired count per table (Pass 3 row counts),
- unrecoverable count per table (Pass 4 counts),
- the id list captured in Pass 1.

## If something looks wrong

Pass 3 is a single transaction — if any statement errors, nothing is committed. `ROLLBACK;` and
re-check the proxy tables exist and are populated. Because the repair only ever moves a row from
`0001-01-01` to a plausible timestamp, and only when guarded by `< '1970-01-01'`, a partial or
repeated run cannot make the data worse than it already is.
