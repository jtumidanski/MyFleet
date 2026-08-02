import { Button } from '../ui/button';
import { Card } from '../ui/card';

/**
 * What a purge would delete, per domain, with the purge control beneath it.
 *
 * The counts come from the server's BlastRadius, which is literally the same
 * Count the purge's Stamp will use — so these figures and the rows actually
 * taken are equal by construction rather than by two queries that agree today
 * (FR-ADMIN-UI-9).
 *
 * When the counts cannot be computed the control is WITHHELD, not disabled: a
 * live destructive button above numbers nobody could produce is the worst state
 * this screen can be in.
 */
export interface BlastRadiusPanelProps {
  counts: Record<string, number> | undefined;
  error?: boolean;
  fleetName: string;
  onPurge: () => void;
  disabled?: boolean;
}

/** Domain keys in a fixed order, so the panel does not reshuffle between fetches. */
const ORDER = [
  'vehicles',
  'maintenance_records',
  'maintenance_schedules',
  'fuel_logs',
  'mileage_records',
  'activity_events',
  'memberships',
  'invites',
  'dashboards',
  'vehicle_media',
];

const LABELS: Record<string, string> = {
  vehicles: 'Vehicles',
  maintenance_records: 'Maintenance records',
  maintenance_schedules: 'Maintenance schedules',
  fuel_logs: 'Fuel logs',
  mileage_records: 'Mileage records',
  activity_events: 'Activity events',
  memberships: 'Memberships',
  invites: 'Invites',
  dashboards: 'Dashboards',
  vehicle_media: 'Vehicle media',
  fleets: 'Fleets',
};

function humanise(key: string): string {
  return LABELS[key] ?? key.replace(/_/g, ' ');
}

export function BlastRadiusPanel({
  counts,
  error,
  fleetName,
  onPurge,
  disabled,
}: BlastRadiusPanelProps) {
  if (error || !counts) {
    return (
      <Card className="border-danger-border p-4">
        <h2 className="text-lg font-semibold">Delete this fleet</h2>
        <div
          role="alert"
          className="mt-2 rounded border border-danger-border bg-danger-subtle p-3 text-sm text-danger-subtle-foreground"
        >
          We could not work out what deleting this fleet would remove, so the purge control is
          unavailable. Reload to try again.
        </div>
      </Card>
    );
  }

  // Keys the server sent that this build does not know about still render,
  // appended after the known order — a count omitted from the list would
  // understate the blast radius, which is the one error that matters here.
  const known = ORDER.filter((k) => k in counts);
  const extra = Object.keys(counts)
    .filter((k) => !ORDER.includes(k))
    .sort();
  const keys = [...known, ...extra];
  const total = keys.reduce((sum, k) => sum + (counts[k] ?? 0), 0);

  return (
    <Card className="border-danger-border p-4">
      <h2 className="text-lg font-semibold">Delete this fleet</h2>
      <p className="mt-1 text-sm text-muted-foreground">
        Deleting {fleetName} would remove {total} record{total === 1 ? '' : 's'}:
      </p>
      <dl className="mt-3 grid gap-2 sm:grid-cols-2">
        {keys.map((k) => (
          <div key={k} className="flex items-baseline justify-between gap-4" data-testid={`radius-${k}`}>
            <dt className="text-sm text-muted-foreground">{humanise(k)}</dt>
            <dd className="text-sm font-semibold tabular-nums">{counts[k]}</dd>
          </div>
        ))}
      </dl>
      <div className="mt-4">
        <Button type="button" variant="destructive" onClick={onPurge} disabled={disabled}>
          Purge this fleet
        </Button>
      </div>
    </Card>
  );
}
