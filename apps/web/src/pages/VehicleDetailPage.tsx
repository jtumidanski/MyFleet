import { useMemo, useState } from 'react';
import { useParams } from 'react-router-dom';
import { toast } from 'sonner';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import { StatusBadge } from '@myfleet/ui-components';
import { useAuth } from '../context/AuthContext';
import { useVehicle, useRestoreVehicle } from '../lib/hooks/api/vehicles';
import { useMaintenanceSchedules, useMaintenanceCategories } from '../lib/hooks/api/maintenance';
import { useVehicleRecords } from '../lib/hooks/api/vehicleRecords';
import { deriveOdometer } from '../lib/vehicleStats';
import { VehicleIdentityRail } from '../components/features/vehicles/detail/VehicleIdentityRail';
import {
  VehicleQuickActions,
  type QuickAction,
} from '../components/features/vehicles/detail/VehicleQuickActions';
import { VehicleStatStrip } from '../components/features/vehicles/detail/VehicleStatStrip';
import { UpcomingScheduleStrip } from '../components/features/vehicles/detail/UpcomingScheduleStrip';
import { VehicleRecordsTable } from '../components/features/vehicles/detail/VehicleRecordsTable';
import { VehicleRecordDrawer } from '../components/features/vehicles/detail/VehicleRecordDrawer';
import { VehicleTrends } from '../components/features/vehicles/detail/VehicleTrends';
import { EditVehicleDialog } from '../components/features/vehicles/dialogs/EditVehicleDialog';
import { LogMileageDialog } from '../components/features/vehicles/dialogs/LogMileageDialog';
import { LogFuelDialog } from '../components/features/vehicles/dialogs/LogFuelDialog';
import { LogMaintenanceDialog } from '../components/features/vehicles/dialogs/LogMaintenanceDialog';
import { AddScheduleDialog } from '../components/features/vehicles/dialogs/AddScheduleDialog';
import { CompleteScheduleDialog } from '../components/features/vehicles/dialogs/CompleteScheduleDialog';
import { ConvertToRecurrenceDialog } from '../components/features/vehicles/dialogs/ConvertToRecurrenceDialog';
import { DeleteVehicleDialog } from '../components/features/vehicles/dialogs/DeleteVehicleDialog';
import { PhotoGalleryDialog } from '../components/features/vehicles/dialogs/PhotoGalleryDialog';
import { Skeleton } from '../components/ui/skeleton';
import { Button } from '../components/ui/button';
import { asVehicleStatus } from '../components/features/vehicles/vehicleBanner';
import { PageHeader } from '../components/PageHeader';
import type { VehicleRecordRow } from '../lib/vehicleRecords';
import type { MaintenanceSchedule } from '../types/models/maintenanceSchedule';

/** Which single dialog, if any, is open. Only one at a time — opening a new one replaces whatever was open. */
type OpenDialog = QuickAction | 'edit' | 'gallery' | 'complete' | 'convert' | null;

/**
 * Shared by all three render branches. Task-015 requires the title's box to
 * stay put when data lands (FR-10/FR-16), which only holds if the loading and
 * not-found branches use the redesign's wide container too — not the
 * `max-w-2xl` they inherited from the pre-redesign page.
 */
const PAGE_CONTAINER = 'mx-auto w-full max-w-[1600px] space-y-6';

