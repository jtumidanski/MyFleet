import type { JsonApiResource } from '@myfleet/shared-ts';

// Mirrors auth-service user resource (apps/auth-service/internal/user/rest.go).
export interface UserAttributes {
  email: string;
  displayName: string;
  avatarUrl: string;
}

export type User = JsonApiResource<UserAttributes>;

// Roles within the active fleet (apps/fleet-service/internal/authz/scope.go).
export type FleetRole = 'owner' | 'member' | 'viewer';

// `GET /api/auth/me` meta block.
export interface AuthMeta {
  activeFleetId: string | null;
  role: FleetRole | null;
}
