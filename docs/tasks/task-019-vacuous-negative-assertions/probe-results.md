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

| 1 | `apps/web/src/components/features/vehicles/VehicleCard.test.tsx` | renders the placeholder at identical dimensions when no photo is set | `mediaService.getContentBlob` | `if (!mediaId)` early return in `VehiclePhotoThumbnail.tsx` (deleted) **and** `enabled: !!id` → `enabled: true` in `useMediaContentUrl` (`src/lib/hooks/api/media.ts`) — both required, same pair as site 2 | green (dispatch point coincides with the assertion — no yielding statement sits between `render()` and it) | red² (call recorded as `[undefined, "card"]`) | falsifiable, fragile (design combining-table row 2: green/red) | migrated to `expectNoCall`, `it` made `async` | red / red |
| 2 | `apps/web/src/components/features/vehicles/VehiclePhotoThumbnail.test.tsx` | shows the "No photo" placeholder when there is no media id, and fetches nothing | `mediaService.getContentBlob` | same pair as site 1 | green (same reasoning — no yield between `render()` and the assertion) | red² (call recorded as `[undefined, "card"]`) | falsifiable, fragile (design combining-table row 2: green/red) | migrated to `expectNoCall`, `it` made `async` | red / red |
| 3 | `apps/web/src/components/features/vehicles/VehiclePhotoThumbnail.test.tsx` | says "Photo unavailable", not "No photo", when a known photo cannot be fetched | `mediaService.getContentBlob` | `networkMode: 'always'` added to the `useQuery` in `useMediaContentUrl` — see finding³ below | red (probe inserted after `renderWithProviders(...)`, before the pre-existing `await screen.findByRole(...)`; observed 1 call) | red³ (call recorded as `["m1", "card"]`) | falsifiable — **not** FR-TRIAGE-4 (see finding³) | migrated to `expectNoCall` | n/a (was never vacuous) |
| 24 | `apps/web/src/components/features/vehicles/VehiclePhotoThumbnail.test.tsx` | fires no toast when a thumbnail fails to load | `toast.error` | `toast.error('probe')` added to the `if (isError \|\| !url)` branch (with the `sonner` import) | red (probe after `renderWithProviders(...)`, before the pre-existing `await screen.findByRole(...)`) | red (2 calls — see note⁴) | falsifiable | migrated to `expectNoCall` | n/a (was never vacuous) |
| 25 | `apps/web/src/components/features/vehicles/VehiclePhotoThumbnail.test.tsx` | fires no toast when a thumbnail fails to load | `toast` | `toast('probe')` added to the same branch⁴ | red (same placement as site 24) | red (2 calls) | falsifiable | migrated to `expectNoCall` | n/a (was never vacuous) |
| 26 | `apps/web/src/components/features/vehicles/VehiclePhotoThumbnail.test.tsx` | fires no toast when a thumbnail fails to load | `toast.warning` | `toast.warning('probe')` added to the same branch⁴ | red (same placement as site 24) | red (2 calls) | falsifiable | migrated to `expectNoCall` | n/a (was never vacuous) |
| 27 | `apps/web/src/components/features/vehicles/VehiclePhotoThumbnail.test.tsx` | fires no toast when a thumbnail fails to load | `toast.info` | `toast.info('probe')` added to the same branch⁴ | red (same placement as site 24) | red (2 calls) | falsifiable | migrated to `expectNoCall` | n/a (was never vacuous) |

² Stage 2 for sites 1 and 2: in the full (unmodified) test the guard-defeat
edit makes an *earlier* assertion fail first — `screen.getByRole('img', {
name: 'No photo' })` — because deleting the `!mediaId` early return removes
the only code path that renders that role/name, so the component now shows a
loading skeleton (or, once the mock resolves, the real photo) instead.
Per migration-context.md's "Stage-2 evidence quality" note, that is weaker
evidence — the assertion under test was never reached. It was strengthened by
temporarily removing the earlier assertion in an uncommitted, reverted copy
of the test: with only `expect(mediaService.getContentBlob).not.toHaveBeenCalled()`
left in the body, the guard defeat reaches it directly and it fails with the
call recorded as `[undefined, "card"]`. The call is **synchronous** — it
happens during the render's effect flush inside `renderWithProviders`'s
`act()`, not via a promise continuation — which is exactly why Stage 1 (a
pure microtask probe placed with nothing yielding before the bare assertion)
came back green while Stage 2 came back red: the design's combining table
calls this "falsifiable only because the defeated guard fires synchronously"
(row 2), not vacuous. Migrating to `expectNoCall` (which both tests needed
regardless, per the task-4 brief, since it also makes them robust to a
promise-continuation-based regression) was still the right move — re-probing
both stages afterward against the migrated test confirms **red/red**, using
the same guard-defeat edits and the same isolation technique described above
(the earlier `getByRole('No photo')` assertion still fails first in the full
migrated test; isolating confirms the migrated assertion itself is red too).

