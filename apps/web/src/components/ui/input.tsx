import * as React from 'react';
import { cn } from '../../lib/utils';

/**
 * Types whose value is picked from a browser-provided calendar/clock overlay.
 * Browsers only open that overlay when the small indicator glyph at the right
 * edge is clicked — focusing the field, or clicking one of its text segments,
 * leaves the user typing into segmented fields they have to discover. These
 * types get `showPicker()` wired to click and focus so the whole control is
 * the affordance.
 */
const PICKER_TYPES = new Set(['date', 'datetime-local', 'month', 'time', 'week']);

/**
 * Spinner buttons on a number field are a ~10px hit target that changes the
 * value by one, which is useless for the quantities this app collects (mileage,
 * cost) and actively harmful next to a scroll wheel. Suppressed for every
 * number input rather than per form, so the treatment cannot drift between
 * them. Typing, arrow keys, and validation are untouched.
 */
const NO_SPINNER =
  '[appearance:textfield] [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none [&::-webkit-inner-spin-button]:m-0 [&::-webkit-outer-spin-button]:m-0';

const Input = React.forwardRef<HTMLInputElement, React.InputHTMLAttributes<HTMLInputElement>>(
  ({ className, type, onClick, onFocus, ...props }, ref) => {
    const isPicker = !!type && PICKER_TYPES.has(type);

    /**
     * `showPicker` is unimplemented on older engines and throws
     * `NotAllowedError` when the call is not attributable to a user gesture —
     * which is exactly what a programmatic `.focus()` (autofocus, a form
     * restoring focus after a validation error) produces. Both are non-events
     * for the user, so they are swallowed: the field still works as a plain
     * typed input.
     */
    const openPicker = (el: HTMLInputElement) => {
      if (typeof el.showPicker !== 'function') return;
      try {
        el.showPicker();
      } catch {
        /* unsupported, or no user activation — the field stays typeable */
      }
    };

    return (
      <input
        type={type}
        className={cn(
          'flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background file:border-0 file:bg-transparent file:text-sm file:font-medium placeholder:text-muted-foreground focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50',
          type === 'number' && NO_SPINNER,
          isPicker && 'cursor-pointer',
          className,
        )}
        ref={ref}
        onClick={(event) => {
          onClick?.(event);
          if (isPicker && !event.defaultPrevented) openPicker(event.currentTarget);
        }}
        onFocus={(event) => {
          onFocus?.(event);
          if (isPicker && !event.defaultPrevented) openPicker(event.currentTarget);
        }}
        {...props}
      />
    );
  },
);
Input.displayName = 'Input';

export { Input };
