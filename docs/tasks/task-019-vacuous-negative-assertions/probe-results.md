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
| 12 | `apps/web/src/components/features/settings/MemberList.test.tsx` | does not fire the DELETE until the dialog is confirmed | `memberService.removeMember` | Remove row button changed to call `void confirmRemove(userId)` directly, bypassing the `AlertDialog` | red (probe after the first click, before the pre-existing `expect(await screen.findByText(...))`; 1 call) | red⁵ (call recorded as `["f1", "other"]`) | falsifiable | migrated to `expectNoCall` | n/a (was never vacuous) |
| 13 | `apps/web/src/components/features/settings/MemberList.test.tsx` | fires nothing when the dialog is cancelled | `memberService.removeMember` | `closeDialog` changed to also call `void confirmRemove(...)` for the pending 'remove' row (re-entrancy note⁶) | red (probe after the cancel click, before the pre-existing `await waitFor(...)`; 1 call) | red (call recorded as `["f1", "other"]`, target assertion hit directly, no earlier-line interference) | falsifiable | migrated to `expectNoCall` | n/a (was never vacuous) |
| 14 | `apps/web/src/components/features/settings/MemberList.test.tsx` | is offered to owners on non-owner rows and confirms before PATCHing | `memberService.updateRole` | Make-owner row button changed to call `void confirmPromote(userId)` directly, bypassing the dialog | red (probe after the first click, before the pre-existing `expect(await screen.findByText(...))`; 1 call) | red⁵ (call recorded as `["f1", "other", "owner"]`) | falsifiable | migrated to `expectNoCall` | n/a (was never vacuous) |
| 15 | `apps/web/src/components/features/settings/MemberList.test.tsx` | offers a plain leave confirmation to a member | `memberService.updateRole` | see finding⁷ — brief's prescribed dialog-bypass edit does not reach this spy; the guard actually defeated is the `if (needsSuccessor)` check in `confirmLeave` | red (probe after the in-dialog Leave click, before the pre-existing `await waitFor(...)`; 1 call) | red⁷ (call recorded as `["f1", "", "owner"]`, after defeating the corrected guard) | falsifiable | migrated to `expectNoCall` | n/a (was never vacuous) |
| 16 | `apps/web/src/components/features/settings/MemberList.test.tsx` | does not remove the leaver when the promote fails | `memberService.removeMember` | `updateRole.mutateAsync(...)` in `confirmLeave` wrapped in `.catch(() => undefined)`, so the rejection no longer stops the subsequent `removeMember.mutateAsync` | red (probe after the "Transfer & leave" click, before the pre-existing `await waitFor(...)`; 1 call) | red (call recorded as `["f1", "me"]`, target assertion hit directly) | falsifiable | migrated to `expectNoCall` | n/a (was never vacuous) |

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

⁵ Stage 2 for sites 12 and 14: in the full (unmodified) test, bypassing the
dialog-open gate means the dialog never renders, so the confirmation-text
assertion that precedes the target assertion (`findByText(/Remove Sam Ito
from this fleet\?/i)` for site 12; `findByText(/Make Sam Ito an owner\?/i)`
for site 14) times out first. Per migration-context.md's "Stage-2 evidence
quality" note, that is weaker evidence — the assertion under test was never
reached. Strengthened by isolating in an uncommitted, reverted copy of the
test (removing the earlier dialog-text assertion so only the target
`not.toHaveBeenCalled()` line remains): with only the guard-defeat edit named
in the Guard-defeated column applied, the target assertion is reached
directly and fails with the call recorded as shown in the table. Isolated
and reverted independently for each site.

⁶ Site 13's literal guard-defeat ("`closeDialog` also calls `confirmRemove`
for the pending row") recurses infinitely if implemented without a guard:
`confirmRemove` calls `closeDialog` again on its own success path, and
because the render-closure's `pending` value is frozen for the lifetime of
that render, an unguarded second call reads the same still-`'remove'`
`pending` value and calls `confirmRemove` again, forever. The first attempt
at this edit hung the run until Node ran out of heap (`FATAL ERROR:
Ineffective mark-compacts near heap limit Allocation failed - JavaScript
heap out of memory`, worker killed after ~68s). The probe was re-run with a
one-off re-entrancy guard local to the edit (`let __probeClosing = false`,
set before the extra call and checked so a recursive re-entry into
`closeDialog` is a no-op); only that second run produced the recorded
result. This is a probe-construction detail, not a defect: the edit was
probe-only and fully reverted, and the committed production file carries
neither the guard-defeat nor the re-entrancy guard.

⁷ **Finding — the task-5 brief's prescribed Stage-2 edit for site 15 does
not defeat the guard this assertion actually depends on.** The brief names
"make the leave control call `confirmLeave()` directly" (the same shape as
sites 12/14: change the row Leave button's `onClick` from
`setPending({kind: 'leave'})` to `void confirmLeave()`, bypassing the
dialog-open gate). Applied literally and isolated (uncommitted, earlier
assertions removed so only `expect(memberService.updateRole)
.not.toHaveBeenCalled()` remains): the test still **passed** — `updateRole`
was never called, contradicting the brief's "expect red" prediction for
Stage 2. Reason: this test's data is a plain member (`needsSuccessor` is
false), and `confirmLeave` only reaches `updateRole.mutateAsync` inside
`if (needsSuccessor)` — bypassing the dialog-open gate changes nothing about
that internal check, so `updateRole` cannot fire via this edit regardless of
data. The guard this assertion actually depends on is that structural
`if (needsSuccessor)` check, not the dialog-confirmation gate the brief
names. Defeating the real guard instead — removing the `if (needsSuccessor)`
wrapper so `updateRole.mutateAsync({ userId: successorId, role: 'owner' })`
runs unconditionally — reaches the target assertion directly and fails with
the call recorded as `["f1", "", "owner"]` (the empty string is
`successorId`, unset for a plain member). The table's Stage 2 verdict and
Guard-defeated cell reflect this corrected edit, not the brief's literal
instruction; both edits were probe-only explorations, reverted with
`git checkout --` immediately after use, and no production behaviour was
altered by this task.

