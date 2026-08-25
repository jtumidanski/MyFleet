# Required Field Indicators Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Every MyFleet form tells the user — visually and to assistive technology — which fields it will refuse to submit without, from one declaration site per field.

**Architecture:** `FormItem` gains a `required` flag carried on `FormItemContext`. `FormLabel` reads it and renders a shared, `aria-hidden` `RequiredMarker`; `FormControl` reads it and emits `aria-required="true"` through the Radix `Slot` onto whatever control the field wraps. Two non-react-hook-form surfaces render the same marker by hand. A source-scanning guard test pins which field declares what, and the frontend guidelines plus a new reviewer rule record the convention.

**Tech Stack:** React 19, TypeScript, Vite, react-hook-form + Zod (`zodResolver`), shadcn/ui primitives over Radix, Tailwind v4, Vitest + Testing Library.

**Spec:** `docs/tasks/task-028-required-field-indicators/design.md` (PRD: `docs/tasks/task-028-required-field-indicators/prd.md`)

## Global Constraints

- **`apps/web` only**, plus `.claude/skills/frontend-dev-guidelines/resources/patterns-forms-validation.md` and `.claude/agents/frontend-guidelines-reviewer.md`. No Go service, no `packages/*`, no `deploy/k8s`. No `make ci`, no kustomize render, no dry-run.
- **No Zod schema may be modified.** `git diff --stat apps/web/src/lib/schemas/` must stay empty for the whole branch.
- **No validation, submit, mutation, dialog-close or toast behaviour changes.** Every form accepts and rejects exactly what it did before.
- **No native HTML `required` attribute** anywhere — it triggers browser validation bubbles that compete with `FormMessage`.
- Marker token is **`text-danger`**, not `text-destructive` (`--destructive` is ~2:1 on the dark background; `--danger` is 6.67:1 light / 7.23:1 dark, `docs/tasks/task-003-dark-mode-branding/contrast.md:18,27`).
- Marker text is exactly `" *"` (leading space inside the span) so rendered output is `Make *`, never `Make*`.
- `aria-required` is emitted as `required || undefined` — React renders `false` as the string `"false"`, and FR-6 forbids `aria-required="false"` on unmarked controls.
- Legend copy is exactly `* Required`, rendered only by `RequiredLegend`, only on `VehicleForm`, `FuelForm`, `MaintenanceRecordForm`, `MaintenanceScheduleForm`.
- Fuel either/or copy constant is exactly `Enter price per gallon or total cost (or both).` (PRD acceptance criterion). The visible line appends ` — the server derives the missing value.` as a suffix; the `sr-only` copies carry the bare constant.
- `fuel.test.ts`, `maintenanceSchedule.test.ts` and `MaintenanceRecordForm.test.tsx`'s `getByText('Category')` assertion must pass **with no edits**.
- Node is not always on `PATH`. Before any npm/make target: `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22`.
- All work happens in the worktree `/home/tumidanski/source/MyFleet/.worktrees/task-028-required-field-indicators` on branch `task-028-required-field-indicators`. Paths below are relative to `apps/web/src` unless stated otherwise.

---

## File Structure

**Created**

| Path | Responsibility |
|---|---|
| `components/ui/required.tsx` | The single definition of the `*` glyph (`RequiredMarker`) and the `* Required` legend (`RequiredLegend`). No react-hook-form dependency, so the two non-RHF surfaces can import it without pulling in the form context. |
| `components/ui/form.test.tsx` | Behavioural truth for the primitive: marker rendering, `aria-required` presence/absence, accessible name unchanged. |
| `components/features/vehicles/VehicleForm.test.tsx` | Create-vs-edit marking. |
| `components/features/vehicles/fuel/FuelForm.test.tsx` | Marking plus the either/or note's visible copy and its per-field announcement. |
| `components/features/vehicles/maintenance/MaintenanceScheduleForm.test.tsx` | Reactive interval marking across all three recurrence types. |
| `test/requiredFieldMarkers.test.ts` | Source-scanning convention guard over all ten form files and the two hand-rolled surfaces. |

**Modified**

| Path | Change |
|---|---|
| `components/ui/form.tsx` | `required` on `FormItemContext` + `FormItem` props; returned by `useFormField`; read by `FormLabel` and `FormControl`. |
| `components/features/vehicles/CategoryCombobox.tsx` | Accept and forward `'aria-required'` to the trigger button. |
| `components/features/vehicles/VehicleForm.tsx` | `required={isCreate}` on make/model/year; legend. |
| `components/features/vehicles/fuel/FuelForm.tsx` | `required` on date/mileage/gallons; `COST_REQUIREMENT` constant, one visible line + two `sr-only` `FormDescription`s; legend. |
| `components/features/vehicles/maintenance/MaintenanceRecordForm.tsx` | `required` on categoryId/performedAt; legend. |
| `components/features/vehicles/maintenance/MaintenanceScheduleForm.tsx` | `required` on categoryId/recurrenceType, `required={showMonths}` / `required={showMiles}` on the intervals; legend. |
| `components/features/vehicles/mileage/MileageForm.tsx` | `required` on mileage. |
| `components/features/vehicles/dialogs/CompleteScheduleDialog.tsx` | `required` on date. |
| `components/features/settings/FleetNameForm.tsx` | `required` on name. |
| `components/features/settings/InviteForm.tsx` | `required` on email and role. |
| `components/features/settings/MemberList.tsx` | Hand-rolled marker + `aria-required` on the successor picker. |
| `components/admin/PurgeConfirmDialog.tsx` | Hand-rolled marker + `aria-required` on the type-to-confirm input. |
| `pages/OnboardingPage.tsx` | `required` on name. |
| `pages/VehiclesPage.test.tsx` | Exact-string `getByLabelText('Make'\|'Model'\|'Year')` queries replaced with accessible-name role queries — see Task 3. |
| `../../.claude/skills/frontend-dev-guidelines/resources/patterns-forms-validation.md` | New "Required field indicators" section. |
| `../../.claude/agents/frontend-guidelines-reviewer.md` | New `FE-18` rule. |

### Known hazard: `getByLabelText` and the marker

Testing Library's `getLabelContent` (`@testing-library/dom/dist/label-helpers.js:11-26`) builds a label's text from **all** descendant text nodes and does **not** honour `aria-hidden`. So after this change a `<label>` for a required field has label-content `"Make *"`, and an exact `getByLabelText('Make')` stops matching.

- Regex queries (`/email/i`, `/fleet name/i`, `/type the fleet name/i`) keep matching and need no edit.
- `getByText('Category')` keeps matching: `getNodeText` concatenates **direct child text nodes only**, and the marker is a nested span.
- Accessible-name queries (`getByRole('textbox', { name: 'Make' })`) keep matching, because `dom-accessibility-api` **does** honour `aria-hidden`.

Only `pages/VehiclesPage.test.tsx` is affected, at six call sites. It is fixed in Task 3 by moving to role+name queries — deliberately, not by loosening to a regex. New tests in this plan use role+name queries throughout for the same reason.

---

## Task 1: The marker primitive and the `FormItem` flag

**Files:**
- Create: `apps/web/src/components/ui/required.tsx`
- Create: `apps/web/src/components/ui/form.test.tsx`
- Modify: `apps/web/src/components/ui/form.tsx`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `export function RequiredMarker(): JSX.Element` — `<span aria-hidden="true" className="text-danger"> *</span>`
  - `export function RequiredLegend(): JSX.Element` — `<p className="text-sm text-muted-foreground"><span className="text-danger">*</span> Required</p>`
  - `FormItem` props: `React.HTMLAttributes<HTMLDivElement> & { required?: boolean }`
  - `useFormField()` return gains `required: boolean`
  - `FormControl` emits `aria-required={required || undefined}`

- [ ] **Step 1: Write the failing test**

Create `apps/web/src/components/ui/form.test.tsx`:

