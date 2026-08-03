import type { User } from '../../types/models/user';

/**
 * The account shown in `Signed in as …` on the fleetless pages (FR-IDENT-2).
 *
 * email → displayName → null.
 *
 * This deliberately DISAGREES with components/frame/identityLines.ts, which
 * orders displayName → email → "Account". The two are not duplicates and must
 * not be collapsed (FR-IDENT-4):
 *
 *   - The profile-menu header is a GREETING. A friendly name is the right first
 *     line there, and "Account" is an acceptable stand-in when there is none.
 *   - This footer is a DISAMBIGUATION, read by someone who may have signed in
 *     with the wrong account. The email is the identifying field, a display
 *     name is not unique (two Google accounts routinely share one), and a
 *     placeholder actively defeats the question the line exists to answer.
 *
 * `null` rather than '' or 'Account' so that "there is nothing to show" is a
 * type-level fact at the call site instead of a `!== ''` convention the next
 * editor has to notice.
 */
export function accountLabel(user: User | null | undefined): string | null {
  const email = (user?.attributes.email ?? '').trim();
  if (email) return email;

  const displayName = (user?.attributes.displayName ?? '').trim();
  if (displayName) return displayName;

  return null;
}
