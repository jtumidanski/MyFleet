import type { LucideIcon } from 'lucide-react';
import { Link, matchPath, useLocation } from 'react-router-dom';
import {
  SidebarGroup,
  SidebarGroupContent,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from '../ui/sidebar';

export interface FrameNavItem {
  to: string;
  label: string;
  icon: LucideIcon;
  /** Exact matching, exactly as NavLink's `end` (FR-NAV-5). */
  end?: boolean;
}

/**
 * One renderer, one nav table per shell (design §4.3).
 *
 * The sidebar primitive renders divs and a ul, no landmark — hence the <nav>
 * wrapper, and a per-shell label so the two shells' landmarks stay
 * distinguishable.
 */
export function FrameNav({
  items,
  ariaLabel,
}: {
  items: readonly FrameNavItem[];
  ariaLabel: string;
}) {
  const { pathname } = useLocation();

  return (
    <nav aria-label={ariaLabel}>
      <SidebarGroup>
        <SidebarGroupContent>
          <SidebarMenu>
            {items.map((item) => {
              const Icon = item.icon;
              // NavLink's own matcher, called directly: SidebarMenuButton needs
              // isActive as a VALUE so it can set data-active and apply the
              // primitive's active styling — including in the collapsed rail.
              // NavLink only hands isActive to a render prop inside the anchor,
              // which is too late to reach the button.
              const isActive =
                matchPath({ path: item.to, end: item.end ?? false }, pathname) !== null;

              return (
                <SidebarMenuItem key={item.to}>
                  {/* tooltip surfaces the label on the collapsed rail only —
                      the primitive hides it while the sidebar is expanded
                      (FR-NAV-4). */}
                  <SidebarMenuButton asChild isActive={isActive} tooltip={item.label}>
                    <Link to={item.to}>
                      <Icon aria-hidden="true" />
                      <span>{item.label}</span>
                    </Link>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              );
            })}
          </SidebarMenu>
        </SidebarGroupContent>
      </SidebarGroup>
    </nav>
  );
}
