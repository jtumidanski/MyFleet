import { Activity, Bell, Car, LayoutDashboard, Settings, Shield } from 'lucide-react';
import { Outlet } from 'react-router-dom';
import { useAuth } from '../context/AuthContext';
import { BrandLink } from './frame/BrandLink';
import { FrameHeader } from './frame/FrameHeader';
import { FrameNav, type FrameNavItem } from './frame/FrameNav';
import {
  Sidebar,
  SidebarContent,
  SidebarHeader,
  SidebarInset,
  SidebarProvider,
  SidebarRail,
} from './ui/sidebar';

const NAV: readonly FrameNavItem[] = [
  { to: '/', label: 'Dashboard', icon: LayoutDashboard, end: true },
  { to: '/vehicles', label: 'Vehicles', icon: Car },
  { to: '/activity', label: 'Activity', icon: Activity },
  { to: '/notifications', label: 'Notifications', icon: Bell },
  { to: '/settings', label: 'Settings', icon: Settings },
];

// The entry point is a convenience, not a control: its absence hides the
// door, and the server refuses entry regardless (FR-ADMIN-UI-5).
const ADMIN_ENTRY: FrameNavItem = { to: '/admin', label: 'Admin', icon: Shield, end: true };

/**
 * The fleet app's shell.
 *
 * What is left here is only what differs from the console: this nav table and
 * this brand target. The header row, the profile menu and the breadcrumb are
 * shared components under ./frame, because FR-HEADER-1, FR-HEADER-2 and
 * FR-PROFILE-6 between them make a copied row three contracts to keep in sync
 * by hand.
 *
 * The sidebar surface's colour rationale now lives beside the --sidebar tokens
 * in index.css, which is where the decision is made.
 */
export function AppLayout() {
  const { platformAdmin } = useAuth();
  const nav = platformAdmin ? [...NAV, ADMIN_ENTRY] : NAV;

  return (
    <SidebarProvider>
      <Sidebar collapsible="icon">
        <SidebarHeader>
          <BrandLink to="/" label="MyFleet" ariaLabel="MyFleet home" />
        </SidebarHeader>
        <SidebarContent>
          <FrameNav items={nav} ariaLabel="Main" />
        </SidebarContent>
        <SidebarRail />
      </Sidebar>
      <SidebarInset>
        <FrameHeader />
        <main className="flex-1 p-6">
          <Outlet />
        </main>
      </SidebarInset>
    </SidebarProvider>
  );
}
