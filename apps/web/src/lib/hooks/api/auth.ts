import { useQuery } from '@tanstack/react-query';
import type { JsonApiDocument } from '@myfleet/shared-ts';
import { apiClient } from '../../api/client';
import { getAccessToken } from '../../api/token';
import type { AuthMeta, User } from '../../../types/models/user';

export const authKeys = {
  all: ['auth'] as const,
  me: () => ['auth', 'me'] as const,
};

export interface MeResult {
  user: User;
  activeFleetId: string | null;
  role: AuthMeta['role'];
}

// `GET /api/auth/me` → user resource + meta:{ activeFleetId, role }.
async function fetchMe(): Promise<MeResult> {
  const doc = await apiClient.request<JsonApiDocument<User> & { meta?: AuthMeta }>('/api/auth/me');
  return {
    user: doc.data,
    activeFleetId: doc.meta?.activeFleetId ?? null,
    role: doc.meta?.role ?? null,
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
