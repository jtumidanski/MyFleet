# Add Vehicle Dialog — Design

Task: `task-016-add-vehicle-dialog`
PRD: [`prd.md`](./prd.md) (approved)
Created: 2026-08-02
Status: Proposed

---

## 1. Scope of this document

The PRD settles *what* ships. This document settles *how*: the shape of the new
`Dialog` primitive, where the open/close state lives, how the "not dismissible
while submitting" rule is expressed without leaking vehicle knowledge into a
shared primitive, how focus is redirected after the empty state disappears, and
how all of it is tested under jsdom.

Everything here is confined to `apps/web`. No Go service, contract, or manifest
is touched.

## 2. Baseline verified in this worktree

Read before designing, so the decisions below rest on the current tree rather
than on memory:

| Fact | Evidence |
| --- | --- |
| No dialog primitive exists | `apps/web/src/components/ui/` has button, card, form, input, label, select, skeleton, switch, textarea |
| `@radix-ui/react-dialog` is not installed | absent from `apps/web/package.json:11-30` and from `node_modules/@radix-ui/` |
| Inline form is a `Card` between header and grid | `VehiclesPage.tsx:58-72` |
| Header trigger hides while the form is open | `VehiclesPage.tsx:51` (`canWrite && !showForm`) |
| `VehicleList` is presentational, no auth import | `VehicleList.tsx:1-35` |
| `VehicleForm` already exposes `onSubmit` / `onCancel` / `submitting` | `VehicleForm.tsx:10-26` |
| `VehicleForm`'s Cancel is **not** disabled while submitting | `VehicleForm.tsx:180-184` |
| `tailwindcss-animate` is **not** installed | `tailwind.config.ts:78` (`plugins: []`), no `node_modules/tailwindcss-animate` |
| Tests render through `renderWithProviders` (QueryClient + MemoryRouter) | `src/test/renderWithProviders.tsx` |
| `src/test/setup.ts` already installs jsdom polyfills | `localStorage`, `matchMedia` |

Two of these rows are consequential and are addressed as explicit decisions
below: the missing animate plugin (§4.5) and the Cancel button (§5.3).

## 3. Architecture at a glance

```
VehiclesPage  ── owns: open state, opener identity, created flag, mutation, toasts
  ├─ header <Button>            (always rendered when canWrite)   ─┐
  ├─ <Dialog open onOpenChange> (controlled)                       │ both call
  │    └─ <DialogContent dismissible={!isPending} …>               │ openFrom(...)
  │         ├─ <DialogHeader><DialogTitle/><DialogDescription/>    │
  │         └─ <VehicleForm mode="create" …/>                      │
  └─ <VehicleList emptyAction={canWrite ? <Button …/> : undefined}/>
                                                                  ─┘
```

`dialog.tsx` knows nothing about vehicles. `VehicleList` knows nothing about
roles. `VehiclesPage` is the only module that knows both.

## 4. The `Dialog` primitive

### 4.1 File and exports

`apps/web/src/components/ui/dialog.tsx` exports the ten names in FR-1.2:
`Dialog`, `DialogTrigger`, `DialogPortal`, `DialogOverlay`, `DialogClose`,
`DialogContent`, `DialogHeader`, `DialogFooter`, `DialogTitle`,
`DialogDescription`.

It mirrors `select.tsx` structurally: `Dialog` / `DialogTrigger` /
`DialogPortal` / `DialogClose` are direct re-exports of the Radix primitives
(as `Select`, `SelectGroup`, `SelectValue` are at `select.tsx:6-8`); everything
that carries styling is a `React.forwardRef` wrapper that merges `className`
through `cn()` and assigns `displayName` from its Radix counterpart.
`DialogHeader` and `DialogFooter` are plain layout `div`s with literal
`displayName` strings — they have no Radix counterpart, which is the one
unavoidable departure from the `displayName` rule and matches shadcn upstream.

### 4.2 `DialogContent` structure

```
<DialogPortal>
  <DialogOverlay />                       fixed inset-0 z-50 bg-black/80
  <DialogPrimitive.Content
     fixed left-1/2 top-1/2 z-50 -translate-x-1/2 -translate-y-1/2
     flex max-h-[85vh] w-full max-w-lg flex-col
     gap-4 border border-border bg-background p-6 shadow-lg sm:rounded-lg>
    <div class="min-h-0 flex-1 overflow-y-auto">{children}</div>
    <DialogPrimitive.Close class="absolute right-4 top-4 …">
      <X class="h-4 w-4" /><span class="sr-only">Close</span>
    </DialogPrimitive.Close>
  </DialogPrimitive.Content>
</DialogPortal>
```

