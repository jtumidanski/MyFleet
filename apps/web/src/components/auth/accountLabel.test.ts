import { describe, it, expect } from 'vitest';
import { accountLabel } from './accountLabel';
import type { User } from '../../types/models/user';

function user(overrides: Partial<User['attributes']> = {}): User {
  return {
    id: 'u1',
    type: 'users',
    attributes: {
      displayName: 'Ada Lovelace',
      email: 'ada@example.com',
      avatarUrl: '',
      themePreference: 'system',
      ...overrides,
    },
  };
}

describe('accountLabel', () => {
  it('returns the email when there is one', () => {
    expect(accountLabel(user({ displayName: '' }))).toBe('ada@example.com');
  });

  // The deliberate divergence from components/frame/identityLines.ts, which
  // orders displayName first. This assertion IS the requirement (FR-IDENT-2):
  // the footer disambiguates an account, and two Google accounts routinely
  // share one display name.
  it('prefers the email over the display name when both are present', () => {
    expect(accountLabel(user())).toBe('ada@example.com');
  });

  it('falls back to the display name when there is no email', () => {
    expect(accountLabel(user({ email: '' }))).toBe('Ada Lovelace');
  });

  it('treats a whitespace-only email as absent', () => {
    expect(accountLabel(user({ email: '   ' }))).toBe('Ada Lovelace');
  });

  it('trims surrounding whitespace off the value it returns', () => {
    expect(accountLabel(user({ email: '  ada@example.com  ' }))).toBe('ada@example.com');
  });

  // FR-IDENT-3: null, not '' and not 'Account'. The caller omits the whole
  // identity line on null; a placeholder would defeat the point of the line.
  it('returns null when both fields are empty', () => {
    expect(accountLabel(user({ email: '', displayName: '' }))).toBeNull();
  });

  it('returns null when both fields are whitespace only', () => {
    expect(accountLabel(user({ email: ' ', displayName: '\t' }))).toBeNull();
  });

  it('returns null for a null user', () => {
    expect(accountLabel(null)).toBeNull();
  });

  it('returns null for an undefined user', () => {
    expect(accountLabel(undefined)).toBeNull();
  });
});
