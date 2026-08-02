import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { VehicleRecordsTable } from './VehicleRecordsTable';
import type { VehicleRecordRow } from '../../../../lib/vehicleRecords';

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
  },
  {
    id: 'modification:3',
    sourceId: '3',
    kind: 'modification',
    date: '2026-05-18T00:00:00Z',
    title: 'Rock sliders',
    mileage: 79430,
    cost: 980,
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
    expect(screen.getByText(/showing 3 of 41/i)).toBeInTheDocument();
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
    expect(onLoadMore).not.toHaveBeenCalled();
  });
});