```tsx
import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { useForm } from 'react-hook-form';
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from './form';
import { Input } from './input';

/**
 * One-field harness, so the primitive is exercised without any feature form's
 * providers. `required` is declared on FormItem — the single declaration site.
 * FormLabel renders the marker from it and FormControl emits aria-required
 * from it, both via FormItemContext.
 */
function Harness({ required }: { required?: boolean }) {
  const form = useForm<{ make: string }>({ defaultValues: { make: '' } });
  return (
    <Form {...form}>
      <form>
        <FormField
          control={form.control}
          name="make"
          render={({ field }) => (
            <FormItem required={required}>
              <FormLabel>Make</FormLabel>
              <FormControl>
                <Input type="text" {...field} />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
      </form>
    </Form>
  );
}

describe('FormItem required', () => {
  it('renders no marker and no aria-required when the flag is absent', () => {
    render(<Harness />);

    const input = screen.getByRole('textbox', { name: 'Make' });
    // A real absent attribute, not a query that could not fail (task-019).
    expect(input).not.toHaveAttribute('aria-required');
    expect(screen.getByText('Make').querySelector('span[aria-hidden="true"]')).toBeNull();
  });

  it('emits aria-required="true" on the control when the flag is set', () => {
    render(<Harness required />);

    expect(screen.getByRole('textbox', { name: 'Make' })).toHaveAttribute('aria-required', 'true');
  });

  it('renders the marker as an aria-hidden span, spaced off the label text', () => {
    render(<Harness required />);

    const label = screen.getByText('Make');
    const marker = label.querySelector('span[aria-hidden="true"]');
    expect(marker).not.toBeNull();
    expect(label.textContent).toBe('Make *');
  });

  it('leaves the accessible name unchanged', () => {
    render(<Harness required />);

    // dom-accessibility-api skips aria-hidden subtrees, so the name is still
    // exactly the label text.
    expect(screen.getByRole('textbox', { name: 'Make' })).toBeInTheDocument();
  });

  it('keeps the label matchable by its exact text', () => {
    render(<Harness required />);

    // getNodeText concatenates direct child text nodes only, which is what
    // keeps MaintenanceRecordForm.test.tsx's getByText('Category') passing.
    expect(screen.getByText('Make')).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

```sh
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
cd apps/web && npx vitest run src/components/ui/form.test.tsx
```

Expected: FAIL. TypeScript rejects `required` on `FormItem` (`Property 'required' does not exist`), and the `aria-required` / marker assertions fail.

- [ ] **Step 3: Create the marker module**

`apps/web/src/components/ui/required.tsx`:

```tsx
/**
 * The `*` on a required field's label. Decorative by construction: the
 * semantic signal is `aria-required` on the control, which FormControl emits
 * from the same FormItem `required` flag. Announcing "Make star" would be a
 * regression, and an asterisk in the label's own text node would break the
 * exact-string label assertions in MaintenanceRecordForm.test.tsx.
 *
 * text-danger, not text-destructive: --destructive is ~2:1 against the dark
 * background, while --danger measures 6.67:1 light / 7.23:1 dark
 * (docs/tasks/task-003-dark-mode-branding/contrast.md:18,27). index.css:104-108
 * records the split — the bare status tokens are for text on --background /
 * --card, --destructive styles destructive controls.
 */
export function RequiredMarker() {
  // Leading space inside the span, on one line, so JSX cannot trim it: the
  // rendered text must be "Make *", never "Make*".
  return <span aria-hidden="true" className="text-danger"> *</span>;
}

/**
 * `* Required` legend. Rendered at the bottom of the field stack on forms with
 * three or more fields (FR-10/FR-11); shorter forms are self-explanatory.
 *
 * Its asterisk is the subject of the sentence rather than decoration, so —
 * unlike RequiredMarker — it is not aria-hidden. It carries the same
 * text-danger so both glyphs read as the same mark.
 */
export function RequiredLegend() {
  return (
    <p className="text-sm text-muted-foreground">
      <span className="text-danger">*</span> Required
    </p>
  );
}
```

- [ ] **Step 4: Thread `required` through `form.tsx`**

In `apps/web/src/components/ui/form.tsx`, add the import next to the existing `Label` import:

```tsx
import { RequiredMarker } from './required';
```

Replace the context type:

```tsx
interface FormItemContextValue {
  id: string;
  /**
   * Declared once per field, on FormItem. FormLabel renders the marker from
   * it; FormControl emits aria-required from it. One declaration site means
   * the glyph and the semantics can never disagree.
   */
  required: boolean;
}
```

Replace the `useFormField` return block:

```tsx
  const fieldState = getFieldState(fieldContext.name, formState);
  const { id, required } = itemContext;

  return {
    id,
    name: fieldContext.name,
    required,
    formItemId: `${id}-form-item`,
    formDescriptionId: `${id}-form-item-description`,
    formMessageId: `${id}-form-item-message`,
    ...fieldState,
  };
```

Replace `FormItem`:

```tsx
type FormItemProps = React.HTMLAttributes<HTMLDivElement> & {
  /** Marks the field required: renders the marker and sets aria-required. */
  required?: boolean;
};

const FormItem = React.forwardRef<HTMLDivElement, FormItemProps>(
  // `required` is destructured out so it never lands on the <div> as a DOM
  // attribute.
  ({ className, required = false, ...props }, ref) => {
    const id = React.useId();
    return (
      <FormItemContext.Provider value={{ id, required }}>
        <div ref={ref} className={cn('space-y-2', className)} {...props} />
      </FormItemContext.Provider>
    );
  },
);
FormItem.displayName = 'FormItem';
```

Replace `FormLabel` (note `children` is now destructured so the marker can follow it):

```tsx
const FormLabel = React.forwardRef<
  React.ElementRef<typeof Label>,
  React.ComponentPropsWithoutRef<typeof Label>
>(({ className, children, ...props }, ref) => {
  const { error, formItemId, required } = useFormField();
  return (
    <Label
      ref={ref}
      className={cn(error && 'text-destructive', className)}
      htmlFor={formItemId}
      {...props}
    >
      {children}
      {/* The marker carries its own text-danger, which wins over the
          text-destructive this label takes on in the error state, so it stays
          legible without extra treatment. */}
      {required && <RequiredMarker />}
    </Label>
  );
});
FormLabel.displayName = 'FormLabel';
```

Replace `FormControl`:

```tsx
const FormControl = React.forwardRef<
  React.ElementRef<typeof Slot>,
  React.ComponentPropsWithoutRef<typeof Slot>
>(({ ...props }, ref) => {
  const { error, formItemId, formDescriptionId, formMessageId, required } = useFormField();
  return (
    <Slot
      ref={ref}
      id={formItemId}
      aria-describedby={error ? `${formDescriptionId} ${formMessageId}` : formDescriptionId}
      aria-invalid={!!error}
      // `|| undefined` is load-bearing: React renders `false` as the string
      // "false", and an unmarked control must carry no aria-required at all.
      aria-required={required || undefined}
      {...props}
    />
  );
});
FormControl.displayName = 'FormControl';
```

- [ ] **Step 5: Run the new test to verify it passes**

```sh
cd apps/web && npx vitest run src/components/ui/form.test.tsx
```

Expected: PASS, 5 tests.

- [ ] **Step 6: Run the whole suite to prove the primitive is backward-compatible**

```sh
cd apps/web && npm run test
```

Expected: PASS with **zero edits to any existing test**. No call site passes `required` yet, so every form's rendered output is byte-identical to `main`. Any failure here is a bug in this task, not in a later one — fix it before moving on.

- [ ] **Step 7: Commit**

```bash
git add apps/web/src/components/ui/required.tsx apps/web/src/components/ui/form.tsx apps/web/src/components/ui/form.test.tsx
git commit -m "feat(web): add required flag to FormItem and a shared required marker"
```

---

## Task 2: Forward `aria-required` through `CategoryCombobox`

**Files:**
- Modify: `apps/web/src/components/features/vehicles/CategoryCombobox.tsx:29-41,140-152`
- Modify: `apps/web/src/components/features/vehicles/CategoryCombobox.test.tsx`

**Interfaces:**
- Consumes: nothing from Task 1 at the type level — `FormControl` injects the prop through Radix `Slot` at runtime.
- Produces: `CategoryComboboxProps` gains `'aria-required'?: boolean`, forwarded to the trigger `<Button>`.

`SelectTrigger`, `Input` and `Textarea` all spread arbitrary props onto their underlying element, so `Slot` injection already reaches them. `CategoryCombobox` destructures its props explicitly and is the only component that needs a change.

- [ ] **Step 1: Write the failing test**

Append inside the existing top-level `describe` in `apps/web/src/components/features/vehicles/CategoryCombobox.test.tsx`:

```tsx
  // FormControl injects a11y props through Radix Slot; this component
  // destructures its props rather than spreading them, so each one has to be
  // forwarded to the trigger by hand.
  it('forwards aria-required to the trigger button', () => {
    renderCombobox({ 'aria-required': true });

    expect(screen.getByRole('combobox', { name: 'Category' })).toHaveAttribute(
      'aria-required',
      'true',
    );
  });

  it('leaves the trigger without aria-required when the prop is absent', () => {
    renderCombobox();

    expect(screen.getByRole('combobox', { name: 'Category' })).not.toHaveAttribute(
      'aria-required',
    );
  });
