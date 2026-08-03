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
| 39 | `apps/web/src/pages/LoginPage.test.tsx` | cycles the theme without issuing a request | `fetchSpy` | (a) no-token early return in `updateThemePreference`; (b) signed-out page pointed at the mutation-bearing `ThemeToggle` (both required — FR-FIX-2) | green¹ | red (3 calls to `/api/auth/me`) | falsifiable, fragile | migrated to `expectNoCall` | red |
| 40 | `apps/web/src/lib/hooks/api/auth.test.ts` | makes no request and resolves when there is no token | `fetchMock` | (a) no-token early return in `updateThemePreference` | green | red (request lands; downstream `TypeError` on the mock's undefined response) | falsifiable, fragile | migrated to `expectNoCall` (uniformity, FR-HELPER-3 — was already falsifiable) | red |

¹ Task 3's brief predicted S1 = red for site 39 on the theory that the
pre-existing hand-rolled flush would let the synthetic probe be observed.
Observed was green. Reason: the probe is inserted **immediately before** the
final `expect(...)`, i.e. *after* the flush has already run and yielded —
scheduling a microtask there and synchronously asserting on the very next line
gives that microtask no chance to run before the assertion, regardless of
whether anything upstream already yielded. This is row 2 of the design's
combining table (`green, red → falsifiable only because the defeated guard
fires synchronously`), not a contradiction of it: S2 defeats the guard at the
real dispatch point (the `click()`s, well before the flush), so the
pre-existing flush *does* catch that real dispatch even though it can never
catch a probe scheduled after itself. Site 40 shows the same S1=green result
for the identical structural reason. Because the mechanism is placement-driven
rather than site-specific, this likely holds for Stage-1 probes generally
whenever the probe is inserted after an existing flush/await rather than at the
point of the real dispatch — worth a heads-up to later migration tasks, not a
blocker to this one.

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
