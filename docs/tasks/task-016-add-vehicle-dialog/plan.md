# Add Vehicle Dialog Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the inline create-vehicle form on the Vehicles page with a focused, accessible modal dialog, introducing the shared `Dialog` primitive the application currently lacks.

**Architecture:** A new shadcn-style `Dialog` primitive wraps `@radix-ui/react-dialog` in `apps/web/src/components/ui/dialog.tsx`, adding one generic affordance Radix does not provide — a `dismissible` prop that refuses Escape, outside-click, and close-button dismissal as a single invariant — plus its own focus restoration, because Radix's built-in restoration only works for dialogs opened from a registered `DialogTrigger`. `VehiclesPage` drives the dialog in controlled mode from two plain buttons (header and empty state), owns the mutation and toasts exactly as it does today, and redirects focus to the header trigger in the one case where the opener unmounts. `VehicleList` gains an optional `emptyAction` node and stays presentational.

**Tech Stack:** React 18.3, TypeScript, Radix UI (`@radix-ui/react-dialog` 1.1.x), Tailwind CSS 3.4, react-hook-form + Zod, TanStack React Query 5, Vitest 2.1 + Testing Library + jsdom.

## Global Constraints

- **Scope is `apps/web` only.** No Go service, contract, deployment manifest, or data model is touched.
- **`apps/web/src/pages/VehicleDetailPage.tsx` must remain unmodified.** It is owned by `task-012-vehicle-detail-redesign`. `git status` must never show it.
- **Node comes from WSL, not Windows.** Every `npm`/`npx` command in this plan must be preceded by, or run in a shell that has already run:
  ```sh
  export PATH="$HOME/.nvm/versions/node/v24.12.0/bin:$PATH"
  ```
  The `npm` on the default `PATH` is the Windows binary and fails on workspace symlinks with `EISDIR`.
- **All work happens in the worktree** `/home/tumidanski/source/MyFleet/.worktrees/task-016-add-vehicle-dialog` on branch `task-016-add-vehicle-dialog`. Paths below are relative to that root.
- **Dependency version:** `@radix-ui/react-dialog` at `^1.1.0`, placed in `apps/web/package.json` `dependencies` alphabetically between `@radix-ui/react-label` and `@radix-ui/react-select`.
- **Styling uses existing Tailwind tokens only** (`bg-background`, `border-border`, `text-muted-foreground`, `ring-ring`, `ring-offset-background`, …). No new CSS variables, no new Tailwind plugins. In particular **do not add `tailwindcss-animate`** — see "Verified environment facts" below.
- **`dialog.tsx` must contain no vehicle-specific strings or logic.** It is a shared primitive that later modals (edit, delete confirmation) will reuse unchanged.
- **`VehicleList` must not import or read auth context**, or take a `role`/`canWrite` prop.
- **Exact user-visible copy** (do not paraphrase):
  - Dialog title: `Add Vehicle`
  - Dialog description: `Make, model, and year are required.`
  - Success toast: `Vehicle added`
  - Error toast: `apiError.message || 'Could not add vehicle'`
  - Empty state, writers: `No vehicles yet. Add your first one to get started.`
  - Empty state, viewers: `No vehicles yet.`
  - Close button accessible name: `Close`
- **Test baseline to hold** (measured in this worktree before any change): **293 passing in `apps/web`** across 39 files, **7 in `shared-ts`**, **10 in `ui-components`**. New tests add to these; nothing pre-existing may regress.

## Verified environment facts

These were established empirically in this worktree before the plan was written. They override anything in `design.md` that contradicts them.

1. **`^1.1.0` resolves to `@radix-ui/react-dialog` 1.1.23.**

2. **Radix's modal `DialogContent` restores focus only to a registered `DialogTrigger`.** Its internal handler is literally:
   ```js
   onCloseAutoFocus: composeEventHandlers(props.onCloseAutoFocus, (event) => {
     event.preventDefault();
     context.triggerRef.current?.focus();
   })
   ```
   and `triggerRef` is populated *only* by `DialogTrigger`. This plan drives the dialog in controlled mode from plain buttons (design §4.4, and correctly so — two buttons open one dialog, and `DialogTrigger` binds one element). A probe confirmed that in that configuration **focus lands on `document.body` on close**, which fails FR-4.4.

   > **This is a correction to design §4.4**, which asserts "Radix restores focus to `document.activeElement` as captured at open time, not to a registered trigger node." That is false for the modal content path. The design's *decision* (controlled mode, plain buttons) is kept; only its stated justification was wrong, and Task 1 supplies the missing mechanism inside the primitive so every future modal inherits the fix.

   The working mechanism, verified by probe: capture `document.activeElement` in `onOpenAutoFocus` (which fires *before* focus moves into the dialog), and restore it in `onCloseAutoFocus` *after* giving the consumer's handler a chance to `preventDefault()`. Capturing at render time or in a mount effect does **not** work — the wrapper component renders before the dialog opens, so it captures `document.body`.

3. **Rendering the close button after `children` puts initial focus on the first form control.** Verified by probe. Do not reorder.

4. **jsdom pointer-events are not a problem for clicks inside the dialog.** While open, `document.body.style.pointerEvents === 'none'`, yet `userEvent.click` and `userEvent.type` on elements *inside* `DialogContent` work correctly. No `pointerEventsCheck` escape hatch is needed.

5. **Overlay dismissal must be driven with `fireEvent.pointerDown(document.body)`**, not a `userEvent` click on the overlay. Verified to close the dialog through the same `DismissableLayer` path a real overlay click takes.

6. **`dismissible={false}` implemented as `preventDefault()` in `onEscapeKeyDown` + `onInteractOutside` plus `disabled` on the close button blocks all three routes.** Verified by probe.

7. **`tailwindcss-animate` is not installed** (`tailwind.config.ts` has `plugins: []`). The `data-[state=*]:animate-in` classes copied from `select.tsx` generate no CSS today. They are included anyway for literal consistency with the stated reference primitive (design §4.5). Do not "fix" this by installing the plugin — that would retroactively animate the existing Select.

8. **Test conventions:** mock `../context/AuthContext` with a `vi.fn<() => AuthContextValue>()` (pattern: `apps/web/src/components/AppLayout.test.tsx:10-13`); mock services at the module boundary (pattern: `apps/web/src/components/features/vehicles/VehicleCard.test.tsx:12-14`); render through `renderWithProviders` (`apps/web/src/test/renderWithProviders.tsx`), which supplies a retry-free QueryClient and a `MemoryRouter`.

## File Structure

| File | Responsibility |
| --- | --- |
| `apps/web/package.json` | **Modify** — add the `@radix-ui/react-dialog` dependency |
| `package-lock.json` | **Regenerated** by the install; expect only `react-dialog` and its transitive additions |
| `apps/web/src/components/ui/dialog.tsx` | **Create** — the shared, vehicle-agnostic Dialog primitive |
| `apps/web/src/components/ui/dialog.test.tsx` | **Create** — behavioural tests for the primitive's own contract |
| `apps/web/src/components/features/vehicles/VehicleList.tsx` | **Modify** — optional `emptyAction`, two empty-state copy variants |
| `apps/web/src/components/features/vehicles/VehicleList.test.tsx` | **Create** — empty-state variants and the populated branch |
| `apps/web/src/components/features/vehicles/VehicleForm.tsx` | **Modify** — one attribute: Cancel disabled while submitting |
| `apps/web/src/pages/VehiclesPage.tsx` | **Modify** — dialog replaces the inline card; two triggers; pending guard; focus redirect |
| `apps/web/src/pages/VehiclesPage.test.tsx` | **Create** — the full dialog flow |

