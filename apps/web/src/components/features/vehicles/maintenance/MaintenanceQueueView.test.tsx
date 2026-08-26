import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MaintenanceQueueView } from './MaintenanceQueueView';
import type { MaintenanceSchedule } from '../../../../types/models/maintenanceSchedule';

const mockUseUpcoming = vi.fn();
const mockUseOverdue = vi.fn();
const mockUseCategoryNameMap = vi.fn();

vi.mock('../../../../lib/hooks/api/maintenance', () => ({
  useUpcomingMaintenanceQueue: (fleetId: string) => mockUseUpcoming(fleetId),
  useOverdueMaintenanceQueue: (fleetId: string) => mockUseOverdue(fleetId),
}));

vi.mock('../../../../lib/hooks/api/labels', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../../../lib/hooks/api/labels')>();
  return { ...actual, useCategoryNameMap: () => mockUseCategoryNameMap() };
});

const CATEGORY_ID = 'a3f2c1e0-0000-4000-8000-000000000001';

function schedule(overrides: Partial<MaintenanceSchedule['attributes']> = {}): MaintenanceSchedule {
  return {
    id: 's1',
    type: 'maintenanceSchedules',
    attributes: {
      vehicleId: 'v1',
      categoryId: CATEGORY_ID,
      recurrenceType: 'time',
      oneTime: false,
      status: 'overdue',
      severity: 'urgent',
      active: true,
      ...overrides,
    },
  };
}

/** Every query settled, both queues empty, names resolvable. */
function settled(names: Map<string, string> = new Map([[CATEGORY_ID, 'Oil Change']])) {
  mockUseUpcoming.mockReturnValue({ data: [], isLoading: false });
  mockUseOverdue.mockReturnValue({ data: [], isLoading: false });
  mockUseCategoryNameMap.mockReturnValue({ names, isLoading: false });
}

describe('MaintenanceQueueView', () => {
  beforeEach(() => {
    mockUseUpcoming.mockReset();
    mockUseOverdue.mockReset();
    mockUseCategoryNameMap.mockReset();
  });

  it('renders the category name, not the id, in the overdue card', () => {
    settled();
    mockUseOverdue.mockReturnValue({ data: [schedule()], isLoading: false });

    render(<MaintenanceQueueView fleetId="f1" />);

    expect(screen.getByText('Oil Change')).toBeInTheDocument();
    expect(screen.queryByText(CATEGORY_ID)).not.toBeInTheDocument();
  });

  it('renders the category name, not the id, in the upcoming card', () => {
    settled();
    mockUseUpcoming.mockReturnValue({
      data: [schedule({ status: 'upcoming', severity: 'recommended' })],
      isLoading: false,
    });

    render(<MaintenanceQueueView fleetId="f1" />);

    expect(screen.getByText('Oil Change')).toBeInTheDocument();
    expect(screen.queryByText(CATEGORY_ID)).not.toBeInTheDocument();
  });

  // FR-LABEL-3: the hook asks for no kind filter, so a modification-kind
  // category resolves just like a maintenance one.
  it('resolves a modification-kind category', () => {
    settled(new Map([[CATEGORY_ID, 'Cold Air Intake']]));
    mockUseOverdue.mockReturnValue({ data: [schedule()], isLoading: false });

    render(<MaintenanceQueueView fleetId="f1" />);

    expect(screen.getByText('Cold Air Intake')).toBeInTheDocument();
  });

  // FR-LOAD-3: the category query failed (settled, no data) but the queue
  // succeeded. Rows still render, with the placeholder.
  it('falls back to Unknown category when the id does not resolve', () => {
    settled(new Map());
    mockUseOverdue.mockReturnValue({ data: [schedule()], isLoading: false });

    render(<MaintenanceQueueView fleetId="f1" />);

    expect(screen.getByText('Unknown category')).toBeInTheDocument();
    expect(screen.queryByText(CATEGORY_ID)).not.toBeInTheDocument();
  });

  // FR-LOAD-1: the queue has landed but the names have not. The skeleton holds;
  // no frame shows a UUID.
  it('holds the skeleton while the category query is in flight', () => {
    settled();
    mockUseOverdue.mockReturnValue({ data: [schedule()], isLoading: false });
    mockUseCategoryNameMap.mockReturnValue({ names: new Map(), isLoading: true });

    const { container } = render(<MaintenanceQueueView fleetId="f1" />);

    expect(container.querySelector('.animate-pulse')).toBeInTheDocument();
    expect(screen.queryByText(CATEGORY_ID)).not.toBeInTheDocument();
  });

  it('preserves the empty-state copy for both cards', () => {
    settled();

    render(<MaintenanceQueueView fleetId="f1" />);

    expect(screen.getByText('No overdue maintenance items.')).toBeInTheDocument();
    expect(screen.getByText('No upcoming maintenance items.')).toBeInTheDocument();
  });
});
