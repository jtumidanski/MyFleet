import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MaintenanceRecordForm } from './MaintenanceRecordForm';
import { mediaService } from '../../../../services/api/MediaService';
import type { MaintenanceCategory } from '../../../../types/models/maintenanceCategory';

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

// jsdom implements neither of these, and Radix's Select trigger calls both
// when opened via pointer events — without the polyfill, opening the Select
// throws "target.hasPointerCapture is not a function" before any listbox
// renders.
Element.prototype.hasPointerCapture = Element.prototype.hasPointerCapture ?? (() => false);
Element.prototype.setPointerCapture = Element.prototype.setPointerCapture ?? (() => {});
Element.prototype.releasePointerCapture = Element.prototype.releasePointerCapture ?? (() => {});
Element.prototype.scrollIntoView = Element.prototype.scrollIntoView ?? (() => {});

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

describe('MaintenanceRecordForm', () => {
  it('offers only the categories of the requested kind', async () => {
    const user = userEvent.setup();
    render(<MaintenanceRecordForm categories={categories} kind="modification" onSubmit={vi.fn()} />);

    // Open the picker and inspect its listbox directly — a Select renders no
    // options until opened, so checking the closed document proves nothing.
    await user.click(screen.getByRole('combobox', { name: /category/i }));
    const listbox = await screen.findByRole('listbox');

    expect(await within(listbox).findByText('Exhaust')).toBeInTheDocument();
    expect(within(listbox).queryByText('Oil Change')).not.toBeInTheDocument();
  });

  it('rejects a description over 200 characters', async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();
    render(<MaintenanceRecordForm categories={categories} onSubmit={onSubmit} />);

    await user.type(screen.getByLabelText(/description/i), 'a'.repeat(201));
    await user.click(screen.getByRole('button', { name: /log record/i }));

    await waitFor(() => expect(screen.getByText(/200 characters or fewer/i)).toBeInTheDocument());
    expect(onSubmit).not.toHaveBeenCalled();
  });

  // A record must never be saved referencing a media object that has not been
  // confirmed (PRD FR-DOC-5).
  it('disables submit while an attachment upload is in flight', async () => {
    vi.mocked(mediaService.initUpload).mockReturnValue(new Promise(() => {}));

    const user = userEvent.setup();
    render(<MaintenanceRecordForm categories={categories} onSubmit={vi.fn()} />);

    const submit = screen.getByRole('button', { name: /log record/i });
    expect(submit).toBeEnabled();

    const input = document.querySelector('input[type="file"]') as HTMLInputElement;
    await user.upload(input, new File(['x'], 'invoice.pdf', { type: 'application/pdf' }));

    await waitFor(() => expect(submit).toBeDisabled());
  });
});
