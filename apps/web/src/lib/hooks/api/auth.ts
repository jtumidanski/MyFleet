import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import type { JsonApiDocument } from '@myfleet/shared-ts';
import { apiClient } from '../../api/client';
import { getAccessToken } from '../../api/token';
import type { AuthMeta, ThemePreference, User } from '../../../types/models/user';

export const authKeys = {
  all: ['auth'] as const,
  me: () => ['auth', 'me'] as const,
};

export interface MeResult {
  user: User;
  activeFleetId: string | null;
  role: AuthMeta['role'];
  platformAdmin: boolean;
}

// `GET /api/auth/me` → user resource + meta:{ activeFleetId, role }.
async function fetchMe(): Promise<MeResult> {
  const doc = await apiClient.request<JsonApiDocument<User> & { meta?: AuthMeta }>('/api/auth/me');
  return {
    user: doc.data,
    activeFleetId: doc.meta?.activeFleetId ?? null,
    role: doc.meta?.role ?? null,
    // Defaults to false: an older server that does not send the field must not
    // accidentally reveal the console's entry point.
    platformAdmin: doc.meta?.platformAdmin ?? false,
  };
}

/**
 * Identity query. Only runs when an access token is present; otherwise the user
 * is treated as unauthenticated without an extra network round-trip.
 */
export function useMe() {
  return useQuery({
    queryKey: authKeys.me(),
    queryFn: fetchMe,
    enabled: !!getAccessToken(),
    staleTime: 5 * 60 * 1000,
    gcTime: 10 * 60 * 1000,
    retry: false,
  });
}

// `POST /api/auth/logout` — clears the HttpOnly refresh cookie server-side.
// credentials:'include' so the browser sends the cookie to be invalidated.
export async function logoutRequest(): Promise<void> {
  await fetch('/api/auth/logout', {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/vnd.api+json' },
  }).catch(() => undefined);
}

/**
 * `PATCH /api/auth/me` — updates the caller's own theme preference.
 *
 * Resolves with `null` and issues no request when there is no access token
 * (FR-PERSIST-8). It resolves rather than rejecting on purpose: a rejection
 * would raise the save-failure toast at a user who never had a session to save
 * into. Since FR-TOGGLE-7 confines the toggle to the authenticated shell, this
 * is defence-in-depth rather than a live path.
 *
 * No user identifier appears in the path or the body — the server derives the
 * target from the validated JWT `sub` (FR-SEC-1).
 */
export async function updateThemePreference(
  themePreference: ThemePreference,
): Promise<User | null> {
  if (!getAccessToken()) return null;
  const doc = await apiClient.request<JsonApiDocument<User>>('/api/auth/me', {
    method: 'PATCH',
    body: JSON.stringify({ data: { type: 'users', attributes: { themePreference } } }),
  });
  return doc.data;
}

/**
 * Optimistic theme mutation.
 *
 * `onMutate` writes the new value into the identity cache so a later refetch
 * cannot momentarily revert the theme (FR-PERSIST-6).
 *
 * There is deliberately NO onError rollback. The textbook optimistic pattern
 * restores the snapshot on failure, but here that would make the next ThemeSync
 * pass re-adopt the old value and flip the theme out from under the user —
 * exactly what FR-PERSIST-5 forbids. The cache stays knowingly
 * optimistic-but-wrong until a genuine refetch, and ThemeToggle's toast tells
 * the user the preference will not survive the session.
 */
export function useUpdateTheme() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: updateThemePreference,
    onMutate: (themePreference: ThemePreference) => {
      queryClient.setQueryData<MeResult>(authKeys.me(), (previous) =>
        previous
          ? {
              ...previous,
              user: {
                ...previous.user,
                attributes: { ...previous.user.attributes, themePreference },
              },
            }
          : previous,
      );
    },
  });
}
