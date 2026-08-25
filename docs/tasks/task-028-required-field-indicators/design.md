# Required Field Indicators — Design

Task: task-028-required-field-indicators
PRD: `docs/tasks/task-028-required-field-indicators/prd.md` (v1, approved)
Status: Draft
Created: 2026-08-25

---

## 1. Shape of the change

The PRD is a wide, shallow change: one primitive gains a flag, 31 call sites
declare it, two hand-rolled surfaces mimic it, and a source-scanning test
pins the result. Nothing about the runtime behaviour of any form changes.

The design decisions that actually matter are four, and three of them are the
PRD's own open questions:

1. **Where the flag is declared** (Open Question 2) — `FormItem` vs `FormLabel`.
2. **What colour the marker is** (Open Question 1) — `text-destructive` vs
   something with measured contrast.
3. **How the guard test works** (Open Question 3) — source scan vs render.
4. **How `FuelForm`'s either/or note gets announced** — the PRD's stated
   mechanism does not work as written, and this design replaces it.

Everything else is mechanical and follows from those.

Two small factual corrections to the PRD, neither of which changes scope:

- The PRD says "28 call sites". A count of `<FormItem` across the ten forms
  gives **31** (Vehicle 8, MaintenanceRecord 7, Fuel 5, MaintenanceSchedule 4,
  CompleteSchedule 2, Invite 2, Mileage 1, FleetName 1, Onboarding 1). The
  per-form tables in FR-14 … FR-26 are complete and correct; only the total is
  off.
- `useFormField()` has **no consumers outside `components/ui/form.tsx`**
  (verified by grep). Its return shape can be extended without a ripple.

---

## 2. Decision 1 — `required` is declared on `FormItem`

**Resolves Open Question 2. Supersedes the literal wording of FR-1 under the
licence FR-9 grants.**

```tsx
<FormItem required>
  <FormLabel>Make</FormLabel>
  <FormControl>
    <Input ... />
  </FormControl>
  <FormMessage />
</FormItem>
```

`FormItemContext` carries `required`. `useFormField()` returns it. `FormLabel`
reads it and renders the marker; `FormControl` reads it and emits
`aria-required`. Neither takes a `required` prop of its own.

### Why not `FormLabel`

FR-1's phrasing (`FormLabel` gains the prop) makes `FormLabel` the declaration
site and `FormItem` a silent carrier, which means the flag must still be
threaded through context for `FormControl` to see it (FR-7) — the same
plumbing, but declared on the node that renders the *decoration* rather than
the node that owns the *field*. Required-ness is a property of the field, not
of its label, and `FormControl`'s `aria-required` is the semantic half of the
feature. Declaring it on the container that already owns the generated `id`,
the description id and the message id is the consistent place.

The decisive argument is the guard test. With the flag on `FormItem`, FR-33's
scan is one unambiguous token per field — `<FormItem required` inside a
`<FormField name="x">` block. With the flag on `FormLabel`, the scan has to
parse a prop off a JSX element whose children are the label text, which is
strictly more regex for strictly less signal.

**Rejected variant:** allow `required` on *both*, with `FormLabel` overriding
context. Two sources of truth for one boolean, and a call site where they
disagree is a bug that renders a marker without `aria-required` (or the
reverse) — exactly the divergence FR-7 exists to prevent. One declaration
site means one.

### The primitive diff

```tsx
interface FormItemContextValue {
  id: string;
  required: boolean;
}

const FormItem = forwardRef<HTMLDivElement, FormItemProps>(
  ({ className, required = false, ...props }, ref) => {
    const id = React.useId();
    return (
      <FormItemContext.Provider value={{ id, required }}>
        <div ref={ref} className={cn('space-y-2', className)} {...props} />
      </FormItemContext.Provider>
    );
  },
);
```

`FormItemProps` is `React.HTMLAttributes<HTMLDivElement> & { required?: boolean }`,
with `required` destructured out so it never lands on the `<div>` as a DOM
attribute.

`useFormField()` returns `required` alongside `id`/`formItemId`/etc.

`FormLabel`:

```tsx
const { error, formItemId, required } = useFormField();
return (
  <Label ref={ref} className={cn(error && 'text-destructive', className)} htmlFor={formItemId} {...props}>
    {props.children}
    {required && <RequiredMarker />}
  </Label>
);
```

(`children` is spread through `props` today; the implementation destructures it
so the marker can be appended after it.)

`FormControl`:

```tsx
const { error, formItemId, formDescriptionId, formMessageId, required } = useFormField();
<Slot
  ...
  aria-invalid={!!error}
  aria-required={required || undefined}
  {...props}
/>
```