### Deviation from design §9.3, stated up front

The design says `dialog.tsx` gets no dedicated test file, on the grounds that "a unit test of a styling wrapper would assert class strings, which is churn." That reasoning is sound for *styling* but does not cover the two pieces of real **behaviour** this primitive owns and Radix does not: the `dismissible` three-part invariant and the focus restoration from fact 2 above. The design's own risk table flags "`dismissible` wired to only some of its three effects" as a Medium risk. This plan therefore creates `dialog.test.tsx` with **behavioural assertions only — no class-string assertions**, which honours the design's stated rationale while testing the primitive's contract at the level that owns it. The page-level coverage in design §9.2 is kept in full as integration proof.

---

## Task 1: The `Dialog` primitive

**Files:**
- Modify: `apps/web/package.json` (dependencies block, lines 11-30)
- Create: `apps/web/src/components/ui/dialog.tsx`
- Test: `apps/web/src/components/ui/dialog.test.tsx`

**Interfaces:**
- Consumes: `cn` from `apps/web/src/lib/utils`; `X` from `lucide-react`.
- Produces, all named exports from `apps/web/src/components/ui/dialog.tsx`:
  - `Dialog`, `DialogTrigger`, `DialogPortal`, `DialogClose` — direct re-exports of `DialogPrimitive.Root` / `.Trigger` / `.Portal` / `.Close`
  - `DialogOverlay`, `DialogTitle`, `DialogDescription` — `forwardRef` styling wrappers
  - `DialogHeader`, `DialogFooter` — plain layout `div`s
  - `DialogContent` — `forwardRef` wrapper accepting
    `DialogContentProps = React.ComponentPropsWithoutRef<typeof DialogPrimitive.Content> & { dismissible?: boolean }`,
    exported as `DialogContentProps`. `dismissible` defaults to `true`.

- [ ] **Step 1: Install the dependency**

```sh
export PATH="$HOME/.nvm/versions/node/v24.12.0/bin:$PATH"
cd /home/tumidanski/source/MyFleet/.worktrees/task-016-add-vehicle-dialog
npm install --workspace apps/web @radix-ui/react-dialog@^1.1.0
```

Then open `apps/web/package.json` and confirm the `@radix-ui/*` run reads exactly:

```json
    "@radix-ui/react-dialog": "^1.1.0",
    "@radix-ui/react-label": "^2.1.0",
    "@radix-ui/react-select": "^2.2.6",
    "@radix-ui/react-slot": "^1.1.0",
    "@radix-ui/react-switch": "^1.2.6",
```

Move the new entry if npm placed it elsewhere. `dialog` sorts *before* `label`, so it heads the run — design §4.6's "alphabetically between `@radix-ui/react-label` and `@radix-ui/react-select`" is a slip. FR-1.1 asks for alphabetical ordering and the existing block is alphabetical, so alphabetical governs.

Review the `package-lock.json` diff: it should add `@radix-ui/react-dialog` plus transitive packages (`react-remove-scroll`, `react-remove-scroll-bar`, `aria-hidden`, `@radix-ui/react-focus-scope`, `@radix-ui/react-focus-guards`, `@radix-ui/react-dismissable-layer`, `@radix-ui/react-portal`, `get-nonce`, `use-sidecar`, `tslib`, …) and change nothing else. If it churns unrelated versions, revert and re-run with `npm install --workspace apps/web @radix-ui/react-dialog@^1.1.0 --no-save` diagnostics before proceeding.

- [ ] **Step 2: Write the failing test**

Create `apps/web/src/components/ui/dialog.test.tsx`:

```tsx
import * as React from 'react';
import { describe, it, expect } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from './dialog';

/**
 * Drives the dialog the way the app does: controlled `open`, opened from an
 * ordinary button rather than a DialogTrigger. That combination is precisely
 * what defeats Radix's built-in focus restoration (it only ever restores to a
 * registered trigger), so it is the configuration the tests must exercise.
 */
function Harness({ dismissible = true }: { dismissible?: boolean }) {
  const [open, setOpen] = React.useState(false);
  return (
    <div>
      <button type="button" onClick={() => setOpen(true)}>
        Open
      </button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent dismissible={dismissible}>
          <DialogHeader>
            <DialogTitle>Panel</DialogTitle>
            <DialogDescription>What this panel is for.</DialogDescription>
          </DialogHeader>
          <input aria-label="First field" />
          <button type="button" onClick={() => setOpen(false)}>
            Done
          </button>
        </DialogContent>
      </Dialog>
    </div>
  );
}

/** Clicks the harness trigger and returns the opened dialog element. */
async function open(): Promise<HTMLElement> {
  await userEvent.click(screen.getByRole('button', { name: 'Open' }));
  return screen.getByRole('dialog');
}

describe('Dialog — announcement', () => {
  it('is a modal dialog labelled by its title and described by its description', async () => {
    render(<Harness />);
    const dialog = await open();
    expect(dialog).toHaveAttribute('aria-modal', 'true');
    expect(dialog).toHaveAccessibleName('Panel');
    expect(dialog).toHaveAccessibleDescription('What this panel is for.');
  });

  it('names the close button "Close"', async () => {
    render(<Harness />);
    await open();
    expect(screen.getByRole('button', { name: 'Close' })).toBeInTheDocument();
  });
});

describe('Dialog — focus', () => {
  it('moves focus to the first control in the body, not to the close button', async () => {
    // Guaranteed by DOM order: the close button is rendered after `children`.
    // Reorder them and this is the test that fails.
    render(<Harness />);
    await open();
    expect(document.activeElement).toBe(screen.getByLabelText('First field'));
  });

  it('returns focus to the plain button that opened it', async () => {
    // Radix restores focus only to a registered DialogTrigger. This dialog is
    // opened from an ordinary button in controlled mode, so without the
    // primitive's own capture-and-restore, focus would land on <body>.
    render(<Harness />);
    const trigger = screen.getByRole('button', { name: 'Open' });
    await userEvent.click(trigger);
    await userEvent.keyboard('{Escape}');
    expect(document.activeElement).toBe(trigger);
  });

  it('lets a consumer redirect focus by preventing the close-autofocus default', async () => {
    function Redirecting() {
      const [isOpen, setIsOpen] = React.useState(false);
      const elsewhereRef = React.useRef<HTMLButtonElement>(null);
      return (
        <div>
          <button type="button" onClick={() => setIsOpen(true)}>
            Open
          </button>
          <button type="button" ref={elsewhereRef}>
            Elsewhere
          </button>
          <Dialog open={isOpen} onOpenChange={setIsOpen}>
            <DialogContent
              onCloseAutoFocus={(event) => {
                event.preventDefault();
                elsewhereRef.current?.focus();
              }}
            >
              <DialogTitle>Panel</DialogTitle>
              <DialogDescription>Body.</DialogDescription>
            </DialogContent>
          </Dialog>
        </div>
      );
    }

    render(<Redirecting />);
    await userEvent.click(screen.getByRole('button', { name: 'Open' }));
    await userEvent.keyboard('{Escape}');
    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Elsewhere' }));
  });
});

describe('Dialog — dismissal', () => {
  it('closes on Escape', async () => {
    render(<Harness />);
    await open();
    await userEvent.keyboard('{Escape}');
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('closes on a pointer-down outside the content', async () => {
    // This is the path a real overlay click takes through DismissableLayer.
    // userEvent cannot drive the overlay itself under jsdom.
    render(<Harness />);
    await open();
    fireEvent.pointerDown(document.body);
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('closes on the close button', async () => {
    render(<Harness />);
    await open();
    await userEvent.click(screen.getByRole('button', { name: 'Close' }));
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });
});

describe('Dialog — dismissible={false}', () => {
  // All three routes are asserted independently. Wiring only two of them is the
  // exact regression this block exists to catch.
  it('ignores Escape', async () => {
    render(<Harness dismissible={false} />);
    await open();
    await userEvent.keyboard('{Escape}');
    expect(screen.getByRole('dialog')).toBeInTheDocument();
  });

  it('ignores a pointer-down outside the content', async () => {
    render(<Harness dismissible={false} />);
    await open();
    fireEvent.pointerDown(document.body);
    expect(screen.getByRole('dialog')).toBeInTheDocument();
  });

  it('disables the close button rather than hiding it', async () => {
    // Hiding it would reflow the header for the duration of the request.
    render(<Harness dismissible={false} />);
    await open();
    expect(screen.getByRole('button', { name: 'Close' })).toBeDisabled();
  });
});

describe('Dialog — content lifecycle', () => {
  it('does not mount its children before the first open', () => {
    render(<Harness />);
    expect(screen.queryByLabelText('First field')).not.toBeInTheDocument();
  });

  it('unmounts its children on close so nothing survives to the next open', async () => {
    render(<Harness />);
    await userEvent.click(screen.getByRole('button', { name: 'Open' }));
    await userEvent.type(screen.getByLabelText('First field'), 'typed');
    await userEvent.keyboard('{Escape}');
    await userEvent.click(screen.getByRole('button', { name: 'Open' }));
    expect(screen.getByLabelText('First field')).toHaveValue('');
  });
});
```

