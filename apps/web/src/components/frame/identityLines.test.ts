import { describe, it, expect } from 'vitest';
import { identityLines } from './identityLines';
import type { User } from '../../types/models/user';

function user(displayName: string, email: string): User {
  return {
    id: 'u1',
    type: 'users',
    attributes: { displayName, email, avatarUrl: '', themePreference: 'system' },
  };
}

/**
 * FR-PROFILE-4. Today's header renders `?? ''`, so a user with no display name
 * gets an empty label — which in a dropdown is an empty menu header rather than
 * a merely blank span.
 */
describe('identityLines', () => {
  it('uses the display name and the email', () => {
    expect(identityLines(user('Ada Lovelace', 'ada@example.com'))).toEqual({
      primary: 'Ada Lovelace',
      secondary: 'ada@example.com',
    });
  });

  it('falls back to the email when there is no display name', () => {
    expect(identityLines(user('', 'ada@example.com'))).toEqual({
      primary: 'ada@example.com',
      secondary: '',
    });
  });

  // The other half of that fallback: a display name present with no email
  // also produces an empty secondary line — ProfileMenu's `secondary !== ''`
  // guard must suppress it exactly the same as the no-display-name case.
  it('leaves the secondary empty when there is a display name but no email', () => {
    expect(identityLines(user('Ada Lovelace', ''))).toEqual({
      primary: 'Ada Lovelace',
      secondary: '',
    });
  });

  it('treats a whitespace-only display name as absent', () => {
    expect(identityLines(user('   ', 'ada@example.com')).primary).toBe('ada@example.com');
  });

  it('reads "Account" when both are empty', () => {
    expect(identityLines(user('', ''))).toEqual({ primary: 'Account', secondary: '' });
  });

  it('reads "Account" when there is no user at all', () => {
    expect(identityLines(null)).toEqual({ primary: 'Account', secondary: '' });
  });
});