```

- [ ] **Step 2: Run the test to verify it fails**

```sh
cd apps/web && npx vitest run src/components/features/vehicles/CategoryCombobox.test.tsx
```

Expected: FAIL — TypeScript rejects `'aria-required'` on the props, and the attribute is absent on the trigger.

- [ ] **Step 3: Add the prop and forward it**

In `CategoryCombobox.tsx`, extend the existing injected-props doc comment and declaration (`:32-41`) so the new prop is documented in the same list rather than getting its own comment:

```tsx
  /**
   * Injected by FormControl (via Radix Slot) when this is used as a form
   * field: the id FormLabel's htmlFor points at, the ids of the description
   * and message nodes, the error flag, and the required flag. They land on the
   * trigger button — the element that actually takes focus — rather than on
   * the Popover root, which renders no DOM node of its own.
   */
  id?: string;
  'aria-describedby'?: string;
  'aria-invalid'?: boolean;
  'aria-required'?: boolean;
```

Destructure it alongside the others (`:64-65`):

```tsx
      'aria-describedby': ariaDescribedBy,
      'aria-invalid': ariaInvalid,
      'aria-required': ariaRequired,
```

Forward it on the trigger `<Button>` (after `aria-invalid`, `:149`):

```tsx
            aria-invalid={ariaInvalid}
            aria-required={ariaRequired}
```

- [ ] **Step 4: Run the test to verify it passes**

```sh
cd apps/web && npx vitest run src/components/features/vehicles/CategoryCombobox.test.tsx
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/components/features/vehicles/CategoryCombobox.tsx apps/web/src/components/features/vehicles/CategoryCombobox.test.tsx
git commit -m "feat(web): forward aria-required to the category combobox trigger"
```

---

## Task 3: `VehicleForm` — create-only marking, legend, and the affected page test

**Files:**
- Modify: `apps/web/src/components/features/vehicles/VehicleForm.tsx:48,64,77,90,178`
- Create: `apps/web/src/components/features/vehicles/VehicleForm.test.tsx`
- Modify: `apps/web/src/pages/VehiclesPage.test.tsx:84-86,107,115,134,203`

**Interfaces:**
- Consumes: `FormItem`'s `required` prop and `RequiredLegend` from Task 1.
- Produces: nothing later tasks depend on.

Marking per FR-14/FR-15: `make`, `model`, `year` → `required={isCreate}`; `nickname`, `trim`, `vin`, `currentMileage`, `notes` → unmarked. In edit mode those three render `disabled` and must carry neither marker nor `aria-required`.

- [ ] **Step 1: Write the failing test**

Create `apps/web/src/components/features/vehicles/VehicleForm.test.tsx`:

```tsx
import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { VehicleForm } from './VehicleForm';

