import { useAdminFleet } from '../../../lib/hooks/api/admin';

/**
 * The crumb standing for `/admin/fleets/:id`.
 *
 * Same shape and same reasons as VehicleNameCrumb: own component so the hook
 * is route-scoped (FR-CRUMBNAME-7), same `adminKeys.fleet(id)` key as
 * AdminFleetsPage so there is no second request (FR-CRUMBNAME-6).
 */
export function FleetNameCrumb({ id }: { id: string }) {
  const { data, isLoading } = useAdminFleet(id);

  // An inline span, not <Skeleton>: it renders inside BreadcrumbPage's
  // <span>, and Skeleton is a <div> — invalid phrasing-content nesting.
  if (isLoading) {
    return <span className="inline-block h-4 w-24 animate-pulse rounded-md bg-muted" aria-hidden />;
  }

  const attributes = data?.attributes;
  if (!attributes) return <>{id}</>;

  return <>{attributes.name}</>;
}
