# Vacuous Negative Assertions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every negative call assertion in `apps/web` capable of failing — triage all 40 by probe, fix the vacuous ones through a shared flush helper, and enforce the helper with lint so the pattern cannot silently return.

**Architecture:** One new test-only module `apps/web/src/test/expectNoCall.ts` exports `flushPending()`, `expectNoCall(spy, label?)`, and `expectNoCallWith(spy, args, label?)`. The flush is `await act(async () => { await new Promise((r) => setTimeout(r, 0)) })`, which drains the microtask queue where React Query's `mutate()` dispatches plus one macrotask tick. Every inventory site is triaged by a **two-stage probe** (stage 1 = synthetic async call, detects timing vacuity; stage 2 = defeat the real production guard, detects reachability vacuity), then migrated to the helper. An ESLint `no-restricted-syntax` rule added early makes `npm run lint` enumerate the un-migrated worklist for free.

**Tech Stack:** TypeScript 5.5, Vitest 2.1.9 (`globals: true`, `jsdom`), `@testing-library/react` 16.3.2, `@testing-library/user-event` 14.6.1, TanStack React Query 5, ESLint 9 flat config (`apps/web/eslint.config.js`), Prettier 3.9.6.

## Global Constraints

- **No production source file may be modified.** `git diff main -- apps/web/src --stat` on the finished branch must show only `*.test.ts`/`*.test.tsx` files and `src/test/expectNoCall.ts`. (PRD FR-FIX-3.)
- **Every probe must be reverted.** `grep -rn "__probe__" apps/web/src` must return nothing on the finished branch. (PRD FR-TRIAGE-5, design §4 "Probe hygiene".)
- **A probe that reveals a real production bug is recorded and raised as a separate issue, not fixed here.** (PRD FR-FIX-3.)
- **The flush is exactly the one proven in issue #20** — `await act(async () => { await new Promise((resolve) => setTimeout(resolve, 0)) })`. It is a floor, not a ceiling: if a probe shows a site needs more, deepen it **in the helper** and re-run every already-migrated file. Never add a bespoke flush at a call site. (PRD FR-HELPER-2, design §3.)
- **`act` must be imported from `@testing-library/react`, never from `react`.** RTL sets `IS_REACT_ACT_ENVIRONMENT = true` at import time; React's own `act` does not, and would warn in the non-React test files (`media.test.ts`, `users.test.ts`, `vehicleRecords.test.ts`, `useDashboardWidgets.test.ts`). This import is load-bearing and carries a comment saying so. (Design F-1.)
- **The helper is incompatible with fake timers.** Its `setTimeout(0)` never fires under `vi.useFakeTimers()`, so a migrated site inside a fake-timer suite would hang until the Vitest timeout. Exactly one inventory file uses fake timers — `src/lib/utils/download.test.ts` — and it is exempt under Group D. Do not migrate any site into a `vi.useFakeTimers()` scope.
- **Record keys are `file + it(...) title + spy`, never line numbers.** Line numbers drift the moment migration starts. (Design §4 "Keying the record".)
- **`make lint-check` is red on this branch from Task 2 until Task 10 completes.** That is deliberate — the red list is the worklist. It is not a finished state. Every task before Task 10 gates on its own files being green (`npx eslint <file>`), not on the whole tree.
- **Node is not on `PATH` by default.** Every shell that runs `npm`/`npx` must first run:
  `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22`
- **All `npm`/`npx` commands run from `apps/web`** unless stated otherwise. `make` targets run from the repo root.
- **Prettier is enforced.** Run `npm run format` from the repo root (or `npx prettier --write <files>`) before each commit; `make lint-check` runs `format:check`.
- **The inventory is 40 sites across 20 files** — 38 bare `not.toHaveBeenCalled()` and 2 `not.toHaveBeenCalledWith(...)`. There are **zero** `toHaveBeenCalledTimes(0)` sites today; that lint selector is purely preventive. (Design F-5.)

---

## File Structure

**Created:**

| Path | Responsibility |
|---|---|
| `apps/web/src/test/expectNoCall.ts` | The flush + the three exported assertions. The only new production-adjacent module. |
| `apps/web/src/test/expectNoCall.test.ts` | Proves the helper catches a promise-continuation call that a bare synchronous assertion misses. |
| `docs/tasks/task-019-vacuous-negative-assertions/probe-results.md` | The FR-DOC-1/2/3 record: one row per site, plus the lint demonstration output. |

**Modified:**

| Path | Change |
|---|---|
| `apps/web/eslint.config.js` | Two selectors in the existing test-files block, plus an exemption block for the helper and its test. |
| 18 test files (all except the two Group D exemptions, which get inline disables instead) | Migrated to `expectNoCall` / `expectNoCallWith`. |

**Touched during probing, reverted before every commit:** production modules under `apps/web/src` — named per site in the probe tables below.

---

## Site Inventory (authoritative, keyed per design §4)

Line numbers below are as of commit `b1c2252` and are for locating the site only. The record keys on file + title + spy.

| # | File | `it(...)` title | Spy | Callback | Group |
|---|---|---|---|---|---|
| 1 | `VehicleCard.test.tsx` | renders the placeholder at identical dimensions when no photo is set | `mediaService.getContentBlob` | **sync** | A |
| 2 | `VehiclePhotoThumbnail.test.tsx` | shows the "No photo" placeholder when there is no media id, and fetches nothing | `mediaService.getContentBlob` | **sync** | A |
| 3 | `VehiclePhotoThumbnail.test.tsx` | says "Photo unavailable", not "No photo", when a known photo cannot be fetched | `mediaService.getContentBlob` | async | A |
| 4 | `media.test.ts` | rejects an oversized file before any request, with a message naming the limit | `deps.initUpload` | async | A |
| 5 | `media.test.ts` | (same title) | `deps.putContent` | async | A |
| 6 | `media.test.ts` | (same title) | `deps.confirm` | async | A |
| 7 | `usePendingAttachments.test.ts` | does not call remove for an item that never uploaded | `mediaService.remove` | async | B |
| 8 | `usePendingAttachments.test.ts` | commit disarms the unmount cleanup and returns the media ids | `mediaService.remove` | async | B |
| 9 | `ActivityFeed.test.tsx` | renders the empty state without asking for any names | `listByIds` | async | C |
| 10 | `useDashboardWidgets.test.ts` | does not call saveLayout when addWidget is invoked while the layout query is still loading | `dashboardService.saveLayout` | async | C |
| 11 | `useDashboardWidgets.test.ts` | does nothing at the list boundaries | `dashboardService.saveLayout` | async | C |
| 12 | `MemberList.test.tsx` | does not fire the DELETE until the dialog is confirmed | `memberService.removeMember` | async | C |
| 13 | `MemberList.test.tsx` | fires nothing when the dialog is cancelled | `memberService.removeMember` | async | C |
| 14 | `MemberList.test.tsx` | is offered to owners on non-owner rows and confirms before PATCHing | `memberService.updateRole` | async | C |
| 15 | `MemberList.test.tsx` | offers a plain leave confirmation to a member | `memberService.updateRole` | async | C |
| 16 | `MemberList.test.tsx` | does not remove the leaver when the promote fails | `memberService.removeMember` | async | C |
| 17 | `CategoryCombobox.test.tsx` | creates a category and selects the id the server returned, not anything derived locally | `onChange` (**With**) | async | C |
| 18 | `CategoryCombobox.test.tsx` | surfaces a toast and selects nothing when creation fails | `onChange` | async | C |
| 19 | `VehicleRecordsTable.test.tsx` | disables load more while a page is in flight | `onLoadMore` | async | C |
| 20 | `PhotoGalleryDialog.test.tsx` | still reports success when only the object cleanup fails | `toast.error` | async | C |
| 21 | `PhotoGalleryDialog.test.tsx` | does not remove anything until the confirmation is accepted | `removeMedia` | async | C |
| 22 | `PhotoGalleryDialog.test.tsx` | (same title) | `removeObject` | async | C |
| 23 | `MaintenanceRecordForm.test.tsx` | rejects a description over 200 characters | `onSubmit` | async | C |
| 24 | `VehiclePhotoThumbnail.test.tsx` | fires no toast when a thumbnail fails to load | `toast.error` | async | C |
| 25 | `VehiclePhotoThumbnail.test.tsx` | (same title) | `toast` | async | C |
| 26 | `VehiclePhotoThumbnail.test.tsx` | (same title) | `toast.warning` | async | C |
| 27 | `VehiclePhotoThumbnail.test.tsx` | (same title) | `toast.info` | async | C |
| 28 | `media.test.ts` | revokes the previous URL exactly once when the id changes, and the new URL survives | `revokeObjectURL` (**With**) | async | C |
| 29 | `members.test.ts` | useRemoveMember does not mint a token when removing another member | `mintAccessToken` | async | C |
| 30 | `members.test.ts` | useUpdateMemberRole does not mint a token | `mintAccessToken` | async | C |
| 31 | `users.test.ts` | does not fire a request when there are no ids | `userService.listByIds` | **sync** | C |
| 32 | `vehicleRecords.test.ts` | loadMore calls fetchNextPage only on sources that still have a next page | `fuel.fetchNextPage` | **sync** | C |
| 33 | `VehiclesPage.test.tsx` | keeps the dialog open with inline errors when required fields are blank | `vehicleService.createInFleet` | async | C |
| 34 | `VehiclesPage.test.tsx` | keeps the dialog open with the typed values when the request fails | `vehicleService.createInFleet` | async | C |
| 35 | `VehiclesPage.test.tsx` | closes on an outside pointer-down without creating a vehicle | `vehicleService.createInFleet` | async | C |
| 36 | `AdminFleetsPage.test.tsx` | opens the confirmation dialog instead of purging directly | `createPurgeMutate` | async | C |
| 37 | `input.test.tsx` | does not reach for a picker on non-picker types | `showPicker` | async | D |
| 38 | `download.test.ts` | revokes the object URL, but not before the click | `URL.revokeObjectURL` | **sync** | D |
| 39 | `LoginPage.test.tsx` | cycles the theme without issuing a request | `fetchSpy` | async | E |
| 40 | `auth.test.ts` | makes no request and resolves when there is no token | `fetchMock` | async | E |

