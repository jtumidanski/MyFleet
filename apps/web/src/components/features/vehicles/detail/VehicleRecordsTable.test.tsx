import { describe, it, expect, vi } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { VehicleRecordsTable } from './VehicleRecordsTable';
import type { VehicleRecordRow } from '../../../../lib/vehicleRecords';
import { useMediaObject } from '../../../../lib/hooks/api/media';
import { expectNoCall } from '../../../../test/expectNoCall';

// Partial mock (importOriginal spread) rather than a bare factory: media.ts
// exports ~18 symbols and a factory replacing the module wholesale would break
// the moment anything here transitively imports one of the others. Only
// useMediaObject is spied, which is the one this table must never call — see
// RecordAttachmentList.tsx:84-87 on the 25 x N metadata fan-out this guards.
vi.mock('../../../../lib/hooks/api/media', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../../../lib/hooks/api/media')>()),
  useMediaObject: vi.fn(),
}));

const rows: VehicleRecordRow[] = [
  {
    id: 'fuel:1',
    sourceId: '1',
    kind: 'fuel',
    date: '2026-07-28T00:00:00Z',
    title: '16.204 gal',
    mileage: 84010,
    cost: 58.31,
  },
  {
    id: 'maintenance:2',
    sourceId: '2',
    kind: 'maintenance',
    date: '2026-07-12T00:00:00Z',
    title: 'Front brake pads',
    mileage: 82940,
    cost: 612.4,
    documentCount: 3,
  },
  {
    id: 'maintenance:4',
    sourceId: '4',
    kind: 'maintenance',
    date: '2026-06-02T00:00:00Z',
    title: 'Cabin air filter',
    mileage: 81200,
    cost: 34.99,
    documentCount: 0,
  },
  {
    id: 'modification:3',
    sourceId: '3',
    kind: 'modification',
    date: '2026-05-18T00:00:00Z',
    title: 'Rock sliders',
    mileage: 79430,
    cost: 980,
    documentCount: 1,
  },
];

function renderTable(props: Partial<React.ComponentProps<typeof VehicleRecordsTable>> = {}) {
  return render(
    <VehicleRecordsTable
      rows={rows}
      total={41}
      hasMore
      isLoading={false}
      isFetchingNextPage={false}
      onLoadMore={vi.fn()}
      onSelectRow={vi.fn()}
      {...props}
    />,
  );
}

describe('VehicleRecordsTable', () => {
  it('shows every row under the All filter', () => {
    renderTable();
    expect(screen.getByText('16.204 gal')).toBeInTheDocument();
    expect(screen.getByText('Front brake pads')).toBeInTheDocument();
    expect(screen.getByText('Rock sliders')).toBeInTheDocument();
  });

  it('narrows to one kind when a chip is pressed', async () => {
    const user = userEvent.setup();
    renderTable();

    await user.click(screen.getByRole('button', { name: /^fuel$/i }));

    expect(screen.getByText('16.204 gal')).toBeInTheDocument();
    expect(screen.queryByText('Front brake pads')).not.toBeInTheDocument();
  });

  it('keeps maintenance and mods on separate chips', async () => {
    const user = userEvent.setup();
    renderTable();

    await user.click(screen.getByRole('button', { name: /^mods$/i }));

    expect(screen.getByText('Rock sliders')).toBeInTheDocument();
    expect(screen.queryByText('Front brake pads')).not.toBeInTheDocument();
  });

  it('reports how many of the total are shown', () => {
    renderTable();
    expect(screen.getByText(/showing 4 of 41/i)).toBeInTheDocument();
  });

  it('passes the clicked row to onSelectRow', async () => {
    const onSelectRow = vi.fn();
    const user = userEvent.setup();
    renderTable({ onSelectRow });

    await user.click(screen.getByText('Front brake pads'));

    expect(onSelectRow).toHaveBeenCalledWith(expect.objectContaining({ id: 'maintenance:2' }));
  });

  it('hides load more when there is nothing left', () => {
    renderTable({ hasMore: false });
    expect(screen.queryByRole('button', { name: /load more/i })).not.toBeInTheDocument();
  });

  // React Query cancels and reissues an in-flight fetchNextPage on every call,
  // so an enabled button lets sustained clicking starve the fetch outright.
  it('disables load more while a page is in flight', async () => {
    const onLoadMore = vi.fn();
    const user = userEvent.setup();
    renderTable({ isFetchingNextPage: true, onLoadMore });

    const button = screen.getByRole('button', { name: /load more/i });
    expect(button).toBeDisabled();

    await user.click(button);
    await expectNoCall(onLoadMore, 'onLoadMore');
  });
});

