import { useState } from 'react';
import { toast } from 'sonner';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import { formatMileage } from '@myfleet/ui-components';
import { Card, CardContent } from '../../../ui/card';
import { Button } from '../../../ui/button';
import { Skeleton } from '../../../ui/skeleton';
import {
  useMileageRecords,
  useCreateMileageRecord,
  getLatestMileage,
} from '../../../../lib/hooks/api/mileage';
import { MileageSparkline } from './MileageSparkline';
import { MileageForm } from './MileageForm';
import type { MileageFormInput } from '../../../../lib/schemas/mileage';

interface VehicleMileageSectionProps {
  vehicleId: string;
  /** Current mileage from the vehicle record (fallback for auto-fill). */
  currentMileage?: number;
  /** Hides write actions for viewers. */
  canWrite: boolean;
}

/**
 * Mileage section shown on the vehicle detail page.
 *  - Lists all mileage records in a simple table.
 *  - Shows an SVG sparkline trend graph.
 *  - Auto-fills the log form with the latest recorded mileage.
 *  - member/owner can log a new record; viewers see read-only.
 */
export function VehicleMileageSection({
  vehicleId,
  currentMileage,
  canWrite,
}: VehicleMileageSectionProps) {
  const [showForm, setShowForm] = useState(false);
  const { data: records, isLoading } = useMileageRecords({ vehicleId });
  const createRecord = useCreateMileageRecord(vehicleId);

  // Auto-fill: prefer latest logged record; fallback to vehicle.currentMileage.
  const latestLogged = records ? getLatestMileage(records) : undefined;
  const autoFillMileage = latestLogged ?? currentMileage;

  const handleSubmit = async (values: MileageFormInput) => {
    try {
      await createRecord.mutateAsync({ mileage: values.mileage });
      toast.success('Mileage logged');
      setShowForm(false);
    } catch (err) {
      const apiError = createErrorFromUnknown(err);
      toast.error(apiError.message || 'Could not log mileage');
    }
  };

  return (
    <Card>
      <CardContent className="pt-6">
        <div className="mb-4 flex items-center justify-between">
          <h2 className="text-base font-semibold">Mileage History</h2>
          {canWrite && !showForm && (
            <Button type="button" variant="outline" size="sm" onClick={() => setShowForm(true)}>
              Log Mileage
            </Button>
          )}
        </div>

        {showForm && (
          <div className="mb-6">
            <MileageForm
              defaultMileage={autoFillMileage}
              onSubmit={handleSubmit}
              onCancel={() => setShowForm(false)}
              submitting={createRecord.isPending}
            />
          </div>
        )}

        {isLoading ? (
          <div className="space-y-2">
            <Skeleton className="h-12 w-full" />
            <Skeleton className="h-12 w-full" />
            <Skeleton className="h-12 w-full" />
          </div>
        ) : !records || records.length === 0 ? (
          <p className="text-sm text-muted-foreground">No mileage records yet.</p>
        ) : (
          <>
            <div className="mb-4">
              <MileageSparkline records={records} width={300} height={56} />
            </div>
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b text-left text-muted-foreground">
                    <th className="pb-2 pr-4 font-medium">Date</th>
                    <th className="pb-2 pr-4 font-medium">Mileage</th>
                    <th className="pb-2 font-medium">Source</th>
                  </tr>
                </thead>
                <tbody>
                  {[...records]
                    .sort(
                      (a, b) =>
                        new Date(b.attributes.recordedAt).getTime() -
                        new Date(a.attributes.recordedAt).getTime(),
                    )
                    .map((rec) => (
                      <tr key={rec.id} className="border-b last:border-0">
                        <td className="py-2 pr-4 text-foreground">
                          {new Date(rec.attributes.recordedAt).toLocaleDateString()}
                        </td>
                        <td className="py-2 pr-4 font-medium text-foreground">
                          {formatMileage(rec.attributes.mileage)}
                        </td>
                        <td className="py-2 text-muted-foreground capitalize">
                          {rec.attributes.source}
                          {rec.attributes.flagged && (
                            <span className="ml-2 text-xs text-destructive">flagged</span>
                          )}
                        </td>
                      </tr>
                    ))}
                </tbody>
              </table>
            </div>
          </>
        )}
      </CardContent>
    </Card>
  );
}
