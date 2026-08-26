import type { ActivityEvent } from '../../../types/models/activity';

/**
 * Maps an activity event `type` string to a small icon character and human-
 * readable label, and turns its structured `payload` into display lines.
 *
 * Separated from ActivityEventIcon.tsx to satisfy the react-refresh/only-export-components
 * rule (a component file should only export components).
 *
 * The types below are the ones fleet-service actually records. Grep for
 * `record(tx,` in apps/fleet-service to see the emit sites and the exact payload
 * keys each one writes — the readers here are keyed to those literals, so a new
 * event type shows up as the generic fallback until it is added.
 */
interface ActivityEventMeta {
  icon: string;
  label: string;
}

const EVENT_META: Record<string, ActivityEventMeta> = {
  'vehicle.created': { icon: '🚗', label: 'Vehicle added' },
  'vehicle.updated': { icon: '✏️', label: 'Vehicle updated' },
  'vehicle.deleted': { icon: '🗑️', label: 'Vehicle removed' },
  'fuel.logged': { icon: '⛽', label: 'Fuel logged' },
  'mileage.recorded': { icon: '📍', label: 'Mileage recorded' },
  'maintenance.completed': { icon: '🔧', label: 'Maintenance completed' },
  'maintenance.scheduled': { icon: '📅', label: 'Maintenance scheduled' },
  'schedule.overdue': { icon: '⚠️', label: 'Maintenance overdue' },
  'media.uploaded': { icon: '📷', label: 'Photo uploaded' },
  // Both halves of a platform-admin vehicle transfer
  // (apps/fleet-service/internal/admin/transfer.go). One row lands in EACH
  // fleet's feed, and the source fleet's is its only record that the car left —
  // so an unlabelled "Event" there is worse than useless.
  'vehicle.transferred_out': { icon: '📤', label: 'Vehicle transferred out' },
  'vehicle.transferred_in': { icon: '📥', label: 'Vehicle transferred in' },
  // Membership events. These carry no vehicleId — they are fleet-level — which
  // is why they used to render as a bare "Event" with a timestamp and nothing
  // else at all.
  'member.invited': { icon: '✉️', label: 'Member joined by invite' },
  'member.role_changed': { icon: '🔑', label: 'Member role changed' },
  'member.left': { icon: '👋', label: 'Member left the fleet' },
  'member.removed': { icon: '🚪', label: 'Member removed from the fleet' },
};

const FALLBACK: ActivityEventMeta = { icon: '📋', label: 'Event' };

/**
 * The actor recorded for system-generated transitions
 * (apps/fleet-service/internal/maintenanceschedule/processor.go). It is not a
 * user id, so it must never be sent to the user-lookup endpoint.
 */
export const SYSTEM_ACTOR = 'system';

export function getActivityEventLabel(type: string): string {
  return (EVENT_META[type] ?? FALLBACK).label;
}

export function getActivityEventIcon(type: string): string {
  return (EVENT_META[type] ?? FALLBACK).icon;
}

/** A `term: value` line rendered under an event's title. */
export interface ActivityDetail {
  term: string;
  value: string;
}

/** Resolves a user id to a display name. */
export type ResolveUser = (userId: string) => string;

/** Resolves a vehicle id to a label, or undefined when it is not known. */
export type ResolveVehicle = (vehicleId: string) => string | undefined;

export interface ActivityDescribeContext {
  resolveUser: ResolveUser;
  resolveVehicle: ResolveVehicle;
  /**
   * Whether to append the "Vehicle" line at all. False on a vehicle-scoped
   * timeline, where every row is the same vehicle the page is already about —
   * repeating it adds nothing, and for events whose payload froze no name
   * (mileage.recorded, fuel.logged) the line would fall through to the raw id.
   */
  includeVehicle?: boolean;
}

/**
 * Every id the feed needs a display name for: the actors, plus any user named
 * inside a payload. De-duplication is left to `useUsers`, which sorts and
 * uniques for its query key anyway.
 *
 * The `system` sentinel is filtered out here rather than at the call site: it
 * is the one actor value that is not a user id, and sending it to
 * `GET /users?ids=` would put a bogus id in the request for every fleet that
 * has ever had an overdue schedule.
 */
