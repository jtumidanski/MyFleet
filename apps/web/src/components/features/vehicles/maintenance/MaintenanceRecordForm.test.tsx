import { describe, it, expect, vi, beforeAll } from 'vitest';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MaintenanceRecordForm } from './MaintenanceRecordForm';
import { mediaService } from '../../../../services/api/MediaService';
import type { MaintenanceCategory } from '../../../../types/models/maintenanceCategory';
import { expectNoCall } from '../../../../test/expectNoCall';

// Mocked the same way usePendingAttachments.test.ts does, so the third test can
// make initUpload hang forever and observe the submit button staying disabled.
vi.mock('../../../../services/api/MediaService', () => ({
  mediaService: {
    initUpload: vi.fn(),
    putContent: vi.fn(),
    confirm: vi.fn(),
    remove: vi.fn(),
  },
}));

// cmdk scrolls the selected item into view on keyboard navigation and mount;
// jsdom does not implement scrollIntoView. (Its list-height measurement uses
// ResizeObserver instead, which is already stubbed in the shared test setup.)
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
    attributes: { name: 'Exhaust', systemDefined: true, kind: 'modification' },
  },
];

// CategoryCombobox renders a React Query mutation hook, so every render needs
// a provider — same pattern as CategoryCombobox.test.tsx.
function renderForm(ui: React.ReactElement) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={client}>{ui}</QueryClientProvider>);
}

describe('MaintenanceRecordForm', () => {
  // The combobox is not a native input, so FormControl's generated id has to
  // reach the trigger button for FormLabel's htmlFor to point at anything.
  // Asserting on the ids rather than getByLabelText, because the combobox
  // carries its own aria-label and would satisfy that query even unassociated.
  it('associates the category label with the combobox trigger', () => {
    renderForm(
      <MaintenanceRecordForm categories={categories} kind="maintenance" onSubmit={vi.fn()} />,
    );

    const trigger = screen.getByRole('combobox', { name: /category/i });
    const label = screen.getByText('Category');

    expect(trigger.id).toBeTruthy();
    expect(label).toHaveAttribute('for', trigger.id);
  });

  it('offers only the categories of the requested kind', async () => {
    const user = userEvent.setup();
    renderForm(
      <MaintenanceRecordForm categories={categories} kind="modification" onSubmit={vi.fn()} />,
    );

    // Open the picker and inspect its listbox directly — a Command renders no
    // items until opened, so checking the closed document proves nothing, and
    // scoping to the listbox guards against a name leaking outside the popover.
    await user.click(screen.getByRole('combobox', { name: /category/i }));
    const listbox = await screen.findByRole('listbox');

    // The modification category is offered; the maintenance one is not.
    expect(await within(listbox).findByText('Exhaust')).toBeInTheDocument();
    expect(within(listbox).queryByText('Oil Change')).not.toBeInTheDocument();
  });

  it('rejects a description over 200 characters', async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();
    renderForm(
      <MaintenanceRecordForm categories={categories} kind="maintenance" onSubmit={onSubmit} />,
    );

    await user.type(screen.getByLabelText(/description/i), 'a'.repeat(201));
    await user.click(screen.getByRole('button', { name: /log record/i }));

    await waitFor(() => expect(screen.getByText(/200 characters or fewer/i)).toBeInTheDocument());
    await expectNoCall(onSubmit, 'onSubmit');
  });

  // A record must never be saved referencing a media object that has not been
  // confirmed (PRD FR-DOC-5).
  it('disables submit while an attachment upload is in flight', async () => {
    vi.mocked(mediaService.initUpload).mockReturnValue(new Promise(() => {}));

    const user = userEvent.setup();
    renderForm(
      <MaintenanceRecordForm categories={categories} kind="maintenance" onSubmit={vi.fn()} />,
    );

    const submit = screen.getByRole('button', { name: /log record/i });
    expect(submit).toBeEnabled();

    const input = document.querySelector('input[type="file"]') as HTMLInputElement;
    await user.upload(input, new File(['x'], 'invoice.pdf', { type: 'application/pdf' }));

    await waitFor(() => expect(submit).toBeDisabled());
  });
});
