import { useNavigate } from 'react-router-dom';
import { toast } from 'sonner';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '../../../ui/dialog';
import { Button } from '../../../ui/button';
import { useSoftDeleteVehicle } from '../../../../lib/hooks/api/vehicles';

interface DeleteVehicleDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  vehicleId: string;
}

/**
 * Confirmation-only dialog; no form. Handler lifted verbatim from
 * VehicleDetailPage.tsx:86-95, including the redirect to /vehicles on
 * success.
 */
export function DeleteVehicleDialog({ open, onOpenChange, vehicleId }: DeleteVehicleDialogProps) {
  const navigate = useNavigate();
  const softDelete = useSoftDeleteVehicle();

  const handleDelete = async () => {
    try {
      await softDelete.mutateAsync(vehicleId);
      toast.success('Vehicle deleted');
      navigate('/vehicles', { replace: true });
    } catch (err) {
      const apiError = createErrorFromUnknown(err);
      toast.error(apiError.message || 'Could not delete vehicle');
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Delete Vehicle</DialogTitle>
          <DialogDescription>
            This removes the vehicle from your fleet. This action cannot be undone from the UI.
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            type="button"
            variant="destructive"
            onClick={() => void handleDelete()}
            disabled={softDelete.isPending}
          >
            Delete
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
