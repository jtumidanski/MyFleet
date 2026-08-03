# Forms & Validation Patterns

## Overview

Forms are react-hook-form + Zod through `zodResolver`, rendered with the shadcn/ui `Form` primitives. Schemas live in `apps/web/src/lib/schemas/<resource>.ts` — resource-named, **no `.schema.` infix**: `vehicle.ts`, `fuel.ts`, `maintenanceSchedule.ts`, `maintenanceRecord.ts`, `mileage.ts`, `fleet.ts`, `fleetSettings.ts`, `runtimeConfig.ts`.

Two rules the reviewer checks:

- **`FE-13`** — every form uses `useForm({ resolver: zodResolver(schema) })`.
- **`FE-14`** — every `z.object` lives in `lib/schemas/` and is paired with an exported `z.infer` type. No inline schema in a component (`FE-04`), with `.refine()` / `.superRefine()` on an imported schema the explicit exception.

## Basic schema

```typescript
// apps/web/src/lib/schemas/vehicle.ts
import { z } from 'zod';

const currentYear = new Date().getFullYear();

// Create/edit vehicle form. make/model/year are required (design + plan);
// the rest are optional. Year is bounded to plausible values.
export const vehicleSchema = z.object({
  make: z.string().trim().min(1, 'Make is required'),
  model: z.string().trim().min(1, 'Model is required'),
  year: z
    .number({ invalid_type_error: 'Year is required' })
    .int('Year must be a whole number')
    .min(1900, 'Year looks too old')
    .max(currentYear + 2, 'Year looks too far in the future'),
  nickname: z.string().trim().max(120).optional().or(z.literal('')),
  currentMileage: z
    .number({ invalid_type_error: 'Mileage must be a number' })
    .int()
    .min(0, 'Mileage cannot be negative')
    .optional(),
  notes: z.string().trim().max(2000).optional().or(z.literal('')),
});

export type VehicleFormInput = z.infer<typeof vehicleSchema>;
```

Conventions visible here and worth copying:

- **Every message is user-facing prose.** `'Year looks too old'`, not `'Invalid year'`. These strings render directly under the field via `<FormMessage />`.
- **`invalid_type_error` on every numeric field.** Without it an empty number input produces Zod's default `"Expected number, received nan"`.
- **`.optional().or(z.literal(''))` for optional text.** A cleared input is `''`, not `undefined`; `.optional()` alone rejects it. The empty strings are stripped at the boundary — `VehiclesPage.tsx:20-30` maps `values.nickname || undefined` before the attributes go to the service.
- **The exported type is named `<Resource>FormInput`**, and it is the form's shape, not the API's. `VehicleFormInput` and `CreateVehicleAttributes` are deliberately different types with a mapping function between them.
- **No defaults object is exported.** Schema modules export the schema and its inferred type, nothing else; defaults are declared at the `useForm` call site where the mode (create vs edit) is known.

Schema modules import only `zod`. There is no shared-primitives module and no cross-module schema import — a validator repeated in two schemas is repeated deliberately, because the two forms' messages differ.

## Cross-field validation

`.superRefine()` on the object, with `path` set so the error lands on a field the user can see:

```typescript
// apps/web/src/lib/schemas/fuel.ts:29-39
.superRefine((data, ctx) => {
  const hasTotal = data.totalCost != null && data.totalCost > 0;
  const hasPrice = data.pricePerGallon != null && data.pricePerGallon > 0;
  if (!hasTotal && !hasPrice) {
    ctx.addIssue({
      code: z.ZodIssueCode.custom,
      path: ['pricePerGallon'],
      message: 'Provide price per gallon or total cost (or both)',
    });
  }
});
```

`maintenanceSchedule.ts:30-48` is the conditional-requirement variant: `intervalMonths` is required when `recurrenceType` is `time` or `hybrid`, `intervalMiles` when it is `mileage` or `hybrid`, and each issue is pushed at the field it concerns. The rule table is written out in the schema's doc comment (`maintenanceSchedule.ts:5-12`) — do that, because a `superRefine` body is not readable as a specification.

Both files declare the base fields `.optional()` and then add the conditional requirement in `superRefine`. That ordering matters: a field cannot be both statically required and conditionally required.

There is no `z.discriminatedUnion` anywhere in `apps/web/src`. If a form genuinely needs one, it will be the first — say so in a comment.

## Form component

Every form is a standalone component taking `onSubmit`, optional `onCancel`, and `submitting`. It owns the `useForm` call and knows nothing about mutations or dialogs.

```typescript
// components/features/vehicles/VehicleForm.tsx:1-8,28-43
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { Loader2 } from 'lucide-react';
import { vehicleSchema, type VehicleFormInput } from '../../../lib/schemas/vehicle';
import { Button } from '../../ui/button';
import { Input } from '../../ui/input';
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from '../../ui/form';

export function VehicleForm({ mode, defaultValues, onSubmit, onCancel, submitting }: VehicleFormProps) {
  const isCreate = mode === 'create';
  const form = useForm<VehicleFormInput>({
    resolver: zodResolver(vehicleSchema),
    defaultValues: { make: '', model: '', nickname: '', notes: '', ...defaultValues },
  });

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit((values) => onSubmit(values))} className="space-y-4">
        {/* fields */}
      </form>
    </Form>
  );
}
```

