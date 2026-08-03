# AI Code Generation Guidance

## Mandatory Implementation Workflow

When generating or modifying MyFleet UI code, **always** follow this sequence:

1. **Read existing files** before editing — understand the patterns already in use
2. **Check the component location** — `ui/`, `features/`, `frame/`, `admin/`, or `packages/ui-components`?
3. **Implement changes** following those patterns
4. **Run tests**: `make fe-test`
5. **Fix failures** — do not proceed with failing tests
6. **Verify the build**: `make fe-build`
7. **Report actual results** — never assume success

`make fe-test` and `make fe-build` are the targets `make ci` runs, so they are what CI will judge you by. Node may need loading first: `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22`.

## Core Rules

### 1. Follow existing patterns
When adding to an existing file or directory, match what is already there. Don't introduce a new convention without being asked.

### 2. Read before write
Read a file before editing it.

### 3. Type everything
TypeScript strict mode is on. Never `any`. Use `unknown` plus a type predicate for genuinely dynamic input, and `z.infer<>` for form types. (`FE-01`)

### 4. JSON:API model structure
Domain models are `JsonApiResource<A>` — `{ id, type, attributes }`. Don't flatten. Read through `.attributes.fieldName`. (`FE-10`)

### 5. Use the component library
shadcn/ui primitives live in `components/ui/`. Don't hand-roll a primitive that already exists. Cross-app presentational components live in `packages/ui-components` (`StatusBadge`, the formatters) — app-shell furniture stays in `apps/web`.

### 6. Use `cn()` for classnames
`cn()` from `lib/utils.ts` for conditional or merged classes. Never string concatenation. (`FE-02`)

### 7. Use sonner for user feedback
`toast.success()` / `toast.error()` from `sonner`. Not `alert()`, not `console.log()`.

### 8. Skeleton loading
`<Skeleton>` for content areas; `<Loader2 className="animate-spin">` only inside a submit button. (`FE-05`)

### 9. Forms use react-hook-form + Zod
Schema in `lib/schemas/<resource>.ts`, `zodResolver(schema)`, shadcn `Form`/`FormField` for rendering. (`FE-13`, `FE-14`)

### 10. Immutable state updates
Never mutate. Spread, `filter`, `map`. Query results in particular are shared with every other subscriber. (`FE-07`)

### 11. Named exports
Named exports for all components. There are zero default exports in `apps/web/src`. (`FE-08`)

