import * as React from 'react';

/**
 * True below the sidebar primitive's mobile breakpoint, where the sidebar
 * becomes an off-canvas sheet (FR-SIDEBAR-6).
 *
 * Vendored alongside components/ui/sidebar.tsx, which shadcn ships as
 * hooks/use-mobile.tsx. It lives here because this repo puts hooks in
 * src/lib/hooks/<camelCase>.ts and components in src/components/ui.
 *
 * The listener re-reads window.innerWidth rather than trusting the event's
 * `matches`, which is what makes it safe under the shared matchMedia stub in
 * src/test/setup.ts: that stub fans every `change` out to every listener
 * regardless of query, and this handler ignores the event entirely.
 */
const MOBILE_BREAKPOINT = 768;

export function useIsMobile(): boolean {
  const [isMobile, setIsMobile] = React.useState<boolean | undefined>(undefined);

  React.useEffect(() => {
    const query = window.matchMedia(`(max-width: ${MOBILE_BREAKPOINT - 1}px)`);
    const onChange = () => {
      setIsMobile(window.innerWidth < MOBILE_BREAKPOINT);
    };
    query.addEventListener('change', onChange);
    setIsMobile(window.innerWidth < MOBILE_BREAKPOINT);
    return () => query.removeEventListener('change', onChange);
  }, []);

  return !!isMobile;
}
