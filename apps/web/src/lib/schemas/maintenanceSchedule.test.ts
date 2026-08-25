import { describe, it, expect } from 'vitest';
import {
  maintenanceScheduleSchema,
  convertToRecurrenceSchema,
} from './maintenanceSchedule';

const DUE_DATE = '2026-11-30';
const DUE_MILEAGE = 60000;

/**
 * The four-rule matrix from design §7:
 *
 * | kind      | covers time                                | covers mileage                                 |
 * | recurring | intervalMonths > 0 AND dueDate required    | intervalMiles > 0 AND dueMileage > 0 required  |
 * | oneTime   | dueDate required, intervalMonths forbidden | dueMileage > 0 required, intervalMiles forbidden |
 */
const recurringTime = {
  categoryId: 'cat-1',
  kind: 'recurring' as const,
  recurrenceType: 'time' as const,
  intervalMonths: 6,
  dueDate: DUE_DATE,
};

const oneTimeMileage = {
  categoryId: 'cat-1',
  kind: 'oneTime' as const,
  recurrenceType: 'mileage' as const,
  dueMileage: DUE_MILEAGE,
};

/** The first issue path, so a test can assert WHICH field was blamed. */
function firstIssuePath(input: unknown): string | undefined {
  const result = maintenanceScheduleSchema.safeParse(input);
  if (result.success) return undefined;
  return result.error.issues[0]?.path.join('.');
}

describe('maintenanceScheduleSchema — recurring', () => {
  it('accepts a complete time schedule', () => {
    expect(maintenanceScheduleSchema.safeParse(recurringTime).success).toBe(true);
  });

  it('requires intervalMonths for a time schedule', () => {
    const { intervalMonths, ...rest } = recurringTime;
    void intervalMonths;
    expect(firstIssuePath(rest)).toBe('intervalMonths');
  });

  it('requires a first-due date for a time schedule', () => {
    const { dueDate, ...rest } = recurringTime;
    void dueDate;
    expect(firstIssuePath(rest)).toBe('dueDate');
  });

  it('rejects a zero intervalMonths', () => {
    expect(maintenanceScheduleSchema.safeParse({ ...recurringTime, intervalMonths: 0 }).success).toBe(false);
  });

  it('accepts a complete mileage schedule', () => {
    const result = maintenanceScheduleSchema.safeParse({
      categoryId: 'cat-1',
      kind: 'recurring',
      recurrenceType: 'mileage',
      intervalMiles: 5000,
      dueMileage: DUE_MILEAGE,
    });
    expect(result.success).toBe(true);
  });

  it('requires a first-due odometer for a mileage schedule', () => {
    expect(
      firstIssuePath({
        categoryId: 'cat-1',
        kind: 'recurring',
        recurrenceType: 'mileage',
        intervalMiles: 5000,
      }),
    ).toBe('dueMileage');
  });

  it('accepts a complete hybrid schedule', () => {
    const result = maintenanceScheduleSchema.safeParse({
      categoryId: 'cat-1',
      kind: 'recurring',
      recurrenceType: 'hybrid',
      intervalMonths: 6,
      intervalMiles: 5000,
      dueDate: DUE_DATE,
      dueMileage: DUE_MILEAGE,
    });
    expect(result.success).toBe(true);
  });

  it('rejects a hybrid schedule missing the mileage axis entirely', () => {
    const result = maintenanceScheduleSchema.safeParse({
      categoryId: 'cat-1',
      kind: 'recurring',
      recurrenceType: 'hybrid',
      intervalMonths: 6,
      dueDate: DUE_DATE,
    });
    expect(result.success).toBe(false);
  });

  it('does not require the mileage axis on a time schedule', () => {
    const result = maintenanceScheduleSchema.safeParse({
      ...recurringTime,
      intervalMiles: undefined,
      dueMileage: undefined,
    });
    expect(result.success).toBe(true);
  });
});