- [ ] **Step 3: Run the test to verify it fails**

```sh
export PATH="$HOME/.nvm/versions/node/v24.12.0/bin:$PATH"
cd apps/web && npx vitest run src/components/ui/dialog.test.tsx
```

Expected: FAIL — `Failed to resolve import "./dialog"`.

- [ ] **Step 4: Write the implementation**

Create `apps/web/src/components/ui/dialog.tsx`:

```tsx
import * as React from 'react';
import * as DialogPrimitive from '@radix-ui/react-dialog';
import { X } from 'lucide-react';
import { cn } from '../../lib/utils';

const Dialog = DialogPrimitive.Root;
const DialogTrigger = DialogPrimitive.Trigger;
const DialogPortal = DialogPrimitive.Portal;
const DialogClose = DialogPrimitive.Close;

const DialogOverlay = React.forwardRef<
  React.ElementRef<typeof DialogPrimitive.Overlay>,
  React.ComponentPropsWithoutRef<typeof DialogPrimitive.Overlay>
>(({ className, ...props }, ref) => (
  <DialogPrimitive.Overlay
    ref={ref}
    className={cn(
      'fixed inset-0 z-50 bg-black/80 data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0',
      className,
    )}
    {...props}
  />
));
DialogOverlay.displayName = DialogPrimitive.Overlay.displayName;

export interface DialogContentProps
  extends React.ComponentPropsWithoutRef<typeof DialogPrimitive.Content> {
  /**
   * When false the dialog refuses every user-driven dismissal at once: Escape
   * and outside pointer-down are cancelled, and the close button renders
   * disabled. Use it while a request the user cannot undo is in flight, so the
   * three routes can never drift apart. Defaults to true.
   */
  dismissible?: boolean;
}

const DialogContent = React.forwardRef<
  React.ElementRef<typeof DialogPrimitive.Content>,
  DialogContentProps
>(
  (
    {
      className,
      children,
      dismissible = true,
      onOpenAutoFocus,
      onCloseAutoFocus,
      onEscapeKeyDown,
      onInteractOutside,
      ...props
    },
    ref,
  ) => {
    // Radix's modal Content restores focus only to a registered DialogTrigger
    // — its own onCloseAutoFocus does `preventDefault(); triggerRef.focus()`.
    // A dialog driven in controlled mode from plain buttons registers no
    // trigger, so focus would land on <body> at close. Capture the opener when
    // the dialog mounts and put it back ourselves.
    const openerRef = React.useRef<HTMLElement | null>(null);

    return (
      <DialogPortal>
        <DialogOverlay />
        <DialogPrimitive.Content
          ref={ref}
          className={cn(
            'fixed left-1/2 top-1/2 z-50 flex max-h-[85vh] w-full max-w-lg -translate-x-1/2 -translate-y-1/2 flex-col gap-4 border border-border bg-background p-6 shadow-lg data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95 sm:rounded-lg',
            className,
          )}
          onOpenAutoFocus={(event) => {
            // Fires before focus moves into the dialog, so document.activeElement
            // is still whatever opened it. Capturing at render time or in a mount
            // effect is too early and too late respectively.
            openerRef.current = document.activeElement as HTMLElement | null;
            onOpenAutoFocus?.(event);
          }}
          onCloseAutoFocus={(event) => {
            onCloseAutoFocus?.(event);
            // A consumer that redirected focus itself has already had its say.
            if (event.defaultPrevented) return;
            event.preventDefault();
            openerRef.current?.focus();
          }}
          onEscapeKeyDown={(event) => {
            onEscapeKeyDown?.(event);
            if (!dismissible) event.preventDefault();
          }}
          onInteractOutside={(event) => {
            onInteractOutside?.(event);
            if (!dismissible) event.preventDefault();
          }}
          {...props}
        >
          {/* The body scrolls, not the box: the close button is positioned
              against the box and would otherwise scroll out of view. */}
          <div className="min-h-0 flex-1 overflow-y-auto">{children}</div>
          {/* Rendered after children so Radix's initial autofocus lands on the
              first control in the body rather than on Close. */}
          <DialogPrimitive.Close
            disabled={!dismissible}
            className="absolute right-4 top-4 rounded-sm opacity-70 ring-offset-background transition-opacity hover:opacity-100 focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-2 disabled:pointer-events-none disabled:opacity-50"
          >
            <X className="h-4 w-4" />
            <span className="sr-only">Close</span>
          </DialogPrimitive.Close>
        </DialogPrimitive.Content>
      </DialogPortal>
    );
  },
);
DialogContent.displayName = DialogPrimitive.Content.displayName;

const DialogHeader = ({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) => (
  <div className={cn('flex flex-col space-y-1.5 text-center sm:text-left', className)} {...props} />
);
DialogHeader.displayName = 'DialogHeader';

const DialogFooter = ({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) => (
  <div
    className={cn('flex flex-col-reverse sm:flex-row sm:justify-end sm:space-x-2', className)}
    {...props}
  />
);
DialogFooter.displayName = 'DialogFooter';

const DialogTitle = React.forwardRef<
  React.ElementRef<typeof DialogPrimitive.Title>,
  React.ComponentPropsWithoutRef<typeof DialogPrimitive.Title>
>(({ className, ...props }, ref) => (
  <DialogPrimitive.Title
    ref={ref}
    className={cn('text-lg font-semibold leading-none tracking-tight', className)}
    {...props}
  />
));
DialogTitle.displayName = DialogPrimitive.Title.displayName;

const DialogDescription = React.forwardRef<
  React.ElementRef<typeof DialogPrimitive.Description>,
  React.ComponentPropsWithoutRef<typeof DialogPrimitive.Description>
>(({ className, ...props }, ref) => (
  <DialogPrimitive.Description
    ref={ref}
    className={cn('text-sm text-muted-foreground', className)}
    {...props}
  />
));
DialogDescription.displayName = DialogPrimitive.Description.displayName;

export {
  Dialog,
  DialogTrigger,
  DialogPortal,
  DialogOverlay,
  DialogClose,
  DialogContent,
  DialogHeader,
  DialogFooter,
  DialogTitle,
  DialogDescription,
};
```

