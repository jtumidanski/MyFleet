import { describe, it, expect, vi, beforeAll } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MaintenanceScheduleForm } from './MaintenanceScheduleForm';
import type { MaintenanceCategory } from '../../../../types/models/maintenanceCategory';

// cmdk scrolls the selected item into view; jsdom does not implement it. Same
// stub MaintenanceRecordForm.test.tsx installs.
beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn();
});

const categories: MaintenanceCategory[] = [
  {
    id: 'c1',
    type: 'maintenanceCategories',
    attributes: { name: 'Oil Change', systemDefined: true, kind: 'maintenance' },
  },
];

// CategoryCombobox mounts a mutation hook, so every render needs a provider.
function renderForm(props: Partial<React.ComponentProps<typeof MaintenanceScheduleForm>> = {}) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const onSubmit = props.onSubmit ?? vi.fn();
  render(
    <QueryClientProvider client={client}>
      <MaintenanceScheduleForm categories={categories} {...props} onSubmit={onSubmit} />
    </QueryClientProvider>,
  );
  return { onSubmit };
}

/** Pick an option from a shadcn/ui Select by its trigger's accessible name. */
async function selectOption(triggerName: RegExp, optionName: RegExp) {
  const user = userEvent.setup();
  await user.click(screen.getByRole('combobox', { name: triggerName }));
  await user.click(await screen.findByRole('option', { name: optionName }));
}

async function chooseRecurrence(label: RegExp) {
  await selectOption(/recurrence type/i, label);
}

describe('MaintenanceScheduleForm required markers', () => {
  it('marks category and recurrence type', () => {
    renderForm();

    expect(screen.getByRole('combobox', { name: /category/i })).toHaveAttribute(
      'aria-required',
      'true',
    );
    expect(screen.getByRole('combobox', { name: /recurrence type/i })).toHaveAttribute(
      'aria-required',
      'true',
    );
  });

  // Default is 'time': months is shown and required, miles is not rendered at
  // all — so its absence is asserted as an absent element, not as a control
  // that happens to lack an attribute.
  it('marks the months interval and renders no miles interval for time', () => {
    renderForm();

    expect(screen.getByRole('spinbutton', { name: 'Every (months)' })).toHaveAttribute(
      'aria-required',
      'true',
    );
    expect(screen.queryByRole('spinbutton', { name: 'Every (miles)' })).not.toBeInTheDocument();
  });

  it('swaps to the miles interval for mileage', async () => {
    renderForm();
    await chooseRecurrence(/mileage-based/i);

    expect(screen.getByRole('spinbutton', { name: 'Every (miles)' })).toHaveAttribute(
      'aria-required',
      'true',
    );
    // No stale marker: the months field is gone, not merely unmarked.
    expect(screen.queryByRole('spinbutton', { name: 'Every (months)' })).not.toBeInTheDocument();
  });

  it('marks both intervals for hybrid', async () => {
    renderForm();
    await chooseRecurrence(/hybrid/i);

    expect(screen.getByRole('spinbutton', { name: 'Every (months)' })).toHaveAttribute(
      'aria-required',
      'true',
    );
    expect(screen.getByRole('spinbutton', { name: 'Every (miles)' })).toHaveAttribute(
      'aria-required',
      'true',
    );
  });

  it('renders the required legend', () => {
    renderForm();

    expect(screen.getByText('Required')).toBeInTheDocument();
  });
});

describe('MaintenanceScheduleForm — kind', () => {
  it('defaults to a repeating schedule and shows the interval field', () => {
    renderForm();
    expect(screen.getByLabelText(/every \(months\)/i)).toBeInTheDocument();
  });

  it('replaces the interval field with a due date when one-time is chosen', async () => {
    renderForm();
    await selectOption(/schedule type/i, /one-time/i);

    await waitFor(() => {
      expect(screen.queryByLabelText(/every \(months\)/i)).not.toBeInTheDocument();
    });
    expect(screen.getByLabelText(/due date/i)).toBeInTheDocument();
  });

  it('shows the due odometer instead of the mileage interval for a one-time mileage schedule', async () => {
    renderForm();
    await selectOption(/schedule type/i, /one-time/i);
    await selectOption(/recurrence type/i, /mileage-based/i);

    await waitFor(() => {
      expect(screen.getByLabelText(/due odometer/i)).toBeInTheDocument();
    });
    expect(screen.queryByLabelText(/every \(miles\)/i)).not.toBeInTheDocument();
  });

  it('keeps both the interval and the first-due field for a repeating schedule', () => {
    renderForm();
    expect(screen.getByLabelText(/every \(months\)/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/first due/i)).toBeInTheDocument();
  });
});