| 4 | `apps/web/src/lib/hooks/api/media.test.ts` | rejects an oversized file before any request, with a message naming the limit | `deps.initUpload` | `if (file.size > MEDIA_MAX_UPLOAD_BYTES)` early throw deleted in `performMediaUpload` (`src/lib/hooks/api/media.ts`) | green (probe inserted before the three `not.toHaveBeenCalled()` lines, after the pre-existing `await expect(...).rejects.toMatchObject(...)`; nothing yields between the probe and the plain assertions) | red⁸ (call recorded as `[{ contentType: 'image/jpeg', originalFilename: 'huge.jpg' }]`) | falsifiable | migrated to `expectNoCall` | n/a (was never vacuous) |
| 5 | `apps/web/src/lib/hooks/api/media.test.ts` | rejects an oversized file before any request, with a message naming the limit | `deps.putContent` | same guard as site 4 | green (same reasoning as site 4) | red⁸ (call recorded as `['m1', File {}]`) | falsifiable | migrated to `expectNoCall` | n/a (was never vacuous) |
| 6 | `apps/web/src/lib/hooks/api/media.test.ts` | rejects an oversized file before any request, with a message naming the limit | `deps.confirm` | same guard as site 4 | green (same reasoning as site 4) | red⁸ (call recorded as `['m1']`) | falsifiable | migrated to `expectNoCall` | n/a (was never vacuous) |
| 7 | `apps/web/src/lib/hooks/usePendingAttachments.test.ts` | does not call remove for an item that never uploaded | `mediaService.remove` | `if (target?.mediaId)` condition removed in `remove` (`src/lib/hooks/usePendingAttachments.ts`), calling `void mediaService.remove(target?.mediaId ?? 'probe')` unconditionally | green (probe after `act(() => result.current.remove(...))`, before the plain assertion; nothing yields in between) | red (call recorded as `['probe']`) | falsifiable | migrated to `expectNoCall` | n/a (was never vacuous) |
| 8 | `apps/web/src/lib/hooks/usePendingAttachments.test.ts` | commit disarms the unmount cleanup and returns the media ids | `mediaService.remove` | `if (committedRef.current) { return; }` disarm deleted from the unmount `useEffect` | green (probe after `unmount()`, before the plain assertion; nothing yields in between) | red (call recorded as `['m1']`) | falsifiable | migrated to `expectNoCall` | n/a (was never vacuous) |
| 28 | `apps/web/src/lib/hooks/api/media.test.ts` | revokes the previous URL exactly once when the id changes, and the new URL survives | `revokeObjectURL` (**With** `secondUrl`) | see finding⁹ — the brief's prescribed "revoke `entry?.url` instead of `objectUrl`" edit is a structural no-op; the guard actually defeated is a stray immediate `URL.revokeObjectURL(objectUrl)` added right after `setEntry(...)` in `useMediaContentUrl`'s effect | green (probe carrying `secondUrl`, inserted after the two pre-existing positive assertions, before the target assertion; nothing yields in between) | red⁹ (3 calls; the 3rd recorded as `secondUrl`) | falsifiable | migrated to `expectNoCallWith`, the two positive assertions above it left in place | n/a (was never vacuous) |

