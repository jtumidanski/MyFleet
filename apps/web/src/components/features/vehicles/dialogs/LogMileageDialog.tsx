import { toast } from 'sonner';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '../../../ui/dialog';
import { MileageForm } from '../mileage/MileageForm';
import { useCreateMileageRecord } from '../../../../lib/hooks/api/mileage';
import type { MileageFormInput } from '../../../../lib/schemas/mileage';

interface LogMileageDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  vehicleId: string;
  defaultMileage?: number;
}

/**
 * Wraps MileageForm in a dialog. The form is unchanged — it already exposes
 * onSubmit/onCancel/submitting, so the dialog only owns the mutation and the
 * close-on-success rule. Errors keep the dialog open so the user's input
 * survives the failure.
 */
export function LogMileageDialog({
  open,
  onOpenChange,
  vehicleId,
  defaultMileage,
}: LogMileageDialogProps) {
  const createRecord = useCreateMileageRecord(vehicleId);

  const handleSubmit = async (values: MileageFormInput) => {
    try {
      await createRecord.mutateAsync({ mileage: values.mileage });
      toast.success('Mileage logged');
      onOpenChange(false);
    } catch (err) {
      const apiError = createErrorFromUnknown(err);
      toast.error(apiError.message || 'Could not log mileage');
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Log Mileage</DialogTitle>
        </DialogHeader>
        <MileageForm
          defaultMileage={defaultMileage}
          onSubmit={handleSubmit}
          onCancel={() => onOpenChange(false)}
          submitting={createRecord.isPending}
        />
      </DialogContent>
    </Dialog>
  );
}