Two things that are load-bearing and easy to break:

- The four handler props (`onOpenAutoFocus`, `onCloseAutoFocus`, `onEscapeKeyDown`, `onInteractOutside`) are **destructured out** of `props`, so the trailing `{...props}` cannot overwrite the wrappers. Leave them destructured.
- `DialogHeader` / `DialogFooter` get literal `displayName` strings because they have no Radix counterpart. Every other wrapper takes its `displayName` from the primitive it wraps.

- [ ] **Step 5: Run the test to verify it passes**

```sh
export PATH="$HOME/.nvm/versions/node/v24.12.0/bin:$PATH"
cd apps/web && npx vitest run src/components/ui/dialog.test.tsx
```

Expected: PASS — 13 tests.

- [ ] **Step 6: Lint and format**

```sh
export PATH="$HOME/.nvm/versions/node/v24.12.0/bin:$PATH"
cd /home/tumidanski/source/MyFleet/.worktrees/task-016-add-vehicle-dialog
npm run format
npm run lint --workspace apps/web
```

Expected: format rewrites nothing of substance; lint exits 0 with no warnings (`--max-warnings 0`).

- [ ] **Step 7: Commit**

```bash
git add apps/web/package.json package-lock.json apps/web/src/components/ui/dialog.tsx apps/web/src/components/ui/dialog.test.tsx
git commit -m "feat(web): add shared Dialog primitive

Wraps @radix-ui/react-dialog following the select.tsx conventions, and adds
two things Radix does not provide: a dismissible prop that refuses Escape,
outside-click and close-button dismissal as one invariant, and focus
restoration for dialogs opened from a plain button in controlled mode (Radix
restores only to a registered DialogTrigger, so focus would land on body)."
```

---

## Task 2: `VehicleList` empty-state action

**Files:**
- Modify: `apps/web/src/components/features/vehicles/VehicleList.tsx:1-35`
- Test: `apps/web/src/components/features/vehicles/VehicleList.test.tsx` (create)

**Interfaces:**
- Consumes: nothing from Task 1.
- Produces: `VehicleList` accepts a third, optional prop
  `emptyAction?: ReactNode`. The empty-state copy branches on that prop's
  presence alone — `VehiclesPage` (Task 3) passes the already-role-gated node.

- [ ] **Step 1: Write the failing test**

Create `apps/web/src/components/features/vehicles/VehicleList.test.tsx`:

```tsx
import { describe, it, expect } from 'vitest';
import { screen } from '@testing-library/react';
import { renderWithProviders } from '../../../test/renderWithProviders';
import { VehicleList } from './VehicleList';
import type { Vehicle } from '../../../types/models/vehicle';

function makeVehicle(id: string, model: string): Vehicle {
  return {
    type: 'vehicles',
    id,
    attributes: { fleetId: 'f1', make: 'Honda', model, year: 2019 },
  };
}

describe('VehicleList — empty state', () => {
  it('invites the user to add one and renders the action it was given', () => {
    renderWithProviders(
      <VehicleList
        vehicles={[]}
        isLoading={false}
        emptyAction={<button type="button">Add Vehicle</button>}
      />,
    );

    expect(
      screen.getByText('No vehicles yet. Add your first one to get started.'),
    ).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Add Vehicle' })).toBeInTheDocument();
  });

  it('states the fact without a call to action when it has no action to offer', () => {
    // A viewer cannot add a vehicle, so "add your first one" would be an
    // instruction they cannot follow. Keying the copy off the prop rather than
    // off a role makes it impossible to promise an action that is not rendered.
    renderWithProviders(<VehicleList vehicles={[]} isLoading={false} />);

    expect(screen.getByText('No vehicles yet.')).toBeInTheDocument();
    expect(screen.queryByText(/Add your first one/)).not.toBeInTheDocument();
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });
});

describe('VehicleList — populated', () => {
  it('renders a card per vehicle and no empty-state copy', () => {
    renderWithProviders(
      <VehicleList
        vehicles={[makeVehicle('v1', 'Civic'), makeVehicle('v2', 'Accord')]}
        isLoading={false}
        emptyAction={<button type="button">Add Vehicle</button>}
      />,
    );

    expect(screen.getByRole('link', { name: '2019 Honda Civic' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: '2019 Honda Accord' })).toBeInTheDocument();
    expect(screen.queryByText(/No vehicles yet/)).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Add Vehicle' })).not.toBeInTheDocument();
  });
});

describe('VehicleList — loading', () => {
  it('shows skeletons and no empty-state copy while loading', () => {
    // The empty-state branch must stay behind the loading branch: an empty
    // array during the first fetch is "not known yet", not "none exist".
    renderWithProviders(
      <VehicleList
        vehicles={[]}
        isLoading
        emptyAction={<button type="button">Add Vehicle</button>}
      />,
    );

    expect(screen.queryByText(/No vehicles yet/)).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Add Vehicle' })).not.toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

```sh
export PATH="$HOME/.nvm/versions/node/v24.12.0/bin:$PATH"
cd apps/web && npx vitest run src/components/features/vehicles/VehicleList.test.tsx
```

Expected: FAIL. The `emptyAction` prop does not exist, so TypeScript rejects it and the viewer-copy test fails on the text (`No vehicles yet.` vs the current combined string).

- [ ] **Step 3: Write the implementation**

Replace `apps/web/src/components/features/vehicles/VehicleList.tsx` in full:

```tsx
import type { ReactNode } from 'react';
import { VehicleCard, VehicleCardSkeleton } from './VehicleCard';
import type { Vehicle } from '../../../types/models/vehicle';

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
  if (isLoading) {
    return (
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {Array.from({ length: 6 }).map((_, i) => (
          <VehicleCardSkeleton key={i} />
        ))}
      </div>
    );
  }

  if (vehicles.length === 0) {
    return (
      <div className="rounded-lg border border-dashed border-border p-8 text-center text-muted-foreground">
        <p>
          {emptyAction
            ? 'No vehicles yet. Add your first one to get started.'
            : 'No vehicles yet.'}
        </p>
        {emptyAction && <div className="mt-4">{emptyAction}</div>}
      </div>
    );
  }

  return (
    <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
      {vehicles.map((vehicle) => (
        <VehicleCard key={vehicle.id} vehicle={vehicle} />
      ))}
    </div>
  );
}
```

The copy moves into a `<p>` so it is an element with its own `textContent`. Left as a bare text node beside the action `div`, `getByText('No vehicles yet. Add your first one to get started.')` would not match anything — the parent `div`'s text content also includes the button's label.

- [ ] **Step 4: Run the test to verify it passes**

```sh
export PATH="$HOME/.nvm/versions/node/v24.12.0/bin:$PATH"
cd apps/web && npx vitest run src/components/features/vehicles/VehicleList.test.tsx
```

Expected: PASS — 4 tests.

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/components/features/vehicles/VehicleList.tsx apps/web/src/components/features/vehicles/VehicleList.test.tsx
git commit -m "feat(web): give VehicleList an optional empty-state action

Copy branches on the action's presence rather than on a role, so the list
stays presentational and cannot promise an action it does not render."
```

