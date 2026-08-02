import { describe, it, expect } from 'vitest';
import { purgeStatusLabel, purgeStatusVariant } from './purgeStatus';

describe('purgeStatusLabel', () => {
  it('speaks in outcomes, not API vocabulary', () => {
    expect(purgeStatusLabel('pending', [])).toBe('Recoverable');
    expect(purgeStatusLabel('reaped', [])).toBe('Deleted for good');
    expect(purgeStatusLabel('cancelled', [])).toBe('Restored');
  });

  // "Partial" tells an operator nothing actionable. Naming the service does.
  it('names what actually failed for a partial operation', () => {
    expect(purgeStatusLabel('partial', ['media'])).toBe('Media not deleted');
    expect(purgeStatusLabel('partial', ['notification'])).toBe('Notifications not deleted');
    expect(purgeStatusLabel('partial', ['media', 'notification'])).toBe(
      'Media and notifications not deleted',
    );
  });

  it('degrades safely on an unknown status rather than rendering a blank chip', () => {
    expect(purgeStatusLabel('something-new' as never, [])).toBe('Unknown');
  });
});

describe('purgeStatusVariant', () => {
  it('maps each status to a badge variant', () => {
    expect(purgeStatusVariant('pending')).toBe('info');
    expect(purgeStatusVariant('partial')).toBe('warning');
    expect(purgeStatusVariant('reaped')).toBe('danger');
    expect(purgeStatusVariant('cancelled')).toBe('success');
  });
});
