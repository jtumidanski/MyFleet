import type { JsonApiDocument, JsonApiResource, PageMeta } from '@myfleet/shared-ts';
import { apiClient } from '../../lib/api/client';
import type {
  AdminFleetAttributes,
  AdminFleetDetailAttributes,
  AdminStatsAttributes,
  AdminUserAttributes,
  AuditEventAttributes,
  CreatePurgeInput,
  DeletedFilter,
  PurgeOperationAttributes,
} from '../../types/models/admin';

/**
 * AdminService — the platform admin console's API surface.
 *
 * Backend routes live in apps/fleet-service/internal/admin/resource.go. Traefik
 * strips /api/fleet, so a service-registered /admin/stats is the gateway path
 * /api/fleet/admin/stats.
 *
 * Every one of these returns 403 when the caller's platform_admin claim is
 * false. The hidden nav entry is cosmetic; the server is authoritative.
 *
 * Pagination is page[number]/page[size] — the platform's actual convention
 * (packages/shared-go/server/pagination.go), not the page/size the PRD sketched.
 */

/** meta on a list response: pagination plus any degradation warnings. */
export interface AdminListMeta {
  page?: PageMeta;
  warnings?: string[];
}

export interface AdminListResult<A> {
  data: Array<JsonApiResource<A>>;
  meta?: AdminListMeta;
}

function pageParams(search: URLSearchParams, page?: number, size?: number): void {
  if (page !== undefined) search.set('page[number]', String(page));
  if (size !== undefined) search.set('page[size]', String(size));
}

function withQuery(path: string, search: URLSearchParams): string {
  const qs = search.toString();
  return qs ? `${path}?${qs}` : path;
}

class AdminService {
  private readonly basePath = '/api/fleet/admin';

  /** GET /api/fleet/admin/stats */
  async stats(): Promise<JsonApiResource<AdminStatsAttributes>> {
    const doc = await apiClient.request<JsonApiDocument<JsonApiResource<AdminStatsAttributes>>>(
      `${this.basePath}/stats`,
    );
    return doc.data;
  }

  /** GET /api/fleet/admin/fleets?q=&deleted=&page[number]=&page[size]= */
  async listFleets(params?: {
    q?: string;
    deleted?: DeletedFilter;
    page?: number;
    size?: number;
  }): Promise<AdminListResult<AdminFleetAttributes>> {
    const search = new URLSearchParams();
    if (params?.q) search.set('q', params.q);
    if (params?.deleted) search.set('deleted', params.deleted);
    pageParams(search, params?.page, params?.size);
    const doc = await apiClient.request<
      JsonApiDocument<Array<JsonApiResource<AdminFleetAttributes>>> & { meta?: AdminListMeta }
    >(withQuery(`${this.basePath}/fleets`, search));
    return { data: doc.data, meta: doc.meta };
  }

  /** GET /api/fleet/admin/fleets/{id} */
  async getFleet(id: string): Promise<JsonApiResource<AdminFleetDetailAttributes>> {
    const doc = await apiClient.request<
      JsonApiDocument<JsonApiResource<AdminFleetDetailAttributes>>
    >(`${this.basePath}/fleets/${id}`);
    return doc.data;
  }

  /** GET /api/fleet/admin/users?page[number]=&page[size]= */
  async listUsers(params?: {
    page?: number;
    size?: number;
  }): Promise<AdminListResult<AdminUserAttributes>> {
    const search = new URLSearchParams();
    pageParams(search, params?.page, params?.size);
    const doc = await apiClient.request<
      JsonApiDocument<Array<JsonApiResource<AdminUserAttributes>>> & { meta?: AdminListMeta }
    >(withQuery(`${this.basePath}/users`, search));
    return { data: doc.data, meta: doc.meta };
  }

  /** GET /api/fleet/admin/purge-operations?status=&page[number]=&page[size]= */
  async listPurges(params?: {
    status?: string;
    page?: number;
    size?: number;
  }): Promise<AdminListResult<PurgeOperationAttributes>> {
    const search = new URLSearchParams();
    if (params?.status) search.set('status', params.status);
    pageParams(search, params?.page, params?.size);
    const doc = await apiClient.request<
      JsonApiDocument<Array<JsonApiResource<PurgeOperationAttributes>>> & { meta?: AdminListMeta }
    >(withQuery(`${this.basePath}/purge-operations`, search));
    return { data: doc.data, meta: doc.meta };
  }

  /** GET /api/fleet/admin/purge-operations/{id} */
  async getPurge(id: string): Promise<JsonApiResource<PurgeOperationAttributes>> {
    const doc = await apiClient.request<JsonApiDocument<JsonApiResource<PurgeOperationAttributes>>>(
      `${this.basePath}/purge-operations/${id}`,
    );
    return doc.data;
  }

  /**
   * POST /api/fleet/admin/purge-operations
   *
   * 409 means the confirmation phrase did not match and NOTHING was written —
   * the server compares it exactly, so the disabled button is a courtesy rather
   * than the control.
   */
  async createPurge(attributes: CreatePurgeInput): Promise<JsonApiResource<PurgeOperationAttributes>> {
    const doc = await apiClient.request<JsonApiDocument<JsonApiResource<PurgeOperationAttributes>>>(
      `${this.basePath}/purge-operations`,
      {
        method: 'POST',
        body: JSON.stringify({ data: { type: 'purge-operations', attributes } }),
      },
    );
    return doc.data;
  }

  /**
   * DELETE /api/fleet/admin/purge-operations/{id} — cancel and restore.
   *
   * Returns the updated operation rather than 204: a cancel whose downstream
   * restore failed comes back `partial` and still cancellable, and the caller
   * needs to see that.
   */
  async cancelPurge(id: string): Promise<JsonApiResource<PurgeOperationAttributes>> {
    const doc = await apiClient.request<JsonApiDocument<JsonApiResource<PurgeOperationAttributes>>>(
      `${this.basePath}/purge-operations/${id}`,
      { method: 'DELETE' },
    );
    return doc.data;
  }

  /** POST /api/fleet/admin/purge-operations/{id}/retry */
  async retryPurge(id: string): Promise<JsonApiResource<PurgeOperationAttributes>> {
    const doc = await apiClient.request<JsonApiDocument<JsonApiResource<PurgeOperationAttributes>>>(
      `${this.basePath}/purge-operations/${id}/retry`,
      { method: 'POST', body: JSON.stringify({ data: { type: 'purge-operations', attributes: {} } }) },
    );
    return doc.data;
  }

  /** GET /api/fleet/admin/audit-events?action=&actor=&page[number]=&page[size]= */
  async listAuditEvents(params?: {
    action?: string;
    actor?: string;
    page?: number;
    size?: number;
  }): Promise<AdminListResult<AuditEventAttributes>> {
    const search = new URLSearchParams();
    if (params?.action) search.set('action', params.action);
    if (params?.actor) search.set('actor', params.actor);
    pageParams(search, params?.page, params?.size);
    const doc = await apiClient.request<
      JsonApiDocument<Array<JsonApiResource<AuditEventAttributes>>> & { meta?: AdminListMeta }
    >(withQuery(`${this.basePath}/audit-events`, search));
    return { data: doc.data, meta: doc.meta };
  }
}

export const adminService = new AdminService();