---

## Task 3: The create dialog on `VehiclesPage`

**Files:**
- Modify: `apps/web/src/pages/VehiclesPage.tsx` (full rewrite of the component; `toCreateAttributes` at lines 13-25 is carried over verbatim)
- Test: `apps/web/src/pages/VehiclesPage.test.tsx` (create)

**Interfaces:**
- Consumes: `Dialog`, `DialogContent`, `DialogHeader`, `DialogTitle`, `DialogDescription` from `../components/ui/dialog` (Task 1); `emptyAction` on `VehicleList` (Task 2).
- Produces: nothing consumed by later tasks — Tasks 4 and 5 modify this same file.

- [ ] **Step 1: Write the failing test**

Create `apps/web/src/pages/VehiclesPage.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { screen, waitFor, within, fireEvent } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { renderWithProviders } from '../test/renderWithProviders';
import { vehicleService } from '../services/api/VehicleService';
import { toast } from 'sonner';
import type { AuthContextValue } from '../context/AuthContext';
import { VehiclesPage } from './VehiclesPage';
import type { Vehicle } from '../types/models/vehicle';

// Mock auth so role and fleet can be varied per test without standing up the
// provider stack — the pattern AppLayout.test.tsx established.
const mockAuth = vi.fn<() => AuthContextValue>();
vi.mock('../context/AuthContext', () => ({
  useAuth: () => mockAuth(),
}));

// Mock at the service boundary, as VehicleCard.test.tsx does, so the real
// query/mutation wiring (keys, invalidation, error normalisation) is exercised.
vi.mock('../services/api/VehicleService', () => ({
  vehicleService: { listByFleet: vi.fn(), createInFleet: vi.fn() },
}));

vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

function makeVehicle(id = 'v1'): Vehicle {
  return {
    type: 'vehicles',
    id,
    attributes: { fleetId: 'f1', make: 'Toyota', model: 'Corolla', year: 2020 },
  };
}

function setRole(role: AuthContextValue['role']): void {
  mockAuth.mockReturnValue({
    user: null,
    activeFleetId: 'f1',
    role,
    isAuthenticated: true,
    isLoading: false,
    login: vi.fn(),
    logout: vi.fn(),
  });
}

/**
 * The page's two triggers and the form's submit button all read "Add Vehicle".
 * This returns only the triggers — the ones outside the dialog — so a test can
 * never accidentally drive the submit button when it means to open the dialog.
 * DOM order puts the header trigger first.
 */
function triggers(): HTMLElement[] {
  return screen
    .getAllByRole('button', { name: 'Add Vehicle' })
    .filter((button) => !button.closest('[role="dialog"]'));
}

const headerTrigger = () => triggers()[0];
const submitButton = () =>
  within(screen.getByRole('dialog')).getByRole('button', { name: 'Add Vehicle' });

/** Fills the three required fields. */
async function fillRequired(): Promise<void> {
  const dialog = within(screen.getByRole('dialog'));
  await userEvent.type(dialog.getByLabelText('Make'), 'Toyota');
  await userEvent.type(dialog.getByLabelText('Model'), 'Corolla');
  await userEvent.type(dialog.getByLabelText('Year'), '2020');
}

beforeEach(() => {
  setRole('owner');
  vi.mocked(vehicleService.listByFleet).mockResolvedValue({ data: [] });
  vi.mocked(vehicleService.createInFleet).mockResolvedValue(makeVehicle());
});

afterEach(() => {
  vi.clearAllMocks();
});

describe('VehiclesPage — opening the dialog', () => {
  it('opens a titled, described dialog containing the create form from the header', async () => {
    renderWithProviders(<VehiclesPage />);
    await userEvent.click(headerTrigger());

    const dialog = screen.getByRole('dialog');
    expect(dialog).toHaveAccessibleName('Add Vehicle');
    expect(dialog).toHaveAccessibleDescription('Make, model, and year are required.');
    expect(within(dialog).getByLabelText('Make')).toBeInTheDocument();
  });

  it('opens the same dialog from the empty state', async () => {
    renderWithProviders(<VehiclesPage />);
    await waitFor(() => expect(triggers()).toHaveLength(2));

    await userEvent.click(triggers()[1]);
    expect(within(screen.getByRole('dialog')).getByLabelText('Make')).toBeInTheDocument();
  });

  it('keeps the header trigger rendered while the dialog is open', async () => {
    // The old inline form hid it; the dialog has no reason to.
    renderWithProviders(<VehiclesPage />);
    await userEvent.click(headerTrigger());
    expect(headerTrigger()).toBeInTheDocument();
  });

  it('leaves no inline card form on the page', async () => {
    renderWithProviders(<VehiclesPage />);
    expect(screen.queryByText('New Vehicle')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Make')).not.toBeInTheDocument();
  });

  it('offers a viewer neither trigger', async () => {
    setRole('viewer');
    renderWithProviders(<VehiclesPage />);
    await waitFor(() => expect(screen.getByText('No vehicles yet.')).toBeInTheDocument());
    expect(screen.queryByRole('button', { name: 'Add Vehicle' })).not.toBeInTheDocument();
  });
});

describe('VehiclesPage — submitting', () => {
  it('omits blank optionals from the payload, closes, and reports success', async () => {
    // The call arguments are the assertion that matters: toCreateAttributes
    // strips empty-string optionals, and that is exactly the behaviour a
    // refactor drops silently.
    renderWithProviders(<VehiclesPage />);
    await userEvent.click(headerTrigger());
    await fillRequired();
    await userEvent.click(submitButton());

    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(vehicleService.createInFleet).toHaveBeenCalledWith('f1', {
      make: 'Toyota',
      model: 'Corolla',
      year: 2020,
      nickname: undefined,
      trim: undefined,
      vin: undefined,
      notes: undefined,
      currentMileage: undefined,
    });
    expect(toast.success).toHaveBeenCalledWith('Vehicle added');
  });

  it('shows the created vehicle in the list', async () => {
    vi.mocked(vehicleService.listByFleet)
      .mockResolvedValueOnce({ data: [] })
      .mockResolvedValue({ data: [makeVehicle()] });

    renderWithProviders(<VehiclesPage />);
    await userEvent.click(headerTrigger());
    await fillRequired();
    await userEvent.click(submitButton());

    expect(await screen.findByRole('link', { name: '2020 Toyota Corolla' })).toBeInTheDocument();
  });

  it('keeps the dialog open with inline errors when required fields are blank', async () => {
    renderWithProviders(<VehiclesPage />);
    await userEvent.click(headerTrigger());
    await userEvent.click(submitButton());

    expect(await screen.findByText('Make is required')).toBeInTheDocument();
    expect(screen.getByText('Model is required')).toBeInTheDocument();
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    expect(vehicleService.createInFleet).not.toHaveBeenCalled();
  });

  it('keeps the dialog open with the typed values when the request fails', async () => {
    vi.mocked(vehicleService.createInFleet).mockRejectedValue(new Error('boom'));

    renderWithProviders(<VehiclesPage />);
    await userEvent.click(headerTrigger());
    await fillRequired();
    await userEvent.click(submitButton());

    await waitFor(() => expect(toast.error).toHaveBeenCalled());
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    expect(within(screen.getByRole('dialog')).getByLabelText('Make')).toHaveValue('Toyota');
  });
});

describe('VehiclesPage — dismissing', () => {
  // Typed explicitly: a bare array literal infers a union element type and the
  // callback's `dismiss` parameter stops being callable.
  const dismissals: Array<[string, () => Promise<void>]> = [
    ['Escape', () => userEvent.keyboard('{Escape}')],
    ['the close button', () => userEvent.click(screen.getByRole('button', { name: 'Close' }))],
    [
      'Cancel',
      () =>
        userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Cancel' })),
    ],
  ];

  it.each(dismissals)('closes on %s without creating a vehicle', async (_label, dismiss) => {
    renderWithProviders(<VehiclesPage />);
    await userEvent.click(headerTrigger());
    await dismiss();

    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(vehicleService.createInFleet).not.toHaveBeenCalled();
  });

  it('closes on an outside pointer-down without creating a vehicle', async () => {
    // The path a real overlay click takes; userEvent cannot drive the overlay.
    renderWithProviders(<VehiclesPage />);
    await userEvent.click(headerTrigger());

    fireEvent.pointerDown(document.body);

    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(vehicleService.createInFleet).not.toHaveBeenCalled();
  });

  it('presents a blank form on reopen', async () => {
    renderWithProviders(<VehiclesPage />);
    await userEvent.click(headerTrigger());
    await userEvent.type(within(screen.getByRole('dialog')).getByLabelText('Nickname'), 'Scratch');
    await userEvent.keyboard('{Escape}');
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());

    await userEvent.click(headerTrigger());
    expect(within(screen.getByRole('dialog')).getByLabelText('Nickname')).toHaveValue('');
  });

  it('returns focus to the header trigger it was opened from', async () => {
    renderWithProviders(<VehiclesPage />);
    const trigger = headerTrigger();
    await userEvent.click(trigger);
    await userEvent.keyboard('{Escape}');

    await waitFor(() => expect(document.activeElement).toBe(trigger));
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

```sh
export PATH="$HOME/.nvm/versions/node/v24.12.0/bin:$PATH"
cd apps/web && npx vitest run src/pages/VehiclesPage.test.tsx
```

Expected: FAIL — no `role="dialog"` is ever found, because the page still renders the inline `Card` form.

- [ ] **Step 3: Write the implementation**

Replace `apps/web/src/pages/VehiclesPage.tsx` in full:

```tsx
import { useState } from 'react';
import { toast } from 'sonner';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import { useAuth } from '../context/AuthContext';
import { useVehicles, useCreateVehicle } from '../lib/hooks/api/vehicles';
import { VehicleList } from '../components/features/vehicles/VehicleList';
import { VehicleForm } from '../components/features/vehicles/VehicleForm';
import { Button } from '../components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '../components/ui/dialog';
import type { VehicleFormInput } from '../lib/schemas/vehicle';
import type { CreateVehicleAttributes } from '../types/models/vehicle';

