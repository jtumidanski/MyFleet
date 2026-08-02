import { useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { Badge } from '../../components/ui/badge';
import { Button } from '../../components/ui/button';
import { Card } from '../../components/ui/card';
import { Input } from '../../components/ui/input';
import { Skeleton } from '../../components/ui/skeleton';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '../../components/ui/table';
import { BlastRadiusPanel } from '../../components/admin/BlastRadiusPanel';
import { PurgeConfirmDialog } from '../../components/admin/PurgeConfirmDialog';
import { useAdminFleet, useAdminFleets, useCreatePurge } from '../../lib/hooks/api/admin';
import { cn } from '../../lib/utils';
import type { DeletedFilter } from '../../types/models/admin';

/**
 * The fleet inspector — list and detail as two panes of one grid.
 *
 * Above md both panes render. Below md the grid collapses to one column and
 * only one pane is shown at a time, with a "← All fleets" link out of the
 * detail view (FR-ADMIN-UI-7).
 *
 * Fleets pending purge are struck through with a countdown rather than
 * vanishing — they are in the default result set because ?deleted= defaults to
 * include, which is the whole point: a console whose recovery window is
 * invisible by default hides the thing it exists to let you undo.
 */

const FILTERS: Array<{ value: DeletedFilter; label: string }> = [
  { value: 'include', label: 'All' },
  { value: 'exclude', label: 'Live only' },
  { value: 'only', label: 'Pending purge' },
];

/**
 * Whole days until the deadline, rounded up, floored at zero.
 *
 * Whole days rather than hours: the operator's decision is "do I still have
 * time", not "how many hours exactly", and the absolute deadline is available on
 * hover for when the precise answer matters.
 */
function daysLeft(purgeAfter: string): number {
  const ms = new Date(purgeAfter).getTime() - Date.now();
  return Math.max(0, Math.ceil(ms / (24 * 60 * 60 * 1000)));
}

/**
 * The deadline a purge started right now would carry.
 *
 * The server is authoritative — it stamps purge_after from its own clock and
 * its own configured window — so this is only what the dialog SHOWS before the
 * operation exists. It uses the same 5 days the service defaults to; a
 * deployment that configures a different window will differ by the drift
 * between them for the few seconds before the real value comes back.
 */
const RECOVERY_WINDOW_DAYS = 5;

function recoveryDeadline(): string {
  return new Date(Date.now() + RECOVERY_WINDOW_DAYS * 24 * 60 * 60 * 1000).toISOString();
}

function CountdownChip({ purgeAfter }: { purgeAfter: string }) {
  const days = daysLeft(purgeAfter);
  const absolute = new Intl.DateTimeFormat(undefined, {
    dateStyle: 'long',
    timeStyle: 'short',
  }).format(new Date(purgeAfter));
  return (
    <Badge variant="warning" title={`Deleted for good on ${absolute}`}>
      {days} day{days === 1 ? '' : 's'} left
    </Badge>
  );
}

function FleetList({
  selectedId,
  q,
  setQ,
  deleted,
  setDeleted,
}: {
  selectedId?: string;
  q: string;
  setQ: (v: string) => void;
  deleted: DeletedFilter;
  setDeleted: (v: DeletedFilter) => void;
}) {
  const { data, isLoading, isError } = useAdminFleets({ q, deleted, page: 1 });

  return (
    <Card className="p-4" data-testid="fleet-list">
      <h2 className="text-lg font-semibold">Fleets</h2>
      {/*
        The label says "on this page" deliberately. Owner email is matched after
        the auth-service lookup, over the fetched page only — emails do not live
        in fleet-service's database and a cross-service join is forbidden, so a
        global email search is not something this box can deliver.
      */}
      <Input
        className="mt-3"
        placeholder="Fleet name, or owner email on this page"
        aria-label="Search fleets"
        value={q}
        onChange={(e) => setQ(e.target.value)}
      />
      <div className="mt-3 flex gap-1">
        {FILTERS.map((f) => (
          <Button
            key={f.value}
            type="button"
            size="sm"
            variant={deleted === f.value ? 'default' : 'outline'}
            onClick={() => setDeleted(f.value)}
          >
            {f.label}
          </Button>
        ))}
      </div>

      {isLoading ? (
        <div className="mt-4 space-y-2">
          <Skeleton className="h-10" />
          <Skeleton className="h-10" />
        </div>
      ) : isError || !data ? (
        <p role="alert" className="mt-4 text-sm text-danger-subtle-foreground">
          Could not load fleets.
        </p>
      ) : data.data.length === 0 ? (
        <p className="mt-4 text-sm text-muted-foreground">No fleets match.</p>
      ) : (
        <ul className="mt-4 space-y-1">
          {data.data.map((f) => (
            <li key={f.id}>
              <Link
                to={`/admin/fleets/${f.id}`}
                className={cn(
                  'block rounded px-3 py-2 text-sm hover:bg-accent hover:text-accent-foreground',
                  selectedId === f.id && 'bg-accent text-accent-foreground',
                )}
              >
                <span
                  className={cn(
                    'font-medium',
                    f.attributes.pending_purge && 'line-through text-muted-foreground',
                  )}
                >
                  {f.attributes.name}
                </span>
                <span className="ml-2 text-xs text-muted-foreground">
                  {f.attributes.vehicle_count} vehicles · {f.attributes.member_count} members
                </span>
                {f.attributes.pending_purge && f.attributes.purge_after ? (
                  <span className="ml-2 inline-block">
                    <CountdownChip purgeAfter={f.attributes.purge_after} />
                  </span>
                ) : null}
              </Link>
            </li>
          ))}
        </ul>
      )}
    </Card>
  );
}

function FleetDetail({ id }: { id: string }) {
  const { data, isLoading, isError } = useAdminFleet(id);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const createPurge = useCreatePurge();

  if (isLoading) {
    return <Skeleton className="h-64" data-testid="fleet-detail-loading" />;
  }
  if (isError || !data) {
    return (
      <p role="alert" className="text-sm text-danger-subtle-foreground">
        Could not load this fleet.
      </p>
    );
  }

  const fleet = data.attributes;

  return (
    <div className="space-y-6" data-testid="fleet-detail">
      <div className="md:hidden">
        <Link to="/admin/fleets" className="text-sm text-muted-foreground hover:underline">
          ← All fleets
        </Link>
      </div>

      <div>
        <h2
          className={cn(
            'text-2xl font-semibold',
            fleet.pending_purge && 'line-through text-muted-foreground',
          )}
        >
          {fleet.name}
        </h2>
        <p className="text-sm text-muted-foreground">
          Owner: {fleet.owner_display_name || fleet.owner_email || fleet.owner_user_id}
        </p>
        {fleet.pending_purge && fleet.purge_after ? (
          <div className="mt-2">
            <CountdownChip purgeAfter={fleet.purge_after} />
          </div>
        ) : null}
      </div>

      {fleet.warnings?.length ? (
        <div
          role="status"
          className="rounded-lg border border-warning-border bg-warning-subtle p-3 text-sm text-warning-subtle-foreground"
        >
          {fleet.warnings.join(' ')}
        </div>
      ) : null}

      <Card className="p-4">
        <h3 className="text-lg font-semibold">Members</h3>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Member</TableHead>
              <TableHead>Role</TableHead>
              <TableHead />
            </TableRow>
          </TableHeader>
          <TableBody>
            {fleet.members.map((m) => (
              <TableRow key={m.user_id} data-testid={`member-${m.user_id}`}>
                <TableCell>{m.display_name || m.email || m.user_id}</TableCell>
                <TableCell>
                  <Badge variant="secondary">{m.role}</Badge>
                </TableCell>
                <TableCell className="text-right">
                  {/*
                    FR-ADMIN-UI-8: the owner's remove action is permanently
                    inert. A fleet must never lose its only owner, and offering
                    an action the server will refuse is worse than not offering
                    it — the title says why rather than leaving a dead button.
                  */}
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    disabled={m.role === 'owner'}
                    title={
                      m.role === 'owner'
                        ? 'A fleet cannot lose its owner. Transfer ownership first.'
                        : undefined
                    }
                  >
                    Remove
                  </Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </Card>

      <Card className="p-4">
        <h3 className="text-lg font-semibold">Vehicles</h3>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Vehicle</TableHead>
              <TableHead>Mileage</TableHead>
              <TableHead>Status</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {fleet.vehicles.map((v) => (
              <TableRow key={v.id} data-testid={`vehicle-${v.id}`}>
                <TableCell className={cn(v.pending_purge && 'line-through text-muted-foreground')}>
                  {v.nickname || `${v.year} ${v.make} ${v.model}`}
                </TableCell>
                <TableCell className="tabular-nums">{v.mileage}</TableCell>
                <TableCell>
                  {v.pending_purge ? (
                    <Badge variant="warning">Pending purge</Badge>
                  ) : v.status ? (
                    <Badge variant="secondary">{v.status}</Badge>
                  ) : null}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </Card>

      {fleet.invites.length > 0 ? (
        <Card className="p-4">
          <h3 className="text-lg font-semibold">Pending invites</h3>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Email</TableHead>
                <TableHead>Role</TableHead>
                <TableHead>Expires</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {fleet.invites.map((i) => (
                <TableRow key={i.id}>
                  <TableCell>{i.email}</TableCell>
                  <TableCell>{i.role}</TableCell>
                  <TableCell>{new Date(i.expires_at).toLocaleDateString()}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Card>
      ) : null}

      <BlastRadiusPanel
        counts={fleet.counts}
        error={!fleet.counts}
        fleetName={fleet.name}
        onPurge={() => setConfirmOpen(true)}
        disabled={fleet.pending_purge || createPurge.isPending}
      />

      {/*
        The phrase is the fleet's own name, which is why the label capture in
        the API matters: the operator has to read what they are about to destroy
        in order to type it.
      */}
      <PurgeConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        scope="fleet"
        confirmationPhrase={fleet.name}
        counts={fleet.counts ?? {}}
        peopleCount={fleet.members.length}
        recoveryDeadline={recoveryDeadline()}
        isPending={createPurge.isPending}
        onConfirm={() => {
          createPurge.mutate(
            { scope: 'fleet', target_type: 'fleet', target_id: id, confirmation: fleet.name },
            { onSuccess: () => setConfirmOpen(false) },
          );
        }}
      />
    </div>
  );
}

export function AdminFleetsPage() {
  const { id } = useParams<{ id: string }>();
  const [q, setQ] = useState('');
  const [deleted, setDeleted] = useState<DeletedFilter>('include');

  return (
    <div className="space-y-4">
      <h1 className="text-2xl font-semibold">Fleets</h1>
      {/*
        One grid whose columns collapse. Below md only one pane is shown, which
        is why each carries its own hidden/block classes rather than the grid
        rendering both and letting them wrap.
      */}
      <div className="grid gap-6 md:grid-cols-[320px_1fr]" data-testid="fleet-inspector">
        <div className={cn(id && 'hidden md:block')}>
          <FleetList selectedId={id} q={q} setQ={setQ} deleted={deleted} setDeleted={setDeleted} />
        </div>
        <div className={cn(!id && 'hidden md:block')}>
          {id ? (
            <FleetDetail id={id} />
          ) : (
            <p className="text-sm text-muted-foreground">Select a fleet to inspect it.</p>
          )}
        </div>
      </div>
    </div>
  );
}
