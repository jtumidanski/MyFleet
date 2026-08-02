import type { ReactNode } from 'react';
import { Card } from '../../components/ui/card';
import { Skeleton } from '../../components/ui/skeleton';
import { useAdminStats } from '../../lib/hooks/api/admin';
import type { AdminStatsAttributes } from '../../types/models/admin';

/**
 * Platform overview — solution-wide counts (FR-ADMIN-STATS-1).
 *
 * The one rule that is easy to get wrong: a null count renders as an em dash
 * with the reason beneath it, NEVER as 0 (FR-ADMIN-UI-6). Zero says "there is no
 * data"; the em dash says "we could not ask". Those are different facts, and an
 * operator about to purge needs to tell them apart.
 */

type CountKey = Exclude<keyof AdminStatsAttributes, 'vehicles' | 'warnings'>;

const TILES: Array<{ key: CountKey; label: string }> = [
  { key: 'fleets', label: 'Fleets' },
  { key: 'users', label: 'Users' },
  { key: 'memberships', label: 'Memberships' },
  { key: 'pending_invites', label: 'Pending invites' },
  { key: 'maintenance_records', label: 'Maintenance records' },
  { key: 'maintenance_schedules', label: 'Maintenance schedules' },
  { key: 'fuel_logs', label: 'Fuel logs' },
  { key: 'mileage_records', label: 'Mileage records' },
  { key: 'activity_events', label: 'Activity events' },
  { key: 'media_objects', label: 'Media objects' },
  { key: 'notifications', label: 'Notifications' },
];

/**
 * The warning that explains a particular missing count.
 *
 * Matched on the attribute key, which is what the server's warning text names
 * ("… ; notifications count omitted"). Showing the reason under the exact tile
 * beats a banner alone: the banner says something is wrong, the tile says which
 * number you cannot trust.
 */
function reasonFor(key: string, warnings: string[]): string | undefined {
  return warnings.find((w) => w.includes(key));
}

function StatTile({
  statKey,
  label,
  value,
  warnings,
  children,
}: {
  statKey: string;
  label: string;
  value: number | null;
  warnings: string[];
  children?: ReactNode;
}) {
  const unavailable = value === null || value === undefined;
  const reason = unavailable ? reasonFor(statKey, warnings) : undefined;

  return (
    <Card className="p-4" data-testid={`stat-${statKey}`}>
      <div className="text-sm text-muted-foreground">{label}</div>
      <div className="text-3xl font-semibold">{unavailable ? '—' : value}</div>
      {reason ? <div className="mt-1 text-xs text-muted-foreground">{reason}</div> : null}
      {children}
    </Card>
  );
}

export function AdminOverviewPage() {
  const { data, isLoading, isError } = useAdminStats();

  if (isLoading) {
    return (
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {TILES.map((t) => (
          <Skeleton key={t.key} className="h-24" />
        ))}
      </div>
    );
  }

  if (isError || !data) {
    return (
      <div
        role="alert"
        className="rounded-lg border border-danger-border bg-danger-subtle p-4 text-sm text-danger-subtle-foreground"
      >
        Could not load platform statistics.
      </div>
    );
  }

  const stats = data.attributes;
  const warnings = stats.warnings ?? [];

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold">Platform overview</h1>
        <p className="text-sm text-muted-foreground">
          Every fleet on this platform, counted. Soft-deleted rows are excluded, so a pending purge
          is reflected here immediately.
        </p>
      </div>

      {/*
        role="status", not role="alert": a source being unreachable degrades the
        page, it does not break it. The counts that DID arrive are still true and
        still usable (FR-ADMIN-STATS-5).
      */}
      {warnings.length > 0 ? (
        <div
          role="status"
          className="rounded-lg border border-warning-border bg-warning-subtle p-3 text-sm text-warning-subtle-foreground"
        >
          <p className="font-medium">Some counts are unavailable.</p>
          <ul className="mt-1 list-inside list-disc">
            {warnings.map((w) => (
              <li key={w}>{w}</li>
            ))}
          </ul>
        </div>
      ) : null}

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <StatTile
          statKey="vehicles"
          label="Vehicles"
          value={stats.vehicles.active}
          warnings={warnings}
        >
          {/*
            Pending-purge sits in the vehicle tile rather than its own, because
            it is what the console can still UNDO — it belongs next to the number
            it will become (FR-ADMIN-STATS-3).
          */}
          {stats.vehicles.pending_purge > 0 ? (
            <div className="mt-1 text-xs text-muted-foreground">
              {stats.vehicles.pending_purge} pending purge
            </div>
          ) : null}
        </StatTile>

        {TILES.map((tile) => (
          <StatTile
            key={tile.key}
            statKey={tile.key}
            label={tile.label}
            value={stats[tile.key]}
            warnings={warnings}
          />
        ))}
      </div>
    </div>
  );
}
