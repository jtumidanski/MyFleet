import type { UserAttributes } from '../../types/models/user';

/**
 * FR-1.5 fallback chain: displayName, then email, then the first 8 characters
 * of the id.
 *
 * `||` not `??`: auth-service marshals an unset displayName as "" (Attributes
 * are plain Go strings), and `??` would let the empty string through and render
 * a blank row.
 *
 * Lives outside MemberList so the activity feed can adopt the same resolution
 * without importing a component module.
 */
export function displayFor(userId: string, users?: Record<string, UserAttributes>): string {
  const u = users?.[userId];
  return u?.displayName || u?.email || userId.slice(0, 8);
}
