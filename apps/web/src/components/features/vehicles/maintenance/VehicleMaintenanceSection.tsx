import { useMemo, useState } from 'react';
import { toast } from 'sonner';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import { Card, CardContent } from '../../../ui/card';
import { Button } from '../../../ui/button';
import { Skeleton } from '../../../ui/skeleton';
import { Loader2, Paperclip } from 'lucide-react';
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
import { RecordAttachmentList } from './RecordAttachmentList';
import type { MaintenanceRecordFormInput } from '../../../../lib/schemas/maintenanceRecord';
import type { MaintenanceScheduleFormInput } from '../../../../lib/schemas/maintenanceSchedule';
import type { MaintenanceCategoryKind } from '../../../../types/models/maintenanceCategory';

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
  /** Which kind the open log form is for. */
  const [formKind, setFormKind] = useState<MaintenanceCategoryKind>('maintenance');
  /** History filter: undefined = everything. */
  const [historyKind, setHistoryKind] = useState<MaintenanceCategoryKind | undefined>();
  const [expandedRecordId, setExpandedRecordId] = useState<string | null>(null);
  const [showScheduleForm, setShowScheduleForm] = useState(false);
  const [completingId, setCompletingId] = useState<string | null>(null);

  const { data: categories, isLoading: categoriesLoading } = useMaintenanceCategories();
  const { data: records, isLoading: recordsLoading } = useMaintenanceRecords(
    vehicleId,
    historyKind,
  );
  const { data: schedules, isLoading: schedulesLoading } = useMaintenanceSchedules(vehicleId);
  const { data: mileageRecords } = useMileageRecords({ vehicleId });

  // Resolve categoryId → category once. The full list is already cached for ten
  // minutes, so every badge, group header and filter label reads from this map
  // instead of issuing a per-row fetch (design D19).
  const categoryById = useMemo(
    () => new Map((categories ?? []).map((c) => [c.id, c])),
    [categories],
  );

  // maintenance_schedules stays maintenance-only (PRD §2 non-goals), so the
  // schedule picker must not offer the seeded modification categories.
  const maintenanceCategories = useMemo(
    () => (categories ?? []).filter((c) => c.attributes.kind === 'maintenance'),
    [categories],
  );

  const createRecord = useCreateMaintenanceRecord(vehicleId);
  const createSchedule = useCreateMaintenanceSchedule(vehicleId);
  const completeSchedule = useCompleteMaintenanceSchedule(vehicleId);

  // Auto-fill mileage: prefer latest logged mileage record; fallback to vehicle.currentMileage.
  const latestLogged = mileageRecords ? getLatestMileage(mileageRecords) : undefined;
  const autoFillMileage = latestLogged ?? currentMileage;

  const handleCreateRecord = async (
    values: MaintenanceRecordFormInput,
    documentMediaIds: string[],
  ) => {
    try {
      await createRecord.mutateAsync({
        categoryId: values.categoryId,
        performedAt: new Date(values.performedAt).toISOString(),
        description: values.description || undefined,
        mileage: values.mileage,
        cost: values.cost,
        vendor: values.vendor || undefined,
        notes: values.notes || undefined,
        documentMediaIds,
      });
      toast.success(
        formKind === 'modification' ? 'Modification logged' : 'Maintenance record logged',
      );
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
                categories={maintenanceCategories}
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
          <div className="mb-4 flex flex-wrap items-center justify-between gap-2">
            <h2 className="text-base font-semibold">History</h2>
            <div className="flex items-center gap-2">
              {(['all', 'maintenance', 'modification'] as const).map((option) => {
                const value = option === 'all' ? undefined : option;
                const isActive = historyKind === value;
                return (
                  <Button
                    key={option}
                    size="sm"
                    variant={isActive ? 'default' : 'outline'}
                    onClick={() => setHistoryKind(value)}
                  >
                    {option === 'all' ? 'All' : option === 'maintenance' ? 'Maintenance' : 'Mods'}
                  </Button>
                );
              })}
              {canWrite && !showRecordForm && (
                <>
                  <Button
                    size="sm"
                    onClick={() => {
                      setFormKind('maintenance');
                      setShowRecordForm(true);
                    }}
                  >
                    Log Record
                  </Button>
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => {
                      setFormKind('modification');
                      setShowRecordForm(true);
                    }}
                  >
                    Log Modification
                  </Button>
                </>
              )}
            </div>
          </div>

          {showRecordForm && categories && (
            <div className="mb-4 rounded-md border p-4">
              <MaintenanceRecordForm
                categories={categories}
                kind={formKind}
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
              {records.map((record) => {
                const category = categoryById.get(record.attributes.categoryId);
                const documentCount = record.attributes.documentMediaIds?.length ?? 0;
                const isExpanded = expandedRecordId === record.id;
                return (
                  <div key={record.id} className="rounded-md border p-3">
                    <div className="flex items-start justify-between gap-2">
                      <div className="min-w-0 flex-1 space-y-0.5">
                        <div className="flex items-center gap-2">
                          {/* description is the primary line; existing records
                              with no description fall back to the category name
                              so history stays readable (PRD FR-REC-2). */}
                          <p className="truncate text-sm font-medium">
                            {record.attributes.description || category?.attributes.name ||
                              record.attributes.categoryId}
                          </p>
                          {category?.attributes.kind === 'modification' && (
                            <span className="inline-flex items-center rounded-full border border-violet-200 bg-violet-100 px-2.5 py-0.5 text-xs font-medium text-violet-800">
                              Mod
                            </span>
                          )}
                        </div>
                        <p className="text-xs text-muted-foreground">
                          {new Date(record.attributes.performedAt).toLocaleDateString()}
                          {record.attributes.description && category
                            ? ` · ${category.attributes.name}`
                            : ''}
                          {record.attributes.mileage
                            ? ` · ${record.attributes.mileage.toLocaleString()} mi`
                            : ''}
                          {record.attributes.cost > 0
                            ? ` · $${record.attributes.cost.toFixed(2)}`
                            : ''}
                        </p>
                        {record.attributes.vendor && (
                          <p className="text-xs text-muted-foreground">
                            {record.attributes.vendor}
                          </p>
                        )}
                      </div>

                      {documentCount > 0 && (
                        <Button
                          size="sm"
                          variant="outline"
                          aria-expanded={isExpanded}
                          onClick={() => setExpandedRecordId(isExpanded ? null : record.id)}
                        >
                          <Paperclip className="mr-1 h-3.5 w-3.5" aria-hidden="true" />
                          {documentCount}
                        </Button>
                      )}
                    </div>

                    {/* Mounted only when expanded, which is what keeps a
                        25-record page from issuing 25 × N metadata requests. */}
                    {isExpanded && (
                      <div className="mt-3 border-t pt-3">
                        <RecordAttachmentList
                          mediaIds={record.attributes.documentMediaIds ?? []}
                        />
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
