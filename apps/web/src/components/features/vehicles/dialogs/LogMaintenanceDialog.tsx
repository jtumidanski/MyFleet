import { toast } from 'sonner';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '../../../ui/dialog';
import { MaintenanceRecordForm } from '../maintenance/MaintenanceRecordForm';
import {
  useMaintenanceCategories,
  useCreateMaintenanceRecord,
} from '../../../../lib/hooks/api/maintenance';
import type { MaintenanceRecordFormInput } from '../../../../lib/schemas/maintenanceRecord';
import type { MaintenanceCategoryKind } from '../../../../types/models/maintenanceCategory';

interface LogMaintenanceDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  vehicleId: string;
  kind: MaintenanceCategoryKind;
  defaultMileage?: number;
}

/**
 * Wraps MaintenanceRecordForm. Handler lifted verbatim from
 * VehicleMaintenanceSection.tsx:85-108. `kind` picks the log-record vs.
 * log-modification flavor; both share this one dialog.
 */
export function LogMaintenanceDialog({
  open,
  onOpenChange,
  vehicleId,
  kind,
  defaultMileage,
}: LogMaintenanceDialogProps) {
  const { data: categories } = useMaintenanceCategories();
  const createRecord = useCreateMaintenanceRecord(vehicleId);

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
      toast.success(kind === 'modification' ? 'Modification logged' : 'Maintenance record logged');
      onOpenChange(false);
    } catch (err) {
      const apiError = createErrorFromUnknown(err);
      toast.error(apiError.message || 'Could not log maintenance record');
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {kind === 'modification' ? 'Log Modification' : 'Log Maintenance'}
          </DialogTitle>
        </DialogHeader>
        {categories && (
          <MaintenanceRecordForm
            categories={categories}
            kind={kind}
            defaultMileage={defaultMileage}
            onSubmit={handleCreateRecord}
            onCancel={() => onOpenChange(false)}
            submitting={createRecord.isPending}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}
