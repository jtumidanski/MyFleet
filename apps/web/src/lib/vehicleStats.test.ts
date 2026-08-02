import { describe, it, expect } from 'vitest';
import {
  deriveOdometer,
  deriveTrailingCost,
  deriveAvgEconomy,
  deriveNextService,
  rankSchedule,
} from './vehicleStats';
import type { VehicleRecordRow } from './vehicleRecords';
import type { MaintenanceSchedule } from '../types/models/maintenanceSchedule';

function mileageRow(date: string, mileage: number): VehicleRecordRow {
  return { id: `mileage:${date}`, sourceId: date, kind: 'mileage', date, title: 'Odometer', mileage };
}
function fuelRow(date: string, mileage: number, cost: number, gallons: number): VehicleRecordRow {
  return {
    id: `fuel:${date}`, sourceId: date, kind: 'fuel', date,
    title: `${gallons} gal`, mileage, cost,
  };
}
function schedule(
  id: string,
  status: string,
  overrides: Partial<MaintenanceSchedule['attributes']> = {},
): MaintenanceSchedule {
  return {
    id,
    type: 'maintenance-schedules',
    attributes: {
      vehicleId: 'v1',
      categoryId: 'oil-change',
      recurrenceType: 'mileage',
      status,
      severity: 'informational',
      active: true,
      ...overrides,
    },
  };
}

describe('deriveOdometer', () => {
  it('prefers the newest mileage row', () => {
    expect(deriveOdometer([mileageRow('2026-01-01', 80000), mileageRow('2026-06-01', 84210)], 1))
      .toBe(84210);
  });

  it('prefers the newest mileage row regardless of array order', () => {
    expect(deriveOdometer([mileageRow('2026-06-01', 84210), mileageRow('2026-01-01', 80000)], 1))
      .toBe(84210);
  });

  it('falls back to the vehicle current mileage when there are no rows', () => {
    expect(deriveOdometer([], 84210)).toBe(84210);
  });

  it('returns undefined when there is nothing at all', () => {
    expect(deriveOdometer([], undefined)).toBeUndefined();
  });
});

describe('deriveTrailingCost', () => {
  const now = new Date('2026-08-02T00:00:00Z');

  it('sums costs inside the trailing twelve months', () => {
    const rows = [
      fuelRow('2026-07-01T00:00:00Z', 84000, 58.31, 16),
      { ...mileageRow('2026-06-01T00:00:00Z', 83000), cost: undefined },
      { id: 'maintenance:a', sourceId: 'a', kind: 'maintenance' as const,
        date: '2026-05-01T00:00:00Z', title: 'Brakes', cost: 612.4 },
    ];
    expect(deriveTrailingCost(rows, now)).toBeCloseTo(670.71, 2);
  });

  it('excludes rows older than twelve months', () => {
    const rows = [
      { id: 'maintenance:old', sourceId: 'old', kind: 'maintenance' as const,
        date: '2025-01-01T00:00:00Z', title: 'Old', cost: 1000 },
    ];
    expect(deriveTrailingCost(rows, now)).toBe(0);
  });

  it('includes a row exactly at the twelve-month cutoff', () => {
    const rows = [
      { id: 'maintenance:edge', sourceId: 'edge', kind: 'maintenance' as const,
        date: '2025-08-02T00:00:00Z', title: 'Edge', cost: 100 },
    ];
    expect(deriveTrailingCost(rows, now)).toBe(100);
  });

  it('returns 0 for no rows', () => {
    expect(deriveTrailingCost([], now)).toBe(0);
  });
});

