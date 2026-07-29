import { useState } from 'react';
import { toast } from 'sonner';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import { Card, CardContent } from '../../../ui/card';
import { Button } from '../../../ui/button';
import { Skeleton } from '../../../ui/skeleton';
import { Loader2 } from 'lucide-react';
import {
  useMaintenanceCategories,
  useMaintenanceRecords,
  useCreateMaintenanceRecord,
  useMaintenanceSchedules,
  useCreateMaintenanceSchedule,
  useCompleteMaintenanceSchedule,
} from '../../../../lib/hooks/api/maintenance';
import { useMileageRecords, getLatestMileage } from '../../../../lib/hooks/api/mileage';
import { MaintenanceRecordForm } from './MaintenanceRecordForm';
import { MaintenanceScheduleForm } from './MaintenanceScheduleForm';
import { SeverityChip } from './SeverityChip';
import type { MaintenanceRecordFormInput } from '../../../../lib/schemas/maintenanceRecord';
import type { MaintenanceScheduleFormInput } from '../../../../lib/schemas/maintenanceSchedule';

interface VehicleMaintenanceSectionProps {
  vehicleId: string;
  currentMileage?: number;
  /** Hides write actions for viewers. */
  canWrite: boolean;
}

/**
 * Maintenance section shown on the vehicle detail page.
 * - Lists maintenance records (newest first).
 * - Lists maintenance schedules with severity and complete action.
 * - Forms gated behind canWrite.
 * - Auto-fills mileage from latest mileage record.
 */
