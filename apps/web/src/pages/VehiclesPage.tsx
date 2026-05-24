import { useState } from 'react';
import { toast } from 'sonner';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import { useAuth } from '../context/AuthContext';
import { useVehicles, useCreateVehicle } from '../lib/hooks/api/vehicles';
import { VehicleList } from '../components/features/vehicles/VehicleList';
import { VehicleForm } from '../components/features/vehicles/VehicleForm';
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
  const [showForm, setShowForm] = useState(false);

  // Viewers are read-only; only members/owners can add vehicles.
  const canWrite = role === 'owner' || role === 'member';

  const handleCreate = async (values: VehicleFormInput) => {
    try {
      await createVehicle.mutateAsync(toCreateAttributes(values));
      toast.success('Vehicle added');
      setShowForm(false);
    } catch (err) {
      const apiError = createErrorFromUnknown(err);
      toast.error(apiError.message || 'Could not add vehicle');
    }
  };

  return (
    <div>
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold">Vehicles</h1>
        {canWrite && !showForm && (
          <button
            type="button"
            onClick={() => setShowForm(true)}
            className="rounded-md bg-gray-900 px-4 py-2 text-sm font-medium text-white hover:bg-gray-800"
          >
            Add vehicle
          </button>
        )}
      </div>

      {canWrite && showForm && (
        <div className="mt-6 rounded-lg border border-gray-200 bg-white p-6 shadow-sm">
          <h2 className="mb-4 text-lg font-medium">New vehicle</h2>
          <VehicleForm
            mode="create"
            onSubmit={handleCreate}
            onCancel={() => setShowForm(false)}
            submitting={createVehicle.isPending}
          />
        </div>
      )}

      <div className="mt-6">
        <VehicleList vehicles={data?.data ?? []} isLoading={isLoading} />
      </div>
    </div>
  );
}
