import { describe, it, expect } from 'vitest';
import { maintenanceScheduleSchema } from './maintenanceSchedule';

const base = {
  categoryId: 'cat-1',
};

describe('maintenanceScheduleSchema', () => {
  describe('recurrenceType: time', () => {
    it('requires intervalMonths (positive) for time recurrence', () => {
      const result = maintenanceScheduleSchema.safeParse({
        ...base,
        recurrenceType: 'time',
      });
      expect(result.success).toBe(false);
    });

    it('fails when intervalMonths is 0 for time recurrence', () => {
      const result = maintenanceScheduleSchema.safeParse({
        ...base,
        recurrenceType: 'time',
        intervalMonths: 0,
      });
      expect(result.success).toBe(false);
    });

    it('passes with valid intervalMonths for time recurrence', () => {
      const result = maintenanceScheduleSchema.safeParse({
        ...base,
        recurrenceType: 'time',
        intervalMonths: 6,
      });
      expect(result.success).toBe(true);
    });

    it('ignores intervalMiles for time recurrence (does not require it)', () => {
      const result = maintenanceScheduleSchema.safeParse({
        ...base,
        recurrenceType: 'time',
        intervalMonths: 6,
        intervalMiles: undefined,
      });
      expect(result.success).toBe(true);
    });
  });

  describe('recurrenceType: mileage', () => {
    it('requires intervalMiles (positive) for mileage recurrence', () => {
      const result = maintenanceScheduleSchema.safeParse({
        ...base,
        recurrenceType: 'mileage',
      });
      expect(result.success).toBe(false);
    });

    it('fails when intervalMiles is 0 for mileage recurrence', () => {
      const result = maintenanceScheduleSchema.safeParse({
        ...base,
        recurrenceType: 'mileage',
        intervalMiles: 0,
      });
      expect(result.success).toBe(false);
    });

    it('passes with valid intervalMiles for mileage recurrence', () => {
      const result = maintenanceScheduleSchema.safeParse({
        ...base,
        recurrenceType: 'mileage',
        intervalMiles: 5000,
      });
      expect(result.success).toBe(true);
    });

    it('does not require intervalMonths for mileage recurrence', () => {
      const result = maintenanceScheduleSchema.safeParse({
        ...base,
        recurrenceType: 'mileage',
        intervalMiles: 5000,
        intervalMonths: undefined,
      });
      expect(result.success).toBe(true);
    });
  });

  describe('recurrenceType: hybrid', () => {
    it('requires both intervalMonths and intervalMiles for hybrid', () => {
      const result = maintenanceScheduleSchema.safeParse({
        ...base,
        recurrenceType: 'hybrid',
      });
      expect(result.success).toBe(false);
    });

    it('fails hybrid with only intervalMonths', () => {
      const result = maintenanceScheduleSchema.safeParse({
        ...base,
        recurrenceType: 'hybrid',
        intervalMonths: 6,
      });
      expect(result.success).toBe(false);
    });

    it('fails hybrid with only intervalMiles', () => {
      const result = maintenanceScheduleSchema.safeParse({
        ...base,
        recurrenceType: 'hybrid',
        intervalMiles: 5000,
      });
      expect(result.success).toBe(false);
    });

    it('passes with both intervalMonths and intervalMiles for hybrid', () => {
      const result = maintenanceScheduleSchema.safeParse({
        ...base,
        recurrenceType: 'hybrid',
        intervalMonths: 6,
        intervalMiles: 5000,
      });
      expect(result.success).toBe(true);
    });

    it('fails hybrid when intervalMonths is 0', () => {
      const result = maintenanceScheduleSchema.safeParse({
        ...base,
        recurrenceType: 'hybrid',
        intervalMonths: 0,
        intervalMiles: 5000,
      });
      expect(result.success).toBe(false);
    });

    it('fails hybrid when intervalMiles is 0', () => {
      const result = maintenanceScheduleSchema.safeParse({
        ...base,
        recurrenceType: 'hybrid',
        intervalMonths: 6,
        intervalMiles: 0,
      });
      expect(result.success).toBe(false);
    });
  });

  describe('categoryId', () => {
    it('requires categoryId', () => {
      const result = maintenanceScheduleSchema.safeParse({
        recurrenceType: 'time',
        intervalMonths: 6,
      });
      expect(result.success).toBe(false);
    });
  });
});
