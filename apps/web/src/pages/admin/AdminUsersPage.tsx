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
import { useAdminUsers } from '../../lib/hooks/api/admin';

/**
 * The cross-fleet user directory (FR-ADMIN-FLEET-6).
 *
 * Read-only by design: granting platform admin is a deliberate out-of-band act
 * against auth.platform_admins, and the console does not offer it (PRD non-goal).
 *
 * There is deliberately no platform-admin column. The plan sketched one, but the
 * admin users endpoint does not return that flag — fleet-service would have to
 * ask auth-service per user to know it — and a column that silently rendered
 * "no" for every account would be worse than no column at all.
 */
export function AdminUsersPage() {
  const [page, setPage] = useState(1);
  const { data, isLoading, isError } = useAdminUsers({ page });
  const total = data?.meta?.page?.totalPages ?? 1;

  return (
    <div className="space-y-4">
      <div>
        <h1 className="text-2xl font-semibold">Users</h1>
        <p className="text-sm text-muted-foreground">
          Every account on this platform and the fleets it belongs to.
        </p>
      </div>

      <Card className="p-4">
        {isLoading ? (
          <div className="space-y-2">
            <Skeleton className="h-10" />
            <Skeleton className="h-10" />
          </div>
        ) : isError || !data ? (
          <p role="alert" className="text-sm text-danger-subtle-foreground">
            Could not load users.
          </p>
        ) : data.data.length === 0 ? (
          <p className="text-sm text-muted-foreground">No users.</p>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>User</TableHead>
                <TableHead>Email</TableHead>
                <TableHead>Fleets</TableHead>
                <TableHead>Last sign-in</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {data.data.map((u) => {
                const a = u.attributes;
                return (
                  <TableRow key={u.id} data-testid={`user-${u.id}`}>
                    <TableCell className="text-sm">{a.display_name || u.id}</TableCell>
                    <TableCell className="text-sm">{a.email}</TableCell>
                    <TableCell>
                      {a.fleets.length === 0 ? (
                        <span className="text-sm text-muted-foreground">None</span>
                      ) : (
                        <div className="flex flex-wrap gap-1">
                          {a.fleets.map((f) => (
                            <Badge key={f.fleet_id} variant="secondary">
                              {f.name} · {f.role}
                            </Badge>
                          ))}
                        </div>
                      )}
                    </TableCell>
                    <TableCell className="text-sm">
                      {a.last_login_at ? (
                        new Date(a.last_login_at).toLocaleDateString()
                      ) : (
                        <span className="text-muted-foreground">Never</span>
                      )}
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        )}
      </Card>

      {total > 1 ? (
        <div className="flex items-center gap-2">
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={page <= 1}
            onClick={() => setPage((p) => Math.max(1, p - 1))}
          >
            Previous
          </Button>
          <span className="text-sm text-muted-foreground">
            Page {page} of {total}
          </span>
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={page >= total}
            onClick={() => setPage((p) => p + 1)}
          >
            Next
          </Button>
        </div>
      ) : null}
    </div>
  );
}
