import { describe, it, expect } from 'vitest';
import { vehicleBanner, asVehicleStatus } from './vehicleBanner';

const NOW = new Date('2026-08-02T12:00:00Z');

/** An ISO timestamp `days` before NOW. */
function daysAgo(days: number): string {
  return new Date(NOW.getTime() - days * 24 * 60 * 60 * 1000).toISOString();
}

describe('vehicleBanner — overdue', () => {
  it('states the mileage overrun with a thousands separator', () => {
    expect(
      vehicleBanner(
        { status: 'Overdue', nextDue: { state: 'overdue', axis: 'mileage', miles: 1120 } },
        NOW,
      ),
    ).toEqual({ tone: 'danger', icon: 'overdue', text: 'Service overdue by 1,120 mi' });
  });

  it('states a day count for a time-axis schedule', () => {
    expect(
      vehicleBanner(
        { status: 'Overdue', nextDue: { state: 'overdue', axis: 'time', days: 12 } },
        NOW,
      ).text,
    ).toBe('Service overdue by 12 days');
  });

  it('uses the singular for one day', () => {
    expect(
      vehicleBanner(
        { status: 'Overdue', nextDue: { state: 'overdue', axis: 'time', days: 1 } },
        NOW,
      ).text,
    ).toBe('Service overdue by 1 day');
  });

  it('stays tinted with generic copy when no due detail arrived', () => {
    // Urgency must survive missing detail.
    expect(vehicleBanner({ status: 'Overdue' }, NOW)).toEqual({
      tone: 'danger',
      icon: 'overdue',
      text: 'Maintenance overdue',
    });
  });
});

describe('vehicleBanner — upcoming', () => {
  it('states the remaining distance', () => {
    expect(
      vehicleBanner(
        {
          status: 'Upcoming Maintenance',
          nextDue: { state: 'upcoming', axis: 'mileage', miles: 310 },
        },
        NOW,
      ),
    ).toEqual({ tone: 'warning', icon: 'upcoming', text: 'Service due in 310 mi' });
  });

  it('says "due today" for a zero-day time axis', () => {
    expect(
      vehicleBanner(
        { status: 'Upcoming Maintenance', nextDue: { state: 'upcoming', axis: 'time', days: 0 } },
        NOW,
      ).text,
    ).toBe('Service due today');
  });

  it('stays tinted with generic copy when no due detail arrived', () => {
    expect(vehicleBanner({ status: 'Upcoming Maintenance' }, NOW)).toEqual({
      tone: 'warning',
      icon: 'upcoming',
      text: 'Maintenance due soon',
    });
  });
});

describe('vehicleBanner — axis discipline', () => {
  it('never renders a mileage figure for a time-axis schedule', () => {
    const text = vehicleBanner(
      // A miles value alongside axis 'time' is a server bug; the axis wins.
      { status: 'Overdue', nextDue: { state: 'overdue', axis: 'time', days: 4, miles: 900 } },
      NOW,
    ).text;
    expect(text).toBe('Service overdue by 4 days');
    expect(text).not.toContain('900');
  });

  it('never renders a day count for a mileage-axis schedule', () => {
    const text = vehicleBanner(
      { status: 'Overdue', nextDue: { state: 'overdue', axis: 'mileage', miles: 900, days: 4 } },
      NOW,
    ).text;
    expect(text).toBe('Service overdue by 900 mi');
    expect(text).not.toContain('4 days');
  });

  it('counts days below sixty and months at sixty and above', () => {
    const at = (days: number) =>
      vehicleBanner({ status: 'Overdue', nextDue: { state: 'overdue', axis: 'time', days } }, NOW)
        .text;
    expect(at(59)).toBe('Service overdue by 59 days');
    expect(at(60)).toBe('Service overdue by 2 months');
    expect(at(100)).toBe('Service overdue by 3 months');
  });

  it('falls back to generic tinted copy when the magnitude for the axis is missing', () => {
    // A hand-rolled fixture or a server bug must not render "Service overdue by
    // undefined mi".
    expect(
      vehicleBanner({ status: 'Overdue', nextDue: { state: 'overdue', axis: 'mileage' } }, NOW),
    ).toEqual({ tone: 'danger', icon: 'overdue', text: 'Maintenance overdue' });
    expect(
      vehicleBanner({ status: 'Overdue', nextDue: { state: 'overdue', axis: 'time' } }, NOW).text,
    ).toBe('Maintenance overdue');
  });

  it('falls back when the axis itself is unrecognised', () => {
    const nextDue = { state: 'overdue', axis: 'torque', miles: 5 } as unknown as never;
    expect(vehicleBanner({ status: 'Overdue', nextDue }, NOW).text).toBe('Maintenance overdue');
  });
});

describe('vehicleBanner — quiet statuses', () => {
  it('reads "Up to date" for a healthy vehicle', () => {
    expect(vehicleBanner({ status: 'Healthy' }, NOW)).toEqual({
      tone: 'quiet',
      icon: 'healthy',
      text: 'Up to date',
    });
  });

  it('states the dormancy in whole months for an inactive vehicle', () => {
    expect(vehicleBanner({ status: 'Inactive', lastActivityAt: daysAgo(400) }, NOW)).toEqual({
      tone: 'quiet',
      icon: 'inactive',
      text: 'No activity in 13 months',
    });
  });

  it('never reports fewer than twelve months for an inactive vehicle', () => {
    // The Inactive threshold is 365 days, so anything reaching this branch is at
    // least a year stale. A rounding artefact reading "11 months" would be wrong
    // in a way the user can see.
    expect(vehicleBanner({ status: 'Inactive', lastActivityAt: daysAgo(366) }, NOW).text).toBe(
      'No activity in 12 months',
    );
  });

  it('says "No activity recorded" when the timestamp is missing', () => {
    expect(vehicleBanner({ status: 'Inactive' }, NOW).text).toBe('No activity recorded');
  });

  it('says "No activity recorded" when the timestamp is unparseable', () => {
    expect(vehicleBanner({ status: 'Inactive', lastActivityAt: 'nope' }, NOW).text).toBe(
      'No activity recorded',
    );
  });
});

describe('vehicleBanner — unknown status', () => {
  it('renders quietly when status is absent', () => {
    expect(vehicleBanner({}, NOW)).toEqual({
      tone: 'quiet',
      icon: 'unknown',
      text: 'Status unavailable',
    });
  });

  it('renders quietly for an unrecognised status rather than tinting a raw string', () => {
    expect(vehicleBanner({ status: 'Exploded' }, NOW)).toEqual({
      tone: 'quiet',
      icon: 'unknown',
      text: 'Status unavailable',
    });
  });

  it('ignores due detail attached to an unrecognised status', () => {
    expect(
      vehicleBanner(
        { status: 'Exploded', nextDue: { state: 'overdue', axis: 'mileage', miles: 500 } },
        NOW,
      ).tone,
    ).toBe('quiet');
  });
});

describe('asVehicleStatus', () => {
  it('passes through the four known statuses', () => {
    for (const s of ['Healthy', 'Upcoming Maintenance', 'Overdue', 'Inactive']) {
      expect(asVehicleStatus(s)).toBe(s);
    }
  });

  it('rejects undefined and anything unrecognised', () => {
    expect(asVehicleStatus(undefined)).toBeNull();
    expect(asVehicleStatus('')).toBeNull();
    expect(asVehicleStatus('healthy')).toBeNull();
  });
});
