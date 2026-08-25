import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { UpcomingScheduleStrip } from './UpcomingScheduleStrip';
import type {
  MaintenanceSchedule,
  MaintenanceScheduleAttributes,
} from '../../../../types/models/maintenanceSchedule';

function schedule(
  id: string,
  overrides: Partial<MaintenanceScheduleAttributes>,
): MaintenanceSchedule {
  return {
    id,
    type: 'maintenanceSchedules',
    attributes: {
      vehicleId: 'v1',
      categoryId: id,
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

const names = new Map([
  ['overdue', 'Overdue Item'],
  ['upcoming', 'Upcoming Item'],
  ['ok', 'Ok Item'],
  ['done-old', 'Old Done Item'],
  ['done-new', 'New Done Item'],
]);

function renderStrip(schedules: MaintenanceSchedule[]) {
  render(
    <UpcomingScheduleStrip
      schedules={schedules}
      categoryNames={names}
      canWrite
      onAddSchedule={vi.fn()}
      onComplete={vi.fn()}
      onConvert={vi.fn()}
    />,
  );
}

/**
 * The rendered category names in DOM order. getAllByText returns matches in
 * document order, which is exactly the ordering under test — asserting on the
 * rendered sequence rather than on the comparator keeps the test honest about
 * what the user sees.
 */
function renderedOrder(): (string | null)[] {
  return screen.getAllByText(/ Item$/).map((node) => node.textContent);
}

describe('UpcomingScheduleStrip ordering', () => {
  it('ranks active schedules overdue, then upcoming, then ok', () => {
    renderStrip([
      schedule('ok', { status: 'ok' }),
      schedule('overdue', { status: 'overdue', severity: 'urgent' }),
      schedule('upcoming', { status: 'upcoming', severity: 'recommended' }),
    ]);
    expect(renderedOrder()).toEqual(['Overdue Item', 'Upcoming Item', 'Ok Item']);
  });

  // A deactivated row's status describes a cycle that is over, so the tier
  // check has to come before rankSchedule rather than inside it — otherwise a
  // completed schedule whose last stored status was 'overdue' would sort to
  // the top of the list.
  it('sorts inactive schedules below every active one regardless of stored status', () => {
    renderStrip([
      schedule('done-old', {
        active: false,
        oneTime: true,
        status: 'overdue',
        lastCompletedDate: '2026-01-01T00:00:00Z',
      }),
      schedule('ok', { status: 'ok' }),
    ]);
    expect(renderedOrder()).toEqual(['Ok Item', 'Old Done Item']);
  });

  it('orders inactive schedules most-recently-completed first', () => {
    renderStrip([
      schedule('done-old', {
        active: false,
        oneTime: true,
        lastCompletedDate: '2026-01-01T00:00:00Z',
      }),
      schedule('done-new', {
        active: false,
        oneTime: true,
        lastCompletedDate: '2026-08-01T00:00:00Z',
      }),
    ]);
    expect(renderedOrder()).toEqual(['New Done Item', 'Old Done Item']);
  });

  it('shows the empty state when there are no schedules', () => {
    renderStrip([]);
    expect(screen.getByText(/no maintenance schedules defined/i)).toBeInTheDocument();
  });
});
