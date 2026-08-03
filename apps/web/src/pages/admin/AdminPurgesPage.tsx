import { useState } from 'react';
import { Badge } from '../../components/ui/badge';
import { Button } from '../../components/ui/button';
import { Card } from '../../components/ui/card';
import { Skeleton } from '../../components/ui/skeleton';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '../../components/ui/table';
import { useCancelPurge, usePurgeOperations, useRetryPurge } from '../../lib/hooks/api/admin';
import { purgeStatusLabel, purgeStatusVariant } from '../../lib/admin/purgeStatus';

/**
 * The purge queue.
 *
 * Every status is rendered through purgeStatusLabel — nothing here reads a raw
 * status string, so an operator never sees "partial" where they need to be told
 * which service did not finish (FR-ADMIN-UI-12).
 */

const STATUS_FILTERS = [
  { value: '', label: 'All' },
  { value: 'pending', label: 'Recoverable' },
  { value: 'partial', label: 'Incomplete' },
  { value: 'cancelled', label: 'Restored' },
  { value: 'reaped', label: 'Deleted for good' },
];

/** Whole days remaining, floored at zero. */
function daysLeft(iso: string): number {
  return Math.max(0, Math.ceil((new Date(iso).getTime() - Date.now()) / (24 * 60 * 60 * 1000)));
}

function absolute(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return new Intl.DateTimeFormat(undefined, { dateStyle: 'long', timeStyle: 'short' }).format(d);
}

export function AdminPurgesPage() {
  const [status, setStatus] = useState('');
  const { data, isLoading, isError } = usePurgeOperations({ status, page: 1 });
  const cancelPurge = useCancelPurge();
  const retryPurge = useRetryPurge();

  return (
    <div className="space-y-4">
      <div>
        <h1 className="text-2xl font-semibold">Purges</h1>
        <p className="text-sm text-muted-foreground">
          Every purge this platform has run. Recoverable ones can still be restored in full.
        </p>
      </div>

      <div className="flex flex-wrap gap-1">
        {STATUS_FILTERS.map((f) => (
          <Button
            key={f.value || 'all'}
            type="button"
            size="sm"
            variant={status === f.value ? 'default' : 'outline'}
            onClick={() => setStatus(f.value)}
          >
            {f.label}
          </Button>
        ))}
      </div>

      {/*
        The description is referenced by every retry button rather than repeated
        per row. FR-ADMIN-UI-11: retry has to READ as safe to repeat, or an
        operator will not press it after the first failure — and every downstream
        stamp is idempotent on purge_operation_id, so it genuinely is.
      */}
      <p id="retry-help" className="text-sm text-muted-foreground">
        Safe to run again — it only re-attempts the parts that did not finish.
      </p>

      <Card className="p-4">
        {isLoading ? (
          <div className="space-y-2">
            <Skeleton className="h-10" />
            <Skeleton className="h-10" />
          </div>
        ) : isError || !data ? (
          <p role="alert" className="text-sm text-danger-subtle-foreground">
            Could not load purges.
          </p>
        ) : data.data.length === 0 ? (
          <p className="text-sm text-muted-foreground">No purges yet.</p>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Target</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Requested by</TableHead>
                <TableHead>Recoverable until</TableHead>
                <TableHead />
              </TableRow>
            </TableHeader>
            <TableBody>
              {data.data.map((op) => {
                const a = op.attributes;
                const recoverable = a.status === 'pending' || a.status === 'partial';
                return (
                  <TableRow key={op.id} data-testid={`purge-${op.id}`}>
                    <TableCell>
                      <div className="font-medium">{a.target_label}</div>
                      <div className="text-xs text-muted-foreground">{a.scope}</div>
                    </TableCell>
                    <TableCell>
                      <Badge variant={purgeStatusVariant(a.status)}>
                        {purgeStatusLabel(a.status, a.failed_services ?? [])}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-sm">{a.requested_by_email}</TableCell>
                    <TableCell className="text-sm">
                      {recoverable && a.purge_after ? (
                        <span title={`Deleted for good on ${absolute(a.purge_after)}`}>
                          {daysLeft(a.purge_after)} day{daysLeft(a.purge_after) === 1 ? '' : 's'}{' '}
                          left to restore
                        </span>
                      ) : (
                        <span className="text-muted-foreground">—</span>
                      )}
                    </TableCell>
                    <TableCell className="space-x-2 text-right">
                      {recoverable ? (
                        <Button
                          type="button"
                          size="sm"
                          variant="outline"
                          disabled={cancelPurge.isPending}
                          onClick={() => cancelPurge.mutate(op.id)}
                        >
                          Restore
                        </Button>
                      ) : null}
                      {a.status === 'partial' ? (
                        <Button
                          type="button"
                          size="sm"
                          variant="outline"
                          aria-describedby="retry-help"
                          disabled={retryPurge.isPending}
                          onClick={() => retryPurge.mutate(op.id)}
                        >
                          Retry
                        </Button>
                      ) : null}
                    </TableCell>
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
