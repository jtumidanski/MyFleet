import { useState } from 'react';
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
import { useAuditEvents } from '../../lib/hooks/api/admin';

/**
 * The audit log (FR-ADMIN-AUDIT-1/3).
 *
 * Append-only server-side and read-only here: there is no API to modify or
 * delete these rows, and a system purge deliberately does not erase them.
 */

const ACTIONS = [
  { value: '', label: 'All' },
  { value: 'purge.created', label: 'Created' },
  { value: 'purge.cancelled', label: 'Restored' },
  { value: 'purge.retried', label: 'Retried' },
  { value: 'purge.reaped', label: 'Deleted for good' },
  { value: 'vehicle.transferred', label: 'Transferred' },
];

// Kept in step with ACTIONS above by AdminAuditPage.test.tsx: the badge has a
// `?? a.action` fallback, so an omission here degrades quietly to a raw action
// string instead of failing.
const ACTION_LABELS: Record<string, string> = {
  'purge.created': 'Created',
  'purge.cancelled': 'Restored',
  'purge.retried': 'Retried',
  'purge.reaped': 'Deleted for good',
  'vehicle.transferred': 'Transferred',
};

/** ActorSystem — the value the reaper writes rather than a user id. */
const SYSTEM_ACTOR = 'system';

export function AdminAuditPage() {
  const [action, setAction] = useState('');
  const [actor, setActor] = useState('');
  const { data, isLoading, isError } = useAuditEvents({ action, actor, page: 1 });

  return (
    <div className="space-y-4">
      <div>
        <h1 className="text-2xl font-semibold">Audit log</h1>
        <p className="text-sm text-muted-foreground">
          Every administrative action, newest first. This log survives a system purge.
        </p>
      </div>

      <div className="flex flex-wrap items-center gap-1">
        {ACTIONS.map((a) => (
          <Button
            key={a.value || 'all'}
            type="button"
            size="sm"
            variant={action === a.value ? 'default' : 'outline'}
            onClick={() => setAction(a.value)}
          >
            {a.label}
          </Button>
        ))}
        {/*
          Filters on actor_user_id, which is what the server matches — the id is
          also what the reaper writes ("system"), so this is the way to isolate
          scheduled deletions from human ones.
        */}
        <Input
          className="ml-2 w-56"
          placeholder="Filter by actor user id"
          aria-label="Filter by actor user id"
          value={actor}
          onChange={(e) => setActor(e.target.value)}
        />
      </div>

      <Card className="p-4">
        {isLoading ? (
          <div className="space-y-2">
            <Skeleton className="h-10" />
            <Skeleton className="h-10" />
          </div>
        ) : isError || !data ? (
          <p role="alert" className="text-sm text-danger-subtle-foreground">
            Could not load the audit log.
          </p>
        ) : data.data.length === 0 ? (
          <p className="text-sm text-muted-foreground">No audit events yet.</p>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>When</TableHead>
                <TableHead>Action</TableHead>
                <TableHead>Actor</TableHead>
                <TableHead>Target</TableHead>
                <TableHead>Correlation</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {data.data.map((ev) => {
                const a = ev.attributes;
                const isSystem = a.actor_user_id === SYSTEM_ACTOR;
                return (
                  <TableRow key={ev.id} data-testid={`audit-${ev.id}`}>
                    <TableCell className="whitespace-nowrap text-sm">
                      {new Date(a.created_at).toLocaleString()}
                    </TableCell>
                    <TableCell>
                      <Badge variant="secondary">{ACTION_LABELS[a.action] ?? a.action}</Badge>
                    </TableCell>
                    {/*
                      The reaper's rows are attributed to the system, not to the
                      admin who requested the purge days earlier — attributing a
                      scheduled deletion to a person misreads the trail
                      (FR-ADMIN-UI-13).
                    */}
                    <TableCell className={isSystem ? 'text-sm text-muted-foreground' : 'text-sm'}>
                      {isSystem ? SYSTEM_ACTOR : a.actor_email}
                    </TableCell>
                    <TableCell className="text-sm">{a.target_label}</TableCell>
                    {/*
                      The correlation id is what ties this row back to the
                      service logs for the same request, which is the only way to
                      reconstruct why a purge behaved the way it did.
                    */}
                    <TableCell className="font-mono text-xs">{a.correlation_id}</TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        )}
      </Card>
    </div>
  );
}
