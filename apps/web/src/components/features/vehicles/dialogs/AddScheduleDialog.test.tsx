import { describe, it, expect, vi, beforeAll, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { AddScheduleDialog } from './AddScheduleDialog';
import { maintenanceScheduleService } from '../../../../services/api/MaintenanceScheduleService';
import { maintenanceCategoryService } from '../../../../services/api/MaintenanceCategoryService';

vi.mock('../../../../services/api/MaintenanceScheduleService', () => ({
  maintenanceScheduleService: { createForVehicle: vi.fn() },
}));

vi.mock('../../../../services/api/MaintenanceCategoryService', () => ({
  maintenanceCategoryService: { list: vi.fn() },
}));

vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

// cmdk scrolls the selected item into view; jsdom does not implement it. Same
// stub MaintenanceScheduleForm.test.tsx installs.
beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn();
});

const categories = [
  {
    id: 'c1',
    type: 'maintenanceCategories',
    attributes: { name: 'Oil Change', systemDefined: true, kind: 'maintenance' as const },
  },
];

function renderDialog() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const onOpenChange = vi.fn();
  render(
    <QueryClientProvider client={client}>
      <AddScheduleDialog
        open
        onOpenChange={onOpenChange}
        vehicleId="v1"
        currentMileage={40000}
      />
    </QueryClientProvider>,
  );
  return { onOpenChange };
}

/** Pick an option from a shadcn/ui Select by its trigger's accessible name. */
async function selectOption(triggerName: RegExp, optionName: RegExp) {
  const user = userEvent.setup();
  await user.click(screen.getByRole('combobox', { name: triggerName }));
  await user.click(await screen.findByRole('option', { name: optionName }));
}

async function chooseCategory() {
  const user = userEvent.setup();
  await user.click(await screen.findByRole('combobox', { name: /category/i }));
  await user.click(await screen.findByRole('option', { name: /oil change/i }));
}

/** The single attributes object handed to the create endpoint. */
async function submittedAttributes() {
  await waitFor(() =>
    expect(maintenanceScheduleService.createForVehicle).toHaveBeenCalledTimes(1),
  );
  const call = vi.mocked(maintenanceScheduleService.createForVehicle).mock.calls[0];
  if (!call) throw new Error('createForVehicle was not called');
  const [vehicleId, attributes] = call;
  return { vehicleId, attributes };
}

beforeEach(() => {
  vi.mocked(maintenanceScheduleService.createForVehicle).mockReset();
  vi.mocked(maintenanceScheduleService.createForVehicle).mockResolvedValue({
    id: 's1',
    type: 'maintenanceSchedules',
    attributes: {
      vehicleId: 'v1',
      categoryId: 'c1',
      recurrenceType: 'time',
      oneTime: false,
      status: 'ok',
      severity: 'informational',
      active: true,
    },
  });
  vi.mocked(maintenanceCategoryService.list).mockReset();
  vi.mocked(maintenanceCategoryService.list).mockResolvedValue({ data: categories });
});

describe('AddScheduleDialog — create payload', () => {
  it('sends a recurring schedule with its intervals and first-due anchor', async () => {
    const user = userEvent.setup();
    renderDialog();

    await chooseCategory();
    await selectOption(/recurrence type/i, /hybrid/i);

    const months = await screen.findByLabelText(/every \(months\)/i);
    await user.clear(months);
    await user.type(months, '6');

    const miles = await screen.findByLabelText(/every \(miles\)/i);
    await user.clear(miles);
    await user.type(miles, '5000');

    const firstDueDate = screen.getByLabelText(/first due date/i);
    await user.clear(firstDueDate);
    await user.type(firstDueDate, '2027-01-15');

    const firstDueMileage = screen.getByLabelText(/first due odometer/i);
    await user.clear(firstDueMileage);
    await user.type(firstDueMileage, '45000');

    await user.click(screen.getByRole('button', { name: /save schedule/i }));

    const { vehicleId, attributes } = await submittedAttributes();
    expect(vehicleId).toBe('v1');
    expect(attributes).toEqual({
      categoryId: 'c1',
      // `kind` is a form-level discriminant only. recurrenceType stays one of
      // time/mileage/hybrid; oneTime is the orthogonal boolean.
      recurrenceType: 'hybrid',
      oneTime: false,
      intervalMonths: 6,
      intervalMiles: 5000,
      // The date input yields YYYY-MM-DD; the API takes RFC3339.
      dueDate: new Date('2027-01-15').toISOString(),
      dueMileage: 45000,
    });
  });

  it('sends a one-time schedule with its due point and no intervals', async () => {
    const user = userEvent.setup();
    renderDialog();

    await chooseCategory();
    await selectOption(/schedule type/i, /one-time/i);
    await selectOption(/recurrence type/i, /hybrid/i);

    const dueDate = await screen.findByLabelText(/^due date$/i);
    await user.clear(dueDate);
    await user.type(dueDate, '2027-01-15');

    const dueMileage = screen.getByLabelText(/^due odometer/i);
    await user.clear(dueMileage);
    await user.type(dueMileage, '60000');

    await user.click(screen.getByRole('button', { name: /save schedule/i }));

    const { vehicleId, attributes } = await submittedAttributes();
    expect(vehicleId).toBe('v1');
    expect(attributes).toEqual({
      categoryId: 'c1',
      recurrenceType: 'hybrid',
      oneTime: true,
      // FR-OT-3: a one-time schedule carries no interval at all, and a value
      // typed before the user switched kinds must not ride along.
      intervalMonths: undefined,
      intervalMiles: undefined,
      dueDate: new Date('2027-01-15').toISOString(),
      dueMileage: 60000,
    });
  });

  // There is deliberately no fourth recurrenceType. `oneTime` carries the
  // "does it repeat" question; recurrenceType carries "which axes".
  it("never sends recurrenceType 'oneTime'", async () => {
    const user = userEvent.setup();
    renderDialog();

    await chooseCategory();
    await selectOption(/schedule type/i, /one-time/i);

    const dueDate = await screen.findByLabelText(/^due date$/i);
    await user.clear(dueDate);
    await user.type(dueDate, '2027-01-15');

    await user.click(screen.getByRole('button', { name: /save schedule/i }));

    const { attributes } = await submittedAttributes();
    expect(attributes.recurrenceType).toBe('time');
    expect(attributes.oneTime).toBe(true);
  });
});
