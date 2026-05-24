import { useState } from 'react';
import { Skeleton } from '../../../ui/skeleton';
import { Card, CardContent } from '../../../ui/card';
import { Button } from '../../../ui/button';
import { useVehicles } from '../../../../lib/hooks/api/vehicles';
import { useMileageTrends } from '../../../../lib/hooks/api/dashboard';

interface MileageTrendsWidgetProps {
  fleetId: string;
}

type Range = '30d' | '90d' | '1y' | 'all';

function rangeToParams(range: Range): { from?: string; to?: string } {
  const now = new Date();
  if (range === 'all') return {};
  const days = range === '30d' ? 30 : range === '90d' ? 90 : 365;
  const from = new Date(now);
  from.setDate(from.getDate() - days);
  return { from: from.toISOString() };
}

interface MileageTrendsVehicleProps {
  vehicleId: string;
  range: DateRange;
}

interface DateRange {
  from?: string;
  to?: string;
}

function VehicleTrend({ vehicleId, range }: MileageTrendsVehicleProps) {
  const { data: points, isLoading } = useMileageTrends(vehicleId, range);

  if (isLoading) return <Skeleton className="h-8 w-full" />;

  if (!points || points.length < 2) {
    return <p className="text-xs text-muted-foreground">Not enough data for {vehicleId}.</p>;
  }

  const first = points[0];
  const last = points[points.length - 1];
  const delta = last.mileage - first.mileage;

  return (
    <div className="flex items-center justify-between text-sm border-b py-1 last:border-0">
      <span className="text-xs text-muted-foreground truncate max-w-[60%]">{vehicleId}</span>
      <span className="font-medium">+{delta.toLocaleString()} mi</span>
    </div>
  );
}

export function MileageTrendsWidget({ fleetId }: MileageTrendsWidgetProps) {
  const [range, setRange] = useState<Range>('30d');
  const { data: vehicleResult, isLoading: vehiclesLoading } = useVehicles(fleetId);
  const params = rangeToParams(range);

  const ranges: Range[] = ['30d', '90d', '1y', 'all'];
  const vehicles = vehicleResult?.data ?? [];

  if (vehiclesLoading) {
    return (
      <Card>
        <CardContent className="pt-4 space-y-2">
          <Skeleton className="h-5 w-36" />
          <Skeleton className="h-24 w-full" />
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardContent className="pt-4">
        <div className="flex items-center justify-between mb-3">
          <h3 className="text-sm font-semibold">Mileage Trends</h3>
          <div className="flex gap-1">
            {ranges.map((r) => (
              <Button
                key={r}
                variant={range === r ? 'default' : 'outline'}
                size="sm"
                onClick={() => setRange(r)}
                className="text-xs px-2 h-6"
              >
                {r}
              </Button>
            ))}
          </div>
        </div>
        {vehicles.length === 0 ? (
          <p className="text-sm text-muted-foreground">No vehicles in this fleet.</p>
        ) : (
          <ul className="space-y-1">
            {vehicles.map((v) => (
              <li key={v.id}>
                <VehicleTrend vehicleId={v.id} range={params} />
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}
