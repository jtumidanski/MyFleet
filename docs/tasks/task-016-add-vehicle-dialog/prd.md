# Add Vehicle Dialog — Product Requirements Document

Version: v1
Status: Draft
Created: 2026-08-02
---

## 1. Overview

The Vehicles page currently creates vehicles through an inline form. Clicking **Add Vehicle** hides the trigger button and expands a `Card` between the page header and the vehicle grid (`apps/web/src/pages/VehiclesPage.tsx:51-72`). The form pushes the entire vehicle list down the page, the trigger disappears while the form is open, and nothing constrains keyboard focus to the form the user is filling in.

This task replaces that inline form with a modal dialog. The **Add Vehicle** button opens a dialog hosting the same `VehicleForm` in `create` mode. The vehicle grid stays where it is, focus is trapped inside the dialog while it is open, Escape and overlay clicks dismiss it, and each open starts from a clean form.

A prerequisite makes this task larger than a page edit: **the application has no dialog primitive.** `apps/web/src/components/ui/` contains `button`, `card`, `form`, `input`, `label`, `select`, `skeleton`, `switch`, and `textarea` — no `dialog` — and `@radix-ui/react-dialog` is not a dependency of `apps/web`. This task therefore also introduces the shared shadcn `Dialog` component, following the same conventions as the existing `select.tsx` primitive, so that later modal work (edit vehicle, delete confirmations) has a foundation to build on.

## 2. Goals

Primary goals:

- Replace the inline create-vehicle form on the Vehicles page with a modal dialog.
- Introduce a reusable, accessible `Dialog` primitive in `apps/web/src/components/ui/dialog.tsx` built on `@radix-ui/react-dialog`, matching the file conventions of the existing shadcn primitives.
- Keep the vehicle grid stationary — opening the create flow must not reflow the page content behind it.
- Give the empty state a direct call to action that opens the same dialog.
- Preserve today's create behavior exactly: same fields, same validation, same success toast, same error toast, same role gating.

Non-goals:

- Converting the **edit** vehicle form on `VehicleDetailPage.tsx:140` to a dialog. That page is being reworked under `task-012-vehicle-detail-redesign`; touching it here would collide.
- Converting the inline forms in `VehicleFuelSection`, `VehicleMileageSection`, or `VehicleMaintenanceSection`.
- Any change to `VehicleForm`'s field set, to `vehicleSchema` (`apps/web/src/lib/schemas/vehicle.ts`), or to validation rules.
- Any backend change. No `fleet-service` endpoint, contract, or model is touched.
- A mobile bottom-sheet / drawer variant. The application is not currently mobile-optimized; a single centered dialog serves all breakpoints.
- An unsaved-changes confirmation prompt. Dismissing discards silently (see FR-2.4).
- Moving the `Dialog` primitive into `packages/ui-components`. That package currently holds only formatters and `StatusBadge`; UI primitives live in `apps/web/src/components/ui/`.

## 3. User Stories

- As a fleet **owner or member**, I want to add a vehicle in a focused modal so that the page behind me stays put and I can see where the new vehicle will land.
- As a fleet **owner or member** with an empty fleet, I want an obvious button in the empty state so that I do not have to hunt for the action in the page header.
- As a **keyboard user**, I want focus trapped in the dialog and returned to the button I came from so that I never lose my place in the page.
- As a **screen reader user**, I want the dialog announced with a title and description so that I know what the modal is asking me for.
- As a fleet **viewer**, I want the add-vehicle affordances hidden so that I am not offered an action I cannot perform.
- As any user, I want a half-completed form discarded when I close the dialog so that reopening gives me a clean slate rather than stale input.

## 4. Functional Requirements

### FR-1 — Dialog primitive

