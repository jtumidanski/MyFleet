# Data Repair Plan — zeroed `created_at` rows

Companion to `prd.md` §4.4. Subject to **Q1** (§9) — if the decision is "leave the rows
alone", this document is discarded rather than implemented.

## What was lost, and why a proxy is possible at all

The bug wrote `0001-01-01` over `created_at` on every full-column UPDATE. The original
value is gone from the row and cannot be recovered from it.

It *can* often be approximated, and the reason is structural rather than lucky: the tables
that would supply a proxy are the insert-only ones (`fleet.activity_events`,
`auth.refresh_tokens`, `fleet.fleet_memberships`), and insert-only tables were never touched
by this bug — GORM populates `CreatedAt` correctly on `Create`. The corruption was confined
to exactly the tables with a `Save`-based `Update`, which leaves the neighbouring history
intact and trustworthy.

## Scope of the repair

Three tables, corresponding to the three `Save` sites whose round-trip dropped `CreatedAt`:

| Table | How it got corrupted | Proxy |
|---|---|---|
| `auth.users` | every re-login (`ProvisionFromGoogle` → `Update`) | earliest `auth.refresh_tokens.created_at` for the user |
| `fleet.vehicles` | every `PATCH /vehicles/:id` | `created_at` of the `vehicle.created` row in `fleet.activity_events` |
| `fleet.fleets` | every fleet rename | earliest `fleet.fleet_memberships.created_at` for the fleet |

`media.media_objects` is **not** in scope: its round-trip already carried `CreatedAt`, so no
media row was ever corrupted.

## Proxy accuracy — read this before trusting a backfilled value

Each proxy is an *upper bound with a known bias*, not the true value. The runbook must say
so, and the migration should be traceable so a backfilled value is never later mistaken for
an original measurement.

**`auth.users` ← earliest refresh token.** A refresh token is minted at first login, which
for a Google-provisioned user is the same request that created the row. Accurate to
sub-second **when the earliest token survives**. Tokens expire and may be pruned, so for
long-tenured users the earliest surviving token can post-date signup by an arbitrary
margin — potentially months. The known-corrupted production row from the issue
(`7a186017-d27e-4d65-90e3-6b240bf9880a`) should be checked by hand against its token history
before the bulk run, since it is the one row whose corruption is independently documented.

**`fleet.vehicles` ← `vehicle.created` activity event.** The strongest proxy of the three.
The event is written in the *same transaction* as the vehicle insert
(`vehicle/administrator.go` `InsertWithHooks`, design §8.2/A8), so where the event exists the
proxy is exact to within the transaction. Vehicles created before activity recording was
introduced will have no such event and are unrecoverable — leave them zeroed.

**`fleet.fleets` ← earliest membership.** The owner membership is created with the fleet in
one transaction (`membership/administrator.go`, `tx.Create(&fe)` then `tx.Create(&me)`), so
this is exact where the owner membership still exists. If the original owner was ever removed
and memberships hard-deleted, the proxy skews late.

## Ordering

**Deploy the code fix first; run the repair second.** (PRD FR-DATA-5.) Running the repair
against unfixed code means the next `PATCH` or login re-zeroes the row and the repair is
wasted work — worse, it produces a repaired-then-recorrupted row that looks like the
migration failed.

## Repair SQL (draft — validate against a bee snapshot before running)

Each statement is idempotent (FR-DATA-3) and guarded to touch only zeroed rows (FR-DATA-2).
The `< '1970-01-01'` predicate is deliberately looser than `= '0001-01-01'` so it also catches
any other pre-epoch garbage, while remaining incapable of matching a legitimate row.

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

-- auth.users ← earliest refresh token (weakest proxy; see accuracy notes)
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

Verification query for the FR-DATA-4 counts, run before and after:

```sql
SELECT 'auth.users'     AS tbl, COUNT(*) AS still_zero FROM auth.users     WHERE created_at < '1970-01-01'
UNION ALL
SELECT 'fleet.vehicles',        COUNT(*)               FROM fleet.vehicles  WHERE created_at < '1970-01-01'
UNION ALL
SELECT 'fleet.fleets',          COUNT(*)               FROM fleet.fleets    WHERE created_at < '1970-01-01';
```

Rows remaining after the repair are genuinely unrecoverable and must be reported as such,
not retried with a weaker proxy.

## Open items for design

- **Delivery mechanism.** A one-shot runbook under `docs/runbooks/` versus a Go migration in
  the service startup path. A runbook keeps a lossy, judgement-laden repair out of the
  automatic deploy path, which argues for it; but it relies on an operator remembering to run
  it. Recommend runbook, given the repair is one-time and needs a human to read the accuracy
  caveats.
- **Traceability.** Whether to record which rows were backfilled (so a future reader can tell
  an approximation from a measurement). Nothing in the current schema distinguishes them once
  written, and adding a column contradicts PRD §6's no-schema-change goal — so this may have
  to live in the runbook's output log rather than in the data.
- **Dry-run.** Recommend the runbook include a `SELECT`-only counting pass to be run and
  eyeballed before the `UPDATE` pass.
