import type { JsonApiDocument, JsonApiResource, PageMeta } from '@myfleet/shared-ts';
import { apiClient } from '../../lib/api/client';

export interface ListResult<A> {
  data: Array<JsonApiResource<A>>;
  meta?: PageMeta;
}

/**
 * Thin, typed wrapper over the shared-ts ApiClient implementing the common
 * JSON:API CRUD verbs. Concrete services set `resourceType` (the JSON:API
 * `type`) and `basePath` (the full gateway path, e.g. `/api/fleet/vehicles`),
 * and add resource-specific methods on top.
 *
 * `apiClient.baseUrl` is '', so paths passed here must be absolute gateway
 * paths; Traefik strips `/api/<service>` before routing to the backend.
 */
export abstract class BaseService<A, CreateA = A, UpdateA = Partial<A>> {
  protected abstract readonly resourceType: string;
  protected abstract readonly basePath: string;

  /** GET a collection (optionally at a custom path, e.g. a nested route). */
  protected async listAt(path: string): Promise<ListResult<A>> {
    const doc = await apiClient.request<JsonApiDocument<Array<JsonApiResource<A>>>>(path);
    return { data: doc.data, meta: doc.meta };
  }

  list(): Promise<ListResult<A>> {
    return this.listAt(this.basePath);
  }

  async get(id: string): Promise<JsonApiResource<A>> {
    const doc = await apiClient.request<JsonApiDocument<JsonApiResource<A>>>(
      `${this.basePath}/${id}`,
    );
    return doc.data;
  }

  /** POST a create, optionally at a custom path (nested create routes). */
  protected async createAt(path: string, attributes: CreateA): Promise<JsonApiResource<A>> {
    const doc = await apiClient.request<JsonApiDocument<JsonApiResource<A>>>(path, {
      method: 'POST',
      body: JSON.stringify({ data: { type: this.resourceType, attributes } }),
    });
    return doc.data;
  }

  create(attributes: CreateA): Promise<JsonApiResource<A>> {
    return this.createAt(this.basePath, attributes);
  }

  async patch(id: string, attributes: UpdateA): Promise<JsonApiResource<A>> {
    const doc = await apiClient.request<JsonApiDocument<JsonApiResource<A>>>(
      `${this.basePath}/${id}`,
      {
        method: 'PATCH',
        body: JSON.stringify({ data: { type: this.resourceType, id, attributes } }),
      },
    );
    return doc.data;
  }

  async remove(id: string): Promise<void> {
    await apiClient.request<null>(`${this.basePath}/${id}`, { method: 'DELETE' });
  }
}