describe('VehicleRecordsTable attachment indicator', () => {
  /** The <tr> containing a row's Item cell text. */
  function rowFor(title: string): HTMLElement {
    const tr = screen.getByText(title).closest('tr');
    if (!tr) throw new Error(`no <tr> found for row titled "${title}"`);
    return tr;
  }

  it('announces the attachment count on a record that has documents', () => {
    renderTable();
    expect(within(rowFor('Front brake pads')).getByText('3 attachments')).toBeInTheDocument();
  });

  it('uses the singular form for a single attachment', () => {
    renderTable();
    expect(within(rowFor('Rock sliders')).getByText('1 attachment')).toBeInTheDocument();
  });

  it('renders nothing for a record with zero attachments', () => {
    renderTable();
    const row = rowFor('Cabin air filter');
    expect(within(row).queryByText(/attachment/i)).not.toBeInTheDocument();
    // Not even a bare "0" — absence of the icon is the negative signal
    // (PRD section 2, Non-goals).
    expect(within(row).queryByText('0')).not.toBeInTheDocument();
  });

  it('renders nothing for fuel and mileage rows, whose count is unset', () => {
    renderTable({
      rows: [
        ...rows,
        {
          id: 'mileage:5',
          sourceId: '5',
          kind: 'mileage',
          date: '2026-04-01T00:00:00Z',
          title: 'Odometer reading',
          mileage: 78000,
        },
      ],
    });
    expect(within(rowFor('16.204 gal')).queryByText(/attachment/i)).not.toBeInTheDocument();
    expect(within(rowFor('Odometer reading')).queryByText(/attachment/i)).not.toBeInTheDocument();
  });

  // FR-UI-4: the indicator must not become a button or add a tab stop. The row
  // itself is the only click target; clicking the indicator must bubble to it.
  it('is not interactive and lets clicks fall through to the row', async () => {
    const onSelectRow = vi.fn();
    const user = userEvent.setup();
    renderTable({ onSelectRow });

    const row = rowFor('Front brake pads');
    const label = within(row).getByText('3 attachments');

    // queryAllByRole, not getAllByRole — the get* variant throws on zero
    // matches, which is exactly the passing case here.
    expect(within(row).queryAllByRole('button')).toHaveLength(0);
    expect(row).toHaveAttribute('role', 'button');
    expect(label.closest('button')).toBeNull();

    await user.click(label);
    expect(onSelectRow).toHaveBeenCalledWith(expect.objectContaining({ id: 'maintenance:2' }));
  });

  // The whole point of storing a count rather than the ids: rendering a page of
  // rows with attachments must issue zero media-metadata requests.
  it('fetches no media metadata when rows with attachments render', async () => {
    renderTable();
    await expectNoCall(vi.mocked(useMediaObject), 'useMediaObject');
  });

  it('spans every column in the skeleton state', () => {
    const { container } = renderTable({ isLoading: true });
    const headerCount = container.querySelectorAll('thead th').length;
    expect(headerCount).toBe(6);
    const spanned = container.querySelectorAll('tbody td[colspan]');
    expect(spanned.length).toBeGreaterThan(0);
    spanned.forEach((cell) => {
      expect(cell.getAttribute('colspan')).toBe(String(headerCount));
    });
  });

  it('spans every column in the empty state', () => {
    const { container } = renderTable({ rows: [] });
    const headerCount = container.querySelectorAll('thead th').length;
    const spanned = container.querySelectorAll('tbody td[colspan]');
    expect(spanned.length).toBeGreaterThan(0);
    spanned.forEach((cell) => {
      expect(cell.getAttribute('colspan')).toBe(String(headerCount));
    });
  });

  it('names the indicator column for assistive technology', () => {
    renderTable();
    expect(screen.getByText('Attachments')).toBeInTheDocument();
  });
});
