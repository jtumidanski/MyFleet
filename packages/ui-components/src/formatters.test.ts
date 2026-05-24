import { describe, it, expect } from 'vitest';
import { formatMoney, formatMileage } from './formatters';

describe('formatters', () => {
  it('formats money in USD', () => {
    expect(formatMoney(1234.5)).toBe('$1,234.50');
  });
  it('formats mileage with thousands + unit', () => {
    expect(formatMileage(12345)).toBe('12,345 mi');
  });
});
