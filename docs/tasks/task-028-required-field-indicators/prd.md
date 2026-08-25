# Required Field Indicators — Product Requirements Document

Version: v1
Status: Draft
Created: 2026-08-25
---

## 1. Overview

Every form in the MyFleet web UI renders its field labels as bare text. Nothing
distinguishes a field the user *must* fill from one they may skip. The only
signal available today arrives after the fact: the user fills in what looks
plausible, presses Save, and `FormMessage` paints a red error under whichever
fields the Zod resolver rejected. On the vehicle form that is a five-field guess
before the first useful feedback; on the fuel form the rule is not even
per-field ("provide price per gallon or total cost") and cannot be inferred from
the layout at all.

This is greenfield, not a consistency cleanup. A grep across
`apps/web/src/components` and `apps/web/src/pages` returns **zero** hits for
`aria-required`, the HTML `required` attribute, asterisk markers in JSX, or a
user-visible "(optional)". Assistive technology gets nothing either: `FormControl`
sets `aria-describedby` and `aria-invalid` but never `aria-required`
(`apps/web/src/components/ui/form.tsx:95-110`), so a screen-reader user has
strictly less information than a sighted one.

The fix is structural rather than cosmetic. All ten react-hook-form forms route
their labels through a single `FormLabel` primitive
(`apps/web/src/components/ui/form.tsx:79-93`), so the marker, its styling, and
its accessibility wiring can be defined once and adopted at 28 call sites. This
task adds an explicit `required` prop to that primitive, threads the flag to
`FormControl` so it can emit `aria-required`, adopts it across every form,
extends the convention to the three input surfaces that sit outside
react-hook-form, records the convention in the frontend guidelines, and pins it
with a guard test so new forms inherit the rule instead of re-deriving it.

## 2. Goals

Primary goals:

- A user can tell, before typing anything, which fields a form will refuse to
  submit without.
- The required marker is defined once in `FormLabel` and rendered identically on
  every form.
- Every required control exposes `aria-required="true"`, so screen-reader users
  get the same information as sighted users.
- Conditionally-required fields are marked truthfully — the marker appears and
  disappears as the condition changes, rather than lying in one direction.
- The either/or requirement in the fuel form is stated in prose where the user
  will read it, since it is not expressible as a per-field marker.
- The convention is documented in `frontend-dev-guidelines` and enforced by an
  executable test, so a form added next quarter is marked without anyone
  remembering to ask.

Non-goals:

- No change to any Zod schema's validation behaviour. Required-ness is being
  *communicated*, not redefined. Every form must accept and reject exactly the
  same inputs after this change as before.
- No change to validation messages, submit behaviour, mutation handling, dialog
  close semantics, or toasts.
- No backend, API-contract, or deployment-manifest change. `apps/web` only.
- No form redesign, relayout, or field reordering.
- No marker on search/filter inputs (`AdminFleetsPage.tsx:105`,
  `AdminAuditPage.tsx:73`) — required-ness is not a meaningful property of a
  filter.
- No marker on the notification preference switches
  (`NotificationPreferences.tsx:60`) — a toggle is always in a valid state.
- No use of the native HTML `required` attribute. It would trigger browser-native
  validation bubbles that compete with `FormMessage`.

## 3. User Stories

- As someone adding their first vehicle, I want to see which fields are
  mandatory so I can fill them in one pass instead of discovering them through
  submit errors.
- As someone logging a fill-up, I want to know that I need *either* a price per
  gallon *or* a total cost — not both, not neither — before I start typing.
- As someone setting up a hybrid maintenance schedule, I want the two interval
  fields to show as required once I pick "hybrid", because they were not
  required a moment ago when I had "time" selected.
- As a screen-reader user, I want my software to announce a field as required so
  I get the same up-front information a sighted user gets from the asterisk.
- As someone editing an existing vehicle, I do not want an asterisk on the
  make/model/year fields I am not allowed to change.
- As a developer adding a new form, I want the required convention to be
  documented and test-enforced so I do not have to guess or copy the wrong
  example.

## 4. Functional Requirements

### 4.1 The marker primitive

**FR-1.** `FormLabel` (`apps/web/src/components/ui/form.tsx:79-93`) gains an
optional `required?: boolean` prop, defaulting to `false`.

