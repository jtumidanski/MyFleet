# Anti-Patterns

Every row below has a matching `FE-*` check in `.claude/agents/frontend-guidelines-reviewer.md`. If you add a rule here, add the check there; a rule with no check is a suggestion, and a check with no rule is unfalsifiable.

## Quick Reference

| Anti-Pattern | Correct Pattern | Check |
|-------------|-----------------|-------|
| `any` type | A named type, or `unknown` + a type predicate | `FE-01` |
| Manual class concatenation | `cn()` from `lib/utils.ts` | `FE-02` |
| Calling `apiClient` (or a service) from a component | Component → hook → service → client | `FE-03` |
| Inline Zod schema in a component | Define it in `lib/schemas/<resource>.ts` | `FE-04` |
| Spinner for content loading | `Skeleton` in the shape of the content | `FE-05` |
| Hardcoded color values | Semantic tokens (`bg-background`) | `FE-06` |
| Mutating state | Spread; build a new value | `FE-07` |
| Default export for a component | Named export | `FE-08` |
| Swallowed async failure | `createErrorFromUnknown()` + `toast.error()` | `FE-09` |

---

## Detailed Anti-Patterns

### 1. Calling the API client — or a service — from a component

The layering is **component → hook → service → `apiClient`**, and each arrow is one step. A component that reaches two layers down skips the query cache, the invalidation rules and the shared error envelope handling.

```tsx
// ❌ Bad — bypasses the service layer AND the query cache
import { apiClient } from '../../lib/api/client';
const doc = await apiClient.request<JsonApiDocument<Vehicle[]>>(
  `/api/fleet/fleets/${fleetId}/vehicles`,
);

// ❌ Also bad — uses the service, but still no cache, no invalidation,
//    no shared loading flag. This is the shape FE-03 exists to catch.
const [vehicles, setVehicles] = useState<Vehicle[]>([]);
useEffect(() => {
  vehicleService.listByFleet(fleetId).then((r) => setVehicles(r.data));
}, [fleetId]);

// ✅ Good — the hook owns the fetch, the cache and the loading flag
import { useVehicles } from '../../lib/hooks/api/vehicles';
const { data, isLoading } = useVehicles(activeFleetId);
```

The client is exported as `apiClient` (`lib/api/client.ts:17`), not `api`, and there is **no `@/` path alias** — imports are relative. Both matter: a grep for the wrong symbol or the wrong specifier finds nothing and reports a clean bill of health.

### 2. Manual class string concatenation

```tsx
// ❌ Bad — no merge, conflicting Tailwind classes both survive
<div className={'flex items-center ' + (active ? 'bg-primary' : '')} />

// ✅ Good — cn() joins conditionally and resolves conflicts
<div className={cn('flex items-center', active && 'bg-primary')} />
```

`cn(...inputs: ClassValue[]): string` is `clsx` + `tailwind-merge` (`lib/utils.ts:9`). The `tailwind-merge` half is the point: without it `cn('p-2', 'p-4')` and `'p-2 p-4'` differ only by which class the CSS cascade happens to prefer.

### 3. Using `any`

```typescript
// ❌ Bad — defeats TypeScript
const handle = (data: any) => { … };

// ✅ Good — the resource type already exists
import type { Vehicle } from '../../types/models/vehicle';
const handle = (vehicle: Vehicle) => { … };

// ✅ Good — unknown + a type predicate for genuinely dynamic input
export function isThemePreference(value: unknown): value is ThemePreference {
  return typeof value === 'string' && (VALID as readonly string[]).includes(value);
}
```

The predicate above is real (`lib/theme.ts:22`) and shows the shape: `unknown` in, `value is T` out, so the narrowing is checked rather than asserted. For API payloads you rarely need one — `types/models/` and `@myfleet/shared-ts` already name the shapes.

### 4. Inline schema definition

```tsx
// ❌ Bad — schema buried in the component, not reusable, not testable
export function AddVehicleDialog() {
  const schema = z.object({ make: z.string().min(1) });
  …
}

// ✅ Good — schema in a dedicated file
// lib/schemas/vehicle.ts
export const vehicleSchema = z.object({
  make: z.string().trim().min(1, 'Make is required'),
  …
});
export type VehicleFormInput = z.infer<typeof vehicleSchema>;

// The component imports both
import { vehicleSchema, type VehicleFormInput } from '../../lib/schemas/vehicle';
```