- **FR-1.1** Add `@radix-ui/react-dialog` (`^1.1.0`) to `dependencies` in `apps/web/package.json`, keeping the alphabetical ordering of the existing `@radix-ui/*` entries.
- **FR-1.2** Create `apps/web/src/components/ui/dialog.tsx` exporting: `Dialog`, `DialogTrigger`, `DialogPortal`, `DialogOverlay`, `DialogClose`, `DialogContent`, `DialogHeader`, `DialogFooter`, `DialogTitle`, `DialogDescription`.
- **FR-1.3** The file must follow the conventions established by `apps/web/src/components/ui/select.tsx`: `React.forwardRef` wrappers over the Radix primitives, `cn()` from `../../lib/utils` for class merging, `className` accepted and merged (not overwritten), and `displayName` assigned from the corresponding Radix primitive.
- **FR-1.4** Styling uses existing Tailwind design tokens only (`bg-background`, `border-border`, `text-muted-foreground`, `ring-ring`, etc.) so the dialog renders correctly in both light and dark themes without new CSS variables.
- **FR-1.5** `DialogContent` renders inside a `DialogPortal` over a `DialogOverlay`, is horizontally and vertically centered, is capped at `max-w-lg` in width and `max-h-[85vh]` in height, and scrolls its own body when content exceeds that height.
- **FR-1.6** `DialogContent` renders a close (`X`) button in its top-right corner, using `lucide-react`'s `X` icon consistent with the other primitives' icon usage. The button carries an accessible name of "Close".
- **FR-1.7** Open and close transitions use the Radix `data-[state=open]` / `data-[state=closed]` attribute selectors, consistent with how `select.tsx` animates.

### FR-2 — Create-vehicle dialog on the Vehicles page

- **FR-2.1** The inline `Card` wrapper containing `VehicleForm` is removed from `VehiclesPage`. `VehicleForm` is rendered inside `DialogContent` instead, still with `mode="create"`.
- **FR-2.2** The dialog's title is **"Add Vehicle"**. It carries a `DialogDescription` conveying that required fields are make, model, and year.
- **FR-2.3** The dialog opens when the header **Add Vehicle** button is activated, and when the empty-state button (FR-3) is activated.
- **FR-2.4** The dialog closes on: successful submission, the **Cancel** button inside `VehicleForm`, the dialog's close (`X`) button, the Escape key, and a click on the overlay. Closing discards any entered values silently — no confirmation prompt, no draft persistence.
- **FR-2.5** While a create request is in flight (`createVehicle.isPending`), the dialog must not be dismissible: Escape, overlay click, and the close (`X`) button are all suppressed. The Cancel button inside `VehicleForm` is likewise unavailable during this window. This prevents a vehicle being created after the user believes they abandoned the flow.
- **FR-2.6** Each open presents a blank form. Values typed during a previous open — whether abandoned or successfully submitted — must not reappear. Unmounting `DialogContent` on close satisfies this; `forceMount` must not be used.
- **FR-2.7** On success, the existing behavior is preserved verbatim: `toast.success('Vehicle added')` fires and the dialog closes.
- **FR-2.8** On failure, the existing behavior is preserved verbatim: `toast.error(apiError.message || 'Could not add vehicle')` fires **and the dialog stays open** with the user's input intact, so they can correct and retry.
- **FR-2.9** The `toCreateAttributes` mapping in `VehiclesPage.tsx:14-25`, which strips empty-string optionals before they reach the backend, is carried over unchanged.

### FR-3 — Triggers and role gating

- **FR-3.1** The header **Add Vehicle** button remains visible whenever the dialog is open. The current `!showForm` condition that hides it is removed.
- **FR-3.2** Both triggers remain gated on write permission. Only `role === 'owner'` and `role === 'member'` see them; viewers see neither, matching the existing `canWrite` check at `VehiclesPage.tsx:34`.
- **FR-3.3** `VehicleList` gains an optional `emptyAction?: ReactNode` prop, rendered beneath the empty-state message. `VehicleList` stays presentational — it must not read auth context or role itself; the page passes the node it has already gated on `canWrite`.
- **FR-3.4** When the fleet has no vehicles and the user can write, the empty state renders an **Add Vehicle** button that opens the same dialog.
- **FR-3.5** When the fleet has no vehicles and the user is a viewer, the empty state renders the message **"No vehicles yet."** with no call to action — the current copy's "Add your first one to get started" is dropped for viewers, who cannot act on it. Writers keep the existing full copy.

