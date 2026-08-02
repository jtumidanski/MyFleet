# Add Vehicle Dialog — Implementation Context

Companion to [`plan.md`](./plan.md). Everything an implementer needs that is not
a task step: the shape of the code being changed, the decisions already settled,
the traps, and the commands.

Task: `task-016-add-vehicle-dialog`
Worktree: `/home/tumidanski/source/MyFleet/.worktrees/task-016-add-vehicle-dialog`
Branch: `task-016-add-vehicle-dialog`

---

## 1. What this task is

The Vehicles page creates vehicles through an inline `Card` form wedged between
the page header and the vehicle grid. Opening it pushes the whole grid down,
hides the trigger that opened it, and traps focus nowhere. This task replaces
that with a modal dialog.

The application has **no dialog primitive** — `apps/web/src/components/ui/`
contains button, card, form, input, label, select, skeleton, switch, textarea,
and nothing else — so the task also introduces the shared shadcn `Dialog`,
built to serve the edit and delete-confirmation modals that come later.

Create behaviour is preserved verbatim: same fields, same Zod schema, same
payload mapping, same toasts, same role gating, same API call. Nothing on the
backend moves.

## 2. Key files

### Being changed

| File | Current state | After |
| --- | --- | --- |
| `apps/web/package.json` | 5 `@radix-ui/*` deps, alphabetical | `@radix-ui/react-dialog@^1.1.0` added at the head of that run |
| `apps/web/src/components/ui/dialog.tsx` | does not exist | the shared primitive, 10 exports |
| `apps/web/src/components/ui/dialog.test.tsx` | does not exist | 13 behavioural tests |
| `apps/web/src/components/features/vehicles/VehicleList.tsx` | 35 lines, 2 props | + optional `emptyAction?: ReactNode`, two empty-state copy variants |
| `apps/web/src/components/features/vehicles/VehicleList.test.tsx` | does not exist | 4 tests |
| `apps/web/src/components/features/vehicles/VehicleForm.tsx` | Cancel never disabled (line 181) | `disabled={submitting}` on Cancel — one attribute |
| `apps/web/src/pages/VehiclesPage.tsx` | 79 lines, inline `Card` form behind `showForm` | dialog, two triggers, pending lock, focus redirect |
| `apps/web/src/pages/VehiclesPage.test.tsx` | does not exist | 21 tests |

### Read but not changed

| File | Why it matters |
| --- | --- |
| `apps/web/src/components/ui/select.tsx` | The reference primitive. `dialog.tsx` mirrors its `forwardRef` + `cn()` + `displayName` structure, and copies its `data-[state=*]` animation classes. |
| `apps/web/src/components/ui/button.tsx` | `Button` is a `forwardRef` over `<button>` (line 37), so `headerButtonRef` attaches without an adapter. |
| `apps/web/src/lib/hooks/api/vehicles.ts:47-57` | `useCreateVehicle` — `mutationFn` → `vehicleService.createInFleet(fleetId, attributes)`, `onSettled` invalidates `vehicleKeys.lists()`. Untouched. |
| `apps/web/src/services/api/VehicleService.ts:30,34` | `listByFleet(fleetId)` and `createInFleet(fleetId, attributes)` — the two methods the page tests mock. |
| `apps/web/src/services/api/BaseService.ts:4-7` | `ListResult<A> = { data: JsonApiResource<A>[]; meta?: PageMeta }` — the shape `listByFleet` mocks must return. |
| `apps/web/src/lib/schemas/vehicle.ts` | `vehicleSchema`. Required: make, model, year. Error strings the tests assert: `Make is required`, `Model is required`. Untouched. |
| `apps/web/src/test/renderWithProviders.tsx` | QueryClient with `retry: false` + `MemoryRouter`. Retries being off is what lets the error-path test resolve on the first rejection. |
| `apps/web/src/test/setup.ts` | jsdom polyfills (`localStorage`, `matchMedia`). The agreed home for any further polyfill, with a comment explaining why — none is expected. |
| `apps/web/src/components/AppLayout.test.tsx:10-13` | The `useAuth` mocking pattern this task copies. |
| `apps/web/src/components/features/vehicles/VehicleCard.test.tsx:12-14` | The service-boundary mocking pattern this task copies. |

## 3. Decisions already made — do not relitigate