// Strip empty-string optionals so the backend receives clean attributes.
function toCreateAttributes(values: VehicleFormInput): CreateVehicleAttributes {
  return {
    make: values.make,
    model: values.model,
    year: values.year,
    nickname: values.nickname || undefined,
    trim: values.trim || undefined,
    vin: values.vin || undefined,
    notes: values.notes || undefined,
    currentMileage: values.currentMileage,
  };
}

export function VehiclesPage() {
  const { activeFleetId, role } = useAuth();
  const { data, isLoading } = useVehicles(activeFleetId);
  const createVehicle = useCreateVehicle(activeFleetId ?? '');
  const [open, setOpen] = useState(false);

  // Viewers are read-only; only members/owners can add vehicles.
  const canWrite = role === 'owner' || role === 'member';

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

  return (
    <div>
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold">Vehicles</h1>
        {canWrite && (
          <Button type="button" onClick={() => setOpen(true)}>
            Add Vehicle
          </Button>
        )}
      </div>

      {canWrite && (
        <Dialog open={open} onOpenChange={setOpen}>
          {/* Unmounted on close, which is what discards the form state — do not
              add forceMount. */}
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Add Vehicle</DialogTitle>
              <DialogDescription>Make, model, and year are required.</DialogDescription>
            </DialogHeader>
            <VehicleForm
              mode="create"
              onSubmit={handleCreate}
              onCancel={() => setOpen(false)}
              submitting={createVehicle.isPending}
            />
          </DialogContent>
        </Dialog>
      )}

      <div className="mt-6">
        <VehicleList
          vehicles={data?.data ?? []}
          isLoading={isLoading}
          emptyAction={
            canWrite ? (
              <Button type="button" onClick={() => setOpen(true)}>
                Add Vehicle
              </Button>
            ) : undefined
          }
        />
      </div>
    </div>
  );
}
```

`Card`, `CardContent`, `CardHeader`, `CardTitle` are no longer used — remove that import line entirely. ESLint's unused-import rule will catch it if you forget.

- [ ] **Step 4: Run the test to verify it passes**

```sh
export PATH="$HOME/.nvm/versions/node/v24.12.0/bin:$PATH"
cd apps/web && npx vitest run src/pages/VehiclesPage.test.tsx
```

Expected: PASS — 15 tests.

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/pages/VehiclesPage.tsx apps/web/src/pages/VehiclesPage.test.tsx
git commit -m "feat(web): create vehicles from a modal dialog

Replaces the inline card form, so the grid no longer reflows when the create
flow opens, and adds a matching trigger to the empty state. Create behaviour,
payload mapping, toasts and role gating are unchanged."
```

---

## Task 4: Not dismissible while the create request is in flight

**Files:**
- Modify: `apps/web/src/components/features/vehicles/VehicleForm.tsx:180-184` (one attribute)
- Modify: `apps/web/src/pages/VehiclesPage.tsx` (the `Dialog` / `DialogContent` props)
- Test: `apps/web/src/pages/VehiclesPage.test.tsx` (append a describe block)

**Interfaces:**
- Consumes: `dismissible?: boolean` on `DialogContent` (Task 1); `createVehicle.isPending` from the existing `useCreateVehicle` mutation.
- Produces: nothing new. `VehicleForm`'s prop signature is unchanged — only the Cancel button's rendered `disabled` attribute changes.

> **Note on design §7 of the PRD.** The PRD expected `VehicleForm.tsx` to need no changes, while FR-2.5 requires Cancel to be unavailable during submit; today it is never disabled. Design §5.3 resolves this in favour of the one-line change, which is what this task implements. It touches no rendering branch: `edit` mode renders the identical element tree and only behaves differently while a save is genuinely in flight, where an inert Cancel is correct anyway.

- [ ] **Step 1: Write the failing test**

Append to `apps/web/src/pages/VehiclesPage.test.tsx`:

```tsx
describe('VehiclesPage — locked while the create request is in flight', () => {
  /**
   * Opens the dialog and submits with a create that never settles, so the page
   * is parked in the pending state for the assertion.
   */
  async function submitAndHang(): Promise<void> {
    // Never settles, so the page stays parked in the pending state.
    vi.mocked(vehicleService.createInFleet).mockReturnValue(new Promise<Vehicle>(() => {}));
    renderWithProviders(<VehiclesPage />);
    await userEvent.click(headerTrigger());
    await fillRequired();
    await userEvent.click(submitButton());
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Close' })).toBeDisabled(),
    );
  }

  // Each route is asserted on its own: wiring two of the three and calling it
  // done is the regression this block exists to catch.
  it('ignores Escape', async () => {
    await submitAndHang();
    await userEvent.keyboard('{Escape}');
    expect(screen.getByRole('dialog')).toBeInTheDocument();
  });

  it('ignores an outside pointer-down', async () => {
    await submitAndHang();
    fireEvent.pointerDown(document.body);
    expect(screen.getByRole('dialog')).toBeInTheDocument();
  });

  it('disables the close button', async () => {
    await submitAndHang();
    expect(screen.getByRole('button', { name: 'Close' })).toBeDisabled();
  });

  it('disables Cancel', async () => {
    // A Cancel that looks live but does nothing reads as a broken app; a
    // Cancel that works would abandon a vehicle that still gets created.
    await submitAndHang();
    expect(
      within(screen.getByRole('dialog')).getByRole('button', { name: 'Cancel' }),
    ).toBeDisabled();
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

```sh
export PATH="$HOME/.nvm/versions/node/v24.12.0/bin:$PATH"
cd apps/web && npx vitest run src/pages/VehiclesPage.test.tsx
```

Expected: FAIL — all four cases. `submitAndHang`'s `waitFor` times out because the close button is never disabled.

- [ ] **Step 3: Disable Cancel while submitting**

In `apps/web/src/components/features/vehicles/VehicleForm.tsx`, change the Cancel button (currently lines 180-184):

```tsx
          {onCancel && (
            <Button type="button" variant="outline" onClick={onCancel} disabled={submitting}>
              Cancel
            </Button>
          )}
```

That is the entire change to this file — symmetric with the submit button on the next line.

- [ ] **Step 4: Lock the dialog while pending**

In `apps/web/src/pages/VehiclesPage.tsx`, replace the `Dialog` opening tag and `DialogContent` opening tag:

```tsx
        <Dialog
          open={open}
          onOpenChange={(next) => {
            // Backstop: `dismissible` already blocks the three user-facing
            // routes, but this guarantees no dismissal path Radix grows later
            // can close the dialog out from under an in-flight create.
            if (!next && createVehicle.isPending) return;
            setOpen(next);
          }}
        >
          <DialogContent dismissible={!createVehicle.isPending}>
```

The success path calls `setOpen(false)` directly rather than going through `onOpenChange`, so this guard does not interfere with closing on a completed create.

- [ ] **Step 5: Run the test to verify it passes**

```sh
export PATH="$HOME/.nvm/versions/node/v24.12.0/bin:$PATH"
cd apps/web && npx vitest run src/pages/VehiclesPage.test.tsx
```

Expected: PASS — 19 tests (15 from Task 3 plus 4).

- [ ] **Step 6: Commit**

```bash
git add apps/web/src/components/features/vehicles/VehicleForm.tsx apps/web/src/pages/VehiclesPage.tsx apps/web/src/pages/VehiclesPage.test.tsx
git commit -m "feat(web): lock the add-vehicle dialog while the create is in flight