⁸ **Finding — Stage 2 for sites 4–6 initially lands on an earlier line.** In
the full (unmodified) test, deleting the `if (file.size >
MEDIA_MAX_UPLOAD_BYTES)` guard makes the pre-existing `await
expect(performMediaUpload(bigFile, deps)).rejects.toMatchObject({ status:
413, ... })` assertion fail first: `deps.initUpload`/`putContent`/`confirm`
are bare `vi.fn()`s with no resolved value, so `media.id` is read off
`undefined` (`media` resolves to `undefined`) and the function rejects with a
`TypeError` instead of the expected `ApiError` shape, without ever reaching
the three `not.toHaveBeenCalled()` lines. Per migration-context.md's
"Stage-2 evidence quality" note, that is weaker evidence. Strengthened in an
isolated, uncommitted copy of the test: `deps` given resolved values
(`initUpload` → `{ id: 'm1' }`, `putContent`/`confirm` → `{}`) and the
`.rejects.toMatchObject(...)` line replaced with `await
performMediaUpload(bigFile, deps).catch(() => undefined);`, then each of the
other two `not.toHaveBeenCalled()` lines removed in turn so only the target
spy's assertion remained. With the guard deleted: `deps.initUpload` fails
directly, call recorded as `[{ contentType: 'image/jpeg', originalFilename:
'huge.jpg' }]`; `deps.putContent` fails with `['m1', File {}]`; `deps.confirm`
fails with `['m1']`. Every one of these calls is **synchronous** — the guard
being gone means `deps.initUpload` is invoked the instant `performMediaUpload`
runs, before its own first `await` even suspends — which is exactly why
Stage 1 (a microtask probe with nothing yielding between it and the plain
assertions, per the human ruling) came back green while Stage 2 came back
red: combining-table row 2 ("falsifiable only because the defeated guard
fires synchronously"), not vacuous, the same shape as sites 1/2 and 24–27
above. Both the mock-strengthening and the guard deletion were probe-only,
reverted (`git checkout --` for the production file; the strengthened test
copy was never committed) immediately after use.

⁹ **Finding — the task-6 brief's prescribed Stage-2 edit for site 28 does not
defeat the guard this assertion depends on; it is a structural no-op, not
merely a fragile pass.** The brief names: in `useMediaContentUrl`'s effect
cleanup, change `return () => URL.revokeObjectURL(objectUrl)` to instead
revoke `entry?.url` (falling back to `objectUrl`), "so the new URL is revoked
too." Applied literally: the full test still **passed unchanged** — no
observable difference from the original. Verified why with temporary
`console.log` instrumentation of the effect (probe-only, reverted): `entry`
inside the cleanup closure is `null` at the moment *every* cleanup for this
hook is created, for both URLs. React Query returns `data: undefined` for a
freshly-keyed id before its fetch resolves, and the hook's
`if (!data) { setEntry(null); ...; return; }` branch always runs in between
two different ids — committing `entry = null` before the *next* id's effect
creates its own object URL — so `entry?.url ?? objectUrl` provably falls back
to `objectUrl` every time, identical to the original code, for any sequence
of id changes. More fundamentally: React guarantees a given effect's cleanup
always runs strictly before the *next* effect body for the same hook slot,
so no closure- or ref-based rewrite of "what the cleanup reads" can ever
observe a value written by a later effect run — and in this specific test
(no `unmount()`, only one id change), the cleanup that would revoke the
*second* url never fires at all before the assertion executes, regardless of
what it reads. The guard this assertion actually depends on is therefore not
"what does the cleanup read" but "does anything revoke the new url before its
own effect's teardown" — defeated instead by adding a stray immediate
`URL.revokeObjectURL(objectUrl)` right after `setEntry(...)` inside the
effect (a plausible copy/paste-style regression: "revoke on every allocation"
instead of "revoke on cleanup only"). Run against the full (otherwise
unmodified) test, this edit fails at the earlier
`expect(revokeObjectURL).toHaveBeenCalledTimes(1)` line (3 calls, not 1) —
weaker evidence per migration-context.md's note. Strengthened in an isolated,
uncommitted copy of the test with the two preceding positive assertions
removed, leaving only `expect(revokeObjectURL).not.toHaveBeenCalledWith(secondUrl)`:
fails directly, 3 calls recorded, the 3rd matching `secondUrl` exactly. Both
the brief's literal edit and the corrected one were probe-only explorations,
reverted with `git checkout --` immediately after use; the committed
production file carries neither, and the table's Guard-defeated cell reflects
the corrected edit, not the brief's literal instruction — the same pattern as
footnote 7's site 15 correction.

| 17 | `apps/web/src/components/features/vehicles/CategoryCombobox.test.tsx` | creates a category and selects the id the server returned, not anything derived locally | `onChange` (**With** `'Skid Plate'`) | see finding¹⁰ — `handleSelect(created.id)` → `handleSelect(trimmed)` in `handleCreate`'s try arm (`CategoryCombobox.tsx`), i.e. select the locally-derived name instead of the server id | red (probe carrying `'Skid Plate'` inserted right after the create-item click, before the pre-existing `expect(mutateAsync)...` and `await waitFor(...)` lines, both of which already yield ahead of the assertion) | red¹⁰ (onChange called with `['Skid Plate']`, after strengthening) | falsifiable | migrated to `expectNoCallWith`, the positive pairing (`await waitFor(() => expect(onChange).toHaveBeenCalledWith('server-assigned-id'))`) left in place per the brief | n/a (was never vacuous) |
| 18 | `apps/web/src/components/features/vehicles/CategoryCombobox.test.tsx` | surfaces a toast and selects nothing when creation fails | `onChange` | `onChange(trimmed)` added to the `catch (err)` arm of `handleCreate` (`CategoryCombobox.tsx`) | red (probe after the create-item click, before the pre-existing `await waitFor(() => expect(toast.error)...)`; 1 call) | red (onChange called with `['Skid Plate']`, target assertion hit directly, no earlier-line interference) | falsifiable | migrated to `expectNoCall` | n/a (was never vacuous) |
| 19 | `apps/web/src/components/features/vehicles/detail/VehicleRecordsTable.test.tsx` | disables load more while a page is in flight | `onLoadMore` | `disabled={isFetchingNextPage}` removed from the Load More button (`VehicleRecordsTable.tsx`) | green (probe after the click, before the plain assertion; nothing yields in between) | red¹¹ (onLoadMore called; target assertion hit directly after strengthening) | falsifiable | migrated to `expectNoCall` | red / red |
| 20 | `apps/web/src/components/features/vehicles/dialogs/PhotoGalleryDialog.test.tsx` | still reports success when only the object cleanup fails | `toast.error` | see finding¹² — the brief's named location (`PhotoGalleryDialog.tsx`'s `handleRemove`) has no cleanup call to move; the swallow is one layer down, in `useRemoveVehiclePhoto`'s `mutationFn` (`src/lib/hooks/api/media.ts`) — its inner `try { await mediaService.remove(mediaId); } catch { ... }` removed so the rejection propagates | red (probe after the confirm-remove click, before the pre-existing `await waitFor(() => expect(toast.success)...)`; 1 call) | red (toast.error called with `'media-service unavailable'`, target assertion hit directly) | falsifiable | migrated to `expectNoCall(vi.mocked(toast.error), ...)` | n/a (was never vacuous) |
| 21 | `apps/web/src/components/features/vehicles/dialogs/PhotoGalleryDialog.test.tsx` | does not remove anything until the confirmation is accepted | `removeMedia` | see finding¹³ — remove-photo button's `onClick` changed from `setPendingRemoval(...)` to `void handleRemove(ref.attributes.mediaId)` directly (`PhotoGalleryDialog.tsx`), bypassing the confirmation | green (probe after the cancel click, before the plain assertion; nothing yields in between) | red¹³ (removeMedia called with `['v1', 'm2']`, target assertion hit directly after strengthening) | falsifiable | migrated to `expectNoCall` | red / red |
| 22 | `apps/web/src/components/features/vehicles/dialogs/PhotoGalleryDialog.test.tsx` | does not remove anything until the confirmation is accepted | `removeObject` | same guard as site 21¹³ | green (same reasoning as site 21) | red¹³ (removeObject called with `['m2']`, target assertion hit directly after strengthening) | falsifiable | migrated to `expectNoCall` | red / red |
| 23 | `apps/web/src/components/features/vehicles/maintenance/MaintenanceRecordForm.test.tsx` | rejects a description over 200 characters | `onSubmit` | see finding¹⁴ — `.max(200, 'Description must be 200 characters or fewer')` relaxed to `.max(10000)` in `maintenanceRecordSchema` (`src/lib/schemas/maintenanceRecord.ts`) | red (probe after the submit click, before the pre-existing `await waitFor(() => expect(screen.getByText(/200 characters or fewer/i))...)`; 1 call) | red¹⁴ (onSubmit called, target assertion hit directly after strengthening) | falsifiable | migrated to `expectNoCall` | n/a (was never vacuous) |

