import { useMutation, useQueryClient } from '@tanstack/react-query';
import type { JsonApiDocument } from '@myfleet/shared-ts';
import { apiClient } from '../../api/client';
import { setAccessToken } from '../../api/token';
import { authKeys } from './auth';
import type { CreateFleetInput } from '../../schemas/fleet';
import type { Fleet } from '../../../types/models/fleet';

export const fleetKeys = {
  all: ['fleets'] as const,
  detail: (id: string) => ['fleets', 'detail', id] as const,
};

interface RefreshResponse {
  data?: { attributes?: { accessToken?: string } };
}

// After the first fleet is created the existing access token still carries a
// null activeFleetId/role (it was minted before membership existed). Refreshing
// re-resolves membership server-side and mints a token reflecting the new fleet.
async function refreshTokenForNewFleet(): Promise<void> {
  const res = await fetch('/api/auth/refresh', {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/vnd.api+json' },
  });
  if (!res.ok) return;
  const body = (await res.json().catch(() => null)) as RefreshResponse | null;
  const token = body?.data?.attributes?.accessToken;
  if (token) setAccessToken(token);
}

/**
 * Onboarding: create a fleet (the caller becomes its owner), then refresh the
 * access token so the new activeFleetId/role are reflected, then invalidate the
 * identity query so the app re-reads membership.
 */
export function useCreateFleet() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: CreateFleetInput): Promise<Fleet> => {
      const doc = await apiClient.request<JsonApiDocument<Fleet>>('/api/fleet/fleets', {
        method: 'POST',
        body: JSON.stringify({ data: { type: 'fleets', attributes: { name: input.name } } }),
      });
      return doc.data;
    },
    onSuccess: async () => {
      await refreshTokenForNewFleet();
      await queryClient.invalidateQueries({ queryKey: authKeys.all });
    },
  });
}