The five **sync** callbacks (#1, #2, #31, #32, #38) must gain `async` when migrated — except #38, which is exempt and stays synchronous.

---

## The probe procedure (referenced by every migration task)

Run both stages per site. Record both verdicts.

### Stage 1 — timing probe (no production edits)

Insert immediately **before** the assertion under test, then run the file:

```ts
void Promise.resolve().then(() => (SPY as unknown as import('vitest').Mock)('__probe__'));
```

For a `not.toHaveBeenCalledWith(...)` site, the synthetic call **must carry the exact banned arguments**, or the probe proves nothing:

```ts
void Promise.resolve().then(() => (SPY as unknown as import('vitest').Mock)(BANNED_ARG));
```

- **Green** → the assertion cannot observe a promise-continuation call. **Mode T vacuous.**
- **Red** → something ahead of it already yields to the event loop.

Delete the probe line before moving on.

### Stage 2 — guard-defeat probe (authoritative)

Edit the production module to defeat the guard named in the task's probe table, run the file, record the verdict, then `git checkout -- <production file>`.

- **Green** → the assertion did not notice the guard being removed. **Vacuous.**
- **Red** → the assertion caught it.
- **Unconstructible** → no production edit can make the spy fire. Record under FR-DOC-3; Stage 1 governs the verdict.

### Combining (design §4)

| Stage 1 | Stage 2 | Verdict | Action |
|---|---|---|---|
| red | red | Falsifiable | Migrate anyway (FR-HELPER-3) |
| green | red | Falsifiable only because the defeated guard fires synchronously | Migrate; note the fragility in the record |
| red | green | Mode R — the test's own setup starved the subject | Investigate; FR-FIX-2 applies (fix the setup too) |
| green | green | Vacuous | Fix, then re-probe **both** stages to red |
| any | unconstructible | FR-TRIAGE-4 | Stage 1 governs; record reasoning + disposition |

### After migrating a vacuous site (FR-FIX-1)

Re-run **both** probes against the migrated code. Both must be red. A fix that is not re-probed is not a fix.

---

## The migration transform (referenced by every migration task)

Mechanical, with three shapes.

**1. Bare assertion, spy is a local `vi.fn()`:**

```ts
// before
expect(onChange).not.toHaveBeenCalled();
// after
await expectNoCall(onChange, 'onChange');
```

**2. Bare assertion, spy is a mocked module method** (needs `vi.mocked()` because the imported symbol is typed as the real function, not as a `MockInstance`):

```ts
// before
expect(mediaService.getContentBlob).not.toHaveBeenCalled();
// after
await expectNoCall(vi.mocked(mediaService.getContentBlob), 'mediaService.getContentBlob');
```

**3. `With` variant:**

```ts
// before
expect(onChange).not.toHaveBeenCalledWith('Skid Plate');
// after
await expectNoCallWith(onChange, ['Skid Plate'], 'onChange');
```

The `label` is passed at **every** site. Vitest reports an unnamed `vi.fn()` as the literal string `"spy"` (design F-3), and every inventory spy is anonymous, so without the label the failure message names nothing.

**Import paths** (add to the file's existing import block, after the other `../test/*` imports):

| Test file location | Import specifier |
|---|---|
| `src/pages/*.test.tsx` | `'../test/expectNoCall'` |
| `src/pages/admin/*.test.tsx` | `'../../test/expectNoCall'` |
| `src/components/features/<area>/*.test.tsx` | `'../../../test/expectNoCall'` |
| `src/components/features/vehicles/<sub>/*.test.tsx` | `'../../../../test/expectNoCall'` |
| `src/lib/hooks/*.test.ts` | `'../../test/expectNoCall'` |
| `src/lib/hooks/api/*.test.ts` | `'../../../test/expectNoCall'` |

If the enclosing `it(...)` callback is synchronous, change `() => {` to `async () => {`.

---

## Task 1: The helper and its falsifiability test

**Files:**
- Create: `apps/web/src/test/expectNoCall.ts`
- Create: `apps/web/src/test/expectNoCall.test.ts`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `flushPending(): Promise<void>`
  - `expectNoCall(spy: MockInstance, label?: string): Promise<void>`
  - `expectNoCallWith(spy: MockInstance, args: unknown[], label?: string): Promise<void>`

  Every later task imports these three by exactly these names and signatures.

- [ ] **Step 1: Record the `fe-test` baseline wall time**

The NFR caps regression at 10% (PRD §8). Capture the number now, before any migration.

```bash
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
cd /home/tumidanski/source/MyFleet/.worktrees/task-019-vacuous-negative-assertions
time make fe-test 2>&1 | tail -30
```

Write the wall-clock `real` figure into a scratch note; Task 12 compares against it.

- [ ] **Step 2: Write the failing test**

Create `apps/web/src/test/expectNoCall.test.ts`:

```ts
import { describe, it, expect, vi } from 'vitest';
import { expectNoCall, expectNoCallWith, flushPending } from './expectNoCall';

// This file is exempted from the no-restricted-syntax rule in eslint.config.js
// precisely so it can contain the bare form below: the contrast between the
// bare assertion and the helper IS the thing under test.

describe('flushPending', () => {
  it('drains microtasks and one macrotask tick, in that order', async () => {
    const order: string[] = [];
    void Promise.resolve().then(() => order.push('microtask'));
    setTimeout(() => order.push('macrotask'), 0);

    await flushPending();

    expect(order).toEqual(['microtask', 'macrotask']);
  });
});

describe('expectNoCall', () => {
  // The whole bug class in four lines: React Query dispatches both queries and
  // mutations from a promise continuation, so a synchronous read sees zero.
  it('catches a promise-continuation call that a bare synchronous assertion misses', async () => {
    const spy = vi.fn();
    void Promise.resolve().then(() => spy());

    // Vacuously green — this is the form the lint rule exists to ban.
    expect(spy).not.toHaveBeenCalled();

    await expect(expectNoCall(spy, 'deferredSpy')).rejects.toThrow(/deferredSpy/);
  });

  it('passes when the spy is genuinely never called', async () => {
    const spy = vi.fn();
    await expectNoCall(spy, 'unusedSpy');
  });

  it('names the spy and reports the call count in the failure', async () => {
    const spy = vi.fn();
    spy();
    spy();
    spy();

    await expect(expectNoCall(spy, 'mediaService.getContentBlob')).rejects.toThrow(
      /mediaService\.getContentBlob.*3 times/s,
    );
  });

  it('works with no label', async () => {
    const spy = vi.fn();
    await expectNoCall(spy);
  });
});

describe('expectNoCallWith', () => {
  it('catches a deferred call carrying the banned arguments', async () => {
    const spy = vi.fn();
    void Promise.resolve().then(() => spy('banned'));

    // Vacuously green for the same reason.
    expect(spy).not.toHaveBeenCalledWith('banned');

    await expect(expectNoCallWith(spy, ['banned'], 'onChange')).rejects.toThrow(/onChange/);
  });

  it('passes when the spy was called with different arguments', async () => {
    const spy = vi.fn();
    void Promise.resolve().then(() => spy('allowed'));

    await expectNoCallWith(spy, ['banned'], 'onChange');
  });
});
```

- [ ] **Step 3: Run it to confirm it fails for the right reason**

```bash
cd apps/web && npx vitest run src/test/expectNoCall.test.ts
```

Expected: FAIL — `Failed to resolve import "./expectNoCall"`. If it fails for any other reason, stop and read the error.

- [ ] **Step 4: Write the helper**

Create `apps/web/src/test/expectNoCall.ts`:

```ts
// `act` MUST come from @testing-library/react, not from react. RTL sets
// IS_REACT_ACT_ENVIRONMENT = true at import time (not at first render), which
// is what lets this helper run inside pure-logic test files with no React root
// without emitting an act-scope warning. React's own `act` does not set the
// flag and would warn. See task-019 design F-1.
import { act } from '@testing-library/react';
import { expect } from 'vitest';
import type { MockInstance } from 'vitest';

/**
 * Drains the microtask queue and one macrotask tick.
 *
 * React Query dispatches both queries and mutations from a promise
 * continuation, so an assertion placed before this flush reads a spy that
 * could not yet have been called — it passes whether or not the guard it
 * exists to prove actually works. See issue #22.
 *
 * NOT compatible with vi.useFakeTimers(): the setTimeout below never fires
 * under fake timers and the await would hang until the test times out.
 */
export async function flushPending(): Promise<void> {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

/**
 * Flushes pending work, then asserts the spy was never called.
 *
 * `label` names the spy in the failure message — Vitest reports an unnamed
 * vi.fn() as the literal string "spy", which tells a reader nothing about
 * which of four toast variants fired.
 */
export async function expectNoCall(spy: MockInstance, label?: string): Promise<void> {
  await flushPending();
  if (label) spy.mockName(label);
  expect(spy).not.toHaveBeenCalled();
}

/**
 * Flushes pending work, then asserts the spy was never called with `args`.
 *
 * Note this assertion is also satisfied when the spy was never called at all,
 * which a flush cannot fix. Where the intent is "called, but not with X", pair
 * this with a positive assertion at the call site.
 */
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

- [ ] **Step 5: Run the test to verify it passes**

```bash
cd apps/web && npx vitest run src/test/expectNoCall.test.ts
```

Expected: PASS, 7 tests. Expect **no** `act(...) is not supported in production builds` or `not wrapped in act(...)` warnings in the output — their absence is the runtime confirmation of design F-1.

- [ ] **Step 6: Verify the types compile**

```bash
cd apps/web && npx tsc -b
```

Expected: clean. If `MockInstance` (default generic) rejects a concrete mock in a later task, the documented widening is `MockInstance<(...args: never[]) => unknown>` — apply it here, in the helper, and re-run this step. **Do not reach for `any`.**

- [ ] **Step 7: Format and commit**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-019-vacuous-negative-assertions
npx prettier --write apps/web/src/test/expectNoCall.ts apps/web/src/test/expectNoCall.test.ts
git add apps/web/src/test/expectNoCall.ts apps/web/src/test/expectNoCall.test.ts
git commit -m "test(task-019): add expectNoCall flush helper with falsifiability test"
```

---

## Task 2: The lint rule, and the worklist it produces

**Files:**
- Modify: `apps/web/eslint.config.js` (the test-files block at the end, currently lines 40-46)
- Create: `docs/tasks/task-019-vacuous-negative-assertions/probe-results.md`

**Interfaces:**
- Consumes: `apps/web/src/test/expectNoCall.ts` from Task 1 (the exemption block names it).
- Produces: `probe-results.md` with the header and empty table that every later task appends rows to.

- [ ] **Step 1: Add the rule and the exemption**

In `apps/web/eslint.config.js`, replace the final block:

```js
  {
    // Test files run under Vitest globals (describe/it/expect, etc.).
    files: ['**/*.{test,spec}.{ts,tsx}', 'src/test/**/*.{ts,tsx}'],
    languageOptions: {
      globals: { ...globals.node },
    },
  },
```

with:

```js
  {
    // Test files run under Vitest globals (describe/it/expect, etc.).
    files: ['**/*.{test,spec}.{ts,tsx}', 'src/test/**/*.{ts,tsx}'],
    languageOptions: {
      globals: { ...globals.node },
    },
    rules: {
      // A bare negative call assertion runs BEFORE any promise-continuation
      // dispatch, so it passes whether or not the guard it covers works.
      // See docs/tasks/task-019-vacuous-negative-assertions/ and issue #22.
      'no-restricted-syntax': [
        'error',
        {
          selector:
            "CallExpression[callee.object.property.name='not']" +
            '[callee.property.name=/^toHaveBeenCalled/]',
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
    // Must come AFTER the block above, which also matches src/test/**.
    files: ['src/test/expectNoCall.ts', 'src/test/expectNoCall.test.ts'],
    rules: { 'no-restricted-syntax': 'off' },
  },
```

- [ ] **Step 2: Confirm the rule fires, and that the exemption holds**

```bash
cd apps/web && npx eslint src 2>&1 | tail -20
```

Expected: 40 `no-restricted-syntax` errors. Confirm **zero** of them are in `src/test/expectNoCall.ts` or `src/test/expectNoCall.test.ts`:

```bash
cd apps/web && npx eslint src -f json 2>/dev/null \
  | python3 -c "import sys,json;d=json.load(sys.stdin);
print('total', sum(len(f['messages']) for f in d));
print('in helper', sum(len(f['messages']) for f in d if 'expectNoCall' in f['filePath']))"
```

Expected: `total 40`, `in helper 0`. If `in helper` is non-zero, the exemption block is in the wrong position — it must come after the test-files block.

- [ ] **Step 3: Capture the worklist**

```bash
cd apps/web && npx eslint src -f compact 2>/dev/null \
  | grep no-restricted-syntax > /tmp/task-019-worklist.txt
wc -l /tmp/task-019-worklist.txt
```

Expected: 40 lines. This is the un-migrated set; it shrinks by exactly the number of sites each later task migrates.

- [ ] **Step 4: Confirm the helper's own tests still pass under the new config**

```bash
cd apps/web && npx vitest run src/test/expectNoCall.test.ts && npx eslint src/test/expectNoCall.ts src/test/expectNoCall.test.ts
```

Expected: tests PASS, eslint clean.

- [ ] **Step 5: Create the record skeleton**

Create `docs/tasks/task-019-vacuous-negative-assertions/probe-results.md`:

```markdown
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
```

- [ ] **Step 6: Commit**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-019-vacuous-negative-assertions
npx prettier --write apps/web/eslint.config.js
git add apps/web/eslint.config.js docs/tasks/task-019-vacuous-negative-assertions/probe-results.md
git commit -m "test(task-019): ban bare negative call assertions in test files

The rule is red across all 40 existing sites on purpose — that red list is
the migration worklist. It goes green in the task that finishes Group D."
```

---

## Task 3: Group E — the two already-resolved sites

**Files:**
- Modify: `apps/web/src/pages/LoginPage.test.tsx` (site 39)
- Modify: `apps/web/src/lib/hooks/api/auth.test.ts` (site 40)
- Modify: `docs/tasks/task-019-vacuous-negative-assertions/probe-results.md`
- Probe only, reverted: `apps/web/src/lib/hooks/api/auth.ts`, `apps/web/src/pages/LoginPage.tsx`

**Interfaces:**
- Consumes: `expectNoCall` from Task 1.
- Produces: the first two rows of `probe-results.md`; a validated end-to-end migration against a site whose correct behaviour is already established.

LoginPage is migrated first deliberately: it already carries the proven flush (added in #20), so it is the one site where "the helper preserves correct behaviour" can be checked against a known-good reference.

**Probe table:**

| # | Spy | Stage 2 guard to defeat |
|---|---|---|
| 39 | `fetchSpy` | Two guards, both required (this is the FR-FIX-2 case). (a) In `apps/web/src/lib/hooks/api/auth.ts`, `updateThemePreference` returns `null` early when there is no access token — delete that early return. (b) The signed-out login page renders a theme toggle that does not carry the mutation; point it at the mutation-bearing `ThemeToggle` in `LoginPage.tsx`. |
| 40 | `fetchMock` | Same guard (a) as above: the no-token early return in `updateThemePreference`. This site's subject **is** the short-circuit, and it `await`s `updateThemePreference()` directly rather than going through a mutation — issue #22 records it as not affected. Confirm that still holds; do not "fix" it. |

- [ ] **Step 1: Stage-1 probe both sites**

In `LoginPage.test.tsx`, inside `it('cycles the theme without issuing a request')`, immediately before `expect(fetchSpy).not.toHaveBeenCalled();`:

```ts
void Promise.resolve().then(() => (fetchSpy as unknown as import('vitest').Mock)('__probe__'));
```

```bash
cd apps/web && npx vitest run src/pages/LoginPage.test.tsx
```

Expected: **RED** — the site already flushes, so the synthetic microtask is observed. Record `S1 = red`.

Repeat for `auth.test.ts`, inserting before `expect(fetchMock).not.toHaveBeenCalled();`:

```ts
void Promise.resolve().then(() => (fetchMock as unknown as import('vitest').Mock)('__probe__'));
```

```bash
cd apps/web && npx vitest run src/lib/hooks/api/auth.test.ts
```

Record the verdict. Remove both probe lines.

- [ ] **Step 2: Stage-2 probe site 40 (auth.test.ts)**

In `apps/web/src/lib/hooks/api/auth.ts`, delete the no-token early return inside `updateThemePreference` so the function proceeds to `fetch`.

```bash
cd apps/web && npx vitest run src/lib/hooks/api/auth.test.ts
```

Expected: **RED** — the test `await`s the function directly, so the request lands before the assertion. Record `S2 = red`, verdict **falsifiable**.

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-019-vacuous-negative-assertions
git checkout -- apps/web/src/lib/hooks/api/auth.ts
```

- [ ] **Step 3: Stage-2 probe site 39 (LoginPage.test.tsx)**

Apply guard (a) again, **plus** guard (b): edit `apps/web/src/pages/LoginPage.tsx` so the signed-out page renders the mutation-bearing `ThemeToggle`.

```bash
cd apps/web && npx vitest run src/pages/LoginPage.test.tsx
```

Expected: **RED**, reporting three calls (the test cycles the toggle three times). This reproduces the #20 finding. Record `S2 = red`, verdict **falsifiable**.

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-019-vacuous-negative-assertions
git checkout -- apps/web/src/lib/hooks/api/auth.ts apps/web/src/pages/LoginPage.tsx
git diff --stat   # must be empty for production files
```

- [ ] **Step 4: Migrate site 39**

In `LoginPage.test.tsx`, add the import:

```ts
import { expectNoCall } from '../test/expectNoCall';
```

Replace the hand-rolled flush **and** the assertion — the whole block from `// Flush microtasks before asserting.` through `expect(fetchSpy).not.toHaveBeenCalled();` — with:

```ts
    // The helper's flush is what makes this spy able to fail at all: react-query's
    // mutate() dispatches its mutationFn in a promise continuation, so a bare
    // assertion right after the last act() ran BEFORE any request could be made.
    // Together with the seeded token above, that is the pair of fixes from #20.
    // Verified by probe: pointing the page at the mutation-bearing ThemeToggle
    // makes this line report 3 calls to /api/auth/me.
    await expectNoCall(fetchSpy, 'fetch');
```

The hand-rolled `await act(async () => { await new Promise(...) })` block is removed — it is now inside the helper, and leaving both would be the per-call-site drift the helper exists to prevent.

- [ ] **Step 5: Migrate site 40**

In `auth.test.ts`, add the import:

```ts
import { expectNoCall } from '../../../test/expectNoCall';
```

Replace:

```ts
    expect(fetchMock).not.toHaveBeenCalled();
```

with:

```ts
    await expectNoCall(fetchMock, 'fetch');
```

Per FR-HELPER-3 this site migrates even though it was already falsifiable — uniformity is the point, and its falsifiability rests on an incidental `await` two lines up.

- [ ] **Step 6: Run both files and lint them**

```bash
cd apps/web && npx vitest run src/pages/LoginPage.test.tsx src/lib/hooks/api/auth.test.ts \
  && npx eslint src/pages/LoginPage.test.tsx src/lib/hooks/api/auth.test.ts
```

Expected: tests PASS, eslint clean on these two files.

- [ ] **Step 7: Re-probe both migrated sites**

Re-apply the Stage-1 probe to each file, run, confirm **RED**, remove. Then re-apply the Stage-2 guards, run, confirm **RED**, revert. Record `Re-probe = red` for both.

- [ ] **Step 8: Record and commit**

Append two rows to the `probe-results.md` table with the guard, both verdicts, the fix (`migrated to expectNoCall`), and the re-probe verdict.

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-019-vacuous-negative-assertions
git diff --stat -- apps/web/src/pages/LoginPage.tsx apps/web/src/lib/hooks/api/auth.ts   # must be empty
grep -rn "__probe__" apps/web/src   # must return nothing
npx prettier --write apps/web/src/pages/LoginPage.test.tsx apps/web/src/lib/hooks/api/auth.test.ts
git add apps/web/src/pages/LoginPage.test.tsx apps/web/src/lib/hooks/api/auth.test.ts \
  docs/tasks/task-019-vacuous-negative-assertions/probe-results.md
git commit -m "test(task-019): migrate Group E sites to expectNoCall"
```

---

## Task 4: Vehicle photo sites (7 sites, 2 files)

**Files:**
- Modify: `apps/web/src/components/features/vehicles/VehiclePhotoThumbnail.test.tsx` (sites 2, 3, 24, 25, 26, 27)
- Modify: `apps/web/src/components/features/vehicles/VehicleCard.test.tsx` (site 1)
- Modify: `docs/tasks/task-019-vacuous-negative-assertions/probe-results.md`
- Probe only, reverted: `apps/web/src/components/features/vehicles/VehiclePhotoThumbnail.tsx`, `apps/web/src/lib/hooks/api/media.ts`

**Interfaces:**
- Consumes: `expectNoCall` from Task 1, imported as `'../../../test/expectNoCall'` in both files.
- Produces: 7 rows in `probe-results.md`, including the task's first expected FR-TRIAGE-4 site.

**Probe table:**

| # | Spy | Stage 2 guard to defeat |
|---|---|---|
| 2 | `mediaService.getContentBlob` | `VehiclePhotoThumbnail.tsx` — the `if (!mediaId)` early return that renders `PhotoPlaceholder label="No photo"`. Delete it, **and** relax `enabled: !!id` to `enabled: true` in `useMediaContentUrl` (`src/lib/hooks/api/media.ts`), since the query is guarded twice. |
| 1 | `mediaService.getContentBlob` | Same pair — `VehicleCard` renders `VehiclePhotoThumbnail` with no `mediaId`. |
| 3 | `mediaService.getContentBlob` | **Expected unconstructible (FR-TRIAGE-4).** The guard here is React Query pausing the query while `onlineManager` reports offline — library behaviour, not app code. No edit to `apps/web/src` makes the fetch fire while offline. Attempt it; if no production edit produces red, record `S2 = n/a` and let Stage 1 govern, with the reasoning in the FR-TRIAGE-4 section. |
| 24-27 | `toast.error`, `toast`, `toast.warning`, `toast.info` | The component calls no toast at all — the probe is to **add** one. In `VehiclePhotoThumbnail.tsx`, add `toast.error('probe')` (with the `sonner` import) to the `if (isError || !url)` branch. The test's own comment records that this is exactly what previously kept every test green. |

- [ ] **Step 1: Stage-1 probe all 7 sites**

For each site, insert before the assertion and run the file:

```ts
void Promise.resolve().then(() => (SPY as unknown as import('vitest').Mock)('__probe__'));
```

Use `vi.mocked(mediaService.getContentBlob)` as `SPY` for sites 1, 2, 3; `toast.error` / `toast` / `toast.warning` / `toast.info` for 24-27.

```bash
cd apps/web && npx vitest run src/components/features/vehicles/VehiclePhotoThumbnail.test.tsx
cd apps/web && npx vitest run src/components/features/vehicles/VehicleCard.test.tsx
```

Sites 1 and 2 sit in synchronous `it(...)` callbacks with no preceding `await` — expect **green** (Mode T vacuous). Sites 3 and 24-27 follow `await screen.find*` — expect **red**. Record each. Remove every probe line.

- [ ] **Step 2: Stage-2 probe each site**

Apply each guard from the probe table, run the file, record red/green/unconstructible, then revert:

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-019-vacuous-negative-assertions
git checkout -- apps/web/src/components/features/vehicles/VehiclePhotoThumbnail.tsx apps/web/src/lib/hooks/api/media.ts
```

Run `git status --short apps/web/src` after each revert; it must show no production files.

- [ ] **Step 3: Migrate `VehiclePhotoThumbnail.test.tsx`**

Add the import after the existing `../../../test/*` imports:

```ts
import { expectNoCall } from '../../../test/expectNoCall';
```

In `it('shows the "No photo" placeholder when there is no media id, and fetches nothing', () => {`, change the callback to `async () => {` and replace:

```ts
    expect(mediaService.getContentBlob).not.toHaveBeenCalled();
```

with:

```ts
    await expectNoCall(vi.mocked(mediaService.getContentBlob), 'mediaService.getContentBlob');
```

In `it('fires no toast when a thumbnail fails to load', ...)` replace the four assertions with:

```ts
    await expectNoCall(vi.mocked(toast.error), 'toast.error');
    await expectNoCall(vi.mocked(toast), 'toast');
    await expectNoCall(vi.mocked(toast.warning), 'toast.warning');
    await expectNoCall(vi.mocked(toast.info), 'toast.info');
```

In `it('says "Photo unavailable", not "No photo", when a known photo cannot be fetched', ...)` replace:

```ts
    expect(mediaService.getContentBlob).not.toHaveBeenCalled();
```

with:

```ts
    await expectNoCall(vi.mocked(mediaService.getContentBlob), 'mediaService.getContentBlob');
```

- [ ] **Step 4: Migrate `VehicleCard.test.tsx`**

Add `import { expectNoCall } from '../../../test/expectNoCall';`. In `it('renders the placeholder at identical dimensions when no photo is set', () => {`, change the callback to `async () => {` and replace the assertion with:

```ts
    await expectNoCall(vi.mocked(mediaService.getContentBlob), 'mediaService.getContentBlob');
```

- [ ] **Step 5: Run both files and lint them**

```bash
cd apps/web && npx vitest run \
  src/components/features/vehicles/VehiclePhotoThumbnail.test.tsx \
  src/components/features/vehicles/VehicleCard.test.tsx \
  && npx eslint src/components/features/vehicles/VehiclePhotoThumbnail.test.tsx \
     src/components/features/vehicles/VehicleCard.test.tsx
```

Expected: tests PASS, eslint clean on these two files.

- [ ] **Step 6: Re-probe every site that was vacuous**

Sites 1 and 2 are expected to have been Mode T vacuous. Re-apply **both** probe stages to each and confirm **RED**, then revert. Record `Re-probe = red`.

- [ ] **Step 7: Record and commit**

Append 7 rows. Record site 3 under the FR-TRIAGE-4 section if Stage 2 proved unconstructible, with its disposition (Stage 1 was red, so the assertion can fail on a timing regression even though the offline pause itself is not defeatable from app code — it stays as a migrated `expectNoCall`).

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-019-vacuous-negative-assertions
git status --short apps/web/src | grep -v '\.test\.' || echo "clean: no production files modified"
grep -rn "__probe__" apps/web/src || echo "clean: no probe scaffolding"
npx prettier --write apps/web/src/components/features/vehicles/VehiclePhotoThumbnail.test.tsx apps/web/src/components/features/vehicles/VehicleCard.test.tsx
git add apps/web/src/components/features/vehicles/VehiclePhotoThumbnail.test.tsx \
  apps/web/src/components/features/vehicles/VehicleCard.test.tsx \
  docs/tasks/task-019-vacuous-negative-assertions/probe-results.md
git commit -m "test(task-019): migrate vehicle photo negative assertions to expectNoCall"
```

---

## Task 5: MemberList (5 sites, 1 file)

**Files:**
- Modify: `apps/web/src/components/features/settings/MemberList.test.tsx` (sites 12-16)
- Modify: `docs/tasks/task-019-vacuous-negative-assertions/probe-results.md`
- Probe only, reverted: `apps/web/src/components/features/settings/MemberList.tsx`

**Interfaces:**
- Consumes: `expectNoCall` from Task 1, imported as `'../../../test/expectNoCall'`.
- Produces: 5 rows in `probe-results.md`.

**Probe table** — all five guards live in `apps/web/src/components/features/settings/MemberList.tsx`:

| # | Spy | Stage 2 guard to defeat |
|---|---|---|
| 12 | `memberService.removeMember` | The row's remove button sets `pending` to open the `AlertDialog`. Change it to call `void confirmRemove(userId)` directly, bypassing the confirmation. |
| 13 | `memberService.removeMember` | `AlertDialogCancel` / the `onOpenChange` close path runs `closeDialog()`. Change `closeDialog` to also call `void confirmRemove(...)` for the pending row. |
| 14 | `memberService.updateRole` | Same shape as 12, on the promote action: call `void confirmPromote(userId)` from the row control instead of opening the dialog. |
| 15 | `memberService.updateRole` | The leave flow's plain confirmation: make the leave control call `void confirmLeave()` directly. |
| 16 | `memberService.removeMember` | In `confirmLeave`, the successor `updateRole.mutateAsync` rejection is what stops the subsequent `removeMember.mutateAsync({ userId: user.id, isSelf: true })`. Wrap the promote in a `.catch(() => undefined)` so the removal proceeds anyway. |

- [ ] **Step 1: Stage-1 probe all 5 sites**

Insert before each assertion, run, record, remove:

```ts
void Promise.resolve().then(() =>
  (vi.mocked(memberService.removeMember) as unknown as import('vitest').Mock)('__probe__'),
);
```

(substituting `memberService.updateRole` for sites 14 and 15).

```bash
cd apps/web && npx vitest run src/components/features/settings/MemberList.test.tsx
```

All five follow `await user.click(...)`, which yields to the event loop, so expect **red**. Confirm rather than assume.

- [ ] **Step 2: Stage-2 probe all 5 sites**

Apply each guard from the table, run the file, record, revert:

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-019-vacuous-negative-assertions
git checkout -- apps/web/src/components/features/settings/MemberList.tsx
```

- [ ] **Step 3: Migrate all 5 sites**

Add the import:

```ts
import { expectNoCall } from '../../../test/expectNoCall';
```

Replace each of the five assertions:

```ts
// expect(memberService.removeMember).not.toHaveBeenCalled();
await expectNoCall(vi.mocked(memberService.removeMember), 'memberService.removeMember');

// expect(memberService.updateRole).not.toHaveBeenCalled();
await expectNoCall(vi.mocked(memberService.updateRole), 'memberService.updateRole');
```

All five `it(...)` callbacks are already `async` — no signature changes.

- [ ] **Step 4: Run and lint**

```bash
cd apps/web && npx vitest run src/components/features/settings/MemberList.test.tsx \
  && npx eslint src/components/features/settings/MemberList.test.tsx
```

Expected: tests PASS, eslint clean.

- [ ] **Step 5: Re-probe any site that was vacuous**

For every row recorded green/green in Step 1-2, re-apply both stages and confirm **RED**.

- [ ] **Step 6: Record and commit**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-019-vacuous-negative-assertions
git status --short apps/web/src | grep -v '\.test\.' || echo "clean"
grep -rn "__probe__" apps/web/src || echo "clean"
npx prettier --write apps/web/src/components/features/settings/MemberList.test.tsx
git add apps/web/src/components/features/settings/MemberList.test.tsx \
  docs/tasks/task-019-vacuous-negative-assertions/probe-results.md
git commit -m "test(task-019): migrate MemberList negative assertions to expectNoCall"
```

---

## Task 6: Media upload and pending attachments (6 sites, 2 files)

**Files:**
- Modify: `apps/web/src/lib/hooks/api/media.test.ts` (sites 4, 5, 6, 28)
- Modify: `apps/web/src/lib/hooks/usePendingAttachments.test.ts` (sites 7, 8)
- Modify: `docs/tasks/task-019-vacuous-negative-assertions/probe-results.md`
- Probe only, reverted: `apps/web/src/lib/hooks/api/media.ts`, `apps/web/src/lib/hooks/usePendingAttachments.ts`

**Interfaces:**
- Consumes: `expectNoCall` and `expectNoCallWith` from Task 1. Import as `'../../../test/expectNoCall'` in `media.test.ts` and `'../../test/expectNoCall'` in `usePendingAttachments.test.ts`.
- Produces: 6 rows in `probe-results.md`, including the first `expectNoCallWith` migration.

**Probe table:**

| # | Spy | Stage 2 guard to defeat |
|---|---|---|
| 4, 5, 6 | `deps.initUpload`, `deps.putContent`, `deps.confirm` | `apps/web/src/lib/hooks/api/media.ts`, in `performMediaUpload`: the `if (file.size > MEDIA_MAX_UPLOAD_BYTES)` early throw. Delete it so an oversized file proceeds to the three requests. |
| 7 | `mediaService.remove` | `apps/web/src/lib/hooks/usePendingAttachments.ts`, in `remove`: the `if (target?.mediaId)` condition. Change to call `void mediaService.remove(target?.mediaId ?? 'probe')` unconditionally. |
| 8 | `mediaService.remove` | Same file, the unmount `useEffect`: the `if (committedRef.current) { return; }` disarm. Delete that early return so a committed set is deleted on unmount anyway. |
| 28 | `revokeObjectURL` (**With** `secondUrl`) | `apps/web/src/lib/hooks/api/media.ts`, `useMediaContentUrl`'s effect cleanup `return () => URL.revokeObjectURL(objectUrl)`. Change it to revoke the current `entry?.url` instead of the captured `objectUrl`, so the new URL is revoked too. |

Note on site 28: `not.toHaveBeenCalledWith` also passes when the spy was never called at all. This site is already protected — the two lines above it pin `toHaveBeenCalledWith(firstUrl)` and `toHaveBeenCalledTimes(1)`. Leave both in place.

- [ ] **Step 1: Stage-1 probe all 6 sites**

For sites 4-6, insert before the first of the three assertions:

```ts
void Promise.resolve().then(() => (deps.initUpload as unknown as import('vitest').Mock)('__probe__'));
```

(and the analogous line per spy). For site 28 the synthetic call **must carry the banned argument**:

```ts
void Promise.resolve().then(() =>
  (revokeObjectURL as unknown as import('vitest').Mock)(secondUrl),
);
```

For sites 7-8:

```ts
void Promise.resolve().then(() =>
  (vi.mocked(mediaService.remove) as unknown as import('vitest').Mock)('__probe__'),
);
```

```bash
cd apps/web && npx vitest run src/lib/hooks/api/media.test.ts src/lib/hooks/usePendingAttachments.test.ts
```

Sites 4-6 follow an `await expect(...).rejects.toMatchObject(...)`, which already yields — the PRD flags them as possibly falsifiable as written. Sites 7-8 follow a synchronous `act(() => ...)` / `unmount()`. Record each verdict; the point of this step is that inspection does not settle it.

- [ ] **Step 2: Stage-2 probe all 6 sites**

Apply each guard, run, record, revert:

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-019-vacuous-negative-assertions
git checkout -- apps/web/src/lib/hooks/api/media.ts apps/web/src/lib/hooks/usePendingAttachments.ts
```

- [ ] **Step 3: Migrate `media.test.ts`**

Add:

```ts
import { expectNoCall, expectNoCallWith } from '../../../test/expectNoCall';
```

Replace the three oversized-file assertions:

```ts
    await expectNoCall(deps.initUpload, 'deps.initUpload');
    await expectNoCall(deps.putContent, 'deps.putContent');
    await expectNoCall(deps.confirm, 'deps.confirm');
```

(`deps.initUpload` etc. are local `vi.fn()`s — no `vi.mocked()` wrapper needed.)

Replace the `With` site:

```ts
    // expect(revokeObjectURL).not.toHaveBeenCalledWith(secondUrl);
    await expectNoCallWith(revokeObjectURL, [secondUrl], 'URL.revokeObjectURL');
```

Leave the two positive assertions above it untouched — they are what stops this assertion passing by never being called.

- [ ] **Step 4: Migrate `usePendingAttachments.test.ts`**

Add:

```ts
import { expectNoCall } from '../../test/expectNoCall';
```

Replace both assertions:

```ts
    await expectNoCall(vi.mocked(mediaService.remove), 'mediaService.remove');
```

- [ ] **Step 5: Run and lint**

```bash
cd apps/web && npx vitest run src/lib/hooks/api/media.test.ts src/lib/hooks/usePendingAttachments.test.ts \
  && npx eslint src/lib/hooks/api/media.test.ts src/lib/hooks/usePendingAttachments.test.ts
```

Expected: tests PASS, eslint clean. Watch specifically for `not wrapped in act(...)` warnings — `media.test.ts` has non-React cases, and their absence confirms design F-1 holds outside a mounted root.

- [ ] **Step 6: Re-probe every vacuous site**

Both stages, confirm **RED**, revert.

- [ ] **Step 7: Record and commit**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-019-vacuous-negative-assertions
git status --short apps/web/src | grep -v '\.test\.' || echo "clean"
grep -rn "__probe__" apps/web/src || echo "clean"
npx prettier --write apps/web/src/lib/hooks/api/media.test.ts apps/web/src/lib/hooks/usePendingAttachments.test.ts
git add apps/web/src/lib/hooks/api/media.test.ts apps/web/src/lib/hooks/usePendingAttachments.test.ts \
  docs/tasks/task-019-vacuous-negative-assertions/probe-results.md
git commit -m "test(task-019): migrate media and attachment negative assertions to expectNoCall"
```

---

## Task 7: Vehicle interaction components (7 sites, 4 files)

**Files:**
- Modify: `apps/web/src/components/features/vehicles/CategoryCombobox.test.tsx` (sites 17, 18)
- Modify: `apps/web/src/components/features/vehicles/dialogs/PhotoGalleryDialog.test.tsx` (sites 20, 21, 22)
- Modify: `apps/web/src/components/features/vehicles/detail/VehicleRecordsTable.test.tsx` (site 19)
- Modify: `apps/web/src/components/features/vehicles/maintenance/MaintenanceRecordForm.test.tsx` (site 23)
- Modify: `docs/tasks/task-019-vacuous-negative-assertions/probe-results.md`
- Probe only, reverted: `apps/web/src/components/features/vehicles/CategoryCombobox.tsx`, `.../dialogs/PhotoGalleryDialog.tsx`, `.../detail/VehicleRecordsTable.tsx`, `apps/web/src/lib/schemas/maintenanceRecord.ts`

**Interfaces:**
- Consumes: `expectNoCall`, `expectNoCallWith` from Task 1. Import as `'../../../test/expectNoCall'` in `CategoryCombobox.test.tsx`; `'../../../../test/expectNoCall'` in the `dialogs/`, `detail/`, and `maintenance/` files.
- Produces: 7 rows in `probe-results.md`.

**Probe table:**

| # | Spy | Stage 2 guard to defeat |
|---|---|---|
| 17 | `onChange` (**With** `'Skid Plate'`) | `CategoryCombobox.tsx`, in `handleCreate`: change `onChange(created.id)` to `onChange(trimmed)` — i.e. select the locally-derived name instead of the server id. That is precisely the regression this assertion exists to catch. |
| 18 | `onChange` | `CategoryCombobox.tsx`, the `catch (err)` arm of `handleCreate`: add `onChange(trimmed)` inside it, so a failed creation still selects something. |
| 19 | `onLoadMore` | `VehicleRecordsTable.tsx`: remove `disabled={isFetchingNextPage}` from the load-more button. |
| 20 | `toast.error` | `PhotoGalleryDialog.tsx`, in `handleRemove`: the object-cleanup failure is swallowed. Move the cleanup call inside the `try` so its rejection reaches the `catch` that raises `toast.error`. |
| 21, 22 | `removeMedia`, `removeObject` | `PhotoGalleryDialog.tsx`: the tile's remove control sets `pendingRemoval` to open the `AlertDialog`. Change it to call `void handleRemove(mediaId)` directly, bypassing the confirmation. |
| 23 | `onSubmit` | `apps/web/src/lib/schemas/maintenanceRecord.ts`: relax `description`'s `.max(200, 'Description must be 200 characters or fewer')` to `.max(10000)` so the over-length value validates and the form submits. |

Site 17 note: `not.toHaveBeenCalledWith` is already paired with a positive `await waitFor(() => expect(onChange).toHaveBeenCalledWith('server-assigned-id'))` on the line above, which pins that `onChange` fired at all. (The design flagged this site as possibly lacking that pairing; it has it. Leave the positive assertion in place.)

- [ ] **Step 1: Stage-1 probe all 7 sites**

Insert before each assertion, run the file, record, remove. For site 17 the synthetic call must carry the banned argument:

```ts
void Promise.resolve().then(() => (onChange as unknown as import('vitest').Mock)('Skid Plate'));
```

For the others:

```ts
void Promise.resolve().then(() => (SPY as unknown as import('vitest').Mock)('__probe__'));
```

```bash
cd apps/web && npx vitest run \
  src/components/features/vehicles/CategoryCombobox.test.tsx \
  src/components/features/vehicles/dialogs/PhotoGalleryDialog.test.tsx \
  src/components/features/vehicles/detail/VehicleRecordsTable.test.tsx \
  src/components/features/vehicles/maintenance/MaintenanceRecordForm.test.tsx
```

- [ ] **Step 2: Stage-2 probe all 7 sites**

Apply each guard, run, record, revert:

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-019-vacuous-negative-assertions
git checkout -- apps/web/src/components/features/vehicles/CategoryCombobox.tsx \
  apps/web/src/components/features/vehicles/dialogs/PhotoGalleryDialog.tsx \
  apps/web/src/components/features/vehicles/detail/VehicleRecordsTable.tsx \
  apps/web/src/lib/schemas/maintenanceRecord.ts
```

- [ ] **Step 3: Migrate `CategoryCombobox.test.tsx`**

```ts
import { expectNoCall, expectNoCallWith } from '../../../test/expectNoCall';
```

```ts
// expect(onChange).not.toHaveBeenCalledWith('Skid Plate');
await expectNoCallWith(onChange, ['Skid Plate'], 'onChange');

// expect(onChange).not.toHaveBeenCalled();
await expectNoCall(onChange, 'onChange');
```

- [ ] **Step 4: Migrate `PhotoGalleryDialog.test.tsx`**

```ts
import { expectNoCall } from '../../../../test/expectNoCall';
```

```ts
await expectNoCall(vi.mocked(toast.error), 'toast.error');
await expectNoCall(removeMedia, 'removeMedia');
await expectNoCall(removeObject, 'removeObject');
```

If `removeMedia` / `removeObject` are local `vi.fn()`s they need no wrapper; if they are mocked module methods, wrap in `vi.mocked(...)` — check the file's `vi.mock` factory before editing.

- [ ] **Step 5: Migrate `VehicleRecordsTable.test.tsx` and `MaintenanceRecordForm.test.tsx`**

```ts
// VehicleRecordsTable.test.tsx
import { expectNoCall } from '../../../../test/expectNoCall';
await expectNoCall(onLoadMore, 'onLoadMore');
```

```ts
// MaintenanceRecordForm.test.tsx
import { expectNoCall } from '../../../../test/expectNoCall';
await expectNoCall(onSubmit, 'onSubmit');
```

- [ ] **Step 6: Run and lint**

```bash
cd apps/web && npx vitest run \
  src/components/features/vehicles/CategoryCombobox.test.tsx \
  src/components/features/vehicles/dialogs/PhotoGalleryDialog.test.tsx \
  src/components/features/vehicles/detail/VehicleRecordsTable.test.tsx \
  src/components/features/vehicles/maintenance/MaintenanceRecordForm.test.tsx \
  && npx eslint src/components/features/vehicles/CategoryCombobox.test.tsx \
     src/components/features/vehicles/dialogs/PhotoGalleryDialog.test.tsx \
     src/components/features/vehicles/detail/VehicleRecordsTable.test.tsx \
     src/components/features/vehicles/maintenance/MaintenanceRecordForm.test.tsx
```

Expected: tests PASS, eslint clean on these four files.

- [ ] **Step 7: Re-probe every vacuous site, then record and commit**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-019-vacuous-negative-assertions
git status --short apps/web/src | grep -v '\.test\.' || echo "clean"
grep -rn "__probe__" apps/web/src || echo "clean"
npx prettier --write apps/web/src/components/features/vehicles/CategoryCombobox.test.tsx apps/web/src/components/features/vehicles/dialogs/PhotoGalleryDialog.test.tsx apps/web/src/components/features/vehicles/detail/VehicleRecordsTable.test.tsx apps/web/src/components/features/vehicles/maintenance/MaintenanceRecordForm.test.tsx
git add apps/web/src/components/features/vehicles/CategoryCombobox.test.tsx \
  apps/web/src/components/features/vehicles/dialogs/PhotoGalleryDialog.test.tsx \
  apps/web/src/components/features/vehicles/detail/VehicleRecordsTable.test.tsx \
  apps/web/src/components/features/vehicles/maintenance/MaintenanceRecordForm.test.tsx \
  docs/tasks/task-019-vacuous-negative-assertions/probe-results.md
git commit -m "test(task-019): migrate vehicle interaction negative assertions to expectNoCall"
```

---

## Task 8: Hooks (6 sites, 4 files)

**Files:**
- Modify: `apps/web/src/components/features/dashboard/useDashboardWidgets.test.ts` (sites 10, 11)
- Modify: `apps/web/src/lib/hooks/api/members.test.ts` (sites 29, 30)
- Modify: `apps/web/src/lib/hooks/api/users.test.ts` (site 31)
- Modify: `apps/web/src/lib/hooks/api/vehicleRecords.test.ts` (site 32)
- Modify: `docs/tasks/task-019-vacuous-negative-assertions/probe-results.md`
- Probe only, reverted: `apps/web/src/components/features/dashboard/useDashboardWidgets.ts`, `apps/web/src/lib/hooks/api/members.ts`, `apps/web/src/lib/hooks/api/users.ts`, `apps/web/src/lib/hooks/api/vehicleRecords.ts`

**Interfaces:**
- Consumes: `expectNoCall` from Task 1. Import as `'../../../test/expectNoCall'` in all four files (`useDashboardWidgets.test.ts` is at `src/components/features/dashboard/`, the same depth).
- Produces: 6 rows in `probe-results.md`.

**Probe table:**

| # | Spy | Stage 2 guard to defeat |
|---|---|---|
| 10 | `dashboardService.saveLayout` | `useDashboardWidgets.ts`, in `addWidget`: the `if (isLoading) return;` early return. Delete it. |
| 11 | `dashboardService.saveLayout` | `useDashboardWidgets.ts`, the list-boundary guards: `if (idx === 0) return;` in `moveUp` and `if (idx === widgets.length - 1) return;` in `moveDown`. Delete both. |
| 29, 30 | `mintAccessToken` | `apps/web/src/lib/hooks/api/members.ts`: the `if (!isSelf) return;` early return in `useRemoveMember`'s `onSuccess`, and the equivalent guard in `useUpdateMemberRole`. Delete them so a token is minted for any member. |
| 31 | `userService.listByIds` | `apps/web/src/lib/hooks/api/users.ts`: change `enabled: sorted.length > 0` to `enabled: true`. |
| 32 | `fuel.fetchNextPage` | `apps/web/src/lib/hooks/api/vehicleRecords.ts`, in `loadMore`: drop the `if (fuel.hasNextPage)` condition so `fetchFuelNextPage()` runs unconditionally. |

- [ ] **Step 1: Stage-1 probe all 6 sites**

Insert before each assertion, run, record, remove:

```ts
void Promise.resolve().then(() => (SPY as unknown as import('vitest').Mock)('__probe__'));
```

```bash
cd apps/web && npx vitest run \
  src/components/features/dashboard/useDashboardWidgets.test.ts \
  src/lib/hooks/api/members.test.ts \
  src/lib/hooks/api/users.test.ts \
  src/lib/hooks/api/vehicleRecords.test.ts
```

Sites 31 and 32 sit in synchronous `it(...)` callbacks — expect **green**. Site 10 follows a synchronous `act(() => ...)` — expect **green**. Sites 29-30 follow `await waitFor(...)` — expect **red**. Confirm each.

- [ ] **Step 2: Stage-2 probe all 6 sites**

Apply each guard, run, record, revert:

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-019-vacuous-negative-assertions
git checkout -- apps/web/src/components/features/dashboard/useDashboardWidgets.ts \
  apps/web/src/lib/hooks/api/members.ts apps/web/src/lib/hooks/api/users.ts \
  apps/web/src/lib/hooks/api/vehicleRecords.ts
```

- [ ] **Step 3: Migrate `useDashboardWidgets.test.ts` and `members.test.ts`**

```ts
// useDashboardWidgets.test.ts
import { expectNoCall } from '../../../test/expectNoCall';
await expectNoCall(vi.mocked(dashboardService.saveLayout), 'dashboardService.saveLayout');
```

```ts
// members.test.ts
import { expectNoCall } from '../../../test/expectNoCall';
await expectNoCall(vi.mocked(mintAccessToken), 'mintAccessToken');
```

Leave the neighbouring `expect(calls).not.toContainEqual(...)` assertions in `members.test.ts` untouched — they are not call assertions and the lint rule does not match them.

- [ ] **Step 4: Migrate `users.test.ts` and `vehicleRecords.test.ts` (both need `async`)**

In `users.test.ts`, change `it('does not fire a request when there are no ids', () => {` to `async () => {` and:

```ts
import { expectNoCall } from '../../../test/expectNoCall';
await expectNoCall(vi.mocked(userService.listByIds), 'userService.listByIds');
```

Keep `expect(result.current.fetchStatus).toBe('idle');` on the following line — the query is disabled, so the flush does not change it.

In `vehicleRecords.test.ts`, change `it('loadMore calls fetchNextPage only on sources that still have a next page', () => {` to `async () => {` and:

```ts
import { expectNoCall } from '../../../test/expectNoCall';
await expectNoCall(fuel.fetchNextPage, 'fuel.fetchNextPage');
```

Keep the two `toHaveBeenCalledTimes(1)` assertions around it — they pin that `loadMore` actually fired, which is what stops this negative assertion passing for the wrong reason.

- [ ] **Step 5: Run and lint**

```bash
cd apps/web && npx vitest run \
  src/components/features/dashboard/useDashboardWidgets.test.ts \
  src/lib/hooks/api/members.test.ts \
  src/lib/hooks/api/users.test.ts \
  src/lib/hooks/api/vehicleRecords.test.ts \
  && npx eslint src/components/features/dashboard/useDashboardWidgets.test.ts \
     src/lib/hooks/api/members.test.ts src/lib/hooks/api/users.test.ts \
     src/lib/hooks/api/vehicleRecords.test.ts
```

Expected: tests PASS, eslint clean, and no act-scope warnings from these non-React-rendering files.

- [ ] **Step 6: Re-probe every vacuous site, then record and commit**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-019-vacuous-negative-assertions
git status --short apps/web/src | grep -v '\.test\.' || echo "clean"
grep -rn "__probe__" apps/web/src || echo "clean"
npx prettier --write apps/web/src/components/features/dashboard/useDashboardWidgets.test.ts apps/web/src/lib/hooks/api/members.test.ts apps/web/src/lib/hooks/api/users.test.ts apps/web/src/lib/hooks/api/vehicleRecords.test.ts
git add apps/web/src/components/features/dashboard/useDashboardWidgets.test.ts \
  apps/web/src/lib/hooks/api/members.test.ts apps/web/src/lib/hooks/api/users.test.ts \
  apps/web/src/lib/hooks/api/vehicleRecords.test.ts \
  docs/tasks/task-019-vacuous-negative-assertions/probe-results.md
git commit -m "test(task-019): migrate hook negative assertions to expectNoCall"
```

---

## Task 9: Pages and activity feed (5 sites, 3 files)

**Files:**
- Modify: `apps/web/src/pages/VehiclesPage.test.tsx` (sites 33, 34, 35)
- Modify: `apps/web/src/pages/admin/AdminFleetsPage.test.tsx` (site 36)
- Modify: `apps/web/src/components/features/activity/ActivityFeed.test.tsx` (site 9)
- Modify: `docs/tasks/task-019-vacuous-negative-assertions/probe-results.md`
- Probe only, reverted: `apps/web/src/lib/schemas/vehicle.ts`, `apps/web/src/pages/VehiclesPage.tsx`, `apps/web/src/pages/admin/AdminFleetsPage.tsx`, `apps/web/src/lib/hooks/api/users.ts`

**Interfaces:**
- Consumes: `expectNoCall` from Task 1. Import as `'../test/expectNoCall'` in `VehiclesPage.test.tsx`, `'../../test/expectNoCall'` in `AdminFleetsPage.test.tsx`, `'../../../test/expectNoCall'` in `ActivityFeed.test.tsx`.
- Produces: the final 5 migration rows in `probe-results.md`.

**Probe table:**

| # | Spy | Stage 2 guard to defeat |
|---|---|---|
| 33 | `vehicleService.createInFleet` | `apps/web/src/lib/schemas/vehicle.ts`: relax the required fields — `make: z.string().trim().min(1, 'Make is required')` → `z.string().trim()`, same for `model`, and drop the `year` number requirement. Blank fields then validate and the form submits. |
| 34 | `vehicleService.createInFleet` | Same schema relaxation is not enough here — this case's request already fails. The guard is the dialog's stay-open-on-error behaviour; defeat it by making the create handler in `VehiclesPage.tsx` swallow the error and close the dialog, then confirm whether the assertion notices the extra call. If no production edit can make a **second** `createInFleet` call happen, record as FR-TRIAGE-4 and let Stage 1 govern. |
| 35 | `vehicleService.createInFleet` | `VehiclesPage.tsx`: make the dialog's outside-pointer-down close path also invoke the create handler. |
| 36 | `createPurgeMutate` | `AdminFleetsPage.tsx`: the purge control calls `setConfirmOpen(true)`. Change it to call `createPurge.mutate(...)` directly, bypassing the confirmation. |
| 9 | `listByIds` | `apps/web/src/lib/hooks/api/users.ts`: change `enabled: sorted.length > 0` to `enabled: true`. `ActivityFeed` passes an empty id array for an empty event list, so this is the guard that stops the request. |

- [ ] **Step 1: Stage-1 probe all 5 sites**

Insert before each assertion, run, record, remove:

```ts
void Promise.resolve().then(() => (SPY as unknown as import('vitest').Mock)('__probe__'));
```

```bash
cd apps/web && npx vitest run src/pages/VehiclesPage.test.tsx \
  src/pages/admin/AdminFleetsPage.test.tsx \
  src/components/features/activity/ActivityFeed.test.tsx
```

- [ ] **Step 2: Stage-2 probe all 5 sites**

Apply each guard, run, record, revert:

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-019-vacuous-negative-assertions
git checkout -- apps/web/src/lib/schemas/vehicle.ts apps/web/src/pages/VehiclesPage.tsx \
  apps/web/src/pages/admin/AdminFleetsPage.tsx apps/web/src/lib/hooks/api/users.ts
```

- [ ] **Step 3: Migrate all three files**

```ts
// VehiclesPage.test.tsx
import { expectNoCall } from '../test/expectNoCall';
await expectNoCall(vi.mocked(vehicleService.createInFleet), 'vehicleService.createInFleet');
```

```ts
// AdminFleetsPage.test.tsx
import { expectNoCall } from '../../test/expectNoCall';
await expectNoCall(createPurgeMutate, 'createPurgeMutate');
```

```ts
// ActivityFeed.test.tsx
import { expectNoCall } from '../../../test/expectNoCall';
await expectNoCall(listByIds, 'userService.listByIds');
```

Wrap `createPurgeMutate` / `listByIds` in `vi.mocked(...)` only if they are mocked module methods rather than local `vi.fn()`s — check each file's `vi.mock` factory before editing.

All five `it(...)` callbacks are already `async`.

- [ ] **Step 4: Run and lint**

```bash
cd apps/web && npx vitest run src/pages/VehiclesPage.test.tsx \
  src/pages/admin/AdminFleetsPage.test.tsx \
  src/components/features/activity/ActivityFeed.test.tsx \
  && npx eslint src/pages/VehiclesPage.test.tsx src/pages/admin/AdminFleetsPage.test.tsx \
     src/components/features/activity/ActivityFeed.test.tsx
```

Expected: tests PASS, eslint clean on these three files.

- [ ] **Step 5: Confirm the worklist is down to the two Group D sites**

```bash
cd apps/web && npx eslint src -f compact 2>/dev/null | grep -c no-restricted-syntax
```

Expected: `2` — `input.test.tsx` and `download.test.ts`, both handled in Task 10. Any other number means a site was missed; diff against `/tmp/task-019-worklist.txt` from Task 2 to find it.

- [ ] **Step 6: Re-probe every vacuous site, then record and commit**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-019-vacuous-negative-assertions
git status --short apps/web/src | grep -v '\.test\.' || echo "clean"
grep -rn "__probe__" apps/web/src || echo "clean"
npx prettier --write apps/web/src/pages/VehiclesPage.test.tsx apps/web/src/pages/admin/AdminFleetsPage.test.tsx apps/web/src/components/features/activity/ActivityFeed.test.tsx
git add apps/web/src/pages/VehiclesPage.test.tsx apps/web/src/pages/admin/AdminFleetsPage.test.tsx \
  apps/web/src/components/features/activity/ActivityFeed.test.tsx \
  docs/tasks/task-019-vacuous-negative-assertions/probe-results.md
git commit -m "test(task-019): migrate page and activity negative assertions to expectNoCall"
```

---

## Task 10: Group D — the two exempted sites

**Files:**
- Modify: `apps/web/src/components/ui/input.test.tsx` (site 37)
- Modify: `apps/web/src/lib/utils/download.test.ts` (site 38)
- Modify: `docs/tasks/task-019-vacuous-negative-assertions/probe-results.md`
- Probe only, reverted: `apps/web/src/components/ui/input.tsx`, `apps/web/src/lib/utils/download.ts`

**Interfaces:**
- Consumes: nothing from Task 1 — these two sites are exempted from the helper (OQ-4) and import nothing new.
- Produces: the last 2 rows of `probe-results.md`; a green `npx eslint src`.

Per PRD FR-TRIAGE-6 both sites are probed even though the answer is expected. Per design §6 step 5 and OQ-4, a site confirmed synchronous-by-construction is exempted via an inline `eslint-disable-next-line` carrying its probe result — an `eslint-disable` with evidence attached is more honest than a flush that provably does nothing.

`download.test.ts` has a second, stronger reason: the file runs under `vi.useFakeTimers()`, so the helper's `setTimeout(0)` would never fire and the migrated test would hang until the Vitest timeout. Its assertion is also a **deliberate ordering assertion** — `vi.runAllTimers()` two lines below proves the revoke does happen — so flushing there would destroy what the test measures.

**Probe table:**

| # | Spy | Stage 2 guard to defeat |
|---|---|---|
| 37 | `showPicker` | `apps/web/src/components/ui/input.tsx`: force `const isPicker = true;` so a `type="number"` input also reaches `el.showPicker()`. |
| 38 | `URL.revokeObjectURL` | `apps/web/src/lib/utils/download.ts`: change `setTimeout(() => URL.revokeObjectURL(url), 0);` to a bare `URL.revokeObjectURL(url);` so the revoke happens synchronously, before the assertion. |

- [ ] **Step 1: Stage-1 probe both sites**

```ts
// input.test.tsx
void Promise.resolve().then(() => (showPicker as unknown as import('vitest').Mock)('__probe__'));
```

```ts
// download.test.ts — a microtask, NOT a timer: Promise.resolve() is unaffected
// by vi.useFakeTimers(), so this probe works here.
void Promise.resolve().then(() =>
  (URL.revokeObjectURL as unknown as import('vitest').Mock)('__probe__'),
);
```

```bash
cd apps/web && npx vitest run src/components/ui/input.test.tsx src/lib/utils/download.test.ts
```

Site 37 follows `await user.click(...)`, which yields — expect **red**. Site 38's assertion is synchronous in the same tick — expect **green**, and that green is *correct*: the assertion means "not called synchronously", which is the whole point. Record both.

Remove both probe lines.

- [ ] **Step 2: Stage-2 probe both sites**

Apply each guard, run, record **RED** for both, then revert:

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-019-vacuous-negative-assertions
git checkout -- apps/web/src/components/ui/input.tsx apps/web/src/lib/utils/download.ts
```

Both are expected red — the spy fires synchronously once its guard is gone, which is what "safe by construction" means. If either comes back **green**, it is not a Group D site: stop, migrate it to the helper as in Tasks 4-9, and record the correction.

- [ ] **Step 3: Confirm the fake-timer incompatibility, so the exemption is evidence-backed**

Temporarily migrate `download.test.ts:43` to `await expectNoCall(vi.mocked(URL.revokeObjectURL), 'URL.revokeObjectURL')` (making the `it` async) and run it:

```bash
cd apps/web && npx vitest run src/lib/utils/download.test.ts --testTimeout=5000
```

Expected: **timeout** — the helper's `setTimeout(0)` never fires under `vi.useFakeTimers()`. Capture the failure output for the record, then revert the file:

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-019-vacuous-negative-assertions
git checkout -- apps/web/src/lib/utils/download.test.ts
```

- [ ] **Step 4: Add the exemptions with their evidence**

In `apps/web/src/components/ui/input.test.tsx`, replace:

```ts
    expect(showPicker).not.toHaveBeenCalled();
```

with:

```ts
    // eslint-disable-next-line no-restricted-syntax -- task-019 probe: synchronous by
    // construction. `showPicker` is a direct DOM call from Input's onClick, not a
    // promise continuation; forcing `isPicker = true` in input.tsx turns this line
    // red with no flush, so expectNoCall's flush would be pure noise here.
    expect(showPicker).not.toHaveBeenCalled();
```

In `apps/web/src/lib/utils/download.test.ts`, replace:

```ts
    expect(URL.revokeObjectURL).not.toHaveBeenCalled();
```

with:

```ts
    // eslint-disable-next-line no-restricted-syntax -- task-019 probe: this is a
    // deliberate ORDERING assertion, not a "never happens" one — vi.runAllTimers()
    // two lines below proves the revoke does occur. Making download.ts revoke
    // synchronously turns this line red with no flush. expectNoCall is also
    // unusable here: the file runs under vi.useFakeTimers(), so the helper's
    // setTimeout(0) never fires and the test would hang until timeout.
    expect(URL.revokeObjectURL).not.toHaveBeenCalled();
```

- [ ] **Step 5: Verify the whole tree is lint-clean**

```bash
cd apps/web && npx eslint src
```

Expected: **no output, exit 0.** This is the first point on the branch where `npx eslint src` is green.

- [ ] **Step 6: Run the full web suite**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-019-vacuous-negative-assertions
time make fe-test
```

Expected: all suites PASS. Compare the wall time against the Task 1 baseline; a regression over 10% means the flush strategy is revisited **in the helper** (PRD §8).

- [ ] **Step 7: Record and commit**

Append the two Group D rows, each noting the probe result that justifies its exemption, plus the fake-timer finding for site 38.

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-019-vacuous-negative-assertions
git status --short apps/web/src | grep -v '\.test\.' || echo "clean"
grep -rn "__probe__" apps/web/src || echo "clean"
npx prettier --write apps/web/src/components/ui/input.test.tsx apps/web/src/lib/utils/download.test.ts
git add apps/web/src/components/ui/input.test.tsx apps/web/src/lib/utils/download.test.ts \
  docs/tasks/task-019-vacuous-negative-assertions/probe-results.md
git commit -m "test(task-019): exempt the two synchronous-by-construction sites with probe evidence"
```

---

## Task 11: Lint demonstration and the completed record

**Files:**
- Modify: `docs/tasks/task-019-vacuous-negative-assertions/probe-results.md`
- Temporarily modified then reverted: one test file of your choice (`apps/web/src/lib/hooks/api/users.test.ts` is suggested — small and fast)

**Interfaces:**
- Consumes: the lint rule from Task 2; the 40 recorded rows from Tasks 3-10.
- Produces: the finished record, satisfying FR-DOC-1, FR-DOC-2, FR-DOC-3.

- [ ] **Step 1: Demonstrate the bare form is rejected (FR-LINT-4)**

Add a bare assertion to `apps/web/src/lib/hooks/api/users.test.ts`, inside any existing test:

```ts
    expect(userService.listByIds).not.toHaveBeenCalled();
```

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-019-vacuous-negative-assertions
make lint-check 2>&1 | tee /tmp/task-019-lint-demo-1.txt | tail -25
```

Expected: FAIL, with the `no-restricted-syntax` message naming `expectNoCall` and issue #22. Revert:

```bash
git checkout -- apps/web/src/lib/hooks/api/users.test.ts
```

- [ ] **Step 2: Demonstrate the `toHaveBeenCalledTimes(0)` spelling is rejected (FR-LINT-2)**

This selector matches nothing on the current tree, so it needs its own demonstration. Add:

```ts
    expect(userService.listByIds).toHaveBeenCalledTimes(0);
```

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-019-vacuous-negative-assertions
make lint-check 2>&1 | tee /tmp/task-019-lint-demo-2.txt | tail -25
git checkout -- apps/web/src/lib/hooks/api/users.test.ts
```

Expected: FAIL, with the second message. Then confirm the neighbouring positive spelling is **not** flagged — add `expect(userService.listByIds).toHaveBeenCalledTimes(1);` instead, run `npx eslint src/lib/hooks/api/users.test.ts`, expect clean, revert.

- [ ] **Step 3: Confirm the tree is clean again**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-019-vacuous-negative-assertions
git status --short apps/web/src
```

Expected: empty.

- [ ] **Step 4: Complete the record**

In `probe-results.md`:

1. Confirm the main table has **exactly 40 rows**, every one with a site number, file, test title, spy, guard, S1, S2, verdict, fix, and re-probe column filled in (`—` where no fix was needed).
2. Paste the two lint demonstrations from Step 1-2 into the "Lint rule demonstration" section, with the actual command and the actual failure output (FR-DOC-2).
3. Fill in the FR-TRIAGE-4 section with every site whose Stage 2 was unconstructible, its reasoning, and its disposition (FR-DOC-3).
4. Add a short summary above the table: how many falsifiable, how many vacuous-and-fixed, how many unprobeable, how many exempted.

- [ ] **Step 5: Verify the record has no unfilled cells**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-019-vacuous-negative-assertions
grep -n "TBD\|TODO\|???\|_(none recorded yet)_\|_(recorded in Task 11)_" \
  docs/tasks/task-019-vacuous-negative-assertions/probe-results.md \
  || echo "clean: no placeholders left"
awk '/^\| [0-9]/ {n++} END {print "site rows:", n}' \
  docs/tasks/task-019-vacuous-negative-assertions/probe-results.md
```

Expected: `clean: no placeholders left` and `site rows: 40`.

- [ ] **Step 6: Commit**

```bash
git add docs/tasks/task-019-vacuous-negative-assertions/probe-results.md
git commit -m "docs(task-019): record all 40 probe verdicts and the lint demonstration"
```

---

## Task 12: Full verification

**Files:** none modified. This task only verifies.

**Interfaces:**
- Consumes: everything from Tasks 1-11.
- Produces: the evidence needed to claim the branch is done.

- [ ] **Step 1: Production-diff purity (PRD acceptance criterion; design §4 hygiene check 1)**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-019-vacuous-negative-assertions
git diff main -- apps/web/src --stat
```

Expected: only `*.test.ts` / `*.test.tsx` files plus `src/test/expectNoCall.ts`. Any other path is a failed revert — find it and restore it. Confirm mechanically:

```bash
git diff main --name-only -- apps/web/src \
  | grep -vE '\.test\.tsx?$|^apps/web/src/test/expectNoCall\.ts$' \
  || echo "clean: no production files changed"
```

- [ ] **Step 2: Probe-scaffolding purity (design §4 hygiene check 2)**

```bash
grep -rn "__probe__" apps/web/src || echo "clean: no probe scaffolding"
```

Expected: `clean: no probe scaffolding`.

- [ ] **Step 3: Confirm every non-exempt site uses the helper**

```bash
cd apps/web
echo "--- remaining bare assertions (expect: 2, both eslint-disabled) ---"
grep -rn --include='*.test.ts' --include='*.test.tsx' \
  -E '\.not\.toHaveBeenCalled|toHaveBeenCalledTimes\(0\)' src \
  | grep -v 'src/test/expectNoCall.test.ts'
echo "--- helper call sites (expect: 38) ---"
grep -rn --include='*.test.ts' --include='*.test.tsx' -c 'expectNoCall' src \
  | grep -v ':0$'
```

Expected: exactly two bare assertions remain (`input.test.tsx`, `download.test.ts`), each immediately preceded by an `eslint-disable-next-line no-restricted-syntax` comment citing its probe result; 38 helper calls across 18 files.

- [ ] **Step 4: Full CI**

```bash
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
cd /home/tumidanski/source/MyFleet/.worktrees/task-019-vacuous-negative-assertions
make ci 2>&1 | tail -40
```

Expected: PASS. `lint-check` is green for the first time since Task 2 — that transition is the signal the migration is complete.

If `make ci` fails on a Go target, it is unrelated to this branch (no Go file is touched); confirm by `git diff main --name-only | grep '\.go$'` returning nothing, and report it rather than fixing it here.

- [ ] **Step 5: Confirm the wall-time NFR**

Compare the Task 10 Step 6 `make fe-test` timing against the Task 1 Step 1 baseline. Expected: under +10%. Record the two numbers in the PR description. If it regressed further, the fix goes **in the helper**, not at the call sites, and every migrated file is re-run.

- [ ] **Step 6: Code review before PR**

Per CLAUDE.md ("Code Review Before PR"), run the review step now — do not skip it.

```
superpowers:requesting-code-review
```

Only the frontend reviewer applies (no Go files changed). Findings go to `docs/tasks/task-019-vacuous-negative-assertions/audit.md`.

- [ ] **Step 7: Open the PR**

The PR body must reference and close issue #22, link `probe-results.md`, and state the two wall-time numbers from Step 5. Note explicitly that no production source changed and that two sites are exempted with inline evidence.

---

## Self-Review

**Spec coverage** — every PRD and design requirement mapped to a task:

| Requirement | Task |
|---|---|
| FR-TRIAGE-1 (all 40 triaged) | Site inventory table + Tasks 3-10 (7+5+6+7+6+5+2+2 = 40) |
| FR-TRIAGE-2 (probe, not inspection) | "The probe procedure" §, Stage 2, in every migration task |
| FR-TRIAGE-3 (red = falsifiable, green = fix) | Verdict matrix in "The probe procedure" |
| FR-TRIAGE-4 (unprobeable sites reported) | Task 4 site 3, Task 9 site 34; recorded in Task 11 Step 4.3 |
| FR-TRIAGE-5 (probes reverted) | Global Constraints; per-task revert steps; Task 12 Steps 1-2 |
| FR-TRIAGE-6 (Group D probed anyway) | Task 10 Steps 1-2 |
| FR-HELPER-1 (shared helper under `src/test/`) | Task 1 Step 4 |
| FR-HELPER-2 (flush at least as strong as #20) | Task 1 Step 4; Global Constraints |
| FR-HELPER-3 (all non-exempt sites use it) | Tasks 3-9; verified Task 12 Step 3 |
| FR-HELPER-4 (`With` variant, informative message) | Task 1 Step 4 (`expectNoCallWith` + `mockName`); tested Task 1 Step 2 |
| FR-HELPER-5 (usable from non-React tests, no act warnings) | Task 1 Step 5; Tasks 6 and 8 Step 5 |
| FR-FIX-1 (re-probe every fix to red) | Re-probe step in every migration task |
| FR-FIX-2 (second, non-timing vacuity fixed too) | Task 3 (LoginPage's two guards); verdict matrix `red/green` row |
| FR-FIX-3 (no production source modified) | Global Constraints; Task 12 Step 1 |
| FR-LINT-1 (selector for the bare form) | Task 2 Step 1 |
| FR-LINT-2 (`toHaveBeenCalledTimes(0)`) | Task 2 Step 1; demonstrated Task 11 Step 2 |
| FR-LINT-3 (helper exempted) | Task 2 Step 1 (exemption block covers helper **and** its test); verified Step 2 |
| FR-LINT-4 (rule demonstrated to fire) | Task 11 Steps 1-2 |
| FR-LINT-5 (`lint-check` green, zero warnings) | Task 10 Step 5; Task 12 Step 4 |
| FR-DOC-1 (probe-results table) | Task 2 Step 5 skeleton; appended per task; completed Task 11 Step 4 |
| FR-DOC-2 (lint demo output recorded) | Task 11 Step 4.2 |
| FR-DOC-3 (unprobeable sites recorded) | Task 11 Step 4.3 |
| Design F-1 (`act` from RTL) | Task 1 Step 4 comment; Global Constraints |
| Design F-3 (`mockName` for the label) | Task 1 Step 4; label passed at every site |
| Design F-4 (both selectors) | Task 2 Step 1 |
| Design F-5 (20 files, zero `Times(0)`, packages gap) | Site inventory; Task 2 Step 5 residual-gaps section |
| Design §4 two-stage probe + verdict matrix | "The probe procedure" § |
| Design §4 keying on file + title + spy | Global Constraints; Task 2 Step 5 |
| Design §6 sequencing | Task order: helper → lint → Group E → A/B/C → D → demo/record → CI |
| Design §8 risk: wall-time regression | Task 1 Step 1 baseline; Task 10 Step 6; Task 12 Step 5 |
| PRD §10 all acceptance criteria | Task 12 Steps 1-7 |

**Additions this plan makes beyond the design, and why:**

- **Fake-timer incompatibility.** `download.test.ts` runs under `vi.useFakeTimers()`, so the helper's `setTimeout(0)` never fires and a migrated site there would hang. The design justified the Group D exemption only by "avoids importing RTL into a pure-util test"; the real blocker is stronger. Captured in Global Constraints and demonstrated in Task 10 Step 3.
- **`vi.mocked()` at mocked-module call sites.** `expect()` accepts anything, but `expectNoCall(spy: MockInstance)` does not — an imported service method is typed as the real function. The transform table makes this explicit so `tsc -b` does not fail 20 files in.
- **Five synchronous `it(...)` callbacks need `async`.** Enumerated in the inventory table so no migration task discovers it by test failure.
- **CategoryCombobox site 17 already has its positive pairing.** The design flagged it as possibly missing one; it has `await waitFor(() => expect(onChange).toHaveBeenCalledWith('server-assigned-id'))` directly above. Task 7 says to leave it.
- **`toHaveBeenCalledTimes(0)` needs its own lint demonstration.** It matches nothing on the tree, so FR-LINT-4's demo of the first selector does not exercise it. Task 11 Step 2.

**Placeholder scan:** no `TBD`/`TODO`/"similar to Task N"/"add appropriate error handling" in this plan. Every code step carries the literal code. Every probe carries the named guard and the file it lives in. The two places that say "check the file's `vi.mock` factory before editing" (Tasks 7 and 9) are instructions to read two lines of an existing file, not deferred decisions — the transform itself is fully specified either way.

**Type consistency:** `flushPending()`, `expectNoCall(spy, label?)`, and `expectNoCallWith(spy, args, label?)` are defined once in Task 1 and used with exactly those names and argument orders in Tasks 3-9. `args` is an array at every `expectNoCallWith` call site (Tasks 6 and 7). `label` is the trailing parameter in both functions.
