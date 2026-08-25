import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { CompleteScheduleDialog } from './CompleteScheduleDialog';
import { maintenanceScheduleService } from '../../../../services/api/MaintenanceScheduleService';
import type {
  MaintenanceSchedule,
  MaintenanceScheduleAttributes,
} from '../../../../types/models/maintenanceSchedule';

vi.mock('../../../../services/api/MaintenanceScheduleService', () => ({
  maintenanceScheduleService: { complete: vi.fn() },
}));

// The vehicle and mileage queries are incidental to what is under test; stub
// the hooks rather than the transport so the dialog mounts without a network.
vi.mock('../../../../lib/hooks/api/vehicles', () => ({
  useVehicle: () => ({
    data: { id: 'v1', type: 'vehicles', attributes: { currentMileage: 40000 } },
  }),
  // useCompleteMaintenanceSchedule's onSettled invalidates vehicleKeys.detail;
  // the mock needs a real implementation of it, not just useVehicle.
  vehicleKeys: {
    all: ['vehicles'],
    lists: () => ['vehicles', 'list'],
    list: (params: { fleetId: string }) => ['vehicles', 'list', params],
    details: () => ['vehicles', 'detail'],
    detail: (id: string) => ['vehicles', 'detail', id],
  },
}));
vi.mock('../../../../lib/hooks/api/mileage', () => ({
  useMileageRecords: () => ({ data: { rows: [] } }),
  getLatestMileage: () => undefined,
}));

const toastSuccess = vi.fn();
vi.mock('sonner', () => ({
  toast: {
    success: (...args: unknown[]) => toastSuccess(...args),
    error: vi.fn(),
  },
}));

function schedule(overrides: Partial<MaintenanceScheduleAttributes>): MaintenanceSchedule {
  return {
    id: 's1',
    type: 'maintenanceSchedules',
    attributes: {
      vehicleId: 'v1',
      categoryId: 'c1',
      recurrenceType: 'time',
      intervalMonths: 6,
      oneTime: false,
      status: 'ok',
      severity: 'informational',
      active: true,
      ...overrides,
    },
  };
}

async function completeVia(s: MaintenanceSchedule) {
  const user = userEvent.setup();
  const onRequestConvert = vi.fn();
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <CompleteScheduleDialog
        open
        onOpenChange={vi.fn()}
        schedule={s}
        onRequestConvert={onRequestConvert}
      />
    </QueryClientProvider>,
  );
  await user.click(screen.getByRole('button', { name: /mark complete/i }));
  return { onRequestConvert };
}

beforeEach(() => {
  vi.mocked(maintenanceScheduleService.complete).mockReset();
  vi.mocked(maintenanceScheduleService.complete).mockResolvedValue({
    id: 's1',
    type: 'maintenanceCompletions',
    attributes: { maintenanceRecordId: 'r1' },
  });
  toastSuccess.mockReset();
});

describe('CompleteScheduleDialog success toast', () => {
  it('offers a Set up recurrence action after completing a one-time schedule', async () => {
    await completeVia(
      schedule({ oneTime: true, intervalMonths: undefined, dueDate: '2026-11-30T00:00:00Z' }),
    );

    await waitFor(() => expect(toastSuccess).toHaveBeenCalled());
    const [, options] = toastSuccess.mock.calls[0] as [string, { action?: { label: string } }];
    expect(options?.action?.label).toBe('Set up recurrence');
  });

  it('invokes onRequestConvert when the toast action is activated', async () => {
    const { onRequestConvert } = await completeVia(
      schedule({ oneTime: true, intervalMonths: undefined, dueDate: '2026-11-30T00:00:00Z' }),
    );

    await waitFor(() => expect(toastSuccess).toHaveBeenCalled());
    const [, options] = toastSuccess.mock.calls[0] as [
      string,
      { action?: { onClick: () => void } },
    ];
    options?.action?.onClick();
    expect(onRequestConvert).toHaveBeenCalledTimes(1);
  });

  // FR-CONV-5: a recurring completion keeps today's plain success toast.
  it('offers no action after completing a recurring schedule', async () => {
    await completeVia(schedule({}));

    await waitFor(() => expect(toastSuccess).toHaveBeenCalled());
    const [, options] = toastSuccess.mock.calls[0] as [string, { action?: unknown } | undefined];
    expect(options?.action).toBeUndefined();
  });
});
