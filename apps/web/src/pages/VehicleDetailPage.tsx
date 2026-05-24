import { useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { toast } from 'sonner';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import { StatusBadge, formatMileage, type VehicleStatus } from '@myfleet/ui-components';
import { useAuth } from '../context/AuthContext';
import {
  useVehicle,
  useUpdateVehicle,
  useSoftDeleteVehicle,
  useRestoreVehicle,
} from '../lib/hooks/api/vehicles';
import { VehicleForm } from '../components/features/vehicles/VehicleForm';
import { Skeleton } from '../components/common/Skeleton';
import type { VehicleFormInput } from '../lib/schemas/vehicle';
import type { UpdateVehicleAttributes } from '../types/models/vehicle';

const KNOWN_STATUSES: readonly VehicleStatus[] = [
  'Healthy',
  'Upcoming Maintenance',
  'Overdue',
  'Inactive',
];

function asVehicleStatus(value: string | undefined): VehicleStatus | null {
  return value && (KNOWN_STATUSES as readonly string[]).includes(value)
    ? (value as VehicleStatus)
    : null;
}

export function VehicleDetailPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { role } = useAuth();
  const { data: vehicle, isLoading } = useVehicle(id);
  const updateVehicle = useUpdateVehicle();
  const softDelete = useSoftDeleteVehicle();
  const restore = useRestoreVehicle();
  const [editing, setEditing] = useState(false);

  // Viewers are read-only. Restore is owner-only (server still enforces).
  const canWrite = role === 'owner' || role === 'member';
  const canRestore = role === 'owner';

  if (isLoading) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-8 w-64" />
        <Skeleton className="h-40 w-full max-w-2xl" />
      </div>
    );
  }

  if (!vehicle) {
    return <p className="text-gray-500">Vehicle not found.</p>;
  }

  const { attributes } = vehicle;
  const status = asVehicleStatus(attributes.status);
  const title =
    attributes.nickname?.trim() || `${attributes.year} ${attributes.make} ${attributes.model}`;

  const handleUpdate = async (values: VehicleFormInput) => {
    const patch: UpdateVehicleAttributes = {
      nickname: values.nickname || undefined,
      currentMileage: values.currentMileage,
      notes: values.notes || undefined,
    };
    try {
      await updateVehicle.mutateAsync({ id: vehicle.id, attributes: patch });
      toast.success('Vehicle updated');
      setEditing(false);
    } catch (err) {
      const apiError = createErrorFromUnknown(err);
      toast.error(apiError.message || 'Could not update vehicle');
    }
  };

  const handleDelete = async () => {
    try {
      await softDelete.mutateAsync(vehicle.id);
      toast.success('Vehicle deleted');
      navigate('/vehicles', { replace: true });
    } catch (err) {
      const apiError = createErrorFromUnknown(err);
      toast.error(apiError.message || 'Could not delete vehicle');
    }
  };

  const handleRestore = async () => {
    try {
      await restore.mutateAsync(vehicle.id);
      toast.success('Vehicle restored');
    } catch (err) {
      const apiError = createErrorFromUnknown(err);
      toast.error(apiError.message || 'Could not restore vehicle');
    }
  };

  return (
    <div className="max-w-2xl">
      <div className="flex items-start justify-between gap-2">
        <div>
          <div className="flex items-center gap-3">
            <h1 className="text-2xl font-semibold">{title}</h1>
            {status && <StatusBadge status={status} />}
          </div>
          <p className="mt-1 text-sm text-gray-500">
            {attributes.year} {attributes.make} {attributes.model}
            {attributes.trim ? ` ${attributes.trim}` : ''}
          </p>
        </div>
        <div className="flex gap-2">
          {canWrite && !editing && (
            <button
              type="button"
              onClick={() => setEditing(true)}
              className="rounded-md border border-gray-300 px-3 py-1.5 text-sm hover:bg-gray-50"
            >
              Edit
            </button>
          )}
          {canWrite && (
            <button
              type="button"
              onClick={() => void handleDelete()}
              disabled={softDelete.isPending}
              className="rounded-md border border-red-300 px-3 py-1.5 text-sm text-red-700 hover:bg-red-50 disabled:opacity-60"
            >
              Delete
            </button>
          )}
          {canRestore && (
            <button
              type="button"
              onClick={() => void handleRestore()}
              disabled={restore.isPending}
              className="rounded-md border border-gray-300 px-3 py-1.5 text-sm hover:bg-gray-50 disabled:opacity-60"
            >
              Restore
            </button>
          )}
        </div>
      </div>

      <div className="mt-6 rounded-lg border border-gray-200 bg-white p-6 shadow-sm">
        {editing ? (
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
            onCancel={() => setEditing(false)}
            submitting={updateVehicle.isPending}
          />
        ) : (
          <dl className="grid grid-cols-2 gap-4 text-sm">
            <div>
              <dt className="text-gray-500">Mileage</dt>
              <dd className="text-gray-900">
                {typeof attributes.currentMileage === 'number'
                  ? formatMileage(attributes.currentMileage)
                  : '—'}
              </dd>
            </div>
            <div>
              <dt className="text-gray-500">VIN</dt>
              <dd className="text-gray-900">{attributes.vin || '—'}</dd>
            </div>
            <div className="col-span-2">
              <dt className="text-gray-500">Notes</dt>
              <dd className="whitespace-pre-wrap text-gray-900">{attributes.notes || '—'}</dd>
            </div>
          </dl>
        )}
      </div>
    </div>
  );
}