export function VehicleMaintenanceSection({
  vehicleId,
  currentMileage,
  canWrite,
}: VehicleMaintenanceSectionProps) {
  const [showRecordForm, setShowRecordForm] = useState(false);
  const [showScheduleForm, setShowScheduleForm] = useState(false);
  const [completingId, setCompletingId] = useState<string | null>(null);

  const { data: categories, isLoading: categoriesLoading } = useMaintenanceCategories();
  const { data: records, isLoading: recordsLoading } = useMaintenanceRecords(vehicleId);
  const { data: schedules, isLoading: schedulesLoading } = useMaintenanceSchedules(vehicleId);
  const { data: mileageRecords } = useMileageRecords({ vehicleId });

  const createRecord = useCreateMaintenanceRecord(vehicleId);
  const createSchedule = useCreateMaintenanceSchedule(vehicleId);
  const completeSchedule = useCompleteMaintenanceSchedule(vehicleId);

  // Auto-fill mileage: prefer latest logged mileage record; fallback to vehicle.currentMileage.
  const latestLogged = mileageRecords ? getLatestMileage(mileageRecords) : undefined;
  const autoFillMileage = latestLogged ?? currentMileage;

  const handleCreateRecord = async (values: MaintenanceRecordFormInput) => {
    try {
      await createRecord.mutateAsync({
        categoryId: values.categoryId,
        performedAt: new Date(values.performedAt).toISOString(),
        mileage: values.mileage ?? 0,
        cost: values.cost ?? 0,
        vendor: values.vendor ?? '',
        notes: values.notes ?? '',
        documentMediaIds: values.documentMediaIds ?? [],
      });
      toast.success('Maintenance record logged');
      setShowRecordForm(false);
    } catch (err) {
      const apiError = createErrorFromUnknown(err);
      toast.error(apiError.message || 'Could not log maintenance record');
    }
  };

  const handleCreateSchedule = async (values: MaintenanceScheduleFormInput) => {
    try {
      await createSchedule.mutateAsync({
        categoryId: values.categoryId,
        recurrenceType: values.recurrenceType,
        intervalMonths: values.intervalMonths,
        intervalMiles: values.intervalMiles,
      });
      toast.success('Maintenance schedule created');
      setShowScheduleForm(false);
    } catch (err) {
      const apiError = createErrorFromUnknown(err);
      toast.error(apiError.message || 'Could not create maintenance schedule');
    }
  };

  const handleComplete = async (scheduleId: string) => {
    setCompletingId(scheduleId);
    try {
      await completeSchedule.mutateAsync({
        id: scheduleId,
        attributes: {
          date: new Date().toISOString(),
          latestMileage: autoFillMileage,
        },
      });
      toast.success('Maintenance marked as complete');
    } catch (err) {
      const apiError = createErrorFromUnknown(err);
      toast.error(apiError.message || 'Could not complete maintenance schedule');
    } finally {
      setCompletingId(null);
    }
  };

  const isLoading = recordsLoading || schedulesLoading || categoriesLoading;

  if (isLoading) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-8 w-48" />
        <Skeleton className="h-24 w-full" />
        <Skeleton className="h-24 w-full" />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* ── Maintenance Schedules ── */}
      <Card>
        <CardContent className="pt-6">
          <div className="mb-4 flex items-center justify-between">
            <h2 className="text-base font-semibold">Maintenance Schedules</h2>
            {canWrite && !showScheduleForm && (
              <Button size="sm" onClick={() => setShowScheduleForm(true)}>
                Add Schedule
              </Button>
            )}
          </div>

          {showScheduleForm && categories && (
            <div className="mb-4 rounded-md border p-4">
              <MaintenanceScheduleForm
                categories={categories}
                onSubmit={handleCreateSchedule}
                onCancel={() => setShowScheduleForm(false)}
                submitting={createSchedule.isPending}
              />
            </div>
          )}

          {!schedules || schedules.length === 0 ? (
            <p className="text-sm text-muted-foreground">No maintenance schedules defined.</p>
          ) : (
            <div className="space-y-3">
              {schedules.map((schedule) => (
                <div
                  key={schedule.id}
                  className="flex items-center justify-between rounded-md border p-3"
                >
                  <div className="min-w-0 flex-1 space-y-1">
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-medium">{schedule.attributes.categoryId}</span>
                      <SeverityChip severity={schedule.attributes.severity} />
                    </div>
                    <p className="text-xs text-muted-foreground">
                      {schedule.attributes.recurrenceType}
                      {schedule.attributes.intervalMonths
                        ? ` · every ${schedule.attributes.intervalMonths} month(s)`
                        : ''}
                      {schedule.attributes.intervalMiles
                        ? ` · every ${schedule.attributes.intervalMiles.toLocaleString()} miles`
                        : ''}
                    </p>
                    {schedule.attributes.nextDueDate && (
                      <p className="text-xs text-muted-foreground">
                        Due {new Date(schedule.attributes.nextDueDate).toLocaleDateString()}
                      </p>
                    )}
                  </div>
                  {canWrite && (
                    <Button
                      size="sm"
                      variant="outline"
                      disabled={completingId === schedule.id}
                      onClick={() => handleComplete(schedule.id)}
                    >
                      {completingId === schedule.id ? (
                        <Loader2 className="h-4 w-4 animate-spin" />
                      ) : (
                        'Complete'
                      )}
                    </Button>
                  )}
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      {/* ── Maintenance Records ── */}
      <Card>
        <CardContent className="pt-6">
          <div className="mb-4 flex items-center justify-between">
            <h2 className="text-base font-semibold">Maintenance History</h2>
            {canWrite && !showRecordForm && (
              <Button size="sm" onClick={() => setShowRecordForm(true)}>
                Log Record
              </Button>
            )}
          </div>

          {showRecordForm && categories && (
            <div className="mb-4 rounded-md border p-4">
              <MaintenanceRecordForm
                categories={categories}
                defaultMileage={autoFillMileage}
                onSubmit={handleCreateRecord}
                onCancel={() => setShowRecordForm(false)}
                submitting={createRecord.isPending}
              />
            </div>
          )}

          {!records || records.length === 0 ? (
            <p className="text-sm text-muted-foreground">No maintenance records yet.</p>
          ) : (
            <div className="space-y-2">
              {records.map((record) => (
                <div
                  key={record.id}
                  className="flex items-start justify-between rounded-md border p-3"
                >
                  <div className="min-w-0 flex-1 space-y-0.5">
                    <p className="text-sm font-medium">{record.attributes.categoryId}</p>
                    <p className="text-xs text-muted-foreground">
                      {new Date(record.attributes.performedAt).toLocaleDateString()}
                      {' · '}
                      {record.attributes.mileage.toLocaleString()} mi
                      {record.attributes.cost > 0 ? ` · $${record.attributes.cost.toFixed(2)}` : ''}
                    </p>
                    {record.attributes.vendor && (
                      <p className="text-xs text-muted-foreground">{record.attributes.vendor}</p>
                    )}
                  </div>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
