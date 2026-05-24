import { Skeleton } from '../../../ui/skeleton';
import { Card, CardContent } from '../../../ui/card';
import { useVehicles } from '../../../../lib/hooks/api/vehicles';

interface VehicleStatusWidgetProps {
  fleetId: string;
}

export function VehicleStatusWidget({ fleetId }: VehicleStatusWidgetProps) {
  const { data: result, isLoading } = useVehicles(fleetId);

  if (isLoading) {
    return (
      <Card>
        <CardContent className="pt-4 space-y-2">
          <Skeleton className="h-5 w-32" />
          <Skeleton className="h-12 w-full" />
        </CardContent>
      </Card>
    );
  }

  const vehicles = result?.data ?? [];

  return (
    <Card>
      <CardContent className="pt-4">
        <h3 className="text-sm font-semibold mb-3">Vehicle Status</h3>
        {vehicles.length === 0 ? (
          <p className="text-sm text-muted-foreground">No vehicles in this fleet.</p>
        ) : (
          <ul className="space-y-1">
            {vehicles.map((v) => (
              <li key={v.id} className="flex items-center justify-between text-sm border-b py-1 last:border-0">
                <span className="font-medium">
                  {v.attributes.nickname || `${v.attributes.year} ${v.attributes.make} ${v.attributes.model}`}
                </span>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}
