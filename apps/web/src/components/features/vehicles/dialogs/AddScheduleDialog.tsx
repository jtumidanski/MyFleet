import { useMemo } from 'react';
import { toast } from 'sonner';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '../../../ui/dialog';
import { MaintenanceScheduleForm } from '../maintenance/MaintenanceScheduleForm';
import {
  useMaintenanceCategories,
  useCreateMaintenanceSchedule,
} from '../../../../lib/hooks/api/maintenance';
import type { MaintenanceScheduleFormInput } from '../../../../lib/schemas/maintenanceSchedule';

interface AddScheduleDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  vehicleId: string;
  /** The vehicle's odometer, used to default a recurring schedule's first-due odometer. */
  currentMileage?: number;
}

/**
 * Wraps MaintenanceScheduleForm. Handler lifted verbatim from
 * VehicleMaintenanceSection.tsx:110-124. maintenance_schedules stays
 * maintenance-only (PRD §2 non-goals), so the picker excludes modification
 * categories, matching the section it replaces.
 */
export function AddScheduleDialog({
  open,
  onOpenChange,
  vehicleId,
  currentMileage,
}: AddScheduleDialogProps) {
  const { data: categories } = useMaintenanceCategories();
  const createSchedule = useCreateMaintenanceSchedule(vehicleId);

  const maintenanceCategories = useMemo(
    () => (categories ?? []).filter((c) => c.attributes.kind === 'maintenance'),
    [categories],
  );

  const handleCreateSchedule = async (values: MaintenanceScheduleFormInput) => {
    try {
      const oneTime = values.kind === 'oneTime';
      await createSchedule.mutateAsync({
        categoryId: values.categoryId,
        recurrenceType: values.recurrenceType,
        oneTime,
        // A one-time schedule must carry no interval at all (FR-OT-3); the
        // schema already blocks a value here, and undefined keeps a stale
        // field from riding along if the user switched kinds.
        intervalMonths: oneTime ? undefined : values.intervalMonths,
        intervalMiles: oneTime ? undefined : values.intervalMiles,
        // The date input yields YYYY-MM-DD; the API takes RFC3339.
        dueDate: values.dueDate ? new Date(values.dueDate).toISOString() : undefined,
        dueMileage: values.dueMileage,
      });
      toast.success('Maintenance schedule created');
      onOpenChange(false);
    } catch (err) {
      const apiError = createErrorFromUnknown(err);
      toast.error(apiError.message || 'Could not create maintenance schedule');
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Add Schedule</DialogTitle>
        </DialogHeader>
        {categories && (
          <MaintenanceScheduleForm
            categories={maintenanceCategories}
            currentMileage={currentMileage}
            onSubmit={handleCreateSchedule}
            onCancel={() => onOpenChange(false)}
            submitting={createSchedule.isPending}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}
