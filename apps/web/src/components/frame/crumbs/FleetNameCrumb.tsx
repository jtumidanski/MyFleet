import { useAdminFleet } from '../../../lib/hooks/api/admin';
import { Skeleton } from '../../ui/skeleton';

/**
 * The crumb standing for `/admin/fleets/:id`.
 *
 * Same shape and same reasons as VehicleNameCrumb: own component so the hook
 * is route-scoped (FR-CRUMBNAME-7), same `adminKeys.fleet(id)` key as
 * AdminFleetsPage so there is no second request (FR-CRUMBNAME-6).
 */
export function FleetNameCrumb({ id }: { id: string }) {
  const { data, isLoading } = useAdminFleet(id);

  if (isLoading) return <Skeleton className="h-4 w-24" />;

  const attributes = data?.attributes;
  if (!attributes) return <>{id}</>;

  return <>{attributes.name}</>;
}
