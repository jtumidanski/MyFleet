import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { BrandLink } from './BrandLink';
import { SidebarProvider } from '../ui/sidebar';

function renderBrand(props: Parameters<typeof BrandLink>[0]) {
  return render(
    <MemoryRouter>
      <SidebarProvider>
        <BrandLink {...props} />
      </SidebarProvider>
    </MemoryRouter>,
  );
}

describe('BrandLink', () => {
  // FR-BRAND-1/2/3: a link with an accessible name, because the wordmark is
  // hidden in the collapsed rail.
  it('is a link to the given target with the given accessible name', () => {
    renderBrand({ to: '/', label: 'MyFleet', ariaLabel: 'MyFleet home' });

    expect(screen.getByRole('link', { name: 'MyFleet home' })).toHaveAttribute('href', '/');
  });

  it('targets the console root in the admin shell', () => {
    renderBrand({
      to: '/admin',
      label: 'MyFleet',
      suffix: 'admin',
      ariaLabel: 'MyFleet admin home',
    });

    expect(screen.getByRole('link', { name: 'MyFleet admin home' })).toHaveAttribute(
      'href',
      '/admin',
    );
  });

  it('renders the wordmark and its suffix', () => {
    renderBrand({
      to: '/admin',
      label: 'MyFleet',
      suffix: 'admin',
      ariaLabel: 'MyFleet admin home',
    });

    const link = screen.getByRole('link', { name: 'MyFleet admin home' });
    expect(link).toHaveTextContent('MyFleet');
    expect(link).toHaveTextContent('admin');
  });

  it('omits the suffix when there is none', () => {
    renderBrand({ to: '/', label: 'MyFleet', ariaLabel: 'MyFleet home' });

    expect(screen.getByRole('link', { name: 'MyFleet home' })).not.toHaveTextContent('admin');
  });

  // The mark is decorative; the link's aria-label is the accessible name, so a
  // labelled mark would make the brand announce twice.
  it('renders the mark and keeps it out of the accessibility tree', () => {
    renderBrand({ to: '/', label: 'MyFleet', ariaLabel: 'MyFleet home' });

    const svg = screen.getByRole('link', { name: 'MyFleet home' }).querySelector('svg');
    expect(svg).toBeTruthy();
    expect(svg).toHaveAttribute('aria-hidden', 'true');
  });
});
