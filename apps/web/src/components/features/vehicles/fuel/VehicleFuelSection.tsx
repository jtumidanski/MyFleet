import { useState } from 'react';
import { toast } from 'sonner';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import { Card, CardContent } from '../../../ui/card';
import { Button } from '../../../ui/button';
import { Skeleton } from '../../../ui/skeleton';
import { useFuelLogs, useCreateFuelLog } from '../../../../lib/hooks/api/fuel';
import { useMileageRecords, getLatestMileage } from '../../../../lib/hooks/api/mileage';
import { FuelForm } from './FuelForm';
import type { FuelFormInput } from '../../../../lib/schemas/fuel';

interface VehicleFuelSectionProps {
  vehicleId: string;
  currentMileage?: number;
  /** Hides write actions for viewers. */
  canWrite: boolean;
}

/**
 * Fuel log section shown on the vehicle detail page.
 * - Lists fuel logs (newest first).
 * - Auto-fills mileage from latest mileage record.
 * - Log form gated behind canWrite.
 */
export function VehicleFuelSection({
  vehicleId,
  currentMileage,
  canWrite,
}: VehicleFuelSectionProps) {
  const [showForm, setShowForm] = useState(false);

  const { data: logs, isLoading: logsLoading } = useFuelLogs(vehicleId);
  const { data: mileageRecords } = useMileageRecords({ vehicleId });
  const createLog = useCreateFuelLog(vehicleId);

  // Auto-fill mileage: prefer latest logged mileage record; fallback to vehicle.currentMileage.
  const latestLogged = mileageRecords ? getLatestMileage(mileageRecords) : undefined;
  const autoFillMileage = latestLogged ?? currentMileage;

  const handleCreate = async (values: FuelFormInput) => {
    try {
      await createLog.mutateAsync({
        date: new Date(values.date).toISOString(),
        mileage: values.mileage,
        gallons: values.gallons,
        totalCost: values.totalCost,
        pricePerGallon: values.pricePerGallon,
      });
      toast.success('Fuel log added');
      setShowForm(false);
    } catch (err) {
      const apiError = createErrorFromUnknown(err);
      toast.error(apiError.message || 'Could not log fuel entry');
    }
  };

  if (logsLoading) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-8 w-48" />
        <Skeleton className="h-24 w-full" />
        <Skeleton className="h-24 w-full" />
      </div>
    );
  }

  return (
    <Card>
      <CardContent className="pt-6">
        <div className="mb-4 flex items-center justify-between">
          <h2 className="text-base font-semibold">Fuel Log</h2>
          {canWrite && !showForm && (
            <Button size="sm" onClick={() => setShowForm(true)}>
              Log Fill-up
            </Button>
          )}
        </div>

        {showForm && (
          <div className="mb-4 rounded-md border p-4">
            <FuelForm
              defaultMileage={autoFillMileage}
              onSubmit={handleCreate}
              onCancel={() => setShowForm(false)}
              submitting={createLog.isPending}
            />
          </div>
        )}

        {!logs || logs.length === 0 ? (
          <p className="text-sm text-muted-foreground">No fuel logs yet.</p>
        ) : (
          <div className="space-y-2">
            {logs.map((log) => (
              <div key={log.id} className="flex items-start justify-between rounded-md border p-3">
                <div className="min-w-0 flex-1 space-y-0.5">
                  <p className="text-sm font-medium">
                    {new Date(log.attributes.date).toLocaleDateString()}
                  </p>
                  <p className="text-xs text-muted-foreground">
                    {log.attributes.gallons.toFixed(3)} gal
                    {' · '}${log.attributes.pricePerGallon.toFixed(3)}/gal
                    {' · '}Total: ${log.attributes.totalCost.toFixed(2)}
                    {' · '}
                    {log.attributes.mileage.toLocaleString()} mi
                  </p>
                </div>
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