`|| undefined` is load-bearing: FR-6 forbids `aria-required="false"` on
unmarked controls, and React only omits an `aria-*` attribute when the value is
`undefined` or `null` — `false` renders as the string `"false"`.

`aria-required` is placed **before** `{...props}`, matching the existing
`id`/`aria-describedby`/`aria-invalid` ordering, so a call site can still
override it if a future case needs to. Radix `Slot` gives child props priority
over slot props, so an explicit `aria-required` on the wrapped element wins
either way; the ordering just keeps `FormControl` internally consistent.

---

## 3. Decision 2 — the marker uses `text-danger`, not `text-destructive`

**Resolves Open Question 1, and deviates from FR-3's stated class.** FR-3
names `text-destructive`; Open Question 1 explicitly reopens the colour for the
design phase, and §8 requires the marker to meet contrast in both themes
"verified against the tokens in `index.css` rather than assuming". Verifying
is what changes the answer.

`--destructive` in dark mode is `0 62.8% 30.6%` — a dark red — on a
`222.2 84% 4.9%` background. That is roughly **2:1**, failing both the 4.5:1
text threshold and the 3:1 non-text threshold. It is legible today only
because `FormMessage` uses it on short bursts of text that the user is already
looking for; a resting-state glyph on every required label is a different
exposure.

`index.css:104-108` records why the alternative exists, in this project's own
words: the bare status tokens are "for text and numerals on `--background` /
`--card`", and are "distinct from `--destructive`, which styles destructive
CONTROLS". A required-field asterisk is text on a card, not a destructive
control. `docs/tasks/task-003-dark-mode-branding/contrast.md:18,27` records
`--danger` against the page background at **6.67:1 light / 7.23:1 dark** —
measured, not asserted.

So: `text-danger`. Same hue family, same visual language, an order of
magnitude better in dark mode, and it satisfies §8's contrast requirement by
citation rather than by a fresh measurement.