| Decision | Where | Short reason |
| --- | --- | --- |
| The `Dialog` primitive is in scope | PRD §1 | It does not exist; the feature cannot ship without it. |
| It lives in `apps/web/src/components/ui/`, not `packages/ui-components` | PRD §2 | That package holds formatters and `StatusBadge`; UI primitives live in the app. |
| Controlled `open`, plain buttons, no `DialogTrigger` | design §4.4 | Two affordances open one dialog; `DialogTrigger` binds one element. |
| `dismissible?: boolean` on `DialogContent`, not ad-hoc handlers at each call site | design §4.3 | Three-part invariant (Escape + outside + close button) stated once, so the next modal cannot get two of three right. |
| The close button is **disabled**, not hidden, while locked | design §4.3 | Hiding it reflows the header mid-request; `disabled` is the honest AT signal. |
| The page *also* guards `onOpenChange` | design §4.3 | Backstop against any future Radix dismissal path. The prop is the contract; the guard is the belt. |
| Close button rendered **after** `children` | design §4.2 | DOM order decides Radix's initial autofocus. Reordering silently breaks FR-4.3. |
| Scroll container is an inner wrapper, not the content box | design §4.2 | shadcn upstream scrolls the box, which scrolls the absolutely-positioned close button out of view. |
| Animation classes copied from `select.tsx` even though they generate no CSS | design §4.5 | Literal consistency with the stated reference; both light up together the day `tailwindcss-animate` lands. Adding the plugin here would retroactively animate Select. |
| No unsaved-changes prompt; dismissing discards silently | PRD FR-2.4 | — |
| No mobile drawer variant | PRD §2 | The app is not mobile-optimised; one centred dialog serves all breakpoints. |
| Empty-state copy keys off `emptyAction` presence, not a role prop | design §6 | Makes it impossible to promise an action the component does not render. |
| No `AddVehicleDialog` extraction | design §5.5 | The page stays ~110 lines with one responsibility; the focus-redirect logic spans the header button *and* the dialog, so the seam would be awkward. Extract when edit becomes a dialog under `task-012`. |
| Edit-vehicle stays inline | PRD §2 | Owned by `task-012-vehicle-detail-redesign`; touching it here collides. |
| `VehicleForm`'s Cancel gains `disabled={submitting}` | design §5.3 | FR-2.5 and the PRD's "no VehicleForm changes" expectation cannot both hold. Narrowest possible change; `edit` mode renders the identical tree. |

## 4. Corrections this plan makes to `design.md`

Two, both verified empirically in this worktree before planning. The design's
*decisions* stand; only these two supporting claims were wrong.

### 4.1 Focus restoration (design §4.4) — consequential

The design states: "Radix restores focus to `document.activeElement` as captured
at open time, not to a registered trigger node, so a plain button that was
clicked gets focus back on close (FR-4.4)."

That is false. `@radix-ui/react-dialog` 1.1.23's modal content does:

```js
onCloseAutoFocus: composeEventHandlers(props.onCloseAutoFocus, (event) => {
  event.preventDefault();
  context.triggerRef.current?.focus();
})
```

`triggerRef` is populated **only** by `DialogTrigger`. Because this design
deliberately uses controlled mode with plain buttons, no trigger is registered,
`triggerRef.current` is `null`, and the unconditional `preventDefault()` also
suppresses `FocusScope`'s own restoration. A probe confirmed focus lands on
`document.body` on every close — FR-4.4 fails outright, not just in the
empty-state edge case.

**Fix, in the primitive so every future modal inherits it:** capture
`document.activeElement` in `onOpenAutoFocus` (which fires before focus moves
inside), restore it in `onCloseAutoFocus` *after* running the consumer's
handler and only if that handler did not `preventDefault()`. Verified working,
including the consumer-override path FR-4.5 needs.

Capturing at render time or in a `useEffect` does **not** work: the wrapper
component renders before the dialog opens, so it captures `document.body`. This
was tried and failed. The capture must happen in `onOpenAutoFocus`.

### 4.2 Dependency ordering (design §4.6) — cosmetic

The design says the new entry goes "alphabetically between `@radix-ui/react-label`
and `@radix-ui/react-select`". `dialog` sorts before `label`, so it heads the
`@radix-ui/*` run. FR-1.1's alphabetical requirement governs.

## 5. Verified environment facts

Established by probe against `@radix-ui/react-dialog` 1.1.23 + React 18.3 +
jsdom before writing the plan. These settle the risks design §10 flagged.

| Fact | Consequence |
| --- | --- |
| `^1.1.0` resolves to **1.1.23** | — |
| Close button after `children` ⇒ initial focus lands on the first form control | FR-4.3 needs no `autoFocus` and no `onOpenAutoFocus` override |
| `document.body.style.pointerEvents === 'none'` while open, **yet `userEvent.click`/`type` inside `DialogContent` work** | No `pointerEventsCheck` escape hatch needed. Design §9.1's claim confirmed. |
| `fireEvent.pointerDown(document.body)` dismisses through `DismissableLayer` | The correct way to test overlay dismissal; `userEvent` cannot drive the overlay |
| `preventDefault()` in `onEscapeKeyDown` + `onInteractOutside`, plus `disabled` on Close, blocks all three routes | The `dismissible` implementation is sound |
| No jsdom polyfill gap | `react-dialog`, unlike `react-select`, uses neither `hasPointerCapture` nor `scrollIntoView`. Risk row 1 in design §10 retired. |
| `tailwindcss-animate` absent (`tailwind.config.ts` `plugins: []`) | The `animate-in` / `zoom-in-95` classes are inert. Expected — do not "fix". |