**FR-2.** When `required` is true, `FormLabel` renders the label's children
followed by a marker element. When it is false or absent, the rendered output is
byte-identical to today's.

**FR-3.** The marker is a `<span>` carrying `aria-hidden="true"`, containing the
literal `*`, styled with the `text-destructive` Tailwind token so it inherits
both themes. It must be `aria-hidden`: the accessible name of the control is
conveyed by the label text, and the required state is conveyed by
`aria-required` (FR-6). Announcing "Make star" is a regression, and a bare
asterisk inside the label would also break the exact-string label assertion at
`MaintenanceRecordForm.test.tsx:~48`.

**FR-4.** The marker is separated from the label text by whitespace that survives
JSX trimming — a leading space inside the span, or an explicit `{' '}`.
`Make*` is not acceptable output.

**FR-5.** `FormLabel` continues to apply `text-destructive` to the whole label
when the field has an error. The marker must remain visually distinguishable in
that state; if it becomes invisible against the errored label colour, give the
marker its own weight or opacity treatment rather than removing it.

### 4.2 Accessibility wiring

**FR-6.** A required field's control carries `aria-required="true"`. A
non-required field's control must not carry the attribute at all (not
`aria-required="false"`).

**FR-7.** The `required` flag reaches `FormControl` through `FormItemContext`,
not by duplicating the prop at every call site. `FormItem` gains an optional
`required?: boolean`; `useFormField()` returns it; `FormControl` reads it. This
means a field is marked in exactly one place.

**FR-8.** Because `FormControl` renders a Radix `Slot`, `aria-required` lands on
whatever element the slot wraps — an `<input>`, a `<textarea>`, a
`SelectTrigger`, or the `CategoryCombobox` trigger button. `CategoryCombobox`
already declares the slot-injected props it accepts
(`CategoryCombobox.tsx:29-39`); its props interface must be extended with
`'aria-required'?: boolean` and the value forwarded to the trigger button
alongside the existing `id` / `aria-describedby` / `aria-invalid`.

**FR-9.** Whether `required` is declared on `FormItem` (and inherited by both
`FormLabel` and `FormControl`) or on `FormLabel` alone (with `FormItem` as the
carrier) is an implementation choice for the design phase, provided FR-7 holds:
one declaration site per field.

### 4.3 Form legend

**FR-10.** A form that renders three or more fields displays a legend reading
`* Required` at the bottom of the field stack, above the submit row, in
`text-sm text-muted-foreground`.

**FR-11.** Forms with fewer than three rendered fields do not display a legend —
the asterisk on a one- or two-field form is self-explanatory and the legend
would outweigh the form.

**FR-12.** Legend applies to: `VehicleForm`, `FuelForm`, `MaintenanceRecordForm`,
`MaintenanceScheduleForm`. Legend does not apply to: `MileageForm`,
`FleetNameForm`, `InviteForm`, `CompleteScheduleDialog`, `OnboardingPage`,
`MemberList` successor select, `PurgeConfirmDialog`.

**FR-13.** The legend is a shared element, not re-typed per form — a small
component or a shared constant, so the wording cannot drift.

### 4.4 Per-form field marking

The required-ness below is read off the Zod schemas in
`apps/web/src/lib/schemas/`. Marking must match the schema; where it cannot
(§4.5), the deviation is specified explicitly.

**FR-14. `VehicleForm.tsx`** — 8 rendered fields.

| Field | Line | Marked |
|---|---|---|
| Nickname | `:49` | no |
| Make | `:65` | **yes, create mode only** |
| Model | `:78` | **yes, create mode only** |
| Year | `:91` | **yes, create mode only** |
| Trim | `:118` | no (create-only field) |
| VIN | `:131` | no (create-only field) |
| Current Mileage | `:147` | no |
| Notes | `:170` | no |

**FR-15.** In edit mode, make/model/year render `disabled`
(`VehicleForm.tsx:67`) because they are immutable after create. They must not
carry the marker or `aria-required` in that mode: the user cannot change the
value and cannot fail to supply it, so an asterisk is noise. Bind the prop to
the existing `isCreate` flag.

**FR-16. `FuelForm.tsx`** — 5 fields.

