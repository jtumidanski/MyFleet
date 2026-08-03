# Vacuous Negative Assertions — Product Requirements Document

Version: v1
Status: Draft
Created: 2026-08-03
Issue: [#22](https://github.com/jtumidanski/MyFleet/issues/22)
---

## 1. Overview

Several web tests assert that a spy was **not** called, immediately after a
synchronous render or `act()`:

```tsx
renderWithProviders(<Thing />);
expect(someService.fetchThing).not.toHaveBeenCalled();
```

When the call under test would be dispatched from a promise continuation —
which is how React Query issues both queries and mutations — the synchronous
assertion runs *before* any dispatch could occur. The assertion then passes
whether or not the guard it exists to prove actually works. It measures timing,
not behaviour. A test in this shape is not a weak test; it is a test that cannot
fail, and it will keep reporting green through the exact regression it was
written to catch.

This was confirmed once already. During the task-010 review (#20),
`apps/web/src/pages/LoginPage.test.tsx` asserted that cycling the theme on the
signed-out login page issues no network request. Two independent reasons made it
unfalsifiable: `updateThemePreference` short-circuits when there is no access
token (`apps/web/src/lib/hooks/api/auth.ts:68`) and the test's `beforeEach`
cleared `localStorage`; and even with a token seeded, `mutate()` dispatches in a
promise continuation. A mutation probe proved it — with the mutation-bearing
`ThemeToggle` swapped into the page, the test passed with neither fix and with
the token fix alone. Only after adding a microtask flush did it correctly report
`expected "fetch" to not be called at all, but actually been called 3 times`.

Issue #22 names six further candidates. A scan of the suite found the true
population is larger: **40 negative call assertions across 18 files**. Some are
provably fine by construction; some are the same latent bug as `LoginPage`;
inspection alone cannot tell them apart, because the deciding factor is whether
the *production* code path would have dispatched asynchronously, not what the
test looks like. This task triages all 40 by probe, fixes what is broken, and
installs a helper plus a lint rule so the pattern cannot silently return.

## 2. Goals

Primary goals:

- Establish, per site, whether each negative call assertion in `apps/web` can
  actually fail — by probe, not by reading.
- Fix every site that cannot fail, so it can.
- Provide one shared helper that makes the correct form the easy form.
- Enforce the helper with lint, so a new bare `not.toHaveBeenCalled()` cannot
  land unnoticed.
- Leave a written, falsifiable record of what each probe showed.

Non-goals:

- Broader test-quality work in `apps/web` (coverage gaps, flakiness, slow
  suites, test restructuring). Only the negative-assertion class is in scope.
- Auditing Go test suites. This bug class is specific to the JS event loop and
  React Query's dispatch model; no Go service is touched.
- Changing production behaviour. Every site's current runtime behaviour is
  believed correct — the guards genuinely work. This task changes only whether
  the tests would *notice* if they stopped working.
- Rewriting tests that are merely awkward but demonstrably falsifiable.

## 3. User Stories

- As a developer, I want a test that asserts "no request was made" to fail when
  a request *is* made, so that the guard it covers stays covered.
- As a developer, I want to write that assertion correctly without knowing about
  React Query's dispatch timing, so that the trap is not re-laid by the next
  person.
- As a reviewer, I want lint to reject the vacuous form, so that catching it does
  not depend on someone remembering issue #22.
- As a maintainer, I want a record of which sites were probed and what each probe
  showed, so that a future audit can check the work rather than repeat it.

## 4. Functional Requirements

### 4.1 Triage (FR-TRIAGE-*)

**FR-TRIAGE-1** — Every negative call assertion in `apps/web/src` must be
triaged. The inventory is the 40 sites in §4.2. The set is defined by the
pattern `.not.toHaveBeenCalled()`, `.not.toHaveBeenCalledWith(...)`, and
`.toHaveBeenCalledTimes(0)` in `*.test.ts` / `*.test.tsx`.

**FR-TRIAGE-2** — Triage is performed by **mutation probe**, not inspection. For
each site: temporarily defeat the guard the assertion depends on (remove the
early return, drop the `enabled` flag, delete the structural check — whatever
makes the call reachable), run that test file, and record whether the test went
**red**.

**FR-TRIAGE-3** — A site whose probe produces red is **falsifiable**; it is left
unchanged except where FR-HELPER-3 applies. A site whose probe stays green is
**vacuous** and must be fixed under FR-FIX-*.

**FR-TRIAGE-4** — A probe that cannot be constructed at all — because no change
to production code could make the spy fire — means the assertion is asserting
something structurally unreachable. Such a site must be reported explicitly and
either strengthened into an assertion that can fail, or deleted with its reason
recorded. It must not be left as decoration.

**FR-TRIAGE-5** — The probe must be reverted after each site. The final diff must
contain no probe scaffolding. Verified by `git diff` review against the
production sources listed in §7.

**FR-TRIAGE-6** — Sites that are safe *by construction* (the spied function is
called synchronously, so no flush could change the outcome) still require a
probe. `apps/web/src/components/ui/input.test.tsx:68` (`showPicker`, a direct DOM
call) and `apps/web/src/lib/utils/download.test.ts:43` (`URL.revokeObjectURL` in
a pure util) are expected to fall here, but the expectation must be confirmed,
not assumed. This is cheap for these sites — the probe is a one-line edit.

### 4.2 Site Inventory

All 40 sites, grouped by the risk signal that a first read suggests. **The
grouping is a triage order, not a verdict** — every row gets a probe, and the
probe overrides the grouping.

**Group A — named in issue #22 as needing triage (6 sites):**

| # | Site | Spy | Notes from issue #22 |
|---|------|-----|----------------------|
| 1 | `src/components/features/vehicles/VehicleCard.test.tsx:86` | `mediaService.getContentBlob` | Guard is "no media id"; assertion follows a synchronous `renderWithProviders` |
| 2 | `src/components/features/vehicles/VehiclePhotoThumbnail.test.tsx:65` | `mediaService.getContentBlob` | Same guard, same shape |
| 3 | `src/components/features/vehicles/VehiclePhotoThumbnail.test.tsx:114` | `mediaService.getContentBlob` | Same guard, same shape |
| 4 | `src/lib/hooks/api/media.test.ts:101` | `deps.initUpload` | Follows an awaited `rejects` assertion — may already be falsifiable |
| 5 | `src/lib/hooks/api/media.test.ts:102` | `deps.putContent` | As above |
| 6 | `src/lib/hooks/api/media.test.ts:103` | `deps.confirm` | As above |

Sites 4–6 illustrate why inspection is not enough: the preceding
`await expect(performMediaUpload(...)).rejects.toMatchObject({...})` already
yields to the event loop, which may well make these three falsifiable as
written. Only the probe settles it.

**Group B — usePendingAttachments, named in #22 (2 sites):**

| # | Site | Spy | Notes |
|---|------|-----|-------|
| 7 | `src/lib/hooks/usePendingAttachments.test.ts:92` | `mediaService.remove` | Assertion follows a synchronous `act(() => result.current.remove(...))` |
| 8 | `src/lib/hooks/usePendingAttachments.test.ts:124` | `mediaService.remove` | Same shape |

**Group C — not named in #22, same structural shape (25 sites):**

| # | Site | Spy |
|---|------|-----|
| 9 | `src/components/features/activity/ActivityFeed.test.tsx:134` | `listByIds` |
| 10 | `src/components/features/dashboard/useDashboardWidgets.test.ts:122` | `dashboardService.saveLayout` |
| 11 | `src/components/features/dashboard/useDashboardWidgets.test.ts:171` | `dashboardService.saveLayout` |
| 12 | `src/components/features/settings/MemberList.test.tsx:166` | `memberService.removeMember` |
| 13 | `src/components/features/settings/MemberList.test.tsx:187` | `memberService.removeMember` |
| 14 | `src/components/features/settings/MemberList.test.tsx:204` | `memberService.updateRole` |
| 15 | `src/components/features/settings/MemberList.test.tsx:249` | `memberService.updateRole` |
| 16 | `src/components/features/settings/MemberList.test.tsx:327` | `memberService.removeMember` |
| 17 | `src/components/features/vehicles/CategoryCombobox.test.tsx:157` | `onChange` (`not.toHaveBeenCalledWith`) |
| 18 | `src/components/features/vehicles/CategoryCombobox.test.tsx:173` | `onChange` |
| 19 | `src/components/features/vehicles/detail/VehicleRecordsTable.test.tsx:111` | `onLoadMore` |
| 20 | `src/components/features/vehicles/dialogs/PhotoGalleryDialog.test.tsx:115` | `toast.error` |
| 21 | `src/components/features/vehicles/dialogs/PhotoGalleryDialog.test.tsx:125` | `removeMedia` |
| 22 | `src/components/features/vehicles/dialogs/PhotoGalleryDialog.test.tsx:126` | `removeObject` |
| 23 | `src/components/features/vehicles/maintenance/MaintenanceRecordForm.test.tsx:92` | `onSubmit` |
| 24 | `src/components/features/vehicles/VehiclePhotoThumbnail.test.tsx:93` | `toast.error` |
| 25 | `src/components/features/vehicles/VehiclePhotoThumbnail.test.tsx:94` | `toast` |
| 26 | `src/components/features/vehicles/VehiclePhotoThumbnail.test.tsx:95` | `toast.warning` |
| 27 | `src/components/features/vehicles/VehiclePhotoThumbnail.test.tsx:96` | `toast.info` |
| 28 | `src/lib/hooks/api/media.test.ts:219` | `revokeObjectURL` (`not.toHaveBeenCalledWith`) |
| 29 | `src/lib/hooks/api/members.test.ts:195` | `mintAccessToken` |
| 30 | `src/lib/hooks/api/members.test.ts:270` | `mintAccessToken` |
| 31 | `src/lib/hooks/api/users.test.ts:78` | `userService.listByIds` |
| 32 | `src/lib/hooks/api/vehicleRecords.test.ts:325` | `fuel.fetchNextPage` |
| 33 | `src/pages/VehiclesPage.test.tsx:188` | `vehicleService.createInFleet` |
| 34 | `src/pages/VehiclesPage.test.tsx:224` | `vehicleService.createInFleet` |
| 35 | `src/pages/VehiclesPage.test.tsx:235` | `vehicleService.createInFleet` |
| 36 | `src/pages/admin/AdminFleetsPage.test.tsx:159` | `createPurgeMutate` |

**Group D — expected safe, probe anyway (2 sites):**

| # | Site | Spy |
|---|------|-----|
| 37 | `src/components/ui/input.test.tsx:68` | `showPicker` |
| 38 | `src/lib/utils/download.test.ts:43` | `URL.revokeObjectURL` |

**Group E — already resolved, re-verify only (2 sites):**

| # | Site | Spy | Status |
|---|------|-----|--------|
| 39 | `src/pages/LoginPage.test.tsx:245` | `fetchSpy` | Fixed in #20 (token seeded + flush). Must be migrated to the helper without losing falsifiability. |
| 40 | `src/lib/hooks/api/auth.test.ts:92` | `fetchMock` | Issue #22 records this as checked and **not** affected: it `await`s `updateThemePreference()` directly rather than going through a mutation, and the short-circuit is the subject under test. Confirm this finding still holds; do not "fix" it. |

### 4.3 Helper (FR-HELPER-*)

**FR-HELPER-1** — A shared helper module must be added under
`apps/web/src/test/`, exporting a function that flushes pending work and then
asserts the spy was not called. Working name: `expectNoCall(spy)`. The issue
suggests `expectNoRequest`; the broader name is preferred because a third of the
inventory spies on non-network callbacks (`onSubmit`, `onChange`, `onLoadMore`,
`toast.*`). Final naming is a design-phase decision (§9, OQ-1).

**FR-HELPER-2** — The flush must be at least as strong as the one proven in #20:

```ts
await act(async () => {
  await new Promise((resolve) => setTimeout(resolve, 0));
});
```

This drains the microtask queue (where React Query's `mutate()` dispatches) and
one macrotask tick. If any probe shows a site needs a deeper flush, the helper —
not the call site — is what changes.

**FR-HELPER-3** — Every site in the inventory that is a genuine negative call
assertion must go through the helper, including sites the probe found already
falsifiable. Uniformity is the point: a reader must not have to work out why one
site flushes and its neighbour does not, and a site that is falsifiable today for
an incidental reason (an `await` two lines up) can silently stop being so.
Group D sites (§4.2) are exempt if the probe confirms they are synchronous by
construction, since a flush there would be noise — this exemption must be
justified per site in the record.

**FR-HELPER-4** — The helper must cover the `toHaveBeenCalledWith` variant used
by sites 17 and 28, either through an optional-arguments parameter or a sibling
export. It must produce a failure message that names the spy and reports the
actual call count, at least as informative as Vitest's built-in.

**FR-HELPER-5** — The helper must be usable from non-React tests
(`media.test.ts`, `download.test.ts`, `users.test.ts`) without emitting act-scope
warnings. If wrapping in `act` proves to warn outside a React render, the helper
must branch (§9, OQ-2).

### 4.4 Fixes (FR-FIX-*)

**FR-FIX-1** — Every site the probe found vacuous must be converted to the helper
and then **re-probed**. The re-probe must produce red. A fix that is not
re-probed is not a fix — it is an untested change to a test.

**FR-FIX-2** — Where a site is vacuous for a *second*, non-timing reason — as
`LoginPage` was, via `localStorage.clear()` starving the token check — that
reason must be fixed too. A flush alone does not rescue an assertion whose
subject was never reachable.

**FR-FIX-3** — No production source file may be modified. If a probe reveals an
actual production bug, it is recorded and raised as a separate issue, not fixed
here.

### 4.5 Enforcement (FR-LINT-*)

**FR-LINT-1** — `apps/web/eslint.config.js` must gain a rule, scoped to the
existing test-files block, that rejects the bare form. A `no-restricted-syntax`
selector avoids adding a dependency:

```js
'no-restricted-syntax': ['error', {
  selector:
    "CallExpression[callee.object.property.name='not']" +
    "[callee.property.name=/^toHaveBeenCalled/]",
  message:
    'Use expectNoCall(spy) from src/test — a bare not.toHaveBeenCalled() ' +
    'runs before promise-continuation dispatch and can pass vacuously. ' +
    'See issue #22.',
}],
```

**FR-LINT-2** — The rule must also reject `toHaveBeenCalledTimes(0)`, which is
the same assertion spelled differently and appears in the pattern the inventory
was built from.

**FR-LINT-3** — The helper's own module must be exempted, since it necessarily
contains the banned expression.

**FR-LINT-4** — The rule must be demonstrated to fire: introduce a bare
assertion, observe `make lint-check` fail, remove it. Recorded per FR-DOC-2.

**FR-LINT-5** — `make lint-check` must pass with zero warnings on the finished
branch (`eslint src --max-warnings 0`).

### 4.6 Record (FR-DOC-*)

**FR-DOC-1** — A probe-results table must be committed to the task folder, one
row per inventory site: site, the guard defeated, the probe verdict (red /
green / unprobeable), the fix applied, and the re-probe verdict where a fix was
applied.

**FR-DOC-2** — The lint-rule demonstration (FR-LINT-4) must be recorded with its
actual failure output.

**FR-DOC-3** — Sites found unprobeable (FR-TRIAGE-4) must be recorded with the
reasoning and the disposition chosen.

## 5. API Surface

None. This task adds no endpoints and modifies no request or response shape. The
only new surface is internal to the test suite:

```ts
// apps/web/src/test/expectNoCall.ts
export async function expectNoCall(spy: Mock): Promise<void>;
export async function expectNoCallWith(spy: Mock, ...args: unknown[]): Promise<void>;
```

Exact signatures are settled in the design phase.

## 6. Data Model

None. No entities, fields, migrations, or persistence changes.

## 7. Service Impact

**`apps/web`** — the only service affected.

Changed:

- `apps/web/src/test/` — new helper module (+ its own unit test).
- `apps/web/eslint.config.js` — new `no-restricted-syntax` rule and helper
  exemption.
- Up to 18 `*.test.ts` / `*.test.tsx` files across `src/components`,
  `src/lib/hooks`, `src/lib/utils`, and `src/pages`.

Touched during probing, restored before commit (FR-TRIAGE-5):

- `apps/web/src/components/features/vehicles/VehiclePhotoThumbnail.tsx`
- `apps/web/src/components/features/vehicles/VehicleCard.tsx`
- `apps/web/src/lib/hooks/usePendingAttachments.ts`
- `apps/web/src/lib/hooks/api/*.ts`
- and other production modules as each probe requires.

Unaffected: `apps/auth-service`, `apps/fleet-service`, `apps/media-service`,
`apps/notification-service`, `packages/*`, `deploy/k8s`.

## 8. Non-Functional Requirements

- **Performance** — the helper adds one macrotask tick per call site. Across ~38
  sites this is negligible; if `make fe-test` wall time regresses by more than
  10%, the flush strategy is revisited.
- **Determinism** — the helper must not introduce flake. A fixed `setTimeout(0)`
  is deterministic under Vitest's default environment; polling (`waitFor`) is
  not preferred here, because waiting for a *non*-event has no success condition
  to poll for and would only add latency.
- **Observability** — none. No runtime code path changes.
- **Security** — none.
- **Maintainability** — the lint rule's message must name the helper and cite
  issue #22, so the next person hitting it learns the reason rather than
  reaching for an `eslint-disable`.

## 9. Open Questions

- **OQ-1 (naming)** — `expectNoCall` vs. the issue's `expectNoRequest`. A third
  of the inventory spies on non-network callbacks, which argues for the broader
  name. Recommendation: `expectNoCall`. Settle in design.
- **OQ-2 (act scope)** — whether wrapping the flush in `act` warns in tests with
  no React root (`download.test.ts`, `media.test.ts`). If it does, the helper
  branches on whether a React root is mounted, or exposes a `{ react: false }`
  option. Determine empirically in design.
- **OQ-3 (flush depth)** — one macrotask tick is proven sufficient for React
  Query mutations. Whether any inventory site needs a deeper flush (nested
  promise chains, multiple `setTimeout` hops) is a probe output, not a
  prediction.
- **OQ-4 (Group D exemption)** — whether to exempt confirmed-synchronous sites
  from the helper (and therefore from the lint rule, via inline disables with a
  justifying comment) or to route everything through it for uniformity.
  Recommendation: exempt, with a required comment naming the probe result — an
  `eslint-disable` carrying evidence is more honest than a flush that does
  nothing.

## 10. Acceptance Criteria

- [ ] All 40 inventory sites in §4.2 have a recorded probe verdict.
- [ ] Every site found vacuous has been fixed and re-probed to red.
- [ ] The probe-results table is committed to
      `docs/tasks/task-019-vacuous-negative-assertions/`, covering site, guard
      defeated, verdict, fix, and re-probe verdict.
- [ ] `apps/web/src/test/` exports the helper, with its own unit test proving it
      catches a call that a bare synchronous assertion misses.
- [ ] Every non-exempt inventory site uses the helper.
- [ ] Any Group D exemption carries an inline comment citing its probe result.
- [ ] `apps/web/eslint.config.js` rejects bare `not.toHaveBeenCalled()`,
      `not.toHaveBeenCalledWith()`, and `toHaveBeenCalledTimes(0)` in test files.
- [ ] The lint rule has been demonstrated to fire, with output recorded.
- [ ] No production source file differs from `main` (`git diff main --
      apps/web/src --stat` shows test, helper, and config changes only).
- [ ] `make ci` passes.
- [ ] Issue #22 is referenced in the PR and closed by it.
