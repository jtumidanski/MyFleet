import { useCallback, useMemo } from 'react';
import { useMaintenanceRecords } from './maintenance';
import { useFuelLogs } from './fuel';
import { useMileageRecords } from './mileage';
import {
  mergeVehicleRecords,
  type RecordSource,
  type VehicleRecordRow,
} from '../../vehicleRecords';
import type { MaintenanceCategory } from '../../../types/models/maintenanceCategory';

/**
 * The subset of `useMaintenanceCategories()`'s return value this hook needs.
 * A plain array is not enough: on cold mount the three record queries can
 * resolve before the categories query does, and "categories not loaded yet"
 * is otherwise indistinguishable from "this categoryId doesn't resolve to
 * any category" — both look like an empty/absent lookup. Taking the query's
 * loading/error state (not just its settled `data`) lets this hook fold
 * "categories haven't settled" into its own `isLoading` (so a caller shows a
 * skeleton instead of every modification record briefly — or, if the
 * categories request fails, permanently — misfiled as `kind: 'maintenance'`
 * with its raw categoryId as the title) and surface a failed categories
 * fetch via `categoriesError` instead of silently misfiling every
 * modification record with no error channel at all.
 */
export interface CategoriesQueryState {
  data: MaintenanceCategory[] | undefined;
  isLoading: boolean;
  isError: boolean;
  error: unknown;
}

/**
 * Stable fallback for `categoriesQuery.data` while it's undefined (still
 * loading, or failed). A fresh `[]` literal here would get a new identity
 * every render, which would defeat `categoryById`'s memoization exactly the
 * way an inline `[]` passed by a caller would — see the identity tests in
 * vehicleRecords.test.ts.
 */
const EMPTY_CATEGORIES: MaintenanceCategory[] = [];

/**
 * Composes the three paginated record sources into one feed.
 *
 * Each source paginates independently. "Load more" advances only the sources
 * currently constraining the watermark — advancing an already-exhausted source
 * would do nothing, and advancing a source whose oldest row is far below the
 * watermark just fetches rows that stay withheld (see mergeVehicleRecords).
 */
export function useVehicleRecords(vehicleId: string, categoriesQuery: CategoriesQueryState) {
  const maintenance = useMaintenanceRecords(vehicleId);
  const fuel = useFuelLogs(vehicleId);
  const mileage = useMileageRecords({ vehicleId });

  const categories = categoriesQuery.data ?? EMPTY_CATEGORIES;

  const categoryById = useMemo(() => new Map(categories.map((c) => [c.id, c])), [categories]);

  const sources = useMemo<RecordSource[]>(() => {
    const maintenanceRows: VehicleRecordRow[] = (maintenance.data?.rows ?? []).map((r) => {
      const category = categoryById.get(r.attributes.categoryId);
      return {
        id: `maintenance:${r.id}`,
        sourceId: r.id,
        // The category owns kind — a record stores none (design D1).
        kind: category?.attributes.kind === 'modification' ? 'modification' : 'maintenance',
        date: r.attributes.performedAt,
        title: r.attributes.description || category?.attributes.name || r.attributes.categoryId,
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
    maintenance.data,
    maintenance.hasNextPage,
    fuel.data,
    fuel.hasNextPage,
    mileage.data,
    mileage.hasNextPage,
    categoryById,
  ]);

  const { rows, withheldCount } = useMemo(() => mergeVehicleRecords(sources), [sources]);

  const total =
    (maintenance.data?.total ?? 0) + (fuel.data?.total ?? 0) + (mileage.data?.total ?? 0);

  const anyHasMore = sources.some((s) => s.hasMore);

  const isLoading =
    maintenance.isLoading || fuel.isLoading || mileage.isLoading || categoriesQuery.isLoading;

  const categoriesError = categoriesQuery.isError ? categoriesQuery.error : undefined;

  // `withheldCount > 0` can currently only occur while at least one source is
  // still incomplete (mergeVehicleRecords returns withheldCount: 0 the moment
  // every source is exhausted — see its zero-watermark short-circuit), so
  // this arm is provably redundant with `anyHasMore` today. It's kept as a
  // defensive disjunct: `hasMore` should mean "more rows can still appear"
  // for whatever reason, and coupling it to mergeVehicleRecords's *current*
  // internals would silently break if that function's watermark logic ever
  // changes to withhold rows for a reason unrelated to incompleteness.
  const hasMore = anyHasMore || withheldCount > 0;

  const isFetchingNextPage =
    maintenance.isFetchingNextPage || fuel.isFetchingNextPage || mileage.isFetchingNextPage;

  // Pulled into plain locals (rather than called as maintenance.fetchNextPage()
  // below) so the useCallback dependency array can name exactly these six
  // values. react-hooks/exhaustive-deps treats `obj.method()` call
  // expressions as needing the whole `obj` in deps (it can't prove the call
  // doesn't rely on `this`), but the hook objects react-query returns from
  // useInfiniteQuery are not referentially stable across renders — putting
  // them in deps would recompute (and hand a new function identity for)
  // loadMore on every render, which is exactly the churn this task exists to
  // avoid (see Important 2 in task-8-report.md).
  const fetchMaintenanceNextPage = maintenance.fetchNextPage;
  const fetchFuelNextPage = fuel.fetchNextPage;
  const fetchMileageNextPage = mileage.fetchNextPage;

  const loadMore = useCallback(() => {
    // Fetch the next page of every source that still has one. Pages
    // accumulate, so this widens coverage and lowers the watermark, which is
    // what releases the withheld rows.
    if (maintenance.hasNextPage) void fetchMaintenanceNextPage();
    if (fuel.hasNextPage) void fetchFuelNextPage();
    if (mileage.hasNextPage) void fetchMileageNextPage();
  }, [
    maintenance.hasNextPage,
    fetchMaintenanceNextPage,
    fuel.hasNextPage,
    fetchFuelNextPage,
    mileage.hasNextPage,
    fetchMileageNextPage,
  ]);

  return useMemo(
    () => ({
      rows,
      withheldCount,
      total,
      isLoading,
      categoriesError,
      // More to show means either unfetched pages or rows held below the
      // watermark (see the comment on `hasMore` above for why the second arm
      // is currently redundant but kept).
      hasMore,
      // True whenever any source has a fetchNextPage request in flight — a
      // caller can use this to disable a "Load more" control, since clicking
      // it again while fetching cancels the in-flight request (React Query's
      // fetchNextPage sets cancelRefetch: true) and reissues, starving the
      // page under sustained clicks.
      isFetchingNextPage,
      loadMore,
    }),
    [rows, withheldCount, total, isLoading, categoriesError, hasMore, isFetchingNextPage, loadMore],
  );
}
