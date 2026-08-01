import type { JsonApiResource } from '@myfleet/shared-ts';

// The user's theme choice, as stored server-side. `system` is a valid
// PREFERENCE but never a valid RESOLVED theme — see ResolvedTheme in
// src/lib/theme.ts. Collapsing the two is the most likely source of bugs here,
// which is why they live in separate files.
export type ThemePreference = 'light' | 'dark' | 'system';

// Mirrors auth-service user resource (apps/auth-service/internal/user/rest.go).
export interface UserAttributes {
  email: string;
  displayName: string;
  avatarUrl: string;
  themePreference: ThemePreference;
}

export type User = JsonApiResource<UserAttributes>;

// Roles within the active fleet (apps/fleet-service/internal/authz/scope.go).
export type FleetRole = 'owner' | 'member' | 'viewer';

// `GET /api/auth/me` meta block.
export interface AuthMeta {
  activeFleetId: string | null;
  role: FleetRole | null;
}
