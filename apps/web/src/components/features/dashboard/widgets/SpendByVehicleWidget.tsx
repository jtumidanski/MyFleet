import { useState } from 'react';
import { Skeleton } from '../../../ui/skeleton';
import { Card, CardContent } from '../../../ui/card';
import { Button } from '../../../ui/button';
import { useSpendByVehicle } from '../../../../lib/hooks/api/dashboard';

interface SpendByVehicleWidgetProps {
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

export function SpendByVehicleWidget({ fleetId }: SpendByVehicleWidgetProps) {
  const [range, setRange] = useState<Range>('30d');
  const params = rangeToParams(range);
  const { data: rows, isLoading } = useSpendByVehicle(fleetId, params);

  const ranges: Range[] = ['30d', '90d', '1y', 'all'];

  if (isLoading) {
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
          <h3 className="text-sm font-semibold">Spend by Vehicle</h3>
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
        {!rows || rows.length === 0 ? (
          <p className="text-sm text-muted-foreground">No spend data for this period.</p>
        ) : (
          <ul className="space-y-1">
            {rows
              .sort((a, b) => b.totalCost - a.totalCost)
              .map((row) => (
                <li
                  key={row.vehicleId}
                  className="flex items-center justify-between text-sm border-b py-1 last:border-0"
                >
                  <span className="text-xs text-muted-foreground truncate max-w-[60%]">
                    {row.vehicleId}
                  </span>
                  <span className="font-medium">${row.totalCost.toFixed(2)}</span>
                </li>
              ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}