| Field | Line | Marked |
|---|---|---|
| Date | `:58` | yes |
| Mileage (miles) | `:73` | yes |
| Gallons | `:96` | yes |
| Total Cost ($) | `:122` | no — see FR-21 |
| Price per Gallon ($) | `:147` | no — see FR-21 |

**FR-17. `MaintenanceRecordForm.tsx`** — 7 fields.

| Field | Line | Marked |
|---|---|---|
| Category | `:93` | yes |
| Date Performed | `:113` | yes |
| Description | `:127` | no |
| Mileage (miles) | `:147` | no |
| Cost ($) | `:170` | no |
| Vendor / Shop | `:195` | no |
| Notes | `:209` | no |

**FR-18. `MaintenanceScheduleForm.tsx`** — 4 fields.

| Field | Line | Marked |
|---|---|---|
| Category | `:60` | yes |
| Recurrence Type | `:80` | yes |
| Every (months) | `:104` | **conditional — see FR-19** |
| Every (miles) | `:130` | **conditional — see FR-19** |

**FR-19.** The two interval fields are marked reactively, driven by the same
`useWatch` result that already governs their visibility
(`MaintenanceScheduleForm.tsx:45-49`). `Every (months)` is marked when
`recurrenceType` is `time` or `hybrid`; `Every (miles)` when it is `mileage` or
`hybrid`. Reuse the existing `showMonths` / `showMiles` booleans — do not
introduce a second pair of conditions that can drift from the schema's
`superRefine` (`maintenanceSchedule.ts:30-48`). Because each field renders only
when its condition holds, a rendered interval field is always a required one.

**FR-20.** State this coupling in a comment at the `required` prop, in the same
terms the schema's doc comment uses (`maintenanceSchedule.ts:5-12`), so the
three sites — schema, visibility, marker — read as one rule.

**FR-21.** `FuelForm`'s either/or rule cannot be expressed as a per-field
asterisk: marking both fields claims both are mandatory, marking neither leaves
the rule invisible. Neither `Total Cost` nor `Price per Gallon` is marked;
instead a `FormDescription` renders under the pair reading: **"Enter price per
gallon or total cost (or both)."** This mirrors the schema's own message
(`fuel.ts:36`) so the up-front hint and the post-submit error say the same thing.
`FormDescription` is already wired into `aria-describedby` by `FormControl`
(`form.tsx:104`), so the note is announced with the field.

**FR-22. `MileageForm.tsx`** — `Mileage (miles)` (`:36`) marked. The schema's
`recordedAt` is not rendered and needs no treatment.

**FR-23. `CompleteScheduleDialog.tsx`** — `Date completed` (`:84`) marked;
`Odometer (miles)` (`:98`) not marked (auto-filled and clearable).

**FR-24. `FleetNameForm.tsx`** — `Fleet Name` (`:47`) marked.

**FR-25. `InviteForm.tsx`** — `Email` (`:48`) and `Role` (`:61`) both marked.
`Role` is a `Select`; `FormControl` wraps its `SelectTrigger` (`:64-68`), so
`aria-required` lands on the trigger.

**FR-26. `OnboardingPage.tsx`** — `Fleet Name` (`:116`) marked.

### 4.5 Non-react-hook-form surfaces

These three use raw labels and cannot receive `FormLabel`'s prop. They get the
same visual and accessible treatment by hand, using the same marker markup.

**FR-27. `MemberList.tsx:274`** — the successor-owner `<Select>` in the
leave-fleet dialog. Its lowercase `<label htmlFor="successor">` gains the marker
span; the `SelectTrigger` gains `aria-required="true"`. Choosing a successor is
mandatory to complete the action.

**FR-28. `PurgeConfirmDialog.tsx:198`** — the type-to-confirm input. Its
`<Label htmlFor="purge-confirmation">` gains the marker; the `<Input>` at `:199`
gains `aria-required="true"`.

**FR-29.** The marker markup must not be copy-pasted a third and fourth time.
Extract it into a shared element (e.g. `RequiredMarker`) that both `FormLabel`
and these two call sites render, so FR-3's styling and `aria-hidden` are defined
once.

**FR-30.** No treatment for `AdminFleetsPage.tsx:105`, `AdminAuditPage.tsx:73`,
`NotificationPreferences.tsx:60`, `AttachmentPicker.tsx`, or
`MediaUploadButton.tsx`.