## 6. Traps

1. **The label collision.** The header trigger, the empty-state trigger, **and
   `VehicleForm`'s submit button** all read "Add Vehicle". Page tests must
   disambiguate: the plan's `triggers()` helper filters to buttons *outside*
   `[role="dialog"]`, and the submit button is reached through
   `within(screen.getByRole('dialog'))`. A bare
   `getByRole('button', { name: 'Add Vehicle' })` throws on multiple matches or
   silently drives the wrong control.

2. **`getByText` on a bare text node.** The empty-state copy must be wrapped in
   a `<p>`. Left bare beside the action `div`, the parent's `textContent`
   includes the button label and
   `getByText('No vehicles yet. Add your first one to get started.')` matches
   nothing.

3. **Do not use `forceMount`.** Form reset between opens (FR-2.6) is free
   *because* Radix unmounts `DialogContent` on close, destroying the `useForm`
   instance. `forceMount` silently reintroduces stale input. No `reset()`, no
   remount `key`, no `useEffect` — just don't add `forceMount`.

4. **Handler props must stay destructured** in `DialogContent`. The trailing
   `{...props}` spread would otherwise overwrite the `dismissible` and
   focus-restoration wrappers with whatever a consumer passed.

5. **The success path must not route through `onOpenChange`.** `handleCreate`
   calls `setOpen(false)` directly, so the `isPending` guard cannot block it.
   Routing it through `onOpenChange` would risk the dialog refusing to close on
   success.

6. **The FR-4.5 condition is outcome-based, not DOM-based.** Use
   `openedFrom === 'empty' && created`, never
   `emptyButtonRef.current?.isConnected`. On success the dialog closes as soon
   as `mutateAsync` resolves while the list is still refetching, so the
   empty-state button is usually still attached when `onCloseAutoFocus` fires —
   a liveness check would pass exactly when it should fail.

7. **`VehicleDetailPage.tsx` must not appear in the diff.** It belongs to
   `task-012`. Task 6 Step 4 checks this explicitly.

## 7. Commands

Node on the default `PATH` is the Windows binary and fails on workspace symlinks
with `EISDIR`. Export the WSL node first, in every shell:

```sh
export PATH="$HOME/.nvm/versions/node/v24.12.0/bin:$PATH"
```

From the worktree root:

```sh
npm test                                  # all three workspaces
npm run lint                              # eslint --max-warnings 0
npm run format                            # prettier --write
npm run format:check                      # what CI runs
npm run build                             # tsc -b + vite build
npm run dev --workspace apps/web          # manual smoke check
```

Single test file (fastest inner loop), from `apps/web`:

```sh
npx vitest run src/pages/VehiclesPage.test.tsx
npx vitest run src/pages/VehiclesPage.test.tsx -t "focus after the empty state"
```

## 8. Test baseline

Measured in this worktree immediately before planning:

| Workspace | Files | Tests |
| --- | --- | --- |
| `apps/web` | 39 | **293** |
| `shared-ts` | 2 | 7 |
| `ui-components` | 1 | 10 |

Expected after this task: `apps/web` at **331 in 42 files** (+13 dialog,
+4 VehicleList, +21 VehiclesPage); the other two unchanged. Nothing pre-existing
may regress.

## 9. Dependency order between tasks

```
Task 1  Dialog primitive + @radix-ui/react-dialog
   │
   ├──────────────┐
   ▼              ▼
Task 2         Task 3  VehiclesPage dialog   ← also needs Task 2's emptyAction
VehicleList       │
emptyAction       ▼
               Task 4  pending lock (+ VehicleForm Cancel)
                  │
                  ▼
               Task 5  focus redirect after the empty state unmounts
                  │
                  ▼
               Task 6  full verification
```

Tasks 1 and 2 are independent of each other and could run in parallel. Task 3
needs both. Tasks 4 and 5 each modify `VehiclesPage.tsx` and its test file, so
they must run in order, after Task 3.

## 10. Out of scope

Edit-vehicle dialog (`task-012`), the fuel / mileage / maintenance inline forms,
any change to `vehicleSchema` or the form's field set, a mobile drawer variant,
unsaved-changes confirmation, moving `Dialog` into `packages/ui-components`,
adding `tailwindcss-animate`, and any backend change.