Escape, outside-click, the close button and Cancel are all inert until the
request settles, so a vehicle can never be created after the user believes
they abandoned the flow."
```

---

## Task 5: Focus after the empty state disappears

**Files:**
- Modify: `apps/web/src/pages/VehiclesPage.tsx`
- Test: `apps/web/src/pages/VehiclesPage.test.tsx` (append a describe block)

**Interfaces:**
- Consumes: `onCloseAutoFocus` pass-through on `DialogContent` (Task 1) — the primitive runs the consumer's handler first and stands down if it calls `preventDefault()`.
- Produces: nothing. Final state of the file.

Creating the first vehicle from the empty-state button unmounts that button along with the empty state, so restoring focus to it would target a detached node. The header trigger is the correct destination.

The condition is `opener === 'empty' && created`, **not** a DOM liveness check such as `emptyButtonRef.current?.isConnected`. Liveness is the wrong signal: on success the dialog closes as soon as `mutateAsync` resolves, while the list is still refetching, so the empty-state button is usually *still* attached when `onCloseAutoFocus` fires. The outcome is deterministic; the DOM timing is not.

- [ ] **Step 1: Write the failing test**

Append to `apps/web/src/pages/VehiclesPage.test.tsx`:

```tsx
describe('VehiclesPage — focus after the empty state unmounts', () => {
  it('leaves focus on the header trigger after creating from the empty state', async () => {
    vi.mocked(vehicleService.listByFleet)
      .mockResolvedValueOnce({ data: [] })
      .mockResolvedValue({ data: [makeVehicle()] });

    renderWithProviders(<VehiclesPage />);
    await waitFor(() => expect(triggers()).toHaveLength(2));

    await userEvent.click(triggers()[1]);
    await fillRequired();
    await userEvent.click(submitButton());

    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    // The empty-state button is gone with the empty state, so the default
    // restoration would have dropped focus on <body>.
    await waitFor(() => expect(document.activeElement).toBe(headerTrigger()));
    expect(document.activeElement).not.toBe(document.body);
  });

  it('returns focus to the empty-state trigger when the create is abandoned', async () => {
    // Cancelling leaves the empty state standing, so the redirect must not fire
    // and the button the user actually came from keeps its focus.
    renderWithProviders(<VehiclesPage />);
    await waitFor(() => expect(triggers()).toHaveLength(2));

    const emptyTrigger = triggers()[1];
    await userEvent.click(emptyTrigger);
    await userEvent.keyboard('{Escape}');

    await waitFor(() => expect(document.activeElement).toBe(emptyTrigger));
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

```sh
export PATH="$HOME/.nvm/versions/node/v24.12.0/bin:$PATH"
cd apps/web && npx vitest run src/pages/VehiclesPage.test.tsx -t "focus after the empty state"
```

Expected: the first case FAILS — `document.activeElement` is `document.body`, because the captured opener has been detached. The second case passes already (the primitive's default restoration handles it) and must keep passing.

- [ ] **Step 3: Write the implementation**

In `apps/web/src/pages/VehiclesPage.tsx`:

Add `useRef` to the React import:

```tsx
import { useRef, useState } from 'react';
```

Add the tracking state below `const [open, setOpen] = useState(false);`:

```tsx
  // Refs, not state: nothing renders from these, they are read only inside
  // onCloseAutoFocus, and making them state would re-render for nothing.
  const openedFromRef = useRef<'header' | 'empty'>('header');
  const createdRef = useRef(false);
  const headerButtonRef = useRef<HTMLButtonElement>(null);

  const openFrom = (source: 'header' | 'empty') => {
    openedFromRef.current = source;
    createdRef.current = false;
    setOpen(true);
  };
```

Record the outcome in `handleCreate`, immediately before closing:

```tsx
      await createVehicle.mutateAsync(toCreateAttributes(values));
      toast.success('Vehicle added');
      createdRef.current = true;
      setOpen(false);
```

Point the header button at its ref and route both triggers through `openFrom`:

```tsx
        {canWrite && (
          <Button type="button" ref={headerButtonRef} onClick={() => openFrom('header')}>
            Add Vehicle
          </Button>
        )}
```

```tsx
            canWrite ? (
              <Button type="button" onClick={() => openFrom('empty')}>
                Add Vehicle
              </Button>
            ) : undefined
```

Add the redirect to `DialogContent`:

```tsx
          <DialogContent
            dismissible={!createVehicle.isPending}
            onCloseAutoFocus={(event) => {
              // The empty-state button unmounts with the empty state once the
              // first vehicle exists, so the opener we would restore to is
              // about to be detached. Send focus to the header trigger instead.
              if (openedFromRef.current === 'empty' && createdRef.current) {
                event.preventDefault();
                headerButtonRef.current?.focus();
              }
            }}
          >
```

Every other close path falls through to the primitive's own restoration, which puts focus back on whichever trigger was used.

- [ ] **Step 4: Run the test to verify it passes**

```sh
export PATH="$HOME/.nvm/versions/node/v24.12.0/bin:$PATH"
cd apps/web && npx vitest run src/pages/VehiclesPage.test.tsx
```

Expected: PASS — 21 tests.

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/pages/VehiclesPage.tsx apps/web/src/pages/VehiclesPage.test.tsx
git commit -m "fix(web): keep focus on the header trigger after the first vehicle

Creating from the empty-state button unmounts that button with the empty
state, so focus would otherwise be restored to a detached node and fall to
document.body."
```

---

## Task 6: Full verification

**Files:** none modified. This task proves the branch.

**Interfaces:** none.

- [ ] **Step 1: Format and lint**

```sh
export PATH="$HOME/.nvm/versions/node/v24.12.0/bin:$PATH"
cd /home/tumidanski/source/MyFleet/.worktrees/task-016-add-vehicle-dialog
npm run format
npm run format:check
npm run lint
```

Expected: `format:check` reports all files formatted; `lint` exits 0 across all three workspaces with no warnings.

- [ ] **Step 2: Type-check and build**

```sh
export PATH="$HOME/.nvm/versions/node/v24.12.0/bin:$PATH"
npm run build
```

Expected: `tsc -b` clean, Vite build succeeds for `apps/web`, and both packages build.

- [ ] **Step 3: Run the full test suite**

```sh
export PATH="$HOME/.nvm/versions/node/v24.12.0/bin:$PATH"
npm test
```

Expected, against the recorded baseline of 293 / 7 / 10:

| Workspace | Baseline | Expected now |
| --- | --- | --- |
| `apps/web` | 293 in 39 files | **331 in 42 files** (+13 dialog, +4 VehicleList, +21 VehiclesPage) |
| `shared-ts` | 7 in 2 files | 7 in 2 files |
| `ui-components` | 10 in 1 file | 10 in 1 file |

Nothing pre-existing may fail. If the `apps/web` count differs from 331, reconcile it against the per-task counts (13 / 4 / 21) before moving on — a silently skipped `it.each` row is the usual cause.

- [ ] **Step 4: Confirm the blast radius**

```sh
cd /home/tumidanski/source/MyFleet/.worktrees/task-016-add-vehicle-dialog
git status --porcelain
git diff --stat main...HEAD
```

Expected: `git status` clean. The diff against `main` touches exactly these nine paths and no others:

```
apps/web/package.json
apps/web/src/components/features/vehicles/VehicleForm.tsx
apps/web/src/components/features/vehicles/VehicleList.test.tsx
apps/web/src/components/features/vehicles/VehicleList.tsx
apps/web/src/components/ui/dialog.test.tsx
apps/web/src/components/ui/dialog.tsx
apps/web/src/pages/VehiclesPage.test.tsx
apps/web/src/pages/VehiclesPage.tsx
package-lock.json
```

`apps/web/src/pages/VehicleDetailPage.tsx` must **not** appear. Verify explicitly:

```sh
git diff --name-only main...HEAD | grep -c VehicleDetailPage   # must print 0
```

- [ ] **Step 5: Manual smoke check against the acceptance criteria**

Run the app and confirm the things jsdom cannot prove — jsdom does no layout, so the centring, the height cap, and the internal scroll are unverified by the suite:

```sh
export PATH="$HOME/.nvm/versions/node/v24.12.0/bin:$PATH"
npm run dev --workspace apps/web
```

Check:
1. The dialog is centred horizontally and vertically over a dimmed overlay.
2. The vehicle grid behind it does not shift when the dialog opens.
3. Narrow the browser window until the form exceeds 85vh: the dialog body scrolls and **the close button stays pinned** in the top-right corner.
4. Toggle dark mode: the dialog surface, border, text and close button all read correctly in both themes.
5. Tab through the open dialog: focus cycles within it and never reaches the page behind. The close button shows a visible focus ring.

- [ ] **Step 6: Commit anything the format pass rewrote**

```bash
git add -A
git commit -m "chore(web): formatting pass for the add-vehicle dialog" || echo "nothing to commit"
```

---

## Coverage map

Every PRD functional requirement, mapped to where it is implemented and proved.

| Requirement | Task | Proof |
| --- | --- | --- |
| FR-1.1 dependency added, alphabetical | 1 | `package.json` diff, Task 6 Step 4 |
| FR-1.2 ten exports | 1 | export block; consumed by `dialog.test.tsx` and `VehiclesPage.tsx` |
| FR-1.3 forwardRef / `cn()` / `displayName` | 1 | implementation; read alongside `select.tsx` |
| FR-1.4 existing tokens only | 1 | class lists; Task 6 Step 5.4 dark-mode check |
| FR-1.5 portal, centred, `max-w-lg`, `max-h-[85vh]`, internal scroll | 1 | class list; Task 6 Step 5.1/5.3 |
| FR-1.6 close button named "Close" | 1 | `Dialog — announcement` |
| FR-1.7 `data-[state=*]` transitions | 1 | class list mirroring `select.tsx:66` |
| FR-2.1 inline card removed, form in dialog | 3 | `leaves no inline card form on the page` |
| FR-2.2 title and description | 3 | `opens a titled, described dialog…` |
| FR-2.3 opens from both triggers | 3 | `…from the header`, `…from the empty state` |
| FR-2.4 closes on success / Cancel / X / Escape / overlay | 3 | `VehiclesPage — dismissing` (4 cases) + the success case |
| FR-2.5 not dismissible while pending | 4 | `VehiclesPage — locked while…` (4 cases) |
| FR-2.6 blank form on reopen, no `forceMount` | 3 | `presents a blank form on reopen`; `Dialog — content lifecycle` |
| FR-2.7 success toast + close | 3 | `omits blank optionals…` |
| FR-2.8 error toast, dialog stays open, values intact | 3 | `keeps the dialog open with the typed values…` |
| FR-2.9 `toCreateAttributes` unchanged | 3 | call-argument assertion in `omits blank optionals…` |
| FR-3.1 header trigger stays visible | 3 | `keeps the header trigger rendered…` |
| FR-3.2 both triggers gated on write | 3 | `offers a viewer neither trigger` |
| FR-3.3 `emptyAction` prop, no auth import | 2 | `VehicleList.test.tsx`; the file imports no auth module |
| FR-3.4 empty-state trigger for writers | 3 | `opens the same dialog from the empty state` |
| FR-3.5 viewer copy without CTA | 2 | `states the fact without a call to action…` |
| FR-4.1 modal, background inert | 1 | `aria-modal` assertion; Radix `modal` not disabled |
| FR-4.2 focus trapped | 1 | Radix `FocusScope`; Task 6 Step 5.5 |
| FR-4.3 initial focus on the first field | 1 | `moves focus to the first control in the body…` |
| FR-4.4 focus returns to the opener | 1, 3 | `returns focus to the plain button that opened it`; `returns focus to the header trigger…`; `…to the empty-state trigger when abandoned` |
| FR-4.5 redirect to header after the empty state unmounts | 5 | `leaves focus on the header trigger after creating from the empty state` |
| FR-4.6 labelled and described | 1, 3 | `toHaveAccessibleName` / `toHaveAccessibleDescription` in both suites |
| NFR contents not mounted until first open | 1 | `does not mount its children before the first open` |
| NFR `dialog.tsx` has no vehicle-specific logic | 1 | file contents; reusable harness in its own test |
| NFR build hygiene | 6 | Steps 1-3 |
