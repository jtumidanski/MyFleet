import { useMemo } from 'react';
import { useMaintenanceCategories } from './maintenance';
import { useVehicles } from './vehicles';
import { vehicleTitle } from '../../utils/vehicleTitle';

/**
 * Client-side label resolution for maintenance queue rows.
 *
 * The queue endpoints return foreign keys, not names: shared-go/server has no
 * JSON:API `included` support, so there is no `?include=category` to ask for.
 * The category list is ~20 effectively-static rows cached for 10 minutes and
 * the fleet's vehicle list is already warm on the dashboard, so resolving in
 * the client costs no extra round trip in practice.
 *
 * The hooks own fetching and memoizing; the label functions own the fallback
 * string. Splitting them keeps the fallback unit-testable without a render, and
 * lets UpcomingScheduleStrip keep its own raw-id fallback over a compatible map.
 */

export const UNKNOWN_CATEGORY = 'Unknown category';
export const UNKNOWN_VEHICLE = 'Unknown vehicle';

/**
 * `||` not `??`: a category whose name marshalled as "" must fall through to
 * the placeholder rather than render a blank cell.
 */
export function categoryLabel(names: Map<string, string>, id: string): string {
  return names.get(id) || UNKNOWN_CATEGORY;
}

export function vehicleLabel(titles: Map<string, string>, id: string): string {
  return titles.get(id) || UNKNOWN_VEHICLE;
}

/**
 * categoryId -> name, for every kind.
 *
 * Deliberately takes no parameter: passing a `kind` to useMaintenanceCategories
 * would silently drop the other kind's categories and reintroduce the UUID for
 * them (FR-LABEL-3). The no-argument call shares its query key with
 * VehicleDetailPage's category query, so the two dedupe.
 */
export function useCategoryNameMap(): { names: Map<string, string>; isLoading: boolean } {
  const { data, isLoading } = useMaintenanceCategories();
  const names = useMemo(
    () => new Map((data ?? []).map((category) => [category.id, category.attributes.name])),
    [data],
  );
  return { names, isLoading };
}

/**
 * vehicleId -> display title.
 *
 * Note `data.data`: useVehicles has no `select`, unlike the maintenance hooks,
 * so the query data is the whole list envelope.
 */
export function useVehicleTitleMap(fleetId: string | null | undefined): {
  titles: Map<string, string>;
  isLoading: boolean;
} {
  const { data, isLoading } = useVehicles(fleetId);
  const titles = useMemo(
    () => new Map((data?.data ?? []).map((vehicle) => [vehicle.id, vehicleTitle(vehicle.attributes)])),
    [data],
  );
  return { titles, isLoading };
}
