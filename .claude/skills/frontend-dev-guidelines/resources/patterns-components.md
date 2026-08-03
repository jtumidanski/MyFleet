# Component Patterns

## Component Organization

```
apps/web/src/components/
├── ui/                     # shadcn/ui primitives — button, card, dialog, form,
│                           #   input, select, table, skeleton, sidebar, …
├── frame/                  # the authenticated app shell: FrameHeader, FrameNav,
│                           #   AppBreadcrumb, BrandLink, ProfileMenu, crumbs/
├── features/               # domain UI, one directory per area
│   ├── activity/
│   ├── dashboard/
│   ├── notifications/
│   ├── settings/
│   └── vehicles/           # CategoryCombobox, VehicleCard, VehicleForm,
│       ├── detail/         #   VehicleList, VehiclePhotoThumbnail, plus
│       ├── dialogs/        #   subdirectories per sub-area
│       ├── fuel/
│       ├── maintenance/
│       ├── media/
│       └── mileage/
├── admin/                  # platform-admin console shell and its guards
├── providers/              # AppProviders — the root provider stack
└── *.tsx                   # app-level furniture: AppLayout, PageHeader,
                            #   RequireAuth, BrandMark, ThemeToggle, ThemeSync
```

Those six entries are the whole taxonomy — there is no seventh directory for shared non-primitive components. That tier is the flat set of `.tsx` files directly under `components/`: `PageHeader.tsx`, `AppLayout.tsx`, `BrandMark.tsx`, `ThemeToggle.tsx`, `RequireAuth.tsx`.

Genuinely design-system-level pieces live outside the app entirely, in `packages/ui-components` (e.g. `StatusBadge`). The boundary is written down at `PageHeader.tsx:10-12`: `PageHeader` stays in the app because it encodes *this app's* h1 level, type scale and 24px rhythm; a component that would carry those assumptions into another consumer does not belong in the package.

**Named exports only.** `export default` has 0 occurrences in `apps/web/src` (`FE-08`).

## Presentational Components

`ui/` holds the shadcn primitives. They take `className` and merge it with `cn()` rather than replacing their base classes (`FE-02`):

```tsx
// components/ui/skeleton.tsx
import { cn } from '../../lib/utils';

function Skeleton({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div className={cn('animate-pulse rounded-md bg-muted', className)} aria-hidden {...props} />
  );
}

export { Skeleton };
```

App-level presentational components follow the same contract — props interface first, then the component, `className` merged last so a caller can override:

```tsx
// components/PageHeader.tsx:38-54
export function PageHeader({ title, titleAdornment, description, actions, className }: PageHeaderProps) {
  return (
    <div
      className={cn(
        'flex justify-between gap-2',
        description ? 'items-start' : 'items-center',
        className,
      )}
    >
```

Note `PageHeader`'s two structural choices, both explained in comments and both pinned by `PageHeader.test.tsx`: `titleAdornment` is a separate slot rather than widening `title` to `ReactNode`, because a badge inside the `<h1>` would join the heading's accessible name (`PageHeader.tsx:20-25`); and `actions` renders only when truthy, so no empty flex child changes how a long title wraps (`:66`, tested at `PageHeader.test.tsx:31-43`).

## Feature Components

`features/<area>/` holds components that know about a domain but still take data as props. `VehicleList` is the model:

```tsx
// components/features/vehicles/VehicleList.tsx:5-17
interface VehicleListProps {
  vehicles: Vehicle[];
  isLoading: boolean;
  /**
   * Call to action for the empty state. The list stays presentational — the
   * caller has already decided whether this viewer may act, so the copy keys
   * off the node's presence rather than off a role this component would have
   * to read for itself.
   */
  emptyAction?: ReactNode;
}

export function VehicleList({ vehicles, isLoading, emptyAction }: VehicleListProps) {
```

The list takes `vehicles` and `isLoading` as props rather than calling `useVehicles` itself. The page owns the hook; the component stays renderable in a test with no query client and no network (`VehicleList.test.tsx`). Passing a *node* rather than a *role* is the same discipline one level down — the component never has to know who is allowed to act.

## Component Structure Convention

Order within a file, as in every component cited here:

1. Imports — relative, deepest-shared first (`../../ui/button`, then `../../../lib/…`, then `type` imports of models).
2. The props interface, named `<Component>Props`. Exported only if a caller needs it (`PageHeaderProps` is; `VehicleListProps` is not).
3. The component, as a named `function` export.
4. Any subcomponent or skeleton that belongs to it, in the same file.

Doc comments go on the props, not in a wall above the component, and they carry the *reason* — `VehicleListProps.emptyAction` above, or `PageHeaderProps.actions` at `PageHeader.tsx:28-32`.

## List and Table Patterns

The only table code in the tree is `components/ui/table.tsx`, a set of thin styled elements. There is no generic table component that takes column definitions, and no headless-table library in `apps/web/package.json` — check the dependency list before reaching for one. Two real patterns:

**Card grid**, for the primary browse surface — `VehicleList.tsx:39-45` maps to `VehicleCard` inside a responsive grid, and returns early for the loading and empty branches rather than threading conditionals through one JSX tree.

