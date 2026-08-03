# Probe Results — task-019

Every negative call assertion in `apps/web/src`, triaged by the two-stage probe
defined in [design.md](./design.md) §4. Rows are keyed on **file + `it(...)`
title + spy** rather than line number, because line numbers drift as sites are
migrated (design §4, "Keying the record").

**Stage 1** — a synthetic `Promise.resolve().then(() => spy())` inserted before
the assertion. Green = the assertion cannot observe promise-continuation
dispatch (Mode T vacuous).

**Stage 2** — the real production guard defeated. Green = the assertion did not
notice (vacuous). `n/a` = unconstructible (FR-TRIAGE-4); Stage 1 governs.

Verdict legend: **falsifiable** / **vacuous** / **unprobeable**.

| # | File | Test title | Spy | Guard defeated (stage 2) | S1 | S2 | Verdict | Fix | Re-probe |
|---|---|---|---|---|---|---|---|---|---|

## FR-TRIAGE-4 sites (unprobeable)

_(none recorded yet)_

## Lint rule demonstration (FR-LINT-4)

_(recorded in Task 11)_

## Residual gaps

- `packages/shared-ts` and `packages/ui-components` run under `make fe-test`
  but have no ESLint config at all (`tools/lint.sh` runs ESLint only for
  `apps/web`), so the new rule cannot reach them. Both currently contain zero
  negative call assertions, so the gap costs nothing today. Not fixed here.
- `expect(spy.mock.calls).toHaveLength(0)` and
  `expect(spy.mock.calls.length).toBe(0)` express the same assertion and are
  not matched by either selector. No site uses that spelling; adding selectors
  would risk flagging `media.test.ts`'s positive `mock.calls.length`
  comparisons. Accepted (design §5).
- `toHaveBeenCalledTimes(0)` matches nothing on the current tree — that
  selector is purely preventive (design F-5).
