import * as React from 'react';

/**
 * True below the sidebar primitive's mobile breakpoint, where the sidebar
 * becomes an off-canvas sheet (FR-SIDEBAR-6).
 *
 * Vendored alongside components/ui/sidebar.tsx, which shadcn ships as
 * hooks/use-mobile.tsx. It lives here because this repo puts hooks in
 * src/lib/hooks/<camelCase>.ts and components in src/components/ui.
 *
 * The snapshot re-reads window.innerWidth rather than trusting the event's
 * `matches`, which is what makes it safe under the shared matchMedia stub in
 * src/test/setup.ts: that stub fans every `change` out to every listener
 * regardless of query, and this subscription ignores the event entirely.
 *
 * useSyncExternalStore rather than useState + useEffect: the viewport is an
 * external store, and reading it through a snapshot means the first render
 * already has the real width instead of a `false` placeholder corrected one
 * commit later. It is also what react-hooks/set-state-in-effect asks for.
 */
const MOBILE_BREAKPOINT = 768;

function subscribe(onStoreChange: () => void): () => void {
  const query = window.matchMedia(`(max-width: ${MOBILE_BREAKPOINT - 1}px)`);
  query.addEventListener('change', onStoreChange);
  return () => query.removeEventListener('change', onStoreChange);
}

function getSnapshot(): boolean {
  return window.innerWidth < MOBILE_BREAKPOINT;
}

export function useIsMobile(): boolean {
  return React.useSyncExternalStore(subscribe, getSnapshot);
}