### FR-4 — Accessibility and focus

- **FR-4.1** The dialog is a modal: content behind it is inert and hidden from assistive technology (Radix default behavior — do not disable it).
- **FR-4.2** Focus is trapped within the dialog while open and cannot Tab out to the page behind.
- **FR-4.3** On open, focus lands on the first focusable field in the form (the Nickname input).
- **FR-4.4** On close, focus returns to the element that opened the dialog.
- **FR-4.5** When the dialog was opened from the empty-state button and the create succeeds, that button unmounts along with the empty state. In that case focus must be redirected to the header **Add Vehicle** button rather than falling through to `document.body` — implement via `onCloseAutoFocus` on `DialogContent`.
- **FR-4.6** The dialog is labelled by its `DialogTitle` and described by its `DialogDescription` via the Radix-managed `aria-labelledby` / `aria-describedby` wiring.

## 5. API Surface

No API changes. The dialog reuses the existing create path unchanged:

- `POST /fleets/{fleetId}/vehicles` via `useCreateVehicle` (`apps/web/src/lib/hooks/api/vehicles.ts`), with the JSON:API request body already produced by `VehicleService`.

Request payload, response shape, error cases, cache invalidation, and the `createErrorFromUnknown` error-normalization path are all untouched.

## 6. Data Model

No data model changes. No entity, field, relationship, constraint, or migration is added or altered. `vehicleSchema` and `VehicleFormInput` are used exactly as they exist today.

## 7. Service Impact

Only `apps/web` is affected. No Go service (`fleet-service`, `auth-service`, `media-service`, `notification-service`) is touched, and no deployment manifest changes.

| File | Change |
| --- | --- |
| `apps/web/package.json` | Add `@radix-ui/react-dialog` dependency |
| `apps/web/src/components/ui/dialog.tsx` | **New** — shadcn Dialog primitive |
| `apps/web/src/pages/VehiclesPage.tsx` | Replace inline `Card` form with dialog; keep trigger always visible; pass `emptyAction`; add pending-guarded dismissal |
| `apps/web/src/components/features/vehicles/VehicleList.tsx` | Add `emptyAction?: ReactNode` prop; role-neutral empty-state copy variants |
| `apps/web/src/pages/VehiclesPage.test.tsx` | **New** — dialog flow coverage |
| `apps/web/src/components/features/vehicles/VehicleList.test.tsx` | **New** — empty-state action coverage |
| `package-lock.json` | Regenerated by the dependency install |

`VehicleForm.tsx` is expected to need **no changes** — it already accepts `onSubmit`, `onCancel`, and `submitting`, which is the full interface the dialog needs. If the implementation finds a change is unavoidable, it must not alter the `edit`-mode rendering path used by `VehicleDetailPage`.

## 8. Non-Functional Requirements

**Performance**
- The dialog's contents must not mount until first open, so the Vehicles page initial render does not pay for the form. Radix's default unmount-on-close behavior satisfies this.
- No new network request is introduced by opening the dialog.

**Accessibility**
- Meets WCAG 2.1 AA for modal dialogs: labelled, focus-trapped, Escape-dismissible (except during submit, per FR-2.5), and focus-restoring.
- Visible focus indicators on the close button and all form controls, using the existing `ring-ring` token.

**Theming**
- Renders correctly in both light and dark mode using existing tokens only, consistent with the work landed in `task-003-dark-mode-branding`.

**Compatibility**
- The new `Dialog` primitive must be generic enough to serve future modals (edit, delete confirmation) without modification — no vehicle-specific assumptions baked into `dialog.tsx`.

