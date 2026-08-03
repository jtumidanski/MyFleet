# Styling & Theming Patterns

## Overview

MyFleet UI uses **Tailwind CSS** with **shadcn/ui** components and CSS variable-based theming for light/dark mode support.

## cn() Utility

All conditional classnames go through `cn()` — a combination of `clsx` and `tailwind-merge`:

```typescript
// lib/utils.ts
import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]): string {
  return twMerge(clsx(inputs));
}
```

**Usage:**
```tsx
<div className={cn(
  "flex items-center gap-2 p-4",           // Base classes
  variant === "destructive" && "bg-destructive text-destructive-foreground",
  disabled && "opacity-50 cursor-not-allowed",
  className,                                // Allow parent override
)} />
```

**Never** concatenate class strings manually — always use `cn()`.

## Tailwind Class Order

Follow this ordering convention for readability:

```
1. Layout       (flex, grid, inline-flex)
2. Positioning  (relative, absolute, sticky)
3. Box model    (p-4, m-2, w-full, h-10)
4. Typography   (text-sm, font-medium, text-muted-foreground)
5. Visual       (bg-background, border, rounded-lg, shadow-sm)
6. Effects      (transition-colors, animate-spin)
7. States       (hover:bg-accent, focus:ring-2, disabled:opacity-50)
```

## CSS Variable Theming

Theme tokens are defined as CSS variables in `apps/web/src/index.css`, under `@layer base`:

```css
@layer base {
  :root {
    --background: 0 0% 100%;
    --foreground: 222.2 84% 4.9%;
    --card: 0 0% 100%;
    --primary: 222.2 47.4% 11.2%;
    --destructive: 0 84.2% 60.2%;
    --muted: 210 40% 96.1%;
    --accent: 210 40% 96.1%;
    --border: 214.3 31.8% 91.4%;
    --ring: 222.2 84% 4.9%;

    /* Modal/dialog scrim. Deliberately the same dark value in both themes —
       a scrim that inverts with the theme stops being a scrim. */
    --overlay: 222.2 84% 4.9%;

    /* Status families, each a set of four: the solid colour, a subtle fill,
       a foreground for that fill, and a border. */
    --success: 142.4 71.8% 29.2%;
    --success-subtle: 140.6 84.2% 92.5%;
    --success-subtle-foreground: 142.8 64.2% 24.1%;
    --success-border: 141 78.9% 85.1%;
    /* ...and the same four for --warning, --danger and --info */

    /* The sidebar surface, mirroring the card family. */
    --sidebar: 0 0% 100%;             /* mirrors --card */
    --sidebar-foreground: 222.2 84% 4.9%;
    /* ... */
  }

  .dark {
    --background: 222.2 84% 4.9%;
    --foreground: 210 40% 98%;
    /* every token above is redeclared here */
  }
}
```

Each token is wired to a Tailwind colour name in `tailwind.config.ts` as `hsl(var(--token))` (`:17-23` and below), which is why the values are bare HSL components with no `hsl()` wrapper — the alpha modifier syntax (`bg-primary/90`) only works if the variable holds the components alone.

Dark mode is `darkMode: ['class']` (`tailwind.config.ts:4`) — the `.dark` class on `<html>` selects the second block. `content` also covers `packages/ui-components/src` (`:8`), so classes used only in that package are not purged.

**Use semantic color names**, not raw Tailwind colors:

```tsx
// ✅ Good — semantic, theme-aware
<div className="bg-background text-foreground border-border" />
<p className="text-muted-foreground" />
<Button variant="destructive" />

// ❌ Bad — hard-coded colors, ignores theme
<div className="bg-white text-gray-900 border-gray-200" />
<p className="text-gray-500" />
```

## shadcn/ui setup

**There is no `components.json`.** The shadcn CLI was not kept wired up; `components/ui/` is a set of ordinary source files that are edited in place like any other component. Do not run `npx shadcn add` expecting it to resolve — and do not add a `components.json` describing `@/` aliases that do not exist.

What the setup actually is:

- `tailwind.config.ts` — `darkMode: ['class']`, `content` covering `apps/web` plus `packages/ui-components/src`, and the semantic colour names bound to the CSS variables above
- `apps/web/src/index.css` — the tokens, in `:root` and `.dark`
- `apps/web/src/lib/utils.ts` — `cn()`
- Icons: `lucide-react`

## Component Variant Pattern (CVA)

`class-variance-authority` is a real dependency (`package.json:26`) and drives every variant system. From `components/ui/button.tsx:6-30`, lightly abridged:

```typescript
import { cva, type VariantProps } from 'class-variance-authority';

const buttonVariants = cva(
  'inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-md text-sm font-medium transition-colors focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50 cursor-pointer',
  {
    variants: {
      variant: {
        default: 'bg-primary text-primary-foreground hover:bg-primary/90',
        destructive: 'bg-destructive text-destructive-foreground hover:bg-destructive/90',
        outline: 'border border-input bg-background hover:bg-accent hover:text-accent-foreground',
        secondary: 'bg-secondary text-secondary-foreground hover:bg-secondary/80',
        ghost: 'hover:bg-accent hover:text-accent-foreground',
        link: 'text-primary underline-offset-4 hover:underline',
      },
      size: {
        default: 'h-10 px-4 py-2',
        sm: 'h-9 rounded-md px-3',
        lg: 'h-11 rounded-md px-8',
        icon: 'h-10 w-10',
      },
    },
    defaultVariants: {
      variant: 'default',
      size: 'default',
    },
  },
);
```

