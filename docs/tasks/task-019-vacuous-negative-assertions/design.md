# Vacuous Negative Assertions — Design

Version: v1
Status: Approved for planning
Created: 2026-08-03
PRD: [prd.md](./prd.md)
Issue: [#22](https://github.com/jtumidanski/MyFleet/issues/22)

---

## 1. What this document settles

The PRD fixes the *what*: triage all 40 negative call assertions in `apps/web`,
fix the vacuous ones, add a helper, enforce it with lint, record the evidence.
This document settles the *how*, and closes the four open questions the PRD
deferred to design — all four by experiment, not by argument.

Three things drive the design:

1. **The helper is trivial; the triage is the work.** The flush is five lines.
   Deciding, per site, whether an assertion can fail is 40 experiments. So the
   design spends most of its weight on the probe procedure and its record.
2. **Two independent vacuity modes exist**, and the PRD's mandated probe only
   sees one of them. §4 adds a second, cheaper probe that sees the other, and
   defines how the two verdicts combine.
3. **Everything asserted here was run.** §2 lists the commands and their output.

## 2. Design-phase findings

Each of these was produced by a throwaway probe in `apps/web`, run under
Vitest 2.1.9 / Node 22.22.2, then deleted. The worktree is clean.

### F-1 — `act()` outside a React root does not warn (closes OQ-2)

`@testing-library/react` sets `IS_REACT_ACT_ENVIRONMENT = true` **at import
time**, not at first `render()`. A probe file that imported RTL, mounted
nothing, and ran the flush recorded:

```
IS_REACT_ACT_ENVIRONMENT = true
console.error calls: []
console.warn calls: []
```

**Consequence:** the helper needs no branching, no `{ react: false }` option, and
no runtime detection of a mounted root. FR-HELPER-5 is satisfied by a single
code path.

**Consequence (binding):** the helper must import `act` from
`@testing-library/react`, **not** from `react`. React's own `act` does not set
the environment flag and *would* warn. This is the one import in the module that
is load-bearing, and it needs a comment saying so.

### F-2 — The flush works, and the bare form really is blind

Same probe, second case: a spy called from `Promise.resolve().then(...)` read
`0` calls synchronously and `1` after the flush.

```
bare sync read: 0
after flush: 1
```

This is the whole bug class, reproduced in four lines. It is also the shape the
helper's own unit test should take (Acceptance Criterion 4).

### F-3 — Vitest's failure message does *not* name the spy (amends FR-HELPER-4)

FR-HELPER-4 asks for a message that "names the spy". Delegating to
`expect(spy).not.toHaveBeenCalled()` does not deliver that — every anonymous
`vi.fn()` reports as the literal string `"spy"`:

```
AssertionError: expected "spy" to not be called at all, but actually been called 1 times
```

`mockName()` fixes it, and the built-in message is otherwise excellent (it
prints call count *and* every recorded argument list):

```
AssertionError: expected "mediaService.getContentBlob" to not be called at all, but actually been called 1 times
```

**Consequence:** the helper takes an optional `label` and applies it via
`spy.mockName(label)` before asserting. This keeps Vitest's rich output and adds
the missing name, rather than replacing a good message with a hand-rolled one.

The stack trace was also checked: it shows the helper frame *and* the calling
test frame, so a failure still points at the site. No `Error.captureStackTrace`
manipulation is needed.

### F-4 — Both proposed lint selectors work exactly as intended (validates FR-LINT-1/2)

Ran the PRD's selector plus a companion for the `Times(0)` form against a file
containing every nearby variant:

| Source line | Flagged | Correct? |
|---|---|---|
| `expect(spy).not.toHaveBeenCalled()` | SEL-A | yes |
| `expect(spy).not.toHaveBeenCalledWith('x')` | SEL-A | yes |
| `expect(spy).toHaveBeenCalledTimes(0)` | SEL-B | yes |
| `expect(spy).toHaveBeenCalledTimes(1)` | — | yes, positive count is not this bug |
| `expect(spy).toHaveBeenCalled()` | — | yes |
| `expect(spy).not.toHaveBeenCalledTimes(2)` | SEL-A | yes — a negative count assertion has the same timing hazard |
| `expect(spy).not.toBeNull()` | — | yes, no false positive on unrelated `.not` |

No false positives, no misses. The PRD's selector is adopted verbatim; SEL-B is
`CallExpression[callee.property.name='toHaveBeenCalledTimes'][arguments.0.value=0]`.

### F-5 — Inventory corrections

Three facts differ from the PRD; none change scope, all change the record.

- **20 files, not 18.** The 40 sites are spread across 20 test files.
  `grep -rc` breakdown: `VehiclePhotoThumbnail` 6, `MemberList` 5, `media` 4,
  `VehiclesPage` 3, `PhotoGalleryDialog` 3, then `usePendingAttachments`,
  `members`, `CategoryCombobox`, `useDashboardWidgets` at 2 each, and eleven
  files at 1.
- **Zero `toHaveBeenCalledTimes(0)` sites exist today.** The 40 = 38 bare
  `not.toHaveBeenCalled()` + 2 `not.toHaveBeenCalledWith(...)`. FR-LINT-2 is
  therefore purely preventive — it protects against a spelling nobody has used
  yet. Keep it; note in the record that it flags nothing on the current tree.
- **`packages/*` is clean but unprotected.** `make fe-test` also runs
  `packages/shared-ts` and `packages/ui-components`. Both currently have zero
  matches, so the PRD's scope boundary costs nothing today — but neither package
  has an ESLint config at all (`tools/lint.sh` runs ESLint only for
  `apps/web`), so the new rule will never reach them. Recorded as a residual
  gap, not fixed here.

## 3. Architecture: the helper

One new module, `apps/web/src/test/expectNoCall.ts`, following the existing
`src/test/` convention of one small module per concern (`renderWithProviders.tsx`,
`objectUrl.ts`) with direct imports and no barrel.

```ts
import { act } from '@testing-library/react'; // NOT from 'react' — see F-1
import { expect } from 'vitest';
import type { MockInstance } from 'vitest';

/**
 * Drains the microtask queue and one macrotask tick. React Query dispatches
 * both queries and mutations from a promise continuation, so an assertion
 * placed before this flush reads a spy that could not yet have been called.
 */
export async function flushPending(): Promise<void> {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

/** Flushes, then asserts the spy was never called. `label` names the spy in the failure. */
export async function expectNoCall(spy: MockInstance, label?: string): Promise<void> {
  await flushPending();
  if (label) spy.mockName(label);
  expect(spy).not.toHaveBeenCalled();
}

/** Flushes, then asserts the spy was never called with `args`. */
export async function expectNoCallWith(
  spy: MockInstance,
  args: unknown[],
  label?: string,
): Promise<void> {
  await flushPending();
  if (label) spy.mockName(label);
  expect(spy).not.toHaveBeenCalledWith(...args);
}
```

Decisions embedded above, with reasons:

- **`expectNoCall`, not `expectNoRequest` (closes OQ-1).** Twelve of the 40 sites
  spy on non-network callbacks — `onSubmit`, `onChange`, `onLoadMore`, and four
  `toast.*` variants. `expectNoRequest` would be a lie at nearly a third of the
  call sites.
- **`flushPending` is exported separately.** Some sites need the flush without
  the assertion (the fix for a site whose second failure mode is ordering, not
  the assertion itself). Exporting it avoids the alternative — call sites
  hand-rolling their own `setTimeout(0)`, which is exactly the drift the lint
  rule exists to stop.
- **`args` as an array, not rest.** Only two sites use the `With` variant, so
  the array costs almost nothing, and it keeps `label` in the same trailing
  position in both functions. Rest args would force `label` into an options
  object or drop it from the `With` variant.
- **`MockInstance` as the parameter type.** It is the widest of Vitest 2's spy
  types: both `vi.fn()` (`Mock`) and `vi.mocked(service.method)`
  (`MockedFunction`) extend it, and `mockName` is declared on it. If the default
  generic rejects a concrete mock at `tsc -b`, widen to
  `MockInstance<(...args: never[]) => unknown>` before reaching for `any`.
- **The flush is exactly the one proven in #20.** FR-HELPER-2 sets it as a floor.
  Do not deepen it speculatively; if OQ-3 turns up a site needing more, deepen it
  **in the helper** and re-run every migrated file, so no call site carries a
  bespoke flush.

### Guidance for the two `With` sites

`not.toHaveBeenCalledWith(x)` has a second vacuity mode that a flush cannot
touch: it passes when the spy was **never called at all**. `media.test.ts:219`
already guards against this by pinning `toHaveBeenCalledTimes(1)` two lines
above; `CategoryCombobox.test.tsx:157` does not obviously do so. Where the
intent is "called, but not with X", the site must carry a positive assertion
alongside the negative one. This is guidance for the migration, not new API.

## 4. Architecture: the triage procedure

### The two vacuity modes

The PRD's `LoginPage` case failed for **two** independent reasons. That is the
key structural fact, and it means one probe is not enough:

- **Mode T (timing).** The assertion runs before a promise continuation could
  dispatch. Detected by making the spy fire asynchronously.
- **Mode R (reachability).** The subject under test could never have called the
  spy in this test's setup — `LoginPage`'s `beforeEach` cleared `localStorage`,
  so `updateThemePreference` short-circuited on the missing token long before
  timing mattered. Detected only by defeating the real guard.

FR-TRIAGE-2's guard-defeat probe sees Mode R and, incidentally, some of Mode T.
It cannot be constructed at all for FR-TRIAGE-4 sites. So the design uses two
stages.

### Stage 1 — timing probe (mechanical, all 40 sites, no production edits)

Immediately before the assertion, insert a synthetic asynchronous call to the
same spy and run the file:

```ts
void Promise.resolve().then(() => (SPY as unknown as Mock)('__probe__'));
```

Green means the assertion cannot observe a promise-continuation call — Mode T
vacuous, by direct measurement. Red means something ahead of it already yields.
For the two `With` sites, the synthetic call must use the exact banned
arguments, or the probe proves nothing.

This is cheap, uniform, scriptable across all 40 sites, requires no production
source to be touched, and — critically — it is the **only** probe available for
a site where FR-TRIAGE-4 applies.

### Stage 2 — guard-defeat probe (per FR-TRIAGE-2, authoritative)

Temporarily defeat the real guard in production source (remove the early return,
drop the `enabled` flag, delete the structural check), run the file, record the
verdict, revert. This is the PRD's mandated probe and stays the authoritative
verdict, because it is the only one that exercises the actual regression the
assertion exists to catch.

### Combining the verdicts

| Stage 1 | Stage 2 | Verdict | Action |
|---|---|---|---|
| red | red | Falsifiable | Migrate to helper anyway (FR-HELPER-3) |
| green | red | Falsifiable only because the defeated guard fires synchronously | Migrate; note the fragility |
| red | green | Suspicious — probe mis-constructed, or Mode R (`LoginPage`'s shape) | Investigate; FR-FIX-2 applies |
| green | green | Vacuous | Fix, then re-probe both stages to red (FR-FIX-1) |
| any | unconstructible | FR-TRIAGE-4 | Stage 1 governs; record reasoning and disposition per FR-DOC-3 |

The `red / green` row is the one that earns the second stage. Without stage 1
that combination is invisible: a lone green stage-2 reads as "probe failed" and
gets retried, when it is often the signal that the test's own setup starved the
subject.

### Probe hygiene (FR-TRIAGE-5)

FR-TRIAGE-5's stated gate — `git diff` against production sources — catches
stage-2 leakage but **not** stage-1 leakage, because stage-1 scaffolding lives in
test files that are legitimately modified by this task. Add two explicit final
checks:

```sh
git diff main -- apps/web/src --stat        # production files must not appear
grep -rn "__probe__" apps/web/src            # must return nothing
```

Both belong in the plan as a named verification step, not as an assumption.

### Keying the record

Test-file line numbers shift as sites are migrated, so the PRD's line-number
inventory decays the moment work starts. The probe record must key each row on
**file + `it(...)` title + spy**, carrying the PRD's site number for
cross-reference. A future audit can then re-find every row.

## 5. Architecture: lint enforcement

Two blocks in `apps/web/eslint.config.js`. The first extends the existing
test-files block — which today sets only `languageOptions.globals` — with the
validated selectors:

```js
{
  files: ['**/*.{test,spec}.{ts,tsx}', 'src/test/**/*.{ts,tsx}'],
  languageOptions: { globals: { ...globals.node } },
  rules: {
    'no-restricted-syntax': ['error',
      {
        selector:
          "CallExpression[callee.object.property.name='not']" +
          "[callee.property.name=/^toHaveBeenCalled/]",
        message:
          'Use expectNoCall(spy) from src/test/expectNoCall — a bare ' +
          'not.toHaveBeenCalled() runs before promise-continuation dispatch ' +
          'and can pass vacuously. See issue #22.',
      },
      {
        selector:
          "CallExpression[callee.property.name='toHaveBeenCalledTimes']" +
          '[arguments.0.value=0]',
        message:
          'toHaveBeenCalledTimes(0) is not.toHaveBeenCalled() spelled ' +
          'differently — use expectNoCall(spy). See issue #22.',
      },
    ],
  },
},
{
  // The helper necessarily contains the banned expression, and its own test
  // must contain the bare form to demonstrate the contrast it exists to fix.
  files: ['src/test/expectNoCall.ts', 'src/test/expectNoCall.test.ts'],
  rules: { 'no-restricted-syntax': 'off' },
},
```

Notes:

- The exemption (FR-LINT-3) must cover the **helper's test file too**, not just
  the helper. That test's whole point is showing a bare assertion passing where
  the helper fails; without the exemption it cannot be written.
- The existing test-files block already includes `src/test/**`, so the exemption
  block must come **after** it in the flat-config array to win.
- **Residual gap, accepted:** `expect(spy.mock.calls).toHaveLength(0)` and
  `expect(spy.mock.calls.length).toBe(0)` express the same assertion and are not
  matched. Adding selectors for them is speculative — no site uses that spelling
  — and the rule's message teaches the reason, which is the real defence.
  `media.test.ts:255-256` uses `mock.calls.length` for *positive* comparisons,
  which any such selector would have to avoid flagging.

## 6. Sequencing

Recommended order, and the one non-obvious call in it:

1. **Helper + its unit test.** The unit test is F-2 turned into a permanent
   assertion: a bare synchronous check passes on a promise-continuation call,
   the helper fails on the same call. This is Acceptance Criterion 4 and it
   validates the helper before 20 files depend on it.
2. **Lint rule, added early — deliberately.** Adding it now makes
   `make lint-check` red across all 40 sites, and that red list *is* the
   worklist: `npx eslint src` enumerates exactly the un-migrated sites, by file
   and line, for free. The cost is that `make lint-check` is red at
   intermediate commits on this branch. That trade is worth taking on a
   single-PR branch; the final commit must be green, and the plan should say so
   explicitly so the red is never mistaken for a finished state.
3. **Group E first** (`LoginPage`, `auth.test.ts`). `LoginPage` is the known-good
   reference — it already has the proven flush — so migrating it first
   end-to-end validates the helper against a site whose correct behaviour is
   already established. `auth.test.ts:92` is the documented not-affected case;
   confirm and leave it (it awaits `updateThemePreference()` directly, and the
   short-circuit is the subject under test — "fixing" it would obscure that).
4. **Groups A, B, C**, file by file — 18 remaining files, one commit per file or
   per tight cluster, so each probe/revert cycle stays small and reviewable.
   `VehiclePhotoThumbnail` (6 sites) and `MemberList` (5) are the largest.
5. **Group D last.** Probe both sites (FR-TRIAGE-6 requires it even though the
   answer is expected). Per OQ-4, **exempt** confirmed-synchronous sites from the
   helper via an inline `eslint-disable-next-line no-restricted-syntax` carrying
   a comment that names the probe result. An `eslint-disable` with evidence
   attached is more honest than a flush that provably does nothing — and it
   keeps `download.test.ts`, a pure-util test, from importing
   `@testing-library/react` for no reason.
6. **Lint demonstration** (FR-LINT-4) and **record** (FR-DOC-1/2/3).
7. **`make ci`**, plus the two hygiene checks from §4.

## 7. Alternatives considered

**Custom Vitest matcher** — `await expect(spy).toHaveNoPendingCalls()` instead of
a free function. Rejected: it needs `expect.extend` in the setup file plus a
TypeScript declaration merge, and the natural spelling (`expect(spy).not.…`) is
the exact syntax the lint rule bans, so the matcher would have to be phrased
positively and awkwardly. More machinery, worse ergonomics, no gain.

**Fake timers** — `vi.useFakeTimers()` + `vi.runAllTimersAsync()`. Rejected:
most inventory sites use `userEvent`, which hangs under fake timers unless every
call site configures `advanceTimers`. It would convert a five-line helper into a
per-file migration hazard.

**`waitFor`-based flush** — Rejected for the reason the PRD already gives, and it
is worth restating because it is counter-intuitive: waiting for a *non*-event
has no success condition to poll for, so `waitFor` would either return
immediately (proving nothing) or burn its full timeout on every one of ~38
sites. A fixed `setTimeout(0)` is both deterministic and faster.

**Global auto-flush** — patching the environment so every assertion is preceded
by a flush. Rejected: assertions are mid-test, there is no interception point,
and it would silently change the timing of the ~200 positive assertions that are
working correctly today.

**A convention test instead of a lint rule** — `src/test/conventions.test.ts`
already enforces cross-file invariants by reading source and asserting on it, so
a regex-over-test-files check would fit the existing house style. Rejected in
favour of lint: ESLint parses the AST rather than matching text (F-4 shows the
selector correctly distinguishes `toHaveBeenCalledTimes(0)` from `(1)`, which a
regex would fumble), it reports at the offending line in the editor, and
`no-restricted-syntax` carries the teaching message the PRD's NFR asks for. The
convention test remains the right tool for invariants ESLint cannot see.

**Inspection instead of probing** — the PRD already rejects this, and the
`LoginPage` history is the proof: two independent reasons for vacuity, neither
visible in the test file alone.

## 8. Risks

| Risk | Mitigation |
|---|---|
| `mockName()` persists for the rest of the file, so a later unrelated failure on the same module-level mock prints the label from an earlier assertion. | The label is always the spy's real name, so it stays accurate. Set it only when a label is passed. |
| Importing the helper into a pure-util test pulls in `@testing-library/react` and its auto-cleanup `afterEach`. | Harmless (cleanup with nothing mounted is a no-op), and the OQ-4 exemption means `download.test.ts` never imports it. |
| `make fe-test` wall time regresses past the NFR's 10% ceiling. | Record the baseline before migration and re-measure at the end. ~38 macrotask ticks should be unmeasurable; if not, the helper — not the call sites — changes. |
| Stage-1 probe scaffolding survives into the final diff. | The `grep -rn "__probe__"` gate in §4, run as a named plan step. |
| A probe reveals a real production bug. | FR-FIX-3: record it, raise a separate issue, do not fix it here. |
| PRD line numbers drift during migration, orphaning the record. | Key the record on file + test title + spy (§4). |

## 9. Open questions — resolved

- **OQ-1 (naming)** — `expectNoCall`, with `expectNoCallWith` and `flushPending`
  alongside. §3.
- **OQ-2 (act scope)** — No branching needed; `act` outside a React root is
  silent because RTL sets `IS_REACT_ACT_ENVIRONMENT` at import. F-1.
- **OQ-3 (flush depth)** — Stays a probe output, as the PRD says. The floor is
  the #20 flush; any deepening happens in the helper and triggers a re-run of
  every migrated file. §3.
- **OQ-4 (Group D exemption)** — Exempt, via inline disable with a comment naming
  the probe result. §6 step 5.

One question is opened and answered by this design rather than by the PRD:
**how to detect Mode R vacuity systematically**, answered by the two-stage probe
in §4.

## 10. Acceptance criteria — coverage

Every criterion in PRD §10 is addressed: probe verdicts by §4 (two-stage, with a
record keyed to survive line drift), fixes and re-probes by §4's verdict matrix,
the helper and its falsifiability test by §3 and §6 step 1, uniform adoption and
the Group D exemption by §6 step 5, all three lint spellings by §5 (validated in
F-4), the lint demonstration by §6 step 6, production-diff purity by §4's two
hygiene checks, and `make ci` by §6 step 7.