Three things about this layout are deliberate:

**The scroll container is an inner wrapper, not the content box.** FR-1.5 wants
`max-h-[85vh]` with internal scrolling. shadcn upstream puts `overflow-y-auto`
on the content element itself, which would scroll the absolutely-positioned
close button out of view on a tall form. Wrapping `children` in a
`min-h-0 flex-1 overflow-y-auto` div keeps the close button pinned while the
body scrolls. The cost is one extra DOM node and the fact that a `DialogFooter`
passed as a child scrolls with the body rather than sticking — acceptable,
since the create form's actions already live inside `VehicleForm` and scroll
with it today.

**The close button is rendered after `children`.** DOM order decides Radix's
initial autofocus target. Rendering the close button last makes the first
focusable element the first form control — the Nickname input — which is
exactly FR-4.3, with no `autoFocus` prop and no `onOpenAutoFocus` override.
Putting it first would satisfy the visual requirement and silently break the
focus requirement.

**The accessible name comes from an `sr-only` span**, not `aria-label`, so it
matches how the codebase already exposes icon-only affordances and is queryable
in tests as `getByRole('button', { name: /close/i })`.

### 4.3 Expressing "cannot be dismissed right now"

FR-2.5 requires Escape, overlay click, and the close button all to be inert
while a create request is in flight. Three ways to express that:

| Option | Mechanics | Verdict |
| --- | --- | --- |
| **A. `dismissible?: boolean` prop on `DialogContent`** (default `true`) | When `false`, the component calls `event.preventDefault()` in `onEscapeKeyDown` and `onInteractOutside`, and renders the close button `disabled` | **Chosen** |
| B. Consumer wires the handlers ad hoc | Page passes `onEscapeKeyDown` / `onInteractOutside` itself and a separate flag to hide the X | Rejected — three-part invariant re-derived at every call site; the next modal (delete confirmation) gets it wrong once |
| C. Guard only in the page's `onOpenChange` | `onOpenChange={(next) => { if (!next && isPending) return; setOpen(next); }}` | Rejected as the *sole* mechanism — it suppresses the state change but leaves the close button looking live and clickable |

Option A keeps the primitive generic: "this dialog is not dismissible right
now" is a modal concept, not a vehicle concept, and the delete-confirmation and
edit modals that follow will want the identical guard. `dismissible` composes
with any handlers the consumer also passes (the wrapper calls the consumer's
handler first, then applies its own `preventDefault` when `dismissible` is
false).

Belt and braces: the page *also* guards `onOpenChange` (option C) so that no
future dismissal path Radix grows can close the dialog mid-flight. The prop is
the user-visible contract; the guard is the backstop.

The close button is **disabled**, not hidden. Hiding it would reflow the header
for the duration of the request; disabling keeps the layout stable and is the
honest assistive-technology signal. `aria-disabled` is not needed — a real
`disabled` attribute on a `button` already conveys it.

### 4.4 Controlled root, plain buttons — no `DialogTrigger`

Two affordances open the same dialog (header button, empty-state button), and
`DialogTrigger` binds one element per dialog. The page therefore drives
`<Dialog open={open} onOpenChange={…}>` in controlled mode and both triggers are
ordinary `<Button onClick={…}>`.

This does not cost focus restoration. Radix restores focus to
`document.activeElement` as captured at open time, not to a registered trigger
node, so a plain button that was clicked gets focus back on close (FR-4.4).
`DialogTrigger` is still exported for future single-trigger modals.

### 4.5 Animation: mirror `select.tsx`, knowing the classes are inert

FR-1.7 asks for `data-[state=open]` / `data-[state=closed]` transitions
"consistent with how `select.tsx` animates". Verified fact: `select.tsx:66`
declares `data-[state=open]:animate-in`, `fade-in-0`, `zoom-in-95` and friends,
but `tailwind.config.ts:78` has `plugins: []` and `tailwindcss-animate` is not
installed — **those classes generate no CSS today.** The Select popover does not
actually animate.

