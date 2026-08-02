import { useEffect, useState, type ReactNode } from 'react';
import { toast } from 'sonner';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import { formatMileage, formatMoney } from '@myfleet/ui-components';
import { Loader2 } from 'lucide-react';
import { Sheet, SheetContent, SheetHeader, SheetTitle } from '../../../ui/sheet';
import { Button } from '../../../ui/button';
import { Skeleton } from '../../../ui/skeleton';
import { RecordAttachmentList } from '../maintenance/RecordAttachmentList';
import { MaintenanceRecordForm } from '../maintenance/MaintenanceRecordForm';
import { FuelForm } from '../fuel/FuelForm';
import {
  useMaintenanceRecord,
  useMaintenanceCategories,
  useUpdateMaintenanceRecord,
  useAppendMaintenanceRecordDocument,
  useDeleteMaintenanceRecord,
} from '../../../../lib/hooks/api/maintenance';
import { useFuelLog, useUpdateFuelLog, useDeleteFuelLog } from '../../../../lib/hooks/api/fuel';
import { useMileageRecords } from '../../../../lib/hooks/api/mileage';
import type { VehicleRecordRow } from '../../../../lib/vehicleRecords';
import type { MaintenanceRecordFormInput } from '../../../../lib/schemas/maintenanceRecord';
import type { FuelFormInput } from '../../../../lib/schemas/fuel';

interface VehicleRecordDrawerProps {
  /** null closes the drawer. */
  row: VehicleRecordRow | null;
  onClose: () => void;
  vehicleId: string;
  canWrite: boolean;
}

function DetailRow({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="text-foreground">{value}</dd>
    </div>
  );
}

/** RFC3339 -> `YYYY-MM-DDTHH:MM`, the shape a `datetime-local` input (and
 * these forms' own "now" default) requires. */
function toDatetimeLocal(iso: string): string {
  return new Date(iso).toISOString().slice(0, 16);
}

/**
 * Side drawer showing one record's full detail, with edit/delete gated by
 * kind to match what the backend actually supports:
 *  - maintenance/modification: full detail + attachments; edit and delete.
 *  - fuel: full detail; edit and delete.
 *  - mileage: read-only. There is no PATCH or DELETE endpoint for mileage
 *    records (mileage-service only exposes list + create), so no Edit/Delete
 *    button is rendered — offering either would produce a 404/405 the user
 *    can do nothing about.
 *
 * Each detail query is enabled only for the row's own kind, so closing the
 * drawer (row -> null) or switching to a different kind stops the request
 * rather than leaving it running in the background.
 */
