import type { ReactNode } from 'react';
import { VehicleCard, VehicleCardSkeleton } from './VehicleCard';
import type { Vehicle } from '../../../types/models/vehicle';

interface VehicleListProps {
  vehicles: Vehicle[];
  isLoading: boolean;
  /**
   * Call to action for the empty state. The list stays presentational — the
   * caller has already decided whether this viewer may act, so the copy keys
   * off the node's presence rather than off a role this component would have
   * to read for itself.
   */
  emptyAction?: ReactNode;
}

export function VehicleList({ vehicles, isLoading, emptyAction }: VehicleListProps) {
  if (isLoading) {
    return (
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {Array.from({ length: 6 }).map((_, i) => (
          <VehicleCardSkeleton key={i} />
        ))}
      </div>
    );
  }

  if (vehicles.length === 0) {
    return (
      <div className="rounded-lg border border-dashed border-border p-8 text-center text-muted-foreground">
        <p>
          {emptyAction ? 'No vehicles yet. Add your first one to get started.' : 'No vehicles yet.'}
        </p>
        {emptyAction && <div className="mt-4">{emptyAction}</div>}
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