| Option | Verdict |
| --- | --- |
| **Mirror `select.tsx`'s class list verbatim** | **Chosen** — literally consistent with the stated reference, zero visual risk, and the day someone adds `tailwindcss-animate` both primitives light up together |
| Add `tailwindcss-animate` in this task | Rejected — retroactively animates the existing Select, a visual change nobody asked for, in a task whose acceptance criteria are about dialogs |
| Hand-roll keyframes in `index.css` | Rejected — bespoke CSS diverging from the shadcn idiom, for motion that is not a requirement |

Recorded here so the reviewer does not read the animation classes as working
code, and so a future "add tailwindcss-animate" task has its rationale ready.

### 4.6 Dependency

`@radix-ui/react-dialog` `^1.1.0` is added to `apps/web/package.json`
`dependencies`, alphabetically between `@radix-ui/react-label` and
`@radix-ui/react-select` (line 15/16). Install regenerates the root
`package-lock.json`. Peer deps it pulls in — `react-remove-scroll`,
`aria-hidden`, `@radix-ui/react-focus-scope` — are already partly present via
`react-select`; no version conflict is expected, and the install must be run
with the WSL node (`export PATH="$HOME/.nvm/versions/node/v24.12.0/bin:$PATH"`),
per the PRD's build-hygiene note.

## 5. `VehiclesPage`

### 5.1 State

Three pieces of local state replace today's single `showForm`:

```ts
const [open, setOpen] = useState(false);
const openerRef  = useRef<'header' | 'empty'>('header');  // which button opened it
const createdRef = useRef(false);                          // did this session create?
const headerButtonRef = useRef<HTMLButtonElement>(null);
```

`openerRef` and `createdRef` are refs, not state: nothing renders from them,
they are read only inside `onCloseAutoFocus`, and making them state would cause
re-renders that change nothing. `openFrom(source)` sets `openerRef.current`,
clears `createdRef.current`, and sets `open` to `true`.

### 5.2 Lifecycle

| Event | Effect |
| --- | --- |
| Header button click | `openFrom('header')` |
| Empty-state button click | `openFrom('empty')` |
| Submit succeeds | `createdRef.current = true`; `toast.success('Vehicle added')`; `setOpen(false)` |
| Submit fails | `toast.error(apiError.message \|\| 'Could not add vehicle')`; dialog **stays open**, RHF state untouched |
| Cancel / X / Escape / overlay | `setOpen(false)` — but only when `!createVehicle.isPending` |
| Close (any reason) | `DialogContent` unmounts → `VehicleForm` unmounts → RHF state is discarded |

`handleCreate` keeps its current try/catch shape verbatim
(`VehiclesPage.tsx:36-45`); the only addition is `createdRef.current = true`
before `setOpen(false)`. `toCreateAttributes` (`VehiclesPage.tsx:14-25`) moves
not at all.

**Form reset (FR-2.6) is free.** Radix unmounts `DialogContent` on close when
`forceMount` is not used, so the `useForm` instance inside `VehicleForm` is
destroyed and rebuilt from its `defaultValues` on the next open. No `reset()`
call, no remount `key`, no `useEffect`. The design's only obligation is *not to
use `forceMount`* — which is why the PRD calls it out and why it is repeated
here.

### 5.3 The `VehicleForm` Cancel button — a necessary one-line change

The PRD's §7 expects `VehicleForm.tsx` to need no changes, while FR-2.5
requires its Cancel button to be unavailable during submit. Today Cancel is
never disabled (`VehicleForm.tsx:180-184`). These cannot both hold.

