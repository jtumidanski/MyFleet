import * as React from 'react';
import * as DialogPrimitive from '@radix-ui/react-dialog';
import { X } from 'lucide-react';
import { cn } from '../../lib/utils';

/**
 * Whether the surrounding Root is modal. Radix keeps this in a context it does
 * not export, and `DialogContent` has to know: the installed version emits no
 * `aria-modal` of its own, so this primitive asserts it, and asserting it on a
 * non-modal dialog would promise a containment Radix is not providing.
 */
const DialogModalContext = React.createContext(true);

const Dialog = ({
  modal = true,
  ...props
}: React.ComponentPropsWithoutRef<typeof DialogPrimitive.Root>) => (
  <DialogModalContext.Provider value={modal}>
    <DialogPrimitive.Root modal={modal} {...props} />
  </DialogModalContext.Provider>
);
Dialog.displayName = 'Dialog';

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
      // conventions.test.ts bans literal palette-color utility classes, which
      // rules out the brief's overlay class verbatim. Ruling: theme-split
      // semantic tokens instead of a single reused token — foreground/80 is
      // a near-black scrim in light mode, and background/80 is a near-black
      // scrim in dark mode, so the backdrop stays genuinely dark in both
      // themes rather than going inert (light-on-light) in one of them.
      'fixed inset-0 z-50 bg-foreground/80 dark:bg-background/80 data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0',
      className,
    )}
    {...props}
  />
));
DialogOverlay.displayName = DialogPrimitive.Overlay.displayName;

export interface DialogContentProps extends React.ComponentPropsWithoutRef<
  typeof DialogPrimitive.Content
> {
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
    const modal = React.useContext(DialogModalContext);

    return (
      <DialogPortal>
        <DialogOverlay />
        <DialogPrimitive.Content
          ref={ref}
          // The installed @radix-ui/react-dialog no longer sets this itself
          // (verified against dist/index.mjs), so it is asserted explicitly
          // here to keep the dialog announced as modal to assistive tech —
          // but only when the Root actually is modal, since a non-modal Root
          // gets neither a focus trap nor an aria-hidden page behind it.
          aria-modal={modal ? 'true' : undefined}
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
