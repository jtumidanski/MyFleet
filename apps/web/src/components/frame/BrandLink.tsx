import { Link } from 'react-router-dom';
import { BrandMark } from '../BrandMark';
import { SidebarMenu, SidebarMenuButton, SidebarMenuItem } from '../ui/sidebar';

interface BrandLinkProps {
  to: string;
  label: string;
  /** The second, muted word — "admin" in the console's lockup. */
  suffix?: string;
  ariaLabel: string;
}

/**
 * The sidebar's brand lockup, as a link home (FR-BRAND-1/2).
 *
 * In AppLayout it targets `/`. In AdminLayout it targets `/admin`, so the
 * console's own lockup returns to the console overview rather than ejecting the
 * operator; leaving the console is what "Back to my fleet" is for (FR-NAV-7).
 *
 * Rendered through SidebarMenuButton so hover, focus ring and — critically —
 * the collapsed-rail behaviour are the primitive's. The mark is wrapped in a
 * span rather than being a direct child, because the button's `[&>svg]:size-4`
 * rule would otherwise shrink it to the nav icons' size.
 */
export function BrandLink({ to, label, suffix, ariaLabel }: BrandLinkProps) {
  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <SidebarMenuButton size="lg" asChild>
          <Link to={to} aria-label={ariaLabel}>
            <span className="flex size-8 shrink-0 items-center justify-center">
              <BrandMark className="h-5 w-5" />
            </span>
            <span className="truncate text-lg font-semibold">
              {label}
              {suffix ? <span className="text-muted-foreground"> {suffix}</span> : null}
            </span>
          </Link>
        </SidebarMenuButton>
      </SidebarMenuItem>
    </SidebarMenu>
  );
}