describe('MaintenanceScheduleForm — first-due defaulting', () => {
  it('defaults the first-due odometer to currentMileage + intervalMiles', async () => {
    const user = userEvent.setup();
    renderForm({ currentMileage: 40000 });
    await selectOption(/recurrence type/i, /mileage-based/i);

    const interval = await screen.findByLabelText(/every \(miles\)/i);
    await user.clear(interval);
    await user.type(interval, '5000');

    await waitFor(() => {
      expect(screen.getByLabelText(/first due odometer/i)).toHaveValue(45000);
    });
  });

  it('recomputes the default when the interval changes', async () => {
    const user = userEvent.setup();
    renderForm({ currentMileage: 40000 });
    await selectOption(/recurrence type/i, /mileage-based/i);

    const interval = await screen.findByLabelText(/every \(miles\)/i);
    await user.clear(interval);
    await user.type(interval, '5000');
    await waitFor(() => {
      expect(screen.getByLabelText(/first due odometer/i)).toHaveValue(45000);
    });

    await user.clear(interval);
    await user.type(interval, '10000');
    await waitFor(() => {
      expect(screen.getByLabelText(/first due odometer/i)).toHaveValue(50000);
    });
  });

  // The whole point of the dirty-field guard: a deliberately chosen anchor must
  // survive a later edit to the interval.
  it('does not stomp a first-due odometer the user has edited', async () => {
    const user = userEvent.setup();
    renderForm({ currentMileage: 40000 });
    await selectOption(/recurrence type/i, /mileage-based/i);

    const interval = await screen.findByLabelText(/every \(miles\)/i);
    await user.clear(interval);
    await user.type(interval, '5000');
    await waitFor(() => {
      expect(screen.getByLabelText(/first due odometer/i)).toHaveValue(45000);
    });

    const anchorField = screen.getByLabelText(/first due odometer/i);
    await user.clear(anchorField);
    await user.type(anchorField, '99000');

    await user.clear(interval);
    await user.type(interval, '10000');

    await waitFor(() => {
      expect(screen.getByLabelText(/every \(miles\)/i)).toHaveValue(10000);
    });
    expect(screen.getByLabelText(/first due odometer/i)).toHaveValue(99000);
  });

  // A one-time schedule has no interval to derive an anchor from.
  it('does not default the due odometer for a one-time schedule', async () => {
    renderForm({ currentMileage: 40000 });
    await selectOption(/schedule type/i, /one-time/i);
    await selectOption(/recurrence type/i, /mileage-based/i);

    const due = await screen.findByLabelText(/due odometer/i);
    expect(due).toHaveValue(null);
  });
});

describe('MaintenanceScheduleForm — submission', () => {
  it('submits kind and the due point alongside the interval', async () => {
    const user = userEvent.setup();
    const { onSubmit } = renderForm({ currentMileage: 40000 });

    await user.click(screen.getByRole('combobox', { name: /category/i }));
    await user.click(await screen.findByRole('option', { name: /oil change/i }));

    const months = screen.getByLabelText(/every \(months\)/i);
    await user.clear(months);
    await user.type(months, '6');

    const firstDue = screen.getByLabelText(/first due date/i);
    await user.clear(firstDue);
    await user.type(firstDue, '2027-01-15');

    await user.click(screen.getByRole('button', { name: /save schedule/i }));

    await waitFor(() => expect(onSubmit).toHaveBeenCalled());
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        categoryId: 'c1',
        kind: 'recurring',
        recurrenceType: 'time',
        intervalMonths: 6,
        dueDate: '2027-01-15',
      }),
    );
  });
});
