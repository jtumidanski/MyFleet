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
        {/* h-40 (160px) is the closest Tailwind step to the rebuilt card's
            ~166px: 2 (Card border, ui/card.tsx) + 32 (p-4, VehicleCard) +
            80 (thumbnail box, VehiclePhotoThumbnail BOX — the text column is
            shorter) + 12 (mt-3 on the actions row) + 40 (size="icon" -> h-10,
            ui/button.tsx). h-44 is 176px and overshoots by 10px; h-40
            undershoots by 6px, and the scale has no step between them.
            Computed, not measured in a browser. */}
        {Array.from({ length: 6 }).map((_, i) => (
          <Skeleton key={i} className="h-40 w-full" />
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