### 12. Title case for interactive text
Button labels, dialog titles, badge text and tab labels use title case; interior prepositions and articles stay lowercase. See [Component Patterns — Text Casing Rules](patterns-components.md#text-casing-rules).

### 13. Cursor pointer on clickable elements
Every clickable element shows `cursor-pointer`. `<Button>` handles this; a clickable `div` or card does not. See [Styling — Cursor affordance for interactive elements](patterns-styling.md). (`FE-15`)

### 14. Capitalize enum display values
Backend enums arrive lowercase (`'owner'`, `'member'`, `'viewer'`) and the wire value must stay lowercase. Capitalize with the Tailwind `capitalize` class at the render site — `OnboardingPage.tsx:57`, `MemberList.tsx:158`, `InviteList.tsx:94` — not by rewriting the value.

There is **no enum + label-map convention** here: `grep -rn "^export enum" apps/web/src/types/models/` returns nothing, and there is no `Labels: Record<…>` anywhere. Attribute discriminants are plain string unions. Do not introduce a label map to satisfy this rule.

## Generation Workflow

When creating a new resource end to end, create files in this order.

### Step 1: Types
```
types/models/<resource>.ts
```
- `export interface <Resource>Attributes { … }`
- `export type <Resource> = JsonApiResource<<Resource>Attributes>` (`JsonApiResource` from `@myfleet/shared-ts`)
- Separate `Create<Resource>Attributes` / `Update<Resource>Attributes` for the write payloads — the create shape is not the read shape, because server-derived fields are read-only
- String unions for discriminants, not enums

There is no `types/api/`. Envelope types (`JsonApiDocument`, `PageMeta`, `ApiError`) come from `@myfleet/shared-ts`.

### Step 2: Service
```
services/api/<Resource>Service.ts     ← PascalCase, matches the class
```
- `class <Resource>Service extends BaseService<A, CreateA, UpdateA>`
- Set `resourceType` (the JSON:API `type`) and `basePath` (the full gateway path, e.g. `/api/fleet/vehicles`)
- Add resource-specific methods on top of the inherited `list`/`get`/`create`/`patch`/`remove`, using the protected `listAt`/`createAt` for nested routes
- `export const <resource>Service = new <Resource>Service();` — a singleton, imported directly

**There is no `services/api/index.ts` barrel.** Import the singleton from its own module: `import { vehicleService } from '../../../services/api/VehicleService';`

### Step 3: React Query hooks
```
lib/hooks/api/<resource>.ts           ← plural resource name, no `use` prefix on the file
```
- A hierarchical key factory with `as const` at every tier (`all` → `lists()` → `list(params)`, `all` → `details()` → `detail(id)`)
- Query hooks take the nullable id directly and gate with `enabled: !!id` — the caller should not have to hold the hook back
- `staleTime` per hook, chosen for that resource; there is no global default
- Mutations invalidate in **`onSettled`**, not `onSuccess`, so a failed write still re-reads authoritative state

Copy the shape from `lib/hooks/api/vehicles.ts:14-90`.

### Step 4: Zod schema (if the resource has a form)
```
lib/schemas/<resource>.ts             ← no `.schema.` infix
```
- `export const <resource>Schema = z.object({ … })` with per-field messages
- `export type <Resource>FormInput = z.infer<typeof <resource>Schema>`
- A sibling `<resource>.test.ts` where the rules are non-trivial (`fuel.test.ts`, `maintenanceSchedule.test.ts` exist)

### Step 5: Feature components
```
components/features/<resource>/
├── <Resource>List.tsx           presentational; takes data + an empty-state node as props
├── <Resource>Card.tsx
├── <Resource>Form.tsx           useForm + zodResolver; owns fields, not the mutation
└── dialogs/
    ├── Add<Resource>Dialog.tsx  owns the mutation and the close-on-success rule
    └── Delete<Resource>Dialog.tsx
```

The split matters: the form component takes `onSubmit`/`onCancel`/`submitting` props and knows nothing about React Query, so it is testable without providers and reusable in a page as well as a dialog.

### Step 6: Pages
```
pages/<Resource>sPage.tsx        list
pages/<Resource>DetailPage.tsx   detail
```
- The page calls the **hook**, never the service (`FE-03`)
- The page owns dialog state, permission decisions and the form-input → API-attributes mapping
- Container is `space-y-6`; `PageHeader` renders the header row only

### Step 7: Navigation
- Add a `<Route>` to `App.tsx` — inside the `AppLayout` layout route for a fleet page, or under the `/admin` route for a console page
- Add an entry to `NAV` in `AppLayout.tsx` (or `ADMIN_NAV` in `AdminLayout.tsx`) if it is a top-level destination
- Add a trail row to `components/frame/breadcrumbTrails.ts`, keyed by route pattern

There is no `components/app-sidebar.tsx` and no `lib/breadcrumbs/`.

### Step 8: Tests
- Sibling `*.test.ts` / `*.test.tsx` next to the file under test — there is no separate tests directory, and no directory of ambient module mocks: every stub is a `vi.mock` in the test file that needs it
- Vitest: `import { describe, it, expect, vi } from 'vitest'`
- Anything using a hook or a `<Link>` renders through `renderWithProviders` (`src/test/renderWithProviders.tsx`), which supplies a retry-free `QueryClient` and a `MemoryRouter`
- Stub the service module with `vi.mock('../../../services/api/MileageService', …)` (`lib/hooks/api/mileage.test.ts:11`) — the specifier is the real relative path, so a moved service breaks the mock loudly

## Validation Rules

Before submitting code, verify:

| Rule | Check |
|------|-------|
| No `any` types | `grep -rn ": any\|as any" apps/web/src` |
| No hardcoded colors | Covered by `src/test/conventions.test.ts` — `make fe-test` fails on a hit |
| `cn()` used for classes | No manual string concatenation in `className` |
| Forms use Zod | Every `useForm` has `resolver: zodResolver(...)` |
| Named exports | `grep -rn "export default" apps/web/src` returns nothing |
| Error handling | Every `catch` calls `createErrorFromUnknown(err)` and surfaces it |
| Skeleton loading | `animate-spin` only inside a submit button |
| No `@/` alias | `grep -rnE "['\"]@/" apps/web/src` returns nothing — no alias is configured, so every import is relative |
| Title-case labels | Buttons, dialog titles and badges use title case |
| Cursor pointer | Clickable non-`<button>`/`<a>` elements carry `cursor-pointer` |
| Enum display casing | Rendered with the `capitalize` class; the wire value stays lowercase |

Two of these are already executable and will fail `make fe-test` on their own: the palette check and the "authenticated pages contain no `<h1>`" check, both in `src/test/conventions.test.ts`.

## Common Composition Examples

### Adding a dialog with a form

The dialog owns the mutation; the form owns the fields. From `LogMileageDialog.tsx:27-54`:

```tsx
export function LogMileageDialog({ open, onOpenChange, vehicleId, defaultMileage }: Props) {
  const createRecord = useCreateMileageRecord(vehicleId);

  const handleSubmit = async (values: MileageFormInput) => {
    try {
      await createRecord.mutateAsync({ mileage: values.mileage });
      toast.success('Mileage logged');
      onOpenChange(false);
    } catch (err) {
      // Keep the dialog open so the user's input survives the failure.
      const apiError = createErrorFromUnknown(err);
      toast.error(apiError.message || 'Could not log mileage');
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Log Mileage</DialogTitle>
        </DialogHeader>
        <MileageForm
          defaultMileage={defaultMileage}
          onSubmit={handleSubmit}
          onCancel={() => onOpenChange(false)}
          submitting={createRecord.isPending}
        />
      </DialogContent>
    </Dialog>
  );
}
```

And the form it wraps (`MileageForm.tsx:20-66`, abridged):

```tsx
export function MileageForm({ defaultMileage, onSubmit, onCancel, submitting }: MileageFormProps) {
  const form = useForm<MileageFormInput>({
    resolver: zodResolver(mileageSchema),
    defaultValues: { mileage: defaultMileage },
  });

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit((values) => onSubmit(values))} className="space-y-4">
        <FormField
          control={form.control}
          name="mileage"
          render={({ field }) => (
            <FormItem>
              <FormLabel>Mileage (miles)</FormLabel>
              <FormControl>
                <Input type="number" value={field.value ?? ''} … />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <div className="flex justify-end gap-2">
          {onCancel && <Button type="button" variant="outline" onClick={onCancel}>Cancel</Button>}
          <Button type="submit" disabled={submitting}>
            {submitting && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            Log Mileage
          </Button>
        </div>
      </form>
    </Form>
  );
}
```

Note `submitting` is a prop, not `form.formState.isSubmitting` — the mutation's `isPending` is the truth, and it lives in the dialog.

### Adding a new hook

From `lib/hooks/api/vehicles.ts:14-56`:

```typescript
export const vehicleKeys = {
  all: ['vehicles'] as const,
  lists: () => [...vehicleKeys.all, 'list'] as const,
  list: (params: { fleetId: string }) => [...vehicleKeys.lists(), params] as const,
  details: () => [...vehicleKeys.all, 'detail'] as const,
  detail: (id: string) => [...vehicleKeys.details(), id] as const,
};

export function useVehicles(fleetId: string | null | undefined) {
  return useQuery({
    queryKey: vehicleKeys.list({ fleetId: fleetId ?? '' }),
    queryFn: () => vehicleService.listByFleet(fleetId as string),
    enabled: !!fleetId,
    staleTime: 60 * 1000,
  });
}

export function useCreateVehicle(fleetId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (attributes: CreateVehicleAttributes) =>
      vehicleService.createInFleet(fleetId, attributes),
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: vehicleKeys.lists() });
    },
  });
}
```

The `queryKey` is built even when `fleetId` is null, because `enabled` — not a conditional key — is what holds the fetch back. A key that changes shape between renders would fragment the cache.
