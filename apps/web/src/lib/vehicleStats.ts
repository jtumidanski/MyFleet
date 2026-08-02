import type { VehicleRecordRow } from './vehicleRecords';
import type { MaintenanceSchedule } from '../types/models/maintenanceSchedule';

/**
 * Every derivation here returns undefined rather than a placeholder number
 * when its inputs are missing. The stat tiles render an em dash for undefined;
 * a confident-looking 0 would be worse than an obvious gap.
 */

/** Newest odometer reading, falling back to the vehicle's stored mileage. */
export function deriveOdometer(
  mileageRows: VehicleRecordRow[],
  currentMileage?: number,
): number | undefined {
  const newest = mileageRows
    .filter((r) => typeof r.mileage === 'number')
    .reduce<VehicleRecordRow | undefined>(
      (best, r) => (best == null || r.date > best.date ? r : best),
      undefined,
    );
  return newest?.mileage ?? currentMileage;
}

/** Sum of every cost in the trailing twelve months. */
export function deriveTrailingCost(rows: VehicleRecordRow[], now: Date): number {
  const cutoff = new Date(now);
  cutoff.setFullYear(cutoff.getFullYear() - 1);
  const cutoffIso = cutoff.toISOString();

  return rows.reduce((sum, r) => (r.date >= cutoffIso ? sum + (r.cost ?? 0) : sum), 0);
}

/**
 * Average miles per gallon across consecutive fill-ups.
 *
 * A tank's gallons pay for the distance travelled SINCE the previous
 * fill-up: iterating oldest-first, pair i's gallons are attributed to
 * mileage[i] - mileage[i-1]. The oldest reading contributes distance but
 * no gallons of its own, which is why two fill-ups are the minimum.
 *
 * A non-advancing odometer between consecutive fill-ups is bad data, not
 * zero economy, so that pair is skipped entirely (both its miles and its
 * gallons are dropped) rather than folded in as a zero-mile leg.
 */
export function deriveAvgEconomy(fuelRows: VehicleRecordRow[]): number | undefined {
  const usable = fuelRows
    .filter((r) => typeof r.mileage === 'number' && typeof r.gallons === 'number')
    .sort((a, b) => (a.date < b.date ? -1 : a.date > b.date ? 1 : 0));

  if (usable.length < 2) return undefined;

  let miles = 0;
  let gallons = 0;
  for (let i = 1; i < usable.length; i += 1) {
    const current = usable[i] as VehicleRecordRow;
    const previous = usable[i - 1] as VehicleRecordRow;
    const delta = (current.mileage as number) - (previous.mileage as number);
    if (delta <= 0) continue; // a non-advancing odometer is bad data, not zero economy
    miles += delta;
    gallons += current.gallons as number;
  }

  if (miles <= 0 || gallons <= 0) return undefined;
  return miles / gallons;
}

export interface NextService {
  label: string;
  severity: 'ok' | 'warning' | 'danger';
}

/**
 * The most urgent schedule, expressed as distance or time remaining.
 * Urgency comes from `rankSchedule`/`severityOf`, both driven by `status`
 * (how due the schedule is), never `severity` (how much it matters) — see
 * the note on `severityOf` for why those two fields must not be confused.
 */
export function deriveNextService(
  schedules: MaintenanceSchedule[],
  odometer: number | undefined,
  now: Date,
): NextService | undefined {
  if (schedules.length === 0) return undefined;

  const ranked = [...schedules].sort((a, b) => rankSchedule(a) - rankSchedule(b));
  // schedules.length === 0 returned above, and sort preserves length, so
  // ranked[0] is always present.
  const next = ranked[0] as MaintenanceSchedule;
  const severity = severityOf(next);

  const dueMileage = next.attributes.nextDueMileage;
  if (typeof dueMileage === 'number' && typeof odometer === 'number') {
    const remaining = dueMileage - odometer;
    return {
      label:
        remaining >= 0
          ? `${remaining.toLocaleString()} mi`
          : `${Math.abs(remaining).toLocaleString()} mi over`,
      severity,
    };
  }

  const dueDate = next.attributes.nextDueDate;
  if (dueDate) {
    const days = Math.round((new Date(dueDate).getTime() - now.getTime()) / 86_400_000);
    return {
      label: days >= 0 ? `${days} days` : `${Math.abs(days)} days over`,
      severity,
    };
  }

  return undefined;
}

/**
 * Urgency comes from `status`, NOT `severity`. They are different
 * vocabularies on the same resource: `status` is 'ok' | 'upcoming' |
 * 'overdue' (how due it is) while `severity` is 'urgent' | 'recommended' |
 * 'informational' (how much it matters, independent of timing). Only
 * `status` answers "what is due next" — an 'urgent' schedule that is not
 * yet due must not render as danger, and an 'informational' schedule that
 * is overdue must.
 */
function severityOf(s: MaintenanceSchedule): NextService['severity'] {
  switch (s.attributes.status) {
    case 'overdue':
      return 'danger';
    case 'upcoming':
      return 'warning';
    default:
      return 'ok';
  }
}

/**
 * Overdue first, then upcoming, then everything else. Ranks by `status`,
 * for the same reason `severityOf` reads `status`. Exported so Task 13's
 * schedule strip sorts by the same order without duplicating the switch.
 */
export function rankSchedule(s: MaintenanceSchedule): number {
  switch (s.attributes.status) {
    case 'overdue':
      return 0;
    case 'upcoming':
      return 1;
    default:
      return 2;
  }
}
