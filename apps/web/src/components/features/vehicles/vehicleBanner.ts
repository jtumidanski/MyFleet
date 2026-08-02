import type { VehicleStatus } from '@myfleet/ui-components';
import type { VehicleAttributes, VehicleNextDue } from '../../../types/models/vehicle';

const KNOWN_STATUSES: readonly VehicleStatus[] = [
  'Healthy',
  'Upcoming Maintenance',
  'Overdue',
  'Inactive',
];

/**
 * Narrows a raw server string to a status the UI recognises, or null.
 *
 * The single definition of "is this a status I know": the card, the banner tone,
 * and the icon all key off this, so an unrecognised value can never reach a
 * tinted band as a raw string.
 */
export function asVehicleStatus(value: string | undefined): VehicleStatus | null {
  return value && (KNOWN_STATUSES as readonly string[]).includes(value)
    ? (value as VehicleStatus)
    : null;
}

export type BannerTone = 'danger' | 'warning' | 'quiet';
export type BannerIcon = 'overdue' | 'upcoming' | 'healthy' | 'inactive' | 'unknown';

export interface BannerContent {
  tone: BannerTone;
  icon: BannerIcon;
  text: string;
}

const DAY_MS = 24 * 60 * 60 * 1000;
/** Past this many days, a duration reads better in months than in days. */
const MONTHS_THRESHOLD_DAYS = 60;
/** The server's Inactive threshold, so nothing reaching that branch is younger. */
const MIN_INACTIVE_MONTHS = 12;

type BannerInput = Pick<VehicleAttributes, 'status' | 'nextDue' | 'lastActivityAt'>;

/**
 * Maps a vehicle's derived attributes onto the card banner's tone, icon, and
 * copy.
 *
 * The icon is a string token rather than a component so callers can assert on
 * plain data; VehicleCard owns the single token -> lucide icon map. `now` is
 * injected because the Inactive duration depends on it, and a test that cannot
 * pin "now" cannot assert "13 months".
 *
 * Tone is spent only where action is required: Healthy and Inactive are quiet,
 * so colour anywhere in the grid always means "look here".
 */
export function vehicleBanner(attributes: BannerInput, now: Date): BannerContent {
  const status = asVehicleStatus(attributes.status);

  switch (status) {
    case 'Overdue': {
      const amount = formatAmount(attributes.nextDue);
      return {
        tone: 'danger',
        icon: 'overdue',
        text: amount ? `Service overdue by ${amount}` : 'Maintenance overdue',
      };
    }
    case 'Upcoming Maintenance': {
      const nextDue = attributes.nextDue;
      if (nextDue?.axis === 'time' && nextDue.days === 0) {
        return { tone: 'warning', icon: 'upcoming', text: 'Service due today' };
      }
      const amount = formatAmount(nextDue);
      return {
        tone: 'warning',
        icon: 'upcoming',
        text: amount ? `Service due in ${amount}` : 'Maintenance due soon',
      };
    }
    case 'Healthy':
      return { tone: 'quiet', icon: 'healthy', text: 'Up to date' };
    case 'Inactive': {
      const months = monthsSince(attributes.lastActivityAt, now);
      return {
        tone: 'quiet',
        icon: 'inactive',
        text: months === null ? 'No activity recorded' : `No activity in ${months} months`,
      };
    }
    default:
      // Absent or unrecognised. Quiet, with no attempt to caption a status we
      // cannot name, and never a raw unknown string in a tinted band.
      return { tone: 'quiet', icon: 'unknown', text: 'Status unavailable' };
  }
}

/**
 * Renders the magnitude for a due detail, driven entirely by `axis` — never by
 * which magnitude field happens to be populated. Returns null when the axis is
 * unrecognised or carries no magnitude, which pushes the caller onto its
 * generic-but-still-tinted copy: urgency has to survive bad data.
 */
function formatAmount(nextDue: VehicleNextDue | undefined): string | null {
  if (!nextDue) return null;

  if (nextDue.axis === 'mileage') {
    if (typeof nextDue.miles !== 'number') return null;
    return `${nextDue.miles.toLocaleString('en-US')} mi`;
  }
  if (nextDue.axis === 'time') {
    const days = nextDue.days;
    if (typeof days !== 'number') return null;
    if (days >= MONTHS_THRESHOLD_DAYS) return `${Math.round(days / 30)} months`;
    return `${days} ${days === 1 ? 'day' : 'days'}`;
  }
  return null;
}

/**
 * Whole months since a timestamp, floored at the server's 365-day Inactive
 * threshold. Returns null when the timestamp is absent or unparseable.
 */
function monthsSince(iso: string | undefined, now: Date): number | null {
  if (!iso) return null;
  const then = Date.parse(iso);
  if (Number.isNaN(then)) return null;
  const days = Math.max(0, Math.floor((now.getTime() - then) / DAY_MS));
  return Math.max(MIN_INACTIVE_MONTHS, Math.floor(days / 30));
}
