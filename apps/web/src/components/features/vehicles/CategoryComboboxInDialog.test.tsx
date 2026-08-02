import { describe, it, expect, vi, beforeAll } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { CategoryCombobox } from './CategoryCombobox';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '../../ui/dialog';
import type { MaintenanceCategory } from '../../../types/models/maintenanceCategory';

/**
 * Every caller mounts CategoryCombobox inside a modal Dialog, and that pairing
 * is what broke the mouse: a non-modal Popover portals its content outside
 * DialogContent and registers no layer of its own, so the dialog's modal
 * isolation governs the popover — its wheel events are cancelled by
 * react-remove-scroll and the dialog owns the pointer-event layer beneath it.
 * The keyboard was unaffected, which is exactly how it presented.
 *
 * These tests exercise the combobox in the container it actually ships in.
 */

beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn();
});

const { mutateAsync } = vi.hoisted(() => ({ mutateAsync: vi.fn() }));

vi.mock('../../../lib/hooks/api/maintenance', () => ({
  useCreateMaintenanceCategory: () => ({ mutateAsync, isPending: false }),
}));

vi.mock('sonner', () => ({
  toast: Object.assign(vi.fn(), { error: vi.fn(), success: vi.fn() }),
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

function renderInDialog(onChange = vi.fn()) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <Dialog open onOpenChange={() => {}}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Log Maintenance</DialogTitle>
          </DialogHeader>
          <CategoryCombobox
            categories={categories}
            kind="maintenance"
            value=""
            onChange={onChange}
            ariaLabel="Category"
          />
        </DialogContent>
      </Dialog>
    </QueryClientProvider>,
  );
  return onChange;
}

describe('CategoryCombobox item interactivity', () => {
  /*
   * jsdom loads no stylesheet, so it can never observe that a Tailwind variant
   * matched — which is exactly why the real defect survived a green suite:
   * every enabled item was getting `pointer-events: none` and `opacity: 0.5`
   * in the browser from `data-[disabled]:` matching `data-disabled="false"`.
   *
   * So assert on the two things jsdom CAN see: the attribute cmdk actually
   * renders, and the class string the component asks for. Together they pin
   * the selector/attribute mismatch that caused it.
   */
  it('marks enabled items with data-disabled="false", not an absent attribute', async () => {
    const user = userEvent.setup();
    renderInDialog();

    await user.click(screen.getByRole('combobox', { name: /category/i }));
    const item = screen.getByText('Oil Change').closest('[cmdk-item]');

    // If this ever becomes null, cmdk changed its convention to Radix's and
    // the value-matching variant below must change with it.
    expect(item).toHaveAttribute('data-disabled', 'false');
  });

  it('gates the disabled styling on the VALUE, so enabled items stay clickable', async () => {
    const user = userEvent.setup();
    renderInDialog();

    await user.click(screen.getByRole('combobox', { name: /category/i }));
    const item = screen.getByText('Oil Change').closest('[cmdk-item]');

    // `data-[disabled]:` would match the "false" above and kill pointer events
    // on every item in a real browser.
    expect(item?.className).toContain('data-[disabled=true]:pointer-events-none');
    expect(item?.className).not.toMatch(/data-\[disabled\]:pointer-events-none/);
  });
});

describe('CategoryCombobox inside a modal Dialog', () => {
  it('takes over the top layer while its list is open', async () => {
    const user = userEvent.setup();
    renderInDialog();

    const dialog = screen.getByRole('dialog');
    expect(dialog).not.toHaveAttribute('aria-hidden');

    await user.click(screen.getByRole('combobox', { name: /category/i }));

    // A modal Popover hides everything outside its own content, which is the
    // observable proof that it pushed its own dismissable/scroll layer rather
    // than sitting inertly inside the dialog's. Without that layer the popover
    // is painted over a modal surface it does not own — no wheel scrolling, no
    // pointer selection.
    await waitFor(() => expect(dialog).toHaveAttribute('aria-hidden', 'true'));
  });

  it('releases the dialog again when the list closes', async () => {
    const user = userEvent.setup();
    const onChange = renderInDialog();

    await user.click(screen.getByRole('combobox', { name: /category/i }));
    await user.click(screen.getByText('Rear Diff Fluid'));

    expect(onChange).toHaveBeenCalledWith('c2');
    // The dialog must be interactive again once the picker is dismissed,
    // otherwise the rest of the form is unusable after choosing a category.
    await waitFor(() =>
      expect(screen.getByRole('dialog')).not.toHaveAttribute('aria-hidden', 'true'),
    );
  });

  it('selects a suggestion by pointer', async () => {
    const user = userEvent.setup();
    const onChange = renderInDialog();

    await user.click(screen.getByRole('combobox', { name: /category/i }));
    await user.click(screen.getByText('Oil Change'));

    expect(onChange).toHaveBeenCalledWith('c1');
  });

  it('creates a typed category by pointer', async () => {
    mutateAsync.mockResolvedValue({
      id: 'server-id',
      type: 'maintenanceCategories',
      attributes: { name: 'Skid Plate', systemDefined: false, kind: 'maintenance' },
    });
    const user = userEvent.setup();
    const onChange = renderInDialog();

    await user.click(screen.getByRole('combobox', { name: /category/i }));
    await user.type(screen.getByPlaceholderText(/search/i), 'Skid Plate');
    await user.click(screen.getByText(/create "Skid Plate"/i));

    await waitFor(() => expect(onChange).toHaveBeenCalledWith('server-id'));
  });

  it('still selects with the keyboard', async () => {
    const user = userEvent.setup();
    const onChange = renderInDialog();

    await user.click(screen.getByRole('combobox', { name: /category/i }));
    await user.type(screen.getByPlaceholderText(/search/i), 'Oil');
    await user.keyboard('{Enter}');

    await waitFor(() => expect(onChange).toHaveBeenCalledWith('c1'));
  });
});
