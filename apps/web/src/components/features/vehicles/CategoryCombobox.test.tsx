import { describe, it, expect, vi, beforeAll, beforeEach } from 'vitest';
import { render, screen, within, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { toast } from 'sonner';
import { CategoryCombobox } from './CategoryCombobox';
import type { MaintenanceCategory } from '../../../types/models/maintenanceCategory';
import { expectNoCall, expectNoCallWith } from '../../../test/expectNoCall';

// cmdk measures its list; jsdom implements neither method.
beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn();
});

// `vi.mock` factories are hoisted above imports, so anything they close over
// must be created through `vi.hoisted` rather than declared as a plain const
// below — otherwise it would be read before initialization.
const { mutateAsync, hookState } = vi.hoisted(() => ({
  mutateAsync: vi.fn(),
  hookState: { isPending: false },
}));

// The create path is this component's entire reason for existing, so it is
// mocked directly rather than left to the real mutation: tests need to
// control what the "server" returns (including an id that could never be
// derived from the typed name) and force the pending/rejected branches on
// demand.
vi.mock('../../../lib/hooks/api/maintenance', () => ({
  useCreateMaintenanceCategory: () => ({
    mutateAsync,
    isPending: hookState.isPending,
  }),
}));

// Spied rather than stubbed out, matching VehiclePhotoThumbnail.test.tsx's
// convention: the rejected-mutation test asserts exactly what was passed to
// toast.error.
vi.mock('sonner', () => ({
  toast: Object.assign(vi.fn(), {
    error: vi.fn(),
    success: vi.fn(),
    warning: vi.fn(),
    info: vi.fn(),
  }),
}));

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

beforeEach(() => {
  mutateAsync.mockReset();
  hookState.isPending = false;
  vi.mocked(toast.error).mockClear();
});

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

    // Asserting the headings exist proves nothing about which items sit
    // under which heading — a swapped `systemDefined` predicate would still
    // render both headings. Scope each assertion to its own `[cmdk-group]`
    // container so membership, not just presence, is checked.
    const suggestedGroup = screen.getByText(/suggested/i).closest('[cmdk-group]');
    const customGroup = screen.getByText(/custom/i).closest('[cmdk-group]');
    expect(suggestedGroup).not.toBeNull();
    expect(customGroup).not.toBeNull();

    expect(within(suggestedGroup as HTMLElement).getByText('Oil Change')).toBeInTheDocument();
    expect(within(customGroup as HTMLElement).getByText('Rear Diff Fluid')).toBeInTheDocument();

    expect(
      within(suggestedGroup as HTMLElement).queryByText('Rear Diff Fluid'),
    ).not.toBeInTheDocument();
    expect(within(customGroup as HTMLElement).queryByText('Oil Change')).not.toBeInTheDocument();
  });

  it('creates a category and selects the id the server returned, not anything derived locally', async () => {
    // The returned id deliberately shares nothing with the typed name or any
    // locally-derivable slug, so the assertion below cannot pass by
    // coincidence if `handleCreate` were changed to select a client-side id.
    mutateAsync.mockResolvedValue({
      id: 'server-assigned-id',
      type: 'maintenanceCategories',
      attributes: { name: 'Skid Plate', systemDefined: false, kind: 'maintenance' },
    });
    const onChange = vi.fn();
    const user = userEvent.setup();
    renderCombobox({ onChange });

    await user.click(screen.getByRole('combobox', { name: /category/i }));
    await user.type(screen.getByPlaceholderText(/search/i), 'Skid Plate');
    await user.click(screen.getByText(/create "Skid Plate"/i));

    expect(mutateAsync).toHaveBeenCalledWith({ name: 'Skid Plate', kind: 'maintenance' });
    await waitFor(() => expect(onChange).toHaveBeenCalledWith('server-assigned-id'));
    await expectNoCallWith(onChange, ['Skid Plate'], 'onChange');
  });

  it('surfaces a toast and selects nothing when creation fails', async () => {
    mutateAsync.mockRejectedValue(new Error('You do not have permission to create categories'));
    const onChange = vi.fn();
    const user = userEvent.setup();
    renderCombobox({ onChange });

    await user.click(screen.getByRole('combobox', { name: /category/i }));
    await user.type(screen.getByPlaceholderText(/search/i), 'Skid Plate');
    await user.click(screen.getByText(/create "Skid Plate"/i));

    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith('You do not have permission to create categories'),
    );
    await expectNoCall(onChange, 'onChange');
  });

  it('disables the create item while the mutation is pending', async () => {
    hookState.isPending = true;
    const user = userEvent.setup();
    renderCombobox();

    await user.click(screen.getByRole('combobox', { name: /category/i }));
    await user.type(screen.getByPlaceholderText(/search/i), 'Skid Plate');

    const createItem = screen.getByText(/create "Skid Plate"/i).closest('[cmdk-item]');
    expect(createItem).toHaveAttribute('aria-disabled', 'true');
  });

  // FormControl injects a11y props through Radix Slot; this component
  // destructures its props rather than spreading them, so each one has to be
  // forwarded to the trigger by hand.
  it('forwards aria-required to the trigger button', () => {
    renderCombobox({ 'aria-required': true });

    expect(screen.getByRole('combobox', { name: 'Category' })).toHaveAttribute(
      'aria-required',
      'true',
    );
  });

  it('leaves the trigger without aria-required when the prop is absent', () => {
    renderCombobox();

    expect(screen.getByRole('combobox', { name: 'Category' })).not.toHaveAttribute('aria-required');
  });
});
