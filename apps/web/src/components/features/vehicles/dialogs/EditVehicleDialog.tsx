import { toast } from 'sonner';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '../../../ui/dialog';
import { VehicleForm } from '../VehicleForm';
import { useUpdateVehicle, useVehicle } from '../../../../lib/hooks/api/vehicles';
import type { VehicleFormInput } from '../../../../lib/schemas/vehicle';
import type { UpdateVehicleAttributes } from '../../../../types/models/vehicle';

interface EditVehicleDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  vehicleId: string;
}

/**
 * Wraps VehicleForm in "edit" mode. Handler lifted verbatim from
 * VehicleDetailPage.tsx:70-84 — only nickname, currentMileage and notes are
 * server-mutable via PATCH, which is why the form's edit mode hides the rest.
 */
export function EditVehicleDialog({ open, onOpenChange, vehicleId }: EditVehicleDialogProps) {
  const { data: vehicle } = useVehicle(vehicleId);
  const updateVehicle = useUpdateVehicle();

  const handleUpdate = async (values: VehicleFormInput) => {
    const patch: UpdateVehicleAttributes = {
      nickname: values.nickname || undefined,
      currentMileage: values.currentMileage,
      notes: values.notes || undefined,
    };
    try {
      await updateVehicle.mutateAsync({ id: vehicleId, attributes: patch });
      toast.success('Vehicle updated');
      onOpenChange(false);
    } catch (err) {
      const apiError = createErrorFromUnknown(err);
      toast.error(apiError.message || 'Could not update vehicle');
    }
  };

  if (!vehicle) return null;
  const { attributes } = vehicle;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Edit vehicle</DialogTitle>
        </DialogHeader>
        <VehicleForm
          mode="edit"
          defaultValues={{
            make: attributes.make,
            model: attributes.model,
            year: attributes.year,
            nickname: attributes.nickname ?? '',
            trim: attributes.trim ?? '',
            vin: attributes.vin ?? '',
            currentMileage: attributes.currentMileage,
            notes: attributes.notes ?? '',
          }}
          onSubmit={handleUpdate}
          onCancel={() => onOpenChange(false)}
          submitting={updateVehicle.isPending}
        />
      </DialogContent>
    </Dialog>
  );
}
