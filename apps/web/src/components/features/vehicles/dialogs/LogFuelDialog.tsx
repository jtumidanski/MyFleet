import { toast } from 'sonner';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '../../../ui/dialog';
import { FuelForm } from '../fuel/FuelForm';
import { useCreateFuelLog } from '../../../../lib/hooks/api/fuel';
import type { FuelFormInput } from '../../../../lib/schemas/fuel';

interface LogFuelDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  vehicleId: string;
  defaultMileage?: number;
}

/** Wraps FuelForm. Handler lifted verbatim from VehicleFuelSection.tsx. */
export function LogFuelDialog({
  open,
  onOpenChange,
  vehicleId,
  defaultMileage,
}: LogFuelDialogProps) {
  const createLog = useCreateFuelLog(vehicleId);

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
      onOpenChange(false);
    } catch (err) {
      const apiError = createErrorFromUnknown(err);
      toast.error(apiError.message || 'Could not log fuel entry');
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Log Fill-Up</DialogTitle>
        </DialogHeader>
        <FuelForm
          defaultMileage={defaultMileage}
          onSubmit={handleCreate}
          onCancel={() => onOpenChange(false)}
          submitting={createLog.isPending}
        />
      </DialogContent>
    </Dialog>
  );
}
