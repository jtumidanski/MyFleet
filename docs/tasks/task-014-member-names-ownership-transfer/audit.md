# task-014 — Code Review Summary

Three reviewers ran in parallel over `main...HEAD`. Their full reports:

- [audit-plan-adherence.md](audit-plan-adherence.md) — plan-adherence-reviewer
- [audit-backend.md](audit-backend.md) — backend-guidelines-reviewer (DOM/SUB/SEC)
- [audit-frontend.md](audit-frontend.md) — frontend-guidelines-reviewer (FE-\*)

| Reviewer | Verdict as reported | Status after fixes |
|---|---|---|
| Plan adherence | FULL adherence — READY_TO_MERGE | unchanged |
| Backend guidelines | NEEDS-WORK — 3 Important | 2 fixed, 1 deferred by decision |
| Frontend guidelines | NEEDS-WORK — 1 blocking, 4 Important | blocking + 2 fixed, 2 deferred |

No Critical findings from any reviewer. Every security property the plan named
(guard ordering, `RequireSameFleet` outside the `isSelf` branch, dedupe-before-cap,
foreign-vs-nonexistent ids indistinguishable, constant error strings,
activity-inside-the-transaction) passed with test-backed evidence.

---

## Fixed in this branch

**IMP-2 — `CountOwners` ignored `status`** (`provider.go:77`). It is the guard for
the zero-owner invariant, while `ValidateRoleChange` refuses to act on a
non-active target. One active plus one revoked owner would have counted as 2 and
made the last real owner demotable. Now filters on `status = 'active'`, matching
`ListActiveByFleetID`. Pinned by `TestCountOwners_countsOnlyActiveOwners` and
`TestCountOwners_isScopedToTheFleet`.

**IMP-3 — writes ignored `RowsAffected`** (`administrator.go:69`, `:102`). The
model is read outside the transaction, so a concurrent delete meant the write
matched zero rows, the transaction still committed, and `member.role_changed` or
a departure event was recorded for a membership that no longer existed — while
the handler returned 200 with a model computed in memory and never read back.
That is a real corruption of the audit trail, because the activity row is the
*only* record a hard-deleted membership ever existed. Both writes now return
`ErrNotFound`, which rolls the transaction back, and both handlers translate it
to a 404 without logging it as an incident. Pinned by
`TestUpdateRole_returnsNotFoundAndRecordsNothingWhenTheRowIsGone` and
`TestRemove_returnsNotFoundAndRecordsNothingWhenTheRowIsGone`.

**FE-15 — cursor affordance** (`ui/select.tsx`). `SelectTrigger` had no
`cursor-pointer` and `SelectItem` set `cursor-default` explicitly, though it
renders as `<div role="option">`. Both now carry `cursor-pointer`. This also
repairs three pre-existing call sites outside this task.

**Owner gating read the JWT claim** (`MemberList.tsx`). The component argued the
members list is the source of truth for role, then gated Remove and Make-owner
on an `isOwner` prop that `SettingsPage` derived from `useAuth().role`. A
just-promoted user saw no owner actions; a just-demoted one saw actions the
server answers with 403. `isOwner` is now derived from the list, the prop is
gone, and `SettingsPage` no longer passes it. The existing "hidden from
non-owners" test pins the direction that matters: the `useAuth` mock claims
`owner` in every test, so a component gating on the claim would fail it.

**`activeMembers` asserted a filter it did not apply** (`MemberList.tsx`). The
whole leave matrix rests on it and the endpoint behind it is the unfiltered one.
Now actually filters on `status === 'active'`.

**Missing colocated test** for `lib/utils/displayName.ts` — added, covering the
`||`-not-`??` empty-string fallthrough and the FR-1.7 lookup-failure path.

---

## Deferred by decision

**IMP-1 — the zero-owner invariant is TOCTOU-racy.** `CountOwners` is read in the
processor (`processor.go:111-117` for demotion, `:72-83` for self-removal)
*outside* the write transactions opened at `administrator.go:71` and `:103`.
There is no row lock, no re-check inside the transaction, and no database
constraint. Owners A and B demoting each other concurrently both read count=2,
both pass the guard, both commit — the fleet is left with zero owners and no
in-band recovery, since both mutating routes require an owner.

Sequentially the guard is correct and well tested; concurrently it is not.
`design.md` D5 discusses the transaction only for the activity record and never
addresses this.

Deferred deliberately: the fix is to re-validate inside the transaction with
`SELECT … FOR UPDATE` on the fleet's owner rows, which needs a dialect gate
because the SQLite test harness cannot execute row locks, and it duplicates the
guard across the processor and the administrator. That is a design change the
plan and design doc never contemplated, and the self-removal half of the race
predates this task. Tracked in `docs/TODO.md`.

**Title case on buttons.** "Make owner", "Transfer & leave" and "New owner" are
sentence case. `patterns-components.md:277` asks for title case; the copy is
verbatim from `ux-flow.md:76,84`. The design doc and the guideline disagree —
worth resolving in one place rather than silently in this PR.

**`MembershipAttributes.role: string`** (`types/models/membership.ts:10`) is an
untyped string while the new mutation uses `FleetRole` (`members.ts:131`). Every
leave-matrix branch compares it against `'owner'`. Tightening the shared type
touches call sites beyond this task.

---

## Noted, not acted on

- **DOM-15** — handlers call `adm.UpdateRole` / `adm.Remove` directly rather than
  going through the processor. Flagged as a FAIL, but it is the house pattern:
  `invite/resource.go:90,191,237`, `mileage:123`, `fuel:211,237`, `fleet:44`.
  Changing it here would make this package the odd one out.
- **DOM-19** — no `t.Run` table tests in the six changed test files.
- **`Administrator.Delete`** (`administrator.go:16,59`) is now dead in production
  code, leaving an unaudited hard-delete path beside the audited `Remove`.
- **Plan template defect** — the `go build ./...` / `go test ./...` commands in
  plan Tasks 1-6 cannot work: there is no root `go.mod`, so `./...` from the
  workspace root errors with *"directory prefix . does not contain modules listed
  in go.work"*. Those "verify the tests fail" steps, as literally written, never
  ran. Use the Makefile's `github.com/jtumidanski/myfleet/...` form in future plans.
- **Radix bump coverage** — the `@radix-ui/react-select` upgrade moved 35
  transitive packages. Of the four `Select` consumers, `InviteForm.tsx` (its test
  never opens the Select) and `MaintenanceScheduleForm.tsx` (no test file) have no
  coverage of that surface.
- **Three PRD §10 criteria have structural rather than automated evidence**: the
  401-no-JWT case (route placement inside the JWT group at
  `auth-service/cmd/main.go:88-96`), the onboarding redirect after leaving (the
  token mint and identity refetch are tested; the redirect is not), and "the
  fleet retains exactly one owner" after Transfer & leave (the web test asserts
  call ordering; the server guards that enforce it are directly tested).