Resolution: add `disabled={submitting}` to the Cancel `Button`. This is the
narrowest possible change, is symmetric with the submit button one line below
it (`VehicleForm.tsx:185`), and touches no rendering branch — `edit` mode
renders the identical element tree, and its behaviour only changes while a save
is genuinely in flight, where an inert Cancel is correct anyway. The PRD
explicitly permits this ("if the implementation finds a change is unavoidable,
it must not alter the `edit`-mode rendering path"), and this does not.

Rejected alternative: have the page pass an `onCancel` that no-ops while
pending. The button would still look live, and a click that does nothing reads
as a broken app.

### 5.4 Focus after the empty state disappears (FR-4.5)

When the dialog is opened from the empty-state button and the create succeeds,
that button unmounts with the empty state, so Radix's focus restoration would
target a detached node and land on `document.body`.

`onCloseAutoFocus` on `DialogContent`:

```ts
onCloseAutoFocus={(e) => {
  if (openerRef.current === 'empty' && createdRef.current) {
    e.preventDefault();
    headerButtonRef.current?.focus();
  }
}}
```

The condition is `opener === 'empty' && created`, not a DOM liveness check such
as `emptyButtonRef.current?.isConnected`. Liveness is the wrong signal: on
success the dialog closes immediately after `mutateAsync` resolves, while the
list is still refetching, so the empty-state button is usually *still* attached
at the moment `onCloseAutoFocus` fires and the check would pass when it should
not. Outcome is deterministic; DOM timing is not.

Every other close path falls through to Radix's default restoration, which
satisfies FR-4.4 for both triggers — including cancelling out of an
empty-fleet dialog, where the empty-state button survives and correctly
regains focus.

### 5.5 Rejected: extracting an `AddVehicleDialog` component

A `components/features/vehicles/AddVehicleDialog.tsx` owning the mutation,
toasts, and `toCreateAttributes` would thin the page out. Rejected for now:

- The PRD's Service Impact table scopes the change to `VehiclesPage.tsx`;
  introducing a component and a second test file expands the diff without
  changing behaviour.
- The page after this change is ~95 lines with one responsibility. It is not
  yet large enough for the split to pay.
- The trigger-identity and focus-redirect logic (§5.4) spans the header button
  *and* the dialog, so a `AddVehicleDialog` boundary would have to take a
  `headerButtonRef` prop — an awkward seam that argues the split is premature.

The natural time to extract is when edit-vehicle also becomes a dialog
(`task-012`), at which point the two share enough shape to justify one.

## 6. `VehicleList`

```ts
interface VehicleListProps {
  vehicles: Vehicle[];
  isLoading: boolean;
  emptyAction?: ReactNode;   // rendered beneath the empty-state message
}
```

The empty state branches on `emptyAction` presence alone — no auth import, no
role prop:

- `emptyAction` present → "No vehicles yet. Add your first one to get started."
  plus the node, in a `mt-4` block.
- `emptyAction` absent → "No vehicles yet."

Keying copy off prop presence rather than a separate `canWrite` prop is
deliberate: it makes the component impossible to misuse into promising an
action it does not render, and it is exactly the semantics FR-3.5 describes. The
loading and populated branches are untouched.

## 7. Data flow and error handling

No API surface changes. `useCreateVehicle` →
`vehicleService.createInFleet(fleetId, attributes)` → JSON:API POST, with
`onSettled` invalidating `vehicleKeys.lists()`
(`lib/hooks/api/vehicles.ts:47-57`) exactly as today.

Error handling is unchanged in substance and changes only in *where the user is
standing* when it happens: `createErrorFromUnknown` normalises the rejection,
`toast.error(apiError.message || 'Could not add vehicle')` fires, and the
dialog is deliberately left open so the typed values survive for a retry
(FR-2.8). Validation failures never reach `handleCreate` at all — `zodResolver`
rejects inside `form.handleSubmit`, inline `FormMessage`s render, and the
dialog is not asked to close.

## 8. Accessibility

| Requirement | Mechanism |
| --- | --- |
| `role="dialog"`, `aria-modal="true"`, inert background | Radix defaults; `modal` is not disabled |
| Focus trapped | Radix `FocusScope`, inherent to `DialogContent` |
| Initial focus on Nickname | DOM order — close button rendered after `children` (§4.2) |
| Focus restored to opener | Radix default restoration + controlled-mode capture (§4.4) |
| Focus after empty state unmounts | `onCloseAutoFocus` redirect (§5.4) |
| Labelled / described | `DialogTitle` + `DialogDescription`, Radix-wired `aria-labelledby` / `aria-describedby` |
| Visible focus rings | `focus:ring-2 focus:ring-ring focus:ring-offset-2` on the close button, matching `select.tsx:17` |

A `DialogDescription` is always supplied for the create dialog ("Make, model,
and year are required."), which also silences Radix's dev-time warning about a
missing description.

## 9. Testing

### 9.1 jsdom notes that shape the tests

Radix's modal layer sets `pointer-events: none` on `document.body` while open,
and `@testing-library/user-event` refuses to click elements whose computed
`pointer-events` is `none`. Clicks *inside* `DialogContent` are fine (the layer
sets `pointer-events: auto` on itself and children inherit it). Clicks on the
**overlay** are not reliably drivable through `userEvent`.

Overlay dismissal is therefore tested the way Radix actually detects it — a
pointer-down outside the content:

```ts
fireEvent.pointerDown(document.body);
```

`DismissableLayer` listens on the document and fires `onPointerDownOutside`,
which is the exact code path an overlay click takes. Escape is driven normally
with `userEvent.keyboard('{Escape}')`.

If Radix's dependencies trip on a DOM API jsdom lacks, the fix goes in
`src/test/setup.ts` alongside the existing `localStorage` and `matchMedia`
polyfills, with a comment explaining why — the pattern that file already
establishes. No such gap is anticipated (`react-dialog`, unlike `react-select`,
does not use `hasPointerCapture` or `scrollIntoView`), but the location is
settled in advance so it does not get improvised into a test file.

### 9.2 `VehiclesPage.test.tsx` (new)

`useAuth` is mocked per test to vary `role` and `activeFleetId`;
`vehicleService` is mocked at the module boundary, matching
`VehicleCard.test.tsx`'s convention. Rendering goes through
`renderWithProviders` (retries off, so the error path resolves on the first
rejection).

Cases:

1. Header **Add Vehicle** opens a dialog titled "Add Vehicle" containing the form.
2. Empty-state **Add Vehicle** opens the same dialog.
3. The header trigger is still in the document while the dialog is open.
4. Submitting with make/model/year blank shows inline errors and leaves the
   dialog open; `vehicleService.createInFleet` is never called.
5. A valid submit calls `createInFleet` with optionals **omitted** (asserting
   `toCreateAttributes` survived), closes the dialog, and fires the success toast.
6. A rejected submit fires the error toast, keeps the dialog open, and keeps the
   typed make value in its input.
7. Escape, `fireEvent.pointerDown(document.body)`, the close button, and Cancel
   each close the dialog with no `createInFleet` call.
8. With the mutation held pending (a never-resolving `createInFleet`), Escape,
   outside pointer-down, and the close button all leave the dialog open.
9. Type into Nickname → close → reopen → the Nickname input is empty.
10. A viewer role renders neither trigger.
11. Creating from the empty-state trigger leaves `document.activeElement` on the
    header button (FR-4.5), not on `document.body`.

Case 5 asserts the *call arguments*, not just that a call happened — the
empty-string-stripping in `toCreateAttributes` is the kind of behaviour a
refactor silently drops. Case 8 is the one that would regress if the
`dismissible` prop were wired to only two of its three effects.

### 9.3 `VehicleList.test.tsx` (new)

1. Zero vehicles with `emptyAction` → full copy plus the node.
2. Zero vehicles without `emptyAction` → "No vehicles yet." and no button.
3. Populated list → a card per vehicle, no empty-state text (guards against the
   new branch leaking).

`dialog.tsx` gets no dedicated test file: it is exercised end-to-end by the page
tests (open, focus, escape, close, overlay), and a unit test of a styling
wrapper would assert class strings, which is churn.

### 9.4 Gates

`npm run lint`, `npm run format:check`, `npm run build`, `npm test` from the
repo root, with the WSL node on `PATH`. Baseline to hold: 293 passing in
`apps/web`, 7 in `shared-ts`, 10 in `ui-components` — plus the new cases, with
nothing pre-existing regressed. `git status` must show `VehicleDetailPage.tsx`
untouched.

## 10. Risks

| Risk | Likelihood | Mitigation |
| --- | --- | --- |
| Radix Dialog + jsdom needs an unanticipated polyfill | Low | Fix location pre-decided (`src/test/setup.ts`, §9.1) |
| Overlay-dismissal test is flaky under `userEvent` | Medium | Use `fireEvent.pointerDown(document.body)` — the actual detection path |
| Initial focus lands on the close button, failing FR-4.3 | Medium if built carelessly | Close button rendered **after** `children` (§4.2); covered by the focus assertions |
| `dismissible` wired to only some of its three effects | Medium | Test case 8 exercises all three independently |
| Dependency install churns `package-lock.json` beyond the one package | Low | Review the lock diff; expect only `react-dialog` and its transitive additions |
| `VehicleForm` change ripples into edit mode | Low | Change is one `disabled` attribute on an existing element (§5.3); `VehicleDetailPage` untouched and asserted so |

## 11. Explicitly not in this design

Edit-vehicle dialog (owned by `task-012`), the fuel/mileage/maintenance inline
forms, any change to `vehicleSchema` or the form's field set, a mobile
drawer variant, unsaved-changes confirmation, moving `Dialog` into
`packages/ui-components`, adding `tailwindcss-animate`, and any backend change.
