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

export type AuditAction =
  'purge.created' | 'purge.cancelled' | 'purge.retried' | 'purge.reaped' | 'vehicle.transferred';

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
  /** Populated only for `vehicle.transferred`; empty string otherwise. */
  source_fleet_id: string;
  destination_fleet_id: string;
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

/** A category the transfer would add to the destination fleet. */
export interface CategoryToCreate {
  name: string;
  kind: string;
}

/**
 * GET /api/fleet/admin/vehicles/{id}/transfer-preview.
 *
 * `vehicle_label` is the EXACT string the confirmation input must match. It is
 * computed server-side and echoed here precisely so the console never derives
 * its own copy of a phrase that has to match byte for byte.
 *
 * The destination fields and `categories_to_create` are empty until a
 * destination is chosen — neither can be computed without one.
 *
 * `counts.media_objects` is "media references held by this vehicle". The
 * completed transfer reports media-service's own read-back, which is lower only
 * when a reference was already dangling before the transfer.
 *
 * `counts` carries no `notifications` key: notification-service owns that
 * relationship and the preview deliberately calls no other service.
 */
export interface VehicleTransferPreviewAttributes {
  vehicle_label: string;
  source_fleet_id: string;
  source_fleet_name: string;
  destination_fleet_id: string;
  destination_fleet_name: string;
  counts: Record<string, number>;
  categories_to_create: CategoryToCreate[];
  warnings: string[];
}

/** POST /api/fleet/admin/vehicles/{id}/transfer — the completed move. */
export interface VehicleTransferAttributes {
  vehicle_id: string;
  source_fleet_id: string;
  destination_fleet_id: string;
  transferred_at: string;
  affected_counts: Record<string, number>;
}

/**
 * `meta` on the transfer response.
 *
 * `count_semantics` maps an `affected_counts` key to a sentence correcting what
 * that number actually measures. The server populates it for exactly
 * `media_objects` and `notifications`: both are read back from the downstream
 * service as "live rows now on the destination fleet", so a row that was
 * already there is included and was NOT moved by this call. Every other key is
 * genuinely "rows this transfer moved" and is deliberately left unannotated —
 * annotating all of them would bury the two that need it.
 *
 * Optional at every level because the server omits `meta` entirely when neither
 * key is present (see admin.TransferMeta, which returns nil).
 */
export interface VehicleTransferMeta {
  count_semantics?: Record<string, string>;
}

export interface TransferVehicleInput {
  destination_fleet_id: string;
  confirmation: string;
}
