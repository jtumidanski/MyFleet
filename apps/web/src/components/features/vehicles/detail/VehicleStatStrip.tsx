import { formatMileage, formatMoney } from '@myfleet/ui-components';
import { cn } from '../../../../lib/utils';
import {
  deriveOdometer,
  deriveTrailingCost,
  deriveAvgEconomy,
  deriveNextService,
} from '../../../../lib/vehicleStats';
import type { VehicleRecordRow } from '../../../../lib/vehicleRecords';
import type { MaintenanceSchedule } from '../../../../types/models/maintenanceSchedule';

interface VehicleStatStripProps {
  rows: VehicleRecordRow[];
  schedules: MaintenanceSchedule[];
  currentMileage?: number;
  /** True when rows are a partial window, which softens the cost/economy tiles. */
  partial: boolean;
}

interface StatTileProps {
  label: string;
  value: string;
  subtitle?: string;
  valueClassName?: string;
}

function StatTile({ label, value, subtitle, valueClassName }: StatTileProps) {
  return (
    <div className="rounded-lg border bg-card p-4">
      <p className="text-xs font-medium text-muted-foreground">{label}</p>
      <p className={cn('mt-1 text-2xl font-semibold text-foreground', valueClassName)}>{value}</p>
      {subtitle && <p className="mt-1 text-xs text-muted-foreground">{subtitle}</p>}
    </div>
  );
}

/** Same trailing-twelve-months boundary as deriveTrailingCost, duplicated here only to count the records behind the number for the subtitle. */
function trailingCutoffIso(now: Date): string {
  const cutoff = new Date(now);
  cutoff.setFullYear(cutoff.getFullYear() - 1);
  return cutoff.toISOString();
}

/**
 * Four at-a-glance tiles: odometer, trailing cost, average economy, next
 * service due. Every value falls back to an em dash rather than a
 * confident-looking 0/undefined placeholder (see vehicleStats.ts's own
 * doc-comment on the same rule).
 *
 * `partial` softens the cost/economy subtitles to "based on recent records"
 * instead of a count — those two numbers are computed over whatever rows are
 * currently loaded, and while more pages remain unfetched a record count
 * would imply a completeness the number doesn't have.
 */
export function VehicleStatStrip({
  rows,
  schedules,
  currentMileage,
  partial,
}: VehicleStatStripProps) {
  const now = new Date();

  const odometer = deriveOdometer(rows, currentMileage);
  const trailingCost = deriveTrailingCost(rows, now);
  const fuelRows = rows.filter((r) => r.kind === 'fuel');
  const avgEconomy = deriveAvgEconomy(fuelRows);
  const nextService = deriveNextService(schedules, odometer, now);

  const cutoffIso = trailingCutoffIso(now);
  const costRecordCount = rows.filter(
    (r) => r.date >= cutoffIso && typeof r.cost === 'number',
  ).length;

  const nextServiceClass =
    nextService?.severity === 'danger'
      ? 'text-danger'
      : nextService?.severity === 'warning'
        ? 'text-warning'
        : undefined;

  return (
    <div className="grid grid-cols-[repeat(auto-fit,minmax(150px,1fr))] gap-3">
      <StatTile
        label="Odometer"
        value={typeof odometer === 'number' ? formatMileage(odometer) : '—'}
      />
      <StatTile
        label="Cost (12 mo)"
        value={formatMoney(trailingCost)}
        subtitle={
          partial
            ? 'based on recent records'
            : `${costRecordCount} record${costRecordCount === 1 ? '' : 's'}`
        }
      />
      <StatTile
        label="Avg economy"
        value={typeof avgEconomy === 'number' ? `${avgEconomy.toFixed(1)} mpg` : '—'}
        subtitle={
          partial
            ? 'based on recent records'
            : `${fuelRows.length} fill-up${fuelRows.length === 1 ? '' : 's'}`
        }
      />
      <StatTile
        label="Next service"
        value={nextService?.label ?? '—'}
        valueClassName={nextServiceClass}
      />
    </div>
  );
}