`cursor-pointer` is in the **base** string, not a variant — that is how `<Button>` satisfies `FE-15` for free and why only hand-rolled clickable surfaces need to add it. The component then composes the variants through `cn()` so a caller's `className` still wins (`button.tsx:41`).

## Common Layout Patterns

### Page container

```tsx
<div className="space-y-6">
  <PageHeader title="Vehicles" actions={canWrite && <Button>Add Vehicle</Button>} />
  …
</div>
```

`space-y-6` on the page container is the whole vertical rhythm — `PageHeader` renders only the header row and deliberately does not own the gap below it (`PageHeader.tsx:5-8`). Page padding comes from the shell, not the page: `AppLayout.tsx:57` renders `<main className="flex-1 p-6">`, so a page that adds its own `p-4` double-pads.

### Header with actions

Do **not** hand-write this row. `PageHeader` is the single owner of page-title markup, and `src/test/conventions.test.ts:184-202` fails the suite if any authenticated page under `src/pages/` contains an `<h1>` at all:

```tsx
// ❌ Bad — fails make fe-test
<div className="flex items-center justify-between">
  <h1 className="text-2xl font-bold">{title}</h1>
  <Button onClick={onCreate}>Create</Button>
</div>

// ✅ Good
<PageHeader title={title} actions={<Button onClick={onCreate}>Create</Button>} />
```

`PageHeader` derives `items-start` vs `items-center` from whether a `description` was passed, and offsets the actions by `-my-1` so a page with buttons is vertically identical to one without (`PageHeader.tsx:45-80`). Both are the kind of detail a hand-written copy gets wrong silently.

### Card-based detail layout

```tsx
<div className="space-y-6">
  <Card>
    <CardHeader><CardTitle>Section Title</CardTitle></CardHeader>
    <CardContent>
      <div className="grid grid-cols-2 gap-4">
        <div><Label>Field</Label><p>{value}</p></div>
      </div>
    </CardContent>
  </Card>
</div>
```

### Responsive grid

```tsx
// DashboardGrid.tsx:58
<div className="grid grid-cols-1 gap-4 md:grid-cols-2">
  {items.map((item) => <Card key={item.id}>…</Card>)}
</div>
```

One column on mobile, two from `md` up. Reach for a `lg:` tier only when a real layout needs it — nothing in the app currently goes past two.

## Icon Usage

Use **Lucide React** icons with consistent sizing:

```tsx
import { Plus, Trash2, ArrowLeft, Loader2, MoreHorizontal } from "lucide-react";

// In buttons
<Button><Plus className="mr-2 h-4 w-4" /> Create</Button>

// Standalone
<Trash2 className="h-4 w-4 text-destructive" />

// Loading
<Loader2 className="h-4 w-4 animate-spin" />
```

Standard sizes: `h-4 w-4` (default), `h-5 w-5` (medium), `h-6 w-6` (large).

## Dark Mode

Class-based, via `ThemeProvider` in `context/ThemeContext.tsx`. Components never branch on the theme — the CSS variables do it — so the only component that reads the context is the toggle itself:

```tsx
// components/ThemeToggle.tsx:20-36 (abridged)
import { useTheme } from '../context/ThemeContext';

export function ThemeToggle() {
  const { preference, setPreference } = useTheme();
  const updateTheme = useUpdateTheme();

  return (
    <ThemeToggleButton
      preference={preference}
      onSelect={(next) => {
        setPreference(next);
        updateTheme.mutate(next, { onError: () => toast.error("Couldn't save your theme preference.") });
      }}
    />
  );
}
```

Note the shape: the context exposes `preference` / `setPreference` (a three-way `'light' | 'dark' | 'system'`), not a two-way `theme` / `setTheme`. `useTheme()` throws outside a `ThemeProvider` (`ThemeContext.tsx:103-105`). The presentation lives in `ThemeToggleButton`, which the signed-out login page renders directly — there is no session there, so the mutation must not come along.

Two pieces around it are pinned by `src/test/conventions.test.ts`:

- **`ThemeSync`** (`components/ThemeSync.tsx`, mounted in `AppProviders.tsx:48`) bridges the server-stored preference to the theme context without either context importing the other.
- **The pre-paint script in `index.html`** (`:35-40`) reads `localStorage['myfleet.theme']` and adds the `dark` class before the module bundle loads. The test asserts it is present, synchronous, wrapped in try/catch, small, and uses the same media query as `ThemeContext` — because a theme applied after first paint is a visible white flash on every load.

## Cursor affordance for interactive elements

Every element that responds to a click must visually communicate clickability via `cursor-pointer`. Native `<button>` and `<a>` elements get a pointer from the browser; custom interactive elements — clickable `<div>`s, popover triggers wrapped in non-`<button>` `render`-prop targets, table rows that open detail views, etc. — must apply `cursor-pointer` explicitly via Tailwind.

Concrete example: a table row rendered as a `PopoverTrigger` but did not show a pointer cursor on hover, so users had no visual cue that it was clickable. Adding `cursor-pointer` to the trigger element fixed it. Apply the same affordance whenever you author a clickable surface, especially when the trigger is composed via a primitive's `render` prop (the rendered element may not naturally inherit a pointer cursor).

Acceptance test: hover over the element with a pointing device. The cursor must change to a pointer. If it does not, add `cursor-pointer` to its `className`.