export function collectActivityUserIds(events: ActivityEvent[]): string[] {
  const ids: string[] = [];
  for (const event of events) {
    const { actorUserId, payload } = event.attributes;
    if (actorUserId && actorUserId !== SYSTEM_ACTOR) ids.push(actorUserId);
    const target = payload?.target_user_id;
    if (typeof target === 'string' && target) ids.push(target);
  }
  return ids;
}

/** Payload values arrive as `unknown`; render only what is actually a scalar. */
function text(value: unknown): string | undefined {
  if (typeof value === 'string') return value.trim() || undefined;
  if (typeof value === 'number' && Number.isFinite(value)) return String(value);
  return undefined;
}

function money(value: unknown): string | undefined {
  if (typeof value !== 'number' || !Number.isFinite(value)) return undefined;
  return value.toLocaleString(undefined, { style: 'currency', currency: 'USD' });
}

function miles(value: unknown): string | undefined {
  if (typeof value !== 'number' || !Number.isFinite(value)) return undefined;
  return `${value.toLocaleString()} mi`;
}

/**
 * Turns one event's payload into the lines shown beneath its title.
 *
 * Only keys with a meaningful rendering are surfaced. Raw identifiers
 * (`schedule_id`, `maintenance_record_id`, `invite_id`, `fuel_log_id`) are
 * deliberately omitted: a UUID on a timeline tells the reader nothing and
 * displaces the fields that do.
 */
export function describeActivityEvent(
  event: ActivityEvent,
  { resolveUser, resolveVehicle, includeVehicle = true }: ActivityDescribeContext,
): ActivityDetail[] {
  const { type, payload } = event.attributes;
  const p = payload ?? {};
  const details: ActivityDetail[] = [];
  const push = (term: string, value: string | undefined) => {
    if (value) details.push({ term, value });
  };

  switch (type) {
    case 'vehicle.created':
      // The vehicle line below already names it; this adds the spec the
      // nickname would otherwise hide.
      push('Make & model', [text(p.make), text(p.model)].filter(Boolean).join(' ') || undefined);
      break;
    case 'fuel.logged':
      push('Odometer', miles(p.mileage));
      push('Cost', money(p.total_cost));
      break;
    case 'schedule.overdue':
      push('Severity', text(p.severity));
      break;
    case 'member.invited':
      push('Invited', text(p.email));
      push('Role', text(p.role));
      break;
    case 'member.role_changed': {
      const target = text(p.target_user_id);
      push('Member', target ? resolveUser(target) : undefined);
      const from = text(p.from_role);
      const to = text(p.to_role);
      push('Role', from && to ? `${from} → ${to}` : to);
      break;
    }
    case 'member.removed': {
      const target = text(p.target_user_id);
      push('Member', target ? resolveUser(target) : undefined);
      push('Role', text(p.role));
      break;
    }
    case 'member.left':
      push('Role', text(p.role));
      break;
    default:
      break;
  }

  // The vehicle line is common to every vehicle-scoped event, so it is appended
  // once here instead of in each branch. Resolution order matters: the live
  // label first, then whatever the payload froze at record time (the only thing
  // left once a vehicle is deleted), and the raw id only as a last resort.
  const vehicleId = event.attributes.vehicleId;
  if (vehicleId && includeVehicle) {
    // `vehicle_label` is the transfer payload's frozen name. It matters most on
    // the SOURCE fleet's `vehicle.transferred_out` row, where the vehicle is by
    // definition no longer in the fleet being viewed, so `resolveVehicle` has
    // nothing to offer and the line would otherwise fall through to a raw UUID.
    const frozen =
      text(p.nickname) ||
      text(p.vehicle_label) ||
      [text(p.make), text(p.model)].filter(Boolean).join(' ');
    // `||` not `??`: an unresolved label and an empty frozen name are both the
    // empty case, and `??` would let '' through as the vehicle's name.
    push('Vehicle', resolveVehicle(vehicleId) || frozen || vehicleId);
  }

  return details;
}
