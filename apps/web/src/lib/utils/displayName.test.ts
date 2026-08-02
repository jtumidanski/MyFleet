import { describe, it, expect } from 'vitest';
import { displayFor } from './displayName';
import type { UserAttributes } from '../../types/models/user';

function attrs(displayName: string, email: string): UserAttributes {
  return { displayName, email, avatarUrl: '', themePreference: 'system' };
}

describe('displayFor', () => {
  it('prefers the display name', () => {
    expect(displayFor('u1', { u1: attrs('Jane Doe', 'jane@example.com') })).toBe('Jane Doe');
  });

  // Go marshals an unset displayName as "", not null, so `??` would let the
  // empty string through and render a blank row.
  it('falls through an empty-string display name to the email', () => {
    expect(displayFor('u1', { u1: attrs('', 'jane@example.com') })).toBe('jane@example.com');
  });

  it('falls back to the first 8 characters of the id when both are empty', () => {
    expect(displayFor('abcdefgh-1234', { 'abcdefgh-1234': attrs('', '') })).toBe('abcdefgh');
  });

  // FR-1.7: a name-lookup failure leaves the map undefined, and the list must
  // still render rather than blanking the card.
  it('falls back to the id when the user is absent or the lookup failed', () => {
    expect(displayFor('abcdefgh-1234', {})).toBe('abcdefgh');
    expect(displayFor('abcdefgh-1234', undefined)).toBe('abcdefgh');
  });

  it('returns a short id unchanged rather than padding it', () => {
    expect(displayFor('u1', undefined)).toBe('u1');
  });
});