// Queried by role + accessible name rather than by label text: the marker is a
// nested aria-hidden span, which dom-accessibility-api skips but Testing
// Library's label-content helper does not.
describe('VehicleForm required markers', () => {
  it('marks make, model and year in create mode', () => {
    render(<VehicleForm mode="create" onSubmit={vi.fn()} />);

    expect(screen.getByRole('textbox', { name: 'Make' })).toHaveAttribute(
      'aria-required',
      'true',
    );
    expect(screen.getByRole('textbox', { name: 'Model' })).toHaveAttribute(
      'aria-required',
      'true',
    );
    expect(screen.getByRole('spinbutton', { name: 'Year' })).toHaveAttribute(
      'aria-required',
      'true',
    );
  });

  it('leaves the optional fields unmarked in create mode', () => {
    render(<VehicleForm mode="create" onSubmit={vi.fn()} />);

    expect(screen.getByRole('textbox', { name: 'Nickname' })).not.toHaveAttribute(
      'aria-required',
    );
    expect(screen.getByRole('textbox', { name: 'VIN' })).not.toHaveAttribute('aria-required');
    expect(screen.getByRole('spinbutton', { name: 'Current Mileage' })).not.toHaveAttribute(
      'aria-required',
    );
  });

  // The user cannot change these in edit mode and cannot fail to supply them,
  // so an asterisk would be noise (FR-15).
  it('marks nothing in edit mode', () => {
    render(<VehicleForm mode="edit" onSubmit={vi.fn()} />);

    const make = screen.getByRole('textbox', { name: 'Make' });
    expect(make).toBeDisabled();
    expect(make).not.toHaveAttribute('aria-required');
    expect(screen.getByRole('textbox', { name: 'Model' })).not.toHaveAttribute('aria-required');
    expect(screen.getByRole('spinbutton', { name: 'Year' })).not.toHaveAttribute('aria-required');
  });

  it('renders the required legend', () => {
    render(<VehicleForm mode="create" onSubmit={vi.fn()} />);

    expect(screen.getByText('Required')).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

```sh
cd apps/web && npx vitest run src/components/features/vehicles/VehicleForm.test.tsx
```

Expected: FAIL — no `aria-required` on make/model/year, no legend.

- [ ] **Step 3: Mark the fields and add the legend**

In `VehicleForm.tsx`, add the import beside the form primitives:

```tsx
import { RequiredLegend } from '../../ui/required';
```

Change three opening tags (leave the other five `<FormItem>`s alone):

- `:64` → `<FormItem required={isCreate}>` (make)
- `:77` → `<FormItem required={isCreate}>` (model)
- `:90` → `<FormItem required={isCreate}>` (year)

Insert the legend between the `notes` field and the submit row (after the closing `/>` of the `notes` `FormField` at `:177`, before `<div className="flex justify-end gap-2">`):

```tsx
        <RequiredLegend />

        <div className="flex justify-end gap-2">
```

- [ ] **Step 4: Run the test to verify it passes**

```sh
cd apps/web && npx vitest run src/components/features/vehicles/VehicleForm.test.tsx
```

Expected: PASS, 4 tests.

- [ ] **Step 5: Run `VehiclesPage.test.tsx` and watch it break**

```sh
cd apps/web && npx vitest run src/pages/VehiclesPage.test.tsx
```

Expected: FAIL at the six `getByLabelText('Make'|'Model'|'Year')` call sites — the label content is now `Make *`. This is the known hazard, not a regression in the component.

- [ ] **Step 6: Move those queries to accessible names**

In `apps/web/src/pages/VehiclesPage.test.tsx`, make exactly these five edits. Do **not** loosen any query to a regex, and do not touch the `Nickname` queries at `:243,248` — that field is not marked.

`:83-85` inside `fillRequired`:

```tsx
  await userEvent.type(dialog.getByRole('textbox', { name: 'Make' }), 'Toyota');
  await userEvent.type(dialog.getByRole('textbox', { name: 'Model' }), 'Corolla');
  await userEvent.type(dialog.getByRole('spinbutton', { name: 'Year' }), '2020');
```

`:107`:

```tsx
    expect(within(dialog).getByRole('textbox', { name: 'Make' })).toBeInTheDocument();
```

`:115`:

```tsx
    expect(
      within(screen.getByRole('dialog')).getByRole('textbox', { name: 'Make' }),
    ).toBeInTheDocument();
```

`:134` — this one is a negative assertion, so it must be a query that could actually have matched:

```tsx
    expect(screen.queryByRole('textbox', { name: 'Make' })).not.toBeInTheDocument();
```

`:203`:

```tsx
    expect(within(screen.getByRole('dialog')).getByRole('textbox', { name: 'Make' })).toHaveValue(
      'Toyota',
    );
```

Add a short comment above `fillRequired` recording why:

```tsx
/**
 * Fills the three required fields. Queried by role + accessible name: the
 * required marker is a nested aria-hidden span, which the accessibility tree
 * skips but Testing Library's label-content helper concatenates, so
 * getByLabelText('Make') would look for "Make *".
 */
```

- [ ] **Step 7: Run both files to verify they pass**

```sh
cd apps/web && npx vitest run src/pages/VehiclesPage.test.tsx src/components/features/vehicles/VehicleForm.test.tsx
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add apps/web/src/components/features/vehicles/VehicleForm.tsx apps/web/src/components/features/vehicles/VehicleForm.test.tsx apps/web/src/pages/VehiclesPage.test.tsx
git commit -m "feat(web): mark required vehicle fields in create mode"
```

---

## Task 4: `FuelForm` — marking, the either/or note, legend

**Files:**
- Modify: `apps/web/src/components/features/vehicles/fuel/FuelForm.tsx:57,72,95,121,146,168-170`
- Create: `apps/web/src/components/features/vehicles/fuel/FuelForm.test.tsx`

**Interfaces:**
- Consumes: `FormItem`'s `required`, `RequiredLegend`, and `FormDescription` (already exported from `ui/form.tsx`).
- Produces: `export const COST_REQUIREMENT = 'Enter price per gallon or total cost (or both).'` from `FuelForm.tsx`, imported by the test.

`date`, `mileage`, `gallons` are marked. Neither cost field is: the rule is cross-field (`fuel.ts:29-38`), and marking both would claim both are mandatory. The rule is stated in prose instead — once visibly under the pair, and once `sr-only` inside **each** cost `FormItem` so `FormControl`'s existing `aria-describedby` resolves for both. A single shared `FormDescription` under the pair is not possible: `FormDescription` calls `useFormField()`, which throws outside a `FormItem` (`form.tsx:52-54`), and its id is scoped to one field.

- [ ] **Step 1: Write the failing test**

Create `apps/web/src/components/features/vehicles/fuel/FuelForm.test.tsx`:

```tsx
import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { FuelForm, COST_REQUIREMENT } from './FuelForm';

describe('FuelForm required markers', () => {
  it('marks date, mileage and gallons', () => {
    render(<FuelForm onSubmit={vi.fn()} />);

    // A datetime-local input has no mapped ARIA role, so it is reached through
    // its generated id rather than by role.
    const date = document.querySelector('input[type="datetime-local"]');
    expect(date).toHaveAttribute('aria-required', 'true');
    expect(screen.getByRole('spinbutton', { name: 'Mileage (miles)' })).toHaveAttribute(
      'aria-required',
      'true',
    );
    expect(screen.getByRole('spinbutton', { name: 'Gallons' })).toHaveAttribute(
      'aria-required',
      'true',
    );
  });

  // The requirement is cross-field, so an asterisk on either one would be a
  // lie (FR-21). It is stated in prose instead.
  it('marks neither cost field', () => {
    render(<FuelForm onSubmit={vi.fn()} />);

    expect(screen.getByRole('spinbutton', { name: 'Total Cost ($)' })).not.toHaveAttribute(
      'aria-required',
    );
    expect(screen.getByRole('spinbutton', { name: 'Price per Gallon ($)' })).not.toHaveAttribute(
      'aria-required',
    );
  });

  it('announces the either/or rule with each cost field', () => {
    render(<FuelForm onSubmit={vi.fn()} />);

    for (const name of ['Total Cost ($)', 'Price per Gallon ($)']) {
      const control = screen.getByRole('spinbutton', { name });
      const describedBy = control.getAttribute('aria-describedby');
      expect(describedBy).toBeTruthy();
      expect(document.getElementById(describedBy as string)).toHaveTextContent(COST_REQUIREMENT);
    }
  });

  it('states the rule once visibly, keeping the server-derives note', () => {
    render(<FuelForm onSubmit={vi.fn()} />);

    const visible = screen.getByText(/the server derives the missing value/i);
    expect(visible).toHaveTextContent(COST_REQUIREMENT.replace(/\.$/, ''));
    expect(visible.className).not.toContain('sr-only');
  });

  it('renders the required legend', () => {
    render(<FuelForm onSubmit={vi.fn()} />);

    expect(screen.getByText('Required')).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

```sh
cd apps/web && npx vitest run src/components/features/vehicles/fuel/FuelForm.test.tsx
```

Expected: FAIL — `COST_REQUIREMENT` is not exported, no `aria-required`, no legend.

- [ ] **Step 3: Implement**

In `FuelForm.tsx`, extend the primitives import and add the legend import:

```tsx
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '../../../ui/form';
import { RequiredLegend } from '../../../ui/required';
```

Add the constant above the component (after the `FuelFormProps` interface):

```tsx
/**
 * The either/or cost rule, stated where the user reads it and, sr-only, inside
 * each cost FormItem so it is announced with both fields. Exported so the
 * visible line and the two hidden copies cannot drift, and so the test asserts
 * against the same string the form renders. It mirrors the schema's own
 * message (`lib/schemas/fuel.ts:36`) — the up-front hint and the post-submit
 * error say the same thing.
 */
export const COST_REQUIREMENT = 'Enter price per gallon or total cost (or both).';
```

Mark three fields:

- `:57` → `<FormItem required>` (date)
- `:72` → `<FormItem required>` (mileage)
- `:95` → `<FormItem required>` (gallons)

Leave `:121` (totalCost) and `:146` (pricePerGallon) as bare `<FormItem>`, and add an `sr-only` description inside each, immediately after its `</FormControl>` and before its `<FormMessage />`:

```tsx
                </FormControl>
                {/* Visually duplicated by the shared line below; kept here so
                    aria-describedby resolves for this field too. A single
                    shared FormDescription is impossible — it calls
                    useFormField(), which throws outside a FormItem. */}
                <FormDescription className="sr-only">{COST_REQUIREMENT}</FormDescription>
                <FormMessage />
```

Replace the existing hint at `:168-170` with the constant plus its suffix, and add the legend below it:

```tsx
        <p className="text-sm text-muted-foreground">
          {COST_REQUIREMENT.replace(/\.$/, '')} — the server derives the missing value.
        </p>

        <RequiredLegend />

        <div className="flex justify-end gap-2">
```

The hint is about two specific fields and the legend is about the form, so the legend sits below it.

- [ ] **Step 4: Run the test to verify it passes**

```sh
cd apps/web && npx vitest run src/components/features/vehicles/fuel/FuelForm.test.tsx
```

Expected: PASS, 5 tests.

- [ ] **Step 5: Confirm the schema is untouched**

```sh
git diff --stat apps/web/src/lib/schemas/
```

Expected: empty output.

- [ ] **Step 6: Commit**

```bash
git add apps/web/src/components/features/vehicles/fuel/FuelForm.tsx apps/web/src/components/features/vehicles/fuel/FuelForm.test.tsx
git commit -m "feat(web): mark required fuel fields and state the cost either/or rule"
```

---

## Task 5: `MaintenanceRecordForm` — marking, legend, combobox slot coverage

**Files:**
- Modify: `apps/web/src/components/features/vehicles/maintenance/MaintenanceRecordForm.tsx:92,112,218`
- Modify: `apps/web/src/components/features/vehicles/maintenance/MaintenanceRecordForm.test.tsx`

**Interfaces:**
- Consumes: `FormItem`'s `required`, `RequiredLegend`, and Task 2's `aria-required` forwarding.
- Produces: nothing later tasks depend on.

`categoryId` and `performedAt` are marked; `description`, `mileage`, `cost`, `vendor`, `notes` are not.

- [ ] **Step 1: Write the failing test**

Append inside the existing `describe('MaintenanceRecordForm', …)` in `MaintenanceRecordForm.test.tsx`:

```tsx
  // The category control is a button, not an input — this is the case that
  // proves aria-required survives the Radix Slot hop onto a custom trigger.
  it('marks the category combobox required', () => {
    renderForm(
      <MaintenanceRecordForm categories={categories} kind="maintenance" onSubmit={vi.fn()} />,
    );

    expect(screen.getByRole('combobox', { name: /category/i })).toHaveAttribute(
      'aria-required',
      'true',
    );
  });

  it('leaves the optional fields unmarked', () => {
    renderForm(
      <MaintenanceRecordForm categories={categories} kind="maintenance" onSubmit={vi.fn()} />,
    );

    expect(screen.getByLabelText(/description/i)).not.toHaveAttribute('aria-required');
    expect(screen.getByRole('spinbutton', { name: 'Cost ($)' })).not.toHaveAttribute(
      'aria-required',
    );
  });

  it('renders the required legend', () => {
    renderForm(
      <MaintenanceRecordForm categories={categories} kind="maintenance" onSubmit={vi.fn()} />,
    );

    expect(screen.getByText('Required')).toBeInTheDocument();
  });
```

- [ ] **Step 2: Run the test to verify it fails**

```sh
cd apps/web && npx vitest run src/components/features/vehicles/maintenance/MaintenanceRecordForm.test.tsx
```

Expected: FAIL on the new `aria-required` and legend assertions. The pre-existing `getByText('Category')` test must still pass.

- [ ] **Step 3: Implement**

Add the import:

```tsx
import { RequiredLegend } from '../../../ui/required';
```

- `:92` → `<FormItem required>` (categoryId)
- `:112` → `<FormItem required>` (performedAt)

Insert the legend after `<AttachmentPicker … />` (closing at `:222`) and before the submit row:

```tsx
        <RequiredLegend />

        <div className="flex justify-end gap-2">
```

- [ ] **Step 4: Run the test to verify it passes**

```sh
cd apps/web && npx vitest run src/components/features/vehicles/maintenance/MaintenanceRecordForm.test.tsx
```

Expected: PASS, all 7 tests — including the untouched `associates the category label with the combobox trigger`, whose `screen.getByText('Category')` still matches because `getNodeText` reads only direct child text nodes.

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/components/features/vehicles/maintenance/MaintenanceRecordForm.tsx apps/web/src/components/features/vehicles/maintenance/MaintenanceRecordForm.test.tsx
git commit -m "feat(web): mark required maintenance record fields"
```

---

## Task 6: `MaintenanceScheduleForm` — reactive interval marking

**Files:**
- Modify: `apps/web/src/components/features/vehicles/maintenance/MaintenanceScheduleForm.tsx:59,79,103,129,150`
- Create: `apps/web/src/components/features/vehicles/maintenance/MaintenanceScheduleForm.test.tsx`

**Interfaces:**
- Consumes: `FormItem`'s `required`, `RequiredLegend`, Task 2's forwarding.
- Produces: nothing later tasks depend on.

`categoryId` and `recurrenceType` take a bare `required`. The two intervals take `required={showMonths}` / `required={showMiles}` — the same booleans that already govern their visibility (`:49-50`), so the schema's `superRefine` (`maintenanceSchedule.ts:29-47`), the visibility rule and the marker rule are one expression rather than three coincidences.

- [ ] **Step 1: Write the failing test**

Create `apps/web/src/components/features/vehicles/maintenance/MaintenanceScheduleForm.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeAll } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MaintenanceScheduleForm } from './MaintenanceScheduleForm';
import type { MaintenanceCategory } from '../../../../types/models/maintenanceCategory';

// cmdk scrolls its selected item into view; jsdom does not implement it.
beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn();
});

const categories: MaintenanceCategory[] = [
  {
    id: 'c1',
    type: 'maintenanceCategories',
    attributes: { name: 'Oil Change', systemDefined: true, kind: 'maintenance' },
  },
];

// CategoryCombobox mounts a mutation hook, so every render needs a provider.
function renderForm() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <MaintenanceScheduleForm categories={categories} onSubmit={vi.fn()} />
    </QueryClientProvider>,
  );
}

async function chooseRecurrence(label: RegExp) {
  await userEvent.click(screen.getByRole('combobox', { name: /recurrence type/i }));
  await userEvent.click(await screen.findByRole('option', { name: label }));
}

describe('MaintenanceScheduleForm required markers', () => {
  it('marks category and recurrence type', () => {
    renderForm();

    expect(screen.getByRole('combobox', { name: /category/i })).toHaveAttribute(
      'aria-required',
      'true',
    );
    expect(screen.getByRole('combobox', { name: /recurrence type/i })).toHaveAttribute(
      'aria-required',
      'true',
    );
  });

  // Default is 'time': months is shown and required, miles is not rendered at
  // all — so its absence is asserted as an absent element, not as a control
  // that happens to lack an attribute.
  it('marks the months interval and renders no miles interval for time', () => {
    renderForm();

    expect(screen.getByRole('spinbutton', { name: 'Every (months)' })).toHaveAttribute(
      'aria-required',
      'true',
    );
    expect(screen.queryByRole('spinbutton', { name: 'Every (miles)' })).not.toBeInTheDocument();
  });

  it('swaps to the miles interval for mileage', async () => {
    renderForm();
    await chooseRecurrence(/mileage-based/i);

    expect(screen.getByRole('spinbutton', { name: 'Every (miles)' })).toHaveAttribute(
      'aria-required',
      'true',
    );
    // No stale marker: the months field is gone, not merely unmarked.
    expect(screen.queryByRole('spinbutton', { name: 'Every (months)' })).not.toBeInTheDocument();
  });

  it('marks both intervals for hybrid', async () => {
    renderForm();
    await chooseRecurrence(/hybrid/i);

    expect(screen.getByRole('spinbutton', { name: 'Every (months)' })).toHaveAttribute(
      'aria-required',
      'true',
    );
    expect(screen.getByRole('spinbutton', { name: 'Every (miles)' })).toHaveAttribute(
      'aria-required',
      'true',
    );
  });

  it('renders the required legend', () => {
    renderForm();

    expect(screen.getByText('Required')).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

```sh
cd apps/web && npx vitest run src/components/features/vehicles/maintenance/MaintenanceScheduleForm.test.tsx
```

Expected: FAIL — no `aria-required` anywhere, no legend.

- [ ] **Step 3: Implement**

Add the import:

```tsx
import { RequiredLegend } from '../../../ui/required';
```

- `:59` → `<FormItem required>` (categoryId)
- `:79` → `<FormItem required>` (recurrenceType)
- `:103` → the months item, with the coupling comment FR-20 asks for, phrased in the schema's terms:

```tsx
              {/* Required for `time` and `hybrid` — the same rule the schema's
                  superRefine enforces (lib/schemas/maintenanceSchedule.ts:5-12)
                  and the same boolean that decides whether this field renders,
                  so schema, visibility and marker cannot drift apart. */}
              <FormItem required={showMonths}>
```

- `:129` → the miles item:

```tsx
              {/* Required for `mileage` and `hybrid` — same rule, same boolean
                  as the field's visibility. */}
              <FormItem required={showMiles}>
```

Insert the legend after the `{showMiles && (…)}` block (closing at `:148`) and before the submit row:

```tsx
        <RequiredLegend />

        <div className="flex justify-end gap-2">
```

- [ ] **Step 4: Run the test to verify it passes**

```sh
cd apps/web && npx vitest run src/components/features/vehicles/maintenance/MaintenanceScheduleForm.test.tsx
```

Expected: PASS, 5 tests.

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/components/features/vehicles/maintenance/MaintenanceScheduleForm.tsx apps/web/src/components/features/vehicles/maintenance/MaintenanceScheduleForm.test.tsx
git commit -m "feat(web): mark schedule interval fields from the same booleans that show them"
```

---

## Task 7: The five short forms

**Files:**
- Modify: `apps/web/src/components/features/vehicles/mileage/MileageForm.tsx:35`
- Modify: `apps/web/src/components/features/vehicles/dialogs/CompleteScheduleDialog.tsx:83`
- Modify: `apps/web/src/components/features/settings/FleetNameForm.tsx:46`
- Modify: `apps/web/src/components/features/settings/InviteForm.tsx:47,60`
- Modify: `apps/web/src/pages/OnboardingPage.tsx:115`
- Modify: `apps/web/src/components/features/settings/InviteForm.test.tsx`

**Interfaces:**
- Consumes: `FormItem`'s `required` from Task 1.
- Produces: nothing later tasks depend on.

None of these gets a legend — each renders fewer than three fields (FR-11/FR-12).

Marking:

| File | Field | `<FormItem required>` |
|---|---|---|
| `MileageForm.tsx:35` | `mileage` | yes |
| `CompleteScheduleDialog.tsx:83` | `date` | yes |
| `CompleteScheduleDialog.tsx:97` | `latestMileage` | **no** — auto-filled and clearable (FR-23) |
| `FleetNameForm.tsx:46` | `name` | yes |
| `InviteForm.tsx:47` | `email` | yes |
| `InviteForm.tsx:60` | `role` | yes — `FormControl` wraps the `SelectTrigger`, so `aria-required` lands on the trigger |
| `OnboardingPage.tsx:115` | `name` | yes |

`role` is marked even though `defaultValues` pre-fills it with `'member'` (`InviteForm.tsx:24`) and its placeholder never renders. Required-ness is a property of the field as the schema defines it, not of whether it happens to be empty at first paint; making the markup depend on initial state is exactly the re-derivation the guard test exists to prevent (design §11). No change to the default or the placeholder.

- [ ] **Step 1: Write the failing test**

Append inside the existing `describe('InviteForm', …)` in `InviteForm.test.tsx`:

```tsx
  // Role is required by the schema even though it opens pre-filled with
  // "Member" — required-ness is a property of the field, not of its initial
  // value. This is also the SelectTrigger slot case.
  it('marks both fields required', () => {
    renderWithProviders(<InviteForm fleetId="f1" />);

    expect(screen.getByLabelText(/email/i)).toHaveAttribute('aria-required', 'true');
    expect(screen.getByRole('combobox', { name: /role/i })).toHaveAttribute(
      'aria-required',
      'true',
    );
  });
```

- [ ] **Step 2: Run the test to verify it fails**

```sh
cd apps/web && npx vitest run src/components/features/settings/InviteForm.test.tsx
```

Expected: FAIL — neither control carries `aria-required`.

- [ ] **Step 3: Apply the six `required` flags**

Change these opening tags to `<FormItem required>`, and nothing else in these files:

- `apps/web/src/components/features/vehicles/mileage/MileageForm.tsx:35`
- `apps/web/src/components/features/vehicles/dialogs/CompleteScheduleDialog.tsx:83`
- `apps/web/src/components/features/settings/FleetNameForm.tsx:46`
- `apps/web/src/components/features/settings/InviteForm.tsx:47`
- `apps/web/src/components/features/settings/InviteForm.tsx:60`
- `apps/web/src/pages/OnboardingPage.tsx:115`

Leave `CompleteScheduleDialog.tsx:97` (`latestMileage`) as a bare `<FormItem>`.

- [ ] **Step 4: Run the affected tests to verify they pass**

```sh
cd apps/web && npx vitest run src/components/features/settings/InviteForm.test.tsx src/pages/OnboardingPage.test.tsx
```

Expected: PASS. `OnboardingPage.test.tsx`'s `findByLabelText(/fleet name/i)` still matches `Fleet Name *` because it is a regex.

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/components/features/vehicles/mileage/MileageForm.tsx apps/web/src/components/features/vehicles/dialogs/CompleteScheduleDialog.tsx apps/web/src/components/features/settings/FleetNameForm.tsx apps/web/src/components/features/settings/InviteForm.tsx apps/web/src/components/features/settings/InviteForm.test.tsx apps/web/src/pages/OnboardingPage.tsx
git commit -m "feat(web): mark required fields on the short forms"
```

---

## Task 8: The two hand-rolled surfaces

**Files:**
- Modify: `apps/web/src/components/features/settings/MemberList.tsx:273-277`
- Modify: `apps/web/src/components/admin/PurgeConfirmDialog.tsx:197-204`
- Modify: `apps/web/src/components/features/settings/MemberList.test.tsx`
- Modify: `apps/web/src/components/admin/PurgeConfirmDialog.test.tsx`

**Interfaces:**
- Consumes: `RequiredMarker` from Task 1 — imported directly, not through `ui/form.tsx`, because neither of these is a react-hook-form surface.
- Produces: nothing later tasks depend on.

- [ ] **Step 1: Write the failing tests**

Append inside the top-level `describe` in `MemberList.test.tsx`:

```tsx
  // Not a react-hook-form surface, so the marker and aria-required are applied
  // by hand from the same RequiredMarker the form primitives use.
  it('marks the successor picker required', async () => {
    seed(
      [membership('me', 'owner'), membership('other', 'member')],
      [userRow('other', 'Sam Ito', 'sam@example.com')],
    );

    renderWithProviders(<MemberList fleetId="f1" />);
    await userEvent.click(await screen.findByRole('button', { name: /^leave$/i }));

    expect(await screen.findByRole('combobox')).toHaveAttribute('aria-required', 'true');
  });
```

Append inside `describe('PurgeConfirmDialog', …)` in `PurgeConfirmDialog.test.tsx`, reusing that file's existing `props()` helper (`:6-19`):

```tsx
  // Not a react-hook-form surface either: typing the phrase is mandatory, so
  // the marker and aria-required are applied by hand.
  it('marks the confirmation input required', () => {
    render(<PurgeConfirmDialog {...props()} />);

    expect(screen.getByLabelText(/type the fleet name/i)).toHaveAttribute(
      'aria-required',
      'true',
    );
  });
```

The `MemberList` case reuses that file's existing `seed` / `membership` / `userRow` helpers, the same way `promotes the successor and then removes the leaver` (`MemberList.test.tsx:283`) does.

- [ ] **Step 2: Run the tests to verify they fail**

```sh
cd apps/web && npx vitest run src/components/features/settings/MemberList.test.tsx src/components/admin/PurgeConfirmDialog.test.tsx
```

Expected: FAIL — neither control carries `aria-required`.

- [ ] **Step 3: Mark the successor picker**

In `MemberList.tsx`, add the import:

```tsx
import { RequiredMarker } from '../../ui/required';
```

Replace `:273-277`:

```tsx
                  <label className="text-sm font-medium" htmlFor="successor">
                    New owner
                    <RequiredMarker />
                  </label>
                  <Select value={successorId} onValueChange={setSuccessorId}>
                    <SelectTrigger id="successor" aria-required="true">
```

- [ ] **Step 4: Mark the purge confirmation input**

In `PurgeConfirmDialog.tsx`, add the import beside the `Label` import:

```tsx
import { RequiredMarker } from '../ui/required';
```

Replace `:197-204`:

```tsx
          <div className="space-y-1">
            <Label htmlFor="purge-confirmation">
              {promptLabel}
              <RequiredMarker />
            </Label>
            <Input
              id="purge-confirmation"
              aria-required="true"
              value={typed}
              autoComplete="off"
              onChange={(e) => setTyped(e.target.value)}
            />
          </div>
```

- [ ] **Step 5: Run the tests to verify they pass**

```sh
cd apps/web && npx vitest run src/components/features/settings/MemberList.test.tsx src/components/admin/PurgeConfirmDialog.test.tsx
```

Expected: PASS. `PurgeConfirmDialog.test.tsx`'s existing `/type the fleet name/i` queries still match the now-marked label because they are regexes.

- [ ] **Step 6: Commit**

```bash
git add apps/web/src/components/features/settings/MemberList.tsx apps/web/src/components/features/settings/MemberList.test.tsx apps/web/src/components/admin/PurgeConfirmDialog.tsx apps/web/src/components/admin/PurgeConfirmDialog.test.tsx
git commit -m "feat(web): mark the successor picker and purge confirmation required"
```

---

## Task 9: The convention guard test

**Files:**
- Create: `apps/web/src/test/requiredFieldMarkers.test.ts`

**Interfaces:**
- Consumes: the finished source of all files touched in Tasks 3–8.
- Produces: nothing.

Source scanning, following `conventions.test.ts` / `sidebarTokens.test.ts` / `tailwindVarSyntax.test.ts`. Behavioural truth — that `required` actually produces a marker and an `aria-required` — is covered once in `form.test.tsx` and at the dynamic call sites; this file only asserts that each field opts in, so the cheap test stays cheap and the expensive one stays truthful.

- [ ] **Step 1: Write the test**

Create `apps/web/src/test/requiredFieldMarkers.test.ts`:

```ts
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, resolve } from 'node:path';
import { describe, it, expect } from 'vitest';

// src/test -> src
const SRC = resolve(dirname(fileURLToPath(import.meta.url)), '..');

/**
 * `true`  — a bare `<FormItem required>`.
 * `false` — no `required` at all.
 * string  — `required={<expr>}`, matched on the exact expression, so swapping a
 *           dynamic flag for a bare one (or the reverse) fails here.
 */
type Expectation = boolean | string;

/**
 * Which field of which form declares itself required. Read off the Zod schemas
 * in lib/schemas/, except where a deviation is annotated below — an
 * undocumented deviation must fail this test, a documented one must not.
 */
const EXPECTED: Record<string, Record<string, Expectation>> = {
  'components/features/vehicles/VehicleForm.tsx': {
    nickname: false,
    // Deviation (FR-15): the schema requires these unconditionally, but in
    // edit mode they render disabled — immutable after create — so the user
    // can neither change them nor fail to supply them, and an asterisk would
    // be noise. Bound to the form's existing isCreate flag.
    make: 'isCreate',
    model: 'isCreate',
    year: 'isCreate',
    trim: false,
    vin: false,
    currentMileage: false,
    notes: false,
  },
  'components/features/vehicles/fuel/FuelForm.tsx': {
    date: true,
    mileage: true,
    gallons: true,
    // Deviation (FR-21): the schema's superRefine requires *one of* these two.
    // Marking both would claim both are mandatory; marking neither would hide
    // the rule. Neither is marked, and the rule is stated in prose instead —
    // visibly under the pair and sr-only inside each FormItem.
    totalCost: false,
    pricePerGallon: false,
  },
  'components/features/vehicles/maintenance/MaintenanceRecordForm.tsx': {
    categoryId: true,
    performedAt: true,
    description: false,
    mileage: false,
    cost: false,
    vendor: false,
    notes: false,
  },
  'components/features/vehicles/maintenance/MaintenanceScheduleForm.tsx': {
    categoryId: true,
    recurrenceType: true,
    // Deviation (FR-19): required only for some recurrence types. Bound to the
    // same booleans that decide whether the field renders at all, so the
    // schema's superRefine, the visibility rule and the marker are one rule.
    intervalMonths: 'showMonths',
    intervalMiles: 'showMiles',
  },
  'components/features/vehicles/mileage/MileageForm.tsx': {
    mileage: true,
  },
  'components/features/vehicles/dialogs/CompleteScheduleDialog.tsx': {
    date: true,
    // Auto-filled from the latest mileage record and clearable (FR-23).
    latestMileage: false,
  },
  'components/features/settings/FleetNameForm.tsx': {
    name: true,
  },
  'components/features/settings/InviteForm.tsx': {
    email: true,
    // Marked despite opening pre-filled with "member": required-ness is a
    // property of the field as the schema defines it, not of its initial value.
    role: true,
  },
  'pages/OnboardingPage.tsx': {
    name: true,
  },
};

/** Forms with three or more rendered fields carry the legend (FR-10/FR-11). */
const EXPECTED_LEGEND: Record<string, boolean> = {
  'components/features/vehicles/VehicleForm.tsx': true,
  'components/features/vehicles/fuel/FuelForm.tsx': true,
  'components/features/vehicles/maintenance/MaintenanceRecordForm.tsx': true,
  'components/features/vehicles/maintenance/MaintenanceScheduleForm.tsx': true,
  'components/features/vehicles/mileage/MileageForm.tsx': false,
  'components/features/vehicles/dialogs/CompleteScheduleDialog.tsx': false,
  'components/features/settings/FleetNameForm.tsx': false,
  'components/features/settings/InviteForm.tsx': false,
  'pages/OnboardingPage.tsx': false,
};

function read(file: string): string {
  return readFileSync(resolve(SRC, file), 'utf8');
}

/** Accepts both `<FormItem required>` and `<FormItem required={expr}>`. */
function declaredRequired(block: string): Expectation {
  const match = /<FormItem\s+required(?:=\{([^}]*)\})?(?:\s|>)/.exec(block);
  if (!match) return false;
  return match[1] === undefined ? true : match[1].trim();
}

describe('required field markers', () => {
  // Every <FormField> in these files is a flat sibling — none nests another —
  // so splitting on the tag yields one block per field.
  it.each(Object.keys(EXPECTED))('%s declares each field as specified', (file) => {
    const blocks = read(file).split('<FormField').slice(1);
    const actual: Record<string, Expectation> = {};

    for (const block of blocks) {
      const name = /name="([^"]+)"/.exec(block)?.[1];
      expect(name, `a <FormField> in ${file} has no static name=""`).toBeTruthy();
      actual[name as string] = declaredRequired(block);
    }

    // Whole-object equality, so a newly added field nobody classified fails
    // loudly instead of passing by omission.
    expect(actual).toEqual(EXPECTED[file]);
  });

  it.each(Object.entries(EXPECTED_LEGEND))('%s legend presence is %s', (file, expected) => {
    expect(read(file).includes('<RequiredLegend />')).toBe(expected);
  });

  // The two surfaces outside react-hook-form get the same treatment by hand.
  it('marks the successor picker in the leave-fleet dialog', () => {
    const source = read('components/features/settings/MemberList.tsx');
    const start = source.indexOf('htmlFor="successor"');
    expect(start).toBeGreaterThan(-1);
    const region = source.slice(start, source.indexOf('</Select>', start));

    expect(region).toContain('<RequiredMarker />');
    expect(region).toContain('aria-required="true"');
  });

  it('marks the purge confirmation input', () => {
    const source = read('components/admin/PurgeConfirmDialog.tsx');
    const start = source.indexOf('htmlFor="purge-confirmation"');
    expect(start).toBeGreaterThan(-1);
    // Delimited by the wrapping </div>, not by the first `/>` — the marker is
    // itself a self-closing tag and would truncate the region before the Input.
    const region = source.slice(start, source.indexOf('</div>', start));

    expect(region).toContain('<RequiredMarker />');
    expect(region).toContain('aria-required="true"');
  });
});
```

- [ ] **Step 2: Run the test to verify it passes against the finished forms**

```sh
cd apps/web && npx vitest run src/test/requiredFieldMarkers.test.ts
```

Expected: PASS.

- [ ] **Step 3: Prove the guard actually guards**

Temporarily delete the `required` from `MileageForm.tsx:35` (`<FormItem required>` → `<FormItem>`), re-run, and confirm a failure naming `mileage`:

```sh
cd apps/web && npx vitest run src/test/requiredFieldMarkers.test.ts
```

Expected: FAIL. Then restore the flag and re-run; expected: PASS. Do not commit the temporary edit.

- [ ] **Step 4: Commit**

```bash
git add apps/web/src/test/requiredFieldMarkers.test.ts
git commit -m "test(web): pin which form fields declare themselves required"
```

---

## Task 10: Guidelines and the reviewer rule

**Files:**
- Modify: `.claude/skills/frontend-dev-guidelines/resources/patterns-forms-validation.md`
- Modify: `.claude/agents/frontend-guidelines-reviewer.md:148-149`

**Interfaces:**
- Consumes: the finished convention.
- Produces: `FE-18` — the next free number (the checklist currently ends at `FE-17`; confirm with `grep -o "FE-[0-9]\+" .claude/agents/frontend-guidelines-reviewer.md | sort -u -V | tail -1` before writing).

- [ ] **Step 1: Confirm the next free `FE-` number**

```sh
grep -rho "FE-[0-9]\+" .claude/ | sort -u -V | tail -1
```

Expected: `FE-17`, so the new rule is `FE-18`. If it is higher, use the next one after it and adjust every reference below.

- [ ] **Step 2: Document the convention**

Append to `.claude/skills/frontend-dev-guidelines/resources/patterns-forms-validation.md`:

````markdown
## Required field indicators

`FE-18`. Required-ness is declared **once per field, on `FormItem`** — never on `FormLabel`, never at both:

```tsx
<FormItem required>
  <FormLabel>Make</FormLabel>
  <FormControl>
    <Input type="text" {...field} />
  </FormControl>
  <FormMessage />
