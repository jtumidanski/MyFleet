import { useVehicle } from '../../../lib/hooks/api/vehicles';
import { Skeleton } from '../../ui/skeleton';

/**
 * The crumb standing for `/vehicles/:id`.
 *
 * Its own component so `useVehicle` is only called on routes whose trail names
 * it (FR-CRUMBNAME-7) — no conditional hooks. It shares
 * `vehicleKeys.detail(id)` with VehicleDetailPage, which mounts in the same
 * commit, so React Query dedupes the two into one request (FR-CRUMBNAME-6).
 */
export function VehicleNameCrumb({ id }: { id: string }) {
  const { data, isLoading } = useVehicle(id);

  // Skeleton is aria-hidden by construction, so the crumb announces nothing
  // until the name lands — better than announcing a UUID (FR-CRUMBNAME-4).
  if (isLoading) return <Skeleton className="h-4 w-24" />;

  const attributes = data?.attributes;
  // Covers error, 404 and soft-deleted alike (FR-CRUMBNAME-5).
  if (!attributes) return <>{id}</>;

  // Byte-for-byte VehicleDetailPage.tsx's rule, including the variable name.
  // VehicleNameCrumb.test.tsx pins the two together.
  const title =
    attributes.nickname?.trim() || `${attributes.year} ${attributes.make} ${attributes.model}`;

  return <>{title}</>;
}