³ **Finding — site 3 is NOT unconstructible, contrary to the task-4 brief's
FR-TRIAGE-4 prediction.** The brief states the guard is "React Query pausing
the query while `onlineManager` reports offline — library behaviour, not app
code" and expects no edit under `apps/web/src` to make the fetch fire while
offline. An edit was found: adding `networkMode: 'always'` to the `useQuery`
call inside `useMediaContentUrl` (`src/lib/hooks/api/media.ts`) is a
documented, first-class TanStack Query option, set at the same app-code call
site as every other query option on that hook, and it does make
`mediaService.getContentBlob` fire while `onlineManager.setOnline(false)` is
in effect. Verified in an isolated, uncommitted copy of the test (replacing
the `await screen.findByRole('img', { name: 'Photo unavailable' })` /
`queryByRole('img', { name: 'No photo' })` assertions with
`await screen.findByAltText('Photo of 2019 Honda Civic')`, since the mock
resolves successfully once the pause is defeated): the target assertion fails
directly, with the call recorded as `["m1", "card"]`. Running the full,
unmodified test with only the `networkMode: 'always'` edit present confirms
no other test in the file regresses (11 passed, 1 failed — only the target
test, and only because an *earlier* assertion in that same test —
`findByRole('img', { name: 'Photo unavailable' })` — fails first, since the
photo now loads instead of pausing; same "red from an earlier line" pattern
as site 39/40 and sites 1/2 above). Disposition: Stage 1 was independently
red for this site (see table), so the combining-table outcome is
**falsifiable** (red/red), not FR-TRIAGE-4 — the assertion is migrated to
`expectNoCall` on that basis, same as every other falsifiable site. This
`networkMode: 'always'` edit was a **probe-only exploration**, reverted with
`git checkout --` immediately after use; it is not proposed as a production
change, and no production behaviour was altered by this task.

⁴ The task-4 brief's Stage-2 instruction ("add `toast.error('probe')` to the
`if (isError || !url)` branch") is written as a single edit against all four
sites (24–27), and it does correctly demonstrate red for site 24 (`toast.error`
itself). Read literally, though, one call to `toast.error(...)` does not call
the plain `toast(...)`, `toast.warning(...)`, or `toast.info(...)` mocks — so
it cannot, by itself, defeat the guard for sites 25–27. Confirmed directly:
with only the `toast.error('probe')` edit in place and the `toast.error`
assertion removed from the test (isolated, uncommitted), the remaining three
assertions (`toast`, `toast.warning`, `toast.info`) all still pass (test
green — those three spies stayed uncalled). Each of the other three sites was
therefore verified with its own natural analogous edit instead —
`toast('probe')`, `toast.warning('probe')`, `toast.info('probe')`, one at a
time, each reverted before the next — and each produced a direct red on its
own assertion. Every one of the four probed spies (including `toast.error`
itself) recorded exactly 2 calls, not 1; the component re-renders through the
error branch twice under this test's conditions, and the cause was not
investigated further since it is immaterial to the verdict — one call would
already be sufficient to fail `not.toHaveBeenCalled()`. All four probe edits
were reverted before migration; the committed component carries none of
them.

## FR-TRIAGE-4 sites (unprobeable)

Task 4 attempted its one predicted FR-TRIAGE-4 candidate (site 3,
`VehiclePhotoThumbnail.test.tsx` — "says \"Photo unavailable\", not \"No
photo\", when a known photo cannot be fetched") and found it **is**
constructible via `networkMode: 'always'`. See footnote ³ on the site 3 row
above for the full investigation and disposition. No site in this task
belongs in this section.

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
