import { ArrowLeft, Building2, LayoutDashboard, ScrollText, Trash2, Users } from 'lucide-react';
import { Link, Outlet } from 'react-router-dom';
import { BrandLink } from '../frame/BrandLink';
import { FrameHeader } from '../frame/FrameHeader';
import { FrameNav, type FrameNavItem } from '../frame/FrameNav';
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarHeader,
  SidebarInset,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
  SidebarRail,
  SidebarSeparator,
} from '../ui/sidebar';

const ADMIN_NAV: readonly FrameNavItem[] = [
  { to: '/admin', label: 'Overview', icon: LayoutDashboard, end: true },
  { to: '/admin/fleets', label: 'Fleets', icon: Building2 },
  { to: '/admin/users', label: 'Users', icon: Users },
  { to: '/admin/purges', label: 'Purges', icon: Trash2 },
  { to: '/admin/audit', label: 'Audit log', icon: ScrollText },
];

/**
 * The admin shell — deliberately NOT AppLayout.
 *
 * A dedicated shell gives destructive tooling an unmistakable mode boundary,
 * makes fleet browsing the centre of the console rather than a side trip, and
 * resolves the fleetless-admin routing problem structurally (FR-ADMIN-UI-2).
 * The two shells share PRIMITIVES and the header row; they are still two files.
 */
export function AdminLayout() {
  return (
    <SidebarProvider>
      <Sidebar collapsible="icon">
        <SidebarHeader>
          <BrandLink to="/admin" label="MyFleet" suffix="admin" ariaLabel="MyFleet admin home" />
        </SidebarHeader>
        <SidebarContent>
          <FrameNav items={ADMIN_NAV} ariaLabel="Admin" />
        </SidebarContent>
        {/*
          "Back to my fleet" sits in the footer, visually separated from the nav
          proper (FR-NAV-7). Footer rather than a sixth nav row is the
          structural expression of "this is the exit, not a destination" — the
          same intent the old border-t block carried, now carried by the
          primitive's own slot.
        */}
        <SidebarFooter>
          <SidebarSeparator />
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton asChild tooltip="Back to my fleet">
                <Link to="/">
                  <ArrowLeft aria-hidden="true" />
                  <span>Back to my fleet</span>
                </Link>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarFooter>
        <SidebarRail />
      </Sidebar>
      <SidebarInset>
        <FrameHeader />
        {/*
          The persistent mode band (FR-ADMIN-UI-3). danger-subtle, NOT
          --destructive: that token is reserved for destructive CONTROLS under
          the task-003 contract, and this is a mode indicator, not a button.

          It also states the stale-claim caveat in plain words rather than a
          tooltip. An operator who does not know that revoking admin takes up to
          15 minutes will assume a revocation took effect immediately, which is
          the one misunderstanding with an irreversible consequence.
        */}
        <div className="border-b border-danger-border bg-danger-subtle px-6 py-2 text-sm text-danger-subtle-foreground">
          <strong className="font-semibold">Platform admin.</strong> You can see and delete data
          across every fleet on this platform. Admin access is read from your sign-in token, so
          granting or revoking it takes up to 15 minutes to take effect.
        </div>
        <main className="flex-1 p-6">
          <Outlet />
        </main>
      </SidebarInset>
    </SidebarProvider>
  );
}