¹⁰ Stage 2 for site 17: in the full (unmodified) test, changing
`handleSelect(created.id)` to `handleSelect(trimmed)` in `handleCreate`'s try
arm makes the pre-existing positive pairing fail first —
`await waitFor(() => expect(onChange).toHaveBeenCalledWith('server-assigned-id'))`
times out, because `onChange` is now called with `'Skid Plate'` instead,
never with `'server-assigned-id'`. Per migration-context.md's Stage-2
evidence-quality note, that is weaker evidence: the assertion under test
(`not.toHaveBeenCalledWith('Skid Plate')`) is never reached. Strengthened in
an isolated, uncommitted copy of the test: the positive pairing relaxed to
`expect(onChange).toHaveBeenCalled()` (proving only that *some* selection
happened, not which one); with the guard defeated, the target assertion then
fails directly — onChange recorded as called with `['Skid Plate']`. The
committed migration keeps the original, un-relaxed positive pairing (per the
brief: "leave the positive assertion in place") — the relaxed variant was
probe-only and never committed.

¹¹ Stage 2 for site 19: in the full (unmodified) test, removing
`disabled={isFetchingNextPage}` also makes the earlier
`expect(button).toBeDisabled()` line fail first (the button that used to be
disabled from render no longer is), before the click and the target
`onLoadMore` assertion are ever reached. Strengthened in an isolated,
uncommitted copy with that earlier line removed: the click then invokes
`onLoadMore` synchronously (its `onClick` is the prop directly, not a
promise continuation), and the target assertion catches it directly. This is
combining-table row 2 (green/red — "falsifiable only because the defeated
guard fires synchronously"), the same shape as sites 1/2 and 4–6 from
earlier tasks, not Stage-1/Stage-2 vacuous (green/green); the re-probe below
was still run for the same reason those earlier tasks ran theirs — extra
assurance on a fragile site.

¹² Finding — the task-7 brief's Stage-2 location for site 20 does not hold
the guard. The brief names `PhotoGalleryDialog.tsx`'s `handleRemove`, saying
"the object-cleanup failure is swallowed; move the cleanup call inside the
try." But `handleRemove` does not call the cleanup itself — it only calls
`await removePhoto.mutateAsync(mediaId)`, already inside its own try. The
swallow is one layer down, inside `useRemoveVehiclePhoto`'s `mutationFn` in
`src/lib/hooks/api/media.ts`: `await vehicleMediaService.removeMedia(...);
try { await mediaService.remove(mediaId); } catch { /* reference is gone;
the object is media-service's to reap */ }`. Because that inner catch
swallows the rejection, `removePhoto.mutateAsync` never rejects, so
`handleRemove`'s own catch (and its `toast.error`) can never fire, regardless
of anything done to `PhotoGalleryDialog.tsx`. The real guard: delete
`media.ts`'s inner try/catch so the rejection propagates through
`mutateAsync` into `handleRemove`'s catch. Verified against the full
(unmodified) test: this edit fails directly on the target assertion
(`toast.error` called with `'media-service unavailable'`), no earlier-line
interference. Probe-only; reverted with `git checkout --` immediately after
use. The table's Guard-defeated cell reflects this corrected location, not
the brief's literal instruction — the same pattern as footnote 7's site 15
and footnote 9's site 28 corrections in earlier tasks.

¹³ Stage 2 for sites 21 and 22: in the full (unmodified) test, changing the
remove-photo button's `onClick` to call `void handleRemove(mediaId)` directly
(bypassing `setPendingRemoval`) makes the earlier
`await user.click(await screen.findByRole('button', { name: /cancel/i }))`
line fail first — the `AlertDialog` never opens (its `open` is
`pendingRemoval !== null`, and `pendingRemoval` is now never set), so no
Cancel button ever appears and `findByRole` times out. Neither target
assertion (`removeMedia`/`removeObject` not called) is reached. Strengthened
in an isolated, uncommitted copy of the test: the now-structurally-moot
cancel-click line removed (once confirmation is bypassed at the trigger
itself, cancelling has nothing left to cancel), leaving only
`beginRemovingSecondPhoto(user)` followed by each target assertion in turn.
With the guard defeated: `removeMedia` fails directly, recorded as called
with `['v1', 'm2']`; `removeObject` fails directly, recorded as called with
`['m2']`. This is combining-table row 2 (green/red — the guard-defeat fires
synchronously off the remove-photo click itself), not vacuous; the
re-probes below were still run, matching the precedent of earlier tasks'
fragile sites.

¹⁴ Stage 2 for site 23: the brief's prescribed edit
(`maintenanceRecordSchema`'s `description.max(200, ...)` relaxed to
`.max(10000)`) is confounded by this test's own render — it never selects a
category, so `categoryId` stays at its `''` default and independently fails
`min(1, 'Category is required')` regardless of description length. Run
against the full (unmodified) test with the guard applied, the earlier
`await waitFor(() => expect(screen.getByText(/200 characters or fewer/i)).toBeInTheDocument())`
line times out (the visible error is now "Category is required", never the
description one), and `onSubmit` is confirmed (via
`onSubmit.mock.calls.length === 0`) never to fire — not because the
description guard failed to bite, but because the independently-invalid
category blocks submission on its own. Strengthened in an isolated,
uncommitted copy of the test: a category-selection step added (click the
combobox, click "Oil Change") before typing the over-length description,
isolating the description guard from the confounding field. With that
change, defeating the guard let the form submit with the 201-character
description, and the target assertion caught it directly — `onSubmit`
called once with the full string. The committed migration does not add the
category-selection step (that would change the test's own scenario); the
Stage-2 disposition above is drawn from the strengthened, uncommitted
variant per migration-context.md's evidence-quality note, the same
treatment as footnote 8's sites 4–6.

| 10 | `apps/web/src/components/features/dashboard/useDashboardWidgets.test.ts` | does not call saveLayout when addWidget is invoked while the layout query is still loading | `dashboardService.saveLayout` | `if (isLoading) return;` early return deleted in `addWidget` (`useDashboardWidgets.ts`) | green (dispatch point coincides with the assertion — `act(() => result.current.addWidget(...))` yields nothing before it) | green¹⁵ (isolated from an earlier-line confound — see finding¹⁵) | vacuous | migrated to `expectNoCall` | red / red¹⁵ |
| 11 | `apps/web/src/components/features/dashboard/useDashboardWidgets.test.ts` | does nothing at the list boundaries | `dashboardService.saveLayout` | see finding¹⁵ — the brief's prescribed edit (both named boundary early returns deleted) is a structural no-op; the guard actually gating the call is each function's secondary bounds check (`if (!above \|\| !current) return;` / `if (!current \|\| !below) return;`) | green (dispatch point coincides with the assertion — the two `act(...)` calls yield nothing before it) | green¹⁵ (isolated from the same earlier-line confound as site 10 — see finding¹⁵) | vacuous | migrated to `expectNoCall` | red / red¹⁵ |
| 29 | `apps/web/src/lib/hooks/api/members.test.ts` | useRemoveMember does not mint a token when removing another member | `mintAccessToken` | `if (!isSelf) return;` early return deleted in `useRemoveMember`'s `onSuccess` (`members.ts`) | red (probe after the `await act(async () => { result.current.mutate(...) })`, before the pre-existing `await waitFor(() => expect(result.current.isSuccess)...)`; 1 call) | red (`mintAccessToken` called once, target assertion hit directly) | falsifiable | migrated to `expectNoCall` | n/a (was never vacuous) |
| 30 | `apps/web/src/lib/hooks/api/members.test.ts` | useUpdateMemberRole does not mint a token | `mintAccessToken` | see finding¹⁶ — `useUpdateMemberRole` never calls `mintAccessToken` under any code path; there is no "equivalent guard" to delete | red (same probe placement as site 29; 1 call) | n/a (unconstructible — see finding¹⁶) | unprobeable | migrated to `expectNoCall` (uniformity, FR-HELPER-3) | n/a (was never vacuous) |
| 31 | `apps/web/src/lib/hooks/api/users.test.ts` | does not fire a request when there are no ids | `userService.listByIds` | `enabled: sorted.length > 0` → `enabled: true` in `useUsers` (`users.ts`) | green (dispatch point coincides with the assertion — `renderHook(...)` yields nothing before it, and the `it` callback was synchronous) | red (call recorded as `[[]]`, target assertion hit directly) | falsifiable | migrated to `expectNoCall`, `it` made `async` | n/a (was never vacuous) |
| 32 | `apps/web/src/lib/hooks/api/vehicleRecords.test.ts` | loadMore calls fetchNextPage only on sources that still have a next page | `fuel.fetchNextPage` | `if (fuel.hasNextPage)` condition dropped in `loadMore` (`vehicleRecords.ts`), calling `fetchFuelNextPage()` unconditionally | green (dispatch point coincides with the assertion — `result.current.loadMore()` yields nothing before it, and the `it` callback was synchronous) | red (call recorded with no arguments, once; target assertion hit directly, neighbouring `toHaveBeenCalledTimes(1)` assertions on `maintenance`/`mileage` unaffected) | falsifiable | migrated to `expectNoCall`, `it` made `async` | n/a (was never vacuous) |
| 33 | `apps/web/src/pages/VehiclesPage.test.tsx` | keeps the dialog open with inline errors when required fields are blank | `vehicleService.createInFleet` | `vehicleSchema`'s `make`/`model` relaxed to `z.string().trim()` and `year` made `.optional()` (`src/lib/schemas/vehicle.ts`) — see finding¹⁷ for the earlier-line confound | red (probe after the submit click, before the pre-existing `expect(await screen.findByText('Make is required'))...`; 1 call) | red¹⁷ (call recorded as `["f1", { make: "", model: "", ... }]`, after strengthening) | falsifiable | migrated to `expectNoCall(vi.mocked(...), ...)` | n/a (was never vacuous) |
| 34 | `apps/web/src/pages/VehiclesPage.test.tsx` | closes on %s without creating a vehicle (`it.each` — Escape / the close button / Cancel) | `vehicleService.createInFleet` | see finding¹⁸ — **not** FR-TRIAGE-4, contrary to the task-9 brief's prediction; the brief's prescribed edit (swallow the error and close the dialog in `handleCreate`'s catch) is a structural no-op for this test, since none of the three dismiss paths ever invoke `handleCreate`. The guard that actually gates this assertion is architectural (none of Escape/Close/Cancel call the create handler at all); defeated by wiring `onOpenChange` (catches Escape and the Close button) and `onCancel` (catches Cancel) in `VehiclesPage.tsx` to each also call `createVehicle.mutateAsync(...)` on close | red (probe after `await dismiss();`, before the pre-existing `await waitFor(...)`; 1 call, for all three parameterized cases) | red¹⁸ (all three parameterized cases fail directly on the target line, each recorded as one call with all-`undefined` attributes) | falsifiable | migrated to `expectNoCall(vi.mocked(...), ...)` | n/a (was never vacuous) |
| 35 | `apps/web/src/pages/VehiclesPage.test.tsx` | closes on an outside pointer-down without creating a vehicle | `vehicleService.createInFleet` | `onInteractOutside` added to `DialogContent` in `VehiclesPage.tsx`, calling `createVehicle.mutateAsync(...)` | red (probe after `fireEvent.pointerDown(document.body)`, before the pre-existing `await waitFor(...)`; 1 call) | red (call recorded as `["f1", { make: undefined, model: undefined, ... }]`, target assertion hit directly, no earlier-line interference) | falsifiable | migrated to `expectNoCall(vi.mocked(...), ...)` | n/a (was never vacuous) |
| 36 | `apps/web/src/pages/admin/AdminFleetsPage.test.tsx` | opens the confirmation dialog instead of purging directly | `createPurgeMutate` | `BlastRadiusPanel`'s `onPurge` in `AdminFleetsPage.tsx` changed from `() => setConfirmOpen(true)` to call `createPurge.mutate(...)` directly, bypassing the confirmation | green (dispatch point coincides with the assertion — nothing yields between the purge-button click and the very next line, which is the target assertion itself) | red (call recorded as `[{ scope: 'fleet', target_type: 'fleet', target_id: 'f1', confirmation: 'Test Fleet' }, { onSuccess }]`, target assertion hit directly) | falsifiable | migrated to `expectNoCall` (local `vi.fn()`, no `vi.mocked()` needed — see finding¹⁹) | n/a (was never vacuous) |
| 9 | `apps/web/src/components/features/activity/ActivityFeed.test.tsx` | renders the empty state without asking for any names | `listByIds` | `enabled: sorted.length > 0` → `enabled: true` in `useUsers` (`src/lib/hooks/api/users.ts`) | green (dispatch point coincides with the assertion — `renderWithProviders(...)` yields nothing before it, and the two lines between it and the target assertion are both synchronous, non-yielding `expect()`s) | red (call recorded as `[[]]`, target assertion hit directly) | falsifiable | migrated to `expectNoCall` (local `vi.fn()` via `vi.hoisted`, no `vi.mocked()` needed — see finding¹⁹) | n/a (was never vacuous) |
| 37 | `apps/web/src/components/ui/input.test.tsx` | does not reach for a picker on non-picker types | `showPicker` | `const isPicker = !!type && PICKER_TYPES.has(type);` → `const isPicker = true;` in `input.tsx`, so a `type="number"` input also reaches `el.showPicker()` | green²¹ (probe placed immediately before the assertion, per the HUMAN RULING — nothing yields between the preceding `await user.click(...)` resolving and the assertion; contrary to the task-10 brief's prediction of red, see finding²¹) | red (`showPicker` called twice, target assertion hit directly) | falsifiable | exempted via inline `eslint-disable-next-line no-restricted-syntax` carrying the probe evidence (design OQ-4) — not migrated | n/a (was never vacuous) |
| 38 | `apps/web/src/lib/utils/download.test.ts` | revokes the object URL, but not before the click | `URL.revokeObjectURL` | `setTimeout(() => URL.revokeObjectURL(url), 0);` → bare `URL.revokeObjectURL(url);` in `download.ts`, so the revoke happens synchronously, before the assertion | green (assertion is synchronous in the same tick as the trigger; this is the *correct* verdict — the assertion means "not called synchronously", which is the point, per the task-10 brief) | red (`URL.revokeObjectURL` called once with `'blob:test-url'`, target assertion hit directly) | falsifiable | exempted via inline `eslint-disable-next-line no-restricted-syntax` carrying the probe evidence (design OQ-4) — not migrated; see finding²² for the fake-timer incompatibility that independently rules out migration | n/a (was never vacuous) |

¹⁵ Stage 2 for sites 10 and 11 (`useDashboardWidgets.test.ts`) is confounded
by an earlier line in the full (unmodified) test, in both directions (pre-
and post-migration).

Site 10: deleting `if (isLoading) return;` in `addWidget` does not make the
target assertion (`expect(dashboardService.saveLayout).not.toHaveBeenCalled()`,
pre-migration) fail — it makes the *preceding*
`expect(result.current.widgets).toEqual([])` line fail instead, because
`save()` writes `localWidgets` synchronously via `setLocalWidgets`, so
`widgets` already contains the new entry by the time that line runs; the
target assertion is never reached. Isolated in an uncommitted copy (the
preceding `widgets` line removed), the bare target assertion is confirmed to
still PASS even with the guard defeated — `saveLayout.mutate(...)` is
dispatched from a promise continuation the same way React Query's queries
are (migration-context.md), so nothing before a bare, synchronous
`expect()` can observe it. This confirms genuine green/green vacuity, not a
masked falsifiable site.

Site 11: the brief's prescribed edit (delete `if (idx === 0) return;` in
`moveUp` and `if (idx === widgets.length - 1) return;` in `moveDown`) is
itself a structural no-op — with only those two lines deleted, all 7 tests
still pass, because each function's own secondary bounds check
(`if (!above || !current) return;` / `if (!current || !below) return;`)
already discards the swap for exactly these boundary indices (there is no
element above index 0, none below the last index, by construction). Forcing
the swap through requires deleting that secondary check too; doing so
naively corrupts the array with `undefined` and crashes inside
`toWidgetInputs` (`Cannot read properties of undefined (reading 'type')`)
before `saveLayout.mutate` is ever called — the file going red from a
wholly unrelated `TypeError`, not the target assertion, is not counted as
evidence either way. Constructing a guard-defeat that reaches `saveLayout`
without crashing requires also filtering the corrupted slot out of the
array before saving (`next.filter((w): w is GridWidget => w != null)`) —
modeling a defensively-written-but-boundary-unchecked swap. With that in
place, the same earlier-line confound as site 10 recurs (the preceding
`expect(result.current.widgets.map(...)).toEqual(['w1', 'w2'])` line fails
first, since the swap now silently drops a widget instead of leaving the
list unchanged). Isolated the same way as site 10, the bare target
assertion again PASSES with the guard defeated, confirming green/green
vacuity by the same reasoning.

Post-migration re-probes for both sites: run against the full committed
file, Stage 1's probe is caught cleanly on both (the `widgets` assertions
above the target line make no reference to the spy, so nothing masks it —
`dashboardService.saveLayout` recorded as called once with `['__probe__']`
in each). Stage 2's guard-defeat, however, still trips the preceding
`widgets` assertion first in the full committed file (the migration does
not touch that line, and it is unaffected by the added flush). Isolated the
same way as the pre-migration check (temporarily removing the preceding
`widgets` line), the migrated `await expectNoCall(...)` independently
catches both: site 10 records `saveLayout` called once with
`['f1', [{ type: 'recent-activity', ... }]]`; site 11 records two calls,
both with the sole surviving `recent-activity` widget. The committed test
is not altered to remove the confounding line — that would change the
test's own scenario — so the Re-probe cells above record "red / red" drawn
from this isolated evidence, the same footnoted-evidence treatment as
footnote 14's site 23.

¹⁶ Site 30 (`useUpdateMemberRole does not mint a token`): the brief names
"the equivalent guard in useUpdateMemberRole" as `mintAccessToken`'s Stage-2
defeat, but `useUpdateMemberRole` (`members.ts`) never calls
`mintAccessToken` under any code path — a repo-wide
`grep -rn "mintAccessToken" apps/web/src` (excluding tests) shows exactly
one call site, inside `useRemoveMember`'s `onSuccess`. There is no
conditional to delete that would make the spy fire from this function; this
is FR-TRIAGE-4 (unconstructible), not a guard that merely doesn't bite in
the prescribed location (contrast footnote 12's site 20, where the real
guard existed one layer down). Stage 1 was red, so per the combining table
("any | unconstructible | FR-TRIAGE-4 | Stage 1 governs") the site is
recorded as **unprobeable** rather than falsifiable or vacuous — Stage 1
alone cannot certify what Stage 2 could not test. Migrated to
`expectNoCall` anyway for uniformity with its sibling row (site 29) and
with the FR-HELPER-3 precedent (footnote 1's sites 39/40).

¹⁷ Stage 2 for site 33: in the full (unmodified) test, relaxing
`vehicleSchema` (`make`/`model` to bare `z.string().trim()`, `year` to
`.optional()`) makes the pre-existing
`expect(await screen.findByText('Make is required')).toBeInTheDocument()`
line fail first — with the fields now valid, the form submits successfully,
the dialog closes, and neither validation message ever renders. Per
migration-context.md's Stage-2 evidence-quality note, that is weaker
evidence — the assertion under test is never reached. Strengthened in an
isolated, uncommitted copy of the test: the three preceding assertions
(the two `findByText`/`getByText` validation-message checks and the
`getByRole('dialog')` check) replaced with the helper's own flush
(`await act(async () => { await new Promise((resolve) => setTimeout(resolve, 0)); })`)
so the mutation has a chance to settle; with only the target
`not.toHaveBeenCalled()` line left, the guard defeat reaches it directly and
fails, call recorded as
`['f1', { make: '', model: '', year: undefined, ... }]`. Both the schema
relaxation and the isolated flush were probe-only, reverted with
`git checkout --` (production file) immediately after use; the committed
migration keeps the test's original preceding assertions unchanged.

¹⁸ **Finding — site 34 is NOT FR-TRIAGE-4, contrary to the task-9 brief's
prediction.** The brief names this as "the last remaining candidate
FR-TRIAGE-4 site" and prescribes making `handleCreate`'s catch swallow the
error and close the dialog, framing the guard as "the dialog's
stay-open-on-error behaviour." Applied literally
(`VehiclesPage.tsx`'s `catch (err) { ...; setOpen(false); }`): run against
the full, unmodified `it.each` test, none of the three parameterized cases
(Escape, the close button, Cancel) regress — all three still pass, 0 calls
recorded. This is a structural no-op for this assertion, not merely a guard
in the wrong spot: none of the three dismiss paths this test exercises ever
invokes `handleCreate` in the first place (Escape and the close button close
via Radix's `onOpenChange`, which only calls `setOpen(next)`; Cancel calls
`setOpen(false)` directly via `VehicleForm`'s `onCancel` prop), so a change
confined to `handleCreate`'s catch block has nothing to run. (The edit does
affect a *different*, unrelated test — "keeps the dialog open with the typed
values when the request fails" — which is not one of this task's assigned
sites and was left unmigrated by this edit's presence.)

The task's own instructions license searching for the real guard before
concluding unconstructible, the same as footnotes 7, 9, and 12 in earlier
tasks. Since the code truly never calls the create handler on any of these
three paths, "defeating the guard" here necessarily means adding a new call,
the same technique site 35's own prescribed edit already uses for the
outside-pointer-down path. Applied analogously: `onOpenChange` in
`VehiclesPage.tsx` changed to also call
`createVehicle.mutateAsync(toCreateAttributes({} as VehicleFormInput))`
whenever `!next` (closes via Escape and the close button, both routed
through `onOpenChange`), and `VehicleForm`'s `onCancel` prop changed the same
way (closes via Cancel, which bypasses `onOpenChange` entirely). Run against
the full, unmodified test: all three parameterized cases now fail directly
on the target `not.toHaveBeenCalled()` line, no earlier-line interference —
`waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())`
still passes on its own (the dialog does close), and each case's call is
recorded as one call with every attribute `undefined` (the dummy values
passed to the direct `mutateAsync` call). This is the same shape as site 35
(a plausible "the close path was wired to also submit" regression), just
applied to the dialog's built-in Escape/close-button/Cancel routes instead
of outside-pointer-down. Disposition: Stage 1 was independently red (see
table), so per the combining table this is **falsifiable** (red/red), the
same correction pattern as site 3 in Task 4 (see the FR-TRIAGE-4 section
below) and sites 15/20/28 in earlier tasks (footnotes 7, 9, 12) — a
prescribed guard that doesn't bite is not the same as unconstructible; the
real guard, once found, did bite. All edits (the brief's literal one and the
corrected one) were probe-only explorations, reverted with `git checkout --`
immediately after use; the committed production file carries none of them,
and the table's Guard-defeated cell reflects the corrected edit.

¹⁹ Neither `createPurgeMutate` (`AdminFleetsPage.test.tsx`, site 36) nor
`listByIds` (`ActivityFeed.test.tsx`, site 9) needed `vi.mocked()` in their
migration, despite both being wired into a `vi.mock(...)` factory for an
imported module. `AdminFleetsPage.test.tsx` declares
`const createPurgeMutate = vi.fn();` and returns it directly from the
`useCreatePurge` mock (`useCreatePurge: () => ({ mutate: createPurgeMutate,
isPending: false })`); the test never accesses it through a module
namespace, only as the bare local variable, so TypeScript already types it
as a `Mock` — no cast needed. `ActivityFeed.test.tsx` declares
`const { listByIds } = vi.hoisted(() => ({ listByIds: vi.fn() }));` and
wires that same reference into `userService.listByIds` inside the mock
factory; the test likewise only ever refers to the bare `listByIds` binding,
never `userService.listByIds`. In both cases the deciding factor is which
reference the test body actually uses, not whether the mock backs an
imported service module — `vi.mocked()` is needed only when the call site
itself is the module-qualified name (as with `vehicleService.createInFleet`
in sites 33–35, which the test does reference that way).

²⁰ Sites 36 and 9 are both design combining-table row 2 (green/red —
"falsifiable only because the defeated guard fires synchronously"), the same
shape as sites 1/2, 4–6, 19, and 21/22 from earlier tasks, not vacuous. In
both cases the call that the guard-defeat produces is dispatched
synchronously from a plain event handler (site 36's button `onClick` calls
`createPurge.mutate(...)` directly, not through a promise continuation; site
9's `renderWithProviders` mounts synchronously and React Query's `enabled`
gate is evaluated during that same synchronous render), so nothing yields
between the trigger and the bare assertion for Stage 1 to observe — but
Stage 2's edit still reaches the target assertion directly with no
earlier-line interference, unlike the confounded sites in footnotes 8/17.
Both were migrated to `expectNoCall` per FR-HELPER-3 regardless, the same
reasoning as every other fragile site in this record.

²¹ Site 37's Stage 1 came back green, contrary to the task-10 brief's
prediction of red ("Site 37 follows `await user.click(...)`, which yields —
expect red"). The HUMAN RULING in `migration-context.md` requires the probe
to be placed immediately after the trigger and before any yielding statement
between the trigger and the assertion; here there is no such intervening
statement — `expect(showPicker).not.toHaveBeenCalled()` directly follows the
resolved `await user.click(...)` with nothing in between that drains the
microtask queue — so per the ruling itself, "the probe sits immediately
before the assertion and green is the honest verdict." Confirmed empirically:
inserting `void Promise.resolve().then(() => showPicker('__probe__'))`
immediately before the assertion left the test passing (green). This does
not change the site's disposition: design combining-table row 2 (green S1 /
red S2 = "falsifiable only because the defeated guard fires synchronously")
still applies, which is exactly the "safe by construction" shape OQ-4
exempts — the mismatch is in the brief's prose, not in the site's
qualification for Group D.

²² Site 38's fake-timer incompatibility was independently confirmed (Task 10
Step 3, not merely asserted from the design doc): temporarily migrating the
assertion to `await expectNoCall(vi.mocked(URL.revokeObjectURL),
'URL.revokeObjectURL')` (with the enclosing `it` made `async`) and running
`npx vitest run src/lib/utils/download.test.ts --testTimeout=5000` produced
`Test timed out in 5000ms` — the helper's internal `setTimeout(0)` flush
never fires while `vi.useFakeTimers()` is active for the suite, so the
`await` never resolves. The migrated form was reverted with `git checkout --`
immediately after capturing this output; it is not part of the committed
diff.

## FR-TRIAGE-4 sites (unprobeable)

Task 4 attempted its one predicted FR-TRIAGE-4 candidate (site 3,
`VehiclePhotoThumbnail.test.tsx` — "says \"Photo unavailable\", not \"No
photo\", when a known photo cannot be fetched") and found it **is**
constructible via `networkMode: 'always'`. See footnote ³ on the site 3 row
above for the full investigation and disposition. No site in this task
belongs in this section.

Task 8 found one genuine site: site 30
(`apps/web/src/lib/hooks/api/members.test.ts`, "useUpdateMemberRole does not
mint a token") — `useUpdateMemberRole` never calls `mintAccessToken` under
any code path, so no production edit to an existing conditional can make the
spy fire from this function. See footnote ¹⁶ on the site 30 row above for
the full investigation and disposition.

Task 9 attempted its own predicted FR-TRIAGE-4 candidate (site 34,
`VehiclesPage.test.tsx` — `it.each` "closes on %s without creating a
vehicle") and, like Task 4's site 3, found it **is** constructible — wiring
`onOpenChange` and `onCancel` to also invoke the create handler makes all
three parameterized cases fail directly on the target assertion. See
footnote ¹⁸ on the site 34 row above for the full investigation and
disposition.

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
