import { fireEvent } from '@testing-library/react';

/**
 * Fires the event sequence a real left-button click outside an open dialog
 * produces, in browser order: pointerdown, mousedown, pointerup, mouseup,
 * click. Defaults to `document.body`, the target an overlay click lands on
 * once the overlay itself is out of reach — userEvent cannot drive the Radix
 * overlay under jsdom.
 *
 * A bare `fireEvent.pointerDown` is not enough. Radix's dialog passes
 * `deferPointerDownOutside`, so a pointerdown whose `button` is 0 — every
 * left-button press — only arms the dismissal: the layer waits for the
 * matching `click` before dismissing, which is what stops a drag that starts
 * inside the content and releases outside it from closing the dialog.
 *
 * The bare pointerdown passed before only because jsdom had no `PointerEvent`
 * constructor. Testing Library fell back to a plain `Event`, whose `button` is
 * `undefined`, and Radix took its non-deferred branch. jsdom 27 added
 * `PointerEvent`, so `button` is now 0 and the deferred branch is taken — the
 * branch a browser has always taken. Driving the whole sequence keeps these
 * tests on the real dismissal path instead of on a jsdom gap.
 */
export function clickOutside(target: Element = document.body): void {
  fireEvent.pointerDown(target);
  fireEvent.mouseDown(target);
  fireEvent.pointerUp(target);
  fireEvent.mouseUp(target);
  fireEvent.click(target);
}
