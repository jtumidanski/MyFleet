import { useMemo } from 'react';
import { useMaintenanceRecords } from './maintenance';
import { useFuelLogs } from './fuel';
import { useMileageRecords } from './mileage';
import { mergeVehicleRecords, type RecordSource, type VehicleRecordRow } from '../../vehicleRecords';
import type { MaintenanceCategory } from '../../../types/models/maintenanceCategory';

/**
 * Composes the three paginated record sources into one feed.
 *
 * Each source paginates independently. "Load more" advances only the sources
 * currently constraining the watermark — advancing an already-exhausted source
 * would do nothing, and advancing a source whose oldest row is far below the
 * watermark just fetches rows that stay withheld (see mergeVehicleRecords).
 */
export function useVehicleRecords(vehicleId: string, categories: MaintenanceCategory[]) {
  const maintenance = useMaintenanceRecords(vehicleId);
  const fuel = useFuelLogs(vehicleId);
  const mileage = useMileageRecords({ vehicleId });

  const categoryById = useMemo(
    () => new Map(categories.map((c) => [c.id, c])),
    [categories],
  );

  const sources = useMemo<RecordSource[]>(() => {
    const maintenanceRows: VehicleRecordRow[] = (maintenance.data?.rows ?? []).map((r) => {
      const category = categoryById.get(r.attributes.categoryId);
      return {
        id: `maintenance:${r.id}`,
        sourceId: r.id,
        // The category owns kind — a record stores none (design D1).
        kind: category?.attributes.kind === 'modification' ? 'modification' : 'maintenance',
        date: r.attributes.performedAt,
        title:
          r.attributes.description || category?.attributes.name || r.attributes.categoryId,
        mileage: r.attributes.mileage,
        cost: r.attributes.cost,
      };
    });

    const fuelRows: VehicleRecordRow[] = (fuel.data?.rows ?? []).map((l) => ({
      id: `fuel:${l.id}`,
      sourceId: l.id,
      kind: 'fuel',
      date: l.attributes.date,
      title: `${l.attributes.gallons.toFixed(3)} gal @ $${l.attributes.pricePerGallon.toFixed(3)}`,
      mileage: l.attributes.mileage,
      // Task 9's deriveAvgEconomy needs gallons; nothing else reads it.
      gallons: l.attributes.gallons,
      cost: l.attributes.totalCost,
    }));

    const mileageRows: VehicleRecordRow[] = (mileage.data?.rows ?? []).map((m) => ({
      id: `mileage:${m.id}`,
      sourceId: m.id,
      kind: 'mileage',
      date: m.attributes.recordedAt,
      title: 'Odometer reading',
      mileage: m.attributes.mileage,
    }));

    return [
      { rows: maintenanceRows, hasMore: maintenance.hasNextPage },
      { rows: fuelRows, hasMore: fuel.hasNextPage },
      { rows: mileageRows, hasMore: mileage.hasNextPage },
    ];
  }, [
    maintenance.data, maintenance.hasNextPage,
    fuel.data, fuel.hasNextPage,
    mileage.data, mileage.hasNextPage,
    categoryById,
  ]);

  const { rows, withheldCount } = useMemo(() => mergeVehicleRecords(sources), [sources]);

  const total =
    (maintenance.data?.total ?? 0) + (fuel.data?.total ?? 0) + (mileage.data?.total ?? 0);

  const anyHasMore = sources.some((s) => s.hasMore);

  return {
    rows,
    withheldCount,
    total,
    isLoading: maintenance.isLoading || fuel.isLoading || mileage.isLoading,
    // More to show means either unfetched pages or rows held below the watermark.
    hasMore: anyHasMore || withheldCount > 0,
    loadMore: () => {
      // Fetch the next page of every source that still has one. Pages
      // accumulate, so this widens coverage and lowers the watermark, which is
      // what releases the withheld rows.
      if (maintenance.hasNextPage) void maintenance.fetchNextPage();
      if (fuel.hasNextPage) void fuel.fetchNextPage();
      if (mileage.hasNextPage) void mileage.fetchNextPage();
    },
  };
}