</FormItem>
```

`FormItemContext` carries the flag. `FormLabel` reads it and renders `<RequiredMarker />`; `FormControl` reads it and emits `aria-required="true"` through the Radix `Slot`. Two consumers, one declaration, so the glyph and the semantics cannot disagree.

- **The marker is `text-danger`, not `text-destructive`.** `--destructive` measures ~2:1 against the dark background; `--danger` is 6.67:1 light / 7.23:1 dark (`docs/tasks/task-003-dark-mode-branding/contrast.md:18,27`). `index.css:104-108` records the split: bare status tokens are for text on `--background` / `--card`, `--destructive` styles destructive *controls*.
- **The marker is `aria-hidden`.** The accessible name comes from the label text and the required state from `aria-required`; announcing "Make star" is a regression. It also keeps exact-string `getByText('Category')` assertions passing, because `getNodeText` reads only direct child text nodes.
- **Never the native `required` attribute** — it triggers browser validation bubbles that compete with `FormMessage`.
- **`aria-required={required || undefined}`**, never a bare boolean: React renders `false` as the string `"false"`, and an unmarked control must carry no attribute at all.
- **Custom controls must forward the prop.** `Input`, `Textarea` and `SelectTrigger` spread arbitrary props, so `Slot` injection reaches them for free. A control that destructures its props — `CategoryCombobox` — has to declare `'aria-required'?: boolean` and pass it to the focusable trigger.

### Legend

Forms rendering **three or more** fields close the field stack, above the submit row, with `<RequiredLegend />` (`* Required`). Shorter forms do not: a lone asterisk on a one- or two-field form is self-explanatory and the legend would outweigh it. The threshold is a judgement, so it is asserted per file in `test/requiredFieldMarkers.test.ts` — adding a fourth field to a short form fails that test and prompts the decision instead of drifting.

### Conditional required-ness

A field that is required only sometimes binds to **the same boolean that governs its visibility**, not to a second condition that can drift from the schema:

```tsx
const showMonths = recurrenceType === 'time' || recurrenceType === 'hybrid';
…
{showMonths && (
  <FormField … render={({ field }) => (
    <FormItem required={showMonths}>
```

The schema's `superRefine`, the visibility rule and the marker then read as one rule rather than three coincidences.

### Cross-field requirements

A rule like "price per gallon **or** total cost" is not expressible as a per-field asterisk — marking both claims both are mandatory, marking neither hides the rule. Neither field is marked; the rule is stated in prose, word-identical to the schema's own message. `FuelForm.tsx` is the reference: one exported constant, rendered visibly under the pair, plus an `sr-only` `FormDescription` **inside each** `FormItem` so `FormControl`'s existing `aria-describedby` resolves for both.

A single shared `FormDescription` under the pair does not work: it calls `useFormField()`, which throws outside a `FormItem`, and its id is scoped to one field.

### Drift protection

`apps/web/src/test/requiredFieldMarkers.test.ts` records every field of every form and fails on an unclassified new one. Deviations from the schema's static optionality are allowed, but each must be annotated in that table with its reason.
````

- [ ] **Step 3: Add the reviewer rule**

In `.claude/agents/frontend-guidelines-reviewer.md`, add a row to the **Architecture Checklist** table, immediately after the `FE-14` row:

```markdown
| FE-18 | Required fields are marked | For each changed form, read its Zod schema in `lib/schemas/` and compare against the `<FormItem>` tags: `grep -n "<FormItem" <form file>` | Every `FormField` whose schema field is statically required declares `<FormItem required>` — on `FormItem`, never on `FormLabel`, and never on both. Conditionally-required fields bind to the same boolean as their visibility. Deviations are listed in `apps/web/src/test/requiredFieldMarkers.test.ts` with a reason; an unlisted deviation is a FAIL. Forms with 3+ fields render `<RequiredLegend />`. See `.claude/skills/frontend-dev-guidelines/resources/patterns-forms-validation.md` → "Required field indicators". |
```

- [ ] **Step 4: Verify the cross-references resolve**

```sh
grep -n "FE-18" .claude/agents/frontend-guidelines-reviewer.md .claude/skills/frontend-dev-guidelines/resources/patterns-forms-validation.md
```

Expected: at least one hit in each file.

- [ ] **Step 5: Commit**

```bash
git add .claude/skills/frontend-dev-guidelines/resources/patterns-forms-validation.md .claude/agents/frontend-guidelines-reviewer.md
git commit -m "docs: record the required-field-indicator convention as FE-18"
```

---

## Task 11: Full verification

**Files:** none modified — this task only runs checks and fixes anything they surface.

- [ ] **Step 1: Lint, test and build the frontend**

```sh
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
make fe-test
make fe-build
```

Expected: both PASS. Capture the output — the PR body quotes it.

- [ ] **Step 2: Confirm no schema was touched**

```sh
git diff --stat main...HEAD -- apps/web/src/lib/schemas/
```

Expected: empty output.

- [ ] **Step 3: Confirm the two regression-sensitive test files were not edited**

```sh
git diff --stat main...HEAD -- apps/web/src/lib/schemas/fuel.test.ts apps/web/src/lib/schemas/maintenanceSchedule.test.ts
grep -n "getByText('Category')" apps/web/src/components/features/vehicles/maintenance/MaintenanceRecordForm.test.tsx
```

Expected: empty diff, and the `getByText('Category')` assertion still present and unmodified. (If the schema tests live elsewhere, locate them with `git ls-files "*fuel.test.ts" "*maintenanceSchedule.test.ts"` and diff those paths.)

- [ ] **Step 4: Confirm no native `required` attribute crept in**

```sh
grep -rn 'required="true"\|required={true}' apps/web/src --include=*.tsx | grep -v aria-required
```

Expected: no output.

- [ ] **Step 5: Look at the marker in both themes**

Run the dev server, open a form with a required field (the Add Vehicle dialog is the quickest), and check the asterisk in light and dark:

```sh
cd apps/web && npm run dev
```

The token choice is argued from recorded measurements, but the glyph is a few pixels of stroke — confirm it reads at a glance in both themes and that it stays visible on a field in its error state (submit the empty form). If the resting-state red is too loud, adjust with opacity on the marker span, not by changing the token.

- [ ] **Step 6: Commit anything the checks surfaced**

If steps 1–5 required no change, there is nothing to commit. Otherwise:

```bash
git add -A
git commit -m "fix(web): address verification findings for required field indicators"
```

- [ ] **Step 7: Run the code review before opening a PR**

Per `CLAUDE.md`, dispatch `superpowers:requesting-code-review` (or `/audit-plan`) and resolve its findings before the PR. Findings land in `docs/tasks/task-028-required-field-indicators/audit.md`.
