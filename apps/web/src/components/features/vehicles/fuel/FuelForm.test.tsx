import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { FuelForm, COST_REQUIREMENT } from './FuelForm';

describe('FuelForm required markers', () => {
  it('marks date, mileage and gallons', () => {
    render(<FuelForm onSubmit={vi.fn()} />);

    // A datetime-local input has no mapped ARIA role, so it is reached through
    // its generated id rather than by role.
    const date = document.querySelector('input[type="datetime-local"]');
    expect(date).toHaveAttribute('aria-required', 'true');
    expect(screen.getByRole('spinbutton', { name: 'Mileage (miles)' })).toHaveAttribute(
      'aria-required',
      'true',
    );
    expect(screen.getByRole('spinbutton', { name: 'Gallons' })).toHaveAttribute(
      'aria-required',
      'true',
    );
  });

  // The requirement is cross-field, so an asterisk on either one would be a
  // lie (FR-21). It is stated in prose instead.
  it('marks neither cost field', () => {
    render(<FuelForm onSubmit={vi.fn()} />);

    expect(screen.getByRole('spinbutton', { name: 'Total Cost ($)' })).not.toHaveAttribute(
      'aria-required',
    );
    expect(screen.getByRole('spinbutton', { name: 'Price per Gallon ($)' })).not.toHaveAttribute(
      'aria-required',
    );
  });

  it('announces the either/or rule with each cost field', () => {
    render(<FuelForm onSubmit={vi.fn()} />);

    for (const name of ['Total Cost ($)', 'Price per Gallon ($)']) {
      const control = screen.getByRole('spinbutton', { name });
      const describedBy = control.getAttribute('aria-describedby');
      expect(describedBy).toBeTruthy();
      expect(document.getElementById(describedBy as string)).toHaveTextContent(COST_REQUIREMENT);
    }
  });

  it('states the rule once visibly, keeping the server-derives note', () => {
    render(<FuelForm onSubmit={vi.fn()} />);

    const visible = screen.getByText(/the server derives the missing value/i);
    expect(visible).toHaveTextContent(COST_REQUIREMENT.replace(/\.$/, ''));
    expect(visible.className).not.toContain('sr-only');
  });

  it('renders the required legend', () => {
    render(<FuelForm onSubmit={vi.fn()} />);

    expect(screen.getByText('Required')).toBeInTheDocument();
  });
});
