import type { MaintenanceCategoryAttributes } from '../../types/models/maintenanceCategory';
import { BaseService } from './BaseService';

/**
 * Maintenance Category service.
 *
 * Routes (apps/fleet-service/internal/maintenancecategory/resource.go, gateway-prefixed):
 *   GET /api/fleet/maintenance-categories — list all categories (paged)
 *
 * Categories are global/system data — any authenticated caller may list them.
 */
class MaintenanceCategoryService extends BaseService<MaintenanceCategoryAttributes> {
  protected readonly resourceType = 'maintenanceCategories';
  protected readonly basePath = '/api/fleet/maintenance-categories';
}

export const maintenanceCategoryService = new MaintenanceCategoryService();