export function VehicleRecordDrawer({ row, onClose, vehicleId, canWrite }: VehicleRecordDrawerProps) {
  const [mode, setMode] = useState<'view' | 'edit'>('view');

  // A newly selected row always starts in view mode, even if the previous
  // row was left mid-edit.
  useEffect(() => {
    setMode('view');
  }, [row?.id]);

  const isMaintenanceKind = row?.kind === 'maintenance' || row?.kind === 'modification';
  const isFuelKind = row?.kind === 'fuel';
  const isMileageKind = row?.kind === 'mileage';

  const { data: categories } = useMaintenanceCategories();

  const { data: record, isLoading: recordLoading } = useMaintenanceRecord(
    isMaintenanceKind ? row?.sourceId : undefined,
  );
  const updateRecord = useUpdateMaintenanceRecord(vehicleId);
  const appendDocument = useAppendMaintenanceRecordDocument(vehicleId);
  const deleteRecord = useDeleteMaintenanceRecord(vehicleId);

  const { data: fuelLog, isLoading: fuelLoading } = useFuelLog(isFuelKind ? row?.sourceId : undefined);
  const updateFuel = useUpdateFuelLog();
  const deleteFuel = useDeleteFuelLog();

  // No GET /mileage/{id} exists — the fleet-service mileage resource only
  // exposes list + create. Reusing the same list query the rest of the page
  // already holds (same vehicleId, same default from/to) is a cache hit, not
  // an extra request, since the row being viewed was itself built from this
  // hook's loaded rows.
  const { data: mileageData } = useMileageRecords(isMileageKind ? { vehicleId } : null);
  const mileageRecord = mileageData?.rows.find((m) => m.id === row?.sourceId);

  if (!row) {
    return (
      <Sheet open={false} onOpenChange={(open) => !open && onClose()}>
        <SheetContent side="right" />
      </Sheet>
    );
  }

  const kind = row.kind === 'modification' ? 'modification' : 'maintenance';

  const handleUpdateRecord = async (
    values: MaintenanceRecordFormInput,
    documentMediaIds: string[],
  ) => {
    if (!record) return;
    try {
      await updateRecord.mutateAsync({
        id: record.id,
        attributes: {
          performedAt: new Date(values.performedAt).toISOString(),
          description: values.description || undefined,
          mileage: values.mileage,
          cost: values.cost,
          vendor: values.vendor || undefined,
          notes: values.notes || undefined,
        },
      });

      // UpdateMaintenanceRecordAttributes (the PATCH body) has no
      // documentMediaIds field, so any files picked in the edit form must be
      // attached separately, through the append-document endpoint, or they'd
      // be uploaded and then silently orphaned. Sequential, not Promise.all:
      // appendDocumentMedia is a single-id POST with no batch form, and this
      // keeps a partial failure's toast attributable to "N attached, then it
      // failed" rather than an ambiguous mixed-settle race.
      let attachFailures = 0;
      for (const mediaId of documentMediaIds) {
        try {
          await appendDocument.mutateAsync({ id: record.id, mediaId });
        } catch {
          attachFailures += 1;
        }
      }

      if (attachFailures > 0) {
        toast.error(
          `Record updated, but ${attachFailures} attachment${attachFailures === 1 ? '' : 's'} could not be attached`,
        );
      } else {
        toast.success(kind === 'modification' ? 'Modification updated' : 'Maintenance record updated');
      }
      setMode('view');
    } catch (err) {
      const apiError = createErrorFromUnknown(err);
      toast.error(apiError.message || 'Could not update the record');
    }
  };

  const handleDeleteRecord = async () => {
    if (!record) return;
    try {
      await deleteRecord.mutateAsync(record.id);
      toast.success(kind === 'modification' ? 'Modification deleted' : 'Maintenance record deleted');
      onClose();
    } catch (err) {
      const apiError = createErrorFromUnknown(err);
      toast.error(apiError.message || 'Could not delete the record');
    }
  };

  const handleUpdateFuel = async (values: FuelFormInput) => {
    if (!fuelLog) return;
    try {
      await updateFuel.mutateAsync({
        id: fuelLog.id,
        attributes: {
          date: new Date(values.date).toISOString(),
          mileage: values.mileage,
          gallons: values.gallons,
          totalCost: values.totalCost,
          pricePerGallon: values.pricePerGallon,
        },
      });
      toast.success('Fuel log updated');
      setMode('view');
    } catch (err) {
      const apiError = createErrorFromUnknown(err);
      toast.error(apiError.message || 'Could not update the fuel log');
    }
  };

  const handleDeleteFuel = async () => {
    if (!fuelLog) return;
    try {
      await deleteFuel.mutateAsync(fuelLog.id);
      toast.success('Fuel log deleted');
      onClose();
    } catch (err) {
      const apiError = createErrorFromUnknown(err);
      toast.error(apiError.message || 'Could not delete the fuel log');
    }
  };

  let body: ReactNode = null;

  if (isMaintenanceKind) {
    if (recordLoading || !record) {
      body = <Skeleton className="h-40 w-full" />;
    } else if (mode === 'edit' && categories) {
      body = (
        <MaintenanceRecordForm
          categories={categories}
          kind={kind}
          defaultMileage={record.attributes.mileage}
          defaultValues={{
            categoryId: record.attributes.categoryId,
            performedAt: toDatetimeLocal(record.attributes.performedAt),
            description: record.attributes.description ?? '',
            mileage: record.attributes.mileage,
            cost: record.attributes.cost,
            vendor: record.attributes.vendor ?? '',
            notes: record.attributes.notes ?? '',
          }}
          onSubmit={handleUpdateRecord}
          onCancel={() => setMode('view')}
          submitting={updateRecord.isPending || appendDocument.isPending}
        />
      );
    } else {
      body = (
        <div className="space-y-4">
          <dl className="grid grid-cols-2 gap-3 text-sm">
            <DetailRow
              label="Performed"
              value={new Date(record.attributes.performedAt).toLocaleDateString()}
            />
            <DetailRow
              label="Odometer"
              value={
                typeof record.attributes.mileage === 'number'
                  ? formatMileage(record.attributes.mileage)
                  : '—'
              }
            />
            <DetailRow
              label="Cost"
              value={record.attributes.cost > 0 ? formatMoney(record.attributes.cost) : '—'}
            />
            <DetailRow label="Vendor" value={record.attributes.vendor || '—'} />
            <div className="col-span-2">
              <dt className="text-muted-foreground">Notes</dt>
              <dd className="whitespace-pre-wrap text-foreground">{record.attributes.notes || '—'}</dd>
            </div>
          </dl>

          <div>
            <p className="mb-2 text-sm font-medium">Attachments</p>
            <RecordAttachmentList mediaIds={record.attributes.documentMediaIds ?? []} />
          </div>

          {canWrite && (
            <div className="flex gap-2">
              <Button type="button" variant="outline" onClick={() => setMode('edit')}>
                Edit
              </Button>
              <Button
                type="button"
                variant="destructive"
                disabled={deleteRecord.isPending}
                onClick={() => void handleDeleteRecord()}
              >
                {deleteRecord.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                Delete
              </Button>
            </div>
          )}
        </div>
      );
    }
  } else if (isFuelKind) {
    if (fuelLoading || !fuelLog) {
      body = <Skeleton className="h-40 w-full" />;
    } else if (mode === 'edit') {
      body = (
        <FuelForm
          defaultMileage={fuelLog.attributes.mileage}
          defaultValues={{
            date: toDatetimeLocal(fuelLog.attributes.date),
            mileage: fuelLog.attributes.mileage,
            gallons: fuelLog.attributes.gallons,
            totalCost: fuelLog.attributes.totalCost,
            pricePerGallon: fuelLog.attributes.pricePerGallon,
          }}
          onSubmit={handleUpdateFuel}
          onCancel={() => setMode('view')}
          submitting={updateFuel.isPending}
        />
      );
    } else {
      body = (
        <div className="space-y-4">
          <dl className="grid grid-cols-2 gap-3 text-sm">
            <DetailRow label="Date" value={new Date(fuelLog.attributes.date).toLocaleDateString()} />
            <DetailRow label="Odometer" value={formatMileage(fuelLog.attributes.mileage)} />
            <DetailRow label="Gallons" value={fuelLog.attributes.gallons.toFixed(3)} />
            <DetailRow
              label="Price / gal"
              value={`$${fuelLog.attributes.pricePerGallon.toFixed(3)}`}
            />
            <DetailRow label="Total" value={formatMoney(fuelLog.attributes.totalCost)} />
          </dl>

          {canWrite && (
            <div className="flex gap-2">
              <Button type="button" variant="outline" onClick={() => setMode('edit')}>
                Edit
              </Button>
              <Button
                type="button"
                variant="destructive"
                disabled={deleteFuel.isPending}
                onClick={() => void handleDeleteFuel()}
              >
                {deleteFuel.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                Delete
              </Button>
            </div>
          )}
        </div>
      );
    }
  } else if (isMileageKind) {
    body = mileageRecord ? (
      <dl className="grid grid-cols-2 gap-3 text-sm">
        <DetailRow
          label="Date"
          value={new Date(mileageRecord.attributes.recordedAt).toLocaleDateString()}
        />
        <DetailRow label="Mileage" value={formatMileage(mileageRecord.attributes.mileage)} />
        <DetailRow label="Source" value={mileageRecord.attributes.source} />
        <DetailRow label="Flagged" value={mileageRecord.attributes.flagged ? 'Yes' : 'No'} />
      </dl>
    ) : (
      <Skeleton className="h-40 w-full" />
    );
  }

  return (
    <Sheet open onOpenChange={(open) => !open && onClose()}>
      <SheetContent side="right" className="w-full overflow-y-auto sm:max-w-md">
        <SheetHeader>
          <SheetTitle>{row.title}</SheetTitle>
        </SheetHeader>
        <div className="mt-4">{body}</div>
      </SheetContent>
    </Sheet>
  );
}
