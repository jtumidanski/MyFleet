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
| 19 | `apps/web/src/components/features/vehicles/detail/VehicleRecordsTable.test.tsx` | disables load more while a page is in flight | `onLoadMore` | `disabled={isFetchingNextPage}` removed from the Load More button (`VehicleRecordsTable.tsx`) | green (probe after the click, before the plain assertion; nothing yields in between) | red¹¹ (onLoadMore called; target assertion hit directly after strengthening) | falsifiable, fragile (design combining-table row 2: green/red) | migrated to `expectNoCall` | red / red |
| 20 | `apps/web/src/components/features/vehicles/dialogs/PhotoGalleryDialog.test.tsx` | still reports success when only the object cleanup fails | `toast.error` | see finding¹² — the brief's named location (`PhotoGalleryDialog.tsx`'s `handleRemove`) has no cleanup call to move; the swallow is one layer down, in `useRemoveVehiclePhoto`'s `mutationFn` (`src/lib/hooks/api/media.ts`) — its inner `try { await mediaService.remove(mediaId); } catch { ... }` removed so the rejection propagates | red (probe after the confirm-remove click, before the pre-existing `await waitFor(() => expect(toast.success)...)`; 1 call) | red (toast.error called with `'media-service unavailable'`, target assertion hit directly) | falsifiable | migrated to `expectNoCall(vi.mocked(toast.error), ...)` | n/a (was never vacuous) |
| 21 | `apps/web/src/components/features/vehicles/dialogs/PhotoGalleryDialog.test.tsx` | does not remove anything until the confirmation is accepted | `removeMedia` | see finding¹³ — remove-photo button's `onClick` changed from `setPendingRemoval(...)` to `void handleRemove(ref.attributes.mediaId)` directly (`PhotoGalleryDialog.tsx`), bypassing the confirmation | green (probe after the cancel click, before the plain assertion; nothing yields in between) | red¹³ (removeMedia called with `['v1', 'm2']`, target assertion hit directly after strengthening) | falsifiable, fragile (design combining-table row 2: green/red) | migrated to `expectNoCall` | red / red |
| 22 | `apps/web/src/components/features/vehicles/dialogs/PhotoGalleryDialog.test.tsx` | does not remove anything until the confirmation is accepted | `removeObject` | same guard as site 21¹³ | green (same reasoning as site 21) | red¹³ (removeObject called with `['m2']`, target assertion hit directly after strengthening) | falsifiable, fragile (design combining-table row 2: green/red) | migrated to `expectNoCall` | red / red |
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
