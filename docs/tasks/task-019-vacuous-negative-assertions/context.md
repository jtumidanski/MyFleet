# Context — task-019 vacuous negative assertions

Companion to [plan.md](./plan.md). Everything an implementer needs that is not
in the plan's task steps: where things live, what was verified against source,
and which decisions are already settled.

Worktree: `/home/tumidanski/source/MyFleet/.worktrees/task-019-vacuous-negative-assertions`
Branch: `task-019-vacuous-negative-assertions`
Base commit at planning time: `b1c2252`
Issue: [#22](https://github.com/jtumidanski/MyFleet/issues/22)

---

## 1. The bug in one paragraph

React Query dispatches both queries and mutations from a **promise
continuation**, not synchronously. So `expect(spy).not.toHaveBeenCalled()`
placed immediately after a synchronous `render()` or `act()` runs *before* any
dispatch could have happened. The assertion passes whether or not the guard it
exists to prove works. It measures timing, not behaviour — and it will keep
reporting green through the exact regression it was written to catch.

This was confirmed once already, during the task-010 review (#20), on
`LoginPage.test.tsx`. That site was vacuous for **two independent reasons**:
the mutation dispatched asynchronously (timing), *and* the test's `beforeEach`
cleared `localStorage` so `updateThemePreference` short-circuited on the
missing token (reachability). Only the second is visible by reading the test.
That is why triage is by probe and why the probe has two stages.

---

## 2. Environment

| Thing | Value | How to check |
|---|---|---|
| Node | 22 (not on `PATH` by default) | `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22` |
| Vitest | 2.1.9 | `package-lock.json` |
| `@testing-library/react` | 16.3.2 | `package-lock.json` |
| Test env | `jsdom`, `globals: true`, setup `./src/test/setup.ts` | `apps/web/vite.config.ts` |
| ESLint | 9, flat config | `apps/web/eslint.config.js` |
| Prettier | 3.9.6, enforced by `make lint-check` | root `package.json` `format:check` |

`npm run lint` at the root fans out to workspaces `--if-present`; only
`apps/web` has a `lint` script (`eslint src --max-warnings 0`). That is why the
new rule cannot reach `packages/shared-ts` or `packages/ui-components` — a
residual gap, recorded not fixed.

Useful commands (all from `apps/web` unless noted):

```sh
npx vitest run <file>          # one test file
npx eslint src                 # the whole worklist
npx eslint <file>              # per-file gate used by every migration task
npx tsc -b                     # typecheck
make fe-test                   # root: all three JS workspaces
make ci                        # root: everything
```

---

## 3. Key files

### Created by this task

| Path | What |
|---|---|
| `apps/web/src/test/expectNoCall.ts` | `flushPending()`, `expectNoCall()`, `expectNoCallWith()` |
| `apps/web/src/test/expectNoCall.test.ts` | Proves the helper catches what a bare assertion misses |
| `docs/tasks/task-019-vacuous-negative-assertions/probe-results.md` | The FR-DOC record |

### Existing, relevant

| Path | Why it matters |
|---|---|
| `apps/web/src/test/renderWithProviders.tsx` | The house convention the helper follows: one small module per concern in `src/test/`, direct imports, **no barrel file**. Its `createTestQueryClient` sets `retry: false`. |
| `apps/web/src/test/objectUrl.ts` | Precedent for a `src/test/` module importing from `@testing-library/react`. |
| `apps/web/src/test/setup.ts` | Global stubs: in-memory `localStorage`, driveable `matchMedia`, `ResizeObserver`, Radix pointer-capture shims. |
| `apps/web/src/test/conventions.test.ts` | Existing cross-file invariant test. Uses `.not.toContain(...)`, which the new selector does **not** match. Untouched. |
| `apps/web/eslint.config.js` | The test-files block (currently only `languageOptions.globals`) is where the rule goes. Flat config is order-sensitive — the exemption block must come **after** it. |
| `apps/web/src/pages/LoginPage.test.tsx` | The known-good reference. Carries the hand-rolled #20 flush with a long explanatory comment; Task 3 replaces both flush and assertion with one `expectNoCall`. |

---

## 4. Settled decisions (do not relitigate)

| Question | Decision | Source |
|---|---|---|
| Helper name | `expectNoCall`, not `expectNoRequest` — 12 of 40 sites spy on non-network callbacks (`onSubmit`, `onChange`, `onLoadMore`, four `toast.*`) | design OQ-1 |
| `act` outside a React root | No branching needed. RTL sets `IS_REACT_ACT_ENVIRONMENT = true` **at import time**, so `act` is silent with nothing mounted. Import `act` from `@testing-library/react`, **never** from `react` | design F-1 / OQ-2 |
| Flush depth | Exactly the #20 flush (microtasks + one macrotask tick). It is a floor. Any deepening happens **in the helper**, and re-runs every migrated file | design OQ-3 |
| Group D | Exempt via inline `eslint-disable-next-line` carrying the probe result. An `eslint-disable` with evidence is more honest than a flush that provably does nothing | design OQ-4 |
| Failure message | Keep Vitest's built-in (it prints call count *and* every recorded argument list) and add the missing name via `spy.mockName(label)`. Do not hand-roll a message | design F-3 |
| Custom matcher instead of a free function | Rejected — needs `expect.extend` plus a declaration merge, and the natural spelling is the exact syntax the lint rule bans | design §7 |
| Fake timers as the flush | Rejected — most sites use `userEvent`, which hangs under fake timers unless every call site configures `advanceTimers` | design §7 |
| `waitFor` as the flush | Rejected — waiting for a *non*-event has no success condition, so it either returns immediately (proving nothing) or burns its timeout on all 38 sites | PRD §8, design §7 |
| A convention test instead of lint | Rejected — ESLint parses the AST (it distinguishes `toHaveBeenCalledTimes(0)` from `(1)`, which a regex would fumble), reports in-editor, and carries the teaching message | design §7 |

---

## 5. Verified against source during planning

Everything below was read, not recalled.

- **The inventory is exactly 40 sites across 20 files** — 38 bare
  `not.toHaveBeenCalled()` and 2 `not.toHaveBeenCalledWith(...)`. **Zero**
  `toHaveBeenCalledTimes(0)` sites exist, so that selector is purely
  preventive. (The PRD said 18 files; design F-5 corrected it to 20. Confirmed
  20.)
- **Five sites sit in synchronous `it(...)` callbacks** and must gain `async`
  when migrated: `VehicleCard.test.tsx`, `VehiclePhotoThumbnail.test.tsx` (the
  "No photo" case), `users.test.ts`, `vehicleRecords.test.ts`, and
  `download.test.ts` (which stays synchronous — it is exempt).
- **`download.test.ts` runs under `vi.useFakeTimers()`.** The helper's
  `setTimeout(0)` would never fire there and the test would hang until the
  Vitest timeout. This is a harder blocker than the design's stated reason for
  exempting the site, and it is why no site may be migrated into a fake-timer
  scope. `runtimeConfig.test.ts` also uses fake timers but has no negative
  assertions.
- **`download.test.ts`'s assertion is a deliberate ordering assertion**, not a
  "never happens" one — `vi.runAllTimers()` two lines below proves the revoke
  does occur. A flush there would destroy what the test measures.
- **`CategoryCombobox.test.tsx`'s `With` site already has its positive
  pairing** (`await waitFor(() => expect(onChange).toHaveBeenCalledWith('server-assigned-id'))`
  on the line above). The design flagged it as possibly missing one. It is not.
  `media.test.ts`'s `With` site is likewise pinned by
  `toHaveBeenCalledTimes(1)` above it.
- **`vi.mocked()` is needed at mocked-module call sites.** `expect()` accepts
  anything; `expectNoCall(spy: MockInstance)` does not, because an imported
  service method is typed as the real function. Local `vi.fn()` spies
  (`onChange`, `onSubmit`, `deps.initUpload`, `onLoadMore`) need no wrapper.
  `vi.mocked` is already the house idiom — the suite is full of
  `vi.mocked(mediaService.getContentBlob).mockResolvedValue(...)`.
- **Every spy in the inventory is anonymous** (`vi.fn()` in a `vi.mock`
  factory, or a local `vi.fn()`), so Vitest reports all of them as the literal
  string `"spy"`. The `label` argument is therefore passed at every site, not
  just some.

---

## 6. The guard map

The Stage-2 probe needs to know which production branch to defeat. Determined
by reading each module; the per-task probe tables in plan.md carry the detail.

| Production module | Guard |
|---|---|
| `components/features/vehicles/VehiclePhotoThumbnail.tsx` | `if (!mediaId)` early return → "No photo" placeholder |
| `lib/hooks/api/media.ts` (`useMediaContentUrl`) | `enabled: !!id`; effect cleanup `return () => URL.revokeObjectURL(objectUrl)` |
| `lib/hooks/api/media.ts` (`performMediaUpload`) | `if (file.size > MEDIA_MAX_UPLOAD_BYTES)` early throw |
| `lib/hooks/usePendingAttachments.ts` | `if (target?.mediaId)` in `remove`; `if (committedRef.current) return;` in the unmount effect |
| `lib/hooks/api/users.ts` | `enabled: sorted.length > 0` |
| `lib/hooks/api/members.ts` | `if (!isSelf) return;` in `onSuccess` |
| `lib/hooks/api/vehicleRecords.ts` | `if (fuel.hasNextPage)` in `loadMore` |
| `lib/hooks/api/auth.ts` | the no-token early return in `updateThemePreference` |
| `components/features/dashboard/useDashboardWidgets.ts` | `if (isLoading) return;` in `addWidget`; `if (idx === 0) return;` / `if (idx === widgets.length - 1) return;` in `moveUp`/`moveDown` |
| `components/features/settings/MemberList.tsx` | the `AlertDialog` confirmation gate in front of `confirmRemove` / `confirmPromote` / `confirmLeave` |
| `components/features/vehicles/dialogs/PhotoGalleryDialog.tsx` | the `AlertDialog` gate in front of `handleRemove`; the swallowed object-cleanup failure |
| `components/features/vehicles/CategoryCombobox.tsx` | `onChange(created.id)` in `handleCreate`; the `catch` arm that selects nothing |
| `components/features/vehicles/detail/VehicleRecordsTable.tsx` | `disabled={isFetchingNextPage}` on the load-more button |
| `lib/schemas/maintenanceRecord.ts` | `description: z.string()...max(200, 'Description must be 200 characters or fewer')` |
| `lib/schemas/vehicle.ts` | `make`/`model` `.min(1, '... is required')`, `year` number requirement |
| `pages/admin/AdminFleetsPage.tsx` | `setConfirmOpen(true)` in front of `createPurge.mutate` |
| `components/ui/input.tsx` | `const isPicker = !!type && PICKER_TYPES.has(type)` |
| `lib/utils/download.ts` | `setTimeout(() => URL.revokeObjectURL(url), 0)` |

**Two sites are expected to be unprobeable at Stage 2** (FR-TRIAGE-4 — Stage 1
governs, disposition recorded):

- `VehiclePhotoThumbnail.test.tsx` "says Photo unavailable, not No photo" — the
  guard is React Query **pausing the query while offline**. That is library
  behaviour; no edit under `apps/web/src` makes the fetch fire while
  `onlineManager` reports offline.
- `VehiclesPage.test.tsx` "keeps the dialog open with the typed values when the
  request fails" — the request already fired and failed; there may be no
  production edit that produces a *second* `createInFleet` call.

Both must still be attempted, and the outcome recorded either way.

---

## 7. Sequencing rationale

Task order follows design §6, and one step in it is deliberately
counter-intuitive:

**The lint rule lands in Task 2, before any migration.** It makes
`make lint-check` red across all 40 sites — and that red list *is* the
worklist. `npx eslint src -f compact | grep no-restricted-syntax` enumerates
exactly the un-migrated sites, by file and line, for free, and its count is a
free progress gate at the end of each migration task. The cost is that
`make lint-check` is red at intermediate commits on this branch. That is
acceptable on a single-PR branch, but it must never be mistaken for a finished
state: the branch is done only when Task 12 Step 4 shows `make ci` green.

Every migration task therefore gates on `npx eslint <its own files>` rather
than on the whole tree, so each task stays independently verifiable.

**Group E goes first** because `LoginPage` is the one site whose correct
behaviour is already established, so migrating it validates the helper against
a known answer before 18 more files depend on it.

**Group D goes last** because its exemptions are what finally turn
`npx eslint src` green.

---

## 8. Dependencies and blast radius

- **No new npm dependency.** `no-restricted-syntax` is a core ESLint rule;
  `act` and `MockInstance` come from packages already installed.
- **No production behaviour changes.** Every guard in the inventory is believed
  to work correctly today. This task changes only whether the tests would
  *notice* if one stopped working.
- **No Go service, no `packages/*`, no `deploy/k8s`.** `make ci` still runs
  them; a failure there is unrelated to this branch and should be reported, not
  fixed here.
- **`make fe-test` also runs `packages/shared-ts` and
  `packages/ui-components`.** Both currently contain zero negative call
  assertions.

---

## 9. Traps

| Trap | Consequence | Guard |
|---|---|---|
| Importing `act` from `react` instead of `@testing-library/react` | Act-scope warnings in every non-React test file; FR-HELPER-5 fails | The comment on the import in `expectNoCall.ts`; watch the output of Tasks 6 and 8 Step 5 |
| Migrating a site inside a `vi.useFakeTimers()` scope | The test hangs until the Vitest timeout | Only `download.test.ts` is affected, and it is exempt |
| Forgetting `async` on the five synchronous `it(...)` callbacks | `await` in a non-async function — a syntax error | Enumerated in plan.md's inventory table |
| Forgetting `vi.mocked()` on a mocked module method | `tsc -b` fails, possibly many files later | The transform table in plan.md |
| Stage-1 probe scaffolding surviving into the diff | It lives in test files, which this task legitimately modifies, so `git diff` against production sources will **not** catch it | `grep -rn "__probe__" apps/web/src` — a named step in every task |
| A stage-2 revert missed | A production file ships in the diff | `git status --short apps/web/src \| grep -v '\.test\.'` after each probe; Task 12 Step 1 |
| Recording the inventory by line number | The record orphans itself as sites are migrated | Key on file + `it(...)` title + spy |
| Adding a bespoke flush at a call site | The exact drift the helper exists to stop | If a site needs more, deepen the helper and re-run every migrated file |
| `mockName()` persisting for the rest of the file | A later unrelated failure on the same module-level mock prints an earlier label | Harmless — the label is always the spy's real name. Set it only when a label is passed |

---

## 10. Definition of done

From PRD §10, with the plan task that produces each:

- [ ] All 40 sites have a recorded probe verdict — Tasks 3-10, table completed Task 11
- [ ] Every vacuous site fixed and re-probed to red — re-probe step in Tasks 3-10
- [ ] `probe-results.md` committed with site / guard / verdict / fix / re-probe — Task 11
- [ ] `src/test/expectNoCall.ts` exported, with a test proving it catches what a bare assertion misses — Task 1
- [ ] Every non-exempt site uses the helper — Tasks 3-9, verified Task 12 Step 3
- [ ] Group D exemptions carry an inline comment citing their probe result — Task 10 Step 4
- [ ] ESLint rejects all three spellings — Task 2, demonstrated Task 11
- [ ] Lint rule demonstrated to fire, output recorded — Task 11 Steps 1-2
- [ ] No production source differs from `main` — Task 12 Step 1
- [ ] `make ci` passes — Task 12 Step 4
- [ ] Code review run before the PR (CLAUDE.md) — Task 12 Step 6
- [ ] Issue #22 referenced and closed by the PR — Task 12 Step 7
