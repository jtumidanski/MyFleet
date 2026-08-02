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
}

/**
 * Wraps MaintenanceScheduleForm. Handler lifted verbatim from
 * VehicleMaintenanceSection.tsx:110-124. maintenance_schedules stays
 * maintenance-only (PRD §2 non-goals), so the picker excludes modification
 * categories, matching the section it replaces.
 */
export function AddScheduleDialog({ open, onOpenChange, vehicleId }: AddScheduleDialogProps) {
  const { data: categories } = useMaintenanceCategories();
  const createSchedule = useCreateMaintenanceSchedule(vehicleId);

  const maintenanceCategories = useMemo(
    () => (categories ?? []).filter((c) => c.attributes.kind === 'maintenance'),
    [categories],
  );

  const handleCreateSchedule = async (values: MaintenanceScheduleFormInput) => {
    try {
      await createSchedule.mutateAsync({
        categoryId: values.categoryId,
        recurrenceType: values.recurrenceType,
        intervalMonths: values.intervalMonths,
        intervalMiles: values.intervalMiles,
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
            onSubmit={handleCreateSchedule}
            onCancel={() => onOpenChange(false)}
            submitting={createSchedule.isPending}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}