**Rejected:** `text-muted-foreground` (Open Question 1's calmer option). A
grey asterisk against a grey-black label at 3px of glyph is close to invisible
at a glance, and the PRD's whole premise is a signal the user can take in
before typing. If the resting-state red proves too loud in review, the cheap
adjustment is opacity on the marker span, not a token change.

**FR-5 falls out for free.** When the field errors, `FormLabel` applies
`text-destructive` to the label element; the marker is a child span carrying
its own `text-danger`, and a colour utility on the child wins over one
inherited from the parent. The marker stays legible in the error state with no
extra treatment.

---

## 4. Decision 3 — `RequiredMarker` lives in its own module

New file: `apps/web/src/components/ui/required.tsx`.

```tsx
/**
 * The `*` on a required field's label. Decorative by construction: the
 * semantic signal is `aria-required` on the control (FormControl emits it
 * from the same FormItem `required` flag). Announcing "Make star" would be a
 * regression, and an asterisk in the label's own text node would break the
 * exact-string label assertions in MaintenanceRecordForm.test.tsx.
 *
 * text-danger, not text-destructive: --destructive is ~2:1 on the dark
 * background; --danger is 6.67:1 / 7.23:1 (task-003 contrast.md).
 */
export function RequiredMarker() {
  // Leading space inside the span, on one line, so JSX cannot trim it: the
  // rendered text must be "Make *", never "Make*".
  return <span aria-hidden="true" className="text-danger"> *</span>;
}

/** `* Required` legend. Forms with 3+ fields only (FR-10/FR-11). */
export function RequiredLegend() {
  return (
    <p className="text-sm text-muted-foreground">
      <span className="text-danger">*</span> Required
    </p>
  );
}
```

**Why a separate file rather than exporting from `ui/form.tsx`:**
`MemberList.tsx` and `PurgeConfirmDialog.tsx` are not react-hook-form surfaces
(FR-27/FR-28). Importing the marker from the module that also exports `Form`,
`FormField` and the RHF context would make two non-form files depend on the
form primitive for a `<span>`. A standalone module states the boundary FR-29
is asking for: this is the one definition of the glyph, and both the RHF path
and the hand-rolled path consume it.

`ui/form.tsx` imports `RequiredMarker`; the two non-RHF sites import it
directly. Three consumers, one definition.

**Why the legend is not composed from `RequiredMarker`:** the legend's
asterisk is the *subject* of its sentence, not decoration — it must not be
`aria-hidden`, or the legend reads as a bare "Required". So it is written out
with the same `text-danger` so the two glyphs read as the same mark.

**Why `RequiredLegend` is a component and not a string constant** (FR-13
allows either): the wording *and* the classes both need pinning. A constant
pins only the wording, and four hand-typed `<p className="text-sm ...">`
wrappers are four chances to drift on the styling.

---

## 5. Decision 4 — `FuelForm`'s either/or note

**FR-21 as written does not work.** It calls for "a `FormDescription` [that]
renders under the pair", relying on `FormControl` having wired it into
`aria-describedby`. But `FormDescription` calls `useFormField()`, which
**throws** when there is no enclosing `FormItem` (`form.tsx:52-54`), and the
id it claims is `${id}-form-item-description` — scoped to one field. A
`FormDescription` under the pair, outside both `FormItem`s, is a runtime crash;
inside one of them, it describes only that field.

Three ways out:

| | Approach | Cost |
|---|---|---|
| A | A visible `FormDescription` inside **each** of the two `FormItem`s | The same sentence printed twice, side by side in the `sm:grid-cols-2` row |
| B | One visible shared `<p>` under the pair (as today), plus an `sr-only` `FormDescription` inside each `FormItem` | One visible line, correct announcement; two hidden duplicate nodes |
| C | Extend `FormItem` with a `describedBy` escape hatch so a shared node's id can be appended to `aria-describedby` | New API on the primitive, for one call site |

**Chosen: B.** It is the only one that satisfies both halves of FR-21 — the
note reads once where the user looks, and it is announced with *each* of the
two fields the rule governs. C is the "correct" general solution and is
rejected on YAGNI: one call site does not justify a new prop on the primitive
that every future reviewer has to understand. A is honest but visually
repetitive in a two-column row.

Concretely, in `FuelForm.tsx`:

```tsx
// Exported so the sr-only copies and the visible line cannot drift, and so
// this stays word-identical to the schema's own message (fuel.ts:36).
const COST_REQUIREMENT = 'Enter price per gallon or total cost (or both).';
```

- inside the `totalCost` `FormItem` and inside the `pricePerGallon`
  `FormItem`: `<FormDescription className="sr-only">{COST_REQUIREMENT}</FormDescription>`
- under the pair, replacing the existing `<p className="text-xs
  text-muted-foreground">Provide total cost, price per gallon, or both — the
  server derives the missing value.</p>` at `FuelForm.tsx:~166`:
  `<p className="text-sm text-muted-foreground">{COST_REQUIREMENT}</p>`

Note that this **replaces** an existing hint rather than adding one. The
current sentence says something the new one does not ("the server derives the
missing value"). That detail is worth keeping; the visible line becomes:

> Enter price per gallon or total cost (or both) — the server derives the
> missing value.

with the `sr-only` copies carrying the bare `COST_REQUIREMENT` sentence, which
is the part that mirrors `fuel.ts:36`. The visible line is the constant plus a
suffix, so the two cannot drift on the requirement itself.

Neither cost field gets `required` on its `FormItem`, so neither gets a marker
or `aria-required` — FR-16 and FR-21 hold.

`FormControl` already emits `aria-describedby={formDescriptionId}`
unconditionally, pointing at a node that until now often did not exist. Adding
the description makes the reference resolve; nothing else changes.

---

## 6. Decision 5 — conditional marking binds to the visibility boolean

`MaintenanceScheduleForm.tsx` already computes:

```tsx
const showMonths = recurrenceType === 'time' || recurrenceType === 'hybrid';
const showMiles = recurrenceType === 'mileage' || recurrenceType === 'hybrid';
```

The interval `FormItem`s take `required={showMonths}` / `required={showMiles}`,
**not** a bare `required`.

Because each field renders only inside `{showMonths && ...}`, the expression is
constant-true wherever it is evaluated, so `required` alone would be
behaviourally identical and shorter. Binding to the boolean anyway costs
nothing and buys two things: the marker survives a future change that renders
the fields always-visible-but-disabled instead of unmounting them, and the
reader sees the schema rule, the visibility rule and the marker rule as one
expression rather than three coincidences. FR-19 asks for exactly this
("reuse the existing `showMonths` / `showMiles` booleans"); FR-20's comment
goes at the `required` prop, phrased in the schema's terms.

`VehicleForm` follows the same shape with the existing `isCreate` flag:
`<FormItem required={isCreate}>` on make/model/year (FR-15). In edit mode the
inputs are `disabled` and carry no marker and no `aria-required`.

These are the only two dynamic call sites. Every other `required` in the
codebase is a bare boolean literal.

---

## 7. Decision 6 — the guard test scans source

**Resolves Open Question 3.**

New file: `apps/web/src/test/requiredFieldMarkers.test.ts`, following
`conventions.test.ts` / `sidebarTokens.test.ts` / `tailwindVarSyntax.test.ts`.

### Why source scanning

The thing FR-33 protects is a **convention**, not a behaviour: "the field
named `make` declares itself required". A rendering test would have to mount
ten forms, each with its own React Query providers, category fixtures and
router context — `MaintenanceRecordForm.test.tsx` needs a `QueryClientProvider`
just to render one combobox. That is a large amount of fixture for an
assertion that is, in the end, about a prop being present. Source scanning is
also the established idiom in this directory, so a reviewer reads one pattern
instead of two.

Behavioural truth — that `required` actually produces a marker and an
`aria-required` — is covered once, properly, in the primitive's own test
(§8), and at the two dynamic call sites. The guard test then only has to
assert that each field opts in. That split keeps the expensive test cheap and
the cheap test truthful.

### Mechanism

For each form file, split the source on `<FormField`, and for each resulting
block read `name="<field>"` and test the block for `<FormItem required`.
Compare against a table:

```ts
const EXPECTED: Record<string, Record<string, boolean>> = {
  'components/features/vehicles/VehicleForm.tsx': {
    nickname: false,
    // FR-15: required={isCreate}. make/model/year are immutable after create
    // and render disabled in edit mode, where an asterisk would be noise.
    make: true, model: true, year: true,
    trim: false, vin: false, currentMileage: false, notes: false,
  },
  // ...
};
```

The block-splitting is safe because every `<FormField>` in these files is a
flat sibling — none nests another. The test asserts the discovered field set
equals the expected key set, so a *new* field that nobody classified fails
loudly rather than passing by omission.

`MemberList.tsx` and `PurgeConfirmDialog.tsx` are checked separately: the
source must contain `<RequiredMarker />` and `aria-required` in the region
around the named control.

### FR-34 exceptions

The exception list is comments in this table, not a separate structure. Three
groups, each annotated where it appears:

- `FuelForm.totalCost` / `pricePerGallon` → `false` despite the schema's
  cross-field rule (FR-21, §5).
- `MaintenanceScheduleForm.intervalMonths` / `intervalMiles` → matched as
  `required={showMonths}` / `required={showMiles}`, not bare (FR-19, §6).
- `VehicleForm.make/model/year` → matched as `required={isCreate}` (FR-15).

So the regex is `<FormItem required(\s|>|=)` — it accepts both the bare form
and an expression form, and the table records which of the two each field is
expected to use, so swapping one for the other fails.

---

## 8. Testing

| Level | What | Where |
|---|---|---|
| Primitive | `FormItem required` renders an `aria-hidden` span containing `*`; the control gets `aria-required="true"`; without it there is no marker and **no `aria-required` attribute at all** (`toHaveAttribute` negated, not `'false'`) | new `components/ui/form.test.tsx` |
| Primitive | Accessible name is unchanged with and without `required`; `getByText('Label')` still matches | same |
| Dynamic | Switching Recurrence Type across `time` / `mileage` / `hybrid` marks the right intervals with no stale marker | new/extended `MaintenanceScheduleForm.test.tsx` |
| Dynamic | `VehicleForm` create marks make/model/year; edit marks none and the disabled inputs carry no `aria-required` | extended `VehicleForm.test.tsx` |
| Slot targets | `aria-required` reaches the `CategoryCombobox` trigger button and the `InviteForm` `SelectTrigger` — not only plain inputs | `MaintenanceRecordForm.test.tsx` (new case), `InviteForm.test.tsx` |
| Convention | Every field in the FR-14 … FR-28 tables | `test/requiredFieldMarkers.test.ts` |
| Regression | `fuel.test.ts`, `maintenanceSchedule.test.ts` pass **with no edits**; `MaintenanceRecordForm.test.tsx`'s `getByText('Category')` assertion stays intact | existing |

On that last row: `getByText` matches on `getNodeText`, which concatenates a
node's **direct child text nodes only**. `<label>Category<span aria-hidden> *</span></label>`
has direct text `"Category"`, so the existing exact-string assertion keeps
passing and must not be loosened to a regex (§8 of the PRD).

The negative assertions above are written against a real absent attribute, not
a vacuous query — see task-019.

---

## 9. `CategoryCombobox`

`CategoryComboboxProps` gains `'aria-required'?: boolean` next to the existing
`id` / `aria-describedby` / `aria-invalid` trio, and forwards it to the trigger
`<Button>` alongside them. The existing doc comment at
`CategoryCombobox.tsx:29-39` explains why those props land on the trigger and
not the popover root; the new prop is added to that list, not given its own
comment.

This is the only component that needs a props change. `SelectTrigger`,
`Input` and `Textarea` all spread arbitrary props onto their underlying
element, so `Slot` injection reaches them with no edit.

---

## 10. Legend placement

`RequiredLegend` renders at the bottom of the field stack, above the submit
row, on the four forms in FR-12: `VehicleForm`, `FuelForm`,
`MaintenanceRecordForm`, `MaintenanceScheduleForm`. In `FuelForm` it sits
*below* the cost hint — the hint is about two specific fields, the legend is
about the form.

The other seven surfaces get no legend (FR-11/FR-12). The three-field
threshold is a judgement, not a computed property; it is recorded in the
guidelines (§11) and asserted by the guard test as a per-file boolean, so
adding a fourth field to `MileageForm` some day fails the test and prompts the
decision rather than silently drifting.

---

## 11. `InviteForm`'s Role field — Open Question 4 resolved

`InviteForm.tsx:24` sets `defaultValues: { email: '', role: 'member' }`, and
the `Select` binds `defaultValue={field.value}`. The placeholder "Select a
role" therefore **never renders** — the field opens pre-filled with "Member".

FR-25 marks it anyway. Required-ness is a property of the field as the schema
defines it, not of whether the field happens to be empty at first paint; a
`Nickname` field left blank is optional whether or not the user types in it.
The asterisk tells the user this is a value the form insists on, which is true,
and the `aria-required` on the trigger is the same statement to a screen
reader. Removing the marker because the default is non-empty would make the
form's markup depend on its initial state, which is the kind of subtlety the
guard test exists to prevent people from re-deriving.

No change to the default or the placeholder — out of scope per §2's non-goals.

---

## 12. Documentation (FR-31, FR-32)

`.claude/skills/frontend-dev-guidelines/resources/patterns-forms-validation.md`
gains a "Required field indicators" section covering: `required` on `FormItem`
(not `FormLabel`) as the single declaration site; `text-danger` and why not
`text-destructive`; the `* Required` legend and its three-field threshold;
`aria-required` reaching the control through context; conditional required-ness
bound to the same `useWatch` booleans as visibility; and the `FuelForm`
`sr-only FormDescription` pattern as the documented escape hatch for
cross-field rules.

FR-32's reviewer rule takes the next free `FE-*` number (read off the checklist
at implementation time, not guessed here) and is phrased as: *every `FormField`
whose schema field is statically required declares `<FormItem required>`;
deviations are listed in `requiredFieldMarkers.test.ts` with a reason.*

---

## 13. Order of work

1. `ui/required.tsx` + `ui/form.tsx` primitive change + `form.test.tsx`.
   Nothing visible changes yet: with no call site passing `required`, the
   rendered output of all ten forms is byte-identical to `main`. This is the
   step to verify that claim against the existing suite.
2. `CategoryCombobox` prop forwarding.
3. The eight RHF forms + `OnboardingPage`, table by table.
4. The two non-RHF surfaces.
5. `FuelForm`'s cost note (§5) — separable from step 3's marker work.
6. Legends on the four forms.
7. Guard test, then the guidelines and the reviewer rule.

Step 1 landing first is what makes the "no behaviour change" claim checkable:
if the full suite passes after step 1 with zero test edits, the primitive is
backward-compatible, and every later failure is attributable to a call site.

## 14. Verification

`make fe-test` and `make fe-build`. No Go service, no `packages/shared-ts`, no
`deploy/k8s` — no `make ci`, no kustomize render, no dry-run.

Plus the two manual checks the automated suite cannot make:

- The asterisk legible on a form in both themes (the token choice in §3 is
  argued from recorded measurements, but the glyph is 3px of stroke — look at
  it).
- `git diff --stat apps/web/src/lib/schemas/` empty.

---

## 15. Deviations from the PRD, collected

| PRD | This design | Why |
|---|---|---|
| FR-1: `FormLabel` gains `required` | `FormItem` gains it; `FormLabel` reads context | FR-9 grants the choice; §2 |
| FR-3: marker styled `text-destructive` | `text-danger` | Open Question 1; `--destructive` is ~2:1 in dark mode; §3 |
| FR-21: one `FormDescription` under the cost pair | one visible `<p>` + an `sr-only FormDescription` per field | `FormDescription` outside a `FormItem` throws; §5 |
| FR-21: hint text replaces nothing | replaces the existing hint, keeping its "server derives" clause | §5 |
| "28 call sites" | 31 | count; §1 |
