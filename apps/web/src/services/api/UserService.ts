/**
 * UserService — auth-service user endpoints.
 *
 * Backend route (apps/auth-service/internal/user/resource.go, gateway-prefixed):
 *   GET /api/auth/users?ids=a,b,c — batch display-name lookup
 *
 * The endpoint is scoped to the CALLER'S ACTIVE FLEET server-side: ids outside
 * that fleet, and ids with no user row, are silently omitted from `data`. A
 * shorter response than the request asked for is therefore normal, never an
 * error condition to surface.
 */
import { BaseService, type ListResult } from './BaseService';
import type { UserAttributes } from '../../types/models/user';

class UserService extends BaseService<UserAttributes> {
  protected readonly resourceType = 'users';
  protected readonly basePath = '/api/auth/users';

  /**
   * GET /api/auth/users?ids=a,b,c
   *
   * Deliberately does NOT chunk. The server caps `ids` at 100 and a household
   * fleet is single-digit, so chunking here would be speculative and untested.
   * If the activity feed later needs it, it belongs there — that is where the
   * id set is actually unbounded.
   */
  async listByIds(ids: string[]): Promise<ListResult<UserAttributes>> {
    const query = ids.map((id) => encodeURIComponent(id)).join(',');
    return this.listAt(`${this.basePath}?ids=${query}`);
  }
}

export const userService = new UserService();
