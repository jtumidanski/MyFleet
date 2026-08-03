import type { User } from '../../types/models/user';

export interface IdentityLines {
  primary: string;
  secondary: string;
}

/**
 * The two lines at the top of the profile menu (FR-PROFILE-4).
 *
 * displayName → email → "Account". Worth naming and testing on its own because
 * today's header renders `?? ''`, which on a user with no display name produces
 * an empty label.
 *
 * The second line is the email, omitted when it would repeat the first — a user
 * with no display name would otherwise show their email twice.
 */
export function identityLines(user: User | null | undefined): IdentityLines {
  const displayName = (user?.attributes.displayName ?? '').trim();
  const email = (user?.attributes.email ?? '').trim();

  if (displayName) return { primary: displayName, secondary: email };
  if (email) return { primary: email, secondary: '' };
  return { primary: 'Account', secondary: '' };
}