**Build hygiene**
- `npm run lint`, `npm run format:check`, `npm run build`, and `npm test` all pass from the repo root.
- Note for the implementer: in this WSL environment the `npm` on `PATH` is the Windows binary and fails on workspace symlinks (`EISDIR`). Use the WSL node: `export PATH="$HOME/.nvm/versions/node/v24.12.0/bin:$PATH"`.

## 9. Open Questions

None. All scoping questions were resolved before this PRD was written:

- The `Dialog` primitive is in scope (it does not exist yet and the feature cannot ship without it).
- Only the create form moves to a dialog; edit is handled by `task-012-vehicle-detail-redesign`.
- Dismissal while dirty discards silently.
- No mobile-specific variant.
- The header trigger stays visible while the dialog is open.
- The empty state gets its own trigger.
- Tests are in scope.

## 10. Acceptance Criteria

**Dialog primitive**
- [ ] `@radix-ui/react-dialog` appears in `apps/web/package.json` dependencies and `package-lock.json` is updated.
- [ ] `apps/web/src/components/ui/dialog.tsx` exists and exports all ten components listed in FR-1.2.
- [ ] Each exported wrapper forwards refs, merges `className` through `cn()`, and sets `displayName` — verifiable by reading the file alongside `select.tsx`.
- [ ] `DialogContent` is centered, capped at `max-w-lg` / `max-h-[85vh]`, scrolls internally past that height, and contains a close button with the accessible name "Close".
- [ ] `dialog.tsx` contains no vehicle-specific strings or logic.

**Vehicles page behavior**
- [ ] No `Card`-wrapped inline create form remains in `VehiclesPage.tsx`.
- [ ] Clicking **Add Vehicle** in the page header opens a dialog titled "Add Vehicle" containing the create form.
- [ ] The vehicle grid does not shift position when the dialog opens.
- [ ] The header **Add Vehicle** button is still rendered while the dialog is open.
- [ ] Submitting a valid vehicle closes the dialog, fires the `Vehicle added` success toast, and the new vehicle appears in the list.
- [ ] Submitting when the request fails keeps the dialog open with entered values intact and fires the `Could not add vehicle` (or server message) error toast.
- [ ] Submitting with make/model/year blank shows the existing inline field errors and does **not** close the dialog.
- [ ] Escape, an overlay click, the close (`X`) button, and **Cancel** each close the dialog without creating a vehicle.
- [ ] While a create request is in flight, Escape, overlay click, and the close button do not dismiss the dialog.
- [ ] Typing into the form, closing the dialog, and reopening presents an empty form.
- [ ] Optional fields left blank are omitted from the create payload rather than sent as empty strings.

**Empty state**
- [ ] With zero vehicles and an owner/member role, the empty state renders an **Add Vehicle** button that opens the same dialog.
- [ ] With zero vehicles and a viewer role, the empty state renders "No vehicles yet." with no button.
- [ ] `VehicleList` does not import or read auth context.

**Accessibility**
- [ ] The dialog exposes `role="dialog"` with `aria-modal="true"` and is labelled by its title.
- [ ] Focus moves into the dialog on open and is confined to it while open.
- [ ] Closing the dialog returns focus to the trigger that opened it.
- [ ] Creating the first vehicle from the empty-state trigger leaves focus on the header **Add Vehicle** button, not on `document.body`.

**Verification**
- [ ] `apps/web/src/pages/VehiclesPage.test.tsx` exists and covers: open from header, open from empty state, validation failure, successful create, error-path retention, dismissal paths, and form reset between opens.
- [ ] `VehicleList` empty-state variants (with and without `emptyAction`) are covered by tests.
- [ ] `npm test` passes from the repo root with no pre-existing test regressed (baseline: 293 passing in `apps/web`, 7 in `shared-ts`, 10 in `ui-components`).
- [ ] `npm run lint`, `npm run format:check`, and `npm run build` pass from the repo root.
- [ ] `VehicleDetailPage.tsx` is unmodified by this task.
