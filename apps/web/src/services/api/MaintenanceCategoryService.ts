import type {
  CreateMaintenanceCategoryAttributes,
  MaintenanceCategoryAttributes,
  MaintenanceCategoryKind,
} from '../../types/models/maintenanceCategory';
import { BaseService, type ListResult } from './BaseService';

/**
 * Maintenance Category service.
 *
 * Routes (apps/fleet-service/internal/maintenancecategory/resource.go, gateway-prefixed):
 *   GET  /api/fleet/maintenance-categories[?kind=maintenance|modification] — list (paged)
 *   POST /api/fleet/maintenance-categories — create a fleet-scoped category (member/owner only)
 *
 * Categories are global/system data — any authenticated caller may list them.
 */
class MaintenanceCategoryService extends BaseService<
  MaintenanceCategoryAttributes,
  CreateMaintenanceCategoryAttributes
> {
  protected readonly resourceType = 'maintenanceCategories';
  protected readonly basePath = '/api/fleet/maintenance-categories';

  /**
   * page[size] is explicit because server.ParsePage defaults to 25 and the
   * seeded list is now 20 rows — five rows from silently truncating the picker
   * the next time a category is added, in a way that would look like "the new
   * category didn't seed" (design D23). 100 is ParsePage's hard ceiling.
   */
  list(kind?: MaintenanceCategoryKind): Promise<ListResult<MaintenanceCategoryAttributes>> {
    const params = new URLSearchParams({ 'page[size]': '100' });
    if (kind) {
      params.set('kind', kind);
    }
    return this.listAt(`${this.basePath}?${params.toString()}`);
  }
}

export const maintenanceCategoryService = new MaintenanceCategoryService();
