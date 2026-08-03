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
 * gallons are dropped) rather than folded in as a zero-mile leg. Two equal
 * readings (delta === 0, e.g. two fill-ups logged the same day with no
 * driving between them) invalidate only that one leg — the readings agree
 * with each other, so there's no reason to distrust the next one. A
 * *decreasing* reading (delta < 0) is different: the two readings disagree
 * about where the odometer was, so which one is wrong is genuinely
 * ambiguous, and the leg immediately after also starts from that disputed
 * value — it is skipped too rather than measured off a reading we already
 * know is suspect.
 */
export function deriveAvgEconomy(fuelRows: VehicleRecordRow[]): number | undefined {
  const usable = fuelRows
    .filter((r) => typeof r.mileage === 'number' && typeof r.gallons === 'number')
    .sort((a, b) => (a.date < b.date ? -1 : a.date > b.date ? 1 : 0));

  if (usable.length < 2) return undefined;

  let miles = 0;
  let gallons = 0;
  let skipNextLeg = false;
  for (let i = 1; i < usable.length; i += 1) {
    const current = usable[i] as VehicleRecordRow;
    const previous = usable[i - 1] as VehicleRecordRow;
    const delta = (current.mileage as number) - (previous.mileage as number);
    if (delta < 0) {
      skipNextLeg = true; // the next leg shares this disputed endpoint
      continue;
    }
    if (delta === 0) {
      continue; // no distance travelled, but the reading itself isn't in question
    }
    if (skipNextLeg) {
      skipNextLeg = false;
      continue;
    }
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
 * Default "how close counts as close" windows, mirroring the backend's own
 * defaults (`DefaultThresholds` in
 * apps/fleet-service/internal/maintenanceschedule/recurrence.go: 30 days /
 * 500 mi). Per-org threshold config isn't exposed to the frontend via
 * `MaintenanceScheduleAttributes`, so these are fixed constants rather than
 * a passed-in setting — good enough to rank "due in 3 days" as more urgent
 * than "due in 5,000 miles" without needing the org's actual configured
 * window.
 */
const DUE_SOON_DAYS = 30;
const DUE_SOON_MILES = 500;

/** One schedule's due-ness on a single axis. Positive = not yet due; negative = overdue by this amount. */
interface DueAxis {
  kind: 'mileage' | 'date';
  remaining: number;
}

/**
 * Urgency score for one axis, normalized against its due-soon window so
 * mileage and time become comparable on one scale: overdue axes always
 * score above 1 (the further over, the higher), and not-yet-due axes score
 * at or below 1, with "inside the due-soon window" near 1 and "far off"
 * strongly negative. Taking the max across axes is what lets a hybrid
 * schedule's nearer dimension win regardless of which one it is.
 */
function urgency(axis: DueAxis): number {
  const threshold = axis.kind === 'mileage' ? DUE_SOON_MILES : DUE_SOON_DAYS;
  return axis.remaining < 0
    ? 1 + Math.abs(axis.remaining) / threshold
    : 1 - axis.remaining / threshold;
}

/**
 * The due-mileage and due-date axes available for one schedule, limited to
 * the data actually present: a mileage axis needs both `nextDueMileage`
 * and a known `odometer` (an odometer-less vehicle can't compute a mileage
 * remaining), and a date axis needs a `nextDueDate` that parses to a valid
 * instant (guards against literal `NaN` from a malformed date string).
 */
function dueAxes(
  schedule: MaintenanceSchedule,
  odometer: number | undefined,
  now: Date,
): DueAxis[] {
  const axes: DueAxis[] = [];

  const dueMileage = schedule.attributes.nextDueMileage;
  if (typeof dueMileage === 'number' && typeof odometer === 'number') {
    axes.push({ kind: 'mileage', remaining: dueMileage - odometer });
  }

  const dueDate = schedule.attributes.nextDueDate;
  if (dueDate) {
    const dueTime = new Date(dueDate).getTime();
    if (Number.isFinite(dueTime)) {
      const days = Math.round((dueTime - now.getTime()) / 86_400_000);
      axes.push({ kind: 'date', remaining: days });
    }
  }

  return axes;
}

function labelFor(axis: DueAxis): string {
  if (axis.kind === 'mileage') {
    return axis.remaining >= 0
      ? `${axis.remaining.toLocaleString()} mi`
      : `${Math.abs(axis.remaining).toLocaleString()} mi over`;
  }
  return axis.remaining >= 0 ? `${axis.remaining} days` : `${Math.abs(axis.remaining)} days over`;
}

/**
 * The most urgent schedule, expressed as whichever of distance or time
 * remaining is actually nearer to due. A `hybrid` schedule carries both
 * `nextDueMileage` and `nextDueDate`, so this is the ordinary case for
 * hybrid, not a corner: the far-off dimension must not upstage a soon one
 * (a schedule 5,000 miles from its mileage limit but due by date in three
 * days is due Wednesday, not "plenty of road left"). Ties prefer the
 * mileage axis, matching the backend's own tie-break in
 * apps/fleet-service/internal/vehicle/nextdue.go.
 *
 * Schedules are tried in `rankSchedule` order, falling through to the next
 * one if the current pick has no usable due data on either axis. This
 * matters because a freshly created schedule can reach the frontend before
 * the backend's hourly recompute has populated `nextDue*`/`status` — a
 * half-initialized top-ranked schedule must not blank the tile for every
 * other schedule that does have real numbers.
 */
export function deriveNextService(
  schedules: MaintenanceSchedule[],
  odometer: number | undefined,
  now: Date,
): NextService | undefined {
  const ranked = [...schedules].sort((a, b) => rankSchedule(a) - rankSchedule(b));

  for (const schedule of ranked) {
    const axes = dueAxes(schedule, odometer, now);
    if (axes.length === 0) continue;

    const nearest = axes.reduce((best, axis) => {
      const diff = urgency(axis) - urgency(best);
      if (diff !== 0) return diff > 0 ? axis : best;
      return axis.kind === 'mileage' ? axis : best; // tie: mileage wins
    });

    return { label: labelFor(nearest), severity: severityOf(schedule) };
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
