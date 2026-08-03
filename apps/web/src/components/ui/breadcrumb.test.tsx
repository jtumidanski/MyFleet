import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from './breadcrumb';

function renderTrail() {
  return render(
    <Breadcrumb>
      <BreadcrumbList>
        <BreadcrumbItem>
          <BreadcrumbLink href="/">Home</BreadcrumbLink>
        </BreadcrumbItem>
        <BreadcrumbSeparator />
        <BreadcrumbItem>
          <BreadcrumbPage>Vehicles</BreadcrumbPage>
        </BreadcrumbItem>
      </BreadcrumbList>
    </Breadcrumb>,
  );
}

describe('Breadcrumb', () => {
  it('is a labelled navigation landmark', () => {
    renderTrail();
    expect(screen.getByRole('navigation', { name: 'Breadcrumb' })).toBeInTheDocument();
  });

  it('renders preceding crumbs as links', () => {
    renderTrail();
    expect(screen.getByRole('link', { name: 'Home' })).toHaveAttribute('href', '/');
  });

  // FR-CRUMB-3: the current page is non-interactive text carrying
  // aria-current="page" — not a link, not a disabled link.
  it('renders the current page as non-interactive text', () => {
    renderTrail();
    expect(screen.getByText('Vehicles')).toHaveAttribute('aria-current', 'page');
    expect(screen.queryByRole('link', { name: 'Vehicles' })).not.toBeInTheDocument();
  });

  it('hides the separator from assistive technology', () => {
    const { container } = renderTrail();
    const separator = container.querySelector('li[role="presentation"]');
    expect(separator).toHaveAttribute('aria-hidden', 'true');
  });
});