describe('deriveAvgEconomy', () => {
  it('is undefined with fewer than two fill-ups', () => {
    expect(deriveAvgEconomy([])).toBeUndefined();
    expect(deriveAvgEconomy([fuelRow('2026-07-01', 84000, 58.31, 16)])).toBeUndefined();
  });

  it('is undefined when the odometer did not advance', () => {
    const rows = [
      fuelRow('2026-07-15', 84000, 58.31, 16),
      fuelRow('2026-07-01', 84000, 55.02, 16),
    ];
    expect(deriveAvgEconomy(rows)).toBeUndefined();
  });

  it('averages miles per gallon across consecutive fill-ups', () => {
    const rows = [
      { ...fuelRow('2026-07-15T00:00:00Z', 84300, 58.31, 16), gallons: 16 },
      { ...fuelRow('2026-07-01T00:00:00Z', 84000, 55.02, 15), gallons: 15 },
    ];
    // 300 miles on the 16 gallons that filled the tank after the first reading.
    expect(deriveAvgEconomy(rows)).toBeCloseTo(18.75, 2);
  });

  it('attributes gallons to the fill-up that consumed them, not the one before it', () => {
    // Guards the exact off-by-one in the semantics: the 40-gallon tank pays
    // for the 100 miles since the previous (10-gallon) fill-up, not for
    // whatever came before that first reading.
    const rows = [
      { ...fuelRow('2026-07-01T00:00:00Z', 84000, 30, 10), gallons: 10 },
      { ...fuelRow('2026-07-10T00:00:00Z', 84100, 120, 40), gallons: 40 },
    ];
    // 100 miles / 40 gallons = 2.5 mpg. Using gallons[i-1] (10) would give 10 mpg.
    expect(deriveAvgEconomy(rows)).toBeCloseTo(2.5, 2);
  });

  it('skips a non-advancing pair but still uses later advancing pairs', () => {
    const rows = [
      { ...fuelRow('2026-07-01T00:00:00Z', 84000, 30, 10), gallons: 10 },
      { ...fuelRow('2026-07-05T00:00:00Z', 84000, 20, 8), gallons: 8 }, // same mileage: bad data
      { ...fuelRow('2026-07-15T00:00:00Z', 84200, 60, 20), gallons: 20 },
    ];
    // Only the second pair (84000 -> 84200, 20 gallons) counts: 200 / 20 = 10 mpg.
    // If the non-advancing pair were treated as 0 miles / 8 gallons instead of
    // skipped, gallons would still total 28 and change the result.
    expect(deriveAvgEconomy(rows)).toBeCloseTo(10, 2);
  });

  it('ignores rows missing gallons or mileage', () => {
    const rows = [
      fuelRow('2026-07-01T00:00:00Z', 84000, 30, 10), // no gallons field set
      { ...fuelRow('2026-07-15T00:00:00Z', 84300, 58.31, 16), gallons: 16 },
    ];
    expect(deriveAvgEconomy(rows)).toBeUndefined();
  });
});

describe('rankSchedule', () => {
  it('ranks overdue before upcoming before everything else', () => {
    expect(rankSchedule(schedule('a', 'overdue'))).toBeLessThan(rankSchedule(schedule('b', 'upcoming')));
    expect(rankSchedule(schedule('b', 'upcoming'))).toBeLessThan(rankSchedule(schedule('c', 'ok')));
  });

  it('ranks by status, not severity: an urgent-but-ok schedule ranks last', () => {
    const urgentButOk = schedule('a', 'ok', { severity: 'urgent' });
    const recommendedButOverdue = schedule('b', 'overdue', { severity: 'recommended' });
    expect(rankSchedule(recommendedButOverdue)).toBeLessThan(rankSchedule(urgentButOk));
  });
});

describe('deriveNextService', () => {
  const now = new Date('2026-08-02T00:00:00Z');

  it('returns undefined when there are no schedules', () => {
    expect(deriveNextService([], 84000, now)).toBeUndefined();
  });

  it('picks the overdue schedule over an upcoming one', () => {
    const schedules = [
      schedule('upcoming-1', 'upcoming', { nextDueMileage: 85000 }),
      schedule('overdue-1', 'overdue', { nextDueMileage: 83000 }),
    ];
    const result = deriveNextService(schedules, 84000, now);
    expect(result?.severity).toBe('danger');
    expect(result?.label).toBe('1,000 mi over');
  });

  it('derives severity from status, not from severity: an urgent-but-ok schedule renders ok', () => {
    const schedules = [schedule('a', 'ok', { severity: 'urgent', nextDueMileage: 90000 })];
    const result = deriveNextService(schedules, 84000, now);
    expect(result?.severity).toBe('ok');
  });

  it('derives severity from status: an informational-but-overdue schedule renders danger', () => {
    const schedules = [schedule('a', 'overdue', { severity: 'informational', nextDueMileage: 80000 })];
    const result = deriveNextService(schedules, 84000, now);
    expect(result?.severity).toBe('danger');
  });

  it('falls back to date-based remaining when mileage is unavailable', () => {
    const schedules = [schedule('a', 'upcoming', { nextDueDate: '2026-08-12T00:00:00Z' })];
    const result = deriveNextService(schedules, undefined, now);
    expect(result?.label).toBe('10 days');
    expect(result?.severity).toBe('warning');
  });

  it('returns undefined when the top schedule has neither due mileage nor due date', () => {
    const schedules = [schedule('a', 'upcoming')];
    expect(deriveNextService(schedules, 84000, now)).toBeUndefined();
  });
});
