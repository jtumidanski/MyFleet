import { describe, it, expect } from 'vitest';
import { formatMoney, formatMileage, formatRelativeTime } from './formatters';

describe('formatters', () => {
  it('formats money in USD', () => {
    expect(formatMoney(1234.5)).toBe('$1,234.50');
  });
  it('formats mileage with thousands + unit', () => {
    expect(formatMileage(12345)).toBe('12,345 mi');
  });
});

describe('formatRelativeTime', () => {
  const now = new Date('2026-08-02T12:00:00Z');
  const iso = (daysAgo: number) =>
    new Date(now.getTime() - daysAgo * 24 * 60 * 60 * 1000).toISOString();

  it('says "today" for anything inside the last day', () => {
    expect(formatRelativeTime(now.toISOString(), now)).toBe('today');
    expect(formatRelativeTime(iso(0.5), now)).toBe('today');
  });

  it('says "yesterday" one day back', () => {
    expect(formatRelativeTime(iso(1), now)).toBe('yesterday');
  });

  it('counts days up to a week', () => {
    expect(formatRelativeTime(iso(6), now)).toBe('6 days ago');
  });

  it('switches to weeks at seven days', () => {
    expect(formatRelativeTime(iso(7), now)).toBe('last week');
    expect(formatRelativeTime(iso(21), now)).toBe('3 weeks ago');
  });

  it('switches to months at five weeks', () => {
    expect(formatRelativeTime(iso(35), now)).toBe('last month');
    expect(formatRelativeTime(iso(120), now)).toBe('4 months ago');
  });

  it('switches to years at a full year', () => {
    expect(formatRelativeTime(iso(365), now)).toBe('last year');
    expect(formatRelativeTime(iso(800), now)).toBe('2 years ago');
  });

  it('clamps a future timestamp to "today" rather than counting forwards', () => {
    // lastActivityAt should never be in the future, but clock skew is real and
    // "in 3 days" on a Last activity slot would read as a bug.
    expect(formatRelativeTime(iso(-3), now)).toBe('today');
  });

  it('returns an empty string for an unparseable input', () => {
    // The card renders an em-dash for this, same as an absent value.
    expect(formatRelativeTime('not-a-date', now)).toBe('');
    expect(formatRelativeTime('', now)).toBe('');
  });
});
