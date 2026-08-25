import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ScheduleCard } from './ScheduleCard';
import type {
  MaintenanceSchedule,
  MaintenanceScheduleAttributes,
} from '../../../../types/models/maintenanceSchedule';

function schedule(overrides: Partial<MaintenanceScheduleAttributes> = {}): MaintenanceSchedule {
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

function renderCard(
  s: MaintenanceSchedule,
  props: Partial<React.ComponentProps<typeof ScheduleCard>> = {},
) {
  const onComplete = props.onComplete ?? vi.fn();
  const onConvert = props.onConvert ?? vi.fn();
  render(
    <ScheduleCard
      schedule={s}
      categoryName="Oil Change"
      canWrite
      {...props}
      onComplete={onComplete}
      onConvert={onConvert}
    />,
  );
  return { onComplete, onConvert };
}

describe('ScheduleCard — recurring', () => {
  it('shows the interval and no one-time badge', () => {
    renderCard(schedule());
    expect(screen.getByText(/every 6 months/i)).toBeInTheDocument();
    expect(screen.queryByText(/one-time/i)).not.toBeInTheDocument();
  });

  it('offers Complete when the user can write', async () => {
    const user = userEvent.setup();
    const { onComplete } = renderCard(schedule());
    await user.click(screen.getByRole('button', { name: /complete/i }));
    expect(onComplete).toHaveBeenCalledTimes(1);
  });

  it('hides the actions for a read-only user', () => {
    renderCard(schedule(), { canWrite: false });
    expect(screen.queryByRole('button', { name: /complete/i })).not.toBeInTheDocument();
  });
});

describe('ScheduleCard — active one-time', () => {
  it('badges the card and states the due point instead of an interval', () => {
    renderCard(
      schedule({
        oneTime: true,
        intervalMonths: undefined,
        dueDate: '2026-11-30T00:00:00Z',
        recurrenceType: 'time',
      }),
    );
    expect(screen.getByText(/one-time/i)).toBeInTheDocument();
    expect(screen.getByText(/^due /i)).toBeInTheDocument();
    expect(screen.queryByText(/every/i)).not.toBeInTheDocument();
  });

  it('states a mileage due point', () => {
    renderCard(
      schedule({
        oneTime: true,
        intervalMonths: undefined,
        recurrenceType: 'mileage',
        dueMileage: 60000,
      }),
    );
    expect(screen.getByText(/at 60,000 miles/i)).toBeInTheDocument();
  });

  it('still offers Complete', () => {
    renderCard(schedule({ oneTime: true, intervalMonths: undefined, dueMileage: 60000, recurrenceType: 'mileage' }));
    expect(screen.getByRole('button', { name: /complete/i })).toBeInTheDocument();
  });
});

describe('ScheduleCard — completed one-time', () => {
  const completed = schedule({
    oneTime: true,
    intervalMonths: undefined,
    recurrenceType: 'mileage',
    active: false,
    lastCompletedDate: '2026-06-01T00:00:00Z',
    lastCompletedMileage: 60100,
  });

  // FR-COMPLETE-3: a deactivated schedule keeps showing BOTH its completion
  // date and its completion odometer.
  it('shows the completion date and odometer and drops the Complete action', () => {
    renderCard(completed);
    const line = screen.getByText(/^completed /i);
    expect(line).toBeInTheDocument();
    expect(line.textContent).toMatch(/60,100 miles/);
    expect(screen.queryByRole('button', { name: /^complete$/i })).not.toBeInTheDocument();
  });

  it('offers Set up recurrence to a writer', async () => {
    const user = userEvent.setup();
    const { onConvert } = renderCard(completed);
    await user.click(screen.getByRole('button', { name: /set up recurrence/i }));
    expect(onConvert).toHaveBeenCalledTimes(1);
  });

  it('does not offer Set up recurrence to a read-only user', () => {
    renderCard(completed, { canWrite: false });
    expect(screen.queryByRole('button', { name: /set up recurrence/i })).not.toBeInTheDocument();
  });

  // The conversion path exists only for one-time schedules; a deactivated
  // recurring schedule has nothing to convert to.
  it('does not offer Set up recurrence on a deactivated recurring schedule', () => {
    renderCard(schedule({ active: false, lastCompletedDate: '2026-06-01T00:00:00Z' }));
    expect(screen.queryByRole('button', { name: /set up recurrence/i })).not.toBeInTheDocument();
  });
});
