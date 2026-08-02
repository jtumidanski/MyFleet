import { Card, CardContent, CardHeader, CardTitle } from '../../../ui/card';
import { MileageSparkline } from '../mileage/MileageSparkline';
import { VehicleActivityTimeline } from '../../activity/VehicleActivityTimeline';
import { useMileageRecords } from '../../../../lib/hooks/api/mileage';
import type { VehicleRecordRow } from '../../../../lib/vehicleRecords';

interface VehicleTrendsProps {
  vehicleId: string;
  /**
   * Unused directly — kept on the prop surface to match the Task 16 brief's
   * interface — MileageSparkline needs raw MileageRecords, not merged rows,
   * so the sparkline fetches its own via useMileageRecords rather than
   * reshaping these.
   */
  mileageRows: VehicleRecordRow[];
}

/**
 * Two-up trends block: the mileage sparkline beside the vehicle's activity
 * timeline. `MileageSparkline` takes raw `MileageRecord`s (it reads
 * `attributes.recordedAt`/`attributes.mileage` directly), so this fetches
 * mileage records itself instead of reshaping the caller's merged
 * `VehicleRecordRow`s — the same query the rest of the page already holds,
 * so this is a cache hit, not an extra request.
 */
export function VehicleTrends({ vehicleId }: VehicleTrendsProps) {
  const { data: mileageData } = useMileageRecords({ vehicleId });

  return (
    <Card>
      <CardContent className="grid grid-cols-[repeat(auto-fit,minmax(260px,1fr))] gap-5 pt-6">
        <div>
          <CardHeader className="p-0 pb-3">
            <CardTitle className="text-base font-semibold">Mileage trend</CardTitle>
          </CardHeader>
          <MileageSparkline records={mileageData?.rows ?? []} width={280} height={64} />
        </div>
        <div>
          <VehicleActivityTimeline vehicleId={vehicleId} />
        </div>
      </CardContent>
    </Card>
  );
}
