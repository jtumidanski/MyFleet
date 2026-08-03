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
| 39 | `apps/web/src/pages/LoginPage.test.tsx` | cycles the theme without issuing a request | `fetchSpy` | (a) no-token early return in `updateThemePreference`; (b) signed-out page pointed at the mutation-bearing `ThemeToggle` (both required — FR-FIX-2) | red¹ (1 call) | red (3 calls to `/api/auth/me`) | falsifiable | migrated to `expectNoCall` | red |
| 40 | `apps/web/src/lib/hooks/api/auth.test.ts` | makes no request and resolves when there is no token | `fetchMock` | (a) no-token early return in `updateThemePreference` | red¹ (1 call) | red (request lands; downstream `TypeError` on the mock's undefined response) | falsifiable | migrated to `expectNoCall` (uniformity, FR-HELPER-3 — was already falsifiable) | red |

¹ Stage 1 for both sites was re-run under the **human ruling recorded in
migration-context.md ("Stage-1 probe placement — HUMAN RULING")**, which
supersedes the plan's original "insert immediately before the assertion"
wording. That literal wording is degenerate — a microtask queued immediately
before a synchronous `expect()` can never run before it, so it returns green
for every site regardless of what precedes it, and the first pass through
this task recorded exactly that false green for both rows above. The ruling
instead places the probe **at the dispatch point**: immediately after the
statement that triggers the call under test, and before any intervening
`await`/flush. Re-run against each site's pre-migration source (`git show
bd5c43c:<path>`) under that placement:

- Site 39: probe inserted after the third `act(() => toggle().click())` and
  before the pre-existing hand-rolled
  `await act(async () => { await new Promise((resolve) => setTimeout(resolve, 0)); })`
  flush. Command: `npx vitest run src/pages/LoginPage.test.tsx`. Result: RED —
  `expected "fetch" to not be called at all, but actually been called 1
  times`, the one call being `["__probe__"]`. The pre-existing flush is real
  and catches a probe queued at the real dispatch point, which is what this
  site's #20 fix is supposed to do.
- Site 40: probe inserted immediately before
  `await expect(updateThemePreference('dark')).resolves.toBeNull();` (the
  trigger — `updateThemePreference` is invoked directly, not through a
  mutation). Command: `npx vitest run src/lib/hooks/api/auth.test.ts`. Result:
  RED — same failure shape, one `__probe__` call observed; the `await` on the
  subject call itself drains the queued microtask.

Both rows are corrected from green to red accordingly, and the verdict
changes from "falsifiable, fragile" (row 2 of the design's combining table)
to plain **falsifiable** (row 1: red/red). Stage 2, the migration, and the
post-migration re-probes are unaffected by this correction — they were
already run with the probe immediately before `await expectNoCall(...)`,
which happens to coincide with the ruled placement once the helper's internal
flush sits between the probe and its own assertion.

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
