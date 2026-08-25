import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { VehicleForm } from './VehicleForm';

// Queried by role + accessible name rather than by label text: the marker is a
// nested aria-hidden span, which dom-accessibility-api skips but Testing
// Library's label-content helper does not.
describe('VehicleForm required markers', () => {
  it('marks make, model and year in create mode', () => {
    render(<VehicleForm mode="create" onSubmit={vi.fn()} />);

    expect(screen.getByRole('textbox', { name: 'Make' })).toHaveAttribute(
      'aria-required',
      'true',
    );
    expect(screen.getByRole('textbox', { name: 'Model' })).toHaveAttribute(
      'aria-required',
      'true',
    );
    expect(screen.getByRole('spinbutton', { name: 'Year' })).toHaveAttribute(
      'aria-required',
      'true',
    );
  });

  it('leaves the optional fields unmarked in create mode', () => {
    render(<VehicleForm mode="create" onSubmit={vi.fn()} />);

    expect(screen.getByRole('textbox', { name: 'Nickname' })).not.toHaveAttribute(
      'aria-required',
    );
    expect(screen.getByRole('textbox', { name: 'VIN' })).not.toHaveAttribute('aria-required');
    expect(screen.getByRole('spinbutton', { name: 'Current Mileage' })).not.toHaveAttribute(
      'aria-required',
    );
  });

  // The user cannot change these in edit mode and cannot fail to supply them,
  // so an asterisk would be noise (FR-15).
  it('marks nothing in edit mode', () => {
    render(<VehicleForm mode="edit" onSubmit={vi.fn()} />);

    const make = screen.getByRole('textbox', { name: 'Make' });
    expect(make).toBeDisabled();
    expect(make).not.toHaveAttribute('aria-required');
    expect(screen.getByRole('textbox', { name: 'Model' })).not.toHaveAttribute('aria-required');
    expect(screen.getByRole('spinbutton', { name: 'Year' })).not.toHaveAttribute('aria-required');
  });

  it('renders the required legend', () => {
    render(<VehicleForm mode="create" onSubmit={vi.fn()} />);

    expect(screen.getByText('Required')).toBeInTheDocument();
  });
});
