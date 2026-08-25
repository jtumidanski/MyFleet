import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { UpcomingMaintenanceWidget } from './UpcomingMaintenanceWidget';
import type { MaintenanceSchedule } from '../../../../types/models/maintenanceSchedule';

const mockUseUpcoming = vi.fn();
const mockUseCategoryNameMap = vi.fn();
const mockUseVehicleTitleMap = vi.fn();

vi.mock('../../../../lib/hooks/api/maintenance', () => ({
  useUpcomingMaintenanceQueue: (fleetId: string) => mockUseUpcoming(fleetId),
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
      status: 'upcoming',
      severity: 'recommended',
      active: true,
      ...overrides,
    },
  };
}

function settled(items: MaintenanceSchedule[]) {
  mockUseUpcoming.mockReturnValue({ data: items, isLoading: false });
  mockUseCategoryNameMap.mockReturnValue({
    names: new Map([[CATEGORY_ID, 'Cold Air Intake']]),
    isLoading: false,
  });
  mockUseVehicleTitleMap.mockReturnValue({
    titles: new Map([[VEHICLE_ID, '2021 Honda Civic']]),
    isLoading: false,
  });
}

describe('UpcomingMaintenanceWidget', () => {
  beforeEach(() => {
    mockUseUpcoming.mockReset();
    mockUseCategoryNameMap.mockReset();
    mockUseVehicleTitleMap.mockReset();
  });

  // The fixture name is a modification-kind category on purpose (FR-LABEL-3):
  // the map comes from a no-kind-filter query, so both kinds resolve. The
  // vehicle has no nickname, exercising the year/make/model branch.
  it('renders the category name and the owning vehicle, not ids', () => {
    settled([schedule()]);

    render(<UpcomingMaintenanceWidget fleetId="f1" />);

    expect(screen.getByText('Cold Air Intake')).toBeInTheDocument();
    expect(screen.getByText('2021 Honda Civic')).toBeInTheDocument();
    expect(screen.queryByText(CATEGORY_ID)).not.toBeInTheDocument();
    expect(screen.queryByText(VEHICLE_ID)).not.toBeInTheDocument();
  });

  it('falls back to the placeholders when neither id resolves', () => {
    settled([schedule()]);
    mockUseCategoryNameMap.mockReturnValue({ names: new Map(), isLoading: false });
    mockUseVehicleTitleMap.mockReturnValue({ titles: new Map(), isLoading: false });

    render(<UpcomingMaintenanceWidget fleetId="f1" />);

    expect(screen.getByText('Unknown category')).toBeInTheDocument();
    expect(screen.getByText('Unknown vehicle')).toBeInTheDocument();
  });

  // FR-DUE-2: present tense here; the overdue widget says "Was due".
  it('renders the due date in the present tense', () => {
    const nextDueDate = '2026-09-14T00:00:00Z';
    settled([schedule({ nextDueDate })]);

    render(<UpcomingMaintenanceWidget fleetId="f1" />);

    expect(screen.getByText(`Due ${new Date(nextDueDate).toLocaleDateString()}`)).toBeInTheDocument();
    expect(screen.queryByText(/Was due/)).not.toBeInTheDocument();
  });

  it('renders the due mileage with thousands separators', () => {
    settled([schedule({ recurrenceType: 'mileage', nextDueMileage: 75000 })]);

    render(<UpcomingMaintenanceWidget fleetId="f1" />);

    expect(screen.getByText(`At ${(75000).toLocaleString()} miles`)).toBeInTheDocument();
  });

  it('omits the mileage line for a pure-time schedule and never renders a bare 0', () => {
    settled([schedule({ nextDueDate: '2026-09-14T00:00:00Z', nextDueMileage: 0 })]);

    render(<UpcomingMaintenanceWidget fleetId="f1" />);

    expect(screen.queryByText(/miles/)).not.toBeInTheDocument();
    expect(screen.queryByText('0')).not.toBeInTheDocument();
  });

  it('omits the date line for a pure-mileage schedule', () => {
    settled([schedule({ recurrenceType: 'mileage', nextDueMileage: 75000 })]);

    render(<UpcomingMaintenanceWidget fleetId="f1" />);

    expect(screen.queryByText(/^Due /)).not.toBeInTheDocument();
  });

  it('holds the skeleton while the category query is in flight', () => {
    settled([schedule()]);
    mockUseCategoryNameMap.mockReturnValue({ names: new Map(), isLoading: true });

    const { container } = render(<UpcomingMaintenanceWidget fleetId="f1" />);

    expect(container.querySelector('.animate-pulse')).toBeInTheDocument();
    expect(screen.queryByText(CATEGORY_ID)).not.toBeInTheDocument();
  });

  it('holds the skeleton while the vehicle query is in flight', () => {
    settled([schedule()]);
    mockUseVehicleTitleMap.mockReturnValue({ titles: new Map(), isLoading: true });

    const { container } = render(<UpcomingMaintenanceWidget fleetId="f1" />);

    expect(container.querySelector('.animate-pulse')).toBeInTheDocument();
    expect(screen.queryByText(VEHICLE_ID)).not.toBeInTheDocument();
  });

  it('preserves the empty-state copy', () => {
    settled([]);

    render(<UpcomingMaintenanceWidget fleetId="f1" />);

    expect(screen.getByText('No upcoming maintenance.')).toBeInTheDocument();
  });

  it('caps the list at five rows', () => {
    settled(Array.from({ length: 7 }, (_, i) => schedule({}, `s${i}`)));

    render(<UpcomingMaintenanceWidget fleetId="f1" />);

    expect(screen.getAllByText('Cold Air Intake')).toHaveLength(5);
  });
});
