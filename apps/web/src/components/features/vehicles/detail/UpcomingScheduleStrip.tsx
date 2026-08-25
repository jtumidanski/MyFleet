import { Card, CardContent, CardHeader, CardTitle } from '../../../ui/card';
import { Button } from '../../../ui/button';
import { ScheduleCard } from '../maintenance/ScheduleCard';
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
  onConvert: (schedule: MaintenanceSchedule) => void;
}

/** Epoch millis of a completion, or 0 when there is none. */
function completedAt(schedule: MaintenanceSchedule): number {
  const value = schedule.attributes.lastCompletedDate;
  return value ? new Date(value).getTime() : 0;
}

/**
 * Two tiers, in this order:
 *  1. active schedules, ranked by `rankSchedule` (overdue → upcoming → ok)
 *  2. inactive (completed) schedules, most recently completed first
 *
 * The tier check comes FIRST and is not folded into `rankSchedule`, because
 * `rankSchedule` reads `status` — which describes a cycle that is over for a
 * deactivated row. A completed one-time schedule whose last stored status was
 * 'overdue' would otherwise sort above every live schedule on the page.
 */
function compareSchedules(a: MaintenanceSchedule, b: MaintenanceSchedule): number {
  const aActive = a.attributes.active;
  const bActive = b.attributes.active;
  if (aActive !== bActive) return aActive ? -1 : 1;
  if (!aActive) return completedAt(b) - completedAt(a);
  return rankSchedule(a) - rankSchedule(b);
}

/**
 * The vehicle's maintenance schedules — active ones first, ranked by how due
 * they are, with completed one-offs settling underneath. Despite the name this
 * is the whole schedule list, not just the upcoming slice: the caller feeds it
 * `ListByVehicle`, which includes inactive rows.
 */
export function UpcomingScheduleStrip({
  schedules,
  categoryNames,
  canWrite,
  onAddSchedule,
  onComplete,
  onConvert,
}: UpcomingScheduleStripProps) {
  const sorted = [...schedules].sort(compareSchedules);

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
            {sorted.map((schedule) => (
              <ScheduleCard
                key={schedule.id}
                schedule={schedule}
                categoryName={
                  categoryNames.get(schedule.attributes.categoryId) ??
                  schedule.attributes.categoryId
                }
                canWrite={canWrite}
                onComplete={onComplete}
                onConvert={onConvert}
              />
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