### 4.6 Documentation and drift protection

**FR-31.** `.claude/skills/frontend-dev-guidelines/resources/patterns-forms-validation.md`
gains a section covering: the `required` prop, the `* Required` legend and its
three-field threshold, `aria-required` reaching the control via `FormItem`
context, and the rule that conditional required-ness is driven by the same
`useWatch` booleans as visibility. Include the `FuelForm` `FormDescription`
pattern as the documented escape hatch for cross-field requirements.

**FR-32.** A numbered rule is added to the frontend reviewer checklist so
`frontend-guidelines-reviewer` enforces it: every `FormField` whose schema field
is statically required renders `<FormLabel required>`. Assign the next free
`FE-*` number.

**FR-33.** A guard test is added under `apps/web/src/test/`, following the
source-scanning style already established by `conventions.test.ts`,
`sidebarTokens.test.ts`, and `tailwindVarSyntax.test.ts`. It reads the form
component sources and asserts that each field named in the FR-14 … FR-28 tables
carries — or does not carry — the `required` prop as specified.

**FR-34.** The guard test carries an explicit, commented exception list for the
fields that deliberately deviate from their schema's static optionality: the two
`FuelForm` cost fields (FR-21), the two `MaintenanceScheduleForm` intervals
(FR-19), and `VehicleForm`'s make/model/year in edit mode (FR-15). An
undocumented deviation must fail the test; a documented one must not.

## 5. API Surface

No API surface changes. No endpoint is added, modified, or removed. No request
or response shape changes. This task does not touch any Go service or any file
under `apps/web/src/services/`.

## 6. Data Model

No data model changes. No entity, field, relationship, constraint, or migration.

Zod schemas in `apps/web/src/lib/schemas/` are **read** to determine which
fields are required but must not be **modified** — see the non-goal in §2. The
schemas remain the single source of truth for validation; this task adds a
second, human-readable projection of that truth into the UI, and FR-33 exists to
keep the two from diverging.

## 7. Service Impact

| Service | Change |
|---|---|
| `apps/web` | All changes. See file list below. |
| All Go services | None. |
| `packages/shared-ts` | None. |
| `packages/ui-components` | None. Contains only a `.gitkeep`; the marker is form-specific and belongs beside `ui/form.tsx`. |
| `deploy/k8s` | None. No manifest render or dry-run needed. |

Files changed in `apps/web/src`:

- `components/ui/form.tsx` — `required` on `FormLabel`/`FormItem`,
  `aria-required` in `FormControl`, `useFormField` return shape
- the shared `RequiredMarker` and `* Required` legend elements (FR-13, FR-29)
- `components/features/vehicles/VehicleForm.tsx`
- `components/features/vehicles/fuel/FuelForm.tsx`
- `components/features/vehicles/maintenance/MaintenanceRecordForm.tsx`
- `components/features/vehicles/maintenance/MaintenanceScheduleForm.tsx`
- `components/features/vehicles/mileage/MileageForm.tsx`
- `components/features/vehicles/dialogs/CompleteScheduleDialog.tsx`
- `components/features/vehicles/CategoryCombobox.tsx` — forward `aria-required`
- `components/features/settings/FleetNameForm.tsx`
- `components/features/settings/InviteForm.tsx`
- `components/features/settings/MemberList.tsx`
- `components/admin/PurgeConfirmDialog.tsx`
- `pages/OnboardingPage.tsx`
- new + updated tests

Outside `apps/web`:

- `.claude/skills/frontend-dev-guidelines/resources/patterns-forms-validation.md`
- the frontend reviewer checklist (FR-32)

## 8. Non-Functional Requirements

**Accessibility.** The marker is decorative (`aria-hidden`); the semantic signal
is `aria-required`. Neither the accessible name nor the accessible description
of any control may change as a result of this task — only the required state.
`text-destructive` on the asterisk must meet contrast requirements against the
form background in both light and dark themes; verify against the tokens in
`index.css` rather than assuming.

**No behaviour change.** Validation is untouched. Every form accepts and rejects
exactly the inputs it did before. This is the primary regression risk and the
existing schema tests (`fuel.test.ts`, `maintenanceSchedule.test.ts`) must pass
unchanged, with no edits.

