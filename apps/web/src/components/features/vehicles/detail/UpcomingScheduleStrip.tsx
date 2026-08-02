import { Card, CardContent, CardHeader, CardTitle } from '../../../ui/card';
import { Button } from '../../../ui/button';
import { SeverityChip } from '../maintenance/SeverityChip';
import { cn } from '../../../../lib/utils';
import { rankSchedule } from '../../../../lib/vehicleStats';
import type { MaintenanceSchedule } from '../../../../types/models/maintenanceSchedule';

interface UpcomingScheduleStripProps {
  schedules: MaintenanceSchedule[];
  /**
   * categoryId -> display name. Resolved by the caller (which already holds
   * the category list for the rest of the page) so this component stays
   * presentational and doesn't issue its own fetch.
   */
  categoryNames: Map<string, string>;
  canWrite: boolean;
  onAddSchedule: () => void;
  onComplete: (schedule: MaintenanceSchedule) => void;
}

function recurrenceLine(schedule: MaintenanceSchedule): string {
  const { recurrenceType, intervalMonths, intervalMiles } = schedule.attributes;
  const parts: string[] = [];
  if (intervalMonths) {
    parts.push(`every ${intervalMonths} month${intervalMonths === 1 ? '' : 's'}`);
  }
  if (intervalMiles) {
    parts.push(`every ${intervalMiles.toLocaleString()} miles`);
  }
  return parts.length > 0 ? parts.join(' · ') : recurrenceType;
}

/**
 * Upcoming/overdue maintenance schedules, overdue first. Ranked by
 * `rankSchedule` (Task 9) rather than a local switch, and colored by
 * `status` — never `severity`, a different vocabulary answering a different
 * question (how much a schedule matters vs. how due it is).
 */
export function UpcomingScheduleStrip({
  schedules,
  categoryNames,
  canWrite,
  onAddSchedule,
  onComplete,
}: UpcomingScheduleStripProps) {
  const sorted = [...schedules].sort((a, b) => rankSchedule(a) - rankSchedule(b));

  return (
    <Card>
      <CardHeader className="flex-row items-center justify-between space-y-0">
        <CardTitle className="text-base font-semibold">Upcoming maintenance</CardTitle>
        {canWrite && (
          <Button type="button" size="sm" onClick={onAddSchedule}>
            Add schedule
          </Button>
        )}
      </CardHeader>
      <CardContent>
        {sorted.length === 0 ? (
          <p className="text-sm text-muted-foreground">No maintenance schedules defined.</p>
        ) : (
          <div className="grid grid-cols-[repeat(auto-fit,minmax(210px,1fr))] gap-3">
            {sorted.map((schedule) => {
              const status = schedule.attributes.status;
              const toneClass =
                status === 'overdue'
                  ? 'border-danger-border bg-danger-subtle/45'
                  : status === 'upcoming'
                    ? 'border-warning-border bg-warning-subtle/45'
                    : '';
              const name =
                categoryNames.get(schedule.attributes.categoryId) ?? schedule.attributes.categoryId;
              return (
                <div key={schedule.id} className={cn('space-y-2 rounded-lg border p-3', toneClass)}>
                  <div className="flex items-center justify-between gap-2">
                    <span className="truncate text-sm font-medium">{name}</span>
                    <SeverityChip severity={schedule.attributes.severity} />
                  </div>
                  <p className="text-xs text-muted-foreground">{recurrenceLine(schedule)}</p>
                  {canWrite && (
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      onClick={() => onComplete(schedule)}
                    >
                      Complete
                    </Button>
                  )}
                </div>
              );
            })}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
