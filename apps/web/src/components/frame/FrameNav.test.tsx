import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { Car, LayoutDashboard, Shield } from 'lucide-react';
import { FrameNav, type FrameNavItem } from './FrameNav';
import { SidebarProvider } from '../ui/sidebar';

const ITEMS: readonly FrameNavItem[] = [
  { to: '/', label: 'Dashboard', icon: LayoutDashboard, end: true },
  { to: '/vehicles', label: 'Vehicles', icon: Car },
  { to: '/admin', label: 'Admin', icon: Shield, end: true },
];

function renderNav(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <SidebarProvider>
        <FrameNav items={ITEMS} ariaLabel="Main" />
      </SidebarProvider>
    </MemoryRouter>,
  );
}

describe('FrameNav', () => {
  it('is a labelled navigation landmark', () => {
    renderNav('/');
    expect(screen.getByRole('navigation', { name: 'Main' })).toBeInTheDocument();
  });

  it('renders every item as a link with its label as the accessible name', () => {
    renderNav('/');
    for (const item of ITEMS) {
      expect(screen.getByRole('link', { name: item.label })).toHaveAttribute('href', item.to);
    }
  });

  // FR-NAV-1: the collapsed rail is only navigable if each link carries an icon.
  it('renders an icon inside every link', () => {
    renderNav('/');
    for (const item of ITEMS) {
      expect(screen.getByRole('link', { name: item.label }).querySelector('svg')).toBeTruthy();
    }
  });

  // FR-NAV-5, the case that matters most: `/` uses exact matching so it does
  // not light up on every route. Getting `end` wrong here lights every nav row
  // on every route — cheap to test, easy to miss by eye.
  it('does not light the exact-match root on a descendant route', () => {
    renderNav('/vehicles');
    expect(screen.getByRole('link', { name: 'Dashboard' })).toHaveAttribute('data-active', 'false');
  });

  it('lights a prefix-match item on its descendant route', () => {
    renderNav('/vehicles/veh-1');
    expect(screen.getByRole('link', { name: 'Vehicles' })).toHaveAttribute('data-active', 'true');
  });

  it('lights the root on the root route', () => {
    renderNav('/');
    expect(screen.getByRole('link', { name: 'Dashboard' })).toHaveAttribute('data-active', 'true');
  });

  it('does not light the exact-match admin entry on an admin subroute', () => {
    renderNav('/admin/fleets');
    expect(screen.getByRole('link', { name: 'Admin' })).toHaveAttribute('data-active', 'false');
  });
});