export function VehicleDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { role } = useAuth();
  const { data: vehicle, isLoading } = useVehicle(id);
  const restore = useRestoreVehicle();

  const [openDialog, setOpenDialog] = useState<OpenDialog>(null);
  const [selectedRow, setSelectedRow] = useState<VehicleRecordRow | null>(null);
  const [completingSchedule, setCompletingSchedule] = useState<MaintenanceSchedule | null>(null);
  const [convertingSchedule, setConvertingSchedule] = useState<MaintenanceSchedule | null>(null);

  const schedulesQuery = useMaintenanceSchedules(vehicle?.id);
  const categoriesQuery = useMaintenanceCategories();
  // Pass the whole query result, not categoriesQuery.data — useVehicleRecords
  // needs the loading/error state to tell "categories not loaded yet" apart
  // from "no categories", which is what keeps modification records from
  // rendering as maintenance during the cold-mount window.
  const recordsState = useVehicleRecords(vehicle?.id ?? '', categoriesQuery);

  const categoryNames = useMemo(
    () => new Map((categoriesQuery.data ?? []).map((c) => [c.id, c.attributes.name])),
    [categoriesQuery.data],
  );

  // Viewers are read-only. Restore is owner-only (server still enforces).
  const canWrite = role === 'owner' || role === 'member';
  const canRestore = role === 'owner';

  const odometer = deriveOdometer(recordsState.rows, vehicle?.attributes.currentMileage);

  const closeDialog = () => setOpenDialog(null);

  const handleQuickAction = (action: QuickAction) => {
    // 'upload' has no dedicated dialog of its own — it opens the gallery,
    // which already carries the upload button.
    setOpenDialog(action === 'upload' ? 'gallery' : action);
  };

  const handleComplete = (schedule: MaintenanceSchedule) => {
    setCompletingSchedule(schedule);
    setOpenDialog('complete');
  };

  const handleConvert = (schedule: MaintenanceSchedule) => {
    setConvertingSchedule(schedule);
    setOpenDialog('convert');
  };

  const handleRestore = async () => {
    if (!vehicle) return;
    try {
      await restore.mutateAsync(vehicle.id);
      toast.success('Vehicle restored');
    } catch (err) {
      const apiError = createErrorFromUnknown(err);
      toast.error(apiError.message || 'Could not restore vehicle');
    }
  };

  // Both non-loaded branches carry the SAME container and width as the loaded
  // branch (FR-10/FR-16), and a real heading rather than a heading-shaped
  // skeleton: the title text swaps when data lands, but its box does not move.
  if (isLoading) {
    return (
      <div className={PAGE_CONTAINER}>
        <PageHeader title="Vehicle" />
        <Skeleton className="h-40 w-full" />
      </div>
    );
  }

  if (!vehicle) {
    return (
      <div className={PAGE_CONTAINER}>
        <PageHeader title="Vehicle" />
        <p className="text-muted-foreground">Vehicle not found.</p>
      </div>
    );
  }

  const maintenanceDialogOpen = openDialog === 'maintenance' || openDialog === 'modification';
  const maintenanceDialogKind = openDialog === 'modification' ? 'modification' : 'maintenance';

  // Identity copy for the header. Lifted out of VehicleIdentityRail, which
  // rendered it before task-015's PageHeader convention reached this page.
  const { attributes } = vehicle;
  const status = asVehicleStatus(attributes.status);
  const title =
    attributes.nickname?.trim() || `${attributes.year} ${attributes.make} ${attributes.model}`;
  const specLine = `${attributes.year} ${attributes.make} ${attributes.model}${
    attributes.trim ? ` ${attributes.trim}` : ''
  }`;

  return (
    <div className={PAGE_CONTAINER}>
      {/*
       * Title, status and spec line live here rather than in the identity rail
       * the design sketched them into (design §5): task-015 landed the
       * PageHeader convention across every authenticated page while this
       * redesign was in flight, and a heading rendered only inside the rail
       * would have moved the title's box between the loading and loaded
       * states.
       */}
      <PageHeader
        title={title}
        titleAdornment={status && <StatusBadge status={status} />}
        description={specLine}
      />

      {!!recordsState.categoriesError && (
        <p className="mb-4 rounded-md border border-danger-border bg-danger-subtle px-3 py-2 text-sm text-danger-subtle-foreground">
          {createErrorFromUnknown(recordsState.categoriesError).message ||
            'Could not load maintenance categories — modification records may appear misfiled until this is retried.'}
        </p>
      )}

      <div className="grid gap-6 lg:grid-cols-[320px_minmax(0,1fr)] lg:items-start">
        <aside className="grid gap-4 lg:sticky lg:top-16 lg:self-start">
          <VehicleIdentityRail
            vehicle={vehicle}
            odometer={odometer}
            canWrite={canWrite}
            onEdit={() => setOpenDialog('edit')}
            onViewGallery={() => setOpenDialog('gallery')}
          />
          {canRestore && (
            <Button
              type="button"
              variant="outline"
              className="w-full"
              onClick={() => void handleRestore()}
              disabled={restore.isPending}
            >
              Restore
            </Button>
          )}
          <VehicleQuickActions
            canWrite={canWrite}
            canDelete={canWrite}
            onAction={handleQuickAction}
          />
        </aside>

        <div className="grid gap-4">
          <VehicleStatStrip
            rows={recordsState.rows}
            schedules={schedulesQuery.data ?? []}
            currentMileage={vehicle.attributes.currentMileage}
            partial={recordsState.hasMore}
          />
          <UpcomingScheduleStrip
            schedules={schedulesQuery.data ?? []}
            categoryNames={categoryNames}
            canWrite={canWrite}
            onAddSchedule={() => setOpenDialog('schedule')}
            onComplete={handleComplete}
            onConvert={handleConvert}
          />
          <VehicleRecordsTable
            rows={recordsState.rows}
            total={recordsState.total}
            hasMore={recordsState.hasMore}
            isLoading={recordsState.isLoading}
            isFetchingNextPage={recordsState.isFetchingNextPage}
            onLoadMore={recordsState.loadMore}
            onSelectRow={setSelectedRow}
          />
          <VehicleTrends vehicleId={vehicle.id} />
        </div>
      </div>

      <EditVehicleDialog
        open={openDialog === 'edit'}
        onOpenChange={(open) => !open && closeDialog()}
        vehicleId={vehicle.id}
      />
      <LogMileageDialog
        open={openDialog === 'mileage'}
        onOpenChange={(open) => !open && closeDialog()}
        vehicleId={vehicle.id}
        defaultMileage={odometer}
      />
      <LogFuelDialog
        open={openDialog === 'fuel'}
        onOpenChange={(open) => !open && closeDialog()}
        vehicleId={vehicle.id}
        defaultMileage={odometer}
      />
      <LogMaintenanceDialog
        open={maintenanceDialogOpen}
        onOpenChange={(open) => !open && closeDialog()}
        vehicleId={vehicle.id}
        kind={maintenanceDialogKind}
        defaultMileage={odometer}
      />
      <AddScheduleDialog
        open={openDialog === 'schedule'}
        onOpenChange={(open) => !open && closeDialog()}
        vehicleId={vehicle.id}
        currentMileage={odometer}
      />
      {completingSchedule && (
        <CompleteScheduleDialog
          open={openDialog === 'complete'}
          onOpenChange={(open) => {
            if (!open) {
              closeDialog();
              setCompletingSchedule(null);
            }
          }}
          schedule={completingSchedule}
          onRequestConvert={handleConvert}
        />
      )}
      {convertingSchedule && (
        <ConvertToRecurrenceDialog
          open={openDialog === 'convert'}
          onOpenChange={(open) => {
            if (!open) {
              closeDialog();
              setConvertingSchedule(null);
            }
          }}
          schedule={convertingSchedule}
          categoryName={
            categoryNames.get(convertingSchedule.attributes.categoryId) ??
            convertingSchedule.attributes.categoryId
          }
        />
      )}
      <DeleteVehicleDialog
        open={openDialog === 'delete'}
        onOpenChange={(open) => !open && closeDialog()}
        vehicleId={vehicle.id}
      />
      <PhotoGalleryDialog
        open={openDialog === 'gallery'}
        onOpenChange={(open) => !open && closeDialog()}
        vehicleId={vehicle.id}
        canWrite={canWrite}
      />

      <VehicleRecordDrawer
        row={selectedRow}
        onClose={() => setSelectedRow(null)}
        vehicleId={vehicle.id}
        canWrite={canWrite}
      />
    </div>
  );
}
