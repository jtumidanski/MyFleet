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
  TransferVehicleInput,
  VehicleTransferAttributes,
  VehicleTransferMeta,
  VehicleTransferPreviewAttributes,
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

/**
 * A completed transfer, with its document `meta` kept alongside the resource.
 *
 * The `meta.count_semantics` annotation is the only place the response says
 * that `affected_counts.media_objects` and `.notifications` are "rows now on
 * the destination" rather than "rows this transfer moved". Returning bare
 * `doc.data` here — as every other method on this class does — would drop it at
 * the last hop and leave the console rendering two numbers under a label they
 * contradict, so this one method returns the envelope.
 */
export interface VehicleTransferResult {
  data: JsonApiResource<VehicleTransferAttributes>;
  meta?: VehicleTransferMeta;
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

  /**
   * GET /api/fleet/admin/vehicles/{id}/transfer-preview?destination_fleet_id=
   *
   * Read-only and cheap: the server counts with aggregates and calls no other
   * service, so this is safe to re-run whenever the chosen destination changes.
   *
   * The destination is optional. Without it the server returns the source-side
   * picture only — the destination fields and `categories_to_create` come back
   * empty, because neither can be computed without knowing where the car goes.
   */
  async previewVehicleTransfer(
    vehicleId: string,
    destinationFleetId?: string,
  ): Promise<JsonApiResource<VehicleTransferPreviewAttributes>> {
    const search = new URLSearchParams();
    if (destinationFleetId) search.set('destination_fleet_id', destinationFleetId);
    const doc = await apiClient.request<
      JsonApiDocument<JsonApiResource<VehicleTransferPreviewAttributes>>
    >(
      withQuery(
        `${this.basePath}/vehicles/${encodeURIComponent(vehicleId)}/transfer-preview`,
        search,
      ),
    );
    return doc.data;
  }

  /**
   * POST /api/fleet/admin/vehicles/{id}/transfer
   *
   * `confirmation` must be WHAT THE OPERATOR TYPED. The server compares it
   * exactly — no trimming, no case folding — so sending the expected phrase
   * instead would make its 409 unreachable and the disabled button the only
   * gate.
   *
   * 409 covers a confirmation mismatch, a pending-purge vehicle or source
   * fleet, and an unavailable destination; 422 covers a missing, malformed or
   * same-fleet destination; 503 means a downstream refused and the transfer was
   * rolled back whole. Every one carries an actionable `detail`, which is why
   * the rejection is propagated untouched rather than remapped to a generic
   * message here.
   */
  async transferVehicle(
    vehicleId: string,
    attributes: TransferVehicleInput,
  ): Promise<VehicleTransferResult> {
    const doc = await apiClient.request<
      JsonApiDocument<JsonApiResource<VehicleTransferAttributes>> & { meta?: VehicleTransferMeta }
    >(`${this.basePath}/vehicles/${encodeURIComponent(vehicleId)}/transfer`, {
      method: 'POST',
      body: JSON.stringify({ data: { type: 'vehicle-transfers', attributes } }),
    });
    return { data: doc.data, meta: doc.meta };
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
  async createPurge(
    attributes: CreatePurgeInput,
  ): Promise<JsonApiResource<PurgeOperationAttributes>> {
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
      {
        method: 'POST',
        body: JSON.stringify({ data: { type: 'purge-operations', attributes: {} } }),
      },
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
