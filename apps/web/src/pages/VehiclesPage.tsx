import { useState } from 'react';
import { toast } from 'sonner';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import { useAuth } from '../context/AuthContext';
import { useVehicles, useCreateVehicle } from '../lib/hooks/api/vehicles';
import { VehicleList } from '../components/features/vehicles/VehicleList';
import { VehicleForm } from '../components/features/vehicles/VehicleForm';
import { Button } from '../components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '../components/ui/dialog';
import type { VehicleFormInput } from '../lib/schemas/vehicle';
import type { CreateVehicleAttributes } from '../types/models/vehicle';

// Strip empty-string optionals so the backend receives clean attributes.
function toCreateAttributes(values: VehicleFormInput): CreateVehicleAttributes {
  return {
    make: values.make,
    model: values.model,
    year: values.year,
    nickname: values.nickname || undefined,
    trim: values.trim || undefined,
    vin: values.vin || undefined,
    notes: values.notes || undefined,
    currentMileage: values.currentMileage,
  };
}

export function VehiclesPage() {
  const { activeFleetId, role } = useAuth();
  const { data, isLoading } = useVehicles(activeFleetId);
  const createVehicle = useCreateVehicle(activeFleetId ?? '');
  const [open, setOpen] = useState(false);

  // Viewers are read-only; only members/owners can add vehicles.
  const canWrite = role === 'owner' || role === 'member';

  const handleCreate = async (values: VehicleFormInput) => {
    try {
      await createVehicle.mutateAsync(toCreateAttributes(values));
      toast.success('Vehicle added');
      setOpen(false);
    } catch (err) {
      // Leave the dialog open so the typed values survive for a retry.
      const apiError = createErrorFromUnknown(err);
      toast.error(apiError.message || 'Could not add vehicle');
    }
  };

  return (
    <div>
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold">Vehicles</h1>
        {canWrite && (
          <Button type="button" onClick={() => setOpen(true)}>
            Add Vehicle
          </Button>
        )}
      </div>

      {canWrite && (
        <Dialog open={open} onOpenChange={setOpen}>
          {/* Unmounted on close, which is what discards the form state — do not
              add forceMount. */}
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Add Vehicle</DialogTitle>
              <DialogDescription>Make, model, and year are required.</DialogDescription>
            </DialogHeader>
            <VehicleForm
              mode="create"
              onSubmit={handleCreate}
              onCancel={() => setOpen(false)}
              submitting={createVehicle.isPending}
            />
          </DialogContent>
        </Dialog>
      )}

      <div className="mt-6">
        <VehicleList
          vehicles={data?.data ?? []}
          isLoading={isLoading}
          emptyAction={
            canWrite ? (
              <Button type="button" onClick={() => setOpen(true)}>
                Add Vehicle
              </Button>
            ) : undefined
          }
        />
      </div>
    </div>
  );
}