describe('maintenanceScheduleSchema — one-time', () => {
  it('accepts a mileage one-off with no interval', () => {
    expect(maintenanceScheduleSchema.safeParse(oneTimeMileage).success).toBe(true);
  });

  it('accepts a date one-off with no interval', () => {
    const result = maintenanceScheduleSchema.safeParse({
      categoryId: 'cat-1',
      kind: 'oneTime',
      recurrenceType: 'time',
      dueDate: DUE_DATE,
    });
    expect(result.success).toBe(true);
  });

  it('requires a due date on the time axis', () => {
    expect(
      firstIssuePath({ categoryId: 'cat-1', kind: 'oneTime', recurrenceType: 'time' }),
    ).toBe('dueDate');
  });

  it('requires a due odometer on the mileage axis', () => {
    expect(
      firstIssuePath({ categoryId: 'cat-1', kind: 'oneTime', recurrenceType: 'mileage' }),
    ).toBe('dueMileage');
  });

  it('rejects an intervalMonths on a one-time schedule', () => {
    const result = maintenanceScheduleSchema.safeParse({
      categoryId: 'cat-1',
      kind: 'oneTime',
      recurrenceType: 'time',
      dueDate: DUE_DATE,
      intervalMonths: 6,
    });
    expect(result.success).toBe(false);
  });

  it('rejects an intervalMiles on a one-time schedule', () => {
    const result = maintenanceScheduleSchema.safeParse({
      ...oneTimeMileage,
      intervalMiles: 5000,
    });
    expect(result.success).toBe(false);
  });

  it('accepts a hybrid one-off carrying both due points', () => {
    const result = maintenanceScheduleSchema.safeParse({
      categoryId: 'cat-1',
      kind: 'oneTime',
      recurrenceType: 'hybrid',
      dueDate: DUE_DATE,
      dueMileage: DUE_MILEAGE,
    });
    expect(result.success).toBe(true);
  });
});

describe('maintenanceScheduleSchema — shared fields', () => {
  it('requires categoryId', () => {
    const { categoryId, ...rest } = recurringTime;
    void categoryId;
    expect(maintenanceScheduleSchema.safeParse(rest).success).toBe(false);
  });

  it('requires kind', () => {
    const { kind, ...rest } = recurringTime;
    void kind;
    expect(maintenanceScheduleSchema.safeParse(rest).success).toBe(false);
  });

  it('rejects an unknown recurrenceType', () => {
    expect(
      maintenanceScheduleSchema.safeParse({ ...recurringTime, recurrenceType: 'once' }).success,
    ).toBe(false);
  });
});

describe('convertToRecurrenceSchema', () => {
  it('accepts a time recurrence with an interval', () => {
    expect(convertToRecurrenceSchema.safeParse({ recurrenceType: 'time', intervalMonths: 12 }).success).toBe(true);
  });

  it('requires intervalMonths for a time recurrence', () => {
    expect(convertToRecurrenceSchema.safeParse({ recurrenceType: 'time' }).success).toBe(false);
  });

  it('requires intervalMiles for a mileage recurrence', () => {
    expect(convertToRecurrenceSchema.safeParse({ recurrenceType: 'mileage' }).success).toBe(false);
  });

  it('requires both intervals for a hybrid recurrence', () => {
    expect(
      convertToRecurrenceSchema.safeParse({ recurrenceType: 'hybrid', intervalMonths: 12 }).success,
    ).toBe(false);
    expect(
      convertToRecurrenceSchema.safeParse({
        recurrenceType: 'hybrid',
        intervalMonths: 12,
        intervalMiles: 5000,
      }).success,
    ).toBe(true);
  });

  // The conversion dialog carries no category or due-point fields: the category
  // is fixed and read-only, and the anchor is being cleared, not set.
  it('does not require a categoryId or a due point', () => {
    expect(convertToRecurrenceSchema.safeParse({ recurrenceType: 'mileage', intervalMiles: 5000 }).success).toBe(true);
  });
});
