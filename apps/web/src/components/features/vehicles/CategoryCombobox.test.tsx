import { describe, it, expect, vi, beforeAll } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { CategoryCombobox } from './CategoryCombobox';
import type { MaintenanceCategory } from '../../../types/models/maintenanceCategory';

// cmdk measures its list; jsdom implements neither method.
beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn();
});

const categories: MaintenanceCategory[] = [
  {
    id: 'c1',
    type: 'maintenanceCategories',
    attributes: { name: 'Oil Change', systemDefined: true, kind: 'maintenance' },
  },
  {
    id: 'c2',
    type: 'maintenanceCategories',
    attributes: { name: 'Rear Diff Fluid', systemDefined: false, kind: 'maintenance' },
  },
];

function renderCombobox(props: Partial<React.ComponentProps<typeof CategoryCombobox>> = {}) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <CategoryCombobox
        categories={categories}
        kind="maintenance"
        value=""
        onChange={vi.fn()}
        ariaLabel="Category"
        {...props}
      />
    </QueryClientProvider>,
  );
}

describe('CategoryCombobox', () => {
  it('selects an existing category by id', async () => {
    const onChange = vi.fn();
    const user = userEvent.setup();
    renderCombobox({ onChange });

    await user.click(screen.getByRole('combobox', { name: /category/i }));
    await user.click(screen.getByText('Rear Diff Fluid'));

    expect(onChange).toHaveBeenCalledWith('c2');
  });

  it('offers to create a name that does not exist', async () => {
    const user = userEvent.setup();
    renderCombobox();

    await user.click(screen.getByRole('combobox', { name: /category/i }));
    await user.type(screen.getByPlaceholderText(/search/i), 'Skid Plate');

    expect(screen.getByText(/create "Skid Plate"/i)).toBeInTheDocument();
  });

  it('does NOT offer to create a name that already exists, ignoring case', async () => {
    const user = userEvent.setup();
    renderCombobox();

    await user.click(screen.getByRole('combobox', { name: /category/i }));
    await user.type(screen.getByPlaceholderText(/search/i), 'oil CHANGE');

    expect(screen.queryByText(/create "/i)).not.toBeInTheDocument();
    expect(screen.getByText('Oil Change')).toBeInTheDocument();
  });

  it('separates suggested from custom categories', async () => {
    const user = userEvent.setup();
    renderCombobox();

    await user.click(screen.getByRole('combobox', { name: /category/i }));

    expect(screen.getByText(/suggested/i)).toBeInTheDocument();
    expect(screen.getByText(/custom/i)).toBeInTheDocument();
  });
});
