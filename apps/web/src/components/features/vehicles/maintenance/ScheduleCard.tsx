import { Button } from '../../../ui/button';
import { SeverityChip } from './SeverityChip';
import { cn } from '../../../../lib/utils';
import type { MaintenanceSchedule } from '../../../../types/models/maintenanceSchedule';

interface ScheduleCardProps {
  schedule: MaintenanceSchedule;
  categoryName: string;
  canWrite: boolean;
  onComplete: (schedule: MaintenanceSchedule) => void;
  onConvert: (schedule: MaintenanceSchedule) => void;
}

/**
 * The recurrence line answers "when is this due, and does it repeat".
 *
 * A one-time schedule has no interval to state, so it states its due point
 * instead — which is also the only place in the card where `dueDate` /
 * `dueMileage` are read directly rather than through `nextDue*`. That is
 * deliberate: `nextDue*` is refreshed by the hourly recompute and can lag the
 * anchor the user just chose.
 */
function recurrenceLine(schedule: MaintenanceSchedule): string {
  const { recurrenceType, intervalMonths, intervalMiles, oneTime, dueDate, dueMileage } =
    schedule.attributes;

  const parts: string[] = [];
  if (oneTime) {
    if (dueDate) parts.push(`due ${new Date(dueDate).toLocaleDateString()}`);
    if (dueMileage) parts.push(`at ${dueMileage.toLocaleString()} miles`);
  } else {
    if (intervalMonths) {
      parts.push(`every ${intervalMonths} month${intervalMonths === 1 ? '' : 's'}`);
    }
    if (intervalMiles) {
      parts.push(`every ${intervalMiles.toLocaleString()} miles`);
    }
  }
  return parts.length > 0 ? parts.join(' · ') : recurrenceType;
}

/**
 * The completed line: date and odometer, both of which FR-COMPLETE-3 requires a
 * deactivated schedule to keep showing. Either may be absent — a schedule can
 * be completed with no odometer reading — so each is included only when set.
 */
function completedLine(schedule: MaintenanceSchedule): string {
  const { lastCompletedDate, lastCompletedMileage } = schedule.attributes;
  const parts: string[] = [];
  if (lastCompletedDate) parts.push(new Date(lastCompletedDate).toLocaleDateString());
  if (lastCompletedMileage) parts.push(`${lastCompletedMileage.toLocaleString()} miles`);
  return parts.length > 0 ? `Completed ${parts.join(' · ')}` : 'Completed';
}

/**
 * One maintenance schedule in the vehicle's schedule list.
 *
 * Three states, in the order the checks read:
 *  - inactive: de-emphasized, no tone border, a completion line, no Complete
 *    button, and — for a one-time schedule — a Set up recurrence action
 *    (FR-UI-3, FR-CONV-4's second entry point).
 *  - active one-time: a One-time badge and a stated due point (FR-UI-2).
 *  - active recurring: unchanged from before this task.
 *
 * Tone comes from `status`, never `severity`: they are different vocabularies
 * on the same resource — `status` is how due it is, `severity` is how much it
 * matters. Only `status` answers "is this urgent right now".
 */
export function ScheduleCard({
  schedule,
  categoryName,
  canWrite,
  onComplete,
  onConvert,
}: ScheduleCardProps) {
  const { status, severity, active, oneTime } = schedule.attributes;

  // A deactivated row's status describes a cycle that is over, so it gets no
  // tone at all rather than a stale one.
  const toneClass = !active
    ? 'opacity-70'
    : status === 'overdue'
      ? 'border-danger-border bg-danger-subtle/45'
      : status === 'upcoming'
        ? 'border-warning-border bg-warning-subtle/45'
        : '';

  return (
    <div className={cn('space-y-2 rounded-lg border p-3', toneClass)}>
      <div className="flex items-center justify-between gap-2">
        <span className={cn('truncate text-sm font-medium', !active && 'text-muted-foreground')}>
          {categoryName}
        </span>
        {active && <SeverityChip severity={severity} />}
      </div>

      {oneTime && (
        <span className="inline-flex items-center rounded-full border border-border px-2 py-0.5 text-xs font-medium text-muted-foreground">
          One-time
        </span>
      )}

      {active ? (
        <p className="text-xs text-muted-foreground">{recurrenceLine(schedule)}</p>
      ) : (
        <p className="text-xs text-muted-foreground">{completedLine(schedule)}</p>
      )}

      {canWrite && active && (
        <Button type="button" size="sm" variant="outline" onClick={() => onComplete(schedule)}>
          Complete
        </Button>
      )}

      {canWrite && !active && oneTime && (
        <Button type="button" size="sm" variant="outline" onClick={() => onConvert(schedule)}>
          Set up recurrence
        </Button>
      )}
    </div>
  );
}
