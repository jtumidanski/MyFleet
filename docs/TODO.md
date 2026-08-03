# TODO

Known work that is documented but not yet scheduled. Items here should graduate
to a numbered task via `/spec-task` when picked up.

## fleet-service

### The zero-owner invariant is TOCTOU-racy

Raised by the backend-guidelines review of task-014; deferred deliberately so
that task could ship. Full analysis in
[docs/tasks/task-014-member-names-ownership-transfer/audit.md](tasks/task-014-member-names-ownership-transfer/audit.md)
and `audit-backend.md` beside it.

`membership.Processor` reads `CountOwners` *outside* the write transaction:

- `internal/membership/processor.go:111-117` — demotion guard in `ValidateRoleChange`
- `internal/membership/processor.go:72-83` — self-removal guard in `ValidateRemoval`

The writes open their transactions later, at `internal/membership/administrator.go:71`
(`UpdateRole`) and `:103` (`Remove`). There is no row lock, no re-check inside
the transaction, and no database constraint expressing "a fleet has at least one
owner".

**Failure:** a fleet with owners A and B. A demotes B while B demotes A — or B
self-leaves — concurrently. Both read count=2, both pass the guard, both commit.
The fleet has zero owners, and neither PATCH nor DELETE can recover it because
both require an owner. Sequentially the guard is correct and covered by tests;
only the concurrent interleaving defeats it.

**Fix sketch:** re-validate inside the transaction, locking the fleet's owner
rows with `SELECT … FOR UPDATE` before counting. Two wrinkles make this more than
a one-liner:

1. The membership tests run on in-memory SQLite, which cannot execute row locks,
   so the lock needs a dialect gate (SQLite serialises writes anyway).
2. It duplicates the guard across the processor and the administrator, which
   `design.md` D5 did not contemplate — D5 discusses the transaction only for the
   activity record.
