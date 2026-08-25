import { describe, it, expect, vi, beforeAll } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MaintenanceScheduleForm } from './MaintenanceScheduleForm';
import type { MaintenanceCategory } from '../../../../types/models/maintenanceCategory';

// cmdk scrolls its selected item into view; jsdom does not implement it.
beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn();
});

const categories: MaintenanceCategory[] = [
  {
    id: 'c1',
    type: 'maintenanceCategories',
    attributes: { name: 'Oil Change', systemDefined: true, kind: 'maintenance' },
  },
];

// CategoryCombobox mounts a mutation hook, so every render needs a provider.
function renderForm() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <MaintenanceScheduleForm categories={categories} onSubmit={vi.fn()} />
    </QueryClientProvider>,
  );
}

async function chooseRecurrence(label: RegExp) {
  await userEvent.click(screen.getByRole('combobox', { name: /recurrence type/i }));
  await userEvent.click(await screen.findByRole('option', { name: label }));
}

describe('MaintenanceScheduleForm required markers', () => {
  it('marks category and recurrence type', () => {
    renderForm();

    expect(screen.getByRole('combobox', { name: /category/i })).toHaveAttribute(
      'aria-required',
      'true',
    );
    expect(screen.getByRole('combobox', { name: /recurrence type/i })).toHaveAttribute(
      'aria-required',
      'true',
    );
  });

  // Default is 'time': months is shown and required, miles is not rendered at
  // all — so its absence is asserted as an absent element, not as a control
  // that happens to lack an attribute.
  it('marks the months interval and renders no miles interval for time', () => {
    renderForm();

    expect(screen.getByRole('spinbutton', { name: 'Every (months)' })).toHaveAttribute(
      'aria-required',
      'true',
    );
    expect(screen.queryByRole('spinbutton', { name: 'Every (miles)' })).not.toBeInTheDocument();
  });

  it('swaps to the miles interval for mileage', async () => {
    renderForm();
    await chooseRecurrence(/mileage-based/i);

    expect(screen.getByRole('spinbutton', { name: 'Every (miles)' })).toHaveAttribute(
      'aria-required',
      'true',
    );
    // No stale marker: the months field is gone, not merely unmarked.
    expect(screen.queryByRole('spinbutton', { name: 'Every (months)' })).not.toBeInTheDocument();
  });

  it('marks both intervals for hybrid', async () => {
    renderForm();
    await chooseRecurrence(/hybrid/i);

    expect(screen.getByRole('spinbutton', { name: 'Every (months)' })).toHaveAttribute(
      'aria-required',
      'true',
    );
    expect(screen.getByRole('spinbutton', { name: 'Every (miles)' })).toHaveAttribute(
      'aria-required',
      'true',
    );
  });

  it('renders the required legend', () => {
    renderForm();

    expect(screen.getByText('Required')).toBeInTheDocument();
  });
});