The file is `lib/schemas/<resource>.ts` — **no `.schema.` infix** and no `@/` alias. Several schemas carry a sibling `<resource>.test.ts`, which is the other reason they live in their own file.

**Exception:** cross-field `.refine()` validation that only one form needs can stay in that form's file.

### 5. Spinner for content loading

```tsx
// ❌ Bad — jarring, and the layout jumps when data lands
if (isLoading) return <div className="animate-spin">Loading…</div>;

// ✅ Good — a skeleton in the shape of the content that replaces it
if (isLoading) {
  return (
    <div className={PAGE_CONTAINER}>
      <PageHeader title="Vehicle" />
      <Skeleton className="h-40 w-full" />
    </div>
  );
}

// ✅ Good — a spinner IS right for an in-flight submit
<Button disabled={isPending}>
  {isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
  Upload
</Button>
```

The loaded and loading branches must share the container (`VehicleDetailPage.tsx:45,103-110`) — a skeleton at a different width is the same layout jump the skeleton was supposed to prevent. The submit-button spinner is real too (`MediaUploadButton.tsx:82`): the distinction is *content arriving* (skeleton) versus *an action in flight* (spinner).

### 6. Hardcoded colors

```tsx
// ❌ Bad — ignores the theme, breaks dark mode
<div className="bg-white text-gray-900 border-gray-200" />

// ✅ Good — semantic tokens
<div className="bg-background text-foreground border-border" />
<p className="text-muted-foreground" />
```

**This one is executable.** `src/test/conventions.test.ts:113-115` greps every `.tsx` under `apps/web/src` and `packages/ui-components/src` for `(bg|text|border|ring|divide)-(gray|slate|zinc|neutral|white|black|red|…)` and fails the suite on a hit. The allowlist is exactly two lines — the dialog and sheet overlay scrims (`:130-133`) — exempted by file *and* exact text, so a new palette class in those same two files still fails. Do not add to it; add a token to `index.css` instead.

### 7. Mutating state

```tsx
// ❌ Bad — same array, so React sees no change and skips the re-render
const updated = vehicles;
updated.push(newVehicle);
setVehicles(updated);

// ✅ Good — a new array
setVehicles([...vehicles, newVehicle]);

// ✅ Good — a new object, nested spread for nested attributes
setVehicle({ ...vehicle, attributes: { ...vehicle.attributes, ...updates } });

// ✅ Good — the real one: derive, don't splice into the source
const nav = platformAdmin ? [...NAV, ADMIN_ENTRY] : NAV;
```

The last example is `AppLayout.tsx:42`. Note it is not `useState` at all — with React Query owning server state, most "state" you would have been tempted to mutate is a query result you must treat as read-only, because the cache hands the same object to every other subscriber.

### 8. Swallowing an async failure

```tsx
// ❌ Bad — a rejected mutation disappears; the user sees nothing happen
const handleCreate = async (values: VehicleFormInput) => {
  await createVehicle.mutateAsync(toCreateAttributes(values));
  setOpen(false);
};

// ✅ Good — normalize the error, tell the user, keep their input
const handleCreate = async (values: VehicleFormInput) => {
  try {
    await createVehicle.mutateAsync(toCreateAttributes(values));
    toast.success('Vehicle added');
    setOpen(false);
  } catch (err) {
    // Leave the dialog open so the typed values survive for a retry.
    const apiError = createErrorFromUnknown(err);
    toast.error(apiError.message || 'Could not add vehicle');
  }
};
```

Copied from `VehiclesPage.tsx:55-66`. Three details:

- `createErrorFromUnknown` is imported from **`@myfleet/shared-ts`**, not from a local util (`packages/shared-ts/src/errors.ts:23`). Every real call site imports it from the package.
- It takes **one argument**. It returns an `ApiError` carrying `status`, `code`, `message`, `detail` and `pointer` unpacked from the JSON:API error envelope; supply your own fallback copy with `||`, not a second parameter.
- The failure path must not close the dialog. Discarding the form on error makes the user retype everything to retry.

### 9. Default exports for components

```tsx
// ❌ Bad — the import site picks its own name; rename-symbol can't follow it
export default function VehicleList() { … }

// ✅ Good — one name, everywhere
export function VehicleList() { … }
```

No component in `apps/web/src` uses a default export. Keep it that way.
