/**
 * Platform admin console resource shapes.
 *
 * These mirror the transforms in apps/fleet-service/internal/admin/rest.go
 * field for field. They were written against that file rather than the PRD's
 * sketch, which had drifted in several places (`affected` vs `affected_counts`,
 * `pending_invites` vs `invites` on the detail view, `current_mileage` vs
 * `mileage`). Where the two disagree, the Go transform is the contract.
 */

export type PurgeStatus = 'pending' | 'partial' | 'cancelled' | 'reaped';
export type PurgeScope = 'system' | 'fleet' | 'record';
export type DeletedFilter = 'include' | 'exclude' | 'only';

export interface AdminVehicleCounts {
  active: number;
  pending_purge: number;
}

/**
 * A null count means "we could not ask that service", not zero. The console
 * renders it as an em dash; rendering 0 would claim there is no data
 * (FR-ADMIN-UI-6).
 */
export interface AdminStatsAttributes {
  fleets: number | null;
  memberships: number | null;
  maintenance_records: number | null;
  maintenance_schedules: number | null;
  fuel_logs: number | null;
  mileage_records: number | null;
  activity_events: number | null;
  pending_invites: number | null;
  users: number | null;
  media_objects: number | null;
  notifications: number | null;
  vehicles: AdminVehicleCounts;
  warnings: string[];
}

export interface AdminFleetAttributes {
  name: string;
  created_at: string;
  owner_user_id: string;
  /** Empty when auth-service could not be reached — see `warnings`. */
  owner_email: string;
  owner_display_name: string;
  member_count: number;
  vehicle_count: number;
  /** Admin-stamped only. A fleet a user deleted is not recoverable here. */
  pending_purge: boolean;
  /** ISO deadline; null unless pending_purge. Drives the countdown chip. */
  purge_after: string | null;
}

export interface AdminMemberRow {
  user_id: string;
  /** Empty when auth-service could not be reached — see `warnings`. */
  email: string;
  display_name: string;
  role: string;
  status: string;
  joined_at: string;
}

export interface AdminVehicleRow {
  id: string;
  nickname: string;
  make: string;
  model: string;
  year: number;
  mileage: number;
  /** Derived server-side via vehicle.StatusDeps; detail view only. */
  status: string;
  pending_purge: boolean;
}

export interface AdminInviteRow {
  id: string;
  email: string;
  role: string;
  expires_at: string;
}

export interface AdminFleetDetailAttributes extends AdminFleetAttributes {
  members: AdminMemberRow[];
  vehicles: AdminVehicleRow[];
  invites: AdminInviteRow[];
  /** Same numbers the purge will report — one Count, one predicate. */
  counts: Record<string, number>;
  warnings: string[];
}

export interface PurgeOperationAttributes {
  scope: PurgeScope;
  target_type: string;
  target_id: string;
  /** Denormalised at request time, so the log reads after the target is gone. */
  target_label: string;
  status: PurgeStatus;
  requested_by_user_id: string;
  requested_by_email: string;
  requested_at: string;
  purge_after: string;
  reaped_at: string | null;
  cancelled_at: string | null;
  affected_counts: Record<string, number>;
  failed_services: string[];
}

export type AuditAction = 'purge.created' | 'purge.cancelled' | 'purge.retried' | 'purge.reaped';

export interface AuditEventAttributes {
  actor_user_id: string;
  actor_email: string;
  action: AuditAction;
  scope: string;
  target_type: string;
  target_id: string;
  target_label: string;
  purge_operation_id: string;
  affected_counts: Record<string, number>;
  correlation_id: string;
  created_at: string;
}

export interface AdminUserFleetRow {
  fleet_id: string;
  name: string;
  role: string;
}

export interface AdminUserAttributes {
  email: string;
  display_name: string;
  created_at: string;
  last_login_at: string | null;
  /** Displayed, never editable here — granting is an out-of-band act. */
  platform_admin: boolean;
  fleets: AdminUserFleetRow[];
}

export interface CreatePurgeInput {
  scope: PurgeScope;
  target_type?: string;
  target_id?: string;
  confirmation?: string;
}