Imports are relative at whatever depth the component sits (`../../ui/form`). There is no `@/` alias.

Each field is a `FormField` with a `render` prop; `FormMessage` renders the Zod error with no wiring:

```typescript
<FormField
  control={form.control}
  name="nickname"
  render={({ field }) => (
    <FormItem>
      <FormLabel>Nickname</FormLabel>
      <FormControl>
        <Input type="text" {...field} value={field.value ?? ''} />
      </FormControl>
      <FormMessage />
    </FormItem>
  )}
/>
```

**Number inputs cannot use the `{...field}` spread.** A native number input reports `''` for empty and a string otherwise, so spreading `field` puts a string where the schema wants a number and every submit fails validation on an untouched optional field. Destructure instead (`VehicleForm.tsx:93-103`):

```typescript
<Input
  type="number"
  value={field.value ?? ''}
  onChange={(e) => field.onChange(e.target.value === '' ? undefined : e.target.valueAsNumber)}
  onBlur={field.onBlur}
  name={field.name}
  ref={field.ref}
/>
```

`value={field.value ?? ''}` on every input, number or text: an `undefined` value makes React treat the input as uncontrolled and log a warning on first keystroke.

Every form uses `FormField`; bare `register()` has 0 call sites in `apps/web/src`.

## Conditionally shown fields

`useWatch` on the field that drives the condition, never `form.watch()` in render:

```typescript
// components/features/vehicles/maintenance/MaintenanceScheduleForm.tsx:45-49
const recurrenceType = useWatch({ control: form.control, name: 'recurrenceType' });

const showMonths = recurrenceType === 'time' || recurrenceType === 'hybrid';
const showMiles = recurrenceType === 'mileage' || recurrenceType === 'hybrid';
```

The visibility conditions must mirror the schema's `superRefine` conditions exactly. When they drift, the form hides a field the resolver still requires and the user gets an error with nothing to fix. Both are written out in the same words in `maintenanceSchedule.ts:8-11` and `MaintenanceScheduleForm.tsx:24-27` for precisely that reason.

## Submit, error display, and the mutation

The form does not know about the API. Its consumer — a dialog or a page — owns the mutation, the toast, and the close:

```typescript
// components/features/vehicles/dialogs/EditVehicleDialog.tsx:24-38
const handleUpdate = async (values: VehicleFormInput) => {
  const patch: UpdateVehicleAttributes = {
    nickname: values.nickname || undefined,
    currentMileage: values.currentMileage,
    notes: values.notes || undefined,
  };
  try {
    await updateVehicle.mutateAsync({ id: vehicleId, attributes: patch });
    toast.success('Vehicle updated');
    onOpenChange(false);
  } catch (err) {
    const apiError = createErrorFromUnknown(err);
    toast.error(apiError.message || 'Could not update vehicle');
  }
};
```

- **`mutateAsync`, not `mutate`** — the handler needs to await the outcome to decide whether to close.
- **`createErrorFromUnknown` from `@myfleet/shared-ts`** (`EditVehicleDialog.tsx:2`), never a bespoke error class. `FE-09` checks this.
- **`|| 'Could not update vehicle'`** — `ApiError` degrades to an empty-ish message when the response carried no JSON:API error envelope (`packages/shared-ts/src/errors.ts:35-36`), so the fallback is not defensive padding.
- **Close only on success.** `VehiclesPage.tsx:61-66` says why: *"Leave the dialog open so the typed values survive for a retry."* Closing on failure discards everything the user typed.
- **Map form input to attributes explicitly.** `VehiclesPage.tsx:20-30` strips empty-string optionals so the backend receives clean attributes. Passing `values` straight through sends `""` where the API expects the field absent.

## Dialog close behaviour

A dialog wrapping a form has to survive an in-flight submit. `VehiclesPage.tsx:81-104` is the reference:

```typescript
<Dialog
  open={open}
  onOpenChange={(next) => {
    // Backstop: `dismissible` already blocks the three user-facing routes,
    // but this guarantees no dismissal path Radix grows later can close the
    // dialog out from under an in-flight create.
    if (!next && createVehicle.isPending) return;
    setOpen(next);
  }}
>
  {/* Unmounted on close, which is what discards the form state — do not
      add forceMount. */}
  <DialogContent dismissible={!createVehicle.isPending} …>
```

Three things to carry over:

1. **Block close while the mutation is pending**, in both `dismissible` and the `onOpenChange` guard. The prop handles the routes that exist; the guard handles the ones the library may grow.
2. **Do not add `forceMount`.** Unmounting on close is what resets the form; with `forceMount` the next open shows the previous attempt's values.
3. **Mind where focus lands on close.** `VehiclesPage.tsx:97-103` redirects `onCloseAutoFocus` to the header button when the dialog was opened from the empty state, because that opener unmounts once the first vehicle exists — restoring focus to a detached node drops it to `<body>`.

The `Form` component is unmounted with the dialog, so there is no `form.reset()` call anywhere. Do not add one to work around a dialog that was kept mounted.