**`ui/table` primitives**, used directly, for dense admin tables. `pages/admin/AdminFleetsPage.tsx:232-250` composes `Table` / `TableHeader` / `TableRow` / `TableHead` / `TableBody` / `TableCell` by hand. There is no column-definition abstraction; a new column is a `<TableHead>` and a `<TableCell>`.

## Loading State Pattern

Skeletons, never spinners, for content that is loading (`FE-05`). `animate-spin` is allowed only on a submit button's in-flight indicator (`VehicleForm.tsx:186`).

The skeleton for a component lives beside it and is exported from the same file:

```tsx
// components/features/vehicles/VehicleCard.tsx:203
export function VehicleCardSkeleton() { … }

// consumed at VehicleList.tsx:18-26
if (isLoading) {
  return (
    <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
      {Array.from({ length: 6 }).map((_, i) => <VehicleCardSkeleton key={i} />)}
    </div>
  );
}
```

The skeleton must occupy the same box as the real thing. `VehicleCard.tsx:210-213` records the arithmetic: the bars are inset inside the real title's 24px and subtitle's 20px line boxes, so the skeleton reads as text without being shorter than the text it stands in for. A skeleton of the wrong height produces a layout jump on every load.

`Skeleton` is `aria-hidden` (`ui/skeleton.tsx:5`) — the loading state is conveyed by the absence of content, not announced as decorative boxes.

## Dialog Pattern

shadcn `Dialog`, always controlled — `open` plus `onOpenChange` come from the caller:

```tsx
// components/features/vehicles/dialogs/EditVehicleDialog.tsx:43-48
<Dialog open={open} onOpenChange={onOpenChange}>
  <DialogContent>
    <DialogHeader>
      <DialogTitle>Edit Vehicle</DialogTitle>
    </DialogHeader>
```

A dialog wrapping a form owns the mutation, the toast and the close decision, and passes only `onSubmit` / `onCancel` / `submitting` down to the form. See `patterns-forms-validation.md` § Dialog close behaviour for the in-flight-submit rules — they are load-bearing and easy to omit.

## Badge Pattern

Two distinct things, do not confuse them:

- **`StatusBadge`** from `@myfleet/ui-components` — the vehicle-status badge, with one semantic token pair per status (`packages/ui-components/src/StatusBadge.tsx:7-12`). Each badge shows its status **as text**, so colour is never the only signal.
- **`ui/badge`'s `<Badge variant=…>`** — the general-purpose badge, used across the admin pages (`AdminFleetsPage.tsx:75,245,299`).

When a variant is derived from a value, put the mapping in a named function beside the value's type rather than inline in JSX — `purgeStatusVariant` (`lib/admin/purgeStatus.ts:49`) returns `NonNullable<BadgeProps['variant']>`, so an unhandled status is a type error rather than an undefined variant.

## Empty State Pattern

Say what is missing, and offer the action **only if the caller supplied one**:

```tsx
// VehicleList.tsx:28-37
if (vehicles.length === 0) {
  return (
    <div className="rounded-lg border border-dashed border-border p-8 text-center text-muted-foreground">
      <p>
        {emptyAction ? 'No vehicles yet. Add your first one to get started.' : 'No vehicles yet.'}
      </p>
      {emptyAction && <div className="mt-4">{emptyAction}</div>}
    </div>
  );
}
```

The copy changes with the action, not just the button: a viewer who cannot add a vehicle should not be told to "add your first one" (`VehicleList.tsx:8-13`). This is asserted at `VehicleList.test.tsx:30-40`.

## Text Casing Rules

All user-facing interactive text uses **title case** — capitalize each significant word.

- **Buttons**: "Add Vehicle", "Save Changes", "Log Fuel"
- **Dialog titles**: "Edit Vehicle", "Add Vehicle"
- **Badge text**: "Pending Purge", "Read Only"
- **Nav and tab labels**: "Dashboard", "Vehicles", "Activity"

```tsx
// ✅ Good — title case
<Button>Add Vehicle</Button>
<DialogTitle>Edit Vehicle</DialogTitle>

// ❌ Bad — sentence case
<Button>Add vehicle</Button>
<DialogTitle>Edit vehicle</DialogTitle>
```

Prepositions and articles inside a label stay lowercase ("Move to Trash", "Import from File"); the first word is always capitalized.

**Enum display values** are capitalized for display while the underlying value is untouched:

```tsx
// ✅ Good — display string differs from the value
<SelectItem value="time">Time-based</SelectItem>

// ❌ Bad — raw enum value shown to the user
<SelectItem value="time">time</SelectItem>
```

`MaintenanceScheduleForm.tsx:85-87` is the live example. Where the display string is more than a capitalization — an audit action, a purge status — use an explicit lookup map (`AdminAuditPage.tsx:115`, `ACTION_LABELS`), not string manipulation.

## Cursor Behavior

Every clickable element shows `cursor-pointer` on hover. `<Button>` already carries it in its CVA definition; custom clickable elements must add it explicitly (`FE-15`, and `patterns-styling.md` § Cursor affordance for interactive elements).

```tsx
// ✅ Good — pointer cursor on clickable elements
<div className="cursor-pointer" onClick={handleClick}>…</div>

// ❌ Bad — default cursor on clickable element
<div onClick={handleClick}>…</div>
```
