import { Skeleton } from '../../ui/skeleton';
import { VehicleCard } from './VehicleCard';
import type { Vehicle } from '../../../types/models/vehicle';

interface VehicleListProps {
  vehicles: Vehicle[];
  isLoading: boolean;
}

export function VehicleList({ vehicles, isLoading }: VehicleListProps) {
  if (isLoading) {
    return (
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {Array.from({ length: 6 }).map((_, i) => (
          <Skeleton key={i} className="h-44 w-full" />
        ))}
      </div>
    );
  }

  if (vehicles.length === 0) {
    return (
      <div className="rounded-lg border border-dashed border-border p-8 text-center text-muted-foreground">
        No vehicles yet. Add your first one to get started.
      </div>
    );
  }

  return (
    <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
      {vehicles.map((vehicle) => (
        <VehicleCard key={vehicle.id} vehicle={vehicle} />
      ))}
    </div>
  );
}
