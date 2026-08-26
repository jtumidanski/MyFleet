import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { OverdueMaintenanceWidget } from './OverdueMaintenanceWidget';
import type { MaintenanceSchedule } from '../../../../types/models/maintenanceSchedule';

const mockUseOverdue = vi.fn();
const mockUseCategoryNameMap = vi.fn();
const mockUseVehicleTitleMap = vi.fn();

vi.mock('../../../../lib/hooks/api/maintenance', () => ({
  useOverdueMaintenanceQueue: (fleetId: string) => mockUseOverdue(fleetId),
}));

vi.mock('../../../../lib/hooks/api/labels', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../../../lib/hooks/api/labels')>();
  return {
    ...actual,
    useCategoryNameMap: () => mockUseCategoryNameMap(),
    useVehicleTitleMap: (fleetId: string | null | undefined) => mockUseVehicleTitleMap(fleetId),
  };
});

const CATEGORY_ID = 'a3f2c1e0-0000-4000-8000-000000000001';
const VEHICLE_ID = 'b4e3d2f1-0000-4000-8000-000000000002';

function schedule(
  overrides: Partial<MaintenanceSchedule['attributes']> = {},
  id = 's1',
): MaintenanceSchedule {
  return {
    id,
    type: 'maintenanceSchedules',
    attributes: {
      vehicleId: VEHICLE_ID,
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

function settled(items: MaintenanceSchedule[]) {
  mockUseOverdue.mockReturnValue({ data: items, isLoading: false });
  mockUseCategoryNameMap.mockReturnValue({
    names: new Map([[CATEGORY_ID, 'Oil Change']]),
    isLoading: false,
  });
  mockUseVehicleTitleMap.mockReturnValue({
    titles: new Map([[VEHICLE_ID, 'Weekend Truck']]),
    isLoading: false,
  });
}

describe('OverdueMaintenanceWidget', () => {
  beforeEach(() => {
    mockUseOverdue.mockReset();
    mockUseCategoryNameMap.mockReset();
    mockUseVehicleTitleMap.mockReset();
  });

  it('renders the category name and the owning vehicle, not ids', () => {
    settled([schedule()]);

    render(<OverdueMaintenanceWidget fleetId="f1" />);

    expect(screen.getByText('Oil Change')).toBeInTheDocument();
    expect(screen.getByText('Weekend Truck')).toBeInTheDocument();
    expect(screen.queryByText(CATEGORY_ID)).not.toBeInTheDocument();
    expect(screen.queryByText(VEHICLE_ID)).not.toBeInTheDocument();
  });

  it('falls back to the placeholders when neither id resolves', () => {
    settled([schedule()]);
    mockUseCategoryNameMap.mockReturnValue({ names: new Map(), isLoading: false });
    mockUseVehicleTitleMap.mockReturnValue({ titles: new Map(), isLoading: false });

    render(<OverdueMaintenanceWidget fleetId="f1" />);

    expect(screen.getByText('Unknown category')).toBeInTheDocument();
    expect(screen.getByText('Unknown vehicle')).toBeInTheDocument();
  });

  // FR-DUE-2: past tense here; the upcoming widget says "Due".
  it('renders the due date in the past tense', () => {
    const nextDueDate = '2026-03-14T00:00:00Z';
    settled([schedule({ nextDueDate })]);

    render(<OverdueMaintenanceWidget fleetId="f1" />);

    const expected = `Was due ${new Date(nextDueDate).toLocaleDateString()}`;
    expect(screen.getByText(expected)).toBeInTheDocument();
  });

  it('renders the due mileage with thousands separators', () => {
    settled([schedule({ recurrenceType: 'mileage', nextDueMileage: 75000 })]);

    render(<OverdueMaintenanceWidget fleetId="f1" />);

    expect(screen.getByText(`At ${(75000).toLocaleString()} miles`)).toBeInTheDocument();
  });

  // FR-DUE-3/4: a pure-time schedule has no mileage line, a pure-mileage
  // schedule no date line, and a zero mileage renders nothing — not a literal 0.
  it('omits the line each schedule kind has no value for', () => {
    settled([schedule({ nextDueDate: '2026-03-14T00:00:00Z', nextDueMileage: 0 })]);

    render(<OverdueMaintenanceWidget fleetId="f1" />);

    expect(screen.queryByText(/miles/)).not.toBeInTheDocument();
    expect(screen.queryByText('0')).not.toBeInTheDocument();
  });

  it('omits the date line for a pure-mileage schedule', () => {
    settled([schedule({ recurrenceType: 'mileage', nextDueMileage: 75000 })]);

    render(<OverdueMaintenanceWidget fleetId="f1" />);

    expect(screen.queryByText(/Was due/)).not.toBeInTheDocument();
  });

  it('renders both lines for a hybrid schedule', () => {
    const nextDueDate = '2026-03-14T00:00:00Z';
    settled([schedule({ recurrenceType: 'hybrid', nextDueDate, nextDueMileage: 75000 })]);

    render(<OverdueMaintenanceWidget fleetId="f1" />);

    expect(
      screen.getByText(`Was due ${new Date(nextDueDate).toLocaleDateString()}`),
    ).toBeInTheDocument();
    expect(screen.getByText(`At ${(75000).toLocaleString()} miles`)).toBeInTheDocument();
  });

  // FR-LOAD-1: the queue landed first; the skeleton holds until the supporting
  // queries do too, so no frame shows a UUID.
  it('holds the skeleton while the category query is in flight', () => {
    settled([schedule()]);
    mockUseCategoryNameMap.mockReturnValue({ names: new Map(), isLoading: true });

    const { container } = render(<OverdueMaintenanceWidget fleetId="f1" />);

    expect(container.querySelector('.animate-pulse')).toBeInTheDocument();
    expect(screen.queryByText(CATEGORY_ID)).not.toBeInTheDocument();
  });

  it('holds the skeleton while the vehicle query is in flight', () => {
    settled([schedule()]);
    mockUseVehicleTitleMap.mockReturnValue({ titles: new Map(), isLoading: true });

    const { container } = render(<OverdueMaintenanceWidget fleetId="f1" />);

    expect(container.querySelector('.animate-pulse')).toBeInTheDocument();
    expect(screen.queryByText(VEHICLE_ID)).not.toBeInTheDocument();
  });

  it('preserves the empty-state copy', () => {
    settled([]);

    render(<OverdueMaintenanceWidget fleetId="f1" />);

    expect(screen.getByText('No overdue maintenance.')).toBeInTheDocument();
  });

  // FR-LOAD-4: the cap is unchanged.
  it('caps the list at five rows', () => {
    settled(Array.from({ length: 7 }, (_, i) => schedule({}, `s${i}`)));

    render(<OverdueMaintenanceWidget fleetId="f1" />);

    expect(screen.getAllByText('Oil Change')).toHaveLength(5);
  });
});
