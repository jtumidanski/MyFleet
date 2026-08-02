import { useRef, useState } from 'react';
import { toast } from 'sonner';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import { useAuth } from '../context/AuthContext';
import { useVehicles, useCreateVehicle } from '../lib/hooks/api/vehicles';
import { VehicleList } from '../components/features/vehicles/VehicleList';
import { VehicleForm } from '../components/features/vehicles/VehicleForm';
import { Button } from '../components/ui/button';
import { PageHeader } from '../components/PageHeader';
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

  // Refs, not state: nothing renders from these, they are read only inside
  // onCloseAutoFocus, and making them state would re-render for nothing.
  const openedFromRef = useRef<'header' | 'empty'>('header');
  const createdRef = useRef(false);
  const headerButtonRef = useRef<HTMLButtonElement>(null);

  const openFrom = (source: 'header' | 'empty') => {
    openedFromRef.current = source;
    createdRef.current = false;
    setOpen(true);
  };

  // Viewers are read-only; only members/owners can add vehicles.
  const canWrite = role === 'owner' || role === 'member';

  const handleCreate = async (values: VehicleFormInput) => {
    try {
      await createVehicle.mutateAsync(toCreateAttributes(values));
      toast.success('Vehicle added');
      createdRef.current = true;
      setOpen(false);
    } catch (err) {
      // Leave the dialog open so the typed values survive for a retry.
      const apiError = createErrorFromUnknown(err);
      toast.error(apiError.message || 'Could not add vehicle');
    }
  };

  return (
    <div className="space-y-6">
      <PageHeader
        title="Vehicles"
        actions={
          canWrite && (
            <Button type="button" ref={headerButtonRef} onClick={() => openFrom('header')}>
              Add Vehicle
            </Button>
          )
        }
      />

      {canWrite && (
        <Dialog
          open={open}
          onOpenChange={(next) => {
            // Backstop: `dismissible` already blocks the three user-facing
            // routes, but this guarantees no dismissal path Radix grows later
            // can close the dialog out from under an in-flight create.
            if (!next && createVehicle.isPending) return;
            setOpen(next);
          }}
        >
          {/* Unmounted on close, which is what discards the form state — do not
              add forceMount. */}
          <DialogContent
            dismissible={!createVehicle.isPending}
            onCloseAutoFocus={(event) => {
              // The empty-state button unmounts with the empty state once the
              // first vehicle exists, so the opener we would restore to is
              // about to be detached. Send focus to the header trigger instead.
              if (openedFromRef.current === 'empty' && createdRef.current) {
                event.preventDefault();
                headerButtonRef.current?.focus();
              }
            }}
          >
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

      <VehicleList
        vehicles={data?.data ?? []}
        isLoading={isLoading}
        emptyAction={
          canWrite ? (
            <Button type="button" onClick={() => openFrom('empty')}>
              Add Vehicle
            </Button>
          ) : undefined
        }
      />
    </div>
  );
}
