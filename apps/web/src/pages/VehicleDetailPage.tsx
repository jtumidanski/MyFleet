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
import { VehicleMediaGallery } from '../components/features/vehicles/media/VehicleMediaGallery';
import { VehicleMileageSection } from '../components/features/vehicles/mileage/VehicleMileageSection';
import { VehicleMaintenanceSection } from '../components/features/vehicles/maintenance/VehicleMaintenanceSection';
import { VehicleFuelSection } from '../components/features/vehicles/fuel/VehicleFuelSection';
import { VehicleActivityTimeline } from '../components/features/activity/VehicleActivityTimeline';
import { Skeleton } from '../components/ui/skeleton';
import { Button } from '../components/ui/button';
import { Card, CardContent } from '../components/ui/card';
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
    return <p className="text-muted-foreground">Vehicle not found.</p>;
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
    <div className="max-w-2xl space-y-6">
      <div className="flex items-start justify-between gap-2">
        <div>
          <div className="flex items-center gap-3">
            <h1 className="text-2xl font-semibold">{title}</h1>
            {status && <StatusBadge status={status} />}
          </div>
          <p className="mt-1 text-sm text-muted-foreground">
            {attributes.year} {attributes.make} {attributes.model}
            {attributes.trim ? ` ${attributes.trim}` : ''}
          </p>
        </div>
        <div className="flex gap-2">
          {canWrite && !editing && (
            <Button type="button" variant="outline" onClick={() => setEditing(true)}>
              Edit
            </Button>
          )}
          {canWrite && (
            <Button
              type="button"
              variant="destructive"
              onClick={() => void handleDelete()}
              disabled={softDelete.isPending}
            >
              Delete
            </Button>
          )}
          {canRestore && (
            <Button
              type="button"
              variant="outline"
              onClick={() => void handleRestore()}
              disabled={restore.isPending}
            >
              Restore
            </Button>
          )}
        </div>
      </div>

      <Card>
        <CardContent className="pt-6">
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
                <dt className="text-muted-foreground">Mileage</dt>
                <dd className="text-foreground">
                  {typeof attributes.currentMileage === 'number'
                    ? formatMileage(attributes.currentMileage)
                    : '—'}
                </dd>
              </div>
              <div>
                <dt className="text-muted-foreground">VIN</dt>
                <dd className="text-foreground">{attributes.vin || '—'}</dd>
              </div>
              <div className="col-span-2">
                <dt className="text-muted-foreground">Notes</dt>
                <dd className="whitespace-pre-wrap text-foreground">{attributes.notes || '—'}</dd>
              </div>
            </dl>
          )}
        </CardContent>
      </Card>

      {/* Task 15.1 — Vehicle media gallery */}
      <VehicleMediaGallery vehicleId={vehicle.id} canWrite={canWrite} />

      {/* Task 15.2 — Mileage history + trend */}
      <VehicleMileageSection
        vehicleId={vehicle.id}
        currentMileage={attributes.currentMileage}
        canWrite={canWrite}
      />

      {/* Task 15.3 — Maintenance records, schedules and modifications */}
      <VehicleMaintenanceSection
        vehicleId={vehicle.id}
        currentMileage={attributes.currentMileage}
        canWrite={canWrite}
      />

      {/* Task 15.4 — Fuel logs */}
      <VehicleFuelSection
        vehicleId={vehicle.id}
        currentMileage={attributes.currentMileage}
        canWrite={canWrite}
      />

      {/* Task 15.5 — Per-vehicle activity timeline */}
      <VehicleActivityTimeline vehicleId={vehicle.id} />
    </div>
  );
}
