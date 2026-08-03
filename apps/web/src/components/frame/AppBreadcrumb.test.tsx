import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { AppBreadcrumb } from './AppBreadcrumb';

// The resolving crumbs have their own tests and their own hooks; stubbing them
// keeps this file about the TRAIL — which crumbs render, which are links, and
// which is the current page.
vi.mock('./crumbs/VehicleNameCrumb', () => ({
  VehicleNameCrumb: ({ id }: { id: string }) => <>vehicle:{id}</>,
}));
vi.mock('./crumbs/FleetNameCrumb', () => ({
  FleetNameCrumb: ({ id }: { id: string }) => <>fleet:{id}</>,
}));

function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <AppBreadcrumb />
    </MemoryRouter>,
  );
}

describe('AppBreadcrumb', () => {
  // FR-CRUMB-5: the shell's own root crumb still renders, as current-page text.
  it('renders the single root crumb on /', () => {
    renderAt('/');
    expect(screen.getByText('Home')).toHaveAttribute('aria-current', 'page');
    expect(screen.queryByRole('link')).not.toBeInTheDocument();
  });

  it('renders the single root crumb on /admin', () => {
    renderAt('/admin');
    expect(screen.getByText('Admin')).toHaveAttribute('aria-current', 'page');
  });

  // FR-CRUMB-3 and FR-CRUMB-6.
  it('links preceding crumbs and marks the last as the current page', () => {
    renderAt('/vehicles');
    expect(screen.getByRole('link', { name: 'Home' })).toHaveAttribute('href', '/');
    expect(screen.getByText('Vehicles')).toHaveAttribute('aria-current', 'page');
    expect(screen.queryByRole('link', { name: 'Vehicles' })).not.toBeInTheDocument();
  });

  it('links the intermediate crumb to its own route in the console', () => {
    renderAt('/admin/fleets');
    expect(screen.getByRole('link', { name: 'Admin' })).toHaveAttribute('href', '/admin');
    expect(screen.getByText('Fleets')).toHaveAttribute('aria-current', 'page');
  });

  it('hands the captured id to the resolving crumb', () => {
    renderAt('/vehicles/veh-1');
    expect(screen.getByText(/vehicle:veh-1/)).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Vehicles' })).toHaveAttribute('href', '/vehicles');
  });

  it('hands the captured id to the fleet crumb', () => {
    renderAt('/admin/fleets/fleet-1');
    expect(screen.getByText(/fleet:fleet-1/)).toBeInTheDocument();
  });

  // FR-CRUMB-7: no shell, no breadcrumb.
  it('renders nothing on a route outside both shells', () => {
    const { container } = renderAt('/login');
    expect(container).toBeEmptyDOMElement();
  });

  it('is a labelled navigation landmark when it renders', () => {
    renderAt('/vehicles');
    expect(screen.getByRole('navigation', { name: 'Breadcrumb' })).toBeInTheDocument();
  });
});