**Test compatibility.** `MaintenanceRecordForm.test.tsx:~48` queries
`screen.getByText('Category')` and compares `htmlFor` against the control's `id`,
with a comment explaining that `getByLabelText` would pass spuriously there
because the combobox carries its own `aria-label`. An asterisk placed inside the
label's text node breaks that exact-string match; the `aria-hidden` span of FR-3
keeps `getByText('Category')` matching. Any test that does need updating must be
updated deliberately, not loosened to a regex to make it pass.

**Performance.** Rendering one extra span per required field is immaterial. The
reactive marking in FR-19 introduces no new subscription — it reuses the
existing `useWatch` result.

**Observability.** Not applicable; no logging, metrics, or telemetry.

**Security.** Not applicable; no auth, authorization, input handling, or data
exposure change.

## 9. Open Questions

1. **Marker colour.** `text-destructive` is the conventional choice and matches
   `FormMessage`, but it also means the form shows red glyphs in its resting
   state. `text-muted-foreground` is calmer at the cost of being easier to miss.
   Resolve during design, ideally by looking at both.

2. **Where the flag is declared.** FR-9 leaves open whether `required` sits on
   `FormItem` (inherited by label and control) or on `FormLabel` (with context
   as the carrier). The former reads better; the latter is a smaller diff to the
   primitive. Design phase decides.

3. **Guard-test mechanism.** FR-33 specifies a source-scanning test. Whether it
   regex-matches the source or renders each form and asserts on the DOM is open.
   Rendering is more truthful but requires mocking each form's hooks; the
   established repo pattern is source scanning.

4. **`InviteForm` role default.** `role` has a `defaultValue` and a placeholder
   ("Select a role", `InviteForm.tsx:66`). It is genuinely required by the
   schema, so FR-25 marks it — but confirm during design that it renders
   unselected rather than pre-filled, or the asterisk marks a field that is
   never empty.

## 10. Acceptance Criteria

- [ ] `FormLabel` accepts `required`; with it absent, rendered output is
      unchanged from `main`.
- [ ] The marker is an `aria-hidden` `<span>` containing `*`, separated from the
      label text by whitespace, defined in exactly one place (FR-29).
- [ ] Every field marked "yes" in the FR-14 … FR-28 tables renders the marker;
      every field marked "no" does not.
- [ ] Every marked control carries `aria-required="true"`; no unmarked control
      carries the attribute in any form.
- [ ] `aria-required` reaches the `CategoryCombobox` trigger button, the
      `InviteForm` `SelectTrigger`, and the `MemberList` successor
      `SelectTrigger` — not only plain `<input>` elements.
- [ ] Switching `Recurrence Type` between `time`, `mileage`, and `hybrid`
      updates which interval fields are marked, with no stale marker.
- [ ] `FuelForm` shows "Enter price per gallon or total cost (or both)." under
      the cost pair, and neither cost field carries an asterisk.
- [ ] `VehicleForm` in create mode marks make/model/year; in edit mode it marks
      none of them and the disabled inputs carry no `aria-required`.
- [ ] The `* Required` legend appears on exactly the four forms in FR-12 and on
      no others.
- [ ] `MemberList.tsx:274` and `PurgeConfirmDialog.tsx:198` are marked and carry
      `aria-required`.
- [ ] No Zod schema file is modified. `git diff --stat apps/web/src/lib/schemas/`
      shows no changes to `.ts` sources.
- [ ] `fuel.test.ts` and `maintenanceSchedule.test.ts` pass with no edits.
- [ ] `MaintenanceRecordForm.test.tsx` passes with its `getByText('Category')`
      assertion intact.
- [ ] New tests cover: `FormLabel` with and without `required`; `aria-required`
      presence and absence; the reactive interval marking across all three
      recurrence types; `VehicleForm` create vs edit.
- [ ] The FR-33 guard test exists, fails when a specified `required` prop is
      removed, and documents its FR-34 exceptions in comments.
- [ ] `patterns-forms-validation.md` documents the convention; the reviewer
      checklist has a new numbered `FE-*` rule.
- [ ] `make fe-test` and `make fe-build` pass, with output pasted into the PR.
- [ ] The asterisk is legible in both light and dark themes, verified visually.
