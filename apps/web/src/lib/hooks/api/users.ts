/**
 * React Query hooks for user display-name resolution (task-014).
 *
 * This is a SECOND, INDEPENDENT query alongside useMembers rather than a
 * `select` over it. That independence is what buys FR-1.7: "memberships loaded,
 * names failed" is a normal renderable state, and the member list falls back to
 * shortened ids instead of going blank.
 */
import { useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import { userService } from '../../../services/api/UserService';
import type { UserAttributes } from '../../../types/models/user';

// ---------------------------------------------------------------------------
// Query key factory
// ---------------------------------------------------------------------------

/**
 * all          -> ['users']
 * byIds(ids)   -> ['users', 'byIds', 'a,b,c']
 *
 * The ids are sorted into the key so it does not depend on the order
 * useMembers happens to return rows in — otherwise a reordered list refetches.
 */
export const userKeys = {
  all: ['users'] as const,
  byIds: (ids: string[]) => [...userKeys.all, 'byIds', [...ids].sort().join(',')] as const,
};

// ---------------------------------------------------------------------------
// Queries
// ---------------------------------------------------------------------------

/**
 * GET /api/auth/users?ids=… — resolves a batch of user ids to their profiles,
 * indexed by id.
 *
 * Sorting and de-duplication happen here so callers can pass the raw membership
 * order. `staleTime` matches useMembers: the two are rendered together and a
 * shorter window here would refetch names on every members cache hit.
 */
export function useUsers(ids: string[]) {
  const sorted = useMemo(() => [...new Set(ids)].sort(), [ids]);
  return useQuery({
    queryKey: userKeys.byIds(sorted),
    queryFn: () => userService.listByIds(sorted),
    enabled: sorted.length > 0,
    staleTime: 60 * 1000,
    gcTime: 5 * 60 * 1000,
    select: (result): Record<string, UserAttributes> =>
      Object.fromEntries(result.data.map((r) => [r.id, r.attributes])),
  });
}
