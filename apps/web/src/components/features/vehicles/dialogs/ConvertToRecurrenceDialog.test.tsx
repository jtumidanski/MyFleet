import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ConvertToRecurrenceDialog } from './ConvertToRecurrenceDialog';
import { maintenanceScheduleService } from '../../../../services/api/MaintenanceScheduleService';
import type { MaintenanceSchedule } from '../../../../types/models/maintenanceSchedule';

vi.mock('../../../../services/api/MaintenanceScheduleService', () => ({
  maintenanceScheduleService: { patch: vi.fn() },
}));

const toastError = vi.fn();
vi.mock('sonner', () => ({
  toast: {
    success: vi.fn(),
    error: (...args: unknown[]) => toastError(...args),
  },
}));

const completedOneTime: MaintenanceSchedule = {
  id: 's1',
  type: 'maintenanceSchedules',
  attributes: {
    vehicleId: 'v1',
    categoryId: 'c1',
    recurrenceType: 'time',
    oneTime: true,
    status: 'ok',
    severity: 'informational',
    active: false,
    lastCompletedDate: '2026-06-01T00:00:00Z',
    lastCompletedMileage: 42000,
  },
};

function renderDialog(onOpenChange = vi.fn()) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <ConvertToRecurrenceDialog
        open
        onOpenChange={onOpenChange}
        schedule={completedOneTime}
        categoryName="Oil Change"
      />
    </QueryClientProvider>,
  );
  return { onOpenChange };
}

beforeEach(() => {
  vi.mocked(maintenanceScheduleService.patch).mockReset();
  toastError.mockReset();
});

describe('ConvertToRecurrenceDialog', () => {
  it('shows the category read-only and states the completion anchor', () => {
    renderDialog();
    expect(screen.getByText('Oil Change')).toBeInTheDocument();
    // The anchor the recurrence will run from — 42,000 miles on the recorded date.
    expect(screen.getByText(/42,000/)).toBeInTheDocument();
    // No category input: the category is fixed.
    expect(screen.queryByRole('combobox', { name: /category/i })).not.toBeInTheDocument();
  });

  it('issues exactly one PATCH clearing the anchor and reactivating the schedule', async () => {
    const user = userEvent.setup();
    vi.mocked(maintenanceScheduleService.patch).mockResolvedValue({
      ...completedOneTime,
      attributes: { ...completedOneTime.attributes, oneTime: false, active: true },
    });
    const { onOpenChange } = renderDialog();

    const months = screen.getByLabelText(/every \(months\)/i);
    await user.clear(months);
    await user.type(months, '12');
    await user.click(screen.getByRole('button', { name: /set up recurrence/i }));

    await waitFor(() => expect(maintenanceScheduleService.patch).toHaveBeenCalledTimes(1));
    expect(maintenanceScheduleService.patch).toHaveBeenCalledWith('s1', {
      oneTime: false,
      recurrenceType: 'time',
      intervalMonths: 12,
      intervalMiles: 0,
      active: true,
      // An explicit null, not an omitted key: the server tells them apart, and
      // omitting it would leave the schedule pinned to its old due date.
      dueDate: null,
      dueMileage: 0,
    });
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
  });

  it('keeps the dialog open and shows an error toast when the PATCH fails', async () => {
    const user = userEvent.setup();
    vi.mocked(maintenanceScheduleService.patch).mockRejectedValue(new Error('nope'));
    const { onOpenChange } = renderDialog();

    const months = screen.getByLabelText(/every \(months\)/i);
    await user.clear(months);
    await user.type(months, '12');
    await user.click(screen.getByRole('button', { name: /set up recurrence/i }));

    await waitFor(() => expect(toastError).toHaveBeenCalled());
    expect(onOpenChange).not.toHaveBeenCalledWith(false);
  });

  it('does not submit without the interval its recurrence type needs', async () => {
    const user = userEvent.setup();
    renderDialog();
    await user.click(screen.getByRole('button', { name: /set up recurrence/i }));
    await waitFor(() => expect(screen.getByText(/interval months is required/i)).toBeInTheDocument());
    expect(maintenanceScheduleService.patch).not.toHaveBeenCalled();
  });
});
